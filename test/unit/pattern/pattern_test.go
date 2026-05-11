// Package pattern provides tests for pattern parsing and rendering.
package pattern

import (
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/pattern"
)

// TestParseTilingPattern tests parsing a tiling pattern.
func TestParseTilingPattern(t *testing.T) {
	tests := []struct {
		name        string
		patternType entity.PatternType
		paintType   int
		tilingType  entity.TilingType
		xStep       float64
		yStep       float64
		bbox        [4]float64
	}{
		{
			name:        "simple_tiling",
			patternType: entity.PatternTiling,
			paintType:   1,
			tilingType:  entity.TilingConstantSpacing,
			xStep:       100,
			yStep:       100,
			bbox:        [4]float64{0, 0, 50, 50},
		},
		{
			name:        "colored_tiling",
			patternType: entity.PatternTiling,
			paintType:   1,
			tilingType:  entity.TilingConstantSpacing,
			xStep:       72,
			yStep:       72,
			bbox:        [4]float64{0, 0, 36, 36},
		},
		{
			name:        "uncolored_tiling",
			patternType: entity.PatternTiling,
			paintType:   2,
			tilingType:  entity.TilingNoDistortion,
			xStep:       48,
			yStep:       48,
			bbox:        [4]float64{0, 0, 24, 24},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a pattern dictionary
			dict := entity.NewDict()

			dict.Set(entity.NewName(string(pattern.KeyPatternType)), entity.NewInteger(int64(tt.patternType)))
			dict.Set(entity.NewName(string(pattern.KeyPaintType)), entity.NewInteger(int64(tt.paintType)))
			dict.Set(entity.NewName(string(pattern.KeyTilingType)), entity.NewInteger(int64(tt.tilingType)))
			dict.Set(entity.NewName(string(pattern.KeyXStep)), entity.NewReal(tt.xStep))
			dict.Set(entity.NewName(string(pattern.KeyYStep)), entity.NewReal(tt.yStep))
			dict.Set(entity.NewName(string(pattern.KeyBBox)), entity.NewArray(
				entity.NewReal(tt.bbox[0]),
				entity.NewReal(tt.bbox[1]),
				entity.NewReal(tt.bbox[2]),
				entity.NewReal(tt.bbox[3]),
			))

			// Parse the pattern
			p, err := pattern.ParseTilingPattern(dict, nil, tt.name)
			if err != nil {
				require.FailNowf(t, "test failed", "Failed to parse tiling pattern: %v", err)
			}

			// Verify pattern type
			if p.Type() != entity.PatternTiling {
				assert.Failf(t, "assertion failed", "Expected pattern type %v, got %v", entity.PatternTiling, p.Type())
			}

			// Verify paint type
			if p.GetPaintType() != tt.paintType {
				assert.Failf(t, "assertion failed", "Expected paint type %d, got %d", tt.paintType, p.GetPaintType())
			}

			// Verify tiling type
			if p.GetTilingType() != tt.tilingType {
				assert.Failf(t, "assertion failed", "Expected tiling type %v, got %v", tt.tilingType, p.GetTilingType())
			}

			// Verify step sizes
			if p.GetXStep() != tt.xStep {
				assert.Failf(t, "assertion failed", "Expected xStep %f, got %f", tt.xStep, p.GetXStep())
			}
			if p.GetYStep() != tt.yStep {
				assert.Failf(t, "assertion failed", "Expected yStep %f, got %f", tt.yStep, p.GetYStep())
			}

			// Verify bounding box
			bbox := p.GetBBox()
			for i := 0; i < 4; i++ {
				if bbox[i] != tt.bbox[i] {
					assert.Failf(t, "assertion failed", "BBox[%d]: expected %f, got %f", i, tt.bbox[i], bbox[i])
				}
			}
		})
	}
}

