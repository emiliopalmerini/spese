package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	apphttp "spese/internal/http"
	ports "spese/internal/sheets"
	gsheet "spese/internal/sheets/google"
	mem "spese/internal/sheets/memory"
	"syscall"
	"time"
)

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	// Choose data backend (default: memory). Seed from ./data if present.
	backend := os.Getenv("DATA_BACKEND")
	if backend == "" {
		backend = "memory"
	}

    var (
        expWriter  ports.ExpenseWriter
        taxReader  ports.TaxonomyReader
        dashReader ports.DashboardReader
        expLister  ports.ExpenseLister
    )

	switch backend {
	case "sheets":
		cli, err := gsheet.NewFromEnv(context.Background())
		if err != nil {
			logger.Error("Failed to initialize Google Sheets client", "error", err, "backend", backend)
			os.Exit(1)
		}
        expWriter, taxReader, dashReader, expLister = cli, cli, cli, cli
		logger.Info("Initialized Google Sheets backend", "backend", backend)
	default:
		store := mem.NewFromFiles("data")
        expWriter, taxReader, dashReader, expLister = store, store, store, store
		logger.Info("Initialized memory backend", "backend", backend)
	}

    srv := apphttp.NewServer(":"+port, expWriter, taxReader, dashReader, expLister)

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

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("Server shutdown error", "error", err)
		}
		cancel()
	}()

	logger.Info("Starting spese server", "port", port, "backend", backend)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("Server error", "error", err, "port", port)
		os.Exit(1)
	}

	<-ctx.Done()
	logger.Info("Server stopped gracefully")
}
