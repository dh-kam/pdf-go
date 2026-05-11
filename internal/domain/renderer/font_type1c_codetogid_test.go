package renderer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type transformedGlyphRecorderFont struct {
	*glyphRenderTestFont
	calls []uint32
}

func (f *transformedGlyphRecorderFont) RenderGlyphBitmapTransformedPhased(
	glyph uint32,
	sizePt, scaleX, scaleY, phaseX, phaseY float64,
) ([]byte, int, int, int, int, error) {
	f.calls = append(f.calls, glyph)
	return []byte{0xff}, 1, 1, 0, 0, nil
}

func TestType1CCodeToGIDFontKeepsCodeKeyedGlyphSources(t *testing.T) {
	base := &glyphRenderTestFont{
		name:   "BaseType1C",
		widths: map[uint32]float64{42: 600},
		glyphs: map[uint32]string{42: "shared-target"},
	}
	source := &transformedGlyphRecorderFont{
		glyphRenderTestFont: &glyphRenderTestFont{name: "SourceType1C"},
	}
	font := &type1CCodeToGIDFont{
		base: base,
		sourceByCode: map[uint32]glyphSourceOverride{
			1: {font: source, glyph: 10},
			2: {font: source, glyph: 11},
		},
		targetByCode: map[uint32]uint32{
			1: 42,
			2: 42,
		},
		nameByCode: map[uint32]string{
			1: "source-one",
			2: "source-two",
		},
	}

	glyph1, err := font.CharCodeToGlyph(1)
	require.NoError(t, err)
	glyph2, err := font.CharCodeToGlyph(2)
	require.NoError(t, err)

	require.NotEqual(t, glyph1, glyph2)
	assert.Equal(t, "source-one", font.GlyphName(glyph1))
	assert.Equal(t, "source-two", font.GlyphName(glyph2))

	width1, err := font.GetGlyphWidth(glyph1)
	require.NoError(t, err)
	width2, err := font.GetGlyphWidth(glyph2)
	require.NoError(t, err)
	assert.Equal(t, 600.0, width1)
	assert.Equal(t, 600.0, width2)

	_, _, _, _, _, err = font.RenderGlyphBitmapTransformedPhased(glyph1, 10, 2, 3, 0.25, 0)
	require.NoError(t, err)
	_, _, _, _, _, err = font.RenderGlyphBitmapTransformedPhased(glyph2, 10, 2, 3, 0.25, 0)
	require.NoError(t, err)
	assert.Equal(t, []uint32{10, 11}, source.calls)
}
