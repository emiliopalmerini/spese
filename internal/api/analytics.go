package api

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type monthlyFlow struct {
	Month        string `json:"month"`
	IncomeCents  int64  `json:"incomeCents"`
	ExpenseCents int64  `json:"expenseCents"`
	RefundCents  int64  `json:"refundCents"`
	NetCents     int64  `json:"netCents"`
	Drilldown    string `json:"drilldown"`
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	month := r.URL.Query().Get("month")
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	if _, err := time.Parse("2006-01", month); err != nil {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Mese non valido.", nil)
		return
	}
	from, to := month+"-01", monthEnd(month)
	var income, expenses, refunds, recurring int64
	err := h.store.DB().QueryRowContext(r.Context(), `
		SELECT
			coalesce(sum(CASE WHEN kind = 'income' THEN amount_cents ELSE 0 END), 0),
			coalesce(sum(CASE WHEN kind = 'expense' THEN amount_cents ELSE 0 END), 0),
			coalesce(sum(CASE WHEN kind = 'refund' THEN amount_cents ELSE 0 END), 0),
			coalesce(sum(CASE WHEN kind = 'expense' AND origin = 'recurring' THEN amount_cents ELSE 0 END), 0)
		FROM movements WHERE status = 'posted' AND business_date BETWEEN ? AND ?
	`, from, to).Scan(&income, &expenses, &refunds, &recurring)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile calcolare la panoramica.", nil)
		return
	}
	netExpenses := expenses - refunds
	attention := map[string]int{}
	var uncategorized, unreconciled, recurringToReview int
	_ = h.store.DB().QueryRowContext(r.Context(), `
		SELECT count(*) FROM movements m WHERE m.status = 'posted' AND m.kind IN ('expense', 'income', 'refund')
		AND NOT EXISTS (
			SELECT 1 FROM movement_allocations a JOIN categories c ON c.id = a.category_id
			WHERE a.movement_id = m.id AND c.parent_id IS NOT NULL
		)
	`).Scan(&uncategorized)
	_ = h.store.DB().QueryRowContext(r.Context(), `
		SELECT count(*) FROM accounts a WHERE a.archived_at = '' AND NOT EXISTS (
			SELECT 1 FROM account_reconciliations r WHERE r.account_id = a.id AND substr(r.closed_through, 1, 7) = ?
		)
	`, month).Scan(&unreconciled)
	_ = h.store.DB().QueryRowContext(r.Context(), `
		SELECT count(*) FROM recurring_occurrences WHERE status IN ('draft', 'failed') AND scheduled_for <= date('now')
	`).Scan(&recurringToReview)
	attention["uncategorized"] = uncategorized
	attention["unreconciledAccounts"] = unreconciled
	attention["recurringToReview"] = recurringToReview

	top, err := h.categoryRows(r, from, to, 5)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile caricare le categorie principali.", nil)
		return
	}
	accounts, netWorth := h.accountBalances(r, to)
	h.writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"month": month, "incomeCents": income, "recurringExpenseCents": recurring,
			"otherExpenseCents": netExpenses - recurring, "expenseCents": netExpenses,
			"savingsCents": income - netExpenses, "attention": attention,
			"topCategories": top, "accounts": accounts, "netWorthCents": netWorth,
			"drilldown": "/movimenti?from=" + from + "&to=" + to,
		},
	})
}

func (h *Handler) cashFlow(w http.ResponseWriter, r *http.Request) {
	from, to := analyticsRange(r)
	rows, err := h.store.DB().QueryContext(r.Context(), `
		SELECT substr(business_date, 1, 7),
			coalesce(sum(CASE WHEN kind = 'income' THEN amount_cents ELSE 0 END), 0),
			coalesce(sum(CASE WHEN kind = 'expense' THEN amount_cents ELSE 0 END), 0),
			coalesce(sum(CASE WHEN kind = 'refund' THEN amount_cents ELSE 0 END), 0)
		FROM movements WHERE status = 'posted' AND business_date BETWEEN ? AND ?
		GROUP BY substr(business_date, 1, 7) ORDER BY 1
	`, from, to)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile calcolare il flusso.", nil)
		return
	}
	defer rows.Close()
	var result []monthlyFlow
	for rows.Next() {
		var item monthlyFlow
		if err := rows.Scan(&item.Month, &item.IncomeCents, &item.ExpenseCents, &item.RefundCents); err != nil {
			h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile leggere il flusso.", nil)
			return
		}
		item.ExpenseCents -= item.RefundCents
		item.NetCents = item.IncomeCents - item.ExpenseCents
		item.Drilldown = "/movimenti?from=" + item.Month + "-01&to=" + monthEnd(item.Month)
		result = append(result, item)
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": result, "from": from, "to": to})
}

func (h *Handler) categoryAnalysis(w http.ResponseWriter, r *http.Request) {
	from, to := analyticsRange(r)
	rows, err := h.categoryRows(r, from, to, 0)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile calcolare le categorie.", nil)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": rows, "from": from, "to": to})
}

