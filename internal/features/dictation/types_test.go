package dictation

import (
	"testing"

	"spese/internal/features/transactions"
)

func TestNormalizeExtractionPreservesKnownIDsAndAssignsNewOnes(t *testing.T) {
	t.Parallel()

	previous := []Draft{{ID: "draft-1", Payee: "Conad"}}
	next := Extraction{Movements: []Draft{
		{ID: "draft-1", Payee: "Conad", Amount: "24,00"},
		{ID: "invented", Payee: "Bar", Amount: "2,50"},
	}}

	got := NormalizeExtraction(previous, next)
	if got.Movements[0].ID != "draft-1" {
		t.Fatalf("known id = %q, want draft-1", got.Movements[0].ID)
	}
	if got.Movements[1].ID != "draft-2" {
		t.Fatalf("new id = %q, want draft-2", got.Movements[1].ID)
	}
}

func TestValidateDraftsValidatesEntireBatch(t *testing.T) {
	t.Parallel()

	drafts := []Draft{
		{ID: "draft-1", Kind: "Expense", Date: "2026-07-26", Account: "Fineco", Amount: "12,50", Payee: "Conad"},
		{ID: "draft-2", Kind: "Expense", Date: "2026-07-26", Account: "Inventato", Amount: "2,50", Payee: "Bar"},
	}

	got, issues := ValidateDrafts(drafts, []string{"Fineco"})
	if got != nil {
		t.Fatalf("transactions = %#v, want nil for invalid batch", got)
	}
	if issues["draft-2"] != "Seleziona un conto esistente." {
		t.Fatalf("issue = %q, want unknown-account issue", issues["draft-2"])
	}
}

func TestValidateDraftsBuildsTransactions(t *testing.T) {
	t.Parallel()

	drafts := []Draft{{
		ID: "draft-1", Kind: "Expense", Date: "2026-07-26", Account: "Fineco",
		Amount: "12,50", Payee: "Conad", Category: "Spesa",
	}}

	got, issues := ValidateDrafts(drafts, []string{"Fineco"})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
	if len(got) != 1 || got[0].Kind != transactions.Expense || int64(got[0].Amount) != -1250 {
		t.Fatalf("transactions = %#v, want normalized expense", got)
	}
}
