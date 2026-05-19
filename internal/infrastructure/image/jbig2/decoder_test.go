package jbig2

import (
	"image"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapper_DelegatesToDecoder(t *testing.T) {
	w := NewWrapper()

	cfg, err := w.DecodeConfig([]byte{
		0x97, 0x4A, 0x42, 0x32, 0x0D, 0x0A, 0x1A, 0x0A,
	})
	require.NoError(t, err)
	assert.True(t, cfg.Width > 0)
	assert.True(t, cfg.Height > 0)

	assert.Equal(t, "DeviceGray", string(w.ColorSpace()))
}

func TestWrapper_DecodeFallbackPath(t *testing.T) {
	w := NewWrapper()
	img, err := w.Decode(testJBIG2EmbeddedPageInfo(16, 8))
	require.NoError(t, err)
	assert.NotNil(t, img)
	assert.Equal(t, 16, img.Bounds().Dx())
	assert.Equal(t, 8, img.Bounds().Dy())
}

func TestNewNativeDecoder_ForcesGoPath(t *testing.T) {
	decoder := NewNativeDecoder()
	img, err := decoder.Decode(testJBIG2EmbeddedPageInfo(12, 5))
	require.NoError(t, err)

	assert.Equal(t, 12, img.Bounds().Dx())
	assert.Equal(t, 5, img.Bounds().Dy())
}

func TestNativeDecoder_UsesPageDefaultPixel(t *testing.T) {
	decoder := NewNativeDecoder()
	img, err := decoder.Decode(testJBIG2EmbeddedPageInfoWithFlags(4, 2, 0x04))
	require.NoError(t, err)

	gray := img.(*image.Gray)
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			assert.Equal(t, uint8(0x00), gray.GrayAt(x, y).Y)
		}
	}
}

func TestNativeDecoder_DecodesMMRGenericRegionAllWhiteLines(t *testing.T) {
	decoder := NewNativeDecoder()
	data := append([]byte{}, testJBIG2EmbeddedPageInfo(12, 5)...)
	regionBody := append(testRegionInfo(8, 2, 0, 0, 0), 0x01, 0b11000000)
	data = append(data,
		0x00, 0x00, 0x00, 0x02, // segment number
		byte(SegmentImmediateGenericRegion),
		0x00,                                    // no referred-to segments
		0x01,                                    // page association
		0x00, 0x00, 0x00, byte(len(regionBody)), // segment data length
	)
	data = append(data, regionBody...)

	img, err := decoder.Decode(data)
	require.NoError(t, err)
	assert.Equal(t, 12, img.Bounds().Dx())
	assert.Equal(t, 5, img.Bounds().Dy())
	assert.Equal(t, uint8(0xff), img.(*image.Gray).GrayAt(0, 0).Y)
}

func TestNativeDecoder_ComposesGenericRegionOnPage(t *testing.T) {
	decoder := NewNativeDecoder()
	data := append([]byte{}, testJBIG2EmbeddedPageInfo(12, 5)...)
	regionBody := append(testRegionInfo(8, 2, 2, 1, 0), 0x01)
	regionBody = append(regionBody, testBits("001"+"1011"+"011"+"001"+"1011"+"011")...)
	data = append(data,
		0x00, 0x00, 0x00, 0x02, // segment number
		byte(SegmentImmediateGenericRegion),
		0x00,                                    // no referred-to segments
		0x01,                                    // page association
		0x00, 0x00, 0x00, byte(len(regionBody)), // segment data length
	)
	data = append(data, regionBody...)

	img, err := decoder.Decode(data)
	require.NoError(t, err)

	gray := img.(*image.Gray)
	assert.Equal(t, 12, gray.Bounds().Dx())
	assert.Equal(t, 5, gray.Bounds().Dy())
	assert.Equal(t, uint8(0xff), gray.GrayAt(5, 1).Y)
	for x := 6; x <= 9; x++ {
		assert.Equal(t, uint8(0x00), gray.GrayAt(x, 1).Y)
	}
	assert.Equal(t, uint8(0xff), gray.GrayAt(10, 1).Y)
}

