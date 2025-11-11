package content

import (
	"fmt"
	"strings"
)

// RangeOptions defines a range selection.
type RangeOptions struct {
	Type  string `json:"type"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// Navigation provides options for expanding or moving around content
type Navigation struct {
	Current *RangeOptions      `json:"current"`
	Options []NavigationOption `json:"options"`
}

// NavigationOption represents a single navigation action
type NavigationOption struct {
	ID          string        `json:"id"`
	Range       *RangeOptions `json:"range"`
	Description string        `json:"description"`
}

// ExtractRange extracts a specific range from content.
func ExtractRange(content string, opts *RangeOptions) (string, error) {
	if opts == nil {
		return content, nil
	}

	switch opts.Type {
	case "chars":
		return extractCharRange(content, opts.Start, opts.End)
	case "lines":
		return extractLineRange(content, opts.Start, opts.End)
	default:
		return "", fmt.Errorf("invalid range type: %s (must be 'chars' or 'lines')", opts.Type)
	}
}

// ExtractRangeBytes extracts a specific range from content as bytes.
func ExtractRangeBytes(content []byte, opts *RangeOptions) ([]byte, error) {
	if opts == nil {
		return content, nil
	}

	switch opts.Type {
	case "chars":
		return extractCharRangeBytes(content, opts.Start, opts.End)
	case "lines":
		return extractLineRangeBytes(content, opts.Start, opts.End)
	default:
		return nil, fmt.Errorf("invalid range type: %s (must be 'chars' or 'lines')", opts.Type)
	}
}

// extractCharRange extracts a character range.
func extractCharRange(content string, start, end int) (string, error) {
	contentLen := len(content)

	if start < 0 {
		start = 0
	}
	if end > contentLen {
		end = contentLen
	}
	if start >= end {
		return "", fmt.Errorf("invalid range: start (%d) must be less than end (%d)", start, end)
	}
	if start >= contentLen {
		return "", fmt.Errorf("start position (%d) exceeds content length (%d)", start, contentLen)
	}

	return content[start:end], nil
}

// extractLineRange extracts a line range.
func extractLineRange(content string, start, end int) (string, error) {
	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	if start < 0 {
		start = 0
	}
	if end > totalLines {
		end = totalLines
	}
	if start >= end {
		return "", fmt.Errorf("invalid range: start line (%d) must be less than end line (%d)", start, end)
	}
	if start >= totalLines {
		return "", fmt.Errorf("start line (%d) exceeds total lines (%d)", start, totalLines)
	}

	selectedLines := lines[start:end]
	return strings.Join(selectedLines, "\n"), nil
}

// extractCharRangeBytes extracts a character range from bytes.
func extractCharRangeBytes(content []byte, start, end int) ([]byte, error) {
	contentLen := len(content)

	if start < 0 {
		start = 0
	}
	if end > contentLen {
		end = contentLen
	}
	if start >= end {
		return nil, fmt.Errorf("invalid range: start (%d) must be less than end (%d)", start, end)
	}
	if start >= contentLen {
		return nil, fmt.Errorf("start position (%d) exceeds content length (%d)", start, contentLen)
	}

	return content[start:end], nil
}

// extractLineRangeBytes extracts a line range from bytes.
func extractLineRangeBytes(content []byte, start, end int) ([]byte, error) {
	lineStarts := []int{0}
	for i, b := range content {
		if b == '\n' && i < len(content)-1 {
			lineStarts = append(lineStarts, i+1)
		}
	}
	totalLines := len(lineStarts)

	if start < 0 {
		start = 0
	}
	if end > totalLines {
		end = totalLines
	}
	if start >= end {
		return nil, fmt.Errorf("invalid range: start line (%d) must be less than end line (%d)", start, end)
	}
	if start >= totalLines {
		return nil, fmt.Errorf("start line (%d) exceeds total lines (%d)", start, totalLines)
	}

	startPos := lineStarts[start]
	var endPos int
	if end < totalLines {
		endPos = lineStarts[end] - 1
	} else {
		endPos = len(content)
	}

	if endPos > len(content) {
		endPos = len(content)
	}

	return content[startPos:endPos], nil
}
