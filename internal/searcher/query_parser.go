package searcher

import (
	"fmt"
	"regexp"
	"strings"
)

// QueryType represents the type of search query
// Different query types use different search algorithms
type QueryType int

const (
	// QueryTypePrefix searches for files starting with the query
	// Example: "/home/user" finds "/home/user/documents/file.txt"
	// Uses: Trie-based search (very fast - O(p + n))
	QueryTypePrefix QueryType = iota

	// QueryTypeExact searches for exact filename matches
	// Example: "config.yaml" finds only files named exactly "config.yaml"
	// Uses: Direct string comparison
	QueryTypeExact

	// QueryTypeSubstring searches for files containing the query
	// Example: "config" finds "myconfig.txt", "config.yaml", "app_config.ini"
	// Uses: String contains check (case-insensitive)
	QueryTypeSubstring

	// QueryTypeWildcard supports ? and * wildcards
	// Example: "file?.txt" finds "file1.txt", "file2.txt"
	// Example: "test*.go" finds "test_utils.go", "test_integration.go"
	// Uses: Pattern matching
	QueryTypeWildcard

	// QueryTypeRegex supports full regular expression matching
	// Example: "test[0-9]+\\.txt" finds "test1.txt", "test42.txt"
	// Uses: Regexp matching (slowest but most powerful)
	QueryTypeRegex

	// QueryTypeFuzzy searches with typo tolerance
	// Example: "cofig" might find "config" (1 character difference)
	// Uses: Edit distance calculation (Levenshtein distance)
	QueryTypeFuzzy
)

// String returns the string representation of QueryType
// Useful for debugging and logging
func (qt QueryType) String() string {
	switch qt {
	case QueryTypePrefix:
		return "prefix"
	case QueryTypeExact:
		return "exact"
	case QueryTypeSubstring:
		return "substring"
	case QueryTypeWildcard:
		return "wildcard"
	case QueryTypeRegex:
		return "regex"
	case QueryTypeFuzzy:
		return "fuzzy"
	default:
		return "unknown"
	}
}

// Query represents a parsed search query with all its parameters
// This struct contains everything needed to execute a search
type Query struct {
	// Raw is the original query string as entered by the user
	Raw string

	// Type is the detected or specified query type
	Type QueryType

	// Pattern is the processed search pattern (cleaned and normalized)
	Pattern string

	// CaseSensitive determines if the search should match case
	// Default: false (case-insensitive is more user-friendly)
	CaseSensitive bool

	// Extensions filters results by file extension
	// Example: [".go", ".txt"] only returns Go and text files
	Extensions []string

	// MinSize filters files smaller than this (in bytes)
	// 0 means no minimum size filter
	MinSize int64

	// MaxSize filters files larger than this (in bytes)
	// 0 means no maximum size filter
	MaxSize int64

	// FuzzyDistance is the maximum edit distance for fuzzy search
	// 1 = allow 1 character difference, 2 = allow 2, etc.
	// Only used when Type == QueryTypeFuzzy
	FuzzyDistance int

	// CompiledRegex is the compiled regular expression
	// Only populated when Type == QueryTypeRegex
	// Pre-compiling improves performance for repeated searches
	CompiledRegex *regexp.Regexp
}

// QueryParser parses search queries and determines their type
// This is the main entry point for query parsing
type QueryParser struct {
	// defaultType is used when query type cannot be auto-detected
	defaultType QueryType

	// defaultFuzzyDistance is the default edit distance for fuzzy search
	defaultFuzzyDistance int
}

// NewQueryParser creates a new query parser with default settings
// Default: substring search, fuzzy distance of 2
func NewQueryParser() *QueryParser {
	return &QueryParser{
		defaultType:          QueryTypeSubstring, // Most user-friendly default
		defaultFuzzyDistance: 2,                  // Allow 2 character differences
	}
}

