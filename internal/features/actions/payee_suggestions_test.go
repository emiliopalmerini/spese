package actions

import (
	"reflect"
	"testing"

	"spese/internal/features/transactions"
	"spese/internal/kernel"
)

func TestBuildPayeeSuggestionsFiltersAndPreservesNewestSpelling(t *testing.T) {
	older := mustPayeeSuggestionDate(t, "2026-01-01")
	newer := mustPayeeSuggestionDate(t, "2026-02-01")
	txns := []transactions.Transaction{
		{Date: older, Kind: transactions.Expense, Payee: "corner shop", Account: "Card", Category: "Food"},
		{Date: newer, Kind: transactions.Expense, Payee: " Corner Shop ", Account: "Cash", Category: "Food"},
		{Date: newer, Kind: transactions.Income, Payee: "Client", Account: "Bank", Category: "Work"},
		{Date: older, Kind: transactions.Income, Payee: "client", Account: "Bank", Category: "Work"},
		{Date: newer, Kind: transactions.Transfer, Payee: "Ignored transfer"},
		{Date: older, Kind: transactions.Transfer, Payee: "Ignored transfer"},
		{Date: newer, Kind: transactions.Adjustment, Payee: "Ignored adjustment"},
		{Date: older, Kind: transactions.Adjustment, Payee: "Ignored adjustment"},
		{Date: newer, Kind: transactions.Expense, Payee: "  "},
		{Date: older, Kind: transactions.Expense, Payee: "One off"},
	}

	got := buildPayeeSuggestions(txns)
	want := []PayeeSuggestion{
		{
			Name:       "Client",
			TotalCount: 2,
			Contexts: []PayeeContext{
				{Kind: transactions.Income, Account: "Bank", Category: "Work", Count: 2},
			},
		},
		{
			Name:       "Corner Shop",
			TotalCount: 2,
			Contexts: []PayeeContext{
				{Kind: transactions.Expense, Account: "Card", Category: "Food", Count: 1},
				{Kind: transactions.Expense, Account: "Cash", Category: "Food", Count: 1},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("suggestions = %#v, want %#v", got, want)
	}
}

func TestRankPayeeSuggestionsUsesWeightedContextThenCountsAndName(t *testing.T) {
	suggestions := []PayeeSuggestion{
		{Name: "Kind", TotalCount: 2, Contexts: []PayeeContext{{Kind: transactions.Expense, Account: "Other", Count: 2}}},
		{Name: "Account", TotalCount: 5, Contexts: []PayeeContext{{Kind: transactions.Income, Account: "Card", Count: 5}}},
		{Name: "Category", TotalCount: 2, Contexts: []PayeeContext{{Kind: transactions.Expense, Account: "Card", Category: "Food", Count: 2}}},
		{Name: "Subcategory", TotalCount: 2, Contexts: []PayeeContext{{Kind: transactions.Expense, Account: "Card", Category: "Food", Subcategory: "Lunch", Count: 2}}},
		{Name: "More context", TotalCount: 3, Contexts: []PayeeContext{{Kind: transactions.Expense, Account: "Card", Category: "Food", Subcategory: "Lunch", Count: 3}}},
		{Name: "Alpha", TotalCount: 4, Contexts: []PayeeContext{{Kind: transactions.Expense, Account: "Card", Category: "Food", Subcategory: "Lunch", Count: 2}, {Kind: transactions.Income, Count: 2}}},
		{Name: "beta", TotalCount: 4, Contexts: []PayeeContext{{Kind: transactions.Expense, Account: "Card", Category: "Food", Subcategory: "Lunch", Count: 2}, {Kind: transactions.Income, Count: 2}}},
	}

	got := rankPayeeSuggestions(suggestions, PayeeContext{
		Kind:        transactions.Expense,
		Account:     "card",
		Category:    "FOOD",
		Subcategory: "Lunch",
	})
	var names []string
	for _, suggestion := range got {
		names = append(names, suggestion.Name)
	}
	want := []string{"More context", "Alpha", "beta", "Subcategory", "Category", "Kind", "Account"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("ranked names = %v, want %v", names, want)
	}
}

func mustPayeeSuggestionDate(t *testing.T, value string) kernel.Date {
	t.Helper()
	date, err := kernel.ParseDate(value)
	if err != nil {
		t.Fatalf("parse date %q: %v", value, err)
	}
	return date
}
