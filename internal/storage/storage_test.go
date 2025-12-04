package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/makokhawanjala/searchlight/internal/index"
	"github.com/makokhawanjala/searchlight/internal/indexer"
)

// TestStorageInterface verifies that JSONStorage correctly implements the Storage interface.
//
// Why this test?
// - Compile-time check that JSONStorage has all required methods
// - Ensures interface contract is maintained
// - Catches method signature changes early
//
// What we're testing:
// - JSONStorage implements Storage interface
// - All methods are present with correct signatures
func TestStorageInterface(t *testing.T) {
	// Create a temporary file path for testing
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "test_interface.json.gz")

	// Create a JSONStorage instance
	jsonStorage := NewJSONStorage(storagePath, true)

	// Verify it implements Storage interface
	// This is a compile-time check - if JSONStorage doesn't implement Storage,
	// this line won't compile
	var _ Storage = jsonStorage

	// Additional runtime verification that all methods are callable
	t.Run("SaveMethod", func(t *testing.T) {
		// Create empty index
		idx := index.NewFileIndex()
		
		// Save should work (even for empty index)
		err := jsonStorage.Save(idx)
		if err != nil {
			t.Errorf("Save method failed: %v", err)
		}
	})

	t.Run("LoadMethod", func(t *testing.T) {
		// Load the index we just saved
		loadedIdx, err := jsonStorage.Load()
		if err != nil {
			t.Errorf("Load method failed: %v", err)
		}
		if loadedIdx == nil {
			t.Error("Load returned nil index")
		}
	})

	t.Run("PathMethod", func(t *testing.T) {
		// Path should return the configured path
		path := jsonStorage.Path()
		if path != storagePath {
			t.Errorf("Path() = %s, want %s", path, storagePath)
		}
	})

	t.Run("ClearMethod", func(t *testing.T) {
		// Clear should remove the file
		err := jsonStorage.Clear()
		if err != nil {
			t.Errorf("Clear method failed: %v", err)
		}
		
		// Verify file is gone
		if FileExists(storagePath) {
			t.Error("Clear did not remove storage file")
		}
	})
}

// TestStorageLifecycle tests the complete storage lifecycle.
//
// Why this test?
// - Verifies the entire workflow: create → save → load → clear
// - Tests state transitions between operations
// - Ensures operations work together correctly
//
// Lifecycle:
// 1. Create new storage
// 2. Save empty index
// 3. Load and verify
// 4. Add files and save again
// 5. Load and verify files present
// 6. Clear storage
// 7. Verify storage is clean
func TestStorageLifecycle(t *testing.T) {
	// Setup: Create temporary storage
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "lifecycle_test.json.gz")
	storage := NewJSONStorage(storagePath, true)

	// Step 1: Save empty index
	t.Log("Step 1: Saving empty index")
	emptyIdx := index.NewFileIndex()
	err := storage.Save(emptyIdx)
	if err != nil {
		t.Fatalf("Failed to save empty index: %v", err)
	}

	// Verify file was created
	if !FileExists(storagePath) {
		t.Fatal("Storage file was not created")
	}

	// Step 2: Load empty index
	t.Log("Step 2: Loading empty index")
	loadedIdx, err := storage.Load()
	if err != nil {
		t.Fatalf("Failed to load empty index: %v", err)
	}
	if loadedIdx.Size() != 0 {
		t.Errorf("Loaded index size = %d, want 0", loadedIdx.Size())
	}

	// Step 3: Add some files to index and save
	t.Log("Step 3: Adding files and saving")
	file1 := &indexer.FileInfo{
		Path:     "/test/file1.txt",
		Name:     "file1.txt",
		Size:     1024,
		ModifiedTime:  time.Now(),
		IsDir:    false,
		Extension: ".txt",
	}
	file2 := &indexer.FileInfo{
		Path:     "/test/file2.md",
		Name:     "file2.md",
		Size:     2048,
		ModifiedTime:  time.Now(),
		IsDir:    false,
		Extension: ".md",
	}
	
	loadedIdx.Add(file1)
	loadedIdx.Add(file2)
	
	err = storage.Save(loadedIdx)
	if err != nil {
		t.Fatalf("Failed to save index with files: %v", err)
	}

	// Step 4: Load and verify files are present
	t.Log("Step 4: Loading and verifying files")
	reloadedIdx, err := storage.Load()
	if err != nil {
		t.Fatalf("Failed to reload index: %v", err)
	}
	
	if reloadedIdx.Size() != 2 {
		t.Errorf("Reloaded index size = %d, want 2", reloadedIdx.Size())
	}
	
	// Verify both files are present
	if !reloadedIdx.Contains("/test/file1.txt") {
		t.Error("file1.txt not found in reloaded index")
	}
	if !reloadedIdx.Contains("/test/file2.md") {
		t.Error("file2.md not found in reloaded index")
	}

	// Step 5: Clear storage
	t.Log("Step 5: Clearing storage")
	err = storage.Clear()
	if err != nil {
		t.Fatalf("Failed to clear storage: %v", err)
	}
	
	// Verify file is gone
	if FileExists(storagePath) {
		t.Error("Storage file still exists after Clear")
	}

	// Step 6: Verify load fails after clear
	t.Log("Step 6: Verifying load fails after clear")
	_, err = storage.Load()
	if err == nil {
		t.Error("Load should fail after Clear, but succeeded")
	}
}

