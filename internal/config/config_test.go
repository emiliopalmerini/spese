package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		wantErr     bool
		errorString string
	}{
		{
			name: "valid sqlite backend config",
			config: Config{
				Port:          "8081",
				DataBackend:   "sqlite",
				SQLiteDBPath:  "./test.db",
				AMQPURL:       "amqp://guest:guest@localhost:5672/",
				AMQPExchange:  "test_exchange",
				AMQPQueue:     "test_queue",
				SyncBatchSize: 5,
				SyncInterval:  15 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "invalid port - non-numeric",
			config: Config{
				Port:          "abc",
				DataBackend:   "sqlite",
				SQLiteDBPath:  "./test.db",
				SyncBatchSize: 10,
				SyncInterval:  30 * time.Second,
			},
			wantErr:     true,
			errorString: "invalid port 'abc': must be a number",
		},
		{
			name: "invalid port - out of range low",
			config: Config{
				Port:          "0",
				DataBackend:   "sqlite",
				SQLiteDBPath:  "./test.db",
				SyncBatchSize: 10,
				SyncInterval:  30 * time.Second,
			},
			wantErr:     true,
			errorString: "invalid port 0: must be between 1 and 65535",
		},
		{
			name: "invalid port - out of range high",
			config: Config{
				Port:          "70000",
				DataBackend:   "sqlite",
				SQLiteDBPath:  "./test.db",
				SyncBatchSize: 10,
				SyncInterval:  30 * time.Second,
			},
			wantErr:     true,
			errorString: "invalid port 70000: must be between 1 and 65535",
		},
		{
			name: "invalid data backend",
			config: Config{
				Port:          "8080",
				DataBackend:   "invalid",
				SyncBatchSize: 10,
				SyncInterval:  30 * time.Second,
			},
			wantErr:     true,
			errorString: "invalid data backend 'invalid': must be one of [sheets sqlite]",
		},
		{
			name: "sqlite backend missing database path",
			config: Config{
				Port:          "8080",
				DataBackend:   "sqlite",
				SQLiteDBPath:  "",
				SyncBatchSize: 10,
				SyncInterval:  30 * time.Second,
			},
			wantErr:     true,
			errorString: "SQLite database path cannot be empty when using sqlite backend",
		},
		{
			name: "invalid AMQP URL",
			config: Config{
				Port:          "8080",
				DataBackend:   "sqlite",
				SQLiteDBPath:  "./test.db",
				AMQPURL:       "://invalid-url",
				SyncBatchSize: 10,
				SyncInterval:  30 * time.Second,
			},
			wantErr:     true,
			errorString: "invalid AMQP URL",
		},
		{
			name: "invalid AMQP URL scheme",
			config: Config{
				Port:          "8080",
				DataBackend:   "sqlite",
				SQLiteDBPath:  "./test.db",
				AMQPURL:       "http://localhost:5672/",
				SyncBatchSize: 10,
				SyncInterval:  30 * time.Second,
			},
			wantErr:     true,
			errorString: "invalid AMQP URL scheme 'http': must be 'amqp' or 'amqps'",
		},
		{
			name: "AMQP URL without exchange",
			config: Config{
				Port:          "8080",
				DataBackend:   "sqlite",
				SQLiteDBPath:  "./test.db",
				AMQPURL:       "amqp://localhost:5672/",
				AMQPExchange:  "",
				AMQPQueue:     "test_queue",
				SyncBatchSize: 10,
				SyncInterval:  30 * time.Second,
			},
			wantErr:     true,
			errorString: "AMQP exchange name cannot be empty when AMQP URL is provided",
		},
		{
			name: "AMQP URL without queue",
			config: Config{
				Port:          "8080",
				DataBackend:   "sqlite",
				SQLiteDBPath:  "./test.db",
				AMQPURL:       "amqp://localhost:5672/",
				AMQPExchange:  "test_exchange",
				AMQPQueue:     "",
				SyncBatchSize: 10,
				SyncInterval:  30 * time.Second,
			},
			wantErr:     true,
			errorString: "AMQP queue name cannot be empty when AMQP URL is provided",
		},
		{
			name: "sheets backend missing spreadsheet ID",
			config: Config{
				Port:                  "8080",
				DataBackend:           "sheets",
				GoogleSpreadsheetID:   "",
				GoogleSheetName:       "Expenses",
				GoogleOAuthClientJSON: "{}",
				GoogleOAuthTokenJSON:  "{}",
				SyncBatchSize:         10,
				SyncInterval:          30 * time.Second,
			},
			wantErr:     true,
			errorString: "Google Spreadsheet ID is required when using sheets backend",
		},
		{
			name: "sheets backend missing sheet name",
			config: Config{
				Port:                  "8080",
				DataBackend:           "sheets",
				GoogleSpreadsheetID:   "123456789",
				GoogleSheetName:       "",
				GoogleOAuthClientJSON: "{}",
				GoogleOAuthTokenJSON:  "{}",
				SyncBatchSize:         10,
				SyncInterval:          30 * time.Second,
			},
			wantErr:     true,
			errorString: "Google Sheet name is required when using sheets backend",
		},
		{
			name: "sheets backend missing OAuth client",
			config: Config{
				Port:                 "8080",
				DataBackend:          "sheets",
				GoogleSpreadsheetID:  "123456789",
				GoogleSheetName:      "Expenses",
				GoogleOAuthTokenJSON: "{}",
				SyncBatchSize:        10,
				SyncInterval:         30 * time.Second,
			},
			wantErr:     true,
			errorString: "either GOOGLE_OAUTH_CLIENT_FILE or GOOGLE_OAUTH_CLIENT_JSON must be provided for sheets backend",
		},
		{
			name: "sheets backend missing OAuth token",
			config: Config{
				Port:                  "8080",
				DataBackend:           "sheets",
				GoogleSpreadsheetID:   "123456789",
				GoogleSheetName:       "Expenses",
				GoogleOAuthClientJSON: "{}",
				SyncBatchSize:         10,
				SyncInterval:          30 * time.Second,
			},
			wantErr:     true,
			errorString: "either GOOGLE_OAUTH_TOKEN_FILE or GOOGLE_OAUTH_TOKEN_JSON must be provided for sheets backend",
		},
		{
			name: "invalid sync batch size - too small",
			config: Config{
				Port:          "8080",
				DataBackend:   "sqlite",
				SQLiteDBPath:  "./test.db",
				SyncBatchSize: 0,
				SyncInterval:  30 * time.Second,
			},
			wantErr:     true,
			errorString: "invalid sync batch size 0: must be at least 1",
		},
		{
			name: "invalid sync batch size - too large",
			config: Config{
				Port:          "8080",
				DataBackend:   "sqlite",
				SQLiteDBPath:  "./test.db",
				SyncBatchSize: 2000,
				SyncInterval:  30 * time.Second,
			},
			wantErr:     true,
			errorString: "invalid sync batch size 2000: must be at most 1000",
		},
		{
			name: "invalid sync interval - too short",
			config: Config{
				Port:          "8080",
				DataBackend:   "sqlite",
				SQLiteDBPath:  "./test.db",
				SyncBatchSize: 10,
				SyncInterval:  500 * time.Millisecond,
			},
			wantErr:     true,
			errorString: "invalid sync interval 500ms: must be at least 1 second",
		},
		{
			name: "invalid sync interval - too long",
			config: Config{
				Port:          "8080",
				DataBackend:   "sqlite",
				SQLiteDBPath:  "./test.db",
				SyncBatchSize: 10,
				SyncInterval:  25 * time.Hour,
			},
			wantErr:     true,
			errorString: "invalid sync interval 25h0m0s: must be at most 24 hours",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Config.Validate() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.errorString != "" && !contains(err.Error(), tt.errorString) {
					t.Errorf("Config.Validate() error = %v, want error containing %v", err.Error(), tt.errorString)
				}
			} else {
				if err != nil {
					t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
				}
			}
		})
	}
}

