// Package searcher provides search functionality for SearchLight.
// The Searcher acts as a high-level search interface that wraps the FileIndex
// and provides different search strategies (prefix, name, extension).
//
// Why separate Searcher from FileIndex?
// - Separation of concerns: FileIndex manages data, Searcher manages queries
// - Allows adding advanced search features (ranking, caching, filters) later
// - Provides a clean API for the REST layer
// - Can add multiple Searcher implementations (fast, comprehensive, etc.)
package searcher

import (
	"strings"

	"github.com/makokhawanjala/searchlight/internal/index"
	"github.com/makokhawanjala/searchlight/internal/indexer"
)

// Searcher provides high-level search operations on an indexed file collection.
//
// Architecture Decision: Why wrap FileIndex instead of using it directly?
// - Abstraction: API/CLI code doesn't need to know about FileIndex internals
// - Future-proofing: Can add caching, ranking, query parsing without changing API
// - Testing: Easy to mock Searcher for API tests
// - Clean separation: Searching logic vs storage logic
type Searcher struct {
	// index is the underlying file index that stores all indexed files
	// We use a pointer because:
	// 1. FileIndex is shared across the application (single source of truth)
	// 2. We don't own the index, we just query it
	// 3. Avoids copying the entire index structure
	index *index.FileIndex
	cache  *Cache
	parser *QueryParser
}

// NewSearcher creates a new Searcher instance.
//
// Why a constructor?
// - Ensures index is never nil (defensive programming)
// - Allows adding configuration options later (e.g., cache settings)
// - Matches Go idioms (NewX() pattern)
//
// Parameters:
//   - idx: The FileIndex to search. Must not be nil.
//
// Returns:
//   - *Searcher: A new Searcher instance ready to perform searches
//
// Example usage:
//
//	fileIndex := index.NewFileIndex()
//	searcher := searcher.NewSearcher(fileIndex)
//	results := searcher.SearchByPrefix("/home/user/documents")

func NewSearcher(idx *index.FileIndex) *Searcher {
	return &Searcher{
		index:  idx,
		cache:  NewCache(100),
		parser: NewQueryParser(),
	}
}

// SearchByPrefix finds all files whose paths start with the given prefix.
//
// This is the fastest search type because it uses the Trie's prefix matching.
// Perfect for:
// - Directory browsing: "Show me everything in /home/user/documents"
// - Path autocomplete: "doc" → "documents/", "docs/"
// - Navigation: "cd /var/l" → "log/", "lib/", "lock/"
//
// How it works:
// 1. Delegates to FileIndex.SearchPrefix()
// 2. FileIndex uses Trie for O(p + n) search where:
//    - p = prefix length
//    - n = number of matching files
//
// Time Complexity: O(p + n)
// - Significantly faster than linear search through all files
// - Example: With 100k files, searching "doc" only examines ~1000 files
//
// Case Sensitivity: Case-sensitive by design
// - File systems vary: Linux is case-sensitive, Windows is not
// - We preserve exact case for accuracy
// - If case-insensitive search is needed, caller should normalize prefix
//
// Parameters:
//   - prefix: The path prefix to search for (e.g., "/home/user/doc")
//
// Returns:
//   - []*indexer.FileInfo: Slice of all matching files, sorted by path
//   - Empty slice if no matches (never returns nil)
//
// Example:
//
//	results := searcher.SearchByPrefix("/var/log")
//	for _, file := range results {
//	    fmt.Printf("Found: %s (%s)\n", file.Path, file.HumanSize())
//	}
func (s *Searcher) SearchByPrefix(prefix string) []*indexer.FileInfo {
	// Input validation: empty prefix handled by FileIndex.SearchPrefix()
	// It will return all files (which might be what the user wants)

	// Direct delegation to FileIndex
	// Why not add logic here?
	// - Keep it simple: Searcher is a thin wrapper at this stage
	// - FileIndex.SearchPrefix already handles all the complexity
	// - In later phases, we'll add ranking, filtering, caching HERE
	return s.index.SearchPrefix(prefix)
}

