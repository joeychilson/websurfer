package robots

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joeychilson/websurfer/cache"
)

func TestChecker_IsAllowed(t *testing.T) {
	robotsTxt := `User-agent: *
Disallow: /private/
Disallow: /admin
Allow: /private/public/

User-agent: BadBot
Disallow: /
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Run("allows public path", func(t *testing.T) {
		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)
		allowed, err := checker.IsAllowed(context.Background(), server.URL+"/public/page.html")
		if err != nil {
			t.Fatalf("IsAllowed() error = %v", err)
		}
		if !allowed {
			t.Error("IsAllowed() = false, want true for public path")
		}
	})

	t.Run("disallows private path", func(t *testing.T) {
		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)
		allowed, err := checker.IsAllowed(context.Background(), server.URL+"/private/secret.html")
		if err != nil {
			t.Fatalf("IsAllowed() error = %v", err)
		}
		if allowed {
			t.Error("IsAllowed() = true, want false for private path")
		}
	})

	t.Run("allows explicitly allowed path in disallowed directory", func(t *testing.T) {
		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)
		allowed, err := checker.IsAllowed(context.Background(), server.URL+"/private/public/page.html")
		if err != nil {
			t.Fatalf("IsAllowed() error = %v", err)
		}
		if !allowed {
			t.Error("IsAllowed() = false, want true for explicitly allowed path")
		}
	})

	t.Run("disallows admin path", func(t *testing.T) {
		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)
		allowed, err := checker.IsAllowed(context.Background(), server.URL+"/admin/users")
		if err != nil {
			t.Fatalf("IsAllowed() error = %v", err)
		}
		if allowed {
			t.Error("IsAllowed() = true, want false for admin path")
		}
	})

	t.Run("allows root path", func(t *testing.T) {
		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)
		allowed, err := checker.IsAllowed(context.Background(), server.URL+"/")
		if err != nil {
			t.Fatalf("IsAllowed() error = %v", err)
		}
		if !allowed {
			t.Error("IsAllowed() = false, want true for root path")
		}
	})
}

func TestChecker_UserAgent(t *testing.T) {
	robotsTxt := `User-agent: GoodBot
Disallow: /restricted/

User-agent: *
Disallow: /private/
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Run("specific user agent rules apply", func(t *testing.T) {
		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("GoodBot/1.0", 24*time.Hour, testCache, nil)
		allowed, _ := checker.IsAllowed(context.Background(), server.URL+"/restricted/page.html")
		if allowed {
			t.Error("IsAllowed() = true, want false for GoodBot on restricted path")
		}

		allowed, _ = checker.IsAllowed(context.Background(), server.URL+"/private/page.html")
		if !allowed {
			t.Error("IsAllowed() = false, want true for GoodBot on private path (not in its rules)")
		}
	})

	t.Run("wildcard user agent rules apply", func(t *testing.T) {
		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("OtherBot/1.0", 24*time.Hour, testCache, nil)
		allowed, _ := checker.IsAllowed(context.Background(), server.URL+"/private/page.html")
		if allowed {
			t.Error("IsAllowed() = true, want false for wildcard rule on private path")
		}
	})
}

func TestChecker_MissingRobotsTxt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Run("allows all when robots.txt not found", func(t *testing.T) {
		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)
		allowed, err := checker.IsAllowed(context.Background(), server.URL+"/any/path")
		if err != nil {
			t.Fatalf("IsAllowed() error = %v", err)
		}
		if !allowed {
			t.Error("IsAllowed() = false, want true when robots.txt missing")
		}
	})
}

