package searcher

import (
	"testing"
)

// TestQueryType_String tests QueryType string representation
func TestQueryType_String(t *testing.T) {
	tests := []struct {
		queryType QueryType
		expected  string
	}{
		{QueryTypePrefix, "prefix"},
		{QueryTypeExact, "exact"},
		{QueryTypeSubstring, "substring"},
		{QueryTypeWildcard, "wildcard"},
		{QueryTypeRegex, "regex"},
		{QueryTypeFuzzy, "fuzzy"},
		{QueryType(999), "unknown"}, // Invalid type
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.queryType.String()
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

// TestNewQueryParser tests query parser creation
func TestNewQueryParser(t *testing.T) {
	parser := NewQueryParser()

	if parser == nil {
		t.Fatal("parser should not be nil")
	}

	if parser.defaultType != QueryTypeSubstring {
		t.Errorf("expected default type substring, got %s", parser.defaultType)
	}

	if parser.defaultFuzzyDistance != 2 {
		t.Errorf("expected default fuzzy distance 2, got %d", parser.defaultFuzzyDistance)
	}
}

// TestQueryParser_Parse_EmptyQuery tests parsing empty queries
func TestQueryParser_Parse_EmptyQuery(t *testing.T) {
	parser := NewQueryParser()

	tests := []string{"", "   ", "\t", "\n"}

	for _, queryStr := range tests {
		t.Run("empty", func(t *testing.T) {
			_, err := parser.Parse(queryStr)
			if err == nil {
				t.Error("expected error for empty query")
			}
		})
	}
}

// TestQueryParser_Parse_SimpleQuery tests basic query parsing
func TestQueryParser_Parse_SimpleQuery(t *testing.T) {
	parser := NewQueryParser()

	query, err := parser.Parse("config")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if query.Raw != "config" {
		t.Errorf("expected raw 'config', got '%s'", query.Raw)
	}

	if query.Type != QueryTypeSubstring {
		t.Errorf("expected substring type, got %s", query.Type)
	}

	if query.Pattern != "config" {
		t.Errorf("expected pattern 'config', got '%s'", query.Pattern)
	}

	if query.CaseSensitive {
		t.Error("expected case-insensitive by default")
	}
}

// TestQueryParser_DetectQueryType tests query type detection
func TestQueryParser_DetectQueryType(t *testing.T) {
	parser := NewQueryParser()

	tests := []struct {
		name      string
		input     string
		wantType  QueryType
		wantPattern string
	}{
		{
			name:      "prefix_unix_path",
			input:     "/home/user/documents",
			wantType:  QueryTypePrefix,
			wantPattern: "/home/user/documents",
		},
		{
			name:      "prefix_windows_path",
			input:     "\\Users\\name\\files",
			wantType:  QueryTypePrefix,
			wantPattern: "\\Users\\name\\files",
		},
		{
			name:      "wildcard_asterisk",
			input:     "test*.txt",
			wantType:  QueryTypeWildcard,
			wantPattern: "test*.txt",
		},
		{
			name:      "wildcard_question",
			input:     "file?.go",
			wantType:  QueryTypeWildcard,
			wantPattern: "file?.go",
		},
		{
			name:      "regex_explicit",
			input:     "regex:test[0-9]+",
			wantType:  QueryTypeRegex,
			wantPattern: "test[0-9]+",
		},
		{
			name:      "fuzzy_explicit",
			input:     "fuzzy:cofig",
			wantType:  QueryTypeFuzzy,
			wantPattern: "cofig",
		},
		{
			name:      "exact_explicit",
			input:     "exact:config.yaml",
			wantType:  QueryTypeExact,
			wantPattern: "config.yaml",
		},
		{
			name:      "substring_default",
			input:     "readme",
			wantType:  QueryTypeSubstring,
			wantPattern: "readme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotPattern := parser.detectQueryType(tt.input)

			if gotType != tt.wantType {
				t.Errorf("expected type %s, got %s", tt.wantType, gotType)
			}

			if gotPattern != tt.wantPattern {
				t.Errorf("expected pattern '%s', got '%s'", tt.wantPattern, gotPattern)
			}
		})
	}
}

// TestQueryParser_ExtractFilters tests filter extraction from queries
func TestQueryParser_ExtractFilters(t *testing.T) {
	parser := NewQueryParser()

	tests := []struct {
		name           string
		input          string
		wantPattern    string
		wantFilterCount int
	}{
		{
			name:           "no_filters",
			input:          "test config",
			wantPattern:    "test config",
			wantFilterCount: 0,
		},
		{
			name:           "single_filter",
			input:          "test ext:.go",
			wantPattern:    "test",
			wantFilterCount: 1,
		},
		{
			name:           "multiple_filters",
			input:          "config ext:.yaml size:>1KB",
			wantPattern:    "config",
			wantFilterCount: 2,
		},
		{
			name:           "filter_only",
			input:          "ext:.txt",
			wantPattern:    "",
			wantFilterCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPattern, gotFilters := parser.extractFilters(tt.input)

			if gotPattern != tt.wantPattern {
				t.Errorf("expected pattern '%s', got '%s'", tt.wantPattern, gotPattern)
			}

			if len(gotFilters) != tt.wantFilterCount {
				t.Errorf("expected %d filters, got %d", tt.wantFilterCount, len(gotFilters))
			}
		})
	}
}

