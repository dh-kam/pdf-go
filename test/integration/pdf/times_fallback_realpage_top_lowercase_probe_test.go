package pdf_test

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageTopLowercaseCoverageRemainsBelowFullFontSkip(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, tc := range realPageLowercaseProbeCases() {
		t.Run(tc.target.name, func(t *testing.T) {
			popplerPNG := preparePopplerPageForProbe(t, tc.target)

			currentSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, nil)
			fullSkipSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, map[string]string{
				"PDF_DEBUG_SKIP_TEXT_BASE_FONTS": tc.baseFont,
			})
			partialSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, map[string]string{
				"PDF_DEBUG_SKIP_TEXT_CODES_FOR_BASE": tc.topSetCodeSpec,
			})

			fullGap := fullSkipSimilarity - currentSimilarity
			partialGap := partialSimilarity - currentSimilarity
			coverage := partialGap / fullGap

			t.Logf(
				"current=%.4f full_skip=%.4f partial=%.4f full_gap=%.4f partial_gap=%.4f coverage=%.4f",
				currentSimilarity,
				fullSkipSimilarity,
				partialSimilarity,
				fullGap,
				partialGap,
				coverage,
			)

			require.Greater(t, fullGap, 0.0)
			require.Greater(t, partialGap, 0.0)
			require.Less(t, coverage, tc.maxCoverage)
			require.Greater(t, coverage, tc.minCoverage)
		})
	}
}

func TestRealPageTopLowercaseGapRemainsLargerThanFastPathGain(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, tc := range realPageLowercaseProbeCases() {
		t.Run(tc.target.name, func(t *testing.T) {
			popplerPNG := preparePopplerPageForProbe(t, tc.target)

			currentSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, nil)
			fastPathSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, map[string]string{
				"PDF_DEBUG_TEXT_RENDER_MODE": "fast-path",
			})
			partialSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, map[string]string{
				"PDF_DEBUG_SKIP_TEXT_CODES_FOR_BASE": tc.topSetCodeSpec,
			})

			fastPathGain := fastPathSimilarity - currentSimilarity
			partialGap := partialSimilarity - currentSimilarity

			t.Logf(
				"current=%.4f fast_path=%.4f partial=%.4f fast_path_gain=%.4f partial_gap=%.4f",
				currentSimilarity,
				fastPathSimilarity,
				partialSimilarity,
				fastPathGain,
				partialGap,
			)

			require.Greater(t, partialGap, fastPathGain)
		})
	}
}

func TestRealPageTopLowercaseGapRemainsLargerThanForcedFamilyGap(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, tc := range realPageLowercaseProbeCases() {
		t.Run(tc.target.name, func(t *testing.T) {
			popplerPNG := preparePopplerPageForProbe(t, tc.target)

			currentSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, nil)
			forcedSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, tc.forcedEnv)
			partialSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, map[string]string{
				"PDF_DEBUG_SKIP_TEXT_CODES_FOR_BASE": tc.topSetCodeSpec,
			})

			forcedGap := forcedSimilarity - currentSimilarity
			partialGap := partialSimilarity - currentSimilarity

			t.Logf(
				"current=%.4f forced=%.4f partial=%.4f forced_gap=%.4f partial_gap=%.4f",
				currentSimilarity,
				forcedSimilarity,
				partialSimilarity,
				forcedGap,
				partialGap,
			)

			require.Greater(t, partialGap, forcedGap)
		})
	}
}

func TestRealPageTopLowercaseGapRemainsLargerThanSingleGlyphGap(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, tc := range realPageLowercaseProbeCases() {
		t.Run(tc.target.name, func(t *testing.T) {
			popplerPNG := preparePopplerPageForProbe(t, tc.target)

			currentSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, nil)
			singleSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, map[string]string{
				"PDF_DEBUG_SKIP_TEXT_CODES_FOR_BASE": tc.singleCodeSpec,
			})
			topSetSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, map[string]string{
				"PDF_DEBUG_SKIP_TEXT_CODES_FOR_BASE": tc.topSetCodeSpec,
			})

			singleGap := singleSimilarity - currentSimilarity
			topSetGap := topSetSimilarity - currentSimilarity

			t.Logf(
				"current=%.4f single=%.4f top_set=%.4f single_gap=%.4f top_set_gap=%.4f",
				currentSimilarity,
				singleSimilarity,
				topSetSimilarity,
				singleGap,
				topSetGap,
			)

			require.Greater(t, topSetGap, singleGap)
		})
	}
}

