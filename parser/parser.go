package parser

import (
	"context"
	"fmt"
	"strings"
)

// contextKey is a private type for context keys to avoid collisions.
type contextKey string

const (
	// urlContextKey stores the URL being parsed in the context.
	urlContextKey contextKey = "parser_url"
)

// Parser transforms content into an LLM-friendly format.
type Parser interface {
	// Parse transforms the content and returns the cleaned result.
	Parse(ctx context.Context, content []byte) ([]byte, error)
}

// WithURL adds the URL to the context for parsers to use.
func WithURL(ctx context.Context, url string) context.Context {
	return context.WithValue(ctx, urlContextKey, url)
}

// GetURL retrieves the URL from the context if it was set with WithURL.
func GetURL(ctx context.Context) string {
	if urlVal := ctx.Value(urlContextKey); urlVal != nil {
		if urlStr, ok := urlVal.(string); ok {
			return urlStr
		}
	}
	return ""
}

// Registry manages multiple parsers and routes content based on content-type.
type Registry struct {
	parsers map[string]Parser
}

// New creates a new parser registry.
func New() *Registry {
	return &Registry{
		parsers: make(map[string]Parser),
	}
}

// Register registers a parser for one or more content types.
func (r *Registry) Register(contentTypes []string, parser Parser) {
	for _, ct := range contentTypes {
		baseType := NormalizeContentType(ct)
		r.parsers[baseType] = parser
	}
}

// Parse transforms content based on its content-type.
func (r *Registry) Parse(ctx context.Context, contentType string, content []byte) ([]byte, error) {
	if contentType == "" || len(content) == 0 {
		return content, nil
	}

	baseType := NormalizeContentType(contentType)

	parser, exists := r.parsers[baseType]
	if !exists {
		return content, nil
	}

	parsed, err := parser.Parse(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", baseType, err)
	}

	return parsed, nil
}

// HasParser returns true if a parser is registered for the given content-type.
func (r *Registry) HasParser(contentType string) bool {
	baseType := NormalizeContentType(contentType)
	_, exists := r.parsers[baseType]
	return exists
}

// NormalizeContentType extracts the base content-type, removing parameters.
func NormalizeContentType(contentType string) string {
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = contentType[:idx]
	}

	return strings.ToLower(strings.TrimSpace(contentType))
}
