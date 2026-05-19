package jbig2

import "testing"

func TestHuffmanDecoderDecodeIntUsesPopplerTablePrefixes(t *testing.T) {
	tests := []struct {
		name string
		bits string
		want int
	}{
		{name: "table N zero", bits: "0", want: 0},
		{name: "table N minus two", bits: "100", want: -2},
		{name: "table N plus two", bits: "111", want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := newHuffmanDecoder(testBits(tt.bits))
			got, ok, err := decoder.decodeInt(huffmanTableN)
			if err != nil {
				t.Fatalf("decodeInt returned error: %v", err)
			}
			if !ok || got != tt.want {
				t.Fatalf("decodeInt = (%d, %t), want (%d, true)", got, ok, tt.want)
			}
		})
	}
}

func TestBuildHuffmanTableAssignsCanonicalPrefixes(t *testing.T) {
	table := []huffmanTableEntry{
		{val: 10, prefixLen: 2},
		{val: 11, prefixLen: 1},
		{val: 12, prefixLen: 3},
		{rangeLen: jbig2HuffmanEOT},
	}
	built, err := buildHuffmanTable(table, 3)
	if err != nil {
		t.Fatalf("buildHuffmanTable returned error: %v", err)
	}
	if built[0].val != 11 || built[0].prefix != 0 {
		t.Fatalf("unexpected first canonical entry: %+v", built[0])
	}
	if built[1].val != 10 || built[1].prefix != 2 {
		t.Fatalf("unexpected second canonical entry: %+v", built[1])
	}
	if built[2].val != 12 || built[2].prefix != 6 {
		t.Fatalf("unexpected third canonical entry: %+v", built[2])
	}
}

func TestHuffmanDecoderSymbolDictionaryDefaultSequence(t *testing.T) {
	decoder := newHuffmanDecoder(testBits(
		"0" + // Height delta = 1 via table D.
			"10" + // Width delta = 1 via table B.
			"111111" + // Width OOB via table B.
			"0" + "0000", // Bitmap size = 0 via table A.
	))

	if got, ok, err := decoder.decodeInt(huffmanTableD); err != nil || !ok || got != 1 {
		t.Fatalf("height delta = %d, %v, %v; want 1, true, nil", got, ok, err)
	}
	if got, ok, err := decoder.decodeInt(huffmanTableB); err != nil || !ok || got != 1 {
		t.Fatalf("width delta = %d, %v, %v; want 1, true, nil", got, ok, err)
	}
	if _, ok, err := decoder.decodeInt(huffmanTableB); err != nil || ok {
		t.Fatalf("width OOB ok=%v err=%v; want false, nil", ok, err)
	}
	if got, ok, err := decoder.decodeInt(huffmanTableA); err != nil || !ok || got != 0 {
		t.Fatalf("bitmap size = %d, %v, %v; want 0, true, nil", got, ok, err)
	}
}
