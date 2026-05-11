package renderer

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyntheticExpandedGlyphSet_Page109NonLowerWorstCodeIsStableCandidate(t *testing.T) {
	cases := syntheticExpandedGlyphSetProbeCases()
	require.Len(t, cases, 2)

	result := measureSyntheticGlyphCodeOrderingForProbe(t, cases[1], cases[1].nonLowerCodes)
	lowestDensity := result.lowestDensity
	lowestCurve := result.lowestCurve

	t.Logf(
		"page109_non_lower lowest_density_code=%d density=%.12f low_ratio=%.6f lowest_curve_code=%d curve_share=%.6f low_ratio=%.6f",
		lowestDensity.code,
		lowestDensity.complexity.segmentDensity(),
		lowestDensity.lowRatio,
		lowestCurve.code,
		lowestCurve.complexity.curveShare(),
		lowestCurve.lowRatio,
	)

	require.NotZero(t, lowestDensity.code)
	require.NotZero(t, lowestCurve.code)
}

func TestSyntheticExpandedGlyphSet_Page109NonLowerWorstCodesExposeSourceOutlineTargets(t *testing.T) {
	cases := syntheticExpandedGlyphSetProbeCases()
	require.Len(t, cases, 2)

	ordering := measureSyntheticGlyphCodeOrderingForProbe(t, cases[1], cases[1].nonLowerCodes)
	lowestDensity := ordering.lowestDensity
	lowestCurve := ordering.lowestCurve

	t.Logf(
		"page109_non_lower density_target code=%d glyph=%d glyph_name=%s low_ratio=%.6f curve_target code=%d glyph=%d glyph_name=%s low_ratio=%.6f",
		lowestDensity.code,
		lowestDensity.glyph,
		lowestDensity.glyphName,
		lowestDensity.lowRatio,
		lowestCurve.code,
		lowestCurve.glyph,
		lowestCurve.glyphName,
		lowestCurve.lowRatio,
	)

	require.Equal(t, 47, lowestDensity.code)
	require.Equal(t, "slash", lowestDensity.glyphName)
	require.False(t, lowestDensity.hasDecodedRune)
	require.Equal(t, 84, lowestCurve.code)
	require.Equal(t, "T", lowestCurve.glyphName)
	require.True(t, lowestCurve.hasDecodedRune)
	require.Equal(t, rune('T'), lowestCurve.decodedRune)
}

func TestSyntheticExpandedGlyphSet_Page109NonLowerWorstCodesExposeDistinctSparseOutlineModes(t *testing.T) {
	cases := syntheticExpandedGlyphSetProbeCases()
	require.Len(t, cases, 2)

	results := measureSyntheticGlyphCodeOutlineProbeResults(t, cases[1], cases[1].nonLowerCodes)
	slash := codeByValueForProbe(t, results, 47)
	upperA := codeByValueForProbe(t, results, 65)

	t.Logf(
		"page109_non_lower slash area_per_segment=%.6f density=%.12f curve_share=%.6f low_ratio=%.6f A area_per_segment=%.6f density=%.12f curve_share=%.6f low_ratio=%.6f",
		slash.complexity.areaPerSegment(),
		slash.complexity.segmentDensity(),
		slash.complexity.curveShare(),
		slash.lowRatio,
		upperA.complexity.areaPerSegment(),
		upperA.complexity.segmentDensity(),
		upperA.complexity.curveShare(),
		upperA.lowRatio,
	)

	require.Equal(t, "slash", slash.glyphName)
	require.Equal(t, "A", upperA.glyphName)
	require.LessOrEqual(t, slash.complexity.segmentDensity(), upperA.complexity.segmentDensity())
	require.GreaterOrEqual(t, slash.complexity.areaPerSegment(), upperA.complexity.areaPerSegment())
	require.GreaterOrEqual(t, slash.complexity.curveShare(), upperA.complexity.curveShare())
}

