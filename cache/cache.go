package cache

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/redis/go-redis/v9"
)

// State represents the cache state of an entry.
type State int

const (
	// StateFresh indicates the entry is within its TTL.
	StateFresh State = iota
	// StateStale indicates the entry is past TTL but within the stale window.
	StateStale
	// StateTooOld indicates the entry is past both TTL and stale window.
	StateTooOld
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

// GetState returns the current state of the cache entry, computing the age only once
// to avoid race conditions between state checks.
func (e *Entry) GetState() State {
	age := time.Since(e.StoredAt)

	if age < e.TTL {
		return StateFresh
	}

	if age < (e.TTL + e.StaleTime) {
		return StateStale
	}

	return StateTooOld
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
	Prefix             string
	TTL                time.Duration
	StaleTime          time.Duration
	EnableCompression  bool
	CompressionLevel   int
	CompressionMinSize int
}

// DefaultConfig returns a cache config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Prefix:             "websurfer:",
		TTL:                5 * time.Minute,
		StaleTime:          1 * time.Hour,
		EnableCompression:  true,
		CompressionLevel:   gzip.DefaultCompression,
		CompressionMinSize: 1024,
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

	if c.config.EnableCompression && len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		data, err = c.decompress(data)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress entry: %w", err)
		}
	}

	var entry Entry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal entry: %w", err)
	}

	if entry.GetState() == StateTooOld {
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

	if c.config.EnableCompression && len(data) >= c.config.CompressionMinSize {
		data, err = c.compress(data)
		if err != nil {
			return fmt.Errorf("failed to compress entry: %w", err)
		}
	}

	expiration := entry.TTL + entry.StaleTime

	if err := c.client.Set(ctx, key, data, expiration).Err(); err != nil {
		return fmt.Errorf("redis set failed: %w", err)
	}

	return nil
}

// makeKey creates a Redis key with the configured prefix.
func (c *Cache) makeKey(url string) string {
	return c.prefix + url
}

// compress compresses data using gzip.
func (c *Cache) compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	level := c.config.CompressionLevel
	if level == 0 {
		level = gzip.DefaultCompression
	}

	gz, err := gzip.NewWriterLevel(&buf, level)
	if err != nil {
		return nil, err
	}

	if _, err := gz.Write(data); err != nil {
		gz.Close()
		return nil, err
	}

	if err := gz.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// decompress decompresses gzipped data.
func (c *Cache) decompress(data []byte) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	return io.ReadAll(gz)
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
	if config.EnableCompression {
		if config.CompressionLevel == 0 {
			config.CompressionLevel = defaults.CompressionLevel
		}
		if config.CompressionMinSize == 0 {
			config.CompressionMinSize = defaults.CompressionMinSize
		}
	}
	return config
}
