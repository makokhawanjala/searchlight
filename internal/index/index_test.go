package index

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/makokhawanjala/searchlight/internal/indexer"
)

// Helper function to create a test FileInfo
//
// Why helper functions in tests?
// - Reduces code duplication (DRY principle)
// - Makes tests more readable: createTestFile("/path/file.txt", 1024)
// - Easy to modify test data format in one place
// - Focuses test code on what's being tested, not setup
func createTestFile(path string, size int64) *indexer.FileInfo {
	lastSlashPos := findLastSlash(path)
	name := path[lastSlashPos:]
	ext := extractExtension(path)

	return &indexer.FileInfo{
		Path:         path,
		Name:         name,
		Size:         size,
		ModifiedTime: time.Now(),
		IsDir:        false,
		Extension:    ext,
	}
}

// Helper to find last slash in path (works for both / and \)
// Returns the position AFTER the last slash
func findLastSlash(path string) int {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return i + 1
		}
	}
	return 0
}

// Helper to extract file extension
func extractExtension(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[i:]
		}
		if path[i] == '/' || path[i] == '\\' {
			break
		}
	}
	return ""
}

// TestFileIndexBasicOperations tests core Add, Get, Remove operations
//
// Why test basics first?
// - Foundation for all other functionality
// - If these fail, everything else is unreliable
// - Quick smoke test: "Does the index work at all?"
func TestFileIndexBasicOperations(t *testing.T) {
	idx := NewFileIndex()

	// Test 1: New index should be empty
	if idx.Size() != 0 {
		t.Errorf("New index should have size 0, got %d", idx.Size())
	}

	// Test 2: Add a file
	file1 := createTestFile("/home/user/document.txt", 1024)
	idx.Add(file1)

	if idx.Size() != 1 {
		t.Errorf("After adding 1 file, size should be 1, got %d", idx.Size())
	}

	// Test 3: Get the file back
	retrieved, exists := idx.Get("/home/user/document.txt")
	if !exists {
		t.Error("File should exist in index after adding")
	}
	if retrieved.Path != file1.Path {
		t.Errorf("Retrieved file path mismatch: got %s, want %s", retrieved.Path, file1.Path)
	}
	if retrieved.Size != file1.Size {
		t.Errorf("Retrieved file size mismatch: got %d, want %d", retrieved.Size, file1.Size)
	}

	// Test 4: Get non-existent file
	_, exists = idx.Get("/nonexistent/file.txt")
	if exists {
		t.Error("Non-existent file should not be found")
	}

	// Test 5: Remove the file
	removed := idx.Remove("/home/user/document.txt")
	if !removed {
		t.Error("Remove should return true for existing file")
	}
	if idx.Size() != 0 {
		t.Errorf("After removing only file, size should be 0, got %d", idx.Size())
	}

	// Test 6: Get after removal
	_, exists = idx.Get("/home/user/document.txt")
	if exists {
		t.Error("File should not exist after removal")
	}
}

// TestFileIndexAddDuplicate tests adding the same file multiple times
//
// Why test this?
// - File watcher might send duplicate events
// - Re-indexing shouldn't create duplicates
// - Updates should replace, not duplicate
func TestFileIndexAddDuplicate(t *testing.T) {
	idx := NewFileIndex()

	file1 := createTestFile("/home/user/file.txt", 1024)
	idx.Add(file1)

	// Add same path with different size (simulating file modification)
	file2 := createTestFile("/home/user/file.txt", 2048)
	idx.Add(file2)

	// Size should still be 1 (updated, not duplicated)
	if idx.Size() != 1 {
		t.Errorf("Duplicate add should not increase size, got %d", idx.Size())
	}

	// Should have the updated size
	retrieved, _ := idx.Get("/home/user/file.txt")
	if retrieved.Size != 2048 {
		t.Errorf("File should be updated to new size 2048, got %d", retrieved.Size)
	}
}

