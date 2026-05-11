package pdf_test

import (
	"fmt"
	"strings"
	"testing"
)

type realPageGlyphSourceOverrideProbeResult struct {
	expandedOnly      float64
	overrideOnly      float64
	overrideGain      float64
	residualGap       float64
	overrideSpec      string
	targetOnlySkipEnv string
}

func glyphSourceOverrideWithinTargetFontEnvForProbe(targetOnlyEnv map[string]string, baseFont string, code int, sourceFont string) map[string]string {
	return mergeProbeEnv(targetOnlyEnv, map[string]string{
		"PDF_DEBUG_FORCE_GLYPH_SOURCE_MAP": fmt.Sprintf("%s:%d=%s", baseFont, code, sourceFont),
	})
}

func glyphSourceOverridesWithinTargetFontEnvForProbe(targetOnlyEnv map[string]string, specs ...string) map[string]string {
	return mergeProbeEnv(targetOnlyEnv, map[string]string{
		"PDF_DEBUG_FORCE_GLYPH_SOURCE_MAP": strings.Join(specs, ","),
	})
}

func measureGlyphSourceOverrideProbeAgainstPopplerAtDPI(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	targetOnlyEnv map[string]string,
	dpi int,
	code int,
	sourceFont string,
) realPageGlyphSourceOverrideProbeResult {
	t.Helper()

	popplerPNG := preparePopplerPageForProbeAtDPI(t, tc.target, dpi)
	expandedEnv := expandedCodeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, tc)
	expandedOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, expandedEnv, dpi)
	overrideOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(
		t,
		tc.target,
		popplerPNG,
		glyphSourceOverrideWithinTargetFontEnvForProbe(expandedEnv, tc.baseFont, code, sourceFont),
		dpi,
	)
	fullSkipOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(
		t,
		tc.target,
		popplerPNG,
		fullTargetFontSkipEnvForProbe(targetOnlyEnv, tc.baseFont),
		dpi,
	)

	return realPageGlyphSourceOverrideProbeResult{
		expandedOnly:      expandedOnly,
		overrideOnly:      overrideOnly,
		overrideGain:      overrideOnly - expandedOnly,
		residualGap:       fullSkipOnly - expandedOnly,
		overrideSpec:      fmt.Sprintf("%s:%d=%s", tc.baseFont, code, sourceFont),
		targetOnlySkipEnv: targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
	}
}

func measureGlyphSourceOverridesProbeAgainstPopplerAtDPI(
	t *testing.T,
	tc realPageLowercaseProbeCase,
	targetOnlyEnv map[string]string,
	dpi int,
	specs ...string,
) realPageGlyphSourceOverrideProbeResult {
	t.Helper()

	popplerPNG := preparePopplerPageForProbeAtDPI(t, tc.target, dpi)
	expandedEnv := expandedCodeSkipWithinTargetFontEnvForProbe(targetOnlyEnv, tc)
	expandedOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(t, tc.target, popplerPNG, expandedEnv, dpi)
	overrideOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(
		t,
		tc.target,
		popplerPNG,
		glyphSourceOverridesWithinTargetFontEnvForProbe(expandedEnv, specs...),
		dpi,
	)
	fullSkipOnly := renderPageSimilarityAgainstPopplerForProbeAtDPI(
		t,
		tc.target,
		popplerPNG,
		fullTargetFontSkipEnvForProbe(targetOnlyEnv, tc.baseFont),
		dpi,
	)

	return realPageGlyphSourceOverrideProbeResult{
		expandedOnly:      expandedOnly,
		overrideOnly:      overrideOnly,
		overrideGain:      overrideOnly - expandedOnly,
		residualGap:       fullSkipOnly - expandedOnly,
		overrideSpec:      strings.Join(specs, ","),
		targetOnlySkipEnv: targetOnlyEnv["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"],
	}
}
