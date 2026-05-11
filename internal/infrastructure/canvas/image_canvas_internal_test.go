package canvas

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/graphics"
	"github.com/dh-kam/pdf-go/internal/domain/renderer"
	infraimage "github.com/dh-kam/pdf-go/internal/infrastructure/image"
	"golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
)

func TestSplitTextCodes(t *testing.T) {
	raw := []byte("AB")
	nonCID := splitTextCodes(raw, nil)
	assert.Equal(t, []uint32{65, 66}, nonCID)

	font := &mockTextCIDFont{}
	cid := splitTextCodes([]byte{0x4E, 0x00, 0x4E, 0x8C}, font)
	assert.Equal(t, []uint32{0x4E00, 0x4E8C}, cid)
}

func TestDebugStrokePathToggles(t *testing.T) {
	t.Setenv("PDF_DEBUG_SKIP_STROKE_SEGMENTS", "1")
	assert.True(t, shouldSkipStrokeSegmentsForDebug())

	t.Setenv("PDF_DEBUG_SKIP_STROKE_SEGMENTS", "0")
	assert.False(t, shouldSkipStrokeSegmentsForDebug())

	t.Setenv("PDF_DEBUG_SKIP_CLOSED_STROKE_OUTLINES", "1")
	assert.True(t, shouldSkipClosedStrokeOutlinesForDebug())

	t.Setenv("PDF_DEBUG_SKIP_CLOSED_STROKE_OUTLINES", "0")
	assert.False(t, shouldSkipClosedStrokeOutlinesForDebug())

	t.Setenv("PDF_DEBUG_STROKE_TRACE_SUBPATHS", "1")
	assert.True(t, shouldTraceStrokeSubpathsForDebug())

	t.Setenv("PDF_DEBUG_STROKE_TRACE_SUBPATHS", "0")
	assert.False(t, shouldTraceStrokeSubpathsForDebug())

	t.Setenv("PDF_DEBUG_SKIP_STROKE_ADJUST_OUTLINES", "1")
	assert.True(t, shouldSkipStrokeAdjustOutlinesForDebug())

	t.Setenv("PDF_DEBUG_SKIP_STROKE_ADJUST_OUTLINES", "0")
	assert.False(t, shouldSkipStrokeAdjustOutlinesForDebug())

	t.Setenv("PDF_DEBUG_STROKE_ADJUST_TRAILING_INSET", "1")
	assert.True(t, shouldUseStrokeAdjustTrailingInsetForDebug())

	t.Setenv("PDF_DEBUG_STROKE_ADJUST_TRAILING_INSET", "0")
	assert.False(t, shouldUseStrokeAdjustTrailingInsetForDebug())

	t.Setenv("PDF_DEBUG_TILING_PATTERN_PER_TILE_REPLAY", "1")
	assert.True(t, shouldReplayTilingPatternPerTileForDebug())

	t.Setenv("PDF_DEBUG_TILING_PATTERN_PER_TILE_REPLAY", "0")
	assert.False(t, shouldReplayTilingPatternPerTileForDebug())
}

func TestShouldReplayTilingPatternPerTile(t *testing.T) {
	t.Setenv("PDF_DEBUG_TILING_PATTERN_PER_TILE_REPLAY", "0")
	fillOps := []renderer.Operator{{Opcode: "f"}}
	strokeOps := []renderer.Operator{{Opcode: "S"}}
	assert.True(t, shouldReplayTilingPatternPerTile(nil, 5, 11, false))
	assert.True(t, shouldReplayTilingPatternPerTile(nil, 11, 5, false))
	assert.False(t, shouldReplayTilingPatternPerTile(nil, 11, 11, false))
	assert.True(t, shouldReplayTilingPatternPerTile(fillOps, 11, 11, true))
	assert.False(t, shouldReplayTilingPatternPerTile(strokeOps, 11, 11, true))

	t.Setenv("PDF_DEBUG_TILING_PATTERN_PER_TILE_REPLAY", "1")
	assert.True(t, shouldReplayTilingPatternPerTile(nil, 11, 11, false))
}

func TestTilingPatternStepMismatch(t *testing.T) {
	assert.False(t, tilingPatternStepMismatch(10, 12, 10, 12))
	assert.False(t, tilingPatternStepMismatch(10, 12, 10.0000001, 12.0000001))
	assert.True(t, tilingPatternStepMismatch(10, 12, 10.5, 12))
	assert.True(t, tilingPatternStepMismatch(10, 12, 10, 11.5))
}

func TestShouldRenderRoundCapJoinStrokeOutlines(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 64, 64)).(*ImageCanvas)
	c.SetLineCap(1)
	c.SetLineJoin(1)

	diagonal := []lineSegment{
		{x1: 8, y1: 8, x2: 24, y2: 24},
		{x1: 24, y1: 24, x2: 40, y2: 16},
	}
	assert.True(t, c.shouldRenderRoundCapJoinStrokeOutlines(diagonal, 1.25))

	axisAligned := []lineSegment{
		{x1: 8, y1: 8, x2: 24, y2: 8},
		{x1: 24, y1: 8, x2: 24, y2: 24},
	}
	assert.False(t, c.shouldRenderRoundCapJoinStrokeOutlines(axisAligned, 1.25))

	c.SetLineJoin(0)
	assert.False(t, c.shouldRenderRoundCapJoinStrokeOutlines(diagonal, 1.25))
}

func TestBuildRoundCapJoinStrokeOutlinesAddsRoundCapsAndJoinWedge(t *testing.T) {
	segments := []lineSegment{
		{x1: 8, y1: 8, x2: 24, y2: 20},
		{x1: 24, y1: 20, x2: 40, y2: 12},
	}

	outlines := buildRoundCapJoinStrokeOutlines(segments, 2)

	require.Len(t, outlines, 5)
	hasJoinCenter := false
	for _, outline := range outlines {
		require.NotEmpty(t, outline)
		assert.LessOrEqual(t, strokePolygonArea(outline), 0.0)
		for _, point := range outline {
			if math.Abs(point.x-24) <= 1e-9 && math.Abs(point.y-20) <= 1e-9 {
				hasJoinCenter = true
				break
			}
		}
	}
	assert.True(t, hasJoinCenter)
}

func TestShouldRenderOpenMiterStrokeOutlines(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 64, 64)).(*ImageCanvas)
	c.SetLineCap(0)
	c.SetLineJoin(0)

	diagonal := []lineSegment{
		{x1: 8, y1: 8, x2: 24, y2: 20},
		{x1: 24, y1: 20, x2: 40, y2: 12},
	}
	assert.True(t, c.shouldRenderOpenMiterStrokeOutlines(diagonal, 1.5))

	axisAligned := []lineSegment{
		{x1: 8, y1: 8, x2: 24, y2: 8},
		{x1: 24, y1: 8, x2: 24, y2: 24},
	}
	assert.False(t, c.shouldRenderOpenMiterStrokeOutlines(axisAligned, 1.5))

	c.SetLineCap(1)
	assert.False(t, c.shouldRenderOpenMiterStrokeOutlines(diagonal, 1.5))
}

func TestBuildOpenMiterJoinStrokeOutline(t *testing.T) {
	segments := []lineSegment{
		{x1: 0, y1: 0, x2: 10, y2: 0},
		{x1: 10, y1: 0, x2: 10, y2: 10},
	}

	outline, ok := buildOpenMiterJoinStrokeOutline(segments, 1, 10)

	require.True(t, ok)
	require.Len(t, outline, 6)
	assert.LessOrEqual(t, strokePolygonArea(outline), 0.0)

	minX, minY, maxX, maxY := strokePointBounds(outline)
	assert.InDelta(t, 0.0, minX, 1e-9)
	assert.InDelta(t, -1.0, minY, 1e-9)
	assert.InDelta(t, 11.0, maxX, 1e-9)
	assert.InDelta(t, 10.0, maxY, 1e-9)
}

func TestSplitStrokeSegmentSubpaths(t *testing.T) {
	segments := []lineSegment{
		{x1: 0, y1: 0, x2: 10, y2: 0},
		{x1: 10, y1: 0, x2: 10, y2: 10},
		{x1: 20, y1: 20, x2: 30, y2: 20},
		{x1: 30, y1: 20, x2: 20, y2: 20},
	}

	subpaths := splitStrokeSegmentSubpaths(segments)

	require.Len(t, subpaths, 2)
	assert.False(t, subpaths[0].closed)
	assert.True(t, subpaths[1].closed)
	assert.Len(t, subpaths[0].segments, 2)
	assert.Len(t, subpaths[1].segments, 2)
}

func TestStrokeAdjustAxisAlignedOutlinesRoundsToNearestPixel(t *testing.T) {
	outlines := [][]strokePoint{
		{
			{x: 10.1, y: 20.49},
			{x: 10.1, y: 30.51},
			{x: 21.6, y: 30.51},
			{x: 21.6, y: 20.49},
		},
	}

	got := strokeAdjustAxisAlignedOutlines(outlines)

	assert.Equal(t, [][]strokePoint{
		{
			{x: 10, y: 20},
			{x: 10, y: 31},
			{x: 22, y: 31},
			{x: 22, y: 20},
		},
	}, got)
}

