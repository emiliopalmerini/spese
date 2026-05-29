// Package kernel holds tiny value types shared across feature slices.
// It must not import anything from internal/features or internal/sheets.
package kernel

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Money is an integer number of cents (1/100 of the base unit).
// Sign is preserved: positive = inflow, negative = outflow.
type Money int64

// ParseMoney parses a human string into Money. Accepts "12.34", "12,34",
// "1.234,56" (italian), "1,234.56" (english), and bare integers.
func ParseMoney(s string) (Money, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty money string")
	}
	s = strings.NewReplacer("€", "", "$", "", " ", "", " ", "").Replace(s)
	// Normalise: drop thousand separators by detecting which of '.' or ','
	// appears last (that's the decimal separator).
	lastDot := strings.LastIndex(s, ".")
	lastComma := strings.LastIndex(s, ",")
	var dec int
	switch {
	case lastDot < 0 && lastComma < 0:
		dec = -1
	case lastDot > lastComma:
		s = strings.ReplaceAll(s, ",", "")
		dec = strings.LastIndex(s, ".")
	default:
		s = strings.ReplaceAll(s, ".", "")
		dec = strings.LastIndex(s, ",")
		// translate the decimal comma to a dot so the string becomes digits-only
		// after the fractional part is normalized below.
		s = s[:dec] + "." + s[dec+1:]
		dec = strings.LastIndex(s, ".")
	}
	if dec >= 0 {
		// Pad or truncate the fractional part to exactly two digits.
		intPart := s[:dec]
		frac := s[dec+1:]
		switch {
		case len(frac) < 2:
			frac += strings.Repeat("0", 2-len(frac))
		case len(frac) > 2:
			frac = frac[:2]
		}
		s = intPart + frac
	} else {
		s += "00"
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse money %q: %w", s, err)
	}
	return Money(n), nil
}

// Float returns the value as a float (for sheet writes).
func (m Money) Float() float64 { return float64(m) / 100 }

// String formats in italian style: "1.234,56".
func (m Money) String() string {
	neg := m < 0
	n := int64(m)
	if neg {
		n = -n
	}
	intPart := n / 100
	frac := n % 100

	// Insert thousand separators.
	intStr := strconv.FormatInt(intPart, 10)
	var buf strings.Builder
	for i, r := range intStr {
		if i > 0 && (len(intStr)-i)%3 == 0 {
			buf.WriteByte('.')
		}
		buf.WriteRune(r)
	}
	sign := ""
	if neg {
		sign = "-"
	}
	return fmt.Sprintf("%s%s,%02d", sign, buf.String(), frac)
}
