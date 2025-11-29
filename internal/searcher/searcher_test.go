package searcher

import (
	"testing"
	"time"

	"github.com/makokhawanjala/searchlight/internal/index"
	"github.com/makokhawanjala/searchlight/internal/indexer"
)

// TestNewSearcher verifies that NewSearcher creates a valid Searcher instance.
//
// What we're testing:
// - Searcher is created successfully
// - Internal index reference is set correctly
// - No nil pointer issues
//
// Why this test matters:
// - Ensures constructor works as expected
// - Prevents nil pointer panics
// - Documents expected initialization behavior
func TestNewSearcher(t *testing.T) {
	// Create a FileIndex
	idx := index.NewFileIndex()

	// Create a Searcher
	searcher := NewSearcher(idx)

	// Verify searcher was created
	if searcher == nil {
		t.Fatal("NewSearcher returned nil")
	}

	// Verify internal index is set
	if searcher.index == nil {
		t.Error("Searcher.index is nil")
	}
}

// TestSearchByPrefix tests prefix-based searching functionality.
//
// Test Strategy:
// 1. Create index with known files
// 2. Search with various prefixes
// 3. Verify correct files are returned
// 4. Test edge cases (empty prefix, no matches)
//
// Why test this thoroughly?
// - Prefix search is the core functionality of SearchLight
// - Must work correctly for the application to be useful
// - Edge cases can cause crashes if not handled
func TestSearchByPrefix(t *testing.T) {
	// Setup: Create index and add test files
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	// Add test files with various paths
	// Why these specific paths?
	// - Cover different directory depths
	// - Include common prefixes ("doc" prefix)
	// - Include files without common prefix
	testFiles := []*indexer.FileInfo{
		{Path: "/home/user/documents/file1.txt", Name: "file1.txt", Size: 100, Extension: ".txt"},
		{Path: "/home/user/documents/file2.txt", Name: "file2.txt", Size: 200, Extension: ".txt"},
		{Path: "/home/user/docs/readme.md", Name: "readme.md", Size: 50, Extension: ".md"},
		{Path: "/var/log/syslog", Name: "syslog", Size: 1000, Extension: ""},
		{Path: "/var/log/messages", Name: "messages", Size: 2000, Extension: ""},
	}

	for _, file := range testFiles {
		idx.Add(file)
	}

	// Test Case 1: Search with common prefix
	t.Run("CommonPrefix", func(t *testing.T) {
		results := searcher.SearchByPrefix("/home/user/doc")

		// Should match both "documents" and "docs" directories
		expectedCount := 3 // file1.txt, file2.txt, readme.md
		if len(results) != expectedCount {
			t.Errorf("Expected %d results, got %d", expectedCount, len(results))
		}

		// Verify all results start with the prefix
		for _, file := range results {
			if len(file.Path) < len("/home/user/doc") || file.Path[:len("/home/user/doc")] != "/home/user/doc" {
				t.Errorf("Result %s does not start with prefix /home/user/doc", file.Path)
			}
		}
	})

	// Test Case 2: Search with exact directory path
	t.Run("ExactDirectory", func(t *testing.T) {
		results := searcher.SearchByPrefix("/var/log/")

		expectedCount := 2 // syslog, messages
		if len(results) != expectedCount {
			t.Errorf("Expected %d results, got %d", expectedCount, len(results))
		}
	})

	// Test Case 3: Empty prefix should return all files
	t.Run("EmptyPrefix", func(t *testing.T) {
		results := searcher.SearchByPrefix("")

		expectedCount := len(testFiles)
		if len(results) != expectedCount {
			t.Errorf("Empty prefix should return all files. Expected %d, got %d", expectedCount, len(results))
		}
	})

	// Test Case 4: No matches
	t.Run("NoMatches", func(t *testing.T) {
		results := searcher.SearchByPrefix("/nonexistent/path")

		if len(results) != 0 {
			t.Errorf("Expected 0 results for nonexistent path, got %d", len(results))
		}
	})

	// Test Case 5: Single character prefix
	t.Run("SingleCharacter", func(t *testing.T) {
		results := searcher.SearchByPrefix("/h")

		// Should match all files starting with /h (home directory)
		expectedCount := 3 // All files in /home/user/
		if len(results) != expectedCount {
			t.Errorf("Expected %d results, got %d", expectedCount, len(results))
		}
	})
}

