package html

import (
	"reflect"
	"testing"
)

func TestExtractLinks(t *testing.T) {
	t.Run("basic link extraction", func(t *testing.T) {
		html := `<html>
			<body>
				<a href="https://example.com/page1">Link 1</a>
				<a href="https://example.com/page2">Link 2</a>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		if len(links) != 2 {
			t.Fatalf("ExtractLinks() returned %d links, want 2", len(links))
		}
		expected := []string{
			"https://example.com/page1",
			"https://example.com/page2",
		}
		if !reflect.DeepEqual(links, expected) {
			t.Errorf("ExtractLinks() = %v, want %v", links, expected)
		}
	})

	t.Run("relative URLs resolved to absolute", func(t *testing.T) {
		html := `<html>
			<body>
				<a href="/about">About</a>
				<a href="/contact">Contact</a>
				<a href="products/item1">Product 1</a>
				<a href="../parent">Parent</a>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com/pages/info")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		if len(links) != 4 {
			t.Fatalf("ExtractLinks() returned %d links, want 4", len(links))
		}
		expected := []string{
			"https://example.com/about",
			"https://example.com/contact",
			"https://example.com/pages/products/item1",
			"https://example.com/parent",
		}
		if !reflect.DeepEqual(links, expected) {
			t.Errorf("ExtractLinks() = %v, want %v", links, expected)
		}
	})

	t.Run("filters anchor links", func(t *testing.T) {
		html := `<html>
			<body>
				<a href="#section1">Section 1</a>
				<a href="#top">Top</a>
				<a href="https://example.com/page">Valid Link</a>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		if len(links) != 1 {
			t.Fatalf("ExtractLinks() returned %d links, want 1 (anchor links should be filtered)", len(links))
		}
		if links[0] != "https://example.com/page" {
			t.Errorf("ExtractLinks() = %v, want [https://example.com/page]", links)
		}
	})

	t.Run("filters javascript links", func(t *testing.T) {
		html := `<html>
			<body>
				<a href="javascript:void(0)">Click</a>
				<a href="javascript:alert('test')">Alert</a>
				<a href="https://example.com/page">Valid Link</a>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		if len(links) != 1 {
			t.Fatalf("ExtractLinks() returned %d links, want 1 (javascript links should be filtered)", len(links))
		}
		if links[0] != "https://example.com/page" {
			t.Errorf("ExtractLinks() = %v, want [https://example.com/page]", links)
		}
	})

	t.Run("filters mailto and tel links", func(t *testing.T) {
		html := `<html>
			<body>
				<a href="mailto:test@example.com">Email</a>
				<a href="tel:+1234567890">Phone</a>
				<a href="https://example.com/page">Valid Link</a>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		if len(links) != 1 {
			t.Fatalf("ExtractLinks() returned %d links, want 1 (mailto/tel links should be filtered)", len(links))
		}
		if links[0] != "https://example.com/page" {
			t.Errorf("ExtractLinks() = %v, want [https://example.com/page]", links)
		}
	})

	t.Run("removes URL fragments", func(t *testing.T) {
		html := `<html>
			<body>
				<a href="https://example.com/page1#section1">Link 1</a>
				<a href="https://example.com/page2#section2">Link 2</a>
				<a href="https://example.com/page1#different">Link 3</a>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		// Should deduplicate to 2 unique URLs (fragments removed)
		if len(links) != 2 {
			t.Fatalf("ExtractLinks() returned %d links, want 2 (fragments removed and deduplicated)", len(links))
		}
		expected := []string{
			"https://example.com/page1",
			"https://example.com/page2",
		}
		if !reflect.DeepEqual(links, expected) {
			t.Errorf("ExtractLinks() = %v, want %v", links, expected)
		}
	})

	t.Run("deduplicates URLs", func(t *testing.T) {
		html := `<html>
			<body>
				<a href="https://example.com/page1">Link 1</a>
				<a href="https://example.com/page1">Link 1 Again</a>
				<a href="https://example.com/page2">Link 2</a>
				<a href="https://example.com/page1">Link 1 Third Time</a>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		if len(links) != 2 {
			t.Fatalf("ExtractLinks() returned %d links, want 2 (deduplicated)", len(links))
		}
		expected := []string{
			"https://example.com/page1",
			"https://example.com/page2",
		}
		if !reflect.DeepEqual(links, expected) {
			t.Errorf("ExtractLinks() = %v, want %v", links, expected)
		}
	})

	t.Run("handles empty href", func(t *testing.T) {
		html := `<html>
			<body>
				<a href="">Empty</a>
				<a href="https://example.com/page">Valid Link</a>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		// Empty href attributes are filtered out by the attr.Val != "" check
		if len(links) != 1 {
			t.Fatalf("ExtractLinks() returned %d links, want 1", len(links))
		}
		if links[0] != "https://example.com/page" {
			t.Errorf("ExtractLinks() = %v, want [https://example.com/page]", links)
		}
	})

	t.Run("whitespace-only href resolves to base URL", func(t *testing.T) {
		html := `<html>
			<body>
				<a href="   ">Whitespace</a>
				<a href="https://example.com/page">Valid Link</a>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		// Whitespace gets trimmed to empty string, which resolves to base URL
		if len(links) != 2 {
			t.Fatalf("ExtractLinks() returned %d links, want 2", len(links))
		}
		expected := []string{
			"https://example.com",
			"https://example.com/page",
		}
		if !reflect.DeepEqual(links, expected) {
			t.Errorf("ExtractLinks() = %v, want %v", links, expected)
		}
	})

	t.Run("extracts links from nested elements", func(t *testing.T) {
		html := `<html>
			<body>
				<nav>
					<ul>
						<li><a href="/home">Home</a></li>
						<li><a href="/about">About</a></li>
					</ul>
				</nav>
				<main>
					<article>
						<p>Text with <a href="/link1">link</a></p>
					</article>
				</main>
				<footer>
					<a href="/contact">Contact</a>
				</footer>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		if len(links) != 4 {
			t.Fatalf("ExtractLinks() returned %d links, want 4", len(links))
		}
		expected := []string{
			"https://example.com/home",
			"https://example.com/about",
			"https://example.com/link1",
			"https://example.com/contact",
		}
		if !reflect.DeepEqual(links, expected) {
			t.Errorf("ExtractLinks() = %v, want %v", links, expected)
		}
	})

	t.Run("preserves query parameters", func(t *testing.T) {
		html := `<html>
			<body>
				<a href="https://example.com/search?q=test&page=1">Search</a>
				<a href="/page?id=123">Page</a>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		if len(links) != 2 {
			t.Fatalf("ExtractLinks() returned %d links, want 2", len(links))
		}
		expected := []string{
			"https://example.com/search?q=test&page=1",
			"https://example.com/page?id=123",
		}
		if !reflect.DeepEqual(links, expected) {
			t.Errorf("ExtractLinks() = %v, want %v", links, expected)
		}
	})

	t.Run("handles external domains", func(t *testing.T) {
		html := `<html>
			<body>
				<a href="https://example.com/internal">Internal</a>
				<a href="https://external.com/page">External</a>
				<a href="https://another.org/resource">Another</a>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		if len(links) != 3 {
			t.Fatalf("ExtractLinks() returned %d links, want 3", len(links))
		}
		expected := []string{
			"https://example.com/internal",
			"https://external.com/page",
			"https://another.org/resource",
		}
		if !reflect.DeepEqual(links, expected) {
			t.Errorf("ExtractLinks() = %v, want %v", links, expected)
		}
	})

	t.Run("handles protocol-relative URLs", func(t *testing.T) {
		html := `<html>
			<body>
				<a href="//cdn.example.com/resource">CDN Resource</a>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		if len(links) != 1 {
			t.Fatalf("ExtractLinks() returned %d links, want 1", len(links))
		}
		if links[0] != "https://cdn.example.com/resource" {
			t.Errorf("ExtractLinks() = %v, want [https://cdn.example.com/resource]", links)
		}
	})

	t.Run("no links in HTML", func(t *testing.T) {
		html := `<html>
			<body>
				<p>This page has no links</p>
				<div>Just some content</div>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		if len(links) != 0 {
			t.Errorf("ExtractLinks() returned %d links, want 0", len(links))
		}
	})

	t.Run("empty HTML", func(t *testing.T) {
		html := ``

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		if len(links) != 0 {
			t.Errorf("ExtractLinks() returned %d links, want 0", len(links))
		}
	})

	t.Run("handles base URL parsing", func(t *testing.T) {
		html := `<html><body><a href="/page">Link</a></body></html>`

		// Go's url.Parse is quite permissive - "not a valid url" is actually parsed successfully
		// It's just treated as an opaque string. Let's test with a valid base URL.
		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		if len(links) != 1 {
			t.Fatalf("ExtractLinks() returned %d links, want 1", len(links))
		}
		if links[0] != "https://example.com/page" {
			t.Errorf("ExtractLinks() = %v, want [https://example.com/page]", links)
		}
	})

	t.Run("malformed HTML still extracts links", func(t *testing.T) {
		html := `<html>
			<body>
				<a href="/page1">Link 1
				<p>Some text</p>
				<a href="/page2">Link 2</a>
			</body>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		if len(links) != 2 {
			t.Fatalf("ExtractLinks() returned %d links, want 2", len(links))
		}
		expected := []string{
			"https://example.com/page1",
			"https://example.com/page2",
		}
		if !reflect.DeepEqual(links, expected) {
			t.Errorf("ExtractLinks() = %v, want %v", links, expected)
		}
	})

	t.Run("complex real-world example", func(t *testing.T) {
		html := `<!DOCTYPE html>
		<html lang="en">
			<head>
				<title>Example Site</title>
			</head>
			<body>
				<header>
					<nav>
						<a href="/">Home</a>
						<a href="/about">About</a>
						<a href="/products">Products</a>
						<a href="https://blog.example.com">Blog</a>
					</nav>
				</header>
				<main>
					<article>
						<h1>Welcome</h1>
						<p>Check out our <a href="/special-offer?promo=summer#details">special offer</a></p>
						<p>Contact us at <a href="mailto:info@example.com">info@example.com</a></p>
						<p>Or call <a href="tel:+1234567890">+1234567890</a></p>
					</article>
					<aside>
						<a href="/special-offer?promo=summer#more">Same offer</a>
						<a href="javascript:openModal()">Open Modal</a>
						<a href="#top">Back to Top</a>
					</aside>
				</main>
				<footer>
					<a href="/privacy">Privacy</a>
					<a href="/terms">Terms</a>
					<a href="https://twitter.com/example">Twitter</a>
				</footer>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}

		expected := []string{
			"https://example.com/",
			"https://example.com/about",
			"https://example.com/products",
			"https://blog.example.com",
			"https://example.com/special-offer?promo=summer", // Fragment removed and deduplicated
			"https://example.com/privacy",
			"https://example.com/terms",
			"https://twitter.com/example",
		}

		if !reflect.DeepEqual(links, expected) {
			t.Errorf("ExtractLinks() = %v, want %v", links, expected)
		}
	})

	t.Run("case sensitivity in URLs", func(t *testing.T) {
		html := `<html>
			<body>
				<a href="https://example.com/Page">Page 1</a>
				<a href="https://example.com/page">Page 2</a>
				<a href="https://EXAMPLE.com/page">Page 3</a>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		// URLs are case-sensitive in the path but not in the domain
		// All three should be preserved as different URLs due to case
		if len(links) != 3 {
			t.Fatalf("ExtractLinks() returned %d links, want 3", len(links))
		}
	})

	t.Run("trailing slash normalization", func(t *testing.T) {
		html := `<html>
			<body>
				<a href="https://example.com/page/">With slash</a>
				<a href="https://example.com/page">Without slash</a>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		// These are treated as different URLs (no normalization of trailing slash)
		if len(links) != 2 {
			t.Fatalf("ExtractLinks() returned %d links, want 2", len(links))
		}
	})

	t.Run("preserves order of first occurrence", func(t *testing.T) {
		html := `<html>
			<body>
				<a href="/page3">Third</a>
				<a href="/page1">First</a>
				<a href="/page2">Second</a>
				<a href="/page1">First duplicate</a>
			</body>
		</html>`

		links, err := ExtractLinks(html, "https://example.com")
		if err != nil {
			t.Fatalf("ExtractLinks() error = %v", err)
		}
		if len(links) != 3 {
			t.Fatalf("ExtractLinks() returned %d links, want 3", len(links))
		}
		// Order should match first occurrence
		expected := []string{
			"https://example.com/page3",
			"https://example.com/page1",
			"https://example.com/page2",
		}
		if !reflect.DeepEqual(links, expected) {
			t.Errorf("ExtractLinks() = %v, want %v", links, expected)
		}
	})
}
