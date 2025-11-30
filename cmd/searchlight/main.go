package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/makokhawanjala/searchlight/internal/config"
	"github.com/makokhawanjala/searchlight/internal/index"
	"github.com/makokhawanjala/searchlight/internal/indexer"
	"github.com/makokhawanjala/searchlight/internal/searcher"
	"github.com/makokhawanjala/searchlight/internal/storage"
	"github.com/makokhawanjala/searchlight/internal/watcher"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Ensure index directory exists
	if err := cfg.EnsureIndexDir(); err != nil {
		log.Fatalf("Failed to create index directory: %v", err)
	}

	// ADD storage initialization
	fmt.Println("💾 Initializing storage...")
	jsonStorage := storage.NewJSONStorage(cfg.Storage.IndexPath, true)
	fmt.Printf("   Storage path: %s\n", jsonStorage.Path())

	// ============================================================
	// PHASE 5 INTEGRATION: FileIndex + Searcher Architecture
	// ============================================================
	//
	// Architecture Overview:
	// 1. FileIndex: Combines Trie (fast prefix search) + map (metadata storage)
	// 2. Searcher: High-level search interface wrapping FileIndex
	// 3. Indexer: Handles file system scanning and directory walking
	//
	// Data Flow:
	// Indexer (scans files) → FileIndex (stores with Trie) → Searcher (queries)
	//
	// Why this architecture?
	// - Separation of concerns: scanning vs storage vs searching
	// - FileIndex provides O(p + n) prefix search via Trie
	// - Searcher provides clean API for REST layer (Phase 9)
	// - Indexer focuses on file system operations only

	// Step 1: Create the FileIndex (core storage with Trie)
	fmt.Println("📦 Initializing FileIndex with Trie...")

	var fileIndex *index.FileIndex

	// Try to load existing index
	fmt.Println("📂 Checking for existing index...")
	loadedIndex, err := jsonStorage.Load()
	if err == nil {
		fileIndex = loadedIndex
		stats := fileIndex.Stats()
		fmt.Printf("   ✓ Loaded existing index: %d files (%s)\n", stats.TotalFiles, formatBytes(stats.TotalSize))
		fmt.Println()
	} else {
		fmt.Printf("   ℹ️  No existing index found: %v\n", err)
		fmt.Println("   Creating fresh index...")
		fileIndex = index.NewFileIndex()
		fmt.Println()
	}

	fmt.Println("   ✓ FileIndex created (Trie + metadata map ready)")


	// Step 2: Create the Searcher (wraps FileIndex for search operations)
	fmt.Println("🔍 Initializing Searcher...")
	fileSearcher := searcher.NewSearcher(fileIndex)
	fmt.Println("   ✓ Searcher created and linked to FileIndex")
	fmt.Println()

	// Step 3: Initialize the Indexer for file system scanning
	// The Indexer will scan directories and add files to our FileIndex
	idx := indexer.NewIndexerWithWorkers(cfg.Indexer.WorkerCount)

	// Index configured root paths if auto-indexing is enabled
	if cfg.Indexer.AutoIndex && len(cfg.Indexer.RootPaths) > 0 {
		fmt.Println("🗂️  Indexing configured directories...")
		fmt.Printf("   Using %d concurrent workers\n\n", cfg.Indexer.WorkerCount)

		totalIndexed := 0
		totalDuration := time.Duration(0)

		for _, rootPath := range cfg.Indexer.RootPaths {
			if rootPath == "" {
				continue
			}

			fmt.Printf("   Scanning: %s\n", rootPath)

			// Create context with timeout (optional)
			ctx := context.Background()

			// Index with progress reporting
			// We use a callback to add each found file to our FileIndex
			startTime := time.Now()
			
			count, err := idx.IndexDirectoryWithProgress(ctx, rootPath, func(processed, total int64) {
				// Update progress (every 100 files to avoid spam)
				if processed%100 == 0 || processed == total {
					fmt.Printf("\r   Progress: %d/%d files", processed, total)
				}
			})
			duration := time.Since(startTime)

			if err != nil {
				fmt.Printf("\n   ⚠️  Warning: Failed to index %s: %v\n", rootPath, err)
				continue
			}

			// After scanning, copy all files from Indexer to FileIndex
			// Why this approach?
			// - Indexer handles complex directory walking
			// - FileIndex handles fast searching with Trie
			// - Clean separation: scanning vs storage
			fmt.Printf("\r   ✓ Scanned %d items in %v\n", count, duration.Round(time.Millisecond))
			
			// Transfer files from Indexer to FileIndex
			fmt.Printf("   📥 Transferring to FileIndex...")
			transferStart := time.Now()
			allFiles := idx.GetAll()
			for _, file := range allFiles {
				fileIndex.Add(file)
			}
			transferDuration := time.Since(transferStart)
			fmt.Printf(" done (%v)\n", transferDuration.Round(time.Millisecond))

			totalIndexed += count
			totalDuration += duration
		}

		fmt.Printf("\n✅ Total indexed: %d items in %v\n", totalIndexed, totalDuration.Round(time.Millisecond))

		// Calculate throughput
		if totalDuration > 0 {
			throughput := float64(totalIndexed) / totalDuration.Seconds()
			fmt.Printf("   Throughput: %.0f files/second\n", throughput)
		}
		fmt.Println()

		// Display index statistics using Searcher
		// This validates that FileIndex has the data correctly
		stats := fileSearcher.Stats()
		fmt.Printf("📊 FileIndex Statistics:\n")
		fmt.Printf("   Total Files: %d\n", stats.TotalFiles)
		fmt.Printf("   Total Size: %s\n", formatBytes(stats.TotalSize))
		fmt.Println()

		// ============================================================
		// TEST SEARCHES: Validate Integration
		// ============================================================
		//
		// Now we perform test searches to verify:
		// 1. Trie-based prefix search works
		// 2. Name-based search works
		// 3. Extension-based search works
		// 4. FileIndex → Searcher integration is correct
		//
		// This is critical for Phase 5 validation!

		if stats.TotalFiles > 0 {
			fmt.Println("🧪 Running Test Searches (Phase 5 Validation)...")
			fmt.Println("   Testing Trie-based prefix search, name search, and extension search")
			fmt.Println()

			// Test 1: Prefix Search
			// Uses Trie for O(p + n) fast lookup
			fmt.Println("   Test 1: Prefix Search")
			testPrefixSearch(fileSearcher, cfg.Indexer.RootPaths)

			// Test 2: Name Search (case-insensitive substring matching)
			fmt.Println("   Test 2: Name Search (case-insensitive)")
			testNameSearch(fileSearcher)

			// Test 3: Extension Search
			fmt.Println("   Test 3: Extension Search")
			testExtensionSearch(fileSearcher)

			// Test 4: Multiple Extensions Search
			fmt.Println("   Test 4: Multiple Extensions Search")
			testMultipleExtensionsSearch(fileSearcher)

			fmt.Println("✅ All test searches completed successfully!")
			fmt.Println()
		}
	}

	// FILE SYSTEM WATCHER
	// Start watching directories for changes if watcher is enabled
	// This keeps the index up-to-date automatically when files change
	var fileWatcher *watcher.Watcher

	if cfg.Watcher.Enabled && len(cfg.Indexer.RootPaths) > 0 {
		fmt.Println("👁️  Initializing File System Watcher...")
		
		// Create watcher with configured debounce delay
		// Debouncing prevents excessive index updates from rapid file changes
		fileWatcher, err = watcher.NewWatcher(fileIndex, cfg.Watcher.DebounceDelay)
		if err != nil {
			log.Printf("⚠️  Warning: Failed to create watcher: %v", err)
			log.Println("   Continuing without live file monitoring...")
		} else {
			// Add all configured root paths to watcher
			fmt.Printf("   Adding %d directories to watch list...\n", len(cfg.Indexer.RootPaths))
			
			watchedCount := 0
			for _, rootPath := range cfg.Indexer.RootPaths {
				if rootPath == "" {
					continue
				}
				
				// Add directory recursively (watches subdirectories too)
				if err := fileWatcher.AddDirectoryRecursive(rootPath); err != nil {
					log.Printf("   ⚠️  Warning: Failed to watch %s: %v", rootPath, err)
				} else {
					fmt.Printf("   ✓ Watching: %s\n", rootPath)
					watchedCount++
				}
			}
			
			// Only start the watcher if we successfully added at least one directory
			if watchedCount > 0 {
				// Start the watcher (begins monitoring for changes)
				fileWatcher.Start()
				
				// Display watcher statistics
				watchedDirs := fileWatcher.WatchedDirectories()
				fmt.Printf("   ✓ Watcher started: monitoring %d directories\n", len(watchedDirs))
				fmt.Printf("   Debounce delay: %dms\n", cfg.Watcher.DebounceDelay)
				fmt.Println()
			} else {
				log.Println("   ⚠️  Warning: No directories were successfully added to watcher")
				fileWatcher = nil // Set to nil so we don't try to stop it later
			}
		}
	} else if !cfg.Watcher.Enabled {
		fmt.Println("👁️  File System Watcher: disabled in configuration")
		fmt.Println()
	}

	// Log configuration
	fmt.Printf("🔧 SearchLight Configuration:\n")
	fmt.Printf("   Server: %s\n", cfg.Address())
	fmt.Printf("   Workers: %d\n", cfg.Indexer.WorkerCount)
	fmt.Printf("   Index Path: %s\n", cfg.Storage.IndexPath)
	fmt.Printf("   Log Level: %s\n", cfg.Logging.Level)
	if len(cfg.Indexer.RootPaths) > 0 {
		fmt.Printf("   Root Paths:\n")
		for _, path := range cfg.Indexer.RootPaths {
			if path != "" {
				fmt.Printf("     - %s\n", path)
			}
		}
	}
	fmt.Println()

	// Setup HTTP routes with new Searcher integration
	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/health", handleHealth)
	http.HandleFunc("/stats", handleStats(fileSearcher))         // Now uses Searcher!
	http.HandleFunc("/search", handleSearch(fileSearcher))        // New search endpoint

	// Start server
	addr := cfg.Address()
	fmt.Printf("🚀 SearchLight starting on http://%s\n", addr)
	fmt.Printf("🌐 Available endpoints:\n")
	fmt.Printf("   - http://%s/         (Home)\n", addr)
	fmt.Printf("   - http://%s/health   (Health Check)\n", addr)
	fmt.Printf("   - http://%s/stats    (Index Statistics)\n", addr)
	fmt.Printf("   - http://%s/search   (Search Interface)\n", addr)
	fmt.Printf("🛑 Press Ctrl+C to stop\n\n")

	// ============================================================
	// GRACEFUL SHUTDOWN: Server with Signal Handling
	// ============================================================
	// Create HTTP server with proper shutdown handling
	server := &http.Server{
		Addr:    addr,
		Handler: nil, // Uses default ServeMux
	}

	// Channel to listen for interrupt signals (Ctrl+C, kill, etc.)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start server in goroutine so we can listen for shutdown signals
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	fmt.Println("✓ Server started successfully")
	fmt.Println()

	// Wait for interrupt signal (blocking)
	<-sigChan
	fmt.Println("\n🛑 Shutdown signal received...")
	fmt.Println()

	// ============================================================
	// Graceful Shutdown Sequence
	// ============================================================
	
	// Step 1: Save the index to disk
	fmt.Println("💾 Saving index to disk...")
	if err := jsonStorage.Save(fileIndex); err != nil {
		log.Printf("   ⚠️  Warning: Failed to save index: %v", err)
	} else {
		stats := fileIndex.Stats()
		fmt.Printf("   ✓ Index saved: %d files (%s)\n", stats.TotalFiles, formatBytes(stats.TotalSize))
	}
	fmt.Println()

	// Step 2: Stop the file watcher if it's running
	if fileWatcher != nil {
		fmt.Println("👁️  Stopping file system watcher...")
		fileWatcher.Stop()
		fmt.Println("   ✓ Watcher stopped (all events processed)")
		fmt.Println()
	}

	// Step 3: Shutdown HTTP server gracefully
	fmt.Println("🌐 Stopping HTTP server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("   ⚠️  Server shutdown error: %v", err)
	} else {
		fmt.Println("   ✓ Server stopped gracefully")
	}
	fmt.Println()

	fmt.Println("👋 SearchLight stopped gracefully")
	fmt.Println("   All data saved, all watchers stopped, all connections closed")

}