func TestSyntheticExpandedGlyphSet_Page109NonLowerWorstCodesExposeDistinctBoundsGeometry(t *testing.T) {
	cases := syntheticExpandedGlyphSetProbeCases()
	require.Len(t, cases, 2)

	results := measureSyntheticGlyphCodeOutlineProbeResults(t, cases[1], cases[1].nonLowerCodes)
	slash := codeByValueForProbe(t, results, 47)
	upperA := codeByValueForProbe(t, results, 65)

	slashWidth := slash.bounds[2] - slash.bounds[0]
	slashHeight := slash.bounds[3] - slash.bounds[1]
	upperAWidth := upperA.bounds[2] - upperA.bounds[0]
	upperAHeight := upperA.bounds[3] - upperA.bounds[1]

	t.Logf(
		"page109_non_lower slash bounds=%v width=%.6f height=%.6f A bounds=%v width=%.6f height=%.6f",
		slash.bounds,
		slashWidth,
		slashHeight,
		upperA.bounds,
		upperAWidth,
		upperAHeight,
	)

	require.Equal(t, "slash", slash.glyphName)
	require.Equal(t, "A", upperA.glyphName)
	require.LessOrEqual(t, slashWidth, upperAWidth)
	require.GreaterOrEqual(t, slashHeight, upperAHeight)
	require.GreaterOrEqual(t, slash.boundsAspectRatio(), upperA.boundsAspectRatio())
}

func TestSyntheticExpandedGlyphSet_Page109SlashIsMostVerticalSparseOutlineTarget(t *testing.T) {
	cases := syntheticExpandedGlyphSetProbeCases()
	require.Len(t, cases, 2)

	results := measureSyntheticGlyphCodeOutlineProbeResults(t, cases[1], cases[1].nonLowerCodes)
	slash := codeByValueForProbe(t, results, 47)
	upperA := codeByValueForProbe(t, results, 65)
	lowestDensity := lowestSegmentDensityCodeForProbe(t, results)

	t.Logf(
		"page109_non_lower slash aspect=%.6f density=%.12f A aspect=%.6f density=%.12f lowest_density_code=%d",
		slash.boundsAspectRatio(),
		slash.complexity.segmentDensity(),
		upperA.boundsAspectRatio(),
		upperA.complexity.segmentDensity(),
		lowestDensity.code,
	)

	require.Equal(t, 47, lowestDensity.code)
	require.Equal(t, "slash", slash.glyphName)
	require.GreaterOrEqual(t, slash.boundsAspectRatio(), upperA.boundsAspectRatio())
	require.LessOrEqual(t, slash.complexity.segmentDensity(), upperA.complexity.segmentDensity())
}

func TestSyntheticExpandedGlyphSet_Page109SlashHasSimplerLineOnlyPathThanA(t *testing.T) {
	cases := syntheticExpandedGlyphSetProbeCases()
	require.Len(t, cases, 2)

	results := measureSyntheticGlyphCodeOutlineProbeResults(t, cases[1], cases[1].nonLowerCodes)
	slash := codeByValueForProbe(t, results, 47)
	upperA := codeByValueForProbe(t, results, 65)

	t.Logf(
		"page109_non_lower slash moves=%d lines=%d closes=%d total=%d line_share=%.6f close_share=%.6f A moves=%d lines=%d closes=%d total=%d line_share=%.6f close_share=%.6f",
		slash.complexity.moves,
		slash.complexity.lines,
		slash.complexity.closes,
		slash.complexity.totalSegments(),
		slash.complexity.lineShare(),
		slash.complexity.closeShare(),
		upperA.complexity.moves,
		upperA.complexity.lines,
		upperA.complexity.closes,
		upperA.complexity.totalSegments(),
		upperA.complexity.lineShare(),
		upperA.complexity.closeShare(),
	)

	require.Greater(t, slash.complexity.curves, 0)
	require.Greater(t, upperA.complexity.curves, 0)
	require.Less(t, slash.complexity.totalSegments(), upperA.complexity.totalSegments())
	require.LessOrEqual(t, slash.complexity.lineShare(), upperA.complexity.lineShare())
}

func TestSyntheticExpandedGlyphSet_Page109SlashPathSegmentsExposeTallNarrowOutline(t *testing.T) {
	cases := syntheticExpandedGlyphSetProbeCases()
	require.Len(t, cases, 2)

	results := measureSyntheticGlyphCodeOutlineProbeResults(t, cases[1], cases[1].nonLowerCodes)
	slash := codeByValueForProbe(t, results, 47)

	t.Logf(
		"page109_slash bounds=%v total_segments=%d",
		slash.bounds,
		slash.complexity.totalSegments(),
	)

	require.Equal(t, "slash", slash.glyphName)
	require.Equal(t, [4]float64{56, -751, 441, 248}, slash.bounds)
	require.Greater(t, slash.complexity.totalSegments(), 0)
}

