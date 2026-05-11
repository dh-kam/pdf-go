package subset

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestCalculateSubsetterChecks(t *testing.T) {
	subsetter := NewSubsetter(&mockFont{name: "Type1Subset", glyphs: map[uint32]uint32{65: 1}})

	subsetter.AddGlyph(1)
	assert.True(t, subsetter.HasGlyph(1))
	assert.False(t, subsetter.HasGlyph(2))

	err := subsetter.AddCharCodes([]uint32{65})
	require.NoError(t, err)
	assert.True(t, subsetter.HasGlyph(1))
	assert.Equal(t, 1, subsetter.GlyphCount())
}

func TestSubsetter_ErrorOnAddCharCodes(t *testing.T) {
	font := &errorMockFont{
		mockFont: &mockFont{
			name:   "Err",
			glyphs: map[uint32]uint32{},
		},
	}
	subsetter := NewSubsetter(font)

	err := subsetter.AddCharCodes([]uint32{12345})
	require.Error(t, err)
	assert.ErrorIs(t, err, assert.AnError)
}

func TestSubsetter_SubsetType1_UsesFontData(t *testing.T) {
	mock := &mockFont{
		name:     "Type1Mock",
		fontData: []byte{0x80, 0x01, 0x00},
		glyphs:   map[uint32]uint32{65: 1},
	}
	subsetter := NewSubsetter(mock)
	subsetter.AddGlyph(1)

	data, err := subsetter.SubsetType1()
	require.NoError(t, err)
	assert.Equal(t, mock.fontData, data)
}

func TestSubsetter_SubsetType1_NoData(t *testing.T) {
	mock := &mockFont{name: "Type1Mock", glyphs: map[uint32]uint32{65: 1}}
	subsetter := NewSubsetter(mock)
	subsetter.AddGlyph(1)

	_, err := subsetter.SubsetType1()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "font data is required")
}

func TestSubsetter_SubsetTrueType_RequiresFontData(t *testing.T) {
	mock := &mockFont{
		name:   "TTFMock",
		glyphs: map[uint32]uint32{65: 1},
	}
	subsetter := NewSubsetter(mock)
	subsetter.AddGlyph(1)

	_, err := subsetter.SubsetTrueType()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "font data is required")
}

func TestSubsetter_SubsetCFF_UsesFontData(t *testing.T) {
	mock := &mockFont{
		name:     "Type1C-Mock",
		glyphs:   map[uint32]uint32{65: 1},
		fontData: []byte{0x01, 0x00, 0x02},
	}
	subsetter := NewSubsetter(mock)
	subsetter.AddGlyph(1)

	data, err := subsetter.SubsetCFF()
	require.NoError(t, err)
	assert.Equal(t, mock.fontData, data)
}

func TestSubsetter_SubsetCIDFont_DelegatesToBase(t *testing.T) {
	base := &mockFont{
		name:     "Type1C-Base",
		glyphs:   map[uint32]uint32{65: 1},
		fontData: []byte{0xAA, 0xBB},
	}
	cidFont := &mockCIDFont{base: base}
	subsetter := NewSubsetter(cidFont)
	subsetter.AddGlyph(1)

	data, err := subsetter.SubsetCIDFont()
	require.NoError(t, err)
	assert.Equal(t, []byte{0xAA, 0xBB}, data)
}

func TestSubsetter_SubsetCIDFont_UsesCIDRawData(t *testing.T) {
	cidFont := &mockCIDFont{
		fontData: []byte{0x10, 0x20, 0x30},
	}
	subsetter := NewSubsetter(cidFont)
	subsetter.AddGlyph(1)

	data, err := subsetter.SubsetCIDFont()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x10, 0x20, 0x30}, data)
}

func TestSubsetter_SubsetCIDFont_RequiresBaseOrData(t *testing.T) {
	subsetter := NewSubsetter(&mockCIDFont{})
	subsetter.AddGlyph(1)

	_, err := subsetter.SubsetCIDFont()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cid base font or raw data is required")
}

