package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"spese/internal/core"
	"spese/internal/storage"
)

// TaxAccrualService computes and persists tax accruals from freelance incomes.
// Configurable tax rates are loaded from storage at compute time.
type TaxAccrualService struct {
	storage *storage.SQLiteRepository
}

// NewTaxAccrualService wires the service.
func NewTaxAccrualService(storage *storage.SQLiteRepository) *TaxAccrualService {
	return &TaxAccrualService{storage: storage}
}

// OnIncomeCreated computes the configured tax accruals for an income whose
// category is configured as freelance. Errors are logged but never blocking.
func (s *TaxAccrualService) OnIncomeCreated(ctx context.Context, incomeID int64, income core.Income) {
	if s == nil || s.storage == nil {
		return
	}

	isFreelance, err := s.storage.IsFreelanceIncomeCategory(ctx, income.Category)
	if err != nil {
		slog.WarnContext(ctx, "Tax accrual: failed to check freelance category",
			"category", income.Category, "error", err)
		return
	}
	if !isFreelance {
		return
	}

	codes, err := s.storage.ListTaxRateCodes(ctx)
	if err != nil {
		slog.WarnContext(ctx, "Tax accrual: failed to list tax codes", "error", err)
		return
	}

	for _, code := range codes {
		rate, err := s.storage.ResolveTaxRate(ctx, code, income.Date.Time)
		if err != nil {
			if errors.Is(err, storage.ErrTaxRateNotFound) {
				slog.WarnContext(ctx, "Tax accrual: rate not configured for date",
					"code", code, "date", income.Date.Time)
				continue
			}
			slog.WarnContext(ctx, "Tax accrual: rate lookup failed",
				"code", code, "error", err)
			continue
		}
		amountCents := computeAccrualCents(income.Amount.Cents, rate.RateBasisPts)
		if err := s.storage.InsertTaxAccrual(ctx, incomeID, code, rate.RateBasisPts,
			core.Money{Cents: amountCents}, income.Date.Time); err != nil {
			slog.ErrorContext(ctx, "Tax accrual: insert failed",
				"income_id", incomeID, "code", code, "error", err)
		} else {
			slog.InfoContext(ctx, "Tax accrual recorded",
				"income_id", incomeID,
				"code", code,
				"amount_cents", amountCents,
				"basis_pts", rate.RateBasisPts)
		}
	}
}

// computeAccrualCents returns round-half-up cents for income * basis_pts/10000.
func computeAccrualCents(grossCents, basisPts int64) int64 {
	if grossCents <= 0 || basisPts <= 0 {
		return 0
	}
	num := grossCents * basisPts
	// Round half up
	return (num + 5000) / 10000
}

// summary helpers (used by ADR-0006 cash flow service)

// MonthlyAccrualByCode returns the accrued cents for a tax code in a year/month.
func (s *TaxAccrualService) MonthlyAccrualByCode(ctx context.Context, code string, year, month int) (core.Money, error) {
	if s == nil || s.storage == nil {
		return core.Money{}, fmt.Errorf("tax accrual service not configured")
	}
	return s.storage.SumTaxAccrualsByMonth(ctx, code, year, month)
}
