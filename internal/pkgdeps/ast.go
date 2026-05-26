package pkgdeps

import (
	"go/ast"
	"strings"
)

const (
	buildTagPrefix           = "//go:build"
	deprecatedBuildTagPrefix = "//+build"
)

// UsesImport is for analyze if the import dependency is in use.
func UsesImport(f *ast.File, packageImports PackageImports, importPath string) bool {
	importPath = strings.Trim(importPath, `"`)
	return UsedImports(f, packageImports)[importPath]
}

// UsedImports walks the AST once and reports which imports are referenced.
func UsedImports(f *ast.File, packageImports PackageImports) map[string]bool {
	used := make(map[string]bool, len(f.Imports))
	aliasToPath := make(map[string]string, len(f.Imports))

	for _, spec := range f.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		if spec.Name != nil {
			name := spec.Name.Name
			switch name {
			case "_", ".":
				used[path] = true
				continue
			default:
				aliasToPath[name] = path
				continue
			}
		}

		pkgName := packageImports[path]
		if pkgName == "" {
			if idx := strings.LastIndex(path, "/"); idx >= 0 {
				pkgName = path[idx+1:]
			} else {
				pkgName = path
			}
		}
		aliasToPath[pkgName] = path
	}

	ast.Inspect(f, func(node ast.Node) bool {
		sel, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Obj != nil {
			return true
		}

		if path, ok := aliasToPath[ident.Name]; ok {
			used[path] = true
		}

		return true
	})

	return used
}

// ParseBuildTag parse `//+build ...` or `//go:build ` on a first line of *ast.File.
func ParseBuildTag(f *ast.File) string {
	for _, g := range f.Comments {
		for _, c := range g.List {
			if !strings.HasPrefix(c.Text, buildTagPrefix) && !strings.HasPrefix(c.Text, deprecatedBuildTagPrefix) {
				continue
			}
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(c.Text, buildTagPrefix), deprecatedBuildTagPrefix))
		}
	}

	return ""
}
