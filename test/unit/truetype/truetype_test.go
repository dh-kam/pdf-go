package truetype_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/infrastructure/font/truetype"
)

// createTestFont creates a minimal test font file
func createTestFont() []byte {
	// This is a minimal TrueType font file structure
	// For testing, we'll create the basic SFNT header

	buf := bytes.NewBuffer(nil)

	// SFNT version (0x00010000 for TrueType)
	buf.Write([]byte{0x00, 0x01, 0x00, 0x00})

	// Number of tables
	buf.Write([]byte{0x00, 0x02}) // 2 tables

	// Search range, entry selector, range shift
	buf.Write([]byte{0x00, 0x80}) // search range = 256
	buf.Write([]byte{0x00, 0x01}) // entry selector = 1
	buf.Write([]byte{0x00, 0x00}) // range shift = 0

	// Table 1: head (font header)
	// Tag: "head"
	buf.WriteString("head")
	buf.Write([]byte{0x00, 0x00, 0x00, 0x00}) // checksum
	buf.Write([]byte{0x00, 0x00, 0x00, 0x3C}) // offset = 60
	buf.Write([]byte{0x00, 0x00, 0x00, 0x54}) // length = 84

	// Table 2: maxp (maximum profile)
	// Tag: "maxp"
	buf.WriteString("maxp")
	buf.Write([]byte{0x00, 0x00, 0x00, 0x00}) // checksum
	buf.Write([]byte{0x00, 0x00, 0x00, 0x90}) // offset = 144
	buf.Write([]byte{0x00, 0x00, 0x00, 0x20}) // length = 32

	// Padding to 60 bytes
	for i := uint32(0); i < 60-44; i++ {
		buf.Write([]byte{0x00})
	}

	// Write head table data
	// Version
	buf.Write([]byte{0x00, 0x01, 0x00, 0x00})

	// Font revision
	buf.Write([]byte{0x00, 0x00, 0x00, 0x01})

	// Checksum adjustment
	buf.Write([]byte{0x00, 0x00, 0x00, 0x00})

	// Magic number
	buf.Write([]byte{0x5F, 0x0F, 0x3C, 0xF5})

	// Flags
	buf.Write([]byte{0x00, 0x00})

	// Units per em
	buf.Write([]byte{0x04, 0x00}) // 1024

	// Created/Modified (dummy values)
	buf.Write(make([]byte, 16))

	// Font bounding box
	buf.Write([]byte{0x00, 0x00}) // MinX (xMin)
	buf.Write([]byte{0x00, 0x00}) // MinY (yMin)
	buf.Write([]byte{0x00, 0x10}) // MaxX (xMax) = 16
	buf.Write([]byte{0x00, 0x10}) // MaxY (yMax) = 16

	// MacStyle, lowestRecPPEM, fontDirectionHint (2+2+2 = 6 bytes, skipped by parser)
	buf.Write([]byte{0x00, 0x00}) // macStyle
	buf.Write([]byte{0x00, 0x00}) // lowestRecPPEM (part of skip uint32)
	buf.Write([]byte{0x00, 0x00}) // fontDirectionHint (part of skip uint32)

	// IndexToLocFormat
	buf.Write([]byte{0x00, 0x00})

	// GlyphDataFormat
	buf.Write([]byte{0x00, 0x00})

	// Padding to 144 bytes (head table is 54 bytes, so we need 144-60-54 = 30 bytes)
	for i := uint32(0); i < 30; i++ {
		buf.Write([]byte{0x00})
	}

	// Write maxp table data
	// Version
	buf.Write([]byte{0x00, 0x01, 0x00, 0x00})

	// NumGlyphs
	buf.Write([]byte{0x00, 0x01}) // 1 glyph

	// Max points, contours (simplified)
	buf.Write([]byte{0x00, 0x02}) // MaxPoints = 2
	buf.Write([]byte{0x00, 0x01}) // MaxContours = 1

	// Rest of maxp fields (zeroes)
	buf.Write(make([]byte, 28))

	return buf.Bytes()
}

func TestParseFontFile_ValidHeader(t *testing.T) {
	data := createTestFont()
	r := bytes.NewReader(data)

	font, err := truetype.ParseFontFile(r)
	require.NoError(t, err)
	assert.NotNil(t, font)
	assert.Equal(t, uint32(0x00010000), font.SFNTVersion)
	assert.Equal(t, 2, len(font.Tables))
}

