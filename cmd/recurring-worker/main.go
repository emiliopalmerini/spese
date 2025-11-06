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
	"spese/internal/services"
	"spese/internal/storage"
)

func main() {
	// Load .env file for local development (ignore errors in production/docker)
	_ = godotenv.Load()

	// Setup structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	logger.Info("Starting recurring-worker")

	// Load configuration
	cfg := config.Load()

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Error("Configuration validation failed", "error", err)
		os.Exit(1)
	}

	// Initialize SQLite repository
	sqliteRepo, err := storage.NewSQLiteRepository(cfg.SQLiteDBPath)
	if err != nil {
		logger.Error("Failed to initialize SQLite repository", "error", err, "path", cfg.SQLiteDBPath)
		os.Exit(1)
	}
	defer sqliteRepo.Close()

	// Initialize AMQP client for publishing expense messages
	// The spese-worker will consume these and sync to Google Sheets
	var amqpClient *amqp.Client
	if cfg.AMQPURL != "" {
		amqpClient, err = amqp.NewClient(cfg.AMQPURL, cfg.AMQPExchange, cfg.AMQPQueue)
		if err != nil {
			logger.Warn("Failed to initialize AMQP client, continuing in SQLite-only mode", "error", err)
			amqpClient = nil
		} else {
			defer amqpClient.Close()
			logger.Info("AMQP client initialized - expenses will sync via spese-worker")
		}
	} else {
		logger.Info("AMQP disabled - expenses will not sync to Google Sheets")
	}

	// Initialize ExpenseService (creates expenses and publishes AMQP messages)
	expenseService := services.NewExpenseService(sqliteRepo, amqpClient)
	defer expenseService.Close()

	// Initialize RecurringProcessor
	processor := services.NewRecurringProcessor(sqliteRepo, expenseService)

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Get processing interval from config
	processingInterval := cfg.RecurringProcessorInterval
	logger.Info("Recurring expense processor configured",
		"interval", processingInterval,
		"sqlite_db", cfg.SQLiteDBPath)

	// Setup periodic processing ticker
	ticker := time.NewTicker(processingInterval)
	defer ticker.Stop()

	// Run initial processing on startup
	logger.Info("Running initial recurring expense processing...")
	if count, err := processor.ProcessDueExpenses(ctx, time.Now()); err != nil {
		logger.Error("Initial processing failed", "error", err)
	} else {
		logger.Info("Initial processing complete", "expenses_created", count)
	}

	// Start periodic processing
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				logger.Info("Processing due recurring expenses...")
				count, err := processor.ProcessDueExpenses(ctx, now)
				if err != nil {
					logger.Error("Periodic processing failed", "error", err)
				} else {
					logger.Info("Periodic processing complete",
						"expenses_created", count,
						"next_check", now.Add(processingInterval).Format("15:04:05"))
				}
			}
		}
	}()

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

	logger.Info("Shutting down recurring-worker...")
	cancel()

	// Wait for shutdown or timeout
	select {
	case <-shutdownCtx.Done():
		logger.Warn("Shutdown timeout reached")
	case <-time.After(2 * time.Second):
		logger.Info("Recurring-worker shutdown complete")
	}
}
