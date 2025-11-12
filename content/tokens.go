package content

import "github.com/joeychilson/websurfer/parser"

var (
	// charsPerTokenRatios defines the estimated characters per token for different content types.
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

// EstimateTokens estimates the number of tokens for given content as bytes.
func EstimateTokens(content []byte, contentType string) int {
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
