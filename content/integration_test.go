package content

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

// TestTruncatePaginationNoDataLoss verifies pagination doesn't lose content between chunks.
// This is CRITICAL for LLMs - they need complete information across pages.
func TestTruncatePaginationNoDataLoss(t *testing.T) {
	// Create large content (simulate 10KB document)
	var content strings.Builder
	for i := 0; i < 100; i++ {
		content.WriteString("# Section ")
		content.WriteString(string(rune('0' + i%10)))
		content.WriteString("\n\nThis is paragraph ")
		content.WriteString(string(rune('0' + i%10)))
		content.WriteString(" with some content that should be preserved across pagination boundaries. ")
		content.WriteString("It contains important information for the LLM.\n\n")
	}

	original := []byte(content.String())
	contentType := "text/plain" // Use text/plain for consistent token estimation
	maxTokens := 500

	// Paginate through entire document using NextOffset
	var reconstructed strings.Builder
	charOffset := 0
	pageCount := 0

	for {
		if charOffset >= len(original) {
			break
		}

		// Get content from current offset
		contentFromOffset := original[charOffset:]
		result := Truncate(contentFromOffset, contentType, maxTokens)

		reconstructed.WriteString(result.Content)
		pageCount++

		if !result.Truncated {
			// Last page - we're done
			break
		}

		// Use NextOffset to get the exact position for the next page
		// Since we're passing a slice starting at charOffset, add that back
		charOffset += result.NextOffset
	}

	// Verify no content was lost
	originalStr := strings.TrimSpace(string(original))
	reconstructedStr := strings.TrimSpace(reconstructed.String())

	assert.Equal(t, originalStr, reconstructedStr, "pagination should not lose content")
	t.Logf("Successfully paginated document into %d pages without data loss", pageCount)
}

// TestTruncateUTF8BoundarySafety verifies truncation never splits UTF-8 characters.
// This prevents corrupted text being sent to LLMs.
func TestTruncateUTF8BoundarySafety(t *testing.T) {
	// Content with multi-byte UTF-8 characters
	tests := []struct {
		content string
		name    string
	}{
		{"Hello 👋 World 🌍 Test 🎉", "emoji"},
		{"こんにちは世界", "japanese"},
		{"Привет мир", "cyrillic"},
		{"مرحبا بالعالم", "arabic"},
		{"Hello\u2028World", "unicode_line_separator"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte(tt.content)

			// Try various truncation sizes
			for maxTokens := 1; maxTokens <= 20; maxTokens++ {
				result := Truncate(content, "text/plain", maxTokens)

				// Verify result is valid UTF-8
				assert.True(t, utf8.Valid([]byte(result.Content)),
					"truncated content must be valid UTF-8 (maxTokens=%d)", maxTokens)
			}
		})
	}
}

// TestTruncateTableRowIntegrity verifies pagination never splits table rows.
// Incomplete tables break LLM data extraction.
func TestTruncateTableRowIntegrity(t *testing.T) {
	// Markdown table
	table := `| Name | Age | City |
|------|-----|------|
| Alice | 25 | NYC |
| Bob | 30 | LA |
| Charlie | 35 | Chicago |
| Diana | 40 | Boston |
| Eve | 45 | Seattle |`

	content := []byte(table)

	// Try various truncation points
	for maxTokens := 10; maxTokens <= 100; maxTokens += 10 {
		result := Truncate(content, "text/markdown", maxTokens)

		if result.Truncated {
			// If table is included, it should be complete (all rows or none)
			resultStr := result.Content
			if strings.Contains(resultStr, "|------|") {
				// Table separator present - verify table structure is intact
				lines := strings.Split(strings.TrimSpace(resultStr), "\n")

				// Find table boundaries
				var tableStart, tableEnd int
				for i, line := range lines {
					if strings.Contains(line, "|------|") {
						tableStart = i - 1 // Header is one line before separator
						// Find end of table
						for j := i + 1; j < len(lines); j++ {
							if !strings.Contains(lines[j], "|") {
								tableEnd = j - 1
								break
							}
							if j == len(lines)-1 {
								tableEnd = j
							}
						}
						break
					}
				}

				// Verify no partial rows (each row has same number of |)
				if tableStart >= 0 && tableEnd > tableStart {
					pipeCount := strings.Count(lines[tableStart], "|")
					for i := tableStart; i <= tableEnd; i++ {
						assert.Equal(t, pipeCount, strings.Count(lines[i], "|"),
							"all table rows should have same column count (maxTokens=%d)", maxTokens)
					}
				}
			}
		}
	}
}

// TestTruncateDeterministicAcrossRuns verifies pagination is deterministic.
// Same URL fetched multiple times should return identical pages.
func TestTruncateDeterministicAcrossRuns(t *testing.T) {
	content := []byte(strings.Repeat("The quick brown fox jumps over the lazy dog. ", 200))
	maxTokens := 100

	// Run truncation 10 times
	results := make([]string, 10)
	for i := 0; i < 10; i++ {
		result := Truncate(content, "text/plain", maxTokens)
		results[i] = result.Content
	}

	// All results should be identical
	for i := 1; i < len(results); i++ {
		assert.Equal(t, results[0], results[i],
			"truncation must be deterministic across runs")
	}
}

// TestTruncateMaxTokensRespected verifies returned content stays within token limit.
// LLMs have strict context windows - exceeding them causes errors.
func TestTruncateMaxTokensRespected(t *testing.T) {
	content := []byte(strings.Repeat("This is a test sentence. ", 500))

	tests := []int{100, 500, 1000, 5000}

	for _, maxTokens := range tests {
		result := Truncate(content, "text/plain", maxTokens)

		// Estimate tokens in returned content
		actualTokens := EstimateTokens([]byte(result.Content), "text/plain")

		// Allow 10% margin (as documented in original tests)
		upperBound := float64(maxTokens) * 1.1
		assert.LessOrEqual(t, float64(actualTokens), upperBound,
			"returned content should not exceed max_tokens by more than 10%% (maxTokens=%d, actual=%d)",
			maxTokens, actualTokens)
	}
}

// TestTruncateCodeBlockIntegrity verifies code blocks are never split.
// Broken code blocks confuse LLMs trying to extract/execute code.
func TestTruncateCodeBlockIntegrity(t *testing.T) {
	content := []byte(`
# Code Example

Here's some Python code:

` + "```python\n" + `def hello():
    print("Hello, World!")
    for i in range(10):
        print(i)
    return True
` + "```\n" + `

And here's more text after the code block.
`)

	// Try various truncation points
	for maxTokens := 10; maxTokens <= 100; maxTokens += 10 {
		result := Truncate(content, "text/markdown", maxTokens)

		if result.Truncated {
			resultStr := result.Content

			// If code block fence is present, verify it's closed
			openFences := strings.Count(resultStr, "```")
			assert.Equal(t, 0, openFences%2,
				"code block fences must be balanced (maxTokens=%d, fences=%d)",
				maxTokens, openFences)
		}
	}
}
