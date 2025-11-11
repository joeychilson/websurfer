package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
)

// Logger is the interface for structured logging.
type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	With(args ...any) Logger
	WithContext(ctx context.Context) Logger
}

// Level represents the log level.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String returns the string representation of the level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// toSlogLevel converts our Level to slog.Level.
func (l Level) toSlogLevel() slog.Level {
	switch l {
	case LevelDebug:
		return slog.LevelDebug
	case LevelInfo:
		return slog.LevelInfo
	case LevelWarn:
		return slog.LevelWarn
	case LevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// New creates a new logger using the standard library's slog.
func New(handler slog.Handler) Logger {
	if handler == nil {
		handler = slog.Default().Handler()
	}
	return &slogLogger{
		logger: slog.New(handler),
	}
}

// NewWithLevel creates a new logger with the specified minimum level.
func NewWithLevel(level Level) Logger {
	opts := &slog.HandlerOptions{
		Level: level.toSlogLevel(),
	}
	handler := slog.NewJSONHandler(os.Stderr, opts)
	return &slogLogger{
		logger: slog.New(handler),
	}
}

// NewText creates a new logger with text output instead of JSON.
func NewText(writer io.Writer, level Level) Logger {
	opts := &slog.HandlerOptions{
		Level: level.toSlogLevel(),
	}
	if writer == nil {
		writer = os.Stderr
	}
	handler := slog.NewTextHandler(writer, opts)
	return &slogLogger{
		logger: slog.New(handler),
	}
}

// Default returns a default logger (Info level, JSON output).
func Default() Logger {
	return NewWithLevel(LevelInfo)
}

// Noop returns a no-op logger that discards all log messages.
func Noop() Logger {
	return &noopLogger{}
}
