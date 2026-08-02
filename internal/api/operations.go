package api

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"spese/internal/features/ledger"
)

type merchantRule struct {
	ID         string `json:"id"`
	Merchant   string `json:"merchant"`
	Kind       string `json:"kind"`
	AccountID  string `json:"accountId,omitempty"`
	CategoryID string `json:"categoryId,omitempty"`
	Priority   int    `json:"priority"`
	ArchivedAt string `json:"archivedAt,omitempty"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
	Version    int    `json:"version"`
}

func (h *Handler) listMerchantRules(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.DB().QueryContext(r.Context(), `
		SELECT id, merchant, kind, account_id, category_id, priority, archived_at, created_at, updated_at, version
		FROM merchant_rules WHERE (? = 'all' OR archived_at = '') ORDER BY priority DESC, merchant_normalized
	`, r.URL.Query().Get("status"))
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile caricare le regole.", nil)
		return
	}
	defer rows.Close()
	result := make([]merchantRule, 0)
	for rows.Next() {
		rule, err := scanMerchantRule(rows)
		if err != nil {
			h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile leggere le regole.", nil)
			return
		}
		result = append(result, rule)
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (h *Handler) createMerchantRule(w http.ResponseWriter, r *http.Request) {
	var input merchantRule
	if err := h.decodeJSON(w, r, &input); err != nil || strings.TrimSpace(input.Merchant) == "" {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Regola esercente non valida.", nil)
		return
	}
	id, now := uuid.NewString(), nowUTC()
	_, err := h.store.DB().ExecContext(r.Context(), `
		INSERT INTO merchant_rules (id, merchant, merchant_normalized, kind, account_id, category_id,
			priority, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, id, strings.TrimSpace(input.Merchant), normalizeMerchant(input.Merchant), input.Kind,
		nullable(input.AccountID), nullable(input.CategoryID), input.Priority, now, now)
	if err != nil {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "La regola esiste già o non è valida.", nil)
		return
	}
	rule, _ := h.getMerchantRule(r, id)
	h.writeJSON(w, http.StatusCreated, map[string]any{"data": rule})
}

func (h *Handler) updateMerchantRule(w http.ResponseWriter, r *http.Request) {
	version, err := parseIfMatch(r)
	if err != nil {
		h.writeError(w, r, http.StatusPreconditionRequired, "if_match_required", "If-Match con la versione è obbligatorio.", nil)
		return
	}
	var input merchantRule
	if err := h.decodeJSON(w, r, &input); err != nil || strings.TrimSpace(input.Merchant) == "" {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Regola esercente non valida.", nil)
		return
	}
	result, err := h.store.DB().ExecContext(r.Context(), `
		UPDATE merchant_rules SET merchant = ?, merchant_normalized = ?, kind = ?, account_id = ?,
			category_id = ?, priority = ?, updated_at = ?, version = version + 1
		WHERE id = ? AND version = ?
	`, strings.TrimSpace(input.Merchant), normalizeMerchant(input.Merchant), input.Kind, nullable(input.AccountID),
		nullable(input.CategoryID), input.Priority, nowUTC(), r.PathValue("id"), version)
	if err != nil {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "La regola non può essere aggiornata.", nil)
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		h.writeError(w, r, http.StatusPreconditionFailed, "version_conflict", "La regola è stata modificata o non esiste.", nil)
		return
	}
	rule, _ := h.getMerchantRule(r, r.PathValue("id"))
	h.writeJSON(w, http.StatusOK, map[string]any{"data": rule})
}

func (h *Handler) archiveMerchantRule(w http.ResponseWriter, r *http.Request) {
	version, err := parseIfMatch(r)
	if err != nil {
		h.writeError(w, r, http.StatusPreconditionRequired, "if_match_required", "If-Match con la versione è obbligatorio.", nil)
		return
	}
	result, err := h.store.DB().ExecContext(r.Context(), `
		UPDATE merchant_rules SET archived_at = ?, updated_at = ?, version = version + 1
		WHERE id = ? AND version = ? AND archived_at = ''
	`, nowUTC(), nowUTC(), r.PathValue("id"), version)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile archiviare la regola.", nil)
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		h.writeError(w, r, http.StatusPreconditionFailed, "version_conflict", "La regola è stata modificata o non esiste.", nil)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": r.PathValue("id"), "archived": true}})
}

