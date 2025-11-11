package cache

import (
	"container/list"
	"context"
	"sync"
	"time"
)

// lruEntry wraps a cache entry with LRU tracking.
type lruEntry struct {
	key   string
	entry *Entry
}

// MemoryCache is an in-memory cache implementation with LRU eviction.
type MemoryCache struct {
	entries map[string]*list.Element
	lruList *list.List
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
	if config.MaxEntries == 0 {
		config.MaxEntries = DefaultConfig().MaxEntries
	}

	mc := &MemoryCache{
		entries: make(map[string]*list.Element),
		lruList: list.New(),
		config:  config,
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
	}

	go mc.cleanup()

	return mc
}

// Get retrieves an entry from the cache.
func (mc *MemoryCache) Get(ctx context.Context, url string) (*Entry, error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	elem, exists := mc.entries[url]
	if !exists {
		return nil, nil
	}

	lruEnt := elem.Value.(*lruEntry)
	if lruEnt.entry.IsTooOld() {
		mc.lruList.Remove(elem)
		delete(mc.entries, url)
		return nil, nil
	}

	mc.lruList.MoveToFront(elem)

	return lruEnt.entry, nil
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
		URL:          entry.URL,
		StatusCode:   entry.StatusCode,
		Headers:      entry.Headers,
		Body:         entry.Body,
		Title:        entry.Title,
		Description:  entry.Description,
		LastModified: entry.LastModified,
		StoredAt:     entry.StoredAt,
		TTL:          entry.TTL,
		StaleTime:    entry.StaleTime,
	}

	if elem, exists := mc.entries[entry.URL]; exists {
		lruEnt := elem.Value.(*lruEntry)
		lruEnt.entry = entryCopy
		mc.lruList.MoveToFront(elem)
	} else {
		if mc.config.MaxEntries > 0 && mc.lruList.Len() >= mc.config.MaxEntries {
			oldest := mc.lruList.Back()
			if oldest != nil {
				lruEnt := oldest.Value.(*lruEntry)
				delete(mc.entries, lruEnt.key)
				mc.lruList.Remove(oldest)
			}
		}

		lruEnt := &lruEntry{
			key:   entry.URL,
			entry: entryCopy,
		}
		elem := mc.lruList.PushFront(lruEnt)
		mc.entries[entry.URL] = elem
	}

	return nil
}

// Delete removes an entry from the cache.
func (mc *MemoryCache) Delete(ctx context.Context, url string) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	if elem, exists := mc.entries[url]; exists {
		mc.lruList.Remove(elem)
		delete(mc.entries, url)
	}
	return nil
}

// Clear removes all entries from the cache.
func (mc *MemoryCache) Clear(ctx context.Context) error {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	mc.entries = make(map[string]*list.Element)
	mc.lruList = list.New()
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

	var toRemove []*list.Element
	for elem := mc.lruList.Front(); elem != nil; elem = elem.Next() {
		lruEnt := elem.Value.(*lruEntry)
		if lruEnt.entry.IsTooOld() {
			toRemove = append(toRemove, elem)
		}
	}

	for _, elem := range toRemove {
		lruEnt := elem.Value.(*lruEntry)
		delete(mc.entries, lruEnt.key)
		mc.lruList.Remove(elem)
	}
}
