package pdf_test

import (
	"context"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	pdfxref "github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
	"github.com/dh-kam/pdf-go/pkg/pdf"
)

type realPageResidualMixProbeResult struct {
	fullSimilarity             float64
	fullNoImageSimilarity      float64
	fullNoXObjectSimilarity    float64
	fullNoStrokeSimilarity     float64
	allTextSkippedSimilarity   float64
	nonTextNoImageSimilarity   float64
	nonTextNoStrokeSimilarity  float64
	vectorOnlySimilarity       float64
	targetOnlySimilarity       float64
	targetSkippedSimilarity    float64
	fullTargetSkipSimilarity   float64
	nonTextImageGap            float64
	nonTextXObjectGap          float64
	nonTextStrokeGap           float64
	allTextResidualGap         float64
	targetOnlyResidualGap      float64
	targetRemovedResidualGap   float64
	targetSkipVsAllTextDelta   float64
	allTextSkipVsBaseSkipDelta float64
}

func TestType1RealPageP95ResidualMixProbe(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	tc := requireRealPageLowercaseProbeCase(t, "009_p95_sfrm1095_top6")
	allBaseFonts := pageBaseFontsForProbe(t, tc.target)
	popplerPNG := preparePopplerPageForProbe(t, tc.target)

	targetOnlyEnv := targetFontOnlyEnvForProbe(allBaseFonts, tc.baseFont)
	targetSkippedEnv := map[string]string{
		"PDF_DEBUG_SKIP_TEXT_BASE_FONTS": tc.baseFont,
	}
	fullNoImageEnv := map[string]string{
		"PDF_DEBUG_SKIP_IMAGES": "1",
	}
	fullNoXObjectEnv := map[string]string{
		"PDF_DEBUG_SKIP_XOBJECTS": "1",
	}
	fullNoStrokeEnv := map[string]string{
		"PDF_DEBUG_SKIP_STROKE_PATHS": "1",
	}
	allTextSkippedEnv := map[string]string{
		"PDF_DEBUG_SKIP_TEXT": "1",
	}
	vectorOnlyEnv := map[string]string{
		"PDF_DEBUG_SKIP_TEXT":     "1",
		"PDF_DEBUG_SKIP_XOBJECTS": "1",
	}
	nonTextNoImageEnv := map[string]string{
		"PDF_DEBUG_SKIP_IMAGES": "1",
		"PDF_DEBUG_SKIP_TEXT":   "1",
	}
	nonTextNoStrokeEnv := map[string]string{
		"PDF_DEBUG_SKIP_STROKE_PATHS": "1",
		"PDF_DEBUG_SKIP_TEXT":         "1",
	}

	result := realPageResidualMixProbeResult{
		fullSimilarity:            renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, nil),
		fullNoImageSimilarity:     renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, fullNoImageEnv),
		fullNoXObjectSimilarity:   renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, fullNoXObjectEnv),
		fullNoStrokeSimilarity:    renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, fullNoStrokeEnv),
		allTextSkippedSimilarity:  renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, allTextSkippedEnv),
		nonTextNoImageSimilarity:  renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, nonTextNoImageEnv),
		nonTextNoStrokeSimilarity: renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, nonTextNoStrokeEnv),
		vectorOnlySimilarity:      renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, vectorOnlyEnv),
		targetOnlySimilarity:      renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, targetOnlyEnv),
		targetSkippedSimilarity:   renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, targetSkippedEnv),
		fullTargetSkipSimilarity:  renderPageSimilarityAgainstPopplerForProbe(t, tc.target, popplerPNG, fullTargetFontSkipEnvForProbe(targetOnlyEnv, tc.baseFont)),
	}

	result.nonTextImageGap = result.allTextSkippedSimilarity - result.nonTextNoImageSimilarity
	result.nonTextXObjectGap = result.allTextSkippedSimilarity - result.vectorOnlySimilarity
	result.nonTextStrokeGap = result.allTextSkippedSimilarity - result.nonTextNoStrokeSimilarity
	result.allTextResidualGap = result.allTextSkippedSimilarity - result.fullSimilarity
	result.targetOnlyResidualGap = result.allTextSkippedSimilarity - result.targetOnlySimilarity
	result.targetRemovedResidualGap = result.targetSkippedSimilarity - result.fullSimilarity
	result.targetSkipVsAllTextDelta = result.targetSkippedSimilarity - result.allTextSkippedSimilarity
	result.allTextSkipVsBaseSkipDelta = result.fullTargetSkipSimilarity - result.allTextSkippedSimilarity

	operatorHistogram := measureSamplePageOperatorHistogramForProbe(t, tc.target.pdfPath, tc.target.pageNumber)
	imagePlacements := measureSamplePageImagePlacementOpsForProbe(t, tc.target.pdfPath, tc.target.pageNumber)
	expandedResidual := measureExpandedResidualProbeAgainstPoppler(t, tc, targetOnlyEnv, popplerPNG)
	contentBounds := nonWhiteBoundsForProbe(loadPNGAsRGBAForProbe(t, popplerPNG))
	fullContentSimilarity := renderPageSimilarityAgainstPopplerWithinBoundsForProbe(t, tc.target, popplerPNG, nil, contentBounds)
	allTextSkippedContentSimilarity := renderPageSimilarityAgainstPopplerWithinBoundsForProbe(t, tc.target, popplerPNG, allTextSkippedEnv, contentBounds)
	targetOnlyContentSimilarity := renderPageSimilarityAgainstPopplerWithinBoundsForProbe(t, tc.target, popplerPNG, targetOnlyEnv, contentBounds)
	targetSkippedContentSimilarity := renderPageSimilarityAgainstPopplerWithinBoundsForProbe(t, tc.target, popplerPNG, targetSkippedEnv, contentBounds)
	fontContentRanking := measureFontOnlyContentSimilarityRankingForProbe(t, tc.target, popplerPNG, allBaseFonts, contentBounds, allTextSkippedContentSimilarity)
	if len(imagePlacements) > 0 {
		objectType, subtype := measureXObjectRefKindForProbe(t, tc.target.pdfPath, imagePlacements[0].imageRef)
		t.Logf(
			"xobject_placement name=%s ref=%v matrix=%v object_type=%s subtype=%s",
			imagePlacements[0].imageName,
			imagePlacements[0].imageRef,
			imagePlacements[0].matrix,
			objectType,
			subtype,
		)
	}

	t.Logf(
		"base_fonts=%v operator_counts=%v image_placements=%d stream_count=%d",
		allBaseFonts,
		operatorHistogram.operatorCounts,
		len(imagePlacements),
		operatorHistogram.streamCount,
	)
	t.Logf(
		"full=%.4f full_no_image=%.4f full_no_xobject=%.4f full_no_stroke=%.4f skip_all_text=%.4f non_text_no_image=%.4f non_text_no_stroke=%.4f vector_only=%.4f target_only=%.4f target_skipped=%.4f full_target_skip=%.4f non_text_image_gap=%.4f non_text_xobject_gap=%.4f non_text_stroke_gap=%.4f all_text_gap=%.4f target_only_gap=%.4f target_removed_gap=%.4f target_skip_vs_all_text_delta=%.4f all_text_skip_vs_base_skip_delta=%.4f",
		result.fullSimilarity,
		result.fullNoImageSimilarity,
		result.fullNoXObjectSimilarity,
		result.fullNoStrokeSimilarity,
		result.allTextSkippedSimilarity,
		result.nonTextNoImageSimilarity,
		result.nonTextNoStrokeSimilarity,
		result.vectorOnlySimilarity,
		result.targetOnlySimilarity,
		result.targetSkippedSimilarity,
		result.fullTargetSkipSimilarity,
		result.nonTextImageGap,
		result.nonTextXObjectGap,
		result.nonTextStrokeGap,
		result.allTextResidualGap,
		result.targetOnlyResidualGap,
		result.targetRemovedResidualGap,
		result.targetSkipVsAllTextDelta,
		result.allTextSkipVsBaseSkipDelta,
	)
	t.Logf(
		"poppler_content_bounds=%v full_content_similarity=%.4f skip_all_text_content_similarity=%.4f target_only_content_similarity=%.4f target_skipped_content_similarity=%.4f",
		contentBounds,
		fullContentSimilarity,
		allTextSkippedContentSimilarity,
		targetOnlyContentSimilarity,
		targetSkippedContentSimilarity,
	)
	t.Logf("font_only_content_ranking=%v", fontContentRanking)
	t.Logf(
		"expanded_only=%.4f full_skip_only=%.4f forced_residual_only=%.4f fast_path_residual_only=%.4f embedded_residual_only=%.4f fallback_residual_only=%.4f supersampled_only=%.4f glyph_source_only=%.4f residual_gap=%.4f max_policy_gain=%.4f expanded_codes=%s target_only_skip=%s",
		expandedResidual.expandedOnly,
		expandedResidual.fullSkipOnly,
		expandedResidual.forcedResidualOnly,
		expandedResidual.fastPathResidualOnly,
		expandedResidual.embeddedResidualOnly,
		expandedResidual.fallbackResidualOnly,
		expandedResidual.supersampledOnly,
		expandedResidual.glyphSourceOnly,
		expandedResidual.residualGap,
		expandedResidual.maxPolicyGain(),
		tc.expandedCodeSpec(),
		expandedResidual.targetOnlySkipBaseEnv,
	)
}

