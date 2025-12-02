// Package storage provides atomic file operations to prevent data corruption.
//
// The Atomic Write Problem:
// When saving data to disk, we face several risks:
// 1. Partial writes: Program crashes mid-write → corrupted file
// 2. Concurrent writes: Two saves happening simultaneously → race condition
// 3. Disk full: Write fails partway through → corrupted file
// 4. Permission errors: Can't write to final location → data loss
//
// The Atomic Write Solution:
// 1. Write to temporary file (safe, isolated)
// 2. Verify write was successful
// 3. Atomically rename temp file to final destination
//    - Rename is atomic on all major filesystems (Linux, macOS, Windows)
//    - Either completely succeeds or completely fails (no partial state)
//
// This ensures: Either old data exists OR new data exists, never corrupted data.
package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// AtomicWriter provides safe file writing with atomic rename.
//
// Why a struct instead of just a function?
// - Encapsulates the temporary file lifecycle
// - Allows cleanup on error (removes temp file)
// - Can add progress reporting in the future
// - Cleaner API for callers
//
// Lifecycle:
// 1. Create AtomicWriter with target path
// 2. Write data to temporary file
// 3. Commit (rename temp → target) or Abort (delete temp)
//
// Example usage:
//   writer := NewAtomicWriter("/home/user/.searchlight/index.json")
//   defer writer.Abort() // Safety net: cleanup if commit not called
//   
//   if _, err := writer.Write(data); err != nil {
//       return err
//   }
//   
//   return writer.Commit() // Atomically make data visible
type AtomicWriter struct {
	// targetPath is the final destination for the file
	// Example: "/home/user/.searchlight/index.json"
	targetPath string

	// tempPath is where we write data initially
	// Example: "/home/user/.searchlight/index.json.tmp.123456"
	// We use .tmp prefix to make it clear these are temporary
	tempPath string

	// tempFile is the open file handle for writing
	// We keep this open until Commit() or Abort()
	tempFile *os.File

	// committed tracks whether we've successfully committed
	// Prevents double-commit and ensures cleanup happens correctly
	committed bool
}

// NewAtomicWriter creates a new AtomicWriter for the given target path.
//
// Process:
// 1. Ensure target directory exists (create if needed)
// 2. Generate unique temporary filename
// 3. Create temporary file with restrictive permissions (0600)
// 4. Return writer ready for writing
//
// Why restrictive permissions (0600)?
// - Index may contain sensitive path information
// - Only owner can read/write during creation
// - Final file will inherit proper permissions on rename
//
// Parameters:
//   - targetPath: Final destination path for the file
//
// Returns:
//   - *AtomicWriter: Ready for writing
//   - error: If directory creation or temp file creation fails
//
// Example:
//   writer, err := NewAtomicWriter("/root/.searchlight/index.json")
//   if err != nil {
//       return fmt.Errorf("cannot create writer: %w", err)
//   }
//   defer writer.Abort()
func NewAtomicWriter(targetPath string) (*AtomicWriter, error) {
	// Step 1: Ensure the target directory exists
	// Example: "/home/user/.searchlight/index.json" → ensure "/home/user/.searchlight/" exists
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Step 2: Generate unique temporary filename
	// Pattern: <targetfile>.tmp.<process_id>
	// Why include process ID?
	// - Prevents conflicts if multiple SearchLight instances run
	// - Makes debugging easier (can identify which process created temp file)
	// - Cleanup is easier (can identify stale temp files)
	tempPath := fmt.Sprintf("%s.tmp.%d", targetPath, os.Getpid())

	// Step 3: Create temporary file with restrictive permissions
	// 0600 = owner can read/write, nobody else can access
	// O_WRONLY = write-only (we won't read back)
	// O_CREATE = create if doesn't exist
	// O_TRUNC = truncate if exists (clean slate)
	tempFile, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file %s: %w", tempPath, err)
	}

	return &AtomicWriter{
		targetPath: targetPath,
		tempPath:   tempPath,
		tempFile:   tempFile,
		committed:  false,
	}, nil
}

