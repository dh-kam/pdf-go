package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageCodeSpecFastPathProbeResult_ComputesDeltaFields(t *testing.T) {
	result := realPageCodeSpecFastPathProbeResult{
		currentOnly:  10,
		fastPathOnly: 11.5,
		codeSpecOnly: 13,
	}

	require.Equal(t, 1.5, result.fastPathGain())
	require.Equal(t, 3.0, result.codeSpecGap())
}

func TestSingleCodeSkipSpecForProbe_FormatsBaseFontAndCode(t *testing.T) {
	require.Equal(t, "SFRM1095=49", singleCodeSkipSpecForProbe("SFRM1095", 49))
}

func TestRunRealPageSFRMNonLowerSingleCodeFastPathProbe_BindsExpectedSingleCodeSpec(t *testing.T) {
	invoked := false

	runRealPageSFRMNonLowerSingleCodeFastPathProbe(t, 49, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, codeSpec string) {
		invoked = true
		require.Equal(t, "009_p109_sfrm1095_top5", tc.target.name)
		require.Equal(t, realPageResidualClassNonLower, tc.dominantResidualClass())
		require.Equal(t, "SFRM1095=49", codeSpec)
		require.NotEmpty(t, targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"])
	})

	require.True(t, invoked)
}

func TestRealPageSFRMLowercaseProbeCaseSelectors_ReturnExpectedResidualClasses(t *testing.T) {
	nonLower := realPageSFRMNonLowerProbeCaseForLowercase(t)
	require.Equal(t, "009_p109_sfrm1095_top5", nonLower.target.name)
	require.Equal(t, realPageResidualClassNonLower, nonLower.dominantResidualClass())

	longTail := realPageSFRMLongTailProbeCaseForLowercase(t)
	require.Equal(t, "009_p95_sfrm1095_top6", longTail.target.name)
	require.Equal(t, realPageResidualClassLongTail, longTail.dominantResidualClass())
}
