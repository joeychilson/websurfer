package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/joeychilson/websurfer/config"
)

func TestFetcher_Fetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/success":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success content"))
		case "/llms.txt":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("llms content"))
		case "/page.md":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("markdown content"))
		case "/404":
			w.WriteHeader(http.StatusNotFound)
		case "/custom-header":
			if r.Header.Get("X-Custom") == "test-value" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("got custom header"))
			} else {
				w.WriteHeader(http.StatusBadRequest)
			}
		case "/user-agent":
			ua := r.Header.Get("User-Agent")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(ua))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Run("successful fetch", func(t *testing.T) {
		fetcher := New(config.FetchConfig{})
		resp, err := fetcher.Fetch(context.Background(), server.URL+"/success")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		if string(resp.Body) != "success content" {
			t.Errorf("Body = %q, want %q", string(resp.Body), "success content")
		}
	})

	t.Run("404 error", func(t *testing.T) {
		fetcher := New(config.FetchConfig{})
		_, err := fetcher.Fetch(context.Background(), server.URL+"/404")
		if err == nil {
			t.Error("Fetch() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "HTTP 404") {
			t.Errorf("error = %v, want HTTP 404 error", err)
		}
	})

	t.Run("custom headers", func(t *testing.T) {
		fetcher := New(config.FetchConfig{
			Headers: map[string]string{
				"X-Custom": "test-value",
			},
		})
		resp, err := fetcher.Fetch(context.Background(), server.URL+"/custom-header")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if string(resp.Body) != "got custom header" {
			t.Errorf("Body = %q, want %q", string(resp.Body), "got custom header")
		}
	})

	t.Run("custom user agent", func(t *testing.T) {
		fetcher := New(config.FetchConfig{
			UserAgent: "CustomBot/1.0",
		})
		resp, err := fetcher.Fetch(context.Background(), server.URL+"/user-agent")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if string(resp.Body) != "CustomBot/1.0" {
			t.Errorf("User-Agent = %q, want %q", string(resp.Body), "CustomBot/1.0")
		}
	})

	t.Run("default user agent", func(t *testing.T) {
		fetcher := New(config.FetchConfig{})
		resp, err := fetcher.Fetch(context.Background(), server.URL+"/user-agent")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if string(resp.Body) != config.DefaultUserAgent {
			t.Errorf("User-Agent = %q, want %q", string(resp.Body), config.DefaultUserAgent)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer slowServer.Close()

		fetcher := New(config.FetchConfig{
			Timeout: 50 * time.Millisecond,
		})

		_, err := fetcher.Fetch(context.Background(), slowServer.URL)
		if err == nil {
			t.Error("Fetch() error = nil, want timeout error")
		}
		if !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "Client.Timeout") {
			t.Errorf("error = %v, want timeout error", err)
		}
	})
}

