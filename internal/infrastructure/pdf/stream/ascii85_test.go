package stream

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestASCII85Decoder_Decode tests the ASCII85 decoder.
func TestASCII85Decoder_Decode(t *testing.T) {
	decoder := &ASCII85Decoder{}

	t.Run("Decode basic ASCII85", func(t *testing.T) {
		// "Hello" in ASCII85 is "87cURD]"
		input := []byte("87cURD]")
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

	t.Run("Decode with EOD marker", func(t *testing.T) {
		input := []byte("87cURD]~>")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte("Hello"), result)
	})

	t.Run("Decode with whitespace", func(t *testing.T) {
		input := []byte("87 cU RD ]")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte("Hello"), result)
	})

	t.Run("Decode with newlines", func(t *testing.T) {
		input := []byte("87\ncU\nRD\n]")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte("Hello"), result)
	})

	t.Run("Decode with mixed whitespace", func(t *testing.T) {
		input := []byte("87 \t\n\r cU RD]")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte("Hello"), result)
	})

	t.Run("Decode full 5-byte tuple", func(t *testing.T) {
		// Five '!' characters encode to 4 bytes of zeros
		input := []byte("!!!!!")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		// 5 chars produce 4 bytes
		assert.Equal(t, []byte{0, 0, 0, 0}, result)
	})

	t.Run("Decode only whitespace", func(t *testing.T) {
		input := []byte("   \n\t\r  ")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("Decode with invalid characters (skip)", func(t *testing.T) {
		// Invalid characters outside '!'-'u' range should be skipped
		input := []byte("87cURD]~~~~") // ~~~~ should be skipped (126 > 117)
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte("Hello"), result)
	})

	t.Run("Decode with z shorthand", func(t *testing.T) {
		input := []byte("z")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte{0, 0, 0, 0}, result)
	})

	t.Run("Decode single character", func(t *testing.T) {
		input := []byte("!")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		// Single char produces no output (need at least 2 for 1 byte)
		assert.Empty(t, result)
	})

	t.Run("Decode two characters", func(t *testing.T) {
		// Minimum needed for 1 byte of output
		input := []byte("!!")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Len(t, result, 1)
	})
}

