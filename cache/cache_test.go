package cache

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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

func TestCompression_RoundTrip(t *testing.T) {
	config := DefaultConfig()
	config.EnableCompression = true
	config.CompressionMinSize = 100

	cache, _ := setupTestCache(t, config)
	ctx := context.Background()

	largeBody := []byte(strings.Repeat("test data ", 50))
	entry := &Entry{
		URL:        "https://example.com/test",
		StatusCode: 200,
		Headers:    map[string][]string{"Content-Type": {"text/html"}},
		Body:       largeBody,
		StoredAt:   time.Now(),
	}

	err := cache.Set(ctx, entry)
	if err != nil {
		t.Fatalf("failed to set entry: %v", err)
	}

	retrieved, err := cache.Get(ctx, entry.URL)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected entry, got nil")
	}

	if string(retrieved.Body) != string(largeBody) {
		t.Errorf("body mismatch: got %d bytes, want %d bytes", len(retrieved.Body), len(largeBody))
	}

	if retrieved.URL != entry.URL {
		t.Errorf("URL mismatch: got %q, want %q", retrieved.URL, entry.URL)
	}
	if retrieved.StatusCode != entry.StatusCode {
		t.Errorf("StatusCode mismatch: got %d, want %d", retrieved.StatusCode, entry.StatusCode)
	}
}

func TestCompression_SmallBody(t *testing.T) {
	config := DefaultConfig()
	config.EnableCompression = true
	config.CompressionMinSize = 1024

	cache, mr := setupTestCache(t, config)
	ctx := context.Background()

	smallBody := []byte("small data")
	entry := &Entry{
		URL:        "https://example.com/small",
		StatusCode: 200,
		Body:       smallBody,
		StoredAt:   time.Now(),
	}

	err := cache.Set(ctx, entry)
	if err != nil {
		t.Fatalf("failed to set entry: %v", err)
	}

	key := cache.makeKey(entry.URL)
	storedData, err := mr.Get(key)
	if err != nil {
		t.Fatalf("failed to get raw data from redis: %v", err)
	}

	if len(storedData) >= 2 && storedData[0] == 0x1f && storedData[1] == 0x8b {
		t.Error("small body should not be compressed")
	}

	retrieved, err := cache.Get(ctx, entry.URL)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}

	if string(retrieved.Body) != string(smallBody) {
		t.Errorf("body mismatch: got %q, want %q", string(retrieved.Body), string(smallBody))
	}
}

func TestCompression_LargeBody(t *testing.T) {
	config := DefaultConfig()
	config.EnableCompression = true
	config.CompressionMinSize = 1024

	cache, mr := setupTestCache(t, config)
	ctx := context.Background()

	largeBody := []byte(strings.Repeat("compressible data ", 200))
	entry := &Entry{
		URL:        "https://example.com/large",
		StatusCode: 200,
		Body:       largeBody,
		StoredAt:   time.Now(),
	}

	err := cache.Set(ctx, entry)
	if err != nil {
		t.Fatalf("failed to set entry: %v", err)
	}

	key := cache.makeKey(entry.URL)
	storedData, err := mr.Get(key)
	if err != nil {
		t.Fatalf("failed to get raw data from redis: %v", err)
	}

	if len(storedData) < 2 || storedData[0] != 0x1f || storedData[1] != 0x8b {
		t.Error("large body should be compressed")
	}

	if len(storedData) >= len(largeBody) {
		t.Errorf("compression ineffective: compressed size %d >= original size %d", len(storedData), len(largeBody))
	}

	retrieved, err := cache.Get(ctx, entry.URL)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}

	if string(retrieved.Body) != string(largeBody) {
		t.Errorf("body mismatch after decompression")
	}
}

func TestCompression_Disabled(t *testing.T) {
	config := DefaultConfig()
	config.EnableCompression = false

	cache, mr := setupTestCache(t, config)
	ctx := context.Background()

	largeBody := []byte(strings.Repeat("test data ", 200))
	entry := &Entry{
		URL:        "https://example.com/disabled",
		StatusCode: 200,
		Body:       largeBody,
		StoredAt:   time.Now(),
	}

	err := cache.Set(ctx, entry)
	if err != nil {
		t.Fatalf("failed to set entry: %v", err)
	}

	key := cache.makeKey(entry.URL)
	storedData, err := mr.Get(key)
	if err != nil {
		t.Fatalf("failed to get raw data from redis: %v", err)
	}

	if len(storedData) >= 2 && storedData[0] == 0x1f && storedData[1] == 0x8b {
		t.Error("compression disabled but data is compressed")
	}

	retrieved, err := cache.Get(ctx, entry.URL)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}

	if string(retrieved.Body) != string(largeBody) {
		t.Errorf("body mismatch")
	}
}

func TestCompression_BackwardsCompatibility(t *testing.T) {
	config := DefaultConfig()
	config.EnableCompression = true

	cache, _ := setupTestCache(t, config)
	ctx := context.Background()

	uncompressedConfig := DefaultConfig()
	uncompressedConfig.EnableCompression = false
	uncompressedCache := &Cache{
		client: cache.client,
		config: uncompressedConfig,
		prefix: cache.prefix,
	}

	body := []byte(strings.Repeat("data ", 500))
	entry := &Entry{
		URL:        "https://example.com/compat",
		StatusCode: 200,
		Body:       body,
		StoredAt:   time.Now(),
	}

	err := uncompressedCache.Set(ctx, entry)
	if err != nil {
		t.Fatalf("failed to set uncompressed entry: %v", err)
	}

	retrieved, err := cache.Get(ctx, entry.URL)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}

	if string(retrieved.Body) != string(body) {
		t.Error("failed to read uncompressed cached data")
	}
}

