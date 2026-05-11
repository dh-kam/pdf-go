package pdf_test

import (
	"image"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseImageSamplingTraceForProbe_ParsesCoreFields(t *testing.T) {
	results := parseImageSamplingTraceForProbe(
		t,
		"[image-sampling] doc=test page=4 filter=DCTDecode colorspace=DeviceGray edge_candidate=rejected edge_mode=default gray_icc_candidate=candidate gray_icc_profile_mode=keep sampler=adaptive_downscale_bilinear_tiny_encoded_gray reason=auto_interpolate=false_downscale experimental_candidate=candidate_tiny_dct_iccbased_gray_downscale ctm=[150.000000 0.000000 0.000000 -150.000000 0.000000 8.000000] phase=(x=0.5000 y=0.5000) dst=(x=0.0000 y=8.0000 w=150.0000 h=150.0000) src=16x16",
	)

	require.Len(t, results, 1)
	assert.Equal(t, "DCTDecode", results[0].filter)
	assert.Equal(t, "DeviceGray", results[0].colorSpace)
	assert.Equal(t, "adaptive_downscale_bilinear_tiny_encoded_gray", results[0].sampler)
	assert.Equal(t, "auto_interpolate=false_downscale", results[0].reason)
	assert.Equal(t, "candidate_tiny_dct_iccbased_gray_downscale", results[0].experimentalCandidate)
	assert.Equal(t, [6]float64{150, 0, 0, -150, 0, 8}, results[0].ctm)
	assert.InDelta(t, 0.5, results[0].phaseX, 1e-9)
	assert.InDelta(t, 0.5, results[0].phaseY, 1e-9)
	assert.InDelta(t, 150.0, results[0].dstW, 1e-9)
	assert.InDelta(t, 150.0, results[0].dstH, 1e-9)
	assert.Equal(t, 16, results[0].srcW)
	assert.Equal(t, 16, results[0].srcH)
}

func TestImageDifferenceBoundsForProbe_ReturnsUnionOfDifferentPixels(t *testing.T) {
	left := filledRGBAForProbe(image.Rect(0, 0, 4, 4), [][]uint8{
		{0, 0, 0, 0},
		{0, 0, 0, 0},
		{0, 0, 0, 0},
		{0, 0, 0, 0},
	})
	right := filledRGBAForProbe(image.Rect(0, 0, 4, 4), [][]uint8{
		{0, 0, 0, 0},
		{0, 255, 0, 0},
		{0, 0, 0, 128},
		{0, 0, 0, 0},
	})

	result := imageDifferenceBoundsForProbe(left, right)
	require.True(t, result.hasDiff)
	assert.Equal(t, image.Rect(1, 1, 4, 3), result.bounds)
}

func TestBestShiftedImageParityForProbe_PrefersImprovingShift(t *testing.T) {
	poppler := filledRGBAForProbe(image.Rect(0, 0, 4, 4), [][]uint8{
		{0, 0, 0, 0},
		{0, 255, 0, 0},
		{0, 0, 128, 0},
		{0, 0, 0, 0},
	})
	ours := filledRGBAForProbe(image.Rect(0, 0, 4, 4), [][]uint8{
		{255, 0, 0, 0},
		{0, 128, 0, 0},
		{0, 0, 0, 0},
		{0, 0, 0, 0},
	})

	result := bestShiftedImageParityForProbe(t, poppler, ours, 1)
	assert.Equal(t, 1, result.shiftX)
	assert.Equal(t, 1, result.shiftY)
	assert.Greater(t, result.similarity, 0.0)
}
