package dictation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"

	ledgerpkg "spese/internal/features/ledger"
	"spese/internal/storage"
)

const (
	maxAudioUpload = 10 << 20
	maxFormBody    = 256 << 10
	maxCaptureTime = 2 * time.Minute
)

// Handler owns realtime capture, batch fallback, and confirmed persistence.
type Handler struct {
	store       *storage.Store
	ledger      *ledgerpkg.Service
	extractor   extractor
	transcriber transcriber
	logger      *slog.Logger
	now         func() time.Time
}

func NewHandler(store *storage.Store, extractor *OpenCodeClient, transcriber *ElevenLabsTranscriber, logger *slog.Logger) *Handler {
	return &Handler{store: store, ledger: ledgerpkg.New(store), extractor: extractor, transcriber: transcriber, logger: logger, now: time.Now}
}

func (h *Handler) Mount(mux *http.ServeMux, prefix string) {
	prefix = strings.TrimRight(prefix, "/")
	mux.Handle("GET "+prefix+"/realtime", websocket.Server{
		Handshake: sameOriginHandshake,
		Handler:   websocket.Handler(h.realtime),
	})
	mux.HandleFunc("POST "+prefix+"/fallback", h.fallback)
	mux.HandleFunc("POST "+prefix+"/confirm", h.confirm)
}

func (h *Handler) realtime(ws *websocket.Conn) {
	ctx, cancel := context.WithTimeout(ws.Request().Context(), maxCaptureTime)
	defer cancel()
	captureContext, err := h.captureContext(ctx)
	if err != nil {
		h.sendSocketError(ws, "Impossibile caricare il contesto contabile.")
		return
	}
	capture, err := StartCapture(ctx, h.extractor, captureContext)
	if err != nil {
		h.sendSocketError(ws, "Il modello locale non è disponibile.")
		return
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if err := capture.Close(cleanupCtx); err != nil {
			h.logger.Warn("delete dictation session", "err", err)
		}
	}()
	realtime, err := h.transcriber.Connect(ctx)
	if err != nil {
		h.sendSocketError(ws, "La trascrizione non è disponibile.")
		return
	}
	defer realtime.Close()

	writer := &socketWriter{conn: ws}
	if err := writer.send(socketMessage{Type: "ready"}); err != nil {
		return
	}
	errCh := make(chan error, 1)
	var stopping atomic.Bool
	go h.receiveAudio(ctx, cancel, ws, realtime, &stopping, errCh)

	for {
		event, err := realtime.Receive()
		if err != nil {
			select {
			case clientErr := <-errCh:
				if !errors.Is(clientErr, io.EOF) {
					h.logger.Debug("dictation browser stream ended", "err", clientErr)
				}
			default:
				if ctx.Err() == nil {
					h.logger.Warn("receive realtime transcript", "err", err)
				}
			}
			return
		}
		if event.Error != "" {
			_ = writer.send(socketMessage{Type: "error", Message: "Errore durante la trascrizione."})
			return
		}
		switch event.Type {
		case "partial_transcript":
			if err := writer.send(socketMessage{Type: "partial", Text: event.Text}); err != nil {
				return
			}
		case "committed_transcript":
			state, err := capture.Apply(ctx, event.Text)
			if err != nil {
				h.logger.Warn("extract dictated movements", "err", err)
				_ = writer.send(socketMessage{Type: "error", Message: "Non riesco a interpretare questo passaggio.", Recoverable: true})
				continue
			}
			if err := writer.send(socketMessage{Type: "drafts", Text: event.Text, Extraction: &state}); err != nil {
				return
			}
			if stopping.Load() || state.FinishRequested {
				_ = writer.send(socketMessage{Type: "stopped"})
				return
			}
		}
	}
}

