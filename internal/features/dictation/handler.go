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

	"spese/internal/features/accounts"
	"spese/internal/features/transactions"
	"spese/internal/kernel"
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
	extractor   extractor
	transcriber transcriber
	logger      *slog.Logger
	now         func() time.Time
}

func NewHandler(store *storage.Store, extractor *OpenCodeClient, transcriber *ElevenLabsTranscriber, logger *slog.Logger) *Handler {
	return &Handler{store: store, extractor: extractor, transcriber: transcriber, logger: logger, now: time.Now}
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

func (h *Handler) confirm(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBody)
	drafts, err := parseBatchForm(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	accountRows, err := accounts.List(r.Context(), h.store, false)
	if err != nil {
		http.Error(w, "Impossibile verificare i conti.", http.StatusBadGateway)
		return
	}
	accountNames := make([]string, len(accountRows))
	for i, account := range accountRows {
		accountNames[i] = account.Name
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
	if err := transactions.Append(r.Context(), h.store, batch); err != nil {
		h.logger.Error("append dictated transactions", "err", err)
		http.Error(w, "Impossibile salvare i movimenti. Riprova.", http.StatusBadGateway)
		return
	}
	w.Header().Set("X-Spese-Success", fmt.Sprintf("Salvati %d movimenti.", len(batch)))
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/transactions")
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/transactions", http.StatusSeeOther)
}

func (h *Handler) captureContext(ctx context.Context) (CaptureContext, error) {
	now := h.now()
	accountRows, err := accounts.List(ctx, h.store, false)
	if err != nil {
		return CaptureContext{}, err
	}
	history, err := transactions.List(ctx, h.store, transactions.Filter{
		From: kernel.Date{Time: now.AddDate(0, 0, -90)},
	}, false)
	if err != nil {
		return CaptureContext{}, err
	}
	result := CaptureContext{Today: now, Accounts: make([]string, len(accountRows))}
	for i, account := range accountRows {
		result.Accounts[i] = account.Name
	}
	categories := make(map[string]bool)
	result.History = make([]HistoryItem, 0, len(history))
	for _, transaction := range history {
		amount := transaction.Amount
		if amount < 0 {
			amount = -amount
		}
		result.History = append(result.History, HistoryItem{
			Date: transaction.Date.ISO(), Kind: string(transaction.Kind), Account: transaction.Account,
			Amount: fmt.Sprintf("%.2f", amount.Float()), Payee: transaction.Payee,
			Category: transaction.Category, Subcategory: transaction.Subcategory,
		})
		if transaction.Category != "" {
			categories[transaction.Category] = true
		}
	}
	for category := range categories {
		result.Categories = append(result.Categories, category)
	}
	sort.Strings(result.Categories)
	return result, nil
}

func parseBatchForm(r *http.Request) ([]Draft, error) {
	if err := r.ParseForm(); err != nil {
		return nil, errors.New("Il modulo inviato non è valido.")
	}
	fields := []string{"id", "kind", "date", "account", "amount", "payee", "category", "subcategory", "note"}
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
			Account: r.Form["account"][i], Amount: r.Form["amount"][i], Payee: r.Form["payee"][i],
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
