package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"spese/internal/adapters"
	"spese/internal/core"
	"spese/internal/services"
	ports "spese/internal/sheets"
	"spese/internal/storage"
)

func formatInt(i int) string     { return strconv.Itoa(i) }
func formatInt64(i int64) string { return strconv.FormatInt(i, 10) }

func coreAccount(name, t string, active bool) core.Account {
	return core.Account{Name: name, Type: core.AccountType(t), Active: active}
}

func coreBalance(accountID int64, year, month int, cents int64) core.AccountBalance {
	return core.AccountBalance{
		AccountID: accountID,
		Year:      year,
		Month:     month,
		Amount:    core.Money{Cents: cents},
	}
}

func newNetWorthServer(t *testing.T) (*Server, *adapters.SQLiteAdapter) {
	t.Helper()
	chdirRepoRoot(t)

	dir := t.TempDir()
	repo, err := storage.NewSQLiteRepository(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	svc := services.NewExpenseService(repo)
	adapter := adapters.NewSQLiteAdapter(repo, svc)
	adapter.SetTaxAccrualService(services.NewTaxAccrualService(repo))

	var ew ports.ExpenseWriter = adapter
	var tr ports.TaxonomyReader = adapter
	var dr ports.DashboardReader = adapter
	var lr ports.ExpenseLister = adapter
	var lrid ports.ExpenseListerWithID = adapter
	srv := NewServer(":0", ew, tr, dr, lr, nil, lrid)
	return srv, adapter
}

func TestNetWorthPageRenders(t *testing.T) {
	srv, _ := newNetWorthServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/networth", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "Patrimonio") {
		t.Fatalf("expected page title in body")
	}
	if !strings.Contains(body, "/ui/networth/accounts") {
		t.Fatalf("expected accounts htmx URL")
	}
}

func TestNetWorthCreateAccountAndListBalance(t *testing.T) {
	srv, _ := newNetWorthServer(t)

	form := url.Values{}
	form.Set("name", "Conto BCC")
	form.Set("type", "cash")
	form.Set("active", "true")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ui/networth/accounts", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("create account expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "Conto BCC") {
		t.Fatalf("expected new account name in returned partial")
	}

	srvAdapter := srv.expWriter.(*adapters.SQLiteAdapter)
	accs, err := srvAdapter.ListAccounts(context.Background(), false)
	if err != nil || len(accs) != 1 {
		t.Fatalf("expected 1 account, got %d (%v)", len(accs), err)
	}
	id := accs[0].ID

	now := time.Now()
	year, month := now.Year(), int(now.Month())

	balForm := url.Values{}
	balForm.Set("account_id", strings.TrimSpace(formatInt64(id)))
	balForm.Set("year", formatInt(year))
	balForm.Set("month", formatInt(month))
	balForm.Set("amount", "1234,56")

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/ui/networth/balances", strings.NewReader(balForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("upsert balance expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "1234,56") {
		t.Fatalf("expected balance amount in partial, body=%s", rr.Body.String())
	}

	// Month partial returns the same balance
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/ui/networth/month?year="+formatInt(year)+"&month="+formatInt(month), nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("month partial expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "1234,56") {
		t.Fatalf("month partial missing balance: %s", rr.Body.String())
	}
}

func TestNetWorthDuplicateAccount(t *testing.T) {
	srv, _ := newNetWorthServer(t)

	form := url.Values{}
	form.Set("name", "Dup")
	form.Set("type", "cash")
	form.Set("active", "true")
	body := strings.NewReader(form.Encode())
	req := httptest.NewRequest(http.MethodPost, "/ui/networth/accounts", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	srv.Handler.ServeHTTP(httptest.NewRecorder(), req)

	// second time
	body = strings.NewReader(form.Encode())
	req = httptest.NewRequest(http.MethodPost, "/ui/networth/accounts", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 on duplicate, got %d", rr.Code)
	}
}

func TestNetWorthBalanceForInactiveAccount(t *testing.T) {
	srv, _ := newNetWorthServer(t)
	srvAdapter := srv.expWriter.(*adapters.SQLiteAdapter)

	id, err := srvAdapter.CreateAccount(context.Background(), coreAccount("Inactive", "cash", false))
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	form := url.Values{}
	form.Set("account_id", formatInt64(id))
	form.Set("year", formatInt(now.Year()))
	form.Set("month", formatInt(int(now.Month())))
	form.Set("amount", "10,00")

	req := httptest.NewRequest(http.MethodPost, "/ui/networth/balances", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for inactive account, got %d", rr.Code)
	}
}

func TestNetWorthDashboardTile(t *testing.T) {
	srv, _ := newNetWorthServer(t)
	srvAdapter := srv.expWriter.(*adapters.SQLiteAdapter)

	id, err := srvAdapter.CreateAccount(context.Background(), coreAccount("Acc", "cash", true))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	year, month := now.Year(), int(now.Month())
	prevMonth := month - 1
	prevYear := year
	if prevMonth < 1 {
		prevMonth = 12
		prevYear--
	}

	if err := srvAdapter.UpsertBalance(context.Background(), coreBalance(id, prevYear, prevMonth, 100000)); err != nil {
		t.Fatal(err)
	}
	if err := srvAdapter.UpsertBalance(context.Background(), coreBalance(id, year, month, 150000)); err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/dashboard/net-worth", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("tile expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !strings.Contains(body, "1.500") || !strings.Contains(body, ",00") {
		t.Fatalf("expected current total in tile, body=%s", body)
	}
	if !strings.Contains(body, "vs.") {
		t.Fatalf("expected delta indicator in tile, body=%s", body)
	}
}

func TestNetWorthInvalidYearMonth(t *testing.T) {
	srv, _ := newNetWorthServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/networth/month?year=abc&month=1", nil)
	srv.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid year, got %d", rr.Code)
	}
}
