package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageTargetFontOnlyBroadGlyphSetBeatsCombinedGap(t *testing.T) {
	assertFn := func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageBroadLowercaseProbeResult) {
		t.Logf(
			"current_only=%.4f combined_only=%.4f broad_only=%.4f combined_gap=%.4f broad_gap=%.4f broad_codes=%s target_only_skip=%s",
			result.currentOnly,
			result.combinedOnly,
			result.broadOnly,
			result.combinedGap(),
			result.broadGap(),
			tc.broadCodeSpec(),
			targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
		)

		require.Greater(t, result.broadGap(), result.combinedGap())
	}

	runRealPageSFRMBroadLowercaseProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageBroadLowercaseProbeResult) {
		require.Contains(t, []realPageResidualClass{
			realPageResidualClassLongTail,
			realPageResidualClassNonLower,
		}, tc.dominantResidualClass())
		assertFn(t, tc, targetOnlyEnv, result)
	})
}

func TestRealPageTargetFontOnlyBroadGlyphSetCoverageRemainsBelowFullFontSkip(t *testing.T) {
	assertFn := func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageBroadLowercaseProbeResult) {
		t.Logf(
			"current_only=%.4f full_skip_only=%.4f broad_only=%.4f full_gap=%.4f broad_gap=%.4f coverage=%.4f broad_codes=%s target_only_skip=%s",
			result.currentOnly,
			result.fullSkipOnly,
			result.broadOnly,
			result.fullGap(),
			result.broadGap(),
			result.broadCoverage(),
			tc.broadCodeSpec(),
			targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
		)

		require.Greater(t, result.fullGap(), 0.0)
		require.Greater(t, result.broadGap(), 0.0)
		require.Less(t, result.broadCoverage(), 1.0)
	}

	runRealPageSFRMBroadLowercaseProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageBroadLowercaseProbeResult) {
		require.Contains(t, []realPageResidualClass{
			realPageResidualClassLongTail,
			realPageResidualClassNonLower,
		}, tc.dominantResidualClass())
		assertFn(t, tc, targetOnlyEnv, result)
	})
}