// SearchByName finds all files whose names (not full paths) contain the query string.
//
// Use Cases:
// - "Find all README files" → SearchByName("README")
// - "Find my config files" → SearchByName("config")
// - "Where are my JPG images?" → SearchByName(".jpg")
//
// Difference from SearchByPrefix:
// - SearchByPrefix: matches path start → "/home/user/doc" finds "/home/user/documents/file.txt"
// - SearchByName: matches filename anywhere → "README" finds "/home/user/project/README.md"
//
// How it works:
// 1. Delegates to FileIndex.SearchByName()
// 2. FileIndex checks every file's name (O(n) operation)
// 3. Uses case-insensitive substring matching
//
// Time Complexity: O(n) where n is total indexed files
// - Must check every file (no Trie optimization possible)
// - Still fast: checking 100k filenames takes only milliseconds
//
// Case Sensitivity: Case-insensitive
// - "readme" matches "README.md", "ReadMe.txt", "readme.doc"
// - User-friendly: matches user expectations
// - Consistent across platforms
//
// Parameters:
//   - query: The text to search for in file names (case-insensitive)
//
// Returns:
//   - []*indexer.FileInfo: All files whose names contain the query, sorted by path
//   - Empty slice if no matches
//
// Example:
//
//	// Find all Go source files
//	results := searcher.SearchByName(".go")
//	
//	// Find configuration files
//	results := searcher.SearchByName("config")
//	
//	// Find all README files in any directory
//	results := searcher.SearchByName("README")
func (s *Searcher) SearchByName(query string) []*indexer.FileInfo {
	// Validation: empty query handled by FileIndex
	// It returns empty slice (intuitive: "search nothing, find nothing")

	// Direct delegation to FileIndex
	// FileIndex handles:
	// - Case normalization (toLowerCase)
	// - Substring matching (strings.Contains)
	// - Result sorting
	return s.index.SearchByName(query)
}

// SearchByExtension finds all files with the given file extension.
//
// Use Cases:
// - "Show all text files" → SearchByExtension(".txt")
// - "Find all images" → SearchByExtension(".jpg") (call multiple times for .png, .gif, etc.)
// - "List Python scripts" → SearchByExtension(".py")
//
// Why a dedicated method for extensions?
// - Common search pattern in file management
// - More efficient than SearchByName (exact extension match vs substring)
// - Enables file type filtering in UI (checkboxes for .txt, .pdf, .jpg, etc.)
// - Semantic clarity: "search by extension" is clearer than "search by name"
//
// How it works:
// 1. Delegates to FileIndex.SearchByExtension()
// 2. FileIndex uses FileInfo.MatchesExtension() for exact matching
// 3. Handles both ".txt" and "txt" formats (auto-adds dot if missing)
//
// Time Complexity: O(n) where n is total indexed files
// - Must check every file's extension
// - Fast in practice: extension comparison is just string equality
//
// Extension Normalization:
// - Automatically adds leading dot: "txt" → ".txt"
// - Consistent behavior regardless of input format
//
// Parameters:
//   - ext: The file extension to search for (with or without leading dot)
//
// Returns:
//   - []*indexer.FileInfo: All files with the given extension, sorted by path
//   - Empty slice if no matches
//
// Example:
//
//	// Find all Markdown files
//	markdownFiles := searcher.SearchByExtension(".md")
//	
//	// Find all images (would need multiple calls in practice)
//	jpgFiles := searcher.SearchByExtension("jpg")  // Auto-converted to ".jpg"
//	pngFiles := searcher.SearchByExtension(".png")
//	
//	// Find files with no extension
//	noExtFiles := searcher.SearchByExtension("")  // Returns empty (no matches)
func (s *Searcher) SearchByExtension(ext string) []*indexer.FileInfo {
	// Validation and normalization handled by FileIndex.SearchByExtension()
	// It ensures extension has leading dot and returns empty slice for ""

	// Direct delegation to FileIndex
	// FileIndex handles:
	// - Extension normalization (adding dot if missing)
	// - FileInfo.MatchesExtension() for accurate comparison
	// - Result sorting
	return s.index.SearchByExtension(ext)
}

