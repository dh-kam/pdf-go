// Package cache_test provides tests for caching.
package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domaincache "github.com/dh-kam/pdf-go/internal/domain/cache"
	"github.com/dh-kam/pdf-go/internal/infrastructure/cache"
)

func TestStringKey(t *testing.T) {
	key := domaincache.StringKey("test")

	assert.Equal(t, "test", key.String())
	assert.True(t, key.Equals(domaincache.StringKey("test")))
	assert.False(t, key.Equals(domaincache.StringKey("other")))
}

func TestNewLRUCache(t *testing.T) {
	config := domaincache.CacheConfig{
		MaxSize:         10,
		MaxBytes:        10240,
		TTL:             time.Minute,
		CleanupInterval: time.Second,
	}

	c := cache.NewLRUCache(config)
	assert.NotNil(t, c)
	assert.Equal(t, int64(10), c.MaxSize())
}

func TestLRUCache_GetSet(t *testing.T) {
	ctx := context.Background()
	config := domaincache.CacheConfig{
		MaxSize:  5,
		MaxBytes: 1024,
		TTL:      time.Minute,
	}

	c := cache.NewLRUCache(config)
	key := domaincache.StringKey("test")
	value := []byte("test data")

	// Get non-existent key
	_, ok := c.Get(ctx, key)
	assert.False(t, ok)

	// Set value
	err := c.Set(ctx, key, value)
	require.NoError(t, err)

	// Get existing key
	retrieved, ok := c.Get(ctx, key)
	assert.True(t, ok)
	assert.Equal(t, value, retrieved.([]byte))
}

func TestLRUCache_TTL(t *testing.T) {
	ctx := context.Background()
	config := domaincache.CacheConfig{
		MaxSize: 5,
		TTL:     100 * time.Millisecond,
	}

	c := cache.NewLRUCache(config)
	key := domaincache.StringKey("test")
	value := []byte("test data")

	// Set value
	err := c.Set(ctx, key, value)
	require.NoError(t, err)

	// Immediately get should work
	_, ok := c.Get(ctx, key)
	assert.True(t, ok)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired now
	_, ok = c.Get(ctx, key)
	assert.False(t, ok)
}

func TestLRUCache_Eviction(t *testing.T) {
	ctx := context.Background()
	config := domaincache.CacheConfig{
		MaxSize:  3,
		MaxBytes: 1024,
	}

	c := cache.NewLRUCache(config)

	// Fill cache to max capacity
	for i := 0; i < 3; i++ {
		key := domaincache.StringKey(string(rune('a' + i)))
		value := make([]byte, 100)
		err := c.Set(ctx, key, value)
		require.NoError(t, err)
	}

	assert.Equal(t, 3, c.Size())

	// Add one more entry - should evict least recently used
	key4 := domaincache.StringKey("d")
	value4 := make([]byte, 100)
	err := c.Set(ctx, key4, value4)
	require.NoError(t, err)

	assert.Equal(t, 3, c.Size())

	// First entry should be evicted
	_, ok := c.Get(ctx, domaincache.StringKey("a"))
	assert.False(t, ok)

	// Other entries should still exist
	_, ok = c.Get(ctx, domaincache.StringKey("b"))
	assert.True(t, ok)
}

func TestLRUCache_Delete(t *testing.T) {
	ctx := context.Background()
	config := domaincache.CacheConfig{
		MaxSize: 5,
	}

	c := cache.NewLRUCache(config)
	key := domaincache.StringKey("test")
	value := []byte("test data")

	// Set value
	err := c.Set(ctx, key, value)
	require.NoError(t, err)

	// Delete value
	err = c.Delete(ctx, key)
	require.NoError(t, err)

	// Should be gone
	_, ok := c.Get(ctx, key)
	assert.False(t, ok)
}

func TestLRUCache_Clear(t *testing.T) {
	ctx := context.Background()
	config := domaincache.CacheConfig{
		MaxSize: 5,
	}

	c := cache.NewLRUCache(config)

	// Add some entries
	for i := 0; i < 3; i++ {
		key := domaincache.StringKey(string(rune('a' + i)))
		value := make([]byte, 100)
		err := c.Set(ctx, key, value)
		require.NoError(t, err)
	}

	assert.Equal(t, 3, c.Size())

	// Clear all
	err := c.Clear(ctx)
	require.NoError(t, err)

	assert.Equal(t, 0, c.Size())
}

func TestLRUCache_Keys(t *testing.T) {
	ctx := context.Background()
	config := domaincache.CacheConfig{
		MaxSize: 5,
	}

	c := cache.NewLRUCache(config)

	// Add some entries
	for i := 0; i < 3; i++ {
		key := domaincache.StringKey(string(rune('a' + i)))
		value := make([]byte, 100)
		err := c.Set(ctx, key, value)
		require.NoError(t, err)
	}

	keys := c.Keys()
	assert.Len(t, keys, 3)
}

