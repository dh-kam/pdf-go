package pdf_test

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"fmt"
	"image"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainimage "github.com/dh-kam/pdf-go/internal/domain/image"
	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
)

func pdfPointMatrixToPixelPlacementForProbe(
	pageW, pageH float64,
	matrix [6]float64,
	dpi float64,
) (image.Rectangle, [6]float64) {
	scale := dpi / 72.0
	pageBounds := image.Rect(
		0,
		0,
		int(math.Round(pageW*scale)),
		int(math.Round(pageH*scale)),
	)
	return pageBounds, [6]float64{
		matrix[0] * scale,
		matrix[1] * scale,
		matrix[2] * scale,
		matrix[3] * scale,
		matrix[4] * scale,
		matrix[5] * scale,
	}
}

func buildSyntheticDCTICCBasedGrayImagePDFFloatWithICCStreamOptions(
	pageW, pageH float64,
	imageW, imageH int,
	matrix [6]float64,
	jpegData []byte,
	iccProfile []byte,
	iccComponents int,
	alternate string,
	iccFilter domainimage.ImageFilter,
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

	iccData := append([]byte(nil), iccProfile...)
	iccDictExtras := ""
	if alternate != "" {
		iccDictExtras += " /Alternate /" + alternate
	}
	if iccFilter == domainimage.FilterFlate {
		var buf bytes.Buffer
		zw := zlib.NewWriter(&buf)
		_, _ = zw.Write(iccProfile)
		_ = zw.Close()
		iccData = buf.Bytes()
		iccDictExtras += " /Filter /FlateDecode"
	}
	if iccFilter == domainimage.FilterASCII85 {
		encoded := make([]byte, ascii85.MaxEncodedLen(len(iccProfile)))
		n := ascii85.Encode(encoded, iccProfile)
		iccData = encoded[:n]
		iccDictExtras += " /Filter /ASCII85Decode"
	}

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
			fmt.Sprintf("<< /N %d%s /Length %d >>", iccComponents, iccDictExtras, len(iccData)),
			iccData,
		),
	}

	return buildSyntheticPDF(objects)
}

func TestSyntheticDoc007MainPage4ExtractedICCBasedExactPlacementProbeAt150DPI(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))

	result := measureSyntheticPDFPlacementProbeAgainstPopplerAtDPI(
		t,
		"doc007_page4_extracted_iccbased_exact_placement.pdf",
		buildSyntheticDCTICCBasedGrayImagePDFFloat(
			3.84,
			3.84,
			imageData.Width,
			imageData.Height,
			[6]float64{3.84, 0, 0, 3.84, 0, 0},
			imageData.Data,
			imageData.ICCProfile,
			imageData.ICCComponents,
		),
		150,
		2,
	)

	require.True(t, result.hasDiffBounds)
	assert.InDelta(t, 86.6422, result.originalSimilarity, 1e-4)
	assert.Equal(t, -2, result.bestShiftX)
	assert.Equal(t, 2, result.bestShiftY)
	assert.InDelta(t, 91.4338, result.bestShiftSimilarity, 1e-4)
	assert.Greater(t, result.bestShiftSimilarity, result.originalSimilarity)
	t.Logf(
		"doc007 extracted exact placement @150dpi: original=%.4f best=(%d,%d)->%.4f",
		result.originalSimilarity,
		result.bestShiftX,
		result.bestShiftY,
		result.bestShiftSimilarity,
	)
}

func TestSyntheticDoc007MainPage4ExactPlacementReferenceProbeAt150DPI(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))
	decodedImage, _ := decodeSampleEncodedImageObjectToRGB(t, pdfPath, entity.NewRef(56, 0))

	pageBounds, pixelMatrix := pdfPointMatrixToPixelPlacementForProbe(
		3.84,
		3.84,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		150,
	)

	_, currentSimilarity, refScores := probeSyntheticCurrentAndReferencesAgainstPopplerAtDPI(
		t,
		"doc007_page4_extracted_iccbased_exact_placement_refs.pdf",
		buildSyntheticDCTICCBasedGrayImagePDFFloat(
			3.84,
			3.84,
			imageData.Width,
			imageData.Height,
			[6]float64{3.84, 0, 0, 3.84, 0, 0},
			imageData.Data,
			imageData.ICCProfile,
			imageData.ICCComponents,
		),
		150,
		map[string]image.Image{
			"affine_phase_0":   simulateSyntheticAffineImageWithMatrixAndPhase(decodedImage, pageBounds, pixelMatrix, true, 0, 0),
			"affine_phase_0.5": simulateSyntheticAffineImageWithMatrixAndPhase(decodedImage, pageBounds, pixelMatrix, true, 0.5, 0.5),
			"rect_two_pass":    simulateSyntheticRectResampleThenPlace(decodedImage, pageBounds, pixelMatrix, "bilinear"),
		},
	)

	assert.InDelta(t, 86.6422, currentSimilarity, 1e-4)
	bestRef := maxFloatForDoc007ExactPlacementProbe(
		refScores["affine_phase_0"],
		refScores["affine_phase_0.5"],
		refScores["rect_two_pass"],
	)
	assert.Less(t, bestRef, currentSimilarity)
	t.Logf(
		"doc007 exact placement refs @150dpi: current=%.4f phase0=%.4f phase05=%.4f rect=%.4f",
		currentSimilarity,
		refScores["affine_phase_0"],
		refScores["affine_phase_0.5"],
		refScores["rect_two_pass"],
	)
}

