package log

import (
	"context"
	"log/slog"
	"os"
)

// Logger wraps slog.Logger with additional context and structured logging
type Logger struct {
	*slog.Logger
	component string
}

// Config holds logger configuration
type Config struct {
	Level     slog.Level
	Component string
	Handler   slog.Handler
}

// DefaultConfig returns sensible defaults for logging
func DefaultConfig() Config {
	return Config{
		Level:     slog.LevelInfo,
		Component: "app",
		Handler:   slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
	}
}

// New creates a new logger with the given configuration
func New(config Config) *Logger {
	var handler slog.Handler
	if config.Handler != nil {
		handler = config.Handler
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: config.Level,
		})
	}
	
	logger := slog.New(handler)
	
	return &Logger{
		Logger:    logger,
		component: config.Component,
	}
}

// With returns a new logger with the given attributes
func (l *Logger) With(args ...any) *Logger {
	return &Logger{
		Logger:    l.Logger.With(args...),
		component: l.component,
	}
}

// WithComponent returns a new logger with a specific component name
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{
		Logger:    l.Logger.With("component", component),
		component: component,
	}
}

// Info logs at Info level with component context
func (l *Logger) Info(msg string, args ...any) {
	l.Logger.Info(msg, append([]any{"component", l.component}, args...)...)
}

// InfoContext logs at Info level with context and component
func (l *Logger) InfoContext(ctx context.Context, msg string, args ...any) {
	l.Logger.InfoContext(ctx, msg, append([]any{"component", l.component}, args...)...)
}

// Warn logs at Warn level with component context
func (l *Logger) Warn(msg string, args ...any) {
	l.Logger.Warn(msg, append([]any{"component", l.component}, args...)...)
}

// WarnContext logs at Warn level with context and component
func (l *Logger) WarnContext(ctx context.Context, msg string, args ...any) {
	l.Logger.WarnContext(ctx, msg, append([]any{"component", l.component}, args...)...)
}

// Error logs at Error level with component context
func (l *Logger) Error(msg string, args ...any) {
	l.Logger.Error(msg, append([]any{"component", l.component}, args...)...)
}

// ErrorContext logs at Error level with context and component
func (l *Logger) ErrorContext(ctx context.Context, msg string, args ...any) {
	l.Logger.ErrorContext(ctx, msg, append([]any{"component", l.component}, args...)...)
}

// Debug logs at Debug level with component context
func (l *Logger) Debug(msg string, args ...any) {
	l.Logger.Debug(msg, append([]any{"component", l.component}, args...)...)
}

// DebugContext logs at Debug level with context and component
func (l *Logger) DebugContext(ctx context.Context, msg string, args ...any) {
	l.Logger.DebugContext(ctx, msg, append([]any{"component", l.component}, args...)...)
}

// SetDefault sets the default logger for the application
func SetDefault(logger *Logger) {
	slog.SetDefault(logger.Logger)
}

// Component returns the logger's component name
func (l *Logger) Component() string {
	return l.component
}