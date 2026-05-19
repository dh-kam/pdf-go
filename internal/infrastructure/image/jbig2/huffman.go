package jbig2

import (
	"fmt"
	"io"
	"sort"
)

const (
	jbig2HuffmanLOW uint32 = 0xfffffffd
	jbig2HuffmanOOB uint32 = 0xfffffffe
	jbig2HuffmanEOT uint32 = 0xffffffff
)

type huffmanTableEntry struct {
	val       int
	prefixLen uint
	rangeLen  uint32
	prefix    uint32
}

type huffmanDecoder struct {
	buf    uint32
	bufLen uint
	data   []byte
	offset int
}

func newHuffmanDecoder(data []byte) *huffmanDecoder {
	return &huffmanDecoder{data: data}
}

func (d *huffmanDecoder) reset() {
	d.buf = 0
	d.bufLen = 0
}

func (d *huffmanDecoder) remainingBytes() []byte {
	if d.offset >= len(d.data) {
		return nil
	}
	return d.data[d.offset:]
}

func (d *huffmanDecoder) advanceBytes(count int) error {
	if count < 0 || d.offset+count > len(d.data) {
		return fmt.Errorf("Huffman decoder byte advance out of range: %d", count)
	}
	d.offset += count
	d.buf = 0
	d.bufLen = 0
	return nil
}

func (d *huffmanDecoder) decodeInt(table []huffmanTableEntry) (int, bool, error) {
	var prefix uint32
	var length uint
	for _, entry := range table {
		if entry.rangeLen == jbig2HuffmanEOT {
			break
		}
		for length < entry.prefixLen {
			bit, err := d.readBit()
			if err != nil {
				return 0, false, err
			}
			prefix = (prefix << 1) | uint32(bit)
			length++
		}
		if prefix != entry.prefix {
			continue
		}
		switch entry.rangeLen {
		case jbig2HuffmanOOB:
			return 0, false, nil
		case jbig2HuffmanLOW:
			bits, err := d.readBits(32)
			if err != nil {
				return 0, false, err
			}
			return entry.val - int(bits), true, nil
		case 0:
			return entry.val, true, nil
		default:
			bits, err := d.readBits(uint(entry.rangeLen))
			if err != nil {
				return 0, false, err
			}
			return entry.val + int(bits), true, nil
		}
	}
	return 0, false, fmt.Errorf("no matching JBIG2 Huffman code")
}

func (d *huffmanDecoder) readBit() (uint8, error) {
	if d.bufLen == 0 {
		if d.offset >= len(d.data) {
			return 0, io.EOF
		}
		d.buf = uint32(d.data[d.offset])
		d.offset++
		d.bufLen = 8
	}
	d.bufLen--
	return uint8((d.buf >> d.bufLen) & 1), nil
}

func (d *huffmanDecoder) readBits(count uint) (uint32, error) {
	if count == 0 {
		return 0, nil
	}
	var value uint32
	for i := uint(0); i < count; i++ {
		bit, err := d.readBit()
		if err != nil {
			return 0, err
		}
		value = (value << 1) | uint32(bit)
	}
	return value, nil
}

func buildHuffmanTable(entries []huffmanTableEntry, activeLen int) ([]huffmanTableEntry, error) {
	if activeLen < 0 || activeLen >= len(entries) {
		return nil, fmt.Errorf("invalid Huffman table length %d", activeLen)
	}
	active := append([]huffmanTableEntry(nil), entries[:activeLen]...)
	sort.SliceStable(active, func(i, j int) bool {
		if active[i].prefixLen == 0 {
			return false
		}
		if active[j].prefixLen == 0 {
			return true
		}
		return active[i].prefixLen < active[j].prefixLen
	})

	filtered := active[:0]
	for _, entry := range active {
		if entry.prefixLen != 0 {
			filtered = append(filtered, entry)
		}
	}
	filtered = append(filtered, entries[activeLen])
	if len(filtered) == 0 || filtered[0].rangeLen == jbig2HuffmanEOT {
		return filtered, nil
	}

	prefix := uint32(0)
	filtered[0].prefix = prefix
	prefix++
	for i := 1; i < len(filtered); i++ {
		if filtered[i].rangeLen == jbig2HuffmanEOT {
			break
		}
		delta := filtered[i].prefixLen - filtered[i-1].prefixLen
		if delta > 32 {
			return nil, fmt.Errorf("invalid JBIG2 Huffman prefix length delta %d", delta)
		}
		prefix <<= delta
		filtered[i].prefix = prefix
		prefix++
	}
	return filtered, nil
}

