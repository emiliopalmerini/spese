package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"spese/internal/storage"
)

type report struct {
	Mode            string          `json:"mode"`
	Database        string          `json:"database"`
	Backup          string          `json:"backup,omitempty"`
	SchemaVersions  int             `json:"schemaVersions"`
	LegacyTables    int             `json:"legacyTables"`
	Accounts        int             `json:"accounts"`
	Categories      int             `json:"categories"`
	Movements       int             `json:"movements"`
	Postings        int             `json:"postings"`
	Allocations     int             `json:"allocations"`
	Reconciliations int             `json:"reconciliations"`
	MonthlyTotals   map[string]sums `json:"monthlyTotals"`
}

type sums struct {
	IncomeCents  int64 `json:"incomeCents"`
	ExpenseCents int64 `json:"expenseCents"`
}

func main() {
	database := flag.String("db", "spese.db", "SQLite database to inspect or migrate")
	dryRun := flag.Bool("dry-run", true, "migrate an isolated copy and print the validation report")
	backup := flag.String("backup", "", "backup destination for an applied migration")
	restore := flag.String("restore", "", "restore this backup over -db after preserving the current file")
	flag.Parse()
	ctx := context.Background()
	if *restore != "" {
		if err := restoreDatabase(*database, *restore); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("restored %s from %s\n", *database, *restore)
		return
	}
	if *dryRun {
		temporary, cleanup, err := disposableCopy(ctx, *database)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer cleanup()
		result, err := migrateAndReport(ctx, temporary)
		if err != nil {
			fmt.Fprintln(os.Stderr, "dry-run failed:", err)
			os.Exit(1)
		}
		result.Mode, result.Database = "dry-run", *database
		writeReport(result)
		return
	}
	backupPath := *backup
	if backupPath == "" {
		backupPath = *database + ".backup-" + time.Now().UTC().Format("20060102T150405Z")
	}
	if err := vacuumCopy(ctx, *database, backupPath); err != nil {
		fmt.Fprintln(os.Stderr, "backup failed:", err)
		os.Exit(1)
	}
	result, err := migrateAndReport(ctx, *database)
	if err != nil {
		fmt.Fprintln(os.Stderr, "migration failed; restore with -restore", backupPath, ":", err)
		os.Exit(1)
	}
	result.Mode, result.Database, result.Backup = "apply", *database, backupPath
	writeReport(result)
}

func migrateAndReport(ctx context.Context, path string) (report, error) {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return report{}, err
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;"); err != nil {
		return report{}, err
	}
	if err := storage.MigrateSQLite(ctx, db); err != nil {
		return report{}, err
	}
	result := report{MonthlyTotals: make(map[string]sums)}
	counts := []struct {
		query string
		value *int
	}{
		{"SELECT count(*) FROM schema_migrations", &result.SchemaVersions},
		{"SELECT count(*) FROM sqlite_master WHERE type='table' AND name LIKE 'legacy_%'", &result.LegacyTables},
		{"SELECT count(*) FROM accounts", &result.Accounts},
		{"SELECT count(*) FROM categories", &result.Categories},
		{"SELECT count(*) FROM movements", &result.Movements},
		{"SELECT count(*) FROM postings", &result.Postings},
		{"SELECT count(*) FROM movement_allocations", &result.Allocations},
		{"SELECT count(*) FROM account_reconciliations", &result.Reconciliations},
	}
	for _, count := range counts {
		if err := db.QueryRowContext(ctx, count.query).Scan(count.value); err != nil {
			return report{}, err
		}
	}
	rows, err := db.QueryContext(ctx, `
		SELECT substr(business_date,1,7),
			coalesce(sum(CASE WHEN kind='income' AND status='posted' THEN amount_cents ELSE 0 END),0),
			coalesce(sum(CASE WHEN kind='expense' AND status='posted' THEN amount_cents ELSE 0 END),0)
		FROM movements GROUP BY 1 ORDER BY 1
	`)
	if err != nil {
		return report{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var month string
		var total sums
		if err := rows.Scan(&month, &total.IncomeCents, &total.ExpenseCents); err != nil {
			return report{}, err
		}
		result.MonthlyTotals[month] = total
	}
	var invalidTransfers int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FROM (
			SELECT m.id FROM movements m JOIN postings p ON p.movement_id=m.id
			WHERE m.kind='transfer' GROUP BY m.id HAVING count(*)<>2 OR sum(p.amount_cents)<>0
		)
	`).Scan(&invalidTransfers); err != nil {
		return report{}, err
	}
	if invalidTransfers != 0 {
		return report{}, fmt.Errorf("%d transfers are not zero-sum", invalidTransfers)
	}
	return result, rows.Err()
}

func disposableCopy(ctx context.Context, source string) (string, func(), error) {
	directory, err := os.MkdirTemp("", "spese-migration-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	destination := filepath.Join(directory, "dry-run.db")
	if err := vacuumCopy(ctx, source, destination); err != nil {
		cleanup()
		return "", nil, err
	}
	return destination, cleanup, nil
}

func vacuumCopy(ctx context.Context, source, destination string) error {
	if _, err := os.Stat(destination); err == nil {
		return fmt.Errorf("destination already exists: %s", destination)
	}
	db, err := sql.Open("sqlite3", source)
	if err != nil {
		return err
	}
	defer db.Close()
	escaped := strings.ReplaceAll(destination, "'", "''")
	if _, err := db.ExecContext(ctx, "VACUUM INTO '"+escaped+"'"); err != nil {
		return err
	}
	return nil
}

func restoreDatabase(destination, backup string) error {
	check, err := sql.Open("sqlite3", "file:"+backup+"?mode=ro")
	if err != nil {
		return err
	}
	if err := check.Ping(); err != nil {
		check.Close()
		return fmt.Errorf("invalid SQLite backup: %w", err)
	}
	check.Close()
	if _, err := os.Stat(destination); err == nil {
		preserved := destination + ".before-restore-" + time.Now().UTC().Format("20060102T150405Z")
		if err := copyFile(destination, preserved); err != nil {
			return fmt.Errorf("preserve current database: %w", err)
		}
	}
	return copyFile(backup, destination)
}

func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func writeReport(value report) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
