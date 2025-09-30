# goimports-rereviser

[![codecov](https://codecov.io/gh/zchee/goimports-rereviser/branch/main/graph/badge.svg)](https://codecov.io/gh/incu6us/goimports-rereviser)
![GitHub All Releases](https://img.shields.io/github/downloads/zchee/goimports-rereviser/total?color=green)
![GitHub release (latest by date)](https://img.shields.io/github/v/release/incu6us/goimports-rereviser?color=green)
![license](https://img.shields.io/github/license/zchee/goimports-rereviser)

Tool goimports-rereviser for Golang to sort goimports by 3-4 groups(with own [linter](linter/README.md)): std, general, company(which is optional) and project dependencies.

Also, formatting for your code will be prepared(so, you don't need to use `gofmt` or `goimports` separately).

Use additional options `-rm-unused` to remove unused imports and `-set-alias` to rewrite import aliases for versioned packages or for packages with additional prefix/suffix(example: `opentracing "github.com/opentracing/opentracing-go"`).

`-company-prefixes` - will create group for company imports(libs inside your organization). Values should be comma-separated.


## Configuration:

### Cmd
```bash
goimports-rereviser -rm-unused -set-alias -format ./reviser/reviser.go
```

You can also apply rules to a dir or recursively apply using ./... as a target:
```bash
goimports-rereviser -rm-unused -set-alias -format -recursive reviser
```

```bash
goimports-rereviser -rm-unused -set-alias -format ./...
```

You can also apply rules to multiple targets:
```bash
goimports-rereviser -rm-unused -set-alias -format ./reviser/reviser.go ./pkg/...
```

### Options:

```text
Usage of goimports-rereviser:
  -apply-to-generated-files
    	Apply imports sorting and formatting(if the option is set) to generated files. Generated file is a file with first comment which starts with comment '// Code generated'. Optional parameter.
  -company-prefixes string
    	Company package prefixes which will be placed after 3rd-party group by default(if defined). Values should be comma-separated. Optional parameters.
  -excludes string
    	Exclude files or dirs, example: '.git/,proto/*.go'.
  -file-path string
    	Deprecated. Put file name as an argument(last item) of command line.
  -format
    	Option will perform additional formatting. Optional parameter.
  -imports-order string
    	Your imports groups can be sorted in your way.
    	std - std import group;
    	general - libs for general purpose;
    	company - inter-org or your company libs(if you set '-company-prefixes'-option, then 4th group will be split separately. In other case, it will be the part of general purpose libs);
    	project - your local project dependencies;
    	blanked - imports with "_" alias;
    	dotted - imports with "." alias.
    	Optional parameter. (default "std,general,company,project")
  -list-diff
    	Option will list files whose formatting differs from goimports-rereviser. Optional parameter.
  -local string
    	Deprecated
  -output string
    	Can be "file", "write" or "stdout". Whether to write the formatted content back to the file or to stdout. When "write" together with "-list-diff" will list the file name and write back to the file. Optional parameter. (default "file")
  -project-name string
    	Your project name(ex.: github.com/zchee/goimports-rereviser). Optional parameter.
  -recursive
    	Apply rules recursively if target is a directory. In case of ./... execution will be recursively applied by default. Optional parameter.
  -rm-unused
    	Remove unused imports. Optional parameter.
  -separate-named
        Separate named imports from their group with a new line. Optional parameter.
  -set-alias
    	Set alias for versioned package names, like 'github.com/go-pg/pg/v9'. In this case import will be set as 'pg "github.com/go-pg/pg/v9"'. Optional parameter.
  -set-exit-status
    	set the exit status to 1 if a change is needed/made. Optional parameter.
  -use-cache
    	Use cache to improve performance. Optional parameter.
  -version
    	Show version.
```

## Install

### With Go

```bash
go install -v github.com/zchee/goimports-rereviser/v4@latest
```

## Examples

Before usage:
```go
package testdata

import (
	"log"

	"github.com/zchee/goimports-rereviser/v4/testdata/innderpkg"

	"bytes"

	"golang.org/x/tools/go/packages"
)
``` 

After usage:
```go
package testdata

import (
	"bytes"
	"log"

	"golang.org/x/tools/go/packages"

	"github.com/zchee/goimports-rereviser/v4/testdata/innderpkg"
)
```

Comments(not Docs) for imports is acceptable. Example:
```go
package testdata

import (
    "fmt" // comments to the package here
)
```  

### Example with `-company-prefixes`-option

Before usage:

```go
package testdata // goimports-rereviser/testdata

import (
	"fmt" //fmt package
	"golang.org/x/tools/go/packages" //custom package
	"github.com/zchee/goimports-rereviser/v4/pkg" // this is a company package which is not a part of the project, but is a part of your organization
	"goimports-rereviser/pkg"
)
```

After usage:
```go
package testdata // goimports-rereviser/testdata

import (
	"fmt" // fmt package

	"golang.org/x/tools/go/packages" // custom package

	"github.com/zchee/goimports-rereviser/v4/pkg" // this is a company package which is not a part of the project, but is a part of your organization

	"goimports-rereviser/pkg"
)
```

### Example with `-imports-order std,general,company,project,blanked,dotted`-option

Before usage:

```go
package testdata // goimports-rereviser/testdata

import (
	_ "github.com/pkg1"
	. "github.com/pkg2"
	"fmt" //fmt package
	"golang.org/x/tools/go/packages" //custom package
	"github.com/zchee/goimports-rereviser/v4/pkg" // this is a company package which is not a part of the project, but is a part of your organization
	"goimports-rereviser/pkg"
)
```

After usage:
```go
package testdata // goimports-rereviser/testdata

import (
	"fmt" // fmt package

	"golang.org/x/tools/go/packages" // custom package

	"github.com/zchee/goimports-rereviser/v4/pkg" // this is a company package which is not a part of the project, but is a part of your organization

	"goimports-rereviser/pkg"

	_ "github.com/pkg1"

	. "github.com/pkg2"
)
```

### Example with `-format`-option

Before usage:
```go
package main
func test(){
}
func additionalTest(){
}
```

After usage:
```go
package main

func test(){
}

func additionalTest(){
}
```

### Example with `-separate-named`-option

Before usage:

```go
package testdata // goimports-rereviser/testdata

import (
	"fmt"
	"github.com/zchee/goimports-rereviser/v4/pkg"
	extpkg "google.com/golang/pkg"
	"golang.org/x/tools/go/packages"
	extslice "github.com/PeterRK/slices"
)
```

After usage:
```go
package testdata // goimports-rereviser/testdata

import (
	"fmt"

	"github.com/zchee/goimports-rereviser/v4/pkg"
	"golang.org/x/tools/go/packages"

	extpkg "google.com/golang/pkg"
	extslice "github.com/PeterRK/slices"
)
```

---
## Contributors

A big thank you to all the amazing people who contributed!

<a href="https://github.com/zchee/goimports-rereviser/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=incu6us/goimports-rereviser" />
</a>

## Give a Star! ⭐

If you like or are using this project, please give it a **star**.

### Stargazers

[![Stargazers over time](https://starchart.cc/zchee/goimports-rereviser.svg)](https://starchart.cc/zchee/goimports-rereviser)
