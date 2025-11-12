package content

import (
	"bytes"
	"strings"
	"unicode/utf8"

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
	NextOffset     int    `json:"next_offset"`
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
			NextOffset:     0,
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
		NextOffset:     truncateAt,
	}
}

// isWhitespace checks if a character is whitespace.
func isWhitespace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

// isInsideMarkdownTable checks if a position is inside a markdown table.
// Returns true if the line contains table pipe characters.
func isInsideMarkdownTable(content []byte, pos int) bool {
	if pos <= 0 || pos >= len(content) {
		return false
	}

	lineStart := pos
	for lineStart > 0 && content[lineStart-1] != '\n' {
		lineStart--
	}

	lineEnd := pos
	for lineEnd < len(content) && content[lineEnd] != '\n' {
		lineEnd++
	}

	line := content[lineStart:lineEnd]

	pipeCount := bytes.Count(line, []byte("|"))
	return pipeCount >= 2
}

// findEndOfTableRow moves position to the end of the current table row.
// This ensures we don't truncate in the middle of a markdown table row.
func findEndOfTableRow(content []byte, pos int) int {
	if pos >= len(content) {
		return pos
	}

	for pos < len(content) && content[pos] != '\n' {
		pos++
	}

	if pos < len(content) && content[pos] == '\n' {
		pos++
	}

	return pos
}

// adjustToUTF8Boundary moves a position backward to the nearest valid UTF-8 character boundary.
// This prevents splitting multi-byte UTF-8 characters (emoji, CJK, etc.)
func adjustToUTF8Boundary(content []byte, pos int) int {
	if pos <= 0 || pos >= len(content) {
		return pos
	}

	if utf8.RuneStart(content[pos]) {
		return pos
	}

	for i := pos - 1; i >= 0 && i > pos-4; i-- {
		if utf8.RuneStart(content[i]) {
			_, size := utf8.DecodeRune(content[i:])
			if i+size <= pos {
				return pos
			}
			return i
		}
	}

	return pos
}

// findTruncationPointBytes finds a smart boundary to truncate at for bytes.
func findTruncationPoint(content []byte, contentType string, targetChars int) int {
	if targetChars >= len(content) {
		return len(content)
	}

	ct := parser.NormalizeContentType(contentType)
	var pos int
	if strings.HasPrefix(ct, "text/html") ||
		strings.HasPrefix(ct, "application/xhtml") {
		pos = findHTMLBoundary(content, targetChars)
	} else {
		pos = findWordBoundary(content, targetChars)
	}

	if isInsideMarkdownTable(content, pos) {
		pos = findEndOfTableRow(content, pos)
	}

	return adjustToUTF8Boundary(content, pos)
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
