package colorspace

import (
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIndexedColorSpace tests
func TestNewIndexedColorSpace(t *testing.T) {
	base := NewDeviceRGB()
	hival := 255
	lookup := make([]byte, (hival+1)*3) // 256 RGB entries

	// Fill lookup with some test data
	for i := 0; i <= hival; i++ {
		lookup[i*3] = byte(i)         // R = index
		lookup[i*3+1] = 0             // G = 0
		lookup[i*3+2] = byte(255 - i) // B = 255-index
	}

	cs := NewIndexedColorSpace(base, hival, lookup)
	require.NotNil(t, cs)
	assert.Equal(t, ColorSpaceIndexed, cs.Type())
	assert.Equal(t, "Indexed", cs.Name())
	assert.Equal(t, 1, cs.GetNumComponents())
}

func TestIndexedColorSpace_WithDeviceGray(t *testing.T) {
	base := NewDeviceGray()
	hival := 15 // 16 gray levels
	lookup := make([]byte, hival+1)

	// Create grayscale palette (0, 17, 34, ..., 255)
	for i := 0; i <= hival; i++ {
		lookup[i] = byte(i * 17)
	}

	cs := NewIndexedColorSpace(base, hival, lookup)

	tests := []struct {
		name     string
		index    float64
		expected color.RGBA
	}{
		{"Black", 0, color.RGBA{0, 0, 0, 255}},
		{"White", 15, color.RGBA{255, 255, 255, 255}},
		{"MidGray", 8, color.RGBA{136, 136, 136, 255}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cs.ConvertToRGBA([]float64{tt.index})
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIndexedColorSpace_WithDeviceRGB(t *testing.T) {
	base := NewDeviceRGB()
	hival := 7 // 8 colors
	lookup := []byte{
		// Index 0: Black
		0, 0, 0,
		// Index 1: Red
		255, 0, 0,
		// Index 2: Green
		0, 255, 0,
		// Index 3: Blue
		0, 0, 255,
		// Index 4: Yellow
		255, 255, 0,
		// Index 5: Cyan
		0, 255, 255,
		// Index 6: Magenta
		255, 0, 255,
		// Index 7: White
		255, 255, 255,
	}

	cs := NewIndexedColorSpace(base, hival, lookup)

	tests := []struct {
		name     string
		index    float64
		expected color.RGBA
	}{
		{"Black", 0, color.RGBA{0, 0, 0, 255}},
		{"Red", 1, color.RGBA{255, 0, 0, 255}},
		{"Green", 2, color.RGBA{0, 255, 0, 255}},
		{"Blue", 3, color.RGBA{0, 0, 255, 255}},
		{"Yellow", 4, color.RGBA{255, 255, 0, 255}},
		{"Cyan", 5, color.RGBA{0, 255, 255, 255}},
		{"Magenta", 6, color.RGBA{255, 0, 255, 255}},
		{"White", 7, color.RGBA{255, 255, 255, 255}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cs.ConvertToRGBA([]float64{tt.index})
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIndexedColorSpace_OutOfRange(t *testing.T) {
	base := NewDeviceRGB()
	hival := 3
	lookup := []byte{
		255, 0, 0,
		0, 255, 0,
		0, 0, 255,
		255, 255, 255,
	}

	cs := NewIndexedColorSpace(base, hival, lookup)

	tests := []struct {
		name     string
		index    float64
		expected color.RGBA
	}{
		{"NegativeIndex", -1, color.RGBA{0, 0, 0, 255}},
		{"TooLarge", 10, color.RGBA{0, 0, 0, 255}},
		{"Empty", 0, color.RGBA{255, 0, 0, 255}}, // Valid: index 0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cs.ConvertToRGBA([]float64{tt.index})
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIndexedColorSpace_EmptyValues(t *testing.T) {
	base := NewDeviceRGB()
	hival := 1
	lookup := []byte{255, 0, 0, 0, 255, 0}

	cs := NewIndexedColorSpace(base, hival, lookup)
	result := cs.ConvertToRGBA([]float64{})
	assert.Equal(t, color.RGBA{0, 0, 0, 255}, result)
}

// TestPatternColorSpace tests
func TestNewPatternColorSpace_Colored(t *testing.T) {
	// Colored pattern (no base color space)
	cs := NewPatternColorSpace(nil, false)
	require.NotNil(t, cs)
	assert.Equal(t, ColorSpacePattern, cs.Type())
	assert.Equal(t, "Pattern", cs.Name())
	assert.Equal(t, 0, cs.GetNumComponents())
	assert.False(t, cs.IsUncolored())
	assert.Nil(t, cs.GetBaseColorSpace())
}

func TestNewPatternColorSpace_Uncolored(t *testing.T) {
	// Uncolored pattern with DeviceRGB base
	base := NewDeviceRGB()
	cs := NewPatternColorSpace(base, true)
	require.NotNil(t, cs)
	assert.Equal(t, ColorSpacePattern, cs.Type())
	assert.Equal(t, "Pattern", cs.Name())
	assert.Equal(t, 3, cs.GetNumComponents())
	assert.True(t, cs.IsUncolored())
	assert.NotNil(t, cs.GetBaseColorSpace())
}

func TestPatternColorSpace_ConvertToRGBA_Colored(t *testing.T) {
	// Colored pattern returns black (patterns don't convert to RGB)
	cs := NewPatternColorSpace(nil, false)
	result := cs.ConvertToRGBA([]float64{1.0, 0.5, 0.25})
	assert.Equal(t, color.RGBA{0, 0, 0, 255}, result)
}

func TestPatternColorSpace_ConvertToRGBA_Uncolored(t *testing.T) {
	// Uncolored pattern uses base color space
	base := NewDeviceRGB()
	cs := NewPatternColorSpace(base, true)

	result := cs.ConvertToRGBA([]float64{1.0, 0.0, 0.0})
	assert.Equal(t, color.RGBA{255, 0, 0, 255}, result)

	result = cs.ConvertToRGBA([]float64{0.0, 1.0, 0.0})
	assert.Equal(t, color.RGBA{0, 255, 0, 255}, result)
}

func TestPatternColorSpace_WithDeviceGray(t *testing.T) {
	base := NewDeviceGray()
	cs := NewPatternColorSpace(base, true)

	assert.Equal(t, 1, cs.GetNumComponents())

	result := cs.ConvertToRGBA([]float64{0.5})
	assert.Equal(t, color.RGBA{128, 128, 128, 255}, result)
}

func TestPatternColorSpace_WithDeviceCMYK(t *testing.T) {
	base := NewDeviceCMYK()
	cs := NewPatternColorSpace(base, true)

	assert.Equal(t, 4, cs.GetNumComponents())

	// Test with CMYK black
	result := cs.ConvertToRGBA([]float64{0, 0, 0, 1})
	// Should be close to black (with some tolerance for CMYK conversion)
	assert.Less(t, result.R, uint8(50))
	assert.Less(t, result.G, uint8(50))
	assert.Less(t, result.B, uint8(55))
}

// TestSeparationColorSpace tests
// Note: Separation tests require Function implementation
// We'll test the basic structure and nil cases

func TestSeparationColorSpace_Basic(t *testing.T) {
	// Create a separation color space without tint function
	// This is just to test the structure
	alternate := NewDeviceRGB()
	cs := NewSeparationColorSpace("Pantone123", alternate, nil)

	require.NotNil(t, cs)
	assert.Equal(t, ColorSpaceSeparation, cs.Type())
	assert.Equal(t, "Pantone123", cs.Name())
	assert.Equal(t, 1, cs.GetNumComponents())
}

func TestSeparationColorSpace_NoTintFunction(t *testing.T) {
	alternate := NewDeviceRGB()
	cs := NewSeparationColorSpace("Spot", alternate, nil)

	// Without tint function, should fall back to alternate with 0
	result := cs.ConvertToRGBA([]float64{0.5})
	assert.Equal(t, color.RGBA{0, 0, 0, 255}, result)
}

func TestSeparationColorSpace_NoAlternate(t *testing.T) {
	cs := NewSeparationColorSpace("Spot", nil, nil)

	// Without alternate, should return black
	result := cs.ConvertToRGBA([]float64{0.5})
	assert.Equal(t, color.RGBA{0, 0, 0, 255}, result)
}

func TestSeparationColorSpace_EmptyValues(t *testing.T) {
	alternate := NewDeviceGray()
	cs := NewSeparationColorSpace("Spot", alternate, nil)

	// Empty values should fall back to alternate with 0
	result := cs.ConvertToRGBA([]float64{})
	assert.Equal(t, color.RGBA{0, 0, 0, 255}, result)
}
