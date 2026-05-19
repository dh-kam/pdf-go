package jbig2

import (
	"fmt"
	"io"
)

type arithmeticQe struct {
	qe     uint32
	mpsXor uint8
	lpsXor uint8
}

var arithmeticQeTable = [...]arithmeticQe{
	{0x5601, 1 ^ 0, 1 ^ 0 ^ 0x80},
	{0x3401, 2 ^ 1, 6 ^ 1},
	{0x1801, 3 ^ 2, 9 ^ 2},
	{0x0AC1, 4 ^ 3, 12 ^ 3},
	{0x0521, 5 ^ 4, 29 ^ 4},
	{0x0221, 38 ^ 5, 33 ^ 5},
	{0x5601, 7 ^ 6, 6 ^ 6 ^ 0x80},
	{0x5401, 8 ^ 7, 14 ^ 7},
	{0x4801, 9 ^ 8, 14 ^ 8},
	{0x3801, 10 ^ 9, 14 ^ 9},
	{0x3001, 11 ^ 10, 17 ^ 10},
	{0x2401, 12 ^ 11, 18 ^ 11},
	{0x1C01, 13 ^ 12, 20 ^ 12},
	{0x1601, 29 ^ 13, 21 ^ 13},
	{0x5601, 15 ^ 14, 14 ^ 14 ^ 0x80},
	{0x5401, 16 ^ 15, 14 ^ 15},
	{0x5101, 17 ^ 16, 15 ^ 16},
	{0x4801, 18 ^ 17, 16 ^ 17},
	{0x3801, 19 ^ 18, 17 ^ 18},
	{0x3401, 20 ^ 19, 18 ^ 19},
	{0x3001, 21 ^ 20, 19 ^ 20},
	{0x2801, 22 ^ 21, 19 ^ 21},
	{0x2401, 23 ^ 22, 20 ^ 22},
	{0x2201, 24 ^ 23, 21 ^ 23},
	{0x1C01, 25 ^ 24, 22 ^ 24},
	{0x1801, 26 ^ 25, 23 ^ 25},
	{0x1601, 27 ^ 26, 24 ^ 26},
	{0x1401, 28 ^ 27, 25 ^ 27},
	{0x1201, 29 ^ 28, 26 ^ 28},
	{0x1101, 30 ^ 29, 27 ^ 29},
	{0x0AC1, 31 ^ 30, 28 ^ 30},
	{0x09C1, 32 ^ 31, 29 ^ 31},
	{0x08A1, 33 ^ 32, 30 ^ 32},
	{0x0521, 34 ^ 33, 31 ^ 33},
	{0x0441, 35 ^ 34, 32 ^ 34},
	{0x02A1, 36 ^ 35, 33 ^ 35},
	{0x0221, 37 ^ 36, 34 ^ 36},
	{0x0141, 38 ^ 37, 35 ^ 37},
	{0x0111, 39 ^ 38, 36 ^ 38},
	{0x0085, 40 ^ 39, 37 ^ 39},
	{0x0049, 41 ^ 40, 38 ^ 40},
	{0x0025, 42 ^ 41, 39 ^ 41},
	{0x0015, 43 ^ 42, 40 ^ 42},
	{0x0009, 44 ^ 43, 41 ^ 43},
	{0x0005, 45 ^ 44, 42 ^ 44},
	{0x0001, 45 ^ 45, 43 ^ 45},
	{0x5601, 46 ^ 46, 46 ^ 46},
}

// ArithmeticDecoder implements the JBIG2 MQ arithmetic decoder.
type ArithmeticDecoder struct {
	data          []byte
	offset        int
	nextWord      uint32
	nextWordBytes int
	c             uint32
	a             uint32
	ct            int
	err           error
	contexts      []uint8
}

