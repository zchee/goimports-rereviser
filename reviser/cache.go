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

const (
	cacheDirPerm     fs.FileMode = 0o700
	cacheFilePerm    fs.FileMode = 0o600
	cacheTempPattern             = ".goimports-rereviser-*"
)

// CacheEntry represents the cached state of a file.
// Hash is always recorded; Size and ModTime are optional and only persisted
// when metadata-aware caching is enabled.
type CacheEntry struct {
	Hash        string `json:"hash"`
	Size        int64  `json:"size,omitempty"`
	ModTime     int64  `json:"mod_time,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
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

// EnsureCacheDir creates the cache directory with private permissions and
// tightens an existing directory when the platform supports permission bits.
func EnsureCacheDir(cacheDir string) error {
	if cacheDir == "" {
		return nil
	}
	if err := os.MkdirAll(cacheDir, cacheDirPerm); err != nil {
		return err
	}
	info, err := os.Lstat(cacheDir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("cache path must not be a symlink: %s", cacheDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("cache path is not a directory: %s", cacheDir)
	}
	if err := os.Chmod(cacheDir, cacheDirPerm); err != nil {
		return err
	}
	info, err = os.Lstat(cacheDir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("cache path became a symlink: %s", cacheDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("cache path is not a directory: %s", cacheDir)
	}
	return nil
}

func writeFileAtomic(path string, payload []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, cacheTempPattern)
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(cacheFilePerm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// writeCacheEntry persists the given entry. When Size/ModTime are zero the
// entry is stored in the legacy hash-only format to stay backward compatible
// and minimize file size.
func writeCacheEntry(cacheDir, absPath string, entry CacheEntry) error {
	if cacheDir == "" {
		return nil
	}
	if err := EnsureCacheDir(cacheDir); err != nil {
		return err
	}
	cacheFile := cacheFilePath(cacheDir, absPath)
	if entry.Size == 0 && entry.ModTime == 0 && entry.Fingerprint == "" {
		return writeFileAtomic(cacheFile, []byte(entry.Hash))
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	return writeFileAtomic(cacheFile, payload)
}

func fileMetadata(path string) (size, modTime int64, err error) {
	info, statErr := os.Stat(path)
	if statErr != nil {
		return 0, 0, statErr
	}
	return info.Size(), info.ModTime().UTC().UnixNano(), nil
}

// metadataMatches compares cached metadata with the current file info.
func metadataMatches(entry *CacheEntry, size, modTime int64) bool {
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
func cacheEntryForMetadata(hash string, size, modTime int64, fingerprint string) CacheEntry {
	if size == 0 || modTime == 0 {
		return CacheEntry{Hash: hash, Fingerprint: fingerprint}
	}
	return CacheEntry{Hash: hash, Size: size, ModTime: modTime, Fingerprint: fingerprint}
}

// NewCacheEntry builds the cache entry for the given path and hash. When
// withMetadata is true it attempts to capture size and modification time to
// enable metadata-based skipping. If metadata retrieval fails with fs.ErrNotExist
// a hash-only entry is returned.
func NewCacheEntry(absPath, hash string, withMetadata bool) (CacheEntry, error) {
	return NewCacheEntryWithFingerprint(absPath, hash, withMetadata, "")
}

// NewCacheEntryWithFingerprint builds a cache entry tied to the formatter
// configuration that produced it. A non-empty fingerprint must match during
// skip checks before hash or metadata equality can skip processing.
func NewCacheEntryWithFingerprint(absPath, hash string, withMetadata bool, fingerprint string) (CacheEntry, error) {
	if !withMetadata {
		return CacheEntry{Hash: hash, Fingerprint: fingerprint}, nil
	}
	size, modTime, err := fileMetadata(absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return CacheEntry{Hash: hash, Fingerprint: fingerprint}, nil
		}
		return CacheEntry{}, err
	}
	return cacheEntryForMetadata(hash, size, modTime, fingerprint), nil
}

func cacheFingerprintMatches(entry *CacheEntry, fingerprint string) bool {
	if fingerprint == "" {
		return true
	}
	return entry != nil && entry.Fingerprint == fingerprint
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
	return ShouldSkipByHashWithFingerprint(cacheDir, absPath, "")
}

// ShouldSkipByHashWithFingerprint verifies content hash equality only when the
// cached formatter fingerprint matches the requested fingerprint.
func ShouldSkipByHashWithFingerprint(cacheDir, absPath, fingerprint string) (bool, error) {
	entry, err := readCacheEntry(cacheDir, absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if entry == nil || entry.Hash == "" || !cacheFingerprintMatches(entry, fingerprint) {
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
	return ShouldSkipByMetadataWithFingerprint(cacheDir, absPath, "")
}

// ShouldSkipByMetadataWithFingerprint uses metadata only when the cached
// formatter fingerprint matches the requested fingerprint.
func ShouldSkipByMetadataWithFingerprint(cacheDir, absPath, fingerprint string) (bool, error) {
	entry, err := readCacheEntry(cacheDir, absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if entry == nil || !cacheFingerprintMatches(entry, fingerprint) {
		return false, nil
	}
	if entry.Size == 0 || entry.ModTime == 0 {
		return ShouldSkipByHashWithFingerprint(cacheDir, absPath, fingerprint)
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
	return ShouldSkipWithFingerprint(cacheDir, absPath, preferMetadata, "")
}

// ShouldSkipWithFingerprint routes to the preferred strategy and rejects cache
// hits produced by a different formatter configuration.
func ShouldSkipWithFingerprint(cacheDir, absPath string, preferMetadata bool, fingerprint string) (bool, error) {
	if cacheDir == "" {
		return false, nil
	}
	if preferMetadata {
		return ShouldSkipByMetadataWithFingerprint(cacheDir, absPath, fingerprint)
	}
	return ShouldSkipByHashWithFingerprint(cacheDir, absPath, fingerprint)
}
