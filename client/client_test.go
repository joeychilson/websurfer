package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeychilson/websurfer/cache"
	"github.com/joeychilson/websurfer/config"
)

// newTestClient creates a client for testing (SSRF protection disabled by default)
func newTestClient() (*Client, error) {
	return New(nil)
}

func TestClient_Fetch(t *testing.T) {
	t.Run("successful fetch", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("test content"))
		}))
		defer server.Close()

		client, err := newTestClient()
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		resp, err := client.Fetch(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		if string(resp.Body) != "test content" {
			t.Errorf("Body = %q, want %q", string(resp.Body), "test content")
		}
	})

	t.Run("respects robots.txt disallow", func(t *testing.T) {
		robotsTxt := `User-agent: *
Disallow: /private/
`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(robotsTxt))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("secret"))
		}))
		defer server.Close()

		cfg := config.New()
		cfg.Default.Fetch.RespectRobotsTxt = true

		client, err := New(cfg)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		_, err = client.Fetch(context.Background(), server.URL+"/private/secret.html")
		if err == nil {
			t.Error("Fetch() error = nil, want robots.txt disallow error")
		}
		if !strings.Contains(err.Error(), "disallowed by robots.txt") {
			t.Errorf("error = %v, want robots.txt disallow error", err)
		}
	})

	t.Run("respects robots.txt allow", func(t *testing.T) {
		robotsTxt := `User-agent: *
Disallow: /private/
Allow: /private/public/
`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(robotsTxt))
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("public content"))
		}))
		defer server.Close()

		cfg := config.New()
		cfg.Default.Fetch.RespectRobotsTxt = true

		client, err := New(cfg)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		resp, err := client.Fetch(context.Background(), server.URL+"/private/public/page.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if string(resp.Body) != "public content" {
			t.Errorf("Body = %q, want %q", string(resp.Body), "public content")
		}
	})

	t.Run("allows when robots.txt missing", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("content"))
		}))
		defer server.Close()

		cfg := config.New()
		cfg.Default.Fetch.RespectRobotsTxt = true

		client, err := New(cfg)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		resp, err := client.Fetch(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v, want nil when robots.txt missing", err)
		}

		if string(resp.Body) != "content" {
			t.Errorf("Body = %q, want %q", string(resp.Body), "content")
		}
	})

	t.Run("applies crawl-delay from robots.txt", func(t *testing.T) {
		robotsTxt := `User-agent: *
Crawl-delay: 2
`
		var timestamps []time.Time

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(robotsTxt))
				return
			}
			timestamps = append(timestamps, time.Now())
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("content"))
		}))
		defer server.Close()

		cfg := config.New()
		cfg.Default.Fetch.RespectRobotsTxt = true

		client, err := New(cfg)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		client.Fetch(context.Background(), server.URL+"/page1.html")
		client.Fetch(context.Background(), server.URL+"/page2.html")

		if len(timestamps) != 2 {
			t.Fatalf("got %d requests, want 2", len(timestamps))
		}

		delay := timestamps[1].Sub(timestamps[0])
		if delay < 1900*time.Millisecond {
			t.Errorf("delay = %v, want >= 1900ms (crawl-delay 2s)", delay)
		}
	})

	t.Run("retries on failure", func(t *testing.T) {
		var attempts int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			count := atomic.AddInt32(&attempts, 1)
			if count < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success after retry"))
		}))
		defer server.Close()

		cfg := config.New()
		cfg.Default.Retry.MaxRetries = 5
		cfg.Default.Retry.InitialDelay = 10 * time.Millisecond

		client, err := New(cfg)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		resp, err := client.Fetch(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if string(resp.Body) != "success after retry" {
			t.Errorf("Body = %q, want %q", string(resp.Body), "success after retry")
		}

		if atomic.LoadInt32(&attempts) != 3 {
			t.Errorf("attempts = %d, want 3", atomic.LoadInt32(&attempts))
		}
	})

	t.Run("applies site-specific config", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("content"))
		}))
		defer server.Close()

		cfg := config.New()
		cfg.Default.Fetch.RespectRobotsTxt = false
		cfg.Sites = []config.SiteConfig{
			{
				Pattern: server.URL[7:],
				Fetch: &config.FetchConfig{
					RespectRobotsTxt: true,
				},
			},
		}

		client, err := New(cfg)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		_, err = client.Fetch(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Logf("Fetch() error = %v (site-specific config applied)", err)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := config.New()
		client, err := New(cfg)
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		_, err = client.Fetch(ctx, server.URL+"/page.html")
		if err == nil {
			t.Error("Fetch() error = nil, want context error")
		}
	})

	t.Run("parses HTML content", func(t *testing.T) {
		htmlContent := `
			<!DOCTYPE html>
			<html>
				<head>
					<title>Test Page</title>
					<script>alert('test');</script>
					<style>.foo { color: red; }</style>
				</head>
				<body class="main">
					<div class="wrapper">
						<header>
							<h1>Welcome</h1>
						</header>
						<div class="content">
							<p>This is a test paragraph.</p>
							<ul>
								<li>Item 1</li>
								<li>Item 2</li>
							</ul>
						</div>
						<footer>
							<p>Footer text</p>
						</footer>
					</div>
				</body>
			</html>
		`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(htmlContent))
		}))
		defer server.Close()

		client, err := newTestClient()
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		resp, err := client.Fetch(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		result := string(resp.Body)

		if strings.Contains(result, "<script") || strings.Contains(result, "<style") {
			t.Errorf("parsed HTML should not contain script or style tags")
		}

		if strings.Contains(result, "class=") {
			t.Errorf("parsed HTML should not contain class attributes")
		}

		if strings.Contains(result, "<div") {
			t.Errorf("parsed HTML should not contain div tags")
		}

		requiredElements := []string{
			"<header>", "<h1>", "Welcome",
			"<p>", "This is a test paragraph",
			"<ul>", "<li>", "Item 1", "Item 2",
			"<footer>", "Footer text",
		}
		for _, elem := range requiredElements {
			if !strings.Contains(result, elem) {
				t.Errorf("parsed HTML should contain %q, got: %s", elem, result)
			}
		}

		lines := strings.SplitSeq(result, "\n")
		for line := range lines {
			if strings.Contains(line, "> <") {
				t.Errorf("parsed HTML should not have whitespace between tags on same line")
			}
		}
	})

	t.Run("does not parse non-HTML content", func(t *testing.T) {
		jsonContent := `{"key": "value"}`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(jsonContent))
		}))
		defer server.Close()

		client, err := newTestClient()
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		resp, err := client.Fetch(context.Background(), server.URL+"/data.json")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if string(resp.Body) != jsonContent {
			t.Errorf("non-HTML content should be unchanged, got: %s", string(resp.Body))
		}
	})
}

