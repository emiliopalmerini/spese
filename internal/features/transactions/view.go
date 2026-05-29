package transactions

import (
	"strings"

	"spese/internal/kernel"
)

const transactionListLimit = 100

// BuildListViewRows returns transactions as they should appear in the
// movements list. Transfer legs remain separate in the sheet, but matching
// source/destination rows are collapsed into a single display row.
func BuildListViewRows(txns []Transaction, limit int) []Transaction {
	out := make([]Transaction, 0, len(txns))
	used := make([]bool, len(txns))
	for i, txn := range txns {
		if used[i] {
			continue
		}
		if txn.Kind == Transfer {
			if j, ok := matchingTransferLeg(txns, used, i); ok {
				out = append(out, combineTransfer(txn, txns[j]))
				used[i] = true
				used[j] = true
				if limitReached(out, limit) {
					return out
				}
				continue
			}
		}
		out = append(out, txn)
		used[i] = true
		if limitReached(out, limit) {
			return out
		}
	}
	return out
}

func matchingTransferLeg(txns []Transaction, used []bool, i int) (int, bool) {
	for j := i + 1; j < len(txns); j++ {
		if used[j] {
			continue
		}
		if isTransferPair(txns[i], txns[j]) {
			return j, true
		}
	}
	return 0, false
}

func isTransferPair(a, b Transaction) bool {
	if a.Kind != Transfer || b.Kind != Transfer {
		return false
	}
	if !sameDate(a.Date, b.Date) || a.Amount+b.Amount != 0 || a.Amount == 0 {
		return false
	}
	if strings.TrimSpace(a.Note) != strings.TrimSpace(b.Note) {
		return false
	}

	negative, positive := transferLegsBySign(a, b)
	if negative.Amount >= 0 || positive.Amount <= 0 {
		return false
	}
	return strings.TrimSpace(negative.Payee) == "to "+strings.TrimSpace(positive.Account) &&
		strings.TrimSpace(positive.Payee) == "from "+strings.TrimSpace(negative.Account)
}

func combineTransfer(a, b Transaction) Transaction {
	negative, positive := transferLegsBySign(a, b)
	description := strings.TrimSpace(negative.Note)
	if description == "" {
		description = "Trasferimento"
	}
	return Transaction{
		Date:        negative.Date,
		Kind:        Transfer,
		Account:     strings.TrimSpace(negative.Account) + " -> " + strings.TrimSpace(positive.Account),
		Amount:      absMoney(negative.Amount),
		Category:    "Transfer",
		Subcategory: negative.Subcategory,
		Payee:       description,
		Note:        negative.Note,
		ID:          negative.ID,
	}
}

func transferLegsBySign(a, b Transaction) (negative Transaction, positive Transaction) {
	if a.Amount < 0 {
		return a, b
	}
	return b, a
}

func sameDate(a, b kernel.Date) bool {
	return a.ISO() == b.ISO()
}

func absMoney(m kernel.Money) kernel.Money {
	if m < 0 {
		return -m
	}
	return m
}

func limitReached[T any](items []T, limit int) bool {
	return limit > 0 && len(items) >= limit
}
