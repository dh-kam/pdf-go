package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageNonLowerPriorityProbeResult_PrefersCodeSpecAndResidualOverGlyphSwap(t *testing.T) {
	result := realPageNonLowerPriorityProbeResult{
		codeSpec: realPageCodeSpecFastPathProbeResult{
			currentOnly:  10,
			fastPathOnly: 11,
			codeSpecOnly: 13,
		},
		expanded: realPageExpandedLowercaseProbeResult{
			currentOnly:  20,
			expandedOnly: 24,
			fullSkipOnly: 26,
		},
		coreGlyphSwap: realPageGlyphSourceOverrideProbeResult{
			expandedOnly: 24,
			overrideOnly: 24.1,
			overrideGain: 0.1,
			residualGap:  2,
		},
		expandedGlyphSwap: realPageGlyphSourceOverrideProbeResult{
			expandedOnly: 24,
			overrideOnly: 24.2,
			overrideGain: 0.2,
			residualGap:  2,
		},
	}

	require.Greater(t, result.codeSpec.codeSpecGap(), result.coreGlyphSwap.overrideGain)
	require.Greater(t, result.codeSpec.codeSpecGap(), result.expandedGlyphSwap.overrideGain)
	require.Greater(t, result.expanded.residualGap(), result.coreGlyphSwap.overrideGain)
	require.Greater(t, result.expanded.residualGap(), result.expandedGlyphSwap.overrideGain)
}
