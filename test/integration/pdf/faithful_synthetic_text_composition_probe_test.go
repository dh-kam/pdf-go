package pdf_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
	infracanvas "github.com/dh-kam/pdf-go/internal/infrastructure/canvas"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"
	internalpdf "github.com/dh-kam/pdf-go/internal/usecase/pdf"
)

type faithfulSyntheticTextCompositionProbeCase struct {
	name       string
	pdfPath    string
	pageNumber int
	dpi        int
}

type faithfulSyntheticTextCompositionProbeSource struct {
	name         string
	mediaBox     [4]float64
	resources    *entity.Dict
	operators    []domainrenderer.Operator
	keptOps      []domainrenderer.Operator
	rawContent   []byte
	fontUsage    []faithfulSyntheticTextFontUsage
	operatorHist map[string]int
	droppedHist  map[string]int
	doc          *entity.Document
}

type faithfulSyntheticTextFontUsage struct {
	resourceName string
	baseFont     string
	subtype      string
	totalCodes   int
}

type faithfulSyntheticTextCompositionProbeResult struct {
	exact       float64
	similarity  float64
	placement   samplePagePlacementProbeResult
	rowHotspots []syntheticFontDiffHotspot
	colHotspots []syntheticFontDiffHotspot
}

type faithfulSyntheticTextXObjectInvocationProbe struct {
	resourceName string
	userBounds   [4]float64
	pixelBounds  image.Rectangle
	effectiveCTM [6]float64
}

type faithfulSyntheticTextVariantProbeCase struct {
	name    string
	keep    func(domainrenderer.Operator) bool
	rewrite func(domainrenderer.Operator) []domainrenderer.Operator
}

type faithfulSyntheticOperatorIndexFilterForProbe func(index int, op domainrenderer.Operator) bool

type faithfulSyntheticTextFormGraphVariantProbeCase struct {
	name   string
	mutate func(t *testing.T, obj entity.Object) entity.Object
}

type faithfulSyntheticTextXObjectDiffProbe struct {
	resourceName   string
	occurrence     int
	pixelBounds    image.Rectangle
	exact          float64
	similarity     float64
	deltas         string
	nonzeroDeltas  string
	mismatchBounds image.Rectangle
	mismatchPixels int
}

type faithfulSyntheticMaskedParityMetrics struct {
	pixels     int
	exact      float64
	similarity float64
}

type faithfulSyntheticBoundaryBandMetrics struct {
	bandRadius int
	support    faithfulSyntheticMaskedParityMetrics
	core       faithfulSyntheticMaskedParityMetrics
	boundary   faithfulSyntheticMaskedParityMetrics
}

type faithfulSyntheticParentContextReplayVariantProbe struct {
	name       string
	content    []byte
	startIndex int
	endIndex   int
}

func TestFaithfulSyntheticTextCompositionProbeAgainstPoppler(t *testing.T) {
	for _, tc := range faithfulSyntheticTextCompositionProbeCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
			defer func() {
				require.NoError(t, source.doc.Close())
			}()

			result := measureFaithfulSyntheticTextCompositionProbeAgainstPoppler(t, source, tc.dpi)
			t.Logf(
				"kept_ops=%d/%d operator_hist=%s dropped_hist=%s font_usage=%s exact=%.4f similarity=%.4f best_shift_similarity=%.4f best_shift=(%d,%d) diff_bounds=%v rows=%s cols=%s",
				len(source.keptOps),
				len(source.operators),
				formatFaithfulSyntheticTextOperatorHistogramForProbe(source.operatorHist),
				formatFaithfulSyntheticTextOperatorHistogramForProbe(source.droppedHist),
				formatFaithfulSyntheticTextFontUsageForProbe(source.fontUsage, 8),
				result.exact,
				result.similarity,
				result.placement.bestShiftSimilarity,
				result.placement.bestShiftX,
				result.placement.bestShiftY,
				result.placement.diffBounds,
				formatSyntheticFontDiffHotspotsForProbe(result.rowHotspots),
				formatSyntheticFontDiffHotspotsForProbe(result.colHotspots),
			)

			require.NotEmpty(t, source.keptOps)
			require.Greater(t, result.similarity, 0.0)
		})
	}
}

func TestFaithfulSyntheticTextCompositionOriginalBridgeProbe(t *testing.T) {
	for _, tc := range faithfulSyntheticTextCompositionProbeCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
			defer func() {
				require.NoError(t, source.doc.Close())
			}()

			pdfBytes := buildFaithfulSyntheticTextCompositionProbePDF(t, source)
			syntheticPDFName := fmt.Sprintf("%s_text_only.pdf", source.name)

			originalPoppler, originalOurs := renderSamplePageAgainstPopplerAtDPIToPNGs(
				t,
				tc.pdfPath,
				tc.pageNumber,
				"",
				tc.dpi,
			)
			syntheticPoppler, syntheticOurs := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(
				t,
				syntheticPDFName,
				pdfBytes,
				"",
				tc.dpi,
			)

			popplerExact, popplerSimilarity, err := parityComparePNGs(syntheticPoppler, originalPoppler)
			require.NoError(t, err)
			oursExact, oursSimilarity, err := parityComparePNGs(syntheticOurs, originalOurs)
			require.NoError(t, err)

			t.Logf(
				"bridge poppler exact=%.4f similarity=%.4f ours exact=%.4f similarity=%.4f",
				popplerExact,
				popplerSimilarity,
				oursExact,
				oursSimilarity,
			)
		})
	}
}

func TestFaithfulSyntheticTextCompositionVariantProbeAgainstPoppler(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	for _, variant := range faithfulSyntheticTextVariantProbeCases() {
		variant := variant
		t.Run(variant.name, func(t *testing.T) {
			operators := rewriteFaithfulSyntheticTextOperatorsForVariantProbe(source.keptOps, variant)
			require.NotEmpty(t, operators)

			pdfBytes := buildFaithfulSyntheticTextCompositionProbePDFWithOperators(t, source, operators)
			pdfName := fmt.Sprintf("%s_%s.pdf", source.name, variant.name)
			popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pdfName, pdfBytes, "", tc.dpi)
			placement := measurePNGPlacementProbeForProbe(t, popplerPNG, oursPNG, 4)

			t.Logf(
				"variant=%s ops=%d exact=%.4f similarity=%.4f best_shift_similarity=%.4f best_shift=(%d,%d)",
				variant.name,
				len(operators),
				placement.originalExact,
				placement.originalSimilarity,
				placement.bestShiftSimilarity,
				placement.bestShiftX,
				placement.bestShiftY,
			)
		})
	}
}

func TestFaithfulSyntheticTextCompositionFormGraphVariantProbeAgainstPoppler(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	for _, variant := range faithfulSyntheticTextFormGraphVariantProbeCases() {
		variant := variant
		t.Run(variant.name, func(t *testing.T) {
			pdfBytes := buildFaithfulSyntheticTextCompositionProbePDFWithImportedMutator(
				t,
				source,
				source.rawContent,
				source.keptOps,
				variant.mutate,
			)
			pdfName := fmt.Sprintf("%s_%s.pdf", source.name, variant.name)
			popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pdfName, pdfBytes, "", tc.dpi)
			placement := measurePNGPlacementProbeForProbe(t, popplerPNG, oursPNG, 4)

			t.Logf(
				"variant=%s exact=%.4f similarity=%.4f best_shift_similarity=%.4f best_shift=(%d,%d)",
				variant.name,
				placement.originalExact,
				placement.originalSimilarity,
				placement.bestShiftSimilarity,
				placement.bestShiftX,
				placement.bestShiftY,
			)
		})
	}
}

func TestFaithfulSyntheticTextCompositionDebugGateProbeAgainstPoppler(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	pdfBytes := buildFaithfulSyntheticTextCompositionProbePDF(t, source)
	variants := []struct {
		name string
		env  map[string]string
	}{
		{name: "baseline"},
		{name: "skip_fill_paths", env: map[string]string{"PDF_DEBUG_SKIP_FILL_PATHS": "1"}},
		{name: "skip_stroke_paths", env: map[string]string{"PDF_DEBUG_SKIP_STROKE_PATHS": "1"}},
		{name: "skip_xobjects", env: map[string]string{"PDF_DEBUG_SKIP_XOBJECTS": "1"}},
	}

	for _, variant := range variants {
		variant := variant
		t.Run(variant.name, func(t *testing.T) {
			restore := setProbeEnvForRender(t, variant.env)
			defer restore()

			pdfName := fmt.Sprintf("%s_%s.pdf", source.name, variant.name)
			popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pdfName, pdfBytes, "", tc.dpi)
			placement := measurePNGPlacementProbeForProbe(t, popplerPNG, oursPNG, 4)

			t.Logf(
				"variant=%s exact=%.4f similarity=%.4f best_shift_similarity=%.4f best_shift=(%d,%d)",
				variant.name,
				placement.originalExact,
				placement.originalSimilarity,
				placement.bestShiftSimilarity,
				placement.bestShiftX,
				placement.bestShiftY,
			)
		})
	}
}

func TestFaithfulSyntheticTextCompositionIm17FocusedDiffProbeAgainstPoppler(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	invocations := collectFaithfulSyntheticTopLevelXObjectInvocationsForProbe(t, source, "Im17", tc.dpi)
	require.NotEmpty(t, invocations)

	focusedBounds := unionFaithfulSyntheticPixelBoundsForProbe(invocations)
	require.False(t, focusedBounds.Empty())

	pdfBytes := buildFaithfulSyntheticTextCompositionProbePDF(t, source)
	pdfName := fmt.Sprintf("%s_im17_focus.pdf", source.name)
	popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pdfName, pdfBytes, "", tc.dpi)

	popplerImg := loadPNGAsRGBAForProbe(t, popplerPNG)
	oursImg := loadPNGAsRGBAForProbe(t, oursPNG)
	popplerCrop := cropRGBAForProbe(popplerImg, focusedBounds)
	oursCrop := cropRGBAForProbe(oursImg, focusedBounds)

	root := t.TempDir()
	popplerCropPath := filepath.Join(root, "poppler-im17.png")
	oursCropPath := filepath.Join(root, "ours-im17.png")
	require.NoError(t, parityWritePNG(popplerCropPath, popplerCrop))
	require.NoError(t, parityWritePNG(oursCropPath, oursCrop))

	placement := measurePNGPlacementProbeForProbe(t, popplerCropPath, oursCropPath, 4)
	rowHotspots, colHotspots := collectSyntheticFontDiffHotspotsForProbe(t, popplerCropPath, oursCropPath, 5)

	t.Logf(
		"resource=%s invocations=%d bounds=%v exact=%.4f similarity=%.4f best_shift_similarity=%.4f best_shift=(%d,%d) rows=%s cols=%s",
		invocations[0].resourceName,
		len(invocations),
		focusedBounds,
		placement.originalExact,
		placement.originalSimilarity,
		placement.bestShiftSimilarity,
		placement.bestShiftX,
		placement.bestShiftY,
		formatSyntheticFontDiffHotspotsForProbe(rowHotspots),
		formatSyntheticFontDiffHotspotsForProbe(colHotspots),
	)
}

func TestFaithfulSyntheticTextCompositionIm17ChildFormDiffProbeAgainstPoppler(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	results := collectFaithfulSyntheticIm17ChildFormDiffsForProbe(t, source, tc.dpi)
	require.NotEmpty(t, results)

	limit := 10
	if len(results) < limit {
		limit = len(results)
	}
	formatted := make([]string, 0, limit)
	for _, result := range results[:limit] {
		formatted = append(formatted, fmt.Sprintf(
			"%s#%d bounds=%v exact=%.4f similarity=%.4f mismatch_pixels=%d mismatch_bounds=%v deltas=%s nonzero_deltas=%s",
			result.resourceName,
			result.occurrence,
			result.pixelBounds,
			result.exact,
			result.similarity,
			result.mismatchPixels,
			result.mismatchBounds,
			result.deltas,
			result.nonzeroDeltas,
		))
	}

	t.Logf("im17_child_forms=%d worst=%s", len(results), strings.Join(formatted, " | "))
}

func TestFaithfulSyntheticTextCompositionFm184PixelSampleProbeAgainstPoppler(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	pdfBytes := buildFaithfulSyntheticTextCompositionProbePDF(t, source)
	pdfName := fmt.Sprintf("%s_fm184_samples.pdf", source.name)
	popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pdfName, pdfBytes, "", tc.dpi)
	popplerImg := loadPNGAsRGBAForProbe(t, popplerPNG)
	oursImg := loadPNGAsRGBAForProbe(t, oursPNG)

	targets := map[string]image.Rectangle{
		"Fm126": image.Rect(773, 351, 807, 359),
		"Fm155": image.Rect(795, 350, 829, 361),
		"Fm228": image.Rect(867, 272, 896, 305),
		"Fm231": image.Rect(865, 260, 895, 295),
		"Fm184": image.Rect(500, 184, 518, 246),
	}

	for _, targetName := range []string{"Fm126", "Fm155", "Fm228", "Fm231", "Fm184"} {
		bounds := targets[targetName]
		deltas, mismatchPixels, mismatchBounds := topMaskedNonzeroRGBDeltasForProbe(popplerImg, oursImg, bounds, nil, 6)
		t.Logf(
			"target=%s bounds=%v mismatch_pixels=%d mismatch_bounds=%v deltas=%s samples=%s",
			targetName,
			bounds,
			mismatchPixels,
			mismatchBounds,
			deltas,
			sampleMaskedMismatchedRGBForProbe(popplerImg, oursImg, bounds, nil, 6),
		)
	}
}

func TestFaithfulSyntheticTextCompositionFm184PatternPlacementProbe(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	im17Invocations := collectFaithfulSyntheticTopLevelXObjectInvocationsForProbe(t, source, "Im17", tc.dpi)
	require.NotEmpty(t, im17Invocations)

	im17Stream := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), source.resources, "Im17")
	require.NotNil(t, im17Stream)

	im17ResourcesObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), im17Stream.Dict().Get(entity.Name("Resources")))
	im17Resources, ok := im17ResourcesObj.(*entity.Dict)
	require.True(t, ok)

	decodedIm17, err := stream.NewFromEntity(im17Stream).Decode()
	require.NoError(t, err)
	im17Operators := parseFaithfulSyntheticRawOperatorsForProbe(t, decodedIm17)
	childInvocations := collectFaithfulSyntheticFormXObjectInvocationsForProbe(
		t,
		source.doc.XRef(),
		im17Resources,
		im17Operators,
		source.mediaBox,
		tc.dpi,
		im17Invocations[0].effectiveCTM,
	)

	for _, targetName := range []string{"Fm126", "Fm155", "Fm228", "Fm231", "Fm184"} {
		t.Run(targetName, func(t *testing.T) {
			var invocationProbe faithfulSyntheticTextXObjectInvocationProbe
			found := false
			for _, invocation := range childInvocations {
				if invocation.resourceName == targetName {
					invocationProbe = invocation
					found = true
					break
				}
			}
			require.True(t, found)

			form := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), im17Resources, targetName)
			require.NotNil(t, form)

			formResourcesObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), form.Dict().Get(entity.Name("Resources")))
			formResources, ok := formResourcesObj.(*entity.Dict)
			require.True(t, ok)

			decodedForm, err := stream.NewFromEntity(form).Decode()
			require.NoError(t, err)
			formOperators := parseFaithfulSyntheticRawOperatorsForProbe(t, decodedForm)

			fillCTM := invocationProbe.effectiveCTM
			stack := make([][6]float64, 0, 8)
			patternName := ""
			for _, op := range formOperators {
				switch op.Opcode {
				case "q":
					stack = append(stack, fillCTM)
				case "Q":
					if len(stack) == 0 {
						continue
					}
					fillCTM = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				case "cm":
					matrix, ok := faithfulSyntheticMatrixOperandsForProbe(op.Operands)
					require.True(t, ok)
					fillCTM = faithfulSyntheticMultiplyMatrixForProbe(fillCTM, matrix)
				case "scn":
					require.NotEmpty(t, op.Operands)
					patternName = syntheticNameValueForProbe(op.Operands[len(op.Operands)-1])
				case "f":
					goto ready
				}
			}

		ready:
			require.NotEmpty(t, patternName)

			patterns, ok := resolveSyntheticFontDictForProbe(source.doc.XRef(), formResources.Get(entity.Name("Pattern")))
			require.True(t, ok)
			patternObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), patterns.Get(entity.Name(patternName)))
			patternDict, ok := patternObj.(*entity.Dict)
			require.True(t, ok)
			patternMatrix, ok := faithfulSyntheticMatrixArrayForProbe(patternDict.Get(entity.Name("Matrix")))
			require.True(t, ok)

			effectivePatternMatrix := faithfulSyntheticMultiplyMatrixForProbe(fillCTM, patternMatrix)

			t.Logf(
				"target=%s pixel_bounds=%v form_effective_ctm=%v fill_ctm=%v pattern_name=%s pattern_matrix=%v effective_pattern_matrix=%v",
				targetName,
				invocationProbe.pixelBounds,
				invocationProbe.effectiveCTM,
				fillCTM,
				patternName,
				patternMatrix,
				effectivePatternMatrix,
			)
		})
	}
}

func TestFaithfulSyntheticTextCompositionFm184StandaloneRenderProbe(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	im17Invocations := collectFaithfulSyntheticTopLevelXObjectInvocationsForProbe(t, source, "Im17", tc.dpi)
	require.NotEmpty(t, im17Invocations)

	im17Stream := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), source.resources, "Im17")
	require.NotNil(t, im17Stream)

	im17ResourcesObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), im17Stream.Dict().Get(entity.Name("Resources")))
	im17Resources, ok := im17ResourcesObj.(*entity.Dict)
	require.True(t, ok)

	decodedIm17, err := stream.NewFromEntity(im17Stream).Decode()
	require.NoError(t, err)
	im17Operators := parseFaithfulSyntheticRawOperatorsForProbe(t, decodedIm17)
	childInvocations := collectFaithfulSyntheticFormXObjectInvocationsForProbe(
		t,
		source.doc.XRef(),
		im17Resources,
		im17Operators,
		source.mediaBox,
		tc.dpi,
		im17Invocations[0].effectiveCTM,
	)

	scale := float64(tc.dpi) / 72.0
	width := int(math.Ceil((source.mediaBox[2] - source.mediaBox[0]) * scale))
	height := int(math.Ceil((source.mediaBox[3] - source.mediaBox[1]) * scale))
	initial := [6]float64{
		scale,
		0,
		0,
		scale,
		-source.mediaBox[0] * scale,
		0,
	}

	for _, targetName := range []string{"Fm126", "Fm155", "Fm228", "Fm231", "Fm184"} {
		t.Run(targetName, func(t *testing.T) {
			var invocationProbe faithfulSyntheticTextXObjectInvocationProbe
			found := false
			for _, invocation := range childInvocations {
				if invocation.resourceName == targetName {
					invocationProbe = invocation
					found = true
					break
				}
			}
			require.True(t, found)

			form := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), im17Resources, targetName)
			require.NotNil(t, form)

			formResourcesObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), form.Dict().Get(entity.Name("Resources")))
			formResources, ok := formResourcesObj.(*entity.Dict)
			require.True(t, ok)
			decodedForm, err := stream.NewFromEntity(form).Decode()
			require.NoError(t, err)
			formOperators := parseFaithfulSyntheticRawOperatorsForProbe(t, decodedForm)

			targetInitial := faithfulSyntheticMultiplyMatrixForProbe(initial, invocationProbe.effectiveCTM)
			recordingCanvas := &fillPatternRecordingCanvas{
				ImageCanvas: infracanvas.NewImageCanvas(image.Rect(0, 0, width, height)).(*infracanvas.ImageCanvas),
			}

			renderStandalone := func(contents []entity.Object) (image.Rectangle, int) {
				recordingCanvas.ImageCanvas.Reset()
				recordingCanvas.lastFillPattern = nil
				recordingCanvas.fillPatternHistory = nil
				recordingCanvas.pathMoveHistory = nil

				evaluator := domainrenderer.NewEvaluator(source.doc.XRef())
				evaluator.SetCanvas(recordingCanvas)
				evaluator.SetResources(formResources)
				evaluator.SetInitialTransform(targetInitial)
				require.NoError(t, evaluator.Evaluate(contents))

				img, ok := recordingCanvas.Image().(*image.RGBA)
				require.True(t, ok)
				return nonTransparentBoundsForProbe(img)
			}

			paintedBounds, paintedPixels := renderStandalone([]entity.Object{form})
			shadingPattern, ok := recordingCanvas.lastFillPattern.(*entity.ShadingPattern)
			require.True(t, ok)
			originalPatternHistory := append([]string(nil), recordingCanvas.fillPatternHistory...)
			originalPathMoves := append([]string(nil), recordingCanvas.pathMoveHistory...)

			solidOps := make([]domainrenderer.Operator, 0, len(formOperators))
			for _, op := range formOperators {
				if op.Opcode == "cs" || op.Opcode == "scn" {
					continue
				}
				solidOps = append(solidOps, op)
			}
			var solidContent strings.Builder
			for _, op := range solidOps {
				solidContent.WriteString(serializeFaithfulSyntheticTextOperatorForProbe(op))
			}
			solidBounds, solidPixels := renderStandalone([]entity.Object{
				entity.NewStream(entity.NewDict(), []byte(solidContent.String())),
			})

			directCanvas := infracanvas.NewImageCanvas(image.Rect(0, 0, width, height)).(*infracanvas.ImageCanvas)
			require.NoError(t, directCanvas.DrawShadingPattern(shadingPattern, [4]float64{
				float64(invocationProbe.pixelBounds.Min.X),
				float64(invocationProbe.pixelBounds.Min.Y),
				float64(invocationProbe.pixelBounds.Max.X),
				float64(invocationProbe.pixelBounds.Max.Y),
			}))
			directPatternBounds, directPatternPixels := nonTransparentBoundsForProbe(directCanvas.Image().(*image.RGBA))

			t.Logf(
				"target=%s expected_bounds=%v painted_bounds=%v painted_pixels=%d solid_bounds=%v solid_pixels=%d direct_pattern_bounds=%v direct_pattern_pixels=%d pattern_matrix=%v pattern_history=%s path_moves=%s",
				targetName,
				invocationProbe.pixelBounds,
				paintedBounds,
				paintedPixels,
				solidBounds,
				solidPixels,
				directPatternBounds,
				directPatternPixels,
				shadingPattern.Matrix(),
				strings.Join(originalPatternHistory, " | "),
				strings.Join(originalPathMoves, " | "),
			)
		})
	}
}

func TestFaithfulSyntheticTextCompositionIm17OperatorResourceProbe(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	im17 := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), source.resources, "Im17")
	require.NotNil(t, im17)

	decoded, err := stream.NewFromEntity(im17).Decode()
	if err != nil {
		decoded = append([]byte(nil), im17.RawBytes()...)
	}
	operators := parseFaithfulSyntheticRawOperatorsForProbe(t, decoded)
	hist := measureFaithfulSyntheticTextOperatorHistogramForProbe(operators)

	var resources *entity.Dict
	if resourcesObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), im17.Dict().Get(entity.Name("Resources"))); resourcesObj != nil {
		if dict, ok := resourcesObj.(*entity.Dict); ok {
			resources = dict
		}
	}

	t.Logf(
		"im17_ops=%d hist=%s patterns=%s xobjects=%s extgstates=%s",
		len(operators),
		formatFaithfulSyntheticTextOperatorHistogramForProbe(hist),
		collectFaithfulSyntheticPatternSummaryForProbe(t, source.doc.XRef(), resources),
		collectFaithfulSyntheticXObjectSummaryForProbe(t, source.doc.XRef(), resources),
		collectFaithfulSyntheticNamedResourceSummaryForProbe(t, source.doc.XRef(), resources, entity.Name("ExtGState")),
	)
}

