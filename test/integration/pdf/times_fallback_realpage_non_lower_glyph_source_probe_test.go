package pdf_test

import "testing"

func TestRealPageTargetFontOnlyResidualSliceSlashGlyphSourceOverrideGainAt72DPI(t *testing.T) {
	runSFRMNonLowerGlyphSourceOverrideProbeAt72DPI(
		t,
		func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string) realPageGlyphSourceOverrideProbeResult {
			return measureNonLowerGlyphSourceOverridesProbeAt72DPI(
				t,
				tc,
				targetOnlyEnv,
				nonLowerSlashGlyphSourceOverrideSpecForProbe(),
			)
		},
	)
}

func TestRealPageTargetFontOnlyResidualSliceAGlyphSourceOverrideGainAt72DPI(t *testing.T) {
	runSFRMNonLowerGlyphSourceOverrideProbeAt72DPI(
		t,
		func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string) realPageGlyphSourceOverrideProbeResult {
			return measureNonLowerGlyphSourceOverridesProbeAt72DPI(
				t,
				tc,
				targetOnlyEnv,
				nonLowerAGlyphSourceOverrideSpecForProbe(),
			)
		},
	)
}

func TestRealPageTargetFontOnlyResidualSliceNonLowerCoreGlyphSourceOverridesGainAt72DPI(t *testing.T) {
	runSFRMNonLowerGlyphSourceOverrideProbeAt72DPI(
		t,
		func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string) realPageGlyphSourceOverrideProbeResult {
			return measureNonLowerGlyphSourceOverridesProbeAt72DPI(
				t,
				tc,
				targetOnlyEnv,
				nonLowerCoreGlyphSourceOverrideSpecsForProbe()...,
			)
		},
	)
}

func TestRealPageTargetFontOnlyResidualSliceNonLowerExpandedGlyphSourceOverridesGainAt72DPI(t *testing.T) {
	runSFRMNonLowerGlyphSourceOverrideProbeAt72DPI(
		t,
		func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string) realPageGlyphSourceOverrideProbeResult {
			return measureNonLowerGlyphSourceOverridesProbeAt72DPI(
				t,
				tc,
				targetOnlyEnv,
				nonLowerExpandedGlyphSourceOverrideSpecsForProbe()...,
			)
		},
	)
}