// GetFileByPath retrieves detailed information for a specific file path.
//
// Use Cases:
// - "Show details for /home/user/document.txt"
// - After search, user clicks a result to see full details
// - Verifying if a specific file is in the index
//
// Why provide this when we have FileIndex.Get()?
// - Consistent API: All searches go through Searcher
// - Allows adding cache, audit logging, access control later
// - Cleaner for API layer: one dependency (Searcher) instead of two
//
// How it works:
// - Direct delegation to FileIndex.Get()
// - O(1) map lookup in FileIndex
//
// Time Complexity: O(1) - just a map lookup
//
// Parameters:
//   - path: The exact file path to look up
//
// Returns:
//   - *indexer.FileInfo: The file metadata if found
//   - bool: true if file exists in index, false otherwise
//
// Example:
//
//	if fileInfo, found := searcher.GetFileByPath("/var/log/syslog"); found {
//	    fmt.Printf("File: %s\n", fileInfo.Name)
//	    fmt.Printf("Size: %s\n", fileInfo.HumanSize())
//	    fmt.Printf("Modified: %s\n", fileInfo.FormattedModTime())
//	} else {
//	    fmt.Println("File not in index")
//	}
func (s *Searcher) GetFileByPath(path string) (*indexer.FileInfo, bool) {
	// Validation: empty path handled by FileIndex.Get()
	// It returns (nil, false) which is correct

	// Direct delegation to FileIndex
	// FileIndex.Get() uses read lock for thread-safety
	return s.index.Get(path)
}

// Count returns the total number of files in the index.
//
// Use Cases:
// - Displaying "150,000 files indexed" in UI
// - Monitoring: tracking index size over time
// - Health checks: ensuring index isn't empty
// - Progress calculation: "Searched 10,000 of 150,000 files"
//
// Why provide this when we have FileIndex.Size()?
// - Consistent API: All operations through Searcher
// - Encapsulation: API layer doesn't need to know about FileIndex
// - Can add derived metrics later (directories count, total size, etc.)
//
// Time Complexity: O(1) - FileIndex.Size() just returns a counter
//
// Returns:
//   - int: Number of files currently indexed
//
// Example:
//
//	count := searcher.Count()
//	fmt.Printf("Indexed %d files\n", count)
func (s *Searcher) Count() int {
	// Direct delegation to FileIndex.Size()
	// FileIndex maintains count atomically during Add/Remove
	return s.index.Size()
}

// SearchAll returns all indexed files.
//
// Use Cases:
// - "Show me everything you've indexed"
// - Export functionality: save all indexed files to JSON
// - Statistics: calculate total size, file type distribution
// - Admin interface: browse entire index
//
// ⚠️ Warning: This can return a LOT of data!
// - With 100k files, this returns 100k FileInfo pointers
// - Memory usage: ~100 bytes per FileInfo = ~10MB for 100k files
// - Use with caution in production
// - Consider adding pagination in future phases
//
// When to use:
// - ✅ Admin tools, debugging, exports
// - ✅ Small indexes (< 10k files)
// - ❌ Public API endpoints without pagination
// - ❌ Frequent calls in UI
//
// Time Complexity: O(n) where n is total indexed files
//
// Returns:
//   - []*indexer.FileInfo: All indexed files, sorted by path
//   - Empty slice if index is empty
//
// Example:
//
//	allFiles := searcher.SearchAll()
//	fmt.Printf("Total indexed files: %d\n", len(allFiles))
//	for _, file := range allFiles {
//	    fmt.Printf("  %s - %s\n", file.Path, file.HumanSize())
//	}
func (s *Searcher) SearchAll() []*indexer.FileInfo {
	// Delegates to FileIndex.GetAll()
	// FileIndex.GetAll() calls Trie.Search("") which returns everything
	return s.index.GetAll()
}

