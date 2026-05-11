package pdf_test

import "testing"

type realPageExpandedLowercaseProbeResult struct {
	currentOnly  float64
	broadOnly    float64
	expandedOnly float64
	fullSkipOnly float64
	fastPathOnly float64
	forcedOnly   float64
}

func (r realPageExpandedLowercaseProbeResult) broadGap() float64 {
	return r.broadOnly - r.currentOnly
}

func (r realPageExpandedLowercaseProbeResult) expandedGap() float64 {
	return r.expandedOnly - r.currentOnly
}

func (r realPageExpandedLowercaseProbeResult) fullGap() float64 {
	return r.fullSkipOnly - r.currentOnly
}

func (r realPageExpandedLowercaseProbeResult) expandedCoverage() float64 {
	return r.expandedGap() / r.fullGap()
}

func (r realPageExpandedLowercaseProbeResult) fastPathGain() float64 {
	return r.fastPathOnly - r.currentOnly
}

func (r realPageExpandedLowercaseProbeResult) forcedGap() float64 {
	return r.forcedOnly - r.currentOnly
}

func (r realPageExpandedLowercaseProbeResult) residualGap() float64 {
	return r.fullSkipOnly - r.expandedOnly
}

func measureRealPageExpandedLowercaseProbeAgainstPoppler(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	popplerPNG string,
	targetOnlyEnv map[string]string,
) realPageExpandedLowercaseProbeResult {
	broadCodeSpec := tc.broadCodeSpec()
	expandedCodeSpec := tc.broadWithNonLowerCodeSpec()

	return realPageExpandedLowercaseProbeResult{
		currentOnly:  renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, targetOnlyEnv),
		broadOnly:    renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, broadCodeSpec)),
		expandedOnly: renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, expandedCodeSpec)),
		fullSkipOnly: renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, fullTargetFontSkipEnvForProbe(targetOnlyEnv, tc.baseFont)),
		fastPathOnly: renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, fastPathWithinTargetFontEnvForProbe(targetOnlyEnv)),
		forcedOnly:   renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, mergeProbeEnv(targetOnlyEnv, tc.forcedEnv)),
	}
}

