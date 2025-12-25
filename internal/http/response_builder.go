// Package http provides HTTP server and handler implementations.
//
// This file implements the Builder Pattern for constructing HTMX responses.
// It provides a type-safe, fluent API for building HX-Trigger headers and
// consistent response formatting.

package http

import (
	"encoding/json"
	"html/template"
	"net/http"
)

// HTMXResponseBuilder provides a fluent API for building HTMX responses.
// It encapsulates the construction of HX-Trigger headers and response bodies.
type HTMXResponseBuilder struct {
	triggers   map[string]interface{}
	statusCode int
	body       []byte
	headers    map[string]string
}

// NewHTMXResponse creates a new response builder with default 200 status.
func NewHTMXResponse() *HTMXResponseBuilder {
	return &HTMXResponseBuilder{
		triggers:   make(map[string]interface{}),
		statusCode: http.StatusOK,
		headers:    make(map[string]string),
	}
}

// Status sets the HTTP status code for the response.
func (b *HTMXResponseBuilder) Status(code int) *HTMXResponseBuilder {
	b.statusCode = code
	return b
}

// Trigger adds a named trigger with optional data to the HX-Trigger header.
func (b *HTMXResponseBuilder) Trigger(name string, data interface{}) *HTMXResponseBuilder {
	b.triggers[name] = data
	return b
}

// TriggerExpenseCreated adds the expense:created trigger with year/month data.
func (b *HTMXResponseBuilder) TriggerExpenseCreated(year, month int) *HTMXResponseBuilder {
	return b.Trigger("expense:created", map[string]int{"year": year, "month": month})
}

// TriggerIncomeCreated adds the income:created trigger with year/month data.
func (b *HTMXResponseBuilder) TriggerIncomeCreated(year, month int) *HTMXResponseBuilder {
	return b.Trigger("income:created", map[string]int{"year": year, "month": month})
}

// TriggerExpenseDeleted adds the expense:deleted trigger with year/month data.
func (b *HTMXResponseBuilder) TriggerExpenseDeleted(year, month int) *HTMXResponseBuilder {
	return b.Trigger("expense:deleted", map[string]int{"year": year, "month": month})
}

// TriggerIncomeDeleted adds the income:deleted trigger with year/month data.
func (b *HTMXResponseBuilder) TriggerIncomeDeleted(year, month int) *HTMXResponseBuilder {
	return b.Trigger("income:deleted", map[string]int{"year": year, "month": month})
}

// TriggerFormReset adds the form:reset trigger.
func (b *HTMXResponseBuilder) TriggerFormReset() *HTMXResponseBuilder {
	return b.Trigger("form:reset", struct{}{})
}

// TriggerOverviewRefresh adds the overview:refresh trigger with year/month data.
func (b *HTMXResponseBuilder) TriggerOverviewRefresh(year, month int) *HTMXResponseBuilder {
	return b.Trigger("overview:refresh", map[string]int{"year": year, "month": month})
}

// TriggerIncomeOverviewRefresh adds the income-overview:refresh trigger with year/month data.
func (b *HTMXResponseBuilder) TriggerIncomeOverviewRefresh(year, month int) *HTMXResponseBuilder {
	return b.Trigger("income-overview:refresh", map[string]int{"year": year, "month": month})
}

// NotificationType represents the type of notification to display.
type NotificationType string

const (
	NotificationSuccess NotificationType = "success"
	NotificationError   NotificationType = "error"
	NotificationWarning NotificationType = "warning"
	NotificationInfo    NotificationType = "info"
)

// TriggerNotification adds a show-notification trigger with the specified parameters.
func (b *HTMXResponseBuilder) TriggerNotification(notifType NotificationType, message string, durationMs int) *HTMXResponseBuilder {
	return b.Trigger("show-notification", map[string]interface{}{
		"type":     string(notifType),
		"message":  message,
		"duration": durationMs,
	})
}

// TriggerSuccessNotification is a convenience method for success notifications.
func (b *HTMXResponseBuilder) TriggerSuccessNotification(message string) *HTMXResponseBuilder {
	return b.TriggerNotification(NotificationSuccess, message, 3000)
}

// TriggerErrorNotification is a convenience method for error notifications.
func (b *HTMXResponseBuilder) TriggerErrorNotification(message string) *HTMXResponseBuilder {
	return b.TriggerNotification(NotificationError, message, 5000)
}

// Header adds a custom header to the response.
func (b *HTMXResponseBuilder) Header(name, value string) *HTMXResponseBuilder {
	b.headers[name] = value
	return b
}

// Body sets the response body as bytes.
func (b *HTMXResponseBuilder) Body(content []byte) *HTMXResponseBuilder {
	b.body = content
	return b
}

// BodyString sets the response body as a string.
func (b *HTMXResponseBuilder) BodyString(content string) *HTMXResponseBuilder {
	b.body = []byte(content)
	return b
}

// BodyHTML sets the response body as HTML content.
func (b *HTMXResponseBuilder) BodyHTML(html string) *HTMXResponseBuilder {
	b.headers["Content-Type"] = "text/html; charset=utf-8"
	b.body = []byte(html)
	return b
}

// Write sends the built response to the http.ResponseWriter.
func (b *HTMXResponseBuilder) Write(w http.ResponseWriter) {
	// Set custom headers
	for name, value := range b.headers {
		w.Header().Set(name, value)
	}

	// Build and set HX-Trigger header if there are triggers
	if len(b.triggers) > 0 {
		triggerJSON, err := json.Marshal(b.triggers)
		if err == nil {
			w.Header().Set("HX-Trigger", string(triggerJSON))
		}
	}

	// Write status code and body
	w.WriteHeader(b.statusCode)
	if len(b.body) > 0 {
		_, _ = w.Write(b.body)
	}
}

// ErrorResponse creates a standard error response with HTML formatting.
// The message is HTML-escaped for safety.
func ErrorResponse(statusCode int, message string) *HTMXResponseBuilder {
	escapedMsg := template.HTMLEscapeString(message)
	return NewHTMXResponse().
		Status(statusCode).
		BodyHTML(`<div class="error">` + escapedMsg + `</div>`)
}

// BadRequestError creates a 400 Bad Request error response.
func BadRequestError(message string) *HTMXResponseBuilder {
	return ErrorResponse(http.StatusBadRequest, message)
}

// UnprocessableEntityError creates a 422 Unprocessable Entity error response.
func UnprocessableEntityError(message string) *HTMXResponseBuilder {
	return ErrorResponse(http.StatusUnprocessableEntity, message)
}

// InternalServerError creates a 500 Internal Server Error response.
func InternalServerError(message string) *HTMXResponseBuilder {
	return ErrorResponse(http.StatusInternalServerError, message)
}

// NotFoundError creates a 404 Not Found error response.
func NotFoundError(message string) *HTMXResponseBuilder {
	return ErrorResponse(http.StatusNotFound, message)
}

// MethodNotAllowedError creates a 405 Method Not Allowed error response.
func MethodNotAllowedError(allowedMethods string) *HTMXResponseBuilder {
	return NewHTMXResponse().
		Status(http.StatusMethodNotAllowed).
		Header("Allow", allowedMethods)
}
