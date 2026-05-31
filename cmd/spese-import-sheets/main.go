package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"spese/internal/kernel"
	"spese/internal/sheets"
	"spese/internal/storage"
)

func main() {
	var (
		dbPath        = flag.String("db", envOr("SPESE_DB_PATH", "spese.db"), "SQLite database path")
		spreadsheetID = flag.String("spreadsheet-id", envOr("GOOGLE_SPREADSHEET_ID", ""), "Google spreadsheet ID")
		credentials   = flag.String("credentials", envOr("GOOGLE_SERVICE_ACCOUNT_FILE", ""), "Google service account JSON path")
		replace       = flag.Bool("replace", true, "delete existing app rows before import")
	)
	flag.Parse()

	if strings.TrimSpace(*spreadsheetID) == "" {
		fatal(errors.New("spreadsheet ID is required"))
	}
	if strings.TrimSpace(*credentials) == "" {
		fatal(errors.New("credentials path is required"))
	}

	ctx := context.Background()
	client, err := sheets.New(ctx, *credentials, *spreadsheetID)
	if err != nil {
		fatal(err)
	}
	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		fatal(err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		fatal(err)
	}
	defer tx.Rollback()

	if *replace {
		if err := recreateAppTables(ctx, tx); err != nil {
			fatal(err)
		}
	}
	if err := storage.MigrateSQLite(ctx, tx); err != nil {
		fatal(err)
	}

	accounts, err := importAccounts(ctx, tx, client)
	if err != nil {
		fatal(err)
	}
	transactions, err := importTransactions(ctx, tx, client)
	if err != nil {
		fatal(err)
	}
	snapshots, err := importSnapshots(ctx, tx, client)
	if err != nil {
		fatal(err)
	}
	if err := tx.Commit(); err != nil {
		fatal(err)
	}

	fmt.Printf("imported %d accounts, %d transactions, %d snapshots into %s\n", accounts, transactions, snapshots, *dbPath)
}

func recreateAppTables(ctx context.Context, tx *sql.Tx) error {
	for _, stmt := range []string{
		"DROP TABLE IF EXISTS snapshot_balances",
		"DROP TABLE IF EXISTS snapshot_batches",
		"DROP TABLE IF EXISTS transactions",
		"DROP TABLE IF EXISTS accounts",
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("%s: %w", stmt, err)
		}
	}
	return nil
}

type rangeReader interface {
	ReadRange(ctx context.Context, rangeA1 string, force bool) ([][]any, error)
}

