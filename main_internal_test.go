package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zchee/goimports-rereviser/v4/reviser"
)

func TestProcessPathsProcessesEachFile(t *testing.T) {
	tmpDir := t.TempDir()
	fileA := filepath.Join(tmpDir, "a.go")
	fileB := filepath.Join(tmpDir, "b.go")

	input := `package main

import (
	"github.com/pkg/errors"
	"fmt"
)

func main() {}
`

	if err := os.WriteFile(fileA, []byte(input), 0o644); err != nil {
		t.Fatalf("failed to write fixture A: %v", err)
	}
	if err := os.WriteFile(fileB, []byte(input), 0o644); err != nil {
		t.Fatalf("failed to write fixture B: %v", err)
	}

	origCfg := cfg
	cfg = Config{
		projectName: "example.com/test",
		output:      "file",
	}
	defer func() { cfg = origCfg }()

	paths := []string{fileA, fileB}

	hasChange, err := processPaths(t.Context(), &cfg, paths, "", nil)
	if err != nil {
		t.Fatalf("processPaths returned error: %v", err)
	}
	if !hasChange {
		t.Fatalf("expected hasChange to be true")
	}

	assertFmtFirst := func(t *testing.T, path string) {
		t.Helper()
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read file %s: %v", path, err)
		}
		if !strings.Contains(string(content), "\n\t\"fmt\"\n\n\t\"github.com/pkg/errors\"") {
			t.Fatalf("file %s not rewritten as expected:\n%s", path, string(content))
		}
	}

	assertFmtFirst(t, fileA)
	assertFmtFirst(t, fileB)
}

func TestProcessPaths_SingleFileCacheWritePathStabilizes(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "main.go")

	input := []byte(`package main

import (
	"github.com/pkg/errors"
	"fmt"
)

func main() {
	fmt.Println(errors.New("cached"))
}
`)
	if err := os.WriteFile(filePath, input, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	origCfg := cfg
	cfg = Config{
		projectName:      "example.com/test",
		output:           "file",
		isUseCache:       true,
		useMetadataCache: true,
	}
	defer func() { cfg = origCfg }()

	hasChange, err := processPaths(t.Context(), &cfg, []string{filePath}, cacheDir, nil)
	if err != nil {
		t.Fatalf("first processPaths returned error: %v", err)
	}
	if !hasChange {
		t.Fatalf("expected first run to rewrite the file")
	}

	formatted, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read rewritten file: %v", err)
	}
	entry, err := reviser.ReadCacheEntry(cacheDir, filePath)
	if err != nil {
		t.Fatalf("failed to read cache entry: %v", err)
	}
	if entry == nil {
		t.Fatalf("expected cache entry after mutating run")
	}
	if got, want := entry.Hash, reviser.ComputeContentHash(formatted); got != want {
		t.Fatalf("cache hash mismatch: got %q want %q", got, want)
	}

	statAfterFirstRun, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("failed to stat rewritten file: %v", err)
	}
	if got, want := entry.Size, statAfterFirstRun.Size(); got != want {
		t.Fatalf("cache size mismatch: got %d want %d", got, want)
	}
	if got, want := entry.ModTime, statAfterFirstRun.ModTime().UTC().UnixNano(); got != want {
		t.Fatalf("cache modtime mismatch: got %d want %d", got, want)
	}

	stableModTime := time.Unix(1_700_000_000, 123_000_000).UTC()
	if err := os.Chtimes(filePath, stableModTime, stableModTime); err != nil {
		t.Fatalf("failed to set stable fixture mtime: %v", err)
	}
	stableStat, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("failed to stat stable fixture mtime: %v", err)
	}
	stableModTime = stableStat.ModTime().UTC()
	stableEntry, err := reviser.NewCacheEntry(filePath, reviser.ComputeContentHash(formatted), cfg.useMetadataCache)
	if err != nil {
		t.Fatalf("failed to rebuild cache entry after setting stable mtime: %v", err)
	}
	if err := reviser.WriteCacheEntry(cacheDir, filePath, stableEntry); err != nil {
		t.Fatalf("failed to write stable cache entry: %v", err)
	}

	hasChange, err = processPaths(t.Context(), &cfg, []string{filePath}, cacheDir, nil)
	if err != nil {
		t.Fatalf("second processPaths returned error: %v", err)
	}
	if hasChange {
		t.Fatalf("expected unchanged formatted file to stay unchanged on second run")
	}

	statAfterSecondRun, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("failed to stat file after second run: %v", err)
	}
	if got, want := statAfterSecondRun.ModTime().UTC().UnixNano(), stableModTime.UnixNano(); got != want {
		t.Fatalf("expected second run not to rewrite file: got mtime %d want %d", got, want)
	}
	if got, want := statAfterSecondRun.Size(), statAfterFirstRun.Size(); got != want {
		t.Fatalf("expected second run to preserve file size: got %d want %d", got, want)
	}
}

