// Package storage provides interfaces and implementations for persisting
// the SearchLight index to disk and loading it back into memory.
//
// Why do we need persistent storage?
// - Fast startup: Load existing index instead of re-scanning everything
// - Durability: Index survives application restarts
// - Backup: Index can be saved periodically or on shutdown
//
// Design Philosophy:
// - Interface-based: Easy to add new storage backends (JSON, Gob, SQLite, etc.)
// - Atomic operations: Prevent corruption with partial writes
// - Graceful degradation: Application continues if load fails
package storage

import (
	"github.com/makokhawanjala/searchlight/internal/index"
)

// Storage defines the interface for persisting and loading the file index.
//
// Why an interface?
// - Abstraction: Callers don't need to know HOW data is stored
// - Flexibility: Can swap storage backends without changing application code
// - Testing: Easy to create mock implementations for tests
// - Future-proof: Can add Redis, PostgreSQL, or other backends later
//
// Design Decisions:
//
// Q: Why separate Save/Load instead of a single "Persist" method?
// A: Different use cases:
//    - Save: Called on shutdown, periodically, or after significant changes
//    - Load: Called only on startup
//    - Explicit operations are clearer than a single method with flags
//
// Q: Why pass FileIndex instead of raw data structures?
// A: Encapsulation - Storage doesn't need to know FileIndex internals
//    FileIndex will provide methods to export/import its data
type Storage interface {
	// Save persists the entire file index to storage.
	//
	// When is this called?
	// - Application shutdown (graceful exit)
	// - Periodic auto-save (every N minutes)
	// - Manual save triggered by user/admin
	// - After significant index changes (e.g., 1000+ files added)
	//
	// Error Handling:
	// - Returns error if save fails (disk full, permissions, corruption)
	// - Should NOT modify the index if save fails
	// - Caller decides whether to retry, log, or alert user
	//
	// Performance Considerations:
	// - Can be slow for large indexes (100k+ files)
	// - Should be called from a goroutine to avoid blocking
	// - Progress reporting would be nice for large saves
	//
	// Thread Safety:
	// - FileIndex handles its own locking
	// - Storage implementation should be safe for concurrent calls
	//
	// Parameters:
	//   - idx: The FileIndex to save
	//
	// Returns:
	//   - error: nil on success, descriptive error on failure
	//
	// Example usage:
	//   if err := storage.Save(fileIndex); err != nil {
	//       log.Errorf("Failed to save index: %v", err)
	//       // Application continues, but index won't persist
	//   }
	Save(idx *index.FileIndex) error

	// Load reads the file index from storage and returns it.
	//
	// When is this called?
	// - Application startup (before starting HTTP server)
	// - Manual reload triggered by admin
	// - Recovery after crash or corruption
	//
	// Behavior on Failure:
	// - Returns error if load fails (file missing, corrupted, wrong format)
	// - Caller typically creates fresh FileIndex and re-scans directories
	// - Application should continue even if load fails
	//
	// Corruption Handling:
	// - Implementation should detect corrupted data
	// - Checksum validation recommended
	// - Backup files can be used for recovery
	//
	// Performance:
	// - Loading should be FAST (ideally < 1 second for 100k files)
	// - This is critical for application startup time
	// - Users expect instant availability after restart
	//
	// Thread Safety:
	// - Load is typically called once at startup (single-threaded)
	// - But implementation should be safe if called concurrently
	//
	// Returns:
	//   - *index.FileIndex: Loaded index ready for searching
	//   - error: nil on success, descriptive error on failure
	//
	// Example usage:
	//   fileIndex, err := storage.Load()
	//   if err != nil {
	//       log.Warnf("Could not load index: %v. Starting fresh...", err)
	//       fileIndex = index.NewFileIndex()
	//       // Trigger full re-scan
	//   } else {
	//       log.Infof("Loaded index with %d files", fileIndex.Size())
	//   }
	Load() (*index.FileIndex, error)

	// Clear removes all persisted data from storage.
	//
	// When is this used?
	// - User wants to reset SearchLight completely
	// - Switching to different root directories
	// - Clearing corrupted data before re-indexing
	// - Testing: clean slate between test runs
	//
	// Behavior:
	// - Deletes storage files/data
	// - Does NOT affect the in-memory FileIndex
	// - Should be idempotent (safe to call multiple times)
	// - Should succeed even if storage was already empty
	//
	// Safety Considerations:
	// - This is a destructive operation!
	// - Consider requiring confirmation in UI
	// - Should NOT clear if Save is in progress
	//
	// Returns:
	//   - error: nil on success, error if deletion fails
	//
	// Example usage:
	//   if err := storage.Clear(); err != nil {
	//       log.Errorf("Failed to clear storage: %v", err)
	//   }
	//   // Now re-index from scratch
	Clear() error

	// Path returns the location where data is stored.
	//
	// Why is this useful?
	// - Logging: "Saved index to /home/user/.searchlight/index.json"
	// - Debugging: Users can inspect the file
	// - Backup: Users can copy the file to another location
	// - Diagnostics: Check file size, modification time, etc.
	//
	// Return Value:
	// - File path for file-based storage (JSON, Gob)
	// - Connection string for database storage (PostgreSQL, Redis)
	// - Empty string if storage is memory-only (testing)
	//
	// Returns:
	//   - string: Storage location identifier
	//
	// Example:
	//   log.Infof("Index stored at: %s", storage.Path())
	Path() string
}

