package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
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

// WithUpdatedTimestamp creates a copy of the entry with an updated StoredAt timestamp.
func (e *Entry) WithUpdatedTimestamp() *Entry {
	updated := *e
	updated.StoredAt = time.Now()
	return &updated
}

// Cache is a Redis-based cache implementation.
type Cache struct {
	client *redis.Client
	config Config
	prefix string
}

// Config holds cache configuration.
type Config struct {
	Prefix    string
	TTL       time.Duration
	StaleTime time.Duration
}

// DefaultConfig returns a cache config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Prefix:    "websurfer:",
		TTL:       5 * time.Minute,
		StaleTime: 1 * time.Hour,
	}
}

// New creates a new Redis cache with the provided client and configuration.
func New(client *redis.Client, config Config) *Cache {
	config = applyDefaults(config)

	return &Cache{
		client: client,
		config: config,
		prefix: config.Prefix,
	}
}

// Get retrieves an entry from Redis.
func (c *Cache) Get(ctx context.Context, url string) (*Entry, error) {
	key := c.makeKey(url)

	data, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get failed: %w", err)
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal entry: %w", err)
	}

	if entry.IsTooOld() {
		c.client.Del(ctx, key)
		return nil, nil
	}

	return &entry, nil
}

// Set stores an entry in Redis with TTL + StaleTime expiration.
func (c *Cache) Set(ctx context.Context, entry *Entry) error {
	if entry.TTL == 0 {
		entry.TTL = c.config.TTL
	}
	if entry.StaleTime == 0 {
		entry.StaleTime = c.config.StaleTime
	}

	key := c.makeKey(entry.URL)

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}

	expiration := entry.TTL + entry.StaleTime

	if err := c.client.Set(ctx, key, data, expiration).Err(); err != nil {
		return fmt.Errorf("redis set failed: %w", err)
	}

	return nil
}

// Delete removes an entry from Redis.
func (c *Cache) Delete(ctx context.Context, url string) error {
	key := c.makeKey(url)

	if err := c.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis delete failed: %w", err)
	}

	return nil
}

// Clear removes all entries with the configured prefix.
func (c *Cache) Clear(ctx context.Context) error {
	pattern := c.prefix + "*"

	iter := c.client.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		if err := c.client.Del(ctx, iter.Val()).Err(); err != nil {
			return fmt.Errorf("redis clear failed: %w", err)
		}
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("redis scan failed: %w", err)
	}

	return nil
}

// makeKey creates a Redis key with the configured prefix.
func (c *Cache) makeKey(url string) string {
	return c.prefix + url
}

// applyDefaults returns a new Config with default values applied for any zero-valued fields.
func applyDefaults(config Config) Config {
	defaults := DefaultConfig()

	if config.Prefix == "" {
		config.Prefix = defaults.Prefix
	}
	if config.TTL == 0 {
		config.TTL = defaults.TTL
	}
	if config.StaleTime == 0 {
		config.StaleTime = defaults.StaleTime
	}
	return config
}