func TestParseShading_BackgroundColor(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.NewName("ShadingType"), entity.NewInteger(int64(entity.ShadingAxial)))
	dict.Set(entity.NewName(string(pattern.KeyColorSpace)), entity.NewName("DeviceRGB"))
	dict.Set(entity.NewName(string(pattern.KeyCoords)), entity.NewArray(
		entity.NewReal(0),
		entity.NewReal(0),
		entity.NewReal(100),
		entity.NewReal(100),
	))
	dict.Set(entity.NewName(string(pattern.KeyBackground)), entity.NewArray(
		entity.NewReal(0.1),
		entity.NewReal(0.2),
		entity.NewReal(0.3),
	))

	shading, err := pattern.ParseShading(dict, nil)
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to parse shading background: %v", err)
	}

	bg := shading.GetBackground()
	if bg == nil {
		require.FailNowf(t, "test failed", "Expected background color to be parsed")
	}

	rgba, ok := bg.(color.RGBA)
	if !ok {
		require.FailNowf(t, "test failed", "Expected RGBA background color, got %T", bg)
	}
	if rgba.R == 0 && rgba.G == 0 && rgba.B == 0 {
		assert.Failf(t, "assertion failed", "Expected non-black parsed background color")
	}
}

// TestParseShadingPattern tests parsing a shading pattern.
func TestParseShadingPattern(t *testing.T) {
	tests := []struct {
		name        string
		colorSpace  string
		coords      []float64
		shadingType entity.ShadingType
	}{
		{
			name:        "axial_shading",
			shadingType: entity.ShadingAxial,
			colorSpace:  "DeviceRGB",
			coords:      []float64{0, 0, 100, 100},
		},
		{
			name:        "radial_shading",
			shadingType: entity.ShadingRadial,
			colorSpace:  "DeviceRGB",
			coords:      []float64{50, 50, 0, 50, 50, 50},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a shading dictionary
			shadingDict := entity.NewDict()
			shadingDict.Set(entity.NewName("ShadingType"), entity.NewInteger(int64(tt.shadingType)))
			shadingDict.Set(entity.NewName(string(pattern.KeyColorSpace)), entity.NewName(tt.colorSpace))
			shadingDict.Set(entity.NewName(string(pattern.KeyCoords)), entity.NewArray(
				float64ToRealArray(tt.coords)...,
			))

			// Create a pattern dictionary
			patternDict := entity.NewDict()
			patternDict.Set(entity.NewName(string(pattern.KeyPatternType)), entity.NewInteger(int64(entity.PatternShading)))
			patternDict.Set(entity.NewName(string(pattern.KeyShading)), shadingDict)

			// Parse the pattern
			p, err := pattern.ParseShadingPattern(patternDict, nil, tt.name)
			if err != nil {
				require.FailNowf(t, "test failed", "Failed to parse shading pattern: %v", err)
			}

			// Verify pattern type
			if p.Type() != entity.PatternShading {
				assert.Failf(t, "assertion failed", "Expected pattern type %v, got %v", entity.PatternShading, p.Type())
			}

			// Verify shading
			shading := p.GetShading()
			if shading == nil {
				require.Fail(t, "Expected shading object, got nil")
			}

			// Verify shading type
			if shading.GetShadingType() != tt.shadingType {
				assert.Failf(t, "assertion failed", "Expected shading type %v, got %v", tt.shadingType, shading.GetShadingType())
			}

			// Verify color space
			if shading.GetColorSpace() != tt.colorSpace {
				assert.Failf(t, "assertion failed", "Expected color space %s, got %s", tt.colorSpace, shading.GetColorSpace())
			}

			// Verify coordinates
			coords := shading.GetCoords()
			if len(coords) != len(tt.coords) {
				assert.Failf(t, "assertion failed", "Expected %d coordinates, got %d", len(tt.coords), len(coords))
			} else {
				for i := 0; i < len(coords); i++ {
					if coords[i] != tt.coords[i] {
						assert.Failf(t, "assertion failed", "Coord[%d]: expected %f, got %f", i, tt.coords[i], coords[i])
					}
				}
			}
		})
	}
}

