package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageBroadLowercaseProbeResult_ComputesDeltaFields(t *testing.T) {
	result := realPageBroadLowercaseProbeResult{
		currentOnly:  10,
		combinedOnly: 13,
		broadOnly:    16,
		fullSkipOnly: 18,
	}

	require.Equal(t, 3.0, result.combinedGap())
	require.Equal(t, 6.0, result.broadGap())
	require.Equal(t, 8.0, result.fullGap())
	require.Equal(t, 0.75, result.broadCoverage())
}

