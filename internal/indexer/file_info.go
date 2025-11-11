package indexer

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FileInfo represents metadata about a file
type FileInfo struct {
	Path         string    `json:"path"`          // Full path to the file
	Name         string    `json:"name"`          // Base name of the file
	Size         int64     `json:"size"`          // Size in bytes
	ModifiedTime time.Time `json:"modified_time"` // Last modification time
	IsDir        bool      `json:"is_dir"`        // Whether this is a directory
	Extension    string    `json:"extension"`     // File extension (with dot)
}

// NewFileInfo creates a FileInfo from an os.FileInfo
func NewFileInfo(path string, info os.FileInfo) *FileInfo {
	return &FileInfo{
		Path:         path,
		Name:         info.Name(),
		Size:         info.Size(),
		ModifiedTime: info.ModTime(),
		IsDir:        info.IsDir(),
		Extension:    filepath.Ext(info.Name()),
	}
}

// HumanSize returns the file size in human-readable format
func (f *FileInfo) HumanSize() string {
	const unit = 1024
	size := float64(f.Size)

	if size < unit {
		return fmt.Sprintf("%d B", f.Size)
	}

	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", size/float64(div), "KMGTPE"[exp])
}

// FormattedModTime returns the modification time in a readable format
func (f *FileInfo) FormattedModTime() string {
	return f.ModifiedTime.Format("2006-01-02 15:04:05")
}

// MatchesExtension checks if the file has the given extension
func (f *FileInfo) MatchesExtension(ext string) bool {
	if ext == "" {
		return true
	}
	// Ensure extension has a leading dot
	if ext[0] != '.' {
		ext = "." + ext
	}
	return f.Extension == ext
}

// String returns a string representation of the FileInfo
func (f *FileInfo) String() string {
	fileType := "file"
	if f.IsDir {
		fileType = "dir"
	}
	return fmt.Sprintf("%s: %s (%s, %s)", fileType, f.Path, f.HumanSize(), f.FormattedModTime())
}
