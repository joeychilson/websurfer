package cache

import (
	"context"
	"sync"
	"time"
)

// MemoryCache is an in-memory cache implementation.
type MemoryCache struct {
	entries map[string]*Entry
	mu      sync.RWMutex
	config  Config
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewMemoryCache creates a new in-memory cache with automatic cleanup.
func NewMemoryCache(config Config) *MemoryCache {
	if config.TTL == 0 {
		config.TTL = DefaultConfig().TTL
	}
	if config.StaleTime == 0 {
		config.StaleTime = DefaultConfig().StaleTime
	}
	if config.CleanupInterval == 0 {
		config.CleanupInterval = DefaultConfig().CleanupInterval
	}

	mc := &MemoryCache{
		entries: make(map[string]*Entry),
		config:  config,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}

	go mc.cleanup()

	return mc
}

// Get retrieves an entry from the cache.
// Returns nil if the entry doesn't exist or is too old.
func (mc *MemoryCache) Get(ctx context.Context, url string) (*Entry, error) {
	mc.mu.RLock()
	entry, exists := mc.entries[url]
	mc.mu.RUnlock()

	if !exists {
		return nil, nil
	}

	if entry.IsTooOld() {
		mc.mu.Lock()
		delete(mc.entries, url)
		mc.mu.Unlock()
		return nil, nil
	}

	return entry, nil
}

// Set stores an entry in the cache.
func (mc *MemoryCache) Set(ctx context.Context, entry *Entry) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if entry.TTL == 0 {
		entry.TTL = mc.config.TTL
	}
	if entry.StaleTime == 0 {
		entry.StaleTime = mc.config.StaleTime
	}

	entryCopy := &Entry{
		URL:         entry.URL,
		StatusCode:  entry.StatusCode,
		Headers:     copyHeaders(entry.Headers),
		Body:        make([]byte, len(entry.Body)),
		Title:       entry.Title,
		Description: entry.Description,
		StoredAt:    entry.StoredAt,
		TTL:         entry.TTL,
		StaleTime:   entry.StaleTime,
	}
	copy(entryCopy.Body, entry.Body)

	mc.entries[entry.URL] = entryCopy
	return nil
}

// Delete removes an entry from the cache.
func (mc *MemoryCache) Delete(ctx context.Context, url string) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	delete(mc.entries, url)
	return nil
}

// Clear removes all entries from the cache.
func (mc *MemoryCache) Clear(ctx context.Context) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.entries = make(map[string]*Entry)
	return nil
}

// Close stops the cleanup goroutine and releases resources.
func (mc *MemoryCache) Close() error {
	close(mc.stopCh)
	<-mc.doneCh
	return nil
}

// cleanup periodically removes expired entries.
func (mc *MemoryCache) cleanup() {
	ticker := time.NewTicker(mc.config.CleanupInterval)
	defer ticker.Stop()
	defer close(mc.doneCh)

	for {
		select {
		case <-ticker.C:
			mc.removeExpired()
		case <-mc.stopCh:
			return
		}
	}
}

// removeExpired removes all entries that are too old.
func (mc *MemoryCache) removeExpired() {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	for url, entry := range mc.entries {
		if entry.IsTooOld() {
			delete(mc.entries, url)
		}
	}
}

// copyHeaders creates a deep copy of HTTP headers.
func copyHeaders(headers map[string][]string) map[string][]string {
	if headers == nil {
		return nil
	}

	headersCopy := make(map[string][]string, len(headers))
	for key, values := range headers {
		valuesCopy := make([]string, len(values))
		copy(valuesCopy, values)
		headersCopy[key] = valuesCopy
	}
	return headersCopy
}