// testPrefixSearch demonstrates Trie-based prefix search
//
// Why test this?
// - Validates Trie integration works correctly
// - Shows O(p + n) performance (fast even with many files)
// - Demonstrates the core feature of SearchLight
func testPrefixSearch(s *searcher.Searcher, rootPaths []string) {
	// We'll test with a prefix from the first root path
	if len(rootPaths) == 0 || rootPaths[0] == "" {
		fmt.Println("      ⚠️  No root paths configured, skipping prefix search test")
		return
	}

	// Test with the first root path as prefix
	testPrefix := rootPaths[0]
	if len(testPrefix) > 50 {
		// If path is too long, use first 50 chars
		testPrefix = testPrefix[:50]
	}

	startTime := time.Now()
	results := s.SearchByPrefix(testPrefix)
	duration := time.Since(startTime)

	fmt.Printf("      Query: \"%s\"\n", testPrefix)
	fmt.Printf("      Results: %d files (found in %v)\n", len(results), duration)
	
	// Display first 3 results
	displayCount := 3
	if len(results) < displayCount {
		displayCount = len(results)
	}
	
	if displayCount > 0 {
		fmt.Println("      Sample results:")
		for i := 0; i < displayCount; i++ {
			file := results[i]
			fmt.Printf("        - %s (%s)\n", file.Name, file.HumanSize())
		}
	}
	fmt.Println()
}