func TestStrokeAdjustAxisAlignedOutlinesLeavesDiagonalGeometryUntouched(t *testing.T) {
	outlines := [][]strokePoint{
		{
			{x: 0.25, y: 0.25},
			{x: 5.75, y: 3.5},
			{x: 1.5, y: 6.75},
		},
	}

	got := strokeAdjustAxisAlignedOutlines(outlines)

	assert.Equal(t, outlines, got)
}

func TestStrokeAdjustAxisAlignedOutlinesAppliesTrailingInsetOnlyUnderDebugGate(t *testing.T) {
	outlines := [][]strokePoint{
		{
			{x: 10.5, y: 20.5},
			{x: 10.5, y: 30.5},
			{x: 21.5, y: 30.5},
			{x: 21.5, y: 20.5},
		},
		{
			{x: 11.5, y: 21.5},
			{x: 11.5, y: 29.5},
			{x: 20.5, y: 29.5},
			{x: 20.5, y: 21.5},
		},
	}

	gotDefault := strokeAdjustAxisAlignedOutlines(outlines)
	assert.Equal(t, 22.0, gotDefault[0][2].x)
	assert.Equal(t, 30.0, gotDefault[0][1].y)

	t.Setenv("PDF_DEBUG_STROKE_ADJUST_TRAILING_INSET", "1")
	gotInset := strokeAdjustAxisAlignedOutlines(outlines)
	assert.Equal(t, 21.99, gotInset[0][2].x)
	assert.Equal(t, 29.99, gotInset[0][1].y)
	assert.Equal(t, 10.0, gotInset[0][0].x)
	assert.Equal(t, 20.0, gotInset[0][0].y)
	assert.Equal(t, 19.99, gotInset[1][2].x)
	assert.Equal(t, 29.99, gotInset[1][1].y)
}

func TestStrokePolylineIsAxisAligned(t *testing.T) {
	assert.True(t, strokePolylineIsAxisAligned([]strokePoint{
		{x: 0, y: 0},
		{x: 0, y: 4},
		{x: 3, y: 4},
		{x: 3, y: 0},
	}))

	assert.False(t, strokePolylineIsAxisAligned([]strokePoint{
		{x: 0, y: 0},
		{x: 2, y: 3},
		{x: 3, y: 0},
	}))
}

func TestPopplerGlyphXPhaseForFontSuppressesLargeGlyphs(t *testing.T) {
	font := &mockTextFont{}

	assert.Equal(t, 0.5, popplerGlyphXPhaseForFont(10.5, font, 20, 1))
	assert.Equal(t, 0.0, popplerGlyphXPhaseForFont(10.5, font, 48, 1))
	assert.Equal(t, 0.0, popplerGlyphXPhaseForFont(10.5, font, 24, 2))
}

func TestImageCanvas_RenderGlyphBitmapPhasedUsesFloorYPlacement(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)

	c.renderGlyphBitmapPhased([]byte{255}, 1, 1, 0, 0, 1.25, 5.25, color.Black)

	assert.Equal(t, color.RGBA{0, 0, 0, 255}, c.img.RGBAAt(1, 2))
	assert.Equal(t, color.RGBA{}, c.img.RGBAAt(1, 3))
}

func TestImageCanvas_SetClipPathDirectClearsCurrentPath(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 10, 10)).(*ImageCanvas)
	c.MoveTo(1, 1)
	c.LineTo(9, 1)

	c.SetClipPathDirect([]interface{}{
		&graphics.MoveTo{X: 0, Y: 0},
		&graphics.LineTo{X: 10, Y: 0},
		&graphics.LineTo{X: 10, Y: 10},
		&graphics.ClosePath{},
	}, graphics.FillRuleNonZero)

	require.NotNil(t, c.clipPath)
	assert.Empty(t, c.currentPath.GetCommands())
}

func TestApplyPremultipliedAlphaUsesSplashDiv255(t *testing.T) {
	src := color.RGBA{R: 1, G: 3, B: 255, A: 255}

	assert.Equal(t, uint8(129), splashDiv255(33023))
	assert.Equal(t, color.RGBA{R: 1, G: 2, B: 128, A: 128}, applyPremultipliedAlpha(src, 128))
	assert.Equal(t, color.RGBA{R: 129, G: 0, B: 0, A: 0}, applyPremultipliedAlpha(color.RGBA{R: 255}, 129))
	assert.Equal(t, color.RGBA{}, applyPremultipliedAlpha(src, 0))
	assert.Equal(t, src, applyPremultipliedAlpha(src, 255))
}

func TestIntersectClipMaskAlphaKeepsSingleCoverageForPartialIntersection(t *testing.T) {
	assert.Equal(t, uint8(0), intersectClipMaskAlpha(0, 255))
	assert.Equal(t, uint8(50), intersectClipMaskAlpha(255, 50))
	assert.Equal(t, uint8(50), intersectClipMaskAlpha(50, 255))
	assert.Equal(t, uint8(50), intersectClipMaskAlpha(50, 50))
}

func TestPatternFillPixelAlphaUsesPopplerRectClipAALineSemantics(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 2, 2)).(*ImageCanvas)

	partial := transformedAxisAlignedRect{
		minX:        0,
		minY:        0,
		maxX:        0.1,
		maxY:        1,
		pixelBounds: image.Rect(0, 0, 1, 1),
	}
	full := transformedAxisAlignedRect{
		minX:        0,
		minY:        0,
		maxX:        1,
		maxY:        1,
		pixelBounds: image.Rect(0, 0, 1, 1),
	}
	rightEdge := transformedAxisAlignedRect{
		minX:        0,
		minY:        0,
		maxX:        0.25,
		maxY:        1,
		pixelBounds: image.Rect(0, 0, 1, 1),
	}
	yFractional := transformedAxisAlignedRect{
		minX:        0,
		minY:        0,
		maxX:        1,
		maxY:        0.1,
		pixelBounds: image.Rect(0, 0, 1, 1),
	}

	assert.Equal(t, uint8(32), c.patternFillPixelAlpha(0, 0, partial, true, nil))
	assert.Equal(t, uint8(90), c.patternFillPixelAlpha(0, 0, rightEdge, true, nil))
	assert.Equal(t, uint8(255), c.patternFillPixelAlpha(0, 0, yFractional, true, nil))
	assert.Equal(t, uint8(255), c.patternFillPixelAlpha(0, 0, full, true, nil))
}

func TestUpdateClipMaskClearsRectFastPathAfterComplexClip(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 40, 40)).(*ImageCanvas)

	c.Rectangle(0, 0, 30, 30)
	c.Clip()
	require.NotNil(t, c.clipMask)
	require.NotNil(t, c.clipRect)

	c.MoveTo(15, 5)
	c.CurveTo(30, 5, 30, 30, 15, 30)
	c.CurveTo(0, 30, 0, 5, 15, 5)
	c.ClosePath()
	c.Clip()

	require.NotNil(t, c.clipMask)
	assert.Nil(t, c.clipRect)
}

func TestUsesPopplerBinaryClipAlphaForOpaqueGouraudPatternFill(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 2, 2)).(*ImageCanvas)
	shading := entity.NewShading(entity.ShadingFreeFormGouraud, "DeviceRGB")
	c.fillPattern = entity.NewShadingPattern("mesh", shading)
	c.fillColor = color.RGBA{A: 255}

	assert.True(t, c.usesPopplerBinaryClipAlphaForPatternFill())

	c.fillColor = color.RGBA{A: 128}
	assert.False(t, c.usesPopplerBinaryClipAlphaForPatternFill())

	c.fillColor = color.RGBA{A: 255}
	c.fillPattern = entity.NewShadingPattern("axial", entity.NewShading(entity.ShadingAxial, "DeviceRGB"))
	assert.False(t, c.usesPopplerBinaryClipAlphaForPatternFill())
}

func TestPopplerRadialParameterUsesShadingDomainAndExtend(t *testing.T) {
	extend := [2]bool{true, false}
	domainMin, domainMax := 0.0, 50.0
	x0, y0, r0 := 0.0, 0.0, 10.0
	dx, dy, dr := 0.0, 0.0, 10.0
	a := dx*dx + dy*dy - dr*dr

	value, ok := popplerRadialParameter(15, 0, x0, y0, r0, dx, dy, dr, a, extend, domainMin, domainMax)
	require.True(t, ok)
	assert.InDelta(t, 25.0, value, 1e-9)

	value, ok = popplerRadialParameter(5, 0, x0, y0, r0, dx, dy, dr, a, extend, domainMin, domainMax)
	require.True(t, ok)
	assert.InDelta(t, domainMin, value, 1e-9)

	_, ok = popplerRadialParameter(25, 0, x0, y0, r0, dx, dy, dr, a, extend, domainMin, domainMax)
	assert.False(t, ok)
}

