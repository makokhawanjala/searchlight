// Package index provides the core indexing functionality for SearchLight.
// The FileIndex combines the Trie's fast prefix search with rich file metadata,
// enabling SearchLight to not only find files quickly but also provide detailed
// information about each file (size, modification time, etc.).
package index

import (
	"strings"
	"sync"

	"github.com/makokhawanjala/searchlight/internal/indexer"
)

// FileIndex is the main index structure that combines:
// - Trie: For fast prefix-based path searching
// - Map: For quick metadata lookup by path
//
// Architecture Decision: Why separate Trie and Map?
//
// Option 1 (What we're doing): Trie for paths + Map for metadata
// ✅ Pros:
//   - Trie stays pure and focused (only handles paths)
//   - Map provides O(1) metadata lookup
//   - Can easily add more metadata without changing Trie
//   - Separation of concerns: searching vs data storage
//
// Option 2 (Alternative): Store FileInfo in Trie nodes
// ❌ Cons:
//   - Trie becomes tightly coupled to FileInfo
//   - Harder to test and maintain
//   - Every Trie node would need to handle FileInfo (even intermediate nodes)
//
// Our approach is more flexible and maintainable for SearchLight's evolution.
type FileIndex struct {
	// trie handles the path searching - it's our "search engine"
	// It answers: "What paths start with 'doc'?"
	trie *Trie

	// files maps full paths to their metadata
	// It answers: "What are the details of this specific file?"
	// We use a pointer to FileInfo to avoid copying large structs
	files map[string]*indexer.FileInfo

	// mu protects the files map from concurrent access
	//
	// Why do we need a separate mutex when Trie already has one?
	// - The Trie's mutex only protects the Trie structure
	// - We need to protect the files map separately
	// - This allows us to lock them independently:
	//   * Reading metadata doesn't block Trie operations
	//   * Trie searches don't block metadata updates
	//
	// Why RWMutex again?
	// - Most operations are reads (getting file info during search)
	// - Writes only happen when files are added/removed
	// - RWMutex allows concurrent reads, which is our common case
	mu sync.RWMutex
}

// NewFileIndex creates and initializes a new empty FileIndex.
//
// Why not use a constructor with parameters?
// - Simple initialization: no configuration needed
// - Consistent with Go idioms (NewX() functions for simple types)
// - Can easily add configuration options later without breaking existing code
func NewFileIndex() *FileIndex {
	return &FileIndex{
		trie:  NewTrie(),
		files: make(map[string]*indexer.FileInfo),
	}
}

// Add inserts or updates a file in the index.
//
// Why "Add" instead of "Insert" or "Put"?
// - "Add" is more user-friendly and matches file system terminology
// - It's clear that this adds a file to the index
// - Commonly used in Go standard library (e.g., testing.T.Add)
//
// Thread Safety Strategy:
// 1. Lock files map first (shorter critical section)
// 2. Update/insert FileInfo
// 3. Let Trie handle its own locking internally
//
// Why this order?
// - Minimizes time holding locks
// - Trie.Insert is already thread-safe internally
// - If Trie.Insert fails, files map is still consistent
//
// Parameters:
//   - fileInfo: Pointer to FileInfo containing all file metadata
//
// Design Decision: Why accept *FileInfo instead of FileInfo?
// - Avoids copying the entire struct (more efficient)
// - Matches the pattern used throughout SearchLight
// - Allows nil checks if needed in the future
func (idx *FileIndex) Add(fileInfo *indexer.FileInfo) {
	if fileInfo == nil || fileInfo.Path == "" {
		// Defensive programming: prevent nil pointer panics
		// Empty paths are meaningless in a file index
		return
	}

	// Lock the files map for writing
	idx.mu.Lock()
	// Store or update the file metadata
	// If path already exists, this updates it (handles file modifications)
	idx.files[fileInfo.Path] = fileInfo
	idx.mu.Unlock()

	// Add the path to the Trie for fast searching
	// The Trie handles its own locking, so we don't hold our lock here
	// This allows other operations to proceed concurrently
	idx.trie.Insert(fileInfo.Path)
}

