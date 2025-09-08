package trace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"
)

// ContextKey type for context keys
type ContextKey string

const (
	// RequestIDKey is the context key for request ID
	RequestIDKey ContextKey = "request_id"
)

// Middleware handles request tracing and logging
type Middleware struct {
	extractIP func(*http.Request) string
	metrics   *Metrics
}

// Metrics tracks request metrics
type Metrics struct {
	TotalRequests       int64
	AverageResponseTime int64 // in microseconds
}

// NewMiddleware creates a new trace middleware
func NewMiddleware(extractIP func(*http.Request) string) *Middleware {
	return &Middleware{
		extractIP: extractIP,
		metrics:   &Metrics{},
	}
}

// Middleware returns HTTP middleware for request tracing
func (m *Middleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Extract client IP
		clientIP := ""
		if m.extractIP != nil {
			clientIP = m.extractIP(r)
		}
		
		// Generate request ID for tracing
		requestID := GenerateRequestID()
		
		// Add request context with metadata and request ID
		ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
		r = r.WithContext(ctx)
		
		// Enhanced structured request logging
		slog.InfoContext(ctx, "HTTP request started", 
			"request_id", requestID,
			"method", r.Method, 
			"path", r.URL.Path, 
			"query", r.URL.RawQuery,
			"client_ip", clientIP, 
			"user_agent", r.Header.Get("User-Agent"),
			"referer", r.Header.Get("Referer"),
			"content_length", r.ContentLength,
			"protocol", r.Proto)
		
		// Track total requests
		atomic.AddInt64(&m.metrics.TotalRequests, 1)
		
		// Create a custom response writer to capture status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)
		
		// Enhanced request completion logging
		duration := time.Since(start)
		durationMs := duration.Milliseconds()
		
		// Update average response time
		atomic.StoreInt64(&m.metrics.AverageResponseTime, durationMs*1000) // convert to microseconds
		
		// Use appropriate log level based on status code
		logLevel := slog.LevelInfo
		if rw.statusCode >= 400 && rw.statusCode < 500 {
			logLevel = slog.LevelWarn
		} else if rw.statusCode >= 500 {
			logLevel = slog.LevelError
		}
		
		slog.Log(ctx, logLevel, "HTTP request completed", 
			"request_id", requestID,
			"method", r.Method, 
			"path", r.URL.Path, 
			"query", r.URL.RawQuery,
			"status_code", rw.statusCode,
			"duration_ms", durationMs,
			"duration_human", duration.String(),
			"client_ip", clientIP,
			"success", rw.statusCode < 400)
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// GenerateRequestID creates a unique request ID for tracing
func GenerateRequestID() string {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp if random fails
		return fmt.Sprintf("req_%d", time.Now().UnixNano())
	}
	return "req_" + hex.EncodeToString(bytes)
}

// GetRequestID extracts the request ID from context
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// GetMetrics returns current metrics
func (m *Middleware) GetMetrics() Metrics {
	return Metrics{
		TotalRequests:       atomic.LoadInt64(&m.metrics.TotalRequests),
		AverageResponseTime: atomic.LoadInt64(&m.metrics.AverageResponseTime),
	}
}

// LoggerMiddleware creates middleware that adds structured context to logs
func LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add request ID to all log calls made during this request
		ctx := r.Context()
		requestID := GetRequestID(ctx)
		
		// Create a logger with request context
		logger := slog.With(
			"request_id", requestID,
			"method", r.Method,
			"path", r.URL.Path,
		)
		
		// Replace the default logger for this request context
		ctx = context.WithValue(ctx, "logger", logger)
		r = r.WithContext(ctx)
		
		next.ServeHTTP(w, r)
	})
}