package jbig2

import (
	"image"
	"strings"
	"testing"
)

func TestParseTextRegionSegmentParsesHeader(t *testing.T) {
	body := append(testRegionInfo(4, 3, 2, 1, jbig2CombineReplace),
		0x82, 0x9a, // refine, logStrips=2, refCorner=1, combOp=1, default pixel, template 1.
		0x00, 0x00, 0x00, 0x05, // instances.
		0xaa,
	)

	region, err := parseTextRegionSegment(body)
	if err != nil {
		t.Fatalf("parseTextRegionSegment returned error: %v", err)
	}
	if !region.refine || region.huff || !region.defaultPix || region.template != 1 {
		t.Fatalf("unexpected text flags: %+v", region)
	}
	if region.logStrips != 2 || region.refCorner != 1 || region.combOp != 1 {
		t.Fatalf("unexpected text placement flags: %+v", region)
	}
	if region.numInstances != 5 || len(region.payload) != 1 {
		t.Fatalf("unexpected instance count or payload: instances=%d payload=%d", region.numInstances, len(region.payload))
	}
}

func TestParseTextRegionSegmentParsesHuffmanFlags(t *testing.T) {
	body := append(testRegionInfo(4, 3, 2, 1, jbig2CombineReplace),
		0x00, 0x01, // Huffman text region.
		0x79, 0x39, // FS=1, DS=2, DT=3, RDW=0, RDH=1, RDX=2, RDY=3.
		0x00, 0x00, 0x00, 0x00,
	)

	region, err := parseTextRegionSegment(body)
	if err != nil {
		t.Fatalf("parseTextRegionSegment returned error: %v", err)
	}
	if !region.huff {
		t.Fatalf("expected Huffman text region")
	}
	if region.huffFS != 1 || region.huffDS != 2 || region.huffDT != 3 ||
		region.huffRDW != 0 || region.huffRDH != 1 || region.huffRDX != 2 || region.huffRDY != 3 {
		t.Fatalf("unexpected Huffman flags: %+v", region)
	}
}

func TestDecodeTextRegionSegmentAcceptsEmptyImmediateRegion(t *testing.T) {
	body := append(testRegionInfo(4, 3, 1, 1, jbig2CombineReplace),
		0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	)

	regionInfo, img, err := decodeTextRegionSegment(segmentHeader{
		number:                   3,
		typ:                      SegmentImmediateText,
		referredToSegmentNumbers: []uint32{2},
		data:                     body,
	}, map[uint32]symbolDictionary{
		2: {symbols: nil},
	})
	if err != nil {
		t.Fatalf("decodeTextRegionSegment returned error: %v", err)
	}
	if regionInfo.x != 1 || regionInfo.y != 1 || img.Bounds().Dx() != 4 || img.Bounds().Dy() != 3 {
		t.Fatalf("unexpected decoded text region: info=%+v bounds=%v", regionInfo, img.Bounds())
	}
}

func TestDecodeHuffmanTextRegionPlacesSymbol(t *testing.T) {
	symbol := makeWhiteGray(1, 1)
	symbol.Pix[0] = 0x00
	region := textRegion{
		info: regionInfo{
			width:  1,
			height: 1,
		},
		huff:         true,
		refCorner:    1,
		combOp:       jbig2CombineOr,
		numInstances: 1,
		payload: testBits(
			textRegionOneSymbolCodeTablePrefixBits() +
				"0" + "000" + // One run-length code, then reset to the next byte.
				"0" + // Initial T = 1.
				"0" + // Delta T = 1, placing the strip at T = 0.
				"00" + "0000000" + // First S = 0 via table F.
				"0" + // Symbol ID 0.
				"01", // Delta S OOB via table H.
		),
	}

	img, err := decodeHuffmanTextRegion(region, []*image.Gray{symbol})
	if err != nil {
		t.Fatalf("decodeHuffmanTextRegion returned error: %v", err)
	}
	if got := img.GrayAt(0, 0).Y; got != 0x00 {
		t.Fatalf("decoded Huffman text pixel = %02x, want black", got)
	}
}

func TestDecodeHuffmanTextRegionRefineFlagZeroUsesBaseSymbol(t *testing.T) {
	symbol := makeWhiteGray(1, 1)
	symbol.Pix[0] = 0x00
	region := textRegion{
		info: regionInfo{
			width:  1,
			height: 1,
		},
		huff:         true,
		refine:       true,
		refCorner:    1,
		combOp:       jbig2CombineOr,
		numInstances: 1,
		payload: testBits(
			textRegionOneSymbolCodeTablePrefixBits() +
				"0" + "000" + // One run-length code, then reset to the next byte.
				"0" + // Initial T = 1.
				"0" + // Delta T = 1, placing the strip at T = 0.
				"00" + "0000000" + // First S = 0 via table F.
				"0" + // Symbol ID 0.
				"0" + // RI = 0, so the base symbol is used.
				"01", // Delta S OOB via table H.
		),
	}

	img, err := decodeHuffmanTextRegion(region, []*image.Gray{symbol})
	if err != nil {
		t.Fatalf("decodeHuffmanTextRegion returned error: %v", err)
	}
	if got := img.GrayAt(0, 0).Y; got != 0x00 {
		t.Fatalf("decoded refined Huffman text pixel = %02x, want black", got)
	}
}

func TestComposeTextSymbolReturnsPopplerAdvance(t *testing.T) {
	dst := makeWhiteGray(10, 10)
	symbol := makeWhiteGray(3, 4)

	if got := composeTextSymbol(dst, symbol, textRegion{}, 2, 5); got != 2 {
		t.Fatalf("non-transposed advance = %d, want 2", got)
	}
	if got := composeTextSymbol(dst, symbol, textRegion{transposed: true}, 2, 5); got != 3 {
		t.Fatalf("transposed advance = %d, want 3", got)
	}
}

func textRegionOneSymbolCodeTablePrefixBits() string {
	var b strings.Builder
	for i := 0; i < 35; i++ {
		if i == 1 {
			b.WriteString("0001")
		} else {
			b.WriteString("0000")
		}
	}
	return b.String()
}

func makeWhiteGray(width, height int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, width, height))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	return img
}