func TestProcessPaths_StdoutCacheDoesNotSuppressRepeatedOutput(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "stdout.go")

	input := []byte(`package main

import (
	"github.com/pkg/errors"
	"fmt"
)

func main() {
	fmt.Println(errors.New("stdout"))
}
`)
	if err := os.WriteFile(filePath, input, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	origCfg := cfg
	cfg = Config{
		projectName:      "example.com/test",
		output:           "stdout",
		isUseCache:       true,
		useMetadataCache: true,
	}
	defer func() { cfg = origCfg }()

	firstStdout := captureStdout(t, func() {
		hasChange, err := processPaths(t.Context(), &cfg, []string{filePath}, cacheDir, nil)
		if err != nil {
			t.Fatalf("first processPaths returned error: %v", err)
		}
		if !hasChange {
			t.Fatalf("expected first stdout run to detect a pending change")
		}
	})
	secondStdout := captureStdout(t, func() {
		hasChange, err := processPaths(t.Context(), &cfg, []string{filePath}, cacheDir, nil)
		if err != nil {
			t.Fatalf("second processPaths returned error: %v", err)
		}
		if !hasChange {
			t.Fatalf("expected second stdout run to continue reporting the unchanged on-disk diff")
		}
	})

	if firstStdout == "" {
		t.Fatalf("expected first stdout run to emit formatted content")
	}
	if firstStdout != secondStdout {
		t.Fatalf("expected repeated stdout output to stay stable\nfirst:\n%s\nsecond:\n%s", firstStdout, secondStdout)
	}

	entry, err := reviser.ReadCacheEntry(cacheDir, filePath)
	if err != nil {
		t.Fatalf("failed to read cache entry: %v", err)
	}
	if entry != nil {
		t.Fatalf("expected non-mutating stdout mode not to persist cache state, got %+v", *entry)
	}
}

func TestProcessPaths_ListDiffReportsOptionMismatchDespiteExistingCache(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "separate_named.go")

	input := []byte(`package main

import (
	"github.com/google/uuid"
	errors "github.com/pkg/errors"
)

func main() {
	_, _ = uuid.New(), errors.New("cached list")
}
`)
	if err := os.WriteFile(filePath, input, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	origCfg := cfg
	defer func() { cfg = origCfg }()

	cfg = Config{
		projectName:      "example.com/test",
		output:           "file",
		isUseCache:       true,
		useMetadataCache: true,
	}

	hasChange, err := processPaths(t.Context(), &cfg, []string{filePath}, cacheDir, nil)
	if err != nil {
		t.Fatalf("mutating processPaths returned error: %v", err)
	}
	if hasChange {
		t.Fatalf("expected default mutating run to leave already-default-formatted file unchanged")
	}
	entry, err := reviser.ReadCacheEntry(cacheDir, filePath)
	if err != nil {
		t.Fatalf("failed to read cache entry after mutating run: %v", err)
	}
	if entry == nil {
		t.Fatalf("expected mutating run to persist a cache entry")
	}

	cfg.listFileName = true
	cfg.shouldSeparateNamedImports = true
	stdout := captureStdout(t, func() {
		hasChange, err = processPaths(t.Context(), &cfg, []string{filePath}, cacheDir, reviser.SourceFileOptions{reviser.WithSeparatedNamedImports})
		if err != nil {
			t.Fatalf("list-diff processPaths returned error: %v", err)
		}
	})
	if !hasChange {
		t.Fatalf("expected list-diff with separate-named to detect option-sensitive formatting")
	}
	if got, want := stdout, filePath+"\n"; got != want {
		t.Fatalf("unexpected list-diff output after option change: got %q want %q", got, want)
	}
}

