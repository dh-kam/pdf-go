package xref

import (
	"bytes"
	"compress/zlib"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/repository"
)

func TestDecodeStreamData_NoFilter(t *testing.T) {
	table := NewTable(nil)
	dict := entity.NewDict()
	raw := []byte("plain stream bytes")

	decoded, err := table.decodeStreamData(dict, raw)
	require.NoError(t, err)
	assert.Equal(t, raw, decoded)
}

func TestDecodeStreamData_FlateDecode(t *testing.T) {
	table := NewTable(nil)
	dict := entity.NewDict()
	dict.Set(entity.Name("Filter"), entity.Name("FlateDecode"))

	expected := []byte("decoded stream content")
	var compressed bytes.Buffer
	writer := zlib.NewWriter(&compressed)
	_, err := writer.Write(expected)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	decoded, err := table.decodeStreamData(dict, compressed.Bytes())
	require.NoError(t, err)
	assert.Equal(t, expected, decoded)
}

func TestDecodeStreamData_UnsupportedFilter(t *testing.T) {
	table := NewTable(nil)
	dict := entity.NewDict()
	dict.Set(entity.Name("Filter"), entity.Name("UnknownDecode"))

	_, err := table.decodeStreamData(dict, []byte("raw"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported filter")
}

func TestExtractXRefStreamDataPreservesLeadingNullStreamByte(t *testing.T) {
	data := []byte(
		"1 0 obj\n" +
			"<< /Type /XRef /Length 4 >>\n" +
			"stream\n" +
			"\x00ABC\n" +
			"endstream\n" +
			"endobj\n",
	)
	table := NewTable(data)
	dict := entity.NewDict()
	dict.Set(entity.Name("/Length"), entity.NewInteger(4))

	streamData, err := table.extractStreamDataFromOffset(0, dict)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00, 'A', 'B', 'C'}, streamData)
}

func TestExtractXRefStreamDataPreservesLeadingSpaceStreamByte(t *testing.T) {
	data := []byte(
		"1 0 obj\n" +
			"<< /Type /XRef /Length 4 >>\n" +
			"stream\n" +
			" ABC\n" +
			"endstream\n" +
			"endobj\n",
	)
	table := NewTable(data)
	dict := entity.NewDict()
	dict.Set(entity.Name("/Length"), entity.NewInteger(4))

	streamData, err := table.extractStreamDataFromOffset(0, dict)
	require.NoError(t, err)
	assert.Equal(t, []byte{' ', 'A', 'B', 'C'}, streamData)
}

func TestReadXRefStreamEntryDefaultsOmittedTypeToUncompressed(t *testing.T) {
	table := NewTable(nil)
	data := []byte{0x00, 0x00, 0x12, 0x34, 0x00, 0x02}
	pos := 0

	entry, size, err := table.readXRefStreamEntry(data, &pos, []int{0, 4, 2})

	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, repository.EntryTypeUncompressed, entry.Type)
	assert.False(t, entry.Free)
	assert.Equal(t, uint64(0x1234), entry.Offset)
	assert.Equal(t, uint16(2), entry.Generation)
	assert.Equal(t, 6, size)
	assert.Equal(t, 6, pos)
}
