package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
)

func printUsage() exitCode {
	if _, err := fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0]); err != nil {
		log.Fatalf("failed to print usage: %s", err)
	}

	flag.PrintDefaults()

	return exitUsage
}

// printUsageAndExit prints usage and exits with status 0
// if err is nil, otherwise it prints the error and exits with status 1
func printUsageAndExit(err error) exitCode {
	printUsage()
	if err != nil {
		log.Printf("%s", err)
		return exitError
	}

	return exitUsage
}

func getBuildInfo() *debug.BuildInfo {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	return bi
}

var modulePathMatcher = regexp.MustCompile(`^github.com/[\w-]+/goimports-rereviser(/v\d+)?@?`) // using a regex here so that this will work with forked repos (at least on github.com)

func getMyModuleInfo(bi *debug.BuildInfo) (*debug.Module, error) {
	if bi == nil {
		return nil, errors.New("no build info available")
	}
	// depending on the context in which we are called, the main module may not be set
	if bi.Main.Path != "" {
		return &bi.Main, nil
	}
	// if the main module is not set, we need to find the dep that contains our module
	for _, m := range bi.Deps {
		if modulePathMatcher.MatchString(m.Path) {
			return m, nil
		}
	}
	return nil, errors.New("no matching module found in build info")
}

func printVersion() exitCode {
	if Tag != "" {
		fmt.Printf(
			"version: %s\nbuilt with: %s\ntag: %s\ncommit: %s\nsource: %s\n",
			strings.TrimPrefix(Tag, "v"),
			GoVersion,
			Tag,
			Commit,
			SourceURL,
		)
		return exitUsage
	}
	bi := getBuildInfo()
	myModule, err := getMyModuleInfo(bi)
	if err != nil {
		log.Printf("failed to get my module info: %s", err)
		return exitError
	}
	fmt.Printf(
		"version: %s\nbuilt with: %s\ntag: %s\ncommit: %s\nsource: %s\n",
		strings.TrimPrefix(myModule.Version, "v"),
		bi.GoVersion,
		myModule.Version,
		"n/a",
		myModule.Path,
	)

	return exitUsage
}

func printVersionOnly() exitCode {
	if Tag != "" {
		fmt.Println(strings.TrimPrefix(Tag, "v"))
		return exitUsage
	}
	bi := getBuildInfo()
	myModule, err := getMyModuleInfo(bi)
	if err != nil {
		log.Printf("failed to get my module info: %s", err)
		return exitError
	}
	fmt.Println(strings.TrimPrefix(myModule.Version, "v"))

	return exitUsage
}
