package robots

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRobotsAllowsWhenNoRobotsTxt verifies we allow access when robots.txt doesn't exist.
func TestRobotsAllowsWhenNoRobotsTxt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}))
	defer server.Close()

	checker := New("TestBot", 1*time.Hour, server.Client())
	allowed, err := checker.IsAllowed(context.Background(), server.URL+"/any/path")

	require.NoError(t, err)
	assert.True(t, allowed, "should allow access when robots.txt doesn't exist")
}

// TestRobotsDisallowPath verifies Disallow rules actually block paths.
func TestRobotsDisallowPath(t *testing.T) {
	robotsTxt := `User-agent: *
Disallow: /admin/
Disallow: /private/
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
	}))
	defer server.Close()

	checker := New("TestBot", 1*time.Hour, server.Client())
	ctx := context.Background()

	tests := []struct {
		path    string
		allowed bool
	}{
		{"/", true},
		{"/public/page", true},
		{"/admin/", false},
		{"/admin/users", false},
		{"/private/data", false},
		{"/admin", true}, // doesn't match /admin/ (no trailing slash)
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			allowed, err := checker.IsAllowed(ctx, server.URL+tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.allowed, allowed, "path %s should be %v", tt.path, tt.allowed)
		})
	}
}

// TestRobotsUserAgentSpecificOverridesWildcard verifies specific user-agent rules override wildcard.
func TestRobotsUserAgentSpecificOverridesWildcard(t *testing.T) {
	robotsTxt := `User-agent: *
Disallow: /

User-agent: GoodBot
Disallow: /private/
Allow: /
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
	}))
	defer server.Close()

	// Generic bot - should be blocked everywhere
	wildcardChecker := New("SomeBot", 1*time.Hour, server.Client())
	allowed, err := wildcardChecker.IsAllowed(context.Background(), server.URL+"/public")
	require.NoError(t, err)
	assert.False(t, allowed, "wildcard user-agent should block all paths")

	// GoodBot - should have access except /private/
	specificChecker := New("GoodBot", 1*time.Hour, server.Client())
	ctx := context.Background()

	allowed, err = specificChecker.IsAllowed(ctx, server.URL+"/public")
	require.NoError(t, err)
	assert.True(t, allowed, "GoodBot should be allowed on /public")

	allowed, err = specificChecker.IsAllowed(ctx, server.URL+"/private/data")
	require.NoError(t, err)
	assert.False(t, allowed, "GoodBot should be blocked on /private/")
}

// TestRobotsCrawlDelay verifies crawl-delay is extracted correctly.
func TestRobotsCrawlDelay(t *testing.T) {
	robotsTxt := `User-agent: *
Crawl-delay: 2
Disallow: /admin/

User-agent: SlowBot
Crawl-delay: 10
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
	}))
	defer server.Close()

	ctx := context.Background()

	// Wildcard should get 2 second delay
	wildcardChecker := New("TestBot", 1*time.Hour, server.Client())
	delay, err := wildcardChecker.GetCrawlDelay(ctx, server.URL+"/page")
	require.NoError(t, err)
	assert.Equal(t, 2*time.Second, delay, "should return 2 second crawl delay for wildcard")

	// SlowBot should get 10 second delay
	slowChecker := New("SlowBot", 1*time.Hour, server.Client())
	delay, err = slowChecker.GetCrawlDelay(ctx, server.URL+"/page")
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, delay, "should return 10 second crawl delay for SlowBot")
}

// TestRobotsCrawlDelayZeroWhenNotSpecified verifies crawl-delay defaults to 0.
func TestRobotsCrawlDelayZeroWhenNotSpecified(t *testing.T) {
	robotsTxt := `User-agent: *
Disallow: /admin/
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
	}))
	defer server.Close()

	checker := New("TestBot", 1*time.Hour, server.Client())
	delay, err := checker.GetCrawlDelay(context.Background(), server.URL+"/page")

	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), delay, "should return 0 when no crawl-delay specified")
}