// TestQueryParser_ParseExtensions tests extension parsing
func TestQueryParser_ParseExtensions(t *testing.T) {
	parser := NewQueryParser()

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single_extension_with_dot",
			input:    ".go",
			expected: []string{".go"},
		},
		{
			name:     "single_extension_without_dot",
			input:    "txt",
			expected: []string{".txt"},
		},
		{
			name:     "multiple_extensions",
			input:    ".go,.txt,.md",
			expected: []string{".go", ".txt", ".md"},
		},
		{
			name:     "mixed_dots",
			input:    "go,.txt,md",
			expected: []string{".go", ".txt", ".md"},
		},
		{
			name:     "with_spaces",
			input:    ".go, .txt, .md",
			expected: []string{".go", ".txt", ".md"},
		},
		{
			name:     "empty",
			input:    "",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.parseExtensions(tt.input)

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d extensions, got %d", len(tt.expected), len(result))
				return
			}

			for i, ext := range tt.expected {
				if result[i] != ext {
					t.Errorf("expected extension '%s', got '%s'", ext, result[i])
				}
			}
		})
	}
}

// TestQueryParser_ParseSize tests size parsing
func TestQueryParser_ParseSize(t *testing.T) {
	parser := NewQueryParser()

	tests := []struct {
		name     string
		input    string
		expected int64
		wantErr  bool
	}{
		{"bytes", "100B", 100, false},
		{"kilobytes", "1KB", 1024, false},
		{"megabytes", "2MB", 2 * 1024 * 1024, false},
		{"gigabytes", "1GB", 1024 * 1024 * 1024, false},
		{"terabytes", "1TB", 1024 * 1024 * 1024 * 1024, false},
		{"decimal", "2.5MB", 2621440, false},
		{"no_unit", "500", 500, false},
		{"lowercase", "1kb", 1024, false},
		{"invalid", "abc", 0, true},
		{"negative", "-100", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parser.parseSize(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

// TestQueryParser_ParseSizeFilter tests size filter parsing
func TestQueryParser_ParseSizeFilter(t *testing.T) {
	parser := NewQueryParser()

	tests := []struct {
		name        string
		input       string
		wantMinSize int64
		wantMaxSize int64
		wantErr     bool
	}{
		{
			name:        "greater_than",
			input:       ">1MB",
			wantMinSize: 1024 * 1024,
			wantMaxSize: 0,
			wantErr:     false,
		},
		{
			name:        "less_than",
			input:       "<10MB",
			wantMinSize: 0,
			wantMaxSize: 10 * 1024 * 1024,
			wantErr:     false,
		},
		{
			name:        "greater_equal",
			input:       ">=500KB",
			wantMinSize: 500 * 1024,
			wantMaxSize: 0,
			wantErr:     false,
		},
		{
			name:        "less_equal",
			input:       "<=2GB",
			wantMinSize: 0,
			wantMaxSize: 2 * 1024 * 1024 * 1024,
			wantErr:     false,
		},
		{
			name:    "no_operator",
			input:   "1MB",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := &Query{}
			err := parser.parseSizeFilter(query, tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if query.MinSize != tt.wantMinSize {
				t.Errorf("expected MinSize %d, got %d", tt.wantMinSize, query.MinSize)
			}

			if query.MaxSize != tt.wantMaxSize {
				t.Errorf("expected MaxSize %d, got %d", tt.wantMaxSize, query.MaxSize)
			}
		})
	}
}

// TestQueryParser_Parse_WithFilters tests full query parsing with filters
func TestQueryParser_Parse_WithFilters(t *testing.T) {
	parser := NewQueryParser()

	tests := []struct {
		name            string
		input           string
		wantPattern     string
		wantType        QueryType
		wantExtensions  int
		wantCaseSensitive bool
		wantErr         bool
	}{
		{
			name:           "extension_filter",
			input:          "test ext:.go,.txt",
			wantPattern:    "test",
			wantType:       QueryTypeSubstring,
			wantExtensions: 2,
			wantErr:        false,
		},
		{
			name:              "case_sensitive",
			input:             "Config case:true",
			wantPattern:       "Config",
			wantType:          QueryTypeSubstring,
			wantCaseSensitive: true,
			wantErr:           false,
		},
		{
			name:        "wildcard_with_filter",
			input:       "test*.go ext:.go",
			wantPattern: "test*.go",
			wantType:    QueryTypeWildcard,
			wantExtensions: 1,
			wantErr:     false,
		},
		{
			name:        "regex_with_filter",
			input:       "regex:test[0-9]+ size:>1KB",
			wantPattern: "test[0-9]+",
			wantType:    QueryTypeRegex,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := parser.Parse(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if query.Pattern != tt.wantPattern {
				t.Errorf("expected pattern '%s', got '%s'", tt.wantPattern, query.Pattern)
			}

			if query.Type != tt.wantType {
				t.Errorf("expected type %s, got %s", tt.wantType, query.Type)
			}

			if len(query.Extensions) != tt.wantExtensions {
				t.Errorf("expected %d extensions, got %d", tt.wantExtensions, len(query.Extensions))
			}

			if query.CaseSensitive != tt.wantCaseSensitive {
				t.Errorf("expected case sensitive %v, got %v", tt.wantCaseSensitive, query.CaseSensitive)
			}
		})
	}
}

// TestQueryParser_PrepareQuery_Regex tests regex compilation
func TestQueryParser_PrepareQuery_Regex(t *testing.T) {
	parser := NewQueryParser()

	tests := []struct {
		name    string
		pattern string
		wantErr bool
	}{
		{"valid_regex", "test[0-9]+", false},
		{"simple_regex", ".*\\.txt", false},
		{"invalid_regex", "[", true},
		{"invalid_bracket", "test[0-", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := &Query{
				Type:    QueryTypeRegex,
				Pattern: tt.pattern,
			}

			err := parser.prepareQuery(query)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error for invalid regex")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if query.CompiledRegex == nil {
				t.Error("expected compiled regex, got nil")
			}
		})
	}
}

// TestQueryParser_PrepareQuery_Wildcard tests wildcard to regex conversion
func TestQueryParser_PrepareQuery_Wildcard(t *testing.T) {
	parser := NewQueryParser()

	tests := []struct {
		name     string
		pattern  string
		testStr  string
		wantMatch bool
	}{
		{"asterisk", "test*.txt", "test123.txt", true},
		{"asterisk_nomatch", "test*.txt", "file.txt", false},
		{"question", "file?.txt", "file1.txt", true},
		{"question_nomatch", "file?.txt", "file12.txt", false},
		{"both", "t*st?.txt", "test1.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := &Query{
				Type:    QueryTypeWildcard,
				Pattern: tt.pattern,
			}

			err := parser.prepareQuery(query)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if query.CompiledRegex == nil {
				t.Fatal("expected compiled regex, got nil")
			}

			match := query.CompiledRegex.MatchString(tt.testStr)
			if match != tt.wantMatch {
				t.Errorf("pattern '%s' match '%s': expected %v, got %v",
					tt.pattern, tt.testStr, tt.wantMatch, match)
			}
		})
	}
}

