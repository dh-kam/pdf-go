package xref

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/repository"
)

func TestFetchRecoversMissingEntryByObjectHeaderScan(t *testing.T) {
	data := []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n")
	table := NewTable(data)

	obj, err := table.Fetch(entity.NewRef(1, 0))
	require.NoError(t, err)

	dict, ok := obj.(*entity.Dict)
	require.True(t, ok)
	assert.Equal(t, entity.Name("Catalog"), dict.Get(entity.Name("/Type")))

	require.GreaterOrEqual(t, len(table.entries), 2)
	require.NotNil(t, table.entries[1])
	assert.Equal(t, uint64(9), table.entries[1].Offset)
}

func TestFetchRecoversMissingCompressedEntryFromObjectStream(t *testing.T) {
	data := []byte(
		"1 0 obj\n" +
			"<< /Type /ObjStm /N 1 /First 4 /Length 14 >>\n" +
			"stream\n" +
			"2 0 << /A 1 >>\n" +
			"endstream\n" +
			"endobj\n",
	)

	table := NewTable(data)
	table.entries = make([]*repository.XRefEntry, 2)
	table.entries[1] = &repository.XRefEntry{
		Offset:     0,
		Generation: 0,
		Type:       repository.EntryTypeUncompressed,
		Free:       false,
	}

	obj, err := table.Fetch(entity.NewRef(2, 0))
	require.NoError(t, err)

	dict, ok := obj.(*entity.Dict)
	require.True(t, ok)
	intVal, ok := dict.Get(entity.Name("/A")).(*entity.Integer)
	require.True(t, ok)
	assert.Equal(t, int64(1), intVal.Value())

	require.GreaterOrEqual(t, len(table.entries), 3)
	require.NotNil(t, table.entries[2])
	assert.Equal(t, repository.EntryTypeCompressed, table.entries[2].Type)
	assert.Equal(t, uint32(1), table.entries[2].ObjectStreamNumber)
	assert.Equal(t, uint16(0), table.entries[2].ObjectStreamIndex)
}
