package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func realPageSFRMNonLowerProbeCaseForGlyphSource() realPageLowercaseProbeCase {
	for _, tc := range realPageLowercaseProbeCases() {
		if tc.baseFont == "SFRM1095" && tc.dominantResidualClass() == realPageResidualClassNonLower {
			return tc
		}
	}
	return realPageLowercaseProbeCase{}
}

func runSFRMNonLowerGlyphSourceOverrideProbeAt72DPI(
	t *testing.T,
	measure realPageGlyphSourceOverrideMeasureFunc,
) {
	t.Helper()

	if _, err := execLookPathPdftoppmForProbe(); err != nil {
		t.Skip("pdftoppm not installed")
	}

	tc := realPageSFRMNonLowerProbeCaseForGlyphSource()
	require.Equal(t, realPageResidualClassNonLower, tc.dominantResidualClass())

	allBaseFonts := pageBaseFontsForProbe(t, tc.target)
	require.Greater(t, len(allBaseFonts), 1)

	t.Run(tc.target.name, func(t *testing.T) {
		targetOnlyEnv := targetFontOnlyEnvForProbe(allBaseFonts, tc.baseFont)
		result := measure(t, tc, targetOnlyEnv)
		logAndAssertGlyphSourceOverrideProbeResult(t, tc, result)
	})
}

func nonLowerExpandedGlyphSourceOverrideSpecsForProbe() []string {
	return []string{
		nonLowerSlashGlyphSourceOverrideSpecForProbe(),
		nonLowerAGlyphSourceOverrideSpecForProbe(),
		"SFRM1095:84=Courier",
		"SFRM1095:48=Helvetica",
		"SFRM1095:50=Helvetica",
		"SFRM1095:51=Helvetica",
	}
}

func nonLowerSlashGlyphSourceOverrideSpecForProbe() string {
	return "SFRM1095:47=Helvetica"
}

func nonLowerAGlyphSourceOverrideSpecForProbe() string {
	return "SFRM1095:65=Courier"
}

func nonLowerCoreGlyphSourceOverrideSpecsForProbe() []string {
	return []string{
		nonLowerSlashGlyphSourceOverrideSpecForProbe(),
		nonLowerAGlyphSourceOverrideSpecForProbe(),
	}
}

func measureNonLowerGlyphSourceOverridesProbeAt72DPI(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	targetOnlyEnv map[string]string,
	specs ...string,
) realPageGlyphSourceOverrideProbeResult {
	t.Helper()
	return measureGlyphSourceOverridesProbeAgainstPopplerAtDPI(t, tc, targetOnlyEnv, 72, specs...)
}
