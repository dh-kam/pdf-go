package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageResidualClass_StringValuesRemainStable(t *testing.T) {
	require.Equal(t, realPageResidualClass("mixed_lowercase"), realPageResidualClassMixedLowercase)
	require.Equal(t, realPageResidualClass("long_tail"), realPageResidualClassLongTail)
	require.Equal(t, realPageResidualClass("non_lower"), realPageResidualClassNonLower)
}

func TestRealPageSFRMResidualClasses_SeparatePage95LongTailFromPage109NonLower(t *testing.T) {
	cases := realPageLowercaseProbeCases()

	classes := make(map[string]realPageResidualClass, len(cases))
	for _, tc := range cases {
		if tc.baseFont != "SFRM1095" {
			continue
		}
		classes[tc.target.name] = tc.dominantResidualClass()
	}

	require.Equal(t, realPageResidualClassLongTail, classes["009_p95_sfrm1095_top6"])
	require.Equal(t, realPageResidualClassNonLower, classes["009_p109_sfrm1095_top5"])
	require.NotEqual(t, classes["009_p95_sfrm1095_top6"], classes["009_p109_sfrm1095_top5"])
}
