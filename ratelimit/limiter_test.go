package ratelimit

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeychilson/websurfer/config"
)

func TestLimiter_Wait(t *testing.T) {
	t.Run("no rate limit when disabled", func(t *testing.T) {
		limiter := New(config.RateLimitConfig{})

		start := time.Now()
		for i := 0; i < 10; i++ {
			if err := limiter.Wait(context.Background(), "https://example.com"); err != nil {
				t.Fatalf("Wait() error = %v", err)
			}
		}
		elapsed := time.Since(start)

		if elapsed > 100*time.Millisecond {
			t.Errorf("elapsed time = %v, want < 100ms (no rate limiting)", elapsed)
		}
	})

	t.Run("enforces delay between requests", func(t *testing.T) {
		limiter := New(config.RateLimitConfig{
			Delay: 100 * time.Millisecond,
		})

		start := time.Now()
		for i := 0; i < 3; i++ {
			if err := limiter.Wait(context.Background(), "https://example.com"); err != nil {
				t.Fatalf("Wait() error = %v", err)
			}
		}
		elapsed := time.Since(start)

		expected := 200 * time.Millisecond
		if elapsed < expected {
			t.Errorf("elapsed time = %v, want >= %v", elapsed, expected)
		}
	})

	t.Run("enforces requests per second", func(t *testing.T) {
		limiter := New(config.RateLimitConfig{
			RequestsPerSecond: 10.0,
		})

		start := time.Now()
		for i := 0; i < 3; i++ {
			if err := limiter.Wait(context.Background(), "https://example.com"); err != nil {
				t.Fatalf("Wait() error = %v", err)
			}
		}
		elapsed := time.Since(start)

		expected := 200 * time.Millisecond
		if elapsed < expected {
			t.Errorf("elapsed time = %v, want >= %v", elapsed, expected)
		}
	})

	t.Run("burst allows temporary bursts", func(t *testing.T) {
		limiter := New(config.RateLimitConfig{
			RequestsPerSecond: 1.0,
			Burst:             5,
		})

		start := time.Now()
		for i := 0; i < 5; i++ {
			if err := limiter.Wait(context.Background(), "https://example.com"); err != nil {
				t.Fatalf("Wait() error = %v", err)
			}
		}
		elapsed := time.Since(start)

		if elapsed > 100*time.Millisecond {
			t.Errorf("burst failed: elapsed time = %v, want < 100ms", elapsed)
		}
	})

	t.Run("isolates domains", func(t *testing.T) {
		limiter := New(config.RateLimitConfig{
			Delay: 100 * time.Millisecond,
		})

		start := time.Now()

		if err := limiter.Wait(context.Background(), "https://example.com"); err != nil {
			t.Fatalf("Wait() error = %v", err)
		}
		if err := limiter.Wait(context.Background(), "https://other.com"); err != nil {
			t.Fatalf("Wait() error = %v", err)
		}

		elapsed := time.Since(start)

		if elapsed > 50*time.Millisecond {
			t.Errorf("domains not isolated: elapsed = %v, want < 50ms", elapsed)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		limiter := New(config.RateLimitConfig{
			Delay: 1 * time.Second,
		})

		limiter.Wait(context.Background(), "https://example.com")

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		err := limiter.Wait(ctx, "https://example.com")
		if err == nil {
			t.Error("Wait() error = nil, want context error")
		}
	})
}

func TestLimiter_MaxConcurrent(t *testing.T) {
	t.Run("limits concurrent requests", func(t *testing.T) {
		limiter := New(config.RateLimitConfig{
			MaxConcurrent: 2,
		})

		var concurrent int32
		var maxConcurrent int32
		var wg sync.WaitGroup

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				if err := limiter.Wait(context.Background(), "https://example.com"); err != nil {
					t.Errorf("Wait() error = %v", err)
					return
				}
				defer limiter.Release("https://example.com")

				current := atomic.AddInt32(&concurrent, 1)
				defer atomic.AddInt32(&concurrent, -1)

				for {
					max := atomic.LoadInt32(&maxConcurrent)
					if current <= max {
						break
					}
					if atomic.CompareAndSwapInt32(&maxConcurrent, max, current) {
						break
					}
				}

				time.Sleep(10 * time.Millisecond)
			}()
		}

		wg.Wait()

		max := atomic.LoadInt32(&maxConcurrent)
		if max > 2 {
			t.Errorf("maxConcurrent = %d, want <= 2", max)
		}
	})

	t.Run("isolates concurrent limits by domain", func(t *testing.T) {
		limiter := New(config.RateLimitConfig{
			MaxConcurrent: 1,
		})

		done := make(chan bool, 2)

		go func() {
			limiter.Wait(context.Background(), "https://example.com")
			time.Sleep(50 * time.Millisecond)
			limiter.Release("https://example.com")
			done <- true
		}()

		go func() {
			time.Sleep(10 * time.Millisecond)
			start := time.Now()
			limiter.Wait(context.Background(), "https://other.com")
			elapsed := time.Since(start)
			limiter.Release("https://other.com")

			if elapsed > 20*time.Millisecond {
				t.Errorf("different domain was blocked: elapsed = %v", elapsed)
			}
			done <- true
		}()

		<-done
		<-done
	})
}

