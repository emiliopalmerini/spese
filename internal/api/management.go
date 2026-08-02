package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"spese/internal/features/ledger"
)

func (h *Handler) updateAccount(w http.ResponseWriter, r *http.Request) {
	version, err := parseIfMatch(r)
	if err != nil {
		h.writeError(w, r, http.StatusPreconditionRequired, "if_match_required", "If-Match con la versione è obbligatorio.", nil)
		return
	}
	var input ledger.AccountInput
	if err := h.decodeJSON(w, r, &input); err != nil || strings.TrimSpace(input.Name) == "" {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Dati del conto non validi.", nil)
		return
	}
	result, err := h.store.DB().ExecContext(r.Context(), `
		UPDATE accounts SET name = ?, type = ?, class = ?, initial_balance_cents = ?, initial_date = ?,
			active_from = ?, active_to = ?, note = ?, updated_at = ?, version = version + 1
		WHERE id = ? AND version = ?
	`, strings.TrimSpace(input.Name), input.Type, input.Class, input.InitialBalanceCents, input.InitialDate,
		input.ActiveFrom, input.ActiveTo, strings.TrimSpace(input.Note), nowUTC(), r.PathValue("id"), version)
	if err != nil {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Il conto non può essere aggiornato.", nil)
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		h.writeError(w, r, http.StatusPreconditionFailed, "version_conflict", "Il conto è stato modificato o non esiste.", nil)
		return
	}
	account, err := h.ledger.GetAccount(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("ETag", quoteVersion(account.Version))
	h.writeJSON(w, http.StatusOK, map[string]any{"data": account})
}

func (h *Handler) archiveAccount(w http.ResponseWriter, r *http.Request) {
	version, err := parseIfMatch(r)
	if err != nil {
		h.writeError(w, r, http.StatusPreconditionRequired, "if_match_required", "If-Match con la versione è obbligatorio.", nil)
		return
	}
	result, err := h.store.DB().ExecContext(r.Context(), `
		UPDATE accounts SET archived_at = ?, updated_at = ?, version = version + 1
		WHERE id = ? AND version = ? AND archived_at = ''
	`, nowUTC(), nowUTC(), r.PathValue("id"), version)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile archiviare il conto.", nil)
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		h.writeError(w, r, http.StatusPreconditionFailed, "version_conflict", "Il conto è stato modificato o non esiste.", nil)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": r.PathValue("id"), "archived": true}})
}

func (h *Handler) updateCategory(w http.ResponseWriter, r *http.Request) {
	version, err := parseIfMatch(r)
	if err != nil {
		h.writeError(w, r, http.StatusPreconditionRequired, "if_match_required", "If-Match con la versione è obbligatorio.", nil)
		return
	}
	var input ledger.CategoryInput
	if err := h.decodeJSON(w, r, &input); err != nil || strings.TrimSpace(input.Name) == "" {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Dati della categoria non validi.", nil)
		return
	}
	result, err := h.store.DB().ExecContext(r.Context(), `
		UPDATE categories SET parent_id = ?, kind = ?, name = ?, icon = ?, color = ?, sort_order = ?,
			updated_at = ?, version = version + 1 WHERE id = ? AND version = ?
	`, nullable(input.ParentID), input.Kind, strings.TrimSpace(input.Name), input.Icon, input.Color,
		input.SortOrder, nowUTC(), r.PathValue("id"), version)
	if err != nil {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "La categoria non può essere aggiornata.", nil)
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		h.writeError(w, r, http.StatusPreconditionFailed, "version_conflict", "La categoria è stata modificata o non esiste.", nil)
		return
	}
	category, err := h.ledger.GetCategory(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("ETag", quoteVersion(category.Version))
	h.writeJSON(w, http.StatusOK, map[string]any{"data": category})
}

func (h *Handler) archiveCategory(w http.ResponseWriter, r *http.Request) {
	version, err := parseIfMatch(r)
	if err != nil {
		h.writeError(w, r, http.StatusPreconditionRequired, "if_match_required", "If-Match con la versione è obbligatorio.", nil)
		return
	}
	result, err := h.store.DB().ExecContext(r.Context(), `
		UPDATE categories SET archived_at = ?, updated_at = ?, version = version + 1
		WHERE id = ? AND version = ? AND archived_at = ''
	`, nowUTC(), nowUTC(), r.PathValue("id"), version)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile archiviare la categoria.", nil)
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		h.writeError(w, r, http.StatusPreconditionFailed, "version_conflict", "La categoria è stata modificata o non esiste.", nil)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": r.PathValue("id"), "archived": true}})
}

func (h *Handler) reparentCategory(w http.ResponseWriter, r *http.Request) {
	version, err := parseIfMatch(r)
	if err != nil {
		h.writeError(w, r, http.StatusPreconditionRequired, "if_match_required", "If-Match con la versione è obbligatorio.", nil)
		return
	}
	var input struct {
		ParentID string `json:"parentId"`
	}
	if err := h.decodeJSON(w, r, &input); err != nil {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Nuovo parent non valido.", nil)
		return
	}
	result, err := h.store.DB().ExecContext(r.Context(), `
		UPDATE categories SET parent_id = ?, updated_at = ?, version = version + 1 WHERE id = ? AND version = ?
	`, nullable(input.ParentID), nowUTC(), r.PathValue("id"), version)
	if err != nil {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Reparenting non consentito.", nil)
		return
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		h.writeError(w, r, http.StatusPreconditionFailed, "version_conflict", "La categoria è stata modificata o non esiste.", nil)
		return
	}
	category, _ := h.ledger.GetCategory(r.Context(), r.PathValue("id"))
	h.writeJSON(w, http.StatusOK, map[string]any{"data": category})
}

func (h *Handler) mergeCategory(w http.ResponseWriter, r *http.Request) {
	version, err := parseIfMatch(r)
	if err != nil {
		h.writeError(w, r, http.StatusPreconditionRequired, "if_match_required", "If-Match con la versione è obbligatorio.", nil)
		return
	}
	var input struct {
		TargetID string `json:"targetId"`
	}
	if err := h.decodeJSON(w, r, &input); err != nil || input.TargetID == "" || input.TargetID == r.PathValue("id") {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Categoria destinazione non valida.", nil)
		return
	}
	tx, err := h.store.DB().BeginTx(r.Context(), nil)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile iniziare il merge.", nil)
		return
	}
	defer tx.Rollback()
	var sourceKind, targetKind string
	if err := tx.QueryRowContext(r.Context(), "SELECT kind FROM categories WHERE id = ? AND version = ? AND archived_at = ''", r.PathValue("id"), version).Scan(&sourceKind); err != nil {
		h.writeError(w, r, http.StatusPreconditionFailed, "version_conflict", "Categoria sorgente modificata o assente.", nil)
		return
	}
	if err := tx.QueryRowContext(r.Context(), "SELECT kind FROM categories WHERE id = ? AND archived_at = ''", input.TargetID).Scan(&targetKind); err != nil || sourceKind != targetKind {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Le categorie devono avere lo stesso tipo.", nil)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `
		UPDATE movement_allocations AS target SET amount_cents = amount_cents + (
			SELECT source.amount_cents FROM movement_allocations source
			WHERE source.movement_id = target.movement_id AND source.category_id = ?
		) WHERE target.category_id = ? AND EXISTS (
			SELECT 1 FROM movement_allocations source WHERE source.movement_id = target.movement_id AND source.category_id = ?
		)
	`, r.PathValue("id"), input.TargetID, r.PathValue("id")); err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile unire le allocazioni.", nil)
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM movement_allocations WHERE category_id = ? AND movement_id IN (SELECT movement_id FROM movement_allocations WHERE category_id = ?)`, r.PathValue("id"), input.TargetID); err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile unire le allocazioni.", nil)
		return
	}
	for _, statement := range []string{
		"UPDATE movement_allocations SET category_id = ? WHERE category_id = ?",
		"UPDATE merchant_rules SET category_id = ? WHERE category_id = ?",
		"UPDATE recurring_rules SET category_id = ? WHERE category_id = ?",
	} {
		if _, err := tx.ExecContext(r.Context(), statement, input.TargetID, r.PathValue("id")); err != nil {
			h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile completare il merge.", nil)
			return
		}
	}
	if _, err := tx.ExecContext(r.Context(), `UPDATE categories SET archived_at = ?, updated_at = ?, version = version + 1 WHERE id = ?`, nowUTC(), nowUTC(), r.PathValue("id")); err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile archiviare la categoria sorgente.", nil)
		return
	}
	if err := tx.Commit(); err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile confermare il merge.", nil)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"sourceId": r.PathValue("id"), "targetId": input.TargetID}})
}

