package renderer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/test/testutil"
)

func TestSyntheticExpandedGlyphSet_Page109HighestLowRatioCodeDiffersFromLowestDensityCode(t *testing.T) {
	results := measurePage109NonLowerCodeOutlineProbeResultsForProbe(t)
	highestLowRatio := highestLowRatioCodeForProbe(t, results)
	lowestDensity := lowestSegmentDensityCodeForProbe(t, results)

	t.Logf(
		"page109_non_lower highest_low_ratio_code=%d glyph=%s low_ratio=%.6f density=%.12f lowest_density_code=%d glyph=%s low_ratio=%.6f density=%.12f",
		highestLowRatio.code,
		highestLowRatio.glyphName,
		highestLowRatio.lowRatio,
		highestLowRatio.complexity.segmentDensity(),
		lowestDensity.code,
		lowestDensity.glyphName,
		lowestDensity.lowRatio,
		lowestDensity.complexity.segmentDensity(),
	)

	require.Equal(t, 49, highestLowRatio.code)
	require.Equal(t, "one", highestLowRatio.glyphName)
	require.Equal(t, 47, lowestDensity.code)
	require.NotEqual(t, highestLowRatio.code, lowestDensity.code)
	require.NotEqual(t, highestLowRatio.lowRatio, lowestDensity.lowRatio)
}

func TestSyntheticExpandedGlyphSet_Page109HighestLowRatioCodeStillBeatsSlashAndAOnSegmentsPerWidth(t *testing.T) {
	results := measurePage109NonLowerCodeOutlineProbeResultsForProbe(t)
	highestLowRatio := highestLowRatioCodeForProbe(t, results)
	highestSegmentsPerWidth := highestSegmentsPerWidthCodeForProbe(t, results)
	slash := codeByValueForProbe(t, results, 47)
	upperA := codeByValueForProbe(t, results, 65)

	t.Logf(
		"page109_non_lower highest_low_ratio_code=%d highest_segments_per_width_code=%d code49 segments_per_width=%.6f low_ratio=%.6f slash segments_per_width=%.6f low_ratio=%.6f A segments_per_width=%.6f low_ratio=%.6f",
		highestLowRatio.code,
		highestSegmentsPerWidth.code,
		highestLowRatio.segmentsPerWidth(),
		highestLowRatio.lowRatio,
		slash.segmentsPerWidth(),
		slash.lowRatio,
		upperA.segmentsPerWidth(),
		upperA.lowRatio,
	)

	require.Equal(t, 49, highestLowRatio.code)
	require.GreaterOrEqual(t, highestLowRatio.segmentsPerWidth(), slash.segmentsPerWidth())
	require.GreaterOrEqual(t, highestLowRatio.segmentsPerWidth(), upperA.segmentsPerWidth())
}

func TestSyntheticExpandedGlyphSet_Page109HighestSegmentsPerWidthCodeDiffersFromHighestLowRatioCode(t *testing.T) {
	results := measurePage109NonLowerCodeOutlineProbeResultsForProbe(t)
	highestLowRatio := highestLowRatioCodeForProbe(t, results)
	highestSegmentsPerWidth := highestSegmentsPerWidthCodeForProbe(t, results)

	t.Logf(
		"page109_non_lower highest_low_ratio_code=%d glyph=%s low_ratio=%.6f segments_per_width=%.6f highest_segments_per_width_code=%d glyph=%s low_ratio=%.6f segments_per_width=%.6f",
		highestLowRatio.code,
		highestLowRatio.glyphName,
		highestLowRatio.lowRatio,
		highestLowRatio.segmentsPerWidth(),
		highestSegmentsPerWidth.code,
		highestSegmentsPerWidth.glyphName,
		highestSegmentsPerWidth.lowRatio,
		highestSegmentsPerWidth.segmentsPerWidth(),
	)

	require.Equal(t, 49, highestLowRatio.code)
	require.Equal(t, 58, highestSegmentsPerWidth.code)
	require.NotEqual(t, highestLowRatio.code, highestSegmentsPerWidth.code)
}

