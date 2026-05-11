package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/test/testutil"
)

func TestRealPageExpandedResidualOrderingProbeResult_LargerGapName(t *testing.T) {
	result := realPageExpandedResidualOrderingProbeResult{
		page95Name:  "009_p95_sfrm1095_top6",
		page95Gap:   0.1,
		page109Name: "009_p109_sfrm1095_top5",
		page109Gap:  0.2,
	}

	require.Equal(t, "009_p109_sfrm1095_top5", result.largerGapName())
	require.Equal(t, "page109", result.largerGapCanonicalKey())
	require.Equal(
		t,
		"page109",
		testutil.NewProbeOrderingAlignment(result.largerGapCanonicalKey(), "page109").SharedCanonicalKey(),
	)

	result.page109Gap = 0.05
	require.Equal(t, "009_p95_sfrm1095_top6", result.largerGapName())
	require.Equal(t, "page95", result.largerGapCanonicalKey())
	require.Equal(
		t,
		"page95",
		testutil.NewProbeOrderingAlignment(result.largerGapCanonicalKey(), "page95").SharedCanonicalKey(),
	)
}

func TestCanonicalOrderingKeys_UseSharedPageLabelsAcrossProbeLayers(t *testing.T) {
	require.Equal(t, "page95", testutil.CanonicalPageKeyForProbeName("009_p95_sfrm1095"))
	require.Equal(t, "page109", testutil.CanonicalPageKeyForProbeName("009_p109_sfrm1095_top5"))
	require.Equal(t, "", testutil.CanonicalPageKeyForProbeName("unexpected"))
}

func TestMeasureExpandedResidualProbeAgainstPopplerResult_ComputesDeltaFields(t *testing.T) {
	result := realPageExpandedResidualProbeResult{
		expandedOnly:         98.28,
		fullSkipOnly:         98.31,
		forcedResidualOnly:   98.2805,
		fastPathResidualOnly: 98.2800,
		forcedFastPathOnly:   98.2807,
		embeddedResidualOnly: 98.2809,
		fallbackResidualOnly: 98.2801,
		supersampledOnly:     98.2812,
		residualGap:          0.03,
		forcedResidualGain:   0.0005,
		fastPathResidualGain: 0.0,
		forcedFastPathGain:   0.0007,
		embeddedResidualGain: 0.0009,
		fallbackResidualGain: 0.0001,
		supersampledGain:     0.0012,
	}

	require.Equal(t, 98.28, result.expandedOnly)
	require.Equal(t, 98.31, result.fullSkipOnly)
	require.Equal(t, 0.03, result.residualGap)
	require.Equal(t, 0.0005, result.forcedResidualGain)
	require.Equal(t, 0.0, result.fastPathResidualGain)
	require.Equal(t, 0.0007, result.forcedFastPathGain)
	require.Equal(t, 0.0009, result.embeddedResidualGain)
	require.Equal(t, 0.0001, result.fallbackResidualGain)
	require.Equal(t, 0.0012, result.supersampledGain)
}

func TestRealPageExpandedResidualProbeResult_MaxPolicyGain(t *testing.T) {
	result := realPageExpandedResidualProbeResult{
		forcedResidualGain:   0.0005,
		fastPathResidualGain: 0.0,
		forcedFastPathGain:   0.0007,
		embeddedResidualGain: 0.0009,
		fallbackResidualGain: 0.0001,
		supersampledGain:     0.0012,
	}

	require.Equal(t, 0.0012, result.maxPolicyGain())
}

func TestRealPageExpandedResidualDPIProbeResult_ResidualGapDelta(t *testing.T) {
	result := realPageExpandedResidualDPIProbeResult{
		dpi72ResidualGap:  0.0123,
		dpi150ResidualGap: 0.0311,
	}

	require.InDelta(t, 0.0188, result.residualGapDelta(), 1e-9)
}
