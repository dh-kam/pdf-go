package pdf_test

import "testing"

type realPageBroadLowercaseProbeResult struct {
	currentOnly  float64
	combinedOnly float64
	broadOnly    float64
	fullSkipOnly float64
}

func (r realPageBroadLowercaseProbeResult) combinedGap() float64 {
	return r.combinedOnly - r.currentOnly
}

func (r realPageBroadLowercaseProbeResult) broadGap() float64 {
	return r.broadOnly - r.currentOnly
}

func (r realPageBroadLowercaseProbeResult) fullGap() float64 {
	return r.fullSkipOnly - r.currentOnly
}

func (r realPageBroadLowercaseProbeResult) broadCoverage() float64 {
	return r.broadGap() / r.fullGap()
}

func measureRealPageBroadLowercaseProbeAgainstPoppler(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	popplerPNG string,
	targetOnlyEnv map[string]string,
) realPageBroadLowercaseProbeResult {
	combinedCodeSpec := tc.combinedCodeSpec()
	broadCodeSpec := tc.broadCodeSpec()

	return realPageBroadLowercaseProbeResult{
		currentOnly:  renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, targetOnlyEnv),
		combinedOnly: renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, combinedCodeSpec)),
		broadOnly:    renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, broadCodeSpec)),
		fullSkipOnly: renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, fullTargetFontSkipEnvForProbe(targetOnlyEnv, tc.baseFont)),
	}
}