// TestParseAxialShading tests parsing an axial shading.
func TestParseAxialShading(t *testing.T) {
	// Create an axial shading dictionary
	dict := entity.NewDict()
	dict.Set(entity.NewName("ShadingType"), entity.NewInteger(int64(entity.ShadingAxial)))
	dict.Set(entity.NewName(string(pattern.KeyColorSpace)), entity.NewName("DeviceRGB"))
	dict.Set(entity.NewName(string(pattern.KeyCoords)), entity.NewArray(
		entity.NewReal(0),
		entity.NewReal(0),
		entity.NewReal(100),
		entity.NewReal(100),
	))
	dict.Set(entity.NewName(string(pattern.KeyExtend)), entity.NewArray(
		entity.NewBoolean(true),
		entity.NewBoolean(false),
	))

	// Parse the shading
	shading, err := pattern.ParseShading(dict, nil)
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to parse axial shading: %v", err)
	}

	// Verify shading type
	if shading.GetShadingType() != entity.ShadingAxial {
		assert.Failf(t, "assertion failed", "Expected shading type %v, got %v", entity.ShadingAxial, shading.GetShadingType())
	}

	// Verify extend flags
	extend := shading.GetExtend()
	if !extend[0] || extend[1] {
		assert.Failf(t, "assertion failed", "Expected extend [true false], got [%v %v]", extend[0], extend[1])
	}
}

// TestParseRadialShading tests parsing a radial shading.
func TestParseRadialShading(t *testing.T) {
	// Create a radial shading dictionary
	dict := entity.NewDict()
	dict.Set(entity.NewName("ShadingType"), entity.NewInteger(int64(entity.ShadingRadial)))
	dict.Set(entity.NewName(string(pattern.KeyColorSpace)), entity.NewName("DeviceRGB"))
	dict.Set(entity.NewName(string(pattern.KeyCoords)), entity.NewArray(
		entity.NewReal(50),
		entity.NewReal(50),
		entity.NewReal(0),
		entity.NewReal(50),
		entity.NewReal(50),
		entity.NewReal(50),
	))
	dict.Set(entity.NewName(string(pattern.KeyExtend)), entity.NewArray(
		entity.NewBoolean(true),
		entity.NewBoolean(true),
	))

	// Parse the shading
	shading, err := pattern.ParseShading(dict, nil)
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to parse radial shading: %v", err)
	}

	// Verify shading type
	if shading.GetShadingType() != entity.ShadingRadial {
		assert.Failf(t, "assertion failed", "Expected shading type %v, got %v", entity.ShadingRadial, shading.GetShadingType())
	}

	// Verify coordinates
	coords := shading.GetCoords()
	if len(coords) != 6 {
		require.FailNowf(t, "test failed", "Expected 6 coordinates, got %d", len(coords))
	}

	// Verify extend flags
	extend := shading.GetExtend()
	if !extend[0] || !extend[1] {
		assert.Failf(t, "assertion failed", "Expected extend [true true], got [%v %v]", extend[0], extend[1])
	}
}

// TestExponentialFunction tests the exponential function.
func TestExponentialFunction(t *testing.T) {
	// Create an exponential interpolation function
	fn := &entity.ExponentialFunction{
		Domain:   [][2]float64{{0, 1}},
		RangeVal: [][2]float64{{0, 1}, {0, 1}, {0, 1}},
		C0:       []float64{0, 0, 0},
		C1:       []float64{1, 1, 1},
		Exponent: 1.0,
		N:        3,
	}

	// Test at t = 0
	result, err := fn.Evaluate([]float64{0})
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to evaluate function: %v", err)
	}
	if len(result) != 3 {
		assert.Failf(t, "assertion failed", "Expected 3 output values, got %d", len(result))
	}
	for i, v := range result {
		if v != 0 {
			assert.Failf(t, "assertion failed", "At t=0, result[%d] = %f, expected 0", i, v)
		}
	}

	// Test at t = 1
	result, err = fn.Evaluate([]float64{1})
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to evaluate function: %v", err)
	}
	if len(result) != 3 {
		assert.Failf(t, "assertion failed", "Expected 3 output values, got %d", len(result))
	}
	for i, v := range result {
		if v != 1 {
			assert.Failf(t, "assertion failed", "At t=1, result[%d] = %f, expected 1", i, v)
		}
	}

	// Test at t = 0.5
	result, err = fn.Evaluate([]float64{0.5})
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to evaluate function: %v", err)
	}
	for i, v := range result {
		if v != 0.5 {
			assert.Failf(t, "assertion failed", "At t=0.5, result[%d] = %f, expected 0.5", i, v)
		}
	}
}