func (h *Handler) receiveAudio(ctx context.Context, cancel context.CancelFunc, ws *websocket.Conn, realtime realtimeTranscript, stopping *atomic.Bool, errCh chan<- error) {
	defer cancel()
	for {
		var frame []byte
		if err := websocket.Message.Receive(ws, &frame); err != nil {
			errCh <- err
			return
		}
		if len(frame) == 0 {
			continue
		}
		switch frame[0] {
		case 0:
			if len(frame) > 1 {
				if err := realtime.SendAudio(frame[1:], false); err != nil {
					errCh <- err
					return
				}
			}
		case 1:
			stopping.Store(true)
			if err := realtime.SendAudio(nil, true); err != nil {
				errCh <- err
				return
			}
			time.AfterFunc(8*time.Second, cancel)
		default:
			errCh <- errors.New("dictation: unsupported browser frame")
			return
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

func (h *Handler) fallback(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAudioUpload)
	if err := r.ParseMultipartForm(maxAudioUpload); err != nil {
		http.Error(w, "La registrazione è troppo grande o non valida.", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("audio")
	if err != nil {
		http.Error(w, "La registrazione audio è obbligatoria.", http.StatusUnprocessableEntity)
		return
	}
	defer file.Close()
	contentType := strings.ToLower(strings.TrimSpace(header.Header.Get("Content-Type")))
	if len(header.Filename) > 255 || !allowedAudioType(contentType) {
		http.Error(w, "Formato audio non supportato.", http.StatusUnsupportedMediaType)
		return
	}
	transcript, err := h.transcriber.Transcribe(r.Context(), header.Filename, file)
	if err != nil {
		h.logger.Warn("transcribe uploaded dictation", "err", err)
		http.Error(w, "Impossibile trascrivere la registrazione.", http.StatusBadGateway)
		return
	}
	captureContext, err := h.captureContext(r.Context())
	if err != nil {
		http.Error(w, "Impossibile caricare il contesto contabile.", http.StatusBadGateway)
		return
	}
	capture, err := StartCapture(r.Context(), h.extractor, captureContext)
	if err != nil {
		http.Error(w, "Il modello locale non è disponibile.", http.StatusBadGateway)
		return
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := capture.Close(cleanupCtx); err != nil {
			h.logger.Warn("delete fallback dictation session", "err", err)
		}
	}()
	state, err := capture.Apply(r.Context(), transcript)
	if err != nil {
		http.Error(w, "Impossibile interpretare la registrazione.", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(socketMessage{Type: "drafts", Text: transcript, Extraction: &state})
}

func allowedAudioType(value string) bool {
	if value == "" || value == "application/octet-stream" {
		return true
	}
	for _, allowed := range []string{"audio/webm", "audio/wav", "audio/x-wav", "audio/mpeg", "audio/mp4", "audio/ogg"} {
		if strings.HasPrefix(value, allowed) {
			return true
		}
	}
	return false
}

func (h *Handler) confirm(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBody)
	drafts, err := parseBatchForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	accountNames, err := h.accountNames(r.Context())
	if err != nil {
		http.Error(w, "Impossibile verificare i conti.", http.StatusBadGateway)
		return
	}
	batch, issues := ValidateDrafts(drafts, accountNames)
	if len(issues) > 0 {
		for _, draft := range drafts {
			if issue := issues[draft.ID]; issue != "" {
				http.Error(w, issue, http.StatusUnprocessableEntity)
				return
			}
		}
	}
	inputs, err := h.ledgerInputs(r.Context(), batch)
	if err != nil {
		h.logger.Warn("validate dictated ledger movements", "err", err)
		http.Error(w, "Verifica conti e categorie delle bozze.", http.StatusUnprocessableEntity)
		return
	}
	created, err := h.ledger.CreateMovementBatch(r.Context(), inputs)
	if err != nil {
		h.logger.Error("append dictated transactions", "err", err)
		http.Error(w, "Impossibile salvare i movimenti. Riprova.", http.StatusBadGateway)
		return
	}
	w.Header().Set("X-Spese-Success", fmt.Sprintf("Salvati %d movimenti.", len(created)))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]any{"data": created})
}

func (h *Handler) ledgerInputs(ctx context.Context, batch []ValidatedDraft) ([]ledgerpkg.MovementInput, error) {
	result := make([]ledgerpkg.MovementInput, 0, len(batch))
	for _, transaction := range batch {
		var accountID string
		if err := h.store.DB().QueryRowContext(ctx, `SELECT id FROM accounts WHERE lower(name) = lower(?) AND archived_at = ''`, transaction.Account).Scan(&accountID); err != nil {
			return nil, err
		}
		kind, categoryKind := ledgerpkg.MovementExpense, ledgerpkg.CategoryExpense
		if transaction.Kind == incomeKind {
			kind, categoryKind = ledgerpkg.MovementIncome, ledgerpkg.CategoryIncome
		}
		var categoryID string
		if strings.TrimSpace(transaction.Subcategory) != "" {
			if err := h.store.DB().QueryRowContext(ctx, `
				SELECT child.id FROM categories child JOIN categories parent ON parent.id = child.parent_id
				WHERE child.kind = ? AND lower(parent.name) = lower(?) AND lower(child.name) = lower(?)
			`, categoryKind, transaction.Category, transaction.Subcategory).Scan(&categoryID); err != nil {
				return nil, err
			}
		} else if err := h.store.DB().QueryRowContext(ctx, `
			SELECT id FROM categories WHERE kind = ? AND parent_id IS NULL AND lower(name) = lower(?)
		`, categoryKind, transaction.Category).Scan(&categoryID); err != nil {
			return nil, err
		}
		amount := int64(transaction.Amount)
		if amount < 0 {
			amount = -amount
		}
		result = append(result, ledgerpkg.MovementInput{
			Kind: kind, Status: ledgerpkg.MovementPosted, Date: transaction.Date.ISO(), AccountID: accountID,
			AmountCents: amount, Merchant: transaction.Merchant, Description: transaction.Description, Note: transaction.Note, Origin: "dictation",
			Allocations: []ledgerpkg.AllocationInput{{CategoryID: categoryID, AmountCents: amount}},
		})
	}
	return result, nil
}

func (h *Handler) captureContext(ctx context.Context) (CaptureContext, error) {
	now := h.now()
	accountNames, err := h.accountNames(ctx)
	if err != nil {
		return CaptureContext{}, err
	}
	rows, err := h.store.DB().QueryContext(ctx, `
		SELECT m.business_date, m.kind, account.name, m.amount_cents, m.merchant, m.description,
			coalesce(parent.name, category.name, ''),
			CASE WHEN category.parent_id IS NOT NULL THEN category.name ELSE '' END
		FROM movements m
		JOIN postings posting ON posting.movement_id = m.id
		JOIN accounts account ON account.id = posting.account_id
		LEFT JOIN movement_allocations allocation ON allocation.movement_id = m.id
		LEFT JOIN categories category ON category.id = allocation.category_id
		LEFT JOIN categories parent ON parent.id = category.parent_id
		WHERE m.status = 'posted' AND m.kind IN ('expense', 'income') AND m.business_date >= ?
		ORDER BY m.business_date DESC, m.id DESC
	`, now.AddDate(0, 0, -90).Format("2006-01-02"))
	if err != nil {
		return CaptureContext{}, err
	}
	defer rows.Close()
	result := CaptureContext{Today: now, Accounts: accountNames}
	categories := make(map[string]bool)
	for rows.Next() {
		var date, kind, account, merchant, description, category, subcategory string
		var amount int64
		if err := rows.Scan(&date, &kind, &account, &amount, &merchant, &description, &category, &subcategory); err != nil {
			return CaptureContext{}, err
		}
		result.History = append(result.History, HistoryItem{
			Date: date, Kind: strings.ToUpper(kind[:1]) + kind[1:], Account: account,
			Amount: fmt.Sprintf("%d,%02d", amount/100, amount%100), Payee: merchant, Description: description,
			Category: category, Subcategory: subcategory,
		})
		if category != "" {
			categories[category] = true
		}
	}
	for category := range categories {
		result.Categories = append(result.Categories, category)
	}
	sort.Strings(result.Categories)
	return result, nil
}

func (h *Handler) accountNames(ctx context.Context) ([]string, error) {
	rows, err := h.store.DB().QueryContext(ctx, `SELECT name FROM accounts WHERE archived_at = '' ORDER BY lower(name)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		result = append(result, name)
	}
	return result, rows.Err()
}

func parseBatchForm(r *http.Request) ([]Draft, error) {
	if err := r.ParseForm(); err != nil {
		return nil, errors.New("Il modulo inviato non è valido.")
	}
	fields := []string{"id", "kind", "date", "account", "amount", "payee", "description", "category", "subcategory", "note"}
	count := len(r.Form["id"])
	if count == 0 {
		return nil, errors.New("Non ci sono movimenti da salvare.")
	}
	if count > maxDrafts {
		return nil, fmt.Errorf("Puoi salvare al massimo %d movimenti alla volta.", maxDrafts)
	}
	for _, field := range fields {
		if len(r.Form[field]) != count {
			return nil, errors.New("I movimenti inviati non sono validi.")
		}
	}
	drafts := make([]Draft, count)
	for i := range drafts {
		drafts[i] = Draft{
			ID: r.Form["id"][i], Kind: r.Form["kind"][i], Date: r.Form["date"][i],
			Account: r.Form["account"][i], Amount: r.Form["amount"][i], Payee: r.Form["payee"][i], Description: r.Form["description"][i],
			Category: r.Form["category"][i], Subcategory: r.Form["subcategory"][i], Note: r.Form["note"][i],
		}
		trimDraft(&drafts[i])
	}
	return drafts, nil
}

func sameOriginHandshake(_ *websocket.Config, r *http.Request) error {
	origin, err := url.Parse(r.Header.Get("Origin"))
	if err != nil || origin.Host == "" || !strings.EqualFold(origin.Host, r.Host) {
		return errors.New("dictation: websocket origin is not allowed")
	}
	return nil
}

type socketMessage struct {
	Type        string      `json:"type"`
	Text        string      `json:"text,omitempty"`
	Message     string      `json:"message,omitempty"`
	Recoverable bool        `json:"recoverable,omitempty"`
	Extraction  *Extraction `json:"extraction,omitempty"`
}

type socketWriter struct {
	mu   sync.Mutex
	conn *websocket.Conn
}

func (w *socketWriter) send(message socketMessage) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return websocket.JSON.Send(w.conn, message)
}

func (h *Handler) sendSocketError(ws *websocket.Conn, message string) {
	_ = websocket.JSON.Send(ws, socketMessage{Type: "error", Message: message})
}