func TestSyntheticExpandedGlyphSet_Page109SlashCurrentPathMatchesSyntheticTimesReference(t *testing.T) {
	cases := syntheticExpandedGlyphSetProbeCases()
	require.Len(t, cases, 2)

	resolved := loadSyntheticExpandedGlyphSetResolvedProbe(t, cases[1])
	defer func() {
		require.NoError(t, resolved.doc.Close())
	}()

	synthetic := syntheticTimesWidthMappedFontForCodes(t, resolved.font, []int{47})

	currentGlyph, err := resolved.font.CharCodeToGlyph(47)
	require.NoError(t, err)
	currentPath, err := resolved.font.RenderGlyph(currentGlyph, 1000)
	require.NoError(t, err)
	require.NotNil(t, currentPath)

	syntheticGlyph, err := synthetic.CharCodeToGlyph(47)
	require.NoError(t, err)
	syntheticPath, err := synthetic.RenderGlyph(syntheticGlyph, 1000)
	require.NoError(t, err)
	require.NotNil(t, syntheticPath)

	t.Logf(
		"page109_slash current_bounds=%v synthetic_bounds=%v current_commands=%d synthetic_commands=%d",
		currentPath.Bounds,
		syntheticPath.Bounds,
		len(currentPath.Commands),
		len(syntheticPath.Commands),
	)

	require.NotEqual(t, [4]float64{}, currentPath.Bounds)
	require.NotEqual(t, [4]float64{}, syntheticPath.Bounds)
}

func TestSyntheticExpandedGlyphSet_Page109HighestLowRatioCodeExposesDistinctGeometry(t *testing.T) {
	cases := syntheticExpandedGlyphSetProbeCases()
	require.Len(t, cases, 2)

	results := measureSyntheticGlyphCodeOutlineProbeResults(t, cases[1], cases[1].nonLowerCodes)
	highestLowRatio := highestLowRatioCodeForProbe(t, results)
	lowestDensity := lowestSegmentDensityCodeForProbe(t, results)
	upperA := codeByValueForProbe(t, results, 65)

	t.Logf(
		"page109_non_lower high_low_ratio code=%d glyph=%s low_ratio=%.6f density=%.12f aspect=%.6f segments=%d line_share=%.6f bounds=%v slash_low_ratio=%.6f slash_density=%.12f slash_aspect=%.6f slash_segments=%d A_low_ratio=%.6f A_density=%.12f A_aspect=%.6f A_segments=%d",
		highestLowRatio.code,
		highestLowRatio.glyphName,
		highestLowRatio.lowRatio,
		highestLowRatio.complexity.segmentDensity(),
		highestLowRatio.boundsAspectRatio(),
		highestLowRatio.complexity.totalSegments(),
		highestLowRatio.complexity.lineShare(),
		highestLowRatio.bounds,
		lowestDensity.lowRatio,
		lowestDensity.complexity.segmentDensity(),
		lowestDensity.boundsAspectRatio(),
		lowestDensity.complexity.totalSegments(),
		upperA.lowRatio,
		upperA.complexity.segmentDensity(),
		upperA.boundsAspectRatio(),
		upperA.complexity.totalSegments(),
	)

	require.Equal(t, 49, highestLowRatio.code)
	require.GreaterOrEqual(t, highestLowRatio.lowRatio, lowestDensity.lowRatio)
	require.GreaterOrEqual(t, highestLowRatio.complexity.segmentDensity(), lowestDensity.complexity.segmentDensity())
}


func TestStandardSlashSourceAlternatives_ExposeLessSparseCandidatesThanTimesRoman(t *testing.T) {
	result := measureStandardGlyphSourceAlternativesForProbe(t, 47, "Times-Roman", "Helvetica", "Courier")
	times := sourceByFontNameForProbe(t, result, "Times-Roman")
	helvetica := sourceByFontNameForProbe(t, result, "Helvetica")
	courier := sourceByFontNameForProbe(t, result, "Courier")
	timesAspect := (times.bounds[3] - times.bounds[1]) / (times.bounds[2] - times.bounds[0])
	helveticaAspect := (helvetica.bounds[3] - helvetica.bounds[1]) / (helvetica.bounds[2] - helvetica.bounds[0])
	courierAspect := (courier.bounds[3] - courier.bounds[1]) / (courier.bounds[2] - courier.bounds[0])

	t.Logf(
		"slash_sources times density=%.12f aspect=%.6f segments=%d helvetica density=%.12f aspect=%.6f segments=%d courier density=%.12f aspect=%.6f segments=%d",
		times.complexity.segmentDensity(),
		timesAspect,
		times.complexity.totalSegments(),
		helvetica.complexity.segmentDensity(),
		helveticaAspect,
		helvetica.complexity.totalSegments(),
		courier.complexity.segmentDensity(),
		courierAspect,
		courier.complexity.totalSegments(),
	)

	require.NotEqual(t, lineSegmentsFromPathForProbe(t, times.path), lineSegmentsFromPathForProbe(t, helvetica.path))
	require.NotEqual(t, lineSegmentsFromPathForProbe(t, times.path), lineSegmentsFromPathForProbe(t, courier.path))
	require.Greater(t, helvetica.complexity.segmentDensity(), times.complexity.segmentDensity())
	require.Less(t, helveticaAspect, timesAspect)
	require.Less(t, courierAspect, timesAspect)
}

