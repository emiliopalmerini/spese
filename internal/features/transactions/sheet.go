package transactions

import (
	"context"
	"fmt"
	"sort"

	"spese/internal/kernel"
	"spese/internal/sheets"
)

// Tab is the sheet tab name for this slice.
const Tab = "transactions"

// Filter narrows the set of transactions returned by List.
type Filter struct {
	From kernel.Date // inclusive; zero = no lower bound
	To   kernel.Date // exclusive; zero = no upper bound
	Kind Kind        // empty = any
	Last int         // if > 0, keep only the last N rows after filtering+sorting (most recent first)
}

// List reads transactions, applies the filter, and returns them sorted by
// date descending.
func List(ctx context.Context, client *sheets.Client, f Filter, force bool) ([]Transaction, error) {
	_, rows, err := client.ReadTable(ctx, Tab, force)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", Tab, err)
	}
	out := make([]Transaction, 0, len(rows))
	for _, r := range rows {
		t := parseRow(r)
		if t.Date.IsZero() {
			continue
		}
		if f.Kind != "" && t.Kind != f.Kind {
			continue
		}
		if !f.From.IsZero() && t.Date.Before(f.From.Time) {
			continue
		}
		if !f.To.IsZero() && !t.Date.Before(f.To.Time) {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.After(out[j].Date.Time) })
	if f.Last > 0 && len(out) > f.Last {
		out = out[:f.Last]
	}
	return out, nil
}

// Append writes one or more transactions to the end of the sheet (atomic
// from the user's POV — one API call).
func Append(ctx context.Context, client *sheets.Client, txns []Transaction) error {
	if len(txns) == 0 {
		return nil
	}
	rows := make([][]any, len(txns))
	for i, t := range txns {
		rows[i] = []any{
			t.Date.ISO(),
			string(t.Kind),
			t.Account,
			t.Amount.Float(),
			t.Category,
			t.Subcategory,
			t.Payee,
			t.Note,
			t.ID,
		}
	}
	return client.AppendRows(ctx, Tab, rows)
}

func parseRow(r []any) Transaction {
	get := func(i int) any {
		if i >= len(r) {
			return nil
		}
		return r[i]
	}
	t := Transaction{
		Kind:        Kind(sheets.CellString(get(1))),
		Account:     sheets.CellString(get(2)),
		Category:    sheets.CellString(get(4)),
		Subcategory: sheets.CellString(get(5)),
		Payee:       sheets.CellString(get(6)),
		Note:        sheets.CellString(get(7)),
		ID:          sheets.CellString(get(8)),
	}
	if d, ok := sheets.CellDate(get(0)); ok {
		t.Date = d
	}
	if m, ok := sheets.CellMoney(get(3)); ok {
		t.Amount = m
	}
	return t
}
