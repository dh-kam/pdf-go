package renderer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyntheticExpandedGlyphSetProbeCases_ContainExpectedTargets(t *testing.T) {
	cases := syntheticExpandedGlyphSetProbeCases()
	require.Len(t, cases, 2)

	require.Equal(t, "009_p95_sfrm1095", cases[0].name)
	require.Equal(t, 95, cases[0].pageNum)
	require.Equal(t, "F16", cases[0].fontResource)
	require.NotEmpty(t, cases[0].codes)

	require.Equal(t, "009_p109_sfrm1095", cases[1].name)
	require.Equal(t, 109, cases[1].pageNum)
	require.Equal(t, "F16", cases[1].fontResource)
	require.NotEmpty(t, cases[1].codes)
}

