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
		return parseFormattedFloat(x)
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

func parseFormattedFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}

	percent := strings.HasSuffix(s, "%")
	if percent {
		s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
	}

	s = strings.NewReplacer(
		"€", "",
		"$", "",
		" ", "",
		" ", "",
		" ", "",
	).Replace(s)
	if s == "" {
		return 0, false
	}

	normalized, ok := normalizeNumberString(s)
	if !ok {
		return 0, false
	}
	f, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return 0, false
	}
	if percent {
		f /= 100
	}
	return f, true
}

func normalizeNumberString(s string) (string, bool) {
	if !validNumberRunes(s) {
		return "", false
	}

	lastDot := strings.LastIndex(s, ".")
	lastComma := strings.LastIndex(s, ",")
	switch {
	case lastDot < 0 && lastComma < 0:
		return s, true
	case lastDot >= 0 && lastComma >= 0:
		if lastDot > lastComma {
			return strings.ReplaceAll(s, ",", ""), true
		}
		s = strings.ReplaceAll(s, ".", "")
		return strings.ReplaceAll(s, ",", "."), true
	case lastDot >= 0:
		if isGroupedNumber(s, '.') {
			return strings.ReplaceAll(s, ".", ""), true
		}
		return s, true
	default:
		if isGroupedNumber(s, ',') {
			return strings.ReplaceAll(s, ",", ""), true
		}
		return strings.ReplaceAll(s, ",", "."), true
	}
}

func validNumberRunes(s string) bool {
	for i, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r == '.' || r == ',':
		case (r == '-' || r == '+') && i == 0:
		default:
			return false
		}
	}
	return true
}

func isGroupedNumber(s string, sep byte) bool {
	if strings.Count(s, string(sep)) == 0 {
		return false
	}

	body := strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	parts := strings.Split(body, string(sep))
	if len(parts) < 2 || parts[0] == "" || len(parts[0]) > 3 {
		return false
	}
	if len(parts) == 2 && parts[0] == "0" {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	for _, part := range parts[1:] {
		if len(part) != 3 {
			return false
		}
	}
	return true
}
