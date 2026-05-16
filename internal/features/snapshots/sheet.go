package snapshots

import (
	"context"
	"fmt"

	"spese/internal/kernel"
	"spese/internal/sheets"
)

// Tab is the sheet tab name for this slice.
const Tab = "snapshots"

// List reads all snapshot rows.
func List(ctx context.Context, client *sheets.Client, force bool) ([]Snapshot, error) {
	_, rows, err := client.ReadTable(ctx, Tab, force)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", Tab, err)
	}
	out := make([]Snapshot, 0, len(rows))
	for _, r := range rows {
		s := parseRow(r)
		if s.Account == "" || s.Month.IsZero() {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// LatestPerAccount returns the most recent snapshot per account, keyed by
// account name.
func LatestPerAccount(ctx context.Context, client *sheets.Client, force bool) (map[string]Snapshot, error) {
	all, err := List(ctx, client, force)
	if err != nil {
		return nil, err
	}
	latest := make(map[string]Snapshot)
	for _, s := range all {
		cur, ok := latest[s.Account]
		if !ok || s.Month.After(cur.Month.Time) {
			latest[s.Account] = s
		}
	}
	return latest, nil
}

// Append writes one or more snapshot rows.
func Append(ctx context.Context, client *sheets.Client, snaps []Snapshot) error {
	if len(snaps) == 0 {
		return nil
	}
	rows := make([][]any, len(snaps))
	for i, s := range snaps {
		rows[i] = []any{
			s.Month.Month(),
			s.Account,
			s.Balance.Float(),
			s.Note,
		}
	}
	return client.AppendRows(ctx, Tab, rows)
}

func parseRow(r []any) Snapshot {
	get := func(i int) any {
		if i >= len(r) {
			return nil
		}
		return r[i]
	}
	s := Snapshot{
		Account: sheets.CellString(get(1)),
		Note:    sheets.CellString(get(3)),
	}
	if d, ok := sheets.CellDate(get(0)); ok {
		s.Month = d.FirstOfMonth()
	}
	if m, ok := sheets.CellMoney(get(2)); ok {
		s.Balance = m
	}
	return s
}

// dateOrEmpty is unused now but kept for the symmetry with accounts.sheet.go.
var _ = kernel.Today
