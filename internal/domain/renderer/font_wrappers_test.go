package renderer

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestApplyFontMetricsFromDict_AfterEncodingMapUsesResolvedGlyphIDs(t *testing.T) {
	eval := NewEvaluator(nil)
	baseFont := &encodingTestFont{
		widths: map[uint32]float64{
			200: 500,
			201: 500,
		},
		names: map[uint32]string{
			200: "A",
			201: "B",
		},
	}

	encoding := entity.NewDict()
	encoding.Set(
		entity.Name("Differences"),
		entity.NewArray(
			entity.NewInteger(65),
			entity.Name("A"),
			entity.Name("B"),
		),
	)

	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Encoding"), encoding)
	fontDict.Set(entity.Name("FirstChar"), entity.NewInteger(65))
	fontDict.Set(entity.Name("LastChar"), entity.NewInteger(66))
	fontDict.Set(
		entity.Name("Widths"),
		entity.NewArray(
			entity.NewInteger(612),
			entity.NewInteger(700),
		),
	)

	wrapped := eval.applyFontEncodingFromDict(fontDict, baseFont)
	mapped := eval.applyFontMetricsFromDict(fontDict, wrapped)

	glyphA, err := mapped.CharCodeToGlyph(65)
	require.NoError(t, err)
	widthA, err := mapped.GetGlyphWidth(glyphA)
	require.NoError(t, err)
	assert.Equal(t, 612.0, widthA)

	glyphB, err := mapped.CharCodeToGlyph(66)
	require.NoError(t, err)
	widthB, err := mapped.GetGlyphWidth(glyphB)
	require.NoError(t, err)
	assert.Equal(t, 700.0, widthB)
}

func TestGlyphAdvanceUsesSimpleFontCharCodeWidthBeforeGlyphWidth(t *testing.T) {
	eval := NewEvaluator(nil)
	baseFont := &encodingTestFont{
		widths: map[uint32]float64{
			200: 700,
		},
		names: map[uint32]string{
			200: "A",
		},
	}

	encoding := entity.NewDict()
	encoding.Set(
		entity.Name("Differences"),
		entity.NewArray(
			entity.NewInteger(65),
			entity.Name("A"),
		),
	)

	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Encoding"), encoding)
	fontDict.Set(entity.Name("FirstChar"), entity.NewInteger(65))
	fontDict.Set(entity.Name("LastChar"), entity.NewInteger(65))
	fontDict.Set(entity.Name("Widths"), entity.NewArray(entity.NewInteger(612)))

	wrapped := eval.applyFontEncodingFromDict(fontDict, baseFont)
	mapped := eval.applyFontMetricsFromDict(fontDict, wrapped)
	require.NotNil(t, mapped)

	assert.Equal(t, 6.12, eval.glyphAdvance(65, mapped, 10))
}

func TestApplyFontEncodingFromDict_UsesGlyphNameLookupThroughWrappedFonts(t *testing.T) {
	eval := NewEvaluator(nil)
	baseFont := &encodingTestFont{
		widths: map[uint32]float64{
			uint32('γ'): 600,
		},
		names: map[uint32]string{
			uint32('γ'): "γ",
		},
	}
	wrapped := &widthMappedFont{
		base: baseFont,
	}

	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Subtype"), entity.Name("Type1"))

	encoded := eval.applyFontEncodingFromDict(fontDict, wrapped)
	require.NotNil(t, encoded)

	glyph, ok := encoded.(glyphIDByNameFont).GlyphIDByName("γ")
	require.True(t, ok)
	assert.Equal(t, uint32('γ'), glyph)
}

func TestWidthMappedFontGetGlyphWidth_UsesBaseWidthWhenDebugIgnoringMapped500MatchesFont(t *testing.T) {
	t.Setenv("PDF_DEBUG_IGNORE_MAPPED_WIDTH_500", "EncodingTestFont")

	font := &widthMappedFont{
		base: &encodingTestFont{
			widths: map[uint32]float64{200: 612},
			names:  map[uint32]string{200: "A"},
		},
		widths: map[uint32]float64{200: 500},
	}

	width, err := font.GetGlyphWidth(200)
	require.NoError(t, err)
	assert.Equal(t, 612.0, width)
}

