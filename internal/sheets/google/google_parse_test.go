package google

import (
	"testing"
)

// Build a small matrix emulating the CSV provided for 2025 Dashboard
func TestParseDashboard_Example2025_July(t *testing.T) {
	values := [][]interface{}{
		{"Primary", "Secondary", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec", "Average", "Total", "None", ""},
		{"Housing", "", 778.0, 509.6, 1170.5, 674.2, 382.2, 40.0, 988.9},
		{"", "Mortage", 648.1, 0.0, 583.9, 568.7, 0.0, 0.0, 339.5},
		{"", "CondoFee", 0.0, 404.6, 323.4, 0.0, 323.4, 0.0, 323.4},
		{"", "Internet", 49.8, 0.0, 25.0, 0.0, 24.9, 40.0, 46.9},
		{"", "Furniture", 44.0, 0.0, 32.4, 105.5, 0.0, 0.0, 204.0},
		{"", "Insurances", 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 513.9},
		{"", "Cleaning", 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0},
		{"", "Electricity", 36.1, 36.1, 36.9, 0.0, 0.0, 0.0, 73.2},
		{"", "Phone", 0.0, 30.9, 0.0, 0.0, 0.0, 0.0, 0.0},
		{"Health", "", 80.0, 144.4, 191.7, 395.1, 425.0, 102.0, 148.0},
		{"Groceries", "", 368.9, 270.1, 220.9, 201.1, 197.1, 128.0, 381.5},
		{"Transport", "", 181.0, 817.1, 240.1, 55.9, 367.0, 171.0, 79.9},
		{"Out", "", 11.5, 73.3, 0.0, 0.0, 80.0, 0.0, 0.0},
		{"Travel", "", 0.0, 0.0, 0.0, 0.0, 0.0, 1652.0, 0.0},
		{"Clothing", "", 0.0, 0.0, 0.0, 0.0, 80.0, 0.0, 0.0},
		{"Leisure", "", 323.0, 413.6, 0.0, 300.0, 161.7, 600.0, 0.0},
		{"Gifts", "", 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0},
		{"Fees", "", 219.2, 0.0, 0.0, 145.6, 0.0, 0.0, 145.6},
		{"OtherExpenses", "", 32.0, 8.0, 18.0, 6.0, 6.0, 387.5, 513.9},
		{"Work", "", 0.0, 404.0, 0.0, 0.0, 0.0, 0.0, 0.0},
		{"total", "", 1994, 2236, 1841, 1778, 1699, 3081, 2258},
	}
	ov, err := parseDashboard(values, 2025, 7)
	if err != nil {
		t.Fatalf("parse err: %v", err)
	}
	if ov.Year != 2025 || ov.Month != 7 {
		t.Fatalf("unexpected year/month: %+v", ov)
	}
	if ov.Total.Cents != 225800 {
		t.Fatalf("total cents: got %d", ov.Total.Cents)
	}
	// Check a couple categories
	find := func(name string) int64 {
		for _, r := range ov.ByCategory {
			if r.Name == name {
				return r.Amount.Cents
			}
		}
		return -1
	}
	if got := find("Housing"); got != 98890 {
		t.Fatalf("Housing cents got %d", got)
	}
	if got := find("Groceries"); got != 38150 {
		t.Fatalf("Groceries cents got %d", got)
	}
}
