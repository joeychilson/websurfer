package content

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEstimateTokensEmpty verifies zero tokens for empty content.
func TestEstimateTokensEmpty(t *testing.T) {
	result := EstimateTokens([]byte(""), "text/plain")
	assert.Equal(t, 0, result, "empty content should have 0 tokens")
}

// TestEstimateTokensPlainText verifies token estimation for plain text.
func TestEstimateTokensPlainText(t *testing.T) {
	// Plain text ratio is 1.7 chars per token
	content := []byte(strings.Repeat("a", 170)) // Should be ~100 tokens
	tokens := EstimateTokens(content, "text/plain")

	// Should be approximately 100 tokens (170 / 1.7)
	assert.InDelta(t, 100, tokens, 5, "plain text estimation should be accurate")
}

// TestEstimateTokensHTML verifies token estimation for HTML.
func TestEstimateTokensHTML(t *testing.T) {
	// HTML ratio is 1.9 chars per token
	content := []byte(strings.Repeat("a", 190)) // Should be ~100 tokens
	tokens := EstimateTokens(content, "text/html")

	// Should be approximately 100 tokens (190 / 1.9)
	assert.InDelta(t, 100, tokens, 5, "HTML estimation should be accurate")
}

// TestEstimateTokensJSON verifies token estimation for JSON.
func TestEstimateTokensJSON(t *testing.T) {
	// JSON ratio is 2.5 chars per token
	content := []byte(strings.Repeat("a", 250)) // Should be ~100 tokens
	tokens := EstimateTokens(content, "application/json")

	// Should be approximately 100 tokens (250 / 2.5)
	assert.InDelta(t, 100, tokens, 5, "JSON estimation should be accurate")
}

// TestEstimateTokensUnknownContentType verifies default ratio is used.
func TestEstimateTokensUnknownContentType(t *testing.T) {
	// Unknown type should use default (2.5)
	content := []byte(strings.Repeat("a", 250))
	tokens := EstimateTokens(content, "application/unknown")

	// Should use default ratio
	assert.InDelta(t, 100, tokens, 5, "unknown content type should use default ratio")
}

// TestEstimateTokensContentTypeWithParams verifies content type normalization.
func TestEstimateTokensContentTypeWithParams(t *testing.T) {
	content := []byte(strings.Repeat("a", 190))

	// With charset parameter
	tokens := EstimateTokens(content, "text/html; charset=utf-8")
	assert.InDelta(t, 100, tokens, 5, "should handle content type with parameters")
}

// TestEstimateTokensConsistency verifies estimation is consistent.
func TestEstimateTokensConsistency(t *testing.T) {
	content := []byte("This is test content for consistency check")

	// Run estimation multiple times
	first := EstimateTokens(content, "text/plain")
	for i := 0; i < 10; i++ {
		result := EstimateTokens(content, "text/plain")
		assert.Equal(t, first, result, "estimation should be consistent")
	}
}

// TestCharsForTokens verifies character calculation from token count.
func TestCharsForTokens(t *testing.T) {
	tests := []struct {
		name        string
		tokens      int
		contentType string
		expected    int
	}{
		{"plain_text", 100, "text/plain", 170},                        // 100 * 1.7
		{"html", 100, "text/html", 190},                               // 100 * 1.9
		{"json", 100, "application/json", 250},                        // 100 * 2.5
		{"xml", 100, "application/xml", 210},                          // 100 * 2.1
		{"unknown", 100, "application/unknown", 250},                  // 100 * 2.5 (default)
		{"xhtml", 100, "application/xhtml+xml", 190},                  // 100 * 1.9
		{"text_xml", 100, "text/xml", 210},                            // 100 * 2.1
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := charsForTokens(tt.tokens, tt.contentType)
			assert.Equal(t, tt.expected, result, "chars for tokens calculation")
		})
	}
}

// TestCharsForTokensRoundTrip verifies chars-to-tokens-to-chars round trip.
func TestCharsForTokensRoundTrip(t *testing.T) {
	targetTokens := 100
	contentType := "text/plain"

	// Calculate chars needed
	chars := charsForTokens(targetTokens, contentType)

	// Create content with that many chars
	content := []byte(strings.Repeat("a", chars))

	// Estimate tokens
	estimatedTokens := EstimateTokens(content, contentType)

	// Should be close to original target
	assert.InDelta(t, targetTokens, estimatedTokens, 1,
		"round trip should preserve token count")
}

// TestEstimateTokensRealWorldHTML verifies estimation on real-world HTML.
func TestEstimateTokensRealWorldHTML(t *testing.T) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Test Page</title>
</head>
<body>
    <h1>Welcome</h1>
    <p>This is a test paragraph with some content.</p>
    <ul>
        <li>Item 1</li>
        <li>Item 2</li>
        <li>Item 3</li>
    </ul>
</body>
</html>`

	tokens := EstimateTokens([]byte(html), "text/html")

	// Should be positive and reasonable
	assert.Greater(t, tokens, 0, "should estimate positive tokens")
	assert.Less(t, tokens, len(html), "tokens should be less than char count")

	// With 1.9 ratio, ~283 chars / 1.9 = ~149 tokens
	assert.InDelta(t, 149, tokens, 20, "should be reasonable estimate for HTML")
}

// TestEstimateTokensRealWorldMarkdown verifies estimation on markdown (plain text).
func TestEstimateTokensRealWorldMarkdown(t *testing.T) {
	markdown := `# Heading

This is a paragraph with **bold** and *italic* text.

## Subheading

- List item 1
- List item 2
- List item 3

Here's a [link](https://example.com).
`

	tokens := EstimateTokens([]byte(markdown), "text/plain")

	assert.Greater(t, tokens, 0)
	assert.Less(t, tokens, len(markdown))

	// With 1.7 ratio, ~190 chars / 1.7 = ~112 tokens
	assert.InDelta(t, 112, tokens, 20)
}

// TestEstimateTokensZeroTokensForZeroContent verifies edge case.
func TestEstimateTokensZeroTokensForZeroContent(t *testing.T) {
	contentTypes := []string{
		"text/plain",
		"text/html",
		"application/json",
		"application/xml",
		"text/xml",
		"application/xhtml+xml",
		"unknown/type",
	}

	for _, ct := range contentTypes {
		t.Run(ct, func(t *testing.T) {
			tokens := EstimateTokens([]byte(""), ct)
			assert.Equal(t, 0, tokens, "zero content should give zero tokens")
		})
	}
}

// TestEstimateTokensSingleCharacter verifies minimum token count.
func TestEstimateTokensSingleCharacter(t *testing.T) {
	tokens := EstimateTokens([]byte("a"), "text/plain")
	// With 1.7 ratio: 1 / 1.7 = 0.588... = 0 when cast to int
	// This is expected behavior - very short content may estimate to 0 tokens
	assert.GreaterOrEqual(t, tokens, 0)
}

// TestEstimateTokensLargeContent verifies no overflow on large content.
func TestEstimateTokensLargeContent(t *testing.T) {
	// 1MB of content
	largeContent := []byte(strings.Repeat("a", 1024*1024))
	tokens := EstimateTokens(largeContent, "text/plain")

	// Should be approximately 1MB / 1.7 = ~620k tokens
	assert.Greater(t, tokens, 500000)
	assert.Less(t, tokens, 700000)
}
