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

var (
	// langRegex extracts the language code from HTML lang attribute
	langRegex = regexp.MustCompile(`(?i)<html[^>]+lang=["']([^"']+)["']`)
)

// FetchRequest represents a request to fetch and process a URL.
type FetchRequest struct {
	URL       string `json:"url"`
	MaxTokens int    `json:"max_tokens,omitempty"`
	Offset    int    `json:"offset,omitempty"`
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
	Metadata   Metadata         `json:"metadata"`
	Content    string           `json:"content,omitempty"`
	Outline    *outline.Outline `json:"outline,omitempty"`
	Pagination *Pagination      `json:"pagination,omitempty"`
}

// Pagination contains pagination information for the response.
type Pagination struct {
	Offset              int  `json:"offset"`
	Limit               int  `json:"limit"`
	TotalTokens         int  `json:"total_tokens"`
	HasMore             bool `json:"has_more"`
	SuggestedNextOffset int  `json:"suggested_next_offset,omitempty"`
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

// processFetch handles the fetch request processing logic.
func (s *Server) processFetch(ctx context.Context, req *FetchRequest) (*FetchResponse, error) {
	fetched, err := s.client.Fetch(ctx, req.URL)
	if err != nil {
		return nil, err
	}

	contentType := firstHeader(fetched.Headers, "Content-Type")
	lastModified := firstHeader(fetched.Headers, "Last-Modified")

	var language string
	if strings.Contains(strings.ToLower(contentType), "html") {
		language = extractLanguage(fetched.Body)
	}

	workingBytes := fetched.Body

	if req.MaxTokens > 0 || req.Offset > 0 {
		return s.buildPaginatedResponse(fetched, workingBytes, contentType, language, lastModified, req)
	}

	return s.buildFullResponse(fetched, workingBytes, contentType, language, lastModified)
}

// buildPaginatedResponse builds a response with pagination for offset/max_tokens requests.
func (s *Server) buildPaginatedResponse(fetched *client.Response, workingBytes []byte, contentType, language, lastModified string, req *FetchRequest) (*FetchResponse, error) {
	totalTokens := content.EstimateTokens(workingBytes, contentType)

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4000
	}

	charsPerToken := float64(len(workingBytes)) / float64(totalTokens)
	charOffset := int(float64(req.Offset) * charsPerToken)

	if charOffset >= len(workingBytes) {
		return nil, fmt.Errorf("offset %d exceeds content length (total tokens: %d)", req.Offset, totalTokens)
	}

	contentFromOffset := workingBytes[charOffset:]

	truncation := content.Truncate(contentFromOffset, contentType, maxTokens)

	metadata := buildFetchMetadata(fetched, contentType, language, lastModified, truncation.ReturnedTokens)

	currentEndOffset := req.Offset + truncation.ReturnedTokens
	hasMore := currentEndOffset < totalTokens

	pagination := &Pagination{
		Offset:      req.Offset,
		Limit:       maxTokens,
		TotalTokens: totalTokens,
		HasMore:     hasMore,
	}

	if hasMore {
		pagination.SuggestedNextOffset = currentEndOffset
	}

	var documentOutline *outline.Outline
	if req.Offset == 0 && strings.Contains(contentType, "markdown") {
		documentOutline = outline.ExtractBytes(workingBytes, contentType)
	}

	return &FetchResponse{
		Metadata:   metadata,
		Content:    truncation.Content,
		Outline:    documentOutline,
		Pagination: pagination,
	}, nil
}

// buildFullResponse builds a response with full content (no pagination).
func (s *Server) buildFullResponse(fetched *client.Response, workingBytes []byte, contentType, language, lastModified string) (*FetchResponse, error) {
	estimatedTokens := content.EstimateTokens(workingBytes, contentType)
	metadata := buildFetchMetadata(fetched, contentType, language, lastModified, estimatedTokens)

	var documentOutline *outline.Outline
	if strings.Contains(contentType, "markdown") {
		documentOutline = outline.ExtractBytes(workingBytes, contentType)
	}

	return &FetchResponse{
		Metadata: metadata,
		Content:  string(workingBytes),
		Outline:  documentOutline,
	}, nil
}

// validateRequest validates the fetch request.
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

	if req.Offset < 0 {
		return fmt.Errorf("offset must be non-negative")
	}

	return nil
}

// handleHealth handles GET /health requests.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	}
	s.sendJSON(w, health, http.StatusOK)
}

// sendJSON sends a JSON response.
func (s *Server) sendJSON(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(data); err != nil {
		s.logger.Error("failed to encode response", "error", err)
	}
}

// sendError sends an error response.
func (s *Server) sendError(w http.ResponseWriter, message string, statusCode int) {
	errResp := ErrorResponse{
		Error:      message,
		StatusCode: statusCode,
	}
	s.sendJSON(w, errResp, statusCode)
}

// extractLanguage extracts the language from the HTML content.
func extractLanguage(htmlContent []byte) string {
	matches := langRegex.FindSubmatch(htmlContent)
	if len(matches) > 1 {
		langCode := strings.TrimSpace(string(matches[1]))
		if idx := strings.Index(langCode, "-"); idx != -1 {
			langCode = langCode[:idx]
		}
		return strings.ToLower(langCode)
	}
	return ""
}

// buildFetchMetadata builds the fetch metadata.
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

// firstHeader returns the first value for a given header key.
func firstHeader(headers map[string][]string, key string) string {
	if values, ok := headers[key]; ok && len(values) > 0 {
		return values[0]
	}
	return ""
}
