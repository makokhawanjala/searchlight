package index

import (
	"fmt"
	"sort"
	"sync"
	"testing"
)

// TestTrieBasicOperations tests the fundamental Trie operations:
// Insert, Search, Contains, Delete
//
// Why test basic operations first?
// - Foundation for all other features
// - If these fail, everything else will fail
// - Helps isolate bugs to specific operations
func TestTrieBasicOperations(t *testing.T) {
	trie := NewTrie()

	// Test 1: Empty trie should have size 0
	if trie.Size() != 0 {
		t.Errorf("New trie should have size 0, got %d", trie.Size())
	}

	// Test 2: Insert a path and verify size increases
	trie.Insert("/home/user/documents/file.txt")
	if trie.Size() != 1 {
		t.Errorf("After inserting 1 path, size should be 1, got %d", trie.Size())
	}

	// Test 3: Contains should return true for inserted path
	if !trie.Contains("/home/user/documents/file.txt") {
		t.Error("Trie should contain the inserted path")
	}

	// Test 4: Contains should return false for non-existent path
	if trie.Contains("/home/user/documents/other.txt") {
		t.Error("Trie should not contain path that wasn't inserted")
	}

	// Test 5: Search should find the inserted path
	results := trie.Search("/home/user")
	if len(results) != 1 {
		t.Errorf("Search should return 1 result, got %d", len(results))
	}
	if results[0] != "/home/user/documents/file.txt" {
		t.Errorf("Search returned wrong path: %s", results[0])
	}

	// Test 6: Delete the path and verify size decreases
	if !trie.Delete("/home/user/documents/file.txt") {
		t.Error("Delete should return true for existing path")
	}
	if trie.Size() != 0 {
		t.Errorf("After deleting last path, size should be 0, got %d", trie.Size())
	}

	// Test 7: Contains should return false after deletion
	if trie.Contains("/home/user/documents/file.txt") {
		t.Error("Trie should not contain deleted path")
	}
}

// TestTrieMultiplePaths tests inserting and searching multiple paths
//
// Why this is important:
// - Real-world usage involves thousands of paths
// - Tests prefix matching with multiple results
// - Verifies sorting of results
func TestTrieMultiplePaths(t *testing.T) {
	trie := NewTrie()

	// Insert multiple paths with common prefixes
	paths := []string{
		"/home/user/documents/report.pdf",
		"/home/user/documents/notes.txt",
		"/home/user/downloads/image.jpg",
		"/home/user/downloads/video.mp4",
		"/home/admin/config.yaml",
	}

	for _, path := range paths {
		trie.Insert(path)
	}

	// Test 1: Size should reflect all inserted paths
	if trie.Size() != len(paths) {
		t.Errorf("Expected size %d, got %d", len(paths), trie.Size())
	}

	// Test 2: Search for common prefix should return multiple results
	results := trie.Search("/home/user/documents")
	expectedCount := 2 // report.pdf and notes.txt
	if len(results) != expectedCount {
		t.Errorf("Expected %d results for /home/user/documents, got %d", expectedCount, len(results))
	}

	// Test 3: Results should be sorted alphabetically
	if !sort.StringsAreSorted(results) {
		t.Error("Search results should be sorted alphabetically")
	}

	// Test 4: Search for broader prefix should return more results
	results = trie.Search("/home/user")
	expectedCount = 4 // All paths under /home/user
	if len(results) != expectedCount {
		t.Errorf("Expected %d results for /home/user, got %d", expectedCount, len(results))
	}

	// Test 5: Empty prefix should return all paths
	results = trie.Search("")
	if len(results) != len(paths) {
		t.Errorf("Empty prefix should return all %d paths, got %d", len(paths), len(results))
	}
}

