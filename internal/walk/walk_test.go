package walk

import (
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
)

func TestIsDirUsesStatForUnreadableDirectory(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("permission bits do not reliably make directories unreadable on Windows")
	}

	dir := filepath.Join(t.TempDir(), "unreadable")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("failed to create directory fixture: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0o755); err != nil {
			t.Fatalf("failed to restore directory permissions: %v", err)
		}
	})
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatalf("failed to make directory unreadable: %v", err)
	}

	gotPath, ok := IsDir(dir)
	if !ok {
		t.Fatalf("expected unreadable directory to still be identified via stat")
	}
	if gotPath != dir {
		t.Fatalf("path mismatch: got %q want %q", gotPath, dir)
	}
}

func TestNewSubmitterWaitsForCreatedPoolTasks(t *testing.T) {
	t.Parallel()

	submit, wait := NewSubmitter(nil, 1)

	var completed atomic.Int32
	const taskCount = 32
	for range taskCount {
		submit(func() {
			completed.Add(1)
		})
	}
	wait()

	if got := completed.Load(); got != taskCount {
		t.Fatalf("wait returned before all tasks completed: got %d want %d", got, taskCount)
	}
}
