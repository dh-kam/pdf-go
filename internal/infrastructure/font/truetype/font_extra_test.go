package truetype

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func minimalTTFHeaderBytes() []byte {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, uint32(0x00010000))
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))
	_ = binary.Write(&buf, binary.BigEndian, uint16(0))
	return buf.Bytes()
}

func TestNewFontConstructors(t *testing.T) {
	_, err := NewFontFromBytes([]byte{0x00})
	require.Error(t, err)

	font, err := NewFontFromBytes(minimalTTFHeaderBytes())
	require.NoError(t, err)
	require.NotNil(t, font)
	assert.Equal(t, "", font.Name())

	font, err = NewFontFromReader(bytes.NewReader(minimalTTFHeaderBytes()))
	require.NoError(t, err)
	require.NotNil(t, font)

	tmpDir := t.TempDir()
	fontPath := filepath.Join(tmpDir, "minimal.ttf")
	require.NoError(t, os.WriteFile(fontPath, minimalTTFHeaderBytes(), 0o644))
	font, err = NewFont(fontPath)
	require.NoError(t, err)
	require.NotNil(t, font)

	_, err = NewFont(filepath.Join(tmpDir, "missing.ttf"))
	require.Error(t, err)
}

func TestFontCoreMethods(t *testing.T) {
	file := &FontFile{
		Head: &HeadTable{
			UnitsPerEm: 1000,
			MinX:       -20,
			MinY:       -30,
			MaxX:       500,
			MaxY:       700,
		},
		OS2: &OS2Table{FsType: 1, WeightClass: 700},
		Post: &PostTable{
			ItalicAngle: 10,
		},
		Cmap: &CmapTable{
			Encodings: []CmapEncoding{
				{
					PlatformID: 3,
					EncodingID: 4,
					Mapping: map[uint16]uint16{
						65: 1,
					},
				},
			},
		},
		Hmtx: &HmtxTable{
			Metrics: []HorizontalMetric{
				{AdvanceWidth: 400, LeftSideBearing: 0},
				{AdvanceWidth: 500, LeftSideBearing: 0},
			},
		},
		Glyf: &GlyfTable{
			Glyphs: []Glyph{
				{},
				{XMin: -1, YMin: -2, XMax: 3, YMax: 4},
			},
		},
		Name: &NameTable{
			Names: map[uint16]string{1: "TestFamily"},
		},
	}
	font := &Font{
		file: file,
		name: "TestFamily",
		data: []byte{1, 2, 3},
	}

	glyphID, err := font.CharCodeToGlyph(65)
	require.NoError(t, err)
	assert.Equal(t, uint32(1), glyphID)

	_, err = font.CharCodeToGlyph(77)
	require.Error(t, err)

	name := font.GlyphName(12)
	assert.Equal(t, "glyph12", name)

	w, err := font.GetGlyphWidth(1)
	require.NoError(t, err)
	assert.Equal(t, 500.0, w)

	x1, y1, x2, y2 := font.GetBoundingBox()
	assert.Equal(t, -20.0, x1)
	assert.Equal(t, -30.0, y1)
	assert.Equal(t, 500.0, x2)
	assert.Equal(t, 700.0, y2)

	x1, y1, x2, y2, err = font.GetGlyphBoundingBox(1)
	require.NoError(t, err)
	assert.Equal(t, -1.0, x1)
	assert.Equal(t, -2.0, y1)
	assert.Equal(t, 3.0, x2)
	assert.Equal(t, 4.0, y2)

	assert.False(t, font.IsCIDFont())
	assert.True(t, font.IsSymbolic())
	assert.Equal(t, uint16(1000), font.UnitsPerEm())
	assert.Equal(t, "TestFamily", font.Name())

	dataCopy := font.FontData()
	require.Equal(t, []byte{1, 2, 3}, dataCopy)
	dataCopy[0] = 9
	assert.Equal(t, byte(1), font.data[0])

	adv, err := font.GetAdvanceWidth(65, 10)
	require.NoError(t, err)
	assert.Equal(t, 5.0, adv)

	adv, err = font.GetAdvanceWidth(99, 10)
	require.NoError(t, err)
	assert.Equal(t, 0.0, adv)

	assert.True(t, font.HasGlyph(65))
	assert.False(t, font.HasGlyph(99))
	assert.Equal(t, uint16(700), font.GetWeight())
	assert.True(t, font.IsBold())
	assert.True(t, font.IsItalic())
}

func TestFontCoreMethods_ErrorFallbackPaths(t *testing.T) {
	emptyFont := &Font{file: &FontFile{}}

	x1, y1, x2, y2 := emptyFont.GetBoundingBox()
	assert.Equal(t, 0.0, x1)
	assert.Equal(t, 0.0, y1)
	assert.Equal(t, 0.0, x2)
	assert.Equal(t, 0.0, y2)

	assert.False(t, emptyFont.IsSymbolic())
	assert.Equal(t, uint16(400), emptyFont.GetWeight())
	assert.False(t, emptyFont.IsBold())
	assert.False(t, emptyFont.IsItalic())

	// upem == 0 fallback to 1000
	font := &Font{
		file: &FontFile{
			Head: &HeadTable{UnitsPerEm: 0},
			Cmap: &CmapTable{
				Encodings: []CmapEncoding{{Mapping: map[uint16]uint16{10: 0}}},
			},
			Hmtx: &HmtxTable{
				Metrics: []HorizontalMetric{{AdvanceWidth: 250, LeftSideBearing: 0}},
			},
		},
	}
	adv, err := font.GetAdvanceWidth(10, 20)
	require.NoError(t, err)
	assert.Equal(t, 5.0, adv)

	fontErr := &Font{
		file: &FontFile{
			Cmap: &CmapTable{Encodings: []CmapEncoding{{Mapping: map[uint16]uint16{10: 0}}}},
		},
	}
	_, err = fontErr.GetAdvanceWidth(10, 20)
	require.Error(t, err)
}

func TestFontScaleAndConvertHelpers(t *testing.T) {
	font := &Font{}

	path := NewGlyphPath()
	path.MoveTo(1, 2)
	path.LineTo(3, 4)
	path.QuadTo(5, 6, 7, 8)
	path.ClosePath()

	scaled := font.scalePath(path, 2)
	require.NotNil(t, scaled)
	elems := scaled.Elements()
	require.Len(t, elems, 4)
	assert.Equal(t, 2.0, elems[0].X)
	assert.Equal(t, 4.0, elems[0].Y)
	assert.Equal(t, 6.0, elems[1].X)
	assert.Equal(t, 8.0, elems[1].Y)
	assert.Equal(t, 10.0, elems[2].CX)
	assert.Equal(t, 16.0, elems[2].Y)

	entityPath := font.convertToEntityPath(scaled)
	require.NotNil(t, entityPath)
	require.Len(t, entityPath.Commands, 4)
	assert.Equal(t, [4]float64{2, -16, 14, -4}, entityPath.Bounds)

	var _ entity.PathCommand = entityPath.Commands[0]
	assert.Nil(t, font.convertToEntityPath(nil))
}