// TestTrieDuplicateInsert tests inserting the same path multiple times
//
// Why this matters:
// - File system watcher might send duplicate events
// - Rebuilding index shouldn't create duplicates
// - Size should remain accurate
func TestTrieDuplicateInsert(t *testing.T) {
	trie := NewTrie()
	path := "/home/user/file.txt"

	// Insert same path 3 times
	trie.Insert(path)
	trie.Insert(path)
	trie.Insert(path)

	// Size should still be 1 (no duplicates)
	if trie.Size() != 1 {
		t.Errorf("Duplicate inserts should not increase size, got %d", trie.Size())
	}

	// Search should return only one result
	results := trie.Search("/home/user")
	if len(results) != 1 {
		t.Errorf("Expected 1 result despite duplicate inserts, got %d", len(results))
	}
}

// TestTrieDeleteNonExistent tests deleting a path that doesn't exist
//
// Why test negative cases?
// - Ensures robustness when file system events are out of sync
// - Verifies size doesn't become negative or incorrect
func TestTrieDeleteNonExistent(t *testing.T) {
	trie := NewTrie()
	trie.Insert("/home/user/file.txt")

	// Try to delete a path that doesn't exist
	if trie.Delete("/home/user/other.txt") {
		t.Error("Delete should return false for non-existent path")
	}

	// Size should remain unchanged
	if trie.Size() != 1 {
		t.Errorf("Size should still be 1 after failed delete, got %d", trie.Size())
	}

	// Original path should still exist
	if !trie.Contains("/home/user/file.txt") {
		t.Error("Original path should not be affected by failed delete")
	}
}

// TestTriePrefixVsCompletePath tests distinguishing between prefixes and complete paths
//
// Why this is critical:
// - "/home/user" might be a directory AND have files inside it
// - Must handle both "exact match" and "prefix match" correctly
func TestTriePrefixVsCompletePath(t *testing.T) {
	trie := NewTrie()

	// Insert paths where some are prefixes of others
	trie.Insert("/home")
	trie.Insert("/home/user")
	trie.Insert("/home/user/file.txt")

	// Test 1: All three should be found by Contains (they're complete paths)
	if !trie.Contains("/home") {
		t.Error("/home should exist as complete path")
	}
	if !trie.Contains("/home/user") {
		t.Error("/home/user should exist as complete path")
	}
	if !trie.Contains("/home/user/file.txt") {
		t.Error("/home/user/file.txt should exist as complete path")
	}

	// Test 2: Contains should return false for non-complete paths (pure prefixes)
	if trie.Contains("/ho") {
		t.Error("/ho is only a prefix, not a complete path")
	}

	// Test 3: Search for prefix should return all paths with that prefix
	results := trie.Search("/home")
	if len(results) != 3 {
		t.Errorf("Search /home should return 3 results, got %d", len(results))
	}

	// Test 4: Size should count all complete paths
	if trie.Size() != 3 {
		t.Errorf("Expected size 3, got %d", trie.Size())
	}
}

// TestTrieEmptyPath tests handling of empty string inputs
//
// Why test edge cases?
// - Prevents panics from unexpected inputs
// - Defines clear behavior for invalid input
func TestTrieEmptyPath(t *testing.T) {
	trie := NewTrie()

	// Insert empty path should be ignored
	trie.Insert("")
	if trie.Size() != 0 {
		t.Error("Empty path should not be inserted")
	}

	// Contains empty path should return false
	if trie.Contains("") {
		t.Error("Empty path should not exist")
	}

	// Delete empty path should return false
	if trie.Delete("") {
		t.Error("Delete empty path should return false")
	}

	// Search empty prefix should return all paths (or empty if trie is empty)
	trie.Insert("/home/user/file.txt")
	results := trie.Search("")
	if len(results) != 1 {
		t.Error("Search with empty prefix should return all paths")
	}
}

