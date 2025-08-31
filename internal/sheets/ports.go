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
)
