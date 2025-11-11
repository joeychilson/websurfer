package search

import (
	"github.com/joeychilson/websurfer/content"
)

// Navigation provides options for expanding or moving around search results
type Navigation struct {
	Current *content.RangeOptions `json:"current"`
	Options []NavigationOption    `json:"options"`
}

// NavigationOption represents a single navigation action
type NavigationOption struct {
	ID          string                `json:"id"`
	Range       *content.RangeOptions `json:"range"`
	Description string                `json:"description"`
}

// Options contains search parameters
type Options struct {
	Query      string  `json:"query"`
	WindowSize int     `json:"window_size,omitempty"`
	MaxResults int     `json:"max_results,omitempty"`
	MinScore   float64 `json:"min_score,omitempty"`
	Highlight  bool    `json:"highlight,omitempty"`
}

// Result contains search results
type Result struct {
	Query           string        `json:"query"`
	TotalMatches    int           `json:"total_matches"`
	ReturnedMatches int           `json:"returned_matches"`
	Results         []Match       `json:"results"`
	Continuation    *Continuation `json:"next,omitempty"`
}

// Match represents a single search result
type Match struct {
	Rank            int         `json:"rank"`
	Score           float64     `json:"score"`
	Location        Location    `json:"location"`
	Snippet         string      `json:"snippet"`
	EstimatedTokens int         `json:"estimated_tokens"`
	Navigation      *Navigation `json:"navigation,omitempty"`
}

// Location describes where the match was found
type Location struct {
	CharStart   int    `json:"char_start"`
	CharEnd     int    `json:"char_end"`
	LineStart   int    `json:"line_start"`
	LineEnd     int    `json:"line_end"`
	SectionPath string `json:"section_path,omitempty"`
}

// Continuation provides pagination info
type Continuation struct {
	Offset           int `json:"offset"`
	RemainingMatches int `json:"remaining_matches"`
}

// Search performs window-based search on content and returns ranked results
func Search(contentText, contentType string, opts Options) (*Result, error) {
	if opts.WindowSize == 0 {
		opts.WindowSize = 10000
	}
	if opts.MaxResults == 0 {
		opts.MaxResults = 10
	}

	queryTokens := tokenize(opts.Query)
	if len(queryTokens) == 0 {
		return &Result{
			Query:           opts.Query,
			TotalMatches:    0,
			ReturnedMatches: 0,
			Results:         []Match{},
		}, nil
	}

	matches := findMatches(contentText, queryTokens)
	if len(matches) == 0 {
		return &Result{
			Query:           opts.Query,
			TotalMatches:    0,
			ReturnedMatches: 0,
			Results:         []Match{},
		}, nil
	}

	windows := createWindows(contentText, matches, opts.WindowSize)

	windows = mergeOverlappingWindows(windows)

	for i := range windows {
		if windows[i].Content == "" {
			windows[i].Content = contentText[windows[i].Start:windows[i].End]
		}
	}

	scoreWindows(windows, queryTokens)

	if opts.MinScore > 0 {
		filtered := []Window{}
		for _, w := range windows {
			if w.Score >= opts.MinScore {
				filtered = append(filtered, w)
			}
		}
		windows = filtered
	}

	if len(windows) > opts.MaxResults {
		windows = windows[:opts.MaxResults]
	}

	contentLen := len(contentText)
	results := make([]Match, len(windows))
	for i, window := range windows {
		if window.Content == "" {
			window.Content = contentText[window.Start:window.End]
		}

		snippet := window.Content
		if opts.Highlight {
			snippet = highlightMatches(snippet, queryTokens)
		}

		estimatedTokens := content.EstimateTokens(snippet, contentType)

		results[i] = Match{
			Rank:  i + 1,
			Score: window.Score,
			Location: Location{
				CharStart: window.Start,
				CharEnd:   window.End,
				LineStart: window.LineStart,
				LineEnd:   window.LineEnd,
			},
			Snippet:         snippet,
			EstimatedTokens: estimatedTokens,
			Navigation:      buildNavigation(window.Start, window.End, contentLen, opts.WindowSize),
		}
	}

	return &Result{
		Query:           opts.Query,
		TotalMatches:    len(windows),
		ReturnedMatches: len(results),
		Results:         results,
	}, nil
}

// buildNavigation creates navigation options for a window
func buildNavigation(start, end, totalLength, windowSize int) *Navigation {
	nav := &Navigation{
		Current: &content.RangeOptions{
			Type:  "chars",
			Start: start,
			End:   end,
		},
		Options: []NavigationOption{},
	}

	expandAmount := windowSize

	if start > 0 {
		expandStart := max(0, start-expandAmount)
		nav.Options = append(nav.Options, NavigationOption{
			ID: "expand_up",
			Range: &content.RangeOptions{
				Type:  "chars",
				Start: expandStart,
				End:   end,
			},
			Description: "Expand window to include content above",
		})
	}

	if end < totalLength {
		expandEnd := min(totalLength, end+expandAmount)
		nav.Options = append(nav.Options, NavigationOption{
			ID: "expand_down",
			Range: &content.RangeOptions{
				Type:  "chars",
				Start: start,
				End:   expandEnd,
			},
			Description: "Expand window to include content below",
		})
	}

	if start > 0 || end < totalLength {
		expandStart := max(0, start-expandAmount)
		expandEnd := min(totalLength, end+expandAmount)
		nav.Options = append(nav.Options, NavigationOption{
			ID: "expand_both",
			Range: &content.RangeOptions{
				Type:  "chars",
				Start: expandStart,
				End:   expandEnd,
			},
			Description: "Expand window in both directions",
		})
	}

	return nav
}