// ContainsPath checks if a specific file path exists in the index.
//
// Use Cases:
// - Quick existence check: "Is /etc/hosts indexed?"
// - Validation before operations: "Should I update this file?"
// - Conditional logic: "If file exists in index, show details"
//
// Why use this instead of GetFileByPath()?
// - More semantic: name clearly indicates boolean check
// - Slightly more efficient: no need to return FileInfo if you just need existence
// - Clearer intent in code: if searcher.ContainsPath(path) vs if _, ok := searcher.Get(path); ok
//
// Time Complexity: O(1) - just a map lookup in FileIndex
//
// Parameters:
//   - path: The file path to check
//
// Returns:
//   - bool: true if the path exists in the index, false otherwise
//
// Example:
//
//	if searcher.ContainsPath("/var/log/syslog") {
//	    fmt.Println("File is indexed")
//	} else {
//	    fmt.Println("File not found in index")
//	}
func (s *Searcher) ContainsPath(path string) bool {
	// Direct delegation to FileIndex.Contains()
	// FileIndex.Contains() uses read lock for thread-safety
	return s.index.Contains(path)
}

// Stats returns statistics about the indexed files.
//
// Use Cases:
// - Dashboard: "150,000 files, 45 GB total"
// - Monitoring: tracking index growth
// - Health checks: ensuring index is healthy
// - API endpoint: GET /api/stats
//
// Why provide this instead of manually calculating?
// - Encapsulation: Stats logic lives in one place
// - Consistency: Everyone gets the same stats format
// - Performance: Can add caching for expensive stats later
// - Extensibility: Easy to add new statistics
//
// Current Statistics Provided:
// - TotalFiles: Number of indexed files
// - TotalSize: Total size of all files in bytes
//
// Future Statistics (Phase 8+):
// - TotalDirectories: Number of directories
// - AverageFileSize: Mean file size
// - FileTypeDistribution: Count by extension
// - LastIndexTime: When index was last updated
//
// Time Complexity: O(n) for TotalSize calculation
// - Must sum all file sizes
// - In practice, very fast (just integer addition)
// - Could be cached if called frequently
//
// Returns:
//   - index.IndexStats: Statistics about the index
//
// Example:
//
//	stats := searcher.Stats()
//	fmt.Printf("Indexed: %d files\n", stats.TotalFiles)
//	fmt.Printf("Total size: %d bytes\n", stats.TotalSize)
//	
//	// Format total size in human-readable form
//	totalGB := float64(stats.TotalSize) / (1024 * 1024 * 1024)
//	fmt.Printf("Total size: %.2f GB\n", totalGB)
func (s *Searcher) Stats() index.IndexStats {
	// Direct delegation to FileIndex.Stats()
	// FileIndex.Stats() calculates total size by iterating all files
	return s.index.Stats()
}

