package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageTargetFontOnlyLongTailGlyphGapRemainsLargerThanFastPathGain(t *testing.T) {
	runRealPageSFRMLongTailLowercaseCodeSpecFastPathProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string) {
		require.True(t, tc.hasLongTailCodeSpec())

		result := measureRealPageCodeSpecFastPathProbeAgainstPoppler(t, tc, targetOnlyEnv, tc.longTailCodeSpec)
		t.Logf(
			"current_only=%.4f fast_path_only=%.4f long_tail_only=%.4f fast_path_gain=%.4f long_tail_gap=%.4f long_tail_codes=%s target_only_skip=%s",
			result.currentOnly,
			result.fastPathOnly,
			result.codeSpecOnly,
			result.fastPathGain(),
			result.codeSpecGap(),
			tc.longTailCodeSpec,
			targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
		)

		require.Greater(t, result.codeSpecGap(), result.fastPathGain())
	})
}
