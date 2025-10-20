package rules

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/joeychilson/websurfer/parser"
)

var (
	// XBRL all namespace-prefixed tags (ix:, xbrli:, xbrldi:, link:, etc.)
	xbrlTagsRegex = regexp.MustCompile(`(?i)</?[a-z]+:[^>]+>`)
	// XBRL context and namespace declarations in body
	xbrlBodyPrefixRegex = regexp.MustCompile(`(?i)^[A-Z]{2,}(false|true)?[0-9]{10,}[A-Z0-9:/#\-.]*`)
	// XBRL namespace URLs (http://fasb.org, http://xbrl.org, etc.)
	xbrlURLsRegex = regexp.MustCompile(`https?://[^\s]*?(xbrl|fasb|sec\.gov/dei)[^\s]*`)
	// XBRL member references (us-gaap:SomethingMember, msft:SomethingMember)
	xbrlMembersRegex = regexp.MustCompile(`[a-z-]+:[A-Za-z]+Member`)
	// ISO currency and date codes that appear at end of XBRL blocks
	xbrlMetadataRegex = regexp.MustCompile(`(iso4217:[A-Z]{3}|xbrli:(pure|shares))`)
	// Year ranges and date patterns common in XBRL
	xbrlDateRangesRegex = regexp.MustCompile(`\b(20\d{2}\s+){3,}`)
	// HTTP URL patterns for XBRL namespaces
	xbrlHTTPPatternRegex = regexp.MustCompile(`https?://[a-z]+\.[a-z]+/[a-z/-]+/\d{4}#[A-Za-z]+`)
	// Body tag regex for cleaning
	bodyTagRegex = regexp.MustCompile(`(?i)<body[^>]*>`)
	// Empty table cell regex
	emptyCellRegex = regexp.MustCompile(`<td[^>]*>\s*(&nbsp;|\s)*\s*</td>`)
	// Empty table row regex
	emptyRowRegex = regexp.MustCompile(`<tr[^>]*>(\s|<td></td>)*</tr>`)
)

// SECRule removes XBRL tags and SEC.gov specific markup from HTML content.
// Only applies to text/html content from sec.gov domains.
type SECRule struct{}

// NewSECRule creates a new SEC.gov cleanup rule for HTML content.
func NewSECRule() *SECRule {
	return &SECRule{}
}

// Match returns true for sec.gov URLs with HTML content.
func (r *SECRule) Match(urlStr, contentType string) bool {
	ct := parser.NormalizeContentType(contentType)
	if ct != "text/html" && ct != "application/xhtml+xml" {
		return false
	}

	urlLower := strings.ToLower(urlStr)
	return strings.Contains(urlLower, "sec.gov")
}

// Apply removes XBRL tags and cleans up SEC-specific content.
func (r *SECRule) Apply(content []byte) []byte {
	result := content
	result = xbrlTagsRegex.ReplaceAll(result, []byte(""))
	result = xbrlURLsRegex.ReplaceAll(result, []byte(""))
	result = xbrlMembersRegex.ReplaceAll(result, []byte(""))
	result = xbrlMetadataRegex.ReplaceAll(result, []byte(""))
	result = xbrlHTTPPatternRegex.ReplaceAll(result, []byte(""))
	result = xbrlDateRangesRegex.ReplaceAll(result, []byte(""))
	result = cleanBodyTag(result)
	result = bytes.ReplaceAll(result, []byte("  "), []byte(" "))
	result = bytes.TrimSpace(result)
	return result
}

// Name returns the rule's name.
func (r *SECRule) Name() string {
	return "SEC.gov XBRL Cleanup"
}

// cleanBodyTag removes XBRL prefix garbage from body tags.
// Example: <body>FYfalse0000789019P2Y... becomes <body>
func cleanBodyTag(content []byte) []byte {
	bodyTagMatch := bodyTagRegex.FindIndex(content)
	if bodyTagMatch == nil {
		return content
	}

	bodyStartIdx := bodyTagMatch[1]

	bodyEndIdx := bytes.Index(content, []byte("</body>"))
	if bodyEndIdx == -1 {
		return content
	}

	bodyContent := content[bodyStartIdx:bodyEndIdx]

	firstTagIdx := bytes.IndexByte(bodyContent, '<')
	if firstTagIdx > 10 {
		prefix := bodyContent[:firstTagIdx]
		if xbrlBodyPrefixRegex.Match(prefix) || bytes.Contains(prefix, []byte("false")) || bytes.Contains(prefix, []byte("true")) {
			var result bytes.Buffer
			result.Write(content[:bodyStartIdx])
			result.Write(bodyContent[firstTagIdx:])
			result.Write(content[bodyEndIdx:])
			return result.Bytes()
		}
	}

	return content
}

// SECTableRule cleans up SEC financial tables in HTML content.
// Only applies to text/html content from sec.gov domains.
type SECTableRule struct{}

// NewSECTableRule creates a rule for SEC table cleanup.
func NewSECTableRule() *SECTableRule {
	return &SECTableRule{}
}

// Match returns true for sec.gov URLs with HTML content.
func (r *SECTableRule) Match(urlStr, contentType string) bool {
	ct := parser.NormalizeContentType(contentType)
	if ct != "text/html" && ct != "application/xhtml+xml" {
		return false
	}

	urlLower := strings.ToLower(urlStr)
	return strings.Contains(urlLower, "sec.gov")
}

// Apply removes empty table cells and cleans up table formatting.
func (r *SECTableRule) Apply(content []byte) []byte {
	result := emptyCellRegex.ReplaceAll(content, []byte("<td></td>"))
	result = emptyRowRegex.ReplaceAll(result, []byte(""))

	return result
}

// Name returns the rule's name.
func (r *SECTableRule) Name() string {
	return "SEC.gov Table Cleanup"
}