// TestQueryParser_Validate tests query validation
func TestQueryParser_Validate(t *testing.T) {
	parser := NewQueryParser()

	tests := []struct {
		name    string
		query   *Query
		wantErr bool
	}{
		{
			name: "valid_substring",
			query: &Query{
				Pattern: "test",
				Type:    QueryTypeSubstring,
			},
			wantErr: false,
		},
		{
			name:    "nil_query",
			query:   nil,
			wantErr: true,
		},
		{
			name: "empty_pattern",
			query: &Query{
				Pattern: "",
				Type:    QueryTypeSubstring,
			},
			wantErr: true,
		},
		{
			name: "regex_without_compiled",
			query: &Query{
				Pattern: "test",
				Type:    QueryTypeRegex,
			},
			wantErr: true,
		},
		{
			name: "invalid_size_range",
			query: &Query{
				Pattern: "test",
				Type:    QueryTypeSubstring,
				MinSize: 1000,
				MaxSize: 500,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parser.Validate(tt.query)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestQueryParser_ComplexQuery tests complex queries with multiple features
func TestQueryParser_ComplexQuery(t *testing.T) {
	parser := NewQueryParser()

	query, err := parser.Parse("regex:test[0-9]+ ext:.go,.txt size:>1KB size:<1MB case:true")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check all fields
	if query.Type != QueryTypeRegex {
		t.Errorf("expected regex type, got %s", query.Type)
	}

	if query.Pattern != "test[0-9]+" {
		t.Errorf("expected pattern 'test[0-9]+', got '%s'", query.Pattern)
	}

	if len(query.Extensions) != 2 {
		t.Errorf("expected 2 extensions, got %d", len(query.Extensions))
	}

	if query.MinSize != 1024 {
		t.Errorf("expected MinSize 1024, got %d", query.MinSize)
	}

	if query.MaxSize != 1024*1024 {
		t.Errorf("expected MaxSize 1048576, got %d", query.MaxSize)
	}

	if !query.CaseSensitive {
		t.Error("expected case sensitive query")
	}

	if query.CompiledRegex == nil {
		t.Error("expected compiled regex")
	}
}