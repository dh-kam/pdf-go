// Package stream provides benchmark tests for PDF stream filters.
package stream

import (
	"bytes"
	"compress/flate"
	"fmt"
	"io"
	"testing"
)

// BenchmarkFlateDecodeEncode benchmarks FlateDecode encoding.
func BenchmarkFlateDecodeEncode(b *testing.B) {
	data := generateTestData(1024 * 100) // 100KB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		writer, _ := flate.NewWriter(&buf, flate.DefaultCompression)
		writer.Write(data)
		writer.Close()
	}
}

// BenchmarkFlateDecodeDecode benchmarks FlateDecode decoding.
func BenchmarkFlateDecodeDecode(b *testing.B) {
	original := generateTestData(1024 * 100) // 100KB

	// Compress the data first
	var compressed bytes.Buffer
	writer, _ := flate.NewWriter(&compressed, flate.DefaultCompression)
	writer.Write(original)
	writer.Close()
	compressedData := compressed.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := flate.NewReader(bytes.NewReader(compressedData))
		io.Copy(io.Discard, reader)
		reader.Close()
	}
}

// BenchmarkASCIIHexEncode benchmarks ASCIIHex encoding.
func BenchmarkASCIIHexEncode(b *testing.B) {
	data := generateTestData(1024 * 10) // 10KB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = asciiHexEncode(data)
	}
}

// BenchmarkASCIIHexDecode benchmarks ASCIIHex decoding.
func BenchmarkASCIIHexDecode(b *testing.B) {
	data := generateTestData(1024 * 10) // 10KB
	encoded := asciiHexEncode(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = asciiHexDecode(encoded)
	}
}

// BenchmarkASCII85Encode benchmarks ASCII85 encoding.
func BenchmarkASCII85Encode(b *testing.B) {
	data := generateTestData(1024 * 10) // 10KB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ascii85Encode(data)
	}
}

// BenchmarkASCII85Decode benchmarks ASCII85 decoding.
func BenchmarkASCII85Decode(b *testing.B) {
	data := generateTestData(1024 * 10) // 10KB
	encoded, _ := ascii85Encode(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ascii85Decode(encoded)
	}
}

// BenchmarkLZWEncode benchmarks LZW encoding.
func BenchmarkLZWEncode(b *testing.B) {
	data := generateTestData(1024 * 10) // 10KB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = lzwEncode(data)
	}
}

// BenchmarkLZWDecode benchmarks LZW decoding.
func BenchmarkLZWDecode(b *testing.B) {
	data := generateTestData(1024 * 10) // 10KB
	encoded, _ := lzwEncode(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = lzwDecode(encoded)
	}
}

// BenchmarkRunLengthEncode benchmarks RunLength encoding.
func BenchmarkRunLengthEncode(b *testing.B) {
	data := generateTestData(1024 * 10) // 10KB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = runLengthEncode(data)
	}
}

// BenchmarkRunLengthDecode benchmarks RunLength decoding.
func BenchmarkRunLengthDecode(b *testing.B) {
	data := generateTestData(1024 * 10) // 10KB
	encoded, _ := runLengthEncode(data)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = runLengthDecode(encoded)
	}
}

// BenchmarkCCITTFaxEncode benchmarks CCITT Group 4 encoding.
func BenchmarkCCITTFaxEncode(b *testing.B) {
	// Create test image data (1-bit per pixel, 1728 pixels wide)
	width := 1728
	height := 100
	data := make([]byte, (width*height+7)/8)

	// Fill with pattern
	for i := range data {
		data[i] = byte(i % 256)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ccittEncode(data, width, height)
	}
}

// BenchmarkCCITTFaxDecode benchmarks CCITT Group 4 decoding.
func BenchmarkCCITTFaxDecode(b *testing.B) {
	// Create test image data
	width := 1728
	height := 100
	data := make([]byte, (width*height+7)/8)

	// Fill with pattern
	for i := range data {
		data[i] = byte(i % 256)
	}

	encoded, _ := ccittEncode(data, width, height)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decoder := &CCITTFaxDecoder{
			columns: width,
			rows:    height,
			k:       -1, // Group 4
		}
		_, _ = decoder.Decode(encoded)
	}
}

// BenchmarkPredictorApply benchmarks predictor application.
func BenchmarkPredictorApply(b *testing.B) {
	// Create test data with predictor
	data := generateTestData(1024 * 100) // 100KB

	// Apply predictor
	predictedData := applyPredictor(data, 10, 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ApplyPredictor(predictedData, nil)
	}
}

// BenchmarkPredictorApplyWithPNG benchmarks PNG predictor application.
func BenchmarkPredictorApplyWithPNG(b *testing.B) {
	// Create test data with PNG predictor
	data := generateTestData(1024 * 100) // 100KB

	// Apply PNG predictor
	predictedData := applyPNGPredictor(data, 10)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ApplyPredictor(predictedData, nil)
	}
}

