package indexer

import (
	"context"
	"fmt"
	"sync"
)

// Indexer manages the file index
// This is the main component that stores information about all indexed files
// and provides methods to add, update, remove, and query files
type Indexer struct {
	files       map[string]*FileInfo // path -> FileInfo - stores all file metadata
	mutex       sync.RWMutex         // protects files map from concurrent access
	workerCount int                  // number of concurrent workers for indexing
}

// NewIndexer creates a new Indexer with default settings
// Default worker count is 5, which works well for most systems
func NewIndexer() *Indexer {
	return &Indexer{
		files:       make(map[string]*FileInfo),
		workerCount: 5, // default worker count
	}
}

// NewIndexerWithWorkers creates a new Indexer with specified worker count
// Use this when you want to control concurrency (e.g., fewer workers for slower disks)
func NewIndexerWithWorkers(workerCount int) *Indexer {
	return &Indexer{
		files:       make(map[string]*FileInfo),
		workerCount: workerCount,
	}
}

// Add adds a file to the index
// If the file already exists, it will be replaced with the new information
// This method is thread-safe and can be called from multiple goroutines
func (idx *Indexer) Add(file *FileInfo) {
	// Lock for writing because we're modifying the map
	idx.mutex.Lock()
	defer idx.mutex.Unlock()
	
	// Store the file info - this will overwrite if path already exists
	idx.files[file.Path] = file
}

// Update updates an existing file in the index
// This is useful when a file's metadata changes (e.g., size, modification time)
// Returns true if the file was found and updated, false if it didn't exist
// Thread-safe: can be called concurrently with other operations
func (idx *Indexer) Update(file *FileInfo) bool {
	// Lock for writing because we're modifying the map
	idx.mutex.Lock()
	defer idx.mutex.Unlock()
	
	// Check if the file exists in the index
	if _, exists := idx.files[file.Path]; exists {
		// File exists, update it with new information
		idx.files[file.Path] = file
		return true
	}
	
	// File doesn't exist, can't update
	return false
}

// Remove removes a file from the index
// Returns true if the file was found and removed, false if it didn't exist
// This is called by the file watcher when a file is deleted
// Thread-safe: uses write lock to protect the map
func (idx *Indexer) Remove(path string) bool {
	// Lock for writing because we're modifying the map
	idx.mutex.Lock()
	defer idx.mutex.Unlock()

	// Check if the file exists before trying to delete it
	if _, exists := idx.files[path]; exists {
		// File exists, delete it from the map
		delete(idx.files, path)
		return true
	}
	
	// File doesn't exist, nothing to remove
	return false
}

// Get retrieves a file from the index
// Returns the FileInfo and true if found, nil and false if not found
// This is a read-only operation, so it uses RLock (allows multiple concurrent reads)
func (idx *Indexer) Get(path string) (*FileInfo, bool) {
	// Use read lock because we're only reading, not modifying
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()

	// Look up the file in the map
	file, exists := idx.files[path]
	return file, exists
}

// GetAll returns all files in the index
// Returns a slice containing all FileInfo pointers
// Creates a new slice to avoid exposing the internal map
// Thread-safe: uses read lock
func (idx *Indexer) GetAll() []*FileInfo {
	// Use read lock because we're only reading
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()

	// Pre-allocate slice with exact capacity for efficiency
	files := make([]*FileInfo, 0, len(idx.files))
	
	// Copy all files to the slice
	for _, file := range idx.files {
		files = append(files, file)
	}
	
	return files
}

// Count returns the number of files in the index
// This is useful for statistics and progress reporting
// Thread-safe: uses read lock
func (idx *Indexer) Count() int {
	// Use read lock because we're only reading
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	
	return len(idx.files)
}

// Clear removes all files from the index
// This is useful when you want to rebuild the entire index from scratch
// Thread-safe: uses write lock
func (idx *Indexer) Clear() {
	// Lock for writing because we're modifying the map
	idx.mutex.Lock()
	defer idx.mutex.Unlock()
	
	// Create a new empty map - Go's garbage collector will clean up the old one
	idx.files = make(map[string]*FileInfo)
}

// IndexDirectory scans a directory and adds all files to the index (sequential)
// This is the simple, single-threaded version
// Returns the number of files added and any error encountered
// Skips common directories like .git and node_modules
func (idx *Indexer) IndexDirectory(rootPath string) (int, error) {
	// Create a new scanner to walk the directory tree
	scanner := NewScanner()

	filesAdded := 0
	
	// Scan the directory and call our callback for each file found
	err := scanner.ScanWithCallback(rootPath, func(file *FileInfo) error {
		// Add each file to the index
		idx.Add(file)
		filesAdded++
		return nil
	})

	if err != nil {
		return filesAdded, fmt.Errorf("failed to index directory: %w", err)
	}

	return filesAdded, nil
}

