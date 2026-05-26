package engine

import (
	"github.com/zchee/goimports-rereviser/v4/internal/modulepath"
	"github.com/zchee/goimports-rereviser/v4/internal/pkgdeps"
)

// clearTestCaches clears all caches used by the reviser package.
// This should be called between tests to prevent cache pollution.
func clearTestCaches() {
	pkgdeps.ClearCache()
	modulepath.ClearNameCache()
}
