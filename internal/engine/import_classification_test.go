package engine

import "testing"

func TestClassifyImport(t *testing.T) {
	tests := []struct {
		name          string
		projectName   string
		localPrefixes []string
		importsOrders ImportsOrders
		separateNamed bool
		importPath    string
		wantBucket    importBucket
		wantNamed     bool
	}{
		{
			name:          "blank standard import ignores blanked bucket when enabled",
			importsOrders: ImportsOrders{StdImportsOrder, GeneralImportsOrder, CompanyImportsOrder, ProjectImportsOrder, BlankedImportsOrder},
			importPath:    `_ "embed"`,
			wantBucket:    importBucketStd,
		},
		{
			name:          "blank standard import is not treated as named",
			importsOrders: ImportsOrders{StdImportsOrder, GeneralImportsOrder, CompanyImportsOrder, ProjectImportsOrder, BlankedImportsOrder},
			separateNamed: true,
			importPath:    `_ "embed"`,
			wantBucket:    importBucketStd,
		},
		{
			name:          "blank general import ignores blanked bucket when enabled",
			projectName:   "github.com/acme/project",
			importsOrders: ImportsOrders{StdImportsOrder, GeneralImportsOrder, CompanyImportsOrder, ProjectImportsOrder, BlankedImportsOrder},
			importPath:    `_ "github.com/other/pkg"`,
			wantBucket:    importBucketGeneral,
		},
		{
			name:          "blank company import ignores blanked bucket when enabled",
			projectName:   "github.com/acme/project",
			localPrefixes: []string{"github.com/acme/"},
			importsOrders: ImportsOrders{StdImportsOrder, GeneralImportsOrder, CompanyImportsOrder, ProjectImportsOrder, BlankedImportsOrder},
			importPath:    `_ "github.com/acme/lib/pkg"`,
			wantBucket:    importBucketCompany,
		},
		{
			name:          "blank project import ignores blanked bucket when enabled",
			projectName:   "github.com/acme/project",
			localPrefixes: []string{"github.com/acme/"},
			importsOrders: ImportsOrders{StdImportsOrder, GeneralImportsOrder, CompanyImportsOrder, ProjectImportsOrder, BlankedImportsOrder},
			importPath:    `_ "github.com/acme/project/internal/pkg"`,
			wantBucket:    importBucketProject,
		},
		{
			name:          "linkname blank import is path classified in standard bucket",
			importsOrders: ImportsOrders{StdImportsOrder, GeneralImportsOrder, CompanyImportsOrder, ProjectImportsOrder, BlankedImportsOrder},
			importPath:    `_ "unsafe"`,
			wantBucket:    importBucketStd,
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
			got := classifyImport(tt.projectName, tt.localPrefixes, tt.importsOrders, tt.separateNamed, tt.importPath)
			if got.bucket != tt.wantBucket {
				t.Fatalf("bucket = %v, want %v", got.bucket, tt.wantBucket)
			}
			if got.named != tt.wantNamed {
				t.Fatalf("named = %v, want %v", got.named, tt.wantNamed)
			}
		})
	}
}

func TestClassifyImportNamedDetectionDoesNotAllocate(t *testing.T) {
	importsOrders := ImportsOrders{StdImportsOrder, GeneralImportsOrder, CompanyImportsOrder, ProjectImportsOrder}

	var got classifiedImport
	allocs := testing.AllocsPerRun(1000, func() {
		got = classifyImport("", nil, importsOrders, true, `fmt "fmt"`)
	})

	if got.bucket != importBucketStd || !got.named {
		t.Fatalf("classifyImport result = %+v, want named std import", got)
	}
	if allocs != 0 {
		t.Fatalf("classifyImport named detection allocated %.1f times per run", allocs)
	}
}

func TestSkipPackageAlias(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"unnamed import": {
			input: `"fmt"`,
			want:  "fmt",
		},
		"named import": {
			input: `jsoniter "github.com/json-iterator/go"`,
			want:  "github.com/json-iterator/go",
		},
		"blank import": {
			input: `_ "embed"`,
			want:  "embed",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := skipPackageAlias(tt.input); got != tt.want {
				t.Fatalf("skipPackageAlias(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
