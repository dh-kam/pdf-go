package pdf_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
)

func TestSampleDecodeOrTransform007MainPage4PlacementProbeAt150DPI(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-images.pdf")

	result := measureSamplePagePlacementProbeAgainstPopplerAtDPI(
		t,
		pdfPath,
		4,
		domainrenderer.ImageSamplingModeLegacy,
		150,
		2,
	)

	require.True(t, result.hasDiffBounds)
	assert.Equal(t, 8, result.diffBounds.Dx())
	assert.Equal(t, 8, result.diffBounds.Dy())
	assert.Equal(t, -2, result.bestShiftX)
	assert.Equal(t, 2, result.bestShiftY)
	assert.Greater(t, result.bestShiftSimilarity, result.originalSimilarity)
	assert.Greater(t, result.improvement(), 4.0)
}

func TestSampleDecodeOrTransform007MainPage1PlacementProbeAt150DPI(t *testing.T) {
	pdfPath := filepath.Join(getSampleDir(), "007-imagemagick-images", "imagemagick-images.pdf")

	result := measureSamplePagePlacementProbeAgainstPopplerAtDPI(
		t,
		pdfPath,
		1,
		domainrenderer.ImageSamplingModeLegacy,
		150,
		2,
	)

	require.True(t, result.hasDiffBounds)
	assert.Equal(t, -1, result.bestShiftX)
	assert.Equal(t, -2, result.bestShiftY)
	assert.Greater(t, result.bestShiftSimilarity, result.originalSimilarity)
	assert.Greater(t, result.improvement(), 4.0)
}
