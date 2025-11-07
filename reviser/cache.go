package reviser

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zeebo/xxh3"
)

// CacheEntry represents the cached state of a file.
// Hash is always recorded; Size and ModTime are optional and only persisted
// when metadata-aware caching is enabled.
type CacheEntry struct {
	Hash    string `json:"hash"`
	Size    int64  `json:"size,omitempty"`
	ModTime int64  `json:"mod_time,omitempty"`
}

func cacheFilePath(cacheDir, absPath string) string {
	sum := hashPath(absPath)
	return filepath.Join(cacheDir, sum)
}

func encodeHash(sum uint64) string {
	return fmt.Sprintf("%016x", sum)
}

// ComputeContentHash returns the deterministic digest for arbitrary content using xxh3.
func ComputeContentHash(data []byte) string {
	return encodeHash(xxh3.Hash(data))
}

// hashPath returns the xxh3 digest of the provided absolute path. Directory and
// single-file flows share this helper to guarantee consistent cache keying.
func hashPath(absPath string) string {
	return encodeHash(xxh3.HashString(absPath))
}

// readCacheEntry loads the cache entry for absPath. It returns (nil, ErrNotExist)
// when no cache is recorded. Legacy cache files that store only the hash are
// transparently upgraded to CacheEntry instances with metadata left zeroed.
func readCacheEntry(cacheDir, absPath string) (*CacheEntry, error) {
	cacheFile := cacheFilePath(cacheDir, absPath)
	b, err := os.ReadFile(cacheFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fs.ErrNotExist
		}
		return nil, err
	}
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return nil, nil
	}
	if b[0] != '{' {
		return &CacheEntry{Hash: string(b)}, nil
	}
	var entry CacheEntry
	if err := json.Unmarshal(b, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// writeCacheEntry persists the given entry. When Size/ModTime are zero the
// entry is stored in the legacy hash-only format to stay backward compatible
// and minimize file size.
func writeCacheEntry(cacheDir, absPath string, entry CacheEntry) error {
	cacheFile := cacheFilePath(cacheDir, absPath)
	if entry.Size == 0 && entry.ModTime == 0 {
		return os.WriteFile(cacheFile, []byte(entry.Hash), 0o644)
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return os.WriteFile(cacheFile, payload, 0o644)
}

func fileMetadata(path string) (size int64, modTime int64, err error) {
	info, statErr := os.Stat(path)
	if statErr != nil {
		return 0, 0, statErr
	}
	return info.Size(), info.ModTime().UTC().UnixNano(), nil
}

// metadataMatches compares cached metadata with the current file info.
func metadataMatches(entry *CacheEntry, size int64, modTime int64) bool {
	if entry == nil {
		return false
	}
	if entry.Size == 0 || entry.ModTime == 0 {
		return false
	}
	return entry.Size == size && entry.ModTime == modTime
}

// cacheEntryForMetadata returns a populated CacheEntry when metadata could be
// collected, otherwise falls back to a hash-only entry.
func cacheEntryForMetadata(hash string, size int64, modTime int64) CacheEntry {
	if size == 0 || modTime == 0 {
		return CacheEntry{Hash: hash}
	}
	return CacheEntry{Hash: hash, Size: size, ModTime: modTime}
}

// NewCacheEntry builds the cache entry for the given path and hash. When
// withMetadata is true it attempts to capture size and modification time to
// enable metadata-based skipping. If metadata retrieval fails with fs.ErrNotExist
// a hash-only entry is returned.
func NewCacheEntry(absPath, hash string, withMetadata bool) (CacheEntry, error) {
	if !withMetadata {
		return CacheEntry{Hash: hash}, nil
	}
	size, modTime, err := fileMetadata(absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return CacheEntry{Hash: hash}, nil
		}
		return CacheEntry{}, err
	}
	return cacheEntryForMetadata(hash, size, modTime), nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := xxh3.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return encodeHash(h.Sum64()), nil
}

// ShouldSkipByHash verifies content hash equality, matching the legacy cache
// behavior that reads the full file to confirm no changes.
func ShouldSkipByHash(cacheDir, absPath string) (bool, error) {
	entry, err := readCacheEntry(cacheDir, absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if entry == nil || entry.Hash == "" {
		return false, nil
	}
	currentHash, err := hashFile(absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			_ = os.Remove(cacheFilePath(cacheDir, absPath))
			return true, nil
		}
		return false, err
	}
	return entry.Hash == currentHash, nil
}

// ShouldSkipByMetadata relies on file size/modtime to avoid reading the file.
// It falls back to hash verification when metadata is not available in the cache
// entry and removes stale cache files when the source file has been deleted.
func ShouldSkipByMetadata(cacheDir, absPath string) (bool, error) {
	entry, err := readCacheEntry(cacheDir, absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if entry == nil {
		return false, nil
	}
	if entry.Size == 0 || entry.ModTime == 0 {
		return ShouldSkipByHash(cacheDir, absPath)
	}
	size, modTime, statErr := fileMetadata(absPath)
	if statErr != nil {
		if errors.Is(statErr, fs.ErrNotExist) {
			_ = os.Remove(cacheFilePath(cacheDir, absPath))
			return true, nil
		}
		return false, statErr
	}
	if metadataMatches(entry, size, modTime) {
		return true, nil
	}
	return false, nil
}

// WriteCacheEntry persists the given entry using either a metadata-aware or
// legacy format based on the presence of Size/ModTime.
func WriteCacheEntry(cacheDir, absPath string, entry CacheEntry) error {
	return writeCacheEntry(cacheDir, absPath, entry)
}

// ReadCacheEntry returns the stored cache entry, translating fs.ErrNotExist to
// a nil entry with no error to simplify callers.
func ReadCacheEntry(cacheDir, absPath string) (*CacheEntry, error) {
	entry, err := readCacheEntry(cacheDir, absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return entry, nil
}

// CacheFilePath exposes the deterministic cache file resolution used by both
// single-file and directory workflows.
func CacheFilePath(cacheDir, absPath string) string {
	return cacheFilePath(cacheDir, absPath)
}

// ShouldSkip routes to the preferred strategy (metadata-first by default) and
// falls back to hash-based verification when metadata is unavailable.
func ShouldSkip(cacheDir, absPath string, preferMetadata bool) (bool, error) {
	if cacheDir == "" {
		return false, nil
	}
	if preferMetadata {
		return ShouldSkipByMetadata(cacheDir, absPath)
	}
	return ShouldSkipByHash(cacheDir, absPath)
}
