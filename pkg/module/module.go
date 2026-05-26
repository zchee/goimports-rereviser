package module

import (
	"errors"

	"github.com/zchee/goimports-rereviser/v4/internal/modulepath"
)

func translateError(err error) error {
	if err == nil {
		return nil
	}
	var pathErr *modulepath.PathIsNotSetError
	if errors.As(err, &pathErr) {
		return &PathIsNotSetError{}
	}
	var undefinedErr *modulepath.UndefinedModuleError
	if errors.As(err, &undefinedErr) {
		return &UndefinedModuleError{}
	}
	return err
}

// ClearModuleNameCache clears the module name cache.
// This is primarily for testing to prevent cache pollution between tests.
func ClearModuleNameCache() {
	modulepath.ClearNameCache()
}

// Name reads module value from ./go.mod
// Results are cached for performance. Multiple calls with the same root path
// will return cached results. Only successful results are cached; errors are
// not cached to avoid persisting transient failures.
func Name(goModRootPath string) (string, error) {
	name, err := modulepath.Name(goModRootPath)
	return name, translateError(err)
}

// GoModRootPath in case of any directory or file of the project will return root dir of the project where go.mod file
// is exist
func GoModRootPath(path string) (string, error) {
	root, err := modulepath.GoModRootPath(path)
	return root, translateError(err)
}

func DetermineProjectName(projectName, filePath string) (string, error) {
	name, err := modulepath.DetermineProjectName(projectName, filePath)
	return name, translateError(err)
}
