package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageExpandedLowercaseProbeResult_ComputesDeltaFields(t *testing.T) {
	result := realPageExpandedLowercaseProbeResult{
		currentOnly:  20,
		broadOnly:    24,
		expandedOnly: 27,
		fullSkipOnly: 29,
		fastPathOnly: 21,
		forcedOnly:   20.5,
	}

	require.Equal(t, 4.0, result.broadGap())
	require.Equal(t, 7.0, result.expandedGap())
	require.Equal(t, 9.0, result.fullGap())
	require.InDelta(t, 7.0/9.0, result.expandedCoverage(), 1e-9)
	require.Equal(t, 1.0, result.fastPathGain())
	require.Equal(t, 0.5, result.forcedGap())
	require.Equal(t, 2.0, result.residualGap())
}
