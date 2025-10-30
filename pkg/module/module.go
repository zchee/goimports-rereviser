package module

import (
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/mod/modfile"
)

const goModFilename = "go.mod"

// moduleNameCacheEntry represents a cached result for module Name lookups
type moduleNameCacheEntry struct {
	name string
	err  error
}

// moduleNameCache provides thread-safe caching for module Name lookups
var moduleNameCache sync.Map // map[string]moduleNameCacheEntry

// ClearModuleNameCache clears the module name cache.
// This is primarily for testing to prevent cache pollution between tests.
func ClearModuleNameCache() {
	moduleNameCache = sync.Map{}
}

// Name reads module value from ./go.mod
// Results are cached for performance. Multiple calls with the same root path
// will return cached results. Only successful results are cached; errors are
// not cached to avoid persisting transient failures.
func Name(goModRootPath string) (string, error) {
	// Try to load from cache
	if cached, ok := moduleNameCache.Load(goModRootPath); ok {
		entry := cached.(moduleNameCacheEntry)
		return entry.name, entry.err
	}

	// Not in cache, load
	name, err := nameUncached(goModRootPath)

	// Only cache successful results, not errors
	if err == nil {
		entry := moduleNameCacheEntry{name: name, err: nil}
		moduleNameCache.Store(goModRootPath, entry)
	}

	return name, err
}

// nameUncached is the actual implementation without caching
func nameUncached(goModRootPath string) (string, error) {
	goModFile := filepath.Join(goModRootPath, goModFilename)

	data, err := os.ReadFile(goModFile)
	if err != nil {
		return "", err
	}

	f, err := modfile.Parse(goModFile, data, nil)
	if err != nil {
		return "", err
	}

	if f.Module != nil {
		return f.Module.Mod.Path, nil
	}

	return "", &UndefinedModuleError{}
}

// GoModRootPath in case of any directory or file of the project will return root dir of the project where go.mod file
// is exist
func GoModRootPath(path string) (string, error) {
	if path == "" {
		return "", &PathIsNotSetError{}
	}

	path = filepath.Clean(path)

	for {
		if fi, err := os.Stat(filepath.Join(path, goModFilename)); err == nil && !fi.IsDir() {
			return path, nil
		}

		d := filepath.Dir(path)
		if d == path {
			break
		}

		path = d
	}

	return "", nil
}

func DetermineProjectName(projectName, filePath string) (string, error) {
	if projectName == "" {
		projectRootPath, err := GoModRootPath(filePath)
		if err != nil {
			return "", err
		}

		moduleName, err := Name(projectRootPath)
		if err != nil {
			return "", err
		}

		return moduleName, nil
	}

	return projectName, nil
}