func TestFaithfulSyntheticTextCompositionIm17WorstChildOperatorProbe(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	results := collectFaithfulSyntheticIm17ChildFormDiffsForProbe(t, source, tc.dpi)
	require.NotEmpty(t, results)

	seenTargets := make(map[string]struct{})
	targets := make([]string, 0, 10)
	for _, result := range results {
		if _, exists := seenTargets[result.resourceName]; exists {
			continue
		}
		seenTargets[result.resourceName] = struct{}{}
		targets = append(targets, result.resourceName)
		if len(targets) == 10 {
			break
		}
	}
	require.NotEmpty(t, targets)

	for _, target := range targets {
		target := target
		t.Run(target, func(t *testing.T) {
			streamObj := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), source.resources, "Im17")
			require.NotNil(t, streamObj)

			im17ResourcesObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), streamObj.Dict().Get(entity.Name("Resources")))
			im17Resources, ok := im17ResourcesObj.(*entity.Dict)
			require.True(t, ok)

			child := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), im17Resources, target)
			require.NotNil(t, child)

			decoded, err := stream.NewFromEntity(child).Decode()
			if err != nil {
				decoded = append([]byte(nil), child.RawBytes()...)
			}
			operators := parseFaithfulSyntheticRawOperatorsForProbe(t, decoded)
			hist := measureFaithfulSyntheticTextOperatorHistogramForProbe(operators)
			sequence := formatFaithfulSyntheticOperatorSequenceForProbe(operators)

			var childResources *entity.Dict
			if resourcesObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), child.Dict().Get(entity.Name("Resources"))); resourcesObj != nil {
				if dict, ok := resourcesObj.(*entity.Dict); ok {
					childResources = dict
				}
			}

			bbox, _ := faithfulSyntheticBBoxArrayForProbe(child.Dict().Get(entity.Name("BBox")))
			matrix, hasMatrix := faithfulSyntheticMatrixArrayForProbe(child.Dict().Get(entity.Name("Matrix")))
			patternDetails := "-"
			if childResources != nil {
				if patternDict, ok := resolveSyntheticFontDictForProbe(source.doc.XRef(), childResources.Get(entity.Name("Pattern"))); ok && patternDict != nil {
					for _, key := range sortedSyntheticFontNamesForProbe(patternDict.Keys()) {
						rawPattern := patternDict.GetRaw(key)
						resolvedPattern := resolveSyntheticFontObjectForProbe(source.doc.XRef(), rawPattern)
						refLabel := ""
						if ref, ok := rawPattern.(entity.Ref); ok {
							refLabel = fmt.Sprintf("@%d_%dR", ref.Num(), ref.Gen())
						}
						switch typed := resolvedPattern.(type) {
						case *entity.Stream:
							patternDetails = fmt.Sprintf(
								"%s%s:stream keys=%v raw=%d",
								strings.TrimPrefix(key.Value(), "/"),
								refLabel,
								sortedSyntheticPatternDictKeysForProbe(typed.Dict()),
								len(typed.RawBytes()),
							)
						case *entity.Dict:
							shadingType := "-"
							shadingKeys := "-"
							bitsFlag := "-"
							bitsCoord := "-"
							bitsComp := "-"
							decodeLen := 0
							hasFunction := false
							functionDetails := "-"
							patternMatrix := "-"
							if shadingObj := typed.Get(entity.Name("Shading")); shadingObj != nil {
								if shadingResolved := resolveSyntheticFontObjectForProbe(source.doc.XRef(), shadingObj); shadingResolved != nil {
									switch shadingTyped := shadingResolved.(type) {
									case *entity.Dict:
										shadingType = syntheticIntStringForProbe(shadingTyped.Get(entity.Name("ShadingType")))
										shadingKeys = fmt.Sprintf("%v", sortedSyntheticPatternDictKeysForProbe(shadingTyped))
										bitsFlag = syntheticIntStringForProbe(shadingTyped.Get(entity.Name("BitsPerFlag")))
										bitsCoord = syntheticIntStringForProbe(shadingTyped.Get(entity.Name("BitsPerCoordinate")))
										bitsComp = syntheticIntStringForProbe(shadingTyped.Get(entity.Name("BitsPerComponent")))
										if decodeObj, ok := shadingTyped.Get(entity.Name("Decode")).(*entity.Array); ok {
											decodeLen = decodeObj.Len()
										}
										hasFunction = shadingTyped.Get(entity.Name("Function")) != nil
										functionDetails = describeFaithfulSyntheticShadingFunctionForProbe(t, source.doc.XRef(), shadingTyped)
									case *entity.Stream:
										shadingType = syntheticIntStringForProbe(shadingTyped.Dict().Get(entity.Name("ShadingType")))
										shadingKeys = fmt.Sprintf("%v", sortedSyntheticPatternDictKeysForProbe(shadingTyped.Dict()))
										bitsFlag = syntheticIntStringForProbe(shadingTyped.Dict().Get(entity.Name("BitsPerFlag")))
										bitsCoord = syntheticIntStringForProbe(shadingTyped.Dict().Get(entity.Name("BitsPerCoordinate")))
										bitsComp = syntheticIntStringForProbe(shadingTyped.Dict().Get(entity.Name("BitsPerComponent")))
										if decodeObj, ok := shadingTyped.Dict().Get(entity.Name("Decode")).(*entity.Array); ok {
											decodeLen = decodeObj.Len()
										}
										hasFunction = shadingTyped.Dict().Get(entity.Name("Function")) != nil
										functionDetails = describeFaithfulSyntheticShadingFunctionForProbe(t, source.doc.XRef(), shadingTyped.Dict())
									}
								}
							}
							if matrixObj, ok := typed.Get(entity.Name("Matrix")).(*entity.Array); ok {
								values := make([]string, 0, matrixObj.Len())
								for i := 0; i < matrixObj.Len(); i++ {
									values = append(values, syntheticFloatStringForProbe(matrixObj.Get(i)))
								}
								patternMatrix = strings.Join(values, ",")
							}
							patternDetails = fmt.Sprintf(
								"%s%s:dict keys=%v matrix=[%s] shading_type=%s shading_keys=%s bits=(%s,%s,%s) decode_len=%d has_function=%t function=%s",
								strings.TrimPrefix(key.Value(), "/"),
								refLabel,
								sortedSyntheticPatternDictKeysForProbe(typed),
								patternMatrix,
								shadingType,
								shadingKeys,
								bitsFlag,
								bitsCoord,
								bitsComp,
								decodeLen,
								hasFunction,
								functionDetails,
							)
						default:
							patternDetails = fmt.Sprintf("%s%s:%T", strings.TrimPrefix(key.Value(), "/"), refLabel, resolvedPattern)
						}
						break
					}
				}
			}
			t.Logf(
				"target=%s bbox=%v has_matrix=%t matrix=%v ops=%d hist=%s sequence=%s patterns=%s pattern_details=%s xobjects=%s extgstates=%s",
				target,
				bbox,
				hasMatrix,
				matrix,
				len(operators),
				formatFaithfulSyntheticTextOperatorHistogramForProbe(hist),
				sequence,
				collectFaithfulSyntheticPatternSummaryForProbe(t, source.doc.XRef(), childResources),
				patternDetails,
				collectFaithfulSyntheticXObjectSummaryForProbe(t, source.doc.XRef(), childResources),
				collectFaithfulSyntheticNamedResourceSummaryForProbe(t, source.doc.XRef(), childResources, entity.Name("ExtGState")),
			)
		})
	}
}

func TestFaithfulSyntheticTextCompositionIm17WorstChildBoundaryBandSplitProbeAgainstPoppler(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	results := collectFaithfulSyntheticIm17ChildFormDiffsForProbe(t, source, tc.dpi)
	require.NotEmpty(t, results)

	pdfBytes := buildFaithfulSyntheticTextCompositionProbePDF(t, source)
	pdfName := fmt.Sprintf("%s_im17_boundary_split.pdf", source.name)
	popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pdfName, pdfBytes, "", tc.dpi)
	popplerImg := loadPNGAsRGBAForProbe(t, popplerPNG)
	oursImg := loadPNGAsRGBAForProbe(t, oursPNG)

	seenTargets := make(map[string]struct{})
	targetCount := 0
	for _, result := range results {
		if _, exists := seenTargets[result.resourceName]; exists {
			continue
		}
		seenTargets[result.resourceName] = struct{}{}
		targetCount++

		t.Run(result.resourceName, func(t *testing.T) {
			bounds := result.pixelBounds.Intersect(popplerImg.Bounds()).Intersect(oursImg.Bounds())
			require.False(t, bounds.Empty())

			metrics := measureFaithfulSyntheticBoundaryBandMetricsForProbe(popplerImg, oursImg, bounds)
			require.Greater(t, metrics.support.pixels, 0)

			t.Logf(
				"target=%s bounds=%v crop_exact=%.4f crop_similarity=%.4f support_pixels=%d support_exact=%.4f support_similarity=%.4f band_radius=%d core_pixels=%d core_exact=%.4f core_similarity=%.4f boundary_pixels=%d boundary_exact=%.4f boundary_similarity=%.4f boundary_similarity_gap=%.4f",
				result.resourceName,
				bounds,
				result.exact,
				result.similarity,
				metrics.support.pixels,
				metrics.support.exact,
				metrics.support.similarity,
				metrics.bandRadius,
				metrics.core.pixels,
				metrics.core.exact,
				metrics.core.similarity,
				metrics.boundary.pixels,
				metrics.boundary.exact,
				metrics.boundary.similarity,
				metrics.core.similarity-metrics.boundary.similarity,
			)
		})

		if targetCount == 6 {
			break
		}
	}
	require.Greater(t, targetCount, 0)
}

func TestFaithfulSyntheticTextCompositionIm17WorstChildStandaloneBoundaryBandSplitProbeAgainstPoppler(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	results := collectFaithfulSyntheticIm17ChildFormDiffsForProbe(t, source, tc.dpi)
	require.NotEmpty(t, results)

	pagePDFBytes := buildFaithfulSyntheticTextCompositionProbePDF(t, source)
	pagePDFName := fmt.Sprintf("%s_im17_standalone_baseline.pdf", source.name)
	pagePopplerPNG, pageOursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pagePDFName, pagePDFBytes, "", tc.dpi)
	pagePopplerImg := loadPNGAsRGBAForProbe(t, pagePopplerPNG)
	pageOursImg := loadPNGAsRGBAForProbe(t, pageOursPNG)

	resultsByName := make(map[string]faithfulSyntheticTextXObjectDiffProbe, len(results))
	for _, result := range results {
		if _, exists := resultsByName[result.resourceName]; exists {
			continue
		}
		resultsByName[result.resourceName] = result
	}

	im17Invocations := collectFaithfulSyntheticTopLevelXObjectInvocationsForProbe(t, source, "Im17", tc.dpi)
	require.NotEmpty(t, im17Invocations)

	im17 := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), source.resources, "Im17")
	require.NotNil(t, im17)

	im17ResourcesObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), im17.Dict().Get(entity.Name("Resources")))
	im17Resources, ok := im17ResourcesObj.(*entity.Dict)
	require.True(t, ok)

	decodedIm17, err := stream.NewFromEntity(im17).Decode()
	require.NoError(t, err)
	im17Operators := parseFaithfulSyntheticRawOperatorsForProbe(t, decodedIm17)
	childInvocations := collectFaithfulSyntheticFormXObjectInvocationsForProbe(
		t,
		source.doc.XRef(),
		im17Resources,
		im17Operators,
		source.mediaBox,
		tc.dpi,
		im17Invocations[0].effectiveCTM,
	)
	require.NotEmpty(t, childInvocations)

	targets := faithfulSyntheticIm17OverlapProbeTargets()
	for _, target := range targets {
		target := target
		t.Run(target, func(t *testing.T) {
			result, exists := resultsByName[target]
			require.True(t, exists)

			child := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), im17Resources, target)
			require.NotNil(t, child)

			childMatrix := faithfulSyntheticFormMatrixForProbe(child)
			inverseChildMatrix, ok := faithfulSyntheticInverseMatrixForProbe(childMatrix)
			require.True(t, ok)

			invocation, ok := faithfulSyntheticXObjectInvocationForOccurrenceForProbe(childInvocations, target, result.occurrence)
			require.True(t, ok)

			childCurrentCTM := faithfulSyntheticMultiplyMatrixForProbe(invocation.effectiveCTM, inverseChildMatrix)
			standalonePDFBytes := buildFaithfulSyntheticStandaloneXObjectProbePDF(
				t,
				source,
				"StandaloneChild",
				child,
				childCurrentCTM,
			)

			standalonePDFName := fmt.Sprintf("%s_%s_im17_standalone_boundary.pdf", source.name, strings.ToLower(target))
			standalonePopplerPNG, standaloneOursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, standalonePDFName, standalonePDFBytes, "", tc.dpi)
			standalonePopplerImg := loadPNGAsRGBAForProbe(t, standalonePopplerPNG)
			standaloneOursImg := loadPNGAsRGBAForProbe(t, standaloneOursPNG)

			pageMetrics := measureFaithfulSyntheticBoundaryBandMetricsForProbe(pagePopplerImg, pageOursImg, result.pixelBounds)
			standaloneMetrics := measureFaithfulSyntheticBoundaryBandMetricsForProbe(standalonePopplerImg, standaloneOursImg, result.pixelBounds)
			require.Greater(t, pageMetrics.support.pixels, 0)
			standalonePopplerBounds := nonWhiteBoundsForProbe(standalonePopplerImg)
			standaloneOursBounds := nonWhiteBoundsForProbe(standaloneOursImg)

			if standaloneMetrics.support.pixels == 0 {
				t.Logf(
					"target=%s occurrence=%d bounds=%v page_support_similarity=%.4f page_core_similarity=%.4f page_boundary_similarity=%.4f standalone_support_pixels=0 standalone_poppler_nonwhite_bounds=%v standalone_ours_nonwhite_bounds=%v",
					target,
					result.occurrence,
					result.pixelBounds,
					pageMetrics.support.similarity,
					pageMetrics.core.similarity,
					pageMetrics.boundary.similarity,
					standalonePopplerBounds,
					standaloneOursBounds,
				)
				return
			}

			t.Logf(
				"target=%s occurrence=%d bounds=%v page_support_similarity=%.4f page_core_similarity=%.4f page_boundary_similarity=%.4f standalone_support_similarity=%.4f standalone_core_similarity=%.4f standalone_boundary_similarity=%.4f standalone_boundary_gain=%.4f standalone_core_gain=%.4f standalone_poppler_nonwhite_bounds=%v standalone_ours_nonwhite_bounds=%v",
				target,
				result.occurrence,
				result.pixelBounds,
				pageMetrics.support.similarity,
				pageMetrics.core.similarity,
				pageMetrics.boundary.similarity,
				standaloneMetrics.support.similarity,
				standaloneMetrics.core.similarity,
				standaloneMetrics.boundary.similarity,
				standaloneMetrics.boundary.similarity-pageMetrics.boundary.similarity,
				standaloneMetrics.core.similarity-pageMetrics.core.similarity,
				standalonePopplerBounds,
				standaloneOursBounds,
			)
		})
	}
}

func TestFaithfulSyntheticTextCompositionIm17ParentContextReplayProbeAgainstPoppler(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	results := collectFaithfulSyntheticIm17ChildFormDiffsForProbe(t, source, tc.dpi)
	require.NotEmpty(t, results)

	pagePDFBytes := buildFaithfulSyntheticTextCompositionProbePDF(t, source)
	pagePDFName := fmt.Sprintf("%s_im17_parent_context_baseline.pdf", source.name)
	pagePopplerPNG, pageOursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pagePDFName, pagePDFBytes, "", tc.dpi)
	pagePopplerImg := loadPNGAsRGBAForProbe(t, pagePopplerPNG)
	pageOursImg := loadPNGAsRGBAForProbe(t, pageOursPNG)

	resultsByName := make(map[string]faithfulSyntheticTextXObjectDiffProbe, len(results))
	for _, result := range results {
		if _, exists := resultsByName[result.resourceName]; exists {
			continue
		}
		resultsByName[result.resourceName] = result
	}

	im17Invocations := collectFaithfulSyntheticTopLevelXObjectInvocationsForProbe(t, source, "Im17", tc.dpi)
	require.NotEmpty(t, im17Invocations)

	im17 := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), source.resources, "Im17")
	require.NotNil(t, im17)

	im17ResourcesObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), im17.Dict().Get(entity.Name("Resources")))
	im17Resources, ok := im17ResourcesObj.(*entity.Dict)
	require.True(t, ok)

	decodedIm17, err := stream.NewFromEntity(im17).Decode()
	if err != nil {
		decodedIm17 = append([]byte(nil), im17.RawBytes()...)
	}
	im17Operators := parseFaithfulSyntheticRawOperatorsForProbe(t, decodedIm17)
	require.NotEmpty(t, im17Operators)

	childInvocations := collectFaithfulSyntheticFormXObjectInvocationsForProbe(
		t,
		source.doc.XRef(),
		im17Resources,
		im17Operators,
		source.mediaBox,
		tc.dpi,
		im17Invocations[0].effectiveCTM,
	)
	require.NotEmpty(t, childInvocations)

	targets := faithfulSyntheticIm17OverlapProbeTargets()
	for _, target := range targets {
		target := target
		t.Run(target, func(t *testing.T) {
			result, exists := resultsByName[target]
			require.True(t, exists)

			invocation, ok := faithfulSyntheticXObjectInvocationForOccurrenceForProbe(childInvocations, target, result.occurrence)
			require.True(t, ok)

			targetIndex, ok := faithfulSyntheticXObjectOperatorIndexForOccurrenceForProbe(im17Operators, target, result.occurrence)
			require.True(t, ok)

			child := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), im17Resources, target)
			require.NotNil(t, child)

			childMatrix := faithfulSyntheticFormMatrixForProbe(child)
			inverseChildMatrix, ok := faithfulSyntheticInverseMatrixForProbe(childMatrix)
			require.True(t, ok)
			childCurrentCTM := faithfulSyntheticMultiplyMatrixForProbe(invocation.effectiveCTM, inverseChildMatrix)

			scopeStart := faithfulSyntheticEnclosingScopeStartForProbe(im17Operators, targetIndex)
			windowStart := targetIndex - 40
			if windowStart < 0 {
				windowStart = 0
			}
			initialIm17CTM := im17Invocations[0].effectiveCTM
			scopeStartCTM := faithfulSyntheticCTMBeforeOperatorForProbe(im17Operators, initialIm17CTM, scopeStart)
			windowStartCTM := faithfulSyntheticCTMBeforeOperatorForProbe(im17Operators, initialIm17CTM, windowStart)

			variants := []faithfulSyntheticParentContextReplayVariantProbe{
				{
					name:       "do_only_current",
					content:    buildFaithfulSyntheticSingleDoContentForProbe(target, childCurrentCTM),
					startIndex: targetIndex,
					endIndex:   targetIndex,
				},
				{
					name: "scope_prefix",
					content: buildFaithfulSyntheticParentContextReplayContentForProbe(
						scopeStartCTM,
						im17Operators,
						scopeStart,
						targetIndex,
					),
					startIndex: scopeStart,
					endIndex:   targetIndex,
				},
				{
					name: "window_40",
					content: buildFaithfulSyntheticParentContextReplayContentForProbe(
						windowStartCTM,
						im17Operators,
						windowStart,
						targetIndex,
					),
					startIndex: windowStart,
					endIndex:   targetIndex,
				},
				{
					name: "full_prefix",
					content: buildFaithfulSyntheticParentContextReplayContentForProbe(
						initialIm17CTM,
						im17Operators,
						0,
						targetIndex,
					),
					startIndex: 0,
					endIndex:   targetIndex,
				},
			}

			pageMetrics := measureFaithfulSyntheticBoundaryBandMetricsForProbe(pagePopplerImg, pageOursImg, result.pixelBounds)
			require.Greater(t, pageMetrics.support.pixels, 0)

			for _, variant := range variants {
				variant := variant
				t.Run(variant.name, func(t *testing.T) {
					pdfBytes := buildFaithfulSyntheticResourceContentProbePDF(t, source, im17Resources, variant.content)
					pdfName := fmt.Sprintf(
						"%s_%s_%s_im17_parent_context.pdf",
						source.name,
						strings.ToLower(target),
						variant.name,
					)
					popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pdfName, pdfBytes, "", tc.dpi)
					popplerImg := loadPNGAsRGBAForProbe(t, popplerPNG)
					oursImg := loadPNGAsRGBAForProbe(t, oursPNG)
					metrics := measureFaithfulSyntheticBoundaryBandMetricsForProbe(popplerImg, oursImg, result.pixelBounds)
					popplerBounds := nonWhiteBoundsForProbe(popplerImg)
					oursBounds := nonWhiteBoundsForProbe(oursImg)

					t.Logf(
						"target=%s occurrence=%d variant=%s op_range=%d..%d target_bounds=%v page_support_similarity=%.4f page_core_similarity=%.4f page_boundary_similarity=%.4f replay_support_pixels=%d replay_support_similarity=%.4f replay_core_similarity=%.4f replay_boundary_similarity=%.4f replay_boundary_gain=%.4f replay_poppler_nonwhite_bounds=%v replay_ours_nonwhite_bounds=%v",
						target,
						result.occurrence,
						variant.name,
						variant.startIndex,
						variant.endIndex,
						result.pixelBounds,
						pageMetrics.support.similarity,
						pageMetrics.core.similarity,
						pageMetrics.boundary.similarity,
						metrics.support.pixels,
						metrics.support.similarity,
						metrics.core.similarity,
						metrics.boundary.similarity,
						metrics.boundary.similarity-pageMetrics.boundary.similarity,
						popplerBounds,
						oursBounds,
					)
				})
			}
		})
	}
}

