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
			KPIs: []dashboard.KPI{
				{Label: "Patrimonio netto", Value: "42.000,00 €", Help: "Ultimo mese disponibile", Tone: "positive"},
				{Label: "Risparmio mese", Value: "2.100,00 €", Help: "Entrate meno uscite", Tone: "positive"},
			},
			CashFlow: dashboard.CashFlowChart{
				Width: 720, Height: 230, AxisStart: 32, AxisEnd: 702, Baseline: 184,
				MaxFmt: "3.500,00 €",
				Months: []dashboard.MonthTick{
					{X: 160, Y: 214, Label: "Mag", Detail: "2026-05"},
				},
				Bars: []dashboard.CashFlowBar{
					{X: 140, Y: 40, Width: 16, Height: 144, Class: "income", Label: "Mag", Kind: "Entrate", ValueFmt: "3.500,00 €"},
					{X: 162, Y: 126, Width: 16, Height: 58, Class: "expense", Label: "Mag", Kind: "Uscite", ValueFmt: "1.400,00 €"},
				},
			},
			NetWorth: dashboard.LineChart{
				Width: 720, Height: 220, AxisStart: 32, AxisEnd: 702, Baseline: 176,
				PointsAttr: "32,160 702,40",
				LatestFmt:  "42.000,00 €",
				MinFmt:     "40.000,00 €",
				MaxFmt:     "42.000,00 €",
				Points: []dashboard.LinePoint{
					{X: 32, Y: 160, Label: "Apr", ValueFmt: "40.000,00 €"},
					{X: 702, Y: 40, Label: "Mag", ValueFmt: "42.000,00 €"},
				},
				Labels: []dashboard.MonthTick{
					{X: 32, Y: 204, Label: "Apr", Detail: "2026-04"},
					{X: 702, Y: 204, Label: "Mag", Detail: "2026-05"},
				},
			},
			Allocation: dashboard.AllocationChart{
				TotalFmt: "42.000,00 €",
				Rows: []dashboard.AllocationRow{
					{Label: "Cash", ValueFmt: "12.000,00 €", PercentFmt: "29%", Width: 29, Tone: "positive"},
					{Label: "Investments", ValueFmt: "30.000,00 €", PercentFmt: "71%", Width: 71, Tone: "positive"},
				},
			},
			Investments: dashboard.InvestmentChart{
				TotalValueFmt:  "30.000,00 €",
				TotalReturnFmt: "3.000,00 €",
				ReturnPctFmt:   "11.1%",
				Rows: []dashboard.InvestmentChartRow{
					{Account: "Broker", CostFmt: "27.000,00 €", ValueFmt: "30.000,00 €", ReturnFmt: "3.000,00 €", ReturnPctFmt: "11.1%", CostWidth: 90, ValueWidth: 100, ReturnTone: "positive"},
				},
			},
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

func TestDashboardTemplateEscapesChartLabels(t *testing.T) {
	templates, err := render.Load(web.TemplatesFS)
	if err != nil {
		t.Fatalf("load templates: %v", err)
	}

	malicious := `"><script>alert(1)</script><img src=x onerror="alert(1)">`
	view := dashboard.View{
		KPIs: []dashboard.KPI{
			{Label: malicious, Value: "1,00 €", Help: malicious, Tone: "positive"},
		},
		CashFlow: dashboard.CashFlowChart{
			Width: 720, Height: 230, AxisStart: 32, AxisEnd: 702, Baseline: 184,
			Months: []dashboard.MonthTick{{X: 120, Y: 214, Label: malicious}},
			Bars: []dashboard.CashFlowBar{
				{X: 100, Y: 120, Width: 16, Height: 64, Class: "income", Label: malicious, Kind: malicious, ValueFmt: "1,00 €"},
			},
		},
		NetWorth: dashboard.LineChart{
			Width: 720, Height: 220, AxisStart: 32, AxisEnd: 702, Baseline: 176,
			PointsAttr: "32,160",
			Points:     []dashboard.LinePoint{{X: 32, Y: 160, Label: malicious, ValueFmt: "1,00 €"}},
			Labels:     []dashboard.MonthTick{{X: 32, Y: 204, Label: malicious}},
		},
		Allocation: dashboard.AllocationChart{
			TotalFmt: "1,00 €",
			Rows: []dashboard.AllocationRow{
				{Label: malicious, ValueFmt: "1,00 €", PercentFmt: "100%", Width: 100, Tone: "positive"},
			},
		},
		Investments: dashboard.InvestmentChart{
			TotalValueFmt: "1,00 €", TotalReturnFmt: "0,00 €", ReturnPctFmt: "0.0%",
			Rows: []dashboard.InvestmentChartRow{
				{Account: malicious, CostFmt: "1,00 €", ValueFmt: "1,00 €", ReturnFmt: "0,00 €", ReturnPctFmt: "0.0%", CostWidth: 100, ValueWidth: 100, ReturnTone: "neutral"},
			},
		},
		Items: []dashboard.Item{{Label: malicious, Value: malicious}},
	}

	w := httptest.NewRecorder()
	if err := templates.Render(w, "dashboard/home", view); err != nil {
		t.Fatalf("render dashboard/home: %v", err)
	}
	body := w.Body.String()
	if strings.Contains(body, `"><script>alert(1)</script>`) || strings.Contains(body, "<img src=x") || strings.Contains(body, `onerror="alert`) {
		t.Fatalf("dashboard rendered unsafe markup:\n%s", body)
	}
	if strings.Contains(body, "ZgotmplZ") {
		t.Fatalf("dashboard rendered a sanitized placeholder:\n%s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;alert") {
		t.Fatalf("dashboard did not preserve escaped malicious label:\n%s", body)
	}
	if !strings.Contains(body, "dashboard-svg--bars") || !strings.Contains(body, "dashboard-svg--line") {
		t.Fatalf("dashboard did not render chart SVGs:\n%s", body)
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
