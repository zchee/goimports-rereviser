package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/alitto/pond"
	"golang.org/x/sync/errgroup"

	"github.com/zchee/goimports-rereviser/v4/pkg/module"
	"github.com/zchee/goimports-rereviser/v4/reviser"
)

// Sets injected via ldflags at build time.
var (
	Tag       string
	Commit    string
	SourceURL string
	GoVersion string
)

// Config holds goimports-rereviser configuration options.
type Config struct {
	projectName        string
	companyPkgPrefixes string
	output             string
	excludes           string
	importsOrder       string

	shouldShowVersionOnly bool
	shouldShowVersion     bool

	listFileName     bool
	setExitStatus    bool
	isRecursive      bool
	isUseCache       bool
	useMetadataCache bool

	shouldRemoveUnusedImports   bool
	shouldSetAlias              bool
	shouldFormat                bool
	shouldSeparateNamedImports  bool
	shouldApplyToGeneratedFiles bool
}

var cfg = Config{}

func init() {
	flag.StringVar(&cfg.projectName, "project-name", "", `Your project name(ex.: github.com/zchee/goimports-rereviser). Optional parameter.`)
	flag.StringVar(&cfg.companyPkgPrefixes, "company-prefixes", "", `Company package prefixes which will be placed after 3rd-party group by default(if defined). Values should be comma-separated. Optional parameters.`)
	flag.StringVar(&cfg.output, "output", "file", `Can be "file", "write" or "stdout". Whether to write the formatted content back to the file or to stdout. When "write" together with "-list-diff" will list the file name and write back to the file. Optional parameter.`)
	flag.StringVar(&cfg.excludes, "excludes", "", `Exclude files or dirs, example: '.git/,proto/*.go'.`)
	flag.StringVar(&cfg.importsOrder, "imports-order", "std,general,company,project", `Your imports groups can be sorted in your way. Optional parameter.
std - std import group.
general - libs for general purpose.
company - inter-org or your company libs(if you set '-company-prefixes'-option, then 4th group will be split separately. In other case, it will be the part of general purpose libs).
project - your local project dependencies.
blanked - imports with "_" alias.
dotted - imports with "." alias.`,
	)
	flag.BoolVar(&cfg.listFileName, "list-diff", false, `Option will list files whose formatting differs from goimports-rereviser. Optional parameter.`)
	flag.BoolVar(&cfg.setExitStatus, "set-exit-status", false, `set the exit status to 1 if a change is needed/made. Optional parameter.`)
	flag.BoolVar(&cfg.isRecursive, "recursive", false, `Apply rules recursively if target is a directory. In case of ./... execution will be recursively applied by default. Optional parameter.`)
	flag.BoolVar(&cfg.isUseCache, "use-cache", false, `Use cache to improve performance. Optional parameter.`)
	flag.BoolVar(&cfg.useMetadataCache, "cache-fast-skip", true, `Metadata-first cache path (default). Uses file metadata to skip hashing unchanged files; disable with -cache-fast-skip=false.`)

	flag.BoolVar(&cfg.shouldRemoveUnusedImports, "rm-unused", false, `Remove unused imports. Optional parameter.`)
	flag.BoolVar(&cfg.shouldSetAlias, "set-alias", false, `Set alias for versioned package names, like 'github.com/go-pg/pg/v9'. In this case import will be set as 'pg \"github.com/go-pg/pg/v9\"'. Optional parameter.`)
	flag.BoolVar(&cfg.shouldFormat, "format", false, `Option will perform additional formatting. Optional parameter.`)
	flag.BoolVar(&cfg.shouldSeparateNamedImports, "separate-named", false, `Option will separate named imports from the rest of the imports, per group. Optional parameter.`)
	flag.BoolVar(&cfg.shouldApplyToGeneratedFiles, "apply-to-generated-files", false, `Apply imports sorting and formatting(if the option is set) to generated files. Generated file is a file with first comment which starts with comment '// Code generated'. Optional parameter.`)
	flag.BoolVar(&cfg.shouldShowVersion, "version", false, `Show version information`)
	flag.BoolVar(&cfg.shouldShowVersionOnly, "version-only", false, `Show only the version string`)
}

