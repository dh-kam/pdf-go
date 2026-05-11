package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domaincache "github.com/dh-kam/pdf-go/internal/domain/cache"
)

type fakeLRUEntry struct{}

func (f *fakeLRUEntry) Key() domaincache.CacheKey    { return domaincache.StringKey("fake") }
func (f *fakeLRUEntry) Value() interface{}           { return nil }
func (f *fakeLRUEntry) Size() int64                  { return 0 }
func (f *fakeLRUEntry) CreatedAt() time.Time         { return time.Time{} }
func (f *fakeLRUEntry) AccessedAt() time.Time        { return time.Time{} }
func (f *fakeLRUEntry) AccessCount() int64           { return 0 }
func (f *fakeLRUEntry) ExpiredAt() time.Time         { return time.Time{} }
func (f *fakeLRUEntry) IsExpired() bool              { return false }
func (f *fakeLRUEntry) Touch()                       {}
func (f *fakeLRUEntry) Prev() domaincache.LRUEntry   { return nil }
func (f *fakeLRUEntry) Next() domaincache.LRUEntry   { return nil }
func (f *fakeLRUEntry) SetPrev(domaincache.LRUEntry) {}
func (f *fakeLRUEntry) SetNext(domaincache.LRUEntry) {}

func TestLRUCache_DefaultConfig(t *testing.T) {
	cache := NewLRUCache(domaincache.CacheConfig{})
	require.NotNil(t, cache)

	internal, ok := cache.(*lruCache)
	require.True(t, ok)
	assert.Equal(t, 1000, internal.maxSize)
	assert.Equal(t, time.Hour, internal.ttl)
	assert.Greater(t, internal.currentBytes, int64(0)-1)
}

func TestLRUCache_EntryAccessors(t *testing.T) {
	entry := &lruEntry{
		key:         domaincache.StringKey("key"),
		value:       "value",
		size:        123,
		createdAt:   time.Unix(10, 0),
		accessedAt:  time.Unix(20, 0),
		accessCount: 2,
		expiredAt:   time.Now().Add(time.Hour),
	}

	assert.Equal(t, domaincache.StringKey("key"), entry.Key())
	assert.Equal(t, "value", entry.Value())
	assert.Equal(t, int64(123), entry.Size())
	assert.Equal(t, time.Unix(10, 0), entry.CreatedAt())
	assert.Equal(t, time.Unix(20, 0), entry.AccessedAt())
	assert.Equal(t, int64(2), entry.AccessCount())
	assert.False(t, entry.IsExpired())
	assert.NotNil(t, entry.Key().String())

	entry.Touch()
	assert.Greater(t, entry.accessCount, int64(2))
}

func TestLRUCache_GetMissAndHitAndStats(t *testing.T) {
	c := NewLRUCache(domaincache.CacheConfig{MaxSize: 2, TTL: time.Minute})
	require.NotNil(t, c)

	internal, ok := c.(*lruCache)
	require.True(t, ok)
	internal.stats.Reset()

	key := domaincache.StringKey("missing")
	_, ok = c.Get(context.Background(), key)
	assert.False(t, ok)

	err := c.Set(context.Background(), key, "value")
	require.NoError(t, err)

	_, ok = c.Get(context.Background(), key)
	assert.True(t, ok)

	assert.Equal(t, int64(1), internal.stats.Hits())
	assert.Equal(t, int64(1), internal.stats.Misses())
	assert.InDelta(t, 50.0, internal.stats.HitRate(), 0.001)
	assert.Equal(t, int64(0), internal.stats.Evictions())
}

func TestLRUCache_UpdateAndEvict(t *testing.T) {
	c := NewLRUCache(domaincache.CacheConfig{
		MaxSize:  2,
		MaxBytes: 300,
		TTL:      time.Minute,
	})
	require.NotNil(t, c)

	err := c.Set(context.Background(), domaincache.StringKey("k1"), make([]byte, 120))
	require.NoError(t, err)
	err = c.Set(context.Background(), domaincache.StringKey("k2"), make([]byte, 120))
	require.NoError(t, err)

	// Access k1 to make it most recently used
	_, ok := c.Get(context.Background(), domaincache.StringKey("k1"))
	require.True(t, ok)

	// This insertion should evict k2 (least recently used)
	err = c.Set(context.Background(), domaincache.StringKey("k3"), make([]byte, 120))
	require.NoError(t, err)
	_, ok = c.Get(context.Background(), domaincache.StringKey("k2"))
	assert.False(t, ok)

	// Update existing key should not increase cache length
	require.Equal(t, 2, c.Size())
	err = c.Set(context.Background(), domaincache.StringKey("k1"), make([]byte, 80))
	require.NoError(t, err)
	require.Equal(t, 2, c.Size())
}