func TestWidthMappedFontGetGlyphWidth_KeepsMapped500WhenDebugFontDoesNotMatch(t *testing.T) {
	t.Setenv("PDF_DEBUG_IGNORE_MAPPED_WIDTH_500", "OtherFont")

	font := &widthMappedFont{
		base: &encodingTestFont{
			widths: map[uint32]float64{200: 612},
			names:  map[uint32]string{200: "A"},
		},
		widths: map[uint32]float64{200: 500},
	}

	width, err := font.GetGlyphWidth(200)
	require.NoError(t, err)
	assert.Equal(t, 500.0, width)
}

type glyphRenderTestFont struct {
	name    string
	widths  map[uint32]float64
	paths   map[uint32]*entity.GlyphPath
	glyphs  map[uint32]string
	charMap map[uint32]uint32
	upem    uint16
}

func (f *glyphRenderTestFont) CharCodeToGlyph(code uint32) (uint32, error) {
	if glyph, ok := f.charMap[code]; ok {
		return glyph, nil
	}
	return 0, errors.New("missing glyph")
}

func (f *glyphRenderTestFont) GlyphName(glyph uint32) string {
	return f.glyphs[glyph]
}

func (f *glyphRenderTestFont) GetGlyphWidth(glyph uint32) (float64, error) {
	width, ok := f.widths[glyph]
	if !ok {
		return 0, errors.New("missing width")
	}
	return width, nil
}

func (f *glyphRenderTestFont) GetBoundingBox() (float64, float64, float64, float64) {
	return 0, 0, 1000, 1000
}

func (f *glyphRenderTestFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	path, ok := f.paths[glyph]
	if !ok {
		return nil, errors.New("missing path")
	}
	return path, nil
}

func (f *glyphRenderTestFont) IsCIDFont() bool {
	return false
}

func (f *glyphRenderTestFont) IsSymbolic() bool {
	return false
}

func (f *glyphRenderTestFont) UnitsPerEm() uint16 {
	if f.upem > 0 {
		return f.upem
	}
	return 1000
}

func (f *glyphRenderTestFont) Name() string {
	return f.name
}

func TestApplyFontMetricsFromDict_CIDWUsesPDFThousandUnitAdvances(t *testing.T) {
	eval := NewEvaluator(nil)
	baseFont := &glyphRenderTestFont{
		name: "CIDIdentity",
		widths: map[uint32]float64{
			914:  1205,
			1012: 500,
		},
		charMap: map[uint32]uint32{
			914:  914,
			1012: 1012,
		},
		upem: 2048,
	}

	fontDict := entity.NewDict()
	fontDict.Set(
		entity.Name("W"),
		entity.NewArray(
			entity.NewInteger(914),
			entity.NewArray(entity.NewInteger(244)),
			entity.NewInteger(1012),
			entity.NewInteger(1012),
			entity.NewInteger(244),
		),
	)

	mapped := eval.applyFontMetricsFromDict(fontDict, baseFont)
	require.NotNil(t, mapped)

	width, err := mapped.GetGlyphWidth(914)
	require.NoError(t, err)
	assert.InDelta(t, 244.0*2048.0/1000.0, width, 1e-9)
	assert.InDelta(t, 6.1, width/float64(mapped.UnitsPerEm())*25.0, 1e-9)

	rangeWidth, err := mapped.GetGlyphWidth(1012)
	require.NoError(t, err)
	assert.InDelta(t, 6.1, rangeWidth/float64(mapped.UnitsPerEm())*25.0, 1e-9)
}

