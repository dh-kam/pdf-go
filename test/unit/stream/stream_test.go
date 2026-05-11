package stream_test

import (
	"bytes"
	"compress/flate"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"
)

func TestStream_NewStream(t *testing.T) {
	dict := entity.NewDict()
	data := []byte("test")

	s := stream.NewStream(dict, data)

	assert.NotNil(t, s)
	assert.Equal(t, dict, s.Dict())
	assert.Equal(t, data, s.RawData())
	assert.Equal(t, 4, s.Length())
}

func TestStream_Decode_NoFilter(t *testing.T) {
	dict := entity.NewDict()
	data := []byte("test data")

	s := stream.NewStream(dict, data)

	decoded, err := s.Decode()
	require.NoError(t, err)
	assert.Equal(t, data, decoded)
}

func TestEntityStream_Decode_UsesRegisteredDecoder(t *testing.T) {
	original := "entity stream decode"
	compressed := compressWithFlate(original)

	dict := entity.NewDict()
	dict.Set(entity.Name("Filter"), entity.Name("FlateDecode"))
	entityStream := entity.NewStream(dict, compressed)

	decoded, err := entityStream.Decode()
	require.NoError(t, err)
	assert.Equal(t, original, string(decoded))
}

func TestStream_Decode_Cached(t *testing.T) {
	dict := entity.NewDict()
	data := []byte("test data")

	s := stream.NewStream(dict, data)

	// First decode
	decoded1, err := s.Decode()
	require.NoError(t, err)

	// Second decode should use cache
	decoded2, err := s.Decode()
	require.NoError(t, err)

	// Check that the decoded data is the same (value equality)
	assert.Equal(t, decoded1, decoded2)
	// Check that the underlying data pointer is the same (cache hit)
	assert.Equal(t, &decoded1[0], &decoded2[0], "cached data should use same underlying array")
}

func TestStream_Reset(t *testing.T) {
	dict := entity.NewDict()
	data := []byte("test data")

	s := stream.NewStream(dict, data)

	// Decode to cache
	_, err := s.Decode()
	require.NoError(t, err)

	// Reset cache
	s.Reset()

	// Decode again (should re-decode)
	_, err = s.Decode()
	require.NoError(t, err)
}

func TestStream_Bytes(t *testing.T) {
	dict := entity.NewDict()
	data := []byte("test data")

	s := stream.NewStream(dict, data)

	bytes, err := s.Bytes()
	require.NoError(t, err)
	assert.Equal(t, data, bytes)
}

func TestStream_Reader(t *testing.T) {
	dict := entity.NewDict()
	data := []byte("test data")

	s := stream.NewStream(dict, data)

	reader, err := s.Reader()
	require.NoError(t, err)
	defer reader.Close()

	readData := make([]byte, len(data))
	n, err := reader.Read(readData)
	require.NoError(t, err)
	assert.Equal(t, len(data), n)
	assert.Equal(t, data, readData)
}

func TestFlateDecoder_Decode(t *testing.T) {
	// Test: compress a simple string and decompress it
	original := "Hello World!"
	compressed := compressWithFlate(original)

	decoder := &stream.FlateDecoder{}
	output, err := decoder.Decode(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, string(output))
}

func TestFlateDecoder_Empty(t *testing.T) {
	decoder := &stream.FlateDecoder{}
	output, err := decoder.Decode([]byte{})
	require.NoError(t, err)
	assert.Equal(t, 0, len(output))
}

func TestASCIIHexDecoder_Decode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple", "48656C6C6F", "Hello"},
		{"with delimiter", "48656C6C6F>", "Hello"},
		{"odd length", "48656C6C6F4", "Hello@"}, // Padded with '0' -> '@'
		{"whitespace", "48 65 6C 6C 6F", "Hello"},
		{"mixed case", "48656c6c6f", "Hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := &stream.ASCIIHexDecoder{}
			output, err := decoder.Decode([]byte(tt.input))
			require.NoError(t, err)
			assert.Equal(t, tt.expected, string(output))
		})
	}
}

func TestASCIIHexDecoder_Empty(t *testing.T) {
	decoder := &stream.ASCIIHexDecoder{}
	output, err := decoder.Decode([]byte{})
	require.NoError(t, err)
	assert.Equal(t, 0, len(output))
}

func TestASCII85Decoder_Decode(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "zero byte (5 chars -> 4 bytes)",
			input:    []byte("!!!!!~>"),
			expected: []byte{0x00, 0x00, 0x00, 0x00},
		},
		{
			name:     "partial tuple (4 chars -> 3 bytes)",
			input:    []byte("!!!!~>"),
			expected: []byte{0x00, 0x00, 0x00},
		},
		{
			name:     "partial tuple (2 chars -> 1 byte)",
			input:    []byte("!!~>"),
			expected: []byte{0x00},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := &stream.ASCII85Decoder{}
			output, err := decoder.Decode(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, output)
		})
	}
}

