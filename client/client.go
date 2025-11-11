package client

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/joeychilson/websurfer/cache"
	"github.com/joeychilson/websurfer/config"
	"github.com/joeychilson/websurfer/fetcher"
	"github.com/joeychilson/websurfer/parser"
	htmlparser "github.com/joeychilson/websurfer/parser/html"
	"github.com/joeychilson/websurfer/parser/pdf"
	"github.com/joeychilson/websurfer/parser/rules"
	"github.com/joeychilson/websurfer/ratelimit"
	"github.com/joeychilson/websurfer/retry"
	"github.com/joeychilson/websurfer/robots"
	"golang.org/x/net/html"
)

// Client orchestrates all components to fetch web content respectfully.
type Client struct {
	config         *config.Config
	robotsChecker  *robots.Checker
	limiter        *ratelimit.Limiter
	parser         *parser.Registry
	cache          *cache.Cache
	logger         *slog.Logger
	refreshing     sync.Map
	userAgent      string
	robotsCacheTTL time.Duration
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
}

// Response represents a fetched webpage with metadata.
type Response struct {
	URL         string
	StatusCode  int
	Headers     map[string][]string
	Body        []byte
	Title       string
	Description string
	CacheState  string
	CachedAt    time.Time
}

// New creates a new Client with the given configuration.
func New(cfg *config.Config) (*Client, error) {
	if cfg == nil {
		cfg = config.New()
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	userAgent := cfg.Default.Fetch.UserAgent
	if userAgent == "" {
		userAgent = config.DefaultUserAgent
	}

	robotsCacheTTL := cfg.Default.Fetch.GetRobotsTxtCacheTTL()

	limiterConfig := cfg.Default.RateLimit
	limiterConfig.RespectRetryAfter = true
	limiter := ratelimit.New(limiterConfig)

	htmlParser := htmlparser.New(
		htmlparser.WithRules(
			rules.NewSECRule(),
			rules.NewSECTableRule(),
		),
	)
	pdfParser := pdf.New()

	parserRegistry := parser.New()
	parserRegistry.Register([]string{"text/html", "application/xhtml+xml"}, htmlParser)
	parserRegistry.Register([]string{"application/pdf"}, pdfParser)

	f := fetcher.New(cfg.Default.Fetch)
	robotsClient := f.GetHTTPClient()

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	return &Client{
		config:         cfg,
		robotsChecker:  robots.New(userAgent, robotsCacheTTL, robotsClient),
		limiter:        limiter,
		parser:         parserRegistry,
		cache:          nil,
		logger:         slog.Default(),
		userAgent:      userAgent,
		robotsCacheTTL: robotsCacheTTL,
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
	}, nil
}

// NewFromFile creates a new Client by loading configuration from a YAML file.
func NewFromFile(path string) (*Client, error) {
	cfg, err := config.LoadConfig(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	return New(cfg)
}

// WithCache sets the cache for response caching.
func (c *Client) WithCache(responseCache *cache.Cache) *Client {
	c.cache = responseCache
	return c
}

// WithLogger sets the logger for the client.
func (c *Client) WithLogger(log *slog.Logger) *Client {
	c.logger = log
	return c
}

// Close releases resources used by the client.
// It must be called when the client is no longer needed to prevent goroutine leaks.
// This will cancel any ongoing background refreshes and close the rate limiter.
func (c *Client) Close() {
	if c.shutdownCancel != nil {
		c.shutdownCancel()
	}
	if c.limiter != nil {
		c.limiter.Close()
	}
}

// Fetch retrieves content from the given URL, respecting robots.txt and rate limits.
func (c *Client) Fetch(ctx context.Context, urlStr string) (*Response, error) {
	c.logger.Debug("fetch started", "url", urlStr)

	entry, err := c.cache.Get(ctx, urlStr)
	if err != nil {
		c.logger.Error("cache get failed", "url", urlStr, "error", err)
		entry = nil
	}

	if entry != nil {
		if entry.IsFresh() {
			c.logger.Debug("cache hit (fresh)", "url", urlStr)
			return c.buildResponse(entry, "hit"), nil
		}

		if entry.IsStale() {
			c.logger.Debug("cache hit (stale, refreshing in background)", "url", urlStr)
			c.startBackgroundRefresh(urlStr, entry)
			return c.buildResponse(entry, "stale"), nil
		}

		c.logger.Debug("cache entry too old", "url", urlStr)
	} else {
		c.logger.Debug("cache miss", "url", urlStr)
	}

	entry, err = c.fetchAndCache(ctx, urlStr)
	if err != nil {
		c.logger.Error("fetch failed", "url", urlStr, "error", err)
		return nil, err
	}

	if err := c.cache.Set(ctx, entry); err != nil {
		c.logger.Error("cache set failed", "url", urlStr, "error", err)
	}

	c.logger.Info("fetch completed", "url", urlStr, "status_code", entry.StatusCode, "body_size", len(entry.Body))
	return c.buildResponse(entry, "miss"), nil
}

// buildResponse creates a Response from a cache Entry.
func (c *Client) buildResponse(entry *cache.Entry, cacheState string) *Response {
	cachedAt := entry.StoredAt
	if cacheState == "miss" {
		cachedAt = time.Time{}
	}
	return &Response{
		URL:         entry.URL,
		StatusCode:  entry.StatusCode,
		Headers:     entry.Headers,
		Body:        entry.Body,
		Title:       entry.Title,
		Description: entry.Description,
		CacheState:  cacheState,
		CachedAt:    cachedAt,
	}
}

// startBackgroundRefresh initiates a background refresh of stale cache content.
func (c *Client) startBackgroundRefresh(urlStr string, entry *cache.Entry) {
	if _, loaded := c.refreshing.LoadOrStore(urlStr, struct{}{}); !loaded {
		go c.refreshInBackground(urlStr, entry)
	} else {
		c.logger.Debug("background refresh already in progress", "url", urlStr)
	}
}

// refreshInBackground performs the actual background refresh work.
func (c *Client) refreshInBackground(urlStr string, entry *cache.Entry) {
	defer func() {
		c.refreshing.Delete(urlStr)
		if r := recover(); r != nil {
			c.logger.Error("background refresh panicked", "url", urlStr, "panic", r)
		}
	}()

	c.logger.Debug("background refresh started", "url", urlStr)

	refreshCtx, cancel := context.WithTimeout(c.shutdownCtx, 30*time.Second)
	defer cancel()

	newEntry, err := c.fetchAndCacheConditional(refreshCtx, urlStr, entry.LastModified)
	if err != nil {
		if c.shutdownCtx.Err() != nil {
			c.logger.Debug("background refresh cancelled due to shutdown", "url", urlStr)
			return
		}
		c.logger.Error("background refresh failed", "url", urlStr, "error", err)
		return
	}

	if newEntry != nil {
		c.handleRefreshWithNewContent(refreshCtx, urlStr, newEntry)
	} else {
		c.handleRefreshNotModified(refreshCtx, urlStr, entry)
	}
}

// handleRefreshWithNewContent stores newly fetched content from background refresh.
func (c *Client) handleRefreshWithNewContent(ctx context.Context, urlStr string, newEntry *cache.Entry) {
	if err := c.cache.Set(ctx, newEntry); err != nil {
		c.logger.Error("background refresh cache set failed", "url", urlStr, "error", err)
	} else {
		c.logger.Debug("background refresh completed with new content", "url", urlStr)
	}
}

// handleRefreshNotModified updates the cache timestamp when content hasn't changed.
func (c *Client) handleRefreshNotModified(ctx context.Context, urlStr string, entry *cache.Entry) {
	c.logger.Debug("background refresh: content not modified", "url", urlStr)
	updatedEntry := &cache.Entry{
		URL:          entry.URL,
		StatusCode:   entry.StatusCode,
		Headers:      entry.Headers,
		Body:         entry.Body,
		Title:        entry.Title,
		Description:  entry.Description,
		LastModified: entry.LastModified,
		StoredAt:     time.Now(),
		TTL:          entry.TTL,
		StaleTime:    entry.StaleTime,
	}
	if err := c.cache.Set(ctx, updatedEntry); err != nil {
		c.logger.Error("background refresh timestamp update failed", "url", urlStr, "error", err)
	} else {
		c.logger.Debug("background refresh completed (not modified)", "url", urlStr)
	}
}

// FetchNoCache retrieves content from the given URL without using cache.
func (c *Client) FetchNoCache(ctx context.Context, urlStr string) (*Response, error) {
	c.logger.Debug("fetch started (no cache)", "url", urlStr)

	entry, err := c.fetchAndCache(ctx, urlStr)
	if err != nil {
		c.logger.Error("fetch failed", "url", urlStr, "error", err)
		return nil, err
	}

	c.logger.Info("fetch completed (no cache)", "url", urlStr, "status_code", entry.StatusCode, "body_size", len(entry.Body))
	return &Response{
		URL:         entry.URL,
		StatusCode:  entry.StatusCode,
		Headers:     entry.Headers,
		Body:        entry.Body,
		Title:       entry.Title,
		Description: entry.Description,
		CacheState:  "miss",
		CachedAt:    time.Time{},
	}, nil
}

// fetchAndCache performs the actual fetch operation with all protections.
func (c *Client) fetchAndCache(ctx context.Context, urlStr string) (*cache.Entry, error) {
	return c.fetchAndCacheConditional(ctx, urlStr, "")
}

// fetchAndCacheConditional performs the actual fetch operation with conditional request support.
func (c *Client) fetchAndCacheConditional(ctx context.Context, urlStr string, cachedLastModified string) (*cache.Entry, error) {
	resolved := c.config.GetConfigForURL(urlStr)

	crawlDelay, err := c.checkRobotsTxt(ctx, urlStr, &resolved)
	if err != nil {
		return nil, err
	}

	if crawlDelay > 0 {
		c.applyCrawlDelayToLimiter(urlStr, crawlDelay)
	}

	fetcherResp, err := c.performFetch(ctx, urlStr, resolved, cachedLastModified)
	if err != nil {
		return nil, err
	}

	if fetcherResp.StatusCode == 304 {
		c.logger.Debug("content not modified, reusing cached content", "url", urlStr)
		return nil, nil
	}

	return c.buildCacheEntry(ctx, urlStr, fetcherResp)
}

// checkRobotsTxt checks robots.txt and returns the crawl delay if configured.
func (c *Client) checkRobotsTxt(ctx context.Context, urlStr string, resolved *config.ResolvedConfig) (time.Duration, error) {
	if !resolved.Fetch.RespectRobotsTxt {
		return 0, nil
	}

	allowed, err := c.robotsChecker.IsAllowed(ctx, urlStr)
	if err != nil {
		c.logger.Error("robots.txt check failed", "url", urlStr, "error", err)
		return 0, fmt.Errorf("robots.txt check failed: %w", err)
	}
	if !allowed {
		c.logger.Warn("blocked by robots.txt", "url", urlStr)
		return 0, fmt.Errorf("disallowed by robots.txt: %s", urlStr)
	}

	delay, err := c.robotsChecker.GetCrawlDelay(ctx, urlStr)
	if err == nil && delay > 0 {
		c.logger.Debug("applying crawl-delay from robots.txt", "url", urlStr, "delay", delay)
		*resolved = c.applyCrawlDelay(*resolved, delay)
		return delay, nil
	}

	return 0, nil
}

// applyCrawlDelayToLimiter updates the rate limiter with the crawl delay.
func (c *Client) applyCrawlDelayToLimiter(urlStr string, crawlDelay time.Duration) {
	fakeHeaders := http.Header{}
	fakeHeaders.Set("Retry-After", fmt.Sprintf("%.0f", crawlDelay.Seconds()))
	c.limiter.UpdateRetryAfter(urlStr, fakeHeaders)
}

// performFetch executes the HTTP fetch with retry logic.
func (c *Client) performFetch(ctx context.Context, urlStr string, resolved config.ResolvedConfig, cachedLastModified string) (*fetcher.Response, error) {
	f := fetcher.New(resolved.Fetch)
	r := retry.New(f, c.limiter, resolved.Retry)

	if cachedLastModified != "" {
		c.logger.Debug("using conditional request", "url", urlStr, "if_modified_since", cachedLastModified)
		opts := &fetcher.FetchOptions{
			IfModifiedSince: cachedLastModified,
		}
		return r.FetchWithOptions(ctx, urlStr, opts)
	}

	return r.Fetch(ctx, urlStr)
}

// buildCacheEntry constructs a cache entry from the fetcher response.
func (c *Client) buildCacheEntry(ctx context.Context, urlStr string, fetcherResp *fetcher.Response) (*cache.Entry, error) {
	contentType := c.getFirstHeader(fetcherResp.Headers, "Content-Type")
	lastModified := c.getFirstHeader(fetcherResp.Headers, "Last-Modified")

	title, description := c.extractMetadata(contentType, fetcherResp.Body)
	body, err := c.parseContent(ctx, urlStr, contentType, fetcherResp.Body)
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
		LastModified: lastModified,
		StoredAt:     time.Now(),
	}, nil
}

// getFirstHeader returns the first value of a header or empty string if not found.
func (c *Client) getFirstHeader(headers map[string][]string, key string) string {
	if values, ok := headers[key]; ok && len(values) > 0 {
		return values[0]
	}
	return ""
}

// extractMetadata extracts title and description from HTML content.
func (c *Client) extractMetadata(contentType string, body []byte) (string, string) {
	if strings.Contains(strings.ToLower(contentType), "html") && len(body) > 0 {
		return extractMetadataFromHTML(string(body))
	}
	return "", ""
}

// parseContent parses the response body using the appropriate parser.
func (c *Client) parseContent(ctx context.Context, urlStr, contentType string, body []byte) ([]byte, error) {
	if len(body) == 0 || !c.parser.HasParser(contentType) {
		return body, nil
	}

	c.logger.Debug("parsing content", "url", urlStr, "content_type", contentType, "original_size", len(body))

	parserCtx := ctx
	if urlStr != "" {
		parserCtx = parser.WithURL(ctx, urlStr)
	}

	parsed, err := c.parser.Parse(parserCtx, contentType, body)
	if err != nil {
		c.logger.Error("failed to parse content", "url", urlStr, "content_type", contentType, "error", err)
		return nil, fmt.Errorf("failed to parse content: %w", err)
	}

	c.logger.Debug("parsing completed", "url", urlStr, "original_size", len(body), "parsed_size", len(parsed))
	return parsed, nil
}

// applyCrawlDelay merges crawl-delay from robots.txt into the rate limit config.
func (c *Client) applyCrawlDelay(resolved config.ResolvedConfig, crawlDelay time.Duration) config.ResolvedConfig {
	resolved.RateLimit.RespectRetryAfter = true

	if resolved.RateLimit.Delay == 0 || crawlDelay > resolved.RateLimit.Delay {
		resolved.RateLimit.Delay = crawlDelay
		resolved.RateLimit.RequestsPerSecond = 0
	}

	return resolved
}

// extractMetadataFromHTML extracts title and description from HTML by parsing the DOM.
func extractMetadataFromHTML(htmlContent string) (title, description string) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", ""
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
			}
		}

		for c := node.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
	}

	extract(doc)

	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)

	return title, description
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
