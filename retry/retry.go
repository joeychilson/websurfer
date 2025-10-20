package retry

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/joeychilson/websurfer/config"
	"github.com/joeychilson/websurfer/fetcher"
	"github.com/joeychilson/websurfer/ratelimit"
)

const (
	// jitterPercent is the percentage of jitter to add to retry delays (+/- 25%).
	jitterPercent = 0.25
)

// Retrier wraps a fetcher with retry logic and exponential backoff.
type Retrier struct {
	fetcher *fetcher.Fetcher
	limiter *ratelimit.Limiter
	config  config.RetryConfig
}

// New creates a new Retrier with the given fetcher, rate limiter, and retry configuration.
func New(f *fetcher.Fetcher, l *ratelimit.Limiter, cfg config.RetryConfig) *Retrier {
	return &Retrier{
		fetcher: f,
		limiter: l,
		config:  cfg,
	}
}

// Fetch attempts to fetch the URL with automatic retries on failure.
// It applies rate limiting, exponential backoff with jitter, and respects Retry-After headers.
func (r *Retrier) Fetch(ctx context.Context, url string) (*fetcher.Response, error) {
	maxRetries := r.config.GetMaxRetries()

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := r.limiter.Wait(ctx, url); err != nil {
			return nil, fmt.Errorf("rate limit wait failed: %w", err)
		}

		resp, err := r.fetcher.Fetch(ctx, url)

		if resp != nil {
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				r.limiter.Release(url)
				return resp, nil
			}

			if !r.config.ShouldRetry(resp.StatusCode) {
				r.limiter.Release(url)
				return resp, nil
			}

			r.limiter.UpdateRetryAfter(url, resp.Headers)
			lastErr = fmt.Errorf("attempt %d: HTTP %d", attempt, resp.StatusCode)
		} else {
			lastErr = fmt.Errorf("attempt %d failed: %w", attempt, err)
		}

		r.limiter.Release(url)

		if attempt < maxRetries {
			backoff := r.calculateBackoff(attempt)
			if sleepErr := r.sleep(ctx, backoff); sleepErr != nil {
				return nil, sleepErr
			}
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("failed after %d attempts: %w", maxRetries+1, lastErr)
	}

	return nil, fmt.Errorf("failed after %d attempts", maxRetries+1)
}

// calculateBackoff computes the backoff duration for a given attempt using exponential backoff.
func (r *Retrier) calculateBackoff(attempt int) time.Duration {
	initialDelay := r.config.GetInitialDelay()
	maxDelay := r.config.GetMaxDelay()
	multiplier := r.config.GetMultiplier()

	delay := float64(initialDelay) * math.Pow(multiplier, float64(attempt))

	if delay > float64(maxDelay) {
		delay = float64(maxDelay)
	}

	duration := time.Duration(delay)
	return r.addJitter(duration)
}

// addJitter adds random jitter to prevent thundering herd.
// Jitter is +/- 25% of the duration.
func (r *Retrier) addJitter(duration time.Duration) time.Duration {
	if duration == 0 {
		return 0
	}

	jitterRange := float64(duration) * jitterPercent
	jitter := (rand.Float64()*2.0 - 1.0) * jitterRange

	result := float64(duration) + jitter
	if result < 0 {
		return 0
	}

	return time.Duration(result)
}

// sleep waits for the specified duration or until context is cancelled.
func (r *Retrier) sleep(ctx context.Context, duration time.Duration) error {
	select {
	case <-time.After(duration):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