func TestSyntheticDoc007MainPage1ExtractedExactPlacementProbeAt150DPI(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(8, 0))

	result := measureSyntheticPDFPlacementProbeAgainstPopplerAtDPI(
		t,
		"doc007_page1_extracted_exact_placement.pdf",
		buildSyntheticDCTICCBasedGrayImagePDFFloat(
			3.84,
			3.84,
			imageData.Width,
			imageData.Height,
			[6]float64{3.84, 0, 0, 3.84, 0, 0},
			imageData.Data,
			imageData.ICCProfile,
			imageData.ICCComponents,
		),
		150,
		2,
	)

	require.True(t, result.hasDiffBounds)
	assert.InDelta(t, 87.8493, result.originalSimilarity, 1e-4)
	assert.Equal(t, 0, result.bestShiftX)
	assert.Equal(t, 0, result.bestShiftY)
	assert.InDelta(t, 87.8493, result.bestShiftSimilarity, 1e-4)
	t.Logf(
		"doc007 page1 extracted exact placement @150dpi: original=%.4f best=(%d,%d)->%.4f",
		result.originalSimilarity,
		result.bestShiftX,
		result.bestShiftY,
		result.bestShiftSimilarity,
	)
}

func TestSyntheticDoc007MainPage4ExtractedExactPlacementModeProbeAt150DPI(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))
	pdfBytes := buildSyntheticDCTICCBasedGrayImagePDFFloat(
		3.84,
		3.84,
		imageData.Width,
		imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
	)

	modes := []string{
		domainrenderer.ImageSamplingModeLegacy,
		domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
		domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1,
	}

	scores := make(map[string]float64, len(modes))
	for _, mode := range modes {
		result := measureSyntheticPDFPlacementProbeAgainstPopplerAtDPIWithMode(
			t,
			"doc007_page4_extracted_exact_placement_mode_probe.pdf",
			pdfBytes,
			150,
			2,
			mode,
		)
		scores[mode] = result.originalSimilarity
	}

	assert.InDelta(t, 86.6422, scores[domainrenderer.ImageSamplingModeLegacy], 1e-4)
	assert.InDelta(t, scores[domainrenderer.ImageSamplingModeLegacy], scores[domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1], 1e-4)
	assert.InDelta(t, scores[domainrenderer.ImageSamplingModeLegacy], scores[domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1], 1e-4)
	t.Logf("doc007 page4 extracted exact placement mode scores @150dpi: %+v", scores)
}

func TestSyntheticDoc007MainPage4DecodedRGBExactPlacementModeProbeAt150DPI(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	_, decodedRGB := decodeSampleEncodedImageObjectToRGB(t, pdfPath, entity.NewRef(56, 0))
	pdfBytes := buildSyntheticRGBImagePDFFloat(
		3.84,
		3.84,
		16,
		16,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		decodedRGB,
	)

	modes := []string{
		domainrenderer.ImageSamplingModeLegacy,
		domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
		domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1,
	}

	scores := make(map[string]float64, len(modes))
	for _, mode := range modes {
		result := measureSyntheticPDFPlacementProbeAgainstPopplerAtDPIWithMode(
			t,
			"doc007_page4_decoded_rgb_exact_placement_mode_probe.pdf",
			pdfBytes,
			150,
			2,
			mode,
		)
		scores[mode] = result.originalSimilarity
	}

	assert.InDelta(t, scores[domainrenderer.ImageSamplingModeLegacy], scores[domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1], 1e-4)
	assert.InDelta(t, scores[domainrenderer.ImageSamplingModeLegacy], scores[domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1], 1e-4)
	t.Logf("doc007 page4 decoded RGB exact placement mode scores @150dpi: %+v", scores)
}

