package actions

import (
	"sort"
	"strings"

	"spese/internal/features/recurring"
	"spese/internal/features/transactions"
)

// CategorySuggestions contains system-known category labels offered by forms.
type CategorySuggestions struct {
	Categories    []string
	Subcategories []string
}

func buildCategorySuggestions(txns []transactions.Transaction, recs []recurring.Recurring) CategorySuggestions {
	categories := map[string]string{}
	subcategories := map[string]string{}

	for _, txn := range txns {
		if !suggestCategoryForKind(txn.Kind) {
			continue
		}
		addSuggestion(categories, txn.Category)
		addSuggestion(subcategories, txn.Subcategory)
	}

	for _, rec := range recs {
		if !suggestCategoryForKind(rec.Kind) {
			continue
		}
		addSuggestion(categories, rec.Category)
		addSuggestion(subcategories, rec.Subcategory)
	}

	return CategorySuggestions{
		Categories:    sortedSuggestions(categories),
		Subcategories: sortedSuggestions(subcategories),
	}
}

func suggestCategoryForKind(kind transactions.Kind) bool {
	return kind == transactions.Income || kind == transactions.Expense
}

func addSuggestion(values map[string]string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	key := strings.ToLower(value)
	if _, ok := values[key]; !ok {
		values[key] = value
	}
}

func sortedSuggestions(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		left := strings.ToLower(out[i])
		right := strings.ToLower(out[j])
		if left == right {
			return out[i] < out[j]
		}
		return left < right
	})
	return out
}
