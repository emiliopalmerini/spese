package log

import (
	"context"
	"log/slog"
	"net/http"
)

// ContextKey type for context keys
type ContextKey string

const (
	// LoggerContextKey is the context key for the logger
	LoggerContextKey ContextKey = "logger"
)

// Middleware creates HTTP middleware that adds a logger to the request context
func Middleware(logger *Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add logger to request context
			ctx := context.WithValue(r.Context(), LoggerContextKey, logger)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// FromContext extracts a logger from the request context
func FromContext(ctx context.Context) *Logger {
	if logger, ok := ctx.Value(LoggerContextKey).(*Logger); ok {
		return logger
	}
	// Return default logger if not found
	return &Logger{
		Logger:    slog.Default(),
		component: "unknown",
	}
}

// ComponentMiddleware creates middleware that adds component context to the logger
func ComponentMiddleware(component string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get logger from context and add component
			logger := FromContext(r.Context()).WithComponent(component)
			
			// Update context with component logger
			ctx := context.WithValue(r.Context(), LoggerContextKey, logger)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequestIDMiddleware adds request ID to logger context
func RequestIDMiddleware(extractRequestID func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := extractRequestID(r)
			
			// Get logger from context and add request ID
			logger := FromContext(r.Context()).With(FieldRequestID, requestID)
			
			// Update context with enriched logger
			ctx := context.WithValue(r.Context(), LoggerContextKey, logger)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// StructuredLogger provides structured logging methods with context awareness
type StructuredLogger struct {
	logger *Logger
}

// NewStructuredLogger creates a new structured logger
func NewStructuredLogger(logger *Logger) *StructuredLogger {
	return &StructuredLogger{
		logger: logger,
	}
}

// LogHTTPStart logs the start of an HTTP request
func (sl *StructuredLogger) LogHTTPStart(ctx context.Context, r *http.Request, clientIP string) {
	fields := NewFields().
		WithHTTPRequest(r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("User-Agent"), r.Header.Get("Referer")).
		WithClientIP(clientIP).
		WithComponent(ComponentHTTP)

	sl.logger.InfoContext(ctx, "HTTP request started", fields.ToSlice()...)
}

// LogHTTPEnd logs the completion of an HTTP request
func (sl *StructuredLogger) LogHTTPEnd(ctx context.Context, r *http.Request, statusCode int, durationMs int64, clientIP string) {
	level := slog.LevelInfo
	if statusCode >= 400 && statusCode < 500 {
		level = slog.LevelWarn
	} else if statusCode >= 500 {
		level = slog.LevelError
	}

	fields := NewFields().
		WithHTTPRequest(r.Method, r.URL.Path, r.URL.RawQuery, "", "").
		WithHTTPResponse(statusCode, durationMs, statusCode < 400).
		WithClientIP(clientIP).
		WithComponent(ComponentHTTP)

	sl.logger.Logger.Log(ctx, level, "HTTP request completed", fields.ToSlice()...)
}

// LogExpenseCreated logs successful expense creation
func (sl *StructuredLogger) LogExpenseCreated(ctx context.Context, desc string, amountCents int64, primary, secondary, ref string) {
	fields := NewFields().
		WithExpense(desc, amountCents, primary, secondary).
		WithOperation(OpCreate).
		WithComponent(ComponentExpense).
		ToSlice()
	
	fields = append(fields, FieldSheetsRef, ref)
	
	sl.logger.InfoContext(ctx, "Expense created successfully", fields...)
}

// LogError logs an error with structured context
func (sl *StructuredLogger) LogError(ctx context.Context, msg string, err error, component string, operation string, fields LogFields) {
	allFields := fields.
		WithError(err).
		WithOperation(operation).
		WithComponent(component)
	
	sl.logger.ErrorContext(ctx, msg, allFields.ToSlice()...)
}