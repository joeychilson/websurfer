package retry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeychilson/websurfer/config"
	"github.com/joeychilson/websurfer/fetcher"
	"github.com/joeychilson/websurfer/ratelimit"
)

func TestRetrier_Fetch(t *testing.T) {
	t.Run("successful fetch on first attempt", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		}))
		defer server.Close()

		f := fetcher.New(config.FetchConfig{})
		l := ratelimit.New(config.RateLimitConfig{})
		retrier := New(f, l, config.RetryConfig{})

		resp, err := retrier.Fetch(context.Background(), server.URL)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		if string(resp.Body) != "success" {
			t.Errorf("Body = %q, want %q", string(resp.Body), "success")
		}
	})

	t.Run("retries on 429 and succeeds", func(t *testing.T) {
		var attempts int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&attempts, 1)
			if count < 3 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success after retries"))
		}))
		defer server.Close()

		f := fetcher.New(config.FetchConfig{})
		l := ratelimit.New(config.RateLimitConfig{})
		retrier := New(f, l, config.RetryConfig{
			MaxRetries:   5,
			InitialDelay: 10 * time.Millisecond,
			Multiplier:   2.0,
		})

		resp, err := retrier.Fetch(context.Background(), server.URL)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		if atomic.LoadInt32(&attempts) != 3 {
			t.Errorf("attempts = %d, want 3", atomic.LoadInt32(&attempts))
		}
	})

	t.Run("retries on 500 and succeeds", func(t *testing.T) {
		var attempts int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&attempts, 1)
			if count < 2 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("recovered"))
		}))
		defer server.Close()

		f := fetcher.New(config.FetchConfig{})
		l := ratelimit.New(config.RateLimitConfig{})
		retrier := New(f, l, config.RetryConfig{
			MaxRetries:   3,
			InitialDelay: 10 * time.Millisecond,
		})

		resp, err := retrier.Fetch(context.Background(), server.URL)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("retries on 503 and succeeds", func(t *testing.T) {
		var attempts int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&attempts, 1)
			if count == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		f := fetcher.New(config.FetchConfig{})
		l := ratelimit.New(config.RateLimitConfig{})
		retrier := New(f, l, config.RetryConfig{
			MaxRetries:   2,
			InitialDelay: 10 * time.Millisecond,
		})

		resp, err := retrier.Fetch(context.Background(), server.URL)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("no retry on 404", func(t *testing.T) {
		var attempts int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&attempts, 1)
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		f := fetcher.New(config.FetchConfig{})
		l := ratelimit.New(config.RateLimitConfig{})
		retrier := New(f, l, config.RetryConfig{
			MaxRetries: 3,
		})

		resp, err := retrier.Fetch(context.Background(), server.URL)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}

		if atomic.LoadInt32(&attempts) != 1 {
			t.Errorf("attempts = %d, want 1 (should not retry 404)", atomic.LoadInt32(&attempts))
		}
	})

	t.Run("no retry on 400", func(t *testing.T) {
		var attempts int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&attempts, 1)
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		f := fetcher.New(config.FetchConfig{})
		l := ratelimit.New(config.RateLimitConfig{})
		retrier := New(f, l, config.RetryConfig{
			MaxRetries: 3,
		})

		resp, err := retrier.Fetch(context.Background(), server.URL)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusBadRequest)
		}

		if atomic.LoadInt32(&attempts) != 1 {
			t.Errorf("attempts = %d, want 1 (should not retry 400)", atomic.LoadInt32(&attempts))
		}
	})

	t.Run("respects max retries", func(t *testing.T) {
		var attempts int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&attempts, 1)
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer server.Close()

		f := fetcher.New(config.FetchConfig{})
		l := ratelimit.New(config.RateLimitConfig{})
		retrier := New(f, l, config.RetryConfig{
			MaxRetries:   2,
			InitialDelay: 10 * time.Millisecond,
		})

		_, err := retrier.Fetch(context.Background(), server.URL)
		if err == nil {
			t.Error("Fetch() error = nil, want error after max retries")
		}

		if atomic.LoadInt32(&attempts) != 3 {
			t.Errorf("attempts = %d, want 3 (initial + 2 retries)", atomic.LoadInt32(&attempts))
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer server.Close()

		f := fetcher.New(config.FetchConfig{})
		l := ratelimit.New(config.RateLimitConfig{})
		retrier := New(f, l, config.RetryConfig{
			MaxRetries:   5,
			InitialDelay: 500 * time.Millisecond,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		start := time.Now()
		_, err := retrier.Fetch(ctx, server.URL)
		elapsed := time.Since(start)

		if err == nil {
			t.Error("Fetch() error = nil, want context error")
		}

		if elapsed > 300*time.Millisecond {
			t.Errorf("elapsed = %v, want < 300ms (should cancel quickly)", elapsed)
		}
	})
}

func TestRetrier_ExponentialBackoff(t *testing.T) {
	t.Run("backoff increases exponentially", func(t *testing.T) {
		var attempts int32
		var timestamps []time.Time

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&attempts, 1)
			timestamps = append(timestamps, time.Now())
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer server.Close()

		f := fetcher.New(config.FetchConfig{})
		l := ratelimit.New(config.RateLimitConfig{})
		retrier := New(f, l, config.RetryConfig{
			MaxRetries:   3,
			InitialDelay: 100 * time.Millisecond,
			Multiplier:   2.0,
		})

		retrier.Fetch(context.Background(), server.URL)

		if len(timestamps) != 4 {
			t.Fatalf("got %d attempts, want 4", len(timestamps))
		}

		delay1 := timestamps[1].Sub(timestamps[0])
		delay2 := timestamps[2].Sub(timestamps[1])
		delay3 := timestamps[3].Sub(timestamps[2])

		if delay1 < 75*time.Millisecond || delay1 > 150*time.Millisecond {
			t.Errorf("first backoff = %v, want ~100ms (with jitter)", delay1)
		}

		if delay2 < 150*time.Millisecond || delay2 > 300*time.Millisecond {
			t.Errorf("second backoff = %v, want ~200ms (with jitter)", delay2)
		}

		if delay3 < 300*time.Millisecond || delay3 > 600*time.Millisecond {
			t.Errorf("third backoff = %v, want ~400ms (with jitter)", delay3)
		}

		if delay2 <= delay1 {
			t.Error("backoff should increase exponentially")
		}
	})

	t.Run("respects max delay", func(t *testing.T) {
		var attempts int32
		var timestamps []time.Time

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&attempts, 1)
			timestamps = append(timestamps, time.Now())
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer server.Close()

		f := fetcher.New(config.FetchConfig{})
		l := ratelimit.New(config.RateLimitConfig{})
		retrier := New(f, l, config.RetryConfig{
			MaxRetries:   3,
			InitialDelay: 100 * time.Millisecond,
			MaxDelay:     150 * time.Millisecond,
			Multiplier:   10.0,
		})

		retrier.Fetch(context.Background(), server.URL)

		if len(timestamps) != 4 {
			t.Fatalf("got %d attempts, want 4", len(timestamps))
		}

		for i := 1; i < len(timestamps); i++ {
			delay := timestamps[i].Sub(timestamps[i-1])
			if delay > 250*time.Millisecond {
				t.Errorf("backoff %d = %v, want <= 150ms + jitter", i, delay)
			}
		}
	})
}

