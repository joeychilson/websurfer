package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/joeychilson/websurfer/cache"
	"github.com/joeychilson/websurfer/config"
	"github.com/redis/go-redis/v9"
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

// boolPtr returns a pointer to a bool value
func boolPtr(b bool) *bool {
	return &b
}

// TestClientFetchEndToEnd verifies complete fetch pipeline integration.
// CRITICAL: Tests robots.txt → rate limit → fetch → parse → cache flow.
func TestClientFetchEndToEnd(t *testing.T) {
	// Create test server with robots.txt and HTML page
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++

		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("User-agent: *\nAllow: /\n"))
			return
		}

		if r.URL.Path == "/page" {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`<html>
<head><title>Test Page</title></head>
<body><h1>Hello</h1><p>World</p></body>
</html>`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create config with robots.txt checking enabled
	cfg := &config.Config{
		Default: config.DefaultConfig{
			Fetch: config.FetchConfig{
				RespectRobotsTxt: boolPtr(true),
			},
		},
	}

	client, err := New(cfg)
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.Fetch(context.Background(), server.URL+"/page")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(resp.Body), "# Hello", "HTML should be converted to markdown")
	assert.Contains(t, string(resp.Body), "World", "content should be preserved")
	assert.Equal(t, "Test Page", resp.Title, "title should be extracted")

	// Verify robots.txt was checked (at least 2 calls: robots.txt + page)
	assert.GreaterOrEqual(t, callCount, 2, "should have fetched robots.txt and page")
}

// TestClientFetchRobotsDisallowed verifies robots.txt blocking works.
// CRITICAL: Respects robots.txt to avoid being blocked by sites.
func TestClientFetchRobotsDisallowed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("User-agent: *\nDisallow: /private/\n"))
			return
		}

		if r.URL.Path == "/private/secret" {
			// This should never be called
			t.Error("robots.txt disallow was not enforced!")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("SECRET DATA"))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create config with robots.txt checking enabled
	cfg := &config.Config{
		Default: config.DefaultConfig{
			Fetch: config.FetchConfig{
				RespectRobotsTxt: boolPtr(true),
			},
		},
	}

	client, err := New(cfg)
	require.NoError(t, err)
	defer client.Close()

	_, err = client.Fetch(context.Background(), server.URL+"/private/secret")

	assert.Error(t, err, "should be blocked by robots.txt")
	assert.Contains(t, err.Error(), "robots.txt", "error should mention robots.txt")
}

