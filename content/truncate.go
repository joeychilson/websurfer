package content

import (
	"bytes"
	"strings"

	"github.com/joeychilson/websurfer/parser"
)

const (
	htmlBoundaryWindowDivisor = 10
	wordBoundaryWindowDivisor = 20
)

var (
	// charsPerTokenRatios defines the estimated characters per token for different content types.
	// These ratios are empirically derived from typical content patterns:
	// - HTML/XHTML (1.9): Lower ratio due to markup overhead (tags, attributes)
	// - Plain text (1.7): Slightly lower due to natural language patterns
	// - JSON (2.5): Higher ratio due to structured format with punctuation
	// - XML (2.1): Moderate ratio with structured tags
	// - Default (2.5): Conservative estimate for unknown types
	charsPerTokenRatios = map[string]float64{
		"text/html":             1.9,
		"application/xhtml+xml": 1.9,
		"text/plain":            1.7,
		"application/json":      2.5,
		"application/xml":       2.1,
		"text/xml":              2.1,
		"default":               2.5,
	}
)

// TruncateResult contains the truncation result.
type TruncateResult struct {
	Content        string `json:"content"`
	Truncated      bool   `json:"truncated"`
	ReturnedChars  int    `json:"returned_chars"`
	ReturnedTokens int    `json:"returned_tokens"`
	TotalChars     int    `json:"total_chars"`
	TotalTokens    int    `json:"total_tokens"`
}

// TruncateBytes truncates content to fit within maxTokens using smart boundaries.
func TruncateBytes(content []byte, contentType string, maxTokens int) *TruncateResult {
	totalChars := len(content)
	totalTokens := EstimateTokensBytes(content, contentType)

	if totalTokens <= maxTokens {
		return &TruncateResult{
			Content:        string(content),
			Truncated:      false,
			ReturnedChars:  totalChars,
			ReturnedTokens: totalTokens,
			TotalChars:     totalChars,
			TotalTokens:    totalTokens,
		}
	}

	targetChars := charsForTokens(maxTokens, contentType)

	truncateAt := findTruncationPointBytes(content, contentType, targetChars)

	truncated := content[:truncateAt]
	returnedTokens := EstimateTokensBytes(truncated, contentType)

	return &TruncateResult{
		Content:        string(truncated),
		Truncated:      true,
		ReturnedChars:  truncateAt,
		ReturnedTokens: returnedTokens,
		TotalChars:     totalChars,
		TotalTokens:    totalTokens,
	}
}

// EstimateTokensBytes estimates the number of tokens for given content as bytes.
func EstimateTokensBytes(content []byte, contentType string) int {
	if len(content) == 0 {
		return 0
	}

	ct := parser.NormalizeContentType(contentType)

	ratio, exists := charsPerTokenRatios[ct]
	if !exists {
		ratio = charsPerTokenRatios["default"]
	}

	return int(float64(len(content)) / ratio)
}

// charsForTokens calculates how many characters are needed for target token count.
func charsForTokens(targetTokens int, contentType string) int {
	ct := parser.NormalizeContentType(contentType)

	ratio, exists := charsPerTokenRatios[ct]
	if !exists {
		ratio = charsPerTokenRatios["default"]
	}

	return int(float64(targetTokens) * ratio)
}

// isWhitespace checks if a character is whitespace.
func isWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

// findTruncationPointBytes finds a smart boundary to truncate at for bytes.
func findTruncationPointBytes(content []byte, contentType string, targetChars int) int {
	if targetChars >= len(content) {
		return len(content)
	}

	ct := parser.NormalizeContentType(contentType)
	if strings.HasPrefix(ct, "text/html") ||
		strings.HasPrefix(ct, "application/xhtml") {
		return findHTMLBoundaryBytes(content, targetChars)
	}

	return findWordBoundaryBytes(content, targetChars)
}

// findHTMLBoundaryBytes finds a good HTML truncation point near targetChars for bytes.
func findHTMLBoundaryBytes(content []byte, targetChars int) int {
	window := targetChars / htmlBoundaryWindowDivisor
	searchStart := max(0, targetChars-window)
	searchEnd := min(len(content), targetChars+window)

	preferredTags := [][]byte{
		[]byte("</article>"), []byte("</section>"), []byte("</div>"), []byte("</main>"),
		[]byte("</header>"), []byte("</footer>"), []byte("</nav>"), []byte("</aside>"),
		[]byte("</p>"), []byte("</li>"), []byte("</tr>"), []byte("</h1>"), []byte("</h2>"), []byte("</h3>"),
		[]byte("</h4>"), []byte("</h5>"), []byte("</h6>"), []byte("</blockquote>"), []byte("</pre>"),
	}

	bestPos := -1
	for _, tag := range preferredTags {
		pos := bytes.LastIndex(content[searchStart:searchEnd], tag)
		if pos != -1 {
			absPos := searchStart + pos + len(tag)
			if absPos > bestPos {
				bestPos = absPos
			}
		}
	}

	if bestPos != -1 {
		return bestPos
	}

	pos := bytes.LastIndexByte(content[:searchEnd], '>')
	if pos != -1 && pos > searchStart {
		return pos + 1
	}

	return findWordBoundaryBytes(content, targetChars)
}

// findWordBoundaryBytes finds a word boundary near targetChars for bytes.
func findWordBoundaryBytes(content []byte, targetChars int) int {
	window := targetChars / wordBoundaryWindowDivisor
	searchStart := max(0, targetChars-window)

	for i := targetChars; i >= searchStart; i-- {
		if i < len(content) && isWhitespace(content[i]) {
			return i
		}
	}

	return min(targetChars, len(content))
}
