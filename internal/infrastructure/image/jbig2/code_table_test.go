package jbig2

import "testing"

func TestParseCodeTableSegmentBuildsCustomHuffmanTable(t *testing.T) {
	data := []byte{
		0x00,                   // No OOB, 1 prefix bit, 1 range bit.
		0x00, 0x00, 0x00, 0x00, // Low value 0.
		0x00, 0x00, 0x00, 0x01, // High value 1.
		0x80, // One active prefix length, zero range length, zero low/high prefixes.
	}

	table, err := parseCodeTableSegment(data)
	if err != nil {
		t.Fatalf("parseCodeTableSegment returned error: %v", err)
	}
	got, ok, err := newHuffmanDecoder(testBits("0")).decodeInt(table)
	if err != nil {
		t.Fatalf("decodeInt returned error: %v", err)
	}
	if !ok || got != 0 {
		t.Fatalf("custom table decode = (%d, %t), want (0, true)", got, ok)
	}
}

func TestCollectReferencedCodeTablesPreservesReferenceOrder(t *testing.T) {
	first := []huffmanTableEntry{{val: 1}, {rangeLen: jbig2HuffmanEOT}}
	second := []huffmanTableEntry{{val: 2}, {rangeLen: jbig2HuffmanEOT}}

	tables := collectReferencedCodeTables(segmentHeader{
		referredToSegmentNumbers: []uint32{20, 10},
	}, map[uint32][]huffmanTableEntry{
		10: second,
		20: first,
	})
	if len(tables) != 2 || tables[0][0].val != 1 || tables[1][0].val != 2 {
		t.Fatalf("unexpected code table order: %+v", tables)
	}
}
