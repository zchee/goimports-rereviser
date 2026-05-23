package reviser

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestEnsureCacheDirUsesPrivatePermissions(t *testing.T) {
	cacheDir := filepath.Join(t.TempDir(), "cache")
	if err := EnsureCacheDir(cacheDir); err != nil {
		t.Fatalf("EnsureCacheDir returned error: %v", err)
	}

	info, err := os.Stat(cacheDir)
	if err != nil {
		t.Fatalf("stat cache dir: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected cache path to be a directory")
	}
	if runtime.GOOS == "windows" {
		return
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o700); got != want {
		t.Fatalf("cache dir permissions mismatch: got %v want %v", got, want)
	}

	if err := os.Chmod(cacheDir, 0o755); err != nil {
		t.Fatalf("broaden cache dir permissions: %v", err)
	}
	if err := EnsureCacheDir(cacheDir); err != nil {
		t.Fatalf("EnsureCacheDir on existing dir returned error: %v", err)
	}
	info, err = os.Stat(cacheDir)
	if err != nil {
		t.Fatalf("stat tightened cache dir: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o700); got != want {
		t.Fatalf("existing cache dir permissions mismatch: got %v want %v", got, want)
	}
}

func TestEnsureCacheDirRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires different privileges on Windows")
	}

	root := t.TempDir()
	target := filepath.Join(root, "target")
	cacheLink := filepath.Join(root, "cache-link")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.Symlink(target, cacheLink); err != nil {
		t.Fatalf("symlink cache dir: %v", err)
	}

	err := EnsureCacheDir(cacheLink)
	if err == nil {
		t.Fatalf("expected symlinked cache dir to be rejected")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got: %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o755); got != want {
		t.Fatalf("target mode changed through symlink: got %v want %v", got, want)
	}
}

func TestWriteCacheEntryUsesPrivateAtomicFile(t *testing.T) {
	workDir := t.TempDir()
	cacheDir := filepath.Join(t.TempDir(), "cache")

	filePath := filepath.Join(workDir, "main.go")
	content := []byte("package main\n")
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	entry, err := NewCacheEntryWithFingerprint(filePath, ComputeContentHash(content), true, "fmt-default")
	if err != nil {
		t.Fatalf("build cache entry: %v", err)
	}
	if err := WriteCacheEntry(cacheDir, filePath, entry); err != nil {
		t.Fatalf("write cache entry: %v", err)
	}

	cacheDirInfo, err := os.Stat(cacheDir)
	if err != nil {
		t.Fatalf("stat cache dir: %v", err)
	}
	if !cacheDirInfo.IsDir() {
		t.Fatalf("expected cache path to be a directory")
	}
	if runtime.GOOS != "windows" {
		if got, want := cacheDirInfo.Mode().Perm(), os.FileMode(0o700); got != want {
			t.Fatalf("cache dir permissions mismatch: got %v want %v", got, want)
		}
	}

	cacheFile := CacheFilePath(cacheDir, filePath)
	info, err := os.Stat(cacheFile)
	if err != nil {
		t.Fatalf("stat cache file: %v", err)
	}
	if runtime.GOOS != "windows" {
		if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
			t.Fatalf("cache file permissions mismatch: got %v want %v", got, want)
		}
	}

	readEntry, err := ReadCacheEntry(cacheDir, filePath)
	if err != nil {
		t.Fatalf("read cache entry: %v", err)
	}
	if readEntry == nil || readEntry.Hash != entry.Hash || readEntry.Fingerprint != entry.Fingerprint {
		t.Fatalf("cache entry mismatch: got %+v want %+v", readEntry, entry)
	}

	temps, err := filepath.Glob(filepath.Join(cacheDir, cacheTempPattern))
	if err != nil {
		t.Fatalf("glob cache temp files: %v", err)
	}
	if len(temps) != 0 {
		t.Fatalf("expected atomic write to clean temp files, got %v", temps)
	}
}

func TestShouldSkipByMetadata(t *testing.T) {
	tests := map[string]struct {
		setup    func(t *testing.T, cacheDir, filePath string)
		wantSkip bool
	}{
		"metadata match skips": {
			setup: func(t *testing.T, cacheDir, filePath string) {
				t.Helper()
				content := []byte("package main\n")
				if err := os.WriteFile(filePath, content, 0o644); err != nil {
					t.Fatalf("write file: %v", err)
				}
				entry, err := NewCacheEntry(filePath, ComputeContentHash(content), true)
				if err != nil {
					t.Fatalf("build cache entry: %v", err)
				}
				if err := WriteCacheEntry(cacheDir, filePath, entry); err != nil {
					t.Fatalf("write cache entry: %v", err)
				}
			},
			wantSkip: true,
		},
		"metadata mismatch requires processing": {
			setup: func(t *testing.T, cacheDir, filePath string) {
				t.Helper()
				initial := []byte("package main\n")
				if err := os.WriteFile(filePath, initial, 0o644); err != nil {
					t.Fatalf("write file: %v", err)
				}
				entry, err := NewCacheEntry(filePath, ComputeContentHash(initial), true)
				if err != nil {
					t.Fatalf("build cache entry: %v", err)
				}
				if err := WriteCacheEntry(cacheDir, filePath, entry); err != nil {
					t.Fatalf("write cache entry: %v", err)
				}
				time.Sleep(5 * time.Millisecond)
				updated := []byte("package main\nconst x = 1\n")
				if err := os.WriteFile(filePath, updated, 0o644); err != nil {
					t.Fatalf("modify file: %v", err)
				}
			},
			wantSkip: false,
		},
		"legacy hash fallback": {
			setup: func(t *testing.T, cacheDir, filePath string) {
				t.Helper()
				content := []byte("package legacy\n")
				if err := os.WriteFile(filePath, content, 0o644); err != nil {
					t.Fatalf("write file: %v", err)
				}
				legacyCache := CacheFilePath(cacheDir, filePath)
				if err := os.WriteFile(legacyCache, []byte(ComputeContentHash(content)), 0o644); err != nil {
					t.Fatalf("write legacy cache: %v", err)
				}
			},
			wantSkip: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			workDir := t.TempDir()
			cacheDir := t.TempDir()
			filePath := filepath.Join(workDir, "main.go")

			tt.setup(t, cacheDir, filePath)

			skip, err := ShouldSkipByMetadata(cacheDir, filePath)
			if err != nil {
				t.Fatalf("ShouldSkipByMetadata: %v", err)
			}
			if skip != tt.wantSkip {
				t.Fatalf("ShouldSkipByMetadata = %v, want %v", skip, tt.wantSkip)
			}
		})
	}
}

