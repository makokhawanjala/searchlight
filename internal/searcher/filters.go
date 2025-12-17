package searcher

import (
	"time"

	"github.com/makokhawanjala/searchlight/internal/indexer"
)

// Filter is a function type that filters FileInfo results
// Returns true if the file passes the filter, false otherwise
// This allows us to compose multiple filters together
type Filter func(*indexer.FileInfo) bool

// FilterChain represents multiple filters combined with AND logic
// A file must pass ALL filters in the chain to be included in results
type FilterChain struct {
	filters []Filter
}

// NewFilterChain creates a new empty filter chain
func NewFilterChain() *FilterChain {
	return &FilterChain{
		filters: make([]Filter, 0),
	}
}

// Add adds a filter to the chain
// Filters are applied in the order they're added
// This allows building complex filter logic step by step
func (fc *FilterChain) Add(filter Filter) *FilterChain {
	fc.filters = append(fc.filters, filter)
	return fc // Return self for method chaining
}

// Apply applies all filters in the chain to a file
// Returns true only if the file passes ALL filters
// Short-circuits on first failed filter for efficiency
func (fc *FilterChain) Apply(file *indexer.FileInfo) bool {
	// If no filters, all files pass
	if len(fc.filters) == 0 {
		return true
	}

	// Apply each filter - if any returns false, file is filtered out
	for _, filter := range fc.filters {
		if !filter(file) {
			return false // Short-circuit - no need to check remaining filters
		}
	}

	// All filters passed
	return true
}

// ApplyToSlice applies the filter chain to a slice of files
// Returns only the files that pass all filters
// This is a convenience method for filtering search results
func (fc *FilterChain) ApplyToSlice(files []*indexer.FileInfo) []*indexer.FileInfo {
	// Pre-allocate with capacity (optimistic - assume most files pass)
	result := make([]*indexer.FileInfo, 0, len(files))

	for _, file := range files {
		if fc.Apply(file) {
			result = append(result, file)
		}
	}

	return result
}

// Count returns the number of filters in the chain
// Useful for debugging and logging
func (fc *FilterChain) Count() int {
	return len(fc.filters)
}

// ============================================================
// SIZE FILTERS
// ============================================================

// SizeFilter creates a filter for file size
// minSize: Minimum size in bytes (0 = no minimum)
// maxSize: Maximum size in bytes (0 = no maximum)
//
// Example Usage:
//   filter := SizeFilter(1024, 1048576)  // Between 1KB and 1MB
//   filter := SizeFilter(1024, 0)        // At least 1KB (no maximum)
//   filter := SizeFilter(0, 1048576)     // At most 1MB (no minimum)
func SizeFilter(minSize, maxSize int64) Filter {
	return func(file *indexer.FileInfo) bool {
		// Skip directories - they don't have meaningful sizes
		if file.IsDir {
			return false
		}

		// Check minimum size
		if minSize > 0 && file.Size < minSize {
			return false
		}

		// Check maximum size
		if maxSize > 0 && file.Size > maxSize {
			return false
		}

		return true
	}
}

// MinSizeFilter creates a filter for minimum file size
// This is a convenience wrapper around SizeFilter
//
// Example: MinSizeFilter(1024) filters files smaller than 1KB
func MinSizeFilter(minSize int64) Filter {
	return SizeFilter(minSize, 0)
}

// MaxSizeFilter creates a filter for maximum file size
// This is a convenience wrapper around SizeFilter
//
// Example: MaxSizeFilter(1048576) filters files larger than 1MB
func MaxSizeFilter(maxSize int64) Filter {
	return SizeFilter(0, maxSize)
}

// EmptyFileFilter filters out empty files (size = 0)
// Useful for finding files that actually contain data
func EmptyFileFilter() Filter {
	return func(file *indexer.FileInfo) bool {
		return !file.IsDir && file.Size > 0
	}
}

// LargeFileFilter filters for large files above a threshold
// Default threshold: 100 MB
// These are files that might need special handling
func LargeFileFilter(threshold int64) Filter {
	if threshold == 0 {
		threshold = 100 * 1024 * 1024 // 100 MB default
	}
	return MinSizeFilter(threshold)
}

// ============================================================
// DATE/TIME FILTERS
// ============================================================

// ModifiedAfterFilter creates a filter for files modified after a specific time
// Useful for finding recently changed files
//
// Example: ModifiedAfterFilter(time.Now().AddDate(0, 0, -7)) finds files modified in last 7 days
func ModifiedAfterFilter(after time.Time) Filter {
	return func(file *indexer.FileInfo) bool {
		return file.ModifiedTime.After(after)
	}
}

// ModifiedBeforeFilter creates a filter for files modified before a specific time
// Useful for finding old files that might need archiving
//
// Example: ModifiedBeforeFilter(time.Now().AddDate(-1, 0, 0)) finds files older than 1 year
func ModifiedBeforeFilter(before time.Time) Filter {
	return func(file *indexer.FileInfo) bool {
		return file.ModifiedTime.Before(before)
	}
}

