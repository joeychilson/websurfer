package search

import (
	"strings"
	"testing"
)

func TestTokenize(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "simple words",
			input:    "hello world test",
			expected: []string{"hello", "world", "test"},
		},
		{
			name:     "mixed case",
			input:    "Hello WORLD TeSt",
			expected: []string{"hello", "world", "test"},
		},
		{
			name:     "with punctuation",
			input:    "Hello, world! How are you?",
			expected: []string{"hello", "world", "how", "you"},
		},
		{
			name:     "stop words removed",
			input:    "the quick brown fox",
			expected: []string{"quick", "brown", "fox"},
		},
		{
			name:     "income statements",
			input:    "income statements",
			expected: []string{"income", "statements"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tokenize(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d tokens, got %d: %v", len(tt.expected), len(result), result)
			}
			for i, token := range result {
				if token != tt.expected[i] {
					t.Errorf("token %d: expected %q, got %q", i, tt.expected[i], token)
				}
			}
		})
	}
}

func TestSearchPlainText(t *testing.T) {
	content := `This is a document about information retrieval.

BM25 is one of the most popular ranking functions.

It uses term frequency and document length normalization.

BM25 has been widely adopted in search engines.`

	opts := Options{
		Query:      "BM25 ranking",
		WindowSize: 200,
		MaxResults: 5,
		Highlight:  true,
	}

	result, err := Search(content, "text/plain", opts)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if result.TotalMatches == 0 {
		t.Fatal("expected at least one match")
	}

	if opts.Highlight && len(result.Results) > 0 {
		snippet := result.Results[0].Snippet
		if !strings.Contains(snippet, "**") {
			t.Error("expected highlighted matches to contain **")
		}
	}

	if len(result.Results) > 1 {
		if result.Results[1].Score > result.Results[0].Score {
			t.Error("results not sorted by score")
		}
	}
}

func TestSearchHTML(t *testing.T) {
	content := `<html>
		<h1>Introduction to Search</h1>
		<p>Search engines use various ranking algorithms.</p>
		<p>BM25 is a popular algorithm for search ranking.</p>
		<table>
			<tr><th>Algorithm</th><th>Type</th></tr>
			<tr><td>BM25</td><td>Ranking</td></tr>
		</table>
	</html>`

	opts := Options{
		Query:      "BM25 ranking",
		WindowSize: 300,
		MaxResults: 10,
		Highlight:  true,
	}

	result, err := Search(content, "text/html", opts)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if result.TotalMatches == 0 {
		t.Fatal("expected at least one match")
	}

	if len(result.Results) > 0 {
		snippet := result.Results[0].Snippet
		if !strings.Contains(snippet, "**") {
			t.Error("expected highlighted matches")
		}

		if !strings.Contains(snippet, "<") {
			t.Error("expected HTML tags to be preserved in snippet")
		}
	}
}

func TestSearchWithMinScore(t *testing.T) {
	content := "BM25 is great. This is another sentence about something else. And one more sentence."

	opts := Options{
		Query:      "BM25",
		WindowSize: 100,
		MaxResults: 10,
		MinScore:   5.0,
		Highlight:  false,
	}

	result, err := Search(content, "text/plain", opts)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	for _, match := range result.Results {
		if match.Score < opts.MinScore {
			t.Errorf("match score %f is below min score %f", match.Score, opts.MinScore)
		}
	}
}

func TestSearchMaxResults(t *testing.T) {
	content := `BM25 paragraph 1.

	BM25 paragraph 2.

	BM25 paragraph 3.

	BM25 paragraph 4.

	BM25 paragraph 5.`

	opts := Options{
		Query:      "BM25",
		WindowSize: 50,
		MaxResults: 3,
		Highlight:  false,
	}

	result, err := Search(content, "text/plain", opts)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(result.Results) > opts.MaxResults {
		t.Errorf("expected max %d results, got %d", opts.MaxResults, len(result.Results))
	}
}