func TestSyntheticExpandedGlyphSet_Page109HighestVerticalSegmentSpanPerWidthCodeDiffersFromHighestLowRatioCode(t *testing.T) {
	results := measurePage109NonLowerCodeOutlineProbeResultsForProbe(t)
	highestLowRatio := highestLowRatioCodeForProbe(t, results)
	highestVerticalSpanRatio := highestVerticalSegmentSpanPerWidthCodeForProbe(t, results)

	t.Logf(
		"page109_non_lower highest_low_ratio_code=%d glyph=%s low_ratio=%.6f vertical_span_per_width=%.6f highest_vertical_span_ratio_code=%d glyph=%s low_ratio=%.6f vertical_span_per_width=%.6f",
		highestLowRatio.code,
		highestLowRatio.glyphName,
		highestLowRatio.lowRatio,
		highestLowRatio.verticalSegmentSpanPerWidth(),
		highestVerticalSpanRatio.code,
		highestVerticalSpanRatio.glyphName,
		highestVerticalSpanRatio.lowRatio,
		highestVerticalSpanRatio.verticalSegmentSpanPerWidth(),
	)

	require.Equal(t, 49, highestLowRatio.code)
	require.Equal(t, 47, highestVerticalSpanRatio.code)
	require.NotEqual(t, highestLowRatio.code, highestVerticalSpanRatio.code)
}

func TestSyntheticExpandedGlyphSet_Page109HighestVerticalStrokeComplexityCodeMatchesHighestLowRatioCode(t *testing.T) {
	results := measurePage109NonLowerCodeOutlineProbeResultsForProbe(t)
	highestLowRatio := highestLowRatioCodeForProbe(t, results)
	highestVerticalStrokeComplexity := highestVerticalStrokeComplexityCodeForProbe(t, results)

	t.Logf(
		"page109_non_lower highest_low_ratio_code=%d glyph=%s low_ratio=%.6f vertical_stroke_complexity=%.6f highest_vertical_stroke_complexity_code=%d glyph=%s low_ratio=%.6f vertical_stroke_complexity=%.6f",
		highestLowRatio.code,
		highestLowRatio.glyphName,
		highestLowRatio.lowRatio,
		highestLowRatio.verticalStrokeComplexity(),
		highestVerticalStrokeComplexity.code,
		highestVerticalStrokeComplexity.glyphName,
		highestVerticalStrokeComplexity.lowRatio,
		highestVerticalStrokeComplexity.verticalStrokeComplexity(),
	)

	require.Equal(t, 49, highestLowRatio.code)
	require.Equal(t, 49, highestVerticalStrokeComplexity.code)
	require.Equal(t, highestLowRatio.code, highestVerticalStrokeComplexity.code)
}

func TestSyntheticExpandedGlyphSet_Page109Code49And58ExposeDifferentLowDPIGeometryModes(t *testing.T) {
	results := measurePage109NonLowerCodeOutlineProbeResultsForProbe(t)
	code49 := codeByValueForProbe(t, results, 49)
	code58 := codeByValueForProbe(t, results, 58)

	t.Logf(
		"page109_non_lower code49 glyph=%s low_ratio=%.6f density=%.12f aspect=%.6f segments=%d segments_per_width=%.6f bounds=%v code58 glyph=%s low_ratio=%.6f density=%.12f aspect=%.6f segments=%d segments_per_width=%.6f bounds=%v",
		code49.glyphName,
		code49.lowRatio,
		code49.complexity.segmentDensity(),
		code49.boundsAspectRatio(),
		code49.complexity.totalSegments(),
		code49.segmentsPerWidth(),
		code49.bounds,
		code58.glyphName,
		code58.lowRatio,
		code58.complexity.segmentDensity(),
		code58.boundsAspectRatio(),
		code58.complexity.totalSegments(),
		code58.segmentsPerWidth(),
		code58.bounds,
	)

	require.Equal(t, "one", code49.glyphName)
	require.Equal(t, "colon", code58.glyphName)
	require.Greater(t, code49.lowRatio, code58.lowRatio)
	require.Less(t, code49.segmentsPerWidth(), code58.segmentsPerWidth())
}

