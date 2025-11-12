package ratelimit

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/joeychilson/websurfer/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLimiterDisabled verifies that disabled limiter allows all requests immediately.
func TestLimiterDisabled(t *testing.T) {
	cfg := config.RateLimitConfig{
		// All fields zero - limiter disabled
	}
	limiter := New(cfg)
	defer limiter.Close()

	ctx := context.Background()
	url := "https://example.com/page"

	// Should allow requests immediately
	start := time.Now()
	err := limiter.Wait(ctx, url)
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.Less(t, elapsed, 10*time.Millisecond, "disabled limiter should not delay")
}

// TestLimiterRequestsPerSecond verifies rate limiting with requests per second.
func TestLimiterRequestsPerSecond(t *testing.T) {
	cfg := config.RateLimitConfig{
		RequestsPerSecond: 2.0, // 2 requests per second = 500ms delay
		Burst:             1,
	}
	limiter := New(cfg)
	defer limiter.Close()

	ctx := context.Background()
	url := "https://example.com/page"

	// First request should be immediate (burst)
	start := time.Now()
	err := limiter.Wait(ctx, url)
	require.NoError(t, err)
	assert.Less(t, time.Since(start), 50*time.Millisecond)

	// Second request should wait ~500ms
	start = time.Now()
	err = limiter.Wait(ctx, url)
	require.NoError(t, err)
	elapsed := time.Since(start)

	// Should wait approximately 500ms (allow 200ms tolerance)
	assert.Greater(t, elapsed, 400*time.Millisecond, "should enforce rate limit")
	assert.Less(t, elapsed, 700*time.Millisecond, "should not wait too long")
}

// TestLimiterDelay verifies rate limiting with explicit delay.
func TestLimiterDelay(t *testing.T) {
	cfg := config.RateLimitConfig{
		Delay: 200 * time.Millisecond,
		Burst: 1,
	}
	limiter := New(cfg)
	defer limiter.Close()

	ctx := context.Background()
	url := "https://example.com/page"

	// First request (burst)
	err := limiter.Wait(ctx, url)
	require.NoError(t, err)

	// Second request should wait 200ms
	start := time.Now()
	err = limiter.Wait(ctx, url)
	require.NoError(t, err)
	elapsed := time.Since(start)

	assert.Greater(t, elapsed, 150*time.Millisecond, "should wait at least 150ms")
	assert.Less(t, elapsed, 300*time.Millisecond, "should not wait more than 300ms")
}

// TestLimiterPerDomainIsolation verifies that different domains have independent limits.
func TestLimiterPerDomainIsolation(t *testing.T) {
	cfg := config.RateLimitConfig{
		Delay: 200 * time.Millisecond,
		Burst: 1,
	}
	limiter := New(cfg)
	defer limiter.Close()

	ctx := context.Background()

	// Two different domains
	url1 := "https://example.com/page"
	url2 := "https://different.com/page"

	// First request to domain1
	err := limiter.Wait(ctx, url1)
	require.NoError(t, err)

	// Request to domain2 should be immediate (different limiter)
	start := time.Now()
	err = limiter.Wait(ctx, url2)
	require.NoError(t, err)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 50*time.Millisecond, "different domains should not share rate limits")
}

// TestLimiterContextCancellation verifies that context cancellation stops waiting.
func TestLimiterContextCancellation(t *testing.T) {
	cfg := config.RateLimitConfig{
		Delay: 5 * time.Second, // Long delay
		Burst: 0,
	}
	limiter := New(cfg)
	defer limiter.Close()

	url := "https://example.com/page"

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// First request exhausts burst
	_ = limiter.Wait(context.Background(), url)

	// Second request should timeout
	start := time.Now()
	err := limiter.Wait(ctx, url)
	elapsed := time.Since(start)

	assert.Error(t, err, "should return error when context cancelled")
	assert.Less(t, elapsed, 200*time.Millisecond, "should stop quickly on cancellation")
}