type exitCode = int

const (
	exitSuccess exitCode = iota
	exitError

	exitUsage = exitSuccess
)

func main() {
	os.Exit(run())
}

func run() exitCode {
	flag.Parse()

	if cfg.shouldShowVersionOnly {
		return printVersionOnly()
	}

	if cfg.shouldShowVersion {
		return printVersion()
	}

	if cfg.useMetadataCache && !cfg.isUseCache {
		return printUsageAndExit(errors.New("cache-fast-skip requires --use-cache"))
	}

	originPaths := flag.Args()
	if len(originPaths) == 0 {
		return printUsageAndExit(errors.New("no file(s) or directory(ies) specified on input"))
	}

	if len(originPaths) == 1 && originPaths[0] == "-" {
		originPaths[0] = reviser.StandardInput
		if err := validateRequiredParam(originPaths[0]); err != nil {
			return printUsageAndExit(err)
		}
	}

	var opts reviser.SourceFileOptions
	if cfg.importsOrder != "" {
		order, err := reviser.StringToImportsOrders(cfg.importsOrder)
		if err != nil {
			return printUsageAndExit(err)
		}
		opts = append(opts, reviser.WithImportsOrder(order))
	}
	if cfg.shouldRemoveUnusedImports {
		opts = append(opts, reviser.WithRemovingUnusedImports)
	}
	if cfg.shouldSetAlias {
		opts = append(opts, reviser.WithUsingAliasForVersionSuffix)
	}
	if cfg.shouldFormat {
		opts = append(opts, reviser.WithCodeFormatting)
	}
	if cfg.shouldSeparateNamedImports {
		opts = append(opts, reviser.WithSeparatedNamedImports)
	}
	if !cfg.shouldApplyToGeneratedFiles {
		opts = append(opts, reviser.WithSkipGeneratedFile)
	}
	if cfg.companyPkgPrefixes != "" {
		opts = append(opts, reviser.WithCompanyPackagePrefixes(cfg.companyPkgPrefixes))
	}

	log.Printf("Paths: %v\n", originPaths)

	var cacheDir string
	if cfg.isUseCache {
		if xdgCacheDir := os.Getenv("XDG_CACHE_HOME"); xdgCacheDir != "" {
			cacheDir = filepath.Join(xdgCacheDir, "goimports-rereviser")
		}

		if cacheDir == "" {
			usr, err := user.Current()
			if err != nil {
				log.Fatalf("Failed to get current user: %+v\n", err)
			}
			cacheDir = filepath.Join(usr.HomeDir, ".cache", "goimports-rereviser")
		}

		if err := os.MkdirAll(cacheDir, os.ModePerm); err != nil {
			log.Fatalf("Failed to create cache directory: %+v\n", err)
		}
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)

	hasChange, err := processPaths(ctx, &cfg, originPaths, cacheDir, opts)
	if err != nil {
		return printUsageAndExit(err)
	}

	if hasChange && cfg.setExitStatus {
		log.Println("detect changed files")
		return exitError
	}

	return exitSuccess
}

