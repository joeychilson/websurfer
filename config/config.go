package config

import (
	"fmt"
	"maps"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v2"
)

const (
	DefaultUserAgent = "websurfer/1.0 (webpage retriever; +https://github.com/joeychilson/websurfer)"
)

// Config represents the top-level configuration structure for the webpage retriever.
type Config struct {
	Default DefaultConfig `yaml:"default"`
	Sites   []SiteConfig  `yaml:"sites"`
}

// New returns a new Config with sensible defaults.
func New() *Config {
	return &Config{
		Default: DefaultConfig{
			Fetch: FetchConfig{
				FollowRedirects: true,
			},
		},
		Sites: []SiteConfig{},
	}
}

// ResolvedConfig is the final merged configuration for a specific URL.
type ResolvedConfig struct {
	Cache     CacheConfig
	Fetch     FetchConfig
	RateLimit RateLimitConfig
	Retry     RetryConfig
}

// GetConfigForURL returns the merged configuration for a given URL
func (c *Config) GetConfigForURL(url string) ResolvedConfig {
	resolved := ResolvedConfig{
		Cache:     c.Default.Cache,
		Fetch:     c.Default.Fetch,
		RateLimit: c.Default.RateLimit,
		Retry:     c.Default.Retry,
	}
	for _, site := range c.Sites {
		if matchPattern(url, site.Pattern) {
			if site.Cache != nil {
				resolved.Cache = mergeCache(resolved.Cache, *site.Cache)
			}
			if site.Fetch != nil {
				resolved.Fetch = mergeFetch(resolved.Fetch, *site.Fetch)
			}
			if site.RateLimit != nil {
				resolved.RateLimit = mergeRateLimit(resolved.RateLimit, *site.RateLimit)
			}
			if site.Retry != nil {
				resolved.Retry = mergeRetry(resolved.Retry, *site.Retry)
			}
		}
	}
	return resolved
}

