package core

import (
	"strings"
	"testing"
	"time"
)

func TestDateValidate(t *testing.T) {
	cases := []struct {
		d  Date
		ok bool
	}{
		{NewDate(2025, 1, 1), true},
		{NewDate(2025, 12, 31), true},
		{Date{Time: time.Time{}}, false}, // zero time
	}
	for i, tc := range cases {
		err := tc.d.Validate()
		if tc.ok && err != nil {
			t.Fatalf("case %d expected ok, got %v", i, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("case %d expected error", i)
		}
	}
}

func TestMoneyValidate(t *testing.T) {
	if err := (Money{Cents: 1}).Validate(); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	if err := (Money{Cents: 0}).Validate(); err == nil {
		t.Fatalf("expected error for zero")
	}
}

func TestAccountValidate(t *testing.T) {
	good := Account{Name: "Conto BCC", Type: AccountCash, Active: true}
	if err := good.Validate(); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}

	longName := strings.Repeat("a", 81)

	bads := []struct {
		a    Account
		want error
	}{
		{Account{Name: "", Type: AccountCash}, ErrEmptyAccountName},
		{Account{Name: "   ", Type: AccountCash}, ErrEmptyAccountName},
		{Account{Name: longName, Type: AccountCash}, ErrAccountNameTooLong},
		{Account{Name: "x", Type: AccountType("bogus")}, ErrInvalidAccountType},
	}
	for i, tc := range bads {
		if err := tc.a.Validate(); err != tc.want {
			t.Fatalf("case %d: expected %v, got %v", i, tc.want, err)
		}
	}
}

func TestAccountBalanceValidate(t *testing.T) {
	good := AccountBalance{AccountID: 1, Year: 2025, Month: 6, Amount: Money{Cents: 1000}}
	if err := good.Validate(); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	// zero amount allowed
	zero := AccountBalance{AccountID: 1, Year: 2025, Month: 6, Amount: Money{Cents: 0}}
	if err := zero.Validate(); err != nil {
		t.Fatalf("zero amount must be allowed, got %v", err)
	}

	bads := []struct {
		b    AccountBalance
		want error
	}{
		{AccountBalance{AccountID: 1, Year: 1999, Month: 1, Amount: Money{Cents: 1}}, ErrInvalidPeriod},
		{AccountBalance{AccountID: 1, Year: 2025, Month: 0, Amount: Money{Cents: 1}}, ErrInvalidPeriod},
		{AccountBalance{AccountID: 1, Year: 2025, Month: 13, Amount: Money{Cents: 1}}, ErrInvalidPeriod},
		{AccountBalance{AccountID: 1, Year: 2025, Month: 6, Amount: Money{Cents: -1}}, ErrInvalidAmount},
	}
	for i, tc := range bads {
		if err := tc.b.Validate(); err != tc.want {
			t.Fatalf("case %d: expected %v, got %v", i, tc.want, err)
		}
	}
}

func TestExpenseValidate(t *testing.T) {
	good := Expense{
		Date:        NewDate(2025, 1, 1),
		Description: "ok",
		Amount:      Money{Cents: 100},
		Primary:     "Cat",
		Secondary:   "Sub",
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}

	bads := []Expense{
		{Date: Date{Time: time.Time{}}, Description: "a", Amount: Money{Cents: 1}, Primary: "c", Secondary: "s"}, // zero date
		{Date: NewDate(2025, 1, 1), Description: "", Amount: Money{Cents: 1}, Primary: "c", Secondary: "s"},
		{Date: NewDate(2025, 1, 1), Description: "a", Amount: Money{Cents: 0}, Primary: "c", Secondary: "s"},
		{Date: NewDate(2025, 1, 1), Description: "a", Amount: Money{Cents: 1}, Primary: "", Secondary: "s"},
		{Date: NewDate(2025, 1, 1), Description: "a", Amount: Money{Cents: 1}, Primary: "c", Secondary: ""},
	}
	for i, e := range bads {
		if err := e.Validate(); err == nil {
			t.Fatalf("case %d expected error", i)
		}
	}
}
