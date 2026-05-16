// Package snapshots is the month-end balance slice. Each row is the
// authoritative balance for one account at one month end. Liabilities are
// entered as negative values.
package snapshots

import "spese/internal/kernel"

// Snapshot is one row of the `snapshots` tab.
type Snapshot struct {
	Month   kernel.Date // first day of month
	Account string
	Balance kernel.Money
	Note    string
}
