package logger

import "context"

// noopLogger is a logger that discards all log messages.
type noopLogger struct{}

// Debug does nothing.
func (n *noopLogger) Debug(msg string, args ...any) {}

// Info does nothing.
func (n *noopLogger) Info(msg string, args ...any) {}

// Warn does nothing.
func (n *noopLogger) Warn(msg string, args ...any) {}

// Error does nothing.
func (n *noopLogger) Error(msg string, args ...any) {}

// With returns the same noop logger.
func (n *noopLogger) With(args ...any) Logger {
	return n
}

// WithContext returns the same noop logger.
func (n *noopLogger) WithContext(ctx context.Context) Logger {
	return n
}
