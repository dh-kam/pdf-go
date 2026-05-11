package cmap

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBinaryParserParse_Format6Subtable(t *testing.T) {
	data := make([]byte, 30)
	binary.BigEndian.PutUint32(data[0:4], 0x54434D66) // TMCf
	binary.BigEndian.PutUint16(data[4:6], 1)
	binary.BigEndian.PutUint16(data[6:8], 1) // num subtables

	// Subtable header
	binary.BigEndian.PutUint16(data[8:10], 3)  // platformID
	binary.BigEndian.PutUint16(data[10:12], 1) // encodingID
	binary.BigEndian.PutUint32(data[12:16], 16)

	// Subtable body at offset 16
	binary.BigEndian.PutUint16(data[16:18], 6)  // format
	binary.BigEndian.PutUint16(data[18:20], 14) // length
	binary.BigEndian.PutUint16(data[20:22], 0)  // language
	binary.BigEndian.PutUint16(data[22:24], 0x20)
	binary.BigEndian.PutUint16(data[24:26], 2) // entryCount
	binary.BigEndian.PutUint16(data[26:28], 1)
	binary.BigEndian.PutUint16(data[28:30], 2)

	parsed, err := ParseBinaryBytes(data)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	cid, ok := parsed.LookupCID(0x20)
	require.True(t, ok)
	assert.Equal(t, uint32(1), cid)

	cid, ok = parsed.LookupCID(0x21)
	require.True(t, ok)
	assert.Equal(t, uint32(2), cid)
}

func TestBinaryParserParseSubtable_InvalidOffset(t *testing.T) {
	p := NewBinaryParser(make([]byte, 16))
	p.pos = 8

	// platformID/encodingID are 0; subtable offset is invalid
	binary.BigEndian.PutUint32(p.data[12:16], 999)

	cmap := &BaseCMap{
		cidMapping: map[uint32]uint32{},
		uniMapping: map[uint32]string{},
	}
	err := p.parseSubtable(cmap, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid subtable offset")
}

func TestBinaryParserParseFormat0(t *testing.T) {
	data := make([]byte, 262)
	binary.BigEndian.PutUint16(data[0:2], 262) // length
	binary.BigEndian.PutUint16(data[2:4], 0)   // language
	data[4+65] = 7                             // char 65 => glyph 7

	p := NewBinaryParser(data)
	cmap := &BaseCMap{
		cidMapping: map[uint32]uint32{},
		uniMapping: map[uint32]string{},
	}
	err := p.parseFormat0(cmap, 3, 1)
	require.NoError(t, err)

	cid, ok := cmap.LookupCID(65)
	require.True(t, ok)
	assert.Equal(t, uint32(7), cid)
}

func TestBinaryParserParseFormat4(t *testing.T) {
	data := make([]byte, 30)
	// length/language/segCountX2/searchRange/entrySelector/rangeShift
	binary.BigEndian.PutUint16(data[0:2], 30)
	binary.BigEndian.PutUint16(data[2:4], 0)
	binary.BigEndian.PutUint16(data[4:6], 4) // segCountX2 => segCount 2
	binary.BigEndian.PutUint16(data[6:8], 0)
	binary.BigEndian.PutUint16(data[8:10], 0)
	binary.BigEndian.PutUint16(data[10:12], 0)

	// endCodes[2]
	binary.BigEndian.PutUint16(data[12:14], 0x0021)
	binary.BigEndian.PutUint16(data[14:16], 0xFFFF)
	// reservedPad
	binary.BigEndian.PutUint16(data[16:18], 0)
	// startCodes[2]
	binary.BigEndian.PutUint16(data[18:20], 0x0020)
	binary.BigEndian.PutUint16(data[20:22], 0xFFFF)
	// idDeltas[2]
	binary.BigEndian.PutUint16(data[22:24], 0x0001)
	binary.BigEndian.PutUint16(data[24:26], 0x0001)
	// idRangeOffsets[2]
	binary.BigEndian.PutUint16(data[26:28], 0)
	binary.BigEndian.PutUint16(data[28:30], 0)

	p := NewBinaryParser(data)
	cmap := &BaseCMap{
		cidMapping: map[uint32]uint32{},
		uniMapping: map[uint32]string{},
	}
	err := p.parseFormat4(cmap, 3, 1)
	require.NoError(t, err)

	cid, ok := cmap.LookupCID(0x20)
	require.True(t, ok)
	assert.Equal(t, uint32(0x21), cid)

	cid, ok = cmap.LookupCID(0x21)
	require.True(t, ok)
	assert.Equal(t, uint32(0x22), cid)
}

func TestBinaryParserParseFormat12(t *testing.T) {
	data := make([]byte, 24)
	// reserved (4 bytes) at 0..3
	binary.BigEndian.PutUint32(data[4:8], 24)  // length
	binary.BigEndian.PutUint32(data[8:12], 0)  // language
	binary.BigEndian.PutUint32(data[12:16], 1) // numGroups
	binary.BigEndian.PutUint32(data[16:20], 0x30)
	binary.BigEndian.PutUint32(data[20:24], 0x31)

	// append startGlyphID
	data = append(data, 0x00, 0x00, 0x00, 0x40)

	p := NewBinaryParser(data)
	cmap := &BaseCMap{
		cidMapping: map[uint32]uint32{},
		uniMapping: map[uint32]string{},
	}
	err := p.parseFormat12(cmap, 3, 1)
	require.NoError(t, err)

	cid, ok := cmap.LookupCID(0x30)
	require.True(t, ok)
	assert.Equal(t, uint32(0x40), cid)

	cid, ok = cmap.LookupCID(0x31)
	require.True(t, ok)
	assert.Equal(t, uint32(0x41), cid)
}

func TestBinaryParserReadUintBounds(t *testing.T) {
	p := NewBinaryParser([]byte{0x00, 0x01, 0x02})
	assert.Equal(t, uint16(1), p.readUint16())
	assert.Equal(t, uint16(0), p.readUint16())

	p2 := NewBinaryParser([]byte{0x00, 0x00, 0x00, 0x05})
	assert.Equal(t, uint32(5), p2.readUint32())
	assert.Equal(t, uint32(0), p2.readUint32())
}

func TestDetectFormatAndParseAuto(t *testing.T) {
	assert.Equal(t, "text", DetectFormat([]byte("abc")))

	bin := make([]byte, 8)
	binary.BigEndian.PutUint32(bin[0:4], 0x54434D20) // TMC<space>
	assert.Equal(t, "binary", DetectFormat(bin))

	textData := []byte("/CIDInit /ProcSet findresource begin")
	assert.Equal(t, "text", DetectFormat(textData))

	parsed, err := ParseAuto(textData)
	require.NoError(t, err)
	require.NotNil(t, parsed)
}
