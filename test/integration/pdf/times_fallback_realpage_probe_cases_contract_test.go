package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealPageFontProbeCases_ContainExpectedTargetsAndForcedModes(t *testing.T) {
	cases := realPageFontProbeCases()
	require.Len(t, cases, 3)

	byName := make(map[string]realPageFontProbeCase, len(cases))
	for _, tc := range cases {
		byName[tc.target.name] = tc
	}

	cmr, ok := byName["004_p3_cmr10"]
	require.True(t, ok)
	require.Equal(t, 3, cmr.target.pageNumber)
	require.Equal(t, "CMR10", cmr.skipBaseFonts)
	require.Equal(t, "CMR10=Courier", cmr.forcedEnv["PDF_DEBUG_FORCE_BASE_FONT_MAP"])

	sfrm95, ok := byName["009_p95_sfrm1095"]
	require.True(t, ok)
	require.Equal(t, 95, sfrm95.target.pageNumber)
	require.Equal(t, "SFRM1095", sfrm95.skipBaseFonts)
	require.Equal(t, "SFRM1095=Times-Italic", sfrm95.forcedEnv["PDF_DEBUG_FORCE_BASE_FONT_MAP"])

	sfrm109, ok := byName["009_p109_sfrm1095"]
	require.True(t, ok)
	require.Equal(t, 109, sfrm109.target.pageNumber)
	require.Equal(t, "SFRM1095", sfrm109.skipBaseFonts)
	require.Equal(t, "SFRM1095=Times-Italic", sfrm109.forcedEnv["PDF_DEBUG_FORCE_BASE_FONT_MAP"])
}

func TestRealPageLowercaseProbeCases_ContainExpectedCodeSpecsAndCoverageBounds(t *testing.T) {
	cases := realPageLowercaseProbeCases()
	require.Len(t, cases, 3)

	byName := make(map[string]realPageLowercaseProbeCase, len(cases))
	for _, tc := range cases {
		byName[tc.target.name] = tc
		require.Less(t, tc.minCoverage, tc.maxCoverage)
	}

	cmr, ok := byName["004_p3_cmr10_top6"]
	require.True(t, ok)
	require.Equal(t, "CMR10", cmr.baseFont)
	require.Equal(t, "CMR10=101", cmr.singleCodeSpec)
	require.Equal(t, "CMR10=101,116,111,110,105,97", cmr.topSetCodeSpec)
	require.Equal(t, "CMR10=108,104,115,114,100,117", cmr.secondaryCodeSpec)
	require.Equal(t, "", cmr.tertiaryCodeSpec)
	require.Equal(t, "CMR10=118,99,107,112", cmr.longTailCodeSpec)
	require.Equal(t, "CMR10=44,46,75,41,40,45,69,83,49,50", cmr.nonLowerCodeSpec)
	require.Equal(t, "CMR10=Courier", cmr.forcedEnv["PDF_DEBUG_FORCE_BASE_FONT_MAP"])
	require.Equal(t, realPageResidualClassMixedLowercase, cmr.dominantResidualClass())

	sfrm95, ok := byName["009_p95_sfrm1095_top6"]
	require.True(t, ok)
	require.Equal(t, "SFRM1095", sfrm95.baseFont)
	require.Equal(t, "SFRM1095=101", sfrm95.singleCodeSpec)
	require.Equal(t, "SFRM1095=101,110,105,100,117,109", sfrm95.topSetCodeSpec)
	require.Equal(t, "SFRM1095=103,97,115,116,98,114", sfrm95.secondaryCodeSpec)
	require.Equal(t, "", sfrm95.tertiaryCodeSpec)
	require.Equal(t, "SFRM1095=108,111,104,118,99,107,112", sfrm95.longTailCodeSpec)
	require.Equal(t, "SFRM1095=44,46,75,41,40,45,69,83,228,49,50", sfrm95.nonLowerCodeSpec)
	require.Equal(t, "SFRM1095=Times-Italic", sfrm95.forcedEnv["PDF_DEBUG_FORCE_BASE_FONT_MAP"])
	require.Equal(t, realPageResidualClassLongTail, sfrm95.dominantResidualClass())

	sfrm109, ok := byName["009_p109_sfrm1095_top5"]
	require.True(t, ok)
	require.Equal(t, "SFRM1095", sfrm109.baseFont)
	require.Equal(t, "SFRM1095=101", sfrm109.singleCodeSpec)
	require.Equal(t, "SFRM1095=101,98,110,97,109", sfrm109.topSetCodeSpec)
	require.Equal(t, "SFRM1095=111,116,105,114,115,108", sfrm109.secondaryCodeSpec)
	require.Equal(t, "SFRM1095=46", sfrm109.tertiaryCodeSpec)
	require.Equal(t, "SFRM1095=99,117,100,104,107,103,120,112,102", sfrm109.longTailCodeSpec)
	require.Equal(t, "SFRM1095=65,49,58,44,47,48,50,51,84", sfrm109.nonLowerCodeSpec)
	require.Equal(t, "SFRM1095=Times-Italic", sfrm109.forcedEnv["PDF_DEBUG_FORCE_BASE_FONT_MAP"])
	require.Equal(t, realPageResidualClassNonLower, sfrm109.dominantResidualClass())
}

func TestRealPageLowercaseProbeCase_CodeSpecHelpers(t *testing.T) {
	tc := realPageLowercaseProbeCase{
		topSetCodeSpec:    "SFRM1095=101,110",
		secondaryCodeSpec: "SFRM1095=97,115",
		tertiaryCodeSpec:  "SFRM1095=46",
		longTailCodeSpec:  "SFRM1095=99,107",
		nonLowerCodeSpec:  "SFRM1095=65,49",
	}

	require.Equal(t, "SFRM1095=101,110,97,115", tc.combinedCodeSpec())
	require.Equal(t, "SFRM1095=101,110,97,115,99,107", tc.broadCodeSpec())
	require.Equal(t, "SFRM1095=101,110,97,115,99,107,65,49", tc.broadWithNonLowerCodeSpec())
	require.Equal(t, "SFRM1095=101,110,97,115,99,107,65,49", tc.expandedCodeSpec())
	require.True(t, tc.hasTertiaryCodeSpec())
	require.True(t, tc.hasLongTailCodeSpec())
	require.True(t, tc.hasNonLowerCodeSpec())
}