func TestFetcher_CheckFormats(t *testing.T) {
	callCount := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount[r.URL.Path]++

		switch r.URL.Path {
		case "/page":
			w.WriteHeader(http.StatusNotFound)
		case "/llms.txt":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("llms content"))
		case "/page.md":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("markdown"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Run("tries alternative formats", func(t *testing.T) {
		fetcher := New(config.FetchConfig{
			CheckFormats: []string{"/llms.txt", ".md"},
		})

		resp, err := fetcher.Fetch(context.Background(), server.URL+"/page")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if string(resp.Body) != "llms content" {
			t.Errorf("Body = %q, want %q", string(resp.Body), "llms content")
		}

		if resp.URL != server.URL+"/llms.txt" {
			t.Errorf("URL = %q, want %q", resp.URL, server.URL+"/llms.txt")
		}
	})

	t.Run("tries formats in order", func(t *testing.T) {
		callCount = make(map[string]int)

		fetcher := New(config.FetchConfig{
			CheckFormats: []string{".md", "/llms.txt"},
		})

		resp, err := fetcher.Fetch(context.Background(), server.URL+"/page")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if string(resp.Body) != "markdown" {
			t.Errorf("Body = %q, want markdown", string(resp.Body))
		}

		if callCount["/page.md"] != 1 {
			t.Errorf("expected /page.md to be called once, got call counts: %v", callCount)
		}

		if callCount["/page"] != 0 {
			t.Errorf("expected /page to not be called, got call counts: %v", callCount)
		}
	})
}

func TestFetcher_URLRewrites(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(r.URL.Path))
	}))
	defer server.Close()

	t.Run("literal rewrite", func(t *testing.T) {
		fetcher := New(config.FetchConfig{
			URLRewrites: []config.URLRewrite{
				{
					Type:        "literal",
					Pattern:     ".html",
					Replacement: ".md",
				},
			},
		})

		resp, err := fetcher.Fetch(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if string(resp.Body) != "/page.md" {
			t.Errorf("rewrote URL path = %q, want /page.md", string(resp.Body))
		}
	})

	t.Run("regex rewrite", func(t *testing.T) {
		fetcher := New(config.FetchConfig{
			URLRewrites: []config.URLRewrite{
				{
					Type:        "regex",
					Pattern:     `/docs/(.+)\.html$`,
					Replacement: "/api/$1.json",
				},
			},
		})

		resp, err := fetcher.Fetch(context.Background(), server.URL+"/docs/user.html")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if string(resp.Body) != "/api/user.json" {
			t.Errorf("rewrote URL path = %q, want /api/user.json", string(resp.Body))
		}
	})

	t.Run("multiple rewrites", func(t *testing.T) {
		fetcher := New(config.FetchConfig{
			URLRewrites: []config.URLRewrite{
				{Pattern: "old", Replacement: "new"},
				{Pattern: "page", Replacement: "document"},
			},
		})

		resp, err := fetcher.Fetch(context.Background(), server.URL+"/old/page")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if string(resp.Body) != "/new/document" {
			t.Errorf("rewrote URL path = %q, want /new/document", string(resp.Body))
		}
	})
}

func TestFetcher_Redirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		if r.URL.Path == "/final" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("final content"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Run("follows redirects when enabled", func(t *testing.T) {
		fetcher := New(config.FetchConfig{
			FollowRedirects: true,
		})

		resp, err := fetcher.Fetch(context.Background(), server.URL+"/redirect")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if string(resp.Body) != "final content" {
			t.Errorf("Body = %q, want final content", string(resp.Body))
		}

		if !strings.HasSuffix(resp.URL, "/final") {
			t.Errorf("URL = %q, want to end with /final", resp.URL)
		}
	})

	t.Run("respects FollowRedirects=false", func(t *testing.T) {
		fetcher := New(config.FetchConfig{
			FollowRedirects: false,
		})

		resp, err := fetcher.Fetch(context.Background(), server.URL+"/redirect")
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if resp.StatusCode != http.StatusFound {
			t.Errorf("StatusCode = %d, want %d (redirect not followed)", resp.StatusCode, http.StatusFound)
		}
	})

	t.Run("respects MaxRedirects", func(t *testing.T) {
		redirectServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/r") {
				num := r.URL.Path[2:]
				if num != "5" {
					http.Redirect(w, r, "/r"+num+"1", http.StatusFound)
					return
				}
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer redirectServer.Close()

		fetcher := New(config.FetchConfig{
			FollowRedirects: true,
			MaxRedirects:    2,
		})

		_, err := fetcher.Fetch(context.Background(), redirectServer.URL+"/r0")
		if err == nil {
			t.Error("Fetch() error = nil, want error for too many redirects")
		}
		if !strings.Contains(err.Error(), "redirect") {
			t.Errorf("error = %v, want redirect error", err)
		}
	})
}

func TestFetcher_Context(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		fetcher := New(config.FetchConfig{})

		_, err := fetcher.Fetch(ctx, server.URL)
		if err == nil {
			t.Error("Fetch() error = nil, want context cancelled error")
		}
	})
}
