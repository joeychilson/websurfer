package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func setupTestRedis(t *testing.T) (*RedisCache, *miniredis.Miniredis) {
	t.Helper()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})

	cache := NewRedisCacheWithClient(client, "test:", DefaultConfig())

	return cache, mr
}

func TestRedisCache_GetSet(t *testing.T) {
	cache, mr := setupTestRedis(t)
	defer mr.Close()
	defer cache.Close()

	ctx := context.Background()
	url := "https://example.com"

	entry, err := cache.Get(ctx, url)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if entry != nil {
		t.Error("Get() should return nil for non-existent entry")
	}

	testEntry := &Entry{
		URL:        url,
		StatusCode: 200,
		Headers: map[string][]string{
			"Content-Type": {"text/html"},
		},
		Body:      []byte("<html>test</html>"),
		StoredAt:  time.Now(),
		TTL:       1 * time.Minute,
		StaleTime: 5 * time.Minute,
	}

	if err := cache.Set(ctx, testEntry); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	entry, err = cache.Get(ctx, url)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if entry == nil {
		t.Fatal("Get() should return entry")
	}
	if entry.URL != url {
		t.Errorf("URL = %s, want %s", entry.URL, url)
	}
	if entry.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", entry.StatusCode)
	}
	if string(entry.Body) != "<html>test</html>" {
		t.Errorf("Body = %s, want <html>test</html>", string(entry.Body))
	}
}

func TestRedisCache_Delete(t *testing.T) {
	cache, mr := setupTestRedis(t)
	defer mr.Close()
	defer cache.Close()

	ctx := context.Background()
	url := "https://example.com"

	testEntry := &Entry{
		URL:      url,
		Body:     []byte("test"),
		StoredAt: time.Now(),
	}
	cache.Set(ctx, testEntry)

	if err := cache.Delete(ctx, url); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	entry, err := cache.Get(ctx, url)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if entry != nil {
		t.Error("Get() should return nil after delete")
	}
}

func TestRedisCache_Clear(t *testing.T) {
	cache, mr := setupTestRedis(t)
	defer mr.Close()
	defer cache.Close()

	ctx := context.Background()

	for i := range 5 {
		entry := &Entry{
			URL:      "https://example.com/" + string(rune('0'+i)),
			Body:     []byte("test"),
			StoredAt: time.Now(),
		}
		cache.Set(ctx, entry)
	}

	if err := cache.Clear(ctx); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}

	for i := 0; i < 5; i++ {
		url := "https://example.com/" + string(rune('0'+i))
		entry, _ := cache.Get(ctx, url)
		if entry != nil {
			t.Errorf("Get(%s) should return nil after clear", url)
		}
	}
}

func TestRedisCache_Expiration(t *testing.T) {
	cache, mr := setupTestRedis(t)
	defer mr.Close()
	defer cache.Close()

	ctx := context.Background()
	url := "https://example.com"

	testEntry := &Entry{
		URL:       url,
		Body:      []byte("test"),
		StoredAt:  time.Now(),
		TTL:       100 * time.Millisecond,
		StaleTime: 200 * time.Millisecond,
	}
	cache.Set(ctx, testEntry)

	entry, _ := cache.Get(ctx, url)
	if entry == nil {
		t.Fatal("Get() should return entry when fresh")
	}

	mr.FastForward(350 * time.Millisecond)

	entry, _ = cache.Get(ctx, url)
	if entry != nil {
		t.Error("Get() should return nil when too old")
	}
}

func TestRedisCache_Ping(t *testing.T) {
	cache, mr := setupTestRedis(t)
	defer mr.Close()
	defer cache.Close()

	ctx := context.Background()

	if err := cache.Ping(ctx); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestRedisCache_DefaultConfig(t *testing.T) {
	cache, mr := setupTestRedis(t)
	defer mr.Close()
	defer cache.Close()

	ctx := context.Background()
	url := "https://example.com"

	testEntry := &Entry{
		URL:      url,
		Body:     []byte("test"),
		StoredAt: time.Now(),
	}
	cache.Set(ctx, testEntry)

	entry, _ := cache.Get(ctx, url)
	if entry == nil {
		t.Fatal("Get() should return entry")
	}
	if entry.TTL != DefaultConfig().TTL {
		t.Errorf("TTL = %v, want %v", entry.TTL, DefaultConfig().TTL)
	}
	if entry.StaleTime != DefaultConfig().StaleTime {
		t.Errorf("StaleTime = %v, want %v", entry.StaleTime, DefaultConfig().StaleTime)
	}
}

func TestNewRedisCache(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	config := RedisConfig{
		Addr:   mr.Addr(),
		Prefix: "custom:",
		Config: Config{
			TTL:       10 * time.Minute,
			StaleTime: 30 * time.Minute,
		},
	}

	cache := NewRedisCache(config)
	defer cache.Close()

	if cache.prefix != "custom:" {
		t.Errorf("prefix = %s, want custom:", cache.prefix)
	}
	if cache.config.TTL != 10*time.Minute {
		t.Errorf("TTL = %v, want 10m", cache.config.TTL)
	}

	ctx := context.Background()
	if err := cache.Ping(ctx); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestRedisCache_KeyPrefix(t *testing.T) {
	cache, mr := setupTestRedis(t)
	defer mr.Close()
	defer cache.Close()

	ctx := context.Background()
	url := "https://example.com"

	testEntry := &Entry{
		URL:      url,
		Body:     []byte("test"),
		StoredAt: time.Now(),
	}
	cache.Set(ctx, testEntry)

	expectedKey := "test:https://example.com"
	exists := mr.Exists(expectedKey)
	if !exists {
		t.Errorf("key %s should exist in Redis", expectedKey)
	}
}

func TestRedisCache_ConcurrentAccess(t *testing.T) {
	cache, mr := setupTestRedis(t)
	defer mr.Close()
	defer cache.Close()

	ctx := context.Background()
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			entry := &Entry{
				URL:      "https://example.com/" + string(rune('0'+idx)),
				Body:     []byte("test"),
				StoredAt: time.Now(),
			}
			cache.Set(ctx, entry)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	for i := 0; i < 10; i++ {
		go func(idx int) {
			url := "https://example.com/" + string(rune('0'+idx))
			cache.Get(ctx, url)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