// TestSearchByName tests filename-based searching.
//
// Why separate from prefix search tests?
// - Different search strategy (substring vs prefix)
// - Case-insensitive behavior needs testing
// - Different performance characteristics
func TestSearchByName(t *testing.T) {
	// Setup
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	// Add files with various names for thorough testing
	testFiles := []*indexer.FileInfo{
		{Path: "/home/user/README.md", Name: "README.md", Size: 100, Extension: ".md"},
		{Path: "/var/www/readme.txt", Name: "readme.txt", Size: 50, Extension: ".txt"},
		{Path: "/home/user/config.yaml", Name: "config.yaml", Size: 200, Extension: ".yaml"},
		{Path: "/etc/systemd/system.conf", Name: "system.conf", Size: 150, Extension: ".conf"},
	}

	for _, file := range testFiles {
		idx.Add(file)
	}

	// Test Case 1: Case-insensitive matching
	t.Run("CaseInsensitive", func(t *testing.T) {
		// Search for "readme" should match both "README.md" and "readme.txt"
		results := searcher.SearchByName("readme")

		expectedCount := 2
		if len(results) != expectedCount {
			t.Errorf("Expected %d results, got %d", expectedCount, len(results))
		}

		// Verify both files were found
		foundUpper := false
		foundLower := false
		for _, file := range results {
			if file.Name == "README.md" {
				foundUpper = true
			}
			if file.Name == "readme.txt" {
				foundLower = true
			}
		}

		if !foundUpper || !foundLower {
			t.Error("Case-insensitive search did not match both uppercase and lowercase filenames")
		}
	})

	// Test Case 2: Partial name matching
	t.Run("PartialMatch", func(t *testing.T) {
		// Search for "conf" should match both "config.yaml" and "system.conf"
		results := searcher.SearchByName("conf")

		expectedCount := 2
		if len(results) != expectedCount {
			t.Errorf("Expected %d results, got %d", expectedCount, len(results))
		}
	})

	// Test Case 3: Extension as query
	t.Run("ExtensionQuery", func(t *testing.T) {
		// Searching by name can also match extensions
		results := searcher.SearchByName(".md")

		// Should find README.md
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}

		if len(results) > 0 && results[0].Name != "README.md" {
			t.Errorf("Expected README.md, got %s", results[0].Name)
		}
	})

	// Test Case 4: Empty query
	t.Run("EmptyQuery", func(t *testing.T) {
		results := searcher.SearchByName("")

		// Empty query should return empty results (not all files)
		if len(results) != 0 {
			t.Errorf("Empty query should return 0 results, got %d", len(results))
		}
	})

	// Test Case 5: No matches
	t.Run("NoMatches", func(t *testing.T) {
		results := searcher.SearchByName("nonexistent")

		if len(results) != 0 {
			t.Errorf("Expected 0 results, got %d", len(results))
		}
	})
}

// TestSearchByExtension tests extension-based file searching.
//
// Extension search is common in file management:
// - "Show all PDFs"
// - "Find all images"
// - "List all code files"
func TestSearchByExtension(t *testing.T) {
	// Setup
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	// Add files with various extensions
	testFiles := []*indexer.FileInfo{
		{Path: "/home/user/doc.txt", Name: "doc.txt", Size: 100, Extension: ".txt"},
		{Path: "/home/user/notes.txt", Name: "notes.txt", Size: 200, Extension: ".txt"},
		{Path: "/home/user/readme.md", Name: "readme.md", Size: 50, Extension: ".md"},
		{Path: "/var/log/syslog", Name: "syslog", Size: 1000, Extension: ""}, // No extension
	}

	for _, file := range testFiles {
		idx.Add(file)
	}

	// Test Case 1: Search with dot
	t.Run("WithDot", func(t *testing.T) {
		results := searcher.SearchByExtension(".txt")

		expectedCount := 2 // doc.txt, notes.txt
		if len(results) != expectedCount {
			t.Errorf("Expected %d results, got %d", expectedCount, len(results))
		}

		// Verify all results have .txt extension
		for _, file := range results {
			if file.Extension != ".txt" {
				t.Errorf("Expected .txt extension, got %s", file.Extension)
			}
		}
	})

	// Test Case 2: Search without dot (should auto-add)
	t.Run("WithoutDot", func(t *testing.T) {
		results := searcher.SearchByExtension("md")

		expectedCount := 1 // readme.md
		if len(results) != expectedCount {
			t.Errorf("Expected %d results, got %d", expectedCount, len(results))
		}

		if len(results) > 0 && results[0].Extension != ".md" {
			t.Errorf("Expected .md extension, got %s", results[0].Extension)
		}
	})

	// Test Case 3: Empty extension
	t.Run("EmptyExtension", func(t *testing.T) {
		results := searcher.SearchByExtension("")

		// Empty extension should return 0 results
		if len(results) != 0 {
			t.Errorf("Empty extension should return 0 results, got %d", len(results))
		}
	})

	// Test Case 4: Non-existent extension
	t.Run("NonExistent", func(t *testing.T) {
		results := searcher.SearchByExtension(".pdf")

		if len(results) != 0 {
			t.Errorf("Expected 0 results for non-existent extension, got %d", len(results))
		}
	})
}

