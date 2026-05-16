// Package transactions is the general-journal slice. Each row is one
// movement against an account: an income, an expense, a transfer leg, or an
// adjustment. amount_eur is signed: positive = into the account, negative
// = out.
package transactions

import "spese/internal/kernel"

// Kind classifies a journal row.
type Kind string

const (
	Income     Kind = "Income"
	Expense    Kind = "Expense"
	Transfer   Kind = "Transfer"
	Adjustment Kind = "Adjustment"
)

// Transaction is one row of the `transactions` tab.
type Transaction struct {
	Date        kernel.Date
	Kind        Kind
	Account     string
	Amount      kernel.Money
	Category    string
	Subcategory string
	Payee       string
	Note        string
	ID          string
}
