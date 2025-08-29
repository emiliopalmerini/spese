package sheets

import (
	"context"
	"spese/internal/core"
)

// Ports for outbound adapters.
type (
	ExpenseWriter interface {
		Append(ctx context.Context, e core.Expense) (rowRef string, err error)
	}

	TaxonomyReader interface {
		List(ctx context.Context) (categories []string, subcategories []string, err error)
	}
)