func TestProcessPaths_MutatingCacheRespectsOptionFingerprint(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "separate_named_write.go")

	input := []byte(`package main

import (
	"github.com/google/uuid"
	errors "github.com/pkg/errors"
)

func main() {
	_, _ = uuid.New(), errors.New("cached write")
}
`)
	if err := os.WriteFile(filePath, input, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	origCfg := cfg
	defer func() { cfg = origCfg }()

	cfg = Config{
		projectName:      "example.com/test",
		output:           "file",
		isUseCache:       true,
		useMetadataCache: true,
	}

	hasChange, err := processPaths(t.Context(), &cfg, []string{filePath}, cacheDir, nil)
	if err != nil {
		t.Fatalf("default processPaths returned error: %v", err)
	}
	if hasChange {
		t.Fatalf("expected default mutating run to leave already-default-formatted file unchanged")
	}

	cfg.shouldSeparateNamedImports = true
	hasChange, err = processPaths(t.Context(), &cfg, []string{filePath}, cacheDir, reviser.SourceFileOptions{reviser.WithSeparatedNamedImports})
	if err != nil {
		t.Fatalf("separate-named processPaths returned error: %v", err)
	}
	if !hasChange {
		t.Fatalf("expected separate-named run to bypass default cache entry and rewrite file")
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read rewritten fixture: %v", err)
	}
	if !bytes.Contains(content, []byte("\n\t\"github.com/google/uuid\"\n\n\terrors \"github.com/pkg/errors\"")) {
		t.Fatalf("expected named import to be separated after option change:\n%s", content)
	}
}

func TestProcessPaths_ListDiffWriteReportsOptionMismatchDespiteExistingCache(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "list_write_separate_named.go")

	input := []byte(`package main

import (
	"github.com/google/uuid"
	errors "github.com/pkg/errors"
)

func main() {
	_, _ = uuid.New(), errors.New("cached list write")
}
`)
	if err := os.WriteFile(filePath, input, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	origCfg := cfg
	defer func() { cfg = origCfg }()

	cfg = Config{
		projectName:      "example.com/test",
		output:           "file",
		isUseCache:       true,
		useMetadataCache: true,
	}

	hasChange, err := processPaths(t.Context(), &cfg, []string{filePath}, cacheDir, nil)
	if err != nil {
		t.Fatalf("default processPaths returned error: %v", err)
	}
	if hasChange {
		t.Fatalf("expected default mutating run to leave already-default-formatted file unchanged")
	}

	cfg.output = "write"
	cfg.listFileName = true
	cfg.shouldSeparateNamedImports = true
	stdout := captureStdout(t, func() {
		hasChange, err = processPaths(t.Context(), &cfg, []string{filePath}, cacheDir, reviser.SourceFileOptions{reviser.WithSeparatedNamedImports})
		if err != nil {
			t.Fatalf("list-diff write processPaths returned error: %v", err)
		}
	})
	if !hasChange {
		t.Fatalf("expected list-diff write to bypass default cache entry and report change")
	}
	if got, want := stdout, filePath+"\n"; got != want {
		t.Fatalf("unexpected list-diff write output after option change: got %q want %q", got, want)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read rewritten fixture: %v", err)
	}
	if !bytes.Contains(content, []byte("\n\t\"github.com/google/uuid\"\n\n\terrors \"github.com/pkg/errors\"")) {
		t.Fatalf("expected list-diff write to rewrite with separated named import:\n%s", content)
	}
}

