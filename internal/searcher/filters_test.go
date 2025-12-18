package searcher

import (
	"testing"
	"time"

	"github.com/makokhawanjala/searchlight/internal/indexer"
)

// Helper function to create test FileInfo
func createTestFile(name string, size int64, modTime time.Time, isDir bool, ext string) *indexer.FileInfo {
	return &indexer.FileInfo{
		Path:         "/test/" + name,
		Name:         name,
		Size:         size,
		ModifiedTime: modTime,
		IsDir:        isDir,
		Extension:    ext,
	}
}

// TestFilterChain_Basic tests basic filter chain operations
func TestFilterChain_Basic(t *testing.T) {
	chain := NewFilterChain()

	if chain == nil {
		t.Fatal("NewFilterChain returned nil")
	}

	if chain.Count() != 0 {
		t.Errorf("expected 0 filters, got %d", chain.Count())
	}

	// Add a filter
	chain.Add(FileOnlyFilter())

	if chain.Count() != 1 {
		t.Errorf("expected 1 filter, got %d", chain.Count())
	}
}

// TestFilterChain_Apply tests applying filter chain
func TestFilterChain_Apply(t *testing.T) {
	chain := NewFilterChain()
	chain.Add(FileOnlyFilter())
	chain.Add(MinSizeFilter(100))

	tests := []struct {
		name     string
		file     *indexer.FileInfo
		expected bool
	}{
		{
			name:     "regular_file_large_enough",
			file:     createTestFile("test.txt", 200, time.Now(), false, ".txt"),
			expected: true,
		},
		{
			name:     "regular_file_too_small",
			file:     createTestFile("small.txt", 50, time.Now(), false, ".txt"),
			expected: false,
		},
		{
			name:     "directory",
			file:     createTestFile("testdir", 0, time.Now(), true, ""),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := chain.Apply(tt.file)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestFilterChain_ApplyToSlice tests filtering a slice of files
func TestFilterChain_ApplyToSlice(t *testing.T) {
	files := []*indexer.FileInfo{
		createTestFile("large.txt", 1000, time.Now(), false, ".txt"),
		createTestFile("small.txt", 50, time.Now(), false, ".txt"),
		createTestFile("dir", 0, time.Now(), true, ""),
		createTestFile("medium.txt", 500, time.Now(), false, ".txt"),
	}

	chain := NewFilterChain()
	chain.Add(FileOnlyFilter())
	chain.Add(MinSizeFilter(100))

	result := chain.ApplyToSlice(files)

	// Should return only regular files >= 100 bytes
	if len(result) != 2 {
		t.Errorf("expected 2 files, got %d", len(result))
	}
}

// TestSizeFilter tests size filtering
func TestSizeFilter(t *testing.T) {
	filter := SizeFilter(100, 1000)

	tests := []struct {
		name     string
		file     *indexer.FileInfo
		expected bool
	}{
		{
			name:     "within_range",
			file:     createTestFile("test.txt", 500, time.Now(), false, ".txt"),
			expected: true,
		},
		{
			name:     "too_small",
			file:     createTestFile("small.txt", 50, time.Now(), false, ".txt"),
			expected: false,
		},
		{
			name:     "too_large",
			file:     createTestFile("large.txt", 2000, time.Now(), false, ".txt"),
			expected: false,
		},
		{
			name:     "directory",
			file:     createTestFile("dir", 0, time.Now(), true, ""),
			expected: false,
		},
		{
			name:     "exact_min",
			file:     createTestFile("min.txt", 100, time.Now(), false, ".txt"),
			expected: true,
		},
		{
			name:     "exact_max",
			file:     createTestFile("max.txt", 1000, time.Now(), false, ".txt"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter(tt.file)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestMinSizeFilter tests minimum size filtering
func TestMinSizeFilter(t *testing.T) {
	filter := MinSizeFilter(1024) // 1KB minimum

	tests := []struct {
		name     string
		size     int64
		expected bool
	}{
		{"above_min", 2048, true},
		{"exactly_min", 1024, true},
		{"below_min", 512, false},
		{"zero", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := createTestFile("test.txt", tt.size, time.Now(), false, ".txt")
			result := filter(file)
			if result != tt.expected {
				t.Errorf("size %d: expected %v, got %v", tt.size, tt.expected, result)
			}
		})
	}
}

// TestMaxSizeFilter tests maximum size filtering
func TestMaxSizeFilter(t *testing.T) {
	filter := MaxSizeFilter(1024 * 1024) // 1MB maximum

	tests := []struct {
		name     string
		size     int64
		expected bool
	}{
		{"below_max", 512 * 1024, true},
		{"exactly_max", 1024 * 1024, true},
		{"above_max", 2 * 1024 * 1024, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := createTestFile("test.txt", tt.size, time.Now(), false, ".txt")
			result := filter(file)
			if result != tt.expected {
				t.Errorf("size %d: expected %v, got %v", tt.size, tt.expected, result)
			}
		})
	}
}

// TestEmptyFileFilter tests empty file filtering
func TestEmptyFileFilter(t *testing.T) {
	filter := EmptyFileFilter()

	tests := []struct {
		name     string
		file     *indexer.FileInfo
		expected bool
	}{
		{
			name:     "non_empty_file",
			file:     createTestFile("test.txt", 100, time.Now(), false, ".txt"),
			expected: true,
		},
		{
			name:     "empty_file",
			file:     createTestFile("empty.txt", 0, time.Now(), false, ".txt"),
			expected: false,
		},
		{
			name:     "directory",
			file:     createTestFile("dir", 0, time.Now(), true, ""),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter(tt.file)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestModifiedAfterFilter tests filtering by modification time (after)
func TestModifiedAfterFilter(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	tomorrow := now.AddDate(0, 0, 1)

	filter := ModifiedAfterFilter(now)

	tests := []struct {
		name     string
		modTime  time.Time
		expected bool
	}{
		{"after_cutoff", tomorrow, true},
		{"exactly_cutoff", now, false},
		{"before_cutoff", yesterday, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := createTestFile("test.txt", 100, tt.modTime, false, ".txt")
			result := filter(file)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestModifiedBeforeFilter tests filtering by modification time (before)
func TestModifiedBeforeFilter(t *testing.T) {
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)
	tomorrow := now.AddDate(0, 0, 1)

	filter := ModifiedBeforeFilter(now)

	tests := []struct {
		name     string
		modTime  time.Time
		expected bool
	}{
		{"after_cutoff", tomorrow, false},
		{"exactly_cutoff", now, false},
		{"before_cutoff", yesterday, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := createTestFile("test.txt", 100, tt.modTime, false, ".txt")
			result := filter(file)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestModifiedBetweenFilter tests filtering by time range
func TestModifiedBetweenFilter(t *testing.T) {
	start := time.Now().AddDate(0, 0, -7)  // 7 days ago
	end := time.Now().AddDate(0, 0, -1)    // yesterday

	filter := ModifiedBetweenFilter(start, end)

	tests := []struct {
		name     string
		modTime  time.Time
		expected bool
	}{
		{"before_range", start.AddDate(0, 0, -1), false},
		{"start_of_range", start, true},
		{"within_range", start.AddDate(0, 0, 3), true},
		{"end_of_range", end, true},
		{"after_range", end.AddDate(0, 0, 1), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := createTestFile("test.txt", 100, tt.modTime, false, ".txt")
			result := filter(file)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestModifiedWithinDaysFilter tests recent file filtering
func TestModifiedWithinDaysFilter(t *testing.T) {
	filter := ModifiedWithinDaysFilter(7) // Last 7 days

	now := time.Now()
	tests := []struct {
		name     string
		modTime  time.Time
		expected bool
	}{
		{"today", now, true},
		{"3_days_ago", now.AddDate(0, 0, -3), true},
		{"exactly_7_days", now.AddDate(0, 0, -7), true},
		{"8_days_ago", now.AddDate(0, 0, -8), false},
		{"30_days_ago", now.AddDate(0, 0, -30), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := createTestFile("test.txt", 100, tt.modTime, false, ".txt")
			result := filter(file)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestModifiedWithinHoursFilter tests very recent file filtering
func TestModifiedWithinHoursFilter(t *testing.T) {
	filter := ModifiedWithinHoursFilter(24) // Last 24 hours

	now := time.Now()
	tests := []struct {
		name     string
		modTime  time.Time
		expected bool
	}{
		{"now", now, true},
		{"12_hours_ago", now.Add(-12 * time.Hour), true},
		{"exactly_24_hours", now.Add(-24 * time.Hour), true},
		{"25_hours_ago", now.Add(-25 * time.Hour), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := createTestFile("test.txt", 100, tt.modTime, false, ".txt")
			result := filter(file)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestTodayFilter tests filtering for files modified today
func TestTodayFilter(t *testing.T) {
	filter := TodayFilter()

	now := time.Now()
	yesterday := now.AddDate(0, 0, -1)

	tests := []struct {
		name     string
		modTime  time.Time
		expected bool
	}{
		{"now", now, true},
		{"today_morning", time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, now.Location()), true},
		{"yesterday", yesterday, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := createTestFile("test.txt", 100, tt.modTime, false, ".txt")
			result := filter(file)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestExtensionFilter tests extension filtering
func TestExtensionFilter(t *testing.T) {
	filter := ExtensionFilter(".txt", ".md")

	tests := []struct {
		name     string
		ext      string
		expected bool
	}{
		{"txt_file", ".txt", true},
		{"md_file", ".md", true},
		{"go_file", ".go", false},
		{"no_extension", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := createTestFile("test"+tt.ext, 100, time.Now(), false, tt.ext)
			result := filter(file)
			if result != tt.expected {
				t.Errorf("ext %s: expected %v, got %v", tt.ext, tt.expected, result)
			}
		})
	}
}

// TestExtensionFilter_WithoutDot tests extension filter with extensions without dots
func TestExtensionFilter_WithoutDot(t *testing.T) {
	filter := ExtensionFilter("txt", "md") // Without dots

	tests := []struct {
		name     string
		ext      string
		expected bool
	}{
		{"txt_file", ".txt", true},
		{"md_file", ".md", true},
		{"go_file", ".go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := createTestFile("test"+tt.ext, 100, time.Now(), false, tt.ext)
			result := filter(file)
			if result != tt.expected {
				t.Errorf("ext %s: expected %v, got %v", tt.ext, tt.expected, result)
			}
		})
	}
}

// TestNoExtensionFilter tests filtering files without extensions
func TestNoExtensionFilter(t *testing.T) {
	filter := NoExtensionFilter()

	tests := []struct {
		name     string
		fileName string
		ext      string
		expected bool
	}{
		{"with_extension", "test.txt", ".txt", false},
		{"no_extension", "Makefile", "", true},
		{"directory", "dir", "", false}, // Directories should be filtered out
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isDir := tt.fileName == "dir"
			file := createTestFile(tt.fileName, 100, time.Now(), isDir, tt.ext)
			result := filter(file)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestFileTypeFilters tests predefined file type filters
func TestFileTypeFilters(t *testing.T) {
	tests := []struct {
		name     string
		filter   Filter
		ext      string
		expected bool
	}{
		{"text_file_txt", TextFileFilter(), ".txt", true},
		{"text_file_md", TextFileFilter(), ".md", true},
		{"text_file_go", TextFileFilter(), ".go", false},
		{"code_file_go", CodeFileFilter(), ".go", true},
		{"code_file_py", CodeFileFilter(), ".py", true},
		{"code_file_txt", CodeFileFilter(), ".txt", false},
		{"image_file_jpg", ImageFileFilter(), ".jpg", true},
		{"image_file_png", ImageFileFilter(), ".png", true},
		{"image_file_txt", ImageFileFilter(), ".txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := createTestFile("test"+tt.ext, 100, time.Now(), false, tt.ext)
			result := tt.filter(file)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestDirectoryFilter tests directory filtering
func TestDirectoryFilter(t *testing.T) {
	filter := DirectoryFilter()

	tests := []struct {
		name     string
		isDir    bool
		expected bool
	}{
		{"regular_file", false, false},
		{"directory", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := createTestFile("test", 0, time.Now(), tt.isDir, "")
			result := filter(file)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestFileOnlyFilter tests file-only filtering
func TestFileOnlyFilter(t *testing.T) {
	filter := FileOnlyFilter()

	tests := []struct {
		name     string
		isDir    bool
		expected bool
	}{
		{"regular_file", false, true},
		{"directory", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := createTestFile("test", 100, time.Now(), tt.isDir, ".txt")
			result := filter(file)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestHiddenFileFilter tests hidden file filtering
func TestHiddenFileFilter(t *testing.T) {
	filter := HiddenFileFilter()

	tests := []struct {
		name     string
		fileName string
		expected bool
	}{
		{"hidden_file", ".hidden", true},
		{"visible_file", "visible.txt", false},
		{"dotfile", ".gitignore", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := createTestFile(tt.fileName, 100, time.Now(), false, "")
			result := filter(file)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestCombineFiltersAND tests AND combination
func TestCombineFiltersAND(t *testing.T) {
	filter := CombineFiltersAND(
		FileOnlyFilter(),
		ExtensionFilter(".txt"),
		MinSizeFilter(100),
	)

	tests := []struct {
		name     string
		file     *indexer.FileInfo
		expected bool
	}{
		{
			name:     "all_pass",
			file:     createTestFile("test.txt", 200, time.Now(), false, ".txt"),
			expected: true,
		},
		{
			name:     "wrong_extension",
			file:     createTestFile("test.go", 200, time.Now(), false, ".go"),
			expected: false,
		},
		{
			name:     "too_small",
			file:     createTestFile("test.txt", 50, time.Now(), false, ".txt"),
			expected: false,
		},
		{
			name:     "is_directory",
			file:     createTestFile("dir", 0, time.Now(), true, ""),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter(tt.file)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestCombineFiltersOR tests OR combination
func TestCombineFiltersOR(t *testing.T) {
	filter := CombineFiltersOR(
		ExtensionFilter(".txt"),
		ExtensionFilter(".md"),
	)

	tests := []struct {
		name     string
		ext      string
		expected bool
	}{
		{"txt_file", ".txt", true},
		{"md_file", ".md", true},
		{"go_file", ".go", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := createTestFile("test"+tt.ext, 100, time.Now(), false, tt.ext)
			result := filter(file)
			if result != tt.expected {
				t.Errorf("ext %s: expected %v, got %v", tt.ext, tt.expected, result)
			}
		})
	}
}

// TestNotFilter tests NOT filter inversion
func TestNotFilter(t *testing.T) {
	filter := NotFilter(ExtensionFilter(".txt"))

	tests := []struct {
		name     string
		ext      string
		expected bool
	}{
		{"txt_file", ".txt", false}, // Inverted - .txt files are excluded
		{"md_file", ".md", true},    // Inverted - non-.txt files pass
		{"go_file", ".go", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := createTestFile("test"+tt.ext, 100, time.Now(), false, tt.ext)
			result := filter(file)
			if result != tt.expected {
				t.Errorf("ext %s: expected %v, got %v", tt.ext, tt.expected, result)
			}
		})
	}
}

// TestApplyFilters tests the convenience function
func TestApplyFilters(t *testing.T) {
	files := []*indexer.FileInfo{
		createTestFile("test1.txt", 200, time.Now(), false, ".txt"),
		createTestFile("test2.txt", 50, time.Now(), false, ".txt"),
		createTestFile("test3.md", 200, time.Now(), false, ".md"),
		createTestFile("dir", 0, time.Now(), true, ""),
	}

	result := ApplyFilters(files,
		FileOnlyFilter(),
		MinSizeFilter(100),
	)

	// Should return only regular files >= 100 bytes
	if len(result) != 2 {
		t.Errorf("expected 2 files, got %d", len(result))
	}
}
