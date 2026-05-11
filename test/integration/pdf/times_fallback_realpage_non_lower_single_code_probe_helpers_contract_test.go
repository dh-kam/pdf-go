package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/test/testutil"
)

func TestRealPageNonLowerSingleCodeOrderingProbeResult_UsesCodeSpecGaps(t *testing.T) {
	result := realPageNonLowerSingleCodeOrderingProbeResult{
		leftCode: 49,
		leftProbe: realPageCodeSpecFastPathProbeResult{
			currentOnly:  10,
			codeSpecOnly: 13,
		},
		rightCode: 58,
		rightProbe: realPageCodeSpecFastPathProbeResult{
			currentOnly:  10,
			codeSpecOnly: 11,
		},
	}

	require.Equal(t, 3.0, result.leftGap())
	require.Equal(t, 1.0, result.rightGap())
	require.Equal(t, "code49", result.largerGapCanonicalKey())
	require.Equal(
		t,
		"code49",
		testutil.NewProbeOrderingAlignment("code49", result.largerGapCanonicalKey()).SharedCanonicalKey(),
	)
}