func TestQuantizeGouraudFunctionOutputUsesPopplerFixedPointTruncation(t *testing.T) {
	quantized := quantizeGouraudFunctionOutput([]float64{42.9 / 65536.0, 1})

	require.Len(t, quantized, 2)
	assert.Equal(t, 42.0/65536.0, quantized[0])
	assert.Equal(t, 1.0, quantized[1])
}

func TestSplashRoundCoordMatchesPopplerFloorHalfUp(t *testing.T) {
	assert.Equal(t, 3, splashRoundCoord(2.5))
	assert.Equal(t, -2, splashRoundCoord(-2.5))
}

func TestBuildTransformedPathSegmentsHelpers(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 100, 100)).(*ImageCanvas)
	c.MoveTo(10, 10)
	c.LineTo(30, 10)
	c.LineTo(30, 30)
	c.ClosePath()

	segments := c.buildTransformedPathSegments(c.currentPath.GetCommands())
	require.Len(t, segments, 3)

	minX, minY, maxX, maxY := segmentBounds(segments)
	assert.Equal(t, 10, minX)
	assert.Equal(t, 70, minY)
	assert.Equal(t, 30, maxX)
	assert.Equal(t, 90, maxY)

	assert.True(t, pointInPathEvenOdd(20, 80, segments))
}

func TestImageCanvas_EffectiveStrokeWidthUsesTransformScale(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 20, 20)).(*ImageCanvas)
	c.lineWidth = 2
	c.Transform([6]float64{2, 0, 0, 4, 0, 0})

	assert.InDelta(t, 6.0, c.effectiveStrokeWidth(), 1e-9)
}

func TestImageCanvas_ApplyDashPatternUsesScaledDashLengths(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 20, 20)).(*ImageCanvas)
	c.Transform([6]float64{2, 0, 0, 2, 0, 0})
	c.SetDashPattern([]float64{1, 1}, 0)

	dashed := c.applyDashPattern([]lineSegment{{x1: 0, y1: 0, x2: 8, y2: 0}})
	require.Len(t, dashed, 2)

	assert.InDelta(t, 0.0, dashed[0].x1, 1e-9)
	assert.InDelta(t, 2.0, dashed[0].x2, 1e-9)
	assert.InDelta(t, 4.0, dashed[1].x1, 1e-9)
	assert.InDelta(t, 6.0, dashed[1].x2, 1e-9)
}

func TestColorAndTempImageHelpers(t *testing.T) {
	col := colorToRGBA(color.NRGBA{R: 1, G: 2, B: 3, A: 255})
	assert.Equal(t, uint8(1), col.R)

	tmp := image.NewRGBA(image.Rect(0, 0, 4, 4))
	tmp.SetRGBA(1, 1, color.RGBA{10, 20, 30, 40})

	c := NewImageCanvas(image.Rect(0, 0, 4, 4)).(*ImageCanvas)
	c.drawTempImageWithClip(tmp)
	assert.Equal(t, uint8(40), c.Image().(*image.RGBA).RGBAAt(1, 1).A)

	c.clipMask = image.NewAlpha(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			c.clipMask.Set(x, y, color.Alpha{A: 255})
		}
	}
	c.drawTempImageWithClip(tmp)
	assert.Equal(t, uint8(55), c.Image().(*image.RGBA).RGBAAt(1, 1).B)
}

func TestDrawTempImageWithClipAndTransform(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 10, 10)).(*ImageCanvas)
	c.Transform([6]float64{1, 0, 0, 1, 2, 3})
	c.Transform([6]float64{2, 0, 0, 2, 0, 0})

	x, y := c.transformPoint(1, 2)
	assert.InDelta(t, 6, x, 0.0001)
	assert.InDelta(t, 10, y, 0.0001)
}

func TestPathRenderFunctions(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 20, 20)).(*ImageCanvas)
	c.MoveTo(10, 10)
	c.LineTo(15, 10)
	c.LineTo(15, 15)
	c.ClosePath()
	c.Fill()
}

func TestMockTextFont_RenderGlyph(t *testing.T) {
	font := &mockTextFont{}
	p, err := font.RenderGlyph(65, 12)
	require.NoError(t, err)
	assert.NotNil(t, p)
	assert.Empty(t, font.rendered)
}

func TestTextGlyphSupersampleFactorForDebug(t *testing.T) {
	t.Setenv("PDF_DEBUG_TEXT_GLYPH_SUPERSAMPLE", "")
	assert.Equal(t, 2, textGlyphSupersampleFactorForDebug())

	t.Setenv("PDF_DEBUG_TEXT_GLYPH_SUPERSAMPLE", "2")
	assert.Equal(t, 2, textGlyphSupersampleFactorForDebug())

	t.Setenv("PDF_DEBUG_TEXT_GLYPH_SUPERSAMPLE", "0")
	assert.Equal(t, 1, textGlyphSupersampleFactorForDebug())

	t.Setenv("PDF_DEBUG_TEXT_GLYPH_SUPERSAMPLE", "9")
	assert.Equal(t, 4, textGlyphSupersampleFactorForDebug())
}

func TestImageCanvas_TextBitmapRenderDPI(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 20, 20)).(*ImageCanvas)

	c.SetGlyphTransform([4]float64{150.0 / 72.0, 0, 0, 150.0 / 72.0})
	dpi, ok := c.textBitmapRenderDPI()
	require.True(t, ok)
	assert.Equal(t, 150, dpi)

	c.SetGlyphTransform([4]float64{2, 0.1, 0, 2})
	_, ok = c.textBitmapRenderDPI()
	assert.False(t, ok)

	c.SetGlyphTransform([4]float64{2, 0, 0, 2.2})
	_, ok = c.textBitmapRenderDPI()
	assert.False(t, ok)
}

func TestPopplerGlyphXPhase(t *testing.T) {
	assert.Equal(t, 0.0, popplerGlyphXPhase(10.249))
	assert.Equal(t, 0.25, popplerGlyphXPhase(10.25))
	assert.Equal(t, 0.75, popplerGlyphXPhase(10.999))
	assert.Equal(t, 0.75, popplerGlyphXPhase(-0.1))
}

func TestImageCanvas_DrawText_UsesBitmapRendererForAxisAlignedType1LikeFont(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 20, 20)).(*ImageCanvas)
	c.SetGlyphTransform([4]float64{2, 0, 0, 2})
	c.SetFillColor(color.Black)

	font := &mockBitmapTextFont{}
	err := c.DrawText("A", 10, 10, font, 12)
	require.NoError(t, err)

	assert.Equal(t, 1, font.bitmapCalls)
	assert.Equal(t, 12.0, font.lastSizePt)
	assert.Equal(t, 144, font.lastDPI)
	assert.Zero(t, font.pathCalls)
	assert.Equal(t, uint8(255), c.img.RGBAAt(10, 9).A)
}

func TestImageCanvas_RenderGlyphBitmap_AppliesColorTransferAfterBlend(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 1, 1)).(*ImageCanvas)
	c.img.SetRGBA(0, 0, color.RGBA{255, 255, 255, 255})

	var red, green, blue, gray [256]uint8
	for i := 0; i < 256; i++ {
		red[i] = uint8(i)
		green[i] = uint8(i)
		blue[i] = uint8(i)
		gray[i] = uint8(i)
	}
	blue[206] = 207
	c.SetColorTransfer(red, green, blue, gray, true)

	c.renderGlyphBitmap([]byte{107}, 1, 1, 0, 1, 0, 0, color.RGBA{236, 0, 140, 255})

	assert.Equal(t, color.RGBA{247, 148, 207, 255}, c.img.RGBAAt(0, 0))
}

func TestImageCanvas_RenderGlyphBitmap_UsesPopplerPaperCompositeRounding(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 1, 1)).(*ImageCanvas)
	c.SetOpaquePaperBackground(color.White)

	c.renderGlyphBitmap([]byte{107}, 1, 1, 0, 1, 0, 0, color.RGBA{236, 0, 140, 255})

	assert.Equal(t, color.RGBA{247, 148, 207, 255}, c.img.RGBAAt(0, 0))
}

func TestImageCanvas_RasterizeGlyphMask_TwoXSupersampleBeatsDirectAgainstFourXReference(t *testing.T) {
	drawCmds := []glyphDrawCommand{
		{kind: entity.CmdMoveTo, x: 10.2, y: 10.1},
		{kind: entity.CmdLineTo, x: 20.7, y: 10.4},
		{kind: entity.CmdLineTo, x: 20.1, y: 20.8},
		{kind: entity.CmdCurveTo, x: 10.2, y: 20.1, c1x: 17.8, c1y: 22.2, c2x: 12.4, c2y: 21.7},
		{kind: entity.CmdClose},
	}
	dstRect := image.Rect(10, 10, 21, 21)

	// Use goVectorStrategy directly: supersample quality improvement is only meaningful
	// for the Go vector rasterizer. Cairo ignores supersample (it's always full quality).
	s := &goVectorStrategy{}
	direct := s.RasterizeGlyphMask(drawCmds, dstRect, 10.2, 10.1, 1)
	twoX := s.RasterizeGlyphMask(drawCmds, dstRect, 10.2, 10.1, 2)
	ref := s.RasterizeGlyphMask(drawCmds, dstRect, 10.2, 10.1, 4)

	directRatio := alphaDiffRatioAgainstReferenceForCanvasTest(direct, ref)
	twoXRatio := alphaDiffRatioAgainstReferenceForCanvasTest(twoX, ref)

	assert.Greater(t, directRatio, 0.0)
	assert.Less(t, twoXRatio, directRatio)
}