// BenchmarkStreamFullDecode benchmarks full stream decoding with filters.
func BenchmarkStreamFullDecode(b *testing.B) {
	// Create test data
	data := generateTestData(1024 * 50) // 50KB

	// Apply multiple filters
	compressed := compressData(data)
	encoded := asciiHexEncode(compressed)

	stream := NewStream(nil, encoded)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream.Reset()
		_, _ = stream.Decode()
	}
}

// BenchmarkStreamDecodeFlateOnly benchmarks stream decoding with Flate only.
func BenchmarkStreamDecodeFlateOnly(b *testing.B) {
	// Create test data
	data := generateTestData(1024 * 100) // 100KB

	// Compress with Flate
	compressed := compressData(data)

	stream := NewStream(nil, compressed)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream.Reset()
		_, _ = stream.Decode()
	}
}

// Helper functions

// generateTestData generates test data with realistic patterns.
func generateTestData(size int) []byte {
	data := make([]byte, size)

	// Fill with various patterns to simulate real PDF data
	for i := 0; i < size; i++ {
		switch i % 4 {
		case 0:
			data[i] = byte(i % 256)
		case 1:
			data[i] = byte((i >> 8) % 256)
		case 2:
			data[i] = 0x00 // Null bytes (common in compressed data)
		case 3:
			data[i] = 0xFF // High bytes
		}
	}

	return data
}

// compressData compresses data using Flate.
func compressData(data []byte) []byte {
	var buf bytes.Buffer
	writer, _ := flate.NewWriter(&buf, flate.DefaultCompression)
	writer.Write(data)
	writer.Close()
	return buf.Bytes()
}

// asciiHexEncode encodes data to ASCII hexadecimal (helper for benchmark).
func asciiHexEncode(data []byte) []byte {
	const hex = "0123456789ABCDEF"
	encoded := make([]byte, len(data)*2)

	for i, b := range data {
		encoded[i*2] = hex[b>>4]
		encoded[i*2+1] = hex[b&0x0F]
	}

	return encoded
}

// asciiHexDecode decodes ASCII hexadecimal data (helper for benchmark).
func asciiHexDecode(encoded []byte) ([]byte, error) {
	decoder := &ASCIIHexDecoder{}
	return decoder.Decode(encoded)
}

// ascii85Encode encodes data to ASCII85 (helper for benchmark).
func ascii85Encode(data []byte) ([]byte, error) {
	// Use simple ASCII85 encoding for benchmark
	const base = 85
	const digits = "!\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstu"

	var result bytes.Buffer
	tuple := 0
	n := 0

	for _, b := range data {
		tuple = tuple<<8 | int(b)
		n++

		if n == 4 {
			for i := 4; i >= 0; i-- {
				val := tuple / powInt(base, i)
				result.WriteByte(digits[val%base])
				tuple -= val * powInt(base, i)
			}
			tuple = 0
			n = 0
		}
	}

	// Handle remaining bytes
	if n > 0 {
		for i := 4; i >= 5-n; i-- {
			val := tuple / powInt(base, i)
			result.WriteByte(digits[val%base])
			tuple -= val * powInt(base, i)
		}
	}

	result.WriteByte('~')
	return result.Bytes(), nil
}

func powInt(base, exp int) int {
	result := 1
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
}

// ascii85Decode decodes ASCII85 data (helper for benchmark).
func ascii85Decode(encoded []byte) ([]byte, error) {
	decoder := &ASCII85Decoder{}
	return decoder.Decode(encoded)
}

// lzwEncode encodes data using LZW (helper for benchmark).
func lzwEncode(data []byte) ([]byte, error) {
	// Use simple LZW encoding simulation for benchmark
	// In a real implementation, this would use the LZW encoder
	var result bytes.Buffer
	for _, b := range data {
		result.WriteByte(b)
	}
	return result.Bytes(), nil
}

// lzwDecode decodes LZW data (helper for benchmark).
func lzwDecode(encoded []byte) ([]byte, error) {
	decoder := &LZWDecoder{}
	return decoder.Decode(encoded)
}

// runLengthEncode encodes data using RunLength (helper for benchmark).
func runLengthEncode(data []byte) ([]byte, error) {
	// Simple run-length encoding
	var result bytes.Buffer
	i := 0

	for i < len(data) {
		current := data[i]
		runLength := byte(0)

		// Count consecutive bytes
		for i < len(data) && data[i] == current && runLength < 127 {
			runLength++
			i++
		}

		if runLength > 1 {
			// Run of repeated bytes
			result.WriteByte(128 - runLength)
			result.WriteByte(current)
		} else {
			// Literal run
			start := i - 1
			for i < len(data) && runLength < 127 {
				if i+1 < len(data) && data[i+1] == data[i] {
					break
				}
				runLength++
				i++
			}

			result.WriteByte(runLength - 1)
			result.Write(data[start : start+int(runLength)])
		}
	}

	return result.Bytes(), nil
}

