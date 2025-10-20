package content

import (
	"strings"

	"github.com/joeychilson/websurfer/parser"
)

// DefaultMaxTokens is the default maximum tokens to return.
const DefaultMaxTokens = 20000

const (
	// htmlBoundaryWindowDivisor determines the search window size for HTML boundaries (1/10 = 10% of target chars).
	htmlBoundaryWindowDivisor = 10
	// wordBoundaryWindowDivisor determines the search window size for word boundaries (1/20 = 5% of target chars).
	wordBoundaryWindowDivisor = 20
)

var (
	// charsPerTokenRatios maps content types to their estimated chars-per-token ratio
	charsPerTokenRatios = map[string]float64{
		"text/html":             2.25,
		"application/xhtml+xml": 2.25,
		"text/plain":            2.0,
		"application/json":      3.0,
		"application/xml":       2.5,
		"text/xml":              2.5,
		"default":               3.0,
	}
)

// TruncateResult contains the truncation result.
type TruncateResult struct {
	// Content is the original content
	Content string `json:"content"`
	// Truncated is true if the content was truncated
	Truncated bool `json:"truncated"`
	// ReturnedChars is the number of characters returned
	ReturnedChars int `json:"returned_chars"`
	// ReturnedTokens is the number of tokens returned
	ReturnedTokens int `json:"returned_tokens"`
	// TotalChars is the total number of characters in the content
	TotalChars int `json:"total_chars"`
	// TotalTokens is the total number of tokens in the content
	TotalTokens int `json:"total_tokens"`
}

// Truncate truncates content to fit within maxTokens using smart boundaries.
func Truncate(content string, contentType string, maxTokens int) *TruncateResult {
	totalChars := len(content)
	totalTokens := EstimateTokens(content, contentType)

	if totalTokens <= maxTokens {
		return &TruncateResult{
			Content:        content,
			Truncated:      false,
			ReturnedChars:  totalChars,
			ReturnedTokens: totalTokens,
			TotalChars:     totalChars,
			TotalTokens:    totalTokens,
		}
	}

	targetChars := charsForTokens(maxTokens, contentType)

	truncateAt := findTruncationPoint(content, contentType, targetChars)

	truncated := content[:truncateAt]
	returnedTokens := EstimateTokens(truncated, contentType)

	return &TruncateResult{
		Content:        truncated,
		Truncated:      true,
		ReturnedChars:  truncateAt,
		ReturnedTokens: returnedTokens,
		TotalChars:     totalChars,
		TotalTokens:    totalTokens,
	}
}

// EstimateTokens estimates the number of tokens for given content.
func EstimateTokens(content string, contentType string) int {
	if content == "" {
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

// findTruncationPoint finds a smart boundary to truncate at.
func findTruncationPoint(content string, contentType string, targetChars int) int {
	if targetChars >= len(content) {
		return len(content)
	}

	ct := parser.NormalizeContentType(contentType)
	if strings.HasPrefix(ct, "text/html") ||
		strings.HasPrefix(ct, "application/xhtml") {
		return findHTMLBoundary(content, targetChars)
	}

	return findWordBoundary(content, targetChars)
}

// findHTMLBoundary finds a good HTML truncation point near targetChars.
func findHTMLBoundary(content string, targetChars int) int {
	window := targetChars / htmlBoundaryWindowDivisor
	searchStart := max(0, targetChars-window)
	searchEnd := min(len(content), targetChars+window)

	preferredTags := []string{
		"</article>", "</section>", "</div>", "</main>",
		"</header>", "</footer>", "</nav>", "</aside>",
		"</p>", "</li>", "</tr>", "</h1>", "</h2>", "</h3>",
		"</h4>", "</h5>", "</h6>", "</blockquote>", "</pre>",
	}

	bestPos := -1
	for _, tag := range preferredTags {
		pos := strings.LastIndex(content[searchStart:searchEnd], tag)
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

	pos := strings.LastIndex(content[:searchEnd], ">")
	if pos != -1 && pos > searchStart {
		return pos + 1
	}

	return findWordBoundary(content, targetChars)
}

// findWordBoundary finds a word boundary near targetChars.
func findWordBoundary(content string, targetChars int) int {
	window := targetChars / wordBoundaryWindowDivisor
	searchStart := max(0, targetChars-window)

	for i := targetChars; i >= searchStart; i-- {
		if i < len(content) && isWhitespace(content[i]) {
			return i
		}
	}

	return min(targetChars, len(content))
}

// isWhitespace checks if a character is whitespace.
func isWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}
