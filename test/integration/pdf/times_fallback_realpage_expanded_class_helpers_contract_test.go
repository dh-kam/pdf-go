package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunRealPageExpandedLowercaseProbeHelpers_InvokeOnlyExpectedResidualClass(t *testing.T) {
	mixedCalls := 0
	runRealPageMixedExpandedLowercaseProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageExpandedLowercaseProbeResult) {
		mixedCalls++
		require.Equal(t, realPageResidualClassMixedLowercase, tc.dominantResidualClass())
	})
	require.Equal(t, 0, mixedCalls)

	longTailCalls := 0
	runRealPageLongTailExpandedLowercaseProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageExpandedLowercaseProbeResult) {
		longTailCalls++
		require.Equal(t, realPageResidualClassLongTail, tc.dominantResidualClass())
	})
	require.Equal(t, 1, longTailCalls)

	nonLowerCalls := 0
	runRealPageNonLowerExpandedLowercaseProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageExpandedLowercaseProbeResult) {
		nonLowerCalls++
		require.Equal(t, realPageResidualClassNonLower, tc.dominantResidualClass())
	})
	require.Equal(t, 1, nonLowerCalls)
}
