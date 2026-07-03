package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	amqp "github.com/rabbitmq/amqp091-go"

	"spese/internal/config"
	"spese/internal/rabbitmq"
	"spese/internal/sheetmirror"
	"spese/internal/sheets"
	"spese/internal/storage"
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

	writer, backend, err := newSheetWriter(ctx, cfg)
	if err != nil {
		logger.Error("sheet writer", "err", err)
		os.Exit(1)
	}

	processor := &sheetmirror.Processor{Store: store, Client: writer, Logger: logger}
	consumer := &rabbitmq.Consumer{URL: cfg.RabbitMQURL, Queue: cfg.RabbitMQQueue, Logger: logger}
	logger.Info("sheet sync worker enabled", "backend", backend, "queue", cfg.RabbitMQQueue)
	if err := consumer.Run(ctx, func(ctx context.Context, delivery amqp.Delivery) error {
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
	}); err != nil {
		logger.Error("sheet sync worker", "err", err)
		os.Exit(1)
	}
}

func newSheetWriter(ctx context.Context, cfg config.Config) (sheetmirror.SheetWriter, string, error) {
	switch cfg.ResolvedSheetMirrorBackend() {
	case config.SheetMirrorBackendGoogle:
		client, err := sheets.New(ctx, cfg.ServiceAccountFile, cfg.SpreadsheetID)
		if err != nil {
			return nil, "", err
		}
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
