package astutil

import (
	"fmt"
	"go/parser"
	"go/token"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestUsesImport(t *testing.T) {
	t.Parallel()

	type args struct {
		fileData       string
		path           string
		packageImports map[string]string
	}

	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "success with github.com/go-pg/pg/v9",
			args: args{
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
			},
			want: true,
		},
		{
			name: `success with "pg2 github.com/go-pg/pg/v9"`,
			args: args{
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
			},
			want: true,
		},
		{
			name: "success with strconv",
			args: args{
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
			},
			want: true,
		},
		{
			name: "success without ast",
			args: args{
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
			},
			want: false,
		},
		{
			name: "success with github.com/zchee/goimports-rereviser/testdata/innderpkg",
			args: args{
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
			},
			want: true,
		},
		{
			name: "success with unused strconv",
			args: args{
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
			},
			want: false,
		},
	}
	for _, tt := range tests {
		fileData := tt.args.fileData

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "", []byte(fileData), parser.ParseComments)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got := UsesImport(f, tt.args.packageImports, tt.args.path)

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUsedImports(t *testing.T) {
	t.Parallel()

	type args struct {
		fileData       string
		packageImports map[string]string
	}

	tests := []struct {
		name string
		args args
		want map[string]bool
	}{
		{
			name: "reports used and unused imports",
			args: args{
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
			},
			want: map[string]bool{
				"fmt":                    true,
				"github.com/go-pg/pg/v9": true,
				"strconv":                false,
			},
		},
		{
			name: "respects explicit alias",
			args: args{
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
			},
			want: map[string]bool{
				"github.com/go-pg/pg/v9": true,
			},
		},
		{
			name: "marks blank and dot imports as used",
			args: args{
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
			},
			want: map[string]bool{
				"github.com/go-pg/pg/v9": true,
				"fmt":                    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "", []byte(tt.args.fileData), parser.ParseComments)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got := UsedImports(f, tt.args.packageImports)
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
	t.Parallel()

	type args struct {
		dir      string
		filename string
	}

	tests := []struct {
		name    string
		args    args
		want    map[string]string
		wantErr bool
	}{
		{
			name: "success",
			args: args{
				dir:      "./testdata/",
				filename: "testdata.go",
			},
			want: map[string]string{
				"fmt":                     "fmt",
				"golang.org/x/exp/slices": "slices",
			},
			wantErr: false,
		},
		{
			name: "success with deprecated build tag",
			args: args{
				dir:      "./testdata/",
				filename: "testdata_with_deprecated_build_tag.go",
			},
			want: map[string]string{
				"fmt":                     "fmt",
				"golang.org/x/exp/slices": "slices",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f, err := parser.ParseFile(
				token.NewFileSet(),
				fmt.Sprintf("%s/%s", tt.args.dir, tt.args.filename),
				nil,
				parser.ParseComments,
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, err := LoadPackageDependencies(tt.args.dir, ParseBuildTag(f))
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, map[string]string(got)); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestLoadPackageDependenciesSingleflight(t *testing.T) {
	t.Parallel()

	ClearPackageDepsCache()

	originalLoader := loadPackageDependenciesFunc
	defer func() { loadPackageDependenciesFunc = originalLoader }()

	var callCount int32
	ready := make(chan struct{})
	proceed := make(chan struct{})

	loadPackageDependenciesFunc = func(dir, buildTag string) (PackageImports, error) {
		if atomic.AddInt32(&callCount, 1) == 1 {
			close(ready)
		}
		<-proceed
		return PackageImports{"example.com/pkg": "pkg"}, nil
	}

	const goroutineCount = 8
	var wg sync.WaitGroup
	errCh := make(chan error, goroutineCount)

	for i := 0; i < goroutineCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
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
		}()
	}

	<-ready
	close(proceed)
	wg.Wait()

	for i := 0; i < goroutineCount; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("goroutine returned error: %v", err)
		}
	}

	if got := atomic.LoadInt32(&callCount); got != 1 {
		t.Fatalf("expected single loader invocation, got %d", got)
	}

	// Ensure cached result is reused without invoking loader again.
	loadPackageDependenciesFunc = func(dir, buildTag string) (PackageImports, error) {
		atomic.AddInt32(&callCount, 1)
		return nil, fmt.Errorf("unexpected loader invocation")
	}

	imports, err := LoadPackageDependencies("/tmp/test", "")
	if err != nil {
		t.Fatalf("unexpected error retrieving from cache: %v", err)
	}
	if imports["example.com/pkg"] != "pkg" {
		t.Fatalf("cached result mismatch: %v", imports)
	}
	if got := atomic.LoadInt32(&callCount); got != 1 {
		t.Fatalf("cache miss incremented loader count: %d", got)
	}
}
