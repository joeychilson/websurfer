package robots

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
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

	if parsedURL.Host == "" {
		return false, fmt.Errorf("url has no host: %s", urlStr)
	}

	robotsURL := fmt.Sprintf("%s://%s/robots.txt", parsedURL.Scheme, parsedURL.Host)
	rules, err := c.getRules(ctx, robotsURL, parsedURL.Host)
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
func parseRobotsTxt(body io.Reader, userAgent string) (*Rules, time.Duration, error) {
	scanner := bufio.NewScanner(body)
	parser := newRobotsParser(userAgent)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		directive, value := parseRobotsLine(line)
		if directive == "" {
			continue
		}

		parser.processDirective(directive, value)
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to read robots.txt: %w", err)
	}

	return parser.getResult()
}

// robotsParser manages the state during robots.txt parsing.
type robotsParser struct {
	userAgent          string
	specificRules      *Rules
	wildcardRules      *Rules
	specificCrawlDelay time.Duration
	wildcardCrawlDelay time.Duration
	matchesSpecific    bool
	matchesWildcard    bool
	foundSpecificMatch bool
}

// newRobotsParser creates a new robots.txt parser.
func newRobotsParser(userAgent string) *robotsParser {
	return &robotsParser{
		userAgent: userAgent,
		specificRules: &Rules{
			UserAgent: userAgent,
			Disallows: []string{},
			Allows:    []string{},
		},
		wildcardRules: &Rules{
			UserAgent: userAgent,
			Disallows: []string{},
			Allows:    []string{},
		},
	}
}

// parseRobotsLine extracts the directive and value from a robots.txt line.
func parseRobotsLine(line string) (directive, value string) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return strings.TrimSpace(strings.ToLower(parts[0])), strings.TrimSpace(parts[1])
}

// processDirective handles a single robots.txt directive.
func (p *robotsParser) processDirective(directive, value string) {
	switch directive {
	case "user-agent":
		p.handleUserAgent(value)
	case "disallow":
		p.handleDisallow(value)
	case "allow":
		p.handleAllow(value)
	case "crawl-delay":
		p.handleCrawlDelay(value)
	}
}

// handleUserAgent processes a User-agent directive.
func (p *robotsParser) handleUserAgent(value string) {
	currentUserAgent := strings.ToLower(value)
	if currentUserAgent == "*" {
		p.matchesWildcard = true
		p.matchesSpecific = false
	} else if strings.Contains(strings.ToLower(p.userAgent), currentUserAgent) {
		p.matchesSpecific = true
		p.matchesWildcard = false
		p.foundSpecificMatch = true
	} else {
		p.matchesSpecific = false
		p.matchesWildcard = false
	}
}

// handleDisallow processes a Disallow directive.
func (p *robotsParser) handleDisallow(value string) {
	if value == "" {
		return
	}
	if p.matchesSpecific {
		p.specificRules.Disallows = append(p.specificRules.Disallows, value)
	} else if p.matchesWildcard {
		p.wildcardRules.Disallows = append(p.wildcardRules.Disallows, value)
	}
}

// handleAllow processes an Allow directive.
func (p *robotsParser) handleAllow(value string) {
	if value == "" {
		return
	}
	if p.matchesSpecific {
		p.specificRules.Allows = append(p.specificRules.Allows, value)
	} else if p.matchesWildcard {
		p.wildcardRules.Allows = append(p.wildcardRules.Allows, value)
	}
}

// handleCrawlDelay processes a Crawl-delay directive.
func (p *robotsParser) handleCrawlDelay(value string) {
	seconds, err := time.ParseDuration(value + "s")
	if err != nil {
		return
	}
	if p.matchesSpecific && p.specificCrawlDelay == 0 {
		p.specificCrawlDelay = seconds
	} else if p.matchesWildcard && p.wildcardCrawlDelay == 0 {
		p.wildcardCrawlDelay = seconds
	}
}

// getResult returns the final parsed rules and crawl delay.
func (p *robotsParser) getResult() (*Rules, time.Duration, error) {
	if p.foundSpecificMatch {
		return p.specificRules, p.specificCrawlDelay, nil
	}
	return p.wildcardRules, p.wildcardCrawlDelay, nil
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
