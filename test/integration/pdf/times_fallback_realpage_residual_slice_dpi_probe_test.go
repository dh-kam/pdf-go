package pdf_test

import (
	"math"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/test/testutil"
)

func TestRealPageTargetFontOnlyExpandedResidualGapShrinksAtHigherDPI(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, tc := range realPageLowercaseProbeCases() {
		if tc.baseFont != "SFRM1095" {
			continue
		}

		allBaseFonts := pageBaseFontsForProbe(t, tc.target)
		if len(allBaseFonts) <= 1 {
			continue
		}

		t.Run(tc.target.name, func(t *testing.T) {
			targetOnlyEnv := targetFontOnlyEnvForProbe(allBaseFonts, tc.baseFont)
			result := measureExpandedResidualGapAcrossDPIForProbe(t, tc, targetOnlyEnv)

			t.Logf(
				"dpi72_expanded=%.4f dpi72_full_skip=%.4f dpi72_residual_gap=%.4f dpi150_expanded=%.4f dpi150_full_skip=%.4f dpi150_residual_gap=%.4f delta=%.4f expanded_codes=%s target_only_skip=%s",
				result.dpi72ExpandedOnly,
				result.dpi72FullSkipOnly,
				result.dpi72ResidualGap,
				result.dpi150ExpandedOnly,
				result.dpi150FullSkipOnly,
				result.dpi150ResidualGap,
				result.residualGapDelta(),
				tc.expandedCodeSpec(),
				targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
			)

			require.Greater(t, result.dpi72ResidualGap, 0.0)
			require.Greater(t, result.dpi150ResidualGap, 0.0)
			require.Greater(t, result.dpi72ResidualGap, result.dpi150ResidualGap)
			require.Less(t, result.residualGapDelta(), 0.0)
		})
	}
}

func TestRealPageTargetFontOnlyResidualSliceMaxPolicyGainRemainsSmallerThanResidualGapAt72DPI(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, tc := range realPageLowercaseProbeCases() {
		if tc.baseFont != "SFRM1095" {
			continue
		}

		allBaseFonts := pageBaseFontsForProbe(t, tc.target)
		if len(allBaseFonts) <= 1 {
			continue
		}

		t.Run(tc.target.name, func(t *testing.T) {
			targetOnlyEnv := targetFontOnlyEnvForProbe(allBaseFonts, tc.baseFont)
			result := measureExpandedResidualProbeAgainstPopplerAtDPI(t, tc, targetOnlyEnv, 72)

			t.Logf(
				"dpi72_expanded_only=%.4f dpi72_full_skip_only=%.4f dpi72_max_policy_gain=%.4f dpi72_residual_gap=%.4f expanded_codes=%s target_only_skip=%s",
				result.expandedOnly,
				result.fullSkipOnly,
				result.maxPolicyGain(),
				result.residualGap,
				tc.expandedCodeSpec(),
				targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
			)

			require.Greater(t, result.residualGap, 0.0)
			require.GreaterOrEqual(t, result.maxPolicyGain(), 0.0)
			require.Greater(t, result.residualGap, result.maxPolicyGain())
		})
	}
}

func TestRealPageTargetFontOnlyResidualSliceGlyphSupersampleGainAt72DPI(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, tc := range realPageLowercaseProbeCases() {
		if tc.baseFont != "SFRM1095" {
			continue
		}

		allBaseFonts := pageBaseFontsForProbe(t, tc.target)
		if len(allBaseFonts) <= 1 {
			continue
		}

		t.Run(tc.target.name, func(t *testing.T) {
			targetOnlyEnv := targetFontOnlyEnvForProbe(allBaseFonts, tc.baseFont)
			result := measureExpandedResidualProbeAgainstPopplerAtDPI(t, tc, targetOnlyEnv, 72)

			t.Logf(
				"dpi72_expanded_only=%.4f dpi72_supersampled_only=%.4f dpi72_supersampled_gain=%.4f dpi72_residual_gap=%.4f expanded_codes=%s target_only_skip=%s",
				result.expandedOnly,
				result.supersampledOnly,
				result.supersampledGain,
				result.residualGap,
				tc.expandedCodeSpec(),
				targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
			)

			require.GreaterOrEqual(t, result.supersampledGain, 0.0)
			require.Greater(t, result.residualGap, result.supersampledGain)
		})
	}
}

func TestRealPageTargetFontOnlyExpandedResidualAt72DPIRanksPage109AbovePage95(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	result := measureRealPageSFRMExpandedResidualOrderingAt72DPI(t)

	t.Logf(
		"%s dpi72_residual_gap=%.4f %s dpi72_residual_gap=%.4f",
		result.page95Name,
		result.page95Gap,
		result.page109Name,
		result.page109Gap,
	)

	require.Greater(t, result.page95Gap, 0.0)
	require.Greater(t, result.page109Gap, 0.0)
	require.Equal(t, result.page109Name, result.largerGapName())
	require.Equal(t, "page109", result.largerGapCanonicalKey())
}

func TestRealPageExpandedResidualOrderingAlignsOnCanonicalPage109(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	result := measureRealPageSFRMExpandedResidualOrderingAt72DPI(t)
	alignment := testutil.NewProbeOrderingAlignment(
		result.largerGapCanonicalKey(),
		"page109",
	)

	t.Logf(
		"%s dpi72_residual_gap=%.4f %s dpi72_residual_gap=%.4f shared=%s",
		result.page95Name,
		result.page95Gap,
		result.page109Name,
		result.page109Gap,
		alignment.SharedCanonicalKey(),
	)

	require.Equal(t, "page109", alignment.SharedCanonicalKey())
}

func TestRealPageTargetFontOnlyExpandedResidualShrinkMagnitudeRanksPage109AbovePage95(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	deltas := make(map[string]float64)
	for _, tc := range realPageLowercaseProbeCases() {
		if tc.baseFont != "SFRM1095" {
			continue
		}

		allBaseFonts := pageBaseFontsForProbe(t, tc.target)
		if len(allBaseFonts) <= 1 {
			continue
		}

		targetOnlyEnv := targetFontOnlyEnvForProbe(allBaseFonts, tc.baseFont)
		result := measureExpandedResidualGapAcrossDPIForProbe(t, tc, targetOnlyEnv)
		deltas[tc.target.name] = math.Abs(result.residualGapDelta())

		t.Logf(
			"%s residual_gap_delta_abs=%.4f dpi72_residual_gap=%.4f dpi150_residual_gap=%.4f expanded_codes=%s target_only_skip=%s",
			tc.target.name,
			math.Abs(result.residualGapDelta()),
			result.dpi72ResidualGap,
			result.dpi150ResidualGap,
			tc.expandedCodeSpec(),
			targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
		)
	}

	require.Greater(t, deltas["009_p95_sfrm1095_top6"], 0.0)
	require.Greater(t, deltas["009_p109_sfrm1095_top5"], 0.0)
	require.Greater(t, deltas["009_p109_sfrm1095_top5"], deltas["009_p95_sfrm1095_top6"])
}
