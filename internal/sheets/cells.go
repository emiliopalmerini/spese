package sheets

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"spese/internal/kernel"
)

// CellString coerces a cell to string. nil → "".
func CellString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}

// CellFloat coerces a cell to float64. Empty or non-numeric returns 0, false.
// Sheets values come back as float64 when USER_ENTERED was used; numbers
// pasted as text come back as string.
func CellFloat(v any) (float64, bool) {
	if v == nil {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0, false
		}
		// Strip currency symbols + thousand separators for robust parsing.
		s = strings.NewReplacer("€", "", "$", "", " ", "", " ", "").Replace(s)
		if f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", "."), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// CellMoney converts a cell to Money (cents).
func CellMoney(v any) (kernel.Money, bool) {
	if f, ok := CellFloat(v); ok {
		return kernel.Money(int64(f*100 + 0.5*sign(f))), true
	}
	if s := CellString(v); s != "" {
		if m, err := kernel.ParseMoney(s); err == nil {
			return m, true
		}
	}
	return 0, false
}

// CellDate parses a cell as a date. Accepts ISO strings and Sheets serial
// numbers (days since 1899-12-30).
func CellDate(v any) (kernel.Date, bool) {
	if v == nil {
		return kernel.Date{}, false
	}
	if f, ok := CellFloat(v); ok {
		t := sheetsEpoch.Add(time.Duration(f) * 24 * time.Hour)
		return kernel.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)}, true
	}
	s := CellString(v)
	if s == "" {
		return kernel.Date{}, false
	}
	if d, err := kernel.ParseDate(s); err == nil {
		return d, true
	}
	return kernel.Date{}, false
}

// sheetsEpoch is the Lotus-bug-compatible epoch Google Sheets uses for
// numeric date cells: 1899-12-30 UTC.
var sheetsEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}
