package modulepath

import (
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/mod/modfile"
)

const goModFilename = "go.mod"

type nameCacheEntry struct {
	name string
	err  error
}

var nameCache sync.Map // map[string]nameCacheEntry

func ClearNameCache() {
	nameCache = sync.Map{}
}

func Name(goModRootPath string) (string, error) {
	if cached, ok := nameCache.Load(goModRootPath); ok {
		entry := cached.(nameCacheEntry)
		return entry.name, entry.err
	}

	name, err := nameUncached(goModRootPath)
	if err == nil {
		nameCache.Store(goModRootPath, nameCacheEntry{name: name})
	}
	return name, err
}

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
	if projectName != "" {
		return projectName, nil
	}

	projectRootPath, err := GoModRootPath(filePath)
	if err != nil {
		return "", err
	}
	if projectRootPath == "" {
		return "", &UndefinedModuleError{}
	}

	moduleName, err := Name(projectRootPath)
	if err != nil {
		return "", err
	}

	return moduleName, nil
}
