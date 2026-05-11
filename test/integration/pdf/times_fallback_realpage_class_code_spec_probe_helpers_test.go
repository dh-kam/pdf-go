package pdf_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type realPageCodeSpecFastPathProbeResult struct {
	currentOnly  float64
	fastPathOnly float64
	codeSpecOnly float64
}

func (r realPageCodeSpecFastPathProbeResult) fastPathGain() float64 {
	return r.fastPathOnly - r.currentOnly
}

func (r realPageCodeSpecFastPathProbeResult) codeSpecGap() float64 {
	return r.codeSpecOnly - r.currentOnly
}

func singleCodeSkipSpecForProbe(baseFont string, code int) string {
	return fmt.Sprintf("%s=%d", baseFont, code)
}

func measureRealPageCodeSpecFastPathProbeAgainstPoppler(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	targetOnlyEnv map[string]string,
	codeSpec string,
) realPageCodeSpecFastPathProbeResult {
	t.Helper()

	return measureRealPageCodeSpecFastPathProbeAgainstPopplerAtDPI(t, tc, targetOnlyEnv, codeSpec, defaultRealPageProbeDPI)
}

func measureRealPageCodeSpecFastPathProbeAgainstPopplerAtDPI(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	targetOnlyEnv map[string]string,
	codeSpec string,
	dpi int,
) realPageCodeSpecFastPathProbeResult {
	t.Helper()

	popplerPNG := preparePopplerPageForProbeAtDPI(t, tc.target, dpi)
	return realPageCodeSpecFastPathProbeResult{
		currentOnly:  renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, targetOnlyEnv, dpi),
		fastPathOnly: renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, fastPathWithinTargetFontEnvForProbe(targetOnlyEnv), dpi),
		codeSpecOnly: renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, codeSpec), dpi),
	}
}

type realPageCodeSpecFastPathProbeAssertFunc func(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	targetOnlyEnv map[string]string,
)

type realPageSingleCodeFastPathProbeAssertFunc func(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	targetOnlyEnv map[string]string,
	codeSpec string,
)

func runRealPageLowercaseCodeSpecFastPathProbe(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	assertFn realPageCodeSpecFastPathProbeAssertFunc,
) {
	t.Helper()

	if _, err := execLookPathPdftoppmForProbe(); err != nil {
		t.Skip("pdftoppm not installed")
	}

	allBaseFonts := pageBaseFontsForProbe(t, tc.target)
	require.Greater(t, len(allBaseFonts), 1)

	t.Run(tc.target.name, func(t *testing.T) {
		targetOnlyEnv := targetFontOnlyEnvForProbe(allBaseFonts, tc.baseFont)
		assertFn(t, tc, targetOnlyEnv)
	})
}

func runRealPageSFRMNonLowerLowercaseCodeSpecFastPathProbe(
	t *testing.T,
	assertFn realPageCodeSpecFastPathProbeAssertFunc,
) {
	t.Helper()

	tc := realPageSFRMNonLowerProbeCaseForLowercase(t)
	runRealPageLowercaseCodeSpecFastPathProbe(t, tc, assertFn)
}

func runRealPageSFRMNonLowerSingleCodeFastPathProbe(
	t *testing.T,
	code int,
	assertFn realPageSingleCodeFastPathProbeAssertFunc,
) {
	t.Helper()

	tc := realPageSFRMNonLowerProbeCaseForLowercase(t)
	runRealPageLowercaseCodeSpecFastPathProbe(t, tc, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string) {
		assertFn(t, tc, targetOnlyEnv, singleCodeSkipSpecForProbe(tc.baseFont, code))
	})
}

func runRealPageSFRMLongTailLowercaseCodeSpecFastPathProbe(
	t *testing.T,
	assertFn realPageCodeSpecFastPathProbeAssertFunc,
) {
	t.Helper()

	tc := realPageSFRMLongTailProbeCaseForLowercase(t)
	runRealPageLowercaseCodeSpecFastPathProbe(t, tc, assertFn)
}
