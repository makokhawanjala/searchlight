package watcher


import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestDebouncer_SingleEvent tests that a single event triggers callback after delay.
//
// Why this test?
// - Verifies basic debouncer functionality
// - Ensures callback is called exactly once
// - Confirms timing works correctly
func TestDebouncer_SingleEvent(t *testing.T) {
	// Track callback invocations
	var called bool
	var calledPath string
	var calledType EventType
	var mu sync.Mutex

	// Create debouncer with 50ms delay
	debouncer := NewDebouncer(50*time.Millisecond, func(event DebouncedEvent) {
		mu.Lock()
		defer mu.Unlock()
		called = true
		calledPath = event.Path
		calledType = event.Type
	})

	// Add a single event
	testPath := "/test/file.txt"
	debouncer.Add(testPath, EventModify)

	// Verify callback hasn't been called yet (delay hasn't passed)
	mu.Lock()
	if called {
		t.Error("Callback called before delay elapsed")
	}
	mu.Unlock()

	// Wait for delay + buffer
	time.Sleep(100 * time.Millisecond)

	// Verify callback was called
	mu.Lock()
	if !called {
		t.Error("Callback was not called after delay")
	}
	if calledPath != testPath {
		t.Errorf("Path = %s, want %s", calledPath, testPath)
	}
	if calledType != EventModify {
		t.Errorf("Type = %v, want %v", calledType, EventModify)
	}
	mu.Unlock()
}

// TestDebouncer_MultipleEventsDebounced tests that multiple rapid events
// for the same file are consolidated into a single callback.
//
// Why this test?
// - Core debouncing behavior
// - Verifies timer reset works
// - Confirms only one callback for multiple events
func TestDebouncer_MultipleEventsDebounced(t *testing.T) {
	// Track callback invocations
	var callCount int
	var mu sync.Mutex

	debouncer := NewDebouncer(50*time.Millisecond, func(event DebouncedEvent) {
		mu.Lock()
		defer mu.Unlock()
		callCount++
	})

	testPath := "/test/file.txt"

	// Add multiple events rapidly (within debounce window)
	debouncer.Add(testPath, EventModify)
	time.Sleep(10 * time.Millisecond)
	debouncer.Add(testPath, EventModify)
	time.Sleep(10 * time.Millisecond)
	debouncer.Add(testPath, EventModify)

	// Wait for debounce delay + buffer
	time.Sleep(100 * time.Millisecond)

	// Should have been called exactly once
	mu.Lock()
	if callCount != 1 {
		t.Errorf("Callback called %d times, want 1", callCount)
	}
	mu.Unlock()
}

// TestDebouncer_DifferentPaths tests that events for different files
// are handled independently.
//
// Why this test?
// - Verifies per-path debouncing
// - Ensures one file's events don't affect another
// - Tests concurrent event handling
func TestDebouncer_DifferentPaths(t *testing.T) {
	// Track which paths triggered callbacks
	calledPaths := make(map[string]int)
	var mu sync.Mutex

	debouncer := NewDebouncer(50*time.Millisecond, func(event DebouncedEvent) {
		mu.Lock()
		defer mu.Unlock()
		calledPaths[event.Path]++
	})

	// Add events for different files
	debouncer.Add("/test/file1.txt", EventModify)
	debouncer.Add("/test/file2.txt", EventModify)
	debouncer.Add("/test/file3.txt", EventCreate)

	// Wait for debounce delay
	time.Sleep(100 * time.Millisecond)

	// All three paths should have triggered callbacks
	mu.Lock()
	if len(calledPaths) != 3 {
		t.Errorf("Callback called for %d paths, want 3", len(calledPaths))
	}
	if calledPaths["/test/file1.txt"] != 1 {
		t.Errorf("file1.txt called %d times, want 1", calledPaths["/test/file1.txt"])
	}
	if calledPaths["/test/file2.txt"] != 1 {
		t.Errorf("file2.txt called %d times, want 1", calledPaths["/test/file2.txt"])
	}
	if calledPaths["/test/file3.txt"] != 1 {
		t.Errorf("file3.txt called %d times, want 1", calledPaths["/test/file3.txt"])
	}
	mu.Unlock()
}

