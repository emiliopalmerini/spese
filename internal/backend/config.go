package backend

import (
	"fmt"
	"spese/internal/config"
)

// FromAppConfig converts the application config to backend config
func FromAppConfig(appConfig *config.Config) (Config, error) {
	if appConfig == nil {
		return Config{}, fmt.Errorf("app config is nil")
	}

	backendType := BackendType(appConfig.DataBackend)
	if !backendType.IsValid() {
		return Config{}, fmt.Errorf("invalid backend type in config: %s", appConfig.DataBackend)
	}

	return Config{
		Type: backendType,
		
		// SQLite configuration
		SQLiteDBPath: appConfig.SQLiteDBPath,
		AMQPURL:      appConfig.AMQPURL,
		AMQPExchange: appConfig.AMQPExchange,
		AMQPQueue:    appConfig.AMQPQueue,
		
		// Google Sheets configuration
		GoogleSpreadsheetID:   appConfig.GoogleSpreadsheetID,
		GoogleSheetName:       appConfig.GoogleSheetName,
		GoogleOAuthClientFile: appConfig.GoogleOAuthClientFile,
		GoogleOAuthTokenFile:  appConfig.GoogleOAuthTokenFile,
		GoogleOAuthClientJSON: appConfig.GoogleOAuthClientJSON,
		GoogleOAuthTokenJSON:  appConfig.GoogleOAuthTokenJSON,
		
		// Memory backend uses default data directory
		DataDirectory: "data",
	}, nil
}

// Validate validates the backend configuration
func (c Config) Validate() error {
	if !c.Type.IsValid() {
		return fmt.Errorf("invalid backend type: %s", c.Type)
	}

	switch c.Type {
	case SQLiteBackend:
		if c.SQLiteDBPath == "" {
			return fmt.Errorf("SQLite database path is required for sqlite backend")
		}
		// AMQP is optional, so we don't validate it
		
	case SheetsBackend:
		if c.GoogleSpreadsheetID == "" {
			return fmt.Errorf("Google Spreadsheet ID is required for sheets backend")
		}
		if c.GoogleSheetName == "" {
			return fmt.Errorf("Google Sheet name is required for sheets backend")
		}
		
		// Must have either client file or JSON
		hasClientFile := c.GoogleOAuthClientFile != ""
		hasClientJSON := c.GoogleOAuthClientJSON != ""
		if !hasClientFile && !hasClientJSON {
			return fmt.Errorf("either GoogleOAuthClientFile or GoogleOAuthClientJSON must be provided for sheets backend")
		}
		
		// Must have either token file or JSON
		hasTokenFile := c.GoogleOAuthTokenFile != ""
		hasTokenJSON := c.GoogleOAuthTokenJSON != ""
		if !hasTokenFile && !hasTokenJSON {
			return fmt.Errorf("either GoogleOAuthTokenFile or GoogleOAuthTokenJSON must be provided for sheets backend")
		}
		
	case MemoryBackend:
		// Memory backend doesn't require additional validation
		// DataDirectory will default to "data" if empty
	}

	return nil
}

// GetBackendTypes returns all valid backend types
func GetBackendTypes() []BackendType {
	return []BackendType{SQLiteBackend, SheetsBackend, MemoryBackend}
}

// GetBackendTypeStrings returns all valid backend type strings
func GetBackendTypeStrings() []string {
	types := GetBackendTypes()
	strings := make([]string, len(types))
	for i, t := range types {
		strings[i] = t.String()
	}
	return strings
}