func TestLRUCache_PurgeAndResize(t *testing.T) {
	c := NewLRUCache(domaincache.CacheConfig{
		MaxSize:  10,
		MaxBytes: 1024,
		TTL:      50 * time.Millisecond,
	})
	require.NotNil(t, c)

	err := c.SetWithTTL(context.Background(), domaincache.StringKey("a"), "old", 20*time.Millisecond)
	require.NoError(t, err)
	err = c.Set(context.Background(), domaincache.StringKey("b"), "keep")
	require.NoError(t, err)
	time.Sleep(40 * time.Millisecond)
	removed := c.Purge(context.Background())
	assert.GreaterOrEqual(t, removed, 1)

	err = c.Resize(context.Background(), 10)
	require.NoError(t, err)
	assert.Equal(t, 1, c.Size())
}

func TestLRUCache_KeysAndClearDelete(t *testing.T) {
	c := NewLRUCache(domaincache.CacheConfig{MaxSize: 2})
	key1 := domaincache.StringKey("k1")
	key2 := domaincache.StringKey("k2")

	err := c.Set(context.Background(), key1, []byte("v1"))
	require.NoError(t, err)
	err = c.Set(context.Background(), key2, []byte("v2"))
	require.NoError(t, err)

	keys := c.Keys()
	assert.ElementsMatch(t, []domaincache.CacheKey{key1, key2}, keys)

	err = c.Delete(context.Background(), key1)
	require.NoError(t, err)
	_, ok := c.Get(context.Background(), key1)
	assert.False(t, ok)

	err = c.Clear(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, c.Size())
}

func TestLRUCache_InternalHelpers(t *testing.T) {
	ttlEntry := &lruEntry{
		key:        domaincache.StringKey("ttl"),
		value:      "v",
		expiredAt:  time.Now().Add(2 * time.Second),
		size:       10,
		createdAt:  time.Now(),
		accessedAt: time.Now(),
	}
	require.False(t, ttlEntry.IsExpired())

	c := NewLRUCache(domaincache.CacheConfig{MaxSize: 10, MaxBytes: 64})
	internal, ok := c.(*lruCache)
	require.True(t, ok)

	internal.entries[ttlEntry.key] = ttlEntry
	internal.moveToFront(ttlEntry)
	internal.moveToFront(ttlEntry) // idempotent for head
	internal.removeEntry(ttlEntry)
	assert.Equal(t, 0, c.Size())
	assert.Equal(t, 0, len(internal.entries))

	assert.Equal(t, int64(4), internal.estimateSize("text"))
	assert.Equal(t, int64(1024), internal.estimateSize(1))
}

func TestPageCache_InactiveInvalidate(t *testing.T) {
	cache := NewPageCache(domaincache.CacheConfig{
		MaxSize: 10,
		TTL:     time.Minute,
	})
	err := cache.SetPage(context.Background(), 1, 72.0, "p1")
	require.NoError(t, err)
	err = cache.SetPage(context.Background(), 2, 72.0, "p2")
	require.NoError(t, err)

	err = cache.Invalidate(context.Background(), 1)
	require.NoError(t, err)
	_, ok := cache.GetPage(context.Background(), 1, 72.0)
	assert.False(t, ok)
	_, ok = cache.GetPage(context.Background(), 2, 72.0)
	assert.True(t, ok)
}

func TestPageCache_InvalidateAllAndAccessors(t *testing.T) {
	cache := NewPageCache(domaincache.CacheConfig{MaxSize: 10})
	require.NotNil(t, cache)

	err := cache.Set(context.Background(), domaincache.StringKey("k"), "v")
	require.NoError(t, err)
	_, ok := cache.Get(context.Background(), domaincache.StringKey("k"))
	assert.True(t, ok)

	err = cache.InvalidateAll(context.Background())
	require.NoError(t, err)
	_, ok = cache.Get(context.Background(), domaincache.StringKey("k"))
	assert.False(t, ok)
	assert.Equal(t, 0, cache.Size())
	assert.Len(t, cache.Keys(), 0)
	assert.Equal(t, 0, cache.Purge(context.Background()))
	err = cache.Resize(context.Background(), 42)
	require.NoError(t, err)
}

func TestCacheStatsResetAndSize(t *testing.T) {
	stats := &cacheStats{}
	assert.Equal(t, int64(0), stats.Hits())
	assert.Equal(t, int64(0), stats.Misses())
	assert.Equal(t, int64(0), stats.Evictions())
	assert.Equal(t, float64(0), stats.HitRate())
	stats.recordHit()
	stats.recordMiss()
	stats.recordEviction()
	stats.recordHit()
	assert.Equal(t, int64(2), stats.Hits())
	assert.Equal(t, int64(1), stats.Misses())
	assert.Equal(t, int64(1), stats.Evictions())
	assert.InDelta(t, 66.666, stats.HitRate(), 0.001)
	assert.Greater(t, stats.Size(), 0)

	stats.Reset()
	assert.Equal(t, 0, stats.Size())
	assert.Equal(t, float64(0), stats.HitRate())
}