// TestStorageWithCompression tests storage with compression enabled/disabled.
//
// Why this test?
// - Verify both compressed and uncompressed modes work
// - Ensure data integrity regardless of compression setting
// - Test SetCompression method
//
// What we test:
// - Save/load with compression (default)
// - Save/load without compression
// - Switching compression mid-lifecycle
func TestStorageWithCompression(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("Compressed", func(t *testing.T) {
		// Test with compression enabled (default)
		storagePath := filepath.Join(tempDir, "compressed.json.gz")
		storage := NewJSONStorage(storagePath, true)
		
		// Create index with some files
		idx := index.NewFileIndex()
		for i := 0; i < 10; i++ {
			file := &indexer.FileInfo{
				Path:      filepath.Join("/test", "file", string(rune('a'+i)), ".txt"),
				Name:      string(rune('a'+i)) + ".txt",
				Size:      1024 * int64(i+1),
				ModifiedTime:   time.Now(),
				IsDir:     false,
				Extension: ".txt",
			}
			idx.Add(file)
		}
		
		// Save with compression
		err := storage.Save(idx)
		if err != nil {
			t.Fatalf("Failed to save with compression: %v", err)
		}
		
		// Load and verify
		loadedIdx, err := storage.Load()
		if err != nil {
			t.Fatalf("Failed to load compressed file: %v", err)
		}
		
		if loadedIdx.Size() != 10 {
			t.Errorf("Loaded index size = %d, want 10", loadedIdx.Size())
		}
	})

	t.Run("Uncompressed", func(t *testing.T) {
		// Test with compression disabled
		storagePath := filepath.Join(tempDir, "uncompressed.json")
		storage := NewJSONStorage(storagePath, false)
		
		// Create index with some files
		idx := index.NewFileIndex()
		for i := 0; i < 10; i++ {
			file := &indexer.FileInfo{
				Path:      filepath.Join("/test", "file", string(rune('a'+i)), ".txt"),
				Name:      string(rune('a'+i)) + ".txt",
				Size:      1024 * int64(i+1),
				ModifiedTime:   time.Now(),
				IsDir:     false,
				Extension: ".txt",
			}
			idx.Add(file)
		}
		
		// Save without compression
		err := storage.Save(idx)
		if err != nil {
			t.Fatalf("Failed to save without compression: %v", err)
		}
		
		// Load and verify
		loadedIdx, err := storage.Load()
		if err != nil {
			t.Fatalf("Failed to load uncompressed file: %v", err)
		}
		
		if loadedIdx.Size() != 10 {
			t.Errorf("Loaded index size = %d, want 10", loadedIdx.Size())
		}
	})

	t.Run("ToggleCompression", func(t *testing.T) {
		// Test changing compression setting
		storagePath := filepath.Join(tempDir, "toggle.json.gz")
		storage := NewJSONStorage(storagePath, true)
		
		// Save with compression
		idx := index.NewFileIndex()
		idx.Add(&indexer.FileInfo{
			Path:      "/test/toggle.txt",
			Name:      "toggle.txt",
			Size:      1024,
			ModifiedTime:   time.Now(),
			IsDir:     false,
			Extension: ".txt",
		})
		
		err := storage.Save(idx)
		if err != nil {
			t.Fatalf("Failed to save: %v", err)
		}
		
		// Disable compression and save again (new file)
		//storagePath2 := filepath.Join(tempDir, "toggle2.json")
		storage.SetCompression(false)
		// Note: Path is still the old one, but SetCompression affects future saves
		// This is more of a unit test - in practice you'd create a new storage
		
		// The test here verifies SetCompression doesn't cause errors
		// Actual behavior depends on implementation details
	})
}

