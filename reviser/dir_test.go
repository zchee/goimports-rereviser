package reviser

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alitto/pond"
	gocmp "github.com/google/go-cmp/cmp"
)

const sep = string(os.PathSeparator)

const dirFixUnformatted = `package dir1
import (
	"strings"
	"fmt"
)
func main() {
	fmt.Println(strings.ToLower("Hello World!"))
}
`

const dirFindUnformatted = `package dir1
import (
	"strings"

	"fmt"
)
func main() {
	fmt.Println(strings.ToLower("Hello World!"))
}
`

// sortedDirContent computes the canonical sorted form of the unformatted
// fixture so excludes-tests can assert against it without depending on a
// shared mutable fixture.
func sortedDirContent(t *testing.T, projectName, unformatted string) string {
	t.Helper()
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "file1.go")
	if err := os.WriteFile(filePath, []byte(unformatted), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	data, _, changed, err := NewSourceFile(projectName, filePath).Fix()
	if err != nil {
		t.Fatalf("Fix on fixture: %v", err)
	}
	if !changed {
		t.Fatalf("expected fixture to be reformatted")
	}
	return string(data)
}

func TestNewSourceDir(t *testing.T) {
	t.Parallel()

	dir := NewSourceDir("project", recursivePath, false, "")
	if diff := gocmp.Diff("project", dir.projectName); diff != "" {
		t.Errorf("projectName mismatch (-want +got):\n%s", diff)
	}
	if strings.Contains(dir.dir, "/...") {
		t.Errorf("expected %q not to contain %q", dir.dir, "/...")
	}
	if !dir.isRecursive {
		t.Errorf("expected isRecursive to be true")
	}
	if got := len(dir.excludePatterns); got != 0 {
		t.Errorf("expected no exclude patterns, got %d", got)
	}
}

