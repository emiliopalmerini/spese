package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"spese/internal/backend"
	"spese/internal/config"
	apphttp "spese/internal/http"
	"spese/internal/log"
)

func main() {
	// Setup structured logging with our centralized logger
	appLogger := log.New(log.Config{
		Level:     slog.LevelInfo,
		Component: log.ComponentApp,
		Handler:   slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
	})
	
	// Set as default for backward compatibility
	log.SetDefault(appLogger)

	// Load configuration
	cfg := config.Load()
	
	// Validate configuration
	if err := cfg.Validate(); err != nil {
		appLogger.Error("Configuration validation failed", log.FieldError, err)
		os.Exit(1)
	}

	// Convert app config to backend config
	backendConfig, err := backend.FromAppConfig(cfg)
	if err != nil {
		appLogger.Error("Failed to convert backend configuration", log.FieldError, err)
		os.Exit(1)
	}

	// Validate backend configuration
	if err := backendConfig.Validate(); err != nil {
		appLogger.Error("Backend configuration validation failed", log.FieldError, err)
		os.Exit(1)
	}

	// Create backend factory
	factory := backend.NewFactory(appLogger.WithComponent(log.ComponentBackend).Logger)

	// Create backend
	backendResult, err := factory.CreateBackend(context.Background(), backendConfig)
	if err != nil {
		appLogger.Error("Failed to create backend", log.FieldError, err, "backend_type", backendConfig.Type)
		os.Exit(1)
	}

	appLogger.Info("Backend initialized successfully", "backend_type", backendConfig.Type)

	// Create HTTP server with logger
	httpLogger := appLogger.WithComponent(log.ComponentHTTP)
	srv := apphttp.NewServer(":"+cfg.Port, httpLogger, backendResult.Backend, backendResult.Backend, backendResult.Backend, backendResult.Backend)

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
		appLogger.Info("Shutdown signal received", "signal", sig.String(), log.FieldOperation, log.OpShutdown)

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		// Shutdown HTTP server
		if err := srv.Shutdown(shutdownCtx); err != nil {
			appLogger.Error("Server shutdown error", log.FieldError, err, log.FieldComponent, log.ComponentHTTP)
		}

		// Cleanup backend resources
		if backendResult.Cleanup != nil {
			if err := backendResult.Cleanup(); err != nil {
				appLogger.Error("Backend cleanup error", log.FieldError, err, log.FieldComponent, log.ComponentBackend)
			}
		}

		cancel()
	}()

	appLogger.Info("Starting spese server", 
		"port", cfg.Port, 
		"backend", backendConfig.Type,
		log.FieldOperation, log.OpStartup,
		log.FieldComponent, log.ComponentApp)
		
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		appLogger.Error("Server error", log.FieldError, err, "port", cfg.Port, log.FieldComponent, log.ComponentHTTP)
		os.Exit(1)
	}

	<-ctx.Done()
	appLogger.Info("Server stopped gracefully", log.FieldOperation, log.OpShutdown, log.FieldComponent, log.ComponentApp)
}