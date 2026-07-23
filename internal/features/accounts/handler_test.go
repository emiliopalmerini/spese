package accounts

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"spese/internal/storage"
)

func TestCreateRejectsInvalidAccountWithLocalizedMessage(t *testing.T) {
	form := url.Values{
		"type":  {"Asset"},
		"class": {"Cash"},
	}
	req := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	(&Handler{}).create(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "Il nome del conto è obbligatorio." {
		t.Fatalf("unexpected validation message %q", got)
	}
}

func TestCreateRejectsUnsupportedCurrency(t *testing.T) {
	form := url.Values{
		"name":     {"Broker USD"},
		"type":     {"Asset"},
		"class":    {"Investment"},
		"currency": {"USD"},
	}
	req := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	(&Handler{}).create(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "Al momento sono supportati solo conti in EUR." {
		t.Fatalf("unexpected validation message %q", got)
	}
}

func TestParseFormRejectsUnknownClass(t *testing.T) {
	form := url.Values{
		"name":  {"Wallet"},
		"type":  {"Asset"},
		"class": {"Crypto"},
	}
	req := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, err := parseForm(req)

	if err == nil || err.Error() != "Seleziona una classe di conto valida." {
		t.Fatalf("parseForm error = %v", err)
	}
}

func TestParseFormRejectsActiveFromAfterActiveTo(t *testing.T) {
	form := url.Values{
		"name":        {"Wallet"},
		"type":        {"Asset"},
		"class":       {"Cash"},
		"active_from": {"2026-06"},
		"active_to":   {"2026-05"},
	}
	req := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_, err := parseForm(req)

	if err == nil || err.Error() != "La data di attivazione non può essere successiva alla data di disattivazione." {
		t.Fatalf("parseForm error = %v", err)
	}
}

func TestCreateRejectsDuplicateNameWithLocalizedMessage(t *testing.T) {
	ctx := context.Background()
	store, err := storage.Open(ctx, filepath.Join(t.TempDir(), "accounts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := Append(ctx, store, Account{Name: "Wallet", Type: Asset, Class: Cash, Currency: "EUR"}); err != nil {
		t.Fatal(err)
	}

	form := url.Values{
		"name":  {"Wallet"},
		"type":  {"Asset"},
		"class": {"Cash"},
	}
	req := httptest.NewRequest(http.MethodPost, "/accounts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h := &Handler{
		Store:  store,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	h.create(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected status %d, got %d", http.StatusUnprocessableEntity, w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "Esiste già un conto con questo nome." {
		t.Fatalf("unexpected duplicate message %q", got)
	}
}