func requireRealPageLowercaseProbeCase(t *testing.T, name string) realPageLowercaseProbeCase {
	t.Helper()

	for _, tc := range realPageLowercaseProbeCases() {
		if tc.target.name == name {
			return tc
		}
	}

	t.Fatalf("real page lowercase probe case %q not found", name)
	return realPageLowercaseProbeCase{}
}

func measureXObjectRefKindForProbe(t *testing.T, pdfPath string, ref entity.Ref) (string, string) {
	t.Helper()
	if ref.Num() == 0 {
		return "unresolved", ""
	}

	data, err := os.ReadFile(pdfPath)
	if err != nil {
		return "read_error", ""
	}

	xrefTable := pdfxref.NewTable(data)
	if err := xrefTable.Parse(); err != nil {
		return "parse_error", ""
	}

	obj, err := xrefTable.Fetch(ref)
	if err != nil {
		return "fetch_error", ""
	}

	switch typed := obj.(type) {
	case *entity.Stream:
		subtype, _ := typed.Dict().Get(entity.Name("Subtype")).(entity.Name)
		return "stream", subtype.Value()
	case *entity.Dict:
		subtype, _ := typed.Get(entity.Name("Subtype")).(entity.Name)
		return "dict", subtype.Value()
	default:
		return "unknown", ""
	}
}

