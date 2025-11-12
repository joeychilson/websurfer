package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// BenchmarkCacheSet measures cache Set performance with various compression settings.
func BenchmarkCacheSet(b *testing.B) {
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	tests := []struct {
		name           string
		size           int
		enableCompress bool
	}{
		{"small_no_compression", 100, false},
		{"small_with_compression", 100, true},
		{"medium_no_compression", 10_000, false},
		{"medium_with_compression", 10_000, true},
		{"large_no_compression", 100_000, false},
		{"large_with_compression", 100_000, true},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			cfg := DefaultConfig()
			cfg.EnableCompression = tt.enableCompress
			cfg.CompressionMinSize = 50

			cache := New(client, cfg)
			ctx := context.Background()

			// Create test entry
			body := make([]byte, tt.size)
			for i := range body {
				body[i] = byte('a' + (i % 26))
			}

			entry := &Entry{
				URL:        "https://example.com/test",
				StatusCode: 200,
				Body:       body,
				StoredAt:   time.Now(),
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := cache.Set(ctx, entry); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkCacheGet measures cache Get performance.
func BenchmarkCacheGet(b *testing.B) {
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		b.Fatal(err)
	}
	defer mr.Close()

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	defer client.Close()

	tests := []struct {
		name           string
		size           int
		enableCompress bool
	}{
		{"small_no_compression", 100, false},
		{"small_with_compression", 100, true},
		{"medium_no_compression", 10_000, false},
		{"medium_with_compression", 10_000, true},
		{"large_no_compression", 100_000, false},
		{"large_with_compression", 100_000, true},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			cfg := DefaultConfig()
			cfg.EnableCompression = tt.enableCompress
			cfg.CompressionMinSize = 50

			cache := New(client, cfg)
			ctx := context.Background()

			// Pre-populate cache
			body := make([]byte, tt.size)
			for i := range body {
				body[i] = byte('a' + (i % 26))
			}

			entry := &Entry{
				URL:        "https://example.com/test",
				StatusCode: 200,
				Body:       body,
				StoredAt:   time.Now(),
			}

			if err := cache.Set(ctx, entry); err != nil {
				b.Fatal(err)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := cache.Get(ctx, entry.URL); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkCacheCompression measures compression overhead.
func BenchmarkCacheCompression(b *testing.B) {
	sizes := []int{1_000, 10_000, 100_000, 1_000_000}

	for _, size := range sizes {
		b.Run(formatSize(size), func(b *testing.B) {
			cfg := DefaultConfig()
			cfg.EnableCompression = true
			cfg.CompressionMinSize = 100

			// Dummy Redis client (we're only testing compression)
			mr := miniredis.NewMiniRedis()
			if err := mr.Start(); err != nil {
				b.Fatal(err)
			}
			defer mr.Close()

			client := redis.NewClient(&redis.Options{
				Addr: mr.Addr(),
			})
			defer client.Close()

			cache := New(client, cfg)

			data := make([]byte, size)
			for i := range data {
				data[i] = byte('a' + (i % 26))
			}

			b.SetBytes(int64(size))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := cache.compress(data)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func formatSize(size int) string {
	if size >= 1_000_000 {
		return "1MB"
	} else if size >= 100_000 {
		return "100KB"
	} else if size >= 10_000 {
		return "10KB"
	}
	return "1KB"
}
