package content

import (
	"testing"
)

func TestExtractRange(t *testing.T) {
	content := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"

	t.Run("extract char range", func(t *testing.T) {
		opts := &RangeOptions{
			Type:  "chars",
			Start: 0,
			End:   6,
		}

		result, err := ExtractRange(content, opts)
		if err != nil {
			t.Fatalf("ExtractRange() error = %v", err)
		}

		if result != "Line 1" {
			t.Errorf("ExtractRange() = %q, want %q", result, "Line 1")
		}
	})

	t.Run("extract line range", func(t *testing.T) {
		opts := &RangeOptions{
			Type:  "lines",
			Start: 1,
			End:   3,
		}

		result, err := ExtractRange(content, opts)
		if err != nil {
			t.Fatalf("ExtractRange() error = %v", err)
		}

		expected := "Line 2\nLine 3"
		if result != expected {
			t.Errorf("ExtractRange() = %q, want %q", result, expected)
		}
	})

	t.Run("char range beyond content", func(t *testing.T) {
		opts := &RangeOptions{
			Type:  "chars",
			Start: 0,
			End:   1000,
		}

		result, err := ExtractRange(content, opts)
		if err != nil {
			t.Fatalf("ExtractRange() error = %v", err)
		}

		if result != content {
			t.Error("should return full content when end exceeds length")
		}
	})

	t.Run("line range beyond content", func(t *testing.T) {
		opts := &RangeOptions{
			Type:  "lines",
			Start: 2,
			End:   100,
		}

		result, err := ExtractRange(content, opts)
		if err != nil {
			t.Fatalf("ExtractRange() error = %v", err)
		}

		expected := "Line 3\nLine 4\nLine 5"
		if result != expected {
			t.Errorf("ExtractRange() = %q, want %q", result, expected)
		}
	})

	t.Run("invalid range type", func(t *testing.T) {
		opts := &RangeOptions{
			Type:  "invalid",
			Start: 0,
			End:   10,
		}

		_, err := ExtractRange(content, opts)
		if err == nil {
			t.Error("ExtractRange() should return error for invalid type")
		}
	})

	t.Run("invalid range bounds", func(t *testing.T) {
		opts := &RangeOptions{
			Type:  "chars",
			Start: 10,
			End:   5,
		}

		_, err := ExtractRange(content, opts)
		if err == nil {
			t.Error("ExtractRange() should return error when start > end")
		}
	})

	t.Run("start beyond content", func(t *testing.T) {
		opts := &RangeOptions{
			Type:  "chars",
			Start: 1000,
			End:   2000,
		}

		_, err := ExtractRange(content, opts)
		if err == nil {
			t.Error("ExtractRange() should return error when start exceeds content length")
		}
	})

	t.Run("nil options returns full content", func(t *testing.T) {
		result, err := ExtractRange(content, nil)
		if err != nil {
			t.Fatalf("ExtractRange() error = %v", err)
		}

		if result != content {
			t.Error("nil options should return full content")
		}
	})
}