// NewArithmeticDecoder creates a new arithmetic decoder.
func NewArithmeticDecoder(data []byte) *ArithmeticDecoder {
	ad := &ArithmeticDecoder{
		data:     data,
		contexts: make([]uint8, 1<<16),
	}
	if ad.readNextWord() == 0 {
		ad.err = io.EOF
		return ad
	}

	ad.c = (^(ad.nextWord >> 8)) & 0xFF0000
	if err := ad.byteIn(); err != nil {
		ad.err = err
		return ad
	}
	ad.c <<= 7
	ad.ct -= 7
	ad.a = 0x8000
	return ad
}

// DecodeBit decodes one bit using the decoder-owned context table.
func (ad *ArithmeticDecoder) DecodeBit(context uint8) (uint8, error) {
	return ad.DecodeBitContext(ad.contexts, uint32(context))
}

// DecodeBitContext decodes one bit using the supplied JBIG2 context state table.
func (ad *ArithmeticDecoder) DecodeBitContext(stats []uint8, context uint32) (uint8, error) {
	if ad.err != nil {
		return 0, ad.err
	}
	if context >= uint32(len(stats)) {
		return 0, fmt.Errorf("arithmetic context out of range: %d", context)
	}

	cx := stats[context]
	index := cx & 0x7f
	if int(index) >= len(arithmeticQeTable) {
		return 0, fmt.Errorf("arithmetic probability index out of range: %d", index)
	}
	qe := arithmeticQeTable[index]

	ad.a -= qe.qe
	if (ad.c >> 16) < ad.a {
		if ad.a&0x8000 != 0 {
			return cx >> 7, nil
		}

		var bit uint8
		if ad.a < qe.qe {
			bit = 1 - (cx >> 7)
			stats[context] = cx ^ qe.lpsXor
		} else {
			bit = cx >> 7
			stats[context] = cx ^ qe.mpsXor
		}
		if err := ad.renorm(); err != nil {
			return 0, err
		}
		return bit, nil
	}

	ad.c -= ad.a << 16
	var bit uint8
	if ad.a < qe.qe {
		ad.a = qe.qe
		bit = cx >> 7
		stats[context] = cx ^ qe.mpsXor
	} else {
		ad.a = qe.qe
		bit = 1 - (cx >> 7)
		stats[context] = cx ^ qe.lpsXor
	}
	if err := ad.renorm(); err != nil {
		return 0, err
	}
	return bit, nil
}

// DecodeInt decodes one JBIG2 arithmetic integer using the supplied context table.
func (ad *ArithmeticDecoder) DecodeInt(stats []uint8) (int, bool, error) {
	prev := uint32(1)
	decodeBit := func() (uint8, error) {
		bit, err := ad.DecodeBitContext(stats, prev)
		if err != nil {
			return 0, err
		}
		if prev < 0x100 {
			prev = (prev << 1) | uint32(bit)
		} else {
			prev = (((prev << 1) | uint32(bit)) & 0x1ff) | 0x100
		}
		return bit, nil
	}

	sign, err := decodeBit()
	if err != nil {
		return 0, false, err
	}
	v, err := decodeArithmeticIntegerMagnitude(decodeBit)
	if err != nil {
		return 0, false, err
	}
	if sign != 0 {
		if v == 0 {
			return 0, false, nil
		}
		return -int(v), true, nil
	}
	return int(v), true, nil
}

func decodeArithmeticIntegerMagnitude(decodeBit func() (uint8, error)) (uint32, error) {
	bit, err := decodeBit()
	if err != nil {
		return 0, err
	}
	if bit == 0 {
		return decodeArithmeticIntegerTail(decodeBit, 2, 0)
	}
	bit, err = decodeBit()
	if err != nil {
		return 0, err
	}
	if bit == 0 {
		return decodeArithmeticIntegerTail(decodeBit, 4, 4)
	}
	bit, err = decodeBit()
	if err != nil {
		return 0, err
	}
	if bit == 0 {
		return decodeArithmeticIntegerTail(decodeBit, 6, 20)
	}
	bit, err = decodeBit()
	if err != nil {
		return 0, err
	}
	if bit == 0 {
		return decodeArithmeticIntegerTail(decodeBit, 8, 84)
	}
	bit, err = decodeBit()
	if err != nil {
		return 0, err
	}
	if bit == 0 {
		return decodeArithmeticIntegerTail(decodeBit, 12, 340)
	}
	return decodeArithmeticIntegerTail(decodeBit, 32, 4436)
}

