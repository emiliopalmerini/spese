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

	now := time.Now()
	month := int(now.Month())
	data := struct {
		Year         int
		EditionRoman string
		MonthLong    string
	}{
		Year:         now.Year(),
		EditionRoman: romanNumeral(month),
		MonthLong:    italianMonthLong(month),
	}

	if err := s.templates.ExecuteTemplate(w, "dashboard_page", data); err != nil {
		slog.ErrorContext(r.Context(), "Dashboard template execution failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleDashboardStatHero returns the stat hero partial (BILANCIO - monthly balance)
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

	adapter, ok := s.expLister.(*adapters.SQLiteAdapter)
	if !ok {
		http.Error(w, "adapter not available", http.StatusInternalServerError)
		return
	}

	// Get current month expenses and income
	expenses, _ := adapter.GetMonthlyExpenseTotal(ctx, year, month)
	income, _ := adapter.GetMonthlyIncomeTotal(ctx, year, month)
	balance := income - expenses

	// Previous month for balance delta
	prevYear, prevMonth := prevYearMonth(year, month)
	prevExpenses, _ := adapter.GetMonthlyExpenseTotal(ctx, prevYear, prevMonth)
	prevIncome, _ := adapter.GetMonthlyIncomeTotal(ctx, prevYear, prevMonth)
	prevBalance := prevIncome - prevExpenses

	balanceDeltaPct := signedDeltaPct(balance, prevBalance)
	runRateCents := dailyRunRate(expenses, asOfDay(now))

	data := struct {
		HasData                bool
		Year                   int
		EditionRoman           string
		MonthLong              string
		PrevMonthShort         string
		BalanceInt             string
		BalanceDec             string
		BalanceIsNegative      bool
		BalanceDeltaPct        int
		BalanceDeltaPctAbs     int
		BalanceDeltaIsPositive bool // true = balance improved
		SpentFmt               string
		RunRateFmt             string
	}{
		HasData:                income > 0 || expenses > 0,
		Year:                   year,
		EditionRoman:           romanNumeral(month),
		MonthLong:              italianMonthLong(month),
		PrevMonthShort:         italianMonthShort(prevMonth),
		BalanceInt:             formatEurosInt(balance),
		BalanceDec:             formatEurosDec(balance),
		BalanceIsNegative:      balance < 0,
		BalanceDeltaPct:        balanceDeltaPct,
		BalanceDeltaPctAbs:     intAbs(balanceDeltaPct),
		BalanceDeltaIsPositive: balanceDeltaPct > 0,
		SpentFmt:               formatEurosInt(expenses),
		RunRateFmt:             formatEurosInt(runRateCents),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "stat_hero", data); err != nil {
		slog.ErrorContext(ctx, "Stat hero template failed", "error", err)
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

// handleSettings renders the Quaderno-style read-only settings page (ADR-0014).
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.templates == nil {
		http.Error(w, "templates not loaded", http.StatusInternalServerError)
		return
	}
	data := struct {
		Backend   string
		SheetName string
		Version   string
	}{
		Backend:   "SQLite + Sheets",
		SheetName: "—",
		Version:   "Quaderno · v0.8",
	}
	if err := s.templates.ExecuteTemplate(w, "settings_page", data); err != nil {
		slog.ErrorContext(r.Context(), "settings template failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleDashboardMonthlyTrend returns the Quaderno monthly bar-chart partial.
func (s *Server) handleDashboardMonthlyTrend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()

	year, month := parseYearMonth(r)

	adapter, ok := s.expLister.(*adapters.SQLiteAdapter)
	if !ok {
		http.Error(w, "adapter not available", http.StatusInternalServerError)
		return
	}

	rows, _, err := adapter.MonthlyExpensesByPrimary(ctx, year)
	if err != nil {
		slog.ErrorContext(ctx, "monthly trend query failed", "error", err)
		http.Error(w, "trend unavailable", http.StatusInternalServerError)
		return
	}
	series := [12]int64{}
	for i, row := range rows {
		if i < 12 {
			series[i] = row.Total
		}
	}

	var minV, maxV, sum int64 = -1, 0, 0
	for _, v := range series {
		sum += v
		if v > maxV {
			maxV = v
		}
		if minV < 0 || v < minV {
			minV = v
		}
	}
	if minV < 0 {
		minV = 0
	}
	mean := sum / 12

	labels := make([]string, 12)
	for i := 0; i < 12; i++ {
		labels[i] = italianMonthShort(i + 1)
	}

	data := struct {
		Series       []int64
		Labels       []string
		HighlightIdx int
		MinFmt       string
		MeanFmt      string
		MaxFmt       string
	}{
		Series:       series[:],
		Labels:       labels,
		HighlightIdx: month - 1,
		MinFmt:       formatEurosInt(minV),
		MeanFmt:      formatEurosInt(mean),
		MaxFmt:       formatEurosInt(maxV),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, "monthly_trend", data); err != nil {
		slog.ErrorContext(ctx, "monthly trend template failed", "error", err)
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
