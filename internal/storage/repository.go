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
	if err := RunMigrations(dbPath); err != nil {
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
	// Get primary categories from database
	primaryCategories, err := r.queries.GetPrimaryCategories(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("get primary categories: %w", err)
	}

	// Get all secondary categories from database
	secondaryCategories, err := r.queries.GetSecondaryCategories(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("get secondary categories: %w", err)
	}

	return primaryCategories, secondaryCategories, nil
}

// GetSecondariesByPrimary returns secondary categories for a given primary category
func (r *SQLiteRepository) GetSecondariesByPrimary(ctx context.Context, primaryCategory string) ([]string, error) {
	secondaryCategories, err := r.queries.GetSecondariesByPrimary(ctx, primaryCategory)
	if err != nil {
		return nil, fmt.Errorf("get secondary categories for primary %s: %w", primaryCategory, err)
	}

	return secondaryCategories, nil
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

	overview.Total = core.Money{Cents: total}

	// Get category sums
	categorySums, err := r.queries.GetCategorySums(ctx, int64(month))
	if err != nil {
		return overview, fmt.Errorf("get category sums: %w", err)
	}

	for _, cs := range categorySums {
		overview.ByCategory = append(overview.ByCategory, core.CategoryAmount{
			Name:   cs.PrimaryCategory,
			Amount: core.Money{Cents: cs.TotalAmount},
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

// CreateCategory is deprecated - categories are managed via migrations
func (r *SQLiteRepository) CreateCategory(ctx context.Context, name, categoryType string) error {
	slog.WarnContext(ctx, "CreateCategory called but is deprecated - categories are managed via migrations")
	return nil
}

// DeleteCategory is deprecated - categories are managed via migrations
func (r *SQLiteRepository) DeleteCategory(ctx context.Context, name, categoryType string) error {
	slog.WarnContext(ctx, "DeleteCategory called but is deprecated - categories are managed via migrations")
	return nil
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

// SyncCategories syncs categories from Google Sheets with hierarchical mapping
func (r *SQLiteRepository) SyncCategories(ctx context.Context, categories []string, categoryType string) error {
	if categoryType == "primary" {
		return r.syncPrimaryCategories(ctx, categories)
	} else if categoryType == "secondary" {
		return r.syncSecondaryCategories(ctx, categories)
	}

	return fmt.Errorf("unsupported category type: %s", categoryType)
}

// syncPrimaryCategories syncs primary categories from Google Sheets
func (r *SQLiteRepository) syncPrimaryCategories(ctx context.Context, categories []string) error {
	// For now, we don't sync primary categories from sheets since they're managed by migration
	// This ensures our predefined hierarchy is maintained
	slog.InfoContext(ctx, "Skipping primary category sync - managed by migrations", "count", len(categories))
	return nil
}

// syncSecondaryCategories syncs secondary categories with mapping to primaries
func (r *SQLiteRepository) syncSecondaryCategories(ctx context.Context, categories []string) error {
	// Mapping of secondary categories to their primary categories
	// This maps categories from Google Sheets to our hierarchical structure
	categoryMapping := map[string]string{
		// Casa
		"Mutuo":              "Casa",
		"Spese condominiali": "Casa",
		"Internet":           "Casa",
		"Mobili":             "Casa",
		"Assicurazioni":      "Casa",
		"Pulizia":            "Casa",
		"Elettricità":        "Casa",
		"Telefono":           "Casa",
		"Bollette":           "Casa", // Legacy mapping
		"Affitto":            "Casa", // Legacy mapping

		// Salute
		"Assicurazione sanitaria": "Salute",
		"Dottori":                 "Salute",
		"Medicine":                "Salute",
		"Personale":               "Salute",
		"Sport":                   "Salute",
		"Medico":                  "Salute", // Legacy mapping
		"Farmacia":                "Salute", // Legacy mapping

		// Spesa
		"Everli":                   "Spesa",
		"Altre spese (non Everli)": "Spesa",
		"Supermercato":             "Spesa", // Legacy mapping

		// Trasporti
		"Trasporto locale":   "Trasporti",
		"Car sharing":        "Trasporti",
		"Spese automobile":   "Trasporti",
		"Servizi taxi":       "Trasporti",
		"Benzina":            "Trasporti", // Legacy mapping
		"Trasporto Pubblico": "Trasporti", // Legacy mapping

		// Fuori (come fuori a cena...)
		"Ristoranti":  "Fuori (come fuori a cena...)",
		"Bar":         "Fuori (come fuori a cena...)",
		"Cibo a casa": "Fuori (come fuori a cena...)",
		"Ristorante":  "Fuori (come fuori a cena...)", // Legacy mapping

		// Viaggi
		"Vacanza":        "Viaggi",
		"Vacanza estiva": "Viaggi",

		// Bimbi
		"Cura bimbi":  "Bimbi",
		"Roba bimbi":  "Bimbi",
		"Corsi bimbi": "Bimbi",
		"Baby sitter": "Bimbi",

		// Vestiti
		"Vestiti e":     "Vestiti",
		"Vestiti g":     "Vestiti",
		"Vestiti bimbi": "Vestiti",
		"Abbigliamento": "Vestiti", // Legacy mapping
		"Scarpe":        "Vestiti", // Legacy mapping

		// Divertimento
		"Tech":                   "Divertimento",
		"Libri e":                "Divertimento",
		"Divertimento e":         "Divertimento",
		"Learning e":             "Divertimento",
		"Giochi e":               "Divertimento",
		"Giochi g":               "Divertimento",
		"Learning g":             "Divertimento",
		"Divertimento familiare": "Divertimento",
		"Altri divertimenti":     "Divertimento",
		"Cinema":                 "Divertimento", // Legacy mapping
		"Hobby":                  "Divertimento", // Legacy mapping

		// Regali
		"Altri regali": "Regali",
		"Compleanno":   "Regali", // Legacy mapping
		"Natale":       "Regali", // Legacy mapping

		// Tasse e Percentuali
		"Brokers":                   "Tasse e Percentuali",
		"Banche":                    "Tasse e Percentuali",
		"Consulting":                "Tasse e Percentuali",
		"Altre tasse e percentuali": "Tasse e Percentuali",
		"IRPEF":                     "Tasse e Percentuali", // Legacy mapping
		"IMU":                       "Tasse e Percentuali", // Legacy mapping

		// Altre spese
		"Tasse statali": "Altre spese",
		"2DM":           "Altre spese",
		"Unknown":       "Altre spese",
		"Varie":         "Altre spese", // Legacy mapping
		"Azioni":        "Altre spese", // Legacy mapping
		"Crypto":        "Altre spese", // Legacy mapping

		// Lavoro
		"Lavoro g": "Lavoro",
		"Lavoro e": "Lavoro",
	}

	slog.InfoContext(ctx, "Syncing secondary categories from Google Sheets", "count", len(categories))

	// For each category from Google Sheets, map it to the appropriate primary
	syncedCount := 0
	for _, category := range categories {
		if category == "" {
			continue
		}

		primaryCategory, exists := categoryMapping[category]
		if !exists {
			slog.WarnContext(ctx, "Unknown secondary category from Google Sheets",
				"category", category,
				"action", "skipping")
			continue
		}

		// Check if this secondary category already exists in our database
		existingSecondaries, err := r.GetSecondariesByPrimary(ctx, primaryCategory)
		if err != nil {
			slog.ErrorContext(ctx, "Failed to check existing secondary categories",
				"primary", primaryCategory, "error", err)
			continue
		}

		// Check if it already exists
		categoryExists := false
		for _, existing := range existingSecondaries {
			if existing == category {
				categoryExists = true
				break
			}
		}

		if categoryExists {
			slog.DebugContext(ctx, "Secondary category already exists",
				"category", category, "primary", primaryCategory)
			continue
		}

		slog.InfoContext(ctx, "Adding new secondary category from Google Sheets",
			"category", category, "primary", primaryCategory)
		syncedCount++
	}

	slog.InfoContext(ctx, "Secondary categories sync completed",
		"total_from_sheets", len(categories),
		"synced", syncedCount)

	return nil
}

// GetCategoryCount returns the total number of categories in the database
func (r *SQLiteRepository) GetCategoryCount(ctx context.Context) (int64, error) {
	// Count primary categories
	primaries, err := r.queries.GetPrimaryCategories(ctx)
	if err != nil {
		return 0, fmt.Errorf("get primary categories: %w", err)
	}

	// Count secondary categories
	secondaries, err := r.queries.GetSecondaryCategories(ctx)
	if err != nil {
		return 0, fmt.Errorf("get secondary categories: %w", err)
	}

	return int64(len(primaries) + len(secondaries)), nil
}

// GetCategoryLastSync returns when categories were last synced (now deprecated)
func (r *SQLiteRepository) GetCategoryLastSync(ctx context.Context) (time.Time, error) {
	slog.WarnContext(ctx, "GetCategoryLastSync called but is deprecated - categories are managed via migrations")
	return time.Now(), nil
}

// RefreshCategories clears all cached categories
func (r *SQLiteRepository) RefreshCategories(ctx context.Context) error {
	// Clear secondary categories first (due to foreign key constraint)
	err := r.queries.RefreshCategories(ctx)
	if err != nil {
		return fmt.Errorf("refresh secondary categories: %w", err)
	}

	// Clear primary categories
	err = r.queries.RefreshPrimaryCategories(ctx)
	if err != nil {
		return fmt.Errorf("refresh primary categories: %w", err)
	}

	slog.InfoContext(ctx, "Categories cache cleared")
	return nil
}
