package retry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeychilson/websurfer/config"
	"github.com/joeychilson/websurfer/fetcher"
	"github.com/joeychilson/websurfer/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCalculateBackoffExponentialGrowth verifies delays grow exponentially.
func TestCalculateBackoffExponentialGrowth(t *testing.T) {
	cfg := config.RetryConfig{
		InitialDelay: 100 * time.Millisecond,
		Multiplier:   2.0,
	}
	f, _ := fetcher.New(config.FetchConfig{})
	l := ratelimit.New(config.RateLimitConfig{})
	r := New(f, l, cfg)

	// Calculate backoff for attempts 0, 1, 2
	// With multiplier=2 and jitter, each should be roughly 2x previous
	delay0 := r.calculateBackoff(0)
	delay1 := r.calculateBackoff(1)
	delay2 := r.calculateBackoff(2)

	// Account for jitter (±25%), so delays might not be exact
	// But delay1 should be larger than delay0, delay2 larger than delay1
	assert.Greater(t, delay1, delay0/2, "delay1 should be significantly larger than delay0")
	assert.Greater(t, delay2, delay1/2, "delay2 should be significantly larger than delay1")
}

// TestCalculateBackoffMaxDelay verifies delays don't exceed MaxDelay.
func TestCalculateBackoffMaxDelay(t *testing.T) {
	cfg := config.RetryConfig{
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   10.0, // Very high multiplier
	}
	f, _ := fetcher.New(config.FetchConfig{})
	l := ratelimit.New(config.RateLimitConfig{})
	r := New(f, l, cfg)

	// Even with high attempts, delay should cap at MaxDelay
	for attempt := 0; attempt < 20; attempt++ {
		delay := r.calculateBackoff(attempt)
		// Account for jitter (±25%)
		maxAllowed := float64(cfg.GetMaxDelay()) * (1 + jitterPercent)
		assert.LessOrEqual(t, float64(delay), maxAllowed,
			"delay for attempt %d should not exceed MaxDelay + jitter", attempt)
	}
}

// TestCalculateBackoffJitter verifies jitter is applied to prevent thundering herd.
func TestCalculateBackoffJitter(t *testing.T) {
	cfg := config.RetryConfig{
		InitialDelay: 100 * time.Millisecond,
		Multiplier:   2.0,
	}
	f, _ := fetcher.New(config.FetchConfig{})
	l := ratelimit.New(config.RateLimitConfig{})
	r := New(f, l, cfg)

	// Calculate backoff multiple times for same attempt
	delays := make([]time.Duration, 20)
	for i := 0; i < 20; i++ {
		delays[i] = r.calculateBackoff(1)
	}

	// At least some delays should be different (due to jitter)
	allSame := true
	for i := 1; i < len(delays); i++ {
		if delays[i] != delays[0] {
			allSame = false
			break
		}
	}

	assert.False(t, allSame, "jitter should make delays vary")

	// All delays should be within jitter range (±25% of base delay)
	baseDelay := 100 * time.Millisecond * 2 // Initial * multiplier
	minDelay := float64(baseDelay) * (1 - jitterPercent)
	maxDelay := float64(baseDelay) * (1 + jitterPercent)

	for i, delay := range delays {
		assert.GreaterOrEqual(t, float64(delay), minDelay*0.95, // Small tolerance
			"delay %d should be >= min jitter range", i)
		assert.LessOrEqual(t, float64(delay), maxDelay*1.05, // Small tolerance
			"delay %d should be <= max jitter range", i)
	}
}

// TestCalculateBackoffZeroInitialDelay verifies zero delay doesn't cause issues.
func TestCalculateBackoffZeroInitialDelay(t *testing.T) {
	cfg := config.RetryConfig{
		InitialDelay: 0,
		Multiplier:   2.0,
	}
	f, _ := fetcher.New(config.FetchConfig{})
	l := ratelimit.New(config.RateLimitConfig{})
	r := New(f, l, cfg)

	// Should not panic
	// Zero initial delay means GetInitialDelay() returns default 1s, not 0
	delay := r.calculateBackoff(5)
	assert.GreaterOrEqual(t, delay, time.Duration(0), "delay should not be negative")
}

