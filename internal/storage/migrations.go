package storage

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

var migrationNamespace = uuid.MustParse("9672b4e9-b9f4-5d86-b78a-a5aa1161cf3c")

// MigrationReport records a legacy conversion. Ambiguous transfers block the
// transaction instead of being silently paired.
type MigrationReport struct {
	LegacyTables       []string
	Accounts           int
	Categories         int
	Movements          int
	Transfers          int
	Reconciliations    int
	AmbiguousTransfers []string
}

func runMigrations(ctx context.Context, runner SQLRunner) error {
	if db, ok := runner.(*sql.DB); ok {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration: %w", err)
		}
		defer tx.Rollback()
		if err := migrateRunner(ctx, tx); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration: %w", err)
		}
		return nil
	}
	return migrateRunner(ctx, runner)
}

func migrateRunner(ctx context.Context, runner SQLRunner) error {
	if _, err := runner.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TEXT NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create schema migrations: %w", err)
	}
	applied, err := appliedMigrations(ctx, runner)
	if err != nil {
		return err
	}
	legacy := false
	if len(applied) == 0 {
		legacy, err = prepareLegacySchema(ctx, runner)
		if err != nil {
			return err
		}
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version, err := strconv.Atoi(strings.SplitN(entry.Name(), "_", 2)[0])
		if err != nil {
			return fmt.Errorf("invalid migration name %q", entry.Name())
		}
		if applied[version] {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		if _, err := runner.ExecContext(ctx, string(body)); err != nil {
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
		if legacy && version == 2 {
			report, err := migrateLegacyData(ctx, runner)
			if err != nil {
				if len(report.AmbiguousTransfers) > 0 {
					return fmt.Errorf("legacy migration blocked by ambiguous transfers: %s", strings.Join(report.AmbiguousTransfers, "; "))
				}
				return fmt.Errorf("migrate legacy data: %w", err)
			}
		}
		if _, err := runner.ExecContext(ctx,
			"INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)",
			version, entry.Name(), utcNow()); err != nil {
			return fmt.Errorf("record migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func appliedMigrations(ctx context.Context, runner SQLRunner) (map[int]bool, error) {
	rows, err := runner.QueryContext(ctx, "SELECT version FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("list schema migrations: %w", err)
	}
	defer rows.Close()
	result := make(map[int]bool)
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		result[version] = true
	}
	return result, rows.Err()
}

func prepareLegacySchema(ctx context.Context, runner SQLRunner) (bool, error) {
	rows, err := runner.QueryContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name <> 'schema_migrations'
		ORDER BY name
	`)
	if err != nil {
		return false, fmt.Errorf("inspect legacy schema: %w", err)
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return false, err
		}
		tables = append(tables, name)
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	if len(tables) == 0 {
		return false, nil
	}

	for _, table := range tables {
		objects, err := schemaObjects(ctx, runner, table)
		if err != nil {
			return false, err
		}
		for _, object := range objects {
			if _, err := runner.ExecContext(ctx, "DROP "+strings.ToUpper(object[1])+" "+quoteIdentifier(object[0])); err != nil {
				return false, fmt.Errorf("drop legacy %s %s: %w", object[1], object[0], err)
			}
		}
	}
	for _, table := range tables {
		legacyName := "legacy_" + strings.TrimPrefix(table, "legacy_")
		exists, err := tableExists(ctx, runner, legacyName)
		if err != nil {
			return false, err
		}
		if exists {
			return false, fmt.Errorf("legacy destination table %q already exists", legacyName)
		}
		if _, err := runner.ExecContext(ctx, "ALTER TABLE "+quoteIdentifier(table)+" RENAME TO "+quoteIdentifier(legacyName)); err != nil {
			return false, fmt.Errorf("preserve legacy table %s: %w", table, err)
		}
	}
	return true, nil
}

func schemaObjects(ctx context.Context, runner SQLRunner, table string) ([][2]string, error) {
	rows, err := runner.QueryContext(ctx, `
		SELECT name, type FROM sqlite_master
		WHERE tbl_name = ? AND type IN ('index', 'trigger') AND sql IS NOT NULL
	`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result [][2]string
	for rows.Next() {
		var name, typ string
		if err := rows.Scan(&name, &typ); err != nil {
			return nil, err
		}
		result = append(result, [2]string{name, typ})
	}
	return result, rows.Err()
}

func tableExists(ctx context.Context, runner SQLRunner, name string) (bool, error) {
	rows, err := runner.QueryContext(ctx, "SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?", name)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), nil
}

func tableColumns(ctx context.Context, runner SQLRunner, table string) (map[string]bool, error) {
	rows, err := runner.QueryContext(ctx, "PRAGMA table_info("+quoteIdentifier(table)+")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, typ string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		result[name] = true
	}
	return result, rows.Err()
}

func stableID(kind, key string) string {
	return uuid.NewSHA1(migrationNamespace, []byte(kind+":"+key)).String()
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func migrateLegacyData(ctx context.Context, runner SQLRunner) (MigrationReport, error) {
	report := MigrationReport{}
	rows, err := runner.QueryContext(ctx, `
		SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE 'legacy_%' ORDER BY name
	`)
	if err != nil {
		return report, err
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return report, err
		}
		report.LegacyTables = append(report.LegacyTables, name)
	}
	rows.Close()

	accountIDs, err := migrateLegacyAccounts(ctx, runner, &report)
	if err != nil {
		return report, err
	}
	defaultAccount, err := ensureLegacyDefaultAccount(ctx, runner, accountIDs, &report)
	if err != nil {
		return report, err
	}
	if exists, _ := tableExists(ctx, runner, "legacy_transactions"); exists {
		if err := migrateUnifiedTransactions(ctx, runner, accountIDs, defaultAccount, &report); err != nil {
			return report, err
		}
	}
	if err := migrateSeparateCashFlow(ctx, runner, defaultAccount, &report); err != nil {
		return report, err
	}
	if err := migrateLegacySnapshots(ctx, runner, accountIDs, &report); err != nil {
		return report, err
	}
	return report, nil
}

func migrateLegacyAccounts(ctx context.Context, runner SQLRunner, report *MigrationReport) (map[string]string, error) {
	result := make(map[string]string)
	exists, err := tableExists(ctx, runner, "legacy_accounts")
	if err != nil || !exists {
		return result, err
	}
	columns, err := tableColumns(ctx, runner, "legacy_accounts")
	if err != nil {
		return nil, err
	}
	createdColumn := "''"
	if columns["created_at"] {
		createdColumn = "created_at"
	}
	query := "SELECT name, type, " + createdColumn + " FROM legacy_accounts ORDER BY id"
	modern := columns["class"]
	if modern {
		query = "SELECT name, type, class, currency, active_from, active_to, note, created_at FROM legacy_accounts ORDER BY id"
	}
	rows, err := runner.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, typ, class, currency, activeFrom, activeTo, note, created string
		if modern {
			if err := rows.Scan(&name, &typ, &class, &currency, &activeFrom, &activeTo, &note, &created); err != nil {
				return nil, err
			}
		} else {
			if err := rows.Scan(&name, &typ, &created); err != nil {
				return nil, err
			}
			typ, class = legacyAccountType(typ)
			currency = "EUR"
		}
		name = strings.TrimSpace(name)
		id := stableID("account", strings.ToLower(name))
		initialDate := "1970-01-01"
		if len(activeFrom) == 7 {
			initialDate = activeFrom + "-01"
		}
		if created == "" {
			created = utcNow()
		}
		if _, err := runner.ExecContext(ctx, `
			INSERT INTO accounts (id, name, type, class, currency, initial_balance_cents, initial_date,
				active_from, active_to, note, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'EUR', 0, ?, ?, ?, ?, ?, ?)
		`, id, name, typ, class, initialDate, activeFrom, activeTo, note, created, created); err != nil {
			return nil, fmt.Errorf("migrate account %q: %w", name, err)
		}
		result[name] = id
		report.Accounts++
	}
	return result, rows.Err()
}

func ensureLegacyDefaultAccount(ctx context.Context, runner SQLRunner, accountIDs map[string]string, report *MigrationReport) (string, error) {
	if len(accountIDs) > 0 {
		names := make([]string, 0, len(accountIDs))
		for name := range accountIDs {
			names = append(names, name)
		}
		sort.Strings(names)
		return accountIDs[names[0]], nil
	}
	id := stableID("account", "conto legacy")
	now := utcNow()
	_, err := runner.ExecContext(ctx, `
		INSERT INTO accounts (id, name, type, class, currency, initial_balance_cents, initial_date, created_at, updated_at)
		VALUES (?, 'Conto legacy', 'Asset', 'Cash', 'EUR', 0, '1970-01-01', ?, ?)
	`, id, now, now)
	if err != nil {
		return "", err
	}
	report.Accounts++
	return id, nil
}

type legacyTransaction struct {
	id                                         int64
	date, kind, account, category, subcategory string
	payee, note, created                       string
	amount                                     int64
}

func migrateUnifiedTransactions(ctx context.Context, runner SQLRunner, accounts map[string]string, defaultAccount string, report *MigrationReport) error {
	rows, err := runner.QueryContext(ctx, `
		SELECT id, date, kind, account, amount_cents, category, subcategory, payee, note, created_at
		FROM legacy_transactions ORDER BY date, id
	`)
	if err != nil {
		return err
	}
	var transactions []legacyTransaction
	for rows.Next() {
		var item legacyTransaction
		if err := rows.Scan(&item.id, &item.date, &item.kind, &item.account, &item.amount, &item.category,
			&item.subcategory, &item.payee, &item.note, &item.created); err != nil {
			rows.Close()
			return err
		}
		transactions = append(transactions, item)
	}
	rows.Close()
	used := make(map[int64]bool)
	for i, item := range transactions {
		if used[item.id] {
			continue
		}
		accountID := accounts[item.account]
		if accountID == "" {
			accountID = defaultAccount
		}
		if item.kind != "Transfer" {
			if err := insertMigratedMovement(ctx, runner, "transaction:"+strconv.FormatInt(item.id, 10), legacyMovementKind(item.kind),
				item.date, item.amount, accountID, item.payee, item.note, item.category, item.subcategory, item.created); err != nil {
				return err
			}
			used[item.id] = true
			report.Movements++
			continue
		}

		var candidates []int
		for j := i + 1; j < len(transactions); j++ {
			candidate := transactions[j]
			if !used[candidate.id] && candidate.kind == "Transfer" && candidate.date == item.date &&
				candidate.amount == -item.amount && candidate.account != item.account &&
				strings.TrimSpace(candidate.note) == strings.TrimSpace(item.note) {
				candidates = append(candidates, j)
			}
		}
		if len(candidates) != 1 {
			report.AmbiguousTransfers = append(report.AmbiguousTransfers,
				fmt.Sprintf("legacy transaction %d has %d matches", item.id, len(candidates)))
			continue
		}
		other := transactions[candidates[0]]
		source, destination := item, other
		if source.amount > 0 {
			source, destination = destination, source
		}
		sourceID, destinationID := accounts[source.account], accounts[destination.account]
		if sourceID == "" {
			sourceID = defaultAccount
		}
		if destinationID == "" {
			destinationID = defaultAccount
		}
		if sourceID == destinationID {
			report.AmbiguousTransfers = append(report.AmbiguousTransfers,
				fmt.Sprintf("legacy transactions %d/%d resolve to the same account", source.id, destination.id))
			continue
		}
		if err := insertMigratedTransfer(ctx, runner, source, sourceID, destinationID); err != nil {
			return err
		}
		used[item.id], used[other.id] = true, true
		report.Movements++
		report.Transfers++
	}
	if len(report.AmbiguousTransfers) > 0 {
		return fmt.Errorf("ambiguous legacy transfers")
	}
	return nil
}

func migrateSeparateCashFlow(ctx context.Context, runner SQLRunner, accountID string, report *MigrationReport) error {
	for _, source := range []struct {
		table, kind, category, subcategory string
		sign                               int64
	}{
		{"legacy_expenses", "expense", "primary_category", "secondary_category", -1},
		{"legacy_incomes", "income", "category", "", 1},
	} {
		exists, err := tableExists(ctx, runner, source.table)
		if err != nil || !exists {
			if err != nil {
				return err
			}
			continue
		}
		subcategory := "''"
		if source.subcategory != "" {
			subcategory = source.subcategory
		}
		query := fmt.Sprintf("SELECT id, date, description, amount_cents, %s, %s, created_at FROM %s ORDER BY date, id",
			source.category, subcategory, source.table)
		rows, err := runner.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			var date, description, category, child, created string
			var amount int64
			if err := rows.Scan(&id, &date, &description, &amount, &category, &child, &created); err != nil {
				rows.Close()
				return err
			}
			if err := insertMigratedMovement(ctx, runner, source.table+":"+strconv.FormatInt(id, 10), source.kind,
				date, source.sign*amount, accountID, description, "", category, child, created); err != nil {
				rows.Close()
				return err
			}
			report.Movements++
		}
		rows.Close()
	}
	return nil
}

func migrateLegacySnapshots(ctx context.Context, runner SQLRunner, accounts map[string]string, report *MigrationReport) error {
	if exists, _ := tableExists(ctx, runner, "legacy_snapshot_balances"); exists {
		rows, err := runner.QueryContext(ctx, `
			WITH ranked AS (
				SELECT b.effective_month, s.account, s.balance_cents,
					row_number() OVER (PARTITION BY b.effective_month, s.account ORDER BY b.captured_at DESC, b.id DESC) AS rn
				FROM legacy_snapshot_balances s JOIN legacy_snapshot_batches b ON b.id = s.batch_id
			)
			SELECT effective_month, account, balance_cents FROM ranked WHERE rn = 1 ORDER BY effective_month, account
		`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var month, account string
			var balance int64
			if err := rows.Scan(&month, &account, &balance); err != nil {
				rows.Close()
				return err
			}
			if err := insertMigratedReconciliation(ctx, runner, month, accounts[account], balance); err != nil {
				rows.Close()
				return err
			}
			report.Reconciliations++
		}
		rows.Close()
	}
	if exists, _ := tableExists(ctx, runner, "legacy_account_balances"); exists {
		rows, err := runner.QueryContext(ctx, `
			SELECT a.name, b.year, b.month, b.amount_cents
			FROM legacy_account_balances b JOIN legacy_accounts a ON a.id = b.account_id
			ORDER BY b.year, b.month, a.name
		`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var account string
			var year, month int
			var balance int64
			if err := rows.Scan(&account, &year, &month, &balance); err != nil {
				rows.Close()
				return err
			}
			period := fmt.Sprintf("%04d-%02d", year, month)
			if err := insertMigratedReconciliation(ctx, runner, period, accounts[account], balance); err != nil {
				rows.Close()
				return err
			}
			report.Reconciliations++
		}
		rows.Close()
	}
	return nil
}

func insertMigratedMovement(ctx context.Context, runner SQLRunner, source, kind, date string, signedAmount int64, accountID, merchant, note, category, subcategory, created string) error {
	if signedAmount == 0 {
		return fmt.Errorf("%s has zero amount", source)
	}
	amount := signedAmount
	if amount < 0 {
		amount = -amount
	}
	if created == "" {
		created = utcNow()
	}
	movementID := stableID("movement", source)
	if _, err := runner.ExecContext(ctx, `
		INSERT INTO movements (id, kind, status, business_date, amount_cents, merchant, note, origin,
			legacy_source, created_at, updated_at)
		VALUES (?, ?, 'posted', ?, ?, ?, ?, 'migration', ?, ?, ?)
	`, movementID, kind, date, amount, merchant, note, source, created, created); err != nil {
		return fmt.Errorf("insert migrated movement %s: %w", source, err)
	}
	posting := signedAmount
	if kind == "expense" && posting > 0 {
		posting = -posting
	}
	if kind == "income" && posting < 0 {
		posting = -posting
	}
	if _, err := runner.ExecContext(ctx, `
		INSERT INTO postings (id, movement_id, account_id, amount_cents, created_at) VALUES (?, ?, ?, ?, ?)
	`, stableID("posting", source+":"+accountID), movementID, accountID, posting, created); err != nil {
		return err
	}
	if kind == "expense" || kind == "income" {
		categoryID, err := ensureMigratedCategory(ctx, runner, kind, category, subcategory)
		if err != nil {
			return err
		}
		if _, err := runner.ExecContext(ctx, `
			INSERT INTO movement_allocations (id, movement_id, category_id, amount_cents, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, stableID("allocation", source+":"+categoryID), movementID, categoryID, amount, created); err != nil {
			return err
		}
	}
	return nil
}

func insertMigratedTransfer(ctx context.Context, runner SQLRunner, source legacyTransaction, sourceID, destinationID string) error {
	amount := source.amount
	if amount < 0 {
		amount = -amount
	}
	created := source.created
	if created == "" {
		created = utcNow()
	}
	key := "transfer:" + strconv.FormatInt(source.id, 10)
	movementID := stableID("movement", key)
	if _, err := runner.ExecContext(ctx, `
		INSERT INTO movements (id, kind, status, business_date, amount_cents, merchant, note, origin,
			legacy_source, created_at, updated_at)
		VALUES (?, 'transfer', 'posted', ?, ?, '', ?, 'migration', ?, ?, ?)
	`, movementID, source.date, amount, source.note, key, created, created); err != nil {
		return err
	}
	for _, posting := range []struct {
		account string
		amount  int64
	}{{sourceID, -amount}, {destinationID, amount}} {
		if _, err := runner.ExecContext(ctx, `
			INSERT INTO postings (id, movement_id, account_id, amount_cents, created_at) VALUES (?, ?, ?, ?, ?)
		`, stableID("posting", key+":"+posting.account), movementID, posting.account, posting.amount, created); err != nil {
			return err
		}
	}
	return nil
}

func ensureMigratedCategory(ctx context.Context, runner SQLRunner, kind, parent, child string) (string, error) {
	if strings.TrimSpace(parent) == "" {
		parent = "Da categorizzare"
	}
	parent = strings.TrimSpace(parent)
	parentID := stableID("category", kind+":"+strings.ToLower(parent))
	now := utcNow()
	if _, err := runner.ExecContext(ctx, `
		INSERT OR IGNORE INTO categories (id, kind, name, icon, color, created_at, updated_at)
		VALUES (?, ?, ?, 'shapes', '#725B86', ?, ?)
	`, parentID, kind, parent, now, now); err != nil {
		return "", err
	}
	child = strings.TrimSpace(child)
	if child == "" {
		return parentID, nil
	}
	childID := stableID("category", kind+":"+strings.ToLower(parent)+":"+strings.ToLower(child))
	if _, err := runner.ExecContext(ctx, `
		INSERT OR IGNORE INTO categories (id, parent_id, kind, name, icon, color, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'shapes', '#725B86', ?, ?)
	`, childID, parentID, kind, child, now, now); err != nil {
		return "", err
	}
	return childID, nil
}

func insertMigratedReconciliation(ctx context.Context, runner SQLRunner, period, accountID string, balance int64) error {
	if accountID == "" {
		return fmt.Errorf("snapshot %s references an unknown account", period)
	}
	month, err := time.Parse("2006-01", period)
	if err != nil {
		return err
	}
	closed := month.AddDate(0, 1, -1).Format("2006-01-02")
	batchID := stableID("reconciliation-batch", period)
	now := utcNow()
	if _, err := runner.ExecContext(ctx, `
		INSERT OR IGNORE INTO reconciliation_batches (id, period, status, created_at, committed_at)
		VALUES (?, ?, 'committed', ?, ?)
	`, batchID, period, now, now); err != nil {
		return err
	}
	_, err = runner.ExecContext(ctx, `
		INSERT INTO account_reconciliations (id, batch_id, account_id, closed_through,
			expected_balance_cents, actual_balance_cents, difference_cents, created_at)
		VALUES (?, ?, ?, ?, ?, ?, 0, ?)
	`, stableID("reconciliation", period+":"+accountID), batchID, accountID, closed, balance, balance, now)
	return err
}

func legacyMovementKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "expense":
		return "expense"
	case "income":
		return "income"
	default:
		return "adjustment"
	}
}

func legacyAccountType(value string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "asset":
		return "Asset", "Other"
	case "liability":
		return "Liability", "Other"
	case "long_term", "investment":
		return "Asset", "Investment"
	case "tax":
		return "Liability", "Tax"
	case "credit":
		return "Liability", "Credit"
	default:
		return "Asset", "Cash"
	}
}
