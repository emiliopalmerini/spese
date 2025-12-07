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
	"sync"
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

	// Row count cache for performance (avoids repeated read requests)
	mu                 sync.Mutex
	cachedRowCount     int
	cacheExpiresAt     time.Time
	cacheValidDuration time.Duration
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
		cacheValidDuration: 2 * time.Minute, // Cache row count for 2 minutes to reduce API calls
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

// getNextRow returns the next available row number, using cached row count when valid
// If cache is expired or this is the first call, it reads column A from the sheet
func (c *Client) getNextRow(ctx context.Context) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if cache is still valid
	if time.Now().Before(c.cacheExpiresAt) && c.cachedRowCount > 0 {
		slog.DebugContext(ctx, "Using cached row count",
			"cached_row_count", c.cachedRowCount,
			"expires_in", time.Until(c.cacheExpiresAt).Round(time.Second))
		return c.cachedRowCount + 1, nil
	}

	// Cache miss or expired: read from sheet
	slog.InfoContext(ctx, "Row count cache expired or invalid, refreshing from sheet",
		"cached_row_count", c.cachedRowCount,
		"expires_at", c.cacheExpiresAt.Format(time.RFC3339))

	rng := fmt.Sprintf("%s!A:A", c.expensesSheet)
	resp, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rng).Context(ctx).Do()
	if err != nil {
		return 0, fmt.Errorf("failed to get sheet dimensions for %s: %w", c.expensesSheet, err)
	}

	// Update cache
	c.cachedRowCount = len(resp.Values)
	c.cacheExpiresAt = time.Now().Add(c.cacheValidDuration)

	nextRow := c.cachedRowCount + 1

	slog.InfoContext(ctx, "Updated row count cache",
		"row_count", c.cachedRowCount,
		"next_row", nextRow,
		"cache_expires_at", c.cacheExpiresAt.Format(time.RFC3339))

	return nextRow, nil
}

// InvalidateRowCache clears the cached row count (called after successful appends)
func (c *Client) InvalidateRowCache() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cacheExpiresAt = time.Now() // Expire cache immediately
	slog.DebugContext(context.Background(), "Row count cache invalidated")
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

	// Get next row using cached row count (reduces API calls significantly)
	nextRow, err := c.getNextRow(ctx)
	if err != nil {
		return "", err
	}

	// Update only the specific columns we want, skipping E and F
	// Update A:D (Month, Day, Description, Amount)
	dataRange1 := fmt.Sprintf("%s!A%d:D%d", c.expensesSheet, nextRow, nextRow)
	vr1 := &gsheet.ValueRange{Values: [][]any{{e.Date.Month(), e.Date.Day(), e.Description, euros}}}

	_, err = c.svc.Spreadsheets.Values.Update(c.spreadsheetID, dataRange1, vr1).
		ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		// Invalidate cache on write failure in case row was actually written
		c.InvalidateRowCache()
		return "", fmt.Errorf("failed to update A:D in sheet %s: %w", c.expensesSheet, err)
	}

	// Update G:H (Primary, Secondary categories)
	dataRange2 := fmt.Sprintf("%s!G%d:H%d", c.expensesSheet, nextRow, nextRow)
	vr2 := &gsheet.ValueRange{Values: [][]any{{e.Primary, e.Secondary}}}

	_, err = c.svc.Spreadsheets.Values.Update(c.spreadsheetID, dataRange2, vr2).
		ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		// Invalidate cache on write failure
		c.InvalidateRowCache()
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
			Date:        core.NewDate(time.Now().Year(), month, day),
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
	// For Google Sheets, ID-based deletion is not supported since we need expense data to find the row
	// This should only be called by non-Google Sheets adapters
	return fmt.Errorf("Google Sheets deletion requires expense data, use DeleteExpenseByData method instead")
}