// TestSearchMultipleExtensions tests searching for multiple file extensions at once.
//
// This is a convenience method that's very useful for:
// - Finding all image files (.jpg, .png, .gif)
// - Finding all documents (.pdf, .doc, .txt)
// - Finding all code files (.go, .py, .js)
func TestSearchMultipleExtensions(t *testing.T) {
	// Setup
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	// Add files with various extensions
	testFiles := []*indexer.FileInfo{
		{Path: "/images/photo1.jpg", Name: "photo1.jpg", Size: 1000, Extension: ".jpg"},
		{Path: "/images/photo2.png", Name: "photo2.png", Size: 2000, Extension: ".png"},
		{Path: "/images/photo3.gif", Name: "photo3.gif", Size: 500, Extension: ".gif"},
		{Path: "/docs/manual.pdf", Name: "manual.pdf", Size: 5000, Extension: ".pdf"},
		{Path: "/docs/readme.txt", Name: "readme.txt", Size: 100, Extension: ".txt"},
	}

	for _, file := range testFiles {
		idx.Add(file)
	}

	// Test Case 1: Multiple image extensions
	t.Run("ImageExtensions", func(t *testing.T) {
		results := searcher.SearchMultipleExtensions([]string{".jpg", ".png", ".gif"})

		expectedCount := 3 // All image files
		if len(results) != expectedCount {
			t.Errorf("Expected %d results, got %d", expectedCount, len(results))
		}

		// Verify all results are images
		validExts := map[string]bool{".jpg": true, ".png": true, ".gif": true}
		for _, file := range results {
			if !validExts[file.Extension] {
				t.Errorf("Unexpected extension %s in image results", file.Extension)
			}
		}
	})

	// Test Case 2: Mixed extensions with and without dots
	t.Run("MixedDotFormat", func(t *testing.T) {
		// Mix of ".pdf" and "txt" (without dot)
		results := searcher.SearchMultipleExtensions([]string{".pdf", "txt"})

		expectedCount := 2 // manual.pdf, readme.txt
		if len(results) != expectedCount {
			t.Errorf("Expected %d results, got %d", expectedCount, len(results))
		}
	})

	// Test Case 3: Empty extensions slice
	t.Run("EmptySlice", func(t *testing.T) {
		results := searcher.SearchMultipleExtensions([]string{})

		if len(results) != 0 {
			t.Errorf("Empty extensions slice should return 0 results, got %d", len(results))
		}
	})

	// Test Case 4: Non-existent extensions
	t.Run("NonExistent", func(t *testing.T) {
		results := searcher.SearchMultipleExtensions([]string{".mp3", ".wav"})

		if len(results) != 0 {
			t.Errorf("Expected 0 results for non-existent extensions, got %d", len(results))
		}
	})

	// Test Case 5: Single extension (should work like SearchByExtension)
	t.Run("SingleExtension", func(t *testing.T) {
		results := searcher.SearchMultipleExtensions([]string{".jpg"})

		expectedCount := 1 // photo1.jpg
		if len(results) != expectedCount {
			t.Errorf("Expected %d results, got %d", expectedCount, len(results))
		}
	})

	// Test Case 6: Duplicate extensions (should not return duplicates)
	t.Run("DuplicateExtensions", func(t *testing.T) {
		results := searcher.SearchMultipleExtensions([]string{".jpg", ".jpg", ".png"})

		expectedCount := 2 // photo1.jpg (once), photo2.png
		if len(results) != expectedCount {
			t.Errorf("Expected %d unique results, got %d", expectedCount, len(results))
		}

		// Verify no duplicate paths
		seen := make(map[string]bool)
		for _, file := range results {
			if seen[file.Path] {
				t.Errorf("Duplicate path found: %s", file.Path)
			}
			seen[file.Path] = true
		}
	})
}

