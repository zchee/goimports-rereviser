package reviser

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceFileFix_NoChangeFastPath(t *testing.T) {
	t.Parallel()

	clearTestCaches()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "noop.go")
	content := []byte(`package main

import "fmt"

func main() {
	fmt.Println("noop")
}
`)

	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	sf := NewSourceFile("example.com/test", filePath)
	got, original, changed, err := sf.Fix()
	if err != nil {
		t.Fatalf("Fix returned error: %v", err)
	}
	if changed {
		t.Fatalf("expected no change for already sorted imports")
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("expected output to match original\nwant: %q\n got: %q", original, got)
	}
}