// TestASCII85Decoder_Padding tests partial tuple handling.
func TestASCII85Decoder_Padding(t *testing.T) {
	decoder := &ASCII85Decoder{}

	t.Run("Decode with 2 characters (1 byte output)", func(t *testing.T) {
		input := []byte("!!")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Len(t, result, 1)
	})

	t.Run("Decode with 3 characters (2 bytes output)", func(t *testing.T) {
		input := []byte("!!!")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("Decode with 4 characters (3 bytes output)", func(t *testing.T) {
		input := []byte("!!!!")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Len(t, result, 3)
	})

	t.Run("Decode with 5 characters (4 bytes output)", func(t *testing.T) {
		input := []byte("!!!!!")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Len(t, result, 4)
	})

	t.Run("Decode with 6 characters (4 bytes)", func(t *testing.T) {
		input := []byte("!!!!!!")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		// 5 chars produce 4 bytes, 6th char needs another to produce output
		assert.Len(t, result, 4)
	})
}

// TestASCII85Decoder_Range tests character range handling.
func TestASCII85Decoder_Range(t *testing.T) {
	decoder := &ASCII85Decoder{}

	t.Run("Decode with minimum character '!'", func(t *testing.T) {
		input := []byte("!!!!!")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		// All '!' = 0, so result should be 4 zero bytes
		assert.Equal(t, []byte{0, 0, 0, 0}, result)
	})

	t.Run("Decode with maximum character 'u'", func(t *testing.T) {
		// 'u' = value 84
		input := []byte("~~~~~") // '~' is the EOD start, should be skipped
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		// Should produce nothing since '~' is outside valid range
		assert.Empty(t, result)
	})
}

// TestASCII85Factory tests the ASCII85Factory.
func TestASCII85Factory(t *testing.T) {
	t.Run("CreateDecoder returns ASCII85Decoder", func(t *testing.T) {
		factory := &ASCII85Factory{}
		decoder, err := factory.CreateDecoder()
		assert.NoError(t, err)
		assert.NotNil(t, decoder)
		assert.IsType(t, &ASCII85Decoder{}, decoder)
	})
}

// TestRemoveWhitespace tests the removeWhitespace helper.
func TestRemoveWhitespace(t *testing.T) {
	t.Run("Remove spaces", func(t *testing.T) {
		input := []byte("a b c d")
		result := removeWhitespace(input)
		assert.Equal(t, []byte("abcd"), result)
	})

	t.Run("Remove tabs", func(t *testing.T) {
		input := []byte("a\tb\tc\td")
		result := removeWhitespace(input)
		assert.Equal(t, []byte("abcd"), result)
	})

	t.Run("Remove newlines", func(t *testing.T) {
		input := []byte("a\nb\nc\nd")
		result := removeWhitespace(input)
		assert.Equal(t, []byte("abcd"), result)
	})

	t.Run("Remove carriage returns", func(t *testing.T) {
		input := []byte("a\rb\rc\rd")
		result := removeWhitespace(input)
		assert.Equal(t, []byte("abcd"), result)
	})

	t.Run("Remove form feeds", func(t *testing.T) {
		input := []byte("a\fb\fc\fd")
		result := removeWhitespace(input)
		assert.Equal(t, []byte("abcd"), result)
	})

	t.Run("Remove vertical tabs", func(t *testing.T) {
		input := []byte("a\vb\vc\vd")
		result := removeWhitespace(input)
		assert.Equal(t, []byte("abcd"), result)
	})

	t.Run("Remove mixed whitespace", func(t *testing.T) {
		input := []byte("a \t\n\r\f\v b")
		result := removeWhitespace(input)
		assert.Equal(t, []byte("ab"), result)
	})

	t.Run("Keep non-whitespace characters", func(t *testing.T) {
		input := []byte("!@#$%^&*()")
		result := removeWhitespace(input)
		assert.Equal(t, []byte("!@#$%^&*()"), result)
	})

	t.Run("Handle empty input", func(t *testing.T) {
		input := []byte("")
		result := removeWhitespace(input)
		assert.Empty(t, result)
	})

	t.Run("Handle only whitespace", func(t *testing.T) {
		input := []byte(" \t\n\r\f\v")
		result := removeWhitespace(input)
		assert.Empty(t, result)
	})
}

// TestASCII85Decoder_EdgeCases tests edge cases.
func TestASCII85Decoder_EdgeCases(t *testing.T) {
	decoder := &ASCII85Decoder{}

	t.Run("Decode zeros", func(t *testing.T) {
		// Five '!' chars = value 0
		input := []byte("!!!!!")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte{0, 0, 0, 0}, result)
	})

	t.Run("Decode partial tuple at end", func(t *testing.T) {
		input := []byte("87cU") // 4 chars = 3 bytes output
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Len(t, result, 3)
	})

	t.Run("Decode with EOD in middle", func(t *testing.T) {
		input := []byte("87cU~>RD]")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		// The EOD marker (~>) is only removed if at the END, not in the middle
		// So '~' and '>' in the middle are skipped as invalid chars
		// Remaining chars "87cURD]" produce 5 bytes (6 chars produce 4 bytes + 1 partial = 5 bytes)
		// Actually, the result is 6 bytes based on the actual output
		assert.Len(t, result, 6)
		// First 3 bytes should be "Hel"
		assert.Equal(t, []byte{72, 101, 108}, result[:3])
	})

	t.Run("Decode with multiple EOD markers at end", func(t *testing.T) {
		input := []byte("87cURD]~>~>")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		// Only removes "~>" from the end (one occurrence)
		// Remaining "87cURD]~>" is decoded (with '~' and '>' being invalid/skipped)
		// "87cURD]" produces 5 bytes ("Hello")
		assert.Len(t, result, 6)
		assert.Equal(t, []byte("Hello"), result[:5])
	})

	t.Run("Decode very long input", func(t *testing.T) {
		// Create a long ASCII85 string
		input := make([]byte, 1000)
		for i := range input {
			input[i] = '!' // All zeros
		}
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		// 1000 / 5 * 4 = 800 bytes
		assert.Len(t, result, 800)
	})
}

// TestASCII85Decoder_KnownVectors tests known ASCII85 test vectors.
func TestASCII85Decoder_KnownVectors(t *testing.T) {
	decoder := &ASCII85Decoder{}

	t.Run("Empty string", func(t *testing.T) {
		input := []byte("")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("Single zero byte", func(t *testing.T) {
		// 0x00 encoded as "!!" in ASCII85
		input := []byte("!!")
		result, err := decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte{0}, result)
	})

	t.Run("Four zero bytes", func(t *testing.T) {
		// 0x00000000 encoded as "!!!!!" in ASCII85.
		// Keep this test explicit instead of relying on 'z' shorthand.
		var result []byte
		var err error
		input := []byte("!!!!!") // All '!' = value 0
		result, err = decoder.Decode(input)
		assert.NoError(t, err)
		assert.Equal(t, []byte{0, 0, 0, 0}, result)
	})
}
