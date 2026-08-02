package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestMigrateAddsAccountColumnsToLegacyDatabase(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`
		CREATE TABLE accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			type TEXT NOT NULL CHECK (type IN ('Asset', 'Liability'))
		);
		INSERT INTO accounts (name, type) VALUES ('Wallet', 'Asset');
	`); err != nil {
		t.Fatal(err)
	}

	if err := MigrateSQLite(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	var class, currency, activeFrom, activeTo, note string
	if err := db.QueryRow(`
		SELECT class, currency, active_from, active_to, note
		FROM accounts
		WHERE name = 'Wallet'
	`).Scan(&class, &currency, &activeFrom, &activeTo, &note); err != nil {
		t.Fatal(err)
	}

	if class != "Other" {
		t.Fatalf("class = %q, want Other", class)
	}
	if currency != "EUR" {
		t.Fatalf("currency = %q, want EUR", currency)
	}
	if activeFrom != "" || activeTo != "" || note != "" {
		t.Fatalf("expected empty legacy metadata, got active_from=%q active_to=%q note=%q", activeFrom, activeTo, note)
	}
}

func TestOpenConcurrentLegacyDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "spese.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE accounts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			type TEXT NOT NULL CHECK (type IN ('Asset', 'Liability'))
		);
		INSERT INTO accounts (name, type) VALUES ('Wallet', 'Asset');
	`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var stores [2]*Store
	var wg sync.WaitGroup
	for i := range stores {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			store, err := Open(context.Background(), dbPath)
			stores[i] = store
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for _, store := range stores {
		if store != nil {
			defer store.Close()
		}
	}
	for err := range errs {
		if err != nil {
			t.Errorf("Open() error = %v", err)
		}
	}
	if t.Failed() {
		return
	}

	var count int
	if err := stores[0].DB().QueryRow("SELECT count(*) FROM accounts WHERE name = 'Wallet'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("migrated Wallet accounts = %d, want 1", count)
	}
}

func TestEnqueueSheetSyncUsesTransaction(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.SetSheetSyncEnabled(true)

	tx, err := store.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnqueueSheetSync(tx, "accounts"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if got := countOutboxRows(t, store.DB()); got != 0 {
		t.Fatalf("outbox rows after rollback = %d, want 0", got)
	}

	tx, err = store.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnqueueSheetSync(tx, "transactions"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := countOutboxRows(t, store.DB()); got != 1 {
		t.Fatalf("outbox rows after commit = %d, want 1", got)
	}

	var payloadJSON string
	if err := store.DB().QueryRow("SELECT payload_json FROM sheet_sync_outbox").Scan(&payloadJSON); err != nil {
		t.Fatal(err)
	}
	var payload SheetSyncPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Version != 1 || payload.Scope != "transactions" || payload.OutboxID == 0 {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestEnqueueSheetSyncDisabledIsNoop(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	tx, err := store.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnqueueSheetSync(tx, "accounts"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if got := countOutboxRows(t, store.DB()); got != 0 {
		t.Fatalf("outbox rows = %d, want 0", got)
	}
}

func TestClaimAndMarkSheetSyncOutbox(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.EnqueueSheetSyncEvent(ctx, SheetSyncBootstrapScope); err != nil {
		t.Fatal(err)
	}
	msg, ok, err := store.ClaimSheetSyncOutbox(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected claimed message")
	}
	if msg.Scope != SheetSyncBootstrapScope || msg.ID == 0 || len(msg.Payload) == 0 || msg.Attempts != 1 {
		t.Fatalf("message = %+v", msg)
	}
	if err := store.MarkSheetSyncPublished(ctx, msg.ID); err != nil {
		t.Fatal(err)
	}
	msg, ok, err = store.ClaimSheetSyncOutbox(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("unexpected message after publish: %+v", msg)
	}
}

func countOutboxRows(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT count(*) FROM sheet_sync_outbox").Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