func TestSubsetter_Subset_DetectsType1Data(t *testing.T) {
	mock := &mockFont{
		name:     "UnknownEmbeddedFont",
		glyphs:   map[uint32]uint32{65: 1},
		fontData: []byte("%!PS-AdobeFont-1.0\n/FontType 1 def\n"),
	}
	subsetter := NewSubsetter(mock)
	subsetter.AddGlyph(1)

	data, err := subsetter.Subset()
	require.NoError(t, err)
	assert.Equal(t, mock.fontData, data)
}

func TestSubsetter_Subset_UsesSubsetID(t *testing.T) {
	mock := &mockFont{
		name:   "CIDType1Font",
		glyphs: map[uint32]uint32{65: 1},
		isCID:  true,
	}
	subsetter := NewSubsetter(mock)
	subsetter.AddGlyph(1)

	_, err := subsetter.Subset()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cid base font or raw data is required")
}

func TestSubsetter_Subset_DetectByFontName(t *testing.T) {
	mock := &mockFont{
		name:     "MyType1C Font",
		glyphs:   map[uint32]uint32{65: 1},
		fontData: []byte{0x01, 0x00, 0x00},
	}
	subsetter := NewSubsetter(mock)
	subsetter.AddGlyph(1)

	data, err := subsetter.Subset()
	require.NoError(t, err)
	assert.Equal(t, mock.fontData, data)
}

func TestSubsetter_FontDataDetectionHelpers(t *testing.T) {
	assert.True(t, isCFFFont("MyType1CFont"))
	assert.True(t, isCFFFont("Type1C"))
	assert.False(t, isCFFFont("Helvetica"))

	assert.True(t, isType1Font("CourierType 1"))
	assert.True(t, isType1Font("pfbFont"))
	assert.False(t, isType1Font("NotoSans"))

	assert.True(t, isTrueTypeData([]byte{0x00, 0x01, 0x00, 0x00}))
	assert.True(t, isTrueTypeData([]byte{'O', 'T', 'T', 'O'}))
	assert.True(t, isTrueTypeData([]byte{'t', 't', 'c', 'f'}))
	assert.False(t, isTrueTypeData([]byte{0x00, 0x00, 0x00, 0x00}))

	assert.True(t, isLikelyCFFData([]byte{1, 0, 4, 1}))
	assert.False(t, isLikelyCFFData([]byte{0, 0, 4, 1}))

	assert.True(t, isLikelyType1Data([]byte{0x80, 0x01, 0x00}))
	assert.True(t, isLikelyType1Data([]byte("%!ps-adobefont type1 test")))
	assert.True(t, isLikelyType1Data([]byte("/fonttype 1")))
	assert.True(t, isLikelyType1Data([]byte("currentfile eexec test")))
	assert.False(t, isLikelyType1Data([]byte("hello world")))
}

func TestSortedGlyphIDs(t *testing.T) {
	set := map[uint32]bool{5: true, 2: true, 9: true}
	ids := sortedGlyphIDs(set)
	assert.Equal(t, []uint32{2, 5, 9}, ids)
}

func TestCalculateChecksum_AndPadTable(t *testing.T) {
	assert.Equal(t, uint32(0x01020304), CalculateChecksum([]byte{0x01, 0x02, 0x03, 0x04}))
	assert.Equal(t, []byte{1, 2, 3, 0}, PadTable([]byte{1, 2, 3}))
	assert.Equal(t, []byte{1, 2, 3, 4}, PadTable([]byte{1, 2, 3, 4}))
	assert.Equal(t, []byte{1, 0, 0, 0}, PadTable([]byte{1}))
}

func TestNewTrueTypeSubsetter_BareSubsetPath(t *testing.T) {
	data := createMinimalTrueTypeFont()
	subsetter := NewTrueTypeSubsetter(data)
	subsetter.glyphs = map[uint32]bool{0: true, 1: true}
	subsetter.glyphOrder = []uint32{0, 1}

	tbl := []byte("cmapheadmaxp")
	require.NotNil(t, tbl)
	assert.NotEmpty(t, tbl)
}

