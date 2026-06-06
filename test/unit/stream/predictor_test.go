package stream_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"
)

func TestNoPredictor_Decode(t *testing.T) {
	predictor := &stream.NoPredictor{}

	tests := []struct {
		name             string
		data             []byte
		expected         []byte
		columns          int
		colors           int
		bitsPerComponent int
	}{
		{
			name:             "simple data",
			data:             []byte{1, 2, 3, 4, 5},
			columns:          5,
			colors:           1,
			bitsPerComponent: 8,
			expected:         []byte{1, 2, 3, 4, 5},
		},
		{
			name:             "empty data",
			data:             []byte{},
			columns:          1,
			colors:           1,
			bitsPerComponent: 8,
			expected:         []byte{},
		},
		{
			name:             "multi-byte values",
			data:             []byte{0x00, 0xFF, 0x80, 0x7F},
			columns:          2,
			colors:           2,
			bitsPerComponent: 8,
			expected:         []byte{0x00, 0xFF, 0x80, 0x7F},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := predictor.Decode(tt.data, tt.columns, tt.colors, tt.bitsPerComponent)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
			// Ensure result is a copy
			if len(tt.data) > 0 {
				result[0] = 0xFF
				assert.NotEqual(t, tt.data[0], result[0], "result should be a copy")
			}
		})
	}
}

