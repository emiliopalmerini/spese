package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"spese/internal/core"
)

// ErrTaxRateNotFound indicates no tax rate is configured for the given code at
// the given date.
var ErrTaxRateNotFound = errors.New("tax rate not found")

// TaxRateRow models a configured tax rate effective on a date window.
type TaxRateRow struct {
	Code         string
	Label        string
	RateBasisPts int64
	ValidFrom    time.Time
	ValidTo      *time.Time
}

// TaxAccrualRow models a stored tax accrual for an income.
type TaxAccrualRow struct {
	ID           int64
	IncomeID     int64
	TaxCode      string
	RateBasisPts int64
	AmountCents  int64
	Date         time.Time
}

// ResolveTaxRate returns the active rate for a code at the given date.
// Returns ErrTaxRateNotFound when no row is in effect.
func (r *SQLiteRepository) ResolveTaxRate(ctx context.Context, code string, at time.Time) (TaxRateRow, error) {
	row, err := r.readQueries.ResolveTaxRate(ctx, ResolveTaxRateParams{
		Code:      code,
		ValidFrom: at,
		ValidTo:   sql.NullTime{Time: at, Valid: true},
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return TaxRateRow{}, ErrTaxRateNotFound
		}
		return TaxRateRow{}, fmt.Errorf("resolve tax rate: %w", err)
	}
	out := TaxRateRow{
		Code:         row.Code,
		Label:        row.Label,
		RateBasisPts: row.RateBasisPts,
		ValidFrom:    row.ValidFrom,
	}
	if row.ValidTo.Valid {
		t := row.ValidTo.Time
		out.ValidTo = &t
	}
	return out, nil
}

// ListTaxRateCodes returns the distinct configured tax codes.
func (r *SQLiteRepository) ListTaxRateCodes(ctx context.Context) ([]string, error) {
	codes, err := r.readQueries.ListTaxRateCodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tax codes: %w", err)
	}
	return codes, nil
}

// IsFreelanceIncomeCategory returns true when the category is configured as
// freelance for tax-accrual purposes.
func (r *SQLiteRepository) IsFreelanceIncomeCategory(ctx context.Context, category string) (bool, error) {
	hits, err := r.readQueries.IsFreelanceIncomeCategory(ctx, category)
	if err != nil {
		return false, fmt.Errorf("check freelance category: %w", err)
	}
	return hits > 0, nil
}

// ListFreelanceIncomeCategories returns the active freelance category names.
func (r *SQLiteRepository) ListFreelanceIncomeCategories(ctx context.Context) ([]string, error) {
	cats, err := r.readQueries.ListFreelanceIncomeCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("list freelance categories: %w", err)
	}
	return cats, nil
}

// InsertTaxAccrual writes a tax accrual idempotently. A duplicate
// (income_id, tax_code) is silently ignored.
func (r *SQLiteRepository) InsertTaxAccrual(ctx context.Context, incomeID int64, code string, rateBasisPts int64, amount core.Money, date time.Time) error {
	dateStr := date.Format("2006-01-02")
	if err := r.queries.InsertTaxAccrual(ctx, InsertTaxAccrualParams{
		IncomeID:     incomeID,
		TaxCode:      code,
		RateBasisPts: rateBasisPts,
		AmountCents:  amount.Cents,
		Date:         dateStr,
	}); err != nil {
		return fmt.Errorf("insert tax accrual: %w", err)
	}
	return nil
}

// ListTaxAccrualsByIncome returns the accrual rows recorded for an income.
func (r *SQLiteRepository) ListTaxAccrualsByIncome(ctx context.Context, incomeID int64) ([]TaxAccrualRow, error) {
	rows, err := r.readQueries.ListTaxAccrualsByIncome(ctx, incomeID)
	if err != nil {
		return nil, fmt.Errorf("list accruals by income: %w", err)
	}
	out := make([]TaxAccrualRow, len(rows))
	for i, row := range rows {
		out[i] = TaxAccrualRow{
			ID:           row.ID,
			IncomeID:     row.IncomeID,
			TaxCode:      row.TaxCode,
			RateBasisPts: row.RateBasisPts,
			AmountCents:  row.AmountCents,
			Date:         row.Date,
		}
	}
	return out, nil
}

// SumTaxAccrualsByMonth returns the total accrued cents for a code in a month.
func (r *SQLiteRepository) SumTaxAccrualsByMonth(ctx context.Context, code string, year, month int) (core.Money, error) {
	total, err := r.readQueries.SumTaxAccrualsByMonth(ctx, SumTaxAccrualsByMonthParams{
		TaxCode:  code,
		PRINTF:   int64(year),
		PRINTF_2: int64(month),
	})
	if err != nil {
		return core.Money{}, fmt.Errorf("sum accruals by month: %w", err)
	}
	return core.Money{Cents: total}, nil
}