type mockTextFont struct {
	rendered  []uint32
	pathCalls int
}

func (m *mockTextFont) CharCodeToGlyph(code uint32) (uint32, error) {
	m.rendered = append(m.rendered, code)
	return code, nil
}

func (m *mockTextFont) GlyphName(glyph uint32) string               { return "" }
func (m *mockTextFont) GetGlyphWidth(glyph uint32) (float64, error) { return 1000, nil }
func (m *mockTextFont) GetBoundingBox() (float64, float64, float64, float64) {
	return 0, 0, 1000, 1000
}
func (m *mockTextFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	m.pathCalls++
	return &entity.GlyphPath{
		Commands: []entity.PathCommand{
			&entity.PathMoveTo{X: 0, Y: 0},
			&entity.PathLineTo{X: 1, Y: 0},
			&entity.PathLineTo{X: 1, Y: 1},
			&entity.PathClose{},
		},
	}, nil
}
func (m *mockTextFont) IsCIDFont() bool    { return false }
func (m *mockTextFont) IsSymbolic() bool   { return false }
func (m *mockTextFont) UnitsPerEm() uint16 { return 1000 }
func (m *mockTextFont) Name() string       { return "mock" }

type mockTextCIDFont struct{}

func (m *mockTextCIDFont) CharCodeToGlyph(code uint32) (uint32, error) {
	return code, nil
}

func (m *mockTextCIDFont) GlyphName(glyph uint32) string               { return "" }
func (m *mockTextCIDFont) GetGlyphWidth(glyph uint32) (float64, error) { return 1000, nil }
func (m *mockTextCIDFont) GetBoundingBox() (float64, float64, float64, float64) {
	return 0, 0, 1000, 1000
}
func (m *mockTextCIDFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return nil, nil
}
func (m *mockTextCIDFont) IsCIDFont() bool    { return true }
func (m *mockTextCIDFont) IsSymbolic() bool   { return false }
func (m *mockTextCIDFont) UnitsPerEm() uint16 { return 1000 }
func (m *mockTextCIDFont) Name() string       { return "mock-cid" }

type mockBitmapTextFont struct {
	mockTextFont
	bitmapCalls int
	lastSizePt  float64
	lastDPI     int
}

func (m *mockBitmapTextFont) RenderGlyphBitmap(glyph uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error) {
	m.bitmapCalls++
	m.lastSizePt = sizePt
	m.lastDPI = dpi
	return []byte{255}, 1, 1, 0, 1, nil
}

func alphaDiffRatioAgainstReferenceForCanvasTest(left, right *image.Alpha) float64 {
	diff := 0.0
	refSum := 0.0
	bounds := left.Bounds().Union(right.Bounds())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			leftA := float64(alphaAtForCanvasTest(left, x, y))
			rightA := float64(alphaAtForCanvasTest(right, x, y))
			if leftA > rightA {
				diff += leftA - rightA
			} else {
				diff += rightA - leftA
			}
			refSum += rightA
		}
	}
	if refSum == 0 {
		return 0
	}
	return diff / refSum
}

func alphaAtForCanvasTest(img *image.Alpha, x, y int) uint8 {
	if !image.Pt(x, y).In(img.Bounds()) {
		return 0
	}
	return img.AlphaAt(x, y).A
}

type constantRGBFunction struct{}

func (f *constantRGBFunction) Evaluate(inputs []float64) ([]float64, error) {
	return []float64{0.2, 0.4, 0.6}, nil
}

func (f *constantRGBFunction) GetInputSize() int { return 1 }

func (f *constantRGBFunction) GetOutputSize() int { return 3 }

func (f *constantRGBFunction) GetDomain() [][2]float64 { return [][2]float64{{0, 1}} }

func (f *constantRGBFunction) GetRange() [][2]float64 { return [][2]float64{{0, 1}, {0, 1}, {0, 1}} }

type scalarFunction struct {
	value float64
}

func (f *scalarFunction) Evaluate(inputs []float64) ([]float64, error) {
	return []float64{f.value}, nil
}

func (f *scalarFunction) GetInputSize() int { return 1 }

func (f *scalarFunction) GetOutputSize() int { return 1 }

func (f *scalarFunction) GetDomain() [][2]float64 { return [][2]float64{{0, 1}} }

func (f *scalarFunction) GetRange() [][2]float64 { return [][2]float64{{0, 1}} }

func TestEvaluateShadingColorFunctionsUsesFunctionArrayComponents(t *testing.T) {
	colors, err := evaluateShadingColorFunctions([]entity.Function{
		&scalarFunction{value: 0.25},
		&scalarFunction{value: 0.50},
		&scalarFunction{value: 0.75},
	}, []float64{0.3})

	require.NoError(t, err)
	assert.Equal(t, []float64{0.25, 0.50, 0.75}, colors)
}

func TestImageCanvas_LineClipAndColorConversionBranches(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 5, 5)).(*ImageCanvas)
	c.drawLine(1, 1, color.RGBA{255, 0, 0, 255})
	assert.Equal(t, uint8(255), c.img.RGBAAt(1, 3).R)
	c.drawLine(-1, -1, color.RGBA{255, 255, 255, 255})

	c.setPixelWithClip(2, 2, color.RGBA{0, 255, 0, 255})
	assert.Equal(t, uint8(255), c.img.RGBAAt(2, 2).G)

	c.clipMask = image.NewAlpha(image.Rect(0, 0, 5, 5))
	c.setPixelWithClip(0, 0, color.RGBA{255, 255, 255, 255})
	assert.Equal(t, uint8(0), c.img.RGBAAt(0, 4).A)

	c.clipMask.SetAlpha(1, 3, color.Alpha{A: 128})
	c.setPixelWithClip(1, 1, color.RGBA{100, 50, 25, 255})
	got := c.img.RGBAAt(1, 3)
	assert.Greater(t, got.A, uint8(0))
	assert.Greater(t, got.R, uint8(0))

	gray := c.colorArrayToRGBA([]float64{0.5})
	assert.Equal(t, gray.R, gray.G)
	assert.Equal(t, gray.G, gray.B)
	assert.Equal(t, color.RGBA{128, 128, 128, 255}, gray)

	rgb := c.colorArrayToRGBA([]float64{1, 0, 0})
	assert.Equal(t, color.RGBA{255, 0, 0, 255}, rgb)

	roundedRGB := c.colorArrayToRGBA([]float64{0.5, 0.5, 0.5})
	assert.Equal(t, color.RGBA{128, 128, 128, 255}, roundedRGB)
	assert.Equal(t, uint8(42), colorComponentToByte(42.5005/255.0))

	cmyk := c.colorArrayToRGBA([]float64{0, 0, 0, 0})
	assert.Equal(t, color.RGBA{255, 255, 255, 255}, cmyk)

	cmykMagenta := c.colorArrayToRGBA([]float64{0, 1, 0, 0})
	assert.Equal(t, color.RGBA{236, 0, 140, 255}, cmykMagenta)

	def := c.colorArrayToRGBA([]float64{})
	assert.Equal(t, color.RGBA{0, 0, 0, 255}, def)
}

func TestImageCanvas_ClipIntersectionKeepsIdenticalEdgeCoverage(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)

	c.Rectangle(0.25, 0.25, 4.5, 4.5)
	c.Clip()

	var sample image.Point
	var firstAlpha uint8
	found := false
	for y := 0; y < c.height && !found; y++ {
		for x := 0; x < c.width; x++ {
			alpha := c.clipMask.AlphaAt(x, y).A
			if alpha > 0 && alpha < 255 {
				sample = image.Point{X: x, Y: y}
				firstAlpha = alpha
				found = true
				break
			}
		}
	}
	require.True(t, found)

	c.Rectangle(0.25, 0.25, 4.5, 4.5)
	c.Clip()

	assert.Equal(t, firstAlpha, c.clipMask.AlphaAt(sample.X, sample.Y).A)
}

