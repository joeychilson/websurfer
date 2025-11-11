package robots

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	urlutil "github.com/joeychilson/websurfer/url"
)

// Checker verifies if URLs can be crawled according to robots.txt rules.
type Checker struct {
	userAgent string
	client    *http.Client
	cache     sync.Map
	cacheTTL  time.Duration
}

// cachedRobots holds parsed robots.txt data with expiration.
type cachedRobots struct {
	Rules      *Rules
	CrawlDelay time.Duration
	ExpiresAt  time.Time
}

// Rules represents parsed robots.txt rules for a specific user agent.
type Rules struct {
	UserAgent string   `json:"user_agent"`
	Disallows []string `json:"disallows"`
	Allows    []string `json:"allows"`
}

// New creates a new robots.txt checker with the given user agent and cache TTL.
func New(userAgent string, cacheTTL time.Duration, client *http.Client) *Checker {
	if client == nil {
		client = &http.Client{
			Timeout: 10 * time.Second,
		}
	}

	return &Checker{
		userAgent: userAgent,
		client:    client,
		cacheTTL:  cacheTTL,
	}
}

// IsAllowed checks if the given URL can be crawled according to robots.txt rules.
func (c *Checker) IsAllowed(ctx context.Context, urlStr string) (bool, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false, fmt.Errorf("invalid url: %w", err)
	}

	host, err := urlutil.ExtractHost(urlStr)
	if err != nil {
		return false, err
	}

	robotsURL := fmt.Sprintf("%s://%s/robots.txt", parsedURL.Scheme, host)
	rules, err := c.getRules(ctx, robotsURL, host)
	if err != nil {
		return true, nil
	}

	if rules == nil {
		return true, nil
	}

	path := parsedURL.Path
	if parsedURL.RawQuery != "" {
		path = path + "?" + parsedURL.RawQuery
	}

	return rules.isAllowed(path), nil
}

// GetCrawlDelay returns the crawl delay for a domain, or 0 if none specified.
func (c *Checker) GetCrawlDelay(ctx context.Context, urlStr string) (time.Duration, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return 0, fmt.Errorf("invalid url: %w", err)
	}

	if parsedURL.Host == "" {
		return 0, fmt.Errorf("url has no host: %s", urlStr)
	}

	robotsURL := fmt.Sprintf("%s://%s/robots.txt", parsedURL.Scheme, parsedURL.Host)
	cached, err := c.getCachedRobots(parsedURL.Host)
	if err == nil && cached != nil {
		return cached.CrawlDelay, nil
	}

	_, err = c.getRules(ctx, robotsURL, parsedURL.Host)
	if err != nil {
		return 0, nil
	}

	cached, err = c.getCachedRobots(parsedURL.Host)
	if err == nil && cached != nil {
		return cached.CrawlDelay, nil
	}

	return 0, nil
}

// getRules retrieves robots.txt rules for a domain, using cache if valid.
func (c *Checker) getRules(ctx context.Context, robotsURL, host string) (*Rules, error) {
	cached, err := c.getCachedRobots(host)
	if err == nil && cached != nil && cached.Rules != nil {
		return cached.Rules, nil
	}

	rules, crawlDelay, err := c.fetchAndParse(ctx, robotsURL)
	if err != nil {
		return nil, err
	}

	c.cache.Store(host, &cachedRobots{
		Rules:      rules,
		CrawlDelay: crawlDelay,
		ExpiresAt:  time.Now().Add(c.cacheTTL),
	})

	return rules, nil
}

// getCachedRobots retrieves cached robots.txt data from the in-memory cache.
func (c *Checker) getCachedRobots(host string) (*cachedRobots, error) {
	val, ok := c.cache.Load(host)
	if !ok {
		return nil, nil
	}

	cached := val.(*cachedRobots)

	if time.Now().After(cached.ExpiresAt) {
		c.cache.Delete(host)
		return nil, nil
	}

	return cached, nil
}

// fetchAndParse fetches and parses robots.txt from the given URL.
func (c *Checker) fetchAndParse(ctx context.Context, robotsURL string) (*Rules, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to fetch robots.txt: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, 0, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return parseRobotsTxt(resp.Body, c.userAgent)
}

