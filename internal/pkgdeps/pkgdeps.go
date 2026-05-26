package pkgdeps

import (
	"errors"
	"fmt"
	"sync"

	"golang.org/x/tools/go/packages"
)

type cacheEntry struct {
	imports PackageImports
	err     error
}

type cacheKey struct {
	dir      string
	buildTag string
}

var cache sync.Map // map[cacheKey]cacheEntry

var calls sync.Map // map[cacheKey]*call

type call struct {
	ready   chan struct{}
	imports PackageImports
	err     error
}

var loadFunc = loadUncached

// PackageImports is map of imports with their package names.
type PackageImports map[string]string

func ClearCache() {
	cache = sync.Map{}
	calls = sync.Map{}
}

func Load(dir, buildTag string) (PackageImports, error) {
	key := cacheKey{dir: dir, buildTag: buildTag}

	if cached, ok := cache.Load(key); ok {
		entry := cached.(cacheEntry)
		return entry.imports, entry.err
	}

	callIface, loaded := calls.LoadOrStore(key, &call{ready: make(chan struct{})})
	currentCall := callIface.(*call)
	if !loaded {
		imports, err := loadFunc(dir, buildTag)
		if err == nil {
			cache.Store(key, cacheEntry{imports: imports})
		}
		currentCall.imports = imports
		currentCall.err = err
		close(currentCall.ready)
		if err != nil {
			calls.Delete(key)
		}
	} else {
		<-currentCall.ready
	}

	if currentCall.err != nil {
		return PackageImports{}, currentCall.err
	}
	return currentCall.imports, nil
}

func loadUncached(dir, buildTag string) (PackageImports, error) {
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
