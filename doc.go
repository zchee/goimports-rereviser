// Tool for Golang to sort goimports by 3-4 groups: std, general, local(which is optional) and project dependencies.
// It will help you to keep your code cleaner.
//
// Example:
//	goimports-rereviser -project-name github.com/zchee/goimports-rereviser -file-path ./reviser/reviser.go -rm-unused
//
// Input:
// 	import (
//		"log"
//
//		"github.com/zchee/goimports-rereviser/testdata/innderpkg"
//
//		"bytes"
//
//		"golang.org/x/exp/slices"
// 	)
//
// Output:
//
//	 import (
//		"bytes"
//		"log"
//
//		"golang.org/x/exp/slices"
//
//		"github.com/zchee/goimports-rereviser/testdata/innderpkg"
//	 )
//
// If you need to set package names explicitly(in import declaration), you can use additional option `-set-alias`.
//
// More:
//
// 	goimports-rereviser -h
//
package main
