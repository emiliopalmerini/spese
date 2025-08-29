package google

import (
    "context"
    "errors"
    "fmt"
    "os"
    "strings"

    "spese/internal/core"
    ports "spese/internal/sheets"

    goption "google.golang.org/api/option"
    gsheet "google.golang.org/api/sheets/v4"
)

type Client struct {
    svc                 *gsheet.Service
    spreadsheetID       string
    expensesSheet       string
    categoriesSheet     string
    subcategoriesSheet  string
}

// Ensure interface conformance
var _ ports.ExpenseWriter = (*Client)(nil)
var _ ports.TaxonomyReader = (*Client)(nil)

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
        expenses = "Spese"
    }
    cats := os.Getenv("GOOGLE_CATEGORIES_SHEET_NAME")
    if cats == "" {
        cats = "Categories"
    }
    subs := os.Getenv("GOOGLE_SUBCATEGORIES_SHEET_NAME")
    if subs == "" {
        subs = "Subcategories"
    }

    var opts []goption.ClientOption
    if credJSON := strings.TrimSpace(os.Getenv("GOOGLE_CREDENTIALS_JSON")); credJSON != "" {
        opts = append(opts, goption.WithCredentialsJSON([]byte(credJSON)))
    } else if credFile := strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")); credFile != "" {
        opts = append(opts, goption.WithCredentialsFile(credFile))
    }
    opts = append(opts, goption.WithScopes(gsheet.SpreadsheetsScope))

    svc, err := gsheet.NewService(ctx, opts...)
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

func (c *Client) Append(ctx context.Context, e core.Expense) (string, error) {
    if err := e.Validate(); err != nil {
        return "", err
    }
    // Convert cents to decimal string
    euros := float64(e.Amount.Cents) / 100.0
    row := []interface{}{e.Date.Day, e.Date.Month, e.Description, euros, e.Category, e.Subcategory}
    vr := &gsheet.ValueRange{Values: [][]interface{}{row}}
    rng := fmt.Sprintf("%s!A:F", c.expensesSheet)
    call := c.svc.Spreadsheets.Values.Append(c.spreadsheetID, rng, vr).
        ValueInputOption("USER_ENTERED").
        InsertDataOption("INSERT_ROWS")
    resp, err := call.Context(ctx).Do()
    if err != nil {
        return "", fmt.Errorf("append: %w", err)
    }
    ref := ""
    if resp.Updates != nil && resp.Updates.UpdatedRange != "" {
        ref = resp.Updates.UpdatedRange
    }
    return ref, nil
}

func (c *Client) List(ctx context.Context) ([]string, []string, error) {
    cats, err := c.readCol(ctx, c.categoriesSheet, "A:A")
    if err != nil {
        return nil, nil, err
    }
    subs, err := c.readCol(ctx, c.subcategoriesSheet, "A:A")
    if err != nil {
        return nil, nil, err
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
    for i, row := range resp.Values {
        if i == 0 { // skip header
            continue
        }
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

