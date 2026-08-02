package dictation

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseBatchForm(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest("POST", "/dictation/confirm", strings.NewReader(
		"id=draft-1&id=draft-2&kind=Expense&kind=Income&date=2026-07-26&date=2026-07-25"+
			"&account=Fineco&account=Fineco&amount=12%2C50&amount=1000&payee=Conad&payee=Stipendio&description=Spesa&description=Mensilit%C3%A0"+
			"&category=Spesa&category=Entrate&subcategory=&subcategory=&note=&note=luglio",
	))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	got, err := parseBatchForm(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "draft-1" || got[1].Payee != "Stipendio" {
		t.Fatalf("drafts = %#v", got)
	}
}

func TestParseBatchFormRejectsMisalignedColumns(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest("POST", "/dictation/confirm", strings.NewReader(
		"id=draft-1&id=draft-2&kind=Expense&date=2026-07-26",
	))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if _, err := parseBatchForm(request); err == nil {
		t.Fatal("parseBatchForm() error = nil")
	}
}
