package logger

import (
	"context"
	"log/slog"
)

// slogLogger is an adapter that wraps slog.Logger to implement our Logger interface.
type slogLogger struct {
	logger *slog.Logger
}

// Debug logs a debug message with optional key-value pairs.
func (l *slogLogger) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

// Info logs an informational message with optional key-value pairs.
func (l *slogLogger) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

// Warn logs a warning message with optional key-value pairs.
func (l *slogLogger) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

// Error logs an error message with optional key-value pairs.
func (l *slogLogger) Error(msg string, args ...any) {
	l.logger.Error(msg, args...)
}

// With returns a new logger with the given key-value pairs added to all log messages.
func (l *slogLogger) With(args ...any) Logger {
	return &slogLogger{
		logger: l.logger.With(args...),
	}
}

// WithContext returns a new logger with context.
// For slog, this is a no-op as context is passed to individual log calls.
func (l *slogLogger) WithContext(ctx context.Context) Logger {
	return l
}
