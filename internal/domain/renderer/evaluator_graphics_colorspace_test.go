package renderer

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/colorspace"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestSetColorBySpaceUsesSeparationTintTransform(t *testing.T) {
	e := NewEvaluator(nil)
	resources := entity.NewDict()
	colorSpaces := entity.NewDict()
	tintFunction := entity.NewDict()
	tintFunction.Set(entity.Name("FunctionType"), entity.NewInteger(2))
	tintFunction.Set(entity.Name("C0"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(0), entity.NewReal(0), entity.NewReal(0),
	))
	tintFunction.Set(entity.Name("C1"), entity.NewArray(
		entity.NewReal(1), entity.NewReal(0), entity.NewReal(0), entity.NewReal(0),
	))
	tintFunction.Set(entity.Name("N"), entity.NewInteger(1))
	colorSpaces.Set(entity.Name("CS0"), entity.NewArray(
		entity.Name("Separation"),
		entity.Name("SpotCyan"),
		entity.Name("DeviceCMYK"),
		tintFunction,
	))
	resources.Set(entity.Name("ColorSpace"), colorSpaces)
	e.SetResources(resources)

	require.NoError(t, e.setFillColorSpace(Operator{Operands: []entity.Object{entity.Name("CS0")}}))
	require.NoError(t, e.setFillColorBySpace(Operator{Operands: []entity.Object{entity.NewReal(0.25)}}))

	expected := colorspace.NewDeviceCMYK().ConvertToRGBA([]float64{0.25, 0, 0, 0})
	assert.Equal(t, "Separation", e.graphics.fillCS)
	assert.Equal(t, fmt.Sprintf("%02X%02X%02X", expected.R, expected.G, expected.B), e.graphics.fillColor.Color.(*Color).Hex)
	assert.NotEqual(t, grayToHex(0.25, 0.25, 0.25), e.graphics.fillColor.Color.(*Color).Hex)
}

func TestSetColorSpaceResetsColorToPopplerDefault(t *testing.T) {
	e := NewEvaluator(nil)
	e.graphics.fillColor = &ColorSpace{Color: &Color{Hex: "FF0000"}}
	e.graphics.strokeColor = &ColorSpace{Color: &Color{Hex: "00FF00"}}

	require.NoError(t, e.setFillColorSpace(Operator{Operands: []entity.Object{entity.Name("DeviceCMYK")}}))
	require.NoError(t, e.setStrokeColorSpace(Operator{Operands: []entity.Object{entity.Name("DeviceRGB")}}))

	expectedCMYKDefault := colorspace.NewDeviceCMYK().ConvertToRGBA([]float64{0, 0, 0, 1})
	assert.Equal(t, fmt.Sprintf("%02X%02X%02X", expectedCMYKDefault.R, expectedCMYKDefault.G, expectedCMYKDefault.B), e.graphics.fillColor.Color.(*Color).Hex)
	assert.Equal(t, "000000", e.graphics.strokeColor.Color.(*Color).Hex)
}

func TestSetColorSpaceClearsPatternEvenForPatternColorSpace(t *testing.T) {
	e := NewEvaluator(nil)
	e.graphics.fillPattern = entity.NewTilingPattern("fill", 1, entity.TilingConstantSpacing)
	e.graphics.strokePattern = entity.NewTilingPattern("stroke", 1, entity.TilingConstantSpacing)

	require.NoError(t, e.setFillColorSpace(Operator{Operands: []entity.Object{entity.Name("Pattern")}}))
	require.NoError(t, e.setStrokeColorSpace(Operator{Operands: []entity.Object{entity.Name("Pattern")}}))

	assert.Nil(t, e.graphics.fillPattern)
	assert.Nil(t, e.graphics.strokePattern)
	assert.Equal(t, "Pattern", e.graphics.fillCS)
	assert.Equal(t, "Pattern", e.graphics.strokeCS)
}

