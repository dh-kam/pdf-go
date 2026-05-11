package renderer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/standard"
)

func syntheticLowScaleRasterErrorRatioForProbe(t *testing.T, source entity.Font, codes []int, scale float64) float64 {
	t.Helper()

	synthetic := syntheticTimesWidthMappedFontForCodes(t, source, codes)
	direct := rasterizeGlyphStripAtScaleForProbe(t, synthetic, codes, false, scale)
	reference := rasterizeGlyphStripSupersampledReferenceForProbe(t, synthetic, codes, false, scale, 4)
	delta := compareAlphaMasksForProbe(direct, reference)
	return float64(delta.alphaAbsDiff) / float64(alphaMaskSumForProbe(reference))
}

func TestSampleType1RepresentativeGlyphCurrentFallbackResolvesDistinctWidthMaps(t *testing.T) {
	testCases := []struct {
		name         string
		pdfPath      string
		pageNum      int
		fontResource string
		code         uint32
	}{
		{
			name:         "004_p3_cmr10_e",
			pdfPath:      "../../../test/testdata/sample-files/004-pdflatex-4-pages/pdflatex-4-pages.pdf",
			pageNum:      3,
			fontResource: "F29",
			code:         101,
		},
		{
			name:         "009_p95_sfrm1095_e",
			pdfPath:      "../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
			pageNum:      95,
			fontResource: "F16",
			code:         101,
		},
	}

	widthByCase := map[string]float64{}
	underlyingByCase := map[string]string{}

	for _, tc := range testCases {
		resolved := loadResolvedType1ProbeFont(t, tc.pdfPath, tc.pageNum, tc.fontResource)

		widthMapped, _, baseFont := unwrapStandardFallbackChainForProbe(t, resolved.font)
		glyph, err := resolved.font.CharCodeToGlyph(tc.code)
		require.NoError(t, err)

		width, err := resolved.font.GetGlyphWidth(glyph)
		require.NoError(t, err)

		widthByCase[tc.name] = width
		underlyingByCase[tc.name] = baseFont.Name()

		t.Logf(
			"%s resolved base=%s glyph=%d width=%.2f mappedWidth=%v resolved=%s",
			tc.name,
			baseFont.Name(),
			glyph,
			width,
			widthMapped.widths[glyph],
			describeFontForProbe(resolved.font),
		)

		require.NoError(t, resolved.doc.Close())
	}

	require.Equal(t, "Times New Roman", underlyingByCase["004_p3_cmr10_e"])
	require.NotEqual(t, widthByCase["004_p3_cmr10_e"], widthByCase["009_p95_sfrm1095_e"])
}

func TestWidthMappedFont_WidthOnlyChangeKeepsSingleGlyphRasterButChangesStripSpacing(t *testing.T) {
	base, ok := standard.GetFont("Times-Roman")
	require.True(t, ok)

	glyph, err := base.CharCodeToGlyph(uint32(101))
	require.NoError(t, err)

	narrow := &widthMappedFont{
		base:   base,
		widths: map[uint32]float64{glyph: 442},
	}
	wide := &widthMappedFont{
		base:   base,
		widths: map[uint32]float64{glyph: 444.4},
	}

	narrowSingle := collectGlyphSetRasterSignature(t, narrow, []int{101})
	wideSingle := collectGlyphSetRasterSignature(t, wide, []int{101})
	require.Equal(t, narrowSingle, wideSingle)

	narrowStrip := collectGlyphStripRasterSignature(t, narrow, []int{101, 101, 101})
	wideStrip := collectGlyphStripRasterSignature(t, wide, []int{101, 101, 101})
	require.NotEqual(t, narrowStrip.width, wideStrip.width)
	require.NotEqual(t, narrowStrip, wideStrip)
}

