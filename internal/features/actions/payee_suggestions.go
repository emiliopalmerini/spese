package actions

import (
	"encoding/json"
	"sort"
	"strings"

	"spese/internal/features/transactions"
	"spese/internal/kernel"
)

const minimumPayeeOccurrences = 2

// PayeeSuggestion is one deduplicated historical transaction description.
type PayeeSuggestion struct {
	Name       string
	TotalCount int
	Contexts   []PayeeContext
}

// PayeeContext aggregates occurrences with the same form choices.
type PayeeContext struct {
	Kind        transactions.Kind `json:"kind"`
	Account     string            `json:"account"`
	Category    string            `json:"category"`
	Subcategory string            `json:"subcategory"`
	Count       int               `json:"count"`
}

// ContextJSON provides compact, safely escaped metadata for client-side ranking.
func (suggestion PayeeSuggestion) ContextJSON() string {
	data, _ := json.Marshal(suggestion.Contexts)
	return string(data)
}

type payeeCount struct {
	name       string
	newestDate kernel.Date
	total      int
	contexts   map[string]*PayeeContext
}

func buildPayeeSuggestions(txns []transactions.Transaction) []PayeeSuggestion {
	payees := map[string]*payeeCount{}
	for _, txn := range txns {
		if txn.Kind != transactions.Income && txn.Kind != transactions.Expense {
			continue
		}
		name := strings.TrimSpace(txn.Payee)
		if name == "" {
			continue
		}

		payeeKey := strings.ToLower(name)
		payee, ok := payees[payeeKey]
		if !ok {
			payee = &payeeCount{name: name, newestDate: txn.Date, contexts: map[string]*PayeeContext{}}
			payees[payeeKey] = payee
		} else if txn.Date.After(payee.newestDate.Time) {
			payee.name = name
			payee.newestDate = txn.Date
		}
		payee.total++

		context := PayeeContext{
			Kind:        txn.Kind,
			Account:     strings.TrimSpace(txn.Account),
			Category:    strings.TrimSpace(txn.Category),
			Subcategory: strings.TrimSpace(txn.Subcategory),
		}
		contextKey := foldedContextKey(context)
		if existing, ok := payee.contexts[contextKey]; ok {
			existing.Count++
		} else {
			context.Count = 1
			payee.contexts[contextKey] = &context
		}
	}

	suggestions := make([]PayeeSuggestion, 0, len(payees))
	for _, payee := range payees {
		if payee.total < minimumPayeeOccurrences {
			continue
		}
		contexts := make([]PayeeContext, 0, len(payee.contexts))
		for _, context := range payee.contexts {
			contexts = append(contexts, *context)
		}
		sort.Slice(contexts, func(i, j int) bool {
			return foldedContextKey(contexts[i]) < foldedContextKey(contexts[j])
		})
		suggestions = append(suggestions, PayeeSuggestion{
			Name:       payee.name,
			TotalCount: payee.total,
			Contexts:   contexts,
		})
	}
	sort.Slice(suggestions, func(i, j int) bool {
		return lessLabel(suggestions[i].Name, suggestions[j].Name)
	})
	return suggestions
}

func rankPayeeSuggestions(suggestions []PayeeSuggestion, selected PayeeContext) []PayeeSuggestion {
	ranked := append([]PayeeSuggestion(nil), suggestions...)
	sort.SliceStable(ranked, func(i, j int) bool {
		leftScore, leftContextCount := payeeContextRank(ranked[i], selected)
		rightScore, rightContextCount := payeeContextRank(ranked[j], selected)
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		if leftContextCount != rightContextCount {
			return leftContextCount > rightContextCount
		}
		if ranked[i].TotalCount != ranked[j].TotalCount {
			return ranked[i].TotalCount > ranked[j].TotalCount
		}
		return lessLabel(ranked[i].Name, ranked[j].Name)
	})
	return ranked
}

func payeeContextRank(suggestion PayeeSuggestion, selected PayeeContext) (score, contextCount int) {
	for _, historical := range suggestion.Contexts {
		// Binary weights make kind > account > category > subcategory; counts only break equal context matches.
		candidate := 0
		if equalChoice(historical.Kind, selected.Kind) {
			candidate += 8
		}
		if equalChoice(historical.Account, selected.Account) {
			candidate += 4
		}
		if equalChoice(historical.Category, selected.Category) {
			candidate += 2
		}
		if equalChoice(historical.Subcategory, selected.Subcategory) {
			candidate++
		}
		if candidate > score {
			score = candidate
			contextCount = historical.Count
		} else if candidate == score {
			contextCount += historical.Count
		}
	}
	return score, contextCount
}

func equalChoice(left any, right any) bool {
	rightValue := strings.TrimSpace(stringValue(right))
	return rightValue != "" && strings.EqualFold(strings.TrimSpace(stringValue(left)), rightValue)
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case transactions.Kind:
		return string(typed)
	default:
		return ""
	}
}

func foldedContextKey(context PayeeContext) string {
	return strings.ToLower(strings.Join([]string{
		string(context.Kind), context.Account, context.Category, context.Subcategory,
	}, "\x00"))
}
