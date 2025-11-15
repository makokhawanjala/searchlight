package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setupTestDir creates a temporary directory with test files
// This is used by many tests to create a realistic file structure
func setupTestDir(t *testing.T) string {
	tmpDir := t.TempDir()

	// Create test file structure with various file types
	files := map[string]string{
		"file1.txt":           "content1",
		"file2.md":            "content2",
		"subdir/file3.go":     "content3",
		"subdir/file4.txt":    "content4",
		"subdir2/file5.pdf":   "content5",
		".git/config":         "git",
		"node_modules/pkg.js": "node",
	}

	// Create each file with its directory
	for path, content := range files {
		fullPath := filepath.Join(tmpDir, path)
		dir := filepath.Dir(fullPath)

		// Create the directory if it doesn't exist
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", dir, err)
		}

		// Write the file with the test content
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", fullPath, err)
		}
	}

	return tmpDir
}

// TestIndexer_AddAndGet tests basic add and retrieve operations
func TestIndexer_AddAndGet(t *testing.T) {
	// Create a new empty indexer
	idx := NewIndexer()

	// Create a test file info
	file := &FileInfo{
		Path: "/test/file.txt",
		Name: "file.txt",
		Size: 100,
	}

	// Add the file to the index
	idx.Add(file)

	// Try to retrieve the file
	retrieved, exists := idx.Get(file.Path)
	if !exists {
		t.Fatal("file should exist in index")
	}

	// Verify the retrieved file matches what we added
	if retrieved.Path != file.Path {
		t.Errorf("expected path %s, got %s", file.Path, retrieved.Path)
	}
}

// TestIndexer_Update tests the new Update operation
// This is important for file watchers that need to update metadata
func TestIndexer_Update(t *testing.T) {
	// Create a new empty indexer
	idx := NewIndexer()

	// Create an initial file
	file := &FileInfo{
		Path: "/test/file.txt",
		Name: "file.txt",
		Size: 100,
	}

	// Add the file to the index
	idx.Add(file)

	// Create an updated version with different size
	updatedFile := &FileInfo{
		Path: "/test/file.txt",
		Name: "file.txt",
		Size: 200, // size changed from 100 to 200
	}

	// Update the file in the index
	updated := idx.Update(updatedFile)
	if !updated {
		t.Error("update should return true for existing file")
	}

	// Verify the file was updated
	retrieved, exists := idx.Get(file.Path)
	if !exists {
		t.Fatal("file should still exist after update")
	}

	if retrieved.Size != 200 {
		t.Errorf("expected size 200, got %d", retrieved.Size)
	}

	// Try to update a non-existent file
	nonExistentFile := &FileInfo{
		Path: "/test/nonexistent.txt",
		Name: "nonexistent.txt",
		Size: 100,
	}

	updated = idx.Update(nonExistentFile)
	if updated {
		t.Error("update should return false for non-existent file")
	}

	// Verify the non-existent file was not added
	_, exists = idx.Get(nonExistentFile.Path)
	if exists {
		t.Error("non-existent file should not be added by Update")
	}
}

// TestIndexer_Remove tests file removal
func TestIndexer_Remove(t *testing.T) {
	// Create a new empty indexer
	idx := NewIndexer()

	// Create and add a test file
	file := &FileInfo{Path: "/test/file.txt"}
	idx.Add(file)

	// Remove the file
	removed := idx.Remove(file.Path)
	if !removed {
		t.Error("file should have been removed")
	}

	// Verify the file is gone
	_, exists := idx.Get(file.Path)
	if exists {
		t.Error("file should not exist after removal")
	}

	// Try removing a non-existent file
	removed = idx.Remove("/nonexistent")
	if removed {
		t.Error("removing non-existent file should return false")
	}
}

