package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageNonLowerCoreGlyphSourceOverrideGainRemainsSmallerThanNonLowerCodeSpecGap(t *testing.T) {
	result := measureRealPageNonLowerPriorityProbeAgainstPoppler(t)

	t.Logf(
		"non_lower_gap=%.4f non_lower_fast_path_gain=%.4f core_glyph_swap_gain=%.4f core_glyph_swap_spec=%s",
		result.codeSpec.codeSpecGap(),
		result.codeSpec.fastPathGain(),
		result.coreGlyphSwap.overrideGain,
		result.coreGlyphSwap.overrideSpec,
	)

	require.Greater(t, result.codeSpec.codeSpecGap(), result.coreGlyphSwap.overrideGain)
}

func TestRealPageNonLowerExpandedGlyphSourceOverrideGainRemainsSmallerThanNonLowerCodeSpecGap(t *testing.T) {
	result := measureRealPageNonLowerPriorityProbeAgainstPoppler(t)

	t.Logf(
		"non_lower_gap=%.4f non_lower_fast_path_gain=%.4f expanded_glyph_swap_gain=%.4f expanded_glyph_swap_spec=%s",
		result.codeSpec.codeSpecGap(),
		result.codeSpec.fastPathGain(),
		result.expandedGlyphSwap.overrideGain,
		result.expandedGlyphSwap.overrideSpec,
	)

	require.Greater(t, result.codeSpec.codeSpecGap(), result.expandedGlyphSwap.overrideGain)
}

func TestRealPageNonLowerCoreGlyphSourceOverrideGainRemainsSmallerThanExpandedResidualGap(t *testing.T) {
	result := measureRealPageNonLowerPriorityProbeAgainstPoppler(t)

	t.Logf(
		"expanded_gap=%.4f expanded_residual_gap=%.4f core_glyph_swap_gain=%.4f core_glyph_swap_spec=%s",
		result.expanded.expandedGap(),
		result.expanded.residualGap(),
		result.coreGlyphSwap.overrideGain,
		result.coreGlyphSwap.overrideSpec,
	)

	require.Greater(t, result.expanded.residualGap(), result.coreGlyphSwap.overrideGain)
}

func TestRealPageNonLowerExpandedGlyphSourceOverrideGainRemainsSmallerThanExpandedResidualGap(t *testing.T) {
	result := measureRealPageNonLowerPriorityProbeAgainstPoppler(t)

	t.Logf(
		"expanded_gap=%.4f expanded_residual_gap=%.4f expanded_glyph_swap_gain=%.4f expanded_glyph_swap_spec=%s",
		result.expanded.expandedGap(),
		result.expanded.residualGap(),
		result.expandedGlyphSwap.overrideGain,
		result.expandedGlyphSwap.overrideSpec,
	)

	require.Greater(t, result.expanded.residualGap(), result.expandedGlyphSwap.overrideGain)
}
