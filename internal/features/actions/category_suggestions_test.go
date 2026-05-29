package actions

import (
	"reflect"
	"testing"

	"spese/internal/features/recurring"
	"spese/internal/features/transactions"
)

func TestBuildCategorySuggestions(t *testing.T) {
	txns := []transactions.Transaction{
		{Kind: transactions.Expense, Category: " Cibo ", Subcategory: "Pranzo"},
		{Kind: transactions.Income, Category: "Stipendio", Subcategory: "Mensile"},
		{Kind: transactions.Transfer, Category: "Transfer"},
		{Kind: transactions.Adjustment, Category: "Manuale"},
		{Kind: transactions.Expense, Category: "cibo", Subcategory: "pranzo"},
	}
	recs := []recurring.Recurring{
		{Kind: transactions.Expense, Category: "Casa", Subcategory: "Affitto"},
		{Kind: transactions.Income, Category: "Stipendio"},
	}

	got := buildCategorySuggestions(txns, recs)

	wantCategories := []string{"Casa", "Cibo", "Stipendio"}
	if !reflect.DeepEqual(got.Categories, wantCategories) {
		t.Fatalf("Categories = %#v, want %#v", got.Categories, wantCategories)
	}

	wantSubcategories := []string{"Affitto", "Mensile", "Pranzo"}
	if !reflect.DeepEqual(got.Subcategories, wantSubcategories) {
		t.Fatalf("Subcategories = %#v, want %#v", got.Subcategories, wantSubcategories)
	}
}