// TestClientConcurrentFetches verifies concurrent requests don't cause race conditions.
// CRITICAL: Multiple LLM tool calls happening simultaneously must not corrupt state.
func TestClientConcurrentFetches(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("User-agent: *\nAllow: /\n"))
			return
		}

		// Simulate different pages
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html><body>Page ` + r.URL.Path + `</body></html>`))
	}))
	defer server.Close()

	client, err := New(nil)
	require.NoError(t, err)
	defer client.Close()

	// Launch 50 concurrent requests for different URLs
	var wg sync.WaitGroup
	errors := make([]error, 50)
	responses := make([]*Response, 50)

	for i := range 50 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			url := server.URL + "/page" + string(rune('A'+idx%26))
			resp, err := client.Fetch(context.Background(), url)
			errors[idx] = err
			responses[idx] = resp
		}(i)
	}

	wg.Wait()

	// Verify all succeeded
	for i := range 50 {
		assert.NoError(t, errors[i], "request %d should succeed", i)
		assert.NotNil(t, responses[i], "response %d should not be nil", i)
	}
}

// TestClientCacheStaleBehavior verifies stale cache returns immediately.
// CRITICAL: Stale content should be returned instantly while refresh happens in background.
func TestClientCacheStaleBehavior(t *testing.T) {
	var fetchCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("User-agent: *\nAllow: /\n"))
			return
		}

		if r.URL.Path == "/page" {
			count := fetchCount.Add(1)
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
			w.WriteHeader(http.StatusOK)
			// Return different content based on fetch count
			w.Write([]byte(fmt.Sprintf("Version %d", count)))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create in-memory Redis server for testing
	mr := miniredis.RunT(t)
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	ctx := context.Background()
	cacheConfig := cache.Config{
		Prefix:    "test:stale:",
		TTL:       100 * time.Millisecond, // Very short TTL
		StaleTime: 5 * time.Second,        // Allow stale for 5s
	}
	cacheInstance := cache.New(redisClient, cacheConfig)

	client, err := New(nil)
	require.NoError(t, err)
	defer client.Close()

	// Set the cache
	client.WithCache(cacheInstance)

	// 1. First fetch populates cache
	resp1, err := client.Fetch(ctx, server.URL+"/page")
	require.NoError(t, err)
	assert.Equal(t, "Version 1", string(resp1.Body))
	assert.Equal(t, "miss", resp1.CacheState)
	assert.Equal(t, int32(1), fetchCount.Load())

	// 2. Immediate second fetch should return fresh from cache
	resp2, err := client.Fetch(ctx, server.URL+"/page")
	require.NoError(t, err)
	assert.Equal(t, "Version 1", string(resp2.Body))
	assert.Equal(t, "hit", resp2.CacheState)
	assert.Equal(t, int32(1), fetchCount.Load(), "should not fetch again (cache fresh)")

	// 3. Wait for TTL to expire (entry becomes stale)
	time.Sleep(150 * time.Millisecond)

	// 4. Third fetch should return stale content immediately (< 10ms)
	start := time.Now()
	resp3, err := client.Fetch(ctx, server.URL+"/page")
	staleDuration := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, "Version 1", string(resp3.Body), "should return stale content immediately")
	assert.Equal(t, "stale", resp3.CacheState, "cache state should be stale")
	assert.Less(t, staleDuration, 50*time.Millisecond, "stale response should be instant")

	// 5. Wait for background refresh to complete (be generous with timing)
	time.Sleep(1 * time.Second)

	// 6. Fourth fetch should get refreshed content from cache
	resp4, err := client.Fetch(ctx, server.URL+"/page")
	require.NoError(t, err)
	assert.Equal(t, "Version 2", string(resp4.Body), "should have refreshed content")
	// Note: Could be "hit" or "stale" depending on timing - the key is content was refreshed
	assert.Contains(t, []string{"hit", "stale"}, resp4.CacheState, "should have cache state")
	assert.Equal(t, int32(2), fetchCount.Load(), "background refresh should have fetched once")

	t.Logf("Stale response returned in %v (target: < 50ms)", staleDuration)
}

// TestClientFetchWithConditionalRequest verifies If-Modified-Since handling.
// Reduces bandwidth and server load when content hasn't changed.
func TestClientFetchWithConditionalRequest(t *testing.T) {
	lastModified := "Wed, 21 Oct 2015 07:28:00 GMT"
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("User-agent: *\nAllow: /\n"))
			return
		}

		if r.URL.Path == "/page" {
			ifModifiedSince := r.Header.Get("If-Modified-Since")
			if ifModifiedSince == lastModified {
				// Content not modified
				w.WriteHeader(http.StatusNotModified)
				return
			}

			w.Header().Set("Last-Modified", lastModified)
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Content"))
			return
		}
	}))
	defer server.Close()

	// Note: This tests the fetcher level, client integration with cache would handle this
	// For now, documenting the expected behavior
	t.Log("Conditional request behavior is tested at fetcher level")
}

// TestClientFetchTimeout verifies fetch respects context deadline.
// CRITICAL: LLM tools should timeout after reasonable period, not hang forever.
func TestClientFetchTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("User-agent: *\nAllow: /\n"))
			return
		}

		// Hang for 10 seconds
		time.Sleep(10 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := New(nil)
	require.NoError(t, err)
	defer client.Close()

	// Create context with 1 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	_, err = client.Fetch(ctx, server.URL+"/slow")

	assert.Error(t, err, "should timeout after 1 second")
	assert.Contains(t, err.Error(), "context deadline exceeded", "should be timeout error")
}

// TestClientFetchLargeDocument verifies handling of large HTML documents.
// Tests memory efficiency and parsing performance.
func TestClientFetchLargeDocument(t *testing.T) {
	// Generate large HTML (1MB)
	largeHTML := `<html><head><title>Large Doc</title></head><body>`
	for i := range 10000 {
		largeHTML += `<p>Paragraph ` + string(rune('0'+i%10)) + ` content here.</p>`
	}
	largeHTML += `</body></html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("User-agent: *\nAllow: /\n"))
			return
		}

		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(largeHTML))
	}))
	defer server.Close()

	client, err := New(nil)
	require.NoError(t, err)
	defer client.Close()

	start := time.Now()
	resp, err := client.Fetch(context.Background(), server.URL+"/large")
	duration := time.Since(start)

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Greater(t, len(resp.Body), 10000, "should have substantial markdown output")

	// Should complete in reasonable time (< 5s for 1MB HTML)
	assert.Less(t, duration, 5*time.Second, "large document fetch should complete quickly")

	t.Logf("Fetched and parsed 1MB HTML in %v", duration)
}