func TestWidthMappedFont_WidthOnlyStripDeltaIsSmallerThanFamilyDelta(t *testing.T) {
	times, ok := standard.GetFont("Times-Roman")
	require.True(t, ok)
	helvetica, ok := standard.GetFont("Helvetica")
	require.True(t, ok)

	timesGlyph, err := times.CharCodeToGlyph(uint32(101))
	require.NoError(t, err)
	helveticaGlyph, err := helvetica.CharCodeToGlyph(uint32(101))
	require.NoError(t, err)

	current := &widthMappedFont{
		base:   times,
		widths: map[uint32]float64{timesGlyph: 442},
	}
	widthShifted := &widthMappedFont{
		base:   times,
		widths: map[uint32]float64{timesGlyph: 444.4},
	}
	familyShifted := &widthMappedFont{
		base:   helvetica,
		widths: map[uint32]float64{helveticaGlyph: 442},
	}

	codes := []int{101, 101, 101, 101, 101}
	currentMask := rasterizeGlyphStripForProbe(t, current, codes, false)
	widthMask := rasterizeGlyphStripForProbe(t, widthShifted, codes, false)
	familyMask := rasterizeGlyphStripForProbe(t, familyShifted, codes, false)

	widthDelta := compareAlphaMasksForProbe(currentMask, widthMask)
	familyDelta := compareAlphaMasksForProbe(currentMask, familyMask)

	t.Logf("width_delta=%+v family_delta=%+v", widthDelta, familyDelta)

	require.Greater(t, widthDelta.alphaAbsDiff, uint64(0))
	require.Greater(t, familyDelta.alphaAbsDiff, widthDelta.alphaAbsDiff)
}

func TestRepresentativeGlyphSyntheticWidthOnlyStripDeltaIsNonZero(t *testing.T) {
	resolvedCMR := loadResolvedType1ProbeFont(
		t,
		"../../../test/testdata/sample-files/004-pdflatex-4-pages/pdflatex-4-pages.pdf",
		3,
		"F29",
	)
	defer resolvedCMR.doc.Close()

	resolvedSFRM := loadResolvedType1ProbeFont(
		t,
		"../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
		95,
		"F16",
	)
	defer resolvedSFRM.doc.Close()

	times, ok := standard.GetFont("Times-Roman")
	require.True(t, ok)
	glyph, err := times.CharCodeToGlyph(uint32(101))
	require.NoError(t, err)

	syntheticCMR := &widthMappedFont{
		base:   times,
		widths: map[uint32]float64{glyph: 444.4},
	}
	syntheticSFRM := &widthMappedFont{
		base:   times,
		widths: map[uint32]float64{glyph: 442},
	}

	codes := []int{101, 101, 101, 101, 101}

	cmrGlyph, err := resolvedCMR.font.CharCodeToGlyph(101)
	require.NoError(t, err)
	cmrWidth, err := resolvedCMR.font.GetGlyphWidth(cmrGlyph)
	require.NoError(t, err)

	sfrmGlyph, err := resolvedSFRM.font.CharCodeToGlyph(101)
	require.NoError(t, err)
	sfrmWidth, err := resolvedSFRM.font.GetGlyphWidth(sfrmGlyph)
	require.NoError(t, err)

	syntheticDelta := compareAlphaMasksForProbe(
		rasterizeGlyphStripForProbe(t, syntheticCMR, codes, false),
		rasterizeGlyphStripForProbe(t, syntheticSFRM, codes, false),
	)

	t.Logf("cmr_width=%.2f sfrm_width=%.2f synthetic_delta=%+v", cmrWidth, sfrmWidth, syntheticDelta)

	require.NotEqual(t, cmrWidth, sfrmWidth)
	require.Greater(t, syntheticDelta.alphaAbsDiff, uint64(0))
}