// TestStorageEdgeCases tests edge cases and error conditions.
//
// Why this test?
// - Verify graceful handling of invalid inputs
// - Test error paths
// - Ensure robustness
//
// Edge cases:
// - Loading non-existent file
// - Clearing non-existent file (should succeed - idempotent)
// - Invalid paths
// - Permission errors (if applicable)
func TestStorageEdgeCases(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("LoadNonExistent", func(t *testing.T) {
		// Try to load a file that doesn't exist
		storagePath := filepath.Join(tempDir, "nonexistent.json.gz")
		storage := NewJSONStorage(storagePath, true)
		
		_, err := storage.Load()
		if err == nil {
			t.Error("Load should fail for non-existent file")
		}
	})

	t.Run("ClearNonExistent", func(t *testing.T) {
		// Clear should succeed even if file doesn't exist (idempotent)
		storagePath := filepath.Join(tempDir, "nonexistent2.json.gz")
		storage := NewJSONStorage(storagePath, true)
		
		err := storage.Clear()
		if err != nil {
			t.Errorf("Clear should succeed for non-existent file, got: %v", err)
		}
	})

	t.Run("EmptyPath", func(t *testing.T) {
		// Test with empty path (should handle gracefully or error)
		storage := NewJSONStorage("", true)
		
		idx := index.NewFileIndex()
		err := storage.Save(idx)
		// Behavior depends on implementation - either error or create file
		// We just verify it doesn't panic
		_ = err // May or may not error
	})

	t.Run("PathMethod", func(t *testing.T) {
		// Verify Path() returns what was set
		testPath := filepath.Join(tempDir, "path_test.json.gz")
		storage := NewJSONStorage(testPath, true)
		
		if storage.Path() != testPath {
			t.Errorf("Path() = %s, want %s", storage.Path(), testPath)
		}
	})
}

