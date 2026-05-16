// Package recurring is the recurring-expense (and income) slice. Each row
// represents a scheduled transaction that fires on a specific day of every
// month. The processor handles firing; the handler exposes CRUD.
//
// Idempotency: when the processor fires a recurring entry it tags the
// transaction's note with "[recurring:<label>]". On subsequent runs in the
// same month it looks for the tag and skips if found, so duplicate fires
// cannot happen even without storing last_run.
package recurring

import (
	"spese/internal/features/transactions"
	"spese/internal/kernel"
)

// Recurring is one row of the `recurring` tab.
type Recurring struct {
	Label       string
	Kind        transactions.Kind
	Account     string
	Amount      kernel.Money // positive number; sign applied per Kind at fire time
	Category    string
	Subcategory string
	Payee       string
	DayOfMonth  int
	Active      bool
	Note        string
}

// Marker returns the idempotency string this recurring stamps onto fired
// transactions.
func (r Recurring) Marker() string { return "[recurring:" + r.Label + "]" }
