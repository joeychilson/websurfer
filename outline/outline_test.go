package outline

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestExtractBytesMarkdown verifies markdown outline extraction.
func TestExtractBytesMarkdown(t *testing.T) {
	content := []byte(`# Main Heading

Some text here.

## Subheading

More content.`)

	result := ExtractBytes(content, "text/markdown")

	assert.NotNil(t, result)
	assert.Len(t, result.Headings, 2, "should extract 2 headings")
	assert.Equal(t, 1, result.Headings[0].Level)
	assert.Equal(t, "Main Heading", result.Headings[0].Text)
	assert.Equal(t, 2, result.Headings[1].Level)
	assert.Equal(t, "Subheading", result.Headings[1].Text)
}

// TestExtractBytesNonMarkdown verifies non-markdown returns empty outline.
func TestExtractBytesNonMarkdown(t *testing.T) {
	content := []byte("<html><body>HTML content</body></html>")

	result := ExtractBytes(content, "text/html")

	assert.NotNil(t, result)
	assert.Empty(t, result.Headings)
	assert.Empty(t, result.Tables)
	assert.Empty(t, result.Lists)
}

// TestExtractMarkdownHeadings verifies heading extraction.
func TestExtractMarkdownHeadings(t *testing.T) {
	content := `# Level 1
## Level 2
### Level 3
#### Level 4`

	result := extractMarkdown(content)

	assert.Len(t, result.Headings, 4)
	assert.Equal(t, 1, result.Headings[0].Level)
	assert.Equal(t, "Level 1", result.Headings[0].Text)
	assert.Equal(t, 2, result.Headings[1].Level)
	assert.Equal(t, "Level 2", result.Headings[1].Text)
	assert.Equal(t, 3, result.Headings[2].Level)
	assert.Equal(t, "Level 3", result.Headings[2].Text)
	assert.Equal(t, 4, result.Headings[3].Level)
	assert.Equal(t, "Level 4", result.Headings[3].Text)
}

// TestExtractMarkdownHeadingsCharPositions verifies character positions are tracked.
func TestExtractMarkdownHeadingsCharPositions(t *testing.T) {
	content := `# First
## Second
### Third`

	result := extractMarkdown(content)

	assert.Len(t, result.Headings, 3)
	// Each heading should have a CharStart
	assert.GreaterOrEqual(t, result.Headings[0].CharStart, 0)
	assert.GreaterOrEqual(t, result.Headings[1].CharStart, result.Headings[0].CharStart)
	assert.GreaterOrEqual(t, result.Headings[2].CharStart, result.Headings[1].CharStart)

	// CharEnd should span to next heading or end of content
	assert.Equal(t, result.Headings[1].CharStart, result.Headings[0].CharEnd)
	assert.Equal(t, result.Headings[2].CharStart, result.Headings[1].CharEnd)
	assert.Equal(t, len(content), result.Headings[2].CharEnd)
}

// TestExtractMarkdownHeadingsWithWhitespace verifies trimming works.
func TestExtractMarkdownHeadingsWithWhitespace(t *testing.T) {
	content := `#    Heading with spaces
##   Another one   `

	result := extractMarkdown(content)

	assert.Len(t, result.Headings, 2)
	assert.Equal(t, "Heading with spaces", result.Headings[0].Text)
	assert.Equal(t, "Another one", result.Headings[1].Text)
}

// TestExtractMarkdownHeadingsNoSpace verifies headings without space after # are ignored.
func TestExtractMarkdownHeadingsNoSpace(t *testing.T) {
	content := `#NoSpace
# Valid Heading
##AlsoNoSpace`

	result := extractMarkdown(content)

	// Only the valid heading with space after # should be extracted
	assert.Len(t, result.Headings, 1)
	assert.Equal(t, "Valid Heading", result.Headings[0].Text)
}

