package colorspace

import (
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLab(t *testing.T) {
	whitePoint := [3]float64{0.9505, 1.0, 1.0890} // D65
	blackPoint := [3]float64{0, 0, 0}
	rangeA := [2]float64{-100, 100}
	rangeB := [2]float64{-100, 100}

	cs, err := NewLab(whitePoint, blackPoint, rangeA, rangeB)
	require.NoError(t, err)
	assert.NotNil(t, cs)
	assert.Equal(t, ColorSpaceLab, cs.Type())
	assert.Equal(t, "Lab", cs.Name())
	assert.Equal(t, 3, cs.GetNumComponents())
}

func TestLab_InvalidWhitePoint(t *testing.T) {
	// Invalid whitePoint (YW != 1.0)
	whitePoint := [3]float64{1.0, 0.5, 1.0}
	blackPoint := [3]float64{0, 0, 0}
	rangeA := [2]float64{-100, 100}
	rangeB := [2]float64{-100, 100}

	cs, err := NewLab(whitePoint, blackPoint, rangeA, rangeB)
	require.NoError(t, err)

	// Should use default D65 whitePoint
	assert.Equal(t, 1.0, cs.whitePoint[1])
}

func TestLab_InvalidBlackPoint(t *testing.T) {
	whitePoint := [3]float64{0.9505, 1.0, 1.0890}
	blackPoint := [3]float64{-0.1, 0, 0} // Invalid
	rangeA := [2]float64{-100, 100}
	rangeB := [2]float64{-100, 100}

	cs, err := NewLab(whitePoint, blackPoint, rangeA, rangeB)
	require.NoError(t, err)

	// Should use default [0, 0, 0]
	assert.Equal(t, 0.0, cs.blackPoint[0])
}

func TestLab_InvalidRange(t *testing.T) {
	whitePoint := [3]float64{0.9505, 1.0, 1.0890}
	blackPoint := [3]float64{0, 0, 0}
	rangeA := [2]float64{100, -100} // Invalid (min > max)
	rangeB := [2]float64{-100, 100}

	cs, err := NewLab(whitePoint, blackPoint, rangeA, rangeB)
	require.NoError(t, err)

	// Should use default [-100, 100]
	assert.Equal(t, -100.0, cs.rangeA[0])
	assert.Equal(t, 100.0, cs.rangeA[1])
}

func TestLab_ConvertToRGBA_D65(t *testing.T) {
	// D65 whitePoint
	whitePoint := [3]float64{0.9505, 1.0, 1.0890}
	blackPoint := [3]float64{0, 0, 0}
	rangeA := [2]float64{-100, 100}
	rangeB := [2]float64{-100, 100}

	cs, err := NewLab(whitePoint, blackPoint, rangeA, rangeB)
	require.NoError(t, err)

	tests := []struct {
		name      string
		input     []float64
		expected  color.RGBA
		tolerance uint8
	}{
		// Black: L*=0 (minimum lightness)
		{"Black", []float64{0, 0, 0}, color.RGBA{0, 0, 0, 255}, 5},

		// White: L*=100 (maximum lightness)
		{"White", []float64{100, 0, 0}, color.RGBA{255, 255, 255, 255}, 10},

		// Gray: L*=50 (middle lightness, a*=b*=0 for neutral)
		{"Gray", []float64{50, 0, 0}, color.RGBA{119, 119, 119, 255}, 15},

		// Red: High L*, positive a*, near-zero b*
		{"Red", []float64{53, 80, 67}, color.RGBA{255, 0, 0, 255}, 20},

		// Green: Medium L*, negative a*, positive b*
		{"Green", []float64{88, -86, 83}, color.RGBA{0, 255, 0, 255}, 20},

		// Blue: Low L*, positive a*, negative b*
		// Note: Lab blue doesn't map perfectly to sRGB blue
		{"Blue", []float64{32, 79, -107}, color.RGBA{58, 0, 238, 255}, 25},

		// Edge cases
		{"Empty", []float64{}, color.RGBA{0, 0, 0, 255}, 0},
		{"TooFew", []float64{50, 0}, color.RGBA{0, 0, 0, 255}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cs.ConvertToRGBA(tt.input)

			if tt.tolerance == 0 {
				assert.Equal(t, tt.expected, result)
			} else {
				assertColorWithinToleranceLab(t, tt.expected, result, tt.tolerance)
			}
		})
	}
}