// TestLimiterConcurrencyLimit verifies max concurrent requests per domain.
func TestLimiterConcurrencyLimit(t *testing.T) {
	cfg := config.RateLimitConfig{
		MaxConcurrent: 2, // Max 2 concurrent requests
	}
	limiter := New(cfg)
	defer limiter.Close()

	url := "https://example.com/page"
	ctx := context.Background()

	// Start 2 concurrent requests (should succeed)
	err1 := limiter.Wait(ctx, url)
	require.NoError(t, err1)

	err2 := limiter.Wait(ctx, url)
	require.NoError(t, err2)

	// Third request should block
	blocked := make(chan bool, 1)
	go func() {
		ctxTimeout, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err := limiter.Wait(ctxTimeout, url)
		blocked <- (err != nil) // Should timeout
	}()

	// Wait for result
	select {
	case wasBlocked := <-blocked:
		assert.True(t, wasBlocked, "third concurrent request should be blocked")
	case <-time.After(200 * time.Millisecond):
		t.Fatal("test timed out")
	}

	// Release one slot
	limiter.Release(url)

	// Now third request should succeed
	err3 := limiter.Wait(ctx, url)
	assert.NoError(t, err3, "request should succeed after release")
}

// TestLimiterRetryAfterHeader verifies Retry-After header handling.
func TestLimiterRetryAfterHeader(t *testing.T) {
	respectRetryAfter := true
	cfg := config.RateLimitConfig{
		RespectRetryAfter: &respectRetryAfter,
	}
	limiter := New(cfg)
	defer limiter.Close()

	url := "https://example.com/page"
	ctx := context.Background()

	// Set Retry-After to 1 second from now
	headers := http.Header{}
	headers.Set("Retry-After", "1")
	limiter.UpdateRetryAfter(url, headers)

	// Request should wait approximately 1 second
	start := time.Now()
	err := limiter.Wait(ctx, url)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Greater(t, elapsed, 900*time.Millisecond, "should respect Retry-After header")
	assert.Less(t, elapsed, 1200*time.Millisecond, "should not wait too long")
}

// TestLimiterRetryAfterHTTPDate verifies Retry-After with HTTP date format.
func TestLimiterRetryAfterHTTPDate(t *testing.T) {
	respectRetryAfter := true
	cfg := config.RateLimitConfig{
		RespectRetryAfter: &respectRetryAfter,
	}
	limiter := New(cfg)
	defer limiter.Close()

	url := "https://example.com/page"
	ctx := context.Background()

	// Set Retry-After using HTTP date format
	// The HTTP time format only has second precision, so we need to be careful
	retryTime := time.Now().Add(2 * time.Second).Truncate(time.Second)
	headers := http.Header{}
	headers.Set("Retry-After", retryTime.UTC().Format(http.TimeFormat))

	// Update before we start timing
	limiter.UpdateRetryAfter(url, headers)

	// Request should wait
	start := time.Now()
	err := limiter.Wait(ctx, url)
	elapsed := time.Since(start)

	require.NoError(t, err)
	// HTTP time format has second precision, so we should wait approximately 1-2 seconds
	// depending on when in the second we started
	assert.Greater(t, elapsed, 900*time.Millisecond, "should respect Retry-After date")
	assert.Less(t, elapsed, 3*time.Second, "should not wait too long")
}

// TestLimiterRetryAfterDisabled verifies that Retry-After is ignored when disabled.
func TestLimiterRetryAfterDisabled(t *testing.T) {
	respectRetryAfter := false
	cfg := config.RateLimitConfig{
		RespectRetryAfter: &respectRetryAfter,
	}
	limiter := New(cfg)
	defer limiter.Close()

	url := "https://example.com/page"
	ctx := context.Background()

	// Set Retry-After
	headers := http.Header{}
	headers.Set("Retry-After", "10") // 10 seconds
	limiter.UpdateRetryAfter(url, headers)

	// Request should be immediate
	start := time.Now()
	err := limiter.Wait(ctx, url)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Less(t, elapsed, 100*time.Millisecond, "should ignore Retry-After when disabled")
}

// TestLimiterClosedState verifies that closed limiter returns errors.
func TestLimiterClosedState(t *testing.T) {
	cfg := config.RateLimitConfig{
		Delay: 100 * time.Millisecond,
	}
	limiter := New(cfg)

	// Close the limiter
	limiter.Close()

	// Wait should return error
	err := limiter.Wait(context.Background(), "https://example.com")
	assert.Error(t, err, "closed limiter should return error")
	assert.Contains(t, err.Error(), "closed", "error should mention closed state")

	// Release should not panic
	assert.NotPanics(t, func() {
		limiter.Release("https://example.com")
	})
}

// TestLimiterMultipleClose verifies that closing multiple times is safe.
func TestLimiterMultipleClose(t *testing.T) {
	cfg := config.RateLimitConfig{}
	limiter := New(cfg)

	// Close multiple times should not panic
	assert.NotPanics(t, func() {
		limiter.Close()
		limiter.Close()
		limiter.Close()
	})
}

