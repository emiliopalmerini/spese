package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"spese/internal/core"
	"strconv"
	"strings"
	"time"

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
	dashboardPrefix    string
}

// Ensure interface conformance
var (
	_ ports.ExpenseWriter   = (*Client)(nil)
	_ ports.TaxonomyReader  = (*Client)(nil)
	_ ports.DashboardReader = (*Client)(nil)
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

	dashPrefix := os.Getenv("DASHBOARD_SHEET_PREFIX")
	if strings.TrimSpace(dashPrefix) == "" {
		dashPrefix = "%d Dashboard"
	}

	return &Client{
		svc:                svc,
		spreadsheetID:      spreadsheetID,
		expensesSheet:      expenses,
		categoriesSheet:    cats,
		subcategoriesSheet: subs,
		dashboardPrefix:    dashPrefix,
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

	// Fail fast with a helpful error if the token is already expired and
	// there is no refresh token available to auto-refresh.
	if !tok.Expiry.IsZero() && tok.Expiry.Before(time.Now()) && strings.TrimSpace(tok.RefreshToken) == "" {
		return nil, fmt.Errorf("oauth token expired and missing refresh_token; re-run 'make oauth-init' to generate a new token (with offline access)")
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

// ReadMonthOverview reads the dashboard sheet for the given year and month
// and extracts totals by primary category and the grand total for that month.
func (c *Client) ReadMonthOverview(ctx context.Context, year int, month int) (core.MonthOverview, error) {
	if c.svc == nil {
		return core.MonthOverview{}, errors.New("sheets service not initialized")
	}
	if month < 1 || month > 12 {
		return core.MonthOverview{}, fmt.Errorf("invalid month: %d", month)
	}
	sheetName := c.dashboardSheetName(year)
	rng := fmt.Sprintf("%s!A2:Q300", sheetName)
	resp, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rng).Context(ctx).Do()
	if err != nil {
		return core.MonthOverview{}, fmt.Errorf("read %s: %w", rng, err)
	}
	if len(resp.Values) == 0 {
		return core.MonthOverview{Year: year, Month: month}, nil
	}
	ov, err := parseDashboard(resp.Values, year, month)
	if err == nil {
		return ov, nil
	}
	// Fallback: if the dashboard header/layout is unexpected, compute the
	// month overview directly by scanning the expenses sheet. This aligns
	// with ADR-0004 for robustness to header changes.
	if strings.Contains(strings.ToLower(err.Error()), "unexpected dashboard header") {
		return c.readMonthOverviewFromExpenses(ctx, year, month)
	}
	return core.MonthOverview{}, err
}

func (c *Client) dashboardSheetName(year int) string {
	if strings.Contains(c.dashboardPrefix, "%d") {
		return fmt.Sprintf(c.dashboardPrefix, year)
	}
	// If prefix doesn’t contain %d, append year at the end with a space
	return strings.TrimSpace(fmt.Sprintf("%s %d", c.dashboardPrefix, year))
}

func toStrings(in []interface{}) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = strings.TrimSpace(fmt.Sprint(v))
	}
	return out
}

// readMonthOverviewFromExpenses scans the expenses sheet for the given month and
// aggregates totals by primary category. Year is inferred by the sheet name and
// only used for the returned struct.
func (c *Client) readMonthOverviewFromExpenses(ctx context.Context, year int, month int) (core.MonthOverview, error) {
	rng := fmt.Sprintf("%s!A:H", c.expensesSheet)
	resp, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rng).Context(ctx).Do()
	if err != nil {
		return core.MonthOverview{}, fmt.Errorf("read %s: %w", rng, err)
	}
	byCat := map[string]int64{}
	order := make([]string, 0)
	var total int64
	for _, row := range resp.Values {
		cols := toStrings(row)
		if len(cols) < 7 {
			// Need at least Month, Day, Desc, Amount, E, F, Primary
			continue
		}
		// Parse month in col A (index 0). Skip header/non-numeric rows.
		m, err := strconv.Atoi(strings.TrimSpace(cols[0]))
		if err != nil || m != month {
			continue
		}
		// Amount in col D (index 3) can come as number or string
		cents, ok := parseEurosToCents(cols[3])
		if !ok {
			// Try fallback for numbers formatted without decimal separator
			if f, ferr := strconv.ParseFloat(strings.TrimSpace(cols[3]), 64); ferr == nil {
				cents = int64((f * 100.0) + 0.5)
				ok = true
			}
		}
		if !ok {
			continue
		}
		primary := strings.TrimSpace(cols[6])
		if primary == "" {
			primary = "(Senza categoria)"
		}
		if _, seen := byCat[primary]; !seen {
			order = append(order, primary)
		}
		byCat[primary] += cents
		total += cents
	}
	// Build list preserving first-seen order
	list := make([]core.CategoryAmount, 0, len(byCat))
	for _, name := range order {
		list = append(list, core.CategoryAmount{Name: name, Amount: core.Money{Cents: byCat[name]}})
	}
	// Append any remaining categories (shouldn't happen unless order tracking missed)
	for name, amt := range byCat {
		found := false
		for _, n := range order {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			list = append(list, core.CategoryAmount{Name: name, Amount: core.Money{Cents: amt}})
		}
	}
	return core.MonthOverview{Year: year, Month: month, Total: core.Money{Cents: total}, ByCategory: list}, nil
}

func indexOf(arr []string, target string) int {
	for i, v := range arr {
		if strings.EqualFold(strings.TrimSpace(v), strings.TrimSpace(target)) {
			return i
		}
	}
	return -1
}

func safeGet(arr []string, idx int) string {
	if idx < 0 || idx >= len(arr) {
		return ""
	}
	return arr[idx]
}

func parseEurosToCents(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	// Normalize decimal comma
	s = strings.ReplaceAll(s, ",", ".")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	cents := int64((f * 100.0) + 0.5)
	return cents, true
}
