package google

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"spese/internal/core"

	gsheet "google.golang.org/api/sheets/v4"
)

// Column letters for the trailing id column on raw tabs. Expenses tab has
// columns A-H plus note (I); id sits at J. Incomes tab has columns A-G plus
// note (H); id sits at I.
const (
	expenseIDColumnLetter = "J"
	incomeIDColumnLetter  = "I"
)

// ErrMissingID is returned when an Upsert is called without a positive ID on the
// domain object. The id is required so writes are idempotent and can later be
// updated in place.
var ErrMissingID = errors.New("missing id; upsert requires e.ID > 0")

// ErrDuplicateRowID is returned when more than one sheet row carries the same
// id. The adapter refuses to write under ambiguity rather than guess which row
// to update.
var ErrDuplicateRowID = errors.New("duplicate id rows in sheet")

// findRowByID locates the 1-based row number whose id-column cell equals id.
// idCol is the values from a single column read (e.g. `Sheet!J:J`). The first
// row is treated as a header and skipped.
//
// Returns 0 when the id is not found. Returns ErrDuplicateRowID when the id
// appears more than once.
func findRowByID(idCol [][]any, id int64) (int, error) {
	want := strconv.FormatInt(id, 10)
	hit := 0
	// Row 1 is the header. Start at row 2.
	for i := 1; i < len(idCol); i++ {
		row := idCol[i]
		if len(row) == 0 {
			continue
		}
		cell := strings.TrimSpace(cellAsString(row[0]))
		if cell == "" {
			continue
		}
		if cell == want {
			if hit != 0 {
				return 0, fmt.Errorf("%w: id=%d (rows %d and %d)", ErrDuplicateRowID, id, hit, i+1)
			}
			hit = i + 1 // 1-based row number
		}
	}
	return hit, nil
}

// cellAsString normalizes a Sheets API cell value (which may be string,
// float64, or int64) to its string representation.
func cellAsString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		// Format without trailing zeros so 42.0 → "42".
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	default:
		return fmt.Sprint(v)
	}
}

// fetchIDColumn reads the id column for the given sheet+letter and returns the
// raw cell values (header included at index 0).
func (c *Client) fetchIDColumn(ctx context.Context, sheet, colLetter string) ([][]any, error) {
	rng := fmt.Sprintf("%s!%s:%s", sheet, colLetter, colLetter)
	resp, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rng).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("read id column %s: %w", rng, err)
	}
	return resp.Values, nil
}

// UpsertExpense locates the row whose id column matches e.ID and updates it
// in place; if no row matches, a new row is appended. Requires e.ID > 0.
func (c *Client) UpsertExpense(ctx context.Context, e core.Expense) (string, error) {
	if err := e.Validate(); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}
	if c.svc == nil {
		return "", errors.New("sheets service not initialized")
	}
	if e.ID <= 0 {
		return "", ErrMissingID
	}
	if err := c.checkRawTabLayout(ctx); err != nil {
		return "", err
	}

	idCol, err := c.fetchIDColumn(ctx, c.expensesSheet, expenseIDColumnLetter)
	if err != nil {
		return "", err
	}
	row, err := findRowByID(idCol, e.ID)
	if err != nil {
		return "", err
	}

	if row == 0 {
		next, err := c.getNextRow(ctx)
		if err != nil {
			return "", err
		}
		row = next
	}
	if err := c.writeExpenseRow(ctx, row, e); err != nil {
		c.InvalidateRowCache()
		return "", err
	}
	ref := fmt.Sprintf("%s!A%d:%s%d", c.expensesSheet, row, expenseIDColumnLetter, row)
	slog.InfoContext(ctx, "Upserted expense in Google Sheets",
		"sheet", c.expensesSheet, "row", row, "id", e.ID)
	return ref, nil
}