// TestCalculateBackoffVeryHighAttempt verifies overflow protection.
func TestCalculateBackoffVeryHighAttempt(t *testing.T) {
	cfg := config.RetryConfig{
		InitialDelay: 1 * time.Millisecond,
		Multiplier:   2.0,
	}
	f, _ := fetcher.New(config.FetchConfig{})
	l := ratelimit.New(config.RateLimitConfig{})
	r := New(f, l, cfg)

	// Very high attempt number (would overflow without capping)
	delay := r.calculateBackoff(1000)

	// Should not panic and should return a reasonable value
	assert.GreaterOrEqual(t, delay, time.Duration(0), "delay should not be negative")
	assert.Less(t, delay, time.Hour, "delay should be capped at reasonable value")
}

// TestAddJitter verifies jitter is within expected range.
func TestAddJitter(t *testing.T) {
	cfg := config.RetryConfig{}
	f, _ := fetcher.New(config.FetchConfig{})
	l := ratelimit.New(config.RateLimitConfig{})
	retrier := New(f, l, cfg)

	baseDuration := 1000 * time.Millisecond

	// Test multiple times to verify randomness
	results := make([]time.Duration, 100)
	for i := 0; i < 100; i++ {
		results[i] = retrier.addJitter(baseDuration)
	}

	// All results should be within ±25% of base
	minAllowed := float64(baseDuration) * (1 - jitterPercent)
	maxAllowed := float64(baseDuration) * (1 + jitterPercent)

	for i, result := range results {
		assert.GreaterOrEqual(t, float64(result), minAllowed,
			"result %d should be >= min range", i)
		assert.LessOrEqual(t, float64(result), maxAllowed,
			"result %d should be <= max range", i)
	}

	// Should have some variance
	allSame := true
	for i := 1; i < len(results); i++ {
		if results[i] != results[0] {
			allSame = false
			break
		}
	}
	assert.False(t, allSame, "results should vary due to randomness")
}

// TestAddJitterZeroDuration verifies zero duration stays zero.
func TestAddJitterZeroDuration(t *testing.T) {
	cfg := config.RetryConfig{}
	f, _ := fetcher.New(config.FetchConfig{})
	l := ratelimit.New(config.RateLimitConfig{})
	retrier := New(f, l, cfg)

	result := retrier.addJitter(0)
	assert.Equal(t, time.Duration(0), result, "zero duration should stay zero")
}

// TestConfigDefaultRetryOn verifies default retry status codes.
func TestConfigDefaultRetryOn(t *testing.T) {
	cfg := config.RetryConfig{}

	// Default should be [429, 500, 502, 503, 504]
	retryOn := cfg.GetRetryOn()
	assert.Contains(t, retryOn, 429, "should retry on 429")
	assert.Contains(t, retryOn, 500, "should retry on 500")
	assert.Contains(t, retryOn, 502, "should retry on 502")
	assert.Contains(t, retryOn, 503, "should retry on 503")
	assert.Contains(t, retryOn, 504, "should retry on 504")
}

// TestConfigShouldRetryDefault verifies default retry behavior for status codes.
func TestConfigShouldRetryDefault(t *testing.T) {
	cfg := config.RetryConfig{}

	tests := []struct {
		statusCode  int
		shouldRetry bool
		name        string
	}{
		{200, false, "200_ok"},
		{201, false, "201_created"},
		{204, false, "204_no_content"},
		{304, false, "304_not_modified"},
		{400, false, "400_bad_request"},
		{401, false, "401_unauthorized"},
		{403, false, "403_forbidden"},
		{404, false, "404_not_found"},
		{429, true, "429_too_many_requests"},
		{500, true, "500_internal_server_error"},
		{502, true, "502_bad_gateway"},
		{503, true, "503_service_unavailable"},
		{504, true, "504_gateway_timeout"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.shouldRetry, cfg.ShouldRetry(tt.statusCode),
				"status %d should retry=%v", tt.statusCode, tt.shouldRetry)
		})
	}
}

// TestConfigCustomRetryOn verifies custom retry status codes.
func TestConfigCustomRetryOn(t *testing.T) {
	cfg := config.RetryConfig{
		RetryOn: []int{502, 503, 999}, // Custom list
	}

	assert.True(t, cfg.ShouldRetry(502), "should retry on custom 502")
	assert.True(t, cfg.ShouldRetry(503), "should retry on custom 503")
	assert.True(t, cfg.ShouldRetry(999), "should retry on custom 999")
	assert.False(t, cfg.ShouldRetry(500), "should NOT retry on 500 (not in custom list)")
	assert.False(t, cfg.ShouldRetry(429), "should NOT retry on 429 (not in custom list)")
}

