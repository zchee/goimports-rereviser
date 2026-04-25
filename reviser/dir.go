package reviser

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/alitto/pond"
	"github.com/charlievieth/fastwalk"
)

type walkCallbackFunc = func(hasChanged bool, path string, content []byte) error

type cachePolicy int

const (
	cacheDisabled cachePolicy = iota
	cacheReadWrite
)

const (
	goExtension              = ".go"
	recursivePath            = "./..."
	defaultParallelThreshold = 8
)

var (
	currentPaths = []string{".", "." + string(filepath.Separator)}
	// ignoredByGoToolNames mirrors the directories the go tool skips when
	// evaluating package patterns (see `go help packages`).
	ignoredByGoToolNames = map[string]struct{}{
		"testdata": {},
		"vendor":   {},
	}
)

var ErrPathIsNotDir = errors.New("path is not a directory")

// SourceDir to validate and fix import
type SourceDir struct {
	projectName         string
	dir                 string
	isRecursive         bool
	excludePatterns     []string // see filepath.Match
	workerPool          *pond.WorkerPool
	sequentialThreshold int
	cacheDir            string
	cacheEnabled        bool
	useMetadataCache    bool
	cacheFingerprint    string
	writeFile           func(name string, data []byte, perm fs.FileMode) error
}

func NewSourceDir(projectName, path string, isRecursive bool, excludes string) *SourceDir {
	patterns := make([]string, 0)

	// get the absolute path
	absPath, err := filepath.Abs(path)

	// if path is recursive, then we need to remove the "/..." suffix
	if path == recursivePath {
		isRecursive = true
		absPath = strings.TrimSuffix(absPath, "/...")
	}

	if err == nil {
		segs := strings.SplitSeq(excludes, ",")
		for seg := range segs {
			p := strings.TrimSpace(seg)
			if p != "" {
				if !filepath.IsAbs(p) {
					// resolve the absolute path
					p = filepath.Join(absPath, p)
				}
				// Check pattern is well-formed.
				if _, err = filepath.Match(p, ""); err == nil {
					patterns = append(patterns, p)
				}
			}
		}
	}
	return &SourceDir{
		projectName:         projectName,
		dir:                 absPath,
		isRecursive:         isRecursive,
		excludePatterns:     patterns,
		sequentialThreshold: defaultParallelThreshold,
		useMetadataCache:    true,
		writeFile:           os.WriteFile,
	}
}

// WithWorkerPool configures SourceDir to reuse an existing worker pool.
func (d *SourceDir) WithWorkerPool(pool *pond.WorkerPool) *SourceDir {
	d.workerPool = pool
	return d
}

// WithCache enables caching using the provided directory.
func (d *SourceDir) WithCache(cacheDir string) *SourceDir {
	if cacheDir == "" {
		return d
	}
	d.cacheDir = cacheDir
	d.cacheEnabled = true
	d.useMetadataCache = true
	return d
}

// WithCacheFingerprint scopes cache hits to the formatter configuration that
// produced them. Empty fingerprints preserve legacy cache behavior.
func (d *SourceDir) WithCacheFingerprint(fingerprint string) *SourceDir {
	d.cacheFingerprint = fingerprint
	return d
}

func (d *SourceDir) WithMetadataCache() *SourceDir {
	d.useMetadataCache = true
	return d
}

// WithoutMetadataCache forces the legacy hash-only cache validation path.
func (d *SourceDir) WithoutMetadataCache() *SourceDir {
	d.useMetadataCache = false
	return d
}

// WithSequentialThreshold overrides the minimum number of files before
// parallel execution is enabled. Primarily used for testing.
func (d *SourceDir) WithSequentialThreshold(threshold int) *SourceDir {
	d.sequentialThreshold = threshold
	return d
}

