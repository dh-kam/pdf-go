package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGlyphSourceOverrideWithinTargetFontEnvForProbe_AddsOverrideSpec(t *testing.T) {
	env := glyphSourceOverrideWithinTargetFontEnvForProbe(
		map[string]string{"PDF_DEBUG_SKIP_TEXT_BASE_FONTS": "CMR10,CMSY10,SFTT0900"},
		"SFRM1095",
		65,
		"Helvetica",
	)
	require.Equal(t, "CMR10,CMSY10,SFTT0900", env["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"])
	require.Equal(t, "SFRM1095:65=Helvetica", env["PDF_DEBUG_FORCE_GLYPH_SOURCE_MAP"])
}

func TestGlyphSourceOverridesWithinTargetFontEnvForProbe_AddsOverrideSpecs(t *testing.T) {
	env := glyphSourceOverridesWithinTargetFontEnvForProbe(
		map[string]string{"PDF_DEBUG_SKIP_TEXT_BASE_FONTS": "CMR10,CMSY10,SFTT0900"},
		"SFRM1095:47=Helvetica",
		"SFRM1095:65=Courier",
	)
	require.Equal(t, "CMR10,CMSY10,SFTT0900", env["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"])
	require.Equal(t, "SFRM1095:47=Helvetica,SFRM1095:65=Courier", env["PDF_DEBUG_FORCE_GLYPH_SOURCE_MAP"])
}

func TestNonLowerExpandedGlyphSourceOverrideSpecsForProbe_ReturnsExpectedCoreSet(t *testing.T) {
	require.Equal(
		t,
		[]string{
			"SFRM1095:47=Helvetica",
			"SFRM1095:65=Courier",
			"SFRM1095:84=Courier",
			"SFRM1095:48=Helvetica",
			"SFRM1095:50=Helvetica",
			"SFRM1095:51=Helvetica",
		},
		nonLowerExpandedGlyphSourceOverrideSpecsForProbe(),
	)
}

func TestResidualClassGlyphSourceOverrideSpecsRemainStable(t *testing.T) {
	require.Equal(t, "SFRM1095:47=Helvetica", nonLowerSlashGlyphSourceOverrideSpecForProbe())
	require.Equal(t, "SFRM1095:65=Courier", nonLowerAGlyphSourceOverrideSpecForProbe())
	require.Equal(
		t,
		[]string{"SFRM1095:47=Helvetica", "SFRM1095:65=Courier"},
		nonLowerCoreGlyphSourceOverrideSpecsForProbe(),
	)
	require.Equal(t, "SFRM1095:118=Courier", longTailVGlyphSourceOverrideSpecForProbe())
}

func TestResidualClassGlyphSourceMeasureHelpers_AreBoundToExpectedSpecs(t *testing.T) {
	require.Equal(
		t,
		[]string{"SFRM1095:47=Helvetica"},
		[]string{nonLowerSlashGlyphSourceOverrideSpecForProbe()},
	)
	require.Equal(
		t,
		[]string{"SFRM1095:118=Courier"},
		[]string{longTailVGlyphSourceOverrideSpecForProbe()},
	)
}

func TestRunSFRMResidualClassGlyphSourceOverrideProbeHelpers_DoNotInvokeMeasureWithoutMatchingCase(t *testing.T) {
	nonLowerCalls := 0
	runSFRMNonLowerGlyphSourceOverrideProbeAt72DPI(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string) realPageGlyphSourceOverrideProbeResult {
		nonLowerCalls++
		require.Equal(t, realPageResidualClassNonLower, tc.dominantResidualClass())
		return realPageGlyphSourceOverrideProbeResult{residualGap: 1.0}
	})
	require.Equal(t, 1, nonLowerCalls)

	longTailCalls := 0
	runSFRMLongTailGlyphSourceOverrideProbeAt72DPI(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string) realPageGlyphSourceOverrideProbeResult {
		longTailCalls++
		require.Equal(t, realPageResidualClassLongTail, tc.dominantResidualClass())
		return realPageGlyphSourceOverrideProbeResult{residualGap: 1.0}
	})
	require.Equal(t, 1, longTailCalls)
}

func TestRealPageSFRMGlyphSourceProbeCaseSelectors_ReturnExpectedResidualClasses(t *testing.T) {
	nonLower := realPageSFRMNonLowerProbeCaseForGlyphSource()
	require.Equal(t, realPageResidualClassNonLower, nonLower.dominantResidualClass())
	require.Equal(t, "009_p109_sfrm1095_top5", nonLower.target.name)

	longTail := realPageSFRMLongTailProbeCaseForGlyphSource()
	require.Equal(t, realPageResidualClassLongTail, longTail.dominantResidualClass())
	require.Equal(t, "009_p95_sfrm1095_top6", longTail.target.name)
}
