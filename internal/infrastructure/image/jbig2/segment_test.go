package jbig2

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNativeSegmentHeader_PageInformation(t *testing.T) {
	header, err := parseNativeSegmentHeader(testJBIG2EmbeddedPageInfo(23, 7))
	require.NoError(t, err)

	assert.Equal(t, uint32(1), header.number)
	assert.Equal(t, SegmentPageInformation, header.typ)
	assert.Equal(t, uint32(1), header.pageAssociation)
	assert.Equal(t, uint32(19), header.dataLength)
	assert.Equal(t, 11, header.headerLength)
	assert.False(t, header.deferredNonRetain)
	assert.False(t, header.largePageAssociation)
	assert.Empty(t, header.referredToSegmentNumbers)
}

func TestParseNativeDocument_ExtractsStandalonePageInfo(t *testing.T) {
	data := append([]byte{}, jbig2FileSignature...)
	data = append(data,
		0x00,                   // sequential organization, known page count
		0x00, 0x00, 0x00, 0x01, // one page
	)
	data = append(data, testJBIG2EmbeddedPageInfo(23, 7)...)

	doc, err := parseNativeDocument(data)
	require.NoError(t, err)

	require.NotNil(t, doc.pageInfo)
	assert.Equal(t, 23, doc.pageInfo.Width)
	assert.Equal(t, 7, doc.pageInfo.Height)
	assert.True(t, doc.fileHeader.standaloneFile)
	assert.True(t, doc.fileHeader.sequential)
	assert.Equal(t, uint32(1), doc.fileHeader.numberOfPages)
}

func TestParseNativeDocument_ExtractsEmbeddedPageInfo(t *testing.T) {
	doc, err := parseNativeDocument(testJBIG2EmbeddedPageInfo(31, 11))
	require.NoError(t, err)

	require.NotNil(t, doc.pageInfo)
	assert.Equal(t, 31, doc.pageInfo.Width)
	assert.Equal(t, 11, doc.pageInfo.Height)
	assert.False(t, doc.fileHeader.standaloneFile)
}

func TestParseNativeDocument_ExtractsPageDefaultPixel(t *testing.T) {
	doc, err := parseNativeDocument(testJBIG2EmbeddedPageInfoWithFlags(31, 11, 0x04))
	require.NoError(t, err)

	require.NotNil(t, doc.pageInfo)
	assert.True(t, doc.pageInfo.DefaultPixel)
}

func TestParseNativeSegmentHeader_RejectsTruncatedHeader(t *testing.T) {
	_, err := parseNativeSegmentHeader([]byte{0x00, 0x00, 0x00, 0x01, 0x30})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "truncated")
}

func TestParseGenericRegionSegment_ParsesMMRRegionHeader(t *testing.T) {
	data := testRegionInfo(13, 7, 2, 3, 0x02)
	data = append(data,
		0x01, // MMR generic region, template 0
		0xaa, 0x55,
	)

	region, err := parseGenericRegionSegment(data)
	require.NoError(t, err)

	assert.Equal(t, 13, region.info.width)
	assert.Equal(t, 7, region.info.height)
	assert.Equal(t, 2, region.info.x)
	assert.Equal(t, 3, region.info.y)
	assert.Equal(t, byte(0x02), region.info.flags)
	assert.True(t, region.mmr)
	assert.Equal(t, byte(0), region.template)
	assert.Equal(t, []byte{0xaa, 0x55}, region.payload)
}

func TestParseGenericRegionSegment_RejectsTruncatedRegionInfo(t *testing.T) {
	_, err := parseGenericRegionSegment([]byte{0x00, 0x00, 0x00})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "truncated region info")
}

func TestParseGenericRegionSegment_ParsesArithmeticTemplate0ATPixels(t *testing.T) {
	data := testRegionInfo(13, 7, 0, 0, 0x00)
	data = append(data,
		0x00,                   // arithmetic generic region, template 0
		0x03, 0xff, 0xfd, 0xff, // AT[0], AT[1]
		0x02, 0xfe, 0xfe, 0xfe, // AT[2], AT[3]
		0xcc, // payload
	)

	region, err := parseGenericRegionSegment(data)
	require.NoError(t, err)

	assert.False(t, region.mmr)
	assert.Equal(t, byte(0), region.template)
	assert.Equal(t, []adaptiveTemplatePixel{
		{x: 3, y: -1},
		{x: -3, y: -1},
		{x: 2, y: -2},
		{x: -2, y: -2},
	}, region.atPixels)
	assert.Equal(t, []byte{0xcc}, region.payload)
}

func TestParseGenericRegionSegment_RejectsTruncatedAdaptiveTemplate(t *testing.T) {
	data := testRegionInfo(13, 7, 0, 0, 0x00)
	data = append(data,
		0x00, // arithmetic generic region, template 0 needs four AT pixels
		0x03, 0xff,
	)

	_, err := parseGenericRegionSegment(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "truncated adaptive template")
}

func TestParseGenericRegionSegment_RejectsInvalidDimensions(t *testing.T) {
	data := testRegionInfo(0, 7, 0, 0, 0x00)
	data = append(data, 0x01) // MMR path avoids AT pixels

	_, err := parseGenericRegionSegment(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid region dimensions")
}

func testRegionInfo(width, height, x, y int, flags byte) []byte {
	data := make([]byte, 17)
	binary.BigEndian.PutUint32(data[0:4], uint32(width))
	binary.BigEndian.PutUint32(data[4:8], uint32(height))
	binary.BigEndian.PutUint32(data[8:12], uint32(x))
	binary.BigEndian.PutUint32(data[12:16], uint32(y))
	data[16] = flags
	return data
}
