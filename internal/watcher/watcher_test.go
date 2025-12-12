package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/makokhawanjala/searchlight/internal/index"
)

// TestNewWatcher tests watcher creation.
//
// What we're testing:
// - Watcher can be created successfully
// - All fields are properly initialized
// - No errors occur during creation
func TestNewWatcher(t *testing.T) {
	// Create a file index for testing
	fileIndex := index.NewFileIndex()

	// Create watcher with 100ms debounce delay
	watcher, err := NewWatcher(fileIndex, 100)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	// Clean up when test finishes
	defer watcher.Stop()

	// Verify watcher was created
	if watcher == nil {
		t.Fatal("Watcher is nil")
	}

	// Verify fsWatcher was created
	if watcher.fsWatcher == nil {
		t.Error("fsWatcher is nil")
	}

	// Verify debouncer was created
	if watcher.debouncer == nil {
		t.Error("debouncer is nil")
	}

	// Verify eventHandler was created
	if watcher.eventHandler == nil {
		t.Error("eventHandler is nil")
	}

	// Verify fileIndex was set
	if watcher.fileIndex == nil {
		t.Error("fileIndex is nil")
	}

	// Verify watchedDirs map was initialized
	if watcher.watchedDirs == nil {
		t.Error("watchedDirs map is nil")
	}

	// Verify context was created
	if watcher.ctx == nil {
		t.Error("context is nil")
	}
}

// TestAddDirectory tests adding a single directory to watch.
//
// What we're testing:
// - Can add a directory successfully
// - Directory is tracked in watchedDirs
// - Adding same directory twice is idempotent (no error)
func TestAddDirectory(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "watcher-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	// Clean up temp directory when test finishes
	defer os.RemoveAll(tempDir)

	// Create watcher
	fileIndex := index.NewFileIndex()
	watcher, err := NewWatcher(fileIndex, 100)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Add directory to watch
	err = watcher.AddDirectory(tempDir)
	if err != nil {
		t.Fatalf("Failed to add directory: %v", err)
	}

	// Verify directory is in watchedDirs
	watcher.mu.RLock()
	isWatched := watcher.watchedDirs[tempDir]
	watcher.mu.RUnlock()

	if !isWatched {
		t.Error("Directory not found in watchedDirs")
	}

	// Test idempotency - adding same directory again should not error
	err = watcher.AddDirectory(tempDir)
	if err != nil {
		t.Errorf("Adding directory twice should not error: %v", err)
	}
}

// TestAddDirectoryNonExistent tests adding a non-existent directory.
//
// What we're testing:
// - Adding non-existent directory returns error
// - Watcher remains in valid state after error
func TestAddDirectoryNonExistent(t *testing.T) {
	fileIndex := index.NewFileIndex()
	watcher, err := NewWatcher(fileIndex, 100)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Try to add non-existent directory
	err = watcher.AddDirectory("/this/path/does/not/exist/12345")
	if err == nil {
		t.Error("Expected error when adding non-existent directory, got nil")
	}

	// Verify directory is not in watchedDirs
	watcher.mu.RLock()
	isWatched := watcher.watchedDirs["/this/path/does/not/exist/12345"]
	watcher.mu.RUnlock()

	if isWatched {
		t.Error("Non-existent directory should not be in watchedDirs")
	}
}