// SearchMultipleExtensions finds all files matching any of the given extensions.
//
// Use Cases:
// - "Find all images" → SearchMultipleExtensions([".jpg", ".png", ".gif"])
// - "Find all documents" → SearchMultipleExtensions([".pdf", ".doc", ".txt"])
// - "Find all code files" → SearchMultipleExtensions([".go", ".py", ".js"])
//
// Why not just call SearchByExtension() multiple times?
// - Convenience: One call instead of many
// - More efficient: Single iteration through files (O(n) vs O(n*m))
// - Cleaner API usage:
//   ```
//   // Without this method:
//   results := append(searcher.SearchByExtension(".jpg"), searcher.SearchByExtension(".png")...)
//   
//   // With this method:
//   results := searcher.SearchMultipleExtensions([]string{".jpg", ".png"})
//   ```
//
// Time Complexity: O(n * m) where:
// - n = total indexed files
// - m = number of extensions to check
// - In practice, m is small (typically 2-5 extensions)
//
// Parameters:
//   - extensions: Slice of file extensions to search for (with or without dots)
//
// Returns:
//   - []*indexer.FileInfo: All files matching any of the extensions, sorted by path
//   - Empty slice if no matches or if extensions slice is empty
//
// Example:
//
//	// Find all image files
//	imageFiles := searcher.SearchMultipleExtensions([]string{".jpg", ".png", ".gif", ".bmp"})
//	
//	// Find all document files
//	docFiles := searcher.SearchMultipleExtensions([]string{"pdf", "doc", "docx", "txt"})
//	
//	fmt.Printf("Found %d image files\n", len(imageFiles))
func (s *Searcher) SearchMultipleExtensions(extensions []string) []*indexer.FileInfo {
	// Handle edge cases
	if len(extensions) == 0 {
		return []*indexer.FileInfo{}
	}

	// Normalize all extensions (add dot if missing)
	normalizedExts := make([]string, len(extensions))
	for i, ext := range extensions {
		if ext != "" && ext[0] != '.' {
			normalizedExts[i] = "." + ext
		} else {
			normalizedExts[i] = ext
		}
	}

	// Use a map to track which files we've already added (avoid duplicates)
	// Why map? O(1) lookup to check if path already seen
	seen := make(map[string]bool)
	var results []*indexer.FileInfo

	// For each extension, get matching files
	for _, ext := range normalizedExts {
		files := s.index.SearchByExtension(ext)
		for _, file := range files {
			// Only add if not already in results
			if !seen[file.Path] {
				seen[file.Path] = true
				results = append(results, file)
			}
		}
	}

	// Sort results by path for consistent output
	// Using the same sorting logic as FileIndex
	sortFileInfoByPath(results)

	return results
}

// Search executes an advanced search with full query parsing, filtering, ranking, and caching
//
// This is the main search method that combines all Phase 8 features:
// - Query parsing (substring, wildcard, regex, fuzzy)
// - Result filtering (size, date, extension)
// - Result ranking (by relevance)
// - Result caching (LRU cache for performance)
//
// Use Cases:
// - "config ext:.go,.txt" → Find config files with specific extensions
// - "test*.go" → Wildcard search for test files
// - "regex:test[0-9]+" → Regex pattern matching
// - "fuzzy:cofig" → Typo-tolerant search
//
// Parameters:
//   - queryStr: The query string (supports filters and type prefixes)
//
// Returns:
//   - []*indexer.FileInfo: Filtered, ranked, sorted results
//   - error: If query parsing fails
//
// Example:
//
//	results, err := searcher.Search("config ext:.go size:>1KB")
//	if err != nil {
//	    log.Fatal(err)
//	}
func (s *Searcher) Search(queryStr string) ([]*indexer.FileInfo, error) {
	// Check cache first
	if cached, found := s.cache.Get(queryStr); found {
		return cached, nil
	}

	// Parse the query
	query, err := s.parser.Parse(queryStr)
	if err != nil {
		return nil, err
	}

	// Execute search based on query type
	var results []*indexer.FileInfo

	switch query.Type {
	case QueryTypePrefix:
		results = s.SearchByPrefix(query.Pattern)

	case QueryTypeExact:
		results = s.searchExact(query.Pattern, query.CaseSensitive)

	case QueryTypeSubstring:
		results = s.searchSubstring(query.Pattern, query.CaseSensitive)

	case QueryTypeWildcard:
		results = s.searchWildcard(query)

	case QueryTypeRegex:
		results = s.searchRegex(query)

	case QueryTypeFuzzy:
		results = s.searchFuzzy(query)

	default:
		results = s.SearchByName(query.Pattern)
	}

	// Apply filters
	results = s.applyQueryFilters(results, query)

	// Apply ranking if query has text
	if query.Pattern != "" {
		ranker := NewRanker(RankingCriteria{
			Query:         query.Pattern,
			PreferExact:   true,
			CaseSensitive: query.CaseSensitive,
			PreferSmaller: true,
		})
		results = ranker.Rank(results)
	}

	// Cache results
	s.cache.Put(queryStr, results)

	return results, nil
}