// TestFileIndexSearchPrefix tests prefix-based searching
//
// This is SearchLight's core functionality!
func TestFileIndexSearchPrefix(t *testing.T) {
	idx := NewFileIndex()

	// Add multiple files with common prefixes
	files := []*indexer.FileInfo{
		createTestFile("/home/user/documents/report.pdf", 1024),
		createTestFile("/home/user/documents/notes.txt", 512),
		createTestFile("/home/user/downloads/image.jpg", 2048),
		createTestFile("/home/admin/config.yaml", 256),
	}

	for _, file := range files {
		idx.Add(file)
	}

	// Test 1: Search for specific prefix
	results := idx.SearchPrefix("/home/user/documents")
	if len(results) != 2 {
		t.Errorf("Expected 2 results for /home/user/documents, got %d", len(results))
	}

	// Test 2: Verify results contain correct files
	foundReport := false
	foundNotes := false
	for _, file := range results {
		if file.Path == "/home/user/documents/report.pdf" {
			foundReport = true
		}
		if file.Path == "/home/user/documents/notes.txt" {
			foundNotes = true
		}
	}
	if !foundReport || !foundNotes {
		t.Error("Search results should contain both report.pdf and notes.txt")
	}

	// Test 3: Search for broader prefix
	results = idx.SearchPrefix("/home/user")
	if len(results) != 3 {
		t.Errorf("Expected 3 results for /home/user, got %d", len(results))
	}

	// Test 4: Empty prefix should return all files
	results = idx.SearchPrefix("")
	if len(results) != 4 {
		t.Errorf("Empty prefix should return all 4 files, got %d", len(results))
	}

	// Test 5: Non-existent prefix
	results = idx.SearchPrefix("/nonexistent")
	if len(results) != 0 {
		t.Errorf("Non-existent prefix should return 0 results, got %d", len(results))
	}
}

// TestFileIndexSearchByName tests name-based searching
//
// Different from prefix search: finds files by name anywhere in the filesystem
func TestFileIndexSearchByName(t *testing.T) {
	idx := NewFileIndex()

	files := []*indexer.FileInfo{
		createTestFile("/home/user/README.md", 1024),
		createTestFile("/var/www/readme.txt", 512),
		createTestFile("/home/user/documents/report.pdf", 2048),
	}

	for _, file := range files {
		idx.Add(file)
	}

	// Test 1: Case-insensitive search
	results := idx.SearchByName("readme")
	if len(results) != 2 {
		t.Errorf("Expected 2 results for 'readme' (case-insensitive), got %d", len(results))
		for _, r := range results {
			t.Logf("Found: %s (name: %s)", r.Path, r.Name)
		}
	}

	// Test 2: Partial name match
	results = idx.SearchByName("READ")
	if len(results) != 2 {
		t.Errorf("Expected 2 results for 'READ', got %d", len(results))
		for _, r := range results {
			t.Logf("Found: %s (name: %s)", r.Path, r.Name)
		}
	}

	// Test 3: No matches
	results = idx.SearchByName("nonexistent")
	if len(results) != 0 {
		t.Errorf("Expected 0 results for non-existent name, got %d", len(results))
	}

	// Test 4: Empty query
	results = idx.SearchByName("")
	if len(results) != 0 {
		t.Errorf("Empty query should return 0 results, got %d", len(results))
	}
}

// TestFileIndexSearchByExtension tests extension-based searching
//
// Common use case: "Show me all .txt files"
func TestFileIndexSearchByExtension(t *testing.T) {
	idx := NewFileIndex()

	files := []*indexer.FileInfo{
		createTestFile("/home/user/document.txt", 1024),
		createTestFile("/home/user/notes.txt", 512),
		createTestFile("/home/user/image.jpg", 2048),
		createTestFile("/home/user/script.sh", 256),
	}

	for _, file := range files {
		idx.Add(file)
	}

	// Test 1: Search with leading dot
	results := idx.SearchByExtension(".txt")
	if len(results) != 2 {
		t.Errorf("Expected 2 .txt files, got %d", len(results))
	}

	// Test 2: Search without leading dot (should still work)
	results = idx.SearchByExtension("txt")
	if len(results) != 2 {
		t.Errorf("Expected 2 txt files (without dot), got %d", len(results))
	}

	// Test 3: Different extension
	results = idx.SearchByExtension(".jpg")
	if len(results) != 1 {
		t.Errorf("Expected 1 .jpg file, got %d", len(results))
	}

	// Test 4: Non-existent extension
	results = idx.SearchByExtension(".pdf")
	if len(results) != 0 {
		t.Errorf("Expected 0 .pdf files, got %d", len(results))
	}
}