func TestLimiter_RetryAfter(t *testing.T) {
	t.Run("respects Retry-After seconds", func(t *testing.T) {
		limiter := New(config.RateLimitConfig{
			RespectRetryAfter: true,
		})

		headers := http.Header{}
		headers.Set("Retry-After", "1")

		start := time.Now()
		limiter.UpdateRetryAfter("https://example.com", headers)

		if err := limiter.Wait(context.Background(), "https://example.com"); err != nil {
			t.Fatalf("Wait() error = %v", err)
		}
		elapsed := time.Since(start)

		if elapsed < 900*time.Millisecond {
			t.Errorf("elapsed = %v, want >= 900ms (Retry-After not respected)", elapsed)
		}
	})

	t.Run("respects Retry-After HTTP-date", func(t *testing.T) {
		limiter := New(config.RateLimitConfig{
			RespectRetryAfter: true,
		})

		headers := http.Header{}
		retryTime := time.Now().Add(2 * time.Second).Truncate(time.Second)
		headers.Set("Retry-After", retryTime.UTC().Format(http.TimeFormat))

		start := time.Now()
		limiter.UpdateRetryAfter("https://example.com", headers)

		if err := limiter.Wait(context.Background(), "https://example.com"); err != nil {
			t.Fatalf("Wait() error = %v", err)
		}
		elapsed := time.Since(start)

		if elapsed < 900*time.Millisecond {
			t.Errorf("elapsed = %v, want >= 900ms (Retry-After not respected)", elapsed)
		}
	})

	t.Run("ignores Retry-After when disabled", func(t *testing.T) {
		limiter := New(config.RateLimitConfig{
			RespectRetryAfter: false,
		})

		headers := http.Header{}
		headers.Set("Retry-After", "10")

		limiter.UpdateRetryAfter("https://example.com", headers)

		start := time.Now()
		if err := limiter.Wait(context.Background(), "https://example.com"); err != nil {
			t.Fatalf("Wait() error = %v", err)
		}
		elapsed := time.Since(start)

		if elapsed > 100*time.Millisecond {
			t.Errorf("elapsed = %v, want < 100ms (should ignore Retry-After)", elapsed)
		}
	})

	t.Run("isolates Retry-After by domain", func(t *testing.T) {
		limiter := New(config.RateLimitConfig{
			RespectRetryAfter: true,
		})

		headers := http.Header{}
		headers.Set("Retry-After", "1")

		limiter.UpdateRetryAfter("https://example.com", headers)

		start := time.Now()
		if err := limiter.Wait(context.Background(), "https://other.com"); err != nil {
			t.Fatalf("Wait() error = %v", err)
		}
		elapsed := time.Since(start)

		if elapsed > 100*time.Millisecond {
			t.Errorf("elapsed = %v, want < 100ms (different domain should not wait)", elapsed)
		}
	})
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    string
		wantErr bool
	}{
		{
			name: "simple URL",
			url:  "https://example.com/path",
			want: "example.com",
		},
		{
			name: "URL with port",
			url:  "https://example.com:8080/path",
			want: "example.com:8080",
		},
		{
			name: "subdomain",
			url:  "https://api.example.com",
			want: "api.example.com",
		},
		{
			name:    "invalid URL",
			url:     "://invalid",
			wantErr: true,
		},
		{
			name:    "no host",
			url:     "/relative/path",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractDomain(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractDomain() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractDomain() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Run("parses seconds", func(t *testing.T) {
		result := parseRetryAfter("120")
		if result.IsZero() {
			t.Error("parseRetryAfter(\"120\") returned zero time")
		}

		expected := time.Now().Add(120 * time.Second)
		diff := result.Sub(expected).Abs()
		if diff > time.Second {
			t.Errorf("parsed time diff = %v, want < 1s", diff)
		}
	})

	t.Run("parses HTTP-date", func(t *testing.T) {
		future := time.Now().Add(1 * time.Hour)
		dateStr := future.UTC().Format(http.TimeFormat)

		result := parseRetryAfter(dateStr)
		if result.IsZero() {
			t.Error("parseRetryAfter() returned zero time for HTTP-date")
		}

		diff := result.Sub(future).Abs()
		if diff > time.Second {
			t.Errorf("parsed time diff = %v, want < 1s", diff)
		}
	})

	t.Run("returns zero for invalid", func(t *testing.T) {
		result := parseRetryAfter("invalid")
		if !result.IsZero() {
			t.Error("parseRetryAfter(\"invalid\") should return zero time")
		}
	})
}

func TestLimiter_ConcurrentAccess(t *testing.T) {
	t.Run("thread-safe domain creation", func(t *testing.T) {
		limiter := New(config.RateLimitConfig{
			Delay: 10 * time.Millisecond,
		})

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := limiter.Wait(context.Background(), "https://example.com"); err != nil {
					t.Errorf("Wait() error = %v", err)
				}
			}()
		}

		wg.Wait()
	})
}
