package search

import (
	"sort"
	"strings"
)

// MatchPosition represents a position where a query term was found
type MatchPosition struct {
	Term     string
	Position int
}

// Window represents a text window containing matches
type Window struct {
	Start       int
	End         int
	Matches     []MatchPosition
	Content     string
	Score       float64
	LineStart   int
	LineEnd     int
	SectionPath string
}

// findMatches finds all positions where query terms appear in the content
func findMatches(content string, queryTokens []string) []MatchPosition {
	contentLower := strings.ToLower(content)
	matches := []MatchPosition{}

	for _, token := range queryTokens {
		tokenLower := strings.ToLower(token)
		pos := 0

		for {
			idx := strings.Index(contentLower[pos:], tokenLower)
			if idx == -1 {
				break
			}

			actualPos := pos + idx
			matches = append(matches, MatchPosition{
				Term:     token,
				Position: actualPos,
			})

			pos = actualPos + 1
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Position < matches[j].Position
	})

	return matches
}

// createWindows creates windows around match positions
func createWindows(content string, matches []MatchPosition, windowSize int) []Window {
	if len(matches) == 0 {
		return nil
	}

	windows := []Window{}
	contentLen := len(content)

	used := make(map[int]bool)

	for i, match := range matches {
		if used[i] {
			continue
		}

		start := max(0, match.Position-windowSize/2)
		end := min(contentLen, match.Position+windowSize/2)

		start = expandToWordBoundary(content, start, false)
		end = expandToWordBoundary(content, end, true)

		windowMatches := []MatchPosition{}
		for j := i; j < len(matches); j++ {
			if matches[j].Position >= start && matches[j].Position < end {
				windowMatches = append(windowMatches, matches[j])
				used[j] = true
			} else if matches[j].Position >= end {
				break
			}
		}

		lineStart := strings.Count(content[:start], "\n")
		lineEnd := strings.Count(content[:end], "\n")

		windows = append(windows, Window{
			Start:     start,
			End:       end,
			Matches:   windowMatches,
			Content:   content[start:end],
			LineStart: lineStart,
			LineEnd:   lineEnd,
		})
	}

	return windows
}

// expandToWordBoundary expands position to nearest word boundary
func expandToWordBoundary(content string, pos int, forward bool) int {
	if pos <= 0 {
		return 0
	}
	if pos >= len(content) {
		return len(content)
	}

	if forward {
		for pos < len(content) {
			c := content[pos]
			if c == ' ' || c == '\n' || c == '\t' || c == '.' || c == ',' || c == ';' || c == '>' {
				return pos
			}
			pos++
		}
		return len(content)
	} else {
		for pos > 0 {
			c := content[pos-1]
			if c == ' ' || c == '\n' || c == '\t' || c == '.' || c == ',' || c == ';' || c == '<' {
				return pos
			}
			pos--
		}
		return 0
	}
}

// countPhraseMatches counts how many times query tokens appear together as a phrase
func countPhraseMatches(content string, queryTokens []string) int {
	if len(queryTokens) == 0 {
		return 0
	}

	contentLower := strings.ToLower(content)
	count := 0

	for i := 0; i < len(queryTokens)-1; i++ {
		term1 := strings.ToLower(queryTokens[i])
		term2 := strings.ToLower(queryTokens[i+1])

		pos := 0
		for {
			idx := strings.Index(contentLower[pos:], term1)
			if idx == -1 {
				break
			}
			actualPos := pos + idx

			searchEnd := min(len(contentLower), actualPos+len(term1)+50)
			remaining := contentLower[actualPos+len(term1) : searchEnd]

			if strings.Contains(remaining, term2) {
				count++
			}

			pos = actualPos + 1
		}
	}

	return count
}

// scoreWindows calculates density-based scores for windows
func scoreWindows(windows []Window, queryTokens []string) {
	for i := range windows {
		window := &windows[i]

		uniqueTerms := make(map[string]bool)
		for _, match := range window.Matches {
			uniqueTerms[strings.ToLower(match.Term)] = true
		}

		plainText := stripHTML(window.Content)
		windowSize := float64(len(plainText))
		if windowSize == 0 {
			windowSize = 1
		}
		matchCount := float64(len(window.Matches))
		density := (matchCount / windowSize) * 1000

		coverage := float64(len(uniqueTerms)) / float64(len(queryTokens))
		coverageBoost := coverage * coverage * 10

		proximityBoost := 0.0
		if len(queryTokens) > 1 {
			phraseCount := countPhraseMatches(window.Content, queryTokens)
			proximityBoost = float64(phraseCount) * 50.0
		}

		contentBoost := 0.0
		if windowSize > 1000 {
			contentBoost = 5.0
		}
		if windowSize > 3000 {
			contentBoost = 10.0
		}

		structureBoost := 0.0
		contentLower := strings.ToLower(window.Content)
		if strings.Contains(contentLower, "<table") {
			structureBoost += 20.0
		}
		if strings.Contains(contentLower, "<tbody") || strings.Contains(contentLower, "<thead") {
			structureBoost += 10.0
		}
		if strings.Contains(contentLower, "<ul") || strings.Contains(contentLower, "<ol") {
			structureBoost += 5.0
		}

		window.Score = density + coverageBoost + proximityBoost + contentBoost + structureBoost
	}

	sort.Slice(windows, func(i, j int) bool {
		return windows[i].Score > windows[j].Score
	})
}

// mergeOverlappingWindows merges windows that overlap
func mergeOverlappingWindows(windows []Window) []Window {
	if len(windows) <= 1 {
		return windows
	}

	sort.Slice(windows, func(i, j int) bool {
		return windows[i].Start < windows[j].Start
	})

	merged := []Window{windows[0]}

	for i := 1; i < len(windows); i++ {
		current := windows[i]
		last := &merged[len(merged)-1]

		if current.Start <= last.End+100 {
			last.End = max(last.End, current.End)
			last.Matches = append(last.Matches, current.Matches...)
			last.Content = ""
			last.LineEnd = current.LineEnd
		} else {
			merged = append(merged, current)
		}
	}

	return merged
}
