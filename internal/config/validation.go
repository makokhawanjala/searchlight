package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate server configuration
	if err := c.validateServer(); err != nil {
		return fmt.Errorf("server config: %w", err)
	}

	// Validate indexer configuration
	if err := c.validateIndexer(); err != nil {
		return fmt.Errorf("indexer config: %w", err)
	}

	// Validate storage configuration
	if err := c.validateStorage(); err != nil {
		return fmt.Errorf("storage config: %w", err)
	}

	// Validate logging configuration
	if err := c.validateLogging(); err != nil {
		return fmt.Errorf("logging config: %w", err)
	}

	return nil
}

// validateServer validates server configuration
func (c *Config) validateServer() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", c.Server.Port)
	}

	if c.Server.Host == "" {
		return fmt.Errorf("host cannot be empty")
	}

	return nil
}

// validateIndexer validates indexer configuration
func (c *Config) validateIndexer() error {
	if c.Indexer.WorkerCount < 1 {
		return fmt.Errorf("worker count must be at least 1, got %d", c.Indexer.WorkerCount)
	}

	if c.Indexer.WorkerCount > 100 {
		return fmt.Errorf("worker count cannot exceed 100, got %d", c.Indexer.WorkerCount)
	}

	// Validate root paths if auto-indexing is enabled
	if c.Indexer.AutoIndex && len(c.Indexer.RootPaths) > 0 {
		for _, path := range c.Indexer.RootPaths {
			if path == "" {
				continue // Skip empty paths
			}

			// Expand ~ to home directory
			if strings.HasPrefix(path, "~") {
				homeDir, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("cannot expand home directory: %w", err)
				}
				path = filepath.Join(homeDir, path[1:])
			}

			// Check if path exists
			info, err := os.Stat(path)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("root path does not exist: %s", path)
				}
				return fmt.Errorf("cannot access root path %s: %w", path, err)
			}

			// Check if path is a directory
			if !info.IsDir() {
				return fmt.Errorf("root path is not a directory: %s", path)
			}
		}
	}

	return nil
}

// validateStorage validates storage configuration
func (c *Config) validateStorage() error {
	if c.Storage.IndexPath == "" {
		return fmt.Errorf("index path cannot be empty")
	}

	// Check if parent directory is writable (if it exists)
	dir := filepath.Dir(c.Storage.IndexPath)
	if info, err := os.Stat(dir); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("index path parent is not a directory: %s", dir)
		}
		// Check write permission by attempting to create a temp file
		tempFile := filepath.Join(dir, ".searchlight_write_test")
		if err := os.WriteFile(tempFile, []byte{}, 0644); err != nil {
			return fmt.Errorf("index directory is not writable: %s", dir)
		}
		os.Remove(tempFile)
	}

	return nil
}

// validateLogging validates logging configuration
func (c *Config) validateLogging() error {
	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}

	if !validLevels[strings.ToLower(c.Logging.Level)] {
		return fmt.Errorf("invalid log level: %s (must be debug, info, warn, or error)", c.Logging.Level)
	}

	validFormats := map[string]bool{
		"text": true,
		"json": true,
	}

	if !validFormats[strings.ToLower(c.Logging.Format)] {
		return fmt.Errorf("invalid log format: %s (must be text or json)", c.Logging.Format)
	}

	return nil
}