// searchExact finds files with exact filename matches
func (s *Searcher) searchExact(pattern string, caseSensitive bool) []*indexer.FileInfo {
	allFiles := s.index.GetAll()
	var results []*indexer.FileInfo

	searchPattern := pattern
	if !caseSensitive {
		searchPattern = strings.ToLower(pattern)
	}

	for _, file := range allFiles {
		fileName := file.Name
		if !caseSensitive {
			fileName = strings.ToLower(file.Name)
		}

		if fileName == searchPattern {
			results = append(results, file)
		}
	}

	return results
}

// searchSubstring finds files containing the pattern
func (s *Searcher) searchSubstring(pattern string, caseSensitive bool) []*indexer.FileInfo {
	if !caseSensitive {
		return s.SearchByName(pattern)
	}

	allFiles := s.index.GetAll()
	var results []*indexer.FileInfo

	for _, file := range allFiles {
		if strings.Contains(file.Name, pattern) {
			results = append(results, file)
		}
	}

	return results
}

// searchWildcard finds files matching wildcard pattern
func (s *Searcher) searchWildcard(query *Query) []*indexer.FileInfo {
	allFiles := s.index.GetAll()
	var results []*indexer.FileInfo

	for _, file := range allFiles {
		name := file.Name
		if !query.CaseSensitive {
			name = strings.ToLower(file.Name)
		}

		if query.CompiledRegex.MatchString(name) {
			results = append(results, file)
		}
	}

	return results
}

// searchRegex finds files matching regex pattern
func (s *Searcher) searchRegex(query *Query) []*indexer.FileInfo {
	allFiles := s.index.GetAll()
	var results []*indexer.FileInfo

	for _, file := range allFiles {
		if query.CompiledRegex.MatchString(file.Name) {
			results = append(results, file)
		}
	}

	return results
}

// searchFuzzy finds files with fuzzy matching (Levenshtein distance)
func (s *Searcher) searchFuzzy(query *Query) []*indexer.FileInfo {
	allFiles := s.index.GetAll()
	var results []*indexer.FileInfo

	pattern := query.Pattern
	if !query.CaseSensitive {
		pattern = strings.ToLower(pattern)
	}

	for _, file := range allFiles {
		name := file.Name
		if !query.CaseSensitive {
			name = strings.ToLower(file.Name)
		}

		if levenshteinDistance(pattern, name) <= query.FuzzyDistance {
			results = append(results, file)
		}
	}

	return results
}

// applyQueryFilters applies size and extension filters from parsed query
func (s *Searcher) applyQueryFilters(files []*indexer.FileInfo, query *Query) []*indexer.FileInfo {
	chain := NewFilterChain()

	// Add size filter if specified
	if query.MinSize > 0 || query.MaxSize > 0 {
		chain.Add(SizeFilter(query.MinSize, query.MaxSize))
	}

	// Add extension filter if specified
	if len(query.Extensions) > 0 {
		chain.Add(ExtensionFilter(query.Extensions...))
	}

	return chain.ApplyToSlice(files)
}

// levenshteinDistance calculates the Levenshtein distance between two strings
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	matrix := make([][]int, len(s1)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(s2)+1)
		matrix[i][0] = i
	}
	for j := range matrix[0] {
		matrix[0][j] = j
	}

	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 0
			if s1[i-1] != s2[j-1] {
				cost = 1
			}

			matrix[i][j] = min(
				matrix[i-1][j]+1,
				matrix[i][j-1]+1,
				matrix[i-1][j-1]+cost,
			)
		}
	}

	return matrix[len(s1)][len(s2)]
}

// min returns the minimum of three integers
func min(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}


// sortFileInfoByPath sorts a slice of FileInfo pointers by their paths.
//
// Why duplicate this function here instead of importing from index package?
// - Avoid cyclic imports: searcher imports index, index shouldn't import searcher
// - Keep packages independent
// - Tiny function, duplication is acceptable
//
// This is a simple bubble sort for educational clarity.
// Production code might use sort.Slice for better performance on large datasets.
//
// Note: Modifies slice in-place (standard Go sorting behavior)
func sortFileInfoByPath(files []*indexer.FileInfo) {
	n := len(files)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if files[j].Path > files[j+1].Path {
				files[j], files[j+1] = files[j+1], files[j]
			}
		}
	}
}