func TestChecker_CrawlDelay(t *testing.T) {
	robotsTxt := `User-agent: *
Crawl-delay: 5
Disallow: /private/
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Run("parses crawl delay", func(t *testing.T) {
		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)
		delay, err := checker.GetCrawlDelay(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("GetCrawlDelay() error = %v", err)
		}
		if delay != 5*time.Second {
			t.Errorf("GetCrawlDelay() = %v, want 5s", delay)
		}
	})
}

func TestChecker_Cache(t *testing.T) {
	var fetchCount int
	robotsTxt := `User-agent: *
Disallow: /private/
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			fetchCount++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Run("caches robots.txt", func(t *testing.T) {
		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)

		for i := 0; i < 5; i++ {
			checker.IsAllowed(context.Background(), server.URL+"/page.html")
		}

		if fetchCount != 1 {
			t.Errorf("fetch count = %d, want 1 (should use cache)", fetchCount)
		}
	})
}

func TestChecker_PathMatching(t *testing.T) {
	tests := []struct {
		name      string
		robotsTxt string
		path      string
		allowed   bool
	}{
		{
			name: "exact match with $",
			robotsTxt: `User-agent: *
Disallow: /page.html$
`,
			path:    "/page.html",
			allowed: false,
		},
		{
			name: "prefix match without $",
			robotsTxt: `User-agent: *
Disallow: /page.html$
`,
			path:    "/page.html?query=1",
			allowed: true,
		},
		{
			name: "wildcard match",
			robotsTxt: `User-agent: *
Disallow: /private/*.pdf
`,
			path:    "/private/document.pdf",
			allowed: false,
		},
		{
			name: "wildcard no match",
			robotsTxt: `User-agent: *
Disallow: /private/*.pdf
`,
			path:    "/private/document.html",
			allowed: true,
		},
		{
			name: "root disallow",
			robotsTxt: `User-agent: *
Disallow: /
`,
			path:    "/any/path",
			allowed: false,
		},
		{
			name: "empty disallow allows all",
			robotsTxt: `User-agent: *
Disallow:
`,
			path:    "/any/path",
			allowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/robots.txt" {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(tt.robotsTxt))
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()

			testCache := cache.NewMemoryCache(cache.DefaultConfig())
			defer testCache.Close()
			checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)
			allowed, err := checker.IsAllowed(context.Background(), server.URL+tt.path)
			if err != nil {
				t.Fatalf("IsAllowed() error = %v", err)
			}
			if allowed != tt.allowed {
				t.Errorf("IsAllowed() = %v, want %v", allowed, tt.allowed)
			}
		})
	}
}

func TestChecker_MostSpecificRule(t *testing.T) {
	robotsTxt := `User-agent: *
Disallow: /documents/
Allow: /documents/public/
Disallow: /documents/public/secret/
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tests := []struct {
		path    string
		allowed bool
		reason  string
	}{
		{"/documents/private.pdf", false, "disallow /documents/"},
		{"/documents/public/info.pdf", true, "allow /documents/public/"},
		{"/documents/public/secret/key.pdf", false, "disallow /documents/public/secret/"},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			testCache := cache.NewMemoryCache(cache.DefaultConfig())
			defer testCache.Close()
			checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)
			allowed, err := checker.IsAllowed(context.Background(), server.URL+tt.path)
			if err != nil {
				t.Fatalf("IsAllowed() error = %v", err)
			}
			if allowed != tt.allowed {
				t.Errorf("IsAllowed(%s) = %v, want %v (%s)", tt.path, allowed, tt.allowed, tt.reason)
			}
		})
	}
}

func TestChecker_ConcurrentAccess(t *testing.T) {
	robotsTxt := `User-agent: *
Disallow: /private/
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Run("thread-safe cache access", func(t *testing.T) {
		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				checker.IsAllowed(context.Background(), server.URL+"/page.html")
			}()
		}

		wg.Wait()
	})
}

