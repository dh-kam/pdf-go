package jbig2

import (
	"bytes"
	stdimage "image"
	"testing"
)

func TestParseGenericRefinementRegionSegment(t *testing.T) {
	data := append(testRegionInfo(4, 3, 2, 1, jbig2CombineReplace),
		0x02,
		0xfe, 0xff, 0x01, 0x00,
		0xaa, 0xbb,
	)

	region, err := parseGenericRefinementRegionSegment(data)
	if err != nil {
		t.Fatalf("parseGenericRefinementRegionSegment: %v", err)
	}
	if region.info.width != 4 || region.info.height != 3 || region.info.x != 2 || region.info.y != 1 {
		t.Fatalf("unexpected region info: %+v", region.info)
	}
	if region.template != 0 || !region.tpgrOn {
		t.Fatalf("unexpected flags: template=%d tpgrOn=%v", region.template, region.tpgrOn)
	}
	if len(region.atPixels) != 2 || region.payload[0] != 0xaa || region.payload[1] != 0xbb {
		t.Fatalf("unexpected AT/payload: %+v %x", region.atPixels, region.payload)
	}
}

func TestGenericRefinementTemplate0Context(t *testing.T) {
	ref := stdimage.NewGray(stdimage.Rect(0, 0, 4, 3))
	cur := stdimage.NewGray(stdimage.Rect(0, 0, 4, 3))
	fillWhite(ref)
	fillWhite(cur)
	setBlack(ref, 1, 0)
	setBlack(ref, 1, 1)
	setBlack(ref, 2, 1)
	setBlack(ref, 1, 2)
	setBlack(cur, 1, 0)
	setBlack(cur, 0, 1)

	region := genericRefinementRegion{
		ref:      ref,
		template: 0,
		atPixels: []adaptiveTemplatePixel{
			{x: -1, y: 0},
			{x: 1, y: 0},
		},
	}
	context := genericRefinementTemplate0Context(region, cur, 1, 1)
	if context >= uint32(refinementRegionStatsSize(0)) {
		t.Fatalf("context out of range: %d", context)
	}
	if context == 0 {
		t.Fatalf("expected non-zero refinement context")
	}
}

func TestGenericRefinementTypicalPixel(t *testing.T) {
	ref := stdimage.NewGray(stdimage.Rect(0, 0, 5, 3))
	fillWhite(ref)
	if bit, ok := genericRefinementTypicalPixel(ref, 0, 1, 0, 0); !ok || bit != 0 {
		t.Fatalf("all-white reference should produce typical white, bit=%d ok=%v", bit, ok)
	}

	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			setBlack(ref, x, y)
		}
	}
	if bit, ok := genericRefinementTypicalPixel(ref, 0, 1, 0, 0); !ok || bit != 1 {
		t.Fatalf("all-black reference should produce typical black, bit=%d ok=%v", bit, ok)
	}
}

func TestDecodeGenericRefinementRegionTPGRTypicalBlack(t *testing.T) {
	ref := stdimage.NewGray(stdimage.Rect(0, 0, 5, 3))
	fillWhite(ref)
	for y := 0; y < 3; y++ {
		for x := 0; x < 3; x++ {
			setBlack(ref, x, y)
		}
	}

	img, err := decodeGenericRefinementRegion(genericRefinementRegion{
		info: regionInfo{
			width:  1,
			height: 1,
		},
		ref:      ref,
		template: 1,
		tpgrOn:   true,
		payload:  bytes.Repeat([]byte{0xff}, 16),
	})
	if err != nil {
		t.Fatalf("decodeGenericRefinementRegion: %v", err)
	}
	if got := img.GrayAt(0, 0).Y; got != 0x00 {
		t.Fatalf("typical black refinement pixel = %02x", got)
	}
}

func fillWhite(img *stdimage.Gray) {
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
}

func setBlack(img *stdimage.Gray, x, y int) {
	img.Pix[y*img.Stride+x] = 0x00
}
