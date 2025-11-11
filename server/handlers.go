package server

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
	"github.com/joeychilson/websurfer/outline"
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

		start := 0
		if req.Range != nil {
			start = req.Range.Start
		}
		end := truncation.ReturnedChars
		if req.Range != nil {
			end = req.Range.Start + truncation.ReturnedChars
		}

		response := &FetchResponse{
			Metadata:   metadata,
			Content:    truncation.Content,
			Navigation: buildNavigationForContent(start, end, len(bodyText), req.MaxTokens),
		}

		return response, nil
	}

	estimatedTokens := content.EstimateTokens(workingText, contentType)

	metadata := buildFetchMetadata(fetched, contentType, language, lastModified, estimatedTokens)

	// Extract outline from the markdown/text content (not original HTML)
	// HTML parser now returns markdown, so outline should be based on that
	documentOutline := outline.Extract(workingText, "text/markdown")

	start := 0
	end := len(workingText)
	if req.Range != nil {
		start = req.Range.Start
		end = req.Range.End
	}

	return &FetchResponse{
		Metadata:   metadata,
		Content:    workingText,
		Outline:    documentOutline,
		Navigation: buildNavigationForContent(start, end, len(workingText), 0),
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

func buildNavigationForContent(start, end, totalLength, maxTokens int) *content.Navigation {
	nav := &content.Navigation{
		Current: &content.RangeOptions{
			Type:  "chars",
			Start: start,
			End:   end,
		},
		Options: []content.NavigationOption{},
	}

	chunkSize := end - start
	if maxTokens > 0 {
		chunkSize = end - start
	} else if chunkSize == 0 {
		chunkSize = 50000
	}

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

	if start > 0 || end < totalLength {
		nav.Options = append(nav.Options, content.NavigationOption{
			ID: "full",
			Range: &content.RangeOptions{
				Type:  "chars",
				Start: 0,
				End:   totalLength,
			},
			Description: "Get entire document (warning: may be very large)",
		})
	}

	return nav
}

func firstHeader(headers map[string][]string, key string) string {
	if values, ok := headers[key]; ok && len(values) > 0 {
		return values[0]
	}
	return ""
}

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

func (h *Handler) parseAndValidateExternalURL(raw string) (*url.URL, error) {
	if err := urlpkg.ValidateExternal(raw); err != nil {
		return nil, err
	}
	return urlpkg.ParseAndValidate(raw)
}

func (h *Handler) sendJSON(w http.ResponseWriter, data interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

func (h *Handler) sendError(w http.ResponseWriter, message string, statusCode int) {
	errResp := ErrorResponse{
		Error:      message,
		StatusCode: statusCode,
	}
	h.sendJSON(w, errResp, statusCode)
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