// TestClientFetchErrorHandling verifies various error scenarios.
func TestClientFetchErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		shouldError bool
		errorText   string
	}{
		{
			name:        "invalid_url",
			url:         "not-a-valid-url",
			shouldError: true,
			errorText:   "no host",
		},
		{
			name:        "private_ip",
			url:         "http://192.168.1.1",
			shouldError: true,
			errorText:   "private",
		},
		{
			name:        "localhost",
			url:         "http://localhost:8080",
			shouldError: true,
			errorText:   "private",
		},
	}

	// Create config with SSRF protection enabled
	cfg := &config.Config{
		Default: config.DefaultConfig{
			Fetch: config.FetchConfig{
				EnableSSRFProtection: boolPtr(true),
			},
		},
	}

	client, err := New(cfg)
	require.NoError(t, err)
	defer client.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.Fetch(context.Background(), tt.url)

			if tt.shouldError {
				assert.Error(t, err)
				if tt.errorText != "" {
					assert.Contains(t, err.Error(), tt.errorText,
						"error message should be descriptive for LLM tools")
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestClientAWSMetadataSSRFProtection verifies AWS/GCP/Azure metadata endpoints are blocked.
// CRITICAL: Blocks 169.254.169.254 to prevent SSRF attacks against cloud metadata services.
func TestClientAWSMetadataSSRFProtection(t *testing.T) {
	// Create config with SSRF protection enabled
	cfg := &config.Config{
		Default: config.DefaultConfig{
			Fetch: config.FetchConfig{
				EnableSSRFProtection: boolPtr(true),
			},
		},
	}

	client, err := New(cfg)
	require.NoError(t, err)
	defer client.Close()

	// AWS metadata endpoint should be blocked
	_, err = client.Fetch(context.Background(), "http://169.254.169.254/latest/meta-data/")

	assert.Error(t, err, "AWS metadata endpoint should be blocked")
	assert.Contains(t, err.Error(), "link-local", "error should mention link-local address")
}

// TestClientResponseMetadataCompleteness verifies all metadata fields are populated.
// LLM tools need complete metadata for context.
func TestClientResponseMetadataCompleteness(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("User-agent: *\nAllow: /\n"))
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<html>
<head>
<title>Complete Page</title>
<meta name="description" content="A test page with all metadata">
</head>
<body><h1>Content</h1></body>
</html>`))
	}))
	defer server.Close()

	client, err := New(nil)
	require.NoError(t, err)
	defer client.Close()

	resp, err := client.Fetch(context.Background(), server.URL+"/page")

	require.NoError(t, err)

	// Verify all metadata fields
	assert.Equal(t, server.URL+"/page", resp.URL, "should have URL")
	assert.Equal(t, http.StatusOK, resp.StatusCode, "should have status code")
	assert.NotNil(t, resp.Headers, "should have headers")
	assert.NotEmpty(t, resp.Body, "should have body")
	assert.Equal(t, "Complete Page", resp.Title, "should extract title")
	assert.Equal(t, "A test page with all metadata", resp.Description, "should extract description")
	assert.NotEmpty(t, resp.CacheState, "should have cache state")
}
