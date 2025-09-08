package backend

import (
	"context"
	"fmt"
	"log/slog"

	"spese/internal/adapters"
	"spese/internal/amqp"
	"spese/internal/services"
	gsheet "spese/internal/sheets/google"
	"spese/internal/sheets/memory"
	"spese/internal/storage"
)

// DefaultFactory implements the Factory interface
type DefaultFactory struct {
	logger *slog.Logger
}

// NewFactory creates a new backend factory
func NewFactory(logger *slog.Logger) Factory {
	if logger == nil {
		logger = slog.Default()
	}
	return &DefaultFactory{
		logger: logger,
	}
}

// CreateBackend implements Factory.CreateBackend
func (f *DefaultFactory) CreateBackend(ctx context.Context, config Config) (*BackendResult, error) {
	if !config.Type.IsValid() {
		return nil, fmt.Errorf("invalid backend type: %s", config.Type)
	}

	switch config.Type {
	case SQLiteBackend:
		return f.createSQLiteBackend(config)
	case SheetsBackend:
		return f.createSheetsBackend(ctx, config)
	case MemoryBackend:
		return f.createMemoryBackend(config)
	default:
		return nil, fmt.Errorf("unsupported backend type: %s", config.Type)
	}
}

func (f *DefaultFactory) createSQLiteBackend(config Config) (*BackendResult, error) {
	// Initialize SQLite repository
	sqliteRepo, err := storage.NewSQLiteRepository(config.SQLiteDBPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SQLite repository: %w", err)
	}

	// Initialize AMQP client (optional)
	var amqpClient *amqp.Client
	if config.AMQPURL != "" {
		amqpClient, err = amqp.NewClient(config.AMQPURL, config.AMQPExchange, config.AMQPQueue)
		if err != nil {
			f.logger.Warn("Failed to initialize AMQP client, continuing without sync", "error", err)
		} else {
			f.logger.Info("Initialized AMQP client", 
				"exchange", config.AMQPExchange, 
				"queue", config.AMQPQueue)
		}
	}

	// Create expense service and adapter
	expenseService := services.NewExpenseService(sqliteRepo, amqpClient)
	adapter := adapters.NewSQLiteAdapter(sqliteRepo, expenseService)

	f.logger.Info("Initialized SQLite backend", 
		"db_path", config.SQLiteDBPath, 
		"amqp_enabled", amqpClient != nil)

	return &BackendResult{
		Backend: adapter,
		Cleanup: expenseService.Close,
	}, nil
}

func (f *DefaultFactory) createSheetsBackend(ctx context.Context, config Config) (*BackendResult, error) {
	cli, err := gsheet.NewFromEnv(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Google Sheets client: %w", err)
	}

	f.logger.Info("Initialized Google Sheets backend")

	return &BackendResult{
		Backend: cli,
		Cleanup: nil, // No cleanup needed for sheets backend
	}, nil
}

func (f *DefaultFactory) createMemoryBackend(config Config) (*BackendResult, error) {
	dataDir := config.DataDirectory
	if dataDir == "" {
		dataDir = "data" // Default directory
	}
	
	store := memory.NewFromFiles(dataDir)

	f.logger.Info("Initialized memory backend", "data_directory", dataDir)

	return &BackendResult{
		Backend: store,
		Cleanup: nil, // No cleanup needed for memory backend
	}, nil
}

// ConfigFromAppConfig converts application config to backend config
func ConfigFromAppConfig(appConfig interface{}) (Config, error) {
	// This is a helper to convert from the main app config
	// We'll implement this based on the actual config structure
	
	// For now, return a basic structure that can be extended
	return Config{}, fmt.Errorf("config conversion not implemented yet")
}