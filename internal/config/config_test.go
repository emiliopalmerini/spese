package config

import "testing"

func TestLoadLocalSheetMirrorBackend(t *testing.T) {
	clearEnv(t)
	t.Setenv("GOOGLE_SPREADSHEET_ID", "real-sheet-id")
	t.Setenv("GOOGLE_SERVICE_ACCOUNT_FILE", "/tmp/service-account.json")
	t.Setenv("SPESE_SHEET_MIRROR_BACKEND", "local")
	t.Setenv("SPESE_LOCAL_SHEET_PATH", "tmp/test-sheet.json")
	t.Setenv("SPESE_RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.ResolvedSheetMirrorBackend(); got != SheetMirrorBackendLocal {
		t.Fatalf("backend = %q, want %q", got, SheetMirrorBackendLocal)
	}
	if !cfg.SheetMirrorEnabled() {
		t.Fatal("sheet mirror should be enabled for local backend")
	}
	if cfg.LocalSheetPath != "tmp/test-sheet.json" {
		t.Fatalf("local sheet path = %q", cfg.LocalSheetPath)
	}
	if cfg.RabbitMQQueue != "spese.sheet-sync" {
		t.Fatalf("rabbit queue = %q, want spese.sheet-sync", cfg.RabbitMQQueue)
	}
}

func TestLoadAutoBackendUsesGoogleWhenCredentialsArePresent(t *testing.T) {
	clearEnv(t)
	t.Setenv("GOOGLE_SPREADSHEET_ID", "sheet-id")
	t.Setenv("GOOGLE_SERVICE_ACCOUNT_FILE", "/tmp/service-account.json")
	t.Setenv("SPESE_RABBITMQ_URL", "amqps://user:pass@example/vhost")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.ResolvedSheetMirrorBackend(); got != SheetMirrorBackendGoogle {
		t.Fatalf("backend = %q, want %q", got, SheetMirrorBackendGoogle)
	}
}

func TestLoadRejectsInvalidSheetMirrorBackend(t *testing.T) {
	clearEnv(t)
	t.Setenv("SPESE_SHEET_MIRROR_BACKEND", "bogus")

	if _, err := Load(); err == nil {
		t.Fatal("Load should reject invalid backend")
	}
}

func TestLoadDoesNotRequireRabbitWhenMirrorDisabled(t *testing.T) {
	clearEnv(t)
	t.Setenv("SPESE_SHEET_MIRROR_BACKEND", "none")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SheetMirrorEnabled() {
		t.Fatal("sheet mirror should be disabled")
	}
}

func TestLoadRequiresRabbitWhenMirrorEnabled(t *testing.T) {
	clearEnv(t)
	t.Setenv("SPESE_SHEET_MIRROR_BACKEND", "local")

	if _, err := Load(); err == nil {
		t.Fatal("Load should require RabbitMQ URL when mirror is enabled")
	}
}

func TestLoadUsesCloudAMQPURLFallback(t *testing.T) {
	clearEnv(t)
	t.Setenv("SPESE_SHEET_MIRROR_BACKEND", "local")
	t.Setenv("CLOUDAMQP_URL", "amqps://cloud.example/vhost")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.RabbitMQURL != "amqps://cloud.example/vhost" {
		t.Fatalf("rabbit url = %q", cfg.RabbitMQURL)
	}
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SPESE_PORT",
		"SPESE_DB_PATH",
		"GOOGLE_SPREADSHEET_ID",
		"GOOGLE_SERVICE_ACCOUNT_FILE",
		"SPESE_SHEET_MIRROR_BACKEND",
		"SPESE_LOCAL_SHEET_PATH",
		"SPESE_DISABLE_SHEET_MIRROR",
		"SPESE_RABBITMQ_URL",
		"SPESE_RABBITMQ_QUEUE",
		"CLOUDAMQP_URL",
		"AMQP_URL",
	} {
		t.Setenv(key, "")
	}
}