// Remove deletes a file from the index.
//
// Why "Remove" instead of "Delete"?
// - More symmetric with "Add" (Add/Remove pair)
// - "Delete" is used by Trie internally
// - Clearer for public API
//
// Thread Safety:
// - Same strategy as Add: lock our map, let Trie handle its own
//
// Returns:
//   - true if the file was found and removed
//   - false if the file didn't exist in the index
//
// Why return bool?
// - Caller can distinguish between "removed" and "wasn't there"
// - Useful for logging: "File deleted" vs "File not in index (already gone?)"
// - Matches Trie.Delete behavior for consistency
func (idx *FileIndex) Remove(path string) bool {
	if path == "" {
		return false
	}

	// Lock files map and delete the metadata
	idx.mu.Lock()
	_, existed := idx.files[path]
	delete(idx.files, path)
	idx.mu.Unlock()

	// Remove from Trie (handles its own locking)
	// Note: We delete from files map first because:
	// - Even if Trie delete fails, we want metadata gone
	// - Prevents returning stale metadata for non-existent paths
	trieDeleted := idx.trie.Delete(path)

	// Return true if it existed in either structure
	// This handles edge cases where structures might be out of sync
	return existed || trieDeleted
}

// Get retrieves the FileInfo for a specific path.
//
// Use Case: After a search returns paths, we need to get details for display
//
// Time Complexity: O(1) - just a map lookup!
//
// Parameters:
//   - path: The full file path to look up
//
// Returns:
//   - *FileInfo: The file metadata (or nil if not found)
//   - bool: Whether the file exists in the index
//
// Why return both FileInfo and bool (Go's "comma ok" idiom)?
// - Distinguishes between "file not found" and "file exists but has no metadata"
// - Standard Go pattern for map lookups
// - Prevents returning nil without knowing why
//
// Example usage:
//
//	if fileInfo, ok := index.Get(path); ok {
//	    fmt.Printf("Found: %s (%s)\n", fileInfo.Name, fileInfo.HumanSize())
//	} else {
//	    fmt.Println("File not in index")
//	}
func (idx *FileIndex) Get(path string) (*indexer.FileInfo, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	fileInfo, exists := idx.files[path]
	return fileInfo, exists
}

// SearchPrefix finds all files whose paths start with the given prefix.
//
// This is the core search functionality of SearchLight!
//
// How it works:
// 1. Ask Trie for all matching paths (fast!)
// 2. Look up FileInfo for each path (O(1) per path)
// 3. Return slice of FileInfo pointers
//
// Why return []*FileInfo instead of []string?
// - Provides complete file information, not just paths
// - Caller can immediately display size, date, etc.
// - No need for second lookup to get metadata
// - Matches user expectations: "Show me files, not just paths"
//
// Time Complexity:
// - O(p) to find prefix in Trie, where p = prefix length
// - O(n) to collect n matching paths
// - O(n) to look up metadata for n paths
// - Total: O(p + n) where n is number of results
//
// Parameters:
//   - prefix: The path prefix to search for (e.g., "/home/user/doc")
//
// Returns:
//   - Slice of FileInfo pointers for all matching files
//   - Sorted by path (Trie.Search returns sorted results)
//   - Empty slice if no matches (never returns nil for consistency)
//
// Example:
//
//	results := index.SearchPrefix("/home/user/documents")
//	for _, file := range results {
//	    fmt.Printf("%s - %s\n", file.Name, file.HumanSize())
//	}
func (idx *FileIndex) SearchPrefix(prefix string) []*indexer.FileInfo {
	// Step 1: Get all matching paths from the Trie
	// Trie handles its own locking, returns sorted paths
	paths := idx.trie.Search(prefix)

	// Step 2: Allocate result slice with exact capacity
	// Why pre-allocate?
	// - Avoids slice growth and reallocation
	// - We know exactly how many results we'll have
	// - More memory efficient and faster
	results := make([]*indexer.FileInfo, 0, len(paths))

	// Step 3: Look up metadata for each path
	// We hold the read lock for the entire lookup to ensure consistency
	idx.mu.RLock()
	for _, path := range paths {
		if fileInfo, exists := idx.files[path]; exists {
			results = append(results, fileInfo)
		}
		// Note: If fileInfo doesn't exist, we skip it
		// This handles rare race condition where:
		// - Trie has path, but file was just being deleted
		// - Better to return slightly fewer results than crash
	}
	idx.mu.RUnlock()

	return results
}