// TestIndexer_UpdateVsAdd tests the difference between Update and Add
// Add will create or overwrite, Update only works on existing files
func TestIndexer_UpdateVsAdd(t *testing.T) {
	idx := NewIndexer()

	// Test Add on non-existent file (should work)
	file1 := &FileInfo{
		Path: "/test/file1.txt",
		Size: 100,
	}
	idx.Add(file1)

	if count := idx.Count(); count != 1 {
		t.Errorf("expected count 1 after Add, got %d", count)
	}

	// Test Update on non-existent file (should not work)
	file2 := &FileInfo{
		Path: "/test/file2.txt",
		Size: 200,
	}
	updated := idx.Update(file2)
	if updated {
		t.Error("Update should fail on non-existent file")
	}

	if count := idx.Count(); count != 1 {
		t.Errorf("expected count 1 after failed Update, got %d", count)
	}

	// Test Add on existing file (should overwrite)
	file1Updated := &FileInfo{
		Path: "/test/file1.txt",
		Size: 150,
	}
	idx.Add(file1Updated)

	retrieved, _ := idx.Get("/test/file1.txt")
	if retrieved.Size != 150 {
		t.Errorf("expected size 150 after Add overwrite, got %d", retrieved.Size)
	}

	// Test Update on existing file (should work)
	file1UpdatedAgain := &FileInfo{
		Path: "/test/file1.txt",
		Size: 175,
	}
	updated = idx.Update(file1UpdatedAgain)
	if !updated {
		t.Error("Update should succeed on existing file")
	}

	retrieved, _ = idx.Get("/test/file1.txt")
	if retrieved.Size != 175 {
		t.Errorf("expected size 175 after Update, got %d", retrieved.Size)
	}
}

// TestIndexer_Count tests the file counter
func TestIndexer_Count(t *testing.T) {
	idx := NewIndexer()

	// Initially empty
	if count := idx.Count(); count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}

	// Add 5 files
	for i := 0; i < 5; i++ {
		idx.Add(&FileInfo{Path: filepath.Join("/test", string(rune('a'+i)))})
	}

	if count := idx.Count(); count != 5 {
		t.Errorf("expected count 5, got %d", count)
	}

	// Remove 2 files
	idx.Remove("/test/a")
	idx.Remove("/test/b")

	if count := idx.Count(); count != 3 {
		t.Errorf("expected count 3 after removals, got %d", count)
	}
}

// TestIndexer_IndexDirectory tests sequential directory indexing
func TestIndexer_IndexDirectory(t *testing.T) {
	tmpDir := setupTestDir(t)
	idx := NewIndexer()

	// Index the test directory
	filesAdded, err := idx.IndexDirectory(tmpDir)
	if err != nil {
		t.Fatalf("IndexDirectory failed: %v", err)
	}

	// Should have indexed some files
	if filesAdded == 0 {
		t.Error("expected some files to be indexed")
	}

	// Verify .git and node_modules were skipped
	for _, file := range idx.GetAll() {
		if filepath.HasPrefix(file.Path, filepath.Join(tmpDir, ".git")) {
			t.Error(".git directory should have been skipped")
		}
		if filepath.HasPrefix(file.Path, filepath.Join(tmpDir, "node_modules")) {
			t.Error("node_modules directory should have been skipped")
		}
	}

	t.Logf("Indexed %d files/directories", filesAdded)
}

// TestIndexer_GetFilesByExtension tests filtering by file extension
func TestIndexer_GetFilesByExtension(t *testing.T) {
	idx := NewIndexer()

	// Add test files with different extensions
	files := []*FileInfo{
		{Path: "/test/file1.txt", Name: "file1.txt", Extension: ".txt"},
		{Path: "/test/file2.txt", Name: "file2.txt", Extension: ".txt"},
		{Path: "/test/file3.md", Name: "file3.md", Extension: ".md"},
	}

	for _, file := range files {
		idx.Add(file)
	}

	// Get .txt files
	txtFiles := idx.GetFilesByExtension(".txt")
	if len(txtFiles) != 2 {
		t.Errorf("expected 2 .txt files, got %d", len(txtFiles))
	}

	// Get .md files
	mdFiles := idx.GetFilesByExtension(".md")
	if len(mdFiles) != 1 {
		t.Errorf("expected 1 .md file, got %d", len(mdFiles))
	}

	// Get files with extension without dot
	txtFiles2 := idx.GetFilesByExtension("txt")
	if len(txtFiles2) != 2 {
		t.Errorf("expected 2 .txt files, got %d", len(txtFiles2))
	}
}

