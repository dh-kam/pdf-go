// Package cache provides LRU cache implementation.
package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	domaincache "github.com/dh-kam/pdf-go/internal/domain/cache"
)

// lruEntry implements a cache entry.
type lruEntry struct {
	createdAt   time.Time
	accessedAt  time.Time
	expiredAt   time.Time
	key         domaincache.CacheKey
	value       interface{}
	prev        *lruEntry
	next        *lruEntry
	size        int64
	accessCount int64
}

// Key returns the cache key.
func (e *lruEntry) Key() domaincache.CacheKey {
	return e.key
}

// Value returns the cached value.
func (e *lruEntry) Value() interface{} {
	return e.value
}

// Size returns the size of the entry in bytes.
func (e *lruEntry) Size() int64 {
	return e.size
}

// CreatedAt returns when the entry was created.
func (e *lruEntry) CreatedAt() time.Time {
	return e.createdAt
}

// AccessedAt returns when the entry was last accessed.
func (e *lruEntry) AccessedAt() time.Time {
	return e.accessedAt
}

// AccessCount returns the number of times the entry was accessed.
func (e *lruEntry) AccessCount() int64 {
	return e.accessCount
}

// ExpiredAt returns when the entry expires.
func (e *lruEntry) ExpiredAt() time.Time {
	return e.expiredAt
}

// IsExpired returns true if the entry is expired.
func (e *lruEntry) IsExpired() bool {
	if e.expiredAt.IsZero() {
		return false
	}
	return time.Now().After(e.expiredAt)
}

// Touch updates the access time and count.
func (e *lruEntry) Touch() {
	e.accessedAt = time.Now()
	e.accessCount++
}

// Prev returns the previous entry in the LRU list.
func (e *lruEntry) Prev() domaincache.LRUEntry {
	if e.prev == nil {
		return nil
	}
	return e.prev
}

// Next returns the next entry in the LRU list.
func (e *lruEntry) Next() domaincache.LRUEntry {
	if e.next == nil {
		return nil
	}
	return e.next
}

// SetPrev sets the previous entry.
func (e *lruEntry) SetPrev(entry domaincache.LRUEntry) {
	if entry == nil {
		e.prev = nil
		return
	}
	if lru, ok := entry.(*lruEntry); ok {
		e.prev = lru
	}
}

// SetNext sets the next entry.
func (e *lruEntry) SetNext(entry domaincache.LRUEntry) {
	if entry == nil {
		e.next = nil
		return
	}
	if lru, ok := entry.(*lruEntry); ok {
		e.next = lru
	}
}

// lruCache implements an LRU cache.
type lruCache struct {
	entries      map[domaincache.CacheKey]*lruEntry
	head         *lruEntry
	tail         *lruEntry
	stats        *cacheStats
	maxSize      int
	maxBytes     int64
	currentBytes int64
	ttl          time.Duration
	mu           sync.RWMutex
}

// NewLRUCache creates a new LRU cache with the given configuration.
func NewLRUCache(config domaincache.CacheConfig) domaincache.LRUCache {
	if config.MaxSize <= 0 {
		config.MaxSize = 1000
	}
	if config.TTL <= 0 {
		config.TTL = time.Hour
	}
	if config.CleanupInterval <= 0 {
		config.CleanupInterval = 5 * time.Minute
	}

	c := &lruCache{
		maxSize:  config.MaxSize,
		maxBytes: config.MaxBytes,
		ttl:      config.TTL,
		entries:  make(map[domaincache.CacheKey]*lruEntry),
		stats:    newCacheStats(),
	}

	// Start cleanup goroutine
	go c.cleanup(config.CleanupInterval)

	return c
}

// Get retrieves a value from the cache.
func (c *lruCache) Get(ctx context.Context, key domaincache.CacheKey) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		c.stats.recordMiss()
		return nil, false
	}

	if entry.IsExpired() {
		c.removeEntry(entry)
		c.stats.recordMiss()
		return nil, false
	}

	// Move to front (most recently used)
	c.moveToFront(entry)
	entry.Touch()

	c.stats.recordHit()
	return entry.value, true
}

// Set stores a value in the cache.
func (c *lruCache) Set(ctx context.Context, key domaincache.CacheKey, value interface{}) error {
	return c.SetWithTTL(ctx, key, value, c.ttl)
}