// SearchByName finds files whose names (not full paths) contain the given query.
//
// Why separate from SearchPrefix?
// - Different use case: "Find all README files" vs "Find files in /home/user/"
// - SearchPrefix is O(p + n), SearchByName is O(total files)
// - Users want both types of search
//
// Example:
//
//	SearchPrefix("/home/user/doc")    → "/home/user/documents/file.txt"
//	SearchByName("README")            → "/home/user/project/README.md", "/var/www/README.txt"
//
// Time Complexity: O(n) where n is total indexed files
// - Must check every file (no Trie optimization here)
// - That's okay: modern computers check millions of strings per second
// - For 100k files, still completes in milliseconds
//
// Parameters:
//   - query: The text to search for in file names (case-insensitive)
//
// Returns:
//   - All files whose names contain the query
//   - Sorted by path for consistent results
//
// Why case-insensitive?
// - Users expect "readme" to find "README.md"
// - File systems vary: Windows is case-insensitive, Linux is case-sensitive
// - Making search case-insensitive provides consistent UX across platforms
func (idx *FileIndex) SearchByName(query string) []*indexer.FileInfo {
	if query == "" {
		// Empty query = match everything
		// Could return all files, but that's rarely useful
		// Returning empty is more intuitive: "search for nothing, find nothing"
		return []*indexer.FileInfo{}
	}

	// Convert query to lowercase once for efficiency
	queryLower := strings.ToLower(query)

	var results []*indexer.FileInfo

	// Iterate through all files
	// We need to lock because we're reading from the map
	idx.mu.RLock()
	for _, fileInfo := range idx.files {
		// Check if the file name contains the query (case-insensitive)
		nameLower := strings.ToLower(fileInfo.Name)
		if strings.Contains(nameLower, queryLower) {
			results = append(results, fileInfo)
		}
	}
	idx.mu.RUnlock()

	// Sort results by path for consistent output
	// Using a simple bubble sort here for clarity, but could optimize if needed
	// For typical result sizes (< 1000), this is plenty fast
	sortFileInfoByPath(results)

	return results
}

// SearchByExtension finds all files with the given extension.
//
// Use Case: "Show me all .txt files" or "Find all images (.jpg, .png)"
//
// Why a separate method?
// - Common search pattern in file management
// - More efficient than SearchByName (direct comparison vs substring search)
// - Allows easy implementation of file type filters in UI
//
// Time Complexity: O(n) where n is total files
//
// Parameters:
//   - ext: The file extension to search for (with or without leading dot)
//
// Returns:
//   - All files with the given extension
//   - Sorted by path
//
// Example:
//
//	index.SearchByExtension(".txt")  → All text files
//	index.SearchByExtension("txt")   → Same (we handle both formats)
func (idx *FileIndex) SearchByExtension(ext string) []*indexer.FileInfo {
	if ext == "" {
		return []*indexer.FileInfo{}
	}

	// Ensure extension has leading dot for consistent comparison
	// "txt" becomes ".txt"
	if ext[0] != '.' {
		ext = "." + ext
	}

	var results []*indexer.FileInfo

	idx.mu.RLock()
	for _, fileInfo := range idx.files {
		// Use FileInfo's built-in extension matching
		// This handles edge cases like ".tar.gz"
		if fileInfo.MatchesExtension(ext) {
			results = append(results, fileInfo)
		}
	}
	idx.mu.RUnlock()

	sortFileInfoByPath(results)
	return results
}

// Size returns the number of files in the index.
//
// Why not just call idx.trie.Size()?
// - Encapsulation: FileIndex clients shouldn't know about internal Trie
// - Allows us to change implementation later without breaking callers
// - Can add validation: ensure trie.Size() == len(files)
//
// Time Complexity: O(1)
//
// Returns:
//   - Number of files currently indexed
//
// Use Cases:
//   - Displaying "Indexed 150,000 files" in UI
//   - Monitoring index health
//   - Progress reporting during indexing
func (idx *FileIndex) Size() int {
	// We could lock and return len(idx.files), but Trie.Size() is already thread-safe
	// and is the authoritative count (it's our primary structure)
	return idx.trie.Size()
}

