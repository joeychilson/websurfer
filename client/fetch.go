package client

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/joeychilson/websurfer/cache"
	"github.com/joeychilson/websurfer/config"
	"github.com/joeychilson/websurfer/fetcher"
	"github.com/joeychilson/websurfer/parser"
	"github.com/joeychilson/websurfer/ratelimit"
	"github.com/joeychilson/websurfer/retry"
	"github.com/joeychilson/websurfer/robots"
)

// FetchCoordinator coordinates robots.txt checking, rate limiting, and HTTP fetching.
type FetchCoordinator struct {
	config        *config.Config
	robotsChecker *robots.Checker
	limiter       *ratelimit.Limiter
	parser        *parser.Registry
	logger        *slog.Logger
}

// NewFetchCoordinator creates a new fetch coordinator.
func NewFetchCoordinator(
	cfg *config.Config,
	robotsChecker *robots.Checker,
	limiter *ratelimit.Limiter,
	parser *parser.Registry,
	logger *slog.Logger,
) *FetchCoordinator {
	return &FetchCoordinator{
		config:        cfg,
		robotsChecker: robotsChecker,
		limiter:       limiter,
		parser:        parser,
		logger:        logger,
	}
}

// Close releases resources.
func (f *FetchCoordinator) Close() {
	if f.limiter != nil {
		f.limiter.Close()
	}
}

// Fetch performs a complete fetch operation with robots.txt checking, rate limiting, and parsing.
func (f *FetchCoordinator) Fetch(ctx context.Context, urlStr string, ifModifiedSince string) (*cache.Entry, error) {
	resolved := f.config.GetConfigForURL(urlStr)

	crawlDelay, err := f.checkRobotsTxt(ctx, urlStr, &resolved)
	if err != nil {
		return nil, err
	}

	if crawlDelay > 0 {
		f.applyCrawlDelayToLimiter(urlStr, crawlDelay)
	}

	fetcherResp, err := f.performFetch(ctx, urlStr, resolved, ifModifiedSince)
	if err != nil {
		return nil, err
	}

	if fetcherResp.StatusCode == 304 {
		f.logger.Debug("content not modified, reusing cached content", "url", urlStr)
		return nil, nil
	}

	return f.buildCacheEntry(ctx, urlStr, fetcherResp)
}

// checkRobotsTxt checks robots.txt and returns the crawl delay if configured.
func (f *FetchCoordinator) checkRobotsTxt(ctx context.Context, urlStr string, resolved *config.ResolvedConfig) (time.Duration, error) {
	if !resolved.Fetch.GetRespectRobotsTxt() {
		return 0, nil
	}

	allowed, err := f.robotsChecker.IsAllowed(ctx, urlStr)
	if err != nil {
		f.logger.Error("robots.txt check failed", "url", urlStr, "error", err)
		return 0, fmt.Errorf("robots.txt check failed: %w", err)
	}
	if !allowed {
		f.logger.Warn("blocked by robots.txt", "url", urlStr)
		return 0, fmt.Errorf("disallowed by robots.txt: %s", urlStr)
	}

	delay, err := f.robotsChecker.GetCrawlDelay(ctx, urlStr)
	if err == nil && delay > 0 {
		f.logger.Debug("applying crawl-delay from robots.txt", "url", urlStr, "delay", delay)
		return delay, nil
	}

	return 0, nil
}

// applyCrawlDelayToLimiter updates the rate limiter with the crawl delay.
func (f *FetchCoordinator) applyCrawlDelayToLimiter(urlStr string, crawlDelay time.Duration) {
	fakeHeaders := http.Header{}
	fakeHeaders.Set("Retry-After", fmt.Sprintf("%.0f", crawlDelay.Seconds()))
	f.limiter.UpdateRetryAfter(urlStr, fakeHeaders)
}

// performFetch executes the HTTP fetch with retry logic.
func (f *FetchCoordinator) performFetch(ctx context.Context, urlStr string, resolved config.ResolvedConfig, cachedLastModified string) (*fetcher.Response, error) {
	fetch, err := fetcher.New(resolved.Fetch)
	if err != nil {
		return nil, fmt.Errorf("failed to create fetcher: %w", err)
	}
	r := retry.New(fetch, f.limiter, resolved.Retry)

	if cachedLastModified != "" {
		f.logger.Debug("using conditional request", "url", urlStr, "if_modified_since", cachedLastModified)
		opts := &fetcher.FetchOptions{
			IfModifiedSince: cachedLastModified,
		}
		return r.FetchWithOptions(ctx, urlStr, opts)
	}

	return r.Fetch(ctx, urlStr)
}

