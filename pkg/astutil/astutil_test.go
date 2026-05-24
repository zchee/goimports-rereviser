package astutil

import (
	"fmt"
	"go/parser"
	"go/token"
	"sync"
	"sync/atomic"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
)

func TestUsesImport(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		fileData       string
		path           string
		packageImports map[string]string
		want           bool
	}{
		"success with github.com/go-pg/pg/v9": {
			fileData: `package main
import(
	"fmt"
	"github.com/go-pg/pg/v9"
	"strconv"
)

func main(){
	_ = strconv.Itoa(1)
	fmt.Println(pg.In([]string{"test"}))
}
`,
			path: "github.com/go-pg/pg/v9",
			packageImports: map[string]string{
				"github.com/go-pg/pg/v9": "pg",
			},
			want: true,
		},
		`success with "pg2 github.com/go-pg/pg/v9"`: {
			fileData: `package main
import(
	"fmt"
	pg2 "github.com/go-pg/pg/v9"
	"strconv"
)

func main(){
	_ = strconv.Itoa(1)
	fmt.Println(pg2.In([]string{"test"}))
}
`,
			path: "github.com/go-pg/pg/v9",
			want: true,
		},
		"success with strconv": {
			fileData: `package main
import(
	"fmt"
	"github.com/go-pg/pg/v9"
	"strconv"
)

func main(){
	_ = strconv.Itoa(1)
	fmt.Println(pg.In([]string{"test"}))
}
`,
			path: "strconv",
			packageImports: map[string]string{
				"strconv": "strconv",
			},
			want: true,
		},
		"success without ast": {
			fileData: `package main
import(
	"fmt"
	"github.com/go-pg/pg/v9"
	"strconv"
)

func main(){
	_ = strconv.Itoa(1)
	fmt.Println(pg.In([]string{"test"}))
}
`,
			path: "ast",
			want: false,
		},
		"success with github.com/zchee/goimports-rereviser/testdata/innderpkg": {
			fileData: `package main
import(
	"fmt"
	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
	"strconv"
)

func main(){
	_ = strconv.Itoa(1)
	fmt.Println(innderpkg.Something())
}
`,
			path: "github.com/zchee/goimports-rereviser/testdata/innderpkg",
			packageImports: map[string]string{
				"github.com/zchee/goimports-rereviser/testdata/innderpkg": "innderpkg",
			},
			want: true,
		},
		"success with unused strconv": {
			fileData: `package main
import(
	"fmt"
	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
	"strconv"
)

func main(){
	fmt.Println(innderpkg.Something())
}
`,
			path: "strconv",
			want: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "", []byte(tt.fileData), parser.ParseComments)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got := UsesImport(f, tt.packageImports, tt.path)

			if diff := gocmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUsedImports(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		fileData       string
		packageImports map[string]string
		want           map[string]bool
	}{
		"reports used and unused imports": {
			fileData: `package main
import(
	"fmt"
	"github.com/go-pg/pg/v9"
	"strconv"
)

func main(){
	fmt.Println(pg.In([]string{"test"}))
}
`,
			packageImports: map[string]string{
				"fmt":                    "fmt",
				"strconv":                "strconv",
				"github.com/go-pg/pg/v9": "pg",
			},
			want: map[string]bool{
				"fmt":                    true,
				"github.com/go-pg/pg/v9": true,
				"strconv":                false,
			},
		},
		"respects explicit alias": {
			fileData: `package main
import(
	pg2 "github.com/go-pg/pg/v9"
)

func main(){
	_ = pg2.In([]string{"test"})
}
`,
			packageImports: map[string]string{
				"github.com/go-pg/pg/v9": "pg",
			},
			want: map[string]bool{
				"github.com/go-pg/pg/v9": true,
			},
		},
		"marks blank and dot imports as used": {
			fileData: `package main
import(
	_ "github.com/go-pg/pg/v9"
	. "fmt"
)

func main(){
	Println("ok")
}
`,
			packageImports: map[string]string{
				"fmt":                    "fmt",
				"github.com/go-pg/pg/v9": "pg",
			},
			want: map[string]bool{
				"github.com/go-pg/pg/v9": true,
				"fmt":                    true,
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "", []byte(tt.fileData), parser.ParseComments)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got := UsedImports(f, tt.packageImports)
			for path, wantUsed := range tt.want {
				gotUsed, ok := got[path]
				if wantUsed {
					if !ok || !gotUsed {
						t.Errorf("expected %s to be marked used, got %v", path, gotUsed)
					}
					continue
				}
				if ok && gotUsed {
					t.Errorf("expected %s to be unused", path)
				}
			}
		})
	}
}

func TestLoadPackageDeps(t *testing.T) {
	tests := map[string]struct {
		dir      string
		filename string
		want     map[string]string
		wantErr  bool
	}{
		"success": {
			dir:      "./testdata/",
			filename: "testdata.go",
			want: map[string]string{
				"fmt":                     "fmt",
				"golang.org/x/exp/slices": "slices",
			},
		},
		"success with deprecated build tag": {
			dir:      "./testdata/",
			filename: "testdata_with_deprecated_build_tag.go",
			want: map[string]string{
				"fmt":                     "fmt",
				"golang.org/x/exp/slices": "slices",
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			f, err := parser.ParseFile(
				token.NewFileSet(),
				fmt.Sprintf("%s/%s", tt.dir, tt.filename),
				nil,
				parser.ParseComments,
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, err := LoadPackageDependencies(tt.dir, ParseBuildTag(f))
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if diff := gocmp.Diff(tt.want, map[string]string(got)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadPackageDependenciesSingleflight(t *testing.T) {
	ClearPackageDepsCache()

	originalLoader := loadPackageDependenciesFunc
	t.Cleanup(func() { loadPackageDependenciesFunc = originalLoader })

	var callCount atomic.Int32
	ready := make(chan struct{})
	proceed := make(chan struct{})

	loadPackageDependenciesFunc = func(dir, buildTag string) (PackageImports, error) {
		if callCount.Add(1) == 1 {
			close(ready)
		}
		<-proceed
		return PackageImports{"example.com/pkg": "pkg"}, nil
	}

	const goroutineCount = 8
	wg := new(sync.WaitGroup)
	errCh := make(chan error, goroutineCount)

	for range goroutineCount {
		wg.Go(func() {
			imports, err := LoadPackageDependencies("/tmp/test", "")
			if err != nil {
				errCh <- fmt.Errorf("LoadPackageDependencies failed: %w", err)
				return
			}
			if imports["example.com/pkg"] != "pkg" {
				errCh <- fmt.Errorf("unexpected imports map: %v", imports)
				return
			}
			errCh <- nil
		})
	}

	<-ready
	close(proceed)
	wg.Wait()

	for range goroutineCount {
		if err := <-errCh; err != nil {
			t.Fatalf("goroutine returned error: %v", err)
		}
	}

	if got := callCount.Load(); got != 1 {
		t.Fatalf("expected single loader invocation, got %d", got)
	}

	loadPackageDependenciesFunc = func(dir, buildTag string) (PackageImports, error) {
		callCount.Add(1)
		return nil, fmt.Errorf("unexpected loader invocation")
	}

	imports, err := LoadPackageDependencies("/tmp/test", "")
	if err != nil {
		t.Fatalf("unexpected error retrieving from cache: %v", err)
	}
	if imports["example.com/pkg"] != "pkg" {
		t.Fatalf("cached result mismatch: %v", imports)
	}
	if got := callCount.Load(); got != 1 {
		t.Fatalf("cache miss incremented loader count: %d", got)
	}
}
