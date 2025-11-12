package content

import (
	"strings"
	"testing"
)

// BenchmarkTruncate measures truncation performance on different content sizes.
func BenchmarkTruncate(b *testing.B) {
	tests := []struct {
		name        string
		size        int
		contentType string
		maxTokens   int
	}{
		{"small_text", 1_000, "text/plain", 100},
		{"medium_text", 10_000, "text/plain", 500},
		{"large_text", 100_000, "text/plain", 2000},
		{"small_html", 1_000, "text/html", 100},
		{"medium_html", 10_000, "text/html", 500},
		{"large_html", 100_000, "text/html", 2000},
		{"huge_html", 1_000_000, "text/html", 5000},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			// Generate content
			var content []byte
			if strings.Contains(tt.contentType, "html") {
				content = generateHTMLContent(tt.size)
			} else {
				content = []byte(strings.Repeat("word ", tt.size/5))
			}

			b.SetBytes(int64(len(content)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = Truncate(content, tt.contentType, tt.maxTokens)
			}
		})
	}
}

// BenchmarkFindHTMLBoundary measures HTML boundary detection performance.
func BenchmarkFindHTMLBoundary(b *testing.B) {
	sizes := []int{1_000, 10_000, 100_000}

	for _, size := range sizes {
		b.Run(formatBytes(size), func(b *testing.B) {
			content := generateHTMLContent(size)
			target := len(content) / 2

			b.SetBytes(int64(len(content)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = findHTMLBoundary(content, target)
			}
		})
	}
}

// BenchmarkFindWordBoundary measures word boundary detection performance.
func BenchmarkFindWordBoundary(b *testing.B) {
	sizes := []int{1_000, 10_000, 100_000}

	for _, size := range sizes {
		b.Run(formatBytes(size), func(b *testing.B) {
			content := []byte(strings.Repeat("word ", size/5))
			target := len(content) / 2

			b.SetBytes(int64(len(content)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_ = findWordBoundary(content, target)
			}
		})
	}
}

func generateHTMLContent(approxSize int) []byte {
	var sb strings.Builder
	sb.WriteString("<html><body>")

	for sb.Len() < approxSize {
		sb.WriteString("<p>This is a paragraph with some text content.</p>\n")
		sb.WriteString("<div><span>Nested content here</span></div>\n")
		sb.WriteString("<ul><li>List item 1</li><li>List item 2</li></ul>\n")
	}

	sb.WriteString("</body></html>")
	return []byte(sb.String())
}

func formatBytes(size int) string {
	if size >= 100_000 {
		return "100KB"
	} else if size >= 10_000 {
		return "10KB"
	}
	return "1KB"
}
