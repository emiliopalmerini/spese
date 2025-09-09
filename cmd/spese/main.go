package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"spese/internal/adapters"
	"spese/internal/amqp"
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
		expWriter  ports.ExpenseWriter
		taxReader  ports.TaxonomyReader
		dashReader ports.DashboardReader
		expLister  ports.ExpenseLister
		cleanup    func() error
	)

	switch cfg.DataBackend {
	case "sqlite":
		// Initialize SQLite repository
		sqliteRepo, err := storage.NewSQLiteRepository(cfg.SQLiteDBPath)
		if err != nil {
			logger.Error("Failed to initialize SQLite repository", "error", err, "path", cfg.SQLiteDBPath)
			os.Exit(1)
		}

		// Initialize AMQP client (optional, can be nil)
		var amqpClient *amqp.Client
		if cfg.AMQPURL != "" {
			amqpClient, err = amqp.NewClient(cfg.AMQPURL, cfg.AMQPExchange, cfg.AMQPQueue)
			if err != nil {
				logger.Warn("Failed to initialize AMQP client, continuing without sync", "error", err)
			} else {
				logger.Info("Initialized AMQP client", "exchange", cfg.AMQPExchange, "queue", cfg.AMQPQueue)
			}
		}

		// Create expense service and adapter
		expenseService := services.NewExpenseService(sqliteRepo, amqpClient)
		adapter := adapters.NewSQLiteAdapter(sqliteRepo, expenseService)

		expWriter, taxReader, dashReader, expLister = adapter, adapter, adapter, adapter
		cleanup = expenseService.Close

		logger.Info("Initialized SQLite backend", "db_path", cfg.SQLiteDBPath, "amqp_enabled", amqpClient != nil)

	case "sheets":
		cli, err := gsheet.NewFromEnv(context.Background())
		if err != nil {
			logger.Error("Failed to initialize Google Sheets client", "error", err)
			os.Exit(1)
		}
		expWriter, taxReader, dashReader, expLister = cli, cli, cli, cli
		logger.Info("Initialized Google Sheets backend")

	default:
		logger.Error("Unsupported data backend", "backend", cfg.DataBackend)
		os.Exit(1)
	}

	srv := apphttp.NewServer(":"+cfg.Port, expWriter, taxReader, dashReader, expLister)

	// Configure server timeouts and limits
	srv.ReadTimeout = 10 * time.Second
	srv.WriteTimeout = 10 * time.Second
	srv.IdleTimeout = 60 * time.Second
	srv.MaxHeaderBytes = 1 << 16 // 64KB

	// Graceful shutdown handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigChan
		logger.Info("Shutdown signal received", "signal", sig.String())

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		// Shutdown HTTP server
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("Server shutdown error", "error", err)
		}

		// Cleanup resources
		if cleanup != nil {
			if err := cleanup(); err != nil {
				logger.Error("Cleanup error", "error", err)
			}
		}

		cancel()
	}()

	logger.Info("Starting spese server", "port", cfg.Port, "backend", cfg.DataBackend)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("Server error", "error", err, "port", cfg.Port)
		os.Exit(1)
	}

	<-ctx.Done()
	logger.Info("Server stopped gracefully")
}
