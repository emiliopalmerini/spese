// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"os"
	"strings"
)

// Config is the resolved application configuration.
type Config struct {
	Port                string
	DBPath              string
	HonkerExtensionPath string
	SpreadsheetID       string
	ServiceAccountFile  string
}

// Load reads config from the environment. It does not call godotenv.Load —
// the caller is expected to do that for local development.
func Load() (Config, error) {
	cfg := Config{
		Port:                envOr("SPESE_PORT", "8080"),
		DBPath:              envOr("SPESE_DB_PATH", "spese.db"),
		HonkerExtensionPath: strings.TrimSpace(os.Getenv("HONKER_EXTENSION_PATH")),
		SpreadsheetID:       strings.TrimSpace(os.Getenv("GOOGLE_SPREADSHEET_ID")),
		ServiceAccountFile:  strings.TrimSpace(os.Getenv("GOOGLE_SERVICE_ACCOUNT_FILE")),
	}

	if cfg.HonkerExtensionPath == "" {
		return Config{}, errors.New("HONKER_EXTENSION_PATH is required")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}
