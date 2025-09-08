package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// HTTP Server
	Port string

	// Database
	SQLiteDBPath string

	// AMQP
	AMQPURL      string
	AMQPExchange string
	AMQPQueue    string

	// Google Sheets (existing)
	GoogleSpreadsheetID   string
	GoogleSheetName       string
	GoogleOAuthClientFile string
	GoogleOAuthTokenFile  string
	GoogleOAuthClientJSON string
	GoogleOAuthTokenJSON  string

	// Worker
	SyncBatchSize int
	SyncInterval  time.Duration

	// Backend selection
	DataBackend string
}

func Load() *Config {
	cfg := &Config{
		Port:         getEnv("PORT", "8081"),
		SQLiteDBPath: getEnv("SQLITE_DB_PATH", "./data/spese.db"),

		AMQPURL:      getEnv("AMQP_URL", "amqp://guest:guest@localhost:5672/"),
		AMQPExchange: getEnv("AMQP_EXCHANGE", "spese"),
		AMQPQueue:    getEnv("AMQP_QUEUE", "sync_expenses"),

		GoogleSpreadsheetID:   getEnv("GOOGLE_SPREADSHEET_ID", ""),
		GoogleSheetName:       getEnv("GOOGLE_SHEET_NAME", ""),
		GoogleOAuthClientFile: getEnv("GOOGLE_OAUTH_CLIENT_FILE", ""),
		GoogleOAuthTokenFile:  getEnv("GOOGLE_OAUTH_TOKEN_FILE", ""),
		GoogleOAuthClientJSON: getEnv("GOOGLE_OAUTH_CLIENT_JSON", ""),
		GoogleOAuthTokenJSON:  getEnv("GOOGLE_OAUTH_TOKEN_JSON", ""),

		SyncBatchSize: getEnvInt("SYNC_BATCH_SIZE", 10),
		SyncInterval:  getEnvDuration("SYNC_INTERVAL", 30*time.Second),

		DataBackend: getEnv("DATA_BACKEND", "memory"),
	}

	return cfg
}

// Validate validates the configuration and returns an error if invalid
func (c *Config) Validate() error {
	var errors []string

	// Validate port
	if port, err := strconv.Atoi(c.Port); err != nil {
		errors = append(errors, fmt.Sprintf("invalid port '%s': must be a number", c.Port))
	} else if port < 1 || port > 65535 {
		errors = append(errors, fmt.Sprintf("invalid port %d: must be between 1 and 65535", port))
	}

	// Validate data backend
	validBackends := []string{"memory", "sheets", "sqlite"}
	isValidBackend := false
	for _, backend := range validBackends {
		if c.DataBackend == backend {
			isValidBackend = true
			break
		}
	}
	if !isValidBackend {
		errors = append(errors, fmt.Sprintf("invalid data backend '%s': must be one of %v", c.DataBackend, validBackends))
	}

	// Validate SQLite configuration if backend is sqlite
	if c.DataBackend == "sqlite" {
		if c.SQLiteDBPath == "" {
			errors = append(errors, "SQLite database path cannot be empty when using sqlite backend")
		} else {
			// Check if directory exists or can be created
			dir := filepath.Dir(c.SQLiteDBPath)
			if dir != "." && dir != "" {
				if _, err := os.Stat(dir); os.IsNotExist(err) {
					if err := os.MkdirAll(dir, 0755); err != nil {
						errors = append(errors, fmt.Sprintf("cannot create SQLite database directory '%s': %v", dir, err))
					}
				}
			}
		}
	}

	// Validate AMQP URL if provided
	if c.AMQPURL != "" {
		if parsedURL, err := url.Parse(c.AMQPURL); err != nil {
			errors = append(errors, fmt.Sprintf("invalid AMQP URL '%s': %v", c.AMQPURL, err))
		} else if parsedURL.Scheme != "amqp" && parsedURL.Scheme != "amqps" {
			errors = append(errors, fmt.Sprintf("invalid AMQP URL scheme '%s': must be 'amqp' or 'amqps'", parsedURL.Scheme))
		}
	}

	// Validate AMQP exchange and queue names if AMQP is configured
	if c.AMQPURL != "" {
		if c.AMQPExchange == "" {
			errors = append(errors, "AMQP exchange name cannot be empty when AMQP URL is provided")
		}
		if c.AMQPQueue == "" {
			errors = append(errors, "AMQP queue name cannot be empty when AMQP URL is provided")
		}
	}

	// Validate Google Sheets configuration if backend is sheets
	if c.DataBackend == "sheets" {
		if c.GoogleSpreadsheetID == "" {
			errors = append(errors, "Google Spreadsheet ID is required when using sheets backend")
		}
		if c.GoogleSheetName == "" {
			errors = append(errors, "Google Sheet name is required when using sheets backend")
		}

		// Must have either client file or JSON
		hasClientFile := c.GoogleOAuthClientFile != ""
		hasClientJSON := c.GoogleOAuthClientJSON != ""
		if !hasClientFile && !hasClientJSON {
			errors = append(errors, "either GOOGLE_OAUTH_CLIENT_FILE or GOOGLE_OAUTH_CLIENT_JSON must be provided for sheets backend")
		}

		// Must have either token file or JSON
		hasTokenFile := c.GoogleOAuthTokenFile != ""
		hasTokenJSON := c.GoogleOAuthTokenJSON != ""
		if !hasTokenFile && !hasTokenJSON {
			errors = append(errors, "either GOOGLE_OAUTH_TOKEN_FILE or GOOGLE_OAUTH_TOKEN_JSON must be provided for sheets backend")
		}

		// Check if client file exists (if specified)
		if hasClientFile {
			if _, err := os.Stat(c.GoogleOAuthClientFile); os.IsNotExist(err) {
				errors = append(errors, fmt.Sprintf("Google OAuth client file does not exist: %s", c.GoogleOAuthClientFile))
			}
		}

		// Check if token file exists (if specified)
		if hasTokenFile {
			if _, err := os.Stat(c.GoogleOAuthTokenFile); os.IsNotExist(err) {
				errors = append(errors, fmt.Sprintf("Google OAuth token file does not exist: %s", c.GoogleOAuthTokenFile))
			}
		}
	}

	// Validate worker configuration
	if c.SyncBatchSize < 1 {
		errors = append(errors, fmt.Sprintf("invalid sync batch size %d: must be at least 1", c.SyncBatchSize))
	} else if c.SyncBatchSize > 1000 {
		errors = append(errors, fmt.Sprintf("invalid sync batch size %d: must be at most 1000", c.SyncBatchSize))
	}

	if c.SyncInterval < time.Second {
		errors = append(errors, fmt.Sprintf("invalid sync interval %v: must be at least 1 second", c.SyncInterval))
	} else if c.SyncInterval > 24*time.Hour {
		errors = append(errors, fmt.Sprintf("invalid sync interval %v: must be at most 24 hours", c.SyncInterval))
	}

	// Return combined errors
	if len(errors) > 0 {
		return fmt.Errorf("configuration validation failed:\n- %s", strings.Join(errors, "\n- "))
	}

	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
