// Package storage owns the local SQLite database and Honker outbox wiring.
package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	honker "github.com/russellromney/honker-go"
)

const SheetSyncOutboxName = "sheet-sync"

//go:embed schema.sql
var schemaFS embed.FS

// Store wraps Honker's SQLite handle so feature slices can share one local
// source of truth and enqueue sheet-sync work transactionally.
type Store struct {
	honker *honker.Database
	db     *sql.DB
}

type Tx interface {
	Exec(query string, args ...any) (sql.Result, error)
	Commit() error
	Rollback() error
}

type SQLRunner interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// SheetSyncPayload is the durable outbox message written after local changes.
type SheetSyncPayload struct {
	Scope string `json:"scope"`
}

// Open opens the local SQLite database, loads Honker, and applies schema.
func Open(ctx context.Context, dbPath, extensionPath string) (*Store, error) {
	hdb, err := honker.Open(dbPath, extensionPath)
	if err != nil {
		return nil, fmt.Errorf("open honker db: %w", err)
	}
	st := &Store{honker: hdb, db: hdb.Raw()}
	if err := st.migrate(ctx); err != nil {
		_ = st.Close()
		return nil, err
	}
	return st, nil
}

// OpenPlain opens the SQLite database without Honker. Use it when the durable
// sheet mirror worker is disabled so idle servers do not pay Honker's watcher
// polling cost.
func OpenPlain(ctx context.Context, dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	if _, err := db.ExecContext(ctx, `
		PRAGMA journal_mode = WAL;
		PRAGMA synchronous = NORMAL;
		PRAGMA busy_timeout = 5000;
		PRAGMA foreign_keys = ON;
		PRAGMA cache_size = -32000;
		PRAGMA temp_store = MEMORY;
	`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite pragmas: %w", err)
	}
	st := &Store{db: db}
	if err := st.migrate(ctx); err != nil {
		_ = st.Close()
		return nil, err
	}
	return st, nil
}

// DB returns the database/sql handle for read-side queries.
func (s *Store) DB() *sql.DB { return s.db }

// Honker returns the underlying Honker database for worker setup.
func (s *Store) Honker() *honker.Database { return s.honker }

// Begin starts a Honker transaction. Writes that need sync should call
// EnqueueSheetSync before Commit.
func (s *Store) Begin(ctx context.Context) (Tx, error) {
	if s.honker == nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("begin tx: %w", err)
		}
		return tx, nil
	}
	tx, err := s.honker.Transaction(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	return tx, nil
}

// EnqueueSheetSync writes a durable Honker queue message in the supplied
// transaction. This is the transactional outbox for the sheet mirror.
func (s *Store) EnqueueSheetSync(tx Tx, scope string) error {
	if s.honker == nil {
		return nil
	}
	honkerTx, ok := tx.(*honker.Transaction)
	if !ok {
		return fmt.Errorf("enqueue sheet sync: transaction is not a honker transaction")
	}
	queue := s.honker.Queue(SheetSyncOutboxName, honker.QueueOptions{
		VisibilityTimeoutS: 300,
		MaxAttempts:        10,
	})
	if _, err := queue.EnqueueTx(honkerTx, SheetSyncPayload{Scope: scope}, honker.EnqueueOptions{}); err != nil {
		return fmt.Errorf("enqueue sheet sync: %w", err)
	}
	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	if s.honker != nil {
		return s.honker.Close()
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	return MigrateSQLite(ctx, s.db)
}

// MigrateSQLite applies the app schema to a plain SQLite handle. It is used by
// the server after Honker opens the database and by one-off maintenance tools
// that do not need to load the Honker extension.
func MigrateSQLite(ctx context.Context, db SQLRunner) error {
	schema, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("read schema: %w", err)
	}
	if _, err := db.ExecContext(ctx, string(schema)); err != nil {
		return fmt.Errorf("migrate db: %w", err)
	}
	if err := ensureAccountColumns(ctx, db); err != nil {
		return fmt.Errorf("migrate accounts: %w", err)
	}
	return nil
}

func ensureAccountColumns(ctx context.Context, db SQLRunner) error {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info(accounts)")
	if err != nil {
		return err
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var (
			cid      int
			name     string
			typ      string
			notNull  int
			defaultV sql.NullString
			primaryK int
		)
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultV, &primaryK); err != nil {
			return err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, stmt := range []struct {
		column string
		sql    string
	}{
		{"class", "ALTER TABLE accounts ADD COLUMN class TEXT NOT NULL DEFAULT 'Other'"},
		{"currency", "ALTER TABLE accounts ADD COLUMN currency TEXT NOT NULL DEFAULT 'EUR'"},
		{"active_from", "ALTER TABLE accounts ADD COLUMN active_from TEXT NOT NULL DEFAULT ''"},
		{"active_to", "ALTER TABLE accounts ADD COLUMN active_to TEXT NOT NULL DEFAULT ''"},
		{"note", "ALTER TABLE accounts ADD COLUMN note TEXT NOT NULL DEFAULT ''"},
		{"created_at", "ALTER TABLE accounts ADD COLUMN created_at TEXT NOT NULL DEFAULT ''"},
	} {
		if columns[stmt.column] {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt.sql); err != nil {
			return fmt.Errorf("add column %s: %w", stmt.column, err)
		}
	}
	return nil
}
