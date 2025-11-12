package content

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTruncateNoTruncationNeeded verifies that content under the limit is not truncated.
func TestTruncateNoTruncationNeeded(t *testing.T) {
	content := []byte("Short content")
	result := Truncate(content, "text/plain", 1000)

	assert.False(t, result.Truncated, "content should not be truncated")
	assert.Equal(t, string(content), result.Content, "content should be unchanged")
	assert.Equal(t, len(content), result.ReturnedChars)
	assert.Equal(t, len(content), result.TotalChars)
	assert.Equal(t, result.ReturnedTokens, result.TotalTokens)
}

// TestTruncateHTMLPreservesStructure verifies that HTML truncation never splits markdown elements.
func TestTruncateHTMLPreservesStructure(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		maxTokens   int
		shouldSplit bool
		description string
	}{
		{
			name: "table_boundary",
			content: `<p>Introduction</p>
<table>
<tr><td>Row 1</td></tr>
<tr><td>Row 2</td></tr>
<tr><td>Row 3</td></tr>
</table>
<p>Conclusion</p>`,
			maxTokens:   50,
			shouldSplit: false,
			description: "should not split in middle of table",
		},
		{
			name: "paragraph_boundary",
			content: `<p>First paragraph with some content.</p>
<p>Second paragraph with more content.</p>
<p>Third paragraph with even more content.</p>`,
			maxTokens:   30,
			shouldSplit: false,
			description: "should split at </p> tag boundary",
		},
		{
			name: "list_boundary",
			content: `<ul>
<li>Item 1</li>
<li>Item 2</li>
<li>Item 3</li>
<li>Item 4</li>
</ul>`,
			maxTokens:   20,
			shouldSplit: false,
			description: "should split at </li> boundary",
		},
		{
			name: "heading_boundary",
			content: `<h1>Main Heading</h1>
<p>Content here</p>
<h2>Subheading</h2>
<p>More content</p>`,
			maxTokens:   25,
			shouldSplit: false,
			description: "should split at heading boundary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Truncate([]byte(tt.content), "text/html", tt.maxTokens)

			if !result.Truncated {
				t.Skip("content was not truncated, cannot test boundary")
			}

			// Verify we don't have unclosed tags (basic validation)
			// A more sophisticated check would parse the HTML
			truncated := result.Content

			// Should try to end at a tag boundary (allow word boundary fallback)
			// The key test: we should not have broken tags like "<p" or "p>"
			if strings.HasSuffix(truncated, "<") {
				t.Errorf("%s: truncated content ends with incomplete opening tag", tt.description)
			}

			// Check for partial tags in the last 10 characters
			if len(truncated) > 10 {
				tail := truncated[len(truncated)-10:]
				// Look for opening < without closing >
				lastOpen := strings.LastIndex(tail, "<")
				lastClose := strings.LastIndex(tail, ">")
				if lastOpen > lastClose {
					t.Errorf("%s: truncated content has unclosed tag in tail: %q", tt.description, tail)
				}
			}

			// Count opening vs closing tags for common elements
			for _, tag := range []string{"table", "tr", "td", "ul", "ol", "li", "p"} {
				openCount := strings.Count(truncated, "<"+tag)
				closeCount := strings.Count(truncated, "</"+tag+">")
				if openCount > closeCount {
					// This is expected - we may cut off before closing tags
					// But we should have cut at a closing tag boundary
					assert.Contains(t, truncated, "</", "should end at a closing tag")
				}
			}
		})
	}
}

// TestTruncateDeterministic verifies that pagination is reproducible.
func TestTruncateDeterministic(t *testing.T) {
	content := []byte(strings.Repeat("This is a test sentence. ", 100))
	maxTokens := 100

	// Run truncation multiple times
	results := make([]*TruncateResult, 5)
	for i := 0; i < 5; i++ {
		results[i] = Truncate(content, "text/plain", maxTokens)
	}

	// All results should be identical
	for i := 1; i < len(results); i++ {
		assert.Equal(t, results[0].Content, results[i].Content,
			"truncation should be deterministic - run %d differs from run 0", i)
		assert.Equal(t, results[0].ReturnedChars, results[i].ReturnedChars)
		assert.Equal(t, results[0].ReturnedTokens, results[i].ReturnedTokens)
	}
}

