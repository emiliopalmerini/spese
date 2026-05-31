// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"os"
	"strings"
)

// SheetMirrorBackend selects where the Honker sheet-mirror worker exports
// source tabs.
type SheetMirrorBackend string

const (
	SheetMirrorBackendAuto   SheetMirrorBackend = "auto"
	SheetMirrorBackendGoogle SheetMirrorBackend = "google"
	SheetMirrorBackendLocal  SheetMirrorBackend = "local"
	SheetMirrorBackendNone   SheetMirrorBackend = "none"
)

// Config is the resolved application configuration.
type Config struct {
	Port                string
	DBPath              string
	HonkerExtensionPath string
	SpreadsheetID       string
	ServiceAccountFile  string
	SheetMirrorBackend  SheetMirrorBackend
	LocalSheetPath      string
}

// Load reads config from the environment. It does not call godotenv.Load —
// the caller is expected to do that for local development.
func Load() (Config, error) {
	backend, err := parseSheetMirrorBackend(envOr("SPESE_SHEET_MIRROR_BACKEND", string(SheetMirrorBackendAuto)))
	if err != nil {
		return Config{}, err
	}
	if truthy(os.Getenv("SPESE_DISABLE_SHEET_MIRROR")) {
		backend = SheetMirrorBackendNone
	}

	cfg := Config{
		Port:                envOr("SPESE_PORT", "8080"),
		DBPath:              envOr("SPESE_DB_PATH", "spese.db"),
		HonkerExtensionPath: strings.TrimSpace(os.Getenv("HONKER_EXTENSION_PATH")),
		SpreadsheetID:       strings.TrimSpace(os.Getenv("GOOGLE_SPREADSHEET_ID")),
		ServiceAccountFile:  strings.TrimSpace(os.Getenv("GOOGLE_SERVICE_ACCOUNT_FILE")),
		SheetMirrorBackend:  backend,
		LocalSheetPath:      envOr("SPESE_LOCAL_SHEET_PATH", "tmp/local-sheet.json"),
	}

	if cfg.HonkerExtensionPath == "" {
		return Config{}, errors.New("HONKER_EXTENSION_PATH is required")
	}
	if cfg.ResolvedSheetMirrorBackend() == SheetMirrorBackendGoogle && (cfg.SpreadsheetID == "" || cfg.ServiceAccountFile == "") {
		return Config{}, errors.New("GOOGLE_SPREADSHEET_ID and GOOGLE_SERVICE_ACCOUNT_FILE are required for google sheet mirror")
	}
	if cfg.ResolvedSheetMirrorBackend() == SheetMirrorBackendLocal && strings.TrimSpace(cfg.LocalSheetPath) == "" {
		return Config{}, errors.New("SPESE_LOCAL_SHEET_PATH is required for local sheet mirror")
	}
	return cfg, nil
}

// ResolvedSheetMirrorBackend expands auto mode using the configured Google
// credentials.
func (c Config) ResolvedSheetMirrorBackend() SheetMirrorBackend {
	if c.SheetMirrorBackend != SheetMirrorBackendAuto {
		return c.SheetMirrorBackend
	}
	if c.SpreadsheetID != "" && c.ServiceAccountFile != "" {
		return SheetMirrorBackendGoogle
	}
	return SheetMirrorBackendNone
}

// SheetMirrorEnabled reports whether a worker should drain the durable Honker
// outbox and export rows.
func (c Config) SheetMirrorEnabled() bool {
	return c.ResolvedSheetMirrorBackend() != SheetMirrorBackendNone
}

func envOr(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func parseSheetMirrorBackend(v string) (SheetMirrorBackend, error) {
	switch SheetMirrorBackend(strings.ToLower(strings.TrimSpace(v))) {
	case SheetMirrorBackendAuto:
		return SheetMirrorBackendAuto, nil
	case SheetMirrorBackendGoogle:
		return SheetMirrorBackendGoogle, nil
	case SheetMirrorBackendLocal:
		return SheetMirrorBackendLocal, nil
	case SheetMirrorBackendNone:
		return SheetMirrorBackendNone, nil
	default:
		return "", errors.New("SPESE_SHEET_MIRROR_BACKEND must be one of auto, google, local, none")
	}
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	default:
		return false
	}
}
