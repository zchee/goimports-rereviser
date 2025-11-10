package reviser

import (
	"github.com/zchee/goimports-rereviser/v4/pkg/astutil"
	"github.com/zchee/goimports-rereviser/v4/pkg/module"
)

// clearTestCaches clears all caches used by the reviser package.
// This should be called between tests to prevent cache pollution.
func clearTestCaches() {
	astutil.ClearPackageDepsCache()
	module.ClearModuleNameCache()
}