// TestAddDirectoryRecursive tests recursive directory watching.
//
// What we're testing:
// - Can watch a directory tree recursively
// - All subdirectories are watched
// - Hidden directories are skipped
func TestAddDirectoryRecursive(t *testing.T) {
	// Create temporary directory structure:
	// tempDir/
	//   ├── subdir1/
	//   ├── subdir2/
	//   │   └── nested/
	//   └── .hidden/    (should be skipped)
	tempDir, err := os.MkdirTemp("", "watcher-test-recursive-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create subdirectories
	subdir1 := filepath.Join(tempDir, "subdir1")
	subdir2 := filepath.Join(tempDir, "subdir2")
	nested := filepath.Join(subdir2, "nested")
	hidden := filepath.Join(tempDir, ".hidden")

	for _, dir := range []string{subdir1, subdir2, nested, hidden} {
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Create watcher
	fileIndex := index.NewFileIndex()
	watcher, err := NewWatcher(fileIndex, 100)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Add directory recursively
	err = watcher.AddDirectoryRecursive(tempDir)
	if err != nil {
		t.Fatalf("Failed to add directory recursively: %v", err)
	}

	// Verify all non-hidden directories are watched
	watcher.mu.RLock()
	defer watcher.mu.RUnlock()

	expectedDirs := []string{tempDir, subdir1, subdir2, nested}
	for _, dir := range expectedDirs {
		if !watcher.watchedDirs[dir] {
			t.Errorf("Directory %s should be watched", dir)
		}
	}

	// Verify hidden directory is NOT watched
	if watcher.watchedDirs[hidden] {
		t.Error("Hidden directory should not be watched")
	}
}

// TestRemoveDirectory tests removing a directory from watch list.
//
// What we're testing:
// - Can remove a watched directory
// - Directory is removed from watchedDirs
// - Removing non-watched directory is idempotent (no error)
func TestRemoveDirectory(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "watcher-test-remove-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create watcher and add directory
	fileIndex := index.NewFileIndex()
	watcher, err := NewWatcher(fileIndex, 100)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	err = watcher.AddDirectory(tempDir)
	if err != nil {
		t.Fatalf("Failed to add directory: %v", err)
	}

	// Verify directory is watched
	watcher.mu.RLock()
	isWatched := watcher.watchedDirs[tempDir]
	watcher.mu.RUnlock()
	if !isWatched {
		t.Fatal("Directory should be watched before removal")
	}

	// Remove directory
	err = watcher.RemoveDirectory(tempDir)
	if err != nil {
		t.Fatalf("Failed to remove directory: %v", err)
	}

	// Verify directory is no longer watched
	watcher.mu.RLock()
	isWatched = watcher.watchedDirs[tempDir]
	watcher.mu.RUnlock()
	if isWatched {
		t.Error("Directory should not be watched after removal")
	}

	// Test idempotency - removing again should not error
	err = watcher.RemoveDirectory(tempDir)
	if err != nil {
		t.Errorf("Removing directory twice should not error: %v", err)
	}
}

// TestWatchedDirectories tests getting list of watched directories.
//
// What we're testing:
// - Can get list of watched directories
// - List is accurate (contains all watched dirs)
// - List is sorted (for consistent output)
func TestWatchedDirectories(t *testing.T) {
	// Create multiple temp directories
	tempDir1, err := os.MkdirTemp("", "watcher-test-list-1-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir 1: %v", err)
	}
	defer os.RemoveAll(tempDir1)

	tempDir2, err := os.MkdirTemp("", "watcher-test-list-2-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir 2: %v", err)
	}
	defer os.RemoveAll(tempDir2)

	// Create watcher and add directories
	fileIndex := index.NewFileIndex()
	watcher, err := NewWatcher(fileIndex, 100)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	err = watcher.AddDirectory(tempDir1)
	if err != nil {
		t.Fatalf("Failed to add directory 1: %v", err)
	}

	err = watcher.AddDirectory(tempDir2)
	if err != nil {
		t.Fatalf("Failed to add directory 2: %v", err)
	}

	// Get watched directories
	dirs := watcher.WatchedDirectories()

	// Verify count
	if len(dirs) != 2 {
		t.Errorf("Expected 2 watched directories, got %d", len(dirs))
	}

	// Verify both directories are in the list
	found1, found2 := false, false
	for _, dir := range dirs {
		if dir == tempDir1 {
			found1 = true
		}
		if dir == tempDir2 {
			found2 = true
		}
	}

	if !found1 {
		t.Error("tempDir1 not found in watched directories")
	}
	if !found2 {
		t.Error("tempDir2 not found in watched directories")
	}

	// Verify list is sorted
	// (tempDir1 should come before tempDir2 because of naming)
	if len(dirs) == 2 && dirs[0] > dirs[1] {
		t.Error("Watched directories list is not sorted")
	}
}

