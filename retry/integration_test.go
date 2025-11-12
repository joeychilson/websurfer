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
// NOTE: Fetcher returns error for non-2xx, so retry sees it as failure
func TestRetryIntegrationNoRetryOn4xx(t *testing.T) {
	t.Skip("TODO: Fetcher returns error for non-2xx status codes, so retry logic can't distinguish 404 vs 500")
	// This test documents that the retry layer doesn't currently see HTTP status codes
	// for non-2xx responses because fetcher.Fetch() returns an error instead of a response
}

// TestRetryIntegrationRetryOn429RateLimited verifies 429 is retried.
// CRITICAL: Respect server rate limits with backoff.
// NOTE: Same issue as 4xx test - fetcher returns error for 429
func TestRetryIntegrationRetryOn429RateLimited(t *testing.T) {
	t.Skip("TODO: Fetcher returns error for 429, retry layer can't see status code to retry")
	// This documents a gap in the current architecture
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
