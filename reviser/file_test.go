package reviser

import (
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"golang.org/x/tools/txtar"
)

// Common fixture identifiers used across SourceFile.Fix table tests.
// The vast majority of cases reuse the same module path and example
// fixture, so naming them once keeps individual cases focused on the
// scenario-specific archive content.
const (
	testProjectName = "github.com/zchee/goimports-rereviser"
	testFilePath    = "./testdata/example.go"
	testCgoFilePath = "./testdata/cgo_example.go"
)

// parseTestArchive extracts input and expected output from a txtar archive.
func parseTestArchive(t *testing.T, archive string) (input, want []byte) {
	t.Helper()

	// Clear caches before each test to prevent pollution between tests
	clearTestCaches()

	ar := txtar.Parse([]byte(archive))
	for _, f := range ar.Files {
		switch f.Name {
		case "input.go":
			input = f.Data
		case "want.go":
			want = f.Data
		}
	}
	if input == nil {
		t.Fatal("txtar archive must contain input.go")
	}
	return input, want
}

// runFixCase executes a SourceFile.Fix table case end-to-end: parse the
// txtar archive, materialize the fixture at filePath (unless filePath is
// the standard-input sentinel or a does-not-exist path), invoke Fix with
// the given options, and assert wantErr/wantChange/output against the
// archive's want.go section.
//
// filePath semantics:
//   - StandardInput or a path containing "does-not-exist": no file is
//     written; the path is passed through to Fix unchanged so the
//     stdin/missing-file branches inside Fix run.
//   - any other value: the input is written verbatim to that path.
//     Callers that want isolation should pass a t.TempDir()-rooted path.
func runFixCase(t *testing.T, projectName, filePath, archive string, wantChange, wantErr bool, opts ...SourceFileOption) {
	t.Helper()

	input, want := parseTestArchive(t, archive)

	if filePath != StandardInput && !strings.Contains(filePath, "does-not-exist") {
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatalf("failed to create test directory: %v", err)
		}
		if err := os.WriteFile(filePath, input, 0o644); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(filePath) })
	}

	got, _, hasChange, err := NewSourceFile(projectName, filePath).Fix(opts...)

	if wantErr {
		if err == nil {
			t.Error("expected error but got none")
		}
		return
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasChange != wantChange {
		t.Errorf("hasChange = %v, want %v\ninput len=%d, got len=%d, want len=%d", hasChange, wantChange, len(input), len(got), len(want))
	}
	if want != nil {
		if diff := gocmp.Diff(string(want), string(got)); diff != "" {
			t.Errorf("output mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestIsLinknameBlankImport(t *testing.T) {
	tests := map[string]struct {
		imprt   string
		comment string
		want    bool
	}{
		"original go linkname marker": {
			imprt:   `_ "unsafe"`,
			comment: "// for go:linkname",
			want:    true,
		},
		"short linkname marker": {
			imprt:   `_ "unsafe"`,
			comment: "// for linkname",
			want:    true,
		},
		"go linkname usage marker": {
			imprt:   `_ "unsafe"`,
			comment: "// added for go linkname usage",
			want:    true,
		},
		"runtime dependency linkname marker": {
			imprt:   `_ "unsafe"`,
			comment: "// depends on the runtime via a linkname'd function",
			want:    true,
		},
		"non blank import is not linkname blank import": {
			imprt:   `"unsafe"`,
			comment: "// for go:linkname",
			want:    false,
		},
		"unrelated side effect comment": {
			imprt:   `_ "embed"`,
			comment: "// pulls in side effects",
			want:    false,
		},
		"block comment is not accepted": {
			imprt:   `_ "unsafe"`,
			comment: "/* for go:linkname */",
			want:    false,
		},
		"function-specific linkname marker is accepted": {
			imprt:   `_ "unsafe"`,
			comment: "// for go:linkname localFunc",
			want:    true,
		},
		"spaced line comment marker is accepted": {
			imprt:   `_ "unsafe"`,
			comment: "//   for linkname",
			want:    true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			meta := &commentsMetadata{
				Comment: &ast.CommentGroup{
					List: []*ast.Comment{{Text: tt.comment}},
				},
			}

			got := isLinknameBlankImport(tt.imprt, meta)
			if got != tt.want {
				t.Fatalf("isLinknameBlankImport(%q, comment %q) = %v, want %v", tt.imprt, tt.comment, got, tt.want)
			}
		})
	}

	t.Run("nil metadata is not accepted", func(t *testing.T) {
		if isLinknameBlankImport(`_ "unsafe"`, nil) {
			t.Fatal("isLinknameBlankImport accepted nil metadata")
		}
	})
}

func TestSourceFile_Fix(t *testing.T) {
	tests := map[string]struct {
		projectName string
		filePath    string
		archive     string
		wantChange  bool
		wantErr     bool
	}{
		"success with comments": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/v4/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
-- want.go --
package testdata

import (
	"bytes"
	"log"

	"golang.org/x/tools/go/packages"

	"github.com/zchee/goimports-rereviser/v4/testdata/innderpkg"
)

// nolint:gomnd
`,
			wantChange: true,
			wantErr:    false,
		},
		"auto-generated header": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
// Code generated by some tool
package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
-- want.go --
// Code generated by some tool
package testdata

import (
	"bytes"
	"log"

	"golang.org/x/tools/go/packages"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)

// nolint:gomnd
`,
			wantChange: true,
			wantErr:    false,
		},
		"auto-generated DO NOT EDIT header": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
// Code generated by some tool, DO NOT EDIT.
package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
-- want.go --
// Code generated by some tool, DO NOT EDIT.
package testdata

import (
	"bytes"
	"log"

	"golang.org/x/tools/go/packages"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)

// nolint:gomnd
`,
			wantChange: true,
			wantErr:    false,
		},
		"success with directive": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"fmt"
	"crypto/sha1" //nolint:gosec // Usage is for collision avoidance not security.
)

func main() {
	s := sha1.New()
	s.Write([]byte("example"))
	fmt.Printf("%x\n", s.Sum(nil))
}
-- want.go --
package testdata

import (
	"crypto/sha1" //nolint:gosec // Usage is for collision avoidance not security.
	"fmt"
)

func main() {
	s := sha1.New()
	s.Write([]byte("example"))
	fmt.Printf("%x\n", s.Sum(nil))
}
`,
			wantChange: true,
			wantErr:    false,
		},
		"success with std & project deps": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"


)

// nolint:gomnd
-- want.go --
package testdata

import (
	"bytes"
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)

// nolint:gomnd
`,
			wantChange: true,
			wantErr:    false,
		},
		"success with std & third-party deps": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
"log"

"bytes"

"golang.org/x/tools/go/packages"
)

// nolint:gomnd
-- want.go --
package testdata

import (
	"bytes"
	"log"

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
`,
			wantChange: true,
			wantErr:    false,
		},
		"success with std deps only": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata
		
