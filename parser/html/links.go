package html

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// ExtractLinks extracts all href URLs from anchor tags in HTML content.
// It resolves relative URLs to absolute URLs using the provided base URL.
// Returns a deduplicated list of URLs.
func ExtractLinks(htmlContent string, baseURL string) ([]string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return nil, err
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var urls []string

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" && attr.Val != "" {
					href := strings.TrimSpace(attr.Val)
					if strings.HasPrefix(href, "#") ||
						strings.HasPrefix(href, "javascript:") ||
						strings.HasPrefix(href, "mailto:") ||
						strings.HasPrefix(href, "tel:") {
						continue
					}

					parsed, err := url.Parse(href)
					if err != nil {
						continue
					}

					absolute := base.ResolveReference(parsed)

					absolute.Fragment = ""

					urlStr := absolute.String()

					if !seen[urlStr] {
						seen[urlStr] = true
						urls = append(urls, urlStr)
					}
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}

	traverse(doc)
	return urls, nil
}
