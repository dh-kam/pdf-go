package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunRealPageBroadLowercaseProbeHelpers_InvokeOnlyExpectedResidualClass(t *testing.T) {
	mixedCalls := 0
	runRealPageMixedBroadLowercaseProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageBroadLowercaseProbeResult) {
		mixedCalls++
		require.Equal(t, realPageResidualClassMixedLowercase, tc.dominantResidualClass())
	})
	require.Equal(t, 0, mixedCalls)

	longTailCalls := 0
	runRealPageLongTailBroadLowercaseProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageBroadLowercaseProbeResult) {
		longTailCalls++
		require.Equal(t, realPageResidualClassLongTail, tc.dominantResidualClass())
	})
	require.Equal(t, 1, longTailCalls)

	nonLowerCalls := 0
	runRealPageNonLowerBroadLowercaseProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageBroadLowercaseProbeResult) {
		nonLowerCalls++
		require.Equal(t, realPageResidualClassNonLower, tc.dominantResidualClass())
	})
	require.Equal(t, 1, nonLowerCalls)
}
