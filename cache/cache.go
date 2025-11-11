package cache

import (
	"context"
	"time"
)

// Entry represents a cached response.
type Entry struct {
	URL          string
	StatusCode   int
	Headers      map[string][]string
	Body         []byte
	Title        string
	Description  string
	LastModified string
	StoredAt     time.Time
	TTL          time.Duration
	StaleTime    time.Duration
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
	Get(ctx context.Context, url string) (*Entry, error)
	Set(ctx context.Context, entry *Entry) error
	Delete(ctx context.Context, url string) error
	Clear(ctx context.Context) error
}

// Config holds cache configuration.
type Config struct {
	TTL             time.Duration
	StaleTime       time.Duration
	CleanupInterval time.Duration
	MaxEntries      int
}

// DefaultConfig returns a cache config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		TTL:             5 * time.Minute,
		StaleTime:       1 * time.Hour,
		CleanupInterval: 10 * time.Minute,
		MaxEntries:      1000,
	}
}

// ApplyDefaults returns a new Config with default values applied for any zero-valued fields.
func ApplyDefaults(config Config) Config {
	defaults := DefaultConfig()

	if config.TTL == 0 {
		config.TTL = defaults.TTL
	}
	if config.StaleTime == 0 {
		config.StaleTime = defaults.StaleTime
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = defaults.CleanupInterval
	}
	if config.MaxEntries == 0 {
		config.MaxEntries = defaults.MaxEntries
	}

	return config
}
