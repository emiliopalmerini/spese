// Package api exposes the versioned JSON surface used by the React SPA.
package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"spese/internal/features/ledger"
	"spese/internal/storage"
)

const maxJSONBody = 1 << 20

//go:embed openapi.yaml
var openAPIDocument []byte

type Handler struct {
	store       *storage.Store
	ledger      *ledger.Service
	mux         *http.ServeMux
	idempotency sync.Mutex
}

type requestIDKey struct{}

type errorEnvelope struct {
	Code        string            `json:"code"`
	Message     string            `json:"message"`
	FieldErrors map[string]string `json:"fieldErrors,omitempty"`
	RequestID   string            `json:"requestId"`
}

func New(store *storage.Store, service *ledger.Service) http.Handler {
	h := &Handler{store: store, ledger: service, mux: http.NewServeMux()}
	h.mux.HandleFunc("GET /api/v1/movements", h.listMovements)
	h.mux.HandleFunc("POST /api/v1/movements", h.createMovement)
	h.mux.HandleFunc("GET /api/v1/movements/{id}", h.getMovement)
	h.mux.HandleFunc("PATCH /api/v1/movements/{id}", h.updateMovement)
	h.mux.HandleFunc("DELETE /api/v1/movements/{id}", h.voidMovement)
	h.mux.HandleFunc("GET /api/v1/accounts", h.listAccounts)
	h.mux.HandleFunc("POST /api/v1/accounts", h.createAccount)
	h.mux.HandleFunc("GET /api/v1/accounts/{id}", h.getAccount)
	h.mux.HandleFunc("PATCH /api/v1/accounts/{id}", h.updateAccount)
	h.mux.HandleFunc("DELETE /api/v1/accounts/{id}", h.archiveAccount)
	h.mux.HandleFunc("GET /api/v1/accounts/{id}/balance", h.accountBalance)
	h.mux.HandleFunc("GET /api/v1/categories", h.listCategories)
	h.mux.HandleFunc("POST /api/v1/categories", h.createCategory)
	h.mux.HandleFunc("GET /api/v1/categories/{id}", h.getCategory)
	h.mux.HandleFunc("PATCH /api/v1/categories/{id}", h.updateCategory)
	h.mux.HandleFunc("DELETE /api/v1/categories/{id}", h.archiveCategory)
	h.mux.HandleFunc("POST /api/v1/categories/{id}/reparent", h.reparentCategory)
	h.mux.HandleFunc("POST /api/v1/categories/{id}/merge", h.mergeCategory)
	h.mux.HandleFunc("POST /api/v1/categories/bulk-reclassify", h.bulkReclassify)
	h.mux.HandleFunc("POST /api/v1/reconciliations/preview", h.previewReconciliation)
	h.mux.HandleFunc("POST /api/v1/reconciliations", h.commitReconciliation)
	h.mux.HandleFunc("GET /api/v1/merchant-rules", h.listMerchantRules)
	h.mux.HandleFunc("POST /api/v1/merchant-rules", h.createMerchantRule)
	h.mux.HandleFunc("PATCH /api/v1/merchant-rules/{id}", h.updateMerchantRule)
	h.mux.HandleFunc("DELETE /api/v1/merchant-rules/{id}", h.archiveMerchantRule)
	h.mux.HandleFunc("GET /api/v1/suggestions/merchant", h.merchantSuggestions)
	h.mux.HandleFunc("GET /api/v1/recurring-rules", h.listRecurringRules)
	h.mux.HandleFunc("POST /api/v1/recurring-rules", h.createRecurringRule)
	h.mux.HandleFunc("GET /api/v1/recurring-rules/{id}", h.getRecurringRule)
	h.mux.HandleFunc("PATCH /api/v1/recurring-rules/{id}", h.updateRecurringRule)
	h.mux.HandleFunc("DELETE /api/v1/recurring-rules/{id}", h.archiveRecurringRule)
	h.mux.HandleFunc("GET /api/v1/recurring-rules/{id}/occurrences", h.listOccurrences)
	h.mux.HandleFunc("POST /api/v1/occurrences/{id}/confirm", h.actOnOccurrence("confirm"))
	h.mux.HandleFunc("POST /api/v1/occurrences/{id}/skip", h.actOnOccurrence("skip"))
	h.mux.HandleFunc("POST /api/v1/occurrences/{id}/post", h.actOnOccurrence("post"))
	h.mux.HandleFunc("GET /api/v1/analytics/overview", h.overview)
	h.mux.HandleFunc("GET /api/v1/analytics/cash-flow", h.cashFlow)
	h.mux.HandleFunc("GET /api/v1/analytics/categories", h.categoryAnalysis)
	h.mux.HandleFunc("GET /api/v1/analytics/net-worth", h.netWorth)
	h.mux.HandleFunc("GET /api/v1/analytics/recurring-forecast", h.recurringForecast)
	h.mux.HandleFunc("GET /api/v1/openapi.yaml", h.openAPI)
	h.mux.HandleFunc("/api/", h.notFound)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if requestID == "" || len(requestID) > 100 {
		requestID = uuid.NewString()
	}
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	w.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(self)")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("Cache-Control", "no-store")
	ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
	r = r.WithContext(ctx)
	if isMutation(r.Method) && !sameOriginMutation(r) {
		h.writeError(w, r, http.StatusForbidden, "origin_forbidden", "Origine della richiesta non consentita.", nil)
		return
	}
	if requiresGenericIdempotency(r) {
		h.serveIdempotent(w, r)
		return
	}
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) notFound(w http.ResponseWriter, r *http.Request) {
	h.writeError(w, r, http.StatusNotFound, "not_found", "Endpoint API non trovato.", nil)
}

