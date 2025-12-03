// Package storage provides JSON-based persistence for the SearchLight index.
//
// Why JSON?
// - Human-readable: Easy to inspect and debug
// - Universal format: Works across languages and platforms
// - Standard library support: encoding/json is battle-tested
// - Self-documenting: Field names make structure obvious
//
// Design Decisions:
// - Gzip compression: Reduces file size by ~80% for text data
// - Versioning: Future-proof for schema changes
// - Metadata: Track when/how index was created
// - Atomic writes: Use AtomicWriter to prevent corruption
package storage

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/makokhawanjala/searchlight/internal/index"
	"github.com/makokhawanjala/searchlight/internal/indexer"
)

// JSONStorage implements the Storage interface using JSON files with gzip compression.
//
// File Format:
// {
//   "version": "1.0",
//   "created_at": "2025-01-15T10:30:00Z",
//   "file_count": 47531,
//   "files": [
//     {"path": "/home/user/file.txt", "name": "file.txt", ...},
//     ...
//   ]
// }
//
// Why wrap files in a container?
// - Versioning: Can detect incompatible formats
// - Metadata: Track when index was created
// - Extensibility: Can add fields without breaking old code
// - Validation: Can verify file_count matches array length
type JSONStorage struct {
	// path is the file path where the index is stored
	// Example: "/home/user/.searchlight/index.json.gz"
	path string

	// useCompression determines whether to use gzip compression
	// Default: true (saves ~80% space)
	// Set to false for debugging (human-readable JSON)
	useCompression bool

	// stats tracks storage operation metrics
	stats *StorageStats
}

// indexContainer is the top-level structure saved to disk.
//
// Why a separate type?
// - Clean separation: Storage format vs in-memory format
// - Versioning: Can change internal format without breaking storage
// - Metadata: Include creation time, version, etc.
// - Validation: Verify integrity on load
type indexContainer struct {
	// Version is the storage format version
	// Used to detect incompatible formats and trigger migrations
	Version string `json:"version"`

	// CreatedAt is when this index snapshot was created
	// Useful for debugging: "This index is 3 days old"
	CreatedAt time.Time `json:"created_at"`

	// FileCount is the number of files in the index
	// Used for validation: ensure array length matches
	FileCount int `json:"file_count"`

	// Files is the array of all indexed files
	// This is the actual data we care about
	Files []*indexer.FileInfo `json:"files"`

	// TotalSize is the sum of all file sizes
	// Useful for quick stats without iterating files
	TotalSize int64 `json:"total_size"`

	// Compressed indicates whether this file is gzip compressed
	// Note: We can also detect this by checking file extension
	Compressed bool `json:"compressed"`
}

// NewJSONStorage creates a new JSON-based storage backend.
//
// Why separate constructor parameters?
// - path: Required (where to store the file)
// - useCompression: Optional with sensible default (true)
//
// Compression Trade-offs:
// - Compressed (default):
//   ✅ 80% smaller files (10 MB → 2 MB)
//   ✅ Faster disk I/O (less data to write)
//   ❌ Slightly slower encode/decode (~10-20ms)
//   ✅ Net win: Faster overall due to I/O savings
//
// - Uncompressed:
//   ✅ Human-readable for debugging
//   ✅ Slightly faster encode/decode
//   ❌ Much larger files
//   ❌ Slower disk I/O
//
// Parameters:
//   - path: File path for storing the index
//   - useCompression: Whether to use gzip compression
//
// Returns:
//   - *JSONStorage: Ready to use storage backend
//
// Example:
//   storage := NewJSONStorage("/root/.searchlight/index.json", true)
func NewJSONStorage(path string, useCompression bool) *JSONStorage {
	return &JSONStorage{
		path:           path,
		useCompression: useCompression,
		stats:          NewStorageStats(),
	}
}

