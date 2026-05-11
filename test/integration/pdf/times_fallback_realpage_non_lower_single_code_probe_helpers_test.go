package pdf_test

import (
	"fmt"
	"testing"
)

type realPageNonLowerSingleCodeOrderingProbeResult struct {
	leftName  string
	leftCode  int
	leftProbe realPageCodeSpecFastPathProbeResult

	rightName  string
	rightCode  int
	rightProbe realPageCodeSpecFastPathProbeResult
}

func (r realPageNonLowerSingleCodeOrderingProbeResult) leftGap() float64 {
	return r.leftProbe.codeSpecGap()
}

func (r realPageNonLowerSingleCodeOrderingProbeResult) rightGap() float64 {
	return r.rightProbe.codeSpecGap()
}

func (r realPageNonLowerSingleCodeOrderingProbeResult) largerGapCanonicalKey() string {
	if r.leftGap() > r.rightGap() {
		return codeCanonicalKeyForRealPageProbe(r.leftCode)
	}
	return codeCanonicalKeyForRealPageProbe(r.rightCode)
}

func codeCanonicalKeyForRealPageProbe(code int) string {
	return fmt.Sprintf("code%d", code)
}

func measureRealPageNonLowerSingleCodeOrderingProbeAt72DPI(
	t *testing.T,
	leftCode int,
	rightCode int,
) realPageNonLowerSingleCodeOrderingProbeResult {
	t.Helper()

	tc := realPageSFRMNonLowerProbeCaseForLowercase(t)
	targetOnlyEnv := targetFontOnlyEnvForProbe(pageBaseFontsForProbe(t, tc.target), tc.baseFont)

	leftSpec := singleCodeSkipSpecForProbe(tc.baseFont, leftCode)
	rightSpec := singleCodeSkipSpecForProbe(tc.baseFont, rightCode)

	return realPageNonLowerSingleCodeOrderingProbeResult{
		leftName:   tc.target.name,
		leftCode:   leftCode,
		leftProbe:  measureRealPageCodeSpecFastPathProbeAgainstPopplerAtDPI(t, tc, targetOnlyEnv, leftSpec, 72),
		rightName:  tc.target.name,
		rightCode:  rightCode,
		rightProbe: measureRealPageCodeSpecFastPathProbeAgainstPopplerAtDPI(t, tc, targetOnlyEnv, rightSpec, 72),
	}
}
