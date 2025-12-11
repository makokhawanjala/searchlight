// Package watcher provides file system watching functionality using fsnotify.
//
// The Watcher monitors directories for changes and automatically updates the index
// when files are created, modified, deleted, or renamed.
//
// Architecture Flow:
// fsnotify â†' Watcher â†' Debouncer â†' EventHandler â†' Index
//
// Why this architecture?
// - fsnotify: Low-level OS notifications (fast, efficient)
// - Watcher: Manages fsnotify, converts OS events to our events
// - Debouncer: Consolidates rapid events (prevents index thrashing)
// - EventHandler: Business logic for index updates
// - Index: The actual data structure
//
// This separation makes each component simple, testable, and maintainable.
package watcher

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/makokhawanjala/searchlight/internal/index"
)

// Watcher monitors file system changes and updates the index.
//
// Design Decisions:
// - Uses fsnotify for cross-platform file watching
// - Debouncer prevents event storms
// - EventHandler separates watching from index logic
// - Context for clean shutdown
//
// Thread Safety:
// - Can be used from multiple goroutines
// - Internal locking protects shared state
type Watcher struct {
	// fsWatcher is the underlying fsnotify watcher
	// This is the low-level component that talks to the OS
	fsWatcher *fsnotify.Watcher

	// debouncer consolidates rapid file system events
	// Without this, saving a file could trigger 5+ index updates
	debouncer *Debouncer

	// eventHandler processes debounced events
	// This is where we decide what to do with each event
	eventHandler *EventHandler

	// fileIndex is the index we're keeping up to date
	// We hold a reference so we can add/remove files
	fileIndex *index.FileIndex

	// watchedDirs tracks which directories we're watching
	// Key: directory path
	// Value: true (using map as a set)
	//
	// Why track this?
	// - Prevent adding the same directory twice
	// - Know what we're watching (for debugging/stats)
	// - Clean shutdown (know what to stop watching)
	watchedDirs map[string]bool

	// mu protects watchedDirs map
	// Why separate mutex?
	// - Only needed for watchedDirs access
	// - fsWatcher has its own internal locking
	// - Debouncer has its own mutex
	// - Smaller critical sections = better concurrency
	mu sync.RWMutex

	// ctx is used for cancellation and shutdown
	// When ctx is cancelled, watcher stops processing events
	ctx context.Context

	// cancel is called to stop the watcher
	// Calling cancel() triggers graceful shutdown
	cancel context.CancelFunc

	// wg tracks active goroutines
	// Used to wait for clean shutdown (all goroutines finished)
	wg sync.WaitGroup
}

// NewWatcher creates a new file system watcher.
//
// Parameters:
//   - fileIndex: The index to keep updated
//   - debounceDelay: How long to wait after events before updating index
//
// Returns:
//   - *Watcher: Ready to watch directories
//   - error: If fsnotify.NewWatcher() fails (rare, usually means OS limitation)
//
// Example:
//   watcher, err := NewWatcher(fileIndex, 100*time.Millisecond)
//   if err != nil {
//       log.Fatal(err)
//   }
//   defer watcher.Stop()
//
// Why can this fail?
// - OS limits on number of file descriptors
// - OS limits on number of watches (inotify on Linux)
// - Permission issues
func NewWatcher(fileIndex *index.FileIndex, debounceDelay int64) (*Watcher, error) {
	// Create the fsnotify watcher
	// This talks directly to the OS (inotify on Linux, FSEvents on macOS, etc.)
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}

	// Create context for shutdown control
	// We create our own context so we can cancel it independently
	ctx, cancel := context.WithCancel(context.Background())

	// Create the watcher struct
	w := &Watcher{
		fsWatcher:   fsWatcher,
		fileIndex:   fileIndex,
		watchedDirs: make(map[string]bool),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Create event handler
	// This connects file system events to index operations
	w.eventHandler = NewEventHandler(
		// onAdd: When a file is created or modified, add it to index
		// FileIndex.Add handles both insert and update
		fileIndex.Add,

		// onUpdate: Same as onAdd (FileIndex.Add handles both)
		fileIndex.Add,

		// onDelete: When a file is deleted, remove it from index
		func(path string) { fileIndex.Remove(path) },
	)

	// Create debouncer with our event handler
	// Debouncer will call eventHandler.Handle after events settle
	w.debouncer = NewDebouncer(time.Duration(debounceDelay)*time.Millisecond, func(event DebouncedEvent) {
		w.eventHandler.Handle(event)
	})

	return w, nil
}

