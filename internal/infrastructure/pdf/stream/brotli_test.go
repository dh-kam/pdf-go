package stream

import (
	"bytes"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// compressBrotli compresses data using Brotli for testing.
func compressBrotli(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := brotli.NewWriter(&buf)
	_, err := writer.Write(data)
	if err != nil {
		return nil, err
	}
	err = writer.Close()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func TestDecodeBrotli_BasicCompression(t *testing.T) {
	original := []byte("Hello, World! This is a test of Brotli compression.")

	compressed, err := compressBrotli(original)
	require.NoError(t, err)

	decompressed, err := DecodeBrotli(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestDecodeBrotli_EmptyInput(t *testing.T) {
	decompressed, err := DecodeBrotli([]byte{})
	require.NoError(t, err)
	assert.Equal(t, []byte{}, decompressed)
}

func TestDecodeBrotli_LargeData(t *testing.T) {
	// Create 100KB of test data
	original := make([]byte, 100*1024)
	for i := range original {
		original[i] = byte(i % 256)
	}

	compressed, err := compressBrotli(original)
	require.NoError(t, err)

	// Brotli should compress this well (repetitive pattern)
	assert.Less(t, len(compressed), len(original))

	decompressed, err := DecodeBrotli(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestDecodeBrotli_RepetitiveData(t *testing.T) {
	// Highly compressible data
	original := bytes.Repeat([]byte("AAAAA"), 1000)

	compressed, err := compressBrotli(original)
	require.NoError(t, err)

	// Should compress very well
	assert.Less(t, len(compressed), len(original)/10)

	decompressed, err := DecodeBrotli(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestDecodeBrotli_BinaryData(t *testing.T) {
	// Binary data with all byte values
	original := make([]byte, 256)
	for i := range original {
		original[i] = byte(i)
	}

	compressed, err := compressBrotli(original)
	require.NoError(t, err)

	decompressed, err := DecodeBrotli(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestDecodeBrotli_CorruptedData(t *testing.T) {
	// Invalid Brotli data
	corrupted := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF}

	_, err := DecodeBrotli(corrupted)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "brotli decode failed")
}

func TestDecodeBrotli_TruncatedData(t *testing.T) {
	original := []byte("This data will be truncated after compression")

	compressed, err := compressBrotli(original)
	require.NoError(t, err)

	// Truncate the compressed data
	truncated := compressed[:len(compressed)/2]

	_, err = DecodeBrotli(truncated)
	assert.Error(t, err)
}

func TestBrotliDecoder_Decode(t *testing.T) {
	original := []byte("Testing BrotliDecoder interface")

	compressed, err := compressBrotli(original)
	require.NoError(t, err)

	decoder := &BrotliDecoder{}
	decompressed, err := decoder.Decode(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestBrotliDecoderFactory_CreateDecoder(t *testing.T) {
	factory := &BrotliDecoderFactory{}
	decoder, err := factory.CreateDecoder()
	require.NoError(t, err)
	assert.NotNil(t, decoder)

	// Test that the decoder works
	original := []byte("Factory-created decoder test")
	compressed, err := compressBrotli(original)
	require.NoError(t, err)

	decompressed, err := decoder.Decode(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

func TestBrotliDecoder_Registration(t *testing.T) {
	// Test that Brotli decoder is registered
	decoder, err := GetDecoder(FilterBrotli)
	require.NoError(t, err)
	assert.NotNil(t, decoder)

	// Test decoding through the registered decoder
	original := []byte("Testing registered Brotli decoder")
	compressed, err := compressBrotli(original)
	require.NoError(t, err)

	decompressed, err := decoder.Decode(compressed)
	require.NoError(t, err)
	assert.Equal(t, original, decompressed)
}

// Benchmark Brotli decoding performance
func BenchmarkDecodeBrotli_Small(b *testing.B) {
	data := []byte("Small test data for benchmarking")
	compressed, _ := compressBrotli(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DecodeBrotli(compressed)
	}
}

func BenchmarkDecodeBrotli_Medium(b *testing.B) {
	data := bytes.Repeat([]byte("Medium test data for benchmarking. "), 100)
	compressed, _ := compressBrotli(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DecodeBrotli(compressed)
	}
}

func BenchmarkDecodeBrotli_Large(b *testing.B) {
	data := make([]byte, 1024*1024) // 1MB
	for i := range data {
		data[i] = byte(i % 256)
	}
	compressed, _ := compressBrotli(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DecodeBrotli(compressed)
	}
}