// TestDebouncer_EventTypes tests that different event types are preserved.
//
// Why this test?
// - Verifies event type is correctly passed to callback
// - Tests all event type variants
func TestDebouncer_EventTypes(t *testing.T) {
	tests := []struct {
		name      string
		eventType EventType
	}{
		{"Create", EventCreate},
		{"Modify", EventModify},
		{"Delete", EventDelete},
		{"Rename", EventRename},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedType EventType
			var mu sync.Mutex

			debouncer := NewDebouncer(50*time.Millisecond, func(event DebouncedEvent) {
				mu.Lock()
				defer mu.Unlock()
				receivedType = event.Type
			})

			debouncer.Add("/test/file.txt", tt.eventType)
			time.Sleep(100 * time.Millisecond)

			mu.Lock()
			if receivedType != tt.eventType {
				t.Errorf("Received type = %v, want %v", receivedType, tt.eventType)
			}
			mu.Unlock()
		})
	}
}

// TestDebouncer_Pending tests the Pending() method.
//
// Why this test?
// - Verifies we can query debouncer state
// - Tests that pending count is accurate
// - Confirms timers are properly tracked and cleaned up
func TestDebouncer_Pending(t *testing.T) {
	debouncer := NewDebouncer(50*time.Millisecond, func(event DebouncedEvent) {
		// No-op callback
	})

	// Initially no pending events
	if pending := debouncer.Pending(); pending != 0 {
		t.Errorf("Initial pending = %d, want 0", pending)
	}

	// Add events
	debouncer.Add("/test/file1.txt", EventModify)
	debouncer.Add("/test/file2.txt", EventModify)
	debouncer.Add("/test/file3.txt", EventModify)

	// Should have 3 pending
	if pending := debouncer.Pending(); pending != 3 {
		t.Errorf("After adding 3 events, pending = %d, want 3", pending)
	}

	// Add another event for file1 (should replace, not add)
	debouncer.Add("/test/file1.txt", EventModify)

	// Still 3 pending (same paths)
	if pending := debouncer.Pending(); pending != 3 {
		t.Errorf("After replacing event, pending = %d, want 3", pending)
	}

	// Wait for all timers to fire
	time.Sleep(100 * time.Millisecond)

	// Should be back to 0
	if pending := debouncer.Pending(); pending != 0 {
		t.Errorf("After timers fired, pending = %d, want 0", pending)
	}
}

// TestDebouncer_Flush tests the Flush() method.
//
// Why this test?
// - Verifies immediate event processing
// - Tests that pending timers are cleared
// - Ensures all events are processed
func TestDebouncer_Flush(t *testing.T) {
	calledPaths := make(map[string]int)
	var mu sync.Mutex

	debouncer := NewDebouncer(1*time.Second, func(event DebouncedEvent) {
		// Use 1 second delay so events won't fire naturally during test
		mu.Lock()
		defer mu.Unlock()
		calledPaths[event.Path]++
	})

	// Add events
	debouncer.Add("/test/file1.txt", EventModify)
	debouncer.Add("/test/file2.txt", EventModify)
	debouncer.Add("/test/file3.txt", EventCreate)

	// Verify pending
	if pending := debouncer.Pending(); pending != 3 {
		t.Errorf("Before flush, pending = %d, want 3", pending)
	}

	// Flush immediately
	debouncer.Flush()

	// All callbacks should have been called
	mu.Lock()
	if len(calledPaths) != 3 {
		t.Errorf("After flush, called %d paths, want 3", len(calledPaths))
	}
	mu.Unlock()

	// No more pending
	if pending := debouncer.Pending(); pending != 0 {
		t.Errorf("After flush, pending = %d, want 0", pending)
	}
}

// TestDebouncer_Stop tests the Stop() method.
//
// Why this test?
// - Verifies timers are canceled without calling callbacks
// - Tests clean shutdown behavior
// - Ensures resources are freed
func TestDebouncer_Stop(t *testing.T) {
	var callCount int
	var mu sync.Mutex

	debouncer := NewDebouncer(50*time.Millisecond, func(event DebouncedEvent) {
		mu.Lock()
		defer mu.Unlock()
		callCount++
	})

	// Add events
	debouncer.Add("/test/file1.txt", EventModify)
	debouncer.Add("/test/file2.txt", EventModify)

	// Verify pending
	if pending := debouncer.Pending(); pending != 2 {
		t.Errorf("Before stop, pending = %d, want 2", pending)
	}

	// Stop immediately
	debouncer.Stop()

	// No more pending
	if pending := debouncer.Pending(); pending != 0 {
		t.Errorf("After stop, pending = %d, want 0", pending)
	}

	// Wait to ensure callbacks don't fire
	time.Sleep(100 * time.Millisecond)

	// Callbacks should NOT have been called
	mu.Lock()
	if callCount != 0 {
		t.Errorf("After stop, callbacks called %d times, want 0", callCount)
	}
	mu.Unlock()
}

