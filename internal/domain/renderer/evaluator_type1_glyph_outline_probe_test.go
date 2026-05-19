package renderer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/test/testutil"
)

func TestSyntheticTimesExpandedGlyphSet_Page109LowScaleErrorExceedsPage95WithoutHigherPathComplexity(t *testing.T) {
	pair := measureSyntheticExpandedGlyphSetOutlineProbePair(t)
	result95 := pair.page95
	result109 := pair.page109

	t.Logf(
		"page95_complexity=%+v page109_complexity=%+v page95_low_ratio=%.6f page109_low_ratio=%.6f totalSegments95=%d totalSegments109=%d",
		result95.complexity,
		result109.complexity,
		result95.lowRatio,
		result109.lowRatio,
		result95.complexity.totalSegments(),
		result109.complexity.totalSegments(),
	)

	require.Greater(t, result95.complexity.curves, 0)
	require.Greater(t, result109.complexity.curves, 0)
	require.Greater(t, result109.lowRatio, result95.lowRatio)
	require.LessOrEqual(t, result109.complexity.totalSegments(), result95.complexity.totalSegments())
	require.Greater(t, result109.complexity.boundsArea, result95.complexity.boundsArea)
}

func TestSyntheticTimesExpandedGlyphSet_Page109AreaPerSegmentExceedsPage95AlongsideLowScaleError(t *testing.T) {
	pair := measureSyntheticExpandedGlyphSetOutlineProbePair(t)
	result95 := pair.page95
	result109 := pair.page109

	t.Logf(
		"page95_low_ratio=%.6f page109_low_ratio=%.6f areaPerSegment95=%.6f areaPerSegment109=%.6f",
		result95.lowRatio,
		result109.lowRatio,
		result95.complexity.areaPerSegment(),
		result109.complexity.areaPerSegment(),
	)

	require.Greater(t, result109.lowRatio, result95.lowRatio)
	require.Greater(t, result109.complexity.areaPerSegment(), result95.complexity.areaPerSegment())
}

func TestSyntheticTimesExpandedGlyphSet_Page109LowerSegmentDensityThanPage95AlongsideLowScaleError(t *testing.T) {
	pair := measureSyntheticExpandedGlyphSetOutlineProbePair(t)
	result95 := pair.page95
	result109 := pair.page109

	t.Logf(
		"page95_low_ratio=%.6f page109_low_ratio=%.6f segmentDensity95=%.12f segmentDensity109=%.12f curveShare95=%.6f curveShare109=%.6f",
		result95.lowRatio,
		result109.lowRatio,
		result95.complexity.segmentDensity(),
		result109.complexity.segmentDensity(),
		result95.complexity.curveShare(),
		result109.complexity.curveShare(),
	)

	require.Greater(t, result109.lowRatio, result95.lowRatio)
	require.Less(t, result109.complexity.segmentDensity(), result95.complexity.segmentDensity())
}

func TestSyntheticTimesExpandedGlyphSet_Page109LowerCurveShareThanPage95AlongsideLowScaleError(t *testing.T) {
	pair := measureSyntheticExpandedGlyphSetOutlineProbePair(t)
	result95 := pair.page95
	result109 := pair.page109

	t.Logf(
		"page95_low_ratio=%.6f page109_low_ratio=%.6f curveShare95=%.6f curveShare109=%.6f segmentDensity95=%.12f segmentDensity109=%.12f",
		result95.lowRatio,
		result109.lowRatio,
		result95.complexity.curveShare(),
		result109.complexity.curveShare(),
		result95.complexity.segmentDensity(),
		result109.complexity.segmentDensity(),
	)

	require.Greater(t, result109.lowRatio, result95.lowRatio)
	require.Less(t, result109.complexity.curveShare(), result95.complexity.curveShare())
}

func TestSyntheticExpandedGlyphSetOutlineOrdering_RanksPage109AbovePage95(t *testing.T) {
	result := measureSyntheticExpandedGlyphSetOutlineOrderingForProbe(t)

	t.Logf(
		"%s low_ratio=%.6f area_per_segment=%.6f %s low_ratio=%.6f area_per_segment=%.6f",
		result.page95Name,
		result.page95.lowRatio,
		result.page95.complexity.areaPerSegment(),
		result.page109Name,
		result.page109.lowRatio,
		result.page109.complexity.areaPerSegment(),
	)

	require.Equal(t, result.page109Name, result.largerLowRatioName())
	require.Equal(t, result.page109Name, result.largerAreaPerSegmentName())
	require.Equal(t, "page109", result.largerLowRatioCanonicalKey())
	require.Equal(t, "page109", result.largerAreaPerSegmentCanonicalKey())
}

