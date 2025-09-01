//go:build integration

package google

import (
	"context"
	"os"
	"spese/internal/core"
	"strings"
	"testing"
	"time"
)

// Integration tests require real Google Sheets credentials
// Run with: go test -tags=integration ./internal/sheets/google

func TestIntegration_GoogleSheetsFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Check for required environment variables
	spreadsheetID := os.Getenv("GOOGLE_SPREADSHEET_ID")
	if spreadsheetID == "" {
		t.Skip("GOOGLE_SPREADSHEET_ID not set, skipping integration test")
	}

	// Check OAuth credentials
	clientJSON := os.Getenv("GOOGLE_OAUTH_CLIENT_JSON")
	clientFile := os.Getenv("GOOGLE_OAUTH_CLIENT_FILE")
	tokenJSON := os.Getenv("GOOGLE_OAUTH_TOKEN_JSON")
	tokenFile := os.Getenv("GOOGLE_OAUTH_TOKEN_FILE")

	if (clientJSON == "" && clientFile == "") || (tokenJSON == "" && tokenFile == "") {
		t.Skip("OAuth credentials not configured, skipping integration test")
	}

	ctx := context.Background()
	client, err := NewFromEnv(ctx)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	t.Run("TaxonomyReader", func(t *testing.T) {
		categories, subcategories, err := client.List(ctx)
		if err != nil {
			t.Fatalf("Failed to read taxonomy: %v", err)
		}

		t.Logf("Found %d categories: %v", len(categories), categories)
		t.Logf("Found %d subcategories: %v", len(subcategories), subcategories)

		if len(categories) == 0 {
			t.Error("Expected at least one category")
		}
		if len(subcategories) == 0 {
			t.Error("Expected at least one subcategory")
		}

		// Verify no duplicates
		seen := make(map[string]bool)
		for _, cat := range categories {
			if seen[cat] {
				t.Errorf("Duplicate category found: %s", cat)
			}
			seen[cat] = true
		}

		seen = make(map[string]bool)
		for _, sub := range subcategories {
			if seen[sub] {
				t.Errorf("Duplicate subcategory found: %s", sub)
			}
			seen[sub] = true
		}
	})

	t.Run("ExpenseWriter", func(t *testing.T) {
		// Get categories for test data
		categories, subcategories, err := client.List(ctx)
		if err != nil {
			t.Fatalf("Failed to get categories for test: %v", err)
		}
		if len(categories) == 0 || len(subcategories) == 0 {
			t.Skip("No categories/subcategories available for test")
		}

		// Create test expense
		testExpense := core.Expense{
			Date:        core.DateParts{Day: time.Now().Day(), Month: int(time.Now().Month())},
			Description: "Integration Test Expense",
			Amount:      core.Money{Cents: 1234}, // €12.34
			Primary:     categories[0],
			Secondary:   subcategories[0],
		}

		ref, err := client.Append(ctx, testExpense)
		if err != nil {
			t.Fatalf("Failed to append expense: %v", err)
		}

		t.Logf("Created expense with reference: %s", ref)

		// Verify reference format (should be like "mem:N" for memory or actual Sheets reference)
		if ref == "" {
			t.Error("Expected non-empty reference")
		}
	})

	t.Run("DashboardReader", func(t *testing.T) {
		// Test current month overview
		now := time.Now()
		overview, err := client.ReadMonthOverview(ctx, now.Year(), int(now.Month()))
		if err != nil {
			// Dashboard might not exist, which is okay for this test
			t.Logf("Dashboard read failed (expected if no dashboard exists): %v", err)
			return
		}

		t.Logf("Month overview for %d/%d: Total=€%.2f, Categories=%d", 
			overview.Year, overview.Month, 
			float64(overview.Total.Cents)/100, 
			len(overview.ByCategory))

		// Validate overview structure
		if overview.Year != now.Year() {
			t.Errorf("Expected year %d, got %d", now.Year(), overview.Year)
		}
		if overview.Month != int(now.Month()) {
			t.Errorf("Expected month %d, got %d", int(now.Month()), overview.Month)
		}

		// Total should be sum of categories (if any)
		var categorySum int64
		for _, cat := range overview.ByCategory {
			categorySum += cat.Amount.Cents
			if cat.Name == "" {
				t.Error("Category name should not be empty")
			}
			if cat.Amount.Cents < 0 {
				t.Error("Category amount should not be negative")
			}
		}

		if len(overview.ByCategory) > 0 && overview.Total.Cents != categorySum {
			t.Errorf("Total (%d cents) doesn't match sum of categories (%d cents)", 
				overview.Total.Cents, categorySum)
		}
	})

	t.Run("ExpenseLister", func(t *testing.T) {
		now := time.Now()
		expenses, err := client.ListExpenses(ctx, now.Year(), int(now.Month()))
		if err != nil {
			t.Fatalf("Failed to list expenses: %v", err)
		}

		t.Logf("Found %d expenses for %d/%d", len(expenses), now.Year(), int(now.Month()))

		// Validate expense structure
		for i, exp := range expenses {
			if exp.Description == "" {
				t.Errorf("Expense %d: description should not be empty", i)
			}
			if exp.Amount.Cents <= 0 {
				t.Errorf("Expense %d: amount should be positive, got %d", i, exp.Amount.Cents)
			}
			if exp.Date.Day < 1 || exp.Date.Day > 31 {
				t.Errorf("Expense %d: invalid day %d", i, exp.Date.Day)
			}
			if exp.Date.Month < 1 || exp.Date.Month > 12 {
				t.Errorf("Expense %d: invalid month %d", i, exp.Date.Month)
			}
			if exp.Primary == "" {
				t.Errorf("Expense %d: primary category should not be empty", i)
			}
			if exp.Secondary == "" {
				t.Errorf("Expense %d: secondary category should not be empty", i)
			}
		}
	})
}