import (
"log"

"bytes"
)

// nolint:gomnd
-- want.go --
package testdata

import (
	"bytes"
	"log"
)

// nolint:gomnd
`,
			wantChange: true,
			wantErr:    false,
		},
		"success with single std deps only": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import "log"

// nolint:gomnd
-- want.go --
package testdata

import "log"

// nolint:gomnd
`,
			wantChange: false,
			wantErr:    false,
		},
		"success with third-party deps only": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
-- want.go --
package testdata

import (
	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
`,
			wantChange: true,
			wantErr:    false,
		},
		"success with single third-party deps": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import "golang.org/x/tools/go/packages"

// nolint:gomnd
-- want.go --
package testdata

import "golang.org/x/tools/go/packages"

// nolint:gomnd
`,
			wantChange: false,
			wantErr:    false,
		},
		"success with project deps only": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

)

// nolint:gomnd
-- want.go --
package testdata

import (
	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)

// nolint:gomnd
`,
			wantChange: true,
			wantErr:    false,
		},
		"success with preserved doc comment for import": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"fmt"


	// test
	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)

// nolint:gomnd
-- want.go --
package testdata

import (
	"fmt"

	// test
	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)

// nolint:gomnd
`,
			wantChange: true,
			wantErr:    false,
		},
		"keep comment before side-effect import": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"unsafe"

	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/mem"

	// Guarantee that the built-in proto is called registered before this one
	// so that it can be replaced.
	_ "google.golang.org/grpc/encoding/proto"
)

-- want.go --
package testdata

