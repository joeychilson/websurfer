package sitemap

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// URLSet represents a sitemap.xml file
type URLSet struct {
	XMLName xml.Name `xml:"urlset"`
	URLs    []URL    `xml:"url"`
}

// SitemapIndex represents a sitemap index file that references other sitemaps
type SitemapIndex struct {
	XMLName  xml.Name  `xml:"sitemapindex"`
	Sitemaps []Sitemap `xml:"sitemap"`
}

// Sitemap represents a reference to another sitemap
type Sitemap struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

// URL represents a single URL entry in a sitemap
type URL struct {
	Loc        string  `xml:"loc"`
	LastMod    string  `xml:"lastmod,omitempty"`
	ChangeFreq string  `xml:"changefreq,omitempty"`
	Priority   float64 `xml:"priority,omitempty"`
}

// ParseResult holds the result of parsing a sitemap
type ParseResult struct {
	URLs           []string
	ChildMaps      []string
	IsSitemapIndex bool
}

// Parse parses sitemap XML content and returns URLs or child sitemap references
func Parse(content []byte) (*ParseResult, error) {
	var urlset URLSet
	if err := xml.Unmarshal(content, &urlset); err == nil && len(urlset.URLs) > 0 {
		urls := make([]string, 0, len(urlset.URLs))
		for _, u := range urlset.URLs {
			if u.Loc != "" {
				urls = append(urls, u.Loc)
			}
		}
		return &ParseResult{
			URLs:           urls,
			IsSitemapIndex: false,
		}, nil
	}

	var index SitemapIndex
	if err := xml.Unmarshal(content, &index); err == nil && len(index.Sitemaps) > 0 {
		childMaps := make([]string, 0, len(index.Sitemaps))
		for _, s := range index.Sitemaps {
			if s.Loc != "" {
				childMaps = append(childMaps, s.Loc)
			}
		}
		return &ParseResult{
			ChildMaps:      childMaps,
			IsSitemapIndex: true,
		}, nil
	}

	return nil, fmt.Errorf("invalid sitemap format")
}

// ParseReader parses sitemap XML from a reader
func ParseReader(r io.Reader) (*ParseResult, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read sitemap: %w", err)
	}
	return Parse(content)
}

// IsSitemapURL checks if a URL looks like a sitemap
func IsSitemapURL(url string) bool {
	lower := strings.ToLower(url)
	return strings.Contains(lower, "sitemap")
}

// NormalizeSitemapURL ensures a sitemap URL ends with .xml
func NormalizeSitemapURL(url string) string {
	if strings.HasSuffix(strings.ToLower(url), ".xml") {
		return url
	}
	if strings.Contains(strings.ToLower(url), "sitemap") {
		return url + ".xml"
	}
	return url
}
