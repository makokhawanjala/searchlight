package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewDefaultConfig(t *testing.T) {
	cfg := NewDefaultConfig()

	if cfg.Server.Port != DefaultPort {
		t.Errorf("expected default port %d, got %d", DefaultPort, cfg.Server.Port)
	}

	if cfg.Server.Host != DefaultHost {
		t.Errorf("expected default host %s, got %s", DefaultHost, cfg.Server.Host)
	}

	if cfg.Indexer.WorkerCount != DefaultWorkerCount {
		t.Errorf("expected default worker count %d, got %d", DefaultWorkerCount, cfg.Indexer.WorkerCount)
	}

	if cfg.Logging.Level != DefaultLogLevel {
		t.Errorf("expected default log level %s, got %s", DefaultLogLevel, cfg.Logging.Level)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "valid default config",
			config:  NewDefaultConfig(),
			wantErr: false,
		},
		{
			name: "invalid port - too low",
			config: &Config{
				Server:  ServerConfig{Port: 0, Host: "localhost"},
				Indexer: IndexerConfig{WorkerCount: 5},
				Storage: StorageConfig{IndexPath: "/tmp/test.json"},
				Logging: LoggingConfig{Level: "info", Format: "text"},
			},
			wantErr: true,
		},
		{
			name: "invalid port - too high",
			config: &Config{
				Server:  ServerConfig{Port: 70000, Host: "localhost"},
				Indexer: IndexerConfig{WorkerCount: 5},
				Storage: StorageConfig{IndexPath: "/tmp/test.json"},
				Logging: LoggingConfig{Level: "info", Format: "text"},
			},
			wantErr: true,
		},
		{
			name: "invalid worker count - too low",
			config: &Config{
				Server:  ServerConfig{Port: 8080, Host: "localhost"},
				Indexer: IndexerConfig{WorkerCount: 0},
				Storage: StorageConfig{IndexPath: "/tmp/test.json"},
				Logging: LoggingConfig{Level: "info", Format: "text"},
			},
			wantErr: true,
		},
		{
			name: "invalid worker count - too high",
			config: &Config{
				Server:  ServerConfig{Port: 8080, Host: "localhost"},
				Indexer: IndexerConfig{WorkerCount: 101},
				Storage: StorageConfig{IndexPath: "/tmp/test.json"},
				Logging: LoggingConfig{Level: "info", Format: "text"},
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			config: &Config{
				Server:  ServerConfig{Port: 8080, Host: "localhost"},
				Indexer: IndexerConfig{WorkerCount: 5},
				Storage: StorageConfig{IndexPath: "/tmp/test.json"},
				Logging: LoggingConfig{Level: "invalid", Format: "text"},
			},
			wantErr: true,
		},
		{
			name: "invalid log format",
			config: &Config{
				Server:  ServerConfig{Port: 8080, Host: "localhost"},
				Indexer: IndexerConfig{WorkerCount: 5},
				Storage: StorageConfig{IndexPath: "/tmp/test.json"},
				Logging: LoggingConfig{Level: "info", Format: "invalid"},
			},
			wantErr: true,
		},
		{
			name: "empty index path",
			config: &Config{
				Server:  ServerConfig{Port: 8080, Host: "localhost"},
				Indexer: IndexerConfig{WorkerCount: 5},
				Storage: StorageConfig{IndexPath: ""},
				Logging: LoggingConfig{Level: "info", Format: "text"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `
server:
  port: 9090
  host: 0.0.0.0

indexer:
  root_paths:
    - /tmp/test1
    - /tmp/test2
  worker_count: 10
  auto_index: true

storage:
  index_path: /tmp/searchlight/index.json

logging:
  level: debug
  format: json
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to create test config file: %v", err)
	}

	cfg := NewDefaultConfig()
	if err := cfg.loadFromFile(configPath); err != nil {
		t.Fatalf("loadFromFile() error = %v", err)
	}

	// Verify loaded values
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}

	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", cfg.Server.Host)
	}

	if cfg.Indexer.WorkerCount != 10 {
		t.Errorf("expected worker count 10, got %d", cfg.Indexer.WorkerCount)
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("expected log level debug, got %s", cfg.Logging.Level)
	}

	if cfg.Logging.Format != "json" {
		t.Errorf("expected log format json, got %s", cfg.Logging.Format)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Set environment variables
	os.Setenv("SEARCHLIGHT_PORT", "9999")
	os.Setenv("SEARCHLIGHT_HOST", "127.0.0.1")
	os.Setenv("SEARCHLIGHT_WORKERS", "8")
	os.Setenv("SEARCHLIGHT_LOG_LEVEL", "warn")
	os.Setenv("SEARCHLIGHT_LOG_FORMAT", "json")

	defer func() {
		// Clean up
		os.Unsetenv("SEARCHLIGHT_PORT")
		os.Unsetenv("SEARCHLIGHT_HOST")
		os.Unsetenv("SEARCHLIGHT_WORKERS")
		os.Unsetenv("SEARCHLIGHT_LOG_LEVEL")
		os.Unsetenv("SEARCHLIGHT_LOG_FORMAT")
	}()

	cfg := NewDefaultConfig()
	cfg.loadFromEnv()

	if cfg.Server.Port != 9999 {
		t.Errorf("expected port 9999, got %d", cfg.Server.Port)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected host 127.0.0.1, got %s", cfg.Server.Host)
	}

	if cfg.Indexer.WorkerCount != 8 {
		t.Errorf("expected worker count 8, got %d", cfg.Indexer.WorkerCount)
	}

	if cfg.Logging.Level != "warn" {
		t.Errorf("expected log level warn, got %s", cfg.Logging.Level)
	}

	if cfg.Logging.Format != "json" {
		t.Errorf("expected log format json, got %s", cfg.Logging.Format)
	}
}

func TestAddress(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Port: 8080,
			Host: "localhost",
		},
	}

	expected := "localhost:8080"
	if addr := cfg.Address(); addr != expected {
		t.Errorf("expected address %s, got %s", expected, addr)
	}
}

func TestEnsureIndexDir(t *testing.T) {
	tmpDir := t.TempDir()
	indexPath := filepath.Join(tmpDir, "nested", "dir", "index.json")

	cfg := &Config{
		Storage: StorageConfig{
			IndexPath: indexPath,
		},
	}

	if err := cfg.EnsureIndexDir(); err != nil {
		t.Fatalf("EnsureIndexDir() error = %v", err)
	}

	// Verify directory was created
	dir := filepath.Dir(indexPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Errorf("expected directory %s to exist", dir)
	}
}
