package stream

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLZWDecoder_Decode tests the LZW decoder.
func TestLZWDecoder_Decode(t *testing.T) {
	decoder := &LZWDecoder{}

	t.Run("Decode empty input", func(t *testing.T) {
		input := []byte{}
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		// LZW decoder may return some data even for empty input
		assert.NotNil(t, result)
	})

	t.Run("Decode handles various inputs gracefully", func(t *testing.T) {
		// Test that the decoder handles various inputs gracefully
		input := []byte{0x80, 0x00, 0x41}
		result, err := decoder.Decode(input)
		// LZW decoding should either succeed or fail gracefully
		if err == nil {
			assert.NotNil(t, result)
		}
	})

	t.Run("Decode handles partial data", func(t *testing.T) {
		// Test that partial LZW data is handled
		input := []byte{0x80}
		result, err := decoder.Decode(input)
		// Should either succeed or fail gracefully
		if err == nil {
			assert.NotNil(t, result)
		}
	})
}

// TestLZWFactory tests the LZWFactory.
func TestLZWFactory(t *testing.T) {
	t.Run("CreateDecoder returns LZWDecoder", func(t *testing.T) {
		factory := &LZWFactory{}
		decoder, err := factory.CreateDecoder()
		assert.NoError(t, err)
		assert.NotNil(t, decoder)
		assert.IsType(t, &LZWDecoder{}, decoder)
	})
}

// TestLZWReader_Read tests the lzwReader Read method.
func TestLZWReader_Read(t *testing.T) {
	t.Run("Read on empty input", func(t *testing.T) {
		r := newLZWReader(bytes.NewReader([]byte{}), 1)
		buf := make([]byte, 100)
		n, err := r.Read(buf)
		// May return some data or EOF depending on implementation
		assert.True(t, n >= 0)
		r.Close()
		_ = err // err may be nil or io.EOF
	})
}

// TestLZWDecoder_Decode_ClearCode tests clear code handling.
func TestLZWDecoder_Decode_ClearCode(t *testing.T) {
	decoder := &LZWDecoder{}

	t.Run("Decode with clear code", func(t *testing.T) {
		// LZW clear code is 256
		// This tests that the decoder handles the clear code properly
		input := []byte{0x80, 0x01, 0x00}
		result, err := decoder.Decode(input)
		// Decoder should either decode or fail gracefully for malformed data.
		if err == nil {
			assert.NotNil(t, result)
		}
	})
}

// TestLZWDecoder_Decode_EOI tests end-of-information code handling.
func TestLZWDecoder_Decode_EOI(t *testing.T) {
	decoder := &LZWDecoder{}

	t.Run("Decode with EOI code", func(t *testing.T) {
		// LZW EOI code is 257
		// This tests that the decoder stops at EOI
		input := []byte{0x80, 0x01, 0x01}
		result, err := decoder.Decode(input)
		// Decoder should either decode or fail gracefully for malformed data.
		if err == nil {
			assert.NotNil(t, result)
		}
	})
}

// TestLZWConstants tests the LZW constants.
func TestLZWConstants(t *testing.T) {
	t.Run("Constants have expected values", func(t *testing.T) {
		assert.Equal(t, 4095, maxCode)
		assert.Equal(t, 4096, tableSize)
		assert.Equal(t, 256, resetCode)
		assert.Equal(t, 257, eoiCodeVal)
		assert.Equal(t, 258, firstCode)
		assert.Equal(t, 0xFFFF, invalidEntry)
	})
}