// Parse parses a query string and returns a Query struct
// This is the main parsing function that determines query type and extracts filters
//
// Query Format Examples:
//   - Simple: "config"                    → substring search
//   - Prefix: "/home/user"                → prefix search (starts with /)
//   - Wildcard: "test*.txt"               → wildcard search (contains * or ?)
//   - Regex: "regex:test[0-9]+"           → regex search (starts with "regex:")
//   - Fuzzy: "fuzzy:cofig"                → fuzzy search (starts with "fuzzy:")
//   - Exact: "exact:config.yaml"          → exact match (starts with "exact:")
//   - With filters: "test ext:.go,.txt"   → substring search with extension filter
//
// Parameters:
//   - queryStr: The raw query string from the user
//
// Returns:
//   - *Query: Parsed query ready for execution
//   - error: If query is invalid (e.g., malformed regex)
func (qp *QueryParser) Parse(queryStr string) (*Query, error) {
	// Trim whitespace from query
	queryStr = strings.TrimSpace(queryStr)

	// Empty query is invalid
	if queryStr == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	// Create base query structure
	query := &Query{
		Raw:           queryStr,
		Type:          qp.defaultType,
		CaseSensitive: false,              // Default to case-insensitive
		FuzzyDistance: qp.defaultFuzzyDistance,
	}

	// Step 1: Extract filters from query
	// Filters are space-separated tokens like "ext:.go" or "size:>1MB"
	queryStr, filters := qp.extractFilters(queryStr)

	// Step 2: Detect and set query type based on pattern
	queryType, pattern := qp.detectQueryType(queryStr)
	query.Type = queryType
	query.Pattern = pattern

	// Step 3: Apply extracted filters to query
	if err := qp.applyFilters(query, filters); err != nil {
		return nil, fmt.Errorf("failed to apply filters: %w", err)
	}

	// Step 4: Validate and prepare query based on type
	if err := qp.prepareQuery(query); err != nil {
		return nil, fmt.Errorf("failed to prepare query: %w", err)
	}

	return query, nil
}

// detectQueryType determines the query type based on the pattern
// This uses heuristics to guess what the user wants
//
// Detection Rules:
// 1. Starts with "regex:" → regex
// 2. Starts with "fuzzy:" → fuzzy
// 3. Starts with "exact:" → exact
// 4. Starts with "/" → prefix (looks like a file path)
// 5. Contains "*" or "?" → wildcard
// 6. Otherwise → substring (default)
//
// Returns:
//   - QueryType: Detected type
//   - string: Pattern with type prefix removed
func (qp *QueryParser) detectQueryType(queryStr string) (QueryType, string) {
	// Check for explicit type prefixes
	if strings.HasPrefix(queryStr, "regex:") {
		return QueryTypeRegex, strings.TrimPrefix(queryStr, "regex:")
	}

	if strings.HasPrefix(queryStr, "fuzzy:") {
		return QueryTypeFuzzy, strings.TrimPrefix(queryStr, "fuzzy:")
	}

	if strings.HasPrefix(queryStr, "exact:") {
		return QueryTypeExact, strings.TrimPrefix(queryStr, "exact:")
	}

	// Check if query looks like a file path (prefix search)
	if strings.HasPrefix(queryStr, "/") || strings.HasPrefix(queryStr, "\\") {
		return QueryTypePrefix, queryStr
	}

	// Check for wildcard characters
	if strings.ContainsAny(queryStr, "*?") {
		return QueryTypeWildcard, queryStr
	}

	// Default to substring search
	return QueryTypeSubstring, queryStr
}

// extractFilters separates filters from the main query pattern
// Filters are tokens like "ext:.go" or "size:>1MB"
//
// Example:
//   Input:  "test config ext:.go,.txt size:>1KB"
//   Output: "test config", ["ext:.go,.txt", "size:>1KB"]
//
// Returns:
//   - string: Query pattern without filters
//   - []string: List of filter tokens
func (qp *QueryParser) extractFilters(queryStr string) (string, []string) {
	// Split query into tokens by whitespace
	tokens := strings.Fields(queryStr)

	var patternTokens []string
	var filters []string

	// Iterate through tokens and separate filters from pattern
	for _, token := range tokens {
		// Filters contain a colon (e.g., "ext:.go" or "size:>1MB")
		if strings.Contains(token, ":") {
			filters = append(filters, token)
		} else {
			patternTokens = append(patternTokens, token)
		}
	}

	// Rejoin pattern tokens with spaces
	pattern := strings.Join(patternTokens, " ")

	return pattern, filters
}