// TestFileIndexClear tests clearing all data
//
// Important for re-indexing scenarios
func TestFileIndexClear(t *testing.T) {
	idx := NewFileIndex()

	// Add some files
	for i := 0; i < 10; i++ {
		path := fmt.Sprintf("/home/user/file%d.txt", i)
		idx.Add(createTestFile(path, 1024))
	}

	if idx.Size() != 10 {
		t.Errorf("Expected 10 files before clear, got %d", idx.Size())
	}

	// Clear the index
	idx.Clear()

	// Verify empty
	if idx.Size() != 0 {
		t.Errorf("Expected 0 files after clear, got %d", idx.Size())
	}

	// Verify searches return empty
	results := idx.SearchPrefix("/home")
	if len(results) != 0 {
		t.Errorf("Expected 0 results after clear, got %d", len(results))
	}

	// Verify can add files after clear
	idx.Add(createTestFile("/new/file.txt", 512))
	if idx.Size() != 1 {
		t.Errorf("Expected 1 file after adding to cleared index, got %d", idx.Size())
	}
}

// TestFileIndexGetAll tests retrieving all indexed files
//
// Important for export/statistics features
func TestFileIndexGetAll(t *testing.T) {
	idx := NewFileIndex()

	// Test 1: Empty index should return empty slice
	all := idx.GetAll()
	if len(all) != 0 {
		t.Errorf("Empty index should return 0 files, got %d", len(all))
	}

	// Test 2: Add files and verify GetAll returns them
	files := []*indexer.FileInfo{
		createTestFile("/home/user/file1.txt", 1024),
		createTestFile("/home/user/file2.txt", 2048),
		createTestFile("/var/log/file3.log", 512),
	}

	for _, file := range files {
		idx.Add(file)
	}

	all = idx.GetAll()
	if len(all) != 3 {
		t.Errorf("Expected 3 files from GetAll, got %d", len(all))
	}

	// Test 3: Verify all files are present
	paths := make(map[string]bool)
	for _, file := range all {
		paths[file.Path] = true
	}

	for _, file := range files {
		if !paths[file.Path] {
			t.Errorf("File %s not found in GetAll results", file.Path)
		}
	}
}

// TestFileIndexContains tests path existence checking
//
// Quick check without retrieving full metadata
func TestFileIndexContains(t *testing.T) {
	idx := NewFileIndex()

	file := createTestFile("/home/user/document.txt", 1024)
	idx.Add(file)

	// Test 1: File should exist
	if !idx.Contains("/home/user/document.txt") {
		t.Error("Contains should return true for existing file")
	}

	// Test 2: Non-existent file should not exist
	if idx.Contains("/nonexistent/file.txt") {
		t.Error("Contains should return false for non-existent file")
	}

	// Test 3: After removal, file should not exist
	idx.Remove("/home/user/document.txt")
	if idx.Contains("/home/user/document.txt") {
		t.Error("Contains should return false after file removal")
	}
}

// TestFileIndexStats tests statistics gathering
//
// Important for monitoring and UI display
func TestFileIndexStats(t *testing.T) {
	idx := NewFileIndex()

	// Test 1: Empty index stats
	stats := idx.Stats()
	if stats.TotalFiles != 0 {
		t.Errorf("Empty index should have 0 files, got %d", stats.TotalFiles)
	}
	if stats.TotalSize != 0 {
		t.Errorf("Empty index should have 0 total size, got %d", stats.TotalSize)
	}

	// Test 2: Add files and verify stats
	files := []*indexer.FileInfo{
		createTestFile("/home/user/file1.txt", 1024),
		createTestFile("/home/user/file2.txt", 2048),
		createTestFile("/home/user/file3.txt", 512),
	}

	expectedTotalSize := int64(1024 + 2048 + 512)

	for _, file := range files {
		idx.Add(file)
	}

	stats = idx.Stats()
	if stats.TotalFiles != 3 {
		t.Errorf("Expected 3 total files, got %d", stats.TotalFiles)
	}
	if stats.TotalSize != expectedTotalSize {
		t.Errorf("Expected total size %d, got %d", expectedTotalSize, stats.TotalSize)
	}

	// Test 3: Stats after removal
	idx.Remove("/home/user/file1.txt")
	stats = idx.Stats()
	if stats.TotalFiles != 2 {
		t.Errorf("Expected 2 files after removal, got %d", stats.TotalFiles)
	}
	if stats.TotalSize != (expectedTotalSize - 1024) {
		t.Errorf("Expected total size %d after removal, got %d", expectedTotalSize-1024, stats.TotalSize)
	}
}

