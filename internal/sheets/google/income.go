package google

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"spese/internal/core"

	gsheet "google.golang.org/api/sheets/v4"
)

// AppendIncome writes an income entry to the Google Sheets income sheet.
// Implements sheets.IncomeWriter interface.
func (c *Client) AppendIncome(ctx context.Context, i core.Income) (string, error) {
	if err := i.Validate(); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}
	if c.svc == nil {
		return "", errors.New("sheets service not initialized")
	}
	if c.incomeSheet == "" {
		return "", errors.New("income sheet not configured")
	}

	// Convert cents to decimal
	euros := float64(i.Amount.Cents) / 100.0

	// Get next row using income-specific cache
	nextRow, err := c.getNextIncomeRow(ctx)
	if err != nil {
		return "", err
	}

	// Income columns: A=Month, B=Day, C=Description, D=Amount, skip E,F, G=Category
	// Update A:D (Month, Day, Description, Amount)
	dataRange1 := fmt.Sprintf("%s!A%d:D%d", c.incomeSheet, nextRow, nextRow)
	vr1 := &gsheet.ValueRange{Values: [][]any{{i.Date.Month(), i.Date.Day(), i.Description, euros}}}

	_, err = c.svc.Spreadsheets.Values.Update(c.spreadsheetID, dataRange1, vr1).
		ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		c.invalidateIncomeRowCache()
		return "", fmt.Errorf("failed to update A:D in sheet %s: %w", c.incomeSheet, err)
	}

	// Update G (Category) - income has single category
	dataRange2 := fmt.Sprintf("%s!G%d:G%d", c.incomeSheet, nextRow, nextRow)
	vr2 := &gsheet.ValueRange{Values: [][]any{{i.Category}}}

	_, err = c.svc.Spreadsheets.Values.Update(c.spreadsheetID, dataRange2, vr2).
		ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		c.invalidateIncomeRowCache()
		return "", fmt.Errorf("failed to update G in sheet %s: %w", c.incomeSheet, err)
	}

	ref := fmt.Sprintf("%s!A%d:G%d", c.incomeSheet, nextRow, nextRow)

	slog.InfoContext(ctx, "Income appended to Google Sheets",
		"sheet", c.incomeSheet,
		"row", nextRow,
		"description", i.Description,
		"amount", euros,
		"category", i.Category)

	return ref, nil
}

// getNextIncomeRow returns the next available row number for the income sheet.
// Uses a separate cache from expenses to avoid interference.
func (c *Client) getNextIncomeRow(ctx context.Context) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if income cache is still valid
	if time.Now().Before(c.incomeCacheExpiresAt) && c.cachedIncomeRowCount > 0 {
		slog.DebugContext(ctx, "Using cached income row count",
			"cached_row_count", c.cachedIncomeRowCount,
			"expires_in", time.Until(c.incomeCacheExpiresAt).Round(time.Second))
		return c.cachedIncomeRowCount + 1, nil
	}

	// Cache miss or expired: read from sheet
	slog.InfoContext(ctx, "Income row count cache expired or invalid, refreshing from sheet",
		"cached_row_count", c.cachedIncomeRowCount,
		"expires_at", c.incomeCacheExpiresAt.Format(time.RFC3339))

	rng := fmt.Sprintf("%s!A:A", c.incomeSheet)
	resp, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rng).Context(ctx).Do()
	if err != nil {
		return 0, fmt.Errorf("failed to get sheet dimensions for %s: %w", c.incomeSheet, err)
	}

	// Update cache
	c.cachedIncomeRowCount = len(resp.Values)
	c.incomeCacheExpiresAt = time.Now().Add(c.cacheValidDuration)

	nextRow := c.cachedIncomeRowCount + 1

	slog.InfoContext(ctx, "Updated income row count cache",
		"row_count", c.cachedIncomeRowCount,
		"next_row", nextRow,
		"cache_expires_at", c.incomeCacheExpiresAt.Format(time.RFC3339))

	return nextRow, nil
}

// invalidateIncomeRowCache clears the cached income row count
func (c *Client) invalidateIncomeRowCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.incomeCacheExpiresAt = time.Now()
	slog.DebugContext(context.Background(), "Income row count cache invalidated")
}
