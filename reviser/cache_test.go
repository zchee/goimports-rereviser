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
	t.Run("metadata match skips", func(t *testing.T) {
		workDir := t.TempDir()
		cacheDir := t.TempDir()

		filePath := filepath.Join(workDir, "main.go")
		content := []byte("package main\n")
		if err := os.WriteFile(filePath, content, 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		hash := ComputeContentHash(content)
		entry, err := NewCacheEntry(filePath, hash, true)
		if err != nil {
			t.Fatalf("build cache entry: %v", err)
		}
		if err := WriteCacheEntry(cacheDir, filePath, entry); err != nil {
			t.Fatalf("write cache entry: %v", err)
		}

		skip, err := ShouldSkipByMetadata(cacheDir, filePath)
		if err != nil {
			t.Fatalf("should skip metadata: %v", err)
		}
		if !skip {
			t.Fatalf("expected metadata shortcut to skip processing")
		}
	})

	t.Run("metadata mismatch requires processing", func(t *testing.T) {
		workDir := t.TempDir()
		cacheDir := t.TempDir()

		filePath := filepath.Join(workDir, "main.go")
		initial := []byte("package main\n")
		if err := os.WriteFile(filePath, initial, 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		hash := ComputeContentHash(initial)
		entry, err := NewCacheEntry(filePath, hash, true)
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

		skip, err := ShouldSkipByMetadata(cacheDir, filePath)
		if err != nil {
			t.Fatalf("should skip metadata: %v", err)
		}
		if skip {
			t.Fatalf("expected metadata mismatch to force processing")
		}
	})

	t.Run("legacy hash fallback", func(t *testing.T) {
		workDir := t.TempDir()
		cacheDir := t.TempDir()

		filePath := filepath.Join(workDir, "main.go")
		content := []byte("package legacy\n")
		if err := os.WriteFile(filePath, content, 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		legacyHash := ComputeContentHash(content)
		legacyCache := CacheFilePath(cacheDir, filePath)
		if err := os.WriteFile(legacyCache, []byte(legacyHash), 0o644); err != nil {
			t.Fatalf("write legacy cache: %v", err)
		}

		skip, err := ShouldSkipByMetadata(cacheDir, filePath)
		if err != nil {
			t.Fatalf("should skip metadata: %v", err)
		}
		if !skip {
			t.Fatalf("expected legacy cache to fall back to hash verification")
		}
	})
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

	skip, err := ShouldSkipWithFingerprint(cacheDir, filePath, true, "fmt-default")
	if err != nil {
		t.Fatalf("matching fingerprint skip check: %v", err)
	}
	if !skip {
		t.Fatalf("expected matching formatter fingerprint to allow cache skip")
	}

	skip, err = ShouldSkipWithFingerprint(cacheDir, filePath, true, "separate-named")
	if err != nil {
		t.Fatalf("mismatched fingerprint skip check: %v", err)
	}
	if skip {
		t.Fatalf("expected mismatched formatter fingerprint to force processing")
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

	skip, err := ShouldSkipWithFingerprint(cacheDir, filePath, true, "fmt-default")
	if err != nil {
		t.Fatalf("fingerprinted skip check on legacy cache: %v", err)
	}
	if skip {
		t.Fatalf("expected legacy cache without formatter fingerprint to miss non-empty fingerprint")
	}

	skip, err = ShouldSkipWithFingerprint(cacheDir, filePath, true, "")
	if err != nil {
		t.Fatalf("legacy-compatible skip check: %v", err)
	}
	if !skip {
		t.Fatalf("expected empty fingerprint to preserve legacy cache behavior")
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