// TestFileIndexNilHandling tests defensive nil checks
//
// Prevents panics from invalid input
func TestFileIndexNilHandling(t *testing.T) {
	idx := NewFileIndex()

	// Test 1: Add nil should not panic
	idx.Add(nil)
	if idx.Size() != 0 {
		t.Errorf("Adding nil should not increase size, got %d", idx.Size())
	}

	// Test 2: Add file with empty path should not panic
	emptyPathFile := &indexer.FileInfo{
		Path: "",
		Name: "test.txt",
		Size: 1024,
	}
	idx.Add(emptyPathFile)
	if idx.Size() != 0 {
		t.Errorf("Adding file with empty path should not increase size, got %d", idx.Size())
	}

	// Test 3: Remove empty path should not panic
	removed := idx.Remove("")
	if removed {
		t.Error("Removing empty path should return false")
	}
}

// TestFileIndexConcurrentReads tests concurrent search operations
//
// Verifies thread-safety for multiple simultaneous searches
func TestFileIndexConcurrentReads(t *testing.T) {
	idx := NewFileIndex()

	// Add test data
	for i := 0; i < 100; i++ {
		path := fmt.Sprintf("/home/user/file%d.txt", i)
		idx.Add(createTestFile(path, int64(i*1024)))
	}

	// Launch multiple concurrent searches
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Perform various read operations
			for j := 0; j < 100; j++ {
				// SearchPrefix
				results := idx.SearchPrefix("/home/user")
				if len(results) != 100 {
					t.Errorf("Goroutine %d: Expected 100 results, got %d", id, len(results))
				}

				// Get
				path := fmt.Sprintf("/home/user/file%d.txt", j%100)
				if _, exists := idx.Get(path); !exists {
					t.Errorf("Goroutine %d: File %s should exist", id, path)
				}

				// Contains
				if !idx.Contains(path) {
					t.Errorf("Goroutine %d: Contains should return true for %s", id, path)
				}

				// Size
				size := idx.Size()
				if size != 100 {
					t.Errorf("Goroutine %d: Expected size 100, got %d", id, size)
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestFileIndexConcurrentWrites tests concurrent add/remove operations
//
// CRITICAL: This test should be run with -race flag to detect race conditions
func TestFileIndexConcurrentWrites(t *testing.T) {
	idx := NewFileIndex()

	var wg sync.WaitGroup
	numGoroutines := 10
	filesPerGoroutine := 100

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < filesPerGoroutine; j++ {
				path := fmt.Sprintf("/home/user/g%d_file%d.txt", id, j)
				idx.Add(createTestFile(path, 1024))
			}
		}(i)
	}

	wg.Wait()

	// Verify all files were added
	expectedSize := numGoroutines * filesPerGoroutine
	if idx.Size() != expectedSize {
		t.Errorf("Expected %d files after concurrent adds, got %d", expectedSize, idx.Size())
	}

	// Concurrent removes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < filesPerGoroutine; j++ {
				path := fmt.Sprintf("/home/user/g%d_file%d.txt", id, j)
				idx.Remove(path)
			}
		}(i)
	}

	wg.Wait()

	// Verify all files were removed
	if idx.Size() != 0 {
		t.Errorf("Expected 0 files after concurrent removes, got %d", idx.Size())
	}
}

// TestFileIndexConcurrentMixed tests mixed concurrent operations
//
// Real-world scenario: adds, removes, and searches happening simultaneously
func TestFileIndexConcurrentMixed(t *testing.T) {
	idx := NewFileIndex()

	// Pre-populate with some files
	for i := 0; i < 50; i++ {
		path := fmt.Sprintf("/home/user/initial%d.txt", i)
		idx.Add(createTestFile(path, 1024))
	}

	var wg sync.WaitGroup
	stopChan := make(chan bool)

	// Writer goroutines (adding files)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			counter := 0
			for {
				select {
				case <-stopChan:
					return
				default:
					path := fmt.Sprintf("/home/user/writer%d_file%d.txt", id, counter)
					idx.Add(createTestFile(path, 1024))
					counter++
					time.Sleep(time.Microsecond)
				}
			}
		}(i)
	}

	// Remover goroutines (removing files)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			counter := 0
			for {
				select {
				case <-stopChan:
					return
				default:
					path := fmt.Sprintf("/home/user/writer%d_file%d.txt", id%3, counter)
					idx.Remove(path)
					counter++
					time.Sleep(time.Microsecond)
				}
			}
		}(i)
	}

	// Reader goroutines (searching)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stopChan:
					return
				default:
					// Various read operations
					idx.SearchPrefix("/home/user")
					idx.SearchByName("file")
					idx.SearchByExtension(".txt")
					idx.Size()
					idx.Stats()
					time.Sleep(time.Microsecond)
				}
			}
		}()
	}

	// Let operations run for a short time
	time.Sleep(100 * time.Millisecond)

	// Signal all goroutines to stop
	close(stopChan)
	wg.Wait()

	// Verify index is still in a valid state
	size := idx.Size()
	if size < 0 {
		t.Error("Index size should not be negative after concurrent operations")
	}

	// All operations should complete without panics
	// The -race flag will catch any race conditions
}

