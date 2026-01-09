package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// HTTP Server
	Port string

	// Database
	SQLiteDBPath string

	// Google Sheets (service account)
	GoogleSpreadsheetID      string
	GoogleSheetName          string
	GoogleServiceAccountFile string
	GoogleServiceAccountJSON string

	// Worker
	SyncBatchSize int
	SyncInterval  time.Duration

	// Recurring Processor
	RecurringProcessorInterval time.Duration

	// Backend selection
	DataBackend string
}

func Load() *Config {
	cfg := &Config{
		Port:         getEnv("PORT", "8081"),
		SQLiteDBPath: getEnv("SQLITE_DB_PATH", "./data/spese.db"),

		GoogleSpreadsheetID:      getEnv("GOOGLE_SPREADSHEET_ID", ""),
		GoogleSheetName:          getEnv("GOOGLE_SHEET_NAME", ""),
		GoogleServiceAccountFile: getEnv("GOOGLE_SERVICE_ACCOUNT_FILE", ""),
		GoogleServiceAccountJSON: getEnv("GOOGLE_SERVICE_ACCOUNT_JSON", ""),

		SyncBatchSize: getEnvInt("SYNC_BATCH_SIZE", 10),
		SyncInterval:  getEnvDuration("SYNC_INTERVAL", 30*time.Second),

		RecurringProcessorInterval: getEnvDuration("RECURRING_PROCESSOR_INTERVAL", 1*time.Hour),

		DataBackend: getEnv("DATA_BACKEND", "sqlite"),
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
	validBackends := []string{"sheets", "sqlite"}
	isValidBackend := slices.Contains(validBackends, c.DataBackend)
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

	// Validate Google Sheets configuration if backend is sheets
	if c.DataBackend == "sheets" {
		if c.GoogleSpreadsheetID == "" {
			errors = append(errors, "Google Spreadsheet ID is required when using sheets backend")
		}
		if c.GoogleSheetName == "" {
			errors = append(errors, "Google Sheet name is required when using sheets backend")
		}

		// Must have either service account file or JSON
		hasServiceAccountFile := c.GoogleServiceAccountFile != ""
		hasServiceAccountJSON := c.GoogleServiceAccountJSON != ""
		hasGoogleApplicationCredentials := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != ""

		if !hasServiceAccountFile && !hasServiceAccountJSON && !hasGoogleApplicationCredentials {
			errors = append(errors, "either GOOGLE_SERVICE_ACCOUNT_FILE, GOOGLE_SERVICE_ACCOUNT_JSON, or GOOGLE_APPLICATION_CREDENTIALS must be provided for sheets backend")
		}

		// Check if service account file exists (if specified)
		if hasServiceAccountFile {
			if _, err := os.Stat(c.GoogleServiceAccountFile); os.IsNotExist(err) {
				errors = append(errors, fmt.Sprintf("Google service account file does not exist: %s", c.GoogleServiceAccountFile))
			}
		}

		// Check if GOOGLE_APPLICATION_CREDENTIALS file exists (if specified)
		if hasGoogleApplicationCredentials {
			gacPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
			if _, err := os.Stat(gacPath); os.IsNotExist(err) {
				errors = append(errors, fmt.Sprintf("GOOGLE_APPLICATION_CREDENTIALS file does not exist: %s", gacPath))
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

	// Validate recurring processor configuration
	if c.RecurringProcessorInterval < time.Minute {
		errors = append(errors, fmt.Sprintf("invalid recurring processor interval %v: must be at least 1 minute", c.RecurringProcessorInterval))
	} else if c.RecurringProcessorInterval > 7*24*time.Hour {
		errors = append(errors, fmt.Sprintf("invalid recurring processor interval %v: must be at most 7 days", c.RecurringProcessorInterval))
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
