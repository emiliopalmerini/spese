package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"spese/internal/adapters"
)

// handleDashboard renders the main dashboard page
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if s.templates == nil {
		slog.ErrorContext(r.Context(), "Templates not loaded")
		http.Error(w, "templates not loaded", http.StatusInternalServerError)
		return
	}

	if err := s.templates.ExecuteTemplate(w, "dashboard_page", nil); err != nil {
		slog.ErrorContext(r.Context(), "Dashboard template execution failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleDashboardStatHero returns the stat hero partial (monthly total with trend)
func (s *Server) handleDashboardStatHero(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()

	now := time.Now()
	year, month := now.Year(), int(now.Month())

	// Get current month expenses
	adapter, ok := s.expLister.(*adapters.SQLiteAdapter)
	if !ok {
		http.Error(w, "adapter not available", http.StatusInternalServerError)
		return
	}

	currentTotal, err := adapter.GetMonthlyExpenseTotal(ctx, year, month)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to get monthly total", "error", err)
		currentTotal = 0
	}

	// Get previous month expenses for trend
	prevMonth := month - 1
	prevYear := year
	if prevMonth < 1 {
		prevMonth = 12
		prevYear--
	}
	prevTotal, _ := adapter.GetMonthlyExpenseTotal(ctx, prevYear, prevMonth)

	// Calculate trend
	var trendValue string
	var trendClass string
	if prevTotal > 0 {
		diff := currentTotal - prevTotal
		diffEuros := float64(diff) / 100
		if diff < 0 {
			trendValue = formatEuros(-diff) + " in meno"
			trendClass = "stat-hero__trend--down"
		} else if diff > 0 {
			trendValue = formatEuros(diff) + " in più"
			trendClass = "stat-hero__trend--up"
		} else {
			trendValue = "invariato"
			trendClass = "stat-hero__trend--neutral"
		}
		_ = diffEuros // suppress unused variable warning
	}

	data := struct {
		HasData     bool
		Total       string
		PeriodLabel string
		TrendValue  string
		TrendClass  string
	}{
		HasData:     currentTotal > 0,
		Total:       formatEuros(currentTotal),
		PeriodLabel: "questo mese",
		TrendValue:  trendValue,
		TrendClass:  trendClass,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "stat_hero", data); err != nil {
		slog.ErrorContext(ctx, "Stat hero template failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleDashboardStatPills returns the stat pills partial (expenses + balance + savings rate)
func (s *Server) handleDashboardStatPills(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()

	now := time.Now()
	year, month := now.Year(), int(now.Month())

	adapter, ok := s.expLister.(*adapters.SQLiteAdapter)
	if !ok {
		http.Error(w, "adapter not available", http.StatusInternalServerError)
		return
	}

	// Get monthly totals
	expenses, _ := adapter.GetMonthlyExpenseTotal(ctx, year, month)
	income, _ := adapter.GetMonthlyIncomeTotal(ctx, year, month)

	balance := income - expenses
	balanceClass := ""
	if balance > 0 {
		balanceClass = "stat-pill__value--positive"
	} else if balance < 0 {
		balanceClass = "stat-pill__value--negative"
	}

	// Calculate savings rate
	savingsRate := 0
	if income > 0 {
		savingsRate = int((balance * 100) / income)
	}

	data := struct {
		TotalExpenses string
		Balance       string
		BalanceClass  string
		SavingsRate   int
	}{
		TotalExpenses: formatEuros(expenses),
		Balance:       formatEuros(balance),
		BalanceClass:  balanceClass,
		SavingsRate:   savingsRate,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "stat_pills", data); err != nil {
		slog.ErrorContext(ctx, "Stat pills template failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleDashboardTransactions returns recent transactions partial
func (s *Server) handleDashboardTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()

	adapter, ok := s.expLister.(*adapters.SQLiteAdapter)
	if !ok {
		http.Error(w, "adapter not available", http.StatusInternalServerError)
		return
	}

	transactions, err := adapter.GetRecentTransactions(ctx, 10)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to get recent transactions", "error", err)
		transactions = []adapters.Transaction{}
	}

	// Convert to template-friendly format
	type txView struct {
		ID          string
		Type        string
		Description string
		Category    string
		Amount      string
		Date        string
	}
	var txs []txView
	for _, tx := range transactions {
		txs = append(txs, txView{
			ID:          tx.ID,
			Type:        tx.Type,
			Description: tx.Description,
			Category:    tx.Category,
			Amount:      formatEuros(tx.AmountCents),
			Date:        tx.Date.Format("02/01"),
		})
	}

	data := struct {
		Transactions []txView
	}{
		Transactions: txs,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "recent_transactions", data); err != nil {
		slog.ErrorContext(ctx, "Transactions template failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleDashboardTrend returns trend data for Chart.js
func (s *Server) handleDashboardTrend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month"
	}

	adapter, ok := s.expLister.(*adapters.SQLiteAdapter)
	if !ok {
		http.Error(w, "adapter not available", http.StatusInternalServerError)
		return
	}

	trendData, err := adapter.GetExpenseTrend(ctx, period)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to get trend data", "error", err)
		trendData = []adapters.TrendPoint{}
	}

	// Convert to JSON-friendly format
	type point struct {
		Date   string `json:"date"`
		Amount int64  `json:"amount"`
	}
	var points []point
	for _, p := range trendData {
		points = append(points, point{
			Date:   p.Date,
			Amount: p.AmountCents,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(points)
}

// handleDashboardCategoriesList returns category breakdown as HTML partial
func (s *Server) handleDashboardCategoriesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month"
	}

	adapter, ok := s.expLister.(*adapters.SQLiteAdapter)
	if !ok {
		http.Error(w, "adapter not available", http.StatusInternalServerError)
		return
	}

	catData, err := adapter.GetCategoryBreakdown(ctx, period)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to get category data", "error", err, "period", period)
		catData = []adapters.CategoryTotal{}
	}

	// Find max for percentage calculation
	var maxAmount int64
	for _, c := range catData {
		if c.AmountCents > maxAmount {
			maxAmount = c.AmountCents
		}
	}

	// Convert to template-friendly format
	type catView struct {
		Name    string
		Amount  string
		Percent int
	}
	var cats []catView
	for _, c := range catData {
		percent := 0
		if maxAmount > 0 {
			percent = int((c.AmountCents * 100) / maxAmount)
		}
		cats = append(cats, catView{
			Name:    c.Name,
			Amount:  formatEuros(c.AmountCents),
			Percent: percent,
		})
	}

	data := struct {
		Categories []catView
	}{
		Categories: cats,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "category_breakdown", data); err != nil {
		slog.ErrorContext(ctx, "Category breakdown template failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleDashboardRecurrents returns the recurrent expenses list partial
func (s *Server) handleDashboardRecurrents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()

	adapter, ok := s.expLister.(*adapters.SQLiteAdapter)
	if !ok {
		http.Error(w, "adapter not available", http.StatusInternalServerError)
		return
	}

	recurrents, err := adapter.GetActiveRecurrentExpenses(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to get recurrent expenses", "error", err)
		recurrents = []adapters.RecurrentExpenseItem{}
	}

	type recView struct {
		ID          int64
		Description string
		Amount      string
		Category    string
		Frequency   string
	}
	var recs []recView
	for _, r := range recurrents {
		freq := "mensile"
		if r.Frequency == "yearly" {
			freq = "annuale"
		}
		recs = append(recs, recView{
			ID:          r.ID,
			Description: r.Description,
			Amount:      formatEuros(r.AmountCents),
			Category:    r.Category,
			Frequency:   freq,
		})
	}

	data := struct {
		Recurrents []recView
	}{
		Recurrents: recs,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "recurrent_list", data); err != nil {
		slog.ErrorContext(ctx, "Recurrent list template failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleFormExpense returns the expense form partial for bottom sheet
func (s *Server) handleFormExpense(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	now := time.Now()
	cats, _, err := s.taxReader.List(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to get categories", "error", err)
	}

	data := struct {
		Day        int
		Month      int
		Categories []string
		Subcats    []string
	}{
		Day:        now.Day(),
		Month:      int(now.Month()),
		Categories: cats,
		Subcats:    []string{},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "expense_form", data); err != nil {
		slog.ErrorContext(r.Context(), "Expense form template failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleFormIncome returns the income form partial for bottom sheet
func (s *Server) handleFormIncome(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	now := time.Now()

	adapter, ok := s.expLister.(*adapters.SQLiteAdapter)
	var categories []string
	if ok {
		cats, err := adapter.ListIncomeCategories(r.Context())
		if err != nil {
			slog.ErrorContext(r.Context(), "Failed to get income categories", "error", err)
		}
		categories = cats
	}

	data := struct {
		Day        int
		Month      int
		Categories []string
	}{
		Day:        now.Day(),
		Month:      int(now.Month()),
		Categories: categories,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "income_form", data); err != nil {
		slog.ErrorContext(r.Context(), "Income form template failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleFormRecurring returns the recurring expense form partial for bottom sheet
func (s *Server) handleFormRecurring(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	cats, _, err := s.taxReader.List(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to get categories", "error", err)
	}

	data := struct {
		Categories []string
		Subcats    []string
	}{
		Categories: cats,
		Subcats:    []string{},
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "recurrent_form", data); err != nil {
		slog.ErrorContext(r.Context(), "Recurrent form template failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleFormRecurrentEdit returns the recurring expense edit form partial for bottom sheet
func (s *Server) handleFormRecurrentEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	idStr := r.URL.Query().Get("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "ID non valido", http.StatusBadRequest)
		return
	}

	adapter, ok := s.expLister.(*adapters.SQLiteAdapter)
	if !ok {
		http.Error(w, "Backend non supportato", http.StatusInternalServerError)
		return
	}

	expense, err := adapter.GetRecurrentExpenseByID(r.Context(), id)
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to get recurrent expense", "error", err, "id", id)
		http.Error(w, "Spesa ricorrente non trovata", http.StatusNotFound)
		return
	}

	cats, subs, err := s.taxReader.List(r.Context())
	if err != nil {
		slog.ErrorContext(r.Context(), "Failed to get categories", "error", err)
	}

	data := struct {
		ID          int64
		Amount      string
		Description string
		StartDate   string
		EndDate     string
		Frequency   string
		Primary     string
		Secondary   string
		Categories  []string
		Subcats     []string
	}{
		ID:          expense.ID,
		Amount:      formatDecimal(expense.AmountCents),
		Description: expense.Description,
		StartDate:   expense.StartDate,
		EndDate:     expense.EndDate,
		Frequency:   expense.Frequency,
		Primary:     expense.Category,
		Secondary:   expense.Subcategory,
		Categories:  cats,
		Subcats:     subs,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "recurrent_edit_form_sheet", data); err != nil {
		slog.ErrorContext(r.Context(), "Recurrent edit form template failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func formatDecimal(cents int64) string {
	return strconv.FormatFloat(float64(cents)/100, 'f', 2, 64)
}

// handleDashboardStatGrid returns the stat grid partial (daily avg, week change, velocity, ratio)
func (s *Server) handleDashboardStatGrid(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()

	adapter, ok := s.expLister.(*adapters.SQLiteAdapter)
	if !ok {
		http.Error(w, "adapter not available", http.StatusInternalServerError)
		return
	}

	// Get daily average
	dailyAvg, _ := adapter.GetDailyAverage(ctx)
	dailyAvgStr := "€0"
	if dailyAvg != nil {
		dailyAvgStr = formatEuros(dailyAvg.AverageCents)
	}

	// Get week-over-week change
	weekChange, _ := adapter.GetWeekOverWeekChange(ctx)
	weekChangeStr := "—"
	weekChangeArrow := ""
	if weekChange != nil && weekChange.LastWeekCents > 0 {
		weekChangeStr = strconv.Itoa(int(weekChange.ChangePercent)) + "%"
		if weekChange.IsDown {
			weekChangeArrow = "↓"
		} else {
			weekChangeArrow = "↑"
		}
	}

	// Get velocity stats
	velocity, _ := adapter.GetVelocityStats(ctx)
	velocityLabel := "In linea"
	velocityClass := "velocity-label--on-track"
	monthProgress := 0
	budgetProgress := 0
	if velocity != nil {
		monthProgress = velocity.MonthProgressPercent
		budgetProgress = velocity.BudgetProgressPercent
		switch velocity.Status {
		case "ahead":
			velocityLabel = "Avanti"
			velocityClass = "velocity-label--ahead"
		case "behind":
			velocityLabel = "Indietro"
			velocityClass = "velocity-label--behind"
		}
	}

	// Get fixed/variable ratio
	ratio, _ := adapter.GetFixedVariableRatio(ctx)
	fixedPercent := 0
	if ratio != nil {
		fixedPercent = ratio.FixedPercent
	}

	data := struct {
		DailyAverage    string
		WeekChangeStr   string
		WeekChangeArrow string
		WeekIsDown      bool
		MonthProgress   int
		BudgetProgress  int
		VelocityLabel   string
		VelocityClass   string
		FixedPercent    int
	}{
		DailyAverage:    dailyAvgStr,
		WeekChangeStr:   weekChangeStr,
		WeekChangeArrow: weekChangeArrow,
		WeekIsDown:      weekChange != nil && weekChange.IsDown,
		MonthProgress:   monthProgress,
		BudgetProgress:  budgetProgress,
		VelocityLabel:   velocityLabel,
		VelocityClass:   velocityClass,
		FixedPercent:    fixedPercent,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "stat_grid", data); err != nil {
		slog.ErrorContext(ctx, "Stat grid template failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleDashboardProjections returns the projections partial (YTD + forecast)
func (s *Server) handleDashboardProjections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()

	adapter, ok := s.expLister.(*adapters.SQLiteAdapter)
	if !ok {
		http.Error(w, "adapter not available", http.StatusInternalServerError)
		return
	}

	// Get YTD totals
	ytd, _ := adapter.GetYTDTotals(ctx)
	ytdExpenses := "€0"
	ytdIncome := "€0"
	if ytd != nil {
		ytdExpenses = formatEuros(ytd.ExpensesCents)
		ytdIncome = formatEuros(ytd.IncomeCents)
	}

	// Get forecast
	forecast, _ := adapter.GetMonthEndForecast(ctx)
	forecastStr := "€0"
	forecastNote := ""
	if forecast != nil {
		forecastStr = formatEuros(forecast.ForecastCents)
		forecastNote = forecast.BasedOn
	}

	data := struct {
		YTDExpenses  string
		YTDIncome    string
		Forecast     string
		ForecastNote string
	}{
		YTDExpenses:  ytdExpenses,
		YTDIncome:    ytdIncome,
		Forecast:     forecastStr,
		ForecastNote: forecastNote,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "projections", data); err != nil {
		slog.ErrorContext(ctx, "Projections template failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleDashboardIncomeBreakdown returns the income breakdown partial
func (s *Server) handleDashboardIncomeBreakdown(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()

	adapter, ok := s.expLister.(*adapters.SQLiteAdapter)
	if !ok {
		http.Error(w, "adapter not available", http.StatusInternalServerError)
		return
	}

	catData, err := adapter.GetIncomeCategoryBreakdown(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to get income category data", "error", err)
		catData = []adapters.CategoryTotal{}
	}

	// Find max for percentage calculation
	var maxAmount int64
	for _, c := range catData {
		if c.AmountCents > maxAmount {
			maxAmount = c.AmountCents
		}
	}

	// Convert to template-friendly format
	type catView struct {
		Name    string
		Amount  string
		Percent int
	}
	var cats []catView
	for _, c := range catData {
		percent := 0
		if maxAmount > 0 {
			percent = int((c.AmountCents * 100) / maxAmount)
		}
		cats = append(cats, catView{
			Name:    c.Name,
			Amount:  formatEuros(c.AmountCents),
			Percent: percent,
		})
	}

	data := struct {
		Categories []catView
	}{
		Categories: cats,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "income_breakdown", data); err != nil {
		slog.ErrorContext(ctx, "Income breakdown template failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleDashboardRecurrentsWithSummary returns the recurrent expenses list with summary
func (s *Server) handleDashboardRecurrentsWithSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()

	adapter, ok := s.expLister.(*adapters.SQLiteAdapter)
	if !ok {
		http.Error(w, "adapter not available", http.StatusInternalServerError)
		return
	}

	recurrents, err := adapter.GetActiveRecurrentExpenses(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to get recurrent expenses", "error", err)
		recurrents = []adapters.RecurrentExpenseItem{}
	}

	// Get monthly total
	monthlyTotal := adapter.GetRecurrentMonthlyTotal(ctx)

	type recView struct {
		ID          int64
		Description string
		Amount      string
		Category    string
		Frequency   string
	}
	var recs []recView
	for _, r := range recurrents {
		freq := "mensile"
		if r.Frequency == "yearly" {
			freq = "annuale"
		} else if r.Frequency == "weekly" {
			freq = "settimanale"
		} else if r.Frequency == "daily" {
			freq = "giornaliero"
		}
		recs = append(recs, recView{
			ID:          r.ID,
			Description: r.Description,
			Amount:      formatEuros(r.AmountCents),
			Category:    r.Category,
			Frequency:   freq,
		})
	}

	data := struct {
		Recurrents   []recView
		MonthlyTotal string
	}{
		Recurrents:   recs,
		MonthlyTotal: formatEuros(monthlyTotal),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "recurrent_list", data); err != nil {
		slog.ErrorContext(ctx, "Recurrent list template failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
