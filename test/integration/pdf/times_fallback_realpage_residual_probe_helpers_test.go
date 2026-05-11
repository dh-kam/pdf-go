package pdf_test

import (
	"math"
	"testing"

	"github.com/dh-kam/pdf-go/test/testutil"
)

type realPageExpandedResidualProbeResult struct {
	expandedOnly          float64
	fullSkipOnly          float64
	forcedResidualOnly    float64
	fastPathResidualOnly  float64
	forcedFastPathOnly    float64
	embeddedResidualOnly  float64
	fallbackResidualOnly  float64
	supersampledOnly      float64
	glyphSourceOnly       float64
	residualGap           float64
	forcedResidualGain    float64
	fastPathResidualGain  float64
	forcedFastPathGain    float64
	embeddedResidualGain  float64
	fallbackResidualGain  float64
	supersampledGain      float64
	glyphSourceGain       float64
	targetOnlySkipBaseEnv string
}

type realPageExpandedResidualDPIProbeResult struct {
	dpi72ExpandedOnly  float64
	dpi72FullSkipOnly  float64
	dpi72ResidualGap   float64
	dpi150ExpandedOnly float64
	dpi150FullSkipOnly float64
	dpi150ResidualGap  float64
}

type realPageExpandedResidualOrderingProbeResult struct {
	page95Name  string
	page95Gap   float64
	page109Name string
	page109Gap  float64
}

func (r realPageExpandedResidualProbeResult) maxPolicyGain() float64 {
	return math.Max(
		math.Max(r.forcedResidualGain, r.fastPathResidualGain),
		math.Max(
			math.Max(r.forcedFastPathGain, r.embeddedResidualGain),
			math.Max(r.fallbackResidualGain, r.supersampledGain),
		),
	)
}

func (r realPageExpandedResidualDPIProbeResult) residualGapDelta() float64 {
	return r.dpi150ResidualGap - r.dpi72ResidualGap
}

func (r realPageExpandedResidualOrderingProbeResult) largerGapName() string {
	return testutil.LargerProbeName(r.page95Name, r.page95Gap, r.page109Name, r.page109Gap)
}

func (r realPageExpandedResidualOrderingProbeResult) largerGapCanonicalKey() string {
	return testutil.LargerProbeCanonicalPageKey(r.page95Name, r.page95Gap, r.page109Name, r.page109Gap)
}

func measureExpandedResidualProbeAgainstPoppler(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	targetOnlyEnv map[string]string,
	popplerPNG string,
) realPageExpandedResidualProbeResult {
	return measureExpandedResidualProbeAgainstPreparedPopplerAtDPI(
		t,
		tc,
		targetOnlyEnv,
		popplerPNG,
		defaultRealPageProbeDPI,
	)
}

func measureExpandedResidualProbeAgainstPreparedPopplerAtDPI(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	targetOnlyEnv map[string]string,
	popplerPNG string,
	dpi int,
) realPageExpandedResidualProbeResult {
	t.Helper()

	expandedEnv := expandedCodeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, tc)
	expandedOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, expandedEnv, dpi)
	fullSkipOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, fullTargetFontSkipEnvForProbe(targetOnlyEnv, tc.baseFont), dpi)
	forcedResidualOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, mergeProbeEnv(expandedEnv, tc.forcedEnv), dpi)
	fastPathResidualOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, fastPathWithinTargetFontEnvForProbe(expandedEnv), dpi)
	forcedFastPathOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, forcedFastPathWithinTargetFontEnvForProbe(expandedEnv, tc.forcedEnv), dpi)
	embeddedResidualOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, embeddedWithinTargetFontEnvForProbe(expandedEnv, tc.baseFont), dpi)
	fallbackResidualOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, fallbackWithinTargetFontEnvForProbe(expandedEnv, tc.baseFont), dpi)
	supersampledOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, glyphSupersampleWithinTargetFontEnvForProbe(expandedEnv, 2), dpi)
	glyphSourceOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, glyphSourceOverrideWithinTargetFontEnvForProbe(expandedEnv, tc.baseFont, 47, "Helvetica"), dpi)

	return realPageExpandedResidualProbeResult{
		expandedOnly:          expandedOnly,
		fullSkipOnly:          fullSkipOnly,
		forcedResidualOnly:    forcedResidualOnly,
		fastPathResidualOnly:  fastPathResidualOnly,
		forcedFastPathOnly:    forcedFastPathOnly,
		embeddedResidualOnly:  embeddedResidualOnly,
		fallbackResidualOnly:  fallbackResidualOnly,
		supersampledOnly:      supersampledOnly,
		glyphSourceOnly:       glyphSourceOnly,
		residualGap:           fullSkipOnly - expandedOnly,
		forcedResidualGain:    forcedResidualOnly - expandedOnly,
		fastPathResidualGain:  fastPathResidualOnly - expandedOnly,
		forcedFastPathGain:    forcedFastPathOnly - expandedOnly,
		embeddedResidualGain:  embeddedResidualOnly - expandedOnly,
		fallbackResidualGain:  fallbackResidualOnly - expandedOnly,
		supersampledGain:      supersampledOnly - expandedOnly,
		glyphSourceGain:       glyphSourceOnly - expandedOnly,
		targetOnlySkipBaseEnv: targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
	}
}

func measureExpandedResidualProbeAgainstPopplerAtDPI(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	targetOnlyEnv map[string]string,
	dpi int,
) realPageExpandedResidualProbeResult {
	t.Helper()

	popplerPNG := preparePopplerPageForProbeAtDPI(t, tc.target, dpi)
	return measureExpandedResidualProbeAgainstPreparedPopplerAtDPI(t, tc, targetOnlyEnv, popplerPNG, dpi)
}

func measureExpandedResidualGapAcrossDPIForProbe(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	targetOnlyEnv map[string]string,
) realPageExpandedResidualDPIProbeResult {
	t.Helper()

	result72 := measureExpandedResidualProbeAgainstPopplerAtDPI(t, tc, targetOnlyEnv, 72)
	result150 := measureExpandedResidualProbeAgainstPopplerAtDPI(t, tc, targetOnlyEnv, 150)
	return realPageExpandedResidualDPIProbeResult{
		dpi72ExpandedOnly:  result72.expandedOnly,
		dpi72FullSkipOnly:  result72.fullSkipOnly,
		dpi72ResidualGap:   result72.residualGap,
		dpi150ExpandedOnly: result150.expandedOnly,
		dpi150FullSkipOnly: result150.fullSkipOnly,
		dpi150ResidualGap:  result150.residualGap,
	}
}

func measureRealPageSFRMExpandedResidualOrderingAt72DPI(
	t *testing.T,
) realPageExpandedResidualOrderingProbeResult {
	t.Helper()

	result := realPageExpandedResidualOrderingProbeResult{
		page95Name:  "009_p95_sfrm1095_top6",
		page109Name: "009_p109_sfrm1095_top5",
	}

	for _, tc := range realPageLowercaseProbeCases() {
		if tc.baseFont != "SFRM1095" {
			continue
		}

		allBaseFonts := pageBaseFontsForProbe(t, tc.target)
		if len(allBaseFonts) <= 1 {
			continue
		}

		targetOnlyEnv := targetFontOnlyEnvForProbe(allBaseFonts, tc.baseFont)
		measured := measureExpandedResidualProbeAgainstPopplerAtDPI(t, tc, targetOnlyEnv, 72)

		switch tc.target.name {
		case result.page95Name:
			result.page95Gap = measured.residualGap
		case result.page109Name:
			result.page109Gap = measured.residualGap
		}
	}

	return result
}
