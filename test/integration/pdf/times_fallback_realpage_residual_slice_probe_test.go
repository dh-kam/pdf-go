package pdf_test

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageTargetFontOnlyResidualSliceForcedFamilyGainRemainsSmallerThanResidualGap(t *testing.T) {
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
			result := measureExpandedResidualProbeAgainstPoppler(t, tc, targetOnlyEnv, popplerPNG)

			t.Logf(
				"expanded_only=%.4f forced_residual_only=%.4f full_skip_only=%.4f forced_residual_gain=%.4f residual_gap=%.4f expanded_codes=%s target_only_skip=%s",
				result.expandedOnly,
				result.forcedResidualOnly,
				result.fullSkipOnly,
				result.forcedResidualGain,
				result.residualGap,
				tc.expandedCodeSpec(),
				result.targetOnlySkipBaseEnv,
			)

			require.Greater(t, result.residualGap, result.forcedResidualGain)
		})
	}
}

func TestRealPageTargetFontOnlyResidualSliceFastPathGainRemainsSmallerThanResidualGap(t *testing.T) {
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
			result := measureExpandedResidualProbeAgainstPoppler(t, tc, targetOnlyEnv, popplerPNG)

			t.Logf(
				"expanded_only=%.4f fast_path_residual_only=%.4f full_skip_only=%.4f fast_path_residual_gain=%.4f residual_gap=%.4f expanded_codes=%s target_only_skip=%s",
				result.expandedOnly,
				result.fastPathResidualOnly,
				result.fullSkipOnly,
				result.fastPathResidualGain,
				result.residualGap,
				tc.expandedCodeSpec(),
				result.targetOnlySkipBaseEnv,
			)

			require.Greater(t, result.residualGap, result.fastPathResidualGain)
		})
	}
}

func TestRealPageTargetFontOnlyResidualSliceForcedFastPathGainRemainsSmallerThanResidualGap(t *testing.T) {
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
			result := measureExpandedResidualProbeAgainstPoppler(t, tc, targetOnlyEnv, popplerPNG)

			t.Logf(
				"expanded_only=%.4f forced_fast_path_only=%.4f full_skip_only=%.4f forced_fast_path_gain=%.4f residual_gap=%.4f expanded_codes=%s target_only_skip=%s",
				result.expandedOnly,
				result.forcedFastPathOnly,
				result.fullSkipOnly,
				result.forcedFastPathGain,
				result.residualGap,
				tc.expandedCodeSpec(),
				result.targetOnlySkipBaseEnv,
			)

			require.Greater(t, result.residualGap, result.forcedFastPathGain)
		})
	}
}

func TestRealPageTargetFontOnlyResidualSliceEmbeddedGainRemainsSmallerThanResidualGap(t *testing.T) {
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
			result := measureExpandedResidualProbeAgainstPoppler(t, tc, targetOnlyEnv, popplerPNG)

			t.Logf(
				"expanded_only=%.4f embedded_residual_only=%.4f full_skip_only=%.4f embedded_residual_gain=%.4f residual_gap=%.4f expanded_codes=%s target_only_skip=%s",
				result.expandedOnly,
				result.embeddedResidualOnly,
				result.fullSkipOnly,
				result.embeddedResidualGain,
				result.residualGap,
				tc.expandedCodeSpec(),
				result.targetOnlySkipBaseEnv,
			)

			require.Greater(t, result.residualGap, result.embeddedResidualGain)
		})
	}
}

func TestRealPageTargetFontOnlyResidualSliceFallbackGainRemainsSmallerThanResidualGap(t *testing.T) {
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
			result := measureExpandedResidualProbeAgainstPoppler(t, tc, targetOnlyEnv, popplerPNG)

			t.Logf(
				"expanded_only=%.4f fallback_residual_only=%.4f full_skip_only=%.4f fallback_residual_gain=%.4f residual_gap=%.4f expanded_codes=%s target_only_skip=%s",
				result.expandedOnly,
				result.fallbackResidualOnly,
				result.fullSkipOnly,
				result.fallbackResidualGain,
				result.residualGap,
				tc.expandedCodeSpec(),
				result.targetOnlySkipBaseEnv,
			)

			require.Greater(t, result.residualGap, result.fallbackResidualGain)
		})
	}
}

func TestRealPageTargetFontOnlyResidualSliceMaxPolicyGainRemainsSmallerThanResidualGap(t *testing.T) {
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
			result := measureExpandedResidualProbeAgainstPoppler(t, tc, targetOnlyEnv, popplerPNG)

			t.Logf(
				"expanded_only=%.4f full_skip_only=%.4f max_policy_gain=%.4f residual_gap=%.4f expanded_codes=%s target_only_skip=%s",
				result.expandedOnly,
				result.fullSkipOnly,
				result.maxPolicyGain(),
				result.residualGap,
				tc.expandedCodeSpec(),
				result.targetOnlySkipBaseEnv,
			)

			require.Greater(t, result.residualGap, result.maxPolicyGain())
		})
	}
}
