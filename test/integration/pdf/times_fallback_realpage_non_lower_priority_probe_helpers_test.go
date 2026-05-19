package pdf_test

import "testing"

type realPageNonLowerPriorityProbeResult struct {
	codeSpec          realPageCodeSpecFastPathProbeResult
	expanded          realPageExpandedLowercaseProbeResult
	coreGlyphSwap     realPageGlyphSourceOverrideProbeResult
	expandedGlyphSwap realPageGlyphSourceOverrideProbeResult
}

func measureRealPageNonLowerPriorityProbeAgainstPoppler(
	t *testing.T,
) realPageNonLowerPriorityProbeResult {
	t.Helper()

	return measureRealPageNonLowerPriorityProbeAgainstPopplerAtDPI(t, 72)
}

type realPageNonLowerPriorityAtDPIProbeResult struct {
	expandedResidualGap   float64
	codeSpecGap           float64
	coreGlyphSwapGain     float64
	expandedGlyphSwapGain float64
	dpi                   int
}

func measureRealPageNonLowerPriorityProbeAgainstPopplerAtDPI(
	t *testing.T,
	dpi int,
) realPageNonLowerPriorityProbeResult {
	t.Helper()

	tc := realPageSFRMNonLowerProbeCaseForLowercase(t)
	targetOnlyEnv := targetFontOnlyEnvForProbe(pageBaseFontsForProbe(t, tc.target), tc.baseFont)
	popplerPNG := preparePopplerPageForProbeAtDPI(t, tc.target, dpi)

	expandedCodeSpec := tc.expandedCodeSpec()
	expandedEnv := codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, expandedCodeSpec)

	currentOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, targetOnlyEnv, dpi)
	broadOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, codeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, tc.broadCodeSpec()), dpi)
	expandedOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, expandedEnv, dpi)
	fullSkipOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, fullTargetFontSkipEnvForProbe(targetOnlyEnv, tc.baseFont), dpi)
	fastPathOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, fastPathWithinTargetFontEnvForProbe(targetOnlyEnv), dpi)
	forcedOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, mergeProbeEnv(targetOnlyEnv, tc.forcedEnv), dpi)

	return realPageNonLowerPriorityProbeResult{
		codeSpec: measureRealPageCodeSpecFastPathProbeAgainstPopplerAtDPI(t, tc, targetOnlyEnv, tc.nonLowerCodeSpec, dpi),
		expanded: realPageExpandedLowercaseProbeResult{
			currentOnly:  currentOnly,
			broadOnly:    broadOnly,
			expandedOnly: expandedOnly,
			fullSkipOnly: fullSkipOnly,
			fastPathOnly: fastPathOnly,
			forcedOnly:   forcedOnly,
		},
		coreGlyphSwap: measureGlyphSourceOverridesProbeAgainstPopplerAtDPI(
			t,
			tc,
			targetOnlyEnv,
			dpi,
			nonLowerCoreGlyphSourceOverrideSpecsForProbe()...,
		),
		expandedGlyphSwap: measureGlyphSourceOverridesProbeAgainstPopplerAtDPI(
			t,
			tc,
			targetOnlyEnv,
			dpi,
			nonLowerExpandedGlyphSourceOverrideSpecsForProbe()...,
		),
	}
}

func measureRealPageNonLowerPriorityAtDPIProbeAgainstPoppler(
	t *testing.T,
	dpi int,
) realPageNonLowerPriorityAtDPIProbeResult {
	t.Helper()

	tc := realPageSFRMNonLowerProbeCaseForLowercase(t)
	targetOnlyEnv := targetFontOnlyEnvForProbe(pageBaseFontsForProbe(t, tc.target), tc.baseFont)
	coreGlyphSwap := measureGlyphSourceOverridesProbeAgainstPopplerAtDPI(
		t,
		tc,
		targetOnlyEnv,
		dpi,
		nonLowerCoreGlyphSourceOverrideSpecsForProbe()...,
	)
	expandedGlyphSwap := measureGlyphSourceOverridesProbeAgainstPopplerAtDPI(
		t,
		tc,
		targetOnlyEnv,
		dpi,
		nonLowerExpandedGlyphSourceOverrideSpecsForProbe()...,
	)
	priority := measureRealPageNonLowerPriorityProbeAgainstPopplerAtDPI(t, dpi)

	return realPageNonLowerPriorityAtDPIProbeResult{
		expandedResidualGap:   priority.expanded.residualGap(),
		codeSpecGap:           priority.codeSpec.codeSpecGap(),
		coreGlyphSwapGain:     coreGlyphSwap.overrideGain,
		expandedGlyphSwapGain: expandedGlyphSwap.overrideGain,
		dpi:                   dpi,
	}
}