func TestParseRobotsTxt(t *testing.T) {
	t.Run("handles comments", func(t *testing.T) {
		content := `# This is a comment
User-agent: *
# Another comment
Disallow: /private/
`
		rules, _, _, err := parseRobotsTxt(strings.NewReader(content), "TestBot")
		if err != nil {
			t.Fatalf("parseRobotsTxt() error = %v", err)
		}
		if len(rules.Disallows) != 1 || rules.Disallows[0] != "/private/" {
			t.Errorf("disallows = %v, want [/private/]", rules.Disallows)
		}
	})

	t.Run("handles empty lines", func(t *testing.T) {
		content := `User-agent: *

Disallow: /private/

Allow: /public/
`
		rules, _, _, err := parseRobotsTxt(strings.NewReader(content), "TestBot")
		if err != nil {
			t.Fatalf("parseRobotsTxt() error = %v", err)
		}
		if len(rules.Disallows) != 1 || len(rules.Allows) != 1 {
			t.Errorf("got %d disallows and %d allows, want 1 and 1", len(rules.Disallows), len(rules.Allows))
		}
	})

	t.Run("handles malformed lines", func(t *testing.T) {
		content := `User-agent: *
Disallow /private/
Disallow: /admin/
Invalid line here
`
		rules, _, _, err := parseRobotsTxt(strings.NewReader(content), "TestBot")
		if err != nil {
			t.Fatalf("parseRobotsTxt() error = %v", err)
		}
		if len(rules.Disallows) != 1 || rules.Disallows[0] != "/admin/" {
			t.Errorf("disallows = %v, want [/admin/]", rules.Disallows)
		}
	})
}

func TestMatchesPath(t *testing.T) {
	tests := []struct {
		path    string
		pattern string
		matches bool
	}{
		{"/page.html", "/page.html", true},
		{"/page.html", "/page", true},
		{"/page.html", "/other", false},
		{"/page.html", "/page.html$", true},
		{"/page.html?query=1", "/page.html$", false},
		{"/private/doc.pdf", "/private/*.pdf", true},
		{"/private/doc.html", "/private/*.pdf", false},
		{"/", "/", true},
		{"/anything", "/", true},
	}

	for _, tt := range tests {
		t.Run(tt.path+" vs "+tt.pattern, func(t *testing.T) {
			result := matchesPath(tt.path, tt.pattern)
			if result != tt.matches {
				t.Errorf("matchesPath(%q, %q) = %v, want %v", tt.path, tt.pattern, result, tt.matches)
			}
		})
	}
}

func TestChecker_GetSitemaps(t *testing.T) {
	t.Run("single sitemap", func(t *testing.T) {
		robotsTxt := `User-agent: *
Disallow: /private/
Sitemap: https://example.com/sitemap.xml
`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(robotsTxt))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)

		sitemaps, err := checker.GetSitemaps(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("GetSitemaps() error = %v", err)
		}
		if len(sitemaps) != 1 {
			t.Fatalf("GetSitemaps() returned %d sitemaps, want 1", len(sitemaps))
		}
		if sitemaps[0] != "https://example.com/sitemap.xml" {
			t.Errorf("GetSitemaps() = %v, want [https://example.com/sitemap.xml]", sitemaps)
		}
	})

	t.Run("multiple sitemaps", func(t *testing.T) {
		robotsTxt := `User-agent: *
Disallow: /private/
Sitemap: https://example.com/sitemap1.xml
Sitemap: https://example.com/sitemap2.xml
Sitemap: https://example.com/sitemap-index.xml
`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(robotsTxt))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)

		sitemaps, err := checker.GetSitemaps(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("GetSitemaps() error = %v", err)
		}
		if len(sitemaps) != 3 {
			t.Fatalf("GetSitemaps() returned %d sitemaps, want 3", len(sitemaps))
		}
		expected := []string{
			"https://example.com/sitemap1.xml",
			"https://example.com/sitemap2.xml",
			"https://example.com/sitemap-index.xml",
		}
		for i, want := range expected {
			if sitemaps[i] != want {
				t.Errorf("GetSitemaps()[%d] = %v, want %v", i, sitemaps[i], want)
			}
		}
	})

	t.Run("no sitemaps", func(t *testing.T) {
		robotsTxt := `User-agent: *
Disallow: /private/
`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/robots.txt" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(robotsTxt))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)

		sitemaps, err := checker.GetSitemaps(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("GetSitemaps() error = %v", err)
		}
		if len(sitemaps) != 0 {
			t.Errorf("GetSitemaps() = %v, want empty slice", sitemaps)
		}
	})

	t.Run("robots.txt not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)

		sitemaps, err := checker.GetSitemaps(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("GetSitemaps() error = %v, want nil", err)
		}
		if sitemaps != nil {
			t.Errorf("GetSitemaps() = %v, want nil", sitemaps)
		}
	})
}