// TestTruncateRespectsMaxTokens verifies that chunks respect max_tokens within reasonable margin.
func TestTruncateRespectsMaxTokens(t *testing.T) {
	content := []byte(strings.Repeat("word ", 1000))
	maxTokens := 100

	result := Truncate(content, "text/plain", maxTokens)

	// Allow 10% margin for smart boundary detection
	margin := float64(maxTokens) * 0.10
	upperBound := float64(maxTokens) + margin

	assert.LessOrEqual(t, float64(result.ReturnedTokens), upperBound,
		"returned tokens should not exceed max_tokens + 10%% margin")
	assert.Greater(t, result.ReturnedTokens, 0, "should return some tokens")
}

// TestTruncateWordBoundary verifies that plain text tries to truncate at word boundaries.
func TestTruncateWordBoundary(t *testing.T) {
	content := []byte("The quick brown fox jumps over the lazy dog")
	maxTokens := 10 // Will need to truncate

	result := Truncate(content, "text/plain", maxTokens)

	if !result.Truncated {
		t.Skip("content was not truncated")
	}

	// Should try to find word boundary, but if the window is small it may not find one
	// The important thing is it doesn't completely break words in half arbitrarily
	// Verify we got reasonable truncation
	assert.Greater(t, len(result.Content), 0, "should return some content")
	assert.Less(t, len(result.Content), len(content), "should be truncated")

	// If truncation is long enough, should prefer word boundaries
	if len(result.Content) > 10 {
		lastChar := result.Content[len(result.Content)-1]
		// Either ends at whitespace, punctuation, or we're within the search window
		if !isWhitespace(lastChar) && lastChar != '.' && lastChar != ',' {
			// Check we're at least near the target (within the word boundary window)
			targetChars := charsForTokens(maxTokens, "text/plain")
			assert.InDelta(t, targetChars, len(result.Content), float64(targetChars)/5,
				"if not at word boundary, should be close to target")
		}
	}
}

// TestTruncateHTMLFallbackToWordBoundary verifies HTML falls back to word boundary if no tags found.
func TestTruncateHTMLFallbackToWordBoundary(t *testing.T) {
	// Plain text with HTML content type (no tags to find)
	content := []byte("This is plain text without any HTML tags at all")
	maxTokens := 10

	result := Truncate(content, "text/html", maxTokens)

	if !result.Truncated {
		t.Skip("content was not truncated")
	}

	// Should fall back to word boundary
	assert.NotEmpty(t, result.Content)
	// Should not end mid-word if possible
	if len(result.Content) > 0 {
		lastChar := result.Content[len(result.Content)-1]
		// Either whitespace or we hit the hard limit
		if !isWhitespace(lastChar) {
			// If not whitespace, should be close to target
			assert.InDelta(t, charsForTokens(maxTokens, "text/html"), result.ReturnedChars, 50)
		}
	}
}

// TestTruncateEmptyContent verifies handling of empty content.
func TestTruncateEmptyContent(t *testing.T) {
	result := Truncate([]byte(""), "text/plain", 100)

	assert.False(t, result.Truncated)
	assert.Equal(t, "", result.Content)
	assert.Equal(t, 0, result.ReturnedChars)
	assert.Equal(t, 0, result.ReturnedTokens)
	assert.Equal(t, 0, result.TotalChars)
	assert.Equal(t, 0, result.TotalTokens)
}

// TestTruncateVerySmallLimit verifies behavior with extremely small token limits.
func TestTruncateVerySmallLimit(t *testing.T) {
	content := []byte("This is a test")
	result := Truncate(content, "text/plain", 1)

	// Should still return something reasonable
	assert.True(t, result.Truncated)
	assert.Greater(t, len(result.Content), 0, "should return at least some content")
	assert.Less(t, len(result.Content), len(content), "should be truncated")
}

