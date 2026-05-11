package cache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestStringKey tests the StringKey type.
func TestStringKey(t *testing.T) {
	t.Run("String returns correct value", func(t *testing.T) {
		key := StringKey("test-key")
		assert.Equal(t, "test-key", key.String())
	})

	t.Run("Equals returns true for same value", func(t *testing.T) {
		key1 := StringKey("test")
		key2 := StringKey("test")
		assert.True(t, key1.Equals(key2))
	})

	t.Run("Equals returns false for different value", func(t *testing.T) {
		key1 := StringKey("test1")
		key2 := StringKey("test2")
		assert.False(t, key1.Equals(key2))
	})

	t.Run("Equals returns false for different type", func(t *testing.T) {
		key := StringKey("test")
		other := &mockCacheKey{}
		assert.False(t, key.Equals(other))
	})
}

// TestCacheConfig tests the CacheConfig type.
func TestCacheConfig(t *testing.T) {
	t.Run("Default values", func(t *testing.T) {
		config := CacheConfig{}
		assert.Equal(t, 0, config.MaxSize)
		assert.Equal(t, int64(0), config.MaxBytes)
		assert.Equal(t, time.Duration(0), config.TTL)
	})

	t.Run("Custom values", func(t *testing.T) {
		config := CacheConfig{
			MaxSize:         100,
			MaxBytes:        1024 * 1024,
			TTL:             time.Minute * 10,
			CleanupInterval: time.Minute,
			EvictionPolicy:  "LRU",
		}
		assert.Equal(t, 100, config.MaxSize)
		assert.Equal(t, int64(1024*1024), config.MaxBytes)
		assert.Equal(t, 10*time.Minute, config.TTL)
		assert.Equal(t, time.Minute, config.CleanupInterval)
		assert.Equal(t, "LRU", config.EvictionPolicy)
	})
}

// TestPoolConfig tests the PoolConfig type.
func TestPoolConfig(t *testing.T) {
	t.Run("Default values", func(t *testing.T) {
		config := PoolConfig{}
		assert.Equal(t, 0, config.MinSize)
		assert.Equal(t, 0, config.MaxSize)
		assert.Nil(t, config.NewFunc)
	})

	t.Run("With NewFunc", func(t *testing.T) {
		config := PoolConfig{
			MinSize: 1,
			MaxSize: 10,
			NewFunc: func() interface{} {
				return "new object"
			},
		}
		assert.Equal(t, 1, config.MinSize)
		assert.Equal(t, 10, config.MaxSize)
		assert.NotNil(t, config.NewFunc)

		result := config.NewFunc()
		assert.Equal(t, "new object", result)
	})
}

// TestCacheKeyInterface tests the CacheKey interface contract.
func TestCacheKeyInterface(t *testing.T) {
	t.Run("StringKey implements CacheKey", func(t *testing.T) {
		var key CacheKey = StringKey("test")
		assert.Equal(t, "test", key.String())
		assert.True(t, key.Equals(StringKey("test")))
		assert.False(t, key.Equals(StringKey("other")))
	})

	t.Run("Custom CacheKey implementation", func(t *testing.T) {
		customKey := &mockCacheKey{id: "test-id"}
		assert.Equal(t, "mock-key:test-id", customKey.String())

		other := &mockCacheKey{id: "test-id"}
		assert.True(t, customKey.Equals(other))

		different := &mockCacheKey{id: "different-id"}
		assert.False(t, customKey.Equals(different))
	})
}

// TestCacheEntryInterface tests the CacheEntry interface contract.
func TestCacheEntryInterface(t *testing.T) {
	t.Run("Mock cache entry", func(t *testing.T) {
		now := time.Now()
		entry := &mockCacheEntry{
			key:         StringKey("test"),
			value:       "test-value",
			size:        100,
			createdAt:   now,
			accessedAt:  now,
			accessCount: 1,
			expiredAt:   now.Add(time.Hour),
		}

		assert.Equal(t, StringKey("test"), entry.Key())
		assert.Equal(t, "test-value", entry.Value())
		assert.Equal(t, int64(100), entry.Size())
		assert.Equal(t, now, entry.CreatedAt())
		assert.Equal(t, now, entry.AccessedAt())
		assert.Equal(t, int64(1), entry.AccessCount())
		assert.Equal(t, now.Add(time.Hour), entry.ExpiredAt())
		assert.False(t, entry.IsExpired())

		// Test Touch
		entry.Touch()
		assert.Equal(t, int64(2), entry.AccessCount())
		assert.True(t, entry.AccessedAt().After(now))
	})

	t.Run("Expired entry", func(t *testing.T) {
		past := time.Now().Add(-time.Hour)
		entry := &mockCacheEntry{
			expiredAt: past,
		}
		assert.True(t, entry.IsExpired())
	})
}

