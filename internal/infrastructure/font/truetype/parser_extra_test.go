package truetype

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFontFileInvalidVersion(t *testing.T) {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, uint32(0xDEADBEEF))
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))

	_, err := ParseFontFile(bytes.NewReader(buf.Bytes()))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported SFNT version")
}

func TestParseFontFileMinimalValidHeader(t *testing.T) {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, uint32(0x00010000))
	_ = binary.Write(&buf, binary.BigEndian, uint16(0)) // numTables
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))

	font, err := ParseFontFile(bytes.NewReader(buf.Bytes()))
	require.NoError(t, err)
	require.NotNil(t, font)
	assert.Equal(t, uint32(0x00010000), font.SFNTVersion)
	assert.Empty(t, font.Tables)
}

func TestParseCoreTables(t *testing.T) {
	font := &FontFile{}

	{
		var buf bytes.Buffer
		_ = binary.Write(&buf, binary.BigEndian, uint32(0x00010000)) // version (skip)
		_ = binary.Write(&buf, binary.BigEndian, uint32(0x00020000)) // revision
		_ = binary.Write(&buf, binary.BigEndian, uint32(3))
		_ = binary.Write(&buf, binary.BigEndian, uint32(4))
		_ = binary.Write(&buf, binary.BigEndian, uint16(5))
		_ = binary.Write(&buf, binary.BigEndian, uint16(1000))
		_ = binary.Write(&buf, binary.BigEndian, uint32(10))
		_ = binary.Write(&buf, binary.BigEndian, uint32(11))
		_ = binary.Write(&buf, binary.BigEndian, uint32(12))
		_ = binary.Write(&buf, binary.BigEndian, uint32(13))
		_ = binary.Write(&buf, binary.BigEndian, int16(-1)) // xMin → MinX
		_ = binary.Write(&buf, binary.BigEndian, int16(-2)) // yMin → MinY
		_ = binary.Write(&buf, binary.BigEndian, int16(3))  // xMax → MaxX
		_ = binary.Write(&buf, binary.BigEndian, int16(4))  // yMax → MaxY
		_ = binary.Write(&buf, binary.BigEndian, uint16(1)) // macStyle
		_ = binary.Write(&buf, binary.BigEndian, uint16(0)) // lowestRecPPEM (skipped)
		_ = binary.Write(&buf, binary.BigEndian, int16(0))  // fontDirectionHint (skipped)
		_ = binary.Write(&buf, binary.BigEndian, uint16(0)) // indexToLocFormat
		_ = binary.Write(&buf, binary.BigEndian, uint16(0)) // glyphDataFormat
		err := font.parseHeadTable(bytes.NewReader(buf.Bytes()), TableEntry{})
		require.NoError(t, err)
		require.NotNil(t, font.Head)
		assert.Equal(t, uint16(1000), font.Head.UnitsPerEm)
		assert.Equal(t, int16(-1), font.Head.MinX)
	}

	{
		var buf bytes.Buffer
		_ = binary.Write(&buf, binary.BigEndian, uint32(1)) // TableVersionNumber
		_ = binary.Write(&buf, binary.BigEndian, int16(2))  // Ascender
		_ = binary.Write(&buf, binary.BigEndian, int16(3))  // Descender
		_ = binary.Write(&buf, binary.BigEndian, int16(4))  // LineGap
		_ = binary.Write(&buf, binary.BigEndian, uint16(5)) // AdvanceWidthMax
		_ = binary.Write(&buf, binary.BigEndian, int16(6))  // MinLeftSideBearing
		_ = binary.Write(&buf, binary.BigEndian, int16(7))  // MinRightSideBearing
		_ = binary.Write(&buf, binary.BigEndian, int16(8))  // XMaxExtent
		_ = binary.Write(&buf, binary.BigEndian, int16(9))  // CaretSlopeRise
		_ = binary.Write(&buf, binary.BigEndian, int16(10)) // CaretSlopeRun
		_ = binary.Write(&buf, binary.BigEndian, int16(11)) // CaretOffset
		_ = binary.Write(&buf, binary.BigEndian, int16(12)) // Reserved0
		_ = binary.Write(&buf, binary.BigEndian, int16(13)) // Reserved1
		_ = binary.Write(&buf, binary.BigEndian, int16(0))  // Reserved2
		_ = binary.Write(&buf, binary.BigEndian, int16(0))  // Reserved3
		_ = binary.Write(&buf, binary.BigEndian, int16(0))  // MetricDataFormat
		_ = binary.Write(&buf, binary.BigEndian, uint16(2)) // NumberOfHMetrics
		err := font.parseHheaTable(bytes.NewReader(buf.Bytes()), TableEntry{})
		require.NoError(t, err)
		require.NotNil(t, font.Hhea)
		assert.Equal(t, uint16(2), font.Hhea.NumberOfHMetrics)
	}

	{
		var buf bytes.Buffer
		_ = binary.Write(&buf, binary.BigEndian, uint32(0x00010000))
		_ = binary.Write(&buf, binary.BigEndian, uint16(3))
		for i := 0; i < 13; i++ {
			_ = binary.Write(&buf, binary.BigEndian, uint16(i+1))
		}
		err := font.parseMaxpTable(bytes.NewReader(buf.Bytes()), TableEntry{})
		require.NoError(t, err)
		require.NotNil(t, font.Maxp)
		assert.Equal(t, uint16(3), font.Maxp.NumGlyphs)
	}
}