// AddDirectory adds a directory to the watch list.
//
// This will:
// 1. Add the directory to fsnotify
// 2. Track it in watchedDirs
//
// Note: Does NOT recursively watch subdirectories!
// - fsnotify doesn't support recursive watching on all platforms
// - To watch subdirectories, call AddDirectory for each one
// - Or use AddDirectoryRecursive (implemented below)
//
// Parameters:
//   - path: Directory path to watch
//
// Returns:
//   - error: If directory doesn't exist or can't be watched
//
// Thread Safety:
// - Safe to call from multiple goroutines
// - Uses mutex to protect watchedDirs
func (w *Watcher) AddDirectory(path string) error {
	// Clean the path to normalize it
	// This prevents watching "/home/user/" and "/home/user" as different dirs
	path = filepath.Clean(path)

	// Lock watchedDirs to check and update
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if we're already watching this directory
	if w.watchedDirs[path] {
		// Already watching - this is not an error
		// Just silently succeed (idempotent operation)
		return nil
	}

	// Add to fsnotify watcher
	// This tells the OS to send us events for this directory
	err := w.fsWatcher.Add(path)
	if err != nil {
		return fmt.Errorf("failed to watch directory %s: %w", path, err)
	}

	// Remember that we're watching this directory
	w.watchedDirs[path] = true

	return nil
}

// AddDirectoryRecursive adds a directory and all its subdirectories to the watch list.
//
// This is useful for watching entire directory trees.
//
// Process:
// 1. Walk the directory tree
// 2. Add each directory to fsnotify
// 3. Track all directories in watchedDirs
//
// Warning: Can be slow for large directory trees!
// - 10,000 directories = 10,000 OS watch handles
// - Some OSes have limits (inotify on Linux)
//
// Parameters:
//   - rootPath: Root directory to watch recursively
//
// Returns:
//   - error: If walk fails or any directory can't be watched
//
// Example:
//   err := watcher.AddDirectoryRecursive("/home/user/documents")
//   // Now watching documents and all subdirectories
func (w *Watcher) AddDirectoryRecursive(rootPath string) error {
	// Walk the directory tree
	// filepath.Walk visits every file and directory
	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Error accessing this path (permission denied, etc.)
			// Log it but continue walking (don't fail entire operation)
			return nil
		}

		// Only watch directories (fsnotify watches directories, not files)
		if info.IsDir() {
			// Skip hidden directories (starting with .)
			// Common cases: .git, .svn, .cache
			if ShouldIgnore(path) {
				// filepath.SkipDir tells Walk to skip this directory and its contents
				return filepath.SkipDir
			}

			// Add this directory to watch list
			if err := w.AddDirectory(path); err != nil {
				// Log error but continue (don't fail entire operation)
				// Some directories might not be watchable (permissions, etc.)
				return nil
			}
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory tree %s: %w", rootPath, err)
	}

	return nil
}

// RemoveDirectory removes a directory from the watch list.
//
// Parameters:
//   - path: Directory path to stop watching
//
// Returns:
//   - error: If removal fails
//
// Thread Safety:
// - Safe to call from multiple goroutines
func (w *Watcher) RemoveDirectory(path string) error {
	path = filepath.Clean(path)

	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if we're watching this directory
	if !w.watchedDirs[path] {
		// Not watching - this is not an error
		return nil
	}

	// Remove from fsnotify
	err := w.fsWatcher.Remove(path)
	if err != nil {
		return fmt.Errorf("failed to stop watching directory %s: %w", path, err)
	}

	// Remove from our tracking map
	delete(w.watchedDirs, path)

	return nil
}