// TestFileIndexLargeDataset tests performance with many files
//
// Ensures the index scales reasonably
func TestFileIndexLargeDataset(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large dataset test in short mode")
	}

	idx := NewFileIndex()
	numFiles := 10000

	// Add many files
	start := time.Now()
	for i := 0; i < numFiles; i++ {
		path := fmt.Sprintf("/home/user/documents/project%d/file%d.txt", i/100, i)
		idx.Add(createTestFile(path, int64(i*1024)))
	}
	addDuration := time.Since(start)

	t.Logf("Added %d files in %v", numFiles, addDuration)

	// Verify size
	if idx.Size() != numFiles {
		t.Errorf("Expected %d files, got %d", numFiles, idx.Size())
	}

	// Test search performance
	start = time.Now()
	results := idx.SearchPrefix("/home/user/documents/project5")
	searchDuration := time.Since(start)

	t.Logf("Search completed in %v, found %d results", searchDuration, len(results))

	// Search should be fast even with large dataset
	if searchDuration > time.Second {
		t.Errorf("Search took too long: %v", searchDuration)
	}
}

// BenchmarkFileIndexAdd benchmarks file addition
func BenchmarkFileIndexAdd(b *testing.B) {
	idx := NewFileIndex()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := fmt.Sprintf("/home/user/file%d.txt", i)
		idx.Add(createTestFile(path, 1024))
	}
}

// BenchmarkFileIndexSearchPrefix benchmarks prefix searching
func BenchmarkFileIndexSearchPrefix(b *testing.B) {
	idx := NewFileIndex()

	// Pre-populate index
	for i := 0; i < 10000; i++ {
		path := fmt.Sprintf("/home/user/documents/file%d.txt", i)
		idx.Add(createTestFile(path, 1024))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.SearchPrefix("/home/user/documents")
	}
}

// BenchmarkFileIndexSearchByName benchmarks name-based searching
func BenchmarkFileIndexSearchByName(b *testing.B) {
	idx := NewFileIndex()

	// Pre-populate index
	for i := 0; i < 10000; i++ {
		path := fmt.Sprintf("/home/user/file%d.txt", i)
		idx.Add(createTestFile(path, 1024))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.SearchByName("file")
	}
}

// BenchmarkFileIndexGet benchmarks metadata retrieval
func BenchmarkFileIndexGet(b *testing.B) {
	idx := NewFileIndex()

	// Pre-populate index
	paths := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		path := fmt.Sprintf("/home/user/file%d.txt", i)
		paths[i] = path
		idx.Add(createTestFile(path, 1024))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Get(paths[i%1000])
	}
}

// BenchmarkFileIndexConcurrentReads benchmarks concurrent search operations
func BenchmarkFileIndexConcurrentReads(b *testing.B) {
	idx := NewFileIndex()

	// Pre-populate index
	for i := 0; i < 10000; i++ {
		path := fmt.Sprintf("/home/user/file%d.txt", i)
		idx.Add(createTestFile(path, 1024))
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			idx.SearchPrefix("/home/user")
		}
	})
}

// BenchmarkFileIndexStats benchmarks statistics gathering
func BenchmarkFileIndexStats(b *testing.B) {
	idx := NewFileIndex()

	// Pre-populate index
	for i := 0; i < 10000; i++ {
		path := fmt.Sprintf("/home/user/file%d.txt", i)
		idx.Add(createTestFile(path, int64(i*1024)))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Stats()
	}
}