func TestImageCanvas_ClipIntersectionKeepsSingleCoverageForDifferentPaths(t *testing.T) {
	buildTriangleClip := func(c *ImageCanvas, offsetX float64) {
		c.MoveTo(0.25+offsetX, 0.25)
		c.LineTo(5.75+offsetX, 0.25)
		c.LineTo(0.25+offsetX, 5.75)
		c.ClosePath()
		c.Clip()
	}

	first := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)
	buildTriangleClip(first, 0)
	require.Nil(t, first.clipRect)

	second := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)
	buildTriangleClip(second, 0.5)
	require.Nil(t, second.clipRect)

	combined := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)
	buildTriangleClip(combined, 0)
	buildTriangleClip(combined, 0.5)
	require.Nil(t, combined.clipRect)

	foundPartialIntersection := false
	for y := 0; y < combined.height; y++ {
		for x := 0; x < combined.width; x++ {
			firstAlpha := first.clipMask.AlphaAt(x, y).A
			secondAlpha := second.clipMask.AlphaAt(x, y).A
			actual := combined.clipMask.AlphaAt(x, y).A
			expected := intersectClipMaskAlpha(firstAlpha, secondAlpha)
			assert.Equalf(t, expected, actual, "pixel (%d,%d)", x, y)

			minAlpha := firstAlpha
			if secondAlpha < minAlpha {
				minAlpha = secondAlpha
			}
			if firstAlpha > 0 && firstAlpha < 255 && secondAlpha > 0 && secondAlpha < 255 {
				assert.Equalf(t, minAlpha, expected, "partial pixel (%d,%d)", x, y)
				foundPartialIntersection = true
			}
		}
	}

	require.True(t, foundPartialIntersection)
}

func TestImageCanvas_TilingPatternAndPatternXRefBranches(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)

	pattern := entity.NewTilingPattern("tile", 1, entity.TilingConstantSpacing)
	pattern.SetBBox([4]float64{0, 0, 1, 1})
	pattern.SetXStep(1)
	pattern.SetYStep(1)
	pattern.SetResources(entity.NewDict())

	c.SetFillPattern(pattern)
	c.SetStrokePattern(pattern)

	err := c.DrawTilingPattern(pattern, [4]float64{0, 0, 2, 2})
	require.NoError(t, err)

	xref := &patternXRef{}
	_, err = xref.Fetch(entity.NewRef(1, 0))
	require.Error(t, err)
	assert.Equal(t, 0, xref.GetNumObjects())
	require.NoError(t, xref.Parse())
	assert.Nil(t, xref.GetTrailer())
	_, err = xref.GetCatalog()
	require.Error(t, err)
}

func TestImageCanvas_DrawTilingPatternTranslatesNonZeroPatternBBox(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)

	pattern := entity.NewTilingPattern("tile", 1, entity.TilingConstantSpacing)
	pattern.SetBBox([4]float64{10, 20, 11, 21})
	pattern.SetXStep(1)
	pattern.SetYStep(1)
	pattern.SetResources(entity.NewDict())
	pattern.SetContent([]byte("0 0 0 rg 10 20 1 1 re f"))

	err := c.DrawTilingPattern(pattern, [4]float64{0, 0, 2, 2})
	require.NoError(t, err)
	assert.Greater(t, countNonTransparentPixels(c.Image().(*image.RGBA)), 0)
}

func TestImageCanvas_DrawTilingPatternRejectsOversizedCell(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)

	pattern := entity.NewTilingPattern("tile", 1, entity.TilingConstantSpacing)
	pattern.SetBBox([4]float64{0, 0, 100000, 100000})
	pattern.SetXStep(1)
	pattern.SetYStep(1)

	err := c.DrawTilingPattern(pattern, [4]float64{0, 0, 1, 1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pattern cell")
}

func TestImageCanvas_ShadingPatternBranches(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)
	bbox := [4]float64{0, 0, 4, 4}

	err := c.DrawShadingPattern(entity.NewShadingPattern("none", nil), bbox)
	require.Error(t, err)

	unsupported := entity.NewShading(entity.ShadingType(99), "DeviceRGB")
	err = c.DrawShadingPattern(entity.NewShadingPattern("bad", unsupported), bbox)
	require.Error(t, err)

	axial := entity.NewShading(entity.ShadingAxial, "DeviceRGB")
	err = c.drawAxialShading(axial, bbox)
	require.Error(t, err)
	axial.SetCoords([]float64{0, 0, 4, 0})
	axial.SetFunctions([]entity.Function{&constantRGBFunction{}})
	axial.SetExtend([2]bool{true, true})
	require.NoError(t, c.DrawShadingPattern(entity.NewShadingPattern("axial", axial), bbox))

	radial := entity.NewShading(entity.ShadingRadial, "DeviceRGB")
	err = c.drawRadialShading(radial, bbox)
	require.Error(t, err)
	radial.SetCoords([]float64{1, 1, 0, 3, 3, 2})
	radial.SetFunctions([]entity.Function{&constantRGBFunction{}})
	radial.SetExtend([2]bool{true, true})
	require.NoError(t, c.DrawShadingPattern(entity.NewShadingPattern("radial", radial), bbox))

	functionBased := entity.NewShading(entity.ShadingFunctionBased, "DeviceRGB")
	functionBased.SetDomain([4]float64{0, 4, 0, 4})
	functionBased.SetMatrix([6]float64{1, 0, 0, 1, 0, 0})
	err = c.drawFunctionBasedShading(functionBased, bbox)
	require.Error(t, err)
	functionBased.SetFunctions([]entity.Function{&constantRGBFunction{}})
	require.NoError(t, c.DrawShadingPattern(entity.NewShadingPattern("func", functionBased), bbox))

	gouraud := entity.NewShading(entity.ShadingFreeFormGouraud, "DeviceRGB")
	err = c.drawGouraudShading(gouraud, bbox)
	require.Error(t, err)
	gouraud.SetVertices([]entity.Vertex{
		entity.NewVertex(0, 0, []float64{1, 0, 0}),
		entity.NewVertex(3, 0, []float64{0, 1, 0}),
		entity.NewVertex(0, 3, []float64{0, 0, 1}),
	})
	require.NoError(t, c.DrawShadingPattern(entity.NewShadingPattern("gouraud", gouraud), bbox))

	patch := entity.NewShading(entity.ShadingCoonsPatch, "DeviceRGB")
	err = c.drawPatchMeshShading(patch, bbox)
	require.Error(t, err)
	patch.SetVertices([]entity.Vertex{
		entity.NewVertex(0, 0, []float64{1, 0, 0}),
		entity.NewVertex(3, 0, []float64{0, 1, 0}),
		entity.NewVertex(3, 3, []float64{0, 0, 1}),
		entity.NewVertex(0, 3, []float64{1, 1, 0}),
	})
	require.NoError(t, c.DrawShadingPattern(entity.NewShadingPattern("patch", patch), bbox))
}

func TestImageCanvas_DrawShadingPatternAppliesPatternMatrixToMesh(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 12, 12)).(*ImageCanvas)

	shading := entity.NewShading(entity.ShadingFreeFormGouraud, "DeviceRGB")
	shading.SetVertices([]entity.Vertex{
		entity.NewVertex(0, 0, []float64{1, 0, 0}),
		entity.NewVertex(3, 0, []float64{1, 0, 0}),
		entity.NewVertex(0, 3, []float64{1, 0, 0}),
	})

	pattern := entity.NewShadingPattern("mesh-translate", shading)
	pattern.SetMatrix([6]float64{1, 0, 0, 1, 4, 4})

	require.NoError(t, c.DrawShadingPattern(pattern, [4]float64{0, 0, 12, 12}))

	assert.Equal(t, uint8(0), rgbaAtPDF(c, 1, 1).R)
	assert.Equal(t, uint8(255), rgbaAtPDF(c, 5, 5).R)
}

func TestImageCanvas_DrawShadingPatternRoundsMeshVerticesForScanlineRasterization(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 6, 6)).(*ImageCanvas)

	shading := entity.NewShading(entity.ShadingFreeFormGouraud, "DeviceRGB")
	shading.SetVertices([]entity.Vertex{
		entity.NewVertex(0, 0, []float64{1, 0, 0}),
		entity.NewVertex(2, 0, []float64{1, 0, 0}),
		entity.NewVertex(0, 2, []float64{1, 0, 0}),
	})

	pattern := entity.NewShadingPattern("mesh-rounding", shading)
	pattern.SetMatrix([6]float64{1, 0, 0, 1, 0.6, 0.6})

	require.NoError(t, c.DrawShadingPattern(pattern, [4]float64{0, 0, 6, 6}))

	assert.Equal(t, color.RGBA{0, 0, 0, 0}, rgbaAtPDF(c, 0, 0))
	assert.Equal(t, color.RGBA{255, 0, 0, 255}, c.img.RGBAAt(2, 4))
}

func TestImageCanvas_DrawShadingPatternClipsMeshToShadingBBox(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)

	shading := entity.NewShading(entity.ShadingFreeFormGouraud, "DeviceRGB")
	shading.SetVertices([]entity.Vertex{
		entity.NewVertex(0, 0, []float64{1, 0, 0}),
		entity.NewVertex(6, 0, []float64{1, 0, 0}),
		entity.NewVertex(0, 6, []float64{1, 0, 0}),
	})
	shading.SetBBox([4]float64{0, 0, 2, 2})

	pattern := entity.NewShadingPattern("mesh-shading-bbox", shading)

	require.NoError(t, c.DrawShadingPattern(pattern, [4]float64{0, 0, 8, 8}))

	assert.Equal(t, color.RGBA{255, 0, 0, 255}, rgbaAtPDF(c, 1, 1))
	assert.Equal(t, color.RGBA{0, 0, 0, 0}, rgbaAtPDF(c, 4, 1))
}

