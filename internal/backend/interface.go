package backend

import (
	"context"
	"spese/internal/sheets"
)

// Backend represents a unified backend interface that provides all necessary operations
type Backend interface {
	sheets.ExpenseWriter
	sheets.TaxonomyReader  
	sheets.DashboardReader
	sheets.ExpenseLister
}

// CleanupFunc represents a cleanup function for resources
type CleanupFunc func() error

// BackendResult contains the backend instance and optional cleanup function
type BackendResult struct {
	Backend Backend
	Cleanup CleanupFunc
}

// Factory creates backends based on configuration
type Factory interface {
	// CreateBackend creates a backend instance based on the provided config
	CreateBackend(ctx context.Context, config Config) (*BackendResult, error)
}

// Config holds configuration for backend creation
type Config struct {
	// Backend type
	Type BackendType
	
	// SQLite specific
	SQLiteDBPath string
	AMQPURL      string
	AMQPExchange string
	AMQPQueue    string
	
	// Google Sheets specific  
	GoogleSpreadsheetID   string
	GoogleSheetName       string
	GoogleOAuthClientFile string
	GoogleOAuthTokenFile  string
	GoogleOAuthClientJSON string
	GoogleOAuthTokenJSON  string
	
	// Memory backend specific
	DataDirectory string
}

// BackendType represents the type of backend
type BackendType string

const (
	SQLiteBackend BackendType = "sqlite"
	SheetsBackend BackendType = "sheets" 
	MemoryBackend BackendType = "memory"
)

// String implements fmt.Stringer
func (bt BackendType) String() string {
	return string(bt)
}

// IsValid returns true if the backend type is valid
func (bt BackendType) IsValid() bool {
	switch bt {
	case SQLiteBackend, SheetsBackend, MemoryBackend:
		return true
	default:
		return false
	}
}