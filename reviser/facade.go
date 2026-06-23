package reviser

import (
	internalengine "github.com/zchee/goimports-rereviser/v4/internal/engine"
)

const (
	// StandardInput identifies stdin as the source file path.
	StandardInput = internalengine.StandardInput

	// StdImportsOrder is std libs, e.g. fmt, errors, strings...
	StdImportsOrder = internalengine.StdImportsOrder
	// CompanyImportsOrder is packages that belong to the same organization.
	CompanyImportsOrder = internalengine.CompanyImportsOrder
	// ProjectImportsOrder is packages that are inside the current project.
	ProjectImportsOrder = internalengine.ProjectImportsOrder
	// GeneralImportsOrder is packages that are outside. In other words it is
	// general purpose libraries.
	GeneralImportsOrder = internalengine.GeneralImportsOrder
	// BlankedImportsOrder is accepted for compatibility and ignored during
	// grouping; blank imports are grouped by package path.
	BlankedImportsOrder = internalengine.BlankedImportsOrder
	// NonBlankImportsOrder is accepted as an explicit no-op; non-blank
	// imports are grouped by package path through the standard categories.
	NonBlankImportsOrder = internalengine.NonBlankImportsOrder
	// DottedImportsOrder is separate group for "." imports.
	DottedImportsOrder = internalengine.DottedImportsOrder
)

// ErrPathIsNotDir is returned when SourceDir is configured with a non-directory path.
var ErrPathIsNotDir = internalengine.ErrPathIsNotDir

type (
	// SourceFile main struct for fixing an existing code.
	SourceFile = internalengine.SourceFile
	// SourceFileOption is an int alias for options.
	SourceFileOption = internalengine.SourceFileOption
	// SourceFileOptions is a slice of executing options.
	SourceFileOptions = internalengine.SourceFileOptions
	// ImportsOrder represents the name of import order.
	ImportsOrder = internalengine.ImportsOrder
	// ImportsOrders alias to []ImportsOrder.
	ImportsOrders = internalengine.ImportsOrders
	// SourceDir validates and fixes imports under a directory.
	SourceDir = internalengine.SourceDir
	// UnformattedCollection is a collection of paths that require formatting.
	UnformattedCollection = internalengine.UnformattedCollection
	// CacheEntry represents the cached state of a file.
	// Hash is always recorded; Size and ModTime are optional and only persisted
	// when metadata-aware caching is enabled.
	CacheEntry = internalengine.CacheEntry
)

// NewSourceFile constructor.
func NewSourceFile(projectName, filePath string) *SourceFile {
	return internalengine.NewSourceFile(projectName, filePath)
}

// WithRemovingUnusedImports is an option to remove unused imports.
func WithRemovingUnusedImports(f *SourceFile) error {
	return internalengine.WithRemovingUnusedImports(f)
}

// WithUsingAliasForVersionSuffix is an option to set explicit package name in imports.
func WithUsingAliasForVersionSuffix(f *SourceFile) error {
	return internalengine.WithUsingAliasForVersionSuffix(f)
}

// WithCodeFormatting use to format the code.
func WithCodeFormatting(f *SourceFile) error {
	return internalengine.WithCodeFormatting(f)
}

// WithCompanyPackagePrefixes option for 3d group(by default), like inter-org
// or company package prefixes.
func WithCompanyPackagePrefixes(s string) SourceFileOption {
	return internalengine.WithCompanyPackagePrefixes(s)
}

// WithImportsOrder will sort by needed order. Default order is
// "std,general,company,project".
func WithImportsOrder(orders []ImportsOrder) SourceFileOption {
	return internalengine.WithImportsOrder(orders)
}

// WithSkipGeneratedFile will skip formatting and imports sorting for
// auto-generated files.
func WithSkipGeneratedFile(f *SourceFile) error {
	return internalengine.WithSkipGeneratedFile(f)
}

// WithSeparatedNamedImports separates named imports from unnamed imports per group.
func WithSeparatedNamedImports(f *SourceFile) error {
	return internalengine.WithSeparatedNamedImports(f)
}

// StringToImportsOrders converts a comma-separated import-order string into
// ImportsOrders. Default value for empty string is
// "std,general,company,project".
func StringToImportsOrders(s string) (ImportsOrders, error) {
	return internalengine.StringToImportsOrders(s)
}

