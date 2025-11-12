package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/joeychilson/websurfer/client"
	"github.com/joeychilson/websurfer/content"
	"github.com/joeychilson/websurfer/outline"
	urlpkg "github.com/joeychilson/websurfer/url"
)

const defaultChunkSize = 50000

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
	Metadata   Metadata            `json:"metadata"`
	Content    string              `json:"content,omitempty"`
	Outline    *outline.Outline    `json:"outline,omitempty"`
	Navigation *content.Navigation `json:"navigation,omitempty"`
}

// ErrorResponse represents an error.
type ErrorResponse struct {
	Error      string            `json:"error"`
	StatusCode int               `json:"status_code"`
	Details    map[string]string `json:"details,omitempty"`
}

// handleFetch handles POST /v1/fetch requests.
func (s *Server) handleFetch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req FetchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.logger.Error("failed to decode request", "error", err)
		s.sendError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := s.validateRequest(&req); err != nil {
		s.logger.Error("invalid request", "error", err)
		s.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.logger.Info("fetch request", "url", req.URL, "max_tokens", req.MaxTokens)

	resp, err := s.processFetch(ctx, &req)
	if err != nil {
		s.logger.Error("fetch failed", "url", req.URL, "error", err)
		s.sendError(w, fmt.Sprintf("failed to fetch %s: %v", req.URL, err), http.StatusInternalServerError)
		return
	}

	s.logger.Info("fetch completed",
		"url", resp.Metadata.URL,
		"status_code", resp.Metadata.StatusCode)

	s.sendJSON(w, resp, http.StatusOK)
}

// handleHealth handles GET /health requests.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	}
	s.sendJSON(w, health, http.StatusOK)
}

// processFetch handles the fetch request processing logic.
func (s *Server) processFetch(ctx context.Context, req *FetchRequest) (*FetchResponse, error) {
	fetched, err := s.client.Fetch(ctx, req.URL)
	if err != nil {
		return nil, err
	}

	contentType := firstHeader(fetched.Headers, "Content-Type")
	lastModified := firstHeader(fetched.Headers, "Last-Modified")
	language := extractLanguageIfHTML(contentType, fetched.Body)

	workingBytes := fetched.Body
	if req.Range != nil {
		workingBytes, err = content.ExtractRangeBytes(workingBytes, req.Range)
		if err != nil {
			return nil, fmt.Errorf("range extraction failed: %w", err)
		}
	}

	if req.MaxTokens > 0 {
		return s.buildTruncatedResponse(fetched, workingBytes, contentType, language, lastModified, req)
	}

	return s.buildFullResponse(fetched, workingBytes, contentType, language, lastModified, req)
}

// extractLanguageIfHTML extracts language from HTML content if applicable.
func extractLanguageIfHTML(contentType string, body []byte) string {
	if strings.Contains(strings.ToLower(contentType), "html") {
		return extractLanguage(string(body))
	}
	return ""
}

// buildTruncatedResponse builds a response with truncated content for max_tokens requests.
func (s *Server) buildTruncatedResponse(fetched *client.Response, workingBytes []byte, contentType, language, lastModified string, req *FetchRequest) (*FetchResponse, error) {
	truncation := content.TruncateBytes(workingBytes, contentType, req.MaxTokens)
	metadata := buildFetchMetadata(fetched, contentType, language, lastModified, truncation.ReturnedTokens)

	start, end := calculateTruncationRange(req.Range, truncation.ReturnedChars)

	return &FetchResponse{
		Metadata:   metadata,
		Content:    truncation.Content,
		Navigation: buildNavigationForContent(start, end, len(fetched.Body), req.MaxTokens),
	}, nil
}

// buildFullResponse builds a response with full content.
func (s *Server) buildFullResponse(fetched *client.Response, workingBytes []byte, contentType, language, lastModified string, req *FetchRequest) (*FetchResponse, error) {
	estimatedTokens := content.EstimateTokensBytes(workingBytes, contentType)
	metadata := buildFetchMetadata(fetched, contentType, language, lastModified, estimatedTokens)
	documentOutline := outline.ExtractBytes(workingBytes, "text/markdown")

	start, end := calculateFullRange(req.Range, len(workingBytes))

	return &FetchResponse{
		Metadata:   metadata,
		Content:    string(workingBytes),
		Outline:    documentOutline,
		Navigation: buildNavigationForContent(start, end, len(workingBytes), 0),
	}, nil
}