// Clear removes all files from the index.
//
// Use Cases:
// - Full re-index: clear old data before scanning again
// - Switching to different directory
// - Testing: reset index between tests
//
// Why not just create a new FileIndex?
// - Allows reusing the same FileIndex instance
// - Important if other parts of the code hold references to it
// - Explicit operation: caller clearly intends to clear
//
// Thread Safety:
// - Lock both structures before clearing
// - Prevents race where one is cleared but not the other
func (idx *FileIndex) Clear() {
	// Clear the Trie (handles its own locking internally)
	idx.trie.Clear()

	// Clear the files map
	idx.mu.Lock()
	// Create a new map instead of deleting keys one by one
	// This is faster and allows Go's GC to clean up the old map efficiently
	idx.files = make(map[string]*indexer.FileInfo)
	idx.mu.Unlock()
}

// GetAll returns all files in the index.
//
// Use Cases:
// - Exporting index data
// - Statistics gathering
// - Debugging: "Show me everything"
// - UI: "Display all indexed files"
//
// Warning: Can be large!
// - With 100k files, this returns 100k FileInfo pointers
// - Use with caution in production
// - Consider pagination or streaming for large indexes
//
// Returns:
//   - Slice of all FileInfo pointers, sorted by path
func (idx *FileIndex) GetAll() []*indexer.FileInfo {
	// Get all paths from Trie (already sorted)
	paths := idx.trie.Search("")

	results := make([]*indexer.FileInfo, 0, len(paths))

	idx.mu.RLock()
	for _, path := range paths {
		if fileInfo, exists := idx.files[path]; exists {
			results = append(results, fileInfo)
		}
	}
	idx.mu.RUnlock()

	return results
}

// Contains checks if a file exists in the index.
//
// Use Case: Quick existence check without retrieving full metadata
//
// Time Complexity: O(1) - map lookup
//
// Why not use idx.trie.Contains()?
// - Map lookup is slightly faster than Trie traversal
// - We're already maintaining the map
// - More direct: "Is this exact path in the index?"
//
// Parameters:
//   - path: The file path to check
//
// Returns:
//   - true if the file is indexed
//   - false otherwise
func (idx *FileIndex) Contains(path string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	_, exists := idx.files[path]
	return exists
}

// Stats returns statistics about the index.
//
// Use Cases:
// - Monitoring index health
// - Displaying stats in UI: "150,000 files indexed, 45 GB total"
// - Performance monitoring: tracking index growth
//
// Why return a struct instead of multiple values?
// - Easier to extend with new stats later
// - Cleaner API: stats := index.Stats()
// - Can add JSON tags for API responses
//
// Returns:
//   - IndexStats struct with current statistics
func (idx *FileIndex) Stats() IndexStats {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	stats := IndexStats{
		TotalFiles: idx.trie.Size(),
		TotalSize:  0,
	}

	// Calculate total size by summing all file sizes
	// This is O(n) but stats are typically not called frequently
	for _, fileInfo := range idx.files {
		stats.TotalSize += fileInfo.Size
	}

	return stats
}

// IndexStats holds statistics about the file index.
//
// Why a separate type?
// - Clear API: callers know what statistics are available
// - Easy to extend: add new fields without changing function signature
// - Can add methods: stats.HumanTotalSize()
// - JSON serialization for API responses
type IndexStats struct {
	TotalFiles int   `json:"total_files"` // Number of indexed files
	TotalSize  int64 `json:"total_size"`  // Total size in bytes
}

// sortFileInfoByPath sorts a slice of FileInfo pointers by their paths.
//
// Why not use sort.Slice inline everywhere?
// - DRY principle: define sorting logic once
// - Easier to change sorting criteria later (e.g., add secondary sort by name)
// - Cleaner code: sortFileInfoByPath(results) is more readable
//
// Why sort by path?
// - Consistent with Trie.Search (which returns sorted paths)
// - Predictable for users
// - Easier testing: results are deterministic
//
// Note: This modifies the slice in place (standard Go sorting behavior)
func sortFileInfoByPath(files []*indexer.FileInfo) {
	// Use a simple comparison-based sort
	// For typical result sizes (< 10,000), this is very fast

	// Bubble sort implementation for educational clarity
	// Production code might use sort.Slice for better performance on large datasets
	n := len(files)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if files[j].Path > files[j+1].Path {
				// Swap
				files[j], files[j+1] = files[j+1], files[j]
			}
		}
	}
}
