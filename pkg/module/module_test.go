package module

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
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
		if diff := cmp.Diff("github.com/zchee/goimports-rereviser/v4", got); diff != "" {
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

	tests := []struct {
		name      string
		prepareFn func() string
	}{
		{
			name: "read empty go.mod",
			prepareFn: func() string {
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
		{
			name: "check failed parsing of go.mod",
			prepareFn: func() string {
				dir := t.TempDir()
				file, err := os.Create(filepath.Join(dir, "go.mod"))
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				_, err = file.WriteString("mod test")
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if err := file.Close(); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return dir
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			goModRootPath := tt.prepareFn()
			got, err := Name(goModRootPath)
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

	type args struct {
		projectName string
		filePath    string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "success with auto determining",
			args: args{
				projectName: "",
				filePath: func() string {
					dir, err := os.Getwd()
					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}
					return filepath.Join(dir, "module.go")
				}(),
			},
			want: "github.com/zchee/goimports-rereviser/v4",
		},

		{
			name: "success with manual set",
			args: args{
				projectName: "github.com/zchee/goimports-rereviser/v4",
				filePath:    "",
			},
			want: "github.com/zchee/goimports-rereviser/v4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DetermineProjectName(tt.args.projectName, tt.args.filePath)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