func TestApplyFontMetricsFromDict_CIDWKeysWidthsAfterIdentityWrapper(t *testing.T) {
	eval := NewEvaluator(nil)
	baseFont := &glyphRenderTestFont{
		name: "CIDIdentityEmbedded",
		widths: map[uint32]float64{
			914: 1200,
			915: 333,
			999: 1200,
		},
		charMap: map[uint32]uint32{
			914: 999, // Embedded cmap may not use the CID as the glyph ID.
		},
		upem: 1000,
	}
	font := &cidIdentityFont{base: baseFont}

	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("DW"), entity.NewInteger(1000))
	fontDict.Set(
		entity.Name("W"),
		entity.NewArray(
			entity.NewInteger(914),
			entity.NewArray(entity.NewInteger(244)),
		),
	)

	mapped := eval.applyFontMetricsFromDict(fontDict, font)
	require.NotNil(t, mapped)

	glyph, err := mapped.CharCodeToGlyph(914)
	require.NoError(t, err)
	assert.Equal(t, uint32(914), glyph)

	width, err := mapped.GetGlyphWidth(glyph)
	require.NoError(t, err)
	assert.Equal(t, 244.0, width)

	defaultWidth, err := mapped.GetGlyphWidth(915)
	require.NoError(t, err)
	assert.Equal(t, 1000.0, defaultWidth)
}

func TestApplyFontMetricsFromDict_CIDHonorsDWWithoutWArray(t *testing.T) {
	eval := NewEvaluator(nil)
	font := &cidIdentityFont{
		base: &glyphRenderTestFont{
			name:   "CIDIdentityDefaultWidth",
			widths: map[uint32]float64{7: 333},
			upem:   1000,
		},
	}

	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("DW"), entity.NewInteger(750))

	mapped := eval.applyFontMetricsFromDict(fontDict, font)
	require.NotNil(t, mapped)

	width, err := mapped.GetGlyphWidth(7)
	require.NoError(t, err)
	assert.Equal(t, 750.0, width)
}

func TestCIDIdentityFontCanMapThroughToUnicodeCMap(t *testing.T) {
	baseFont := &glyphRenderTestFont{
		name: "CIDIdentityUnicode",
		charMap: map[uint32]uint32{
			uint32('K'): 46,
		},
	}

	direct := &cidIdentityFont{
		base:      baseFont,
		toUnicode: map[uint32]rune{0x002e: 'K'},
	}
	glyph, err := direct.CharCodeToGlyph(0x002e)
	require.NoError(t, err)
	assert.Equal(t, uint32(0x002e), glyph)

	viaUnicode := &cidIdentityFont{
		base:                baseFont,
		toUnicode:           map[uint32]rune{0x002e: 'K'},
		preferToUnicodeCMap: true,
	}
	glyph, err = viaUnicode.CharCodeToGlyph(0x002e)
	require.NoError(t, err)
	assert.Equal(t, uint32(46), glyph)
}

func TestShouldUseToUnicodeCMapForCIDIdentity_AutoOnlyLatinFallback(t *testing.T) {
	t.Setenv("PDF_DEBUG_CID_IDENTITY_TOUNICODE_CMAP", "auto")

	latinMap := map[uint32]rune{
		0x002c: 'I',
		0x0057: 't',
		0x00b1: 0x2013,
	}
	assert.True(t, shouldUseToUnicodeCMapForCIDIdentity("YZIAWQ+ArialMT", latinMap, []byte("bad truetype"), nil))
	assert.True(t, shouldUseToUnicodeCMapForCIDIdentity("ArialMT", latinMap, nil, assert.AnError))

	cjkMap := map[uint32]rune{
		0x95b5: 0xac00,
	}
	assert.False(t, shouldUseToUnicodeCMapForCIDIdentity("QOXWJG+ArialUnicodeMS", cjkMap, []byte("bad truetype"), nil))
	assert.False(t, shouldUseToUnicodeCMapForCIDIdentity("BCDIEE+MalgunGothic", cjkMap, []byte("bad truetype"), nil))
}