// TestGetFileByPath tests retrieving a specific file by its exact path.
//
// Why test this?
// - Validates O(1) lookup works correctly
// - Ensures proper handling of non-existent files
// - Verifies "comma ok" idiom implementation
func TestGetFileByPath(t *testing.T) {
	// Setup
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	// Add test file
	testFile := &indexer.FileInfo{
		Path:      "/home/user/test.txt",
		Name:      "test.txt",
		Size:      100,
		Extension: ".txt",
	}
	idx.Add(testFile)

	// Test Case 1: Existing file
	t.Run("ExistingFile", func(t *testing.T) {
		file, found := searcher.GetFileByPath("/home/user/test.txt")

		if !found {
			t.Fatal("Expected file to be found")
		}

		if file == nil {
			t.Fatal("File is nil despite found=true")
		}

		// Verify it's the correct file
		if file.Path != testFile.Path {
			t.Errorf("Expected path %s, got %s", testFile.Path, file.Path)
		}
		if file.Size != testFile.Size {
			t.Errorf("Expected size %d, got %d", testFile.Size, file.Size)
		}
	})

	// Test Case 2: Non-existent file
	t.Run("NonExistentFile", func(t *testing.T) {
		file, found := searcher.GetFileByPath("/nonexistent/file.txt")

		if found {
			t.Error("Expected found=false for non-existent file")
		}

		if file != nil {
			t.Error("Expected nil file for non-existent path")
		}
	})

	// Test Case 3: Empty path
	t.Run("EmptyPath", func(t *testing.T) {
		file, found := searcher.GetFileByPath("")

		if found {
			t.Error("Expected found=false for empty path")
		}

		if file != nil {
			t.Error("Expected nil file for empty path")
		}
	})
}

// TestContainsPath tests the existence check functionality.
//
// Why test this separately from GetFileByPath?
// - ContainsPath is optimized for boolean checks
// - Different use case (existence vs retrieval)
// - Should have identical behavior to GetFileByPath for existence
func TestContainsPath(t *testing.T) {
	// Setup
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	// Add test file
	testFile := &indexer.FileInfo{
		Path: "/home/user/test.txt",
		Name: "test.txt",
	}
	idx.Add(testFile)

	// Test Case 1: Existing file
	t.Run("ExistingFile", func(t *testing.T) {
		if !searcher.ContainsPath("/home/user/test.txt") {
			t.Error("Expected path to exist")
		}
	})

	// Test Case 2: Non-existent file
	t.Run("NonExistentFile", func(t *testing.T) {
		if searcher.ContainsPath("/nonexistent/file.txt") {
			t.Error("Expected path to not exist")
		}
	})

	// Test Case 3: Empty path
	t.Run("EmptyPath", func(t *testing.T) {
		if searcher.ContainsPath("") {
			t.Error("Expected empty path to not exist")
		}
	})
}

// TestCount tests the file count functionality.
//
// Why test this?
// - Verifies count stays in sync with Add/Remove operations
// - Ensures thread-safe counting (though we don't test concurrency here)
func TestCount(t *testing.T) {
	// Setup
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	// Test Case 1: Empty index
	t.Run("EmptyIndex", func(t *testing.T) {
		count := searcher.Count()
		if count != 0 {
			t.Errorf("Expected count 0 for empty index, got %d", count)
		}
	})

	// Test Case 2: After adding files
	t.Run("AfterAdding", func(t *testing.T) {
		idx.Add(&indexer.FileInfo{Path: "/file1.txt", Name: "file1.txt"})
		idx.Add(&indexer.FileInfo{Path: "/file2.txt", Name: "file2.txt"})
		idx.Add(&indexer.FileInfo{Path: "/file3.txt", Name: "file3.txt"})

		count := searcher.Count()
		expectedCount := 3
		if count != expectedCount {
			t.Errorf("Expected count %d, got %d", expectedCount, count)
		}
	})

	// Test Case 3: After removing a file
	t.Run("AfterRemoving", func(t *testing.T) {
		idx.Remove("/file2.txt")

		count := searcher.Count()
		expectedCount := 2
		if count != expectedCount {
			t.Errorf("Expected count %d after removal, got %d", expectedCount, count)
		}
	})
}

// TestSearchAll tests retrieving all indexed files.
//
// Why test this?
// - Verifies we can get complete index contents
// - Ensures results are sorted
// - Validates behavior with empty index
func TestSearchAll(t *testing.T) {
	// Setup
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	// Test Case 1: Empty index
	t.Run("EmptyIndex", func(t *testing.T) {
		results := searcher.SearchAll()

		if len(results) != 0 {
			t.Errorf("Expected 0 results from empty index, got %d", len(results))
		}

		// Verify it returns a slice, not nil
		if results == nil {
			t.Error("SearchAll should return empty slice, not nil")
		}
	})

	// Test Case 2: With files
	t.Run("WithFiles", func(t *testing.T) {
		// Add files in non-alphabetical order
		testFiles := []*indexer.FileInfo{
			{Path: "/zebra.txt", Name: "zebra.txt"},
			{Path: "/apple.txt", Name: "apple.txt"},
			{Path: "/banana.txt", Name: "banana.txt"},
		}

		for _, file := range testFiles {
			idx.Add(file)
		}

		results := searcher.SearchAll()

		expectedCount := 3
		if len(results) != expectedCount {
			t.Errorf("Expected %d results, got %d", expectedCount, len(results))
		}

		// Verify results are sorted
		for i := 0; i < len(results)-1; i++ {
			if results[i].Path > results[i+1].Path {
				t.Errorf("Results not sorted: %s comes before %s", results[i].Path, results[i+1].Path)
			}
		}
	})
}

