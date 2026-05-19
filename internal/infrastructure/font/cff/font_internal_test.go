package cff

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFont_MinimalData(t *testing.T) {
	font, err := NewFont(buildMinimalCFFData())
	require.NoError(t, err)
	require.NotNil(t, font)

	assert.Equal(t, "CFF Font", font.Name())
	assert.Equal(t, uint16(1000), font.UnitsPerEm())
	assert.False(t, font.IsCIDFont())
	assert.False(t, font.IsSymbolic())
	assert.Equal(t, [6]float64{0.001, 0, 0, 0.001, 0, 0}, font.GetFontMatrix())
	assert.Equal(t, buildMinimalCFFData(), font.FontData())
}

func TestNewFont_InvalidOffSize(t *testing.T) {
	_, err := NewFont([]byte{1, 0, 4, 0, 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid offsize")
}

func TestNewFont_InvalidHeader(t *testing.T) {
	_, err := NewFont([]byte{1, 0, 4})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EOF")
}

func TestFont_CharacterOps(t *testing.T) {
	font, err := NewFont(buildMinimalCFFData())
	require.NoError(t, err)

	glyph, err := font.CharCodeToGlyph(12)
	require.NoError(t, err)
	assert.Equal(t, uint32(12), glyph)

	name := font.GlyphName(12)
	assert.Equal(t, "gid12", name)

	width, err := font.GetGlyphWidth(12)
	require.NoError(t, err)
	assert.Equal(t, 500.0, width)
}

func TestFont_RenderGlyph_WithCharString(t *testing.T) {
	font, err := NewFont(buildMinimalCFFData())
	require.NoError(t, err)
	font.charStringsIndex = &Index{
		data:    []byte{3, 5, 141, 14, 0},
		count:   1,
		offSize: 1,
	}

	path, err := font.RenderGlyph(0, 2000)
	require.NoError(t, err)
	require.NotNil(t, path)
	require.Len(t, path.Commands, 1)
	assert.Equal(t, 0.0, path.Bounds[0])
}

func TestFont_RenderGlyphFallback(t *testing.T) {
	font, err := NewFont(buildMinimalCFFData())
	require.NoError(t, err)

	path, err := font.RenderGlyph(3, 10)
	require.NoError(t, err)
	require.NotNil(t, path)
	assert.Empty(t, path.Commands)
	assert.Equal(t, [4]float64{0, 0, 500, 0}, path.Bounds)
}

func TestFont_FontDataAndCID(t *testing.T) {
	font, err := NewFont(buildMinimalCFFData())
	require.NoError(t, err)
	assert.Equal(t, buildMinimalCFFData(), font.FontData())

	font.topDict = &TopDict{
		ROS: []byte{1, 2, 3},
	}
	assert.True(t, font.IsCIDFont())
}

func TestFont_ParseIndexAndHelpers(t *testing.T) {
	font := &Font{header: &Header{OffSize: 1}}

	indexData := []byte{1, 1, 1, 2, 0xAB}
	idx, err := font.parseIndex(bytes.NewReader(indexData))
	require.NoError(t, err)
	require.NotNil(t, idx)
	assert.Equal(t, int32(1), idx.count)
	assert.Equal(t, uint8(1), idx.offSize)

	emptyIdxData := []byte{0, 1}
	emptyIdx, err := font.parseIndex(bytes.NewReader(emptyIdxData))
	require.NoError(t, err)
	assert.Equal(t, int32(0), emptyIdx.count)

	_, err = font.parseIndex(bytes.NewReader([]byte{1}))
	require.Error(t, err)

	font.header.OffSize = 4
	offsize4Malformed := []byte{0, 0}
	_, err = font.parseIndex(bytes.NewReader(offsize4Malformed))
	require.Error(t, err)
}

func TestFont_ParseTopDictAndDeltaHelpers(t *testing.T) {
	font := &Font{topDict: &TopDict{}}
	topDictData := []byte{
		6, // FontBBox
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		17, 141, // CharStrings offset 2
		19, 0x03, 0x04, 0x05, // ROS for op19
		0,
	}

	err := font.parseTopDict(topDictData)
	require.NoError(t, err)
	assert.Equal(t, []float64{0, 0, 0, 0}, font.topDict.FontBBox)
	assert.Equal(t, int32(2), font.topDict.CharStringsOffset)
	assert.Equal(t, []byte{0x03, 0x04, 0x05}, font.topDict.ROS)
}

func TestFont_DeltaParsing(t *testing.T) {
	font := &Font{}

	array, err := font.parseDelta(bytes.NewReader([]byte{251, 140, 141}))
	require.NoError(t, err)
	assert.Equal(t, []int32{1, 2}, array)

	invalidArray, err := font.parseDeltaArrayN(bytes.NewReader([]byte{140}), 2)
	assert.Nil(t, invalidArray)
	require.Error(t, err)

	values, err := font.parseFixedArray(bytes.NewReader([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00}), 2)
	require.NoError(t, err)
	assert.Equal(t, 0.0, values[0])
	assert.InDelta(t, 1.0, values[1], 1e-9)

	_, err = font.parseFixedArray(bytes.NewReader([]byte{0x00, 0x00}), 2)
	require.Error(t, err)
}

func TestFont_ParsePrivateDict(t *testing.T) {
	font := &Font{
		topDict: &TopDict{
			PrivateOffset: 1,
			PrivateSize:   7,
		},
		data: []byte{0x00, 0xFF, 0x00, 0x01, 0x00, 0x00, 0x0A, 0x00},
	}

	err := font.parsePrivateDict()
	require.NoError(t, err)
	require.NotNil(t, font.privateDict)
	assert.InDelta(t, 1.0, font.privateDict.BlueScale, 1e-9)
}

func TestFont_ParseOperandAndDelta(t *testing.T) {
	font := &Font{}

	val, err := font.parseOperand(bytes.NewReader([]byte{32}))
	require.NoError(t, err)
	require.IsType(t, int32(0), val)
	assert.Equal(t, int32(-107), val.(int32))

	val2, err := font.parseOperand(bytes.NewReader([]byte{247, 1}))
	require.NoError(t, err)
	assert.Equal(t, int32(109), val2.(int32))

	val3, err := font.parseOperand(bytes.NewReader([]byte{251, 1}))
	require.NoError(t, err)
	assert.Equal(t, int32(-107), val3.(int32))

	flt, err := font.parseOperand(bytes.NewReader([]byte{255, 0x00, 0x01, 0x00, 0x00}))
	require.NoError(t, err)
	assert.InDelta(t, 1.0, flt.(float64), 1e-9)

	nilVal, err := font.parseOperand(bytes.NewReader([]byte{12}))
	require.NoError(t, err)
	assert.Nil(t, nilVal)

	_, err = font.parseOperand(bytes.NewReader([]byte{255, 0x00}))
	require.Error(t, err)
}

func TestFont_CalculateBiasAndCharStringParser(t *testing.T) {
	font := &Font{}
	assert.Equal(t, int32(0), font.calculateBias([]int32{}))
	assert.Equal(t, int32(113), font.calculateBias([]int32{100}))
	assert.Equal(t, int32(107), font.calculateBias([]int32{1240}))

	commands, err := font.parseCharString([]byte{
		33, 34, 21, // args and rmoveto
		14, // endchar
	})
	require.NoError(t, err)
	require.Len(t, commands, 2)
	assert.Equal(t, opMoveTo, commands[0].opcode)
	assert.Equal(t, opClosePath, commands[1].opcode)

	complexCommands, err := font.parseCharString([]byte{
		140, 141, 21,
		142, 143, 5,
		144, 145, 6,
		146, 147, 7,
		148, 149, 150, 151, 152, 153, 8,
		140, 141, 142, 143, 144, 145, 146, 12, 25,
		14,
	})
	require.NoError(t, err)
	require.Greater(t, len(complexCommands), 1)
	assert.Equal(t, opClosePath, complexCommands[len(complexCommands)-1].opcode)
}

func TestFont_GetCharStringData(t *testing.T) {
	font, err := NewFont(buildMinimalCFFData())
	require.NoError(t, err)

	_, err = font.getCharStringData(0)
	require.Error(t, err)

	font.charStringsIndex = &Index{
		data:    []byte{0},
		count:   0,
		offSize: 1,
	}
	_, err = font.getCharStringData(0)
	require.Error(t, err)

	font.charStringsIndex = &Index{
		data:    []byte{3, 5, 0x8D, 14, 0},
		count:   1,
		offSize: 1,
	}
	data, err := font.getCharStringData(0)
	require.NoError(t, err)
	assert.Equal(t, []byte{0x8D, 14}, data)
}

func TestFont_UpdateBoundsAndMatrix(t *testing.T) {
	var minX, minY, maxX, maxY float64 = 5, 5, 1, 1
	updateBounds(&minX, &minY, &maxX, &maxY, 2, 10)

	assert.Equal(t, 2.0, minX)
	assert.Equal(t, 5.0, minY)
	assert.Equal(t, 2.0, maxX)
	assert.Equal(t, 10.0, maxY)

	font, err := NewFont(buildMinimalCFFData())
	require.NoError(t, err)
	assert.Equal(t, [6]float64{0.001, 0, 0, 0.001, 0, 0}, font.GetFontMatrix())
}

func TestFont_ParseIndexEmptyData(t *testing.T) {
	font := &Font{header: &Header{OffSize: 4}}
	_, err := font.parseIndex(bytes.NewReader([]byte{0}))
	require.Error(t, err)
}

func TestFont_IsCIDFont(t *testing.T) {
	font, err := NewFont(buildMinimalCFFData())
	require.NoError(t, err)
	require.NotNil(t, font)

	assert.False(t, font.IsCIDFont())
	font.topDict = &TopDict{ROS: []byte{1, 2, 3}}
	assert.True(t, font.IsCIDFont())
}

func TestFont_ParseNameIndex_WithSidValues(t *testing.T) {
	font := &Font{header: &Header{OffSize: 1}}
	r := bytes.NewReader([]byte{
		1,    // count
		1,    // offSize
		1, 2, // offsets
		0x00, 0x00,
		42, // sid
	})

	err := font.parseNameIndex(r)
	require.NoError(t, err)
	require.NotNil(t, font.nameIndex)
	require.Len(t, font.nameIndex.Names, 1)
	assert.Equal(t, "sid42", font.nameIndex.Names[0])
}

func TestFont_ParseStringIndex_WithOneString(t *testing.T) {
	font := &Font{header: &Header{OffSize: 1}}
	r := bytes.NewReader([]byte{
		1, // count
		1, // offSize
		1, 4,
		'a', 'b', 'c', 'd', // string data
		4, // next offset
	})

	err := font.parseStringIndex(r)
	require.NoError(t, err)
	require.NotNil(t, font.stringIndex)
	require.Len(t, font.stringIndex.Strings, 1)
	assert.Equal(t, "abcd", font.stringIndex.Strings[0])
}

func TestFont_ParseGlobalSubrsAndCharStringsAndLocalSubrs(t *testing.T) {
	font := &Font{header: &Header{OffSize: 1}}
	r := bytes.NewReader([]byte{
		1, 1, // count, offSize
		1, 1,
		0x00, // global subr index payload
		9,    // global subr offset
	})
	err := font.parseGlobalSubrs(r)
	require.NoError(t, err)
	require.NotNil(t, font.globalSubrs)
	assert.Equal(t, int32(1), int32(len(font.globalSubrs.Subrs)))
	assert.Equal(t, int32(9), font.globalSubrs.Subrs[0])
	assert.Equal(t, int32(113), font.globalSubrs.Bias)

	font = &Font{
		header: &Header{OffSize: 1},
		data: []byte{
			1, 1, // count, offSize
			1, 1,
			0x8D, // payload
		},
		topDict: &TopDict{CharStringsOffset: 0},
	}
	err = font.parseCharStrings(bytes.NewReader(font.data))
	require.NoError(t, err)
	require.NotNil(t, font.charStringsIndex)
	assert.Equal(t, int32(1), font.charStringsIndex.count)

	font = &Font{
		header:      &Header{OffSize: 1},
		data:        []byte{0, 1, 1, 1, 1, 0x42, 10},
		topDict:     &TopDict{PrivateOffset: 0},
		privateDict: &PrivateDict{SubrsOffset: 1},
	}
	err = font.parseLocalSubrs()
	require.NoError(t, err)
	require.NotNil(t, font.privateDict.LocalSubrs)
	assert.Equal(t, int32(1), int32(len(font.privateDict.LocalSubrs.Subrs)))
	assert.Equal(t, int32(10), font.privateDict.LocalSubrs.Subrs[0])
	assert.Equal(t, int32(113), font.privateDict.LocalSubrs.Bias)
}

func TestFont_ParseTopDict_WithAdditionalOperators(t *testing.T) {
	font := &Font{topDict: &TopDict{}}
	err := font.parseTopDict([]byte{
		7, 139, // BlueValues
		17, 139, // CharStrings offset
		18, 251, 139, 139, // Private [size=0 offset=0]
		19, 0x41, 0x42, 0x43, // ROS
		0, // END
	})
	require.NoError(t, err)
	assert.Equal(t, int32(0), font.topDict.CharStringsOffset)
	assert.Equal(t, int32(0), font.topDict.PrivateSize)
	assert.Equal(t, int32(0), font.topDict.PrivateOffset)
	assert.Equal(t, []byte{'A', 'B', 'C'}, font.topDict.ROS)
}

func TestFont_ParseDelta_ReservedOpcode(t *testing.T) {
	font := &Font{}
	values, err := font.parseDelta(bytes.NewReader([]byte{
		255,
	}))
	require.NoError(t, err)
	assert.Nil(t, values)
}

func buildMinimalCFFData() []byte {
	return []byte{
		0x01, 0x00, 0x04, 0x01,
		// Name INDEX (count=0)
		0x00, 0x01,
		// Top DICT INDEX (count=0)
		0x00, 0x01,
		// String INDEX empty
		0x00, 0x01,
		// Global SUBRS empty
		0x00, 0x01,
	}
}