func TestExponentialFunction_FractionalExponent(t *testing.T) {
	fn := &entity.ExponentialFunction{
		Domain:   [][2]float64{{0, 1}},
		RangeVal: [][2]float64{{0, 1}},
		C0:       []float64{0},
		C1:       []float64{1},
		Exponent: 0.5,
		N:        1,
	}

	result, err := fn.Evaluate([]float64{0.25})
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to evaluate function: %v", err)
	}
	if len(result) != 1 {
		require.FailNowf(t, "test failed", "Expected 1 output value, got %d", len(result))
	}
	if result[0] != 0.5 {
		require.FailNowf(t, "test failed", "Expected 0.5, got %f", result[0])
	}
}

func TestExponentialFunction_ClampAndMismatchedCoefficients(t *testing.T) {
	fn := &entity.ExponentialFunction{
		Domain:   [][2]float64{{0, 1}},
		RangeVal: [][2]float64{{0, 1}, {0, 0.5}},
		C0:       []float64{0},
		C1:       []float64{1, 2},
		Exponent: 1,
		N:        2,
	}

	// Input above domain should clamp to domain max (x=1).
	result, err := fn.Evaluate([]float64{2})
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to evaluate function: %v", err)
	}
	if len(result) != 2 {
		require.FailNowf(t, "test failed", "Expected 2 output values, got %d", len(result))
	}
	if result[0] != 1 {
		require.FailNowf(t, "test failed", "Expected first output 1, got %f", result[0])
	}
	if result[1] != 0.5 {
		require.FailNowf(t, "test failed", "Expected second output clamped to 0.5, got %f", result[1])
	}
}

func TestStitchingFunction_EvaluateSelectsLastSegment(t *testing.T) {
	fn0 := &entity.ExponentialFunction{
		Domain:   [][2]float64{{0, 1}},
		RangeVal: [][2]float64{{0, 1}},
		C0:       []float64{0},
		C1:       []float64{0},
		Exponent: 1,
		N:        1,
	}
	fn1 := &entity.ExponentialFunction{
		Domain:   [][2]float64{{0, 1}},
		RangeVal: [][2]float64{{0, 1}},
		C0:       []float64{1},
		C1:       []float64{1},
		Exponent: 1,
		N:        1,
	}

	st := &entity.StitchingFunction{
		Domain:    [][2]float64{{0, 1}},
		RangeVal:  [][2]float64{{0, 1}},
		Functions: []entity.Function{fn0, fn1},
		Bounds:    []float64{0.5},
		Encode:    [][2]float64{{0, 1}, {0, 1}},
	}

	left, err := st.Evaluate([]float64{0.25})
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to evaluate left segment: %v", err)
	}
	if len(left) != 1 || left[0] != 0 {
		require.FailNowf(t, "test failed", "Unexpected left segment output: %#v", left)
	}

	right, err := st.Evaluate([]float64{0.75})
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to evaluate right segment: %v", err)
	}
	if len(right) != 1 || right[0] != 1 {
		require.FailNowf(t, "test failed", "Unexpected right segment output: %#v", right)
	}
}

func TestStitchingFunction_EvaluateDoesNotClampParentRange(t *testing.T) {
	fn := &entity.ExponentialFunction{
		Domain:   [][2]float64{{0, 1}},
		RangeVal: [][2]float64{{0, 2}},
		C0:       []float64{2},
		C1:       []float64{2},
		Exponent: 1,
		N:        1,
	}

	st := &entity.StitchingFunction{
		Domain:    [][2]float64{{0, 1}},
		RangeVal:  [][2]float64{{0, 1}},
		Functions: []entity.Function{fn},
		Encode:    [][2]float64{{0, 1}},
	}

	out, err := st.Evaluate([]float64{0.5})
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to evaluate stitched function: %v", err)
	}
	if len(out) != 1 || out[0] != 2 {
		require.FailNowf(t, "test failed", "Expected Poppler-style unclamped output 2, got %#v", out)
	}
}

func TestPostScriptFunction_EvaluateSimpleProgram(t *testing.T) {
	fn := &entity.PostScriptFunction{
		Domain:   [][2]float64{{0, 1}},
		RangeVal: [][2]float64{{0, 1}, {0, 1}, {0, 1}},
		Program:  "{ dup dup }",
	}

	result, err := fn.Evaluate([]float64{0.25})
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to evaluate PostScript function: %v", err)
	}
	if len(result) != 3 {
		require.FailNowf(t, "test failed", "Expected 3 output values, got %d", len(result))
	}
	for i, v := range result {
		if v != 0.25 {
			assert.Failf(t, "assertion failed", "result[%d] = %f, expected 0.25", i, v)
		}
	}
}

