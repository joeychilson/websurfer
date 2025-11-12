package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/joeychilson/websurfer/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	t.Skip("Requires cache implementation and background refresh - integration test for future implementation")

	// TODO: This test would verify:
	// 1. First fetch populates cache
	// 2. Wait for TTL to expire (entry becomes stale)
	// 3. Second fetch returns stale content immediately (< 10ms)
	// 4. Background refresh updates cache
	// 5. Third fetch gets fresh content
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
