package accounts

import (
	"context"
	"fmt"

	"spese/internal/kernel"
	"spese/internal/sheets"
)

// Tab is the sheet tab name for this slice.
const Tab = "accounts"

// List reads every account row from the sheet.
func List(ctx context.Context, client *sheets.Client, forceRefresh bool) ([]Account, error) {
	_, rows, err := client.ReadTable(ctx, Tab, forceRefresh)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", Tab, err)
	}
	out := make([]Account, 0, len(rows))
	for _, r := range rows {
		a := parseRow(r)
		if a.Name == "" {
			continue // skip blank rows
		}
		out = append(out, a)
	}
	return out, nil
}

// Append writes a new account row to the end of the sheet.
func Append(ctx context.Context, client *sheets.Client, a Account) error {
	row := []any{
		a.Name,
		string(a.Type),
		string(a.Class),
		a.Currency,
		dateOrEmpty(a.ActiveFrom),
		dateOrEmpty(a.ActiveTo),
		a.Note,
	}
	return client.AppendRows(ctx, Tab, [][]any{row})
}

// parseRow turns one sheet row into an Account, tolerating short rows.
func parseRow(r []any) Account {
	get := func(i int) any {
		if i >= len(r) {
			return nil
		}
		return r[i]
	}
	a := Account{
		Name:     sheets.CellString(get(0)),
		Type:     Type(sheets.CellString(get(1))),
		Class:    Class(sheets.CellString(get(2))),
		Currency: sheets.CellString(get(3)),
		Note:     sheets.CellString(get(6)),
	}
	if d, ok := sheets.CellDate(get(4)); ok {
		a.ActiveFrom = d
	}
	if d, ok := sheets.CellDate(get(5)); ok {
		a.ActiveTo = d
	}
	return a
}

func dateOrEmpty(d kernel.Date) any {
	if d.IsZero() {
		return ""
	}
	return d.Month()
}
