package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/test/testutil"
)

func TestRealPageTargetFontOnlyCode49GapExceedsCode47GapAt72DPI(t *testing.T) {
	result := measureRealPageNonLowerSingleCodeOrderingProbeAt72DPI(t, 49, 47)

	t.Logf(
		"code49_gap=%.4f code49_only=%.4f code47_gap=%.4f code47_only=%.4f current_only=%.4f left_name=%s",
		result.leftGap(),
		result.leftProbe.codeSpecOnly,
		result.rightGap(),
		result.rightProbe.codeSpecOnly,
		result.leftProbe.currentOnly,
		result.leftName,
	)

	require.Greater(t, result.leftGap(), 0.0)
	require.Greater(t, result.rightGap(), 0.0)
	require.Greater(t, result.leftGap(), result.rightGap())
}

func TestRealPageTargetFontOnlyCode49GapExceedsCode58GapAt72DPI(t *testing.T) {
	result := measureRealPageNonLowerSingleCodeOrderingProbeAt72DPI(t, 49, 58)

	t.Logf(
		"code49_gap=%.4f code49_only=%.4f code58_gap=%.4f code58_only=%.4f current_only=%.4f left_name=%s",
		result.leftGap(),
		result.leftProbe.codeSpecOnly,
		result.rightGap(),
		result.rightProbe.codeSpecOnly,
		result.leftProbe.currentOnly,
		result.leftName,
	)

	require.Greater(t, result.leftGap(), 0.0)
	require.Greater(t, result.rightGap(), 0.0)
	require.Greater(t, result.leftGap(), result.rightGap())
}

func TestRealPageTargetFontOnlyCode49And58GapOrderingAlignsOnCanonicalCode49At72DPI(t *testing.T) {
	result := measureRealPageNonLowerSingleCodeOrderingProbeAt72DPI(t, 49, 58)
	alignment := testutil.NewProbeOrderingAlignment("code49", result.largerGapCanonicalKey())

	t.Logf(
		"page109_non_lower code49_gap=%.4f code58_gap=%.4f shared=%s",
		result.leftGap(),
		result.rightGap(),
		alignment.SharedCanonicalKey(),
	)

	require.Equal(t, "code49", alignment.SharedCanonicalKey())
}