// ModifiedBetweenFilter creates a filter for files modified within a time range
// Both start and end are inclusive
//
// Example: ModifiedBetweenFilter(start, end) finds files modified in that period
func ModifiedBetweenFilter(start, end time.Time) Filter {
	return func(file *indexer.FileInfo) bool {
		// File modified time must be >= start and <= end
		return (file.ModifiedTime.Equal(start) || file.ModifiedTime.After(start)) &&
			(file.ModifiedTime.Equal(end) || file.ModifiedTime.Before(end))
	}
}

// ModifiedWithinDaysFilter creates a filter for files modified within N days
// This is a convenience wrapper for recent file searches
//
// Example: ModifiedWithinDaysFilter(7) finds files modified in last 7 days
func ModifiedWithinDaysFilter(days int) Filter {
	cutoff := time.Now().AddDate(0, 0, -days)
	return ModifiedAfterFilter(cutoff)
}

// ModifiedWithinHoursFilter creates a filter for files modified within N hours
// Useful for finding very recently changed files
//
// Example: ModifiedWithinHoursFilter(24) finds files modified in last 24 hours
func ModifiedWithinHoursFilter(hours int) Filter {
	cutoff := time.Now().Add(time.Duration(-hours) * time.Hour)
	return ModifiedAfterFilter(cutoff)
}

// OldFileFilter filters for files older than a specified number of days
// Useful for finding stale files that might need cleanup
//
// Example: OldFileFilter(365) finds files not modified in over a year
func OldFileFilter(days int) Filter {
	cutoff := time.Now().AddDate(0, 0, -days)
	return ModifiedBeforeFilter(cutoff)
}

// TodayFilter filters for files modified today
// Useful for daily activity reports
func TodayFilter() Filter {
	// Get start of today (midnight)
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	
	return ModifiedAfterFilter(startOfDay)
}

// ============================================================
// EXTENSION FILTERS
// ============================================================

// ExtensionFilter creates a filter for specific file extensions
// Extensions should include the dot (e.g., ".txt", ".go")
// If extension doesn't have a dot, it will be added automatically
//
// Example: ExtensionFilter(".txt", ".md") filters for text and markdown files
func ExtensionFilter(extensions ...string) Filter {
	// Normalize extensions (ensure they have leading dot)
	normalized := make([]string, len(extensions))
	for i, ext := range extensions {
		if ext != "" && ext[0] != '.' {
			normalized[i] = "." + ext
		} else {
			normalized[i] = ext
		}
	}

	return func(file *indexer.FileInfo) bool {
		// Directories don't have extensions
		if file.IsDir {
			return false
		}

		// Check if file extension matches any of the specified extensions
		for _, ext := range normalized {
			if file.Extension == ext {
				return true
			}
		}

		return false
	}
}

// NoExtensionFilter filters for files without extensions
// These are often configuration files or executables
//
// Example: Files like "Makefile", "Dockerfile", "README"
func NoExtensionFilter() Filter {
	return func(file *indexer.FileInfo) bool {
		return !file.IsDir && file.Extension == ""
	}
}

// HasExtensionFilter filters for files that have any extension
// Opposite of NoExtensionFilter
func HasExtensionFilter() Filter {
	return func(file *indexer.FileInfo) bool {
		return !file.IsDir && file.Extension != ""
	}
}

// ============================================================
// FILE TYPE FILTERS (by common extensions)
// ============================================================

// TextFileFilter filters for common text file types
// Includes: .txt, .md, .log, .csv, .json, .xml, .yaml, .yml
func TextFileFilter() Filter {
	return ExtensionFilter(".txt", ".md", ".log", ".csv", ".json", ".xml", ".yaml", ".yml")
}

// CodeFileFilter filters for common programming language files
// Includes: .go, .py, .js, .java, .c, .cpp, .rs, .rb, .php, .swift
func CodeFileFilter() Filter {
	return ExtensionFilter(
		".go", ".py", ".js", ".ts", ".java", ".c", ".cpp", ".h", ".hpp",
		".rs", ".rb", ".php", ".swift", ".kt", ".scala", ".sh", ".bash",
	)
}

// ImageFileFilter filters for common image file types
// Includes: .jpg, .jpeg, .png, .gif, .bmp, .svg, .webp, .ico
func ImageFileFilter() Filter {
	return ExtensionFilter(".jpg", ".jpeg", ".png", ".gif", ".bmp", ".svg", ".webp", ".ico")
}

// VideoFileFilter filters for common video file types
// Includes: .mp4, .avi, .mkv, .mov, .wmv, .flv, .webm
func VideoFileFilter() Filter {
	return ExtensionFilter(".mp4", ".avi", ".mkv", ".mov", ".wmv", ".flv", ".webm", ".m4v")
}