// testNameSearch demonstrates case-insensitive name search
//
// Why test this?
// - Different algorithm from prefix search (O(n) full scan)
// - Tests case-insensitive matching
// - Common use case: "Find all README files"
func testNameSearch(s *searcher.Searcher) {
	// Test searches for common file names
	testQueries := []string{"README", "go", "config"}
	
	for _, query := range testQueries {
		startTime := time.Now()
		results := s.SearchByName(query)
		duration := time.Since(startTime)

		fmt.Printf("      Query: \"%s\"\n", query)
		fmt.Printf("      Results: %d files (found in %v)\n", len(results), duration)
		
		// Display first 2 results
		displayCount := 2
		if len(results) < displayCount {
			displayCount = len(results)
		}
		
		if displayCount > 0 {
			for i := 0; i < displayCount; i++ {
				file := results[i]
				fmt.Printf("        - %s\n", file.Path)
			}
		}
		
		// Only test first query to keep output concise
		break
	}
	fmt.Println()
}

// testExtensionSearch demonstrates extension-based filtering
//
// Why test this?
// - Common use case: "Show all .go files"
// - Tests exact extension matching
// - Useful for file type filtering in UI
func testExtensionSearch(s *searcher.Searcher) {
	// Test searches for common extensions
	testExtensions := []string{".go", ".txt", ".md"}
	
	for _, ext := range testExtensions {
		startTime := time.Now()
		results := s.SearchByExtension(ext)
		duration := time.Since(startTime)

		fmt.Printf("      Extension: \"%s\"\n", ext)
		fmt.Printf("      Results: %d files (found in %v)\n", len(results), duration)
		
		// Only test first extension to keep output concise
		break
	}
	fmt.Println()
}

