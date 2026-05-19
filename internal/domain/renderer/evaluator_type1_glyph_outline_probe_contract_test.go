package renderer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/test/testutil"
)

func TestGlyphPathComplexitySignature_TotalSegments(t *testing.T) {
	signature := glyphPathComplexitySignature{
		moves:  2,
		lines:  3,
		curves: 5,
		closes: 7,
	}

	require.Equal(t, 15, signature.totalSegments())
}

func TestGlyphPathComplexitySignature_AreaPerSegment(t *testing.T) {
	signature := glyphPathComplexitySignature{
		lines:      3,
		curves:     5,
		closes:     2,
		boundsArea: 250,
	}

	require.InDelta(t, 25.0, signature.areaPerSegment(), 0.0001)
	require.Zero(t, glyphPathComplexitySignature{}.areaPerSegment())
}

func TestGlyphPathComplexitySignature_SegmentDensity(t *testing.T) {
	signature := glyphPathComplexitySignature{
		lines:      3,
		curves:     5,
		closes:     2,
		boundsArea: 250,
	}

	require.InDelta(t, 0.04, signature.segmentDensity(), 0.0001)
	require.Zero(t, glyphPathComplexitySignature{}.segmentDensity())
}

func TestGlyphPathComplexitySignature_CurveShare(t *testing.T) {
	signature := glyphPathComplexitySignature{
		lines:  3,
		curves: 5,
		closes: 2,
	}

	require.InDelta(t, 0.5, signature.curveShare(), 0.0001)
	require.Zero(t, glyphPathComplexitySignature{}.curveShare())
}

func TestGlyphPathComplexitySignature_LineAndCloseShare(t *testing.T) {
	signature := glyphPathComplexitySignature{
		lines:  3,
		curves: 5,
		closes: 2,
	}

	require.InDelta(t, 0.3, signature.lineShare(), 0.0001)
	require.InDelta(t, 0.2, signature.closeShare(), 0.0001)
	require.Zero(t, glyphPathComplexitySignature{}.lineShare())
	require.Zero(t, glyphPathComplexitySignature{}.closeShare())
}

func TestMeasureSyntheticExpandedGlyphSetOutlineProbeResult_UsesExpectedSurface(t *testing.T) {
	result := measureSyntheticExpandedGlyphSetOutlineProbeResult(t, syntheticExpandedGlyphSetProbeCases()[0])

	require.Equal(t, "009_p95_sfrm1095", result.name)
	require.Greater(t, result.lowRatio, 0.0)
	require.Greater(t, result.complexity.curves, 0)
	require.Greater(t, result.complexity.areaPerSegment(), 0.0)
}

func TestSyntheticExpandedGlyphSetOutlineOrderingProbeResult_LargerNames(t *testing.T) {
	result := syntheticExpandedGlyphSetOutlineOrderingProbeResult{
		page95Name: "009_p95_sfrm1095",
		page95: syntheticExpandedGlyphSetOutlineProbeResult{
			lowRatio: 1.0,
			complexity: glyphPathComplexitySignature{
				boundsArea: 100,
				lines:      10,
				curves:     10,
			},
		},
		page109Name: "009_p109_sfrm1095",
		page109: syntheticExpandedGlyphSetOutlineProbeResult{
			lowRatio: 2.0,
			complexity: glyphPathComplexitySignature{
				boundsArea: 300,
				lines:      10,
				curves:     5,
			},
		},
	}

	require.Equal(t, "009_p109_sfrm1095", result.largerLowRatioName())
	require.Equal(t, "page109", result.largerLowRatioCanonicalKey())
	require.Equal(t, "009_p109_sfrm1095", result.largerAreaPerSegmentName())
	require.Equal(t, "page109", result.largerAreaPerSegmentCanonicalKey())
	require.Equal(t, "009_p109_sfrm1095", result.lowerSegmentDensityName())
	require.Equal(t, "page109", result.lowerSegmentDensityCanonicalKey())
	require.Equal(t, "009_p109_sfrm1095", result.lowerCurveShareName())
	require.Equal(t, "page109", result.lowerCurveShareCanonicalKey())
	require.Equal(
		t,
		"page109",
		testutil.NewProbeOrderingAlignment(
			result.largerLowRatioCanonicalKey(),
			testutil.SharedCanonicalPageKey(
				result.largerAreaPerSegmentCanonicalKey(),
				result.lowerSegmentDensityCanonicalKey(),
				result.lowerCurveShareCanonicalKey(),
			),
		).SharedCanonicalKey(),
	)
}

