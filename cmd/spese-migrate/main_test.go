package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestMigrationDryRunConvertsLedgerAndMonthEndSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		PRAGMA foreign_keys=ON;
		CREATE TABLE accounts (id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, type TEXT NOT NULL, class TEXT NOT NULL, currency TEXT NOT NULL, active_from TEXT NOT NULL DEFAULT '', active_to TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL);
		CREATE TABLE transactions (id INTEGER PRIMARY KEY, date TEXT NOT NULL, kind TEXT NOT NULL, account TEXT NOT NULL, amount_cents INTEGER NOT NULL, category TEXT NOT NULL DEFAULT '', subcategory TEXT NOT NULL DEFAULT '', payee TEXT NOT NULL DEFAULT '', note TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL);
		CREATE TABLE snapshot_batches (id INTEGER PRIMARY KEY, effective_month TEXT NOT NULL, captured_at TEXT NOT NULL, note TEXT NOT NULL DEFAULT '');
		CREATE TABLE snapshot_balances (id INTEGER PRIMARY KEY, batch_id INTEGER NOT NULL, account TEXT NOT NULL, balance_cents INTEGER NOT NULL, note TEXT NOT NULL DEFAULT '');
		INSERT INTO accounts VALUES (1,'Conto','Asset','Cash','EUR','','','','2026-01-01T00:00:00Z');
		INSERT INTO accounts VALUES (2,'Carta','Liability','Credit','EUR','','','','2026-01-01T00:00:00Z');
		INSERT INTO transactions VALUES (1,'2026-02-02','Expense','Conto',-1200,'Casa','Utenze','Energia','','2026-02-02T12:00:00Z');
		INSERT INTO transactions VALUES (2,'2026-02-03','Transfer','Conto',-500,'','','to Carta','giro','2026-02-03T12:00:00Z');
		INSERT INTO transactions VALUES (3,'2026-02-03','Transfer','Carta',500,'','','from Conto','giro','2026-02-03T12:00:01Z');
		INSERT INTO snapshot_batches VALUES (1,'2026-02','2026-03-01T09:00:00Z','');
		INSERT INTO snapshot_balances VALUES (1,1,'Conto',8300,'');
	`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	copy, cleanup, err := disposableCopy(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	report, err := migrateAndReport(context.Background(), copy)
	if err != nil {
		t.Fatal(err)
	}
	if report.Movements != 2 || report.Postings != 3 || report.Reconciliations != 1 {
		t.Fatalf("unexpected report: %#v", report)
	}
	migrated, err := sql.Open("sqlite3", copy)
	if err != nil {
		t.Fatal(err)
	}
	defer migrated.Close()
	var closedThrough string
	if err := migrated.QueryRow("SELECT closed_through FROM account_reconciliations").Scan(&closedThrough); err != nil {
		t.Fatal(err)
	}
	if closedThrough != "2026-02-28" {
		t.Fatalf("closed_through = %s, want month end", closedThrough)
	}
	var legacyCount int
	if err := migrated.QueryRow("SELECT count(*) FROM legacy_transactions").Scan(&legacyCount); err != nil || legacyCount != 3 {
		t.Fatalf("legacy rows = %d, err = %v", legacyCount, err)
	}
}

func TestRestorePreservesCurrentDatabase(t *testing.T) {
	directory := t.TempDir()
	database := filepath.Join(directory, "spese.db")
	backup := filepath.Join(directory, "backup.db")
	if err := os.WriteFile(database, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", backup)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE marker (value TEXT); INSERT INTO marker VALUES ('backup')"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if err := restoreDatabase(database, backup); err != nil {
		t.Fatal(err)
	}
	restored, err := sql.Open("sqlite3", database)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var value string
	if err := restored.QueryRow("SELECT value FROM marker").Scan(&value); err != nil || value != "backup" {
		t.Fatalf("restored marker = %q, err = %v", value, err)
	}
	matches, err := filepath.Glob(database + ".before-restore-*")
	if err != nil || len(matches) != 1 {
		t.Fatalf("preserved copies = %v, err = %v", matches, err)
	}
}
