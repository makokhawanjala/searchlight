// Package index provides data structures for efficient file path indexing and searching.
// The Trie (prefix tree) is the core structure that enables fast prefix-based searches,
// which is essential for autocomplete and "starts-with" file searches.
package index

import (
	"sort"
	"sync"
)

// TrieNode represents a single node in the Trie data structure.
//
// Why use a Trie? A Trie allows us to search for all paths starting with a given prefix
// in O(m) time where m is the length of the prefix, regardless of how many total paths
// we've indexed. This is much faster than checking every path with strings.HasPrefix().
//
// Example: If we search for "doc", the Trie immediately gives us:
// - "documents/", "docs/", "docker/" without checking "images/", "music/", etc.
type TrieNode struct {
	// children maps a single character (rune) to the next TrieNode.
	// We use map instead of array because:
	// 1. File paths can contain many different characters (letters, numbers, symbols)
	// 2. Most nodes only have a few children, so a map is more memory-efficient
	// 3. Go's map gives us O(1) average lookup time
	children map[rune]*TrieNode

	// isEndOfPath marks whether this node represents the end of a complete path.
	// This is crucial because "doc" might be a complete path AND a prefix of "docs".
	// Without this flag, we couldn't distinguish between them.
	isEndOfPath bool

	// path stores the complete path if this node marks the end of a path.
	// We store the full path here (not just incrementally building it) because:
	// 1. It's faster to retrieve complete paths during search
	// 2. It avoids reconstructing paths by traversing back up the tree
	// 3. Memory trade-off: slightly more memory for significantly faster searches
	path string
}

// Trie is the main prefix tree structure that holds all indexed file paths.
//
// Thread Safety: The Trie includes a RWMutex to allow:
// - Multiple concurrent readers (searches can happen in parallel)
// - Exclusive access for writers (only one insert/delete at a time)
// This is important because SearchLight will be:
// - Handling multiple search requests simultaneously (reads)
// - Updating the index when files change (writes)
type Trie struct {
	// root is the starting point of the Trie. It doesn't represent any character,
	// it's just the entry point for all paths.
	root *TrieNode

	// size tracks the total number of complete paths stored in the Trie.
	// We maintain this separately rather than counting nodes because:
	// 1. O(1) access to know "how many files are indexed?"
	// 2. Useful for statistics and monitoring
	size int

	// mu protects concurrent access to the Trie.
	// Why RWMutex instead of Mutex?
	// - RWMutex allows multiple simultaneous readers (RLock)
	// - Only blocks when a writer needs exclusive access (Lock)
	// - Since searches (reads) are more frequent than updates (writes), this improves performance
	mu sync.RWMutex
}

// NewTrie creates and initializes a new empty Trie.
//
// We initialize the root node immediately because:
// 1. It simplifies Insert logic (no need to check if root is nil)
// 2. The root always exists, even for an empty Trie
func NewTrie() *Trie {
	return &Trie{
		root: &TrieNode{
			children: make(map[rune]*TrieNode),
		},
		size: 0,
	}
}

// Insert adds a new path to the Trie.
//
// Algorithm:
// 1. Start at root
// 2. For each character in the path, traverse or create nodes
// 3. Mark the final node as end-of-path
//
// Time Complexity: O(m) where m is the length of the path
// Space Complexity: O(m) in worst case (when path shares no prefix with existing paths)
//
// Parameters:
//   - path: The file path to insert (e.g., "/home/user/documents/file.txt")
//
// Why we need the mutex Lock (not RLock):
// Insert modifies the Trie structure, so we need exclusive write access.
func (t *Trie) Insert(path string) {
	// Validate input - empty paths don't make sense in a file system
	if path == "" {
		return
	}

	// Acquire exclusive write lock - no other reads or writes can happen during insert
	t.mu.Lock()
	defer t.mu.Unlock() // Always release the lock when function exits

	// Start traversal from the root node
	current := t.root

	// Traverse the path character by character
	// Why use []rune instead of []byte?
	// - Correctly handles UTF-8 characters in file names (e.g., "文档.txt")
	// - A rune is a single Unicode code point
	for _, char := range path {
		// Check if a child node for this character already exists
		if _, exists := current.children[char]; !exists {
			// No existing node for this character, so create a new one
			// We initialize the children map immediately to avoid nil map errors later
			current.children[char] = &TrieNode{
				children: make(map[rune]*TrieNode),
			}
		}
		// Move down to the next level of the tree
		current = current.children[char]
	}

	// After processing all characters, we're at the node representing the complete path

	// Check if this path already exists in the Trie
	// This prevents counting duplicates if Insert is called multiple times with same path
	if !current.isEndOfPath {
		// This is a new path, mark it as complete and increment size
		current.isEndOfPath = true
		current.path = path
		t.size++
	} else {
		// Path already exists - update the stored path in case there are any differences
		// (e.g., case changes or normalization)
		current.path = path
	}
}