// applyFilters applies extracted filter tokens to the query
// This parses each filter and updates the Query struct accordingly
//
// Supported Filters:
//   - ext:.go,.txt       → Extensions filter
//   - size:>1MB          → Size filter (min)
//   - size:<10MB         → Size filter (max)
//   - case:true          → Case sensitivity
//   - fuzzy:2            → Fuzzy distance
//
// Parameters:
//   - query: The query to update
//   - filters: List of filter tokens
//
// Returns:
//   - error: If any filter is malformed
func (qp *QueryParser) applyFilters(query *Query, filters []string) error {
	for _, filter := range filters {
		// Split filter into key and value
		parts := strings.SplitN(filter, ":", 2)
		if len(parts) != 2 {
			// Malformed filter (no colon)
			continue
		}

		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch key {
		case "ext", "extension":
			// Parse extension filter: "ext:.go,.txt"
			query.Extensions = qp.parseExtensions(value)

		case "size":
			// Parse size filter: "size:>1MB" or "size:<10MB"
			if err := qp.parseSizeFilter(query, value); err != nil {
				return fmt.Errorf("invalid size filter '%s': %w", value, err)
			}

		case "case":
			// Parse case sensitivity: "case:true" or "case:false"
			query.CaseSensitive = strings.ToLower(value) == "true"

		case "fuzzy":
			// Parse fuzzy distance: "fuzzy:2"
			var distance int
			if _, err := fmt.Sscanf(value, "%d", &distance); err == nil {
				query.FuzzyDistance = distance
			}

		default:
			// Unknown filter - ignore it
			// We could return an error here, but ignoring is more forgiving
		}
	}

	return nil
}

// parseExtensions parses a comma-separated list of extensions
// Example: ".go,.txt,.md" → [".go", ".txt", ".md"]
//
// Parameters:
//   - extStr: Comma-separated extension list
//
// Returns:
//   - []string: List of extensions (normalized with leading dots)
func (qp *QueryParser) parseExtensions(extStr string) []string {
	// Split by comma
	parts := strings.Split(extStr, ",")

	var extensions []string
	for _, part := range parts {
		ext := strings.TrimSpace(part)
		if ext == "" {
			continue
		}

		// Ensure extension starts with a dot
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}

		extensions = append(extensions, ext)
	}

	return extensions
}

// parseSizeFilter parses a size filter like ">1MB" or "<10KB"
// Supports: B, KB, MB, GB units
// Supports: >, <, >=, <= operators
//
// Examples:
//   - ">1MB"   → MinSize = 1048576
//   - "<10KB"  → MaxSize = 10240
//   - ">=500B" → MinSize = 500
//
// Parameters:
//   - query: The query to update
//   - sizeStr: Size filter string
//
// Returns:
//   - error: If size format is invalid
func (qp *QueryParser) parseSizeFilter(query *Query, sizeStr string) error {
	// Determine operator and value
	var operator string
	var valueStr string

	if strings.HasPrefix(sizeStr, ">=") {
		operator = ">="
		valueStr = strings.TrimPrefix(sizeStr, ">=")
	} else if strings.HasPrefix(sizeStr, "<=") {
		operator = "<="
		valueStr = strings.TrimPrefix(sizeStr, "<=")
	} else if strings.HasPrefix(sizeStr, ">") {
		operator = ">"
		valueStr = strings.TrimPrefix(sizeStr, ">")
	} else if strings.HasPrefix(sizeStr, "<") {
		operator = "<"
		valueStr = strings.TrimPrefix(sizeStr, "<")
	} else {
		return fmt.Errorf("size filter must start with >, <, >=, or <=")
	}

	// Parse the size value
	size, err := qp.parseSize(valueStr)
	if err != nil {
		return err
	}

	// Apply operator to query
	switch operator {
	case ">", ">=":
		query.MinSize = size
	case "<", "<=":
		query.MaxSize = size
	}

	return nil
}