// writeExpenseRow updates A:D, G:H and J for the given row.
func (c *Client) writeExpenseRow(ctx context.Context, row int, e core.Expense) error {
	euros := float64(e.Amount.Cents) / 100.0
	r1 := fmt.Sprintf("%s!A%d:D%d", c.expensesSheet, row, row)
	v1 := &gsheet.ValueRange{Values: [][]any{{e.Date.Month(), e.Date.Day(), e.Description, euros}}}
	if _, err := c.svc.Spreadsheets.Values.Update(c.spreadsheetID, r1, v1).
		ValueInputOption("USER_ENTERED").Context(ctx).Do(); err != nil {
		return fmt.Errorf("update A:D row %d: %w", row, err)
	}
	r2 := fmt.Sprintf("%s!G%d:H%d", c.expensesSheet, row, row)
	v2 := &gsheet.ValueRange{Values: [][]any{{e.Primary, e.Secondary}}}
	if _, err := c.svc.Spreadsheets.Values.Update(c.spreadsheetID, r2, v2).
		ValueInputOption("USER_ENTERED").Context(ctx).Do(); err != nil {
		return fmt.Errorf("update G:H row %d: %w", row, err)
	}
	r3 := fmt.Sprintf("%s!%s%d", c.expensesSheet, expenseIDColumnLetter, row)
	v3 := &gsheet.ValueRange{Values: [][]any{{strconv.FormatInt(e.ID, 10)}}}
	if _, err := c.svc.Spreadsheets.Values.Update(c.spreadsheetID, r3, v3).
		ValueInputOption("RAW").Context(ctx).Do(); err != nil {
		return fmt.Errorf("update id col row %d: %w", row, err)
	}
	return nil
}

// UpsertIncome locates the row whose id column matches i.ID and updates it
// in place; if no row matches, a new row is appended. Requires i.ID > 0.
func (c *Client) UpsertIncome(ctx context.Context, i core.Income) (string, error) {
	if err := i.Validate(); err != nil {
		return "", fmt.Errorf("validation failed: %w", err)
	}
	if c.svc == nil {
		return "", errors.New("sheets service not initialized")
	}
	if c.incomeSheet == "" {
		return "", errors.New("income sheet not configured")
	}
	if i.ID <= 0 {
		return "", ErrMissingID
	}
	if err := c.checkRawTabLayout(ctx); err != nil {
		return "", err
	}

	idCol, err := c.fetchIDColumn(ctx, c.incomeSheet, incomeIDColumnLetter)
	if err != nil {
		return "", err
	}
	row, err := findRowByID(idCol, i.ID)
	if err != nil {
		return "", err
	}

	if row == 0 {
		next, err := c.getNextIncomeRow(ctx)
		if err != nil {
			return "", err
		}
		row = next
	}
	if err := c.writeIncomeRow(ctx, row, i); err != nil {
		c.invalidateIncomeRowCache()
		return "", err
	}
	ref := fmt.Sprintf("%s!A%d:%s%d", c.incomeSheet, row, incomeIDColumnLetter, row)
	slog.InfoContext(ctx, "Upserted income in Google Sheets",
		"sheet", c.incomeSheet, "row", row, "id", i.ID)
	return ref, nil
}

// writeIncomeRow updates A:D, G and I (id) for the given row.
func (c *Client) writeIncomeRow(ctx context.Context, row int, i core.Income) error {
	euros := float64(i.Amount.Cents) / 100.0
	r1 := fmt.Sprintf("%s!A%d:D%d", c.incomeSheet, row, row)
	v1 := &gsheet.ValueRange{Values: [][]any{{i.Date.Month(), i.Date.Day(), i.Description, euros}}}
	if _, err := c.svc.Spreadsheets.Values.Update(c.spreadsheetID, r1, v1).
		ValueInputOption("USER_ENTERED").Context(ctx).Do(); err != nil {
		return fmt.Errorf("update A:D row %d: %w", row, err)
	}
	r2 := fmt.Sprintf("%s!G%d", c.incomeSheet, row)
	v2 := &gsheet.ValueRange{Values: [][]any{{i.Category}}}
	if _, err := c.svc.Spreadsheets.Values.Update(c.spreadsheetID, r2, v2).
		ValueInputOption("USER_ENTERED").Context(ctx).Do(); err != nil {
		return fmt.Errorf("update G row %d: %w", row, err)
	}
	r3 := fmt.Sprintf("%s!%s%d", c.incomeSheet, incomeIDColumnLetter, row)
	v3 := &gsheet.ValueRange{Values: [][]any{{strconv.FormatInt(i.ID, 10)}}}
	if _, err := c.svc.Spreadsheets.Values.Update(c.spreadsheetID, r3, v3).
		ValueInputOption("RAW").Context(ctx).Do(); err != nil {
		return fmt.Errorf("update id col row %d: %w", row, err)
	}
	return nil
}
