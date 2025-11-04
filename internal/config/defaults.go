package config

import (
	"os"
	"path/filepath"
)

// Default configuration values
const (
	DefaultPort        = 8080
	DefaultHost        = "localhost"
	DefaultWorkerCount = 5
	DefaultLogLevel    = "info"
	DefaultLogFormat   = "text"
	DefaultAutoIndex   = true
	DefaultWatcherEnabled      = true 
	DefaultWatcherDebounceDelay = 100  
)

// getDefaultIndexPath returns the default path for index storage
func getDefaultIndexPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".searchlight/index.json"
	}
	return filepath.Join(homeDir, ".searchlight", "index.json")
}

// getDefaultConfigPath returns the default path for config file
func getDefaultConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(homeDir, ".searchlight", "config.yaml")
}

// NewDefaultConfig returns a Config with default values
func NewDefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: DefaultPort,
			Host: DefaultHost,
		},
		Indexer: IndexerConfig{
			RootPaths:   []string{},
			WorkerCount: DefaultWorkerCount,
			AutoIndex:   DefaultAutoIndex,
		},
		Storage: StorageConfig{
			IndexPath: getDefaultIndexPath(),
		},
		Watcher: WatcherConfig{                          
			Enabled:       DefaultWatcherEnabled,        
			DebounceDelay: DefaultWatcherDebounceDelay,  
		},                                               
		Logging: LoggingConfig{
			Level:  DefaultLogLevel,
			Format: DefaultLogFormat,
		},
	}
}
