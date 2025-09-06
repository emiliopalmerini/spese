package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	// HTTP Server
	Port string
	
	// Database
	SQLiteDBPath string
	
	// AMQP
	AMQPURL      string
	AMQPExchange string
	AMQPQueue    string
	
	// Google Sheets (existing)
	GoogleSpreadsheetID   string
	GoogleSheetName       string
	GoogleOAuthClientFile string
	GoogleOAuthTokenFile  string
	GoogleOAuthClientJSON string
	GoogleOAuthTokenJSON  string
	
	// Worker
	SyncBatchSize int
	SyncInterval  time.Duration
	
	// Backend selection
	DataBackend string
}

func Load() *Config {
	cfg := &Config{
		Port:         getEnv("PORT", "8081"),
		SQLiteDBPath: getEnv("SQLITE_DB_PATH", "./data/spese.db"),
		
		AMQPURL:      getEnv("AMQP_URL", "amqp://guest:guest@localhost:5672/"),
		AMQPExchange: getEnv("AMQP_EXCHANGE", "spese"),
		AMQPQueue:    getEnv("AMQP_QUEUE", "sync_expenses"),
		
		GoogleSpreadsheetID:   getEnv("GOOGLE_SPREADSHEET_ID", ""),
		GoogleSheetName:       getEnv("GOOGLE_SHEET_NAME", ""),
		GoogleOAuthClientFile: getEnv("GOOGLE_OAUTH_CLIENT_FILE", ""),
		GoogleOAuthTokenFile:  getEnv("GOOGLE_OAUTH_TOKEN_FILE", ""),
		GoogleOAuthClientJSON: getEnv("GOOGLE_OAUTH_CLIENT_JSON", ""),
		GoogleOAuthTokenJSON:  getEnv("GOOGLE_OAUTH_TOKEN_JSON", ""),
		
		SyncBatchSize: getEnvInt("SYNC_BATCH_SIZE", 10),
		SyncInterval:  getEnvDuration("SYNC_INTERVAL", 30*time.Second),
		
		DataBackend: getEnv("DATA_BACKEND", "memory"),
	}
	
	return cfg
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}