package actions

import (
	"reflect"
	"testing"

	"spese/internal/features/transactions"
)

func TestBuildCategorySuggestions(t *testing.T) {
	txns := []transactions.Transaction{
		{Kind: transactions.Expense, Category: " Cibo ", Subcategory: "Pranzo"},
		{Kind: transactions.Expense, Category: "Casa", Subcategory: "Affitto"},
		{Kind: transactions.Income, Category: "Stipendio", Subcategory: "Mensile"},
		{Kind: transactions.Transfer, Category: "Transfer"},
		{Kind: transactions.Adjustment, Category: "Manuale"},
		{Kind: transactions.Expense, Category: "cibo", Subcategory: "pranzo"},
		{Kind: transactions.Expense, Category: "Cibo", Subcategory: "Cena"},
		{Kind: transactions.Expense, Category: "Casa", Subcategory: "Bollette"},
		{Kind: transactions.Expense, Category: "Casa", Subcategory: "Affitto"},
		{Kind: transactions.Income, Category: "stipendio", Subcategory: "mensile"},
		{Kind: transactions.Income, Category: "Regali"},
		{Kind: transactions.Expense, Category: "", Subcategory: "Orfana"},
	}

	got := buildCategorySuggestions(txns)

	wantExpense := []CategorySuggestion{
		{
			Name:  "Casa",
			Count: 3,
			Subcategories: []ValueSuggestion{
				{Name: "Affitto", Count: 2},
				{Name: "Bollette", Count: 1},
			},
		},
		{
			Name:  "Cibo",
			Count: 3,
			Subcategories: []ValueSuggestion{
				{Name: "Pranzo", Count: 2},
				{Name: "Cena", Count: 1},
			},
		},
	}
	if !reflect.DeepEqual(got.Expense, wantExpense) {
		t.Fatalf("Expense = %#v, want %#v", got.Expense, wantExpense)
	}

	wantIncome := []CategorySuggestion{
		{
			Name:          "Stipendio",
			Count:         2,
			Subcategories: []ValueSuggestion{{Name: "Mensile", Count: 2}},
		},
		{Name: "Regali", Count: 1},
	}
	if !reflect.DeepEqual(got.Income, wantIncome) {
		t.Fatalf("Income = %#v, want %#v", got.Income, wantIncome)
	}
}
