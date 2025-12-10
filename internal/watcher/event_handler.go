package watcher

import (
	"os"
	"path/filepath"

	"github.com/makokhawanjala/searchlight/internal/indexer"
)

// EventHandler processes debounced file system events.
//
// Why separate from Debouncer?
// - Single Responsibility: Debouncer handles timing, EventHandler handles logic
// - Testability: Can test event handling without dealing with timers
// - Flexibility: Easy to swap different handling strategies
//
// Architecture:
// Watcher → Debouncer → EventHandler → Index
//
// The EventHandler is the bridge between file system events and the index.
type EventHandler struct {
	// onAdd is called when a file should be added to the index
	// This is typically FileIndex.Add()
	onAdd func(*indexer.FileInfo)

	// onUpdate is called when a file should be updated in the index
	// This is typically FileIndex.Add() (which handles updates)
	onUpdate func(*indexer.FileInfo)

	// onDelete is called when a file should be removed from the index
	// This is typically FileIndex.Remove()
	onDelete func(path string)
}

// NewEventHandler creates a new event handler.
//
// Parameters:
//   - onAdd: Function to call when adding a file to the index
//   - onUpdate: Function to call when updating a file in the index
//   - onDelete: Function to call when removing a file from the index
//
// Returns:
//   - *EventHandler: Ready to process events
//
// Example:
//   handler := NewEventHandler(
//       fileIndex.Add,      // Add or update file
//       fileIndex.Add,      // Update is same as add
//       fileIndex.Remove,   // Remove file
//   )
func NewEventHandler(
	onAdd func(*indexer.FileInfo),
	onUpdate func(*indexer.FileInfo),
	onDelete func(path string),
) *EventHandler {
	return &EventHandler{
		onAdd:    onAdd,
		onUpdate: onUpdate,
		onDelete: onDelete,
	}
}

// Handle processes a debounced file system event.
//
// This is the main entry point for event processing.
// It's called by the Debouncer after events have settled.
//
// Event Flow:
// 1. Determine what happened (create/modify/delete/rename)
// 2. For create/modify: Read file info and add to index
// 3. For delete: Remove from index
// 4. For rename: Treat as delete + create (handled at watcher level)
//
// Error Handling:
// - If file doesn't exist during create/modify: Ignore (race condition)
// - If file info can't be read: Ignore (permission error or file deleted)
// - Errors are silent because:
//   * User might delete file right after event
//   * File might be temporarily inaccessible
//   * We don't want to crash on transient errors
//
// Parameters:
//   - event: The debounced file system event
//
// Design Decision: Why no error return?
// - Errors are expected and normal (files deleted, permissions changed)
// - Caller can't do anything useful with the error
// - Silent failures are acceptable for file watching
// - Critical errors (like index corruption) are handled at index level
func (eh *EventHandler) Handle(event DebouncedEvent) {
	switch event.Type {
	case EventCreate, EventModify:
		// File was created or modified - add/update in index
		eh.handleCreateOrModify(event.Path)

	case EventDelete:
		// File was deleted - remove from index
		eh.handleDelete(event.Path)

	case EventRename:
		// Rename is handled at watcher level as delete + create
		// If we receive a rename here, treat it as a delete
		// The new name will come as a separate create event
		eh.handleDelete(event.Path)
	}
}

// handleCreateOrModify processes file creation and modification events.
//
// Process:
// 1. Check if file still exists (might have been deleted)
// 2. Get file metadata using os.Stat()
// 3. Create FileInfo from metadata
// 4. Add to index (or update if already exists)
//
// Edge Cases:
// - File deleted between event and handling: Silently ignore
// - Permission denied: Silently ignore (can't index anyway)
// - Directory created: Add to index (directories are indexed too)
// - Symlink: Follow symlink and index target (os.Stat follows symlinks)
//
// Why both Create and Modify use same handler?
// - Both need same operation: read file info and update index
// - Index.Add() handles both insert and update
// - Simplifies code and reduces duplication
//
// Parameters:
//   - path: File path to add or update
func (eh *EventHandler) handleCreateOrModify(path string) {
	// Get file information
	// os.Stat follows symlinks (use os.Lstat if you don't want this)
	info, err := os.Stat(path)
	if err != nil {
		// File might have been deleted, or we don't have permissions
		// This is normal - just ignore the event
		//
		// Common scenarios:
		// - File created then immediately deleted
		// - Temp file that no longer exists
		// - Permission denied on protected file
		//
		// We could log this at debug level in Phase 12 (Logging)
		return
	}

	// Create FileInfo from os.FileInfo
	fileInfo := indexer.NewFileInfo(path, info)

	// Decide whether this is an add or update
	// Actually, we don't need to decide - both call the same function
	// The index handles the distinction internally
	//
	// For now, we'll always call onAdd (which handles updates too)
	// In the future, we might want to distinguish for:
	// - Different logging messages
	// - Metrics (track adds vs updates separately)
	// - Notification types (tell UI "file created" vs "file modified")
	if eh.onAdd != nil {
		eh.onAdd(fileInfo)
	}

	// Note: We could also call onUpdate separately, but it's redundant
	// if both do the same thing (FileIndex.Add handles both cases)
}