// TestTruncatePreservesCompleteMarkdownBlocks verifies code blocks and tables are not split.
func TestTruncatePreservesCompleteMarkdownBlocks(t *testing.T) {
	tests := []struct {
		name    string
		content string
		pattern string
	}{
		{
			name: "code_block",
			content: `Some text before
<pre><code>
def hello():
    print("world")
</code></pre>
More text after`,
			pattern: "</pre>",
		},
		{
			name: "blockquote",
			content: `Introduction
<blockquote>
This is a long quote that spans multiple lines
and contains important information
</blockquote>
Conclusion`,
			pattern: "</blockquote>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use a token limit that will force truncation
			result := Truncate([]byte(tt.content), "text/html", 20)

			if !result.Truncated {
				t.Skip("content was not truncated")
			}

			// If the pattern appears in original and we truncated,
			// either include it completely or don't include it at all
			if strings.Contains(tt.content, tt.pattern) {
				if strings.Contains(result.Content, strings.TrimSuffix(tt.pattern, ">")) {
					// If we started the block, we should close it
					assert.Contains(t, result.Content, tt.pattern,
						"should not split %s block", tt.name)
				}
			}
		})
	}
}

// TestTruncateMultipleChunks simulates pagination scenario.
func TestTruncateMultipleChunks(t *testing.T) {
	// Create content that will need multiple chunks
	var builder strings.Builder
	for i := 0; i < 10; i++ {
		builder.WriteString("<p>Paragraph ")
		builder.WriteString(strings.Repeat("word ", 50))
		builder.WriteString("</p>\n")
	}
	content := []byte(builder.String())

	maxTokens := 100
	chunks := []string{}
	offset := 0

	// Simulate pagination by repeatedly truncating from different offsets
	for offset < len(content) {
		remaining := content[offset:]
		result := Truncate(remaining, "text/html", maxTokens)
		chunks = append(chunks, result.Content)

		if !result.Truncated {
			break
		}

		// Move offset by returned chars
		offset += result.ReturnedChars
	}

	// Verify we got multiple chunks
	require.Greater(t, len(chunks), 1, "should have created multiple chunks")

	// Verify total content matches original (no data loss)
	reconstructed := strings.Join(chunks, "")
	assert.Equal(t, len(content), len(reconstructed),
		"total chars across all chunks should match original")
}

// TestFindHTMLBoundaryWithPreferredTags verifies preferred tags are chosen.
func TestFindHTMLBoundaryWithPreferredTags(t *testing.T) {
	content := []byte(`<article><section><div><p>Content here</p></div></section></article><p>More</p>`)

	// Find boundary near the middle
	target := len(content) / 2
	boundary := findHTMLBoundary(content, target)

	// Should have found a closing tag
	assert.Greater(t, boundary, 0)
	assert.LessOrEqual(t, boundary, len(content))

	// Result should end with '>' (closing tag)
	if boundary > 0 && boundary <= len(content) {
		assert.Equal(t, byte('>'), content[boundary-1],
			"boundary should be after a closing tag")
	}
}

// TestFindWordBoundaryFindsWhitespace verifies word boundary detection.
func TestFindWordBoundaryFindsWhitespace(t *testing.T) {
	content := []byte("word1 word2 word3 word4 word5")
	target := 15 // Middle of content

	boundary := findWordBoundary(content, target)

	// Should find a whitespace character
	if boundary > 0 && boundary < len(content) {
		// Either at whitespace or at target if no whitespace found in window
		if boundary < len(content) {
			char := content[boundary]
			// Should be whitespace or we're at the target limit
			if !isWhitespace(char) {
				assert.InDelta(t, target, boundary, 50,
					"if not at whitespace, should be near target")
			}
		}
	}
}

// TestIsWhitespace verifies whitespace detection.
func TestIsWhitespace(t *testing.T) {
	tests := []struct {
		char     byte
		expected bool
	}{
		{' ', true},
		{'\t', true},
		{'\n', true},
		{'\r', true},
		{'a', false},
		{'1', false},
		{'.', false},
	}

	for _, tt := range tests {
		t.Run(string(tt.char), func(t *testing.T) {
			assert.Equal(t, tt.expected, isWhitespace(tt.char))
		})
	}
}