func TestParseFontFile_TableEntries(t *testing.T) {
	data := createTestFont()
	r := bytes.NewReader(data)

	font, err := truetype.ParseFontFile(r)
	require.NoError(t, err)

	assert.Equal(t, 2, len(font.Tables))

	// Check head table
	foundHead := false
	foundMaxp := false
	for _, table := range font.Tables {
		if table.Tag == "head" {
			foundHead = true
			assert.Equal(t, uint32(60), table.Offset)
			assert.Equal(t, uint32(84), table.Length)
		}
		if table.Tag == "maxp" {
			foundMaxp = true
			assert.Equal(t, uint32(144), table.Offset)
			assert.Equal(t, uint32(32), table.Length)
		}
	}

	assert.True(t, foundHead, "head table not found")
	assert.True(t, foundMaxp, "maxp table not found")
}

func TestParseFontFile_UnsupportedVersion(t *testing.T) {
	// Invalid SFNT version
	buf := bytes.NewBuffer(nil)
	buf.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF}) // Invalid version
	buf.Write([]byte{0x00, 0x01})             // 1 table
	buf.Write([]byte{0x00, 0x80})             // search range
	buf.Write([]byte{0x00, 0x01})             // entry selector
	buf.Write([]byte{0x00, 0x00})             // range shift
	// Write one table
	buf.WriteString("head")
	buf.Write(make([]byte, 12))

	r := bytes.NewReader(buf.Bytes())
	_, err := truetype.ParseFontFile(r)
	assert.Error(t, err)
}

func TestParseFontFile_EmptyFile(t *testing.T) {
	r := bytes.NewReader([]byte{})

	_, err := truetype.ParseFontFile(r)
	assert.Error(t, err)
}

func TestHeadTable_Parsing(t *testing.T) {
	data := createTestFont()
	r := bytes.NewReader(data)

	font, err := truetype.ParseFontFile(r)
	require.NoError(t, err)

	require.NotNil(t, font.Head)
	assert.Equal(t, uint16(1024), font.Head.UnitsPerEm)
	assert.Equal(t, uint32(0x5F0F3CF5), font.Head.MagicNumber)
	assert.Equal(t, uint16(0), font.Head.IndexToLocFormat)
	assert.Equal(t, uint16(0), font.Head.GlyphDataFormat)
}

func TestMaxpTable_Parsing(t *testing.T) {
	data := createTestFont()
	r := bytes.NewReader(data)

	font, err := truetype.ParseFontFile(r)
	require.NoError(t, err)

	require.NotNil(t, font.Maxp)
	assert.Equal(t, uint16(1), font.Maxp.NumGlyphs)
	assert.Equal(t, uint16(2), font.Maxp.MaxPoints)
	assert.Equal(t, uint16(1), font.Maxp.MaxContours)
}

func TestFont_GetUnitsPerEm(t *testing.T) {
	data := createTestFont()
	r := bytes.NewReader(data)

	fontFile, err := truetype.ParseFontFile(r)
	require.NoError(t, err)

	assert.Equal(t, uint16(1024), fontFile.UnitsPerEm())
}

func TestFont_CharCodeToGlyph_NoCmap(t *testing.T) {
	// Create a font without cmap table
	data := createTestFont()
	r := bytes.NewReader(data)

	fontFile, err := truetype.ParseFontFile(r)
	require.NoError(t, err)

	// No cmap means no character to glyph mapping
	glyphID, ok := fontFile.CharCodeToGlyph(65) // 'A'
	assert.False(t, ok)
	assert.Equal(t, uint16(0), glyphID)
}

func TestFont_GetGlyphWidth_NoHmtx(t *testing.T) {
	// Create a font without hmtx table
	data := createTestFont()
	r := bytes.NewReader(data)

	fontFile, err := truetype.ParseFontFile(r)
	require.NoError(t, err)

	// No hmtx means we can't get width
	_, err = fontFile.GetGlyphWidth(0)
	assert.Error(t, err)
}

func TestFont_GetGlyphData_NoGlyf(t *testing.T) {
	data := createTestFont()
	r := bytes.NewReader(data)

	fontFile, err := truetype.ParseFontFile(r)
	require.NoError(t, err)

	// No loca/glyf tables parsed
	_, err = fontFile.GetGlyphData(0)
	assert.Error(t, err)
}