func TestImageCanvas_DrawImageInterpolationModes(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			src.SetGray(x, y, color.Gray{Y: uint8((x + y*3) % 255)})
		}
	}

	interpCanvas := NewImageCanvas(image.Rect(0, 0, 7, 7)).(*ImageCanvas)
	require.NoError(t, interpCanvas.DrawImage(src, 0, 0, 7, 7, true))

	nearestCanvas := NewImageCanvas(image.Rect(0, 0, 7, 7)).(*ImageCanvas)
	require.NoError(t, nearestCanvas.DrawImage(src, 0, 0, 7, 7, false))

	var interpSum uint64
	var nearestSum uint64
	diffSum := 0
	diffCount := 0
	for y := 0; y < 7; y++ {
		for x := 0; x < 7; x++ {
			iv := interpCanvas.img.RGBAAt(x, y).R
			nv := nearestCanvas.img.RGBAAt(x, y).R
			interpSum += uint64(iv)
			nearestSum += uint64(nv)
			if iv != nv {
				diffCount++
				diffSum += int(absUint8Diff(iv, nv))
			}
		}
	}

	assert.Greater(t, diffCount, 0)
	assert.NotEqual(t, interpSum, nearestSum)
	assert.Greater(t, diffSum, 0)
}

func TestImageCanvas_DrawImageIdentityPreservesPixels(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 8, 8))
	src.SetGray(4, 4, color.Gray{Y: 255})
	src.SetGray(6, 2, color.Gray{Y: 127})

	c := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)
	require.NoError(t, c.DrawImage(src, 0, 0, 8, 8, true))

	assert.Equal(t, uint8(255), c.img.RGBAAt(4, 4).R)
	assert.Equal(t, uint8(127), c.img.RGBAAt(6, 2).R)
	assert.Equal(t, uint8(0), c.img.RGBAAt(5, 4).R)
	assert.Equal(t, uint8(0), c.img.RGBAAt(4, 5).R)
}

func TestImageCanvas_DrawImageWithSoftMaskAppliesMaskAfterScaling(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		src.SetRGBA(0, y, color.RGBA{R: 255, A: 255})
		src.SetRGBA(1, y, color.RGBA{B: 255, A: 255})
	}

	maskGray := image.NewGray(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		maskGray.SetGray(0, y, color.Gray{Y: 0})
		maskGray.SetGray(1, y, color.Gray{Y: 255})
	}
	mask := infraimage.NewBitmapMaskFromImage(maskGray, false)

	c := NewImageCanvas(image.Rect(0, 0, 1, 1)).(*ImageCanvas)
	require.NoError(t, c.DrawImageWithSoftMaskPhaseSamplerAndEdgeMode(
		src,
		mask,
		0,
		0,
		1,
		1,
		false,
		"box",
		0,
		0,
		"",
	))

	assert.Equal(t, color.RGBA{R: 63, B: 63, A: 127}, c.img.RGBAAt(0, 0))
}

func TestImageCanvas_DrawImageUnitSquareTransformMatchesSplashBilinear(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 4, 4))
	src.Pix = []uint8{
		0, 63, 127, 255,
		255, 127, 63, 0,
		0, 255, 0, 255,
		255, 0, 255, 0,
	}

	actual := NewImageCanvas(image.Rect(0, 0, 4, 4)).(*ImageCanvas)
	actual.Transform([6]float64{4, 0, 0, 4, 0, 0})
	require.NoError(t, actual.DrawImage(src, 0, 0, 1, 1, true))

	// Expected values from Poppler's scaleImageYupXupBilinear for this source+transform.
	popplerTarget := image.NewRGBA(image.Rect(0, 0, 4, 4))
	rows := [][]uint8{
		{0, 50, 101, 178},
		{204, 131, 90, 65},
		{101, 183, 95, 76},
		{102, 142, 121, 122},
	}
	for y := range rows {
		for x, v := range rows[y] {
			popplerTarget.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}

	assert.Equal(t, 0, countDifferentPixels(actual.img, popplerTarget))
}

func TestImageCanvas_DrawImageSplashBilinearMatchesPopplerTarget(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 4, 4))
	src.Pix = []uint8{
		0, 63, 127, 255,
		255, 127, 63, 0,
		0, 255, 0, 255,
		255, 0, 255, 0,
	}

	popplerTarget := image.NewRGBA(image.Rect(0, 0, 4, 4))
	rows := [][]uint8{
		{0, 50, 101, 178},
		{204, 131, 90, 65},
		{101, 183, 95, 76},
		{102, 142, 121, 122},
	}
	for y := range rows {
		for x, v := range rows[y] {
			popplerTarget.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}

	current := NewImageCanvas(image.Rect(0, 0, 4, 4)).(*ImageCanvas)
	current.Transform([6]float64{4, 0, 0, 4, 0, 0})
	require.NoError(t, current.DrawImage(src, 0, 0, 1, 1, true))

	// Our Splash bilinear now exactly matches Poppler's output.
	assert.Equal(t, 0, sumAbsoluteRGBDiff(current.img, popplerTarget))
}

func TestImageCanvas_DrawImageSkewedTransformUsesFullAffine(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 5, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 5; x++ {
			src.SetRGBA(x, y, color.RGBA{uint8(32 + x*13), uint8(32 + y*17), 64, 255})
		}
	}

	actual := NewImageCanvas(image.Rect(0, 0, 24, 24)).(*ImageCanvas)
	actual.Transform([6]float64{1, 0.4, 0.35, 1, 2, 3})
	err := actual.DrawImage(src, 1, 2, 5, 4, false)
	require.NoError(t, err)

	legacy := referenceDrawImageWithLegacyAxis(src, actual, 1, 2, 5, 4, false, 0.5, 0.5)

	assert.Greater(t, countNonTransparentPixels(actual.Image().(*image.RGBA)), 0)
	assert.NotZero(t, countDifferentPixels(actual.Image().(*image.RGBA), legacy))
}

func TestImageCanvas_DrawImageWithRotateAndSkew(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	src.SetRGBA(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	src.SetRGBA(1, 0, color.RGBA{R: 0, G: 255, B: 0, A: 255})
	src.SetRGBA(0, 1, color.RGBA{R: 0, G: 0, B: 255, A: 255})
	src.SetRGBA(1, 1, color.RGBA{R: 255, G: 255, B: 0, A: 255})

	c := NewImageCanvas(image.Rect(0, 0, 5, 5)).(*ImageCanvas)
	c.Transform([6]float64{0, 1, -1, 0, 2, 1})
	err := c.DrawImageWithPhase(src, 0, 0, 2, 2, false, 0, 0)
	require.NoError(t, err)

	opaquePoints := nonTransparentPoints(c.img)
	assert.Equal(t, 4, len(opaquePoints))
	assert.Len(t, colorSetFromPixels(c.img, opaquePoints), 4)
}

func TestImageCanvas_DrawImage_ClipWithRotate(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	src.SetRGBA(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	src.SetRGBA(1, 0, color.RGBA{R: 0, G: 255, B: 0, A: 255})
	src.SetRGBA(0, 1, color.RGBA{R: 0, G: 0, B: 255, A: 255})
	src.SetRGBA(1, 1, color.RGBA{R: 255, G: 255, B: 0, A: 255})

	cBase := NewImageCanvas(image.Rect(0, 0, 5, 5)).(*ImageCanvas)
	cBase.Transform([6]float64{0, 1, -1, 0, 2, 1})
	require.NoError(t, cBase.DrawImageWithPhase(src, 0, 0, 2, 2, false, 0, 0))
	basePoints := nonTransparentPoints(cBase.img)
	require.NotEmpty(t, basePoints)
	pivot := basePoints[0]

	c := NewImageCanvas(image.Rect(0, 0, 5, 5)).(*ImageCanvas)
	c.Transform([6]float64{0, 1, -1, 0, 2, 1})
	c.clipMask = image.NewAlpha(c.Bounds())
	for y := 0; y < c.height; y++ {
		for x := 0; x < c.width; x++ {
			c.clipMask.SetAlpha(x, y, color.Alpha{A: 0})
		}
	}
	c.clipMask.SetAlpha(pivot.X, pivot.Y, color.Alpha{A: 255})

	err := c.DrawImageWithPhase(src, 0, 0, 2, 2, false, 0, 0)
	require.NoError(t, err)

	assert.Equal(t, color.RGBA{cBase.img.RGBAAt(pivot.X, pivot.Y).R, cBase.img.RGBAAt(pivot.X, pivot.Y).G, cBase.img.RGBAAt(pivot.X, pivot.Y).B, cBase.img.RGBAAt(pivot.X, pivot.Y).A}, c.img.RGBAAt(pivot.X, pivot.Y))
	assert.Equal(t, 1, countNonTransparentPixels(c.img))
}

func TestImageCanvas_DrawImageNearestScaleFastPathMatchesAffineReference(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			src.SetRGBA(x, y, color.RGBA{
				R: uint8(x * 11),
				G: uint8(y * 9),
				B: uint8((x + y) * 7),
				A: 255,
			})
		}
	}

	actual := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)
	err := actual.DrawImageWithPhaseAndSampler(
		src,
		0,
		0,
		8,
		8,
		false,
		"auto_downscale_nearest",
		0,
		0,
	)
	require.NoError(t, err)

	reference := referenceDrawImageWithFullAffine(
		src,
		actual,
		0,
		0,
		8,
		8,
		false,
		0,
		0,
	)

	assert.Equal(t, 0, countDifferentPixels(actual.img, reference))
}

