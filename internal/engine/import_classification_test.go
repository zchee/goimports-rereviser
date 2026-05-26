package engine

import (
	"go/ast"
	"testing"
)

func TestClassifyImport(t *testing.T) {
	tests := []struct {
		name          string
		projectName   string
		localPrefixes []string
		importsOrders ImportsOrders
		separateNamed bool
		importPath    string
		meta          *commentsMetadata
		wantBucket    importBucket
		wantNamed     bool
	}{
		{
			name:          "blank import goes to blanked bucket when enabled",
			importsOrders: ImportsOrders{StdImportsOrder, GeneralImportsOrder, CompanyImportsOrder, ProjectImportsOrder, BlankedImportsOrder},
			importPath:    `_ "fmt"`,
			wantBucket:    importBucketBlanked,
		},
		{
			name:          "linkname blank import stays in standard bucket",
			importsOrders: ImportsOrders{StdImportsOrder, GeneralImportsOrder, CompanyImportsOrder, ProjectImportsOrder, BlankedImportsOrder},
			importPath:    `_ "unsafe"`,
			meta: &commentsMetadata{
				Comment: &ast.CommentGroup{List: []*ast.Comment{{Text: "// for go:linkname"}}},
			},
			wantBucket: importBucketStd,
		},
		{
			name:          "stdlib named import is tracked separately",
			importsOrders: ImportsOrders{StdImportsOrder, GeneralImportsOrder, CompanyImportsOrder, ProjectImportsOrder},
			separateNamed: true,
			importPath:    `fmt "fmt"`,
			wantBucket:    importBucketStd,
			wantNamed:     true,
		},
		{
			name:          "company import is grouped by local prefix",
			projectName:   "github.com/acme/project",
			localPrefixes: []string{"github.com/acme/"},
			importsOrders: ImportsOrders{StdImportsOrder, GeneralImportsOrder, CompanyImportsOrder, ProjectImportsOrder},
			importPath:    `github.com/acme/lib/pkg`,
			wantBucket:    importBucketCompany,
		},
		{
			name:          "project import wins over local prefix",
			projectName:   "github.com/acme/project",
			localPrefixes: []string{"github.com/acme/"},
			importsOrders: ImportsOrders{StdImportsOrder, GeneralImportsOrder, CompanyImportsOrder, ProjectImportsOrder},
			importPath:    `github.com/acme/project/internal/pkg`,
			wantBucket:    importBucketProject,
		},
		{
			name:          "general import is fallback bucket",
			projectName:   "github.com/acme/project",
			importsOrders: ImportsOrders{StdImportsOrder, GeneralImportsOrder, CompanyImportsOrder, ProjectImportsOrder},
			importPath:    `github.com/other/pkg`,
			wantBucket:    importBucketGeneral,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyImport(tt.projectName, tt.localPrefixes, tt.importsOrders, tt.separateNamed, tt.importPath, tt.meta)
			if got.bucket != tt.wantBucket {
				t.Fatalf("bucket = %v, want %v", got.bucket, tt.wantBucket)
			}
			if got.named != tt.wantNamed {
				t.Fatalf("named = %v, want %v", got.named, tt.wantNamed)
			}
		})
	}
}
