package client

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/joeychilson/websurfer/cache"
	"github.com/joeychilson/websurfer/config"
	"github.com/joeychilson/websurfer/fetcher"
	"github.com/joeychilson/websurfer/parser"
	htmlparser "github.com/joeychilson/websurfer/parser/html"
	"github.com/joeychilson/websurfer/parser/pdf"
	"github.com/joeychilson/websurfer/parser/rules"
	"github.com/joeychilson/websurfer/ratelimit"
	"github.com/joeychilson/websurfer/robots"
)

// Client is a thin facade that coordinates FetchCoordinator and CacheManager.
type Client struct {
	coordinator  *FetchCoordinator
	cacheManager *CacheManager
	logger       *slog.Logger
}

// New creates a new Client with the given configuration.
func New(cfg *config.Config) (*Client, error) {
	if cfg == nil {
		cfg = config.New()
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	logger := slog.Default()

	userAgent := cfg.Default.Fetch.UserAgent
	if userAgent == "" {
		userAgent = config.DefaultUserAgent
	}

	robotsCacheTTL := cfg.Default.Fetch.GetRobotsTxtCacheTTL()
	f, err := fetcher.New(cfg.Default.Fetch)
	if err != nil {
		return nil, fmt.Errorf("failed to create fetcher: %w", err)
	}
	robotsClient := f.GetHTTPClient()
	robotsChecker := robots.New(userAgent, robotsCacheTTL, robotsClient)

	limiterConfig := cfg.Default.RateLimit
	respectRetryAfter := true
	limiterConfig.RespectRetryAfter = &respectRetryAfter
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

	coordinator := NewFetchCoordinator(cfg, robotsChecker, limiter, parserRegistry, logger)
	cacheManager := NewCacheManager(nil, logger, coordinator)

	return &Client{
		coordinator:  coordinator,
		cacheManager: cacheManager,
		logger:       logger,
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
	c.cacheManager.cache = responseCache
	return c
}

// WithLogger sets the logger for the client.
func (c *Client) WithLogger(log *slog.Logger) *Client {
	c.logger = log
	c.coordinator.logger = log
	c.cacheManager.logger = log
	return c
}

// Close releases resources used by the client.
func (c *Client) Close() {
	c.cacheManager.Close()
	c.coordinator.Close()
}

// Response represents a fetched webpage with metadata.
type Response struct {
	URL         string
	StatusCode  int
	Headers     map[string][]string
	Body        []byte
	Title       string
	Description string
	FaviconURL  string
	CacheState  string
	CachedAt    time.Time
}

// Fetch retrieves content from the given URL, respecting robots.txt and rate limits.
func (c *Client) Fetch(ctx context.Context, urlStr string) (*Response, error) {
	c.logger.Debug("fetch started", "url", urlStr)

	entry := c.cacheManager.Get(ctx, urlStr)

	if entry != nil {
		state := entry.GetState()

		switch state {
		case cache.StateFresh:
			c.logger.Debug("cache hit (fresh)", "url", urlStr)
			return buildResponse(entry, "hit"), nil

		case cache.StateStale:
			c.logger.Debug("cache hit (stale, refreshing in background)", "url", urlStr)
			c.cacheManager.StartBackgroundRefresh(urlStr, entry)
			return buildResponse(entry, "stale"), nil

		case cache.StateTooOld:
			c.logger.Debug("cache entry too old", "url", urlStr)
		}
	} else {
		c.logger.Debug("cache miss", "url", urlStr)
	}

	entry, err := c.coordinator.Fetch(ctx, urlStr, "")
	if err != nil {
		c.logger.Error("fetch failed", "url", urlStr, "error", err)
		return nil, err
	}

	c.cacheManager.Set(ctx, entry)

	c.logger.Info("fetch completed", "url", urlStr, "status_code", entry.StatusCode, "body_size", len(entry.Body))
	return buildResponse(entry, "miss"), nil
}

// buildResponse creates a Response from a cache Entry.
func buildResponse(entry *cache.Entry, cacheState string) *Response {
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
		FaviconURL:  entry.FaviconURL,
		CacheState:  cacheState,
		CachedAt:    cachedAt,
	}
}