// TestExtractMarkdownTables verifies table extraction.
func TestExtractMarkdownTables(t *testing.T) {
	content := `Some text

| Header 1 | Header 2 | Header 3 |
|----------|----------|----------|
| Cell 1   | Cell 2   | Cell 3   |
| Cell 4   | Cell 5   | Cell 6   |

More text`

	result := extractMarkdown(content)

	assert.Len(t, result.Tables, 1)
	assert.Len(t, result.Tables[0].Headers, 3)
	assert.Equal(t, "Header 1", result.Tables[0].Headers[0])
	assert.Equal(t, "Header 2", result.Tables[0].Headers[1])
	assert.Equal(t, "Header 3", result.Tables[0].Headers[2])
	assert.Equal(t, 2, result.Tables[0].RowCount)
}

// TestExtractMarkdownTablesCharPositions verifies table positions are tracked.
func TestExtractMarkdownTablesCharPositions(t *testing.T) {
	content := `| Col 1 | Col 2 |
|-------|-------|
| A     | B     |`

	result := extractMarkdown(content)

	assert.Len(t, result.Tables, 1)
	assert.GreaterOrEqual(t, result.Tables[0].CharStart, 0)
	assert.Greater(t, result.Tables[0].CharEnd, result.Tables[0].CharStart)
}

// TestExtractMarkdownTablesMultiple verifies multiple tables are extracted.
func TestExtractMarkdownTablesMultiple(t *testing.T) {
	content := `| Table 1 |
|---------|
| Data 1  |

Text between tables

| Table 2 |
|---------|
| Data 2  |`

	result := extractMarkdown(content)

	assert.Len(t, result.Tables, 2)
	assert.Len(t, result.Tables[0].Headers, 1)
	assert.Len(t, result.Tables[1].Headers, 1)
}

// TestExtractMarkdownListsUnordered verifies unordered list extraction.
func TestExtractMarkdownListsUnordered(t *testing.T) {
	content := `- Item 1
- Item 2
- Item 3`

	result := extractMarkdown(content)

	assert.Len(t, result.Lists, 1)
	assert.Equal(t, "unordered", result.Lists[0].Type)
	assert.Equal(t, 3, result.Lists[0].ItemCount)
	assert.Len(t, result.Lists[0].Items, 3)
	assert.Equal(t, "Item 1", result.Lists[0].Items[0])
	assert.Equal(t, "Item 2", result.Lists[0].Items[1])
	assert.Equal(t, "Item 3", result.Lists[0].Items[2])
}

// TestExtractMarkdownListsOrdered verifies ordered list extraction.
func TestExtractMarkdownListsOrdered(t *testing.T) {
	content := `1. First item
2. Second item
3. Third item`

	result := extractMarkdown(content)

	assert.Len(t, result.Lists, 1)
	assert.Equal(t, "ordered", result.Lists[0].Type)
	assert.Equal(t, 3, result.Lists[0].ItemCount)
	assert.Len(t, result.Lists[0].Items, 3)
	assert.Equal(t, "First item", result.Lists[0].Items[0])
}

// TestExtractMarkdownListsAlternateBullets verifies different bullet styles.
func TestExtractMarkdownListsAlternateBullets(t *testing.T) {
	tests := []struct {
		content string
		bullets string
	}{
		{"- Dash bullet", "-"},
		{"* Star bullet", "*"},
		{"+ Plus bullet", "+"},
	}

	for _, tt := range tests {
		result := extractMarkdown(tt.content)
		assert.Len(t, result.Lists, 1, "bullet style: %s", tt.bullets)
		assert.Equal(t, "unordered", result.Lists[0].Type)
	}
}

// TestExtractMarkdownListsItemLimit verifies only first 3 items are stored.
func TestExtractMarkdownListsItemLimit(t *testing.T) {
	content := `- Item 1
- Item 2
- Item 3
- Item 4
- Item 5`

	result := extractMarkdown(content)

	assert.Len(t, result.Lists, 1)
	assert.Equal(t, 5, result.Lists[0].ItemCount, "should count all items")
	assert.Len(t, result.Lists[0].Items, 3, "should only store first 3 items")
}

