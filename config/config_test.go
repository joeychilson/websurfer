package config

import (
	"strings"
	"testing"
	"time"
)

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		pattern string
		want    bool
	}{
		{
			name:    "exact domain match",
			url:     "https://example.com",
			pattern: "example.com",
			want:    true,
		},
		{
			name:    "exact domain no match",
			url:     "https://other.com",
			pattern: "example.com",
			want:    false,
		},
		{
			name:    "substring should not match",
			url:     "https://fakeexample.com",
			pattern: "example.com",
			want:    false,
		},
		{
			name:    "domain hijack should not match",
			url:     "https://example.com.evil.com",
			pattern: "example.com",
			want:    false,
		},
		{
			name:    "wildcard subdomain match",
			url:     "https://sub.example.com",
			pattern: "*.example.com",
			want:    true,
		},
		{
			name:    "wildcard subdomain deep match",
			url:     "https://deep.sub.example.com",
			pattern: "*.example.com",
			want:    true,
		},
		{
			name:    "wildcard matches root domain",
			url:     "https://example.com",
			pattern: "*.example.com",
			want:    true,
		},
		{
			name:    "wildcard no match different domain",
			url:     "https://other.com",
			pattern: "*.example.com",
			want:    false,
		},
		{
			name:    "wildcard no match domain hijack",
			url:     "https://fakeexample.com",
			pattern: "*.example.com",
			want:    false,
		},
		{
			name:    "path prefix match",
			url:     "https://example.com/api/v1/users",
			pattern: "example.com/api/*",
			want:    true,
		},
		{
			name:    "path prefix exact boundary",
			url:     "https://example.com/api/",
			pattern: "example.com/api/*",
			want:    true,
		},
		{
			name:    "path prefix no match",
			url:     "https://example.com/docs/api",
			pattern: "example.com/api/*",
			want:    false,
		},
		{
			name:    "exact path match",
			url:     "https://example.com/api/v1",
			pattern: "example.com/api/v1",
			want:    true,
		},
		{
			name:    "exact path no match",
			url:     "https://example.com/api/v2",
			pattern: "example.com/api/v1",
			want:    false,
		},
		{
			name:    "wildcard host prefix",
			url:     "https://api.example.com",
			pattern: "*example.com",
			want:    true,
		},
		{
			name:    "wildcard host suffix",
			url:     "https://example-prod.com",
			pattern: "example*",
			want:    true,
		},
		{
			name:    "wildcard host contains",
			url:     "https://my-example-api.com",
			pattern: "*example*",
			want:    true,
		},
		{
			name:    "wildcard domain with path prefix",
			url:     "https://api.example.com/v1/users",
			pattern: "*.example.com/v1/*",
			want:    true,
		},
		{
			name:    "wildcard domain with exact path",
			url:     "https://api.example.com/health",
			pattern: "*.example.com/health",
			want:    true,
		},
		{
			name:    "wildcard domain with path no match",
			url:     "https://api.example.com/v2/users",
			pattern: "*.example.com/v1/*",
			want:    false,
		},
		{
			name:    "invalid url requires exact match",
			url:     "not-a-url",
			pattern: "not-a-url",
			want:    true,
		},
		{
			name:    "invalid url no match",
			url:     "not-a-url",
			pattern: "url",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchPattern(tt.url, tt.pattern)
			if got != tt.want {
				t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.url, tt.pattern, got, tt.want)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	t.Run("valid default config", func(t *testing.T) {
		cfg := &Config{
			Default: DefaultConfig{
				RateLimit: RateLimitConfig{
					RequestsPerSecond: 1.0,
					MaxConcurrent:     2,
				},
				Retry: RetryConfig{
					MaxRetries:   3,
					InitialDelay: time.Second,
					MaxDelay:     30 * time.Second,
					Multiplier:   2.0,
				},
				Fetch: FetchConfig{
					Timeout:      30 * time.Second,
					MaxRedirects: 10,
				},
			},
		}

		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("conflicting rate limit delay and requests_per_second", func(t *testing.T) {
		cfg := &Config{
			Default: DefaultConfig{
				RateLimit: RateLimitConfig{
					Delay:             5 * time.Second,
					RequestsPerSecond: 1.0,
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "cannot specify both 'delay' and 'requests_per_second'") {
			t.Errorf("Validate() error = %v, want error about conflicting delay/requests_per_second", err)
		}
	})

	t.Run("burst without rate limit", func(t *testing.T) {
		cfg := &Config{
			Default: DefaultConfig{
				RateLimit: RateLimitConfig{
					Burst: 5,
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "'burst' requires either 'requests_per_second' or 'delay'") {
			t.Errorf("Validate() error = %v, want error about burst requirement", err)
		}
	})

	t.Run("negative max_concurrent", func(t *testing.T) {
		cfg := &Config{
			Default: DefaultConfig{
				RateLimit: RateLimitConfig{
					MaxConcurrent: -1,
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "'max_concurrent' must be >= 0") {
			t.Errorf("Validate() error = %v, want error about max_concurrent", err)
		}
	})

	t.Run("retry multiplier less than 1.0", func(t *testing.T) {
		cfg := &Config{
			Default: DefaultConfig{
				Retry: RetryConfig{
					Multiplier: 0.5,
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "'multiplier' must be >= 1.0") {
			t.Errorf("Validate() error = %v, want error about multiplier", err)
		}
	})

	t.Run("negative max_retries", func(t *testing.T) {
		cfg := &Config{
			Default: DefaultConfig{
				Retry: RetryConfig{
					MaxRetries: -1,
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "'max_retries' must be >= 0") {
			t.Errorf("Validate() error = %v, want error about max_retries", err)
		}
	})

	t.Run("initial_delay greater than max_delay", func(t *testing.T) {
		cfg := &Config{
			Default: DefaultConfig{
				Retry: RetryConfig{
					InitialDelay: 30 * time.Second,
					MaxDelay:     10 * time.Second,
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "cannot be greater than 'max_delay'") {
			t.Errorf("Validate() error = %v, want error about delay mismatch", err)
		}
	})

	t.Run("invalid HTTP status code", func(t *testing.T) {
		cfg := &Config{
			Default: DefaultConfig{
				Retry: RetryConfig{
					RetryOn: []int{200, 600},
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "invalid HTTP status code 600") {
			t.Errorf("Validate() error = %v, want error about invalid status code", err)
		}
	})

	t.Run("empty pattern in site config", func(t *testing.T) {
		cfg := &Config{
			Sites: []SiteConfig{
				{Pattern: ""},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "pattern cannot be empty") {
			t.Errorf("Validate() error = %v, want error about empty pattern", err)
		}
	})

	t.Run("invalid url rewrite type", func(t *testing.T) {
		cfg := &Config{
			Default: DefaultConfig{
				Fetch: FetchConfig{
					URLRewrites: []URLRewrite{
						{
							Type:        "invalid",
							Pattern:     "test",
							Replacement: "new",
						},
					},
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "'type' must be 'regex' or 'literal'") {
			t.Errorf("Validate() error = %v, want error about invalid rewrite type", err)
		}
	})

	t.Run("empty url rewrite pattern", func(t *testing.T) {
		cfg := &Config{
			Default: DefaultConfig{
				Fetch: FetchConfig{
					URLRewrites: []URLRewrite{
						{Pattern: ""},
					},
				},
			},
		}

		err := cfg.Validate()
		if err == nil {
			t.Error("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "'pattern' cannot be empty") {
			t.Errorf("Validate() error = %v, want error about empty pattern", err)
		}
	})
}

func TestMergeCache(t *testing.T) {
	tests := []struct {
		name     string
		base     CacheConfig
		override CacheConfig
		want     CacheConfig
	}{
		{
			name: "override TTL",
			base: CacheConfig{
				TTL:       10 * time.Minute,
				StaleTime: 1 * time.Hour,
			},
			override: CacheConfig{
				TTL: 15 * time.Minute,
			},
			want: CacheConfig{
				TTL:       15 * time.Minute,
				StaleTime: 1 * time.Hour,
			},
		},
		{
			name: "override StaleTime",
			base: CacheConfig{
				TTL:       10 * time.Minute,
				StaleTime: 1 * time.Hour,
			},
			override: CacheConfig{
				StaleTime: 2 * time.Hour,
			},
			want: CacheConfig{
				TTL:       10 * time.Minute,
				StaleTime: 2 * time.Hour,
			},
		},
		{
			name: "override both",
			base: CacheConfig{
				TTL:       10 * time.Minute,
				StaleTime: 1 * time.Hour,
			},
			override: CacheConfig{
				TTL:       20 * time.Minute,
				StaleTime: 3 * time.Hour,
			},
			want: CacheConfig{
				TTL:       20 * time.Minute,
				StaleTime: 3 * time.Hour,
			},
		},
		{
			name: "zero values don't override",
			base: CacheConfig{
				TTL:       10 * time.Minute,
				StaleTime: 1 * time.Hour,
			},
			override: CacheConfig{},
			want: CacheConfig{
				TTL:       10 * time.Minute,
				StaleTime: 1 * time.Hour,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeCache(tt.base, tt.override)
			if got.TTL != tt.want.TTL {
				t.Errorf("TTL = %v, want %v", got.TTL, tt.want.TTL)
			}
			if got.StaleTime != tt.want.StaleTime {
				t.Errorf("StaleTime = %v, want %v", got.StaleTime, tt.want.StaleTime)
			}
		})
	}
}

func TestMergeRateLimit(t *testing.T) {
	tests := []struct {
		name     string
		base     RateLimitConfig
		override RateLimitConfig
		want     RateLimitConfig
	}{
		{
			name: "override all fields",
			base: RateLimitConfig{
				RequestsPerSecond: 1.0,
				Burst:             5,
				MaxConcurrent:     2,
				RespectRetryAfter: false,
			},
			override: RateLimitConfig{
				RequestsPerSecond: 2.0,
				Burst:             10,
				MaxConcurrent:     5,
				RespectRetryAfter: true,
			},
			want: RateLimitConfig{
				RequestsPerSecond: 2.0,
				Burst:             10,
				MaxConcurrent:     5,
				RespectRetryAfter: true,
			},
		},
		{
			name: "delay takes precedence",
			base: RateLimitConfig{
				RequestsPerSecond: 1.0,
			},
			override: RateLimitConfig{
				Delay: 5 * time.Second,
			},
			want: RateLimitConfig{
				RequestsPerSecond: 1.0,
				Delay:             5 * time.Second,
			},
		},
		{
			name: "zero values don't override except bools",
			base: RateLimitConfig{
				RequestsPerSecond: 1.0,
				Burst:             5,
				RespectRetryAfter: true,
			},
			override: RateLimitConfig{
				RespectRetryAfter: false,
			},
			want: RateLimitConfig{
				RequestsPerSecond: 1.0,
				Burst:             5,
				RespectRetryAfter: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeRateLimit(tt.base, tt.override)
			if got.RequestsPerSecond != tt.want.RequestsPerSecond {
				t.Errorf("RequestsPerSecond = %v, want %v", got.RequestsPerSecond, tt.want.RequestsPerSecond)
			}
			if got.Burst != tt.want.Burst {
				t.Errorf("Burst = %v, want %v", got.Burst, tt.want.Burst)
			}
			if got.Delay != tt.want.Delay {
				t.Errorf("Delay = %v, want %v", got.Delay, tt.want.Delay)
			}
			if got.MaxConcurrent != tt.want.MaxConcurrent {
				t.Errorf("MaxConcurrent = %v, want %v", got.MaxConcurrent, tt.want.MaxConcurrent)
			}
			if got.RespectRetryAfter != tt.want.RespectRetryAfter {
				t.Errorf("RespectRetryAfter = %v, want %v", got.RespectRetryAfter, tt.want.RespectRetryAfter)
			}
		})
	}
}

func TestMergeRetry(t *testing.T) {
	tests := []struct {
		name     string
		base     RetryConfig
		override RetryConfig
		want     RetryConfig
	}{
		{
			name: "override all fields",
			base: RetryConfig{
				MaxRetries:   3,
				InitialDelay: 1 * time.Second,
				MaxDelay:     30 * time.Second,
				Multiplier:   2.0,
				RetryOn:      []int{429, 503},
			},
			override: RetryConfig{
				MaxRetries:   5,
				InitialDelay: 2 * time.Second,
				MaxDelay:     60 * time.Second,
				Multiplier:   3.0,
				RetryOn:      []int{500, 502, 503},
			},
			want: RetryConfig{
				MaxRetries:   5,
				InitialDelay: 2 * time.Second,
				MaxDelay:     60 * time.Second,
				Multiplier:   3.0,
				RetryOn:      []int{500, 502, 503},
			},
		},
		{
			name: "zero values don't override",
			base: RetryConfig{
				MaxRetries:   3,
				InitialDelay: 1 * time.Second,
				Multiplier:   2.0,
			},
			override: RetryConfig{},
			want: RetryConfig{
				MaxRetries:   3,
				InitialDelay: 1 * time.Second,
				Multiplier:   2.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeRetry(tt.base, tt.override)
			if got.MaxRetries != tt.want.MaxRetries {
				t.Errorf("MaxRetries = %v, want %v", got.MaxRetries, tt.want.MaxRetries)
			}
			if got.InitialDelay != tt.want.InitialDelay {
				t.Errorf("InitialDelay = %v, want %v", got.InitialDelay, tt.want.InitialDelay)
			}
			if got.MaxDelay != tt.want.MaxDelay {
				t.Errorf("MaxDelay = %v, want %v", got.MaxDelay, tt.want.MaxDelay)
			}
			if got.Multiplier != tt.want.Multiplier {
				t.Errorf("Multiplier = %v, want %v", got.Multiplier, tt.want.Multiplier)
			}
			if len(got.RetryOn) != len(tt.want.RetryOn) {
				t.Errorf("RetryOn length = %v, want %v", len(got.RetryOn), len(tt.want.RetryOn))
			}
		})
	}
}

func TestRateLimitConfig_GetDelay(t *testing.T) {
	tests := []struct {
		name   string
		config RateLimitConfig
		want   time.Duration
	}{
		{
			name:   "delay takes precedence",
			config: RateLimitConfig{Delay: 5 * time.Second, RequestsPerSecond: 1.0},
			want:   5 * time.Second,
		},
		{
			name:   "calculate from requests_per_second",
			config: RateLimitConfig{RequestsPerSecond: 2.0},
			want:   500 * time.Millisecond,
		},
		{
			name:   "no rate limit configured",
			config: RateLimitConfig{},
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetDelay()
			if got != tt.want {
				t.Errorf("GetDelay() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRetryConfig_ShouldRetry(t *testing.T) {
	config := RetryConfig{
		RetryOn: []int{429, 500, 502, 503, 504},
	}

	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{"retry 429", 429, true},
		{"retry 500", 500, true},
		{"retry 503", 503, true},
		{"don't retry 200", 200, false},
		{"don't retry 404", 404, false},
		{"don't retry 400", 400, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := config.ShouldRetry(tt.statusCode)
			if got != tt.want {
				t.Errorf("ShouldRetry(%d) = %v, want %v", tt.statusCode, got, tt.want)
			}
		})
	}
}

func TestRetryConfig_GetRetryOn(t *testing.T) {
	t.Run("returns configured values", func(t *testing.T) {
		config := RetryConfig{
			RetryOn: []int{429, 503},
		}
		got := config.GetRetryOn()
		if len(got) != 2 {
			t.Errorf("len(GetRetryOn()) = %d, want 2", len(got))
		}
	})

	t.Run("returns defaults when empty", func(t *testing.T) {
		config := RetryConfig{}
		got := config.GetRetryOn()
		want := []int{429, 500, 502, 503, 504}
		if len(got) != len(want) {
			t.Errorf("len(GetRetryOn()) = %d, want %d", len(got), len(want))
		}
	})
}

func TestFetchConfig_GetHeaders(t *testing.T) {
	t.Run("uses custom user agent", func(t *testing.T) {
		config := FetchConfig{
			UserAgent: "CustomAgent/1.0",
		}
		got := config.GetHeaders()
		if got["User-Agent"] != "CustomAgent/1.0" {
			t.Errorf("User-Agent = %q, want %q", got["User-Agent"], "CustomAgent/1.0")
		}
	})

	t.Run("uses default user agent when empty", func(t *testing.T) {
		config := FetchConfig{}
		got := config.GetHeaders()
		if got["User-Agent"] != DefaultUserAgent {
			t.Errorf("User-Agent = %q, want %q", got["User-Agent"], DefaultUserAgent)
		}
	})

	t.Run("merges custom headers", func(t *testing.T) {
		config := FetchConfig{
			Headers: map[string]string{
				"X-Custom": "value",
			},
		}
		got := config.GetHeaders()
		if got["X-Custom"] != "value" {
			t.Errorf("X-Custom = %q, want %q", got["X-Custom"], "value")
		}
	})
}

func TestFetchConfig_GetMaxRedirects(t *testing.T) {
	tests := []struct {
		name   string
		config FetchConfig
		want   int
	}{
		{
			name:   "uses configured value",
			config: FetchConfig{MaxRedirects: 5},
			want:   5,
		},
		{
			name:   "returns default when zero and follow enabled",
			config: FetchConfig{FollowRedirects: true},
			want:   10,
		},
		{
			name:   "returns zero when follow disabled",
			config: FetchConfig{FollowRedirects: false},
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetMaxRedirects()
			if got != tt.want {
				t.Errorf("GetMaxRedirects() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_GetConfigForURL(t *testing.T) {
	config := &Config{
		Default: DefaultConfig{
			Cache: CacheConfig{
				TTL: 10 * time.Minute,
			},
			RateLimit: RateLimitConfig{
				RequestsPerSecond: 1.0,
			},
		},
		Sites: []SiteConfig{
			{
				Pattern: "*.sec.gov",
				RateLimit: &RateLimitConfig{
					RequestsPerSecond: 0.1,
				},
			},
			{
				Pattern: "example.com/api/*",
				Cache: &CacheConfig{
					TTL: 5 * time.Minute,
				},
			},
		},
	}

	t.Run("returns default for non-matching URL", func(t *testing.T) {
		got := config.GetConfigForURL("https://unknown.com")
		if got.RateLimit.RequestsPerSecond != 1.0 {
			t.Errorf("RequestsPerSecond = %v, want 1.0", got.RateLimit.RequestsPerSecond)
		}
		if got.Cache.TTL != 10*time.Minute {
			t.Errorf("TTL = %v, want 10m", got.Cache.TTL)
		}
	})

	t.Run("merges site-specific rate limit", func(t *testing.T) {
		got := config.GetConfigForURL("https://www.sec.gov/edgar")
		if got.RateLimit.RequestsPerSecond != 0.1 {
			t.Errorf("RequestsPerSecond = %v, want 0.1", got.RateLimit.RequestsPerSecond)
		}
		if got.Cache.TTL != 10*time.Minute {
			t.Errorf("TTL = %v, want 10m (should keep default)", got.Cache.TTL)
		}
	})

	t.Run("merges site-specific cache", func(t *testing.T) {
		got := config.GetConfigForURL("https://example.com/api/users")
		if got.Cache.TTL != 5*time.Minute {
			t.Errorf("TTL = %v, want 5m", got.Cache.TTL)
		}
		if got.RateLimit.RequestsPerSecond != 1.0 {
			t.Errorf("RequestsPerSecond = %v, want 1.0 (should keep default)", got.RateLimit.RequestsPerSecond)
		}
	})
}

func TestChunkConfig_GetTokens(t *testing.T) {
	tests := []struct {
		name   string
		config ChunkConfig
		want   int
	}{
		{
			name:   "returns configured value",
			config: ChunkConfig{Tokens: 800},
			want:   800,
		},
		{
			name:   "returns default when zero",
			config: ChunkConfig{Tokens: 0},
			want:   600,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetTokens()
			if got != tt.want {
				t.Errorf("GetTokens() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChunkConfig_GetOverlap(t *testing.T) {
	tests := []struct {
		name   string
		config ChunkConfig
		want   int
	}{
		{
			name:   "returns configured value",
			config: ChunkConfig{Overlap: 100},
			want:   100,
		},
		{
			name:   "returns default when zero",
			config: ChunkConfig{Overlap: 0},
			want:   75,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetOverlap()
			if got != tt.want {
				t.Errorf("GetOverlap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCrawlConfig_GetStrategy(t *testing.T) {
	tests := []struct {
		name   string
		config CrawlConfig
		want   string
	}{
		{
			name:   "returns sitemap strategy",
			config: CrawlConfig{Strategy: "sitemap"},
			want:   "sitemap",
		},
		{
			name:   "returns links strategy",
			config: CrawlConfig{Strategy: "links"},
			want:   "links",
		},
		{
			name:   "returns both strategy",
			config: CrawlConfig{Strategy: "both"},
			want:   "both",
		},
		{
			name:   "returns default sitemap when empty",
			config: CrawlConfig{Strategy: ""},
			want:   "sitemap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetStrategy()
			if got != tt.want {
				t.Errorf("GetStrategy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCrawlConfig_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		config CrawlConfig
		want   bool
	}{
		{
			name:   "sitemap is valid",
			config: CrawlConfig{Strategy: "sitemap"},
			want:   true,
		},
		{
			name:   "links is valid",
			config: CrawlConfig{Strategy: "links"},
			want:   true,
		},
		{
			name:   "both is valid",
			config: CrawlConfig{Strategy: "both"},
			want:   true,
		},
		{
			name:   "empty defaults to sitemap (valid)",
			config: CrawlConfig{Strategy: ""},
			want:   true,
		},
		{
			name:   "invalid strategy",
			config: CrawlConfig{Strategy: "invalid"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.IsValid()
			if got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSiteConfig_IsCrawlable(t *testing.T) {
	tests := []struct {
		name string
		site SiteConfig
		want bool
	}{
		{
			name: "has crawl config",
			site: SiteConfig{
				Pattern: "example.com",
				Crawl:   &CrawlConfig{Strategy: "sitemap"},
			},
			want: true,
		},
		{
			name: "no crawl config",
			site: SiteConfig{
				Pattern: "example.com",
				Crawl:   nil,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.site.IsCrawlable()
			if got != tt.want {
				t.Errorf("IsCrawlable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSiteConfig_GetCrawlBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    string
	}{
		{
			name:    "simple domain",
			pattern: "example.com",
			want:    "https://example.com",
		},
		{
			name:    "wildcard subdomain",
			pattern: "*.example.com",
			want:    "https://example.com",
		},
		{
			name:    "domain with path",
			pattern: "example.com/api",
			want:    "https://example.com",
		},
		{
			name:    "wildcard domain with path",
			pattern: "*.example.com/api/*",
			want:    "https://example.com",
		},
		{
			name:    "domain with trailing wildcard",
			pattern: "example.com/*",
			want:    "https://example.com",
		},
		{
			name:    "prefix wildcard",
			pattern: "*example.com",
			want:    "https://example.com",
		},
		{
			name:    "subdomain",
			pattern: "en.wikipedia.org",
			want:    "https://en.wikipedia.org",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			site := SiteConfig{Pattern: tt.pattern}
			got := site.GetCrawlBaseURL()
			if got != tt.want {
				t.Errorf("GetCrawlBaseURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfig_GetCrawlableSites(t *testing.T) {
	config := &Config{
		Sites: []SiteConfig{
			{
				Pattern: "example.com",
				Crawl:   &CrawlConfig{Strategy: "sitemap"},
			},
			{
				Pattern: "test.com",
				Crawl:   nil, // Not crawlable
			},
			{
				Pattern: "wiki.org",
				Crawl:   &CrawlConfig{Strategy: "links"},
			},
		},
	}

	got := config.GetCrawlableSites()
	if len(got) != 2 {
		t.Errorf("GetCrawlableSites() returned %d sites, want 2", len(got))
	}

	if got[0].Pattern != "example.com" {
		t.Errorf("First crawlable site = %v, want example.com", got[0].Pattern)
	}
	if got[1].Pattern != "wiki.org" {
		t.Errorf("Second crawlable site = %v, want wiki.org", got[1].Pattern)
	}
}

func TestMergeChunk(t *testing.T) {
	tests := []struct {
		name     string
		base     ChunkConfig
		override ChunkConfig
		want     ChunkConfig
	}{
		{
			name: "override tokens",
			base: ChunkConfig{
				Tokens:  600,
				Overlap: 75,
			},
			override: ChunkConfig{
				Tokens: 800,
			},
			want: ChunkConfig{
				Tokens:  800,
				Overlap: 75,
			},
		},
		{
			name: "override overlap",
			base: ChunkConfig{
				Tokens:  600,
				Overlap: 75,
			},
			override: ChunkConfig{
				Overlap: 100,
			},
			want: ChunkConfig{
				Tokens:  600,
				Overlap: 100,
			},
		},
		{
			name: "override both",
			base: ChunkConfig{
				Tokens:  600,
				Overlap: 75,
			},
			override: ChunkConfig{
				Tokens:  1000,
				Overlap: 150,
			},
			want: ChunkConfig{
				Tokens:  1000,
				Overlap: 150,
			},
		},
		{
			name: "zero values don't override",
			base: ChunkConfig{
				Tokens:  600,
				Overlap: 75,
			},
			override: ChunkConfig{},
			want: ChunkConfig{
				Tokens:  600,
				Overlap: 75,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeChunk(tt.base, tt.override)
			if got.Tokens != tt.want.Tokens {
				t.Errorf("Tokens = %v, want %v", got.Tokens, tt.want.Tokens)
			}
			if got.Overlap != tt.want.Overlap {
				t.Errorf("Overlap = %v, want %v", got.Overlap, tt.want.Overlap)
			}
		})
	}
}

func TestConfig_ValidateChunk(t *testing.T) {
	t.Run("valid chunk config", func(t *testing.T) {
		cfg := &Config{
			Default: DefaultConfig{
				Chunk: ChunkConfig{
					Tokens:  600,
					Overlap: 75,
				},
			},
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("negative tokens", func(t *testing.T) {
		cfg := &Config{
			Default: DefaultConfig{
				Chunk: ChunkConfig{
					Tokens: -1,
				},
			},
		}
		err := cfg.Validate()
		if err == nil {
			t.Error("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "'tokens' must be >= 0") {
			t.Errorf("Validate() error = %v, want error about negative tokens", err)
		}
	})

	t.Run("negative overlap", func(t *testing.T) {
		cfg := &Config{
			Default: DefaultConfig{
				Chunk: ChunkConfig{
					Overlap: -1,
				},
			},
		}
		err := cfg.Validate()
		if err == nil {
			t.Error("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "'overlap' must be >= 0") {
			t.Errorf("Validate() error = %v, want error about negative overlap", err)
		}
	})

	t.Run("overlap greater than or equal to tokens", func(t *testing.T) {
		cfg := &Config{
			Default: DefaultConfig{
				Chunk: ChunkConfig{
					Tokens:  100,
					Overlap: 100,
				},
			},
		}
		err := cfg.Validate()
		if err == nil {
			t.Error("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "'overlap' (100) must be less than 'tokens' (100)") {
			t.Errorf("Validate() error = %v, want error about overlap >= tokens", err)
		}
	})
}

func TestConfig_ValidateCrawl(t *testing.T) {
	t.Run("valid crawl config with sitemap", func(t *testing.T) {
		cfg := &Config{
			Sites: []SiteConfig{
				{
					Pattern: "example.com",
					Crawl: &CrawlConfig{
						Strategy: "sitemap",
						MaxPages: 1000,
					},
				},
			},
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("valid crawl config with links", func(t *testing.T) {
		cfg := &Config{
			Sites: []SiteConfig{
				{
					Pattern: "example.com",
					Crawl: &CrawlConfig{
						Strategy: "links",
						MaxDepth: 3,
					},
				},
			},
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("invalid crawl strategy", func(t *testing.T) {
		cfg := &Config{
			Sites: []SiteConfig{
				{
					Pattern: "example.com",
					Crawl: &CrawlConfig{
						Strategy: "invalid",
					},
				},
			},
		}
		err := cfg.Validate()
		if err == nil {
			t.Error("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "'strategy' must be one of: sitemap, links, both") {
			t.Errorf("Validate() error = %v, want error about invalid strategy", err)
		}
	})

	t.Run("negative max_pages", func(t *testing.T) {
		cfg := &Config{
			Sites: []SiteConfig{
				{
					Pattern: "example.com",
					Crawl: &CrawlConfig{
						Strategy: "sitemap",
						MaxPages: -1,
					},
				},
			},
		}
		err := cfg.Validate()
		if err == nil {
			t.Error("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "'max_pages' must be >= 0") {
			t.Errorf("Validate() error = %v, want error about negative max_pages", err)
		}
	})

	t.Run("negative max_depth", func(t *testing.T) {
		cfg := &Config{
			Sites: []SiteConfig{
				{
					Pattern: "example.com",
					Crawl: &CrawlConfig{
						Strategy: "links",
						MaxDepth: -1,
					},
				},
			},
		}
		err := cfg.Validate()
		if err == nil {
			t.Error("Validate() error = nil, want error")
		}
		if !strings.Contains(err.Error(), "'max_depth' must be >= 0") {
			t.Errorf("Validate() error = %v, want error about negative max_depth", err)
		}
	})
}