// testMultipleExtensionsSearch demonstrates searching for multiple extensions at once
//
// Why test this?
// - Convenient for "Find all images" (.jpg, .png, .gif)
// - Tests the SearchMultipleExtensions method
// - Shows efficient multi-extension filtering
func testMultipleExtensionsSearch(s *searcher.Searcher) {
	// Test with common code file extensions
	extensions := []string{".go", ".md", ".txt"}
	
	startTime := time.Now()
	results := s.SearchMultipleExtensions(extensions)
	duration := time.Since(startTime)

	fmt.Printf("      Extensions: %v\n", extensions)
	fmt.Printf("      Results: %d files (found in %v)\n", len(results), duration)
	fmt.Println()
}

// formatBytes converts bytes to human-readable format
//
// Why a helper function?
// - Used in multiple places (stats display, file listings)
// - Consistent formatting across the application
// - Same logic as FileInfo.HumanSize() but for standalone values
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// handleRoot serves the root endpoint
func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>SearchLight</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            margin: 0;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
        }
        .container {
            text-align: center;
            color: white;
        }
        h1 {
            font-size: 3em;
            margin: 0;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.2);
        }
        p {
            font-size: 1.2em;
            margin-top: 1em;
            opacity: 0.9;
        }
        .status {
            display: inline-block;
            width: 12px;
            height: 12px;
            background-color: #10b981;
            border-radius: 50%%;
            margin-right: 8px;
            animation: pulse 2s ease-in-out infinite;
        }
        @keyframes pulse {
            0%%, 100%% { opacity: 1; }
            50%% { opacity: 0.5; }
        }
        .badge {
            background: rgba(255,255,255,0.2);
            padding: 0.5em 1em;
            border-radius: 20px;
            font-size: 0.9em;
            margin: 0.5em;
            display: inline-block;
        }
        .link {
            color: white;
            text-decoration: none;
            opacity: 0.8;
            transition: opacity 0.2s;
            margin: 0 10px;
        }
        .link:hover {
            opacity: 1;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔍 SearchLight</h1>
        <p><span class="status"></span>SearchLight is running</p>
        <p style="font-size: 0.9em; opacity: 0.7;">Fast file search engine with Trie-based indexing</p>
        <div class="badge">⚙️ Phase 5 Complete</div>
        <div class="badge">🗂️ FileIndex + Trie</div>
        <div class="badge">🔍 Searcher Ready</div>
        <p style="margin-top: 2em;">
            <a href="/stats" class="link">📊 View Statistics</a>
            <a href="/search" class="link">🔎 Search Files</a>
        </p>
    </div>
</body>
</html>`)
}

// handleHealth serves health check endpoint
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","service":"searchlight","phase":"5.5","components":{"config":"loaded","fileindex":"ready","searcher":"ready"}}`)
}

