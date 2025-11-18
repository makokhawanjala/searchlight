package indexer

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewWorkerPool(t *testing.T) {
	tests := []struct {
		name        string
		workerCount int
		expected    int
	}{
		{"valid count", 5, 5},
		{"zero workers", 0, 1},
		{"negative workers", -5, 1},
		{"too many workers", 150, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wp := NewWorkerPool(tt.workerCount)
			if wp.workerCount != tt.expected {
				t.Errorf("expected %d workers, got %d", tt.expected, wp.workerCount)
			}
		})
	}
}

func TestWorkerPool_IndexDirectory(t *testing.T) {
	tmpDir := setupTestDir(t)

	wp := NewWorkerPool(3)
	ctx := context.Background()

	var filesIndexed atomic.Int64
	callback := func(fi *FileInfo) error {
		filesIndexed.Add(1)
		return nil
	}

	err := wp.IndexDirectory(ctx, tmpDir, callback)
	if err != nil {
		t.Fatalf("IndexDirectory failed: %v", err)
	}

	if filesIndexed.Load() == 0 {
		t.Error("expected files to be indexed")
	}

	t.Logf("Indexed %d files with %d workers", filesIndexed.Load(), wp.workerCount)
}

func TestWorkerPool_Cancellation(t *testing.T) {
	tmpDir := setupTestDir(t)

	wp := NewWorkerPool(2)
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	var filesIndexed atomic.Int64
	callback := func(fi *FileInfo) error {
		filesIndexed.Add(1)
		return nil
	}

	err := wp.IndexDirectory(ctx, tmpDir, callback)
	if err == nil {
		t.Error("expected cancellation error")
	}

	t.Logf("Indexed %d files before cancellation", filesIndexed.Load())
}

func TestWorkerPool_CallbackError(t *testing.T) {
	tmpDir := setupTestDir(t)

	wp := NewWorkerPool(2)
	ctx := context.Background()

	// Callback that fails on second file
	var count atomic.Int64
	callback := func(fi *FileInfo) error {
		if count.Add(1) == 2 {
			return fmt.Errorf("simulated callback error")
		}
		return nil
	}

	err := wp.IndexDirectory(ctx, tmpDir, callback)
	if err == nil {
		t.Error("expected callback error")
	}
}

func TestWorkerPool_NonExistentPath(t *testing.T) {
	wp := NewWorkerPool(2)
	ctx := context.Background()

	callback := func(fi *FileInfo) error {
		return nil
	}

	err := wp.IndexDirectory(ctx, "/nonexistent/path", callback)
	if err == nil {
		t.Error("expected error for non-existent path")
	}
}

func TestWorkerPool_ProcessedCount(t *testing.T) {
	tmpDir := setupTestDir(t)

	wp := NewWorkerPool(3)
	ctx := context.Background()

	callback := func(fi *FileInfo) error {
		return nil
	}

	err := wp.IndexDirectory(ctx, tmpDir, callback)
	if err != nil {
		t.Fatalf("IndexDirectory failed: %v", err)
	}

	count := wp.ProcessedCount()
	if count == 0 {
		t.Error("expected processed count > 0")
	}

	t.Logf("Processed count: %d", count)
}

func TestWorkerPool_ConcurrentSafety(t *testing.T) {
	tmpDir := setupTestDir(t)

	// Create multiple worker pools running concurrently
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			wp := NewWorkerPool(3)
			ctx := context.Background()

			var count atomic.Int64
			callback := func(fi *FileInfo) error {
				count.Add(1)
				return nil
			}

			if err := wp.IndexDirectory(ctx, tmpDir, callback); err != nil {
				t.Errorf("Worker pool %d failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait()
}

func TestProgressReporter(t *testing.T) {
	var called atomic.Bool
	var lastProcessed, lastTotal int64

	pr := NewProgressReporter(func(processed, total int64) {
		called.Store(true)
		lastProcessed = processed
		lastTotal = total
	})

	pr.SetTotal(10)
	pr.Update()
	pr.Update()
	pr.Update()

	// Give callback time to execute
	time.Sleep(10 * time.Millisecond)

	if !called.Load() {
		t.Error("expected callback to be called")
	}

	if lastProcessed != 3 {
		t.Errorf("expected processed=3, got %d", lastProcessed)
	}

	if lastTotal != 10 {
		t.Errorf("expected total=10, got %d", lastTotal)
	}
}

// BenchmarkWorkerPool compares sequential vs concurrent indexing
func BenchmarkWorkerPool(b *testing.B) {
	tmpDir := setupLargeBenchDir(b)

	benchmarks := []struct {
		name    string
		workers int
	}{
		{"Sequential", 1},
		{"Workers-2", 2},
		{"Workers-4", 4},
		{"Workers-8", 8},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				wp := NewWorkerPool(bm.workers)
				ctx := context.Background()

				callback := func(fi *FileInfo) error {
					return nil
				}

				if err := wp.IndexDirectory(ctx, tmpDir, callback); err != nil {
					b.Fatalf("IndexDirectory failed: %v", err)
				}
			}
		})
	}
}