// DeleteExpenseByData provides expense deletion using expense data for Google Sheets
func (c *Client) DeleteExpenseByData(ctx context.Context, expenseData core.Expense) error {
	if c.svc == nil {
		return errors.New("sheets service not initialized")
	}

	// Validate expense data
	if err := expenseData.Validate(); err != nil {
		return fmt.Errorf("invalid expense data for deletion: %w", err)
	}

	// Read all data from the expenses sheet
	rng := fmt.Sprintf("%s!A:H", c.expensesSheet)
	resp, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rng).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to read expenses sheet %s: %w", c.expensesSheet, err)
	}

	// Find the row that matches the expense data
	var targetRow int = -1
	var matchingRows []int
	for i, row := range resp.Values {
		if len(row) < 7 { // Need at least A-G columns
			continue
		}
		
		cols := toStrings(row)
		
		// Skip header rows (first row if it contains non-numeric month)
		if i == 0 {
			if _, err := strconv.Atoi(strings.TrimSpace(cols[0])); err != nil {
				continue
			}
		}

		// Match month (column A)
		month, err := strconv.Atoi(strings.TrimSpace(cols[0]))
		if err != nil || month != expenseData.Date.Month() {
			continue
		}

		// Match day (column B)
		day, err := strconv.Atoi(strings.TrimSpace(cols[1]))
		if err != nil || day != expenseData.Date.Day() {
			continue
		}

		// Match description (column C) - handle timestamped descriptions
		description := strings.TrimSpace(cols[2])
		// Google Sheets will have timestamp added by worker: "Original Description [ts:1234567890]"
		// So we need to check if the Google Sheets description starts with our expense description
		if !strings.HasPrefix(description, expenseData.Description) {
			continue
		}

		// Match amount (column D) - convert to cents for comparison
		cents, ok := parseEurosToCents(cols[3])
		if !ok {
			// Try simple float parsing as fallback
			if f, ferr := strconv.ParseFloat(strings.TrimSpace(cols[3]), 64); ferr == nil {
				cents = int64((f * 100.0) + 0.5)
				ok = true
			}
		}
		if !ok || cents != expenseData.Amount.Cents {
			continue
		}

		// Match primary category (column G)
		primary := strings.TrimSpace(cols[6])
		if primary != expenseData.Primary {
			continue
		}

		// Match secondary category (column H) if present
		if len(cols) >= 8 {
			secondary := strings.TrimSpace(cols[7])
			if secondary != expenseData.Secondary {
				continue
			}
		} else if expenseData.Secondary != "" {
			// Row doesn't have secondary category but expense data does
			continue
		}

		// Found a matching row
		rowIndex := i + 1 // Convert to 1-based indexing for Google Sheets API
		matchingRows = append(matchingRows, rowIndex)
		if targetRow == -1 {
			targetRow = rowIndex // Use the first match
		}
	}

	// Check for multiple matches
	if len(matchingRows) > 1 {
		slog.WarnContext(ctx, "Multiple matching rows found for expense deletion",
			"sheet", c.expensesSheet,
			"matching_rows", matchingRows,
			"using_row", targetRow,
			"expense", map[string]interface{}{
				"month": expenseData.Date.Month,
				"day": expenseData.Date.Day,
				"description": expenseData.Description,
				"amount": float64(expenseData.Amount.Cents)/100.0,
				"primary": expenseData.Primary,
				"secondary": expenseData.Secondary,
			})
		// With timestamped descriptions, we should ideally have only one match
		// But we'll proceed with the first match and log the issue
	}

	if targetRow == -1 {
		// Log the search details for debugging
		slog.WarnContext(ctx, "Expense not found in Google Sheets for deletion",
			"sheet", c.expensesSheet,
			"month", expenseData.Date.Month,
			"day", expenseData.Date.Day,
			"description", expenseData.Description,
			"amount", float64(expenseData.Amount.Cents)/100.0,
			"primary", expenseData.Primary,
			"secondary", expenseData.Secondary,
			"total_rows_scanned", len(resp.Values))
		
		return fmt.Errorf("expense not found in Google Sheets: month=%d day=%d description=%s amount=%.2f primary=%s secondary=%s",
		expenseData.Date.Month(), expenseData.Date.Day(), expenseData.Description,
		float64(expenseData.Amount.Cents)/100.0, expenseData.Primary, expenseData.Secondary)
	}

	// Get the sheet ID for the batchUpdate API
	sheetId := c.getSheetId(ctx, c.expensesSheet)
	if sheetId == 0 {
		return fmt.Errorf("could not determine sheet ID for %s", c.expensesSheet)
	}

	// Delete the found row using the batchUpdate API
	deleteRequest := &gsheet.BatchUpdateSpreadsheetRequest{
		Requests: []*gsheet.Request{
			{
				DeleteDimension: &gsheet.DeleteDimensionRequest{
					Range: &gsheet.DimensionRange{
						SheetId:    sheetId,
						Dimension:  "ROWS",
						StartIndex: int64(targetRow - 1), // Convert back to 0-based for API
						EndIndex:   int64(targetRow),     // Exclusive end, so this deletes just one row
					},
				},
			},
		},
	}

	_, err = c.svc.Spreadsheets.BatchUpdate(c.spreadsheetID, deleteRequest).Context(ctx).Do()
	if err != nil {
		slog.ErrorContext(ctx, "Google Sheets API delete request failed",
			"sheet", c.expensesSheet,
			"sheet_id", sheetId,
			"target_row", targetRow,
			"spreadsheet_id", c.spreadsheetID,
			"error", err)
		return fmt.Errorf("failed to delete row %d from sheet %s: %w", targetRow, c.expensesSheet, err)
	}

	slog.InfoContext(ctx, "Successfully deleted expense from Google Sheets",
		"sheet", c.expensesSheet,
		"row", targetRow,
		"month", expenseData.Date.Month,
		"day", expenseData.Date.Day,
		"description", expenseData.Description,
		"amount", float64(expenseData.Amount.Cents)/100.0)

	return nil
}

// getSheetId retrieves the sheet ID for a given sheet name
func (c *Client) getSheetId(ctx context.Context, sheetName string) int64 {
	// Get spreadsheet metadata to find the sheet ID
	spreadsheet, err := c.svc.Spreadsheets.Get(c.spreadsheetID).Context(ctx).Do()
	if err != nil {
		slog.WarnContext(ctx, "Failed to get spreadsheet metadata for sheet ID", "error", err, "sheet", sheetName)
		return 0
	}

	for _, sheet := range spreadsheet.Sheets {
		if sheet.Properties.Title == sheetName {
			return int64(sheet.Properties.SheetId)
		}
	}

	slog.WarnContext(ctx, "Sheet ID not found", "sheet", sheetName)
	return 0
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
