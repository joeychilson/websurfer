package content

import (
	"strings"
	"testing"
)

func TestTruncate(t *testing.T) {
	t.Run("content fits within max tokens", func(t *testing.T) {
		content := "<html><body><p>Short content</p></body></html>"
		result := Truncate(content, "text/html", 1000)

		if result.Truncated {
			t.Error("content should not be truncated")
		}
		if result.Content != content {
			t.Error("content should be unchanged")
		}
		if result.ReturnedChars != len(content) {
			t.Errorf("ReturnedChars = %d, want %d", result.ReturnedChars, len(content))
		}
	})

	t.Run("content exceeds max tokens", func(t *testing.T) {
		content := strings.Repeat("<p>This is a long paragraph with lots of text.</p>", 100)
		result := Truncate(content, "text/html", 100)

		if !result.Truncated {
			t.Error("content should be truncated")
		}
		if result.ReturnedChars >= len(content) {
			t.Error("returned content should be smaller than original")
		}
		if result.TotalChars != len(content) {
			t.Errorf("TotalChars = %d, want %d", result.TotalChars, len(content))
		}
		if result.ReturnedTokens > 100 {
			t.Errorf("ReturnedTokens = %d, should be <= 100", result.ReturnedTokens)
		}
	})

	t.Run("HTML truncation at closing tag", func(t *testing.T) {
		content := "<p>First paragraph with some content.</p><p>Second paragraph with more text.</p><p>Third paragraph.</p><p>Fourth paragraph.</p>"
		result := Truncate(content, "text/html", 40)

		if !result.Truncated {
			t.Error("content should be truncated")
		}

		if !strings.HasSuffix(result.Content, ">") {
			t.Errorf("truncated content should end with complete tag, got: %s", result.Content)
		}

		if len(result.Content) > 0 && result.Content[len(result.Content)-1] != '>' {
			t.Errorf("truncated content should end at tag boundary, got: %s", result.Content)
		}
	})

	t.Run("plain text truncation at word boundary", func(t *testing.T) {
		content := "The quick brown fox jumps over the lazy dog"
		result := Truncate(content, "text/plain", 6)

		if !result.Truncated {
			t.Error("content should be truncated")
		}

		if strings.HasSuffix(result.Content, "quic") || strings.HasSuffix(result.Content, "brow") {
			t.Errorf("should not cut mid-word, got: %q", result.Content)
		}
	})
}

func TestFindTruncationPoint(t *testing.T) {
	t.Run("HTML boundary", func(t *testing.T) {
		content := "<p>Test</p><p>Content</p><p>Here</p>"
		point := findTruncationPoint(content, "text/html", 20)

		if point <= 0 || point >= len(content) {
			t.Errorf("truncation point %d out of range", point)
		}
	})

	t.Run("word boundary", func(t *testing.T) {
		content := "This is a test sentence"
		point := findWordBoundary(content, 10)

		if point < 5 || point > 15 {
			t.Errorf("word boundary %d not near target 10", point)
		}
	})
}

func TestEstimateTokens(t *testing.T) {
	t.Run("empty content", func(t *testing.T) {
		tokens := EstimateTokens("", "text/html")
		if tokens != 0 {
			t.Errorf("EstimateTokens(\"\") = %d, want 0", tokens)
		}
	})

	t.Run("HTML content", func(t *testing.T) {
		content := strings.Repeat("x", 225)
		tokens := EstimateTokens(content, "text/html")
		if tokens != 100 {
			t.Errorf("EstimateTokens(225 chars HTML) = %d, want 100", tokens)
		}
	})

	t.Run("plain text content", func(t *testing.T) {
		content := strings.Repeat("x", 200)
		tokens := EstimateTokens(content, "text/plain")
		if tokens != 100 {
			t.Errorf("EstimateTokens(200 chars plain) = %d, want 100", tokens)
		}
	})

	t.Run("unknown content type uses default", func(t *testing.T) {
		content := strings.Repeat("x", 300)
		tokens := EstimateTokens(content, "unknown/type")
		if tokens != 100 {
			t.Errorf("EstimateTokens(300 chars unknown) = %d, want 100", tokens)
		}
	})
}
