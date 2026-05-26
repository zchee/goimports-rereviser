package pkgdeps

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestLoadSingleflight(t *testing.T) {
	ClearCache()

	originalLoader := loadFunc
	t.Cleanup(func() { loadFunc = originalLoader })

	var callCount atomic.Int32
	ready := make(chan struct{})
	proceed := make(chan struct{})

	loadFunc = func(dir, buildTag string) (PackageImports, error) {
		if callCount.Add(1) == 1 {
			close(ready)
		}
		<-proceed
		return PackageImports{"example.com/pkg": "pkg"}, nil
	}

	const goroutineCount = 8
	wg := new(sync.WaitGroup)
	errCh := make(chan error, goroutineCount)

	for range goroutineCount {
		wg.Go(func() {
			imports, err := Load("/tmp/test", "")
			if err != nil {
				errCh <- fmt.Errorf("Load failed: %w", err)
				return
			}
			if imports["example.com/pkg"] != "pkg" {
				errCh <- fmt.Errorf("unexpected imports map: %v", imports)
				return
			}
			errCh <- nil
		})
	}

	<-ready
	close(proceed)
	wg.Wait()

	for range goroutineCount {
		if err := <-errCh; err != nil {
			t.Fatalf("goroutine returned error: %v", err)
		}
	}

	if got := callCount.Load(); got != 1 {
		t.Fatalf("expected single loader invocation, got %d", got)
	}

	loadFunc = func(dir, buildTag string) (PackageImports, error) {
		callCount.Add(1)
		return nil, fmt.Errorf("unexpected loader invocation")
	}

	imports, err := Load("/tmp/test", "")
	if err != nil {
		t.Fatalf("unexpected error retrieving from cache: %v", err)
	}
	if imports["example.com/pkg"] != "pkg" {
		t.Fatalf("cached result mismatch: %v", imports)
	}
	if got := callCount.Load(); got != 1 {
		t.Fatalf("cache miss incremented loader count: %d", got)
	}
}

func TestLoadDoesNotCacheErrors(t *testing.T) {
	ClearCache()

	originalLoader := loadFunc
	t.Cleanup(func() { loadFunc = originalLoader })

	var callCount atomic.Int32
	loadFunc = func(dir, buildTag string) (PackageImports, error) {
		if callCount.Add(1) == 1 {
			return nil, fmt.Errorf("transient failure")
		}
		return PackageImports{"example.com/pkg": "pkg"}, nil
	}

	if _, err := Load("/tmp/test", ""); err == nil {
		t.Fatal("expected first call to fail")
	}

	imports, err := Load("/tmp/test", "")
	if err != nil {
		t.Fatalf("expected second call to succeed, got %v", err)
	}
	if imports["example.com/pkg"] != "pkg" {
		t.Fatalf("unexpected imports map: %v", imports)
	}
	if got := callCount.Load(); got != 2 {
		t.Fatalf("expected uncached retry after error, got %d loader calls", got)
	}

	imports, err = Load("/tmp/test", "")
	if err != nil {
		t.Fatalf("expected recovered result to be cached, got %v", err)
	}
	if imports["example.com/pkg"] != "pkg" {
		t.Fatalf("cached imports mismatch: %v", imports)
	}
	if got := callCount.Load(); got != 2 {
		t.Fatalf("cached recovered result re-ran loader: %d", got)
	}
}

func TestLoadSeparatesBuildTags(t *testing.T) {
	ClearCache()

	originalLoader := loadFunc
	t.Cleanup(func() { loadFunc = originalLoader })

	var callCount atomic.Int32
	loadFunc = func(dir, buildTag string) (PackageImports, error) {
		callCount.Add(1)
		return PackageImports{buildTag: buildTag}, nil
	}

	untagged, err := Load("/tmp/test", "")
	if err != nil {
		t.Fatalf("unexpected error loading untagged deps: %v", err)
	}
	if got := untagged[""]; got != "" {
		t.Fatalf("unexpected untagged value: %q", got)
	}

	tagged, err := Load("/tmp/test", "custom")
	if err != nil {
		t.Fatalf("unexpected error loading tagged deps: %v", err)
	}
	if got := tagged["custom"]; got != "custom" {
		t.Fatalf("unexpected tagged value: %q", got)
	}

	if got := callCount.Load(); got != 2 {
		t.Fatalf("expected separate cache entries per build tag, got %d loader calls", got)
	}
}
