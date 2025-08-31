package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"spese/internal/core"
	"strings"

	ports "spese/internal/sheets"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	goption "google.golang.org/api/option"
	gsheet "google.golang.org/api/sheets/v4"
)

type Client struct {
	svc                *gsheet.Service
	spreadsheetID      string
	expensesSheet      string
	categoriesSheet    string
	subcategoriesSheet string
}

// Ensure interface conformance
var (
	_ ports.ExpenseWriter  = (*Client)(nil)
	_ ports.TaxonomyReader = (*Client)(nil)
)

// NewFromEnv creates a Sheets client using environment variables and ADC.
// Required: GOOGLE_SPREADSHEET_ID
// Optional: GOOGLE_CREDENTIALS_JSON or GOOGLE_APPLICATION_CREDENTIALS for auth
// Optional sheet names: GOOGLE_SHEET_NAME (default "Spese"),
// GOOGLE_CATEGORIES_SHEET_NAME (default "Categories"),
// GOOGLE_SUBCATEGORIES_SHEET_NAME (default "Subcategories").
func NewFromEnv(ctx context.Context) (*Client, error) {
	spreadsheetID := strings.TrimSpace(os.Getenv("GOOGLE_SPREADSHEET_ID"))
	if spreadsheetID == "" {
		return nil, errors.New("missing GOOGLE_SPREADSHEET_ID")
	}

	expenses := os.Getenv("GOOGLE_SHEET_NAME")
	if expenses == "" {
		expenses = "2025 Expenses"
	}
	cats := os.Getenv("GOOGLE_CATEGORIES_SHEET_NAME")
	if cats == "" {
		cats = "2025 Dashboard"
	}
	subs := os.Getenv("GOOGLE_SUBCATEGORIES_SHEET_NAME")
	if subs == "" {
		subs = "2025 Dashboard"
	}

	svc, err := newSheetsService(ctx)
	if err != nil {
		return nil, fmt.Errorf("sheets service: %w", err)
	}

	return &Client{
		svc:                svc,
		spreadsheetID:      spreadsheetID,
		expensesSheet:      expenses,
		categoriesSheet:    cats,
		subcategoriesSheet: subs,
	}, nil
}

// newSheetsService initializes a Sheets Service using either OAuth (user credentials)
// or Service Account credentials. Preference order:
//  1. OAuth: GOOGLE_OAUTH_CLIENT_JSON or GOOGLE_OAUTH_CLIENT_FILE combined with
//     GOOGLE_OAUTH_TOKEN_JSON or GOOGLE_OAUTH_TOKEN_FILE.
//  2. Service Account: GOOGLE_CREDENTIALS_JSON or GOOGLE_APPLICATION_CREDENTIALS.
func newSheetsService(ctx context.Context) (*gsheet.Service, error) {
	// OAuth only: require client + token
	clientJSON := strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_CLIENT_JSON"))
	clientFile := strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_CLIENT_FILE"))
	tokenJSON := strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_TOKEN_JSON"))
	tokenFile := strings.TrimSpace(os.Getenv("GOOGLE_OAUTH_TOKEN_FILE"))

	var b []byte
	var err error
	switch {
	case clientJSON != "":
		b = []byte(clientJSON)
	case clientFile != "":
		b, err = os.ReadFile(clientFile)
		if err != nil {
			return nil, fmt.Errorf("read oauth client file: %w", err)
		}
	default:
		return nil, errors.New("missing oauth client (set GOOGLE_OAUTH_CLIENT_JSON or GOOGLE_OAUTH_CLIENT_FILE)")
	}

	cfg, err := google.ConfigFromJSON(b, gsheet.SpreadsheetsScope)
	if err != nil {
		return nil, fmt.Errorf("oauth config: %w", err)
	}

	var tok *oauth2.Token
	switch {
	case tokenJSON != "":
		tok = &oauth2.Token{}
		if err := jsonUnmarshal([]byte(tokenJSON), tok); err != nil {
			return nil, fmt.Errorf("oauth token json: %w", err)
		}
	case tokenFile != "":
		data, err := os.ReadFile(tokenFile)
		if err != nil {
			return nil, fmt.Errorf("read oauth token file: %w", err)
		}
		tok = &oauth2.Token{}
		if err := jsonUnmarshal(data, tok); err != nil {
			return nil, fmt.Errorf("oauth token file: %w", err)
		}
	default:
		return nil, errors.New("missing oauth token (set GOOGLE_OAUTH_TOKEN_JSON or GOOGLE_OAUTH_TOKEN_FILE)")
	}

	httpClient := cfg.Client(ctx, tok)
	return gsheet.NewService(ctx, goption.WithHTTPClient(httpClient))
}

// jsonUnmarshal is a tiny indirection to allow testing if needed.
var jsonUnmarshal = func(b []byte, v any) error { return json.Unmarshal(b, v) }

func (c *Client) Append(ctx context.Context, e core.Expense) (string, error) {
	if err := e.Validate(); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}
	if c.svc == nil {
		return "", errors.New("sheets service not initialized")
	}
	
    // Convert cents to decimal string
    euros := float64(e.Amount.Cents) / 100.0
    // Structure: Month, Day, Expense, Amount, [E blank], [F blank], Primary, Secondary
    // Leave E and F empty to allow sheet formulas/autofill to manage them.
    row := []any{e.Date.Month, e.Date.Day, e.Description, euros, "", "", e.Primary, e.Secondary}
	vr := &gsheet.ValueRange{Values: [][]any{row}}
	rng := fmt.Sprintf("%s!A:H", c.expensesSheet)
	
	call := c.svc.Spreadsheets.Values.Append(c.spreadsheetID, rng, vr).
		ValueInputOption("USER_ENTERED").
		InsertDataOption("INSERT_ROWS")
	
	resp, err := call.Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("failed to append to sheet %s: %w", c.expensesSheet, err)
	}
	
	ref := ""
	if resp.Updates != nil && resp.Updates.UpdatedRange != "" {
		ref = resp.Updates.UpdatedRange
	}
	return ref, nil
}

func (c *Client) List(ctx context.Context) ([]string, []string, error) {
	if c.svc == nil {
		return nil, nil, errors.New("sheets service not initialized")
	}
	
	cats, err := c.readCol(ctx, c.categoriesSheet, "A2:A65")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read categories: %w", err)
	}
	subs, err := c.readCol(ctx, c.subcategoriesSheet, "B2:B65")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read subcategories: %w", err)
	}
	return cats, subs, nil
}

func (c *Client) readCol(ctx context.Context, sheetName, col string) ([]string, error) {
	rng := fmt.Sprintf("%s!%s", sheetName, col)
	resp, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rng).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rng, err)
	}
	var out []string
	for _, row := range resp.Values {
		if len(row) == 0 {
			continue
		}
		v := strings.TrimSpace(fmt.Sprint(row[0]))
		if v == "" || strings.HasPrefix(v, "#") {
			continue
		}
		out = append(out, v)
	}
	// Dedup while preserving order
	seen := map[string]struct{}{}
	uniq := make([]string, 0, len(out))
	for _, v := range out {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		uniq = append(uniq, v)
	}
	return uniq, nil
}