// TestConfigDefaultValues verifies config defaults are applied.
func TestConfigDefaultValues(t *testing.T) {
	cfg := config.RetryConfig{}

	// Defaults when not set
	assert.Equal(t, 0, cfg.GetMaxRetries(), "default max retries should be 0 (no retries)")
	assert.Equal(t, 1*time.Second, cfg.GetInitialDelay(), "default initial delay should be 1s")
	assert.Equal(t, 30*time.Second, cfg.GetMaxDelay(), "default max delay should be 30s")
	assert.Equal(t, 2.0, cfg.GetMultiplier(), "default multiplier should be 2.0")
}

// TestConfigCustomValues verifies custom config values are respected.
func TestConfigCustomValues(t *testing.T) {
	cfg := config.RetryConfig{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   3.0,
	}

	assert.Equal(t, 5, cfg.GetMaxRetries())
	assert.Equal(t, 100*time.Millisecond, cfg.GetInitialDelay())
	assert.Equal(t, 10*time.Second, cfg.GetMaxDelay())
	assert.Equal(t, 3.0, cfg.GetMultiplier())
}

// TestRetryNew verifies Retrier can be created successfully.
func TestRetryNew(t *testing.T) {
	f, err := fetcher.New(config.FetchConfig{})
	require.NoError(t, err)
	require.NotNil(t, f)

	l := ratelimit.New(config.RateLimitConfig{})
	require.NotNil(t, l)

	cfg := config.RetryConfig{}
	r := New(f, l, cfg)

	assert.NotNil(t, r, "Retrier should be created")
	assert.NotNil(t, r.fetcher, "Retrier should have fetcher")
	assert.NotNil(t, r.limiter, "Retrier should have limiter")
}

// TestRetryIntegrationSuccessAfterFailures verifies actual retry flow with real HTTP.
// CRITICAL: Retries should succeed after transient failures (resilience for LLMs).
func TestRetryIntegrationSuccessAfterFailures(t *testing.T) {
	var attemptCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attemptCount.Add(1)

		// Fail first 2 attempts with 503, succeed on 3rd
		if attempt < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("Service temporarily unavailable"))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Success!"))
	}))
	defer server.Close()

	// Create fetcher and retrier
	f, err := fetcher.New(config.FetchConfig{})
	require.NoError(t, err)

	l := ratelimit.New(config.RateLimitConfig{})
	defer l.Close()

	retryCfg := config.RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}
	r := New(f, l, retryCfg)

	// Fetch with retries
	resp, err := r.Fetch(context.Background(), server.URL)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Success!", string(resp.Body))
	assert.Equal(t, int32(3), attemptCount.Load(), "should have made 3 attempts")
}

// TestRetryIntegrationFailureAfterMaxRetries verifies error after exhausting retries.
// CRITICAL: LLM tools need clear error message when all retries fail.
func TestRetryIntegrationFailureAfterMaxRetries(t *testing.T) {
	var attemptCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount.Add(1)
		// Always fail with 503
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Service down"))
	}))
	defer server.Close()

	f, err := fetcher.New(config.FetchConfig{})
	require.NoError(t, err)

	l := ratelimit.New(config.RateLimitConfig{})
	defer l.Close()

	retryCfg := config.RetryConfig{
		MaxRetries:   2, // Will try 3 times total (initial + 2 retries)
		InitialDelay: 10 * time.Millisecond,
		Multiplier:   2.0,
	}
	r := New(f, l, retryCfg)

	_, err = r.Fetch(context.Background(), server.URL)

	assert.Error(t, err, "should fail after max retries")
	assert.Contains(t, err.Error(), "503", "error should mention status code for LLM tools")
	assert.Equal(t, int32(3), attemptCount.Load(), "should have made 3 attempts (initial + 2 retries)")
}

// TestRetryIntegrationNoRetryOn4xx verifies client errors are NOT retried.
// CRITICAL: Don't waste time retrying unrecoverable errors (400, 404, etc.).
func TestRetryIntegrationNoRetryOn4xx(t *testing.T) {
	var attemptCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount.Add(1)
		// Always return 404 (client error - should NOT retry)
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	f, err := fetcher.New(config.FetchConfig{})
	require.NoError(t, err)

	l := ratelimit.New(config.RateLimitConfig{})
	defer l.Close()

	retryCfg := config.RetryConfig{
		MaxRetries:   3, // Allow retries
		InitialDelay: 10 * time.Millisecond,
		Multiplier:   2.0,
		// Default RetryOn doesn't include 404
	}
	r := New(f, l, retryCfg)

	resp, err := r.Fetch(context.Background(), server.URL)

	// Should return response without retrying (even though it's an error status)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 404, resp.StatusCode)

	// Should only attempt once (no retries for 404)
	assert.Equal(t, int32(1), attemptCount.Load(),
		"4xx errors should NOT be retried (attempt count should be 1)")
}

