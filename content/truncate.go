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
func Truncate(content []byte, contentType string, maxTokens int) *TruncateResult {
	totalChars := len(content)
	totalTokens := EstimateTokens(content, contentType)

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

	truncateAt := findTruncationPoint(content, contentType, targetChars)

	truncated := content[:truncateAt]
	returnedTokens := EstimateTokens(truncated, contentType)

	return &TruncateResult{
		Content:        string(truncated),
		Truncated:      true,
		ReturnedChars:  truncateAt,
		ReturnedTokens: returnedTokens,
		TotalChars:     totalChars,
		TotalTokens:    totalTokens,
	}
}

// isWhitespace checks if a character is whitespace.
func isWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

// findTruncationPointBytes finds a smart boundary to truncate at for bytes.
func findTruncationPoint(content []byte, contentType string, targetChars int) int {
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

// findHTMLBoundary finds a good HTML truncation point near targetChars for bytes.
func findHTMLBoundary(content []byte, targetChars int) int {
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

	return findWordBoundary(content, targetChars)
}

// findWordBoundary finds a word boundary near targetChars for bytes.
func findWordBoundary(content []byte, targetChars int) int {
	window := targetChars / wordBoundaryWindowDivisor
	searchStart := max(0, targetChars-window)

	for i := targetChars; i >= searchStart; i-- {
		if i < len(content) && isWhitespace(content[i]) {
			return i
		}
	}

	return min(targetChars, len(content))
}