func (h *Handler) categoryRows(r *http.Request, from, to string, limit int) ([]map[string]any, error) {
	query := `
		SELECT c.id, c.name, coalesce(parent.name || ' › ', '') || c.name AS path, c.color, c.icon,
			coalesce(sum(CASE WHEN m.kind = 'refund' THEN -a.amount_cents ELSE a.amount_cents END), 0) AS total,
			count(DISTINCT m.id), max(m.business_date),
			(SELECT count(*) FROM merchant_rules mr WHERE mr.category_id = c.id AND mr.archived_at = '')
		FROM categories c
		LEFT JOIN categories parent ON parent.id = c.parent_id
		LEFT JOIN movement_allocations a ON a.category_id = c.id
		LEFT JOIN movements m ON m.id = a.movement_id AND m.status = 'posted' AND m.business_date BETWEEN ? AND ?
		WHERE c.kind = 'expense' AND c.archived_at = ''
		GROUP BY c.id ORDER BY total DESC, path
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}
	rows, err := h.store.DB().QueryContext(r.Context(), query, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []map[string]any
	for rows.Next() {
		var id, name, path, color, icon string
		var total int64
		var count, rules int
		var lastUsed sql.NullString
		if err := rows.Scan(&id, &name, &path, &color, &icon, &total, &count, &lastUsed, &rules); err != nil {
			return nil, err
		}
		result = append(result, map[string]any{
			"id": id, "name": name, "path": path, "color": color, "icon": icon,
			"amountCents": total, "movementCount": count, "lastUsed": lastUsed.String,
			"merchantRuleCount": rules, "drilldown": "/movimenti?category=" + url.QueryEscape(id) + "&from=" + from + "&to=" + to,
		})
	}
	return result, rows.Err()
}

func (h *Handler) netWorth(w http.ResponseWriter, r *http.Request) {
	_, to := analyticsRange(r)
	accounts, total := h.accountBalances(r, to)
	markersRows, err := h.store.DB().QueryContext(r.Context(), `
		SELECT account_id, closed_through, actual_balance_cents FROM account_reconciliations
		WHERE closed_through <= ? ORDER BY closed_through, account_id
	`, to)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile caricare le riconciliazioni.", nil)
		return
	}
	defer markersRows.Close()
	var markers []map[string]any
	for markersRows.Next() {
		var accountID, date string
		var amount int64
		_ = markersRows.Scan(&accountID, &date, &amount)
		markers = append(markers, map[string]any{"accountId": accountID, "date": date, "balanceCents": amount, "reconciled": true})
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"asOf": to, "netWorthCents": total, "accounts": accounts, "markers": markers}})
}

func (h *Handler) recurringForecast(w http.ResponseWriter, r *http.Request) {
	_, to := analyticsRange(r)
	rows, err := h.store.DB().QueryContext(r.Context(), `
		SELECT id, next_due, amount_cents, amount_mode, kind, merchant
		FROM recurring_rules WHERE state = 'active' AND next_due <= ? ORDER BY next_due, id
	`, to)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile calcolare il forecast.", nil)
		return
	}
	defer rows.Close()
	var items []map[string]any
	var certain, estimated int64
	for rows.Next() {
		var id, date, mode, kind, merchant string
		var amount int64
		if err := rows.Scan(&id, &date, &amount, &mode, &kind, &merchant); err != nil {
			return
		}
		certainty := "certain"
		if mode == "variable" {
			certainty = "estimated"
			estimated += amount
		} else {
			certain += amount
		}
		items = append(items, map[string]any{"ruleId": id, "date": date, "amountCents": amount, "kind": kind, "merchant": merchant, "certainty": certainty})
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"certainCents": certain, "estimatedCents": estimated, "items": items}})
}

func (h *Handler) accountBalances(r *http.Request, asOf string) ([]map[string]any, int64) {
	rows, err := h.store.DB().QueryContext(r.Context(), `SELECT id, name, type, class FROM accounts WHERE archived_at = '' ORDER BY name`)
	if err != nil {
		return nil, 0
	}
	type accountRow struct{ id, name, accountType, class string }
	var source []accountRow
	for rows.Next() {
		var account accountRow
		if err := rows.Scan(&account.id, &account.name, &account.accountType, &account.class); err == nil {
			source = append(source, account)
		}
	}
	rows.Close()
	var result []map[string]any
	var total int64
	for _, account := range source {
		balance, err := h.ledger.Balance(r.Context(), account.id, asOf)
		if err != nil {
			continue
		}
		total += balance.BalanceCents
		result = append(result, map[string]any{"id": account.id, "name": account.name, "type": account.accountType, "class": account.class, "balance": balance})
	}
	return result, total
}

func analyticsRange(r *http.Request) (string, string) {
	now := time.Now()
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" {
		from = now.AddDate(0, -11, 0).Format("2006-01") + "-01"
	}
	if len(from) == 7 {
		from += "-01"
	}
	if to == "" {
		to = now.Format("2006-01-02")
	}
	if len(to) == 7 {
		to = monthEnd(to)
	}
	return from, to
}

func monthEnd(month string) string {
	parsed, err := time.Parse("2006-01", month)
	if err != nil {
		return month
	}
	return parsed.AddDate(0, 1, -1).Format("2006-01-02")
}