// calculateTruncationRange calculates the start and end positions for truncated content.
func calculateTruncationRange(rangeOpt *content.RangeOptions, returnedChars int) (start, end int) {
	start = 0
	if rangeOpt != nil {
		start = rangeOpt.Start
	}
	end = returnedChars
	if rangeOpt != nil {
		end = rangeOpt.Start + returnedChars
	}
	return start, end
}

// calculateFullRange calculates the start and end positions for full content.
func calculateFullRange(rangeOpt *content.RangeOptions, totalLength int) (start, end int) {
	start = 0
	end = totalLength
	if rangeOpt != nil {
		start = rangeOpt.Start
		end = rangeOpt.End
	}
	return start, end
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

func buildNavigationForContent(start, end, totalLength, maxTokens int) *content.Navigation {
	nav := &content.Navigation{
		Current: &content.RangeOptions{
			Type:  "chars",
			Start: start,
			End:   end,
		},
		Options: []content.NavigationOption{},
	}

	chunkSize := calculateChunkSize(start, end, maxTokens)

	addPreviousNavigation(nav, start, chunkSize)
	addNextNavigation(nav, end, totalLength, chunkSize)
	addExpandNavigation(nav, start, end, totalLength, chunkSize)
	addFullNavigation(nav, start, end, totalLength)

	return nav
}

// calculateChunkSize determines the chunk size for navigation.
func calculateChunkSize(start, end, maxTokens int) int {
	chunkSize := end - start
	if chunkSize == 0 && maxTokens == 0 {
		chunkSize = defaultChunkSize
	}
	return chunkSize
}

// addPreviousNavigation adds a "previous" navigation option if applicable.
func addPreviousNavigation(nav *content.Navigation, start, chunkSize int) {
	if start > 0 {
		prevStart := max(0, start-chunkSize)
		nav.Options = append(nav.Options, content.NavigationOption{
			ID: "previous",
			Range: &content.RangeOptions{
				Type:  "chars",
				Start: prevStart,
				End:   start,
			},
			Description: "Get previous chunk of content",
		})
	}
}

// addNextNavigation adds a "next" navigation option if applicable.
func addNextNavigation(nav *content.Navigation, end, totalLength, chunkSize int) {
	if end < totalLength {
		nextEnd := min(totalLength, end+chunkSize)
		nav.Options = append(nav.Options, content.NavigationOption{
			ID: "next",
			Range: &content.RangeOptions{
				Type:  "chars",
				Start: end,
				End:   nextEnd,
			},
			Description: "Get next chunk of content",
		})
	}
}

// addExpandNavigation adds an "expand_forward" navigation option if applicable.
func addExpandNavigation(nav *content.Navigation, start, end, totalLength, chunkSize int) {
	if end < totalLength {
		expandEnd := min(totalLength, end+chunkSize)
		nav.Options = append(nav.Options, content.NavigationOption{
			ID: "expand_forward",
			Range: &content.RangeOptions{
				Type:  "chars",
				Start: start,
				End:   expandEnd,
			},
			Description: "Expand current view to include more content",
		})
	}
}

// addFullNavigation adds a "full" navigation option if the view is partial.
func addFullNavigation(nav *content.Navigation, start, end, totalLength int) {
	if start > 0 || end < totalLength {
		nav.Options = append(nav.Options, content.NavigationOption{
			ID: "full",
			Range: &content.RangeOptions{
				Type:  "chars",
				Start: 0,
				End:   totalLength,
			},
			Description: "Get entire document",
		})
	}
}

func firstHeader(headers map[string][]string, key string) string {
	if values, ok := headers[key]; ok && len(values) > 0 {
		return values[0]
	}
	return ""
}

func (s *Server) validateRequest(req *FetchRequest) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}

	if _, err := urlpkg.ValidateExternal(req.URL); err != nil {
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

func (s *Server) sendJSON(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(data); err != nil {
		s.logger.Error("failed to encode response", "error", err)
	}
}

func (s *Server) sendError(w http.ResponseWriter, message string, statusCode int) {
	errResp := ErrorResponse{
		Error:      message,
		StatusCode: statusCode,
	}
	s.sendJSON(w, errResp, statusCode)
}

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
