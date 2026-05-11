package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type realPageBroadLowercaseProbeAssertFunc func(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	targetOnlyEnv map[string]string,
	result realPageBroadLowercaseProbeResult,
)

func runRealPageBroadLowercaseProbeCases(
	t *testing.T,
	cases []realPageLowercaseProbeCase,
	assertFn realPageBroadLowercaseProbeAssertFunc,
) {
	t.Helper()

	if _, err := execLookPathPdftoppmForProbe(); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, tc := range cases {
		allBaseFonts := pageBaseFontsForProbe(t, tc.target)
		if len(allBaseFonts) <= 1 {
			continue
		}

		t.Run(tc.target.name, func(t *testing.T) {
			popplerPNG := preparePopplerPageForProbe(t, tc.target)
			targetOnlyEnv := targetFontOnlyEnvForProbe(allBaseFonts, tc.baseFont)
			result := measureRealPageBroadLowercaseProbeAgainstPoppler(t, tc, popplerPNG, targetOnlyEnv)
			assertFn(t, tc, targetOnlyEnv, result)
		})
	}
}

func runRealPageMixedBroadLowercaseProbe(
	t *testing.T,
	assertFn realPageBroadLowercaseProbeAssertFunc,
) {
	t.Helper()
	runRealPageBroadLowercaseProbeCases(t, realPageCMRMixedBroadLowercaseProbeCases(), assertFn)
}

func runRealPageLongTailBroadLowercaseProbe(
	t *testing.T,
	assertFn realPageBroadLowercaseProbeAssertFunc,
) {
	t.Helper()
	runRealPageBroadLowercaseProbeCases(t, realPageSFRMLongTailBroadLowercaseProbeCases(), assertFn)
}

func runRealPageNonLowerBroadLowercaseProbe(
	t *testing.T,
	assertFn realPageBroadLowercaseProbeAssertFunc,
) {
	t.Helper()
	runRealPageBroadLowercaseProbeCases(t, realPageSFRMNonLowerBroadLowercaseProbeCases(), assertFn)
}

func runRealPageSFRMBroadLowercaseProbe(
	t *testing.T,
	assertFn realPageBroadLowercaseProbeAssertFunc,
) {
	t.Helper()
	runRealPageBroadLowercaseProbeCases(t, realPageSFRMBroadLowercaseProbeCases(), assertFn)
}

func assertSingleResidualClassForBroadCases(
	t *testing.T,
	expected realPageResidualClass,
	tc realPageLowercaseProbeCase,
) {
	t.Helper()
	require.Equal(t, expected, tc.dominantResidualClass())
}