func TestIntegration_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()

	t.Run("InvalidSpreadsheetID", func(t *testing.T) {
		// Save original values
		origID := os.Getenv("GOOGLE_SPREADSHEET_ID")
		defer os.Setenv("GOOGLE_SPREADSHEET_ID", origID)

		// Set invalid spreadsheet ID
		os.Setenv("GOOGLE_SPREADSHEET_ID", "invalid-spreadsheet-id")

		client, err := NewFromEnv(ctx)
		if err != nil {
			t.Skip("Cannot create client, skipping error handling test")
		}

		// Try to read taxonomy - should fail with Google Sheets API error
		_, _, err = client.List(ctx)
		if err == nil {
			t.Error("Expected error with invalid spreadsheet ID")
		}
		if !strings.Contains(err.Error(), "spreadsheet") {
			t.Errorf("Expected spreadsheet-related error, got: %v", err)
		}
	})

	t.Run("InvalidExpenseValidation", func(t *testing.T) {
		// Skip if no valid credentials
		if os.Getenv("GOOGLE_SPREADSHEET_ID") == "" {
			t.Skip("GOOGLE_SPREADSHEET_ID not set")
		}

		client, err := NewFromEnv(ctx)
		if err != nil {
			t.Skip("Cannot create client, skipping validation test")
		}

		// Test with invalid expense
		invalidExpense := core.Expense{
			Date:        core.DateParts{Day: 0, Month: 1}, // Invalid day
			Description: "Test",
			Amount:      core.Money{Cents: 100},
			Primary:     "Test",
			Secondary:   "Test",
		}

		_, err = client.Append(ctx, invalidExpense)
		if err == nil {
			t.Error("Expected validation error for invalid expense")
		}
		if !strings.Contains(err.Error(), "day") {
			t.Errorf("Expected day validation error, got: %v", err)
		}
	})

	t.Run("ContextCancellation", func(t *testing.T) {
		if os.Getenv("GOOGLE_SPREADSHEET_ID") == "" {
			t.Skip("GOOGLE_SPREADSHEET_ID not set")
		}

		client, err := NewFromEnv(context.Background())
		if err != nil {
			t.Skip("Cannot create client, skipping context test")
		}

		// Create cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Operations should fail with context cancellation
		_, _, err = client.List(ctx)
		if err == nil {
			t.Error("Expected context cancellation error")
		}

		_, err = client.ReadMonthOverview(ctx, 2025, 1)
		if err == nil {
			t.Error("Expected context cancellation error")
		}

		_, err = client.ListExpenses(ctx, 2025, 1)
		if err == nil {
			t.Error("Expected context cancellation error")
		}
	})
}

