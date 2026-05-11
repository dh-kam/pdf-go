package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageResidualClassCodeSpecGapOrderingRanksNonLowerAboveLongTail(t *testing.T) {
	result := measureRealPageResidualClassCodeSpecOrderingAgainstPoppler(t)

	t.Logf(
		"long_tail_gap=%.4f long_tail_fast_path_gain=%.4f non_lower_gap=%.4f non_lower_fast_path_gain=%.4f",
		result.longTailLowercase.codeSpecGap(),
		result.longTailLowercase.fastPathGain(),
		result.nonLowerLowercase.codeSpecGap(),
		result.nonLowerLowercase.fastPathGain(),
	)

	require.Greater(t, result.nonLowerLowercase.codeSpecGap(), result.longTailLowercase.codeSpecGap())
}

func TestRealPageResidualClassExpandedResidualGapOrderingRanksNonLowerAboveLongTail(t *testing.T) {
	result := measureRealPageResidualClassExpandedOrderingAgainstPoppler(t)

	t.Logf(
		"long_tail_expanded_gap=%.4f long_tail_residual_gap=%.4f non_lower_expanded_gap=%.4f non_lower_residual_gap=%.4f",
		result.longTailExpanded.expandedGap(),
		result.longTailExpanded.residualGap(),
		result.nonLowerExpanded.expandedGap(),
		result.nonLowerExpanded.residualGap(),
	)

	require.Greater(t, result.nonLowerExpanded.residualGap(), result.longTailExpanded.residualGap())
}