// SetWithTTL stores a value with a time-to-live.
func (c *lruCache) SetWithTTL(ctx context.Context, key domaincache.CacheKey, value interface{}, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if entry already exists
	if entry, ok := c.entries[key]; ok {
		// Update existing entry
		entry.value = value
		entry.expiredAt = time.Now().Add(ttl)
		c.moveToFront(entry)
		entry.Touch()
		return nil
	}

	// Create new entry
	entry := &lruEntry{
		key:       key,
		value:     value,
		size:      c.estimateSize(value),
		createdAt: time.Now(),
		expiredAt: time.Now().Add(ttl),
	}

	// Check if we need to evict entries
	if c.maxBytes > 0 && (c.currentBytes+entry.size) > c.maxBytes {
		c.evictBytes(entry.size)
	}
	if len(c.entries) >= c.maxSize {
		c.evictOne()
	}

	c.entries[key] = entry
	c.currentBytes += entry.size
	c.moveToFront(entry)

	return nil
}

// Delete removes a value from the cache.
func (c *lruCache) Delete(ctx context.Context, key domaincache.CacheKey) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, ok := c.entries[key]; ok {
		c.removeEntry(entry)
	}

	return nil
}

// Clear removes all entries from the cache.
func (c *lruCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[domaincache.CacheKey]*lruEntry)
	c.currentBytes = 0
	c.head = nil
	c.tail = nil

	return nil
}

// Size returns the number of entries in the cache.
func (c *lruCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Keys returns all cache keys.
func (c *lruCache) Keys() []domaincache.CacheKey {
	c.mu.RLock()
	defer c.mu.RUnlock()

	keys := make([]domaincache.CacheKey, 0, len(c.entries))
	for k := range c.entries {
		keys = append(keys, k)
	}
	return keys
}

// Purge removes expired entries.
func (c *lruCache) Purge(ctx context.Context) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for _, entry := range c.entries {
		if entry.IsExpired() {
			c.removeEntry(entry)
			count++
		}
	}

	return count
}

// Resize changes the cache capacity.
func (c *lruCache) Resize(ctx context.Context, capacity int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.maxBytes = capacity

	// Evict entries if necessary
	for c.currentBytes > c.maxBytes {
		c.evictOne()
	}

	return nil
}

// MaxSize returns the maximum number of entries.
func (c *lruCache) MaxSize() int64 {
	return int64(c.maxSize)
}

// CurrentSize returns the current size in bytes.
func (c *lruCache) CurrentSize() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentBytes
}

// EvictionPolicy returns the eviction policy.
func (c *lruCache) EvictionPolicy() string {
	return "LRU"
}

// moveToFront moves an entry to the front of the LRU list.
func (c *lruCache) moveToFront(entry *lruEntry) {
	if c.head == entry {
		return
	}

	// Remove from current position
	if entry.prev != nil {
		entry.prev.next = entry.next
	}
	if entry.next != nil {
		entry.next.prev = entry.prev
	}

	// Update tail if removing tail
	if c.tail == entry {
		c.tail = entry.prev
	}

	// Insert at head
	entry.prev = nil
	entry.next = c.head
	if c.head != nil {
		c.head.prev = entry
	}
	c.head = entry

	// Update tail if empty
	if c.tail == nil {
		c.tail = entry
	}
}

// evictOne removes the least recently used entry.
func (c *lruCache) evictOne() {
	if c.tail == nil {
		return
	}

	c.removeEntry(c.tail)
	c.stats.recordEviction()
}

// evictBytes evicts entries until we have enough space.
func (c *lruCache) evictBytes(needed int64) {
	for c.currentBytes+needed > c.maxBytes && c.tail != nil {
		c.evictOne()
	}
}

// removeEntry removes an entry from the cache.
func (c *lruCache) removeEntry(entry *lruEntry) {
	delete(c.entries, entry.key)
	c.currentBytes -= entry.size

	// Remove from list
	if entry.prev != nil {
		entry.prev.next = entry.next
	}
	if entry.next != nil {
		entry.next.prev = entry.prev
	}

	if c.head == entry {
		c.head = entry.next
	}
	if c.tail == entry {
		c.tail = entry.prev
	}
}

// estimateSize estimates the size of a value in bytes.
func (c *lruCache) estimateSize(value interface{}) int64 {
	// Default size estimation
	switch v := value.(type) {
	case []byte:
		return int64(len(v))
	case string:
		return int64(len(v))
	default:
		// Default to 1KB for unknown types
		return 1024
	}
}

// cleanup periodically removes expired entries.
func (c *lruCache) cleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		c.Purge(context.Background())
	}
}

// cacheStats implements cache statistics.
type cacheStats struct {
	mu        sync.RWMutex
	hits      int64
	misses    int64
	evictions int64
}

func newCacheStats() *cacheStats {
	return &cacheStats{}
}

func (s *cacheStats) recordHit() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits++
}

func (s *cacheStats) recordMiss() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.misses++
}

func (s *cacheStats) recordEviction() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictions++
}

// Hits is an exported API.
func (s *cacheStats) Hits() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.hits
}