func TestSyntheticDoc007MainPage4DeviceGrayDCTExactPlacementModeProbeAt150DPI(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))
	pdfBytes := buildSyntheticDCTGrayImagePDFFloat(
		3.84,
		3.84,
		imageData.Width,
		imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
	)

	modes := []string{
		domainrenderer.ImageSamplingModeLegacy,
		domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
		domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1,
	}

	scores := make(map[string]float64, len(modes))
	for _, mode := range modes {
		result := measureSyntheticPDFPlacementProbeAgainstPopplerAtDPIWithMode(
			t,
			"doc007_page4_devicegray_dct_exact_placement_mode_probe.pdf",
			pdfBytes,
			150,
			2,
			mode,
		)
		scores[mode] = result.originalSimilarity
	}

	assert.InDelta(t, scores[domainrenderer.ImageSamplingModeLegacy], scores[domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1], 1e-4)
	assert.InDelta(t, scores[domainrenderer.ImageSamplingModeLegacy], scores[domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1], 1e-4)
	t.Logf("doc007 page4 devicegray DCT exact placement mode scores @150dpi: %+v", scores)
}

func TestSyntheticDoc007MainPage4AdaptiveExactPlacementAxisProbeAt150DPI(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))
	decodedImage, _ := decodeSampleEncodedImageObjectToRGB(t, pdfPath, entity.NewRef(56, 0))

	pdfBytes := buildSyntheticDCTICCBasedGrayImagePDFFloat(
		3.84,
		3.84,
		imageData.Width,
		imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
	)
	pageBounds, pixelMatrix := pdfPointMatrixToPixelPlacementForProbe(
		3.84,
		3.84,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		150,
	)

	currentSimilarity, refScores := probeSyntheticPDFModeAndReferencesAgainstPopplerAtDPI(
		t,
		"doc007_page4_adaptive_exact_placement_axis_probe.pdf",
		pdfBytes,
		150,
		domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
		map[string]image.Image{
			"adaptive_like_nearest_phase0": simulateSyntheticAffineImageWithMatrixAndPhase(decodedImage, pageBounds, pixelMatrix, false, 0, 0),
			"phase_only_legacy_side":       simulateSyntheticAffineImageWithMatrixAndPhase(decodedImage, pageBounds, pixelMatrix, false, 0.5, 0.5),
			"sampler_only_legacy_side":     simulateSyntheticAffineImageWithMatrixAndPhase(decodedImage, pageBounds, pixelMatrix, true, 0, 0),
			"sampler_and_phase_legacy":     simulateSyntheticAffineImageWithMatrixAndPhase(decodedImage, pageBounds, pixelMatrix, true, 0.5, 0.5),
		},
	)
	ignoreICCSimilarity := measureSyntheticPDFPlacementProbeAgainstPopplerAtDPIWithMode(
		t,
		"doc007_page4_adaptive_exact_placement_axis_probe.pdf",
		pdfBytes,
		150,
		2,
		domainrenderer.ImageSamplingModeExperimentalDCTGrayIgnoreICCV1,
	).originalSimilarity

	assert.InDelta(t, 86.6422, currentSimilarity, 1e-4)
	assert.Greater(t, currentSimilarity, refScores["phase_only_legacy_side"])
	assert.Greater(t, refScores["phase_only_legacy_side"], refScores["sampler_only_legacy_side"])
	assert.Greater(t, refScores["sampler_only_legacy_side"], refScores["adaptive_like_nearest_phase0"])
	assert.InDelta(t, refScores["phase_only_legacy_side"], refScores["sampler_and_phase_legacy"], 1e-4)
	assert.InDelta(t, ignoreICCSimilarity, currentSimilarity, 1e-4)
	t.Logf("doc007 page4 adaptive exact placement axis scores @150dpi: current=%.4f refs=%+v", currentSimilarity, refScores)
	t.Logf(
		"doc007 page4 adaptive exact placement gains @150dpi: icc_only=%.4f phase_only=%.4f sampler_only=%.4f",
		ignoreICCSimilarity-currentSimilarity,
		refScores["phase_only_legacy_side"]-refScores["adaptive_like_nearest_phase0"],
		refScores["sampler_only_legacy_side"]-refScores["adaptive_like_nearest_phase0"],
	)
}