func TestStandardASourceAlternatives_ExposeCourierAsLessSparseCandidateThanTimesRoman(t *testing.T) {
	result := measureStandardGlyphSourceAlternativesForProbe(t, 65, "Times-Roman", "Helvetica", "Courier")
	times := sourceByFontNameForProbe(t, result, "Times-Roman")
	helvetica := sourceByFontNameForProbe(t, result, "Helvetica")
	courier := sourceByFontNameForProbe(t, result, "Courier")

	t.Logf(
		"a_sources times density=%.12f curve_share=%.6f segments=%d helvetica density=%.12f curve_share=%.6f segments=%d courier density=%.12f curve_share=%.6f segments=%d",
		times.complexity.segmentDensity(),
		times.complexity.curveShare(),
		times.complexity.totalSegments(),
		helvetica.complexity.segmentDensity(),
		helvetica.complexity.curveShare(),
		helvetica.complexity.totalSegments(),
		courier.complexity.segmentDensity(),
		courier.complexity.curveShare(),
		courier.complexity.totalSegments(),
	)

	require.NotEqual(t, lineSegmentsFromPathForProbe(t, times.path), lineSegmentsFromPathForProbe(t, helvetica.path))
	require.NotEqual(t, lineSegmentsFromPathForProbe(t, times.path), lineSegmentsFromPathForProbe(t, courier.path))
	require.Less(t, helvetica.complexity.segmentDensity(), times.complexity.segmentDensity())
	require.Greater(t, courier.complexity.segmentDensity(), times.complexity.segmentDensity())
}

func TestSyntheticExpandedGlyphSet_Page95LongTailWorstCodesExposeCandidates(t *testing.T) {
	cases := syntheticExpandedGlyphSetProbeCases()
	require.Len(t, cases, 2)

	ordering := measureSyntheticGlyphCodeOrderingForProbe(t, cases[0], cases[0].longTailCodes)
	lowestDensity := ordering.lowestDensity
	lowestCurve := ordering.lowestCurve

	t.Logf(
		"page95_long_tail lowest_density_code=%d glyph_name=%s density=%.12f low_ratio=%.6f lowest_curve_code=%d glyph_name=%s curve_share=%.6f low_ratio=%.6f",
		lowestDensity.code,
		lowestDensity.glyphName,
		lowestDensity.complexity.segmentDensity(),
		lowestDensity.lowRatio,
		lowestCurve.code,
		lowestCurve.glyphName,
		lowestCurve.complexity.curveShare(),
		lowestCurve.lowRatio,
	)

	require.Equal(t, 111, lowestDensity.code)
	require.Equal(t, "o", lowestDensity.glyphName)
	require.Equal(t, 108, lowestCurve.code)
	require.Equal(t, "l", lowestCurve.glyphName)
}

func TestStandardVSourceAlternatives_ExposeBestCandidateAgainstTimesRoman(t *testing.T) {
	result := measureStandardGlyphSourceAlternativesForProbe(t, 118, "Times-Roman", "Helvetica", "Courier")
	times := sourceByFontNameForProbe(t, result, "Times-Roman")
	helvetica := sourceByFontNameForProbe(t, result, "Helvetica")
	courier := sourceByFontNameForProbe(t, result, "Courier")

	t.Logf(
		"v_sources times density=%.12f curve_share=%.6f segments=%d helvetica density=%.12f curve_share=%.6f segments=%d courier density=%.12f curve_share=%.6f segments=%d",
		times.complexity.segmentDensity(),
		times.complexity.curveShare(),
		times.complexity.totalSegments(),
		helvetica.complexity.segmentDensity(),
		helvetica.complexity.curveShare(),
		helvetica.complexity.totalSegments(),
		courier.complexity.segmentDensity(),
		courier.complexity.curveShare(),
		courier.complexity.totalSegments(),
	)

	require.NotEqual(t, lineSegmentsFromPathForProbe(t, times.path), lineSegmentsFromPathForProbe(t, helvetica.path))
	require.NotEqual(t, lineSegmentsFromPathForProbe(t, times.path), lineSegmentsFromPathForProbe(t, courier.path))
}