func TestImageCanvas_DrawImageBoxDownscalePreservesSparseSignal(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 16, 16))
	points := [][2]int{
		{3, 3}, {12, 3},
		{7, 5}, {7, 6}, {7, 7}, {7, 8},
		{3, 10}, {11, 10},
		{3, 11}, {10, 11}, {11, 11},
		{4, 12}, {5, 12}, {6, 12}, {7, 12}, {8, 12}, {9, 12}, {10, 12},
	}
	for _, pt := range points {
		src.SetGray(pt[0], pt[1], color.Gray{Y: 255})
	}

	c := NewImageCanvas(image.Rect(0, 0, 4, 4)).(*ImageCanvas)
	err := c.DrawImageWithPhaseAndSampler(
		src,
		0,
		0,
		4,
		4,
		true,
		"auto_box_tiny_iccbased_gray_downscale",
		0.5,
		0.5,
	)
	require.NoError(t, err)

	expected := [4][4]uint8{
		{15, 0, 0, 15},
		{0, 47, 0, 0},
		{31, 15, 47, 0},
		{0, 63, 47, 0},
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			assert.Equal(t, expected[y][x], c.img.RGBAAt(x, y).R, "pixel (%d,%d)", x, y)
		}
	}
}

func TestImageCanvas_DrawImageBoxDownscaleHonorsClipMask(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 16, 16))
	for x := 4; x <= 10; x++ {
		src.SetGray(x, 12, color.Gray{Y: 255})
	}

	c := NewImageCanvas(image.Rect(0, 0, 4, 4)).(*ImageCanvas)
	c.clipMask = image.NewAlpha(c.Bounds())
	c.clipMask.SetAlpha(1, 3, color.Alpha{A: 255})

	err := c.DrawImageWithPhaseAndSampler(
		src,
		0,
		0,
		4,
		4,
		true,
		"auto_box_tiny_iccbased_gray_downscale",
		0.5,
		0.5,
	)
	require.NoError(t, err)

	assert.Equal(t, uint8(63), c.img.RGBAAt(1, 3).R)
	assert.Equal(t, 1, countNonTransparentPixels(c.img))
}

func TestImageCanvas_DrawImageBoxDownscale16To8UsesExact2x2Averages(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 16, 16))

	setBlock := func(blockX, blockY int, values [4]uint8) {
		src.SetGray(blockX*2+0, blockY*2+0, color.Gray{Y: values[0]})
		src.SetGray(blockX*2+1, blockY*2+0, color.Gray{Y: values[1]})
		src.SetGray(blockX*2+0, blockY*2+1, color.Gray{Y: values[2]})
		src.SetGray(blockX*2+1, blockY*2+1, color.Gray{Y: values[3]})
	}

	setBlock(0, 0, [4]uint8{255, 255, 255, 255})
	setBlock(1, 0, [4]uint8{255, 0, 0, 0})
	setBlock(2, 0, [4]uint8{255, 255, 0, 0})
	setBlock(3, 0, [4]uint8{255, 255, 255, 0})
	setBlock(4, 0, [4]uint8{10, 20, 30, 40})

	c := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)
	c.drawAxisAlignedBoxDownscale(src, src.Bounds(), image.Rect(0, 0, 8, 8))

	expected := []uint8{
		255,
		63,
		127,
		191,
		25,
	}
	for x, want := range expected {
		assert.Equal(t, want, c.img.RGBAAt(x, 0).R, "pixel (%d,%d)", x, 0)
		assert.Equal(t, want, c.img.RGBAAt(x, 0).G, "pixel (%d,%d)", x, 0)
		assert.Equal(t, want, c.img.RGBAAt(x, 0).B, "pixel (%d,%d)", x, 0)
	}
}

func TestImageCanvas_DrawImageBoxDownscale16To8ProducesCorrectBoxAverages(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 16, 16))
	copy(src.Pix, []uint8{
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 255, 0, 0, 0, 0, 0, 0, 0, 0, 255, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 255, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 255, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 255, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 255, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 255, 0, 0, 0, 0, 0, 0, 0, 255, 0, 0, 0, 0,
		0, 0, 0, 255, 0, 0, 0, 0, 0, 0, 255, 255, 0, 0, 0, 0,
		0, 0, 0, 0, 255, 255, 255, 255, 255, 255, 255, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0,
	})

	current := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)
	current.drawAxisAlignedBoxDownscale(src, src.Bounds(), image.Rect(0, 0, 8, 8))

	// Expected: exact 2×2 box averages of the source image.
	expected := [8][8]uint8{
		{0, 0, 0, 0, 0, 0, 0, 0},
		{0, 63, 0, 0, 0, 0, 63, 0},
		{0, 0, 0, 63, 0, 0, 0, 0},
		{0, 0, 0, 127, 0, 0, 0, 0},
		{0, 0, 0, 63, 0, 0, 0, 0},
		{0, 127, 0, 0, 0, 191, 0, 0},
		{0, 0, 127, 127, 127, 63, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0},
	}
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			got := current.img.RGBAAt(x, y)
			assert.Equal(t, expected[y][x], got.R, "pixel (%d,%d) R", x, y)
			assert.Equal(t, expected[y][x], got.G, "pixel (%d,%d) G", x, y)
			assert.Equal(t, expected[y][x], got.B, "pixel (%d,%d) B", x, y)
			assert.Equal(t, uint8(255), got.A, "pixel (%d,%d) A", x, y)
		}
	}
}

func TestImageCanvas_CanUseAxisAlignedBoxDownscaleFastPath_SkipsTinyGray8x8Only(t *testing.T) {
	c := NewImageCanvas(image.Rect(0, 0, 16, 16)).(*ImageCanvas)
	srcBounds := image.Rect(0, 0, 16, 16)

	assert.True(t, c.canUseAxisAlignedBoxDownscaleFastPath(
		"auto_box_tiny_iccbased_gray_downscale",
		0,
		0,
		4,
		0,
		0,
		4,
		srcBounds,
		image.Rect(0, 0, 4, 4),
		0,
		0,
	))

	// 16→8 tiny ICC gray still uses the generic affine path because the real
	// 007 sample regresses when forced through exact 2x2 box averages.
	assert.False(t, c.canUseAxisAlignedBoxDownscaleFastPath(
		"auto_box_tiny_iccbased_gray_downscale",
		0,
		0,
		8,
		0,
		0,
		8,
		srcBounds,
		image.Rect(0, 0, 8, 8),
		0,
		0,
	))
}

func TestImageCanvas_DrawImageBoxDownscale16To8MatchesPopplerReference(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 16, 16))
	points := [][2]int{
		{3, 3}, {12, 3},
		{7, 5}, {7, 6}, {7, 7}, {7, 8},
		{3, 10}, {11, 10},
		{3, 11}, {10, 11}, {11, 11},
		{4, 12}, {5, 12}, {6, 12}, {7, 12}, {8, 12}, {9, 12}, {10, 12},
	}
	for _, pt := range points {
		src.SetGray(pt[0], pt[1], color.Gray{Y: 255})
	}

	actual := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)
	err := actual.DrawImageWithPhaseAndSampler(
		src,
		0,
		0,
		8,
		8,
		true,
		"auto_box_tiny_iccbased_gray_downscale",
		0.5,
		0.5,
	)
	require.NoError(t, err)

	// Build the expected Poppler-matching result using popplerSourceRange1D.
	expected := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for dy := 0; dy < 8; dy++ {
		rowStart, rowEnd := popplerSourceRange1D(dy, 8, 16)
		for dx := 0; dx < 8; dx++ {
			colStart, colEnd := popplerSourceRange1D(dx, 8, 16)
			count := (rowEnd - rowStart) * (colEnd - colStart)
			var sum uint64
			for sy := rowStart; sy < rowEnd; sy++ {
				for sx := colStart; sx < colEnd; sx++ {
					r, _, _, _ := rgba8Components(src.At(sx, sy))
					sum += uint64(r)
				}
			}
			v := uint8(sum / uint64(count))
			expected.SetRGBA(dx, dy, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}

	assert.Equal(t, 0, countDifferentPixels(actual.img, expected))
}

func TestImageCanvas_DrawImagePopplerStyle2xBoxAverages8BitSamples(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 16, 16))
	src.SetGray(5, 12, color.Gray{Y: 255})
	src.SetGray(6, 12, color.Gray{Y: 255})
	src.SetGray(5, 13, color.Gray{Y: 1})

	actual := NewImageCanvas(image.Rect(0, 0, 8, 8)).(*ImageCanvas)
	err := actual.DrawImageWithPhaseAndSampler(
		src,
		0,
		0,
		8,
		8,
		true,
		"auto_box_tiny_iccbased_gray_downscale",
		0.5,
		0.5,
	)
	require.NoError(t, err)

	// Poppler accumulates 8-bit Guchar samples and truncates the integer average.
	assert.Equal(t, color.RGBA{R: 127, G: 127, B: 127, A: 255}, actual.img.RGBAAt(3, 7))
}