func cloneHuffmanTable(entries []huffmanTableEntry) []huffmanTableEntry {
	return append([]huffmanTableEntry(nil), entries...)
}

var huffmanTableA = []huffmanTableEntry{
	{val: 0, prefixLen: 1, rangeLen: 4, prefix: 0x000},
	{val: 16, prefixLen: 2, rangeLen: 8, prefix: 0x002},
	{val: 272, prefixLen: 3, rangeLen: 16, prefix: 0x006},
	{val: 65808, prefixLen: 3, rangeLen: 32, prefix: 0x007},
	{rangeLen: jbig2HuffmanEOT},
}

var huffmanTableB = []huffmanTableEntry{
	{val: 0, prefixLen: 1, rangeLen: 0, prefix: 0x000},
	{val: 1, prefixLen: 2, rangeLen: 0, prefix: 0x002},
	{val: 2, prefixLen: 3, rangeLen: 0, prefix: 0x006},
	{val: 3, prefixLen: 4, rangeLen: 3, prefix: 0x00e},
	{val: 11, prefixLen: 5, rangeLen: 6, prefix: 0x01e},
	{val: 75, prefixLen: 6, rangeLen: 32, prefix: 0x03e},
	{val: 0, prefixLen: 6, rangeLen: jbig2HuffmanOOB, prefix: 0x03f},
	{rangeLen: jbig2HuffmanEOT},
}

var huffmanTableC = []huffmanTableEntry{
	{val: 0, prefixLen: 1, rangeLen: 0, prefix: 0x000},
	{val: 1, prefixLen: 2, rangeLen: 0, prefix: 0x002},
	{val: 2, prefixLen: 3, rangeLen: 0, prefix: 0x006},
	{val: 3, prefixLen: 4, rangeLen: 3, prefix: 0x00e},
	{val: 11, prefixLen: 5, rangeLen: 6, prefix: 0x01e},
	{val: 0, prefixLen: 6, rangeLen: jbig2HuffmanOOB, prefix: 0x03e},
	{val: 75, prefixLen: 7, rangeLen: 32, prefix: 0x0fe},
	{val: -256, prefixLen: 8, rangeLen: 8, prefix: 0x0fe},
	{val: -257, prefixLen: 8, rangeLen: jbig2HuffmanLOW, prefix: 0x0ff},
	{rangeLen: jbig2HuffmanEOT},
}

var huffmanTableD = []huffmanTableEntry{
	{val: 1, prefixLen: 1, rangeLen: 0, prefix: 0x000},
	{val: 2, prefixLen: 2, rangeLen: 0, prefix: 0x002},
	{val: 3, prefixLen: 3, rangeLen: 0, prefix: 0x006},
	{val: 4, prefixLen: 4, rangeLen: 3, prefix: 0x00e},
	{val: 12, prefixLen: 5, rangeLen: 6, prefix: 0x01e},
	{val: 76, prefixLen: 5, rangeLen: 32, prefix: 0x01f},
	{rangeLen: jbig2HuffmanEOT},
}

