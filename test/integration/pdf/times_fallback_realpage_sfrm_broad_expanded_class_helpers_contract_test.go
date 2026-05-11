package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunRealPageSFRMBroadAndExpandedLowercaseProbeHelpers_InvokeOnlyActiveSFRMResidualClasses(t *testing.T) {
	broadSeen := make([]realPageResidualClass, 0, 2)
	runRealPageSFRMBroadLowercaseProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageBroadLowercaseProbeResult) {
		broadSeen = append(broadSeen, tc.dominantResidualClass())
	})
	require.Equal(t, []realPageResidualClass{
		realPageResidualClassLongTail,
		realPageResidualClassNonLower,
	}, broadSeen)

	expandedSeen := make([]realPageResidualClass, 0, 2)
	runRealPageSFRMExpandedLowercaseProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageExpandedLowercaseProbeResult) {
		expandedSeen = append(expandedSeen, tc.dominantResidualClass())
	})
	require.Equal(t, []realPageResidualClass{
		realPageResidualClassLongTail,
		realPageResidualClassNonLower,
	}, expandedSeen)
}