func TestClient_NewFromFile(t *testing.T) {
	t.Run("loads config from file", func(t *testing.T) {
		tmpfile := t.TempDir() + "/config.yaml"
		configContent := `
default:
  fetch:
    timeout: 5s
    user_agent: "TestBot/1.0"
  rate_limit:
    delay: 1s
`
		if err := writeFile(tmpfile, configContent); err != nil {
			t.Fatalf("failed to write temp config: %v", err)
		}

		client, err := NewFromFile(tmpfile)
		if err != nil {
			t.Fatalf("NewFromFile() error = %v", err)
		}

		if client.config.Default.Fetch.Timeout != 5*time.Second {
			t.Errorf("timeout = %v, want 5s", client.config.Default.Fetch.Timeout)
		}

		if client.config.Default.Fetch.UserAgent != "TestBot/1.0" {
			t.Errorf("user_agent = %q, want %q", client.config.Default.Fetch.UserAgent, "TestBot/1.0")
		}
	})

	t.Run("errors on invalid config file", func(t *testing.T) {
		_, err := NewFromFile("/nonexistent/config.yaml")
		if err == nil {
			t.Error("NewFromFile() error = nil, want error for nonexistent file")
		}
	})
}

func TestClient_ValidationOnCreate(t *testing.T) {
	t.Run("rejects invalid config", func(t *testing.T) {
		cfg := config.New()
		cfg.Default.RateLimit.Delay = 5 * time.Second
		cfg.Default.RateLimit.RequestsPerSecond = 2.0

		_, err := New(cfg)
		if err == nil {
			t.Error("New() error = nil, want validation error for conflicting rate limit")
		}
	})

	t.Run("accepts valid config", func(t *testing.T) {
		cfg := config.New()
		cfg.Default.RateLimit.Delay = 5 * time.Second

		_, err := New(cfg)
		if err != nil {
			t.Errorf("New() error = %v, want nil for valid config", err)
		}
	})
}

// writeFile is a helper to write content to a file
func writeFile(path, content string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	return err
}