func TestChecker_SitemapsCaching(t *testing.T) {
	var fetchCount int
	robotsTxt := `User-agent: *
Disallow: /private/
Sitemap: https://example.com/sitemap.xml
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			fetchCount++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	t.Run("caches sitemaps with robots.txt", func(t *testing.T) {
		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)

		fetchCount = 0

		for i := 0; i < 5; i++ {
			sitemaps, err := checker.GetSitemaps(context.Background(), server.URL+"/page.html")
			if err != nil {
				t.Fatalf("GetSitemaps() error = %v", err)
			}
			if len(sitemaps) != 1 || sitemaps[0] != "https://example.com/sitemap.xml" {
				t.Errorf("GetSitemaps()[%d] = %v, want [https://example.com/sitemap.xml]", i, sitemaps)
			}
		}

		if fetchCount != 1 {
			t.Errorf("fetch count = %d, want 1 (should use cache)", fetchCount)
		}
	})

	t.Run("sitemaps cached with rules and crawl-delay", func(t *testing.T) {
		testCache := cache.NewMemoryCache(cache.DefaultConfig())
		defer testCache.Close()
		checker := New("TestBot/1.0", 24*time.Hour, testCache, nil)

		fetchCount = 0

		checker.IsAllowed(context.Background(), server.URL+"/page.html")
		sitemaps, err := checker.GetSitemaps(context.Background(), server.URL+"/page.html")
		if err != nil {
			t.Fatalf("GetSitemaps() error = %v", err)
		}
		if len(sitemaps) != 1 {
			t.Errorf("GetSitemaps() = %v, want 1 sitemap", sitemaps)
		}

		if fetchCount != 1 {
			t.Errorf("fetch count = %d, want 1 (sitemaps should be cached with rules)", fetchCount)
		}
	})
}

func TestParseRobotsTxt_Sitemaps(t *testing.T) {
	t.Run("single sitemap", func(t *testing.T) {
		content := `User-agent: *
Disallow: /private/
Sitemap: https://example.com/sitemap.xml
`
		_, _, sitemaps, err := parseRobotsTxt(strings.NewReader(content), "TestBot")
		if err != nil {
			t.Fatalf("parseRobotsTxt() error = %v", err)
		}
		if len(sitemaps) != 1 {
			t.Fatalf("sitemaps count = %d, want 1", len(sitemaps))
		}
		if sitemaps[0] != "https://example.com/sitemap.xml" {
			t.Errorf("sitemaps[0] = %v, want https://example.com/sitemap.xml", sitemaps[0])
		}
	})

	t.Run("multiple sitemaps", func(t *testing.T) {
		content := `User-agent: *
Disallow: /private/
Sitemap: https://example.com/sitemap1.xml
Sitemap: https://example.com/sitemap2.xml
Sitemap: https://example.com/news-sitemap.xml
`
		_, _, sitemaps, err := parseRobotsTxt(strings.NewReader(content), "TestBot")
		if err != nil {
			t.Fatalf("parseRobotsTxt() error = %v", err)
		}
		if len(sitemaps) != 3 {
			t.Fatalf("sitemaps count = %d, want 3", len(sitemaps))
		}
		expected := []string{
			"https://example.com/sitemap1.xml",
			"https://example.com/sitemap2.xml",
			"https://example.com/news-sitemap.xml",
		}
		for i, want := range expected {
			if sitemaps[i] != want {
				t.Errorf("sitemaps[%d] = %v, want %v", i, sitemaps[i], want)
			}
		}
	})

	t.Run("sitemaps mixed with rules", func(t *testing.T) {
		content := `User-agent: *
Sitemap: https://example.com/sitemap-before.xml
Disallow: /private/
Allow: /public/
Sitemap: https://example.com/sitemap-after.xml
Crawl-delay: 5
`
		rules, delay, sitemaps, err := parseRobotsTxt(strings.NewReader(content), "TestBot")
		if err != nil {
			t.Fatalf("parseRobotsTxt() error = %v", err)
		}
		if len(rules.Disallows) != 1 || rules.Disallows[0] != "/private/" {
			t.Errorf("disallows = %v, want [/private/]", rules.Disallows)
		}
		if len(rules.Allows) != 1 || rules.Allows[0] != "/public/" {
			t.Errorf("allows = %v, want [/public/]", rules.Allows)
		}
		if delay != 5*time.Second {
			t.Errorf("crawl-delay = %v, want 5s", delay)
		}
		if len(sitemaps) != 2 {
			t.Fatalf("sitemaps count = %d, want 2", len(sitemaps))
		}
		if sitemaps[0] != "https://example.com/sitemap-before.xml" {
			t.Errorf("sitemaps[0] = %v, want https://example.com/sitemap-before.xml", sitemaps[0])
		}
		if sitemaps[1] != "https://example.com/sitemap-after.xml" {
			t.Errorf("sitemaps[1] = %v, want https://example.com/sitemap-after.xml", sitemaps[1])
		}
	})

	t.Run("no sitemaps", func(t *testing.T) {
		content := `User-agent: *
Disallow: /private/
`
		_, _, sitemaps, err := parseRobotsTxt(strings.NewReader(content), "TestBot")
		if err != nil {
			t.Fatalf("parseRobotsTxt() error = %v", err)
		}
		if len(sitemaps) != 0 {
			t.Errorf("sitemaps = %v, want empty slice", sitemaps)
		}
	})

	t.Run("empty sitemap value ignored", func(t *testing.T) {
		content := `User-agent: *
Disallow: /private/
Sitemap:
Sitemap: https://example.com/valid-sitemap.xml
`
		_, _, sitemaps, err := parseRobotsTxt(strings.NewReader(content), "TestBot")
		if err != nil {
			t.Fatalf("parseRobotsTxt() error = %v", err)
		}
		if len(sitemaps) != 1 {
			t.Fatalf("sitemaps count = %d, want 1 (empty sitemap should be ignored)", len(sitemaps))
		}
		if sitemaps[0] != "https://example.com/valid-sitemap.xml" {
			t.Errorf("sitemaps[0] = %v, want https://example.com/valid-sitemap.xml", sitemaps[0])
		}
	})

	t.Run("sitemaps are global, not user-agent specific", func(t *testing.T) {
		content := `User-agent: GoodBot
Disallow: /restricted/
Sitemap: https://example.com/sitemap1.xml

User-agent: *
Disallow: /private/
Sitemap: https://example.com/sitemap2.xml
`
		_, _, sitemaps, err := parseRobotsTxt(strings.NewReader(content), "TestBot")
		if err != nil {
			t.Fatalf("parseRobotsTxt() error = %v", err)
		}
		if len(sitemaps) != 2 {
			t.Fatalf("sitemaps count = %d, want 2 (sitemaps are global)", len(sitemaps))
		}
		expected := []string{
			"https://example.com/sitemap1.xml",
			"https://example.com/sitemap2.xml",
		}
		for i, want := range expected {
			if sitemaps[i] != want {
				t.Errorf("sitemaps[%d] = %v, want %v", i, sitemaps[i], want)
			}
		}
	})
}