func TestPostScriptFunction_EvaluateIfElse(t *testing.T) {
	fn := &entity.PostScriptFunction{
		Domain:   [][2]float64{{0, 1}},
		RangeVal: [][2]float64{{0, 1}},
		Program:  "{ dup 0.5 gt { pop 1 } { pop 0 } ifelse }",
	}

	low, err := fn.Evaluate([]float64{0.2})
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to evaluate PostScript function (low): %v", err)
	}
	if len(low) != 1 || low[0] != 0 {
		require.FailNowf(t, "test failed", "Unexpected low result: %#v", low)
	}

	high, err := fn.Evaluate([]float64{0.8})
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to evaluate PostScript function (high): %v", err)
	}
	if len(high) != 1 || high[0] != 1 {
		require.FailNowf(t, "test failed", "Unexpected high result: %#v", high)
	}
}

func TestParseAxialShading_WithPostScriptFunction(t *testing.T) {
	functionDict := entity.NewDict()
	functionDict.Set(entity.NewName("FunctionType"), entity.NewInteger(4))
	functionDict.Set(entity.NewName("Domain"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(1),
	))
	functionDict.Set(entity.NewName("Range"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(1),
		entity.NewReal(0), entity.NewReal(1),
		entity.NewReal(0), entity.NewReal(1),
	))
	functionStream := entity.NewStream(functionDict, []byte("{ dup dup }"))

	shadingDict := entity.NewDict()
	shadingDict.Set(entity.NewName("ShadingType"), entity.NewInteger(int64(entity.ShadingAxial)))
	shadingDict.Set(entity.NewName("ColorSpace"), entity.NewName("DeviceRGB"))
	shadingDict.Set(entity.NewName("Coords"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(0),
		entity.NewReal(100), entity.NewReal(0),
	))
	shadingDict.Set(entity.NewName("Function"), functionStream)

	shading, err := pattern.ParseShading(shadingDict, nil)
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to parse axial shading with PostScript function: %v", err)
	}

	functions := shading.GetFunctions()
	if len(functions) != 1 {
		require.FailNowf(t, "test failed", "Expected one parsed function, got %d", len(functions))
	}

	out, err := functions[0].Evaluate([]float64{0.5})
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to evaluate parsed function: %v", err)
	}
	if len(out) != 3 || out[0] != 0.5 || out[1] != 0.5 || out[2] != 0.5 {
		require.FailNowf(t, "test failed", "Unexpected parsed function output: %#v", out)
	}
}

// TestAxialGradientPoints tests parsing axial gradient coordinates.
func TestAxialGradientPoints(t *testing.T) {
	shading := entity.NewAxialShading("DeviceRGB", 10, 20, 30, 40, nil, [2]bool{true, false})

	x0, y0, x1, y1, err := pattern.ParseAxialGradientPoints(shading)
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to parse axial gradient points: %v", err)
	}

	if x0 != 10 || y0 != 20 || x1 != 30 || y1 != 40 {
		assert.Failf(t, "assertion failed", "Expected (10, 20, 30, 40), got (%f, %f, %f, %f)", x0, y0, x1, y1)
	}
}

// TestRadialGradientPoints tests parsing radial gradient coordinates.
func TestRadialGradientPoints(t *testing.T) {
	shading := entity.NewRadialShading("DeviceRGB", 10, 20, 5, 30, 40, 15, nil, [2]bool{true, true})

	x0, y0, r0, x1, y1, r1, err := pattern.ParseRadialGradientPoints(shading)
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to parse radial gradient points: %v", err)
	}

	if x0 != 10 || y0 != 20 || r0 != 5 || x1 != 30 || y1 != 40 || r1 != 15 {
		assert.Failf(t, "assertion failed", "Expected (10, 20, 5, 30, 40, 15), got (%f, %f, %f, %f, %f, %f)", x0, y0, r0, x1, y1, r1)
	}
}