func TestConfig_ValidateWithFiles(t *testing.T) {
	// Create temp directory for test files
	tmpDir := t.TempDir()

	// Create test OAuth files
	clientFile := filepath.Join(tmpDir, "client.json")
	tokenFile := filepath.Join(tmpDir, "token.json")

	if err := os.WriteFile(clientFile, []byte(`{"client_id":"test"}`), 0644); err != nil {
		t.Fatalf("Failed to create test client file: %v", err)
	}
	if err := os.WriteFile(tokenFile, []byte(`{"access_token":"test"}`), 0644); err != nil {
		t.Fatalf("Failed to create test token file: %v", err)
	}

	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid sheets backend with files",
			config: Config{
				Port:                  "8080",
				DataBackend:           "sheets",
				GoogleSpreadsheetID:   "123456789",
				GoogleSheetName:       "Expenses",
				GoogleOAuthClientFile: clientFile,
				GoogleOAuthTokenFile:  tokenFile,
				SyncBatchSize:         10,
				SyncInterval:          30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "sheets backend with non-existent client file",
			config: Config{
				Port:                  "8080",
				DataBackend:           "sheets",
				GoogleSpreadsheetID:   "123456789",
				GoogleSheetName:       "Expenses",
				GoogleOAuthClientFile: "/non/existent/file.json",
				GoogleOAuthTokenJSON:  "{}",
				SyncBatchSize:         10,
				SyncInterval:          30 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "sheets backend with non-existent token file",
			config: Config{
				Port:                  "8080",
				DataBackend:           "sheets",
				GoogleSpreadsheetID:   "123456789",
				GoogleSheetName:       "Expenses",
				GoogleOAuthClientJSON: "{}",
				GoogleOAuthTokenFile:  "/non/existent/file.json",
				SyncBatchSize:         10,
				SyncInterval:          30 * time.Second,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoad(t *testing.T) {
	// Save original env vars
	originalVars := map[string]string{
		"PORT":            os.Getenv("PORT"),
		"DATA_BACKEND":    os.Getenv("DATA_BACKEND"),
		"SQLITE_DB_PATH":  os.Getenv("SQLITE_DB_PATH"),
		"AMQP_URL":        os.Getenv("AMQP_URL"),
		"SYNC_BATCH_SIZE": os.Getenv("SYNC_BATCH_SIZE"),
		"SYNC_INTERVAL":   os.Getenv("SYNC_INTERVAL"),
	}

	// Clean environment
	for key := range originalVars {
		os.Unsetenv(key)
	}

	// Restore env vars at end of test
	defer func() {
		for key, value := range originalVars {
			if value != "" {
				os.Setenv(key, value)
			} else {
				os.Unsetenv(key)
			}
		}
	}()

	t.Run("default values", func(t *testing.T) {
		cfg := Load()

		if cfg.Port != "8081" {
			t.Errorf("Load() Port = %v, want 8081", cfg.Port)
		}
		if cfg.DataBackend != "sqlite" {
			t.Errorf("Load() DataBackend = %v, want sqlite", cfg.DataBackend)
		}
		if cfg.SQLiteDBPath != "./data/spese.db" {
			t.Errorf("Load() SQLiteDBPath = %v, want ./data/spese.db", cfg.SQLiteDBPath)
		}
		if cfg.SyncBatchSize != 10 {
			t.Errorf("Load() SyncBatchSize = %v, want 10", cfg.SyncBatchSize)
		}
		if cfg.SyncInterval != 30*time.Second {
			t.Errorf("Load() SyncInterval = %v, want 30s", cfg.SyncInterval)
		}
	})

	t.Run("environment variables", func(t *testing.T) {
		os.Setenv("PORT", "9090")
		os.Setenv("DATA_BACKEND", "sqlite")
		os.Setenv("SQLITE_DB_PATH", "/tmp/test.db")
		os.Setenv("AMQP_URL", "amqp://test:test@localhost:5672/")
		os.Setenv("SYNC_BATCH_SIZE", "25")
		os.Setenv("SYNC_INTERVAL", "45s")

		cfg := Load()

		if cfg.Port != "9090" {
			t.Errorf("Load() Port = %v, want 9090", cfg.Port)
		}
		if cfg.DataBackend != "sqlite" {
			t.Errorf("Load() DataBackend = %v, want sqlite", cfg.DataBackend)
		}
		if cfg.SQLiteDBPath != "/tmp/test.db" {
			t.Errorf("Load() SQLiteDBPath = %v, want /tmp/test.db", cfg.SQLiteDBPath)
		}
		if cfg.AMQPURL != "amqp://test:test@localhost:5672/" {
			t.Errorf("Load() AMQPURL = %v, want amqp://test:test@localhost:5672/", cfg.AMQPURL)
		}
		if cfg.SyncBatchSize != 25 {
			t.Errorf("Load() SyncBatchSize = %v, want 25", cfg.SyncBatchSize)
		}
		if cfg.SyncInterval != 45*time.Second {
			t.Errorf("Load() SyncInterval = %v, want 45s", cfg.SyncInterval)
		}
	})

	t.Run("invalid environment variables use defaults", func(t *testing.T) {
		os.Setenv("SYNC_BATCH_SIZE", "invalid")
		os.Setenv("SYNC_INTERVAL", "invalid")

		cfg := Load()

		if cfg.SyncBatchSize != 10 {
			t.Errorf("Load() SyncBatchSize = %v, want 10 (default for invalid input)", cfg.SyncBatchSize)
		}
		if cfg.SyncInterval != 30*time.Second {
			t.Errorf("Load() SyncInterval = %v, want 30s (default for invalid input)", cfg.SyncInterval)
		}
	})
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}())
}