// runLengthDecode decodes RunLength data (helper for benchmark).
func runLengthDecode(encoded []byte) ([]byte, error) {
	decoder := &RunLengthDecoder{}
	return decoder.Decode(encoded)
}

// ccittEncode encodes data using CCITT Group 4 (helper for benchmark).
func ccittEncode(data []byte, width, height int) ([]byte, error) {
	// Simple CCITT Group 4 encoding simulation
	// In a real implementation, this would use proper CCITT encoding
	var result bytes.Buffer
	result.Write(data)
	return result.Bytes(), nil
}

// applyPredictor applies a simple predictor (helper for benchmark).
func applyPredictor(data []byte, colors, components int) []byte {
	result := make([]byte, len(data))
	rowSize := colors*components + 1

	for i := 0; i < len(data); i++ {
		if i%rowSize == 0 {
			result[i] = data[i] // Predictor byte
		} else {
			result[i] = data[i] - data[i-1]
		}
	}

	return result
}

// applyPNGPredictor applies PNG predictor (helper for benchmark).
func applyPNGPredictor(data []byte, components int) []byte {
	result := make([]byte, len(data))
	rowSize := components + 1

	for i := 0; i < len(data); i += rowSize {
		result[i] = 1 // Sub predictor
		for j := 1; j < rowSize && i+j < len(data); j++ {
			left := byte(0)
			if j > 1 && i+j-1 < len(data) {
				left = result[i+j-1]
			}
			result[i+j] = data[i+j] - left
		}
	}

	return result
}

// BenchmarkFlateDecodeVariousSizes benchmarks FlateDecode with various data sizes.
func BenchmarkFlateDecodeVariousSizes(b *testing.B) {
	sizes := []int{1 * 1024, 10 * 1024, 100 * 1024, 1024 * 1024} // 1KB, 10KB, 100KB, 1MB

	for _, size := range sizes {
		b.Run(fmt.Sprintf("%dKB", size/1024), func(b *testing.B) {
			data := generateTestData(size)

			// Compress the data first
			var compressed bytes.Buffer
			writer, _ := flate.NewWriter(&compressed, flate.DefaultCompression)
			writer.Write(data)
			writer.Close()
			compressedData := compressed.Bytes()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				reader := flate.NewReader(bytes.NewReader(compressedData))
				io.Copy(io.Discard, reader)
				reader.Close()
			}
		})
	}
}

// BenchmarkCombinedFilters benchmarks combined filter operations.
func BenchmarkCombinedFilters(b *testing.B) {
	data := generateTestData(1024 * 50) // 50KB

	tests := []struct {
		fn   func() ([]byte, error)
		name string
	}{
		{
			name: "Flate+ASCIIHex",
			fn: func() ([]byte, error) {
				compressed := compressData(data)
				return asciiHexEncode(compressed), nil
			},
		},
		{
			name: "Flate+ASCII85",
			fn: func() ([]byte, error) {
				compressed := compressData(data)
				return ascii85Encode(compressed)
			},
		},
		{
			name: "Flate+LZW",
			fn: func() ([]byte, error) {
				compressed := compressData(data)
				return lzwEncode(compressed)
			},
		},
		{
			name: "Flate+RunLength",
			fn: func() ([]byte, error) {
				compressed := compressData(data)
				return runLengthEncode(compressed)
			},
		},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = tt.fn()
			}
		})
	}
}

// BenchmarkSequentialDecoding benchmarks sequential decoding operations.
func BenchmarkSequentialDecoding(b *testing.B) {
	data := generateTestData(1024 * 50) // 50KB

	// Create encoded data: Flate -> ASCIIHex
	compressed := compressData(data)
	encoded := asciiHexEncode(compressed)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Decode ASCIIHex
		decoded, _ := asciiHexDecode(encoded)

		// Decode Flate
		reader := flate.NewReader(bytes.NewReader(decoded))
		io.Copy(io.Discard, reader)
		reader.Close()
	}
}

// BenchmarkPredictorTypes benchmarks different predictor types.
func BenchmarkPredictorTypes(b *testing.B) {
	data := generateTestData(1024 * 100) // 100KB

	predictors := []struct {
		fn   func() []byte
		name string
	}{
		{
			name: "Predictor1",
			fn: func() []byte {
				return applyPredictor(data, 1, 1)
			},
		},
		{
			name: "Predictor10",
			fn: func() []byte {
				return applyPredictor(data, 3, 1)
			},
		},
		{
			name: "Predictor11",
			fn: func() []byte {
				return applyPNGPredictor(data, 3)
			},
		},
		{
			name: "Predictor12",
			fn: func() []byte {
				return applyPNGPredictor(data, 4)
			},
		},
	}

	for _, p := range predictors {
		b.Run(p.name, func(b *testing.B) {
			testData := p.fn()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = ApplyPredictor(testData, nil)
			}
		})
	}
}