// Start begins watching for file system events.
//
// This starts a goroutine that:
// 1. Listens for fsnotify events
// 2. Converts them to our event types
// 3. Sends them to the debouncer
// 4. Handles errors
//
// Must be called after AddDirectory/AddDirectoryRecursive!
//
// Example:
//   watcher.AddDirectoryRecursive("/home/user/documents")
//   watcher.Start()  // Now monitoring for changes
//   // ... later ...
//   watcher.Stop()   // Clean shutdown
func (w *Watcher) Start() {
	// Add to wait group so we can wait for clean shutdown
	w.wg.Add(1)

	// Start event processing goroutine
	go w.watchLoop()
}

// watchLoop is the main event processing loop.
//
// This runs in a separate goroutine and:
// - Listens for fsnotify events
// - Converts OS events to our event types
// - Sends events to debouncer
// - Handles errors
// - Stops when context is cancelled
//
// Why a separate method instead of inline in Start()?
// - Cleaner code organization
// - Easier to understand the event loop
// - Easier to test (can call watchLoop directly in tests)
func (w *Watcher) watchLoop() {
	// Ensure we signal completion when this goroutine exits
	defer w.wg.Done()

	// Main event loop
	// Runs until context is cancelled or fsWatcher is closed
	for {
		select {
		case <-w.ctx.Done():
			// Context cancelled - time to shutdown
			// This happens when Stop() is called
			return

		case event, ok := <-w.fsWatcher.Events:
			// Received a file system event from fsnotify
			//
			// Why check 'ok'?
			// - Channel closed means watcher is shutting down
			// - Prevents panic on closed channel
			if !ok {
				// Events channel closed - watcher is done
				return
			}

			// Process the event
			w.handleFsnotifyEvent(event)

		case err, ok := <-w.fsWatcher.Errors:
			// Received an error from fsnotify
			//
			// Common errors:
			// - File descriptor limit reached
			// - Directory became unavailable
			// - Permission denied
			if !ok {
				// Errors channel closed - watcher is done
				return
			}

			// Handle the error
			w.handleFsnotifyError(err)
		}
	}
}

// handleFsnotifyEvent processes a single fsnotify event.
//
// Converts fsnotify events to our EventType and sends to debouncer.
//
// fsnotify event types:
// - fsnotify.Create: File or directory created
// - fsnotify.Write: File modified
// - fsnotify.Remove: File or directory deleted
// - fsnotify.Rename: File or directory renamed
// - fsnotify.Chmod: File permissions changed
//
// Our handling:
// - Create â†' EventCreate
// - Write â†' EventModify
// - Remove â†' EventDelete
// - Rename â†' EventDelete (old path) + EventCreate (new path, separate event)
// - Chmod â†' Ignore (we don't care about permission changes for searching)
func (w *Watcher) handleFsnotifyEvent(event fsnotify.Event) {
	// Check if we should ignore this path
	// (hidden files, temp files, etc.)
	if ShouldIgnore(event.Name) {
		return
	}

	// Convert fsnotify event to our event type
	// We check the Op field which is a bitmask of operations

	if event.Op&fsnotify.Create == fsnotify.Create {
		// File or directory created
		w.debouncer.Add(event.Name, EventCreate)

		// If it's a directory, we might want to watch it too
		// This handles the case where a new subdirectory is created
		if isDir, _ := IsDirectory(event.Name); isDir {
			// Add new directory to watch list
			// Ignore errors (directory might be deleted before we can watch it)
			_ = w.AddDirectory(event.Name)
		}
	}

	if event.Op&fsnotify.Write == fsnotify.Write {
		// File modified (written to)
		w.debouncer.Add(event.Name, EventModify)
	}

	if event.Op&fsnotify.Remove == fsnotify.Remove {
		// File or directory deleted
		w.debouncer.Add(event.Name, EventDelete)
	}

	if event.Op&fsnotify.Rename == fsnotify.Rename {
		// File or directory renamed
		//
		// Rename is tricky:
		// - We get an event for the old path
		// - We get a separate Create event for the new path
		// - So we handle rename as a delete (old path will be removed)
		// - And the create event will add the new path
		w.debouncer.Add(event.Name, EventDelete)
	}

	// We don't handle fsnotify.Chmod (permission changes)
	// These don't affect whether a file exists or what its name is
	// If needed in the future, we could update file metadata
}

