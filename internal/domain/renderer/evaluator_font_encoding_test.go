package renderer

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/cff"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/standard"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
)

type stubFontCandidateResolver struct {
	font    entity.Font
	called  bool
	subtype string
	base    string
}

type stubTextRenderer struct {
	called    bool
	text      string
	font      entity.Font
	fontSize  float64
	codeUnits []textCodeUnit
	err       error
}

type stubTextRenderPolicy struct {
	skipAll   bool
	skipFont  bool
	fastPath  bool
	skipCodes map[uint32]struct{}
}

type stubTextPlacement struct {
	currentX     float64
	currentY     float64
	advanceValue float64
	advances     []float64
	adjustments  []float64
	moves        [][2]float64
}

func (s *stubFontCandidateResolver) ResolveCandidate(_ *Evaluator, _ *entity.Dict, subtype, baseFont string, _ []byte, _ error) entity.Font {
	s.called = true
	s.subtype = subtype
	s.base = baseFont
	return s.font
}

func (s *stubTextRenderer) Render(_ *Evaluator, text string, font entity.Font, fontSize float64, codeUnits []textCodeUnit) error {
	s.called = true
	s.text = text
	s.font = font
	s.fontSize = fontSize
	s.codeUnits = append([]textCodeUnit(nil), codeUnits...)
	return s.err
}

func (s stubTextRenderPolicy) ShouldSkipAllText() bool {
	return s.skipAll
}

func (s stubTextRenderPolicy) ShouldSkipTextFont(_ string, _ entity.Font) bool {
	return s.skipFont
}

func (s stubTextRenderPolicy) ShouldUseFastPathTextRenderMode() bool {
	return s.fastPath
}

func (s stubTextRenderPolicy) HasSkippedTextCodes(_ string, _ entity.Font) bool {
	return len(s.skipCodes) > 0
}

func (s stubTextRenderPolicy) ShouldSkipTextCode(_ string, _ entity.Font, code uint32) bool {
	_, ok := s.skipCodes[code]
	return ok
}

func (s *stubTextPlacement) CurrentPosition(_ *Evaluator) (float64, float64) {
	return s.currentX, s.currentY
}

func (s *stubTextPlacement) CurrentRenderingMatrix(_ *Evaluator) [6]float64 {
	return [6]float64{1, 0, 0, 1, s.currentX, s.currentY}
}

func (s *stubTextPlacement) GlyphAdvance(_ *Evaluator, _ uint32, _ entity.Font, _ float64) float64 {
	return s.advanceValue
}

func (s *stubTextPlacement) AdvanceTextMatrix(_ *Evaluator, tx float64) {
	s.advances = append(s.advances, tx)
	s.currentX += tx
}

func (s *stubTextPlacement) MoveTextBy(_ *Evaluator, tx, ty float64) {
	s.moves = append(s.moves, [2]float64{tx, ty})
	s.currentX += tx
	s.currentY += ty
}

func (s *stubTextPlacement) ApplyTextAdjustment(_ *Evaluator, adjustment, _ float64) {
	s.adjustments = append(s.adjustments, adjustment)
}

type encodingTestFont struct {
	widths map[uint32]float64
	names  map[uint32]string
}

func (f *encodingTestFont) CharCodeToGlyph(code uint32) (uint32, error) {
	if _, ok := f.names[code]; !ok {
		return 0, errors.New("missing glyph")
	}
	return code, nil
}

func (f *encodingTestFont) GlyphName(glyph uint32) string {
	return f.names[glyph]
}

func (f *encodingTestFont) GetGlyphWidth(glyph uint32) (float64, error) {
	width, ok := f.widths[glyph]
	if !ok {
		return 0, errors.New("missing width")
	}
	return width, nil
}

func (f *encodingTestFont) GetBoundingBox() (float64, float64, float64, float64) {
	return 0, 0, 1000, 1000
}

func (f *encodingTestFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return nil, nil
}

func (f *encodingTestFont) IsCIDFont() bool {
	return false
}

func (f *encodingTestFont) IsSymbolic() bool {
	return false
}