// Search returns all paths in the Trie that start with the given prefix.
//
// Algorithm:
// 1. Navigate to the node representing the prefix
// 2. If prefix exists, collect all complete paths in the subtree below it
// 3. Return results sorted alphabetically
//
// Time Complexity:
// - O(p) to find the prefix node, where p is the length of the prefix
// - O(n) to collect all matching paths, where n is the number of matches
// - O(n log n) to sort results
//
// Parameters:
//   - prefix: The prefix to search for (e.g., "doc" finds "docs", "documents", etc.)
//
// Returns:
//   - A sorted slice of all matching paths
//   - Empty slice if no matches found (we return empty slice, not nil, for consistency)
//
// Why we use RLock instead of Lock:
// Search only reads the Trie, doesn't modify it. RLock allows multiple concurrent searches.
func (t *Trie) Search(prefix string) []string {
	// Acquire read lock - multiple searches can happen simultaneously
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Handle edge case: empty prefix should return all paths
	// This is useful for "show me everything" queries
	if prefix == "" {
		return t.collectAllPaths()
	}

	// Step 1: Navigate to the node representing the prefix
	current := t.root
	for _, char := range prefix {
		child, exists := current.children[char]
		if !exists {
			// Prefix doesn't exist in the Trie - no matches possible
			return []string{}
		}
		current = child
	}

	// Step 2: We found the prefix node! Now collect all complete paths below it.
	// Why not just return here if current.isEndOfPath is true?
	// Because "doc" might be a complete path AND have children like "docs", "docker", etc.
	// We want to return ALL matches, not just exact matches.
	var results []string
	t.collectPaths(current, &results)

	// Step 3: Sort results alphabetically for consistent output
	// Users expect search results in predictable order
	sort.Strings(results)

	return results
}

// collectPaths is a helper function that recursively collects all complete paths
// from a given node downward.
//
// This is a Depth-First Search (DFS) traversal of the Trie subtree.
//
// Why recursive instead of iterative?
// - Recursive code is cleaner and easier to understand for tree traversal
// - The maximum depth is bounded by the longest path (typically < 500 characters)
// - Stack overflow risk is minimal for typical file paths
//
// Parameters:
//   - node: The starting node to collect paths from
//   - results: Pointer to slice where we accumulate found paths
//
// Why pass results as a pointer?
// - Allows us to modify the same slice across all recursive calls
// - More memory efficient than returning and merging slices at each level
func (t *Trie) collectPaths(node *TrieNode, results *[]string) {
	// Base case: if this node marks the end of a complete path, add it to results
	if node.isEndOfPath {
		*results = append(*results, node.path)
	}

	// Recursive case: traverse all children
	// We visit children in arbitrary order (map iteration is random in Go)
	// but we'll sort the final results, so order here doesn't matter
	for _, child := range node.children {
		t.collectPaths(child, results)
	}
}

// collectAllPaths returns all paths stored in the Trie.
//
// This is essentially Search("") - find all paths regardless of prefix.
// Useful for:
// - Exporting the entire index
// - Statistics gathering
// - Debugging
func (t *Trie) collectAllPaths() []string {
	var results []string
	t.collectPaths(t.root, &results)
	sort.Strings(results)
	return results
}

// Delete removes a path from the Trie.
//
// Algorithm:
// 1. Navigate to the node representing the path
// 2. Unmark it as end-of-path
// 3. Clean up unnecessary nodes (nodes with no children and not end-of-path)
//
// Why cleanup? To prevent memory waste. If we delete "/home/user/temp.txt" and it's
// the only file in /home/user/, we should remove all those nodes.
//
// Time Complexity: O(m) where m is the length of the path
//
// Parameters:
//   - path: The file path to remove
//
// Returns:
//   - true if the path was found and deleted
//   - false if the path didn't exist in the Trie
func (t *Trie) Delete(path string) bool {
	if path == "" {
		return false
	}

	// Acquire exclusive write lock
	t.mu.Lock()
	defer t.mu.Unlock()

	// We need to track the path we take through the Trie so we can clean up afterward
	// Store each node and the character that led to it
	type step struct {
		node *TrieNode
		char rune
	}
	var pathStack []step

	// Navigate to the target node, recording the path
	current := t.root
	for _, char := range path {
		child, exists := current.children[char]
		if !exists {
			// Path doesn't exist in the Trie
			return false
		}
		pathStack = append(pathStack, step{node: current, char: char})
		current = child
	}

	// Check if this is actually a complete path (not just a prefix)
	if !current.isEndOfPath {
		// This is a prefix but not a complete path
		return false
	}

	// Unmark as end-of-path and clear the stored path
	current.isEndOfPath = false
	current.path = ""
	t.size--

	// Cleanup: Remove unnecessary nodes from bottom to top
	// A node is unnecessary if:
	// 1. It doesn't mark the end of a path (isEndOfPath == false)
	// 2. It has no children (no other paths continue through it)
	//
	// We traverse backwards (from leaf to root) because we can only determine
	// if a node is unnecessary after checking its children.
	for i := len(pathStack) - 1; i >= 0; i-- {
		step := pathStack[i]
		child := step.node.children[step.char]

		// Can we remove this node?
		if !child.isEndOfPath && len(child.children) == 0 {
			// Yes! This node is no longer needed
			delete(step.node.children, step.char)
		} else {
			// No! This node is still serving a purpose
			// Either it's the end of another path, or other paths pass through it
			// Stop cleanup here - nodes above this must also be kept
			break
		}
	}

	return true
}