// TestRobotsLongestMatchWins verifies longest matching rule takes precedence.
func TestRobotsLongestMatchWins(t *testing.T) {
	robotsTxt := `User-agent: *
Disallow: /files/
Allow: /files/public/
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
	}))
	defer server.Close()

	checker := New("TestBot", 1*time.Hour, server.Client())
	ctx := context.Background()

	// /files/ is disallowed
	allowed, err := checker.IsAllowed(ctx, server.URL+"/files/private.txt")
	require.NoError(t, err)
	assert.False(t, allowed, "/files/ should be disallowed")

	// /files/public/ is allowed (longer match)
	allowed, err = checker.IsAllowed(ctx, server.URL+"/files/public/doc.txt")
	require.NoError(t, err)
	assert.True(t, allowed, "/files/public/ should be allowed (longest match wins)")
}

// TestRobotsPrefixMatching verifies prefix-based path matching (most common pattern).
func TestRobotsPrefixMatching(t *testing.T) {
	robotsTxt := `User-agent: *
Disallow: /admin
Disallow: /api/
Disallow: /temp
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
	}))
	defer server.Close()

	checker := New("TestBot", 1*time.Hour, server.Client())
	ctx := context.Background()

	tests := []struct {
		path    string
		allowed bool
		reason  string
	}{
		{"/", true, "root should be allowed"},
		{"/public", true, "public path should be allowed"},
		{"/admin", false, "/admin prefix should be blocked"},
		{"/admin/users", false, "/admin* should block all subpaths"},
		{"/api/", false, "/api/ should be blocked"},
		{"/api/v1/users", false, "/api/ should block all subpaths"},
		{"/temp", false, "/temp should be blocked"},
		{"/temporary", false, "/temp prefix matches /temporary"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			allowed, err := checker.IsAllowed(ctx, server.URL+tt.path)
			require.NoError(t, err)
			assert.Equal(t, tt.allowed, allowed, tt.reason)
		})
	}
}

// TestRobotsQueryStringHandling verifies query strings are checked.
func TestRobotsQueryStringHandling(t *testing.T) {
	robotsTxt := `User-agent: *
Disallow: /search?
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
	}))
	defer server.Close()

	checker := New("TestBot", 1*time.Hour, server.Client())
	ctx := context.Background()

	// Without query string - allowed
	allowed, err := checker.IsAllowed(ctx, server.URL+"/search")
	require.NoError(t, err)
	assert.True(t, allowed, "/search without query should be allowed")

	// With query string - disallowed
	allowed, err = checker.IsAllowed(ctx, server.URL+"/search?q=test")
	require.NoError(t, err)
	assert.False(t, allowed, "/search with query should be disallowed")
}

// TestRobotsCaching verifies robots.txt is cached and reused.
func TestRobotsCaching(t *testing.T) {
	fetchCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			fetchCount++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("User-agent: *\nDisallow: /admin/\n"))
			return
		}
	}))
	defer server.Close()

	checker := New("TestBot", 1*time.Hour, server.Client())
	ctx := context.Background()

	// First request - should fetch
	_, err := checker.IsAllowed(ctx, server.URL+"/page1")
	require.NoError(t, err)
	assert.Equal(t, 1, fetchCount, "should fetch robots.txt on first request")

	// Second request - should use cache
	_, err = checker.IsAllowed(ctx, server.URL+"/page2")
	require.NoError(t, err)
	assert.Equal(t, 1, fetchCount, "should use cached robots.txt on second request")

	// Third request - should still use cache
	_, err = checker.IsAllowed(ctx, server.URL+"/page3")
	require.NoError(t, err)
	assert.Equal(t, 1, fetchCount, "should still use cached robots.txt")
}

// TestRobotsCacheExpiration verifies cache expires after TTL.
func TestRobotsCacheExpiration(t *testing.T) {
	fetchCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			fetchCount++
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("User-agent: *\nDisallow: /admin/\n"))
			return
		}
	}))
	defer server.Close()

	// Very short TTL for testing
	checker := New("TestBot", 100*time.Millisecond, server.Client())
	ctx := context.Background()

	// First request
	_, err := checker.IsAllowed(ctx, server.URL+"/page1")
	require.NoError(t, err)
	assert.Equal(t, 1, fetchCount)

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Should fetch again
	_, err = checker.IsAllowed(ctx, server.URL+"/page2")
	require.NoError(t, err)
	assert.Equal(t, 2, fetchCount, "should re-fetch after cache expiration")
}

// TestRobotsEmptyDisallow verifies empty Disallow allows everything.
func TestRobotsEmptyDisallow(t *testing.T) {
	robotsTxt := `User-agent: *
Disallow:
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
	}))
	defer server.Close()

	checker := New("TestBot", 1*time.Hour, server.Client())
	ctx := context.Background()

	allowed, err := checker.IsAllowed(ctx, server.URL+"/any/path")
	require.NoError(t, err)
	assert.True(t, allowed, "empty Disallow should allow all paths")
}