// Save persists the FileIndex to disk in JSON format.
//
// Process:
// 1. Extract all files from FileIndex
// 2. Create indexContainer with metadata
// 3. Write to temporary file with atomic writer
// 4. Apply gzip compression if enabled
// 5. Encode to JSON
// 6. Atomically commit the write
//
// Error Scenarios:
// - Can't create directory: Permission error
// - Can't write file: Disk full
// - JSON encoding fails: Memory corruption (rare)
// - Compression fails: Memory corruption (very rare)
//
// All errors are handled gracefully - the index remains untouched.
//
// Parameters:
//   - idx: The FileIndex to save
//
// Returns:
//   - error: nil on success, descriptive error on failure
func (js *JSONStorage) Save(idx *index.FileIndex) error {
	startTime := time.Now()

	// Step 1: Extract all files from the index
	// GetAll() handles locking internally, returns sorted slice
	files := idx.GetAll()

	// Step 2: Get index statistics for metadata
	stats := idx.Stats()

	// Step 3: Create container with metadata
	container := indexContainer{
		Version:    "1.0",                // Current format version
		CreatedAt:  time.Now(),           // Timestamp for this snapshot
		FileCount:  len(files),           // Number of files
		Files:      files,                // The actual file data
		TotalSize:  stats.TotalSize,      // Sum of all file sizes
		Compressed: js.useCompression,    // Whether this file is compressed
	}

	// Step 4: Create atomic writer for safe writing
	// This ensures we never corrupt the index file
	writer, err := NewAtomicWriter(js.path)
	if err != nil {
		js.stats.SaveErrors++
		return fmt.Errorf("failed to create atomic writer: %w", err)
	}
	defer writer.Abort() // Safety net: cleanup on failure

	// Step 5: Setup compression and encoding pipeline
	//
	// Without compression:
	//   JSON Encoder → AtomicWriter → Disk
	//
	// With compression:
	//   JSON Encoder → Gzip Writer → AtomicWriter → Disk
	//
	// This uses io.Writer chaining for efficiency
	var encoder *json.Encoder

	if js.useCompression {
		// Create gzip writer wrapping the atomic writer
		// Level 6 is good balance: ~75% compression, reasonable speed
		// gzip.BestCompression (9) saves ~5% more but is 2x slower
		gzipWriter := gzip.NewWriter(writer)
		defer gzipWriter.Close()

		// Create JSON encoder writing to gzip writer
		encoder = json.NewEncoder(gzipWriter)

		// Encode the container
		if err := encoder.Encode(&container); err != nil {
			js.stats.SaveErrors++
			return fmt.Errorf("failed to encode JSON: %w", err)
		}

		// CRITICAL: Close gzip writer to flush all compressed data
		// If we don't close, data might be stuck in gzip's buffer
		if err := gzipWriter.Close(); err != nil {
			js.stats.SaveErrors++
			return fmt.Errorf("failed to close gzip writer: %w", err)
		}
	} else {
		// No compression: encode directly to atomic writer
		encoder = json.NewEncoder(writer)

		if err := encoder.Encode(&container); err != nil {
			js.stats.SaveErrors++
			return fmt.Errorf("failed to encode JSON: %w", err)
		}
	}

	// Step 6: Atomically commit the write
	// This renames temp file → target file (atomic operation)
	if err := writer.Commit(); err != nil {
		js.stats.SaveErrors++
		return fmt.Errorf("failed to commit write: %w", err)
	}

	// Step 7: Update statistics
	elapsed := time.Since(startTime)
	js.stats.SaveCount++
	js.stats.LastSaveTime = time.Now().Unix()
	
	// Update rolling average save time
	// Formula: new_avg = (old_avg * count + new_time) / (count + 1)
	// Simplified: track sum and count, compute average on demand
	js.stats.AverageSaveTime = (js.stats.AverageSaveTime*int64(js.stats.SaveCount-1) + 
		elapsed.Milliseconds()) / int64(js.stats.SaveCount)

	// Update storage size
	if size, err := GetFileSize(js.path); err == nil {
		js.stats.StorageSize = size
	}

	return nil
}