// Contains checks if a path exists in the Trie.
//
// This is useful for quick existence checks without retrieving all matches.
//
// Time Complexity: O(m) where m is the length of the path
//
// Parameters:
//   - path: The file path to check
//
// Returns:
//   - true if the path exists as a complete path in the Trie
//   - false otherwise
func (t *Trie) Contains(path string) bool {
	if path == "" {
		return false
	}

	// Acquire read lock
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Navigate to the target node
	current := t.root
	for _, char := range path {
		child, exists := current.children[char]
		if !exists {
			return false
		}
		current = child
	}

	// Check if this node marks a complete path
	return current.isEndOfPath
}

// Size returns the number of paths stored in the Trie.
//
// Thread-safe and O(1) because we maintain the count during Insert/Delete.
//
// Why track size separately instead of counting nodes?
// - Much faster: O(1) vs O(n) where n is number of nodes
// - Useful for statistics: "Index contains 150,000 files"
func (t *Trie) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.size
}

// Clear removes all paths from the Trie, resetting it to empty state.
//
// More efficient than deleting paths one by one because we just:
// 1. Create a new root node
// 2. Let Go's garbage collector clean up the old tree
//
// Use case: When rebuilding the entire index from scratch.
func (t *Trie) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Create a fresh root node with empty children map
	t.root = &TrieNode{
		children: make(map[rune]*TrieNode),
	}
	t.size = 0
}

// PrefixCount returns the number of paths that start with the given prefix.
//
// This is useful for:
// - Showing "123 results" before displaying them
// - Deciding whether to show autocomplete suggestions
// - Performance monitoring
//
// Time Complexity: O(p + n) where:
// - p is the length of the prefix
// - n is the number of nodes in the matching subtree
//
// Parameters:
//   - prefix: The prefix to count matches for
//
// Returns:
//   - Number of complete paths that start with the prefix
func (t *Trie) PrefixCount(prefix string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if prefix == "" {
		return t.size
	}

	// Navigate to the prefix node
	current := t.root
	for _, char := range prefix {
		child, exists := current.children[char]
		if !exists {
			return 0
		}
		current = child
	}

	// Count all complete paths in this subtree
	return t.countPaths(current)
}

// countPaths recursively counts all complete paths from a given node downward.
//
// Helper function for PrefixCount.
func (t *Trie) countPaths(node *TrieNode) int {
	count := 0

	// Count this node if it's a complete path
	if node.isEndOfPath {
		count++
	}

	// Recursively count all children
	for _, child := range node.children {
		count += t.countPaths(child)
	}

	return count
}

// HasPrefix checks if any paths in the Trie start with the given prefix.
//
// This is faster than PrefixCount when you only need to know "are there ANY matches?"
// because it returns as soon as it finds one, without counting all of them.
//
// Use case: Showing/hiding autocomplete dropdown
//
// Time Complexity: O(p) where p is the length of the prefix (best case)
//
// Parameters:
//   - prefix: The prefix to check
//
// Returns:
//   - true if at least one path starts with the prefix
//   - false otherwise
func (t *Trie) HasPrefix(prefix string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if prefix == "" {
		return t.size > 0
	}

	// Navigate to the prefix node
	current := t.root
	for _, char := range prefix {
		child, exists := current.children[char]
		if !exists {
			return false
		}
		current = child
	}

	// If we got here, the prefix exists
	// Check if there's at least one complete path below it
	return t.hasAnyPath(current)
}

// hasAnyPath checks if there are any complete paths in the subtree.
//
// Returns true as soon as it finds one (early exit for performance).
func (t *Trie) hasAnyPath(node *TrieNode) bool {
	// Found a complete path!
	if node.isEndOfPath {
		return true
	}

	// Check children - return true on first match
	for _, child := range node.children {
		if t.hasAnyPath(child) {
			return true
		}
	}

	return false
}
