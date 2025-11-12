package content

import (
	"strings"
	"testing"
)

// BenchmarkEstimateTokens measures token estimation performance.
func BenchmarkEstimateTokens(b *testing.B) {
	tests := []struct {
		name        string
		size        int
		contentType string
	}{
		{"small_text", 1_000, "text/plain"},
		{"medium_text", 10_000, "text/plain"},
		{"large_text", 100_000, "text/plain"},
		{"huge_text", 1_000_000, "text/plain"},
		{"small_html", 1_000, "text/html"},
		{"medium_html", 10_000, "text/html"},
		{"large_html", 100_000, "text/html"},
		{"huge_html", 1_000_000, "text/html"},
		{"json", 50_000, "application/json"},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			content := []byte(strings.Repeat("a", tt.size))
			b.SetBytes(int64(len(content)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = EstimateTokens(content, tt.contentType)
			}
		})
	}
}

// BenchmarkCharsForTokens measures character calculation performance.
func BenchmarkCharsForTokens(b *testing.B) {
	contentTypes := []string{
		"text/plain",
		"text/html",
		"application/json",
		"application/xml",
	}

	for _, ct := range contentTypes {
		b.Run(ct, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = charsForTokens(1000, ct)
			}
		})
	}
}