func TestSyntheticExpandedGlyphSet_Page109Code49HasGreaterVerticalExtentThanCode58(t *testing.T) {
	results := measurePage109NonLowerCodeOutlineProbeResultsForProbe(t)
	code49 := codeByValueForProbe(t, results, 49)
	code58 := codeByValueForProbe(t, results, 58)

	t.Logf(
		"page109_non_lower code49 vertical_extent=%.6f low_ratio=%.6f code58 vertical_extent=%.6f low_ratio=%.6f code49_segments_per_width=%.6f code58_segments_per_width=%.6f",
		code49.verticalExtent(),
		code49.lowRatio,
		code58.verticalExtent(),
		code58.lowRatio,
		code49.segmentsPerWidth(),
		code58.segmentsPerWidth(),
	)

	require.Greater(t, code49.verticalExtent(), code58.verticalExtent())
	require.Greater(t, code49.lowRatio, code58.lowRatio)
}

func TestSyntheticExpandedGlyphSet_Page109Code49HasLargerVerticalSegmentSpanThanCode58(t *testing.T) {
	results := measurePage109NonLowerCodeOutlineProbeResultsForProbe(t)
	code49 := codeByValueForProbe(t, results, 49)
	code58 := codeByValueForProbe(t, results, 58)
	segments49 := lineSegmentsForProbe(t, code49)
	segments58 := lineSegmentsForProbe(t, code58)

	t.Logf(
		"page109_non_lower code49 max_abs_dx=%.6f max_abs_dy=%.6f low_ratio=%.6f code58 max_abs_dx=%.6f max_abs_dy=%.6f low_ratio=%.6f",
		maxAbsDXForSegments(segments49),
		maxAbsDYForSegments(segments49),
		code49.lowRatio,
		maxAbsDXForSegments(segments58),
		maxAbsDYForSegments(segments58),
		code58.lowRatio,
	)

	require.GreaterOrEqual(t, maxAbsDYForSegments(segments49), maxAbsDYForSegments(segments58))
	require.Greater(t, code49.lowRatio, code58.lowRatio)
}

func TestSyntheticExpandedGlyphSet_Page109Code49HasHigherVerticalSegmentSpanPerWidthThanCode58(t *testing.T) {
	result := measureSyntheticNonLowerCode49Code58OrderingForProbe(t)

	t.Logf(
		"page109_non_lower code49 vertical_span_per_width=%.6f low_ratio=%.6f code58 vertical_span_per_width=%.6f low_ratio=%.6f",
		result.code49.verticalSegmentSpanPerWidth(),
		result.code49.lowRatio,
		result.code58.verticalSegmentSpanPerWidth(),
		result.code58.lowRatio,
	)

	require.Greater(t, result.code49.verticalSegmentSpanPerWidth(), result.code58.verticalSegmentSpanPerWidth())
	require.Greater(t, result.code49.lowRatio, result.code58.lowRatio)
}

func TestSyntheticExpandedGlyphSet_Page109Code49GeometrySignalsAlignOnCanonicalCode49(t *testing.T) {
	result := measureSyntheticNonLowerCode49Code58OrderingForProbe(t)
	alignment := testutil.NewProbeOrderingAlignment(
		result.largerLowRatioCanonicalKey(),
		testutil.SharedCanonicalPageKey(
			result.largerVerticalExtentCanonicalKey(),
			result.largerVerticalSegmentSpanCanonicalKey(),
			result.largerVerticalSegmentSpanPerWidthCanonicalKey(),
		),
	)

	t.Logf(
		"page109_non_lower code49 low_ratio=%.6f vertical_extent=%.6f max_abs_dy=%.6f vertical_span_per_width=%.6f code58 low_ratio=%.6f vertical_extent=%.6f max_abs_dy=%.6f vertical_span_per_width=%.6f shared=%s",
		result.code49.lowRatio,
		result.code49.verticalExtent(),
		maxAbsDYForSegments(lineSegmentsForProbe(t, result.code49)),
		result.code49.verticalSegmentSpanPerWidth(),
		result.code58.lowRatio,
		result.code58.verticalExtent(),
		maxAbsDYForSegments(lineSegmentsForProbe(t, result.code58)),
		result.code58.verticalSegmentSpanPerWidth(),
		alignment.SharedCanonicalKey(),
	)

	require.Equal(t, "code49", alignment.SharedCanonicalKey())
}