func TestDeviceColorOperatorsClearPatternState(t *testing.T) {
	tests := []struct {
		name       string
		op         Operator
		apply      func(*Evaluator, Operator) error
		wantFill   bool
		wantStroke bool
	}{
		{
			name:     "gray fill",
			op:       Operator{Operands: []entity.Object{entity.NewReal(0.5)}},
			apply:    (*Evaluator).setGrayFill,
			wantFill: true,
		},
		{
			name:       "gray stroke",
			op:         Operator{Operands: []entity.Object{entity.NewReal(0.5)}},
			apply:      (*Evaluator).setGrayStroke,
			wantStroke: true,
		},
		{
			name:     "rgb fill",
			op:       Operator{Operands: []entity.Object{entity.NewReal(1), entity.NewReal(0), entity.NewReal(0)}},
			apply:    (*Evaluator).setRGBFill,
			wantFill: true,
		},
		{
			name:       "rgb stroke",
			op:         Operator{Operands: []entity.Object{entity.NewReal(1), entity.NewReal(0), entity.NewReal(0)}},
			apply:      (*Evaluator).setRGBStroke,
			wantStroke: true,
		},
		{
			name:     "cmyk fill",
			op:       Operator{Operands: []entity.Object{entity.NewReal(0), entity.NewReal(1), entity.NewReal(0), entity.NewReal(0)}},
			apply:    (*Evaluator).setCMYKFill,
			wantFill: true,
		},
		{
			name:       "cmyk stroke",
			op:         Operator{Operands: []entity.Object{entity.NewReal(0), entity.NewReal(1), entity.NewReal(0), entity.NewReal(0)}},
			apply:      (*Evaluator).setCMYKStroke,
			wantStroke: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEvaluator(nil)
			e.graphics.fillPattern = entity.NewTilingPattern("fill", 1, entity.TilingConstantSpacing)
			e.graphics.strokePattern = entity.NewTilingPattern("stroke", 1, entity.TilingConstantSpacing)
			e.graphics.fillPatternBaseCS = "DeviceRGB"
			e.graphics.strokePatternBaseCS = "DeviceRGB"

			require.NoError(t, tt.apply(e, tt.op))

			if tt.wantFill {
				assert.Nil(t, e.graphics.fillPattern)
				assert.Empty(t, e.graphics.fillPatternBaseCS)
				assert.NotNil(t, e.graphics.strokePattern)
			}
			if tt.wantStroke {
				assert.Nil(t, e.graphics.strokePattern)
				assert.Empty(t, e.graphics.strokePatternBaseCS)
				assert.NotNil(t, e.graphics.fillPattern)
			}
		})
	}
}

func TestSetColorBySpaceClearsPatternForNonPatternColorSpace(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*Evaluator)
		op         Operator
		apply      func(*Evaluator, Operator) error
		wantFill   bool
		wantStroke bool
	}{
		{
			name: "fill sc clears prior pattern",
			setup: func(e *Evaluator) {
				e.graphics.fillCS = "DeviceRGB"
			},
			op:       Operator{Operands: []entity.Object{entity.NewReal(1), entity.NewReal(1), entity.NewReal(1)}},
			apply:    (*Evaluator).setFillColorBySpace,
			wantFill: true,
		},
		{
			name: "stroke SC clears prior pattern",
			setup: func(e *Evaluator) {
				e.graphics.strokeCS = "DeviceRGB"
			},
			op:         Operator{Operands: []entity.Object{entity.NewReal(0), entity.NewReal(0), entity.NewReal(0)}},
			apply:      (*Evaluator).setStrokeColorBySpace,
			wantStroke: true,
		},
		{
			name: "fill scn clears prior pattern for non-pattern parsed space",
			setup: func(e *Evaluator) {
				e.graphics.fillCS = "DeviceGray"
				e.graphics.fillParsedCS = colorspace.NewDeviceGray()
			},
			op:       Operator{Operands: []entity.Object{entity.NewReal(0.25)}},
			apply:    (*Evaluator).setFillColorBySpace,
			wantFill: true,
		},
		{
			name: "stroke SCN clears prior pattern for non-pattern parsed space",
			setup: func(e *Evaluator) {
				e.graphics.strokeCS = "DeviceGray"
				e.graphics.strokeParsedCS = colorspace.NewDeviceGray()
			},
			op:         Operator{Operands: []entity.Object{entity.NewReal(0.75)}},
			apply:      (*Evaluator).setStrokeColorBySpace,
			wantStroke: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEvaluator(nil)
			e.graphics.fillPattern = entity.NewTilingPattern("fill", 1, entity.TilingConstantSpacing)
			e.graphics.strokePattern = entity.NewTilingPattern("stroke", 1, entity.TilingConstantSpacing)
			e.graphics.fillPatternBaseCS = "DeviceRGB"
			e.graphics.strokePatternBaseCS = "DeviceRGB"
			tt.setup(e)

			require.NoError(t, tt.apply(e, tt.op))

			if tt.wantFill {
				assert.Nil(t, e.graphics.fillPattern)
				assert.Empty(t, e.graphics.fillPatternBaseCS)
				assert.NotNil(t, e.graphics.strokePattern)
			}
			if tt.wantStroke {
				assert.Nil(t, e.graphics.strokePattern)
				assert.Empty(t, e.graphics.strokePatternBaseCS)
				assert.NotNil(t, e.graphics.fillPattern)
			}
		})
	}
}

func TestSetColorBySpaceRejectsWrongComponentCount(t *testing.T) {
	e := NewEvaluator(nil)
	oldFill := entity.NewTilingPattern("old-fill", 1, entity.TilingConstantSpacing)
	e.graphics.fillCS = "DeviceRGB"
	e.graphics.fillPattern = oldFill
	e.graphics.fillPatternBaseCS = "DeviceRGB"
	e.graphics.fillColor = &ColorSpace{Color: &Color{Hex: "123456"}}

	require.NoError(t, e.setFillColorBySpace(Operator{Operands: []entity.Object{
		entity.NewReal(1),
		entity.NewReal(0),
	}}))

	assert.True(t, oldFill == e.graphics.fillPattern)
	assert.Equal(t, "DeviceRGB", e.graphics.fillPatternBaseCS)
	require.NotNil(t, e.graphics.fillColor)
	fillColor, ok := e.graphics.fillColor.Color.(*Color)
	require.True(t, ok)
	assert.Equal(t, "123456", fillColor.Hex)
}

