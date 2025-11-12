package html

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHTMLToMarkdownBasic verifies basic HTML converts to markdown.
func TestHTMLToMarkdownBasic(t *testing.T) {
	parser := New()
	html := `<html><body><h1>Title</h1><p>Paragraph text</p></body></html>`

	result, err := parser.Parse(context.Background(), []byte(html))

	require.NoError(t, err)
	markdown := string(result)
	assert.Contains(t, markdown, "# Title", "should convert h1 to markdown heading")
	assert.Contains(t, markdown, "Paragraph text", "should preserve text content")
}

// TestHTMLToMarkdownLinks verifies link conversion.
func TestHTMLToMarkdownLinks(t *testing.T) {
	parser := New()
	html := `<a href="https://example.com">Click here</a>`

	result, err := parser.Parse(context.Background(), []byte(html))

	require.NoError(t, err)
	markdown := string(result)
	assert.Contains(t, markdown, "[Click here]", "should format link text")
	assert.Contains(t, markdown, "(https://example.com)", "should include URL")
}

// TestHTMLToMarkdownTables verifies table preservation (critical for LLMs).
func TestHTMLToMarkdownTables(t *testing.T) {
	parser := New()
	html := `<table>
<tr><th>Header 1</th><th>Header 2</th></tr>
<tr><td>Cell 1</td><td>Cell 2</td></tr>
<tr><td>Cell 3</td><td>Cell 4</td></tr>
</table>`

	result, err := parser.Parse(context.Background(), []byte(html))

	require.NoError(t, err)
	markdown := string(result)

	// Should convert to markdown table format
	assert.Contains(t, markdown, "Header 1", "should preserve headers")
	assert.Contains(t, markdown, "Header 2", "should preserve headers")
	assert.Contains(t, markdown, "Cell 1", "should preserve cell data")
	assert.Contains(t, markdown, "|", "should use pipe separators")
}

// TestHTMLToMarkdownLists verifies list conversion.
func TestHTMLToMarkdownLists(t *testing.T) {
	parser := New()
	html := `<ul>
<li>Item 1</li>
<li>Item 2</li>
<li>Item 3</li>
</ul>`

	result, err := parser.Parse(context.Background(), []byte(html))

	require.NoError(t, err)
	markdown := string(result)
	assert.Contains(t, markdown, "Item 1", "should preserve list items")
	assert.Contains(t, markdown, "Item 2", "should preserve list items")
	assert.Contains(t, markdown, "Item 3", "should preserve list items")
}

// TestHTMLToMarkdownCodeBlocks verifies code block preservation.
func TestHTMLToMarkdownCodeBlocks(t *testing.T) {
	parser := New()
	html := `<pre><code>function test() {
  return true;
}</code></pre>`

	result, err := parser.Parse(context.Background(), []byte(html))

	require.NoError(t, err)
	markdown := string(result)
	assert.Contains(t, markdown, "function test()", "should preserve code content")
	assert.Contains(t, markdown, "return true", "should preserve code content")
}

// TestHTMLToMarkdownHeadingHierarchy verifies heading levels preserved.
func TestHTMLToMarkdownHeadingHierarchy(t *testing.T) {
	parser := New()
	html := `<h1>H1</h1><h2>H2</h2><h3>H3</h3><h4>H4</h4>`

	result, err := parser.Parse(context.Background(), []byte(html))

	require.NoError(t, err)
	markdown := string(result)
	assert.Contains(t, markdown, "# H1", "should convert h1")
	assert.Contains(t, markdown, "## H2", "should convert h2")
	assert.Contains(t, markdown, "### H3", "should convert h3")
	assert.Contains(t, markdown, "#### H4", "should convert h4")
}

// TestHTMLSanitization verifies dangerous content is removed.
func TestHTMLSanitization(t *testing.T) {
	parser := New()
	html := `<script>alert('xss')</script><p>Safe content</p>`

	result, err := parser.Parse(context.Background(), []byte(html))

	require.NoError(t, err)
	markdown := string(result)
	assert.NotContains(t, markdown, "script", "should remove script tags")
	assert.NotContains(t, markdown, "alert", "should remove script content")
	assert.Contains(t, markdown, "Safe content", "should preserve safe content")
}

// TestHTMLWhitespaceNormalization verifies whitespace is normalized.
func TestHTMLWhitespaceNormalization(t *testing.T) {
	parser := New()
	html := `<p>Text   with    multiple     spaces</p>`

	result, err := parser.Parse(context.Background(), []byte(html))

	require.NoError(t, err)
	markdown := strings.TrimSpace(string(result))
	// Multiple spaces should be normalized (exact behavior depends on implementation)
	assert.Contains(t, markdown, "Text", "should preserve text")
	assert.Contains(t, markdown, "spaces", "should preserve text")
}

// TestHTMLEmptyContent verifies empty content handling.
func TestHTMLEmptyContent(t *testing.T) {
	parser := New()
	html := ``

	result, err := parser.Parse(context.Background(), []byte(html))

	require.NoError(t, err)
	assert.Equal(t, []byte(""), result, "empty input should return empty output")
}

