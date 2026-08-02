// Package ledger owns Spese's canonical double-entry-style account postings.
package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"spese/internal/storage"
)

var (
	ErrNotFound           = errors.New("not found")
	ErrVersionConflict    = errors.New("version conflict")
	ErrValidation         = errors.New("validation error")
	ErrAllocationMismatch = errors.New("allocations must cover the movement amount")
	ErrTransferAllocation = errors.New("transfers cannot have category allocations")
)

var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

type AccountType string

const (
	AccountAsset     AccountType = "Asset"
	AccountLiability AccountType = "Liability"
)

type CategoryKind string

const (
	CategoryExpense CategoryKind = "expense"
	CategoryIncome  CategoryKind = "income"
)

type MovementKind string

const (
	MovementExpense    MovementKind = "expense"
	MovementIncome     MovementKind = "income"
	MovementRefund     MovementKind = "refund"
	MovementTransfer   MovementKind = "transfer"
	MovementAdjustment MovementKind = "adjustment"
)

type MovementStatus string

const (
	MovementDraft  MovementStatus = "draft"
	MovementPosted MovementStatus = "posted"
	MovementVoid   MovementStatus = "void"
)

type RecurringFrequency string

const (
	FrequencyDaily   RecurringFrequency = "daily"
	FrequencyWeekly  RecurringFrequency = "weekly"
	FrequencyMonthly RecurringFrequency = "monthly"
	FrequencyYearly  RecurringFrequency = "yearly"
)

const (
	RecurringAutoPost          = "auto_post"
	RecurringNeedsConfirmation = "needs_confirmation"
	RecurringFixed             = "fixed"
	RecurringVariable          = "variable"
	OccurrenceDraft            = "draft"
	OccurrencePosted           = "posted"
	OccurrenceSkipped          = "skipped"
)

type Account struct {
	ID                  string      `json:"id"`
	Name                string      `json:"name"`
	Type                AccountType `json:"type"`
	Class               string      `json:"class"`
	Currency            string      `json:"currency"`
	InitialBalanceCents int64       `json:"initialBalanceCents"`
	InitialDate         string      `json:"initialDate"`
	ActiveFrom          string      `json:"activeFrom,omitempty"`
	ActiveTo            string      `json:"activeTo,omitempty"`
	Note                string      `json:"note,omitempty"`
	ArchivedAt          string      `json:"archivedAt,omitempty"`
	CreatedAt           string      `json:"createdAt"`
	UpdatedAt           string      `json:"updatedAt"`
	Version             int         `json:"version"`
}

type AccountInput struct {
	Name                string      `json:"name"`
	Type                AccountType `json:"type"`
	Class               string      `json:"class"`
	InitialBalanceCents int64       `json:"initialBalanceCents"`
	InitialDate         string      `json:"initialDate"`
	ActiveFrom          string      `json:"activeFrom"`
	ActiveTo            string      `json:"activeTo"`
	Note                string      `json:"note"`
}

type Category struct {
	ID         string       `json:"id"`
	ParentID   string       `json:"parentId,omitempty"`
	Kind       CategoryKind `json:"kind"`
	Name       string       `json:"name"`
	Icon       string       `json:"icon"`
	Color      string       `json:"color"`
	SortOrder  int          `json:"sortOrder"`
	ArchivedAt string       `json:"archivedAt,omitempty"`
	CreatedAt  string       `json:"createdAt"`
	UpdatedAt  string       `json:"updatedAt"`
	Version    int          `json:"version"`
}

type CategoryInput struct {
	ParentID  string       `json:"parentId"`
	Kind      CategoryKind `json:"kind"`
	Name      string       `json:"name"`
	Icon      string       `json:"icon"`
	Color     string       `json:"color"`
	SortOrder int          `json:"sortOrder"`
}

type Posting struct {
	ID          string `json:"id"`
	AccountID   string `json:"accountId"`
	AmountCents int64  `json:"amountCents"`
}

type Allocation struct {
	ID          string `json:"id"`
	CategoryID  string `json:"categoryId"`
	AmountCents int64  `json:"amountCents"`
}

type AllocationInput struct {
	CategoryID  string `json:"categoryId"`
	AmountCents int64  `json:"amountCents"`
}

type Movement struct {
	ID                    string         `json:"id"`
	Kind                  MovementKind   `json:"kind"`
	Status                MovementStatus `json:"status"`
	Date                  string         `json:"date"`
	AmountCents           int64          `json:"amountCents"`
	Merchant              string         `json:"merchant"`
	Description           string         `json:"description"`
	Note                  string         `json:"note"`
	Origin                string         `json:"origin"`
	RecurringOccurrenceID string         `json:"recurringOccurrenceId,omitempty"`
	VoidedAt              string         `json:"voidedAt,omitempty"`
	VoidReason            string         `json:"voidReason,omitempty"`
	CreatedAt             string         `json:"createdAt"`
	UpdatedAt             string         `json:"updatedAt"`
	Version               int            `json:"version"`
	Postings              []Posting      `json:"postings"`
	Allocations           []Allocation   `json:"allocations"`
}

