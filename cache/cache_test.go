package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestCache(t *testing.T, config Config) (*Cache, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache := New(client, config)
	return cache, mr
}

// TestCacheStateFresh verifies that entries within TTL are marked as fresh.
func TestCacheStateFresh(t *testing.T) {
	entry := &Entry{
		URL:       "https://example.com",
		StoredAt:  time.Now(),
		TTL:       5 * time.Minute,
		StaleTime: 1 * time.Hour,
	}

	assert.Equal(t, StateFresh, entry.GetState(), "newly stored entry should be fresh")
}

// TestCacheStateStale verifies that entries past TTL but within stale window are marked as stale.
func TestCacheStateStale(t *testing.T) {
	entry := &Entry{
		URL:       "https://example.com",
		StoredAt:  time.Now().Add(-10 * time.Minute), // 10 minutes ago
		TTL:       5 * time.Minute,
		StaleTime: 1 * time.Hour,
	}

	assert.Equal(t, StateStale, entry.GetState(), "entry past TTL but within stale window should be stale")
}

// TestCacheStateTooOld verifies that entries past both TTL and stale window are marked as too old.
func TestCacheStateTooOld(t *testing.T) {
	entry := &Entry{
		URL:       "https://example.com",
		StoredAt:  time.Now().Add(-2 * time.Hour), // 2 hours ago
		TTL:       5 * time.Minute,
		StaleTime: 1 * time.Hour, // Total: 65 minutes max
	}

	assert.Equal(t, StateTooOld, entry.GetState(), "entry past TTL + stale window should be too old")
}

// TestCacheGetMiss verifies that Get returns nil for non-existent keys.
func TestCacheGetMiss(t *testing.T) {
	cache, _ := setupTestCache(t, DefaultConfig())
	ctx := context.Background()

	entry, err := cache.Get(ctx, "https://nonexistent.com")
	require.NoError(t, err)
	assert.Nil(t, entry, "non-existent entry should return nil")
}

// TestCacheSetGet verifies basic cache set and get operations work correctly.
func TestCacheSetGet(t *testing.T) {
	cache, _ := setupTestCache(t, DefaultConfig())
	ctx := context.Background()

	original := &Entry{
		URL:          "https://example.com",
		StatusCode:   200,
		Headers:      map[string][]string{"Content-Type": {"text/html"}},
		Body:         []byte("<html><body>Hello World</body></html>"),
		Title:        "Example Page",
		Description:  "A test page",
		LastModified: "Wed, 21 Oct 2015 07:28:00 GMT",
		StoredAt:     time.Now(),
	}

	// Store entry
	err := cache.Set(ctx, original)
	require.NoError(t, err, "Set should succeed")

	// Retrieve entry
	retrieved, err := cache.Get(ctx, original.URL)
	require.NoError(t, err, "Get should succeed")
	require.NotNil(t, retrieved, "retrieved entry should not be nil")

	// Verify all fields match
	assert.Equal(t, original.URL, retrieved.URL)
	assert.Equal(t, original.StatusCode, retrieved.StatusCode)
	assert.Equal(t, original.Headers, retrieved.Headers)
	assert.Equal(t, original.Body, retrieved.Body)
	assert.Equal(t, original.Title, retrieved.Title)
	assert.Equal(t, original.Description, retrieved.Description)
	assert.Equal(t, original.LastModified, retrieved.LastModified)
	assert.Equal(t, StateFresh, retrieved.GetState(), "newly stored entry should be fresh")
}

// TestCacheCompressionRoundTrip verifies compression and decompression work correctly.
func TestCacheCompressionRoundTrip(t *testing.T) {
	config := DefaultConfig()
	config.EnableCompression = true
	config.CompressionMinSize = 100 // Low threshold to ensure compression

	cache, _ := setupTestCache(t, config)
	ctx := context.Background()

	// Create entry with large body to ensure compression
	largeBody := make([]byte, 10000)
	for i := range largeBody {
		largeBody[i] = byte('A' + (i % 26))
	}

	original := &Entry{
		URL:        "https://example.com/large",
		StatusCode: 200,
		Body:       largeBody,
		StoredAt:   time.Now(),
	}

	// Store and retrieve
	err := cache.Set(ctx, original)
	require.NoError(t, err)

	retrieved, err := cache.Get(ctx, original.URL)
	require.NoError(t, err)
	require.NotNil(t, retrieved)

	// Verify body is intact after compression round-trip
	assert.Equal(t, original.Body, retrieved.Body, "body should survive compression round-trip")
}

// TestCacheTooOldAutoDelete verifies that entries marked as too old are automatically deleted.
func TestCacheTooOldAutoDelete(t *testing.T) {
	cache, mr := setupTestCache(t, DefaultConfig())
	ctx := context.Background()

	// Create entry that's already too old
	entry := &Entry{
		URL:        "https://example.com/old",
		StatusCode: 200,
		Body:       []byte("old content"),
		StoredAt:   time.Now().Add(-2 * time.Hour),
		TTL:        5 * time.Minute,
		StaleTime:  1 * time.Hour,
	}

	// Store it (bypassing the too-old check)
	err := cache.Set(ctx, entry)
	require.NoError(t, err)

	// Fast forward time to make it too old
	mr.FastForward(2 * time.Hour)

	// Try to retrieve - should return nil and delete
	retrieved, err := cache.Get(ctx, entry.URL)
	require.NoError(t, err)
	assert.Nil(t, retrieved, "too old entry should be deleted and return nil")

	// Verify it's actually gone from Redis
	exists := mr.Exists(cache.makeKey(entry.URL))
	assert.False(t, exists, "too old entry should be deleted from Redis")
}

