package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/joeychilson/websurfer/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFetcherBasicFetch verifies basic successful HTTP fetch.
func TestFetcherBasicFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, World!"))
	}))
	defer server.Close()

	fetcher, err := New(config.FetchConfig{})
	require.NoError(t, err)

	resp, err := fetcher.FetchWithOptions(context.Background(), server.URL, nil)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Hello, World!", string(resp.Body))
	assert.Equal(t, server.URL, resp.URL)
}

// TestFetcherFollowsRedirects verifies redirect following works.
func TestFetcherFollowsRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		if r.URL.Path == "/final" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Final destination"))
			return
		}
	}))
	defer server.Close()

	followRedirects := true
	fetcher, err := New(config.FetchConfig{
		FollowRedirects: &followRedirects,
		MaxRedirects:    5,
	})
	require.NoError(t, err)

	resp, err := fetcher.FetchWithOptions(context.Background(), server.URL+"/redirect", nil)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Final destination", string(resp.Body))
	assert.Contains(t, resp.URL, "/final", "should return final URL after redirect")
}

// TestFetcherMaxRedirects verifies max redirects is enforced.
func TestFetcherMaxRedirects(t *testing.T) {
	redirectCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		if redirectCount < 10 {
			http.Redirect(w, r, "/redirect", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Done"))
	}))
	defer server.Close()

	followRedirects := true
	fetcher, err := New(config.FetchConfig{
		FollowRedirects: &followRedirects,
		MaxRedirects:    3,
	})
	require.NoError(t, err)

	_, err = fetcher.FetchWithOptions(context.Background(), server.URL+"/redirect", nil)

	assert.Error(t, err, "should fail after max redirects")
	assert.Contains(t, err.Error(), "redirects", "error should mention redirects")
}

// TestFetcherDisabledRedirects verifies redirects can be disabled.
func TestFetcherDisabledRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer server.Close()

	followRedirects := false
	fetcher, err := New(config.FetchConfig{
		FollowRedirects: &followRedirects,
		MaxRedirects:    0,
	})
	require.NoError(t, err)

	resp, err := fetcher.FetchWithOptions(context.Background(), server.URL, nil)

	// Fetcher returns error for non-2xx responses
	assert.Error(t, err, "non-2xx response returns error")
	if resp != nil {
		assert.Equal(t, http.StatusFound, resp.StatusCode, "should have redirect status in response")
	}
}

// TestFetcherConditionalRequest verifies If-Modified-Since header.
func TestFetcherConditionalRequest(t *testing.T) {
	lastModified := "Wed, 21 Oct 2015 07:28:00 GMT"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-Modified-Since") == lastModified {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Fresh content"))
	}))
	defer server.Close()

	fetcher, err := New(config.FetchConfig{})
	require.NoError(t, err)

	// First request - no conditional header
	resp, err := fetcher.FetchWithOptions(context.Background(), server.URL, nil)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Fresh content", string(resp.Body))

	// Second request - with If-Modified-Since
	// Note: fetcher returns error for 304 since it's not 2xx
	opts := &FetchOptions{IfModifiedSince: lastModified}
	resp, err = fetcher.FetchWithOptions(context.Background(), server.URL, opts)
	assert.Error(t, err, "304 is not 2xx, so fetcher returns error")
	if resp != nil {
		assert.Equal(t, http.StatusNotModified, resp.StatusCode)
	}
}

// TestFetcherCustomHeaders verifies custom headers are sent.
func TestFetcherCustomHeaders(t *testing.T) {
	var receivedUA, receivedCustom string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		receivedCustom = r.Header.Get("X-Custom-Header")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	fetcher, err := New(config.FetchConfig{
		Headers: map[string]string{
			"User-Agent":      "TestBot/1.0",
			"X-Custom-Header": "custom-value",
		},
	})
	require.NoError(t, err)

	_, err = fetcher.FetchWithOptions(context.Background(), server.URL, nil)
	require.NoError(t, err)

	assert.Equal(t, "TestBot/1.0", receivedUA)
	assert.Equal(t, "custom-value", receivedCustom)
}

// TestFetcherMaxBodySize verifies body size limit is enforced.
func TestFetcherMaxBodySize(t *testing.T) {
	largeBody := strings.Repeat("a", 2000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(largeBody))
	}))
	defer server.Close()

	fetcher, err := New(config.FetchConfig{
		MaxBodySize: 1000, // 1KB limit
	})
	require.NoError(t, err)

	_, err = fetcher.FetchWithOptions(context.Background(), server.URL, nil)

	assert.Error(t, err, "should fail when body exceeds max size")
	assert.Contains(t, err.Error(), "exceeds maximum size", "error should mention size limit")
}

// TestFetcherTimeout verifies timeout is enforced.
func TestFetcherTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	fetcher, err := New(config.FetchConfig{
		Timeout: 50 * time.Millisecond,
	})
	require.NoError(t, err)

	_, err = fetcher.FetchWithOptions(context.Background(), server.URL, nil)

	assert.Error(t, err, "should timeout")
}

