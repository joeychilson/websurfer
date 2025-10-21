package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/joeychilson/websurfer/client"
	"github.com/joeychilson/websurfer/content"
	"github.com/joeychilson/websurfer/logger"
	"github.com/joeychilson/websurfer/parser/html"
	"github.com/joeychilson/websurfer/parser/sitemap"
)

// FetchRequest represents a request to fetch and process a URL.
type FetchRequest struct {
	URL       string                `json:"url"`
	MaxTokens int                   `json:"max_tokens,omitempty"`
	Range     *content.RangeOptions `json:"range,omitempty"`
}

// Metadata contains metadata about the fetched content.
type Metadata struct {
	URL             string `json:"url"`
	StatusCode      int    `json:"status_code"`
	ContentType     string `json:"content_type"`
	Language        string `json:"language,omitempty"`
	Title           string `json:"title,omitempty"`
	Description     string `json:"description,omitempty"`
	EstimatedTokens int    `json:"estimated_tokens"`
	LastModified    string `json:"last_modified,omitempty"`
	CacheState      string `json:"cache_state,omitempty"`
	CachedAt        string `json:"cached_at,omitempty"`
}

// FetchResponse represents the response from a fetch request.
type FetchResponse struct {
	Metadata  Metadata              `json:"metadata"`
	Content   string                `json:"content"`
	NextRange *content.RangeOptions `json:"next_range,omitempty"`
}

// MapRequest represents a request to map/discover URLs from a website.
type MapRequest struct {
	URL        string `json:"url"`
	MaxURLs    int    `json:"max_urls,omitempty"`
	SameDomain bool   `json:"same_domain,omitempty"`
	Depth      int    `json:"depth,omitempty"`
}

