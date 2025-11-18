package indexer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// WorkerPool manages concurrent file indexing
type WorkerPool struct {
	workerCount int
	jobs        chan string // paths to process
	results     chan *FileInfo
	errors      chan error
	wg          sync.WaitGroup
	scanner     *Scanner
	processed   atomic.Int64 // count of processed files
}

// NewWorkerPool creates a new worker pool with the specified number of workers
func NewWorkerPool(workerCount int) *WorkerPool {
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > 100 {
		workerCount = 100
	}

	return &WorkerPool{
		workerCount: workerCount,
		jobs:        make(chan string, workerCount*2), // Buffered channel
		results:     make(chan *FileInfo, workerCount*2),
		errors:      make(chan error, workerCount),
		scanner:     NewScanner(),
	}
}

// worker processes paths from the jobs channel
func (wp *WorkerPool) worker(ctx context.Context, id int) {
	defer wp.wg.Done()

	for {
		select {
		case <-ctx.Done():
			// Context cancelled, stop working
			return

		case path, ok := <-wp.jobs:
			if !ok {
				// Jobs channel closed, no more work
				return
			}

			// Process the path
			if err := wp.processPath(path); err != nil {
				// Send error but don't block
				select {
				case wp.errors <- err:
				default:
					// Error channel full, log and continue
					fmt.Fprintf(os.Stderr, "Worker %d: error channel full, dropping error: %v\n", id, err)
				}
			}

			// Increment processed counter
			wp.processed.Add(1)
		}
	}
}

// processPath processes a single path and sends results
func (wp *WorkerPool) processPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", path, err)
	}

	// Create FileInfo
	fileInfo := NewFileInfo(path, info)

	// Send result
	select {
	case wp.results <- fileInfo:
		return nil
	default:
		// Results channel full (shouldn't happen with proper buffer size)
		return fmt.Errorf("results channel full for %s", path)
	}
}

// walkDirectory walks a directory and sends paths to the jobs channel
func (wp *WorkerPool) walkDirectory(ctx context.Context, rootPath string) error {
	return filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Handle walk errors
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to access %s: %v\n", path, err)
			return filepath.SkipDir
		}

		// Skip directories in the skip list
		if info.IsDir() && path != rootPath {
			if wp.scanner.SkipDirs[info.Name()] {
				return filepath.SkipDir
			}
		}

		// Send path to workers
		select {
		case <-ctx.Done():
			return ctx.Err()
		case wp.jobs <- path:
			return nil
		}
	})
}

// IndexDirectory indexes a directory concurrently using the worker pool
func (wp *WorkerPool) IndexDirectory(ctx context.Context, rootPath string, callback func(*FileInfo) error) error {
	// Normalize the path
	rootPath, err := filepath.Abs(rootPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if root path exists
	if _, err := os.Stat(rootPath); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", rootPath)
	}

	// Start workers
	for i := 0; i < wp.workerCount; i++ {
		wp.wg.Add(1)
		go wp.worker(ctx, i)
	}

	// Start result collector
	collectorDone := make(chan struct{})
	var collectorErr error
	go func() {
		defer close(collectorDone)
		for result := range wp.results {
			if err := callback(result); err != nil {
				collectorErr = fmt.Errorf("callback error: %w", err)
				return
			}
		}
	}()

	// Walk directory and send jobs
	walkErr := wp.walkDirectory(ctx, rootPath)

	// Close jobs channel to signal workers to stop
	close(wp.jobs)

	// Wait for all workers to finish
	wp.wg.Wait()

	// Close results channel
	close(wp.results)

	// Wait for collector to finish
	<-collectorDone

	// Check for errors
	if walkErr != nil && walkErr != context.Canceled {
		return fmt.Errorf("directory walk error: %w", walkErr)
	}

	if collectorErr != nil {
		return collectorErr
	}

	// Check for worker errors
	close(wp.errors)
	var workerErrors []error
	for err := range wp.errors {
		workerErrors = append(workerErrors, err)
	}

	if len(workerErrors) > 0 {
		return fmt.Errorf("worker errors occurred: %d errors", len(workerErrors))
	}

	return nil
}

// ProcessedCount returns the number of files processed
func (wp *WorkerPool) ProcessedCount() int64 {
	return wp.processed.Load()
}

// ProgressReporter provides progress updates during indexing
type ProgressReporter struct {
	Total     int64
	Processed atomic.Int64
	callback  func(processed, total int64)
}

// NewProgressReporter creates a new progress reporter
func NewProgressReporter(callback func(processed, total int64)) *ProgressReporter {
	return &ProgressReporter{
		callback: callback,
	}
}

// Update increments the processed counter and calls the callback
func (pr *ProgressReporter) Update() {
	processed := pr.Processed.Add(1)
	if pr.callback != nil {
		pr.callback(processed, pr.Total)
	}
}

// SetTotal sets the total number of items to process
func (pr *ProgressReporter) SetTotal(total int64) {
	pr.Total = total
}
