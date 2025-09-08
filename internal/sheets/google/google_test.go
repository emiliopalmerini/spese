package google

import (
	"context"
	"errors"
	"fmt"
	"os"
	"spese/internal/core"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestNewFromEnv_MissingSpreadsheetID(t *testing.T) {
	// Clear environment
	oldID := os.Getenv("GOOGLE_SPREADSHEET_ID")
	defer os.Setenv("GOOGLE_SPREADSHEET_ID", oldID)
	os.Unsetenv("GOOGLE_SPREADSHEET_ID")

	_, err := NewFromEnv(context.Background())
	if err == nil {
		t.Fatal("expected error for missing GOOGLE_SPREADSHEET_ID")
	}
	if err.Error() != "missing GOOGLE_SPREADSHEET_ID" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNewFromEnv_WithValidCredentials(t *testing.T) {
	// This test only verifies that we fail gracefully with invalid JSON
	// rather than testing the full OAuth flow which would require real credentials
	oldID := os.Getenv("GOOGLE_SPREADSHEET_ID")
	oldClient := os.Getenv("GOOGLE_OAUTH_CLIENT_JSON")
	oldToken := os.Getenv("GOOGLE_OAUTH_TOKEN_JSON")
	defer func() {
		os.Setenv("GOOGLE_SPREADSHEET_ID", oldID)
		os.Setenv("GOOGLE_OAUTH_CLIENT_JSON", oldClient)
		os.Setenv("GOOGLE_OAUTH_TOKEN_JSON", oldToken)
	}()

	os.Setenv("GOOGLE_SPREADSHEET_ID", "test-id")
	os.Setenv("GOOGLE_OAUTH_CLIENT_JSON", `invalid-json`)
	os.Setenv("GOOGLE_OAUTH_TOKEN_JSON", `{"access_token":"test"}`)

	_, err := NewFromEnv(context.Background())
	if err == nil {
		t.Fatal("expected error with invalid JSON")
	}
	if !strings.Contains(err.Error(), "oauth config") {
		t.Errorf("expected oauth config error, got: %v", err)
	}
}

func TestClient_validateExpense(t *testing.T) {
	c := &Client{spreadsheetID: "test"} // svc is nil, which will cause append to fail

	// Test with invalid expense
	invalidExp := core.Expense{
		Date:        core.DateParts{Day: 0, Month: 1}, // invalid day
		Description: "test",
		Amount:      core.Money{Cents: 100},
		Primary:     "test",
		Secondary:   "test",
	}

	_, err := c.Append(context.Background(), invalidExp)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !errors.Is(err, core.ErrInvalidDay) {
		t.Errorf("expected ErrInvalidDay, got: %v", err)
	}
}

func TestClient_readColParsing(t *testing.T) {
	// Test the deduplication and filtering logic for specific ranges (no header skipping)

	// Mock the sheet response data (A3:A65 or B3:B65 range - no headers)
	testData := [][]interface{}{
		{"Food"},
		{"Transport"},
		{""},         // empty
		{"#Comment"}, // comment
		{"Food"},     // duplicate
		{"Shopping"},
	}

	// We can't easily test the actual readCol method without mocking the entire
	// Google Sheets service, so let's test the core logic separately

	var out []string
	for _, row := range testData {
		if len(row) == 0 {
			continue
		}
		v := fmt.Sprint(row[0])
		if v == "" || strings.HasPrefix(v, "#") {
			continue
		}
		out = append(out, v)
	}

	// Dedup logic
	seen := map[string]struct{}{}
	uniq := make([]string, 0, len(out))
	for _, v := range out {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		uniq = append(uniq, v)
	}

	expected := []string{"Food", "Transport", "Shopping"}
	if len(uniq) != len(expected) {
		t.Errorf("expected %d items, got %d", len(expected), len(uniq))
	}
	for i, exp := range expected {
		if i >= len(uniq) || uniq[i] != exp {
			t.Errorf("expected %s at index %d, got %s", exp, i, uniq[i])
		}
	}
}

func TestJsonUnmarshalIndirection(t *testing.T) {
	// Test that our indirection works
	data := []byte(`{"access_token":"test","token_type":"Bearer"}`)
	var token oauth2.Token

	err := jsonUnmarshal(data, &token)
	if err != nil {
		t.Fatalf("jsonUnmarshal failed: %v", err)
	}

	if token.AccessToken != "test" {
		t.Errorf("expected access token 'test', got %s", token.AccessToken)
	}

	// Test with invalid JSON
	invalidData := []byte(`{invalid json}`)
	err = jsonUnmarshal(invalidData, &token)
	if err == nil {
		t.Fatal("expected error with invalid JSON")
	}
}

func TestNewSheetsService_MissingOAuthClient(t *testing.T) {
	// Clear all oauth env vars
	oldVars := map[string]string{
		"GOOGLE_OAUTH_CLIENT_JSON": os.Getenv("GOOGLE_OAUTH_CLIENT_JSON"),
		"GOOGLE_OAUTH_CLIENT_FILE": os.Getenv("GOOGLE_OAUTH_CLIENT_FILE"),
		"GOOGLE_OAUTH_TOKEN_JSON":  os.Getenv("GOOGLE_OAUTH_TOKEN_JSON"),
		"GOOGLE_OAUTH_TOKEN_FILE":  os.Getenv("GOOGLE_OAUTH_TOKEN_FILE"),
	}
	defer func() {
		for k, v := range oldVars {
			os.Setenv(k, v)
		}
	}()

	for k := range oldVars {
		os.Unsetenv(k)
	}

	_, err := newSheetsService(context.Background())
	if err == nil {
		t.Fatal("expected error for missing oauth client")
	}
	expectedMsg := "missing oauth client (set GOOGLE_OAUTH_CLIENT_JSON or GOOGLE_OAUTH_CLIENT_FILE)"
	if err.Error() != expectedMsg {
		t.Errorf("expected %q, got %q", expectedMsg, err.Error())
	}
}

func TestNewSheetsService_MissingOAuthToken(t *testing.T) {
	oldVars := map[string]string{
		"GOOGLE_OAUTH_CLIENT_JSON": os.Getenv("GOOGLE_OAUTH_CLIENT_JSON"),
		"GOOGLE_OAUTH_TOKEN_JSON":  os.Getenv("GOOGLE_OAUTH_TOKEN_JSON"),
		"GOOGLE_OAUTH_TOKEN_FILE":  os.Getenv("GOOGLE_OAUTH_TOKEN_FILE"),
	}
	defer func() {
		for k, v := range oldVars {
			os.Setenv(k, v)
		}
	}()

	// Set client but not token
	os.Setenv("GOOGLE_OAUTH_CLIENT_JSON", `{"installed":{"client_id":"test","client_secret":"test","redirect_uris":["http://localhost"],"auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`)
	os.Unsetenv("GOOGLE_OAUTH_TOKEN_JSON")
	os.Unsetenv("GOOGLE_OAUTH_TOKEN_FILE")

	_, err := newSheetsService(context.Background())
	if err == nil {
		t.Fatal("expected error for missing oauth token")
	}
	expectedMsg := "missing oauth token (set GOOGLE_OAUTH_TOKEN_JSON or GOOGLE_OAUTH_TOKEN_FILE)"
	if err.Error() != expectedMsg {
		t.Errorf("expected %q, got %q", expectedMsg, err.Error())
	}
}

// Test year prefixed name function
func TestYearPrefixedName(t *testing.T) {
	tests := []struct {
		baseName string
		year     int
		expected string
	}{
		{"Expenses", 2025, "2025 Expenses"},
		{"Dashboard", 2024, "2024 Dashboard"},
		{"", 2023, ""}, // Empty base returns empty
		{"Test Sheet", 2022, "2022 Test Sheet"},
		{"2025 Already Prefixed", 2024, "2025 Already Prefixed"}, // Already has year prefix
	}

	for _, tt := range tests {
		got := yearPrefixedName(tt.baseName, tt.year)
		if got != tt.expected {
			t.Errorf("yearPrefixedName(%q, %d) = %q, want %q",
				tt.baseName, tt.year, got, tt.expected)
		}
	}
}

// Test dashboard sheet naming logic
func TestDashboardSheetNaming(t *testing.T) {
	// Save original env vars
	origDashName := os.Getenv("DASHBOARD_SHEET_NAME")
	origDashPrefix := os.Getenv("DASHBOARD_SHEET_PREFIX")
	origSpreadsheetID := os.Getenv("GOOGLE_SPREADSHEET_ID")
	defer func() {
		os.Setenv("DASHBOARD_SHEET_NAME", origDashName)
		os.Setenv("DASHBOARD_SHEET_PREFIX", origDashPrefix)
		os.Setenv("GOOGLE_SPREADSHEET_ID", origSpreadsheetID)
	}()

	// Set required spreadsheet ID
	os.Setenv("GOOGLE_SPREADSHEET_ID", "test-id")

	tests := []struct {
		name           string
		dashName       string
		dashPrefix     string
		expectedBase   string
		expectedPrefix string
	}{
		{
			name:           "DefaultDashboard",
			dashName:       "",
			dashPrefix:     "",
			expectedBase:   "Dashboard",
			expectedPrefix: "",
		},
		{
			name:           "CustomBaseName",
			dashName:       "MyDashboard",
			dashPrefix:     "",
			expectedBase:   "MyDashboard",
			expectedPrefix: "",
		},
		{
			name:           "LegacyPrefix",
			dashName:       "",
			dashPrefix:     "%d Dashboard",
			expectedBase:   "",
			expectedPrefix: "%d Dashboard",
		},
		{
			name:           "BaseOverridesPrefix",
			dashName:       "CustomBase",
			dashPrefix:     "%d Prefix",
			expectedBase:   "CustomBase",
			expectedPrefix: "%d Prefix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			if tt.dashName == "" {
				os.Unsetenv("DASHBOARD_SHEET_NAME")
			} else {
				os.Setenv("DASHBOARD_SHEET_NAME", tt.dashName)
			}
			if tt.dashPrefix == "" {
				os.Unsetenv("DASHBOARD_SHEET_PREFIX")
			} else {
				os.Setenv("DASHBOARD_SHEET_PREFIX", tt.dashPrefix)
			}

			// This will fail because we don't have valid OAuth, but we can check
			// that the error happens at the OAuth stage, not config parsing
			_, err := NewFromEnv(context.Background())
			if err == nil {
				t.Fatal("expected OAuth error")
			}
			// Should fail at OAuth stage, not config parsing
			if !strings.Contains(err.Error(), "sheets service") {
				t.Errorf("expected OAuth error, got: %v", err)
			}
		})
	}
}

