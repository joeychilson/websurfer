package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/joeychilson/websurfer/cache"
	"github.com/joeychilson/websurfer/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClientCreation verifies client can be created with default config.
func TestClientCreation(t *testing.T) {
	cfg := config.New()

	client, err := New(cfg)

	require.NoError(t, err)
	assert.NotNil(t, client)
}

// TestClientCreationWithNilConfig verifies client uses default config when nil.
func TestClientCreationWithNilConfig(t *testing.T) {
	client, err := New(nil)

	require.NoError(t, err)
	assert.NotNil(t, client)
}

// TestClientClose verifies client can be closed without panicking.
func TestClientClose(t *testing.T) {
	client, err := New(nil)
	require.NoError(t, err)

	// Should not panic
	assert.NotPanics(t, func() {
		client.Close()
	})
}

// TestClientFetchBasic verifies basic fetch functionality.
func TestClientFetchBasic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, World!"))
	}))
	defer server.Close()

	client, err := New(nil)
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.Fetch(context.Background(), server.URL)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(resp.Body), "Hello, World!")
	assert.Equal(t, server.URL, resp.URL)
}

// TestClientFetchHTML verifies HTML content is parsed to markdown.
func TestClientFetchHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><head><title>Test Page</title></head><body><h1>Title</h1><p>Content</p></body></html>"))
	}))
	defer server.Close()

	client, err := New(nil)
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.Fetch(context.Background(), server.URL)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	// HTML should be converted to markdown
	assert.Contains(t, string(resp.Body), "# Title", "should convert to markdown")
	assert.Equal(t, "Test Page", resp.Title, "should extract title")
}

// TestClientFetchTitleAndDescription verifies metadata extraction.
func TestClientFetchTitleAndDescription(t *testing.T) {
	html := `<html>
<head>
<title>Example Page</title>
<meta name="description" content="This is a test page">
</head>
<body>Content</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	}))
	defer server.Close()

	client, err := New(nil)
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.Fetch(context.Background(), server.URL)

	require.NoError(t, err)
	assert.Equal(t, "Example Page", resp.Title)
	assert.Equal(t, "This is a test page", resp.Description)
}

// TestClientFetchCacheMiss verifies cache state on first fetch.
func TestClientFetchCacheMiss(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("content"))
	}))
	defer server.Close()

	client, err := New(nil)
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.Fetch(context.Background(), server.URL)

	require.NoError(t, err)
	assert.Equal(t, "miss", resp.CacheState, "first fetch should be cache miss")
}

// TestBuildResponse verifies response building from cache entry.
func TestBuildResponse(t *testing.T) {
	entry := &cache.Entry{
		URL:         "https://example.com",
		StatusCode:  200,
		Headers:     map[string][]string{"Content-Type": {"text/html"}},
		Body:        []byte("content"),
		Title:       "Test",
		Description: "Desc",
		StoredAt:    time.Now(),
	}

	resp := buildResponse(entry, "hit")

	assert.Equal(t, entry.URL, resp.URL)
	assert.Equal(t, entry.StatusCode, resp.StatusCode)
	assert.Equal(t, entry.Body, resp.Body)
	assert.Equal(t, entry.Title, resp.Title)
	assert.Equal(t, entry.Description, resp.Description)
	assert.Equal(t, "hit", resp.CacheState)
	assert.False(t, resp.CachedAt.IsZero())
}

// TestBuildResponseCacheMiss verifies CachedAt is zero for cache miss.
func TestBuildResponseCacheMiss(t *testing.T) {
	entry := &cache.Entry{
		URL:        "https://example.com",
		StatusCode: 200,
		Body:       []byte("content"),
		StoredAt:   time.Now(),
	}

	resp := buildResponse(entry, "miss")

	assert.Equal(t, "miss", resp.CacheState)
	assert.True(t, resp.CachedAt.IsZero(), "cache miss should have zero CachedAt")
}
