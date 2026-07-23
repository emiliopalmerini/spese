package transactions

import (
	"reflect"
	"testing"

	"spese/internal/kernel"
)

func TestBuildCategorySuggestionsRanksHistoricalIncomeAndExpenses(t *testing.T) {
	txns := []Transaction{
		{Kind: Expense, Category: "tempo Libero"},
		{Kind: Income, Category: "STIPENDIO"},
		{Kind: Expense, Category: "Casa"},
		{Kind: Expense, Category: "Tempo libero"},
		{Kind: Income, Category: "stipendio"},
		{Kind: Expense, Category: "casa"},
		{Kind: Expense, Category: "Spesa"},
		{Kind: Transfer, Category: "Transfer"},
		{Kind: Adjustment, Category: "Rettifica"},
		{Kind: Expense, Category: "  "},
	}

	got := BuildCategorySuggestions(txns)
	want := []CategorySuggestion{
		{Name: "Casa", Count: 2},
		{Name: "STIPENDIO", Count: 2},
		{Name: "tempo Libero", Count: 2},
		{Name: "Spesa", Count: 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("suggestions = %+v, want %+v", got, want)
	}
}

func TestBuildListViewRowsCombinesTransferLegs(t *testing.T) {
	day := mustDate(t, "2026-05-15")
	rows := []Transaction{
		{
			Date:     day,
			Kind:     Transfer,
			Account:  "Conto",
			Amount:   kernel.Money(-5000),
			Category: "Transfer",
			Payee:    "to Broker",
			Note:     "PAC",
		},
		{
			Date:     day,
			Kind:     Transfer,
			Account:  "Broker",
			Amount:   kernel.Money(5000),
			Category: "Transfer",
			Payee:    "from Conto",
			Note:     "PAC",
		},
	}

	got := BuildListViewRows(rows, 100)
	if len(got) != 1 {
		t.Fatalf("expected 1 display row, got %d", len(got))
	}
	if got[0].Kind != Transfer {
		t.Fatalf("expected transfer kind, got %q", got[0].Kind)
	}
	if got[0].Account != "Conto -> Broker" {
		t.Fatalf("expected combined account, got %q", got[0].Account)
	}
	if got[0].Amount != kernel.Money(5000) {
		t.Fatalf("expected positive amount, got %v", got[0].Amount)
	}
	if got[0].Payee != "PAC" {
		t.Fatalf("expected note as description, got %q", got[0].Payee)
	}
}

func TestBuildListViewRowsDoesNotCombineUnmatchedTransfers(t *testing.T) {
	day := mustDate(t, "2026-05-15")
	rows := []Transaction{
		{
			Date:     day,
			Kind:     Transfer,
			Account:  "Conto",
			Amount:   kernel.Money(-5000),
			Category: "Transfer",
			Payee:    "to Broker",
			Note:     "PAC",
		},
		{
			Date:     day,
			Kind:     Transfer,
			Account:  "Risparmi",
			Amount:   kernel.Money(5000),
			Category: "Transfer",
			Payee:    "from Conto",
			Note:     "PAC",
		},
	}

	got := BuildListViewRows(rows, 100)
	if len(got) != 2 {
		t.Fatalf("expected unmatched transfer legs to stay separate, got %d rows", len(got))
	}
}

func TestBuildListViewRowsAppliesLimitAfterCombiningTransfers(t *testing.T) {
	day := mustDate(t, "2026-05-15")
	rows := []Transaction{
		{Date: day, Kind: Transfer, Account: "Conto", Amount: kernel.Money(-5000), Category: "Transfer", Payee: "to Broker"},
		{Date: day, Kind: Transfer, Account: "Broker", Amount: kernel.Money(5000), Category: "Transfer", Payee: "from Conto"},
		{Date: day, Kind: Expense, Account: "Conto", Amount: kernel.Money(-1000), Payee: "Bar"},
	}

	got := BuildListViewRows(rows, 1)
	if len(got) != 1 {
		t.Fatalf("expected 1 display row, got %d", len(got))
	}
	if got[0].Kind != Transfer {
		t.Fatalf("expected first display row to be the combined transfer, got %q", got[0].Kind)
	}
}

func mustDate(t *testing.T, value string) kernel.Date {
	t.Helper()
	day, err := kernel.ParseDate(value)
	if err != nil {
		t.Fatalf("parse date: %v", err)
	}
	return day
}