func TestStandardNonLowerCoreSourceAlternatives_ExposeImprovedCandidatesBeyondTimesRoman(t *testing.T) {
	cases := []struct {
		code int
		name string
	}{
		{code: 84, name: "T"},
		{code: 48, name: "0"},
		{code: 49, name: "1"},
		{code: 50, name: "2"},
		{code: 51, name: "3"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := measureStandardGlyphSourceAlternativesForProbe(t, tc.code, "Times-Roman", "Helvetica", "Courier")
			times := sourceByFontNameForProbe(t, result, "Times-Roman")
			helvetica := sourceByFontNameForProbe(t, result, "Helvetica")
			courier := sourceByFontNameForProbe(t, result, "Courier")

			t.Logf(
				"code=%d name=%s times=%.12f helvetica=%.12f courier=%.12f best=%s best_density=%.12f",
				tc.code,
				tc.name,
				times.complexity.segmentDensity(),
				helvetica.complexity.segmentDensity(),
				courier.complexity.segmentDensity(),
				result.bestDensity.fontName,
				result.bestDensity.complexity.segmentDensity(),
			)

			require.GreaterOrEqual(t, result.bestDensity.complexity.segmentDensity(), times.complexity.segmentDensity())
			require.Contains(t, []string{"Times-Roman", "Helvetica", "Courier"}, result.bestDensity.fontName, fmt.Sprintf("unexpected best font for code %d", tc.code))
		})
	}
}

func TestStandardWorstCodeSourceAlternatives_ExposeDistinctResidualClassCandidates(t *testing.T) {
	nonLowerResult := measureStandardGlyphSourceAlternativesForProbe(t, 47, "Times-Roman", "Helvetica", "Courier")
	longTailResult := measureStandardGlyphSourceAlternativesForProbe(t, 118, "Times-Roman", "Helvetica", "Courier")

	t.Logf(
		"page109_non_lower code=%d best=%s density=%.12f page95_long_tail code=%d best=%s density=%.12f",
		nonLowerResult.code,
		nonLowerResult.bestDensity.fontName,
		nonLowerResult.bestDensity.complexity.segmentDensity(),
		longTailResult.code,
		longTailResult.bestDensity.fontName,
		longTailResult.bestDensity.complexity.segmentDensity(),
	)

	require.Equal(t, "Helvetica", nonLowerResult.bestDensity.fontName)
	require.Equal(t, "Courier", longTailResult.bestDensity.fontName)
	require.NotEqual(t, nonLowerResult.bestDensity.fontName, longTailResult.bestDensity.fontName)
}

func TestStandardNonLowerCoreSourceSelectionSet_RequiresMixedFamilies(t *testing.T) {
	result := measureStandardGlyphSourceSelectionSetForProbe(
		t,
		[]int{47, 65, 84, 48, 49, 50, 51},
		"Times-Roman",
		"Helvetica",
		"Courier",
	)

	slash := selectionByCodeForProbe(t, result, 47)
	upperA := selectionByCodeForProbe(t, result, 65)
	one := selectionByCodeForProbe(t, result, 49)
	distinctFonts := distinctBestDensityFontsForProbe(result)

	t.Logf(
		"non_lower_core_best_sources slash=%s A=%s one=%s distinct=%v",
		slash.bestDensityFont,
		upperA.bestDensityFont,
		one.bestDensityFont,
		distinctFonts,
	)

	require.Equal(t, "Helvetica", slash.bestDensityFont)
	require.Equal(t, "Courier", upperA.bestDensityFont)
	require.Equal(t, "Times-Roman", one.bestDensityFont)
	require.Len(t, distinctFonts, 3)
}

