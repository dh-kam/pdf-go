package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageTargetFontOnlyTertiaryGlyphGapRemainsSmallerThanFastPathGain(t *testing.T) {
	runRealPageSFRMNonLowerLowercaseCodeSpecFastPathProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string) {
		require.True(t, tc.hasTertiaryCodeSpec())

		result := measureRealPageCodeSpecFastPathProbeAgainstPoppler(t, tc, targetOnlyEnv, tc.tertiaryCodeSpec)
		t.Logf(
			"current_only=%.4f fast_path_only=%.4f tertiary_only=%.4f fast_path_gain=%.4f tertiary_gap=%.4f tertiary_codes=%s target_only_skip=%s",
			result.currentOnly,
			result.fastPathOnly,
			result.codeSpecOnly,
			result.fastPathGain(),
			result.codeSpecGap(),
			tc.tertiaryCodeSpec,
			targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
		)

		require.Less(t, result.codeSpecGap(), result.fastPathGain())
	})
}

func TestRealPageTargetFontOnlyNonLowerGlyphGapRemainsLargerThanFastPathGain(t *testing.T) {
	runRealPageSFRMNonLowerLowercaseCodeSpecFastPathProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string) {
		require.True(t, tc.hasNonLowerCodeSpec())

		result := measureRealPageCodeSpecFastPathProbeAgainstPoppler(t, tc, targetOnlyEnv, tc.nonLowerCodeSpec)
		t.Logf(
			"current_only=%.4f fast_path_only=%.4f non_lower_only=%.4f fast_path_gain=%.4f non_lower_gap=%.4f non_lower_codes=%s target_only_skip=%s",
			result.currentOnly,
			result.fastPathOnly,
			result.codeSpecOnly,
			result.fastPathGain(),
			result.codeSpecGap(),
			tc.nonLowerCodeSpec,
			targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
		)

		require.Greater(t, result.codeSpecGap(), result.fastPathGain())
	})
}