// TestExtractMarkdownListsCharPositions verifies list positions are tracked.
func TestExtractMarkdownListsCharPositions(t *testing.T) {
	content := `- Item 1
- Item 2`

	result := extractMarkdown(content)

	assert.Len(t, result.Lists, 1)
	assert.GreaterOrEqual(t, result.Lists[0].CharStart, 0)
	assert.Greater(t, result.Lists[0].CharEnd, result.Lists[0].CharStart)
}

// TestExtractMarkdownListsMixed verifies ordered and unordered lists are separate.
func TestExtractMarkdownListsMixed(t *testing.T) {
	content := `- Unordered item 1
- Unordered item 2

1. Ordered item 1
2. Ordered item 2`

	result := extractMarkdown(content)

	assert.Len(t, result.Lists, 2)
	assert.Equal(t, "unordered", result.Lists[0].Type)
	assert.Equal(t, "ordered", result.Lists[1].Type)
}

// TestExtractMarkdownListsTypeChange verifies type change creates new list.
func TestExtractMarkdownListsTypeChange(t *testing.T) {
	content := `- Unordered item
1. Ordered item
- Back to unordered`

	result := extractMarkdown(content)

	// Should create 3 separate lists (type changes split lists)
	assert.Len(t, result.Lists, 3)
	assert.Equal(t, "unordered", result.Lists[0].Type)
	assert.Equal(t, "ordered", result.Lists[1].Type)
	assert.Equal(t, "unordered", result.Lists[2].Type)
}

// TestExtractMarkdownComplete verifies complete outline extraction.
func TestExtractMarkdownComplete(t *testing.T) {
	content := `# Main Title

Introduction paragraph.

## Features

- Feature 1
- Feature 2
- Feature 3

## Comparison

| Product | Price | Rating |
|---------|-------|--------|
| A       | $10   | 5/5    |
| B       | $15   | 4/5    |

### Conclusion

Final thoughts.`

	result := extractMarkdown(content)

	// Should extract all structures (4 headings: Main Title, Features, Comparison, Conclusion)
	assert.Len(t, result.Headings, 4, "should extract all headings")
	assert.Len(t, result.Tables, 1, "should extract table")
	assert.Len(t, result.Lists, 1, "should extract list")

	// Verify heading hierarchy
	assert.Equal(t, "Main Title", result.Headings[0].Text)
	assert.Equal(t, "Features", result.Headings[1].Text)
	assert.Equal(t, "Comparison", result.Headings[2].Text)
	assert.Equal(t, "Conclusion", result.Headings[3].Text)

	// Verify table
	assert.Equal(t, 3, len(result.Tables[0].Headers))
	assert.Equal(t, 2, result.Tables[0].RowCount)

	// Verify list
	assert.Equal(t, 3, result.Lists[0].ItemCount)
}

// TestIsMarkdown verifies markdown content type detection.
func TestIsMarkdown(t *testing.T) {
	tests := []struct {
		contentType string
		expected    bool
	}{
		{"text/markdown", true},
		{"text/x-markdown", true},
		{"markdown", true},
		{"text/html", false},
		{"application/json", false},
		{"", false},
	}

	for _, tt := range tests {
		result := isMarkdown(tt.contentType)
		assert.Equal(t, tt.expected, result, "contentType: %s", tt.contentType)
	}
}

// TestExtractBytesEmpty verifies empty content handling.
func TestExtractBytesEmpty(t *testing.T) {
	result := ExtractBytes([]byte(""), "text/markdown")

	assert.NotNil(t, result)
	assert.Empty(t, result.Headings)
	assert.Empty(t, result.Tables)
	assert.Empty(t, result.Lists)
}
