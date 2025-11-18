package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