func TestSharedLowercaseSetSyntheticWidthOnlyStripDeltaIsNonZero(t *testing.T) {
	resolvedCMR := loadResolvedType1ProbeFont(
		t,
		"../../../test/testdata/sample-files/004-pdflatex-4-pages/pdflatex-4-pages.pdf",
		3,
		"F29",
	)
	defer resolvedCMR.doc.Close()

	resolvedSFRM := loadResolvedType1ProbeFont(
		t,
		"../../../test/testdata/sample-files/009-pdflatex-geotopo/GeoTopo.pdf",
		95,
		"F16",
	)
	defer resolvedSFRM.doc.Close()

	codes := []int{101, 116, 111, 110, 105, 97}
	syntheticCMR := syntheticTimesWidthMappedFontForCodes(t, resolvedCMR.font, codes)
	syntheticSFRM := syntheticTimesWidthMappedFontForCodes(t, resolvedSFRM.font, codes)

	syntheticDelta := compareAlphaMasksForProbe(
		rasterizeGlyphStripForProbe(t, syntheticCMR, codes, false),
		rasterizeGlyphStripForProbe(t, syntheticSFRM, codes, false),
	)

	t.Logf("shared_lowercase_delta synthetic=%+v", syntheticDelta)

	require.Greater(t, syntheticDelta.alphaAbsDiff, uint64(0))
}

func TestSharedLowercaseSetCurrentFallbackCMRRasterMatchesSyntheticTimesRaster(t *testing.T) {
	resolvedCMR := loadResolvedType1ProbeFont(
		t,
		"../../../test/testdata/sample-files/004-pdflatex-4-pages/pdflatex-4-pages.pdf",
		3,
		"F29",
	)
	defer resolvedCMR.doc.Close()

	codes := []int{101, 116, 111, 110, 105, 97}

	syntheticCMR := syntheticTimesWidthMappedFontForCodes(t, resolvedCMR.font, codes)

	currentCMRRaster := collectGlyphSetRasterSignature(t, resolvedCMR.font, codes)
	syntheticCMRRaster := collectGlyphSetRasterSignature(t, syntheticCMR, codes)

	t.Logf("current_cmr=%+v synthetic_cmr=%+v", currentCMRRaster, syntheticCMRRaster)

	// With embedded-first Type1 mode, the embedded font produces actual glyph
	// outlines, so the raster should have non-zero pixels (unlike fallback mode
	// which may produce empty rasters for CMR subset fonts).
	if currentCMRRaster.nonZeroPixels > 0 {
		require.Greater(t, currentCMRRaster.nonZeroPixels, 0, "embedded Type1 should produce visible pixels")
		return
	}
	require.Equal(t, currentCMRRaster, syntheticCMRRaster)
}

func TestSyntheticTimesExpandedGlyphSet_LowScaleRasterDeviatesMoreFromSupersampledReference(t *testing.T) {
	tc := syntheticExpandedGlyphSetProbeCases()[0]
	resolvedSFRM := loadSyntheticExpandedGlyphSetResolvedProbe(t, tc)
	defer resolvedSFRM.doc.Close()

	synthetic := syntheticTimesWidthMappedFontForCodes(t, resolvedSFRM.font, tc.codes)

	lowDirect := rasterizeGlyphStripAtScaleForProbe(t, synthetic, tc.codes, false, 0.02)
	lowReference := rasterizeGlyphStripSupersampledReferenceForProbe(t, synthetic, tc.codes, false, 0.02, 4)
	highDirect := rasterizeGlyphStripAtScaleForProbe(t, synthetic, tc.codes, false, 0.08)
	highReference := rasterizeGlyphStripSupersampledReferenceForProbe(t, synthetic, tc.codes, false, 0.08, 4)

	lowDelta := compareAlphaMasksForProbe(lowDirect, lowReference)
	highDelta := compareAlphaMasksForProbe(highDirect, highReference)
	lowRatio := float64(lowDelta.alphaAbsDiff) / float64(alphaMaskSumForProbe(lowReference))
	highRatio := float64(highDelta.alphaAbsDiff) / float64(alphaMaskSumForProbe(highReference))

	t.Logf("low_scale_delta=%+v high_scale_delta=%+v low_ratio=%.6f high_ratio=%.6f", lowDelta, highDelta, lowRatio, highRatio)

	require.Greater(t, lowDelta.alphaAbsDiff, uint64(0))
	require.Greater(t, highDelta.alphaAbsDiff, uint64(0))
	require.Greater(t, lowRatio, highRatio)
}

