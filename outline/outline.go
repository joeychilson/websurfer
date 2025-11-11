package outline

import "strings"

// Outline represents a structured summary of document content
type Outline struct {
	Headings []Heading `json:"headings,omitempty"`
	Tables   []Table   `json:"tables,omitempty"`
	Lists    []List    `json:"lists,omitempty"`
}

// Heading represents a document heading
type Heading struct {
	Level     int    `json:"level"`
	Text      string `json:"text"`
	CharStart int    `json:"char_start"`
	CharEnd   int    `json:"char_end"`
}

// Table represents a table structure
type Table struct {
	Headers   []string `json:"headers"`
	RowCount  int      `json:"row_count"`
	CharStart int      `json:"char_start"`
	CharEnd   int      `json:"char_end"`
	Caption   string   `json:"caption,omitempty"`
}

// List represents a list structure
type List struct {
	Type      string   `json:"type"`
	Items     []string `json:"items,omitempty"`
	ItemCount int      `json:"item_count"`
	CharStart int      `json:"char_start"`
	CharEnd   int      `json:"char_end"`
}

// Extract generates an outline from content based on content type
func Extract(content, contentType string) *Outline {
	if isMarkdown(contentType) {
		return extractMarkdown(content)
	}

	return &Outline{}
}

// ExtractBytes generates an outline from content bytes based on content type.
func ExtractBytes(content []byte, contentType string) *Outline {
	if isMarkdown(contentType) {
		return extractMarkdown(string(content))
	}

	return &Outline{}
}

func isMarkdown(contentType string) bool {
	return strings.Contains(contentType, "markdown") ||
		strings.Contains(contentType, "text/markdown") ||
		strings.Contains(contentType, "text/x-markdown")
}