// IndexDirectoryConcurrent scans a directory concurrently using worker pool
// This is much faster than IndexDirectory for large directory trees
// Uses the configured number of workers to process files in parallel
// Returns the number of files added and any error encountered
func (idx *Indexer) IndexDirectoryConcurrent(ctx context.Context, rootPath string) (int, error) {
	// Create a worker pool with the configured number of workers
	wp := NewWorkerPool(idx.workerCount)

	filesAdded := 0
	var mutex sync.Mutex // protects the filesAdded counter

	// This callback is called for each file found
	// It runs in parallel across multiple workers
	callback := func(file *FileInfo) error {
		// Add the file to the index (thread-safe)
		idx.Add(file)
		
		// Increment the counter (need mutex because multiple workers access this)
		mutex.Lock()
		filesAdded++
		mutex.Unlock()
		
		return nil
	}

	// Start the worker pool to index the directory
	err := wp.IndexDirectory(ctx, rootPath, callback)
	if err != nil {
		return filesAdded, fmt.Errorf("failed to index directory concurrently: %w", err)
	}

	return filesAdded, nil
}

// IndexDirectoryWithProgress scans a directory with progress reporting
// This is like IndexDirectoryConcurrent but also calls a progress callback
// The progress callback receives (processed, total) after each file
// Useful for showing progress bars or status updates in the UI
func (idx *Indexer) IndexDirectoryWithProgress(ctx context.Context, rootPath string, progressCallback func(processed, total int64)) (int, error) {
	// Create a worker pool with the configured number of workers
	wp := NewWorkerPool(idx.workerCount)

	// First, count total files (quick scan without reading file contents)
	scanner := NewScanner()
	totalCount, err := scanner.CountFiles(rootPath)
	if err != nil {
		return 0, fmt.Errorf("failed to count files: %w", err)
	}

	// Create a progress reporter that will call our callback
	pr := NewProgressReporter(progressCallback)
	pr.SetTotal(int64(totalCount))

	filesAdded := 0
	var mutex sync.Mutex // protects the filesAdded counter

	// This callback is called for each file found
	callback := func(file *FileInfo) error {
		// Add the file to the index (thread-safe)
		idx.Add(file)
		
		// Increment the counter (need mutex because multiple workers access this)
		mutex.Lock()
		filesAdded++
		mutex.Unlock()
		
		// Report progress
		pr.Update()
		
		return nil
	}

	// Start the worker pool to index the directory
	err = wp.IndexDirectory(ctx, rootPath, callback)
	if err != nil {
		return filesAdded, fmt.Errorf("failed to index directory with progress: %w", err)
	}

	return filesAdded, nil
}

// SetWorkerCount sets the number of workers for concurrent indexing
// Clamps the value between 1 and 100 to prevent misconfiguration
// Changes take effect on the next call to IndexDirectoryConcurrent
func (idx *Indexer) SetWorkerCount(count int) {
	// Ensure at least 1 worker
	if count < 1 {
		count = 1
	}
	
	// Cap at 100 workers (more is usually wasteful)
	if count > 100 {
		count = 100
	}
	
	idx.workerCount = count
}

// GetFilesByExtension returns all files with a specific extension
// Example: GetFilesByExtension(".txt") returns all text files
// The extension can be with or without the leading dot
// Thread-safe: uses read lock
func (idx *Indexer) GetFilesByExtension(ext string) []*FileInfo {
	// Use read lock because we're only reading
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()

	var result []*FileInfo
	
	// Iterate through all files and check extension
	for _, file := range idx.files {
		// Skip directories (they don't have extensions)
		if !file.IsDir && file.MatchesExtension(ext) {
			result = append(result, file)
		}
	}
	
	return result
}

// GetStats returns statistics about the index
// Includes counts of files and directories, plus total size
// Thread-safe: uses read lock
func (idx *Indexer) GetStats() IndexStats {
	// Use read lock because we're only reading
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()

	stats := IndexStats{}
	var totalSize int64

	// Iterate through all files and gather statistics
	for _, file := range idx.files {
		if file.IsDir {
			stats.DirectoryCount++
		} else {
			stats.FileCount++
			totalSize += file.Size
		}
	}

	stats.TotalSize = totalSize
	stats.TotalCount = len(idx.files)

	return stats
}

// IndexStats holds statistics about the index
// Used to display information about the indexed files
type IndexStats struct {
	FileCount      int   `json:"file_count"`      // number of regular files
	DirectoryCount int   `json:"directory_count"` // number of directories
	TotalCount     int   `json:"total_count"`     // total items (files + dirs)
	TotalSize      int64 `json:"total_size"`      // total size in bytes
}

// HumanTotalSize returns the total size in human-readable format
// Examples: "1.5 KB", "23.4 MB", "1.2 GB"
// Makes it easy to display sizes to users
func (s *IndexStats) HumanTotalSize() string {
	const unit = 1024
	size := float64(s.TotalSize)

	// If less than 1 KB, show bytes
	if size < unit {
		return fmt.Sprintf("%d B", s.TotalSize)
	}

	// Calculate the appropriate unit (KB, MB, GB, etc.)
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	// Format with one decimal place and the unit letter
	// "KMGTPE" = Kilobyte, Megabyte, Gigabyte, Terabyte, Petabyte, Exabyte
	return fmt.Sprintf("%.1f %cB", size/float64(div), "KMGTPE"[exp])
}