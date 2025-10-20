package cache

import (
	"context"
	"testing"
	"time"
)

func TestMemoryCache_GetSet(t *testing.T) {
	cache := NewMemoryCache(DefaultConfig())
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

func TestMemoryCache_Delete(t *testing.T) {
	cache := NewMemoryCache(DefaultConfig())
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

func TestMemoryCache_Clear(t *testing.T) {
	cache := NewMemoryCache(DefaultConfig())
	defer cache.Close()

	ctx := context.Background()

	for i := 0; i < 5; i++ {
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

func TestMemoryCache_Expiration(t *testing.T) {
	config := Config{
		TTL:             100 * time.Millisecond,
		StaleTime:       200 * time.Millisecond,
		CleanupInterval: 50 * time.Millisecond,
	}
	cache := NewMemoryCache(config)
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
	if !entry.IsFresh() {
		t.Error("Entry should be fresh")
	}

	time.Sleep(150 * time.Millisecond)

	entry, _ = cache.Get(ctx, url)
	if entry == nil {
		t.Fatal("Get() should return entry when stale")
	}
	if !entry.IsStale() {
		t.Error("Entry should be stale")
	}

	time.Sleep(200 * time.Millisecond)

	entry, _ = cache.Get(ctx, url)
	if entry != nil {
		t.Error("Get() should return nil when too old")
	}
}

func TestMemoryCache_Cleanup(t *testing.T) {
	config := Config{
		TTL:             50 * time.Millisecond,
		StaleTime:       50 * time.Millisecond,
		CleanupInterval: 100 * time.Millisecond,
	}
	cache := NewMemoryCache(config)
	defer cache.Close()

	ctx := context.Background()

	for i := range 5 {
		entry := &Entry{
			URL:       "https://example.com/" + string(rune('0'+i)),
			Body:      []byte("test"),
			StoredAt:  time.Now(),
			TTL:       50 * time.Millisecond,
			StaleTime: 50 * time.Millisecond,
		}
		cache.Set(ctx, entry)
	}

	time.Sleep(250 * time.Millisecond)

	cache.mu.RLock()
	count := len(cache.entries)
	cache.mu.RUnlock()

	if count != 0 {
		t.Errorf("cleanup should remove expired entries, got %d entries", count)
	}
}

func TestMemoryCache_DefaultConfig(t *testing.T) {
	cache := NewMemoryCache(Config{})
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

func TestEntry_IsFresh(t *testing.T) {
	entry := &Entry{
		StoredAt: time.Now(),
		TTL:      1 * time.Minute,
	}
	if !entry.IsFresh() {
		t.Error("Entry should be fresh")
	}

	entry.StoredAt = time.Now().Add(-2 * time.Minute)
	if entry.IsFresh() {
		t.Error("Entry should not be fresh")
	}
}

func TestEntry_IsStale(t *testing.T) {
	entry := &Entry{
		StoredAt:  time.Now().Add(-2 * time.Minute),
		TTL:       1 * time.Minute,
		StaleTime: 5 * time.Minute,
	}
	if !entry.IsStale() {
		t.Error("Entry should be stale")
	}

	entry.StoredAt = time.Now()
	if entry.IsStale() {
		t.Error("Fresh entry should not be stale")
	}

	entry.StoredAt = time.Now().Add(-10 * time.Minute)
	if entry.IsStale() {
		t.Error("Too old entry should not be stale")
	}
}

func TestEntry_IsTooOld(t *testing.T) {
	entry := &Entry{
		StoredAt:  time.Now().Add(-10 * time.Minute),
		TTL:       1 * time.Minute,
		StaleTime: 5 * time.Minute,
	}
	if !entry.IsTooOld() {
		t.Error("Entry should be too old")
	}

	entry.StoredAt = time.Now()
	if entry.IsTooOld() {
		t.Error("Fresh entry should not be too old")
	}

	entry.StoredAt = time.Now().Add(-2 * time.Minute)
	if entry.IsTooOld() {
		t.Error("Stale entry should not be too old")
	}
}

func TestMemoryCache_ConcurrentAccess(t *testing.T) {
	cache := NewMemoryCache(DefaultConfig())
	defer cache.Close()

	ctx := context.Background()
	done := make(chan bool)

	for i := range 10 {
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

	for range 10 {
		<-done
	}

	for i := range 10 {
		go func(idx int) {
			url := "https://example.com/" + string(rune('0'+idx))
			cache.Get(ctx, url)
			done <- true
		}(i)
	}

	for range 10 {
		<-done
	}
}
