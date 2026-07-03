// Package storage owns the local SQLite database and sheet-sync outbox.
package storage

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const SheetSyncOutboxName = "sheet-sync"
const SheetSyncBootstrapScope = "bootstrap"

//go:embed schema.sql
var schemaFS embed.FS

// Store wraps the SQLite handle so feature slices can share one local source
// of truth and enqueue sheet-sync work transactionally.
type Store struct {
	db               *sql.DB
	sheetSyncEnabled bool
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
	Version  int    `json:"version"`
	OutboxID int64  `json:"outbox_id"`
	Scope    string `json:"scope"`
}

// Open opens the local SQLite database and applies schema.
func Open(ctx context.Context, dbPath string) (*Store, error) {
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

// SetSheetSyncEnabled controls whether feature writes append sheet-sync outbox
// messages. The worker opens the same store with this left disabled.
func (s *Store) SetSheetSyncEnabled(enabled bool) {
	s.sheetSyncEnabled = enabled
}

// Begin starts a SQLite transaction. Writes that need sync should call
// EnqueueSheetSync before Commit.
func (s *Store) Begin(ctx context.Context) (Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	return tx, nil
}

// EnqueueSheetSync writes a durable outbox message in the supplied
// transaction. This is the transactional outbox for the sheet mirror.
func (s *Store) EnqueueSheetSync(tx Tx, scope string) error {
	if !s.sheetSyncEnabled {
		return nil
	}
	return insertSheetSyncOutbox(tx, scope)
}

// EnqueueSheetSyncEvent writes an outbox message outside a feature transaction.
func (s *Store) EnqueueSheetSyncEvent(ctx context.Context, scope string) error {
	tx, err := s.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := insertSheetSyncOutbox(tx, scope); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sheet sync outbox: %w", err)
	}
	return nil
}

func insertSheetSyncOutbox(tx Tx, scope string) error {
	now := utcNow()
	res, err := tx.Exec(`
		INSERT INTO sheet_sync_outbox (
			scope, payload_json, status, available_at, created_at, updated_at
		) VALUES (?, '', 'pending', ?, ?, ?)
	`, scope, now, now, now)
	if err != nil {
		return fmt.Errorf("enqueue sheet sync: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("sheet sync outbox id: %w", err)
	}
	payload, err := json.Marshal(SheetSyncPayload{Version: 1, OutboxID: id, Scope: scope})
	if err != nil {
		return fmt.Errorf("encode sheet sync payload: %w", err)
	}
	if _, err := tx.Exec(`
		UPDATE sheet_sync_outbox
		SET payload_json = ?, updated_at = ?
		WHERE id = ?
	`, string(payload), utcNow(), id); err != nil {
		return fmt.Errorf("store sheet sync payload: %w", err)
	}
	return nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	return s.db.Close()
}

// SheetSyncOutboxMessage is one claimed message ready for RabbitMQ publish.
type SheetSyncOutboxMessage struct {
	ID       int64
	Scope    string
	Payload  []byte
	Attempts int
}

// ClaimSheetSyncOutbox claims the next due outbox message. A publishing row
// older than staleAfter is eligible so crashes cannot strand messages forever.
func (s *Store) ClaimSheetSyncOutbox(ctx context.Context, staleAfter time.Duration) (SheetSyncOutboxMessage, bool, error) {
	now := time.Now().UTC()
	staleBefore := now.Add(-staleAfter).Format(time.RFC3339Nano)
	nowStr := now.Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SheetSyncOutboxMessage{}, false, fmt.Errorf("begin outbox claim: %w", err)
	}
	defer tx.Rollback()

	var msg SheetSyncOutboxMessage
	var payload string
	err = tx.QueryRowContext(ctx, `
		SELECT id, scope, payload_json, attempts
		FROM sheet_sync_outbox
		WHERE published_at = ''
		  AND (
			(status = 'pending' AND available_at <= ?)
			OR (status = 'publishing' AND updated_at <= ?)
		  )
		ORDER BY id
		LIMIT 1
	`, nowStr, staleBefore).Scan(&msg.ID, &msg.Scope, &payload, &msg.Attempts)
	if err == sql.ErrNoRows {
		if err := tx.Commit(); err != nil {
			return SheetSyncOutboxMessage{}, false, fmt.Errorf("commit empty outbox claim: %w", err)
		}
		return SheetSyncOutboxMessage{}, false, nil
	}
	if err != nil {
		return SheetSyncOutboxMessage{}, false, fmt.Errorf("select outbox message: %w", err)
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE sheet_sync_outbox
		SET status = 'publishing', attempts = attempts + 1, updated_at = ?
		WHERE id = ? AND published_at = ''
	`, nowStr, msg.ID)
	if err != nil {
		return SheetSyncOutboxMessage{}, false, fmt.Errorf("claim outbox message: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return SheetSyncOutboxMessage{}, false, fmt.Errorf("claim outbox rows affected: %w", err)
	}
	if affected == 0 {
		if err := tx.Commit(); err != nil {
			return SheetSyncOutboxMessage{}, false, fmt.Errorf("commit skipped outbox claim: %w", err)
		}
		return SheetSyncOutboxMessage{}, false, nil
	}
	if err := tx.Commit(); err != nil {
		return SheetSyncOutboxMessage{}, false, fmt.Errorf("commit outbox claim: %w", err)
	}

	msg.Payload = []byte(payload)
	msg.Attempts++
	return msg, true, nil
}

// MarkSheetSyncPublished records that RabbitMQ confirmed a published outbox
// message.
func (s *Store) MarkSheetSyncPublished(ctx context.Context, id int64) error {
	now := utcNow()
	if _, err := s.db.ExecContext(ctx, `
		UPDATE sheet_sync_outbox
		SET status = 'published', published_at = ?, updated_at = ?, last_error = ''
		WHERE id = ?
	`, now, now, id); err != nil {
		return fmt.Errorf("mark sheet sync published: %w", err)
	}
	return nil
}

// MarkSheetSyncPublishFailed makes a failed message retryable after delay.
func (s *Store) MarkSheetSyncPublishFailed(ctx context.Context, id int64, cause error, delay time.Duration) error {
	now := time.Now().UTC()
	available := now.Add(delay).Format(time.RFC3339Nano)
	errMsg := ""
	if cause != nil {
		errMsg = cause.Error()
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE sheet_sync_outbox
		SET status = 'pending', available_at = ?, last_error = ?, updated_at = ?
		WHERE id = ? AND published_at = ''
	`, available, errMsg, now.Format(time.RFC3339Nano), id); err != nil {
		return fmt.Errorf("mark sheet sync publish failed: %w", err)
	}
	return nil
}

func utcNow() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func (s *Store) migrate(ctx context.Context) error {
	return MigrateSQLite(ctx, s.db)
}

// MigrateSQLite applies the app schema to a plain SQLite handle.
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
