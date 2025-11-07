package reviser

import (
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShouldSkipByMetadata(t *testing.T) {
	t.Run("metadata match skips", func(t *testing.T) {
		workDir := t.TempDir()
		cacheDir := t.TempDir()

		filePath := filepath.Join(workDir, "main.go")
		content := []byte("package main\n")
		if err := os.WriteFile(filePath, content, 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		sum := md5.Sum(content)
		hash := hex.EncodeToString(sum[:])
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

		sum := md5.Sum(initial)
		hash := hex.EncodeToString(sum[:])
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

		sum := md5.Sum(content)
		hash := hex.EncodeToString(sum[:])
		legacyCache := CacheFilePath(cacheDir, filePath)
		if err := os.WriteFile(legacyCache, []byte(hash), 0o644); err != nil {
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