func TestSampleDecodeOrTransform007AdaptiveModeFocusedParityAt150DPI(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()

	for _, pageNum := range []int{1, 4} {
		scores := probeSamplePageModesAgainstPopplerAtDPI(
			t,
			pdfPath,
			pageNum,
			150,
			[]string{
				domainrenderer.ImageSamplingModeLegacy,
				domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
			},
		)

		t.Logf("doc007 main page%d adaptive focused parity @150dpi: %+v", pageNum, scores)
	}
}

func TestSyntheticDoc007MainPage4OriginalAndSyntheticImageObjectDictProbe(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	original := measurePDFPageImageObjectForProbe(t, pdfPath, 4, "Im3")

	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))
	syntheticPDF := buildSyntheticDCTICCBasedGrayImagePDFFloat(
		3.84,
		3.84,
		imageData.Width,
		imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
	)
	syntheticPath := filepath.Join(t.TempDir(), "doc007_page4_exact_placement_dict_probe.pdf")
	require.NoError(t, os.WriteFile(syntheticPath, syntheticPDF, 0o644))
	synthetic := measurePDFPageImageObjectForProbe(t, syntheticPath, 1, "Im0")

	assert.Equal(t, original.filter, synthetic.filter)
	assert.Equal(t, original.colorSpace, synthetic.colorSpace)
	assert.Equal(t, original.width, synthetic.width)
	assert.Equal(t, original.height, synthetic.height)
	assert.Equal(t, original.bitsPerComponent, synthetic.bitsPerComponent)
	assert.Equal(t, original.iccComponents, synthetic.iccComponents)

	t.Logf("doc007 page4 original image dict keys=%v", original.dictKeys)
	t.Logf("doc007 page4 synthetic image dict keys=%v", synthetic.dictKeys)
}

func TestSyntheticDoc007MainPage4OriginalAndSyntheticICCProfileDictProbe(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	original := measurePDFPageImageICCProfileForProbe(t, pdfPath, 4, "Im3")

	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))
	syntheticPDF := buildSyntheticDCTICCBasedGrayImagePDFFloat(
		3.84,
		3.84,
		imageData.Width,
		imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
	)
	syntheticPath := filepath.Join(t.TempDir(), "doc007_page4_exact_placement_icc_dict_probe.pdf")
	require.NoError(t, os.WriteFile(syntheticPath, syntheticPDF, 0o644))
	synthetic := measurePDFPageImageICCProfileForProbe(t, syntheticPath, 1, "Im0")

	assert.Equal(t, original.components, synthetic.components)
	assert.Equal(t, original.profileLength, synthetic.profileLength)

	t.Logf("doc007 page4 original ICC dict keys=%v filter=%s alternate=%s", original.dictKeys, original.filter, original.alternate)
	t.Logf("doc007 page4 synthetic ICC dict keys=%v filter=%s alternate=%s", synthetic.dictKeys, synthetic.filter, synthetic.alternate)
}

func TestSyntheticDoc007MainPage4ICCProfileMetadataAxisProbeAt150DPI(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))
	originalICC := measurePDFPageImageICCProfileForProbe(t, pdfPath, 4, "Im3")

	basePDF := buildSyntheticDCTICCBasedGrayImagePDFFloat(
		3.84, 3.84,
		imageData.Width, imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
	)
	withAlternatePDF := buildSyntheticDCTICCBasedGrayImagePDFFloatWithICCStreamOptions(
		3.84, 3.84,
		imageData.Width, imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
		originalICC.alternate,
		domainimage.FilterNone,
	)
	withAlternateAndFilterPDF := buildSyntheticDCTICCBasedGrayImagePDFFloatWithICCStreamOptions(
		3.84, 3.84,
		imageData.Width, imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
		originalICC.alternate,
		originalICC.filter,
	)

	base := measureSyntheticPDFPlacementProbeAgainstPopplerAtDPIWithMode(
		t, "doc007_page4_icc_axis_base.pdf", basePDF, 150, 2, domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
	)
	withAlternate := measureSyntheticPDFPlacementProbeAgainstPopplerAtDPIWithMode(
		t, "doc007_page4_icc_axis_alternate.pdf", withAlternatePDF, 150, 2, domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
	)
	withAlternateAndFilter := measureSyntheticPDFPlacementProbeAgainstPopplerAtDPIWithMode(
		t, "doc007_page4_icc_axis_alternate_filter.pdf", withAlternateAndFilterPDF, 150, 2, domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
	)

	t.Logf(
		"doc007 page4 ICC metadata axis @150dpi: base=%.4f alternate=%.4f alternate+filter=%.4f",
		base.originalSimilarity,
		withAlternate.originalSimilarity,
		withAlternateAndFilter.originalSimilarity,
	)
}

