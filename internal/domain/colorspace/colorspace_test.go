package colorspace

import (
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestDeviceGray_Basic(t *testing.T) {
	cs := NewDeviceGray()

	assert.Equal(t, ColorSpaceDeviceGray, cs.Type())
	assert.Equal(t, "DeviceGray", cs.Name())
	assert.Equal(t, 1, cs.GetNumComponents())
}

func TestDeviceGray_ConvertToRGBA(t *testing.T) {
	cs := NewDeviceGray()

	tests := []struct {
		name     string
		input    []float64
		expected color.RGBA
	}{
		{"Black", []float64{0.0}, color.RGBA{0, 0, 0, 255}},
		{"White", []float64{1.0}, color.RGBA{255, 255, 255, 255}},
		{"MidGray", []float64{0.5}, color.RGBA{128, 128, 128, 255}},
		{"Empty", []float64{}, color.RGBA{0, 0, 0, 255}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cs.ConvertToRGBA(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeviceRGB_Basic(t *testing.T) {
	cs := NewDeviceRGB()

	assert.Equal(t, ColorSpaceDeviceRGB, cs.Type())
	assert.Equal(t, "DeviceRGB", cs.Name())
	assert.Equal(t, 3, cs.GetNumComponents())
}

func TestDeviceRGB_ConvertToRGBA(t *testing.T) {
	cs := NewDeviceRGB()

	tests := []struct {
		name     string
		input    []float64
		expected color.RGBA
	}{
		{"Black", []float64{0.0, 0.0, 0.0}, color.RGBA{0, 0, 0, 255}},
		{"White", []float64{1.0, 1.0, 1.0}, color.RGBA{255, 255, 255, 255}},
		{"Red", []float64{1.0, 0.0, 0.0}, color.RGBA{255, 0, 0, 255}},
		{"Green", []float64{0.0, 1.0, 0.0}, color.RGBA{0, 255, 0, 255}},
		{"Blue", []float64{0.0, 0.0, 1.0}, color.RGBA{0, 0, 255, 255}},
		{"Yellow", []float64{1.0, 1.0, 0.0}, color.RGBA{255, 255, 0, 255}},
		{"Cyan", []float64{0.0, 1.0, 1.0}, color.RGBA{0, 255, 255, 255}},
		{"Magenta", []float64{1.0, 0.0, 1.0}, color.RGBA{255, 0, 255, 255}},
		{"MidGray", []float64{0.5, 0.5, 0.5}, color.RGBA{128, 128, 128, 255}},
		{"Empty", []float64{}, color.RGBA{0, 0, 0, 255}},
		{"TooFew", []float64{1.0, 0.5}, color.RGBA{0, 0, 0, 255}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cs.ConvertToRGBA(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeviceCMYK_Basic(t *testing.T) {
	cs := NewDeviceCMYK()

	assert.Equal(t, ColorSpaceDeviceCMYK, cs.Type())
	assert.Equal(t, "DeviceCMYK", cs.Name())
	assert.Equal(t, 4, cs.GetNumComponents())
}

func TestDeviceCMYK_ConvertToRGBA(t *testing.T) {
	cs := NewDeviceCMYK()

	tests := []struct {
		name      string
		input     []float64
		expected  color.RGBA
		tolerance uint8 // Allow small differences due to floating point
	}{
		{"Black", []float64{0.0, 0.0, 0.0, 1.0}, color.RGBA{35, 31, 32, 255}, 1},
		{"White", []float64{0.0, 0.0, 0.0, 0.0}, color.RGBA{255, 255, 255, 255}, 1},
		{"Cyan", []float64{1.0, 0.0, 0.0, 0.0}, color.RGBA{0, 173, 239, 255}, 1},
		{"Magenta", []float64{0.0, 1.0, 0.0, 0.0}, color.RGBA{236, 0, 140, 255}, 1},
		{"Yellow", []float64{0.0, 0.0, 1.0, 0.0}, color.RGBA{255, 242, 0, 255}, 1},
		{"Gray50", []float64{0.0, 0.0, 0.0, 0.5}, color.RGBA{145, 143, 143, 255}, 1},

		// Edge cases
		{"Empty", []float64{}, color.RGBA{0, 0, 0, 255}, 0},
		{"TooFew", []float64{0.5, 0.5, 0.5}, color.RGBA{0, 0, 0, 255}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cs.ConvertToRGBA(tt.input)

			// Check within tolerance
			diffR := abs(int(result.R) - int(tt.expected.R))
			diffG := abs(int(result.G) - int(tt.expected.G))
			diffB := abs(int(result.B) - int(tt.expected.B))

			assert.LessOrEqual(t, diffR, int(tt.tolerance), "R component")
			assert.LessOrEqual(t, diffG, int(tt.tolerance), "G component")
			assert.LessOrEqual(t, diffB, int(tt.tolerance), "B component")
			assert.Equal(t, uint8(255), result.A, "A component")
		})
	}
}

func TestColorSpaceType_String(t *testing.T) {
	tests := []struct {
		expected string
		csType   ColorSpaceType
	}{
		{"DeviceGray", ColorSpaceDeviceGray},
		{"DeviceRGB", ColorSpaceDeviceRGB},
		{"DeviceCMYK", ColorSpaceDeviceCMYK},
		{"Pattern", ColorSpacePattern},
		{"ICCBased", ColorSpaceICCBased},
		{"CalGray", ColorSpaceCalGray},
		{"CalRGB", ColorSpaceCalRGB},
		{"Lab", ColorSpaceLab},
		{"Indexed", ColorSpaceIndexed},
		{"Separation", ColorSpaceSeparation},
		{"DeviceN", ColorSpaceDeviceN},
		{"Unknown", ColorSpaceType(999)},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.csType.String())
		})
	}
}

func TestParseFunctionFromObject_PostScriptType4Stream(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.NewName("FunctionType"), entity.NewInteger(4))
	dict.Set(entity.NewName("Domain"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(1),
	))
	dict.Set(entity.NewName("Range"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(1),
		entity.NewReal(0), entity.NewReal(1),
		entity.NewReal(0), entity.NewReal(1),
	))
	streamObj := entity.NewStream(dict, []byte("{ dup dup }"))

	fn, err := parseFunctionFromObject(streamObj)
	assert.NoError(t, err)
	if !assert.NotNil(t, fn) {
		return
	}

	out, evalErr := fn.Evaluate([]float64{0.25})
	assert.NoError(t, evalErr)
	if !assert.Len(t, out, 3) {
		return
	}
	assert.Equal(t, 0.25, out[0])
	assert.Equal(t, 0.25, out[1])
	assert.Equal(t, 0.25, out[2])
}

func TestParseFunctionFromObject_PostScriptType4DictFallback(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.NewName("FunctionType"), entity.NewInteger(4))
	dict.Set(entity.NewName("Domain"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(1),
	))
	dict.Set(entity.NewName("Range"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(1),
	))

	fn, err := parseFunctionFromObject(dict)
	assert.NoError(t, err)
	if !assert.NotNil(t, fn) {
		return
	}

	out, evalErr := fn.Evaluate([]float64{0.75})
	assert.NoError(t, evalErr)
	if !assert.Len(t, out, 1) {
		return
	}
	assert.Equal(t, 0.75, out[0])
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