// TestStats tests the statistics functionality.
//
// Why test this?
// - Ensures stats accurately reflect index contents
// - Validates total size calculation
// - Verifies stats update as files are added/removed
func TestStats(t *testing.T) {
	// Setup
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	// Test Case 1: Empty index
	t.Run("EmptyIndex", func(t *testing.T) {
		stats := searcher.Stats()

		if stats.TotalFiles != 0 {
			t.Errorf("Expected TotalFiles=0, got %d", stats.TotalFiles)
		}

		if stats.TotalSize != 0 {
			t.Errorf("Expected TotalSize=0, got %d", stats.TotalSize)
		}
	})

	// Test Case 2: With files
	t.Run("WithFiles", func(t *testing.T) {
		// Add files with known sizes
		idx.Add(&indexer.FileInfo{Path: "/file1.txt", Name: "file1.txt", Size: 100})
		idx.Add(&indexer.FileInfo{Path: "/file2.txt", Name: "file2.txt", Size: 200})
		idx.Add(&indexer.FileInfo{Path: "/file3.txt", Name: "file3.txt", Size: 300})

		stats := searcher.Stats()

		expectedFiles := 3
		if stats.TotalFiles != expectedFiles {
			t.Errorf("Expected TotalFiles=%d, got %d", expectedFiles, stats.TotalFiles)
		}

		expectedSize := int64(600) // 100 + 200 + 300
		if stats.TotalSize != expectedSize {
			t.Errorf("Expected TotalSize=%d, got %d", expectedSize, stats.TotalSize)
		}
	})

	// Test Case 3: After removing files
	t.Run("AfterRemoval", func(t *testing.T) {
		idx.Remove("/file2.txt") // Remove 200 byte file

		stats := searcher.Stats()

		expectedFiles := 2
		if stats.TotalFiles != expectedFiles {
			t.Errorf("Expected TotalFiles=%d after removal, got %d", expectedFiles, stats.TotalFiles)
		}

		expectedSize := int64(400) // 100 + 300
		if stats.TotalSize != expectedSize {
			t.Errorf("Expected TotalSize=%d after removal, got %d", expectedSize, stats.TotalSize)
		}
	})
}

// TestSearcherWithRealFileInfo tests the Searcher with realistic FileInfo data.
//
// Why this test?
// - Validates Searcher works with complete FileInfo structs
// - Tests with actual timestamps and realistic data
// - Ensures no issues with FileInfo methods (HumanSize, etc.)
func TestSearcherWithRealFileInfo(t *testing.T) {
	// Setup
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	// Create realistic FileInfo with all fields populated
	now := time.Now()
	testFiles := []*indexer.FileInfo{
		{
			Path:         "/home/user/documents/report.pdf",
			Name:         "report.pdf",
			Size:         1024 * 1024, // 1 MB
			ModifiedTime: now.Add(-24 * time.Hour),
			IsDir:        false,
			Extension:    ".pdf",
		},
		{
			Path:         "/home/user/images/photo.jpg",
			Name:         "photo.jpg",
			Size:         2 * 1024 * 1024, // 2 MB
			ModifiedTime: now,
			IsDir:        false,
			Extension:    ".jpg",
		},
		{
			Path:         "/home/user/projects",
			Name:         "projects",
			Size:         4096, // Directory size
			ModifiedTime: now.Add(-48 * time.Hour),
			IsDir:        true,
			Extension:    "",
		},
	}

	for _, file := range testFiles {
		idx.Add(file)
	}

	// Test that all FileInfo fields are preserved
	t.Run("PreservesAllFields", func(t *testing.T) {
		file, found := searcher.GetFileByPath("/home/user/documents/report.pdf")

		if !found {
			t.Fatal("File not found")
		}

		// Verify all fields
		if file.Name != "report.pdf" {
			t.Errorf("Name mismatch: got %s", file.Name)
		}
		if file.Size != 1024*1024 {
			t.Errorf("Size mismatch: got %d", file.Size)
		}
		if file.IsDir != false {
			t.Errorf("IsDir mismatch: got %v", file.IsDir)
		}
		if file.Extension != ".pdf" {
			t.Errorf("Extension mismatch: got %s", file.Extension)
		}

		// Test FileInfo methods
		humanSize := file.HumanSize()
		if humanSize == "" {
			t.Error("HumanSize returned empty string")
		}

		formattedTime := file.FormattedModTime()
		if formattedTime == "" {
			t.Error("FormattedModTime returned empty string")
		}
	})

	// Test searching directories
	t.Run("SearchDirectories", func(t *testing.T) {
		results := searcher.SearchByPrefix("/home/user/projects")

		expectedCount := 1
		if len(results) != expectedCount {
			t.Errorf("Expected %d directory result, got %d", expectedCount, len(results))
		}

		if len(results) > 0 && !results[0].IsDir {
			t.Error("Expected IsDir=true for directory")
		}
	})
}

