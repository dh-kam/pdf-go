// Package subset_test provides unit tests for font subsetting.
package subset_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/subset"
)

// TestCalculateChecksum tests the checksum calculation function.
func TestCalculateChecksum(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected uint32
	}{
		{
			name:     "Empty data",
			data:     []byte{},
			expected: 0,
		},
		{
			name:     "Single byte",
			data:     []byte{0x01},
			expected: 0x01000000,
		},
		{
			name:     "Four bytes",
			data:     []byte{0x01, 0x02, 0x03, 0x04},
			expected: 0x01020304,
		},
		{
			name:     "Multiple of 4 bytes",
			data:     []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08},
			expected: 0x01020304 + 0x05060708,
		},
		{
			name:     "Non-multiple of 4 bytes",
			data:     []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06},
			expected: 0x01020304 + 0x05060000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := subset.CalculateChecksum(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestPadTable tests the table padding function.
func TestPadTable(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		expectedLen int
	}{
		{
			name:        "Already aligned",
			data:        []byte{0x01, 0x02, 0x03, 0x04},
			expectedLen: 4,
		},
		{
			name:        "One byte padding needed",
			data:        []byte{0x01, 0x02, 0x03},
			expectedLen: 4,
		},
		{
			name:        "Two bytes padding needed",
			data:        []byte{0x01, 0x02},
			expectedLen: 4,
		},
		{
			name:        "Three bytes padding needed",
			data:        []byte{0x01},
			expectedLen: 4,
		},
		{
			name:        "Five bytes needs three padding",
			data:        []byte{0x01, 0x02, 0x03, 0x04, 0x05},
			expectedLen: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := subset.PadTable(tt.data)
			assert.Equal(t, tt.expectedLen, len(result))
			// Original data should be preserved
			assert.Equal(t, tt.data, result[:len(tt.data)])
		})
	}
}

// createMinimalTrueTypeFont creates a minimal valid TrueType font for testing.
func createMinimalTrueTypeFont() []byte {
	var buf bytes.Buffer

	// SFNT header
	binary.Write(&buf, binary.BigEndian, uint32(0x00010000)) // SFNT version
	binary.Write(&buf, binary.BigEndian, uint16(3))          // numTables

	// Calculate search range, entry selector, range shift
	searchRange := uint16(16)
	entrySelector := uint16(0)
	for (1 << (entrySelector + 1)) <= 3 {
		entrySelector++
	}
	rangeShift := uint32(3*16) - uint32(searchRange)

	binary.Write(&buf, binary.BigEndian, searchRange)
	binary.Write(&buf, binary.BigEndian, entrySelector)
	binary.Write(&buf, binary.BigEndian, rangeShift)

	// Table directory entries (sorted by tag)
	// cmap table
	buf.WriteString("cmap")
	binary.Write(&buf, binary.BigEndian, uint32(0))       // checksum (placeholder)
	binary.Write(&buf, binary.BigEndian, uint32(12+3*16)) // offset
	binary.Write(&buf, binary.BigEndian, uint32(24))      // length

	// head table
	buf.WriteString("head")
	binary.Write(&buf, binary.BigEndian, uint32(0))          // checksum (placeholder)
	binary.Write(&buf, binary.BigEndian, uint32(12+3*16+24)) // offset
	binary.Write(&buf, binary.BigEndian, uint32(54))         // length

	// maxp table
	buf.WriteString("maxp")
	binary.Write(&buf, binary.BigEndian, uint32(0))             // checksum (placeholder)
	binary.Write(&buf, binary.BigEndian, uint32(12+3*16+24+54)) // offset
	binary.Write(&buf, binary.BigEndian, uint32(32))            // length

	// Write minimal cmap table (24 bytes)
	// Cmap header
	binary.Write(&buf, binary.BigEndian, uint16(0)) // version
	binary.Write(&buf, binary.BigEndian, uint16(1)) // numTables
	// Subtable header
	binary.Write(&buf, binary.BigEndian, uint16(0))  // platform ID (Unicode)
	binary.Write(&buf, binary.BigEndian, uint16(4))  // encoding ID
	binary.Write(&buf, binary.BigEndian, uint32(12)) // offset to subtable
	// Format 4 subtable (minimal)
	binary.Write(&buf, binary.BigEndian, uint16(4))  // format
	binary.Write(&buf, binary.BigEndian, uint16(26)) // length
	binary.Write(&buf, binary.BigEndian, uint16(0))  // language
	binary.Write(&buf, binary.BigEndian, uint16(2))  // segCountX2
	binary.Write(&buf, binary.BigEndian, uint16(1))  // searchRange
	binary.Write(&buf, binary.BigEndian, uint16(0))  // entrySelector
	binary.Write(&buf, binary.BigEndian, uint16(0))  // rangeShift
	binary.Write(&buf, binary.BigEndian, uint16(0))  // endCount[0]
	binary.Write(&buf, binary.BigEndian, uint16(0))  // reserved
	binary.Write(&buf, binary.BigEndian, uint16(0))  // startCount[0]
	binary.Write(&buf, binary.BigEndian, int16(0))   // idDelta[0]
	binary.Write(&buf, binary.BigEndian, uint16(0))  // idRangeOffset[0]
	binary.Write(&buf, binary.BigEndian, uint16(0))  // glyphIndexArray[0]

	// Write minimal head table (54 bytes)
	headData := make([]byte, 54)
	binary.BigEndian.PutUint32(headData[0:4], 0x00010000)   // table version
	binary.BigEndian.PutUint32(headData[4:8], 0x5F0F3CF5)   // fontRevision
	binary.BigEndian.PutUint32(headData[12:16], 0x5F0F3CF5) // created
	binary.BigEndian.PutUint32(headData[16:20], 0x5F0F3CF5) // modified
	binary.BigEndian.PutUint16(headData[50:52], 0)          // indexToLocFormat (short)
	buf.Write(headData)

	// Write minimal maxp table (32 bytes)
	maxpData := make([]byte, 32)
	binary.BigEndian.PutUint32(maxpData[0:4], 0x00010000) // version
	binary.BigEndian.PutUint16(maxpData[4:6], 1)          // numGlyphs
	buf.Write(maxpData)

	return buf.Bytes()
}

