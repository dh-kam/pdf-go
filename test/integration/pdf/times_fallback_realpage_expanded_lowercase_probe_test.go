package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageTargetFontOnlyExpandedGlyphSetBeatsBroadGap(t *testing.T) {
	assertFn := func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageExpandedLowercaseProbeResult) {
		t.Logf(
			"current_only=%.4f broad_only=%.4f expanded_only=%.4f broad_gap=%.4f expanded_gap=%.4f expanded_codes=%s target_only_skip=%s",
			result.currentOnly,
			result.broadOnly,
			result.expandedOnly,
			result.broadGap(),
			result.expandedGap(),
			tc.broadWithNonLowerCodeSpec(),
			targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
		)

		require.Greater(t, result.expandedGap(), result.broadGap())
	}

	runRealPageSFRMExpandedLowercaseProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageExpandedLowercaseProbeResult) {
		require.Contains(t, []realPageResidualClass{
			realPageResidualClassLongTail,
			realPageResidualClassNonLower,
		}, tc.dominantResidualClass())
		assertFn(t, tc, targetOnlyEnv, result)
	})
}

func TestRealPageTargetFontOnlyExpandedGlyphSetCoverageRemainsBelowFullFontSkip(t *testing.T) {
	assertFn := func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageExpandedLowercaseProbeResult) {
		t.Logf(
			"current_only=%.4f full_skip_only=%.4f expanded_only=%.4f full_gap=%.4f expanded_gap=%.4f coverage=%.4f expanded_codes=%s target_only_skip=%s",
			result.currentOnly,
			result.fullSkipOnly,
			result.expandedOnly,
			result.fullGap(),
			result.expandedGap(),
			result.expandedCoverage(),
			tc.broadWithNonLowerCodeSpec(),
			targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
		)

		require.Greater(t, result.fullGap(), 0.0)
		require.Greater(t, result.expandedGap(), 0.0)
		require.Less(t, result.expandedCoverage(), 1.0)
	}

	runRealPageSFRMExpandedLowercaseProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageExpandedLowercaseProbeResult) {
		require.Contains(t, []realPageResidualClass{
			realPageResidualClassLongTail,
			realPageResidualClassNonLower,
		}, tc.dominantResidualClass())
		assertFn(t, tc, targetOnlyEnv, result)
	})
}

func TestRealPageTargetFontOnlyResidualAfterExpandedGlyphSetRemainsLargerThanFastPathGain(t *testing.T) {
	assertFn := func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageExpandedLowercaseProbeResult) {
		t.Logf(
			"current_only=%.4f fast_path_only=%.4f expanded_only=%.4f full_skip_only=%.4f fast_path_gain=%.4f residual_gap=%.4f expanded_codes=%s target_only_skip=%s",
			result.currentOnly,
			result.fastPathOnly,
			result.expandedOnly,
			result.fullSkipOnly,
			result.fastPathGain(),
			result.residualGap(),
			tc.expandedCodeSpec(),
			targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
		)

		require.Greater(t, result.residualGap(), result.fastPathGain())
	}

	runRealPageSFRMExpandedLowercaseProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageExpandedLowercaseProbeResult) {
		require.Contains(t, []realPageResidualClass{
			realPageResidualClassLongTail,
			realPageResidualClassNonLower,
		}, tc.dominantResidualClass())
		assertFn(t, tc, targetOnlyEnv, result)
	})
}

func TestRealPageTargetFontOnlyResidualAfterExpandedGlyphSetRemainsLargerThanForcedFamilyGap(t *testing.T) {
	assertFn := func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageExpandedLowercaseProbeResult) {
		t.Logf(
			"current_only=%.4f forced_only=%.4f expanded_only=%.4f full_skip_only=%.4f forced_gap=%.4f residual_gap=%.4f expanded_codes=%s target_only_skip=%s",
			result.currentOnly,
			result.forcedOnly,
			result.expandedOnly,
			result.fullSkipOnly,
			result.forcedGap(),
			result.residualGap(),
			tc.expandedCodeSpec(),
			targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
		)

		require.Greater(t, result.residualGap(), result.forcedGap())
	}

	runRealPageSFRMExpandedLowercaseProbe(t, func(t *testing.T, tc realPageLowercaseProbeCase, targetOnlyEnv map[string]string, result realPageExpandedLowercaseProbeResult) {
		require.Contains(t, []realPageResidualClass{
			realPageResidualClassLongTail,
			realPageResidualClassNonLower,
		}, tc.dominantResidualClass())
		assertFn(t, tc, targetOnlyEnv, result)
	})
}