type MovementInput struct {
	Kind                 MovementKind      `json:"kind"`
	Status               MovementStatus    `json:"status"`
	Date                 string            `json:"date"`
	AccountID            string            `json:"accountId"`
	DestinationAccountID string            `json:"destinationAccountId"`
	AmountCents          int64             `json:"amountCents"`
	Merchant             string            `json:"merchant"`
	Description          string            `json:"description"`
	Note                 string            `json:"note"`
	Origin               string            `json:"origin"`
	Allocations          []AllocationInput `json:"allocations"`
}

type AccountBalance struct {
	AccountID    string `json:"accountId"`
	AsOf         string `json:"asOf"`
	BalanceCents int64  `json:"balanceCents"`
	AnchorDate   string `json:"anchorDate"`
	Reconciled   bool   `json:"reconciled"`
}

type ReconciliationInput struct {
	AccountID          string `json:"accountId"`
	ActualBalanceCents int64  `json:"actualBalanceCents"`
}

type ReconciliationAccount struct {
	AccountID            string `json:"accountId"`
	ExpectedBalanceCents int64  `json:"expectedBalanceCents"`
	ActualBalanceCents   int64  `json:"actualBalanceCents"`
	DifferenceCents      int64  `json:"differenceCents"`
}

type ReconciliationPreview struct {
	ID            string                  `json:"id"`
	Period        string                  `json:"period"`
	ClosedThrough string                  `json:"closedThrough"`
	Accounts      []ReconciliationAccount `json:"accounts"`
}

type RecurringRuleInput struct {
	Kind        MovementKind       `json:"kind"`
	Frequency   RecurringFrequency `json:"frequency"`
	Interval    int                `json:"interval"`
	StartDate   string             `json:"startDate"`
	EndDate     string             `json:"endDate"`
	DayOfMonth  int                `json:"dayOfMonth"`
	Timezone    string             `json:"timezone"`
	AmountCents int64              `json:"amountCents"`
	AmountMode  string             `json:"amountMode"`
	AccountID   string             `json:"accountId"`
	CategoryID  string             `json:"categoryId"`
	Merchant    string             `json:"merchant"`
	Note        string             `json:"note"`
	Mode        string             `json:"mode"`
}

type RecurringRule struct {
	ID string `json:"id"`
	RecurringRuleInput
	State     string `json:"state"`
	NextDue   string `json:"nextDue"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	Version   int    `json:"version"`
}

type RecurringOccurrence struct {
	ID              string `json:"id"`
	RuleID          string `json:"ruleId"`
	ScheduledFor    string `json:"scheduledFor"`
	Status          string `json:"status"`
	AmountCents     int64  `json:"amountCents"`
	AmountCertainty string `json:"amountCertainty"`
	MovementID      string `json:"movementId,omitempty"`
}

type Service struct {
	store       *storage.Store
	now         func() time.Time
	recurringMu sync.Mutex
}

func New(store *storage.Store) *Service {
	return &Service{store: store, now: time.Now}
}

func (s *Service) CreateAccount(ctx context.Context, input AccountInput) (Account, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 200 || len(input.Note) > 2000 ||
		(input.Type != AccountAsset && input.Type != AccountLiability) || !validOptionalMonth(input.ActiveFrom) || !validOptionalMonth(input.ActiveTo) ||
		(input.ActiveFrom != "" && input.ActiveTo != "" && input.ActiveFrom > input.ActiveTo) {
		return Account{}, ErrValidation
	}
	validClass := map[string]bool{"Cash": true, "Investment": true, "Property": true, "Tax": true, "Credit": true, "Other": true}
	if !validClass[input.Class] {
		return Account{}, ErrValidation
	}
	if input.InitialDate == "" {
		input.InitialDate = s.now().Format("2006-01-02")
	}
	if !validDate(input.InitialDate) {
		return Account{}, ErrValidation
	}
	now, id := utc(s.now()), uuid.NewString()
	_, err := s.store.DB().ExecContext(ctx, `
		INSERT INTO accounts (id, name, type, class, currency, initial_balance_cents, initial_date,
			active_from, active_to, note, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'EUR', ?, ?, ?, ?, ?, ?, ?)
	`, id, input.Name, input.Type, input.Class, input.InitialBalanceCents, input.InitialDate,
		input.ActiveFrom, input.ActiveTo, strings.TrimSpace(input.Note), now, now)
	if err != nil {
		return Account{}, fmt.Errorf("create account: %w", err)
	}
	return s.GetAccount(ctx, id)
}

func (s *Service) GetAccount(ctx context.Context, id string) (Account, error) {
	var account Account
	err := s.store.DB().QueryRowContext(ctx, `
		SELECT id, name, type, class, currency, initial_balance_cents, initial_date,
			active_from, active_to, note, archived_at, created_at, updated_at, version
		FROM accounts WHERE id = ?
	`, id).Scan(&account.ID, &account.Name, &account.Type, &account.Class, &account.Currency,
		&account.InitialBalanceCents, &account.InitialDate, &account.ActiveFrom, &account.ActiveTo,
		&account.Note, &account.ArchivedAt, &account.CreatedAt, &account.UpdatedAt, &account.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return account, err
}

func (s *Service) CreateCategory(ctx context.Context, input CategoryInput) (Category, error) {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" || len(input.Name) > 200 || (input.Kind != CategoryExpense && input.Kind != CategoryIncome) {
		return Category{}, ErrValidation
	}
	if input.Icon == "" {
		input.Icon = "shapes"
	}
	if input.Color == "" {
		input.Color = "#725B86"
	}
	if !hexColor.MatchString(input.Color) || len(input.Icon) > 100 {
		return Category{}, ErrValidation
	}
	now, id := utc(s.now()), uuid.NewString()
	var parent any
	if input.ParentID != "" {
		parent = input.ParentID
	}
	_, err := s.store.DB().ExecContext(ctx, `
		INSERT INTO categories (id, parent_id, kind, name, icon, color, sort_order, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, parent, input.Kind, input.Name, input.Icon, input.Color, input.SortOrder, now, now)
	if err != nil {
		return Category{}, fmt.Errorf("create category: %w", err)
	}
	return s.GetCategory(ctx, id)
}

