package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"spese/internal/core"
	
	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	db      *sql.DB
	queries *Queries
}

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Run migrations
	if err := RunMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	repo := &SQLiteRepository{
		db:      db,
		queries: New(db),
	}

	return repo, nil
}

func (r *SQLiteRepository) Close() error {
	if r.db != nil {
		return r.db.Close()
	}
	return nil
}

// Append implements sheets.ExpenseWriter
func (r *SQLiteRepository) Append(ctx context.Context, e core.Expense) (string, error) {
	expense, err := r.queries.CreateExpense(ctx, CreateExpenseParams{
		Day:               int64(e.Date.Day),
		Month:             int64(e.Date.Month),
		Description:       e.Description,
		AmountCents:       e.Amount.Cents,
		PrimaryCategory:   e.Primary,
		SecondaryCategory: e.Secondary,
	})
	if err != nil {
		return "", fmt.Errorf("create expense: %w", err)
	}

	slog.InfoContext(ctx, "Expense saved to SQLite", 
		"id", expense.ID, 
		"description", expense.Description, 
		"amount_cents", expense.AmountCents,
		"day", expense.Day,
		"month", expense.Month)

	return strconv.FormatInt(expense.ID, 10), nil
}

// List implements sheets.TaxonomyReader
func (r *SQLiteRepository) List(ctx context.Context) ([]string, []string, error) {
	// For now, return hardcoded categories since we don't store them in SQLite
	// In futuro, potremmo aggiungere una tabella categories
	categories := []string{
		"Alimentari", "Trasporti", "Casa", "Sanità", "Svago", 
		"Vestiti", "Regali", "Tasse", "Investimenti", "Altro",
	}
	
	subcategories := []string{
		"Supermercato", "Ristorante", "Benzina", "Trasporto Pubblico",
		"Affitto", "Bollette", "Medico", "Farmacia", "Cinema", 
		"Hobby", "Abbigliamento", "Scarpe", "Compleanno", "Natale",
		"IRPEF", "IMU", "Azioni", "Crypto", "Varie",
	}
	
	return categories, subcategories, nil
}

// ReadMonthOverview implements sheets.DashboardReader
func (r *SQLiteRepository) ReadMonthOverview(ctx context.Context, year int, month int) (core.MonthOverview, error) {
	overview := core.MonthOverview{
		Year:  year,
		Month: month,
	}

	// Get total for the month
	total, err := r.queries.GetMonthTotal(ctx, int64(month))
	if err != nil {
		return overview, fmt.Errorf("get month total: %w", err)
	}
	
	overview.Total = core.Money{Cents: total.(int64)}

	// Get category sums
	categorySums, err := r.queries.GetCategorySums(ctx, int64(month))
	if err != nil {
		return overview, fmt.Errorf("get category sums: %w", err)
	}

	for _, cs := range categorySums {
		overview.ByCategory = append(overview.ByCategory, core.CategoryAmount{
			Name:   cs.PrimaryCategory,
			Amount: core.Money{Cents: int64(cs.TotalAmount.Float64)},
		})
	}

	return overview, nil
}

// ListExpenses implements sheets.ExpenseLister
func (r *SQLiteRepository) ListExpenses(ctx context.Context, year int, month int) ([]core.Expense, error) {
	dbExpenses, err := r.queries.GetExpensesByMonth(ctx, int64(month))
	if err != nil {
		return nil, fmt.Errorf("get expenses by month: %w", err)
	}

	expenses := make([]core.Expense, len(dbExpenses))
	for i, e := range dbExpenses {
		expenses[i] = core.Expense{
			Date: core.DateParts{
				Day:   int(e.Day),
				Month: int(e.Month),
			},
			Description: e.Description,
			Amount:      core.Money{Cents: e.AmountCents},
			Primary:     e.PrimaryCategory,
			Secondary:   e.SecondaryCategory,
		}
	}

	return expenses, nil
}

// GetPendingSyncExpenses returns expenses that need to be synced to Google Sheets
func (r *SQLiteRepository) GetPendingSyncExpenses(ctx context.Context, limit int) ([]PendingSyncExpense, error) {
	dbExpenses, err := r.queries.GetPendingSyncExpenses(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("get pending sync expenses: %w", err)
	}

	expenses := make([]PendingSyncExpense, len(dbExpenses))
	for i, e := range dbExpenses {
		expenses[i] = PendingSyncExpense{
			ID:        e.ID,
			Version:   e.Version,
			CreatedAt: e.CreatedAt.Time,
		}
	}

	return expenses, nil
}

// MarkSynced marks an expense as successfully synced
func (r *SQLiteRepository) MarkSynced(ctx context.Context, id int64) error {
	err := r.queries.MarkExpenseSynced(ctx, id)
	if err != nil {
		return fmt.Errorf("mark expense synced: %w", err)
	}

	slog.InfoContext(ctx, "Expense marked as synced", "id", id)
	return nil
}

// MarkSyncError marks an expense as having sync errors
func (r *SQLiteRepository) MarkSyncError(ctx context.Context, id int64) error {
	err := r.queries.MarkExpenseSyncError(ctx, id)
	if err != nil {
		return fmt.Errorf("mark expense sync error: %w", err)
	}

	slog.WarnContext(ctx, "Expense marked with sync error", "id", id)
	return nil
}

// GetExpense retrieves a single expense by ID
func (r *SQLiteRepository) GetExpense(ctx context.Context, id int64) (*Expense, error) {
	expense, err := r.queries.GetExpense(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get expense by id: %w", err)
	}
	return &expense, nil
}

// ExpenseWithID represents an expense with its database ID for sync operations
type ExpenseWithID struct {
	ID        int64
	Expense   core.Expense
	CreatedAt time.Time
}

// PendingSyncExpense represents minimal data needed for sync queue messages
type PendingSyncExpense struct {
	ID        int64
	Version   int64
	CreatedAt time.Time
}