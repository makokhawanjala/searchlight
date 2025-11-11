package indexer

import (
	"fmt"
	"os"
	"path/filepath"
)

// Scanner handles recursive directory scanning
type Scanner struct {
	// SkipDirs is a set of directory names to skip (e.g., .git, node_modules)
	SkipDirs map[string]bool
}

// NewScanner creates a new Scanner with default settings
func NewScanner() *Scanner {
	return &Scanner{
		SkipDirs: map[string]bool{
			".git":         true,
			"node_modules": true,
			".svn":         true,
			".hg":          true,
			"__pycache__":  true,
			".vscode":      true,
			".idea":        true,
		},
	}
}

// Scan recursively scans a directory and returns all files found
func (s *Scanner) Scan(rootPath string) ([]*FileInfo, error) {
	var files []*FileInfo
	var scanErr error

	// Normalize the path
	rootPath, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if root path exists
	if _, err := os.Stat(rootPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("path does not exist: %s", rootPath)
	}

	// Walk the directory tree
	err = filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		// Handle walk errors
		if err != nil {
			// Log error but continue walking
			// In production, this would use proper logging
			fmt.Fprintf(os.Stderr, "Warning: failed to access %s: %v\n", path, err)
			return filepath.SkipDir
		}

		// Skip directories in the skip list
		if info.IsDir() && path != rootPath {
			if s.SkipDirs[info.Name()] {
				return filepath.SkipDir
			}
		}

		// Create FileInfo for this path
		fileInfo := NewFileInfo(path, info)
		files = append(files, fileInfo)

		return nil
	})

	if err != nil {
		scanErr = fmt.Errorf("failed to scan directory: %w", err)
	}

	return files, scanErr
}

// ScanWithCallback scans a directory and calls the callback for each file found
// This is useful for processing files as they're discovered without storing them all in memory
func (s *Scanner) ScanWithCallback(rootPath string, callback func(*FileInfo) error) error {
	// Normalize the path
	rootPath, err := filepath.Abs(rootPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check if root path exists
	if _, err := os.Stat(rootPath); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", rootPath)
	}

	// Walk the directory tree
	return filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		// Handle walk errors
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to access %s: %v\n", path, err)
			return filepath.SkipDir
		}

		// Skip directories in the skip list
		if info.IsDir() && path != rootPath {
			if s.SkipDirs[info.Name()] {
				return filepath.SkipDir
			}
		}

		// Create FileInfo and call callback
		fileInfo := NewFileInfo(path, info)
		if err := callback(fileInfo); err != nil {
			return fmt.Errorf("callback error for %s: %w", path, err)
		}

		return nil
	})
}

// CountFiles counts the number of files in a directory without storing them
func (s *Scanner) CountFiles(rootPath string) (int, error) {
	count := 0
	err := s.ScanWithCallback(rootPath, func(fi *FileInfo) error {
		if !fi.IsDir {
			count++
		}
		return nil
	})
	return count, err
}