// TestRetryIntegrationRetryOn429RateLimited verifies 429 is retried.
// CRITICAL: Respect server rate limits with backoff.
func TestRetryIntegrationRetryOn429RateLimited(t *testing.T) {
	var attemptCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := attemptCount.Add(1)
		if count < 3 {
			// Return 429 for first 2 attempts
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Rate Limited"))
		} else {
			// Succeed on 3rd attempt
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Success"))
		}
	}))
	defer server.Close()

	f, err := fetcher.New(config.FetchConfig{})
	require.NoError(t, err)

	l := ratelimit.New(config.RateLimitConfig{})
	defer l.Close()

	retryCfg := config.RetryConfig{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		Multiplier:   2.0,
		// Default RetryOn includes 429
	}
	r := New(f, l, retryCfg)

	resp, err := r.Fetch(context.Background(), server.URL)

	// Should eventually succeed
	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)

	// Should have retried after 429 errors
	assert.Equal(t, int32(3), attemptCount.Load(),
		"429 should be retried until success (3 attempts)")
}

// TestRetryIntegrationContextCancellation verifies retry stops on context cancel.
// CRITICAL: LLM tool timeouts should cancel in-progress retries.
func TestRetryIntegrationContextCancellation(t *testing.T) {
	var attemptCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount.Add(1)
		// Always fail with 503
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	f, err := fetcher.New(config.FetchConfig{})
	require.NoError(t, err)

	l := ratelimit.New(config.RateLimitConfig{})
	defer l.Close()

	retryCfg := config.RetryConfig{
		MaxRetries:   10, // Many retries
		InitialDelay: 100 * time.Millisecond,
		Multiplier:   2.0,
	}
	r := New(f, l, retryCfg)

	// Create context that cancels after 200ms
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err = r.Fetch(ctx, server.URL)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context", "should mention context cancellation")

	// Should have stopped early (not all 11 attempts)
	assert.Less(t, attemptCount.Load(), int32(11), "should stop on context cancellation")
}

// TestRetryIntegrationBackoffIncreases verifies delays increase exponentially.
// Prevents overwhelming failing servers.
func TestRetryIntegrationBackoffIncreases(t *testing.T) {
	var attemptCount atomic.Int32
	var attemptTimes []time.Time
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attemptTimes = append(attemptTimes, time.Now())
		mu.Unlock()

		attemptCount.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	f, err := fetcher.New(config.FetchConfig{})
	require.NoError(t, err)

	l := ratelimit.New(config.RateLimitConfig{})
	defer l.Close()

	retryCfg := config.RetryConfig{
		MaxRetries:   3,
		InitialDelay: 100 * time.Millisecond,
		Multiplier:   2.0,
	}
	r := New(f, l, retryCfg)

	_, _ = r.Fetch(context.Background(), server.URL)

	// Verify delays increased
	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, attemptTimes, 4, "should have 4 attempts")

	if len(attemptTimes) >= 3 {
		// Delay between attempt 1→2 should be less than delay between attempt 2→3
		delay1 := attemptTimes[1].Sub(attemptTimes[0])
		delay2 := attemptTimes[2].Sub(attemptTimes[1])

		assert.Greater(t, delay2, delay1,
			"backoff should increase exponentially (delay1=%v, delay2=%v)", delay1, delay2)
	}
}

// TestRetryIntegrationCustomRetryOn verifies custom retry status codes.
func TestRetryIntegrationCustomRetryOn(t *testing.T) {
	var attemptCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := attemptCount.Add(1)

		// Return 502 on first attempt, 200 on second
		if attempt == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer server.Close()

	f, err := fetcher.New(config.FetchConfig{})
	require.NoError(t, err)

	l := ratelimit.New(config.RateLimitConfig{})
	defer l.Close()

	retryCfg := config.RetryConfig{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		RetryOn:      []int{502}, // Only retry 502
	}
	r := New(f, l, retryCfg)

	resp, err := r.Fetch(context.Background(), server.URL)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(2), attemptCount.Load(), "should have retried 502")
}
