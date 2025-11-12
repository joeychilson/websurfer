package html

import (
	"context"
	"strings"
	"testing"
)

// BenchmarkHTMLToMarkdown benchmarks HTML to Markdown conversion at various sizes.
func BenchmarkHTMLToMarkdown(b *testing.B) {
	tests := []struct {
		name string
		html string
	}{
		{
			name: "small_simple",
			html: `<html><body><h1>Title</h1><p>Simple paragraph</p></body></html>`,
		},
		{
			name: "medium_article",
			html: generateArticleHTML(1000), // ~1KB
		},
		{
			name: "large_article",
			html: generateArticleHTML(10000), // ~10KB
		},
		{
			name: "huge_article",
			html: generateArticleHTML(100000), // ~100KB
		},
		{
			name: "very_large_article",
			html: generateArticleHTML(1000000), // ~1MB
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			parser := New()
			htmlBytes := []byte(tt.html)
			b.SetBytes(int64(len(htmlBytes)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := parser.Parse(context.Background(), htmlBytes)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkHTMLTable benchmarks table conversion (critical for data extraction).
func BenchmarkHTMLTable(b *testing.B) {
	sizes := []struct {
		name string
		rows int
		cols int
	}{
		{"small_table_5x3", 5, 3},
		{"medium_table_50x5", 50, 5},
		{"large_table_500x10", 500, 10},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			html := generateTableHTML(size.rows, size.cols)
			parser := New()
			htmlBytes := []byte(html)
			b.SetBytes(int64(len(htmlBytes)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := parser.Parse(context.Background(), htmlBytes)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkHTMLList benchmarks list conversion.
func BenchmarkHTMLList(b *testing.B) {
	sizes := []int{10, 100, 1000}

	for _, size := range sizes {
		b.Run(formatItems(size), func(b *testing.B) {
			html := generateListHTML(size)
			parser := New()
			htmlBytes := []byte(html)
			b.SetBytes(int64(len(htmlBytes)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := parser.Parse(context.Background(), htmlBytes)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkHTMLNested benchmarks deeply nested structures.
func BenchmarkHTMLNested(b *testing.B) {
	depths := []int{5, 10, 20}

	for _, depth := range depths {
		b.Run(formatDepth(depth), func(b *testing.B) {
			html := generateNestedHTML(depth)
			parser := New()
			htmlBytes := []byte(html)
			b.SetBytes(int64(len(htmlBytes)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := parser.Parse(context.Background(), htmlBytes)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkHTMLSanitization benchmarks HTML sanitization overhead.
func BenchmarkHTMLSanitization(b *testing.B) {
	html := generateArticleHTML(10000)
	parser := New()
	htmlBytes := []byte(html)
	b.SetBytes(int64(len(htmlBytes)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := parser.Parse(context.Background(), htmlBytes)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkHTMLRealWorld benchmarks realistic webpage conversion.
func BenchmarkHTMLRealWorld(b *testing.B) {
	html := generateRealisticWebpage()
	parser := New()
	htmlBytes := []byte(html)
	b.SetBytes(int64(len(htmlBytes)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := parser.Parse(context.Background(), htmlBytes)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Helper functions to generate test HTML

func generateArticleHTML(approxSize int) string {
	var sb strings.Builder
	sb.WriteString("<html><body>")
	sb.WriteString("<h1>Main Article Title</h1>")

	paragraphSize := 200
	numParagraphs := approxSize / paragraphSize

	for i := 0; i < numParagraphs; i++ {
		sb.WriteString("<h2>Section ")
		sb.WriteString(formatNum(i))
		sb.WriteString("</h2>")
		sb.WriteString("<p>")
		sb.WriteString("This is paragraph ")
		sb.WriteString(formatNum(i))
		sb.WriteString(". ")
		sb.WriteString(strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ", 3))
		sb.WriteString("</p>")
	}

	sb.WriteString("</body></html>")
	return sb.String()
}

func generateTableHTML(rows, cols int) string {
	var sb strings.Builder
	sb.WriteString("<table><tr>")

	for c := 0; c < cols; c++ {
		sb.WriteString("<th>Header ")
		sb.WriteString(formatNum(c))
		sb.WriteString("</th>")
	}
	sb.WriteString("</tr>")

	for r := 0; r < rows; r++ {
		sb.WriteString("<tr>")
		for c := 0; c < cols; c++ {
			sb.WriteString("<td>Cell ")
			sb.WriteString(formatNum(r))
			sb.WriteString(",")
			sb.WriteString(formatNum(c))
			sb.WriteString("</td>")
		}
		sb.WriteString("</tr>")
	}

	sb.WriteString("</table>")
	return sb.String()
}

func generateListHTML(items int) string {
	var sb strings.Builder
	sb.WriteString("<ul>")

	for i := 0; i < items; i++ {
		sb.WriteString("<li>Item ")
		sb.WriteString(formatNum(i))
		sb.WriteString("</li>")
	}

	sb.WriteString("</ul>")
	return sb.String()
}

func generateNestedHTML(depth int) string {
	var sb strings.Builder

	for i := 0; i < depth; i++ {
		sb.WriteString("<div>")
	}

	sb.WriteString("<p>Content at depth ")
	sb.WriteString(formatNum(depth))
	sb.WriteString("</p>")

	for i := 0; i < depth; i++ {
		sb.WriteString("</div>")
	}

	return sb.String()
}

func generateRealisticWebpage() string {
	return `<!DOCTYPE html>
<html>
<head>
    <title>Example Article</title>
    <meta charset="utf-8">
</head>
<body>
    <header>
        <h1>Complete Guide to Web Technologies</h1>
        <nav>
            <a href="/home">Home</a>
            <a href="/docs">Documentation</a>
            <a href="/api">API</a>
        </nav>
    </header>
    <main>
        <article>
            <h2>Introduction</h2>
            <p>This is a comprehensive guide covering modern web development practices and technologies. We'll explore HTML, CSS, JavaScript, and backend frameworks in detail.</p>

            <h2>Key Technologies</h2>
            <ul>
                <li><strong>HTML5</strong> - Markup language</li>
                <li><strong>CSS3</strong> - Styling</li>
                <li><strong>JavaScript</strong> - Programming language</li>
                <li><strong>React</strong> - UI framework</li>
            </ul>

            <h2>Performance Metrics</h2>
            <table>
                <tr>
                    <th>Metric</th>
                    <th>Target</th>
                    <th>Actual</th>
                </tr>
                <tr>
                    <td>First Contentful Paint</td>
                    <td>&lt;1.8s</td>
                    <td>1.2s</td>
                </tr>
                <tr>
                    <td>Time to Interactive</td>
                    <td>&lt;3.8s</td>
                    <td>2.9s</td>
                </tr>
            </table>

            <h3>Code Example</h3>
            <pre><code>function fetchData(url) {
  return fetch(url)
    .then(response => response.json())
    .catch(error => console.error(error));
}</code></pre>

            <h2>Best Practices</h2>
            <ol>
                <li>Minimize HTTP requests</li>
                <li>Use CDN for static assets</li>
                <li>Enable gzip compression</li>
                <li>Optimize images</li>
            </ol>

            <blockquote>
                "Performance is not just about speed, it's about user experience."
            </blockquote>
        </article>
    </main>
    <footer>
        <p>&copy; 2024 Web Technologies Inc.</p>
    </footer>
</body>
</html>`
}

func formatNum(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0' + n/10)) + string(rune('0' + n%10))
}

func formatItems(n int) string {
	if n >= 1000 {
		return "1000_items"
	} else if n >= 100 {
		return "100_items"
	}
	return "10_items"
}

func formatDepth(n int) string {
	if n >= 20 {
		return "depth_20"
	} else if n >= 10 {
		return "depth_10"
	}
	return "depth_5"
}
