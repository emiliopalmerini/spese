package google

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

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
