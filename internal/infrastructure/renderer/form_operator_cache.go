package renderer

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	domaincache "github.com/dh-kam/pdf-go/internal/domain/cache"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
	"github.com/dh-kam/pdf-go/internal/infrastructure/cache"
)

type formOperatorCache struct {
	cache domaincache.LRUCache
	hits  atomic.Int64
	// misses count negative lookups/type mismatches.
	misses atomic.Int64
	sets   atomic.Int64
}

type formOperatorCacheStats struct {
	Hits   int64
	Misses int64
	Sets   int64
}

func newFormOperatorCache(maxSize int, ttl time.Duration) *formOperatorCache {
	if maxSize <= 0 {
		maxSize = 32
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}

	return &formOperatorCache{
		cache: cache.NewLRUCache(domaincache.CacheConfig{
			MaxSize: maxSize,
			TTL:     ttl,
		}),
	}
}

// Get returns the requested value.
func (c *formOperatorCache) Get(xobj *entity.Stream) ([]domainrenderer.Operator, bool) {
	if c == nil || xobj == nil {
		return nil, false
	}

	value, ok := c.cache.Get(context.Background(), formOperatorCacheKey(xobj))
	if !ok {
		c.misses.Add(1)
		return nil, false
	}

	ops, ok := value.([]domainrenderer.Operator)
	if !ok {
		c.misses.Add(1)
		return nil, false
	}

	c.hits.Add(1)
	return append([]domainrenderer.Operator(nil), ops...), true
}

// Set sets the target value.
func (c *formOperatorCache) Set(xobj *entity.Stream, ops []domainrenderer.Operator) {
	if c == nil || xobj == nil || len(ops) == 0 {
		return
	}

	copied := append([]domainrenderer.Operator(nil), ops...)
	if err := c.cache.Set(context.Background(), formOperatorCacheKey(xobj), copied); err != nil {
		return
	}
	c.sets.Add(1)
}

func formOperatorCacheKey(xobj *entity.Stream) domaincache.CacheKey {
	return domaincache.StringKey(fmt.Sprintf("form_stream_%p", xobj))
}

// Stats is an exported API.
func (c *formOperatorCache) Stats() formOperatorCacheStats {
	if c == nil {
		return formOperatorCacheStats{}
	}
	return formOperatorCacheStats{
		Hits:   c.hits.Load(),
		Misses: c.misses.Load(),
		Sets:   c.sets.Load(),
	}
}