import (
	"unsafe"

	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/mem"

	// Guarantee that the built-in proto is called registered before this one
	// so that it can be replaced.
	_ "google.golang.org/grpc/encoding/proto"
)
`,
			wantChange: true,
			wantErr:    false,
		},
		"success with comment for import": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"github.com/zchee/goimports-rereviser/testdata/innderpkg" // test1
	
	"fmt" //test2
	// this should be skipped
)

// nolint:gomnd
-- want.go --
package testdata

import (
	"fmt" //test2

	"github.com/zchee/goimports-rereviser/testdata/innderpkg" // test1
)

// nolint:gomnd
`,
			wantChange: true,
			wantErr:    false,
		},
		"success with no changes": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)

// nolint:gomnd
-- want.go --
package testdata

import (
	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)

// nolint:gomnd
`,
			wantChange: false,
			wantErr:    false,
		},
		"success no changes by imports and comments": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/lib/pq" // configure database/sql with postgres driver
	"go.uber.org/fx"
	"golang.org/x/tools/go/packages"
	"github.com/zchee/goimports-rereviser/pkg/somepkg"
)
-- want.go --
package testdata

import (
	"context"
	"database/sql"
	"fmt"

	"go.uber.org/fx"
	"golang.org/x/tools/go/packages"

	_ "github.com/lib/pq" // configure database/sql with postgres driver

	"github.com/zchee/goimports-rereviser/pkg/somepkg"
)
`,
			wantChange: true,
			wantErr:    false,
		},
		"success with multiple import statements": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

	import "sync" //test comment
	import "testing"

	// yolo
	import "fmt"


	// not sure why this is here but we shall find out soon enough
	import "io"
-- want.go --
package testdata

import (
	"fmt"
	"io"
	"sync" //test comment
	"testing"
)
`,
			wantChange: true,
			wantErr:    false,
		},
		"preserves cgo import": {
			projectName: testProjectName,
			filePath:    testCgoFilePath,
			archive: `
-- input.go --
package testdata

/*
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"fmt"
)
-- want.go --
package testdata

/*
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"fmt"
)
`,
			wantChange: false,
			wantErr:    false,
		},
		"preserves cgo import with single std deps": {
			projectName: testProjectName,
			filePath:    testCgoFilePath,
			archive: `
-- input.go --
package testdata

/*
#include <stdlib.h>
*/
import "C"

import "errors"
-- want.go --
package testdata

/*
#include <stdlib.h>
*/
import "C"

import "errors"
`,
			wantChange: false,
			wantErr:    false,
		},
		"preserves cgo import with single import": {
			projectName: testProjectName,
			filePath:    testCgoFilePath,
			archive: `
-- input.go --
package testdata

/*
#include <stdlib.h>
*/
import "C"
-- want.go --
package testdata

/*
#include <stdlib.h>
*/
import "C"
`,
			wantChange: false,
			wantErr:    false,
		},
		"preserves cgo import even when reordering": {
			projectName: testProjectName,
			filePath:    testCgoFilePath,
			archive: `
-- input.go --
package testdata

import (
	"fmt"
	"errors"
)

/*
#include <stdlib.h>
*/
import "C"

import "errors"
-- want.go --
package testdata

import (
	"errors"
	"fmt"
)

/*
#include <stdlib.h>
*/
import "C"
`,
			wantChange: true,
			wantErr:    false,
		},
		"try to read from stdin": {
			projectName: testProjectName,
			filePath:    StandardInput,
			archive: `
-- input.go --
`,
			wantChange: false,
			wantErr:    true,
		},
		"error with non-existent file": {
			projectName: testProjectName,
			filePath:    "./testdatax/does-not-exist.go",
			archive: `
-- input.go --
`,
			wantChange: false,
			wantErr:    true,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			runFixCase(t, tt.projectName, tt.filePath, tt.archive, tt.wantChange, tt.wantErr)
		})
	}
}

func TestSourceFile_Fix_WithImportsOrder(t *testing.T) {
	tests := map[string]struct {
		projectName  string
		filePath     string
		archive      string
		importsOrder string
		wantChange   bool
		wantErr      bool
	}{
		"success with default order": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
-- want.go --
package testdata

import (
	"bytes"
	"log"

	"golang.org/x/tools/go/packages"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)

// nolint:gomnd
`,
			importsOrder: "",
			wantChange:   true,
			wantErr:      false,
		},
		"success std,general,company,project": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
-- want.go --
package testdata

import (
	"bytes"
	"log"

	"golang.org/x/tools/go/packages"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)

