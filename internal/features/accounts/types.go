// Package accounts is the chart-of-accounts slice. It exposes a list of
// accounts (Asset / Liability) and a writer for new ones. Used directly by
// other slices that need to pick or filter accounts (transactions, transfers,
// snapshots).
package accounts

import "spese/internal/kernel"

// Type is the high-level accounting type.
type Type string

const (
	Asset     Type = "Asset"
	Liability Type = "Liability"
)

// Class is the operational sub-grouping inside a Type.
type Class string

const (
	Cash       Class = "Cash"
	Investment Class = "Investment"
	Property   Class = "Property"
	Tax        Class = "Tax"
	Credit     Class = "Credit"
	Other      Class = "Other"
)

// Account is one row of the `accounts` chart of accounts.
type Account struct {
	Name       string
	Type       Type
	Class      Class
	Currency   string
	ActiveFrom kernel.Date
	ActiveTo   kernel.Date
	Note       string
}

// IsActive returns true if the account is open on date d. Either bound being
// zero means open-ended.
func (a Account) IsActive(d kernel.Date) bool {
	if !a.ActiveFrom.IsZero() && d.Before(a.ActiveFrom.Time) {
		return false
	}
	if !a.ActiveTo.IsZero() && d.After(a.ActiveTo.Time) {
		return false
	}
	return true
}
