package sitemap

import (
	"strings"
	"testing"
)

func TestParse_URLSet(t *testing.T) {
	t.Run("basic urlset", func(t *testing.T) {
		content := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/page1</loc>
    <lastmod>2023-01-01</lastmod>
    <changefreq>daily</changefreq>
    <priority>0.8</priority>
  </url>
  <url>
    <loc>https://example.com/page2</loc>
  </url>
</urlset>`

		result, err := Parse([]byte(content))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if result.IsSitemapIndex {
			t.Error("Parse() IsSitemapIndex = true, want false")
		}
		if len(result.URLs) != 2 {
			t.Fatalf("Parse() returned %d URLs, want 2", len(result.URLs))
		}
		if result.URLs[0] != "https://example.com/page1" {
			t.Errorf("URLs[0] = %v, want https://example.com/page1", result.URLs[0])
		}
		if result.URLs[1] != "https://example.com/page2" {
			t.Errorf("URLs[1] = %v, want https://example.com/page2", result.URLs[1])
		}
		if len(result.ChildMaps) != 0 {
			t.Errorf("ChildMaps = %v, want empty", result.ChildMaps)
		}
	})

	t.Run("urlset without namespace", func(t *testing.T) {
		content := `<?xml version="1.0" encoding="UTF-8"?>
<urlset>
  <url>
    <loc>https://example.com/page1</loc>
  </url>
</urlset>`

		result, err := Parse([]byte(content))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if len(result.URLs) != 1 {
			t.Fatalf("Parse() returned %d URLs, want 1", len(result.URLs))
		}
		if result.URLs[0] != "https://example.com/page1" {
			t.Errorf("URLs[0] = %v, want https://example.com/page1", result.URLs[0])
		}
	})

	t.Run("urlset with empty loc values", func(t *testing.T) {
		content := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/page1</loc>
  </url>
  <url>
    <loc></loc>
  </url>
  <url>
    <loc>https://example.com/page2</loc>
  </url>
</urlset>`

		result, err := Parse([]byte(content))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if len(result.URLs) != 2 {
			t.Fatalf("Parse() returned %d URLs, want 2 (empty loc should be filtered)", len(result.URLs))
		}
		if result.URLs[0] != "https://example.com/page1" {
			t.Errorf("URLs[0] = %v, want https://example.com/page1", result.URLs[0])
		}
		if result.URLs[1] != "https://example.com/page2" {
			t.Errorf("URLs[1] = %v, want https://example.com/page2", result.URLs[1])
		}
	})

	t.Run("empty urlset", func(t *testing.T) {
		content := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
</urlset>`

		result, err := Parse([]byte(content))
		if err == nil {
			t.Error("Parse() should return error for empty urlset")
		}
		if result != nil {
			t.Errorf("Parse() result = %v, want nil for empty urlset", result)
		}
	})
}

func TestParse_SitemapIndex(t *testing.T) {
	t.Run("basic sitemap index", func(t *testing.T) {
		content := `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap>
    <loc>https://example.com/sitemap1.xml</loc>
    <lastmod>2023-01-01</lastmod>
  </sitemap>
  <sitemap>
    <loc>https://example.com/sitemap2.xml</loc>
  </sitemap>
</sitemapindex>`

		result, err := Parse([]byte(content))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if !result.IsSitemapIndex {
			t.Error("Parse() IsSitemapIndex = false, want true")
		}
		if len(result.ChildMaps) != 2 {
			t.Fatalf("Parse() returned %d child sitemaps, want 2", len(result.ChildMaps))
		}
		if result.ChildMaps[0] != "https://example.com/sitemap1.xml" {
			t.Errorf("ChildMaps[0] = %v, want https://example.com/sitemap1.xml", result.ChildMaps[0])
		}
		if result.ChildMaps[1] != "https://example.com/sitemap2.xml" {
			t.Errorf("ChildMaps[1] = %v, want https://example.com/sitemap2.xml", result.ChildMaps[1])
		}
		if len(result.URLs) != 0 {
			t.Errorf("URLs = %v, want empty", result.URLs)
		}
	})

	t.Run("sitemap index without namespace", func(t *testing.T) {
		content := `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex>
  <sitemap>
    <loc>https://example.com/sitemap1.xml</loc>
  </sitemap>
</sitemapindex>`

		result, err := Parse([]byte(content))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if !result.IsSitemapIndex {
			t.Error("Parse() IsSitemapIndex = false, want true")
		}
		if len(result.ChildMaps) != 1 {
			t.Fatalf("Parse() returned %d child sitemaps, want 1", len(result.ChildMaps))
		}
	})

	t.Run("sitemap index with empty loc values", func(t *testing.T) {
		content := `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap>
    <loc>https://example.com/sitemap1.xml</loc>
  </sitemap>
  <sitemap>
    <loc></loc>
  </sitemap>
  <sitemap>
    <loc>https://example.com/sitemap2.xml</loc>
  </sitemap>
</sitemapindex>`

		result, err := Parse([]byte(content))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if len(result.ChildMaps) != 2 {
			t.Fatalf("Parse() returned %d child sitemaps, want 2 (empty loc should be filtered)", len(result.ChildMaps))
		}
		if result.ChildMaps[0] != "https://example.com/sitemap1.xml" {
			t.Errorf("ChildMaps[0] = %v, want https://example.com/sitemap1.xml", result.ChildMaps[0])
		}
		if result.ChildMaps[1] != "https://example.com/sitemap2.xml" {
			t.Errorf("ChildMaps[1] = %v, want https://example.com/sitemap2.xml", result.ChildMaps[1])
		}
	})

	t.Run("empty sitemap index", func(t *testing.T) {
		content := `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
</sitemapindex>`

		result, err := Parse([]byte(content))
		if err == nil {
			t.Error("Parse() should return error for empty sitemap index")
		}
		if result != nil {
			t.Errorf("Parse() result = %v, want nil for empty sitemap index", result)
		}
	})
}

func TestParse_InvalidXML(t *testing.T) {
	t.Run("invalid xml", func(t *testing.T) {
		content := `this is not xml`

		result, err := Parse([]byte(content))
		if err == nil {
			t.Error("Parse() should return error for invalid XML")
		}
		if result != nil {
			t.Errorf("Parse() result = %v, want nil for invalid XML", result)
		}
	})

	t.Run("malformed xml", func(t *testing.T) {
		content := `<?xml version="1.0" encoding="UTF-8"?>
<urlset>
  <url>
    <loc>https://example.com/page1</loc>
  </url>
</urlset`

		result, err := Parse([]byte(content))
		if err == nil {
			t.Error("Parse() should return error for malformed XML")
		}
		if result != nil {
			t.Errorf("Parse() result = %v, want nil for malformed XML", result)
		}
	})

	t.Run("wrong root element", func(t *testing.T) {
		content := `<?xml version="1.0" encoding="UTF-8"?>
<rss>
  <channel>
    <title>Test</title>
  </channel>
</rss>`

		result, err := Parse([]byte(content))
		if err == nil {
			t.Error("Parse() should return error for wrong root element")
		}
		if result != nil {
			t.Errorf("Parse() result = %v, want nil for wrong root element", result)
		}
	})
}

func TestParseReader(t *testing.T) {
	t.Run("parse from reader", func(t *testing.T) {
		content := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>https://example.com/page1</loc>
  </url>
</urlset>`

		reader := strings.NewReader(content)
		result, err := ParseReader(reader)
		if err != nil {
			t.Fatalf("ParseReader() error = %v", err)
		}
		if len(result.URLs) != 1 {
			t.Fatalf("ParseReader() returned %d URLs, want 1", len(result.URLs))
		}
		if result.URLs[0] != "https://example.com/page1" {
			t.Errorf("URLs[0] = %v, want https://example.com/page1", result.URLs[0])
		}
	})
}