func TestProcessPaths_ListDiffCacheDoesNotSuppressRepeatedReports(t *testing.T) {
	tmpDir := t.TempDir()
	cacheDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "list.go")

	input := []byte(`package main

import (
	"github.com/pkg/errors"
	"fmt"
)

func main() {
	fmt.Println(errors.New("list"))
}
`)
	if err := os.WriteFile(filePath, input, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	origCfg := cfg
	cfg = Config{
		projectName:      "example.com/test",
		output:           "file",
		listFileName:     true,
		isUseCache:       true,
		useMetadataCache: true,
	}
	defer func() { cfg = origCfg }()

	expected := filePath + "\n"
	firstStdout := captureStdout(t, func() {
		hasChange, err := processPaths(t.Context(), &cfg, []string{filePath}, cacheDir, nil)
		if err != nil {
			t.Fatalf("first processPaths returned error: %v", err)
		}
		if !hasChange {
			t.Fatalf("expected first list-diff run to report a pending change")
		}
	})
	secondStdout := captureStdout(t, func() {
		hasChange, err := processPaths(t.Context(), &cfg, []string{filePath}, cacheDir, nil)
		if err != nil {
			t.Fatalf("second processPaths returned error: %v", err)
		}
		if !hasChange {
			t.Fatalf("expected second list-diff run to keep reporting the unchanged file")
		}
	})

	if got := firstStdout; got != expected {
		t.Fatalf("unexpected first list-diff output:\nwant: %q\n got: %q", expected, got)
	}
	if got := secondStdout; got != expected {
		t.Fatalf("unexpected second list-diff output:\nwant: %q\n got: %q", expected, got)
	}

	contentAfterRuns, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read file after list-diff runs: %v", err)
	}
	if !bytes.Equal(contentAfterRuns, input) {
		t.Fatalf("expected list-diff mode to leave the file unchanged\nwant:\n%s\n got:\n%s", input, contentAfterRuns)
	}

	entry, err := reviser.ReadCacheEntry(cacheDir, filePath)
	if err != nil {
		t.Fatalf("failed to read cache entry: %v", err)
	}
	if entry != nil {
		t.Fatalf("expected list-only mode not to persist cache state, got %+v", *entry)
	}
}

func TestResultPostProcessUsesProvidedConfig(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "result.go")
	original := []byte("package main\n")
	formatted := []byte("package main\n\nfunc main() {}\n")
	if err := os.WriteFile(filePath, original, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	origCfg := cfg
	cfg = Config{
		output:       "stdout",
		listFileName: true,
	}
	defer func() { cfg = origCfg }()

	localCfg := Config{output: "file"}
	if err := resultPostProcess(&localCfg, true, filePath, formatted); err != nil {
		t.Fatalf("resultPostProcess returned error: %v", err)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read post-processed file: %v", err)
	}
	if !bytes.Equal(content, formatted) {
		t.Fatalf("expected provided file config to control output\nwant:\n%s\n got:\n%s", formatted, content)
	}
}

func TestCLI_DefaultCacheFastSkipDoesNotRequireUseCache(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "main.go")
	input := []byte(`package main

import "fmt"

func main() {
	fmt.Println("ok")
}
`)
	if err := os.WriteFile(filePath, input, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	cmd := exec.Command("go", "run", ".", filePath)
	cmd.Dir = "."
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected go run to succeed without --use-cache, got err=%v\noutput:\n%s", err, output)
	}
	if bytes.Contains(output, []byte("cache-fast-skip requires --use-cache")) {
		t.Fatalf("unexpected stale cache-fast-skip validation in output:\n%s", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}

	originalStdout := os.Stdout
	os.Stdout = writer
	defer func() {
		os.Stdout = originalStdout
	}()

	outputCh := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, reader)
		outputCh <- buf.String()
	}()

	fn()

	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close stdout writer: %v", err)
	}
	output := <-outputCh
	if err := reader.Close(); err != nil {
		t.Fatalf("failed to close stdout reader: %v", err)
	}

	return output
}
