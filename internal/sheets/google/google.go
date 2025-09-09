package google

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"spese/internal/core"
	"strconv"
	"strings"
	"time"

	ports "spese/internal/sheets"

	goption "google.golang.org/api/option"
	gsheet "google.golang.org/api/sheets/v4"
)

type Client struct {
	svc                *gsheet.Service
	spreadsheetID      string
	expensesSheet      string
	categoriesSheet    string
	subcategoriesSheet string
	// Preferred: base name without year (e.g. "Dashboard"); code prefixes year.
	dashboardBase string
	// Legacy fallback: pattern or plain prefix (e.g. "%d Dashboard" or "Dashboard").
	dashboardPrefix string
}

// Ensure interface conformance
var (
	_ ports.ExpenseWriter   = (*Client)(nil)
	_ ports.TaxonomyReader  = (*Client)(nil)
	_ ports.DashboardReader = (*Client)(nil)
	_ ports.ExpenseLister   = (*Client)(nil)
	_ ports.ExpenseDeleter  = (*Client)(nil)
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

	// Base sheet names (without year). We will prefix the current year automatically.
	expensesBase := strings.TrimSpace(os.Getenv("GOOGLE_SHEET_NAME"))
	if expensesBase == "" {
		expensesBase = "Expenses"
	}
	catsBase := strings.TrimSpace(os.Getenv("GOOGLE_CATEGORIES_SHEET_NAME"))
	if catsBase == "" {
		catsBase = "Dashboard"
	}
	subsBase := strings.TrimSpace(os.Getenv("GOOGLE_SUBCATEGORIES_SHEET_NAME"))
	if subsBase == "" {
		subsBase = "Dashboard"
	}

	svc, err := newSheetsService(ctx)
	if err != nil {
		return nil, fmt.Errorf("sheets service: %w", err)
	}

	// Dashboard naming: prefer a base name (without year). Legacy prefix is supported.
	dashBase := strings.TrimSpace(os.Getenv("DASHBOARD_SHEET_NAME"))
	dashPrefix := strings.TrimSpace(os.Getenv("DASHBOARD_SHEET_PREFIX"))
	if dashBase == "" && dashPrefix == "" {
		// Default to base name when nothing provided
		dashBase = "Dashboard"
	}

	// Compute year-prefixed names for this client instance
	currentYear := time.Now().Year()
	expenses := yearPrefixedName(expensesBase, currentYear)
	cats := yearPrefixedName(catsBase, currentYear)
	subs := yearPrefixedName(subsBase, currentYear)

	return &Client{
		svc:                svc,
		spreadsheetID:      spreadsheetID,
		expensesSheet:      expenses,
		categoriesSheet:    cats,
		subcategoriesSheet: subs,
		dashboardBase:      dashBase,
		dashboardPrefix:    dashPrefix,
	}, nil
}

// newSheetsService initializes a Sheets Service using Service Account credentials.
// Uses GOOGLE_SERVICE_ACCOUNT_JSON, GOOGLE_SERVICE_ACCOUNT_FILE, or GOOGLE_APPLICATION_CREDENTIALS.
func newSheetsService(ctx context.Context) (*gsheet.Service, error) {
	serviceAccountJSON := strings.TrimSpace(os.Getenv("GOOGLE_SERVICE_ACCOUNT_JSON"))
	serviceAccountFile := strings.TrimSpace(os.Getenv("GOOGLE_SERVICE_ACCOUNT_FILE"))
	
	slog.InfoContext(ctx, "Checking Service Account environment variables",
		"has_json", serviceAccountJSON != "",
		"file_path", serviceAccountFile,
		"json_length", len(serviceAccountJSON))
	
	// Also check the standard Google Cloud environment variable
	if serviceAccountJSON == "" && serviceAccountFile == "" {
		serviceAccountFile = strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"))
		slog.InfoContext(ctx, "Checking GOOGLE_APPLICATION_CREDENTIALS", "path", serviceAccountFile)
	}

	var credentialsJSON []byte
	var err error

	switch {
	case serviceAccountJSON != "":
		slog.InfoContext(ctx, "Using inline JSON credentials")
		credentialsJSON = []byte(serviceAccountJSON)
	case serviceAccountFile != "":
		slog.InfoContext(ctx, "Reading credentials from file", "path", serviceAccountFile)
		credentialsJSON, err = os.ReadFile(serviceAccountFile)
		if err != nil {
			return nil, fmt.Errorf("read service account file: %w", err)
		}
		slog.InfoContext(ctx, "Successfully read credentials file", "size", len(credentialsJSON))
	default:
		return nil, errors.New("missing service account credentials (set GOOGLE_SERVICE_ACCOUNT_JSON, GOOGLE_SERVICE_ACCOUNT_FILE, or GOOGLE_APPLICATION_CREDENTIALS)")
	}

	// Create service using service account credentials
	slog.InfoContext(ctx, "Creating Google Sheets service with Service Account",
		"credentials_size", len(credentialsJSON),
		"scope", gsheet.SpreadsheetsScope)
	
	// Try without custom HTTP client first to debug
	service, err := gsheet.NewService(ctx, 
		goption.WithCredentialsJSON(credentialsJSON),
		goption.WithScopes(gsheet.SpreadsheetsScope))
	
	if err != nil {
		return nil, fmt.Errorf("create sheets service: %w", err)
	}
	
	slog.InfoContext(ctx, "Google Sheets service created successfully")
	return service, nil
}

