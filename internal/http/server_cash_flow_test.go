package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"spese/internal/adapters"
	"spese/internal/core"
)

func TestCashFlowEmptyYear(t *testing.T) {
	srv, _ := newNetWorthServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/dashboard/cash-flow", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Nessun dato") {
		t.Fatalf("expected placeholder, got: %s", rr.Body.String())
	}
}

func TestCashFlowFreelanceWithAccruals(t *testing.T) {
	srv, _ := newNetWorthServer(t)
	a := srv.expWriter.(*adapters.SQLiteAdapter)

	year := time.Now().Year()
	// Append a freelance income; accrual hook fires automatically.
	if _, err := a.AppendIncome(context.Background(), core.Income{
		Date:        core.NewDate(year, 6, 15),
		Description: "consulting",
		Amount:      core.Money{Cents: 100000},
		Category:    "GFreelance",
	}); err != nil {
		t.Fatalf("append income: %v", err)
	}
	// And a salary income for a separate row.
	if _, err := a.AppendIncome(context.Background(), core.Income{
		Date:        core.NewDate(year, 6, 10),
		Description: "stipendio",
		Amount:      core.Money{Cents: 250000},
		Category:    "Stipendio E",
	}); err != nil {
		t.Fatalf("append income: %v", err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/dashboard/cash-flow", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "GFreelance") {
		t.Fatalf("expected GFreelance row, got: %s", body)
	}
	if !strings.Contains(body, "Stipendio E") {
		t.Fatalf("expected Stipendio E row, got: %s", body)
	}
	if !strings.Contains(body, "Tasse statali") {
		t.Fatalf("expected Tasse statali section")
	}
	if !strings.Contains(body, "Imposta sostitutiva") || !strings.Contains(body, "INPS") {
		t.Fatalf("expected tax labels")
	}
	if !strings.Contains(body, "-€50,00") {
		t.Fatalf("expected imposta sostitutiva monthly negative -€50,00 in body, got: %s", body)
	}
}
