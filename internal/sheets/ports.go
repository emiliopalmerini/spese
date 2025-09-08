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

	// DashboardReader provides aggregated monthly data from a dashboard sheet.
	DashboardReader interface {
		// ReadMonthOverview returns totals for a specific year and month.
		ReadMonthOverview(ctx context.Context, year int, month int) (core.MonthOverview, error)
	}

	// ExpenseLister returns the detailed list of expenses for a given month.
	ExpenseLister interface {
		// ListExpenses returns all expenses for the specified year and month.
		ListExpenses(ctx context.Context, year int, month int) ([]core.Expense, error)
	}
)
