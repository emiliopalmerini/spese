// Package storage owns the local SQLite database and sheet-sync outbox.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"
)

const SheetSyncOutboxName = "sheet-sync"
const SheetSyncBootstrapScope = "bootstrap"

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
	separator := "?"
	if strings.Contains(dbPath, "?") {
		separator = "&"
	}
	dsn := dbPath + separator + "_busy_timeout=5000&_txlock=immediate"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	// SQLite pragmas are connection-local. A single pooled connection preserves
	// foreign-key enforcement and serializes writers consistently.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	var pragmaErr error
pragmaLoop:
	for attempt := 0; attempt < 100; attempt++ {
		_, pragmaErr = db.ExecContext(ctx, `
			PRAGMA busy_timeout = 5000;
			PRAGMA journal_mode = WAL;
			PRAGMA synchronous = NORMAL;
			PRAGMA foreign_keys = ON;
			PRAGMA cache_size = -32000;
			PRAGMA temp_store = MEMORY;
		`)
		if pragmaErr == nil {
			break
		}
		var sqliteErr sqlite3.Error
		if !errors.As(pragmaErr, &sqliteErr) || (sqliteErr.Code != sqlite3.ErrBusy && sqliteErr.Code != sqlite3.ErrLocked) {
			break
		}
		select {
		case <-ctx.Done():
			pragmaErr = ctx.Err()
			break pragmaLoop
		case <-time.After(50 * time.Millisecond):
		}
	}
	if pragmaErr != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite pragmas: %w", pragmaErr)
	}
	if _, err := backupBeforeLegacyMigration(ctx, db, dbPath); err != nil {
		_ = db.Close()
		return nil, err
	}
	st := &Store{db: db}
	if err := st.migrate(ctx); err != nil {
		_ = st.Close()
		return nil, err
	}
	return st, nil
}

func backupBeforeLegacyMigration(ctx context.Context, db *sql.DB, dbPath string) (string, error) {
	if dbPath == ":memory:" || strings.HasPrefix(dbPath, "file:") {
		return "", nil
	}
	info, err := os.Stat(dbPath)
	if errors.Is(err, os.ErrNotExist) || err == nil && info.Size() == 0 {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("inspect database before migration: %w", err)
	}
	var legacyTables int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name <> 'schema_migrations'
			AND NOT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations')
	`).Scan(&legacyTables); err != nil {
		return "", fmt.Errorf("inspect legacy database: %w", err)
	}
	if legacyTables == 0 {
		return "", nil
	}
	backupPattern := filepath.Base(dbPath) + ".pre-v2-" + time.Now().UTC().Format("20060102T150405Z") + "-*.bak"
	backupFile, err := os.CreateTemp(filepath.Dir(dbPath), backupPattern)
	if err != nil {
		return "", fmt.Errorf("allocate legacy database backup: %w", err)
	}
	backupPath := backupFile.Name()
	if err := backupFile.Close(); err != nil {
		return "", fmt.Errorf("close legacy database backup placeholder: %w", err)
	}
	if err := os.Remove(backupPath); err != nil {
		return "", fmt.Errorf("prepare legacy database backup: %w", err)
	}
	escaped := strings.ReplaceAll(backupPath, "'", "''")
	if _, err := db.ExecContext(ctx, "VACUUM INTO '"+escaped+"'"); err != nil {
		return "", fmt.Errorf("backup legacy database to %s: %w", backupPath, err)
	}
	return backupPath, nil
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

// MigrateSQLite applies ordered, transactional migrations to a SQLite handle.
func MigrateSQLite(ctx context.Context, db SQLRunner) error {
	return runMigrations(ctx, db)
}
