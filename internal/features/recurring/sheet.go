package recurring

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"spese/internal/features/transactions"
	"spese/internal/sheets"
)

// Tab is the sheet tab name for this slice.
const Tab = "recurring"

// List returns every row from the recurring tab.
func List(ctx context.Context, client *sheets.Client, force bool) ([]Recurring, error) {
	_, rows, err := client.ReadTable(ctx, Tab, force)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", Tab, err)
	}
	out := make([]Recurring, 0, len(rows))
	for _, r := range rows {
		rec := parseRow(r)
		if rec.Label == "" {
			continue
		}
		out = append(out, rec)
	}
	return out, nil
}

// Append writes a new recurring row.
func Append(ctx context.Context, client *sheets.Client, r Recurring) error {
	row := []any{
		r.Label,
		string(r.Kind),
		r.Account,
		r.Amount.Float(),
		r.Category,
		r.Subcategory,
		r.Payee,
		r.DayOfMonth,
		boolStr(r.Active),
		r.Note,
	}
	return client.AppendRows(ctx, Tab, [][]any{row})
}

func parseRow(r []any) Recurring {
	get := func(i int) any {
		if i >= len(r) {
			return nil
		}
		return r[i]
	}
	rec := Recurring{
		Label:       sheets.CellString(get(0)),
		Kind:        transactions.Kind(sheets.CellString(get(1))),
		Account:     sheets.CellString(get(2)),
		Category:    sheets.CellString(get(4)),
		Subcategory: sheets.CellString(get(5)),
		Payee:       sheets.CellString(get(6)),
		Active:      parseBool(sheets.CellString(get(8))),
		Note:        sheets.CellString(get(9)),
	}
	if m, ok := sheets.CellMoney(get(3)); ok {
		rec.Amount = m
		if rec.Amount < 0 {
			rec.Amount = -rec.Amount
		}
	}
	if f, ok := sheets.CellFloat(get(7)); ok {
		rec.DayOfMonth = int(f)
	} else if n, err := strconv.Atoi(sheets.CellString(get(7))); err == nil {
		rec.DayOfMonth = n
	}
	return rec
}

func parseBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "y", "1", "x":
		return true
	}
	return false
}

func boolStr(b bool) string {
	if b {
		return "TRUE"
	}
	return "FALSE"
}