func (d *SourceDir) Fix(options ...SourceFileOption) error {
	var ok bool
	d.dir, ok = IsDir(d.dir)
	if !ok {
		return ErrPathIsNotDir
	}

	submit, wait := d.makeSubmitter()

	// Collect files and submit to worker pool
	var collectErr error
	var processingErr error
	var errMu sync.Mutex

	err := fastwalk.Walk(&fastwalk.DefaultConfig, d.dir, d.walk(
		submit,
		func(hasChanged bool, path string, content []byte) error {
			if !hasChanged {
				return nil
			}
			if err := d.writeFile(path, content, 0o644); err != nil {
				return fmt.Errorf("failed to write fixed result to file(%s): %w", path, err)
			}
			return nil
		},
		&errMu,
		&processingErr,
		cacheReadWrite,
		options...,
	))
	wait()
	if err != nil {
		collectErr = fmt.Errorf("failed to walk dir: %w", err)
	}

	// Return first error encountered
	if collectErr != nil {
		return collectErr
	}
	if processingErr != nil {
		return processingErr
	}

	return nil
}

// Find collection of bad formatted paths
func (d *SourceDir) Find(options ...SourceFileOption) (*UnformattedCollection, error) {
	var (
		ok                     bool
		badFormattedCollection []string
		collectionMu           sync.Mutex
	)
	d.dir, ok = IsDir(d.dir)
	if !ok {
		return nil, ErrPathIsNotDir
	}

	submit, wait := d.makeSubmitter()

	var collectErr error
	var processingErr error
	var errMu sync.Mutex

	err := filepath.WalkDir(d.dir, d.walk(
		submit,
		func(hasChanged bool, path string, content []byte) error {
			if !hasChanged {
				return nil
			}
			collectionMu.Lock()
			badFormattedCollection = append(badFormattedCollection, path)
			collectionMu.Unlock()
			return nil
		},
		&errMu,
		&processingErr,
		cacheDisabled,
		options...,
	))
	wait()
	if err != nil {
		collectErr = fmt.Errorf("failed to walk dir: %w", err)
	}

	// Return first error encountered
	if collectErr != nil {
		return nil, collectErr
	}
	if processingErr != nil {
		return nil, processingErr
	}

	if len(badFormattedCollection) == 0 {
		return nil, nil
	}

	return newUnformattedCollection(badFormattedCollection), nil
}

// walk submits file processing to worker pool for concurrent execution.
func (d *SourceDir) walk(submit func(func()), callback walkCallbackFunc, errMu *sync.Mutex, processingErr *error, cacheMode cachePolicy, options ...SourceFileOption) fs.WalkDirFunc {
	return func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.isRecursive && dirEntry.IsDir() && filepath.Base(d.dir) != dirEntry.Name() {
			return filepath.SkipDir
		}
		if dirEntry.IsDir() && d.isExcluded(path) {
			return filepath.SkipDir
		}

		// Submit Go file processing to worker pool
		if isGoFile(path) && !dirEntry.IsDir() && !d.isExcluded(path) {
			filePath := path

			submit(func() {
				absPath := filePath
				if !filepath.IsAbs(absPath) {
					absPath = filepath.Join(d.dir, filePath)
				}

				if d.cacheEnabled && cacheMode == cacheReadWrite {
					skip, cacheErr := ShouldSkipWithFingerprint(d.cacheDir, absPath, d.useMetadataCache, d.cacheFingerprint)
					if cacheErr != nil {
						errMu.Lock()
						if *processingErr == nil {
							*processingErr = cacheErr
						}
						errMu.Unlock()
						return
					}
					if skip {
						return
					}
				}

				content, _, hasChange, err := NewSourceFile(d.projectName, absPath).Fix(options...)
				if err != nil {
					errMu.Lock()
					if *processingErr == nil {
						*processingErr = fmt.Errorf("failed to fix %s: %w", absPath, err)
					}
					errMu.Unlock()
					return
				}

				if err := callback(hasChange, absPath, content); err != nil {
					errMu.Lock()
					if *processingErr == nil {
						*processingErr = err
					}
					errMu.Unlock()
					return
				}

				if d.cacheEnabled && cacheMode == cacheReadWrite {
					hash := ComputeContentHash(content)
					if hash == "" {
						return
					}

					entry, metaErr := NewCacheEntryWithFingerprint(absPath, hash, d.useMetadataCache, d.cacheFingerprint)
					if metaErr != nil {
						errMu.Lock()
						if *processingErr == nil {
							*processingErr = metaErr
						}
						errMu.Unlock()
						return
					}

					if cacheErr := d.writeCache(absPath, entry); cacheErr != nil {
						errMu.Lock()
						if *processingErr == nil {
							*processingErr = cacheErr
						}
						errMu.Unlock()
					}
				}
			})
		}
		return nil
	}
}