// Misses is an exported API.
func (s *cacheStats) Misses() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.misses
}

// HitRate is an exported API.
func (s *cacheStats) HitRate() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := s.hits + s.misses
	if total == 0 {
		return 0
	}
	return float64(s.hits) / float64(total) * 100
}

// Evictions is an exported API.
func (s *cacheStats) Evictions() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.evictions
}

// Size is an exported API.
func (s *cacheStats) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int(s.hits + s.misses)
}

// Reset is an exported API.
func (s *cacheStats) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hits = 0
	s.misses = 0
	s.evictions = 0
}

// pageCacheKey implements a cache key for pages.
type pageCacheKey struct {
	pageNum int
	dpi     float64
}

// String returns the string representation of the key.
func (k pageCacheKey) String() string {
	return fmt.Sprintf("page_%d_%.0f", k.pageNum, k.dpi)
}

// Equals returns true if the key is equal to another key.
func (k pageCacheKey) Equals(other domaincache.CacheKey) bool {
	if o, ok := other.(pageCacheKey); ok {
		return k.pageNum == o.pageNum && k.dpi == o.dpi
	}
	return false
}

// PageCache implements a cache for rendered pages.
type PageCache struct {
	cache domaincache.LRUCache
}

// NewPageCache creates a new page cache.
func NewPageCache(config domaincache.CacheConfig) *PageCache {
	lruCache := NewLRUCache(config)
	cacheImpl, ok := lruCache.(domaincache.LRUCache)
	if !ok {
		panic("cache.NewLRUCache does not implement domaincache.LRUCache")
	}
	return &PageCache{
		cache: cacheImpl,
	}
}

// GetPage retrieves a rendered page from the cache.
func (c *PageCache) GetPage(ctx context.Context, pageNum int, dpi float64) (interface{}, bool) {
	key := pageCacheKey{pageNum: pageNum, dpi: dpi}
	return c.cache.Get(ctx, key)
}

// SetPage stores a rendered page in the cache.
func (c *PageCache) SetPage(ctx context.Context, pageNum int, dpi float64, page interface{}) error {
	key := pageCacheKey{pageNum: pageNum, dpi: dpi}
	return c.cache.Set(ctx, key, page)
}

// Invalidate invalidates cached pages for a specific page.
func (c *PageCache) Invalidate(ctx context.Context, pageNum int) error {
	// Get the underlying lruCache
	if lru, ok := c.cache.(*lruCache); ok {
		// Collect keys to delete first (under lock)
		lru.mu.RLock()
		var keysToDelete []domaincache.CacheKey
		for key := range lru.entries {
			if pk, ok := key.(pageCacheKey); ok && pk.pageNum == pageNum {
				keysToDelete = append(keysToDelete, key)
			}
		}
		lru.mu.RUnlock()

		// Delete each key (Delete is thread-safe and handles its own locking)
		for _, key := range keysToDelete {
			if err := lru.Delete(ctx, key); err != nil {
				return err
			}
		}
	}

	return nil
}

// InvalidateAll invalidates all cached pages.
func (c *PageCache) InvalidateAll(ctx context.Context) error {
	return c.cache.Clear(ctx)
}

// Get retrieves a value from the cache.
func (c *PageCache) Get(ctx context.Context, key domaincache.CacheKey) (interface{}, bool) {
	return c.cache.Get(ctx, key)
}

// Set stores a value in the cache.
func (c *PageCache) Set(ctx context.Context, key domaincache.CacheKey, value interface{}) error {
	return c.cache.Set(ctx, key, value)
}

// SetWithTTL stores a value with a time-to-live.
func (c *PageCache) SetWithTTL(ctx context.Context, key domaincache.CacheKey, value interface{}, ttl time.Duration) error {
	return c.cache.SetWithTTL(ctx, key, value, ttl)
}

// Delete removes a value from the cache.
func (c *PageCache) Delete(ctx context.Context, key domaincache.CacheKey) error {
	return c.cache.Delete(ctx, key)
}

// Clear removes all entries from the cache.
func (c *PageCache) Clear(ctx context.Context) error {
	return c.cache.Clear(ctx)
}

// Size returns the number of entries in the cache.
func (c *PageCache) Size() int {
	return c.cache.Size()
}

// Keys returns all cache keys.
func (c *PageCache) Keys() []domaincache.CacheKey {
	return c.cache.Keys()
}

// Purge removes expired entries.
func (c *PageCache) Purge(ctx context.Context) int {
	return c.cache.Purge(ctx)
}

// Resize changes the cache capacity.
func (c *PageCache) Resize(ctx context.Context, capacity int64) error {
	return c.cache.Resize(ctx, capacity)
}
