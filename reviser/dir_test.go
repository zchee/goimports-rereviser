package reviser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const sep = string(os.PathSeparator)

func TestNewSourceDir(t *testing.T) {
	t.Run("should generate source dir from recursive path", func(tt *testing.T) {
		dir := NewSourceDir("project", recursivePath, false, "")
		if diff := cmp.Diff("project", dir.projectName); diff != "" {
			tt.Errorf("mismatch (-want +got):\n%s", diff)
		}
		if strings.Contains(dir.dir, "/...") {
			tt.Errorf("expected %q not to contain %q", dir.dir, "/...")
		}
		if diff := cmp.Diff(true, dir.isRecursive); diff != "" {
			tt.Errorf("mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(0, len(dir.excludePatterns)); diff != "" {
			tt.Errorf("mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestSourceDir_Fix(t *testing.T) {
	testFile := "testdata/dir/dir1/file1.go"

	originContent := `package dir1
import (
	"strings"
	"fmt"
)
func main() {
	fmt.Println(strings.ToLower("Hello World!"))
}
`
	exec := func(tt *testing.T, fn func(*testing.T) error) {
		// create test file
		err := os.MkdirAll(filepath.Dir(testFile), os.ModePerm)
		if err != nil {
			tt.Errorf("unexpected error: %v", err)
		}
		err = os.WriteFile(testFile, []byte(originContent), os.ModePerm)
		if err != nil {
			tt.Errorf("unexpected error: %v", err)
		}

		// exec test func
		err = fn(tt)
		if err != nil {
			tt.Errorf("unexpected error: %v", err)
		}

		// remove test file
		err = os.Remove(testFile)
		if err != nil {
			tt.Errorf("unexpected error: %v", err)
		}
	}
	var sortedContent string
	exec(t, func(tt *testing.T) error {
		// get sorted content via SourceFile.Fix
		sortedData, _, changed, err := NewSourceFile("testdata", testFile).Fix()
		if err != nil {
			tt.Errorf("unexpected error: %v", err)
		}
		sortedContent = string(sortedData)
		if diff := cmp.Diff(true, changed); diff != "" {
			tt.Errorf("mismatch (-want +got):\n%s", diff)
		}
		if originContent == sortedContent {
			tt.Errorf("expected content to be different")
		}
		return nil
	})

	type args struct {
		project  string
		path     string
		excludes string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "exclude-file",
			args: args{project: "testdata", path: "testdata/dir", excludes: "dir1" + sep + "file1.go"},
			want: originContent,
		}, {
			name: "exclude-dir",
			args: args{project: "testdata", path: "testdata/dir", excludes: "dir1" + sep},
			want: originContent,
		}, {
			name: "exclude-file-*",
			args: args{project: "testdata", path: "testdata/dir", excludes: "dir1" + sep + "f*1.go"},
			want: originContent,
		}, {
			name: "exclude-file-?",
			args: args{project: "testdata", path: "testdata/dir", excludes: "dir1" + sep + "file?.go"},
			want: originContent,
		}, {
			name: "exclude-file-multi",
			args: args{project: "testdata", path: "testdata/dir", excludes: "dir1" + sep + "file?.go,aaa,bbb"},
			want: originContent,
		}, {
			name: "not-exclude",
			args: args{project: "testdata", path: "testdata/dir", excludes: "dir1" + sep + "test.go"},
			want: sortedContent,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			exec(tt, func(ttt *testing.T) error {
				// executing SourceDir.Fix
				err := NewSourceDir(test.args.project, test.args.path, true, test.args.excludes).Fix()
				if err != nil {
					tt.Errorf("unexpected error: %v", err)
				}
				// read new content
				content, err := os.ReadFile(testFile)
				if err != nil {
					tt.Errorf("unexpected error: %v", err)
				}
				if diff := cmp.Diff(test.want, string(content)); diff != "" {
					tt.Errorf("mismatch (-want +got):\n%s", diff)
				}
				return nil
			})
		})
	}
}

func TestSourceDir_IsExcluded(t *testing.T) {
	type args struct {
		project  string
		path     string
		excludes string
		testPath string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "default-vendor-dir",
			args: args{project: "project", path: "project", excludes: "", testPath: filepath.Join("vendor")},
			want: true,
		},
		{
			name: "default-testdata-dir",
			args: args{project: "project", path: "project", excludes: "", testPath: filepath.Join("nested", "testdata")},
			want: true,
		},
		{
			name: "default-dot-file",
			args: args{project: "project", path: "project", excludes: "", testPath: ".hidden.go"},
			want: true,
		},
		{
			name: "default-underscore-dir",
			args: args{project: "project", path: "project", excludes: "", testPath: filepath.Join("_tmp")},
			want: true,
		},
		{
			name: "normal",
			args: args{project: "project", path: "project", excludes: "test.go", testPath: "test.go"},
			want: true,
		},
		{
			name: "dir",
			args: args{project: "project", path: "project", excludes: "test/", testPath: "test"},
			want: true,
		},
		{
			name: "wildcard-1",
			args: args{project: "project", path: "project", excludes: "tes?.go", testPath: "test.go"},
			want: true,
		},
		{
			name: "wildcard-2",
			args: args{project: "project", path: "project", excludes: "t*.go", testPath: "test.go"},
			want: true,
		},
		{
			name: "not-excluded",
			args: args{project: "project", path: "project", excludes: "t*.go", testPath: "abc.go"},
			want: false,
		},
		{
			name: "multi-excludes",
			args: args{project: "project", path: "project", excludes: "t*.go,abc.go", testPath: "abc.go"},
			want: true,
		},
		{
			name: "vendor-go-file-not-excluded",
			args: args{project: "project", path: "project", excludes: "", testPath: "vendor.go"},
			want: false,
		},
	}

	for _, test := range tests {
		args := test.args
		t.Run(test.name, func(tt *testing.T) {
			excluded := NewSourceDir(args.project, args.path, true, args.excludes).isExcluded(args.testPath)
			if diff := cmp.Diff(test.want, excluded); diff != "" {
				tt.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSourceDir_Find(t *testing.T) {
	testFile := "testdata/dir/dir1/file1.go"

	originContent := `package dir1
import (
	"strings"

	"fmt"
)
func main() {
	fmt.Println(strings.ToLower("Hello World!"))
}
`
	exec := func(tt *testing.T, fn func(*testing.T) error) {
		// create test file
		err := os.MkdirAll(filepath.Dir(testFile), os.ModePerm)
		if err != nil {
			tt.Errorf("unexpected error: %v", err)
		}
		err = os.WriteFile(testFile, []byte(originContent), os.ModePerm)
		if err != nil {
			tt.Errorf("unexpected error: %v", err)
		}

		// exec test func
		err = fn(tt)
		if err != nil {
			tt.Errorf("unexpected error: %v", err)
		}

		// remove test file
		err = os.Remove(testFile)
		if err != nil {
			tt.Errorf("unexpected error: %v", err)
		}
	}
	var sortedContent string
	exec(t, func(tt *testing.T) error {
		sortedData, _, changed, err := NewSourceFile("testdata", testFile).Fix()
		if err != nil {
			tt.Errorf("unexpected error: %v", err)
		}
		sortedContent = string(sortedData)
		if diff := cmp.Diff(true, changed); diff != "" {
			tt.Errorf("mismatch (-want +got):\n%s", diff)
		}
		if originContent == sortedContent {
			tt.Errorf("expected content to be different")
		}
		return nil
	})

	type args struct {
		project  string
		path     string
		excludes string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "found-unformatted",
			args: args{project: "testdata", path: "testdata/dir", excludes: "dir1" + sep + "test.go"},
			want: []string{"testdata/dir/dir1/file1.go"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			exec(tt, func(ttt *testing.T) error {
				files, err := NewSourceDir(test.args.project, test.args.path, true, test.args.excludes).Find()
				if err != nil {
					tt.Errorf("unexpected error: %v", err)
				}
				rootPath, err := os.Getwd()
				if err != nil {
					tt.Errorf("unexpected error: %v", err)
				}
				var want []string
				for _, w := range test.want {
					want = append(want, filepath.Join(rootPath, w))
				}
				if diff := cmp.Diff(want, files.List()); diff != "" {
					tt.Errorf("mismatch (-want +got):\n%s", diff)
				}
				return nil
			})
		})
	}
}

func TestSourceDir_Fix_CacheSkipsUnchangedFiles(t *testing.T) {
	t.Parallel()

	project := "github.com/example/project"
	tmpDir := t.TempDir()
	cacheRoot := t.TempDir()
	cacheDir := filepath.Join(cacheRoot, "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("failed to create cache directory: %v", err)
	}

	filePath := filepath.Join(tmpDir, "cached.go")
	unformatted := []byte(`package testdata

import (
	"github.com/pkg/errors"
	"fmt"
)

func main() {
	fmt.Println(errors.New("cached"))
}
`)

	if err := os.WriteFile(filePath, unformatted, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	dir := NewSourceDir(project, tmpDir, true, "").
		WithSequentialThreshold(0).
		WithCache(cacheDir)

	if err := dir.Fix(); err != nil {
		t.Fatalf("Fix returned error: %v", err)
	}

	skip, err := dir.shouldSkipByCache(filePath)
	if err != nil {
		t.Fatalf("shouldSkipByCache returned error: %v", err)
	}
	if !skip {
		t.Fatalf("expected cache to skip unchanged file")
	}

	// mutate the file and verify the cache no longer skips processing
	if err := os.WriteFile(filePath, unformatted[:len(unformatted)-1], 0o644); err != nil {
		t.Fatalf("failed to modify fixture: %v", err)
	}

	skip, err = dir.shouldSkipByCache(filePath)
	if err != nil {
		t.Fatalf("shouldSkipByCache returned error: %v", err)
	}
	if skip {
		t.Fatalf("expected cache miss after file modification")
	}
}

func TestSourceDirCacheDefaults(t *testing.T) {
	project := "github.com/zchee/goimports-rereviser/v4"
	tmpDir := t.TempDir()
	cacheDir := t.TempDir()

	dir := NewSourceDir(project, tmpDir, false, "").WithCache(cacheDir)
	if !dir.useMetadataCache {
		t.Fatalf("expected metadata cache to be enabled by default once cache is configured")
	}

	dir = dir.WithoutMetadataCache()
	if dir.useMetadataCache {
		t.Fatalf("expected WithoutMetadataCache to disable metadata usage")
	}

	dir = dir.WithMetadataCache()
	if !dir.useMetadataCache {
		t.Fatalf("expected WithMetadataCache to re-enable metadata path")
	}
}

func TestUnformattedCollection_List(t *testing.T) {
	tests := []struct {
		name    string
		init    func(t *testing.T) *UnformattedCollection
		inspect func(r *UnformattedCollection, t *testing.T) // inspects receiver after test run

		want1 []string
	}{
		{
			name: "sucess",
			init: func(t *testing.T) *UnformattedCollection {
				return newUnformattedCollection([]string{"1", "2"})
			},
			inspect: func(r *UnformattedCollection, t *testing.T) {
			},
			want1: []string{"1", "2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receiver := tt.init(t)
			got1 := receiver.List()

			if tt.inspect != nil {
				tt.inspect(receiver, t)
			}

			if diff := cmp.Diff(tt.want1, got1); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUnformattedCollection_String(t *testing.T) {
	tests := []struct {
		name    string
		init    func(t *testing.T) *UnformattedCollection
		inspect func(r *UnformattedCollection, t *testing.T) // inspects receiver after test run
		want    string
	}{
		{
			name: "success",
			init: func(t *testing.T) *UnformattedCollection {
				return newUnformattedCollection([]string{"1", "2"})
			},
			inspect: func(r *UnformattedCollection, t *testing.T) {
			},
			want: `1
2`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receiver := tt.init(t)
			if tt.inspect != nil {
				tt.inspect(receiver, t)
			}
			if diff := cmp.Diff(tt.want, receiver.String()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
