package core

import (
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
