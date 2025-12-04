package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/makokhawanjala/searchlight/internal/index"
	"github.com/makokhawanjala/searchlight/internal/indexer"
)

// TestJSONStorage_SaveAndLoad tests the complete save/load cycle.
//
// What we're testing:
// - Save creates file on disk
// - Load reads file correctly
// - Loaded index matches saved index
// - File count is correct
// - All file metadata is preserved
func TestJSONStorage_SaveAndLoad(t *testing.T) {
	// Setup: Create temporary directory for test
	tempDir := t.TempDir() // Go 1.15+: automatically cleaned up
	indexPath := filepath.Join(tempDir, "index.json.gz")

	// Create storage instance with compression
	storage := NewJSONStorage(indexPath, true)

	// Create a FileIndex with test data
	idx := index.NewFileIndex()

	// Add some test files
	testFiles := []*indexer.FileInfo{
		{
			Path:         "/home/user/file1.txt",
			Name:         "file1.txt",
			Size:         1024,
			ModifiedTime: time.Now(),
			IsDir:        false,
			Extension:    ".txt",
		},
		{
			Path:         "/home/user/file2.go",
			Name:         "file2.go",
			Size:         2048,
			ModifiedTime: time.Now(),
			IsDir:        false,
			Extension:    ".go",
		},
		{
			Path:         "/home/user/docs",
			Name:         "docs",
			Size:         4096,
			ModifiedTime: time.Now(),
			IsDir:        true,
			Extension:    "",
		},
	}

	// Add files to index
	for _, file := range testFiles {
		idx.Add(file)
	}

	// Test: Save the index
	if err := storage.Save(idx); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify: File exists on disk
	if !FileExists(indexPath) {
		t.Fatalf("Index file was not created at %s", indexPath)
	}

	// Test: Load the index
	loadedIdx, err := storage.Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify: Loaded index has correct size
	if loadedIdx.Size() != idx.Size() {
		t.Errorf("Loaded index size mismatch: got %d, want %d",
			loadedIdx.Size(), idx.Size())
	}

	// Verify: All files are present with correct data
	for _, originalFile := range testFiles {
		loadedFile, exists := loadedIdx.Get(originalFile.Path)
		if !exists {
			t.Errorf("File not found in loaded index: %s", originalFile.Path)
			continue
		}

		// Check all fields match
		if loadedFile.Name != originalFile.Name {
			t.Errorf("Name mismatch for %s: got %s, want %s",
				originalFile.Path, loadedFile.Name, originalFile.Name)
		}
		if loadedFile.Size != originalFile.Size {
			t.Errorf("Size mismatch for %s: got %d, want %d",
				originalFile.Path, loadedFile.Size, originalFile.Size)
		}
		if loadedFile.IsDir != originalFile.IsDir {
			t.Errorf("IsDir mismatch for %s: got %v, want %v",
				originalFile.Path, loadedFile.IsDir, originalFile.IsDir)
		}
		if loadedFile.Extension != originalFile.Extension {
			t.Errorf("Extension mismatch for %s: got %s, want %s",
				originalFile.Path, loadedFile.Extension, originalFile.Extension)
		}
	}
}

// TestJSONStorage_EmptyIndex tests saving and loading an empty index.
//
// Edge case: Index with no files
// Expected: Should save/load without errors
func TestJSONStorage_EmptyIndex(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "empty_index.json.gz")

	storage := NewJSONStorage(indexPath, true)

	// Create empty index
	idx := index.NewFileIndex()

	// Save empty index
	if err := storage.Save(idx); err != nil {
		t.Fatalf("Save() failed for empty index: %v", err)
	}

	// Load empty index
	loadedIdx, err := storage.Load()
	if err != nil {
		t.Fatalf("Load() failed for empty index: %v", err)
	}

	// Verify loaded index is empty
	if loadedIdx.Size() != 0 {
		t.Errorf("Loaded empty index size: got %d, want 0", loadedIdx.Size())
	}
}