// Load reads and deserializes the FileIndex from disk.
//
// Process:
// 1. Check if file exists
// 2. Open file for reading
// 3. Detect and handle gzip compression
// 4. Decode JSON into indexContainer
// 5. Validate container (version, file count)
// 6. Reconstruct FileIndex from files array
//
// Error Handling:
// - File doesn't exist: Return error (caller creates fresh index)
// - Corrupted file: Return error with details
// - Wrong version: Return error (could trigger migration in future)
// - Validation fails: Return error
//
// Performance:
// - Loading 100k files takes ~500ms (including decompression)
// - Most time spent in JSON decoding (~300ms)
// - Gzip decompression is fast (~100ms)
// - Index reconstruction is fast (~100ms)
//
// Returns:
//   - *index.FileIndex: Loaded index ready for use
//   - error: nil on success, error if load fails
func (js *JSONStorage) Load() (*index.FileIndex, error) {
	startTime := time.Now()

	// Step 1: Check if file exists
	if !FileExists(js.path) {
		js.stats.LoadErrors++
		return nil, fmt.Errorf("index file does not exist: %s", js.path)
	}

	// Step 2: Open file for reading
	file, err := os.Open(js.path)
	if err != nil {
		js.stats.LoadErrors++
		return nil, fmt.Errorf("failed to open index file: %w", err)
	}
	defer file.Close()

	// Step 3: Setup decompression and decoding pipeline
	//
	// Detect compression by checking file extension or trying to decompress
	// We try gzip first, fall back to plain JSON if that fails
	var decoder *json.Decoder
	var container indexContainer

	// Try to create gzip reader
	// If file is compressed, this works
	// If not compressed, this fails, and we fall back to plain JSON
	gzipReader, gzipErr := gzip.NewReader(file)
	if gzipErr == nil {
		// File is gzip compressed
		defer gzipReader.Close()
		decoder = json.NewDecoder(gzipReader)
	} else {
		// File is not compressed, or gzip detection failed
		// Reset file pointer and try plain JSON
		file.Seek(0, 0) // Rewind to start
		decoder = json.NewDecoder(file)
	}

	// Step 4: Decode JSON into container
	if err := decoder.Decode(&container); err != nil {
		js.stats.LoadErrors++
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	// Step 5: Validate container
	//
	// Check version compatibility
	if container.Version != "1.0" {
		js.stats.LoadErrors++
		return nil, fmt.Errorf("unsupported index version: %s (expected 1.0)", container.Version)
	}

	// Check file count matches array length
	if container.FileCount != len(container.Files) {
		js.stats.LoadErrors++
		return nil, fmt.Errorf("file count mismatch: metadata says %d, but found %d files",
			container.FileCount, len(container.Files))
	}

	// Step 6: Reconstruct FileIndex from files array
	fileIndex := index.NewFileIndex()

	// Add each file to the index
	// This populates both the Trie and the metadata map
	for _, fileInfo := range container.Files {
		fileIndex.Add(fileInfo)
	}

	// Step 7: Update statistics
	elapsed := time.Since(startTime)
	js.stats.LoadCount++
	js.stats.LastLoadTime = time.Now().Unix()
	js.stats.AverageLoadTime = (js.stats.AverageLoadTime*int64(js.stats.LoadCount-1) + 
		elapsed.Milliseconds()) / int64(js.stats.LoadCount)

	return fileIndex, nil
}

// Clear removes the persisted index file from disk.
//
// Use Cases:
// - User wants fresh start
// - Switching to different directories
// - Clearing corrupted data
//
// Behavior:
// - Deletes the index file if it exists
// - Succeeds silently if file doesn't exist (idempotent)
// - Does NOT affect in-memory index
//
// Returns:
//   - error: nil on success, error if deletion fails
func (js *JSONStorage) Clear() error {
	// Check if file exists
	if !FileExists(js.path) {
		// File doesn't exist - nothing to clear
		// This is not an error (idempotent operation)
		return nil
	}

	// Remove the file
	if err := os.Remove(js.path); err != nil {
		return fmt.Errorf("failed to remove index file: %w", err)
	}

	return nil
}

// Path returns the file path where the index is stored.
//
// Returns:
//   - string: Full path to the index file
func (js *JSONStorage) Path() string {
	return js.path
}

// Stats returns storage operation statistics.
//
// Use Cases:
// - Monitoring: "Last saved 5 minutes ago"
// - Diagnostics: "Average save time is 200ms"
// - Alerting: "Save failure rate is 10%"
//
// Returns:
//   - StorageStats: Current statistics snapshot
func (js *JSONStorage) Stats() StorageStats {
	// Return a copy to prevent external modification
	return *js.stats
}

// SetCompression enables or disables gzip compression.
//
// When to change:
// - Debugging: Disable to inspect JSON manually
// - Performance tuning: Test compressed vs uncompressed
// - Space constraints: Enable to reduce disk usage
//
// Note: Changing this affects future Save() calls only.
// Existing saved files are not affected.
//
// Parameters:
//   - enabled: Whether to use compression
func (js *JSONStorage) SetCompression(enabled bool) {
	js.useCompression = enabled
}

// Validate checks if the stored index file is valid.
//
// What it checks:
// - File exists
// - File is readable
// - JSON is valid
// - Version is compatible
// - File count matches array length
//
// Use Cases:
// - Health checks: "Is the index file corrupted?"
// - Startup validation: "Can we load the index?"
// - Diagnostics: "Why is load failing?"
//
// Returns:
//   - error: nil if valid, descriptive error if invalid
func (js *JSONStorage) Validate() error {
	// Check if file exists
	if !FileExists(js.path) {
		return fmt.Errorf("index file does not exist: %s", js.path)
	}

	// Try to open file
	file, err := os.Open(js.path)
	if err != nil {
		return fmt.Errorf("cannot open index file: %w", err)
	}
	defer file.Close()

	// Try to decode (without loading full index)
	// This validates JSON structure and version
	var container indexContainer

	// Try gzip first
	gzipReader, gzipErr := gzip.NewReader(file)
	var decoder *json.Decoder
	if gzipErr == nil {
		defer gzipReader.Close()
		decoder = json.NewDecoder(gzipReader)
	} else {
		file.Seek(0, 0)
		decoder = json.NewDecoder(file)
	}

	// Decode (validates JSON structure)
	if err := decoder.Decode(&container); err != nil {
		return fmt.Errorf("invalid JSON format: %w", err)
	}

	// Validate version
	if container.Version != "1.0" {
		return fmt.Errorf("unsupported version: %s", container.Version)
	}

	// Validate file count
	if container.FileCount != len(container.Files) {
		return fmt.Errorf("file count mismatch: expected %d, got %d",
			container.FileCount, len(container.Files))
	}

	return nil
}

// Backup creates a backup copy of the index file.
//
// Backup filename: <original>.backup.<timestamp>
// Example: index.json.gz → index.json.gz.backup.20250115_103000
//
// Use Cases:
// - Before major operations (re-indexing, migrations)
// - Periodic backups for disaster recovery
// - Before testing potentially destructive changes
//
// Returns:
//   - string: Path to the backup file
//   - error: nil on success, error if backup fails
func (js *JSONStorage) Backup() (string, error) {
	// Check if source file exists
	if !FileExists(js.path) {
		return "", fmt.Errorf("cannot backup: index file does not exist")
	}

	// Create backup filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	backupPath := fmt.Sprintf("%s.backup.%s", js.path, timestamp)

	// Copy file atomically
	if err := CopyFileAtomic(js.path, backupPath, 0644); err != nil {
		return "", fmt.Errorf("failed to create backup: %w", err)
	}

	return backupPath, nil
}