func TestFaithfulSyntheticTextCompositionIm17PrefixDivergenceProbeAgainstPoppler(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	results := collectFaithfulSyntheticIm17ChildFormDiffsForProbe(t, source, tc.dpi)
	require.NotEmpty(t, results)

	resultsByName := make(map[string]faithfulSyntheticTextXObjectDiffProbe, len(results))
	for _, result := range results {
		if _, exists := resultsByName[result.resourceName]; exists {
			continue
		}
		resultsByName[result.resourceName] = result
	}

	im17Invocations := collectFaithfulSyntheticTopLevelXObjectInvocationsForProbe(t, source, "Im17", tc.dpi)
	require.NotEmpty(t, im17Invocations)

	im17 := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), source.resources, "Im17")
	require.NotNil(t, im17)

	im17ResourcesObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), im17.Dict().Get(entity.Name("Resources")))
	im17Resources, ok := im17ResourcesObj.(*entity.Dict)
	require.True(t, ok)

	decodedIm17, err := stream.NewFromEntity(im17).Decode()
	if err != nil {
		decodedIm17 = append([]byte(nil), im17.RawBytes()...)
	}
	im17Operators := parseFaithfulSyntheticRawOperatorsForProbe(t, decodedIm17)
	require.NotEmpty(t, im17Operators)

	initialIm17CTM := im17Invocations[0].effectiveCTM
	targets := []string{"Fm164", "Fm186", "Fm204"}
	targetIndexes := make(map[string]int, len(targets))
	for _, target := range targets {
		result, exists := resultsByName[target]
		require.True(t, exists)
		targetIndex, ok := faithfulSyntheticXObjectOperatorIndexForOccurrenceForProbe(im17Operators, target, result.occurrence)
		require.True(t, ok)
		targetIndexes[target] = targetIndex
	}

	checkpoints := faithfulSyntheticPrefixDivergenceCheckpointsForProbe(targetIndexes, len(im17Operators))
	for _, checkpoint := range checkpoints {
		content := buildFaithfulSyntheticParentContextReplayContentForProbe(
			initialIm17CTM,
			im17Operators,
			0,
			checkpoint,
		)
		pdfBytes := buildFaithfulSyntheticResourceContentProbePDF(t, source, im17Resources, content)
		pdfName := fmt.Sprintf("%s_im17_prefix_%04d.pdf", source.name, checkpoint)
		popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pdfName, pdfBytes, "", tc.dpi)
		popplerImg := loadPNGAsRGBAForProbe(t, popplerPNG)
		oursImg := loadPNGAsRGBAForProbe(t, oursPNG)
		operatorSummary := faithfulSyntheticOperatorSummaryForProbe(im17Operators, checkpoint)

		for _, target := range targets {
			result := resultsByName[target]
			exact, similarity := compareCroppedRGBAParityForProbe(popplerImg, oursImg, result.pixelBounds)
			metrics := measureFaithfulSyntheticBoundaryBandMetricsForProbe(popplerImg, oursImg, result.pixelBounds)
			t.Logf(
				"checkpoint=%d op=%s target=%s target_index=%d bounds=%v crop_exact=%.4f crop_similarity=%.4f support_pixels=%d support_exact=%.4f support_similarity=%.4f boundary_similarity=%.4f",
				checkpoint,
				operatorSummary,
				target,
				targetIndexes[target],
				result.pixelBounds,
				exact,
				similarity,
				metrics.support.pixels,
				metrics.support.exact,
				metrics.support.similarity,
				metrics.boundary.similarity,
			)
		}
	}
}

func TestFaithfulSyntheticTextCompositionIm17OverlapIsolationProbeAgainstPoppler(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	results := collectFaithfulSyntheticIm17ChildFormDiffsForProbe(t, source, tc.dpi)
	require.NotEmpty(t, results)

	pagePDFBytes := buildFaithfulSyntheticTextCompositionProbePDF(t, source)
	pagePDFName := fmt.Sprintf("%s_im17_overlap_baseline.pdf", source.name)
	pagePopplerPNG, pageOursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pagePDFName, pagePDFBytes, "", tc.dpi)
	pagePopplerImg := loadPNGAsRGBAForProbe(t, pagePopplerPNG)
	pageOursImg := loadPNGAsRGBAForProbe(t, pageOursPNG)

	resultsByName := make(map[string]faithfulSyntheticTextXObjectDiffProbe, len(results))
	for _, result := range results {
		if _, exists := resultsByName[result.resourceName]; exists {
			continue
		}
		resultsByName[result.resourceName] = result
	}

	im17Invocations := collectFaithfulSyntheticTopLevelXObjectInvocationsForProbe(t, source, "Im17", tc.dpi)
	require.NotEmpty(t, im17Invocations)

	im17 := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), source.resources, "Im17")
	require.NotNil(t, im17)

	im17ResourcesObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), im17.Dict().Get(entity.Name("Resources")))
	im17Resources, ok := im17ResourcesObj.(*entity.Dict)
	require.True(t, ok)

	decodedIm17, err := stream.NewFromEntity(im17).Decode()
	if err != nil {
		decodedIm17 = append([]byte(nil), im17.RawBytes()...)
	}
	im17Operators := parseFaithfulSyntheticRawOperatorsForProbe(t, decodedIm17)
	require.NotEmpty(t, im17Operators)

	childInvocations := collectFaithfulSyntheticFormXObjectInvocationsForProbe(
		t,
		source.doc.XRef(),
		im17Resources,
		im17Operators,
		source.mediaBox,
		tc.dpi,
		im17Invocations[0].effectiveCTM,
	)
	require.NotEmpty(t, childInvocations)

	targets := faithfulSyntheticIm17OverlapProbeTargets()
	for _, target := range targets {
		target := target
		t.Run(target, func(t *testing.T) {
			result, exists := resultsByName[target]
			require.True(t, exists)

			invocation, ok := faithfulSyntheticXObjectInvocationForOccurrenceForProbe(childInvocations, target, result.occurrence)
			require.True(t, ok)

			targetIndex, ok := faithfulSyntheticXObjectOperatorIndexForOccurrenceForProbe(im17Operators, target, result.occurrence)
			require.True(t, ok)

			child := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), im17Resources, target)
			require.NotNil(t, child)

			childMatrix := faithfulSyntheticFormMatrixForProbe(child)
			inverseChildMatrix, ok := faithfulSyntheticInverseMatrixForProbe(childMatrix)
			require.True(t, ok)
			childCurrentCTM := faithfulSyntheticMultiplyMatrixForProbe(invocation.effectiveCTM, inverseChildMatrix)

			targetOnlyContent := buildFaithfulSyntheticSingleDoContentForProbe(target, childCurrentCTM)
			targetOnlyPDFBytes := buildFaithfulSyntheticResourceContentProbePDF(t, source, im17Resources, targetOnlyContent)
			targetOnlyPDFName := fmt.Sprintf("%s_%s_im17_overlap_target_only.pdf", source.name, strings.ToLower(target))
			targetPopplerPNG, targetOursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, targetOnlyPDFName, targetOnlyPDFBytes, "", tc.dpi)
			targetPopplerImg := loadPNGAsRGBAForProbe(t, targetPopplerPNG)
			targetOursImg := loadPNGAsRGBAForProbe(t, targetOursPNG)

			withoutTargetContent := buildFaithfulSyntheticParentContextReplayContentExcludingOperatorForProbe(
				im17Invocations[0].effectiveCTM,
				im17Operators,
				0,
				len(im17Operators)-1,
				targetIndex,
			)
			withoutTargetPDFBytes := buildFaithfulSyntheticResourceContentProbePDF(t, source, im17Resources, withoutTargetContent)
			withoutTargetPDFName := fmt.Sprintf("%s_%s_im17_overlap_without_target.pdf", source.name, strings.ToLower(target))
			withoutTargetPopplerPNG, withoutTargetOursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(
				t,
				withoutTargetPDFName,
				withoutTargetPDFBytes,
				"",
				tc.dpi,
			)
			withoutTargetPopplerImg := loadPNGAsRGBAForProbe(t, withoutTargetPopplerPNG)
			withoutTargetOursImg := loadPNGAsRGBAForProbe(t, withoutTargetOursPNG)

			bounds := result.pixelBounds.Intersect(pagePopplerImg.Bounds()).Intersect(pageOursImg.Bounds())
			require.False(t, bounds.Empty())

			targetMask := buildUnionNonWhiteMaskForProbe(targetPopplerImg, targetOursImg, bounds)
			withoutTargetMask := buildUnionNonWhiteMaskForProbe(withoutTargetPopplerImg, withoutTargetOursImg, bounds)
			overlapMask := intersectMasksForProbe(targetMask, withoutTargetMask)
			isolatedMask := subtractMaskForProbe(targetMask, overlapMask)

			targetPixels := countMaskPixelsForProbe(targetMask)
			overlapPixels := countMaskPixelsForProbe(overlapMask)
			isolatedPixels := countMaskPixelsForProbe(isolatedMask)
			require.Greater(t, targetPixels, 0)

			targetExact, targetSimilarity, _ := compareMaskedRGBAParityForProbe(targetPopplerImg, targetOursImg, bounds, targetMask)
			targetIsolatedExact, targetIsolatedSimilarity, _ := compareMaskedRGBAParityForProbe(targetPopplerImg, targetOursImg, bounds, isolatedMask)
			withoutTargetOverlapExact, withoutTargetOverlapSimilarity, _ := compareMaskedRGBAParityForProbe(withoutTargetPopplerImg, withoutTargetOursImg, bounds, overlapMask)
			pageIsolatedExact, pageIsolatedSimilarity, _ := compareMaskedRGBAParityForProbe(pagePopplerImg, pageOursImg, bounds, isolatedMask)
			pageOverlapExact, pageOverlapSimilarity, _ := compareMaskedRGBAParityForProbe(pagePopplerImg, pageOursImg, bounds, overlapMask)

			overlapRatio := 0.0
			if targetPixels > 0 {
				overlapRatio = float64(overlapPixels) * 100.0 / float64(targetPixels)
			}

			t.Logf(
				"target=%s occurrence=%d bounds=%v target_pixels=%d overlap_pixels=%d isolated_pixels=%d overlap_ratio=%.2f target_exact=%.4f target_similarity=%.4f target_isolated_exact=%.4f target_isolated_similarity=%.4f without_target_overlap_exact=%.4f without_target_overlap_similarity=%.4f page_isolated_exact=%.4f page_isolated_similarity=%.4f page_overlap_exact=%.4f page_overlap_similarity=%.4f target_isolated_deltas=%s without_target_overlap_deltas=%s page_overlap_deltas=%s target_poppler_nonwhite_bounds=%v target_ours_nonwhite_bounds=%v without_target_poppler_nonwhite_bounds=%v without_target_ours_nonwhite_bounds=%v",
				target,
				result.occurrence,
				bounds,
				targetPixels,
				overlapPixels,
				isolatedPixels,
				overlapRatio,
				targetExact,
				targetSimilarity,
				targetIsolatedExact,
				targetIsolatedSimilarity,
				withoutTargetOverlapExact,
				withoutTargetOverlapSimilarity,
				pageIsolatedExact,
				pageIsolatedSimilarity,
				pageOverlapExact,
				pageOverlapSimilarity,
				topMaskedRGBDeltasForProbe(targetPopplerImg, targetOursImg, bounds, isolatedMask, 6),
				topMaskedRGBDeltasForProbe(withoutTargetPopplerImg, withoutTargetOursImg, bounds, overlapMask, 6),
				topMaskedRGBDeltasForProbe(pagePopplerImg, pageOursImg, bounds, overlapMask, 6),
				nonWhiteBoundsForProbe(targetPopplerImg),
				nonWhiteBoundsForProbe(targetOursImg),
				nonWhiteBoundsForProbe(withoutTargetPopplerImg),
				nonWhiteBoundsForProbe(withoutTargetOursImg),
			)
		})
	}
}

func TestFaithfulSyntheticTextCompositionIm17BackdropOperatorClassProbeAgainstPoppler(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	results := collectFaithfulSyntheticIm17ChildFormDiffsForProbe(t, source, tc.dpi)
	require.NotEmpty(t, results)

	resultsByName := make(map[string]faithfulSyntheticTextXObjectDiffProbe, len(results))
	for _, result := range results {
		if _, exists := resultsByName[result.resourceName]; exists {
			continue
		}
		resultsByName[result.resourceName] = result
	}

	im17Invocations := collectFaithfulSyntheticTopLevelXObjectInvocationsForProbe(t, source, "Im17", tc.dpi)
	require.NotEmpty(t, im17Invocations)

	im17 := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), source.resources, "Im17")
	require.NotNil(t, im17)

	im17ResourcesObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), im17.Dict().Get(entity.Name("Resources")))
	im17Resources, ok := im17ResourcesObj.(*entity.Dict)
	require.True(t, ok)

	decodedIm17, err := stream.NewFromEntity(im17).Decode()
	if err != nil {
		decodedIm17 = append([]byte(nil), im17.RawBytes()...)
	}
	im17Operators := parseFaithfulSyntheticRawOperatorsForProbe(t, decodedIm17)
	require.NotEmpty(t, im17Operators)

	childInvocations := collectFaithfulSyntheticFormXObjectInvocationsForProbe(
		t,
		source.doc.XRef(),
		im17Resources,
		im17Operators,
		source.mediaBox,
		tc.dpi,
		im17Invocations[0].effectiveCTM,
	)
	require.NotEmpty(t, childInvocations)

	target := "Fm186"
	result, exists := resultsByName[target]
	require.True(t, exists)

	invocation, ok := faithfulSyntheticXObjectInvocationForOccurrenceForProbe(childInvocations, target, result.occurrence)
	require.True(t, ok)

	targetIndex, ok := faithfulSyntheticXObjectOperatorIndexForOccurrenceForProbe(im17Operators, target, result.occurrence)
	require.True(t, ok)

	child := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), im17Resources, target)
	require.NotNil(t, child)
	childMatrix := faithfulSyntheticFormMatrixForProbe(child)
	inverseChildMatrix, ok := faithfulSyntheticInverseMatrixForProbe(childMatrix)
	require.True(t, ok)
	childCurrentCTM := faithfulSyntheticMultiplyMatrixForProbe(invocation.effectiveCTM, inverseChildMatrix)

	targetOnlyContent := buildFaithfulSyntheticSingleDoContentForProbe(target, childCurrentCTM)
	targetOnlyPDFBytes := buildFaithfulSyntheticResourceContentProbePDF(t, source, im17Resources, targetOnlyContent)
	targetOnlyPDFName := fmt.Sprintf("%s_%s_im17_backdrop_class_target_only.pdf", source.name, strings.ToLower(target))
	targetPopplerPNG, targetOursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, targetOnlyPDFName, targetOnlyPDFBytes, "", tc.dpi)
	targetPopplerImg := loadPNGAsRGBAForProbe(t, targetPopplerPNG)
	targetOursImg := loadPNGAsRGBAForProbe(t, targetOursPNG)

	withoutTargetContent := buildFaithfulSyntheticParentContextReplayContentExcludingOperatorForProbe(
		im17Invocations[0].effectiveCTM,
		im17Operators,
		0,
		len(im17Operators)-1,
		targetIndex,
	)
	withoutTargetPDFBytes := buildFaithfulSyntheticResourceContentProbePDF(t, source, im17Resources, withoutTargetContent)
	withoutTargetPDFName := fmt.Sprintf("%s_%s_im17_backdrop_class_without_target.pdf", source.name, strings.ToLower(target))
	withoutTargetPopplerPNG, withoutTargetOursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(
		t,
		withoutTargetPDFName,
		withoutTargetPDFBytes,
		"",
		tc.dpi,
	)
	withoutTargetPopplerImg := loadPNGAsRGBAForProbe(t, withoutTargetPopplerPNG)
	withoutTargetOursImg := loadPNGAsRGBAForProbe(t, withoutTargetOursPNG)

	bounds := result.pixelBounds.Intersect(targetPopplerImg.Bounds()).Intersect(targetOursImg.Bounds())
	bounds = bounds.Intersect(withoutTargetPopplerImg.Bounds()).Intersect(withoutTargetOursImg.Bounds())
	require.False(t, bounds.Empty())

	targetMask := buildUnionNonWhiteMaskForProbe(targetPopplerImg, targetOursImg, bounds)
	withoutTargetMask := buildUnionNonWhiteMaskForProbe(withoutTargetPopplerImg, withoutTargetOursImg, bounds)
	overlapMask := intersectMasksForProbe(targetMask, withoutTargetMask)
	require.Greater(t, countMaskPixelsForProbe(overlapMask), 0)

	variants := []struct {
		name string
		keep faithfulSyntheticOperatorIndexFilterForProbe
	}{
		{
			name: "full_without_target",
			keep: func(index int, _ domainrenderer.Operator) bool {
				return index != targetIndex
			},
		},
		{
			name: "no_do",
			keep: func(index int, op domainrenderer.Operator) bool {
				return index != targetIndex && op.Opcode != "Do"
			},
		},
		{
			name: "do_only",
			keep: func(index int, op domainrenderer.Operator) bool {
				return index != targetIndex &&
					(faithfulSyntheticGraphicsStateOperatorForProbe(op.Opcode) || op.Opcode == "Do")
			},
		},
		{
			name: "no_path_paint",
			keep: func(index int, op domainrenderer.Operator) bool {
				return index != targetIndex && !faithfulSyntheticPathPaintOperatorForProbe(op.Opcode)
			},
		},
		{
			name: "path_paint_only",
			keep: func(index int, op domainrenderer.Operator) bool {
				return index != targetIndex &&
					(faithfulSyntheticGraphicsStateOperatorForProbe(op.Opcode) ||
						faithfulSyntheticPathConstructionOperatorForProbe(op.Opcode) ||
						faithfulSyntheticPathPaintOperatorForProbe(op.Opcode))
			},
		},
		{
			name: "stroke_only",
			keep: func(index int, op domainrenderer.Operator) bool {
				return index != targetIndex &&
					(faithfulSyntheticGraphicsStateOperatorForProbe(op.Opcode) ||
						faithfulSyntheticPathConstructionOperatorForProbe(op.Opcode) ||
						faithfulSyntheticStrokePaintOperatorForProbe(op.Opcode))
			},
		},
		{
			name: "fill_only",
			keep: func(index int, op domainrenderer.Operator) bool {
				return index != targetIndex &&
					(faithfulSyntheticGraphicsStateOperatorForProbe(op.Opcode) ||
						faithfulSyntheticPathConstructionOperatorForProbe(op.Opcode) ||
						faithfulSyntheticFillPaintOperatorForProbe(op.Opcode))
			},
		},
	}

	for _, variant := range variants {
		variant := variant
		t.Run(variant.name, func(t *testing.T) {
			content := buildFaithfulSyntheticParentContextReplayContentFilteredForProbe(
				im17Invocations[0].effectiveCTM,
				im17Operators,
				0,
				len(im17Operators)-1,
				variant.keep,
			)
			pdfBytes := buildFaithfulSyntheticResourceContentProbePDF(t, source, im17Resources, content)
			pdfName := fmt.Sprintf(
				"%s_%s_im17_backdrop_class_%s.pdf",
				source.name,
				strings.ToLower(target),
				variant.name,
			)
			popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pdfName, pdfBytes, "", tc.dpi)
			popplerImg := loadPNGAsRGBAForProbe(t, popplerPNG)
			oursImg := loadPNGAsRGBAForProbe(t, oursPNG)
			exact, similarity, pixels := compareMaskedRGBAParityForProbe(popplerImg, oursImg, bounds, overlapMask)
			t.Logf(
				"target=%s variant=%s overlap_pixels=%d exact=%.4f similarity=%.4f deltas=%s nonwhite_poppler=%v nonwhite_ours=%v",
				target,
				variant.name,
				pixels,
				exact,
				similarity,
				topMaskedRGBDeltasForProbe(popplerImg, oursImg, bounds, overlapMask, 8),
				nonWhiteBoundsForProbe(popplerImg),
				nonWhiteBoundsForProbe(oursImg),
			)
		})
	}
}

func TestFaithfulSyntheticTextCompositionIm17StrokePrefixProbeAgainstPoppler(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	results := collectFaithfulSyntheticIm17ChildFormDiffsForProbe(t, source, tc.dpi)
	require.NotEmpty(t, results)

	resultsByName := make(map[string]faithfulSyntheticTextXObjectDiffProbe, len(results))
	for _, result := range results {
		if _, exists := resultsByName[result.resourceName]; exists {
			continue
		}
		resultsByName[result.resourceName] = result
	}

	im17Invocations := collectFaithfulSyntheticTopLevelXObjectInvocationsForProbe(t, source, "Im17", tc.dpi)
	require.NotEmpty(t, im17Invocations)

	im17 := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), source.resources, "Im17")
	require.NotNil(t, im17)

	im17ResourcesObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), im17.Dict().Get(entity.Name("Resources")))
	im17Resources, ok := im17ResourcesObj.(*entity.Dict)
	require.True(t, ok)

	decodedIm17, err := stream.NewFromEntity(im17).Decode()
	if err != nil {
		decodedIm17 = append([]byte(nil), im17.RawBytes()...)
	}
	im17Operators := parseFaithfulSyntheticRawOperatorsForProbe(t, decodedIm17)
	require.NotEmpty(t, im17Operators)

	childInvocations := collectFaithfulSyntheticFormXObjectInvocationsForProbe(
		t,
		source.doc.XRef(),
		im17Resources,
		im17Operators,
		source.mediaBox,
		tc.dpi,
		im17Invocations[0].effectiveCTM,
	)
	require.NotEmpty(t, childInvocations)

	target := "Fm186"
	result, exists := resultsByName[target]
	require.True(t, exists)

	invocation, ok := faithfulSyntheticXObjectInvocationForOccurrenceForProbe(childInvocations, target, result.occurrence)
	require.True(t, ok)

	child := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), im17Resources, target)
	require.NotNil(t, child)
	childMatrix := faithfulSyntheticFormMatrixForProbe(child)
	inverseChildMatrix, ok := faithfulSyntheticInverseMatrixForProbe(childMatrix)
	require.True(t, ok)
	childCurrentCTM := faithfulSyntheticMultiplyMatrixForProbe(invocation.effectiveCTM, inverseChildMatrix)

	targetOnlyContent := buildFaithfulSyntheticSingleDoContentForProbe(target, childCurrentCTM)
	targetOnlyPDFBytes := buildFaithfulSyntheticResourceContentProbePDF(t, source, im17Resources, targetOnlyContent)
	targetOnlyPDFName := fmt.Sprintf("%s_%s_im17_stroke_prefix_target_only.pdf", source.name, strings.ToLower(target))
	targetPopplerPNG, targetOursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, targetOnlyPDFName, targetOnlyPDFBytes, "", tc.dpi)
	targetPopplerImg := loadPNGAsRGBAForProbe(t, targetPopplerPNG)
	targetOursImg := loadPNGAsRGBAForProbe(t, targetOursPNG)

	strokeOnlyContent := buildFaithfulSyntheticParentContextReplayContentFilteredForProbe(
		im17Invocations[0].effectiveCTM,
		im17Operators,
		0,
		len(im17Operators)-1,
		func(_ int, op domainrenderer.Operator) bool {
			return faithfulSyntheticGraphicsStateOperatorForProbe(op.Opcode) ||
				faithfulSyntheticPathConstructionOperatorForProbe(op.Opcode) ||
				faithfulSyntheticStrokePaintOperatorForProbe(op.Opcode)
		},
	)
	strokeOnlyPDFBytes := buildFaithfulSyntheticResourceContentProbePDF(t, source, im17Resources, strokeOnlyContent)
	strokeOnlyPDFName := fmt.Sprintf("%s_%s_im17_stroke_prefix_full_stroke_only.pdf", source.name, strings.ToLower(target))
	strokeOnlyPopplerPNG, strokeOnlyOursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(
		t,
		strokeOnlyPDFName,
		strokeOnlyPDFBytes,
		"",
		tc.dpi,
	)
	strokeOnlyPopplerImg := loadPNGAsRGBAForProbe(t, strokeOnlyPopplerPNG)
	strokeOnlyOursImg := loadPNGAsRGBAForProbe(t, strokeOnlyOursPNG)

	bounds := result.pixelBounds.Intersect(targetPopplerImg.Bounds()).Intersect(targetOursImg.Bounds())
	bounds = bounds.Intersect(strokeOnlyPopplerImg.Bounds()).Intersect(strokeOnlyOursImg.Bounds())
	require.False(t, bounds.Empty())

	targetMask := buildUnionNonWhiteMaskForProbe(targetPopplerImg, targetOursImg, bounds)
	strokeOnlyMask := buildUnionNonWhiteMaskForProbe(strokeOnlyPopplerImg, strokeOnlyOursImg, bounds)
	overlapMask := intersectMasksForProbe(targetMask, strokeOnlyMask)
	require.Greater(t, countMaskPixelsForProbe(overlapMask), 0)

	strokeIndices := make([]int, 0, 128)
	for index, op := range im17Operators {
		if faithfulSyntheticStrokePaintOperatorForProbe(op.Opcode) {
			strokeIndices = append(strokeIndices, index)
		}
	}
	require.NotEmpty(t, strokeIndices)

	renderPrefix := func(endIndex int) (float64, float64, string) {
		content := buildFaithfulSyntheticParentContextReplayContentFilteredForProbe(
			im17Invocations[0].effectiveCTM,
			im17Operators,
			0,
			endIndex,
			func(_ int, op domainrenderer.Operator) bool {
				return faithfulSyntheticGraphicsStateOperatorForProbe(op.Opcode) ||
					faithfulSyntheticPathConstructionOperatorForProbe(op.Opcode) ||
					faithfulSyntheticStrokePaintOperatorForProbe(op.Opcode)
			},
		)
		pdfBytes := buildFaithfulSyntheticResourceContentProbePDF(t, source, im17Resources, content)
		pdfName := fmt.Sprintf("%s_%s_im17_stroke_prefix_%04d.pdf", source.name, strings.ToLower(target), endIndex)
		popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pdfName, pdfBytes, "", tc.dpi)
		popplerImg := loadPNGAsRGBAForProbe(t, popplerPNG)
		oursImg := loadPNGAsRGBAForProbe(t, oursPNG)
		exact, similarity, _ := compareMaskedRGBAParityForProbe(popplerImg, oursImg, bounds, overlapMask)
		return exact, similarity, topMaskedRGBDeltasForProbe(popplerImg, oursImg, bounds, overlapMask, 6)
	}

	firstMismatch := -1
	for left, right := 0, len(strokeIndices)-1; left <= right; {
		mid := left + (right-left)/2
		endIndex := strokeIndices[mid]
		exact, similarity, deltas := renderPrefix(endIndex)
		t.Logf(
			"target=%s search_stroke_slot=%d operator_index=%d exact=%.4f similarity=%.4f deltas=%s op=%s",
			target,
			mid,
			endIndex,
			exact,
			similarity,
			deltas,
			faithfulSyntheticOperatorSummaryForProbe(im17Operators, endIndex),
		)
		if exact < 100 {
			firstMismatch = mid
			right = mid - 1
		} else {
			left = mid + 1
		}
	}
	require.NotEqual(t, -1, firstMismatch)

	start := firstMismatch - 3
	if start < 0 {
		start = 0
	}
	end := firstMismatch + 3
	if end >= len(strokeIndices) {
		end = len(strokeIndices) - 1
	}
	for slot := start; slot <= end; slot++ {
		endIndex := strokeIndices[slot]
		exact, similarity, deltas := renderPrefix(endIndex)
		t.Logf(
			"target=%s nearby_stroke_slot=%d operator_index=%d exact=%.4f similarity=%.4f deltas=%s context=%s detail=%s",
			target,
			slot,
			endIndex,
			exact,
			similarity,
			deltas,
			formatFaithfulSyntheticOperatorSequenceForProbe(im17Operators[maxIntForProbe(0, endIndex-8):minIntForProbe(len(im17Operators), endIndex+1)]),
			compactFaithfulSyntheticProgramForProbe(formatFaithfulSyntheticOperatorProgramForProbe(
				im17Operators[maxIntForProbe(0, endIndex-8):minIntForProbe(len(im17Operators), endIndex+1)],
			), 260),
		)
	}
}