// TestTrieClear tests clearing all paths from the trie
//
// Why Clear is useful:
// - Full index rebuild scenarios
// - Switching to a different directory
// - Memory cleanup
func TestTrieClear(t *testing.T) {
	trie := NewTrie()

	// Insert some paths
	trie.Insert("/home/user/file1.txt")
	trie.Insert("/home/user/file2.txt")
	trie.Insert("/home/admin/config.yaml")

	// Verify paths exist
	if trie.Size() != 3 {
		t.Errorf("Expected size 3 before clear, got %d", trie.Size())
	}

	// Clear the trie
	trie.Clear()

	// Verify trie is empty
	if trie.Size() != 0 {
		t.Errorf("Size should be 0 after clear, got %d", trie.Size())
	}

	// Verify paths no longer exist
	if trie.Contains("/home/user/file1.txt") {
		t.Error("Path should not exist after clear")
	}

	// Verify search returns empty
	results := trie.Search("/home")
	if len(results) != 0 {
		t.Errorf("Search should return 0 results after clear, got %d", len(results))
	}

	// Verify we can insert new paths after clear
	trie.Insert("/new/path.txt")
	if trie.Size() != 1 {
		t.Error("Should be able to insert paths after clear")
	}
}

// TestTriePrefixCount tests counting paths with a given prefix
//
// Why PrefixCount is useful:
// - Show "123 results" before rendering them
// - Decide whether to show autocomplete dropdown
func TestTriePrefixCount(t *testing.T) {
	trie := NewTrie()

	paths := []string{
		"/home/user/documents/file1.txt",
		"/home/user/documents/file2.txt",
		"/home/user/downloads/image.jpg",
		"/home/admin/config.yaml",
	}

	for _, path := range paths {
		trie.Insert(path)
	}

	// Test 1: Count with specific prefix
	count := trie.PrefixCount("/home/user/documents")
	if count != 2 {
		t.Errorf("Expected 2 paths with prefix /home/user/documents, got %d", count)
	}

	// Test 2: Count with broader prefix
	count = trie.PrefixCount("/home/user")
	if count != 3 {
		t.Errorf("Expected 3 paths with prefix /home/user, got %d", count)
	}

	// Test 3: Count with empty prefix (should return total size)
	count = trie.PrefixCount("")
	if count != len(paths) {
		t.Errorf("Empty prefix should return total size %d, got %d", len(paths), count)
	}

	// Test 4: Count with non-existent prefix
	count = trie.PrefixCount("/nonexistent")
	if count != 0 {
		t.Errorf("Non-existent prefix should return 0, got %d", count)
	}
}

// TestTrieHasPrefix tests checking if any paths start with a prefix
//
// Why HasPrefix is useful:
// - Faster than PrefixCount when you only need yes/no
// - Early exit optimization
func TestTrieHasPrefix(t *testing.T) {
	trie := NewTrie()

	trie.Insert("/home/user/file.txt")
	trie.Insert("/var/log/system.log")

	// Test 1: Existing prefix
	if !trie.HasPrefix("/home") {
		t.Error("Should have paths with prefix /home")
	}

	// Test 2: Non-existent prefix
	if trie.HasPrefix("/nonexistent") {
		t.Error("Should not have paths with prefix /nonexistent")
	}

	// Test 3: Empty prefix with non-empty trie
	if !trie.HasPrefix("") {
		t.Error("Empty prefix should return true for non-empty trie")
	}

	// Test 4: Empty prefix with empty trie
	emptyTrie := NewTrie()
	if emptyTrie.HasPrefix("") {
		t.Error("Empty prefix should return false for empty trie")
	}
}

// TestTrieUnicodeSupport tests handling of non-ASCII characters
//
// Why test Unicode?
// - Modern file systems support Unicode filenames
// - Users worldwide use non-ASCII characters
// - Go's rune type should handle this correctly, but we verify it
func TestTrieUnicodeSupport(t *testing.T) {
	trie := NewTrie()

	// Test with various Unicode characters
	paths := []string{
		"/home/用户/文档/报告.txt",                   // Chinese
		"/home/usuario/documentos/archivo.pdf", // Spanish
		"/home/пользователь/файл.doc",          // Russian
		"/home/user/émoji/😀.txt",               // Emoji
	}

	// Insert all paths
	for _, path := range paths {
		trie.Insert(path)
	}

	// Verify all paths were inserted
	if trie.Size() != len(paths) {
		t.Errorf("Expected size %d, got %d", len(paths), trie.Size())
	}

	// Verify each path can be found
	for _, path := range paths {
		if !trie.Contains(path) {
			t.Errorf("Trie should contain Unicode path: %s", path)
		}
	}

	// Search with Unicode prefix
	results := trie.Search("/home/用户")
	if len(results) != 1 {
		t.Errorf("Search with Unicode prefix should return 1 result, got %d", len(results))
	}
}

