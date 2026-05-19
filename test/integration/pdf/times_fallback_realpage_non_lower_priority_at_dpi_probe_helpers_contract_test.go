package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageNonLowerPriorityAtDPIProbeResult_PrefersCodeSpecAndResidualOverGlyphSwap(t *testing.T) {
	result := realPageNonLowerPriorityAtDPIProbeResult{
		codeSpecGap:           3.0,
		expandedResidualGap:   2.0,
		coreGlyphSwapGain:     0.1,
		expandedGlyphSwapGain: 0.2,
		dpi:                   150,
	}

	require.Greater(t, result.codeSpecGap, result.coreGlyphSwapGain)
	require.Greater(t, result.codeSpecGap, result.expandedGlyphSwapGain)
	require.Greater(t, result.expandedResidualGap, result.coreGlyphSwapGain)
	require.Greater(t, result.expandedResidualGap, result.expandedGlyphSwapGain)
}
