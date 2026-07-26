// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"os"
	"strings"
)

// SheetMirrorBackend selects where the sheet-mirror worker exports source tabs.
type SheetMirrorBackend string

const (
	SheetMirrorBackendAuto   SheetMirrorBackend = "auto"
	SheetMirrorBackendGoogle SheetMirrorBackend = "google"
	SheetMirrorBackendLocal  SheetMirrorBackend = "local"
	SheetMirrorBackendNone   SheetMirrorBackend = "none"
)

// Config is the resolved application configuration.
type Config struct {
	Host               string
	Port               string
	DBPath             string
	SpreadsheetID      string
	ServiceAccountFile string
	SheetMirrorBackend SheetMirrorBackend
	LocalSheetPath     string
	RabbitMQURL        string
	RabbitMQQueue      string
	DictationEnabled   bool
	ElevenLabsAPIKey   string
	OpenCodeURL        string
	OpenCodeUsername   string
	OpenCodePassword   string
	OpenCodeProvider   string
	OpenCodeModel      string
	OpenCodeAgent      string
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
		Host:               strings.TrimSpace(os.Getenv("SPESE_HOST")),
		Port:               envOr("SPESE_PORT", "8080"),
		DBPath:             envOr("SPESE_DB_PATH", "spese.db"),
		SpreadsheetID:      strings.TrimSpace(os.Getenv("GOOGLE_SPREADSHEET_ID")),
		ServiceAccountFile: strings.TrimSpace(os.Getenv("GOOGLE_SERVICE_ACCOUNT_FILE")),
		SheetMirrorBackend: backend,
		LocalSheetPath:     envOr("SPESE_LOCAL_SHEET_PATH", "tmp/local-sheet.json"),
		RabbitMQURL:        firstEnv("SPESE_RABBITMQ_URL", "CLOUDAMQP_URL", "AMQP_URL"),
		RabbitMQQueue:      envOr("SPESE_RABBITMQ_QUEUE", "spese.sheet-sync"),
		DictationEnabled:   truthy(os.Getenv("SPESE_DICTATION_ENABLED")),
		ElevenLabsAPIKey:   strings.TrimSpace(os.Getenv("ELEVENLABS_API_KEY")),
		OpenCodeURL:        envOr("SPESE_OPENCODE_URL", "http://127.0.0.1:4096"),
		OpenCodeUsername:   envOr("SPESE_OPENCODE_USERNAME", "opencode"),
		OpenCodePassword:   strings.TrimSpace(os.Getenv("SPESE_OPENCODE_PASSWORD")),
		OpenCodeProvider:   strings.TrimSpace(os.Getenv("SPESE_OPENCODE_PROVIDER")),
		OpenCodeModel:      strings.TrimSpace(os.Getenv("SPESE_OPENCODE_MODEL")),
		OpenCodeAgent:      envOr("SPESE_OPENCODE_AGENT", "dictation"),
	}

	if cfg.ResolvedSheetMirrorBackend() == SheetMirrorBackendGoogle && (cfg.SpreadsheetID == "" || cfg.ServiceAccountFile == "") {
		return Config{}, errors.New("GOOGLE_SPREADSHEET_ID and GOOGLE_SERVICE_ACCOUNT_FILE are required for google sheet mirror")
	}
	if cfg.ResolvedSheetMirrorBackend() == SheetMirrorBackendLocal && strings.TrimSpace(cfg.LocalSheetPath) == "" {
		return Config{}, errors.New("SPESE_LOCAL_SHEET_PATH is required for local sheet mirror")
	}
	if cfg.SheetMirrorEnabled() && cfg.RabbitMQURL == "" {
		return Config{}, errors.New("SPESE_RABBITMQ_URL is required when sheet mirror is enabled")
	}
	if cfg.DictationEnabled && (cfg.ElevenLabsAPIKey == "" || cfg.OpenCodePassword == "" || cfg.OpenCodeProvider == "" || cfg.OpenCodeModel == "") {
		return Config{}, errors.New("ELEVENLABS_API_KEY, SPESE_OPENCODE_PASSWORD, SPESE_OPENCODE_PROVIDER, and SPESE_OPENCODE_MODEL are required when dictation is enabled")
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

// SheetMirrorEnabled reports whether the app should publish sheet-sync work.
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

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
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