// PageInfo contains information about a discovered page.
type PageInfo struct {
	URL         string `json:"url"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	NoIndex     bool   `json:"noindex,omitempty"`
}

// MapResponse represents the response from a map request.
type MapResponse struct {
	BaseURL   string     `json:"base_url"`
	Source    string     `json:"source"`
	Pages     []PageInfo `json:"pages"`
	Count     int        `json:"count"`
	Truncated bool       `json:"truncated"`
}

// ErrorResponse represents an error.
type ErrorResponse struct {
	Error      string            `json:"error"`
	StatusCode int               `json:"status_code"`
	Details    map[string]string `json:"details,omitempty"`
}

// Handler contains the HTTP handlers for the API.
type Handler struct {
	client *client.Client
	logger logger.Logger
}

// NewHandler creates a new Handler.
func NewHandler(c *client.Client, log logger.Logger) *Handler {
	if log == nil {
		log = logger.Noop()
	}
	return &Handler{
		client: c,
		logger: log,
	}
}

// HandleFetch handles POST /fetch requests.
func (h *Handler) HandleFetch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req FetchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("failed to decode request", "error", err)
		h.sendError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := h.validateRequest(&req); err != nil {
		h.logger.Error("invalid request", "error", err)
		h.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.logger.Info("fetch request", "url", req.URL, "max_tokens", req.MaxTokens)

	resp, err := h.processFetch(ctx, &req)
	if err != nil {
		h.logger.Error("fetch failed", "url", req.URL, "error", err)
		h.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.logger.Info("fetch completed",
		"url", resp.Metadata.URL,
		"status_code", resp.Metadata.StatusCode)

	h.sendJSON(w, resp, http.StatusOK)
}

// HandleMap handles POST /map requests.
func (h *Handler) HandleMap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req MapRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("failed to decode map request", "error", err)
		h.sendError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := h.validateMapRequest(&req); err != nil {
		h.logger.Error("invalid map request", "error", err)
		h.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.logger.Info("map request", "url", req.URL, "max_urls", req.MaxURLs, "same_domain", req.SameDomain)

	resp, err := h.processMap(ctx, &req)
	if err != nil {
		h.logger.Error("map failed", "url", req.URL, "error", err)
		h.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.logger.Info("map completed", "url", req.URL, "pages_found", resp.Count, "truncated", resp.Truncated)

	h.sendJSON(w, resp, http.StatusOK)
}

// HandleHealth handles GET /health requests.
func (h *Handler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	}
	h.sendJSON(w, health, http.StatusOK)
}

// processFetch handles the fetch request processing logic.
func (h *Handler) processFetch(ctx context.Context, req *FetchRequest) (*FetchResponse, error) {
	fetched, err := h.client.Fetch(ctx, req.URL)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %w", err)
	}

	bodyText := string(fetched.Body)
	contentType := firstHeader(fetched.Headers, "Content-Type")
	lastModified := firstHeader(fetched.Headers, "Last-Modified")

	language := ""
	if strings.Contains(strings.ToLower(contentType), "html") {
		language = extractLanguage(bodyText)
	}

	workingText := bodyText
	if req.Range != nil {
		extracted, err := content.ExtractRange(workingText, req.Range)
		if err != nil {
			return nil, fmt.Errorf("range extraction failed: %w", err)
		}
		workingText = extracted
	}

	if req.MaxTokens > 0 {
		truncation := content.Truncate(workingText, contentType, req.MaxTokens)

		metadata := buildFetchMetadata(fetched, contentType, language, lastModified, truncation.ReturnedTokens)
		response := &FetchResponse{
			Metadata:  metadata,
			Content:   truncation.Content,
			NextRange: nextRangeForTruncation(truncation),
		}

		return response, nil
	}

	estimatedTokens := content.EstimateTokens(workingText, contentType)

	metadata := buildFetchMetadata(fetched, contentType, language, lastModified, estimatedTokens)

	return &FetchResponse{
		Metadata: metadata,
		Content:  workingText,
	}, nil
}

func buildFetchMetadata(resp *client.Response, contentType, language, lastModified string, tokens int) Metadata {
	metadata := Metadata{
		URL:             resp.URL,
		StatusCode:      resp.StatusCode,
		ContentType:     contentType,
		Language:        language,
		Title:           resp.Title,
		Description:     resp.Description,
		EstimatedTokens: tokens,
		LastModified:    lastModified,
		CacheState:      resp.CacheState,
	}

	if !resp.CachedAt.IsZero() {
		metadata.CachedAt = resp.CachedAt.Format(time.RFC3339Nano)
	}

	return metadata
}

func nextRangeForTruncation(result *content.TruncateResult) *content.RangeOptions {
	if result == nil || !result.Truncated {
		return nil
	}

	nextStart := result.ReturnedChars
	if nextStart >= result.TotalChars {
		return nil
	}

	nextEnd := min(result.TotalChars, nextStart+result.ReturnedChars)
	return &content.RangeOptions{
		Type:  "chars",
		Start: nextStart,
		End:   nextEnd,
	}
}

func firstHeader(headers map[string][]string, key string) string {
	if values, ok := headers[key]; ok && len(values) > 0 {
		return values[0]
	}
	return ""
}

// validateRequest validates the fetch request.
func (h *Handler) validateRequest(req *FetchRequest) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}

	if _, err := h.parseAndValidateExternalURL(req.URL); err != nil {
		return err
	}

	if req.MaxTokens < 0 {
		return fmt.Errorf("max_tokens must be non-negative")
	}

	if req.Range != nil {
		if req.Range.Type != "lines" && req.Range.Type != "chars" {
			return fmt.Errorf("range type must be 'lines' or 'chars'")
		}
		if req.Range.Start < 0 || req.Range.End < 0 {
			return fmt.Errorf("range start and end must be non-negative")
		}
		if req.Range.Start >= req.Range.End {
			return fmt.Errorf("range start must be less than end")
		}
	}

	return nil
}

// fetchSitemapRecursive recursively fetches a sitemap and all child sitemaps it references.
// depth is the current recursion depth, maxDepth is the maximum allowed depth.
func (h *Handler) fetchSitemapRecursive(ctx context.Context, sitemapURL string, depth int, maxDepth int) []string {
	if depth >= maxDepth {
		h.logger.Warn("max sitemap recursion depth reached", "url", sitemapURL, "depth", depth)
		return nil
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	fetched, err := h.client.Fetch(fetchCtx, sitemapURL)
	if err != nil {
		h.logger.Debug("failed to fetch sitemap", "url", sitemapURL, "error", err)
		return nil
	}

	if fetched.StatusCode != 200 {
		h.logger.Debug("sitemap returned non-200 status", "url", sitemapURL, "status", fetched.StatusCode)
		return nil
	}

	result, err := sitemap.Parse(fetched.Body)
	if err != nil {
		h.logger.Debug("failed to parse sitemap", "url", sitemapURL, "error", err)
		return nil
	}

	if result == nil {
		return nil
	}

	pageURLs := make([]string, 0)

	if result.IsSitemapIndex {
		h.logger.Debug("found nested sitemap index", "url", sitemapURL, "child_count", len(result.ChildMaps))

		resultCh := make(chan []string, len(result.ChildMaps))
		semaphore := make(chan struct{}, 5)

		for _, childURL := range result.ChildMaps {
			go func(url string) {
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				childURLs := h.fetchSitemapRecursive(ctx, url, depth+1, maxDepth)
				select {
				case resultCh <- childURLs:
				case <-ctx.Done():
				}
			}(childURL)
		}

		for i := 0; i < len(result.ChildMaps); i++ {
			select {
			case urls := <-resultCh:
				pageURLs = append(pageURLs, urls...)
			case <-ctx.Done():
				h.logger.Warn("sitemap index fetch interrupted", "url", sitemapURL, "received", i, "total", len(result.ChildMaps))
				return pageURLs
			}
		}
		return pageURLs
	}

	for _, u := range result.URLs {
		if sitemap.IsSitemapURL(u) {
			normalized := sitemap.NormalizeSitemapURL(u)
			h.logger.Debug("found nested child sitemap", "url", u, "normalized", normalized, "depth", depth)
			childURLs := h.fetchSitemapRecursive(ctx, normalized, depth+1, maxDepth)
			pageURLs = append(pageURLs, childURLs...)
		} else {
			pageURLs = append(pageURLs, u)
		}
	}

	h.logger.Debug("fetched sitemap", "url", sitemapURL, "page_urls", len(pageURLs), "depth", depth)
	return pageURLs
}

// processMap handles the map request processing logic.
func (h *Handler) processMap(ctx context.Context, req *MapRequest) (*MapResponse, error) {
	var links []string
	var source string

	h.logger.Debug("checking robots.txt for sitemap directives", "url", req.URL)
	robotsSitemaps, err := h.client.GetSitemapsFromRobotsTxt(ctx, req.URL)
	if err == nil && len(robotsSitemaps) > 0 {
		h.logger.Info("found sitemaps in robots.txt", "count", len(robotsSitemaps), "sitemaps", robotsSitemaps)
		allURLs := make([]string, 0)
		for _, sitemapURL := range robotsSitemaps {
			childURLs := h.fetchSitemapRecursive(ctx, sitemapURL, 0, 3)
			allURLs = append(allURLs, childURLs...)
		}
		if len(allURLs) > 0 {
			links = allURLs
			source = "sitemap"
			h.logger.Info("extracted URLs from robots.txt sitemaps", "total_urls", len(links))
		}
	}

	if source == "" {
		parsedURL, _ := url.Parse(req.URL)
		sitemapURL := fmt.Sprintf("%s://%s/sitemap.xml", parsedURL.Scheme, parsedURL.Host)

		h.logger.Debug("attempting to fetch sitemap", "url", sitemapURL)
		sitemapFetched, err := h.client.Fetch(ctx, sitemapURL)

		if err == nil && sitemapFetched.StatusCode == 200 {
			contentType := ""
			if ct, ok := sitemapFetched.Headers["Content-Type"]; ok && len(ct) > 0 {
				contentType = ct[0]
			}

			if strings.Contains(strings.ToLower(contentType), "xml") {
				h.logger.Debug("found sitemap, parsing", "url", sitemapURL)
				result, err := sitemap.Parse(sitemapFetched.Body)
				if err == nil && result != nil {
					if result.IsSitemapIndex {
						h.logger.Info("found sitemap index, fetching child sitemaps", "count", len(result.ChildMaps))
						allURLs := make([]string, 0)
						for _, childURL := range result.ChildMaps {
							childURLs := h.fetchSitemapRecursive(ctx, childURL, 0, 3)
							allURLs = append(allURLs, childURLs...)
						}
						if len(allURLs) > 0 {
							links = allURLs
							source = "sitemap"
							h.logger.Info("extracted URLs from sitemap index", "total_urls", len(links))
						}
					} else if len(result.URLs) > 0 {
						pageURLs := make([]string, 0)
						childSitemapURLs := make([]string, 0)

						for _, u := range result.URLs {
							if sitemap.IsSitemapURL(u) {
								normalized := sitemap.NormalizeSitemapURL(u)
								childSitemapURLs = append(childSitemapURLs, normalized)
								h.logger.Debug("found child sitemap URL in regular sitemap", "url", u, "normalized", normalized)
							} else {
								pageURLs = append(pageURLs, u)
							}
						}

						if len(childSitemapURLs) > 0 {
							h.logger.Info("found child sitemap URLs, fetching recursively", "count", len(childSitemapURLs))
							for _, childURL := range childSitemapURLs {
								childURLs := h.fetchSitemapRecursive(ctx, childURL, 0, 3)
								pageURLs = append(pageURLs, childURLs...)
							}
							h.logger.Info("fetched all child sitemaps", "total_page_urls", len(pageURLs))
						}

						if len(pageURLs) > 0 {
							links = pageURLs
							source = "sitemap"
							h.logger.Info("extracted URLs from sitemap", "url", sitemapURL, "count", len(links))
						}
					}
				}
			}
		}
	}

	if source == "" {
		h.logger.Debug("sitemap not found or failed, falling back to HTML link extraction")

		fetched, err := h.client.Fetch(ctx, req.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch base URL: %w", err)
		}

		contentType := ""
		if ct, ok := fetched.Headers["Content-Type"]; ok && len(ct) > 0 {
			contentType = ct[0]
		}
		if !strings.Contains(strings.ToLower(contentType), "html") {
			return nil, fmt.Errorf("URL does not return HTML content (got: %s)", contentType)
		}

		links, err = html.ExtractLinks(string(fetched.Body), req.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to extract links: %w", err)
		}
		source = "html_links"
	}

	if req.Depth > 0 && len(links) > 0 {
		h.logger.Info("crawling discovered pages", "depth", req.Depth, "initial_urls", len(links))

		maxURLs := req.MaxURLs
		if maxURLs == 0 {
			maxURLs = 100
		}
		if maxURLs > 1000 {
			maxURLs = 1000
		}

		allLinks := make(map[string]bool)
		for _, link := range links {
			allLinks[link] = true
		}

		type crawlResult struct {
			pageURL string
			links   []string
		}

		resultCh := make(chan crawlResult, len(links))
		semaphore := make(chan struct{}, 10)

		crawlCount := 0
		for _, pageURL := range links {
			if len(allLinks) >= maxURLs {
				h.logger.Info("reached max_urls, stopping crawl early", "urls_found", len(allLinks))
				break
			}

			crawlCount++
			go func(url string) {
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				pageCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()

				pageFetched, err := h.client.Fetch(pageCtx, url)
				if err == nil && pageFetched.StatusCode == 200 {
					pageLinks, err := html.ExtractLinks(string(pageFetched.Body), url)
					if err == nil {
						select {
						case resultCh <- crawlResult{pageURL: url, links: pageLinks}:
						case <-ctx.Done():
						}
					}
				}
			}(pageURL)
		}

	crawlLoop:
		for i := 0; i < crawlCount; i++ {
			select {
			case result := <-resultCh:
				for _, link := range result.links {
					if len(allLinks) < maxURLs {
						allLinks[link] = true
					}
				}
			case <-ctx.Done():
				h.logger.Info("crawl interrupted by context", "received", i, "expected", crawlCount)
				break crawlLoop
			}
		}

		links = make([]string, 0, len(allLinks))
		for link := range allLinks {
			links = append(links, link)
		}
		h.logger.Info("crawl complete", "total_urls", len(links))
	}

	if req.SameDomain {
		baseURL, _ := url.Parse(req.URL)
		filtered := make([]string, 0, len(links))
		for _, link := range links {
			linkURL, err := url.Parse(link)
			if err == nil && linkURL.Host == baseURL.Host {
				filtered = append(filtered, link)
			}
		}
		links = filtered
	}

	maxURLs := req.MaxURLs
	if maxURLs == 0 {
		maxURLs = 100
	}
	if maxURLs > 1000 {
		maxURLs = 1000
	}

	truncated := false
	if len(links) > maxURLs {
		links = links[:maxURLs]
		truncated = true
	}

	type result struct {
		index int
		page  PageInfo
	}

	resultCh := make(chan result, len(links))
	semaphore := make(chan struct{}, 10)

	for i, link := range links {
		go func(idx int, url string) {
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			page := PageInfo{URL: url}

			pageCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			pageFetched, err := h.client.Fetch(pageCtx, url)
			if err == nil && pageFetched != nil {
				page.Title = pageFetched.Title
				page.Description = pageFetched.Description
				page.NoIndex = hasNoIndex(pageFetched.Body, pageFetched.Headers)
			} else {
				h.logger.Debug("failed to fetch metadata for link", "url", url, "error", err)
			}

			select {
			case resultCh <- result{index: idx, page: page}:
			case <-ctx.Done():
				return
			}
		}(i, link)
	}

	pages := make([]PageInfo, len(links))
	for i := 0; i < len(links); i++ {
		select {
		case res := <-resultCh:
			pages[res.index] = res.page
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return &MapResponse{
		BaseURL:   req.URL,
		Source:    source,
		Pages:     pages,
		Count:     len(pages),
		Truncated: truncated,
	}, nil
}

// validateMapRequest validates the map request.
func (h *Handler) validateMapRequest(req *MapRequest) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}

	if _, err := h.parseAndValidateExternalURL(req.URL); err != nil {
		return err
	}

	if req.MaxURLs < 0 {
		return fmt.Errorf("max_urls must be non-negative")
	}

	return nil
}

func (h *Handler) parseAndValidateExternalURL(raw string) (*url.URL, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("url cannot be empty")
	}

	parsedURL, err := url.ParseRequestURI(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("url must be absolute with scheme (http/https) and host")
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("url scheme must be http or https")
	}

	if err := h.validateNotInternalURL(parsedURL); err != nil {
		return nil, err
	}

	return parsedURL, nil
}

// validateNotInternalURL prevents SSRF attacks by blocking private/internal IP addresses.
func (h *Handler) validateNotInternalURL(u *url.URL) error {
	host, _, err := net.SplitHostPort(u.Host)
	if err != nil {
		host = u.Host
	}

	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() {
			return fmt.Errorf("requests to private IP addresses are not allowed")
		}
		return nil
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return nil
	}

	for _, resolvedIP := range ips {
		if resolvedIP.IsLoopback() || resolvedIP.IsPrivate() {
			return fmt.Errorf("url resolves to private IP address: %s", host)
		}
	}

	return nil
}

// sendJSON sends a JSON response.
func (h *Handler) sendJSON(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// sendError sends an error response.
func (h *Handler) sendError(w http.ResponseWriter, message string, statusCode int) {
	errResp := ErrorResponse{
		Error:      message,
		StatusCode: statusCode,
	}
	h.sendJSON(w, errResp, statusCode)
}

// hasNoIndex checks if a page indicates it should not be indexed.
// Checks both <meta name="robots" content="noindex"> tags and X-Robots-Tag HTTP headers.
func hasNoIndex(body []byte, headers map[string][]string) bool {
	if xrobots, ok := headers["X-Robots-Tag"]; ok {
		for _, value := range xrobots {
			if strings.Contains(strings.ToLower(value), "noindex") {
				return true
			}
		}
	}

	searchLimit := len(body)
	if searchLimit > 8192 {
		searchLimit = 8192
		if headEnd := strings.Index(string(body[:searchLimit]), "</head>"); headEnd != -1 {
			searchLimit = headEnd + 7
		}
	}

	bodyStr := string(body[:searchLimit])
	lowerBody := strings.ToLower(bodyStr)

	if !strings.Contains(lowerBody, "<meta") || !strings.Contains(lowerBody, "robots") {
		return false
	}

	if !strings.Contains(lowerBody, "noindex") {
		return false
	}

	if strings.Contains(lowerBody, `name="robots"`) || strings.Contains(lowerBody, `name='robots'`) {
		metaStart := strings.Index(lowerBody, "<meta")
		for metaStart != -1 {
			metaEnd := strings.Index(lowerBody[metaStart:], ">")
			if metaEnd == -1 {
				break
			}
			metaTag := lowerBody[metaStart : metaStart+metaEnd]

			if strings.Contains(metaTag, "robots") && strings.Contains(metaTag, "noindex") {
				return true
			}

			metaStart = strings.Index(lowerBody[metaStart+metaEnd:], "<meta")
			if metaStart != -1 {
				metaStart += metaEnd
			}
		}
	}

	return false
}

// extractLanguage extracts the language code from HTML's lang attribute.
// It looks for the lang attribute on the <html> tag and returns the primary language code.
// For example: "en-US" becomes "en", "fr" remains "fr".
func extractLanguage(htmlContent string) string {
	langRegex := regexp.MustCompile(`(?i)<html[^>]+lang=["']([^"']+)["']`)
	matches := langRegex.FindStringSubmatch(htmlContent)
	if len(matches) > 1 {
		langCode := strings.TrimSpace(matches[1])
		if idx := strings.Index(langCode, "-"); idx != -1 {
			langCode = langCode[:idx]
		}
		return strings.ToLower(langCode)
	}
	return ""
}
