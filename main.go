package main

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/user"
	"path"
	"path/filepath"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/zchee/goimports-rereviser/v4/pkg/module"
	"github.com/zchee/goimports-rereviser/v4/reviser"
)

var (
	Tag       string
	Commit    string
	SourceURL string
	GoVersion string
)

type Config struct {
	projectName        string
	companyPkgPrefixes string
	output             string
	importsOrder       string
	excludes           string

	shouldShowVersion           bool
	shouldShowVersionOnly       bool
	shouldRemoveUnusedImports   bool
	shouldSetAlias              bool
	shouldFormat                bool
	shouldApplyToGeneratedFiles bool
	shouldSeparateNamedImports  bool
	listFileName                bool
	setExitStatus               bool
	isRecursive                 bool
	isUseCache                  bool
}

var cfg = Config{}

func init() {
	flag.StringVar(&cfg.excludes, "excludes", "", `Exclude files or dirs, example: '.git/,proto/*.go'.`)
	flag.StringVar(&cfg.projectName, "project-name", "", `Your project name(ex.: github.com/zchee/goimports-rereviser). Optional parameter.`)
	flag.StringVar(&cfg.companyPkgPrefixes, "company-prefixes", "", `Company package prefixes which will be placed after 3rd-party group by default(if defined). Values should be comma-separated. Optional parameters.`)
	flag.StringVar(&cfg.output, "output", "file", `Can be "file", "write" or "stdout". Whether to write the formatted content back to the file or to stdout. When "write" together with "-list-diff" will list the file name and write back to the file. Optional parameter.`)
	flag.StringVar(&cfg.importsOrder, "imports-order", "std,general,company,project", `Your imports groups can be sorted in your way. 
std - std import group; 
general - libs for general purpose; 
company - inter-org or your company libs(if you set '-company-prefixes'-option, then 4th group will be split separately. In other case, it will be the part of general purpose libs); 
project - your local project dependencies;
blanked - imports with "_" alias;
dotted - imports with "." alias.
Optional parameter.`,
	)
	flag.BoolVar(&cfg.listFileName, "list-diff", false, `Option will list files whose formatting differs from goimports-rereviser. Optional parameter.`)
	flag.BoolVar(&cfg.setExitStatus, "set-exit-status", false, `set the exit status to 1 if a change is needed/made. Optional parameter.`)
	flag.BoolVar(&cfg.shouldRemoveUnusedImports, "rm-unused", false, `Remove unused imports. Optional parameter.`)
	flag.BoolVar(&cfg.shouldSetAlias, "set-alias", false, `Set alias for versioned package names, like 'github.com/go-pg/pg/v9'. In this case import will be set as 'pg \"github.com/go-pg/pg/v9\"'. Optional parameter.`)
	flag.BoolVar(&cfg.shouldFormat, "format", false, `Option will perform additional formatting. Optional parameter.`)
	flag.BoolVar(&cfg.shouldSeparateNamedImports, "separate-named", false, `Option will separate named imports from the rest of the imports, per group. Optional parameter.`)
	flag.BoolVar(&cfg.isRecursive, "recursive", false, `Apply rules recursively if target is a directory. In case of ./... execution will be recursively applied by default. Optional parameter.`)
	flag.BoolVar(&cfg.isUseCache, "use-cache", false, `Use cache to improve performance. Optional parameter.`)
	flag.BoolVar(&cfg.shouldApplyToGeneratedFiles, "apply-to-generated-files", false, `Apply imports sorting and formatting(if the option is set) to generated files. Generated file is a file with first comment which starts with comment '// Code generated'. Optional parameter.`)
	flag.BoolVar(&cfg.shouldShowVersion, "version", false, `Show version information`)
	flag.BoolVar(&cfg.shouldShowVersionOnly, "version-only", false, `Show only the version string`)
}