func (f *encodingTestFont) UnitsPerEm() uint16 {
	return 1000
}

func (f *encodingTestFont) Name() string {
	return "EncodingTestFont"
}

func (f *encodingTestFont) GlyphIDByName(name string) (uint32, bool) {
	for glyph, glyphName := range f.names {
		if glyphName == name {
			return glyph, true
		}
	}
	return 0, false
}

func TestApplyFontEncodingFromDict_UsesDifferencesGlyphNames(t *testing.T) {
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

	wrapped := eval.applyFontEncodingFromDict(fontDict, baseFont)
	require.NotNil(t, wrapped)

	glyphA, err := wrapped.CharCodeToGlyph(65)
	require.NoError(t, err)
	assert.Equal(t, uint32(200), glyphA)

	glyphB, err := wrapped.CharCodeToGlyph(66)
	require.NoError(t, err)
	assert.Equal(t, uint32(201), glyphB)
}

func TestApplyFontEncodingFromDict_UsesAliasGlyphNamesWhenExactNameMissing(t *testing.T) {
	eval := NewEvaluator(nil)
	baseFont := &encodingTestFont{
		widths: map[uint32]float64{
			200: 500,
			201: 500,
			202: 500,
			203: 500,
		},
		names: map[uint32]string{
			200: `"`,
			201: ",",
			202: "-",
			203: "f",
		},
	}

	encoding := entity.NewDict()
	encoding.Set(
		entity.Name("Differences"),
		entity.NewArray(
			entity.NewInteger(16),
			entity.Name("quotedblleft"),
			entity.Name("quotedblbase"),
			entity.Name("endash"),
			entity.Name("fi"),
		),
	)

	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Encoding"), encoding)

	wrapped := eval.applyFontEncodingFromDict(fontDict, baseFont)
	require.NotNil(t, wrapped)

	glyph, err := wrapped.CharCodeToGlyph(16)
	require.NoError(t, err)
	assert.Equal(t, uint32(200), glyph)

	glyph, err = wrapped.CharCodeToGlyph(17)
	require.NoError(t, err)
	assert.Equal(t, uint32(201), glyph)

	glyph, err = wrapped.CharCodeToGlyph(18)
	require.NoError(t, err)
	assert.Equal(t, uint32(202), glyph)

	glyph, err = wrapped.CharCodeToGlyph(19)
	require.NoError(t, err)
	assert.Equal(t, uint32(203), glyph)
}

func TestEncodingGlyphNameCandidates_ExtendsType1ProgramAliases(t *testing.T) {
	assert.Contains(t, encodingGlyphNameCandidates("ff"), "f")
	assert.Contains(t, encodingGlyphNameCandidates("minus"), "-")
	assert.Contains(t, encodingGlyphNameCandidates("periodcentered"), ".")
	assert.Contains(t, encodingGlyphNameCandidates("plusminus"), "+")
}

func TestNewEmbeddedType1FontCanPreferCFFForRawType1CBytes(t *testing.T) {
	t.Setenv("PDF_DEBUG_TYPE1C_CFF_FIRST", "1")
	eval := NewEvaluator(nil)

	font := eval.newEmbeddedType1Font(minimalCFFFontDataForRendererTest(), nil)

	require.NotNil(t, font)
	_, ok := font.(*cff.Font)
	assert.True(t, ok, "raw Type1C/CFF bytes must not be accepted as permissive Type1")
}

func TestResolveEmbeddedType1EncodingUsesCFFBuiltinEncoding(t *testing.T) {
	data, err := os.ReadFile("../../../test/integration/pdf/testdata/009-pdflatex-geotopo/GeoTopo-komprimiert.pdf")
	require.NoError(t, err)

	table := xref.NewTable(data)
	require.NoError(t, table.Parse())
	obj, err := table.Fetch(entity.NewRef(1912, 0))
	require.NoError(t, err)
	fontDict, ok := obj.(*entity.Dict)
	require.True(t, ok)

	encoding := NewEvaluator(table).resolveEmbeddedType1Encoding(fontDict)

	require.Equal(t, "intersection", encoding[0x5c])
	assert.NotEqual(t, "backslash", encoding[0x5c])
}