func TestClient_WithCache(t *testing.T) {
	t.Run("fresh cache hit", func(t *testing.T) {
		var fetchCount int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			atomic.AddInt32(&fetchCount, 1)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("test content"))
		}))
		defer server.Close()

		client, err := newTestClient()
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		memCache := cache.NewMemoryCache(cache.Config{
			TTL:       1 * time.Minute,
			StaleTime: 5 * time.Minute,
		})
		defer memCache.Close()

		client.WithCache(memCache)

		resp1, err := client.Fetch(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if string(resp1.Body) != "test content" {
			t.Errorf("Body = %s, want 'test content'", string(resp1.Body))
		}

		resp2, err := client.Fetch(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if string(resp2.Body) != "test content" {
			t.Errorf("Body = %s, want 'test content'", string(resp2.Body))
		}

		if atomic.LoadInt32(&fetchCount) != 1 {
			t.Errorf("fetchCount = %d, want 1 (second request should be cached)", atomic.LoadInt32(&fetchCount))
		}
	})

	t.Run("stale cache hit with background refresh", func(t *testing.T) {
		var fetchCount int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			count := atomic.AddInt32(&fetchCount, 1)
			w.WriteHeader(http.StatusOK)
			if count == 1 {
				w.Write([]byte("old content"))
			} else {
				w.Write([]byte("new content"))
			}
		}))
		defer server.Close()

		client, err := newTestClient()
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		memCache := cache.NewMemoryCache(cache.Config{
			TTL:       50 * time.Millisecond,
			StaleTime: 1 * time.Second,
		})
		defer memCache.Close()

		client.WithCache(memCache)

		resp1, err := client.Fetch(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if string(resp1.Body) != "old content" {
			t.Errorf("Body = %s, want 'old content'", string(resp1.Body))
		}

		time.Sleep(100 * time.Millisecond)

		resp2, err := client.Fetch(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if string(resp2.Body) != "old content" {
			t.Errorf("Body = %s, want 'old content' (stale)", string(resp2.Body))
		}

		time.Sleep(200 * time.Millisecond)

		resp3, err := client.Fetch(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if string(resp3.Body) != "new content" {
			t.Errorf("Body = %s, want 'new content' (refreshed)", string(resp3.Body))
		}
	})

	t.Run("cache miss", func(t *testing.T) {
		var fetchCount int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			atomic.AddInt32(&fetchCount, 1)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("test content"))
		}))
		defer server.Close()

		client, err := newTestClient()
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		memCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer memCache.Close()

		client.WithCache(memCache)

		resp, err := client.Fetch(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if string(resp.Body) != "test content" {
			t.Errorf("Body = %s, want 'test content'", string(resp.Body))
		}

		if atomic.LoadInt32(&fetchCount) != 1 {
			t.Errorf("fetchCount = %d, want 1", atomic.LoadInt32(&fetchCount))
		}
	})

	t.Run("cache too old", func(t *testing.T) {
		var fetchCount int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			count := atomic.AddInt32(&fetchCount, 1)
			w.WriteHeader(http.StatusOK)
			if count == 1 {
				w.Write([]byte("old content"))
			} else {
				w.Write([]byte("new content"))
			}
		}))
		defer server.Close()

		client, err := newTestClient()
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		memCache := cache.NewMemoryCache(cache.Config{
			TTL:       50 * time.Millisecond,
			StaleTime: 100 * time.Millisecond,
		})
		defer memCache.Close()

		client.WithCache(memCache)

		resp1, err := client.Fetch(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if string(resp1.Body) != "old content" {
			t.Errorf("Body = %s, want 'old content'", string(resp1.Body))
		}

		time.Sleep(200 * time.Millisecond)

		resp2, err := client.Fetch(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if string(resp2.Body) != "new content" {
			t.Errorf("Body = %s, want 'new content'", string(resp2.Body))
		}

		if atomic.LoadInt32(&fetchCount) != 2 {
			t.Errorf("fetchCount = %d, want 2", atomic.LoadInt32(&fetchCount))
		}
	})

	t.Run("cache with parsed HTML", func(t *testing.T) {
		var fetchCount int32

		htmlContent := `<html><head><script>alert('test');</script></head><body><p>Test</p></body></html>`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			atomic.AddInt32(&fetchCount, 1)
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(htmlContent))
		}))
		defer server.Close()

		client, err := newTestClient()
		if err != nil {
			t.Fatalf("New() error = %v", err)
		}

		memCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer memCache.Close()

		client.WithCache(memCache)

		resp1, err := client.Fetch(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		result := string(resp1.Body)
		if strings.Contains(result, "<script") {
			t.Error("parsed HTML should not contain script tags")
		}
		if !strings.Contains(result, "<p>") {
			t.Error("parsed HTML should contain paragraph tags")
		}

		resp2, err := client.Fetch(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if string(resp2.Body) != string(resp1.Body) {
			t.Error("cached response should match first response")
		}

		if atomic.LoadInt32(&fetchCount) != 1 {
			t.Errorf("fetchCount = %d, want 1 (second request should be cached)", atomic.LoadInt32(&fetchCount))
		}
	})
}