func TestParseLocaTableShortAndLong(t *testing.T) {
	// Error branches.
	{
		font := &FontFile{}
		err := font.parseLocaTable(bytes.NewReader(nil), TableEntry{})
		require.Error(t, err)
	}
	{
		font := &FontFile{Head: &HeadTable{}}
		err := font.parseLocaTable(bytes.NewReader(nil), TableEntry{})
		require.Error(t, err)
	}

	// Short format.
	{
		font := &FontFile{
			Head: &HeadTable{IndexToLocFormat: 0},
			Maxp: &MaxpTable{NumGlyphs: 2},
		}
		var buf bytes.Buffer
		_ = binary.Write(&buf, binary.BigEndian, uint16(1))
		_ = binary.Write(&buf, binary.BigEndian, uint16(2))
		_ = binary.Write(&buf, binary.BigEndian, uint16(3))
		err := font.parseLocaTable(bytes.NewReader(buf.Bytes()), TableEntry{})
		require.NoError(t, err)
		assert.Equal(t, []uint32{2, 4, 6}, font.Loca.Offsets)
	}

	// Long format.
	{
		font := &FontFile{
			Head: &HeadTable{IndexToLocFormat: 1},
			Maxp: &MaxpTable{NumGlyphs: 2},
		}
		var buf bytes.Buffer
		_ = binary.Write(&buf, binary.BigEndian, uint32(0))
		_ = binary.Write(&buf, binary.BigEndian, uint32(10))
		_ = binary.Write(&buf, binary.BigEndian, uint32(20))
		err := font.parseLocaTable(bytes.NewReader(buf.Bytes()), TableEntry{})
		require.NoError(t, err)
		assert.Equal(t, []uint32{0, 10, 20}, font.Loca.Offsets)
	}
}

func TestParseGlyfTableAndHelpers(t *testing.T) {
	font := &FontFile{
		Maxp: &MaxpTable{NumGlyphs: 2},
		Loca: &LocaTable{Offsets: []uint32{0, 0, 12}},
	}

	var glyf bytes.Buffer
	_ = binary.Write(&glyf, binary.BigEndian, int16(1))  // contours
	_ = binary.Write(&glyf, binary.BigEndian, int16(-2)) // xmin
	_ = binary.Write(&glyf, binary.BigEndian, int16(-3)) // ymin
	_ = binary.Write(&glyf, binary.BigEndian, int16(4))  // xmax
	_ = binary.Write(&glyf, binary.BigEndian, int16(5))  // ymax
	glyf.Write([]byte{0xAA, 0xBB})

	err := font.parseGlyfTable(bytes.NewReader(glyf.Bytes()), TableEntry{Offset: 0})
	require.NoError(t, err)
	require.NotNil(t, font.Glyf)
	require.Len(t, font.Glyf.Glyphs, 2)
	assert.Equal(t, int16(0), font.Glyf.Glyphs[0].NumberOfContours) // empty glyph
	assert.Equal(t, int16(1), font.Glyf.Glyphs[1].NumberOfContours)
	assert.Equal(t, []byte{0xAA, 0xBB}, font.Glyf.Glyphs[1].Instructions)

	_, err = font.GetGlyphData(5)
	require.Error(t, err)
	g, err := font.GetGlyphData(1)
	require.NoError(t, err)
	assert.Equal(t, int16(-2), g.XMin)

	xMin, yMin, xMax, yMax, err := font.GetGlyphBoundingBox(1)
	require.NoError(t, err)
	assert.Equal(t, int16(-2), xMin)
	assert.Equal(t, int16(-3), yMin)
	assert.Equal(t, int16(4), xMax)
	assert.Equal(t, int16(5), yMax)
}

