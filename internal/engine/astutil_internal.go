package engine

import (
	"go/ast"
	"strings"
)

const (
	buildTagPrefix           = "//go:build"
	deprecatedBuildTagPrefix = "//+build"
)

func computeUsedImports(file *ast.File, packageImports map[string]string) map[string]bool {
	used := make(map[string]bool, len(file.Imports))
	aliasToPath := make(map[string]string, len(file.Imports))

	for _, spec := range file.Imports {
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

	ast.Inspect(file, func(node ast.Node) bool {
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

func parseBuildTag(file *ast.File) string {
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if !strings.HasPrefix(comment.Text, buildTagPrefix) && !strings.HasPrefix(comment.Text, deprecatedBuildTagPrefix) {
				continue
			}
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(comment.Text, buildTagPrefix), deprecatedBuildTagPrefix))
		}
	}

	return ""
}