// TestDebouncer_ConcurrentAdds tests thread safety with concurrent Add() calls.
//
// Why this test?
// - Verifies mutex protection works
// - Tests real-world concurrent usage
// - Ensures no race conditions
//
// Run with: go test -race
func TestDebouncer_ConcurrentAdds(t *testing.T) {
	var callCount int
	var mu sync.Mutex

	debouncer := NewDebouncer(50*time.Millisecond, func(event DebouncedEvent) {
		mu.Lock()
		defer mu.Unlock()
		callCount++
	})

	// Launch multiple goroutines adding events concurrently
	const numGoroutines = 10
	const eventsPerGoroutine = 5

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				// Each goroutine adds events for different files
				path := fmt.Sprintf("/test/file%d.txt", id)
				debouncer.Add(path, EventModify)
				time.Sleep(time.Millisecond) // Small delay between adds
			}
		}(i)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Wait for all debounce delays to pass
	time.Sleep(150 * time.Millisecond)

	// Should have one callback per unique path (10 files)
	mu.Lock()
	if callCount != numGoroutines {
		t.Errorf("Callback called %d times, want %d", callCount, numGoroutines)
	}
	mu.Unlock()
}

// TestEventType_String tests the String() method for EventType.
//
// Why this test?
// - Verifies string representation is correct
// - Tests all event type values
// - Ensures debugging output is readable
func TestEventType_String(t *testing.T) {
	tests := []struct {
		eventType EventType
		want      string
	}{
		{EventCreate, "Create"},
		{EventModify, "Modify"},
		{EventDelete, "Delete"},
		{EventRename, "Rename"},
		{EventType(999), "Unknown"}, // Invalid value
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.eventType.String()
			if got != tt.want {
				t.Errorf("String() = %s, want %s", got, tt.want)
			}
		})
	}
}

// TestDebouncer_RapidFireSamePath tests extreme rapid-fire events.
//
// Why this test?
// - Tests debouncer under stress
// - Verifies timer reset works correctly even with very rapid events
// - Ensures performance with high event rates
func TestDebouncer_RapidFireSamePath(t *testing.T) {
	var callCount int
	var mu sync.Mutex

	debouncer := NewDebouncer(100*time.Millisecond, func(event DebouncedEvent) {
		mu.Lock()
		defer mu.Unlock()
		callCount++
	})

	testPath := "/test/rapid-fire.txt"

	// Add 100 events as fast as possible
	for i := 0; i < 100; i++ {
		debouncer.Add(testPath, EventModify)
	}

	// Wait for debounce delay
	time.Sleep(200 * time.Millisecond)

	// Should still only be called once (all events debounced together)
	mu.Lock()
	if callCount != 1 {
		t.Errorf("Callback called %d times, want 1", callCount)
	}
	mu.Unlock()
}

// TestDebouncer_Timestamp tests that timestamp is captured correctly.
//
// Why this test?
// - Verifies timestamp reflects when event was added
// - Tests that timestamp is preserved through debouncing
func TestDebouncer_Timestamp(t *testing.T) {
	var receivedTime time.Time
	var mu sync.Mutex

	debouncer := NewDebouncer(50*time.Millisecond, func(event DebouncedEvent) {
		mu.Lock()
		defer mu.Unlock()
		receivedTime = event.Timestamp
	})

	beforeAdd := time.Now()
	debouncer.Add("/test/file.txt", EventModify)
	afterAdd := time.Now()

	// Wait for callback
	time.Sleep(100 * time.Millisecond)

	// Timestamp should be between beforeAdd and afterAdd
	mu.Lock()
	if receivedTime.Before(beforeAdd) || receivedTime.After(afterAdd) {
		t.Errorf("Timestamp %v not between %v and %v", receivedTime, beforeAdd, afterAdd)
	}
	mu.Unlock()
}