var huffmanTableE = []huffmanTableEntry{
	{val: 1, prefixLen: 1, rangeLen: 0, prefix: 0x000},
	{val: 2, prefixLen: 2, rangeLen: 0, prefix: 0x002},
	{val: 3, prefixLen: 3, rangeLen: 0, prefix: 0x006},
	{val: 4, prefixLen: 4, rangeLen: 3, prefix: 0x00e},
	{val: 12, prefixLen: 5, rangeLen: 6, prefix: 0x01e},
	{val: 76, prefixLen: 6, rangeLen: 32, prefix: 0x03e},
	{val: -255, prefixLen: 7, rangeLen: 8, prefix: 0x07e},
	{val: -256, prefixLen: 7, rangeLen: jbig2HuffmanLOW, prefix: 0x07f},
	{rangeLen: jbig2HuffmanEOT},
}

var huffmanTableF = []huffmanTableEntry{
	{val: 0, prefixLen: 2, rangeLen: 7, prefix: 0x000},
	{val: 128, prefixLen: 3, rangeLen: 7, prefix: 0x002},
	{val: 256, prefixLen: 3, rangeLen: 8, prefix: 0x003},
	{val: -1024, prefixLen: 4, rangeLen: 9, prefix: 0x008},
	{val: -512, prefixLen: 4, rangeLen: 8, prefix: 0x009},
	{val: -256, prefixLen: 4, rangeLen: 7, prefix: 0x00a},
	{val: -32, prefixLen: 4, rangeLen: 5, prefix: 0x00b},
	{val: 512, prefixLen: 4, rangeLen: 9, prefix: 0x00c},
	{val: 1024, prefixLen: 4, rangeLen: 10, prefix: 0x00d},
	{val: -2048, prefixLen: 5, rangeLen: 10, prefix: 0x01c},
	{val: -128, prefixLen: 5, rangeLen: 6, prefix: 0x01d},
	{val: -64, prefixLen: 5, rangeLen: 5, prefix: 0x01e},
	{val: -2049, prefixLen: 6, rangeLen: jbig2HuffmanLOW, prefix: 0x03e},
	{val: 2048, prefixLen: 6, rangeLen: 32, prefix: 0x03f},
	{rangeLen: jbig2HuffmanEOT},
}

var huffmanTableG = []huffmanTableEntry{
	{val: -512, prefixLen: 3, rangeLen: 8, prefix: 0x000},
	{val: 256, prefixLen: 3, rangeLen: 8, prefix: 0x001},
	{val: 512, prefixLen: 3, rangeLen: 9, prefix: 0x002},
	{val: 1024, prefixLen: 3, rangeLen: 10, prefix: 0x003},
	{val: -1024, prefixLen: 4, rangeLen: 9, prefix: 0x008},
	{val: -256, prefixLen: 4, rangeLen: 7, prefix: 0x009},
	{val: -32, prefixLen: 4, rangeLen: 5, prefix: 0x00a},
	{val: 0, prefixLen: 4, rangeLen: 5, prefix: 0x00b},
	{val: 128, prefixLen: 4, rangeLen: 7, prefix: 0x00c},
	{val: -128, prefixLen: 5, rangeLen: 6, prefix: 0x01a},
	{val: -64, prefixLen: 5, rangeLen: 5, prefix: 0x01b},
	{val: 32, prefixLen: 5, rangeLen: 5, prefix: 0x01c},
	{val: 64, prefixLen: 5, rangeLen: 6, prefix: 0x01d},
	{val: -1025, prefixLen: 5, rangeLen: jbig2HuffmanLOW, prefix: 0x01e},
	{val: 2048, prefixLen: 5, rangeLen: 32, prefix: 0x01f},
	{rangeLen: jbig2HuffmanEOT},
}