func TestSourceDir_Fix(t *testing.T) {
	const projectName = "testdata"

	sortedContent := sortedDirContent(t, projectName, dirFixUnformatted)

	tests := map[string]struct {
		excludes string
		want     string
	}{
		"exclude-file": {
			excludes: "dir1" + sep + "file1.go",
			want:     dirFixUnformatted,
		},
		"exclude-dir": {
			excludes: "dir1" + sep,
			want:     dirFixUnformatted,
		},
		"exclude-file-*": {
			excludes: "dir1" + sep + "f*1.go",
			want:     dirFixUnformatted,
		},
		"exclude-file-?": {
			excludes: "dir1" + sep + "file?.go",
			want:     dirFixUnformatted,
		},
		"exclude-file-multi": {
			excludes: "dir1" + sep + "file?.go,aaa,bbb",
			want:     dirFixUnformatted,
		},
		"not-exclude": {
			excludes: "dir1" + sep + "test.go",
			want:     sortedContent,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			rootDir := t.TempDir()
			pkgDir := filepath.Join(rootDir, "dir1")
			if err := os.MkdirAll(pkgDir, 0o755); err != nil {
				t.Fatalf("mkdir pkg: %v", err)
			}
			testFile := filepath.Join(pkgDir, "file1.go")
			if err := os.WriteFile(testFile, []byte(dirFixUnformatted), 0o644); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			if _, err := NewSourceDir(projectName, rootDir, true, tt.excludes).Fix(); err != nil {
				t.Fatalf("Fix: %v", err)
			}

			got, err := os.ReadFile(testFile)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			if diff := gocmp.Diff(tt.want, string(got)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSourceDir_IsExcluded(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		excludes string
		testPath string
		want     bool
	}{
		"default-vendor-dir": {
			excludes: "",
			testPath: filepath.Join("vendor"),
			want:     true,
		},
		"default-testdata-dir": {
			excludes: "",
			testPath: filepath.Join("nested", "testdata"),
			want:     true,
		},
		"default-dot-file": {
			excludes: "",
			testPath: ".hidden.go",
			want:     true,
		},
		"default-underscore-dir": {
			excludes: "",
			testPath: filepath.Join("_tmp"),
			want:     true,
		},
		"normal": {
			excludes: "test.go",
			testPath: "test.go",
			want:     true,
		},
		"dir": {
			excludes: "test/",
			testPath: "test",
			want:     true,
		},
		"wildcard-1": {
			excludes: "tes?.go",
			testPath: "test.go",
			want:     true,
		},
		"wildcard-2": {
			excludes: "t*.go",
			testPath: "test.go",
			want:     true,
		},
		"not-excluded": {
			excludes: "t*.go",
			testPath: "abc.go",
			want:     false,
		},
		"multi-excludes": {
			excludes: "t*.go,abc.go",
			testPath: "abc.go",
			want:     true,
		},
		"vendor-go-file-not-excluded": {
			excludes: "",
			testPath: "vendor.go",
			want:     false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := NewSourceDir("project", "project", true, tt.excludes).isExcluded(tt.testPath)
			if diff := gocmp.Diff(tt.want, got); diff != "" {
				t.Errorf("isExcluded mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSourceDir_Find(t *testing.T) {
	t.Parallel()

	const projectName = "testdata"

	rootDir := t.TempDir()
	pkgDir := filepath.Join(rootDir, "dir1")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("mkdir pkg: %v", err)
	}
	testFile := filepath.Join(pkgDir, "file1.go")
	if err := os.WriteFile(testFile, []byte(dirFindUnformatted), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	files, err := NewSourceDir(projectName, rootDir, true, "dir1"+sep+"test.go").Find()
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if diff := gocmp.Diff([]string{testFile}, files.List()); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
	}
}

func TestSourceDir_FindWithWorkerPoolWaitsForResults(t *testing.T) {
	t.Parallel()

	project := "github.com/example/project"
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "unformatted.go")
	unformatted := []byte("package testdata\n\nimport (\n\t\"github.com/pkg/errors\"\n\t\"fmt\"\n)\n\nfunc main() {\n\tfmt.Println(errors.New(\"dir list\"))\n}\n")
	if err := os.WriteFile(filePath, unformatted, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	pool := pond.New(1, 0)
	t.Cleanup(pool.StopAndWait)

	files, err := NewSourceDir(project, tmpDir, true, "").
		WithWorkerPool(pool).
		Find()
	if err != nil {
		t.Fatalf("Find returned error: %v", err)
	}
	if files == nil {
		t.Fatalf("expected Find to report the unformatted file")
	}
	if diff := gocmp.Diff([]string{filePath}, files.List()); diff != "" {
		t.Fatalf("unformatted files mismatch (-want +got):\n%s", diff)
	}
}

func TestSourceDir_FindDoesNotWriteCache(t *testing.T) {
	t.Parallel()

	project := "github.com/example/project"
	tmpDir := t.TempDir()
	cacheDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "unformatted.go")
	unformatted := []byte("package testdata\n\nimport (\n\t\"github.com/pkg/errors\"\n\t\"fmt\"\n)\n\nfunc main() {\n\tfmt.Println(errors.New(\"dir cache\"))\n}\n")
	if err := os.WriteFile(filePath, unformatted, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	files, err := NewSourceDir(project, tmpDir, true, "").
		WithCache(cacheDir).
		Find()
	if err != nil {
		t.Fatalf("Find returned error: %v", err)
	}
	if files == nil {
		t.Fatalf("expected Find to report the unformatted file")
	}

	entry, err := ReadCacheEntry(cacheDir, filePath)
	if err != nil {
		t.Fatalf("ReadCacheEntry returned error: %v", err)
	}
	if entry != nil {
		t.Fatalf("expected Find not to write cache entries, got %+v", *entry)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read fixture after Find: %v", err)
	}
	if diff := gocmp.Diff(string(unformatted), string(content)); diff != "" {
		t.Fatalf("Find should not mutate files (-want +got):\n%s", diff)
	}
}

func TestSourceDir_Fix_CacheRespectsFingerprint(t *testing.T) {
	t.Parallel()

	project := "github.com/example/project"
	tmpDir := t.TempDir()
	cacheDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "separate_named.go")

	input := []byte(`package testdata

import (
	"github.com/google/uuid"
	errors "github.com/pkg/errors"
)

func main() {
	_, _ = uuid.New(), errors.New("cached dir")
}
`)
	if err := os.WriteFile(filePath, input, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	if _, err := NewSourceDir(project, tmpDir, true, "").
		WithSequentialThreshold(0).
		WithCache(cacheDir).
		WithCacheFingerprint("default").
		Fix(); err != nil {
		t.Fatalf("default Fix returned error: %v", err)
	}

	contentAfterDefault, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read fixture after default Fix: %v", err)
	}
	if !bytes.Equal(contentAfterDefault, input) {
		t.Fatalf("expected default Fix to leave already-default-formatted file unchanged\nwant:\n%s\n got:\n%s", input, contentAfterDefault)
	}

	if _, err := NewSourceDir(project, tmpDir, true, "").
		WithSequentialThreshold(0).
		WithCache(cacheDir).
		WithCacheFingerprint("separate-named").
		Fix(WithSeparatedNamedImports); err != nil {
		t.Fatalf("separate-named Fix returned error: %v", err)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("failed to read rewritten fixture: %v", err)
	}
	if !bytes.Contains(content, []byte("\n\t\"github.com/google/uuid\"\n\n\terrors \"github.com/pkg/errors\"")) {
		t.Fatalf("expected directory Fix to rewrite with separated named import:\n%s", content)
	}
}

func TestSourceDir_Fix_ReturnsWriteErrorWithoutCaching(t *testing.T) {
	t.Parallel()

	project := "github.com/example/project"
	tmpDir := t.TempDir()
	cacheDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "write_error.go")
	writeErr := errors.New("injected write failure")

	unformatted := []byte(`package testdata

import (
	"github.com/pkg/errors"
	"fmt"
)

func main() {
	fmt.Println(errors.New("readonly"))
}
`)
	if err := os.WriteFile(filePath, unformatted, 0o644); err != nil {
		t.Fatalf("failed to write fixture: %v", err)
	}

	dir := NewSourceDir(project, tmpDir, true, "").
		WithSequentialThreshold(0).
		WithCache(cacheDir)
	dir.writeFile = func(string, []byte, fs.FileMode) error {
		return writeErr
	}

	_, err := dir.Fix()
	if err == nil {
		t.Fatalf("expected injected write failure to be returned")
	}
	if !strings.Contains(err.Error(), "failed to write fixed result to file") {
		t.Fatalf("expected write failure to be returned, got: %v", err)
	}
	if !errors.Is(err, writeErr) {
		t.Fatalf("expected returned error to wrap injected write failure, got: %v", err)
	}

	entry, readErr := ReadCacheEntry(cacheDir, filePath)
	if readErr != nil {
		t.Fatalf("ReadCacheEntry returned error: %v", readErr)
	}
	if entry != nil {
		t.Fatalf("expected failed write not to cache formatted content, got %+v", *entry)
	}
}

func TestSourceDirCacheDefaults(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	tests := map[string]struct {
		input []string
		want  []string
	}{
		"success": {
			input: []string{"1", "2"},
			want:  []string{"1", "2"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := newUnformattedCollection(tt.input).List()
			if diff := gocmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUnformattedCollection_String(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input []string
		want  string
	}{
		"success": {
			input: []string{"1", "2"},
			want: `1
2`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := newUnformattedCollection(tt.input).String()
			if diff := gocmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