// nolint:gomnd
`,
			importsOrder: "std,general,company,project",
			wantChange:   true,
			wantErr:      false,
		},
		"linkname blank import stays adjacent to supporting std imports": {
			projectName: "github.com/gaudiy/gaudiy-go-kit",
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"io"
	"os"
	"strings"
	_ "unsafe" // for go:linkname

	"github.com/kortschak/utter"
)
-- want.go --
package testdata

import (
	"io"
	"os"
	"strings"
	_ "unsafe" // for go:linkname

	"github.com/kortschak/utter"
)
`,
			importsOrder: "std,general,company,project",
			wantChange:   false,
			wantErr:      false,
		},
		"success project,company,general,std": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
-- want.go --
package testdata

import (
	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"golang.org/x/tools/go/packages"

	"bytes"
	"log"
)

// nolint:gomnd
`,
			importsOrder: "project,company,general,std",
			wantChange:   true,
			wantErr:      false,
		},
		"success project,company,general,std,blanked,dotted": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	. "io"

	"golang.org/x/tools/go/packages"

	_ "fmt"
)

// nolint:gomnd
-- want.go --
package testdata

import (
	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"golang.org/x/tools/go/packages"

	"bytes"
	"log"

	_ "fmt"

	. "io"
)

// nolint:gomnd
`,
			importsOrder: "project,company,general,std,blanked,dotted",
			wantChange:   true,
			wantErr:      false,
		},
		"linkname blank import stays in std group even when blanked group is configured": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	_ "unsafe" // for go:linkname

	"fmt"

	_ "embed"
)
-- want.go --
package testdata

import (
	"fmt"
	_ "unsafe" // for go:linkname

	_ "embed"
)
`,
			importsOrder: "std,general,company,project,blanked,dotted",
			wantChange:   true,
			wantErr:      false,
		},
		"alternative linkname blank import marker stays in std group": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	_ "unsafe" // depends on the runtime via a linkname'd function

	"fmt"

	_ "embed"
)
-- want.go --
package testdata

import (
	"fmt"
	_ "unsafe" // depends on the runtime via a linkname'd function

	_ "embed"
)
`,
			importsOrder: "std,general,company,project,blanked,dotted",
			wantChange:   true,
			wantErr:      false,
		},
		"blank import without linkname marker is routed to blanked group": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	_ "unsafe" // pulls in side effects

	"fmt"
)
-- want.go --
package testdata

import (
	"fmt"

	_ "unsafe" // pulls in side effects
)
`,
			importsOrder: "std,general,company,project,blanked,dotted",
			wantChange:   true,
			wantErr:      false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			order, err := StringToImportsOrders(tt.importsOrder)
			if err != nil {
				t.Fatalf("failed to parse imports order: %v", err)
			}
			runFixCase(t, tt.projectName, testFilePath, tt.archive, tt.wantChange, tt.wantErr, WithImportsOrder(order))
		})
	}
}

func TestSourceFile_Fix_WithRemoveUnusedImports(t *testing.T) {
	tests := map[string]struct {
		projectName string
		filePath    string
		archive     string
		wantChange  bool
		wantErr     bool
	}{
		"remove unused import": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"fmt" //fmt package
	"golang.org/x/tools/go/packages" //custom package
)

// nolint:gomnd
func main(){
  _ = fmt.Println("test")
}
-- want.go --
package testdata

import (
	"fmt" //fmt package
)

// nolint:gomnd
func main() {
	_ = fmt.Println("test")
}
`,
			wantChange: true,
			wantErr:    false,
		},
		"remove unused import with alias": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"fmt" //fmt package
	p "golang.org/x/tools/go/packages" //p package
)

// nolint:gomnd
func main(){
  _ = fmt.Println("test")
}
-- want.go --
package testdata

import (
	"fmt" //fmt package
)

// nolint:gomnd
func main() {
	_ = fmt.Println("test")
}
`,
			wantChange: true,
			wantErr:    false,
		},
		"use loaded import but not used": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"fmt" //fmt package
	_ "golang.org/x/tools/go/packages" //custom package
)

// nolint:gomnd
func main(){
  _ = fmt.Println("test")
}
-- want.go --
package testdata

import (
	"fmt" //fmt package

	_ "golang.org/x/tools/go/packages" //custom package
)

// nolint:gomnd
func main() {
	_ = fmt.Println("test")
}
`,
			wantChange: true,
			wantErr:    false,
		},
		"success with comments before imports": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