func (h *Handler) merchantSuggestions(w http.ResponseWriter, r *http.Request) {
	merchant := strings.TrimSpace(r.URL.Query().Get("merchant"))
	if merchant == "" {
		h.writeJSON(w, http.StatusOK, map[string]any{"data": []any{}})
		return
	}
	rows, err := h.store.DB().QueryContext(r.Context(), `
		SELECT m.id, p.account_id, a.category_id, count(*) AS uses, max(m.business_date) AS last_used
		FROM movements m
		JOIN postings p ON p.movement_id = m.id
		LEFT JOIN movement_allocations a ON a.movement_id = m.id
		WHERE m.status = 'posted' AND lower(m.merchant) = lower(?)
		GROUP BY p.account_id, a.category_id ORDER BY uses DESC, last_used DESC LIMIT 5
	`, merchant)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile calcolare i suggerimenti.", nil)
		return
	}
	defer rows.Close()
	result := make([]map[string]any, 0)
	for rows.Next() {
		var movementID, accountID, lastUsed string
		var category sql.NullString
		var uses int
		if err := rows.Scan(&movementID, &accountID, &category, &uses, &lastUsed); err != nil {
			h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile leggere i suggerimenti.", nil)
			return
		}
		result = append(result, map[string]any{
			"accountId": accountID, "categoryId": category.String, "uses": uses,
			"reason": "Usato " + strconvItoa(uses) + " volte, l'ultima il " + lastUsed,
		})
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (h *Handler) getMerchantRule(r *http.Request, id string) (merchantRule, error) {
	row := h.store.DB().QueryRowContext(r.Context(), `
		SELECT id, merchant, kind, account_id, category_id, priority, archived_at, created_at, updated_at, version
		FROM merchant_rules WHERE id = ?
	`, id)
	return scanMerchantRule(row)
}

type rowScanner interface{ Scan(...any) error }

func scanMerchantRule(row rowScanner) (merchantRule, error) {
	var rule merchantRule
	var account, category sql.NullString
	err := row.Scan(&rule.ID, &rule.Merchant, &rule.Kind, &account, &category, &rule.Priority,
		&rule.ArchivedAt, &rule.CreatedAt, &rule.UpdatedAt, &rule.Version)
	rule.AccountID, rule.CategoryID = account.String, category.String
	return rule, err
}

func normalizeMerchant(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func (h *Handler) listRecurringRules(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.DB().QueryContext(r.Context(), `SELECT id FROM recurring_rules WHERE (? = 'all' OR state <> 'archived') ORDER BY next_due, id`, r.URL.Query().Get("status"))
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile caricare le ricorrenze.", nil)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	items := make([]ledger.RecurringRule, 0, len(ids))
	for _, id := range ids {
		rule, err := h.ledger.GetRecurringRule(r.Context(), id)
		if err != nil {
			h.writeDomainError(w, r, err)
			return
		}
		items = append(items, rule)
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) createRecurringRule(w http.ResponseWriter, r *http.Request) {
	var input ledger.RecurringRuleInput
	if err := h.decodeJSON(w, r, &input); err != nil {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Ricorrenza non valida.", nil)
		return
	}
	rule, err := h.ledger.CreateRecurringRule(r.Context(), input)
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, map[string]any{"data": rule})
}

func (h *Handler) getRecurringRule(w http.ResponseWriter, r *http.Request) {
	rule, err := h.ledger.GetRecurringRule(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("ETag", quoteVersion(rule.Version))
	h.writeJSON(w, http.StatusOK, map[string]any{"data": rule})
}

func (h *Handler) updateRecurringRule(w http.ResponseWriter, r *http.Request) {
	version, err := parseIfMatch(r)
	if err != nil {
		h.writeError(w, r, http.StatusPreconditionRequired, "if_match_required", "If-Match con la versione è obbligatorio.", nil)
		return
	}
	var input struct {
		ledger.RecurringRuleInput
		State string `json:"state"`
	}
	if err := h.decodeJSON(w, r, &input); err != nil {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Ricorrenza non valida.", nil)
		return
	}
	if input.State == "" {
		input.State = "active"
	}
	result, err := h.store.DB().ExecContext(r.Context(), `
		UPDATE recurring_rules SET kind = ?, frequency = ?, interval_count = ?, start_date = ?, end_date = ?,
			day_of_month = ?, timezone = ?, amount_cents = ?, amount_mode = ?, account_id = ?, category_id = ?,
			merchant = ?, note = ?, state = ?, mode = ?, updated_at = ?, version = version + 1
		WHERE id = ? AND version = ?
	`, input.Kind, input.Frequency, input.Interval, input.StartDate, nullable(input.EndDate), nullableInt(input.DayOfMonth),
		input.Timezone, input.AmountCents, input.AmountMode, input.AccountID, input.CategoryID, input.Merchant,
		input.Note, input.State, input.Mode, nowUTC(), r.PathValue("id"), version)
	if err != nil {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "La ricorrenza non può essere aggiornata.", nil)
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		h.writeError(w, r, http.StatusPreconditionFailed, "version_conflict", "La ricorrenza è stata modificata o non esiste.", nil)
		return
	}
	rule, _ := h.ledger.GetRecurringRule(r.Context(), r.PathValue("id"))
	h.writeJSON(w, http.StatusOK, map[string]any{"data": rule})
}

func (h *Handler) archiveRecurringRule(w http.ResponseWriter, r *http.Request) {
	version, err := parseIfMatch(r)
	if err != nil {
		h.writeError(w, r, http.StatusPreconditionRequired, "if_match_required", "If-Match con la versione è obbligatorio.", nil)
		return
	}
	result, err := h.store.DB().ExecContext(r.Context(), `UPDATE recurring_rules SET state = 'archived', updated_at = ?, version = version + 1 WHERE id = ? AND version = ?`, nowUTC(), r.PathValue("id"), version)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile archiviare la ricorrenza.", nil)
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		h.writeError(w, r, http.StatusPreconditionFailed, "version_conflict", "La ricorrenza è stata modificata o non esiste.", nil)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": r.PathValue("id"), "state": "archived"}})
}

func (h *Handler) listOccurrences(w http.ResponseWriter, r *http.Request) {
	items, err := h.ledger.ListOccurrences(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) actOnOccurrence(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		occurrence, err := h.ledger.ActOnOccurrence(r.Context(), r.PathValue("id"), action)
		if err != nil {
			h.writeDomainError(w, r, err)
			return
		}
		h.writeJSON(w, http.StatusOK, map[string]any{"data": occurrence})
	}
}

func nullableInt(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func strconvItoa(value int) string {
	return strconv.Itoa(value)
}
