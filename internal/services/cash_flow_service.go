package services

import (
	"context"
	"fmt"
	"sort"

	"spese/internal/storage"
)

// CashFlowRow is one rendered row in the cash flow panel.
// Cents in Months are positive for income, negative for taxes.
type CashFlowRow struct {
	Label   string
	Group   string // "income" | "tax"
	Section string
	Months  [12]int64
	Total   int64
}

// CashFlowService combines income totals and tax accruals for a given year
// into the grouped, signed rows rendered by the cash flow panel.
type CashFlowService struct {
	storage *storage.SQLiteRepository
}

// NewCashFlowService wires the service.
func NewCashFlowService(storage *storage.SQLiteRepository) *CashFlowService {
	return &CashFlowService{storage: storage}
}

// sectionForCategory returns the rendered section name for an income category.
// Unknown categories fall back to "Other".
func sectionForCategory(cat string) string {
	switch cat {
	case "EFreelance", "GFreelance", "Freelance E", "Freelance G", "2DP+", "2DP":
		return "Sole Proprietorship"
	case "ESalary", "GSalary", "Stipendio E", "Stipendio G":
		return "Employment"
	case "Gifts n reimbursements", "Regali", "Rimborsi":
		return "Other"
	case "Gold", "2DM", "Interessi":
		return "Other"
	}
	return "Other"
}

// taxLabels translates tax codes into the label rendered in the panel.
var taxLabels = map[string]string{
	"imposta_sostitutiva": "Imposta sostitutiva",
	"inps":                "INPS",
}

// BuildYear assembles a slice of CashFlowRow for the requested year.
func (s *CashFlowService) BuildYear(ctx context.Context, year int) ([]CashFlowRow, error) {
	if s == nil || s.storage == nil {
		return nil, fmt.Errorf("cash flow service not configured")
	}
	incomes, err := s.storage.MonthlyIncomeByCategory(ctx, year)
	if err != nil {
		return nil, err
	}
	taxes, err := s.storage.MonthlyTaxAccrualsByCode(ctx, year)
	if err != nil {
		return nil, err
	}

	rows := make([]CashFlowRow, 0, len(incomes)+len(taxes))
	for cat, months := range incomes {
		row := CashFlowRow{
			Label:   cat,
			Group:   "income",
			Section: sectionForCategory(cat),
			Months:  months,
		}
		for _, c := range months {
			row.Total += c
		}
		rows = append(rows, row)
	}
	for code, months := range taxes {
		row := CashFlowRow{
			Label:   labelForTaxCode(code),
			Group:   "tax",
			Section: "Tasse statali",
		}
		for i, c := range months {
			row.Months[i] = -c
			row.Total -= c
		}
		rows = append(rows, row)
	}

	sortRows(rows)
	return rows, nil
}

func labelForTaxCode(code string) string {
	if l, ok := taxLabels[code]; ok {
		return l
	}
	return code
}

// sortRows orders rows by section, then group (income before tax), then label.
func sortRows(rows []CashFlowRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Section != rows[j].Section {
			return rows[i].Section < rows[j].Section
		}
		if rows[i].Group != rows[j].Group {
			return rows[i].Group < rows[j].Group
		}
		return rows[i].Label < rows[j].Label
	})
}
