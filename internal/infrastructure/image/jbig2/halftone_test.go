package jbig2

import (
	"encoding/binary"
	stdimage "image"
	"testing"
)

func TestHalftoneBitsPerPixel(t *testing.T) {
	tests := []struct {
		patterns int
		want     int
	}{
		{patterns: 0, want: 0},
		{patterns: 1, want: 0},
		{patterns: 2, want: 1},
		{patterns: 3, want: 2},
		{patterns: 4, want: 2},
		{patterns: 5, want: 3},
	}
	for _, tt := range tests {
		if got := halftoneBitsPerPixel(tt.patterns); got != tt.want {
			t.Fatalf("halftoneBitsPerPixel(%d) = %d, want %d", tt.patterns, got, tt.want)
		}
	}
}

func TestDecodeHalftoneRegionSinglePattern(t *testing.T) {
	pattern := stdimage.NewGray(stdimage.Rect(0, 0, 2, 2))
	for i := range pattern.Pix {
		pattern.Pix[i] = 0xff
	}
	pattern.Pix[0] = 0x00
	pattern.Pix[3] = 0x00

	region := halftoneRegion{
		info: regionInfo{
			width:  4,
			height: 4,
		},
		gridW:  1,
		gridH:  1,
		gridX:  0,
		gridY:  0,
		stepX:  2 << 8,
		stepY:  0,
		combOp: jbig2CombineOr,
	}
	img, err := decodeHalftoneRegion(region, patternDictionary{patterns: []*stdimage.Gray{pattern}})
	if err != nil {
		t.Fatalf("decodeHalftoneRegion: %v", err)
	}
	if img.Pix[0] != 0x00 || img.Pix[1] != 0xff || img.Pix[img.Stride] != 0xff || img.Pix[img.Stride+1] != 0x00 {
		t.Fatalf("unexpected halftone pattern pixels: %v %v / %v %v", img.Pix[0], img.Pix[1], img.Pix[img.Stride], img.Pix[img.Stride+1])
	}
}

func TestDecodeHalftoneRegionSkipBitmap(t *testing.T) {
	pattern := stdimage.NewGray(stdimage.Rect(0, 0, 1, 1))
	pattern.Pix[0] = 0x00

	region := halftoneRegion{
		info: regionInfo{
			width:  2,
			height: 1,
		},
		enableSkip: true,
		gridW:      2,
		gridH:      1,
		gridX:      255,
		gridY:      255,
		stepX:      3 << 8,
		stepY:      0,
		combOp:     jbig2CombineOr,
	}
	img, err := decodeHalftoneRegion(region, patternDictionary{patterns: []*stdimage.Gray{pattern}})
	if err != nil {
		t.Fatalf("decodeHalftoneRegion: %v", err)
	}
	if got := img.GrayAt(0, 0).Y; got != 0x00 {
		t.Fatalf("first grid point should paint pattern, got %02x", got)
	}
	if got := img.GrayAt(1, 0).Y; got != 0xff {
		t.Fatalf("off-page skipped grid point should not paint, got %02x", got)
	}
}

func TestDecodeHalftoneRegionMMRReadsEveryGrayPlane(t *testing.T) {
	patterns := make([]*stdimage.Gray, 4)
	for i := range patterns {
		patterns[i] = stdimage.NewGray(stdimage.Rect(0, 0, 1, 1))
		patterns[i].Pix[0] = 0xff
	}
	patterns[1].Pix[0] = 0x00

	region := halftoneRegion{
		info: regionInfo{
			width:  4,
			height: 1,
		},
		payload: testBits("1" + "001" + "00110101" + "011"),
		mmr:     true,
		gridW:   4,
		gridH:   1,
		stepX:   1 << 8,
		combOp:  jbig2CombineOr,
	}
	img, err := decodeHalftoneRegion(region, patternDictionary{patterns: patterns})
	if err != nil {
		t.Fatalf("decodeHalftoneRegion: %v", err)
	}
	for x := 0; x < 4; x++ {
		if got := img.GrayAt(x, 0).Y; got != 0x00 {
			t.Fatalf("pixel %d should use second gray-plane pattern, got %02x", x, got)
		}
	}
}

func TestAccumulateHalftoneGrayPlaneUsesJBIG2GrayCode(t *testing.T) {
	gray := []uint32{0, 0}
	first := stdimage.NewGray(stdimage.Rect(0, 0, 2, 1))
	first.Pix[0] = 0x00
	first.Pix[1] = 0xff
	second := stdimage.NewGray(stdimage.Rect(0, 0, 2, 1))
	second.Pix[0] = 0x00
	second.Pix[1] = 0x00

	accumulateHalftoneGrayPlane(gray, first)
	accumulateHalftoneGrayPlane(gray, second)

	if gray[0] != 2 || gray[1] != 1 {
		t.Fatalf("unexpected JBIG2 halftone gray values: %v", gray)
	}
}

func TestParseHalftoneRegionSegment(t *testing.T) {
	data := make([]byte, regionInfoLength+21)
	binary.BigEndian.PutUint32(data[0:4], 10)
	binary.BigEndian.PutUint32(data[4:8], 11)
	binary.BigEndian.PutUint32(data[8:12], 2)
	binary.BigEndian.PutUint32(data[12:16], 3)
	data[16] = jbig2CombineReplace
	off := regionInfoLength
	data[off] = 0x80 | (jbig2CombineXor << 4)
	binary.BigEndian.PutUint32(data[off+1:off+5], 4)
	binary.BigEndian.PutUint32(data[off+5:off+9], 5)
	binary.BigEndian.PutUint32(data[off+9:off+13], 0xffffff00)
	binary.BigEndian.PutUint32(data[off+13:off+17], uint32(int32(512)))
	binary.BigEndian.PutUint16(data[off+17:off+19], 256)
	binary.BigEndian.PutUint16(data[off+19:off+21], 128)

	region, err := parseHalftoneRegionSegment(data)
	if err != nil {
		t.Fatalf("parseHalftoneRegionSegment: %v", err)
	}
	if region.info.width != 10 || region.info.height != 11 || region.info.x != 2 || region.info.y != 3 {
		t.Fatalf("unexpected region info: %+v", region.info)
	}
	if !region.defaultPix || region.combOp != jbig2CombineXor || region.gridW != 4 || region.gridH != 5 {
		t.Fatalf("unexpected halftone fields: %+v", region)
	}
	if region.gridX != -256 || region.gridY != 512 || region.stepX != 256 || region.stepY != 128 {
		t.Fatalf("unexpected halftone grid fields: %+v", region)
	}
}
