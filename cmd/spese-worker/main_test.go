package main

import "testing"

func TestLoadWorkerRuntimeConfigDefaultsToDaemonAndTenWritesPerMinute(t *testing.T) {
	t.Setenv("SPESE_WORKER_MODE", "")
	t.Setenv("SPESE_SHEETS_WRITE_RATE_PER_MINUTE", "")

	cfg, err := loadWorkerRuntimeConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != workerModeDaemon {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, workerModeDaemon)
	}
	if cfg.SheetsWriteRatePerMinute != 10 {
		t.Fatalf("SheetsWriteRatePerMinute = %d, want 10", cfg.SheetsWriteRatePerMinute)
	}
}

func TestLoadWorkerRuntimeConfigAcceptsOnceAndCustomRate(t *testing.T) {
	t.Setenv("SPESE_WORKER_MODE", "once")
	t.Setenv("SPESE_SHEETS_WRITE_RATE_PER_MINUTE", "5")

	cfg, err := loadWorkerRuntimeConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mode != workerModeOnce {
		t.Fatalf("Mode = %q, want %q", cfg.Mode, workerModeOnce)
	}
	if cfg.SheetsWriteRatePerMinute != 5 {
		t.Fatalf("SheetsWriteRatePerMinute = %d, want 5", cfg.SheetsWriteRatePerMinute)
	}
}

func TestLoadWorkerRuntimeConfigRejectsInvalidValues(t *testing.T) {
	t.Setenv("SPESE_WORKER_MODE", "sometimes")
	if _, err := loadWorkerRuntimeConfig(); err == nil {
		t.Fatal("invalid mode error = nil")
	}

	t.Setenv("SPESE_WORKER_MODE", "once")
	t.Setenv("SPESE_SHEETS_WRITE_RATE_PER_MINUTE", "-1")
	if _, err := loadWorkerRuntimeConfig(); err == nil {
		t.Fatal("negative rate error = nil")
	}
}
