package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestXRefEntry_Fields(t *testing.T) {
	entry := XRefEntry{
		Object:             entity.NewInteger(11),
		Offset:             123,
		Type:               EntryTypeCompressed,
		ObjectStreamNumber: 7,
		Generation:         2,
		ObjectStreamIndex:  3,
		Free:               true,
	}

	assert.Equal(t, entity.NewInteger(11), entry.Object)
	assert.Equal(t, uint64(123), entry.Offset)
	assert.Equal(t, EntryTypeCompressed, entry.Type)
	assert.Equal(t, uint32(7), entry.ObjectStreamNumber)
	assert.Equal(t, uint16(2), entry.Generation)
	assert.Equal(t, uint16(3), entry.ObjectStreamIndex)
	assert.True(t, entry.Free)
}

func TestXRefEntryTypeConstants(t *testing.T) {
	assert.Equal(t, EntryType(0), EntryTypeFree)
	assert.Equal(t, EntryType(1), EntryTypeUncompressed)
	assert.Equal(t, EntryType(2), EntryTypeCompressed)
	assert.Equal(t, EntryType(3), EntryTypeNull)
}

func TestXRef_EntriesAreCopyable(t *testing.T) {
	entries := []XRefEntry{
		{Type: EntryTypeFree, Generation: 0},
		{Type: EntryTypeUncompressed, Generation: 2},
	}

	copyEntries := append([]XRefEntry(nil), entries...)

	assert.Len(t, copyEntries, 2)
	assert.Equal(t, entries, copyEntries)
}