func TestRetrier_RetryAfter(t *testing.T) {
	t.Run("respects Retry-After header", func(t *testing.T) {
		var attempts int32
		var timestamps []time.Time

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			count := atomic.AddInt32(&attempts, 1)
			timestamps = append(timestamps, time.Now())

			if count == 1 {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		f := fetcher.New(config.FetchConfig{})
		l := ratelimit.New(config.RateLimitConfig{
			RespectRetryAfter: true,
		})
		retrier := New(f, l, config.RetryConfig{
			MaxRetries:   2,
			InitialDelay: 10 * time.Millisecond,
		})

		resp, err := retrier.Fetch(context.Background(), server.URL)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		if len(timestamps) != 2 {
			t.Fatalf("got %d attempts, want 2", len(timestamps))
		}

		delay := timestamps[1].Sub(timestamps[0])
		if delay < 900*time.Millisecond {
			t.Errorf("delay = %v, want >= 900ms (Retry-After not respected)", delay)
		}
	})
}

func TestCalculateBackoff(t *testing.T) {
	retrier := New(nil, nil, config.RetryConfig{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
	})

	t.Run("attempt 0", func(t *testing.T) {
		backoff := retrier.calculateBackoff(0)
		if backoff < 75*time.Millisecond || backoff > 125*time.Millisecond {
			t.Errorf("backoff = %v, want ~100ms with jitter", backoff)
		}
	})

	t.Run("attempt 1", func(t *testing.T) {
		backoff := retrier.calculateBackoff(1)
		if backoff < 150*time.Millisecond || backoff > 250*time.Millisecond {
			t.Errorf("backoff = %v, want ~200ms with jitter", backoff)
		}
	})

	t.Run("attempt 2", func(t *testing.T) {
		backoff := retrier.calculateBackoff(2)
		if backoff < 300*time.Millisecond || backoff > 500*time.Millisecond {
			t.Errorf("backoff = %v, want ~400ms with jitter", backoff)
		}
	})
}

func TestAddJitter(t *testing.T) {
	retrier := New(nil, nil, config.RetryConfig{})

	t.Run("jitter within 25% range", func(t *testing.T) {
		duration := 1000 * time.Millisecond

		for i := 0; i < 100; i++ {
			jittered := retrier.addJitter(duration)

			if jittered < 750*time.Millisecond || jittered > 1250*time.Millisecond {
				t.Errorf("jittered = %v, want between 750ms and 1250ms", jittered)
			}
		}
	})

	t.Run("zero duration returns zero", func(t *testing.T) {
		jittered := retrier.addJitter(0)
		if jittered != 0 {
			t.Errorf("jittered = %v, want 0", jittered)
		}
	})

	t.Run("jitter varies across calls", func(t *testing.T) {
		duration := 1000 * time.Millisecond
		results := make(map[time.Duration]bool)

		for i := 0; i < 20; i++ {
			jittered := retrier.addJitter(duration)
			results[jittered] = true
		}

		if len(results) < 10 {
			t.Errorf("got %d unique values, want at least 10 (jitter should vary)", len(results))
		}
	})
}