func TestStandardSlashSourceAlternatives_ExposeDensityVsLowScaleTradeoff(t *testing.T) {
	result := measureStandardGlyphSourceAlternativesForProbe(t, 47, "Times-Roman", "Helvetica", "Courier")
	times := sourceByFontNameForProbe(t, result, "Times-Roman")
	helvetica := sourceByFontNameForProbe(t, result, "Helvetica")
	courier := sourceByFontNameForProbe(t, result, "Courier")

	t.Logf(
		"slash_sources times density=%.12f low_ratio=%.6f helvetica density=%.12f low_ratio=%.6f courier density=%.12f low_ratio=%.6f",
		times.complexity.segmentDensity(),
		times.lowRatio,
		helvetica.complexity.segmentDensity(),
		helvetica.lowRatio,
		courier.complexity.segmentDensity(),
		courier.lowRatio,
	)

	require.Greater(t, helvetica.complexity.segmentDensity(), times.complexity.segmentDensity())
	require.Greater(t, times.lowRatio, 0.0)
	require.Greater(t, helvetica.lowRatio, 0.0)
	require.Greater(t, courier.lowRatio, 0.0)
	require.InDelta(t, times.lowRatio, helvetica.lowRatio, 0.000001)
	require.InDelta(t, times.lowRatio, courier.lowRatio, 0.000001)
}

func TestStandardASourceAlternatives_ExposeDensityVsLowScaleTradeoff(t *testing.T) {
	result := measureStandardGlyphSourceAlternativesForProbe(t, 65, "Times-Roman", "Helvetica", "Courier")
	times := sourceByFontNameForProbe(t, result, "Times-Roman")
	helvetica := sourceByFontNameForProbe(t, result, "Helvetica")
	courier := sourceByFontNameForProbe(t, result, "Courier")

	t.Logf(
		"a_sources times density=%.12f low_ratio=%.6f helvetica density=%.12f low_ratio=%.6f courier density=%.12f low_ratio=%.6f",
		times.complexity.segmentDensity(),
		times.lowRatio,
		helvetica.complexity.segmentDensity(),
		helvetica.lowRatio,
		courier.complexity.segmentDensity(),
		courier.lowRatio,
	)

	require.Greater(t, courier.complexity.segmentDensity(), times.complexity.segmentDensity())
	require.Greater(t, times.lowRatio, 0.0)
	require.Greater(t, helvetica.lowRatio, 0.0)
	require.Greater(t, courier.lowRatio, 0.0)
	require.InDelta(t, times.lowRatio, helvetica.lowRatio, 0.000001)
	require.InDelta(t, times.lowRatio, courier.lowRatio, 0.000001)
}

func TestStandardNonLowerCoreSourceSelectionSet_ImprovesDensityWithoutImprovingLowScaleAverage(t *testing.T) {
	result := measureStandardGlyphSourceSelectionSetSummaryForProbe(
		t,
		[]int{47, 65, 84, 48, 49, 50, 51},
		"Times-Roman",
		"Helvetica",
		"Courier",
	)

	t.Logf(
		"non_lower_core_summary codes=%d times_density_avg=%.12f best_density_avg=%.12f times_low_ratio_avg=%.6f best_low_ratio_avg=%.6f max_low_ratio_delta=%.6f",
		result.codeCount,
		result.timesDensityAvg,
		result.bestDensityAvg,
		result.timesLowRatioAvg,
		result.bestLowRatioAvg,
		result.maxLowRatioDelta,
	)

	require.Greater(t, result.bestDensityAvg, result.timesDensityAvg)
	require.InDelta(t, result.timesLowRatioAvg, result.bestLowRatioAvg, 0.000001)
	require.InDelta(t, 0.0, result.maxLowRatioDelta, 0.000001)
}

func TestStandardNonLowerCoreSourceAlternatives_HaveZeroPerCodeLowRatioSpread(t *testing.T) {
	codes := []int{47, 65, 84, 48, 49, 50, 51}

	for _, code := range codes {
		t.Run(fmt.Sprintf("code_%d", code), func(t *testing.T) {
			result := measureStandardGlyphSourceAlternativesForProbe(t, code, "Times-Roman", "Helvetica", "Courier")

			t.Logf(
				"code=%d best=%s spread=%.6f times_low=%.6f helvetica_low=%.6f courier_low=%.6f",
				code,
				result.bestDensity.fontName,
				lowRatioSpreadForProbe(result),
				sourceByFontNameForProbe(t, result, "Times-Roman").lowRatio,
				sourceByFontNameForProbe(t, result, "Helvetica").lowRatio,
				sourceByFontNameForProbe(t, result, "Courier").lowRatio,
			)

			require.InDelta(t, 0.0, lowRatioSpreadForProbe(result), 0.000001)
		})
	}
}
