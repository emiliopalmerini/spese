// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

// Config is the resolved application configuration.
type Config struct {
	Port                       string
	SpreadsheetID              string
	ServiceAccountFile         string
	RecurringProcessorInterval time.Duration
}

// Load reads config from the environment. It does not call godotenv.Load —
// the caller is expected to do that for local development.
func Load() (Config, error) {
	cfg := Config{
		Port:               envOr("SPESE_PORT", "8080"),
		SpreadsheetID:      strings.TrimSpace(os.Getenv("GOOGLE_SPREADSHEET_ID")),
		ServiceAccountFile: strings.TrimSpace(os.Getenv("GOOGLE_SERVICE_ACCOUNT_FILE")),
	}
	d, err := time.ParseDuration(envOr("RECURRING_PROCESSOR_INTERVAL", "6h"))
	if err != nil {
		return Config{}, err
	}
	cfg.RecurringProcessorInterval = d

	if cfg.SpreadsheetID == "" {
		return Config{}, errors.New("GOOGLE_SPREADSHEET_ID is required")
	}
	if cfg.ServiceAccountFile == "" {
		return Config{}, errors.New("GOOGLE_SERVICE_ACCOUNT_FILE is required")
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
