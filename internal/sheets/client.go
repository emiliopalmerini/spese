// Package sheets is the shared low-level Google Sheets adapter.
// It exposes ReadRange / ReadTable / AppendRows with an ETag-validated read
// cache that is invalidated whenever a tab is written.
//
// Feature slices wrap this with their own typed parsers (parse a row into
// a Transaction, an Account, etc.).
package sheets

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// Client is a thin wrapper around the Google Sheets API for a single
// spreadsheet. Safe for concurrent use.
type Client struct {
	svc           *sheets.Service
	spreadsheetID string

	mu    sync.RWMutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	data [][]any
	etag string
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
		cache:         make(map[string]cacheEntry),
	}, nil
}

// SpreadsheetID returns the configured spreadsheet ID. Useful for building
// share links in UI.
func (c *Client) SpreadsheetID() string { return c.spreadsheetID }

// ReadRange returns raw cell values for an A1 range like "transactions!A2:I"
// or a bare tab name like "accounts". Cached results are revalidated with the
// range ETag on every read unless force is true.
func (c *Client) ReadRange(ctx context.Context, rangeA1 string, force bool) ([][]any, error) {
	var cached cacheEntry
	if !force {
		c.mu.RLock()
		var ok bool
		cached, ok = c.cache[rangeA1]
		c.mu.RUnlock()
		if !ok {
			cached = cacheEntry{}
		}
	}

	call := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rangeA1).Context(ctx)
	if cached.etag != "" {
		call = call.IfNoneMatch(cached.etag)
	}

	resp, err := call.Do()
	if err != nil {
		if googleapi.IsNotModified(err) {
			return cached.data, nil
		}
		if isMissingRange(err) {
			c.mu.Lock()
			c.cache[rangeA1] = cacheEntry{}
			c.mu.Unlock()
			return nil, nil
		}
		return nil, fmt.Errorf("sheets get %s: %w", rangeA1, err)
	}

	c.mu.Lock()
	c.cache[rangeA1] = cacheEntry{
		data: resp.Values,
		etag: resp.ServerResponse.Header.Get("ETag"),
	}
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

// ReplaceRows clears a tab and writes the supplied rows from A1. It is used by
// the SQLite-to-Sheets mirror, where the sheet is derived state.
func (c *Client) ReplaceRows(ctx context.Context, tab string, rows [][]any) error {
	_, err := c.svc.Spreadsheets.Values.Clear(c.spreadsheetID, tab, &sheets.ClearValuesRequest{}).
		Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("sheets clear %s: %w", tab, err)
	}
	if len(rows) > 0 {
		vr := &sheets.ValueRange{Values: rows}
		_, err = c.svc.Spreadsheets.Values.Update(c.spreadsheetID, tab+"!A1", vr).
			ValueInputOption("USER_ENTERED").
			Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("sheets update %s: %w", tab, err)
		}
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

// isMissingRange detects the Sheets 400 returned when a tab or A1 range
// does not exist in the spreadsheet. Callers treat it as empty data so a
// first-run spreadsheet renders blank pages instead of 5xx.
func isMissingRange(err error) bool {
	return err != nil && strings.Contains(err.Error(), "Unable to parse range")
}
