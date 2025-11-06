package reviser

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
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

const (
	goExtension              = ".go"
	recursivePath            = "./..."
	defaultParallelThreshold = 8
)

var currentPaths = []string{".", "." + string(filepath.Separator)}

var ErrPathIsNotDir = errors.New("path is not a directory")

// SourceDir to validate and fix import
type SourceDir struct {
	projectName         string
	dir                 string
	isRecursive         bool
	excludePatterns     []string // see filepath.Match
	workerPool          *pond.WorkerPool
	sequentialThreshold int
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
	}
}

// WithWorkerPool configures SourceDir to reuse an existing worker pool.
func (d *SourceDir) WithWorkerPool(pool *pond.WorkerPool) *SourceDir {
	d.workerPool = pool
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
	defer wait()

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
			if err := os.WriteFile(path, content, 0o644); err != nil {
				log.Fatalf("failed to write fixed result to file(%s): %+v\n", path, err)
				return err
			}
			return nil
		},
		&errMu,
		&processingErr,
		options...,
	))

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
	defer wait()

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
		options...,
	))

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
func (d *SourceDir) walk(submit func(func()), callback walkCallbackFunc, errMu *sync.Mutex, processingErr *error, options ...SourceFileOption) fs.WalkDirFunc {
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
				content, _, hasChange, err := NewSourceFile(d.projectName, filePath).Fix(options...)
				if err != nil {
					errMu.Lock()
					if *processingErr == nil {
						*processingErr = fmt.Errorf("failed to fix %s: %w", filePath, err)
					}
					errMu.Unlock()
					return
				}

				if err := callback(hasChange, filePath, content); err != nil {
					errMu.Lock()
					if *processingErr == nil {
						*processingErr = err
					}
					errMu.Unlock()
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
		fileCount    int32
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

		count := atomic.AddInt32(&fileCount, 1)
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
	for _, pattern := range d.excludePatterns {
		matched, err := filepath.Match(pattern, absPath)
		if err == nil && matched {
			return true
		}
	}
	return false
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