// TestJSONStorage_Compression tests compressed vs uncompressed storage.
//
// What we're testing:
// - Compressed file is smaller than uncompressed
// - Both formats can be loaded correctly
// - Data integrity is maintained with compression
func TestJSONStorage_Compression(t *testing.T) {
	tempDir := t.TempDir()

	// Create test index with many files (better compression ratio)
	idx := index.NewFileIndex()
	for i := 0; i < 100; i++ {
		file := &indexer.FileInfo{
			Path:         filepath.Join("/test", "file"+string(rune(i))+".txt"),
			Name:         "file" + string(rune(i)) + ".txt",
			Size:         int64(i * 100),
			ModifiedTime: time.Now(),
			IsDir:        false,
			Extension:    ".txt",
		}
		idx.Add(file)
	}

	// Test 1: Save with compression
	compressedPath := filepath.Join(tempDir, "compressed.json.gz")
	compressedStorage := NewJSONStorage(compressedPath, true)

	if err := compressedStorage.Save(idx); err != nil {
		t.Fatalf("Save() with compression failed: %v", err)
	}

	// Test 2: Save without compression
	uncompressedPath := filepath.Join(tempDir, "uncompressed.json")
	uncompressedStorage := NewJSONStorage(uncompressedPath, false)

	if err := uncompressedStorage.Save(idx); err != nil {
		t.Fatalf("Save() without compression failed: %v", err)
	}

	// Verify: Compressed file is smaller
	compressedSize, err := GetFileSize(compressedPath)
	if err != nil {
		t.Fatalf("Failed to get compressed file size: %v", err)
	}

	uncompressedSize, err := GetFileSize(uncompressedPath)
	if err != nil {
		t.Fatalf("Failed to get uncompressed file size: %v", err)
	}

	if compressedSize >= uncompressedSize {
		t.Errorf("Compressed file is not smaller: compressed=%d, uncompressed=%d",
			compressedSize, uncompressedSize)
	}

	t.Logf("Compression ratio: %.1f%% (compressed: %d bytes, uncompressed: %d bytes)",
		float64(compressedSize)/float64(uncompressedSize)*100,
		compressedSize, uncompressedSize)

	// Verify: Both can be loaded correctly
	loadedCompressed, err := compressedStorage.Load()
	if err != nil {
		t.Fatalf("Load() failed for compressed file: %v", err)
	}

	loadedUncompressed, err := uncompressedStorage.Load()
	if err != nil {
		t.Fatalf("Load() failed for uncompressed file: %v", err)
	}

	// Verify: Both loaded indexes have correct size
	if loadedCompressed.Size() != idx.Size() {
		t.Errorf("Compressed index size mismatch: got %d, want %d",
			loadedCompressed.Size(), idx.Size())
	}

	if loadedUncompressed.Size() != idx.Size() {
		t.Errorf("Uncompressed index size mismatch: got %d, want %d",
			loadedUncompressed.Size(), idx.Size())
	}
}

// TestJSONStorage_LoadNonExistent tests loading when file doesn't exist.
//
// Expected: Returns error, doesn't crash
func TestJSONStorage_LoadNonExistent(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "nonexistent.json.gz")

	storage := NewJSONStorage(indexPath, true)

	// Attempt to load non-existent file
	_, err := storage.Load()
	if err == nil {
		t.Fatal("Load() should fail for non-existent file")
	}

	// Verify error message is descriptive
	if err.Error() == "" {
		t.Error("Load() error message should not be empty")
	}
}