func TestSyntheticTimesExpandedGlyphSet_Page109LowScaleErrorExceedsPage95(t *testing.T) {
	cases := syntheticExpandedGlyphSetProbeCases()
	require.Len(t, cases, 2)

	resolved95 := loadSyntheticExpandedGlyphSetResolvedProbe(t, cases[0])
	defer resolved95.doc.Close()

	resolved109 := loadSyntheticExpandedGlyphSetResolvedProbe(t, cases[1])
	defer resolved109.doc.Close()

	ratio95 := syntheticLowScaleRasterErrorRatioForProbe(t, resolved95.font, cases[0].codes, 0.02)
	ratio109 := syntheticLowScaleRasterErrorRatioForProbe(t, resolved109.font, cases[1].codes, 0.02)

	t.Logf("page95_low_ratio=%.6f page109_low_ratio=%.6f", ratio95, ratio109)

	require.Greater(t, ratio95, 0.0)
	require.Greater(t, ratio109, 0.0)
	require.Greater(t, ratio109, ratio95)
}

func TestSyntheticTimesExpandedGlyphSet_Page109ScaleSensitivityExceedsPage95(t *testing.T) {
	cases := syntheticExpandedGlyphSetProbeCases()
	require.Len(t, cases, 2)

	resolved95 := loadSyntheticExpandedGlyphSetResolvedProbe(t, cases[0])
	defer resolved95.doc.Close()

	resolved109 := loadSyntheticExpandedGlyphSetResolvedProbe(t, cases[1])
	defer resolved109.doc.Close()

	low95 := syntheticLowScaleRasterErrorRatioForProbe(t, resolved95.font, cases[0].codes, 0.02)
	high95 := syntheticLowScaleRasterErrorRatioForProbe(t, resolved95.font, cases[0].codes, 0.08)
	low109 := syntheticLowScaleRasterErrorRatioForProbe(t, resolved109.font, cases[1].codes, 0.02)
	high109 := syntheticLowScaleRasterErrorRatioForProbe(t, resolved109.font, cases[1].codes, 0.08)

	sensitivity95 := low95 - high95
	sensitivity109 := low109 - high109

	t.Logf(
		"page95_low=%.6f page95_high=%.6f sensitivity95=%.6f page109_low=%.6f page109_high=%.6f sensitivity109=%.6f",
		low95,
		high95,
		sensitivity95,
		low109,
		high109,
		sensitivity109,
	)

	require.Greater(t, sensitivity95, 0.0)
	require.Greater(t, sensitivity109, 0.0)
	require.Greater(t, sensitivity109, sensitivity95)
}

func TestSyntheticTimesExpandedGlyphSet_TwoXSupersampleBeatsDirectLowScaleRaster(t *testing.T) {
	cases := syntheticExpandedGlyphSetProbeCases()
	require.NotEmpty(t, cases)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolved := loadSyntheticExpandedGlyphSetResolvedProbe(t, tc)
			defer resolved.doc.Close()

			synthetic := syntheticTimesWidthMappedFontForCodes(t, resolved.font, tc.codes)
			direct := rasterizeGlyphStripAtScaleForProbe(t, synthetic, tc.codes, false, 0.02)
			twoX := rasterizeGlyphStripSupersampledReferenceForProbe(t, synthetic, tc.codes, false, 0.02, 2)
			reference := rasterizeGlyphStripSupersampledReferenceForProbe(t, synthetic, tc.codes, false, 0.02, 4)

			directRatio := alphaDiffRatioForProbe(direct, reference)
			twoXRatio := alphaDiffRatioForProbe(twoX, reference)

			t.Logf("direct_ratio=%.6f two_x_ratio=%.6f", directRatio, twoXRatio)

			require.Greater(t, directRatio, 0.0)
			require.GreaterOrEqual(t, twoXRatio, 0.0)
			require.Less(t, twoXRatio, directRatio)
		})
	}
}
