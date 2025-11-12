package config

import (
	"fmt"
	"testing"
	"time"
)

func TestCompilePattern(t *testing.T) {
	tests := []struct {
		pattern      string
		expectType   patternType
		expectHost   string
		expectPath   string
		expectDomain string
	}{
		{
			pattern:    "example.com",
			expectType: patternExact,
			expectHost: "example.com",
		},
		{
			pattern:      "*.example.com",
			expectType:   patternWildcardDomain,
			expectDomain: "example.com",
		},
		{
			pattern:      "*.example.com/docs/*",
			expectType:   patternWildcardDomainPath,
			expectDomain: "example.com",
			expectPath:   "/docs/*",
		},
		{
			pattern:    "example.com/docs/*",
			expectType: patternHostPath,
			expectHost: "example.com",
			expectPath: "/docs/*",
		},
		{
			pattern:    "*example*",
			expectType: patternWildcardHost,
			expectHost: "*example*",
		},
		{
			pattern:    "api-*",
			expectType: patternWildcardHost,
			expectHost: "api-*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			cp := compilePattern(tt.pattern)

			if cp.patternType != tt.expectType {
				t.Errorf("pattern type = %v, want %v", cp.patternType, tt.expectType)
			}
			if cp.host != tt.expectHost {
				t.Errorf("host = %q, want %q", cp.host, tt.expectHost)
			}
			if cp.path != tt.expectPath {
				t.Errorf("path = %q, want %q", cp.path, tt.expectPath)
			}
			if cp.domain != tt.expectDomain {
				t.Errorf("domain = %q, want %q", cp.domain, tt.expectDomain)
			}
		})
	}
}

func TestMatchCompiledPattern(t *testing.T) {
	tests := []struct {
		url     string
		pattern string
		match   bool
	}{
		// Exact matches
		{"https://example.com", "example.com", true},
		{"https://example.com/path", "example.com", true},
		{"https://other.com", "example.com", false},

		// Wildcard domain matches
		{"https://docs.example.com", "*.example.com", true},
		{"https://api.example.com", "*.example.com", true},
		{"https://example.com", "*.example.com", true},
		{"https://other.com", "*.example.com", false},

		// Wildcard domain + path matches
		{"https://docs.example.com/api/v1", "*.example.com/api/*", true},
		{"https://api.example.com/api/v2", "*.example.com/api/*", true},
		{"https://docs.example.com/other", "*.example.com/api/*", false},

		// Host + path matches
		{"https://example.com/docs/page", "example.com/docs/*", true},
		{"https://example.com/docs/", "example.com/docs/*", true},
		{"https://example.com/api/", "example.com/docs/*", false},

		// Wildcard host matches
		{"https://api-prod.example.com", "api-*", true},
		{"https://api-staging.example.com", "api-*", true},
		{"https://web.example.com", "api-*", false},

		// SEC.gov pattern from config
		{"https://www.sec.gov/files/edgar.html", "*.sec.gov", true},
		{"https://sec.gov/files/edgar.html", "*.sec.gov", true},
		{"https://data.sec.gov/api/endpoint", "*.sec.gov", true},

		// Docs patterns from config
		{"https://docs.example.com/guide", "docs.*", true},
		{"https://docs.python.org/3/", "docs.*", true},
		{"https://example.com/docs/guide", "*/docs/*", true},
		{"https://api.github.com/docs/rest", "*/docs/*", true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s matches %s", tt.url, tt.pattern), func(t *testing.T) {
			cp := compilePattern(tt.pattern)
			result := matchCompiledPattern(tt.url, cp)

			if result != tt.match {
				t.Errorf("matchCompiledPattern(%q, %q) = %v, want %v",
					tt.url, tt.pattern, result, tt.match)
			}
		})
	}
}