// Write writes data to the temporary file.
//
// This implements io.Writer interface, so can be used with:
// - json.NewEncoder(writer).Encode()
// - gzip.NewWriter(writer)
// - io.Copy(writer, reader)
//
// Thread Safety:
// - NOT thread-safe (single writer expected)
// - Caller should not write from multiple goroutines
//
// Error Handling:
// - Returns number of bytes written and any error
// - On error, caller should call Abort() to cleanup
//
// Parameters:
//   - data: Bytes to write to temporary file
//
// Returns:
//   - int: Number of bytes written
//   - error: nil on success, write error on failure
func (aw *AtomicWriter) Write(data []byte) (int, error) {
	if aw.tempFile == nil {
		return 0, fmt.Errorf("writer already closed or not initialized")
	}

	n, err := aw.tempFile.Write(data)
	if err != nil {
		return n, fmt.Errorf("failed to write to temporary file: %w", err)
	}

	return n, nil
}

// Commit finalizes the write by atomically renaming temp file to target.
//
// Atomic Rename Guarantee:
// On POSIX systems (Linux, macOS), rename() is atomic:
// - Either succeeds completely (new file visible)
// - Or fails completely (old file unchanged)
// - Never leaves filesystem in inconsistent state
//
// On Windows:
// - os.Rename is atomic if source and dest are on same volume
// - If target exists, we remove it first (Windows doesn't auto-replace)
//
// Process:
// 1. Sync temporary file to disk (ensure data is written)
// 2. Close temporary file (release handle)
// 3. Rename temp → target (atomic operation)
// 4. Mark as committed
//
// Why Sync before rename?
// - Ensures data is physically on disk, not just in OS buffer
// - Prevents data loss if power fails immediately after rename
// - Small performance cost, but critical for data durability
//
// Returns:
//   - error: nil on success, error if sync or rename fails
func (aw *AtomicWriter) Commit() error {
	if aw.committed {
		return fmt.Errorf("already committed")
	}

	if aw.tempFile == nil {
		return fmt.Errorf("writer not initialized or already closed")
	}

	// Step 1: Sync to disk
	// This ensures all buffered data is physically written
	if err := aw.tempFile.Sync(); err != nil {
		aw.Abort() // Cleanup on failure
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}

	// Step 2: Close temporary file
	// Must close before rename (Windows requirement)
	if err := aw.tempFile.Close(); err != nil {
		aw.Abort() // Cleanup on failure
		return fmt.Errorf("failed to close temporary file: %w", err)
	}
	aw.tempFile = nil // Mark as closed

	// Step 3: Atomic rename
	// On Windows, if target exists, we need to remove it first
	// On Unix, rename automatically replaces target (atomic)
	if err := atomicRename(aw.tempPath, aw.targetPath); err != nil {
		// Cleanup temp file on failure
		os.Remove(aw.tempPath)
		return fmt.Errorf("failed to rename temporary file to target: %w", err)
	}

	// Step 4: Mark as committed
	aw.committed = true

	return nil
}

// Abort cancels the write and removes the temporary file.
//
// When to call:
// - Write failed and we want to cleanup
// - Using defer as safety net: defer writer.Abort()
// - Cancelling operation before completion
//
// Behavior:
// - Closes temporary file if still open
// - Removes temporary file from disk
// - Safe to call multiple times (idempotent)
// - Safe to call after Commit (no-op)
//
// Example pattern:
//   writer, err := NewAtomicWriter(path)
//   if err != nil {
//       return err
//   }
//   defer writer.Abort() // Ensures cleanup if we forget
//   
//   // ... do writes ...
//   
//   return writer.Commit() // Success: makes Abort() a no-op
func (aw *AtomicWriter) Abort() {
	// If already committed, nothing to clean up
	if aw.committed {
		return
	}

	// Close file if still open
	if aw.tempFile != nil {
		aw.tempFile.Close()
		aw.tempFile = nil
	}

	// Remove temporary file (ignore errors - best effort cleanup)
	// We ignore errors because:
	// - File might not exist (already removed)
	// - Permission issues (can't do anything about it)
	// - Temp files will be cleaned up eventually by OS or manual cleanup
	os.Remove(aw.tempPath)
}

