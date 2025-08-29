package core

import (
    "errors"
    "math"
    "strconv"
    "strings"
)

// ParseDecimalToCents converts a decimal string (e.g., "12.34" or "12,34") to cents with half-up rounding.
func ParseDecimalToCents(s string) (int64, error) {
    s = strings.TrimSpace(s)
    if s == "" {
        return 0, ErrInvalidAmount
    }
    // Accept comma as decimal separator by normalizing to dot.
    s = strings.ReplaceAll(s, ",", ".")
    // Split integral and fractional parts to control rounding.
    parts := strings.SplitN(s, ".", 3)
    if len(parts) > 2 {
        return 0, ErrInvalidAmount
    }
    // Use ParseFloat then round half-up to 2 decimals to keep behavior consistent.
    f, err := strconv.ParseFloat(s, 64)
    if err != nil {
        return 0, ErrInvalidAmount
    }
    if f <= 0 {
        return 0, ErrInvalidAmount
    }
    // Half-up rounding to 2 decimals
    scaled := f * 100.0
    cents := int64(math.Floor(scaled + 0.5))
    if cents <= 0 {
        return 0, ErrInvalidAmount
    }
    return cents, nil
}

var _ = errors.Is // keep errors imported if unused yet

