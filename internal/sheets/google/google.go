package google

import (
    "context"
    "encoding/json"
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
    // Preferred: base name without year (e.g. "Dashboard"); code prefixes year.
    dashboardBase      string
    // Legacy fallback: pattern or plain prefix (e.g. "%d Dashboard" or "Dashboard").
    dashboardPrefix    string
}

// Ensure interface conformance
var (
    _ ports.ExpenseWriter   = (*Client)(nil)
    _ ports.TaxonomyReader  = (*Client)(nil)
    _ ports.DashboardReader = (*Client)(nil)
    _ ports.ExpenseLister   = (*Client)(nil)
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

	// Create HTTP client with connection pooling and proper timeouts
	baseClient := newHTTPClientWithPooling()
	httpClient := cfg.Client(ctx, tok)
	
	// Replace the transport with our optimized one while preserving OAuth
	if transport, ok := httpClient.Transport.(*oauth2.Transport); ok {
		transport.Base = baseClient.Transport
	}
	
	return gsheet.NewService(ctx, goption.WithHTTPClient(httpClient))
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
	// Structure: Month, Day, Expense, Amount, [E skip], [F skip], Primary, Secondary
	// Use null values for E and F to preserve existing formulas/data
	row := []any{e.Date.Month, e.Date.Day, e.Description, euros, nil, nil, e.Primary, e.Secondary}
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
