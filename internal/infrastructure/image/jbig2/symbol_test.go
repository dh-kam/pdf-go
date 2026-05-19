package jbig2

import (
	"strings"
	"testing"
)

func TestParseSymbolDictionarySegmentParsesArithmeticHeader(t *testing.T) {
	data := []byte{
		0x08, 0x00, // SD template 2, arithmetic coding.
		0xfd, 0xff, // one AT pixel.
		0x00, 0x00, 0x00, 0x01, // exported symbols.
		0x00, 0x00, 0x00, 0x02, // new symbols.
		0xaa, 0xbb,
	}

	segment, err := parseSymbolDictionarySegment(data)
	if err != nil {
		t.Fatalf("parseSymbolDictionarySegment returned error: %v", err)
	}
	if segment.huff || segment.refAgg {
		t.Fatalf("unexpected symbol dictionary flags: %+v", segment)
	}
	if segment.sdTemplate != 2 || len(segment.atPixels) != 1 {
		t.Fatalf("unexpected template or AT pixels: %+v", segment)
	}
	if segment.atPixels[0] != (adaptiveTemplatePixel{x: -3, y: -1}) {
		t.Fatalf("unexpected AT pixel: %+v", segment.atPixels[0])
	}
	if segment.numExSyms != 1 || segment.numNewSyms != 2 {
		t.Fatalf("unexpected symbol counts: exported=%d new=%d", segment.numExSyms, segment.numNewSyms)
	}
	if len(segment.payload) != 2 {
		t.Fatalf("unexpected payload length: %d", len(segment.payload))
	}
}

func TestParseSymbolDictionarySegmentParsesRefinementATPixels(t *testing.T) {
	data := []byte{
		0x04, 0x02, // SD template 1 with refinement/aggregate coding.
		0xfd, 0xff, // one symbol AT pixel.
		0xfe, 0xff, 0xff, 0xfe, // two refinement AT pixels.
		0x00, 0x00, 0x00, 0x00, // exported symbols.
		0x00, 0x00, 0x00, 0x00, // new symbols.
	}

	segment, err := parseSymbolDictionarySegment(data)
	if err != nil {
		t.Fatalf("parseSymbolDictionarySegment returned error: %v", err)
	}
	if !segment.refAgg || segment.sdrTemplate != 0 {
		t.Fatalf("unexpected refinement flags: %+v", segment)
	}
	if len(segment.atPixels) != 1 || segment.atPixels[0] != (adaptiveTemplatePixel{x: -3, y: -1}) {
		t.Fatalf("unexpected symbol AT pixels: %+v", segment.atPixels)
	}
	if len(segment.refinementATPixels) != 2 {
		t.Fatalf("unexpected refinement AT pixel count: %d", len(segment.refinementATPixels))
	}
	if segment.refinementATPixels[0] != (adaptiveTemplatePixel{x: -2, y: -1}) ||
		segment.refinementATPixels[1] != (adaptiveTemplatePixel{x: -1, y: -2}) {
		t.Fatalf("unexpected refinement AT pixels: %+v", segment.refinementATPixels)
	}
}

func TestParseSymbolDictionarySegmentParsesHuffmanFlags(t *testing.T) {
	data := []byte{
		0x00, 0x75, // Huffman, DH=1, DW=3, bitmap-size=1, aggregate-instance=0.
		0x00, 0x00, 0x00, 0x00, // exported symbols.
		0x00, 0x00, 0x00, 0x00, // new symbols.
	}

	segment, err := parseSymbolDictionarySegment(data)
	if err != nil {
		t.Fatalf("parseSymbolDictionarySegment returned error: %v", err)
	}
	if !segment.huff || segment.huffDH != 1 || segment.huffDW != 3 ||
		segment.huffBMSize != 1 || segment.huffAggInst != 0 {
		t.Fatalf("unexpected Huffman flags: %+v", segment)
	}
}

func TestDecodeSymbolDictionarySegmentAcceptsEmptyDictionary(t *testing.T) {
	dict, err := decodeSymbolDictionarySegment(segmentHeader{
		number: 1,
		typ:    SegmentSymbolDictionary,
		data:   testEmptySymbolDictionaryBody(),
	}, nil)
	if err != nil {
		t.Fatalf("decodeSymbolDictionarySegment returned error: %v", err)
	}
	if len(dict.symbols) != 0 {
		t.Fatalf("empty dictionary decoded %d symbols", len(dict.symbols))
	}
}

func TestDecodeHuffmanSymbolDictionaryRawCollectiveBitmap(t *testing.T) {
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

	data := []byte{
		0x00, 0x01, // Huffman symbol dictionary with default tables.
		0x00, 0x00, 0x00, 0x01, // exported symbols.
		0x00, 0x00, 0x00, 0x01, // new symbols.
	}
	data = append(data, payload...)

	dict, err := decodeSymbolDictionarySegment(segmentHeader{
		number: 1,
		typ:    SegmentSymbolDictionary,
		data:   data,
	}, nil)
	if err != nil {
		t.Fatalf("decodeSymbolDictionarySegment returned error: %v", err)
	}
	if len(dict.symbols) != 1 {
		t.Fatalf("decoded %d exported symbols, want 1", len(dict.symbols))
	}
	if got := dict.symbols[0].GrayAt(0, 0).Y; got != 0x00 {
		t.Fatalf("decoded Huffman symbol pixel = %02x, want black", got)
	}
}

func TestDecodeSymbolDictionarySegmentRefAggReachesArithmeticDecoder(t *testing.T) {
	data := []byte{
		0x04, 0x02, // SD template 1 with refinement/aggregate coding.
		0xfd, 0xff, // one symbol AT pixel.
		0xfe, 0xff, 0xff, 0xfe, // two refinement AT pixels.
		0x00, 0x00, 0x00, 0x01, // exported symbols.
		0x00, 0x00, 0x00, 0x01, // new symbols.
	}

	_, err := decodeSymbolDictionarySegment(segmentHeader{
		number: 1,
		typ:    SegmentSymbolDictionary,
		data:   data,
	}, nil)
	if err == nil {
		t.Fatalf("decodeSymbolDictionarySegment should fail on truncated arithmetic data")
	}
	if strings.Contains(strings.ToLower(err.Error()), "not implemented") {
		t.Fatalf("refinement aggregate path should not return NotImplemented: %v", err)
	}
}

func TestSymbolCodeLengthMatchesJBIG2CeilLog2Rule(t *testing.T) {
	tests := []struct {
		count int
		huff  bool
		want  uint
	}{
		{count: 0, want: 0},
		{count: 1, want: 0},
		{count: 1, huff: true, want: 1},
		{count: 2, want: 1},
		{count: 3, want: 2},
		{count: 4, want: 2},
		{count: 5, want: 3},
	}
	for _, tt := range tests {
		if got := symbolCodeLength(tt.count, tt.huff); got != tt.want {
			t.Fatalf("symbolCodeLength(%d, %t) = %d, want %d", tt.count, tt.huff, got, tt.want)
		}
	}
}
