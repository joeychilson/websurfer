package search

import (
	"github.com/joeychilson/websurfer/content"
)

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
	Rank            int      `json:"rank"`
	Score           float64  `json:"score"`
	Location        Location `json:"location"`
	Snippet         string   `json:"snippet"`
	EstimatedTokens int      `json:"estimated_tokens"`
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
		opts.WindowSize = 2000
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

	results := make([]Match, len(windows))
	for i, window := range windows {
		if window.Content == "" {
			window.Content = contentText[window.Start:window.End]
		}

		snippet := window.Content
		if opts.Highlight {
			snippet = highlightMatches(snippet, queryTokens)
		}

		estimatedTokens := estimateTokens(snippet, contentType)

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
		}
	}

	return &Result{
		Query:           opts.Query,
		TotalMatches:    len(windows),
		ReturnedMatches: len(results),
		Results:         results,
	}, nil
}

func estimateTokens(text, contentType string) int {
	return content.EstimateTokens(text, contentType)
}
