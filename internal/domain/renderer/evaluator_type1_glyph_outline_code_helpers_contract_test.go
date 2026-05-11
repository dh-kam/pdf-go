package renderer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/test/testutil"
)

func TestDistinctBestDensityFontsForProbe_DeduplicatesInEncounterOrder(t *testing.T) {
	result := standardGlyphSourceSelectionSetProbeResult{
		selections: []standardGlyphSourceSelectionProbeResult{
			{code: 47, bestDensityFont: "Helvetica"},
			{code: 65, bestDensityFont: "Courier"},
			{code: 49, bestDensityFont: "Times-Roman"},
			{code: 50, bestDensityFont: "Helvetica"},
		},
	}

	require.Equal(t, []string{"Helvetica", "Courier", "Times-Roman"}, distinctBestDensityFontsForProbe(result))
}

func TestMeasureStandardGlyphSourceSelectionSetSummaryForProbe_TracksDensityAndLowRatioAverages(t *testing.T) {
	result := measureStandardGlyphSourceSelectionSetSummaryForProbe(
		t,
		[]int{47, 65},
		"Times-Roman",
		"Helvetica",
		"Courier",
	)

	require.Equal(t, 2, result.codeCount)
	require.Greater(t, result.timesDensityAvg, 0.0)
	require.Greater(t, result.bestDensityAvg, 0.0)
	require.Greater(t, result.timesLowRatioAvg, 0.0)
	require.Greater(t, result.bestLowRatioAvg, 0.0)
	require.GreaterOrEqual(t, result.maxLowRatioDelta, 0.0)
}

func TestLowRatioSpreadForProbe_ReturnsMaxMinusMin(t *testing.T) {
	result := standardGlyphSourceAlternativesProbeResult{
		sources: []standardGlyphSourceProbeResult{
			{fontName: "Times-Roman", lowRatio: 1.25},
			{fontName: "Helvetica", lowRatio: 1.50},
			{fontName: "Courier", lowRatio: 1.40},
		},
	}

	require.InDelta(t, 0.25, lowRatioSpreadForProbe(result), 0.000001)
	require.Zero(t, lowRatioSpreadForProbe(standardGlyphSourceAlternativesProbeResult{}))
}

func TestHighestLowRatioCodeForProbe_ReturnsLargestLowRatio(t *testing.T) {
	results := []syntheticGlyphCodeOutlineProbeResult{
		{code: 47, glyphName: "/", lowRatio: 1.5},
		{code: 49, glyphName: "1", lowRatio: 7.0},
		{code: 65, glyphName: "A", lowRatio: 3.0},
	}

	highest := highestLowRatioCodeForProbe(t, results)
	require.Equal(t, 49, highest.code)
	require.Equal(t, "1", highest.glyphName)
	require.InDelta(t, 7.0, highest.lowRatio, 0.000001)
}

func TestHighestSegmentsPerWidthCodeForProbe_ReturnsLargestSegmentsPerWidth(t *testing.T) {
	results := []syntheticGlyphCodeOutlineProbeResult{
		{code: 47, glyphName: "/", bounds: [4]float64{0, 0, 20, 100}, complexity: glyphPathComplexitySignature{lines: 4}},
		{code: 49, glyphName: "1", bounds: [4]float64{0, 0, 10, 100}, complexity: glyphPathComplexitySignature{lines: 5}},
		{code: 65, glyphName: "A", bounds: [4]float64{0, 0, 40, 100}, complexity: glyphPathComplexitySignature{lines: 8}},
	}

	highest := highestSegmentsPerWidthCodeForProbe(t, results)
	require.Equal(t, 49, highest.code)
	require.Equal(t, "1", highest.glyphName)
	require.InDelta(t, 0.5, highest.segmentsPerWidth(), 0.000001)
}

