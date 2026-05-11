package pdf_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestGeoTopoPage35ResidualTriageProbeAgainstPoppler(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	target := realPageProbeTarget{
		name:       "009_p35_geotopo",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 35,
	}

	popplerPNG := preparePopplerPageForProbe(t, target)
	fullPNG := renderPageToPNGForProbe(t, target, nil)
	full := measurePNGPlacementProbeForProbe(t, popplerPNG, fullPNG, 4)
	popplerImg := loadPNGAsRGBAForProbe(t, popplerPNG)
	fullImg := loadPNGAsRGBAForProbe(t, fullPNG)
	rows, cols := collectSyntheticFontDiffHotspotsForProbe(t, popplerPNG, fullPNG, 10)
	deltas, mismatchPixels, deltaBounds := topMaskedNonzeroRGBDeltasForProbe(
		popplerImg,
		fullImg,
		popplerImg.Bounds(),
		nil,
		12,
	)
	t.Logf(
		"baseline exact=%.4f similarity=%.4f best_shift=(%d,%d) best_shift_exact=%.4f best_shift_similarity=%.4f diff_bounds=%v row_hotspots=%s col_hotspots=%s mismatch_pixels=%d delta_bounds=%v deltas=%s samples=%s",
		full.originalExact,
		full.originalSimilarity,
		full.bestShiftX,
		full.bestShiftY,
		full.bestShiftExact,
		full.bestShiftSimilarity,
		full.diffBounds,
		formatGeoTopoPage35HotspotsForProbe(rows),
		formatGeoTopoPage35HotspotsForProbe(cols),
		mismatchPixels,
		deltaBounds,
		deltas,
		sampleMaskedMismatchedRGBForProbe(popplerImg, fullImg, popplerImg.Bounds(), nil, 8),
	)

	operatorHistogram := measureSamplePageOperatorHistogramForProbe(t, target.pdfPath, target.pageNumber)
	t.Logf(
		"raw_content stream_count=%d operator_hist=%s",
		operatorHistogram.streamCount,
		formatFaithfulSyntheticTextOperatorHistogramForProbe(operatorHistogram.operatorCounts),
	)

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, faithfulSyntheticTextCompositionProbeCase{
		name:       target.name,
		pdfPath:    target.pdfPath,
		pageNumber: target.pageNumber,
		dpi:        defaultRealPageProbeDPI,
	})
	defer source.doc.Close()
	t.Logf(
		"evaluated operator_hist=%s dropped_hist=%s font_usage=%s",
		formatFaithfulSyntheticTextOperatorHistogramForProbe(source.operatorHist),
		formatFaithfulSyntheticTextOperatorHistogramForProbe(source.droppedHist),
		formatFaithfulSyntheticTextFontUsageForProbe(source.fontUsage, 12),
	)
	t.Logf(
		"resources xobjects=%s patterns=%s extgstate=%s shadings=%s",
		collectFaithfulSyntheticXObjectSummaryForProbe(t, source.doc.XRef(), source.resources),
		collectFaithfulSyntheticPatternSummaryForProbe(t, source.doc.XRef(), source.resources),
		collectFaithfulSyntheticNamedResourceSummaryForProbe(t, source.doc.XRef(), source.resources, entity.Name("ExtGState")),
		collectFaithfulSyntheticNamedResourceSummaryForProbe(t, source.doc.XRef(), source.resources, entity.Name("Shading")),
	)

	for _, variant := range geoTopoPage35ProbeVariants() {
		oursPNG := renderPageToPNGForProbe(t, target, variant.env)
		placement := measurePNGPlacementProbeForProbe(t, popplerPNG, oursPNG, 0)
		variantImg := loadPNGAsRGBAForProbe(t, oursPNG)
		variantRows, variantCols := collectSyntheticFontDiffHotspotsForProbe(t, popplerPNG, oursPNG, 5)
		variantDeltas, variantMismatchPixels, variantDeltaBounds := topMaskedNonzeroRGBDeltasForProbe(
			popplerImg,
			variantImg,
			popplerImg.Bounds(),
			nil,
			8,
		)
		t.Logf(
			"variant=%s exact=%.4f similarity=%.4f diff_bounds=%v nonwhite_bounds=%v row_hotspots=%s col_hotspots=%s mismatch_pixels=%d delta_bounds=%v deltas=%s",
			variant.name,
			placement.originalExact,
			placement.originalSimilarity,
			placement.diffBounds,
			nonWhiteBoundsForProbe(variantImg),
			formatGeoTopoPage35HotspotsForProbe(variantRows),
			formatGeoTopoPage35HotspotsForProbe(variantCols),
			variantMismatchPixels,
			variantDeltaBounds,
			variantDeltas,
		)
	}
}

type geoTopoPage35ProbeVariant struct {
	name string
	env  map[string]string
}

func geoTopoPage35ProbeVariants() []geoTopoPage35ProbeVariant {
	return []geoTopoPage35ProbeVariant{
		{name: "full"},
		{name: "skip_text", env: map[string]string{"PDF_DEBUG_SKIP_TEXT": "1"}},
		{name: "skip_xobjects", env: map[string]string{"PDF_DEBUG_SKIP_XOBJECTS": "1"}},
		{name: "skip_images", env: map[string]string{"PDF_DEBUG_SKIP_IMAGES": "1"}},
		{name: "skip_fill_paths", env: map[string]string{"PDF_DEBUG_SKIP_FILL_PATHS": "1"}},
		{name: "skip_stroke_paths", env: map[string]string{"PDF_DEBUG_SKIP_STROKE_PATHS": "1"}},
		{name: "vector_only", env: map[string]string{
			"PDF_DEBUG_SKIP_TEXT":     "1",
			"PDF_DEBUG_SKIP_XOBJECTS": "1",
		}},
	}
}

func formatGeoTopoPage35HotspotsForProbe(hotspots []syntheticFontDiffHotspot) string {
	if len(hotspots) == 0 {
		return "-"
	}

	parts := make([]string, 0, len(hotspots))
	for _, hotspot := range hotspots {
		parts = append(parts, fmt.Sprintf("%d:%d", hotspot.index, hotspot.count))
	}
	return strings.Join(parts, ",")
}