func (s *Service) GetCategory(ctx context.Context, id string) (Category, error) {
	var category Category
	var parent sql.NullString
	err := s.store.DB().QueryRowContext(ctx, `
		SELECT id, parent_id, kind, name, icon, color, sort_order, archived_at, created_at, updated_at, version
		FROM categories WHERE id = ?
	`, id).Scan(&category.ID, &parent, &category.Kind, &category.Name, &category.Icon,
		&category.Color, &category.SortOrder, &category.ArchivedAt, &category.CreatedAt, &category.UpdatedAt, &category.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return Category{}, ErrNotFound
	}
	category.ParentID = parent.String
	return category, err
}

func validDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func validOptionalMonth(value string) bool {
	if value == "" {
		return true
	}
	parsed, err := time.Parse("2006-01", value)
	return err == nil && parsed.Format("2006-01") == value
}

func utc(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func (s *Service) CreateMovement(ctx context.Context, input MovementInput) (Movement, error) {
	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return Movement{}, err
	}
	defer tx.Rollback()
	movement, err := s.createMovementTx(ctx, tx, input, "")
	if err != nil {
		return Movement{}, err
	}
	if err := s.store.EnqueueSheetSync(tx, "movements"); err != nil {
		return Movement{}, err
	}
	if err := tx.Commit(); err != nil {
		return Movement{}, err
	}
	return s.GetMovement(ctx, movement.ID)
}

func (s *Service) CreateMovementBatch(ctx context.Context, inputs []MovementInput) ([]Movement, error) {
	if len(inputs) == 0 || len(inputs) > 20 {
		return nil, ErrValidation
	}
	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result := make([]Movement, 0, len(inputs))
	for _, input := range inputs {
		movement, err := s.createMovementTx(ctx, tx, input, "")
		if err != nil {
			return nil, err
		}
		result = append(result, movement)
	}
	if err := s.store.EnqueueSheetSync(tx, "movements"); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for i := range result {
		movement, err := s.GetMovement(ctx, result[i].ID)
		if err != nil {
			return nil, err
		}
		result[i] = movement
	}
	return result, nil
}