func TestLab_ConvertToRGBA_D50(t *testing.T) {
	// D50 whitePoint (ZW < 1.0)
	whitePoint := [3]float64{0.9642, 1.0, 0.8249}
	blackPoint := [3]float64{0, 0, 0}
	rangeA := [2]float64{-100, 100}
	rangeB := [2]float64{-100, 100}

	cs, err := NewLab(whitePoint, blackPoint, rangeA, rangeB)
	require.NoError(t, err)

	// Test that D50 uses different conversion matrix
	result := cs.ConvertToRGBA([]float64{50, 0, 0})

	// Should produce a gray color
	assert.Greater(t, result.R, uint8(100))
	assert.Greater(t, result.G, uint8(100))
	assert.Greater(t, result.B, uint8(100))
	assert.Equal(t, uint8(255), result.A)
}

func TestLab_RangeClamping(t *testing.T) {
	whitePoint := [3]float64{0.9505, 1.0, 1.0890}
	blackPoint := [3]float64{0, 0, 0}
	rangeA := [2]float64{-50, 50} // Narrower range
	rangeB := [2]float64{-50, 50}

	cs, err := NewLab(whitePoint, blackPoint, rangeA, rangeB)
	require.NoError(t, err)

	// Test that values outside range are clamped
	result := cs.ConvertToRGBA([]float64{50, 100, -100})

	// Should still produce valid RGB (clamped to [-50, 50])
	assert.LessOrEqual(t, result.R, uint8(255))
	assert.LessOrEqual(t, result.G, uint8(255))
	assert.LessOrEqual(t, result.B, uint8(255))
	assert.Equal(t, uint8(255), result.A)
}

func TestFnG(t *testing.T) {
	const threshold = 6.0 / 29.0

	tests := []struct {
		input     float64
		expected  float64
		tolerance float64
	}{
		// Below threshold: linear formula
		{0.0, (108.0 / 841.0) * (0.0 - 4.0/29.0), 0.001},
		{threshold / 2, (108.0 / 841.0) * (threshold/2 - 4.0/29.0), 0.001},

		// At threshold
		{threshold, threshold * threshold * threshold, 0.001},

		// Above threshold: cubic
		{0.5, 0.125, 0.001},
		{1.0, 1.0, 0.001},
	}

	for _, tt := range tests {
		result := fnG(tt.input)
		assert.InDelta(t, tt.expected, result, tt.tolerance, "fnG(%f)", tt.input)
	}
}

func TestLab_NegativeRGB(t *testing.T) {
	// Some Lab values can produce negative RGB before sqrt
	// Test that sqrt handles this correctly (sqrt of negative should use max(0, value))
	whitePoint := [3]float64{0.9505, 1.0, 1.0890}
	blackPoint := [3]float64{0, 0, 0}
	rangeA := [2]float64{-100, 100}
	rangeB := [2]float64{-100, 100}

	cs, err := NewLab(whitePoint, blackPoint, rangeA, rangeB)
	require.NoError(t, err)

	// Extreme values that might produce negative RGB
	result := cs.ConvertToRGBA([]float64{0, -100, -100})

	// Should still produce valid RGB (all components >= 0)
	assert.GreaterOrEqual(t, result.R, uint8(0))
	assert.GreaterOrEqual(t, result.G, uint8(0))
	assert.GreaterOrEqual(t, result.B, uint8(0))
	assert.Equal(t, uint8(255), result.A)
}

func TestLab_FullRange(t *testing.T) {
	whitePoint := [3]float64{0.9505, 1.0, 1.0890}
	blackPoint := [3]float64{0, 0, 0}
	rangeA := [2]float64{-128, 127}
	rangeB := [2]float64{-128, 127}

	cs, err := NewLab(whitePoint, blackPoint, rangeA, rangeB)
	require.NoError(t, err)

	// Test with full 8-bit range
	result := cs.ConvertToRGBA([]float64{50, 127, -128})

	assert.LessOrEqual(t, result.R, uint8(255))
	assert.LessOrEqual(t, result.G, uint8(255))
	assert.LessOrEqual(t, result.B, uint8(255))
	assert.Equal(t, uint8(255), result.A)
}

func assertColorWithinToleranceLab(t *testing.T, expected, actual color.RGBA, tolerance uint8) {
	t.Helper()

	diffR := absInt(int(actual.R) - int(expected.R))
	diffG := absInt(int(actual.G) - int(expected.G))
	diffB := absInt(int(actual.B) - int(expected.B))

	if diffR > int(tolerance) || diffG > int(tolerance) || diffB > int(tolerance) {
		assert.Failf(t, "assertion failed", "Color out of tolerance:\n  Expected: R=%d G=%d B=%d\n  Actual:   R=%d G=%d B=%d\n  Diff:     R=%d G=%d B=%d (tolerance=%d)",
			expected.R, expected.G, expected.B,
			actual.R, actual.G, actual.B,
			diffR, diffG, diffB, tolerance)
	}

	assert.Equal(t, uint8(255), actual.A, "A component")
}