func (d *SourceDir) makeSubmitter() (func(func()), func()) {
	var (
		providedPool = d.workerPool
		pool         = providedPool
		poolMu       sync.Mutex
		poolCreated  bool
		pending      sync.WaitGroup
		fileCount    atomic.Int32
	)

	threshold := d.sequentialThreshold
	if threshold <= 0 {
		threshold = defaultParallelThreshold
	}

	canCreatePool := providedPool == nil

	submit := func(task func()) {
		poolMu.Lock()
		currentPool := pool
		poolMu.Unlock()

		if currentPool != nil {
			pending.Add(1)
			currentPool.Submit(func() {
				defer pending.Done()
				task()
			})
			return
		}

		count := fileCount.Add(1)
		if !canCreatePool || int(count) <= threshold {
			task()
			return
		}

		poolMu.Lock()
		if pool == nil && canCreatePool {
			pool = pond.New(runtime.GOMAXPROCS(0)*2, 0)
			poolCreated = true
		}
		currentPool = pool
		poolMu.Unlock()

		if currentPool != nil {
			pending.Add(1)
			currentPool.Submit(func() {
				defer pending.Done()
				task()
			})
			return
		}

		task()
	}

	wait := func() {
		pending.Wait()
		if poolCreated {
			pool.StopAndWait()
		}
	}

	return submit, wait
}

func (d *SourceDir) isExcluded(path string) bool {
	var absPath string
	if filepath.IsAbs(path) {
		absPath = path
	} else {
		absPath = filepath.Join(d.dir, path)
	}

	if isGoToolIgnored(absPath) {
		return true
	}

	for _, pattern := range d.excludePatterns {
		matched, err := filepath.Match(pattern, absPath)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// isGoToolIgnored implements the go command's implicit exclusion rules:
// directories named vendor or testdata and any path component beginning with
// '.' or '_' are skipped when expanding patterns such as ./... .
func isGoToolIgnored(path string) bool {
	base := filepath.Base(path)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return false
	}

	switch base[0] {
	case '.':
		return true
	case '_':
		return true
	}

	_, ok := ignoredByGoToolNames[base]
	return ok
}

type UnformattedCollection struct {
	list []string
}

func newUnformattedCollection(list []string) *UnformattedCollection {
	return &UnformattedCollection{
		list: list,
	}
}

func (c *UnformattedCollection) List() []string {
	list := make([]string, len(c.list))
	copy(list, c.list)
	return list
}

func (c *UnformattedCollection) String() string {
	if c == nil {
		return ""
	}

	var builder strings.Builder
	for i, file := range c.list {
		builder.WriteString(file)
		if len(c.list)-1 > i {
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func IsDir(path string) (string, bool) {
	if path == recursivePath || slices.Contains(currentPaths, path) {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return path, false
		}
	}

	dir, err := os.Open(path)
	if err != nil {
		return path, false
	}

	dirStat, err := dir.Stat()
	if err != nil {
		return path, false
	}

	return path, dirStat.IsDir()
}

func isGoFile(path string) bool {
	return filepath.Ext(path) == goExtension
}

func (d *SourceDir) shouldSkipByCache(path string) (bool, error) {
	if !d.cacheEnabled || d.cacheDir == "" {
		return false, nil
	}

	absPath := path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(d.dir, path)
	}

	return ShouldSkipWithFingerprint(d.cacheDir, absPath, d.useMetadataCache, d.cacheFingerprint)
}

func (d *SourceDir) writeCache(path string, entry CacheEntry) error {
	if !d.cacheEnabled || d.cacheDir == "" || entry.Hash == "" {
		return nil
	}

	absPath := path
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(d.dir, path)
	}

	return WriteCacheEntry(d.cacheDir, absPath, entry)
}
