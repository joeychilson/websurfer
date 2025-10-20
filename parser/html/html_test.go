package html

import (
	"context"
	"strings"
	"testing"
)

func TestParser_Parse(t *testing.T) {
	parser := New()
	ctx := context.Background()

	t.Run("removes scripts and styles", func(t *testing.T) {
		input := []byte(`
			<html>
				<head>
					<script>alert('hello');</script>
					<style>.foo { color: red; }</style>
				</head>
				<body>
					<p>Content</p>
					<script>console.log('test');</script>
				</body>
			</html>
		`)

		output, err := parser.Parse(ctx, input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		result := string(output)
		if strings.Contains(result, "script") || strings.Contains(result, "style") {
			t.Errorf("output should not contain script or style tags, got: %s", result)
		}
		if !strings.Contains(result, "Content") {
			t.Errorf("output should contain text content, got: %s", result)
		}
	})

	t.Run("removes empty elements", func(t *testing.T) {
		input := []byte(`
			<html>
				<body>
					<p>Content</p>
					<p></p>
					<p>   </p>
					<ul>
						<li>Item</li>
						<li></li>
					</ul>
				</body>
			</html>
		`)

		output, err := parser.Parse(ctx, input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		result := string(output)
		if !strings.Contains(result, "Content") || !strings.Contains(result, "Item") {
			t.Errorf("output should contain non-empty content, got: %s", result)
		}
	})

	t.Run("preserves void elements", func(t *testing.T) {
		input := []byte(`
			<html>
				<body>
					<img src="test.jpg" alt="Test Image">
					<br>
					<hr>
					<p>Text</p>
				</body>
			</html>
		`)

		output, err := parser.Parse(ctx, input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		result := string(output)
		if !strings.Contains(result, "<img") {
			t.Errorf("output should contain img tag, got: %s", result)
		}
		if !strings.Contains(result, "<br") {
			t.Errorf("output should contain br tag, got: %s", result)
		}
		if !strings.Contains(result, "<hr") {
			t.Errorf("output should contain hr tag, got: %s", result)
		}
		if !strings.Contains(result, "src=\"test.jpg\"") {
			t.Errorf("output should preserve src attribute, got: %s", result)
		}
		if !strings.Contains(result, "alt=\"Test Image\"") {
			t.Errorf("output should preserve alt attribute, got: %s", result)
		}
	})

	t.Run("unwraps divs", func(t *testing.T) {
		input := []byte(`
			<html>
				<body>
					<div>
						<p>Paragraph 1</p>
						<div>
							<p>Paragraph 2</p>
						</div>
					</div>
				</body>
			</html>
		`)

		output, err := parser.Parse(ctx, input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		result := string(output)
		if strings.Contains(result, "<div") {
			t.Errorf("output should not contain div tags, got: %s", result)
		}
		if !strings.Contains(result, "Paragraph 1") || !strings.Contains(result, "Paragraph 2") {
			t.Errorf("output should contain unwrapped content, got: %s", result)
		}
	})

	t.Run("normalizes whitespace", func(t *testing.T) {
		input := []byte(`
			<html>
				<body>
					<p>This    has     multiple     spaces</p>
					<p>
						Text with
						newlines
					</p>
				</body>
			</html>
		`)

		output, err := parser.Parse(ctx, input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		result := string(output)
		if strings.Contains(result, "    ") {
			t.Errorf("output should collapse multiple spaces, got: %s", result)
		}
		// Note: output now contains semantic newlines after block elements (</p>, etc.) for LLM-friendly line-based navigation
		// This is intentional behavior, so we don't check for absence of newlines
	})

	t.Run("preserves essential attributes", func(t *testing.T) {
		input := []byte(`
			<html>
				<body>
					<a href="https://example.com" class="link" id="main">Link</a>
					<img src="test.jpg" alt="Test" class="image" id="img1">
					<table>
						<tr>
							<td colspan="2" rowspan="3">Cell</td>
						</tr>
					</table>
				</body>
			</html>
		`)

		output, err := parser.Parse(ctx, input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		result := string(output)
		if !strings.Contains(result, "href=\"https://example.com\"") {
			t.Errorf("output should preserve href attribute, got: %s", result)
		}
		if !strings.Contains(result, "src=\"test.jpg\"") {
			t.Errorf("output should preserve src attribute, got: %s", result)
		}
		if !strings.Contains(result, "alt=\"Test\"") {
			t.Errorf("output should preserve alt attribute, got: %s", result)
		}
		if !strings.Contains(result, "colspan=\"2\"") {
			t.Errorf("output should preserve colspan attribute, got: %s", result)
		}
		if !strings.Contains(result, "rowspan=\"3\"") {
			t.Errorf("output should preserve rowspan attribute, got: %s", result)
		}
		if strings.Contains(result, "class=") || strings.Contains(result, "id=") {
			t.Errorf("output should remove class and id attributes, got: %s", result)
		}
	})

	t.Run("preserves structural elements", func(t *testing.T) {
		input := []byte(`
			<html>
				<body>
					<header><h1>Title</h1></header>
					<nav><a href="/">Home</a></nav>
					<main>
						<h2>Section</h2>
						<ul>
							<li>Item 1</li>
							<li>Item 2</li>
						</ul>
						<table>
							<thead><tr><th>Header</th></tr></thead>
							<tbody><tr><td>Data</td></tr></tbody>
						</table>
					</main>
					<footer><p>Footer</p></footer>
				</body>
			</html>
		`)

		output, err := parser.Parse(ctx, input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		result := string(output)
		requiredElements := []string{
			"<header>", "<h1>", "Title",
			"<nav>", "<a", "Home",
			"<h2>", "Section",
			"<ul>", "<li>", "Item 1", "Item 2",
			"<table>", "<thead>", "<tbody>", "<tr>", "<th>", "<td>",
			"<footer>", "<p>", "Footer",
		}

		for _, elem := range requiredElements {
			if !strings.Contains(result, elem) {
				t.Errorf("output should contain %q, got: %s", elem, result)
			}
		}
	})

	t.Run("produces minified output with semantic line breaks", func(t *testing.T) {
		input := []byte(`
			<html>
				<body>
					<p>Line 1</p>
					<p>Line 2</p>
				</body>
			</html>
		`)

		output, err := parser.Parse(ctx, input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		result := string(output)
		// Output now contains newlines after block elements for LLM-friendly line-based navigation
		lines := strings.Split(result, "\n")
		if len(lines) < 2 {
			t.Errorf("output should have multiple lines separated by block elements, got: %s", result)
		}
		// Should still remove whitespace between tags on the same line
		for _, line := range lines {
			if strings.Contains(line, "> <") {
				t.Errorf("output should not have whitespace between tags on same line, got: %s", line)
			}
		}
	})

	t.Run("handles empty content", func(t *testing.T) {
		input := []byte("")
		output, err := parser.Parse(ctx, input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if len(output) != 0 {
			t.Errorf("output should be empty for empty input, got: %s", string(output))
		}
	})

	t.Run("real-world example with complex structure", func(t *testing.T) {
		input := []byte(`
			<!DOCTYPE html>
			<html lang="en">
				<head>
					<meta charset="UTF-8">
					<title>Sports News</title>
					<script src="analytics.js"></script>
					<style>
						.container { max-width: 1200px; }
						.header { background: blue; }
					</style>
					<link rel="stylesheet" href="styles.css">
				</head>
				<body class="sports-page" id="main-body">
					<div class="wrapper">
						<header class="site-header">
							<nav class="main-nav">
								<a href="/" class="nav-link">Home</a>
								<a href="/scores" class="nav-link">Scores</a>
							</nav>
						</header>
						<div class="content">
							<main class="main-content">
								<h1 class="page-title">Latest Scores</h1>
								<div class="empty-div"></div>
								<table class="scores-table">
									<thead>
										<tr>
											<th>Team</th>
											<th>Score</th>
										</tr>
									</thead>
									<tbody>
										<tr>
											<td>Lakers</td>
											<td>105</td>
										</tr>
										<tr>
											<td>Warriors</td>
											<td>98</td>
										</tr>
									</tbody>
								</table>
								<ul class="news-list">
									<li><a href="/article1">Article 1</a></li>
									<li><a href="/article2">Article 2</a></li>
								</ul>
								<p class="empty-paragraph">   </p>
								<img src="player.jpg" alt="Star Player" class="player-image">
							</main>
						</div>
						<footer class="site-footer">
							<p>&copy; 2024 Sports News</p>
						</footer>
					</div>
					<script>
						console.log('Page loaded');
					</script>
				</body>
			</html>
		`)

		output, err := parser.Parse(ctx, input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		result := string(output)

		if strings.Contains(result, "<script") || strings.Contains(result, "<style") || strings.Contains(result, "<link") {
			t.Errorf("output should not contain script/style/link tags")
		}

		if strings.Contains(result, "class=") || strings.Contains(result, "id=") {
			t.Errorf("output should not contain class or id attributes")
		}

		if strings.Contains(result, "<div") {
			t.Errorf("output should not contain div tags")
		}

		requiredElements := []string{
			"<header>", "<nav>", "<footer>",
			"<h1>", "<table>", "<thead>", "<tbody>", "<tr>", "<th>", "<td>",
			"<ul>", "<li>", "<a", "<img", "<p>",
		}
		for _, elem := range requiredElements {
			if !strings.Contains(result, elem) {
				t.Errorf("output should contain %q", elem)
			}
		}

		requiredContent := []string{
			"Home", "Scores", "Latest Scores",
			"Team", "Score", "Lakers", "105", "Warriors", "98",
			"Article 1", "Article 2", "2024 Sports News",
		}
		for _, content := range requiredContent {
			if !strings.Contains(result, content) {
				t.Errorf("output should contain %q", content)
			}
		}

		if !strings.Contains(result, `href="/"`) {
			t.Errorf("output should preserve href attributes")
		}
		if !strings.Contains(result, `href="/scores"`) {
			t.Errorf("output should preserve href attributes")
		}
		if !strings.Contains(result, `src="player.jpg"`) {
			t.Errorf("output should preserve src attribute")
		}
		if !strings.Contains(result, `alt="Star Player"`) {
			t.Errorf("output should preserve alt attribute")
		}

		// Output now contains semantic newlines after block elements for LLM-friendly navigation
		// Check that whitespace between tags on the same line is still removed
		lines := strings.Split(result, "\n")
		for _, line := range lines {
			if strings.Contains(line, "> <") {
				t.Errorf("output should not have whitespace between tags on same line")
			}
		}

		if strings.Contains(result, "empty-div") {
			t.Errorf("output should remove empty divs")
		}
	})

	t.Run("preserves table structure with attributes", func(t *testing.T) {
		input := []byte(`
			<html>
				<body>
					<table>
						<tr>
							<td colspan="2">Header Cell</td>
						</tr>
						<tr>
							<td>Cell 1</td>
							<td rowspan="2">Tall Cell</td>
						</tr>
						<tr>
							<td>Cell 2</td>
						</tr>
					</table>
				</body>
			</html>
		`)

		output, err := parser.Parse(ctx, input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		result := string(output)
		if !strings.Contains(result, "colspan=\"2\"") {
			t.Errorf("output should preserve colspan attribute")
		}
		if !strings.Contains(result, "rowspan=\"2\"") {
			t.Errorf("output should preserve rowspan attribute")
		}
		if !strings.Contains(result, "Header Cell") {
			t.Errorf("output should preserve table content")
		}
	})

	t.Run("handles nested lists", func(t *testing.T) {
		input := []byte(`
			<html>
				<body>
					<ul>
						<li>Item 1</li>
						<li>
							Item 2
							<ul>
								<li>Nested 1</li>
								<li>Nested 2</li>
							</ul>
						</li>
						<li>Item 3</li>
					</ul>
				</body>
			</html>
		`)

		output, err := parser.Parse(ctx, input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		result := string(output)
		requiredContent := []string{"Item 1", "Item 2", "Nested 1", "Nested 2", "Item 3"}
		for _, content := range requiredContent {
			if !strings.Contains(result, content) {
				t.Errorf("output should contain %q", content)
			}
		}
	})

	t.Run("handles links with various attributes", func(t *testing.T) {
		input := []byte(`
			<html>
				<body>
					<a href="https://example.com" target="_blank" rel="noopener" class="external-link">External</a>
					<a href="/internal" data-id="123">Internal</a>
				</body>
			</html>
		`)

		output, err := parser.Parse(ctx, input)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}

		result := string(output)
		if !strings.Contains(result, `href="https://example.com"`) {
			t.Errorf("output should preserve href attribute")
		}
		if !strings.Contains(result, `href="/internal"`) {
			t.Errorf("output should preserve href attribute")
		}
		if strings.Contains(result, "target=") || strings.Contains(result, "rel=") ||
			strings.Contains(result, "class=") || strings.Contains(result, "data-") {
			t.Errorf("output should remove non-essential attributes")
		}
		if !strings.Contains(result, "External") || !strings.Contains(result, "Internal") {
			t.Errorf("output should preserve link text")
		}
	})
}

func TestNew(t *testing.T) {
	parser := New()
	if parser == nil {
		t.Fatal("New() should return non-nil parser")
	}
	if parser.policy == nil {
		t.Fatal("New() should initialize policy")
	}
}
