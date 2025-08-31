package core

import "testing"

func TestDatePartsValidate(t *testing.T) {
	cases := []struct {
		d  DateParts
		ok bool
	}{
		{DateParts{Day: 1, Month: 1}, true},
		{DateParts{Day: 31, Month: 12}, true},
		{DateParts{Day: 0, Month: 1}, false},
		{DateParts{Day: 32, Month: 1}, false},
		{DateParts{Day: 1, Month: 0}, false},
		{DateParts{Day: 1, Month: 13}, false},
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
		Date:        DateParts{Day: 1, Month: 1},
		Description: "ok",
		Amount:      Money{Cents: 100},
		Primary:     "Cat",
		Secondary:   "Sub",
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}

	bads := []Expense{
		{Date: DateParts{Day: 0, Month: 1}, Description: "a", Amount: Money{Cents: 1}, Primary: "c", Secondary: "s"},
		{Date: DateParts{Day: 1, Month: 1}, Description: "", Amount: Money{Cents: 1}, Primary: "c", Secondary: "s"},
		{Date: DateParts{Day: 1, Month: 1}, Description: "a", Amount: Money{Cents: 0}, Primary: "c", Secondary: "s"},
		{Date: DateParts{Day: 1, Month: 1}, Description: "a", Amount: Money{Cents: 1}, Primary: "", Secondary: "s"},
		{Date: DateParts{Day: 1, Month: 1}, Description: "a", Amount: Money{Cents: 1}, Primary: "c", Secondary: ""},
	}
	for i, e := range bads {
		if err := e.Validate(); err == nil {
			t.Fatalf("case %d expected error", i)
		}
	}
}