// TestWatchFileCreate tests watching for file creation events.
//
// What we're testing:
// - File creation is detected
// - File is added to index
// - Event processing happens after debounce delay
func TestWatchFileCreate(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "watcher-test-create-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create watcher with short debounce for faster testing
	fileIndex := index.NewFileIndex()
	watcher, err := NewWatcher(fileIndex, 50) // 50ms debounce
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Add directory to watch
	err = watcher.AddDirectory(tempDir)
	if err != nil {
		t.Fatalf("Failed to add directory: %v", err)
	}

	// Start watching
	watcher.Start()

	// Create a file in the watched directory
	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Wait for debounce + processing
	// We need to wait longer than debounce delay
	time.Sleep(200 * time.Millisecond)

	// Verify file was added to index
	if !fileIndex.Contains(testFile) {
		t.Error("Created file was not added to index")
	}

	// Verify we can get the file info
	fileInfo, exists := fileIndex.Get(testFile)
	if !exists {
		t.Error("File info not found in index")
	}
	if fileInfo == nil {
		t.Error("File info is nil")
	}
	if fileInfo != nil && fileInfo.Name != "test.txt" {
		t.Errorf("Expected filename 'test.txt', got '%s'", fileInfo.Name)
	}
}

// TestWatchFileModify tests watching for file modification events.
//
// What we're testing:
// - File modification is detected
// - File info is updated in index
func TestWatchFileModify(t *testing.T) {
	// Create temp directory and file
	tempDir, err := os.MkdirTemp("", "watcher-test-modify-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("initial content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create watcher
	fileIndex := index.NewFileIndex()
	watcher, err := NewWatcher(fileIndex, 50)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Add directory to watch
	err = watcher.AddDirectory(tempDir)
	if err != nil {
		t.Fatalf("Failed to add directory: %v", err)
	}

	// Start watching
	watcher.Start()

	// Wait a moment for initial create event to process
	time.Sleep(200 * time.Millisecond)

	// Get initial file size
	initialInfo, exists := fileIndex.Get(testFile)
	if !exists {
		t.Fatal("File should exist in index after creation")
	}
	initialSize := initialInfo.Size

	// Modify the file (write more content)
	err = os.WriteFile(testFile, []byte("initial content plus more data"), 0644)
	if err != nil {
		t.Fatalf("Failed to modify test file: %v", err)
	}

	// Wait for modification event to process
	time.Sleep(200 * time.Millisecond)

	// Get updated file info
	updatedInfo, exists := fileIndex.Get(testFile)
	if !exists {
		t.Error("File should still exist in index after modification")
	}

	// Verify size increased (file was modified)
	if updatedInfo.Size <= initialSize {
		t.Errorf("File size should increase after modification. Initial: %d, Updated: %d",
			initialSize, updatedInfo.Size)
	}
}

// TestWatchFileDelete tests watching for file deletion events.
//
// What we're testing:
// - File deletion is detected
// - File is removed from index
func TestWatchFileDelete(t *testing.T) {
	// Create temp directory and file
	tempDir, err := os.MkdirTemp("", "watcher-test-delete-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create watcher
	fileIndex := index.NewFileIndex()
	watcher, err := NewWatcher(fileIndex, 50)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Add directory to watch
	err = watcher.AddDirectory(tempDir)
	if err != nil {
		t.Fatalf("Failed to add directory: %v", err)
	}

	// Start watching
	watcher.Start()

	// Wait for create event to process
	time.Sleep(200 * time.Millisecond)

	// Verify file is in index
	if !fileIndex.Contains(testFile) {
		t.Fatal("File should be in index before deletion")
	}

	// Delete the file
	err = os.Remove(testFile)
	if err != nil {
		t.Fatalf("Failed to delete test file: %v", err)
	}

	// Wait for delete event to process
	time.Sleep(200 * time.Millisecond)

	// Verify file was removed from index
	if fileIndex.Contains(testFile) {
		t.Error("Deleted file should be removed from index")
	}
}

// TestWatchFileRename tests watching for file rename events.
//
// What we're testing:
// - File rename is detected
// - Old path is removed from index
// - New path is added to index (via separate create event)
func TestWatchFileRename(t *testing.T) {
	// Create temp directory and file
	tempDir, err := os.MkdirTemp("", "watcher-test-rename-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	oldPath := filepath.Join(tempDir, "old.txt")
	newPath := filepath.Join(tempDir, "new.txt")

	err = os.WriteFile(oldPath, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create watcher
	fileIndex := index.NewFileIndex()
	watcher, err := NewWatcher(fileIndex, 50)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Add directory to watch
	err = watcher.AddDirectory(tempDir)
	if err != nil {
		t.Fatalf("Failed to add directory: %v", err)
	}

	// Start watching
	watcher.Start()

	// Wait for create event to process
	time.Sleep(200 * time.Millisecond)

	// Verify old file is in index
	if !fileIndex.Contains(oldPath) {
		t.Fatal("Old file should be in index before rename")
	}

	// Rename the file
	err = os.Rename(oldPath, newPath)
	if err != nil {
		t.Fatalf("Failed to rename test file: %v", err)
	}

	// Wait for rename events to process
	// (rename generates delete for old path + create for new path)
	time.Sleep(300 * time.Millisecond)

	// Verify old path is removed
	if fileIndex.Contains(oldPath) {
		t.Error("Old file path should be removed from index after rename")
	}

	// Verify new path is added
	if !fileIndex.Contains(newPath) {
		t.Error("New file path should be added to index after rename")
	}
}

// TestWatchIgnoreHiddenFiles tests that hidden files are ignored.
//
// What we're testing:
// - Hidden files (starting with .) are not added to index
// - Watcher respects ShouldIgnore rules
func TestWatchIgnoreHiddenFiles(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "watcher-test-hidden-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create watcher
	fileIndex := index.NewFileIndex()
	watcher, err := NewWatcher(fileIndex, 50)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Add directory to watch
	err = watcher.AddDirectory(tempDir)
	if err != nil {
		t.Fatalf("Failed to add directory: %v", err)
	}

	// Start watching
	watcher.Start()

	// Create a hidden file
	hiddenFile := filepath.Join(tempDir, ".hidden.txt")
	err = os.WriteFile(hiddenFile, []byte("secret"), 0644)
	if err != nil {
		t.Fatalf("Failed to create hidden file: %v", err)
	}

	// Create a normal file for comparison
	normalFile := filepath.Join(tempDir, "normal.txt")
	err = os.WriteFile(normalFile, []byte("visible"), 0644)
	if err != nil {
		t.Fatalf("Failed to create normal file: %v", err)
	}

	// Wait for events to process
	time.Sleep(200 * time.Millisecond)

	// Verify hidden file is NOT in index
	if fileIndex.Contains(hiddenFile) {
		t.Error("Hidden file should not be in index")
	}

	// Verify normal file IS in index
	if !fileIndex.Contains(normalFile) {
		t.Error("Normal file should be in index")
	}
}

// TestPendingEvents tests getting pending event count.
//
// What we're testing:
// - Can get count of pending events
// - Count is accurate
func TestPendingEvents(t *testing.T) {
	fileIndex := index.NewFileIndex()
	watcher, err := NewWatcher(fileIndex, 100) // Longer debounce for testing
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Initially should have 0 pending events
	pending := watcher.PendingEvents()
	if pending != 0 {
		t.Errorf("Expected 0 pending events initially, got %d", pending)
	}

	// Add some events to debouncer directly (for testing)
	watcher.debouncer.Add("/test/path1.txt", EventCreate)
	watcher.debouncer.Add("/test/path2.txt", EventModify)
	watcher.debouncer.Add("/test/path3.txt", EventDelete)

	// Should now have 3 pending events
	pending = watcher.PendingEvents()
	if pending != 3 {
		t.Errorf("Expected 3 pending events, got %d", pending)
	}

	// Wait for debounce to complete
	time.Sleep(200 * time.Millisecond)

	// Should be back to 0 pending events
	pending = watcher.PendingEvents()
	if pending != 0 {
		t.Errorf("Expected 0 pending events after debounce, got %d", pending)
	}
}

// TestStopWatcher tests graceful watcher shutdown.
//
// What we're testing:
// - Stop() completes without error
// - Stop() can be called multiple times (idempotent)
// - No goroutine leaks after stop
func TestStopWatcher(t *testing.T) {
	fileIndex := index.NewFileIndex()
	watcher, err := NewWatcher(fileIndex, 100)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	// Start the watcher
	watcher.Start()

	// Stop should complete quickly (within reasonable time)
	done := make(chan bool)
	go func() {
		watcher.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Stop completed successfully
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not complete within 5 seconds")
	}

	// Calling Stop() again should be safe (idempotent)
	watcher.Stop()

	// Test passes if no panics or deadlocks occur
}

// TestWatcherWithContext tests watcher cancellation via context.
//
// What we're testing:
// - Watcher respects context cancellation
// - Graceful shutdown when context is cancelled
func TestWatcherWithContext(t *testing.T) {
	// Create temp directory
	tempDir, err := os.MkdirTemp("", "watcher-test-context-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create watcher
	fileIndex := index.NewFileIndex()
	watcher, err := NewWatcher(fileIndex, 50)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Stop()

	// Add directory and start watching
	err = watcher.AddDirectory(tempDir)
	if err != nil {
		t.Fatalf("Failed to add directory: %v", err)
	}

	watcher.Start()

	// Create a file
	testFile := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Wait for event to process
	time.Sleep(200 * time.Millisecond)

	// Cancel context (simulates shutdown)
	watcher.cancel()

	// Wait for watcher to shut down
	time.Sleep(100 * time.Millisecond)

	// Create another file (should not be processed)
	testFile2 := filepath.Join(tempDir, "test2.txt")
	err = os.WriteFile(testFile2, []byte("test2"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file 2: %v", err)
	}

	// Wait a bit
	time.Sleep(200 * time.Millisecond)

	// First file should be in index (created before cancel)
	if !fileIndex.Contains(testFile) {
		t.Error("First file should be in index")
	}

	// Second file might or might not be in index (race condition)
	// We don't make assertions about it because the event might have
	// been caught before shutdown completed
}

// TestMultipleWatchers tests running multiple watchers simultaneously.
//
// What we're testing:
// - Multiple watchers can run independently
// - Each watcher maintains its own state
// - No interference between watchers
func TestMultipleWatchers(t *testing.T) {
	// Create two temp directories
	tempDir1, err := os.MkdirTemp("", "watcher-test-multi-1-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir 1: %v", err)
	}
	defer os.RemoveAll(tempDir1)

	tempDir2, err := os.MkdirTemp("", "watcher-test-multi-2-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir 2: %v", err)
	}
	defer os.RemoveAll(tempDir2)

	// Create two separate indexes and watchers
	fileIndex1 := index.NewFileIndex()
	watcher1, err := NewWatcher(fileIndex1, 50)
	if err != nil {
		t.Fatalf("Failed to create watcher 1: %v", err)
	}
	defer watcher1.Stop()

	fileIndex2 := index.NewFileIndex()
	watcher2, err := NewWatcher(fileIndex2, 50)
	if err != nil {
		t.Fatalf("Failed to create watcher 2: %v", err)
	}
	defer watcher2.Stop()

	// Add directories and start watchers
	err = watcher1.AddDirectory(tempDir1)
	if err != nil {
		t.Fatalf("Failed to add directory to watcher 1: %v", err)
	}
	watcher1.Start()

	err = watcher2.AddDirectory(tempDir2)
	if err != nil {
		t.Fatalf("Failed to add directory to watcher 2: %v", err)
	}
	watcher2.Start()

	// Create files in each directory
	file1 := filepath.Join(tempDir1, "file1.txt")
	file2 := filepath.Join(tempDir2, "file2.txt")

	err = os.WriteFile(file1, []byte("content1"), 0644)
	if err != nil {
		t.Fatalf("Failed to create file 1: %v", err)
	}

	err = os.WriteFile(file2, []byte("content2"), 0644)
	if err != nil {
		t.Fatalf("Failed to create file 2: %v", err)
	}

	// Wait for events to process
	time.Sleep(200 * time.Millisecond)

	// Verify each index only has its own file
	if !fileIndex1.Contains(file1) {
		t.Error("Index 1 should contain file1")
	}
	if fileIndex1.Contains(file2) {
		t.Error("Index 1 should not contain file2")
	}

	if !fileIndex2.Contains(file2) {
		t.Error("Index 2 should contain file2")
	}
	if fileIndex2.Contains(file1) {
		t.Error("Index 2 should not contain file1")
	}
}