// handleDelete processes file deletion events.
//
// Process:
// 1. Remove file from index
// 2. That's it - no need to check if file exists (it shouldn't!)
//
// Why so simple?
// - Delete events are straightforward
// - No need to read file metadata (file is gone)
// - Index.Remove() is idempotent (safe to remove non-existent file)
//
// Edge Cases:
// - File doesn't exist in index: No-op (safe)
// - Directory deleted: Remove from index
// - Multiple delete events: Only first has effect
//
// Parameters:
//   - path: File path to remove from index
func (eh *EventHandler) handleDelete(path string) {
	if eh.onDelete != nil {
		eh.onDelete(path)
	}
}

// HandleBatch processes multiple events in a batch.
//
// Why batch processing?
// - More efficient than one-by-one
// - Can optimize operations (e.g., single index update)
// - Useful for initial directory scans
//
// Use Cases:
// - Startup: Process all files in watched directory
// - Reconnect: Catch up after watcher was stopped
// - Testing: Process multiple events at once
//
// Parameters:
//   - events: Slice of events to process
//
// Implementation Note:
// - Currently just calls Handle() for each event
// - Future optimization: Could batch index operations
func (eh *EventHandler) HandleBatch(events []DebouncedEvent) {
	for _, event := range events {
		eh.Handle(event)
	}
}

// ShouldIgnore determines if a path should be ignored.
//
// Why ignore certain paths?
// - Hidden files (starting with .)
// - Temporary files (.tmp, .swp, ~)
// - System files (.DS_Store, Thumbs.db)
// - Version control (.git, .svn)
//
// Default Ignore Patterns:
// - Hidden files: .*
// - Temp files: *.tmp, *.swp, *~
// - System: .DS_Store, Thumbs.db, desktop.ini
// - VCS: .git, .svn, .hg
//
// Parameters:
//   - path: File path to check
//
// Returns:
//   - bool: true if path should be ignored, false otherwise
//
// Future Enhancement (Phase 8+):
// - Make ignore patterns configurable
// - Support glob patterns
// - Support regex patterns
// - Per-directory ignore rules (.searchlightignore files)
func ShouldIgnore(path string) bool {
	// Get the base name (last component of path)
	base := filepath.Base(path)

	// Ignore hidden files (starting with .)
	if len(base) > 0 && base[0] == '.' {
		return true
	}

	// Ignore common temporary files
	ext := filepath.Ext(base)
	switch ext {
	case ".tmp", ".swp", ".bak":
		return true
	}

	// Ignore files ending with ~
	if len(base) > 0 && base[len(base)-1] == '~' {
		return true
	}

	// Ignore common system files
	switch base {
	case "Thumbs.db", "desktop.ini", ".DS_Store":
		return true
	}

	// Don't ignore by default
	return false
}

// IsDirectory checks if a path is a directory.
//
// Why a helper function?
// - Used in multiple places
// - Handles errors gracefully
// - Consistent error handling
//
// Parameters:
//   - path: Path to check
//
// Returns:
//   - bool: true if path is a directory, false otherwise
//   - error: error if stat fails
//
// Usage:
//   if isDir, _ := IsDirectory(path); isDir {
//       // Handle directory
//   }
func IsDirectory(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

// GetFileInfo reads file metadata for a path.
//
// Why a helper function?
// - Consistent error handling
// - Abstraction over os.Stat
// - Easy to add caching in the future
//
// Parameters:
//   - path: File path to read
//
// Returns:
//   - *indexer.FileInfo: File metadata
//   - error: error if file doesn't exist or can't be read
//
// Example:
//   fileInfo, err := GetFileInfo("/home/user/file.txt")
//   if err != nil {
//       log.Printf("Can't read file: %v", err)
//       return
//   }
//   fmt.Printf("Size: %s\n", fileInfo.HumanSize())
func GetFileInfo(path string) (*indexer.FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	return indexer.NewFileInfo(path, info), nil
}