func TestFaithfulSyntheticTextCompositionPageHotspotPrefixProbeAgainstPoppler(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()
	require.NotEmpty(t, source.keptOps)

	hotspots := []struct {
		name string
		rect image.Rectangle
	}{
		{name: "bottom_see", rect: image.Rect(465, 1490, 535, 1535)},
		{name: "bottom_url", rect: image.Rect(620, 1490, 690, 1535)},
		{name: "mid_row_997", rect: image.Rect(455, 970, 530, 1025)},
	}
	type prefixMetric struct {
		exact          float64
		similarity     float64
		nonzeroDeltas  string
		mismatchPixels int
		mismatchBounds image.Rectangle
	}
	for _, hotspot := range hotspots {
		hotspot := hotspot
		t.Run(hotspot.name, func(t *testing.T) {
			cache := make(map[int]prefixMetric)
			measurePrefix := func(count int) prefixMetric {
				if count < 0 {
					count = 0
				}
				if count > len(source.keptOps) {
					count = len(source.keptOps)
				}
				if metric, exists := cache[count]; exists {
					return metric
				}
				pdfBytes := buildFaithfulSyntheticTextCompositionProbePDFWithOperators(t, source, source.keptOps[:count])
				pdfName := fmt.Sprintf("%s_%s_hotspot_prefix_%05d.pdf", source.name, hotspot.name, count)
				popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pdfName, pdfBytes, "", tc.dpi)
				popplerImg := loadPNGAsRGBAForProbe(t, popplerPNG)
				oursImg := loadPNGAsRGBAForProbe(t, oursPNG)
				exact, similarity := compareCroppedRGBAParityForProbe(popplerImg, oursImg, hotspot.rect)
				nonzeroDeltas, mismatchPixels, mismatchBounds := topMaskedNonzeroRGBDeltasForProbe(popplerImg, oursImg, hotspot.rect, nil, 6)
				metric := prefixMetric{
					exact:          exact,
					similarity:     similarity,
					nonzeroDeltas:  nonzeroDeltas,
					mismatchPixels: mismatchPixels,
					mismatchBounds: mismatchBounds,
				}
				cache[count] = metric
				return metric
			}

			low := 0
			high := len(source.keptOps)
			for low < high {
				mid := low + (high-low)/2
				metric := measurePrefix(mid)
				if metric.exact < 100 {
					high = mid
					continue
				}
				low = mid + 1
			}
			firstMismatch := low
			if firstMismatch > len(source.keptOps) {
				firstMismatch = len(source.keptOps)
			}

			for _, count := range []int{
				maxIntForProbe(0, firstMismatch-4),
				maxIntForProbe(0, firstMismatch-2),
				maxIntForProbe(0, firstMismatch-1),
				firstMismatch,
				minIntForProbe(len(source.keptOps), firstMismatch+1),
				minIntForProbe(len(source.keptOps), firstMismatch+2),
				minIntForProbe(len(source.keptOps), firstMismatch+4),
			} {
				metric := measurePrefix(count)
				t.Logf(
					"hotspot=%s rect=%v prefix_count=%d exact=%.4f similarity=%.4f mismatch_pixels=%d mismatch_bounds=%v nonzero_deltas=%s previous_op=%d:%s next_op=%d:%s",
					hotspot.name,
					hotspot.rect,
					count,
					metric.exact,
					metric.similarity,
					metric.mismatchPixels,
					metric.mismatchBounds,
					metric.nonzeroDeltas,
					count-1,
					faithfulSyntheticOperatorSummaryForProbe(source.keptOps, count-1),
					count,
					faithfulSyntheticOperatorSummaryForProbe(source.keptOps, count),
				)
			}

			windowStart := maxIntForProbe(0, firstMismatch-8)
			windowEnd := minIntForProbe(len(source.keptOps), firstMismatch+8)
			t.Logf(
				"hotspot=%s rect=%v first_mismatch_prefix=%d operator_window=%s",
				hotspot.name,
				hotspot.rect,
				firstMismatch,
				formatFaithfulSyntheticOperatorProgramForProbe(source.keptOps[windowStart:windowEnd]),
			)
		})
	}
}

func TestFaithfulSyntheticTextCompositionF176BottomURLGlyphPrefixProbeAgainstPoppler(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()
	require.NotEmpty(t, source.keptOps)

	targetOpIndex := -1
	for index, op := range source.keptOps {
		if op.Opcode != "TJ" {
			continue
		}
		if strings.Contains(serializeFaithfulSyntheticTextOperatorForProbe(op), "68747470733A2F2F6769746875622E636F6D") {
			targetOpIndex = index
			break
		}
	}
	require.NotEqual(t, -1, targetOpIndex)

	targetOp := source.keptOps[targetOpIndex]
	totalGlyphs := faithfulSyntheticTextArrayGlyphCountForProbe(targetOp)
	require.Greater(t, totalGlyphs, 0)

	hotspot := image.Rect(620, 1490, 690, 1535)
	type glyphPrefixMetric struct {
		exact          float64
		similarity     float64
		nonzeroDeltas  string
		samples        string
		mismatchPixels int
		mismatchBounds image.Rectangle
		popplerImg     *image.RGBA
		oursImg        *image.RGBA
	}

	cache := make(map[int]glyphPrefixMetric)
	measureGlyphPrefix := func(glyphCount int) glyphPrefixMetric {
		if glyphCount < 0 {
			glyphCount = 0
		}
		if glyphCount > totalGlyphs {
			glyphCount = totalGlyphs
		}
		if metric, exists := cache[glyphCount]; exists {
			return metric
		}

		operators := append([]domainrenderer.Operator(nil), source.keptOps[:targetOpIndex]...)
		if glyphCount > 0 {
			operators = append(operators, faithfulSyntheticTextArrayPrefixOperatorForProbe(targetOp, glyphCount))
		}

		pdfBytes := buildFaithfulSyntheticTextCompositionProbePDFWithOperators(t, source, operators)
		pdfName := fmt.Sprintf("%s_f176_bottom_url_glyph_prefix_%03d.pdf", source.name, glyphCount)
		popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pdfName, pdfBytes, "", tc.dpi)
		popplerImg := loadPNGAsRGBAForProbe(t, popplerPNG)
		oursImg := loadPNGAsRGBAForProbe(t, oursPNG)
		exact, similarity := compareCroppedRGBAParityForProbe(popplerImg, oursImg, hotspot)
		nonzeroDeltas, mismatchPixels, mismatchBounds := topMaskedNonzeroRGBDeltasForProbe(popplerImg, oursImg, hotspot, nil, 6)
		samples := sampleMaskedMismatchedRGBForProbe(popplerImg, oursImg, hotspot, nil, 4)

		metric := glyphPrefixMetric{
			exact:          exact,
			similarity:     similarity,
			nonzeroDeltas:  nonzeroDeltas,
			samples:        samples,
			mismatchPixels: mismatchPixels,
			mismatchBounds: mismatchBounds,
			popplerImg:     popplerImg,
			oursImg:        oursImg,
		}
		cache[glyphCount] = metric
		return metric
	}

	if measureGlyphPrefix(totalGlyphs).exact == 100 {
		t.Logf("target_op_index=%d total_glyphs=%d hotspot=%v all glyph prefixes match", targetOpIndex, totalGlyphs, hotspot)
		return
	}

	low := 1
	high := totalGlyphs
	for low < high {
		mid := low + (high-low)/2
		metric := measureGlyphPrefix(mid)
		if metric.exact < 100 {
			high = mid
			continue
		}
		low = mid + 1
	}
	firstMismatch := low
	backdropMetric := measureGlyphPrefix(maxIntForProbe(0, firstMismatch-1))
	firstMismatchMetric := measureGlyphPrefix(firstMismatch)

	for _, glyphCount := range uniqueSortedIntsForProbe([]int{
		0,
		maxIntForProbe(0, firstMismatch-4),
		maxIntForProbe(0, firstMismatch-2),
		maxIntForProbe(0, firstMismatch-1),
		firstMismatch,
		minIntForProbe(totalGlyphs, firstMismatch+1),
		minIntForProbe(totalGlyphs, firstMismatch+2),
		minIntForProbe(totalGlyphs, firstMismatch+4),
		totalGlyphs,
	}) {
		metric := measureGlyphPrefix(glyphCount)
		t.Logf(
			"target_op_index=%d hotspot=%v glyph_prefix=%d/%d glyph=%s exact=%.4f similarity=%.4f mismatch_pixels=%d mismatch_bounds=%v nonzero_deltas=%s samples=%s",
			targetOpIndex,
			hotspot,
			glyphCount,
			totalGlyphs,
			faithfulSyntheticTextArrayPrefixLabelForProbe(targetOp, glyphCount),
			metric.exact,
			metric.similarity,
			metric.mismatchPixels,
			metric.mismatchBounds,
			metric.nonzeroDeltas,
			metric.samples,
		)
	}

	windowStart := maxIntForProbe(0, targetOpIndex-4)
	windowEnd := minIntForProbe(len(source.keptOps), targetOpIndex+4)
	t.Logf(
		"target_op_index=%d first_mismatch_glyph_prefix=%d backdrop_samples=%s operator_window=%s",
		targetOpIndex,
		firstMismatch,
		sampleMaskedMismatchedRGBWithBackdropForProbe(
			firstMismatchMetric.popplerImg,
			firstMismatchMetric.oursImg,
			backdropMetric.popplerImg,
			hotspot,
			nil,
			4,
		),
		formatFaithfulSyntheticOperatorProgramForProbe(source.keptOps[windowStart:windowEnd]),
	)
}

func TestFaithfulSyntheticTextCompositionDeviceCMYKMagentaSolidProbeAgainstPoppler(t *testing.T) {
	pdfBytes := buildSyntheticContentPDF(20, 20, []byte("0 1 0 0 k\n0 0 20 20 re f\n"))
	popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, "device_cmyk_magenta_solid.pdf", pdfBytes, "", 72)
	popplerImg := loadPNGAsRGBAForProbe(t, popplerPNG)
	oursImg := loadPNGAsRGBAForProbe(t, oursPNG)

	sample := image.Pt(10, 10)
	popplerColor := popplerImg.RGBAAt(sample.X, sample.Y)
	oursColor := oursImg.RGBAAt(sample.X, sample.Y)
	t.Logf("sample=%v poppler=(%d,%d,%d,%d) ours=(%d,%d,%d,%d)",
		sample,
		popplerColor.R,
		popplerColor.G,
		popplerColor.B,
		popplerColor.A,
		oursColor.R,
		oursColor.G,
		oursColor.B,
		oursColor.A,
	)
}

func faithfulSyntheticTextArrayGlyphCountForProbe(op domainrenderer.Operator) int {
	if op.Opcode != "TJ" || len(op.Operands) == 0 {
		return 0
	}
	array, ok := op.Operands[0].(*entity.Array)
	if !ok {
		return 0
	}

	count := 0
	for _, item := range array.Items() {
		if text, ok := item.(*entity.String); ok {
			count += len(text.Value())
		}
	}
	return count
}

func faithfulSyntheticTextArrayPrefixOperatorForProbe(op domainrenderer.Operator, glyphCount int) domainrenderer.Operator {
	if glyphCount <= 0 || op.Opcode != "TJ" || len(op.Operands) == 0 {
		return domainrenderer.Operator{Resources: op.Resources, Opcode: op.Opcode, Operands: []entity.Object{entity.NewArray()}}
	}
	array, ok := op.Operands[0].(*entity.Array)
	if !ok {
		return op
	}

	remaining := glyphCount
	items := make([]entity.Object, 0, array.Len())
	for _, item := range array.Items() {
		text, isText := item.(*entity.String)
		if !isText {
			if remaining > 0 {
				items = append(items, item.Clone())
			}
			continue
		}

		raw := text.Value()
		if remaining >= len(raw) {
			items = append(items, entity.NewString(raw))
			remaining -= len(raw)
			continue
		}
		if remaining > 0 {
			items = append(items, entity.NewString(raw[:remaining]))
		}
		break
	}

	return domainrenderer.Operator{
		Resources: op.Resources,
		Opcode:    op.Opcode,
		Operands:  []entity.Object{entity.NewArray(items...)},
	}
}

func faithfulSyntheticTextArrayPrefixLabelForProbe(op domainrenderer.Operator, glyphCount int) string {
	if glyphCount <= 0 {
		return "-"
	}
	raw := faithfulSyntheticTextArrayPrefixBytesForProbe(op, glyphCount)
	if len(raw) == 0 {
		return "-"
	}

	last := raw[len(raw)-1]
	preview := string(raw)
	if len(preview) > 36 {
		preview = "..." + preview[len(preview)-36:]
	}
	if last >= 0x20 && last <= 0x7e {
		return fmt.Sprintf("0x%02X/%q prefix=%q", last, string([]byte{last}), preview)
	}
	return fmt.Sprintf("0x%02X prefix=%q", last, preview)
}

func faithfulSyntheticTextArrayPrefixBytesForProbe(op domainrenderer.Operator, glyphCount int) []byte {
	if op.Opcode != "TJ" || len(op.Operands) == 0 || glyphCount <= 0 {
		return nil
	}
	array, ok := op.Operands[0].(*entity.Array)
	if !ok {
		return nil
	}

	out := make([]byte, 0, glyphCount)
	remaining := glyphCount
	for _, item := range array.Items() {
		text, ok := item.(*entity.String)
		if !ok {
			continue
		}
		raw := text.Value()
		if remaining >= len(raw) {
			out = append(out, raw...)
			remaining -= len(raw)
			if remaining == 0 {
				break
			}
			continue
		}
		out = append(out, raw[:remaining]...)
		break
	}
	return out
}

func uniqueSortedIntsForProbe(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	out := make([]int, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func faithfulSyntheticIm17OverlapProbeTargets() []string {
	return []string{"Fm186", "Fm228", "Fm204", "Fm164", "Fm213", "Fm223", "Fm205", "Fm210"}
}

func minIntForProbe(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxIntForProbe(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func faithfulSyntheticPrefixDivergenceCheckpointsForProbe(
	targetIndexes map[string]int,
	operatorCount int,
) []int {
	candidates := []int{0, 250, 500, 750, 1000, 1500, 2000, 2500, 3000}
	for _, targetIndex := range targetIndexes {
		for _, delta := range []int{400, 200, 100, 60, 40, 20, 10, 5, 1, 0} {
			candidates = append(candidates, targetIndex-delta)
		}
	}

	seen := make(map[int]struct{}, len(candidates))
	checkpoints := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate < 0 || candidate >= operatorCount {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		checkpoints = append(checkpoints, candidate)
	}
	sort.Ints(checkpoints)
	return checkpoints
}

func faithfulSyntheticOperatorSummaryForProbe(operators []domainrenderer.Operator, index int) string {
	if index < 0 || index >= len(operators) {
		return "<out-of-range>"
	}
	summary := strings.TrimSpace(serializeFaithfulSyntheticTextOperatorForProbe(operators[index]))
	if len(summary) > 96 {
		return summary[:96] + "..."
	}
	return summary
}

func TestFaithfulSyntheticTextCompositionIm17OverlapContributorProbe(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	results := collectFaithfulSyntheticIm17ChildFormDiffsForProbe(t, source, tc.dpi)
	require.NotEmpty(t, results)

	resultsByName := make(map[string]faithfulSyntheticTextXObjectDiffProbe, len(results))
	for _, result := range results {
		if _, exists := resultsByName[result.resourceName]; exists {
			continue
		}
		resultsByName[result.resourceName] = result
	}

	im17Invocations := collectFaithfulSyntheticTopLevelXObjectInvocationsForProbe(t, source, "Im17", tc.dpi)
	require.NotEmpty(t, im17Invocations)

	im17 := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), source.resources, "Im17")
	require.NotNil(t, im17)

	im17ResourcesObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), im17.Dict().Get(entity.Name("Resources")))
	im17Resources, ok := im17ResourcesObj.(*entity.Dict)
	require.True(t, ok)

	decodedIm17, err := stream.NewFromEntity(im17).Decode()
	if err != nil {
		decodedIm17 = append([]byte(nil), im17.RawBytes()...)
	}
	im17Operators := parseFaithfulSyntheticRawOperatorsForProbe(t, decodedIm17)
	require.NotEmpty(t, im17Operators)

	childInvocations := collectFaithfulSyntheticFormXObjectInvocationsForProbe(
		t,
		source.doc.XRef(),
		im17Resources,
		im17Operators,
		source.mediaBox,
		tc.dpi,
		im17Invocations[0].effectiveCTM,
	)
	require.NotEmpty(t, childInvocations)

	type invocationWithOccurrence struct {
		index      int
		occurrence int
		invocation faithfulSyntheticTextXObjectInvocationProbe
	}

	countByName := make(map[string]int)
	invocations := make([]invocationWithOccurrence, 0, len(childInvocations))
	for index, invocation := range childInvocations {
		countByName[invocation.resourceName]++
		invocations = append(invocations, invocationWithOccurrence{
			index:      index,
			occurrence: countByName[invocation.resourceName],
			invocation: invocation,
		})
	}

	type overlapContributor struct {
		index      int
		occurrence int
		name       string
		relation   string
		area       int
		bounds     image.Rectangle
	}

	for _, target := range []string{"Fm186", "Fm228", "Fm204", "Fm164"} {
		targetResult, exists := resultsByName[target]
		require.True(t, exists)

		targetIndex := -1
		for _, invocation := range invocations {
			if invocation.invocation.resourceName == target && invocation.occurrence == targetResult.occurrence {
				targetIndex = invocation.index
				break
			}
		}
		require.NotEqual(t, -1, targetIndex)

		contributors := make([]overlapContributor, 0, 16)
		for _, invocation := range invocations {
			if invocation.index == targetIndex {
				continue
			}
			overlap := targetResult.pixelBounds.Intersect(invocation.invocation.pixelBounds)
			if overlap.Empty() {
				continue
			}
			relation := "before"
			if invocation.index > targetIndex {
				relation = "after"
			}
			contributors = append(contributors, overlapContributor{
				index:      invocation.index,
				occurrence: invocation.occurrence,
				name:       invocation.invocation.resourceName,
				relation:   relation,
				area:       overlap.Dx() * overlap.Dy(),
				bounds:     overlap,
			})
		}

		sort.Slice(contributors, func(left, right int) bool {
			if contributors[left].area == contributors[right].area {
				return contributors[left].index < contributors[right].index
			}
			return contributors[left].area > contributors[right].area
		})

		limit := len(contributors)
		if limit > 12 {
			limit = 12
		}
		formatted := make([]string, 0, limit)
		for _, contributor := range contributors[:limit] {
			formatted = append(formatted, fmt.Sprintf(
				"%s#%d@%d:%s area=%d bounds=%v",
				contributor.name,
				contributor.occurrence,
				contributor.index,
				contributor.relation,
				contributor.area,
				contributor.bounds,
			))
		}

		t.Logf(
			"target=%s occurrence=%d index=%d bounds=%v contributors=%s",
			target,
			targetResult.occurrence,
			targetIndex,
			targetResult.pixelBounds,
			strings.Join(formatted, "; "),
		)
	}
}

