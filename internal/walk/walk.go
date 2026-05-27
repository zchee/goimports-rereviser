package walk

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/alitto/pond"
)

const (
	GoExtension              = ".go"
	RecursivePath            = "./..."
	DefaultParallelThreshold = 8
)

var currentPaths = []string{".", "." + string(filepath.Separator)}

// ignoredByGoToolNames mirrors the directories the go tool skips when
// evaluating package patterns (see `go help packages`).
var ignoredByGoToolNames = map[string]struct{}{
	"testdata": {},
	"vendor":   {},
}

func IsDir(path string) (string, bool) {
	if path == RecursivePath || slices.Contains(currentPaths, path) {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return path, false
		}
	}

	dirStat, err := os.Stat(path)
	if err != nil {
		return path, false
	}

	return path, dirStat.IsDir()
}

func IsGoFile(path string) bool {
	return filepath.Ext(path) == GoExtension
}

// IsGoToolIgnored implements the go command's implicit exclusion rules:
// directories named vendor or testdata and any path component beginning with
// '.' or '_' are skipped when expanding patterns such as ./... .
func IsGoToolIgnored(path string) bool {
	base := filepath.Base(path)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return false
	}

	switch base[0] {
	case '.', '_':
		return true
	}

	_, ok := ignoredByGoToolNames[base]
	return ok
}

func NewSubmitter(providedPool *pond.WorkerPool, threshold int) (func(func()), func()) {
	var (
		pool        = providedPool
		poolMu      sync.Mutex
		poolCreated bool
		pending     sync.WaitGroup
		fileCount   atomic.Int32
	)

	if threshold <= 0 {
		threshold = DefaultParallelThreshold
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
		poolMu.Lock()
		createdPool := poolCreated
		currentPool := pool
		poolMu.Unlock()
		if createdPool && currentPool != nil {
			currentPool.StopAndWait()
		}
	}

	return submit, wait
}