func TestNewFontFromBytes(t *testing.T) {
	data := createTestFont()

	font, err := truetype.NewFontFromBytes(data)
	require.NoError(t, err)
	assert.NotNil(t, font)
}

func TestNewFontFromBytes_InvalidData(t *testing.T) {
	data := []byte{0xFF, 0xFF, 0xFF, 0xFF} // Invalid SFNT version

	font, err := truetype.NewFontFromBytes(data)
	assert.Error(t, err)
	assert.Nil(t, font)
}

func TestFont_CharCodeToGlyph(t *testing.T) {
	data := createTestFont()

	font, err := truetype.NewFontFromBytes(data)
	require.NoError(t, err)

	// Without cmap, character mapping won't work
	glyph, err := font.CharCodeToGlyph(65)
	assert.Error(t, err) // No cmap, so glyph not found
	assert.Equal(t, uint32(0), glyph)
}

func TestFont_Name(t *testing.T) {
	data := createTestFont()

	font, err := truetype.NewFontFromBytes(data)
	require.NoError(t, err)
	assert.NotNil(t, font.Name())
}

func TestFont_UnitsPerEm(t *testing.T) {
	data := createTestFont()

	font, err := truetype.NewFontFromBytes(data)
	require.NoError(t, err)

	assert.Equal(t, uint16(1024), font.UnitsPerEm())
}

func TestFont_GetBoundingBox(t *testing.T) {
	data := createTestFont()

	font, err := truetype.NewFontFromBytes(data)
	require.NoError(t, err)

	// Get font-wide bounding box
	xMin, yMin, xMax, yMax := font.GetBoundingBox()
	assert.Equal(t, float64(0), xMin)
	assert.Equal(t, float64(0), yMin)
	assert.Equal(t, float64(16), xMax)
	assert.Equal(t, float64(16), yMax)
}

func TestFont_GetGlyphBoundingBox(t *testing.T) {
	data := createTestFont()

	font, err := truetype.NewFontFromBytes(data)
	require.NoError(t, err)

	// Try to get glyph bounding box (will fail without glyf table)
	_, _, _, _, err = font.GetGlyphBoundingBox(0)
	assert.Error(t, err)
}

func TestFont_IsCIDFont(t *testing.T) {
	data := createTestFont()

	font, err := truetype.NewFontFromBytes(data)
	require.NoError(t, err)

	assert.False(t, font.IsCIDFont())
}

func TestFont_HasGlyph(t *testing.T) {
	data := createTestFont()

	font, err := truetype.NewFontFromBytes(data)
	require.NoError(t, err)

	// Without cmap, no glyphs are available
	assert.False(t, font.HasGlyph(65))
}

func TestFont_RenderGlyph(t *testing.T) {
	data := createTestFont()

	font, err := truetype.NewFontFromBytes(data)
	require.NoError(t, err)

	// RenderGlyph is not implemented yet
	_, err = font.RenderGlyph(0, 12)
	assert.Error(t, err)
}

func TestFont_GetAdvanceWidth(t *testing.T) {
	data := createTestFont()

	font, err := truetype.NewFontFromBytes(data)
	require.NoError(t, err)

	// Without cmap, missing characters return 0 width with no error
	width, err := font.GetAdvanceWidth(65, 12)
	assert.NoError(t, err)
	assert.Equal(t, float64(0), width)
}

func TestFont_IsSymbolic(t *testing.T) {
	data := createTestFont()

	font, err := truetype.NewFontFromBytes(data)
	require.NoError(t, err)

	// No OS/2 table, so defaults to false
	assert.False(t, font.IsSymbolic())
}

func TestFont_IsBold(t *testing.T) {
	data := createTestFont()

	font, err := truetype.NewFontFromBytes(data)
	require.NoError(t, err)

	// No OS/2 table, defaults to false
	assert.False(t, font.IsBold())
}

func TestFont_IsItalic(t *testing.T) {
	data := createTestFont()

	font, err := truetype.NewFontFromBytes(data)
	require.NoError(t, err)

	// No post table, defaults to false
	assert.False(t, font.IsItalic())
}

func TestFont_GetWeight(t *testing.T) {
	data := createTestFont()

	font, err := truetype.NewFontFromBytes(data)
	require.NoError(t, err)

	// No OS/2 table, defaults to 400
	assert.Equal(t, uint16(400), font.GetWeight())
}
