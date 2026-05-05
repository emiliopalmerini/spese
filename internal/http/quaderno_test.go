package http

import "testing"

func TestItalianMonthLong(t *testing.T) {
	cases := map[int]string{
		1: "Gennaio", 5: "Maggio", 12: "Dicembre",
		0: "", 13: "", -1: "",
	}
	for in, want := range cases {
		if got := italianMonthLong(in); got != want {
			t.Errorf("italianMonthLong(%d)=%q want %q", in, got, want)
		}
	}
}

func TestItalianMonthShort(t *testing.T) {
	cases := map[int]string{
		1: "Gen", 11: "Nov", 12: "Dic", 0: "", 13: "",
	}
	for in, want := range cases {
		if got := italianMonthShort(in); got != want {
			t.Errorf("italianMonthShort(%d)=%q want %q", in, got, want)
		}
	}
}

func TestRomanNumeral(t *testing.T) {
	cases := map[int]string{
		1: "I", 4: "IV", 9: "IX", 12: "XII", 0: "", 13: "",
	}
	for in, want := range cases {
		if got := romanNumeral(in); got != want {
			t.Errorf("romanNumeral(%d)=%q want %q", in, got, want)
		}
	}
}

func TestSignedDeltaPct(t *testing.T) {
	type tc struct {
		curr, prev int64
		want       int
	}
	cases := []tc{
		{100, 0, 0},    // prev zero → 0
		{100, 100, 0},  // equal → 0
		{120, 100, 20}, // +20%
		{80, 100, -20}, // -20%
		{150, 100, 50},
		{50, 100, -50},
		{105, 100, 5},
		{99, 100, -1},
	}
	for _, c := range cases {
		if got := signedDeltaPct(c.curr, c.prev); got != c.want {
			t.Errorf("signedDeltaPct(%d,%d)=%d want %d", c.curr, c.prev, got, c.want)
		}
	}
}

func TestDailyRunRate(t *testing.T) {
	if got := dailyRunRate(10000, 10); got != 1000 {
		t.Errorf("dailyRunRate(10000,10)=%d want 1000", got)
	}
	if got := dailyRunRate(10000, 0); got != 10000 {
		t.Errorf("dailyRunRate divisor 0 should clamp to 1; got %d", got)
	}
	if got := dailyRunRate(0, 5); got != 0 {
		t.Errorf("dailyRunRate(0,5)=%d want 0", got)
	}
}

func TestPrevYearMonth(t *testing.T) {
	type tc struct {
		y, m, py, pm int
	}
	cases := []tc{
		{2026, 5, 2026, 4},
		{2026, 1, 2025, 12},
		{2026, 12, 2026, 11},
	}
	for _, c := range cases {
		py, pm := prevYearMonth(c.y, c.m)
		if py != c.py || pm != c.pm {
			t.Errorf("prevYearMonth(%d,%d)=(%d,%d) want (%d,%d)", c.y, c.m, py, pm, c.py, c.pm)
		}
	}
}

func TestFormatEurosInt(t *testing.T) {
	cases := map[int64]string{
		0:        "0",
		99:       "0",
		100:      "1",
		123456:   "1.234",
		1234567:  "12.345",
		12345678: "123.456",
		-123456:  "-1.234",
	}
	for in, want := range cases {
		if got := formatEurosInt(in); got != want {
			t.Errorf("formatEurosInt(%d)=%q want %q", in, got, want)
		}
	}
}

func TestFormatEurosDec(t *testing.T) {
	cases := map[int64]string{
		0:    "00",
		99:   "99",
		100:  "00",
		199:  "99",
		1234: "34",
		-7:   "07",
	}
	for in, want := range cases {
		if got := formatEurosDec(in); got != want {
			t.Errorf("formatEurosDec(%d)=%q want %q", in, got, want)
		}
	}
}
