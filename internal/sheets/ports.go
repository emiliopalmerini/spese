package sheets

import (
	"context"
	"spese/internal/core"
)

// ExpenseWithID represents an expense with its storage ID
type ExpenseWithID struct {
	ID      string
	Expense core.Expense
}

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

	// ExpenseListerWithID returns expenses along with their IDs for frontend operations
	ExpenseListerWithID interface {
		// ListExpensesWithID returns all expenses with their IDs for the specified year and month.
		ListExpensesWithID(ctx context.Context, year int, month int) ([]ExpenseWithID, error)
	}

	// ExpenseDeleter provides expense deletion functionality.
	ExpenseDeleter interface {
		// DeleteExpense removes an expense by ID.
		DeleteExpense(ctx context.Context, id string) error
	}

	// RecurrentExpenseWriter manages recurrent expenses.
	RecurrentExpenseWriter interface {
		// SaveRecurrentExpense creates a new recurrent expense.
		SaveRecurrentExpense(ctx context.Context, re core.RecurrentExpenses) error
	}

	// RecurrentExpenseLister returns the list of active recurrent expenses.
	RecurrentExpenseLister interface {
		// ListActiveRecurrentExpenses returns all active recurrent expenses.
		ListActiveRecurrentExpenses(ctx context.Context) ([]core.RecurrentExpenses, error)
	}

	// IncomeWriter provides income append functionality.
	IncomeWriter interface {
		// AppendIncome writes an income to the backing store.
		AppendIncome(ctx context.Context, i core.Income) (rowRef string, err error)
	}

	// RemoteExpenseWriter is the sync-target port: idempotent upsert keyed by
	// expense ID. Distinct from ExpenseWriter (local-insert path used by
	// handlers). Implementations require e.ID > 0; zero returns ErrMissingID
	// from the adapter package.
	RemoteExpenseWriter interface {
		UpsertExpense(ctx context.Context, e core.Expense) (rowRef string, err error)
	}

	// RemoteIncomeWriter mirrors RemoteExpenseWriter for incomes.
	RemoteIncomeWriter interface {
		UpsertIncome(ctx context.Context, i core.Income) (rowRef string, err error)
	}

	// NetWorthWriter writes a single account balance for a given month into
	// the Net Worth section of the dashboard sheet.
	NetWorthWriter interface {
		UpsertBalance(ctx context.Context, accountName string, accountType core.AccountType,
			year, month int, amount core.Money) (rowRef string, err error)
	}
)