// parseRobotsTxt parses robots.txt content for a specific user agent.
func parseRobotsTxt(body interface{ Read([]byte) (int, error) }, userAgent string) (*Rules, time.Duration, error) {
	scanner := bufio.NewScanner(body)

	specificRules := &Rules{
		UserAgent: userAgent,
		Disallows: []string{},
		Allows:    []string{},
	}
	wildcardRules := &Rules{
		UserAgent: userAgent,
		Disallows: []string{},
		Allows:    []string{},
	}

	var specificCrawlDelay time.Duration
	var wildcardCrawlDelay time.Duration
	var currentUserAgent string
	var matchesSpecific bool
	var matchesWildcard bool
	var foundSpecificMatch bool

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		directive := strings.TrimSpace(strings.ToLower(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch directive {
		case "user-agent":
			currentUserAgent = strings.ToLower(value)
			if currentUserAgent == "*" {
				matchesWildcard = true
				matchesSpecific = false
			} else if strings.Contains(strings.ToLower(userAgent), currentUserAgent) {
				matchesSpecific = true
				matchesWildcard = false
				foundSpecificMatch = true
			} else {
				matchesSpecific = false
				matchesWildcard = false
			}

		case "disallow":
			if value != "" {
				if matchesSpecific {
					specificRules.Disallows = append(specificRules.Disallows, value)
				} else if matchesWildcard {
					wildcardRules.Disallows = append(wildcardRules.Disallows, value)
				}
			}

		case "allow":
			if value != "" {
				if matchesSpecific {
					specificRules.Allows = append(specificRules.Allows, value)
				} else if matchesWildcard {
					wildcardRules.Allows = append(wildcardRules.Allows, value)
				}
			}

		case "crawl-delay":
			if seconds, err := time.ParseDuration(value + "s"); err == nil {
				if matchesSpecific && specificCrawlDelay == 0 {
					specificCrawlDelay = seconds
				} else if matchesWildcard && wildcardCrawlDelay == 0 {
					wildcardCrawlDelay = seconds
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to read robots.txt: %w", err)
	}

	if foundSpecificMatch {
		return specificRules, specificCrawlDelay, nil
	}

	return wildcardRules, wildcardCrawlDelay, nil
}

// isAllowed checks if a path is allowed according to the rules.
func (r *Rules) isAllowed(path string) bool {
	if path == "" {
		path = "/"
	}

	var longestMatch string
	var isAllow bool

	for _, allow := range r.Allows {
		if matchesPath(path, allow) && len(allow) > len(longestMatch) {
			longestMatch = allow
			isAllow = true
		}
	}

	for _, disallow := range r.Disallows {
		if matchesPath(path, disallow) && len(disallow) > len(longestMatch) {
			longestMatch = disallow
			isAllow = false
		}
	}

	if longestMatch == "" {
		return true
	}

	return isAllow
}

// matchesPath checks if a path matches a robots.txt pattern.
func matchesPath(path, pattern string) bool {
	if pattern == "/" {
		return true
	}

	if strings.HasSuffix(pattern, "$") {
		pattern = strings.TrimSuffix(pattern, "$")
		return path == pattern
	}

	if strings.Contains(pattern, "*") {
		return wildcardMatch(path, pattern)
	}

	return strings.HasPrefix(path, pattern)
}

// wildcardMatch checks if a path matches a pattern with wildcards.
func wildcardMatch(path, pattern string) bool {
	parts := strings.Split(pattern, "*")

	if len(parts) == 0 {
		return false
	}

	if !strings.HasPrefix(path, parts[0]) {
		return false
	}

	currentPos := len(parts[0])

	for i := 1; i < len(parts)-1; i++ {
		if parts[i] == "" {
			continue
		}

		idx := strings.Index(path[currentPos:], parts[i])
		if idx == -1 {
			return false
		}
		currentPos += idx + len(parts[i])
	}

	if len(parts) > 1 && parts[len(parts)-1] != "" {
		return strings.HasSuffix(path, parts[len(parts)-1])
	}

	return true
}
