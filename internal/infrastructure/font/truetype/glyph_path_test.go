package truetype

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCompositeGlyph_ParsesMoreComponentsFlag(t *testing.T) {
	font := testCompositeFont()

	data := buildCompositeGlyphData(
		compositeComponent{
			flags:    compositeArgWords | compositeArgsAreXYValues | compositeMoreComponents,
			glyphID:  0,
			arg1Word: 10,
			arg2Word: 0,
		},
		compositeComponent{
			flags:    compositeArgWords | compositeArgsAreXYValues,
			glyphID:  0,
			arg1Word: 20,
			arg2Word: 0,
		},
	)

	path, err := font.parseCompositeGlyph(data)
	require.NoError(t, err)
	require.NotNil(t, path)

	moveCount := 0
	moveXs := make([]float64, 0, 2)
	for _, elem := range path.Elements() {
		if elem.Op == opMoveTo {
			moveCount++
			moveXs = append(moveXs, elem.X)
		}
	}

	assert.Equal(t, 2, moveCount)
	assert.Equal(t, []float64{10, 20}, moveXs)
}

func TestParseCompositeGlyph_HandlesInstructionsAndFixedScale(t *testing.T) {
	font := testCompositeFont()

	data := buildCompositeGlyphData(
		compositeComponent{
			flags:         compositeArgWords | compositeArgsAreXYValues | compositeHaveScale | compositeHaveInstructions,
			glyphID:       0,
			arg1Word:      30,
			arg2Word:      40,
			scaleFixed214: 0x2000, // 0.5
		},
	)

	path, err := font.parseCompositeGlyph(data)
	require.NoError(t, err)
	require.NotNil(t, path)

	elems := path.Elements()
	require.GreaterOrEqual(t, len(elems), 2)
	require.Equal(t, opMoveTo, elems[0].Op)
	require.Equal(t, opLineTo, elems[1].Op)

	assert.Equal(t, 30.0, elems[0].X)
	assert.Equal(t, 40.0, elems[0].Y)
	assert.InDelta(t, 5.0, math.Abs(elems[1].X-elems[0].X), 0.001)
}

type compositeComponent struct {
	flags         uint16
	scaleFixed214 int16
	arg1Word      int16
	arg2Word      int16
	glyphID       uint16
}

func buildCompositeGlyphData(components ...compositeComponent) []byte {
	buf := bytes.NewBuffer(nil)

	for _, c := range components {
		_ = binary.Write(buf, binary.BigEndian, c.flags)
		_ = binary.Write(buf, binary.BigEndian, c.glyphID)

		if c.flags&compositeArgWords != 0 {
			_ = binary.Write(buf, binary.BigEndian, c.arg1Word)
			_ = binary.Write(buf, binary.BigEndian, c.arg2Word)
		} else {
			buf.WriteByte(byte(c.arg1Word))
			buf.WriteByte(byte(c.arg2Word))
		}

		switch {
		case c.flags&compositeHaveScale != 0:
			_ = binary.Write(buf, binary.BigEndian, c.scaleFixed214)
		case c.flags&compositeHaveXYScale != 0:
			_ = binary.Write(buf, binary.BigEndian, c.scaleFixed214)
			_ = binary.Write(buf, binary.BigEndian, c.scaleFixed214)
		case c.flags&compositeHaveTwoByTwo != 0:
			_ = binary.Write(buf, binary.BigEndian, c.scaleFixed214)
			_ = binary.Write(buf, binary.BigEndian, int16(0))
			_ = binary.Write(buf, binary.BigEndian, int16(0))
			_ = binary.Write(buf, binary.BigEndian, c.scaleFixed214)
		}
	}

	if len(components) > 0 && components[len(components)-1].flags&compositeHaveInstructions != 0 {
		_ = binary.Write(buf, binary.BigEndian, uint16(3))
		buf.Write([]byte{0x01, 0x02, 0x03})
	}

	return buf.Bytes()
}

func testCompositeFont() *Font {
	return &Font{
		file: &FontFile{
			Glyf: &GlyfTable{
				Glyphs: []Glyph{
					{
						NumberOfContours: 1,
						Instructions: []byte{
							0x00, 0x01, // endPtsOfContours[0]
							0x00, 0x00, // instructionLength
							0x31, 0x33, // flags
							0x0A, // x delta for point 1
						},
					},
				},
			},
		},
	}
}
