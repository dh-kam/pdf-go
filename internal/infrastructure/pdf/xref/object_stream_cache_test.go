package xref

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestObjectStreamCacheStoresWithinLimits(t *testing.T) {
	table := NewTable(nil)
	parsed := &parsedObjectStream{
		decodedData: []byte("obj"),
		offsets:     []int{0},
		objNumbers:  []int64{1},
	}

	table.cacheParsedObjectStream(7, parsed)

	require.Same(t, parsed, table.objectStreamCache[7])
	require.Equal(t, parsed.cacheBytes(), table.objectStreamBytes)
}

func TestObjectStreamCacheSkipsWhenTotalLimitWouldBeExceeded(t *testing.T) {
	table := NewTable(nil)
	table.objectStreamBytes = maxTotalCachedObjectStreamBytes - 8
	parsed := &parsedObjectStream{
		decodedData: []byte("too-large-for-remaining-cap"),
	}

	table.cacheParsedObjectStream(9, parsed)

	require.Nil(t, table.objectStreamCache[9])
	require.Equal(t, maxTotalCachedObjectStreamBytes-8, table.objectStreamBytes)
}
