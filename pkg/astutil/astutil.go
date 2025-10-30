package astutil

import (
	"errors"
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/packages"
)

const (
	buildTagPrefix           = "//go:build"
	deprecatedBuildTagPrefix = "//+build"
)

// PackageImports is map of imports with their package names
type PackageImports map[string]string

// UsesImport is for analyze if the import dependency is in use
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

	ast.Inspect(
		f,
		func(node ast.Node) bool {
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
		},
	)

	return used
}

// LoadPackageDependencies will return all package's imports with it names:
//
//	key - package(ex.: github/pkg/errors), value - name(ex.: errors)
func LoadPackageDependencies(dir, buildTag string) (PackageImports, error) {
	cfg := &packages.Config{
		Dir:   dir,
		Tests: true,
		Mode:  packages.NeedName | packages.NeedImports,
	}

	if buildTag != "" {
		cfg.BuildFlags = []string{fmt.Sprintf(`-tags=%s`, buildTag)}
	}

	pkgs, err := packages.Load(cfg)
	if err != nil {
		return PackageImports{}, err
	}

	if packages.PrintErrors(pkgs) > 0 {
		return PackageImports{}, errors.New("package has an errors")
	}

	result := PackageImports{}

	for _, pkg := range pkgs {
		for imprt, pkg := range pkg.Imports {
			result[imprt] = pkg.Name
		}
	}

	return result, nil
}

// ParseBuildTag parse `//+build ...` or `//go:build ` on a first line of *ast.File
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

type visitFn func(node ast.Node)

func (f visitFn) Visit(node ast.Node) ast.Visitor {
	f(node)
	return f
}
