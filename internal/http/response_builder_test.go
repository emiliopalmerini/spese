package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTMXResponseBuilder_Basic(t *testing.T) {
	w := httptest.NewRecorder()

	NewHTMXResponse().
		Status(http.StatusOK).
		BodyString("test").
		Write(w)

	if w.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "test" {
		t.Errorf("Body = %q, want %q", w.Body.String(), "test")
	}
}

func TestHTMXResponseBuilder_Triggers(t *testing.T) {
	w := httptest.NewRecorder()

	NewHTMXResponse().
		TriggerExpenseCreated(2024, 1).
		TriggerFormReset().
		TriggerOverviewRefresh(2024, 1).
		TriggerSuccessNotification("Test message").
		Write(w)

	trigger := w.Header().Get("HX-Trigger")
	if trigger == "" {
		t.Fatal("HX-Trigger header not set")
	}

	// Verify trigger contains expected events
	expectedParts := []string{
		`"expense:created"`,
		`"form:reset"`,
		`"overview:refresh"`,
		`"show-notification"`,
		`"year":2024`,
		`"month":1`,
		`"type":"success"`,
	}
	for _, part := range expectedParts {
		if !strings.Contains(trigger, part) {
			t.Errorf("HX-Trigger missing %q: %s", part, trigger)
		}
	}
}

func TestHTMXResponseBuilder_IncomeCreated(t *testing.T) {
	w := httptest.NewRecorder()

	NewHTMXResponse().
		TriggerIncomeCreated(2024, 6).
		TriggerFormReset().
		TriggerIncomeOverviewRefresh(2024, 6).
		Write(w)

	trigger := w.Header().Get("HX-Trigger")
	if !strings.Contains(trigger, `"income:created"`) {
		t.Errorf("Missing income:created trigger: %s", trigger)
	}
	if !strings.Contains(trigger, `"income-overview:refresh"`) {
		t.Errorf("Missing income-overview:refresh trigger: %s", trigger)
	}
}

func TestHTMXResponseBuilder_CustomHeader(t *testing.T) {
	w := httptest.NewRecorder()

	NewHTMXResponse().
		Header("X-Custom", "value").
		Status(http.StatusCreated).
		Write(w)

	if w.Header().Get("X-Custom") != "value" {
		t.Errorf("Custom header not set")
	}
	if w.Code != http.StatusCreated {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusCreated)
	}
}

func TestErrorResponse(t *testing.T) {
	tests := []struct {
		name       string
		builder    *HTMXResponseBuilder
		wantStatus int
		wantBody   string
	}{
		{
			name:       "bad request",
			builder:    BadRequestError("Invalid input"),
			wantStatus: http.StatusBadRequest,
			wantBody:   `<div class="error">Invalid input</div>`,
		},
		{
			name:       "unprocessable entity",
			builder:    UnprocessableEntityError("Validation failed"),
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   `<div class="error">Validation failed</div>`,
		},
		{
			name:       "internal server error",
			builder:    InternalServerError("Something broke"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   `<div class="error">Something broke</div>`,
		},
		{
			name:       "not found",
			builder:    NotFoundError("Resource not found"),
			wantStatus: http.StatusNotFound,
			wantBody:   `<div class="error">Resource not found</div>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tt.builder.Write(w)

			if w.Code != tt.wantStatus {
				t.Errorf("Status code = %d, want %d", w.Code, tt.wantStatus)
			}
			if w.Body.String() != tt.wantBody {
				t.Errorf("Body = %q, want %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestErrorResponse_EscapesHTML(t *testing.T) {
	w := httptest.NewRecorder()

	BadRequestError("<script>alert('xss')</script>").Write(w)

	body := w.Body.String()
	if strings.Contains(body, "<script>") {
		t.Error("Error response did not escape HTML")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("Error response did not properly escape HTML entities")
	}
}

func TestMethodNotAllowedError(t *testing.T) {
	w := httptest.NewRecorder()

	MethodNotAllowedError("GET, POST").Write(w)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
	if w.Header().Get("Allow") != "GET, POST" {
		t.Errorf("Allow header = %q, want %q", w.Header().Get("Allow"), "GET, POST")
	}
}

func TestNotificationTypes(t *testing.T) {
	tests := []struct {
		notifType NotificationType
		want      string
	}{
		{NotificationSuccess, "success"},
		{NotificationError, "error"},
		{NotificationWarning, "warning"},
		{NotificationInfo, "info"},
	}

	for _, tt := range tests {
		w := httptest.NewRecorder()
		NewHTMXResponse().
			TriggerNotification(tt.notifType, "test", 1000).
			Write(w)

		trigger := w.Header().Get("HX-Trigger")
		if !strings.Contains(trigger, `"type":"`+tt.want+`"`) {
			t.Errorf("Notification type %q not found in trigger: %s", tt.want, trigger)
		}
	}
}