func TestSyntheticDoc007MainPage4ICCProfileMetadataDecodeAxisProbe(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))
	originalICC := measurePDFPageImageICCProfileForProbe(t, pdfPath, 4, "Im3")

	root := t.TempDir()
	basePath := filepath.Join(root, "base.pdf")
	withAlternatePath := filepath.Join(root, "alternate.pdf")
	withAlternateAndFilterPath := filepath.Join(root, "alternate_filter.pdf")

	require.NoError(t, os.WriteFile(basePath, buildSyntheticDCTICCBasedGrayImagePDFFloat(
		3.84, 3.84,
		imageData.Width, imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
	), 0o644))
	require.NoError(t, os.WriteFile(withAlternatePath, buildSyntheticDCTICCBasedGrayImagePDFFloatWithICCStreamOptions(
		3.84, 3.84,
		imageData.Width, imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
		originalICC.alternate,
		domainimage.FilterNone,
	), 0o644))
	require.NoError(t, os.WriteFile(withAlternateAndFilterPath, buildSyntheticDCTICCBasedGrayImagePDFFloatWithICCStreamOptions(
		3.84, 3.84,
		imageData.Width, imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
		originalICC.alternate,
		originalICC.filter,
	), 0o644))

	original := decodePDFPageImageObjectToRGBForProbe(t, pdfPath, 4, "Im3")
	base := decodePDFPageImageObjectToRGBForProbe(t, basePath, 1, "Im0")
	withAlternate := decodePDFPageImageObjectToRGBForProbe(t, withAlternatePath, 1, "Im0")
	withAlternateAndFilter := decodePDFPageImageObjectToRGBForProbe(t, withAlternateAndFilterPath, 1, "Im0")

	assert.Equal(t, base.rgbSHA256, withAlternate.rgbSHA256)
	assert.NotEqual(t, original.rgbSHA256, base.rgbSHA256)
	assert.Equal(t, original.rgbSHA256, withAlternateAndFilter.rgbSHA256)

	t.Logf(
		"doc007 page4 ICC decode axis: original=%s base=%s alternate=%s alternate+filter=%s",
		original.rgbSHA256,
		base.rgbSHA256,
		withAlternate.rgbSHA256,
		withAlternateAndFilter.rgbSHA256,
	)
}

func TestSyntheticDoc007MainPage4ICCProfileFilterTransportAxisProbeAt150DPI(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))
	originalICC := measurePDFPageImageICCProfileForProbe(t, pdfPath, 4, "Im3")

	root := t.TempDir()
	basePath := filepath.Join(root, "base.pdf")
	flatePath := filepath.Join(root, "flate.pdf")
	ascii85Path := filepath.Join(root, "ascii85.pdf")

	basePDF := buildSyntheticDCTICCBasedGrayImagePDFFloatWithICCStreamOptions(
		3.84, 3.84,
		imageData.Width, imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
		originalICC.alternate,
		domainimage.FilterNone,
	)
	flatePDF := buildSyntheticDCTICCBasedGrayImagePDFFloatWithICCStreamOptions(
		3.84, 3.84,
		imageData.Width, imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
		originalICC.alternate,
		domainimage.FilterFlate,
	)
	ascii85PDF := buildSyntheticDCTICCBasedGrayImagePDFFloatWithICCStreamOptions(
		3.84, 3.84,
		imageData.Width, imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
		originalICC.alternate,
		domainimage.FilterASCII85,
	)

	require.NoError(t, os.WriteFile(basePath, basePDF, 0o644))
	require.NoError(t, os.WriteFile(flatePath, flatePDF, 0o644))
	require.NoError(t, os.WriteFile(ascii85Path, ascii85PDF, 0o644))

	originalDecode := decodePDFPageImageObjectToRGBForProbe(t, pdfPath, 4, "Im3")
	baseDecode := decodePDFPageImageObjectToRGBForProbe(t, basePath, 1, "Im0")
	flateDecode := decodePDFPageImageObjectToRGBForProbe(t, flatePath, 1, "Im0")
	ascii85Decode := decodePDFPageImageObjectToRGBForProbe(t, ascii85Path, 1, "Im0")

	baseParity := measureSyntheticPDFPlacementProbeAgainstPopplerAtDPIWithMode(
		t, "doc007_page4_icc_transport_base.pdf", basePDF, 150, 2, domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
	)
	flateParity := measureSyntheticPDFPlacementProbeAgainstPopplerAtDPIWithMode(
		t, "doc007_page4_icc_transport_flate.pdf", flatePDF, 150, 2, domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
	)
	ascii85Parity := measureSyntheticPDFPlacementProbeAgainstPopplerAtDPIWithMode(
		t, "doc007_page4_icc_transport_ascii85.pdf", ascii85PDF, 150, 2, domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
	)

	t.Logf(
		"doc007 page4 ICC transport decode axis: original=%s base=%s flate=%s ascii85=%s",
		originalDecode.rgbSHA256,
		baseDecode.rgbSHA256,
		flateDecode.rgbSHA256,
		ascii85Decode.rgbSHA256,
	)
	t.Logf(
		"doc007 page4 ICC transport parity axis @150dpi: base=%.4f flate=%.4f ascii85=%.4f",
		baseParity.originalSimilarity,
		flateParity.originalSimilarity,
		ascii85Parity.originalSimilarity,
	)

	assert.NotEqual(t, originalDecode.rgbSHA256, baseDecode.rgbSHA256)
	assert.Equal(t, originalDecode.rgbSHA256, flateDecode.rgbSHA256)
	assert.Equal(t, originalDecode.rgbSHA256, ascii85Decode.rgbSHA256)
	assert.Greater(t, baseParity.originalSimilarity, flateParity.originalSimilarity)
	assert.InDelta(t, flateParity.originalSimilarity, ascii85Parity.originalSimilarity, 1e-4)
}