func (s *Service) createMovementTx(ctx context.Context, tx *sql.Tx, input MovementInput, occurrenceID string) (Movement, error) {
	if input.Status == "" {
		input.Status = MovementPosted
	}
	if input.Origin == "" {
		input.Origin = "manual"
	}
	if !validDate(input.Date) || input.AmountCents <= 0 || len(input.Merchant) > 300 || len(input.Description) > 500 || len(input.Note) > 2000 ||
		(input.Status != MovementDraft && input.Status != MovementPosted) {
		return Movement{}, ErrValidation
	}
	if input.Kind != MovementExpense && input.Kind != MovementIncome && input.Kind != MovementRefund &&
		input.Kind != MovementTransfer && input.Kind != MovementAdjustment {
		return Movement{}, ErrValidation
	}
	if input.Origin != "manual" && input.Origin != "recurring" && input.Origin != "dictation" && input.Origin != "migration" {
		return Movement{}, ErrValidation
	}
	if err := accountExists(ctx, tx, input.AccountID); err != nil {
		return Movement{}, err
	}
	if input.Kind == MovementTransfer {
		if len(input.Allocations) > 0 {
			return Movement{}, ErrTransferAllocation
		}
		if input.DestinationAccountID == "" || input.DestinationAccountID == input.AccountID {
			return Movement{}, ErrValidation
		}
		if err := accountExists(ctx, tx, input.DestinationAccountID); err != nil {
			return Movement{}, err
		}
	} else if input.Kind != MovementAdjustment {
		var total int64
		for _, allocation := range input.Allocations {
			if allocation.AmountCents <= 0 {
				return Movement{}, ErrAllocationMismatch
			}
			total += allocation.AmountCents
			wantKind := CategoryExpense
			if input.Kind == MovementIncome {
				wantKind = CategoryIncome
			}
			if err := categoryMatches(ctx, tx, allocation.CategoryID, wantKind); err != nil {
				return Movement{}, err
			}
		}
		if total != input.AmountCents {
			return Movement{}, ErrAllocationMismatch
		}
	}

	id, now := uuid.NewString(), utc(s.now())
	_, err := tx.ExecContext(ctx, `
		INSERT INTO movements (id, kind, status, business_date, amount_cents, merchant, description, note, origin,
			recurring_occurrence_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, input.Kind, input.Status, input.Date, input.AmountCents, strings.TrimSpace(input.Merchant),
		strings.TrimSpace(input.Description), strings.TrimSpace(input.Note), input.Origin, nullable(occurrenceID), now, now)
	if err != nil {
		return Movement{}, fmt.Errorf("insert movement: %w", err)
	}
	postings := []struct {
		accountID string
		amount    int64
	}{{input.AccountID, input.AmountCents}}
	switch input.Kind {
	case MovementExpense:
		postings[0].amount = -input.AmountCents
	case MovementIncome, MovementRefund:
		postings[0].amount = input.AmountCents
	case MovementTransfer:
		postings[0].amount = -input.AmountCents
		postings = append(postings, struct {
			accountID string
			amount    int64
		}{input.DestinationAccountID, input.AmountCents})
	case MovementAdjustment:
		postings[0].amount = input.AmountCents
	}
	for _, posting := range postings {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO postings (id, movement_id, account_id, amount_cents, created_at) VALUES (?, ?, ?, ?, ?)
		`, uuid.NewString(), id, posting.accountID, posting.amount, now); err != nil {
			return Movement{}, fmt.Errorf("insert posting: %w", err)
		}
	}
	for _, allocation := range input.Allocations {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO movement_allocations (id, movement_id, category_id, amount_cents, created_at)
			VALUES (?, ?, ?, ?, ?)
		`, uuid.NewString(), id, allocation.CategoryID, allocation.AmountCents, now); err != nil {
			return Movement{}, fmt.Errorf("insert allocation: %w", err)
		}
	}
	return Movement{ID: id}, nil
}

func (s *Service) GetMovement(ctx context.Context, id string) (Movement, error) {
	return getMovement(ctx, s.store.DB(), id)
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getMovement(ctx context.Context, db queryer, id string) (Movement, error) {
	var movement Movement
	var occurrence sql.NullString
	err := db.QueryRowContext(ctx, `
		SELECT id, kind, status, business_date, amount_cents, merchant, description, note, origin,
			recurring_occurrence_id, voided_at, void_reason, created_at, updated_at, version
		FROM movements WHERE id = ?
	`, id).Scan(&movement.ID, &movement.Kind, &movement.Status, &movement.Date, &movement.AmountCents,
		&movement.Merchant, &movement.Description, &movement.Note, &movement.Origin, &occurrence, &movement.VoidedAt,
		&movement.VoidReason, &movement.CreatedAt, &movement.UpdatedAt, &movement.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return Movement{}, ErrNotFound
	}
	if err != nil {
		return Movement{}, err
	}
	movement.RecurringOccurrenceID = occurrence.String
	rows, err := db.QueryContext(ctx, `SELECT id, account_id, amount_cents FROM postings WHERE movement_id = ? ORDER BY id`, id)
	if err != nil {
		return Movement{}, err
	}
	for rows.Next() {
		var posting Posting
		if err := rows.Scan(&posting.ID, &posting.AccountID, &posting.AmountCents); err != nil {
			rows.Close()
			return Movement{}, err
		}
		movement.Postings = append(movement.Postings, posting)
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, `SELECT id, category_id, amount_cents FROM movement_allocations WHERE movement_id = ? ORDER BY id`, id)
	if err != nil {
		return Movement{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var allocation Allocation
		if err := rows.Scan(&allocation.ID, &allocation.CategoryID, &allocation.AmountCents); err != nil {
			return Movement{}, err
		}
		movement.Allocations = append(movement.Allocations, allocation)
	}
	return movement, rows.Err()
}

func (s *Service) UpdateMovement(ctx context.Context, id string, expectedVersion int, input MovementInput) (Movement, error) {
	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return Movement{}, err
	}
	defer tx.Rollback()
	current, err := getMovement(ctx, tx, id)
	if err != nil {
		return Movement{}, err
	}
	if current.Version != expectedVersion {
		return Movement{}, ErrVersionConflict
	}
	if input.Status == "" {
		input.Status = current.Status
	}
	if input.Origin == "" {
		input.Origin = current.Origin
	}
	// Validate using the same insertion path inside a savepoint, then retain the
	// original stable movement ID and audit history.
	if _, err := tx.ExecContext(ctx, "SAVEPOINT validate_movement"); err != nil {
		return Movement{}, err
	}
	validated, err := s.createMovementTx(ctx, tx, input, current.RecurringOccurrenceID)
	if err != nil {
		return Movement{}, err
	}
	if _, err := tx.ExecContext(ctx, "ROLLBACK TO validate_movement"); err != nil {
		return Movement{}, err
	}
	if _, err := tx.ExecContext(ctx, "RELEASE validate_movement"); err != nil {
		return Movement{}, err
	}
	_ = validated
	if _, err := tx.ExecContext(ctx, "DELETE FROM postings WHERE movement_id = ?", id); err != nil {
		return Movement{}, err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM movement_allocations WHERE movement_id = ?", id); err != nil {
		return Movement{}, err
	}
	now := utc(s.now())
	result, err := tx.ExecContext(ctx, `
		UPDATE movements SET kind = ?, status = ?, business_date = ?, amount_cents = ?, merchant = ?, description = ?, note = ?,
			origin = ?, updated_at = ?, version = version + 1
		WHERE id = ? AND version = ?
	`, input.Kind, input.Status, input.Date, input.AmountCents, strings.TrimSpace(input.Merchant),
		strings.TrimSpace(input.Description), strings.TrimSpace(input.Note), input.Origin, now, id, expectedVersion)
	if err != nil {
		return Movement{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return Movement{}, ErrVersionConflict
	}
	postings := []struct {
		account string
		amount  int64
	}{{input.AccountID, input.AmountCents}}
	switch input.Kind {
	case MovementExpense:
		postings[0].amount = -input.AmountCents
	case MovementTransfer:
		postings[0].amount = -input.AmountCents
		postings = append(postings, struct {
			account string
			amount  int64
		}{input.DestinationAccountID, input.AmountCents})
	}
	for _, posting := range postings {
		if _, err := tx.ExecContext(ctx, `INSERT INTO postings (id, movement_id, account_id, amount_cents, created_at) VALUES (?, ?, ?, ?, ?)`,
			uuid.NewString(), id, posting.account, posting.amount, now); err != nil {
			return Movement{}, err
		}
	}
	for _, allocation := range input.Allocations {
		if _, err := tx.ExecContext(ctx, `INSERT INTO movement_allocations (id, movement_id, category_id, amount_cents, created_at) VALUES (?, ?, ?, ?, ?)`,
			uuid.NewString(), id, allocation.CategoryID, allocation.AmountCents, now); err != nil {
			return Movement{}, err
		}
	}
	if err := s.store.EnqueueSheetSync(tx, "movements"); err != nil {
		return Movement{}, err
	}
	if err := tx.Commit(); err != nil {
		return Movement{}, err
	}
	return s.GetMovement(ctx, id)
}

func (s *Service) VoidMovement(ctx context.Context, id string, expectedVersion int, reason string) (Movement, error) {
	now := utc(s.now())
	result, err := s.store.DB().ExecContext(ctx, `
		UPDATE movements SET status = 'void', voided_at = ?, void_reason = ?, updated_at = ?, version = version + 1
		WHERE id = ? AND version = ? AND status <> 'void'
	`, now, strings.TrimSpace(reason), now, id, expectedVersion)
	if err != nil {
		return Movement{}, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		if _, err := s.GetMovement(ctx, id); errors.Is(err, ErrNotFound) {
			return Movement{}, ErrNotFound
		}
		return Movement{}, ErrVersionConflict
	}
	return s.GetMovement(ctx, id)
}

func accountExists(ctx context.Context, tx *sql.Tx, id string) error {
	var exists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM accounts WHERE id = ? AND archived_at = ''", id).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return ErrValidation
	} else {
		return err
	}
}

func categoryMatches(ctx context.Context, tx *sql.Tx, id string, kind CategoryKind) error {
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM categories WHERE id = ? AND kind = ? AND archived_at = ''`, id, kind).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return ErrValidation
	} else {
		return err
	}
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *Service) Balance(ctx context.Context, accountID, asOf string) (AccountBalance, error) {
	if !validDate(asOf) {
		return AccountBalance{}, ErrValidation
	}
	var initial int64
	var initialDate string
	if err := s.store.DB().QueryRowContext(ctx, `
		SELECT initial_balance_cents, initial_date FROM accounts WHERE id = ?
	`, accountID).Scan(&initial, &initialDate); errors.Is(err, sql.ErrNoRows) {
		return AccountBalance{}, ErrNotFound
	} else if err != nil {
		return AccountBalance{}, err
	}
	result := AccountBalance{AccountID: accountID, AsOf: asOf, BalanceCents: initial, AnchorDate: initialDate}
	err := s.store.DB().QueryRowContext(ctx, `
		SELECT r.actual_balance_cents, r.closed_through
		FROM account_reconciliations r
		JOIN reconciliation_batches b ON b.id = r.batch_id AND b.status = 'committed'
		WHERE r.account_id = ? AND r.closed_through <= ?
		ORDER BY r.closed_through DESC, r.created_at DESC LIMIT 1
	`, accountID, asOf).Scan(&result.BalanceCents, &result.AnchorDate)
	if err == nil {
		result.Reconciled = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return AccountBalance{}, err
	}
	var delta sql.NullInt64
	if err := s.store.DB().QueryRowContext(ctx, `
		SELECT sum(p.amount_cents)
		FROM postings p JOIN movements m ON m.id = p.movement_id
		WHERE p.account_id = ? AND m.status = 'posted'
			AND m.business_date > ? AND m.business_date <= ?
	`, accountID, result.AnchorDate, asOf).Scan(&delta); err != nil {
		return AccountBalance{}, err
	}
	result.BalanceCents += delta.Int64
	return result, nil
}

