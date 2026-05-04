package storage

import (
	"context"
	"fmt"
)

// MonthlyPrimaryRow holds aggregated expense totals for a month, broken down
// by primary category, plus the row total (excluding "Lavoro").
type MonthlyPrimaryRow struct {
	Month     int
	ByPrimary map[string]int64
	Total     int64
}

// MonthlyExpensesByPrimary returns 12 rows (Jan..Dec) for the given year,
// each row containing per-primary totals and a row total in cents. Months
// with no data still appear with empty maps and zero totals.
func (r *SQLiteRepository) MonthlyExpensesByPrimary(ctx context.Context, year int) ([]MonthlyPrimaryRow, []string, error) {
	rows, err := r.readQueries.GetMonthlyExpensesByPrimary(ctx, int64(year))
	if err != nil {
		return nil, nil, fmt.Errorf("monthly expenses by primary: %w", err)
	}

	out := make([]MonthlyPrimaryRow, 12)
	primarySeen := map[string]struct{}{}
	for i := range out {
		out[i] = MonthlyPrimaryRow{Month: i + 1, ByPrimary: map[string]int64{}}
	}
	for _, row := range rows {
		m := int(row.Month)
		if m < 1 || m > 12 {
			continue
		}
		out[m-1].ByPrimary[row.PrimaryCategory] += row.TotalCents
		out[m-1].Total += row.TotalCents
		primarySeen[row.PrimaryCategory] = struct{}{}
	}

	primaries := make([]string, 0, len(primarySeen))
	for p := range primarySeen {
		primaries = append(primaries, p)
	}
	return out, primaries, nil
}

// MonthlyIncomeByCategory returns a map indexed by category, each entry being
// the 12 monthly cents totals for the requested year.
func (r *SQLiteRepository) MonthlyIncomeByCategory(ctx context.Context, year int) (map[string][12]int64, error) {
	rows, err := r.readQueries.GetMonthlyIncomeByCategory(ctx, int64(year))
	if err != nil {
		return nil, fmt.Errorf("monthly income by category: %w", err)
	}
	out := map[string][12]int64{}
	for _, row := range rows {
		m := int(row.Month)
		if m < 1 || m > 12 {
			continue
		}
		arr := out[row.Category]
		arr[m-1] += row.TotalCents
		out[row.Category] = arr
	}
	return out, nil
}

// MonthlyTaxAccrualsByCode returns a map indexed by tax_code, each entry being
// the 12 monthly accrued cents for the requested year.
func (r *SQLiteRepository) MonthlyTaxAccrualsByCode(ctx context.Context, year int) (map[string][12]int64, error) {
	rows, err := r.readQueries.GetMonthlyTaxAccrualsByCode(ctx, int64(year))
	if err != nil {
		return nil, fmt.Errorf("monthly tax accruals by code: %w", err)
	}
	out := map[string][12]int64{}
	for _, row := range rows {
		m := int(row.Month)
		if m < 1 || m > 12 {
			continue
		}
		arr := out[row.TaxCode]
		arr[m-1] += row.TotalCents
		out[row.TaxCode] = arr
	}
	return out, nil
}
