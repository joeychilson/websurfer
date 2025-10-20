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
	"github.com/joeychilson/websurfer/content"
	"github.com/joeychilson/websurfer/fetcher"
	"github.com/joeychilson/websurfer/logger"
	"github.com/joeychilson/websurfer/parser"
	htmlparser "github.com/joeychilson/websurfer/parser/html"
	"github.com/joeychilson/websurfer/parser/pdf"
	"github.com/joeychilson/websurfer/parser/rules"
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

	return &Client{
		config:         cfg,
		robotsChecker:  robots.New(userAgent, robotsCacheTTL, defaultCache, nil),
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
						defer c.refreshing.Delete(urlStr)
						c.logger.Debug("background refresh started", "url", urlStr)

						refreshCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
						defer cancel()

						newEntry, err := c.fetchAndCache(refreshCtx, urlStr)
						if err == nil && newEntry != nil {
							if err := c.cache.Set(refreshCtx, newEntry); err != nil {
								c.logger.Error("background refresh cache set failed", "url", urlStr, "error", err)
							} else {
								c.logger.Debug("background refresh completed", "url", urlStr)
							}
						} else if err != nil {
							c.logger.Error("background refresh failed", "url", urlStr, "error", err)
						} else {
							c.logger.Warn("background refresh returned nil entry without error", "url", urlStr)
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

// FetchAndExtractRange fetches a URL and extracts a specific range from the content.
// This is a convenience method that combines Fetch with content.ExtractRange.
// The returned Response will have its Body modified to contain only the extracted range.
func (c *Client) FetchAndExtractRange(ctx context.Context, urlStr string, opts *content.RangeOptions) (*Response, error) {
	resp, err := c.Fetch(ctx, urlStr)
	if err != nil {
		return nil, err
	}

	if opts != nil {
		extracted, err := content.ExtractRange(string(resp.Body), opts)
		if err != nil {
			return nil, fmt.Errorf("range extraction failed: %w", err)
		}
		resp.Body = []byte(extracted)
	}

	return resp, nil
}

// FetchAndTruncate fetches a URL and truncates the content to fit within maxTokens.
// This is a convenience method that combines Fetch with content.Truncate.
// Returns the response with truncated body, the truncation result, and any error.
func (c *Client) FetchAndTruncate(ctx context.Context, urlStr string, maxTokens int) (*Response, *content.TruncateResult, error) {
	resp, err := c.Fetch(ctx, urlStr)
	if err != nil {
		return nil, nil, err
	}

	contentType := ""
	if ct, ok := resp.Headers["Content-Type"]; ok && len(ct) > 0 {
		contentType = ct[0]
	}

	result := content.Truncate(string(resp.Body), contentType, maxTokens)
	resp.Body = []byte(result.Content)

	return resp, result, nil
}

// GetSitemapsFromRobotsTxt retrieves sitemap URLs declared in robots.txt for a URL.
// Returns an empty slice if robots.txt doesn't exist or doesn't declare sitemaps.
func (c *Client) GetSitemapsFromRobotsTxt(ctx context.Context, urlStr string) ([]string, error) {
	return c.robotsChecker.GetSitemaps(ctx, urlStr)
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

	fetcherResp, err := r.Fetch(ctx, urlStr)
	if err != nil {
		return nil, err
	}

	contentType := ""
	if ct, ok := fetcherResp.Headers["Content-Type"]; ok && len(ct) > 0 {
		contentType = ct[0]
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
		URL:         fetcherResp.URL,
		StatusCode:  fetcherResp.StatusCode,
		Headers:     fetcherResp.Headers,
		Body:        body,
		Title:       title,
		Description: description,
		StoredAt:    time.Now(),
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
	titleRegex := regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
	if matches := titleRegex.FindStringSubmatch(htmlContent); len(matches) > 1 {
		title := strings.TrimSpace(matches[1])
		return html.UnescapeString(title)
	}
	return ""
}

// extractDescription extracts the meta description from HTML content.
// Checks both standard meta description and Open Graph description.
func extractDescription(htmlContent string) string {
	descRegex := regexp.MustCompile(`(?i)<meta\s+name=["']description["']\s+content=["']([^"']+)["']`)
	if matches := descRegex.FindStringSubmatch(htmlContent); len(matches) > 1 {
		return html.UnescapeString(strings.TrimSpace(matches[1]))
	}

	descRegex2 := regexp.MustCompile(`(?i)<meta\s+content=["']([^"']+)["']\s+name=["']description["']`)
	if matches := descRegex2.FindStringSubmatch(htmlContent); len(matches) > 1 {
		return html.UnescapeString(strings.TrimSpace(matches[1]))
	}

	ogDescRegex := regexp.MustCompile(`(?i)<meta\s+property=["']og:description["']\s+content=["']([^"']+)["']`)
	if matches := ogDescRegex.FindStringSubmatch(htmlContent); len(matches) > 1 {
		return html.UnescapeString(strings.TrimSpace(matches[1]))
	}

	ogDescRegex2 := regexp.MustCompile(`(?i)<meta\s+content=["']([^"']+)["']\s+property=["']og:description["']`)
	if matches := ogDescRegex2.FindStringSubmatch(htmlContent); len(matches) > 1 {
		return html.UnescapeString(strings.TrimSpace(matches[1]))
	}
	return ""
}
