package standard

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/errors"
)

func TestGetStandard14Names(t *testing.T) {
	names := GetStandard14Names()
	assert.Len(t, names, 14)

	expected := []string{
		"Times-Roman",
		"Times-Bold",
		"Times-Italic",
		"Times-BoldItalic",
		"Helvetica",
		"Helvetica-Bold",
		"Helvetica-Oblique",
		"Helvetica-BoldOblique",
		"Courier",
		"Courier-Bold",
		"Courier-Oblique",
		"Courier-BoldOblique",
		"Symbol",
		"ZapfDingbats",
	}
	assert.ElementsMatch(t, expected, names)
}

func TestGetFontKnownAndUnknown(t *testing.T) {
	font, ok := GetFont("Helvetica")
	require.True(t, ok)
	assert.NotNil(t, font)
	assert.Equal(t, "Helvetica", font.Name())

	_, ok = GetFont("DoesNotExist")
	assert.False(t, ok)
}

func TestStandardFontProperties(t *testing.T) {
	font, ok := GetFont("Times-Roman")
	require.True(t, ok)
	assert.Equal(t, "Times New Roman", font.Name())
	assert.False(t, font.IsCIDFont())
	assert.False(t, font.IsSymbolic())
	assert.Equal(t, uint16(2048), font.UnitsPerEm())
}

func TestStandardFontBoundingBoxesArePopulated(t *testing.T) {
	for _, name := range GetStandard14Names() {
		font, ok := GetFont(name)
		require.True(t, ok)
		xMin, yMin, xMax, yMax := font.GetBoundingBox()
		assert.Greater(t, xMax-xMin, 0.0, name)
		assert.Greater(t, yMax-yMin, 0.0, name)
	}
}

func TestStandardFontHelveticaBBoxTriggersPopplerLargeGlyphPhaseRule(t *testing.T) {
	font, ok := GetFont("Helvetica")
	require.True(t, ok)

	_, yMin, _, yMax := font.GetBoundingBox()
	cacheHeight := math.Abs(yMax-yMin)/float64(font.UnitsPerEm())*24*(150.0/72.0) + 3

	assert.Greater(t, cacheHeight, 50.0)
}

func TestReadURWAFMFontBBoxFallsBackToPopplerBuiltinBBox(t *testing.T) {
	bbox := readURWAFMFontBBox("missing.afm", [4]float64{-166, -225, 1000, 931})

	assert.InDelta(t, -166*standardWidthScale, bbox.XMin, 1e-9)
	assert.InDelta(t, -225*standardWidthScale, bbox.YMin, 1e-9)
	assert.InDelta(t, 1000*standardWidthScale, bbox.XMax, 1e-9)
	assert.InDelta(t, 931*standardWidthScale, bbox.YMax, 1e-9)
}

func TestStandardFontSymbolicAndWidths(t *testing.T) {
	font, ok := GetFont("Symbol")
	require.True(t, ok)
	assert.True(t, font.IsSymbolic())

	_, err := font.GetGlyphWidth(255)
	assert.NoError(t, err)

	_, err = font.GetGlyphWidth(999)
	assert.Error(t, err)
	assert.IsType(t, &errors.OutOfRangeError{}, err)
}

func TestStandardFontRenderGlyph(t *testing.T) {
	font, ok := GetFont("Helvetica-Bold")
	require.True(t, ok)

	path, err := font.RenderGlyph(65, 12.0)
	require.NoError(t, err)
	require.NotNil(t, path)
	assert.NotEmpty(t, path.Commands)
}

func TestStandardFontGlyphIDByNameAndUnicodeRender(t *testing.T) {
	font, ok := GetFont("Times-Roman")
	require.True(t, ok)

	glyph, found := font.GlyphIDByName("gamma")
	require.True(t, found)
	assert.Equal(t, uint32('γ'), glyph)

	width, err := font.GetGlyphWidth(glyph)
	require.NoError(t, err)
	assert.Greater(t, width, 0.0)

	path, err := font.RenderGlyph(glyph, 12.0)
	require.NoError(t, err)
	require.NotNil(t, path)
	assert.NotEmpty(t, path.Commands)
}
