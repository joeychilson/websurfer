package cache

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// RedisCache is a Redis-based cache implementation.
type RedisCache struct {
	client *redis.Client
	config Config
	prefix string
}

// RedisConfig holds Redis-specific configuration.
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	Prefix   string
	Config   Config
}

// NewRedisCache creates a new Redis cache.
func NewRedisCache(config RedisConfig) *RedisCache {
	if config.Addr == "" {
		config.Addr = "localhost:6379"
	}
	if config.Prefix == "" {
		config.Prefix = "plainhtml:"
	}
	if config.Config.TTL == 0 {
		config.Config.TTL = DefaultConfig().TTL
	}
	if config.Config.StaleTime == 0 {
		config.Config.StaleTime = DefaultConfig().StaleTime
	}

	client := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
	})

	return &RedisCache{
		client: client,
		config: config.Config,
		prefix: config.Prefix,
	}
}

// NewRedisCacheFromURL creates a new Redis cache from a Redis URL.
// URL format: redis://[user[:password]@]host[:port][/db][?option=value]
func NewRedisCacheFromURL(redisURL string, prefix string, config Config) (*RedisCache, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis URL: %w", err)
	}

	if prefix == "" {
		prefix = "plainhtml:"
	}
	if config.TTL == 0 {
		config.TTL = DefaultConfig().TTL
	}
	if config.StaleTime == 0 {
		config.StaleTime = DefaultConfig().StaleTime
	}

	client := redis.NewClient(opts)

	return &RedisCache{
		client: client,
		config: config,
		prefix: prefix,
	}, nil
}

// NewRedisCacheWithClient creates a Redis cache with an existing client.
func NewRedisCacheWithClient(client *redis.Client, prefix string, config Config) *RedisCache {
	if prefix == "" {
		prefix = "plainhtml:"
	}
	if config.TTL == 0 {
		config.TTL = DefaultConfig().TTL
	}
	if config.StaleTime == 0 {
		config.StaleTime = DefaultConfig().StaleTime
	}

	return &RedisCache{
		client: client,
		config: config,
		prefix: prefix,
	}
}

// Get retrieves an entry from Redis.
// Returns nil if the entry doesn't exist or is too old.
func (rc *RedisCache) Get(ctx context.Context, url string) (*Entry, error) {
	key := rc.makeKey(url)

	data, err := rc.client.Get(ctx, key).Bytes()
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
		rc.client.Del(ctx, key)
		return nil, nil
	}

	return &entry, nil
}

// Set stores an entry in Redis with TTL + StaleTime expiration.
func (rc *RedisCache) Set(ctx context.Context, entry *Entry) error {
	if entry.TTL == 0 {
		entry.TTL = rc.config.TTL
	}
	if entry.StaleTime == 0 {
		entry.StaleTime = rc.config.StaleTime
	}

	key := rc.makeKey(entry.URL)

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}

	expiration := entry.TTL + entry.StaleTime

	if err := rc.client.Set(ctx, key, data, expiration).Err(); err != nil {
		return fmt.Errorf("redis set failed: %w", err)
	}

	return nil
}

// Delete removes an entry from Redis.
func (rc *RedisCache) Delete(ctx context.Context, url string) error {
	key := rc.makeKey(url)

	if err := rc.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis delete failed: %w", err)
	}

	return nil
}

// Clear removes all entries with the configured prefix.
func (rc *RedisCache) Clear(ctx context.Context) error {
	pattern := rc.prefix + "*"

	iter := rc.client.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		if err := rc.client.Del(ctx, iter.Val()).Err(); err != nil {
			return fmt.Errorf("redis clear failed: %w", err)
		}
	}

	if err := iter.Err(); err != nil {
		return fmt.Errorf("redis scan failed: %w", err)
	}

	return nil
}

// Close closes the Redis connection.
func (rc *RedisCache) Close() error {
	return rc.client.Close()
}

// Ping checks if Redis connection is healthy.
func (rc *RedisCache) Ping(ctx context.Context) error {
	return rc.client.Ping(ctx).Err()
}

// makeKey creates a Redis key with the configured prefix.
func (rc *RedisCache) makeKey(url string) string {
	return rc.prefix + url
}
