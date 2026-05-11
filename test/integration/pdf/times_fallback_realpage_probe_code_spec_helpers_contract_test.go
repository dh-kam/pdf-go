package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCombineCodeSkipSpecsForProbe_MergesCodesForSameBaseFont(t *testing.T) {
	spec := combineCodeSkipSpecsForProbe(
		"SFRM1095=101,110,105,100,117,109",
		"SFRM1095=103,97,115,116,98,114",
	)
	require.Equal(t, "SFRM1095=101,110,105,100,117,109,103,97,115,116,98,114", spec)
}