func TestApplyEmbeddedSimpleFontBBoxFromDictUsesType1FontDescriptorBBox(t *testing.T) {
	eval := NewEvaluator(nil)
	baseFont := &encodingTestFont{
		widths: map[uint32]float64{65: 500},
		names:  map[uint32]string{65: "A"},
	}

	descriptor := entity.NewDict()
	descriptor.Set(
		entity.Name("FontBBox"),
		entity.NewArray(
			entity.NewInteger(-234),
			entity.NewInteger(-319),
			entity.NewInteger(1740),
			entity.NewInteger(892),
		),
	)

	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Subtype"), entity.Name("Type1"))
	fontDict.Set(entity.Name("FontDescriptor"), descriptor)

	wrapped := eval.applyEmbeddedSimpleFontBBoxFromDict(fontDict, baseFont, []byte("%!PS-AdobeFont-1.0\n"))
	xMin, yMin, xMax, yMax := wrapped.GetBoundingBox()

	assert.Equal(t, -234.0, xMin)
	assert.Equal(t, -319.0, yMin)
	assert.Equal(t, 1740.0, xMax)
	assert.Equal(t, 892.0, yMax)
}

func TestLooksLikeCFFEmbeddedFont(t *testing.T) {
	assert.True(t, looksLikeCFFEmbeddedFont(minimalCFFFontDataForRendererTest()))
	assert.False(t, looksLikeCFFEmbeddedFont([]byte("%!PS-AdobeFont-1.0\n")))
	assert.False(t, looksLikeCFFEmbeddedFont([]byte{0x80, 0x01, 0x00, 0x00}))
	assert.False(t, looksLikeCFFEmbeddedFont([]byte{0x01, 0x00, 0x04, 0x00}))
}

func minimalCFFFontDataForRendererTest() []byte {
	return []byte{
		0x01, 0x00, 0x04, 0x01,
		0x00, 0x01,
		0x00, 0x01,
		0x00, 0x01,
		0x00, 0x01,
	}
}

func TestShouldUseEmbeddedType1ForBaseFontDebug_MatchesNormalizedBaseFontName(t *testing.T) {
	t.Setenv("PDF_DEBUG_TYPE1_EMBEDDED_FOR_BASE", "cmr10,sfrm1095")

	assert.True(t, shouldUseEmbeddedType1ForBaseFontDebug("IYCZZB+CMR10"))
	assert.True(t, shouldUseEmbeddedType1ForBaseFontDebug("/FJKNGJ+SFRM1095"))
	assert.False(t, shouldUseEmbeddedType1ForBaseFontDebug("CMSY10"))
}

func TestShouldUseFallbackType1ForBaseFontDebug_MatchesSubsetFreeBaseFontName(t *testing.T) {
	t.Setenv("PDF_DEBUG_TYPE1_FALLBACK_FOR_BASE", "sftt1095,nimbussanl-bold")

	assert.True(t, shouldUseFallbackType1ForBaseFontDebug("CNQMHB+SFTT1095"))
	assert.True(t, shouldUseFallbackType1ForBaseFontDebug("/OUCZRR+NimbusSanL-Bold"))
	assert.False(t, shouldUseFallbackType1ForBaseFontDebug("CMR10"))
}

func TestForcedFallbackFontNameForBaseFontDebug_MatchesSubsetFreeBaseFontName(t *testing.T) {
	t.Setenv("PDF_DEBUG_FORCE_BASE_FONT_MAP", "cmr10=Helvetica,sftt1095=Courier")

	assert.Equal(t, "Helvetica", forcedFallbackFontNameForBaseFontDebug("IYCZZB+CMR10"))
	assert.Equal(t, "Courier", forcedFallbackFontNameForBaseFontDebug("/CNQMHB+SFTT1095"))
	assert.Empty(t, forcedFallbackFontNameForBaseFontDebug("CMSY10"))
}

