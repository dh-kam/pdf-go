package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageLowercaseProbeCasesForResidualClass_ReturnExpectedSFRMCases(t *testing.T) {
	mixed := realPageCMRMixedLowercaseProbeCases()
	require.Len(t, mixed, 1)
	require.Equal(t, "004_p3_cmr10_top6", mixed[0].target.name)
	require.Equal(t, realPageResidualClassMixedLowercase, mixed[0].dominantResidualClass())

	nonLower := realPageSFRMNonLowerLowercaseProbeCases()
	require.Len(t, nonLower, 1)
	require.Equal(t, "009_p109_sfrm1095_top5", nonLower[0].target.name)
	require.Equal(t, realPageResidualClassNonLower, nonLower[0].dominantResidualClass())

	longTail := realPageSFRMLongTailLowercaseProbeCases()
	require.Len(t, longTail, 1)
	require.Equal(t, "009_p95_sfrm1095_top6", longTail[0].target.name)
	require.Equal(t, realPageResidualClassLongTail, longTail[0].dominantResidualClass())
}

func TestRealPageLowercaseProbeCasesForResidualClass_PartitionsKnownCases(t *testing.T) {
	allCases := realPageLowercaseProbeCases()
	require.Len(t, allCases, 3)

	partitioned := [][]realPageLowercaseProbeCase{
		realPageCMRMixedLowercaseProbeCases(),
		realPageSFRMNonLowerLowercaseProbeCases(),
		realPageSFRMLongTailLowercaseProbeCases(),
	}

	seen := make(map[string]realPageResidualClass, len(allCases))
	for _, cases := range partitioned {
		for _, tc := range cases {
			_, exists := seen[tc.target.name]
			require.False(t, exists, "duplicate lowercase residual class case: %s", tc.target.name)
			seen[tc.target.name] = tc.dominantResidualClass()
		}
	}

	require.Len(t, seen, len(allCases))
	for _, tc := range allCases {
		class, ok := seen[tc.target.name]
		require.True(t, ok, "missing lowercase residual class case: %s", tc.target.name)
		require.Equal(t, tc.dominantResidualClass(), class)
	}
}

func TestRealPageBroadAndExpandedLowercaseProbeCaseSelectors_ReturnExpectedResidualClasses(t *testing.T) {
	broad := realPageBroadLowercaseProbeCases()
	require.Len(t, broad, 3)

	expanded := realPageExpandedLowercaseProbeCases()
	require.Len(t, expanded, 3)

	mixedBroad := realPageCMRMixedBroadLowercaseProbeCases()
	require.Len(t, mixedBroad, 1)
	require.Equal(t, "004_p3_cmr10_top6", mixedBroad[0].target.name)
	require.Equal(t, realPageResidualClassMixedLowercase, mixedBroad[0].dominantResidualClass())

	longTailBroad := realPageSFRMLongTailBroadLowercaseProbeCases()
	require.Len(t, longTailBroad, 1)
	require.Equal(t, "009_p95_sfrm1095_top6", longTailBroad[0].target.name)
	require.Equal(t, realPageResidualClassLongTail, longTailBroad[0].dominantResidualClass())

	nonLowerBroad := realPageSFRMNonLowerBroadLowercaseProbeCases()
	require.Len(t, nonLowerBroad, 1)
	require.Equal(t, "009_p109_sfrm1095_top5", nonLowerBroad[0].target.name)
	require.Equal(t, realPageResidualClassNonLower, nonLowerBroad[0].dominantResidualClass())

	mixedExpanded := realPageCMRMixedExpandedLowercaseProbeCases()
	require.Len(t, mixedExpanded, 1)
	require.Equal(t, "004_p3_cmr10_top6", mixedExpanded[0].target.name)
	require.Equal(t, realPageResidualClassMixedLowercase, mixedExpanded[0].dominantResidualClass())

	longTailExpanded := realPageSFRMLongTailExpandedLowercaseProbeCases()
	require.Len(t, longTailExpanded, 1)
	require.Equal(t, "009_p95_sfrm1095_top6", longTailExpanded[0].target.name)
	require.Equal(t, realPageResidualClassLongTail, longTailExpanded[0].dominantResidualClass())

	nonLowerExpanded := realPageSFRMNonLowerExpandedLowercaseProbeCases()
	require.Len(t, nonLowerExpanded, 1)
	require.Equal(t, "009_p109_sfrm1095_top5", nonLowerExpanded[0].target.name)
	require.Equal(t, realPageResidualClassNonLower, nonLowerExpanded[0].dominantResidualClass())
}

func TestRealPageBroadLowercaseProbeCases_PartitionKnownCases(t *testing.T) {
	allCases := realPageBroadLowercaseProbeCases()
	require.Len(t, allCases, 3)

	partitioned := [][]realPageLowercaseProbeCase{
		realPageCMRMixedBroadLowercaseProbeCases(),
		realPageSFRMLongTailBroadLowercaseProbeCases(),
		realPageSFRMNonLowerBroadLowercaseProbeCases(),
	}

	seen := make(map[string]realPageResidualClass, len(allCases))
	for _, cases := range partitioned {
		for _, tc := range cases {
			_, exists := seen[tc.target.name]
			require.False(t, exists, "duplicate broad lowercase residual class case: %s", tc.target.name)
			seen[tc.target.name] = tc.dominantResidualClass()
		}
	}

	require.Len(t, seen, len(allCases))
	for _, tc := range allCases {
		class, ok := seen[tc.target.name]
		require.True(t, ok, "missing broad lowercase residual class case: %s", tc.target.name)
		require.Equal(t, tc.dominantResidualClass(), class)
	}
}

func TestRealPageExpandedLowercaseProbeCases_PartitionKnownCases(t *testing.T) {
	allCases := realPageExpandedLowercaseProbeCases()
	require.Len(t, allCases, 3)

	partitioned := [][]realPageLowercaseProbeCase{
		realPageCMRMixedExpandedLowercaseProbeCases(),
		realPageSFRMLongTailExpandedLowercaseProbeCases(),
		realPageSFRMNonLowerExpandedLowercaseProbeCases(),
	}

	seen := make(map[string]realPageResidualClass, len(allCases))
	for _, cases := range partitioned {
		for _, tc := range cases {
			_, exists := seen[tc.target.name]
			require.False(t, exists, "duplicate expanded lowercase residual class case: %s", tc.target.name)
			seen[tc.target.name] = tc.dominantResidualClass()
		}
	}

	require.Len(t, seen, len(allCases))
	for _, tc := range allCases {
		class, ok := seen[tc.target.name]
		require.True(t, ok, "missing expanded lowercase residual class case: %s", tc.target.name)
		require.Equal(t, tc.dominantResidualClass(), class)
	}
}