func TestHighlightMatches(t *testing.T) {
	content := "The quick brown fox"
	queryTokens := []string{"quick", "fox"}

	highlighted := highlightMatches(content, queryTokens)

	if !strings.Contains(highlighted, "**quick**") {
		t.Error("expected 'quick' to be highlighted")
	}
	if !strings.Contains(highlighted, "**fox**") {
		t.Error("expected 'fox' to be highlighted")
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	content := "Some content here"

	opts := Options{
		Query:      "",
		WindowSize: 100,
		MaxResults: 10,
	}

	result, err := Search(content, "text/plain", opts)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if result.TotalMatches != 0 {
		t.Error("expected 0 matches for empty query")
	}
}

func TestSearchNoMatches(t *testing.T) {
	content := "This is some content about cats"

	opts := Options{
		Query:      "xyzabc12345",
		WindowSize: 100,
		MaxResults: 10,
	}

	result, err := Search(content, "text/plain", opts)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if result.TotalMatches != 0 {
		t.Errorf("expected 0 matches for non-matching query, got %d", result.TotalMatches)
	}
}

func TestSearchMultipleTerms(t *testing.T) {
	content := `First paragraph about revenue.

	Second paragraph about growth.

	Third paragraph about revenue growth together.

	Fourth paragraph unrelated.`

	opts := Options{
		Query:      "revenue growth",
		WindowSize: 100,
		MaxResults: 10,
		Highlight:  false,
	}

	result, err := Search(content, "text/plain", opts)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if result.TotalMatches == 0 {
		t.Fatal("expected at least one match")
	}

	if len(result.Results) > 0 {
		topResult := result.Results[0].Snippet
		if !strings.Contains(strings.ToLower(topResult), "revenue") ||
			!strings.Contains(strings.ToLower(topResult), "growth") {
			t.Error("expected top result to contain both query terms")
		}
	}
}

func TestCountPhraseMatches(t *testing.T) {
	tests := []struct {
		name    string
		content string
		tokens  []string
		want    int
	}{
		{
			name:    "simple phrase",
			content: "<p>INCOME STATEMENTS</p>",
			tokens:  []string{"income", "statements"},
			want:    1,
		},
		{
			name:    "with more content",
			content: "<p>INCOME STATEMENTS</p><table><tr><td>Revenue</td></tr></table>",
			tokens:  []string{"income", "statements"},
			want:    1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countPhraseMatches(tt.content, tt.tokens)
			if got != tt.want {
				t.Errorf("countPhraseMatches() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestPhraseMatching(t *testing.T) {
	content := `<p>INCOME STATEMENTS</p>
	<table><tr><td>Revenue</td><td>100</td></tr></table>

	<p>Accounting for income taxes requires...</p>
	<p>The income tax provision includes...</p>
	<p>Deferred income tax assets...</p>
	<p>Income tax accounting...</p>`

	opts := Options{
		Query:      "income statements",
		WindowSize: 500,
		MaxResults: 10,
		Highlight:  false,
	}

	result, err := Search(content, "text/html", opts)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(result.Results) == 0 {
		t.Fatal("expected at least one match")
	}

	// The window with "INCOME STATEMENTS" phrase should rank first
	// Note: highlighting=false but we still need to check for the phrase
	topResult := result.Results[0].Snippet
	if !strings.Contains(strings.ToUpper(topResult), "INCOME") || !strings.Contains(strings.ToUpper(topResult), "STATEMENTS") {
		t.Errorf("expected phrase with 'INCOME' and 'STATEMENTS' to rank first, got: %s", topResult)
	}

	// More specifically, it should be in the first line
	lines := strings.Split(topResult, "\n")
	firstLine := strings.ToUpper(lines[0])
	if !strings.Contains(firstLine, "INCOME") || !strings.Contains(firstLine, "STATEMENTS") {
		t.Errorf("expected 'INCOME STATEMENTS' in first line, got: %s", lines[0])
	}
}
