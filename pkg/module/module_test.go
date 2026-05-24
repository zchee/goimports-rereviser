package module

import (
	"os"
	"path/filepath"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
)

func TestGoModRootPathAndName(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		dir, err := os.Getwd()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		goModRootPath, err := GoModRootPath(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		got, err := Name(goModRootPath)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if diff := gocmp.Diff("github.com/zchee/goimports-rereviser/v4", got); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("path is not set error", func(t *testing.T) {
		t.Parallel()

		goModPath, err := GoModRootPath("")
		if err == nil {
			t.Error("expected error, got nil")
		}
		if goModPath != "" {
			t.Errorf("expected empty string, got: %v", goModPath)
		}
	})

	t.Run("path is empty", func(t *testing.T) {
		t.Parallel()

		goModPath, err := GoModRootPath(".")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		got, err := Name(goModPath)
		if err == nil {
			t.Error("expected error, got nil")
		}
		if got != "" {
			t.Errorf("expected empty string, got: %v", got)
		}
	})
}

func TestName(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		prepareFn func(t *testing.T) string
	}{
		"read empty go.mod": {
			prepareFn: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				f, err := os.Create(filepath.Join(dir, "go.mod"))
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if err := f.Close(); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return dir
			},
		},
		"check failed parsing of go.mod": {
			prepareFn: func(t *testing.T) string {
				t.Helper()
				dir := t.TempDir()
				file, err := os.Create(filepath.Join(dir, "go.mod"))
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if _, err := file.WriteString("mod test"); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if err := file.Close(); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return dir
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := Name(tt.prepareFn(t))
			if err == nil {
				t.Error("expected error, got nil")
			}
			if got != "" {
				t.Errorf("expected empty string, got: %v", got)
			}
		})
	}
}

func TestDetermineProjectName(t *testing.T) {
	t.Parallel()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tests := map[string]struct {
		projectName string
		filePath    string
		want        string
	}{
		"success with auto determining": {
			projectName: "",
			filePath:    filepath.Join(wd, "module.go"),
			want:        "github.com/zchee/goimports-rereviser/v4",
		},
		"success with manual set": {
			projectName: "github.com/zchee/goimports-rereviser/v4",
			filePath:    "",
			want:        "github.com/zchee/goimports-rereviser/v4",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := DetermineProjectName(tt.projectName, tt.filePath)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := gocmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
