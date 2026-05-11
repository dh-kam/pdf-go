package pdf_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPageBaseFontsForProbe_ReturnsNormalizedUniqueBaseFonts(t *testing.T) {
	cmr := pageBaseFontsForProbe(t, realPageProbeTarget{
		name:       "004_p3_cmr10",
		pdfPath:    getSampleDir() + "/004-pdflatex-4-pages/pdflatex-4-pages.pdf",
		pageNumber: 3,
	})
	require.Equal(t, []string{"CMR10"}, cmr)

	sfrm95 := pageBaseFontsForProbe(t, realPageProbeTarget{
		name:       "009_p95_sfrm1095",
		pdfPath:    getSampleDir() + "/009-pdflatex-geotopo/GeoTopo.pdf",
		pageNumber: 95,
	})
	require.Contains(t, sfrm95, "SFRM1095")
	require.Contains(t, sfrm95, "CMR10")
	require.Contains(t, sfrm95, "SFTT0900")
	require.NotContains(t, sfrm95, "FJKNGJ+SFRM1095")
}

func TestSkipBaseFontsExceptForProbe_SkipsAllButRequestedBaseFont(t *testing.T) {
	skip := skipBaseFontsExceptForProbe(
		[]string{"CMR10", "SFRM1095", "SFTT0900", "CMSY10"},
		"SFRM1095",
	)
	require.Equal(t, "CMR10,CMSY10,SFTT0900", skip)
}

func TestTargetFontOnlyEnvForProbe_BuildsSkipEnvForAllOtherFonts(t *testing.T) {
	env := targetFontOnlyEnvForProbe(
		[]string{"CMR10", "SFRM1095", "SFTT0900", "CMSY10"},
		"SFRM1095",
	)
	require.Equal(t, "CMR10,CMSY10,SFTT0900", env["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"])
}

func TestFullTargetFontSkipEnvForProbe_AddsTargetBaseFontToSkipList(t *testing.T) {
	env := fullTargetFontSkipEnvForProbe(
		map[string]string{"PDF_DEBUG_SKIP_TEXT_BASE_FONTS": "CMR10,CMSY10,SFTT0900"},
		"SFRM1095",
	)
	require.Equal(t, "SFRM1095,CMR10,CMSY10,SFTT0900", env["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"])
}

func TestCodeSkipWithinTargetFontEnvForProbe_AddsCodeSkipSpec(t *testing.T) {
	env := codeSkipWithinTargetFontEnvForProbe(
		map[string]string{"PDF_DEBUG_SKIP_TEXT_BASE_FONTS": "CMR10,CMSY10,SFTT0900"},
		"SFRM1095=101,110",
	)
	require.Equal(t, "CMR10,CMSY10,SFTT0900", env["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"])
	require.Equal(t, "SFRM1095=101,110", env["PDF_DEBUG_SKIP_TEXT_CODES_FOR_BASE"])
}

func TestFastPathWithinTargetFontEnvForProbe_AddsFastPathMode(t *testing.T) {
	env := fastPathWithinTargetFontEnvForProbe(
		map[string]string{"PDF_DEBUG_SKIP_TEXT_BASE_FONTS": "CMR10,CMSY10,SFTT0900"},
	)
	require.Equal(t, "CMR10,CMSY10,SFTT0900", env["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"])
	require.Equal(t, "fast-path", env["PDF_DEBUG_TEXT_RENDER_MODE"])
}

func TestGlyphSupersampleWithinTargetFontEnvForProbe_AddsFactor(t *testing.T) {
	env := glyphSupersampleWithinTargetFontEnvForProbe(
		map[string]string{"PDF_DEBUG_SKIP_TEXT_BASE_FONTS": "CMR10,CMSY10,SFTT0900"},
		2,
	)
	require.Equal(t, "CMR10,CMSY10,SFTT0900", env["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"])
	require.Equal(t, "2", env["PDF_DEBUG_TEXT_GLYPH_SUPERSAMPLE"])
}

func TestForcedFastPathWithinTargetFontEnvForProbe_MergesForcedFontMapAndFastPath(t *testing.T) {
	env := forcedFastPathWithinTargetFontEnvForProbe(
		map[string]string{"PDF_DEBUG_SKIP_TEXT_BASE_FONTS": "CMR10,CMSY10,SFTT0900"},
		map[string]string{"PDF_DEBUG_FORCE_BASE_FONT_MAP": "SFRM1095=Times-Italic"},
	)
	require.Equal(t, "CMR10,CMSY10,SFTT0900", env["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"])
	require.Equal(t, "SFRM1095=Times-Italic", env["PDF_DEBUG_FORCE_BASE_FONT_MAP"])
	require.Equal(t, "fast-path", env["PDF_DEBUG_TEXT_RENDER_MODE"])
}

