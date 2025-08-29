package core

import (
    "errors"
    "strconv"
    "strings"
    "unicode"
)

// ParseDecimalToCents converts a decimal string (e.g., "12.34" or "12,34") to cents with half-up rounding.
func ParseDecimalToCents(s string) (int64, error) {
    s = strings.TrimSpace(s)
    if s == "" {
        return 0, ErrInvalidAmount
    }
    // Normalize decimal comma to dot
    s = strings.ReplaceAll(s, ",", ".")
    if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
        // Only positive values allowed
        return 0, ErrInvalidAmount
    }
    // Split into integer and fractional part
    parts := strings.Split(s, ".")
    if len(parts) > 2 {
        return 0, ErrInvalidAmount
    }
    intPart := parts[0]
    fracPart := ""
    if len(parts) == 2 {
        fracPart = parts[1]
    }
    if intPart == "" {
        intPart = "0"
    }
    for _, r := range intPart {
        if !unicode.IsDigit(r) {
            return 0, ErrInvalidAmount
        }
    }
    for _, r := range fracPart {
        if !unicode.IsDigit(r) {
            return 0, ErrInvalidAmount
        }
    }
    // Convert integer part
    iv, err := strconv.ParseInt(intPart, 10, 64)
    if err != nil {
        return 0, ErrInvalidAmount
    }
    // Take first two fractional digits; then half-up rounding on third
    var fracCents int64 = 0
    if len(fracPart) > 0 {
        d1 := int64(fracPart[0]-'0')
        fracCents = d1 * 10
        if len(fracPart) > 1 {
            d2 := int64(fracPart[1]-'0')
            fracCents += d2
            if len(fracPart) > 2 {
                d3 := fracPart[2] - '0'
                if d3 >= '5' {
                    fracCents++
                }
            }
        }
    }
    cents := iv*100 + fracCents
    if cents <= 0 {
        return 0, ErrInvalidAmount
    }
    return cents, nil
}

var _ = errors.Is // keep errors imported if unused yet
