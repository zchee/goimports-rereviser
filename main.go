package main

import (
	"os"

	"github.com/zchee/goimports-rereviser/v4/internal/cli"
)

// Sets injected via ldflags at build time.
var (
	Tag       string
	Commit    string
	SourceURL string
	GoVersion string
)

func main() {
	os.Exit(cli.Run(cli.VersionInfo{
		Tag:       Tag,
		Commit:    Commit,
		SourceURL: SourceURL,
		GoVersion: GoVersion,
	}))
}