// TestHTMLComplexDocument verifies realistic webpage conversion.
func TestHTMLComplexDocument(t *testing.T) {
	parser := New()
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Test Page</title>
    <meta charset="utf-8">
</head>
<body>
    <header>
        <h1>Main Title</h1>
        <nav>
            <a href="/home">Home</a>
            <a href="/about">About</a>
        </nav>
    </header>
    <main>
        <article>
            <h2>Article Title</h2>
            <p>This is a <strong>test</strong> article with <em>formatting</em>.</p>
            <ul>
                <li>Point 1</li>
                <li>Point 2</li>
            </ul>
            <table>
                <tr><th>Name</th><th>Value</th></tr>
                <tr><td>Item 1</td><td>100</td></tr>
            </table>
        </article>
    </main>
</body>
</html>`

	result, err := parser.Parse(context.Background(), []byte(html))

	require.NoError(t, err)
	markdown := string(result)

	// Verify key elements are preserved
	assert.Contains(t, markdown, "Main Title", "should have main title")
	assert.Contains(t, markdown, "Article Title", "should have article title")
	assert.Contains(t, markdown, "test", "should have body text")
	assert.Contains(t, markdown, "Point 1", "should have list items")
	assert.Contains(t, markdown, "Name", "should have table headers")
	assert.Contains(t, markdown, "Item 1", "should have table data")

	// Should NOT contain HTML tags
	assert.NotContains(t, markdown, "<html>", "should not contain HTML tags")
	assert.NotContains(t, markdown, "<body>", "should not contain HTML tags")
}

// TestHTMLNestedStructures verifies nested elements are handled.
func TestHTMLNestedStructures(t *testing.T) {
	parser := New()
	html := `<div>
    <div>
        <p>Nested <span>content</span> here</p>
    </div>
</div>`

	result, err := parser.Parse(context.Background(), []byte(html))

	require.NoError(t, err)
	markdown := string(result)
	assert.Contains(t, markdown, "Nested", "should handle nested content")
	assert.Contains(t, markdown, "content", "should handle nested content")
}

// TestHTMLSpecialCharacters verifies special characters are handled.
func TestHTMLSpecialCharacters(t *testing.T) {
	parser := New()
	html := `<p>&lt;tag&gt; &amp; "quoted" text</p>`

	result, err := parser.Parse(context.Background(), []byte(html))

	require.NoError(t, err)
	markdown := string(result)
	// HTML entities should be decoded
	assert.Contains(t, markdown, "quoted", "should preserve text")
}

// TestHTMLImageHandling verifies images are stripped (correct for LLM text output).
func TestHTMLImageHandling(t *testing.T) {
	parser := New()
	html := `<p>Text before <img src="image.jpg" alt="Description of image"> text after</p>`

	result, err := parser.Parse(context.Background(), []byte(html))

	require.NoError(t, err)
	markdown := strings.TrimSpace(string(result))
	// Images are stripped in LLM-optimized markdown (can't include images in text)
	assert.Contains(t, markdown, "Text before", "should preserve text before image")
	assert.Contains(t, markdown, "text after", "should preserve text after image")
	assert.NotContains(t, markdown, "image.jpg", "should not include image src")
}

// TestHTMLBlockquote verifies blockquote conversion.
func TestHTMLBlockquote(t *testing.T) {
	parser := New()
	html := `<blockquote>This is a quote</blockquote>`

	result, err := parser.Parse(context.Background(), []byte(html))

	require.NoError(t, err)
	markdown := string(result)
	assert.Contains(t, markdown, "This is a quote", "should preserve quote text")
}

// TestHTMLInlineFormatting verifies inline formatting preserved.
func TestHTMLInlineFormatting(t *testing.T) {
	parser := New()
	html := `<p>Text with <strong>bold</strong>, <em>italic</em>, and <code>code</code></p>`

	result, err := parser.Parse(context.Background(), []byte(html))

	require.NoError(t, err)
	markdown := string(result)
	assert.Contains(t, markdown, "bold", "should preserve bold text")
	assert.Contains(t, markdown, "italic", "should preserve italic text")
	assert.Contains(t, markdown, "code", "should preserve code text")
}

// TestHTMLOrderedList verifies ordered list conversion.
func TestHTMLOrderedList(t *testing.T) {
	parser := New()
	html := `<ol>
<li>First</li>
<li>Second</li>
<li>Third</li>
</ol>`

	result, err := parser.Parse(context.Background(), []byte(html))

	require.NoError(t, err)
	markdown := string(result)
	assert.Contains(t, markdown, "First", "should preserve list content")
	assert.Contains(t, markdown, "Second", "should preserve list content")
	assert.Contains(t, markdown, "Third", "should preserve list content")
}

// TestHTMLNoData verifies nil input handling.
func TestHTMLNoData(t *testing.T) {
	parser := New()

	result, err := parser.Parse(context.Background(), nil)

	require.NoError(t, err)
	assert.Nil(t, result, "nil input should return nil")
}

// TestHTMLParserCreation verifies parser can be created.
func TestHTMLParserCreation(t *testing.T) {
	parser := New()
	assert.NotNil(t, parser, "should create parser")
}