// TestSearcherResultsSorted tests that all search methods return sorted results.
//
// Why is sorting important?
// - Predictable output for users
// - Easier to find files in long result lists
// - Consistent behavior across different search types
// - Essential for testing (deterministic results)
func TestSearcherResultsSorted(t *testing.T) {
	// Setup
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	// Add files in random order
	testFiles := []*indexer.FileInfo{
		{Path: "/zebra/file.txt", Name: "file.txt"},
		{Path: "/alpha/file.txt", Name: "file.txt"},
		{Path: "/beta/file.txt", Name: "file.txt"},
		{Path: "/gamma/file.txt", Name: "file.txt"},
	}

	for _, file := range testFiles {
		idx.Add(file)
	}

	// Helper function to verify sorting
	verifySorted := func(t *testing.T, results []*indexer.FileInfo) {
		for i := 0; i < len(results)-1; i++ {
			if results[i].Path > results[i+1].Path {
				t.Errorf("Results not sorted: %s > %s", results[i].Path, results[i+1].Path)
			}
		}
	}

	// Test all search methods return sorted results
	t.Run("SearchByPrefix", func(t *testing.T) {
		results := searcher.SearchByPrefix("/")
		verifySorted(t, results)
	})

	t.Run("SearchByName", func(t *testing.T) {
		results := searcher.SearchByName("file")
		verifySorted(t, results)
	})

	t.Run("SearchByExtension", func(t *testing.T) {
		results := searcher.SearchByExtension(".txt")
		verifySorted(t, results)
	})

	t.Run("SearchMultipleExtensions", func(t *testing.T) {
		results := searcher.SearchMultipleExtensions([]string{".txt"})
		verifySorted(t, results)
	})

	t.Run("SearchAll", func(t *testing.T) {
		results := searcher.SearchAll()
		verifySorted(t, results)
	})
}

// TestSearch tests the main Search method with query parsing
func TestSearch(t *testing.T) {
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	testFiles := []*indexer.FileInfo{
		{Path: "/home/user/config.yaml", Name: "config.yaml", Size: 100, Extension: ".yaml"},
		{Path: "/home/user/config.txt", Name: "config.txt", Size: 200, Extension: ".txt"},
		{Path: "/home/user/test.go", Name: "test.go", Size: 500, Extension: ".go"},
		{Path: "/home/user/test_utils.go", Name: "test_utils.go", Size: 600, Extension: ".go"},
		{Path: "/var/log/README.md", Name: "README.md", Size: 50, Extension: ".md"},
	}

	for _, file := range testFiles {
		idx.Add(file)
	}

	t.Run("SubstringSearch", func(t *testing.T) {
		results, err := searcher.Search("config")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})

	t.Run("PrefixSearch", func(t *testing.T) {
		results, err := searcher.Search("/home/user")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) != 4 {
			t.Errorf("Expected 4 results, got %d", len(results))
		}
	})

	t.Run("ExactSearch", func(t *testing.T) {
		results, err := searcher.Search("exact:config.yaml")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}

		if len(results) > 0 && results[0].Name != "config.yaml" {
			t.Errorf("Expected config.yaml, got %s", results[0].Name)
		}
	})

	t.Run("SearchWithExtensionFilter", func(t *testing.T) {
		results, err := searcher.Search("test ext:.go")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) != 2 {
			t.Errorf("Expected 2 .go files, got %d", len(results))
		}

		for _, file := range results {
			if file.Extension != ".go" {
				t.Errorf("Expected .go extension, got %s", file.Extension)
			}
		}
	})

	t.Run("SearchWithSizeFilter", func(t *testing.T) {
		results, err := searcher.Search("test size:>500")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		for _, file := range results {
			if file.Size <= 500 {
				t.Errorf("File %s size %d should be > 500", file.Name, file.Size)
			}
		}
	})

	t.Run("CachedSearch", func(t *testing.T) {
		query := "config"
		
		// First search
		results1, err := searcher.Search(query)
		if err != nil {
			t.Fatalf("First search failed: %v", err)
		}

		// Second search (should hit cache)
		results2, err := searcher.Search(query)
		if err != nil {
			t.Fatalf("Second search failed: %v", err)
		}

		if len(results1) != len(results2) {
			t.Errorf("Cached results differ: %d vs %d", len(results1), len(results2))
		}
	})

	t.Run("EmptyQuery", func(t *testing.T) {
		_, err := searcher.Search("")
		if err == nil {
			t.Error("Expected error for empty query")
		}
	})
}

