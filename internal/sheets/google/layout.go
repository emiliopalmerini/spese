package google

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ExpectedExpenseHeader is the column order the adapter writes against on the
// `YYYY Expenses` raw tab. The id column is the last entry and is the anchor
// used by id-based upsert.
var ExpectedExpenseHeader = []string{
	"m", "d", "expense", "amount", "curr", "EUR", "primary", "secondary", "note", "id",
}

// ExpectedIncomeHeader is the column order the adapter writes against on the
// `YYYY Incomes` raw tab.
var ExpectedIncomeHeader = []string{
	"m", "d", "income", "amount", "curr", "EUR", "primary", "note", "id",
}

// ErrSheetLayoutMismatch indicates the sheet's raw-tab header has drifted from
// what the adapter expects. The adapter refuses to write so it does not corrupt
// dashboard formulas referencing column letters.
var ErrSheetLayoutMismatch = errors.New("sheet layout mismatch")

// validateHeader compares the actual header row to the expected one. Matching
// is case-insensitive on trimmed cells. Extra trailing columns are tolerated
// (the user may keep scratch columns to the right). Missing or renamed
// expected columns produce ErrSheetLayoutMismatch wrapped with detail.
func validateHeader(tab string, got, want []string) error {
	if len(got) < len(want) {
		return fmt.Errorf("%w: %s header has %d cols, expected at least %d (%v)",
			ErrSheetLayoutMismatch, tab, len(got), len(want), want)
	}
	for i, w := range want {
		g := strings.TrimSpace(got[i])
		if !strings.EqualFold(g, w) {
			return fmt.Errorf("%w: %s header[%d]=%q, expected %q",
				ErrSheetLayoutMismatch, tab, i, g, w)
		}
	}
	return nil
}

// checkRawTabLayout reads the header row of `expensesSheet` and `incomeSheet`
// (when configured) and validates them against the expected column order.
// Result is memoized for the lifetime of the Client.
func (c *Client) checkRawTabLayout(ctx context.Context) error {
	c.layoutOnce.Do(func() {
		c.layoutErr = c.doCheckRawTabLayout(ctx)
	})
	return c.layoutErr
}

func (c *Client) doCheckRawTabLayout(ctx context.Context) error {
	if c.svc == nil {
		return errors.New("sheets service not initialized")
	}
	if c.expensesSheet != "" {
		header, err := c.readHeaderRow(ctx, c.expensesSheet, len(ExpectedExpenseHeader))
		if err != nil {
			return err
		}
		if err := validateHeader(c.expensesSheet, header, ExpectedExpenseHeader); err != nil {
			return err
		}
	}
	if c.incomeSheet != "" {
		header, err := c.readHeaderRow(ctx, c.incomeSheet, len(ExpectedIncomeHeader))
		if err != nil {
			return err
		}
		if err := validateHeader(c.incomeSheet, header, ExpectedIncomeHeader); err != nil {
			return err
		}
	}
	return nil
}

// readHeaderRow reads the first row of `sheet`, returning at most `width`
// trimmed string cells.
func (c *Client) readHeaderRow(ctx context.Context, sheet string, width int) ([]string, error) {
	rng := fmt.Sprintf("%s!1:1", sheet)
	resp, err := c.svc.Spreadsheets.Values.Get(c.spreadsheetID, rng).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("read header %s: %w", rng, err)
	}
	if len(resp.Values) == 0 {
		return nil, nil
	}
	row := resp.Values[0]
	if len(row) > width {
		row = row[:width]
	}
	out := make([]string, len(row))
	for i, v := range row {
		out[i] = cellAsString(v)
	}
	return out, nil
}
