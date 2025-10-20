package rules

import (
	"testing"
)

func TestDomainRule(t *testing.T) {
	callCount := 0
	rule := NewDomainRule("example.com", "text/html", "test", func(b []byte) []byte {
		callCount++
		return []byte("transformed")
	})

	tests := []struct {
		name          string
		url           string
		contentType   string
		shouldMatch   bool
		expectedCalls int
	}{
		{
			name:          "domain and content-type match",
			url:           "https://example.com/path",
			contentType:   "text/html",
			shouldMatch:   true,
			expectedCalls: 1,
		},
		{
			name:          "domain matches, wrong content-type",
			url:           "https://example.com/path",
			contentType:   "application/json",
			shouldMatch:   false,
			expectedCalls: 1,
		},
		{
			name:          "subdomain match with correct content-type",
			url:           "https://www.example.com/path",
			contentType:   "text/html",
			shouldMatch:   true,
			expectedCalls: 2,
		},
		{
			name:          "different domain",
			url:           "https://other.com/path",
			contentType:   "text/html",
			shouldMatch:   false,
			expectedCalls: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := rule.Match(tt.url, tt.contentType)
			if matched != tt.shouldMatch {
				t.Errorf("Match() = %v, want %v", matched, tt.shouldMatch)
			}

			if matched {
				rule.Apply([]byte("test"))
			}

			if callCount != tt.expectedCalls {
				t.Errorf("Apply() call count = %d, want %d", callCount, tt.expectedCalls)
			}
		})
	}
}

func TestDomainRule_AllContentTypes(t *testing.T) {
	rule := NewDomainRule("example.com", "", "test", func(b []byte) []byte {
		return []byte("transformed")
	})

	tests := []struct {
		name        string
		contentType string
		shouldMatch bool
	}{
		{
			name:        "HTML",
			contentType: "text/html",
			shouldMatch: true,
		},
		{
			name:        "JSON",
			contentType: "application/json",
			shouldMatch: true,
		},
		{
			name:        "PDF",
			contentType: "application/pdf",
			shouldMatch: true,
		},
	}

	url := "https://example.com/file"
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := rule.Match(url, tt.contentType)
			if matched != tt.shouldMatch {
				t.Errorf("Match() = %v, want %v for content-type %s", matched, tt.shouldMatch, tt.contentType)
			}
		})
	}
}

func TestPathRule(t *testing.T) {
	rule, err := NewPathRule(`/financial/.*`, "text/html", "test", func(b []byte) []byte {
		return []byte("transformed")
	})
	if err != nil {
		t.Fatalf("Failed to create path rule: %v", err)
	}

	tests := []struct {
		name        string
		url         string
		contentType string
		shouldMatch bool
	}{
		{
			name:        "path and content-type match",
			url:         "https://example.com/financial/report.html",
			contentType: "text/html",
			shouldMatch: true,
		},
		{
			name:        "path matches, wrong content-type",
			url:         "https://example.com/financial/data.json",
			contentType: "application/json",
			shouldMatch: false,
		},
		{
			name:        "non-matching path",
			url:         "https://example.com/about",
			contentType: "text/html",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := rule.Match(tt.url, tt.contentType)
			if matched != tt.shouldMatch {
				t.Errorf("Match() = %v, want %v", matched, tt.shouldMatch)
			}
		})
	}
}

func TestCompositeRule(t *testing.T) {
	rule, err := NewCompositeRule("sec.gov", `/Archives/.*`, "text/html", "test", func(b []byte) []byte {
		return []byte("transformed")
	})
	if err != nil {
		t.Fatalf("Failed to create composite rule: %v", err)
	}

	tests := []struct {
		name        string
		url         string
		contentType string
		shouldMatch bool
	}{
		{
			name:        "all match",
			url:         "https://www.sec.gov/Archives/edgar/data/123/file.html",
			contentType: "text/html",
			shouldMatch: true,
		},
		{
			name:        "domain and path match, wrong content-type",
			url:         "https://www.sec.gov/Archives/edgar/data/123/file.json",
			contentType: "application/json",
			shouldMatch: false,
		},
		{
			name:        "domain matches, path doesn't",
			url:         "https://www.sec.gov/about",
			contentType: "text/html",
			shouldMatch: false,
		},
		{
			name:        "path matches, domain doesn't",
			url:         "https://example.com/Archives/file.html",
			contentType: "text/html",
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := rule.Match(tt.url, tt.contentType)
			if matched != tt.shouldMatch {
				t.Errorf("Match() = %v, want %v", matched, tt.shouldMatch)
			}
		})
	}
}

func TestRuleChain(t *testing.T) {
	rule1 := NewDomainRule("example.com", "text/html", "rule1", func(b []byte) []byte {
		return append(b, []byte(" rule1")...)
	})

	rule2 := NewDomainRule("example.com", "text/html", "rule2", func(b []byte) []byte {
		return append(b, []byte(" rule2")...)
	})

	rule3 := NewDomainRule("other.com", "text/html", "rule3", func(b []byte) []byte {
		return append(b, []byte(" rule3")...)
	})

	chain := NewRuleChain(rule1, rule2, rule3)

	t.Run("applies matching rules in order", func(t *testing.T) {
		result := chain.Apply("https://example.com/path", "text/html", []byte("content"))
		expected := "content rule1 rule2"
		if string(result) != expected {
			t.Errorf("Apply() = %q, want %q", string(result), expected)
		}
	})

	t.Run("only applies matching rules", func(t *testing.T) {
		result := chain.Apply("https://other.com/path", "text/html", []byte("content"))
		expected := "content rule3"
		if string(result) != expected {
			t.Errorf("Apply() = %q, want %q", string(result), expected)
		}
	})

	t.Run("returns original if no rules match", func(t *testing.T) {
		result := chain.Apply("https://nomatch.com/path", "text/html", []byte("content"))
		expected := "content"
		if string(result) != expected {
			t.Errorf("Apply() = %q, want %q", string(result), expected)
		}
	})

	t.Run("respects content-type filtering", func(t *testing.T) {
		result := chain.Apply("https://example.com/path", "application/json", []byte("content"))
		expected := "content"
		if string(result) != expected {
			t.Errorf("Apply() = %q, want %q (should not match JSON)", string(result), expected)
		}
	})
}