func TestNativeDecoder_ComposesMultipleImmediateGenericRegionsInSegmentOrder(t *testing.T) {
	decoder := NewNativeDecoder()
	data := append([]byte{}, testJBIG2EmbeddedPageInfo(12, 1)...)
	data = appendJBIG2Segment(data, 2, SegmentImmediateGenericRegion, 1, testMMRAllBlackGenericRegionBody(8, 1, 0, 0, 0))
	data = appendJBIG2Segment(data, 3, SegmentImmediateGenericRegion, 1, testMMRAllBlackGenericRegionBody(8, 1, 4, 0, 0))

	img, err := decoder.Decode(data)
	require.NoError(t, err)

	gray := img.(*image.Gray)
	assert.Equal(t, 12, gray.Bounds().Dx())
	assert.Equal(t, 1, gray.Bounds().Dy())
	for x := 0; x < 12; x++ {
		assert.Equal(t, uint8(0x00), gray.GrayAt(x, 0).Y)
	}
}

func TestNativeDecoder_DiscardsReferencedBitmapAfterGenericRefinement(t *testing.T) {
	decoder := NewNativeDecoder()
	data := append([]byte{}, testJBIG2EmbeddedPageInfo(1, 1)...)
	data = appendJBIG2Segment(data, 2, SegmentIntermediateGenericRegion, 1, testMMRAllWhiteGenericRegionBody(1, 1, 0, 0, 0))
	data = appendJBIG2SegmentWithRefs(data, 3, SegmentIntermediateGenericRefinementRegion, 1, []uint32{2}, testGenericRefinementRegionBody(1, 1, 0, 0, 0))
	data = appendJBIG2SegmentWithRefs(data, 4, SegmentImmediateGenericRefinementRegion, 1, []uint32{2}, testGenericRefinementRegionBody(1, 1, 0, 0, 0))

	_, err := decoder.Decode(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing referenced bitmap 2")
}

func TestNativeDecoder_DecodeWithOptionsPrependsGlobalsWithoutRenderingThem(t *testing.T) {
	decoder := NewNativeDecoder()
	data := append([]byte{}, testJBIG2EmbeddedPageInfo(12, 5)...)
	regionBody := append(testRegionInfo(8, 2, 0, 0, 0), 0x01, 0b11000000)
	data = append(data,
		0x00, 0x00, 0x00, 0x02, // segment number
		byte(SegmentImmediateGenericRegion),
		0x00,                                    // no referred-to segments
		0x01,                                    // page association
		0x00, 0x00, 0x00, byte(len(regionBody)), // segment data length
	)
	data = append(data, regionBody...)

	img, err := decoder.DecodeWithOptions(data, DecodeOptions{
		Globals: testJBIG2GlobalSymbolDictionary(),
	})
	require.NoError(t, err)
	assert.Equal(t, 12, img.Bounds().Dx())
	assert.Equal(t, 5, img.Bounds().Dy())
}

func TestNativeDecoder_FallbackDecodesTextRegionWithGlobalsWithoutPageInfo(t *testing.T) {
	decoder := NewNativeDecoder()
	textBody := append(testRegionInfo(1, 1, 0, 0, 0),
		0x00, 0x01, // Huffman text region.
		0x00, 0x00, // Default Huffman tables.
		0x00, 0x00, 0x00, 0x01, // One instance.
	)
	textBody = append(textBody, testBits(
		textRegionOneSymbolCodeTablePrefixBits()+
			"0"+"000"+ // One symbol-code prefix run, then reset to next byte.
			"0"+ // Initial T = 1.
			"0"+ // Delta T = 1, placing the strip at T = 0.
			"00"+"0000000"+ // First S = 0 via table F.
			"0"+ // Symbol ID 0.
			"01", // Delta S OOB via table H.
	)...)
	data := appendJBIG2SegmentWithRefs(nil, 2, SegmentImmediateText, 1, []uint32{1}, textBody)

	img, err := decoder.DecodeWithOptions(data, DecodeOptions{
		Globals: testJBIG2GlobalOneBlackSymbolDictionary(),
		Width:   1,
		Height:  1,
	})
	require.NoError(t, err)

	gray := img.(*image.Gray)
	assert.Equal(t, 1, gray.Bounds().Dx())
	assert.Equal(t, 1, gray.Bounds().Dy())
	assert.Equal(t, uint8(0x00), gray.GrayAt(0, 0).Y)
}

func TestNativeDecoder_GlobalsTakePriorityWhenSegmentNumbersCollide(t *testing.T) {
	decoder := NewNativeDecoder()
	data := appendJBIG2Segment(nil, 1, SegmentSymbolDictionary, 0, testEmptySymbolDictionaryBody())
	textBody := append(testRegionInfo(1, 1, 0, 0, 0),
		0x00, 0x01, // Huffman text region.
		0x00, 0x00, // Default Huffman tables.
		0x00, 0x00, 0x00, 0x01, // One instance.
	)
	textBody = append(textBody, testBits(
		textRegionOneSymbolCodeTablePrefixBits()+
			"0"+"000"+ // One symbol-code prefix run, then reset to next byte.
			"0"+ // Initial T = 1.
			"0"+ // Delta T = 1, placing the strip at T = 0.
			"00"+"0000000"+ // First S = 0 via table F.
			"0"+ // Symbol ID 0.
			"01", // Delta S OOB via table H.
	)...)
	data = appendJBIG2SegmentWithRefs(data, 2, SegmentImmediateText, 1, []uint32{1}, textBody)

	img, err := decoder.DecodeWithOptions(data, DecodeOptions{
		Globals: testJBIG2GlobalOneBlackSymbolDictionary(),
		Width:   1,
		Height:  1,
	})
	require.NoError(t, err)

	gray := img.(*image.Gray)
	assert.Equal(t, uint8(0x00), gray.GrayAt(0, 0).Y)
}

func TestNativeDecoder_DecodesArithmeticBitmapRegions(t *testing.T) {
	decoder := NewNativeDecoder()
	data := append([]byte{}, testJBIG2EmbeddedPageInfo(12, 5)...)
	regionBody := append(testRegionInfo(8, 1, 0, 0, 0),
		0x00,
		0x03, 0xff, 0xfd, 0xff,
		0x02, 0xfe, 0xfe, 0xfe,
		0x00, 0x00,
	)
	data = append(data,
		0x00, 0x00, 0x00, 0x02, // segment number
		byte(SegmentImmediateGenericRegion),
		0x00,                                    // no referred-to segments
		0x01,                                    // page association
		0x00, 0x00, 0x00, byte(len(regionBody)), // segment data length
	)
	data = append(data, regionBody...)

	img, err := decoder.Decode(data)
	require.NoError(t, err)
	assert.Equal(t, 12, img.Bounds().Dx())
	assert.Equal(t, 5, img.Bounds().Dy())
}

func TestNativeDecoder_ValidatesGenericRegionBodyBeforeNotImplemented(t *testing.T) {
	decoder := NewNativeDecoder()
	data := append([]byte{}, testJBIG2EmbeddedPageInfo(12, 5)...)
	data = append(data,
		0x00, 0x00, 0x00, 0x02, // segment number
		byte(SegmentImmediateGenericRegion),
		0x00,                   // no referred-to segments
		0x01,                   // page association
		0x00, 0x00, 0x00, 0x03, // segment data length
		0x00, 0x00, 0x00, // truncated region info
	)

	_, err := decoder.Decode(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "truncated region info")
}

func testJBIG2EmbeddedPageInfo(width, height int) []byte {
	return testJBIG2EmbeddedPageInfoWithFlags(width, height, 0x00)
}

func testJBIG2EmbeddedPageInfoWithFlags(width, height int, flags byte) []byte {
	data := make([]byte, 0, 33)
	data = append(data,
		0x00, 0x00, 0x00, 0x01, // segment number
		0x30,                   // page information segment
		0x00,                   // no referred-to segments
		0x01,                   // page association
		0x00, 0x00, 0x00, 0x13, // segment data length
	)
	data = append(data,
		byte(width>>24), byte(width>>16), byte(width>>8), byte(width),
		byte(height>>24), byte(height>>16), byte(height>>8), byte(height),
		0x00, 0x00, 0x00, 0x00, // x resolution
		0x00, 0x00, 0x00, 0x00, // y resolution
		flags,      // page flags
		0x00, 0x00, // default stripe size
	)
	return data
}

func appendJBIG2Segment(data []byte, number uint32, typ SegmentType, pageAssociation byte, body []byte) []byte {
	data = append(data,
		byte(number>>24), byte(number>>16), byte(number>>8), byte(number),
		byte(typ),
		0x00,
		pageAssociation,
		byte(len(body)>>24), byte(len(body)>>16), byte(len(body)>>8), byte(len(body)),
	)
	return append(data, body...)
}

func appendJBIG2SegmentWithRefs(data []byte, number uint32, typ SegmentType, pageAssociation byte, refs []uint32, body []byte) []byte {
	data = append(data,
		byte(number>>24), byte(number>>16), byte(number>>8), byte(number),
		byte(typ),
		byte(len(refs)<<5),
	)
	refSize := referredToSegmentNumberSize(number)
	for _, ref := range refs {
		switch refSize {
		case 1:
			data = append(data, byte(ref))
		case 2:
			data = append(data, byte(ref>>8), byte(ref))
		default:
			data = append(data, byte(ref>>24), byte(ref>>16), byte(ref>>8), byte(ref))
		}
	}
	data = append(data,
		pageAssociation,
		byte(len(body)>>24), byte(len(body)>>16), byte(len(body)>>8), byte(len(body)),
	)
	return append(data, body...)
}

func testMMRAllBlackGenericRegionBody(width, height, x, y int, flags byte) []byte {
	body := append(testRegionInfo(width, height, x, y, flags), 0x01)
	return append(body, 0x26, 0xa2, 0x80)
}

func testMMRAllWhiteGenericRegionBody(width, height, x, y int, flags byte) []byte {
	body := append(testRegionInfo(width, height, x, y, flags), 0x01)
	return append(body, 0x80)
}

func testGenericRefinementRegionBody(width, height, x, y int, flags byte) []byte {
	body := append(testRegionInfo(width, height, x, y, flags), 0x03)
	return append(body,
		0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff,
	)
}

func testJBIG2GlobalSymbolDictionary() []byte {
	body := testEmptySymbolDictionaryBody()
	data := []byte{
		0x00, 0x00, 0x00, 0x01, // segment number
		byte(SegmentSymbolDictionary),
		0x00, // no referred-to segments
		0x00, // global page association
		byte(len(body) >> 24), byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body)),
	}
	return append(data, body...)
}

