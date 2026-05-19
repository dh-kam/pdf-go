package colorspace

import (
	"image/color"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCalGray(t *testing.T) {
	// D65 whitePoint
	whitePoint := [3]float64{0.9505, 1.0, 1.0890}
	blackPoint := [3]float64{0, 0, 0}
	gamma := 2.2

	cs, err := NewCalGray(whitePoint, blackPoint, gamma)
	require.NoError(t, err)
	assert.NotNil(t, cs)
	assert.Equal(t, ColorSpaceCalGray, cs.Type())
	assert.Equal(t, "CalGray", cs.Name())
	assert.Equal(t, 1, cs.GetNumComponents())
}

func TestCalGray_InvalidWhitePoint(t *testing.T) {
	// Invalid whitePoint (YW != 1.0) should fallback to default
	whitePoint := [3]float64{1.0, 0.5, 1.0}
	blackPoint := [3]float64{0, 0, 0}
	gamma := 1.0

	cs, err := NewCalGray(whitePoint, blackPoint, gamma)
	require.NoError(t, err)

	// Should use default D65 whitePoint
	assert.Equal(t, 1.0, cs.whitePoint[1])
}

func TestCalGray_InvalidBlackPoint(t *testing.T) {
	whitePoint := [3]float64{0.9505, 1.0, 1.0890}
	blackPoint := [3]float64{-0.1, 0, 0} // Invalid (negative)
	gamma := 1.0

	cs, err := NewCalGray(whitePoint, blackPoint, gamma)
	require.NoError(t, err)

	// Should use default [0, 0, 0]
	assert.Equal(t, 0.0, cs.blackPoint[0])
	assert.Equal(t, 0.0, cs.blackPoint[1])
	assert.Equal(t, 0.0, cs.blackPoint[2])
}

func TestCalGray_InvalidGamma(t *testing.T) {
	whitePoint := [3]float64{0.9505, 1.0, 1.0890}
	blackPoint := [3]float64{0, 0, 0}
	gamma := 0.5 // Invalid (< 1.0)

	cs, err := NewCalGray(whitePoint, blackPoint, gamma)
	require.NoError(t, err)

	// Should fallback to 1.0
	assert.Equal(t, 1.0, cs.gamma)
}

func TestCalGray_ConvertToRGBA(t *testing.T) {
	whitePoint := [3]float64{0.9505, 1.0, 1.0890}
	blackPoint := [3]float64{0, 0, 0}
	gamma := 1.0

	cs, err := NewCalGray(whitePoint, blackPoint, gamma)
	require.NoError(t, err)

	tests := []struct {
		name      string
		input     []float64
		expected  color.RGBA
		tolerance uint8
	}{
		{"Black", []float64{0.0}, color.RGBA{0, 0, 0, 255}, 5},
		{"White", []float64{1.0}, color.RGBA{255, 255, 255, 255}, 5},
		{"MidGray", []float64{0.5}, color.RGBA{194, 194, 194, 255}, 10},
		{"Empty", []float64{}, color.RGBA{0, 0, 0, 255}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cs.ConvertToRGBA(tt.input)

			if tt.tolerance == 0 {
				assert.Equal(t, tt.expected, result)
			} else {
				assertColorWithinTolerance(t, tt.expected, result, tt.tolerance)
			}
		})
	}
}

func TestCalGray_WithGamma(t *testing.T) {
	whitePoint := [3]float64{0.9505, 1.0, 1.0890}
	blackPoint := [3]float64{0, 0, 0}
	gamma := 2.2

	cs, err := NewCalGray(whitePoint, blackPoint, gamma)
	require.NoError(t, err)

	// With gamma 2.2, midpoint should be darker
	result := cs.ConvertToRGBA([]float64{0.5})

	// Should be darker than linear (194) - around 137
	assert.Less(t, result.R, uint8(194))
	assert.Greater(t, result.R, uint8(120))
	assert.Equal(t, result.R, result.G)
	assert.Equal(t, result.R, result.B)
	assert.Equal(t, uint8(255), result.A)
}

func TestNewCalRGB(t *testing.T) {
	whitePoint := [3]float64{0.9505, 1.0, 1.0890}
	blackPoint := [3]float64{0, 0, 0}
	gamma := [3]float64{2.2, 2.2, 2.2}
	matrix := [9]float64{1, 0, 0, 0, 1, 0, 0, 0, 1} // Identity

	cs, err := NewCalRGB(whitePoint, blackPoint, gamma, matrix)
	require.NoError(t, err)
	assert.NotNil(t, cs)
	assert.Equal(t, ColorSpaceCalRGB, cs.Type())
	assert.Equal(t, "CalRGB", cs.Name())
	assert.Equal(t, 3, cs.GetNumComponents())
}