// NewSourceDir constructor.
func NewSourceDir(projectName, path string, isRecursive bool, excludes string) *SourceDir {
	return internalengine.NewSourceDir(projectName, path, isRecursive, excludes)
}

// IsDir reports whether path is a directory and returns the normalized path.
func IsDir(path string) (string, bool) {
	return internalengine.IsDir(path)
}

// ComputeContentHash returns the deterministic digest for arbitrary content using xxh3.
func ComputeContentHash(data []byte) string {
	return internalengine.ComputeContentHash(data)
}

// EnsureCacheDir creates the cache directory with private permissions and
// tightens an existing directory when the platform supports permission bits.
func EnsureCacheDir(cacheDir string) error {
	return internalengine.EnsureCacheDir(cacheDir)
}

// NewCacheEntry builds the cache entry for the given path and hash. When
// withMetadata is true it attempts to capture size and modification time to
// enable metadata-based skipping. If metadata retrieval fails with fs.ErrNotExist
// a hash-only entry is returned.
func NewCacheEntry(absPath, hash string, withMetadata bool) (CacheEntry, error) {
	return internalengine.NewCacheEntry(absPath, hash, withMetadata)
}

// NewCacheEntryWithFingerprint builds a cache entry tied to the formatter
// configuration that produced it. A non-empty fingerprint must match during
// skip checks before hash or metadata equality can skip processing.
func NewCacheEntryWithFingerprint(absPath, hash string, withMetadata bool, fingerprint string) (CacheEntry, error) {
	return internalengine.NewCacheEntryWithFingerprint(absPath, hash, withMetadata, fingerprint)
}

// ShouldSkipByHash verifies content hash equality, matching the legacy cache
// behavior that reads the full file to confirm no changes.
func ShouldSkipByHash(cacheDir, absPath string) (bool, error) {
	return internalengine.ShouldSkipByHash(cacheDir, absPath)
}

// ShouldSkipByHashWithFingerprint verifies content hash equality only when the
// cached formatter fingerprint matches the requested fingerprint.
func ShouldSkipByHashWithFingerprint(cacheDir, absPath, fingerprint string) (bool, error) {
	return internalengine.ShouldSkipByHashWithFingerprint(cacheDir, absPath, fingerprint)
}

// ShouldSkipByMetadata relies on file size/modtime to avoid reading the file.
// It falls back to hash verification when metadata is not available in the cache
// entry and removes stale cache files when the source file has been deleted.
func ShouldSkipByMetadata(cacheDir, absPath string) (bool, error) {
	return internalengine.ShouldSkipByMetadata(cacheDir, absPath)
}

// ShouldSkipByMetadataWithFingerprint uses metadata only when the cached
// formatter fingerprint matches the requested fingerprint.
func ShouldSkipByMetadataWithFingerprint(cacheDir, absPath, fingerprint string) (bool, error) {
	return internalengine.ShouldSkipByMetadataWithFingerprint(cacheDir, absPath, fingerprint)
}

// WriteCacheEntry persists the given entry using either a metadata-aware or
// legacy format based on the presence of Size/ModTime.
func WriteCacheEntry(cacheDir, absPath string, entry CacheEntry) error {
	return internalengine.WriteCacheEntry(cacheDir, absPath, entry)
}

// ReadCacheEntry returns the stored cache entry, translating fs.ErrNotExist to
// a nil entry with no error to simplify callers.
func ReadCacheEntry(cacheDir, absPath string) (*CacheEntry, error) {
	return internalengine.ReadCacheEntry(cacheDir, absPath)
}

// CacheFilePath exposes the deterministic cache file resolution used by both
// single-file and directory workflows.
func CacheFilePath(cacheDir, absPath string) string {
	return internalengine.CacheFilePath(cacheDir, absPath)
}

// ShouldSkip routes to the preferred strategy (metadata-first by default) and
// falls back to hash-based verification when metadata is unavailable.
func ShouldSkip(cacheDir, absPath string, preferMetadata bool) (bool, error) {
	return internalengine.ShouldSkip(cacheDir, absPath, preferMetadata)
}

// ShouldSkipWithFingerprint routes to the preferred strategy and rejects cache
// hits produced by a different formatter configuration.
func ShouldSkipWithFingerprint(cacheDir, absPath string, preferMetadata bool, fingerprint string) (bool, error) {
	return internalengine.ShouldSkipWithFingerprint(cacheDir, absPath, preferMetadata, fingerprint)
}