func TestImageCanvas_DrawImageSplashDownscaleUsesBresenhamAreaAverage(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 5, 5))
	for y := 0; y < 5; y++ {
		for x := 0; x < 5; x++ {
			src.SetGray(x, y, color.Gray{Y: uint8(y*50 + x)})
		}
	}

	actual := NewImageCanvas(image.Rect(0, 0, 3, 3)).(*ImageCanvas)
	actual.drawAxisAlignedSplashDownscale(src, src.Bounds(), 0, 0, 2.1, 2.1)

	expected := [3][3]uint8{
		{0, 1, 3},
		{75, 76, 78},
		{175, 176, 178},
	}
	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			got := actual.img.RGBAAt(x, y)
			assert.Equal(t, expected[y][x], got.R, "pixel (%d,%d) R", x, y)
			assert.Equal(t, expected[y][x], got.G, "pixel (%d,%d) G", x, y)
			assert.Equal(t, expected[y][x], got.B, "pixel (%d,%d) B", x, y)
			assert.Equal(t, uint8(255), got.A, "pixel (%d,%d) A", x, y)
		}
	}
}

func TestImageCanvas_DrawImageSplashDownscaleKeepsOpaqueSourceAlpha(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 7, 7))
	for y := 0; y < 7; y++ {
		for x := 0; x < 7; x++ {
			src.SetRGBA(x, y, color.RGBA{A: 255})
		}
	}

	actual := NewImageCanvas(image.Rect(0, 0, 3, 3)).(*ImageCanvas)
	actual.drawAxisAlignedSplashDownscale(src, src.Bounds(), 0, 0, 2.1, 2.1)

	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			got := actual.img.RGBAAt(x, y)
			assert.Equal(t, color.RGBA{A: 255}, got, "pixel (%d,%d)", x, y)
		}
	}
}

func TestDrawLineSegmentUsesPopplerAAGammaCoverage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 5, 5))
	drawLineSegment(img, 0, 0, 5, 5, color.RGBA{A: 255}, 1, 1)

	allowedAlpha := make(map[uint8]bool, len(splashAAGamma))
	for _, alpha := range splashAAGamma {
		allowedAlpha[alpha] = true
	}

	partialPixels := 0
	for y := 0; y < 5; y++ {
		for x := 0; x < 5; x++ {
			alpha := img.RGBAAt(x, y).A
			if alpha == 0 || alpha == 255 {
				continue
			}
			partialPixels++
			assert.True(t, allowedAlpha[alpha], "pixel (%d,%d) alpha=%d", x, y, alpha)
		}
	}
	assert.Greater(t, partialPixels, 0)
}

func TestSplashCompositeOverUsesRoundedDiv255(t *testing.T) {
	dst := color.RGBA{R: 1, G: 1, B: 1, A: 255}
	src := color.RGBA{A: 127}

	got := splashCompositeOver(dst, src)

	assert.Equal(t, color.RGBA{R: 1, G: 1, B: 1, A: 255}, got)
	assert.Equal(t, color.RGBA{R: 0, G: 0, B: 0, A: 255}, compositeOver(dst, src))
}

func absUint8Diff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func countDifferentPixels(a, b *image.RGBA) int {
	if !a.Bounds().Eq(b.Bounds()) {
		return -1
	}
	diffCount := 0
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			av := a.RGBAAt(x, y)
			bv := b.RGBAAt(x, y)
			if av != bv {
				diffCount++
			}
		}
	}
	return diffCount
}

func sumAbsoluteRGBDiff(a, b *image.RGBA) int {
	if !a.Bounds().Eq(b.Bounds()) {
		return int(^uint(0) >> 1)
	}
	diff := 0
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			av := a.RGBAAt(x, y)
			bv := b.RGBAAt(x, y)
			diff += absUint8Diff(av.R, bv.R)
			diff += absUint8Diff(av.G, bv.G)
			diff += absUint8Diff(av.B, bv.B)
		}
	}
	return diff
}

func countNonTransparentPixels(img *image.RGBA) int {
	count := 0
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if img.RGBAAt(x, y).A != 0 {
				count++
			}
		}
	}
	return count
}

func nonTransparentPoints(img *image.RGBA) []image.Point {
	points := make([]image.Point, 0, img.Bounds().Dx()*img.Bounds().Dy())
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if img.RGBAAt(x, y).A != 0 {
				points = append(points, image.Point{X: x, Y: y})
			}
		}
	}
	return points
}

func colorSetFromPixels(img *image.RGBA, points []image.Point) map[color.RGBA]struct{} {
	colors := make(map[color.RGBA]struct{}, len(points))
	for _, p := range points {
		colors[img.RGBAAt(p.X, p.Y)] = struct{}{}
	}
	return colors
}

func referenceDrawImageWithLegacyAxis(
	src image.Image,
	c *ImageCanvas,
	x, y, width, height float64,
	interpolate bool,
	phaseX, phaseY float64,
) *image.RGBA {
	dst := image.NewRGBA(c.Bounds())
	srcBounds := src.Bounds()
	srcW := float64(srcBounds.Dx())
	srcH := float64(srcBounds.Dy())
	if srcW <= 0 || srcH <= 0 {
		return dst
	}

	x0, y0 := c.transformPoint(x, y)
	x1, y1 := c.transformPoint(x+width, y+height)
	minX := math.Min(x0, x1)
	maxX := math.Max(x0, x1)
	minY := math.Min(y0, y1)
	maxY := math.Max(y0, y1)

	top := float64(c.height) - maxY
	bottom := float64(c.height) - minY
	dstMinXf := minX
	dstMaxXf := maxX
	dstMinYf := top
	dstMaxYf := bottom
	dstWidth := dstMaxXf - dstMinXf
	dstHeight := dstMaxYf - dstMinYf
	if dstWidth <= 0 || dstHeight <= 0 {
		return dst
	}

	scaleX := dstWidth / srcW
	scaleY := dstHeight / srcH
	m := f64.Aff3{
		scaleX,
		0,
		dstMinXf + phaseX,
		0,
		scaleY,
		dstMinYf + phaseY,
	}
	if interpolate {
		draw.ApproxBiLinear.Transform(dst, m, src, srcBounds, draw.Over, nil)
		return dst
	}
	draw.NearestNeighbor.Transform(dst, m, src, srcBounds, draw.Over, nil)
	return dst
}

func referenceDrawImageWithFullAffine(
	src image.Image,
	c *ImageCanvas,
	x, y, width, height float64,
	interpolate bool,
	phaseX, phaseY float64,
) *image.RGBA {
	dst := image.NewRGBA(c.Bounds())
	srcBounds := src.Bounds()
	srcWidth := float64(srcBounds.Dx())
	srcHeight := float64(srcBounds.Dy())
	if srcWidth <= 0 || srcHeight <= 0 {
		return dst
	}

	src = flippedImage{src: src}

	p00X, p00Y := c.transformPoint(x, y)
	p10X, p10Y := c.transformPoint(x+width, y)
	p01X, p01Y := c.transformPoint(x, y+height)

	uScaleX := (p10X - p00X) / srcWidth
	uScaleY := (p10Y - p00Y) / srcWidth
	vScaleX := (p01X - p00X) / srcHeight
	vScaleY := (p01Y - p00Y) / srcHeight

	transform := f64.Aff3{
		uScaleX,
		vScaleX,
		p00X + uScaleX*phaseX + vScaleX*phaseY,
		-uScaleY,
		-vScaleY,
		float64(c.height) - p00Y - (uScaleY*phaseX + vScaleY*phaseY),
	}

	if interpolate {
		draw.ApproxBiLinear.Transform(dst, transform, src, srcBounds, draw.Over, nil)
		return dst
	}
	draw.NearestNeighbor.Transform(dst, transform, src, srcBounds, draw.Over, nil)
	return dst
}

func rgbaAtPDF(c *ImageCanvas, x, y int) color.RGBA {
	return c.img.RGBAAt(x, c.height-1-y)
}