func TestASCII85Decoder_Empty(t *testing.T) {
	decoder := &stream.ASCII85Decoder{}
	output, err := decoder.Decode([]byte{})
	require.NoError(t, err)
	assert.Equal(t, 0, len(output))
}

func TestRunLengthDecoder_Decode(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "literal run",
			input:    []byte{3, 'A', 'B', 'C', 'D'},
			expected: []byte("ABCD"),
		},
		{
			name:     "repeat run",
			input:    []byte{255, 'X'},
			expected: bytes.Repeat([]byte("X"), 257-255),
		},
		{
			name:     "mixed",
			input:    []byte{1, 'A', 'B', 130, 'C', 'D'},
			expected: append([]byte("AB"), bytes.Repeat([]byte("C"), 257-130)...),
		},
		{
			name:     "with EOD",
			input:    []byte{1, 'A', 'B', 128, 'X', 'Y'}, // XY after EOD should be ignored
			expected: []byte("AB"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decoder := &stream.RunLengthDecoder{}
			output, err := decoder.Decode(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, output)
		})
	}
}

func TestRunLengthDecoder_Empty(t *testing.T) {
	decoder := &stream.RunLengthDecoder{}
	output, err := decoder.Decode([]byte{})
	require.NoError(t, err)
	assert.Equal(t, 0, len(output))
}

func TestGetDecoder_Flate(t *testing.T) {
	decoder, err := stream.GetDecoder(stream.FilterFlate)
	require.NoError(t, err)
	assert.NotNil(t, decoder)
}

func TestGetDecoder_ASCIIHex(t *testing.T) {
	decoder, err := stream.GetDecoder(stream.FilterASCIIHex)
	require.NoError(t, err)
	assert.NotNil(t, decoder)
}

func TestGetDecoder_ASCII85(t *testing.T) {
	decoder, err := stream.GetDecoder(stream.FilterASCII85)
	require.NoError(t, err)
	assert.NotNil(t, decoder)
}

func TestGetDecoder_LZW(t *testing.T) {
	decoder, err := stream.GetDecoder(stream.FilterLZW)
	require.NoError(t, err)
	assert.NotNil(t, decoder)
}

func TestGetDecoder_RunLength(t *testing.T) {
	decoder, err := stream.GetDecoder(stream.FilterRunLength)
	require.NoError(t, err)
	assert.NotNil(t, decoder)
}

func TestGetDecoder_CCITTFax(t *testing.T) {
	decoder, err := stream.GetDecoder(stream.FilterCCITTFax)
	require.NoError(t, err)
	assert.NotNil(t, decoder)
}

func TestGetDecoder_Unsupported(t *testing.T) {
	_, err := stream.GetDecoder(entity.Name("UnsupportedFilter"))
	assert.Error(t, err)
}

func TestUnsupportedFilterError_Error(t *testing.T) {
	err := &stream.UnsupportedFilterError{Filter: "TestFilter"}
	assert.Contains(t, err.Error(), "TestFilter")
	assert.Contains(t, err.Error(), "unsupported")
}

func TestStream_Decode_Flate(t *testing.T) {
	// Create a simple flate-compressed data
	original := "Hello, World!"
	compressed := compressWithFlate(original)

	dict := entity.NewDict()
	dict.Set(entity.Name("Filter"), entity.Name("FlateDecode"))

	s := stream.NewStream(dict, compressed)

	decoded, err := s.Decode()
	require.NoError(t, err)
	assert.Equal(t, original, string(decoded))
}

func TestStream_Decode_ASCIIHex(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("Filter"), entity.Name("ASCIIHexDecode"))

	data := []byte("48656C6C6F")
	s := stream.NewStream(dict, data)

	decoded, err := s.Decode()
	require.NoError(t, err)
	assert.Equal(t, "Hello", string(decoded))
}

func TestStream_Decode_MultipleFilters(t *testing.T) {
	dict := entity.NewDict()
	// First ASCIIHex decode, then Flate decode
	dict.Set(entity.Name("Filter"), entity.NewArray(
		entity.Name("ASCIIHexDecode"),
		entity.Name("FlateDecode"),
	))

	// Create flate-compressed data
	original := "Hello!"
	compressed := compressWithFlate(original)
	// Encode as hex
	hexData := encodeToHex(compressed)

	s := stream.NewStream(dict, hexData)

	decoded, err := s.Decode()
	require.NoError(t, err)
	assert.Equal(t, original, string(decoded))
}

// Helper functions for testing

func compressWithFlate(s string) []byte {
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.DefaultCompression)
	w.Write([]byte(s))
	w.Close()
	return buf.Bytes()
}

func encodeToHex(data []byte) []byte {
	hex := make([]byte, len(data)*2)
	for i, b := range data {
		hex[i*2] = "0123456789ABCDEF"[b>>4]
		hex[i*2+1] = "0123456789ABCDEF"[b&0x0F]
	}
	return hex
}
