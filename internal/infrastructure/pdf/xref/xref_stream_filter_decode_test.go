package xref

import (
	"bytes"
	"compress/zlib"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestDecodeXRefStream_NoFilter(t *testing.T) {
	table := NewTable(nil)
	dict := entity.NewDict()
	raw := []byte("raw xref stream")

	decoded, err := table.decodeStream(raw, dict)
	require.NoError(t, err)
	assert.Equal(t, raw, decoded)
}

func TestDecodeXRefStream_FlateDecode(t *testing.T) {
	table := NewTable(nil)
	dict := entity.NewDict()
	dict.Set(entity.Name("Filter"), entity.Name("FlateDecode"))

	expected := []byte("decoded xref stream")
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	_, err := writer.Write(expected)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	decoded, err := table.decodeStream(compressed.Bytes(), dict)
	require.NoError(t, err)
	assert.Equal(t, expected, decoded)
}

func TestDecodeXRefStream_UnsupportedFilter(t *testing.T) {
	table := NewTable(nil)
	dict := entity.NewDict()
	dict.Set(entity.Name("Filter"), entity.Name("UnknownDecode"))

	_, err := table.decodeStream([]byte("raw"), dict)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported filter")
}
