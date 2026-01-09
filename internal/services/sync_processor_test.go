package services

import (
	"context"
	"testing"
	"time"
)

func TestNewSyncProcessor(t *testing.T) {
	config := DefaultSyncProcessorConfig()
	processor := NewSyncProcessor(nil, nil, nil, config)

	if processor == nil {
		t.Error("NewSyncProcessor should return non-nil processor")
	}
	if processor.storage != nil {
		t.Error("storage should be nil when passed nil")
	}
	if processor.sheets != nil {
		t.Error("sheets should be nil when passed nil")
	}
	if processor.deleter != nil {
		t.Error("deleter should be nil when passed nil")
	}
}

func TestDefaultSyncProcessorConfig(t *testing.T) {
	config := DefaultSyncProcessorConfig()

	if config.PollInterval != 10*time.Second {
		t.Errorf("expected PollInterval 10s, got %v", config.PollInterval)
	}
	if config.BatchSize != 10 {
		t.Errorf("expected BatchSize 10, got %d", config.BatchSize)
	}
	if config.MaxRetries != 3 {
		t.Errorf("expected MaxRetries 3, got %d", config.MaxRetries)
	}
	if config.CleanupInterval != 1*time.Hour {
		t.Errorf("expected CleanupInterval 1h, got %v", config.CleanupInterval)
	}
	if config.CleanupAge != 24*time.Hour {
		t.Errorf("expected CleanupAge 24h, got %v", config.CleanupAge)
	}
}

func TestSyncProcessor_IsRunning(t *testing.T) {
	config := DefaultSyncProcessorConfig()
	processor := NewSyncProcessor(nil, nil, nil, config)

	if processor.IsRunning() {
		t.Error("processor should not be running initially")
	}
}

func TestSyncProcessor_StartTwice(t *testing.T) {
	config := DefaultSyncProcessorConfig()
	config.PollInterval = 100 * time.Millisecond
	processor := NewSyncProcessor(nil, nil, nil, config)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// First start should succeed (will fail on processing but that's ok)
	// We can't actually start without a real storage, so we just test the running state
	processor.mu.Lock()
	processor.running = true
	processor.mu.Unlock()

	// Second start should fail
	err := processor.Start(ctx)
	if err == nil {
		t.Error("expected error when starting already running processor")
	}
}

func TestSyncProcessor_StopNotRunning(t *testing.T) {
	config := DefaultSyncProcessorConfig()
	processor := NewSyncProcessor(nil, nil, nil, config)

	ctx := context.Background()

	// Stop when not running should not error
	err := processor.Stop(ctx)
	if err != nil {
		t.Errorf("Stop should not error when not running: %v", err)
	}
}

func TestSyncProcessorConfig_CustomValues(t *testing.T) {
	config := SyncProcessorConfig{
		PollInterval:    5 * time.Second,
		BatchSize:       20,
		MaxRetries:      5,
		CleanupInterval: 30 * time.Minute,
		CleanupAge:      12 * time.Hour,
	}

	processor := NewSyncProcessor(nil, nil, nil, config)

	if processor.config.PollInterval != 5*time.Second {
		t.Errorf("expected custom PollInterval 5s, got %v", processor.config.PollInterval)
	}
	if processor.config.BatchSize != 20 {
		t.Errorf("expected custom BatchSize 20, got %d", processor.config.BatchSize)
	}
	if processor.config.MaxRetries != 5 {
		t.Errorf("expected custom MaxRetries 5, got %d", processor.config.MaxRetries)
	}
	if processor.config.CleanupInterval != 30*time.Minute {
		t.Errorf("expected custom CleanupInterval 30m, got %v", processor.config.CleanupInterval)
	}
	if processor.config.CleanupAge != 12*time.Hour {
		t.Errorf("expected custom CleanupAge 12h, got %v", processor.config.CleanupAge)
	}
}