func TestSyntheticDoc007MainPage4ICCProfileStreamTransportFingerprintProbe(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))
	originalICC := measurePDFPageImageICCProfileForProbe(t, pdfPath, 4, "Im3")

	root := t.TempDir()
	basePath := filepath.Join(root, "base.pdf")
	flatePath := filepath.Join(root, "flate.pdf")
	ascii85Path := filepath.Join(root, "ascii85.pdf")

	require.NoError(t, os.WriteFile(basePath, buildSyntheticDCTICCBasedGrayImagePDFFloatWithICCStreamOptions(
		3.84, 3.84,
		imageData.Width, imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
		originalICC.alternate,
		domainimage.FilterNone,
	), 0o644))
	require.NoError(t, os.WriteFile(flatePath, buildSyntheticDCTICCBasedGrayImagePDFFloatWithICCStreamOptions(
		3.84, 3.84,
		imageData.Width, imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
		originalICC.alternate,
		domainimage.FilterFlate,
	), 0o644))
	require.NoError(t, os.WriteFile(ascii85Path, buildSyntheticDCTICCBasedGrayImagePDFFloatWithICCStreamOptions(
		3.84, 3.84,
		imageData.Width, imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
		originalICC.alternate,
		domainimage.FilterASCII85,
	), 0o644))

	original := measurePDFPageImageICCProfileStreamFingerprintForProbe(t, pdfPath, 4, "Im3")
	base := measurePDFPageImageICCProfileStreamFingerprintForProbe(t, basePath, 1, "Im0")
	flate := measurePDFPageImageICCProfileStreamFingerprintForProbe(t, flatePath, 1, "Im0")
	ascii85 := measurePDFPageImageICCProfileStreamFingerprintForProbe(t, ascii85Path, 1, "Im0")

	t.Logf("doc007 page4 ICC stream fingerprint original=%+v", original)
	t.Logf("doc007 page4 ICC stream fingerprint base=%+v", base)
	t.Logf("doc007 page4 ICC stream fingerprint flate=%+v", flate)
	t.Logf("doc007 page4 ICC stream fingerprint ascii85=%+v", ascii85)

	assert.Equal(t, original.filter, domainimage.FilterASCII85)
	assert.Equal(t, base.decodedSHA256, base.rawSHA256)
	assert.NotEqual(t, original.decodedSHA256, base.decodedSHA256)
	assert.Equal(t, original.decodedSHA256, flate.decodedSHA256)
	assert.Equal(t, original.decodedSHA256, ascii85.decodedSHA256)
	assert.NotEqual(t, flate.rawSHA256, ascii85.rawSHA256)
}

