package main

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"spese/internal/config"
	"spese/internal/features/accounts"
	"spese/internal/features/actions"
	"spese/internal/features/dashboard"
	"spese/internal/features/reports"
	"spese/internal/features/snapshots"
	"spese/internal/features/transactions"
	"spese/internal/features/transfers"
	"spese/internal/render"
	"spese/internal/sheetmirror"
	"spese/internal/sheets"
	"spese/internal/storage"
	"spese/web"
)

func main() {
	_ = godotenv.Load()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, err := storage.Open(ctx, cfg.DBPath, cfg.HonkerExtensionPath)
	if err != nil {
		logger.Error("storage", "err", err)
		os.Exit(1)
	}
	defer store.Close()

	if cfg.SpreadsheetID != "" && cfg.ServiceAccountFile != "" {
		client, err := sheets.New(ctx, cfg.ServiceAccountFile, cfg.SpreadsheetID)
		if err != nil {
			logger.Error("sheets client", "err", err)
			os.Exit(1)
		}
		mirror := &sheetmirror.Processor{Store: store, Client: client, Logger: logger}
		go func() {
			if err := mirror.Run(ctx); err != nil {
				logger.Error("sheet mirror", "err", err)
				cancel()
			}
		}()
	} else {
		logger.Info("sheet mirror disabled; GOOGLE_SPREADSHEET_ID or GOOGLE_SERVICE_ACCOUNT_FILE missing")
	}

	tmpl, err := render.Load(web.TemplatesFS)
	if err != nil {
		logger.Error("templates", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()

	(&dashboard.Handler{Store: store, Logger: logger, Render: tmpl}).Mount(mux, "/")
	(&actions.Handler{Store: store, Logger: logger, Render: tmpl}).Mount(mux, "/actions")
	(&accounts.Handler{Store: store, Logger: logger, Render: tmpl}).Mount(mux, "/accounts")
	(&transactions.Handler{Store: store, Logger: logger, Render: tmpl}).Mount(mux, "/transactions")
	(&transfers.Handler{Store: store, Logger: logger}).Mount(mux, "/transfers")
	(&snapshots.Handler{Store: store, Logger: logger}).Mount(mux, "/snapshots")
	(&reports.Handler{Store: store, Logger: logger, Render: tmpl}).Mount(mux, "/reports")

	staticFS, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		logger.Error("static fs", "err", err)
		os.Exit(1)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// HTTP server lifecycle.
	go func() {
		logger.Info("listening", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen", "err", err)
			cancel()
		}
	}()

	// Wait for signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	logger.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "err", err)
	}
	cancel()
}
