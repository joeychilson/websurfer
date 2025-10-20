package cache

import (
	"context"
	"time"
)

// Entry represents a cached response.
type Entry struct {
	URL         string
	StatusCode  int
	Headers     map[string][]string
	Body        []byte
	Title       string
	Description string
	StoredAt    time.Time
	TTL         time.Duration
	StaleTime   time.Duration
}

// IsFresh returns true if the entry is still within its TTL.
func (e *Entry) IsFresh() bool {
	return time.Since(e.StoredAt) < e.TTL
}

// IsStale returns true if the entry is past TTL but still within stale window.
func (e *Entry) IsStale() bool {
	age := time.Since(e.StoredAt)
	return age >= e.TTL && age < (e.TTL+e.StaleTime)
}

// IsTooOld returns true if the entry is past both TTL and stale window.
func (e *Entry) IsTooOld() bool {
	return time.Since(e.StoredAt) >= (e.TTL + e.StaleTime)
}

// Cache is the interface for cache implementations.
type Cache interface {
	// Get retrieves an entry from the cache.
	// Returns nil if the entry doesn't exist or is too old.
	Get(ctx context.Context, url string) (*Entry, error)
	// Set stores an entry in the cache.
	Set(ctx context.Context, entry *Entry) error
	// Delete removes an entry from the cache.
	Delete(ctx context.Context, url string) error
	// Clear removes all entries from the cache.
	Clear(ctx context.Context) error
	// Close releases any resources held by the cache.
	Close() error
}

// Config holds cache configuration.
type Config struct {
	// TTL is how long content is considered fresh
	TTL time.Duration
	// StaleTime is how long after TTL to serve stale content while revalidating
	StaleTime time.Duration
	// CleanupInterval is how often to remove expired entries (in-memory only)
	CleanupInterval time.Duration
}

// DefaultConfig returns a cache config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		TTL:             5 * time.Minute,
		StaleTime:       1 * time.Hour,
		CleanupInterval: 10 * time.Minute,
	}
}
