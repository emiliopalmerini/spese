// Package cli provides common CLI initialization utilities.
// This package consolidates repeated initialization patterns across
// cmd/spese, cmd/spese-worker, and cmd/recurring-worker.
package cli

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"spese/internal/config"
	"spese/internal/storage"
)

// SetupLogger initializes structured logging with default settings.
// Returns the configured logger and sets it as the default logger.
func SetupLogger() *slog.Logger {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
	return logger
}

// LoadEnvFile loads the .env file for local development.
// Errors are ignored silently as this is optional in production.
func LoadEnvFile() {
	_ = godotenv.Load()
}

// LoadAndValidateConfig loads configuration and validates it.
// Returns the config or exits the process on validation failure.
func LoadAndValidateConfig(logger *slog.Logger) *config.Config {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		logger.Error("Configuration validation failed", "error", err)
		os.Exit(1)
	}
	return cfg
}

// InitSQLite initializes a SQLite repository with the given path.
// Returns the repository or exits the process on failure.
func InitSQLite(logger *slog.Logger, dbPath string) *storage.SQLiteRepository {
	sqliteRepo, err := storage.NewSQLiteRepository(dbPath)
	if err != nil {
		logger.Error("Failed to initialize SQLite repository", "error", err, "path", dbPath)
		os.Exit(1)
	}
	return sqliteRepo
}

// GracefulShutdown sets up signal handling for graceful shutdown.
// Returns a context that will be cancelled on shutdown signals,
// and a channel that signals when shutdown is complete.
func GracefulShutdown(logger *slog.Logger, timeout time.Duration, cleanup func()) (context.Context, <-chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigChan
		logger.Info("Shutdown signal received", "signal", sig.String())

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), timeout)
		defer shutdownCancel()

		if cleanup != nil {
			cleanup()
		}

		cancel()

		select {
		case <-shutdownCtx.Done():
			logger.Warn("Shutdown timeout reached")
		case <-time.After(2 * time.Second):
			logger.Info("Shutdown complete")
		}
		close(done)
	}()

	return ctx, done
}

// WaitForShutdown blocks until the context is cancelled.
func WaitForShutdown(ctx context.Context, done <-chan struct{}) {
	<-ctx.Done()
	<-done
}
