package dictation

import (
	"fmt"
	"strconv"
	"strings"

	"spese/internal/features/transactions"
)

const maxDrafts = 20

// Draft is one editable movement inferred from a dictation session.
type Draft struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Date        string   `json:"date"`
	Account     string   `json:"account"`
	Amount      string   `json:"amount"`
	Payee       string   `json:"payee"`
	Category    string   `json:"category"`
	Subcategory string   `json:"subcategory"`
	Note        string   `json:"note"`
	Issues      []string `json:"issues"`
}

// Extraction is the complete current state returned by the language model.
type Extraction struct {
	Movements       []Draft `json:"movements"`
	FinishRequested bool    `json:"finish_requested"`
}

// NormalizeExtraction keeps IDs controlled by the application and trims model
// output before it reaches validation or the browser.
func NormalizeExtraction(previous []Draft, next Extraction) Extraction {
	known := make(map[string]bool, len(previous))
	nextID := 1
	for _, draft := range previous {
		known[draft.ID] = true
		if n, ok := draftNumber(draft.ID); ok && n >= nextID {
			nextID = n + 1
		}
	}
	used := make(map[string]bool, len(next.Movements))
	if len(next.Movements) > maxDrafts {
		next.Movements = next.Movements[:maxDrafts]
	}
	for i := range next.Movements {
		draft := &next.Movements[i]
		if !known[draft.ID] || used[draft.ID] {
			for known[fmt.Sprintf("draft-%d", nextID)] || used[fmt.Sprintf("draft-%d", nextID)] {
				nextID++
			}
			draft.ID = fmt.Sprintf("draft-%d", nextID)
			nextID++
		}
		used[draft.ID] = true
		trimDraft(draft)
	}
	return next
}

// ValidateDrafts validates every draft and only returns transactions when the
// complete batch is valid.
func ValidateDrafts(drafts []Draft, accountNames []string) ([]transactions.Transaction, map[string]string) {
	accounts := make(map[string]string, len(accountNames))
	for _, name := range accountNames {
		accounts[strings.ToLower(strings.TrimSpace(name))] = strings.TrimSpace(name)
	}
	result := make([]transactions.Transaction, 0, len(drafts))
	issues := make(map[string]string)
	for _, draft := range drafts {
		account, ok := accounts[strings.ToLower(strings.TrimSpace(draft.Account))]
		if !ok {
			issues[draft.ID] = "Seleziona un conto esistente."
			continue
		}
		transaction, err := transactions.ValidateInput(transactions.Input{
			Kind:        draft.Kind,
			Date:        draft.Date,
			Account:     account,
			Amount:      draft.Amount,
			Category:    draft.Category,
			Subcategory: draft.Subcategory,
			Payee:       draft.Payee,
			Note:        draft.Note,
		})
		if err != nil {
			issues[draft.ID] = err.Error()
			continue
		}
		result = append(result, transaction)
	}
	if len(issues) > 0 {
		return nil, issues
	}
	return result, issues
}

func draftNumber(id string) (int, bool) {
	value, ok := strings.CutPrefix(id, "draft-")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(value)
	return n, err == nil && n > 0
}

func trimDraft(draft *Draft) {
	draft.Kind = strings.TrimSpace(draft.Kind)
	draft.Date = strings.TrimSpace(draft.Date)
	draft.Account = strings.TrimSpace(draft.Account)
	draft.Amount = strings.TrimSpace(draft.Amount)
	draft.Payee = strings.TrimSpace(draft.Payee)
	draft.Category = strings.TrimSpace(draft.Category)
	draft.Subcategory = strings.TrimSpace(draft.Subcategory)
	draft.Note = strings.TrimSpace(draft.Note)
}