func TestSyntheticExpandedGlyphSetShapeSparsitySignalsAlignOnCanonicalPage109(t *testing.T) {
	result := measureSyntheticExpandedGlyphSetOutlineOrderingForProbe(t)
	alignment := testutil.NewProbeOrderingAlignment(
		result.largerLowRatioCanonicalKey(),
		testutil.SharedCanonicalPageKey(
			result.largerAreaPerSegmentCanonicalKey(),
			result.lowerSegmentDensityCanonicalKey(),
			result.lowerCurveShareCanonicalKey(),
		),
	)

	t.Logf(
		"%s low_ratio=%.6f area_per_segment=%.6f segment_density=%.12f curve_share=%.6f %s low_ratio=%.6f area_per_segment=%.6f segment_density=%.12f curve_share=%.6f shared=%s",
		result.page95Name,
		result.page95.lowRatio,
		result.page95.complexity.areaPerSegment(),
		result.page95.complexity.segmentDensity(),
		result.page95.complexity.curveShare(),
		result.page109Name,
		result.page109.lowRatio,
		result.page109.complexity.areaPerSegment(),
		result.page109.complexity.segmentDensity(),
		result.page109.complexity.curveShare(),
		alignment.SharedCanonicalKey(),
	)

	require.Equal(t, "page109", alignment.SharedCanonicalKey())
}

func TestSyntheticExpandedGlyphSet_Page109WorstClusterIsSparserThanPage95WorstCluster(t *testing.T) {
	result := measureSyntheticExpandedGlyphSetClusterOrderingForProbe(t)

	t.Logf(
		"page95_lowest_density_cluster=%s density=%.12f low_ratio=%.6f page109_lowest_density_cluster=%s density=%.12f low_ratio=%.6f page95_lowest_curve_cluster=%s curve_share=%.6f low_ratio=%.6f page109_lowest_curve_cluster=%s curve_share=%.6f low_ratio=%.6f",
		result.page95LowestDensity.clusterName,
		result.page95LowestDensity.complexity.segmentDensity(),
		result.page95LowestDensity.lowRatio,
		result.page109LowestDensity.clusterName,
		result.page109LowestDensity.complexity.segmentDensity(),
		result.page109LowestDensity.lowRatio,
		result.page95LowestCurve.clusterName,
		result.page95LowestCurve.complexity.curveShare(),
		result.page95LowestCurve.lowRatio,
		result.page109LowestCurve.clusterName,
		result.page109LowestCurve.complexity.curveShare(),
		result.page109LowestCurve.lowRatio,
	)

	require.LessOrEqual(t, result.page109LowestDensity.complexity.segmentDensity(), result.page95LowestDensity.complexity.segmentDensity())
	require.LessOrEqual(t, result.page109LowestCurve.complexity.curveShare(), result.page95LowestCurve.complexity.curveShare())
}

func TestSyntheticExpandedGlyphSet_Page109NonLowerClusterIsWorstShapeSignal(t *testing.T) {
	pair := measureSyntheticExpandedGlyphSetClusterResultsPair(t)
	nonLower := clusterByNameForProbe(t, pair.page109, "non_lower")
	lowestDensity := lowestSegmentDensityClusterForProbe(t, pair.page109)
	lowestCurve := lowestCurveShareClusterForProbe(t, pair.page109)

	t.Logf(
		"page109_non_lower density=%.12f curve_share=%.6f low_ratio=%.6f lowest_density_cluster=%s lowest_curve_cluster=%s",
		nonLower.complexity.segmentDensity(),
		nonLower.complexity.curveShare(),
		nonLower.lowRatio,
		lowestDensity.clusterName,
		lowestCurve.clusterName,
	)

	require.Equal(t, "non_lower", lowestDensity.clusterName)
	require.Equal(t, "non_lower", lowestCurve.clusterName)
}

func TestSyntheticExpandedGlyphSet_Page95ShapeSignalsSplitAcrossLongTailAndNonLower(t *testing.T) {
	pair := measureSyntheticExpandedGlyphSetClusterResultsPair(t)
	longTail := clusterByNameForProbe(t, pair.page95, "long_tail")
	nonLower := clusterByNameForProbe(t, pair.page95, "non_lower")
	lowestDensity := lowestSegmentDensityClusterForProbe(t, pair.page95)
	lowestCurve := lowestCurveShareClusterForProbe(t, pair.page95)

	t.Logf(
		"page95_long_tail density=%.12f low_ratio=%.6f page95_non_lower curve_share=%.6f low_ratio=%.6f lowest_density_cluster=%s lowest_curve_cluster=%s",
		longTail.complexity.segmentDensity(),
		longTail.lowRatio,
		nonLower.complexity.curveShare(),
		nonLower.lowRatio,
		lowestDensity.clusterName,
		lowestCurve.clusterName,
	)

	require.Equal(t, "long_tail", lowestDensity.clusterName)
	require.Equal(t, "non_lower", lowestCurve.clusterName)
}