// TestValidatePattern tests pattern validation.
func TestValidatePattern(t *testing.T) {
	tests := []struct {
		pattern entity.Pattern
		name    string
		valid   bool
	}{
		{
			name: "valid_tiling",
			pattern: func() entity.Pattern {
				p := entity.NewTilingPattern("test", 1, entity.TilingConstantSpacing)
				p.SetXStep(100)
				p.SetYStep(100)
				return p
			}(),
			valid: true,
		},
		{
			name: "invalid_tiling_zero_step",
			pattern: func() entity.Pattern {
				p := entity.NewTilingPattern("test", 1, entity.TilingConstantSpacing)
				p.SetXStep(0)
				p.SetYStep(100)
				return p
			}(),
			valid: false,
		},
		{
			name: "valid_shading",
			pattern: func() entity.Pattern {
				s := entity.NewShading(entity.ShadingAxial, "DeviceRGB")
				return entity.NewShadingPattern("test", s)
			}(),
			valid: true,
		},
		{
			name: "invalid_shading_no_shading",
			pattern: func() entity.Pattern {
				return entity.NewShadingPattern("test", nil)
			}(),
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pattern.ValidatePattern(tt.pattern)
			if tt.valid && err != nil {
				assert.Failf(t, "assertion failed", "Expected pattern to be valid, got error: %v", err)
			}
			if !tt.valid && err == nil {
				assert.Failf(t, "assertion failed", "Expected pattern to be invalid, got no error")
			}
		})
	}
}

// TestTransformPatternPoint tests pattern point transformation.
func TestTransformPatternPoint(t *testing.T) {
	// Create a pattern with identity matrix
	p := entity.NewTilingPattern("test", 1, entity.TilingConstantSpacing)
	matrix := [6]float64{1, 0, 0, 1, 0, 0}
	p.SetMatrix(matrix)

	// Test transformation
	x, y := pattern.TransformPatternPoint(p, 10, 20)
	if x != 10 || y != 20 {
		assert.Failf(t, "assertion failed", "Identity matrix: expected (10, 20), got (%f, %f)", x, y)
	}

	// Create a pattern with translation matrix
	p.SetMatrix([6]float64{1, 0, 0, 1, 5, 10})
	x, y = pattern.TransformPatternPoint(p, 10, 20)
	if x != 15 || y != 30 {
		assert.Failf(t, "assertion failed", "Translation matrix: expected (15, 30), got (%f, %f)", x, y)
	}

	// Create a pattern with scale matrix
	p.SetMatrix([6]float64{2, 0, 0, 2, 0, 0})
	x, y = pattern.TransformPatternPoint(p, 10, 20)
	if x != 20 || y != 40 {
		assert.Failf(t, "assertion failed", "Scale matrix: expected (20, 40), got (%f, %f)", x, y)
	}
}

// TestPatternTypeString tests pattern type string representation.
func TestPatternTypeString(t *testing.T) {
	tests := []struct {
		expected    string
		patternType entity.PatternType
	}{
		{"Tiling", entity.PatternTiling},
		{"Shading", entity.PatternShading},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.patternType.String() != tt.expected {
				assert.Failf(t, "assertion failed", "Expected %s, got %s", tt.expected, tt.patternType.String())
			}
		})
	}
}

// TestShadingTypeString tests shading type string representation.
func TestShadingTypeString(t *testing.T) {
	tests := []struct {
		expected    string
		shadingType entity.ShadingType
	}{
		{"FunctionBased", entity.ShadingFunctionBased},
		{"Axial", entity.ShadingAxial},
		{"Radial", entity.ShadingRadial},
		{"FreeFormGouraud", entity.ShadingFreeFormGouraud},
		{"LatticeGouraud", entity.ShadingLatticeGouraud},
		{"CoonsPatch", entity.ShadingCoonsPatch},
		{"TensorProductPatch", entity.ShadingTensorProductPatch},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.shadingType.String() != tt.expected {
				assert.Failf(t, "assertion failed", "Expected %s, got %s", tt.expected, tt.shadingType.String())
			}
		})
	}
}

// TestTilingTypeString tests tiling type string representation.
func TestTilingTypeString(t *testing.T) {
	tests := []struct {
		name       string
		tilingType entity.TilingType
	}{
		{"ConstantSpacing", entity.TilingConstantSpacing},
		{"NoDistortion", entity.TilingNoDistortion},
		{"ConstantSpacingFaster", entity.TilingConstantSpacingFaster},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify that the tiling type can be set and retrieved
			p := entity.NewTilingPattern("test", 1, tt.tilingType)
			if p.GetTilingType() != tt.tilingType {
				assert.Failf(t, "assertion failed", "Expected tiling type %v, got %v", tt.tilingType, p.GetTilingType())
			}
		})
	}
}