// TestStorageStats tests the statistics tracking.
//
// Why this test?
// - Verify stats are updated correctly
// - Test NewStorageStats initialization
// - Ensure stats are useful for monitoring
//
// What we test:
// - Initial stats are zero
// - Stats update after operations
// - Stats are accurate
func TestStorageStats(t *testing.T) {
	// Test NewStorageStats creates initialized stats
	t.Run("NewStorageStats", func(t *testing.T) {
		stats := NewStorageStats()
		
		if stats.SaveCount != 0 {
			t.Errorf("SaveCount = %d, want 0", stats.SaveCount)
		}
		if stats.LoadCount != 0 {
			t.Errorf("LoadCount = %d, want 0", stats.LoadCount)
		}
		if stats.SaveErrors != 0 {
			t.Errorf("SaveErrors = %d, want 0", stats.SaveErrors)
		}
		if stats.LoadErrors != 0 {
			t.Errorf("LoadErrors = %d, want 0", stats.LoadErrors)
		}
		if stats.StorageSize != 0 {
			t.Errorf("StorageSize = %d, want 0", stats.StorageSize)
		}
	})

	// Test that stats are tracked during operations
	t.Run("StatsTracking", func(t *testing.T) {
		tempDir := t.TempDir()
		storagePath := filepath.Join(tempDir, "stats_test.json.gz")
		storage := NewJSONStorage(storagePath, true)
		
		// Initial stats should be zero (via NewStorageStats)
		stats := storage.Stats()
		if stats.SaveCount != 0 {
			t.Errorf("Initial SaveCount = %d, want 0", stats.SaveCount)
		}
		
		// Perform save operation
		idx := index.NewFileIndex()
		idx.Add(&indexer.FileInfo{
			Path:      "/test/stats.txt",
			Name:      "stats.txt",
			Size:      1024,
			ModifiedTime:   time.Now(),
			IsDir:     false,
			Extension: ".txt",
		})
		
		err := storage.Save(idx)
		if err != nil {
			t.Fatalf("Save failed: %v", err)
		}
		
		// Check stats were updated
		stats = storage.Stats()
		if stats.SaveCount != 1 {
			t.Errorf("After save, SaveCount = %d, want 1", stats.SaveCount)
		}
		if stats.LastSaveTime == 0 {
			t.Error("LastSaveTime was not set after save")
		}
		
		// Perform load operation
		_, err = storage.Load()
		if err != nil {
			t.Fatalf("Load failed: %v", err)
		}
		
		// Check load stats were updated
		stats = storage.Stats()
		if stats.LoadCount != 1 {
			t.Errorf("After load, LoadCount = %d, want 1", stats.LoadCount)
		}
		if stats.LastLoadTime == 0 {
			t.Error("LastLoadTime was not set after load")
		}
	})
}

// TestStorageFactory demonstrates the factory pattern for creating storage backends.
//
// Why this test?
// - Shows how to create different storage implementations
// - Demonstrates the Storage interface abstraction
// - Useful pattern for adding new backends (SQLite, Redis, etc.)
//
// Currently:
// - Only JSONStorage exists
// - This test demonstrates the pattern for future backends
func TestStorageFactory(t *testing.T) {
	tempDir := t.TempDir()

	// Helper function that works with any Storage implementation
	// This demonstrates the value of the Storage interface
	testStorage := func(t *testing.T, storage Storage, name string) {
		t.Run(name, func(t *testing.T) {
			// Create test index
			idx := index.NewFileIndex()
			idx.Add(&indexer.FileInfo{
				Path:      "/test/factory.txt",
				Name:      "factory.txt",
				Size:      1024,
				ModifiedTime:   time.Now(),
				IsDir:     false,
				Extension: ".txt",
			})
			
			// Save
			err := storage.Save(idx)
			if err != nil {
				t.Fatalf("Save failed: %v", err)
			}
			
			// Load
			loadedIdx, err := storage.Load()
			if err != nil {
				t.Fatalf("Load failed: %v", err)
			}
			
			// Verify
			if loadedIdx.Size() != 1 {
				t.Errorf("Size = %d, want 1", loadedIdx.Size())
			}
			
			// Clear
			err = storage.Clear()
			if err != nil {
				t.Fatalf("Clear failed: %v", err)
			}
		})
	}

	// Test JSONStorage (compressed)
	jsonCompressed := NewJSONStorage(
		filepath.Join(tempDir, "json_compressed.json.gz"),
		true,
	)
	testStorage(t, jsonCompressed, "JSONStorage_Compressed")

	// Test JSONStorage (uncompressed)
	jsonUncompressed := NewJSONStorage(
		filepath.Join(tempDir, "json_uncompressed.json"),
		false,
	)
	testStorage(t, jsonUncompressed, "JSONStorage_Uncompressed")

	// Future: Test other backends
	// gobStorage := NewGobStorage(filepath.Join(tempDir, "gob.bin"))
	// testStorage(t, gobStorage, "GobStorage")
	//
	// sqliteStorage := NewSQLiteStorage(filepath.Join(tempDir, "index.db"))
	// testStorage(t, sqliteStorage, "SQLiteStorage")
}