func main() {
	flag.Parse()

	if cfg.shouldShowVersionOnly {
		printVersionOnly()
		return
	}

	if cfg.shouldShowVersion {
		printVersion()
		return
	}

	originPaths := flag.Args()

	if len(originPaths) == 0 {
		printUsageAndExit(errors.New("no file(s) or directory(ies) specified on input"))
	}

	if len(originPaths) == 1 && originPaths[0] == "-" {
		originPaths[0] = reviser.StandardInput
		if err := validateRequiredParam(originPaths[0]); err != nil {
			printUsageAndExit(err)
		}
	}

	var options reviser.SourceFileOptions
	if cfg.shouldRemoveUnusedImports {
		options = append(options, reviser.WithRemovingUnusedImports)
	}

	if cfg.shouldSetAlias {
		options = append(options, reviser.WithUsingAliasForVersionSuffix)
	}

	if cfg.shouldFormat {
		options = append(options, reviser.WithCodeFormatting)
	}

	if !cfg.shouldApplyToGeneratedFiles {
		options = append(options, reviser.WithSkipGeneratedFile)
	}

	if cfg.shouldSeparateNamedImports {
		options = append(options, reviser.WithSeparatedNamedImports)
	}

	if cfg.companyPkgPrefixes != "" {
		options = append(options, reviser.WithCompanyPackagePrefixes(cfg.companyPkgPrefixes))
	}

	if cfg.importsOrder != "" {
		order, err := reviser.StringToImportsOrders(cfg.importsOrder)
		if err != nil {
			printUsageAndExit(err)
		}
		options = append(options, reviser.WithImportsOrder(order))
	}

	var hasChange bool
	var hasChangeMu sync.Mutex
	log.Printf("Paths: %v\n", originPaths)

	var cacheDir string
	if cfg.isUseCache {
		u, err := user.Current()
		if err != nil {
			log.Fatalf("Failed to get current user: %+v\n", err)
		}

		cacheDir = path.Join(u.HomeDir, ".cache", "goimports-rereviser")
		if err = os.MkdirAll(cacheDir, os.ModePerm); err != nil {
			log.Fatalf("Failed to create cache directory: %+v\n", err)
		}
	}

	// Process paths concurrently using errgroup for coordinated execution
	g := new(errgroup.Group)

	for _, originPath := range originPaths {
		// Capture loop variable for goroutine

		g.Go(func() error {
			log.Printf("Processing %s\n", originPath)
			originProjectName, err := determineProjectName(cfg.projectName, originPath, osGetwdOption)
			if err != nil {
				return fmt.Errorf("Could not determine project name for path %s: %s", originPath, err)
			}

			if _, ok := reviser.IsDir(originPath); ok {
				if cfg.listFileName {
					unformattedFiles, err := reviser.NewSourceDir(originProjectName, originPath, cfg.isRecursive, cfg.excludes).Find(options...)
					if err != nil {
						log.Fatalf("Failed to find unformatted files %s: %+v\n", originPath, err)
					}

					if unformattedFiles != nil {
						fmt.Printf("%s\n", unformattedFiles.String())
						if cfg.setExitStatus {
							os.Exit(1)
						}
					}

					return nil
				}

				err := reviser.NewSourceDir(originProjectName, originPath, cfg.isRecursive, cfg.excludes).Fix(options...)
				if err != nil {
					log.Fatalf("Failed to fix directory %s: %+v\n", originPath, err)
				}

				return nil
			}

			pathToProcess := originPath
			if originPath != reviser.StandardInput {
				pathToProcess, err = filepath.Abs(originPath)
				if err != nil {
					log.Fatalf("Failed to get abs path: %+v\n", err)
				}
			}

			var formattedOutput []byte
			var pathHasChange bool
			if cfg.isUseCache {
				hash := md5.Sum([]byte(pathToProcess))
				cacheFile := path.Join(cacheDir, hex.EncodeToString(hash[:]))

				var cacheContent, fileContent []byte
				if cacheContent, err = os.ReadFile(cacheFile); err == nil {
					// compare file content hash
					var fileHashHex string
					if fileContent, err = os.ReadFile(pathToProcess); err == nil {
						fileHash := md5.Sum(fileContent)
						fileHashHex = hex.EncodeToString(fileHash[:])
					}
					if string(cacheContent) == fileHashHex {
						// cache hit - skip processing
						return nil
					}
				}
				formattedOutput, _, pathHasChange, err = reviser.NewSourceFile(originProjectName, pathToProcess).Fix(options...)
				if err != nil {
					log.Fatalf("Failed to fix file: %+v\n", err)
				}
				fileHash := md5.Sum(formattedOutput)
				fileHashHex := hex.EncodeToString(fileHash[:])
				if err = os.WriteFile(cacheFile, []byte(fileHashHex), 0o644); err != nil {
					log.Fatalf("Failed to write cache file: %+v\n", err)
				}
			} else {
				formattedOutput, _, pathHasChange, err = reviser.NewSourceFile(originProjectName, pathToProcess).Fix(options...)
				if err != nil {
					log.Fatalf("Failed to fix file: %+v\n", err)
				}
			}

			// Thread-safe update of hasChange
			if pathHasChange {
				hasChangeMu.Lock()
				hasChange = true
				hasChangeMu.Unlock()
			}

			resultPostProcess(pathHasChange, pathToProcess, formattedOutput)
			return nil
		})
	}

	// Wait for all paths to complete
	if err := g.Wait(); err != nil {
		printUsageAndExit(err)
	}

	if hasChange && cfg.setExitStatus {
		os.Exit(1)
	}
}

func resultPostProcess(hasChange bool, originFilePath string, formattedOutput []byte) {
	switch {
	case hasChange && cfg.listFileName && cfg.output != "write":
		fmt.Println(originFilePath)
	case cfg.output == "stdout" || originFilePath == reviser.StandardInput:
		fmt.Print(string(formattedOutput))
	case cfg.output == "file" || cfg.output == "write":
		if err := os.WriteFile(originFilePath, formattedOutput, 0o644); err != nil {
			log.Fatalf("failed to write fixed result to file(%s): %+v\n", originFilePath, err)
		}
		if cfg.listFileName {
			fmt.Println(originFilePath)
		}
	default:
		log.Fatalf(`invalid output %q specified`, cfg.output)
	}
}

func validateRequiredParam(filePath string) error {
	if filePath == reviser.StandardInput {
		stat, _ := os.Stdin.Stat()
		if stat.Mode()&os.ModeNamedPipe == 0 {
			// no data on stdin
			return errors.New("no data on stdin")
		}
	}
	return nil
}

type Option func() (string, error)

func osGetwdOption() (string, error) {
	return os.Getwd()
}

func determineProjectName(projectName, filePath string, option Option) (string, error) {
	if filePath == reviser.StandardInput {
		var err error
		filePath, err = option()
		if err != nil {
			return "", err
		}
	}

	return module.DetermineProjectName(projectName, filePath)
}
