package engine

import (
	"strings"

	"github.com/zchee/goimports-rereviser/v4/pkg/std"
)

type importBucket int

const (
	importBucketGeneral importBucket = iota
	importBucketStd
	importBucketCompany
	importBucketProject
	importBucketBlanked
	importBucketDotted
)

type classifiedImport struct {
	bucket importBucket
	named  bool
}

func classifyImport(
	projectName string,
	localPkgPrefixes []string,
	importsOrders ImportsOrders,
	separateNamed bool,
	imprt string,
	meta *commentsMetadata,
) classifiedImport {
	isLinknameBlank := isLinknameBlankImport(imprt, meta)
	if importsOrders.hasBlankedImportOrder() && strings.HasPrefix(imprt, "_") && !isLinknameBlank {
		return classifiedImport{bucket: importBucketBlanked}
	}

	if importsOrders.hasDottedImportOrder() && strings.HasPrefix(imprt, ".") {
		return classifiedImport{bucket: importBucketDotted}
	}

	pkgWithoutAlias := skipPackageAlias(imprt)
	isNamed := separateNamed && !isLinknameBlank && strings.Contains(imprt, " ")

	if _, ok := std.StdPackages[pkgWithoutAlias]; ok {
		return classifiedImport{bucket: importBucketStd, named: isNamed}
	}

	for _, localPackagePrefix := range localPkgPrefixes {
		if strings.HasPrefix(pkgWithoutAlias, localPackagePrefix) && pkgWithoutAlias != projectName && !strings.HasPrefix(pkgWithoutAlias, projectName+"/") {
			return classifiedImport{bucket: importBucketCompany, named: isNamed}
		}
	}

	if pkgWithoutAlias == projectName || strings.HasPrefix(pkgWithoutAlias, projectName+"/") {
		return classifiedImport{bucket: importBucketProject, named: isNamed}
	}

	return classifiedImport{bucket: importBucketGeneral, named: isNamed}
}