// TestCacheWithUpdatedTimestamp verifies that timestamp updates work correctly.
func TestCacheWithUpdatedTimestamp(t *testing.T) {
	original := &Entry{
		URL:       "https://example.com",
		StoredAt:  time.Now().Add(-1 * time.Hour),
		TTL:       5 * time.Minute,
		StaleTime: 1 * time.Hour,
	}

	// Original should be stale
	assert.Equal(t, StateStale, original.GetState())

	// Create updated copy
	time.Sleep(1 * time.Millisecond) // Ensure time difference
	updated := original.WithUpdatedTimestamp()

	// Updated should be fresh
	assert.Equal(t, StateFresh, updated.GetState())

	// Original should be unchanged
	assert.Equal(t, StateStale, original.GetState())

	// Verify it's a true copy (not same reference)
	assert.NotEqual(t, original.StoredAt, updated.StoredAt)
}

// TestCacheDefaultTTL verifies that default TTL is applied when not set.
func TestCacheDefaultTTL(t *testing.T) {
	config := DefaultConfig()
	config.TTL = 10 * time.Minute
	config.StaleTime = 30 * time.Minute

	cache, _ := setupTestCache(t, config)
	ctx := context.Background()

	entry := &Entry{
		URL:        "https://example.com",
		StatusCode: 200,
		Body:       []byte("test"),
		StoredAt:   time.Now(),
		// TTL and StaleTime intentionally not set
	}

	err := cache.Set(ctx, entry)
	require.NoError(t, err)

	// Verify defaults were applied
	assert.Equal(t, config.TTL, entry.TTL, "default TTL should be applied")
	assert.Equal(t, config.StaleTime, entry.StaleTime, "default StaleTime should be applied")
}

// TestCacheKeyPrefix verifies that custom prefixes work correctly.
func TestCacheKeyPrefix(t *testing.T) {
	config := DefaultConfig()
	config.Prefix = "custom-prefix:"

	cache, _ := setupTestCache(t, config)

	key := cache.makeKey("https://example.com")
	assert.Equal(t, "custom-prefix:https://example.com", key)
}

// TestCacheCompressionThreshold verifies that small entries are not compressed.
func TestCacheCompressionThreshold(t *testing.T) {
	config := DefaultConfig()
	config.EnableCompression = true
	config.CompressionMinSize = 1000

	cache, mr := setupTestCache(t, config)
	ctx := context.Background()

	// Small entry below threshold
	smallEntry := &Entry{
		URL:        "https://example.com/small",
		StatusCode: 200,
		Body:       []byte("small body"),
		StoredAt:   time.Now(),
	}

	err := cache.Set(ctx, smallEntry)
	require.NoError(t, err)

	// Get raw data from Redis
	key := cache.makeKey(smallEntry.URL)
	rawData, err := mr.Get(key)
	require.NoError(t, err)

	// Verify it's NOT gzip compressed (check for gzip magic bytes)
	data := []byte(rawData)
	isGzipped := len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
	assert.False(t, isGzipped, "small entry should not be compressed")

	// Large entry above threshold
	largeBody := make([]byte, 2000)
	largeEntry := &Entry{
		URL:        "https://example.com/large",
		StatusCode: 200,
		Body:       largeBody,
		StoredAt:   time.Now(),
	}

	err = cache.Set(ctx, largeEntry)
	require.NoError(t, err)

	// Get raw data from Redis
	key = cache.makeKey(largeEntry.URL)
	rawData, err = mr.Get(key)
	require.NoError(t, err)

	// Verify it IS gzip compressed
	data = []byte(rawData)
	isGzipped = len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
	assert.True(t, isGzipped, "large entry should be compressed")
}

// TestCacheRedisExpiration verifies that Redis TTL is set correctly.
func TestCacheRedisExpiration(t *testing.T) {
	config := DefaultConfig()
	config.TTL = 5 * time.Minute
	config.StaleTime = 10 * time.Minute

	cache, mr := setupTestCache(t, config)
	ctx := context.Background()

	entry := &Entry{
		URL:        "https://example.com",
		StatusCode: 200,
		Body:       []byte("test"),
		StoredAt:   time.Now(),
	}

	err := cache.Set(ctx, entry)
	require.NoError(t, err)

	// Get TTL from Redis
	key := cache.makeKey(entry.URL)
	ttl := mr.TTL(key)

	// Should be TTL + StaleTime = 15 minutes
	expectedTTL := config.TTL + config.StaleTime

	// Allow 1 second tolerance for test execution time
	assert.InDelta(t, expectedTTL.Seconds(), ttl.Seconds(), 1.0,
		"Redis TTL should be TTL + StaleTime")
}
