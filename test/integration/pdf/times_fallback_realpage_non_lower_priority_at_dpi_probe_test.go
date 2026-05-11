package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageNonLowerGlyphSourceOverrideGainRemainsSmallerThanCodeSpecAndResidualAt150DPI(t *testing.T) {
	result := measureRealPageNonLowerPriorityAtDPIProbeAgainstPoppler(t, 150)

	t.Logf(
		"dpi=%d code_spec_gap=%.4f expanded_residual_gap=%.4f core_glyph_swap_gain=%.4f expanded_glyph_swap_gain=%.4f",
		result.dpi,
		result.codeSpecGap,
		result.expandedResidualGap,
		result.coreGlyphSwapGain,
		result.expandedGlyphSwapGain,
	)

	require.Greater(t, result.codeSpecGap, result.coreGlyphSwapGain)
	require.Greater(t, result.codeSpecGap, result.expandedGlyphSwapGain)
	require.Greater(t, result.expandedResidualGap, result.coreGlyphSwapGain)
	require.Greater(t, result.expandedResidualGap, result.expandedGlyphSwapGain)
}

