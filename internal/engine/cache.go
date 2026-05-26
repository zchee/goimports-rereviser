package engine

import (
	"github.com/zchee/goimports-rereviser/v4/internal/cache"
)

const cacheTempPattern = ".goimports-rereviser-*"

// CacheEntry represents the cached state of a file.
// Hash is always recorded; Size and ModTime are optional and only persisted
// when metadata-aware caching is enabled.
type CacheEntry struct {
	Hash        string `json:"hash"`
	Size        int64  `json:"size,omitempty"`
	ModTime     int64  `json:"mod_time,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

func toInternalCacheEntry(entry CacheEntry) cache.CacheEntry {
	return cache.CacheEntry{
		Hash:        entry.Hash,
		Size:        entry.Size,
		ModTime:     entry.ModTime,
		Fingerprint: entry.Fingerprint,
	}
}

func fromInternalCacheEntry(entry cache.CacheEntry) CacheEntry {
	return CacheEntry{
		Hash:        entry.Hash,
		Size:        entry.Size,
		ModTime:     entry.ModTime,
		Fingerprint: entry.Fingerprint,
	}
}

// ComputeContentHash returns the deterministic digest for arbitrary content using xxh3.
func ComputeContentHash(data []byte) string {
	return cache.ComputeContentHash(data)
}

// EnsureCacheDir creates the cache directory with private permissions and
// tightens an existing directory when the platform supports permission bits.
//
// The MkdirAll -> Lstat -> Chmod -> Lstat sequence intentionally accepts a
// small TOCTOU window between the post-MkdirAll Lstat and the Chmod. The
// threat model here is a single-user developer tool writing into its own XDG
// cache dir, not a multi-tenant adversarial filesystem; an attacker able to
// race the cache path already controls the user's environment. We pay the
// double Lstat to catch the common case (the dir was replaced with a symlink
// by an unrelated process) without locking the cache directory.
func EnsureCacheDir(cacheDir string) error {
	return cache.EnsureCacheDir(cacheDir)
}

// NewCacheEntry builds the cache entry for the given path and hash. When
// withMetadata is true it attempts to capture size and modification time to
// enable metadata-based skipping. If metadata retrieval fails with fs.ErrNotExist
// a hash-only entry is returned.
func NewCacheEntry(absPath, hash string, withMetadata bool) (CacheEntry, error) {
	entry, err := cache.NewCacheEntry(absPath, hash, withMetadata)
	if err != nil {
		return CacheEntry{}, err
	}
	return fromInternalCacheEntry(entry), nil
}

// NewCacheEntryWithFingerprint builds a cache entry tied to the formatter
// configuration that produced it. A non-empty fingerprint must match during
// skip checks before hash or metadata equality can skip processing.
func NewCacheEntryWithFingerprint(absPath, hash string, withMetadata bool, fingerprint string) (CacheEntry, error) {
	entry, err := cache.NewCacheEntryWithFingerprint(absPath, hash, withMetadata, fingerprint)
	if err != nil {
		return CacheEntry{}, err
	}
	return fromInternalCacheEntry(entry), nil
}

// ShouldSkipByHash verifies content hash equality, matching the legacy cache
// behavior that reads the full file to confirm no changes.
func ShouldSkipByHash(cacheDir, absPath string) (bool, error) {
	return cache.ShouldSkipByHash(cacheDir, absPath)
}

// ShouldSkipByHashWithFingerprint verifies content hash equality only when the
// cached formatter fingerprint matches the requested fingerprint.
func ShouldSkipByHashWithFingerprint(cacheDir, absPath, fingerprint string) (bool, error) {
	return cache.ShouldSkipByHashWithFingerprint(cacheDir, absPath, fingerprint)
}

// ShouldSkipByMetadata relies on file size/modtime to avoid reading the file.
// It falls back to hash verification when metadata is not available in the cache
// entry and removes stale cache files when the source file has been deleted.
func ShouldSkipByMetadata(cacheDir, absPath string) (bool, error) {
	return cache.ShouldSkipByMetadata(cacheDir, absPath)
}

// ShouldSkipByMetadataWithFingerprint uses metadata only when the cached
// formatter fingerprint matches the requested fingerprint.
func ShouldSkipByMetadataWithFingerprint(cacheDir, absPath, fingerprint string) (bool, error) {
	return cache.ShouldSkipByMetadataWithFingerprint(cacheDir, absPath, fingerprint)
}

// WriteCacheEntry persists the given entry using either a metadata-aware or
// legacy format based on the presence of Size/ModTime.
func WriteCacheEntry(cacheDir, absPath string, entry CacheEntry) error {
	return cache.WriteCacheEntry(cacheDir, absPath, toInternalCacheEntry(entry))
}

// ReadCacheEntry returns the stored cache entry, translating fs.ErrNotExist to
// a nil entry with no error to simplify callers.
func ReadCacheEntry(cacheDir, absPath string) (*CacheEntry, error) {
	entry, err := cache.ReadCacheEntry(cacheDir, absPath)
	if err != nil || entry == nil {
		return nil, err
	}
	publicEntry := fromInternalCacheEntry(*entry)
	return &publicEntry, nil
}

// CacheFilePath exposes the deterministic cache file resolution used by both
// single-file and directory workflows.
func CacheFilePath(cacheDir, absPath string) string {
	return cache.CacheFilePath(cacheDir, absPath)
}

// ShouldSkip routes to the preferred strategy (metadata-first by default) and
// falls back to hash-based verification when metadata is unavailable.
func ShouldSkip(cacheDir, absPath string, preferMetadata bool) (bool, error) {
	return cache.ShouldSkip(cacheDir, absPath, preferMetadata)
}

// ShouldSkipWithFingerprint routes to the preferred strategy and rejects cache
// hits produced by a different formatter configuration.
func ShouldSkipWithFingerprint(cacheDir, absPath string, preferMetadata bool, fingerprint string) (bool, error) {
	return cache.ShouldSkipWithFingerprint(cacheDir, absPath, preferMetadata, fingerprint)
}