func testJBIG2GlobalOneBlackSymbolDictionary() []byte {
	payload := append(testBits(
		"0"+ // Height delta = 1 via table D.
			"10"+ // Width delta = 1 via table B.
			"111111"+ // Width OOB via table B.
			"0"+"0000"+ // Collective bitmap size = 0 via table A.
			"00", // Reset to the next byte before the raw collective bitmap.
	),
		0x80, // One raw 1x1 black symbol.
	)
	payload = append(payload, testBits(
		"0"+"0000"+ // Skip run 0.
			"0"+"0001", // Export run 1.
	)...)
	body := []byte{
		0x00, 0x01, // Huffman symbol dictionary with default tables.
		0x00, 0x00, 0x00, 0x01, // exported symbols.
		0x00, 0x00, 0x00, 0x01, // new symbols.
	}
	body = append(body, payload...)
	data := []byte{
		0x00, 0x00, 0x00, 0x01, // segment number
		byte(SegmentSymbolDictionary),
		0x00, // no referred-to segments
		0x00, // global page association
		byte(len(body) >> 24), byte(len(body) >> 16), byte(len(body) >> 8), byte(len(body)),
	}
	return append(data, body...)
}

func testEmptySymbolDictionaryBody() []byte {
	return []byte{
		0x00, 0x00, // arithmetic, no refinement/aggregation, template 0.
		0x03, 0xff, // SDAT 0.
		0xfd, 0xff, // SDAT 1.
		0x02, 0xfe, // SDAT 2.
		0xfe, 0xfe, // SDAT 3.
		0x00, 0x00, 0x00, 0x00, // exported symbols.
		0x00, 0x00, 0x00, 0x00, // new symbols.
	}
}