// TestIndexer_GetStats tests statistics gathering
func TestIndexer_GetStats(t *testing.T) {
	idx := NewIndexer()

	// Add test files and directories
	idx.Add(&FileInfo{Path: "/test/file1.txt", Size: 100, IsDir: false})
	idx.Add(&FileInfo{Path: "/test/file2.txt", Size: 200, IsDir: false})
	idx.Add(&FileInfo{Path: "/test/dir1", Size: 0, IsDir: true})

	stats := idx.GetStats()

	if stats.FileCount != 2 {
		t.Errorf("expected 2 files, got %d", stats.FileCount)
	}

	if stats.DirectoryCount != 1 {
		t.Errorf("expected 1 directory, got %d", stats.DirectoryCount)
	}

	if stats.TotalSize != 300 {
		t.Errorf("expected total size 300, got %d", stats.TotalSize)
	}

	if stats.TotalCount != 3 {
		t.Errorf("expected total count 3, got %d", stats.TotalCount)
	}
}

// TestIndexer_Clear tests clearing the entire index
func TestIndexer_Clear(t *testing.T) {
	idx := NewIndexer()

	// Add files
	idx.Add(&FileInfo{Path: "/test/file1.txt"})
	idx.Add(&FileInfo{Path: "/test/file2.txt"})

	if count := idx.Count(); count != 2 {
		t.Errorf("expected count 2 before clear, got %d", count)
	}

	// Clear the index
	idx.Clear()

	if count := idx.Count(); count != 0 {
		t.Errorf("expected count 0 after clear, got %d", count)
	}
}

// TestIndexer_ConcurrentAccess tests thread safety
// This test runs multiple goroutines that add, update, and read files simultaneously
func TestIndexer_ConcurrentAccess(t *testing.T) {
	idx := NewIndexer()

	// Channel to signal when goroutines are done
	done := make(chan bool)

	// Writer goroutines (add files)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				path := filepath.Join("/test", string(rune('a'+id)), string(rune('a'+j)))
				idx.Add(&FileInfo{Path: path})
			}
			done <- true
		}(i)
	}

	// Reader goroutines (read statistics)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = idx.Count()
				_ = idx.GetAll()
			}
			done <- true
		}()
	}

	// Wait for all goroutines to finish
	for i := 0; i < 20; i++ {
		<-done
	}

	// Verify final count
	if count := idx.Count(); count != 1000 {
		t.Errorf("expected count 1000, got %d", count)
	}
}

// TestIndexer_ConcurrentUpdateAndRemove tests concurrent Update and Remove operations
// This ensures thread safety when the watcher is making live updates
func TestIndexer_ConcurrentUpdateAndRemove(t *testing.T) {
	idx := NewIndexer()

	// Add initial files
	for i := 0; i < 100; i++ {
		path := fmt.Sprintf("/test/file%d.txt", i)
		idx.Add(&FileInfo{
			Path: path,
			Name: fmt.Sprintf("file%d.txt", i),
			Size: 100,
		})
	}

	done := make(chan bool)

	// Updater goroutines (update existing files)
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 50; j++ {
				path := fmt.Sprintf("/test/file%d.txt", j%100)
				idx.Update(&FileInfo{
					Path: path,
					Name: fmt.Sprintf("file%d.txt", j%100),
					Size: 200, // update size
				})
			}
			done <- true
		}()
	}

	// Remover goroutines (remove files)
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				path := fmt.Sprintf("/test/file%d.txt", (id*10)+j)
				idx.Remove(path)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have removed 50 files
	if count := idx.Count(); count != 50 {
		t.Errorf("expected count 50, got %d", count)
	}
}

// TestIndexer_IndexDirectoryConcurrent tests concurrent directory indexing
func TestIndexer_IndexDirectoryConcurrent(t *testing.T) {
	tmpDir := setupTestDir(t)
	idx := NewIndexerWithWorkers(3)
	ctx := context.Background()

	// Index concurrently
	count, err := idx.IndexDirectoryConcurrent(ctx, tmpDir)
	if err != nil {
		t.Fatalf("IndexDirectoryConcurrent failed: %v", err)
	}

	if count == 0 {
		t.Error("expected files to be indexed")
	}

	t.Logf("Indexed %d files concurrently", count)
}