func TestCompression_HTMLContent(t *testing.T) {
	config := DefaultConfig()
	config.EnableCompression = true
	config.CompressionMinSize = 1024

	cache, mr := setupTestCache(t, config)
	ctx := context.Background()

	htmlBody := []byte(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Test Page</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 0; padding: 20px; }
        .container { max-width: 1200px; margin: 0 auto; }
        h1 { color: #333; }
        p { line-height: 1.6; color: #666; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Welcome to Test Page</h1>
        <p>This is a test paragraph with some content.</p>
        <p>Another paragraph with more content to make the HTML larger.</p>
        <p>And another one with even more content to ensure compression is worthwhile.</p>
    </div>
</body>
</html>`)

	entry := &Entry{
		URL:        "https://example.com/page",
		StatusCode: 200,
		Headers:    map[string][]string{"Content-Type": {"text/html; charset=utf-8"}},
		Body:       htmlBody,
		StoredAt:   time.Now(),
	}

	err := cache.Set(ctx, entry)
	if err != nil {
		t.Fatalf("failed to set entry: %v", err)
	}

	key := cache.makeKey(entry.URL)
	storedData, err := mr.Get(key)
	if err != nil {
		t.Fatalf("failed to get raw data from redis: %v", err)
	}

	compressionRatio := float64(len(storedData)) / float64(len(htmlBody))
	t.Logf("HTML compression ratio: %.2f%% (original: %d bytes, compressed: %d bytes)",
		compressionRatio*100, len(htmlBody), len(storedData))

	retrieved, err := cache.Get(ctx, entry.URL)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}

	if string(retrieved.Body) != string(htmlBody) {
		t.Error("HTML body mismatch after compression/decompression")
	}
}

func TestCompression_DifferentLevels(t *testing.T) {
	tests := []struct {
		name  string
		level int
	}{
		{"default compression", 0},
		{"best speed", 1},
		{"best compression", 9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.EnableCompression = true
			config.CompressionLevel = tt.level
			config.CompressionMinSize = 100

			cache, _ := setupTestCache(t, config)
			ctx := context.Background()

			body := []byte(strings.Repeat("compressible test data ", 100))
			entry := &Entry{
				URL:        "https://example.com/level" + string(rune(tt.level)),
				StatusCode: 200,
				Body:       body,
				StoredAt:   time.Now(),
			}

			err := cache.Set(ctx, entry)
			if err != nil {
				t.Fatalf("failed to set entry: %v", err)
			}

			retrieved, err := cache.Get(ctx, entry.URL)
			if err != nil {
				t.Fatalf("failed to get entry: %v", err)
			}

			if string(retrieved.Body) != string(body) {
				t.Error("body mismatch with compression level", tt.level)
			}
		})
	}
}

func BenchmarkCompression_Set(b *testing.B) {
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	config := DefaultConfig()
	config.EnableCompression = true
	config.CompressionMinSize = 1024

	cache := New(client, config)
	ctx := context.Background()

	htmlBody := []byte(strings.Repeat(`<div class="content"><p>This is some HTML content with text.</p></div>`, 50))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := &Entry{
			URL:        "https://example.com/bench",
			StatusCode: 200,
			Body:       htmlBody,
			StoredAt:   time.Now(),
		}
		cache.Set(ctx, entry)
	}
}

func BenchmarkCompression_Get(b *testing.B) {
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	config := DefaultConfig()
	config.EnableCompression = true
	config.CompressionMinSize = 1024

	cache := New(client, config)
	ctx := context.Background()

	htmlBody := []byte(strings.Repeat(`<div class="content"><p>This is some HTML content with text.</p></div>`, 50))
	entry := &Entry{
		URL:        "https://example.com/bench",
		StatusCode: 200,
		Body:       htmlBody,
		StoredAt:   time.Now(),
	}
	cache.Set(ctx, entry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(ctx, entry.URL)
	}
}

func BenchmarkCompression_SetUncompressed(b *testing.B) {
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	config := DefaultConfig()
	config.EnableCompression = false

	cache := New(client, config)
	ctx := context.Background()

	htmlBody := []byte(strings.Repeat(`<div class="content"><p>This is some HTML content with text.</p></div>`, 50))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := &Entry{
			URL:        "https://example.com/bench",
			StatusCode: 200,
			Body:       htmlBody,
			StoredAt:   time.Now(),
		}
		cache.Set(ctx, entry)
	}
}

func BenchmarkCompression_GetUncompressed(b *testing.B) {
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		b.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	config := DefaultConfig()
	config.EnableCompression = false

	cache := New(client, config)
	ctx := context.Background()

	htmlBody := []byte(strings.Repeat(`<div class="content"><p>This is some HTML content with text.</p></div>`, 50))
	entry := &Entry{
		URL:        "https://example.com/bench",
		StatusCode: 200,
		Body:       htmlBody,
		StoredAt:   time.Now(),
	}
	cache.Set(ctx, entry)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(ctx, entry.URL)
	}
}
