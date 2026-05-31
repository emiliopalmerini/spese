package storage

import (
	"context"
	"database/sql"
	"testing"

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

	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
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