// DefaultConfig contains default settings applied to all sites unless overridden.
type DefaultConfig struct {
	Cache     CacheConfig     `yaml:"cache"`
	Fetch     FetchConfig     `yaml:"fetch"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	Retry     RetryConfig     `yaml:"retry"`
}

// CacheConfig defines caching behavior for fetched webpages.
type CacheConfig struct {
	TTL       time.Duration `yaml:"ttl,omitempty"`
	StaleTime time.Duration `yaml:"stale_time,omitempty"`
}

// IsEnabled returns true if caching is enabled
func (c *CacheConfig) IsEnabled() bool {
	return c.TTL > 0
}

// IsStaleWhileRevalidateEnabled returns true if stale while revalidate is enabled
func (c *CacheConfig) IsStaleWhileRevalidateEnabled() bool {
	return c.StaleTime > 0
}

// FetchConfig defines how to fetch webpages, including HTTP client settings.
type FetchConfig struct {
	Timeout              time.Duration     `yaml:"timeout,omitempty"`
	UserAgent            string            `yaml:"user_agent,omitempty"`
	Headers              map[string]string `yaml:"headers,omitempty"`
	CheckFormats         []string          `yaml:"check_formats,omitempty"`
	URLRewrites          []URLRewrite      `yaml:"url_rewrites,omitempty"`
	RespectRobotsTxt     bool              `yaml:"respect_robots_txt,omitempty"`
	RobotsTxtCacheTTL    time.Duration     `yaml:"robots_txt_cache_ttl,omitempty"`
	FollowRedirects      bool              `yaml:"follow_redirects,omitempty"`
	MaxRedirects         int               `yaml:"max_redirects,omitempty"`
	EnableSSRFProtection bool              `yaml:"enable_ssrf_protection,omitempty"`
}

// GetHeaders returns the headers to use for a request
func (f *FetchConfig) GetHeaders() map[string]string {
	headers := make(map[string]string)
	if f.UserAgent != "" {
		headers["User-Agent"] = f.UserAgent
	} else {
		headers["User-Agent"] = DefaultUserAgent
	}
	maps.Copy(headers, f.Headers)
	return headers
}

// GetRobotsTxtCacheTTL returns the robots.txt cache TTL with a default of 24 hours
func (f *FetchConfig) GetRobotsTxtCacheTTL() time.Duration {
	if f.RobotsTxtCacheTTL > 0 {
		return f.RobotsTxtCacheTTL
	}
	return 24 * time.Hour
}

// ShouldFollowRedirects returns whether to follow redirects (default: true)
func (f *FetchConfig) ShouldFollowRedirects() bool {
	if f.MaxRedirects > 0 {
		return true
	}
	return f.FollowRedirects
}

// GetMaxRedirects returns the max number of redirects with a default of 10
func (f *FetchConfig) GetMaxRedirects() int {
	if f.MaxRedirects > 0 {
		return f.MaxRedirects
	}
	if !f.ShouldFollowRedirects() {
		return 0
	}
	return 10
}

// URLRewrite defines a URL transformation rule applied before fetching.
type URLRewrite struct {
	Type        string `yaml:"type"`
	Pattern     string `yaml:"pattern,omitempty"`
	Replacement string `yaml:"replacement,omitempty"`
}

// SiteConfig represents configuration overrides for URLs matching a specific pattern.
type SiteConfig struct {
	Pattern   string           `yaml:"pattern"`
	Cache     *CacheConfig     `yaml:"cache,omitempty"`
	Fetch     *FetchConfig     `yaml:"fetch,omitempty"`
	RateLimit *RateLimitConfig `yaml:"rate_limit,omitempty"`
	Retry     *RetryConfig     `yaml:"retry,omitempty"`
}

// RateLimitConfig defines rate limiting behavior to avoid overwhelming servers.
type RateLimitConfig struct {
	RequestsPerSecond float64       `yaml:"requests_per_second,omitempty"`
	Burst             int           `yaml:"burst,omitempty"`
	Delay             time.Duration `yaml:"delay,omitempty"`
	MaxConcurrent     int           `yaml:"max_concurrent,omitempty"`
	RespectRetryAfter bool          `yaml:"respect_retry_after,omitempty"`
}

// GetDelay returns the minimum delay between requests based on rate limits
func (r *RateLimitConfig) GetDelay() time.Duration {
	if r.Delay > 0 {
		return r.Delay
	}
	if r.RequestsPerSecond > 0 {
		return time.Duration(float64(time.Second) / r.RequestsPerSecond)
	}
	return 0
}

// IsEnabled returns true if any rate limiting is configured
func (r *RateLimitConfig) IsEnabled() bool {
	return r.RequestsPerSecond > 0 || r.Delay > 0 || r.MaxConcurrent > 0 || r.RespectRetryAfter
}

// GetMaxConcurrent returns the max concurrent requests (default unlimited)
func (r *RateLimitConfig) GetMaxConcurrent() int {
	if r.MaxConcurrent <= 0 {
		return 0
	}
	return r.MaxConcurrent
}

// RetryConfig defines retry and exponential backoff behavior for failed requests.
type RetryConfig struct {
	MaxRetries   int           `yaml:"max_retries,omitempty"`
	InitialDelay time.Duration `yaml:"initial_delay,omitempty"`
	MaxDelay     time.Duration `yaml:"max_delay,omitempty"`
	Multiplier   float64       `yaml:"multiplier,omitempty"`
	RetryOn      []int         `yaml:"retry_on,omitempty"`
}

// IsEnabled returns true if retries are configured
func (r *RetryConfig) IsEnabled() bool {
	return r.MaxRetries > 0
}

// GetMaxRetries returns the max retries with a default of 0 (no retries)
func (r *RetryConfig) GetMaxRetries() int {
	if r.MaxRetries < 0 {
		return 0
	}
	return r.MaxRetries
}

// GetInitialDelay returns the initial delay with a default of 1 second
func (r *RetryConfig) GetInitialDelay() time.Duration {
	if r.InitialDelay > 0 {
		return r.InitialDelay
	}
	return time.Second
}

// GetMaxDelay returns the max delay with a default of 30 seconds
func (r *RetryConfig) GetMaxDelay() time.Duration {
	if r.MaxDelay > 0 {
		return r.MaxDelay
	}
	return 30 * time.Second
}

// GetMultiplier returns the backoff multiplier with a default of 2.0
func (r *RetryConfig) GetMultiplier() float64 {
	if r.Multiplier > 0 {
		return r.Multiplier
	}
	return 2.0
}

// GetRetryOn returns the status codes to retry on with defaults [429, 500, 502, 503, 504]
func (r *RetryConfig) GetRetryOn() []int {
	if len(r.RetryOn) > 0 {
		return r.RetryOn
	}
	return []int{429, 500, 502, 503, 504}
}

// ShouldRetry returns true if the given status code should be retried
func (r *RetryConfig) ShouldRetry(statusCode int) bool {
	return slices.Contains(r.GetRetryOn(), statusCode)
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	return &cfg, nil
}

// Validate checks the configuration for errors and conflicts
func (c *Config) Validate() error {
	if err := c.validateRateLimit("default", c.Default.RateLimit); err != nil {
		return err
	}
	if err := c.validateRetry("default", c.Default.Retry); err != nil {
		return err
	}
	if err := c.validateFetch("default", c.Default.Fetch); err != nil {
		return err
	}

	for i, site := range c.Sites {
		if site.Pattern == "" {
			return fmt.Errorf("sites[%d]: pattern cannot be empty", i)
		}

		siteCtx := fmt.Sprintf("sites[%d](%s)", i, site.Pattern)

		if site.RateLimit != nil {
			if err := c.validateRateLimit(siteCtx, *site.RateLimit); err != nil {
				return err
			}
		}
		if site.Retry != nil {
			if err := c.validateRetry(siteCtx, *site.Retry); err != nil {
				return err
			}
		}
		if site.Fetch != nil {
			if err := c.validateFetch(siteCtx, *site.Fetch); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *Config) validateRateLimit(ctx string, rl RateLimitConfig) error {
	if rl.Delay > 0 && rl.RequestsPerSecond > 0 {
		return fmt.Errorf("%s.rate_limit: cannot specify both 'delay' and 'requests_per_second'", ctx)
	}

	if rl.Burst > 0 && rl.RequestsPerSecond == 0 && rl.Delay == 0 {
		return fmt.Errorf("%s.rate_limit: 'burst' requires either 'requests_per_second' or 'delay'", ctx)
	}

	if rl.MaxConcurrent < 0 {
		return fmt.Errorf("%s.rate_limit: 'max_concurrent' must be >= 0", ctx)
	}

	return nil
}

func (c *Config) validateRetry(ctx string, r RetryConfig) error {
	if r.Multiplier > 0 && r.Multiplier < 1.0 {
		return fmt.Errorf("%s.retry: 'multiplier' must be >= 1.0 (got %.2f)", ctx, r.Multiplier)
	}

	if r.MaxRetries < 0 {
		return fmt.Errorf("%s.retry: 'max_retries' must be >= 0", ctx)
	}

	if r.MaxDelay > 0 && r.InitialDelay > r.MaxDelay {
		return fmt.Errorf("%s.retry: 'initial_delay' (%s) cannot be greater than 'max_delay' (%s)",
			ctx, r.InitialDelay, r.MaxDelay)
	}

	for _, code := range r.RetryOn {
		if code < 100 || code > 599 {
			return fmt.Errorf("%s.retry: invalid HTTP status code %d in 'retry_on'", ctx, code)
		}
	}

	return nil
}

func (c *Config) validateFetch(ctx string, f FetchConfig) error {
	if f.Timeout < 0 {
		return fmt.Errorf("%s.fetch: 'timeout' must be >= 0", ctx)
	}

	if f.RobotsTxtCacheTTL < 0 {
		return fmt.Errorf("%s.fetch: 'robots_txt_cache_ttl' must be >= 0", ctx)
	}

	if f.MaxRedirects < 0 {
		return fmt.Errorf("%s.fetch: 'max_redirects' must be >= 0", ctx)
	}

	for i, rewrite := range f.URLRewrites {
		if rewrite.Pattern == "" {
			return fmt.Errorf("%s.fetch.url_rewrites[%d]: 'pattern' cannot be empty", ctx, i)
		}
		if rewrite.Type != "" && rewrite.Type != "regex" && rewrite.Type != "literal" {
			return fmt.Errorf("%s.fetch.url_rewrites[%d]: 'type' must be 'regex' or 'literal'", ctx, i)
		}
	}

	return nil
}

func matchPattern(urlStr, pattern string) bool {
	parsedURL, err := url.Parse(urlStr)
	if err != nil || parsedURL.Host == "" {
		return urlStr == pattern
	}

	host := parsedURL.Host
	path := parsedURL.Path

	if strings.HasPrefix(pattern, "*.") {
		if strings.Contains(pattern, "/") {
			return matchWildcardDomainAndPath(host, path, pattern)
		}
		return matchWildcardDomain(host, pattern[2:])
	}

	if strings.Contains(pattern, "/") {
		return matchHostAndPath(host, path, pattern)
	}

	if strings.Contains(pattern, "*") {
		return matchWildcardHost(host, pattern)
	}

	return host == pattern
}

func matchWildcardDomain(host, domain string) bool {
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func matchWildcardDomainAndPath(host, path, pattern string) bool {
	parts := strings.SplitN(pattern, "/", 2)
	if len(parts) != 2 {
		return false
	}

	domainPattern := parts[0]
	if len(domainPattern) < 2 {
		return false
	}

	pathPattern := "/" + parts[1]

	domain := domainPattern[2:]
	if !matchWildcardDomain(host, domain) {
		return false
	}

	return matchPathPattern(path, pathPattern)
}

func matchWildcardHost(host, pattern string) bool {
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		substring := strings.Trim(pattern, "*")
		return strings.Contains(host, substring)
	}

	if after, ok := strings.CutPrefix(pattern, "*"); ok {
		return strings.HasSuffix(host, after)
	}

	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(host, prefix)
	}

	return false
}

func matchHostAndPath(host, path, pattern string) bool {
	parts := strings.SplitN(pattern, "/", 2)
	hostPattern := parts[0]
	pathPattern := "/" + parts[1]

	if !matchHostPattern(host, hostPattern) {
		return false
	}

	return matchPathPattern(path, pathPattern)
}

func matchHostPattern(host, pattern string) bool {
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		substring := strings.Trim(pattern, "*")
		return strings.Contains(host, substring)
	}

	if after, ok := strings.CutPrefix(pattern, "*"); ok {
		return strings.HasSuffix(host, after)
	}

	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(host, prefix)
	}

	return host == pattern
}

func matchPathPattern(path, pattern string) bool {
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(path, prefix)
	}

	return path == pattern
}

func mergeCache(base, override CacheConfig) CacheConfig {
	result := base

	if override.TTL != 0 {
		result.TTL = override.TTL
	}

	if override.StaleTime != 0 {
		result.StaleTime = override.StaleTime
	}

	return result
}

func mergeFetch(base, override FetchConfig) FetchConfig {
	result := base

	if override.Timeout != 0 {
		result.Timeout = override.Timeout
	}

	if override.UserAgent != "" {
		result.UserAgent = override.UserAgent
	}

	if result.Headers == nil {
		result.Headers = make(map[string]string)
	}
	maps.Copy(result.Headers, override.Headers)

	if len(override.CheckFormats) > 0 {
		result.CheckFormats = override.CheckFormats
	}

	if len(override.URLRewrites) > 0 {
		result.URLRewrites = override.URLRewrites
	}

	result.RespectRobotsTxt = override.RespectRobotsTxt
	if override.RobotsTxtCacheTTL > 0 {
		result.RobotsTxtCacheTTL = override.RobotsTxtCacheTTL
	}

	result.FollowRedirects = override.FollowRedirects
	if override.MaxRedirects > 0 {
		result.MaxRedirects = override.MaxRedirects
	}

	if override.EnableSSRFProtection {
		result.EnableSSRFProtection = true
	}

	return result
}

func mergeRateLimit(base, override RateLimitConfig) RateLimitConfig {
	result := base

	if override.RequestsPerSecond > 0 {
		result.RequestsPerSecond = override.RequestsPerSecond
	}

	if override.Burst > 0 {
		result.Burst = override.Burst
	}

	if override.Delay > 0 {
		result.Delay = override.Delay
	}

	if override.MaxConcurrent > 0 {
		result.MaxConcurrent = override.MaxConcurrent
	}

	result.RespectRetryAfter = override.RespectRetryAfter

	return result
}

func mergeRetry(base, override RetryConfig) RetryConfig {
	result := base

	if override.MaxRetries > 0 {
		result.MaxRetries = override.MaxRetries
	}

	if override.InitialDelay > 0 {
		result.InitialDelay = override.InitialDelay
	}

	if override.MaxDelay > 0 {
		result.MaxDelay = override.MaxDelay
	}

	if override.Multiplier > 0 {
		result.Multiplier = override.Multiplier
	}

	if len(override.RetryOn) > 0 {
		result.RetryOn = override.RetryOn
	}

	return result
}