func (s *Service) PreviewReconciliation(ctx context.Context, period string, inputs []ReconciliationInput) (ReconciliationPreview, error) {
	month, err := time.Parse("2006-01", period)
	if err != nil || month.Format("2006-01") != period || len(inputs) == 0 {
		return ReconciliationPreview{}, ErrValidation
	}
	closed := month.AddDate(0, 1, -1).Format("2006-01-02")
	preview := ReconciliationPreview{ID: uuid.NewString(), Period: period, ClosedThrough: closed}
	seen := make(map[string]bool)
	for _, input := range inputs {
		if seen[input.AccountID] {
			return ReconciliationPreview{}, ErrValidation
		}
		seen[input.AccountID] = true
		balance, err := s.Balance(ctx, input.AccountID, closed)
		if err != nil {
			return ReconciliationPreview{}, err
		}
		preview.Accounts = append(preview.Accounts, ReconciliationAccount{
			AccountID: input.AccountID, ExpectedBalanceCents: balance.BalanceCents,
			ActualBalanceCents: input.ActualBalanceCents, DifferenceCents: input.ActualBalanceCents - balance.BalanceCents,
		})
	}
	return preview, nil
}

func (s *Service) CommitReconciliation(ctx context.Context, preview ReconciliationPreview) (ReconciliationPreview, error) {
	if preview.ID == "" || preview.Period == "" || !validDate(preview.ClosedThrough) || len(preview.Accounts) == 0 {
		return ReconciliationPreview{}, ErrValidation
	}
	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return ReconciliationPreview{}, err
	}
	defer tx.Rollback()
	now := utc(s.now())
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO reconciliation_batches (id, period, status, created_at, committed_at)
		VALUES (?, ?, 'committed', ?, ?)
	`, preview.ID, preview.Period, now, now); err != nil {
		return ReconciliationPreview{}, err
	}
	for _, account := range preview.Accounts {
		if err := accountExists(ctx, tx, account.AccountID); err != nil {
			return ReconciliationPreview{}, err
		}
		if account.DifferenceCents != account.ActualBalanceCents-account.ExpectedBalanceCents {
			return ReconciliationPreview{}, ErrValidation
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO account_reconciliations (id, batch_id, account_id, closed_through,
				expected_balance_cents, actual_balance_cents, difference_cents, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, uuid.NewString(), preview.ID, account.AccountID, preview.ClosedThrough, account.ExpectedBalanceCents,
			account.ActualBalanceCents, account.DifferenceCents, now); err != nil {
			return ReconciliationPreview{}, err
		}
	}
	if err := s.store.EnqueueSheetSync(tx, "reconciliations"); err != nil {
		return ReconciliationPreview{}, err
	}
	if err := tx.Commit(); err != nil {
		return ReconciliationPreview{}, err
	}
	return preview, nil
}

func (s *Service) CreateRecurringRule(ctx context.Context, input RecurringRuleInput) (RecurringRule, error) {
	if input.Kind != MovementExpense && input.Kind != MovementIncome || input.AmountCents <= 0 || !validDate(input.StartDate) {
		return RecurringRule{}, ErrValidation
	}
	if input.Interval == 0 {
		input.Interval = 1
	}
	if input.Interval < 1 || (input.Frequency != FrequencyDaily && input.Frequency != FrequencyWeekly &&
		input.Frequency != FrequencyMonthly && input.Frequency != FrequencyYearly) {
		return RecurringRule{}, ErrValidation
	}
	if input.Timezone == "" {
		input.Timezone = "Europe/Rome"
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return RecurringRule{}, ErrValidation
	}
	if input.AmountMode == "" {
		input.AmountMode = RecurringFixed
	}
	if input.Mode == "" {
		if input.AmountMode == RecurringVariable {
			input.Mode = RecurringNeedsConfirmation
		} else {
			input.Mode = RecurringAutoPost
		}
	}
	if input.AmountMode != RecurringFixed && input.AmountMode != RecurringVariable ||
		input.Mode != RecurringAutoPost && input.Mode != RecurringNeedsConfirmation {
		return RecurringRule{}, ErrValidation
	}
	if input.AmountMode == RecurringVariable && input.Mode == RecurringAutoPost {
		return RecurringRule{}, ErrValidation
	}
	if input.EndDate != "" && (!validDate(input.EndDate) || input.EndDate < input.StartDate) {
		return RecurringRule{}, ErrValidation
	}
	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return RecurringRule{}, err
	}
	defer tx.Rollback()
	if err := accountExists(ctx, tx, input.AccountID); err != nil {
		return RecurringRule{}, err
	}
	kind := CategoryExpense
	if input.Kind == MovementIncome {
		kind = CategoryIncome
	}
	if err := categoryMatches(ctx, tx, input.CategoryID, kind); err != nil {
		return RecurringRule{}, err
	}
	id, now := uuid.NewString(), utc(s.now())
	var endDate any
	if input.EndDate != "" {
		endDate = input.EndDate
	}
	var day any
	if input.DayOfMonth > 0 {
		day = input.DayOfMonth
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO recurring_rules (id, kind, frequency, interval_count, start_date, end_date, day_of_month,
			timezone, amount_cents, amount_mode, account_id, category_id, merchant, note, state, mode,
			next_due, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?, ?, ?)
	`, id, input.Kind, input.Frequency, input.Interval, input.StartDate, endDate, day, input.Timezone,
		input.AmountCents, input.AmountMode, input.AccountID, input.CategoryID, strings.TrimSpace(input.Merchant),
		strings.TrimSpace(input.Note), input.Mode, input.StartDate, now, now)
	if err != nil {
		return RecurringRule{}, err
	}
	if err := tx.Commit(); err != nil {
		return RecurringRule{}, err
	}
	return s.GetRecurringRule(ctx, id)
}

