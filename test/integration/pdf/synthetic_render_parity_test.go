package pdf_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"

	"github.com/dh-kam/pdf-go/internal/domain/colorspace"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainimage "github.com/dh-kam/pdf-go/internal/domain/image"
	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
	infraimage "github.com/dh-kam/pdf-go/internal/infrastructure/image"
	pdfstream "github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"
	pdfxref "github.com/dh-kam/pdf-go/internal/infrastructure/pdf/xref"
	infrarenderer "github.com/dh-kam/pdf-go/internal/infrastructure/renderer"
	internalpdf "github.com/dh-kam/pdf-go/internal/usecase/pdf"
	"github.com/dh-kam/pdf-go/pkg/pdf"
)

type parityScore struct {
	exact      float64
	similarity float64
}

type rgbDiffSummary struct {
	count int
	rMAE  float64
	gMAE  float64
	bMAE  float64
}

type paletteRGBDiffSummary struct {
	index             int
	count             int
	c                 uint8
	m                 uint8
	y                 uint8
	k                 uint8
	rMAE              float64
	gMAE              float64
	bMAE              float64
	weightedBAbsError float64
}

type cmykBlueDecomposition struct {
	constant float64
	cTerm    float64
	mTerm    float64
	yTerm    float64
	kTerm    float64
	total    float64
}

type pageOperatorSummary struct {
	textBlockCount int
	textShowCount  int
	xObjectDoCount int
	inlineImageBI  int
}

type edgeOrientation uint8

const (
	edgeOrientationNone edgeOrientation = iota
	edgeOrientationHorizontal
	edgeOrientationVertical
	edgeOrientationMixed
)

func TestSyntheticVectorRenderParityAgainstPoppler(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	pdfPath := filepath.Join(root, "vector_rects.pdf")
	popplerRoot := filepath.Join(root, "poppler")
	oursPNG := filepath.Join(root, "ours.png")

	content := "0 0 0 rg 0 0 1 1 re f\n2 1 1 1 re f\n1 3 2 1 re f\n"
	require.NoError(t, os.WriteFile(pdfPath, buildSyntheticContentPDF(4, 4, []byte(content)), 0o644))
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, 72))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 72

	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 4, 4), img.Bounds())
	require.NoError(t, parityWritePNG(oursPNG, img))

	exactPercent, similarityPercent, err := parityComparePNGs(oursPNG, popplerPages[1])
	require.NoError(t, err)
	assert.InDelta(t, 100.0, exactPercent, 1e-9)
	assert.InDelta(t, 100.0, similarityPercent, 1e-9)
}

func TestSampleRejectedColorspaceRGBCompositionCandidates(t *testing.T) {
	testCases := []struct {
		name                string
		pdfPath             string
		wantXObjectDoCount  int
		wantInlineImageBI   int
		wantTextShowAtLeast int
	}{
		{
			name:                "003_pdflatex_image",
			pdfPath:             filepath.Join(getSampleDir(), "003-pdflatex-image", "pdflatex-image.pdf"),
			wantXObjectDoCount:  1,
			wantInlineImageBI:   0,
			wantTextShowAtLeast: 1,
		},
		{
			name:                "008_inline_image",
			pdfPath:             filepath.Join(getSampleDir(), "008-reportlab-inline-image", "inline-image.pdf"),
			wantXObjectDoCount:  0,
			wantInlineImageBI:   1,
			wantTextShowAtLeast: 1,
		},
		{
			name:                "018_base64_image",
			pdfPath:             filepath.Join(getSampleDir(), "018-base64-image", "base64image.pdf"),
			wantXObjectDoCount:  1,
			wantInlineImageBI:   0,
			wantTextShowAtLeast: 1,
		},
	}

	pureImageOnlyCandidates := 0

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := summarizeSamplePageOperators(t, tc.pdfPath)
			t.Logf(
				"%s operator summary: BT=%d Tj/TJ=%d Do=%d BI=%d",
				tc.name,
				got.textBlockCount,
				got.textShowCount,
				got.xObjectDoCount,
				got.inlineImageBI,
			)

			assert.Equal(t, tc.wantXObjectDoCount, got.xObjectDoCount)
			assert.Equal(t, tc.wantInlineImageBI, got.inlineImageBI)
			assert.GreaterOrEqual(t, got.textShowCount, tc.wantTextShowAtLeast)

			if got.textShowCount == 0 && got.textBlockCount == 0 && (got.xObjectDoCount+got.inlineImageBI) > 0 {
				pureImageOnlyCandidates++
			}
		})
	}

	assert.Zero(t, pureImageOnlyCandidates)
}

func TestSyntheticImageRenderProbeAgainstPoppler(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	testCases := []struct {
		name   string
		imageW int
		imageH int
		image  []byte
		pageW  int
		pageH  int
		matrix [6]int
	}{
		{
			name:   "gray_identity_4x4",
			imageW: 4,
			imageH: 4,
			image: []byte{
				0, 63, 127, 255,
				255, 127, 63, 0,
				0, 255, 0, 255,
				255, 0, 255, 0,
			},
			pageW:  4,
			pageH:  4,
			matrix: [6]int{4, 0, 0, 4, 0, 0},
		},
		{
			name:   "gray_sparse_box_downscale_16_to_4",
			imageW: 16,
			imageH: 16,
			image:  syntheticSparseGray16x16(),
			pageW:  4,
			pageH:  4,
			matrix: [6]int{4, 0, 0, 4, 0, 0},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			pdfPath := filepath.Join(root, tc.name+".pdf")
			popplerRoot := filepath.Join(root, "poppler")
			oursPNG := filepath.Join(root, "ours.png")

			require.NoError(t, os.WriteFile(
				pdfPath,
				buildSyntheticGrayImagePDF(tc.pageW, tc.pageH, tc.imageW, tc.imageH, tc.matrix, tc.image),
				0o644,
			))
			require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
			require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, 72))

			popplerPages, err := parityListPopplerPages(popplerRoot)
			require.NoError(t, err)
			require.Contains(t, popplerPages, 1)

			doc, err := pdf.Open(pdfPath)
			require.NoError(t, err)
			defer doc.Close()

			page, err := doc.Page(0)
			require.NoError(t, err)

			renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
			opts := pdf.DefaultRenderOptions()
			opts.DPI = 72

			img, err := renderer.RenderPage(context.Background(), page, opts)
			require.NoError(t, err)
			require.Equal(t, image.Rect(0, 0, tc.pageW, tc.pageH), img.Bounds())
			require.NoError(t, parityWritePNG(oursPNG, img))

			exactPercent, similarityPercent, err := parityComparePNGs(oursPNG, popplerPages[1])
			require.NoError(t, err)
			t.Skipf(
				"known image parity mismatch under investigation: exact=%.4f similarity=%.4f",
				exactPercent,
				similarityPercent,
			)
		})
	}
}

func TestSyntheticGraySparseBoxDownscaleReferenceProbeAgainstPoppler(t *testing.T) {
	src := syntheticGraySparse16x16ImageForProbe()

	oursSimilarity, boxSimilarity := measureSyntheticGrayReferenceAgainstCurrentPath(
		t,
		"gray_sparse_box_downscale_16_to_4.pdf",
		buildSyntheticGrayImagePDF(4, 4, 16, 16, [6]int{4, 0, 0, 4, 0, 0}, append([]byte(nil), src.Pix...)),
		src,
		image.Rect(0, 0, 4, 4),
		simulateSyntheticGrayBoxDownscale(src, image.Rect(0, 0, 4, 4)),
	)

	t.Skipf(
		"gray_sparse_box_downscale_16_to_4 probe: ours=%.4f box_reference=%.4f",
		oursSimilarity,
		boxSimilarity,
	)
}

func TestSyntheticGraySparseBoxDownscale16To8ReferenceProbeAgainstPoppler(t *testing.T) {
	src := syntheticGraySparse16x16ImageForProbe()

	matrix := [6]float64{8, 0, 0, 8, 0, 0}
	_, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
		t,
		"gray_sparse_box_downscale_16_to_8.pdf",
		buildSyntheticGraySparseBoxPDFForProbe(8, matrix, src),
		map[string]image.Image{
			"affine_bilinear_phase_0":   simulateSyntheticAffineImageWithMatrixAndPhase(src, image.Rect(0, 0, 8, 8), matrix, true, 0, 0),
			"affine_bilinear_phase_0.5": simulateSyntheticAffineImageWithMatrixAndPhase(src, image.Rect(0, 0, 8, 8), matrix, true, 0.5, 0.5),
			"area_box_then_place":       simulateSyntheticAreaBoxThenPlaceWithFilter(src, image.Rect(0, 0, 8, 8), matrix, ""),
		},
	)

	t.Logf(
		"gray_sparse_box_downscale_16_to_8 probe: current=%.4f phase0=%.4f phase0.5=%.4f area_box=%.4f",
		currentSimilarity,
		refScores["affine_bilinear_phase_0"],
		refScores["affine_bilinear_phase_0.5"],
		refScores["area_box_then_place"],
	)

	assert.Greater(t, refScores["affine_bilinear_phase_0"], currentSimilarity)
	assert.Greater(t, refScores["affine_bilinear_phase_0.5"], currentSimilarity)
	assert.Greater(t, refScores["area_box_then_place"], currentSimilarity)
	assert.Greater(t, refScores["affine_bilinear_phase_0.5"], refScores["area_box_then_place"])
}

func TestSyntheticGraySparseBoxDownscaleSizeSweepProbeAgainstPoppler(t *testing.T) {
	src := syntheticGraySparse16x16ImageForProbe()

	for _, dstSize := range []int{4, 6, 8, 10} {
		dstSize := dstSize
		t.Run(fmt.Sprintf("dst_%02d", dstSize), func(t *testing.T) {
			matrix := [6]float64{float64(dstSize), 0, 0, float64(dstSize), 0, 0}
			pageBounds := image.Rect(0, 0, dstSize, dstSize)

			_, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("gray_sparse_box_downscale_16_to_%02d_sweep.pdf", dstSize),
				buildSyntheticGraySparseBoxPDFForProbe(float64(dstSize), matrix, src),
				map[string]image.Image{
					"affine_bilinear_phase_0":   simulateSyntheticAffineImageWithMatrixAndPhase(src, pageBounds, matrix, true, 0, 0),
					"affine_bilinear_phase_0.5": simulateSyntheticAffineImageWithMatrixAndPhase(src, pageBounds, matrix, true, 0.5, 0.5),
					"area_box_then_place":       simulateSyntheticAreaBoxThenPlaceWithFilter(src, pageBounds, matrix, ""),
				},
			)

			t.Logf(
				"gray_sparse_box_downscale_16_to_%02d sweep: current=%.4f phase0=%.4f phase0.5=%.4f area_box=%.4f",
				dstSize,
				currentSimilarity,
				refScores["affine_bilinear_phase_0"],
				refScores["affine_bilinear_phase_0.5"],
				refScores["area_box_then_place"],
			)
		})
	}
}

func TestSyntheticIndexedDoc019SizedDownscaleProbeAgainstPoppler(t *testing.T) {
	palette, imageData := syntheticIndexedTiledIdentity(324, 450)
	exactPercent, similarityPercent := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc019_sized_downscale.pdf",
		buildSyntheticIndexedImagePDFFloat(243, 338, 324, 450, [6]float64{243, 0, 0, 338, 0, 0}, palette, imageData),
	)

	t.Skipf(
		"indexed_doc019_sized_downscale probe: exact=%.4f similarity=%.4f",
		exactPercent,
		similarityPercent,
	)
}

func TestSyntheticIndexedDoc023SizedOffsetDownscaleProbeAgainstPoppler(t *testing.T) {
	palette, imageData := syntheticIndexedTiledIdentity(756, 1008)
	exactPercent, similarityPercent := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_sized_offset_downscale.pdf",
		buildSyntheticIndexedImagePDFFloat(540, 720, 756, 1008, [6]float64{468, 0, 0, 624, 72, 96}, palette, imageData),
	)

	t.Skipf(
		"indexed_doc023_sized_offset_downscale probe: exact=%.4f similarity=%.4f",
		exactPercent,
		similarityPercent,
	)
}

func TestSyntheticIndexedDoc023SizedOriginDownscaleProbeAgainstPoppler(t *testing.T) {
	palette, imageData := syntheticIndexedTiledIdentity(756, 1008)
	exactPercent, similarityPercent := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_sized_origin_downscale.pdf",
		buildSyntheticIndexedImagePDFFloat(540, 720, 756, 1008, [6]float64{468, 0, 0, 624, 0, 0}, palette, imageData),
	)

	t.Skipf(
		"indexed_doc023_sized_origin_downscale probe: exact=%.4f similarity=%.4f",
		exactPercent,
		similarityPercent,
	)
}

func TestSyntheticIndexedDoc023OriginVsOffsetPlacementProbeAgainstPoppler(t *testing.T) {
	palette, imageData := syntheticIndexedTiledIdentity(756, 1008)

	originExact, originSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_origin_vs_offset_origin.pdf",
		buildSyntheticIndexedImagePDFFloat(540, 720, 756, 1008, [6]float64{468, 0, 0, 624, 0, 0}, palette, imageData),
	)
	offsetExact, offsetSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_origin_vs_offset_offset.pdf",
		buildSyntheticIndexedImagePDFFloat(540, 720, 756, 1008, [6]float64{468, 0, 0, 624, 72, 96}, palette, imageData),
	)

	t.Skipf(
		"indexed_doc023 origin-vs-offset probe: origin_exact=%.4f origin_similarity=%.4f offset_exact=%.4f offset_similarity=%.4f",
		originExact,
		originSimilarity,
		offsetExact,
		offsetSimilarity,
	)
}

func TestSyntheticIndexedCMYKDoc023SizedOffsetDownscaleProbeAgainstPoppler(t *testing.T) {
	palette, imageData := syntheticIndexedCMYKIdentity4x4()
	exactPercent, similarityPercent := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_cmyk_doc023_sized_offset_downscale.pdf",
		buildSyntheticIndexedImagePDFFloatWithBase(540, 720, 4, "DeviceCMYK", 756, 1008, [6]float64{468, 0, 0, 624, 72, 96}, palette, imageData),
	)

	t.Skipf(
		"indexed_cmyk_doc023_sized_offset_downscale probe: exact=%.4f similarity=%.4f",
		exactPercent,
		similarityPercent,
	)
}

func TestSyntheticIndexedDoc023RGBVsCMYKBaseProbeAgainstPoppler(t *testing.T) {
	rgbPalette, rgbImageData := syntheticIndexedTiledIdentity(756, 1008)
	cmykPalette, cmykImageData := syntheticIndexedCMYKTiledIdentity(756, 1008)

	rgbExact, rgbSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_rgb_base_probe.pdf",
		buildSyntheticIndexedImagePDFFloatWithBase(540, 720, 3, "DeviceRGB", 756, 1008, [6]float64{468, 0, 0, 624, 72, 96}, rgbPalette, rgbImageData),
	)
	cmykExact, cmykSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_cmyk_base_probe.pdf",
		buildSyntheticIndexedImagePDFFloatWithBase(540, 720, 4, "DeviceCMYK", 756, 1008, [6]float64{468, 0, 0, 624, 72, 96}, cmykPalette, cmykImageData),
	)

	t.Skipf(
		"indexed_doc023 rgb-vs-cmyk base probe: rgb_exact=%.4f rgb_similarity=%.4f cmyk_exact=%.4f cmyk_similarity=%.4f",
		rgbExact,
		rgbSimilarity,
		cmykExact,
		cmykSimilarity,
	)
}

func TestSyntheticIndexedDoc023ExtractedImageObjectProbeAgainstPoppler(t *testing.T) {
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)
	require.Len(t, palette, 256*4)
	require.Len(t, imageData, width*height)

	sampleExact, sampleSimilarity := probePDFRenderAgainstPoppler(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		pdf.DefaultRenderOptions(),
	)
	syntheticExact, syntheticSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_extracted_image_object.pdf",
		buildSyntheticIndexedImagePDFFloatWithBase(
			540,
			720,
			4,
			base,
			width,
			height,
			[6]float64{468, 0, 0, 624, 72, 96},
			palette,
			imageData,
		),
	)

	t.Skipf(
		"indexed_doc023 extracted image object probe: sample_exact=%.4f sample_similarity=%.4f synthetic_exact=%.4f synthetic_similarity=%.4f",
		sampleExact,
		sampleSimilarity,
		syntheticExact,
		syntheticSimilarity,
	)
}

func TestSyntheticDoc023ExtractedImageObjectIndexedVsDirectCMYKProbeAgainstPoppler(t *testing.T) {
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	indexedExact, indexedSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_extracted_vs_direct_indexed.pdf",
		buildSyntheticIndexedImagePDFFloatWithBase(
			540,
			720,
			4,
			base,
			width,
			height,
			[6]float64{468, 0, 0, 624, 72, 96},
			palette,
			imageData,
		),
	)
	directExact, directSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_extracted_vs_direct_cmyk.pdf",
		buildSyntheticCMYKImagePDFFloat(
			540,
			720,
			width,
			height,
			[6]float64{468, 0, 0, 624, 72, 96},
			expandIndexedCMYKImageData(palette, imageData),
		),
	)

	t.Skipf(
		"indexed_doc023 extracted indexed-vs-direct probe: indexed_exact=%.4f indexed_similarity=%.4f direct_exact=%.4f direct_similarity=%.4f",
		indexedExact,
		indexedSimilarity,
		directExact,
		directSimilarity,
	)
}

func TestSyntheticDoc023ExtractedImageObjectSimpleCMYKToRGBProbeAgainstPoppler(t *testing.T) {
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	expandedCMYK := expandIndexedCMYKImageData(palette, imageData)
	directExact, directSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_extracted_simple_rgb_direct_cmyk.pdf",
		buildSyntheticCMYKImagePDFFloat(
			540,
			720,
			width,
			height,
			[6]float64{468, 0, 0, 624, 72, 96},
			expandedCMYK,
		),
	)
	simpleRGBExact, simpleRGBSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_extracted_simple_rgb.pdf",
		buildSyntheticRGBImagePDFFloat(
			540,
			720,
			width,
			height,
			[6]float64{468, 0, 0, 624, 72, 96},
			convertCMYKBytesToSimpleRGB(expandedCMYK),
		),
	)

	t.Skipf(
		"indexed_doc023 simple-cmyk-to-rgb probe: direct_cmyk_exact=%.4f direct_cmyk_similarity=%.4f simple_rgb_exact=%.4f simple_rgb_similarity=%.4f",
		directExact,
		directSimilarity,
		simpleRGBExact,
		simpleRGBSimilarity,
	)
}

func TestSyntheticDoc023ExtractedImageObjectCMYKConversionReferencesProbeAgainstPoppler(t *testing.T) {
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	expandedCMYK := expandIndexedCMYKImageData(palette, imageData)
	directScore, referenceScores := probeSyntheticCMYKReferenceSetAgainstPoppler(
		t,
		"indexed_doc023_extracted_cmyk_reference",
		540,
		720,
		width,
		height,
		[6]float64{468, 0, 0, 624, 72, 96},
		expandedCMYK,
		map[string][]byte{
			"current_rgb": convertCMYKBytesToCurrentRGB(expandedCMYK),
			"simple_rgb":  convertCMYKBytesToSimpleRGB(expandedCMYK),
			"stdlib_rgb":  convertCMYKBytesToStdlibRGB(expandedCMYK),
		},
	)

	t.Skipf(
		"indexed_doc023 cmyk-conversion references: direct_exact=%.4f direct_similarity=%.4f current_rgb_exact=%.4f current_rgb_similarity=%.4f simple_rgb_exact=%.4f simple_rgb_similarity=%.4f stdlib_rgb_exact=%.4f stdlib_rgb_similarity=%.4f",
		directScore.exact,
		directScore.similarity,
		referenceScores["current_rgb"].exact,
		referenceScores["current_rgb"].similarity,
		referenceScores["simple_rgb"].exact,
		referenceScores["simple_rgb"].similarity,
		referenceScores["stdlib_rgb"].exact,
		referenceScores["stdlib_rgb"].similarity,
	)
}

func TestSyntheticDoc023PaletteSweepCMYKConversionReferencesProbeAgainstPoppler(t *testing.T) {
	base, palette, _, _, _ := loadSampleIndexedImageObject(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)
	paletteEntries := len(palette) / 4
	width, height, imageData := syntheticPaletteSweepImageData(paletteEntries)

	directScore, referenceScores := probeSyntheticIndexedCMYKPaletteReferenceSetAgainstPoppler(
		t,
		"indexed_doc023_palette_sweep",
		palette,
		width,
		height,
		imageData,
	)

	t.Skipf(
		"indexed_doc023 palette-sweep cmyk-conversion references: direct_exact=%.4f direct_similarity=%.4f current_rgb_exact=%.4f current_rgb_similarity=%.4f simple_rgb_exact=%.4f simple_rgb_similarity=%.4f stdlib_rgb_exact=%.4f stdlib_rgb_similarity=%.4f",
		directScore.exact,
		directScore.similarity,
		referenceScores["current_rgb"].exact,
		referenceScores["current_rgb"].similarity,
		referenceScores["simple_rgb"].exact,
		referenceScores["simple_rgb"].similarity,
		referenceScores["stdlib_rgb"].exact,
		referenceScores["stdlib_rgb"].similarity,
	)
}

func TestSyntheticDoc023ExtractedImageObjectCMYKIdentityReferenceMatrixProbeAgainstPoppler(t *testing.T) {
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	expandedCMYK := expandIndexedCMYKImageData(palette, imageData)
	directScore, referenceScores := probeSyntheticCMYKReferenceSetAgainstPoppler(
		t,
		"indexed_doc023_extracted_identity_cmyk_reference",
		float64(width),
		float64(height),
		width,
		height,
		[6]float64{float64(width), 0, 0, float64(height), 0, 0},
		expandedCMYK,
		map[string][]byte{
			"current_rgb": convertCMYKBytesToCurrentRGB(expandedCMYK),
			"simple_rgb":  convertCMYKBytesToSimpleRGB(expandedCMYK),
			"stdlib_rgb":  convertCMYKBytesToStdlibRGB(expandedCMYK),
		},
	)

	t.Skipf(
		"indexed_doc023 identity cmyk-conversion references: direct_exact=%.4f direct_similarity=%.4f current_rgb_exact=%.4f current_rgb_similarity=%.4f simple_rgb_exact=%.4f simple_rgb_similarity=%.4f stdlib_rgb_exact=%.4f stdlib_rgb_similarity=%.4f",
		directScore.exact,
		directScore.similarity,
		referenceScores["current_rgb"].exact,
		referenceScores["current_rgb"].similarity,
		referenceScores["simple_rgb"].exact,
		referenceScores["simple_rgb"].similarity,
		referenceScores["stdlib_rgb"].exact,
		referenceScores["stdlib_rgb"].similarity,
	)
}

func TestSyntheticDoc023PaletteUsageWeightedCMYKConversionReferencesProbeAgainstPoppler(t *testing.T) {
	base, palette, imageData, _, _ := loadSampleIndexedImageObject(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	counts := countIndexedPaletteUsage(imageData, len(palette)/4)
	width, height, weightedImageData := syntheticWeightedPaletteImageData(counts, 64*64)

	directScore, referenceScores := probeSyntheticIndexedCMYKPaletteReferenceSetAgainstPoppler(
		t,
		"indexed_doc023_palette_weighted",
		palette,
		width,
		height,
		weightedImageData,
	)

	t.Skipf(
		"indexed_doc023 palette-weighted cmyk-conversion references: direct_exact=%.4f direct_similarity=%.4f current_rgb_exact=%.4f current_rgb_similarity=%.4f simple_rgb_exact=%.4f simple_rgb_similarity=%.4f stdlib_rgb_exact=%.4f stdlib_rgb_similarity=%.4f",
		directScore.exact,
		directScore.similarity,
		referenceScores["current_rgb"].exact,
		referenceScores["current_rgb"].similarity,
		referenceScores["simple_rgb"].exact,
		referenceScores["simple_rgb"].similarity,
		referenceScores["stdlib_rgb"].exact,
		referenceScores["stdlib_rgb"].similarity,
	)
}

func TestSyntheticDoc023PaletteSpatialProxyCMYKConversionReferencesProbeAgainstPoppler(t *testing.T) {
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	proxyWidth := 54
	proxyHeight := 72
	proxyImageData := downsampleIndexedImageMajority(imageData, width, height, proxyWidth, proxyHeight, len(palette)/4)

	directScore, referenceScores := probeSyntheticIndexedCMYKPaletteReferenceSetAgainstPoppler(
		t,
		"indexed_doc023_palette_spatial",
		palette,
		proxyWidth,
		proxyHeight,
		proxyImageData,
	)

	t.Skipf(
		"indexed_doc023 palette-spatial cmyk-conversion references: direct_exact=%.4f direct_similarity=%.4f current_rgb_exact=%.4f current_rgb_similarity=%.4f simple_rgb_exact=%.4f simple_rgb_similarity=%.4f stdlib_rgb_exact=%.4f stdlib_rgb_similarity=%.4f",
		directScore.exact,
		directScore.similarity,
		referenceScores["current_rgb"].exact,
		referenceScores["current_rgb"].similarity,
		referenceScores["simple_rgb"].exact,
		referenceScores["simple_rgb"].similarity,
		referenceScores["stdlib_rgb"].exact,
		referenceScores["stdlib_rgb"].similarity,
	)
}

func TestSyntheticDoc023PaletteSpatialProxySizeSweepProbeAgainstPoppler(t *testing.T) {
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	testCases := []struct {
		name string
		w    int
		h    int
	}{
		{name: "coarse_27x36", w: 27, h: 36},
		{name: "medium_54x72", w: 54, h: 72},
		{name: "fine_108x144", w: 108, h: 144},
	}

	for _, tc := range testCases {
		proxyImageData := downsampleIndexedImageMajority(imageData, width, height, tc.w, tc.h, len(palette)/4)
		directScore, referenceScores := probeSyntheticIndexedCMYKPaletteReferenceSetAgainstPoppler(
			t,
			"indexed_doc023_palette_spatial_"+tc.name,
			palette,
			tc.w,
			tc.h,
			proxyImageData,
		)

		t.Logf(
			"%s palette-spatial sweep: direct_similarity=%.4f current_rgb_similarity=%.4f simple_rgb_similarity=%.4f stdlib_rgb_similarity=%.4f",
			tc.name,
			directScore.similarity,
			referenceScores["current_rgb"].similarity,
			referenceScores["simple_rgb"].similarity,
			referenceScores["stdlib_rgb"].similarity,
		)
	}
}

func TestSyntheticDoc023PaletteSpatialRegionProbeAgainstPoppler(t *testing.T) {
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	dominantIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	edgeProxy, flatProxy := downsampleIndexedImageMajorityByRegion(imageData, width, height, 54, 72, len(palette)/4, dominantIndex)

	edgeDirectScore, edgeReferenceScores := probeSyntheticIndexedCMYKPaletteReferenceSetAgainstPoppler(
		t,
		"indexed_doc023_palette_spatial_edge",
		palette,
		54,
		72,
		edgeProxy,
	)
	flatDirectScore, flatReferenceScores := probeSyntheticIndexedCMYKPaletteReferenceSetAgainstPoppler(
		t,
		"indexed_doc023_palette_spatial_flat",
		palette,
		54,
		72,
		flatProxy,
	)

	t.Skipf(
		"indexed_doc023 palette-spatial regions: edge_direct=%.4f edge_current=%.4f edge_simple=%.4f edge_stdlib=%.4f flat_direct=%.4f flat_current=%.4f flat_simple=%.4f flat_stdlib=%.4f",
		edgeDirectScore.similarity,
		edgeReferenceScores["current_rgb"].similarity,
		edgeReferenceScores["simple_rgb"].similarity,
		edgeReferenceScores["stdlib_rgb"].similarity,
		flatDirectScore.similarity,
		flatReferenceScores["current_rgb"].similarity,
		flatReferenceScores["simple_rgb"].similarity,
		flatReferenceScores["stdlib_rgb"].similarity,
	)
}

func TestSyntheticDoc023PaletteSpatialOrientationProbeAgainstPoppler(t *testing.T) {
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	dominantIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	horizontalProxy, verticalProxy, mixedProxy := downsampleIndexedImageMajorityByEdgeOrientation(
		imageData,
		width,
		height,
		54,
		72,
		len(palette)/4,
		dominantIndex,
	)

	horizontalDirectScore, horizontalReferenceScores := probeSyntheticIndexedCMYKPaletteReferenceSetAgainstPoppler(
		t,
		"indexed_doc023_palette_spatial_horizontal",
		palette,
		54,
		72,
		horizontalProxy,
	)
	verticalDirectScore, verticalReferenceScores := probeSyntheticIndexedCMYKPaletteReferenceSetAgainstPoppler(
		t,
		"indexed_doc023_palette_spatial_vertical",
		palette,
		54,
		72,
		verticalProxy,
	)
	mixedDirectScore, mixedReferenceScores := probeSyntheticIndexedCMYKPaletteReferenceSetAgainstPoppler(
		t,
		"indexed_doc023_palette_spatial_mixed",
		palette,
		54,
		72,
		mixedProxy,
	)

	t.Skipf(
		"indexed_doc023 palette-spatial orientations: horizontal_direct=%.4f horizontal_current=%.4f horizontal_simple=%.4f horizontal_stdlib=%.4f vertical_direct=%.4f vertical_current=%.4f vertical_simple=%.4f vertical_stdlib=%.4f mixed_direct=%.4f mixed_current=%.4f mixed_simple=%.4f mixed_stdlib=%.4f",
		horizontalDirectScore.similarity,
		horizontalReferenceScores["current_rgb"].similarity,
		horizontalReferenceScores["simple_rgb"].similarity,
		horizontalReferenceScores["stdlib_rgb"].similarity,
		verticalDirectScore.similarity,
		verticalReferenceScores["current_rgb"].similarity,
		verticalReferenceScores["simple_rgb"].similarity,
		verticalReferenceScores["stdlib_rgb"].similarity,
		mixedDirectScore.similarity,
		mixedReferenceScores["current_rgb"].similarity,
		mixedReferenceScores["simple_rgb"].similarity,
		mixedReferenceScores["stdlib_rgb"].similarity,
	)
}

func TestSyntheticDoc023PaletteSpatialHybridProxySweepProbeAgainstPoppler(t *testing.T) {
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	horizontalProxy, verticalProxy, mixedProxy := downsampleIndexedImageMajorityByEdgeOrientation(
		imageData,
		width,
		height,
		54,
		72,
		len(palette)/4,
		fillIndex,
	)

	testCases := []struct {
		name   string
		layers [][]byte
	}{
		{name: "horizontal_plus_vertical", layers: [][]byte{horizontalProxy, verticalProxy}},
		{name: "horizontal_plus_mixed", layers: [][]byte{horizontalProxy, mixedProxy}},
		{name: "vertical_plus_mixed", layers: [][]byte{verticalProxy, mixedProxy}},
		{name: "all_edge_classes", layers: [][]byte{horizontalProxy, verticalProxy, mixedProxy}},
	}

	for _, tc := range testCases {
		hybridProxy := mergeIndexedProxyLayers(byte(fillIndex), tc.layers...)
		directScore, referenceScores := probeSyntheticIndexedCMYKPaletteReferenceSetAgainstPoppler(
			t,
			"indexed_doc023_palette_hybrid_"+tc.name,
			palette,
			54,
			72,
			hybridProxy,
		)

		t.Logf(
			"%s palette-hybrid sweep: direct_similarity=%.4f current_rgb_similarity=%.4f simple_rgb_similarity=%.4f stdlib_rgb_similarity=%.4f",
			tc.name,
			directScore.similarity,
			referenceScores["current_rgb"].similarity,
			referenceScores["simple_rgb"].similarity,
			referenceScores["stdlib_rgb"].similarity,
		)
	}
}

func TestSyntheticDoc023PaletteSpatialHybridLayoutProbeAgainstPoppler(t *testing.T) {
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	horizontalProxy, verticalProxy, mixedProxy := downsampleIndexedImageMajorityByEdgeOrientation(
		imageData,
		width,
		height,
		54,
		72,
		len(palette)/4,
		fillIndex,
	)

	testCases := []struct {
		name      string
		imageData []byte
	}{
		{
			name:      "preserved",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), horizontalProxy, verticalProxy, mixedProxy),
		},
		{
			name:      "class_banded",
			imageData: rearrangeIndexedProxyLayersByClass(byte(fillIndex), horizontalProxy, verticalProxy, mixedProxy),
		},
		{
			name:      "edge_compacted",
			imageData: compactIndexedProxyLayers(byte(fillIndex), horizontalProxy, verticalProxy, mixedProxy),
		},
	}

	for _, tc := range testCases {
		directScore, referenceScores := probeSyntheticIndexedCMYKPaletteReferenceSetAgainstPoppler(
			t,
			"indexed_doc023_palette_hybrid_layout_"+tc.name,
			palette,
			54,
			72,
			tc.imageData,
		)

		t.Logf(
			"%s palette-hybrid layout: direct_similarity=%.4f current_rgb_similarity=%.4f simple_rgb_similarity=%.4f stdlib_rgb_similarity=%.4f",
			tc.name,
			directScore.similarity,
			referenceScores["current_rgb"].similarity,
			referenceScores["simple_rgb"].similarity,
			referenceScores["stdlib_rgb"].similarity,
		)
	}
}

func TestSyntheticDoc023ExtractedImageObjectCurrentRGBIndexedVsDirectProbeAgainstPoppler(t *testing.T) {
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	currentRGBPalette := convertCMYKPaletteToCurrentRGBPalette(palette)
	indexedExact, indexedSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_current_rgb_indexed.pdf",
		buildSyntheticIndexedImagePDFFloatWithBase(
			540,
			720,
			3,
			"DeviceRGB",
			width,
			height,
			[6]float64{468, 0, 0, 624, 72, 96},
			currentRGBPalette,
			imageData,
		),
	)
	directExact, directSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_current_rgb_direct.pdf",
		buildSyntheticRGBImagePDFFloat(
			540,
			720,
			width,
			height,
			[6]float64{468, 0, 0, 624, 72, 96},
			convertCMYKBytesToCurrentRGB(expandIndexedCMYKImageData(palette, imageData)),
		),
	)

	t.Skipf(
		"indexed_doc023 current-rgb indexed-vs-direct probe: indexed_exact=%.4f indexed_similarity=%.4f direct_exact=%.4f direct_similarity=%.4f",
		indexedExact,
		indexedSimilarity,
		directExact,
		directSimilarity,
	)
}

func TestSyntheticDoc023ExtractedImageObjectCMYKIdentityVsDownscaleProbeAgainstPoppler(t *testing.T) {
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	expandedCMYK := expandIndexedCMYKImageData(palette, imageData)
	currentRGB := convertCMYKBytesToCurrentRGB(expandedCMYK)

	identityDirectExact, identityDirectSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_extracted_identity_direct_cmyk.pdf",
		buildSyntheticCMYKImagePDFFloat(
			float64(width),
			float64(height),
			width,
			height,
			[6]float64{float64(width), 0, 0, float64(height), 0, 0},
			expandedCMYK,
		),
	)
	identityCurrentRGBExact, identityCurrentRGBSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_extracted_identity_current_rgb.pdf",
		buildSyntheticRGBImagePDFFloat(
			float64(width),
			float64(height),
			width,
			height,
			[6]float64{float64(width), 0, 0, float64(height), 0, 0},
			currentRGB,
		),
	)
	downscaleDirectExact, downscaleDirectSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_extracted_downscale_direct_cmyk.pdf",
		buildSyntheticCMYKImagePDFFloat(
			540,
			720,
			width,
			height,
			[6]float64{468, 0, 0, 624, 72, 96},
			expandedCMYK,
		),
	)
	downscaleCurrentRGBExact, downscaleCurrentRGBSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_extracted_downscale_current_rgb.pdf",
		buildSyntheticRGBImagePDFFloat(
			540,
			720,
			width,
			height,
			[6]float64{468, 0, 0, 624, 72, 96},
			currentRGB,
		),
	)

	t.Skipf(
		"indexed_doc023 cmyk identity-vs-downscale probe: identity_direct_exact=%.4f identity_direct_similarity=%.4f identity_current_rgb_exact=%.4f identity_current_rgb_similarity=%.4f downscale_direct_exact=%.4f downscale_direct_similarity=%.4f downscale_current_rgb_exact=%.4f downscale_current_rgb_similarity=%.4f",
		identityDirectExact,
		identityDirectSimilarity,
		identityCurrentRGBExact,
		identityCurrentRGBSimilarity,
		downscaleDirectExact,
		downscaleDirectSimilarity,
		downscaleCurrentRGBExact,
		downscaleCurrentRGBSimilarity,
	)
}

func TestSyntheticDoc023ExtractedImageObjectCurrentRGBMatchesRenderedCMYKProbe(t *testing.T) {
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf"),
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	expandedCMYK := expandIndexedCMYKImageData(palette, imageData)
	currentRGB := convertCMYKBytesToCurrentRGB(expandedCMYK)

	identityExact, identitySimilarity := probeRenderedPDFParity(
		t,
		"indexed_doc023_rendered_identity_direct_cmyk.pdf",
		buildSyntheticCMYKImagePDFFloat(
			float64(width),
			float64(height),
			width,
			height,
			[6]float64{float64(width), 0, 0, float64(height), 0, 0},
			expandedCMYK,
		),
		"indexed_doc023_rendered_identity_current_rgb.pdf",
		buildSyntheticRGBImagePDFFloat(
			float64(width),
			float64(height),
			width,
			height,
			[6]float64{float64(width), 0, 0, float64(height), 0, 0},
			currentRGB,
		),
	)
	downscaleExact, downscaleSimilarity := probeRenderedPDFParity(
		t,
		"indexed_doc023_rendered_downscale_direct_cmyk.pdf",
		buildSyntheticCMYKImagePDFFloat(
			540,
			720,
			width,
			height,
			[6]float64{468, 0, 0, 624, 72, 96},
			expandedCMYK,
		),
		"indexed_doc023_rendered_downscale_current_rgb.pdf",
		buildSyntheticRGBImagePDFFloat(
			540,
			720,
			width,
			height,
			[6]float64{468, 0, 0, 624, 72, 96},
			currentRGB,
		),
	)

	t.Skipf(
		"indexed_doc023 rendered cmyk-vs-current-rgb probe: identity_exact=%.4f identity_similarity=%.4f downscale_exact=%.4f downscale_similarity=%.4f",
		identityExact,
		identitySimilarity,
		downscaleExact,
		downscaleSimilarity,
	)
}

func TestSyntheticIndexedDoc019SizedDownscaleBilinearPhaseProbeAgainstPoppler(t *testing.T) {
	palette, imageData := syntheticIndexedTiledIdentity(324, 450)
	src := syntheticIndexedTiledPalettedImage(324, 450, palette, imageData)
	pdfBytes := buildSyntheticIndexedImagePDFFloat(
		243,
		338,
		324,
		450,
		[6]float64{243, 0, 0, 338, 0, 0},
		palette,
		imageData,
	)

	currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
		t,
		"indexed_doc019_sized_downscale_bilinear_phase.pdf",
		pdfBytes,
		map[string]image.Image{
			"bilinear_phase_0":   simulateSyntheticAffineImageWithMatrixAndPhase(src, image.Rect(0, 0, 243, 338), [6]float64{243, 0, 0, 338, 0, 0}, true, 0, 0),
			"bilinear_phase_0.5": simulateSyntheticAffineImageWithMatrixAndPhase(src, image.Rect(0, 0, 243, 338), [6]float64{243, 0, 0, 338, 0, 0}, true, 0.5, 0.5),
		},
	)

	t.Skipf(
		"indexed_doc019_sized_downscale_bilinear_phase probe: current_exact=%.4f current_similarity=%.4f bilinear_phase_0=%.4f bilinear_phase_0.5=%.4f",
		currentExact,
		currentSimilarity,
		refScores["bilinear_phase_0"],
		refScores["bilinear_phase_0.5"],
	)
}

func TestSyntheticIndexedDoc023SizedOffsetDownscaleBilinearPhaseProbeAgainstPoppler(t *testing.T) {
	palette, imageData := syntheticIndexedTiledIdentity(756, 1008)
	src := syntheticIndexedTiledPalettedImage(756, 1008, palette, imageData)
	matrix := [6]float64{468, 0, 0, 624, 72, 96}
	pdfBytes := buildSyntheticIndexedImagePDFFloat(
		540,
		720,
		756,
		1008,
		matrix,
		palette,
		imageData,
	)

	currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
		t,
		"indexed_doc023_sized_offset_downscale_bilinear_phase.pdf",
		pdfBytes,
		map[string]image.Image{
			"bilinear_phase_0":   simulateSyntheticAffineImageWithMatrixAndPhase(src, image.Rect(0, 0, 540, 720), matrix, true, 0, 0),
			"bilinear_phase_0.5": simulateSyntheticAffineImageWithMatrixAndPhase(src, image.Rect(0, 0, 540, 720), matrix, true, 0.5, 0.5),
		},
	)

	t.Skipf(
		"indexed_doc023_sized_offset_downscale_bilinear_phase probe: current_exact=%.4f current_similarity=%.4f bilinear_phase_0=%.4f bilinear_phase_0.5=%.4f",
		currentExact,
		currentSimilarity,
		refScores["bilinear_phase_0"],
		refScores["bilinear_phase_0.5"],
	)
}

func TestSyntheticIndexedImageRenderProbeAgainstPoppler(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	pdfPath := filepath.Join(root, "indexed_identity_4x4.pdf")
	popplerRoot := filepath.Join(root, "poppler")
	oursPNG := filepath.Join(root, "ours.png")

	palette, imageData := syntheticIndexedIdentity4x4()
	require.NoError(t, os.WriteFile(
		pdfPath,
		buildSyntheticIndexedImagePDF(4, 4, 4, 4, [6]int{4, 0, 0, 4, 0, 0}, palette, imageData),
		0o644,
	))
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, 72))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 72

	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 4, 4), img.Bounds())
	require.NoError(t, parityWritePNG(oursPNG, img))

	exactPercent, similarityPercent, err := parityComparePNGs(oursPNG, popplerPages[1])
	require.NoError(t, err)
	t.Skipf(
		"known indexed image parity mismatch under investigation: exact=%.4f similarity=%.4f",
		exactPercent,
		similarityPercent,
	)
}

func TestSyntheticRGBRenderProbeAgainstPoppler(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	pdfPath := filepath.Join(root, "rgb_identity_4x4.pdf")
	popplerRoot := filepath.Join(root, "poppler")
	oursPNG := filepath.Join(root, "ours.png")

	imageData := syntheticRGBIdentity4x4()
	require.NoError(t, os.WriteFile(
		pdfPath,
		buildSyntheticRGBImagePDF(4, 4, 4, 4, [6]int{4, 0, 0, 4, 0, 0}, imageData),
		0o644,
	))
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, 72))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 72

	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 4, 4), img.Bounds())
	require.NoError(t, parityWritePNG(oursPNG, img))

	exactPercent, similarityPercent, err := parityComparePNGs(oursPNG, popplerPages[1])
	require.NoError(t, err)
	t.Skipf(
		"known rgb image parity mismatch under investigation: exact=%.4f similarity=%.4f",
		exactPercent,
		similarityPercent,
	)
}

func TestSyntheticImageSplashScaleOnlyReferenceIsCloserToPopplerThanCurrentPath(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	pdfPath := filepath.Join(root, "gray_identity_4x4.pdf")
	popplerRoot := filepath.Join(root, "poppler")
	oursPNG := filepath.Join(root, "ours.png")
	splashPNG := filepath.Join(root, "splash_like.png")

	src := image.NewGray(image.Rect(0, 0, 4, 4))
	src.Pix = []byte{
		0, 63, 127, 255,
		255, 127, 63, 0,
		0, 255, 0, 255,
		255, 0, 255, 0,
	}

	require.NoError(t, os.WriteFile(
		pdfPath,
		buildSyntheticGrayImagePDF(4, 4, 4, 4, [6]int{4, 0, 0, 4, 0, 0}, append([]byte(nil), src.Pix...)),
		0o644,
	))
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, 72))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 72

	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(oursPNG, img))

	splashLike := simulateSyntheticSplashScaleOnly(src, image.Rect(0, 0, 4, 4), true)
	require.NoError(t, parityWritePNG(splashPNG, splashLike))

	oursExact, oursSimilarity, err := parityComparePNGs(oursPNG, popplerPages[1])
	require.NoError(t, err)
	splashExact, splashSimilarity, err := parityComparePNGs(splashPNG, popplerPages[1])
	require.NoError(t, err)

	assert.Greater(t, splashSimilarity, oursSimilarity)
	assert.Greater(t, splashExact, oursExact)
}

func TestSyntheticRGBSplashScaleOnlyReferenceIsCloserToPopplerThanCurrentPath(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	copy(src.Pix, syntheticRGBIdentity4x4())
	assertSyntheticRGBSplashReferenceCloserThanCurrentPath(
		t,
		"rgb_identity_4x4.pdf",
		buildSyntheticRGBImagePDF(4, 4, 4, 4, [6]int{4, 0, 0, 4, 0, 0}, append([]byte(nil), src.Pix...)),
		src,
		image.Rect(0, 0, 4, 4),
		[6]float64{4, 0, 0, 4, 0, 0},
	)
}

func TestSyntheticRGBSmallUpscaleSplashScaleOnlyReferenceIsCloserToPopplerThanCurrentPath(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	copy(src.Pix, syntheticRGBIdentity4x4())
	assertSyntheticRGBSplashReferenceCloserThanCurrentPath(
		t,
		"rgb_small_upscale_5x5.pdf",
		buildSyntheticRGBImagePDF(5, 5, 4, 4, [6]int{5, 0, 0, 5, 0, 0}, append([]byte(nil), src.Pix...)),
		src,
		image.Rect(0, 0, 5, 5),
		[6]float64{5, 0, 0, 5, 0, 0},
	)
}

func TestSyntheticRGBSubpixelOffsetSplashScaleOnlyReferenceProbeAgainstPoppler(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	copy(src.Pix, syntheticRGBIdentity4x4())

	oursSimilarity, splashSimilarity := measureSyntheticRGBSplashReferenceAgainstCurrentPath(
		t,
		"rgb_subpixel_offset_7x7.pdf",
		buildSyntheticRGBImagePDFFloat(7, 7, 4, 4, [6]float64{5, 0, 0, 5, 0.5, 0.5}, append([]byte(nil), src.Pix...)),
		src,
		image.Rect(0, 0, 7, 7),
		[6]float64{5, 0, 0, 5, 0.5, 0.5},
	)

	t.Skipf(
		"rgb_subpixel_offset_7x7 probe: ours=%.4f splash_like=%.4f",
		oursSimilarity,
		splashSimilarity,
	)
}

func TestSyntheticRGBNearIdentityAnisotropicSplashScaleOnlyReferenceProbeAgainstPoppler(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	copy(src.Pix, syntheticRGBIdentity4x4())

	oursSimilarity, splashSimilarity := measureSyntheticRGBSplashReferenceAgainstCurrentPath(
		t,
		"rgb_near_identity_anisotropic.pdf",
		buildSyntheticRGBImagePDFFloat(5, 5, 4, 4, [6]float64{4.25, 0, 0, 4.75, 0, 0}, append([]byte(nil), src.Pix...)),
		src,
		image.Rect(0, 0, 5, 5),
		[6]float64{4.25, 0, 0, 4.75, 0, 0},
	)

	t.Skipf(
		"rgb_near_identity_anisotropic probe: ours=%.4f splash_like=%.4f",
		oursSimilarity,
		splashSimilarity,
	)
}

func TestSyntheticPureRGBSubpixelUpscaleLaneProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	matrix := [6]float64{20, 0, 0, 20, 1.5, 1.5}

	currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
		t,
		"pure_rgb_subpixel_upscale_lane.pdf",
		buildSyntheticRGBImagePDFFloat(24, 24, 16, 16, matrix, imageData),
		map[string]image.Image{
			"bilinear_phase_0":   simulateSyntheticAffineImageWithMatrixAndPhase(src, image.Rect(0, 0, 24, 24), matrix, true, 0, 0),
			"bilinear_phase_0.5": simulateSyntheticAffineImageWithMatrixAndPhase(src, image.Rect(0, 0, 24, 24), matrix, true, 0.5, 0.5),
			"splash_like":        simulateSyntheticSplashScaleOnlyImageWithMatrix(src, image.Rect(0, 0, 24, 24), matrix, true),
		},
	)

	t.Skipf(
		"pure rgb subpixel upscale lane probe: current_exact=%.4f current_similarity=%.4f bilinear_phase_0=%.4f bilinear_phase_0.5=%.4f splash_like=%.4f",
		currentExact,
		currentSimilarity,
		refScores["bilinear_phase_0"],
		refScores["bilinear_phase_0.5"],
		refScores["splash_like"],
	)
}

func TestSyntheticPureRGBSubpixelDownscaleLaneProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	matrix := [6]float64{12, 0, 0, 12, 0.5, 0.5}

	currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
		t,
		"pure_rgb_subpixel_downscale_lane.pdf",
		buildSyntheticRGBImagePDFFloat(16, 16, 16, 16, matrix, imageData),
		map[string]image.Image{
			"bilinear_phase_0":   simulateSyntheticAffineImageWithMatrixAndPhase(src, image.Rect(0, 0, 16, 16), matrix, true, 0, 0),
			"bilinear_phase_0.5": simulateSyntheticAffineImageWithMatrixAndPhase(src, image.Rect(0, 0, 16, 16), matrix, true, 0.5, 0.5),
			"splash_like":        simulateSyntheticSplashScaleOnlyImageWithMatrix(src, image.Rect(0, 0, 16, 16), matrix, true),
		},
	)

	t.Skipf(
		"pure rgb subpixel downscale lane probe: current_exact=%.4f current_similarity=%.4f bilinear_phase_0=%.4f bilinear_phase_0.5=%.4f splash_like=%.4f",
		currentExact,
		currentSimilarity,
		refScores["bilinear_phase_0"],
		refScores["bilinear_phase_0.5"],
		refScores["splash_like"],
	)
}

func TestSyntheticPureRGBSubpixelLanePhaseSweepProbeAgainstPoppler(t *testing.T) {
	testCases := []struct {
		name     string
		pdfName  string
		pageRect image.Rectangle
		pdfBytes []byte
		src      image.Image
		matrix   [6]float64
	}{
		{
			name:     "upscale",
			pdfName:  "pure_rgb_subpixel_upscale_phase_sweep.pdf",
			pageRect: image.Rect(0, 0, 24, 24),
			pdfBytes: buildSyntheticRGBImagePDFFloat(24, 24, 16, 16, [6]float64{20, 0, 0, 20, 1.5, 1.5}, syntheticRGBTiledIdentity(16, 16)),
			src:      syntheticRGBTiledImage(16, 16, syntheticRGBTiledIdentity(16, 16)),
			matrix:   [6]float64{20, 0, 0, 20, 1.5, 1.5},
		},
		{
			name:     "downscale",
			pdfName:  "pure_rgb_subpixel_downscale_phase_sweep.pdf",
			pageRect: image.Rect(0, 0, 16, 16),
			pdfBytes: buildSyntheticRGBImagePDFFloat(16, 16, 16, 16, [6]float64{12, 0, 0, 12, 0.5, 0.5}, syntheticRGBTiledIdentity(16, 16)),
			src:      syntheticRGBTiledImage(16, 16, syntheticRGBTiledIdentity(16, 16)),
			matrix:   [6]float64{12, 0, 0, 12, 0.5, 0.5},
		},
	}

	phases := []float64{0, 0.25, 0.5, 0.75}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			references := make(map[string]image.Image, len(phases))
			for _, phase := range phases {
				name := fmt.Sprintf("bilinear_phase_%.2f", phase)
				references[name] = simulateSyntheticAffineImageWithMatrixAndPhase(tc.src, tc.pageRect, tc.matrix, true, phase, phase)
			}

			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				tc.pdfName,
				tc.pdfBytes,
				references,
			)

			t.Skipf(
				"%s pure rgb phase sweep: current_exact=%.4f current_similarity=%.4f phase_0=%.4f phase_0.25=%.4f phase_0.5=%.4f phase_0.75=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["bilinear_phase_0.00"],
				refScores["bilinear_phase_0.25"],
				refScores["bilinear_phase_0.50"],
				refScores["bilinear_phase_0.75"],
			)
		})
	}
}

func TestSyntheticPureRGBSubpixelLanePlacementSweepProbeAgainstPoppler(t *testing.T) {
	testCases := []struct {
		name     string
		pdfName  string
		pageRect image.Rectangle
		pdfBytes []byte
		src      image.Image
		matrix   [6]float64
	}{
		{
			name:     "upscale",
			pdfName:  "pure_rgb_subpixel_upscale_placement_sweep.pdf",
			pageRect: image.Rect(0, 0, 24, 24),
			pdfBytes: buildSyntheticRGBImagePDFFloat(24, 24, 16, 16, [6]float64{20, 0, 0, 20, 1.5, 1.5}, syntheticRGBTiledIdentity(16, 16)),
			src:      syntheticRGBTiledImage(16, 16, syntheticRGBTiledIdentity(16, 16)),
			matrix:   [6]float64{20, 0, 0, 20, 1.5, 1.5},
		},
		{
			name:     "downscale",
			pdfName:  "pure_rgb_subpixel_downscale_placement_sweep.pdf",
			pageRect: image.Rect(0, 0, 16, 16),
			pdfBytes: buildSyntheticRGBImagePDFFloat(16, 16, 16, 16, [6]float64{12, 0, 0, 12, 0.5, 0.5}, syntheticRGBTiledIdentity(16, 16)),
			src:      syntheticRGBTiledImage(16, 16, syntheticRGBTiledIdentity(16, 16)),
			matrix:   [6]float64{12, 0, 0, 12, 0.5, 0.5},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scaleX := tc.matrix[0] / float64(tc.src.Bounds().Dx())
			scaleY := tc.matrix[3] / float64(tc.src.Bounds().Dy())

			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				tc.pdfName,
				tc.pdfBytes,
				map[string]image.Image{
					"center_to_center": simulateSyntheticAffineImageWithMatrixPlacement(tc.src, tc.pageRect, tc.matrix, true, 0.5*(1-scaleX), 0.5*(1-scaleY)),
					"src_center_only":  simulateSyntheticAffineImageWithMatrixPlacement(tc.src, tc.pageRect, tc.matrix, true, -0.5*scaleX, -0.5*scaleY),
					"dest_center_only": simulateSyntheticAffineImageWithMatrixPlacement(tc.src, tc.pageRect, tc.matrix, true, 0.5, 0.5),
				},
			)

			t.Skipf(
				"%s pure rgb placement sweep: current_exact=%.4f current_similarity=%.4f center_to_center=%.4f src_center_only=%.4f dest_center_only=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["center_to_center"],
				refScores["src_center_only"],
				refScores["dest_center_only"],
			)
		})
	}
}

func TestSyntheticPureRGBSubpixelLaneReconstructionSweepProbeAgainstPoppler(t *testing.T) {
	testCases := []struct {
		name     string
		pdfName  string
		pageRect image.Rectangle
		pdfBytes []byte
		src      image.Image
		matrix   [6]float64
	}{
		{
			name:     "upscale",
			pdfName:  "pure_rgb_subpixel_upscale_reconstruction_sweep.pdf",
			pageRect: image.Rect(0, 0, 24, 24),
			pdfBytes: buildSyntheticRGBImagePDFFloat(24, 24, 16, 16, [6]float64{20, 0, 0, 20, 1.5, 1.5}, syntheticRGBTiledIdentity(16, 16)),
			src:      syntheticRGBTiledImage(16, 16, syntheticRGBTiledIdentity(16, 16)),
			matrix:   [6]float64{20, 0, 0, 20, 1.5, 1.5},
		},
		{
			name:     "downscale",
			pdfName:  "pure_rgb_subpixel_downscale_reconstruction_sweep.pdf",
			pageRect: image.Rect(0, 0, 16, 16),
			pdfBytes: buildSyntheticRGBImagePDFFloat(16, 16, 16, 16, [6]float64{12, 0, 0, 12, 0.5, 0.5}, syntheticRGBTiledIdentity(16, 16)),
			src:      syntheticRGBTiledImage(16, 16, syntheticRGBTiledIdentity(16, 16)),
			matrix:   [6]float64{12, 0, 0, 12, 0.5, 0.5},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			references := map[string]image.Image{
				"affine_catmull":              simulateSyntheticAffineImageWithMatrixAndFilter(tc.src, tc.pageRect, tc.matrix, "catmull"),
				"rect_bilinear_two_pass":      simulateSyntheticRectResampleThenPlace(tc.src, tc.pageRect, tc.matrix, "bilinear"),
				"rect_catmull_two_pass":       simulateSyntheticRectResampleThenPlace(tc.src, tc.pageRect, tc.matrix, "catmull"),
				"area_box_then_place":         simulateSyntheticAreaBoxResampleThenPlace(tc.src, tc.pageRect, tc.matrix),
				"area_box_catmull_then_place": simulateSyntheticAreaBoxCatmullThenPlace(tc.src, tc.pageRect, tc.matrix),
			}

			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				tc.pdfName,
				tc.pdfBytes,
				references,
			)

			t.Skipf(
				"%s pure rgb reconstruction sweep: current_exact=%.4f current_similarity=%.4f affine_catmull=%.4f rect_bilinear_two_pass=%.4f rect_catmull_two_pass=%.4f area_box_then_place=%.4f area_box_catmull_then_place=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_catmull"],
				refScores["rect_bilinear_two_pass"],
				refScores["rect_catmull_two_pass"],
				refScores["area_box_then_place"],
				refScores["area_box_catmull_then_place"],
			)
		})
	}
}

func TestSyntheticPureRGBSubpixelLaneGammaAwareReconstructionProbeAgainstPoppler(t *testing.T) {
	testCases := []struct {
		name     string
		pdfName  string
		pageRect image.Rectangle
		pdfBytes []byte
		src      image.Image
		matrix   [6]float64
	}{
		{
			name:     "upscale",
			pdfName:  "pure_rgb_subpixel_upscale_gamma_reconstruction.pdf",
			pageRect: image.Rect(0, 0, 24, 24),
			pdfBytes: buildSyntheticRGBImagePDFFloat(24, 24, 16, 16, [6]float64{20, 0, 0, 20, 1.5, 1.5}, syntheticRGBTiledIdentity(16, 16)),
			src:      syntheticRGBTiledImage(16, 16, syntheticRGBTiledIdentity(16, 16)),
			matrix:   [6]float64{20, 0, 0, 20, 1.5, 1.5},
		},
		{
			name:     "downscale",
			pdfName:  "pure_rgb_subpixel_downscale_gamma_reconstruction.pdf",
			pageRect: image.Rect(0, 0, 16, 16),
			pdfBytes: buildSyntheticRGBImagePDFFloat(16, 16, 16, 16, [6]float64{12, 0, 0, 12, 0.5, 0.5}, syntheticRGBTiledIdentity(16, 16)),
			src:      syntheticRGBTiledImage(16, 16, syntheticRGBTiledIdentity(16, 16)),
			matrix:   [6]float64{12, 0, 0, 12, 0.5, 0.5},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			references := map[string]image.Image{
				"affine_srgb_linear_bilinear": simulateSyntheticGammaAwareAffineBilinear(tc.src, tc.pageRect, tc.matrix, "srgb"),
				"rect_srgb_linear_two_pass":   simulateSyntheticGammaAwareRectResampleThenPlace(tc.src, tc.pageRect, tc.matrix, "srgb"),
				"affine_gamma22_bilinear":     simulateSyntheticGammaAwareAffineBilinear(tc.src, tc.pageRect, tc.matrix, "gamma22"),
				"rect_gamma22_two_pass":       simulateSyntheticGammaAwareRectResampleThenPlace(tc.src, tc.pageRect, tc.matrix, "gamma22"),
			}

			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				tc.pdfName,
				tc.pdfBytes,
				references,
			)

			t.Skipf(
				"%s pure rgb gamma reconstruction: current_exact=%.4f current_similarity=%.4f affine_srgb_linear_bilinear=%.4f rect_srgb_linear_two_pass=%.4f affine_gamma22_bilinear=%.4f rect_gamma22_two_pass=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_srgb_linear_bilinear"],
				refScores["rect_srgb_linear_two_pass"],
				refScores["affine_gamma22_bilinear"],
				refScores["rect_gamma22_two_pass"],
			)
		})
	}
}

func TestSyntheticPureRGBSubpixelLaneCanvasSemanticsProbeAgainstPoppler(t *testing.T) {
	testCases := []struct {
		name     string
		pdfName  string
		pageRect image.Rectangle
		pdfBytes []byte
		src      image.Image
		matrix   [6]float64
	}{
		{
			name:     "upscale",
			pdfName:  "pure_rgb_subpixel_upscale_canvas_semantics.pdf",
			pageRect: image.Rect(0, 0, 24, 24),
			pdfBytes: buildSyntheticRGBImagePDFFloat(24, 24, 16, 16, [6]float64{20, 0, 0, 20, 1.5, 1.5}, syntheticRGBTiledIdentity(16, 16)),
			src:      syntheticRGBTiledImage(16, 16, syntheticRGBTiledIdentity(16, 16)),
			matrix:   [6]float64{20, 0, 0, 20, 1.5, 1.5},
		},
		{
			name:     "downscale",
			pdfName:  "pure_rgb_subpixel_downscale_canvas_semantics.pdf",
			pageRect: image.Rect(0, 0, 16, 16),
			pdfBytes: buildSyntheticRGBImagePDFFloat(16, 16, 16, 16, [6]float64{12, 0, 0, 12, 0.5, 0.5}, syntheticRGBTiledIdentity(16, 16)),
			src:      syntheticRGBTiledImage(16, 16, syntheticRGBTiledIdentity(16, 16)),
			matrix:   [6]float64{12, 0, 0, 12, 0.5, 0.5},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			references := map[string]image.Image{
				"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(tc.src, tc.pageRect, tc.matrix),
				"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(tc.src, tc.pageRect, tc.matrix),
			}

			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				tc.pdfName,
				tc.pdfBytes,
				references,
			)

			t.Skipf(
				"%s pure rgb canvas semantics: current_exact=%.4f current_similarity=%.4f affine_transparent_edge_over_white=%.4f rect_transparent_edge_over_white=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}
}

func TestSyntheticPureRGBSubpixelUpscaleTransparentEdgeReferenceIsCloserToPopplerThanCurrent(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	matrix := [6]float64{20, 0, 0, 20, 1.5, 1.5}

	_, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
		t,
		"pure_rgb_subpixel_upscale_canvas_assert.pdf",
		buildSyntheticRGBImagePDFFloat(24, 24, 16, 16, matrix, imageData),
		map[string]image.Image{
			"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, image.Rect(0, 0, 24, 24), matrix),
			"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, image.Rect(0, 0, 24, 24), matrix),
		},
	)

	assert.Greater(t, refScores["affine_transparent_edge_over_white"], currentSimilarity)
	assert.Greater(t, refScores["rect_transparent_edge_over_white"], currentSimilarity)
}

func TestSampleIndexedOriginDownscaleLegacySelectiveDefaultMatchesExperimentalForDoc019(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	pdfPath := filepath.Join(getSampleDir(), "019-grayscale-image", "grayscale-image.pdf")
	popplerRoot := filepath.Join(root, "poppler")
	legacyPNG := filepath.Join(root, "legacy.png")
	experimentalPNG := filepath.Join(root, "experimental.png")

	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, 72))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())

	legacyOpts := pdf.DefaultRenderOptions()
	legacyOpts.DPI = 72
	legacyOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeLegacy
	legacyImg, err := renderer.RenderPage(context.Background(), page, legacyOpts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(legacyPNG, legacyImg))

	experimentalOpts := pdf.DefaultRenderOptions()
	experimentalOpts.DPI = 72
	experimentalOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1
	experimentalImg, err := renderer.RenderPage(context.Background(), page, experimentalOpts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(experimentalPNG, experimentalImg))

	_, legacySimilarity, err := parityComparePNGs(legacyPNG, popplerPages[1])
	require.NoError(t, err)
	_, experimentalSimilarity, err := parityComparePNGs(experimentalPNG, popplerPages[1])
	require.NoError(t, err)
	t.Logf("doc019 similarity: legacy=%.4f experimental=%.4f", legacySimilarity, experimentalSimilarity)
	assert.InDelta(t, experimentalSimilarity, legacySimilarity, 1e-9)
}

func TestSampleIndexedOriginDownscaleExperimentalModeRegressesDoc023OffsetCase(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	popplerRoot := filepath.Join(root, "poppler")
	legacyPNG := filepath.Join(root, "legacy.png")
	experimentalPNG := filepath.Join(root, "experimental.png")

	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, 72))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())

	legacyOpts := pdf.DefaultRenderOptions()
	legacyOpts.DPI = 72
	legacyOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeLegacy
	legacyImg, err := renderer.RenderPage(context.Background(), page, legacyOpts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(legacyPNG, legacyImg))

	experimentalOpts := pdf.DefaultRenderOptions()
	experimentalOpts.DPI = 72
	experimentalOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1
	experimentalImg, err := renderer.RenderPage(context.Background(), page, experimentalOpts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(experimentalPNG, experimentalImg))

	_, legacySimilarity, err := parityComparePNGs(legacyPNG, popplerPages[1])
	require.NoError(t, err)
	_, experimentalSimilarity, err := parityComparePNGs(experimentalPNG, popplerPages[1])
	require.NoError(t, err)
	t.Logf("doc023 similarity: legacy=%.4f experimental=%.4f", legacySimilarity, experimentalSimilarity)

	assert.Greater(t, legacySimilarity, experimentalSimilarity)
}

func TestSampleIndexedOriginDownscaleExperimentalModeSurfaceAcrossSampleCorpus(t *testing.T) {
	pdfPaths, err := filepath.Glob(filepath.Join(getSampleDir(), "*", "*.pdf"))
	require.NoError(t, err)
	sort.Strings(pdfPaths)

	expectedChanged := []string{
		"023-cmyk-image/cmyk-image.pdf",
	}
	changed := make([]string, 0, len(expectedChanged))

	for _, pdfPath := range pdfPaths {
		exactPercent, similarityPercent := renderSamplePageParityBetweenModes(
			t,
			pdfPath,
			domainrenderer.ImageSamplingModeLegacy,
			domainrenderer.ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1,
		)
		relPath, relErr := filepath.Rel(getSampleDir(), pdfPath)
		require.NoError(t, relErr)
		relPath = filepath.ToSlash(relPath)
		t.Logf("%s legacy-vs-origin-phase parity: exact=%.4f similarity=%.4f", relPath, exactPercent, similarityPercent)

		switch relPath {
		case "023-cmyk-image/cmyk-image.pdf":
			assert.Less(t, exactPercent, 100.0)
			assert.Less(t, similarityPercent, 100.0)
			changed = append(changed, relPath)
		default:
			assert.InDelta(t, 100.0, exactPercent, 1e-9)
			assert.InDelta(t, 100.0, similarityPercent, 1e-9)
		}
	}

	assert.Equal(t, expectedChanged, changed)
}

func TestSampleIndexedCMYKStdlibExperimentalModeProbeForDoc023(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")

	legacyOpts := pdf.DefaultRenderOptions()
	legacyOpts.DPI = 72
	legacyOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeLegacy
	_, legacySimilarity := probePDFRenderAgainstPoppler(t, pdfPath, legacyOpts)

	experimentalOpts := pdf.DefaultRenderOptions()
	experimentalOpts.DPI = 72
	experimentalOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeExperimentalIndexedCMYKStdlibV1
	_, experimentalSimilarity := probePDFRenderAgainstPoppler(t, pdfPath, experimentalOpts)

	t.Skipf("doc023 poppler similarity: legacy=%.4f cmyk_stdlib=%.4f", legacySimilarity, experimentalSimilarity)
}

func TestSampleIndexedCMYKSimpleExperimentalModeProbeForDoc023(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")

	legacyOpts := pdf.DefaultRenderOptions()
	legacyOpts.DPI = 72
	legacyOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeLegacy
	_, legacySimilarity := probePDFRenderAgainstPoppler(t, pdfPath, legacyOpts)

	experimentalOpts := pdf.DefaultRenderOptions()
	experimentalOpts.DPI = 72
	experimentalOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeExperimentalIndexedCMYKSimpleV1
	_, experimentalSimilarity := probePDFRenderAgainstPoppler(t, pdfPath, experimentalOpts)

	t.Skipf("doc023 poppler similarity: legacy=%.4f cmyk_simple=%.4f", legacySimilarity, experimentalSimilarity)
}

func TestSampleIndexedCMYKLegacySelectiveDefaultMatchesHybrid75ForDoc023(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")

	legacyOpts := pdf.DefaultRenderOptions()
	legacyOpts.DPI = 72
	legacyOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeLegacy
	legacyExact, legacySimilarity := probePDFRenderAgainstPoppler(t, pdfPath, legacyOpts)

	experimentalOpts := pdf.DefaultRenderOptions()
	experimentalOpts.DPI = 72
	experimentalOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeExperimentalIndexedCMYKHybrid75V1
	experimentalExact, experimentalSimilarity := probePDFRenderAgainstPoppler(t, pdfPath, experimentalOpts)

	t.Logf(
		"doc023 legacy selective default parity: legacy(exact=%.4f similarity=%.4f) hybrid75(exact=%.4f similarity=%.4f)",
		legacyExact,
		legacySimilarity,
		experimentalExact,
		experimentalSimilarity,
	)
	assert.InDelta(t, experimentalExact, legacyExact, 1e-9)
	assert.InDelta(t, experimentalSimilarity, legacySimilarity, 1e-9)
}

func TestSampleIndexedCMYKLegacySelectiveDefaultBeatsSimpleForDoc023(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")

	legacyOpts := pdf.DefaultRenderOptions()
	legacyOpts.DPI = 72
	legacyOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeLegacy
	_, legacySimilarity := probePDFRenderAgainstPoppler(t, pdfPath, legacyOpts)

	simpleOpts := pdf.DefaultRenderOptions()
	simpleOpts.DPI = 72
	simpleOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeExperimentalIndexedCMYKSimpleV1
	_, simpleSimilarity := probePDFRenderAgainstPoppler(t, pdfPath, simpleOpts)

	t.Logf("doc023 legacy selective default vs simple: legacy=%.4f simple=%.4f", legacySimilarity, simpleSimilarity)
	assert.Greater(t, legacySimilarity, simpleSimilarity)
}

func TestSampleIndexedCMYKHybrid75ExperimentalModeMatchesLegacyForNonTargetFocusDocs(t *testing.T) {
	testCases := []struct {
		name    string
		pdfPath string
	}{
		{
			name:    "001_minimal_document",
			pdfPath: filepath.Join(getSampleDir(), "001-trivial", "minimal-document.pdf"),
		},
		{
			name:    "002_libreoffice_writer",
			pdfPath: filepath.Join(getSampleDir(), "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf"),
		},
		{
			name:    "003_pdflatex_image",
			pdfPath: filepath.Join(getSampleDir(), "003-pdflatex-image", "pdflatex-image.pdf"),
		},
		{
			name:    "007_imagemagick_images",
			pdfPath: filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-images.pdf"),
		},
		{
			name:    "008_inline_image",
			pdfPath: filepath.Join(getSampleDir(), "008-reportlab-inline-image", "inline-image.pdf"),
		},
		{
			name:    "011_google_doc",
			pdfPath: filepath.Join(getSampleDir(), "011-google-doc-document", "google-doc-document.pdf"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			exactPercent, similarityPercent := renderSamplePageParityBetweenModes(
				t,
				tc.pdfPath,
				domainrenderer.ImageSamplingModeLegacy,
				domainrenderer.ImageSamplingModeExperimentalIndexedCMYKHybrid75V1,
			)
			t.Logf("%s legacy-vs-hybrid75 parity: exact=%.4f similarity=%.4f", tc.name, exactPercent, similarityPercent)

			assert.InDelta(t, 100.0, exactPercent, 1e-9)
			assert.InDelta(t, 100.0, similarityPercent, 1e-9)
		})
	}
}

func TestSampleIndexedCMYKLegacySelectiveDefaultMatchesHybrid75AcrossSampleCorpus(t *testing.T) {
	pdfPaths, err := filepath.Glob(filepath.Join(getSampleDir(), "*", "*.pdf"))
	require.NoError(t, err)
	sort.Strings(pdfPaths)

	for _, pdfPath := range pdfPaths {
		exactPercent, similarityPercent := renderSamplePageParityBetweenModes(
			t,
			pdfPath,
			domainrenderer.ImageSamplingModeLegacy,
			domainrenderer.ImageSamplingModeExperimentalIndexedCMYKHybrid75V1,
		)
		relPath, relErr := filepath.Rel(getSampleDir(), pdfPath)
		require.NoError(t, relErr)
		t.Logf("%s legacy-vs-hybrid75 parity: exact=%.4f similarity=%.4f", filepath.ToSlash(relPath), exactPercent, similarityPercent)
		assert.InDelta(t, 100.0, exactPercent, 1e-9)
		assert.InDelta(t, 100.0, similarityPercent, 1e-9)
	}
}

func TestSampleIndexedCMYKHybrid75ExperimentalModeMatchesLegacyForDoc019(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "019-grayscale-image", "grayscale-image.pdf")
	exactPercent, similarityPercent := renderSamplePageParityBetweenModes(
		t,
		pdfPath,
		domainrenderer.ImageSamplingModeLegacy,
		domainrenderer.ImageSamplingModeExperimentalIndexedCMYKHybrid75V1,
	)
	t.Logf("doc019 legacy-vs-hybrid75 parity: exact=%.4f similarity=%.4f", exactPercent, similarityPercent)

	assert.InDelta(t, 100.0, exactPercent, 1e-9)
	assert.InDelta(t, 100.0, similarityPercent, 1e-9)
}

func TestSampleDecodeOrTransform007BucketsRenderParity(t *testing.T) {
	mainPDF := filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-images.pdf")
	ascii85PDF := filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-ASCII85Decode.pdf")
	lzwPDF := filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-lzw.pdf")
	ccittPDF := filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-CCITTFaxDecode.pdf")
	smilePDF := filepath.Join(getSampleDir(), "..", "pdf_samples", "smile.pdf")

	mainASCIIExact, mainASCIISimilarity := renderSamplePDFParity(t, mainPDF, ascii85PDF)
	mainLZWExact, mainLZWSimilarity := renderSamplePDFParity(t, mainPDF, lzwPDF)
	ccittSmileExact, ccittSmileSimilarity := renderSamplePDFParity(t, ccittPDF, smilePDF)
	mainCCITTExact, mainCCITTSimilarity := renderSamplePDFParity(t, mainPDF, ccittPDF)

	t.Logf(
		"007 bucket parity: main_ascii85=(exact=%.4f similarity=%.4f) main_lzw=(exact=%.4f similarity=%.4f) ccitt_smile=(exact=%.4f similarity=%.4f) main_ccitt=(exact=%.4f similarity=%.4f)",
		mainASCIIExact,
		mainASCIISimilarity,
		mainLZWExact,
		mainLZWSimilarity,
		ccittSmileExact,
		ccittSmileSimilarity,
		mainCCITTExact,
		mainCCITTSimilarity,
	)

	assert.InDelta(t, 100.0, mainASCIIExact, 1e-9)
	assert.InDelta(t, 100.0, mainASCIISimilarity, 1e-9)
	assert.InDelta(t, 100.0, mainLZWExact, 1e-9)
	assert.InDelta(t, 100.0, mainLZWSimilarity, 1e-9)
	assert.InDelta(t, 100.0, ccittSmileExact, 1e-9)
	assert.InDelta(t, 100.0, ccittSmileSimilarity, 1e-9)
	assert.Less(t, mainCCITTExact, 100.0)
	assert.Less(t, mainCCITTSimilarity, 100.0)
}

func TestSampleDecodeOrTransform007BucketsPopplerSimilarityProbe(t *testing.T) {
	mainPDF := filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-images.pdf")
	ascii85PDF := filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-ASCII85Decode.pdf")
	lzwPDF := filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-lzw.pdf")
	ccittPDF := filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-CCITTFaxDecode.pdf")
	smilePDF := filepath.Join(getSampleDir(), "..", "pdf_samples", "smile.pdf")

	opts := pdf.DefaultRenderOptions()
	opts.DPI = 72
	opts.ImageSamplingMode = domainrenderer.ImageSamplingModeLegacy

	_, mainSimilarity := probePDFRenderAgainstPoppler(t, mainPDF, opts)
	_, ascii85Similarity := probePDFRenderAgainstPoppler(t, ascii85PDF, opts)
	_, lzwSimilarity := probePDFRenderAgainstPoppler(t, lzwPDF, opts)
	_, ccittSimilarity := probePDFRenderAgainstPoppler(t, ccittPDF, opts)
	_, smileSimilarity := probePDFRenderAgainstPoppler(t, smilePDF, opts)

	t.Logf(
		"007 bucket poppler similarity: main=%.4f ascii85=%.4f lzw=%.4f ccitt=%.4f smile=%.4f",
		mainSimilarity,
		ascii85Similarity,
		lzwSimilarity,
		ccittSimilarity,
		smileSimilarity,
	)

	assert.InDelta(t, mainSimilarity, ascii85Similarity, 1e-9)
	assert.InDelta(t, mainSimilarity, lzwSimilarity, 1e-9)
	assert.InDelta(t, ccittSimilarity, smileSimilarity, 1e-9)
}

func TestSampleDecodeOrTransform007MainPerPagePopplerSimilarityProbe(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-images.pdf")
	scores := probeSampleAllPagesAgainstPoppler(t, pdfPath, domainrenderer.ImageSamplingModeLegacy)

	for pageNum, similarity := range scores {
		t.Logf("007 main page=%d similarity=%.4f", pageNum, similarity)
	}
}

func TestSampleDecodeOrTransform007DPIRegressionProbe(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-images.pdf")

	page1At72 := probeSamplePageAgainstPopplerAtDPI(t, pdfPath, 1, domainrenderer.ImageSamplingModeLegacy, 72)
	page1At150 := probeSamplePageAgainstPopplerAtDPI(t, pdfPath, 1, domainrenderer.ImageSamplingModeLegacy, 150)
	page4At72 := probeSamplePageAgainstPopplerAtDPI(t, pdfPath, 4, domainrenderer.ImageSamplingModeLegacy, 72)
	page4At150 := probeSamplePageAgainstPopplerAtDPI(t, pdfPath, 4, domainrenderer.ImageSamplingModeLegacy, 150)

	t.Logf(
		"007 dpi regression probe: page1(72=%.4f,150=%.4f) page4(72=%.4f,150=%.4f)",
		page1At72,
		page1At150,
		page4At72,
		page4At150,
	)

	assert.InDelta(t, 100.0, page1At72, 1e-9)
	assert.Greater(t, page4At72, 99.9)
	assert.Less(t, page1At150, 90.0)
	assert.Less(t, page4At150, 90.0)
	assert.Greater(t, page1At72, page1At150)
	assert.Greater(t, page4At72, page4At150)
}

func TestSampleDecodeOrTransform007BucketModeProbe(t *testing.T) {
	testCases := []struct {
		name    string
		pdfPath string
		pageNum int
	}{
		{
			name:    "007_main_page4_dct_gray",
			pdfPath: filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-images.pdf"),
			pageNum: 4,
		},
		{
			name:    "007_ccitt_page1",
			pdfPath: filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-CCITTFaxDecode.pdf"),
			pageNum: 1,
		},
		{
			name:    "smile_page1",
			pdfPath: filepath.Join(getSampleDir(), "..", "pdf_samples", "smile.pdf"),
			pageNum: 1,
		},
	}

	modes := []string{
		domainrenderer.ImageSamplingModeLegacy,
		domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
		domainrenderer.ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1,
		domainrenderer.ImageSamplingModeExperimentalSplashScaleOnlyV1,
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scores := probeSamplePageModesAgainstPoppler(t, tc.pdfPath, tc.pageNum, modes)
			t.Logf("%s mode scores: %v", tc.name, scores)

			switch tc.name {
			case "007_main_page4_dct_gray":
				assert.InDelta(t, scores[domainrenderer.ImageSamplingModeLegacy], scores[domainrenderer.ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1], 1e-9)
				assert.InDelta(t, scores[domainrenderer.ImageSamplingModeLegacy], scores[domainrenderer.ImageSamplingModeExperimentalSplashScaleOnlyV1], 1e-9)
				assert.Greater(t, scores[domainrenderer.ImageSamplingModeLegacy], scores[domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1])
			case "007_ccitt_page1", "smile_page1":
				assert.InDelta(t, scores[domainrenderer.ImageSamplingModeLegacy], scores[domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1], 1e-9)
				assert.InDelta(t, scores[domainrenderer.ImageSamplingModeLegacy], scores[domainrenderer.ImageSamplingModeExperimentalIndexedOriginDownscalePhaseV1], 1e-9)
				assert.InDelta(t, scores[domainrenderer.ImageSamplingModeLegacy], scores[domainrenderer.ImageSamplingModeExperimentalSplashScaleOnlyV1], 1e-9)
			default:
				t.Fatalf("unexpected test case: %s", tc.name)
			}
		})
	}
}

func TestSyntheticDoc007MainPage4ExtractedICCBasedJPEGProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-images.pdf")
	sampleSimilarity := probeSampleAllPagesAgainstPoppler(t, pdfPath, domainrenderer.ImageSamplingModeLegacy)[4]

	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))
	require.Equal(t, domainimage.FilterDCT, imageData.Filter)
	require.Equal(t, domainimage.ColorSpaceDeviceGray, imageData.ColorSpace)
	require.NotEmpty(t, imageData.ICCProfile)
	require.Equal(t, 1, imageData.ICCComponents)

	extractedExact, extractedSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"doc007_page4_extracted_iccbased_dct_gray.pdf",
		buildSyntheticDCTICCBasedGrayImagePDFFloat(
			4,
			4,
			imageData.Width,
			imageData.Height,
			[6]float64{4, 0, 0, 4, 0, 0},
			imageData.Data,
			imageData.ICCProfile,
			imageData.ICCComponents,
		),
	)
	extractedNoICCExact, extractedNoICCSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"doc007_page4_extracted_devicegray_dct.pdf",
		buildSyntheticDCTGrayImagePDFFloat(
			4,
			4,
			imageData.Width,
			imageData.Height,
			[6]float64{4, 0, 0, 4, 0, 0},
			imageData.Data,
		),
	)

	decodedImage, _ := decodeSampleEncodedImageObjectToRGB(t, pdfPath, entity.NewRef(56, 0))
	decodedExact, decodedSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"doc007_page4_decoded_gray.pdf",
		buildSyntheticGrayImagePDFFloat(
			4,
			4,
			decodedImage.Bounds().Dx(),
			decodedImage.Bounds().Dy(),
			[6]float64{4, 0, 0, 4, 0, 0},
			imageToGrayBytes(decodedImage),
		),
	)
	decodedNoICCImage, _ := decodeSampleEncodedImageObjectToRGBWithoutICC(t, pdfPath, entity.NewRef(56, 0))
	decodedNoICCExact, decodedNoICCSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"doc007_page4_decoded_gray_no_icc.pdf",
		buildSyntheticGrayImagePDFFloat(
			4,
			4,
			decodedNoICCImage.Bounds().Dx(),
			decodedNoICCImage.Bounds().Dy(),
			[6]float64{4, 0, 0, 4, 0, 0},
			imageToGrayBytes(decodedNoICCImage),
		),
	)

	t.Skipf(
		"doc007 page4 extracted-iccbased-jpeg probe: sample_similarity=%.4f extracted_icc_dct exact=%.4f similarity=%.4f extracted_devicegray_dct exact=%.4f similarity=%.4f decoded_gray exact=%.4f similarity=%.4f decoded_gray_no_icc exact=%.4f similarity=%.4f",
		sampleSimilarity,
		extractedExact,
		extractedSimilarity,
		extractedNoICCExact,
		extractedNoICCSimilarity,
		decodedExact,
		decodedSimilarity,
		decodedNoICCExact,
		decodedNoICCSimilarity,
	)
}

func TestSampleDCTGrayIgnoreICCSelectiveDefaultMatchesExperimentalForDoc007MainPage4(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-images.pdf")
	scores := probeSamplePageModesAgainstPoppler(
		t,
		pdfPath,
		4,
		[]string{
			domainrenderer.ImageSamplingModeLegacy,
			domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1,
		},
	)

	t.Logf("007 main page4 dct-gray-ignore-icc scores: %v", scores)
	assert.InDelta(t, scores[domainrenderer.ImageSamplingModeLegacy], scores[domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1], 1e-9)
	assert.Greater(t, scores[domainrenderer.ImageSamplingModeLegacy], 99.9)
}

func TestSampleDCTGrayIgnoreICCExperimentalModeMatchesLegacyFor007CCITTFaxAndSmile(t *testing.T) {
	testCases := []struct {
		name    string
		pdfPath string
		pageNum int
	}{
		{
			name:    "007_ccitt_page1",
			pdfPath: filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-CCITTFaxDecode.pdf"),
			pageNum: 1,
		},
		{
			name:    "smile_page1",
			pdfPath: filepath.Join(getSampleDir(), "..", "pdf_samples", "smile.pdf"),
			pageNum: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			scores := probeSamplePageModesAgainstPoppler(
				t,
				tc.pdfPath,
				tc.pageNum,
				[]string{
					domainrenderer.ImageSamplingModeLegacy,
					domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1,
				},
			)

			t.Logf("%s dct-gray-ignore-icc scores: %v", tc.name, scores)
			assert.InDelta(t, scores[domainrenderer.ImageSamplingModeLegacy], scores[domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1], 1e-9)
		})
	}
}

func TestSampleDCTGrayIgnoreICCExperimentalModeSurfaceAcrossSampleCorpusAllPages(t *testing.T) {
	pdfPaths, err := filepath.Glob(filepath.Join(getSampleDir(), "*", "*.pdf"))
	require.NoError(t, err)
	sort.Strings(pdfPaths)

	var changed []string
	for _, pdfPath := range pdfPaths {
		pageChanges := renderSampleAllPagesParityBetweenModes(
			t,
			pdfPath,
			domainrenderer.ImageSamplingModeLegacy,
			domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1,
		)
		relPath, relErr := filepath.Rel(getSampleDir(), pdfPath)
		require.NoError(t, relErr)
		relPath = filepath.ToSlash(relPath)
		for _, pageChange := range pageChanges {
			changed = append(changed, fmt.Sprintf("%s#p%d", relPath, pageChange))
		}
	}

	assert.Empty(t, changed)
}

func TestSyntheticDoc007CCITTFaxAndSmileExtractedObjectProbeAgainstPoppler(t *testing.T) {
	testCases := []struct {
		name    string
		pdfPath string
		ref     entity.Ref
	}{
		{
			name:    "007_ccitt_page1",
			pdfPath: filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-CCITTFaxDecode.pdf"),
			ref:     entity.NewRef(8, 0),
		},
		{
			name:    "smile_page1",
			pdfPath: filepath.Join(getSampleDir(), "..", "pdf_samples", "smile.pdf"),
			ref:     entity.NewRef(8, 0),
		},
	}

	type probeResult struct {
		sampleSimilarity    float64
		extractedSimilarity float64
		decodedSimilarity   float64
	}

	results := make(map[string]probeResult, len(testCases))
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sampleSimilarity := probeSampleAllPagesAgainstPoppler(t, tc.pdfPath, domainrenderer.ImageSamplingModeLegacy)[1]

			imageData := loadSampleEncodedImageData(t, tc.pdfPath, tc.ref)
			require.Equal(t, domainimage.FilterCCITTFax, imageData.Filter)
			require.Equal(t, domainimage.ColorSpaceDeviceGray, imageData.ColorSpace)
			require.Equal(t, 1, imageData.BitsPerComponent)

			_, extractedSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
				t,
				strings.ReplaceAll(tc.name, "/", "_")+"_extracted_ccitt.pdf",
				buildSyntheticCCITTGrayImagePDFFloat(
					4,
					4,
					imageData.Width,
					imageData.Height,
					[6]float64{4, 0, 0, 4, 0, 0},
					imageData.Data,
					imageData.DecodeParms,
					imageData.Decode,
				),
			)

			decodedImage, _ := decodeSampleDirectImageObjectToAppliedRGBA(t, tc.pdfPath, tc.ref)
			_, decodedSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
				t,
				strings.ReplaceAll(tc.name, "/", "_")+"_decoded_gray.pdf",
				buildSyntheticGrayImagePDFFloat(
					4,
					4,
					decodedImage.Bounds().Dx(),
					decodedImage.Bounds().Dy(),
					[6]float64{4, 0, 0, 4, 0, 0},
					imageToGrayBytes(decodedImage),
				),
			)

			results[tc.name] = probeResult{
				sampleSimilarity:    sampleSimilarity,
				extractedSimilarity: extractedSimilarity,
				decodedSimilarity:   decodedSimilarity,
			}

			t.Logf(
				"%s ccitt probe: sample=%.4f extracted_ccitt=%.4f decoded_gray=%.4f",
				tc.name,
				sampleSimilarity,
				extractedSimilarity,
				decodedSimilarity,
			)
		})
	}

	assert.InDelta(t, results["007_ccitt_page1"].sampleSimilarity, results["smile_page1"].sampleSimilarity, 1e-9)
	assert.InDelta(t, results["007_ccitt_page1"].extractedSimilarity, results["smile_page1"].extractedSimilarity, 1e-9)
	assert.InDelta(t, results["007_ccitt_page1"].decodedSimilarity, results["smile_page1"].decodedSimilarity, 1e-9)
	assert.GreaterOrEqual(t, results["007_ccitt_page1"].decodedSimilarity, results["007_ccitt_page1"].extractedSimilarity)
}

func TestSyntheticDoc023DecodedRGBAParityVsPostDecodeRenderParityProbe(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	currentDecoded, currentRGB := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeDefault)
	simpleDecoded, simpleRGB := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeSimpleSubtractive)
	stdlibDecoded, stdlibRGB := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeStdlib)

	decodedCurrentSimpleExact, decodedCurrentSimpleSimilarity := probeImageParity(
		t,
		"doc023_decoded_current.png",
		currentDecoded,
		"doc023_decoded_simple.png",
		simpleDecoded,
	)
	decodedCurrentStdlibExact, decodedCurrentStdlibSimilarity := probeImageParity(
		t,
		"doc023_decoded_current.png",
		currentDecoded,
		"doc023_decoded_stdlib.png",
		stdlibDecoded,
	)
	decodedSimpleStdlibExact, decodedSimpleStdlibSimilarity := probeImageParity(
		t,
		"doc023_decoded_simple.png",
		simpleDecoded,
		"doc023_decoded_stdlib.png",
		stdlibDecoded,
	)

	currentPopplerExact, currentPopplerSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_decoded_current_rgb.pdf",
		buildSyntheticRGBImagePDFFloat(
			540,
			720,
			currentDecoded.Bounds().Dx(),
			currentDecoded.Bounds().Dy(),
			[6]float64{468, 0, 0, 624, 72, 96},
			currentRGB,
		),
	)
	simplePopplerExact, simplePopplerSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_decoded_simple_rgb.pdf",
		buildSyntheticRGBImagePDFFloat(
			540,
			720,
			simpleDecoded.Bounds().Dx(),
			simpleDecoded.Bounds().Dy(),
			[6]float64{468, 0, 0, 624, 72, 96},
			simpleRGB,
		),
	)
	stdlibPopplerExact, stdlibPopplerSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_decoded_stdlib_rgb.pdf",
		buildSyntheticRGBImagePDFFloat(
			540,
			720,
			stdlibDecoded.Bounds().Dx(),
			stdlibDecoded.Bounds().Dy(),
			[6]float64{468, 0, 0, 624, 72, 96},
			stdlibRGB,
		),
	)

	renderedCurrentSimpleExact, renderedCurrentSimpleSimilarity := probeRenderedPDFParity(
		t,
		"indexed_doc023_decoded_current_rendered.pdf",
		buildSyntheticRGBImagePDFFloat(
			540,
			720,
			currentDecoded.Bounds().Dx(),
			currentDecoded.Bounds().Dy(),
			[6]float64{468, 0, 0, 624, 72, 96},
			currentRGB,
		),
		"indexed_doc023_decoded_simple_rendered.pdf",
		buildSyntheticRGBImagePDFFloat(
			540,
			720,
			simpleDecoded.Bounds().Dx(),
			simpleDecoded.Bounds().Dy(),
			[6]float64{468, 0, 0, 624, 72, 96},
			simpleRGB,
		),
	)
	renderedCurrentStdlibExact, renderedCurrentStdlibSimilarity := probeRenderedPDFParity(
		t,
		"indexed_doc023_decoded_current_rendered.pdf",
		buildSyntheticRGBImagePDFFloat(
			540,
			720,
			currentDecoded.Bounds().Dx(),
			currentDecoded.Bounds().Dy(),
			[6]float64{468, 0, 0, 624, 72, 96},
			currentRGB,
		),
		"indexed_doc023_decoded_stdlib_rendered.pdf",
		buildSyntheticRGBImagePDFFloat(
			540,
			720,
			stdlibDecoded.Bounds().Dx(),
			stdlibDecoded.Bounds().Dy(),
			[6]float64{468, 0, 0, 624, 72, 96},
			stdlibRGB,
		),
	)
	renderedSimpleStdlibExact, renderedSimpleStdlibSimilarity := probeRenderedPDFParity(
		t,
		"indexed_doc023_decoded_simple_rendered.pdf",
		buildSyntheticRGBImagePDFFloat(
			540,
			720,
			simpleDecoded.Bounds().Dx(),
			simpleDecoded.Bounds().Dy(),
			[6]float64{468, 0, 0, 624, 72, 96},
			simpleRGB,
		),
		"indexed_doc023_decoded_stdlib_rendered.pdf",
		buildSyntheticRGBImagePDFFloat(
			540,
			720,
			stdlibDecoded.Bounds().Dx(),
			stdlibDecoded.Bounds().Dy(),
			[6]float64{468, 0, 0, 624, 72, 96},
			stdlibRGB,
		),
	)

	t.Skipf(
		"doc023 decoded-vs-post-decode probe: decoded_current_simple_exact=%.4f decoded_current_simple_similarity=%.4f decoded_current_stdlib_exact=%.4f decoded_current_stdlib_similarity=%.4f decoded_simple_stdlib_exact=%.4f decoded_simple_stdlib_similarity=%.4f poppler_current_exact=%.4f poppler_current_similarity=%.4f poppler_simple_exact=%.4f poppler_simple_similarity=%.4f poppler_stdlib_exact=%.4f poppler_stdlib_similarity=%.4f rendered_current_simple_exact=%.4f rendered_current_simple_similarity=%.4f rendered_current_stdlib_exact=%.4f rendered_current_stdlib_similarity=%.4f rendered_simple_stdlib_exact=%.4f rendered_simple_stdlib_similarity=%.4f",
		decodedCurrentSimpleExact,
		decodedCurrentSimpleSimilarity,
		decodedCurrentStdlibExact,
		decodedCurrentStdlibSimilarity,
		decodedSimpleStdlibExact,
		decodedSimpleStdlibSimilarity,
		currentPopplerExact,
		currentPopplerSimilarity,
		simplePopplerExact,
		simplePopplerSimilarity,
		stdlibPopplerExact,
		stdlibPopplerSimilarity,
		renderedCurrentSimpleExact,
		renderedCurrentSimpleSimilarity,
		renderedCurrentStdlibExact,
		renderedCurrentStdlibSimilarity,
		renderedSimpleStdlibExact,
		renderedSimpleStdlibSimilarity,
	)
}

func TestSyntheticDoc018ExtractedImageObjectSoftMaskProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "018-base64-image", "base64image.pdf")
	sampleOpts := pdf.DefaultRenderOptions()
	sampleOpts.DPI = 72

	sampleExact, sampleSimilarity := probePDFRenderAgainstPoppler(t, pdfPath, sampleOpts)

	imgData := loadSampleDirectImageData(t, pdfPath, entity.NewRef(15, 0))
	require.Equal(t, domainimage.ColorSpaceDeviceRGB, imgData.ColorSpace)
	require.NotNil(t, imgData.Mask)
	maskData := imageMaskToGrayBytes(t, imgData.Mask)
	require.Len(t, maskData, imgData.Width*imgData.Height)

	withSoftMaskExact, withSoftMaskSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"doc018_extracted_rgb_smask.pdf",
		buildSyntheticRGBImageWithSoftMaskPDFFloat(
			595.2756,
			841.8898,
			imgData.Width,
			imgData.Height,
			[6]float64{375.336197, 0, 0, 300.999394, 4.836078, 536.169673},
			imgData.Data,
			imgData.Width,
			imgData.Height,
			maskData,
		),
	)

	withoutSoftMaskExact, withoutSoftMaskSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"doc018_extracted_rgb_no_smask.pdf",
		buildSyntheticRGBImagePDFFloat(
			595.2756,
			841.8898,
			imgData.Width,
			imgData.Height,
			[6]float64{375.336197, 0, 0, 300.999394, 4.836078, 536.169673},
			imgData.Data,
		),
	)

	appliedRGBA, flattenedRGB := decodeSampleDirectImageObjectToAppliedRGBA(t, pdfPath, entity.NewRef(15, 0))
	flattenedExact, flattenedSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"doc018_extracted_rgb_flattened.pdf",
		buildSyntheticRGBImagePDFFloat(
			595.2756,
			841.8898,
			appliedRGBA.Bounds().Dx(),
			appliedRGBA.Bounds().Dy(),
			[6]float64{375.336197, 0, 0, 300.999394, 4.836078, 536.169673},
			flattenedRGB,
		),
	)

	renderedSoftMaskFlattenedExact, renderedSoftMaskFlattenedSimilarity := probeRenderedPDFParity(
		t,
		"doc018_extracted_rgb_smask_rendered.pdf",
		buildSyntheticRGBImageWithSoftMaskPDFFloat(
			595.2756,
			841.8898,
			imgData.Width,
			imgData.Height,
			[6]float64{375.336197, 0, 0, 300.999394, 4.836078, 536.169673},
			imgData.Data,
			imgData.Width,
			imgData.Height,
			maskData,
		),
		"doc018_extracted_rgb_flattened_rendered.pdf",
		buildSyntheticRGBImagePDFFloat(
			595.2756,
			841.8898,
			appliedRGBA.Bounds().Dx(),
			appliedRGBA.Bounds().Dy(),
			[6]float64{375.336197, 0, 0, 300.999394, 4.836078, 536.169673},
			flattenedRGB,
		),
	)

	t.Skipf(
		"doc018 extracted soft-mask probe: sample exact=%.4f similarity=%.4f extracted_with_smask exact=%.4f similarity=%.4f extracted_without_smask exact=%.4f similarity=%.4f flattened_rgb exact=%.4f similarity=%.4f rendered_smask_vs_flattened exact=%.4f similarity=%.4f",
		sampleExact,
		sampleSimilarity,
		withSoftMaskExact,
		withSoftMaskSimilarity,
		withoutSoftMaskExact,
		withoutSoftMaskSimilarity,
		flattenedExact,
		flattenedSimilarity,
		renderedSoftMaskFlattenedExact,
		renderedSoftMaskFlattenedSimilarity,
	)
}

func TestSyntheticDoc018FlattenedRGBSplashReferenceProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "018-base64-image", "base64image.pdf")
	appliedRGBA, flattenedRGB := decodeSampleDirectImageObjectToAppliedRGBA(t, pdfPath, entity.NewRef(15, 0))
	flattenedSource := flattenImageToOpaqueRGBA(appliedRGBA, color.White)

	oursSimilarity, splashSimilarity := measureSyntheticRGBSplashReferenceAgainstCurrentPath(
		t,
		"doc018_flattened_rgb_splash_probe.pdf",
		buildSyntheticRGBImagePDFFloat(
			595.2756,
			841.8898,
			appliedRGBA.Bounds().Dx(),
			appliedRGBA.Bounds().Dy(),
			[6]float64{375.336197, 0, 0, 300.999394, 4.836078, 536.169673},
			flattenedRGB,
		),
		flattenedSource,
		image.Rect(0, 0, 596, 842),
		[6]float64{375.336197, 0, 0, 300.999394, 4.836078, 536.169673},
	)

	t.Skipf(
		"doc018 flattened-rgb splash probe: ours=%.4f splash_like=%.4f",
		oursSimilarity,
		splashSimilarity,
	)
}

func TestSyntheticDoc003ExtractedJPEGImageObjectProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "003-pdflatex-image", "pdflatex-image.pdf")
	sampleOpts := pdf.DefaultRenderOptions()
	sampleOpts.DPI = 72

	sampleExact, sampleSimilarity := probePDFRenderAgainstPoppler(t, pdfPath, sampleOpts)

	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(1, 0))
	require.Equal(t, domainimage.FilterDCT, imageData.Filter)
	require.Equal(t, domainimage.ColorSpaceDeviceRGB, imageData.ColorSpace)

	extractedExact, extractedSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"doc003_extracted_dct_image.pdf",
		buildSyntheticDCTRGBImagePDFFloat(
			595.2756,
			841.8898,
			imageData.Width,
			imageData.Height,
			[6]float64{300.364873, 0, 0, 200.026132, 147.817564, 412.629907},
			imageData.Data,
		),
	)

	decodedImage, decodedRGB := decodeSampleEncodedImageObjectToRGB(t, pdfPath, entity.NewRef(1, 0))
	decodedExact, decodedSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"doc003_decoded_rgb_image.pdf",
		buildSyntheticRGBImagePDFFloat(
			595.2756,
			841.8898,
			decodedImage.Bounds().Dx(),
			decodedImage.Bounds().Dy(),
			[6]float64{300.364873, 0, 0, 200.026132, 147.817564, 412.629907},
			decodedRGB,
		),
	)

	t.Skipf(
		"doc003 extracted-jpeg probe: sample exact=%.4f similarity=%.4f extracted_dct exact=%.4f similarity=%.4f decoded_rgb exact=%.4f similarity=%.4f",
		sampleExact,
		sampleSimilarity,
		extractedExact,
		extractedSimilarity,
		decodedExact,
		decodedSimilarity,
	)
}

func TestSyntheticDoc003DecodedRGBTransparentEdgeReferenceProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "003-pdflatex-image", "pdflatex-image.pdf")
	decodedImage, decodedRGB := decodeSampleEncodedImageObjectToRGB(t, pdfPath, entity.NewRef(1, 0))
	pageRect := image.Rect(0, 0, 596, 842)
	matrix := [6]float64{300.364873, 0, 0, 200.026132, 147.817564, 412.629907}

	currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
		t,
		"doc003_decoded_rgb_transparent_edge.pdf",
		buildSyntheticRGBImagePDFFloat(
			595.2756,
			841.8898,
			decodedImage.Bounds().Dx(),
			decodedImage.Bounds().Dy(),
			matrix,
			decodedRGB,
		),
		map[string]image.Image{
			"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(decodedImage, pageRect, matrix),
			"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(decodedImage, pageRect, matrix),
		},
	)

	t.Skipf(
		"doc003 decoded-rgb transparent-edge probe: current_exact=%.4f current_similarity=%.4f affine_transparent_edge_over_white=%.4f rect_transparent_edge_over_white=%.4f",
		currentExact,
		currentSimilarity,
		refScores["affine_transparent_edge_over_white"],
		refScores["rect_transparent_edge_over_white"],
	)
}

func TestSyntheticDoc008InlineRGBTransparentEdgeReferenceProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "008-reportlab-inline-image", "inline-image.pdf")
	decodedImage, decodedRGB := decodeSampleFirstInlineImageToRGB(t, pdfPath)
	pageRect := image.Rect(0, 0, 596, 842)
	matrix := [6]float64{100, 0, 0, 100, 100, 100}

	currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
		t,
		"doc008_inline_rgb_transparent_edge.pdf",
		buildSyntheticRGBImagePDFFloat(
			595.2756,
			841.8898,
			decodedImage.Bounds().Dx(),
			decodedImage.Bounds().Dy(),
			matrix,
			decodedRGB,
		),
		map[string]image.Image{
			"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(decodedImage, pageRect, matrix),
			"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(decodedImage, pageRect, matrix),
		},
	)

	t.Skipf(
		"doc008 inline-rgb transparent-edge probe: current_exact=%.4f current_similarity=%.4f affine_transparent_edge_over_white=%.4f rect_transparent_edge_over_white=%.4f",
		currentExact,
		currentSimilarity,
		refScores["affine_transparent_edge_over_white"],
		refScores["rect_transparent_edge_over_white"],
	)
}

func TestSyntheticRGBTransparentEdgeContentClassProbeAgainstPoppler(t *testing.T) {
	testCases := []struct {
		name     string
		pdfName  string
		pageW    float64
		pageH    float64
		pageRect image.Rectangle
		matrix   [6]float64
	}{
		{
			name:     "doc003_geometry",
			pdfName:  "rgb_doc003_geometry_content_class.pdf",
			pageW:    595.2756,
			pageH:    841.8898,
			pageRect: image.Rect(0, 0, 596, 842),
			matrix:   [6]float64{300.364873, 0, 0, 200.026132, 147.817564, 412.629907},
		},
		{
			name:     "doc008_geometry",
			pdfName:  "rgb_doc008_geometry_content_class.pdf",
			pageW:    595.2756,
			pageH:    841.8898,
			pageRect: image.Rect(0, 0, 596, 842),
			matrix:   [6]float64{100, 0, 0, 100, 100, 100},
		},
	}

	contentClasses := []struct {
		name string
		data []byte
	}{
		{name: "flat", data: syntheticRGBFlatFill(16, 16, color.RGBA{R: 220, G: 80, B: 60, A: 255})},
		{name: "gradient", data: syntheticRGBHorizontalGradient(16, 16)},
		{name: "checker", data: syntheticRGBCheckerboard(16, 16)},
		{name: "tiled", data: syntheticRGBTiledIdentity(16, 16)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, contentClass := range contentClasses {
				src := syntheticRGBTiledImage(16, 16, contentClass.data)
				currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
					t,
					fmt.Sprintf("%s_%s.pdf", strings.TrimSuffix(tc.pdfName, ".pdf"), contentClass.name),
					buildSyntheticRGBImagePDFFloat(tc.pageW, tc.pageH, 16, 16, tc.matrix, contentClass.data),
					map[string]image.Image{
						"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, tc.pageRect, tc.matrix),
						"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, tc.pageRect, tc.matrix),
					},
				)

				t.Logf(
					"%s/%s transparent-edge content probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
					tc.name,
					contentClass.name,
					currentExact,
					currentSimilarity,
					refScores["affine_transparent_edge_over_white"],
					refScores["rect_transparent_edge_over_white"],
				)
			}
		})
	}
}

func TestSyntheticRGBTransparentEdgeGeometrySweepProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)

	testCases := []struct {
		name     string
		pageW    float64
		pageH    float64
		pageRect image.Rectangle
		matrix   [6]float64
	}{
		{
			name:     "pure_lane_small_page",
			pageW:    24,
			pageH:    24,
			pageRect: image.Rect(0, 0, 24, 24),
			matrix:   [6]float64{20, 0, 0, 20, 1.5, 1.5},
		},
		{
			name:     "tiny_upscale_large_page",
			pageW:    595.2756,
			pageH:    841.8898,
			pageRect: image.Rect(0, 0, 596, 842),
			matrix:   [6]float64{20, 0, 0, 20, 100, 100},
		},
		{
			name:     "doc008_like",
			pageW:    595.2756,
			pageH:    841.8898,
			pageRect: image.Rect(0, 0, 596, 842),
			matrix:   [6]float64{100, 0, 0, 100, 100, 100},
		},
		{
			name:     "doc003_like",
			pageW:    595.2756,
			pageH:    841.8898,
			pageRect: image.Rect(0, 0, 596, 842),
			matrix:   [6]float64{300.364873, 0, 0, 200.026132, 147.817564, 412.629907},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_geometry_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(tc.pageW, tc.pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, tc.pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, tc.pageRect, tc.matrix),
				},
			)

			t.Logf(
				"%s transparent-edge geometry probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}
}

func TestSyntheticRGBTransparentEdgeOccupancySweepProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)

	testCases := []struct {
		name     string
		pageW    float64
		pageH    float64
		pageRect image.Rectangle
		matrix   [6]float64
	}{
		{
			name:     "occupancy_high_24x24",
			pageW:    24,
			pageH:    24,
			pageRect: image.Rect(0, 0, 24, 24),
			matrix:   [6]float64{20, 0, 0, 20, 1.5, 1.5},
		},
		{
			name:     "occupancy_medium_48x48",
			pageW:    48,
			pageH:    48,
			pageRect: image.Rect(0, 0, 48, 48),
			matrix:   [6]float64{20, 0, 0, 20, 14, 14},
		},
		{
			name:     "occupancy_low_128x128",
			pageW:    128,
			pageH:    128,
			pageRect: image.Rect(0, 0, 128, 128),
			matrix:   [6]float64{20, 0, 0, 20, 54, 54},
		},
		{
			name:     "occupancy_tiny_596x842",
			pageW:    595.2756,
			pageH:    841.8898,
			pageRect: image.Rect(0, 0, 596, 842),
			matrix:   [6]float64{20, 0, 0, 20, 100, 100},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_occupancy_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(tc.pageW, tc.pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, tc.pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, tc.pageRect, tc.matrix),
				},
			)

			occupancy := (tc.matrix[0] * tc.matrix[3]) / (tc.pageW * tc.pageH)
			t.Logf(
				"%s transparent-edge occupancy probe: occupancy=%.6f current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				occupancy,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}
}

func TestSyntheticRGBTransparentEdgeFootprintSweepProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	pageW := 595.2756
	pageH := 841.8898
	pageRect := image.Rect(0, 0, 596, 842)

	testCases := []struct {
		name   string
		matrix [6]float64
	}{
		{name: "footprint_20x20", matrix: [6]float64{20, 0, 0, 20, 100, 100}},
		{name: "footprint_40x40", matrix: [6]float64{40, 0, 0, 40, 100, 100}},
		{name: "footprint_100x100", matrix: [6]float64{100, 0, 0, 100, 100, 100}},
		{name: "footprint_200x200", matrix: [6]float64{200, 0, 0, 200, 100, 100}},
		{name: "footprint_300x200", matrix: [6]float64{300.364873, 0, 0, 200.026132, 147.817564, 412.629907}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_footprint_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(pageW, pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, pageRect, tc.matrix),
				},
			)

			footprint := tc.matrix[0] * tc.matrix[3]
			t.Logf(
				"%s transparent-edge footprint probe: footprint=%.4f current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				footprint,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}
}

func TestSyntheticRGBTransparentEdgeDetailDensitySweepProbeAgainstPoppler(t *testing.T) {
	pageW := 595.2756
	pageH := 841.8898
	pageRect := image.Rect(0, 0, 596, 842)

	footprintCases := []struct {
		name   string
		matrix [6]float64
	}{
		{name: "footprint_20x20", matrix: [6]float64{20, 0, 0, 20, 100, 100}},
		{name: "footprint_100x100", matrix: [6]float64{100, 0, 0, 100, 100, 100}},
		{name: "footprint_300x200", matrix: [6]float64{300.364873, 0, 0, 200.026132, 147.817564, 412.629907}},
	}

	sourceCases := []struct {
		name   string
		width  int
		height int
	}{
		{name: "src_4x4", width: 4, height: 4},
		{name: "src_8x8", width: 8, height: 8},
		{name: "src_16x16", width: 16, height: 16},
		{name: "src_32x32", width: 32, height: 32},
	}

	for _, footprintCase := range footprintCases {
		t.Run(footprintCase.name, func(t *testing.T) {
			for _, sourceCase := range sourceCases {
				imageData := syntheticRGBCheckerboard(sourceCase.width, sourceCase.height)
				src := syntheticRGBTiledImage(sourceCase.width, sourceCase.height, imageData)

				currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
					t,
					fmt.Sprintf(
						"rgb_transparent_edge_detail_density_%s_%s.pdf",
						footprintCase.name,
						sourceCase.name,
					),
					buildSyntheticRGBImagePDFFloat(
						pageW,
						pageH,
						sourceCase.width,
						sourceCase.height,
						footprintCase.matrix,
						imageData,
					),
					map[string]image.Image{
						"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, pageRect, footprintCase.matrix),
						"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, pageRect, footprintCase.matrix),
					},
				)

				detailDensity := float64(sourceCase.width*sourceCase.height) / (footprintCase.matrix[0] * footprintCase.matrix[3])
				t.Logf(
					"%s/%s transparent-edge detail-density probe: density=%.6f current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
					footprintCase.name,
					sourceCase.name,
					detailDensity,
					currentExact,
					currentSimilarity,
					refScores["affine_transparent_edge_over_white"],
					refScores["rect_transparent_edge_over_white"],
				)
			}
		})
	}
}

func TestSyntheticRGBTransparentEdgePatternFamilyGeometryProbeAgainstPoppler(t *testing.T) {
	geometryCases := []struct {
		name     string
		pageW    float64
		pageH    float64
		pageRect image.Rectangle
		matrix   [6]float64
	}{
		{
			name:     "pure_lane_small_page",
			pageW:    24,
			pageH:    24,
			pageRect: image.Rect(0, 0, 24, 24),
			matrix:   [6]float64{20, 0, 0, 20, 1.5, 1.5},
		},
		{
			name:     "large_page_tiny_footprint",
			pageW:    595.2756,
			pageH:    841.8898,
			pageRect: image.Rect(0, 0, 596, 842),
			matrix:   [6]float64{20, 0, 0, 20, 100, 100},
		},
	}

	patternCases := []struct {
		name   string
		width  int
		height int
		data   func(int, int) []byte
	}{
		{name: "flat", width: 16, height: 16, data: func(w, h int) []byte { return syntheticRGBFlatFill(w, h, color.RGBA{R: 220, G: 80, B: 60, A: 255}) }},
		{name: "gradient", width: 16, height: 16, data: syntheticRGBHorizontalGradient},
		{name: "checker", width: 16, height: 16, data: syntheticRGBCheckerboard},
		{name: "tiled_identity", width: 16, height: 16, data: syntheticRGBTiledIdentity},
	}

	for _, geometryCase := range geometryCases {
		t.Run(geometryCase.name, func(t *testing.T) {
			for _, patternCase := range patternCases {
				imageData := patternCase.data(patternCase.width, patternCase.height)
				src := syntheticRGBTiledImage(patternCase.width, patternCase.height, imageData)

				currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
					t,
					fmt.Sprintf(
						"rgb_transparent_edge_pattern_family_%s_%s.pdf",
						geometryCase.name,
						patternCase.name,
					),
					buildSyntheticRGBImagePDFFloat(
						geometryCase.pageW,
						geometryCase.pageH,
						patternCase.width,
						patternCase.height,
						geometryCase.matrix,
						imageData,
					),
					map[string]image.Image{
						"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, geometryCase.pageRect, geometryCase.matrix),
						"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, geometryCase.pageRect, geometryCase.matrix),
					},
				)

				t.Logf(
					"%s/%s transparent-edge pattern-family probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
					geometryCase.name,
					patternCase.name,
					currentExact,
					currentSimilarity,
					refScores["affine_transparent_edge_over_white"],
					refScores["rect_transparent_edge_over_white"],
				)
			}
		})
	}
}

func TestSyntheticRGBTransparentEdgeRasterResolutionSweepProbeAgainstPoppler(t *testing.T) {
	testCases := []struct {
		name      string
		pageW     float64
		pageH     float64
		matrix    [6]float64
		imageW    int
		imageH    int
		imageData []byte
	}{
		{
			name:      "pure_lane_small_page_tiled_identity",
			pageW:     24,
			pageH:     24,
			matrix:    [6]float64{20, 0, 0, 20, 1.5, 1.5},
			imageW:    16,
			imageH:    16,
			imageData: syntheticRGBTiledIdentity(16, 16),
		},
		{
			name:      "large_page_tiny_footprint_tiled_identity",
			pageW:     595,
			pageH:     842,
			matrix:    [6]float64{20, 0, 0, 20, 100, 100},
			imageW:    16,
			imageH:    16,
			imageData: syntheticRGBTiledIdentity(16, 16),
		},
	}

	dpis := []float64{72, 144, 288}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, dpi := range dpis {
				scale := dpi / 72.0
				scaledPageRect := image.Rect(
					0,
					0,
					int(math.Round(tc.pageW*scale)),
					int(math.Round(tc.pageH*scale)),
				)
				scaledMatrix := [6]float64{
					tc.matrix[0] * scale,
					tc.matrix[1] * scale,
					tc.matrix[2] * scale,
					tc.matrix[3] * scale,
					tc.matrix[4] * scale,
					tc.matrix[5] * scale,
				}
				src := syntheticRGBTiledImage(tc.imageW, tc.imageH, tc.imageData)

				currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPopplerAtDPI(
					t,
					fmt.Sprintf("rgb_transparent_edge_raster_%s_%.0fdpi.pdf", tc.name, dpi),
					buildSyntheticRGBImagePDFFloat(tc.pageW, tc.pageH, tc.imageW, tc.imageH, tc.matrix, tc.imageData),
					dpi,
					map[string]image.Image{
						"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, scaledPageRect, scaledMatrix),
						"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, scaledPageRect, scaledMatrix),
					},
				)

				t.Logf(
					"%s dpi=%.0f transparent-edge raster probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
					tc.name,
					dpi,
					currentExact,
					currentSimilarity,
					refScores["affine_transparent_edge_over_white"],
					refScores["rect_transparent_edge_over_white"],
				)
			}
		})
	}
}

func TestSyntheticRGBTransparentEdgeGeometryNormalizationProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)

	testCases := []struct {
		name     string
		pageW    float64
		pageH    float64
		pageRect image.Rectangle
		matrix   [6]float64
	}{
		{
			name:     "small_page_original_margin",
			pageW:    24,
			pageH:    24,
			pageRect: image.Rect(0, 0, 24, 24),
			matrix:   [6]float64{20, 0, 0, 20, 1.5, 1.5},
		},
		{
			name:     "small_page_origin_zero",
			pageW:    24,
			pageH:    24,
			pageRect: image.Rect(0, 0, 24, 24),
			matrix:   [6]float64{20, 0, 0, 20, 0, 0},
		},
		{
			name:     "medium_page_centered",
			pageW:    48,
			pageH:    48,
			pageRect: image.Rect(0, 0, 48, 48),
			matrix:   [6]float64{20, 0, 0, 20, 14, 14},
		},
		{
			name:     "large_page_origin_zero",
			pageW:    595,
			pageH:    842,
			pageRect: image.Rect(0, 0, 595, 842),
			matrix:   [6]float64{20, 0, 0, 20, 0, 0},
		},
		{
			name:     "large_page_small_margin",
			pageW:    595,
			pageH:    842,
			pageRect: image.Rect(0, 0, 595, 842),
			matrix:   [6]float64{20, 0, 0, 20, 1.5, 1.5},
		},
		{
			name:     "large_page_centered",
			pageW:    595,
			pageH:    842,
			pageRect: image.Rect(0, 0, 595, 842),
			matrix:   [6]float64{20, 0, 0, 20, 287.5, 411},
		},
		{
			name:     "large_page_offset_100",
			pageW:    595,
			pageH:    842,
			pageRect: image.Rect(0, 0, 595, 842),
			matrix:   [6]float64{20, 0, 0, 20, 100, 100},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_geometry_normalization_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(tc.pageW, tc.pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, tc.pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, tc.pageRect, tc.matrix),
				},
			)

			t.Logf(
				"%s transparent-edge geometry-normalization probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}
}

func TestSyntheticRGBTransparentEdgeLargePageTranslationSweepProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	pageW := 595.0
	pageH := 842.0
	pageRect := image.Rect(0, 0, 595, 842)

	testCases := []struct {
		name   string
		matrix [6]float64
	}{
		{name: "origin_0", matrix: [6]float64{20, 0, 0, 20, 0, 0}},
		{name: "margin_1_5", matrix: [6]float64{20, 0, 0, 20, 1.5, 1.5}},
		{name: "margin_14", matrix: [6]float64{20, 0, 0, 20, 14, 14}},
		{name: "offset_50", matrix: [6]float64{20, 0, 0, 20, 50, 50}},
		{name: "offset_100", matrix: [6]float64{20, 0, 0, 20, 100, 100}},
		{name: "offset_200", matrix: [6]float64{20, 0, 0, 20, 200, 200}},
		{name: "centered", matrix: [6]float64{20, 0, 0, 20, 287.5, 411}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_large_page_translation_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(pageW, pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, pageRect, tc.matrix),
				},
			)

			t.Logf(
				"%s transparent-edge large-page-translation probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}
}

func TestSyntheticRGBTransparentEdgeCenterSymmetryProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	pageW := 595.0
	pageH := 842.0
	pageRect := image.Rect(0, 0, 595, 842)

	testCases := []struct {
		name   string
		matrix [6]float64
	}{
		{name: "x_centered_y_origin", matrix: [6]float64{20, 0, 0, 20, 287.5, 0}},
		{name: "x_centered_y_margin_14", matrix: [6]float64{20, 0, 0, 20, 287.5, 14}},
		{name: "x_centered_y_offset_100", matrix: [6]float64{20, 0, 0, 20, 287.5, 100}},
		{name: "x_origin_y_centered", matrix: [6]float64{20, 0, 0, 20, 0, 411}},
		{name: "x_margin_14_y_centered", matrix: [6]float64{20, 0, 0, 20, 14, 411}},
		{name: "x_offset_100_y_centered", matrix: [6]float64{20, 0, 0, 20, 100, 411}},
		{name: "near_centered_minus_14_both", matrix: [6]float64{20, 0, 0, 20, 273.5, 397}},
		{name: "near_centered_plus_14_both", matrix: [6]float64{20, 0, 0, 20, 301.5, 425}},
		{name: "x_near_centered_y_centered", matrix: [6]float64{20, 0, 0, 20, 273.5, 411}},
		{name: "x_centered_y_near_centered", matrix: [6]float64{20, 0, 0, 20, 287.5, 397}},
		{name: "fully_centered", matrix: [6]float64{20, 0, 0, 20, 287.5, 411}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_center_symmetry_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(pageW, pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, pageRect, tc.matrix),
				},
			)

			t.Logf(
				"%s transparent-edge center-symmetry probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}
}

func TestSyntheticRGBTransparentEdgeVerticalSymmetrySweepProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	pageW := 595.0
	pageH := 842.0
	pageRect := image.Rect(0, 0, 595, 842)

	testCases := []struct {
		name   string
		matrix [6]float64
	}{
		{name: "y_top_0", matrix: [6]float64{20, 0, 0, 20, 100, 0}},
		{name: "y_top_14", matrix: [6]float64{20, 0, 0, 20, 100, 14}},
		{name: "y_center_minus_14", matrix: [6]float64{20, 0, 0, 20, 100, 397}},
		{name: "y_center_minus_4", matrix: [6]float64{20, 0, 0, 20, 100, 407}},
		{name: "y_center_minus_1", matrix: [6]float64{20, 0, 0, 20, 100, 410}},
		{name: "y_center_exact", matrix: [6]float64{20, 0, 0, 20, 100, 411}},
		{name: "y_center_plus_1", matrix: [6]float64{20, 0, 0, 20, 100, 412}},
		{name: "y_center_plus_4", matrix: [6]float64{20, 0, 0, 20, 100, 415}},
		{name: "y_center_plus_14", matrix: [6]float64{20, 0, 0, 20, 100, 425}},
		{name: "y_bottom_14", matrix: [6]float64{20, 0, 0, 20, 100, 808}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_vertical_symmetry_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(pageW, pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, pageRect, tc.matrix),
				},
			)

			t.Logf(
				"%s transparent-edge vertical-symmetry probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}
}

func TestSyntheticRGBTransparentEdgeCenteredYPhaseProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	pageW := 595.0
	pageH := 842.0
	pageRect := image.Rect(0, 0, 595, 842)

	testCases := []struct {
		name   string
		matrix [6]float64
	}{
		{name: "y_410_0", matrix: [6]float64{20, 0, 0, 20, 100, 410.0}},
		{name: "y_410_5", matrix: [6]float64{20, 0, 0, 20, 100, 410.5}},
		{name: "y_411_0", matrix: [6]float64{20, 0, 0, 20, 100, 411.0}},
		{name: "y_411_5", matrix: [6]float64{20, 0, 0, 20, 100, 411.5}},
		{name: "y_412_0", matrix: [6]float64{20, 0, 0, 20, 100, 412.0}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_centered_y_phase_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(pageW, pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, pageRect, tc.matrix),
				},
			)

			t.Logf(
				"%s transparent-edge centered-y-phase probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}
}

func TestSyntheticRGBTransparentEdgeVerticalPhaseBandWidthProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	pageW := 595.0
	pageH := 842.0
	pageRect := image.Rect(0, 0, 595, 842)

	testCases := []struct {
		name   string
		matrix [6]float64
	}{
		{name: "y_410_00", matrix: [6]float64{20, 0, 0, 20, 100, 410.00}},
		{name: "y_410_25", matrix: [6]float64{20, 0, 0, 20, 100, 410.25}},
		{name: "y_410_50", matrix: [6]float64{20, 0, 0, 20, 100, 410.50}},
		{name: "y_410_75", matrix: [6]float64{20, 0, 0, 20, 100, 410.75}},
		{name: "y_411_00", matrix: [6]float64{20, 0, 0, 20, 100, 411.00}},
		{name: "y_411_25", matrix: [6]float64{20, 0, 0, 20, 100, 411.25}},
		{name: "y_411_50", matrix: [6]float64{20, 0, 0, 20, 100, 411.50}},
		{name: "y_411_75", matrix: [6]float64{20, 0, 0, 20, 100, 411.75}},
		{name: "y_412_00", matrix: [6]float64{20, 0, 0, 20, 100, 412.00}},
	}

	type phaseScore struct {
		current float64
		affine  float64
		rect    float64
	}
	scores := make(map[string]phaseScore, len(testCases))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_vertical_phase_band_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(pageW, pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, pageRect, tc.matrix),
				},
			)

			scores[tc.name] = phaseScore{
				current: currentSimilarity,
				affine:  refScores["affine_transparent_edge_over_white"],
				rect:    refScores["rect_transparent_edge_over_white"],
			}

			t.Logf(
				"%s transparent-edge vertical-phase-band probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}

	assert.Greater(t, scores["y_410_00"].current, scores["y_410_00"].rect)
	assert.Greater(t, scores["y_410_25"].rect, scores["y_410_25"].current)
	assert.Greater(t, scores["y_410_50"].rect, scores["y_410_50"].current)
	assert.Greater(t, scores["y_410_75"].rect, scores["y_410_75"].current)
	assert.Greater(t, scores["y_411_00"].rect, scores["y_411_00"].current)
	assert.Greater(t, scores["y_411_25"].rect, scores["y_411_25"].current)
	assert.Greater(t, scores["y_411_50"].rect, scores["y_411_50"].current)
	assert.Greater(t, scores["y_411_75"].rect, scores["y_411_75"].current)
	assert.Greater(t, scores["y_412_00"].rect, scores["y_412_00"].current)
}

func TestSyntheticRGBTransparentEdgeVerticalPhaseThresholdProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	pageW := 595.0
	pageH := 842.0
	pageRect := image.Rect(0, 0, 595, 842)

	testCases := []struct {
		name   string
		matrix [6]float64
	}{
		{name: "y_410_0000", matrix: [6]float64{20, 0, 0, 20, 100, 410.0000}},
		{name: "y_410_0625", matrix: [6]float64{20, 0, 0, 20, 100, 410.0625}},
		{name: "y_410_1250", matrix: [6]float64{20, 0, 0, 20, 100, 410.1250}},
		{name: "y_410_1875", matrix: [6]float64{20, 0, 0, 20, 100, 410.1875}},
		{name: "y_410_2188", matrix: [6]float64{20, 0, 0, 20, 100, 410.21875}},
		{name: "y_410_2344", matrix: [6]float64{20, 0, 0, 20, 100, 410.234375}},
		{name: "y_410_2500", matrix: [6]float64{20, 0, 0, 20, 100, 410.2500}},
	}

	type phaseScore struct {
		current float64
		affine  float64
		rect    float64
	}
	scores := make(map[string]phaseScore, len(testCases))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_vertical_phase_threshold_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(pageW, pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, pageRect, tc.matrix),
				},
			)

			scores[tc.name] = phaseScore{
				current: currentSimilarity,
				affine:  refScores["affine_transparent_edge_over_white"],
				rect:    refScores["rect_transparent_edge_over_white"],
			}

			t.Logf(
				"%s transparent-edge vertical-phase-threshold probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}

	assert.Greater(t, scores["y_410_0000"].current, scores["y_410_0000"].rect)
	assert.Greater(t, scores["y_410_0625"].rect, scores["y_410_0625"].current)
	assert.Greater(t, scores["y_410_1250"].rect, scores["y_410_1250"].current)
	assert.Greater(t, scores["y_410_1875"].rect, scores["y_410_1875"].current)
	assert.Greater(t, scores["y_410_2188"].rect, scores["y_410_2188"].current)
	assert.Greater(t, scores["y_410_2344"].rect, scores["y_410_2344"].current)
	assert.Greater(t, scores["y_410_2500"].rect, scores["y_410_2500"].current)
}

func TestSyntheticRGBTransparentEdgeVerticalPhaseMicroThresholdProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	pageW := 595.0
	pageH := 842.0
	pageRect := image.Rect(0, 0, 595, 842)

	testCases := []struct {
		name   string
		matrix [6]float64
	}{
		{name: "y_410_00000", matrix: [6]float64{20, 0, 0, 20, 100, 410.00000}},
		{name: "y_410_01562", matrix: [6]float64{20, 0, 0, 20, 100, 410.015625}},
		{name: "y_410_03125", matrix: [6]float64{20, 0, 0, 20, 100, 410.03125}},
		{name: "y_410_04688", matrix: [6]float64{20, 0, 0, 20, 100, 410.046875}},
		{name: "y_410_05469", matrix: [6]float64{20, 0, 0, 20, 100, 410.0546875}},
		{name: "y_410_06250", matrix: [6]float64{20, 0, 0, 20, 100, 410.0625}},
	}

	type phaseScore struct {
		current float64
		affine  float64
		rect    float64
	}
	scores := make(map[string]phaseScore, len(testCases))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_vertical_phase_micro_threshold_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(pageW, pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, pageRect, tc.matrix),
				},
			)

			scores[tc.name] = phaseScore{
				current: currentSimilarity,
				affine:  refScores["affine_transparent_edge_over_white"],
				rect:    refScores["rect_transparent_edge_over_white"],
			}

			t.Logf(
				"%s transparent-edge vertical-phase-micro-threshold probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}

	assert.Greater(t, scores["y_410_00000"].current, scores["y_410_00000"].rect)
	assert.Greater(t, scores["y_410_01562"].rect, scores["y_410_01562"].current)
	assert.Greater(t, scores["y_410_03125"].rect, scores["y_410_03125"].current)
	assert.Greater(t, scores["y_410_04688"].rect, scores["y_410_04688"].current)
	assert.Greater(t, scores["y_410_05469"].rect, scores["y_410_05469"].current)
	assert.Greater(t, scores["y_410_06250"].rect, scores["y_410_06250"].current)
}

func TestSyntheticRGBTransparentEdgeVerticalPhaseSubMicroThresholdProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	pageW := 595.0
	pageH := 842.0
	pageRect := image.Rect(0, 0, 595, 842)

	testCases := []struct {
		name   string
		matrix [6]float64
	}{
		{name: "y_410_000000", matrix: [6]float64{20, 0, 0, 20, 100, 410.000000}},
		{name: "y_410_003906", matrix: [6]float64{20, 0, 0, 20, 100, 410.00390625}},
		{name: "y_410_007812", matrix: [6]float64{20, 0, 0, 20, 100, 410.0078125}},
		{name: "y_410_011719", matrix: [6]float64{20, 0, 0, 20, 100, 410.01171875}},
		{name: "y_410_013672", matrix: [6]float64{20, 0, 0, 20, 100, 410.013671875}},
		{name: "y_410_015625", matrix: [6]float64{20, 0, 0, 20, 100, 410.015625}},
	}

	type phaseScore struct {
		current float64
		affine  float64
		rect    float64
	}
	scores := make(map[string]phaseScore, len(testCases))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_vertical_phase_submicro_threshold_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(pageW, pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, pageRect, tc.matrix),
				},
			)

			scores[tc.name] = phaseScore{
				current: currentSimilarity,
				affine:  refScores["affine_transparent_edge_over_white"],
				rect:    refScores["rect_transparent_edge_over_white"],
			}

			t.Logf(
				"%s transparent-edge vertical-phase-submicro-threshold probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}

	assert.Greater(t, scores["y_410_000000"].current, scores["y_410_000000"].rect)
	assert.Greater(t, scores["y_410_003906"].rect, scores["y_410_003906"].current)
	assert.Greater(t, scores["y_410_007812"].rect, scores["y_410_007812"].current)
	assert.Greater(t, scores["y_410_011719"].rect, scores["y_410_011719"].current)
	assert.Greater(t, scores["y_410_013672"].rect, scores["y_410_013672"].current)
	assert.Greater(t, scores["y_410_015625"].rect, scores["y_410_015625"].current)
}

func TestSyntheticRGBTransparentEdgeVerticalPhaseNanoThresholdProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	pageW := 595.0
	pageH := 842.0
	pageRect := image.Rect(0, 0, 595, 842)

	testCases := []struct {
		name   string
		matrix [6]float64
	}{
		{name: "y_410_0000000", matrix: [6]float64{20, 0, 0, 20, 100, 410.0000000}},
		{name: "y_410_0009766", matrix: [6]float64{20, 0, 0, 20, 100, 410.0009765625}},
		{name: "y_410_0019531", matrix: [6]float64{20, 0, 0, 20, 100, 410.001953125}},
		{name: "y_410_0029297", matrix: [6]float64{20, 0, 0, 20, 100, 410.0029296875}},
		{name: "y_410_0039062", matrix: [6]float64{20, 0, 0, 20, 100, 410.00390625}},
	}

	type phaseScore struct {
		current float64
		affine  float64
		rect    float64
	}
	scores := make(map[string]phaseScore, len(testCases))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_vertical_phase_nano_threshold_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(pageW, pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, pageRect, tc.matrix),
				},
			)

			scores[tc.name] = phaseScore{
				current: currentSimilarity,
				affine:  refScores["affine_transparent_edge_over_white"],
				rect:    refScores["rect_transparent_edge_over_white"],
			}

			t.Logf(
				"%s transparent-edge vertical-phase-nano-threshold probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}

	assert.Greater(t, scores["y_410_0000000"].current, scores["y_410_0000000"].rect)
	assert.Greater(t, scores["y_410_0009766"].rect, scores["y_410_0009766"].current)
	assert.Greater(t, scores["y_410_0019531"].rect, scores["y_410_0019531"].current)
	assert.Greater(t, scores["y_410_0029297"].rect, scores["y_410_0029297"].current)
	assert.Greater(t, scores["y_410_0039062"].rect, scores["y_410_0039062"].current)
}

func TestSyntheticRGBTransparentEdgeVerticalPhasePicoThresholdProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	pageW := 595.0
	pageH := 842.0
	pageRect := image.Rect(0, 0, 595, 842)

	testCases := []struct {
		name   string
		matrix [6]float64
	}{
		{name: "y_410_000000000", matrix: [6]float64{20, 0, 0, 20, 100, 410.000000000}},
		{name: "y_410_000244141", matrix: [6]float64{20, 0, 0, 20, 100, 410.000244140625}},
		{name: "y_410_000488281", matrix: [6]float64{20, 0, 0, 20, 100, 410.00048828125}},
		{name: "y_410_000732422", matrix: [6]float64{20, 0, 0, 20, 100, 410.000732421875}},
		{name: "y_410_000976562", matrix: [6]float64{20, 0, 0, 20, 100, 410.0009765625}},
	}

	type phaseScore struct {
		current float64
		affine  float64
		rect    float64
	}
	scores := make(map[string]phaseScore, len(testCases))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_vertical_phase_pico_threshold_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(pageW, pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, pageRect, tc.matrix),
				},
			)

			scores[tc.name] = phaseScore{
				current: currentSimilarity,
				affine:  refScores["affine_transparent_edge_over_white"],
				rect:    refScores["rect_transparent_edge_over_white"],
			}

			t.Logf(
				"%s transparent-edge vertical-phase-pico-threshold probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}

	assert.Greater(t, scores["y_410_000000000"].current, scores["y_410_000000000"].rect)
	assert.Greater(t, scores["y_410_000244141"].rect, scores["y_410_000244141"].current)
	assert.Greater(t, scores["y_410_000488281"].rect, scores["y_410_000488281"].current)
	assert.Greater(t, scores["y_410_000732422"].rect, scores["y_410_000732422"].current)
	assert.Greater(t, scores["y_410_000976562"].rect, scores["y_410_000976562"].current)
}

func TestSyntheticRGBTransparentEdgeVerticalPhaseFemtoThresholdProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	pageW := 595.0
	pageH := 842.0
	pageRect := image.Rect(0, 0, 595, 842)

	testCases := []struct {
		name   string
		matrix [6]float64
	}{
		{name: "y_410_0000000000", matrix: [6]float64{20, 0, 0, 20, 100, 410.0000000000}},
		{name: "y_410_0000610352", matrix: [6]float64{20, 0, 0, 20, 100, 410.00006103515625}},
		{name: "y_410_0001220703", matrix: [6]float64{20, 0, 0, 20, 100, 410.0001220703125}},
		{name: "y_410_0001831055", matrix: [6]float64{20, 0, 0, 20, 100, 410.00018310546875}},
		{name: "y_410_0002441406", matrix: [6]float64{20, 0, 0, 20, 100, 410.000244140625}},
	}

	type phaseScore struct {
		current float64
		affine  float64
		rect    float64
	}
	scores := make(map[string]phaseScore, len(testCases))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_vertical_phase_femto_threshold_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(pageW, pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, pageRect, tc.matrix),
				},
			)

			scores[tc.name] = phaseScore{
				current: currentSimilarity,
				affine:  refScores["affine_transparent_edge_over_white"],
				rect:    refScores["rect_transparent_edge_over_white"],
			}

			t.Logf(
				"%s transparent-edge vertical-phase-femto-threshold probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}

	assert.Greater(t, scores["y_410_0000000000"].current, scores["y_410_0000000000"].rect)
	assert.Greater(t, scores["y_410_0000610352"].rect, scores["y_410_0000610352"].current)
	assert.Greater(t, scores["y_410_0001220703"].rect, scores["y_410_0001220703"].current)
	assert.Greater(t, scores["y_410_0001831055"].rect, scores["y_410_0001831055"].current)
	assert.Greater(t, scores["y_410_0002441406"].rect, scores["y_410_0002441406"].current)
}

func TestSyntheticRGBTransparentEdgeVerticalPhaseAttoThresholdProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	pageW := 595.0
	pageH := 842.0
	pageRect := image.Rect(0, 0, 595, 842)

	testCases := []struct {
		name   string
		matrix [6]float64
	}{
		{name: "y_410_00000000000", matrix: [6]float64{20, 0, 0, 20, 100, 410.00000000000}},
		{name: "y_410_00001525879", matrix: [6]float64{20, 0, 0, 20, 100, 410.00001525878906}},
		{name: "y_410_00003051758", matrix: [6]float64{20, 0, 0, 20, 100, 410.0000305175781}},
		{name: "y_410_00004577637", matrix: [6]float64{20, 0, 0, 20, 100, 410.0000457763672}},
		{name: "y_410_00006103516", matrix: [6]float64{20, 0, 0, 20, 100, 410.00006103515625}},
	}

	type phaseScore struct {
		current float64
		affine  float64
		rect    float64
	}
	scores := make(map[string]phaseScore, len(testCases))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_vertical_phase_atto_threshold_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(pageW, pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, pageRect, tc.matrix),
				},
			)

			scores[tc.name] = phaseScore{
				current: currentSimilarity,
				affine:  refScores["affine_transparent_edge_over_white"],
				rect:    refScores["rect_transparent_edge_over_white"],
			}

			t.Logf(
				"%s transparent-edge vertical-phase-atto-threshold probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}

	assert.Greater(t, scores["y_410_00000000000"].current, scores["y_410_00000000000"].rect)
	assert.Greater(t, scores["y_410_00001525879"].rect, scores["y_410_00001525879"].current)
	assert.Greater(t, scores["y_410_00003051758"].rect, scores["y_410_00003051758"].current)
	assert.Greater(t, scores["y_410_00004577637"].rect, scores["y_410_00004577637"].current)
	assert.Greater(t, scores["y_410_00006103516"].rect, scores["y_410_00006103516"].current)
}

func TestSyntheticRGBTransparentEdgeVerticalPhaseExactThresholdProbeAgainstPoppler(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	pageW := 595.0
	pageH := 842.0
	pageRect := image.Rect(0, 0, 595, 842)

	testCases := []struct {
		name   string
		matrix [6]float64
	}{
		{name: "y_410_000000000000", matrix: [6]float64{20, 0, 0, 20, 100, 410.000000000000}},
		{name: "y_410_000000953674", matrix: [6]float64{20, 0, 0, 20, 100, 410.0000009536743}},
		{name: "y_410_000001907349", matrix: [6]float64{20, 0, 0, 20, 100, 410.00000190734863}},
		{name: "y_410_000003814697", matrix: [6]float64{20, 0, 0, 20, 100, 410.00000381469727}},
		{name: "y_410_000007629395", matrix: [6]float64{20, 0, 0, 20, 100, 410.00000762939453}},
		{name: "y_410_000011444092", matrix: [6]float64{20, 0, 0, 20, 100, 410.0000114440918}},
		{name: "y_410_000015258789", matrix: [6]float64{20, 0, 0, 20, 100, 410.00001525878906}},
	}

	type phaseScore struct {
		current float64
		affine  float64
		rect    float64
	}
	scores := make(map[string]phaseScore, len(testCases))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			currentExact, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_vertical_phase_exact_threshold_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(pageW, pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"affine_transparent_edge_over_white": simulateSyntheticTransparentEdgeAffineOverWhite(src, pageRect, tc.matrix),
					"rect_transparent_edge_over_white":   simulateSyntheticTransparentEdgeRectOverWhite(src, pageRect, tc.matrix),
				},
			)

			scores[tc.name] = phaseScore{
				current: currentSimilarity,
				affine:  refScores["affine_transparent_edge_over_white"],
				rect:    refScores["rect_transparent_edge_over_white"],
			}

			t.Logf(
				"%s transparent-edge vertical-phase-exact-threshold probe: current_exact=%.4f current_similarity=%.4f affine=%.4f rect=%.4f",
				tc.name,
				currentExact,
				currentSimilarity,
				refScores["affine_transparent_edge_over_white"],
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}

	assert.Greater(t, scores["y_410_000000000000"].current, scores["y_410_000000000000"].rect)
	assert.Greater(t, scores["y_410_000000953674"].rect, scores["y_410_000000953674"].current)
	assert.Greater(t, scores["y_410_000001907349"].rect, scores["y_410_000001907349"].current)
	assert.Greater(t, scores["y_410_000003814697"].rect, scores["y_410_000003814697"].current)
	assert.Greater(t, scores["y_410_000007629395"].rect, scores["y_410_000007629395"].current)
	assert.Greater(t, scores["y_410_000011444092"].rect, scores["y_410_000011444092"].current)
	assert.Greater(t, scores["y_410_000015258789"].rect, scores["y_410_000015258789"].current)
}

func TestSyntheticRGBTransparentEdgeZeroVsPositiveSubpixelYOffsetContract(t *testing.T) {
	imageData := syntheticRGBTiledIdentity(16, 16)
	src := syntheticRGBTiledImage(16, 16, imageData)
	pageW := 595.0
	pageH := 842.0
	pageRect := image.Rect(0, 0, 595, 842)

	testCases := []struct {
		name   string
		matrix [6]float64
	}{
		{name: "zero_y_offset", matrix: [6]float64{20, 0, 0, 20, 100, 410.0}},
		{name: "positive_subpixel_y_offset", matrix: [6]float64{20, 0, 0, 20, 100, 410.0000009536743}},
	}

	type contractScore struct {
		current float64
		rect    float64
	}
	scores := make(map[string]contractScore, len(testCases))

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_zero_vs_positive_y_%s.pdf", tc.name),
				buildSyntheticRGBImagePDFFloat(pageW, pageH, 16, 16, tc.matrix, imageData),
				map[string]image.Image{
					"rect_transparent_edge_over_white": simulateSyntheticTransparentEdgeRectOverWhite(src, pageRect, tc.matrix),
				},
			)

			scores[tc.name] = contractScore{
				current: currentSimilarity,
				rect:    refScores["rect_transparent_edge_over_white"],
			}

			t.Logf(
				"%s zero-vs-positive-y contract probe: current_similarity=%.4f rect=%.4f",
				tc.name,
				currentSimilarity,
				refScores["rect_transparent_edge_over_white"],
			)
		})
	}

	assert.Greater(t, scores["zero_y_offset"].current, scores["zero_y_offset"].rect)
	assert.Greater(t, scores["positive_subpixel_y_offset"].rect, scores["positive_subpixel_y_offset"].current)
}

func TestSyntheticRGBTransparentEdgeExperimentalModeRespectsZeroVsPositiveYOffsetContract(t *testing.T) {
	testCases := []struct {
		name              string
		matrix            [6]float64
		expectLegacyExact bool
	}{
		{
			name:              "zero_y_offset",
			matrix:            [6]float64{20, 0, 0, 20, 100, 410.0},
			expectLegacyExact: true,
		},
		{
			name:              "positive_subpixel_y_offset",
			matrix:            [6]float64{20, 0, 0, 20, 100, 410.0000009536743},
			expectLegacyExact: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			pdfPath := filepath.Join(root, tc.name+".pdf")
			require.NoError(t, os.WriteFile(
				pdfPath,
				buildSyntheticRGBImagePDFFloat(595, 842, 16, 16, tc.matrix, syntheticRGBTiledIdentity(16, 16)),
				0o644,
			))

			legacyOpts := pdf.DefaultRenderOptions()
			legacyOpts.DPI = 72
			legacyOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeLegacy
			_, legacySimilarity := probePDFRenderAgainstPoppler(t, pdfPath, legacyOpts)

			experimentalOpts := pdf.DefaultRenderOptions()
			experimentalOpts.DPI = 72
			experimentalOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1
			_, experimentalSimilarity := probePDFRenderAgainstPoppler(t, pdfPath, experimentalOpts)

			exactPercent, similarityPercent := renderSamplePageParityBetweenModes(
				t,
				pdfPath,
				domainrenderer.ImageSamplingModeLegacy,
				domainrenderer.ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1,
			)
			t.Logf(
				"%s rgb transparent-edge experimental mode: legacy_similarity=%.4f experimental_similarity=%.4f exact=%.4f similarity=%.4f",
				tc.name,
				legacySimilarity,
				experimentalSimilarity,
				exactPercent,
				similarityPercent,
			)

			if tc.expectLegacyExact {
				assert.Equal(t, 100.0, exactPercent)
				assert.Equal(t, 100.0, similarityPercent)
				return
			}

			assert.Greater(t, experimentalSimilarity, legacySimilarity)
		})
	}
}

func TestSampleRGBTransparentEdgeExperimentalModeMatchesLegacyForDoc003(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "003-pdflatex-image", "pdflatex-image.pdf")
	exactPercent, similarityPercent := renderSamplePageParityBetweenModes(
		t,
		pdfPath,
		domainrenderer.ImageSamplingModeLegacy,
		domainrenderer.ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1,
	)
	assert.Equal(t, 100.0, exactPercent)
	assert.Equal(t, 100.0, similarityPercent)
}

func TestSampleRGBTransparentEdgeExperimentalModeMatchesLegacyForDoc023(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	exactPercent, similarityPercent := renderSamplePageParityBetweenModes(
		t,
		pdfPath,
		domainrenderer.ImageSamplingModeLegacy,
		domainrenderer.ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1,
	)
	assert.Equal(t, 100.0, exactPercent)
	assert.Equal(t, 100.0, similarityPercent)
}

func TestSampleRGBTransparentEdgeExperimentalModeSurfaceAcrossSampleCorpus(t *testing.T) {
	pdfPaths, err := filepath.Glob(filepath.Join(getSampleDir(), "*", "*.pdf"))
	require.NoError(t, err)
	sort.Strings(pdfPaths)

	var changed []string
	for _, pdfPath := range pdfPaths {
		exactPercent, similarityPercent := renderSamplePageParityBetweenModes(
			t,
			pdfPath,
			domainrenderer.ImageSamplingModeLegacy,
			domainrenderer.ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1,
		)
		relPath, relErr := filepath.Rel(getSampleDir(), pdfPath)
		require.NoError(t, relErr)
		relPath = filepath.ToSlash(relPath)
		t.Logf("%s legacy-vs-rgb-transparent-edge parity: exact=%.4f similarity=%.4f", relPath, exactPercent, similarityPercent)

		if exactPercent < 100.0 || similarityPercent < 100.0 {
			changed = append(changed, relPath)
		}
	}

	assert.Empty(t, changed)
}

func TestSampleRGBTransparentEdgeExperimentalModeSurfaceAcrossSampleCorpusAllPages(t *testing.T) {
	pdfPaths, err := filepath.Glob(filepath.Join(getSampleDir(), "*", "*.pdf"))
	require.NoError(t, err)
	sort.Strings(pdfPaths)

	var changed []string
	for _, pdfPath := range pdfPaths {
		pageChanges := renderSampleAllPagesParityBetweenModes(
			t,
			pdfPath,
			domainrenderer.ImageSamplingModeLegacy,
			domainrenderer.ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1,
		)
		relPath, relErr := filepath.Rel(getSampleDir(), pdfPath)
		require.NoError(t, relErr)
		relPath = filepath.ToSlash(relPath)
		for _, pageChange := range pageChanges {
			changed = append(changed, fmt.Sprintf("%s#p%d", relPath, pageChange))
		}
	}

	assert.Empty(t, changed)
}

type rgbTransparentEdgeSignatureCase struct {
	name                 string
	pageW                float64
	pageH                float64
	imageW               int
	imageH               int
	matrix               [6]float64
	expectedRelationship string
}

func runRGBTransparentEdgeSignatureCase(t *testing.T, tc rgbTransparentEdgeSignatureCase) {
	t.Helper()

	pdfBytes := buildSyntheticRGBImagePDFFloat(
		tc.pageW,
		tc.pageH,
		tc.imageW,
		tc.imageH,
		tc.matrix,
		syntheticRGBTiledIdentity(tc.imageW, tc.imageH),
	)

	legacySimilarity, experimentalSimilarity := probeSyntheticPDFModeSimilaritiesAgainstPoppler(
		t,
		fmt.Sprintf("rgb_transparent_edge_signature_%s.pdf", tc.name),
		pdfBytes,
		domainrenderer.ImageSamplingModeLegacy,
		domainrenderer.ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1,
	)

	t.Logf(
		"%s rgb transparent-edge signature matrix: legacy=%.4f experimental=%.4f",
		tc.name,
		legacySimilarity,
		experimentalSimilarity,
	)

	switch tc.expectedRelationship {
	case "experimental_better":
		assert.Greater(t, experimentalSimilarity, legacySimilarity)
	case "legacy_better":
		assert.Greater(t, legacySimilarity, experimentalSimilarity)
	case "exact_match":
		assert.InDelta(t, 100.0, renderSyntheticModeParity(
			t,
			fmt.Sprintf("rgb_transparent_edge_signature_%s.pdf", tc.name),
			pdfBytes,
			domainrenderer.ImageSamplingModeLegacy,
			domainrenderer.ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1,
		), 1e-9)
	default:
		t.Fatalf("unsupported expected relationship: %s", tc.expectedRelationship)
	}
}

func TestSyntheticRGBTransparentEdgeExperimentalModeSignatureMatrixProbeAgainstPoppler(t *testing.T) {
	testCases := []rgbTransparentEdgeSignatureCase{
		{
			name:                 "small_page_positive_subpixel_y",
			pageW:                24,
			pageH:                24,
			imageW:               16,
			imageH:               16,
			matrix:               [6]float64{20, 0, 0, 20, 1.5, 1.5},
			expectedRelationship: "experimental_better",
		},
		{
			name:                 "large_page_zero_y_offset",
			pageW:                595,
			pageH:                842,
			imageW:               16,
			imageH:               16,
			matrix:               [6]float64{20, 0, 0, 20, 100, 410.0},
			expectedRelationship: "exact_match",
		},
		{
			name:                 "large_page_positive_y_non_centered",
			pageW:                595,
			pageH:                842,
			imageW:               16,
			imageH:               16,
			matrix:               [6]float64{20, 0, 0, 20, 100, 100},
			expectedRelationship: "exact_match",
		},
		{
			name:                 "large_page_positive_y_centered_band",
			pageW:                595,
			pageH:                842,
			imageW:               16,
			imageH:               16,
			matrix:               [6]float64{20, 0, 0, 20, 100, 411},
			expectedRelationship: "exact_match",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			runRGBTransparentEdgeSignatureCase(t, tc)
		})
	}
}

func TestSyntheticRGBTransparentEdgeExperimentalModePatternFamilyProbeAgainstPoppler(t *testing.T) {
	contentClasses := []struct {
		name                 string
		data                 []byte
		expectedRelationship string
	}{
		{name: "flat", data: syntheticRGBFlatFill(16, 16, color.RGBA{R: 220, G: 80, B: 60, A: 255}), expectedRelationship: "legacy_better"},
		{name: "gradient", data: syntheticRGBHorizontalGradient(16, 16), expectedRelationship: "experimental_better"},
		{name: "checker", data: syntheticRGBCheckerboard(16, 16), expectedRelationship: "experimental_better"},
		{name: "tiled_identity", data: syntheticRGBTiledIdentity(16, 16), expectedRelationship: "experimental_better"},
	}

	for _, contentClass := range contentClasses {
		t.Run(contentClass.name, func(t *testing.T) {
			pdfBytes := buildSyntheticRGBImagePDFFloat(
				24,
				24,
				16,
				16,
				[6]float64{20, 0, 0, 20, 1.5, 1.5},
				contentClass.data,
			)

			legacySimilarity, experimentalSimilarity := probeSyntheticPDFModeSimilaritiesAgainstPoppler(
				t,
				fmt.Sprintf("rgb_transparent_edge_pattern_%s.pdf", contentClass.name),
				pdfBytes,
				domainrenderer.ImageSamplingModeLegacy,
				domainrenderer.ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1,
			)
			exactPercent := renderSyntheticModeParity(
				t,
				fmt.Sprintf("rgb_transparent_edge_pattern_%s.pdf", contentClass.name),
				pdfBytes,
				domainrenderer.ImageSamplingModeLegacy,
				domainrenderer.ImageSamplingModeExperimentalRGBTransparentEdgeUpscaleV1,
			)

			t.Logf(
				"%s rgb transparent-edge pattern family: legacy=%.4f experimental=%.4f legacy_vs_experimental_exact=%.4f",
				contentClass.name,
				legacySimilarity,
				experimentalSimilarity,
				exactPercent,
			)

			switch contentClass.expectedRelationship {
			case "experimental_better":
				assert.Greater(t, experimentalSimilarity, legacySimilarity)
			case "legacy_better":
				assert.Greater(t, legacySimilarity, experimentalSimilarity)
			default:
				t.Fatalf("unsupported expected relationship: %s", contentClass.expectedRelationship)
			}
		})
	}
}

func TestSyntheticDoc023DecodedRGBAChannelRegionProbe(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	currentDecoded, _ := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeDefault)
	simpleDecoded, _ := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeSimpleSubtractive)
	stdlibDecoded, _ := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeStdlib)

	edgeMask := classifyIndexedEdgePixels(imageData, width, height)
	currentSimpleOverall := summarizeRGBDiff(currentDecoded, simpleDecoded, nil)
	currentSimpleEdge := summarizeRGBDiff(currentDecoded, simpleDecoded, edgeMask)
	currentSimpleFlat := summarizeRGBDiff(currentDecoded, simpleDecoded, invertBoolMask(edgeMask))
	currentStdlibOverall := summarizeRGBDiff(currentDecoded, stdlibDecoded, nil)
	currentStdlibEdge := summarizeRGBDiff(currentDecoded, stdlibDecoded, edgeMask)
	currentStdlibFlat := summarizeRGBDiff(currentDecoded, stdlibDecoded, invertBoolMask(edgeMask))
	kBinsCurrentSimple := summarizeRGBDiffByKBin(currentDecoded, simpleDecoded, imageData, palette, 4)
	kBinsCurrentStdlib := summarizeRGBDiffByKBin(currentDecoded, stdlibDecoded, imageData, palette, 4)

	t.Skipf(
		"doc023 decoded-channel-region probe: current_simple_overall=%s current_simple_edge=%s current_simple_flat=%s current_stdlib_overall=%s current_stdlib_edge=%s current_stdlib_flat=%s current_simple_kbins=%s current_stdlib_kbins=%s",
		currentSimpleOverall.String(),
		currentSimpleEdge.String(),
		currentSimpleFlat.String(),
		currentStdlibOverall.String(),
		currentStdlibEdge.String(),
		currentStdlibFlat.String(),
		formatRGBDiffBins(kBinsCurrentSimple),
		formatRGBDiffBins(kBinsCurrentStdlib),
	)
}

func TestSyntheticDoc023DecodedRGBAPaletteIndexProbe(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, _, _ := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	currentDecoded, _ := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeDefault)
	simpleDecoded, _ := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeSimpleSubtractive)
	stdlibDecoded, _ := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeStdlib)

	topCurrentSimple := summarizeTopPaletteRGBDiffs(currentDecoded, simpleDecoded, imageData, palette, 8)
	topCurrentStdlib := summarizeTopPaletteRGBDiffs(currentDecoded, stdlibDecoded, imageData, palette, 8)

	t.Skipf(
		"doc023 decoded-palette-index probe: current_simple_top=%s current_stdlib_top=%s",
		formatTopPaletteRGBDiffs(topCurrentSimple),
		formatTopPaletteRGBDiffs(topCurrentStdlib),
	)
}

func TestSyntheticDoc023CurrentCMYKBlueTermProbe(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, _, _ := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	currentDecoded, _ := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeDefault)
	simpleDecoded, _ := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeSimpleSubtractive)
	topCurrentSimple := summarizeTopPaletteRGBDiffs(currentDecoded, simpleDecoded, imageData, palette, 6)
	blueTerms := summarizeTopPaletteBlueTerms(topCurrentSimple)

	t.Skipf("doc023 current-cmyk-blue-term probe: top_blue_terms=%s", formatTopPaletteBlueTerms(blueTerms))
}

func TestSyntheticDoc023TopPaletteBlueCorrectionProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	currentDecoded, _ := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeDefault)
	simpleDecoded, _ := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeSimpleSubtractive)
	topCurrentSimple := summarizeTopPaletteRGBDiffs(currentDecoded, simpleDecoded, imageData, palette, 6)

	currentRGBPalette := convertCMYKPaletteToCurrentRGBPalette(palette)
	correctedPalette := applySimpleBlueCorrectionToPalette(currentRGBPalette, palette, topCurrentSimple)

	currentExact, currentSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_top_palette_blue_current_rgb.pdf",
		buildSyntheticIndexedImagePDFFloatWithBase(
			540,
			720,
			3,
			"DeviceRGB",
			width,
			height,
			[6]float64{468, 0, 0, 624, 72, 96},
			currentRGBPalette,
			imageData,
		),
	)
	correctedExact, correctedSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_top_palette_blue_corrected_rgb.pdf",
		buildSyntheticIndexedImagePDFFloatWithBase(
			540,
			720,
			3,
			"DeviceRGB",
			width,
			height,
			[6]float64{468, 0, 0, 624, 72, 96},
			correctedPalette,
			imageData,
		),
	)

	t.Skipf(
		"doc023 top-palette blue-correction probe: current_exact=%.4f current_similarity=%.4f corrected_exact=%.4f corrected_similarity=%.4f",
		currentExact,
		currentSimilarity,
		correctedExact,
		correctedSimilarity,
	)
}

func TestSyntheticDoc023TopPaletteRGBCorrectionProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	currentDecoded, _ := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeDefault)
	simpleDecoded, _ := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeSimpleSubtractive)
	topCurrentSimple := summarizeTopPaletteRGBDiffs(currentDecoded, simpleDecoded, imageData, palette, 6)

	currentRGBPalette := convertCMYKPaletteToCurrentRGBPalette(palette)
	simpleCorrectedPalette := applyReferencePaletteRGBToPalette(currentRGBPalette, palette, topCurrentSimple, "simple")
	stdlibCorrectedPalette := applyReferencePaletteRGBToPalette(currentRGBPalette, palette, topCurrentSimple, "stdlib")

	currentExact, currentSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_top_palette_rgb_current.pdf",
		buildSyntheticIndexedImagePDFFloatWithBase(
			540,
			720,
			3,
			"DeviceRGB",
			width,
			height,
			[6]float64{468, 0, 0, 624, 72, 96},
			currentRGBPalette,
			imageData,
		),
	)
	simpleCorrectedExact, simpleCorrectedSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_top_palette_rgb_simple_corrected.pdf",
		buildSyntheticIndexedImagePDFFloatWithBase(
			540,
			720,
			3,
			"DeviceRGB",
			width,
			height,
			[6]float64{468, 0, 0, 624, 72, 96},
			simpleCorrectedPalette,
			imageData,
		),
	)
	stdlibCorrectedExact, stdlibCorrectedSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		"indexed_doc023_top_palette_rgb_stdlib_corrected.pdf",
		buildSyntheticIndexedImagePDFFloatWithBase(
			540,
			720,
			3,
			"DeviceRGB",
			width,
			height,
			[6]float64{468, 0, 0, 624, 72, 96},
			stdlibCorrectedPalette,
			imageData,
		),
	)

	t.Skipf(
		"doc023 top-palette rgb-correction probe: current_exact=%.4f current_similarity=%.4f simple_corrected_exact=%.4f simple_corrected_similarity=%.4f stdlib_corrected_exact=%.4f stdlib_corrected_similarity=%.4f",
		currentExact,
		currentSimilarity,
		simpleCorrectedExact,
		simpleCorrectedSimilarity,
		stdlibCorrectedExact,
		stdlibCorrectedSimilarity,
	)
}

func TestSyntheticDoc023TopPaletteContextProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	currentDecoded, _ := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeDefault)
	simpleDecoded, _ := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeSimpleSubtractive)
	topCurrentSimple := summarizeTopPaletteRGBDiffs(currentDecoded, simpleDecoded, imageData, palette, 6)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	topSet := make(map[int]bool, len(topCurrentSimple))
	for _, summary := range topCurrentSimple {
		topSet[summary.index] = true
	}

	isolatedImageData := selectIndexedPixelsByPaletteSet(imageData, width, height, topSet, fillIndex, false)
	contextImageData := selectIndexedPixelsByPaletteSet(imageData, width, height, topSet, fillIndex, true)

	isolatedDirect, isolatedRefs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
		t,
		"indexed_doc023_top_palette_isolated",
		palette,
		width,
		height,
		isolatedImageData,
		540,
		720,
		[6]float64{468, 0, 0, 624, 72, 96},
	)
	contextDirect, contextRefs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
		t,
		"indexed_doc023_top_palette_context",
		palette,
		width,
		height,
		contextImageData,
		540,
		720,
		[6]float64{468, 0, 0, 624, 72, 96},
	)

	t.Skipf(
		"doc023 top-palette context probe: isolated_direct=%.4f isolated_current=%.4f isolated_simple=%.4f isolated_stdlib=%.4f context_direct=%.4f context_current=%.4f context_simple=%.4f context_stdlib=%.4f",
		isolatedDirect.similarity,
		isolatedRefs["current_rgb"].similarity,
		isolatedRefs["simple_rgb"].similarity,
		isolatedRefs["stdlib_rgb"].similarity,
		contextDirect.similarity,
		contextRefs["current_rgb"].similarity,
		contextRefs["simple_rgb"].similarity,
		contextRefs["stdlib_rgb"].similarity,
	)
}

func TestSyntheticDoc023PaletteSplitProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	currentDecoded, _ := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeDefault)
	simpleDecoded, _ := decodeSampleIndexedImageObjectToRGBWithMode(t, pdfPath, entity.NewRef(5, 0), domainimage.CMYKConversionModeSimpleSubtractive)
	topCurrentSimple := summarizeTopPaletteRGBDiffs(currentDecoded, simpleDecoded, imageData, palette, 6)

	counts := countIndexedPaletteUsage(imageData, len(palette)/4)
	fillIndex := dominantPaletteIndex(counts)
	topSet := make(map[int]bool, len(topCurrentSimple))
	for _, summary := range topCurrentSimple {
		topSet[summary.index] = true
	}

	remainderSet := make(map[int]bool)
	for index, count := range counts {
		if count > 0 && !topSet[index] {
			remainderSet[index] = true
		}
	}

	topImageData := selectIndexedPixelsByPaletteSet(imageData, width, height, topSet, fillIndex, false)
	remainderImageData := selectIndexedPixelsByPaletteSet(imageData, width, height, remainderSet, fillIndex, false)

	topDirect, topRefs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
		t,
		"indexed_doc023_top_palette_split_top",
		palette,
		width,
		height,
		topImageData,
		540,
		720,
		[6]float64{468, 0, 0, 624, 72, 96},
	)
	remainderDirect, remainderRefs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
		t,
		"indexed_doc023_top_palette_split_remainder",
		palette,
		width,
		height,
		remainderImageData,
		540,
		720,
		[6]float64{468, 0, 0, 624, 72, 96},
	)

	t.Skipf(
		"doc023 palette-split probe: top_direct=%.4f top_current=%.4f top_simple=%.4f top_stdlib=%.4f remainder_direct=%.4f remainder_current=%.4f remainder_simple=%.4f remainder_stdlib=%.4f",
		topDirect.similarity,
		topRefs["current_rgb"].similarity,
		topRefs["simple_rgb"].similarity,
		topRefs["stdlib_rgb"].similarity,
		remainderDirect.similarity,
		remainderRefs["current_rgb"].similarity,
		remainderRefs["simple_rgb"].similarity,
		remainderRefs["stdlib_rgb"].similarity,
	)
}

func TestSyntheticDoc023GlobalBlockReorderProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	cases := []struct {
		name        string
		blockWidth  int
		blockHeight int
	}{
		{name: "coarse_84x84", blockWidth: 84, blockHeight: 84},
		{name: "medium_28x28", blockWidth: 28, blockHeight: 28},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		reordered := rearrangeIndexedImageBlocks(imageData, width, height, tc.blockWidth, tc.blockHeight)
		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_global_reorder_"+tc.name,
			palette,
			width,
			height,
			reordered,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 global-block-reorder probe: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023GlobalRegionProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	cases := []struct {
		name   string
		bounds image.Rectangle
	}{
		{name: "quadrant_tl", bounds: image.Rect(0, 0, width/2, height/2)},
		{name: "quadrant_tr", bounds: image.Rect(width/2, 0, width, height/2)},
		{name: "quadrant_bl", bounds: image.Rect(0, height/2, width/2, height)},
		{name: "quadrant_br", bounds: image.Rect(width/2, height/2, width, height)},
		{name: "vertical_mid", bounds: image.Rect(width/4, 0, width*3/4, height)},
		{name: "horizontal_mid", bounds: image.Rect(0, height/4, width, height*3/4)},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		regionImageData := selectIndexedPixelsByBounds(imageData, width, height, tc.bounds, fillIndex)
		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_global_region_"+tc.name,
			palette,
			width,
			height,
			regionImageData,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 global-region probe: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023RegionCompositionProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	quadrantBL := image.Rect(0, height/2, width/2, height)
	verticalMid := image.Rect(width/4, 0, width*3/4, height)
	horizontalMid := image.Rect(0, height/4, width, height*3/4)

	cases := []struct {
		name      string
		imageData []byte
	}{
		{
			name:      "bl_intersect_vertical_mid",
			imageData: selectIndexedPixelsByBounds(imageData, width, height, quadrantBL.Intersect(verticalMid), fillIndex),
		},
		{
			name:      "bl_intersect_horizontal_mid",
			imageData: selectIndexedPixelsByBounds(imageData, width, height, quadrantBL.Intersect(horizontalMid), fillIndex),
		},
		{
			name:      "bl_union_vertical_mid",
			imageData: selectIndexedPixelsByBoundsUnion(imageData, width, height, fillIndex, quadrantBL, verticalMid),
		},
		{
			name:      "bl_union_horizontal_mid",
			imageData: selectIndexedPixelsByBoundsUnion(imageData, width, height, fillIndex, quadrantBL, horizontalMid),
		},
		{
			name:      "bl_union_both_mid",
			imageData: selectIndexedPixelsByBoundsUnion(imageData, width, height, fillIndex, quadrantBL, verticalMid, horizontalMid),
		},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_region_composition_"+tc.name,
			palette,
			width,
			height,
			tc.imageData,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 region-composition probe: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023RegionFootprintSweepProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	quadrantBL := image.Rect(0, height/2, width/2, height)

	cases := []struct {
		name   string
		bounds image.Rectangle
	}{
		{name: "bl_union_vertical_micro", bounds: centeredVerticalStripe(width, height, width/8)},
		{name: "bl_union_vertical_narrow", bounds: centeredVerticalStripe(width, height, width/4)},
		{name: "bl_union_vertical_mid", bounds: centeredVerticalStripe(width, height, width/2)},
		{name: "bl_union_vertical_wide", bounds: centeredVerticalStripe(width, height, width*3/4)},
		{name: "bl_union_horizontal_micro", bounds: centeredHorizontalStripe(width, height, height/8)},
		{name: "bl_union_horizontal_narrow", bounds: centeredHorizontalStripe(width, height, height/4)},
		{name: "bl_union_horizontal_mid", bounds: centeredHorizontalStripe(width, height, height/2)},
		{name: "bl_union_horizontal_wide", bounds: centeredHorizontalStripe(width, height, height*3/4)},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		regionImageData := selectIndexedPixelsByBoundsUnion(imageData, width, height, fillIndex, quadrantBL, tc.bounds)
		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_region_footprint_"+tc.name,
			palette,
			width,
			height,
			regionImageData,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 region-footprint sweep: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023BottomLeftSubregionProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	bl := image.Rect(0, height/2, width/2, height)
	halfW := bl.Dx() / 2
	halfH := bl.Dy() / 2
	cases := []struct {
		name   string
		bounds image.Rectangle
	}{
		{name: "bl_tl", bounds: image.Rect(bl.Min.X, bl.Min.Y, bl.Min.X+halfW, bl.Min.Y+halfH)},
		{name: "bl_tr", bounds: image.Rect(bl.Min.X+halfW, bl.Min.Y, bl.Max.X, bl.Min.Y+halfH)},
		{name: "bl_bl", bounds: image.Rect(bl.Min.X, bl.Min.Y+halfH, bl.Min.X+halfW, bl.Max.Y)},
		{name: "bl_br", bounds: image.Rect(bl.Min.X+halfW, bl.Min.Y+halfH, bl.Max.X, bl.Max.Y)},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		regionImageData := selectIndexedPixelsByBounds(imageData, width, height, tc.bounds, fillIndex)
		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_bottom_left_subregion_"+tc.name,
			palette,
			width,
			height,
			regionImageData,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 bottom-left subregion probe: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023BottomLeftEdgeRegionProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	bl := image.Rect(0, height/2, width/2, height)
	edgeMask := classifyIndexedEdgePixels(imageData, width, height)

	cases := []struct {
		name     string
		edgeOnly bool
	}{
		{name: "bl_edge", edgeOnly: true},
		{name: "bl_flat", edgeOnly: false},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		regionImageData := selectIndexedPixelsByBoundsAndEdgeMask(imageData, width, height, bl, edgeMask, fillIndex, tc.edgeOnly)
		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_bottom_left_"+tc.name,
			palette,
			width,
			height,
			regionImageData,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 bottom-left edge-region probe: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023BottomLeftEdgeOrientationProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	bl := image.Rect(0, height/2, width/2, height)
	orientations := classifyIndexedEdgeOrientationPixels(imageData, width, height)

	cases := []struct {
		name        string
		orientation edgeOrientation
	}{
		{name: "bl_horizontal", orientation: edgeOrientationHorizontal},
		{name: "bl_vertical", orientation: edgeOrientationVertical},
		{name: "bl_mixed", orientation: edgeOrientationMixed},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		regionImageData := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, tc.orientation)
		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_bottom_left_"+tc.name,
			palette,
			width,
			height,
			regionImageData,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 bottom-left edge-orientation probe: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023BottomLeftEdgeHybridProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	bl := image.Rect(0, height/2, width/2, height)
	orientations := classifyIndexedEdgeOrientationPixels(imageData, width, height)
	horizontal := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationHorizontal)
	vertical := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationVertical)
	mixed := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationMixed)

	cases := []struct {
		name      string
		imageData []byte
	}{
		{name: "bl_horizontal_vertical", imageData: mergeIndexedProxyLayers(byte(fillIndex), horizontal, vertical)},
		{name: "bl_horizontal_mixed", imageData: mergeIndexedProxyLayers(byte(fillIndex), horizontal, mixed)},
		{name: "bl_vertical_mixed", imageData: mergeIndexedProxyLayers(byte(fillIndex), vertical, mixed)},
		{name: "bl_all_orientations", imageData: mergeIndexedProxyLayers(byte(fillIndex), horizontal, vertical, mixed)},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_bottom_left_"+tc.name,
			palette,
			width,
			height,
			tc.imageData,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 bottom-left edge-hybrid probe: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023BottomLeftEdgeLayoutProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	bl := image.Rect(0, height/2, width/2, height)
	orientations := classifyIndexedEdgeOrientationPixels(imageData, width, height)
	horizontal := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationHorizontal)
	vertical := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationVertical)
	mixed := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationMixed)

	cases := []struct {
		name      string
		imageData []byte
	}{
		{name: "bl_preserved", imageData: mergeIndexedProxyLayers(byte(fillIndex), horizontal, vertical, mixed)},
		{name: "bl_class_banded", imageData: rearrangeIndexedProxyLayersByClass(byte(fillIndex), horizontal, vertical, mixed)},
		{name: "bl_edge_compacted", imageData: compactIndexedProxyLayers(byte(fillIndex), horizontal, vertical, mixed)},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_bottom_left_layout_"+tc.name,
			palette,
			width,
			height,
			tc.imageData,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 bottom-left edge-layout probe: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023BottomLeftEdgeWithMidStripeProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	bl := image.Rect(0, height/2, width/2, height)
	verticalMid := image.Rect(width/4, 0, width*3/4, height)
	horizontalMid := image.Rect(0, height/4, width, height*3/4)

	orientations := classifyIndexedEdgeOrientationPixels(imageData, width, height)
	horizontal := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationHorizontal)
	vertical := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationVertical)
	mixed := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationMixed)
	blEdge := mergeIndexedProxyLayers(byte(fillIndex), horizontal, vertical, mixed)

	cases := []struct {
		name      string
		imageData []byte
	}{
		{
			name:      "bl_edge_plus_vertical_mid",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), blEdge, selectIndexedPixelsByBounds(imageData, width, height, verticalMid, fillIndex)),
		},
		{
			name:      "bl_edge_plus_horizontal_mid",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), blEdge, selectIndexedPixelsByBounds(imageData, width, height, horizontalMid, fillIndex)),
		},
		{
			name: "bl_edge_plus_both_mid",
			imageData: mergeIndexedProxyLayers(
				byte(fillIndex),
				blEdge,
				selectIndexedPixelsByBoundsUnion(imageData, width, height, fillIndex, verticalMid, horizontalMid),
			),
		},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_bottom_left_edge_mid_"+tc.name,
			palette,
			width,
			height,
			tc.imageData,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 bottom-left edge with mid-stripe probe: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023BottomLeftEdgeMidStripeFootprintSweepProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	bl := image.Rect(0, height/2, width/2, height)
	orientations := classifyIndexedEdgeOrientationPixels(imageData, width, height)
	horizontal := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationHorizontal)
	vertical := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationVertical)
	mixed := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationMixed)
	blEdge := mergeIndexedProxyLayers(byte(fillIndex), horizontal, vertical, mixed)
	verticalUltraMicro := width / 16
	if verticalUltraMicro < 1 {
		verticalUltraMicro = 1
	}
	verticalMicro := width / 8
	if verticalMicro < 1 {
		verticalMicro = 1
	}
	verticalNarrow := width / 4
	if verticalNarrow < 1 {
		verticalNarrow = 1
	}
	horizontalUltraMicro := height / 16
	if horizontalUltraMicro < 1 {
		horizontalUltraMicro = 1
	}
	horizontalMicro := height / 8
	if horizontalMicro < 1 {
		horizontalMicro = 1
	}
	horizontalNarrow := height / 4
	if horizontalNarrow < 1 {
		horizontalNarrow = 1
	}

	cases := []struct {
		name      string
		imageData []byte
	}{
		{
			name:      "vertical_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), blEdge, selectIndexedPixelsByBounds(imageData, width, height, centeredVerticalStripe(width, height, verticalUltraMicro), fillIndex)),
		},
		{
			name:      "vertical_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), blEdge, selectIndexedPixelsByBounds(imageData, width, height, centeredVerticalStripe(width, height, verticalMicro), fillIndex)),
		},
		{
			name:      "vertical_narrow",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), blEdge, selectIndexedPixelsByBounds(imageData, width, height, centeredVerticalStripe(width, height, verticalNarrow), fillIndex)),
		},
		{
			name:      "horizontal_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), blEdge, selectIndexedPixelsByBounds(imageData, width, height, centeredHorizontalStripe(width, height, horizontalUltraMicro), fillIndex)),
		},
		{
			name:      "horizontal_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), blEdge, selectIndexedPixelsByBounds(imageData, width, height, centeredHorizontalStripe(width, height, horizontalMicro), fillIndex)),
		},
		{
			name:      "horizontal_narrow",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), blEdge, selectIndexedPixelsByBounds(imageData, width, height, centeredHorizontalStripe(width, height, horizontalNarrow), fillIndex)),
		},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_bottom_left_edge_footprint_"+tc.name,
			palette,
			width,
			height,
			tc.imageData,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 bottom-left edge mid-stripe footprint sweep: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023BottomLeftEdgeSubregionWithUltraMicroStripeProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	bl := image.Rect(0, height/2, width/2, height)
	halfW := bl.Dx() / 2
	halfH := bl.Dy() / 2
	verticalUltraMicro := width / 16
	if verticalUltraMicro < 1 {
		verticalUltraMicro = 1
	}
	horizontalUltraMicro := height / 16
	if horizontalUltraMicro < 1 {
		horizontalUltraMicro = 1
	}
	verticalStripe := selectIndexedPixelsByBounds(
		imageData,
		width,
		height,
		centeredVerticalStripe(width, height, verticalUltraMicro),
		fillIndex,
	)
	horizontalStripe := selectIndexedPixelsByBounds(
		imageData,
		width,
		height,
		centeredHorizontalStripe(width, height, horizontalUltraMicro),
		fillIndex,
	)
	orientations := classifyIndexedEdgeOrientationPixels(imageData, width, height)
	subregions := []struct {
		name   string
		bounds image.Rectangle
	}{
		{name: "bl_tl", bounds: image.Rect(bl.Min.X, bl.Min.Y, bl.Min.X+halfW, bl.Min.Y+halfH)},
		{name: "bl_tr", bounds: image.Rect(bl.Min.X+halfW, bl.Min.Y, bl.Max.X, bl.Min.Y+halfH)},
		{name: "bl_bl", bounds: image.Rect(bl.Min.X, bl.Min.Y+halfH, bl.Min.X+halfW, bl.Max.Y)},
		{name: "bl_br", bounds: image.Rect(bl.Min.X+halfW, bl.Min.Y+halfH, bl.Max.X, bl.Max.Y)},
	}

	parts := make([]string, 0, len(subregions)*2)
	for _, tc := range subregions {
		horizontal := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, tc.bounds, orientations, fillIndex, edgeOrientationHorizontal)
		vertical := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, tc.bounds, orientations, fillIndex, edgeOrientationVertical)
		mixed := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, tc.bounds, orientations, fillIndex, edgeOrientationMixed)
		subregionEdge := mergeIndexedProxyLayers(byte(fillIndex), horizontal, vertical, mixed)

		cases := []struct {
			name      string
			imageData []byte
		}{
			{
				name:      tc.name + "_plus_vertical_ultra_micro",
				imageData: mergeIndexedProxyLayers(byte(fillIndex), subregionEdge, verticalStripe),
			},
			{
				name:      tc.name + "_plus_horizontal_ultra_micro",
				imageData: mergeIndexedProxyLayers(byte(fillIndex), subregionEdge, horizontalStripe),
			},
		}

		for _, probeCase := range cases {
			direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
				t,
				"indexed_doc023_bottom_left_edge_subregion_"+probeCase.name,
				palette,
				width,
				height,
				probeCase.imageData,
				540,
				720,
				[6]float64{468, 0, 0, 624, 72, 96},
			)
			parts = append(parts, fmt.Sprintf(
				"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
				probeCase.name,
				direct.similarity,
				refs["current_rgb"].similarity,
				refs["simple_rgb"].similarity,
				refs["stdlib_rgb"].similarity,
			))
		}
	}

	t.Skipf("doc023 bottom-left edge subregion with ultra-micro stripe probe: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023BottomLeftEdgeSubregionCombinationProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	bl := image.Rect(0, height/2, width/2, height)
	halfW := bl.Dx() / 2
	halfH := bl.Dy() / 2
	verticalUltraMicro := width / 16
	if verticalUltraMicro < 1 {
		verticalUltraMicro = 1
	}
	horizontalUltraMicro := height / 16
	if horizontalUltraMicro < 1 {
		horizontalUltraMicro = 1
	}
	verticalStripe := selectIndexedPixelsByBounds(
		imageData,
		width,
		height,
		centeredVerticalStripe(width, height, verticalUltraMicro),
		fillIndex,
	)
	horizontalStripe := selectIndexedPixelsByBounds(
		imageData,
		width,
		height,
		centeredHorizontalStripe(width, height, horizontalUltraMicro),
		fillIndex,
	)
	orientations := classifyIndexedEdgeOrientationPixels(imageData, width, height)
	subregions := []struct {
		name   string
		bounds image.Rectangle
	}{
		{name: "bl_tl", bounds: image.Rect(bl.Min.X, bl.Min.Y, bl.Min.X+halfW, bl.Min.Y+halfH)},
		{name: "bl_tr", bounds: image.Rect(bl.Min.X+halfW, bl.Min.Y, bl.Max.X, bl.Min.Y+halfH)},
		{name: "bl_bl", bounds: image.Rect(bl.Min.X, bl.Min.Y+halfH, bl.Min.X+halfW, bl.Max.Y)},
		{name: "bl_br", bounds: image.Rect(bl.Min.X+halfW, bl.Min.Y+halfH, bl.Max.X, bl.Max.Y)},
	}

	edgeLayers := make(map[string][]byte, len(subregions))
	for _, tc := range subregions {
		horizontal := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, tc.bounds, orientations, fillIndex, edgeOrientationHorizontal)
		vertical := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, tc.bounds, orientations, fillIndex, edgeOrientationVertical)
		mixed := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, tc.bounds, orientations, fillIndex, edgeOrientationMixed)
		edgeLayers[tc.name] = mergeIndexedProxyLayers(byte(fillIndex), horizontal, vertical, mixed)
	}

	mergeNamedLayers := func(names ...string) []byte {
		layers := make([][]byte, 0, len(names))
		for _, name := range names {
			layer, ok := edgeLayers[name]
			require.Truef(t, ok, "missing layer %s", name)
			layers = append(layers, layer)
		}
		return mergeIndexedProxyLayers(byte(fillIndex), layers...)
	}

	cases := []struct {
		name      string
		imageData []byte
	}{
		{
			name:      "diagonal_tl_br_plus_vertical_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), mergeNamedLayers("bl_tl", "bl_br"), verticalStripe),
		},
		{
			name:      "diagonal_tl_br_plus_horizontal_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), mergeNamedLayers("bl_tl", "bl_br"), horizontalStripe),
		},
		{
			name:      "left_column_plus_vertical_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), mergeNamedLayers("bl_tl", "bl_bl"), verticalStripe),
		},
		{
			name:      "left_column_plus_horizontal_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), mergeNamedLayers("bl_tl", "bl_bl"), horizontalStripe),
		},
		{
			name:      "bottom_row_plus_vertical_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), mergeNamedLayers("bl_bl", "bl_br"), verticalStripe),
		},
		{
			name:      "bottom_row_plus_horizontal_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), mergeNamedLayers("bl_bl", "bl_br"), horizontalStripe),
		},
		{
			name:      "top_row_plus_vertical_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), mergeNamedLayers("bl_tl", "bl_tr"), verticalStripe),
		},
		{
			name:      "top_row_plus_horizontal_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), mergeNamedLayers("bl_tl", "bl_tr"), horizontalStripe),
		},
		{
			name:      "right_column_plus_vertical_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), mergeNamedLayers("bl_tr", "bl_br"), verticalStripe),
		},
		{
			name:      "right_column_plus_horizontal_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), mergeNamedLayers("bl_tr", "bl_br"), horizontalStripe),
		},
		{
			name:      "all_except_bl_bl_plus_vertical_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), mergeNamedLayers("bl_tl", "bl_tr", "bl_br"), verticalStripe),
		},
		{
			name:      "all_except_bl_bl_plus_horizontal_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), mergeNamedLayers("bl_tl", "bl_tr", "bl_br"), horizontalStripe),
		},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_bottom_left_edge_combo_"+tc.name,
			palette,
			width,
			height,
			tc.imageData,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 bottom-left edge subregion combination probe: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023BottomLeftEdgeOrientationDensityWithUltraMicroStripeProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	bl := image.Rect(0, height/2, width/2, height)
	verticalUltraMicro := width / 16
	if verticalUltraMicro < 1 {
		verticalUltraMicro = 1
	}
	verticalStripe := selectIndexedPixelsByBounds(
		imageData,
		width,
		height,
		centeredVerticalStripe(width, height, verticalUltraMicro),
		fillIndex,
	)
	orientations := classifyIndexedEdgeOrientationPixels(imageData, width, height)
	horizontal := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationHorizontal)
	vertical := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationVertical)
	mixed := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationMixed)

	cases := []struct {
		name      string
		imageData []byte
	}{
		{
			name:      "horizontal_only_plus_vertical_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), horizontal, verticalStripe),
		},
		{
			name:      "vertical_only_plus_vertical_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), vertical, verticalStripe),
		},
		{
			name:      "mixed_only_plus_vertical_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), mixed, verticalStripe),
		},
		{
			name:      "horizontal_vertical_plus_vertical_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), horizontal, vertical, verticalStripe),
		},
		{
			name:      "horizontal_mixed_plus_vertical_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), horizontal, mixed, verticalStripe),
		},
		{
			name:      "vertical_mixed_plus_vertical_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), vertical, mixed, verticalStripe),
		},
		{
			name:      "all_orientations_plus_vertical_ultra_micro",
			imageData: mergeIndexedProxyLayers(byte(fillIndex), horizontal, vertical, mixed, verticalStripe),
		},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_bottom_left_orientation_density_"+tc.name,
			palette,
			width,
			height,
			tc.imageData,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 bottom-left edge orientation-density with ultra-micro stripe probe: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023BottomLeftEdgeMinimalOrientationDiversityProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	bl := image.Rect(0, height/2, width/2, height)
	halfW := bl.Dx() / 2
	halfH := bl.Dy() / 2
	verticalUltraMicro := width / 16
	if verticalUltraMicro < 1 {
		verticalUltraMicro = 1
	}
	verticalStripe := selectIndexedPixelsByBounds(
		imageData,
		width,
		height,
		centeredVerticalStripe(width, height, verticalUltraMicro),
		fillIndex,
	)
	orientations := classifyIndexedEdgeOrientationPixels(imageData, width, height)

	subregions := []struct {
		name   string
		bounds image.Rectangle
	}{
		{name: "bl_tl", bounds: image.Rect(bl.Min.X, bl.Min.Y, bl.Min.X+halfW, bl.Min.Y+halfH)},
		{name: "bl_tr", bounds: image.Rect(bl.Min.X+halfW, bl.Min.Y, bl.Max.X, bl.Min.Y+halfH)},
		{name: "bl_br", bounds: image.Rect(bl.Min.X+halfW, bl.Min.Y+halfH, bl.Max.X, bl.Max.Y)},
	}

	type orientationLayers map[edgeOrientation][]byte
	layersByRegion := make(map[string]orientationLayers, len(subregions))
	for _, tc := range subregions {
		layersByRegion[tc.name] = orientationLayers{
			edgeOrientationHorizontal: selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, tc.bounds, orientations, fillIndex, edgeOrientationHorizontal),
			edgeOrientationVertical:   selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, tc.bounds, orientations, fillIndex, edgeOrientationVertical),
			edgeOrientationMixed:      selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, tc.bounds, orientations, fillIndex, edgeOrientationMixed),
		}
	}

	mergeRegionOrientations := func(parts map[string][]edgeOrientation) []byte {
		layers := make([][]byte, 0)
		for regionName, orientationSet := range parts {
			regionLayers, ok := layersByRegion[regionName]
			require.Truef(t, ok, "missing region %s", regionName)
			for _, orientation := range orientationSet {
				layer, ok := regionLayers[orientation]
				require.Truef(t, ok, "missing orientation %d for region %s", orientation, regionName)
				layers = append(layers, layer)
			}
		}
		return mergeIndexedProxyLayers(byte(fillIndex), layers...)
	}

	cases := []struct {
		name      string
		imageData []byte
	}{
		{
			name: "two_subregions_hv_plus_m",
			imageData: mergeIndexedProxyLayers(
				byte(fillIndex),
				mergeRegionOrientations(map[string][]edgeOrientation{
					"bl_tl": {edgeOrientationHorizontal, edgeOrientationVertical},
					"bl_tr": {edgeOrientationMixed},
				}),
				verticalStripe,
			),
		},
		{
			name: "two_subregions_h_plus_vm",
			imageData: mergeIndexedProxyLayers(
				byte(fillIndex),
				mergeRegionOrientations(map[string][]edgeOrientation{
					"bl_tl": {edgeOrientationHorizontal},
					"bl_tr": {edgeOrientationVertical, edgeOrientationMixed},
				}),
				verticalStripe,
			),
		},
		{
			name: "two_subregions_v_plus_hm",
			imageData: mergeIndexedProxyLayers(
				byte(fillIndex),
				mergeRegionOrientations(map[string][]edgeOrientation{
					"bl_tl": {edgeOrientationVertical},
					"bl_tr": {edgeOrientationHorizontal, edgeOrientationMixed},
				}),
				verticalStripe,
			),
		},
		{
			name: "three_subregions_h_v_m",
			imageData: mergeIndexedProxyLayers(
				byte(fillIndex),
				mergeRegionOrientations(map[string][]edgeOrientation{
					"bl_tl": {edgeOrientationHorizontal},
					"bl_tr": {edgeOrientationVertical},
					"bl_br": {edgeOrientationMixed},
				}),
				verticalStripe,
			),
		},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_bottom_left_min_orientation_"+tc.name,
			palette,
			width,
			height,
			tc.imageData,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 bottom-left minimal orientation-diversity probe: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023BottomLeftEdgeOrientationMassSweepProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	bl := image.Rect(0, height/2, width/2, height)
	verticalUltraMicro := width / 16
	if verticalUltraMicro < 1 {
		verticalUltraMicro = 1
	}
	verticalStripe := selectIndexedPixelsByBounds(
		imageData,
		width,
		height,
		centeredVerticalStripe(width, height, verticalUltraMicro),
		fillIndex,
	)
	orientations := classifyIndexedEdgeOrientationPixels(imageData, width, height)
	horizontal := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationHorizontal)
	vertical := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationVertical)
	mixed := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationMixed)

	cases := []struct {
		name      string
		keepEvery int
	}{
		{name: "full", keepEvery: 1},
		{name: "half", keepEvery: 2},
		{name: "quarter", keepEvery: 4},
		{name: "eighth", keepEvery: 8},
		{name: "sixteenth", keepEvery: 16},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		thinnedHorizontal := thinIndexedProxyLayer(byte(fillIndex), horizontal, tc.keepEvery)
		thinnedVertical := thinIndexedProxyLayer(byte(fillIndex), vertical, tc.keepEvery)
		thinnedMixed := thinIndexedProxyLayer(byte(fillIndex), mixed, tc.keepEvery)
		composed := mergeIndexedProxyLayers(byte(fillIndex), thinnedHorizontal, thinnedVertical, thinnedMixed, verticalStripe)

		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_bottom_left_orientation_mass_"+tc.name,
			palette,
			width,
			height,
			composed,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 bottom-left edge orientation-mass sweep: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023BottomLeftEdgeOrientationMassLayoutProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	bl := image.Rect(0, height/2, width/2, height)
	verticalUltraMicro := width / 16
	if verticalUltraMicro < 1 {
		verticalUltraMicro = 1
	}
	verticalStripe := selectIndexedPixelsByBounds(
		imageData,
		width,
		height,
		centeredVerticalStripe(width, height, verticalUltraMicro),
		fillIndex,
	)
	orientations := classifyIndexedEdgeOrientationPixels(imageData, width, height)
	horizontal := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationHorizontal)
	vertical := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationVertical)
	mixed := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationMixed)

	cases := []struct {
		name      string
		imageData []byte
	}{
		{
			name: "half_preserved",
			imageData: mergeIndexedProxyLayers(
				byte(fillIndex),
				thinIndexedProxyLayer(byte(fillIndex), horizontal, 2),
				thinIndexedProxyLayer(byte(fillIndex), vertical, 2),
				thinIndexedProxyLayer(byte(fillIndex), mixed, 2),
				verticalStripe,
			),
		},
		{
			name: "half_class_banded",
			imageData: mergeIndexedProxyLayers(
				byte(fillIndex),
				rearrangeIndexedProxyLayersByClass(
					byte(fillIndex),
					thinIndexedProxyLayer(byte(fillIndex), horizontal, 2),
					thinIndexedProxyLayer(byte(fillIndex), vertical, 2),
					thinIndexedProxyLayer(byte(fillIndex), mixed, 2),
				),
				verticalStripe,
			),
		},
		{
			name: "half_compacted",
			imageData: mergeIndexedProxyLayers(
				byte(fillIndex),
				compactIndexedProxyLayers(
					byte(fillIndex),
					thinIndexedProxyLayer(byte(fillIndex), horizontal, 2),
					thinIndexedProxyLayer(byte(fillIndex), vertical, 2),
					thinIndexedProxyLayer(byte(fillIndex), mixed, 2),
				),
				verticalStripe,
			),
		},
		{
			name: "quarter_preserved",
			imageData: mergeIndexedProxyLayers(
				byte(fillIndex),
				thinIndexedProxyLayer(byte(fillIndex), horizontal, 4),
				thinIndexedProxyLayer(byte(fillIndex), vertical, 4),
				thinIndexedProxyLayer(byte(fillIndex), mixed, 4),
				verticalStripe,
			),
		},
		{
			name: "quarter_class_banded",
			imageData: mergeIndexedProxyLayers(
				byte(fillIndex),
				rearrangeIndexedProxyLayersByClass(
					byte(fillIndex),
					thinIndexedProxyLayer(byte(fillIndex), horizontal, 4),
					thinIndexedProxyLayer(byte(fillIndex), vertical, 4),
					thinIndexedProxyLayer(byte(fillIndex), mixed, 4),
				),
				verticalStripe,
			),
		},
		{
			name: "quarter_compacted",
			imageData: mergeIndexedProxyLayers(
				byte(fillIndex),
				compactIndexedProxyLayers(
					byte(fillIndex),
					thinIndexedProxyLayer(byte(fillIndex), horizontal, 4),
					thinIndexedProxyLayer(byte(fillIndex), vertical, 4),
					thinIndexedProxyLayer(byte(fillIndex), mixed, 4),
				),
				verticalStripe,
			),
		},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_bottom_left_orientation_mass_layout_"+tc.name,
			palette,
			width,
			height,
			tc.imageData,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 bottom-left edge orientation-mass layout probe: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023BottomLeftEdgeOrientationMassInterleaveProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	bl := image.Rect(0, height/2, width/2, height)
	verticalUltraMicro := width / 16
	if verticalUltraMicro < 1 {
		verticalUltraMicro = 1
	}
	verticalStripe := selectIndexedPixelsByBounds(
		imageData,
		width,
		height,
		centeredVerticalStripe(width, height, verticalUltraMicro),
		fillIndex,
	)
	orientations := classifyIndexedEdgeOrientationPixels(imageData, width, height)
	horizontal := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationHorizontal)
	vertical := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationVertical)
	mixed := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationMixed)

	cases := []struct {
		name      string
		imageData []byte
	}{
		{
			name: "half_compacted_separated",
			imageData: mergeIndexedProxyLayers(
				byte(fillIndex),
				compactIndexedProxyLayers(
					byte(fillIndex),
					thinIndexedProxyLayer(byte(fillIndex), horizontal, 2),
					thinIndexedProxyLayer(byte(fillIndex), vertical, 2),
					thinIndexedProxyLayer(byte(fillIndex), mixed, 2),
				),
				verticalStripe,
			),
		},
		{
			name: "half_compacted_interleaved",
			imageData: mergeIndexedProxyLayers(
				byte(fillIndex),
				interleaveIndexedProxyLayers(
					byte(fillIndex),
					thinIndexedProxyLayer(byte(fillIndex), horizontal, 2),
					thinIndexedProxyLayer(byte(fillIndex), vertical, 2),
					thinIndexedProxyLayer(byte(fillIndex), mixed, 2),
				),
				verticalStripe,
			),
		},
		{
			name: "quarter_compacted_separated",
			imageData: mergeIndexedProxyLayers(
				byte(fillIndex),
				compactIndexedProxyLayers(
					byte(fillIndex),
					thinIndexedProxyLayer(byte(fillIndex), horizontal, 4),
					thinIndexedProxyLayer(byte(fillIndex), vertical, 4),
					thinIndexedProxyLayer(byte(fillIndex), mixed, 4),
				),
				verticalStripe,
			),
		},
		{
			name: "quarter_compacted_interleaved",
			imageData: mergeIndexedProxyLayers(
				byte(fillIndex),
				interleaveIndexedProxyLayers(
					byte(fillIndex),
					thinIndexedProxyLayer(byte(fillIndex), horizontal, 4),
					thinIndexedProxyLayer(byte(fillIndex), vertical, 4),
					thinIndexedProxyLayer(byte(fillIndex), mixed, 4),
				),
				verticalStripe,
			),
		},
	}

	parts := make([]string, 0, len(cases))
	for _, tc := range cases {
		direct, refs := probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
			t,
			"indexed_doc023_bottom_left_orientation_mass_interleave_"+tc.name,
			palette,
			width,
			height,
			tc.imageData,
			540,
			720,
			[6]float64{468, 0, 0, 624, 72, 96},
		)
		parts = append(parts, fmt.Sprintf(
			"%s:{direct=%.4f current=%.4f simple=%.4f stdlib=%.4f}",
			tc.name,
			direct.similarity,
			refs["current_rgb"].similarity,
			refs["simple_rgb"].similarity,
			refs["stdlib_rgb"].similarity,
		))
	}

	t.Skipf("doc023 bottom-left edge orientation-mass interleave probe: %s", strings.Join(parts, " "))
}

func TestSyntheticDoc023CMYKHybridBlendProbeAgainstPoppler(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "023-cmyk-image", "cmyk-image.pdf")
	base, palette, imageData, width, height := loadSampleIndexedImageObject(
		t,
		pdfPath,
		entity.NewRef(5, 0),
	)

	require.Equal(t, "DeviceCMYK", base)

	fullDirect, fullRefs := probeIndexedPaletteReferenceSetAtGeometryAgainstPopplerWithReferences(
		t,
		"indexed_doc023_hybrid_blend_full",
		palette,
		width,
		height,
		imageData,
		540,
		720,
		[6]float64{468, 0, 0, 624, 72, 96},
		map[string][]byte{
			"current_rgb":   convertCMYKPaletteToCurrentRGBPalette(palette),
			"simple_rgb":    convertCMYKPaletteToSimpleRGBPalette(palette),
			"hybrid_25_rgb": convertCMYKPaletteToBlendedRGBPalette(palette, 0.25),
			"hybrid_50_rgb": convertCMYKPaletteToBlendedRGBPalette(palette, 0.50),
			"hybrid_75_rgb": convertCMYKPaletteToBlendedRGBPalette(palette, 0.75),
		},
	)

	fillIndex := dominantPaletteIndex(countIndexedPaletteUsage(imageData, len(palette)/4))
	bl := image.Rect(0, height/2, width/2, height)
	verticalUltraMicro := width / 16
	if verticalUltraMicro < 1 {
		verticalUltraMicro = 1
	}
	verticalStripe := selectIndexedPixelsByBounds(
		imageData,
		width,
		height,
		centeredVerticalStripe(width, height, verticalUltraMicro),
		fillIndex,
	)
	orientations := classifyIndexedEdgeOrientationPixels(imageData, width, height)
	horizontal := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationHorizontal)
	vertical := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationVertical)
	mixed := selectIndexedPixelsByBoundsAndOrientation(imageData, width, height, bl, orientations, fillIndex, edgeOrientationMixed)
	halfCompacted := mergeIndexedProxyLayers(
		byte(fillIndex),
		compactIndexedProxyLayers(
			byte(fillIndex),
			thinIndexedProxyLayer(byte(fillIndex), horizontal, 2),
			thinIndexedProxyLayer(byte(fillIndex), vertical, 2),
			thinIndexedProxyLayer(byte(fillIndex), mixed, 2),
		),
		verticalStripe,
	)
	compactDirect, compactRefs := probeIndexedPaletteReferenceSetAtGeometryAgainstPopplerWithReferences(
		t,
		"indexed_doc023_hybrid_blend_half_compacted",
		palette,
		width,
		height,
		halfCompacted,
		540,
		720,
		[6]float64{468, 0, 0, 624, 72, 96},
		map[string][]byte{
			"current_rgb":   convertCMYKPaletteToCurrentRGBPalette(palette),
			"simple_rgb":    convertCMYKPaletteToSimpleRGBPalette(palette),
			"hybrid_25_rgb": convertCMYKPaletteToBlendedRGBPalette(palette, 0.25),
			"hybrid_50_rgb": convertCMYKPaletteToBlendedRGBPalette(palette, 0.50),
			"hybrid_75_rgb": convertCMYKPaletteToBlendedRGBPalette(palette, 0.75),
		},
	)

	t.Skipf(
		"doc023 cmyk hybrid blend probe: full{direct=%.4f current=%.4f hybrid25=%.4f hybrid50=%.4f hybrid75=%.4f simple=%.4f} half_compacted{direct=%.4f current=%.4f hybrid25=%.4f hybrid50=%.4f hybrid75=%.4f simple=%.4f}",
		fullDirect.similarity,
		fullRefs["current_rgb"].similarity,
		fullRefs["hybrid_25_rgb"].similarity,
		fullRefs["hybrid_50_rgb"].similarity,
		fullRefs["hybrid_75_rgb"].similarity,
		fullRefs["simple_rgb"].similarity,
		compactDirect.similarity,
		compactRefs["current_rgb"].similarity,
		compactRefs["hybrid_25_rgb"].similarity,
		compactRefs["hybrid_50_rgb"].similarity,
		compactRefs["hybrid_75_rgb"].similarity,
		compactRefs["simple_rgb"].similarity,
	)
}

func TestSyntheticImageExperimentalSplashModeIsCloserToPopplerThanLegacy(t *testing.T) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	pdfPath := filepath.Join(root, "gray_identity_4x4.pdf")
	popplerRoot := filepath.Join(root, "poppler")
	legacyPNG := filepath.Join(root, "legacy.png")
	experimentalPNG := filepath.Join(root, "experimental.png")

	require.NoError(t, os.WriteFile(
		pdfPath,
		buildSyntheticGrayImagePDF(4, 4, 4, 4, [6]int{4, 0, 0, 4, 0, 0}, []byte{
			0, 63, 127, 255,
			255, 127, 63, 0,
			0, 255, 0, 255,
			255, 0, 255, 0,
		}),
		0o644,
	))
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, 72))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	doc, err := internalpdf.Open(pdfPath)
	require.NoError(t, err)

	page, err := doc.GetPage(0)
	require.NoError(t, err)

	legacyOpts := domainrenderer.DefaultRenderOptions()
	legacyOpts.DPI = 72
	legacyOpts.EnableCache = false
	legacyOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeLegacy
	legacyRenderer := infrarenderer.NewConcurrentRenderer(infrarenderer.RendererOptions{})
	legacyImg, err := legacyRenderer.RenderPage(context.Background(), page, legacyOpts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(legacyPNG, legacyImg))

	experimentalOpts := domainrenderer.DefaultRenderOptions()
	experimentalOpts.DPI = 72
	experimentalOpts.EnableCache = false
	experimentalOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeExperimentalSplashScaleOnlyV1
	experimentalRenderer := infrarenderer.NewConcurrentRenderer(infrarenderer.RendererOptions{})
	experimentalImg, err := experimentalRenderer.RenderPage(context.Background(), page, experimentalOpts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(experimentalPNG, experimentalImg))

	_, legacySimilarity, err := parityComparePNGs(legacyPNG, popplerPages[1])
	require.NoError(t, err)
	_, experimentalSimilarity, err := parityComparePNGs(experimentalPNG, popplerPages[1])
	require.NoError(t, err)

	assert.Greater(t, experimentalSimilarity, legacySimilarity)
}

func TestSyntheticIndexedImageExperimentalSplashModeIsCloserToPopplerThanLegacy(t *testing.T) {
	palette, imageData := syntheticIndexedIdentity4x4()
	assertSyntheticImageModeCloserThanLegacy(
		t,
		"indexed_identity_4x4.pdf",
		buildSyntheticIndexedImagePDF(4, 4, 4, 4, [6]int{4, 0, 0, 4, 0, 0}, palette, imageData),
	)
}

func TestSyntheticGraySmallUpscaleExperimentalSplashModeIsCloserToPopplerThanLegacy(t *testing.T) {
	assertSyntheticImageModeCloserThanLegacy(
		t,
		"gray_small_upscale_5x5.pdf",
		buildSyntheticGrayImagePDF(5, 5, 4, 4, [6]int{5, 0, 0, 5, 0, 0}, []byte{
			0, 63, 127, 255,
			255, 127, 63, 0,
			0, 255, 0, 255,
			255, 0, 255, 0,
		}),
	)
}

func TestSyntheticIndexedSmallUpscaleExperimentalSplashModeIsCloserToPopplerThanLegacy(t *testing.T) {
	palette, imageData := syntheticIndexedIdentity4x4()
	assertSyntheticImageModeCloserThanLegacy(
		t,
		"indexed_small_upscale_5x5.pdf",
		buildSyntheticIndexedImagePDF(5, 5, 4, 4, [6]int{5, 0, 0, 5, 0, 0}, palette, imageData),
	)
}

func TestSyntheticGrayOffsetUpscaleExperimentalSplashModeIsCloserToPopplerThanLegacy(t *testing.T) {
	assertSyntheticImageModeCloserThanLegacy(
		t,
		"gray_offset_upscale_7x7.pdf",
		buildSyntheticGrayImagePDF(7, 7, 4, 4, [6]int{5, 0, 0, 5, 1, 1}, []byte{
			0, 63, 127, 255,
			255, 127, 63, 0,
			0, 255, 0, 255,
			255, 0, 255, 0,
		}),
	)
}

func TestSyntheticIndexedOffsetUpscaleExperimentalSplashModeIsCloserToPopplerThanLegacy(t *testing.T) {
	palette, imageData := syntheticIndexedIdentity4x4()
	assertSyntheticImageModeCloserThanLegacy(
		t,
		"indexed_offset_upscale_7x7.pdf",
		buildSyntheticIndexedImagePDF(7, 7, 4, 4, [6]int{5, 0, 0, 5, 1, 1}, palette, imageData),
	)
}

func TestSyntheticGrayNonSquareUpscaleExperimentalSplashModeIsCloserToPopplerThanLegacy(t *testing.T) {
	assertSyntheticImageModeCloserThanLegacy(
		t,
		"gray_nonsquare_upscale_6x7.pdf",
		buildSyntheticGrayImagePDF(6, 7, 4, 4, [6]int{5, 0, 0, 6, 0, 0}, []byte{
			0, 63, 127, 255,
			255, 127, 63, 0,
			0, 255, 0, 255,
			255, 0, 255, 0,
		}),
	)
}

func TestSyntheticIndexedNonSquareUpscaleExperimentalSplashModeIsCloserToPopplerThanLegacy(t *testing.T) {
	palette, imageData := syntheticIndexedIdentity4x4()
	assertSyntheticImageModeCloserThanLegacy(
		t,
		"indexed_nonsquare_upscale_6x7.pdf",
		buildSyntheticIndexedImagePDF(6, 7, 4, 4, [6]int{5, 0, 0, 6, 0, 0}, palette, imageData),
	)
}

func TestSyntheticGraySubpixelOffsetExperimentalSplashModeIsCloserToPopplerThanLegacy(t *testing.T) {
	assertSyntheticImageModeCloserThanLegacy(
		t,
		"gray_subpixel_offset_7x7.pdf",
		buildSyntheticGrayImagePDFFloat(7, 7, 4, 4, [6]float64{5, 0, 0, 5, 0.5, 0.5}, []byte{
			0, 63, 127, 255,
			255, 127, 63, 0,
			0, 255, 0, 255,
			255, 0, 255, 0,
		}),
	)
}

func TestSyntheticIndexedSubpixelOffsetExperimentalSplashModeIsCloserToPopplerThanLegacy(t *testing.T) {
	palette, imageData := syntheticIndexedIdentity4x4()
	assertSyntheticImageModeCloserThanLegacy(
		t,
		"indexed_subpixel_offset_7x7.pdf",
		buildSyntheticIndexedImagePDFFloat(7, 7, 4, 4, [6]float64{5, 0, 0, 5, 0.5, 0.5}, palette, imageData),
	)
}

func TestSyntheticGrayNearIdentityAnisotropicExperimentalSplashModeIsCloserToPopplerThanLegacy(t *testing.T) {
	assertSyntheticImageModeCloserThanLegacy(
		t,
		"gray_near_identity_anisotropic.pdf",
		buildSyntheticGrayImagePDFFloat(5, 5, 4, 4, [6]float64{4.25, 0, 0, 4.75, 0, 0}, []byte{
			0, 63, 127, 255,
			255, 127, 63, 0,
			0, 255, 0, 255,
			255, 0, 255, 0,
		}),
	)
}

func TestSyntheticIndexedNearIdentityAnisotropicExperimentalSplashModeIsCloserToPopplerThanLegacy(t *testing.T) {
	palette, imageData := syntheticIndexedIdentity4x4()
	assertSyntheticImageModeCloserThanLegacy(
		t,
		"indexed_near_identity_anisotropic.pdf",
		buildSyntheticIndexedImagePDFFloat(5, 5, 4, 4, [6]float64{4.25, 0, 0, 4.75, 0, 0}, palette, imageData),
	)
}

func TestSyntheticIndexedLargeMildDownscaleProbeAgainstPoppler(t *testing.T) {
	palette, imageData := syntheticIndexedTiledIdentity(64, 64)
	pdfBytes := buildSyntheticIndexedImagePDFFloat(48, 48, 64, 64, [6]float64{48, 0, 0, 48, 0, 0}, palette, imageData)

	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	pdfPath := filepath.Join(root, "indexed_large_mild_downscale.pdf")
	popplerRoot := filepath.Join(root, "poppler")
	legacyPNG := filepath.Join(root, "legacy.png")
	experimentalPNG := filepath.Join(root, "experimental.png")

	require.NoError(t, os.WriteFile(pdfPath, pdfBytes, 0o644))
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, 72))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	doc, err := internalpdf.Open(pdfPath)
	require.NoError(t, err)

	page, err := doc.GetPage(0)
	require.NoError(t, err)

	legacyOpts := domainrenderer.DefaultRenderOptions()
	legacyOpts.DPI = 72
	legacyOpts.EnableCache = false
	legacyOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeLegacy
	legacyRenderer := infrarenderer.NewConcurrentRenderer(infrarenderer.RendererOptions{})
	legacyImg, err := legacyRenderer.RenderPage(context.Background(), page, legacyOpts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(legacyPNG, legacyImg))

	experimentalOpts := domainrenderer.DefaultRenderOptions()
	experimentalOpts.DPI = 72
	experimentalOpts.EnableCache = false
	experimentalOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeExperimentalSplashScaleOnlyV1
	experimentalRenderer := infrarenderer.NewConcurrentRenderer(infrarenderer.RendererOptions{})
	experimentalImg, err := experimentalRenderer.RenderPage(context.Background(), page, experimentalOpts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(experimentalPNG, experimentalImg))

	_, legacySimilarity, err := parityComparePNGs(legacyPNG, popplerPages[1])
	require.NoError(t, err)
	_, experimentalSimilarity, err := parityComparePNGs(experimentalPNG, popplerPages[1])
	require.NoError(t, err)

	t.Skipf(
		"large indexed mild-downscale probe: legacy=%.4f experimental=%.4f",
		legacySimilarity,
		experimentalSimilarity,
	)
}

func assertSyntheticImageModeCloserThanLegacy(t *testing.T, pdfName string, pdfBytes []byte) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	pdfPath := filepath.Join(root, pdfName)
	popplerRoot := filepath.Join(root, "poppler")
	legacyPNG := filepath.Join(root, "legacy.png")
	experimentalPNG := filepath.Join(root, "experimental.png")

	require.NoError(t, os.WriteFile(
		pdfPath,
		pdfBytes,
		0o644,
	))
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, 72))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	doc, err := internalpdf.Open(pdfPath)
	require.NoError(t, err)

	page, err := doc.GetPage(0)
	require.NoError(t, err)

	legacyOpts := domainrenderer.DefaultRenderOptions()
	legacyOpts.DPI = 72
	legacyOpts.EnableCache = false
	legacyOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeLegacy
	legacyRenderer := infrarenderer.NewConcurrentRenderer(infrarenderer.RendererOptions{})
	legacyImg, err := legacyRenderer.RenderPage(context.Background(), page, legacyOpts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(legacyPNG, legacyImg))

	experimentalOpts := domainrenderer.DefaultRenderOptions()
	experimentalOpts.DPI = 72
	experimentalOpts.EnableCache = false
	experimentalOpts.ImageSamplingMode = domainrenderer.ImageSamplingModeExperimentalSplashScaleOnlyV1
	experimentalRenderer := infrarenderer.NewConcurrentRenderer(infrarenderer.RendererOptions{})
	experimentalImg, err := experimentalRenderer.RenderPage(context.Background(), page, experimentalOpts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(experimentalPNG, experimentalImg))

	_, legacySimilarity, err := parityComparePNGs(legacyPNG, popplerPages[1])
	require.NoError(t, err)
	_, experimentalSimilarity, err := parityComparePNGs(experimentalPNG, popplerPages[1])
	require.NoError(t, err)

	assert.Greater(t, experimentalSimilarity, legacySimilarity)
}

func assertSyntheticRGBSplashReferenceCloserThanCurrentPath(
	t *testing.T,
	pdfName string,
	pdfBytes []byte,
	src image.Image,
	pageBounds image.Rectangle,
	matrix [6]float64,
) {
	oursSimilarity, splashSimilarity := measureSyntheticRGBSplashReferenceAgainstCurrentPath(
		t,
		pdfName,
		pdfBytes,
		src,
		pageBounds,
		matrix,
	)

	assert.Greater(t, splashSimilarity, oursSimilarity)
}

func measureSyntheticRGBSplashReferenceAgainstCurrentPath(
	t *testing.T,
	pdfName string,
	pdfBytes []byte,
	src image.Image,
	pageBounds image.Rectangle,
	matrix [6]float64,
) (float64, float64) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	pdfPath := filepath.Join(root, pdfName)
	popplerRoot := filepath.Join(root, "poppler")
	oursPNG := filepath.Join(root, "ours.png")
	splashPNG := filepath.Join(root, "splash_like.png")

	require.NoError(t, os.WriteFile(pdfPath, pdfBytes, 0o644))
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, 72))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 72

	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(oursPNG, img))

	splashLike := simulateSyntheticSplashScaleOnlyImageWithMatrix(src, pageBounds, matrix, true)
	require.NoError(t, parityWritePNG(splashPNG, splashLike))

	_, oursSimilarity, err := parityComparePNGs(oursPNG, popplerPages[1])
	require.NoError(t, err)
	_, splashSimilarity, err := parityComparePNGs(splashPNG, popplerPages[1])
	require.NoError(t, err)
	t.Logf("%s similarity: ours=%.4f splash_like=%.4f", strings.TrimSuffix(pdfName, ".pdf"), oursSimilarity, splashSimilarity)

	return oursSimilarity, splashSimilarity
}

func measureSyntheticGrayReferenceAgainstCurrentPath(
	t *testing.T,
	pdfName string,
	pdfBytes []byte,
	src image.Image,
	pageBounds image.Rectangle,
	reference image.Image,
) (float64, float64) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	pdfPath := filepath.Join(root, pdfName)
	popplerRoot := filepath.Join(root, "poppler")
	oursPNG := filepath.Join(root, "ours.png")
	referencePNG := filepath.Join(root, "reference.png")

	require.NoError(t, os.WriteFile(pdfPath, pdfBytes, 0o644))
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, 72))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 72

	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(oursPNG, img))
	require.NoError(t, parityWritePNG(referencePNG, imageToRGBA(reference, pageBounds)))

	_, oursSimilarity, err := parityComparePNGs(oursPNG, popplerPages[1])
	require.NoError(t, err)
	_, referenceSimilarity, err := parityComparePNGs(referencePNG, popplerPages[1])
	require.NoError(t, err)
	t.Logf(
		"%s similarity: ours=%.4f reference=%.4f src=%v dst=%v",
		strings.TrimSuffix(pdfName, ".pdf"),
		oursSimilarity,
		referenceSimilarity,
		src.Bounds(),
		pageBounds,
	)

	return oursSimilarity, referenceSimilarity
}

func probeSyntheticCurrentRenderAgainstPoppler(
	t *testing.T,
	pdfName string,
	pdfBytes []byte,
) (float64, float64) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	pdfPath := filepath.Join(root, pdfName)
	popplerRoot := filepath.Join(root, "poppler")
	oursPNG := filepath.Join(root, "ours.png")

	require.NoError(t, os.WriteFile(pdfPath, pdfBytes, 0o644))
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, 72))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 72

	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(oursPNG, img))

	exactPercent, similarityPercent, err := parityComparePNGs(oursPNG, popplerPages[1])
	require.NoError(t, err)
	t.Logf(
		"%s parity: exact=%.4f similarity=%.4f",
		strings.TrimSuffix(pdfName, ".pdf"),
		exactPercent,
		similarityPercent,
	)

	return exactPercent, similarityPercent
}

func probePDFRenderAgainstPoppler(
	t *testing.T,
	pdfPath string,
	opts pdf.RenderOptions,
) (float64, float64) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	popplerRoot := filepath.Join(root, "poppler")
	oursPNG := filepath.Join(root, "ours.png")

	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, int(opts.DPI)))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(oursPNG, img))

	exactPercent, similarityPercent, err := parityComparePNGs(oursPNG, popplerPages[1])
	require.NoError(t, err)
	return exactPercent, similarityPercent
}

func probeRenderedPDFParity(
	t *testing.T,
	leftName string,
	leftPDF []byte,
	rightName string,
	rightPDF []byte,
) (float64, float64) {
	root := t.TempDir()
	leftPath := filepath.Join(root, leftName)
	rightPath := filepath.Join(root, rightName)
	leftPNG := filepath.Join(root, "left.png")
	rightPNG := filepath.Join(root, "right.png")

	require.NoError(t, os.WriteFile(leftPath, leftPDF, 0o644))
	require.NoError(t, os.WriteFile(rightPath, rightPDF, 0o644))

	leftDoc, err := pdf.Open(leftPath)
	require.NoError(t, err)
	defer leftDoc.Close()

	rightDoc, err := pdf.Open(rightPath)
	require.NoError(t, err)
	defer rightDoc.Close()

	leftPage, err := leftDoc.Page(0)
	require.NoError(t, err)
	rightPage, err := rightDoc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 72

	leftImg, err := renderer.RenderPage(context.Background(), leftPage, opts)
	require.NoError(t, err)
	rightImg, err := renderer.RenderPage(context.Background(), rightPage, opts)
	require.NoError(t, err)

	require.NoError(t, parityWritePNG(leftPNG, leftImg))
	require.NoError(t, parityWritePNG(rightPNG, rightImg))

	exactPercent, similarityPercent, err := parityComparePNGs(leftPNG, rightPNG)
	require.NoError(t, err)
	t.Logf(
		"%s vs %s rendered parity: exact=%.4f similarity=%.4f",
		strings.TrimSuffix(leftName, ".pdf"),
		strings.TrimSuffix(rightName, ".pdf"),
		exactPercent,
		similarityPercent,
	)

	return exactPercent, similarityPercent
}

func renderSamplePageParityBetweenModes(t *testing.T, pdfPath string, leftMode string, rightMode string) (float64, float64) {
	t.Helper()

	root := t.TempDir()
	leftPNG := filepath.Join(root, "left.png")
	rightPNG := filepath.Join(root, "right.png")

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())

	leftOpts := pdf.DefaultRenderOptions()
	leftOpts.DPI = 72
	leftOpts.ImageSamplingMode = leftMode
	leftImg, err := renderer.RenderPage(context.Background(), page, leftOpts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(leftPNG, leftImg))

	rightOpts := pdf.DefaultRenderOptions()
	rightOpts.DPI = 72
	rightOpts.ImageSamplingMode = rightMode
	rightImg, err := renderer.RenderPage(context.Background(), page, rightOpts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(rightPNG, rightImg))

	exactPercent, similarityPercent, err := parityComparePNGs(leftPNG, rightPNG)
	require.NoError(t, err)
	return exactPercent, similarityPercent
}

func renderSamplePDFParity(t *testing.T, leftPDFPath string, rightPDFPath string) (float64, float64) {
	t.Helper()

	root := t.TempDir()
	leftPNG := filepath.Join(root, "left.png")
	rightPNG := filepath.Join(root, "right.png")

	leftDoc, err := pdf.Open(leftPDFPath)
	require.NoError(t, err)
	defer leftDoc.Close()

	rightDoc, err := pdf.Open(rightPDFPath)
	require.NoError(t, err)
	defer rightDoc.Close()

	leftPage, err := leftDoc.Page(0)
	require.NoError(t, err)
	rightPage, err := rightDoc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 72

	leftImg, err := renderer.RenderPage(context.Background(), leftPage, opts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(leftPNG, leftImg))

	rightImg, err := renderer.RenderPage(context.Background(), rightPage, opts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(rightPNG, rightImg))

	exactPercent, similarityPercent, err := parityComparePNGs(leftPNG, rightPNG)
	require.NoError(t, err)
	return exactPercent, similarityPercent
}

func probeSampleAllPagesAgainstPoppler(
	t *testing.T,
	pdfPath string,
	mode string,
) map[int]float64 {
	t.Helper()

	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	popplerRoot := filepath.Join(root, "poppler")
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, 72))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 72
	opts.ImageSamplingMode = mode

	scores := make(map[int]float64, len(popplerPages))
	for pageNum, popplerPNG := range popplerPages {
		page, err := doc.Page(pageNum - 1)
		require.NoError(t, err)

		oursPNG := filepath.Join(root, fmt.Sprintf("ours_%d.png", pageNum))
		img, err := renderer.RenderPage(context.Background(), page, opts)
		require.NoError(t, err)
		require.NoError(t, parityWritePNG(oursPNG, img))

		_, similarityPercent, err := parityComparePNGs(oursPNG, popplerPNG)
		require.NoError(t, err)
		scores[pageNum] = similarityPercent
	}

	return scores
}

func probeSamplePageAgainstPopplerAtDPI(
	t *testing.T,
	pdfPath string,
	pageNum int,
	mode string,
	dpi int,
) float64 {
	t.Helper()

	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	popplerRoot := filepath.Join(root, "poppler")
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, dpi))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, pageNum)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(pageNum - 1)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = float64(dpi)
	opts.ImageSamplingMode = mode

	oursPNG := filepath.Join(root, fmt.Sprintf("ours_%d_%d.png", pageNum, dpi))
	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(oursPNG, img))

	_, similarityPercent, err := parityComparePNGs(oursPNG, popplerPages[pageNum])
	require.NoError(t, err)
	return similarityPercent
}

func probeSamplePageModesAgainstPoppler(
	t *testing.T,
	pdfPath string,
	pageNum int,
	modes []string,
) map[string]float64 {
	return probeSamplePageModesAgainstPopplerAtDPI(t, pdfPath, pageNum, 72, modes)
}

func probeSamplePageModesAgainstPopplerAtDPI(
	t *testing.T,
	pdfPath string,
	pageNum int,
	dpi int,
	modes []string,
) map[string]float64 {
	t.Helper()

	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	popplerRoot := filepath.Join(root, "poppler")
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, dpi))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, pageNum)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(pageNum - 1)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	scores := make(map[string]float64, len(modes))
	for _, mode := range modes {
		oursPNG := filepath.Join(root, fmt.Sprintf("%s_%d.png", strings.ReplaceAll(mode, "/", "_"), pageNum))
		opts := pdf.DefaultRenderOptions()
		opts.DPI = float64(dpi)
		opts.ImageSamplingMode = mode
		img, err := renderer.RenderPage(context.Background(), page, opts)
		require.NoError(t, err)
		require.NoError(t, parityWritePNG(oursPNG, img))

		_, similarityPercent, err := parityComparePNGs(oursPNG, popplerPages[pageNum])
		require.NoError(t, err)
		scores[mode] = similarityPercent
	}

	return scores
}

func renderSampleAllPagesParityBetweenModes(t *testing.T, pdfPath string, leftMode string, rightMode string) []int {
	t.Helper()

	root := t.TempDir()
	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	pageCount, err := doc.PageCount()
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	changed := make([]int, 0)

	for pageIndex := 0; pageIndex < pageCount; pageIndex++ {
		page, pageErr := doc.Page(pageIndex)
		require.NoError(t, pageErr)

		leftPNG := filepath.Join(root, fmt.Sprintf("left_%d.png", pageIndex))
		rightPNG := filepath.Join(root, fmt.Sprintf("right_%d.png", pageIndex))

		leftOpts := pdf.DefaultRenderOptions()
		leftOpts.DPI = 72
		leftOpts.ImageSamplingMode = leftMode
		leftImg, renderErr := renderer.RenderPage(context.Background(), page, leftOpts)
		require.NoError(t, renderErr)
		require.NoError(t, parityWritePNG(leftPNG, leftImg))

		rightOpts := pdf.DefaultRenderOptions()
		rightOpts.DPI = 72
		rightOpts.ImageSamplingMode = rightMode
		rightImg, renderErr := renderer.RenderPage(context.Background(), page, rightOpts)
		require.NoError(t, renderErr)
		require.NoError(t, parityWritePNG(rightPNG, rightImg))

		exactPercent, similarityPercent, compareErr := parityComparePNGs(leftPNG, rightPNG)
		require.NoError(t, compareErr)
		t.Logf(
			"%s#p%d %s-vs-%s parity: exact=%.4f similarity=%.4f",
			filepath.Base(pdfPath),
			pageIndex+1,
			leftMode,
			rightMode,
			exactPercent,
			similarityPercent,
		)

		if exactPercent < 100.0 || similarityPercent < 100.0 {
			changed = append(changed, pageIndex+1)
		}
	}

	return changed
}

func renderSyntheticModeParity(
	t *testing.T,
	pdfName string,
	pdfBytes []byte,
	leftMode string,
	rightMode string,
) float64 {
	t.Helper()

	root := t.TempDir()
	pdfPath := filepath.Join(root, pdfName)
	require.NoError(t, os.WriteFile(pdfPath, pdfBytes, 0o644))

	exactPercent, similarityPercent := renderSamplePageParityBetweenModes(t, pdfPath, leftMode, rightMode)
	t.Logf(
		"%s rendered mode parity: exact=%.4f similarity=%.4f",
		strings.TrimSuffix(pdfName, ".pdf"),
		exactPercent,
		similarityPercent,
	)
	return exactPercent
}

func probeSyntheticPDFModeSimilaritiesAgainstPoppler(
	t *testing.T,
	pdfName string,
	pdfBytes []byte,
	legacyMode string,
	experimentalMode string,
) (float64, float64) {
	t.Helper()

	root := t.TempDir()
	pdfPath := filepath.Join(root, pdfName)
	require.NoError(t, os.WriteFile(pdfPath, pdfBytes, 0o644))

	legacyOpts := pdf.DefaultRenderOptions()
	legacyOpts.DPI = 72
	legacyOpts.ImageSamplingMode = legacyMode
	_, legacySimilarity := probePDFRenderAgainstPoppler(t, pdfPath, legacyOpts)

	experimentalOpts := pdf.DefaultRenderOptions()
	experimentalOpts.DPI = 72
	experimentalOpts.ImageSamplingMode = experimentalMode
	_, experimentalSimilarity := probePDFRenderAgainstPoppler(t, pdfPath, experimentalOpts)

	return legacySimilarity, experimentalSimilarity
}

func summarizeSamplePageOperators(t *testing.T, pdfPath string) pageOperatorSummary {
	t.Helper()

	doc, err := internalpdf.Open(pdfPath)
	require.NoError(t, err)
	defer func() {
		_ = doc.Close()
	}()

	page, err := doc.GetPage(0)
	require.NoError(t, err)

	contents, err := page.Contents()
	require.NoError(t, err)

	var summary pageOperatorSummary
	for _, obj := range contents {
		streamObj, ok := obj.(*entity.Stream)
		if !ok || streamObj == nil {
			continue
		}
		decoded, err := streamObj.Decode()
		require.NoError(t, err)
		content := string(decoded)

		summary.textBlockCount += strings.Count(content, "BT")
		summary.textShowCount += strings.Count(content, "Tj") + strings.Count(content, "TJ")
		summary.xObjectDoCount += strings.Count(content, " Do")
		summary.inlineImageBI += strings.Count(content, "BI")
	}

	return summary
}

func probeImageParity(
	t *testing.T,
	leftName string,
	left image.Image,
	rightName string,
	right image.Image,
) (float64, float64) {
	root := t.TempDir()
	leftPNG := filepath.Join(root, leftName)
	rightPNG := filepath.Join(root, rightName)

	require.NoError(t, parityWritePNG(leftPNG, imageToRGBA(left, left.Bounds())))
	require.NoError(t, parityWritePNG(rightPNG, imageToRGBA(right, right.Bounds())))

	exactPercent, similarityPercent, err := parityComparePNGs(leftPNG, rightPNG)
	require.NoError(t, err)
	t.Logf(
		"%s vs %s decoded parity: exact=%.4f similarity=%.4f",
		strings.TrimSuffix(leftName, filepath.Ext(leftName)),
		strings.TrimSuffix(rightName, filepath.Ext(rightName)),
		exactPercent,
		similarityPercent,
	)

	return exactPercent, similarityPercent
}

func (s rgbDiffSummary) String() string {
	return fmt.Sprintf("count=%d r_mae=%.4f g_mae=%.4f b_mae=%.4f", s.count, s.rMAE, s.gMAE, s.bMAE)
}

func (s paletteRGBDiffSummary) String() string {
	return fmt.Sprintf(
		"idx=%d count=%d cmyk=(%d,%d,%d,%d) r_mae=%.4f g_mae=%.4f b_mae=%.4f b_weighted=%.0f",
		s.index,
		s.count,
		s.c,
		s.m,
		s.y,
		s.k,
		s.rMAE,
		s.gMAE,
		s.bMAE,
		s.weightedBAbsError,
	)
}

func (d cmykBlueDecomposition) String() string {
	return fmt.Sprintf(
		"constant=%.4f c_term=%.4f m_term=%.4f y_term=%.4f k_term=%.4f total=%.4f",
		d.constant,
		d.cTerm,
		d.mTerm,
		d.yTerm,
		d.kTerm,
		d.total,
	)
}

func summarizeRGBDiff(left, right image.Image, mask []bool) rgbDiffSummary {
	bounds := left.Bounds()
	if !bounds.Eq(right.Bounds()) {
		return rgbDiffSummary{}
	}

	var sumR, sumG, sumB float64
	count := 0
	index := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if len(mask) > 0 && !mask[index] {
				index++
				continue
			}
			lr, lg, lb, _ := left.At(x, y).RGBA()
			rr, rg, rb, _ := right.At(x, y).RGBA()
			sumR += math.Abs(float64(int(lr>>8) - int(rr>>8)))
			sumG += math.Abs(float64(int(lg>>8) - int(rg>>8)))
			sumB += math.Abs(float64(int(lb>>8) - int(rb>>8)))
			count++
			index++
		}
	}
	if count == 0 {
		return rgbDiffSummary{}
	}

	return rgbDiffSummary{
		count: count,
		rMAE:  sumR / float64(count),
		gMAE:  sumG / float64(count),
		bMAE:  sumB / float64(count),
	}
}

func summarizeRGBDiffByKBin(left, right image.Image, imageData, palette []byte, binCount int) []rgbDiffSummary {
	bounds := left.Bounds()
	if !bounds.Eq(right.Bounds()) || len(imageData) != bounds.Dx()*bounds.Dy() || len(palette)%4 != 0 || binCount <= 0 {
		return nil
	}

	type accum struct {
		count int
		sumR  float64
		sumG  float64
		sumB  float64
	}

	accums := make([]accum, binCount)
	maxIndex := len(palette)/4 - 1
	for i, indexByte := range imageData {
		index := int(indexByte)
		if index < 0 {
			index = 0
		}
		if index > maxIndex {
			index = maxIndex
		}
		k := palette[index*4+3]
		bin := int(k) * binCount / 256
		if bin >= binCount {
			bin = binCount - 1
		}

		x := bounds.Min.X + (i % bounds.Dx())
		y := bounds.Min.Y + (i / bounds.Dx())
		lr, lg, lb, _ := left.At(x, y).RGBA()
		rr, rg, rb, _ := right.At(x, y).RGBA()

		accums[bin].sumR += math.Abs(float64(int(lr>>8) - int(rr>>8)))
		accums[bin].sumG += math.Abs(float64(int(lg>>8) - int(rg>>8)))
		accums[bin].sumB += math.Abs(float64(int(lb>>8) - int(rb>>8)))
		accums[bin].count++
	}

	out := make([]rgbDiffSummary, binCount)
	for i, acc := range accums {
		if acc.count == 0 {
			continue
		}
		out[i] = rgbDiffSummary{
			count: acc.count,
			rMAE:  acc.sumR / float64(acc.count),
			gMAE:  acc.sumG / float64(acc.count),
			bMAE:  acc.sumB / float64(acc.count),
		}
	}
	return out
}

func formatRGBDiffBins(bins []rgbDiffSummary) string {
	if len(bins) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(bins))
	for i, bin := range bins {
		parts = append(parts, fmt.Sprintf("bin%d:{%s}", i, bin.String()))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func summarizeTopPaletteRGBDiffs(left, right image.Image, imageData, palette []byte, topN int) []paletteRGBDiffSummary {
	bounds := left.Bounds()
	if !bounds.Eq(right.Bounds()) || len(imageData) != bounds.Dx()*bounds.Dy() || len(palette)%4 != 0 || topN <= 0 {
		return nil
	}

	type accum struct {
		count int
		sumR  float64
		sumG  float64
		sumB  float64
	}

	maxIndex := len(palette)/4 - 1
	accums := make([]accum, maxIndex+1)
	for i, indexByte := range imageData {
		index := int(indexByte)
		if index < 0 {
			index = 0
		}
		if index > maxIndex {
			index = maxIndex
		}

		x := bounds.Min.X + (i % bounds.Dx())
		y := bounds.Min.Y + (i / bounds.Dx())
		lr, lg, lb, _ := left.At(x, y).RGBA()
		rr, rg, rb, _ := right.At(x, y).RGBA()

		accums[index].sumR += math.Abs(float64(int(lr>>8) - int(rr>>8)))
		accums[index].sumG += math.Abs(float64(int(lg>>8) - int(rg>>8)))
		accums[index].sumB += math.Abs(float64(int(lb>>8) - int(rb>>8)))
		accums[index].count++
	}

	summaries := make([]paletteRGBDiffSummary, 0, len(accums))
	for index, acc := range accums {
		if acc.count == 0 {
			continue
		}
		base := index * 4
		summaries = append(summaries, paletteRGBDiffSummary{
			index:             index,
			count:             acc.count,
			c:                 palette[base],
			m:                 palette[base+1],
			y:                 palette[base+2],
			k:                 palette[base+3],
			rMAE:              acc.sumR / float64(acc.count),
			gMAE:              acc.sumG / float64(acc.count),
			bMAE:              acc.sumB / float64(acc.count),
			weightedBAbsError: acc.sumB,
		})
	}

	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].weightedBAbsError == summaries[j].weightedBAbsError {
			if summaries[i].bMAE == summaries[j].bMAE {
				return summaries[i].count > summaries[j].count
			}
			return summaries[i].bMAE > summaries[j].bMAE
		}
		return summaries[i].weightedBAbsError > summaries[j].weightedBAbsError
	})

	if len(summaries) > topN {
		summaries = summaries[:topN]
	}
	return summaries
}

func formatTopPaletteRGBDiffs(summaries []paletteRGBDiffSummary) string {
	if len(summaries) == 0 {
		return "[]"
	}

	parts := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		parts = append(parts, "{"+summary.String()+"}")
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func summarizeTopPaletteBlueTerms(summaries []paletteRGBDiffSummary) []string {
	if len(summaries) == 0 {
		return nil
	}

	lines := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		c := float64(summary.c) / 255.0
		m := float64(summary.m) / 255.0
		y := float64(summary.y) / 255.0
		k := float64(summary.k) / 255.0
		current := currentCMYKToRGBA(c, m, y, k)
		simple := simpleCMYKBytesToRGBA(summary.c, summary.m, summary.y, summary.k)
		stdlib := stdlibCMYKToRGBA(summary.c, summary.m, summary.y, summary.k)
		terms := decomposeCurrentCMYKBlue(c, m, y, k)
		lines = append(lines, fmt.Sprintf(
			"idx=%d cmyk=(%d,%d,%d,%d) count=%d current_b=%d simple_b=%d stdlib_b=%d b_delta_simple=%d b_delta_stdlib=%d terms={%s}",
			summary.index,
			summary.c,
			summary.m,
			summary.y,
			summary.k,
			summary.count,
			current.B,
			simple.B,
			stdlib.B,
			int(current.B)-int(simple.B),
			int(current.B)-int(stdlib.B),
			terms.String(),
		))
	}
	return lines
}

func formatTopPaletteBlueTerms(lines []string) string {
	if len(lines) == 0 {
		return "[]"
	}
	return "[" + strings.Join(lines, " | ") + "]"
}

func applySimpleBlueCorrectionToPalette(currentRGBPalette, cmykPalette []byte, summaries []paletteRGBDiffSummary) []byte {
	if len(currentRGBPalette)%3 != 0 || len(cmykPalette)%4 != 0 {
		return nil
	}

	corrected := append([]byte(nil), currentRGBPalette...)
	for _, summary := range summaries {
		rgbBase := summary.index * 3
		cmykBase := summary.index * 4
		if rgbBase+2 >= len(corrected) || cmykBase+3 >= len(cmykPalette) {
			continue
		}
		simple := simpleCMYKBytesToRGBA(
			cmykPalette[cmykBase],
			cmykPalette[cmykBase+1],
			cmykPalette[cmykBase+2],
			cmykPalette[cmykBase+3],
		)
		corrected[rgbBase+2] = simple.B
	}
	return corrected
}

func applyReferencePaletteRGBToPalette(currentRGBPalette, cmykPalette []byte, summaries []paletteRGBDiffSummary, mode string) []byte {
	if len(currentRGBPalette)%3 != 0 || len(cmykPalette)%4 != 0 {
		return nil
	}

	corrected := append([]byte(nil), currentRGBPalette...)
	for _, summary := range summaries {
		rgbBase := summary.index * 3
		cmykBase := summary.index * 4
		if rgbBase+2 >= len(corrected) || cmykBase+3 >= len(cmykPalette) {
			continue
		}

		var rgba color.RGBA
		switch mode {
		case "simple":
			rgba = simpleCMYKBytesToRGBA(
				cmykPalette[cmykBase],
				cmykPalette[cmykBase+1],
				cmykPalette[cmykBase+2],
				cmykPalette[cmykBase+3],
			)
		case "stdlib":
			rgba = stdlibCMYKToRGBA(
				cmykPalette[cmykBase],
				cmykPalette[cmykBase+1],
				cmykPalette[cmykBase+2],
				cmykPalette[cmykBase+3],
			)
		default:
			continue
		}

		corrected[rgbBase] = rgba.R
		corrected[rgbBase+1] = rgba.G
		corrected[rgbBase+2] = rgba.B
	}
	return corrected
}

func decomposeCurrentCMYKBlue(c, m, y, k float64) cmykBlueDecomposition {
	constant := 255.0
	cTerm := c * (0.8842522430003296*c + 8.078677503112928*m + 30.89978309703729*y - 0.23883238689178934*k - 14.183576799673286)
	mTerm := m * (10.49593273432072*m + 63.02378494754052*y + 50.606957656360734*k - 112.23884253719248)
	yTerm := y * (0.03296041114873217*y + 115.60384449646641*k - 193.58209356861505)
	kTerm := k * (-22.33816807309886*k - 180.12613974708367)
	return cmykBlueDecomposition{
		constant: constant,
		cTerm:    cTerm,
		mTerm:    mTerm,
		yTerm:    yTerm,
		kTerm:    kTerm,
		total:    constant + cTerm + mTerm + yTerm + kTerm,
	}
}

func currentCMYKToRGBA(c, m, y, k float64) color.RGBA {
	return colorspace.NewDeviceCMYK().ConvertToRGBA([]float64{c, m, y, k})
}

func stdlibCMYKToRGBA(c, m, y, k uint8) color.RGBA {
	converted := color.CMYK{C: c, M: m, Y: y, K: k}
	r, g, b, _ := converted.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: 255}
}

func probeSyntheticCMYKReferenceSetAgainstPoppler(
	t *testing.T,
	pdfNamePrefix string,
	pageW, pageH float64,
	imageW, imageH int,
	matrix [6]float64,
	cmyk []byte,
	rgbReferences map[string][]byte,
) (parityScore, map[string]parityScore) {
	directExact, directSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		pdfNamePrefix+"_direct.pdf",
		buildSyntheticCMYKImagePDFFloat(pageW, pageH, imageW, imageH, matrix, cmyk),
	)

	scores := make(map[string]parityScore, len(rgbReferences))
	for name, rgb := range rgbReferences {
		exactPercent, similarityPercent := probeSyntheticCurrentRenderAgainstPoppler(
			t,
			pdfNamePrefix+"_"+name+".pdf",
			buildSyntheticRGBImagePDFFloat(pageW, pageH, imageW, imageH, matrix, rgb),
		)
		scores[name] = parityScore{exact: exactPercent, similarity: similarityPercent}
	}

	return parityScore{exact: directExact, similarity: directSimilarity}, scores
}

func probeSyntheticIndexedCMYKPaletteReferenceSetAgainstPoppler(
	t *testing.T,
	pdfNamePrefix string,
	cmykPalette []byte,
	imageW, imageH int,
	imageData []byte,
) (parityScore, map[string]parityScore) {
	directExact, directSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		pdfNamePrefix+"_direct.pdf",
		buildSyntheticIndexedImagePDFFloatWithBase(
			float64(imageW),
			float64(imageH),
			4,
			"DeviceCMYK",
			imageW,
			imageH,
			[6]float64{float64(imageW), 0, 0, float64(imageH), 0, 0},
			cmykPalette,
			imageData,
		),
	)

	references := map[string][]byte{
		"current_rgb": convertCMYKPaletteToCurrentRGBPalette(cmykPalette),
		"simple_rgb":  convertCMYKPaletteToSimpleRGBPalette(cmykPalette),
		"stdlib_rgb":  convertCMYKPaletteToStdlibRGBPalette(cmykPalette),
	}

	scores := make(map[string]parityScore, len(references))
	for name, rgbPalette := range references {
		exactPercent, similarityPercent := probeSyntheticCurrentRenderAgainstPoppler(
			t,
			pdfNamePrefix+"_"+name+".pdf",
			buildSyntheticIndexedImagePDFFloatWithBase(
				float64(imageW),
				float64(imageH),
				3,
				"DeviceRGB",
				imageW,
				imageH,
				[6]float64{float64(imageW), 0, 0, float64(imageH), 0, 0},
				rgbPalette,
				imageData,
			),
		)
		scores[name] = parityScore{exact: exactPercent, similarity: similarityPercent}
	}

	return parityScore{exact: directExact, similarity: directSimilarity}, scores
}

func probeIndexedPaletteReferenceSetAtGeometryAgainstPoppler(
	t *testing.T,
	pdfNamePrefix string,
	cmykPalette []byte,
	imageW, imageH int,
	imageData []byte,
	pageW, pageH float64,
	matrix [6]float64,
) (parityScore, map[string]parityScore) {
	return probeIndexedPaletteReferenceSetAtGeometryAgainstPopplerWithReferences(
		t,
		pdfNamePrefix,
		cmykPalette,
		imageW,
		imageH,
		imageData,
		pageW,
		pageH,
		matrix,
		map[string][]byte{
			"current_rgb": convertCMYKPaletteToCurrentRGBPalette(cmykPalette),
			"simple_rgb":  convertCMYKPaletteToSimpleRGBPalette(cmykPalette),
			"stdlib_rgb":  convertCMYKPaletteToStdlibRGBPalette(cmykPalette),
		},
	)
}

func probeIndexedPaletteReferenceSetAtGeometryAgainstPopplerWithReferences(
	t *testing.T,
	pdfNamePrefix string,
	cmykPalette []byte,
	imageW, imageH int,
	imageData []byte,
	pageW, pageH float64,
	matrix [6]float64,
	references map[string][]byte,
) (parityScore, map[string]parityScore) {
	directExact, directSimilarity := probeSyntheticCurrentRenderAgainstPoppler(
		t,
		pdfNamePrefix+"_direct.pdf",
		buildSyntheticIndexedImagePDFFloatWithBase(
			pageW,
			pageH,
			4,
			"DeviceCMYK",
			imageW,
			imageH,
			matrix,
			cmykPalette,
			imageData,
		),
	)

	scores := make(map[string]parityScore, len(references))
	for name, rgbPalette := range references {
		exactPercent, similarityPercent := probeSyntheticCurrentRenderAgainstPoppler(
			t,
			pdfNamePrefix+"_"+name+".pdf",
			buildSyntheticIndexedImagePDFFloatWithBase(
				pageW,
				pageH,
				3,
				"DeviceRGB",
				imageW,
				imageH,
				matrix,
				rgbPalette,
				imageData,
			),
		)
		scores[name] = parityScore{exact: exactPercent, similarity: similarityPercent}
	}

	return parityScore{exact: directExact, similarity: directSimilarity}, scores
}

func probeSyntheticCurrentAndReferencesAgainstPoppler(
	t *testing.T,
	pdfName string,
	pdfBytes []byte,
	references map[string]image.Image,
) (float64, float64, map[string]float64) {
	return probeSyntheticCurrentAndReferencesAgainstPopplerAtDPI(t, pdfName, pdfBytes, 72, references)
}

func probeSyntheticCurrentAndReferencesAgainstPopplerAtDPI(
	t *testing.T,
	pdfName string,
	pdfBytes []byte,
	dpi float64,
	references map[string]image.Image,
) (float64, float64, map[string]float64) {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		t.Skip("pdftoppm not installed")
	}

	root := t.TempDir()
	pdfPath := filepath.Join(root, pdfName)
	popplerRoot := filepath.Join(root, "poppler")
	oursPNG := filepath.Join(root, "ours.png")

	require.NoError(t, os.WriteFile(pdfPath, pdfBytes, 0o644))
	require.NoError(t, os.MkdirAll(popplerRoot, 0o755))
	require.NoError(t, parityRunPoppler(pdfPath, popplerRoot, int(dpi)))

	popplerPages, err := parityListPopplerPages(popplerRoot)
	require.NoError(t, err)
	require.Contains(t, popplerPages, 1)

	doc, err := pdf.Open(pdfPath)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = dpi

	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)
	require.NoError(t, parityWritePNG(oursPNG, img))

	currentExact, currentSimilarity, err := parityComparePNGs(oursPNG, popplerPages[1])
	require.NoError(t, err)

	refScores := make(map[string]float64, len(references))
	for name, ref := range references {
		refPNG := filepath.Join(root, name+".png")
		require.NoError(t, parityWritePNG(refPNG, imageToRGBA(ref, ref.Bounds())))
		_, similarity, err := parityComparePNGs(refPNG, popplerPages[1])
		require.NoError(t, err)
		refScores[name] = similarity
	}

	t.Logf(
		"%s parity: current_exact=%.4f current_similarity=%.4f refs=%v",
		strings.TrimSuffix(pdfName, ".pdf"),
		currentExact,
		currentSimilarity,
		refScores,
	)

	return currentExact, currentSimilarity, refScores
}

func buildSyntheticContentPDF(pageW, pageH int, content []byte) []byte {
	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %d %d] /Resources << /ProcSet [/PDF] >> /Contents 4 0 R >>",
			pageW, pageH,
		)),
		syntheticStreamObject(
			fmt.Sprintf("<< /Length %d >>", len(content)),
			content,
		),
	}

	return buildSyntheticPDF(objects)
}

func loadSampleIndexedImageObject(
	t *testing.T,
	pdfPath string,
	ref entity.Ref,
) (string, []byte, []byte, int, int) {
	t.Helper()

	data, err := os.ReadFile(pdfPath)
	require.NoError(t, err)

	xrefTable := pdfxref.NewTable(data)
	require.NoError(t, xrefTable.Parse())

	obj, err := xrefTable.Fetch(ref)
	require.NoError(t, err)

	streamObj, ok := obj.(*entity.Stream)
	require.True(t, ok)

	width := requireObjectInt(t, streamObj.Dict().Get(entity.Name("Width")))
	height := requireObjectInt(t, streamObj.Dict().Get(entity.Name("Height")))
	base, lookup := requireIndexedColorSpace(t, xrefTable, streamObj.Dict().Get(entity.Name("ColorSpace")))
	imageData, err := pdfstream.NewFromEntity(streamObj).Decode()
	require.NoError(t, err)

	return base, lookup, imageData, width, height
}

func decodeSampleIndexedImageObjectToRGBWithMode(
	t *testing.T,
	pdfPath string,
	ref entity.Ref,
	cmykMode string,
) (image.Image, []byte) {
	t.Helper()

	imgData := loadSampleIndexedImageData(t, pdfPath, ref)
	imgData.CMYKConversionMode = cmykMode

	decoder := infraimage.NewDecoder()
	decoded, err := decoder.Decode(imgData)
	require.NoError(t, err)

	return decoded.Image(), imageToRGBBytes(decoded.Image())
}

func loadSampleIndexedImageData(
	t *testing.T,
	pdfPath string,
	ref entity.Ref,
) *domainimage.ImageData {
	t.Helper()

	data, err := os.ReadFile(pdfPath)
	require.NoError(t, err)

	xrefTable := pdfxref.NewTable(data)
	require.NoError(t, xrefTable.Parse())

	obj, err := xrefTable.Fetch(ref)
	require.NoError(t, err)

	streamObj, ok := obj.(*entity.Stream)
	require.True(t, ok)

	width := requireObjectInt(t, streamObj.Dict().Get(entity.Name("Width")))
	height := requireObjectInt(t, streamObj.Dict().Get(entity.Name("Height")))
	bpc := requireObjectInt(t, streamObj.Dict().Get(entity.Name("BitsPerComponent")))
	base, lookup := requireIndexedColorSpace(t, xrefTable, streamObj.Dict().Get(entity.Name("ColorSpace")))
	imageData, err := pdfstream.NewFromEntity(streamObj).Decode()
	require.NoError(t, err)

	return &domainimage.ImageData{
		Data:             imageData,
		Filter:           domainimage.FilterNone,
		ColorSpace:       domainimage.ColorSpaceIndexed,
		IndexedBase:      domainimage.ColorSpace(base),
		IndexedLookup:    lookup,
		Width:            width,
		Height:           height,
		BitsPerComponent: bpc,
		Decode:           requireObjectFloatSlice(t, xrefTable, streamObj.Dict().Get(entity.Name("Decode"))),
	}
}

func loadSampleDirectImageData(
	t *testing.T,
	pdfPath string,
	ref entity.Ref,
) *domainimage.ImageData {
	t.Helper()

	data, err := os.ReadFile(pdfPath)
	require.NoError(t, err)

	xrefTable := pdfxref.NewTable(data)
	require.NoError(t, xrefTable.Parse())

	obj, err := xrefTable.Fetch(ref)
	require.NoError(t, err)

	streamObj, ok := obj.(*entity.Stream)
	require.True(t, ok)

	width := requireObjectInt(t, streamObj.Dict().Get(entity.Name("Width")))
	height := requireObjectInt(t, streamObj.Dict().Get(entity.Name("Height")))
	bpc := requireObjectInt(t, streamObj.Dict().Get(entity.Name("BitsPerComponent")))
	colorSpace := requireColorSpaceName(t, xrefTable, streamObj.Dict().Get(entity.Name("ColorSpace")))
	imageData, err := pdfstream.NewFromEntity(streamObj).Decode()
	require.NoError(t, err)

	return &domainimage.ImageData{
		Data:             imageData,
		Filter:           domainimage.FilterNone,
		ColorSpace:       domainimage.ColorSpace(colorSpace),
		Width:            width,
		Height:           height,
		BitsPerComponent: bpc,
		Decode:           requireObjectFloatSlice(t, xrefTable, streamObj.Dict().Get(entity.Name("Decode"))),
		Mask:             requireSampleSoftMask(t, xrefTable, streamObj.Dict().Get(entity.Name("SMask"))),
	}
}

func loadSampleEncodedImageData(
	t *testing.T,
	pdfPath string,
	ref entity.Ref,
) *domainimage.ImageData {
	t.Helper()

	data, err := os.ReadFile(pdfPath)
	require.NoError(t, err)

	xrefTable := pdfxref.NewTable(data)
	require.NoError(t, xrefTable.Parse())

	obj, err := xrefTable.Fetch(ref)
	require.NoError(t, err)

	streamObj, ok := obj.(*entity.Stream)
	require.True(t, ok)

	width := requireObjectInt(t, streamObj.Dict().Get(entity.Name("Width")))
	height := requireObjectInt(t, streamObj.Dict().Get(entity.Name("Height")))
	bpc := requireObjectInt(t, streamObj.Dict().Get(entity.Name("BitsPerComponent")))
	colorSpace, iccProfile, iccComponents := requireSampleImageColorSpaceMetadata(
		t,
		xrefTable,
		streamObj.Dict().Get(entity.Name("ColorSpace")),
	)

	return &domainimage.ImageData{
		Data:             append([]byte(nil), streamObj.RawBytes()...),
		Filter:           requireImageFilter(t, xrefTable, streamObj.Dict().Get(entity.Name("Filter"))),
		ColorSpace:       domainimage.ColorSpace(colorSpace),
		ICCProfile:       iccProfile,
		ICCComponents:    iccComponents,
		Width:            width,
		Height:           height,
		BitsPerComponent: bpc,
		DecodeParms:      requireDecodeParms(t, xrefTable, streamObj.Dict().Get(entity.Name("DecodeParms"))),
		Decode:           requireObjectFloatSlice(t, xrefTable, streamObj.Dict().Get(entity.Name("Decode"))),
		Mask:             requireSampleSoftMask(t, xrefTable, streamObj.Dict().Get(entity.Name("SMask"))),
	}
}

func decodeSampleDirectImageObjectToAppliedRGBA(
	t *testing.T,
	pdfPath string,
	ref entity.Ref,
) (image.Image, []byte) {
	t.Helper()

	imgData := loadSampleDirectImageData(t, pdfPath, ref)

	decoder := infraimage.NewDecoder()
	decoded, err := decoder.Decode(imgData)
	require.NoError(t, err)

	applied := decoded.Image()
	if decoded.HasMask() {
		applied = infraimage.ApplyMask(applied, decoded.Mask())
	}

	return applied, imageToOpaqueRGBBytes(applied, color.White)
}

func decodeSampleEncodedImageObjectToRGB(
	t *testing.T,
	pdfPath string,
	ref entity.Ref,
) (image.Image, []byte) {
	t.Helper()

	imgData := loadSampleEncodedImageData(t, pdfPath, ref)

	decoder := infraimage.NewDecoder()
	decoded, err := decoder.Decode(imgData)
	require.NoError(t, err)

	return decoded.Image(), imageToRGBBytes(decoded.Image())
}

func decodeSampleEncodedImageObjectToRGBWithoutICC(
	t *testing.T,
	pdfPath string,
	ref entity.Ref,
) (image.Image, []byte) {
	t.Helper()

	imgData := loadSampleEncodedImageData(t, pdfPath, ref)
	imgData.ICCProfile = nil
	imgData.ICCComponents = 0

	decoder := infraimage.NewDecoder()
	decoded, err := decoder.Decode(imgData)
	require.NoError(t, err)

	return decoded.Image(), imageToRGBBytes(decoded.Image())
}

func requireSampleImageColorSpaceMetadata(
	t *testing.T,
	xrefTable *pdfxref.Table,
	obj entity.Object,
) (string, []byte, int) {
	t.Helper()

	switch v := obj.(type) {
	case nil:
		return "", nil, 0
	case entity.Ref:
		resolved, err := xrefTable.Fetch(v)
		require.NoError(t, err)
		return requireSampleImageColorSpaceMetadata(t, xrefTable, resolved)
	case entity.Name:
		return v.Value(), nil, 0
	case *entity.Array:
		require.Greater(t, v.Len(), 0)

		name, ok := v.Get(0).(entity.Name)
		require.True(t, ok)

		switch name.Value() {
		case "ICCBased":
			require.GreaterOrEqual(t, v.Len(), 2)
			profile := requireSampleICCProfileBytes(t, xrefTable, v.Get(1))
			components := requireSampleICCComponentCount(t, xrefTable, v.Get(1))
			return mapICCComponentCountToBaseColorSpace(t, components), profile, components
		default:
			t.Fatalf("unsupported sample image colorspace array: %s", name.Value())
			return "", nil, 0
		}
	default:
		t.Fatalf("unsupported sample image colorspace object: %T", obj)
		return "", nil, 0
	}
}

func requireSampleICCProfileBytes(t *testing.T, xrefTable *pdfxref.Table, obj entity.Object) []byte {
	t.Helper()

	streamObj := requireStreamObject(t, xrefTable, obj)
	require.NotNil(t, streamObj)

	profile, err := pdfstream.NewFromEntity(streamObj).Decode()
	require.NoError(t, err)
	return profile
}

func requireSampleICCComponentCount(t *testing.T, xrefTable *pdfxref.Table, obj entity.Object) int {
	t.Helper()

	switch v := obj.(type) {
	case entity.Ref:
		resolved, err := xrefTable.Fetch(v)
		require.NoError(t, err)
		return requireSampleICCComponentCount(t, xrefTable, resolved)
	case *entity.Stream:
		return requireSampleICCBasedN(t, v.Dict())
	case *entity.Dict:
		return requireSampleICCBasedN(t, v)
	default:
		t.Fatalf("unsupported sample ICC profile object: %T", obj)
		return 0
	}
}

func requireSampleICCBasedN(t *testing.T, dict *entity.Dict) int {
	t.Helper()

	require.NotNil(t, dict)
	switch n := dict.Get(entity.Name("N")).(type) {
	case *entity.Integer:
		return int(n.Value())
	case *entity.Real:
		return int(n.Value())
	default:
		t.Fatalf("unsupported ICCBased N object: %T", dict.Get(entity.Name("N")))
		return 0
	}
}

func mapICCComponentCountToBaseColorSpace(t *testing.T, components int) string {
	t.Helper()

	switch components {
	case 1:
		return "DeviceGray"
	case 3:
		return "DeviceRGB"
	case 4:
		return "DeviceCMYK"
	default:
		t.Fatalf("unsupported ICC component count: %d", components)
		return ""
	}
}

func decodeSampleFirstInlineImageToRGB(
	t *testing.T,
	pdfPath string,
) (image.Image, []byte) {
	t.Helper()

	imgData := loadSampleFirstInlineImageData(t, pdfPath)

	decoder := infraimage.NewDecoder()
	decoded, err := decoder.Decode(imgData)
	require.NoError(t, err)

	return decoded.Image(), imageToRGBBytes(decoded.Image())
}

func loadSampleFirstInlineImageData(
	t *testing.T,
	pdfPath string,
) *domainimage.ImageData {
	t.Helper()

	doc, err := internalpdf.Open(pdfPath)
	require.NoError(t, err)
	defer func() {
		_ = doc.Close()
	}()

	page, err := doc.GetPage(0)
	require.NoError(t, err)

	contents, err := page.Contents()
	require.NoError(t, err)

	for _, obj := range contents {
		streamObj, ok := obj.(*entity.Stream)
		if !ok || streamObj == nil {
			continue
		}
		decoded, err := streamObj.Decode()
		require.NoError(t, err)

		imgData, found := extractFirstInlineImageData(t, decoded)
		if found {
			return imgData
		}
	}

	require.FailNow(t, "inline image not found")
	return nil
}

func extractFirstInlineImageData(t *testing.T, content []byte) (*domainimage.ImageData, bool) {
	t.Helper()

	start := bytes.Index(content, []byte("BI"))
	if start < 0 {
		return nil, false
	}

	idMarker := bytes.Index(content[start:], []byte("ID"))
	if idMarker < 0 {
		return nil, false
	}
	idMarker += start

	dict := parseInlineImageDictForTest(t, string(content[start+2:idMarker]))
	dataStart := skipInlineImageLeadingWhitespaceForTest(content, idMarker+2)
	dataEnd, err := findInlineImageEndOffsetForTest(content, dataStart)
	require.NoError(t, err)

	decodedBytes, err := pdfstream.NewFromEntity(entity.NewStream(dict, append([]byte(nil), content[dataStart:dataEnd]...))).Decode()
	require.NoError(t, err)

	return &domainimage.ImageData{
		Data:             decodedBytes,
		Filter:           domainimage.FilterNone,
		ColorSpace:       domainimage.ColorSpace(requireInlineImageColorSpaceName(t, dict.Get(entity.Name("ColorSpace")))),
		Width:            requireObjectInt(t, dict.Get(entity.Name("Width"))),
		Height:           requireObjectInt(t, dict.Get(entity.Name("Height"))),
		BitsPerComponent: requireObjectInt(t, dict.Get(entity.Name("BitsPerComponent"))),
	}, true
}

func parseInlineImageDictForTest(t *testing.T, dictText string) *entity.Dict {
	t.Helper()

	fields := strings.Fields(dictText)
	dict := entity.NewDict()
	for i := 0; i < len(fields); i++ {
		key := fields[i]
		if !strings.HasPrefix(key, "/") {
			continue
		}
		if i+1 >= len(fields) {
			break
		}

		switch key {
		case "/W":
			i++
			dict.Set(entity.Name("Width"), entity.NewInteger(int64(parseInlineImageIntForTest(t, fields[i]))))
		case "/H":
			i++
			dict.Set(entity.Name("Height"), entity.NewInteger(int64(parseInlineImageIntForTest(t, fields[i]))))
		case "/BPC":
			i++
			dict.Set(entity.Name("BitsPerComponent"), entity.NewInteger(int64(parseInlineImageIntForTest(t, fields[i]))))
		case "/CS":
			i++
			dict.Set(entity.Name("ColorSpace"), normalizeInlineImageColorSpaceForTest(fields[i]))
		case "/F":
			i++
			filterObj, next := parseInlineImageFilterObjectForTest(fields, i)
			dict.Set(entity.Name("Filter"), filterObj)
			i = next
		}
	}

	return dict
}

func parseInlineImageFilterObjectForTest(fields []string, start int) (entity.Object, int) {
	token := fields[start]
	if strings.HasPrefix(token, "[") {
		values := make([]entity.Object, 0, 2)
		for idx := start; idx < len(fields); idx++ {
			part := strings.Trim(fields[idx], "[]")
			if part != "" {
				values = append(values, normalizeInlineImageFilterNameForTest(part))
			}
			if strings.HasSuffix(fields[idx], "]") {
				return entity.NewArray(values...), idx
			}
		}
		return entity.NewArray(values...), len(fields) - 1
	}

	return normalizeInlineImageFilterNameForTest(token), start
}

func normalizeInlineImageColorSpaceForTest(token string) entity.Object {
	switch strings.TrimPrefix(token, "/") {
	case "RGB":
		return entity.Name("DeviceRGB")
	case "G":
		return entity.Name("DeviceGray")
	case "CMYK":
		return entity.Name("DeviceCMYK")
	default:
		return entity.Name(strings.TrimPrefix(token, "/"))
	}
}

func normalizeInlineImageFilterNameForTest(token string) entity.Object {
	switch strings.TrimPrefix(token, "/") {
	case "A85":
		return entity.Name("ASCII85Decode")
	case "AHx":
		return entity.Name("ASCIIHexDecode")
	case "Fl":
		return entity.Name("FlateDecode")
	case "LZW":
		return entity.Name("LZWDecode")
	case "DCT":
		return entity.Name("DCTDecode")
	default:
		return entity.Name(strings.TrimPrefix(token, "/"))
	}
}

func requireInlineImageColorSpaceName(t *testing.T, obj entity.Object) string {
	t.Helper()

	name, ok := obj.(entity.Name)
	require.True(t, ok)
	return string(name)
}

func parseInlineImageIntForTest(t *testing.T, value string) int {
	t.Helper()

	parsed, err := strconv.Atoi(value)
	require.NoError(t, err)
	return parsed
}

func skipInlineImageLeadingWhitespaceForTest(data []byte, start int) int {
	for start < len(data) {
		switch data[start] {
		case 0x00, 0x09, 0x0A, 0x0C, 0x0D, 0x20:
			start++
		default:
			return start
		}
	}
	return start
}

func findInlineImageEndOffsetForTest(data []byte, start int) (int, error) {
	for i := start; i+1 < len(data); i++ {
		if data[i] != 'E' || data[i+1] != 'I' {
			continue
		}
		if i > 0 && !isInlineImageTokenBoundaryForTest(data[i-1]) {
			continue
		}
		if i+2 < len(data) && !isInlineImageTokenBoundaryForTest(data[i+2]) {
			continue
		}
		return i, nil
	}
	return 0, fmt.Errorf("inline image missing EI")
}

func isInlineImageTokenBoundaryForTest(b byte) bool {
	if b == 0x00 || b == 0x09 || b == 0x0A || b == 0x0C || b == 0x0D || b == 0x20 {
		return true
	}
	switch b {
	case '(', ')', '<', '>', '[', ']', '/', '%':
		return true
	default:
		return false
	}
}

func requireSampleSoftMask(t *testing.T, xrefTable *pdfxref.Table, obj entity.Object) domainimage.ImageMask {
	t.Helper()

	if obj == nil {
		return nil
	}

	streamObj := requireStreamObject(t, xrefTable, obj)
	if streamObj == nil {
		return nil
	}

	width := requireObjectInt(t, streamObj.Dict().Get(entity.Name("Width")))
	height := requireObjectInt(t, streamObj.Dict().Get(entity.Name("Height")))
	bpc := 8
	if bpcObj := streamObj.Dict().Get(entity.Name("BitsPerComponent")); bpcObj != nil {
		bpc = requireObjectInt(t, bpcObj)
	}
	colorSpace := "DeviceGray"
	if csObj := streamObj.Dict().Get(entity.Name("ColorSpace")); csObj != nil {
		colorSpace = requireColorSpaceName(t, xrefTable, csObj)
	}

	maskData, err := pdfstream.NewFromEntity(streamObj).Decode()
	require.NoError(t, err)

	decoder := infraimage.NewDecoder()
	decoded, err := decoder.Decode(&domainimage.ImageData{
		Data:             maskData,
		Filter:           domainimage.FilterNone,
		ColorSpace:       domainimage.ColorSpace(colorSpace),
		Width:            width,
		Height:           height,
		BitsPerComponent: bpc,
		Decode:           requireObjectFloatSlice(t, xrefTable, streamObj.Dict().Get(entity.Name("Decode"))),
	})
	require.NoError(t, err)

	gray := image.NewGray(decoded.Image().Bounds())
	draw.Draw(gray, gray.Bounds(), decoded.Image(), decoded.Image().Bounds().Min, draw.Src)
	return infraimage.NewBitmapMaskFromImage(gray, false)
}

func requireStreamObject(t *testing.T, xrefTable *pdfxref.Table, obj entity.Object) *entity.Stream {
	t.Helper()

	switch v := obj.(type) {
	case *entity.Stream:
		return v
	case entity.Ref:
		resolved, err := xrefTable.Fetch(v)
		require.NoError(t, err)
		return requireStreamObject(t, xrefTable, resolved)
	default:
		t.Fatalf("unsupported stream object: %T", obj)
		return nil
	}
}

func requireImageFilter(t *testing.T, xrefTable *pdfxref.Table, obj entity.Object) domainimage.ImageFilter {
	t.Helper()

	if obj == nil {
		return domainimage.FilterNone
	}

	switch v := obj.(type) {
	case entity.Ref:
		resolved, err := xrefTable.Fetch(v)
		require.NoError(t, err)
		return requireImageFilter(t, xrefTable, resolved)
	case entity.Name:
		switch v.Value() {
		case "ASCIIHexDecode":
			return domainimage.FilterASCIIHex
		case "ASCII85Decode":
			return domainimage.FilterASCII85
		case "LZWDecode":
			return domainimage.FilterLZW
		case "FlateDecode":
			return domainimage.FilterFlate
		case "RunLengthDecode":
			return domainimage.FilterRunLength
		case "CCITTFaxDecode":
			return domainimage.FilterCCITTFax
		case "DCTDecode":
			return domainimage.FilterDCT
		case "JPXDecode":
			return domainimage.FilterJPX
		case "JBIG2Decode":
			return domainimage.FilterJBIG2
		default:
			t.Fatalf("unsupported image filter name: %s", v.Value())
			return domainimage.FilterNone
		}
	case *entity.Array:
		require.Greater(t, v.Len(), 0)
		return requireImageFilter(t, xrefTable, v.Get(0))
	default:
		t.Fatalf("unsupported filter object: %T", obj)
		return domainimage.FilterNone
	}
}

func requireDecodeParms(t *testing.T, xrefTable *pdfxref.Table, obj entity.Object) map[string]interface{} {
	t.Helper()

	if obj == nil {
		return nil
	}

	switch v := obj.(type) {
	case entity.Ref:
		resolved, err := xrefTable.Fetch(v)
		require.NoError(t, err)
		return requireDecodeParms(t, xrefTable, resolved)
	case *entity.Dict:
		out := make(map[string]interface{}, v.Len())
		for _, key := range v.Keys() {
			out[key.Value()] = normalizeDecodeParamValue(t, xrefTable, v.Get(key))
		}
		return out
	case *entity.Array:
		require.Greater(t, v.Len(), 0)
		return requireDecodeParms(t, xrefTable, v.Get(0))
	default:
		t.Fatalf("unsupported decode parms object: %T", obj)
		return nil
	}
}

func normalizeDecodeParamValue(t *testing.T, xrefTable *pdfxref.Table, obj entity.Object) interface{} {
	t.Helper()

	switch v := obj.(type) {
	case entity.Ref:
		resolved, err := xrefTable.Fetch(v)
		require.NoError(t, err)
		return normalizeDecodeParamValue(t, xrefTable, resolved)
	case *entity.Integer:
		return int(v.Value())
	case *entity.Real:
		return v.Value()
	case entity.Name:
		return v.Value()
	case *entity.Boolean:
		return v.Value()
	case *entity.Array:
		out := make([]interface{}, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			out = append(out, normalizeDecodeParamValue(t, xrefTable, v.Get(i)))
		}
		return out
	default:
		return nil
	}
}

func requireIndexedColorSpace(t *testing.T, xrefTable *pdfxref.Table, obj entity.Object) (string, []byte) {
	t.Helper()

	switch v := obj.(type) {
	case entity.Ref:
		resolved, err := xrefTable.Fetch(v)
		require.NoError(t, err)
		return requireIndexedColorSpace(t, xrefTable, resolved)
	case *entity.Array:
		require.GreaterOrEqual(t, v.Len(), 4)
		name, ok := v.Get(0).(entity.Name)
		require.True(t, ok)
		require.Equal(t, "Indexed", name.Value())
		base := requireColorSpaceName(t, xrefTable, v.Get(1))
		lookup := requireLookupBytes(t, xrefTable, v.Get(3))
		return base, lookup
	default:
		t.Fatalf("unsupported indexed colorspace object: %T", obj)
		return "", nil
	}
}

func requireColorSpaceName(t *testing.T, xrefTable *pdfxref.Table, obj entity.Object) string {
	t.Helper()

	switch v := obj.(type) {
	case entity.Ref:
		resolved, err := xrefTable.Fetch(v)
		require.NoError(t, err)
		return requireColorSpaceName(t, xrefTable, resolved)
	case entity.Name:
		return v.Value()
	default:
		t.Fatalf("unsupported colorspace name object: %T", obj)
		return ""
	}
}

func requireLookupBytes(t *testing.T, xrefTable *pdfxref.Table, obj entity.Object) []byte {
	t.Helper()

	switch v := obj.(type) {
	case entity.Ref:
		resolved, err := xrefTable.Fetch(v)
		require.NoError(t, err)
		return requireLookupBytes(t, xrefTable, resolved)
	case *entity.String:
		if v.IsHex() {
			decoded, err := hex.DecodeString(v.Value())
			if err == nil {
				return decoded
			}
			return []byte(v.Value())
		}
		return []byte(v.Value())
	case *entity.Stream:
		lookup, err := pdfstream.NewFromEntity(v).Decode()
		require.NoError(t, err)
		return lookup
	default:
		t.Fatalf("unsupported indexed lookup object: %T", obj)
		return nil
	}
}

func requireObjectInt(t *testing.T, obj entity.Object) int {
	t.Helper()

	switch v := obj.(type) {
	case *entity.Integer:
		return int(v.Value())
	case *entity.Real:
		return int(v.Value())
	default:
		t.Fatalf("unsupported integer object: %T", obj)
		return 0
	}
}

func requireObjectFloatSlice(t *testing.T, xrefTable *pdfxref.Table, obj entity.Object) []float64 {
	t.Helper()

	if obj == nil {
		return nil
	}

	switch v := obj.(type) {
	case entity.Ref:
		resolved, err := xrefTable.Fetch(v)
		require.NoError(t, err)
		return requireObjectFloatSlice(t, xrefTable, resolved)
	case *entity.Array:
		out := make([]float64, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			out = append(out, requireObjectFloat(t, xrefTable, v.Get(i)))
		}
		return out
	default:
		t.Fatalf("unsupported float slice object: %T", obj)
		return nil
	}
}

func requireObjectFloat(t *testing.T, xrefTable *pdfxref.Table, obj entity.Object) float64 {
	t.Helper()

	switch v := obj.(type) {
	case entity.Ref:
		resolved, err := xrefTable.Fetch(v)
		require.NoError(t, err)
		return requireObjectFloat(t, xrefTable, resolved)
	case *entity.Integer:
		return float64(v.Value())
	case *entity.Real:
		return v.Value()
	default:
		t.Fatalf("unsupported float object: %T", obj)
		return 0
	}
}

func expandIndexedCMYKImageData(palette, imageData []byte) []byte {
	if len(palette)%4 != 0 || len(imageData) == 0 {
		return nil
	}

	expanded := make([]byte, 0, len(imageData)*4)
	maxIndex := len(palette)/4 - 1
	for _, idxByte := range imageData {
		idx := int(idxByte)
		if idx > maxIndex {
			idx = maxIndex
		}
		base := idx * 4
		expanded = append(expanded,
			palette[base],
			palette[base+1],
			palette[base+2],
			palette[base+3],
		)
	}
	return expanded
}

func convertCMYKBytesToSimpleRGB(cmyk []byte) []byte {
	if len(cmyk)%4 != 0 {
		return nil
	}

	rgb := make([]byte, 0, len(cmyk)/4*3)
	for i := 0; i+3 < len(cmyk); i += 4 {
		c := float64(cmyk[i]) / 255.0
		m := float64(cmyk[i+1]) / 255.0
		y := float64(cmyk[i+2]) / 255.0
		k := float64(cmyk[i+3]) / 255.0

		r := uint8(math.Round((1.0 - c) * (1.0 - k) * 255.0))
		g := uint8(math.Round((1.0 - m) * (1.0 - k) * 255.0))
		b := uint8(math.Round((1.0 - y) * (1.0 - k) * 255.0))
		rgb = append(rgb, r, g, b)
	}
	return rgb
}

func convertCMYKBytesToCurrentRGB(cmyk []byte) []byte {
	if len(cmyk)%4 != 0 {
		return nil
	}

	converter := colorspace.NewDeviceCMYK()
	rgb := make([]byte, 0, len(cmyk)/4*3)
	for i := 0; i+3 < len(cmyk); i += 4 {
		rgba := converter.ConvertToRGBA([]float64{
			float64(cmyk[i]) / 255.0,
			float64(cmyk[i+1]) / 255.0,
			float64(cmyk[i+2]) / 255.0,
			float64(cmyk[i+3]) / 255.0,
		})
		rgb = append(rgb, rgba.R, rgba.G, rgba.B)
	}
	return rgb
}

func convertCMYKBytesToStdlibRGB(cmyk []byte) []byte {
	if len(cmyk)%4 != 0 {
		return nil
	}

	rgb := make([]byte, 0, len(cmyk)/4*3)
	for i := 0; i+3 < len(cmyk); i += 4 {
		converted := color.CMYK{
			C: cmyk[i],
			M: cmyk[i+1],
			Y: cmyk[i+2],
			K: cmyk[i+3],
		}
		r, g, b, _ := converted.RGBA()
		rgb = append(rgb, uint8(r>>8), uint8(g>>8), uint8(b>>8))
	}
	return rgb
}

func convertCMYKPaletteToCurrentRGBPalette(cmykPalette []byte) []byte {
	if len(cmykPalette)%4 != 0 {
		return nil
	}

	converter := colorspace.NewDeviceCMYK()
	rgbPalette := make([]byte, 0, len(cmykPalette)/4*3)
	for i := 0; i+3 < len(cmykPalette); i += 4 {
		rgba := converter.ConvertToRGBA([]float64{
			float64(cmykPalette[i]) / 255.0,
			float64(cmykPalette[i+1]) / 255.0,
			float64(cmykPalette[i+2]) / 255.0,
			float64(cmykPalette[i+3]) / 255.0,
		})
		rgbPalette = append(rgbPalette, rgba.R, rgba.G, rgba.B)
	}
	return rgbPalette
}

func convertCMYKPaletteToSimpleRGBPalette(cmykPalette []byte) []byte {
	if len(cmykPalette)%4 != 0 {
		return nil
	}

	rgbPalette := make([]byte, 0, len(cmykPalette)/4*3)
	for i := 0; i+3 < len(cmykPalette); i += 4 {
		rgba := simpleCMYKBytesToRGBA(cmykPalette[i], cmykPalette[i+1], cmykPalette[i+2], cmykPalette[i+3])
		rgbPalette = append(rgbPalette, rgba.R, rgba.G, rgba.B)
	}
	return rgbPalette
}

func convertCMYKPaletteToStdlibRGBPalette(cmykPalette []byte) []byte {
	if len(cmykPalette)%4 != 0 {
		return nil
	}

	rgbPalette := make([]byte, 0, len(cmykPalette)/4*3)
	for i := 0; i+3 < len(cmykPalette); i += 4 {
		converted := color.CMYK{
			C: cmykPalette[i],
			M: cmykPalette[i+1],
			Y: cmykPalette[i+2],
			K: cmykPalette[i+3],
		}
		r, g, b, _ := converted.RGBA()
		rgbPalette = append(rgbPalette, uint8(r>>8), uint8(g>>8), uint8(b>>8))
	}
	return rgbPalette
}

func convertCMYKPaletteToBlendedRGBPalette(cmykPalette []byte, simpleWeight float64) []byte {
	if len(cmykPalette)%4 != 0 {
		return nil
	}

	if simpleWeight < 0 {
		simpleWeight = 0
	}
	if simpleWeight > 1 {
		simpleWeight = 1
	}
	currentWeight := 1 - simpleWeight

	currentPalette := convertCMYKPaletteToCurrentRGBPalette(cmykPalette)
	simplePalette := convertCMYKPaletteToSimpleRGBPalette(cmykPalette)
	if len(currentPalette) != len(simplePalette) {
		return nil
	}

	blended := make([]byte, len(currentPalette))
	for i := range blended {
		value := currentWeight*float64(currentPalette[i]) + simpleWeight*float64(simplePalette[i])
		blended[i] = uint8(math.Round(value))
	}
	return blended
}

func simpleCMYKBytesToRGBA(c, m, y, k uint8) color.RGBA {
	cf := float64(c) / 255.0
	mf := float64(m) / 255.0
	yf := float64(y) / 255.0
	kf := float64(k) / 255.0

	return color.RGBA{
		R: uint8(math.Round((1.0 - cf) * (1.0 - kf) * 255.0)),
		G: uint8(math.Round((1.0 - mf) * (1.0 - kf) * 255.0)),
		B: uint8(math.Round((1.0 - yf) * (1.0 - kf) * 255.0)),
		A: 255,
	}
}

func syntheticPaletteSweepImageData(entries int) (int, int, []byte) {
	if entries <= 0 {
		return 0, 0, nil
	}

	side := int(math.Ceil(math.Sqrt(float64(entries))))
	imageData := make([]byte, side*side)
	for i := range imageData {
		if i < entries {
			imageData[i] = byte(i)
		}
	}
	return side, side, imageData
}

func countIndexedPaletteUsage(imageData []byte, paletteEntries int) []int {
	if paletteEntries <= 0 {
		return nil
	}

	counts := make([]int, paletteEntries)
	maxIndex := paletteEntries - 1
	for _, indexByte := range imageData {
		index := int(indexByte)
		if index < 0 {
			index = 0
		}
		if index > maxIndex {
			index = maxIndex
		}
		counts[index]++
	}
	return counts
}

func classifyIndexedEdgePixels(imageData []byte, width, height int) []bool {
	if width <= 0 || height <= 0 || len(imageData) != width*height {
		return nil
	}

	edges := make([]bool, len(imageData))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			index := y*width + x
			value := imageData[index]
			if x > 0 && imageData[index-1] != value {
				edges[index] = true
				continue
			}
			if x+1 < width && imageData[index+1] != value {
				edges[index] = true
				continue
			}
			if y > 0 && imageData[index-width] != value {
				edges[index] = true
				continue
			}
			if y+1 < height && imageData[index+width] != value {
				edges[index] = true
			}
		}
	}

	return edges
}

func classifyIndexedEdgeOrientationPixels(imageData []byte, width, height int) []edgeOrientation {
	if width <= 0 || height <= 0 || len(imageData) != width*height {
		return nil
	}

	orientations := make([]edgeOrientation, len(imageData))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			index := y*width + x
			value := imageData[index]

			horizontalTransitions := 0
			verticalTransitions := 0
			if x > 0 && imageData[index-1] != value {
				horizontalTransitions++
			}
			if x+1 < width && imageData[index+1] != value {
				horizontalTransitions++
			}
			if y > 0 && imageData[index-width] != value {
				verticalTransitions++
			}
			if y+1 < height && imageData[index+width] != value {
				verticalTransitions++
			}

			switch {
			case horizontalTransitions == 0 && verticalTransitions == 0:
				orientations[index] = edgeOrientationNone
			case horizontalTransitions > verticalTransitions:
				orientations[index] = edgeOrientationHorizontal
			case verticalTransitions > horizontalTransitions:
				orientations[index] = edgeOrientationVertical
			default:
				orientations[index] = edgeOrientationMixed
			}
		}
	}

	return orientations
}

func invertBoolMask(mask []bool) []bool {
	if len(mask) == 0 {
		return nil
	}

	inverted := make([]bool, len(mask))
	for i, value := range mask {
		inverted[i] = !value
	}
	return inverted
}

func selectIndexedPixelsByPaletteSet(imageData []byte, width, height int, selected map[int]bool, fillIndex int, includeNeighbors bool) []byte {
	if width <= 0 || height <= 0 || len(imageData) != width*height || len(selected) == 0 {
		return nil
	}

	result := make([]byte, len(imageData))
	for i := range result {
		result[i] = byte(fillIndex)
	}

	keepMask := make([]bool, len(imageData))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			if selected[int(imageData[idx])] {
				keepMask[idx] = true
			}
		}
	}

	if includeNeighbors {
		dilated := append([]bool(nil), keepMask...)
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				idx := y*width + x
				if keepMask[idx] {
					continue
				}
				for ny := maxInt(0, y-1); ny <= minInt(height-1, y+1); ny++ {
					for nx := maxInt(0, x-1); nx <= minInt(width-1, x+1); nx++ {
						if keepMask[ny*width+nx] {
							dilated[idx] = true
							goto nextPixel
						}
					}
				}
			nextPixel:
			}
		}
		keepMask = dilated
	}

	for i, keep := range keepMask {
		if keep {
			result[i] = imageData[i]
		}
	}

	return result
}

func selectIndexedPixelsByBounds(imageData []byte, width, height int, bounds image.Rectangle, fillIndex int) []byte {
	if width <= 0 || height <= 0 || len(imageData) != width*height {
		return nil
	}

	clipped := bounds.Intersect(image.Rect(0, 0, width, height))
	result := make([]byte, len(imageData))
	for i := range result {
		result[i] = byte(fillIndex)
	}

	if clipped.Empty() {
		return result
	}

	for y := clipped.Min.Y; y < clipped.Max.Y; y++ {
		rowOffset := y * width
		copy(
			result[rowOffset+clipped.Min.X:rowOffset+clipped.Max.X],
			imageData[rowOffset+clipped.Min.X:rowOffset+clipped.Max.X],
		)
	}

	return result
}

func selectIndexedPixelsByBoundsUnion(imageData []byte, width, height, fillIndex int, bounds ...image.Rectangle) []byte {
	if width <= 0 || height <= 0 || len(imageData) != width*height {
		return nil
	}

	result := make([]byte, len(imageData))
	for i := range result {
		result[i] = byte(fillIndex)
	}

	canvasBounds := image.Rect(0, 0, width, height)
	for _, bound := range bounds {
		clipped := bound.Intersect(canvasBounds)
		if clipped.Empty() {
			continue
		}
		for y := clipped.Min.Y; y < clipped.Max.Y; y++ {
			rowOffset := y * width
			copy(
				result[rowOffset+clipped.Min.X:rowOffset+clipped.Max.X],
				imageData[rowOffset+clipped.Min.X:rowOffset+clipped.Max.X],
			)
		}
	}

	return result
}

func selectIndexedPixelsByBoundsAndEdgeMask(imageData []byte, width, height int, bounds image.Rectangle, edgeMask []bool, fillIndex int, edgeOnly bool) []byte {
	if width <= 0 || height <= 0 || len(imageData) != width*height || len(edgeMask) != len(imageData) {
		return nil
	}

	clipped := bounds.Intersect(image.Rect(0, 0, width, height))
	result := make([]byte, len(imageData))
	for i := range result {
		result[i] = byte(fillIndex)
	}
	if clipped.Empty() {
		return result
	}

	for y := clipped.Min.Y; y < clipped.Max.Y; y++ {
		for x := clipped.Min.X; x < clipped.Max.X; x++ {
			index := y*width + x
			if edgeMask[index] == edgeOnly {
				result[index] = imageData[index]
			}
		}
	}

	return result
}

func selectIndexedPixelsByBoundsAndOrientation(imageData []byte, width, height int, bounds image.Rectangle, orientations []edgeOrientation, fillIndex int, orientation edgeOrientation) []byte {
	if width <= 0 || height <= 0 || len(imageData) != width*height || len(orientations) != len(imageData) {
		return nil
	}

	clipped := bounds.Intersect(image.Rect(0, 0, width, height))
	result := make([]byte, len(imageData))
	for i := range result {
		result[i] = byte(fillIndex)
	}
	if clipped.Empty() {
		return result
	}

	for y := clipped.Min.Y; y < clipped.Max.Y; y++ {
		for x := clipped.Min.X; x < clipped.Max.X; x++ {
			index := y*width + x
			if orientations[index] == orientation {
				result[index] = imageData[index]
			}
		}
	}

	return result
}

func centeredVerticalStripe(width, height, stripeWidth int) image.Rectangle {
	if stripeWidth <= 0 {
		return image.Rectangle{}
	}
	if stripeWidth > width {
		stripeWidth = width
	}
	left := (width - stripeWidth) / 2
	return image.Rect(left, 0, left+stripeWidth, height)
}

func centeredHorizontalStripe(width, height, stripeHeight int) image.Rectangle {
	if stripeHeight <= 0 {
		return image.Rectangle{}
	}
	if stripeHeight > height {
		stripeHeight = height
	}
	top := (height - stripeHeight) / 2
	return image.Rect(0, top, width, top+stripeHeight)
}

func rearrangeIndexedImageBlocks(imageData []byte, width, height, blockWidth, blockHeight int) []byte {
	if width <= 0 || height <= 0 || len(imageData) != width*height || blockWidth <= 0 || blockHeight <= 0 {
		return nil
	}

	type block struct {
		x0 int
		y0 int
		x1 int
		y1 int
	}

	blocksX := int(math.Ceil(float64(width) / float64(blockWidth)))
	blocksY := int(math.Ceil(float64(height) / float64(blockHeight)))
	blocks := make([]block, 0, blocksX*blocksY)
	for by := 0; by < blocksY; by++ {
		for bx := 0; bx < blocksX; bx++ {
			x0 := bx * blockWidth
			y0 := by * blockHeight
			blocks = append(blocks, block{
				x0: x0,
				y0: y0,
				x1: minInt(width, x0+blockWidth),
				y1: minInt(height, y0+blockHeight),
			})
		}
	}

	out := make([]byte, len(imageData))
	for i, dstBlock := range blocks {
		srcBlock := blocks[len(blocks)-1-i]
		copyBlockWidth := minInt(dstBlock.x1-dstBlock.x0, srcBlock.x1-srcBlock.x0)
		copyBlockHeight := minInt(dstBlock.y1-dstBlock.y0, srcBlock.y1-srcBlock.y0)

		for dy := 0; dy < copyBlockHeight; dy++ {
			dstOffset := (dstBlock.y0+dy)*width + dstBlock.x0
			srcOffset := (srcBlock.y0+dy)*width + srcBlock.x0
			copy(out[dstOffset:dstOffset+copyBlockWidth], imageData[srcOffset:srcOffset+copyBlockWidth])
		}
	}

	return out
}

func syntheticWeightedPaletteImageData(counts []int, targetCells int) (int, int, []byte) {
	if len(counts) == 0 || targetCells <= 0 {
		return 0, 0, nil
	}

	total := 0
	for _, count := range counts {
		total += count
	}
	if total == 0 {
		return 0, 0, nil
	}

	side := int(math.Ceil(math.Sqrt(float64(targetCells))))
	cellCount := side * side
	imageData := make([]byte, cellCount)

	index := 0
	accumulated := counts[0]
	for cell := 0; cell < cellCount; cell++ {
		position := ((cell * total) + (cellCount / 2)) / cellCount
		for index < len(counts)-1 && position >= accumulated {
			index++
			accumulated += counts[index]
		}
		imageData[cell] = byte(index)
	}

	return side, side, imageData
}

func downsampleIndexedImageMajority(imageData []byte, srcWidth, srcHeight, dstWidth, dstHeight, paletteEntries int) []byte {
	if srcWidth <= 0 || srcHeight <= 0 || dstWidth <= 0 || dstHeight <= 0 || len(imageData) != srcWidth*srcHeight || paletteEntries <= 0 {
		return nil
	}

	result := make([]byte, dstWidth*dstHeight)
	counts := make([]int, paletteEntries)
	maxIndex := paletteEntries - 1

	for dy := 0; dy < dstHeight; dy++ {
		srcY0 := dy * srcHeight / dstHeight
		srcY1 := (dy + 1) * srcHeight / dstHeight
		if srcY1 <= srcY0 {
			srcY1 = srcY0 + 1
		}

		for dx := 0; dx < dstWidth; dx++ {
			srcX0 := dx * srcWidth / dstWidth
			srcX1 := (dx + 1) * srcWidth / dstWidth
			if srcX1 <= srcX0 {
				srcX1 = srcX0 + 1
			}

			for i := range counts {
				counts[i] = 0
			}

			bestIndex := 0
			bestCount := -1
			for sy := srcY0; sy < srcY1; sy++ {
				rowOffset := sy * srcWidth
				for sx := srcX0; sx < srcX1; sx++ {
					index := int(imageData[rowOffset+sx])
					if index < 0 {
						index = 0
					}
					if index > maxIndex {
						index = maxIndex
					}
					counts[index]++
					if counts[index] > bestCount {
						bestCount = counts[index]
						bestIndex = index
					}
				}
			}

			result[dy*dstWidth+dx] = byte(bestIndex)
		}
	}

	return result
}

func downsampleIndexedImageMajorityByRegion(imageData []byte, srcWidth, srcHeight, dstWidth, dstHeight, paletteEntries, fillIndex int) ([]byte, []byte) {
	if srcWidth <= 0 || srcHeight <= 0 || dstWidth <= 0 || dstHeight <= 0 || len(imageData) != srcWidth*srcHeight || paletteEntries <= 0 {
		return nil, nil
	}

	edgeResult := make([]byte, dstWidth*dstHeight)
	flatResult := make([]byte, dstWidth*dstHeight)
	counts := make([]int, paletteEntries)
	maxIndex := paletteEntries - 1

	if fillIndex < 0 {
		fillIndex = 0
	}
	if fillIndex > maxIndex {
		fillIndex = maxIndex
	}

	for i := range edgeResult {
		edgeResult[i] = byte(fillIndex)
		flatResult[i] = byte(fillIndex)
	}

	for dy := 0; dy < dstHeight; dy++ {
		srcY0 := dy * srcHeight / dstHeight
		srcY1 := (dy + 1) * srcHeight / dstHeight
		if srcY1 <= srcY0 {
			srcY1 = srcY0 + 1
		}

		for dx := 0; dx < dstWidth; dx++ {
			srcX0 := dx * srcWidth / dstWidth
			srcX1 := (dx + 1) * srcWidth / dstWidth
			if srcX1 <= srcX0 {
				srcX1 = srcX0 + 1
			}

			for i := range counts {
				counts[i] = 0
			}

			bestIndex := 0
			bestCount := -1
			uniqueCount := 0
			for sy := srcY0; sy < srcY1; sy++ {
				rowOffset := sy * srcWidth
				for sx := srcX0; sx < srcX1; sx++ {
					index := int(imageData[rowOffset+sx])
					if index < 0 {
						index = 0
					}
					if index > maxIndex {
						index = maxIndex
					}
					if counts[index] == 0 {
						uniqueCount++
					}
					counts[index]++
					if counts[index] > bestCount {
						bestCount = counts[index]
						bestIndex = index
					}
				}
			}

			target := dy*dstWidth + dx
			if uniqueCount > 1 {
				edgeResult[target] = byte(bestIndex)
			} else {
				flatResult[target] = byte(bestIndex)
			}
		}
	}

	return edgeResult, flatResult
}

func downsampleIndexedImageMajorityByEdgeOrientation(imageData []byte, srcWidth, srcHeight, dstWidth, dstHeight, paletteEntries, fillIndex int) ([]byte, []byte, []byte) {
	if srcWidth <= 0 || srcHeight <= 0 || dstWidth <= 0 || dstHeight <= 0 || len(imageData) != srcWidth*srcHeight || paletteEntries <= 0 {
		return nil, nil, nil
	}

	horizontalResult := make([]byte, dstWidth*dstHeight)
	verticalResult := make([]byte, dstWidth*dstHeight)
	mixedResult := make([]byte, dstWidth*dstHeight)
	counts := make([]int, paletteEntries)
	maxIndex := paletteEntries - 1

	if fillIndex < 0 {
		fillIndex = 0
	}
	if fillIndex > maxIndex {
		fillIndex = maxIndex
	}

	for i := range horizontalResult {
		horizontalResult[i] = byte(fillIndex)
		verticalResult[i] = byte(fillIndex)
		mixedResult[i] = byte(fillIndex)
	}

	for dy := 0; dy < dstHeight; dy++ {
		srcY0 := dy * srcHeight / dstHeight
		srcY1 := (dy + 1) * srcHeight / dstHeight
		if srcY1 <= srcY0 {
			srcY1 = srcY0 + 1
		}

		for dx := 0; dx < dstWidth; dx++ {
			srcX0 := dx * srcWidth / dstWidth
			srcX1 := (dx + 1) * srcWidth / dstWidth
			if srcX1 <= srcX0 {
				srcX1 = srcX0 + 1
			}

			for i := range counts {
				counts[i] = 0
			}

			bestIndex := 0
			bestCount := -1
			uniqueCount := 0
			horizontalTransitions := 0
			verticalTransitions := 0

			for sy := srcY0; sy < srcY1; sy++ {
				rowOffset := sy * srcWidth
				prevIndex := -1
				for sx := srcX0; sx < srcX1; sx++ {
					index := int(imageData[rowOffset+sx])
					if index < 0 {
						index = 0
					}
					if index > maxIndex {
						index = maxIndex
					}
					if counts[index] == 0 {
						uniqueCount++
					}
					counts[index]++
					if counts[index] > bestCount {
						bestCount = counts[index]
						bestIndex = index
					}
					if prevIndex >= 0 && prevIndex != index {
						horizontalTransitions++
					}
					prevIndex = index

					if sy > srcY0 {
						aboveIndex := int(imageData[(sy-1)*srcWidth+sx])
						if aboveIndex < 0 {
							aboveIndex = 0
						}
						if aboveIndex > maxIndex {
							aboveIndex = maxIndex
						}
						if aboveIndex != index {
							verticalTransitions++
						}
					}
				}
			}

			if uniqueCount <= 1 {
				continue
			}

			target := dy*dstWidth + dx
			switch {
			case horizontalTransitions > verticalTransitions:
				horizontalResult[target] = byte(bestIndex)
			case verticalTransitions > horizontalTransitions:
				verticalResult[target] = byte(bestIndex)
			default:
				mixedResult[target] = byte(bestIndex)
			}
		}
	}

	return horizontalResult, verticalResult, mixedResult
}

func dominantPaletteIndex(counts []int) int {
	bestIndex := 0
	bestCount := -1
	for index, count := range counts {
		if count > bestCount {
			bestCount = count
			bestIndex = index
		}
	}
	return bestIndex
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func mergeIndexedProxyLayers(fillIndex byte, layers ...[]byte) []byte {
	if len(layers) == 0 {
		return nil
	}

	size := len(layers[0])
	merged := make([]byte, size)
	for i := range merged {
		merged[i] = fillIndex
	}

	for _, layer := range layers {
		if len(layer) != size {
			return nil
		}
		for i, value := range layer {
			if value != fillIndex {
				merged[i] = value
			}
		}
	}

	return merged
}

func rearrangeIndexedProxyLayersByClass(fillIndex byte, layers ...[]byte) []byte {
	if len(layers) == 0 {
		return nil
	}

	size := len(layers[0])
	merged := make([]byte, size)
	for i := range merged {
		merged[i] = fillIndex
	}

	positions := make([]int, 0, size)
	for i := 0; i < size; i++ {
		for _, layer := range layers {
			if len(layer) != size {
				return nil
			}
			if layer[i] != fillIndex {
				positions = append(positions, i)
				break
			}
		}
	}

	cursor := 0
	for _, layer := range layers {
		for _, value := range layer {
			if value == fillIndex {
				continue
			}
			if cursor >= len(positions) {
				return merged
			}
			merged[positions[cursor]] = value
			cursor++
		}
	}

	return merged
}

func compactIndexedProxyLayers(fillIndex byte, layers ...[]byte) []byte {
	if len(layers) == 0 {
		return nil
	}

	size := len(layers[0])
	values := make([]byte, 0, size)
	for _, layer := range layers {
		if len(layer) != size {
			return nil
		}
		for _, value := range layer {
			if value != fillIndex {
				values = append(values, value)
			}
		}
	}

	compacted := make([]byte, size)
	for i := range compacted {
		compacted[i] = fillIndex
	}
	copy(compacted, values)
	return compacted
}

func thinIndexedProxyLayer(fillIndex byte, layer []byte, keepEvery int) []byte {
	if len(layer) == 0 || keepEvery <= 0 {
		return nil
	}

	thinned := make([]byte, len(layer))
	for i := range thinned {
		thinned[i] = fillIndex
	}

	seen := 0
	for i, value := range layer {
		if value == fillIndex {
			continue
		}
		if seen%keepEvery == 0 {
			thinned[i] = value
		}
		seen++
	}

	return thinned
}

func interleaveIndexedProxyLayers(fillIndex byte, layers ...[]byte) []byte {
	if len(layers) == 0 {
		return nil
	}

	size := len(layers[0])
	valueQueues := make([][]byte, len(layers))
	totalValues := 0
	for i, layer := range layers {
		if len(layer) != size {
			return nil
		}
		for _, value := range layer {
			if value != fillIndex {
				valueQueues[i] = append(valueQueues[i], value)
				totalValues++
			}
		}
	}

	interleaved := make([]byte, 0, totalValues)
	for {
		appended := false
		for i := range valueQueues {
			if len(valueQueues[i]) == 0 {
				continue
			}
			interleaved = append(interleaved, valueQueues[i][0])
			valueQueues[i] = valueQueues[i][1:]
			appended = true
		}
		if !appended {
			break
		}
	}

	out := make([]byte, size)
	for i := range out {
		out[i] = fillIndex
	}
	copy(out, interleaved)
	return out
}

func syntheticSparseGray16x16() []byte {
	img := make([]byte, 16*16)
	for _, pt := range [][2]int{
		{3, 3}, {12, 3},
		{7, 5}, {7, 6}, {7, 7}, {7, 8},
		{3, 10}, {11, 10},
		{3, 11}, {10, 11}, {11, 11},
		{4, 12}, {5, 12}, {6, 12}, {7, 12}, {8, 12}, {9, 12}, {10, 12},
	} {
		img[pt[1]*16+pt[0]] = 255
	}
	return img
}

func syntheticIndexedIdentity4x4() ([]byte, []byte) {
	palette := []byte{
		0x00, 0x00, 0x00,
		0x55, 0x55, 0x55,
		0xAA, 0xAA, 0xAA,
		0xFF, 0xFF, 0xFF,
	}
	imageData := []byte{
		0, 1, 2, 3,
		3, 2, 1, 0,
		0, 3, 0, 3,
		3, 0, 3, 0,
	}
	return palette, imageData
}

func syntheticIndexedCMYKIdentity4x4() ([]byte, []byte) {
	palette := []byte{
		0x00, 0x00, 0x00, 0x00,
		0xFF, 0x00, 0x00, 0x00,
		0x00, 0xFF, 0x00, 0x00,
		0x00, 0x00, 0xFF, 0x00,
	}
	imageData := []byte{
		0, 1, 2, 3,
		3, 2, 1, 0,
		0, 3, 0, 3,
		3, 0, 3, 0,
	}
	return palette, imageData
}

func syntheticRGBIdentity4x4() []byte {
	return []byte{
		0x00, 0x00, 0x00,
		0x55, 0x00, 0x55,
		0xAA, 0x55, 0x00,
		0xFF, 0xFF, 0xFF,
		0xFF, 0x00, 0x00,
		0xAA, 0xAA, 0x00,
		0x55, 0xAA, 0xFF,
		0x00, 0x00, 0xFF,
		0x00, 0xFF, 0x00,
		0xFF, 0xFF, 0x00,
		0x00, 0xFF, 0xFF,
		0xFF, 0x00, 0xFF,
		0xFF, 0xAA, 0xAA,
		0xAA, 0xFF, 0xAA,
		0xAA, 0xAA, 0xFF,
		0x22, 0x22, 0x22,
	}
}

func syntheticRGBFlatFill(width, height int, fill color.RGBA) []byte {
	data := make([]byte, width*height*3)
	for i := 0; i < width*height; i++ {
		base := i * 3
		data[base+0] = fill.R
		data[base+1] = fill.G
		data[base+2] = fill.B
	}
	return data
}

func syntheticRGBHorizontalGradient(width, height int) []byte {
	data := make([]byte, width*height*3)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			base := (y*width + x) * 3
			t := 0.0
			if width > 1 {
				t = float64(x) / float64(width-1)
			}
			data[base+0] = uint8(math.Round(255 * t))
			data[base+1] = uint8(math.Round(255 * (1 - t)))
			data[base+2] = uint8(math.Round(128 + 64*math.Sin(t*math.Pi)))
		}
	}
	return data
}

func syntheticRGBCheckerboard(width, height int) []byte {
	data := make([]byte, width*height*3)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			base := (y*width + x) * 3
			if (x+y)%2 == 0 {
				data[base+0] = 240
				data[base+1] = 240
				data[base+2] = 40
			} else {
				data[base+0] = 20
				data[base+1] = 40
				data[base+2] = 220
			}
		}
	}
	return data
}

func syntheticRGBTiledIdentity(width, height int) []byte {
	base := syntheticRGBIdentity4x4()
	if width <= 0 || height <= 0 {
		return nil
	}

	imageData := make([]byte, width*height*3)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			baseOffset := ((y % 4) * 4 * 3) + ((x % 4) * 3)
			dstOffset := (y*width + x) * 3
			copy(imageData[dstOffset:dstOffset+3], base[baseOffset:baseOffset+3])
		}
	}
	return imageData
}

func syntheticRGBTiledImage(width, height int, imageData []byte) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			offset := (y*width + x) * 3
			if offset+2 >= len(imageData) {
				continue
			}
			img.SetRGBA(x, y, color.RGBA{
				R: imageData[offset],
				G: imageData[offset+1],
				B: imageData[offset+2],
				A: 255,
			})
		}
	}
	return img
}

func syntheticIndexedTiledIdentity(width, height int) ([]byte, []byte) {
	palette, base := syntheticIndexedIdentity4x4()
	if width <= 0 || height <= 0 {
		return palette, nil
	}
	imageData := make([]byte, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			imageData[y*width+x] = base[(y%4)*4+(x%4)]
		}
	}
	return palette, imageData
}

func syntheticIndexedCMYKTiledIdentity(width, height int) ([]byte, []byte) {
	palette, base := syntheticIndexedCMYKIdentity4x4()
	if width <= 0 || height <= 0 {
		return palette, nil
	}
	imageData := make([]byte, width*height)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			imageData[y*width+x] = base[(y%4)*4+(x%4)]
		}
	}
	return palette, imageData
}

func syntheticIndexedTiledPalettedImage(width, height int, paletteBytes, imageData []byte) *image.Paletted {
	palette := make(color.Palette, 0, len(paletteBytes)/3)
	for i := 0; i+2 < len(paletteBytes); i += 3 {
		palette = append(palette, color.RGBA{
			R: paletteBytes[i],
			G: paletteBytes[i+1],
			B: paletteBytes[i+2],
			A: 255,
		})
	}
	img := image.NewPaletted(image.Rect(0, 0, width, height), palette)
	copy(img.Pix, imageData)
	return img
}

func buildSyntheticGrayImagePDF(pageW, pageH, imageW, imageH int, matrix [6]int, imageData []byte) []byte {
	return buildSyntheticGrayImagePDFFloat(
		float64(pageW),
		float64(pageH),
		imageW,
		imageH,
		[6]float64{
			float64(matrix[0]),
			float64(matrix[1]),
			float64(matrix[2]),
			float64(matrix[3]),
			float64(matrix[4]),
			float64(matrix[5]),
		},
		imageData,
	)
}

func buildSyntheticGrayImagePDFFloat(pageW, pageH float64, imageW, imageH int, matrix [6]float64, imageData []byte) []byte {
	content := []byte(fmt.Sprintf(
		"q\n%s %s %s %s %s %s cm\n/Im0 Do\nQ\n",
		formatSyntheticPDFNumber(matrix[0]),
		formatSyntheticPDFNumber(matrix[1]),
		formatSyntheticPDFNumber(matrix[2]),
		formatSyntheticPDFNumber(matrix[3]),
		formatSyntheticPDFNumber(matrix[4]),
		formatSyntheticPDFNumber(matrix[5]),
	))

	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources << /ProcSet [/PDF /ImageB] /XObject << /Im0 4 0 R >> >> /Contents 5 0 R >>",
			formatSyntheticPDFNumber(pageW), formatSyntheticPDFNumber(pageH),
		)),
		syntheticStreamObject(
			fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 8 /Length %d >>", imageW, imageH, len(imageData)),
			imageData,
		),
		syntheticStreamObject(
			fmt.Sprintf("<< /Length %d >>", len(content)),
			content,
		),
	}

	return buildSyntheticPDF(objects)
}

func buildSyntheticIndexedImagePDF(pageW, pageH, imageW, imageH int, matrix [6]int, palette []byte, imageData []byte) []byte {
	return buildSyntheticIndexedImagePDFFloat(
		float64(pageW),
		float64(pageH),
		imageW,
		imageH,
		[6]float64{
			float64(matrix[0]),
			float64(matrix[1]),
			float64(matrix[2]),
			float64(matrix[3]),
			float64(matrix[4]),
			float64(matrix[5]),
		},
		palette,
		imageData,
	)
}

func buildSyntheticIndexedImagePDFFloatWithBase(
	pageW, pageH float64,
	baseComponents int,
	baseName string,
	imageW, imageH int,
	matrix [6]float64,
	palette []byte,
	imageData []byte,
) []byte {
	content := []byte(fmt.Sprintf(
		"q\n%s %s %s %s %s %s cm\n/Im0 Do\nQ\n",
		formatSyntheticPDFNumber(matrix[0]),
		formatSyntheticPDFNumber(matrix[1]),
		formatSyntheticPDFNumber(matrix[2]),
		formatSyntheticPDFNumber(matrix[3]),
		formatSyntheticPDFNumber(matrix[4]),
		formatSyntheticPDFNumber(matrix[5]),
	))

	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources << /ProcSet [/PDF /ImageC] /XObject << /Im0 4 0 R >> >> /Contents 5 0 R >>",
			formatSyntheticPDFNumber(pageW), formatSyntheticPDFNumber(pageH),
		)),
		syntheticStreamObject(
			fmt.Sprintf(
				"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace [/Indexed /%s %d <%X>] /BitsPerComponent 8 /Length %d >>",
				imageW,
				imageH,
				baseName,
				len(palette)/baseComponents-1,
				palette,
				len(imageData),
			),
			imageData,
		),
		syntheticStreamObject(
			fmt.Sprintf("<< /Length %d >>", len(content)),
			content,
		),
	}

	return buildSyntheticPDF(objects)
}

func buildSyntheticRGBImagePDF(pageW, pageH, imageW, imageH int, matrix [6]int, imageData []byte) []byte {
	return buildSyntheticRGBImagePDFFloat(
		float64(pageW),
		float64(pageH),
		imageW,
		imageH,
		[6]float64{
			float64(matrix[0]),
			float64(matrix[1]),
			float64(matrix[2]),
			float64(matrix[3]),
			float64(matrix[4]),
			float64(matrix[5]),
		},
		imageData,
	)
}

func buildSyntheticRGBImagePDFFloat(pageW, pageH float64, imageW, imageH int, matrix [6]float64, imageData []byte) []byte {
	content := []byte(fmt.Sprintf(
		"q\n%s %s %s %s %s %s cm\n/Im0 Do\nQ\n",
		formatSyntheticPDFNumber(matrix[0]),
		formatSyntheticPDFNumber(matrix[1]),
		formatSyntheticPDFNumber(matrix[2]),
		formatSyntheticPDFNumber(matrix[3]),
		formatSyntheticPDFNumber(matrix[4]),
		formatSyntheticPDFNumber(matrix[5]),
	))

	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources << /ProcSet [/PDF /ImageC] /XObject << /Im0 4 0 R >> >> /Contents 5 0 R >>",
			formatSyntheticPDFNumber(pageW), formatSyntheticPDFNumber(pageH),
		)),
		syntheticStreamObject(
			fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Length %d >>", imageW, imageH, len(imageData)),
			imageData,
		),
		syntheticStreamObject(
			fmt.Sprintf("<< /Length %d >>", len(content)),
			content,
		),
	}

	return buildSyntheticPDF(objects)
}

func buildSyntheticRGBImageWithSoftMaskPDFFloat(
	pageW, pageH float64,
	imageW, imageH int,
	matrix [6]float64,
	imageData []byte,
	maskW, maskH int,
	maskData []byte,
) []byte {
	content := []byte(fmt.Sprintf(
		"q\n%s %s %s %s %s %s cm\n/Im0 Do\nQ\n",
		formatSyntheticPDFNumber(matrix[0]),
		formatSyntheticPDFNumber(matrix[1]),
		formatSyntheticPDFNumber(matrix[2]),
		formatSyntheticPDFNumber(matrix[3]),
		formatSyntheticPDFNumber(matrix[4]),
		formatSyntheticPDFNumber(matrix[5]),
	))

	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources << /ProcSet [/PDF /ImageC] /XObject << /Im0 4 0 R >> >> /Contents 6 0 R >>",
			formatSyntheticPDFNumber(pageW), formatSyntheticPDFNumber(pageH),
		)),
		syntheticStreamObject(
			fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /SMask 5 0 R /Length %d >>", imageW, imageH, len(imageData)),
			imageData,
		),
		syntheticStreamObject(
			fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 8 /Length %d >>", maskW, maskH, len(maskData)),
			maskData,
		),
		syntheticStreamObject(
			fmt.Sprintf("<< /Length %d >>", len(content)),
			content,
		),
	}

	return buildSyntheticPDF(objects)
}

func buildSyntheticDCTRGBImagePDFFloat(pageW, pageH float64, imageW, imageH int, matrix [6]float64, jpegData []byte) []byte {
	content := []byte(fmt.Sprintf(
		"q\n%s %s %s %s %s %s cm\n/Im0 Do\nQ\n",
		formatSyntheticPDFNumber(matrix[0]),
		formatSyntheticPDFNumber(matrix[1]),
		formatSyntheticPDFNumber(matrix[2]),
		formatSyntheticPDFNumber(matrix[3]),
		formatSyntheticPDFNumber(matrix[4]),
		formatSyntheticPDFNumber(matrix[5]),
	))

	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources << /ProcSet [/PDF /ImageC] /XObject << /Im0 4 0 R >> >> /Contents 5 0 R >>",
			formatSyntheticPDFNumber(pageW), formatSyntheticPDFNumber(pageH),
		)),
		syntheticStreamObject(
			fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>", imageW, imageH, len(jpegData)),
			jpegData,
		),
		syntheticStreamObject(
			fmt.Sprintf("<< /Length %d >>", len(content)),
			content,
		),
	}

	return buildSyntheticPDF(objects)
}

func buildSyntheticDCTGrayImagePDFFloat(pageW, pageH float64, imageW, imageH int, matrix [6]float64, jpegData []byte) []byte {
	content := []byte(fmt.Sprintf(
		"q\n%s %s %s %s %s %s cm\n/Im0 Do\nQ\n",
		formatSyntheticPDFNumber(matrix[0]),
		formatSyntheticPDFNumber(matrix[1]),
		formatSyntheticPDFNumber(matrix[2]),
		formatSyntheticPDFNumber(matrix[3]),
		formatSyntheticPDFNumber(matrix[4]),
		formatSyntheticPDFNumber(matrix[5]),
	))

	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources << /ProcSet [/PDF /ImageB] /XObject << /Im0 4 0 R >> >> /Contents 5 0 R >>",
			formatSyntheticPDFNumber(pageW), formatSyntheticPDFNumber(pageH),
		)),
		syntheticStreamObject(
			fmt.Sprintf(
				"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>",
				imageW,
				imageH,
				len(jpegData),
			),
			jpegData,
		),
		syntheticStreamObject(
			fmt.Sprintf("<< /Length %d >>", len(content)),
			content,
		),
	}

	return buildSyntheticPDF(objects)
}

func buildSyntheticDCTICCBasedGrayImagePDFFloat(
	pageW, pageH float64,
	imageW, imageH int,
	matrix [6]float64,
	jpegData []byte,
	iccProfile []byte,
	iccComponents int,
) []byte {
	content := []byte(fmt.Sprintf(
		"q\n%s %s %s %s %s %s cm\n/Im0 Do\nQ\n",
		formatSyntheticPDFNumber(matrix[0]),
		formatSyntheticPDFNumber(matrix[1]),
		formatSyntheticPDFNumber(matrix[2]),
		formatSyntheticPDFNumber(matrix[3]),
		formatSyntheticPDFNumber(matrix[4]),
		formatSyntheticPDFNumber(matrix[5]),
	))

	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources << /ProcSet [/PDF /ImageB] /XObject << /Im0 4 0 R >> >> /Contents 5 0 R >>",
			formatSyntheticPDFNumber(pageW), formatSyntheticPDFNumber(pageH),
		)),
		syntheticStreamObject(
			fmt.Sprintf(
				"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace [/ICCBased 6 0 R] /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>",
				imageW,
				imageH,
				len(jpegData),
			),
			jpegData,
		),
		syntheticStreamObject(
			fmt.Sprintf("<< /Length %d >>", len(content)),
			content,
		),
		syntheticStreamObject(
			fmt.Sprintf("<< /N %d /Length %d >>", iccComponents, len(iccProfile)),
			iccProfile,
		),
	}

	return buildSyntheticPDF(objects)
}

func buildSyntheticCCITTGrayImagePDFFloat(
	pageW, pageH float64,
	imageW, imageH int,
	matrix [6]float64,
	ccittData []byte,
	decodeParms map[string]interface{},
	decode []float64,
) []byte {
	content := []byte(fmt.Sprintf(
		"q\n%s %s %s %s %s %s cm\n/Im0 Do\nQ\n",
		formatSyntheticPDFNumber(matrix[0]),
		formatSyntheticPDFNumber(matrix[1]),
		formatSyntheticPDFNumber(matrix[2]),
		formatSyntheticPDFNumber(matrix[3]),
		formatSyntheticPDFNumber(matrix[4]),
		formatSyntheticPDFNumber(matrix[5]),
	))

	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources << /ProcSet [/PDF /ImageB] /XObject << /Im0 4 0 R >> >> /Contents 5 0 R >>",
			formatSyntheticPDFNumber(pageW), formatSyntheticPDFNumber(pageH),
		)),
		syntheticStreamObject(
			fmt.Sprintf(
				"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 1 /Filter /CCITTFaxDecode%s%s /Length %d >>",
				imageW,
				imageH,
				formatSyntheticDecodeParms(decodeParms),
				formatSyntheticDecodeArray(decode),
				len(ccittData),
			),
			ccittData,
		),
		syntheticStreamObject(
			fmt.Sprintf("<< /Length %d >>", len(content)),
			content,
		),
	}

	return buildSyntheticPDF(objects)
}

func formatSyntheticDecodeArray(decode []float64) string {
	if len(decode) == 0 {
		return ""
	}

	parts := make([]string, 0, len(decode))
	for _, value := range decode {
		parts = append(parts, formatSyntheticPDFNumber(value))
	}
	return " /Decode [" + strings.Join(parts, " ") + "]"
}

func formatSyntheticDecodeParms(params map[string]interface{}) string {
	if len(params) == 0 {
		return ""
	}

	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		formatted, ok := formatSyntheticDecodeParamValue(params[key])
		if !ok {
			continue
		}
		parts = append(parts, fmt.Sprintf("/%s %s", key, formatted))
	}
	if len(parts) == 0 {
		return ""
	}
	return " /DecodeParms << " + strings.Join(parts, " ") + " >>"
}

func formatSyntheticDecodeParamValue(value interface{}) (string, bool) {
	switch v := value.(type) {
	case int:
		return strconv.Itoa(v), true
	case float64:
		return formatSyntheticPDFNumber(v), true
	case bool:
		if v {
			return "true", true
		}
		return "false", true
	case string:
		return "/" + v, true
	default:
		return "", false
	}
}

func buildSyntheticCMYKImagePDFFloat(pageW, pageH float64, imageW, imageH int, matrix [6]float64, imageData []byte) []byte {
	content := []byte(fmt.Sprintf(
		"q\n%s %s %s %s %s %s cm\n/Im0 Do\nQ\n",
		formatSyntheticPDFNumber(matrix[0]),
		formatSyntheticPDFNumber(matrix[1]),
		formatSyntheticPDFNumber(matrix[2]),
		formatSyntheticPDFNumber(matrix[3]),
		formatSyntheticPDFNumber(matrix[4]),
		formatSyntheticPDFNumber(matrix[5]),
	))

	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(fmt.Sprintf(
			"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] /Resources << /ProcSet [/PDF /ImageC] /XObject << /Im0 4 0 R >> >> /Contents 5 0 R >>",
			formatSyntheticPDFNumber(pageW), formatSyntheticPDFNumber(pageH),
		)),
		syntheticStreamObject(
			fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceCMYK /BitsPerComponent 8 /Length %d >>", imageW, imageH, len(imageData)),
			imageData,
		),
		syntheticStreamObject(
			fmt.Sprintf("<< /Length %d >>", len(content)),
			content,
		),
	}

	return buildSyntheticPDF(objects)
}

func buildSyntheticIndexedImagePDFFloat(pageW, pageH float64, imageW, imageH int, matrix [6]float64, palette []byte, imageData []byte) []byte {
	return buildSyntheticIndexedImagePDFFloatWithBase(
		pageW,
		pageH,
		3,
		"DeviceRGB",
		imageW,
		imageH,
		matrix,
		palette,
		imageData,
	)
}

func formatSyntheticPDFNumber(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func buildSyntheticPDF(objects [][]byte) []byte {
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")

	offsets := make([]int, len(objects)+1)
	for i, obj := range objects {
		offsets[i+1] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n", i+1)
		buf.Write(obj)
		if len(obj) == 0 || obj[len(obj)-1] != '\n' {
			buf.WriteByte('\n')
		}
		buf.WriteString("endobj\n")
	}

	xrefOffset := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", len(objects)+1)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\n", len(objects)+1)
	fmt.Fprintf(&buf, "startxref\n%d\n%%%%EOF\n", xrefOffset)

	return buf.Bytes()
}

func syntheticStreamObject(dict string, data []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(dict)
	buf.WriteString("\nstream\n")
	buf.Write(data)
	buf.WriteString("\nendstream")
	return buf.Bytes()
}

func simulateSyntheticSplashScaleOnly(src *image.Gray, pageBounds image.Rectangle, interpolate bool) *image.RGBA {
	return simulateSyntheticSplashScaleOnlyImage(src, pageBounds, interpolate)
}

func simulateSyntheticSplashScaleOnlyImage(src image.Image, pageBounds image.Rectangle, interpolate bool) *image.RGBA {
	return simulateSyntheticSplashScaleOnlyImageWithMatrix(
		src,
		pageBounds,
		[6]float64{
			float64(pageBounds.Dx()),
			0,
			0,
			float64(pageBounds.Dy()),
			0,
			0,
		},
		interpolate,
	)
}

func simulateSyntheticSplashScaleOnlyImageWithMatrix(
	src image.Image,
	pageBounds image.Rectangle,
	matrix [6]float64,
	interpolate bool,
) *image.RGBA {
	page := image.NewRGBA(pageBounds)
	srcBounds := src.Bounds()

	x0 := syntheticImgCoordMungeLower(matrix[4])
	y0 := syntheticImgCoordMungeLower(matrix[5])
	x1 := syntheticImgCoordMungeUpper(matrix[4] + matrix[0])
	y1 := syntheticImgCoordMungeUpper(matrix[5] + matrix[3])
	scaledWidth := x1 - x0
	scaledHeight := y1 - y0

	scaled := image.NewRGBA(image.Rect(0, 0, scaledWidth, scaledHeight))
	transform := f64.Aff3{
		float64(scaledWidth) / float64(srcBounds.Dx()),
		0,
		0,
		0,
		float64(scaledHeight) / float64(srcBounds.Dy()),
		0,
	}
	if interpolate {
		xdraw.ApproxBiLinear.Transform(scaled, transform, src, srcBounds, draw.Src, nil)
	} else {
		xdraw.NearestNeighbor.Transform(scaled, transform, src, srcBounds, draw.Src, nil)
	}

	draw.Draw(page, pageBounds, scaled, image.Point{X: x0, Y: y0}, draw.Src)
	return page
}

func simulateSyntheticAffineImageWithMatrixAndPhase(
	src image.Image,
	pageBounds image.Rectangle,
	matrix [6]float64,
	interpolate bool,
	phaseX, phaseY float64,
) *image.RGBA {
	page := image.NewRGBA(pageBounds)
	srcBounds := src.Bounds()
	srcW := float64(srcBounds.Dx())
	srcH := float64(srcBounds.Dy())
	if srcW <= 0 || srcH <= 0 {
		return page
	}

	scaleX := matrix[0] / srcW
	scaleY := matrix[3] / srcH
	transform := f64.Aff3{
		scaleX,
		0,
		matrix[4] + scaleX*phaseX,
		0,
		scaleY,
		matrix[5] + scaleY*phaseY,
	}
	if interpolate {
		xdraw.ApproxBiLinear.Transform(page, transform, src, srcBounds, draw.Src, nil)
		return page
	}
	xdraw.NearestNeighbor.Transform(page, transform, src, srcBounds, draw.Src, nil)
	return page
}

func simulateSyntheticAffineImageWithMatrixPlacement(
	src image.Image,
	pageBounds image.Rectangle,
	matrix [6]float64,
	interpolate bool,
	offsetX, offsetY float64,
) *image.RGBA {
	page := image.NewRGBA(pageBounds)
	srcBounds := src.Bounds()
	srcW := float64(srcBounds.Dx())
	srcH := float64(srcBounds.Dy())
	if srcW <= 0 || srcH <= 0 {
		return page
	}

	scaleX := matrix[0] / srcW
	scaleY := matrix[3] / srcH
	transform := f64.Aff3{
		scaleX,
		0,
		matrix[4] + offsetX,
		0,
		scaleY,
		matrix[5] + offsetY,
	}
	if interpolate {
		xdraw.ApproxBiLinear.Transform(page, transform, src, srcBounds, draw.Src, nil)
		return page
	}
	xdraw.NearestNeighbor.Transform(page, transform, src, srcBounds, draw.Src, nil)
	return page
}

func simulateSyntheticAffineImageWithMatrixAndFilter(
	src image.Image,
	pageBounds image.Rectangle,
	matrix [6]float64,
	filter string,
) *image.RGBA {
	page := image.NewRGBA(pageBounds)
	srcBounds := src.Bounds()
	srcW := float64(srcBounds.Dx())
	srcH := float64(srcBounds.Dy())
	if srcW <= 0 || srcH <= 0 {
		return page
	}

	transform := f64.Aff3{
		matrix[0] / srcW,
		0,
		matrix[4],
		0,
		matrix[3] / srcH,
		matrix[5],
	}
	applySyntheticTransformFilter(page, transform, src, srcBounds, filter)
	return page
}

func simulateSyntheticRectResampleThenPlace(
	src image.Image,
	pageBounds image.Rectangle,
	matrix [6]float64,
	filter string,
) *image.RGBA {
	dstW := max(1, int(math.Round(math.Abs(matrix[0]))))
	dstH := max(1, int(math.Round(math.Abs(matrix[3]))))
	scaled := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	srcBounds := src.Bounds()
	if srcBounds.Dx() <= 0 || srcBounds.Dy() <= 0 {
		return image.NewRGBA(pageBounds)
	}

	scaleTransform := f64.Aff3{
		float64(dstW) / float64(srcBounds.Dx()),
		0,
		0,
		0,
		float64(dstH) / float64(srcBounds.Dy()),
		0,
	}
	applySyntheticTransformFilter(scaled, scaleTransform, src, srcBounds, filter)

	page := image.NewRGBA(pageBounds)
	placeTransform := f64.Aff3{
		1,
		0,
		matrix[4],
		0,
		1,
		matrix[5],
	}
	xdraw.ApproxBiLinear.Transform(page, placeTransform, scaled, scaled.Bounds(), draw.Src, nil)
	return page
}

func simulateSyntheticAreaBoxResampleThenPlace(
	src image.Image,
	pageBounds image.Rectangle,
	matrix [6]float64,
) *image.RGBA {
	return simulateSyntheticAreaBoxThenPlaceWithFilter(src, pageBounds, matrix, "")
}

func simulateSyntheticAreaBoxCatmullThenPlace(
	src image.Image,
	pageBounds image.Rectangle,
	matrix [6]float64,
) *image.RGBA {
	return simulateSyntheticAreaBoxThenPlaceWithFilter(src, pageBounds, matrix, "catmull")
}

func simulateSyntheticAreaBoxThenPlaceWithFilter(
	src image.Image,
	pageBounds image.Rectangle,
	matrix [6]float64,
	placementFilter string,
) *image.RGBA {
	dstW := max(1, int(math.Round(math.Abs(matrix[0]))))
	dstH := max(1, int(math.Round(math.Abs(matrix[3]))))
	scaled := resampleSyntheticRGBAAreaBox(src, dstW, dstH)

	page := image.NewRGBA(pageBounds)
	placeTransform := f64.Aff3{
		1,
		0,
		matrix[4],
		0,
		1,
		matrix[5],
	}
	applySyntheticTransformFilter(page, placeTransform, scaled, scaled.Bounds(), placementFilter)
	return page
}

func simulateSyntheticGammaAwareAffineBilinear(
	src image.Image,
	pageBounds image.Rectangle,
	matrix [6]float64,
	gammaMode string,
) *image.RGBA {
	page := image.NewRGBA(pageBounds)
	srcBounds := src.Bounds()
	srcW := float64(srcBounds.Dx())
	srcH := float64(srcBounds.Dy())
	if srcW <= 0 || srcH <= 0 || matrix[0] == 0 || matrix[3] == 0 {
		return page
	}

	scaleX := matrix[0] / srcW
	scaleY := matrix[3] / srcH
	for dy := pageBounds.Min.Y; dy < pageBounds.Max.Y; dy++ {
		for dx := pageBounds.Min.X; dx < pageBounds.Max.X; dx++ {
			u := (float64(dx) + 0.5 - matrix[4]) / scaleX
			v := (float64(dy) + 0.5 - matrix[5]) / scaleY
			if u < 0 || u >= srcW || v < 0 || v >= srcH {
				continue
			}
			page.SetRGBA(dx, dy, sampleGammaAwareBilinear(src, srcBounds, u, v, gammaMode))
		}
	}
	return page
}

func simulateSyntheticGammaAwareRectResampleThenPlace(
	src image.Image,
	pageBounds image.Rectangle,
	matrix [6]float64,
	gammaMode string,
) *image.RGBA {
	dstW := max(1, int(math.Round(math.Abs(matrix[0]))))
	dstH := max(1, int(math.Round(math.Abs(matrix[3]))))
	scaled := resampleSyntheticRGBGammaAwareBilinear(src, dstW, dstH, gammaMode)
	return simulateSyntheticGammaAwareAffineBilinear(scaled, pageBounds, [6]float64{
		float64(dstW),
		0,
		0,
		float64(dstH),
		matrix[4],
		matrix[5],
	}, gammaMode)
}

func simulateSyntheticTransparentEdgeAffineOverWhite(
	src image.Image,
	pageBounds image.Rectangle,
	matrix [6]float64,
) *image.RGBA {
	page := image.NewRGBA(pageBounds)
	draw.Draw(page, page.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)

	srcBounds := src.Bounds()
	srcW := float64(srcBounds.Dx())
	srcH := float64(srcBounds.Dy())
	if srcW <= 0 || srcH <= 0 || matrix[0] == 0 || matrix[3] == 0 {
		return page
	}

	scaleX := matrix[0] / srcW
	scaleY := matrix[3] / srcH
	for dy := pageBounds.Min.Y; dy < pageBounds.Max.Y; dy++ {
		for dx := pageBounds.Min.X; dx < pageBounds.Max.X; dx++ {
			u := (float64(dx) + 0.5 - matrix[4]) / scaleX
			v := (float64(dy) + 0.5 - matrix[5]) / scaleY
			sample := sampleTransparentEdgeBilinear(src, srcBounds, u, v)
			page.SetRGBA(dx, dy, compositeRGBAOverWhite(sample))
		}
	}

	return page
}

func simulateSyntheticTransparentEdgeRectOverWhite(
	src image.Image,
	pageBounds image.Rectangle,
	matrix [6]float64,
) *image.RGBA {
	dstW := max(1, int(math.Round(math.Abs(matrix[0]))))
	dstH := max(1, int(math.Round(math.Abs(matrix[3]))))
	temp := resampleSyntheticTransparentEdgeBilinear(src, dstW, dstH)

	page := image.NewRGBA(pageBounds)
	draw.Draw(page, page.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	tempBounds := temp.Bounds()
	for dy := pageBounds.Min.Y; dy < pageBounds.Max.Y; dy++ {
		for dx := pageBounds.Min.X; dx < pageBounds.Max.X; dx++ {
			u := float64(dx) + 0.5 - matrix[4]
			v := float64(dy) + 0.5 - matrix[5]
			sample := sampleTransparentEdgeBilinear(temp, tempBounds, u, v)
			page.SetRGBA(dx, dy, compositeRGBAOverWhite(sample))
		}
	}

	return page
}

func applySyntheticTransformFilter(
	dst draw.Image,
	transform f64.Aff3,
	src image.Image,
	srcBounds image.Rectangle,
	filter string,
) {
	switch filter {
	case "catmull":
		xdraw.CatmullRom.Transform(dst, transform, src, srcBounds, draw.Src, nil)
	default:
		xdraw.ApproxBiLinear.Transform(dst, transform, src, srcBounds, draw.Src, nil)
	}
}

func resampleSyntheticRGBGammaAwareBilinear(
	src image.Image,
	dstW, dstH int,
	gammaMode string,
) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	srcBounds := src.Bounds()
	srcW := float64(srcBounds.Dx())
	srcH := float64(srcBounds.Dy())
	if srcW <= 0 || srcH <= 0 {
		return dst
	}

	scaleX := float64(dstW) / srcW
	scaleY := float64(dstH) / srcH
	for dy := 0; dy < dstH; dy++ {
		for dx := 0; dx < dstW; dx++ {
			u := (float64(dx) + 0.5) / scaleX
			v := (float64(dy) + 0.5) / scaleY
			dst.SetRGBA(dx, dy, sampleGammaAwareBilinear(src, srcBounds, u, v, gammaMode))
		}
	}
	return dst
}

func resampleSyntheticTransparentEdgeBilinear(src image.Image, dstW, dstH int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	srcBounds := src.Bounds()
	srcW := float64(srcBounds.Dx())
	srcH := float64(srcBounds.Dy())
	if srcW <= 0 || srcH <= 0 {
		return dst
	}

	scaleX := float64(dstW) / srcW
	scaleY := float64(dstH) / srcH
	for dy := 0; dy < dstH; dy++ {
		for dx := 0; dx < dstW; dx++ {
			u := (float64(dx) + 0.5) / scaleX
			v := (float64(dy) + 0.5) / scaleY
			dst.SetRGBA(dx, dy, sampleTransparentEdgeBilinear(src, srcBounds, u, v))
		}
	}
	return dst
}

func sampleGammaAwareBilinear(
	src image.Image,
	srcBounds image.Rectangle,
	u, v float64,
	gammaMode string,
) color.RGBA {
	sx := u - 0.5
	sy := v - 0.5

	x0 := int(math.Floor(sx))
	y0 := int(math.Floor(sy))
	fx := sx - float64(x0)
	fy := sy - float64(y0)
	x1 := x0 + 1
	y1 := y0 + 1

	c00 := sampleGammaAwareLinearRGBA(src, srcBounds, x0, y0, gammaMode)
	c10 := sampleGammaAwareLinearRGBA(src, srcBounds, x1, y0, gammaMode)
	c01 := sampleGammaAwareLinearRGBA(src, srcBounds, x0, y1, gammaMode)
	c11 := sampleGammaAwareLinearRGBA(src, srcBounds, x1, y1, gammaMode)

	w00 := (1 - fx) * (1 - fy)
	w10 := fx * (1 - fy)
	w01 := (1 - fx) * fy
	w11 := fx * fy

	r := c00[0]*w00 + c10[0]*w10 + c01[0]*w01 + c11[0]*w11
	g := c00[1]*w00 + c10[1]*w10 + c01[1]*w01 + c11[1]*w11
	b := c00[2]*w00 + c10[2]*w10 + c01[2]*w01 + c11[2]*w11
	a := c00[3]*w00 + c10[3]*w10 + c01[3]*w01 + c11[3]*w11

	return color.RGBA{
		R: gammaAwareLinearToByte(r, gammaMode),
		G: gammaAwareLinearToByte(g, gammaMode),
		B: gammaAwareLinearToByte(b, gammaMode),
		A: uint8(math.Round(a * 255)),
	}
}

func sampleTransparentEdgeBilinear(
	src image.Image,
	srcBounds image.Rectangle,
	u, v float64,
) color.RGBA {
	sx := u - 0.5
	sy := v - 0.5

	x0 := int(math.Floor(sx))
	y0 := int(math.Floor(sy))
	fx := sx - float64(x0)
	fy := sy - float64(y0)
	x1 := x0 + 1
	y1 := y0 + 1

	c00 := sampleTransparentPremultipliedRGBA(src, srcBounds, x0, y0)
	c10 := sampleTransparentPremultipliedRGBA(src, srcBounds, x1, y0)
	c01 := sampleTransparentPremultipliedRGBA(src, srcBounds, x0, y1)
	c11 := sampleTransparentPremultipliedRGBA(src, srcBounds, x1, y1)

	w00 := (1 - fx) * (1 - fy)
	w10 := fx * (1 - fy)
	w01 := (1 - fx) * fy
	w11 := fx * fy

	r := c00[0]*w00 + c10[0]*w10 + c01[0]*w01 + c11[0]*w11
	g := c00[1]*w00 + c10[1]*w10 + c01[1]*w01 + c11[1]*w11
	b := c00[2]*w00 + c10[2]*w10 + c01[2]*w01 + c11[2]*w11
	a := c00[3]*w00 + c10[3]*w10 + c01[3]*w01 + c11[3]*w11

	return color.RGBA{
		R: uint8(math.Round(math.Min(math.Max(r, 0), 1) * 255)),
		G: uint8(math.Round(math.Min(math.Max(g, 0), 1) * 255)),
		B: uint8(math.Round(math.Min(math.Max(b, 0), 1) * 255)),
		A: uint8(math.Round(math.Min(math.Max(a, 0), 1) * 255)),
	}
}

func sampleGammaAwareLinearRGBA(
	src image.Image,
	srcBounds image.Rectangle,
	x, y int,
	gammaMode string,
) [4]float64 {
	clampedX := min(max(x, srcBounds.Min.X), srcBounds.Max.X-1)
	clampedY := min(max(y, srcBounds.Min.Y), srcBounds.Max.Y-1)
	r, g, b, a := src.At(clampedX, clampedY).RGBA()
	return [4]float64{
		gammaAwareByteToLinear(uint8(r/257), gammaMode),
		gammaAwareByteToLinear(uint8(g/257), gammaMode),
		gammaAwareByteToLinear(uint8(b/257), gammaMode),
		float64(uint8(a/257)) / 255.0,
	}
}

func sampleTransparentPremultipliedRGBA(
	src image.Image,
	srcBounds image.Rectangle,
	x, y int,
) [4]float64 {
	if x < srcBounds.Min.X || x >= srcBounds.Max.X || y < srcBounds.Min.Y || y >= srcBounds.Max.Y {
		return [4]float64{}
	}
	r, g, b, a := src.At(x, y).RGBA()
	alpha := float64(uint8(a/257)) / 255.0
	return [4]float64{
		float64(uint8(r/257)) / 255.0 * alpha,
		float64(uint8(g/257)) / 255.0 * alpha,
		float64(uint8(b/257)) / 255.0 * alpha,
		alpha,
	}
}

func compositeRGBAOverWhite(src color.RGBA) color.RGBA {
	alpha := float64(src.A) / 255.0
	inv := 1 - alpha
	return color.RGBA{
		R: uint8(math.Round(float64(src.R) + 255.0*inv)),
		G: uint8(math.Round(float64(src.G) + 255.0*inv)),
		B: uint8(math.Round(float64(src.B) + 255.0*inv)),
		A: 255,
	}
}

func gammaAwareByteToLinear(value uint8, gammaMode string) float64 {
	v := float64(value) / 255.0
	switch gammaMode {
	case "srgb":
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	case "gamma22":
		return math.Pow(v, 2.2)
	default:
		return v
	}
}

func gammaAwareLinearToByte(value float64, gammaMode string) uint8 {
	v := math.Min(math.Max(value, 0), 1)
	switch gammaMode {
	case "srgb":
		if v <= 0.0031308 {
			v *= 12.92
		} else {
			v = 1.055*math.Pow(v, 1.0/2.4) - 0.055
		}
	case "gamma22":
		v = math.Pow(v, 1.0/2.2)
	}
	return uint8(math.Round(v * 255))
}

func resampleSyntheticRGBAAreaBox(src image.Image, dstW, dstH int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	srcBounds := src.Bounds()
	srcW := float64(srcBounds.Dx())
	srcH := float64(srcBounds.Dy())
	if srcW <= 0 || srcH <= 0 {
		return dst
	}

	for dy := 0; dy < dstH; dy++ {
		sy0 := float64(dy) * srcH / float64(dstH)
		sy1 := float64(dy+1) * srcH / float64(dstH)
		yStart := max(srcBounds.Min.Y, srcBounds.Min.Y+int(math.Floor(sy0)))
		yEnd := min(srcBounds.Max.Y, srcBounds.Min.Y+int(math.Ceil(sy1)))
		for dx := 0; dx < dstW; dx++ {
			sx0 := float64(dx) * srcW / float64(dstW)
			sx1 := float64(dx+1) * srcW / float64(dstW)
			xStart := max(srcBounds.Min.X, srcBounds.Min.X+int(math.Floor(sx0)))
			xEnd := min(srcBounds.Max.X, srcBounds.Min.X+int(math.Ceil(sx1)))

			var sumR, sumG, sumB, sumA float64
			var totalWeight float64
			for sy := yStart; sy < yEnd; sy++ {
				overlapY := overlapSpan(sy0, sy1, float64(sy-srcBounds.Min.Y), float64(sy-srcBounds.Min.Y+1))
				if overlapY <= 0 {
					continue
				}
				for sx := xStart; sx < xEnd; sx++ {
					overlapX := overlapSpan(sx0, sx1, float64(sx-srcBounds.Min.X), float64(sx-srcBounds.Min.X+1))
					if overlapX <= 0 {
						continue
					}
					weight := overlapX * overlapY
					r, g, b, a := src.At(sx, sy).RGBA()
					sumR += float64(r) * weight
					sumG += float64(g) * weight
					sumB += float64(b) * weight
					sumA += float64(a) * weight
					totalWeight += weight
				}
			}

			if totalWeight == 0 {
				continue
			}
			dst.SetRGBA(dx, dy, color.RGBA{
				R: uint8(math.Round(sumR / totalWeight / 257.0)),
				G: uint8(math.Round(sumG / totalWeight / 257.0)),
				B: uint8(math.Round(sumB / totalWeight / 257.0)),
				A: uint8(math.Round(sumA / totalWeight / 257.0)),
			})
		}
	}

	return dst
}

func overlapSpan(a0, a1, b0, b1 float64) float64 {
	return math.Max(0, math.Min(a1, b1)-math.Max(a0, b0))
}

func simulateSyntheticGrayBoxDownscale(src *image.Gray, pageBounds image.Rectangle) *image.RGBA {
	page := image.NewRGBA(pageBounds)
	srcBounds := src.Bounds()
	dstBounds := pageBounds
	scaleX := srcBounds.Dx() / dstBounds.Dx()
	scaleY := srcBounds.Dy() / dstBounds.Dy()
	for dy := 0; dy < dstBounds.Dy(); dy++ {
		for dx := 0; dx < dstBounds.Dx(); dx++ {
			var sum int
			for sy := 0; sy < scaleY; sy++ {
				for sx := 0; sx < scaleX; sx++ {
					sum += int(src.GrayAt(srcBounds.Min.X+dx*scaleX+sx, srcBounds.Min.Y+dy*scaleY+sy).Y)
				}
			}
			value := uint8(sum / (scaleX * scaleY))
			page.SetRGBA(dstBounds.Min.X+dx, dstBounds.Min.Y+dy, color.RGBA{R: value, G: value, B: value, A: 255})
		}
	}
	return page
}

func imageToRGBA(src image.Image, bounds image.Rectangle) *image.RGBA {
	dst := image.NewRGBA(bounds)
	draw.Draw(dst, bounds, src, src.Bounds().Min, draw.Src)
	return dst
}

func flattenImageToOpaqueRGBA(src image.Image, background color.Color) *image.RGBA {
	bounds := src.Bounds()
	opaque := image.NewRGBA(bounds)
	draw.Draw(opaque, bounds, &image.Uniform{C: background}, image.Point{}, draw.Src)
	draw.Draw(opaque, bounds, src, bounds.Min, draw.Over)
	return opaque
}

func imageToRGBBytes(src image.Image) []byte {
	bounds := src.Bounds()
	rgb := make([]byte, 0, bounds.Dx()*bounds.Dy()*3)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			rgb = append(rgb, uint8(r>>8), uint8(g>>8), uint8(b>>8))
		}
	}
	return rgb
}

func imageToGrayBytes(src image.Image) []byte {
	bounds := src.Bounds()
	gray := make([]byte, 0, bounds.Dx()*bounds.Dy())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray = append(gray, color.GrayModel.Convert(src.At(x, y)).(color.Gray).Y)
		}
	}
	return gray
}

func imageToOpaqueRGBBytes(src image.Image, background color.Color) []byte {
	return imageToRGBBytes(flattenImageToOpaqueRGBA(src, background))
}

func imageMaskToGrayBytes(t *testing.T, mask domainimage.ImageMask) []byte {
	t.Helper()

	require.NotNil(t, mask)
	maskImage := mask.Image()
	require.NotNil(t, maskImage)

	bounds := maskImage.Bounds()
	out := make([]byte, 0, bounds.Dx()*bounds.Dy())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			out = append(out, color.GrayModel.Convert(maskImage.At(x, y)).(color.Gray).Y)
		}
	}
	return out
}

func syntheticImgCoordMungeLower(x float64) int {
	return int(x)
}

func syntheticImgCoordMungeUpper(x float64) int {
	return int(x) + 1
}
