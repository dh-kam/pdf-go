package jbig2

import (
	"image"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeGenericRegion_MMRAllWhiteVerticalZeroLines(t *testing.T) {
	data := testRegionInfo(8, 2, 0, 0, 0x00)
	data = append(data,
		0x01,       // MMR generic region
		0b11000000, // two vertical-0 line codes
	)
	region, err := parseGenericRegionSegment(data)
	require.NoError(t, err)

	img, err := decodeGenericRegion(region)
	require.NoError(t, err)

	assert.Equal(t, image.Rect(0, 0, 8, 2), img.Bounds())
	assert.Equal(t, []byte{
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	}, img.Pix)
}

func TestDecodeGenericRegion_ArithmeticTemplate0DecodesBitmap(t *testing.T) {
	data := testRegionInfo(8, 1, 0, 0, 0x00)
	data = append(data,
		0x00, // arithmetic template 0
		0x03, 0xff, 0xfd, 0xff,
		0x02, 0xfe, 0xfe, 0xfe,
		0x00, 0x00,
	)
	region, err := parseGenericRegionSegment(data)
	require.NoError(t, err)

	img, err := decodeGenericRegion(region)
	require.NoError(t, err)

	assert.Equal(t, image.Rect(0, 0, 8, 1), img.Bounds())
}

func TestGenericRegionTypicalPredictionContext(t *testing.T) {
	tests := []struct {
		template byte
		want     uint32
	}{
		{template: 0, want: 0x3953},
		{template: 1, want: 0x079a},
		{template: 2, want: 0x0e3},
		{template: 3, want: 0x18b},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, genericRegionTypicalPredictionContext(tt.template))
	}
}

func TestCombineBilevelPixel_UsesJBIG2CombinationOperators(t *testing.T) {
	tests := []struct {
		name string
		dst  byte
		src  byte
		op   byte
		want byte
	}{
		{name: "or paints source black", dst: 0xff, src: 0x00, op: jbig2CombineOr, want: 0x00},
		{name: "or preserves destination black", dst: 0x00, src: 0xff, op: jbig2CombineOr, want: 0x00},
		{name: "and keeps only overlapping black", dst: 0x00, src: 0xff, op: jbig2CombineAnd, want: 0xff},
		{name: "xor toggles unequal pixels", dst: 0xff, src: 0x00, op: jbig2CombineXor, want: 0x00},
		{name: "xor clears equal black pixels", dst: 0x00, src: 0x00, op: jbig2CombineXor, want: 0xff},
		{name: "xnor paints equal white pixels black", dst: 0xff, src: 0xff, op: jbig2CombineXnor, want: 0x00},
		{name: "replace copies source", dst: 0x00, src: 0xff, op: jbig2CombineReplace, want: 0xff},
		{name: "unknown operator leaves destination unchanged", dst: 0x00, src: 0xff, op: 7, want: 0x00},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, combineBilevelPixel(tt.dst, tt.src, tt.op))
		})
	}
}

func TestComposeRegionIntoPage_UsesExternalCombinationOperator(t *testing.T) {
	page := image.NewGray(image.Rect(0, 0, 2, 1))
	for i := range page.Pix {
		page.Pix[i] = 0xff
	}
	page.Pix[0] = 0x00

	region := image.NewGray(image.Rect(0, 0, 2, 1))
	region.Pix[0] = 0xff
	region.Pix[1] = 0x00

	composeRegionIntoPage(page, regionInfo{
		width:  2,
		height: 1,
		flags:  jbig2CombineAnd,
	}, region)

	assert.Equal(t, []byte{0xff, 0xff}, page.Pix)
}