// TestLimiterConcurrentAccess verifies thread safety with concurrent access.
func TestLimiterConcurrentAccess(t *testing.T) {
	cfg := config.RateLimitConfig{
		Delay: 10 * time.Millisecond,
		Burst: 10,
	}
	limiter := New(cfg)
	defer limiter.Close()

	ctx := context.Background()
	urls := []string{
		"https://example.com/1",
		"https://example.com/2",
		"https://different.com/1",
		"https://another.com/1",
	}

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Launch 100 concurrent requests across multiple domains
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			url := urls[idx%len(urls)]
			if err := limiter.Wait(ctx, url); err != nil {
				errors <- err
			}
			limiter.Release(url)
		}(i)
	}

	wg.Wait()
	close(errors)

	// No errors should occur
	errorCount := 0
	for err := range errors {
		t.Errorf("unexpected error: %v", err)
		errorCount++
	}
	assert.Equal(t, 0, errorCount, "should handle concurrent access safely")
}

// TestParseRetryAfterSeconds verifies parsing Retry-After as seconds.
func TestParseRetryAfterSeconds(t *testing.T) {
	tests := []struct {
		value    string
		expected time.Duration
	}{
		{"0", 0},
		{"1", 1 * time.Second},
		{"30", 30 * time.Second},
		{"3600", 3600 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			start := time.Now()
			result := parseRetryAfter(tt.value)

			if tt.expected == 0 {
				assert.InDelta(t, start.Unix(), result.Unix(), 1.0)
			} else {
				expectedTime := start.Add(tt.expected)
				assert.InDelta(t, expectedTime.Unix(), result.Unix(), 1.0)
			}
		})
	}
}

// TestParseRetryAfterInvalid verifies invalid Retry-After values return zero time.
func TestParseRetryAfterInvalid(t *testing.T) {
	invalid := []string{
		"invalid",
		"not a number",
		"",
		"abc123",
	}

	for _, val := range invalid {
		t.Run(val, func(t *testing.T) {
			result := parseRetryAfter(val)
			assert.True(t, result.IsZero(), "invalid value should return zero time")
		})
	}
}

// TestLimiterBurstAllowsInitialRequests verifies burst allows initial requests through.
func TestLimiterBurstAllowsInitialRequests(t *testing.T) {
	cfg := config.RateLimitConfig{
		RequestsPerSecond: 1.0, // 1 request per second
		Burst:             5,   // Allow 5 initial requests
	}
	limiter := New(cfg)
	defer limiter.Close()

	ctx := context.Background()
	url := "https://example.com/page"

	// First 5 requests should be fast (burst)
	start := time.Now()
	for i := 0; i < 5; i++ {
		err := limiter.Wait(ctx, url)
		require.NoError(t, err)
	}
	elapsed := time.Since(start)

	// All 5 should complete quickly
	assert.Less(t, elapsed, 500*time.Millisecond, "burst should allow initial requests quickly")

	// 6th request should wait
	start = time.Now()
	err := limiter.Wait(ctx, url)
	require.NoError(t, err)
	elapsed = time.Since(start)

	// Should wait approximately 1 second
	assert.Greater(t, elapsed, 900*time.Millisecond, "should enforce rate limit after burst")
}

// TestLimiterQueueTimeout verifies queueing with timeout (simulates 30s wait policy).
func TestLimiterQueueTimeout(t *testing.T) {
	cfg := config.RateLimitConfig{
		MaxConcurrent: 1, // Only 1 concurrent request
	}
	limiter := New(cfg)
	defer limiter.Close()

	url := "https://example.com/page"

	// Occupy the slot
	err := limiter.Wait(context.Background(), url)
	require.NoError(t, err)

	// Try to wait with 30s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start waiting
	done := make(chan bool)
	go func() {
		start := time.Now()
		err := limiter.Wait(ctx, url)
		elapsed := time.Since(start)

		// If slot becomes available within 30s, should succeed
		// Otherwise should timeout
		if err != nil {
			assert.Greater(t, elapsed, 29*time.Second, "should wait close to 30s before timeout")
		}
		done <- true
	}()

	// Release after 100ms
	time.Sleep(100 * time.Millisecond)
	limiter.Release(url)

	// Should complete quickly after release
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("should have completed after release")
	}
}