// TestRobotsCommentsAndBlankLines verifies comments and blank lines are ignored.
func TestRobotsCommentsAndBlankLines(t *testing.T) {
	robotsTxt := `# This is a comment
User-agent: *

# Another comment
Disallow: /admin/

Disallow: /private/
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
	}))
	defer server.Close()

	checker := New("TestBot", 1*time.Hour, server.Client())
	ctx := context.Background()

	allowed, err := checker.IsAllowed(ctx, server.URL+"/admin/users")
	require.NoError(t, err)
	assert.False(t, allowed, "should parse rules correctly despite comments and blank lines")
}

// TestRobotsUserAgentMatching verifies user-agent matching is case-insensitive and partial.
func TestRobotsUserAgentMatching(t *testing.T) {
	robotsTxt := `User-agent: googlebot
Disallow: /private/
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
	}))
	defer server.Close()

	ctx := context.Background()

	// Exact match
	checker1 := New("Googlebot", 1*time.Hour, server.Client())
	allowed, err := checker1.IsAllowed(ctx, server.URL+"/private/page")
	require.NoError(t, err)
	assert.False(t, allowed, "exact match should work")

	// Partial match (user agent contains "googlebot")
	checker2 := New("Googlebot/2.1", 1*time.Hour, server.Client())
	allowed, err = checker2.IsAllowed(ctx, server.URL+"/private/page")
	require.NoError(t, err)
	assert.False(t, allowed, "partial match should work")

	// Case insensitive
	checker3 := New("GOOGLEBOT", 1*time.Hour, server.Client())
	allowed, err = checker3.IsAllowed(ctx, server.URL+"/private/page")
	require.NoError(t, err)
	assert.False(t, allowed, "case insensitive match should work")
}

// TestRobotsRootDisallow verifies Disallow: / blocks everything.
func TestRobotsRootDisallow(t *testing.T) {
	robotsTxt := `User-agent: BadBot
Disallow: /
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(robotsTxt))
			return
		}
	}))
	defer server.Close()

	checker := New("BadBot", 1*time.Hour, server.Client())
	ctx := context.Background()

	tests := []string{"/", "/page", "/admin/users", "/any/deep/path"}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			allowed, err := checker.IsAllowed(ctx, server.URL+path)
			require.NoError(t, err)
			assert.False(t, allowed, "Disallow: / should block all paths")
		})
	}
}

// TestParseRobotsLine verifies line parsing extracts directive and value correctly.
func TestParseRobotsLine(t *testing.T) {
	tests := []struct {
		line              string
		expectedDirective string
		expectedValue     string
	}{
		{"User-agent: *", "user-agent", "*"},
		{"Disallow: /admin/", "disallow", "/admin/"},
		{"Allow: /public/", "allow", "/public/"},
		{"Crawl-delay: 2", "crawl-delay", "2"},
		{"  User-agent:  TestBot  ", "user-agent", "TestBot"},
		{"Invalid line", "", ""},
		{"NoColon", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			directive, value := parseRobotsLine(strings.TrimSpace(tt.line))
			assert.Equal(t, tt.expectedDirective, directive)
			assert.Equal(t, tt.expectedValue, value)
		})
	}
}
