package client

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/joeychilson/websurfer/cache"
	"github.com/joeychilson/websurfer/config"
	"github.com/joeychilson/websurfer/fetcher"
	"github.com/joeychilson/websurfer/logger"
	"github.com/joeychilson/websurfer/parser"
	htmlparser "github.com/joeychilson/websurfer/parser/html"
	"github.com/joeychilson/websurfer/parser/pdf"
	"github.com/joeychilson/websurfer/parser/rules"
	"github.com/joeychilson/websurfer/parser/sitemap"
	"github.com/joeychilson/websurfer/ratelimit"
	"github.com/joeychilson/websurfer/retry"
	"github.com/joeychilson/websurfer/robots"
)

// Client orchestrates all components to fetch web content respectfully.
type Client struct {
	config         *config.Config
	robotsChecker  *robots.Checker
	limiter        *ratelimit.Limiter
	parser         *parser.Registry
	cache          cache.Cache
	logger         logger.Logger
	refreshing     sync.Map
	userAgent      string
	robotsCacheTTL time.Duration
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

var (
	titleRegex           = regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
	descriptionMetaRegex = regexp.MustCompile(`(?is)<meta\s+(?:name=["']description["']\s+content=["']([^"']+)["']|content=["']([^"']+)["']\s+name=["']description["']|property=["']og:description["']\s+content=["']([^"']+)["']|content=["']([^"']+)["']\s+property=["']og:description["'])`)
)

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

	defaultCache := cache.NewMemoryCache(cache.DefaultConfig())

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

	return &Client{
		config:         cfg,
		robotsChecker:  robots.New(userAgent, robotsCacheTTL, defaultCache, robotsClient),
		limiter:        limiter,
		parser:         parserRegistry,
		cache:          defaultCache,
		logger:         logger.Noop(),
		userAgent:      userAgent,
		robotsCacheTTL: robotsCacheTTL,
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

// WithCache sets the cache for the client and updates robots checker to use the same cache.
func (c *Client) WithCache(sharedCache cache.Cache) *Client {
	c.cache = sharedCache
	c.robotsChecker = robots.New(c.userAgent, c.robotsCacheTTL, sharedCache, nil)
	return c
}

// WithLogger sets the logger for the client.
func (c *Client) WithLogger(log logger.Logger) *Client {
	c.logger = log
	return c
}

// Fetch retrieves content from the given URL, respecting robots.txt and rate limits.
// If caching is enabled, implements stale-while-revalidate behavior.
func (c *Client) Fetch(ctx context.Context, urlStr string) (*Response, error) {
	c.logger.Debug("fetch started", "url", urlStr)

	if c.cache != nil {
		entry, err := c.cache.Get(ctx, urlStr)
		if err != nil {
			c.logger.Error("cache get failed", "url", urlStr, "error", err)
			return nil, fmt.Errorf("cache get failed: %w", err)
		}

		if entry != nil {
			if entry.IsFresh() {
				c.logger.Debug("cache hit (fresh)", "url", urlStr)
				return &Response{
					URL:         entry.URL,
					StatusCode:  entry.StatusCode,
					Headers:     entry.Headers,
					Body:        entry.Body,
					Title:       entry.Title,
					Description: entry.Description,
					CacheState:  "hit",
					CachedAt:    entry.StoredAt,
				}, nil
			}

			if entry.IsStale() {
				c.logger.Debug("cache hit (stale, refreshing in background)", "url", urlStr)
				if _, loaded := c.refreshing.LoadOrStore(urlStr, struct{}{}); !loaded {
					go func() {
						defer func() {
							c.refreshing.Delete(urlStr)
							if r := recover(); r != nil {
								c.logger.Error("background refresh panicked", "url", urlStr, "panic", r)
							}
						}()
						c.logger.Debug("background refresh started", "url", urlStr)

						refreshCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer cancel()

						newEntry, err := c.fetchAndCacheConditional(refreshCtx, urlStr, entry.LastModified)
						if err == nil && newEntry != nil {
							if err := c.cache.Set(refreshCtx, newEntry); err != nil {
								c.logger.Error("background refresh cache set failed", "url", urlStr, "error", err)
							} else {
								c.logger.Debug("background refresh completed with new content", "url", urlStr)
							}
						} else if err == nil && newEntry == nil {
							c.logger.Debug("background refresh: content not modified", "url", urlStr)
							entry.StoredAt = time.Now()
							if err := c.cache.Set(refreshCtx, entry); err != nil {
								c.logger.Error("background refresh timestamp update failed", "url", urlStr, "error", err)
							} else {
								c.logger.Debug("background refresh completed (not modified)", "url", urlStr)
							}
						} else if err != nil {
							c.logger.Error("background refresh failed", "url", urlStr, "error", err)
						}
					}()
				} else {
					c.logger.Debug("background refresh already in progress", "url", urlStr)
				}

				return &Response{
					URL:         entry.URL,
					StatusCode:  entry.StatusCode,
					Headers:     entry.Headers,
					Body:        entry.Body,
					Title:       entry.Title,
					Description: entry.Description,
					CacheState:  "stale",
					CachedAt:    entry.StoredAt,
				}, nil
			}

			c.logger.Debug("cache entry too old", "url", urlStr)
		} else {
			c.logger.Debug("cache miss", "url", urlStr)
		}
	}

	entry, err := c.fetchAndCache(ctx, urlStr)
	if err != nil {
		c.logger.Error("fetch failed", "url", urlStr, "error", err)
		return nil, err
	}

	if c.cache != nil && entry != nil {
		if err := c.cache.Set(ctx, entry); err != nil {
			c.logger.Error("cache set failed", "url", urlStr, "error", err)
		}
	}

	c.logger.Info("fetch completed", "url", urlStr, "status_code", entry.StatusCode, "body_size", len(entry.Body))
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

// FetchNoCache retrieves content from the given URL without using cache.
// It still respects robots.txt and rate limits.
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

// GetSitemapsFromRobotsTxt retrieves sitemap URLs declared in robots.txt for a URL.
// Returns an empty slice if robots.txt doesn't exist or doesn't declare sitemaps.
func (c *Client) GetSitemapsFromRobotsTxt(ctx context.Context, urlStr string) ([]string, error) {
	return c.robotsChecker.GetSitemaps(ctx, urlStr)
}

// FetchSitemapURLs fetches and parses a sitemap URL, returning all page URLs.
// Recursively fetches child sitemaps up to maxDepth levels deep.
// Results are cached according to the SitemapCacheTTL configuration.
func (c *Client) FetchSitemapURLs(ctx context.Context, sitemapURL string, maxDepth int) ([]string, error) {
	return c.fetchSitemapRecursive(ctx, sitemapURL, 0, maxDepth)
}

// fetchSitemapRecursive recursively fetches a sitemap and all child sitemaps it references.
// Checks cache first, and stores parsed results in cache for subsequent requests.
func (c *Client) fetchSitemapRecursive(ctx context.Context, sitemapURL string, depth int, maxDepth int) ([]string, error) {
	if depth >= maxDepth {
		c.logger.Warn("max sitemap recursion depth reached", "url", sitemapURL, "depth", depth)
		return nil, nil
	}

	cacheKey := "sitemap:" + sitemapURL
	if cached, err := c.cache.Get(ctx, cacheKey); err == nil && cached != nil {
		c.logger.Debug("sitemap cache hit", "url", sitemapURL)
		urls := strings.Split(strings.TrimSpace(string(cached.Body)), "\n")
		if len(urls) == 1 && urls[0] == "" {
			return []string{}, nil
		}
		return urls, nil
	}

	c.logger.Debug("sitemap cache miss, fetching", "url", sitemapURL)

	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := c.Fetch(fetchCtx, sitemapURL)
	if err != nil {
		c.logger.Debug("failed to fetch sitemap", "url", sitemapURL, "error", err)
		return nil, err
	}

	if resp.StatusCode != 200 {
		c.logger.Debug("sitemap returned non-200 status", "url", sitemapURL, "status", resp.StatusCode)
		return nil, fmt.Errorf("sitemap returned status %d", resp.StatusCode)
	}

	result, err := c.parseSitemap(resp.Body)
	if err != nil {
		c.logger.Debug("failed to parse sitemap", "url", sitemapURL, "error", err)
		return nil, err
	}

	if result == nil {
		c.cacheSitemapURLs(ctx, sitemapURL, []string{})
		return []string{}, nil
	}

	pageURLs := make([]string, 0)

	if result.IsSitemapIndex {
		c.logger.Debug("found nested sitemap index", "url", sitemapURL, "child_count", len(result.ChildMaps))

		for _, childURL := range result.ChildMaps {
			childURLs, err := c.fetchSitemapRecursive(ctx, childURL, depth+1, maxDepth)
			if err != nil {
				c.logger.Debug("failed to fetch child sitemap", "url", childURL, "error", err)
				continue
			}
			pageURLs = append(pageURLs, childURLs...)
		}

		c.cacheSitemapURLs(ctx, sitemapURL, pageURLs)
		return pageURLs, nil
	}

	for _, u := range result.URLs {
		if c.isSitemapURL(u) {
			normalized := c.normalizeSitemapURL(u)
			c.logger.Debug("found nested child sitemap", "url", u, "normalized", normalized, "depth", depth)
			childURLs, err := c.fetchSitemapRecursive(ctx, normalized, depth+1, maxDepth)
			if err != nil {
				c.logger.Debug("failed to fetch nested sitemap", "url", normalized, "error", err)
				continue
			}
			pageURLs = append(pageURLs, childURLs...)
		} else {
			pageURLs = append(pageURLs, u)
		}
	}

	c.logger.Debug("fetched sitemap", "url", sitemapURL, "page_urls", len(pageURLs), "depth", depth)
	c.cacheSitemapURLs(ctx, sitemapURL, pageURLs)
	return pageURLs, nil
}

// parseSitemap parses sitemap XML content and returns URLs or child sitemap references.
func (c *Client) parseSitemap(content []byte) (*sitemap.ParseResult, error) {
	return sitemap.Parse(content)
}

func (c *Client) isSitemapURL(url string) bool {
	lower := strings.ToLower(url)
	return strings.Contains(lower, "sitemap")
}

func (c *Client) normalizeSitemapURL(url string) string {
	if strings.HasSuffix(strings.ToLower(url), ".xml") {
		return url
	}
	if strings.Contains(strings.ToLower(url), "sitemap") {
		return url + ".xml"
	}
	return url
}

func (c *Client) cacheSitemapURLs(ctx context.Context, sitemapURL string, urls []string) {
	cacheKey := "sitemap:" + sitemapURL

	resolvedConfig := c.config.GetConfigForURL(sitemapURL)
	ttl := resolvedConfig.Fetch.GetSitemapCacheTTL()

	body := []byte(strings.Join(urls, "\n"))

	entry := &cache.Entry{
		URL:       cacheKey,
		Body:      body,
		StoredAt:  time.Now(),
		TTL:       ttl,
		StaleTime: 0,
	}

	if err := c.cache.Set(ctx, entry); err != nil {
		c.logger.Warn("failed to cache sitemap URLs", "url", sitemapURL, "error", err)
	} else {
		c.logger.Debug("cached sitemap URLs", "url", sitemapURL, "count", len(urls), "ttl", ttl)
	}
}

// Close releases resources held by the client.
func (c *Client) Close() error {
	if c.cache != nil {
		return c.cache.Close()
	}
	return nil
}

// fetchAndCache performs the actual fetch operation with all protections.
func (c *Client) fetchAndCache(ctx context.Context, urlStr string) (*cache.Entry, error) {
	return c.fetchAndCacheConditional(ctx, urlStr, "")
}

// fetchAndCacheConditional performs the actual fetch operation with conditional request support.
// If cachedLastModified is provided, it sends an If-Modified-Since header.
// If the server responds with 304 Not Modified, returns nil entry (caller should reuse cached content).
func (c *Client) fetchAndCacheConditional(ctx context.Context, urlStr string, cachedLastModified string) (*cache.Entry, error) {
	resolved := c.config.GetConfigForURL(urlStr)

	var crawlDelay time.Duration
	if resolved.Fetch.RespectRobotsTxt {
		allowed, err := c.robotsChecker.IsAllowed(ctx, urlStr)
		if err != nil {
			c.logger.Error("robots.txt check failed", "url", urlStr, "error", err)
			return nil, fmt.Errorf("robots.txt check failed: %w", err)
		}
		if !allowed {
			c.logger.Warn("blocked by robots.txt", "url", urlStr)
			return nil, fmt.Errorf("disallowed by robots.txt: %s", urlStr)
		}

		delay, err := c.robotsChecker.GetCrawlDelay(ctx, urlStr)
		if err == nil && delay > 0 {
			c.logger.Debug("applying crawl-delay from robots.txt", "url", urlStr, "delay", delay)
			crawlDelay = delay
			resolved = c.applyCrawlDelay(resolved, delay)
		}
	}

	if crawlDelay > 0 {
		fakeHeaders := http.Header{}
		fakeHeaders.Set("Retry-After", fmt.Sprintf("%.0f", crawlDelay.Seconds()))
		c.limiter.UpdateRetryAfter(urlStr, fakeHeaders)
	}

	f := fetcher.New(resolved.Fetch)
	r := retry.New(f, c.limiter, resolved.Retry)

	var fetcherResp *fetcher.Response
	var err error

	if cachedLastModified != "" {
		c.logger.Debug("using conditional request", "url", urlStr, "if_modified_since", cachedLastModified)
		opts := &fetcher.FetchOptions{
			IfModifiedSince: cachedLastModified,
		}
		fetcherResp, err = r.FetchWithOptions(ctx, urlStr, opts)
	} else {
		fetcherResp, err = r.Fetch(ctx, urlStr)
	}

	if err != nil {
		return nil, err
	}

	if fetcherResp.StatusCode == 304 {
		c.logger.Debug("content not modified, reusing cached content", "url", urlStr)
		return nil, nil
	}

	contentType := ""
	if ct, ok := fetcherResp.Headers["Content-Type"]; ok && len(ct) > 0 {
		contentType = ct[0]
	}

	lastModified := ""
	if lm, ok := fetcherResp.Headers["Last-Modified"]; ok && len(lm) > 0 {
		lastModified = lm[0]
	}

	title := ""
	description := ""
	if strings.Contains(strings.ToLower(contentType), "html") && len(fetcherResp.Body) > 0 {
		htmlContent := string(fetcherResp.Body)
		title = extractTitle(htmlContent)
		description = extractDescription(htmlContent)
	}

	body := fetcherResp.Body
	if len(body) > 0 && c.parser.HasParser(contentType) {
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
		body = parsed
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

// applyCrawlDelay merges crawl-delay from robots.txt into the rate limit config.
func (c *Client) applyCrawlDelay(resolved config.ResolvedConfig, crawlDelay time.Duration) config.ResolvedConfig {
	resolved.RateLimit.RespectRetryAfter = true

	if resolved.RateLimit.Delay == 0 || crawlDelay > resolved.RateLimit.Delay {
		resolved.RateLimit.Delay = crawlDelay
		resolved.RateLimit.RequestsPerSecond = 0
	}

	return resolved
}

// extractTitle extracts the title from HTML content.
func extractTitle(htmlContent string) string {
	if matches := titleRegex.FindStringSubmatch(htmlContent); len(matches) > 1 {
		title := strings.TrimSpace(matches[1])
		return html.UnescapeString(title)
	}
	return ""
}

// extractDescription extracts the meta description from HTML content.
// Checks both standard meta description and Open Graph description.
func extractDescription(htmlContent string) string {
	matches := descriptionMetaRegex.FindStringSubmatch(htmlContent)
	if len(matches) == 0 {
		return ""
	}
	for _, match := range matches[1:] {
		if match != "" {
			return html.UnescapeString(strings.TrimSpace(match))
		}
	}
	return ""
}