func TestHighestVerticalSegmentSpanPerWidthCodeForProbe_ReturnsLargestSpanRatio(t *testing.T) {
	results := []syntheticGlyphCodeOutlineProbeResult{
		{
			code:      47,
			glyphName: "/",
			bounds:    [4]float64{0, 0, 20, 100},
			path: &entity.GlyphPath{Commands: []entity.PathCommand{
				&entity.PathMoveTo{X: 0, Y: 0},
				&entity.PathLineTo{X: 5, Y: 30},
			}},
		},
		{
			code:      49,
			glyphName: "1",
			bounds:    [4]float64{0, 0, 10, 100},
			path: &entity.GlyphPath{Commands: []entity.PathCommand{
				&entity.PathMoveTo{X: 0, Y: 0},
				&entity.PathLineTo{X: 2, Y: 80},
			}},
		},
		{
			code:      58,
			glyphName: ":",
			bounds:    [4]float64{0, 0, 5, 100},
			path: &entity.GlyphPath{Commands: []entity.PathCommand{
				&entity.PathMoveTo{X: 0, Y: 0},
				&entity.PathLineTo{X: 1, Y: 20},
			}},
		},
	}

	highest := highestVerticalSegmentSpanPerWidthCodeForProbe(t, results)
	require.Equal(t, 49, highest.code)
	require.Equal(t, "1", highest.glyphName)
	require.InDelta(t, 8.0, highest.verticalSegmentSpanPerWidth(), 0.000001)
}

func TestHighestVerticalStrokeComplexityCodeForProbe_ReturnsLargestComposite(t *testing.T) {
	results := []syntheticGlyphCodeOutlineProbeResult{
		{
			code:      47,
			glyphName: "/",
			bounds:    [4]float64{0, 0, 20, 100},
			complexity: glyphPathComplexitySignature{
				lines: 4,
			},
			path: &entity.GlyphPath{Commands: []entity.PathCommand{
				&entity.PathMoveTo{X: 0, Y: 0},
				&entity.PathLineTo{X: 5, Y: 30},
			}},
		},
		{
			code:      49,
			glyphName: "1",
			bounds:    [4]float64{0, 0, 10, 100},
			complexity: glyphPathComplexitySignature{
				lines: 5,
			},
			path: &entity.GlyphPath{Commands: []entity.PathCommand{
				&entity.PathMoveTo{X: 0, Y: 0},
				&entity.PathLineTo{X: 2, Y: 80},
			}},
		},
		{
			code:      58,
			glyphName: ":",
			bounds:    [4]float64{0, 0, 5, 100},
			complexity: glyphPathComplexitySignature{
				lines: 2,
			},
			path: &entity.GlyphPath{Commands: []entity.PathCommand{
				&entity.PathMoveTo{X: 0, Y: 0},
				&entity.PathLineTo{X: 1, Y: 20},
			}},
		},
	}

	highest := highestVerticalStrokeComplexityCodeForProbe(t, results)
	require.Equal(t, 49, highest.code)
	require.Equal(t, "1", highest.glyphName)
	require.InDelta(t, 40.0, highest.verticalStrokeComplexity(), 0.000001)
}

func TestSyntheticGlyphCodeOutlineProbeResult_SegmentsPerWidth(t *testing.T) {
	result := syntheticGlyphCodeOutlineProbeResult{
		bounds:     [4]float64{10, 0, 20, 100},
		complexity: glyphPathComplexitySignature{lines: 3, curves: 1},
	}

	require.InDelta(t, 0.4, result.segmentsPerWidth(), 0.000001)
	require.Zero(t, syntheticGlyphCodeOutlineProbeResult{complexity: glyphPathComplexitySignature{lines: 2}}.segmentsPerWidth())
}

func TestSyntheticGlyphCodeOutlineProbeResult_VerticalExtent(t *testing.T) {
	result := syntheticGlyphCodeOutlineProbeResult{
		bounds: [4]float64{10, -50, 20, 125},
	}

	require.InDelta(t, 175.0, result.verticalExtent(), 0.000001)
}

