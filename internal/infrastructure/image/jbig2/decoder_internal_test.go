package jbig2

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArithmeticDecoder_DecodeBit_FallbackBitStream(t *testing.T) {
	decoder := NewArithmeticDecoder([]byte{0b10110000})

	bits := make([]uint8, 0, 8)
	for i := 0; i < 8; i++ {
		bit, err := decoder.DecodeBit(0)
		require.NoError(t, err)
		bits = append(bits, bit)
	}

	assert.Equal(t, []uint8{1, 0, 1, 1, 0, 0, 0, 0}, bits)

	_, err := decoder.DecodeBit(0)
	require.Error(t, err)
	assert.ErrorIs(t, err, io.EOF)
}

func TestMMRDecoder_DecodeLine_FallbackReadsRawBytes(t *testing.T) {
	decoder := NewMMRDecoder([]byte{0xAA, 0x55}, 8, 2)

	line1, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0xAA}, line1)

	line2, err := decoder.DecodeLine()
	require.NoError(t, err)
	assert.Equal(t, []byte{0x55}, line2)

	_, err = decoder.DecodeLine()
	require.Error(t, err)
	assert.ErrorIs(t, err, io.EOF)
}

func TestMMRDecoder_DecodeLine_InvalidWidth(t *testing.T) {
	decoder := NewMMRDecoder([]byte{0x00}, 0, 1)

	_, err := decoder.DecodeLine()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid MMR width")
}