func TestCalRGB_InvalidParameters(t *testing.T) {
	// Invalid whitePoint
	whitePoint := [3]float64{1.0, 0.5, 1.0}
	blackPoint := [3]float64{0, 0, 0}
	gamma := [3]float64{1, 1, 1}
	matrix := [9]float64{1, 0, 0, 0, 1, 0, 0, 0, 1}

	cs, err := NewCalRGB(whitePoint, blackPoint, gamma, matrix)
	require.NoError(t, err)
	assert.Equal(t, 1.0, cs.whitePoint[1])
}

func TestCalRGB_ConvertToRGBA(t *testing.T) {
	whitePoint := [3]float64{0.9505, 1.0, 1.0890}
	blackPoint := [3]float64{0, 0, 0}
	gamma := [3]float64{1, 1, 1}
	matrix := [9]float64{1, 0, 0, 0, 1, 0, 0, 0, 1}

	cs, err := NewCalRGB(whitePoint, blackPoint, gamma, matrix)
	require.NoError(t, err)

	tests := []struct {
		name      string
		input     []float64
		expected  color.RGBA
		tolerance uint8
	}{
		{"Black", []float64{0.0, 0.0, 0.0}, color.RGBA{0, 0, 0, 255}, 10},
		{"White", []float64{1.0, 1.0, 1.0}, color.RGBA{255, 249, 244, 255}, 15},
		{"Red", []float64{1.0, 0.0, 0.0}, color.RGBA{255, 0, 67, 255}, 15},
		{"Green", []float64{0.0, 1.0, 0.0}, color.RGBA{0, 255, 0, 255}, 15},
		{"Blue", []float64{0.0, 0.0, 1.0}, color.RGBA{0, 57, 255, 255}, 15},
		{"MidGray", []float64{0.5, 0.5, 0.5}, color.RGBA{188, 188, 188, 255}, 16},
		{"Empty", []float64{}, color.RGBA{0, 0, 0, 255}, 0},
		{"TooFew", []float64{1.0, 0.5}, color.RGBA{0, 0, 0, 255}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cs.ConvertToRGBA(tt.input)

			if tt.tolerance == 0 {
				assert.Equal(t, tt.expected, result)
			} else {
				assertColorWithinTolerance(t, tt.expected, result, tt.tolerance)
			}
		})
	}
}

func TestSRGBTransferFunction(t *testing.T) {
	tests := []struct {
		input     float64
		expected  float64
		tolerance float64
	}{
		{0.0, 0.0, 0.001},
		{0.0031308, 0.04045, 0.001},
		{0.5, 0.7353, 0.001},
		{0.99554525, 1.0, 0.001},
		{1.0, 1.0, 0.001},
	}

	for _, tt := range tests {
		result := srgbTransferFunction(tt.input)
		assert.InDelta(t, tt.expected, result, tt.tolerance)
	}
}

func TestMatrixProduct(t *testing.T) {
	// Identity matrix
	identity := [9]float64{1, 0, 0, 0, 1, 0, 0, 0, 1}
	vec := [3]float64{2, 3, 4}

	result := matrixProduct(identity, vec)
	assert.Equal(t, vec, result)

	// 2x scale matrix
	scale := [9]float64{2, 0, 0, 0, 2, 0, 0, 0, 2}
	result = matrixProduct(scale, vec)
	assert.Equal(t, [3]float64{4, 6, 8}, result)
}

func TestDecodeL(t *testing.T) {
	tests := []struct {
		input     float64
		expected  float64
		tolerance float64
	}{
		{0.0, 0.0, 0.001},
		{8.0, 8.0 * decodeLConstant, 0.001},
		{50.0, math.Pow((50.0+16)/116, 3), 0.001},
		{100.0, math.Pow((100.0+16)/116, 3), 0.001},
		{-10.0, -decodeL(10.0), 0.001}, // Negative
	}

	for _, tt := range tests {
		result := decodeL(tt.input)
		assert.InDelta(t, tt.expected, result, tt.tolerance)
	}
}

func assertColorWithinTolerance(t *testing.T, expected, actual color.RGBA, tolerance uint8) {
	t.Helper()

	diffR := absInt(int(actual.R) - int(expected.R))
	diffG := absInt(int(actual.G) - int(expected.G))
	diffB := absInt(int(actual.B) - int(expected.B))

	assert.LessOrEqual(t, diffR, int(tolerance), "R component out of tolerance")
	assert.LessOrEqual(t, diffG, int(tolerance), "G component out of tolerance")
	assert.LessOrEqual(t, diffB, int(tolerance), "B component out of tolerance")
	assert.Equal(t, uint8(255), actual.A, "A component")
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