func TestSyntheticGlyphCodeOutlineProbeResult_VerticalSegmentSpanPerWidth(t *testing.T) {
	result := syntheticGlyphCodeOutlineProbeResult{
		bounds: [4]float64{10, -50, 20, 125},
		path: &entity.GlyphPath{Commands: []entity.PathCommand{
			&entity.PathMoveTo{X: 10, Y: 0},
			&entity.PathLineTo{X: 12, Y: 80},
			&entity.PathMoveTo{X: 15, Y: 10},
			&entity.PathLineTo{X: 16, Y: 60},
		}},
	}

	require.InDelta(t, 8.0, result.verticalSegmentSpanPerWidth(), 0.000001)
	require.Zero(t, syntheticGlyphCodeOutlineProbeResult{}.verticalSegmentSpanPerWidth())
}

func TestSyntheticGlyphCodeOutlineProbeResult_VerticalStrokeComplexity(t *testing.T) {
	result := syntheticGlyphCodeOutlineProbeResult{
		bounds: [4]float64{10, -50, 20, 125},
		complexity: glyphPathComplexitySignature{
			lines: 2,
			moves: 2,
		},
		path: &entity.GlyphPath{Commands: []entity.PathCommand{
			&entity.PathMoveTo{X: 10, Y: 0},
			&entity.PathLineTo{X: 12, Y: 80},
			&entity.PathMoveTo{X: 15, Y: 10},
			&entity.PathLineTo{X: 16, Y: 60},
		}},
	}

	require.InDelta(t, 16.0, result.verticalStrokeComplexity(), 0.000001)
	require.Zero(t, syntheticGlyphCodeOutlineProbeResult{}.verticalStrokeComplexity())
}

func TestGlyphLineSegmentProbe_AxisSpans(t *testing.T) {
	segment := glyphLineSegmentProbe{fromX: 10, fromY: -20, toX: -5, toY: 40}

	require.InDelta(t, 15.0, segment.absDX(), 0.000001)
	require.InDelta(t, 60.0, segment.absDY(), 0.000001)
}

func TestSegmentSpanHelpers_ReturnLargestAxisSpan(t *testing.T) {
	segments := []glyphLineSegmentProbe{
		{fromX: 0, fromY: 0, toX: 10, toY: 30},
		{fromX: 0, fromY: 0, toX: 25, toY: 5},
		{fromX: 0, fromY: 0, toX: 3, toY: 40},
	}

	require.InDelta(t, 25.0, maxAbsDXForSegments(segments), 0.000001)
	require.InDelta(t, 40.0, maxAbsDYForSegments(segments), 0.000001)
}

func TestSyntheticNonLowerCode49Code58OrderingProbeResult_PrefersCode49AcrossGeometrySignals(t *testing.T) {
	result := syntheticNonLowerCode49Code58OrderingProbeResult{
		code49: syntheticGlyphCodeOutlineProbeResult{
			code:     49,
			lowRatio: 12,
			bounds:   [4]float64{0, 0, 10, 100},
			path: &entity.GlyphPath{Commands: []entity.PathCommand{
				&entity.PathMoveTo{X: 0, Y: 0},
				&entity.PathLineTo{X: 2, Y: 80},
			}},
		},
		code58: syntheticGlyphCodeOutlineProbeResult{
			code:     58,
			lowRatio: 4,
			bounds:   [4]float64{0, 0, 10, 60},
			path: &entity.GlyphPath{Commands: []entity.PathCommand{
				&entity.PathMoveTo{X: 0, Y: 0},
				&entity.PathLineTo{X: 2, Y: 20},
			}},
		},
	}

	require.Equal(t, "code49", result.largerLowRatioCanonicalKey())
	require.Equal(t, "code49", result.largerVerticalExtentCanonicalKey())
	require.Equal(t, "code49", result.largerVerticalSegmentSpanCanonicalKey())
	require.Equal(t, "code49", result.largerVerticalSegmentSpanPerWidthCanonicalKey())
	require.Equal(
		t,
		"code49",
		testutil.NewProbeOrderingAlignment(
			result.largerLowRatioCanonicalKey(),
			testutil.SharedCanonicalPageKey(
				result.largerVerticalExtentCanonicalKey(),
				result.largerVerticalSegmentSpanCanonicalKey(),
				result.largerVerticalSegmentSpanPerWidthCanonicalKey(),
			),
		).SharedCanonicalKey(),
	)
}