// Some comments are here
package testdata

// test
import (
	"fmt" //fmt package
	_ "golang.org/x/tools/go/packages" //custom package
)

// nolint:gomnd
func main(){
  _ = fmt.Println("test")
}
-- want.go --
// Some comments are here
package testdata

// test
import (
	"fmt" //fmt package

	_ "golang.org/x/tools/go/packages" //custom package
)

// nolint:gomnd
func main() {
	_ = fmt.Println("test")
}
`,
			wantChange: true,
			wantErr:    false,
		},
		"success without imports": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
// Some comments are here
package testdata

// OutputDir the output directory where the built version of Authelia is located.
var OutputDir = "dist"

// DockerImageName the official name of Authelia docker image.
var DockerImageName = "authelia/authelia"

// IntermediateDockerImageName local name of the docker image.
var IntermediateDockerImageName = "authelia:dist"

const masterTag = "master"
const stringFalse = "false"
const stringTrue = "true"
const suitePathPrefix = "PathPrefix"
const webDirectory = "web"
-- want.go --
// Some comments are here
package testdata

// OutputDir the output directory where the built version of Authelia is located.
var OutputDir = "dist"

// DockerImageName the official name of Authelia docker image.
var DockerImageName = "authelia/authelia"

// IntermediateDockerImageName local name of the docker image.
var IntermediateDockerImageName = "authelia:dist"

const masterTag = "master"
const stringFalse = "false"
const stringTrue = "true"
const suitePathPrefix = "PathPrefix"
const webDirectory = "web"
`,
			wantChange: false,
			wantErr:    false,
		},
		"cleanup empty import block": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
// Some comments are here
package testdata

import (
	"fmt"
)

// nolint:gomnd
func main(){
}
-- want.go --
// Some comments are here
package testdata

// nolint:gomnd
func main() {
}
`,
			wantChange: true,
			wantErr:    false,
		},
		"skip blanked and dotted import names": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
// Some comments are here
package testdata

import (
	_ "fmt"
	. "io"
)

// nolint:gomnd
func main() {
}
-- want.go --
// Some comments are here
package testdata

import (
	. "io"

	_ "fmt"
)

// nolint:gomnd
func main() {
}
`,
			wantChange: true,
			wantErr:    false,
		},
		"success with \"C\"": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata
/*
#cgo CFLAGS: -I
#cgo LDFLAGS: -L
#include <stdio.h>
#include <stdlib.h>
*/
import "C"
import(
	"fmt"
	"golang.org/x/tools/go/packages"
	"strconv"
)

func main(){
	_ = strconv.Itoa(1)
	fmt.Println(pg.In([]string{"test"}))
}
-- want.go --
package testdata

/*
#cgo CFLAGS: -I
#cgo LDFLAGS: -L
#include <stdio.h>
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"strconv"
)

func main() {
	_ = strconv.Itoa(1)
	fmt.Println(pg.In([]string{"test"}))
}
`,
			wantChange: true,
			wantErr:    false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			runFixCase(t, tt.projectName, testFilePath, tt.archive, tt.wantChange, tt.wantErr, WithRemovingUnusedImports)
		})
	}
}

func TestSourceFile_Fix_WithAliasForVersionSuffix(t *testing.T) {
	tests := map[string]struct {
		projectName string
		filePath    string
		archive     string
		wantChange  bool
		wantErr     bool
	}{
		"success with golang.org/x/tools/go/packages": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata
import(
	"fmt"
	"golang.org/x/tools/go/packages"
	"strconv"
)

func main(){
	_ = strconv.Itoa(1)
	fmt.Println(pg.In([]string{"test"}))
}
-- want.go --
package testdata

import (
	"fmt"
	"strconv"

	"golang.org/x/tools/go/packages"
)

func main() {
	_ = strconv.Itoa(1)
	fmt.Println(pg.In([]string{"test"}))
}
`,
			wantChange: true,
			wantErr:    false,
		},
		"success with \"C\"": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata
/*
#cgo CFLAGS: -I
#cgo LDFLAGS: -L
#include <stdio.h>
#include <stdlib.h>
*/
import "C"
import(
	"fmt"
	"golang.org/x/tools/go/packages"
	"strconv"
)

