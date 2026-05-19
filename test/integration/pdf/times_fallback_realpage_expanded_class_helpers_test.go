package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type realPageExpandedLowercaseProbeAssertFunc func(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	targetOnlyEnv map[string]string,
	result realPageExpandedLowercaseProbeResult,
)

func runRealPageExpandedLowercaseProbeCases(
	t *testing.T,
	cases []realPageLowercaseProbeCase,
	assertFn realPageExpandedLowercaseProbeAssertFunc,
) {
	t.Helper()
	requirePopplerProbeOptIn(t)

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
			result := measureRealPageExpandedLowercaseProbeAgainstPoppler(t, tc, popplerPNG, targetOnlyEnv)
			assertFn(t, tc, targetOnlyEnv, result)
		})
	}
}

func runRealPageMixedExpandedLowercaseProbe(
	t *testing.T,
	assertFn realPageExpandedLowercaseProbeAssertFunc,
) {
	t.Helper()
	runRealPageExpandedLowercaseProbeCases(t, realPageCMRMixedExpandedLowercaseProbeCases(), assertFn)
}

func runRealPageLongTailExpandedLowercaseProbe(
	t *testing.T,
	assertFn realPageExpandedLowercaseProbeAssertFunc,
) {
	t.Helper()
	runRealPageExpandedLowercaseProbeCases(t, realPageSFRMLongTailExpandedLowercaseProbeCases(), assertFn)
}

func runRealPageNonLowerExpandedLowercaseProbe(
	t *testing.T,
	assertFn realPageExpandedLowercaseProbeAssertFunc,
) {
	t.Helper()
	runRealPageExpandedLowercaseProbeCases(t, realPageSFRMNonLowerExpandedLowercaseProbeCases(), assertFn)
}

func runRealPageSFRMExpandedLowercaseProbe(
	t *testing.T,
	assertFn realPageExpandedLowercaseProbeAssertFunc,
) {
	t.Helper()
	runRealPageExpandedLowercaseProbeCases(t, realPageSFRMExpandedLowercaseProbeCases(), assertFn)
}

func assertSingleResidualClassForExpandedCases(
	t *testing.T,
	expected realPageResidualClass,
	tc realPageLowercaseProbeCase,
) {
	t.Helper()
	require.Equal(t, expected, tc.dominantResidualClass())
}