func TestFaithfulSyntheticTextCompositionIm17OverlapLastPainterProbeAgainstPoppler(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	results := collectFaithfulSyntheticIm17ChildFormDiffsForProbe(t, source, tc.dpi)
	require.NotEmpty(t, results)

	resultsByName := make(map[string]faithfulSyntheticTextXObjectDiffProbe, len(results))
	for _, result := range results {
		if _, exists := resultsByName[result.resourceName]; exists {
			continue
		}
		resultsByName[result.resourceName] = result
	}

	im17Invocations := collectFaithfulSyntheticTopLevelXObjectInvocationsForProbe(t, source, "Im17", tc.dpi)
	require.NotEmpty(t, im17Invocations)

	im17 := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), source.resources, "Im17")
	require.NotNil(t, im17)

	im17ResourcesObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), im17.Dict().Get(entity.Name("Resources")))
	im17Resources, ok := im17ResourcesObj.(*entity.Dict)
	require.True(t, ok)

	decodedIm17, err := stream.NewFromEntity(im17).Decode()
	if err != nil {
		decodedIm17 = append([]byte(nil), im17.RawBytes()...)
	}
	im17Operators := parseFaithfulSyntheticRawOperatorsForProbe(t, decodedIm17)
	require.NotEmpty(t, im17Operators)

	childInvocations := collectFaithfulSyntheticFormXObjectInvocationsForProbe(
		t,
		source.doc.XRef(),
		im17Resources,
		im17Operators,
		source.mediaBox,
		tc.dpi,
		im17Invocations[0].effectiveCTM,
	)
	require.NotEmpty(t, childInvocations)

	type invocationWithOccurrence struct {
		index      int
		occurrence int
		invocation faithfulSyntheticTextXObjectInvocationProbe
	}

	countByName := make(map[string]int)
	invocations := make([]invocationWithOccurrence, 0, len(childInvocations))
	for index, invocation := range childInvocations {
		countByName[invocation.resourceName]++
		invocations = append(invocations, invocationWithOccurrence{
			index:      index,
			occurrence: countByName[invocation.resourceName],
			invocation: invocation,
		})
	}

	pagePDFBytes := buildFaithfulSyntheticTextCompositionProbePDF(t, source)
	pagePDFName := fmt.Sprintf("%s_im17_last_painter_baseline.pdf", source.name)
	pagePopplerPNG, pageOursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pagePDFName, pagePDFBytes, "", tc.dpi)
	pagePopplerImg := loadPNGAsRGBAForProbe(t, pagePopplerPNG)
	pageOursImg := loadPNGAsRGBAForProbe(t, pageOursPNG)

	target := "Fm186"
	targetResult, exists := resultsByName[target]
	require.True(t, exists)

	targetIndex := -1
	for _, invocation := range invocations {
		if invocation.invocation.resourceName == target && invocation.occurrence == targetResult.occurrence {
			targetIndex = invocation.index
			break
		}
	}
	require.NotEqual(t, -1, targetIndex)

	type painterProbe struct {
		index      int
		occurrence int
		name       string
		mask       []bool
		popplerImg *image.RGBA
		oursImg    *image.RGBA
	}

	bounds := targetResult.pixelBounds.Intersect(pagePopplerImg.Bounds()).Intersect(pageOursImg.Bounds())
	require.False(t, bounds.Empty())

	painters := make([]painterProbe, 0, 16)
	for _, invocation := range invocations {
		overlap := targetResult.pixelBounds.Intersect(invocation.invocation.pixelBounds)
		if overlap.Empty() && invocation.index != targetIndex {
			continue
		}

		child := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), im17Resources, invocation.invocation.resourceName)
		require.NotNil(t, child)
		childMatrix := faithfulSyntheticFormMatrixForProbe(child)
		inverseChildMatrix, ok := faithfulSyntheticInverseMatrixForProbe(childMatrix)
		require.True(t, ok)
		childCurrentCTM := faithfulSyntheticMultiplyMatrixForProbe(invocation.invocation.effectiveCTM, inverseChildMatrix)

		content := buildFaithfulSyntheticSingleDoContentForProbe(invocation.invocation.resourceName, childCurrentCTM)
		pdfBytes := buildFaithfulSyntheticResourceContentProbePDF(t, source, im17Resources, content)
		pdfName := fmt.Sprintf(
			"%s_%s_%03d_im17_last_painter.pdf",
			source.name,
			strings.ToLower(invocation.invocation.resourceName),
			invocation.index,
		)
		popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pdfName, pdfBytes, "", tc.dpi)
		popplerImg := loadPNGAsRGBAForProbe(t, popplerPNG)
		oursImg := loadPNGAsRGBAForProbe(t, oursPNG)
		mask := buildUnionNonWhiteMaskForProbe(popplerImg, oursImg, bounds)
		if countMaskPixelsForProbe(mask) == 0 {
			continue
		}
		exact, similarity, pixels := compareMaskedRGBAParityForProbe(popplerImg, oursImg, bounds, mask)
		t.Logf(
			"painter=%s#%d index=%d mask_pixels=%d exact=%.4f similarity=%.4f nonwhite_poppler=%v nonwhite_ours=%v",
			invocation.invocation.resourceName,
			invocation.occurrence,
			invocation.index,
			pixels,
			exact,
			similarity,
			nonWhiteBoundsForProbe(popplerImg),
			nonWhiteBoundsForProbe(oursImg),
		)
		painters = append(painters, painterProbe{
			index:      invocation.index,
			occurrence: invocation.occurrence,
			name:       invocation.invocation.resourceName,
			mask:       mask,
			popplerImg: popplerImg,
			oursImg:    oursImg,
		})
	}
	require.NotEmpty(t, painters)
	sort.Slice(painters, func(left, right int) bool {
		return painters[left].index < painters[right].index
	})

	counts := make(map[string]int)
	replacementCounts := make(map[string]int)
	width := bounds.Dx()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			left := pagePopplerImg.RGBAAt(x, y)
			right := pageOursImg.RGBAAt(x, y)
			if left == right {
				continue
			}

			maskIndex := (y-bounds.Min.Y)*width + (x - bounds.Min.X)
			last := "none"
			lastIndex := -1
			lastOccurrence := 0
			var lastPainter *painterProbe
			for _, painter := range painters {
				if maskIndex < 0 || maskIndex >= len(painter.mask) || !painter.mask[maskIndex] {
					continue
				}
				last = painter.name
				lastIndex = painter.index
				lastOccurrence = painter.occurrence
				lastPainter = &painter
			}
			counts[fmt.Sprintf("%s#%d@%d", last, lastOccurrence, lastIndex)]++
			if lastPainter == nil {
				replacementCounts["none"]++
				continue
			}
			popplerReplaced := pagePopplerImg.RGBAAt(x, y) == lastPainter.popplerImg.RGBAAt(x, y)
			oursReplaced := pageOursImg.RGBAAt(x, y) == lastPainter.oursImg.RGBAAt(x, y)
			replacementCounts[fmt.Sprintf("poppler_replaced=%t/ours_replaced=%t", popplerReplaced, oursReplaced)]++
		}
	}

	type lastPainterCount struct {
		key   string
		count int
	}
	grouped := make([]lastPainterCount, 0, len(counts))
	for key, count := range counts {
		grouped = append(grouped, lastPainterCount{key: key, count: count})
	}
	sort.Slice(grouped, func(left, right int) bool {
		if grouped[left].count == grouped[right].count {
			return grouped[left].key < grouped[right].key
		}
		return grouped[left].count > grouped[right].count
	})
	parts := make([]string, 0, len(grouped))
	for _, item := range grouped {
		parts = append(parts, fmt.Sprintf("%s:%d", item.key, item.count))
	}
	replacementParts := formatCountMapDescendingForProbe(replacementCounts)
	t.Logf(
		"target=%s bounds=%v mismatch_last_painters=%s replacement=%s",
		target,
		bounds,
		strings.Join(parts, ","),
		replacementParts,
	)
}

func formatCountMapDescendingForProbe(counts map[string]int) string {
	if len(counts) == 0 {
		return "-"
	}

	type countEntry struct {
		key   string
		count int
	}
	entries := make([]countEntry, 0, len(counts))
	for key, count := range counts {
		entries = append(entries, countEntry{key: key, count: count})
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].count == entries[right].count {
			return entries[left].key < entries[right].key
		}
		return entries[left].count > entries[right].count
	})

	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, fmt.Sprintf("%s:%d", entry.key, entry.count))
	}
	return strings.Join(parts, ",")
}

func collectFaithfulSyntheticIm17ChildFormDiffsForProbe(
	t *testing.T,
	source faithfulSyntheticTextCompositionProbeSource,
	dpi int,
) []faithfulSyntheticTextXObjectDiffProbe {
	t.Helper()

	im17Invocations := collectFaithfulSyntheticTopLevelXObjectInvocationsForProbe(t, source, "Im17", dpi)
	require.NotEmpty(t, im17Invocations)

	im17 := resolveFaithfulSyntheticXObjectStreamForProbe(t, source.doc.XRef(), source.resources, "Im17")
	require.NotNil(t, im17)

	decoded, err := stream.NewFromEntity(im17).Decode()
	if err != nil {
		decoded = append([]byte(nil), im17.RawBytes()...)
	}
	operators := parseFaithfulSyntheticRawOperatorsForProbe(t, decoded)

	var im17Resources *entity.Dict
	if resourcesObj := resolveSyntheticFontObjectForProbe(source.doc.XRef(), im17.Dict().Get(entity.Name("Resources"))); resourcesObj != nil {
		if dict, ok := resourcesObj.(*entity.Dict); ok {
			im17Resources = dict
		}
	}
	require.NotNil(t, im17Resources)

	childInvocations := collectFaithfulSyntheticFormXObjectInvocationsForProbe(
		t,
		source.doc.XRef(),
		im17Resources,
		operators,
		source.mediaBox,
		dpi,
		im17Invocations[0].effectiveCTM,
	)
	require.NotEmpty(t, childInvocations)

	pdfBytes := buildFaithfulSyntheticTextCompositionProbePDF(t, source)
	pdfName := fmt.Sprintf("%s_im17_children.pdf", source.name)
	popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pdfName, pdfBytes, "", dpi)
	popplerImg := loadPNGAsRGBAForProbe(t, popplerPNG)
	oursImg := loadPNGAsRGBAForProbe(t, oursPNG)

	countByName := make(map[string]int)
	results := make([]faithfulSyntheticTextXObjectDiffProbe, 0, len(childInvocations))
	for _, invocation := range childInvocations {
		bounds := invocation.pixelBounds.Intersect(popplerImg.Bounds()).Intersect(oursImg.Bounds())
		if bounds.Empty() || bounds.Dx()*bounds.Dy() < 64 {
			continue
		}
		exact, similarity := compareCroppedRGBAParityForProbe(popplerImg, oursImg, bounds)
		deltas := topMaskedRGBDeltasForProbe(popplerImg, oursImg, bounds, nil, 5)
		nonzeroDeltas, mismatchPixels, mismatchBounds := topMaskedNonzeroRGBDeltasForProbe(popplerImg, oursImg, bounds, nil, 5)
		countByName[invocation.resourceName]++
		results = append(results, faithfulSyntheticTextXObjectDiffProbe{
			resourceName:   invocation.resourceName,
			occurrence:     countByName[invocation.resourceName],
			pixelBounds:    bounds,
			exact:          exact,
			similarity:     similarity,
			deltas:         deltas,
			nonzeroDeltas:  nonzeroDeltas,
			mismatchBounds: mismatchBounds,
			mismatchPixels: mismatchPixels,
		})
	}

	sort.Slice(results, func(left, right int) bool {
		if results[left].similarity == results[right].similarity {
			if results[left].exact == results[right].exact {
				leftArea := results[left].pixelBounds.Dx() * results[left].pixelBounds.Dy()
				rightArea := results[right].pixelBounds.Dx() * results[right].pixelBounds.Dy()
				if leftArea == rightArea {
					if results[left].resourceName == results[right].resourceName {
						return results[left].occurrence < results[right].occurrence
					}
					return results[left].resourceName < results[right].resourceName
				}
				return leftArea > rightArea
			}
			return results[left].exact < results[right].exact
		}
		return results[left].similarity < results[right].similarity
	})

	return results
}

func TestFaithfulSyntheticTextCompositionPopplerAssetProbe(t *testing.T) {
	tc := faithfulSyntheticTextCompositionProbeCase{
		name:       "009_p95_all_text",
		pdfPath:    filepath.Join(getSampleDir(), "009-pdflatex-geotopo", "GeoTopo.pdf"),
		pageNumber: 95,
		dpi:        150,
	}

	source := loadFaithfulSyntheticTextCompositionProbeSource(t, tc)
	defer func() {
		require.NoError(t, source.doc.Close())
	}()

	pdfBytes := buildFaithfulSyntheticTextCompositionProbePDF(t, source)
	root := t.TempDir()
	syntheticPDFPath := filepath.Join(root, fmt.Sprintf("%s_text_only.pdf", source.name))
	require.NoError(t, os.WriteFile(syntheticPDFPath, pdfBytes, 0o644))

	t.Logf("original pdffonts:\n%s", runFaithfulSyntheticPopplerAssetCommandForProbe(
		t,
		"pdffonts",
		"-f", strconv.Itoa(tc.pageNumber),
		"-l", strconv.Itoa(tc.pageNumber),
		tc.pdfPath,
	))
	t.Logf("synthetic pdffonts:\n%s", runFaithfulSyntheticPopplerAssetCommandForProbe(t, "pdffonts", syntheticPDFPath))
	t.Logf("original pdfimages:\n%s", runFaithfulSyntheticPopplerAssetCommandForProbe(
		t,
		"pdfimages",
		"-list",
		"-f", strconv.Itoa(tc.pageNumber),
		"-l", strconv.Itoa(tc.pageNumber),
		tc.pdfPath,
	))
	t.Logf("synthetic pdfimages:\n%s", runFaithfulSyntheticPopplerAssetCommandForProbe(t, "pdfimages", "-list", syntheticPDFPath))
	t.Logf("original xobjects: %s", collectFaithfulSyntheticXObjectSummaryForProbe(t, source.doc.XRef(), source.resources))

	syntheticDoc, err := internalpdf.Open(syntheticPDFPath)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, syntheticDoc.Close())
	}()
	syntheticPage, err := syntheticDoc.GetPage(0)
	require.NoError(t, err)
	syntheticResources, err := syntheticPage.Resources()
	require.NoError(t, err)
	t.Logf("synthetic xobjects: %s", collectFaithfulSyntheticXObjectSummaryForProbe(t, syntheticDoc.XRef(), syntheticResources))
}

func faithfulSyntheticTextCompositionProbeCases() []faithfulSyntheticTextCompositionProbeCase {
	sampleDir := getSampleDir()

	return []faithfulSyntheticTextCompositionProbeCase{
		{
			name:       "009_p95_all_text",
			pdfPath:    filepath.Join(sampleDir, "009-pdflatex-geotopo", "GeoTopo.pdf"),
			pageNumber: 95,
			dpi:        150,
		},
		{
			name:       "009_p109_all_text",
			pdfPath:    filepath.Join(sampleDir, "009-pdflatex-geotopo", "GeoTopo.pdf"),
			pageNumber: 109,
			dpi:        150,
		},
		{
			name:       "015_p1_all_text",
			pdfPath:    filepath.Join(sampleDir, "015-arabic", "habibi-oneline-cmap.pdf"),
			pageNumber: 1,
			dpi:        150,
		},
	}
}

func loadFaithfulSyntheticTextCompositionProbeSource(
	t *testing.T,
	tc faithfulSyntheticTextCompositionProbeCase,
) faithfulSyntheticTextCompositionProbeSource {
	t.Helper()

	doc, err := internalpdf.Open(tc.pdfPath)
	require.NoError(t, err)

	page, err := doc.GetPage(tc.pageNumber - 1)
	require.NoError(t, err)

	resources, err := page.Resources()
	require.NoError(t, err)
	require.NotNil(t, resources)

	contents, err := page.Contents()
	require.NoError(t, err)

	evaluator := domainrenderer.NewEvaluator(doc.XRef())
	evaluator.SetResources(resources)
	require.NoError(t, evaluator.Evaluate(contents))

	operators := append([]domainrenderer.Operator(nil), evaluator.GetOperators()...)
	keptOps := filterFaithfulSyntheticTextOperatorsForProbe(operators)
	rawContent := extractFaithfulSyntheticTextRawContentForProbe(t, contents)

	fontUsage := collectFaithfulSyntheticTextFontUsageForProbe(t, doc.XRef(), resources, operators)
	operatorHist := measureFaithfulSyntheticTextOperatorHistogramForProbe(keptOps)
	droppedHist := measureFaithfulSyntheticTextDroppedOperatorHistogramForProbe(operators, keptOps)

	return faithfulSyntheticTextCompositionProbeSource{
		name:         tc.name,
		mediaBox:     page.MediaBox(),
		resources:    resources,
		operators:    operators,
		keptOps:      keptOps,
		rawContent:   rawContent,
		fontUsage:    fontUsage,
		operatorHist: operatorHist,
		droppedHist:  droppedHist,
		doc:          doc,
	}
}

func measureFaithfulSyntheticTextCompositionProbeAgainstPoppler(
	t *testing.T,
	source faithfulSyntheticTextCompositionProbeSource,
	dpi int,
) faithfulSyntheticTextCompositionProbeResult {
	t.Helper()

	pdfBytes := buildFaithfulSyntheticTextCompositionProbePDF(t, source)
	pdfName := fmt.Sprintf("%s_text_only.pdf", source.name)
	popplerPNG, oursPNG := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(t, pdfName, pdfBytes, "", dpi)
	placement := measurePNGPlacementProbeForProbe(t, popplerPNG, oursPNG, 4)
	rowHotspots, colHotspots := collectSyntheticFontDiffHotspotsForProbe(t, popplerPNG, oursPNG, 5)

	return faithfulSyntheticTextCompositionProbeResult{
		exact:       placement.originalExact,
		similarity:  placement.originalSimilarity,
		placement:   placement,
		rowHotspots: rowHotspots,
		colHotspots: colHotspots,
	}
}

func collectFaithfulSyntheticTopLevelXObjectInvocationsForProbe(
	t *testing.T,
	source faithfulSyntheticTextCompositionProbeSource,
	resourceName string,
	dpi int,
) []faithfulSyntheticTextXObjectInvocationProbe {
	t.Helper()

	operators := parseFaithfulSyntheticRawOperatorsForProbe(t, source.rawContent)
	currentCTM := [6]float64{1, 0, 0, 1, 0, 0}
	stack := make([][6]float64, 0, 16)
	invocations := make([]faithfulSyntheticTextXObjectInvocationProbe, 0, 4)

	for _, op := range operators {
		switch op.Opcode {
		case "q":
			stack = append(stack, currentCTM)
		case "Q":
			if len(stack) == 0 {
				currentCTM = [6]float64{1, 0, 0, 1, 0, 0}
				continue
			}
			currentCTM = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		case "cm":
			matrix, ok := faithfulSyntheticMatrixOperandsForProbe(op.Operands)
			if !ok {
				continue
			}
			currentCTM = faithfulSyntheticMultiplyMatrixForProbe(currentCTM, matrix)
		case "Do":
			if len(op.Operands) == 0 {
				continue
			}
			if syntheticNameValueForProbe(op.Operands[0]) != resourceName {
				continue
			}

			userBounds, effectiveCTM, ok := faithfulSyntheticXObjectUserBoundsAndCTMForProbe(
				t,
				source.doc.XRef(),
				source.resources,
				resourceName,
				currentCTM,
			)
			if !ok {
				continue
			}
			invocations = append(invocations, faithfulSyntheticTextXObjectInvocationProbe{
				resourceName: resourceName,
				userBounds:   userBounds,
				pixelBounds: faithfulSyntheticUserBoundsToPixelBoundsForProbe(
					userBounds,
					source.mediaBox,
					dpi,
				),
				effectiveCTM: effectiveCTM,
			})
		}
	}

	return invocations
}

func parseFaithfulSyntheticRawOperatorsForProbe(
	t *testing.T,
	data []byte,
) []domainrenderer.Operator {
	t.Helper()

	lexer := parser.NewLexerBytes(data)
	p := parser.NewParser(lexer, nil)
	operators := make([]domainrenderer.Operator, 0, 256)
	operands := make([]entity.Object, 0, 8)

	for {
		if p.HasBufferedObject() {
			obj, _, _, err := p.ParseObjectWithSpan()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			operands = append(operands, obj)
			continue
		}

		token, err := lexer.Peek()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if token.Type == parser.TokenEOF {
			break
		}

		if token.Type == parser.TokenKeyword && isFaithfulSyntheticTextContentOperatorForProbe(token.Value) {
			_, err := lexer.NextToken()
			require.NoError(t, err)

			op := domainrenderer.Operator{
				Opcode:   token.Value,
				Operands: append([]entity.Object(nil), operands...),
			}
			operators = append(operators, op)
			operands = operands[:0]
			continue
		}

		obj, _, _, err := p.ParseObjectWithSpan()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		operands = append(operands, obj)
	}

	return operators
}

func faithfulSyntheticMatrixOperandsForProbe(operands []entity.Object) ([6]float64, bool) {
	if len(operands) < 6 {
		return [6]float64{}, false
	}

	var matrix [6]float64
	for idx := 0; idx < 6; idx++ {
		value, ok := faithfulSyntheticNumberValueForProbe(operands[idx])
		if !ok {
			return [6]float64{}, false
		}
		matrix[idx] = value
	}
	return matrix, true
}

func faithfulSyntheticNumberValueForProbe(obj entity.Object) (float64, bool) {
	switch typed := obj.(type) {
	case *entity.Integer:
		return float64(typed.Value()), true
	case *entity.Real:
		return typed.Value(), true
	default:
		return 0, false
	}
}

func faithfulSyntheticXObjectUserBoundsForProbe(
	t *testing.T,
	xref entity.XRef,
	resources *entity.Dict,
	resourceName string,
	currentCTM [6]float64,
) ([4]float64, bool) {
	userBounds, _, ok := faithfulSyntheticXObjectUserBoundsAndCTMForProbe(
		t,
		xref,
		resources,
		resourceName,
		currentCTM,
	)
	return userBounds, ok
}

func faithfulSyntheticXObjectUserBoundsAndCTMForProbe(
	t *testing.T,
	xref entity.XRef,
	resources *entity.Dict,
	resourceName string,
	currentCTM [6]float64,
) ([4]float64, [6]float64, bool) {
	t.Helper()

	if resources == nil {
		return [4]float64{}, [6]float64{}, false
	}

	xobjects, ok := resolveSyntheticFontDictForProbe(xref, resources.Get(entity.Name("XObject")))
	if !ok || xobjects == nil {
		return [4]float64{}, [6]float64{}, false
	}

	xobject := resolveSyntheticFontObjectForProbe(xref, xobjects.Get(entity.Name(resourceName)))
	streamObj, ok := xobject.(*entity.Stream)
	if !ok {
		return [4]float64{}, [6]float64{}, false
	}
	if syntheticNameValueForProbe(streamObj.Dict().Get(entity.Name("Subtype"))) != "Form" {
		return [4]float64{}, [6]float64{}, false
	}

	bbox, ok := faithfulSyntheticBBoxArrayForProbe(streamObj.Dict().Get(entity.Name("BBox")))
	if !ok {
		return [4]float64{}, [6]float64{}, false
	}

	formMatrix := [6]float64{1, 0, 0, 1, 0, 0}
	if rawMatrix := streamObj.Dict().Get(entity.Name("Matrix")); rawMatrix != nil {
		if matrix, ok := faithfulSyntheticMatrixArrayForProbe(rawMatrix); ok {
			formMatrix = matrix
		}
	}

	effectiveCTM := faithfulSyntheticMultiplyMatrixForProbe(currentCTM, formMatrix)
	return faithfulSyntheticTransformRectForProbe(effectiveCTM, bbox), effectiveCTM, true
}