func TestSyntheticExpandedGlyphSetClusterOrderingProbeResult_UsesExpectedWorstClusters(t *testing.T) {
	result := syntheticExpandedGlyphSetClusterOrderingProbeResult{
		page95LowestDensity:  syntheticExpandedGlyphSetClusterOutlineProbeResult{clusterName: "long_tail"},
		page109LowestDensity: syntheticExpandedGlyphSetClusterOutlineProbeResult{clusterName: "non_lower"},
		page95LowestCurve:    syntheticExpandedGlyphSetClusterOutlineProbeResult{clusterName: "non_lower"},
		page109LowestCurve:   syntheticExpandedGlyphSetClusterOutlineProbeResult{clusterName: "non_lower"},
	}

	require.Equal(t, "long_tail", result.page95LowestDensity.clusterName)
	require.Equal(t, "non_lower", result.page109LowestDensity.clusterName)
	require.Equal(t, "non_lower", result.page95LowestCurve.clusterName)
	require.Equal(t, "non_lower", result.page109LowestCurve.clusterName)
}

func TestMeasureSyntheticGlyphCodeOutlineProbeResults_ResolvesGlyphMetadata(t *testing.T) {
	results := measureSyntheticGlyphCodeOutlineProbeResults(
		t,
		syntheticExpandedGlyphSetProbeCases()[1],
		[]int{47, 65},
	)

	require.Len(t, results, 2)
	require.Equal(t, 47, results[0].code)
	require.Equal(t, "slash", results[0].glyphName)
	require.NotZero(t, results[0].glyph)
	require.NotNil(t, results[0].path)
	require.InDelta(t, 0, results[0].bounds[0], 0.0001)
	require.InDelta(t, -722.65625, results[0].bounds[1], 0.0001)
	require.InDelta(t, 277.83203125, results[0].bounds[2], 0.0001)
	require.InDelta(t, 68.359375, results[0].bounds[3], 0.0001)
	require.False(t, results[0].hasDecodedRune)
	require.Equal(t, 65, results[1].code)
	require.Equal(t, "A", results[1].glyphName)
	require.NotNil(t, results[1].path)
	require.NotEqual(t, [4]float64{}, results[1].bounds)
	require.True(t, results[1].hasDecodedRune)
	require.Equal(t, rune('A'), results[1].decodedRune)
}

func TestMeasureSyntheticGlyphCodeOrderingForProbe_UsesExpectedWorstCodes(t *testing.T) {
	result := measureSyntheticGlyphCodeOrderingForProbe(
		t,
		syntheticExpandedGlyphSetProbeCases()[1],
		syntheticExpandedGlyphSetProbeCases()[1].nonLowerCodes,
	)

	require.Equal(t, 47, result.lowestDensity.code)
	require.Equal(t, "slash", result.lowestDensity.glyphName)
	require.Equal(t, 65, result.lowestCurve.code)
	require.Equal(t, "A", result.lowestCurve.glyphName)
}

func TestSyntheticGlyphCodeOutlineProbeResult_BoundsHelpers(t *testing.T) {
	result := syntheticGlyphCodeOutlineProbeResult{
		bounds: [4]float64{10, -20, 30, 80},
	}

	require.InDelta(t, 20.0, result.boundsWidth(), 0.0001)
	require.InDelta(t, 100.0, result.boundsHeight(), 0.0001)
	require.InDelta(t, 5.0, result.boundsAspectRatio(), 0.0001)
}

func TestMeasureStandardGlyphSourceAlternativesForProbe_SelectsBestDensityCandidate(t *testing.T) {
	result := measureStandardGlyphSourceAlternativesForProbe(t, 47, "Times-Roman", "Helvetica", "Courier")

	require.Equal(t, 47, result.code)
	require.Len(t, result.sources, 3)
	require.Equal(t, "Helvetica", result.bestDensity.fontName)
	require.GreaterOrEqual(
		t,
		result.bestDensity.complexity.segmentDensity(),
		sourceByFontNameForProbe(t, result, "Times-Roman").complexity.segmentDensity(),
	)
}