var huffmanTableH = []huffmanTableEntry{
	{val: 0, prefixLen: 2, rangeLen: 1, prefix: 0x000},
	{val: 0, prefixLen: 2, rangeLen: jbig2HuffmanOOB, prefix: 0x001},
	{val: 4, prefixLen: 3, rangeLen: 4, prefix: 0x004},
	{val: -1, prefixLen: 4, rangeLen: 0, prefix: 0x00a},
	{val: 22, prefixLen: 4, rangeLen: 4, prefix: 0x00b},
	{val: 38, prefixLen: 4, rangeLen: 5, prefix: 0x00c},
	{val: 2, prefixLen: 5, rangeLen: 0, prefix: 0x01a},
	{val: 70, prefixLen: 5, rangeLen: 6, prefix: 0x01b},
	{val: 134, prefixLen: 5, rangeLen: 7, prefix: 0x01c},
	{val: 3, prefixLen: 6, rangeLen: 0, prefix: 0x03a},
	{val: 20, prefixLen: 6, rangeLen: 1, prefix: 0x03b},
	{val: 262, prefixLen: 6, rangeLen: 7, prefix: 0x03c},
	{val: 646, prefixLen: 6, rangeLen: 10, prefix: 0x03d},
	{val: -2, prefixLen: 7, rangeLen: 0, prefix: 0x07c},
	{val: 390, prefixLen: 7, rangeLen: 8, prefix: 0x07d},
	{val: -15, prefixLen: 8, rangeLen: 3, prefix: 0x0fc},
	{val: -5, prefixLen: 8, rangeLen: 1, prefix: 0x0fd},
	{val: -7, prefixLen: 9, rangeLen: 1, prefix: 0x1fc},
	{val: -3, prefixLen: 9, rangeLen: 0, prefix: 0x1fd},
	{val: -16, prefixLen: 9, rangeLen: jbig2HuffmanLOW, prefix: 0x1fe},
	{val: 1670, prefixLen: 9, rangeLen: 32, prefix: 0x1ff},
	{rangeLen: jbig2HuffmanEOT},
}

var huffmanTableI = []huffmanTableEntry{
	{val: 0, prefixLen: 2, rangeLen: jbig2HuffmanOOB, prefix: 0x000},
	{val: -1, prefixLen: 3, rangeLen: 1, prefix: 0x002},
	{val: 1, prefixLen: 3, rangeLen: 1, prefix: 0x003},
	{val: 7, prefixLen: 3, rangeLen: 5, prefix: 0x004},
	{val: -3, prefixLen: 4, rangeLen: 1, prefix: 0x00a},
	{val: 43, prefixLen: 4, rangeLen: 5, prefix: 0x00b},
	{val: 75, prefixLen: 4, rangeLen: 6, prefix: 0x00c},
	{val: 3, prefixLen: 5, rangeLen: 1, prefix: 0x01a},
	{val: 139, prefixLen: 5, rangeLen: 7, prefix: 0x01b},
	{val: 267, prefixLen: 5, rangeLen: 8, prefix: 0x01c},
	{val: 5, prefixLen: 6, rangeLen: 1, prefix: 0x03a},
	{val: 39, prefixLen: 6, rangeLen: 2, prefix: 0x03b},
	{val: 523, prefixLen: 6, rangeLen: 8, prefix: 0x03c},
	{val: 1291, prefixLen: 6, rangeLen: 11, prefix: 0x03d},
	{val: -5, prefixLen: 7, rangeLen: 1, prefix: 0x07c},
	{val: 779, prefixLen: 7, rangeLen: 9, prefix: 0x07d},
	{val: -31, prefixLen: 8, rangeLen: 4, prefix: 0x0fc},
	{val: -11, prefixLen: 8, rangeLen: 2, prefix: 0x0fd},
	{val: -15, prefixLen: 9, rangeLen: 2, prefix: 0x1fc},
	{val: -7, prefixLen: 9, rangeLen: 1, prefix: 0x1fd},
	{val: -32, prefixLen: 9, rangeLen: jbig2HuffmanLOW, prefix: 0x1fe},
	{val: 3339, prefixLen: 9, rangeLen: 32, prefix: 0x1ff},
	{rangeLen: jbig2HuffmanEOT},
}