func TestSyntheticDoc007MainPage4OriginalAndSyntheticRenderBridgeProbeAt150DPI(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))
	syntheticPDF := buildSyntheticDCTICCBasedGrayImagePDFFloat(
		3.84,
		3.84,
		imageData.Width,
		imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
	)

	originalPoppler, originalOurs := renderSamplePageAgainstPopplerAtDPIToPNGs(
		t,
		pdfPath,
		4,
		domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
		150,
	)
	syntheticPoppler, syntheticOurs := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(
		t,
		"doc007_page4_exact_bridge.pdf",
		syntheticPDF,
		domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
		150,
	)

	popplerExact, popplerSimilarity, err := parityComparePNGs(syntheticPoppler, originalPoppler)
	require.NoError(t, err)
	oursExact, oursSimilarity, err := parityComparePNGs(syntheticOurs, originalOurs)
	require.NoError(t, err)

	t.Logf(
		"doc007 page4 original-vs-synthetic bridge @150dpi: poppler exact=%.4f sim=%.4f ours exact=%.4f sim=%.4f",
		popplerExact,
		popplerSimilarity,
		oursExact,
		oursSimilarity,
	)
}

func TestSyntheticDoc007MainPage4OriginalAndSyntheticASCII85FilteredRenderBridgeProbeAt150DPI(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))
	originalICC := measurePDFPageImageICCProfileForProbe(t, pdfPath, 4, "Im3")
	require.Equal(t, domainimage.FilterASCII85, originalICC.filter)

	syntheticPDF := buildSyntheticDCTICCBasedGrayImagePDFFloatWithICCStreamOptions(
		3.84,
		3.84,
		imageData.Width,
		imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
		originalICC.alternate,
		originalICC.filter,
	)

	originalPoppler, originalOurs := renderSamplePageAgainstPopplerAtDPIToPNGs(
		t,
		pdfPath,
		4,
		domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
		150,
	)
	syntheticPoppler, syntheticOurs := renderSyntheticPDFAgainstPopplerAtDPIToPNGs(
		t,
		"doc007_page4_exact_bridge_ascii85.pdf",
		syntheticPDF,
		domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
		150,
	)

	popplerExact, popplerSimilarity, err := parityComparePNGs(syntheticPoppler, originalPoppler)
	require.NoError(t, err)
	oursExact, oursSimilarity, err := parityComparePNGs(syntheticOurs, originalOurs)
	require.NoError(t, err)

	assert.InDelta(t, 100.0, popplerExact, 1e-9)
	assert.InDelta(t, 100.0, popplerSimilarity, 1e-9)
	t.Logf(
		"doc007 page4 original-vs-synthetic ASCII85 bridge @150dpi: poppler exact=%.4f sim=%.4f ours exact=%.4f sim=%.4f",
		popplerExact,
		popplerSimilarity,
		oursExact,
		oursSimilarity,
	)
}

func TestSyntheticDoc007MainPage4OriginalAndSyntheticDecodeBridgeProbe(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))
	syntheticPDF := buildSyntheticDCTICCBasedGrayImagePDFFloat(
		3.84,
		3.84,
		imageData.Width,
		imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
	)
	syntheticPath := filepath.Join(t.TempDir(), "doc007_page4_exact_decode_bridge.pdf")
	require.NoError(t, os.WriteFile(syntheticPath, syntheticPDF, 0o644))

	original := decodePDFPageImageObjectToRGBForProbe(t, pdfPath, 4, "Im3")
	synthetic := decodePDFPageImageObjectToRGBForProbe(t, syntheticPath, 1, "Im0")

	assert.Equal(t, original.width, synthetic.width)
	assert.Equal(t, original.height, synthetic.height)
	assert.Equal(t, original.colorSpace, synthetic.colorSpace)
	assert.Equal(t, original.bitsPerComponent, synthetic.bitsPerComponent)
	assert.Equal(t, original.rgbLength, synthetic.rgbLength)
	assert.NotEqual(t, original.rgbSHA256, synthetic.rgbSHA256)

	t.Logf(
		"doc007 page4 original-vs-synthetic decode bridge: original=%+v synthetic=%+v",
		original,
		synthetic,
	)
}

func TestSyntheticDoc007MainPage4OriginalAndSyntheticImageDataFingerprintBridgeProbe(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))
	syntheticPDF := buildSyntheticDCTICCBasedGrayImagePDFFloat(
		3.84,
		3.84,
		imageData.Width,
		imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
	)

	root := t.TempDir()
	syntheticPath := filepath.Join(root, "doc007_page4_fingerprint_bridge.pdf")
	require.NoError(t, os.WriteFile(syntheticPath, syntheticPDF, 0o644))

	original := fingerprintPDFPageImageDataForProbe(t, pdfPath, 4, "Im3")
	synthetic := fingerprintPDFPageImageDataForProbe(t, syntheticPath, 1, "Im0")

	assert.Equal(t, original.filter, synthetic.filter)
	assert.Equal(t, original.colorSpace, synthetic.colorSpace)
	assert.Equal(t, original.bitsPerComponent, synthetic.bitsPerComponent)
	assert.Equal(t, original.width, synthetic.width)
	assert.Equal(t, original.height, synthetic.height)
	assert.Equal(t, original.decode, synthetic.decode)
	assert.Equal(t, original.decodeParms, synthetic.decodeParms)
	assert.Equal(t, original.iccComponents, synthetic.iccComponents)
	assert.Equal(t, original.dataSHA256, synthetic.dataSHA256)
	assert.NotEqual(t, original.iccProfileSHA256, synthetic.iccProfileSHA256)

	t.Logf("doc007 page4 original image data fingerprint=%+v", original)
	t.Logf("doc007 page4 synthetic image data fingerprint=%+v", synthetic)
}

