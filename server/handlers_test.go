package server

import "testing"

func TestExtractLanguage(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "simple en",
			html:     `<html lang="en"><body>test</body></html>`,
			expected: "en",
		},
		{
			name:     "en-US with hyphen",
			html:     `<html lang="en-US"><body>test</body></html>`,
			expected: "en",
		},
		{
			name:     "single quotes",
			html:     `<html lang='fr'><body>test</body></html>`,
			expected: "fr",
		},
		{
			name:     "with attributes before",
			html:     `<html class="no-js" lang="de"><body>test</body></html>`,
			expected: "de",
		},
		{
			name:     "with attributes after",
			html:     `<html lang="es" dir="ltr"><body>test</body></html>`,
			expected: "es",
		},
		{
			name:     "uppercase HTML tag",
			html:     `<HTML lang="ja"><body>test</body></html>`,
			expected: "ja",
		},
		{
			name:     "mixed case lang attribute",
			html:     `<html LANG="zh"><body>test</body></html>`,
			expected: "zh",
		},
		{
			name:     "no lang attribute",
			html:     `<html><body>test</body></html>`,
			expected: "",
		},
		{
			name:     "empty lang",
			html:     `<html lang=""><body>test</body></html>`,
			expected: "",
		},
		{
			name:     "whitespace in lang",
			html:     `<html lang=" en-GB "><body>test</body></html>`,
			expected: "en",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractLanguage(tt.html)
			if result != tt.expected {
				t.Errorf("extractLanguage(%q) = %q, want %q", tt.html, result, tt.expected)
			}
		})
	}
}

func BenchmarkExtractLanguage(b *testing.B) {
	html := `<!DOCTYPE html>
<html lang="en-US" class="no-js">
<head>
    <meta charset="UTF-8">
    <title>Test Page</title>
</head>
<body>
    <h1>Hello World</h1>
</body>
</html>`

	for b.Loop() {
		extractLanguage(html)
	}
}