// TestNewTrueTypeSubsetter tests creating a new TrueType subsetter.
func TestNewTrueTypeSubsetter(t *testing.T) {
	data := createMinimalTrueTypeFont()
	subsetter := subset.NewTrueTypeSubsetter(data)

	assert.NotNil(t, subsetter)
}

// TestTrueTypeSubsetter_parseTableDirectory tests parsing the table directory.
func TestTrueTypeSubsetter_parseTableDirectory(t *testing.T) {
	t.Skip("Full font subsetting requires complete TrueType font with all required tables")

	data := createMinimalTrueTypeFont()
	subsetter := subset.NewTrueTypeSubsetter(data)

	// This is a white-box test accessing internal functionality
	// In production, we'd test through the public API
	// For now, we just verify the Subset method doesn't crash
	_, err := subsetter.Subset()
	assert.NoError(t, err)
}

// TestSubsetter tests the main Subsetter struct.
func TestSubsetter(t *testing.T) {
	// Create a mock font for testing
	mockFont := &mockFont{
		name:   "TestFont",
		isCID:  false,
		glyphs: map[uint32]uint32{65: 1, 66: 2, 67: 3},
	}

	subsetter := subset.NewSubsetter(mockFont)

	// Test adding individual glyphs
	subsetter.AddGlyph(1)
	assert.True(t, subsetter.HasGlyph(1))
	assert.False(t, subsetter.HasGlyph(2))

	// Test adding character codes
	err := subsetter.AddCharCodes([]uint32{65, 66})
	require.NoError(t, err)
	assert.True(t, subsetter.HasGlyph(1)) // 'A' maps to glyph 1
	assert.True(t, subsetter.HasGlyph(2)) // 'B' maps to glyph 2

	// Test glyph count
	assert.Equal(t, 2, subsetter.GlyphCount())
}

func TestSubsetter_SubsetCFF_UsesFontData(t *testing.T) {
	mock := &mockFont{
		name:     "Type1C-Mock",
		glyphs:   map[uint32]uint32{65: 1},
		fontData: []byte{0x01, 0x02, 0x03},
	}
	subsetter := subset.NewSubsetter(mock)
	subsetter.AddGlyph(1)

	data, err := subsetter.SubsetCFF()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x01, 0x02, 0x03}, data)
}

func TestSubsetter_SubsetTrueType_RequiresFontData(t *testing.T) {
	mock := &mockFont{
		name:   "TrueTypeMock",
		glyphs: map[uint32]uint32{65: 1},
	}
	subsetter := subset.NewSubsetter(mock)
	subsetter.AddGlyph(1)

	_, err := subsetter.SubsetTrueType()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "font data is required")
}

func TestSubsetter_SubsetType1_UsesFontData(t *testing.T) {
	mock := &mockFont{
		name:     "Type1Mock",
		glyphs:   map[uint32]uint32{65: 1},
		fontData: []byte("%!PS-AdobeFont-1.0\n/FontType 1 def\n"),
	}
	subsetter := subset.NewSubsetter(mock)
	subsetter.AddGlyph(1)

	data, err := subsetter.SubsetType1()
	require.NoError(t, err)
	assert.Equal(t, mock.fontData, data)
}