// newHTTPClientWithPooling creates an HTTP client optimized for Google Sheets API
// with connection pooling, proper timeouts, and keep-alive settings
func newHTTPClientWithPooling() *http.Client {
	// Create a custom dialer with timeouts
	dialer := &net.Dialer{
		Timeout:   30 * time.Second, // TCP connection timeout
		KeepAlive: 30 * time.Second, // Keep-alive probe interval
	}

	transport := &http.Transport{
		// Use custom dialer
		DialContext: dialer.DialContext,

		// Connection pooling settings
		MaxIdleConns:        100,              // Total max idle connections across all hosts
		MaxIdleConnsPerHost: 10,               // Max idle connections per host (Google APIs)
		MaxConnsPerHost:     50,               // Max total connections per host
		IdleConnTimeout:     90 * time.Second, // How long idle connections stay open

		// TLS and response timeouts
		TLSHandshakeTimeout:   10 * time.Second, // TLS handshake timeout
		ResponseHeaderTimeout: 30 * time.Second, // Time to read response headers
		ExpectContinueTimeout: 1 * time.Second,  // 100-continue timeout

		// Keep-alive and HTTP/2 settings
		DisableKeepAlives: false, // Enable HTTP keep-alive
		ForceAttemptHTTP2: true,  // Prefer HTTP/2
	}

	return &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second, // Overall request timeout
	}
}


func (c *Client) Append(ctx context.Context, e core.Expense) (string, error) {
	if err := e.Validate(); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}
	if c.svc == nil {
		return "", errors.New("sheets service not initialized")
	}

	// Convert cents to decimal string
	euros := float64(e.Amount.Cents) / 100.0

	// Find the next empty row by getting the sheet dimensions first
	rng := fmt.Sprintf("%s!A:A", c.expensesSheet)
	resp, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rng).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("failed to get sheet dimensions for %s: %w", c.expensesSheet, err)
	}

	// Calculate next row (number of existing rows + 1)
	nextRow := len(resp.Values) + 1

	// Update only the specific columns we want, skipping E and F
	// Update A:D (Month, Day, Description, Amount)
	dataRange1 := fmt.Sprintf("%s!A%d:D%d", c.expensesSheet, nextRow, nextRow)
	vr1 := &gsheet.ValueRange{Values: [][]any{{e.Date.Month, e.Date.Day, e.Description, euros}}}

	_, err = c.svc.Spreadsheets.Values.Update(c.spreadsheetID, dataRange1, vr1).
		ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("failed to update A:D in sheet %s: %w", c.expensesSheet, err)
	}

	// Update G:H (Primary, Secondary categories)
	dataRange2 := fmt.Sprintf("%s!G%d:H%d", c.expensesSheet, nextRow, nextRow)
	vr2 := &gsheet.ValueRange{Values: [][]any{{e.Primary, e.Secondary}}}

	_, err = c.svc.Spreadsheets.Values.Update(c.spreadsheetID, dataRange2, vr2).
		ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("failed to update G:H in sheet %s: %w", c.expensesSheet, err)
	}

	// Return reference in the format expected by callers
	ref := fmt.Sprintf("%s!A%d:H%d", c.expensesSheet, nextRow, nextRow)

	return ref, nil
}

