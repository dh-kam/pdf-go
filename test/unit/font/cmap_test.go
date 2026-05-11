package font_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/cmap"
)

func TestCMapParser_Parse_Basic(t *testing.T) {
	data := `/CMapName /TestCMap def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 4 >> def
/WMode 0 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <20> 1] def
/CIDChar <21> 100`

	cmap, err := cmap.ParseString(data)
	require.NoError(t, err)

	assert.Equal(t, "TestCMap", cmap.Name())
	assert.True(t, cmap.IsCIDBased())
}

func TestCMapParser_LookupCID(t *testing.T) {
	data := `/CMapName /TestCMap def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 4 >> def
/CodeSpaceRange [<00> <FF>] def
/CIDChar <41> 100
/CIDChar <42> 200`

	cmap, err := cmap.ParseString(data)
	require.NoError(t, err)

	// Test direct CID mapping
	cid, ok := cmap.LookupCID(0x41)
	assert.True(t, ok)
	assert.Equal(t, uint32(100), cid)

	cid, ok = cmap.LookupCID(0x42)
	assert.True(t, ok)
	assert.Equal(t, uint32(200), cid)

	// Test non-existent mapping
	_, ok = cmap.LookupCID(0x43)
	assert.False(t, ok)
}

func TestCMapParser_CIDRange(t *testing.T) {
	data := `/CMapName /TestCMap def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 4 >> def
/CIDRange [<10> <15> 100] def`

	cmap, err := cmap.ParseString(data)
	require.NoError(t, err)

	// Test range mapping: <10>-<15> should map to CIDs 100-105
	tests := []struct {
		code     uint32
		expected uint32
		found    bool
	}{
		{0x10, 100, true},
		{0x11, 101, true},
		{0x12, 102, true},
		{0x13, 103, true},
		{0x14, 104, true},
		{0x15, 105, true},
		{0x16, 0, false}, // Outside range
		{0x0F, 0, false}, // Outside range
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			cid, ok := cmap.LookupCID(tt.code)
			assert.Equal(t, tt.found, ok)
			if tt.found {
				assert.Equal(t, tt.expected, cid)
			}
		})
	}
}

func TestCMapParser_WritingMode(t *testing.T) {
	t.Run("horizontal", func(t *testing.T) {
		data := `/CMapName /TestCMap def
/CMapType 1 def
/WMode 0 def`

		cmap, err := cmap.ParseString(data)
		require.NoError(t, err)
		// WMode 0 means horizontal writing
		assert.NotNil(t, cmap)
	})

	t.Run("vertical", func(t *testing.T) {
		data := `/CMapName /TestCMap def
/CMapType 1 def
/WMode 1 def`

		cmap, err := cmap.ParseString(data)
		require.NoError(t, err)
		// WMode 1 means vertical writing
		assert.NotNil(t, cmap)
	})
}

func TestPredefinedCMap(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"RKSJ-H"},
		{"RKSJ-V"},
		{"EUC-H"},
		{"EUC-V"},
		{"GBK-EUC-H"},
		{"CNS-EUC-H"},
		{"KSC-EUC-H"},
		{"Adobe-Japan1-0"},
		{"Adobe-Korea1-0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmap, err := cmap.PredefinedCMap(tt.name)
			require.NoError(t, err)
			assert.NotNil(t, cmap)
			assert.Equal(t, tt.name, cmap.Name())
		})
	}
}

func TestPredefinedCMap_NotFound(t *testing.T) {
	_, err := cmap.PredefinedCMap("NonExistent-CMap")
	assert.Error(t, err)
}

func TestBaseCMap_SetCIDMapping(t *testing.T) {
	// Test the SetCIDMapping method directly through parsing
	data := `/CMapName /TestCMap def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 4 >> def
/CIDChar <41> 100
/CIDChar <42> 200`

	cmap, err := cmap.ParseString(data)
	require.NoError(t, err)

	assert.True(t, cmap.IsCIDBased())
}

func TestBaseCMap_SetUnicodeMapping(t *testing.T) {
	// Create a base CMap and test Unicode mapping
	data := `/CMapName /TestCMap def
/CMapType 1 def`

	cmap, err := cmap.ParseString(data)
	require.NoError(t, err)

	// This would test Unicode mapping if we had that in the CMap format
	// For now, just verify the CMap is not nil
	assert.NotNil(t, cmap)
}

func TestCMapParser_CommentHandling(t *testing.T) {
	data := `% This is a comment
/CMapName /TestCMap def
% Another comment
/CMapType 1 def
% Yet another comment`

	cmap, err := cmap.ParseString(data)
	require.NoError(t, err)
	assert.Equal(t, "TestCMap", cmap.Name())
}

func TestCMapParser_EmptyInput(t *testing.T) {
	data := ``

	cmap, err := cmap.ParseString(data)
	require.NoError(t, err)
	assert.NotNil(t, cmap)
}

func TestCMapParser_InvalidHex(t *testing.T) {
	data := `/CMapName /TestCMap def
/CMapChar <ZZ> 100`

	// Should not panic, just handle gracefully
	cmap, err := cmap.ParseString(data)
	require.NoError(t, err)
	assert.NotNil(t, cmap)
}

func TestCMap_CIDToUnicode(t *testing.T) {
	t.Run("range based", func(t *testing.T) {
		m := entity.NewRangeCIDtoUnicodeMap()
		m.AddRange(1, 10, 0x4E00) // CJK Unified Ideographs start

		r, ok := m.ToUnicode(1)
		assert.True(t, ok)
		assert.Equal(t, rune(0x4E00), r)

		r, ok = m.ToUnicode(5)
		assert.True(t, ok)
		assert.Equal(t, rune(0x4E04), r)

		_, ok = m.ToUnicode(11)
		assert.False(t, ok)
	})

	t.Run("map based", func(t *testing.T) {
		m := entity.NewMapCIDtoUnicodeMap()
		m.Add(1, 0x4E00)
		m.Add(2, 0x4E01)

		r, ok := m.ToUnicode(1)
		assert.True(t, ok)
		assert.Equal(t, rune(0x4E00), r)

		_, ok = m.ToUnicode(3)
		assert.False(t, ok)
	})
}

func TestCMap_ParseBytes(t *testing.T) {
	data := []byte(`/CMapName /TestCMap def
/CMapType 1 def`)

	cmap, err := cmap.ParseBytes(data)
	require.NoError(t, err)
	assert.NotNil(t, cmap)
}

func TestCMap_SystemInfo(t *testing.T) {
	data := `/CMapName /TestCMap def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 4 >> def`

	cmap, err := cmap.ParseString(data)
	require.NoError(t, err)

	// The parser should have captured the system info
	assert.NotNil(t, cmap)
}

func TestCMap_MultipleRanges(t *testing.T) {
	data := `/CMapName /TestCMap def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 4 >> def
/CIDRange [<00> <05> 1]
/CIDRange [<10> <15> 100] def`

	cmap, err := cmap.ParseString(data)
	require.NoError(t, err)

	// First range: <00>-<05> -> CIDs 1-6
	cid, ok := cmap.LookupCID(0x00)
	assert.True(t, ok)
	assert.Equal(t, uint32(1), cid)

	cid, ok = cmap.LookupCID(0x05)
	assert.True(t, ok)
	assert.Equal(t, uint32(6), cid)

	// Second range: <10>-<15> -> CIDs 100-105
	cid, ok = cmap.LookupCID(0x10)
	assert.True(t, ok)
	assert.Equal(t, uint32(100), cid)

	cid, ok = cmap.LookupCID(0x15)
	assert.True(t, ok)
	assert.Equal(t, uint32(105), cid)
}
