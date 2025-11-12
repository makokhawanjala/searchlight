package indexer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewFileInfo(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	content := []byte("Hello, World!")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatalf("failed to stat test file: %v", err)
	}

	fileInfo := NewFileInfo(testFile, info)

	// Verify fields
	if fileInfo.Path != testFile {
		t.Errorf("expected path %s, got %s", testFile, fileInfo.Path)
	}

	if fileInfo.Name != "test.txt" {
		t.Errorf("expected name test.txt, got %s", fileInfo.Name)
	}

	if fileInfo.Size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), fileInfo.Size)
	}

	if fileInfo.IsDir {
		t.Error("expected IsDir to be false for a file")
	}

	if fileInfo.Extension != ".txt" {
		t.Errorf("expected extension .txt, got %s", fileInfo.Extension)
	}
}

func TestFileInfo_HumanSize(t *testing.T) {
	tests := []struct {
		name     string
		size     int64
		expected string
	}{
		{"bytes", 512, "512 B"},
		{"kilobytes", 1536, "1.5 KB"},
		{"megabytes", 2097152, "2.0 MB"},
		{"gigabytes", 3221225472, "3.0 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fi := &FileInfo{Size: tt.size}
			result := fi.HumanSize()
			if result != tt.expected {
				t.Errorf("HumanSize() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestFileInfo_MatchesExtension(t *testing.T) {
	fi := &FileInfo{Extension: ".txt"}

	tests := []struct {
		name     string
		ext      string
		expected bool
	}{
		{"exact match with dot", ".txt", true},
		{"exact match without dot", "txt", true},
		{"no match", ".pdf", false},
		{"empty extension", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fi.MatchesExtension(tt.ext)
			if result != tt.expected {
				t.Errorf("MatchesExtension(%s) = %v, want %v", tt.ext, result, tt.expected)
			}
		})
	}
}

func TestFileInfo_FormattedModTime(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 14, 30, 45, 0, time.UTC)
	fi := &FileInfo{ModifiedTime: fixedTime}

	expected := "2024-01-15 14:30:45"
	result := fi.FormattedModTime()

	if result != expected {
		t.Errorf("FormattedModTime() = %s, want %s", result, expected)
	}
}
