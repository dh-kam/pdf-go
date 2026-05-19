package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageResidualClassCodeSpecOrderingProbeResult_PrefersNonLowerGap(t *testing.T) {
	result := realPageResidualClassCodeSpecOrderingProbeResult{
		longTailLowercase: realPageCodeSpecFastPathProbeResult{
			currentOnly:  10,
			fastPathOnly: 11,
			codeSpecOnly: 12,
		},
		nonLowerLowercase: realPageCodeSpecFastPathProbeResult{
			currentOnly:  20,
			fastPathOnly: 21,
			codeSpecOnly: 23,
		},
	}

	require.Greater(t, result.nonLowerLowercase.codeSpecGap(), result.longTailLowercase.codeSpecGap())
}

func TestRealPageResidualClassExpandedOrderingProbeResult_PrefersNonLowerResidualGap(t *testing.T) {
	result := realPageResidualClassExpandedOrderingProbeResult{
		longTailExpanded: realPageExpandedLowercaseProbeResult{
			currentOnly:  10,
			expandedOnly: 14,
			fullSkipOnly: 15,
		},
		nonLowerExpanded: realPageExpandedLowercaseProbeResult{
			currentOnly:  20,
			expandedOnly: 24,
			fullSkipOnly: 26,
		},
	}

	require.Greater(t, result.nonLowerExpanded.residualGap(), result.longTailExpanded.residualGap())
}