// TestShadingConstructors tests shading constructor functions.
func TestShadingConstructors(t *testing.T) {
	t.Run("NewAxialShading", func(t *testing.T) {
		s := entity.NewAxialShading("DeviceRGB", 0, 0, 100, 100, nil, [2]bool{true, false})
		if s.GetShadingType() != entity.ShadingAxial {
			assert.Failf(t, "assertion failed", "Expected ShadingAxial, got %v", s.GetShadingType())
		}
		if s.GetColorSpace() != "DeviceRGB" {
			assert.Failf(t, "assertion failed", "Expected DeviceRGB, got %s", s.GetColorSpace())
		}
	})

	t.Run("NewRadialShading", func(t *testing.T) {
		s := entity.NewRadialShading("DeviceRGB", 50, 50, 0, 50, 50, 50, nil, [2]bool{true, true})
		if s.GetShadingType() != entity.ShadingRadial {
			assert.Failf(t, "assertion failed", "Expected ShadingRadial, got %v", s.GetShadingType())
		}
	})

	t.Run("NewFunctionBasedShading", func(t *testing.T) {
		s := entity.NewFunctionBasedShading("DeviceRGB", [4]float64{0, 1, 0, 1}, [6]float64{1, 0, 0, 1, 0, 0}, nil)
		if s.GetShadingType() != entity.ShadingFunctionBased {
			assert.Failf(t, "assertion failed", "Expected ShadingFunctionBased, got %v", s.GetShadingType())
		}
	})

	t.Run("NewGouraudShading", func(t *testing.T) {
		vertices := []entity.Vertex{
			entity.NewVertex(0, 0, []float64{1, 0, 0}),
			entity.NewVertex(100, 0, []float64{0, 1, 0}),
			entity.NewVertex(50, 100, []float64{0, 0, 1}),
		}
		s := entity.NewGouraudShading("DeviceRGB", entity.ShadingFreeFormGouraud, vertices, 16, 8, []float64{0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1})
		if s.GetShadingType() != entity.ShadingFreeFormGouraud {
			assert.Failf(t, "assertion failed", "Expected ShadingFreeFormGouraud, got %v", s.GetShadingType())
		}
		if len(s.GetVertices()) != 3 {
			assert.Failf(t, "assertion failed", "Expected 3 vertices, got %d", len(s.GetVertices()))
		}
	})

	t.Run("NewPatchMeshShading", func(t *testing.T) {
		vertices := []entity.Vertex{
			entity.NewVertex(0, 0, []float64{1, 0, 0}),
			entity.NewVertex(100, 0, []float64{0, 1, 0}),
			entity.NewVertex(100, 100, []float64{0, 0, 1}),
			entity.NewVertex(0, 100, []float64{1, 1, 0}),
		}
		s := entity.NewPatchMeshShading("DeviceRGB", entity.ShadingCoonsPatch, vertices, 2, 16, 8, []float64{0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1})
		if s.GetShadingType() != entity.ShadingCoonsPatch {
			assert.Failf(t, "assertion failed", "Expected ShadingCoonsPatch, got %v", s.GetShadingType())
		}
	})
}

// TestVertexConstructor tests vertex constructor function.
func TestVertexConstructor(t *testing.T) {
	v := entity.NewVertex(10, 20, []float64{0.5, 0.7, 0.9})

	if v.X != 10 || v.Y != 20 {
		assert.Failf(t, "assertion failed", "Expected (10, 20), got (%f, %f)", v.X, v.Y)
	}

	if len(v.Colors) != 3 {
		require.FailNowf(t, "test failed", "Expected 3 color components, got %d", len(v.Colors))
	}

	if v.Colors[0] != 0.5 || v.Colors[1] != 0.7 || v.Colors[2] != 0.9 {
		assert.Failf(t, "assertion failed", "Expected [0.5, 0.7, 0.9], got %v", v.Colors)
	}
}

// Helper function to convert float array to entity Object array
func float64ToRealArray(values []float64) []entity.Object {
	result := make([]entity.Object, len(values))
	for i, v := range values {
		result[i] = entity.NewReal(v)
	}
	return result
}
