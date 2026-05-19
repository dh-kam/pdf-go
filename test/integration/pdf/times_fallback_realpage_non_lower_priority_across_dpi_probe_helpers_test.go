package pdf_test

import "testing"

type realPageNonLowerPriorityAcrossDPIProbeResult struct {
	dpi72  realPageNonLowerPriorityAtDPIProbeResult
	dpi150 realPageNonLowerPriorityAtDPIProbeResult
}

func measureRealPageNonLowerPriorityAcrossDPIProbeAgainstPoppler(
	t *testing.T,
) realPageNonLowerPriorityAcrossDPIProbeResult {
	t.Helper()

	return realPageNonLowerPriorityAcrossDPIProbeResult{
		dpi72:  measureRealPageNonLowerPriorityAtDPIProbeAgainstPoppler(t, 72),
		dpi150: measureRealPageNonLowerPriorityAtDPIProbeAgainstPoppler(t, 150),
	}
}