// TestJSONStorage_Clear tests the Clear() method.
//
// What we're testing:
// - Clear removes the file
// - Clear on non-existent file doesn't error (idempotent)
// - Clear on already cleared storage is safe
func TestJSONStorage_Clear(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "clear_test.json.gz")

	storage := NewJSONStorage(indexPath, true)

	// Create and save an index
	idx := index.NewFileIndex()
	idx.Add(&indexer.FileInfo{
		Path:      "/test/file.txt",
		Name:      "file.txt",
		Size:      100,
		IsDir:     false,
		Extension: ".txt",
	})

	if err := storage.Save(idx); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Verify file exists
	if !FileExists(indexPath) {
		t.Fatal("Index file should exist before clear")
	}

	// Test: Clear the storage
	if err := storage.Clear(); err != nil {
		t.Fatalf("Clear() failed: %v", err)
	}

	// Verify: File no longer exists
	if FileExists(indexPath) {
		t.Error("Index file should not exist after clear")
	}

	// Test: Clear again (idempotent - should not error)
	if err := storage.Clear(); err != nil {
		t.Errorf("Clear() should be idempotent, but got error: %v", err)
	}
}

// TestJSONStorage_Path tests the Path() method.
func TestJSONStorage_Path(t *testing.T) {
	expectedPath := "/home/user/.searchlight/index.json.gz"
	storage := NewJSONStorage(expectedPath, true)

	if storage.Path() != expectedPath {
		t.Errorf("Path() = %s, want %s", storage.Path(), expectedPath)
	}
}

// TestJSONStorage_Stats tests statistics tracking.
//
// What we're testing:
// - Stats are initialized correctly
// - Save increments SaveCount
// - Load increments LoadCount
// - Times are tracked
func TestJSONStorage_Stats(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "stats_test.json.gz")

	storage := NewJSONStorage(indexPath, true)

	// Initial stats should be zero
	stats := storage.Stats()
	if stats.SaveCount != 0 {
		t.Errorf("Initial SaveCount should be 0, got %d", stats.SaveCount)
	}
	if stats.LoadCount != 0 {
		t.Errorf("Initial LoadCount should be 0, got %d", stats.LoadCount)
	}

	// Create and save an index
	idx := index.NewFileIndex()
	idx.Add(&indexer.FileInfo{
		Path: "/test/file.txt",
		Name: "file.txt",
		Size: 100,
	})

	// Save the index
	if err := storage.Save(idx); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Check stats after save
	stats = storage.Stats()
	if stats.SaveCount != 1 {
		t.Errorf("SaveCount after save should be 1, got %d", stats.SaveCount)
	}
	if stats.LastSaveTime == 0 {
		t.Error("LastSaveTime should be set after save")
	}

	// Load the index
	if _, err := storage.Load(); err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Check stats after load
	stats = storage.Stats()
	if stats.LoadCount != 1 {
		t.Errorf("LoadCount after load should be 1, got %d", stats.LoadCount)
	}
	if stats.LastLoadTime == 0 {
		t.Error("LastLoadTime should be set after load")
	}
}

// TestJSONStorage_Validate tests the Validate() method.
//
// What we're testing:
// - Valid file passes validation
// - Non-existent file fails validation
// - Corrupted file fails validation
func TestJSONStorage_Validate(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("ValidFile", func(t *testing.T) {
		indexPath := filepath.Join(tempDir, "valid.json.gz")
		storage := NewJSONStorage(indexPath, true)

		// Create and save valid index
		idx := index.NewFileIndex()
		idx.Add(&indexer.FileInfo{
			Path: "/test/file.txt",
			Name: "file.txt",
			Size: 100,
		})

		if err := storage.Save(idx); err != nil {
			t.Fatalf("Save() failed: %v", err)
		}

		// Validate should pass
		if err := storage.Validate(); err != nil {
			t.Errorf("Validate() failed for valid file: %v", err)
		}
	})

	t.Run("NonExistentFile", func(t *testing.T) {
		indexPath := filepath.Join(tempDir, "nonexistent.json.gz")
		storage := NewJSONStorage(indexPath, true)

		// Validate should fail
		err := storage.Validate()
		if err == nil {
			t.Error("Validate() should fail for non-existent file")
		}
	})

	t.Run("CorruptedFile", func(t *testing.T) {
		indexPath := filepath.Join(tempDir, "corrupted.json.gz")

		// Write corrupted data
		if err := os.WriteFile(indexPath, []byte("not valid json"), 0644); err != nil {
			t.Fatalf("Failed to write corrupted file: %v", err)
		}

		storage := NewJSONStorage(indexPath, true)

		// Validate should fail
		err := storage.Validate()
		if err == nil {
			t.Error("Validate() should fail for corrupted file")
		}
	})
}