// StorageStats holds statistics about storage operations.
//
// Why a separate stats type?
// - Monitoring: Track storage performance and reliability
// - Diagnostics: Identify save/load bottlenecks
// - User feedback: "Last saved 2 minutes ago"
// - Alerting: Notify if save failure rate is high
//
// Future Enhancements:
// - Add method: GetStats() StorageStats to Storage interface
// - Track compression ratios
// - Monitor storage space usage
// - Record error types and frequencies
type StorageStats struct {
	// LastSaveTime is when the index was last successfully saved
	LastSaveTime int64 `json:"last_save_time"`

	// LastLoadTime is when the index was last successfully loaded
	LastLoadTime int64 `json:"last_load_time"`

	// SaveCount is the total number of successful saves
	SaveCount int `json:"save_count"`

	// LoadCount is the total number of successful loads
	LoadCount int `json:"load_count"`

	// SaveErrors is the number of failed save attempts
	SaveErrors int `json:"save_errors"`

	// LoadErrors is the number of failed load attempts
	LoadErrors int `json:"load_errors"`

	// StorageSize is the size of the persisted data in bytes
	// For JSON: size of .json file
	// For database: size of table/collection
	StorageSize int64 `json:"storage_size"`

	// AverageSaveTime is the average time to save in milliseconds
	AverageSaveTime int64 `json:"average_save_time_ms"`

	// AverageLoadTime is the average time to load in milliseconds
	AverageLoadTime int64 `json:"average_load_time_ms"`
}

// NewStorageStats creates an initialized StorageStats.
//
// Why a constructor?
// - Ensures all fields are properly initialized
// - Can add validation in the future
// - Consistent initialization pattern
func NewStorageStats() *StorageStats {
	return &StorageStats{
		LastSaveTime:    0,
		LastLoadTime:    0,
		SaveCount:       0,
		LoadCount:       0,
		SaveErrors:      0,
		LoadErrors:      0,
		StorageSize:     0,
		AverageSaveTime: 0,
		AverageLoadTime: 0,
	}
}

// Implementation Notes for Future Storage Backends:
//
// JSONStorage (Phase 6.3):
// - Human-readable, easy to debug
// - Good for small-medium indexes (< 100k files)
// - Compression with gzip recommended
// - Atomic writes with temp file + rename
//
// GobStorage (Optional):
// - Binary format, faster than JSON
// - More compact (better for large indexes)
// - Not human-readable (harder to debug)
// - Native Go serialization (no schema management)
//
// SQLiteStorage (Future):
// - SQL queries for advanced filtering
// - Incremental saves (only changed files)
// - ACID guarantees
// - Good for very large indexes (> 500k files)
//
// RedisStorage (Future):
// - In-memory with persistence
// - Sub-millisecond load times
// - Shared index across multiple SearchLight instances
// - Good for distributed deployments
//
// PostgreSQLStorage (Future):
// - Full-text search capabilities
// - Robust concurrency handling
// - Backup and replication built-in
// - Overkill for most use cases, but enterprise-ready