// AudioFileFilter filters for common audio file types
// Includes: .mp3, .wav, .flac, .aac, .ogg, .m4a, .wma
func AudioFileFilter() Filter {
	return ExtensionFilter(".mp3", ".wav", ".flac", ".aac", ".ogg", ".m4a", ".wma")
}

// DocumentFileFilter filters for common document file types
// Includes: .pdf, .doc, .docx, .xls, .xlsx, .ppt, .pptx, .odt
func DocumentFileFilter() Filter {
	return ExtensionFilter(".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx", ".odt", ".ods")
}

// ArchiveFileFilter filters for common archive/compression file types
// Includes: .zip, .tar, .gz, .bz2, .7z, .rar, .xz
func ArchiveFileFilter() Filter {
	return ExtensionFilter(".zip", ".tar", ".gz", ".bz2", ".7z", ".rar", ".xz", ".tgz")
}

// ============================================================
// DIRECTORY FILTERS
// ============================================================

// DirectoryFilter filters for directories only
// Useful when you want to list only directories
func DirectoryFilter() Filter {
	return func(file *indexer.FileInfo) bool {
		return file.IsDir
	}
}

// FileOnlyFilter filters for regular files only (not directories)
// This is the most common case for file searches
func FileOnlyFilter() Filter {
	return func(file *indexer.FileInfo) bool {
		return !file.IsDir
	}
}

// ============================================================
// NAME FILTERS
// ============================================================

// HiddenFileFilter filters for hidden files (starting with dot)
// These are typically configuration files or system files
// Note: On Windows, this only checks filename, not file attributes
func HiddenFileFilter() Filter {
	return func(file *indexer.FileInfo) bool {
		return len(file.Name) > 0 && file.Name[0] == '.'
	}
}

// VisibleFileFilter filters for non-hidden files (not starting with dot)
// This is the opposite of HiddenFileFilter
func VisibleFileFilter() Filter {
	return func(file *indexer.FileInfo) bool {
		return len(file.Name) > 0 && file.Name[0] != '.'
	}
}

// ============================================================
// COMPOSITE FILTERS
// ============================================================

// RecentLargeFilesFilter creates a filter for large files modified recently
// Useful for finding files that are both large and actively used
//
// Parameters:
//   - minSize: Minimum file size in bytes
//   - days: Files modified within this many days
func RecentLargeFilesFilter(minSize int64, days int) Filter {
	sizeFilter := MinSizeFilter(minSize)
	dateFilter := ModifiedWithinDaysFilter(days)

	return func(file *indexer.FileInfo) bool {
		return sizeFilter(file) && dateFilter(file)
	}
}

// OldLargeFilesFilter creates a filter for large old files
// Useful for finding candidates for archiving or deletion
//
// Parameters:
//   - minSize: Minimum file size in bytes
//   - days: Files NOT modified within this many days
func OldLargeFilesFilter(minSize int64, days int) Filter {
	sizeFilter := MinSizeFilter(minSize)
	dateFilter := OldFileFilter(days)

	return func(file *indexer.FileInfo) bool {
		return sizeFilter(file) && dateFilter(file)
	}
}

// ============================================================
// UTILITY FUNCTIONS
// ============================================================

// CombineFiltersAND combines multiple filters with AND logic
// A file must pass ALL filters to be included
// This is functionally equivalent to FilterChain but more functional style
func CombineFiltersAND(filters ...Filter) Filter {
	return func(file *indexer.FileInfo) bool {
		for _, filter := range filters {
			if !filter(file) {
				return false
			}
		}
		return true
	}
}

// CombineFiltersOR combines multiple filters with OR logic
// A file only needs to pass ONE filter to be included
// Useful for "find .txt OR .md files" type queries
func CombineFiltersOR(filters ...Filter) Filter {
	return func(file *indexer.FileInfo) bool {
		// If no filters, everything passes (empty OR is true)
		if len(filters) == 0 {
			return true
		}

		// Check each filter - if any passes, file is included
		for _, filter := range filters {
			if filter(file) {
				return true
			}
		}
		return false
	}
}

// NotFilter inverts a filter (logical NOT)
// A file passes if it FAILS the original filter
// Useful for "all files EXCEPT .txt" type queries
func NotFilter(filter Filter) Filter {
	return func(file *indexer.FileInfo) bool {
		return !filter(file)
	}
}

// ApplyFilters is a convenience function to apply multiple filters to a slice
// Uses AND logic - files must pass all filters
func ApplyFilters(files []*indexer.FileInfo, filters ...Filter) []*indexer.FileInfo {
	if len(filters) == 0 {
		return files
	}

	chain := NewFilterChain()
	for _, filter := range filters {
		chain.Add(filter)
	}

	return chain.ApplyToSlice(files)
}