// TestTrieConcurrentAccess tests thread-safety of the Trie
//
// Why test concurrency?
// - SearchLight will have multiple goroutines accessing the Trie simultaneously
// - File watcher updating while searches are happening
// - Must prevent race conditions and data corruption
//
// This test uses the race detector: go test -race
func TestTrieConcurrentAccess(t *testing.T) {
	trie := NewTrie()

	// Pre-populate with some paths
	for i := 0; i < 100; i++ {
		trie.Insert(fmt.Sprintf("/path/to/file%d.txt", i))
	}

	// Use WaitGroup to coordinate goroutines
	var wg sync.WaitGroup
	numGoroutines := 10
	operationsPerGoroutine := 100

	// Test 1: Concurrent reads (searches)
	// Multiple goroutines searching simultaneously should be safe
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				results := trie.Search("/path/to")
				if len(results) == 0 {
					t.Error("Search should return results")
				}
			}
		}()
	}
	wg.Wait()

	// Test 2: Concurrent writes (inserts)
	// Multiple goroutines inserting should be safe and not duplicate
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutineID := i
		go func() {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				path := fmt.Sprintf("/concurrent/%d/%d.txt", goroutineID, j)
				trie.Insert(path)
			}
		}()
	}
	wg.Wait()

	// Verify correct number of paths (original 100 + new concurrent inserts)
	expectedSize := 100 + (numGoroutines * operationsPerGoroutine)
	if trie.Size() != expectedSize {
		t.Errorf("Expected size %d after concurrent inserts, got %d", expectedSize, trie.Size())
	}

	// Test 3: Mixed reads and writes
	// Some goroutines reading, some writing - should all be safe
	for i := 0; i < numGoroutines; i++ {
		wg.Add(2) // One reader, one writer

		// Reader goroutine
		go func() {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				_ = trie.Search("/path")
				_ = trie.Contains("/path/to/file1.txt")
				_ = trie.PrefixCount("/path")
			}
		}()

		// Writer goroutine
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine; j++ {
				path := fmt.Sprintf("/mixed/%d/%d.txt", id, j)
				trie.Insert(path)
			}
		}(i)
	}
	wg.Wait()

	// Verify trie is still consistent (no corruption)
	size := trie.Size()
	allPaths := trie.Search("")
	if len(allPaths) != size {
		t.Errorf("Inconsistent state: size=%d but Search returns %d paths", size, len(allPaths))
	}
}

// TestTrieDeleteNodeCleanup tests that Delete properly cleans up unnecessary nodes
//
// Why test cleanup?
// - Memory efficiency: prevent memory leaks
// - Verify tree structure remains optimal after deletions
func TestTrieDeleteNodeCleanup(t *testing.T) {
	trie := NewTrie()

	// Insert paths that share prefixes
	trie.Insert("/home/user/temp/file1.txt")
	trie.Insert("/home/user/temp/file2.txt")

	// Delete one file - "temp" node should remain (file2 still uses it)
	trie.Delete("/home/user/temp/file1.txt")

	// Verify file2 still exists
	if !trie.Contains("/home/user/temp/file2.txt") {
		t.Error("file2 should still exist after deleting file1")
	}

	// Delete the second file - "temp" node should now be cleaned up
	trie.Delete("/home/user/temp/file2.txt")

	// Verify the prefix no longer exists (cleanup worked)
	if trie.HasPrefix("/home/user/temp") {
		t.Error("After deleting all files, the temp prefix should be cleaned up")
	}

	// Size should be 0
	if trie.Size() != 0 {
		t.Errorf("Size should be 0 after deleting all paths, got %d", trie.Size())
	}
}