func TestIsSitemapURL(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"https://example.com/sitemap.xml", true},
		{"https://example.com/sitemap", true},
		{"https://example.com/SITEMAP.XML", true},
		{"https://example.com/news-sitemap.xml", true},
		{"https://example.com/sitemap_index.xml", true},
		{"https://example.com/Sitemap", true},
		{"https://example.com/page.html", false},
		{"https://example.com/about", false},
		{"https://example.com/", false},
		{"https://example.com/site-map.xml", false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result := IsSitemapURL(tt.url)
			if result != tt.expected {
				t.Errorf("IsSitemapURL(%q) = %v, want %v", tt.url, result, tt.expected)
			}
		})
	}
}

func TestNormalizeSitemapURL(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://example.com/sitemap.xml", "https://example.com/sitemap.xml"},
		{"https://example.com/sitemap", "https://example.com/sitemap.xml"},
		{"https://example.com/SITEMAP.XML", "https://example.com/SITEMAP.XML"},
		{"https://example.com/news-sitemap", "https://example.com/news-sitemap.xml"},
		{"https://example.com/sitemap_index", "https://example.com/sitemap_index.xml"},
		{"https://example.com/page.html", "https://example.com/page.html"},
		{"https://example.com/about", "https://example.com/about"},
		{"https://example.com/Sitemap", "https://example.com/Sitemap.xml"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeSitemapURL(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeSitemapURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParse_RealWorldExamples(t *testing.T) {
	t.Run("complex urlset with all fields", func(t *testing.T) {
		content := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"
        xmlns:news="http://www.google.com/schemas/sitemap-news/0.9"
        xmlns:image="http://www.google.com/schemas/sitemap-image/1.1">
  <url>
    <loc>https://example.com/article1</loc>
    <lastmod>2024-01-15T10:30:00Z</lastmod>
    <changefreq>weekly</changefreq>
    <priority>0.9</priority>
  </url>
  <url>
    <loc>https://example.com/article2</loc>
    <lastmod>2024-01-14T08:00:00Z</lastmod>
    <changefreq>monthly</changefreq>
    <priority>0.7</priority>
  </url>
  <url>
    <loc>https://example.com/article3</loc>
  </url>
</urlset>`

		result, err := Parse([]byte(content))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if len(result.URLs) != 3 {
			t.Fatalf("Parse() returned %d URLs, want 3", len(result.URLs))
		}
		expectedURLs := []string{
			"https://example.com/article1",
			"https://example.com/article2",
			"https://example.com/article3",
		}
		for i, expected := range expectedURLs {
			if result.URLs[i] != expected {
				t.Errorf("URLs[%d] = %v, want %v", i, result.URLs[i], expected)
			}
		}
	})

	t.Run("large sitemap index", func(t *testing.T) {
		content := `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap>
    <loc>https://example.com/sitemap-posts.xml</loc>
    <lastmod>2024-01-15</lastmod>
  </sitemap>
  <sitemap>
    <loc>https://example.com/sitemap-pages.xml</loc>
    <lastmod>2024-01-14</lastmod>
  </sitemap>
  <sitemap>
    <loc>https://example.com/sitemap-categories.xml</loc>
    <lastmod>2024-01-13</lastmod>
  </sitemap>
  <sitemap>
    <loc>https://example.com/sitemap-tags.xml</loc>
    <lastmod>2024-01-12</lastmod>
  </sitemap>
</sitemapindex>`

		result, err := Parse([]byte(content))
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if !result.IsSitemapIndex {
			t.Error("Parse() IsSitemapIndex = false, want true")
		}
		if len(result.ChildMaps) != 4 {
			t.Fatalf("Parse() returned %d child sitemaps, want 4", len(result.ChildMaps))
		}
		expectedMaps := []string{
			"https://example.com/sitemap-posts.xml",
			"https://example.com/sitemap-pages.xml",
			"https://example.com/sitemap-categories.xml",
			"https://example.com/sitemap-tags.xml",
		}
		for i, expected := range expectedMaps {
			if result.ChildMaps[i] != expected {
				t.Errorf("ChildMaps[%d] = %v, want %v", i, result.ChildMaps[i], expected)
			}
		}
	})
}
