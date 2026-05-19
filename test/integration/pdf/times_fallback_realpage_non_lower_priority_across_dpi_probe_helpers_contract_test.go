package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageNonLowerPriorityAcrossDPIProbeResult_CapturesGapShrinkWithFlatGlyphSwap(t *testing.T) {
	result := realPageNonLowerPriorityAcrossDPIProbeResult{
		dpi72: realPageNonLowerPriorityAtDPIProbeResult{
			codeSpecGap:           0.08,
			expandedResidualGap:   0.13,
			coreGlyphSwapGain:     0.0,
			expandedGlyphSwapGain: 0.0,
			dpi:                   72,
		},
		dpi150: realPageNonLowerPriorityAtDPIProbeResult{
			codeSpecGap:           0.02,
			expandedResidualGap:   0.03,
			coreGlyphSwapGain:     0.0,
			expandedGlyphSwapGain: 0.0,
			dpi:                   150,
		},
	}

	require.Greater(t, result.dpi72.codeSpecGap, result.dpi150.codeSpecGap)
	require.Greater(t, result.dpi72.expandedResidualGap, result.dpi150.expandedResidualGap)
	require.Equal(t, result.dpi72.coreGlyphSwapGain, result.dpi150.coreGlyphSwapGain)
	require.Equal(t, result.dpi72.expandedGlyphSwapGain, result.dpi150.expandedGlyphSwapGain)
}
