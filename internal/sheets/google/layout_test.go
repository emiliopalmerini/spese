package google

import (
	"errors"
	"testing"
)

func TestValidateHeader_OK(t *testing.T) {
	want := []string{"m", "d", "expense", "amount", "curr", "EUR", "primary", "secondary", "note", "id"}
	got := []string{"m", "d", "expense", "amount", "curr", "EUR", "primary", "secondary", "note", "id"}
	if err := validateHeader("Expenses", got, want); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateHeader_OKExtraTrailing(t *testing.T) {
	// Extra trailing columns are tolerated (user might add notes/scratch
	// columns on the right).
	want := []string{"m", "d", "expense"}
	got := []string{"m", "d", "expense", "scratch"}
	if err := validateHeader("Expenses", got, want); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidateHeader_MissingColumn(t *testing.T) {
	want := []string{"m", "d", "expense", "amount", "curr", "EUR", "primary", "secondary", "note", "id"}
	// id missing
	got := []string{"m", "d", "expense", "amount", "curr", "EUR", "primary", "secondary", "note"}
	err := validateHeader("Expenses", got, want)
	if !errors.Is(err, ErrSheetLayoutMismatch) {
		t.Fatalf("expected ErrSheetLayoutMismatch, got %v", err)
	}
}

func TestValidateHeader_RenamedColumn(t *testing.T) {
	want := []string{"m", "d", "expense", "amount", "curr", "EUR", "primary", "secondary", "note", "id"}
	// "primary" renamed to "category"
	got := []string{"m", "d", "expense", "amount", "curr", "EUR", "category", "secondary", "note", "id"}
	err := validateHeader("Expenses", got, want)
	if !errors.Is(err, ErrSheetLayoutMismatch) {
		t.Fatalf("expected ErrSheetLayoutMismatch, got %v", err)
	}
}

func TestValidateHeader_CaseInsensitive(t *testing.T) {
	want := []string{"m", "d", "expense", "amount", "curr", "EUR", "primary", "secondary", "note", "id"}
	got := []string{"M", "D", "Expense", "Amount", "Curr", "eur", "Primary", "Secondary", "Note", "ID"}
	if err := validateHeader("Expenses", got, want); err != nil {
		t.Fatalf("expected case-insensitive match, got %v", err)
	}
}

func TestExpectedHeaders(t *testing.T) {
	// Guard the constants so accidental edits surface in CI.
	if len(ExpectedExpenseHeader) != 10 {
		t.Fatalf("expected 10 expense columns, got %d", len(ExpectedExpenseHeader))
	}
	if ExpectedExpenseHeader[len(ExpectedExpenseHeader)-1] != "id" {
		t.Fatalf("expense header must end with id; got %v", ExpectedExpenseHeader)
	}
	if len(ExpectedIncomeHeader) != 9 {
		t.Fatalf("expected 9 income columns, got %d", len(ExpectedIncomeHeader))
	}
	if ExpectedIncomeHeader[len(ExpectedIncomeHeader)-1] != "id" {
		t.Fatalf("income header must end with id; got %v", ExpectedIncomeHeader)
	}
}