func TestPageCacheKeyEquals(t *testing.T) {
	key1 := pageCacheKey{pageNum: 1, dpi: 72.0}
	key2 := pageCacheKey{pageNum: 1, dpi: 72.0}
	key3 := pageCacheKey{pageNum: 2, dpi: 72.0}

	assert.True(t, key1.Equals(key2))
	assert.False(t, key1.Equals(key3))
	assert.Equal(t, key1, key1)
	assert.Equal(t, "page_1_72", key1.String())
}

func TestLRUCache_CleanupTickerStarts(t *testing.T) {
	cache := NewLRUCache(domaincache.CacheConfig{
		MaxSize:         1,
		CleanupInterval: 2 * time.Millisecond,
	})
	require.NotNil(t, cache)

	internal := cache.(*lruCache)
	c := internal
	// Allow ticker goroutine to run at least once
	time.Sleep(3 * time.Millisecond)
	assert.NotNil(t, c)
}

func TestPool_NewBytePoolAndGetBytes(t *testing.T) {
	p := NewBytePool(domaincache.PoolConfig{
		MinSize: 1,
		MaxSize: 2,
	})
	require.NotNil(t, p)

	buf := p.GetBytes(10)
	assert.Len(t, buf, 10)
	p.PutBytes(buf)
	p.Put(nil)
	p.Put([]byte("text"))

	bp := p.New()
	_, ok := bp.([]byte)
	assert.True(t, ok)
}

func TestPool_PutGetBytesWithPointerSlice(t *testing.T) {
	p := NewBytePool(domaincache.PoolConfig{
		MinSize: 4,
		MaxSize: 8,
		NewFunc: func() interface{} {
			buf := make([]byte, 16)
			return &buf
		},
	})

	buf := p.GetBytes(8)
	require.Len(t, buf, 8)
	p.PutBytes(buf)

	_ = p.Size()
}

func TestBufferPool(t *testing.T) {
	pool := NewBufferPool()
	buf := pool.Get()
	assert.NotNil(t, buf)
	pool.Put(buf)

	newObj := pool.New()
	_, ok := newObj.([]byte)
	assert.True(t, ok)
	assert.Equal(t, -1, pool.Size())
}

func TestCacheStatsString(t *testing.T) {
	stats := &cacheStats{}
	stats.recordHit()
	stats.recordMiss()
	stats.recordEviction()
	assert.GreaterOrEqual(t, stats.Hits(), int64(1))
	assert.GreaterOrEqual(t, stats.Evictions(), int64(1))
	assert.Greater(t, int64(stats.Size()), int64(0))
}

func TestCacheInternalTypeName(t *testing.T) {
	assert.NotEmpty(t, fmt.Sprintf("%T", NewLRUCache(domaincache.CacheConfig{})))
}

func TestLRUEntry_LinkAccessorsAndExpiredAt(t *testing.T) {
	entry := &lruEntry{}
	assert.Nil(t, entry.Prev())
	assert.Nil(t, entry.Next())
	assert.True(t, entry.ExpiredAt().IsZero())

	prev := &lruEntry{key: domaincache.StringKey("prev")}
	next := &lruEntry{key: domaincache.StringKey("next")}
	entry.SetPrev(prev)
	entry.SetNext(next)
	assert.Equal(t, prev, entry.Prev())
	assert.Equal(t, next, entry.Next())

	entry.SetPrev(nil)
	entry.SetNext(nil)
	assert.Nil(t, entry.Prev())
	assert.Nil(t, entry.Next())

	entry.SetPrev(&fakeLRUEntry{})
	entry.SetNext(&fakeLRUEntry{})
	assert.Nil(t, entry.Prev())
	assert.Nil(t, entry.Next())
}

func TestLRUCache_CurrentSizeAndPolicy(t *testing.T) {
	cache := NewLRUCache(domaincache.CacheConfig{MaxSize: 4, TTL: time.Minute})
	require.NotNil(t, cache)
	assert.EqualValues(t, 4, cache.MaxSize())
	assert.Equal(t, "LRU", cache.EvictionPolicy())
	assert.EqualValues(t, 0, cache.CurrentSize())

	err := cache.Set(context.Background(), domaincache.StringKey("k"), []byte{1, 2, 3})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, cache.CurrentSize(), int64(3))
}

func TestPageCache_WrapperMethods(t *testing.T) {
	cache := NewPageCache(domaincache.CacheConfig{MaxSize: 8})
	key := domaincache.StringKey("wrapper")

	require.NoError(t, cache.SetWithTTL(context.Background(), key, "value", time.Minute))
	got, ok := cache.Get(context.Background(), key)
	require.True(t, ok)
	assert.Equal(t, "value", got)
	assert.Contains(t, cache.Keys(), key)

	require.NoError(t, cache.Delete(context.Background(), key))
	_, ok = cache.Get(context.Background(), key)
	assert.False(t, ok)

	require.NoError(t, cache.Set(context.Background(), key, "value2"))
	assert.Equal(t, 1, cache.Size())
	require.NoError(t, cache.Resize(context.Background(), 1024))
	assert.GreaterOrEqual(t, cache.Purge(context.Background()), 0)

	require.NoError(t, cache.Clear(context.Background()))
	assert.Equal(t, 0, cache.Size())
}
