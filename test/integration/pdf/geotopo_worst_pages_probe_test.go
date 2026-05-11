package pdf_test

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestGeoTopoWorstPagesResidualTriageProbeAgainstPoppler(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	for _, pageNumber := range []int{24, 25} {
		target := realPageProbeTarget{
			name:       "009_geotopo_worst",
			pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
			pageNumber: pageNumber,
		}
		t.Run("page_"+fmtPageNumberForGeoTopoProbe(pageNumber), func(t *testing.T) {
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
				"baseline exact=%.4f similarity=%.4f diff_bounds=%v row_hotspots=%s col_hotspots=%s mismatch_pixels=%d delta_bounds=%v deltas=%s samples=%s",
				full.originalExact,
				full.originalSimilarity,
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
			t.Logf("image_placements=%s", formatGeoTopoImagePlacementsForProbe(
				measureSamplePageImagePlacementOpsForProbe(t, target.pdfPath, target.pageNumber),
			))

			for _, variant := range geoTopoPage35ProbeVariants() {
				oursPNG := renderPageToPNGForProbe(t, target, variant.env)
				placement := measurePNGPlacementProbeForProbe(t, popplerPNG, oursPNG, 0)
				t.Logf(
					"variant=%s exact=%.4f similarity=%.4f diff_bounds=%v",
					variant.name,
					placement.originalExact,
					placement.originalSimilarity,
					placement.diffBounds,
				)
			}
		})
	}
}

func fmtPageNumberForGeoTopoProbe(pageNumber int) string {
	return fmt.Sprintf("%02d", pageNumber)
}

func formatGeoTopoImagePlacementsForProbe(placements []samplePageImagePlacementInvokeProbeResult) string {
	if len(placements) == 0 {
		return "-"
	}

	parts := make([]string, 0, len(placements))
	for _, placement := range placements {
		parts = append(parts, fmt.Sprintf(
			"%s@%d_0R=[%.6f %.6f %.6f %.6f %.6f %.6f]",
			placement.imageName,
			placement.imageRef.Num(),
			placement.matrix[0],
			placement.matrix[1],
			placement.matrix[2],
			placement.matrix[3],
			placement.matrix[4],
			placement.matrix[5],
		))
	}
	return strings.Join(parts, ";")
}