func sameOriginMutation(r *http.Request) bool {
	if r.Header.Get("X-Spese-CSRF") != "1" {
		return false
	}
	origin, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && origin.Host != "" && strings.EqualFold(origin.Host, r.Host) &&
		(origin.Scheme == "http" || origin.Scheme == "https")
}

func isMutation(method string) bool {
	return method == http.MethodPost || method == http.MethodPatch || method == http.MethodPut || method == http.MethodDelete
}

func requiresGenericIdempotency(r *http.Request) bool {
	if r.Method != http.MethodPost || r.URL.Path == "/api/v1/movements" || strings.HasSuffix(r.URL.Path, "/preview") || strings.Contains(r.URL.Path, "/occurrences/") {
		return false
	}
	switch r.URL.Path {
	case "/api/v1/accounts", "/api/v1/categories", "/api/v1/merchant-rules", "/api/v1/reconciliations", "/api/v1/recurring-rules":
		return true
	default:
		return false
	}
}

func (h *Handler) serveIdempotent(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || len(key) > 200 {
		h.writeError(w, r, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key è obbligatorio.", nil)
		return
	}
	body, err := readBody(w, r)
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "invalid_body", "Corpo della richiesta non valido.", nil)
		return
	}
	hash := sha256.Sum256(body)
	hashString := hex.EncodeToString(hash[:])
	h.idempotency.Lock()
	defer h.idempotency.Unlock()
	var storedHash, response string
	var status int
	err = h.store.DB().QueryRowContext(r.Context(), `
		SELECT request_hash, status_code, response_json FROM api_idempotency
		WHERE idempotency_key = ? AND method = ? AND path = ?
	`, key, r.Method, r.URL.Path).Scan(&storedHash, &status, &response)
	if err == nil {
		if storedHash != hashString {
			h.writeError(w, r, http.StatusConflict, "idempotency_conflict", "La chiave è già associata a un'altra richiesta.", nil)
			return
		}
		writeRawJSON(w, status, []byte(response))
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile verificare la richiesta.", nil)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	recorder := &captureWriter{header: make(http.Header), status: http.StatusOK}
	h.mux.ServeHTTP(recorder, r)
	if recorder.status < http.StatusInternalServerError {
		_, err = h.store.DB().ExecContext(r.Context(), `
			INSERT INTO api_idempotency (idempotency_key, method, path, request_hash, status_code, response_json, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, key, r.Method, r.URL.Path, hashString, recorder.status, recorder.body.String(), time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Risposta idempotente non registrata.", nil)
			return
		}
	}
	for key, values := range recorder.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(recorder.status)
	_, _ = w.Write(recorder.body.Bytes())
}

type captureWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *captureWriter) Header() http.Header             { return w.header }
func (w *captureWriter) WriteHeader(status int)          { w.status = status }
func (w *captureWriter) Write(value []byte) (int, error) { return w.body.Write(value) }

func (h *Handler) createMovement(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || len(key) > 200 {
		h.writeError(w, r, http.StatusBadRequest, "idempotency_key_required", "Idempotency-Key è obbligatorio.", nil)
		return
	}
	body, err := readBody(w, r)
	if err != nil {
		h.writeError(w, r, http.StatusBadRequest, "invalid_body", "Corpo della richiesta non valido.", nil)
		return
	}
	hash := sha256.Sum256(body)
	hashString := hex.EncodeToString(hash[:])
	h.idempotency.Lock()
	defer h.idempotency.Unlock()
	var storedHash, response string
	var status int
	err = h.store.DB().QueryRowContext(r.Context(), `
		SELECT request_hash, status_code, response_json FROM api_idempotency
		WHERE idempotency_key = ? AND method = ? AND path = ?
	`, key, r.Method, r.URL.Path).Scan(&storedHash, &status, &response)
	if err == nil {
		if storedHash != hashString {
			h.writeError(w, r, http.StatusConflict, "idempotency_conflict", "La chiave è già associata a un'altra richiesta.", nil)
			return
		}
		writeRawJSON(w, status, []byte(response))
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile verificare la richiesta.", nil)
		return
	}
	var input ledger.MovementInput
	if err := decodeBytes(body, &input); err != nil {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Dati del movimento non validi.", map[string]string{"body": err.Error()})
		return
	}
	movement, err := h.ledger.CreateMovement(r.Context(), input)
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	payload := h.movementMutationResponse(r.Context(), movement)
	encoded, err := json.Marshal(payload)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "encoding_error", "Impossibile preparare la risposta.", nil)
		return
	}
	_, err = h.store.DB().ExecContext(r.Context(), `
		INSERT INTO api_idempotency (idempotency_key, method, path, request_hash, status_code, response_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, key, r.Method, r.URL.Path, hashString, http.StatusCreated, string(encoded), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Movimento salvato, ma risposta idempotente non registrata.", nil)
		return
	}
	w.Header().Set("Location", "/api/v1/movements/"+movement.ID)
	w.Header().Set("ETag", quoteVersion(movement.Version))
	writeRawJSON(w, http.StatusCreated, encoded)
}

func (h *Handler) getMovement(w http.ResponseWriter, r *http.Request) {
	movement, err := h.ledger.GetMovement(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("ETag", quoteVersion(movement.Version))
	h.writeJSON(w, http.StatusOK, map[string]any{"data": movement})
}

func (h *Handler) updateMovement(w http.ResponseWriter, r *http.Request) {
	version, err := parseIfMatch(r)
	if err != nil {
		h.writeError(w, r, http.StatusPreconditionRequired, "if_match_required", "If-Match con la versione è obbligatorio.", nil)
		return
	}
	var input ledger.MovementInput
	if err := h.decodeJSON(w, r, &input); err != nil {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Dati del movimento non validi.", map[string]string{"body": err.Error()})
		return
	}
	movement, err := h.ledger.UpdateMovement(r.Context(), r.PathValue("id"), version, input)
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("ETag", quoteVersion(movement.Version))
	h.writeJSON(w, http.StatusOK, h.movementMutationResponse(r.Context(), movement))
}

func (h *Handler) voidMovement(w http.ResponseWriter, r *http.Request) {
	version, err := parseIfMatch(r)
	if err != nil {
		h.writeError(w, r, http.StatusPreconditionRequired, "if_match_required", "If-Match con la versione è obbligatorio.", nil)
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if r.ContentLength != 0 {
		if err := h.decodeJSON(w, r, &input); err != nil {
			h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Motivo non valido.", nil)
			return
		}
	}
	movement, err := h.ledger.VoidMovement(r.Context(), r.PathValue("id"), version, input.Reason)
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("ETag", quoteVersion(movement.Version))
	h.writeJSON(w, http.StatusOK, h.movementMutationResponse(r.Context(), movement))
}

func (h *Handler) listMovements(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if value, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && value > 0 && value <= 100 {
		limit = value
	}
	query := `
		SELECT id FROM movements
		WHERE (? = '' OR business_date >= ?) AND (? = '' OR business_date <= ?)
			AND (? = '' OR kind = ?) AND (? = '' OR status = ?) AND (? = '' OR origin = ?)
			AND (? = '' OR lower(merchant) LIKE '%' || lower(?) || '%' OR lower(description) LIKE '%' || lower(?) || '%' OR lower(note) LIKE '%' || lower(?) || '%')
			AND (? = '' OR EXISTS (SELECT 1 FROM postings p WHERE p.movement_id = movements.id AND p.account_id = ?))
			AND (? = '' OR EXISTS (
				SELECT 1 FROM movement_allocations a JOIN categories c ON c.id = a.category_id
				WHERE a.movement_id = movements.id AND (c.id = ? OR c.parent_id = ?)
			))
			AND (? = '' OR EXISTS (
				SELECT 1 FROM movement_allocations a JOIN categories c ON c.id = a.category_id
				WHERE a.movement_id = movements.id AND c.id = ? AND c.parent_id IS NOT NULL
			))
			AND (? <> 'uncategorized' OR NOT EXISTS (
				SELECT 1 FROM movement_allocations a JOIN categories c ON c.id = a.category_id
				WHERE a.movement_id = movements.id AND c.parent_id IS NOT NULL
			))
			AND (? = '' OR (business_date || ':' || id) < ?)
		ORDER BY business_date DESC, id DESC LIMIT ?
	`
	params := r.URL.Query()
	from, to, kind, status, origin, search, cursor := params.Get("from"), params.Get("to"), params.Get("kind"), params.Get("status"), params.Get("origin"), params.Get("q"), params.Get("cursor")
	account, category, subcategory, queue := params.Get("account"), params.Get("category"), params.Get("subcategory"), params.Get("queue")
	rows, err := h.store.DB().QueryContext(r.Context(), query,
		from, from, to, to, kind, kind, status, status, origin, origin, search, search, search, search,
		account, account, category, category, category, subcategory, subcategory, queue, cursor, cursor, limit+1)
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile caricare i movimenti.", nil)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile leggere i movimenti.", nil)
			return
		}
		ids = append(ids, id)
	}
	rows.Close()
	nextCursor := ""
	if len(ids) > limit {
		last, _ := h.ledger.GetMovement(r.Context(), ids[limit-1])
		nextCursor = last.Date + ":" + last.ID
		ids = ids[:limit]
	}
	items := make([]ledger.Movement, 0, len(ids))
	for _, id := range ids {
		movement, err := h.ledger.GetMovement(r.Context(), id)
		if err != nil {
			h.writeDomainError(w, r, err)
			return
		}
		items = append(items, movement)
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": items, "nextCursor": nextCursor})
}

func (h *Handler) movementMutationResponse(ctx context.Context, movement ledger.Movement) map[string]any {
	balances := make([]ledger.AccountBalance, 0, len(movement.Postings))
	asOf := time.Now().Format("2006-01-02")
	for _, posting := range movement.Postings {
		balance, err := h.ledger.Balance(ctx, posting.AccountID, asOf)
		if err == nil {
			balances = append(balances, balance)
		}
	}
	return map[string]any{"data": movement, "balances": balances}
}

func (h *Handler) listAccounts(w http.ResponseWriter, r *http.Request) {
	rows, err := h.store.DB().QueryContext(r.Context(), `
		SELECT id, name, type, class, currency, initial_balance_cents, initial_date, active_from,
			active_to, note, archived_at, created_at, updated_at, version
		FROM accounts WHERE (? = 'all' OR archived_at = '') ORDER BY lower(name)
	`, r.URL.Query().Get("status"))
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile caricare i conti.", nil)
		return
	}
	defer rows.Close()
	items := make([]ledger.Account, 0)
	for rows.Next() {
		var account ledger.Account
		if err := rows.Scan(&account.ID, &account.Name, &account.Type, &account.Class, &account.Currency,
			&account.InitialBalanceCents, &account.InitialDate, &account.ActiveFrom, &account.ActiveTo,
			&account.Note, &account.ArchivedAt, &account.CreatedAt, &account.UpdatedAt, &account.Version); err != nil {
			h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile leggere i conti.", nil)
			return
		}
		items = append(items, account)
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) createAccount(w http.ResponseWriter, r *http.Request) {
	var input ledger.AccountInput
	if err := h.decodeJSON(w, r, &input); err != nil {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Dati del conto non validi.", map[string]string{"body": err.Error()})
		return
	}
	account, err := h.ledger.CreateAccount(r.Context(), input)
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/accounts/"+account.ID)
	w.Header().Set("ETag", quoteVersion(account.Version))
	h.writeJSON(w, http.StatusCreated, map[string]any{"data": account})
}

func (h *Handler) getAccount(w http.ResponseWriter, r *http.Request) {
	account, err := h.ledger.GetAccount(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("ETag", quoteVersion(account.Version))
	h.writeJSON(w, http.StatusOK, map[string]any{"data": account})
}

func (h *Handler) accountBalance(w http.ResponseWriter, r *http.Request) {
	asOf := r.URL.Query().Get("asOf")
	if asOf == "" {
		asOf = time.Now().Format("2006-01-02")
	}
	balance, err := h.ledger.Balance(r.Context(), r.PathValue("id"), asOf)
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": balance})
}

func (h *Handler) listCategories(w http.ResponseWriter, r *http.Request) {
	kind := r.URL.Query().Get("kind")
	rows, err := h.store.DB().QueryContext(r.Context(), `
		SELECT id, parent_id, kind, name, icon, color, sort_order, archived_at, created_at, updated_at, version
		FROM categories WHERE (? = '' OR kind = ?) AND (? = 'all' OR archived_at = '')
		ORDER BY kind, parent_id IS NOT NULL, sort_order, lower(name)
	`, kind, kind, r.URL.Query().Get("status"))
	if err != nil {
		h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile caricare le categorie.", nil)
		return
	}
	defer rows.Close()
	items := make([]ledger.Category, 0)
	for rows.Next() {
		var category ledger.Category
		var parent sql.NullString
		if err := rows.Scan(&category.ID, &parent, &category.Kind, &category.Name, &category.Icon, &category.Color,
			&category.SortOrder, &category.ArchivedAt, &category.CreatedAt, &category.UpdatedAt, &category.Version); err != nil {
			h.writeError(w, r, http.StatusInternalServerError, "storage_error", "Impossibile leggere le categorie.", nil)
			return
		}
		category.ParentID = parent.String
		items = append(items, category)
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) createCategory(w http.ResponseWriter, r *http.Request) {
	var input ledger.CategoryInput
	if err := h.decodeJSON(w, r, &input); err != nil {
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", "Dati della categoria non validi.", map[string]string{"body": err.Error()})
		return
	}
	category, err := h.ledger.CreateCategory(r.Context(), input)
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("Location", "/api/v1/categories/"+category.ID)
	w.Header().Set("ETag", quoteVersion(category.Version))
	h.writeJSON(w, http.StatusCreated, map[string]any{"data": category})
}

func (h *Handler) getCategory(w http.ResponseWriter, r *http.Request) {
	category, err := h.ledger.GetCategory(r.Context(), r.PathValue("id"))
	if err != nil {
		h.writeDomainError(w, r, err)
		return
	}
	w.Header().Set("ETag", quoteVersion(category.Version))
	h.writeJSON(w, http.StatusOK, map[string]any{"data": category})
}

func (h *Handler) openAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write(openAPIDocument)
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBody)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("empty body")
	}
	return body, nil
}

func decodeBytes(body []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("multiple JSON values")
	}
	return nil
}