// parseSize converts a size string like "1MB" to bytes
// Supports: B, KB, MB, GB, TB units (case-insensitive)
//
// Examples:
//   - "100B"  → 100
//   - "1KB"   → 1024
//   - "2.5MB" → 2621440
//   - "1GB"   → 1073741824
//
// Parameters:
//   - sizeStr: Size string with unit
//
// Returns:
//   - int64: Size in bytes
//   - error: If format is invalid
func (qp *QueryParser) parseSize(sizeStr string) (int64, error) {
	sizeStr = strings.ToUpper(strings.TrimSpace(sizeStr))

	// Unit multipliers
	units := map[string]int64{
		"B":  1,
		"KB": 1024,
		"MB": 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
		"TB": 1024 * 1024 * 1024 * 1024,
	}

	// Try each unit
	for unit, multiplier := range units {
		if strings.HasSuffix(sizeStr, unit) {
			// Remove unit and parse number
			numStr := strings.TrimSuffix(sizeStr, unit)
			var num float64
			if _, err := fmt.Sscanf(numStr, "%f", &num); err != nil {
				return 0, fmt.Errorf("invalid size number: %s", numStr)
			}

			// Convert to bytes
			return int64(num * float64(multiplier)), nil
		}
	}

	// No unit found - assume bytes
	var num int64
	if _, err := fmt.Sscanf(sizeStr, "%d", &num); err != nil {
		return 0, fmt.Errorf("invalid size format: %s", sizeStr)
	}

	return num, nil
}

// prepareQuery validates and prepares the query for execution
// This includes compiling regex patterns, validating fuzzy distance, etc.
//
// Parameters:
//   - query: The query to prepare
//
// Returns:
//   - error: If query is invalid
func (qp *QueryParser) prepareQuery(query *Query) error {
	// Validate pattern is not empty after all processing
	if strings.TrimSpace(query.Pattern) == "" {
		return fmt.Errorf("search pattern cannot be empty")
	}

	// Type-specific preparation
	switch query.Type {
	case QueryTypeRegex:
		// Compile regex pattern
		regex, err := regexp.Compile(query.Pattern)
		if err != nil {
			return fmt.Errorf("invalid regex pattern: %w", err)
		}
		query.CompiledRegex = regex

	case QueryTypeFuzzy:
		// Validate fuzzy distance
		if query.FuzzyDistance < 1 {
			query.FuzzyDistance = 1
		}
		if query.FuzzyDistance > 5 {
			// Cap at 5 to prevent extremely slow searches
			query.FuzzyDistance = 5
		}

	case QueryTypeWildcard:
		// Convert wildcard pattern to regex for easier matching
		// * → .* (any characters)
		// ? → .  (single character)
		regexPattern := regexp.QuoteMeta(query.Pattern)
		regexPattern = strings.ReplaceAll(regexPattern, `\*`, ".*")
		regexPattern = strings.ReplaceAll(regexPattern, `\?`, ".")
		regexPattern = "^" + regexPattern + "$" // Anchor to start and end

		regex, err := regexp.Compile(regexPattern)
		if err != nil {
			return fmt.Errorf("invalid wildcard pattern: %w", err)
		}
		query.CompiledRegex = regex
	}

	return nil
}

// Validate checks if the query is valid and ready for execution
// This is called after parsing to ensure everything is consistent
//
// Parameters:
//   - query: The query to validate
//
// Returns:
//   - error: If query is invalid
func (qp *QueryParser) Validate(query *Query) error {
	if query == nil {
		return fmt.Errorf("query is nil")
	}

	if query.Pattern == "" {
		return fmt.Errorf("query pattern is empty")
	}

	// Regex queries must have compiled regex
	if query.Type == QueryTypeRegex && query.CompiledRegex == nil {
		return fmt.Errorf("regex query missing compiled regex")
	}

	// Wildcard queries must have compiled regex (converted from wildcard)
	if query.Type == QueryTypeWildcard && query.CompiledRegex == nil {
		return fmt.Errorf("wildcard query missing compiled regex")
	}

	// Size filters must be consistent
	if query.MinSize > 0 && query.MaxSize > 0 && query.MinSize > query.MaxSize {
		return fmt.Errorf("min size (%d) cannot be greater than max size (%d)", query.MinSize, query.MaxSize)
	}

	return nil
}