func TestRealPageTargetFontOnlyTopLowercaseGapRemainsLargerThanForcedFamilyGap(t *testing.T) {
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
			forcedOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, mergeProbeEnv(targetOnlyEnv, tc.forcedEnv))
			topSetOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, tc.topSetCodeSpec))

			forcedGap := forcedOnly - currentOnly
			topSetGap := topSetOnly - currentOnly

			t.Logf(
				"current_only=%.4f forced_only=%.4f top_set_only=%.4f forced_gap=%.4f top_set_gap=%.4f target_only_skip=%s",
				currentOnly,
				forcedOnly,
				topSetOnly,
				forcedGap,
				topSetGap,
				targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
			)

			require.Greater(t, topSetGap, forcedGap)
		})
	}
}

func TestRealPageTargetFontOnlyTopLowercaseCoverageRemainsBelowFullFontSkip(t *testing.T) {
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
			fullSkipOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, fullTargetFontSkipEnvForProbe(targetOnlyEnv, tc.baseFont))
			topSetOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, tc.topSetCodeSpec))

			fullGap := fullSkipOnly - currentOnly
			topSetGap := topSetOnly - currentOnly
			coverage := topSetGap / fullGap

			t.Logf(
				"current_only=%.4f full_skip_only=%.4f top_set_only=%.4f full_gap=%.4f top_set_gap=%.4f coverage=%.4f target_only_skip=%s",
				currentOnly,
				fullSkipOnly,
				topSetOnly,
				fullGap,
				topSetGap,
				coverage,
				targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
			)

			require.Greater(t, fullGap, 0.0)
			require.Greater(t, topSetGap, 0.0)
			require.Less(t, coverage, tc.maxCoverage)
			require.Greater(t, coverage, tc.minCoverage)
		})
	}
}

func TestRealPageTargetFontOnlyTopLowercaseGapRemainsLargerThanFastPathGain(t *testing.T) {
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
			topSetOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, tc.topSetCodeSpec))

			fastPathGain := fastPathOnly - currentOnly
			topSetGap := topSetOnly - currentOnly

			t.Logf(
				"current_only=%.4f fast_path_only=%.4f top_set_only=%.4f fast_path_gain=%.4f top_set_gap=%.4f target_only_skip=%s",
				currentOnly,
				fastPathOnly,
				topSetOnly,
				fastPathGain,
				topSetGap,
				targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
			)

			require.Greater(t, topSetGap, fastPathGain)
		})
	}
}

func TestRealPageTargetFontOnlyTopLowercaseGapRemainsLargerThanSingleGlyphGap(t *testing.T) {
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
			singleOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, tc.singleCodeSpec))
			topSetOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, tc.topSetCodeSpec))

			singleGap := singleOnly - currentOnly
			topSetGap := topSetOnly - currentOnly

			t.Logf(
				"current_only=%.4f single_only=%.4f top_set_only=%.4f single_gap=%.4f top_set_gap=%.4f target_only_skip=%s",
				currentOnly,
				singleOnly,
				topSetOnly,
				singleGap,
				topSetGap,
				targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
			)

			require.Greater(t, topSetGap, singleGap)
		})
	}
}

func TestRealPageTargetFontOnlyResidualAfterTopLowercaseRemainsLargerThanFastPathGain(t *testing.T) {
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
			topSetOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, tc.topSetCodeSpec))
			fullSkipOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, fullTargetFontSkipEnvForProbe(targetOnlyEnv, tc.baseFont))

			fastPathGain := fastPathOnly - currentOnly
			residualGap := fullSkipOnly - topSetOnly

			t.Logf(
				"current_only=%.4f fast_path_only=%.4f top_set_only=%.4f full_skip_only=%.4f fast_path_gain=%.4f residual_gap=%.4f target_only_skip=%s",
				currentOnly,
				fastPathOnly,
				topSetOnly,
				fullSkipOnly,
				fastPathGain,
				residualGap,
				targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
			)

			require.Greater(t, residualGap, fastPathGain)
		})
	}
}
