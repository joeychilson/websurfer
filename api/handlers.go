package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/joeychilson/websurfer/client"
	"github.com/joeychilson/websurfer/content"
	"github.com/joeychilson/websurfer/logger"
	"github.com/joeychilson/websurfer/parser/html"
	urlpkg "github.com/joeychilson/websurfer/url"
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
	URL               string `json:"url"`
	MaxURLs           int    `json:"max_urls,omitempty"`
	SameDomain        bool   `json:"same_domain,omitempty"`
	IncludeSubdomains bool   `json:"include_subdomains,omitempty"`
	Depth             int    `json:"depth,omitempty"`
	PathPrefix        string `json:"path_prefix,omitempty"`
}

// MapResponse represents the response from a map request.
type MapResponse struct {
	BaseURL   string   `json:"base_url"`
	Source    string   `json:"source"`
	URLs      []string `json:"urls"`
	Count     int      `json:"count"`
	Truncated bool     `json:"truncated"`
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
			childURLs, err := h.client.FetchSitemapURLs(ctx, sitemapURL, 3)
			if err == nil {
				allURLs = append(allURLs, childURLs...)
			}
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
		sitemapURLs, err := h.client.FetchSitemapURLs(ctx, sitemapURL, 3)
		if err == nil && len(sitemapURLs) > 0 {
			links = sitemapURLs
			source = "sitemap"
			h.logger.Info("extracted URLs from sitemap.xml", "url", sitemapURL, "count", len(links))
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
		filtered := make([]string, 0, len(links))
		for _, link := range links {
			var matches bool
			if req.IncludeSubdomains {
				// Include all subdomains (e.g., blog.example.com matches example.com)
				matches = urlpkg.IsSameBaseDomain(req.URL, link)
			} else {
				// Exact domain match only (ignoring www)
				matches = urlpkg.IsSameDomain(req.URL, link)
			}
			if matches {
				filtered = append(filtered, link)
			}
		}
		links = filtered
		h.logger.Debug("filtered by domain", "same_domain", req.SameDomain, "include_subdomains", req.IncludeSubdomains, "count", len(links))
	}

	links = urlpkg.Deduplicate(links)
	h.logger.Debug("deduplicated URLs", "count", len(links))

	if req.PathPrefix != "" {
		filtered := make([]string, 0, len(links))
		for _, link := range links {
			parsedURL, err := url.Parse(link)
			if err == nil && strings.HasPrefix(parsedURL.Path, req.PathPrefix) {
				filtered = append(filtered, link)
			}
		}
		links = filtered
		h.logger.Debug("filtered by path prefix", "prefix", req.PathPrefix, "count", len(links))
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

	return &MapResponse{
		BaseURL:   req.URL,
		Source:    source,
		URLs:      links,
		Count:     len(links),
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
	if err := urlpkg.ValidateExternal(raw); err != nil {
		return nil, err
	}
	return urlpkg.ParseAndValidate(raw)
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