func TestParseCmapTableFormat4(t *testing.T) {
	font := &FontFile{}

	var buf bytes.Buffer
	// cmap header
	_ = binary.Write(&buf, binary.BigEndian, uint16(0)) // version
	_ = binary.Write(&buf, binary.BigEndian, uint16(1)) // numTables
	_ = binary.Write(&buf, binary.BigEndian, uint16(3)) // platform
	_ = binary.Write(&buf, binary.BigEndian, uint16(1)) // encoding (Windows Unicode BMP)
	_ = binary.Write(&buf, binary.BigEndian, uint32(12))

	// format 4 subtable at offset 12
	_ = binary.Write(&buf, binary.BigEndian, uint16(4))  // format
	_ = binary.Write(&buf, binary.BigEndian, uint16(24)) // length
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))  // language
	_ = binary.Write(&buf, binary.BigEndian, uint16(2))  // segCountX2 => 1 segment
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))  // searchRange
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))  // entrySelector
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))  // rangeShift
	_ = binary.Write(&buf, binary.BigEndian, uint16(66)) // endCode
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))  // reservedPad
	_ = binary.Write(&buf, binary.BigEndian, uint16(65)) // startCode
	_ = binary.Write(&buf, binary.BigEndian, int16(1))   // idDelta
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))  // idRangeOffset

	err := font.parseCmapTable(bytes.NewReader(buf.Bytes()), TableEntry{Offset: 0})
	require.NoError(t, err)
	require.NotNil(t, font.Cmap)
	require.Len(t, font.Cmap.Encodings, 1)
	glyph, ok := font.CharCodeToGlyph(65)
	require.True(t, ok)
	assert.Equal(t, uint16(66), glyph)
}

func TestParseCmapTableFormat6MacRoman(t *testing.T) {
	font := &FontFile{}

	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, uint16(0)) // version
	_ = binary.Write(&buf, binary.BigEndian, uint16(1)) // numTables
	_ = binary.Write(&buf, binary.BigEndian, uint16(1)) // platform (Macintosh)
	_ = binary.Write(&buf, binary.BigEndian, uint16(0)) // encoding (Roman)
	_ = binary.Write(&buf, binary.BigEndian, uint32(12))

	// format 6 subtable at offset 12: firstCode=32, entryCount=4.
	_ = binary.Write(&buf, binary.BigEndian, uint16(6))  // format
	_ = binary.Write(&buf, binary.BigEndian, uint16(18)) // length
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))  // language
	_ = binary.Write(&buf, binary.BigEndian, uint16(32)) // firstCode
	_ = binary.Write(&buf, binary.BigEndian, uint16(4))  // entryCount
	_ = binary.Write(&buf, binary.BigEndian, uint16(7))  // code 32
	_ = binary.Write(&buf, binary.BigEndian, uint16(8))  // code 33
	_ = binary.Write(&buf, binary.BigEndian, uint16(9))  // code 34
	_ = binary.Write(&buf, binary.BigEndian, uint16(10)) // code 35

	err := font.parseCmapTable(bytes.NewReader(buf.Bytes()), TableEntry{Offset: 0})
	require.NoError(t, err)
	require.NotNil(t, font.Cmap)
	require.Len(t, font.Cmap.Encodings, 1)
	glyph, ok := font.CharCodeToGlyph(35)
	require.True(t, ok)
	assert.Equal(t, uint16(10), glyph)
}

