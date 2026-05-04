package google

import (
	"testing"

	"spese/internal/core"
)

func sampleNetWorthGrid() [][]string {
	// Row 1: header (year/month). Row 2: Net Worth label. Sections + accounts follow.
	return [][]string{
		{"Asset/Liability", "Cur", "2025", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec", "2026", "Jan", "Feb"},
		{"Net Worth", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"Cash - Liquidity", "EUR", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"Conto BCC", "EUR", "", "100", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"Patreon", "EUR", "", "50", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"Rain day funds", "EUR", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"Fondo Fonte", "EUR", "", "200", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"Long Term investment", "EUR", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"Trade Republic E", "EUR", "", "1000", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"FirstHouse", "EUR", "", "5000", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
		{"", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
	}
}

func TestParseNetWorthLayout(t *testing.T) {
	grid := sampleNetWorthGrid()
	layout, err := parseNetWorthLayout(grid, 1)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if layout.NetWorthRow != 2 {
		t.Fatalf("expected NetWorthRow=2, got %d", layout.NetWorthRow)
	}
	if layout.HeaderRowIndex != 1 {
		t.Fatalf("expected HeaderRowIndex=1, got %d", layout.HeaderRowIndex)
	}
	if len(layout.Sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(layout.Sections))
	}
	cash := layout.Sections[0]
	if cash.Type != core.AccountCash {
		t.Fatalf("expected first section cash, got %s", cash.Type)
	}
	if len(cash.Accounts) != 2 || cash.Accounts[0].Name != "Conto BCC" || cash.Accounts[0].Row != 4 {
		t.Fatalf("unexpected cash accounts: %+v", cash.Accounts)
	}
	if cash.BlankRow != 6 {
		t.Fatalf("expected cash BlankRow=6, got %d", cash.BlankRow)
	}
	rainy := layout.Sections[1]
	if rainy.Type != core.AccountRainyDay {
		t.Fatalf("expected rainy_day, got %s", rainy.Type)
	}
	if len(rainy.Accounts) != 1 || rainy.Accounts[0].Name != "Fondo Fonte" {
		t.Fatalf("unexpected rainy: %+v", rainy.Accounts)
	}
	long := layout.Sections[2]
	if long.Type != core.AccountLongTerm {
		t.Fatalf("expected long_term, got %s", long.Type)
	}
	if len(long.Accounts) != 2 || long.Accounts[1].Name != "FirstHouse" {
		t.Fatalf("unexpected long-term accounts: %+v", long.Accounts)
	}
}

func TestParseNetWorthLayout_MissingHeader(t *testing.T) {
	grid := [][]string{
		{"random", "junk"},
	}
	if _, err := parseNetWorthLayout(grid, 1); err == nil {
		t.Fatal("expected error when header missing")
	}
}

func TestFindMonthColumn(t *testing.T) {
	header := sampleNetWorthGrid()[0]

	// Jan 2025 → index 3 (0-based "Jan" of 2025)
	col, err := findMonthColumn(header, 2025, 1)
	if err != nil || col != 3 {
		t.Fatalf("expected col=3 for Jan 2025, got col=%d err=%v", col, err)
	}
	// Feb 2025 → index 4
	col, err = findMonthColumn(header, 2025, 2)
	if err != nil || col != 4 {
		t.Fatalf("expected col=4 for Feb 2025, got col=%d err=%v", col, err)
	}
	// Feb 2026 → index 17
	col, err = findMonthColumn(header, 2026, 2)
	if err != nil || col != 17 {
		t.Fatalf("expected col=17 for Feb 2026, got col=%d err=%v", col, err)
	}
	// Year not in header
	if _, err := findMonthColumn(header, 2099, 1); err == nil {
		t.Fatal("expected error for missing year")
	}
}

func TestColumnLetter(t *testing.T) {
	cases := []struct {
		idx int
		s   string
	}{
		{0, "A"},
		{25, "Z"},
		{26, "AA"},
		{27, "AB"},
		{51, "AZ"},
		{52, "BA"},
	}
	for _, tc := range cases {
		if got := columnLetter(tc.idx); got != tc.s {
			t.Fatalf("columnLetter(%d)=%q, want %q", tc.idx, got, tc.s)
		}
	}
}
