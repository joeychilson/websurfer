package fetcher

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/joeychilson/websurfer/config"
)

// Response represents the fetched webpage response.
type Response struct {
	URL        string
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// Fetcher fetches webpages using the provided configuration.
type Fetcher struct {
	config           config.FetchConfig
	client           *http.Client
	compiledRewrites []*compiledRewrite
}

// compiledRewrite holds a pre-compiled regex and its replacement.
type compiledRewrite struct {
	regex       *regexp.Regexp
	replacement string
}

// ssrfProtectedTransport wraps http.DefaultTransport with SSRF protection.
type ssrfProtectedTransport struct {
	base http.RoundTripper
}

// RoundTrip validates that the destination IP is not private/internal before making the request.
func (t *ssrfProtectedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	host, _, err := net.SplitHostPort(req.URL.Host)
	if err != nil {
		host = req.URL.Host
	}

	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() {
			return nil, fmt.Errorf("requests to private IP addresses are not allowed: %s", host)
		}
	} else {
		ips, err := net.LookupIP(host)
		if err != nil {
			return t.base.RoundTrip(req)
		}

		for _, resolvedIP := range ips {
			if resolvedIP.IsLoopback() || resolvedIP.IsPrivate() {
				return nil, fmt.Errorf("url resolves to private IP address: %s -> %s", host, resolvedIP.String())
			}
		}
	}

	return t.base.RoundTrip(req)
}

// New creates a new Fetcher with the given configuration.
func New(cfg config.FetchConfig) *Fetcher {
	maxRedirects := cfg.GetMaxRedirects()

	var transport http.RoundTripper = http.DefaultTransport
	if cfg.EnableSSRFProtection {
		transport = &ssrfProtectedTransport{
			base: http.DefaultTransport,
		}
	}

	client := &http.Client{
		Timeout:   cfg.Timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if maxRedirects == 0 {
				return http.ErrUseLastResponse
			}
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			return nil
		},
	}

	var compiledRewrites []*compiledRewrite
	for _, rewrite := range cfg.URLRewrites {
		if rewrite.Type == "regex" {
			re, err := regexp.Compile(rewrite.Pattern)
			if err == nil {
				compiledRewrites = append(compiledRewrites, &compiledRewrite{
					regex:       re,
					replacement: rewrite.Replacement,
				})
			}
		}
	}

	return &Fetcher{
		config:           cfg,
		client:           client,
		compiledRewrites: compiledRewrites,
	}
}

// Fetch retrieves the content at the given URL.
// It applies URL rewrites, tries alternative formats, and returns the response.
func (f *Fetcher) Fetch(ctx context.Context, urlStr string) (*Response, error) {
	urlStr = f.applyRewrites(urlStr)

	urls := f.buildURLsToTry(urlStr)

	var lastErr error
	var lastResp *Response

	for _, tryURL := range urls {
		resp, err := f.fetchURL(ctx, tryURL)
		if err != nil {
			lastErr = err
			continue
		}

		if f.isSuccessfulResponse(resp.StatusCode) {
			return resp, nil
		}

		lastResp = resp
		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	if lastResp != nil {
		return lastResp, fmt.Errorf("failed to fetch %s: %w", urlStr, lastErr)
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", urlStr, lastErr)
	}

	return nil, fmt.Errorf("no URLs succeeded for %s", urlStr)
}

// SetTimeout updates the client timeout. Useful for testing.
func (f *Fetcher) SetTimeout(timeout time.Duration) {
	f.client.Timeout = timeout
}

// GetHTTPClient returns the underlying HTTP client.
func (f *Fetcher) GetHTTPClient() *http.Client {
	return f.client
}

// fetchURL performs the actual HTTP request for a single URL.
func (f *Fetcher) fetchURL(ctx context.Context, urlStr string) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, value := range f.config.GetHeaders() {
		req.Header.Set(key, value)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return &Response{
		URL:        resp.Request.URL.String(),
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       body,
	}, nil
}

// buildURLsToTry creates a list of URLs to attempt based on CheckFormats.
// If CheckFormats is configured, it tries alternative content paths first,
// then falls back to the original URL.
func (f *Fetcher) buildURLsToTry(urlStr string) []string {
	if len(f.config.CheckFormats) == 0 {
		return []string{urlStr}
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return []string{urlStr}
	}

	urls := []string{}
	for _, format := range f.config.CheckFormats {
		tryURL := f.applyFormat(parsedURL, format)
		if tryURL != urlStr {
			urls = append(urls, tryURL)
		}
	}

	urls = append(urls, urlStr)

	return urls
}

// applyFormat applies a format transformation to a URL.
// Formats can be:
// - Absolute paths like "/llms.txt" (replaces the path)
// - Extensions like ".md" (replaces or appends the extension)
func (f *Fetcher) applyFormat(parsedURL *url.URL, format string) string {
	newURL := *parsedURL

	if strings.HasPrefix(format, "/") {
		newURL.Path = format
		return newURL.String()
	}

	if strings.HasPrefix(format, ".") {
		ext := format
		path := parsedURL.Path

		if idx := strings.LastIndex(path, "."); idx != -1 {
			lastSlash := strings.LastIndex(path, "/")
			if lastSlash == -1 || idx > lastSlash {
				path = path[:idx]
			}
		}

		newURL.Path = path + ext
		return newURL.String()
	}

	return parsedURL.String()
}

// applyRewrites applies all configured URL rewrites to the given URL.
func (f *Fetcher) applyRewrites(urlStr string) string {
	result := urlStr

	for _, compiled := range f.compiledRewrites {
		result = compiled.regex.ReplaceAllString(result, compiled.replacement)
	}

	for _, rewrite := range f.config.URLRewrites {
		if rewrite.Pattern == "" || rewrite.Type == "regex" {
			continue
		}
		result = strings.ReplaceAll(result, rewrite.Pattern, rewrite.Replacement)
	}

	return result
}

// isSuccessfulResponse determines if a status code represents a successful fetch.
// 2xx codes are always successful. 3xx codes are successful if redirects are disabled.
func (f *Fetcher) isSuccessfulResponse(statusCode int) bool {
	if statusCode >= 200 && statusCode < 300 {
		return true
	}

	if statusCode >= 300 && statusCode < 400 && f.config.GetMaxRedirects() == 0 {
		return true
	}

	return false
}
