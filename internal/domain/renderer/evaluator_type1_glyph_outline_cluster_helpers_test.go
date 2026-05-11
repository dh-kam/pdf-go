package renderer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type syntheticExpandedGlyphSetCluster struct {
	name  string
	codes []int
}

type syntheticExpandedGlyphSetClusterOutlineProbeResult struct {
	clusterName string
	complexity  glyphPathComplexitySignature
	lowRatio    float64
}

type syntheticExpandedGlyphSetClusterOrderingProbeResult struct {
	page95LowestDensity  syntheticExpandedGlyphSetClusterOutlineProbeResult
	page109LowestDensity syntheticExpandedGlyphSetClusterOutlineProbeResult
	page95LowestCurve    syntheticExpandedGlyphSetClusterOutlineProbeResult
	page109LowestCurve   syntheticExpandedGlyphSetClusterOutlineProbeResult
}

type syntheticExpandedGlyphSetClusterResultsPair struct {
	page95  []syntheticExpandedGlyphSetClusterOutlineProbeResult
	page109 []syntheticExpandedGlyphSetClusterOutlineProbeResult
}

func (c syntheticExpandedGlyphSetProbeCase) clusters() []syntheticExpandedGlyphSetCluster {
	return []syntheticExpandedGlyphSetCluster{
		{name: "top", codes: c.topCodes},
		{name: "secondary", codes: c.secondaryCodes},
		{name: "long_tail", codes: c.longTailCodes},
		{name: "non_lower", codes: c.nonLowerCodes},
	}
}

func measureSyntheticExpandedGlyphSetClusterOutlineProbeResults(
	t *testing.T,
	tc syntheticExpandedGlyphSetProbeCase,
) []syntheticExpandedGlyphSetClusterOutlineProbeResult {
	t.Helper()

	resolved := loadSyntheticExpandedGlyphSetResolvedProbe(t, tc)
	defer func() {
		require.NoError(t, resolved.doc.Close())
	}()

	clusters := tc.clusters()
	results := make([]syntheticExpandedGlyphSetClusterOutlineProbeResult, 0, len(clusters))
	for _, cluster := range clusters {
		results = append(results, syntheticExpandedGlyphSetClusterOutlineProbeResult{
			clusterName: cluster.name,
			complexity:  collectGlyphPathComplexitySignature(t, resolved.font, cluster.codes),
			lowRatio:    syntheticLowScaleRasterErrorRatioForProbe(t, resolved.font, cluster.codes, 0.02),
		})
	}
	return results
}

func lowestSegmentDensityClusterForProbe(
	t *testing.T,
	results []syntheticExpandedGlyphSetClusterOutlineProbeResult,
) syntheticExpandedGlyphSetClusterOutlineProbeResult {
	t.Helper()
	require.NotEmpty(t, results)

	lowest := results[0]
	for _, result := range results[1:] {
		if result.complexity.segmentDensity() < lowest.complexity.segmentDensity() {
			lowest = result
		}
	}
	return lowest
}

func lowestCurveShareClusterForProbe(
	t *testing.T,
	results []syntheticExpandedGlyphSetClusterOutlineProbeResult,
) syntheticExpandedGlyphSetClusterOutlineProbeResult {
	t.Helper()
	require.NotEmpty(t, results)

	lowest := results[0]
	for _, result := range results[1:] {
		if result.complexity.curveShare() < lowest.complexity.curveShare() {
			lowest = result
		}
	}
	return lowest
}

func clusterByNameForProbe(
	t *testing.T,
	results []syntheticExpandedGlyphSetClusterOutlineProbeResult,
	name string,
) syntheticExpandedGlyphSetClusterOutlineProbeResult {
	t.Helper()

	for _, result := range results {
		if result.clusterName == name {
			return result
		}
	}

	t.Fatalf("cluster %q not found", name)
	return syntheticExpandedGlyphSetClusterOutlineProbeResult{}
}

func measureSyntheticExpandedGlyphSetClusterResultsPair(
	t *testing.T,
) syntheticExpandedGlyphSetClusterResultsPair {
	t.Helper()

	cases := syntheticExpandedGlyphSetProbeCases()
	require.Len(t, cases, 2)

	return syntheticExpandedGlyphSetClusterResultsPair{
		page95:  measureSyntheticExpandedGlyphSetClusterOutlineProbeResults(t, cases[0]),
		page109: measureSyntheticExpandedGlyphSetClusterOutlineProbeResults(t, cases[1]),
	}
}

func measureSyntheticExpandedGlyphSetClusterOrderingForProbe(
	t *testing.T,
) syntheticExpandedGlyphSetClusterOrderingProbeResult {
	t.Helper()

	pair := measureSyntheticExpandedGlyphSetClusterResultsPair(t)

	return syntheticExpandedGlyphSetClusterOrderingProbeResult{
		page95LowestDensity:  lowestSegmentDensityClusterForProbe(t, pair.page95),
		page109LowestDensity: lowestSegmentDensityClusterForProbe(t, pair.page109),
		page95LowestCurve:    lowestCurveShareClusterForProbe(t, pair.page95),
		page109LowestCurve:   lowestCurveShareClusterForProbe(t, pair.page109),
	}
}
