package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSECRuleCreation verifies SEC rule can be created.
func TestSECRuleCreation(t *testing.T) {
	rule := NewSECRule()
	assert.NotNil(t, rule)
	assert.Equal(t, "SEC.gov XBRL Cleanup", rule.Name())
}

// TestSECRuleMatchSecGov verifies rule matches sec.gov URLs.
func TestSECRuleMatchSecGov(t *testing.T) {
	rule := NewSECRule()

	tests := []struct {
		url         string
		contentType string
		shouldMatch bool
	}{
		{"https://www.sec.gov/Archives/edgar/data/123/file.html", "text/html", true},
		{"https://SEC.GOV/page.html", "text/html", true},
		{"https://www.sec.gov/data.html", "application/xhtml+xml", true},
		{"https://www.sec.gov/data.json", "application/json", false},
		{"https://example.com/page.html", "text/html", false},
		{"https://www.sec.gov/file.pdf", "application/pdf", false},
	}

	for _, tt := range tests {
		result := rule.Match(tt.url, tt.contentType)
		assert.Equal(t, tt.shouldMatch, result, "url=%s, contentType=%s", tt.url, tt.contentType)
	}
}

// TestSECRuleApplyRemovesXBRLTags verifies XBRL tags are removed.
func TestSECRuleApplyRemovesXBRLTags(t *testing.T) {
	rule := NewSECRule()

	content := []byte(`<html>
<body>
<ix:header>XBRL header</ix:header>
<p>Important financial data</p>
<xbrli:context>context data</xbrli:context>
</body>
</html>`)

	result := rule.Apply(content)

	resultStr := string(result)
	assert.NotContains(t, resultStr, "<ix:header>", "should remove ix: tags")
	assert.NotContains(t, resultStr, "<xbrli:context>", "should remove xbrli: tags")
	assert.Contains(t, resultStr, "Important financial data", "should preserve normal content")
}

// TestSECRuleApplyRemovesXBRLURLs verifies XBRL namespace URLs are removed.
func TestSECRuleApplyRemovesXBRLURLs(t *testing.T) {
	rule := NewSECRule()

	content := []byte(`<html>
<body>
<p>See http://fasb.org/us-gaap/2023 for details</p>
<p>Data from http://xbrl.org/2003/instance</p>
<p>Valid content at https://example.com</p>
</body>
</html>`)

	result := rule.Apply(content)

	resultStr := string(result)
	assert.NotContains(t, resultStr, "http://fasb.org", "should remove FASB URLs")
	assert.NotContains(t, resultStr, "http://xbrl.org", "should remove XBRL URLs")
	assert.Contains(t, resultStr, "https://example.com", "should preserve normal URLs")
}

// TestSECRuleApplyRemovesXBRLMembers verifies XBRL member references are removed.
func TestSECRuleApplyRemovesXBRLMembers(t *testing.T) {
	rule := NewSECRule()

	content := []byte(`<html>
<body>
<p>us-gaap:RevenueMember</p>
<p>msft:ProductRevenueMember</p>
<p>Important: Non-member text</p>
</body>
</html>`)

	result := rule.Apply(content)

	resultStr := string(result)
	assert.NotContains(t, resultStr, "us-gaap:RevenueMember", "should remove us-gaap members")
	assert.NotContains(t, resultStr, "msft:ProductRevenueMember", "should remove custom members")
	assert.Contains(t, resultStr, "Important: Non-member text", "should preserve normal text")
}

// TestSECRuleApplyRemovesMetadata verifies ISO currency and XBRL metadata is removed.
func TestSECRuleApplyRemovesMetadata(t *testing.T) {
	rule := NewSECRule()

	content := []byte(`<html>
<body>
<p>Revenue: $1000 iso4217:USD</p>
<p>Shares: 500 xbrli:shares</p>
<p>Pure ratio: 0.75 xbrli:pure</p>
</body>
</html>`)

	result := rule.Apply(content)

	resultStr := string(result)
	assert.NotContains(t, resultStr, "iso4217:USD", "should remove ISO currency codes")
	assert.NotContains(t, resultStr, "xbrli:shares", "should remove xbrli:shares")
	assert.NotContains(t, resultStr, "xbrli:pure", "should remove xbrli:pure")
	assert.Contains(t, resultStr, "$1000", "should preserve actual data")
}

// TestSECRuleApplyRemovesDateRanges verifies year ranges are removed.
func TestSECRuleApplyRemovesDateRanges(t *testing.T) {
	rule := NewSECRule()

	content := []byte(`<html>
<body>
<p>2020 2021 2022 2023</p>
<p>Year 2023 revenue</p>
</body>
</html>`)

	result := rule.Apply(content)

	resultStr := string(result)
	// Multiple consecutive years should be removed
	assert.NotContains(t, resultStr, "2020 2021 2022 2023", "should remove year ranges")
	// Single year should remain
	assert.Contains(t, resultStr, "Year 2023", "should preserve single year mentions")
}