// TestCacheStatsInterface tests the CacheStats interface contract.
func TestCacheStatsInterface(t *testing.T) {
	stats := &mockCacheStats{
		hits:      100,
		misses:    50,
		evictions: 10,
		size:      25,
	}

	assert.Equal(t, int64(100), stats.Hits())
	assert.Equal(t, int64(50), stats.Misses())
	assert.InDelta(t, 66.67, stats.HitRate(), 0.01)
	assert.Equal(t, int64(10), stats.Evictions())
	assert.Equal(t, int64(25), stats.Size())

	// Test Reset
	stats.Reset()
	assert.Equal(t, int64(0), stats.Hits())
	assert.Equal(t, int64(0), stats.Misses())
	assert.Equal(t, float64(0), stats.HitRate())
	assert.Equal(t, int64(0), stats.Evictions())
}

// TestContextCancellation tests context handling in cache operations.
func TestContextCancellation(t *testing.T) {
	t.Run("Cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		key := StringKey("test")

		// Mock cache that respects context cancellation
		cache := &mockCache{}

		_, ok := cache.Get(ctx, key)
		assert.False(t, ok) // Should return false for cancelled context

		err := cache.Set(ctx, key, "value")
		assert.Error(t, err) // Should return error for cancelled context
	})

	t.Run("Timeout context", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
		defer cancel()

		// Wait for timeout
		<-ctx.Done()

		key := StringKey("test")
		cache := &mockCache{}

		_, ok := cache.Get(ctx, key)
		assert.False(t, ok)
	})
}

// TestCacheKeyEquality tests edge cases for key equality.
func TestCacheKeyEquality(t *testing.T) {
	t.Run("Empty StringKey", func(t *testing.T) {
		key1 := StringKey("")
		key2 := StringKey("")
		assert.True(t, key1.Equals(key2))
		assert.Equal(t, "", key1.String())
	})

	t.Run("StringKey with special characters", func(t *testing.T) {
		key := StringKey("test/with/slashes")
		assert.Equal(t, "test/with/slashes", key.String())

		other := StringKey("test/with/slashes")
		assert.True(t, key.Equals(other))
	})
}

// mockCacheKey is a mock implementation of CacheKey for testing.
type mockCacheKey struct {
	id string
}

func (m *mockCacheKey) String() string {
	return "mock-key:" + m.id
}

func (m *mockCacheKey) Equals(other CacheKey) bool {
	if o, ok := other.(*mockCacheKey); ok {
		return m.id == o.id
	}
	return false
}

// mockCacheEntry is a mock implementation of CacheEntry for testing.
type mockCacheEntry struct {
	createdAt   time.Time
	accessedAt  time.Time
	expiredAt   time.Time
	key         CacheKey
	value       interface{}
	size        int64
	accessCount int64
}

func (m *mockCacheEntry) Key() CacheKey {
	return m.key
}

func (m *mockCacheEntry) Value() interface{} {
	return m.value
}

func (m *mockCacheEntry) Size() int64 {
	return m.size
}

func (m *mockCacheEntry) CreatedAt() time.Time {
	return m.createdAt
}

func (m *mockCacheEntry) AccessedAt() time.Time {
	return m.accessedAt
}

func (m *mockCacheEntry) AccessCount() int64 {
	return m.accessCount
}

func (m *mockCacheEntry) ExpiredAt() time.Time {
	return m.expiredAt
}

func (m *mockCacheEntry) IsExpired() bool {
	return time.Now().After(m.expiredAt)
}

func (m *mockCacheEntry) Touch() {
	m.accessedAt = time.Now()
	m.accessCount++
}

// mockCacheStats is a mock implementation of CacheStats for testing.
type mockCacheStats struct {
	hits      int64
	misses    int64
	evictions int64
	size      int64
}

func (m *mockCacheStats) Hits() int64 {
	return m.hits
}

func (m *mockCacheStats) Misses() int64 {
	return m.misses
}

func (m *mockCacheStats) HitRate() float64 {
	total := m.hits + m.misses
	if total == 0 {
		return 0
	}
	return float64(m.hits) / float64(total) * 100
}

func (m *mockCacheStats) Evictions() int64 {
	return m.evictions
}

func (m *mockCacheStats) Size() int64 {
	return m.size
}

func (m *mockCacheStats) Reset() {
	m.hits = 0
	m.misses = 0
	m.evictions = 0
	m.size = 0
}

// mockCache is a mock implementation of Cache for testing context handling.
type mockCache struct{}

func (m *mockCache) Get(ctx context.Context, key CacheKey) (interface{}, bool) {
	select {
	case <-ctx.Done():
		return nil, false
	default:
		return "value", true
	}
}

func (m *mockCache) Set(ctx context.Context, key CacheKey, value interface{}) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (m *mockCache) SetWithTTL(ctx context.Context, key CacheKey, value interface{}, ttl time.Duration) error {
	return m.Set(ctx, key, value)
}

func (m *mockCache) Delete(ctx context.Context, key CacheKey) error {
	return nil
}

func (m *mockCache) Clear(ctx context.Context) error {
	return nil
}

func (m *mockCache) Size() int {
	return 0
}

func (m *mockCache) Keys() []CacheKey {
	return nil
}

func (m *mockCache) Purge(ctx context.Context) int {
	return 0
}

func (m *mockCache) Resize(ctx context.Context, capacity int64) error {
	return nil
}