func (s *Service) GetRecurringRule(ctx context.Context, id string) (RecurringRule, error) {
	var rule RecurringRule
	var endDate sql.NullString
	var day sql.NullInt64
	err := s.store.DB().QueryRowContext(ctx, `
		SELECT id, kind, frequency, interval_count, start_date, end_date, day_of_month, timezone,
			amount_cents, amount_mode, account_id, category_id, merchant, note, state, mode, next_due,
			created_at, updated_at, version
		FROM recurring_rules WHERE id = ?
	`, id).Scan(&rule.ID, &rule.Kind, &rule.Frequency, &rule.Interval, &rule.StartDate, &endDate, &day,
		&rule.Timezone, &rule.AmountCents, &rule.AmountMode, &rule.AccountID, &rule.CategoryID,
		&rule.Merchant, &rule.Note, &rule.State, &rule.Mode, &rule.NextDue, &rule.CreatedAt, &rule.UpdatedAt, &rule.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return RecurringRule{}, ErrNotFound
	}
	rule.EndDate, rule.DayOfMonth = endDate.String, int(day.Int64)
	return rule, err
}

func (s *Service) ProcessRecurring(ctx context.Context, through time.Time) (int, error) {
	s.recurringMu.Lock()
	defer s.recurringMu.Unlock()
	throughDate := through.Format("2006-01-02")
	rows, err := s.store.DB().QueryContext(ctx, `SELECT id FROM recurring_rules WHERE state = 'active' AND next_due <= ? ORDER BY next_due, id`, throughDate)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	created := 0
	for _, id := range ids {
		for {
			rule, err := s.GetRecurringRule(ctx, id)
			if err != nil {
				return created, err
			}
			if rule.State != "active" || rule.NextDue > throughDate || rule.EndDate != "" && rule.NextDue > rule.EndDate {
				break
			}
			made, err := s.processOccurrence(ctx, rule)
			if err != nil {
				return created, err
			}
			if made {
				created++
			}
		}
	}
	return created, nil
}