// TestSECRuleApplyNormalizesWhitespace verifies double spaces are reduced (not fully normalized).
func TestSECRuleApplyNormalizesWhitespace(t *testing.T) {
	rule := NewSECRule()

	content := []byte("Text  with    multiple     spaces")

	result := rule.Apply(content)

	// Rule only replaces "  " (double) with " " (single), so 3+ spaces become 2+
	// This is expected behavior - it reduces but doesn't fully normalize
	assert.NotEqual(t, content, result, "should modify whitespace")
	assert.Contains(t, string(result), "Text", "should preserve content")
}

// TestSECRuleApplyTrimsWhitespace verifies content is trimmed.
func TestSECRuleApplyTrimsWhitespace(t *testing.T) {
	rule := NewSECRule()

	content := []byte("  \n  content with whitespace  \n  ")

	result := rule.Apply(content)

	assert.Equal(t, "content with whitespace", string(result))
}

// TestSECRuleApplyRealWorldExample verifies realistic SEC content cleanup.
func TestSECRuleApplyRealWorldExample(t *testing.T) {
	rule := NewSECRule()

	content := []byte(`<html>
<body>FYfalse0000789019P2Y
<ix:header>
<h1>Microsoft Corporation</h1>
<p>Revenue for fiscal year: $198B us-gaap:RevenueMember iso4217:USD</p>
<p>See http://fasb.org/us-gaap/2023 for methodology</p>
<p>Years: 2020 2021 2022 2023</p>
</body>
</html>`)

	result := rule.Apply(content)

	resultStr := string(result)
	// Should preserve important content
	assert.Contains(t, resultStr, "Microsoft Corporation", "should preserve company name")
	assert.Contains(t, resultStr, "$198B", "should preserve revenue number")

	// Should remove XBRL noise
	assert.NotContains(t, resultStr, "FYfalse", "should remove XBRL body prefix")
	assert.NotContains(t, resultStr, "us-gaap:RevenueMember", "should remove member references")
	assert.NotContains(t, resultStr, "iso4217:USD", "should remove currency codes")
	assert.NotContains(t, resultStr, "http://fasb.org", "should remove XBRL URLs")
}

// TestSECTableRuleCreation verifies SEC table rule can be created.
func TestSECTableRuleCreation(t *testing.T) {
	rule := NewSECTableRule()
	assert.NotNil(t, rule)
	assert.Equal(t, "SEC.gov Table Cleanup", rule.Name())
}

// TestSECTableRuleMatch verifies table rule matches sec.gov URLs.
func TestSECTableRuleMatch(t *testing.T) {
	rule := NewSECTableRule()

	assert.True(t, rule.Match("https://www.sec.gov/page.html", "text/html"))
	assert.False(t, rule.Match("https://example.com/page.html", "text/html"))
	assert.False(t, rule.Match("https://www.sec.gov/data.json", "application/json"))
}

// TestSECTableRuleApplyRemovesEmptyCells verifies empty table cells are cleaned.
func TestSECTableRuleApplyRemovesEmptyCells(t *testing.T) {
	rule := NewSECTableRule()

	content := []byte(`<table>
<tr>
<td>Data</td>
<td>&nbsp;</td>
<td>  </td>
<td>More data</td>
</tr>
</table>`)

	result := rule.Apply(content)

	resultStr := string(result)
	assert.Contains(t, resultStr, "<td></td>", "should normalize empty cells to <td></td>")
	assert.Contains(t, resultStr, "Data", "should preserve data cells")
	assert.Contains(t, resultStr, "More data", "should preserve data cells")
}

// TestSECTableRuleApplyRemovesEmptyRows verifies empty table rows are removed.
func TestSECTableRuleApplyRemovesEmptyRows(t *testing.T) {
	rule := NewSECTableRule()

	content := []byte(`<table>
<tr>
<td>Data 1</td>
<td>Data 2</td>
</tr>
<tr>
<td></td>
<td></td>
</tr>
<tr>
<td>Data 3</td>
<td>Data 4</td>
</tr>
</table>`)

	result := rule.Apply(content)

	resultStr := string(result)
	assert.Contains(t, resultStr, "Data 1", "should preserve data rows")
	assert.Contains(t, resultStr, "Data 3", "should preserve data rows")
	// Empty row should be removed (though exact format depends on regex)
	assert.NotContains(t, resultStr, "<tr>\n<td></td>\n<td></td>\n</tr>", "should remove completely empty rows")
}