func TestGlyphSourceOverridesForBaseFontDebug_MatchesNormalizedBaseFontName(t *testing.T) {
	t.Setenv(
		"PDF_DEBUG_FORCE_GLYPH_SOURCE_MAP",
		"cmr10:47=Helvetica,/CNQMHB+SFTT1095:65=Courier,sfrm1095:47=Times-Roman,broken,cmr10:oops=Helvetica",
	)

	cmr := glyphSourceOverridesForBaseFontDebug("IYCZZB+CMR10")
	require.Equal(t, map[int]string{47: "Helvetica"}, cmr)

	sftt := glyphSourceOverridesForBaseFontDebug("/CNQMHB+SFTT1095")
	require.Equal(t, map[int]string{65: "Courier"}, sftt)

	assert.Empty(t, glyphSourceOverridesForBaseFontDebug("CMSY10"))
}

func TestFormatGlyphSourceOverrideSpecForDebug(t *testing.T) {
	assert.Equal(t, "SFRM1095:47=Helvetica", formatGlyphSourceOverrideSpecForDebug("SFRM1095", 47, "Helvetica"))
}

func TestShouldSkipDebugTextFont_MatchesBaseAndResolvedNames(t *testing.T) {
	t.Setenv("PDF_DEBUG_SKIP_TEXT_BASE_FONTS", "cmr10")

	font := &encodingTestFont{
		widths: map[uint32]float64{200: 500},
		names:  map[uint32]string{200: "A"},
	}

	assert.True(t, shouldSkipDebugTextFont("IYCZZB+CMR10", font))
	assert.True(t, shouldSkipDebugTextFont("/IYCZZB+CMR10", font))
	assert.False(t, shouldSkipDebugTextFont("CMSY10", font))
}

func TestShouldSkipAllTextForDebug(t *testing.T) {
	t.Setenv("PDF_DEBUG_SKIP_TEXT", "1")
	assert.True(t, shouldSkipAllTextForDebug())

	t.Setenv("PDF_DEBUG_SKIP_TEXT", " 0 ")
	assert.False(t, shouldSkipAllTextForDebug())
}

func TestShouldSkipAllImagesForDebug(t *testing.T) {
	t.Setenv("PDF_DEBUG_SKIP_IMAGES", "1")
	assert.True(t, shouldSkipAllImagesForDebug())

	t.Setenv("PDF_DEBUG_SKIP_IMAGES", " 0 ")
	assert.False(t, shouldSkipAllImagesForDebug())
}

func TestShouldSkipAllXObjectsForDebug(t *testing.T) {
	t.Setenv("PDF_DEBUG_SKIP_XOBJECTS", "1")
	assert.True(t, shouldSkipAllXObjectsForDebug())

	t.Setenv("PDF_DEBUG_SKIP_XOBJECTS", " 0 ")
	assert.False(t, shouldSkipAllXObjectsForDebug())
}

func TestShouldSkipStrokePathsForDebug(t *testing.T) {
	t.Setenv("PDF_DEBUG_SKIP_STROKE_PATHS", "1")
	assert.True(t, shouldSkipStrokePathsForDebug())

	t.Setenv("PDF_DEBUG_SKIP_STROKE_PATHS", " 0 ")
	assert.False(t, shouldSkipStrokePathsForDebug())
}

func TestShouldSkipFillPathsForDebug(t *testing.T) {
	t.Setenv("PDF_DEBUG_SKIP_FILL_PATHS", "1")
	assert.True(t, shouldSkipFillPathsForDebug())

	t.Setenv("PDF_DEBUG_SKIP_FILL_PATHS", " 0 ")
	assert.False(t, shouldSkipFillPathsForDebug())
}

func TestShouldUseFastPathTextRenderModeForDebug(t *testing.T) {
	t.Setenv("PDF_DEBUG_TEXT_RENDER_MODE", "fast-path")
	assert.True(t, shouldUseFastPathTextRenderModeForDebug())

	t.Setenv("PDF_DEBUG_TEXT_RENDER_MODE", " char-by-char ")
	assert.False(t, shouldUseFastPathTextRenderModeForDebug())
}