func resolveFaithfulSyntheticXObjectStreamForProbe(
	t *testing.T,
	xref entity.XRef,
	resources *entity.Dict,
	resourceName string,
) *entity.Stream {
	t.Helper()

	if resources == nil {
		return nil
	}
	xobjects, ok := resolveSyntheticFontDictForProbe(xref, resources.Get(entity.Name("XObject")))
	if !ok || xobjects == nil {
		return nil
	}
	xobject := resolveSyntheticFontObjectForProbe(xref, xobjects.Get(entity.Name(resourceName)))
	streamObj, _ := xobject.(*entity.Stream)
	return streamObj
}

func faithfulSyntheticBBoxArrayForProbe(obj entity.Object) ([4]float64, bool) {
	array, ok := obj.(*entity.Array)
	if !ok || array.Len() != 4 {
		return [4]float64{}, false
	}

	var out [4]float64
	for idx := 0; idx < 4; idx++ {
		value, ok := faithfulSyntheticNumberValueForProbe(array.Get(idx))
		if !ok {
			return [4]float64{}, false
		}
		out[idx] = value
	}
	return out, true
}

func faithfulSyntheticMatrixArrayForProbe(obj entity.Object) ([6]float64, bool) {
	array, ok := obj.(*entity.Array)
	if !ok || array.Len() != 6 {
		return [6]float64{}, false
	}

	var out [6]float64
	for idx := 0; idx < 6; idx++ {
		value, ok := faithfulSyntheticNumberValueForProbe(array.Get(idx))
		if !ok {
			return [6]float64{}, false
		}
		out[idx] = value
	}
	return out, true
}

func faithfulSyntheticMultiplyMatrixForProbe(a, b [6]float64) [6]float64 {
	return [6]float64{
		a[0]*b[0] + a[2]*b[1],
		a[1]*b[0] + a[3]*b[1],
		a[0]*b[2] + a[2]*b[3],
		a[1]*b[2] + a[3]*b[3],
		a[0]*b[4] + a[2]*b[5] + a[4],
		a[1]*b[4] + a[3]*b[5] + a[5],
	}
}

func faithfulSyntheticTransformRectForProbe(matrix [6]float64, rect [4]float64) [4]float64 {
	points := [][2]float64{
		{rect[0], rect[1]},
		{rect[2], rect[1]},
		{rect[2], rect[3]},
		{rect[0], rect[3]},
	}

	minX := math.MaxFloat64
	minY := math.MaxFloat64
	maxX := -math.MaxFloat64
	maxY := -math.MaxFloat64
	for _, point := range points {
		x := matrix[0]*point[0] + matrix[2]*point[1] + matrix[4]
		y := matrix[1]*point[0] + matrix[3]*point[1] + matrix[5]
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
	}

	return [4]float64{minX, minY, maxX, maxY}
}

func faithfulSyntheticUserBoundsToPixelBoundsForProbe(
	userBounds [4]float64,
	mediaBox [4]float64,
	dpi int,
) image.Rectangle {
	scale := float64(dpi) / 72.0

	minX := int(math.Floor((userBounds[0] - mediaBox[0]) * scale))
	maxX := int(math.Ceil((userBounds[2] - mediaBox[0]) * scale))
	minY := int(math.Floor((mediaBox[3] - userBounds[3]) * scale))
	maxY := int(math.Ceil((mediaBox[3] - userBounds[1]) * scale))

	return image.Rect(minX, minY, maxX, maxY).Inset(-2)
}

func unionFaithfulSyntheticPixelBoundsForProbe(
	invocations []faithfulSyntheticTextXObjectInvocationProbe,
) image.Rectangle {
	if len(invocations) == 0 {
		return image.Rectangle{}
	}

	bounds := invocations[0].pixelBounds
	for _, invocation := range invocations[1:] {
		bounds = bounds.Union(invocation.pixelBounds)
	}
	return bounds
}

func buildFaithfulSyntheticTextCompositionProbePDF(
	t *testing.T,
	source faithfulSyntheticTextCompositionProbeSource,
) []byte {
	return buildFaithfulSyntheticTextCompositionProbePDFWithContent(t, source, source.rawContent, source.keptOps)
}

func buildFaithfulSyntheticTextCompositionProbePDFWithOperators(
	t *testing.T,
	source faithfulSyntheticTextCompositionProbeSource,
	operators []domainrenderer.Operator,
) []byte {
	return buildFaithfulSyntheticTextCompositionProbePDFWithContent(t, source, nil, operators)
}

func buildFaithfulSyntheticTextCompositionProbePDFWithContent(
	t *testing.T,
	source faithfulSyntheticTextCompositionProbeSource,
	content []byte,
	operators []domainrenderer.Operator,
) []byte {
	return buildFaithfulSyntheticTextCompositionProbePDFWithImportedMutator(t, source, content, operators, nil)
}

func buildFaithfulSyntheticTextCompositionProbePDFWithImportedMutator(
	t *testing.T,
	source faithfulSyntheticTextCompositionProbeSource,
	content []byte,
	operators []domainrenderer.Operator,
	mutate func(t *testing.T, obj entity.Object) entity.Object,
) []byte {
	t.Helper()

	importer := newSyntheticFontGraphImporter(t, source.doc.XRef(), 5)
	resourcesObjectNumber := importer.importRoot(source.resources)
	if len(content) == 0 {
		content = buildFaithfulSyntheticTextCompositionContentForProbe(operators)
	}

	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [%s %s %s %s] /Resources %d 0 R /Contents 4 0 R >>",
			formatFaithfulSyntheticTextNumberForProbe(source.mediaBox[0]),
			formatFaithfulSyntheticTextNumberForProbe(source.mediaBox[1]),
			formatFaithfulSyntheticTextNumberForProbe(source.mediaBox[2]),
			formatFaithfulSyntheticTextNumberForProbe(source.mediaBox[3]),
			resourcesObjectNumber,
		)),
		syntheticStreamObject(fmt.Sprintf("<< /Length %d >>", len(content)), content),
	}

	for _, imported := range importer.objects() {
		importedObject := imported.object
		if mutate != nil {
			importedObject = mutate(t, imported.object)
		}
		objects = append(objects, serializeSyntheticFontEntityObject(importedObject))
	}

	return buildSyntheticPDF(objects)
}

func buildFaithfulSyntheticResourceContentProbePDF(
	t *testing.T,
	source faithfulSyntheticTextCompositionProbeSource,
	resources *entity.Dict,
	content []byte,
) []byte {
	t.Helper()

	importer := newSyntheticFontGraphImporter(t, source.doc.XRef(), 5)
	resourcesObjectNumber := importer.importRoot(resources)

	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [%s %s %s %s] /Resources %d 0 R /Contents 4 0 R >>",
			formatFaithfulSyntheticTextNumberForProbe(source.mediaBox[0]),
			formatFaithfulSyntheticTextNumberForProbe(source.mediaBox[1]),
			formatFaithfulSyntheticTextNumberForProbe(source.mediaBox[2]),
			formatFaithfulSyntheticTextNumberForProbe(source.mediaBox[3]),
			resourcesObjectNumber,
		)),
		syntheticStreamObject(fmt.Sprintf("<< /Length %d >>", len(content)), content),
	}

	for _, imported := range importer.objects() {
		objects = append(objects, serializeSyntheticFontEntityObject(imported.object))
	}
	return buildSyntheticPDF(objects)
}

func buildFaithfulSyntheticStandaloneXObjectProbePDF(
	t *testing.T,
	source faithfulSyntheticTextCompositionProbeSource,
	resourceName string,
	xObject entity.Object,
	currentCTM [6]float64,
) []byte {
	t.Helper()

	importer := newSyntheticFontGraphImporter(t, source.doc.XRef(), 5)
	xObjectNumber := importer.importRoot(xObject)

	xObjectResources := entity.NewDict()
	xObjectResources.Set(entity.Name(resourceName), entity.NewRef(uint32(xObjectNumber), 0))
	pageResources := entity.NewDict()
	pageResources.Set(entity.Name("XObject"), xObjectResources)
	resourcesObjectNumber := importer.importRoot(pageResources)

	content := buildFaithfulSyntheticSingleDoContentForProbe(resourceName, currentCTM)
	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [%s %s %s %s] /Resources %d 0 R /Contents 4 0 R >>",
			formatFaithfulSyntheticTextNumberForProbe(source.mediaBox[0]),
			formatFaithfulSyntheticTextNumberForProbe(source.mediaBox[1]),
			formatFaithfulSyntheticTextNumberForProbe(source.mediaBox[2]),
			formatFaithfulSyntheticTextNumberForProbe(source.mediaBox[3]),
			resourcesObjectNumber,
		)),
		syntheticStreamObject(fmt.Sprintf("<< /Length %d >>", len(content)), content),
	}

	for _, imported := range importer.objects() {
		objects = append(objects, serializeSyntheticFontEntityObject(imported.object))
	}
	return buildSyntheticPDF(objects)
}

func buildFaithfulSyntheticTextCompositionContentForProbe(
	operators []domainrenderer.Operator,
) []byte {
	var builder strings.Builder
	for _, op := range operators {
		builder.WriteString(serializeFaithfulSyntheticTextOperatorForProbe(op))
	}
	return []byte(builder.String())
}

func filterFaithfulSyntheticTextOperatorsForProbe(
	operators []domainrenderer.Operator,
) []domainrenderer.Operator {
	out := make([]domainrenderer.Operator, 0, len(operators))
	for _, op := range operators {
		if !shouldKeepFaithfulSyntheticTextOperatorForProbe(op.Opcode) {
			continue
		}
		out = append(out, op)
	}
	return out
}

func collectFaithfulSyntheticFormXObjectInvocationsForProbe(
	t *testing.T,
	xref entity.XRef,
	resources *entity.Dict,
	operators []domainrenderer.Operator,
	mediaBox [4]float64,
	dpi int,
	initialCTM [6]float64,
) []faithfulSyntheticTextXObjectInvocationProbe {
	t.Helper()

	currentCTM := initialCTM
	stack := make([][6]float64, 0, 32)
	invocations := make([]faithfulSyntheticTextXObjectInvocationProbe, 0, 64)

	for _, op := range operators {
		switch op.Opcode {
		case "q":
			stack = append(stack, currentCTM)
		case "Q":
			if len(stack) == 0 {
				currentCTM = initialCTM
				continue
			}
			currentCTM = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		case "cm":
			matrix, ok := faithfulSyntheticMatrixOperandsForProbe(op.Operands)
			if !ok {
				continue
			}
			currentCTM = faithfulSyntheticMultiplyMatrixForProbe(currentCTM, matrix)
		case "Do":
			resourceName := syntheticNameValueForProbe(firstSyntheticOperandForProbe(op.Operands))
			if resourceName == "" {
				continue
			}

			userBounds, effectiveCTM, ok := faithfulSyntheticXObjectUserBoundsAndCTMForProbe(
				t,
				xref,
				resources,
				resourceName,
				currentCTM,
			)
			if !ok {
				continue
			}

			invocations = append(invocations, faithfulSyntheticTextXObjectInvocationProbe{
				resourceName: resourceName,
				userBounds:   userBounds,
				pixelBounds: faithfulSyntheticUserBoundsToPixelBoundsForProbe(
					userBounds,
					mediaBox,
					dpi,
				),
				effectiveCTM: effectiveCTM,
			})
		}
	}

	return invocations
}

func firstSyntheticOperandForProbe(operands []entity.Object) entity.Object {
	if len(operands) == 0 {
		return nil
	}
	return operands[0]
}

func compareCroppedRGBAParityForProbe(
	popplerImg *image.RGBA,
	oursImg *image.RGBA,
	bounds image.Rectangle,
) (float64, float64) {
	popplerCrop := cropRGBAForProbe(popplerImg, bounds)
	oursCrop := cropRGBAForProbe(oursImg, bounds)
	if !popplerCrop.Bounds().Eq(oursCrop.Bounds()) || popplerCrop.Bounds().Empty() {
		return 0, 0
	}

	totalPixels := popplerCrop.Bounds().Dx() * popplerCrop.Bounds().Dy()
	if totalPixels == 0 {
		return 0, 0
	}

	matchingPixels := 0
	totalDiff := 0.0
	for y := popplerCrop.Bounds().Min.Y; y < popplerCrop.Bounds().Max.Y; y++ {
		for x := popplerCrop.Bounds().Min.X; x < popplerCrop.Bounds().Max.X; x++ {
			left := popplerCrop.RGBAAt(x, y)
			right := oursCrop.RGBAAt(x, y)
			if left.R == right.R && left.G == right.G && left.B == right.B {
				matchingPixels++
			}
			totalDiff += math.Abs(float64(left.R)-float64(right.R)) +
				math.Abs(float64(left.G)-float64(right.G)) +
				math.Abs(float64(left.B)-float64(right.B))
		}
	}

	exactPercent := float64(matchingPixels) * 100.0 / float64(totalPixels)
	similarityPercent := 100.0 * (1.0 - totalDiff/(float64(totalPixels)*255.0*3.0))
	if similarityPercent < 0 {
		similarityPercent = 0
	}
	return exactPercent, similarityPercent
}

func compareMaskedRGBAParityForProbe(
	popplerImg *image.RGBA,
	oursImg *image.RGBA,
	bounds image.Rectangle,
	mask []bool,
) (float64, float64, int) {
	bounds = bounds.Intersect(popplerImg.Bounds()).Intersect(oursImg.Bounds())
	if bounds.Empty() {
		return 0, 0, 0
	}

	width := bounds.Dx()
	height := bounds.Dy()
	if width == 0 || height == 0 {
		return 0, 0, 0
	}
	if mask != nil && len(mask) != width*height {
		return 0, 0, 0
	}

	matchingPixels := 0
	totalDiff := 0.0
	totalPixels := 0

	for localY := 0; localY < height; localY++ {
		for localX := 0; localX < width; localX++ {
			idx := localY*width + localX
			if mask != nil && !mask[idx] {
				continue
			}

			left := popplerImg.RGBAAt(bounds.Min.X+localX, bounds.Min.Y+localY)
			right := oursImg.RGBAAt(bounds.Min.X+localX, bounds.Min.Y+localY)
			totalPixels++
			if left.R == right.R && left.G == right.G && left.B == right.B {
				matchingPixels++
			}
			totalDiff += math.Abs(float64(left.R)-float64(right.R)) +
				math.Abs(float64(left.G)-float64(right.G)) +
				math.Abs(float64(left.B)-float64(right.B))
		}
	}

	if totalPixels == 0 {
		return 0, 0, 0
	}

	exactPercent := float64(matchingPixels) * 100.0 / float64(totalPixels)
	similarityPercent := 100.0 * (1.0 - totalDiff/(float64(totalPixels)*255.0*3.0))
	if similarityPercent < 0 {
		similarityPercent = 0
	}
	return exactPercent, similarityPercent, totalPixels
}

func topMaskedRGBDeltasForProbe(
	popplerImg *image.RGBA,
	oursImg *image.RGBA,
	bounds image.Rectangle,
	mask []bool,
	limit int,
) string {
	bounds = bounds.Intersect(popplerImg.Bounds()).Intersect(oursImg.Bounds())
	if bounds.Empty() || limit <= 0 {
		return ""
	}

	width := bounds.Dx()
	height := bounds.Dy()
	if mask != nil && len(mask) != width*height {
		return ""
	}

	counts := make(map[string]int)
	for localY := 0; localY < height; localY++ {
		for localX := 0; localX < width; localX++ {
			idx := localY*width + localX
			if mask != nil && !mask[idx] {
				continue
			}

			left := popplerImg.RGBAAt(bounds.Min.X+localX, bounds.Min.Y+localY)
			right := oursImg.RGBAAt(bounds.Min.X+localX, bounds.Min.Y+localY)
			key := fmt.Sprintf("%+d/%+d/%+d", int(right.R)-int(left.R), int(right.G)-int(left.G), int(right.B)-int(left.B))
			counts[key]++
		}
	}
	if len(counts) == 0 {
		return ""
	}

	type deltaCount struct {
		key   string
		count int
	}
	deltas := make([]deltaCount, 0, len(counts))
	for key, count := range counts {
		deltas = append(deltas, deltaCount{key: key, count: count})
	}
	sort.Slice(deltas, func(left, right int) bool {
		if deltas[left].count == deltas[right].count {
			return deltas[left].key < deltas[right].key
		}
		return deltas[left].count > deltas[right].count
	})

	if len(deltas) > limit {
		deltas = deltas[:limit]
	}
	parts := make([]string, 0, len(deltas))
	for _, delta := range deltas {
		parts = append(parts, fmt.Sprintf("%s:%d", delta.key, delta.count))
	}
	return strings.Join(parts, ",")
}

func topMaskedNonzeroRGBDeltasForProbe(
	popplerImg *image.RGBA,
	oursImg *image.RGBA,
	bounds image.Rectangle,
	mask []bool,
	limit int,
) (string, int, image.Rectangle) {
	bounds = bounds.Intersect(popplerImg.Bounds()).Intersect(oursImg.Bounds())
	if bounds.Empty() || limit <= 0 {
		return "", 0, image.Rectangle{}
	}

	width := bounds.Dx()
	height := bounds.Dy()
	if mask != nil && len(mask) != width*height {
		return "", 0, image.Rectangle{}
	}

	counts := make(map[string]int)
	mismatchBounds := image.Rectangle{}
	mismatchPixels := 0
	for localY := 0; localY < height; localY++ {
		for localX := 0; localX < width; localX++ {
			idx := localY*width + localX
			if mask != nil && !mask[idx] {
				continue
			}

			x := bounds.Min.X + localX
			y := bounds.Min.Y + localY
			left := popplerImg.RGBAAt(x, y)
			right := oursImg.RGBAAt(x, y)
			dr := int(right.R) - int(left.R)
			dg := int(right.G) - int(left.G)
			db := int(right.B) - int(left.B)
			if dr == 0 && dg == 0 && db == 0 {
				continue
			}

			key := fmt.Sprintf("%+d/%+d/%+d", dr, dg, db)
			counts[key]++
			mismatchPixels++
			pixelRect := image.Rect(x, y, x+1, y+1)
			if mismatchBounds.Empty() {
				mismatchBounds = pixelRect
			} else {
				mismatchBounds = mismatchBounds.Union(pixelRect)
			}
		}
	}
	if len(counts) == 0 {
		return "", 0, image.Rectangle{}
	}

	type deltaCount struct {
		key   string
		count int
	}
	deltas := make([]deltaCount, 0, len(counts))
	for key, count := range counts {
		deltas = append(deltas, deltaCount{key: key, count: count})
	}
	sort.Slice(deltas, func(left, right int) bool {
		if deltas[left].count == deltas[right].count {
			return deltas[left].key < deltas[right].key
		}
		return deltas[left].count > deltas[right].count
	})

	if len(deltas) > limit {
		deltas = deltas[:limit]
	}
	parts := make([]string, 0, len(deltas))
	for _, delta := range deltas {
		parts = append(parts, fmt.Sprintf("%s:%d", delta.key, delta.count))
	}
	return strings.Join(parts, ","), mismatchPixels, mismatchBounds
}

func sampleMaskedMismatchedRGBForProbe(
	popplerImg *image.RGBA,
	oursImg *image.RGBA,
	bounds image.Rectangle,
	mask []bool,
	limit int,
) string {
	bounds = bounds.Intersect(popplerImg.Bounds()).Intersect(oursImg.Bounds())
	if bounds.Empty() || limit <= 0 {
		return ""
	}

	width := bounds.Dx()
	height := bounds.Dy()
	if mask != nil && len(mask) != width*height {
		return ""
	}

	parts := make([]string, 0, limit)
	for localY := 0; localY < height; localY++ {
		for localX := 0; localX < width; localX++ {
			idx := localY*width + localX
			if mask != nil && !mask[idx] {
				continue
			}

			x := bounds.Min.X + localX
			y := bounds.Min.Y + localY
			left := popplerImg.RGBAAt(x, y)
			right := oursImg.RGBAAt(x, y)
			if left.R == right.R && left.G == right.G && left.B == right.B {
				continue
			}

			alphaNote := ""
			if left.G == right.G {
				alpha := 255 - int(left.G)
				alphaNote = fmt.Sprintf(
					" alpha_from_green=%d pop_src_range=(R:%s B:%s) ours_src_range=(R:%s B:%s)",
					alpha,
					opaqueWhiteBlendSourceRangeForProbe(int(left.R), alpha),
					opaqueWhiteBlendSourceRangeForProbe(int(left.B), alpha),
					opaqueWhiteBlendSourceRangeForProbe(int(right.R), alpha),
					opaqueWhiteBlendSourceRangeForProbe(int(right.B), alpha),
				)
			}

			parts = append(parts, fmt.Sprintf(
				"(%d,%d) poppler=(%d,%d,%d) ours=(%d,%d,%d) delta=(%+d,%+d,%+d)%s",
				x,
				y,
				left.R,
				left.G,
				left.B,
				right.R,
				right.G,
				right.B,
				int(right.R)-int(left.R),
				int(right.G)-int(left.G),
				int(right.B)-int(left.B),
				alphaNote,
			))
			if len(parts) >= limit {
				return strings.Join(parts, ";")
			}
		}
	}

	return strings.Join(parts, ";")
}

func sampleMaskedMismatchedRGBWithBackdropForProbe(
	popplerImg *image.RGBA,
	oursImg *image.RGBA,
	backdropImg *image.RGBA,
	bounds image.Rectangle,
	mask []bool,
	limit int,
) string {
	if backdropImg == nil {
		return ""
	}
	bounds = bounds.Intersect(popplerImg.Bounds()).Intersect(oursImg.Bounds()).Intersect(backdropImg.Bounds())
	if bounds.Empty() || limit <= 0 {
		return ""
	}

	width := bounds.Dx()
	height := bounds.Dy()
	if mask != nil && len(mask) != width*height {
		return ""
	}

	parts := make([]string, 0, limit)
	for localY := 0; localY < height; localY++ {
		for localX := 0; localX < width; localX++ {
			idx := localY*width + localX
			if mask != nil && !mask[idx] {
				continue
			}

			x := bounds.Min.X + localX
			y := bounds.Min.Y + localY
			left := popplerImg.RGBAAt(x, y)
			right := oursImg.RGBAAt(x, y)
			if left.R == right.R && left.G == right.G && left.B == right.B {
				continue
			}

			backdrop := backdropImg.RGBAAt(x, y)
			alphaCandidates := opaqueBlendAlphaCandidatesForProbe(int(backdrop.G), 0, int(left.G))
			parts = append(parts, fmt.Sprintf(
				"(%d,%d) backdrop=(%d,%d,%d) poppler=(%d,%d,%d) ours=(%d,%d,%d) delta=(%+d,%+d,%+d) alpha_candidates=%s pop_src_range=(R:%s B:%s) ours_src_range=(R:%s B:%s)",
				x,
				y,
				backdrop.R,
				backdrop.G,
				backdrop.B,
				left.R,
				left.G,
				left.B,
				right.R,
				right.G,
				right.B,
				int(right.R)-int(left.R),
				int(right.G)-int(left.G),
				int(right.B)-int(left.B),
				formatIntRangeForProbe(alphaCandidates),
				opaqueBlendSourceRangeForProbe(int(backdrop.R), int(left.R), alphaCandidates),
				opaqueBlendSourceRangeForProbe(int(backdrop.B), int(left.B), alphaCandidates),
				opaqueBlendSourceRangeForProbe(int(backdrop.R), int(right.R), alphaCandidates),
				opaqueBlendSourceRangeForProbe(int(backdrop.B), int(right.B), alphaCandidates),
			))
			if len(parts) >= limit {
				return strings.Join(parts, ";")
			}
		}
	}

	return strings.Join(parts, ";")
}

