package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/joeychilson/websurfer/config"
	urlutil "github.com/joeychilson/websurfer/url"
	"golang.org/x/time/rate"
)

// Limiter manages rate limiting for multiple domains.
type Limiter struct {
	config   config.RateLimitConfig
	mu       sync.RWMutex
	limiters map[string]*domainLimiter
	stopCh   chan struct{}
}

// domainLimiter holds rate limiting state for a single domain.
type domainLimiter struct {
	limiter    *rate.Limiter
	semaphore  chan struct{}
	retryAfter time.Time
	lastAccess time.Time
	mu         sync.RWMutex
}

// New creates a new rate limiter with the given configuration.
func New(cfg config.RateLimitConfig) *Limiter {
	l := &Limiter{
		config:   cfg,
		limiters: make(map[string]*domainLimiter),
		stopCh:   make(chan struct{}),
	}
	go l.cleanupInactiveDomains()
	return l
}

// Wait blocks until the rate limit allows a request to the given URL.
// It extracts the domain from the URL and applies per-domain rate limiting.
func (l *Limiter) Wait(ctx context.Context, urlStr string) error {
	if !l.config.IsEnabled() {
		return nil
	}

	domain, err := urlutil.ExtractHost(urlStr)
	if err != nil {
		return fmt.Errorf("failed to extract domain: %w", err)
	}

	dl := l.getLimiterForDomain(domain)

	if err := dl.wait(ctx); err != nil {
		return err
	}

	return nil
}

// Release releases resources held for a domain (e.g., concurrency semaphore).
func (l *Limiter) Release(urlStr string) {
	if !l.config.IsEnabled() {
		return
	}

	domain, err := urlutil.ExtractHost(urlStr)
	if err != nil {
		return
	}

	dl := l.getLimiterForDomain(domain)
	dl.release()
}

// UpdateRetryAfter updates the retry-after time for a domain based on HTTP response headers.
func (l *Limiter) UpdateRetryAfter(urlStr string, headers http.Header) {
	if !l.config.RespectRetryAfter {
		return
	}

	domain, err := urlutil.ExtractHost(urlStr)
	if err != nil {
		return
	}

	retryAfterStr := headers.Get("Retry-After")
	if retryAfterStr == "" {
		return
	}

	retryAfter := parseRetryAfter(retryAfterStr)
	if retryAfter.IsZero() {
		return
	}

	dl := l.getLimiterForDomain(domain)
	dl.setRetryAfter(retryAfter)
}

// getLimiterForDomain retrieves or creates a domain-specific limiter.
func (l *Limiter) getLimiterForDomain(domain string) *domainLimiter {
	l.mu.RLock()
	dl, exists := l.limiters[domain]
	l.mu.RUnlock()

	if exists {
		return dl
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	dl, exists = l.limiters[domain]
	if exists {
		return dl
	}

	dl = newDomainLimiter(l.config)
	l.limiters[domain] = dl

	return dl
}

// Close stops the cleanup goroutine and releases resources.
func (l *Limiter) Close() {
	close(l.stopCh)
}

// newDomainLimiter creates a new domain-specific limiter.
func newDomainLimiter(cfg config.RateLimitConfig) *domainLimiter {
	dl := &domainLimiter{
		lastAccess: time.Now(),
	}

	delay := cfg.GetDelay()
	if delay > 0 {
		limit := rate.Every(delay)
		burst := cfg.Burst
		if burst == 0 {
			burst = 1
		}
		dl.limiter = rate.NewLimiter(limit, burst)
	}

	maxConcurrent := cfg.GetMaxConcurrent()
	if maxConcurrent > 0 {
		dl.semaphore = make(chan struct{}, maxConcurrent)
	}

	return dl
}

// wait blocks until rate limiting allows the request.
func (dl *domainLimiter) wait(ctx context.Context) error {
	dl.mu.Lock()
	dl.lastAccess = time.Now()
	retryAfter := dl.retryAfter
	dl.mu.Unlock()

	if !retryAfter.IsZero() && time.Now().Before(retryAfter) {
		waitDuration := time.Until(retryAfter)
		select {
		case <-time.After(waitDuration):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if dl.semaphore != nil {
		select {
		case dl.semaphore <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if dl.limiter != nil {
		if err := dl.limiter.Wait(ctx); err != nil {
			if dl.semaphore != nil {
				<-dl.semaphore
			}
			return err
		}
	}

	return nil
}

// release releases concurrency resources.
func (dl *domainLimiter) release() {
	if dl.semaphore != nil {
		select {
		case <-dl.semaphore:
		default:
		}
	}
}

// setRetryAfter updates the retry-after time for this domain.
func (dl *domainLimiter) setRetryAfter(retryAfter time.Time) {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	if retryAfter.After(dl.retryAfter) {
		dl.retryAfter = retryAfter
	}
}

// parseRetryAfter parses a Retry-After header value.
func parseRetryAfter(value string) time.Time {
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Now().Add(time.Duration(seconds) * time.Second)
	}

	if t, err := http.ParseTime(value); err == nil {
		return t
	}

	return time.Time{}
}

// cleanupInactiveDomains periodically removes limiters for domains that haven't been accessed recently.
func (l *Limiter) cleanupInactiveDomains() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			now := time.Now()
			for domain, dl := range l.limiters {
				dl.mu.RLock()
				inactive := now.Sub(dl.lastAccess) > 30*time.Minute
				dl.mu.RUnlock()

				if inactive {
					delete(l.limiters, domain)
				}
			}
			l.mu.Unlock()
		case <-l.stopCh:
			return
		}
	}
}