// TestTrieLargeDataset tests performance with realistic file counts
//
// Why test with large datasets?
// - Typical computers have 100k-1M files
// - Ensures operations remain fast at scale
// - Identifies performance issues early
func TestTrieLargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	trie := NewTrie()
	numFiles := 10000

	// Insert many files
	for i := 0; i < numFiles; i++ {
		path := fmt.Sprintf("/home/user/documents/folder%d/file%d.txt", i%100, i)
		trie.Insert(path)
	}

	// Verify size
	if trie.Size() != numFiles {
		t.Errorf("Expected size %d, got %d", numFiles, trie.Size())
	}

	// Test search performance - should be fast even with 10k files
	results := trie.Search("/home/user/documents/folder5")
	if len(results) == 0 {
		t.Error("Search should return results")
	}

	// Test that we can still find specific files
	if !trie.Contains("/home/user/documents/folder50/file5050.txt") {
		t.Error("Should be able to find specific file in large dataset")
	}
}

// TestTrieSearchSortedResults verifies that search results are always sorted
//
// Why sorting matters?
// - Predictable output for users
// - Easier testing and debugging
// - Better UX in search interface
func TestTrieSearchSortedResults(t *testing.T) {
	trie := NewTrie()

	// Insert paths in random order
	paths := []string{
		"/zebra.txt",
		"/apple.txt",
		"/monkey.txt",
		"/banana.txt",
	}

	for _, path := range paths {
		trie.Insert(path)
	}

	// Search should return sorted results
	results := trie.Search("/")

	// Check if sorted
	if !sort.StringsAreSorted(results) {
		t.Errorf("Results should be sorted, got: %v", results)
	}

	// Verify expected order
	expected := []string{"/apple.txt", "/banana.txt", "/monkey.txt", "/zebra.txt"}
	if len(results) != len(expected) {
		t.Errorf("Expected %d results, got %d", len(expected), len(results))
	}

	for i, path := range expected {
		if results[i] != path {
			t.Errorf("Result[%d]: expected %s, got %s", i, path, results[i])
		}
	}
}

// BenchmarkTrieInsert measures insert performance
//
// Why benchmark?
// - Ensures operations stay fast as code evolves
// - Identifies performance regressions
// - Helps decide on optimization priorities
func BenchmarkTrieInsert(b *testing.B) {
	trie := NewTrie()
	path := "/home/user/documents/report.pdf"

	b.ResetTimer() // Don't count setup time
	for i := 0; i < b.N; i++ {
		trie.Insert(path)
	}
}

// BenchmarkTrieSearch measures search performance
func BenchmarkTrieSearch(b *testing.B) {
	trie := NewTrie()

	// Setup: populate trie with 1000 paths
	for i := 0; i < 1000; i++ {
		path := fmt.Sprintf("/home/user/folder%d/file%d.txt", i%10, i)
		trie.Insert(path)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = trie.Search("/home/user")
	}
}

// BenchmarkTrieContains measures existence check performance
func BenchmarkTrieContains(b *testing.B) {
	trie := NewTrie()

	// Setup: populate trie
	for i := 0; i < 1000; i++ {
		path := fmt.Sprintf("/home/user/file%d.txt", i)
		trie.Insert(path)
	}

	testPath := "/home/user/file500.txt"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = trie.Contains(testPath)
	}
}

// BenchmarkTrieDelete measures deletion performance
func BenchmarkTrieDelete(b *testing.B) {
	// Note: This benchmark recreates the trie each iteration
	// because Delete modifies the trie
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		trie := NewTrie()
		path := "/home/user/documents/report.pdf"
		trie.Insert(path)
		b.StartTimer()

		trie.Delete(path)
	}
}