func main(){
	_ = strconv.Itoa(1)
	fmt.Println(pg.In([]string{"test"}))
}
-- want.go --
package testdata

/*
#cgo CFLAGS: -I
#cgo LDFLAGS: -L
#include <stdio.h>
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"strconv"

	"golang.org/x/tools/go/packages"
)

func main() {
	_ = strconv.Itoa(1)
	fmt.Println(pg.In([]string{"test"}))
}
`,
			wantChange: true,
			wantErr:    false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			runFixCase(t, tt.projectName, tt.filePath, tt.archive, tt.wantChange, tt.wantErr, WithUsingAliasForVersionSuffix)
		})
	}
}

func TestSourceFile_Fix_WithRemovingUnusedImportsAndAlias(t *testing.T) {
	tests := map[string]struct {
		projectName string
		filePath    string
		archive     string
		wantChange  bool
		wantErr     bool
	}{
		"removes unused import and sets alias": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `-- input.go --
package testdata
import(
	"fmt"
	"github.com/zchee/goimports-rereviser/v4/testdata/aliaspkg/v2"
	"strconv"
)

func main(){
	fmt.Println(aliaspkg.Value())
}
-- want.go --
package testdata

import (
	"fmt"

	aliaspkg "github.com/zchee/goimports-rereviser/v4/testdata/aliaspkg/v2"
)

func main() {
	fmt.Println(aliaspkg.Value())
}
`,
			wantChange: true,
			wantErr:    false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			runFixCase(
				t, tt.projectName, tt.filePath, tt.archive, tt.wantChange, tt.wantErr,
				WithRemovingUnusedImports,
				WithUsingAliasForVersionSuffix,
			)
		})
	}
}

func TestSourceFile_Fix_WithLocalPackagePrefixes(t *testing.T) {
	tests := map[string]struct {
		projectName      string
		filePath         string
		archive          string
		localPkgPrefixes string
		wantChange       bool
		wantErr          bool
	}{
		"group local packages by short prefix": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"fmt" //fmt package
	"golang.org/x/tools/go/packages" //custom package
	"github.com/zchee/goimports-rereviser/pkg"
	"goimports-rereviser/pkg"
)

/*
#include <stdlib.h>
*/
import "C"

// nolint:gomnd
func main(){
  _ = fmt.Println("test")
}
-- want.go --
package testdata

import (
	"fmt" //fmt package

	"golang.org/x/tools/go/packages" //custom package

	"goimports-rereviser/pkg"

	"github.com/zchee/goimports-rereviser/pkg"
)

/*
#include <stdlib.h>
*/
import "C"

// nolint:gomnd
func main() {
	_ = fmt.Println("test")
}
`,
			localPkgPrefixes: "goimports-rereviser",
			wantChange:       true,
			wantErr:          false,
		},
		"group local packages by full module prefix": {
			projectName: "goimports-rereviser",
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"fmt" // fmt package
	"golang.org/x/tools/go/packages" //custom package
	"github.com/zchee/goimports-rereviser/pkg"
	"goimports-rereviser/pkg"
)
// nolint:gomnd
func main(){
  _ = fmt.Println("test")
}
-- want.go --
package testdata

import (
	"fmt" // fmt package

	"golang.org/x/tools/go/packages" //custom package

	"github.com/zchee/goimports-rereviser/pkg"

	"goimports-rereviser/pkg"
)

// nolint:gomnd
func main() {
	_ = fmt.Println("test")
}
`,
			localPkgPrefixes: "github.com/zchee/goimports-rereviser",
			wantChange:       true,
			wantErr:          false,
		},
		"group local packages separately from project files": {
			projectName: "github.com/zchee/goimports-rereviser/code/thispkg",
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"fmt"
	"github.com/3rdparty/pkg"
	"github.com/zchee/goimports-rereviser/code/foopkg"
	"github.com/zchee/goimports-rereviser/code/otherpkg"
	"github.com/zchee/goimports-rereviser/code/thispkg/stuff"
	"github.com/zchee/goimports-rereviser/code/thispkg/morestuff"
)

// nolint:gomnd
func main(){
  _ = fmt.Println("test")
}
-- want.go --
package testdata

import (
	"fmt"

	"github.com/3rdparty/pkg"

	"github.com/zchee/goimports-rereviser/code/foopkg"
	"github.com/zchee/goimports-rereviser/code/otherpkg"

	"github.com/zchee/goimports-rereviser/code/thispkg/morestuff"
	"github.com/zchee/goimports-rereviser/code/thispkg/stuff"
)