var huffmanTableJ = []huffmanTableEntry{
	{val: -2, prefixLen: 2, rangeLen: 2, prefix: 0x000},
	{val: 6, prefixLen: 2, rangeLen: 6, prefix: 0x001},
	{val: 0, prefixLen: 2, rangeLen: jbig2HuffmanOOB, prefix: 0x002},
	{val: -3, prefixLen: 5, rangeLen: 0, prefix: 0x018},
	{val: 2, prefixLen: 5, rangeLen: 0, prefix: 0x019},
	{val: 70, prefixLen: 5, rangeLen: 5, prefix: 0x01a},
	{val: 3, prefixLen: 6, rangeLen: 0, prefix: 0x036},
	{val: 102, prefixLen: 6, rangeLen: 5, prefix: 0x037},
	{val: 134, prefixLen: 6, rangeLen: 6, prefix: 0x038},
	{val: 198, prefixLen: 6, rangeLen: 7, prefix: 0x039},
	{val: 326, prefixLen: 6, rangeLen: 8, prefix: 0x03a},
	{val: 582, prefixLen: 6, rangeLen: 9, prefix: 0x03b},
	{val: 1094, prefixLen: 6, rangeLen: 10, prefix: 0x03c},
	{val: -21, prefixLen: 7, rangeLen: 4, prefix: 0x07a},
	{val: -4, prefixLen: 7, rangeLen: 0, prefix: 0x07b},
	{val: 4, prefixLen: 7, rangeLen: 0, prefix: 0x07c},
	{val: 2118, prefixLen: 7, rangeLen: 11, prefix: 0x07d},
	{val: -5, prefixLen: 8, rangeLen: 0, prefix: 0x0fc},
	{val: 5, prefixLen: 8, rangeLen: 0, prefix: 0x0fd},
	{val: -22, prefixLen: 8, rangeLen: jbig2HuffmanLOW, prefix: 0x0fe},
	{val: 4166, prefixLen: 8, rangeLen: 32, prefix: 0x0ff},
	{rangeLen: jbig2HuffmanEOT},
}

var huffmanTableK = []huffmanTableEntry{
	{val: 1, prefixLen: 1, rangeLen: 0, prefix: 0x000},
	{val: 2, prefixLen: 2, rangeLen: 1, prefix: 0x002},
	{val: 4, prefixLen: 4, rangeLen: 0, prefix: 0x00c},
	{val: 5, prefixLen: 4, rangeLen: 1, prefix: 0x00d},
	{val: 7, prefixLen: 5, rangeLen: 1, prefix: 0x01c},
	{val: 9, prefixLen: 5, rangeLen: 2, prefix: 0x01d},
	{val: 13, prefixLen: 6, rangeLen: 2, prefix: 0x03c},
	{val: 17, prefixLen: 7, rangeLen: 2, prefix: 0x07a},
	{val: 21, prefixLen: 7, rangeLen: 3, prefix: 0x07b},
	{val: 29, prefixLen: 7, rangeLen: 4, prefix: 0x07c},
	{val: 45, prefixLen: 7, rangeLen: 5, prefix: 0x07d},
	{val: 77, prefixLen: 7, rangeLen: 6, prefix: 0x07e},
	{val: 141, prefixLen: 7, rangeLen: 32, prefix: 0x07f},
	{rangeLen: jbig2HuffmanEOT},
}

