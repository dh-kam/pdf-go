package xref

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/repository"
)

func TestFetchWrapsStreamWhenLengthIsIndirectReference(t *testing.T) {
	data := []byte(
		"1 0 obj\n" +
			"<< /Type /XObject /Subtype /Form /Length 2 0 R >>\n" +
			"stream\n" +
			"ABC\n" +
			"endstream\n" +
			"endobj\n" +
			"2 0 obj\n" +
			"3\n" +
			"endobj\n",
	)

	table := NewTable(data)
	table.entries = make([]*repository.XRefEntry, 3)
	table.entries[1] = &repository.XRefEntry{
		Offset:     uint64(bytes.Index(data, []byte("1 0 obj"))),
		Generation: 0,
		Type:       repository.EntryTypeUncompressed,
	}
	table.entries[2] = &repository.XRefEntry{
		Offset:     uint64(bytes.Index(data, []byte("2 0 obj"))),
		Generation: 0,
		Type:       repository.EntryTypeUncompressed,
	}

	obj, err := table.Fetch(entity.NewRef(1, 0))
	require.NoError(t, err)

	streamObj, ok := obj.(*entity.Stream)
	require.True(t, ok)
	assert.Equal(t, []byte("ABC"), streamObj.RawBytes())
	assert.Equal(t, entity.Name("Form"), streamObj.Dict().Get(entity.Name("/Subtype")))

	streamData, err := table.GetStreamData(entity.NewRef(1, 0))
	require.NoError(t, err)
	assert.Equal(t, []byte("ABC"), streamData)
}

func TestFetchWrapsStreamWhenDictionaryExceedsLegacySearchWindow(t *testing.T) {
	var data bytes.Buffer
	data.WriteString("1 0 obj\n<< /Type /XObject /Subtype /Form /Length 3 ")
	for idx := 0; idx < 600; idx++ {
		fmt.Fprintf(&data, "/Fm%d %d 0 R ", idx+1, idx+2)
	}
	data.WriteString(">>\nstream\nABC\nendstream\nendobj\n")

	table := NewTable(data.Bytes())
	table.entries = make([]*repository.XRefEntry, 2)
	table.entries[1] = &repository.XRefEntry{
		Offset:     uint64(bytes.Index(data.Bytes(), []byte("1 0 obj"))),
		Generation: 0,
		Type:       repository.EntryTypeUncompressed,
	}

	obj, err := table.Fetch(entity.NewRef(1, 0))
	require.NoError(t, err)

	streamObj, ok := obj.(*entity.Stream)
	require.True(t, ok)
	assert.Equal(t, []byte("ABC"), streamObj.RawBytes())
}

func TestFetchPreservesLeadingNullStreamByte(t *testing.T) {
	data := []byte(
		"1 0 obj\n" +
			"<< /Length 4 >>\n" +
			"stream\n" +
			"\x00ABC\n" +
			"endstream\n" +
			"endobj\n",
	)

	table := NewTable(data)
	table.entries = make([]*repository.XRefEntry, 2)
	table.entries[1] = &repository.XRefEntry{
		Offset:     uint64(bytes.Index(data, []byte("1 0 obj"))),
		Generation: 0,
		Type:       repository.EntryTypeUncompressed,
	}

	obj, err := table.Fetch(entity.NewRef(1, 0))
	require.NoError(t, err)

	streamObj, ok := obj.(*entity.Stream)
	require.True(t, ok)
	assert.Equal(t, []byte{0x00, 'A', 'B', 'C'}, streamObj.RawBytes())

	streamData, err := table.GetStreamData(entity.NewRef(1, 0))
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00, 'A', 'B', 'C'}, streamData)
}