// nolint:gomnd
func main() {
	_ = fmt.Println("test")
}
`,
			localPkgPrefixes: "github.com/zchee/goimports-rereviser/code",
			wantChange:       true,
			wantErr:          false,
		},
		"check without local packages": {
			projectName: "github.com/zchee/goimports-rereviser/code/thispkg",
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"fmt"
	"github.com/3rdparty/pkg"
	"github.com/zchee/goimports-rereviser/code/foopkg"
	"github.com/zchee/goimports-rereviser/code/otherpkg"
	"github.com/zchee/goimports-rereviser/code/thispkg/stuff"
	"github.com/zchee/goimports-rereviser/code/thispkg/morestuff"
)

// nolint:gomnd
func main(){
  _ = fmt.Println("test")
}
-- want.go --
package testdata

import (
	"fmt"

	"github.com/3rdparty/pkg"
	"github.com/zchee/goimports-rereviser/code/foopkg"
	"github.com/zchee/goimports-rereviser/code/otherpkg"

	"github.com/zchee/goimports-rereviser/code/thispkg/morestuff"
	"github.com/zchee/goimports-rereviser/code/thispkg/stuff"
)

// nolint:gomnd
func main() {
	_ = fmt.Println("test")
}
`,
			localPkgPrefixes: "",
			wantChange:       true,
			wantErr:          false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			runFixCase(t, tt.projectName, testFilePath, tt.archive, tt.wantChange, tt.wantErr, WithCompanyPackagePrefixes(tt.localPkgPrefixes))
		})
	}
}

func TestSourceFile_Fix_WithFormat(t *testing.T) {
	tests := map[string]struct {
		projectName string
		filePath    string
		archive     string
		wantChange  bool
		wantErr     bool
	}{
		"success": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata
type SomeStruct struct{}
type SomeStruct1 struct{}
// SomeStruct2 comments
type SomeStruct2 struct{}
func (s *SomeStruct2) test() {}
func test(){}
func test1(){}
-- want.go --
package testdata

type SomeStruct struct{}

type SomeStruct1 struct{}

// SomeStruct2 comments
type SomeStruct2 struct{}

func (s *SomeStruct2) test() {}

func test() {}

func test1() {}
`,
			wantChange: true,
			wantErr:    false,
		},
		"success with comments": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata
// test -  test comment
func test(){}
// test1 -  test comment
func test1(){}
-- want.go --
package testdata

// test -  test comment
func test() {}

// test1 -  test comment
func test1() {}
`,
			wantChange: true,
			wantErr:    false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			runFixCase(t, tt.projectName, testFilePath, tt.archive, tt.wantChange, tt.wantErr, WithCodeFormatting)
		})
	}
}

func TestSourceFile_Fix_WithSkipGeneratedFile(t *testing.T) {
	tests := map[string]struct {
		projectName string
		filePath    string
		archive     string
		wantChange  bool
		wantErr     bool
	}{
		"plain Code generated header is not a skip marker": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
// Code generated by some tool
package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
-- want.go --
// Code generated by some tool
package testdata

import (
	"bytes"
	"log"

	"golang.org/x/tools/go/packages"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)

// nolint:gomnd
`,
			wantChange: true,
			wantErr:    false,
		},
		"DO NOT EDIT in generated header skips": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
// Code generated by some tool, DO NOT EDIT.
package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
-- want.go --
// Code generated by some tool, DO NOT EDIT.
package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
`,
			wantChange: false,
			wantErr:    false,
		},
		"DO NOT EDIT without comma in generated header skips": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
// Code generated by some tool DO NOT EDIT.
package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
-- want.go --
// Code generated by some tool DO NOT EDIT.
package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
`,
			wantChange: false,
			wantErr:    false,
		},
		"DO NOT EDIT after preceding copyright block skips": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
/*
Other comment or copyright
*/

// Code generated by some tool DO NOT EDIT.

package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
-- want.go --
/*
Other comment or copyright
*/

// Code generated by some tool DO NOT EDIT.

package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
`,
			wantChange: false,
			wantErr:    false,
		},
		"DO NOT EDIT after package keyword is not a skip marker": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
/*
Other comment or copyright
*/

package testdata