func importAccounts(ctx context.Context, tx *sql.Tx, client rangeReader) (int, error) {
	headers, rows, err := readSheet(ctx, client, "accounts", [][]string{
		{"name", "account", "conto", "nome"},
		{"type", "tipo"},
	})
	if err != nil {
		return 0, err
	}
	index := headerIndex(headers)
	count := 0
	for i, row := range rows {
		if emptyRow(row) {
			continue
		}
		name := value(index, row, "name", "account", "conto", "nome")
		typ := value(index, row, "type", "tipo")
		if name == "" || typ == "" {
			return 0, fmt.Errorf("accounts row %d: name and type are required", i+2)
		}
		class := defaultValue(value(index, row, "class", "classe"), "Other")
		typ, class = normaliseAccountTypeClass(typ, class)
		currency := defaultValue(value(index, row, "currency", "valuta"), "EUR")
		activeFrom := value(index, row, "active_from", "active from", "attivo_da")
		activeTo := value(index, row, "active_to", "active to", "attivo_a")
		note := value(index, row, "note", "nota")
		if activeFrom != "" {
			d, err := parseDate(activeFrom)
			if err != nil {
				return 0, fmt.Errorf("accounts row %d active_from: %w", i+2, err)
			}
			activeFrom = d.Month()
		}
		if activeTo != "" {
			d, err := parseDate(activeTo)
			if err != nil {
				return 0, fmt.Errorf("accounts row %d active_to: %w", i+2, err)
			}
			activeTo = d.Month()
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO accounts (name, type, class, currency, active_from, active_to, note)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(name) DO UPDATE SET
				type = excluded.type,
				class = excluded.class,
				currency = excluded.currency,
				active_from = excluded.active_from,
				active_to = excluded.active_to,
				note = excluded.note
		`, name, typ, class, currency, activeFrom, activeTo, note); err != nil {
			return 0, fmt.Errorf("accounts row %d: %w", i+2, err)
		}
		count++
	}
	return count, nil
}

func importTransactions(ctx context.Context, tx *sql.Tx, client rangeReader) (int, error) {
	headers, rows, err := readSheet(ctx, client, "transactions", [][]string{
		{"date", "data"},
		{"kind", "type", "tipo"},
		{"account", "conto"},
		{"amount", "amount_eur", "amount_cents", "importo", "importo_eur"},
	})
	if err != nil {
		return 0, err
	}
	index := headerIndex(headers)
	count := 0
	for i, row := range rows {
		if emptyRow(row) {
			continue
		}
		date, err := parseDate(value(index, row, "date", "data"))
		if err != nil {
			return 0, fmt.Errorf("transactions row %d date: %w", i+2, err)
		}
		kind := value(index, row, "kind", "type", "tipo")
		account := value(index, row, "account", "conto")
		if kind == "" || account == "" {
			return 0, fmt.Errorf("transactions row %d: kind and account are required", i+2)
		}
		amount, err := moneyValue(index, row, "amount", "amount_eur", "importo", "importo_eur", "amount_cents")
		if err != nil {
			return 0, fmt.Errorf("transactions row %d amount: %w", i+2, err)
		}
		args := []any{
			date.ISO(),
			kind,
			account,
			amount,
			value(index, row, "category", "categoria"),
			value(index, row, "subcategory", "sub_category", "sottocategoria"),
			value(index, row, "payee", "beneficiario"),
			value(index, row, "note", "nota"),
		}
		stmt := `
			INSERT INTO transactions (date, kind, account, amount_cents, category, subcategory, payee, note)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`
		if _, err := tx.ExecContext(ctx, stmt, args...); err != nil {
			return 0, fmt.Errorf("transactions row %d: %w", i+2, err)
		}
		count++
	}
	return count, nil
}

func importSnapshots(ctx context.Context, tx *sql.Tx, client rangeReader) (int, error) {
	headers, rows, err := readSheet(ctx, client, "snapshots", [][]string{
		{"month", "mese", "effective_month"},
		{"account", "conto"},
		{"balance", "balance_eur", "balance_cents", "saldo", "saldo_eur"},
	})
	if err != nil {
		return 0, err
	}
	index := headerIndex(headers)
	batches := make(map[string]int64)
	count := 0
	for i, row := range rows {
		if emptyRow(row) {
			continue
		}
		month, err := parseDate(value(index, row, "month", "mese", "effective_month"))
		if err != nil {
			return 0, fmt.Errorf("snapshots row %d month: %w", i+2, err)
		}
		monthKey := month.FirstOfMonth().Month()
		account := value(index, row, "account", "conto")
		if account == "" {
			return 0, fmt.Errorf("snapshots row %d: account is required", i+2)
		}
		balance, err := moneyValue(index, row, "balance", "balance_eur", "saldo", "saldo_eur", "balance_cents")
		if err != nil {
			return 0, fmt.Errorf("snapshots row %d balance: %w", i+2, err)
		}
		batchID, ok := batches[monthKey]
		if !ok {
			res, err := tx.ExecContext(ctx, "INSERT INTO snapshot_batches (effective_month) VALUES (?)", monthKey)
			if err != nil {
				return 0, fmt.Errorf("snapshots row %d batch: %w", i+2, err)
			}
			batchID, err = res.LastInsertId()
			if err != nil {
				return 0, fmt.Errorf("snapshots row %d batch id: %w", i+2, err)
			}
			batches[monthKey] = batchID
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO snapshot_balances (batch_id, account, balance_cents, note)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(batch_id, account) DO UPDATE SET
				balance_cents = excluded.balance_cents,
				note = excluded.note
		`, batchID, account, balance, value(index, row, "note", "nota")); err != nil {
			return 0, fmt.Errorf("snapshots row %d: %w", i+2, err)
		}
		count++
	}
	return count, nil
}

