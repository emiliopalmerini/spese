// Package core provides money parsing and handling utilities.
//
// This file contains functions for parsing monetary amounts from strings
// and converting between cents and euro representations.
package core

import (
	"errors"
	"strconv"
	"strings"
	"unicode"
)

// ParseDecimalToCents converts a decimal string to cents with proper rounding.
//
// It accepts both dot (12.34) and comma (12,34) decimal separators and performs
// half-up rounding on the third decimal place. The result is always positive cents.
// Returns an error for invalid formats, negative values, or zero amounts.
//
// Examples:
//   ParseDecimalToCents("12.34") -> 1234, nil
//   ParseDecimalToCents("12,34") -> 1234, nil
//   ParseDecimalToCents("12.345") -> 1234, nil (rounds down)
//   ParseDecimalToCents("12.346") -> 1235, nil (rounds up)
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
	// Convert integer part - check for overflow
	iv, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil {
		return 0, ErrInvalidAmount
	}
	// Prevent overflow when multiplying by 100
	const maxSafeInt64 = (1<<63 - 1) / 100
	if iv > maxSafeInt64 {
		return 0, ErrInvalidAmount
	}
	// Take first two fractional digits; then half-up rounding on third
	var fracCents int64 = 0
	if len(fracPart) > 0 {
		d1 := int64(fracPart[0] - '0')
		fracCents = d1 * 10
		if len(fracPart) > 1 {
			d2 := int64(fracPart[1] - '0')
			fracCents += d2
			if len(fracPart) > 2 {
				if fracPart[2] >= '5' {
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

// Euros returns the euro value as a float64 for display purposes.
// This method is primarily used for formatting money amounts in user interfaces.
// Note: Use cents for calculations to avoid floating-point precision issues.
func (m Money) Euros() float64 {
	return float64(m.Cents) / 100.0
}

var _ = errors.Is // keep errors imported if unused yet