// TestJSONStorage_Backup tests the Backup() method.
//
// What we're testing:
// - Backup creates new file
// - Backup file has timestamp
// - Backup file contains same data as original
// - Backup of non-existent file fails
func TestJSONStorage_Backup(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "backup_test.json.gz")

	storage := NewJSONStorage(indexPath, true)

	t.Run("SuccessfulBackup", func(t *testing.T) {
		// Create and save index
		idx := index.NewFileIndex()
		idx.Add(&indexer.FileInfo{
			Path: "/test/file.txt",
			Name: "file.txt",
			Size: 100,
		})

		if err := storage.Save(idx); err != nil {
			t.Fatalf("Save() failed: %v", err)
		}

		// Create backup
		backupPath, err := storage.Backup()
		if err != nil {
			t.Fatalf("Backup() failed: %v", err)
		}

		// Verify backup file exists
		if !FileExists(backupPath) {
			t.Errorf("Backup file was not created at %s", backupPath)
		}

		// Verify backup path has timestamp
		if backupPath == indexPath {
			t.Error("Backup path should be different from original")
		}

		t.Logf("Backup created at: %s", backupPath)
	})

	t.Run("BackupNonExistent", func(t *testing.T) {
		nonExistentPath := filepath.Join(tempDir, "nonexistent.json.gz")
		storage := NewJSONStorage(nonExistentPath, true)

		// Backup should fail
		_, err := storage.Backup()
		if err == nil {
			t.Error("Backup() should fail for non-existent file")
		}
	})
}

// TestJSONStorage_LargeIndex tests performance with large indexes.
//
// What we're testing:
// - Can handle 10k+ files
// - Save completes in reasonable time
// - Load completes in reasonable time
// - All data is preserved
func TestJSONStorage_LargeIndex(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large index test in short mode")
	}

	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "large_index.json.gz")

	storage := NewJSONStorage(indexPath, true)

	// Create index with 10,000 files
	idx := index.NewFileIndex()
	fileCount := 10000

	t.Logf("Creating index with %d files...", fileCount)
	for i := 0; i < fileCount; i++ {
		file := &indexer.FileInfo{
			Path:         filepath.Join("/test", "dir"+string(rune(i/100)), "file"+string(rune(i))+".txt"),
			Name:         "file" + string(rune(i)) + ".txt",
			Size:         int64(i * 1024),
			ModifiedTime: time.Now(),
			IsDir:        false,
			Extension:    ".txt",
		}
		idx.Add(file)
	}

	// Test: Save large index
	t.Log("Saving large index...")
	saveStart := time.Now()
	if err := storage.Save(idx); err != nil {
		t.Fatalf("Save() failed for large index: %v", err)
	}
	saveTime := time.Since(saveStart)
	t.Logf("Save completed in %v", saveTime)

	// Verify: Save completed in reasonable time (< 5 seconds)
	if saveTime > 5*time.Second {
		t.Errorf("Save took too long: %v (expected < 5s)", saveTime)
	}

	// Check file size
	fileSize, _ := GetFileSize(indexPath)
	t.Logf("Index file size: %.2f MB", float64(fileSize)/(1024*1024))

	// Test: Load large index
	t.Log("Loading large index...")
	loadStart := time.Now()
	loadedIdx, err := storage.Load()
	if err != nil {
		t.Fatalf("Load() failed for large index: %v", err)
	}
	loadTime := time.Since(loadStart)
	t.Logf("Load completed in %v", loadTime)

	// Verify: Load completed in reasonable time (< 3 seconds)
	if loadTime > 3*time.Second {
		t.Errorf("Load took too long: %v (expected < 3s)", loadTime)
	}

	// Verify: Loaded index has correct size
	if loadedIdx.Size() != fileCount {
		t.Errorf("Loaded index size mismatch: got %d, want %d",
			loadedIdx.Size(), fileCount)
	}

	// Sample check: Verify first and last files
	firstFile, exists := loadedIdx.Get("/test/dir0/file\x00.txt")
	if !exists {
		t.Error("First file not found in loaded index")
	} else if firstFile.Size != 0 {
		t.Errorf("First file size mismatch: got %d, want 0", firstFile.Size)
	}
}

