// Package sheets is the shared low-level Google Sheets adapter.
// It exposes ReadRange / ReadTable / AppendRows with a 5-minute in-memory
// read cache that is invalidated whenever a tab is written.
//
// Feature slices wrap this with their own typed parsers (parse a row into
// a Transaction, an Account, etc.).
package sheets

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// Client is a thin wrapper around the Google Sheets API for a single
// spreadsheet. Safe for concurrent use.
type Client struct {
	svc           *sheets.Service
	spreadsheetID string
	cacheTTL      time.Duration

	mu    sync.RWMutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	data    [][]any
	fetched time.Time
}

// New builds a client authenticated with a service-account JSON file.
func New(ctx context.Context, credentialsPath, spreadsheetID string) (*Client, error) {
	svc, err := sheets.NewService(ctx, option.WithCredentialsFile(credentialsPath))
	if err != nil {
		return nil, fmt.Errorf("sheets service: %w", err)
	}
	return &Client{
		svc:           svc,
		spreadsheetID: spreadsheetID,
		cacheTTL:      5 * time.Minute,
		cache:         make(map[string]cacheEntry),
	}, nil
}

// SpreadsheetID returns the configured spreadsheet ID. Useful for building
// share links in UI.
func (c *Client) SpreadsheetID() string { return c.spreadsheetID }

// ReadRange returns raw cell values for an A1 range like "transactions!A2:I"
// or a bare tab name like "accounts". Results are cached for the client's TTL
// unless force is true.
func (c *Client) ReadRange(ctx context.Context, rangeA1 string, force bool) ([][]any, error) {
	if !force {
		c.mu.RLock()
		entry, ok := c.cache[rangeA1]
		c.mu.RUnlock()
		if ok && time.Since(entry.fetched) < c.cacheTTL {
			return entry.data, nil
		}
	}

	resp, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rangeA1).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("sheets get %s: %w", rangeA1, err)
	}

	c.mu.Lock()
	c.cache[rangeA1] = cacheEntry{data: resp.Values, fetched: time.Now()}
	c.mu.Unlock()

	return resp.Values, nil
}

// ReadTable reads a tab and splits header from data rows.
func (c *Client) ReadTable(ctx context.Context, tab string, force bool) (headers []string, rows [][]any, err error) {
	data, err := c.ReadRange(ctx, tab, force)
	if err != nil {
		return nil, nil, err
	}
	if len(data) == 0 {
		return nil, nil, nil
	}
	for _, v := range data[0] {
		headers = append(headers, fmt.Sprint(v))
	}
	if len(data) > 1 {
		rows = data[1:]
	}
	return headers, rows, nil
}

// AppendRows adds rows to the end of a tab (auto-detects the table extent).
// Invalidates any cached read of that tab.
func (c *Client) AppendRows(ctx context.Context, tab string, rows [][]any) error {
	if len(rows) == 0 {
		return nil
	}
	vr := &sheets.ValueRange{Values: rows}
	_, err := c.svc.Spreadsheets.Values.Append(c.spreadsheetID, tab, vr).
		ValueInputOption("USER_ENTERED").
		InsertDataOption("INSERT_ROWS").
		Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("sheets append %s: %w", tab, err)
	}
	c.invalidate(tab)
	return nil
}

// Invalidate drops cached reads for any range starting with tabPrefix.
// Call this manually if a feature edits cells outside its own AppendRows.
func (c *Client) Invalidate(tabPrefix string) { c.invalidate(tabPrefix) }

func (c *Client) invalidate(tabPrefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k := range c.cache {
		if strings.HasPrefix(k, tabPrefix) {
			delete(c.cache, k)
		}
	}
}
