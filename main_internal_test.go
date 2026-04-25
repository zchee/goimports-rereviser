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

	time.Sleep(5 * time.Millisecond)

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
	if got, want := statAfterSecondRun.ModTime().UTC().UnixNano(), statAfterFirstRun.ModTime().UTC().UnixNano(); got != want {
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
