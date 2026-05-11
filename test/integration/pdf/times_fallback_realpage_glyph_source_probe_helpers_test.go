package pdf_test

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

type realPageGlyphSourceOverrideMeasureFunc func(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	targetOnlyEnv map[string]string,
) realPageGlyphSourceOverrideProbeResult

func runSFRMTargetFontOnlyGlyphSourceOverrideProbeAt72DPI(
	t *testing.T,
	measure realPageGlyphSourceOverrideMeasureFunc,
) {
	runSFRMTargetFontOnlyGlyphSourceOverrideProbeAt72DPIForClass(t, "", measure)
}

func runSFRMTargetFontOnlyGlyphSourceOverrideProbeAt72DPIForClass(
	t *testing.T,
	class realPageResidualClass,
	measure realPageGlyphSourceOverrideMeasureFunc,
) {
	t.Helper()

	if _, err := execLookPathPdftoppmForProbe(); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, tc := range realPageLowercaseProbeCases() {
		if tc.baseFont != "SFRM1095" {
			continue
		}
		if class != "" && tc.dominantResidualClass() != class {
			continue
		}

		allBaseFonts := pageBaseFontsForProbe(t, tc.target)
		if len(allBaseFonts) <= 1 {
			continue
		}

		t.Run(tc.target.name, func(t *testing.T) {
			targetOnlyEnv := targetFontOnlyEnvForProbe(allBaseFonts, tc.baseFont)
			result := measure(t, tc, targetOnlyEnv)
			logAndAssertGlyphSourceOverrideProbeResult(t, tc, result)
		})
	}
}

func execLookPathPdftoppmForProbe() (string, error) {
	return exec.LookPath("pdftoppm")
}

func logAndAssertGlyphSourceOverrideProbeResult(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	result realPageGlyphSourceOverrideProbeResult,
) {
	t.Helper()

	t.Logf(
		"dpi72_expanded_only=%.4f dpi72_glyph_source_only=%.4f dpi72_glyph_source_gain=%.4f dpi72_residual_gap=%.4f expanded_codes=%s target_only_skip=%s glyph_source=%s",
		result.expandedOnly,
		result.overrideOnly,
		result.overrideGain,
		result.residualGap,
		tc.expandedCodeSpec(),
		result.targetOnlySkipEnv,
		result.overrideSpec,
	)

	require.GreaterOrEqual(t, result.overrideGain, 0.0)
	require.Greater(t, result.residualGap, result.overrideGain)
}