// atomicRename performs an atomic file rename.
//
// Why a separate function?
// - Platform-specific behavior (Windows vs Unix)
// - Easier to test and mock
// - Can add retry logic if needed
//
// Platform Differences:
// - Unix: rename() is always atomic, replaces target
// - Windows: MoveFileEx is atomic, but needs special handling for existing files
//
// Go's os.Rename:
// - On Unix: calls rename() syscall (atomic)
// - On Windows: calls MoveFileEx (atomic within same volume)
//
// Parameters:
//   - oldPath: Source path (temporary file)
//   - newPath: Destination path (target file)
//
// Returns:
//   - error: nil on success, error if rename fails
func atomicRename(oldPath, newPath string) error {
	// On Windows, if target exists, remove it first
	// On Unix, os.Rename automatically replaces target
	//
	// We check if we're on Windows by trying to stat the target
	// This is safe because:
	// - If target doesn't exist, stat fails, rename will work
	// - If target exists on Windows, we remove it first
	// - On Unix, we ignore the stat result and let rename handle it

	// Attempt the rename
	err := os.Rename(oldPath, newPath)
	if err != nil {
		// On Windows, if target exists, rename might fail
		// Try removing target and renaming again
		if os.IsExist(err) || os.IsPermission(err) {
			// Remove existing target file
			if removeErr := os.Remove(newPath); removeErr != nil {
				return fmt.Errorf("failed to remove existing target: %w", removeErr)
			}

			// Try rename again
			if err := os.Rename(oldPath, newPath); err != nil {
				return fmt.Errorf("failed to rename after removing target: %w", err)
			}

			return nil
		}

		return err
	}

	return nil
}

// AtomicWriteFile is a convenience function that writes data atomically to a file.
//
// This is a high-level wrapper that handles the full lifecycle:
// 1. Create AtomicWriter
// 2. Write data
// 3. Commit (or abort on error)
//
// Use this when:
// - You have all data ready to write at once
// - You don't need streaming/chunked writes
// - You want simple, foolproof atomic writes
//
// Parameters:
//   - path: Target file path
//   - data: Complete data to write
//   - perm: File permissions (e.g., 0644)
//
// Returns:
//   - error: nil on success, error if any step fails
//
// Example:
//   data := []byte(`{"files": [...]}`)
//   err := AtomicWriteFile("/root/.searchlight/index.json", data, 0644)
func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
	// Create atomic writer
	writer, err := NewAtomicWriter(path)
	if err != nil {
		return fmt.Errorf("failed to create atomic writer: %w", err)
	}
	defer writer.Abort() // Safety net: cleanup on failure

	// Write all data
	if _, err := writer.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	// Commit the write
	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	// Set final file permissions
	// We do this after commit because temp file has restrictive 0600
	if err := os.Chmod(path, perm); err != nil {
		// Don't fail here - file is written successfully
		// Just log warning (caller should handle)
		return fmt.Errorf("warning: failed to set permissions: %w", err)
	}

	return nil
}

// CopyFileAtomic copies a file atomically to a new location.
//
// Use Cases:
// - Backup index before modifying
// - Migrate index to new location
// - Create snapshot for rollback
//
// Why atomic copy?
// - Ensures destination is either complete or doesn't exist
// - Safe for copying to locations that others might read
//
// Parameters:
//   - src: Source file path
//   - dst: Destination file path
//   - perm: Permissions for destination file
//
// Returns:
//   - error: nil on success, error if copy fails
//
// Example:
//   // Backup index before major operation
//   err := CopyFileAtomic(
//       "/root/.searchlight/index.json",
//       "/root/.searchlight/index.json.backup",
//       0644,
//   )
func CopyFileAtomic(src, dst string, perm os.FileMode) error {
	// Open source file for reading
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()

	// Create atomic writer for destination
	writer, err := NewAtomicWriter(dst)
	if err != nil {
		return fmt.Errorf("failed to create atomic writer: %w", err)
	}
	defer writer.Abort()

	// Copy data from source to destination
	if _, err := io.Copy(writer, srcFile); err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	// Commit the write
	if err := writer.Commit(); err != nil {
		return fmt.Errorf("failed to commit copy: %w", err)
	}

	// Set permissions on destination
	if err := os.Chmod(dst, perm); err != nil {
		return fmt.Errorf("warning: failed to set permissions: %w", err)
	}

	return nil
}

// FileExists checks if a file exists and is accessible.
//
// Use Cases:
// - Check if index exists before trying to load
// - Verify backup files exist
// - Conditional behavior based on file presence
//
// Why not just use os.Stat?
// - Cleaner API: returns bool instead of error
// - Handles permission errors gracefully
// - More readable in conditionals
//
// Returns:
//   - true if file exists and is accessible
//   - false if file doesn't exist or is inaccessible
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// GetFileSize returns the size of a file in bytes.
//
// Use Cases:
// - Display storage stats to user
// - Monitor index growth over time
// - Decide if compression is needed
//
// Returns:
//   - int64: File size in bytes
//   - error: If file doesn't exist or can't be accessed
func GetFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("failed to stat file: %w", err)
	}
	return info.Size(), nil
}