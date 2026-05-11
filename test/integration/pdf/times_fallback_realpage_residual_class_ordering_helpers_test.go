package pdf_test

import "testing"

type realPageResidualClassCodeSpecOrderingProbeResult struct {
	longTailLowercase realPageCodeSpecFastPathProbeResult
	nonLowerLowercase realPageCodeSpecFastPathProbeResult
}

func measureRealPageResidualClassCodeSpecOrderingAgainstPoppler(
	t *testing.T,
) realPageResidualClassCodeSpecOrderingProbeResult {
	t.Helper()

	longTail := realPageSFRMLongTailProbeCaseForLowercase(t)
	nonLower := realPageSFRMNonLowerProbeCaseForLowercase(t)

	longTailTargetOnlyEnv := targetFontOnlyEnvForProbe(pageBaseFontsForProbe(t, longTail.target), longTail.baseFont)
	nonLowerTargetOnlyEnv := targetFontOnlyEnvForProbe(pageBaseFontsForProbe(t, nonLower.target), nonLower.baseFont)

	return realPageResidualClassCodeSpecOrderingProbeResult{
		longTailLowercase: measureRealPageCodeSpecFastPathProbeAgainstPoppler(t, longTail, longTailTargetOnlyEnv, longTail.longTailCodeSpec),
		nonLowerLowercase: measureRealPageCodeSpecFastPathProbeAgainstPoppler(t, nonLower, nonLowerTargetOnlyEnv, nonLower.nonLowerCodeSpec),
	}
}

type realPageResidualClassExpandedOrderingProbeResult struct {
	longTailExpanded realPageExpandedLowercaseProbeResult
	nonLowerExpanded realPageExpandedLowercaseProbeResult
}

func measureRealPageResidualClassExpandedOrderingAgainstPoppler(
	t *testing.T,
) realPageResidualClassExpandedOrderingProbeResult {
	t.Helper()

	longTail := realPageSFRMLongTailExpandedLowercaseProbeCases()[0]
	nonLower := realPageSFRMNonLowerExpandedLowercaseProbeCases()[0]

	longTailPopplerPNG := preparePopplerPageForProbe(t, longTail.target)
	nonLowerPopplerPNG := preparePopplerPageForProbe(t, nonLower.target)

	longTailTargetOnlyEnv := targetFontOnlyEnvForProbe(pageBaseFontsForProbe(t, longTail.target), longTail.baseFont)
	nonLowerTargetOnlyEnv := targetFontOnlyEnvForProbe(pageBaseFontsForProbe(t, nonLower.target), nonLower.baseFont)

	return realPageResidualClassExpandedOrderingProbeResult{
		longTailExpanded: measureRealPageExpandedLowercaseProbeAgainstPoppler(t, longTail, longTailPopplerPNG, longTailTargetOnlyEnv),
		nonLowerExpanded: measureRealPageExpandedLowercaseProbeAgainstPoppler(t, nonLower, nonLowerPopplerPNG, nonLowerTargetOnlyEnv),
	}
}