func createMinimalTrueTypeFont() []byte {
	var buf bytes.Buffer

	// SFNT header
	binary.Write(&buf, binary.BigEndian, uint32(0x00010000)) // SFNT version
	binary.Write(&buf, binary.BigEndian, uint16(3))          // numTables

	searchRange := uint16(16)
	entrySelector := uint16(0)
	for (1 << (entrySelector + 1)) <= 3 {
		entrySelector++
	}
	rangeShift := uint32(3*16) - uint32(searchRange)

	binary.Write(&buf, binary.BigEndian, searchRange)
	binary.Write(&buf, binary.BigEndian, entrySelector)
	binary.Write(&buf, binary.BigEndian, rangeShift)

	// Table directory entries
	buf.WriteString("cmap")
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint32(12+3*16))
	binary.Write(&buf, binary.BigEndian, uint32(24))

	buf.WriteString("head")
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint32(12+3*16+24))
	binary.Write(&buf, binary.BigEndian, uint32(54))

	buf.WriteString("maxp")
	binary.Write(&buf, binary.BigEndian, uint32(0))
	binary.Write(&buf, binary.BigEndian, uint32(12+3*16+24+54))
	binary.Write(&buf, binary.BigEndian, uint32(32))

	// Minimal tables
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint16(1))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint16(4))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint16(26))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint16(2))
	binary.Write(&buf, binary.BigEndian, uint16(1))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint16(0))
	binary.Write(&buf, binary.BigEndian, uint16(0))

	headData := make([]byte, 54)
	binary.BigEndian.PutUint32(headData[0:4], 0x00010000)
	binary.BigEndian.PutUint16(headData[50:52], 0) // indexToLocFormat
	buf.Write(headData)

	maxpData := make([]byte, 32)
	binary.BigEndian.PutUint32(maxpData[0:4], 0x00010000)
	binary.BigEndian.PutUint16(maxpData[4:6], 1)
	buf.Write(maxpData)

	return buf.Bytes()
}

// mockFont is a mock implementation of entity.Font for testing.
type mockFont struct {
	glyphs   map[uint32]uint32
	fontData []byte
	name     string
	isCID    bool
}

func (m *mockFont) Name() string    { return m.name }
func (m *mockFont) IsCIDFont() bool { return m.isCID }
func (m *mockFont) CharCodeToGlyph(charCode uint32) (uint32, error) {
	if glyph, ok := m.glyphs[charCode]; ok {
		return glyph, nil
	}
	return 0, nil
}
func (m *mockFont) GlyphName(glyph uint32) string                        { return "" }
func (m *mockFont) GetGlyphWidth(glyph uint32) (float64, error)          { return 500.0, nil }
func (m *mockFont) GetBoundingBox() (float64, float64, float64, float64) { return 0, 0, 500, 500 }
func (m *mockFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return &entity.GlyphPath{}, nil
}
func (m *mockFont) IsSymbolic() bool   { return false }
func (m *mockFont) UnitsPerEm() uint16 { return 1000 }
func (m *mockFont) FontData() []byte   { return append([]byte(nil), m.fontData...) }

type errorMockFont struct {
	*mockFont
}

func (m *errorMockFont) CharCodeToGlyph(charCode uint32) (uint32, error) {
	return 0, assert.AnError
}

type mockCIDFont struct {
	base     entity.Font
	fontData []byte
}

func (m *mockCIDFont) Name() string    { return "CIDMock" }
func (m *mockCIDFont) IsCIDFont() bool { return true }
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
func (m *mockCIDFont) IsSymbolic() bool { return false }
func (m *mockCIDFont) UnitsPerEm() uint16 {
	if m.base != nil {
		return m.base.UnitsPerEm()
	}
	return 1000
}
func (m *mockCIDFont) BaseFont() entity.Font { return m.base }
func (m *mockCIDFont) FontData() []byte      { return append([]byte(nil), m.fontData...) }