func TestShouldSkipWithFingerprint(t *testing.T) {
	workDir := t.TempDir()
	cacheDir := t.TempDir()

	filePath := filepath.Join(workDir, "main.go")
	content := []byte("package main\n")
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	hash := ComputeContentHash(content)
	entry, err := NewCacheEntryWithFingerprint(filePath, hash, true, "fmt-default")
	if err != nil {
		t.Fatalf("build fingerprinted cache entry: %v", err)
	}
	if err := WriteCacheEntry(cacheDir, filePath, entry); err != nil {
		t.Fatalf("write cache entry: %v", err)
	}

	tests := map[string]struct {
		fingerprint string
		wantSkip    bool
	}{
		"matching fingerprint allows skip": {
			fingerprint: "fmt-default",
			wantSkip:    true,
		},
		"mismatched fingerprint forces processing": {
			fingerprint: "separate-named",
			wantSkip:    false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			skip, err := ShouldSkipWithFingerprint(cacheDir, filePath, true, tt.fingerprint)
			if err != nil {
				t.Fatalf("ShouldSkipWithFingerprint(%q): %v", tt.fingerprint, err)
			}
			if skip != tt.wantSkip {
				t.Fatalf("ShouldSkipWithFingerprint(%q) = %v, want %v", tt.fingerprint, skip, tt.wantSkip)
			}
		})
	}
}

func TestLegacyCacheEntryDoesNotMatchNonEmptyFingerprint(t *testing.T) {
	workDir := t.TempDir()
	cacheDir := t.TempDir()

	filePath := filepath.Join(workDir, "legacy.go")
	content := []byte("package legacy\n")
	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	legacyHash := ComputeContentHash(content)
	legacyCache := CacheFilePath(cacheDir, filePath)
	if err := os.WriteFile(legacyCache, []byte(legacyHash), 0o644); err != nil {
		t.Fatalf("write legacy cache: %v", err)
	}

	tests := map[string]struct {
		fingerprint string
		wantSkip    bool
	}{
		"non-empty fingerprint misses legacy cache": {
			fingerprint: "fmt-default",
			wantSkip:    false,
		},
		"empty fingerprint preserves legacy behavior": {
			fingerprint: "",
			wantSkip:    true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			skip, err := ShouldSkipWithFingerprint(cacheDir, filePath, true, tt.fingerprint)
			if err != nil {
				t.Fatalf("ShouldSkipWithFingerprint(%q): %v", tt.fingerprint, err)
			}
			if skip != tt.wantSkip {
				t.Fatalf("ShouldSkipWithFingerprint(%q) = %v, want %v", tt.fingerprint, skip, tt.wantSkip)
			}
		})
	}
}

func TestComputeContentHash_CollisionCheck(t *testing.T) {
	samples := make([][]byte, 0, 256)

	for i := range 64 {
		samples = append(samples, fmt.Appendf(nil, "sample-%02d", i))
	}
	for i := range 16 {
		samples = append(samples, fmt.Appendf(nil, "package p%02d\n\nfunc f() {\n\tprintln(%d)\n}\n", i, i))
	}

	md5Seen := make(map[string]int, len(samples))
	xxh3Seen := make(map[string]int, len(samples))

	for idx, sample := range samples {
		md5Sum := md5.Sum(sample)
		md5Hash := hex.EncodeToString(md5Sum[:])
		if prev, ok := md5Seen[md5Hash]; ok {
			t.Fatalf("md5 collision between sample %d and %d", prev, idx)
		}
		md5Seen[md5Hash] = idx

		xxh3Hash := ComputeContentHash(sample)
		if prev, ok := xxh3Seen[xxh3Hash]; ok {
			t.Fatalf("xxh3 collision between sample %d and %d", prev, idx)
		}
		xxh3Seen[xxh3Hash] = idx
	}

	if len(md5Seen) != len(samples) {
		t.Fatalf("expected unique md5 hashes, got %d/%d", len(md5Seen), len(samples))
	}
	if len(xxh3Seen) != len(samples) {
		t.Fatalf("expected unique xxh3 hashes, got %d/%d", len(xxh3Seen), len(samples))
	}
}
