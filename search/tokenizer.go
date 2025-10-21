package search

import (
	"regexp"
	"strings"
	"unicode"
)

// stripHTML removes HTML tags from text for density calculation
func stripHTML(text string) string {
	tagPattern := regexp.MustCompile(`<[^>]*>`)
	stripped := tagPattern.ReplaceAllString(text, " ")

	spacePattern := regexp.MustCompile(`\s+`)
	stripped = spacePattern.ReplaceAllString(stripped, " ")

	return strings.TrimSpace(stripped)
}

// tokenize splits text into normalized tokens for BM25
func tokenize(text string) []string {
	text = strings.ToLower(text)

	var tokens []string
	var current strings.Builder

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				token := current.String()
				if len(token) >= 2 && !isStopWord(token) {
					tokens = append(tokens, token)
				}
				current.Reset()
			}
		}
	}

	if current.Len() > 0 {
		token := current.String()
		if len(token) >= 2 && !isStopWord(token) {
			tokens = append(tokens, token)
		}
	}

	return tokens
}

// isStopWord checks if a token is a common stop word
// Minimal list - only remove the most common words
func isStopWord(token string) bool {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true,
		"and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true,
		"to": true, "for": true, "of": true,
		"as": true, "is": true, "was": true,
		"be": true, "are": true, "by": true,
	}

	return stopWords[token]
}

// highlightMatches wraps matched terms in the content with **
func highlightMatches(content string, queryTokens []string) string {
	highlighted := content

	for _, token := range queryTokens {
		pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(token) + `\b`)
		highlighted = pattern.ReplaceAllStringFunc(highlighted, func(match string) string {
			return "**" + match + "**"
		})
	}

	return highlighted
}
