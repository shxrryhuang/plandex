package validation

import (
	"context"
	"sync"
	"time"
)

// CacheEntry stores a cached validation result with expiration
type CacheEntry struct {
	Result    *ValidationResult
	Timestamp time.Time
	TTL       time.Duration
}

// IsExpired returns true if the cache entry has expired
func (e *CacheEntry) IsExpired() bool {
	if e == nil {
		return true
	}
	return time.Since(e.Timestamp) > e.TTL
}

// ValidationCache caches validation results to avoid redundant checks
type ValidationCache struct {
	mu      sync.RWMutex
	entries map[string]*CacheEntry
	enabled bool
}

// NewValidationCache creates a new validation cache
func NewValidationCache(enabled bool) *ValidationCache {
	return &ValidationCache{
		entries: make(map[string]*CacheEntry),
		enabled: enabled,
	}
}

// Get retrieves a cached validation result
func (c *ValidationCache) Get(key string) (*ValidationResult, bool) {
	if !c.enabled {
		return nil, false
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists || entry.IsExpired() {
		return nil, false
	}

	return entry.Result, true
}

// Set stores a validation result in the cache
func (c *ValidationCache) Set(key string, result *ValidationResult, ttl time.Duration) {
	if !c.enabled {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &CacheEntry{
		Result:    result,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
}

// Clear removes all cached entries
func (c *ValidationCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]*CacheEntry)
}

// ClearExpired removes expired entries from the cache
func (c *ValidationCache) ClearExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.entries {
		if entry.IsExpired() {
			delete(c.entries, key)
		}
	}
}

// Size returns the number of cached entries
func (c *ValidationCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.entries)
}

// Enable enables caching
func (c *ValidationCache) Enable() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.enabled = true
}

// Disable disables caching and clears entries
func (c *ValidationCache) Disable() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.enabled = false
	c.entries = make(map[string]*CacheEntry)
}

// Global cache instance
var globalCache = NewValidationCache(false)

// GetGlobalCache returns the global validation cache
func GetGlobalCache() *ValidationCache {
	return globalCache
}

// EnableCaching enables global validation caching
func EnableCaching() {
	globalCache.Enable()
}

// DisableCaching disables global validation caching
func DisableCaching() {
	globalCache.Disable()
}

// CachedValidator wraps a validator with caching support
type CachedValidator struct {
	validator *Validator
	cache     *ValidationCache
	cacheTTL  time.Duration
}

// NewCachedValidator creates a validator with caching
func NewCachedValidator(opts ValidationOptions, cacheTTL time.Duration) *CachedValidator {
	return &CachedValidator{
		validator: NewValidator(opts),
		cache:     globalCache,
		cacheTTL:  cacheTTL,
	}
}

// ValidateAll runs validation with caching
func (cv *CachedValidator) ValidateAll(ctx context.Context) *ValidationResult {
	// Generate cache key based on validation options
	cacheKey := cv.validator.options.String()

	// Try to get from cache
	if cached, found := cv.cache.Get(cacheKey); found {
		return cached
	}

	// Run validation
	result := cv.validator.ValidateAll(ctx)

	// Store in cache
	cv.cache.Set(cacheKey, result, cv.cacheTTL)

	return result
}

// String returns a string representation of validation options for cache key
func (opts ValidationOptions) String() string {
	return string(opts.Phase)
}

// StartCacheCleanup starts a background goroutine to periodically clean expired cache entries
func StartCacheCleanup(interval time.Duration, stopCh <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			globalCache.ClearExpired()
		case <-stopCh:
			return
		}
	}
}