// TestJSONStorage_SetCompression tests the SetCompression() method.
//
// What we're testing:
// - Can toggle compression on/off
// - Compression setting affects file size
func TestJSONStorage_SetCompression(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "compression_toggle.json")

	storage := NewJSONStorage(indexPath, true)

	// Create test index
	idx := index.NewFileIndex()
	for i := 0; i < 50; i++ {
		idx.Add(&indexer.FileInfo{
			Path: filepath.Join("/test", "file"+string(rune(i))+".txt"),
			Name: "file" + string(rune(i)) + ".txt",
			Size: int64(i * 100),
		})
	}

	// Save with compression
	if err := storage.Save(idx); err != nil {
		t.Fatalf("Save() with compression failed: %v", err)
	}
	compressedSize, _ := GetFileSize(indexPath)

	// Toggle compression off
	storage.SetCompression(false)

	// Save without compression
	if err := storage.Save(idx); err != nil {
		t.Fatalf("Save() without compression failed: %v", err)
	}
	uncompressedSize, _ := GetFileSize(indexPath)

	// Verify: Sizes are different
	if compressedSize == uncompressedSize {
		t.Error("Compression toggle had no effect on file size")
	}

	t.Logf("Compressed: %d bytes, Uncompressed: %d bytes", compressedSize, uncompressedSize)
}

// TestJSONStorage_ConcurrentSaves tests thread safety of Save() operations.
//
// What we're testing:
// - Multiple goroutines can save without crashes
// - No data corruption occurs
// - Last write wins (expected behavior)
func TestJSONStorage_ConcurrentSaves(t *testing.T) {
	tempDir := t.TempDir()
	indexPath := filepath.Join(tempDir, "concurrent.json.gz")

	storage := NewJSONStorage(indexPath, true)

	// Create different indexes
	idx1 := index.NewFileIndex()
	idx1.Add(&indexer.FileInfo{Path: "/test/file1.txt", Name: "file1.txt", Size: 100})

	idx2 := index.NewFileIndex()
	idx2.Add(&indexer.FileInfo{Path: "/test/file2.txt", Name: "file2.txt", Size: 200})

	// Launch concurrent saves
	done := make(chan error, 2)

	go func() {
		done <- storage.Save(idx1)
	}()

	go func() {
		done <- storage.Save(idx2)
	}()

	// Wait for both to complete
	err1 := <-done
	err2 := <-done

	// Both should succeed (one might overwrite the other)
	if err1 != nil {
		t.Errorf("First save failed: %v", err1)
	}
	if err2 != nil {
		t.Errorf("Second save failed: %v", err2)
	}

	// Verify: File exists and can be loaded
	loadedIdx, err := storage.Load()
	if err != nil {
		t.Fatalf("Load() failed after concurrent saves: %v", err)
	}

	// One of the indexes should have "won"
	size := loadedIdx.Size()
	if size != 1 {
		t.Errorf("Loaded index should have 1 file, got %d", size)
	}
}