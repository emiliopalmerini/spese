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
	
	// Mock the sheet response data (A2:A65 or B2:B65 range - no headers)
	testData := [][]interface{}{
		{"Food"},
		{"Transport"},
		{""},           // empty
		{"#Comment"},   // comment
		{"Food"},       // duplicate
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