func TestShouldSkipTextFontForDebug_DelegatesToFontMatcher(t *testing.T) {
	t.Setenv("PDF_DEBUG_SKIP_TEXT_BASE_FONTS", "cmr10")

	font := &encodingTestFont{
		widths: map[uint32]float64{200: 500},
		names:  map[uint32]string{200: "A"},
	}

	assert.True(t, shouldSkipTextFontForDebug("IYCZZB+CMR10", font))
	assert.False(t, shouldSkipTextFontForDebug("CMSY10", font))
}

func TestShouldSkipTextCodeForDebug_MatchesBaseAndCode(t *testing.T) {
	t.Setenv("PDF_DEBUG_SKIP_TEXT_CODES_FOR_BASE", "cmr10=101,116;sfrm1095=97")

	font := &encodingTestFont{
		widths: map[uint32]float64{200: 500},
		names:  map[uint32]string{200: "A"},
	}

	assert.True(t, hasSkippedTextCodesForDebug("IYCZZB+CMR10", font))
	assert.True(t, shouldSkipTextCodeForDebug("IYCZZB+CMR10", font, 101))
	assert.True(t, shouldSkipTextCodeForDebug("IYCZZB+CMR10", font, 116))
	assert.False(t, shouldSkipTextCodeForDebug("IYCZZB+CMR10", font, 97))
	assert.False(t, shouldSkipTextCodeForDebug("CMSY10", font, 101))
}

func TestGetFontFromDict_UsesInjectedFontResolver(t *testing.T) {
	eval := NewEvaluator(nil)
	font, ok := standard.GetFont("Helvetica")
	require.True(t, ok)
	resolver := &stubFontCandidateResolver{
		font: font,
	}
	eval.fontResolver = resolver

	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Subtype"), entity.Name("Type1"))

	resolvedFont, err := eval.getFontFromDict(fontDict, "CustomBase")
	require.NoError(t, err)
	require.True(t, resolver.called)
	assert.Equal(t, "Type1", resolver.subtype)
	assert.Equal(t, "CustomBase", resolver.base)
	assert.NotNil(t, resolvedFont)
}

func TestGetFontFromDict_UsesInjectedFontFallbackResolverForMissingCandidate(t *testing.T) {
	eval := NewEvaluator(nil)
	eval.fontResolver = &stubFontCandidateResolver{}

	font, ok := standard.GetFont("Helvetica")
	require.True(t, ok)
	fallback := &stubFontFallbackResolver{
		missingFont: font,
	}
	eval.fontFallback = fallback

	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Subtype"), entity.Name("Type1"))

	resolvedFont, err := eval.getFontFromDict(fontDict, "CustomBase")
	require.NoError(t, err)
	require.True(t, fallback.missingCalled)
	assert.Equal(t, "Type1", fallback.subtype)
	assert.Equal(t, "CustomBase", fallback.base)
	assert.Equal(t, font.Name(), resolvedFont.Name())
}

func TestGetFontFromDict_UsesInjectedFontFallbackResolverForNonRenderableCandidate(t *testing.T) {
	eval := NewEvaluator(nil)
	eval.fontResolver = &stubFontCandidateResolver{
		font: &encodingTestFont{
			widths: map[uint32]float64{65: 500},
			names:  map[uint32]string{65: "A"},
		},
	}

	font, ok := standard.GetFont("Helvetica")
	require.True(t, ok)
	fallback := &stubFontFallbackResolver{
		nonRenderable:   font,
		nonRenderableOK: true,
	}
	eval.fontFallback = fallback

	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Subtype"), entity.Name("Type1"))

	resolvedFont, err := eval.getFontFromDict(fontDict, "CustomBase")
	require.NoError(t, err)
	require.True(t, fallback.nonRenderCalled)
	assert.Equal(t, "Type1", fallback.subtype)
	assert.Equal(t, "CustomBase", fallback.base)
	assert.Equal(t, font.Name(), resolvedFont.Name())
}

