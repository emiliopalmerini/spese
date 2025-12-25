package http

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestParseDateParams(t *testing.T) {
	tests := []struct {
		name      string
		formData  url.Values
		wantYear  bool // if true, check that year is non-zero
		wantMonth int
		wantDay   int
	}{
		{
			name:      "all values provided",
			formData:  url.Values{"year": {"2024"}, "month": {"6"}, "day": {"15"}},
			wantYear:  true,
			wantMonth: 6,
			wantDay:   15,
		},
		{
			name:      "only month and day",
			formData:  url.Values{"month": {"3"}, "day": {"20"}},
			wantYear:  true, // should use current year
			wantMonth: 3,
			wantDay:   20,
		},
		{
			name:      "empty form uses defaults",
			formData:  url.Values{},
			wantYear:  true,
			wantMonth: 0, // 0 means check it's current month
			wantDay:   0, // 0 means check it's current day
		},
		{
			name:      "invalid values are ignored",
			formData:  url.Values{"month": {"abc"}, "day": {"xyz"}},
			wantYear:  true,
			wantMonth: 0,
			wantDay:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseDateParams(tt.formData)

			if tt.wantYear && result.Year == 0 {
				t.Error("Year should not be zero")
			}

			if tt.wantMonth != 0 && result.Month != tt.wantMonth {
				t.Errorf("Month = %d, want %d", result.Month, tt.wantMonth)
			}

			if tt.wantDay != 0 && result.Day != tt.wantDay {
				t.Errorf("Day = %d, want %d", result.Day, tt.wantDay)
			}
		})
	}
}

func TestParseMonthParams(t *testing.T) {
	tests := []struct {
		name      string
		query     url.Values
		wantYear  int
		wantMonth int
	}{
		{
			name:      "both values provided",
			query:     url.Values{"year": {"2024"}, "month": {"12"}},
			wantYear:  2024,
			wantMonth: 12,
		},
		{
			name:      "only year",
			query:     url.Values{"year": {"2023"}},
			wantYear:  2023,
			wantMonth: 0, // will be current month
		},
		{
			name:      "only month",
			query:     url.Values{"month": {"5"}},
			wantYear:  0, // will be current year
			wantMonth: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseMonthParams(tt.query)

			if tt.wantYear != 0 && result.Year != tt.wantYear {
				t.Errorf("Year = %d, want %d", result.Year, tt.wantYear)
			}

			if tt.wantMonth != 0 && result.Month != tt.wantMonth {
				t.Errorf("Month = %d, want %d", result.Month, tt.wantMonth)
			}
		})
	}
}

func TestRequestBodyParser_JSON(t *testing.T) {
	body := `{"id": "123", "name": "test", "amount": 42.5}`
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	parser := NewRequestBodyParser(req)
	err := parser.Parse()
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if !parser.IsJSON() {
		t.Error("Expected IsJSON() to be true")
	}

	if id := parser.Get("id"); id != "123" {
		t.Errorf("Get('id') = %q, want '123'", id)
	}

	if name := parser.Get("name"); name != "test" {
		t.Errorf("Get('name') = %q, want 'test'", name)
	}

	if amount := parser.Get("amount"); amount != "42.5" {
		t.Errorf("Get('amount') = %q, want '42.5'", amount)
	}
}

func TestRequestBodyParser_FormData(t *testing.T) {
	body := "id=456&name=form+test&value=100"
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	parser := NewRequestBodyParser(req)
	err := parser.Parse()
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if parser.IsJSON() {
		t.Error("Expected IsJSON() to be false for form data")
	}

	if id := parser.Get("id"); id != "456" {
		t.Errorf("Get('id') = %q, want '456'", id)
	}

	if name := parser.Get("name"); name != "form test" {
		t.Errorf("Get('name') = %q, want 'form test'", name)
	}
}

func TestRequestBodyParser_EmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(""))

	parser := NewRequestBodyParser(req)
	err := parser.Parse()
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if val := parser.Get("nonexistent"); val != "" {
		t.Errorf("Get('nonexistent') = %q, want empty string", val)
	}
}

func TestRequireMethod(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		allowed []string
		wantErr bool
	}{
		{"POST allowed", http.MethodPost, []string{http.MethodPost}, false},
		{"DELETE allowed with multiple", http.MethodDelete, []string{http.MethodDelete, http.MethodPost}, false},
		{"GET not allowed", http.MethodGet, []string{http.MethodPost}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			result := RequireMethod(req, tt.allowed...)

			if tt.wantErr && result == nil {
				t.Error("Expected error response but got nil")
			}
			if !tt.wantErr && result != nil {
				t.Error("Expected nil but got error response")
			}
		})
	}
}

func TestRequirePOST(t *testing.T) {
	postReq := httptest.NewRequest(http.MethodPost, "/test", nil)
	if result := RequirePOST(postReq); result != nil {
		t.Error("RequirePOST should allow POST requests")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/test", nil)
	if result := RequirePOST(getReq); result == nil {
		t.Error("RequirePOST should reject GET requests")
	}
}

func TestRequireDeleteOrPOST(t *testing.T) {
	tests := []struct {
		method  string
		wantErr bool
	}{
		{http.MethodPost, false},
		{http.MethodDelete, false},
		{http.MethodGet, true},
		{http.MethodPut, true},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			result := RequireDeleteOrPOST(req)

			if tt.wantErr && result == nil {
				t.Error("Expected error response but got nil")
			}
			if !tt.wantErr && result != nil {
				t.Error("Expected nil but got error response")
			}
		})
	}
}

func TestParseFormOrFail(t *testing.T) {
	// Valid form request
	body := "field=value"
	req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	result := ParseFormOrFail(req)
	if result != nil {
		t.Error("Expected nil for valid form, got error response")
	}

	// Verify form was parsed
	if req.Form.Get("field") != "value" {
		t.Error("Form was not parsed correctly")
	}
}
