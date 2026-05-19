package jbig2

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/dh-kam/pdf-go/internal/domain/errors"
)

func decodeCodeTableSegment(segment segmentHeader) ([]huffmanTableEntry, error) {
	table, err := parseCodeTableSegment(segment.data)
	if err != nil {
		return nil, errors.Invalid("jbig2_code_table", err)
	}
	return table, nil
}

func parseCodeTableSegment(data []byte) ([]huffmanTableEntry, error) {
	if len(data) < 9 {
		return nil, fmt.Errorf("truncated code table segment")
	}

	flags := data[0]
	lowVal := int(int32(binary.BigEndian.Uint32(data[1:5])))
	highVal := int(int32(binary.BigEndian.Uint32(data[5:9])))
	if highVal < lowVal {
		return nil, fmt.Errorf("invalid code table range: %d..%d", lowVal, highVal)
	}

	oob := flags&0x01 != 0
	prefixBits := uint(((flags >> 1) & 0x07) + 1)
	rangeBits := uint(((flags >> 4) & 0x07) + 1)
	decoder := newHuffmanDecoder(data[9:])

	entries := make([]huffmanTableEntry, 0, 8)
	for val := lowVal; val < highVal; {
		prefixLen, err := decoder.readBits(prefixBits)
		if err != nil {
			return nil, err
		}
		rangeLen, err := decoder.readBits(rangeBits)
		if err != nil {
			return nil, err
		}
		if rangeLen >= 31 && val > math.MaxInt-int(uint(1)<<rangeLen) {
			return nil, fmt.Errorf("code table range overflow")
		}
		entries = append(entries, huffmanTableEntry{
			val:       val,
			prefixLen: uint(prefixLen),
			rangeLen:  rangeLen,
		})
		val += 1 << rangeLen
	}

	prefixLen, err := decoder.readBits(prefixBits)
	if err != nil {
		return nil, err
	}
	entries = append(entries, huffmanTableEntry{
		val:       lowVal - 1,
		prefixLen: uint(prefixLen),
		rangeLen:  jbig2HuffmanLOW,
	})

	prefixLen, err = decoder.readBits(prefixBits)
	if err != nil {
		return nil, err
	}
	entries = append(entries, huffmanTableEntry{
		val:       highVal,
		prefixLen: uint(prefixLen),
		rangeLen:  32,
	})

	if oob {
		prefixLen, err = decoder.readBits(prefixBits)
		if err != nil {
			return nil, err
		}
		entries = append(entries, huffmanTableEntry{
			prefixLen: uint(prefixLen),
			rangeLen:  jbig2HuffmanOOB,
		})
	}
	entries = append(entries, huffmanTableEntry{rangeLen: jbig2HuffmanEOT})
	return buildHuffmanTable(entries, len(entries)-1)
}

func collectReferencedCodeTables(segment segmentHeader, codeTables map[uint32][]huffmanTableEntry) [][]huffmanTableEntry {
	if len(codeTables) == 0 {
		return nil
	}
	var tables [][]huffmanTableEntry
	for _, ref := range segment.referredToSegmentNumbers {
		if table, ok := codeTables[ref]; ok {
			tables = append(tables, table)
		}
	}
	return tables
}