func readSheet(ctx context.Context, client rangeReader, tab string, required [][]string) ([]string, [][]any, error) {
	data, err := client.ReadRange(ctx, tab, true)
	if err != nil {
		return nil, nil, err
	}
	for i, row := range data {
		if hasRequiredHeaders(row, required) {
			headers := make([]string, 0, len(row))
			for _, v := range row {
				headers = append(headers, fmt.Sprint(v))
			}
			return headers, data[i+1:], nil
		}
	}
	return nil, nil, fmt.Errorf("%s: header row not found; first rows: %s", tab, previewRows(data, 5))
}

func hasRequiredHeaders(row []any, required [][]string) bool {
	seen := make(map[string]bool, len(row))
	for _, cell := range row {
		seen[normalise(fmt.Sprint(cell))] = true
	}
	for _, alternatives := range required {
		found := false
		for _, name := range alternatives {
			if seen[normalise(name)] {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func headerIndex(headers []string) map[string]int {
	out := make(map[string]int, len(headers))
	for i, h := range headers {
		out[normalise(h)] = i
	}
	return out
}

func value(index map[string]int, row []any, names ...string) string {
	for _, name := range names {
		i, ok := index[normalise(name)]
		if !ok || i >= len(row) {
			continue
		}
		return strings.TrimSpace(fmt.Sprint(row[i]))
	}
	return ""
}

func moneyValue(index map[string]int, row []any, names ...string) (int64, error) {
	for _, name := range names {
		raw := value(index, row, name)
		if raw == "" {
			continue
		}
		if normalise(name) == "amount_cents" || normalise(name) == "balance_cents" {
			return strconv.ParseInt(raw, 10, 64)
		}
		m, err := kernel.ParseMoney(raw)
		return int64(m), err
	}
	return 0, errors.New("amount is required")
}

func normaliseAccountTypeClass(typ, class string) (string, string) {
	switch normalise(typ) {
	case "asset", "assets", "attivo", "attivita":
		return "Asset", normaliseAccountClass(class)
	case "liability", "liabilities", "passivo", "passivita", "debito", "debiti":
		return "Liability", normaliseAccountClass(class)
	case "cash", "contanti", "conto_corrente":
		return "Asset", "Cash"
	case "rainy_day", "emergenza", "fondo_emergenza":
		return "Asset", "Cash"
	case "long_term", "investimenti", "investimento", "investment":
		return "Asset", "Investment"
	case "property", "immobile", "immobili":
		return "Asset", "Property"
	case "tax", "taxes", "tasse":
		return "Liability", "Tax"
	case "credit", "credito", "carta":
		return "Liability", "Credit"
	default:
		return typ, normaliseAccountClass(class)
	}
}

func normaliseAccountClass(class string) string {
	switch normalise(class) {
	case "cash", "contanti", "conto_corrente":
		return "Cash"
	case "investment", "investments", "investimento", "investimenti", "long_term":
		return "Investment"
	case "property", "immobile", "immobili":
		return "Property"
	case "tax", "taxes", "tasse":
		return "Tax"
	case "credit", "credito", "carta":
		return "Credit"
	case "", "other", "altro":
		return "Other"
	default:
		return class
	}
}

func parseDate(raw string) (kernel.Date, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return kernel.Date{}, errors.New("date is required")
	}
	if d, err := kernel.ParseDate(raw); err == nil {
		return d, nil
	}
	for _, layout := range []string{"02/01/2006", "2/1/2006"} {
		t, err := time.ParseInLocation(layout, raw, time.Local)
		if err == nil {
			return kernel.Date{Time: t}, nil
		}
	}
	for _, layout := range []string{"01/2006", "1/2006"} {
		t, err := time.ParseInLocation(layout, raw, time.Local)
		if err == nil {
			return kernel.Date{Time: t}.FirstOfMonth(), nil
		}
	}
	return kernel.Date{}, fmt.Errorf("parse date %q", raw)
}

func emptyRow(row []any) bool {
	for _, cell := range row {
		if strings.TrimSpace(fmt.Sprint(cell)) != "" {
			return false
		}
	}
	return true
}

func previewRows(rows [][]any, limit int) string {
	if len(rows) < limit {
		limit = len(rows)
	}
	parts := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		parts = append(parts, fmt.Sprintf("%d=%v", i+1, rows[i]))
	}
	return strings.Join(parts, "; ")
}

func defaultValue(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func normalise(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.NewReplacer(" ", "_", "-", "_").Replace(s)
	return s
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "spese-import-sheets:", err)
	os.Exit(1)
}