func TestGetConfigForURL_UsesCompiledPatterns(t *testing.T) {
	cfg := &Config{
		Default: DefaultConfig{
			Cache: CacheConfig{
				TTL: 3600 * time.Second,
			},
		},
		Sites: []SiteConfig{
			{
				Pattern: "*.sec.gov",
				Cache: &CacheConfig{
					TTL: 86400 * time.Second,
				},
			},
			{
				Pattern: "docs.*",
				Cache: &CacheConfig{
					TTL: 10800 * time.Second,
				},
			},
		},
	}

	tests := []struct {
		url        string
		expectTTL  time.Duration
		expectName string
	}{
		{"https://example.com", 3600 * time.Second, "default config"},
		{"https://www.sec.gov/files/edgar.html", 86400 * time.Second, "SEC.gov pattern"},
		{"https://docs.python.org/3/", 10800 * time.Second, "docs pattern"},
	}

	for _, tt := range tests {
		t.Run(tt.expectName, func(t *testing.T) {
			resolved := cfg.GetConfigForURL(tt.url)
			if resolved.Cache.TTL != tt.expectTTL {
				t.Errorf("TTL = %v, want %v", resolved.Cache.TTL, tt.expectTTL)
			}
		})
	}

	// Verify patterns were compiled (should happen on first call)
	if !cfg.compiledOnce {
		t.Error("patterns were not compiled")
	}
	if len(cfg.compiledSites) != 2 {
		t.Errorf("expected 2 compiled patterns, got %d", len(cfg.compiledSites))
	}

	// Second call should reuse compiled patterns
	_ = cfg.GetConfigForURL("https://example.com")
	if len(cfg.compiledSites) != 2 {
		t.Error("compiled patterns were regenerated on second call")
	}
}

// Benchmark pattern matching performance
func BenchmarkPatternMatching(b *testing.B) {
	patterns := []string{
		"*.sec.gov",
		"docs.*",
		"*/docs/*",
		"example.com",
		"*.example.com/api/*",
		"api-*",
	}

	testURLs := []string{
		"https://www.sec.gov/files/edgar.html",
		"https://docs.python.org/3/library/",
		"https://github.com/user/repo/docs/readme",
		"https://example.com/index.html",
		"https://api.example.com/api/v1/users",
		"https://api-prod.service.com/health",
	}

	// Pre-compile patterns
	compiled := make([]compiledPattern, len(patterns))
	for i, p := range patterns {
		compiled[i] = compilePattern(p)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, urlStr := range testURLs {
			for _, cp := range compiled {
				matchCompiledPattern(urlStr, cp)
			}
		}
	}
}

func BenchmarkGetConfigForURL(b *testing.B) {
	cfg := &Config{
		Default: DefaultConfig{
			Cache: CacheConfig{TTL: 3600 * time.Second},
			Fetch: FetchConfig{Timeout: 30 * time.Second},
		},
		Sites: []SiteConfig{
			{Pattern: "*.sec.gov", Cache: &CacheConfig{TTL: 86400 * time.Second}},
			{Pattern: "docs.*", Cache: &CacheConfig{TTL: 10800 * time.Second}},
			{Pattern: "*/docs/*", Cache: &CacheConfig{TTL: 7200 * time.Second}},
			{Pattern: "*.github.com", Cache: &CacheConfig{TTL: 1800 * time.Second}},
			{Pattern: "api-*", Cache: &CacheConfig{TTL: 600 * time.Second}},
		},
	}

	testURLs := []string{
		"https://www.sec.gov/files/edgar.html",
		"https://docs.python.org/3/library/",
		"https://github.com/user/repo/docs/readme",
		"https://example.com/index.html",
		"https://api.example.com/api/v1/users",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, url := range testURLs {
			cfg.GetConfigForURL(url)
		}
	}
}

func BenchmarkGetConfigForURL_ManyPatterns(b *testing.B) {
	// Simulate a config with many site-specific patterns
	cfg := &Config{
		Default: DefaultConfig{
			Cache: CacheConfig{TTL: 3600 * time.Second},
		},
		Sites: make([]SiteConfig, 50),
	}

	// Create 50 different patterns
	for i := 0; i < 50; i++ {
		ttl := time.Duration(3600+i*100) * time.Second
		cfg.Sites[i] = SiteConfig{
			Pattern: fmt.Sprintf("*.site%d.com", i),
			Cache:   &CacheConfig{TTL: ttl},
		}
	}

	testURL := "https://www.site25.com/page"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cfg.GetConfigForURL(testURL)
	}
}
