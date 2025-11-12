package parser

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockParser is a simple parser for testing
type mockParser struct {
	result []byte
	err    error
}

func (m *mockParser) Parse(ctx context.Context, content []byte) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

// TestRegistryNew verifies registry can be created.
func TestRegistryNew(t *testing.T) {
	registry := New()
	assert.NotNil(t, registry, "should create registry")
}

// TestRegistryRegisterAndParse verifies parsers can be registered and invoked.
func TestRegistryRegisterAndParse(t *testing.T) {
	registry := New()
	mockP := &mockParser{result: []byte("parsed content")}

	registry.Register([]string{"text/html"}, mockP)

	result, err := registry.Parse(context.Background(), "text/html", []byte("original"))

	require.NoError(t, err)
	assert.Equal(t, []byte("parsed content"), result)
}

// TestRegistryMultipleContentTypes verifies one parser can handle multiple types.
func TestRegistryMultipleContentTypes(t *testing.T) {
	registry := New()
	mockP := &mockParser{result: []byte("parsed")}

	registry.Register([]string{"text/html", "application/xhtml+xml", "text/plain"}, mockP)

	tests := []string{"text/html", "application/xhtml+xml", "text/plain"}
	for _, contentType := range tests {
		result, err := registry.Parse(context.Background(), contentType, []byte("original"))
		require.NoError(t, err)
		assert.Equal(t, []byte("parsed"), result, "should work for %s", contentType)
	}
}

// TestRegistryNoParserRegistered verifies unregistered types return original content.
func TestRegistryNoParserRegistered(t *testing.T) {
	registry := New()

	original := []byte("unchanged")
	result, err := registry.Parse(context.Background(), "application/json", original)

	require.NoError(t, err)
	assert.Equal(t, original, result, "should return original when no parser registered")
}

// TestRegistryEmptyContent verifies empty content is handled.
func TestRegistryEmptyContent(t *testing.T) {
	registry := New()
	mockP := &mockParser{result: []byte("should not be called")}
	registry.Register([]string{"text/html"}, mockP)

	result, err := registry.Parse(context.Background(), "text/html", []byte(""))

	require.NoError(t, err)
	assert.Equal(t, []byte(""), result, "should return empty for empty input")
}

// TestRegistryEmptyContentType verifies empty content-type returns original.
func TestRegistryEmptyContentType(t *testing.T) {
	registry := New()
	original := []byte("content")

	result, err := registry.Parse(context.Background(), "", original)

	require.NoError(t, err)
	assert.Equal(t, original, result)
}

// TestRegistryHasParser verifies parser detection.
func TestRegistryHasParser(t *testing.T) {
	registry := New()
	mockP := &mockParser{}

	registry.Register([]string{"text/html"}, mockP)

	assert.True(t, registry.HasParser("text/html"), "should have parser for text/html")
	assert.False(t, registry.HasParser("application/json"), "should not have parser for json")
}

// TestRegistryHasParserWithParameters verifies content-type normalization in HasParser.
func TestRegistryHasParserWithParameters(t *testing.T) {
	registry := New()
	mockP := &mockParser{}

	registry.Register([]string{"text/html"}, mockP)

	assert.True(t, registry.HasParser("text/html; charset=utf-8"))
	assert.True(t, registry.HasParser("TEXT/HTML"))
	assert.True(t, registry.HasParser("text/html "))
}

// TestNormalizeContentType verifies content-type normalization.
func TestNormalizeContentType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"text/html", "text/html"},
		{"text/html; charset=utf-8", "text/html"},
		{"TEXT/HTML", "text/html"},
		{"text/html ", "text/html"},
		{" text/html", "text/html"},
		{"application/json; charset=utf-8; boundary=something", "application/json"},
	}

	for _, tt := range tests {
		result := NormalizeContentType(tt.input)
		assert.Equal(t, tt.expected, result, "input: %s", tt.input)
	}
}

// TestWithURL verifies URL can be added to context.
func TestWithURL(t *testing.T) {
	ctx := context.Background()
	url := "https://example.com/page"

	ctx = WithURL(ctx, url)

	retrieved := GetURL(ctx)
	assert.Equal(t, url, retrieved)
}

// TestGetURLEmpty verifies GetURL returns empty for context without URL.
func TestGetURLEmpty(t *testing.T) {
	ctx := context.Background()

	url := GetURL(ctx)
	assert.Equal(t, "", url)
}

// TestGetURLWrongType verifies GetURL handles wrong type in context gracefully.
func TestGetURLWrongType(t *testing.T) {
	ctx := context.Background()
	// Manually add wrong type to context (shouldn't happen in practice)
	ctx = context.WithValue(ctx, urlContextKey, 123)

	url := GetURL(ctx)
	assert.Equal(t, "", url, "should return empty for wrong type")
}

// TestRegistryParserError verifies parser errors are propagated.
func TestRegistryParserError(t *testing.T) {
	registry := New()
	mockP := &mockParser{err: assert.AnError}

	registry.Register([]string{"text/html"}, mockP)

	_, err := registry.Parse(context.Background(), "text/html", []byte("content"))

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse text/html")
}

// TestRegistryContentTypeNormalizationInParse verifies Parse normalizes content-type.
func TestRegistryContentTypeNormalizationInParse(t *testing.T) {
	registry := New()
	mockP := &mockParser{result: []byte("normalized")}

	registry.Register([]string{"text/html"}, mockP)

	// Try with parameters and case variations
	inputs := []string{
		"text/html; charset=utf-8",
		"TEXT/HTML",
		"text/html ",
	}

	for _, input := range inputs {
		result, err := registry.Parse(context.Background(), input, []byte("original"))
		require.NoError(t, err)
		assert.Equal(t, []byte("normalized"), result, "should normalize: %s", input)
	}
}
