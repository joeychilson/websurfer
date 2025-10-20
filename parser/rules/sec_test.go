package rules

import (
	"strings"
	"testing"
)

func TestSECRule_Match(t *testing.T) {
	rule := NewSECRule()

	tests := []struct {
		name        string
		url         string
		contentType string
		shouldMatch bool
	}{
		{
			name:        "sec.gov HTML",
			url:         "https://www.sec.gov/Archives/edgar/data/123/file.html",
			contentType: "text/html",
			shouldMatch: true,
		},
		{
			name:        "sec.gov XHTML",
			url:         "https://www.sec.gov/file.xhtml",
			contentType: "application/xhtml+xml",
			shouldMatch: true,
		},
		{
			name:        "sec.gov JSON - should NOT match",
			url:         "https://data.sec.gov/submissions/file.json",
			contentType: "application/json",
			shouldMatch: false,
		},
		{
			name:        "sec.gov PDF - should NOT match",
			url:         "https://www.sec.gov/Archives/file.pdf",
			contentType: "application/pdf",
			shouldMatch: false,
		},
		{
			name:        "non-sec.gov HTML - should NOT match",
			url:         "https://example.com/page.html",
			contentType: "text/html",
			shouldMatch: false,
		},
		{
			name:        "case insensitive URL",
			url:         "https://www.SEC.GOV/files",
			contentType: "text/html",
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := rule.Match(tt.url, tt.contentType)
			if matched != tt.shouldMatch {
				t.Errorf("Match(%q, %q) = %v, want %v", tt.url, tt.contentType, matched, tt.shouldMatch)
			}
		})
	}
}

func TestSECRule_Apply_RemovesXBRLTags(t *testing.T) {
	rule := NewSECRule()

	tests := []struct {
		name        string
		input       string
		contains    []string
		notContains []string
	}{
		{
			name:        "removes inline XBRL tags",
			input:       `<html><body><ix:nonfraction>12345</ix:nonfraction><p>Content</p></body></html>`,
			contains:    []string{"<html>", "<body>", "12345", "<p>Content</p>"},
			notContains: []string{"<ix:nonfraction>", "</ix:nonfraction>"},
		},
		{
			name:        "removes XBRL namespace URLs",
			input:       `<body>http://fasb.org/us-gaap/2024#DerivativeAssets Some text</body>`,
			contains:    []string{"Some text"},
			notContains: []string{"http://fasb.org/us-gaap/2024#DerivativeAssets"},
		},
		{
			name:        "removes XBRL member references",
			input:       `<p>us-gaap:SomethingMember msft:AnotherMember normal text</p>`,
			contains:    []string{"<p>", "normal text", "</p>"},
			notContains: []string{"us-gaap:SomethingMember", "msft:AnotherMember"},
		},
		{
			name:        "removes ISO currency codes",
			input:       `<body>iso4217:USD iso4217:EUR xbrli:pure xbrli:shares text</body>`,
			contains:    []string{"<body>", "text", "</body>"},
			notContains: []string{"iso4217:USD", "iso4217:EUR", "xbrli:pure", "xbrli:shares"},
		},
		{
			name:        "removes XBRL HTTP patterns",
			input:       `http://fasb.org/us-gaap/2024#Revenue regular content`,
			contains:    []string{"regular content"},
			notContains: []string{"http://fasb.org/us-gaap/2024#Revenue"},
		},
		{
			name:        "removes year ranges",
			input:       `2020 2021 2022 2023 2024 2025 normal text`,
			contains:    []string{"normal text"},
			notContains: []string{"2020 2021 2022 2023 2024 2025"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rule.Apply([]byte(tt.input))
			resultStr := string(result)

			for _, want := range tt.contains {
				if !strings.Contains(resultStr, want) {
					t.Errorf("Result should contain %q, got: %q", want, resultStr)
				}
			}

			for _, notWant := range tt.notContains {
				if strings.Contains(resultStr, notWant) {
					t.Errorf("Result should not contain %q, got: %q", notWant, resultStr)
				}
			}
		})
	}
}

func TestSECRule_Apply_RealWorldExample(t *testing.T) {
	rule := NewSECRule()

	input := `<body>FYfalse0000789019P2YP5YP3YP1Yhttp://fasb.org/us-gaap/2024#DerivativeAssetshttp://fasb.org/us-gaap/2024#DerivativeAssetshttp://fasb.org/us-gaap/2024#DerivativeLiabilitiesus-gaap:CommonStockMembermsft:ProductivityAndBusinessProcessesMemberiso4217:EURxbrli:purexbrli:sharesiso4217:USDActual financial content here.</body>`

	result := rule.Apply([]byte(input))
	resultStr := string(result)

	if strings.Contains(resultStr, "http://fasb.org") {
		t.Errorf("Result should not contain XBRL URLs, got: %q", resultStr)
	}

	if strings.Contains(resultStr, "us-gaap:") {
		t.Errorf("Result should not contain XBRL member references, got: %q", resultStr)
	}

	if strings.Contains(resultStr, "iso4217:") {
		t.Errorf("Result should not contain ISO currency codes, got: %q", resultStr)
	}

	if !strings.Contains(resultStr, "financial content here") {
		t.Errorf("Result should contain actual content, got: %q", resultStr)
	}
}

func TestSECTableRule_Match(t *testing.T) {
	rule := NewSECTableRule()

	tests := []struct {
		name        string
		url         string
		contentType string
		shouldMatch bool
	}{
		{
			name:        "sec.gov HTML",
			url:         "https://www.sec.gov/file",
			contentType: "text/html",
			shouldMatch: true,
		},
		{
			name:        "sec.gov JSON - should NOT match",
			url:         "https://data.sec.gov/file.json",
			contentType: "application/json",
			shouldMatch: false,
		},
		{
			name:        "non-sec.gov HTML - should NOT match",
			url:         "https://example.com/file",
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

func TestSECTableRule_Apply(t *testing.T) {
	rule := NewSECTableRule()

	tests := []struct {
		name        string
		input       string
		notContains []string
	}{
		{
			name:        "removes empty table cells",
			input:       `<table><tr><td>&nbsp;</td><td>Data</td></tr></table>`,
			notContains: []string{"<td>&nbsp;</td>"},
		},
		{
			name:        "removes empty table rows",
			input:       `<table><tr><td></td><td></td></tr><tr><td>Data</td></tr></table>`,
			notContains: []string{"<tr><td></td><td></td></tr>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rule.Apply([]byte(tt.input))
			resultStr := string(result)

			for _, notWant := range tt.notContains {
				if strings.Contains(resultStr, notWant) {
					t.Errorf("Result should not contain %q, got: %q", notWant, resultStr)
				}
			}
		})
	}
}