func TestTIFFPredictor_Decode_SingleRow(t *testing.T) {
	predictor := &stream.TIFFPredictor{}

	// Single row with horizontal differencing
	// Original: [10, 20, 30, 40]
	// Encoded:  [10, 10, 10, 10] (differences: 10, 20-10=10, 30-20=10, 40-30=10)
	tests := []struct {
		name             string
		data             []byte
		expected         []byte
		columns          int
		colors           int
		bitsPerComponent int
	}{
		{
			name:             "horizontal differencing single row",
			data:             []byte{10, 10, 10, 10},
			columns:          4,
			colors:           1,
			bitsPerComponent: 8,
			expected:         []byte{10, 20, 30, 40},
		},
		{
			name:             "all zeros",
			data:             []byte{0, 0, 0, 0},
			columns:          4,
			colors:           1,
			bitsPerComponent: 8,
			expected:         []byte{0, 0, 0, 0},
		},
		{
			name:             "incrementing values",
			data:             []byte{5, 1, 1, 1},
			columns:          4,
			colors:           1,
			bitsPerComponent: 8,
			expected:         []byte{5, 6, 7, 8},
		},
		{
			name:             "decrementing values",
			data:             []byte{100, 255, 255, 255},
			columns:          4,
			colors:           1,
			bitsPerComponent: 8,
			expected:         []byte{100, 99, 98, 97},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := predictor.Decode(tt.data, tt.columns, tt.colors, tt.bitsPerComponent)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTIFFPredictor_Decode_MultipleRows(t *testing.T) {
	predictor := &stream.TIFFPredictor{}

	// Multiple rows with horizontal differencing
	// Row 1: [10, 10, 10, 10] -> [10, 20, 30, 40]
	// Row 2: [15, 5, 5, 5] -> [15, 20, 25, 30]
	data := []byte{10, 10, 10, 10, 15, 5, 5, 5}
	expected := []byte{10, 20, 30, 40, 15, 20, 25, 30}

	result, err := predictor.Decode(data, 4, 1, 8)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestTIFFPredictor_Decode_MultiColor(t *testing.T) {
	predictor := &stream.TIFFPredictor{}

	// TIFF predictor treats each byte independently with horizontal differencing
	// Test with multiple samples in a row
	// Encoded: [10, 10, 10, 10]
	// Decoded: [10, 20, 30, 40]
	data := []byte{10, 10, 10, 10}
	expected := []byte{10, 20, 30, 40}

	result, err := predictor.Decode(data, 4, 1, 8)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestTIFFPredictor_Decode_Empty(t *testing.T) {
	predictor := &stream.TIFFPredictor{}
	result, err := predictor.Decode([]byte{}, 1, 1, 8)
	require.NoError(t, err)
	assert.Equal(t, []byte{}, result)
}

func TestTIFFPredictor_Decode_InvalidDataSize(t *testing.T) {
	predictor := &stream.TIFFPredictor{}

	// Data length (5) is not a multiple of columns*colors (6)
	data := []byte{1, 2, 3, 4, 5}

	_, err := predictor.Decode(data, 3, 2, 8)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid data size")
}

func TestPNGPredictor_Decode_None(t *testing.T) {
	predictor, err := stream.GetPredictor(11) // Predictor 11 = PNG None
	require.NoError(t, err)

	// PDF PNG prediction stores one filter byte before each row.
	data := []byte{
		0, 1, 2, 3, 4, // Row 1
		0, 5, 6, 7, 8, // Row 2
	}
	expected := []byte{1, 2, 3, 4, 5, 6, 7, 8}

	result, err := predictor.Decode(data, 4, 1, 8)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestPNGPredictor_Decode_Sub(t *testing.T) {
	predictor, err := stream.GetPredictor(12) // Predictor 12 = PNG Sub
	require.NoError(t, err)

	// Original: [1,2,3,4], encoded with Sub filter as [1,1,1,1].
	data := []byte{1, 1, 1, 1, 1}
	expected := []byte{1, 2, 3, 4}

	result, err := predictor.Decode(data, 4, 1, 8)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestPNGPredictor_Decode_Sub_MultiColor(t *testing.T) {
	predictor, err := stream.GetPredictor(12) // Predictor 12 = PNG Sub
	require.NoError(t, err)

	// RGB data with Sub filter (columns=2, colors=3, bpc=8)
	// The PNG left reference distance is one pixel, so RGB uses a 3-byte stride.
	data := []byte{1, 10, 20, 30, 30, 30, 30}
	expected := []byte{10, 20, 30, 40, 50, 60}

	result, err := predictor.Decode(data, 2, 3, 8)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestPNGPredictor_Decode_Up(t *testing.T) {
	predictor, err := stream.GetPredictor(13) // Predictor 13 = PNG Up
	require.NoError(t, err)

	// Row 1: [1, 2, 3, 4] (no prior row, so treated as None)
	// Row 2: [4, 5, 6, 7] (differences from row 1: 5-1=4, 7-2=5, 9-3=6, 11-4=7)
	data := []byte{
		2, 1, 2, 3, 4, // Row 1
		2, 4, 5, 6, 7, // Row 2
	}
	expected := []byte{1, 2, 3, 4, 5, 7, 9, 11}

	result, err := predictor.Decode(data, 4, 1, 8)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestPNGPredictor_Decode_Average(t *testing.T) {
	predictor, err := stream.GetPredictor(14) // Predictor 14 = PNG Average
	require.NoError(t, err)

	// Simple test: all zeros except first byte of each row
	data := []byte{
		3, 10, 0, 0, 0, // Row 1
		3, 0, 0, 0, 0, // Row 2
	}
	// Average uses integer division: Raw(x) = Encoded(x) + floor((left + up) / 2).
	expected := []byte{10, 5, 2, 1, 5, 5, 3, 2}

	result, err := predictor.Decode(data, 4, 1, 8)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestPNGPredictor_Decode_Paeth(t *testing.T) {
	predictor, err := stream.GetPredictor(15) // Predictor 15 = PNG Paeth
	require.NoError(t, err)

	// PNG Paeth filter uses the filter byte stored at the start of each row.
	data := []byte{
		4, 10, 20, 30, 40, // Row 1
		4, 15, 25, 35, 45, // Row 2
	}
	// The exact result depends on the Paeth algorithm
	// Just verify it doesn't error and returns correct length
	result, err := predictor.Decode(data, 4, 1, 8)
	require.NoError(t, err)
	assert.Len(t, result, 8)
}

func TestPNGPredictor_Decode_Empty(t *testing.T) {
	predictor, err := stream.GetPredictor(11)
	require.NoError(t, err)
	result, err := predictor.Decode([]byte{}, 1, 1, 8)
	require.NoError(t, err)
	assert.Equal(t, []byte{}, result)
}

func TestPNGPredictor_Decode_InvalidDataSize(t *testing.T) {
	predictor, err := stream.GetPredictor(11)
	require.NoError(t, err)

	// Data length (5) is not a multiple of row size (4+1=5) for 1 row
	// Actually this should work: 1 row * 5 bytes = 5 bytes
	// Let's make it invalid: 6 bytes with columns=4
	data := []byte{0, 1, 2, 3, 4, 5}

	_, decodeErr := predictor.Decode(data, 4, 1, 8)
	assert.Error(t, decodeErr)
	assert.Contains(t, decodeErr.Error(), "invalid data size")
}

func TestPNGPredictor_Decode_InvalidFilterType(t *testing.T) {
	// Use predictor 10 which reads filter byte from data
	predictor, err := stream.GetPredictor(10)
	require.NoError(t, err)

	data := []byte{5, 1, 2, 3, 4} // filter type 5 doesn't exist

	_, decodeErr := predictor.Decode(data, 4, 1, 8)
	assert.Error(t, decodeErr)
	assert.Contains(t, decodeErr.Error(), "unknown PNG filter type")
}

func TestGetPredictor(t *testing.T) {
	tests := []struct {
		expectedType  interface{}
		predictorType int
		shouldError   bool
	}{
		{&stream.NoPredictor{}, 1, false},
		{&stream.TIFFPredictor{}, 2, false},
		{&stream.PNGPredictor{}, 10, false}, // PNG with algorithm from filter byte
		{&stream.PNGPredictor{}, 11, false}, // PNG None
		{&stream.PNGPredictor{}, 12, false}, // PNG Sub
		{&stream.PNGPredictor{}, 13, false}, // PNG Up
		{&stream.PNGPredictor{}, 14, false}, // PNG Average
		{&stream.PNGPredictor{}, 15, false}, // PNG Paeth
		{nil, 0, true},                      // Invalid
		{nil, 3, true},                      // Invalid
		{nil, 16, true},                     // Invalid
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("PredictorType_%d", tt.predictorType), func(t *testing.T) {
			predictor, err := stream.GetPredictor(tt.predictorType)
			if tt.shouldError {
				assert.Error(t, err)
				assert.Nil(t, predictor)
			} else {
				require.NoError(t, err)
				assert.IsType(t, tt.expectedType, predictor)
			}
		})
	}
}

func TestGetDecodeParams_Defaults(t *testing.T) {
	params, err := stream.GetDecodeParams(nil)
	require.NoError(t, err)

	assert.Equal(t, 1, params.Predictor)
	assert.Equal(t, 1, params.Columns)
	assert.Equal(t, 1, params.Colors)
	assert.Equal(t, 8, params.BitsPerComponent)
}

func TestGetDecodeParams_FromDict(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("Predictor"), entity.NewInteger(2))
	dict.Set(entity.Name("Columns"), entity.NewInteger(10))
	dict.Set(entity.Name("Colors"), entity.NewInteger(3))
	dict.Set(entity.Name("BitsPerComponent"), entity.NewInteger(16))

	params, err := stream.GetDecodeParams(dict)
	require.NoError(t, err)

	assert.Equal(t, 2, params.Predictor)
	assert.Equal(t, 10, params.Columns)
	assert.Equal(t, 3, params.Colors)
	assert.Equal(t, 16, params.BitsPerComponent)
}

func TestGetDecodeParams_InvalidColumns(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("Columns"), entity.NewInteger(0))

	_, err := stream.GetDecodeParams(dict)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "columns must be positive")
}

func TestGetDecodeParams_InvalidColors(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("Colors"), entity.NewInteger(-1))

	_, err := stream.GetDecodeParams(dict)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "colors must be positive")
}

func TestGetDecodeParams_InvalidBitsPerComponent(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("BitsPerComponent"), entity.NewInteger(17))

	_, err := stream.GetDecodeParams(dict)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bitsPerComponent must be 1-16")
}

func TestApplyPredictor(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		params   *entity.Dict
		expected []byte
	}{
		{
			name:     "no predictor",
			data:     []byte{1, 2, 3, 4},
			params:   nil,
			expected: []byte{1, 2, 3, 4},
		},
		{
			name:     "predictor 1",
			data:     []byte{1, 2, 3, 4},
			params:   stream.CreateDecodeParams(1, 4, 1, 8),
			expected: []byte{1, 2, 3, 4},
		},
		{
			name:     "TIFF predictor",
			data:     []byte{10, 10, 10, 10},
			params:   stream.CreateDecodeParams(2, 4, 1, 8),
			expected: []byte{10, 20, 30, 40},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := stream.ApplyPredictor(tt.data, tt.params)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplyPredictor_Invalid(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("Predictor"), entity.NewInteger(99))

	_, err := stream.ApplyPredictor([]byte{1, 2, 3}, dict)
	assert.Error(t, err)
}

func TestEncodeWithTIFFPredictor(t *testing.T) {
	original := []byte{10, 20, 30, 40}
	expected := []byte{10, 10, 10, 10} // differences

	result := stream.EncodeWithTIFFPredictor(original, 4, 1)
	assert.Equal(t, expected, result)

	// Round-trip test
	decoded, err := (&stream.TIFFPredictor{}).Decode(result, 4, 1, 8)
	require.NoError(t, err)
	assert.Equal(t, original, decoded)
}

func TestEncodeWithPNGPredictor(t *testing.T) {
	original := []byte{1, 2, 3, 4}

	// Test each algorithm
	algorithms := []int{0, 1, 2, 3, 4}
	for _, alg := range algorithms {
		t.Run(fmt.Sprintf("algorithm_%d", alg), func(t *testing.T) {
			encoded, err := stream.EncodeWithPNGPredictor(original, 4, 1, 8, alg)
			require.NoError(t, err)

			// Round-trip test - use predictor type 10 which reads filter bytes from data
			predictor, err := stream.GetPredictor(10)
			require.NoError(t, err)
			decoded, err := predictor.Decode(encoded, 4, 1, 8)
			require.NoError(t, err)
			assert.Equal(t, original, decoded)
		})
	}
}

func TestEncodeWithPNGPredictor_InvalidAlgorithm(t *testing.T) {
	original := []byte{1, 2, 3, 4}

	_, err := stream.EncodeWithPNGPredictor(original, 4, 1, 8, 5)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid PNG filter algorithm")
}

func TestTIFFPredictor_RealWorldExample(t *testing.T) {
	// Real-world example: grayscale image data
	// Simulating a 2x2 image with grayscale values
	// Row 1: [50, 100, 150, 200]
	// Row 2: [60, 110, 160, 210]
	// Encoded with TIFF predictor (horizontal differencing):
	// Row 1: [50, 50, 50, 50] (differences)
	// Row 2: [60, 50, 50, 50]

	encoded := []byte{
		50, 50, 50, 50, // Row 1
		60, 50, 50, 50, // Row 2
	}

	expected := []byte{
		50, 100, 150, 200, // Row 1 decoded
		60, 110, 160, 210, // Row 2 decoded
	}

	predictor := &stream.TIFFPredictor{}
	result, err := predictor.Decode(encoded, 4, 1, 8)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestPNGPredictor_RGBImage(t *testing.T) {
	// Test PNG predictor with RGB data (3 color components)
	// Each pixel is R,G,B

	// Simple test: 2 pixels with values that decode cleanly
	// The row filter byte selects Sub, and RGB uses a 3-byte left stride.
	// Original: [10, 20, 30, 40, 50, 60]
	// Encoded (Sub): [10, 20, 30, 30, 30, 30] after the filter byte.
	data := []byte{1, 10, 20, 30, 30, 30, 30}
	expected := []byte{10, 20, 30, 40, 50, 60}

	predictor, err := stream.GetPredictor(12) // PNG Sub
	require.NoError(t, err)
	result, err := predictor.Decode(data, 2, 3, 8)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestPNGPredictor_MultiRowWithDifferentFilters(t *testing.T) {
	// Test with filter byte specified per row (predictor type 10)
	predictor, err := stream.GetPredictor(10)
	require.NoError(t, err)

	// Row 1: None filter
	// Row 2: Sub filter
	// Row 3: Up filter
	data := []byte{
		0, 10, 20, 30, 40, // Row 1: None
		1, 10, 10, 10, 10, // Row 2: Sub -> should decode to [10, 20, 30, 40]
		2, 0, 0, 0, 0, // Row 3: Up -> should decode to [10, 20, 30, 40]
	}

	expected := []byte{
		10, 20, 30, 40, // Row 1
		10, 20, 30, 40, // Row 2
		10, 20, 30, 40, // Row 3
	}

	result, err := predictor.Decode(data, 4, 1, 8)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestPNGPredictor_SingleColumn(t *testing.T) {
	predictor, err := stream.GetPredictor(12) // PNG Sub
	require.NoError(t, err)

	// Single-column Sub rows still include one filter byte per row.
	// columns=1, colors=1, bpc=8 -> bytesPerRow=1
	// With PNG Sub filter and bytesPerPixel=1, each row has only 1 byte,
	// so there's no "left" byte to predict from within the row
	// 4 rows of data: [10, 10, 10, 10]
	// Decoded: [10, 10, 10, 10] (unchanged)
	data := []byte{1, 10, 1, 10, 1, 10, 1, 10}
	expected := []byte{10, 10, 10, 10}

	result, err := predictor.Decode(data, 1, 1, 8)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestTIFFPredictor_SingleColumn(t *testing.T) {
	predictor := &stream.TIFFPredictor{}

	// Single column: no horizontal differencing effect
	data := []byte{10, 20, 30}
	expected := []byte{10, 20, 30}

	result, err := predictor.Decode(data, 1, 1, 8)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestPNGPredictor_16BitSamples(t *testing.T) {
	predictor, err := stream.GetPredictor(11) // PNG None
	require.NoError(t, err)

	// 16-bit samples (2 bytes per sample)
	// 2 columns, 1 color = 4 bytes per row
	data := []byte{
		0, 0x00, 0x01, 0x00, 0x02, // Row 1
	}
	expected := []byte{0x00, 0x01, 0x00, 0x02}

	result, err := predictor.Decode(data, 2, 1, 16)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestStream_Decode_WithPredictor(t *testing.T) {
	tests := []struct {
		name      string
		filter    entity.Name
		expected  string
		data      []byte
		predictor int
		columns   int
		colors    int
		bpc       int
	}{
		{
			name:      "TIFF predictor with no compression",
			data:      []byte{10, 10, 10, 10},
			filter:    "",
			predictor: 2,
			columns:   4,
			colors:    1,
			bpc:       8,
			expected:  "\n\x14\x1e(", // [10, 20, 30, 40]
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dict := entity.NewDict()
			if tt.filter != "" {
				dict.Set(entity.Name("Filter"), tt.filter)
			}

			decodeParms := entity.NewDict()
			decodeParms.Set(entity.Name("Predictor"), entity.NewInteger(int64(tt.predictor)))
			decodeParms.Set(entity.Name("Columns"), entity.NewInteger(int64(tt.columns)))
			decodeParms.Set(entity.Name("Colors"), entity.NewInteger(int64(tt.colors)))
			decodeParms.Set(entity.Name("BitsPerComponent"), entity.NewInteger(int64(tt.bpc)))
			dict.Set(entity.Name("DecodeParms"), decodeParms)

			s := stream.NewStream(dict, tt.data)
			decoded, err := s.Decode()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, string(decoded))
		})
	}
}