func TestSetColorNBySpaceUsesPopplerFallbackForNonNumericOperands(t *testing.T) {
	e := NewEvaluator(nil)
	e.graphics.fillCS = "DeviceRGB"

	require.NoError(t, e.setFillColorBySpace(Operator{Opcode: "scn", Operands: []entity.Object{
		entity.NewReal(1),
		entity.Name("BadComponent"),
		entity.NewReal(0),
	}}))

	require.NotNil(t, e.graphics.fillColor)
	fillColor, ok := e.graphics.fillColor.Color.(*Color)
	require.True(t, ok)
	assert.Equal(t, "FF0000", fillColor.Hex)
}

func TestSetColorBySpaceRejectsNonNumericOperandsForSC(t *testing.T) {
	e := NewEvaluator(nil)
	e.graphics.fillCS = "DeviceRGB"
	e.graphics.fillColor = &ColorSpace{Color: &Color{Hex: "123456"}}

	require.NoError(t, e.setFillColorBySpace(Operator{Opcode: "sc", Operands: []entity.Object{
		entity.NewReal(1),
		entity.Name("BadComponent"),
		entity.NewReal(0),
	}}))

	require.NotNil(t, e.graphics.fillColor)
	fillColor, ok := e.graphics.fillColor.Color.(*Color)
	require.True(t, ok)
	assert.Equal(t, "123456", fillColor.Hex)
}

func TestFixedArityColorOperatorsUseTrailingOperands(t *testing.T) {
	e := NewEvaluator(nil)

	require.NoError(t, e.setRGBFill(Operator{Opcode: "rg", Operands: []entity.Object{
		entity.Name("IgnoredLeadingExtra"),
		entity.NewReal(1),
		entity.NewReal(0),
		entity.NewReal(0),
	}}))

	require.NotNil(t, e.graphics.fillColor)
	fillColor, ok := e.graphics.fillColor.Color.(*Color)
	require.True(t, ok)
	assert.Equal(t, "FF0000", fillColor.Hex)
}

func TestPatternColorSpaceWithoutPatternSkipsPaint(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := newRecordingCanvas()
	e.SetCanvas(canvas)
	e.graphics.fillCS = "Pattern"
	e.graphics.fillPattern = nil
	e.graphics.path.AddElement(&MoveTo{X: 0, Y: 0})
	e.graphics.path.AddElement(&LineTo{X: 1, Y: 0})
	e.graphics.path.AddElement(&LineTo{X: 1, Y: 1})
	e.graphics.path.AddElement(&Close{})

	require.NoError(t, e.fillPath())

	assert.Zero(t, canvas.fillCalls)
	assert.Zero(t, canvas.strokeCalls)
}

func TestPatternColorSpaceWithoutStrokePatternKeepsFillOnly(t *testing.T) {
	e := NewEvaluator(nil)
	canvas := &combinedTestCanvas{testCanvas: newRecordingCanvas()}
	e.SetCanvas(canvas)
	e.graphics.fillCS = "DeviceRGB"
	e.graphics.strokeCS = "Pattern"
	e.graphics.strokePattern = nil
	e.graphics.path.AddElement(&MoveTo{X: 0, Y: 0})
	e.graphics.path.AddElement(&LineTo{X: 1, Y: 0})
	e.graphics.path.AddElement(&LineTo{X: 1, Y: 1})
	e.graphics.path.AddElement(&Close{})

	require.NoError(t, e.fillAndStrokePath())

	assert.Equal(t, 1, canvas.fillCalls)
	assert.Zero(t, canvas.strokeCalls)
	assert.Zero(t, canvas.fillAndStrokeCalls)
}

func TestSetSeparationColorSpaceDefaultsToTintOne(t *testing.T) {
	e := NewEvaluator(nil)
	resources := entity.NewDict()
	colorSpaces := entity.NewDict()
	tintFunction := entity.NewDict()
	tintFunction.Set(entity.Name("FunctionType"), entity.NewInteger(2))
	tintFunction.Set(entity.Name("C0"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(0), entity.NewReal(0), entity.NewReal(0),
	))
	tintFunction.Set(entity.Name("C1"), entity.NewArray(
		entity.NewReal(1), entity.NewReal(0), entity.NewReal(0), entity.NewReal(0),
	))
	tintFunction.Set(entity.Name("N"), entity.NewInteger(1))
	colorSpaces.Set(entity.Name("CS0"), entity.NewArray(
		entity.Name("Separation"),
		entity.Name("SpotCyan"),
		entity.Name("DeviceCMYK"),
		tintFunction,
	))
	resources.Set(entity.Name("ColorSpace"), colorSpaces)
	e.SetResources(resources)

	require.NoError(t, e.setFillColorSpace(Operator{Operands: []entity.Object{entity.Name("CS0")}}))

	expected := colorspace.NewDeviceCMYK().ConvertToRGBA([]float64{1, 0, 0, 0})
	assert.Equal(t, fmt.Sprintf("%02X%02X%02X", expected.R, expected.G, expected.B), e.graphics.fillColor.Color.(*Color).Hex)
}