func (c *Client) List(ctx context.Context) ([]string, []string, error) {
	if c.svc == nil {
		return nil, nil, errors.New("sheets service not initialized")
	}

	cats, err := c.readCol(ctx, c.categoriesSheet, "A3:A65")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read categories: %w", err)
	}
	subs, err := c.readCol(ctx, c.subcategoriesSheet, "B3:B65")
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
	rng := fmt.Sprintf("%s!A2:Q67", sheetName)
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
		slog.WarnContext(ctx, "Dashboard header mismatch, falling back to expenses sheet", "year", year, "month", month, "sheet", sheetName, "range", rng, "error", err)
		return c.readMonthOverviewFromExpenses(ctx, year, month)
	}
	return core.MonthOverview{}, err
}

func (c *Client) dashboardSheetName(year int) string {
	// Preferred: base name present => "<year> <base>"
	if strings.TrimSpace(c.dashboardBase) != "" {
		return yearPrefixedName(c.dashboardBase, year)
	}
	// Legacy: if a printf-style pattern is provided, format it
	if strings.Contains(c.dashboardPrefix, "%d") {
		return fmt.Sprintf(c.dashboardPrefix, year)
	}
	// Legacy: treat prefix as a plain name and append year
	return strings.TrimSpace(fmt.Sprintf("%s %d", c.dashboardPrefix, year))
}

func toStrings(in []interface{}) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[i] = strings.TrimSpace(fmt.Sprint(v))
	}
	return out
}

// yearPrefixedName returns "<year> <base>" unless base already starts with a 4-digit year.
func yearPrefixedName(base string, year int) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return base
	}
	if len(base) >= 5 {
		if y, err := strconv.Atoi(base[0:4]); err == nil && base[4] == ' ' && y > 1900 && y < 3000 {
			return base
		}
	}
	return fmt.Sprintf("%d %s", year, base)
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

// ListExpenses lists raw expenses for the given year and month by scanning the expenses sheet.
func (c *Client) ListExpenses(ctx context.Context, year int, month int) ([]core.Expense, error) {
	if c.svc == nil {
		return nil, errors.New("sheets service not initialized")
	}
	if month < 1 || month > 12 {
		return nil, fmt.Errorf("invalid month: %d", month)
	}
	rng := fmt.Sprintf("%s!A:H", c.expensesSheet)
	resp, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rng).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rng, err)
	}
	var out []core.Expense
	for i, row := range resp.Values {
		cols := toStrings(row)
		if len(cols) < 7 {
			continue
		}
		// Skip likely header row if first row has non-numeric month
		if i == 0 {
			if _, err := strconv.Atoi(strings.TrimSpace(cols[0])); err != nil {
				continue
			}
		}
		m, err := strconv.Atoi(strings.TrimSpace(cols[0]))
		if err != nil || m != month {
			continue
		}
		day, _ := strconv.Atoi(strings.TrimSpace(cols[1]))
		desc := strings.TrimSpace(cols[2])
		cents, ok := parseEurosToCents(cols[3])
		if !ok {
			// Try simple float parsing
			if f, ferr := strconv.ParseFloat(strings.TrimSpace(cols[3]), 64); ferr == nil {
				cents = int64((f * 100.0) + 0.5)
				ok = true
			}
		}
		if !ok {
			continue
		}
		primary := strings.TrimSpace(cols[6])
		secondary := ""
		if len(cols) >= 8 {
			secondary = strings.TrimSpace(cols[7])
		}
		e := core.Expense{
			Date:        core.DateParts{Day: day, Month: month},
			Description: desc,
			Amount:      core.Money{Cents: cents},
			Primary:     primary,
			Secondary:   secondary,
		}
		// Do not enforce validation strictly here; list is best-effort. Filter obviously empty rows.
		if strings.TrimSpace(e.Description) == "" && e.Amount.Cents == 0 {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// DeleteExpense implements ports.ExpenseDeleter
func (c *Client) DeleteExpense(ctx context.Context, id string) error {
	// For Google Sheets, we need to find the row by expense ID and delete it
	// Since Google Sheets doesn't have a natural ID system like databases,
	// we'll need to search for the expense by its properties
	// For now, return an error indicating that deletion via Google Sheets
	// should be handled via AMQP sync messages to maintain data consistency
	return fmt.Errorf("direct Google Sheets deletion not supported - use AMQP sync messages")
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