// Code generated by some tool DO NOT EDIT.

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
-- want.go --
/*
Other comment or copyright
*/

package testdata

// Code generated by some tool DO NOT EDIT.

import (
	"bytes"
	"log"

	"golang.org/x/tools/go/packages"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)

// nolint:gomnd
`,
			wantChange: true,
			wantErr:    false,
		},
		"DO NOT EDIT after package without copyright is not a skip marker": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

// Code generated by some tool DO NOT EDIT.

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// nolint:gomnd
-- want.go --
package testdata

// Code generated by some tool DO NOT EDIT.

import (
	"bytes"
	"log"

	"golang.org/x/tools/go/packages"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)

// nolint:gomnd
`,
			wantChange: true,
			wantErr:    false,
		},
		"DO NOT EDIT in trailing comment is not a skip marker": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// Code generated by some tool DO NOT EDIT.
// nolint:gomnd
-- want.go --
package testdata

import (
	"bytes"
	"log"

	"golang.org/x/tools/go/packages"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)

// Code generated by some tool DO NOT EDIT.
// nolint:gomnd
`,
			wantChange: true,
			wantErr:    false,
		},
		"DO NOT EDIT in trailing comment block is not a skip marker": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)

// Inner comment

// Code generated by some tool DO NOT EDIT.

// nolint:gomnd
-- want.go --
package testdata

import (
	"bytes"
	"log"

	"golang.org/x/tools/go/packages"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)

// Inner comment

// Code generated by some tool DO NOT EDIT.

// nolint:gomnd
`,
			wantChange: true,
			wantErr:    false,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			runFixCase(t, tt.projectName, testFilePath, tt.archive, tt.wantChange, tt.wantErr, WithSkipGeneratedFile)
		})
	}
}

func TestSourceFile_Fix_WithSeparatedNamedImports(t *testing.T) {
	tests := map[string]struct {
		projectName string
		filePath    string
		archive     string
		wantChange  bool
		wantErr     bool
	}{
		"simple": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata
import (
	"fmt"
	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
	"bytes"
	"golang.org/x/tools/go/packages"
)
-- want.go --
package testdata

import (
	"bytes"
	"fmt"

	"golang.org/x/tools/go/packages"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)
`,
			wantChange: true,
			wantErr:    false,
		},
		"named": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata
import (
	"fmt"
	second "github.com/zchee/goimports-rereviser/testdata/secondpkg"
	by "bytes"
	js "encoding/json"
	"golang.org/x/tools/go/packages"
	er "golang.org/x/errors"
	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)
-- want.go --
package testdata

import (
	"fmt"

	by "bytes"
	js "encoding/json"

	"golang.org/x/tools/go/packages"

	er "golang.org/x/errors"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	second "github.com/zchee/goimports-rereviser/testdata/secondpkg"
)
`,
			wantChange: true,
			wantErr:    false,
		},
		"named with comments": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata
import (
	"fmt" //fmt package
	second "github.com/zchee/goimports-rereviser/testdata/secondpkg" //secondpkg package
	by "bytes"
	js "encoding/json"
	"golang.org/x/tools/go/packages" //packages package
	er "golang.org/x/errors"
	"github.com/zchee/goimports-rereviser/testdata/innderpkg"
)
-- want.go --
package testdata

import (
	"fmt" //fmt package

	by "bytes"
	js "encoding/json"

	"golang.org/x/tools/go/packages" //packages package

	er "golang.org/x/errors"

	"github.com/zchee/goimports-rereviser/testdata/innderpkg"

	second "github.com/zchee/goimports-rereviser/testdata/secondpkg" //secondpkg package
)
`,
			wantChange: true,
			wantErr:    false,
		},
		"linkname blank import is not treated as named import": {
			projectName: testProjectName,
			filePath:    testFilePath,
			archive: `
-- input.go --
package testdata

import (
	_ "unsafe" // for go:linkname
	"fmt"
	js "encoding/json"
)
-- want.go --
package testdata

import (
	"fmt"
	_ "unsafe" // for go:linkname

	js "encoding/json"
)
`,
			wantChange: true,
			wantErr:    false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			filePath := filepath.Join(t.TempDir(), "example.go")
			runFixCase(t, tt.projectName, filePath, tt.archive, tt.wantChange, tt.wantErr, WithSeparatedNamedImports)
		})
	}
}