func opaqueWhiteBlendSourceRangeForProbe(result, alpha int) string {
	if alpha <= 0 || alpha > 255 {
		return "n/a"
	}

	minSource := -1
	maxSource := -1
	for source := 0; source <= 255; source++ {
		blended := ((255-alpha)*255 + alpha*source) / 255
		if blended != result {
			continue
		}
		if minSource == -1 {
			minSource = source
		}
		maxSource = source
	}
	if minSource == -1 {
		return "none"
	}
	if minSource == maxSource {
		return fmt.Sprintf("%d", minSource)
	}
	return fmt.Sprintf("%d-%d", minSource, maxSource)
}

func opaqueBlendAlphaCandidatesForProbe(backdrop, source, result int) []int {
	candidates := make([]int, 0, 4)
	for alpha := 0; alpha <= 255; alpha++ {
		if opaqueBlendChannelForProbe(backdrop, source, alpha) == result {
			candidates = append(candidates, alpha)
		}
	}
	return candidates
}

func opaqueBlendSourceRangeForProbe(backdrop, result int, alphaCandidates []int) string {
	if len(alphaCandidates) == 0 {
		return "none"
	}

	minSource := -1
	maxSource := -1
	for _, alpha := range alphaCandidates {
		if alpha <= 0 {
			continue
		}
		for source := 0; source <= 255; source++ {
			if opaqueBlendChannelForProbe(backdrop, source, alpha) != result {
				continue
			}
			if minSource == -1 || source < minSource {
				minSource = source
			}
			if source > maxSource {
				maxSource = source
			}
		}
	}
	if minSource == -1 {
		return "none"
	}
	if minSource == maxSource {
		return fmt.Sprintf("%d", minSource)
	}
	return fmt.Sprintf("%d-%d", minSource, maxSource)
}

func opaqueBlendChannelForProbe(backdrop, source, alpha int) int {
	return ((255-alpha)*backdrop + alpha*source) / 255
}

func formatIntRangeForProbe(values []int) string {
	if len(values) == 0 {
		return "none"
	}
	if len(values) == 1 {
		return fmt.Sprintf("%d", values[0])
	}

	parts := make([]string, 0, len(values))
	start := values[0]
	previous := values[0]
	for _, value := range values[1:] {
		if value == previous+1 {
			previous = value
			continue
		}
		parts = append(parts, formatClosedIntRangeForProbe(start, previous))
		start = value
		previous = value
	}
	parts = append(parts, formatClosedIntRangeForProbe(start, previous))
	return strings.Join(parts, "|")
}

func formatClosedIntRangeForProbe(start, end int) string {
	if start == end {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d-%d", start, end)
}

func measureFaithfulSyntheticBoundaryBandMetricsForProbe(
	popplerImg *image.RGBA,
	oursImg *image.RGBA,
	bounds image.Rectangle,
) faithfulSyntheticBoundaryBandMetrics {
	bounds = bounds.Intersect(popplerImg.Bounds()).Intersect(oursImg.Bounds())
	if bounds.Empty() {
		return faithfulSyntheticBoundaryBandMetrics{}
	}

	supportMask := buildUnionNonWhiteMaskForProbe(popplerImg, oursImg, bounds)
	bandRadius := 2
	coreMask := erodeMaskForProbe(supportMask, bounds.Dx(), bounds.Dy(), bandRadius)
	if countMaskPixelsForProbe(coreMask) == 0 && bandRadius > 1 {
		bandRadius = 1
		coreMask = erodeMaskForProbe(supportMask, bounds.Dx(), bounds.Dy(), bandRadius)
	}
	boundaryMask := subtractMaskForProbe(supportMask, coreMask)

	supportExact, supportSimilarity, supportPixels := compareMaskedRGBAParityForProbe(popplerImg, oursImg, bounds, supportMask)
	coreExact, coreSimilarity, corePixels := compareMaskedRGBAParityForProbe(popplerImg, oursImg, bounds, coreMask)
	boundaryExact, boundarySimilarity, boundaryPixels := compareMaskedRGBAParityForProbe(popplerImg, oursImg, bounds, boundaryMask)

	return faithfulSyntheticBoundaryBandMetrics{
		bandRadius: bandRadius,
		support: faithfulSyntheticMaskedParityMetrics{
			pixels:     supportPixels,
			exact:      supportExact,
			similarity: supportSimilarity,
		},
		core: faithfulSyntheticMaskedParityMetrics{
			pixels:     corePixels,
			exact:      coreExact,
			similarity: coreSimilarity,
		},
		boundary: faithfulSyntheticMaskedParityMetrics{
			pixels:     boundaryPixels,
			exact:      boundaryExact,
			similarity: boundarySimilarity,
		},
	}
}

func buildUnionNonWhiteMaskForProbe(
	popplerImg *image.RGBA,
	oursImg *image.RGBA,
	bounds image.Rectangle,
) []bool {
	bounds = bounds.Intersect(popplerImg.Bounds()).Intersect(oursImg.Bounds())
	if bounds.Empty() {
		return nil
	}

	width := bounds.Dx()
	height := bounds.Dy()
	mask := make([]bool, width*height)
	for localY := 0; localY < height; localY++ {
		for localX := 0; localX < width; localX++ {
			left := popplerImg.RGBAAt(bounds.Min.X+localX, bounds.Min.Y+localY)
			right := oursImg.RGBAAt(bounds.Min.X+localX, bounds.Min.Y+localY)
			mask[localY*width+localX] = isNonWhitePixelForProbe(left) || isNonWhitePixelForProbe(right)
		}
	}
	return mask
}

func isNonWhitePixelForProbe(pixel color.RGBA) bool {
	if pixel.A == 0 {
		return false
	}
	return pixel.R != 255 || pixel.G != 255 || pixel.B != 255
}

func erodeMaskForProbe(mask []bool, width, height, radius int) []bool {
	if len(mask) != width*height {
		return nil
	}
	if radius <= 0 {
		out := make([]bool, len(mask))
		copy(out, mask)
		return out
	}

	out := make([]bool, len(mask))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			if !mask[idx] {
				continue
			}

			keep := true
			for offsetY := -radius; offsetY <= radius && keep; offsetY++ {
				for offsetX := -radius; offsetX <= radius; offsetX++ {
					neighborX := x + offsetX
					neighborY := y + offsetY
					if neighborX < 0 || neighborX >= width || neighborY < 0 || neighborY >= height {
						keep = false
						break
					}
					if !mask[neighborY*width+neighborX] {
						keep = false
						break
					}
				}
			}
			out[idx] = keep
		}
	}
	return out
}

func subtractMaskForProbe(left []bool, right []bool) []bool {
	if len(left) != len(right) {
		return nil
	}
	out := make([]bool, len(left))
	for idx := range left {
		out[idx] = left[idx] && !right[idx]
	}
	return out
}

func intersectMasksForProbe(left []bool, right []bool) []bool {
	if len(left) != len(right) {
		return nil
	}
	out := make([]bool, len(left))
	for idx := range left {
		out[idx] = left[idx] && right[idx]
	}
	return out
}

func countMaskPixelsForProbe(mask []bool) int {
	count := 0
	for _, value := range mask {
		if value {
			count++
		}
	}
	return count
}

func faithfulSyntheticFormMatrixForProbe(streamObj *entity.Stream) [6]float64 {
	if streamObj == nil {
		return [6]float64{1, 0, 0, 1, 0, 0}
	}
	if matrix, ok := faithfulSyntheticMatrixArrayForProbe(streamObj.Dict().Get(entity.Name("Matrix"))); ok {
		return matrix
	}
	return [6]float64{1, 0, 0, 1, 0, 0}
}

func faithfulSyntheticInverseMatrixForProbe(matrix [6]float64) ([6]float64, bool) {
	det := matrix[0]*matrix[3] - matrix[1]*matrix[2]
	if det == 0 {
		return [6]float64{}, false
	}

	inverse := [6]float64{
		matrix[3] / det,
		-matrix[1] / det,
		-matrix[2] / det,
		matrix[0] / det,
		(matrix[2]*matrix[5] - matrix[3]*matrix[4]) / det,
		(matrix[1]*matrix[4] - matrix[0]*matrix[5]) / det,
	}
	return inverse, true
}

func faithfulSyntheticXObjectInvocationForOccurrenceForProbe(
	invocations []faithfulSyntheticTextXObjectInvocationProbe,
	resourceName string,
	occurrence int,
) (faithfulSyntheticTextXObjectInvocationProbe, bool) {
	if occurrence <= 0 {
		return faithfulSyntheticTextXObjectInvocationProbe{}, false
	}

	count := 0
	for _, invocation := range invocations {
		if invocation.resourceName != resourceName {
			continue
		}
		count++
		if count == occurrence {
			return invocation, true
		}
	}
	return faithfulSyntheticTextXObjectInvocationProbe{}, false
}

func faithfulSyntheticXObjectOperatorIndexForOccurrenceForProbe(
	operators []domainrenderer.Operator,
	resourceName string,
	occurrence int,
) (int, bool) {
	if occurrence <= 0 {
		return 0, false
	}

	count := 0
	for idx, op := range operators {
		if op.Opcode != "Do" {
			continue
		}
		if syntheticNameValueForProbe(firstSyntheticOperandForProbe(op.Operands)) != resourceName {
			continue
		}
		count++
		if count == occurrence {
			return idx, true
		}
	}
	return 0, false
}

func faithfulSyntheticEnclosingScopeStartForProbe(operators []domainrenderer.Operator, targetIndex int) int {
	if targetIndex <= 0 {
		return 0
	}
	if targetIndex > len(operators) {
		targetIndex = len(operators)
	}

	stack := make([]int, 0, 16)
	for idx := 0; idx < targetIndex; idx++ {
		switch operators[idx].Opcode {
		case "q":
			stack = append(stack, idx)
		case "Q":
			if len(stack) == 0 {
				continue
			}
			stack = stack[:len(stack)-1]
		}
	}
	if len(stack) == 0 {
		return 0
	}
	return stack[len(stack)-1]
}

func faithfulSyntheticCTMBeforeOperatorForProbe(
	operators []domainrenderer.Operator,
	initialCTM [6]float64,
	targetIndex int,
) [6]float64 {
	if targetIndex <= 0 {
		return initialCTM
	}
	if targetIndex > len(operators) {
		targetIndex = len(operators)
	}

	currentCTM := initialCTM
	stack := make([][6]float64, 0, 32)
	for idx := 0; idx < targetIndex; idx++ {
		switch operators[idx].Opcode {
		case "q":
			stack = append(stack, currentCTM)
		case "Q":
			if len(stack) == 0 {
				currentCTM = initialCTM
				continue
			}
			currentCTM = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		case "cm":
			matrix, ok := faithfulSyntheticMatrixOperandsForProbe(operators[idx].Operands)
			if !ok {
				continue
			}
			currentCTM = faithfulSyntheticMultiplyMatrixForProbe(currentCTM, matrix)
		}
	}
	return currentCTM
}

func buildFaithfulSyntheticSingleDoContentForProbe(resourceName string, currentCTM [6]float64) []byte {
	var content strings.Builder
	content.WriteString("q\n")
	writeFaithfulSyntheticMatrixConcatForProbe(&content, currentCTM)
	content.WriteByte('/')
	content.WriteString(strings.TrimPrefix(resourceName, "/"))
	content.WriteString(" Do\nQ\n")
	return []byte(content.String())
}

func buildFaithfulSyntheticParentContextReplayContentForProbe(
	initialCTM [6]float64,
	operators []domainrenderer.Operator,
	startIndex int,
	endIndex int,
) []byte {
	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex >= len(operators) {
		endIndex = len(operators) - 1
	}
	if startIndex > endIndex || endIndex < 0 {
		return nil
	}

	var content strings.Builder
	content.WriteString("q\n")
	writeFaithfulSyntheticMatrixConcatForProbe(&content, initialCTM)

	openScopes := 0
	for idx := startIndex; idx <= endIndex; idx++ {
		op := operators[idx]
		if op.Opcode == "Q" && openScopes == 0 {
			continue
		}

		content.WriteString(serializeFaithfulSyntheticTextOperatorForProbe(op))
		switch op.Opcode {
		case "q":
			openScopes++
		case "Q":
			if openScopes > 0 {
				openScopes--
			}
		}
	}

	for ; openScopes > 0; openScopes-- {
		content.WriteString("Q\n")
	}
	content.WriteString("Q\n")
	return []byte(content.String())
}

func buildFaithfulSyntheticParentContextReplayContentExcludingOperatorForProbe(
	initialCTM [6]float64,
	operators []domainrenderer.Operator,
	startIndex int,
	endIndex int,
	excludeIndex int,
) []byte {
	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex >= len(operators) {
		endIndex = len(operators) - 1
	}
	if startIndex > endIndex || endIndex < 0 {
		return nil
	}

	var content strings.Builder
	content.WriteString("q\n")
	writeFaithfulSyntheticMatrixConcatForProbe(&content, initialCTM)

	openScopes := 0
	for idx := startIndex; idx <= endIndex; idx++ {
		op := operators[idx]
		if idx == excludeIndex {
			continue
		}
		if op.Opcode == "Q" && openScopes == 0 {
			continue
		}

		content.WriteString(serializeFaithfulSyntheticTextOperatorForProbe(op))
		switch op.Opcode {
		case "q":
			openScopes++
		case "Q":
			if openScopes > 0 {
				openScopes--
			}
		}
	}

	for ; openScopes > 0; openScopes-- {
		content.WriteString("Q\n")
	}
	content.WriteString("Q\n")
	return []byte(content.String())
}

func buildFaithfulSyntheticParentContextReplayContentFilteredForProbe(
	initialCTM [6]float64,
	operators []domainrenderer.Operator,
	startIndex int,
	endIndex int,
	keep faithfulSyntheticOperatorIndexFilterForProbe,
) []byte {
	if startIndex < 0 {
		startIndex = 0
	}
	if endIndex >= len(operators) {
		endIndex = len(operators) - 1
	}
	if startIndex > endIndex || endIndex < 0 {
		return nil
	}

	var content strings.Builder
	content.WriteString("q\n")
	writeFaithfulSyntheticMatrixConcatForProbe(&content, initialCTM)

	openScopes := 0
	for idx := startIndex; idx <= endIndex; idx++ {
		op := operators[idx]
		if keep != nil && !keep(idx, op) {
			continue
		}
		if op.Opcode == "Q" && openScopes == 0 {
			continue
		}

		content.WriteString(serializeFaithfulSyntheticTextOperatorForProbe(op))
		switch op.Opcode {
		case "q":
			openScopes++
		case "Q":
			if openScopes > 0 {
				openScopes--
			}
		}
	}

	for ; openScopes > 0; openScopes-- {
		content.WriteString("Q\n")
	}
	content.WriteString("Q\n")
	return []byte(content.String())
}

func writeFaithfulSyntheticMatrixConcatForProbe(content *strings.Builder, matrix [6]float64) {
	content.WriteString(formatFaithfulSyntheticTextNumberForProbe(matrix[0]))
	content.WriteByte(' ')
	content.WriteString(formatFaithfulSyntheticTextNumberForProbe(matrix[1]))
	content.WriteByte(' ')
	content.WriteString(formatFaithfulSyntheticTextNumberForProbe(matrix[2]))
	content.WriteByte(' ')
	content.WriteString(formatFaithfulSyntheticTextNumberForProbe(matrix[3]))
	content.WriteByte(' ')
	content.WriteString(formatFaithfulSyntheticTextNumberForProbe(matrix[4]))
	content.WriteByte(' ')
	content.WriteString(formatFaithfulSyntheticTextNumberForProbe(matrix[5]))
	content.WriteString(" cm\n")
}

func sortedSyntheticPatternDictKeysForProbe(dict *entity.Dict) []string {
	if dict == nil {
		return nil
	}

	keys := dict.Keys()
	out := make([]string, 0, len(keys))
	for _, key := range sortedSyntheticFontNamesForProbe(keys) {
		out = append(out, strings.TrimPrefix(key.Value(), "/"))
	}
	return out
}

func describeFaithfulSyntheticShadingFunctionForProbe(
	t *testing.T,
	xref entity.XRef,
	shadingDict *entity.Dict,
) string {
	t.Helper()

	if shadingDict == nil {
		return "-"
	}
	rawFunction := shadingDict.GetRaw(entity.Name("Function"))
	if rawFunction == nil {
		return "-"
	}
	return describeFaithfulSyntheticFunctionObjectForProbe(t, xref, rawFunction, 0)
}

func describeFaithfulSyntheticFunctionObjectForProbe(
	t *testing.T,
	xref entity.XRef,
	obj entity.Object,
	depth int,
) string {
	t.Helper()

	if obj == nil {
		return "-"
	}
	if depth > 2 {
		return "..."
	}

	refLabel := ""
	if ref, ok := obj.(entity.Ref); ok {
		refLabel = fmt.Sprintf("@%d_%dR", ref.Num(), ref.Gen())
	}
	resolved := resolveSyntheticFontObjectForProbe(xref, obj)
	switch typed := resolved.(type) {
	case *entity.Array:
		parts := make([]string, 0, typed.Len())
		for i := 0; i < typed.Len(); i++ {
			parts = append(parts, describeFaithfulSyntheticFunctionObjectForProbe(t, xref, typed.Get(i), depth+1))
		}
		return "[" + strings.Join(parts, ";") + "]"
	case *entity.Stream:
		return describeFaithfulSyntheticFunctionDictForProbe(t, xref, typed.Dict(), typed, refLabel, depth)
	case *entity.Dict:
		return describeFaithfulSyntheticFunctionDictForProbe(t, xref, typed, nil, refLabel, depth)
	default:
		return fmt.Sprintf("%s%T", refLabel, resolved)
	}
}

func describeFaithfulSyntheticFunctionDictForProbe(
	t *testing.T,
	xref entity.XRef,
	dict *entity.Dict,
	streamObj *entity.Stream,
	refLabel string,
	depth int,
) string {
	t.Helper()

	if dict == nil {
		return refLabel + "nil"
	}

	functionType := syntheticIntStringForProbe(dict.Get(entity.Name("FunctionType")))
	parts := []string{
		fmt.Sprintf("%stype=%s", refLabel, functionType),
		fmt.Sprintf("domain=%s", syntheticNumberArrayStringForProbe(dict.Get(entity.Name("Domain")), 10)),
		fmt.Sprintf("range=%s", syntheticNumberArrayStringForProbe(dict.Get(entity.Name("Range")), 10)),
	}

	switch functionType {
	case "0":
		parts = append(parts,
			fmt.Sprintf("size=%s", syntheticNumberArrayStringForProbe(dict.Get(entity.Name("Size")), 8)),
			fmt.Sprintf("bits_per_sample=%s", syntheticIntStringForProbe(dict.Get(entity.Name("BitsPerSample")))),
			fmt.Sprintf("encode=%s", syntheticNumberArrayStringForProbe(dict.Get(entity.Name("Encode")), 10)),
			fmt.Sprintf("decode=%s", syntheticNumberArrayStringForProbe(dict.Get(entity.Name("Decode")), 10)),
		)
	case "2":
		parts = append(parts,
			fmt.Sprintf("c0=%s", syntheticNumberArrayStringForProbe(dict.Get(entity.Name("C0")), 10)),
			fmt.Sprintf("c1=%s", syntheticNumberArrayStringForProbe(dict.Get(entity.Name("C1")), 10)),
			fmt.Sprintf("n=%s", syntheticFloatStringForProbe(dict.Get(entity.Name("N")))),
		)
	case "3":
		parts = append(parts,
			fmt.Sprintf("bounds=%s", syntheticNumberArrayStringForProbe(dict.Get(entity.Name("Bounds")), 10)),
			fmt.Sprintf("encode=%s", syntheticNumberArrayStringForProbe(dict.Get(entity.Name("Encode")), 12)),
		)
		if rawFunctions := dict.GetRaw(entity.Name("Functions")); rawFunctions != nil {
			parts = append(parts, fmt.Sprintf("functions=%s", describeFaithfulSyntheticFunctionObjectForProbe(t, xref, rawFunctions, depth+1)))
		}
	case "4":
		if streamObj != nil {
			decoded, err := stream.NewFromEntity(streamObj).Decode()
			if err != nil {
				parts = append(parts, fmt.Sprintf("program_error=%q", err.Error()))
			} else {
				preview := compactFaithfulSyntheticProgramForProbe(string(decoded), 160)
				parts = append(parts, fmt.Sprintf("program_len=%d", len(decoded)), fmt.Sprintf("program=%q", preview))
			}
		}
	}

	return strings.Join(parts, "/")
}

func syntheticNumberArrayStringForProbe(obj entity.Object, maxItems int) string {
	array, ok := obj.(*entity.Array)
	if !ok {
		return "-"
	}
	if maxItems <= 0 || maxItems > array.Len() {
		maxItems = array.Len()
	}

	values := make([]string, 0, maxItems+1)
	for i := 0; i < maxItems; i++ {
		values = append(values, syntheticFloatStringForProbe(array.Get(i)))
	}
	if array.Len() > maxItems {
		values = append(values, "...")
	}
	return "[" + strings.Join(values, ",") + "]"
}

func compactFaithfulSyntheticProgramForProbe(program string, maxLen int) string {
	compact := strings.Join(strings.Fields(program), " ")
	if maxLen > 0 && len(compact) > maxLen {
		return compact[:maxLen] + "..."
	}
	return compact
}

func extractFaithfulSyntheticTextRawContentForProbe(
	t *testing.T,
	contents []entity.Object,
) []byte {
	t.Helper()

	var out bytes.Buffer
	for _, content := range contents {
		entityStream, ok := content.(*entity.Stream)
		if !ok {
			continue
		}

		decoded, err := stream.NewFromEntity(entityStream).Decode()
		if err != nil {
			decoded = append([]byte(nil), entityStream.RawBytes()...)
		}
		if len(decoded) == 0 {
			continue
		}

		out.Write(extractFaithfulSyntheticTextRawOperatorChunksForProbe(t, decoded))
	}
	return out.Bytes()
}