func TestSubsetter_SubsetCIDFont_DelegatesToBase(t *testing.T) {
	base := &mockFont{
		name:     "Type1C-Base",
		glyphs:   map[uint32]uint32{65: 1},
		fontData: []byte{0xAA, 0xBB},
	}
	cidFont := &mockCIDFont{
		base: base,
	}
	subsetter := subset.NewSubsetter(cidFont)
	subsetter.AddGlyph(1)

	data, err := subsetter.SubsetCIDFont()
	require.NoError(t, err)
	assert.Equal(t, []byte{0xAA, 0xBB}, data)
}

func TestSubsetter_SubsetCIDFont_UsesCIDRawData(t *testing.T) {
	cidFont := &mockCIDFont{
		fontData: []byte{0x10, 0x20, 0x30},
	}
	subsetter := subset.NewSubsetter(cidFont)
	subsetter.AddGlyph(1)

	data, err := subsetter.SubsetCIDFont()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x10, 0x20, 0x30}, data)
}

func TestSubsetter_SubsetCIDFont_RequiresBaseOrData(t *testing.T) {
	subsetter := subset.NewSubsetter(&mockCIDFont{})
	subsetter.AddGlyph(1)

	_, err := subsetter.SubsetCIDFont()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cid base font or raw data is required")
}

func TestGetSubsetter_CID_DelegatesSubsetPath(t *testing.T) {
	base := &mockFont{
		name:     "Type1C-Base",
		glyphs:   map[uint32]uint32{65: 1},
		fontData: []byte{0xDE, 0xAD},
	}
	cidFont := &mockCIDFont{base: base}

	fontSubset := subset.GetSubsetter(cidFont)
	data, err := fontSubset.Subset()
	require.NoError(t, err)
	assert.Equal(t, []byte{0xDE, 0xAD}, data)
}

func TestSubsetter_Subset_DetectsType1Data(t *testing.T) {
	mock := &mockFont{
		name:     "UnknownEmbeddedFont",
		glyphs:   map[uint32]uint32{65: 1},
		fontData: []byte("%!PS-AdobeFont-1.0\n/FontType 1 def\n"),
	}
	subsetter := subset.NewSubsetter(mock)
	subsetter.AddGlyph(1)

	data, err := subsetter.Subset()
	require.NoError(t, err)
	assert.Equal(t, mock.fontData, data)
}

// mockFont is a mock implementation of entity.Font for testing.
type mockFont struct {
	glyphs   map[uint32]uint32
	fontData []byte
	name     string
	isCID    bool
}

func (m *mockFont) Name() string {
	return m.name
}

func (m *mockFont) IsCIDFont() bool {
	return m.isCID
}

func (m *mockFont) CharCodeToGlyph(charCode uint32) (uint32, error) {
	if glyph, ok := m.glyphs[charCode]; ok {
		return glyph, nil
	}
	return 0, nil
}

// Additional methods required by entity.Font interface
func (m *mockFont) GlyphName(glyph uint32) string {
	return ""
}

func (m *mockFont) GetGlyphWidth(glyph uint32) (float64, error) {
	return 500.0, nil
}

func (m *mockFont) GetBoundingBox() (float64, float64, float64, float64) {
	return 0, 0, 500, 500
}

func (m *mockFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return &entity.GlyphPath{}, nil
}

func (m *mockFont) IsSymbolic() bool {
	return false
}

func (m *mockFont) UnitsPerEm() uint16 {
	return 1000
}

func (m *mockFont) FontData() []byte {
	return append([]byte(nil), m.fontData...)
}

type mockCIDFont struct {
	base     entity.Font
	fontData []byte
}

func (m *mockCIDFont) Name() string {
	return "CIDMock"
}

func (m *mockCIDFont) IsCIDFont() bool {
	return true
}

func (m *mockCIDFont) CharCodeToGlyph(charCode uint32) (uint32, error) {
	if m.base != nil {
		return m.base.CharCodeToGlyph(charCode)
	}
	return 0, nil
}

func (m *mockCIDFont) GlyphName(glyph uint32) string {
	if m.base != nil {
		return m.base.GlyphName(glyph)
	}
	return ""
}

func (m *mockCIDFont) GetGlyphWidth(glyph uint32) (float64, error) {
	if m.base != nil {
		return m.base.GetGlyphWidth(glyph)
	}
	return 500, nil
}

func (m *mockCIDFont) GetBoundingBox() (float64, float64, float64, float64) {
	if m.base != nil {
		return m.base.GetBoundingBox()
	}
	return 0, 0, 500, 500
}

func (m *mockCIDFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	if m.base != nil {
		return m.base.RenderGlyph(glyph, size)
	}
	return &entity.GlyphPath{}, nil
}

func (m *mockCIDFont) IsSymbolic() bool {
	return false
}

func (m *mockCIDFont) UnitsPerEm() uint16 {
	if m.base != nil {
		return m.base.UnitsPerEm()
	}
	return 1000
}

func (m *mockCIDFont) BaseFont() entity.Font {
	return m.base
}

func (m *mockCIDFont) FontData() []byte {
	return append([]byte(nil), m.fontData...)
}
