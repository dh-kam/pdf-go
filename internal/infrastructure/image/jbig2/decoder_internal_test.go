package jbig2

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArithmeticDecoder_DecodeBit_UsesMQState(t *testing.T) {
	decoder := NewArithmeticDecoder([]byte{0b10110000})

	bits := make([]uint8, 0, 8)
	for i := 0; i < 8; i++ {
		bit, err := decoder.DecodeBit(0)
		require.NoError(t, err)
		bits = append(bits, bit)
	}

	assert.NotEqual(t, []uint8{1, 0, 1, 1, 0, 0, 0, 0}, bits)

	_, err := decoder.DecodeBit(0)
	require.NoError(t, err)
}

func TestMMRDecoder_DecodeLine_AllWhiteVerticalZeroCodes(t *testing.T) {
	decoder := NewMMRDecoder([]byte{0b11000000}, 8, 2)

	line1, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00}, line1)

	line2, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00}, line2)

	_, err = decoder.DecodeLine()
	require.Error(t, err)
	assert.ErrorIs(t, err, io.EOF)
}

func TestMMRDecoder_DecodeLine_VerticalZeroCopiesReferenceLine(t *testing.T) {
	// First line: horizontal white 4, black 4. Second line: vertical-0, vertical-0.
	decoder := NewMMRDecoder(testBits("001"+"1011"+"011"+"1"+"1"), 8, 2)

	line1, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x0f}, line1)

	line2, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x0f}, line2)
}

func TestMMRDecoder_DecodeLine_VerticalRightOne(t *testing.T) {
	// First line: white 4, black 4. Second line moves the first boundary right by one.
	decoder := NewMMRDecoder(testBits("001"+"1011"+"011"+"011"+"1"), 8, 2)

	line1, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x0f}, line1)

	line2, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x07}, line2)
}

func TestMMRDecoder_DecodeLine_VerticalLeftOne(t *testing.T) {
	// First line: white 4, black 4. Second line moves the first boundary left by one.
	decoder := NewMMRDecoder(testBits("001"+"1011"+"011"+"010"+"1"), 8, 2)

	line1, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x0f}, line1)

	line2, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x1f}, line2)
}

func TestMMRDecoder_DecodeLine_PassModeSkipsReferenceRunPair(t *testing.T) {
	// First line: white 4, black 4. Second line pass-skips both reference runs to all white.
	decoder := NewMMRDecoder(testBits("001"+"1011"+"011"+"0001"), 8, 2)

	line1, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x0f}, line1)

	line2, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00}, line2)
}

func TestMMRDecoder_DecodeLine_DoesNotTreatPayloadAsRawRows(t *testing.T) {
	decoder := NewMMRDecoder([]byte{0x00}, 8, 1)

	_, err := decoder.DecodeLine()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported MMR code")
}

func TestMMRDecoder_DecodeLine_HorizontalAllBlackLine(t *testing.T) {
	// Horizontal mode, white run 0, black run 8.
	decoder := NewMMRDecoder([]byte{0x26, 0xA2, 0x80}, 8, 1)

	line, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0xff}, line)

	_, err = decoder.DecodeLine()
	require.Error(t, err)
	assert.ErrorIs(t, err, io.EOF)
}

func TestMMRDecoder_DecodeLine_HorizontalWhite4Black4Line(t *testing.T) {
	// Horizontal mode, white run 4, black run 4.
	decoder := NewMMRDecoder([]byte{0x36, 0xC0}, 8, 1)

	line, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x0f}, line)
}

func TestMMRDecoder_DecodeLine_HorizontalWhite8Black8Line(t *testing.T) {
	// Horizontal mode, white run 8, black run 8.
	decoder := NewMMRDecoder([]byte{0x33, 0x14}, 16, 1)

	line, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00, 0xff}, line)
}

func TestMMRDecoder_DecodeLine_HorizontalWhite8Black0Line(t *testing.T) {
	// Horizontal mode, white run 8, black run 0.
	decoder := NewMMRDecoder(testBits("001"+"10011"+"0000110111"), 8, 1)

	line, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x00}, line)
}

func TestMMRDecoder_DecodeLine_RepeatedHorizontalPairs(t *testing.T) {
	// Two horizontal pairs: white 4, black 4, white 4, black 4.
	decoder := NewMMRDecoder([]byte{0x36, 0xCD, 0xB0}, 16, 1)

	line, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x0f, 0x0f}, line)
}

func TestMMRDecoder_DecodeLine_HorizontalWhite1Black7Line(t *testing.T) {
	// Horizontal mode, white run 1, black run 7.
	decoder := NewMMRDecoder([]byte{0x23, 0x8c}, 8, 1)

	line, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x7f}, line)
}

func TestMMRDecoder_DecodeLine_NonByteAlignedWidth(t *testing.T) {
	// Horizontal mode, white run 1, black run 8 on a 9-pixel row.
	decoder := NewMMRDecoder(testBits("001"+"000111"+"000101"), 9, 1)

	line, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x7f, 0x80}, line)
}

func TestMMRDecoder_DecodeLine_HorizontalSmallTerminatingRuns(t *testing.T) {
	tests := []struct {
		name string
		bits string
		want byte
	}{
		{
			name: "white2 black6",
			bits: "001" + "0111" + "0010",
			want: 0x3f,
		},
		{
			name: "white3 black5",
			bits: "001" + "1000" + "0011",
			want: 0x1f,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := NewMMRDecoder(testBits(tt.bits), 8, 1)

			line, err := decoder.DecodeLine()
			require.NoError(t, err)
			assert.Equal(t, []byte{tt.want}, line)
		})
	}
}

func TestMMRDecoder_DecodeLine_InvalidWidth(t *testing.T) {
	decoder := NewMMRDecoder([]byte{0x00}, 0, 1)

	_, err := decoder.DecodeLine()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid MMR width")
}

func testBits(bits string) []byte {
	bits = strings.ReplaceAll(bits, " ", "")
	if bits == "" {
		return nil
	}
	out := make([]byte, (len(bits)+7)/8)
	for i, bit := range bits {
		if bit == '1' {
			out[i/8] |= 1 << (7 - (i % 8))
		}
	}
	return out
}
