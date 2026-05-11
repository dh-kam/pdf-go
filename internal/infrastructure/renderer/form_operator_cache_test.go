package renderer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
)

func TestFormOperatorCache_SetGetAndIsolation(t *testing.T) {
	c := newFormOperatorCache(4, time.Minute)
	stream := entity.NewStream(entity.NewDict(), []byte("q Q"))
	ops := []domainrenderer.Operator{
		{Opcode: "q"},
		{Opcode: "Q"},
	}

	c.Set(stream, ops)

	got1, ok := c.Get(stream)
	require.True(t, ok)
	require.Len(t, got1, 2)
	assert.Equal(t, "q", got1[0].Opcode)

	got1[0].Opcode = "mutated"
	got2, ok := c.Get(stream)
	require.True(t, ok)
	assert.Equal(t, "q", got2[0].Opcode)
}

func TestFormOperatorCache_TTLExpiration(t *testing.T) {
	c := newFormOperatorCache(4, 10*time.Millisecond)
	stream := entity.NewStream(entity.NewDict(), []byte("q Q"))

	c.Set(stream, []domainrenderer.Operator{{Opcode: "q"}})

	_, ok := c.Get(stream)
	require.True(t, ok)

	time.Sleep(25 * time.Millisecond)
	_, ok = c.Get(stream)
	assert.False(t, ok)
}

func TestFormOperatorCache_UsesStreamIdentity(t *testing.T) {
	c := newFormOperatorCache(4, time.Minute)
	streamA := entity.NewStream(entity.NewDict(), []byte("q Q"))
	streamB := entity.NewStream(entity.NewDict(), []byte("q Q"))

	c.Set(streamA, []domainrenderer.Operator{{Opcode: "q"}})

	_, okA := c.Get(streamA)
	_, okB := c.Get(streamB)
	assert.True(t, okA)
	assert.False(t, okB)
}

func TestFormOperatorCache_Stats(t *testing.T) {
	c := newFormOperatorCache(4, time.Minute)
	stream := entity.NewStream(entity.NewDict(), []byte("q Q"))
	unknown := entity.NewStream(entity.NewDict(), []byte("BT ET"))

	c.Set(stream, []domainrenderer.Operator{{Opcode: "q"}})
	_, _ = c.Get(stream)
	_, _ = c.Get(unknown)

	stats := c.Stats()
	assert.Equal(t, int64(1), stats.Sets)
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
}