// buildCacheEntry constructs a cache entry from the fetcher response.
func (f *FetchCoordinator) buildCacheEntry(ctx context.Context, urlStr string, fetcherResp *fetcher.Response) (*cache.Entry, error) {
	var (
		contentType  string
		lastModified string
	)
	if values, ok := fetcherResp.Headers["Content-Type"]; ok && len(values) > 0 {
		contentType = values[0]
	}
	if values, ok := fetcherResp.Headers["Last-Modified"]; ok && len(values) > 0 {
		lastModified = values[0]
	}

	var title, description, faviconURL string
	if strings.Contains(strings.ToLower(contentType), "html") && len(fetcherResp.Body) > 0 {
		title, description, faviconURL = extractMetadataFromHTML(fetcherResp.Body)
		if faviconURL != "" {
			faviconURL = resolveFaviconURL(fetcherResp.URL, faviconURL)
		}
	}

	body, err := f.parseContent(ctx, urlStr, contentType, fetcherResp.Body)
	if err != nil {
		return nil, err
	}

	return &cache.Entry{
		URL:          fetcherResp.URL,
		StatusCode:   fetcherResp.StatusCode,
		Headers:      fetcherResp.Headers,
		Body:         body,
		Title:        title,
		Description:  description,
		FaviconURL:   faviconURL,
		LastModified: lastModified,
		StoredAt:     time.Now(),
	}, nil
}

// parseContent parses the response body using the appropriate parser.
func (f *FetchCoordinator) parseContent(ctx context.Context, urlStr, contentType string, body []byte) ([]byte, error) {
	if len(body) == 0 || !f.parser.HasParser(contentType) {
		return body, nil
	}

	f.logger.Debug("parsing content", "url", urlStr, "content_type", contentType, "original_size", len(body))

	parserCtx := ctx
	if urlStr != "" {
		parserCtx = parser.WithURL(ctx, urlStr)
	}

	parsed, err := f.parser.Parse(parserCtx, contentType, body)
	if err != nil {
		f.logger.Error("failed to parse content", "url", urlStr, "content_type", contentType, "error", err)
		return nil, fmt.Errorf("failed to parse content: %w", err)
	}

	f.logger.Debug("parsing completed", "url", urlStr, "original_size", len(body), "parsed_size", len(parsed))
	return parsed, nil
}

// extractMetadataFromHTML extracts title, description, and favicon URL from HTML by parsing the DOM.
func extractMetadataFromHTML(htmlContent []byte) (title, description, faviconURL string) {
	doc, err := html.Parse(bytes.NewReader(htmlContent))
	if err != nil {
		return "", "", ""
	}

	var extract func(*html.Node)
	extract = func(node *html.Node) {
		if node.Type == html.ElementNode {
			switch node.Data {
			case "title":
				if title == "" {
					title = getNodeText(node)
				}
			case "meta":
				if description == "" {
					name := getAttr(node, "name")
					property := getAttr(node, "property")

					if name == "description" {
						description = getAttr(node, "content")
					}
					if property == "og:description" && description == "" {
						description = getAttr(node, "content")
					}
				}
			case "link":
				if faviconURL == "" {
					rel := strings.ToLower(getAttr(node, "rel"))
					if rel == "icon" || rel == "shortcut icon" || rel == "apple-touch-icon" {
						href := getAttr(node, "href")
						if href != "" {
							faviconURL = href
						}
					}
				}
			}
		}

		for c := node.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
	}

	extract(doc)

	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)

	return title, description, faviconURL
}

// getNodeText extracts all text content from a node and its children.
func getNodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}

	var text strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		text.WriteString(getNodeText(c))
	}

	return text.String()
}

// getAttr returns the value of an attribute from an HTML node.
func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// resolveFaviconURL resolves a relative favicon URL to an absolute URL using the base page URL.
func resolveFaviconURL(baseURL, faviconPath string) string {
	if faviconPath == "" {
		return ""
	}

	base, err := url.Parse(baseURL)
	if err != nil {
		return faviconPath
	}

	favicon, err := url.Parse(faviconPath)
	if err != nil {
		return faviconPath
	}

	resolved := base.ResolveReference(favicon)
	return resolved.String()
}
