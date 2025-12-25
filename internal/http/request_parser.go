// Package http provides HTTP server and handler implementations.
//
// This file implements utilities for parsing and validating HTTP request data.
// It reduces code duplication by providing reusable functions for common
// form parsing, date extraction, and input sanitization patterns.

package http

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DateParams holds parsed year/month/day values from request parameters.
type DateParams struct {
	Year  int
	Month int
	Day   int
}

// MonthParams holds parsed year/month values from request parameters.
type MonthParams struct {
	Year  int
	Month int
}

// ParseDateParams extracts day and month from form values, using current date as defaults.
// This consolidates the repeated pattern of extracting date params with fallback to now.
func ParseDateParams(form url.Values) DateParams {
	now := time.Now()
	params := DateParams{
		Year:  now.Year(),
		Month: int(now.Month()),
		Day:   now.Day(),
	}

	if v := strings.TrimSpace(form.Get("year")); v != "" {
		if y, err := strconv.Atoi(v); err == nil {
			params.Year = y
		}
	}
	if v := strings.TrimSpace(form.Get("month")); v != "" {
		if m, err := strconv.Atoi(v); err == nil {
			params.Month = m
		}
	}
	if v := strings.TrimSpace(form.Get("day")); v != "" {
		if d, err := strconv.Atoi(v); err == nil {
			params.Day = d
		}
	}

	return params
}

// ParseMonthParams extracts year and month from query parameters, using current date as defaults.
// This consolidates the repeated pattern of extracting month/year from URL query strings.
func ParseMonthParams(query url.Values) MonthParams {
	now := time.Now()
	params := MonthParams{
		Year:  now.Year(),
		Month: int(now.Month()),
	}

	if v := strings.TrimSpace(query.Get("year")); v != "" {
		if y, err := strconv.Atoi(v); err == nil {
			params.Year = y
		}
	}
	if v := strings.TrimSpace(query.Get("month")); v != "" {
		if m, err := strconv.Atoi(v); err == nil {
			params.Month = m
		}
	}

	return params
}

// RequestBodyParser handles different content types for request body parsing.
// It supports both JSON and form-encoded data, commonly used with HTMX.
type RequestBodyParser struct {
	body        []byte
	contentType string
	jsonData    map[string]interface{}
	formData    url.Values
	parsed      bool
	err         error
}

// NewRequestBodyParser creates a parser for the given request.
// It reads the body once and stores it for subsequent parsing.
func NewRequestBodyParser(r *http.Request) *RequestBodyParser {
	p := &RequestBodyParser{
		contentType: r.Header.Get("Content-Type"),
	}

	p.body, p.err = io.ReadAll(r.Body)
	return p
}

// Parse attempts to parse the body as JSON or form data.
func (p *RequestBodyParser) Parse() error {
	if p.parsed {
		return p.err
	}
	p.parsed = true

	if p.err != nil {
		return p.err
	}

	if len(p.body) == 0 {
		p.formData = url.Values{}
		return nil
	}

	// Try JSON first if content looks like JSON
	if p.body[0] == '{' || p.body[0] == '[' {
		p.jsonData = make(map[string]interface{})
		if err := json.Unmarshal(p.body, &p.jsonData); err != nil {
			p.err = err
			return err
		}
		return nil
	}

	// Fall back to form parsing
	p.formData, p.err = url.ParseQuery(string(p.body))
	return p.err
}

// Get returns a string value from the parsed data (JSON or form).
func (p *RequestBodyParser) Get(key string) string {
	if p.jsonData != nil {
		if val, ok := p.jsonData[key]; ok {
			return strings.TrimSpace(sanitizeInput(stringValue(val)))
		}
	}
	if p.formData != nil {
		return strings.TrimSpace(sanitizeInput(p.formData.Get(key)))
	}
	return ""
}

// GetRaw returns the raw body bytes.
func (p *RequestBodyParser) GetRaw() []byte {
	return p.body
}

// ContentType returns the Content-Type header value.
func (p *RequestBodyParser) ContentType() string {
	return p.contentType
}

// IsJSON returns true if the parsed content was JSON.
func (p *RequestBodyParser) IsJSON() bool {
	return p.jsonData != nil
}

// stringValue converts an interface{} to string.
func stringValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case bool:
		return strconv.FormatBool(val)
	default:
		return ""
	}
}

// RequireMethod checks if the request method matches the expected method(s).
// Returns an error response builder if the method doesn't match.
func RequireMethod(r *http.Request, methods ...string) *HTMXResponseBuilder {
	for _, m := range methods {
		if r.Method == m {
			return nil
		}
	}
	return MethodNotAllowedError(strings.Join(methods, ", "))
}

// RequirePOST is a convenience function for POST-only handlers.
func RequirePOST(r *http.Request) *HTMXResponseBuilder {
	return RequireMethod(r, http.MethodPost)
}

// RequireDeleteOrPOST is a convenience function for DELETE/POST handlers.
func RequireDeleteOrPOST(r *http.Request) *HTMXResponseBuilder {
	return RequireMethod(r, http.MethodDelete, http.MethodPost)
}

// ParseFormOrFail parses the request form and returns an error response on failure.
// Returns nil on success.
func ParseFormOrFail(r *http.Request) *HTMXResponseBuilder {
	if err := r.ParseForm(); err != nil {
		return BadRequestError("Formato richiesta non valido")
	}
	return nil
}
