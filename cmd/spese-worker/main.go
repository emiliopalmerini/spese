package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"spese/internal/amqp"
	"spese/internal/config"
	gsheet "spese/internal/sheets/google"
	"spese/internal/storage"
	"spese/internal/worker"
)

func main() {
	// Load .env file for local development (ignore errors in production/docker)
	_ = godotenv.Load()

	// Setup structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.Info("Starting spese-worker")

	// Load configuration
	cfg := config.Load()

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Error("Configuration validation failed", "error", err)
		os.Exit(1)
	}

	// Initialize SQLite repository to read pending expenses
	sqliteRepo, err := storage.NewSQLiteRepository(cfg.SQLiteDBPath)
	if err != nil {
		logger.Error("Failed to initialize SQLite repository", "error", err, "path", cfg.SQLiteDBPath)
		os.Exit(1)
	}
	defer sqliteRepo.Close()

	// Initialize Google Sheets client for sync operations (optional)
	var sheetsClient *gsheet.Client
	if cfg.GoogleSpreadsheetID != "" {
		var err error
		sheetsClient, err = gsheet.NewFromEnv(context.Background())
		if err != nil {
			logger.Error("Failed to initialize Google Sheets client", "error", err)
			os.Exit(1)
		}
		logger.Info("Google Sheets client initialized", "spreadsheet_id", cfg.GoogleSpreadsheetID)
	} else {
		logger.Info("Google Sheets disabled - no GOOGLE_SPREADSHEET_ID provided")
	}

	// Initialize AMQP client for consuming messages
	amqpClient, err := amqp.NewClient(cfg.AMQPURL, cfg.AMQPExchange, cfg.AMQPQueue)
	if err != nil {
		logger.Error("Failed to initialize AMQP client", "error", err)
		os.Exit(1)
	}
	defer amqpClient.Close()

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var syncWorker *worker.SyncWorker
	if sheetsClient != nil {
		// Create sync worker only if Google Sheets is available
		syncWorker = worker.NewSyncWorker(sqliteRepo, sheetsClient, sheetsClient, cfg.SyncBatchSize)

		// On startup, sync categories from Google Sheets if database is empty
		logger.Info("Checking category cache...")
		if err := syncWorker.SyncCategoriesIfNeeded(ctx); err != nil {
			logger.Error("Failed to sync categories", "error", err)
			// Don't exit - continue with normal operation
		}

		// On startup, process any pending expenses that might have been missed
		logger.Info("Performing startup sync check...")
		if err := syncWorker.StartupSyncCheck(ctx); err != nil {
			logger.Error("Failed startup sync check", "error", err)
			// Don't exit - continue with normal operation
		}
	} else {
		logger.Info("Skipping Google Sheets sync operations - no client available")
	}

	// Start message consumption only if we have a sync worker
	if syncWorker != nil {
		go func() {
			if err := amqpClient.ConsumeMessages(ctx, syncWorker.HandleSyncMessage, syncWorker.HandleDeleteMessage); err != nil {
				if err != context.Canceled {
					logger.Error("Message consumption failed", "error", err)
				}
				cancel()
			}
		}()
	} else {
		logger.Info("Skipping AMQP message consumption - no sync worker available")
	}

	// Setup periodic sync for any missed messages (only if sync worker is available)
	if syncWorker != nil {
		ticker := time.NewTicker(cfg.SyncInterval)
		defer ticker.Stop()

		// Setup daily category refresh (check once per day)
		categoryTicker := time.NewTicker(24 * time.Hour)
		defer categoryTicker.Stop()

		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := syncWorker.ProcessPendingExpenses(ctx); err != nil {
						logger.Error("Periodic sync failed", "error", err)
					}
				case <-categoryTicker.C:
					// Periodic category refresh (respects cache age)
					if err := syncWorker.PeriodicCategoryRefresh(ctx); err != nil {
						logger.Error("Periodic category refresh failed", "error", err)
					}
				}
			}
		}()
	} else {
		logger.Info("Skipping periodic sync operations - no sync worker available")
	}

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		logger.Info("Shutdown signal received", "signal", sig.String())
	case <-ctx.Done():
		logger.Info("Context cancelled")
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Give worker time to finish current operations
	logger.Info("Shutting down worker...")
	cancel()

	// Wait for shutdown or timeout
	select {
	case <-shutdownCtx.Done():
		logger.Warn("Shutdown timeout reached")
	case <-time.After(5 * time.Second):
		logger.Info("Worker shutdown complete")
	}
}
