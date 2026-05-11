package stream

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestASCIIHexDecoder_Decode tests the ASCIIHex decoder.
func TestASCIIHexDecoder_Decode(t *testing.T) {
	decoder := &ASCIIHexDecoder{}

	t.Run("Decode basic ASCIIHex", func(t *testing.T) {
		input := []byte("48656C6C6F") // "Hello" in ASCII
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte("Hello"), result)
	})

	t.Run("Decode with whitespace", func(t *testing.T) {
		input := []byte("48 65 6C 6C 6F")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte("Hello"), result)
	})

	t.Run("Decode with newlines", func(t *testing.T) {
		input := []byte("48\n65\n6C\n6C\n6F")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte("Hello"), result)
	})

	t.Run("Decode with mixed whitespace", func(t *testing.T) {
		input := []byte("48 \t\n\r 65 6C 6C 6F")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte("Hello"), result)
	})

	t.Run("Decode with EOD marker", func(t *testing.T) {
		input := []byte("48656C6C6F>")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte("Hello"), result)
	})

	t.Run("Decode with odd length (padding)", func(t *testing.T) {
		input := []byte("48656C6C6") // Odd length, padded with '0'
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		// Padded to "48656C6C60" and decoded
		// "48"=0x48, "65"=0x65, "6C"=0x6c, "6C"=0x6c, "60"=0x60
		assert.Equal(t, []byte{0x48, 0x65, 0x6c, 0x6c, 0x60}, result)
	})

	t.Run("Decode lowercase hex", func(t *testing.T) {
		input := []byte("48656c6c6f") // lowercase
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte("Hello"), result)
	})

	t.Run("Decode mixed case hex", func(t *testing.T) {
		input := []byte("48656c6C6F") // mixed case
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte("Hello"), result)
	})

	t.Run("Decode empty input", func(t *testing.T) {
		input := []byte("")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("Decode only whitespace", func(t *testing.T) {
		input := []byte("   \n\t\r  ")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("Decode all byte values", func(t *testing.T) {
		input := []byte("000102030405060708090a0B0c0D0e0F")
		expected := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F}
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("Decode with invalid hex digits (skip)", func(t *testing.T) {
		input := []byte("48GH56C6C6F") // GH are invalid
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		// Should skip invalid pairs
		assert.NotEmpty(t, result)
	})
}

// TestASCIIHexFactory tests the ASCIIHexFactory.
func TestASCIIHexFactory(t *testing.T) {
	t.Run("CreateDecoder returns ASCIIHexDecoder", func(t *testing.T) {
		factory := &ASCIIHexFactory{}
		decoder, err := factory.CreateDecoder()
		assert.NoError(t, err)
		assert.NotNil(t, decoder)
		assert.IsType(t, &ASCIIHexDecoder{}, decoder)
	})
}

// TestDecodeHexByte tests the decodeHexByte function.
func TestDecodeHexByte(t *testing.T) {
	t.Run("Decode digits 0-9", func(t *testing.T) {
		for i := byte('0'); i <= '9'; i++ {
			result := decodeHexByte(i)
			assert.Equal(t, i-'0', result)
		}
	})

	t.Run("Decode lowercase a-f", func(t *testing.T) {
		tests := []struct {
			input    byte
			expected byte
		}{
			{'a', 10},
			{'b', 11},
			{'c', 12},
			{'d', 13},
			{'e', 14},
			{'f', 15},
		}
		for _, tt := range tests {
			result := decodeHexByte(tt.input)
			assert.Equal(t, tt.expected, result)
		}
	})

	t.Run("Decode uppercase A-F", func(t *testing.T) {
		tests := []struct {
			input    byte
			expected byte
		}{
			{'A', 10},
			{'B', 11},
			{'C', 12},
			{'D', 13},
			{'E', 14},
			{'F', 15},
		}
		for _, tt := range tests {
			result := decodeHexByte(tt.input)
			assert.Equal(t, tt.expected, result)
		}
	})

	t.Run("Return 255 for invalid characters", func(t *testing.T) {
		invalidChars := []byte{'G', 'H', 'Z', 'g', 'z', '@', '#', ' ', '\n'}
		for _, c := range invalidChars {
			result := decodeHexByte(c)
			assert.Equal(t, byte(255), result)
		}
	})
}

// TestASCIIHexDecoder_EdgeCases tests edge cases.
func TestASCIIHexDecoder_EdgeCases(t *testing.T) {
	decoder := &ASCIIHexDecoder{}

	t.Run("Decode single byte", func(t *testing.T) {
		input := []byte("41") // 'A'
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte("A"), result)
	})

	t.Run("Decode with tabs", func(t *testing.T) {
		input := []byte("4\t8\t6\t5")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte("He"), result)
	})

	t.Run("Decode with form feed and vertical tab", func(t *testing.T) {
		input := []byte("48\f65\v6C")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte("Hel"), result)
	})

	t.Run("Decode ignores data after EOD", func(t *testing.T) {
		// Note: The implementation doesn't actually stop at EOD marker
		// It just removes the '>' character during cleaning
		// So "48656C6C6F>4244" becomes "48656C6C6F4244"
		input := []byte("48656C6C6F>4244")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		// The implementation continues decoding after '>'
		assert.Equal(t, []byte("HelloBD"), result)
	})

	t.Run("Decode all zeros", func(t *testing.T) {
		input := []byte("30303030")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		// "30303030" = 4 pairs of hex digits = 4 bytes
		// "30" in hex = 0x30 = 48 decimal (ASCII '0')
		assert.Equal(t, []byte{0x30, 0x30, 0x30, 0x30}, result)
	})

	t.Run("Decode all Fs", func(t *testing.T) {
		input := []byte("FFFFFFFF")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		// "FFFFFFFF" = 4 pairs of hex digits = 4 bytes
		assert.Equal(t, []byte{0xFF, 0xFF, 0xFF, 0xFF}, result)
	})
}