// TestFetcherContextCancellation verifies context cancellation stops request.
func TestFetcherContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	fetcher, err := New(config.FetchConfig{})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err = fetcher.FetchWithOptions(ctx, server.URL, nil)

	assert.Error(t, err, "should fail on context cancellation")
}

// TestFetcherURLRewrites verifies URL rewriting works.
func TestFetcherURLRewrites(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rewritten" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Rewritten URL"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher, err := New(config.FetchConfig{
		URLRewrites: []config.URLRewrite{
			{
				Type:        "literal",
				Pattern:     "/original",
				Replacement: "/rewritten",
			},
		},
	})
	require.NoError(t, err)

	resp, err := fetcher.FetchWithOptions(context.Background(), server.URL+"/original", nil)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Rewritten URL", string(resp.Body))
}

// TestFetcherRegexRewrites verifies regex URL rewriting.
func TestFetcherRegexRewrites(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/resource" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("v2 endpoint"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	fetcher, err := New(config.FetchConfig{
		URLRewrites: []config.URLRewrite{
			{
				Type:        "regex",
				Pattern:     "/api/v1/(.+)",
				Replacement: "/api/v2/$1",
			},
		},
	})
	require.NoError(t, err)

	resp, err := fetcher.FetchWithOptions(context.Background(), server.URL+"/api/v1/resource", nil)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "v2 endpoint", string(resp.Body))
}

// TestFetcherInvalidRegexRewrite verifies invalid regex patterns fail at creation.
func TestFetcherInvalidRegexRewrite(t *testing.T) {
	_, err := New(config.FetchConfig{
		URLRewrites: []config.URLRewrite{
			{
				Type:        "regex",
				Pattern:     "[invalid(regex",
				Replacement: "/replacement",
			},
		},
	})

	assert.Error(t, err, "should fail with invalid regex pattern")
	assert.Contains(t, err.Error(), "invalid regex pattern")
}

// TestFetcherAlternativeFormats verifies trying alternative formats.
func TestFetcherAlternativeFormats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/page.md" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("# Markdown content"))
			return
		}
		if r.URL.Path == "/page" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}))
	defer server.Close()

	fetcher, err := New(config.FetchConfig{
		CheckFormats: []string{".md"},
	})
	require.NoError(t, err)

	resp, err := fetcher.FetchWithOptions(context.Background(), server.URL+"/page", nil)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "# Markdown content", string(resp.Body))
	assert.Contains(t, resp.URL, "/page.md", "should have tried .md format")
}

// TestFetcherSSRFProtection verifies SSRF protection blocks private IPs.
func TestFetcherSSRFProtection(t *testing.T) {
	enableSSRF := true
	fetcher, err := New(config.FetchConfig{
		EnableSSRFProtection: &enableSSRF,
	})
	require.NoError(t, err)

	// Try to fetch localhost (should be blocked)
	_, err = fetcher.FetchWithOptions(context.Background(), "http://127.0.0.1", nil)
	assert.Error(t, err, "should block localhost")
	assert.Contains(t, err.Error(), "private", "error should mention private IP")

	// Try to fetch private IP (should be blocked)
	_, err = fetcher.FetchWithOptions(context.Background(), "http://192.168.1.1", nil)
	assert.Error(t, err, "should block private IP")
}

// TestFetcherSSRFProtectionDisabled verifies SSRF protection can be disabled.
func TestFetcherSSRFProtectionDisabled(t *testing.T) {
	enableSSRF := false
	fetcher, err := New(config.FetchConfig{
		EnableSSRFProtection: &enableSSRF,
	})
	require.NoError(t, err)

	// When SSRF protection is disabled, this will fail with connection error
	// not SSRF error (which is expected - we're just testing the protection isn't blocking)
	_, err = fetcher.FetchWithOptions(context.Background(), "http://127.0.0.1:9999", nil)
	if err != nil {
		assert.NotContains(t, err.Error(), "private", "should not block with SSRF message when disabled")
	}
}

// TestFetcherResponseMetadata verifies response includes all necessary metadata.
func TestFetcherResponseMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html>Test</html>"))
	}))
	defer server.Close()

	fetcher, err := New(config.FetchConfig{})
	require.NoError(t, err)

	resp, err := fetcher.FetchWithOptions(context.Background(), server.URL, nil)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, server.URL, resp.URL)
	assert.NotNil(t, resp.Headers)
	assert.Equal(t, "text/html", resp.Headers.Get("Content-Type"))
	assert.Equal(t, "Wed, 21 Oct 2015 07:28:00 GMT", resp.Headers.Get("Last-Modified"))
	assert.NotEmpty(t, resp.Body)
}

// TestFetcherGetHTTPClient verifies HTTP client can be retrieved.
func TestFetcherGetHTTPClient(t *testing.T) {
	fetcher, err := New(config.FetchConfig{})
	require.NoError(t, err)

	client := fetcher.GetHTTPClient()
	assert.NotNil(t, client, "should return HTTP client")
}
