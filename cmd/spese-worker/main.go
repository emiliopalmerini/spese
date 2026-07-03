package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/joho/godotenv"
	amqp "github.com/rabbitmq/amqp091-go"

	"spese/internal/config"
	"spese/internal/rabbitmq"
	"spese/internal/sheetmirror"
	"spese/internal/sheets"
	"spese/internal/storage"
)

const defaultSheetsWriteRatePerMinute = 10

type workerMode string

const (
	workerModeDaemon workerMode = "daemon"
	workerModeOnce   workerMode = "once"
)

type workerRuntimeConfig struct {
	Mode                     workerMode
	SheetsWriteRatePerMinute int
}

func main() {
	_ = godotenv.Load()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("config", "err", err)
		os.Exit(1)
	}
	runtimeCfg, err := loadWorkerRuntimeConfig()
	if err != nil {
		logger.Error("worker config", "err", err)
		os.Exit(1)
	}
	if !cfg.SheetMirrorEnabled() {
		logger.Info("sheet mirror disabled", "backend", cfg.ResolvedSheetMirrorBackend())
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := storage.Open(ctx, cfg.DBPath)
	if err != nil {
		logger.Error("storage", "err", err)
		os.Exit(1)
	}
	defer store.Close()

	writer, backend, err := newSheetWriter(ctx, cfg, runtimeCfg)
	if err != nil {
		logger.Error("sheet writer", "err", err)
		os.Exit(1)
	}

	processor := &sheetmirror.Processor{Store: store, Client: writer, Logger: logger}
	consumer := &rabbitmq.Consumer{URL: cfg.RabbitMQURL, Queue: cfg.RabbitMQQueue, Logger: logger}
	handler := func(ctx context.Context, delivery amqp.Delivery) error {
		var payload storage.SheetSyncPayload
		if err := json.Unmarshal(delivery.Body, &payload); err != nil {
			return fmt.Errorf("decode sheet sync payload: %w", err)
		}
		if payload.Version != 1 {
			return fmt.Errorf("unsupported sheet sync payload version: %d", payload.Version)
		}
		if err := processor.Export(ctx); err != nil {
			return err
		}
		logger.Info(
			"sheet sync consumed",
			"message_id",
			delivery.MessageId,
			"outbox_id",
			payload.OutboxID,
			"scope",
			payload.Scope,
		)
		return nil
	}

	logger.Info(
		"sheet sync worker enabled",
		"backend",
		backend,
		"queue",
		cfg.RabbitMQQueue,
		"mode",
		runtimeCfg.Mode,
		"sheets_write_rate_per_minute",
		runtimeCfg.SheetsWriteRatePerMinute,
	)
	switch runtimeCfg.Mode {
	case workerModeOnce:
		count, err := consumer.RunAvailable(ctx, 0, handler)
		if err != nil {
			logger.Error("sheet sync worker", "err", err)
			os.Exit(1)
		}
		if count == 0 {
			logger.Info("sheet sync queue empty")
		} else {
			logger.Info("sheet sync run complete", "messages", count)
		}
	case workerModeDaemon:
		if err := consumer.Run(ctx, handler); err != nil {
			logger.Error("sheet sync worker", "err", err)
			os.Exit(1)
		}
	}
}

func loadWorkerRuntimeConfig() (workerRuntimeConfig, error) {
	mode := workerMode(strings.ToLower(strings.TrimSpace(os.Getenv("SPESE_WORKER_MODE"))))
	if mode == "" {
		mode = workerModeDaemon
	}
	switch mode {
	case workerModeDaemon, workerModeOnce:
	default:
		return workerRuntimeConfig{}, fmt.Errorf("SPESE_WORKER_MODE must be one of daemon, once")
	}

	rate := defaultSheetsWriteRatePerMinute
	if raw := strings.TrimSpace(os.Getenv("SPESE_SHEETS_WRITE_RATE_PER_MINUTE")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return workerRuntimeConfig{}, fmt.Errorf("parse SPESE_SHEETS_WRITE_RATE_PER_MINUTE: %w", err)
		}
		if parsed < 0 {
			return workerRuntimeConfig{}, fmt.Errorf("SPESE_SHEETS_WRITE_RATE_PER_MINUTE must be >= 0")
		}
		rate = parsed
	}

	return workerRuntimeConfig{Mode: mode, SheetsWriteRatePerMinute: rate}, nil
}

func newSheetWriter(ctx context.Context, cfg config.Config, runtimeCfg workerRuntimeConfig) (sheetmirror.SheetWriter, string, error) {
	switch cfg.ResolvedSheetMirrorBackend() {
	case config.SheetMirrorBackendGoogle:
		client, err := sheets.New(ctx, cfg.ServiceAccountFile, cfg.SpreadsheetID)
		if err != nil {
			return nil, "", err
		}
		limiter, err := sheets.NewWriteRateLimiter(runtimeCfg.SheetsWriteRatePerMinute)
		if err != nil {
			return nil, "", err
		}
		client.SetWriteLimiter(limiter)
		return client, string(config.SheetMirrorBackendGoogle), nil
	case config.SheetMirrorBackendLocal:
		client, err := sheets.NewFileClient(cfg.LocalSheetPath)
		if err != nil {
			return nil, "", err
		}
		return client, string(config.SheetMirrorBackendLocal), nil
	default:
		return nil, "", fmt.Errorf("unsupported sheet mirror backend %q", cfg.ResolvedSheetMirrorBackend())
	}
}
