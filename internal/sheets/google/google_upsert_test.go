package google

import (
	"errors"
	"testing"
)

func TestFindRowByID(t *testing.T) {
	col := [][]any{
		{"id"}, // header row 1
		{"42"}, // row 2
		{""},   // row 3 (gap)
		{"7"},  // row 4
		{"99"}, // row 5
	}
	cases := []struct {
		name string
		id   int64
		want int
	}{
		{"first", 42, 2},
		{"middle", 7, 4},
		{"last", 99, 5},
		{"absent", 1234, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := findRowByID(col, tc.id)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("findRowByID(%d) = %d, want %d", tc.id, got, tc.want)
			}
		})
	}
}

func TestFindRowByID_Duplicate(t *testing.T) {
	col := [][]any{
		{"id"},
		{"42"},
		{"42"},
	}
	_, err := findRowByID(col, 42)
	if !errors.Is(err, ErrDuplicateRowID) {
		t.Fatalf("expected ErrDuplicateRowID, got %v", err)
	}
}

func TestFindRowByID_EmptyColumn(t *testing.T) {
	got, err := findRowByID(nil, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 0 {
		t.Fatalf("expected 0 for empty column, got %d", got)
	}
}

func TestFindRowByID_NumericCells(t *testing.T) {
	// Sheets API returns numeric cells as float64.
	col := [][]any{
		{"id"},
		{float64(42)},
		{int64(7)},
	}
	got, err := findRowByID(col, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 2 {
		t.Fatalf("expected row 2, got %d", got)
	}
	got, err = findRowByID(col, 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 3 {
		t.Fatalf("expected row 3, got %d", got)
	}
}
