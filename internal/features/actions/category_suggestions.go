package actions

import (
	"sort"
	"strings"

	"spese/internal/features/transactions"
)

// CategorySuggestions contains historical labels ranked for each transaction kind.
type CategorySuggestions struct {
	Expense []CategorySuggestion
	Income  []CategorySuggestion
}

type CategorySuggestion struct {
	Name          string
	Count         int
	Subcategories []ValueSuggestion
}

type ValueSuggestion struct {
	Name  string
	Count int
}

func buildCategorySuggestions(txns []transactions.Transaction) CategorySuggestions {
	return CategorySuggestions{
		Expense: suggestionsForKind(txns, transactions.Expense),
		Income:  suggestionsForKind(txns, transactions.Income),
	}
}

type categoryCount struct {
	name          string
	count         int
	subcategories map[string]*valueCount
}

type valueCount struct {
	name  string
	count int
}

func suggestionsForKind(txns []transactions.Transaction, kind transactions.Kind) []CategorySuggestion {
	categories := map[string]*categoryCount{}
	for _, txn := range txns {
		if txn.Kind != kind {
			continue
		}
		category := strings.TrimSpace(txn.Category)
		if category == "" {
			continue
		}

		key := strings.ToLower(category)
		count, ok := categories[key]
		if !ok {
			count = &categoryCount{name: category, subcategories: map[string]*valueCount{}}
			categories[key] = count
		}
		count.count++

		subcategory := strings.TrimSpace(txn.Subcategory)
		if subcategory == "" {
			continue
		}
		subcategoryKey := strings.ToLower(subcategory)
		subcategoryCount, ok := count.subcategories[subcategoryKey]
		if !ok {
			subcategoryCount = &valueCount{name: subcategory}
			count.subcategories[subcategoryKey] = subcategoryCount
		}
		subcategoryCount.count++
	}

	out := make([]CategorySuggestion, 0, len(categories))
	for _, category := range categories {
		out = append(out, CategorySuggestion{
			Name:          category.name,
			Count:         category.count,
			Subcategories: sortedValues(category.subcategories),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return lessLabel(out[i].Name, out[j].Name)
	})
	return out
}

func sortedValues(values map[string]*valueCount) []ValueSuggestion {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for key := range values {
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool {
		left := values[out[i]]
		right := values[out[j]]
		if left.count != right.count {
			return left.count > right.count
		}
		return lessLabel(left.name, right.name)
	})

	suggestions := make([]ValueSuggestion, 0, len(out))
	for _, key := range out {
		value := values[key]
		suggestions = append(suggestions, ValueSuggestion{Name: value.name, Count: value.count})
	}
	return suggestions
}

func lessLabel(left, right string) bool {
	leftFolded := strings.ToLower(left)
	rightFolded := strings.ToLower(right)
	if leftFolded == rightFolded {
		return left < right
	}
	return leftFolded < rightFolded
}
