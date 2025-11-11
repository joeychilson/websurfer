package outline

import (
	"regexp"
	"strings"
)

// extractMarkdown extracts outline from Markdown content
func extractMarkdown(content string) *Outline {
	outline := &Outline{
		Headings: extractMarkdownHeadings(content),
		Tables:   extractMarkdownTables(content),
		Lists:    extractMarkdownLists(content),
	}

	return outline
}

// extractMarkdownHeadings extracts # headings from markdown
func extractMarkdownHeadings(content string) []Heading {
	headings := []Heading{}

	lines := strings.Split(content, "\n")
	charPos := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "#") {
			level := 0
			for i, char := range trimmed {
				if char == '#' {
					level++
				} else if char == ' ' {
					text := strings.TrimSpace(trimmed[i+1:])
					if text != "" {
						headings = append(headings, Heading{
							Level:     level,
							Text:      text,
							CharStart: charPos,
							CharEnd:   0,
						})
					}
					break
				} else {
					break
				}
			}
		}

		charPos += len(line) + 1
	}

	contentLen := len(content)
	for i := range headings {
		if i < len(headings)-1 {
			headings[i].CharEnd = headings[i+1].CharStart
		} else {
			headings[i].CharEnd = contentLen
		}
	}

	return headings
}

// extractMarkdownTables extracts table structures from markdown
func extractMarkdownTables(content string) []Table {
	tables := []Table{}

	lines := strings.Split(content, "\n")
	charPos := 0
	inTable := false
	tableStart := 0
	var headers []string
	rowCount := 0

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if !inTable {
			if strings.Contains(trimmed, "|") && trimmed != "" {
				if i+1 < len(lines) {
					nextLine := strings.TrimSpace(lines[i+1])
					if isTableSeparator(nextLine) {
						inTable = true
						tableStart = charPos
						headers = parseTableRow(trimmed)
						rowCount = 0
						charPos += len(line) + 1
						continue
					}
				}
			}
		} else {
			if strings.Contains(trimmed, "|") && trimmed != "" && !isTableSeparator(trimmed) {
				rowCount++
			} else if trimmed == "" || !strings.Contains(trimmed, "|") {
				tables = append(tables, Table{
					Headers:   headers,
					RowCount:  rowCount,
					CharStart: tableStart,
					CharEnd:   charPos,
				})
				inTable = false
			}
		}

		charPos += len(line) + 1
	}

	if inTable {
		tables = append(tables, Table{
			Headers:   headers,
			RowCount:  rowCount,
			CharStart: tableStart,
			CharEnd:   charPos,
		})
	}

	return tables
}

// isTableSeparator checks if line is a markdown table separator (---|---|---)
func isTableSeparator(line string) bool {
	cleaned := strings.ReplaceAll(strings.ReplaceAll(line, " ", ""), ":", "")
	for _, char := range cleaned {
		if char != '|' && char != '-' {
			return false
		}
	}
	return strings.Contains(cleaned, "-")
}

// parseTableRow extracts cells from a markdown table row
func parseTableRow(line string) []string {
	cells := []string{}

	parts := strings.SplitSeq(line, "|")
	for part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			cells = append(cells, trimmed)
		}
	}

	return cells
}

// extractMarkdownLists extracts lists from markdown
func extractMarkdownLists(content string) []List {
	lists := []List{}

	lines := strings.Split(content, "\n")
	charPos := 0
	inList := false
	listStart := 0
	listType := ""
	items := []string{}
	itemCount := 0

	orderedRegex := regexp.MustCompile(`^\s*\d+\.\s+(.+)`)
	unorderedRegex := regexp.MustCompile(`^\s*[-*+]\s+(.+)`)

	for _, line := range lines {
		var match []string
		var currentType string

		if match = orderedRegex.FindStringSubmatch(line); match != nil {
			currentType = "ordered"
		} else if match = unorderedRegex.FindStringSubmatch(line); match != nil {
			currentType = "unordered"
		}

		if match != nil {
			if !inList {
				inList = true
				listStart = charPos
				listType = currentType
				items = []string{}
				itemCount = 0
			} else if currentType != listType {
				lists = append(lists, List{
					Type:      listType,
					Items:     items,
					ItemCount: itemCount,
					CharStart: listStart,
					CharEnd:   charPos,
				})
				listStart = charPos
				listType = currentType
				items = []string{}
				itemCount = 0
			}

			itemCount++
			if len(items) < 3 {
				items = append(items, match[1])
			}
		} else if inList && strings.TrimSpace(line) == "" {
			lists = append(lists, List{
				Type:      listType,
				Items:     items,
				ItemCount: itemCount,
				CharStart: listStart,
				CharEnd:   charPos,
			})
			inList = false
		}

		charPos += len(line) + 1
	}

	if inList {
		lists = append(lists, List{
			Type:      listType,
			Items:     items,
			ItemCount: itemCount,
			CharStart: listStart,
			CharEnd:   charPos,
		})
	}

	return lists
}
