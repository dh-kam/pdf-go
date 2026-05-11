package pdf_test

import "testing"

func TestRealPageTargetFontOnlyResidualSliceLongTailVGlyphSourceOverrideGainAt72DPI(t *testing.T) {
	runSFRMLongTailGlyphSourceOverrideProbeAt72DPI(
		t,
		func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string) realPageGlyphSourceOverrideProbeResult {
			return measureLongTailGlyphSourceOverridesProbeAt72DPI(
				t,
				tc,
				targetOnlyEnv,
				longTailVGlyphSourceOverrideSpecForProbe(),
			)
		},
	)
}