// TestIndexer_IndexDirectoryWithProgress tests indexing with progress reporting
func TestIndexer_IndexDirectoryWithProgress(t *testing.T) {
	tmpDir := setupTestDir(t)
	idx := NewIndexerWithWorkers(2)
	ctx := context.Background()

	// Track progress updates
	var progressUpdates []int64
	progressCallback := func(processed, total int64) {
		progressUpdates = append(progressUpdates, processed)
		t.Logf("Progress: %d/%d", processed, total)
	}

	// Index with progress reporting
	count, err := idx.IndexDirectoryWithProgress(ctx, tmpDir, progressCallback)
	if err != nil {
		t.Fatalf("IndexDirectoryWithProgress failed: %v", err)
	}

	if count == 0 {
		t.Error("expected files to be indexed")
	}

	if len(progressUpdates) == 0 {
		t.Error("expected progress updates")
	}

	t.Logf("Indexed %d files with %d progress updates", count, len(progressUpdates))
}

// TestIndexer_ConcurrentCancellation tests cancellation during concurrent indexing
func TestIndexer_ConcurrentCancellation(t *testing.T) {
	tmpDir := setupTestDir(t)
	idx := NewIndexerWithWorkers(2)
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(1 * time.Millisecond)
		cancel()
	}()

	// Try to index (should be cancelled)
	_, err := idx.IndexDirectoryConcurrent(ctx, tmpDir)
	if err == nil {
		t.Log("Indexing completed before cancellation (directory too small)")
	} else {
		t.Logf("Indexing cancelled as expected: %v", err)
	}
}

// TestIndexer_SetWorkerCount tests worker count configuration
func TestIndexer_SetWorkerCount(t *testing.T) {
	idx := NewIndexer()

	tests := []struct {
		name     string
		count    int
		expected int
	}{
		{"valid count", 10, 10},
		{"too low", 0, 1},    // should be clamped to 1
		{"too high", 150, 100}, // should be clamped to 100
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx.SetWorkerCount(tt.count)
			if idx.workerCount != tt.expected {
				t.Errorf("expected worker count %d, got %d", tt.expected, idx.workerCount)
			}
		})
	}
}

// BenchmarkIndexer_Sequential benchmarks sequential indexing
func BenchmarkIndexer_Sequential(b *testing.B) {
	tmpDir := setupLargeBenchDir(b)
	idx := NewIndexer()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Clear()
		_, err := idx.IndexDirectory(tmpDir)
		if err != nil {
			b.Fatalf("IndexDirectory failed: %v", err)
		}
	}
}

// BenchmarkIndexer_Concurrent benchmarks concurrent indexing
func BenchmarkIndexer_Concurrent(b *testing.B) {
	tmpDir := setupLargeBenchDir(b)
	idx := NewIndexerWithWorkers(4)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Clear()
		_, err := idx.IndexDirectoryConcurrent(ctx, tmpDir)
		if err != nil {
			b.Fatalf("IndexDirectoryConcurrent failed: %v", err)
		}
	}
}

// BenchmarkIndexer_Update benchmarks the Update operation
func BenchmarkIndexer_Update(b *testing.B) {
	idx := NewIndexer()

	// Pre-populate with files
	for i := 0; i < 1000; i++ {
		path := fmt.Sprintf("/test/file%d.txt", i)
		idx.Add(&FileInfo{
			Path: path,
			Name: fmt.Sprintf("file%d.txt", i),
			Size: 100,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		path := fmt.Sprintf("/test/file%d.txt", i%1000)
		idx.Update(&FileInfo{
			Path: path,
			Name: fmt.Sprintf("file%d.txt", i%1000),
			Size: 200,
		})
	}
}

// setupLargeBenchDir creates a larger directory for benchmarking
func setupLargeBenchDir(b *testing.B) string {
	tmpDir := b.TempDir()

	// Create 50 directories with 10 files each (500 files total)
	for i := 0; i < 50; i++ {
		dir := filepath.Join(tmpDir, fmt.Sprintf("dir%d", i))
		if err := os.MkdirAll(dir, 0755); err != nil {
			b.Fatalf("failed to create directory: %v", err)
		}

		for j := 0; j < 10; j++ {
			file := filepath.Join(dir, fmt.Sprintf("file%d.txt", j))
			if err := os.WriteFile(file, []byte("benchmark data"), 0644); err != nil {
				b.Fatalf("failed to create file: %v", err)
			}
		}
	}

	return tmpDir
}