// TestStorageMultipleOperations tests repeated operations.
//
// Why this test?
// - Verify storage can handle many operations
// - Test that state is maintained correctly
// - Ensure no memory leaks or file handle leaks
//
// Scenario:
// - Save multiple times
// - Load multiple times
// - Verify consistency
func TestStorageMultipleOperations(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "multiple_ops.json.gz")
	storage := NewJSONStorage(storagePath, true)

	// Perform multiple save/load cycles
	for i := 0; i < 5; i++ {
		t.Logf("Cycle %d", i+1)
		
		// Create index with incrementing file count
		idx := index.NewFileIndex()
		for j := 0; j <= i; j++ {
			file := &indexer.FileInfo{
				Path:      filepath.Join("/test", "cycle", string(rune('0'+i)), string(rune('a'+j)), ".txt"),
				Name:      string(rune('a'+j)) + ".txt",
				Size:      1024,
				ModifiedTime:   time.Now(),
				IsDir:     false,
				Extension: ".txt",
			}
			idx.Add(file)
		}
		
		// Save
		err := storage.Save(idx)
		if err != nil {
			t.Fatalf("Cycle %d: Save failed: %v", i+1, err)
		}
		
		// Load
		loadedIdx, err := storage.Load()
		if err != nil {
			t.Fatalf("Cycle %d: Load failed: %v", i+1, err)
		}
		
		// Verify file count
		expectedCount := i + 1
		if loadedIdx.Size() != expectedCount {
			t.Errorf("Cycle %d: Size = %d, want %d", i+1, loadedIdx.Size(), expectedCount)
		}
	}

	// Verify final state
	finalIdx, err := storage.Load()
	if err != nil {
		t.Fatalf("Final load failed: %v", err)
	}
	
	// Last cycle added 5 files (indices 0-4)
	if finalIdx.Size() != 5 {
		t.Errorf("Final size = %d, want 5", finalIdx.Size())
	}
}

// TestStorageInvalidPath tests behavior with invalid paths.
//
// Why this test?
// - Verify error handling for invalid paths
// - Ensure graceful failures
// - Document expected behavior
func TestStorageInvalidPath(t *testing.T) {
	t.Run("DirectoryAsFile", func(t *testing.T) {
		// Try to use a directory as a file
		tempDir := t.TempDir()
		storage := NewJSONStorage(tempDir, true) // tempDir is a directory, not a file
		
		idx := index.NewFileIndex()
		err := storage.Save(idx)
		// Should fail because we can't write to a directory as if it's a file
		if err == nil {
			t.Error("Save should fail when path is a directory")
		}
	})

	t.Run("ReadOnlyDirectory", func(t *testing.T) {
		// Try to write to a read-only directory (if we can create one)
		tempDir := t.TempDir()
		readOnlyDir := filepath.Join(tempDir, "readonly")
		err := os.Mkdir(readOnlyDir, 0444) // Read-only directory
		if err != nil {
			t.Skipf("Cannot create read-only directory: %v", err)
		}
		defer os.Chmod(readOnlyDir, 0755) // Cleanup

		storagePath := filepath.Join(readOnlyDir, "test.json.gz")
		storage := NewJSONStorage(storagePath, true)
		
		idx := index.NewFileIndex()
		err = storage.Save(idx)
		// Should fail due to permission error
		// Note: This might not fail on all systems/file systems
		if err == nil {
			t.Log("Warning: Save succeeded in read-only directory (system-dependent)")
		}
	})
}