// handleFsnotifyError handles errors from fsnotify.
//
// Common errors:
// - File descriptor limit reached
// - Directory became unavailable (unmounted, deleted while watching)
// - Permission denied
//
// For now, we just log errors.
// In Phase 12 (Logging), we'll use proper structured logging.
func (w *Watcher) handleFsnotifyError(err error) {
	// For now, we'll just print errors
	// Phase 12 will replace this with proper logging
	fmt.Printf("Watcher error: %v\n", err)

	// We don't stop the watcher on error
	// Errors are usually recoverable or affect only specific files
	// The watcher should keep running for other files
}

// Stop gracefully shuts down the watcher.
//
// Process:
// 1. Cancel context (signals watchLoop to exit)
// 2. Flush debouncer (process pending events)
// 3. Close fsWatcher (stops OS notifications)
// 4. Wait for watchLoop goroutine to finish
//
// This ensures:
// - No events are lost (debouncer flushed)
// - Clean OS resource cleanup (fsWatcher closed)
// - No goroutine leaks (wait for completion)
//
// Safe to call multiple times (idempotent).
func (w *Watcher) Stop() {
	// Cancel context to signal watchLoop to exit
	w.cancel()

	// Flush any pending debounced events
	// This ensures no events are lost during shutdown
	w.debouncer.Flush()

	// Close the fsnotify watcher
	// This stops OS notifications and closes the event channels
	// After this, watchLoop will exit when it sees closed channels
	if w.fsWatcher != nil {
		_ = w.fsWatcher.Close()
	}

	// Wait for watchLoop goroutine to finish
	// This ensures clean shutdown before returning
	w.wg.Wait()
}

// WatchedDirectories returns a list of all watched directories.
//
// Use cases:
// - Debugging: What are we watching?
// - UI: Display watched directories
// - Stats: How many directories are we monitoring?
//
// Returns:
//   - []string: Sorted list of watched directory paths
//
// Thread Safety:
// - Safe to call from multiple goroutines
// - Returns a copy (caller can modify without affecting watcher)
func (w *Watcher) WatchedDirectories() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	// Create slice with exact capacity
	dirs := make([]string, 0, len(w.watchedDirs))

	// Copy all directory paths
	for dir := range w.watchedDirs {
		dirs = append(dirs, dir)
	}

	// Sort for consistent output
	// Makes testing easier and output more predictable
	sortStrings(dirs)

	return dirs
}

// PendingEvents returns the number of events waiting to be processed.
//
// Use cases:
// - Monitoring: Are events building up?
// - Testing: Verify events are being processed
// - UI: Show "Processing X changes..."
//
// Returns:
//   - int: Number of events in debouncer queue
func (w *Watcher) PendingEvents() int {
	return w.debouncer.Pending()
}

// sortStrings sorts a slice of strings in place.
//
// Why not use sort.Strings?
// - We will in production! This is a placeholder.
// - In Phase 14 (Complete Test Suite), we'll import "sort"
// - For now, simple bubble sort is clear and works fine
//
// Parameters:
//   - slice: String slice to sort (modified in place)
func sortStrings(slice []string) {
	n := len(slice)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if slice[j] > slice[j+1] {
				slice[j], slice[j+1] = slice[j+1], slice[j]
			}
		}
	}
}