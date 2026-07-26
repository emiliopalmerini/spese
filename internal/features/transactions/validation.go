package transactions

import (
	"errors"
	"strings"

	"spese/internal/kernel"
)

// Input is the transport-independent representation of an income or expense.
type Input struct {
	Kind        string
	Date        string
	Account     string
	Amount      string
	Category    string
	Subcategory string
	Payee       string
	Note        string
}

// ValidateInput normalizes and validates one user-provided movement.
func ValidateInput(input Input) (Transaction, error) {
	kind := Kind(strings.TrimSpace(input.Kind))
	if kind != Income && kind != Expense {
		return Transaction{}, errors.New("Seleziona un tipo di movimento valido.")
	}
	account := strings.TrimSpace(input.Account)
	if account == "" {
		return Transaction{}, errors.New("Seleziona un conto.")
	}
	date := strings.TrimSpace(input.Date)
	if date == "" {
		return Transaction{}, errors.New("La data è obbligatoria.")
	}
	d, err := kernel.ParseDate(date)
	if err != nil {
		return Transaction{}, errors.New("Inserisci una data valida.")
	}
	amount, err := kernel.ParseMoney(input.Amount)
	if err != nil {
		return Transaction{}, errors.New("Inserisci un importo valido.")
	}
	if amount < 0 {
		amount = -amount
	}
	if amount == 0 {
		return Transaction{}, errors.New("L'importo deve essere maggiore di zero.")
	}
	if kind == Expense {
		amount = -amount
	}
	payee := strings.TrimSpace(input.Payee)
	if payee == "" {
		return Transaction{}, errors.New("La descrizione è obbligatoria.")
	}
	return Transaction{
		Date:        d,
		Kind:        kind,
		Account:     account,
		Amount:      amount,
		Category:    strings.TrimSpace(input.Category),
		Subcategory: strings.TrimSpace(input.Subcategory),
		Payee:       payee,
		Note:        strings.TrimSpace(input.Note),
	}, nil
}