func (h *Handler) decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	if contentType := r.Header.Get("Content-Type"); !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		return errors.New("Content-Type must be application/json")
	}
	body, err := readBody(w, r)
	if err != nil {
		return err
	}
	return decodeBytes(body, destination)
}

func parseIfMatch(r *http.Request) (int, error) {
	value := strings.Trim(strings.TrimSpace(r.Header.Get("If-Match")), `"`)
	version, err := strconv.Atoi(value)
	if err != nil || version < 1 {
		return 0, errors.New("invalid If-Match")
	}
	return version, nil
}

func quoteVersion(version int) string {
	return `"` + strconv.Itoa(version) + `"`
}

func (h *Handler) writeDomainError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ledger.ErrNotFound):
		h.writeError(w, r, http.StatusNotFound, "not_found", "Risorsa non trovata.", nil)
	case errors.Is(err, ledger.ErrVersionConflict):
		h.writeError(w, r, http.StatusPreconditionFailed, "version_conflict", "La risorsa è stata modificata. Ricarica e riprova.", nil)
	case errors.Is(err, ledger.ErrValidation), errors.Is(err, ledger.ErrAllocationMismatch), errors.Is(err, ledger.ErrTransferAllocation):
		h.writeError(w, r, http.StatusUnprocessableEntity, "validation_error", err.Error(), nil)
	default:
		h.writeError(w, r, http.StatusInternalServerError, "internal_error", "Si è verificato un errore interno.", nil)
	}
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, status int, code, message string, fields map[string]string) {
	h.writeJSON(w, status, errorEnvelope{Code: code, Message: message, FieldErrors: fields, RequestID: requestID(r.Context())})
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeRawJSON(w http.ResponseWriter, status int, payload []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(payload)
}

func requestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey{}).(string)
	return value
}