func TestRenderTextString_UsesInjectedTextRenderer(t *testing.T) {
	eval := NewEvaluator(nil)
	font := &encodingTestFont{
		widths: map[uint32]float64{65: 500},
		names:  map[uint32]string{65: "A"},
	}
	renderer := &stubTextRenderer{}
	eval.textRenderer = renderer
	eval.graphics.currentState.SetFont(font)
	eval.graphics.currentState.SetFontSize(18)

	err := eval.renderTextString("A")
	require.NoError(t, err)
	require.True(t, renderer.called)
	assert.Equal(t, "A", renderer.text)
	assert.Equal(t, font, renderer.font)
	assert.Equal(t, 18.0, renderer.fontSize)
	require.Len(t, renderer.codeUnits, 1)
	assert.Equal(t, uint32(65), renderer.codeUnits[0].code)
}

func TestRenderTextCharByChar_UsesInjectedTextPolicyForCodeSkipping(t *testing.T) {
	eval := NewEvaluator(nil)
	font := &encodingTestFont{
		widths: map[uint32]float64{
			65: 500,
			66: 500,
		},
		names: map[uint32]string{
			65: "A",
			66: "B",
		},
	}
	eval.textPolicy = stubTextRenderPolicy{
		skipCodes: map[uint32]struct{}{
			65: {},
		},
	}
	eval.graphics.currentState.SetFont(font)
	eval.graphics.currentState.SetFontSize(10)

	err := eval.renderTextCharByChar("AB", font, 10)
	require.NoError(t, err)
	assert.Equal(t, "AB", eval.textBuffer.String())
}

func TestCaptureTextWithoutRendering_UsesInjectedTextPlacement(t *testing.T) {
	eval := NewEvaluator(nil)
	font := &encodingTestFont{
		widths: map[uint32]float64{
			65: 500,
			66: 500,
		},
		names: map[uint32]string{
			65: "A",
			66: "B",
		},
	}
	placement := &stubTextPlacement{
		currentX:     10,
		currentY:     20,
		advanceValue: 7,
	}
	eval.textPlacement = placement

	eval.captureTextWithoutRendering([]textCodeUnit{
		{code: 65, raw: []byte("A")},
		{code: 66, raw: []byte("B")},
	}, font, 12)

	assert.Equal(t, "AB", eval.textBuffer.String())
	assert.Equal(t, []float64{7, 7}, placement.advances)
	assert.Equal(t, 24.0, placement.currentX)
	assert.Equal(t, 20.0, placement.currentY)
}

func TestShowTextArray_UsesInjectedTextPlacementForAdjustments(t *testing.T) {
	eval := NewEvaluator(nil)
	font := &encodingTestFont{
		widths: map[uint32]float64{65: 500},
		names:  map[uint32]string{65: "A"},
	}
	placement := &stubTextPlacement{}
	eval.textPlacement = placement
	eval.graphics.currentState.SetFont(font)
	eval.graphics.currentState.SetFontSize(12)

	arr := entity.NewArray(
		entity.NewString("A"),
		entity.NewInteger(120),
		entity.NewReal(45.5),
	)
	op := Operator{Operands: []entity.Object{arr}}

	err := eval.showTextArray(op)
	require.NoError(t, err)
	assert.Equal(t, []float64{120, 45.5}, placement.adjustments)
}

func TestMoveTextOperators_UseInjectedTextPlacementForMovement(t *testing.T) {
	eval := NewEvaluator(nil)
	placement := &stubTextPlacement{}
	eval.textPlacement = placement
	eval.graphics.currentState.SetTextLeading(9)

	require.NoError(t, eval.moveText(Operator{Operands: []entity.Object{
		entity.NewReal(3),
		entity.NewReal(4),
	}}))
	require.NoError(t, eval.moveTextSetLeading(Operator{Operands: []entity.Object{
		entity.NewReal(5),
		entity.NewReal(-6),
	}}))
	require.NoError(t, eval.moveTextNextLine())

	assert.Equal(t, [][2]float64{
		{3, 4},
		{5, -6},
		{0, -6},
	}, placement.moves)
	assert.Equal(t, 6.0, eval.graphics.currentState.GetTextLeading())
}