func TestParseHmtxNamePostOS2AndFontMethods(t *testing.T) {
	font := &FontFile{
		Maxp: &MaxpTable{NumGlyphs: 3},
		Hhea: &HheaTable{NumberOfHMetrics: 2},
		Head: &HeadTable{UnitsPerEm: 2048},
	}

	{
		var hmtx bytes.Buffer
		_ = binary.Write(&hmtx, binary.BigEndian, uint16(500))
		_ = binary.Write(&hmtx, binary.BigEndian, int16(10))
		_ = binary.Write(&hmtx, binary.BigEndian, uint16(600))
		_ = binary.Write(&hmtx, binary.BigEndian, int16(20))
		_ = binary.Write(&hmtx, binary.BigEndian, int16(30)) // extra LSB

		err := font.parseHmtxTable(bytes.NewReader(hmtx.Bytes()), TableEntry{})
		require.NoError(t, err)
		require.NotNil(t, font.Hmtx)
		require.Len(t, font.Hmtx.Metrics, 2)
		require.Len(t, font.Hmtx.LeftSideBearings, 1)

		width0, err := font.GetGlyphWidth(0)
		require.NoError(t, err)
		assert.Equal(t, uint16(500), width0)
		width2, err := font.GetGlyphWidth(2) // fallback to last metric
		require.NoError(t, err)
		assert.Equal(t, uint16(600), width2)
	}

	{
		var name bytes.Buffer
		_ = binary.Write(&name, binary.BigEndian, uint16(0)) // format
		_ = binary.Write(&name, binary.BigEndian, uint16(1)) // count
		_ = binary.Write(&name, binary.BigEndian, uint16(18))
		_ = binary.Write(&name, binary.BigEndian, uint16(1)) // platform
		_ = binary.Write(&name, binary.BigEndian, uint16(0)) // encoding
		_ = binary.Write(&name, binary.BigEndian, uint16(0)) // language
		_ = binary.Write(&name, binary.BigEndian, uint16(1)) // nameID
		_ = binary.Write(&name, binary.BigEndian, uint16(4)) // length
		_ = binary.Write(&name, binary.BigEndian, uint16(0)) // offset
		name.WriteString("Test")

		err := font.parseNameTable(bytes.NewReader(name.Bytes()), TableEntry{Offset: 0})
		require.NoError(t, err)
		assert.Equal(t, "Test", font.GetFontName())
	}

	{
		var post bytes.Buffer
		_ = binary.Write(&post, binary.BigEndian, uint32(0x00020000)) // format2
		_ = binary.Write(&post, binary.BigEndian, uint32(0))
		_ = binary.Write(&post, binary.BigEndian, int16(0))
		_ = binary.Write(&post, binary.BigEndian, int16(0))
		_ = binary.Write(&post, binary.BigEndian, uint32(0))
		_ = binary.Write(&post, binary.BigEndian, uint32(0))
		_ = binary.Write(&post, binary.BigEndian, uint32(0))
		_ = binary.Write(&post, binary.BigEndian, uint16(2)) // numGlyphs
		_ = binary.Write(&post, binary.BigEndian, uint16(1))
		_ = binary.Write(&post, binary.BigEndian, uint16(2))
		err := font.parsePostTable(bytes.NewReader(post.Bytes()), TableEntry{})
		require.NoError(t, err)
		require.NotNil(t, font.Post)
		assert.Equal(t, uint32(0x00020000), font.Post.FormatType)
	}

	{
		os2Data := make([]byte, 160)
		binary.BigEndian.PutUint16(os2Data[0:2], 5) // version => run >=1, >=2, >=5 paths
		err := font.parseOS2Table(bytes.NewReader(os2Data), TableEntry{})
		require.NoError(t, err)
		require.NotNil(t, font.OS2)
		assert.Equal(t, uint16(5), font.OS2.Version)
	}

	assert.Equal(t, uint16(2048), font.UnitsPerEm())

	empty := &FontFile{}
	assert.Equal(t, uint16(1000), empty.UnitsPerEm())
	_, err := empty.GetGlyphWidth(0)
	require.Error(t, err)
	_, ok := empty.CharCodeToGlyph(65)
	assert.False(t, ok)
	assert.Equal(t, "", empty.GetFontName())
}
