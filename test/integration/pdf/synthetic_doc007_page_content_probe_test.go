package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSampleDecodeOrTransform007MainPage4ImagePlacementOpsProbe(t *testing.T) {
	results := measureSamplePageImagePlacementOpsForProbe(t, sampleDirDoc007MainPDFForProbe(), 4)

	require.Len(t, results, 1)
	assert.Equal(t, "Im3", results[0].imageName)
	assert.Equal(t, [6]float64{3.84, 0, 0, 3.84, 0, 0}, results[0].matrix)
	t.Logf("007 main page4 image placement matrix=%v", results[0].matrix)
}

func TestSampleDecodeOrTransform007MainPage1ImagePlacementOpsProbe(t *testing.T) {
	results := measureSamplePageImagePlacementOpsForProbe(t, sampleDirDoc007MainPDFForProbe(), 1)

	require.Len(t, results, 1)
	assert.Equal(t, "Im0", results[0].imageName)
	assert.Equal(t, [6]float64{3.84, 0, 0, 3.84, 0, 0}, results[0].matrix)
	t.Logf("007 main page1 image placement matrix=%v", results[0].matrix)
}

func TestSampleDecodeOrTransform007MainPageOperatorHistogramProbe(t *testing.T) {
	page1 := measureSamplePageOperatorHistogramForProbe(t, sampleDirDoc007MainPDFForProbe(), 1)
	page4 := measureSamplePageOperatorHistogramForProbe(t, sampleDirDoc007MainPDFForProbe(), 4)

	assert.Equal(t, 1, page1.streamCount)
	assert.Equal(t, 1, page4.streamCount)
	assert.Equal(t, map[string]int{"q": 1, "cm": 1, "Do": 1, "Q": 1}, page4.operatorCounts)
	assert.Equal(t, map[string]int{"q": 1, "cm": 1, "Do": 1, "Q": 1, "BT": 1, "ET": 1, "Tj": 1}, page1.operatorCounts)
	t.Logf("007 main page1 operator histogram=%v", page1.operatorCounts)
	t.Logf("007 main page4 operator histogram=%v", page4.operatorCounts)
}

func TestSampleDecodeOrTransform007MainPageImageObjectProbe(t *testing.T) {
	page1 := measureSamplePageImageObjectForProbe(t, sampleDirDoc007MainPDFForProbe(), 1, "Im0")
	page4 := measureSamplePageImageObjectForProbe(t, sampleDirDoc007MainPDFForProbe(), 4, "Im3")

	assert.Equal(t, 16, page1.width)
	assert.Equal(t, 16, page1.height)
	assert.Equal(t, 8, page1.bitsPerComponent)
	assert.Equal(t, 16, page4.width)
	assert.Equal(t, 16, page4.height)
	assert.Equal(t, 8, page4.bitsPerComponent)
	assert.NotEmpty(t, page1.filter)
	assert.NotEmpty(t, page4.filter)
	assert.NotEmpty(t, page1.colorSpace)
	assert.NotEmpty(t, page4.colorSpace)
	assert.Greater(t, page1.rawStreamLength, 0)
	assert.Greater(t, page4.rawStreamLength, 0)
	assert.NotZero(t, page1.decodedDataLength)
	assert.NotZero(t, page4.decodedDataLength)

	t.Logf(
		"007 main page1 image object: ref=%v filter=%s colorspace=%s decode=%v decodeParms=%v icc_len=%d icc_n=%d raw=%d decoded=%d",
		page1.imageRef,
		page1.filter,
		page1.colorSpace,
		page1.decode,
		page1.decodeParms,
		page1.iccProfileLength,
		page1.iccComponents,
		page1.rawStreamLength,
		page1.decodedDataLength,
	)
	t.Logf(
		"007 main page4 image object: ref=%v filter=%s colorspace=%s decode=%v decodeParms=%v icc_len=%d icc_n=%d raw=%d decoded=%d",
		page4.imageRef,
		page4.filter,
		page4.colorSpace,
		page4.decode,
		page4.decodeParms,
		page4.iccProfileLength,
		page4.iccComponents,
		page4.rawStreamLength,
		page4.decodedDataLength,
	)
}

func TestSampleDecodeOrTransform007MainPageLayoutProbe(t *testing.T) {
	page1 := measureSamplePageLayoutForProbe(t, sampleDirDoc007MainPDFForProbe(), 1)
	page4 := measureSamplePageLayoutForProbe(t, sampleDirDoc007MainPDFForProbe(), 4)

	assert.Equal(t, page1.mediaBox, page1.cropBox)
	assert.Equal(t, page4.mediaBox, page4.cropBox)

	t.Logf("007 main page1 layout: media=%v crop=%v", page1.mediaBox, page1.cropBox)
	t.Logf("007 main page4 layout: media=%v crop=%v", page4.mediaBox, page4.cropBox)
}
