package pdf_test

import (
	"math"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageFontSkipGapRemainsLargerThanForcedFamilyGap(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, tc := range realPageFontProbeCases() {
		t.Run(tc.target.name, func(t *testing.T) {
			popplerPNG := preparePopplerPageForProbe(t, tc.target)

			currentSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, nil)
			forcedSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, tc.forcedEnv)
			skipSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, map[string]string{
				"PDF_DEBUG_SKIP_TEXT_BASE_FONTS": tc.skipBaseFonts,
			})

			forcedGap := forcedSimilarity - currentSimilarity
			skipGap := skipSimilarity - currentSimilarity

			t.Logf(
				"current=%.4f forced=%.4f skip=%.4f forced_gap=%.4f skip_gap=%.4f",
				currentSimilarity,
				forcedSimilarity,
				skipSimilarity,
				forcedGap,
				skipGap,
			)

			require.Greater(t, skipGap, forcedGap)
		})
	}
}

func TestRealPageFontSkipGapRemainsLargerThanFastPathGain(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, tc := range realPageFontProbeCases() {
		t.Run(tc.target.name, func(t *testing.T) {
			popplerPNG := preparePopplerPageForProbe(t, tc.target)

			currentSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, nil)
			fastPathSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, map[string]string{
				"PDF_DEBUG_TEXT_RENDER_MODE": "fast-path",
			})
			skipSimilarity := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, map[string]string{
				"PDF_DEBUG_SKIP_TEXT_BASE_FONTS": tc.skipBaseFonts,
			})

			fastPathGain := fastPathSimilarity - currentSimilarity
			skipGap := skipSimilarity - currentSimilarity

			t.Logf(
				"current=%.4f fast_path=%.4f skip=%.4f fast_path_gain=%.4f skip_gap=%.4f",
				currentSimilarity,
				fastPathSimilarity,
				skipSimilarity,
				fastPathGain,
				skipGap,
			)

			require.Greater(t, skipGap, fastPathGain)
		})
	}
}

func TestRealPageTargetFontOnlyForcedFamilyGapRemainsSmall(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, tc := range realPageFontProbeCases() {
		allBaseFonts := pageBaseFontsForProbe(t, tc.target)
		if len(allBaseFonts) <= 1 {
			continue
		}

		t.Run(tc.target.name, func(t *testing.T) {
			popplerPNG := preparePopplerPageForProbe(t, tc.target)
			targetOnlyEnv := targetFontOnlyEnvForProbe(allBaseFonts, tc.skipBaseFonts)

			currentOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, targetOnlyEnv)
			forcedOnlyEnv := mergeProbeEnv(targetOnlyEnv, tc.forcedEnv)
			forcedOnly := renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, forcedOnlyEnv)
			gap := math.Abs(forcedOnly - currentOnly)

			t.Logf(
				"current_only=%.4f forced_only=%.4f abs_gap=%.4f target_only_skip=%s",
				currentOnly,
				forcedOnly,
				gap,
				targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
			)

			require.Less(t, gap, 0.05)
		})
	}
}
