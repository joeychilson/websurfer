package client

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/joeychilson/websurfer/cache"
)

const (
	// backgroundRefreshTimeout is the maximum time allowed for background cache refresh operations.
	backgroundRefreshTimeout = 30 * time.Second
)

// CacheManager handles all caching operations including background refresh.
type CacheManager struct {
	cache          *cache.Cache
	logger         *slog.Logger
	refreshing     sync.Map
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
	coordinator    *FetchCoordinator
}

// NewCacheManager creates a new cache manager.
func NewCacheManager(cache *cache.Cache, logger *slog.Logger, coordinator *FetchCoordinator) *CacheManager {
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	return &CacheManager{
		cache:          cache,
		logger:         logger,
		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
		coordinator:    coordinator,
	}
}

// Close stops all background refresh operations.
func (m *CacheManager) Close() {
	if m.shutdownCancel != nil {
		m.shutdownCancel()
	}
}

// Get retrieves an entry from cache, returning nil if not found or on error.
func (m *CacheManager) Get(ctx context.Context, urlStr string) *cache.Entry {
	if m.cache == nil {
		return nil
	}

	entry, err := m.cache.Get(ctx, urlStr)
	if err != nil {
		m.logger.Error("cache get failed", "url", urlStr, "error", err)
		return nil
	}

	return entry
}

// Set stores an entry in cache, logging errors but not failing.
func (m *CacheManager) Set(ctx context.Context, entry *cache.Entry) {
	if m.cache == nil {
		return
	}

	if err := m.cache.Set(ctx, entry); err != nil {
		m.logger.Error("cache set failed", "url", entry.URL, "error", err)
	}
}

// StartBackgroundRefresh initiates a background refresh of stale cache content.
func (m *CacheManager) StartBackgroundRefresh(urlStr string, entry *cache.Entry) {
	if m.cache == nil {
		return
	}

	if _, loaded := m.refreshing.LoadOrStore(urlStr, struct{}{}); !loaded {
		go m.refreshInBackground(urlStr, entry)
	} else {
		m.logger.Debug("background refresh already in progress", "url", urlStr)
	}
}

// refreshInBackground performs the actual background refresh work.
func (m *CacheManager) refreshInBackground(urlStr string, entry *cache.Entry) {
	defer func() {
		m.refreshing.Delete(urlStr)
		if r := recover(); r != nil {
			m.logger.Error("background refresh panicked", "url", urlStr, "panic", r)
		}
	}()

	m.logger.Debug("background refresh started", "url", urlStr)

	refreshCtx, cancel := context.WithTimeout(m.shutdownCtx, backgroundRefreshTimeout)
	defer cancel()

	newEntry, err := m.coordinator.Fetch(refreshCtx, urlStr, entry.LastModified)
	if err != nil {
		if m.shutdownCtx.Err() != nil {
			m.logger.Debug("background refresh cancelled due to shutdown", "url", urlStr)
			return
		}
		m.logger.Error("background refresh failed", "url", urlStr, "error", err)
		return
	}

	if newEntry != nil {
		m.handleRefreshWithNewContent(refreshCtx, urlStr, newEntry)
	} else {
		m.handleRefreshNotModified(refreshCtx, urlStr, entry)
	}
}

// handleRefreshWithNewContent stores newly fetched content from background refresh.
func (m *CacheManager) handleRefreshWithNewContent(ctx context.Context, urlStr string, newEntry *cache.Entry) {
	if err := m.cache.Set(ctx, newEntry); err != nil {
		m.logger.Error("background refresh cache set failed", "url", urlStr, "error", err)
	} else {
		m.logger.Debug("background refresh completed with new content", "url", urlStr)
	}
}

// handleRefreshNotModified updates the cache timestamp when content hasn't changed.
func (m *CacheManager) handleRefreshNotModified(ctx context.Context, urlStr string, entry *cache.Entry) {
	m.logger.Debug("background refresh: content not modified", "url", urlStr)
	updatedEntry := entry.WithUpdatedTimestamp()
	if err := m.cache.Set(ctx, updatedEntry); err != nil {
		m.logger.Error("background refresh timestamp update failed", "url", urlStr, "error", err)
	} else {
		m.logger.Debug("background refresh completed (not modified)", "url", urlStr)
	}
}
