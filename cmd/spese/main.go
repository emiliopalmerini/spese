package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/emiliopalmerini/elevenlabs-go/elevenlabs"
	"github.com/google/uuid"
	"github.com/joho/godotenv"

	"spese/internal/api"
	"spese/internal/config"
	"spese/internal/features/dictation"
	"spese/internal/features/ledger"
	"spese/internal/rabbitmq"
	"spese/internal/sheetmirror"
	"spese/internal/spa"
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
	store, err := storage.Open(ctx, cfg.DBPath)
	if err != nil {
		logger.Error("storage", "err", err)
		os.Exit(1)
	}
	defer store.Close()

	store.SetSheetSyncEnabled(cfg.SheetMirrorEnabled())
	if cfg.SheetMirrorEnabled() {
		if err := store.EnqueueSheetSyncEvent(ctx, storage.SheetSyncBootstrapScope); err != nil {
			logger.Error("bootstrap sheet sync", "err", err)
			os.Exit(1)
		}
		startSheetSyncRelay(ctx, cfg, store, logger)
	} else {
		logger.Info("sheet mirror disabled", "backend", cfg.ResolvedSheetMirrorBackend())
	}

	ledgerService := ledger.New(store)
	if _, err := ledgerService.ProcessRecurring(ctx, time.Now()); err != nil {
		logger.Error("initial recurring catch-up", "err", err)
		os.Exit(1)
	}
	startRecurringScheduler(ctx, ledgerService, logger)

	spaHandler, err := spa.New(web.AppFS)
	if err != nil {
		logger.Error("embedded frontend", "err", err)
		os.Exit(1)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := store.DB().PingContext(ctx); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ready\n"))
	})
	mux.Handle("/api/", api.New(store, ledgerService))

	if cfg.DictationEnabled {
		elevenClient, err := elevenlabs.NewClient(
			elevenlabs.WithAuthToken(cfg.ElevenLabsAPIKey),
			elevenlabs.WithTimeout(60*time.Second),
		)
		if err != nil {
			logger.Error("elevenlabs client", "err", err)
			os.Exit(1)
		}
		openCodeClient := dictation.NewOpenCodeClient(dictation.OpenCodeConfig{
			BaseURL: cfg.OpenCodeURL, Username: cfg.OpenCodeUsername, Password: cfg.OpenCodePassword,
			ProviderID: cfg.OpenCodeProvider, ModelID: cfg.OpenCodeModel, Agent: cfg.OpenCodeAgent,
		}, &http.Client{Timeout: 60 * time.Second})
		dictation.NewHandler(store, openCodeClient, dictation.NewElevenLabsTranscriber(elevenClient), logger).Mount(mux, "/api/v1/dictation")
		logger.Info("movement dictation enabled", "model", cfg.OpenCodeProvider+"/"+cfg.OpenCodeModel)
	}
	mux.Handle("/", spaHandler)

	srv := &http.Server{
		Addr:              net.JoinHostPort(cfg.Host, cfg.Port),
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("listening", "address", srv.Addr)
		serverErr <- srv.ListenAndServe()
	}()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case signal := <-sigCh:
		logger.Info("shutting down", "signal", signal.String())
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen", "err", err)
			os.Exit(1)
		}
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "err", err)
	}
	cancel()
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if requestID == "" || len(requestID) > 100 {
			requestID = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", requestID)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(self)")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self' ws: wss:; font-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		if isMutation(r.Method) && strings.HasPrefix(r.URL.Path, "/api/v1/dictation/") && !sameOrigin(r) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isMutation(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete
}

func sameOrigin(r *http.Request) bool {
	if r.Header.Get("X-Spese-CSRF") != "1" {
		return false
	}
	origin, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && origin.Host != "" && strings.EqualFold(origin.Host, r.Host) && (origin.Scheme == "http" || origin.Scheme == "https")
}

func startRecurringScheduler(ctx context.Context, service *ledger.Service, logger *slog.Logger) {
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				created, err := service.ProcessRecurring(ctx, now)
				if err != nil {
					logger.Error("process recurring", "err", err)
				} else if created > 0 {
					logger.Info("recurring generated", "occurrences", created)
				}
			}
		}
	}()
}

func startSheetSyncRelay(ctx context.Context, cfg config.Config, store *storage.Store, logger *slog.Logger) {
	logger.Info("sheet sync publisher enabled", "backend", cfg.ResolvedSheetMirrorBackend(), "queue", cfg.RabbitMQQueue)
	publisher := rabbitmq.NewPublisher(cfg.RabbitMQURL, cfg.RabbitMQQueue, logger)
	relay := &sheetmirror.Relay{Store: store, Publisher: publisher, Logger: logger}
	go func() {
		if err := relay.Run(ctx); err != nil {
			logger.Error("sheet sync relay", "err", err)
		}
	}()
}