func renderPageSimilarityAgainstPopplerWithinBoundsForProbe(
	t *testing.T,
	target realPageProbeTarget,
	popplerPNG string,
	env map[string]string,
	bounds image.Rectangle,
) float64 {
	t.Helper()

	if bounds.Empty() {
		return renderPageSimilarityAgainstPopplerForProbe(t, target, popplerPNG, env)
	}

	oursPNG := renderPageToPNGForProbe(t, target, env)
	popplerImg := loadPNGAsRGBAForProbe(t, popplerPNG)
	oursImg := loadPNGAsRGBAForProbe(t, oursPNG)

	popplerCropped := cropRGBAForProbe(popplerImg, bounds)
	oursCropped := cropRGBAForProbe(oursImg, bounds)

	root := t.TempDir()
	popplerCropPath := filepath.Join(root, "poppler-crop.png")
	oursCropPath := filepath.Join(root, "ours-crop.png")
	if err := parityWritePNG(popplerCropPath, popplerCropped); err != nil {
		t.Fatalf("write poppler crop: %v", err)
	}
	if err := parityWritePNG(oursCropPath, oursCropped); err != nil {
		t.Fatalf("write ours crop: %v", err)
	}

	_, similarity, err := parityComparePNGs(oursCropPath, popplerCropPath)
	if err != nil {
		t.Fatalf("compare cropped parity: %v", err)
	}
	return similarity
}

func renderPageToPNGForProbe(
	t *testing.T,
	target realPageProbeTarget,
	env map[string]string,
) string {
	t.Helper()

	doc, err := pdf.Open(target.pdfPath)
	if err != nil {
		t.Fatalf("open pdf: %v", err)
	}
	defer doc.Close()

	page, err := doc.Page(target.pageNumber - 1)
	if err != nil {
		t.Fatalf("open page: %v", err)
	}

	restore := setProbeEnvForRender(t, env)
	defer restore()

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = float64(defaultRealPageProbeDPI)

	img, err := renderer.RenderPage(context.Background(), page, opts)
	if err != nil {
		t.Fatalf("render page: %v", err)
	}

	path := filepath.Join(t.TempDir(), "ours.png")
	if err := parityWritePNG(path, img); err != nil {
		t.Fatalf("write png: %v", err)
	}
	return path
}

func nonWhiteBoundsForProbe(img *image.RGBA) image.Rectangle {
	bounds := img.Bounds()
	minX, minY := bounds.Max.X, bounds.Max.Y
	maxX, maxY := bounds.Min.X, bounds.Min.Y
	found := false

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			if r >= 0xFF00 && g >= 0xFF00 && b >= 0xFF00 {
				continue
			}
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
			found = true
		}
	}

	if !found {
		return image.Rectangle{}
	}

	rect := image.Rect(minX, minY, maxX+1, maxY+1)
	return rect.Inset(-2).Intersect(bounds)
}

func cropRGBAForProbe(src *image.RGBA, bounds image.Rectangle) *image.RGBA {
	bounds = bounds.Intersect(src.Bounds())
	dst := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			dst.Set(x-bounds.Min.X, y-bounds.Min.Y, src.At(x, y))
		}
	}
	return dst
}

func measureFontOnlyContentSimilarityRankingForProbe(
	t *testing.T,
	target realPageProbeTarget,
	popplerPNG string,
	baseFonts []string,
	contentBounds image.Rectangle,
	baselineSimilarity float64,
) []string {
	t.Helper()

	type fontContentProbeResult struct {
		name       string
		similarity float64
		gain       float64
	}

	results := make([]fontContentProbeResult, 0, len(baseFonts))
	for _, baseFont := range baseFonts {
		targetOnlyEnv := targetFontOnlyEnvForProbe(baseFonts, baseFont)
		similarity := renderPageSimilarityAgainstPopplerWithinBoundsForProbe(t, target, popplerPNG, targetOnlyEnv, contentBounds)
		results = append(results, fontContentProbeResult{
			name:       baseFont,
			similarity: similarity,
			gain:       similarity - baselineSimilarity,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].gain == results[j].gain {
			return results[i].name < results[j].name
		}
		return results[i].gain > results[j].gain
	})

	formatted := make([]string, 0, len(results))
	for _, result := range results {
		formatted = append(formatted, fmt.Sprintf("%s=%.4f/%.4f", result.name, result.similarity, result.gain))
	}
	return formatted
}