func extractFaithfulSyntheticTextRawOperatorChunksForProbe(
	t *testing.T,
	data []byte,
) []byte {
	t.Helper()

	lexer := parser.NewLexerBytes(data)
	p := parser.NewParser(lexer, nil)
	var out bytes.Buffer
	chunkStart := -1

	for {
		if p.HasBufferedObject() {
			_, start, _, err := p.ParseObjectWithSpan()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			if chunkStart < 0 {
				chunkStart = start
			}
			continue
		}

		token, err := lexer.Peek()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if token.Type == parser.TokenEOF {
			break
		}

		if token.Type == parser.TokenKeyword && isFaithfulSyntheticTextContentOperatorForProbe(token.Value) {
			if chunkStart < 0 {
				chunkStart = token.Pos
			}
			_, err := lexer.NextToken()
			require.NoError(t, err)
			if shouldKeepFaithfulSyntheticTextOperatorForProbe(token.Value) {
				out.Write(data[chunkStart:lexer.Pos()])
				out.WriteByte('\n')
			}
			chunkStart = -1
			continue
		}

		_, start, _, err := p.ParseObjectWithSpan()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if chunkStart < 0 {
			chunkStart = start
		}
	}

	return out.Bytes()
}

func rewriteFaithfulSyntheticContentStreamBytesForProbe(
	t *testing.T,
	data []byte,
	rewrite func(opcode string, chunk []byte) []byte,
) []byte {
	t.Helper()

	if len(data) == 0 || rewrite == nil {
		return append([]byte(nil), data...)
	}

	lexer := parser.NewLexerBytes(data)
	p := parser.NewParser(lexer, nil)
	var out bytes.Buffer
	chunkStart := -1

	for {
		if p.HasBufferedObject() {
			_, start, _, err := p.ParseObjectWithSpan()
			if err == io.EOF {
				break
			}
			if err != nil {
				return append([]byte(nil), data...)
			}
			if chunkStart < 0 {
				chunkStart = start
			}
			continue
		}

		token, err := lexer.Peek()
		if err == io.EOF {
			break
		}
		if err != nil {
			return append([]byte(nil), data...)
		}
		if token.Type == parser.TokenEOF {
			break
		}

		if token.Type == parser.TokenKeyword && isFaithfulSyntheticTextContentOperatorForProbe(token.Value) {
			if chunkStart < 0 {
				chunkStart = token.Pos
			}
			_, err := lexer.NextToken()
			if err != nil {
				return append([]byte(nil), data...)
			}

			chunk := append([]byte(nil), data[chunkStart:lexer.Pos()]...)
			chunk = rewrite(token.Value, chunk)
			if len(chunk) > 0 {
				out.Write(chunk)
				if chunk[len(chunk)-1] != '\n' {
					out.WriteByte('\n')
				}
			}
			chunkStart = -1
			continue
		}

		_, start, _, err := p.ParseObjectWithSpan()
		if err == io.EOF {
			break
		}
		if err != nil {
			return append([]byte(nil), data...)
		}
		if chunkStart < 0 {
			chunkStart = start
		}
	}

	return out.Bytes()
}

func isFaithfulSyntheticTextContentOperatorForProbe(keyword string) bool {
	switch keyword {
	case "q", "Q", "cm", "w", "J", "j", "M", "d", "ri", "i", "gs", "BX", "EX":
		return true
	case "m", "l", "c", "v", "y", "h", "re", "S", "s", "f", "F", "f*", "B", "B*", "b", "b*", "n", "W", "W*":
		return true
	case "CS", "cs", "SC", "SCN", "sc", "scn", "G", "g", "RG", "rg", "K", "k", "sh":
		return true
	case "BT", "ET", "Tc", "Tw", "Tz", "TL", "Tf", "Tr", "Ts", "Td", "TD", "Tm", "T*", "Tj", "TJ", "'", "\"":
		return true
	case "Do", "BI", "ID", "EI", "d0", "d1":
		return true
	default:
		return false
	}
}

func faithfulSyntheticGraphicsStateOperatorForProbe(opcode string) bool {
	switch opcode {
	case "q", "Q", "cm", "w", "J", "j", "M", "d", "ri", "i", "gs", "BX", "EX":
		return true
	case "CS", "cs", "SC", "SCN", "sc", "scn", "G", "g", "RG", "rg", "K", "k":
		return true
	default:
		return false
	}
}

func faithfulSyntheticPathConstructionOperatorForProbe(opcode string) bool {
	switch opcode {
	case "m", "l", "c", "v", "y", "h", "re", "W", "W*", "n":
		return true
	default:
		return false
	}
}

func faithfulSyntheticPathPaintOperatorForProbe(opcode string) bool {
	return faithfulSyntheticStrokePaintOperatorForProbe(opcode) || faithfulSyntheticFillPaintOperatorForProbe(opcode)
}

func faithfulSyntheticStrokePaintOperatorForProbe(opcode string) bool {
	switch opcode {
	case "S", "s", "B", "B*", "b", "b*":
		return true
	default:
		return false
	}
}

func faithfulSyntheticFillPaintOperatorForProbe(opcode string) bool {
	switch opcode {
	case "f", "F", "f*", "B", "B*", "b", "b*":
		return true
	default:
		return false
	}
}

func shouldKeepFaithfulSyntheticTextOperatorForProbe(opcode string) bool {
	switch opcode {
	case "q", "Q", "cm", "w", "J", "j", "M", "d", "ri", "i", "gs", "BX", "EX":
		return true
	case "m", "l", "c", "v", "y", "h", "re", "W", "W*", "n":
		return true
	case "S", "s", "f", "F", "f*", "B", "B*", "b", "b*":
		return true
	case "CS", "cs", "SC", "SCN", "sc", "scn", "G", "g", "RG", "rg", "K", "k":
		return true
	case "BT", "ET", "Tc", "Tw", "Tz", "TL", "Tf", "Tr", "Ts", "Td", "TD", "Tm", "T*", "Tj", "TJ", "'", "\"":
		return true
	case "Do":
		return true
	default:
		return false
	}
}

func serializeFaithfulSyntheticTextOperatorForProbe(op domainrenderer.Operator) string {
	var builder strings.Builder
	for idx, operand := range op.Operands {
		if idx > 0 {
			builder.WriteByte(' ')
		}
		builder.WriteString(serializeSyntheticFontPrimitiveObject(operand))
	}
	if len(op.Operands) > 0 {
		builder.WriteByte(' ')
	}
	builder.WriteString(op.Opcode)
	builder.WriteByte('\n')
	return builder.String()
}

func collectFaithfulSyntheticTextFontUsageForProbe(
	t *testing.T,
	xref entity.XRef,
	resources *entity.Dict,
	operators []domainrenderer.Operator,
) []faithfulSyntheticTextFontUsage {
	t.Helper()

	fonts, ok := resolveSyntheticFontDictForProbe(xref, resources.Get(entity.Name("Font")))
	if !ok || fonts == nil {
		return nil
	}

	fontResources := loadSyntheticFontPageResourcesForProbe(t, xref, fonts)
	countsByFont := collectSyntheticFontCodeFrequencyByResource(t, operators, fontResources)

	out := make([]faithfulSyntheticTextFontUsage, 0, len(countsByFont))
	for resourceName, codeCounts := range countsByFont {
		resource, ok := fontResources[resourceName]
		if !ok {
			continue
		}
		out = append(out, faithfulSyntheticTextFontUsage{
			resourceName: resourceName,
			baseFont:     resource.baseFont,
			subtype:      resource.subtype,
			totalCodes:   totalSyntheticFontCodeCountForProbe(codeCounts),
		})
	}

	sort.Slice(out, func(left, right int) bool {
		if out[left].totalCodes == out[right].totalCodes {
			return out[left].resourceName < out[right].resourceName
		}
		return out[left].totalCodes > out[right].totalCodes
	})
	return out
}

func measureFaithfulSyntheticTextOperatorHistogramForProbe(
	operators []domainrenderer.Operator,
) map[string]int {
	hist := make(map[string]int, len(operators))
	for _, op := range operators {
		hist[op.Opcode]++
	}
	return hist
}

func measureFaithfulSyntheticTextDroppedOperatorHistogramForProbe(
	operators []domainrenderer.Operator,
	_ []domainrenderer.Operator,
) map[string]int {
	dropped := make(map[string]int)
	for _, op := range operators {
		if shouldKeepFaithfulSyntheticTextOperatorForProbe(op.Opcode) {
			continue
		}
		dropped[op.Opcode]++
	}
	return dropped
}

func formatFaithfulSyntheticTextFontUsageForProbe(
	fontUsage []faithfulSyntheticTextFontUsage,
	limit int,
) string {
	if len(fontUsage) == 0 {
		return "-"
	}

	if limit > 0 && len(fontUsage) > limit {
		fontUsage = fontUsage[:limit]
	}

	parts := make([]string, 0, len(fontUsage))
	for _, usage := range fontUsage {
		parts = append(parts, fmt.Sprintf(
			"%s:%s/%s(x%d)",
			usage.resourceName,
			emptyDashForSyntheticFontProbe(usage.baseFont),
			emptyDashForSyntheticFontProbe(usage.subtype),
			usage.totalCodes,
		))
	}
	return strings.Join(parts, ",")
}

func formatFaithfulSyntheticTextOperatorHistogramForProbe(
	hist map[string]int,
) string {
	if len(hist) == 0 {
		return "-"
	}

	type entry struct {
		opcode string
		count  int
	}

	entries := make([]entry, 0, len(hist))
	for opcode, count := range hist {
		entries = append(entries, entry{opcode: opcode, count: count})
	}

	sort.Slice(entries, func(left, right int) bool {
		if entries[left].count == entries[right].count {
			return entries[left].opcode < entries[right].opcode
		}
		return entries[left].count > entries[right].count
	})

	parts := make([]string, 0, len(entries))
	for _, item := range entries {
		parts = append(parts, fmt.Sprintf("%s:%d", item.opcode, item.count))
	}
	return strings.Join(parts, ",")
}

func formatFaithfulSyntheticTextNumberForProbe(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func faithfulSyntheticTextVariantProbeCases() []faithfulSyntheticTextVariantProbeCase {
	return []faithfulSyntheticTextVariantProbeCase{
		{
			name: "full_serialized",
			keep: func(op domainrenderer.Operator) bool { return true },
			rewrite: func(op domainrenderer.Operator) []domainrenderer.Operator {
				return []domainrenderer.Operator{op}
			},
		},
		{
			name: "no_do",
			keep: func(op domainrenderer.Operator) bool {
				return op.Opcode != "Do"
			},
			rewrite: func(op domainrenderer.Operator) []domainrenderer.Operator {
				return []domainrenderer.Operator{op}
			},
		},
		{
			name: "no_path_paint",
			keep: func(op domainrenderer.Operator) bool { return true },
			rewrite: func(op domainrenderer.Operator) []domainrenderer.Operator {
				switch op.Opcode {
				case "S", "s", "f", "F", "f*", "B", "B*", "b", "b*":
					return []domainrenderer.Operator{{Opcode: "n"}}
				default:
					return []domainrenderer.Operator{op}
				}
			},
		},
		{
			name: "no_clip",
			keep: func(op domainrenderer.Operator) bool { return true },
			rewrite: func(op domainrenderer.Operator) []domainrenderer.Operator {
				switch op.Opcode {
				case "W", "W*":
					return nil
				default:
					return []domainrenderer.Operator{op}
				}
			},
		},
	}
}

func faithfulSyntheticTextFormGraphVariantProbeCases() []faithfulSyntheticTextFormGraphVariantProbeCase {
	return []faithfulSyntheticTextFormGraphVariantProbeCase{
		{
			name: "form_graph_baseline",
			mutate: func(_ *testing.T, obj entity.Object) entity.Object {
				return obj
			},
		},
		{
			name: "form_no_bbox",
			mutate: func(t *testing.T, obj entity.Object) entity.Object {
				return mutateFaithfulSyntheticFormStreamForProbe(t, obj, true, nil)
			},
		},
		{
			name: "form_white_image_xobjects",
			mutate: func(t *testing.T, obj entity.Object) entity.Object {
				return replaceFaithfulSyntheticImageWithWhitePixelForProbe(obj)
			},
		},
	}
}

func rewriteFaithfulSyntheticTextOperatorsForVariantProbe(
	operators []domainrenderer.Operator,
	variant faithfulSyntheticTextVariantProbeCase,
) []domainrenderer.Operator {
	out := make([]domainrenderer.Operator, 0, len(operators))
	for _, op := range operators {
		if variant.keep != nil && !variant.keep(op) {
			continue
		}
		if variant.rewrite == nil {
			out = append(out, op)
			continue
		}
		out = append(out, variant.rewrite(op)...)
	}
	return out
}

func mutateFaithfulSyntheticFormStreamForProbe(
	t *testing.T,
	obj entity.Object,
	stripBBox bool,
	rewrite func(opcode string, chunk []byte) []byte,
) entity.Object {
	t.Helper()

	streamObj, ok := obj.(*entity.Stream)
	if !ok {
		return obj
	}
	if syntheticNameValueForProbe(streamObj.Dict().Get(entity.Name("Subtype"))) != "Form" {
		return obj
	}

	dict := cloneFaithfulSyntheticDictForProbe(streamObj.Dict(), stripBBox)
	data := append([]byte(nil), streamObj.RawBytes()...)
	if rewrite != nil {
		data = rewriteFaithfulSyntheticContentStreamBytesForProbe(t, data, rewrite)
	}
	dict.Set(entity.Name("Length"), entity.NewInteger(int64(len(data))))
	return entity.NewStream(dict, data)
}

func replaceFaithfulSyntheticImageWithWhitePixelForProbe(obj entity.Object) entity.Object {
	streamObj, ok := obj.(*entity.Stream)
	if !ok {
		return obj
	}
	if syntheticNameValueForProbe(streamObj.Dict().Get(entity.Name("Subtype"))) != "Image" {
		return obj
	}

	dict := entity.NewDict()
	for _, key := range sortedSyntheticFontNamesForProbe(streamObj.Dict().Keys()) {
		switch normalizeFaithfulSyntheticDictKeyForProbe(key) {
		case "Mask", "SMask", "Decode", "Filter", "DecodeParms":
			continue
		}
		value := streamObj.Dict().GetRaw(key)
		if value == nil {
			continue
		}
		dict.Set(key, value.Clone())
	}
	dict.Set(entity.Name("Type"), entity.Name("XObject"))
	dict.Set(entity.Name("Subtype"), entity.Name("Image"))
	dict.Set(entity.Name("Width"), entity.NewInteger(1))
	dict.Set(entity.Name("Height"), entity.NewInteger(1))
	dict.Set(entity.Name("BitsPerComponent"), entity.NewInteger(8))
	dict.Set(entity.Name("ColorSpace"), entity.Name("DeviceGray"))
	dict.Set(entity.Name("ImageMask"), entity.NewBoolean(false))

	data := []byte{0xff}
	dict.Set(entity.Name("Length"), entity.NewInteger(int64(len(data))))
	return entity.NewStream(dict, data)
}

func cloneFaithfulSyntheticDictForProbe(dict *entity.Dict, stripBBox bool) *entity.Dict {
	out := entity.NewDict()
	if dict == nil {
		return out
	}
	for _, key := range sortedSyntheticFontNamesForProbe(dict.Keys()) {
		if stripBBox && normalizeFaithfulSyntheticDictKeyForProbe(key) == "BBox" {
			continue
		}
		value := dict.GetRaw(key)
		if value == nil {
			continue
		}
		out.Set(key, value.Clone())
	}
	return out
}

func normalizeFaithfulSyntheticDictKeyForProbe(key entity.Name) string {
	return strings.TrimPrefix(key.Value(), "/")
}

func runFaithfulSyntheticPopplerAssetCommandForProbe(
	t *testing.T,
	name string,
	args ...string,
) string {
	t.Helper()

	path, err := exec.LookPath(name)
	require.NoError(t, err)

	cmd := exec.Command(path, args...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s %v failed: %s", name, args, string(output))
	return string(output)
}

func collectFaithfulSyntheticXObjectSummaryForProbe(
	t *testing.T,
	xref entity.XRef,
	resources *entity.Dict,
) string {
	t.Helper()

	if resources == nil {
		return "-"
	}

	xobjects, ok := resolveSyntheticFontDictForProbe(xref, resources.Get(entity.Name("XObject")))
	if !ok || xobjects == nil {
		return "-"
	}

	names := sortedSyntheticFontNamesForProbe(xobjects.Keys())
	parts := make([]string, 0, len(names))
	for _, name := range names {
		rawObject := xobjects.GetRaw(name)
		resolved := resolveSyntheticFontObjectForProbe(xref, rawObject)
		refSuffix := ""
		if ref, ok := rawObject.(entity.Ref); ok {
			refSuffix = fmt.Sprintf("@%d_%dR", ref.Num(), ref.Gen())
		}
		switch typed := resolved.(type) {
		case *entity.Stream:
			subtype := syntheticNameValueForProbe(typed.Dict().Get(entity.Name("Subtype")))
			parts = append(parts, fmt.Sprintf("%s%s:stream/%s", strings.TrimPrefix(name.Value(), "/"), refSuffix, emptyDashForSyntheticFontProbe(subtype)))
		case *entity.Dict:
			subtype := syntheticNameValueForProbe(typed.Get(entity.Name("Subtype")))
			parts = append(parts, fmt.Sprintf("%s%s:dict/%s", strings.TrimPrefix(name.Value(), "/"), refSuffix, emptyDashForSyntheticFontProbe(subtype)))
		default:
			parts = append(parts, fmt.Sprintf("%s%s:%T", strings.TrimPrefix(name.Value(), "/"), refSuffix, resolved))
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func collectFaithfulSyntheticPatternSummaryForProbe(
	t *testing.T,
	xref entity.XRef,
	resources *entity.Dict,
) string {
	t.Helper()

	return collectFaithfulSyntheticNamedResourceSummaryForProbe(t, xref, resources, entity.Name("Pattern"))
}

func collectFaithfulSyntheticNamedResourceSummaryForProbe(
	t *testing.T,
	xref entity.XRef,
	resources *entity.Dict,
	category entity.Name,
) string {
	t.Helper()

	if resources == nil {
		return "-"
	}
	categoryDict, ok := resolveSyntheticFontDictForProbe(xref, resources.Get(category))
	if !ok || categoryDict == nil {
		return "-"
	}

	names := sortedSyntheticFontNamesForProbe(categoryDict.Keys())
	parts := make([]string, 0, len(names))
	for _, name := range names {
		rawObject := categoryDict.GetRaw(name)
		resolved := resolveSyntheticFontObjectForProbe(xref, rawObject)
		label := strings.TrimPrefix(name.Value(), "/")
		switch category.Value() {
		case "/Pattern":
			switch typed := resolved.(type) {
			case *entity.Stream:
				patternType := syntheticIntStringForProbe(typed.Dict().Get(entity.Name("PatternType")))
				paintType := syntheticIntStringForProbe(typed.Dict().Get(entity.Name("PaintType")))
				parts = append(parts, fmt.Sprintf("%s:stream/patternType=%s/paintType=%s", label, patternType, paintType))
			case *entity.Dict:
				patternType := syntheticIntStringForProbe(typed.Get(entity.Name("PatternType")))
				paintType := syntheticIntStringForProbe(typed.Get(entity.Name("PaintType")))
				parts = append(parts, fmt.Sprintf("%s:dict/patternType=%s/paintType=%s", label, patternType, paintType))
			default:
				parts = append(parts, fmt.Sprintf("%s:%T", label, resolved))
			}
		default:
			switch resolved.(type) {
			case *entity.Stream:
				parts = append(parts, fmt.Sprintf("%s:stream", label))
			case *entity.Dict:
				parts = append(parts, fmt.Sprintf("%s:dict", label))
			default:
				parts = append(parts, fmt.Sprintf("%s:%T", label, resolved))
			}
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

func syntheticIntStringForProbe(obj entity.Object) string {
	switch typed := obj.(type) {
	case *entity.Integer:
		return strconv.FormatInt(typed.Value(), 10)
	case *entity.Real:
		return strconv.FormatFloat(typed.Value(), 'f', -1, 64)
	default:
		return "-"
	}
}

func syntheticFloatStringForProbe(obj entity.Object) string {
	switch typed := obj.(type) {
	case *entity.Integer:
		return strconv.FormatFloat(float64(typed.Value()), 'f', -1, 64)
	case *entity.Real:
		return strconv.FormatFloat(typed.Value(), 'f', -1, 64)
	default:
		return "-"
	}
}

func formatFaithfulSyntheticOperatorSequenceForProbe(operators []domainrenderer.Operator) string {
	if len(operators) == 0 {
		return "-"
	}

	parts := make([]string, 0, len(operators))
	for _, op := range operators {
		switch op.Opcode {
		case "cm":
			if matrix, ok := faithfulSyntheticMatrixOperandsForProbe(op.Operands); ok {
				parts = append(parts, fmt.Sprintf("cm[%0.5f,%0.5f]", matrix[4], matrix[5]))
				continue
			}
		case "scn", "SCN":
			if len(op.Operands) > 0 {
				if name := syntheticNameValueForProbe(op.Operands[len(op.Operands)-1]); name != "" {
					parts = append(parts, fmt.Sprintf("%s[%s]", op.Opcode, name))
					continue
				}
			}
		}
		parts = append(parts, op.Opcode)
	}
	return strings.Join(parts, " ")
}

func formatFaithfulSyntheticOperatorProgramForProbe(operators []domainrenderer.Operator) string {
	if len(operators) == 0 {
		return "-"
	}

	var builder strings.Builder
	for _, op := range operators {
		builder.WriteString(serializeFaithfulSyntheticTextOperatorForProbe(op))
	}
	return strings.TrimSpace(builder.String())
}

func nonTransparentBoundsForProbe(img *image.RGBA) (image.Rectangle, int) {
	if img == nil {
		return image.Rectangle{}, 0
	}

	bounds := img.Bounds()
	minX, minY := bounds.Max.X, bounds.Max.Y
	maxX, maxY := bounds.Min.X, bounds.Min.Y
	painted := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if img.RGBAAt(x, y).A == 0 {
				continue
			}
			painted++
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x+1 > maxX {
				maxX = x + 1
			}
			if y+1 > maxY {
				maxY = y + 1
			}
		}
	}
	if painted == 0 {
		return image.Rectangle{}, 0
	}
	return image.Rect(minX, minY, maxX, maxY), painted
}

type fillPatternRecordingCanvas struct {
	*infracanvas.ImageCanvas
	fillPatternHistory []string
	lastFillPattern    entity.Pattern
	pathMoveHistory    []string
}

func (c *fillPatternRecordingCanvas) SetFillPattern(pattern entity.Pattern) {
	c.lastFillPattern = pattern
	switch typed := pattern.(type) {
	case *entity.ShadingPattern:
		c.fillPatternHistory = append(c.fillPatternHistory, fmt.Sprintf("shading:%v", typed.Matrix()))
	case nil:
		c.fillPatternHistory = append(c.fillPatternHistory, "nil")
	default:
		c.fillPatternHistory = append(c.fillPatternHistory, fmt.Sprintf("%T", pattern))
	}
	c.ImageCanvas.SetFillPattern(pattern)
}

func (c *fillPatternRecordingCanvas) MoveTo(x, y float64) {
	c.pathMoveHistory = append(c.pathMoveHistory, fmt.Sprintf("(%.3f,%.3f)", x, y))
	c.ImageCanvas.MoveTo(x, y)
}

func formatSyntheticRGBASampleForProbe(value color.RGBA) string {
	return fmt.Sprintf("rgba(%d,%d,%d,%d)", value.R, value.G, value.B, value.A)
}