func decodeArithmeticIntegerTail(decodeBit func() (uint8, error), bitCount int, base uint32) (uint32, error) {
	var value uint32
	for i := 0; i < bitCount; i++ {
		bit, err := decodeBit()
		if err != nil {
			return 0, err
		}
		value = (value << 1) | uint32(bit)
	}
	return value + base, nil
}

// DecodeIAID decodes one immediate arithmetic integer ID.
func (ad *ArithmeticDecoder) DecodeIAID(codeLen uint, stats []uint8) (uint32, error) {
	prev := uint32(1)
	for i := uint(0); i < codeLen; i++ {
		bit, err := ad.DecodeBitContext(stats, prev)
		if err != nil {
			return 0, err
		}
		prev = (prev << 1) | uint32(bit)
	}
	return prev - (1 << codeLen), nil
}

func (ad *ArithmeticDecoder) renorm() error {
	for {
		if ad.ct == 0 {
			if err := ad.byteIn(); err != nil {
				return err
			}
		}
		ad.a <<= 1
		ad.c <<= 1
		ad.ct--
		if ad.a&0x8000 != 0 {
			return nil
		}
	}
}

func (ad *ArithmeticDecoder) readNextWord() int {
	ad.nextWord = 0
	ad.nextWordBytes = 0
	for i := 0; i < 4 && ad.offset+i < len(ad.data); i++ {
		ad.nextWord |= uint32(ad.data[ad.offset+i]) << uint(24-i*8)
		ad.nextWordBytes++
	}
	ad.offset += ad.nextWordBytes
	return ad.nextWordBytes
}

func (ad *ArithmeticDecoder) byteIn() error {
	if ad.nextWordBytes == 0 {
		ad.assumeTerminatingMarker()
		return nil
	}

	b := byte(ad.nextWord >> 24)
	if b == 0xff {
		return ad.byteInAfterFF()
	}

	ad.nextWord <<= 8
	ad.nextWordBytes--
	if ad.nextWordBytes == 0 && ad.readNextWord() == 0 {
		ad.assumeTerminatingMarker()
		ad.c += 0xff00
		return nil
	}

	b = byte(ad.nextWord >> 24)
	ad.c += 0xff00 - (uint32(b) << 8)
	ad.ct = 8
	return nil
}

func (ad *ArithmeticDecoder) byteInAfterFF() error {
	if ad.nextWordBytes <= 1 {
		if ad.readNextWord() == 0 {
			ad.assumeTerminatingMarker()
			ad.c += 0xff00
			return nil
		}

		b1 := byte(ad.nextWord >> 24)
		if b1 > 0x8f {
			ad.ct = 8
			ad.nextWord = 0xff000000 | (ad.nextWord >> 8)
			ad.nextWordBytes = 2
			if ad.offset > 0 {
				ad.offset--
			}
			return nil
		}

		ad.c += 0xfe00 - (uint32(b1) << 9)
		ad.ct = 7
		return nil
	}

	b1 := byte(ad.nextWord >> 16)
	if b1 > 0x8f {
		ad.ct = 8
		return nil
	}

	ad.nextWordBytes--
	ad.nextWord <<= 8
	ad.c += 0xfe00 - (uint32(b1) << 9)
	ad.ct = 7
	return nil
}

func (ad *ArithmeticDecoder) assumeTerminatingMarker() {
	ad.nextWord = 0xff900000
	ad.nextWordBytes = 2
	ad.ct = 8
}