var huffmanTableL = []huffmanTableEntry{
	{val: 1, prefixLen: 1, rangeLen: 0, prefix: 0x000},
	{val: 2, prefixLen: 2, rangeLen: 0, prefix: 0x002},
	{val: 3, prefixLen: 3, rangeLen: 1, prefix: 0x006},
	{val: 5, prefixLen: 5, rangeLen: 0, prefix: 0x01c},
	{val: 6, prefixLen: 5, rangeLen: 1, prefix: 0x01d},
	{val: 8, prefixLen: 6, rangeLen: 1, prefix: 0x03c},
	{val: 10, prefixLen: 7, rangeLen: 0, prefix: 0x07a},
	{val: 11, prefixLen: 7, rangeLen: 1, prefix: 0x07b},
	{val: 13, prefixLen: 7, rangeLen: 2, prefix: 0x07c},
	{val: 17, prefixLen: 7, rangeLen: 3, prefix: 0x07d},
	{val: 25, prefixLen: 7, rangeLen: 4, prefix: 0x07e},
	{val: 41, prefixLen: 8, rangeLen: 5, prefix: 0x0fe},
	{val: 73, prefixLen: 8, rangeLen: 32, prefix: 0x0ff},
	{rangeLen: jbig2HuffmanEOT},
}

var huffmanTableM = []huffmanTableEntry{
	{val: 1, prefixLen: 1, rangeLen: 0, prefix: 0x000},
	{val: 2, prefixLen: 3, rangeLen: 0, prefix: 0x004},
	{val: 7, prefixLen: 3, rangeLen: 3, prefix: 0x005},
	{val: 3, prefixLen: 4, rangeLen: 0, prefix: 0x00c},
	{val: 5, prefixLen: 4, rangeLen: 1, prefix: 0x00d},
	{val: 4, prefixLen: 5, rangeLen: 0, prefix: 0x01c},
	{val: 15, prefixLen: 6, rangeLen: 1, prefix: 0x03a},
	{val: 17, prefixLen: 6, rangeLen: 2, prefix: 0x03b},
	{val: 21, prefixLen: 6, rangeLen: 3, prefix: 0x03c},
	{val: 29, prefixLen: 6, rangeLen: 4, prefix: 0x03d},
	{val: 45, prefixLen: 6, rangeLen: 5, prefix: 0x03e},
	{val: 77, prefixLen: 7, rangeLen: 6, prefix: 0x07e},
	{val: 141, prefixLen: 7, rangeLen: 32, prefix: 0x07f},
	{rangeLen: jbig2HuffmanEOT},
}

var huffmanTableN = []huffmanTableEntry{
	{val: 0, prefixLen: 1, rangeLen: 0, prefix: 0x000},
	{val: -2, prefixLen: 3, rangeLen: 0, prefix: 0x004},
	{val: -1, prefixLen: 3, rangeLen: 0, prefix: 0x005},
	{val: 1, prefixLen: 3, rangeLen: 0, prefix: 0x006},
	{val: 2, prefixLen: 3, rangeLen: 0, prefix: 0x007},
	{rangeLen: jbig2HuffmanEOT},
}

var huffmanTableO = []huffmanTableEntry{
	{val: 0, prefixLen: 1, rangeLen: 0, prefix: 0x000},
	{val: -1, prefixLen: 3, rangeLen: 0, prefix: 0x004},
	{val: 1, prefixLen: 3, rangeLen: 0, prefix: 0x005},
	{val: -2, prefixLen: 4, rangeLen: 0, prefix: 0x00c},
	{val: 2, prefixLen: 4, rangeLen: 0, prefix: 0x00d},
	{val: -4, prefixLen: 5, rangeLen: 1, prefix: 0x01c},
	{val: 3, prefixLen: 5, rangeLen: 1, prefix: 0x01d},
	{val: -8, prefixLen: 6, rangeLen: 2, prefix: 0x03c},
	{val: 5, prefixLen: 6, rangeLen: 2, prefix: 0x03d},
	{val: -24, prefixLen: 7, rangeLen: 4, prefix: 0x07c},
	{val: 9, prefixLen: 7, rangeLen: 4, prefix: 0x07d},
	{val: -25, prefixLen: 7, rangeLen: jbig2HuffmanLOW, prefix: 0x07e},
	{val: 25, prefixLen: 7, rangeLen: 32, prefix: 0x07f},
	{rangeLen: jbig2HuffmanEOT},
}
