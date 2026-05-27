package pkgdeps

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/tools/go/packages"
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

func TestCollectPackageErrorsReturnsJoinedPackageErrors(t *testing.T) {
	t.Parallel()

	err := collectPackageErrors([]*packages.Package{
		{
			Errors: []packages.Error{
				{Pos: "a.go:3:8", Msg: "no required module provides package example.com/does/not/exist", Kind: packages.ListError},
				{Pos: "b.go:7:9", Msg: "undefined: missing", Kind: packages.TypeError},
			},
		},
	})
	if err == nil {
		t.Fatalf("expected joined package errors")
	}

	var joined interface{ Unwrap() []error }
	if !errors.As(err, &joined) {
		t.Fatalf("expected joined package errors, got %T: %v", err, err)
	}
	if len(joined.Unwrap()) == 0 {
		t.Fatalf("expected joined error to contain package load errors")
	}
	var pkgErr packages.Error
	if !errors.As(err, &pkgErr) {
		t.Fatalf("expected error chain to contain packages.Error, got %T: %v", err, err)
	}
	if got := err.Error(); !strings.Contains(got, "example.com/does/not/exist") || !strings.Contains(got, "undefined: missing") {
		t.Fatalf("expected specific package load errors, got: %v", err)
	}
}
