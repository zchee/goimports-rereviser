package reviser

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
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

func TestComputeContentHash_CollisionCheck(t *testing.T) {
	samples := make([][]byte, 0, 256)

	for i := 0; i < 64; i++ {
		samples = append(samples, []byte(fmt.Sprintf("sample-%02d", i)))
	}
	for i := 0; i < 16; i++ {
		samples = append(samples, []byte(fmt.Sprintf("package p%02d\n\nfunc f() {\n\tprintln(%d)\n}\n", i, i)))
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
