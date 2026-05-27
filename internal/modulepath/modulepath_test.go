package modulepath

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestDetermineProjectNameReturnsUndefinedModuleWhenNoGoModRootExists(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "nested", "main.go")

	got, err := DetermineProjectName("", filePath)
	if err == nil {
		t.Fatalf("expected missing go.mod root to return UndefinedModuleError")
	}
	if got != "" {
		t.Fatalf("expected empty project name on error, got %q", got)
	}
	var undefinedErr *UndefinedModuleError
	if !errors.As(err, &undefinedErr) {
		t.Fatalf("expected UndefinedModuleError, got %T: %v", err, err)
	}
}
