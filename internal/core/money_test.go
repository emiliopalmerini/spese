package core

import "testing"

func TestParseDecimalToCents(t *testing.T) {
	cases := []struct {
		in  string
		out int64
		ok  bool
	}{
		{"1", 100, true},
		{"1.0", 100, true},
		{"1.23", 123, true},
		{"1,23", 123, true},
		{"0.01", 1, true},
		{"1.005", 101, true}, // half-up rounding
		{" 2.50 ", 250, true},
		{"-1", 0, false},
		{"0", 0, false},
		{"abc", 0, false},
		{"1.2.3", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		got, err := ParseDecimalToCents(tc.in)
		if tc.ok {
			if err != nil || got != tc.out {
				t.Fatalf("%q expected %d, got %d (err=%v)", tc.in, tc.out, got, err)
			}
		} else {
			if err == nil {
				t.Fatalf("%q expected error", tc.in)
			}
		}
	}
}
