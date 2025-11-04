package config

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the application configuration
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Indexer IndexerConfig `yaml:"indexer"`
	Storage StorageConfig `yaml:"storage"`
	Watcher WatcherConfig `yaml:"watcher"`
	Logging LoggingConfig `yaml:"logging"`
}

// ServerConfig holds HTTP server settings
type ServerConfig struct {
	Port int    `yaml:"port"`
	Host string `yaml:"host"`
}

// IndexerConfig holds file indexing settings
type IndexerConfig struct {
	RootPaths   []string `yaml:"root_paths"`
	WorkerCount int      `yaml:"worker_count"`
	AutoIndex   bool     `yaml:"auto_index"`
}

// StorageConfig holds persistence settings
type StorageConfig struct {
	IndexPath string `yaml:"index_path"`
}

// LoggingConfig holds logging settings
type LoggingConfig struct {
	Level  string `yaml:"level"`  // debug, info, warn, error
	Format string `yaml:"format"` // text, json
}

// WatcherConfig holds file system watcher settings
type WatcherConfig struct {
	Enabled       bool  `yaml:"enabled"`        // enable/disable file watching
	DebounceDelay int64 `yaml:"debounce_delay"` // delay in milliseconds before processing events
}

// Load loads configuration from multiple sources with priority:
// 1. Command-line flags (highest priority)
// 2. Environment variables
// 3. Configuration file
// 4. Default values (lowest priority)
func Load() (*Config, error) {
	// Start with defaults
	cfg := NewDefaultConfig()

	// Define command-line flags
	configFile := flag.String("config", "", "path to config file")
	port := flag.Int("port", 0, "server port")
	host := flag.String("host", "", "server host")
	workerCount := flag.Int("workers", 0, "number of indexer workers")
	indexPath := flag.String("index-path", "", "path to index file")
	logLevel := flag.String("log-level", "", "log level (debug, info, warn, error)")
	logFormat := flag.String("log-format", "", "log format (text, json)")
	rootPaths := flag.String("root-paths", "", "comma-separated list of root paths to index")

	flag.Parse()

	// Load from config file (if specified or exists at default location)
	configPath := *configFile
	if configPath == "" {
		configPath = getDefaultConfigPath()
	}

	if _, err := os.Stat(configPath); err == nil {
		if err := cfg.loadFromFile(configPath); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}

	// Override with environment variables
	cfg.loadFromEnv()

	// Override with command-line flags (highest priority)
	if *port != 0 {
		cfg.Server.Port = *port
	}
	if *host != "" {
		cfg.Server.Host = *host
	}
	if *workerCount != 0 {
		cfg.Indexer.WorkerCount = *workerCount
	}
	if *indexPath != "" {
		cfg.Storage.IndexPath = *indexPath
	}
	if *logLevel != "" {
		cfg.Logging.Level = *logLevel
	}
	if *logFormat != "" {
		cfg.Logging.Format = *logFormat
	}
	if *rootPaths != "" {
		cfg.Indexer.RootPaths = strings.Split(*rootPaths, ",")
		// Trim spaces from each path
		for i, path := range cfg.Indexer.RootPaths {
			cfg.Indexer.RootPaths[i] = strings.TrimSpace(path)
		}
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// loadFromFile loads configuration from a YAML file
func (c *Config) loadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("failed to parse config file: %w", err)
	}

	return nil
}

// loadFromEnv loads configuration from environment variables
func (c *Config) loadFromEnv() {
	// Server configuration
	if port := os.Getenv("SEARCHLIGHT_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			c.Server.Port = p
		}
	}
	if host := os.Getenv("SEARCHLIGHT_HOST"); host != "" {
		c.Server.Host = host
	}

	// Indexer configuration
	if workers := os.Getenv("SEARCHLIGHT_WORKERS"); workers != "" {
		if w, err := strconv.Atoi(workers); err == nil {
			c.Indexer.WorkerCount = w
		}
	}
	if autoIndex := os.Getenv("SEARCHLIGHT_AUTO_INDEX"); autoIndex != "" {
		if ai, err := strconv.ParseBool(autoIndex); err == nil {
			c.Indexer.AutoIndex = ai
		}
	}
	if rootPaths := os.Getenv("SEARCHLIGHT_ROOT_PATHS"); rootPaths != "" {
		c.Indexer.RootPaths = strings.Split(rootPaths, ",")
		for i, path := range c.Indexer.RootPaths {
			c.Indexer.RootPaths[i] = strings.TrimSpace(path)
		}
	}

	// Storage configuration
	if indexPath := os.Getenv("SEARCHLIGHT_INDEX_PATH"); indexPath != "" {
		c.Storage.IndexPath = indexPath
	}

	// Logging configuration
	if logLevel := os.Getenv("SEARCHLIGHT_LOG_LEVEL"); logLevel != "" {
		c.Logging.Level = logLevel
	}
	if logFormat := os.Getenv("SEARCHLIGHT_LOG_FORMAT"); logFormat != "" {
		c.Logging.Format = logFormat
	}
}

// Address returns the full server address (host:port)
func (c *Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// EnsureIndexDir creates the directory for the index file if it doesn't exist
func (c *Config) EnsureIndexDir() error {
	dir := filepath.Dir(c.Storage.IndexPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create index directory: %w", err)
	}
	return nil
}
