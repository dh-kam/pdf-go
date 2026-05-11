// Package cache provides caching interfaces and types for PDF rendering.
//
//revive:disable:exported
package cache

import (
	"context"
	"time"
)

// CacheKey represents a cache key.
type CacheKey interface {
	// String returns the string representation of the key.
	String() string

	// Equals returns true if the key is equal to another key.
	Equals(CacheKey) bool
}

// StringKey implements CacheKey for string keys.
type StringKey string

// String returns the string representation of the key.
func (k StringKey) String() string {
	return string(k)
}

// Equals returns true if the key is equal to another key.
func (k StringKey) Equals(other CacheKey) bool {
	if o, ok := other.(StringKey); ok {
		return k == o
	}
	return false
}

// CacheEntry represents a cached entry with metadata.
type CacheEntry interface {
	// Key returns the cache key.
	Key() CacheKey

	// Value returns the cached value.
	Value() interface{}

	// Size returns the size of the entry in bytes.
	Size() int64

	// CreatedAt returns when the entry was created.
	CreatedAt() time.Time

	// AccessedAt returns when the entry was last accessed.
	AccessedAt() time.Time

	// AccessCount returns the number of times the entry was accessed.
	AccessCount() int64

	// ExpiredAt returns when the entry expires.
	ExpiredAt() time.Time

	// IsExpired returns true if the entry is expired.
	IsExpired() bool

	// Touch updates the access time and count.
	Touch()
}

// Cache represents a generic cache interface.
type Cache interface {
	// Get retrieves a value from the cache.
	Get(ctx context.Context, key CacheKey) (interface{}, bool)

	// Set stores a value in the cache.
	Set(ctx context.Context, key CacheKey, value interface{}) error

	// SetWithTTL stores a value with a time-to-live.
	SetWithTTL(ctx context.Context, key CacheKey, value interface{}, ttl time.Duration) error

	// Delete removes a value from the cache.
	Delete(ctx context.Context, key CacheKey) error

	// Clear removes all entries from the cache.
	Clear(ctx context.Context) error

	// Size returns the number of entries in the cache.
	Size() int

	// Keys returns all cache keys.
	Keys() []CacheKey

	// Purge removes expired entries.
	Purge(ctx context.Context) int

	// Resize changes the cache capacity.
	Resize(ctx context.Context, capacity int64) error
}

// LRUCache is a cache that evicts least recently used items.
type LRUCache interface {
	Cache

	// MaxSize returns the maximum number of entries.
	MaxSize() int64

	// CurrentSize returns the current size in bytes.
	CurrentSize() int64

	// EvictionPolicy returns the eviction policy.
	EvictionPolicy() string
}

// LRUEntry represents an entry in an LRU cache.
type LRUEntry interface {
	CacheEntry

	// Prev returns the previous entry in the LRU list.
	Prev() LRUEntry

	// Next returns the next entry in the LRU list.
	Next() LRUEntry

	// SetPrev sets the previous entry.
	SetPrev(LRUEntry)

	// SetNext sets the next entry.
	SetNext(LRUEntry)
}

// CacheStats represents cache statistics.
type CacheStats interface {
	// Hits returns the number of cache hits.
	Hits() int64

	// Misses returns the number of cache misses.
	Misses() int64

	// HitRate returns the cache hit rate as a percentage.
	HitRate() float64

	// Evictions returns the number of evicted entries.
	Evictions() int64

	// Size returns the current cache size.
	Size() int64

	// Reset resets the statistics.
	Reset()
}

// CacheConfig represents cache configuration.
type CacheConfig struct {
	EvictionPolicy  string
	MaxSize         int
	MaxBytes        int64
	TTL             time.Duration
	CleanupInterval time.Duration
}

// PageCache represents a cache for rendered pages.
type PageCache interface {
	Cache

	// GetPage retrieves a rendered page from the cache.
	GetPage(ctx context.Context, pageNum int, dpi float64) (interface{}, bool)

	// SetPage stores a rendered page in the cache.
	SetPage(ctx context.Context, pageNum int, dpi float64, page interface{}) error

	// Invalidate invalidates cached pages.
	Invalidate(ctx context.Context, pageNum int) error

	// InvalidateAll invalidates all cached pages.
	InvalidateAll(ctx context.Context) error
}

// FontCache represents a cache for parsed fonts.
type FontCache interface {
	Cache

	// GetFont retrieves a parsed font from the cache.
	GetFont(ctx context.Context, fontName string) (interface{}, bool)

	// SetFont stores a parsed font in the cache.
	SetFont(ctx context.Context, fontName string, font interface{}) error
}

// ImageCache represents a cache for decoded images.
type ImageCache interface {
	Cache

	// GetImage retrieves a decoded image from the cache.
	GetImage(ctx context.Context, imageID string) (interface{}, bool)

	// SetImage stores a decoded image in the cache.
	SetImage(ctx context.Context, imageID string, image interface{}) error
}

// Pool represents a generic object pool.
type Pool interface {
	// Get retrieves an object from the pool.
	Get() interface{}

	// Put returns an object to the pool.
	Put(interface{})

	// New creates a new object.
	New() interface{}

	// Size returns the number of objects in the pool.
	Size() int
}

// BytePool represents a pool of byte slices.
type BytePool interface {
	Pool

	// GetBytes retrieves a byte slice of the given size.
	GetBytes(size int) []byte

	// PutBytes returns a byte slice to the pool.
	PutBytes([]byte)
}

// PoolConfig represents pool configuration.
type PoolConfig struct {
	NewFunc func() interface{}
	MinSize int
	MaxSize int
}
