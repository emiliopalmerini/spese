package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/joho/godotenv"
	"spese/internal/adapters"
	"spese/internal/config"
	apphttp "spese/internal/http"
	"spese/internal/services"
	ports "spese/internal/sheets"
	gsheet "spese/internal/sheets/google"
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

	// Load configuration
	cfg := config.Load()

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		logger.Error("Configuration validation failed", "error", err)
		os.Exit(1)
	}

	var (
		expWriter       ports.ExpenseWriter
		taxReader       ports.TaxonomyReader
		dashReader      ports.DashboardReader
		expLister       ports.ExpenseLister
		expDeleter      ports.ExpenseDeleter
		expListerWithID ports.ExpenseListerWithID
		sqliteRepo      *storage.SQLiteRepository
		expenseService  *services.ExpenseService
		sheetsClient    *gsheet.Client
	)

	switch cfg.DataBackend {
	case "sqlite":
		// Initialize SQLite repository
		var err error
		sqliteRepo, err = storage.NewSQLiteRepository(cfg.SQLiteDBPath)
		if err != nil {
			logger.Error("Failed to initialize SQLite repository", "error", err, "path", cfg.SQLiteDBPath)
			os.Exit(1)
		}

		// Create expense service (no longer needs AMQP - uses sync queue)
		expenseService = services.NewExpenseService(sqliteRepo)
		adapter := adapters.NewSQLiteAdapter(sqliteRepo, expenseService)

		expWriter, taxReader, dashReader, expLister, expDeleter, expListerWithID = adapter, adapter, adapter, adapter, adapter, adapter

		// Initialize Google Sheets client for sync processor (optional)
		sheetsClient, err = gsheet.NewFromEnv(context.Background())
		if err != nil {
			logger.Warn("Google Sheets client not available, sync processor will be disabled", "error", err)
		}

		logger.Info("Initialized SQLite backend", "db_path", cfg.SQLiteDBPath, "sheets_sync_enabled", sheetsClient != nil)

	case "sheets":
		var err error
		sheetsClient, err = gsheet.NewFromEnv(context.Background())
		if err != nil {
			logger.Error("Failed to initialize Google Sheets client", "error", err)
			os.Exit(1)
		}
		expWriter, taxReader, dashReader, expLister, expDeleter = sheetsClient, sheetsClient, sheetsClient, sheetsClient, sheetsClient
		expListerWithID = nil // Google Sheets backend doesn't support listing with IDs yet
		logger.Info("Initialized Google Sheets backend")

	default:
		logger.Error("Unsupported data backend", "backend", cfg.DataBackend)
		os.Exit(1)
	}

	srv := apphttp.NewServer(":"+cfg.Port, expWriter, taxReader, dashReader, expLister, expDeleter, expListerWithID)

	// Configure server timeouts and limits
	srv.ReadTimeout = 10 * time.Second
	srv.WriteTimeout = 10 * time.Second
	srv.IdleTimeout = 60 * time.Second
	srv.MaxHeaderBytes = 1 << 16 // 64KB

	// Create context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create errgroup for managing goroutines
	g, gCtx := errgroup.WithContext(ctx)

	// Handle shutdown signals
	g.Go(func() error {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

		select {
		case sig := <-sigChan:
			logger.Info("Shutdown signal received", "signal", sig.String())
			cancel()
			return nil
		case <-gCtx.Done():
			return nil
		}
	})

	// Start HTTP server
	g.Go(func() error {
		logger.Info("Starting HTTP server", "port", cfg.Port, "backend", cfg.DataBackend)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})

	// Graceful shutdown of HTTP server when context is cancelled
	g.Go(func() error {
		<-gCtx.Done()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		logger.Info("Shutting down HTTP server")
		return srv.Shutdown(shutdownCtx)
	})

	// Start SyncProcessor (SQLite backend with Google Sheets client)
	var syncProcessor *services.SyncProcessor
	if cfg.DataBackend == "sqlite" && sheetsClient != nil && sqliteRepo != nil {
		syncConfig := services.SyncProcessorConfig{
			PollInterval:    cfg.SyncInterval,
			BatchSize:       cfg.SyncBatchSize,
			MaxRetries:      3,
			CleanupInterval: 1 * time.Hour,
			CleanupAge:      24 * time.Hour,
		}
		syncProcessor = services.NewSyncProcessor(sqliteRepo, sheetsClient, sheetsClient, syncConfig)

		g.Go(func() error {
			logger.Info("Starting sync processor",
				"poll_interval", cfg.SyncInterval,
				"batch_size", cfg.SyncBatchSize)
			return syncProcessor.Start(gCtx)
		})

		// Graceful shutdown of sync processor
		g.Go(func() error {
			<-gCtx.Done()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer shutdownCancel()

			logger.Info("Stopping sync processor")
			return syncProcessor.Stop(shutdownCtx)
		})
	}

	// Start RecurringProcessor (SQLite backend only)
	if cfg.DataBackend == "sqlite" && sqliteRepo != nil && expenseService != nil {
		recurringProcessor := services.NewRecurringProcessor(sqliteRepo, expenseService)

		g.Go(func() error {
			ticker := time.NewTicker(cfg.RecurringProcessorInterval)
			defer ticker.Stop()

			logger.Info("Starting recurring processor", "interval", cfg.RecurringProcessorInterval)

			// Process immediately on startup
			if count, err := recurringProcessor.ProcessDueExpenses(gCtx, time.Now()); err != nil {
				logger.Error("Failed to process recurring expenses on startup", "error", err)
			} else if count > 0 {
				logger.Info("Processed recurring expenses on startup", "count", count)
			}

			for {
				select {
				case <-gCtx.Done():
					logger.Info("Stopping recurring processor")
					return nil
				case <-ticker.C:
					if count, err := recurringProcessor.ProcessDueExpenses(gCtx, time.Now()); err != nil {
						logger.Error("Failed to process recurring expenses", "error", err)
					} else if count > 0 {
						logger.Info("Processed recurring expenses", "count", count)
					}
				}
			}
		})
	}

	// Wait for all goroutines to complete
	if err := g.Wait(); err != nil {
		logger.Error("Error during shutdown", "error", err)
	}

	// Cleanup resources
	if expenseService != nil {
		if err := expenseService.Close(); err != nil {
			logger.Error("Failed to close expense service", "error", err)
		}
	}

	logger.Info("Server stopped gracefully")
}
