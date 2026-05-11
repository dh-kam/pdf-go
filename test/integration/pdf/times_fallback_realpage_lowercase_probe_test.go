package pdf_test

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageTargetFontOnlySecondaryLowercaseGapRemainsLargerThanFastPathGain(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, tc := range realPageLowercaseProbeCases() {
		allBaseFonts := pageBaseFontsForProbe(t, tc.target)
		if len(allBaseFonts) <= 1 {
			continue
		}

		t.Run(tc.target.name, func(t *testing.T) {
			popplerPNG := preparePopplerPageForProbe(t, tc.target)
			targetOnlyEnv := targetFontOnlyEnvForProbe(allBaseFonts, tc.baseFont)

			currentOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, targetOnlyEnv)
			fastPathOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, fastPathWithinTargetFontEnvForProbe(targetOnlyEnv))
			secondaryOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, tc.secondaryCodeSpec))

			fastPathGain := fastPathOnly - currentOnly
			secondaryGap := secondaryOnly - currentOnly

			t.Logf(
				"current_only=%.4f fast_path_only=%.4f secondary_only=%.4f fast_path_gain=%.4f secondary_gap=%.4f target_only_skip=%s",
				currentOnly,
				fastPathOnly,
				secondaryOnly,
				fastPathGain,
				secondaryGap,
				targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
			)

			require.Greater(t, secondaryGap, fastPathGain)
		})
	}
}

func TestRealPageTargetFontOnlyCombinedLowercaseGapBeatsTopSetGap(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, tc := range realPageLowercaseProbeCases() {
		allBaseFonts := pageBaseFontsForProbe(t, tc.target)
		if len(allBaseFonts) <= 1 {
			continue
		}

		t.Run(tc.target.name, func(t *testing.T) {
			popplerPNG := preparePopplerPageForProbe(t, tc.target)
			targetOnlyEnv := targetFontOnlyEnvForProbe(allBaseFonts, tc.baseFont)
			combinedCodeSpec := tc.combinedCodeSpec()

			currentOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, targetOnlyEnv)
			topSetOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, tc.topSetCodeSpec))
			combinedOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, combinedCodeSpec))

			topSetGap := topSetOnly - currentOnly
			combinedGap := combinedOnly - currentOnly

			t.Logf(
				"current_only=%.4f top_set_only=%.4f combined_only=%.4f top_set_gap=%.4f combined_gap=%.4f combined_codes=%s target_only_skip=%s",
				currentOnly,
				topSetOnly,
				combinedOnly,
				topSetGap,
				combinedGap,
				combinedCodeSpec,
				targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
			)

			require.Greater(t, combinedGap, topSetGap)
		})
	}
}

func TestRealPageTargetFontOnlyCombinedLowercaseCoverageRemainsBelowFullFontSkip(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, tc := range realPageLowercaseProbeCases() {
		allBaseFonts := pageBaseFontsForProbe(t, tc.target)
		if len(allBaseFonts) <= 1 {
			continue
		}

		t.Run(tc.target.name, func(t *testing.T) {
			popplerPNG := preparePopplerPageForProbe(t, tc.target)
			targetOnlyEnv := targetFontOnlyEnvForProbe(allBaseFonts, tc.baseFont)
			combinedCodeSpec := tc.combinedCodeSpec()

			currentOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, targetOnlyEnv)
			fullSkipOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, fullTargetFontSkipEnvForProbe(targetOnlyEnv, tc.baseFont))
			combinedOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, combinedCodeSpec))

			fullGap := fullSkipOnly - currentOnly
			combinedGap := combinedOnly - currentOnly
			coverage := combinedGap / fullGap

			t.Logf(
				"current_only=%.4f full_skip_only=%.4f combined_only=%.4f full_gap=%.4f combined_gap=%.4f coverage=%.4f combined_codes=%s target_only_skip=%s",
				currentOnly,
				fullSkipOnly,
				combinedOnly,
				fullGap,
				combinedGap,
				coverage,
				combinedCodeSpec,
				targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
			)

			require.Greater(t, fullGap, 0.0)
			require.Greater(t, combinedGap, 0.0)
			require.Less(t, coverage, 1.0)
		})
	}
}

func TestRealPageTargetFontOnlyResidualAfterCombinedLowercaseRemainsLargerThanFastPathGain(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, tc := range realPageLowercaseProbeCases() {
		allBaseFonts := pageBaseFontsForProbe(t, tc.target)
		if len(allBaseFonts) <= 1 {
			continue
		}

		t.Run(tc.target.name, func(t *testing.T) {
			popplerPNG := preparePopplerPageForProbe(t, tc.target)
			targetOnlyEnv := targetFontOnlyEnvForProbe(allBaseFonts, tc.baseFont)
			combinedCodeSpec := tc.combinedCodeSpec()

			currentOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, targetOnlyEnv)
			fastPathOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, fastPathWithinTargetFontEnvForProbe(targetOnlyEnv))
			combinedOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, combinedCodeSpec))
			fullSkipOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, fullTargetFontSkipEnvForProbe(targetOnlyEnv, tc.baseFont))

			fastPathGain := fastPathOnly - currentOnly
			residualGap := fullSkipOnly - combinedOnly

			t.Logf(
				"current_only=%.4f fast_path_only=%.4f combined_only=%.4f full_skip_only=%.4f fast_path_gain=%.4f residual_gap=%.4f combined_codes=%s target_only_skip=%s",
				currentOnly,
				fastPathOnly,
				combinedOnly,
				fullSkipOnly,
				fastPathGain,
				residualGap,
				combinedCodeSpec,
				targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
			)

			require.Greater(t, residualGap, fastPathGain)
		})
	}
}