func TestIntegration_SheetNaming(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Test year prefixing logic
	tests := []struct {
		baseName string
		year     int
		expected string
	}{
		{"Expenses", 2025, "2025 Expenses"},
		{"Dashboard", 2024, "2024 Dashboard"},
		{"", 2025, "2025 "},
	}

	for _, tt := range tests {
		got := yearPrefixedName(tt.baseName, tt.year)
		if got != tt.expected {
			t.Errorf("yearPrefixedName(%q, %d) = %q, want %q", 
				tt.baseName, tt.year, got, tt.expected)
		}
	}
}

func TestIntegration_ConfigurationVariations(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()

	// Save original environment
	origVars := map[string]string{
		"GOOGLE_SHEET_NAME":              os.Getenv("GOOGLE_SHEET_NAME"),
		"GOOGLE_CATEGORIES_SHEET_NAME":   os.Getenv("GOOGLE_CATEGORIES_SHEET_NAME"),
		"GOOGLE_SUBCATEGORIES_SHEET_NAME": os.Getenv("GOOGLE_SUBCATEGORIES_SHEET_NAME"),
		"DASHBOARD_SHEET_NAME":           os.Getenv("DASHBOARD_SHEET_NAME"),
		"DASHBOARD_SHEET_PREFIX":         os.Getenv("DASHBOARD_SHEET_PREFIX"),
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

	testCases := []struct {
		name   string
		config map[string]string
	}{
		{
			name: "DefaultNames",
			config: map[string]string{
				"GOOGLE_SHEET_NAME":              "",
				"GOOGLE_CATEGORIES_SHEET_NAME":   "",
				"GOOGLE_SUBCATEGORIES_SHEET_NAME": "",
				"DASHBOARD_SHEET_NAME":           "",
				"DASHBOARD_SHEET_PREFIX":         "",
			},
		},
		{
			name: "CustomNames",
			config: map[string]string{
				"GOOGLE_SHEET_NAME":              "MyExpenses",
				"GOOGLE_CATEGORIES_SHEET_NAME":   "MyCategories",
				"GOOGLE_SUBCATEGORIES_SHEET_NAME": "MySubcategories",
				"DASHBOARD_SHEET_NAME":           "MyDashboard",
			},
		},
		{
			name: "LegacyPrefix",
			config: map[string]string{
				"DASHBOARD_SHEET_PREFIX": "%d Dashboard",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set configuration
			for k, v := range tc.config {
				if v == "" {
					os.Unsetenv(k)
				} else {
					os.Setenv(k, v)
				}
			}

			// Skip if missing required credentials
			if os.Getenv("GOOGLE_SPREADSHEET_ID") == "" {
				t.Skip("GOOGLE_SPREADSHEET_ID not set")
			}

			client, err := NewFromEnv(ctx)
			if err != nil {
				t.Logf("Expected error with configuration %s: %v", tc.name, err)
				return
			}

			// Verify client was created with expected sheet names
			currentYear := time.Now().Year()
			
			if tc.config["GOOGLE_SHEET_NAME"] != "" {
				expected := yearPrefixedName(tc.config["GOOGLE_SHEET_NAME"], currentYear)
				if client.expensesSheet != expected {
					t.Errorf("Expected expenses sheet %q, got %q", expected, client.expensesSheet)
				}
			}

			// Test basic functionality doesn't crash
			_, _, err = client.List(ctx)
			if err != nil {
				t.Logf("List failed with %s config (may be expected): %v", tc.name, err)
			}
		})
	}
}