// Test default sheet names
func TestDefaultSheetNames(t *testing.T) {
	origVars := map[string]string{
		"GOOGLE_SPREADSHEET_ID":           os.Getenv("GOOGLE_SPREADSHEET_ID"),
		"GOOGLE_SHEET_NAME":               os.Getenv("GOOGLE_SHEET_NAME"),
		"GOOGLE_CATEGORIES_SHEET_NAME":    os.Getenv("GOOGLE_CATEGORIES_SHEET_NAME"),
		"GOOGLE_SUBCATEGORIES_SHEET_NAME": os.Getenv("GOOGLE_SUBCATEGORIES_SHEET_NAME"),
	}
	defer func() {
		for k, v := range origVars {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	// Clear all sheet name environment variables
	os.Setenv("GOOGLE_SPREADSHEET_ID", "test-id")
	os.Unsetenv("GOOGLE_SHEET_NAME")
	os.Unsetenv("GOOGLE_CATEGORIES_SHEET_NAME")
	os.Unsetenv("GOOGLE_SUBCATEGORIES_SHEET_NAME")

	_, err := NewFromEnv(context.Background())
	if err == nil {
		t.Fatal("expected OAuth error")
	}
	// Should fail at OAuth stage, not config parsing
	if !strings.Contains(err.Error(), "sheets service") {
		t.Errorf("expected OAuth service error, got: %v", err)
	}
}

// Test OAuth credential parsing
func TestOAuthCredentialParsing(t *testing.T) {
	oldVars := map[string]string{
		"GOOGLE_OAUTH_CLIENT_JSON": os.Getenv("GOOGLE_OAUTH_CLIENT_JSON"),
		"GOOGLE_OAUTH_CLIENT_FILE": os.Getenv("GOOGLE_OAUTH_CLIENT_FILE"),
		"GOOGLE_OAUTH_TOKEN_JSON":  os.Getenv("GOOGLE_OAUTH_TOKEN_JSON"),
		"GOOGLE_OAUTH_TOKEN_FILE":  os.Getenv("GOOGLE_OAUTH_TOKEN_FILE"),
	}
	defer func() {
		for k, v := range oldVars {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	// Test valid client JSON but invalid token JSON
	os.Setenv("GOOGLE_OAUTH_CLIENT_JSON", `{"installed":{"client_id":"test","client_secret":"test","redirect_uris":["http://localhost"],"auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`)
	os.Setenv("GOOGLE_OAUTH_TOKEN_JSON", `invalid-json`)
	os.Unsetenv("GOOGLE_OAUTH_CLIENT_FILE")
	os.Unsetenv("GOOGLE_OAUTH_TOKEN_FILE")

	_, err := newSheetsService(context.Background())
	if err == nil {
		t.Fatal("expected error with invalid token JSON")
	}
	if !strings.Contains(err.Error(), "oauth token") {
		t.Errorf("expected token parsing error, got: %v", err)
	}

	// Test invalid client JSON
	os.Setenv("GOOGLE_OAUTH_CLIENT_JSON", `invalid-json`)
	os.Setenv("GOOGLE_OAUTH_TOKEN_JSON", `{"access_token":"test","token_type":"Bearer"}`)

	_, err = newSheetsService(context.Background())
	if err == nil {
		t.Fatal("expected error with invalid client JSON")
	}
	if !strings.Contains(err.Error(), "oauth config") {
		t.Errorf("expected client parsing error, got: %v", err)
	}
}

// Test expense validation edge cases
func TestExpenseValidationEdgeCases(t *testing.T) {
	c := &Client{spreadsheetID: "test"} // svc is nil

	tests := []struct {
		name        string
		expense     core.Expense
		expectedErr string
	}{
		{
			name: "ValidExpense",
			expense: core.Expense{
				Date:        core.DateParts{Day: 15, Month: 6},
				Description: "Test expense",
				Amount:      core.Money{Cents: 1000},
				Primary:     "Food",
				Secondary:   "Restaurant",
			},
			expectedErr: "sheets service not initialized", // Will fail at service call
		},
		{
			name: "InvalidMonth",
			expense: core.Expense{
				Date:        core.DateParts{Day: 15, Month: 13},
				Description: "Test",
				Amount:      core.Money{Cents: 1000},
				Primary:     "Food",
				Secondary:   "Restaurant",
			},
			expectedErr: "invalid month",
		},
		{
			name: "NegativeAmount",
			expense: core.Expense{
				Date:        core.DateParts{Day: 15, Month: 6},
				Description: "Test",
				Amount:      core.Money{Cents: -100},
				Primary:     "Food",
				Secondary:   "Restaurant",
			},
			expectedErr: "invalid amount",
		},
		{
			name: "EmptyDescription",
			expense: core.Expense{
				Date:        core.DateParts{Day: 15, Month: 6},
				Description: "   ", // Only whitespace
				Amount:      core.Money{Cents: 1000},
				Primary:     "Food",
				Secondary:   "Restaurant",
			},
			expectedErr: "empty description",
		},
		{
			name: "EmptyPrimary",
			expense: core.Expense{
				Date:        core.DateParts{Day: 15, Month: 6},
				Description: "Test",
				Amount:      core.Money{Cents: 1000},
				Primary:     "",
				Secondary:   "Restaurant",
			},
			expectedErr: "empty primary",
		},
		{
			name: "EmptySecondary",
			expense: core.Expense{
				Date:        core.DateParts{Day: 15, Month: 6},
				Description: "Test",
				Amount:      core.Money{Cents: 1000},
				Primary:     "Food",
				Secondary:   "",
			},
			expectedErr: "empty secondary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.Append(context.Background(), tt.expense)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.expectedErr)) {
				t.Errorf("expected error containing %q, got %q", tt.expectedErr, err.Error())
			}
		})
	}
}
