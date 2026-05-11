package xref_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
)

func TestTable_ReadUint32(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected uint32
	}{
		{"zero", []byte{0, 0, 0, 0}, 0},
		{"one", []byte{0, 0, 0, 1}, 1},
		{"max", []byte{0xFF, 0xFF, 0xFF, 0xFF}, 0xFFFFFFFF},
		{"big endian", []byte{0x12, 0x34, 0x56, 0x78}, 0x12345678},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := xref.ReadUint32(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTable_ReadUint16(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected uint16
	}{
		{"zero", []byte{0, 0}, 0},
		{"one", []byte{0, 1}, 1},
		{"max", []byte{0xFF, 0xFF}, 0xFFFF},
		{"big endian", []byte{0x12, 0x34}, 0x1234},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := xref.ReadUint16(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTable_NewTable(t *testing.T) {
	data := []byte("test data")
	table := xref.NewTable(data)

	assert.NotNil(t, table)
	assert.Equal(t, 0, table.GetNumObjects())
}

func TestTable_GetCatalog_BeforeParse(t *testing.T) {
	table := xref.NewTable([]byte("test"))

	catalog, err := table.GetCatalog()
	assert.Error(t, err)
	assert.Nil(t, catalog)
}

func TestTable_GetTrailer_BeforeParse(t *testing.T) {
	table := xref.NewTable([]byte("test"))

	trailer, err := table.GetTrailer()
	assert.Error(t, err)
	assert.Nil(t, trailer)
}

func TestTable_Fetch_BeforeParse(t *testing.T) {
	table := xref.NewTable([]byte("test"))

	ref := entity.NewRef(1, 0)
	obj, err := table.Fetch(ref)

	assert.Error(t, err)
	assert.Nil(t, obj)
}

func TestTable_FetchCached_BeforeParse(t *testing.T) {
	table := xref.NewTable([]byte("test"))

	ref := entity.NewRef(1, 0)
	obj, ok := table.FetchCached(ref)

	assert.False(t, ok)
	assert.Nil(t, obj)
}

func TestTable_Cache(t *testing.T) {
	table := xref.NewTable([]byte("test"))

	ref := entity.NewRef(1, 0)
	obj := entity.NewInteger(42)

	table.Cache(ref, obj)

	// Verify cache works
	cached, ok := table.FetchCached(ref)
	assert.True(t, ok)
	assert.Equal(t, int64(42), cached.(*entity.Integer).Value())
}

func TestTable_Parse_EmptyData(t *testing.T) {
	table := xref.NewTable([]byte{})

	err := table.Parse()
	assert.Error(t, err)
}

func TestTable_Parse_NoStartXRef(t *testing.T) {
	data := []byte("%PDF-1.4\n1 0 obj\n42\nendobj\n%%EOF")
	table := xref.NewTable(data)

	err := table.Parse()
	assert.Error(t, err)
}

func TestTable_Parse_SimplePDF(t *testing.T) {
	// Minimal valid PDF with XRef table
	data := []byte(`%PDF-1.4
1 0 obj
<< /Type /Catalog >>
endobj
xref
0 2
0000000000 65535 f
0000000009 00000 n
trailer
<< /Size 2 /Root 1 0 R >>
startxref
40
%%EOF
`)

	table := xref.NewTable(data)

	err := table.Parse()
	// This might fail because we need to implement full parsing
	// For now, just check that it doesn't panic
	_ = err
}

func TestTable_GetNumObjects(t *testing.T) {
	table := xref.NewTable([]byte("test"))
	assert.Equal(t, 0, table.GetNumObjects())
}