// TestSearchWildcard tests wildcard pattern matching
func TestSearchWildcard(t *testing.T) {
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	testFiles := []*indexer.FileInfo{
		{Path: "/home/test1.txt", Name: "test1.txt", Extension: ".txt"},
		{Path: "/home/test2.txt", Name: "test2.txt", Extension: ".txt"},
		{Path: "/home/test10.txt", Name: "test10.txt", Extension: ".txt"},
		{Path: "/home/testing.txt", Name: "testing.txt", Extension: ".txt"},
	}

	for _, file := range testFiles {
		idx.Add(file)
	}

	t.Run("SingleCharacterWildcard", func(t *testing.T) {
		results, err := searcher.Search("test?.txt")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) != 2 {
			t.Errorf("Expected 2 results (test1.txt, test2.txt), got %d", len(results))
		}
	})

	t.Run("MultiCharacterWildcard", func(t *testing.T) {
		results, err := searcher.Search("test*.txt")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) != 4 {
			t.Errorf("Expected 4 results, got %d", len(results))
		}
	})

	t.Run("MixedWildcards", func(t *testing.T) {
		results, err := searcher.Search("test??.txt")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("Expected 1 result (test10.txt), got %d", len(results))
		}
	})
}

// TestSearchRegex tests regular expression matching
func TestSearchRegex(t *testing.T) {
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	testFiles := []*indexer.FileInfo{
		{Path: "/home/test1.txt", Name: "test1.txt"},
		{Path: "/home/test2.txt", Name: "test2.txt"},
		{Path: "/home/test10.txt", Name: "test10.txt"},
		{Path: "/home/testing.txt", Name: "testing.txt"},
	}

	for _, file := range testFiles {
		idx.Add(file)
	}

	t.Run("DigitPattern", func(t *testing.T) {
		results, err := searcher.Search("regex:test[0-9]+\\.txt")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) != 3 {
			t.Errorf("Expected 3 results, got %d", len(results))
		}
	})

	t.Run("StartAnchor", func(t *testing.T) {
		results, err := searcher.Search("regex:^test[12]\\.txt$")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) != 2 {
			t.Errorf("Expected 2 results, got %d", len(results))
		}
	})

	t.Run("InvalidRegex", func(t *testing.T) {
		_, err := searcher.Search("regex:[invalid(")
		if err == nil {
			t.Error("Expected error for invalid regex")
		}
	})
}

// TestSearchFuzzy tests fuzzy matching with Levenshtein distance
func TestSearchFuzzy(t *testing.T) {
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	testFiles := []*indexer.FileInfo{
		{Path: "/home/config.txt", Name: "config.txt"},
		{Path: "/home/konfig.txt", Name: "konfig.txt"},
		{Path: "/home/contig.txt", Name: "contig.txt"},
		{Path: "/home/readme.txt", Name: "readme.txt"},
	}

	for _, file := range testFiles {
		idx.Add(file)
	}

	t.Run("DefaultDistance", func(t *testing.T) {
		results, err := searcher.Search("fuzzy:config")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) < 2 {
			t.Errorf("Expected at least 2 results with fuzzy matching, got %d", len(results))
		}
	})

	t.Run("CustomDistance", func(t *testing.T) {
		results, err := searcher.Search("fuzzy:config fuzzy:1")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		// With distance=1, should match config, konfig, contig
		if len(results) != 3 {
			t.Errorf("Expected 3 results with distance=1, got %d", len(results))
		}
	})
}

// TestLevenshteinDistance tests the Levenshtein distance calculation
func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		s1       string
		s2       string
		expected int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "adc", 1},
		{"kitten", "sitting", 3},
		{"config", "konfig", 1},
		{"config", "contig", 1},
	}

	for _, tt := range tests {
		t.Run(tt.s1+"_"+tt.s2, func(t *testing.T) {
			result := levenshteinDistance(tt.s1, tt.s2)
			if result != tt.expected {
				t.Errorf("levenshteinDistance(%q, %q) = %d, expected %d",
					tt.s1, tt.s2, result, tt.expected)
			}
		})
	}
}