func processPaths(ctx context.Context, cfg *Config, originPaths []string, cacheDir string, options reviser.SourceFileOptions) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	var (
		hasChange      bool
		hasChangeMu    sync.Mutex
		sharedPool     *pond.WorkerPool
		sharedPoolOnce sync.Once
	)

	getSharedPool := func() *pond.WorkerPool {
		sharedPoolOnce.Do(func() {
			workerCount := runtime.GOMAXPROCS(0) * 2
			sharedPool = pond.New(workerCount, 0)
		})
		return sharedPool
	}

	g, ctx := errgroup.WithContext(ctx)

	for _, original := range originPaths {
		pathValue := original

		g.Go(func() error {
			log.Printf("Processing %s\n", pathValue)
			originProjectName, err := determineProjectName(cfg.projectName, pathValue)
			if err != nil {
				return fmt.Errorf("Could not determine project name for path %s: %w", pathValue, err)
			}

			if _, ok := reviser.IsDir(pathValue); ok {
				if cfg.listFileName {
					dir := reviser.NewSourceDir(originProjectName, pathValue, cfg.isRecursive, cfg.excludes).
						WithWorkerPool(getSharedPool())
					if cfg.isUseCache && cacheDir != "" {
						dir = dir.WithCache(cacheDir)
						if !cfg.useMetadataCache {
							dir = dir.WithoutMetadataCache()
						}
					}

					unformattedFiles, err := dir.Find(options...)
					if err != nil {
						return fmt.Errorf("Failed to find unformatted files %s: %w", pathValue, err)
					}
					if unformattedFiles != nil {
						fmt.Printf("%s\n", unformattedFiles.String())
						if cfg.setExitStatus {
							os.Exit(1)
						}
					}
					return nil
				}

				dir := reviser.NewSourceDir(originProjectName, pathValue, cfg.isRecursive, cfg.excludes).
					WithWorkerPool(getSharedPool())
				if cfg.isUseCache && cacheDir != "" {
					dir = dir.WithCache(cacheDir)
					if !cfg.useMetadataCache {
						dir = dir.WithoutMetadataCache()
					}
				}

				if err := dir.Fix(options...); err != nil {
					return fmt.Errorf("Failed to fix directory %s: %w", pathValue, err)
				}
				return nil
			}

			pathToProcess := pathValue
			if pathValue != reviser.StandardInput {
				pathToProcess, err = filepath.Abs(pathValue)
				if err != nil {
					return fmt.Errorf("Failed to get abs path for %s: %w", pathValue, err)
				}
			}

			var (
				formattedOutput []byte
				pathHasChange   bool
			)

			if cfg.isUseCache {
				skip, checkErr := reviser.ShouldSkip(cacheDir, pathToProcess, cfg.useMetadataCache)
				if checkErr != nil {
					return fmt.Errorf("Failed to evaluate cache for %s: %w", pathToProcess, checkErr)
				}
				if skip {
					return nil
				}
			}

			formattedOutput, _, pathHasChange, err = reviser.NewSourceFile(originProjectName, pathToProcess).Fix(options...)
			if err != nil {
				return fmt.Errorf("Failed to fix file %s: %w", pathToProcess, err)
			}

			if cfg.isUseCache {
				hash := reviser.ComputeContentHash(formattedOutput)
				entry, entryErr := reviser.NewCacheEntry(pathToProcess, hash, cfg.useMetadataCache)
				if entryErr != nil {
					return fmt.Errorf("Failed to build cache entry for %s: %w", pathToProcess, entryErr)
				}
				if writeErr := reviser.WriteCacheEntry(cacheDir, pathToProcess, entry); writeErr != nil {
					cacheFile := reviser.CacheFilePath(cacheDir, pathToProcess)
					return fmt.Errorf("Failed to write cache file %s: %w", cacheFile, writeErr)
				}
			}

			if pathHasChange {
				hasChangeMu.Lock()
				hasChange = true
				hasChangeMu.Unlock()
			}

			return resultPostProcess(pathHasChange, pathToProcess, formattedOutput)
		})
	}

	err := g.Wait()

	if sharedPool != nil {
		sharedPool.StopAndWait()
	}

	if err != nil {
		return false, err
	}

	return hasChange, nil
}

func resultPostProcess(hasChange bool, originFilePath string, formattedOutput []byte) error {
	switch {
	case hasChange && cfg.listFileName && cfg.output != "write":
		fmt.Println(originFilePath)

	case cfg.output == "stdout" || originFilePath == reviser.StandardInput:
		fmt.Print(string(formattedOutput))

	case cfg.output == "file" || cfg.output == "write":
		if err := os.WriteFile(originFilePath, formattedOutput, 0o644); err != nil {
			return fmt.Errorf("failed to write fixed result to file(%s): %w", originFilePath, err)
		}

		if cfg.listFileName {
			fmt.Println(originFilePath)
		}

	default:
		return fmt.Errorf("invalid output %q specified", cfg.output)
	}

	return nil
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

func determineProjectName(projectName, filePath string) (string, error) {
	if filePath == reviser.StandardInput {
		var err error
		filePath, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}

	return module.DetermineProjectName(projectName, filePath)
}