func TestLRUCache_Purge(t *testing.T) {
	ctx := context.Background()
	config := domaincache.CacheConfig{
		MaxSize: 5,
		TTL:     50 * time.Millisecond,
	}

	c := cache.NewLRUCache(config)

	// Add entries
	for i := 0; i < 3; i++ {
		key := domaincache.StringKey(string(rune('a' + i)))
		value := make([]byte, 100)
		err := c.Set(ctx, key, value)
		require.NoError(t, err)
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Purge expired entries
	count := c.Purge(ctx)
	assert.GreaterOrEqual(t, count, 3) // All entries should be purged
}

func TestLRUCache_Resize(t *testing.T) {
	ctx := context.Background()
	config := domaincache.CacheConfig{
		MaxSize:  10,
		MaxBytes: 5120, // 5KB
	}

	c := cache.NewLRUCache(config)

	// Add some entries
	for i := 0; i < 5; i++ {
		key := domaincache.StringKey(string(rune('a' + i)))
		value := make([]byte, 1024) // 1KB each
		err := c.Set(ctx, key, value)
		require.NoError(t, err)
	}

	// Resize to smaller capacity
	err := c.Resize(ctx, 2048) // 2KB
	require.NoError(t, err)

	// Should have evicted some entries
	assert.LessOrEqual(t, c.Size(), 2)
}

func TestPageCache(t *testing.T) {
	ctx := context.Background()
	config := domaincache.CacheConfig{
		MaxSize: 10,
		TTL:     time.Minute,
	}

	pc := cache.NewPageCache(config)

	// Set page at 72 DPI
	page1 := []byte("page data at 72 DPI")
	err := pc.SetPage(ctx, 1, 72.0, page1)
	require.NoError(t, err)

	// Get page at 72 DPI
	retrieved, ok := pc.GetPage(ctx, 1, 72.0)
	assert.True(t, ok)
	assert.Equal(t, page1, retrieved.([]byte))

	// Get page at different DPI - should miss
	_, ok = pc.GetPage(ctx, 1, 150.0)
	assert.False(t, ok)

	// Set page at different DPI
	page2 := []byte("page data at 150 DPI")
	err = pc.SetPage(ctx, 1, 150.0, page2)
	require.NoError(t, err)

	// Now should get it
	retrieved, ok = pc.GetPage(ctx, 1, 150.0)
	assert.True(t, ok)
	assert.Equal(t, page2, retrieved.([]byte))
}

func TestPageCache_Invalidate(t *testing.T) {
	ctx := context.Background()
	config := domaincache.CacheConfig{
		MaxSize: 10,
	}

	pc := cache.NewPageCache(config)

	// Set pages at different DPIs
	for _, dpi := range []float64{72.0, 96.0, 150.0} {
		page := []byte("page data")
		err := pc.SetPage(ctx, 1, dpi, page)
		require.NoError(t, err)
	}

	// Invalidate page 1 (should remove all DPI variations)
	err := pc.Invalidate(ctx, 1)
	require.NoError(t, err)

	// All DPI variations should be gone
	for _, dpi := range []float64{72.0, 96.0, 150.0} {
		_, ok := pc.GetPage(ctx, 1, dpi)
		assert.False(t, ok)
	}
}

func TestPageCache_InvalidateAll(t *testing.T) {
	ctx := context.Background()
	config := domaincache.CacheConfig{
		MaxSize: 10,
	}

	pc := cache.NewPageCache(config)

	// Add some pages
	for i := 1; i <= 3; i++ {
		page := []byte("page data")
		err := pc.SetPage(ctx, 1, 72.0, page)
		require.NoError(t, err)
	}

	// Invalidate all
	err := pc.InvalidateAll(ctx)
	require.NoError(t, err)

	// All pages should be gone
	_, ok := pc.GetPage(ctx, 1, 72.0)
	assert.False(t, ok)
}

func TestNewBytePool(t *testing.T) {
	config := domaincache.PoolConfig{
		MinSize: 10,
		MaxSize: 100,
	}

	p := cache.NewBytePool(config)
	assert.NotNil(t, p)
}

func TestBytePool_GetBytes(t *testing.T) {
	config := domaincache.PoolConfig{
		MinSize: 10,
		MaxSize: 100,
	}

	p := cache.NewBytePool(config)

	// Get byte slice
	buf1 := p.GetBytes(1024)
	assert.NotNil(t, buf1)
	assert.Len(t, buf1, 1024)

	// Put byte slice back
	p.PutBytes(buf1)

	// Get another byte slice - might reuse the same buffer
	buf2 := p.GetBytes(512)
	assert.NotNil(t, buf2)
	assert.Len(t, buf2, 512)
}

func TestBytePool_PutBytes(t *testing.T) {
	config := domaincache.PoolConfig{
		MinSize: 10,
		MaxSize: 100,
	}

	p := cache.NewBytePool(config)

	// Create buffer
	buf := make([]byte, 4096)

	// Put back to pool
	p.PutBytes(buf)

	// Should not error (buffers in valid size range are pooled)
	// We can't directly test this, but we verify no panic
}

func TestBufferPool(t *testing.T) {
	p := cache.NewBufferPool()
	assert.NotNil(t, p)

	// Get buffer
	buf := p.Get()
	assert.NotNil(t, buf)

	if b, ok := buf.([]byte); ok {
		assert.Equal(t, 4096, cap(b))
	}

	// Put buffer back
	p.Put(buf)
}
