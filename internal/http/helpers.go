package http

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"spese/internal/core"
)

// parseYearMonth extracts year and month from query parameters.
// Returns current year/month as defaults if not provided or invalid.
func parseYearMonth(r *http.Request) (year, month int) {
	now := time.Now()
	year = now.Year()
	month = int(now.Month())

	if v := strings.TrimSpace(r.URL.Query().Get("year")); v != "" {
		if y, err := strconv.Atoi(v); err == nil {
			year = y
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("month")); v != "" {
		if m, err := strconv.Atoi(v); err == nil {
			month = m
		}
	}

	return year, month
}

// parseDate parses a date string in YYYY-MM-DD format.
func parseDate(dateStr string) (core.Date, error) {
	parsedTime, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return core.Date{}, err
	}
	return core.Date{Time: parsedTime}, nil
}

// formatEuros formats cents as a Euro currency string (e.g., "€12,34").
func formatEuros(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	euros := cents / 100
	rem := cents % 100
	s := strconv.FormatInt(euros, 10) + "," + fmt.Sprintf("%02d", rem)
	if neg {
		return "-€" + s
	}
	return "€" + s
}

// sanitizeInput removes potentially dangerous characters and trims whitespace.
func sanitizeInput(s string) string {
	s = strings.TrimSpace(s)
	result := strings.Map(func(r rune) rune {
		if r < 32 && r != 9 && r != 10 && r != 13 {
			return -1
		}
		return r
	}, s)
	return result
}

// generateRequestID creates a unique request ID for tracing.
func generateRequestID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return "req_" + hex.EncodeToString(bytes)
}