func TestShouldUseToUnicodeCMapForCIDIdentity_DefaultAutoCanBeDisabledAndForced(t *testing.T) {
	latinMap := map[uint32]rune{0x002c: 'I'}

	assert.True(t, shouldUseToUnicodeCMapForCIDIdentity("ArialMT", latinMap, nil, assert.AnError))
	assert.False(t, shouldUseToUnicodeCMapForCIDIdentity("QOXWJG+ArialUnicodeMS", map[uint32]rune{0x95b5: 0xac00}, nil, nil))

	t.Setenv("PDF_DEBUG_CID_IDENTITY_TOUNICODE_CMAP", "0")
	assert.False(t, shouldUseToUnicodeCMapForCIDIdentity("ArialMT", latinMap, nil, assert.AnError))

	t.Setenv("PDF_DEBUG_CID_IDENTITY_TOUNICODE_CMAP", "1")
	assert.True(t, shouldUseToUnicodeCMapForCIDIdentity("QOXWJG+ArialUnicodeMS", map[uint32]rune{0x95b5: 0xac00}, nil, nil))
}

func TestApplyFontMetricsFromDict_CIDWAfterToUnicodeWrapperUsesRenderedGlyph(t *testing.T) {
	eval := NewEvaluator(nil)
	baseFont := &glyphRenderTestFont{
		name: "CIDIdentityUnicode",
		widths: map[uint32]float64{
			46: 500,
		},
		charMap: map[uint32]uint32{
			uint32('K'): 46,
		},
		upem: 1000,
	}
	font := &cidIdentityFont{
		base:                baseFont,
		toUnicode:           map[uint32]rune{0x002e: 'K'},
		preferToUnicodeCMap: true,
	}

	fontDict := entity.NewDict()
	fontDict.Set(
		entity.Name("W"),
		entity.NewArray(
			entity.NewInteger(0x002e),
			entity.NewArray(entity.NewInteger(722)),
		),
	)

	mapped := eval.applyFontMetricsFromDict(fontDict, font)
	require.NotNil(t, mapped)

	glyph, err := mapped.CharCodeToGlyph(0x002e)
	require.NoError(t, err)
	assert.Equal(t, uint32(46), glyph)

	width, err := mapped.GetGlyphWidth(glyph)
	require.NoError(t, err)
	assert.Equal(t, 722.0, width)
}

func TestGlyphSourceOverrideFont_RenderGlyphUsesOverridePathButKeepsBaseWidth(t *testing.T) {
	baseGlyph := uint32(47)
	basePath := &entity.GlyphPath{
		Commands: []entity.PathCommand{
			&entity.PathMoveTo{X: 0, Y: 0},
			&entity.PathLineTo{X: 10, Y: 10},
		},
		Bounds: [4]float64{0, 0, 10, 10},
	}
	overridePath := &entity.GlyphPath{
		Commands: []entity.PathCommand{
			&entity.PathMoveTo{X: 0, Y: 0},
			&entity.PathLineTo{X: 20, Y: 30},
		},
		Bounds: [4]float64{0, 0, 20, 30},
	}

	base := &glyphRenderTestFont{
		name:   "BaseFont",
		widths: map[uint32]float64{baseGlyph: 444.0},
		paths:  map[uint32]*entity.GlyphPath{baseGlyph: basePath},
		glyphs: map[uint32]string{baseGlyph: "/"},
		charMap: map[uint32]uint32{
			47: baseGlyph,
		},
	}
	override := &glyphRenderTestFont{
		name:   "OverrideFont",
		widths: map[uint32]float64{101: 999.0},
		paths:  map[uint32]*entity.GlyphPath{101: overridePath},
		glyphs: map[uint32]string{101: "/"},
		charMap: map[uint32]uint32{
			47: 101,
		},
	}

	font := &glyphSourceOverrideFont{
		base: base,
		overrides: map[uint32]glyphSourceOverride{
			baseGlyph: {
				font:  override,
				glyph: 101,
			},
		},
	}

	path, err := font.RenderGlyph(baseGlyph, 1000)
	require.NoError(t, err)
	require.NotNil(t, path)
	assert.Same(t, overridePath, path)

	width, err := font.GetGlyphWidth(baseGlyph)
	require.NoError(t, err)
	assert.Equal(t, 444.0, width)
}