func (h *Handler) bulkReclassify(w http.ResponseWriter, r *http.Request) {
	var input struct {
		MovementIDs []string `json:"movementIds"`
		CategoryID  string   `json:"categoryId"`
	}
	if err := h.decodeJSON(w, r, &input); err != nil || len(input.MovementIDs) == 0 || len(input.MovementIDs) > 500 || input.CategoryID == "" {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Riclassificazione non valida.", nil)
		return
	}
	tx, err := h.store.DB().BeginTx(r.Context(), nil)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile iniziare la riclassificazione.", nil)
		return
	}
	defer tx.Rollback()
	for _, movementID := range input.MovementIDs {
		var amount int64
		var kind string
		if err := tx.QueryRowContext(r.Context(), `SELECT amount_cents, kind FROM movements WHERE id = ? AND kind <> 'transfer'`, movementID).Scan(&amount, &kind); err != nil {
			h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Un movimento non è riclassificabile.", nil)
			return
		}
		if _, err := tx.ExecContext(r.Context(), "DELETE FROM movement_allocations WHERE movement_id = ?", movementID); err != nil {
			h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile riclassificare i movimenti.", nil)
			return
		}
		if _, err := tx.ExecContext(r.Context(), `INSERT INTO movement_allocations (id, movement_id, category_id, amount_cents, created_at) VALUES (?, ?, ?, ?, ?)`, uuid.NewString(), movementID, input.CategoryID, amount, nowUTC()); err != nil {
			h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Categoria non compatibile.", nil)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile confermare la riclassificazione.", nil)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"updated": len(input.MovementIDs)}})
}

func (h *Handler) previewReconciliation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Period   string                       `json:"period"`
		Accounts []ledger.ReconciliationInput `json:"accounts"`
	}
	if err := h.decodeJSON(w, r, &input); err != nil {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Dati di riconciliazione non validi.", nil)
		return
	}
	preview, err := h.ledger.PreviewReconciliation(r.Context(), input.Period, input.Accounts)
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": preview})
}

func (h *Handler) commitReconciliation(w http.ResponseWriter, r *http.Request) {
	var preview ledger.ReconciliationPreview
	if err := h.decodeJSON(w, r, &preview); err != nil {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Anteprima di riconciliazione non valida.", nil)
		return
	}
	committed, err := h.ledger.CommitReconciliation(r.Context(), preview)
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, map[string]any{"data": committed})
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
