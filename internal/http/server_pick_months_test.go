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

func TestPickMonthsEmptyYear(t *testing.T) {
	srv, _ := newNetWorthServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/dashboard/pick-months", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "Nessun dato") {
		t.Fatalf("expected placeholder, got: %s", rr.Body.String())
	}
}

func TestPickMonthsWithExpenses(t *testing.T) {
	srv, _ := newNetWorthServer(t)
	a := srv.expWriter.(*adapters.SQLiteAdapter)

	// Seed expenses across two months (current year)
	year := time.Now().Year()
	for _, e := range []core.Expense{
		{Date: core.NewDate(year, 1, 5), Description: "groceries", Amount: core.Money{Cents: 5000}, Primary: "Spesa", Secondary: "Supermercato"},
		{Date: core.NewDate(year, 2, 8), Description: "metro", Amount: core.Money{Cents: 1500}, Primary: "Trasporti", Secondary: "Trasporto Pubblico"},
		// Lavoro should be excluded
		{Date: core.NewDate(year, 2, 9), Description: "lunch w/ client", Amount: core.Money{Cents: 4000}, Primary: "Lavoro", Secondary: "Lavoro g"},
	} {
		if _, err := a.Append(context.Background(), e); err != nil {
			t.Fatalf("append expense: %v", err)
		}
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/dashboard/pick-months", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Spesa") {
		t.Fatalf("expected Spesa column header in body")
	}
	if !strings.Contains(body, "Trasporti") {
		t.Fatalf("expected Trasporti column header in body")
	}
	if strings.Contains(body, "Lavoro") {
		t.Fatalf("Lavoro should be excluded from Pick Months")
	}
	if !strings.Contains(body, "50,00") {
		t.Fatalf("expected 50,00 cents value in body, got %s", body)
	}
}

func TestPickMonthsInvalidYear(t *testing.T) {
	srv, _ := newNetWorthServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/dashboard/pick-months?year=abc", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}
