package render_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"spese/internal/features/accounts"
	"spese/internal/features/actions"
	"spese/internal/features/dashboard"
	"spese/internal/features/recurring"
	"spese/internal/features/reports"
	"spese/internal/features/snapshots"
	"spese/internal/features/transactions"
	"spese/internal/features/transfers"
	"spese/internal/kernel"
	"spese/internal/render"
	"spese/web"
)

func TestTemplatesRender(t *testing.T) {
	templates, err := render.Load(web.TemplatesFS)
	if err != nil {
		t.Fatalf("load templates: %v", err)
	}

	day := mustDate(t, "2026-05-15")
	month := mustDate(t, "2026-05")
	account := accounts.Account{
		Name:     "Conto corrente principale",
		Type:     accounts.Asset,
		Class:    accounts.Cash,
		Currency: "EUR",
		Note:     "Uso quotidiano",
	}
	accountList := []accounts.Account{account}

	cases := map[string]any{
		"accounts/list": accounts.ListView{
			Accounts: accountList,
		},
		"dashboard/home": dashboard.View{
			Items: []dashboard.Item{
				{Label: "Liquidita", Value: "12.345,67 EUR"},
				{Label: "", Value: ""},
				{Label: "Mese corrente"},
				{Label: "Uscite", Value: "1.234,00 EUR"},
			},
		},
		"recurring/list": recurring.ListView{
			Recurrings: []recurring.Recurring{
				{
					Label:      "Affitto",
					Kind:       transactions.Expense,
					Account:    account.Name,
					Amount:     kernel.Money(95000),
					Category:   "Casa",
					DayOfMonth: 5,
					Active:     true,
				},
			},
			Accounts: accountList,
		},
		"reports/balance_sheet": reports.BalanceSheetView{
			Rows: []reports.BalanceRow{
				{Account: account.Name, Type: "Asset", Class: "Cash", Balance: kernel.Money(1234567), LatestMonth: month},
			},
		},
		"reports/income_statement": reports.IncomeStatementView{
			Rows: []reports.IncomeRow{
				{Month: month, Revenue: kernel.Money(350000), Expenses: kernel.Money(-140000), NetIncome: kernel.Money(210000), SavingsRate: 0.60},
			},
		},
		"reports/index": reports.IndexView{},
		"reports/investments": reports.InvestmentsView{
			Rows: []reports.InvestmentRow{
				{Account: "Broker", CostBasis: kernel.Money(800000), Value: kernel.Money(930000), Return: kernel.Money(130000), ReturnPct: 0.1625, LatestMonth: month},
			},
		},
		"reports/nw_timeline": reports.NwTimelineView{
			Rows: []reports.NwRow{
				{Month: month, NetWorth: kernel.Money(4200000)},
			},
		},
		"snapshots/form": snapshots.FormView{
			Month: month,
			Rows: []snapshots.Row{
				{Account: account, LastBalance: kernel.Money(1200000), LastMonth: month},
			},
		},
		"transactions/list": transactions.ListView{
			Transactions: []transactions.Transaction{
				{Date: day, Kind: transactions.Expense, Account: account.Name, Amount: kernel.Money(-4230), Category: "Cibo", Subcategory: "Pranzo", Payee: "Bar"},
			},
			Accounts: accountList,
		},
		"transfers/form": transfers.FormView{
			Accounts: accountList,
			Today:    day,
		},
	}

	for name, data := range cases {
		t.Run(strings.ReplaceAll(name, "/", "_"), func(t *testing.T) {
			w := httptest.NewRecorder()
			if err := templates.Render(w, name, data); err != nil {
				t.Fatalf("render %s: %v", name, err)
			}
			body := w.Body.String()
			if !strings.Contains(body, "<main class=\"container\">") {
				t.Fatalf("render %s did not include the base layout", name)
			}
			if !strings.Contains(body, "data-global-action-toggle") {
				t.Fatalf("render %s did not include global actions", name)
			}
		})
	}
}

func TestTemplateFragmentsRender(t *testing.T) {
	templates, err := render.Load(web.TemplatesFS)
	if err != nil {
		t.Fatalf("load templates: %v", err)
	}

	day := mustDate(t, "2026-05-15")
	month := mustDate(t, "2026-05")
	account := accounts.Account{
		Name:     "Conto corrente principale",
		Type:     accounts.Asset,
		Class:    accounts.Cash,
		Currency: "EUR",
	}
	accountPicker := actions.AccountPickerView{
		Accounts: []accounts.Account{account},
		Today:    day,
	}

	cases := map[string]any{
		"action_form_account":     nil,
		"action_form_recurring":   accountPicker,
		"action_form_transaction": accountPicker,
		"action_form_transfer":    accountPicker,
		"action_form_snapshot": snapshots.FormView{
			Month: month,
			Rows: []snapshots.Row{
				{Account: account, LastBalance: kernel.Money(1200000), LastMonth: month},
			},
		},
	}

	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			if err := templates.RenderFragment(w, name, data); err != nil {
				t.Fatalf("render fragment %s: %v", name, err)
			}
			body := w.Body.String()
			if !strings.Contains(body, "data-global-action-form") {
				t.Fatalf("fragment %s did not render an action form", name)
			}
		})
	}
}

func mustDate(t *testing.T, value string) kernel.Date {
	t.Helper()
	d, err := kernel.ParseDate(value)
	if err != nil {
		t.Fatalf("parse date %q: %v", value, err)
	}
	return d
}