func (s *Service) processOccurrence(ctx context.Context, rule RecurringRule) (bool, error) {
	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var currentDue, state string
	var version int
	if err := tx.QueryRowContext(ctx, `SELECT next_due, state, version FROM recurring_rules WHERE id = ?`, rule.ID).Scan(&currentDue, &state, &version); err != nil {
		return false, err
	}
	if currentDue != rule.NextDue || state != "active" {
		return false, nil
	}
	occurrenceID, now := uuid.NewString(), utc(s.now())
	status, certainty := OccurrenceDraft, "estimated"
	movementStatus := MovementDraft
	if rule.Mode == RecurringAutoPost && rule.AmountMode == RecurringFixed {
		status, certainty, movementStatus = OccurrencePosted, "certain", MovementPosted
	}
	result, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO recurring_occurrences (id, rule_id, scheduled_for, status, amount_cents,
			amount_certainty, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, occurrenceID, rule.ID, rule.NextDue, status, rule.AmountCents, certainty, now, now)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	next, err := nextOccurrence(rule, rule.NextDue)
	if err != nil {
		return false, err
	}
	if affected == 0 {
		_, err = tx.ExecContext(ctx, `UPDATE recurring_rules SET next_due = ?, updated_at = ?, version = version + 1 WHERE id = ? AND version = ?`, next, now, rule.ID, version)
		if err != nil {
			return false, err
		}
		return false, tx.Commit()
	}
	movement, err := s.createMovementTx(ctx, tx, MovementInput{
		Kind: rule.Kind, Status: movementStatus, Date: rule.NextDue, AccountID: rule.AccountID,
		AmountCents: rule.AmountCents, Merchant: rule.Merchant, Note: rule.Note, Origin: "recurring",
		Allocations: []AllocationInput{{CategoryID: rule.CategoryID, AmountCents: rule.AmountCents}},
	}, occurrenceID)
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE recurring_occurrences SET movement_id = ?, updated_at = ? WHERE id = ?`, movement.ID, now, occurrenceID); err != nil {
		return false, err
	}
	result, err = tx.ExecContext(ctx, `
		UPDATE recurring_rules SET next_due = ?, updated_at = ?, version = version + 1 WHERE id = ? AND version = ?
	`, next, now, rule.ID, version)
	if err != nil {
		return false, err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return false, ErrVersionConflict
	}
	if err := s.store.EnqueueSheetSync(tx, "recurring"); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func nextOccurrence(rule RecurringRule, current string) (string, error) {
	date, err := time.Parse("2006-01-02", current)
	if err != nil {
		return "", err
	}
	interval := rule.Interval
	if interval < 1 {
		interval = 1
	}
	switch rule.Frequency {
	case FrequencyDaily:
		date = date.AddDate(0, 0, interval)
	case FrequencyWeekly:
		date = date.AddDate(0, 0, 7*interval)
	case FrequencyMonthly:
		target := date.AddDate(0, interval, -date.Day()+1)
		day := rule.DayOfMonth
		if day == 0 {
			day = date.Day()
		}
		last := target.AddDate(0, 1, -1).Day()
		if day > last {
			day = last
		}
		date = time.Date(target.Year(), target.Month(), day, 0, 0, 0, 0, time.UTC)
	case FrequencyYearly:
		targetYear := date.Year() + interval
		day := date.Day()
		first := time.Date(targetYear, date.Month(), 1, 0, 0, 0, 0, time.UTC)
		last := first.AddDate(0, 1, -1).Day()
		if day > last {
			day = last
		}
		date = time.Date(targetYear, date.Month(), day, 0, 0, 0, 0, time.UTC)
	default:
		return "", ErrValidation
	}
	return date.Format("2006-01-02"), nil
}

func (s *Service) ListOccurrences(ctx context.Context, ruleID string) ([]RecurringOccurrence, error) {
	rows, err := s.store.DB().QueryContext(ctx, `
		SELECT id, rule_id, scheduled_for, status, amount_cents, amount_certainty, movement_id
		FROM recurring_occurrences WHERE rule_id = ? ORDER BY scheduled_for
	`, ruleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]RecurringOccurrence, 0)
	for rows.Next() {
		var occurrence RecurringOccurrence
		var movement sql.NullString
		if err := rows.Scan(&occurrence.ID, &occurrence.RuleID, &occurrence.ScheduledFor, &occurrence.Status,
			&occurrence.AmountCents, &occurrence.AmountCertainty, &movement); err != nil {
			return nil, err
		}
		occurrence.MovementID = movement.String
		result = append(result, occurrence)
	}
	return result, rows.Err()
}

func (s *Service) ActOnOccurrence(ctx context.Context, id, action string) (RecurringOccurrence, error) {
	if action != "confirm" && action != "skip" && action != "post" {
		return RecurringOccurrence{}, ErrValidation
	}
	tx, err := s.store.DB().BeginTx(ctx, nil)
	if err != nil {
		return RecurringOccurrence{}, err
	}
	defer tx.Rollback()
	var occurrence RecurringOccurrence
	var movementID sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT id, rule_id, scheduled_for, status, amount_cents, amount_certainty, movement_id
		FROM recurring_occurrences WHERE id = ?
	`, id).Scan(&occurrence.ID, &occurrence.RuleID, &occurrence.ScheduledFor, &occurrence.Status,
		&occurrence.AmountCents, &occurrence.AmountCertainty, &movementID); errors.Is(err, sql.ErrNoRows) {
		return RecurringOccurrence{}, ErrNotFound
	} else if err != nil {
		return RecurringOccurrence{}, err
	}
	if occurrence.Status == OccurrenceSkipped || occurrence.Status == OccurrencePosted {
		return RecurringOccurrence{}, ErrVersionConflict
	}
	now := utc(s.now())
	switch action {
	case "skip":
		if movementID.Valid {
			if _, err := tx.ExecContext(ctx, `
				UPDATE movements SET status = 'void', voided_at = ?, void_reason = 'Ricorrenza saltata',
					updated_at = ?, version = version + 1 WHERE id = ? AND status = 'draft'
			`, now, now, movementID.String); err != nil {
				return RecurringOccurrence{}, err
			}
		}
		occurrence.Status = OccurrenceSkipped
	case "confirm", "post":
		if !movementID.Valid {
			return RecurringOccurrence{}, ErrValidation
		}
		date := occurrence.ScheduledFor
		if action == "post" {
			date = s.now().Format("2006-01-02")
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE movements SET status = 'posted', business_date = ?, updated_at = ?, version = version + 1
			WHERE id = ? AND status = 'draft'
		`, date, now, movementID.String); err != nil {
			return RecurringOccurrence{}, err
		}
		occurrence.Status = OccurrencePosted
	}
	if _, err := tx.ExecContext(ctx, `UPDATE recurring_occurrences SET status = ?, updated_at = ? WHERE id = ?`, occurrence.Status, now, id); err != nil {
		return RecurringOccurrence{}, err
	}
	if err := s.store.EnqueueSheetSync(tx, "recurring"); err != nil {
		return RecurringOccurrence{}, err
	}
	if err := tx.Commit(); err != nil {
		return RecurringOccurrence{}, err
	}
	occurrence.MovementID = movementID.String
	return occurrence, nil
}
