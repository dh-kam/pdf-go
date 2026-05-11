package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageNonLowerGapShrinksAt150DPIWhileGlyphSourceSwapStaysFlat(t *testing.T) {
	result := measureRealPageNonLowerPriorityAcrossDPIProbeAgainstPoppler(t)

	t.Logf(
		"dpi72 code_spec_gap=%.4f residual_gap=%.4f core_gain=%.4f expanded_gain=%.4f dpi150 code_spec_gap=%.4f residual_gap=%.4f core_gain=%.4f expanded_gain=%.4f",
		result.dpi72.codeSpecGap,
		result.dpi72.expandedResidualGap,
		result.dpi72.coreGlyphSwapGain,
		result.dpi72.expandedGlyphSwapGain,
		result.dpi150.codeSpecGap,
		result.dpi150.expandedResidualGap,
		result.dpi150.coreGlyphSwapGain,
		result.dpi150.expandedGlyphSwapGain,
	)

	require.Greater(t, result.dpi72.codeSpecGap, result.dpi150.codeSpecGap)
	require.Greater(t, result.dpi72.expandedResidualGap, result.dpi150.expandedResidualGap)
	require.Equal(t, result.dpi72.coreGlyphSwapGain, result.dpi150.coreGlyphSwapGain)
	require.Equal(t, result.dpi72.expandedGlyphSwapGain, result.dpi150.expandedGlyphSwapGain)
}