func TestEmbeddedWithinTargetFontEnvForProbe_AddsEmbeddedBaseFont(t *testing.T) {
	env := embeddedWithinTargetFontEnvForProbe(
		map[string]string{"PDF_DEBUG_SKIP_TEXT_BASE_FONTS": "CMR10,CMSY10,SFTT0900"},
		"SFRM1095",
	)
	require.Equal(t, "CMR10,CMSY10,SFTT0900", env["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"])
	require.Equal(t, "SFRM1095", env["PDF_DEBUG_TYPE1_EMBEDDED_FOR_BASE"])
}

func TestFallbackWithinTargetFontEnvForProbe_AddsFallbackBaseFont(t *testing.T) {
	env := fallbackWithinTargetFontEnvForProbe(
		map[string]string{"PDF_DEBUG_SKIP_TEXT_BASE_FONTS": "CMR10,CMSY10,SFTT0900"},
		"SFRM1095",
	)
	require.Equal(t, "CMR10,CMSY10,SFTT0900", env["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"])
	require.Equal(t, "SFRM1095", env["PDF_DEBUG_TYPE1_FALLBACK_FOR_BASE"])
}

func TestExpandedCodeSkipWithinTargetFontEnvForProbe_AddsExpandedCodeSpec(t *testing.T) {
	tc := realPageLowercaseProbeCase{
		topSetCodeSpec:    "SFRM1095=101,110",
		secondaryCodeSpec: "SFRM1095=97,115",
		longTailCodeSpec:  "SFRM1095=99,107",
		nonLowerCodeSpec:  "SFRM1095=65,49",
	}
	env := expandedCodeSkipWithinTargetFontEnvForProbe(
		map[string]string{"PDF_DEBUG_SKIP_TEXT_BASE_FONTS": "CMR10,CMSY10,SFTT0900"},
		tc,
	)
	require.Equal(t, "CMR10,CMSY10,SFTT0900", env["PDF_DEBUG_SKIP_TEXT_BASE_FONTS"])
	require.Equal(t, "SFRM1095=101,110,97,115,99,107,65,49", env["PDF_DEBUG_SKIP_TEXT_CODES_FOR_BASE"])
}

func TestSetProbeEnvForRender_RestoresAdditionalDebugKeys(t *testing.T) {
	t.Setenv("PDF_DEBUG_SKIP_TEXT", "0")
	t.Setenv("PDF_DEBUG_TYPE1_EMBEDDED_FOR_BASE", "CMR10")
	t.Setenv("PDF_DEBUG_TYPE1_FALLBACK_FOR_BASE", "CMSY10")
	t.Setenv("PDF_DEBUG_TEXT_GLYPH_GAMMA", "0.9")

	restore := setProbeEnvForRender(t, map[string]string{
		"PDF_DEBUG_SKIP_TEXT":               "1",
		"PDF_DEBUG_TYPE1_EMBEDDED_FOR_BASE": "SFRM1095",
		"PDF_DEBUG_TYPE1_FALLBACK_FOR_BASE": "SFRM1095",
		"PDF_DEBUG_TEXT_GLYPH_GAMMA":        "0.8",
	})

	require.Equal(t, "1", os.Getenv("PDF_DEBUG_SKIP_TEXT"))
	require.Equal(t, "SFRM1095", os.Getenv("PDF_DEBUG_TYPE1_EMBEDDED_FOR_BASE"))
	require.Equal(t, "SFRM1095", os.Getenv("PDF_DEBUG_TYPE1_FALLBACK_FOR_BASE"))
	require.Equal(t, "0.8", os.Getenv("PDF_DEBUG_TEXT_GLYPH_GAMMA"))

	restore()

	require.Equal(t, "0", os.Getenv("PDF_DEBUG_SKIP_TEXT"))
	require.Equal(t, "CMR10", os.Getenv("PDF_DEBUG_TYPE1_EMBEDDED_FOR_BASE"))
	require.Equal(t, "CMSY10", os.Getenv("PDF_DEBUG_TYPE1_FALLBACK_FOR_BASE"))
	require.Equal(t, "0.9", os.Getenv("PDF_DEBUG_TEXT_GLYPH_GAMMA"))
}