func TestSyntheticDoc007MainPage4BuilderInputICCProfileProbe(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()

	originalByPage := fingerprintPDFPageImageDataForProbe(t, pdfPath, 4, "Im3")
	builderInput := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))

	assert.Equal(t, originalByPage.filter, builderInput.Filter)
	assert.Equal(t, originalByPage.colorSpace, string(builderInput.ColorSpace))
	assert.Equal(t, originalByPage.bitsPerComponent, builderInput.BitsPerComponent)
	assert.Equal(t, originalByPage.width, builderInput.Width)
	assert.Equal(t, originalByPage.height, builderInput.Height)
	assert.Equal(t, originalByPage.dataSHA256, sha256HexForProbe(builderInput.Data))
	assert.Equal(t, originalByPage.decode, builderInput.Decode)
	assert.Equal(t, originalByPage.decodeParms, builderInput.DecodeParms)
	assert.Equal(t, originalByPage.iccComponents, builderInput.ICCComponents)

	t.Logf(
		"doc007 page4 builder input ICC probe: page_icc=%s builder_icc=%s",
		originalByPage.iccProfileSHA256,
		sha256HexForProbe(builderInput.ICCProfile),
	)
}

func TestSyntheticDoc007MainPage4OriginalAndSyntheticImageSamplingTraceBridgeProbe(t *testing.T) {
	pdfPath := sampleDirDoc007MainPDFForProbe()
	imageData := loadSampleEncodedImageData(t, pdfPath, entity.NewRef(56, 0))
	syntheticPDF := buildSyntheticDCTICCBasedGrayImagePDFFloat(
		3.84,
		3.84,
		imageData.Width,
		imageData.Height,
		[6]float64{3.84, 0, 0, 3.84, 0, 0},
		imageData.Data,
		imageData.ICCProfile,
		imageData.ICCComponents,
	)

	original := renderSamplePageImageSamplingTraceForProbe(
		t,
		pdfPath,
		4,
		domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
		150,
	)
	synthetic := renderSyntheticPDFImageSamplingTraceForProbe(
		t,
		"doc007_page4_trace_bridge.pdf",
		syntheticPDF,
		domainrenderer.ImageSamplingModeAdaptiveDCTICCBasedV1,
		150,
	)

	require.Len(t, original, 1)
	require.Len(t, synthetic, 1)

	assert.Equal(t, original[0].filter, synthetic[0].filter)
	assert.Equal(t, original[0].colorSpace, synthetic[0].colorSpace)
	assert.Equal(t, original[0].sampler, synthetic[0].sampler)
	assert.Equal(t, original[0].reason, synthetic[0].reason)
	assert.Equal(t, original[0].experimentalCandidate, synthetic[0].experimentalCandidate)
	assert.Equal(t, original[0].srcW, synthetic[0].srcW)
	assert.Equal(t, original[0].srcH, synthetic[0].srcH)
	assert.Equal(t, original[0].ctm, synthetic[0].ctm)
	assert.InDelta(t, original[0].phaseX, synthetic[0].phaseX, 1e-9)
	assert.InDelta(t, original[0].phaseY, synthetic[0].phaseY, 1e-9)
	assert.InDelta(t, original[0].dstX, synthetic[0].dstX, 1e-9)
	assert.InDelta(t, original[0].dstY, synthetic[0].dstY, 1e-9)
	assert.InDelta(t, original[0].dstW, synthetic[0].dstW, 1e-9)
	assert.InDelta(t, original[0].dstH, synthetic[0].dstH, 1e-9)

	t.Logf("doc007 page4 original trace=%+v", original[0])
	t.Logf("doc007 page4 synthetic trace=%+v", synthetic[0])
}

func maxFloatForDoc007ExactPlacementProbe(values ...float64) float64 {
	best := values[0]
	for _, v := range values[1:] {
		if v > best {
			best = v
		}
	}
	return best
}