// TestSearchWithFilters tests applying filters to search results
func TestSearchWithFilters(t *testing.T) {
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	testFiles := []*indexer.FileInfo{
		{Path: "/small.txt", Name: "small.txt", Size: 100, Extension: ".txt"},
		{Path: "/medium.txt", Name: "medium.txt", Size: 1024, Extension: ".txt"},
		{Path: "/large.txt", Name: "large.txt", Size: 1024 * 1024, Extension: ".txt"},
		{Path: "/small.go", Name: "small.go", Size: 200, Extension: ".go"},
	}

	for _, file := range testFiles {
		idx.Add(file)
	}

	t.Run("SizeFilterMin", func(t *testing.T) {
		results, err := searcher.Search("size:>1000")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		for _, file := range results {
			if file.Size <= 1000 {
				t.Errorf("File %s size %d should be > 1000", file.Name, file.Size)
			}
		}
	})

	t.Run("SizeFilterMax", func(t *testing.T) {
		results, err := searcher.Search("size:<500")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		for _, file := range results {
			if file.Size >= 500 {
				t.Errorf("File %s size %d should be < 500", file.Name, file.Size)
			}
		}
	})

	t.Run("ExtensionFilter", func(t *testing.T) {
		results, err := searcher.Search("small ext:.txt")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("Expected 1 .txt file named small, got %d", len(results))
		}
	})

	t.Run("MultipleExtensionFilter", func(t *testing.T) {
		results, err := searcher.Search("small ext:.txt,.go")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) != 2 {
			t.Errorf("Expected 2 files, got %d", len(results))
		}
	})

	t.Run("CombinedFilters", func(t *testing.T) {
		results, err := searcher.Search("size:>150 ext:.txt")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		for _, file := range results {
			if file.Size <= 150 {
				t.Errorf("File %s size %d should be > 150", file.Name, file.Size)
			}
			if file.Extension != ".txt" {
				t.Errorf("File %s should have .txt extension", file.Name)
			}
		}
	})
}

// TestSearchCaseSensitivity tests case-sensitive and case-insensitive searches
func TestSearchCaseSensitivity(t *testing.T) {
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	testFiles := []*indexer.FileInfo{
		{Path: "/README.txt", Name: "README.txt"},
		{Path: "/readme.txt", Name: "readme.txt"},
		{Path: "/ReadMe.txt", Name: "ReadMe.txt"},
	}

	for _, file := range testFiles {
		idx.Add(file)
	}

	t.Run("CaseInsensitive", func(t *testing.T) {
		results, err := searcher.Search("readme")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) != 3 {
			t.Errorf("Expected 3 results with case-insensitive search, got %d", len(results))
		}
	})

	t.Run("CaseSensitive", func(t *testing.T) {
		results, err := searcher.Search("readme case:true")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) != 1 {
			t.Errorf("Expected 1 result with case-sensitive search, got %d", len(results))
		}

		if len(results) > 0 && results[0].Name != "readme.txt" {
			t.Errorf("Expected readme.txt, got %s", results[0].Name)
		}
	})
}

// TestSearchRanking tests that results are ranked by relevance
func TestSearchRanking(t *testing.T) {
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	testFiles := []*indexer.FileInfo{
		{Path: "/config", Name: "config", Size: 100},
		{Path: "/config.yaml", Name: "config.yaml", Size: 200},
		{Path: "/my_config.txt", Name: "my_config.txt", Size: 300},
	}

	for _, file := range testFiles {
		idx.Add(file)
	}

	t.Run("ExactMatchFirst", func(t *testing.T) {
		results, err := searcher.Search("config")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}

		if len(results) == 0 {
			t.Fatal("No results returned")
		}

		// Exact match should be ranked first
		if results[0].Name != "config" {
			t.Errorf("Expected 'config' to be ranked first, got '%s'", results[0].Name)
		}
	})
}

// TestSearchCache tests the LRU cache functionality
func TestSearchCache(t *testing.T) {
	idx := index.NewFileIndex()
	searcher := NewSearcher(idx)

	testFiles := []*indexer.FileInfo{
		{Path: "/file1.txt", Name: "file1.txt"},
		{Path: "/file2.txt", Name: "file2.txt"},
	}

	for _, file := range testFiles {
		idx.Add(file)
	}

	t.Run("CacheHit", func(t *testing.T) {
		query := "file"

		// First search - populates cache
		results1, err := searcher.Search(query)
		if err != nil {
			t.Fatalf("First search failed: %v", err)
		}

		// Modify index (add file)
		idx.Add(&indexer.FileInfo{Path: "/file3.txt", Name: "file3.txt"})

		// Second search - should return cached results (won't include file3)
		results2, err := searcher.Search(query)
		if err != nil {
			t.Fatalf("Second search failed: %v", err)
		}

		if len(results1) != len(results2) {
			t.Errorf("Cache should return same results: %d vs %d", len(results1), len(results2))
		}

		// Results should be identical (from cache)
		if len(results2) != 2 {
			t.Errorf("Expected 2 cached results, got %d", len(results2))
		}
	})

	t.Run("CacheSize", func(t *testing.T) {
		// Cache should have at least one entry
		if searcher.cache.Size() == 0 {
			t.Error("Cache should have entries after searches")
		}
	})
}