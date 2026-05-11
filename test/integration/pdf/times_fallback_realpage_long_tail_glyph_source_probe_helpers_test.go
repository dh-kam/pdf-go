package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func realPageSFRMLongTailProbeCaseForGlyphSource() realPageLowercaseProbeCase {
	for _, tc := range realPageLowercaseProbeCases() {
		if tc.baseFont == "SFRM1095" && tc.dominantResidualClass() == realPageResidualClassLongTail {
			return tc
		}
	}
	return realPageLowercaseProbeCase{}
}

func runSFRMLongTailGlyphSourceOverrideProbeAt72DPI(
	t *testing.T,
	measure realPageGlyphSourceOverrideMeasureFunc,
) {
	t.Helper()

	if _, err := execLookPathPdftoppmForProbe(); err != nil {
		t.Skip("pdftoppm not installed")
	}

	tc := realPageSFRMLongTailProbeCaseForGlyphSource()
	require.Equal(t, realPageResidualClassLongTail, tc.dominantResidualClass())

	allBaseFonts := pageBaseFontsForProbe(t, tc.target)
	require.Greater(t, len(allBaseFonts), 1)

	t.Run(tc.target.name, func(t *testing.T) {
		targetOnlyEnv := targetFontOnlyEnvForProbe(allBaseFonts, tc.baseFont)
		result := measure(t, tc, targetOnlyEnv)
		logAndAssertGlyphSourceOverrideProbeResult(t, tc, result)
	})
}

func longTailVGlyphSourceOverrideSpecForProbe() string {
	return "SFRM1095:118=Courier"
}

func measureLongTailGlyphSourceOverridesProbeAt72DPI(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	targetOnlyEnv map[string]string,
	specs ...string,
) realPageGlyphSourceOverrideProbeResult {
	t.Helper()
	return measureGlyphSourceOverridesProbeAgainstPopplerAtDPI(t, tc, targetOnlyEnv, 72, specs...)
}
