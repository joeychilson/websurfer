package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestSlogLogger(t *testing.T) {
	t.Run("logs at different levels", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
		logger := New(handler)

		logger.Debug("debug message", "key", "value")
		logger.Info("info message", "key", "value")
		logger.Warn("warn message", "key", "value")
		logger.Error("error message", "key", "value")

		output := buf.String()
		if !strings.Contains(output, "debug message") {
			t.Error("output should contain debug message")
		}
		if !strings.Contains(output, "info message") {
			t.Error("output should contain info message")
		}
		if !strings.Contains(output, "warn message") {
			t.Error("output should contain warn message")
		}
		if !strings.Contains(output, "error message") {
			t.Error("output should contain error message")
		}
	})

	t.Run("respects log level", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
		logger := New(handler)

		logger.Debug("debug message")
		logger.Info("info message")
		logger.Warn("warn message")
		logger.Error("error message")

		output := buf.String()
		if strings.Contains(output, "debug message") {
			t.Error("output should not contain debug message when level is Warn")
		}
		if strings.Contains(output, "info message") {
			t.Error("output should not contain info message when level is Warn")
		}
		if !strings.Contains(output, "warn message") {
			t.Error("output should contain warn message")
		}
		if !strings.Contains(output, "error message") {
			t.Error("output should contain error message")
		}
	})

	t.Run("logs with key-value pairs", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		logger := New(handler)

		logger.Info("test message", "user", "alice", "count", 42)

		var logEntry map[string]any
		if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
			t.Fatalf("failed to unmarshal log entry: %v", err)
		}

		if logEntry["msg"] != "test message" {
			t.Errorf("msg = %v, want 'test message'", logEntry["msg"])
		}
		if logEntry["user"] != "alice" {
			t.Errorf("user = %v, want 'alice'", logEntry["user"])
		}
		if logEntry["count"] != float64(42) {
			t.Errorf("count = %v, want 42", logEntry["count"])
		}
	})

	t.Run("With adds context to all logs", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		logger := New(handler).With("request_id", "123", "service", "test")

		logger.Info("first message")
		logger.Info("second message")

		output := buf.String()
		lines := strings.Split(strings.TrimSpace(output), "\n")

		if len(lines) != 2 {
			t.Fatalf("expected 2 log lines, got %d", len(lines))
		}

		for i, line := range lines {
			var logEntry map[string]any
			if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
				t.Fatalf("failed to unmarshal log entry %d: %v", i, err)
			}

			if logEntry["request_id"] != "123" {
				t.Errorf("line %d: request_id = %v, want '123'", i, logEntry["request_id"])
			}
			if logEntry["service"] != "test" {
				t.Errorf("line %d: service = %v, want 'test'", i, logEntry["service"])
			}
		}
	})

	t.Run("WithContext returns logger", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		logger := New(handler)

		ctx := context.Background()
		contextLogger := logger.WithContext(ctx)

		if contextLogger == nil {
			t.Error("WithContext should return a logger")
		}

		// Should still be able to log
		contextLogger.Info("test message")
		if !strings.Contains(buf.String(), "test message") {
			t.Error("contextLogger should be able to log")
		}
	})
}

func TestNoopLogger(t *testing.T) {
	t.Run("discards all log messages", func(t *testing.T) {
		logger := Noop()

		// These should not panic
		logger.Debug("debug message", "key", "value")
		logger.Info("info message", "key", "value")
		logger.Warn("warn message", "key", "value")
		logger.Error("error message", "key", "value")
	})

	t.Run("With returns same logger", func(t *testing.T) {
		logger := Noop()
		withLogger := logger.With("key", "value")

		// Should return same instance
		if withLogger != logger {
			t.Error("With should return same noop logger instance")
		}
	})

	t.Run("WithContext returns same logger", func(t *testing.T) {
		logger := Noop()
		ctx := context.Background()
		contextLogger := logger.WithContext(ctx)

		// Should return same instance
		if contextLogger != logger {
			t.Error("WithContext should return same noop logger instance")
		}
	})
}

func TestLevel(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{LevelDebug, "DEBUG"},
		{LevelInfo, "INFO"},
		{LevelWarn, "WARN"},
		{LevelError, "ERROR"},
		{Level(999), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.level.String(); got != tt.expected {
				t.Errorf("Level.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLevelToSlogLevel(t *testing.T) {
	tests := []struct {
		level    Level
		expected slog.Level
	}{
		{LevelDebug, slog.LevelDebug},
		{LevelInfo, slog.LevelInfo},
		{LevelWarn, slog.LevelWarn},
		{LevelError, slog.LevelError},
		{Level(999), slog.LevelInfo},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			if got := tt.level.toSlogLevel(); got != tt.expected {
				t.Errorf("Level.toSlogLevel() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNew(t *testing.T) {
	t.Run("with nil handler uses default", func(t *testing.T) {
		logger := New(nil)
		if logger == nil {
			t.Fatal("New(nil) should return a logger")
		}

		// Should be able to log
		logger.Info("test message")
	})

	t.Run("with custom handler", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		logger := New(handler)

		logger.Info("test message")

		if !strings.Contains(buf.String(), "test message") {
			t.Error("custom handler should be used")
		}
	})
}

func TestNewWithLevel(t *testing.T) {
	t.Run("creates logger with Info level", func(t *testing.T) {
		logger := NewWithLevel(LevelInfo)
		if logger == nil {
			t.Fatal("NewWithLevel should return a logger")
		}
	})

	t.Run("creates logger with Debug level", func(t *testing.T) {
		logger := NewWithLevel(LevelDebug)
		if logger == nil {
			t.Fatal("NewWithLevel should return a logger")
		}
	})
}

func TestNewText(t *testing.T) {
	t.Run("creates text logger", func(t *testing.T) {
		var buf bytes.Buffer
		logger := NewText(&buf, LevelInfo)

		logger.Info("test message", "key", "value")

		output := buf.String()
		if !strings.Contains(output, "test message") {
			t.Error("output should contain message")
		}
		if !strings.Contains(output, "key=value") {
			t.Error("output should contain key=value pair")
		}
	})

	t.Run("with nil writer uses stderr", func(t *testing.T) {
		logger := NewText(nil, LevelInfo)
		if logger == nil {
			t.Fatal("NewText with nil writer should return a logger")
		}

		// Should not panic
		logger.Info("test message")
	})
}

func TestDefault(t *testing.T) {
	logger := Default()
	if logger == nil {
		t.Fatal("Default() should return a logger")
	}

	// Should not panic
	logger.Info("test message")
}