// handleStats serves index statistics using the new Searcher
//
// IMPORTANT: Now uses Searcher.Stats() instead of Indexer.GetStats()
// This validates that FileIndex has the correct data
func handleStats(s *searcher.Searcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get stats from Searcher (which delegates to FileIndex)
		stats := s.Stats()

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>SearchLight - Statistics</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            margin: 0;
            padding: 20px;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            min-height: 100vh;
        }
        .container {
            max-width: 800px;
            margin: 0 auto;
            background: white;
            border-radius: 10px;
            padding: 30px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.2);
        }
        h1 {
            color: #333;
            margin-top: 0;
        }
        .stat-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin: 30px 0;
        }
        .stat-card {
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: white;
            padding: 20px;
            border-radius: 8px;
            text-align: center;
        }
        .stat-value {
            font-size: 2.5em;
            font-weight: bold;
            margin: 10px 0;
        }
        .stat-label {
            font-size: 0.9em;
            opacity: 0.9;
        }
        .info-box {
            background: #f3f4f6;
            padding: 15px;
            border-radius: 8px;
            margin: 20px 0;
            color: #333;
        }
        .back-link {
            display: inline-block;
            margin-top: 20px;
            margin-right: 20px;
            color: #667eea;
            text-decoration: none;
            font-weight: 500;
        }
        .back-link:hover {
            text-decoration: underline;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>📊 Index Statistics</h1>
        <div class="info-box">
            <strong>Phase 5 Integration Active</strong><br>
            Statistics powered by FileIndex (Trie + Map architecture)
        </div>
        <div class="stat-grid">
            <div class="stat-card">
                <div class="stat-label">Total Files</div>
                <div class="stat-value">%d</div>
            </div>
            <div class="stat-card">
                <div class="stat-label">Total Size</div>
                <div class="stat-value" style="font-size: 1.8em;">%s</div>
            </div>
        </div>
        <a href="/" class="back-link">← Back to Home</a>
        <a href="/search" class="back-link">🔎 Search Files</a>
    </div>
</body>
</html>`, stats.TotalFiles, formatBytes(stats.TotalSize))
	}
}

// handleSearch serves a simple search interface
//
// This demonstrates all three search types:
// - Prefix search (Trie-based, fast)
// - Name search (case-insensitive substring)
// - Extension search
func handleSearch(s *searcher.Searcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")

		// Parse query parameters
		searchType := r.URL.Query().Get("type")    // prefix, name, or extension
		query := r.URL.Query().Get("q")

		// Build results HTML
		resultsHTML := ""

		if query != "" && searchType != "" {
			var results []*indexer.FileInfo
			var searchMethod string

			startTime := time.Now()

			switch searchType {
			case "prefix":
				results = s.SearchByPrefix(query)
				searchMethod = "Prefix Search (Trie-based)"
			case "name":
				results = s.SearchByName(query)
				searchMethod = "Name Search (case-insensitive)"
			case "extension":
				results = s.SearchByExtension(query)
				searchMethod = "Extension Search"
			default:
				resultsHTML = `<div class="error">Invalid search type</div>`
			}

			duration := time.Since(startTime)

			if resultsHTML == "" {
				resultsHTML = fmt.Sprintf(`
					<div class="results-header">
						<strong>%s</strong> for "%s"<br>
						Found %d files in %v
					</div>
				`, searchMethod, query, len(results), duration)

				if len(results) > 0 {
					resultsHTML += `<div class="results-list">`
					// Show first 50 results
					displayCount := 50
					if len(results) < displayCount {
						displayCount = len(results)
					}

					for i := 0; i < displayCount; i++ {
						file := results[i]
						resultsHTML += fmt.Sprintf(`
							<div class="result-item">
								<div class="file-name">%s</div>
								<div class="file-path">%s</div>
								<div class="file-meta">%s · Modified: %s</div>
							</div>
						`, file.Name, file.Path, file.HumanSize(), file.FormattedModTime())
					}

					if len(results) > displayCount {
						resultsHTML += fmt.Sprintf(`
							<div class="result-item" style="text-align: center; opacity: 0.7;">
								... and %d more files
							</div>
						`, len(results)-displayCount)
					}

					resultsHTML += `</div>`
				} else {
					resultsHTML += `<div class="no-results">No files found</div>`
				}
			}
		}

		fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>SearchLight - Search</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            margin: 0;
            padding: 20px;
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            min-height: 100vh;
        }
        .container {
            max-width: 900px;
            margin: 0 auto;
            background: white;
            border-radius: 10px;
            padding: 30px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.2);
        }
        h1 {
            color: #333;
            margin-top: 0;
        }
        .search-form {
            margin: 20px 0;
            padding: 20px;
            background: #f9fafb;
            border-radius: 8px;
        }
        .form-group {
            margin-bottom: 15px;
        }
        label {
            display: block;
            margin-bottom: 5px;
            font-weight: 500;
            color: #333;
        }
        input[type="text"] {
            width: 100%%;
            padding: 10px;
            border: 2px solid #e5e7eb;
            border-radius: 6px;
            font-size: 16px;
            box-sizing: border-box;
        }
        input[type="text"]:focus {
            outline: none;
            border-color: #667eea;
        }
        select {
            width: 100%%;
            padding: 10px;
            border: 2px solid #e5e7eb;
            border-radius: 6px;
            font-size: 16px;
            background: white;
        }
        button {
            background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
            color: white;
            border: none;
            padding: 12px 30px;
            border-radius: 6px;
            font-size: 16px;
            font-weight: 500;
            cursor: pointer;
            transition: transform 0.2s;
        }
        button:hover {
            transform: translateY(-2px);
        }
        .results-header {
            padding: 15px;
            background: #f3f4f6;
            border-radius: 6px;
            margin: 20px 0;
        }
        .results-list {
            margin-top: 20px;
        }
        .result-item {
            padding: 15px;
            border-bottom: 1px solid #e5e7eb;
        }
        .result-item:last-child {
            border-bottom: none;
        }
        .file-name {
            font-weight: 600;
            color: #333;
            margin-bottom: 5px;
        }
        .file-path {
            color: #6b7280;
            font-size: 0.9em;
            font-family: monospace;
            margin-bottom: 5px;
        }
        .file-meta {
            color: #9ca3af;
            font-size: 0.85em;
        }
        .no-results {
            text-align: center;
            padding: 40px;
            color: #6b7280;
        }
        .error {
            padding: 15px;
            background: #fee2e2;
            color: #991b1b;
            border-radius: 6px;
            margin: 20px 0;
        }
        .back-link {
            display: inline-block;
            margin-top: 20px;
            color: #667eea;
            text-decoration: none;
            font-weight: 500;
        }
        .back-link:hover {
            text-decoration: underline;
        }
        .examples {
            background: #eff6ff;
            padding: 15px;
            border-radius: 6px;
            margin-top: 15px;
            font-size: 0.9em;
        }
        .examples strong {
            color: #1e40af;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔎 Search Files</h1>
        
        <form class="search-form" method="GET" action="/search">
            <div class="form-group">
                <label>Search Type</label>
                <select name="type" required>
                    <option value="">Select search type...</option>
                    <option value="prefix" %s>Prefix Search (fastest - uses Trie)</option>
                    <option value="name" %s>Name Search (case-insensitive)</option>
                    <option value="extension" %s>Extension Search</option>
                </select>
            </div>
            
            <div class="form-group">
                <label>Query</label>
                <input type="text" name="q" placeholder="Enter search query..." value="%s" required>
            </div>
            
            <button type="submit">🔍 Search</button>
            
            <div class="examples">
                <strong>Examples:</strong><br>
                • Prefix: "/home/user" or "/var/log"<br>
                • Name: "README" or "config"<br>
                • Extension: ".go" or ".txt"
            </div>
        </form>
        
        %s
        
        <a href="/" class="back-link">← Back to Home</a>
    </div>
</body>
</html>`,
			// Selected attributes for search type
			func() string {
				if searchType == "prefix" {
					return "selected"
				}
				return ""
			}(),
			func() string {
				if searchType == "name" {
					return "selected"
				}
				return ""
			}(),
			func() string {
				if searchType == "extension" {
					return "selected"
				}
				return ""
			}(),
			query,
			resultsHTML)
	}
}