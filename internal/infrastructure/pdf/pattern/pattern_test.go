// Package pattern tests for parser behavior and branch coverage.
package pattern

import (
	"errors"
	"image/color"
	"testing"

	"github.com/dh-kam/pdf-go/internal/domain/entity"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubXRef struct {
	objects map[entity.Ref]entity.Object
}

func (s *stubXRef) Fetch(ref entity.Ref) (entity.Object, error) {
	return s.objects[ref], nil
}

func TestParsePattern_Errors(t *testing.T) {
	_, err := ParsePattern(nil, nil, "nil")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pattern")

	dict := entity.NewDict()
	_, err = ParsePattern(dict, nil, "missing_type")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pattern_type")

	dict.Set(entity.NewName(string(KeyPatternType)), entity.NewString("invalid"))
	_, err = ParsePattern(dict, nil, "invalid_type")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pattern_type")

	dict.Set(entity.NewName(string(KeyPatternType)), entity.NewInteger(99))
	_, err = ParsePattern(dict, nil, "unknown_type")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown pattern type")
}

func TestParsePattern_DelegatesByType(t *testing.T) {
	t.Run("tiling", func(t *testing.T) {
		dict := entity.NewDict()
		dict.Set(entity.NewName(string(KeyPatternType)), entity.NewInteger(int64(entity.PatternTiling)))

		got, err := ParsePattern(dict, nil, "tiling")
		require.NoError(t, err)
		assert.Equal(t, entity.PatternTiling, got.Type())
		assert.Equal(t, "tiling", got.Name())
	})

	t.Run("shading_dict", func(t *testing.T) {
		shading := entity.NewDict()
		shading.Set(entity.NewName("ShadingType"), entity.NewInteger(int64(entity.ShadingAxial)))
		shading.Set(entity.NewName(string(KeyColorSpace)), entity.NewName("DeviceRGB"))
		shading.Set(entity.NewName(string(KeyCoords)), entity.NewArray(
			entity.NewReal(0),
			entity.NewReal(0),
			entity.NewReal(100),
			entity.NewReal(100),
		))
		fnDict := entity.NewDict()
		fnDict.Set(entity.NewName("FunctionType"), entity.NewInteger(2))
		shading.Set(entity.NewName("Function"), fnDict)

		patternDict := entity.NewDict()
		patternDict.Set(entity.NewName(string(KeyPatternType)), entity.NewInteger(int64(entity.PatternShading)))
		patternDict.Set(entity.NewName(string(KeyShading)), shading)

		got, err := ParsePattern(patternDict, nil, "shading")
		require.NoError(t, err)
		assert.Equal(t, entity.PatternShading, got.Type())
	})
}

func TestParseTilingPattern_ParsesOptionalProperties(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.NewName(string(KeyPatternType)), entity.NewInteger(int64(entity.PatternTiling)))
	dict.Set(entity.NewName(string(KeyPaintType)), entity.NewInteger(2))
	dict.Set(entity.NewName(string(KeyTilingType)), entity.NewInteger(int64(entity.TilingNoDistortion)))
	dict.Set(entity.NewName(string(KeyXStep)), entity.NewInteger(8))
	dict.Set(entity.NewName(string(KeyYStep)), entity.NewReal(9.5))
	dict.Set(entity.NewName(string(KeyBBox)), entity.NewArray(
		entity.NewReal(0),
		entity.NewReal(1),
		entity.NewReal(2),
		entity.NewReal(3),
	))
	dict.Set(entity.NewName(string(KeyMatrix)), entity.NewArray(
		entity.NewReal(1),
		entity.NewReal(0),
		entity.NewReal(0),
		entity.NewReal(1),
		entity.NewReal(10),
		entity.NewReal(20),
	))
	resources := entity.NewDict()
	dict.Set(entity.NewName(string(KeyResources)), resources)

	got, err := ParseTilingPattern(dict, nil, "pattern")
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, 2, got.GetPaintType())
	assert.Equal(t, entity.TilingNoDistortion, got.GetTilingType())
	assert.InDelta(t, 8, got.GetXStep(), 0.0001)
	assert.InDelta(t, 9.5, got.GetYStep(), 0.0001)
	assert.Equal(t, [4]float64{0, 1, 2, 3}, got.GetBBox())
	assert.Equal(t, [6]float64{1, 0, 0, 1, 10, 20}, got.Matrix())
	assert.Equal(t, resources, got.GetResources())
}

func TestParseShadingPattern_ResolvesShadingRefs(t *testing.T) {
	shading := entity.NewDict()
	shading.Set(entity.NewName("ShadingType"), entity.NewInteger(int64(entity.ShadingRadial)))
	shading.Set(entity.NewName(string(KeyColorSpace)), entity.NewName("DeviceRGB"))
	shading.Set(entity.NewName(string(KeyCoords)), entity.NewArray(
		entity.NewReal(1),
		entity.NewReal(2),
		entity.NewReal(3),
		entity.NewReal(4),
		entity.NewReal(5),
		entity.NewReal(6),
	))
	fnDict := entity.NewDict()
	fnDict.Set(entity.NewName("FunctionType"), entity.NewInteger(3))
	fnDict.Set(entity.NewName("Bounds"), entity.NewArray())
	fnDict.Set(entity.NewName("Encode"), entity.NewArray())
	shading.Set(entity.NewName("Function"), fnDict)

	dictByRef := entity.NewDict()
	dictByRef.Set(entity.NewName(string(KeyPatternType)), entity.NewInteger(int64(entity.PatternShading)))
	ref := entity.NewRef(1, 0)
	shadingStream := entity.NewStream(shading, []byte("shading-bytes"))
	xref := &stubXRef{
		objects: map[entity.Ref]entity.Object{
			ref: shading,
		},
	}

	t.Run("from_shading_dict", func(t *testing.T) {
		dictByRef.Set(entity.NewName(string(KeyShading)), shading)
		got, err := ParseShadingPattern(dictByRef, nil, "dict")
		require.NoError(t, err)
		assert.Equal(t, entity.ShadingRadial, got.GetShading().GetShadingType())
	})

	t.Run("from_ref", func(t *testing.T) {
		dictByRef.Set(entity.NewName(string(KeyShading)), ref)
		got, err := ParseShadingPattern(dictByRef, xref, "ref")
		require.NoError(t, err)
		assert.Equal(t, entity.ShadingRadial, got.GetShading().GetShadingType())
	})

	t.Run("from_stream", func(t *testing.T) {
		dictByRef.Set(entity.NewName(string(KeyShading)), shadingStream)
		got, err := ParseShadingPattern(dictByRef, nil, "stream")
		require.NoError(t, err)
		assert.Equal(t, entity.ShadingRadial, got.GetShading().GetShadingType())
	})
}

func TestParseShading_ParsesFunctionBasedShading(t *testing.T) {
	shadingDict := entity.NewDict()
	shadingDict.Set(entity.NewName("ShadingType"), entity.NewInteger(int64(entity.ShadingFunctionBased)))
	shadingDict.Set(entity.NewName(string(KeyColorSpace)), entity.NewName("DeviceRGB"))
	shadingDict.Set(entity.NewName(string(KeyDomain)), entity.NewArray(entity.NewReal(0), entity.NewReal(1), entity.NewReal(0), entity.NewReal(1)))
	shadingDict.Set(entity.NewName(string(KeyMatrixShading)), entity.NewArray(entity.NewInteger(1), entity.NewInteger(0), entity.NewInteger(0), entity.NewInteger(1), entity.NewInteger(10), entity.NewInteger(20)))
	shadingDict.Set(entity.NewName(string(KeyBackground)), entity.NewArray(entity.NewReal(0.2), entity.NewReal(0.3), entity.NewReal(0.4)))
	fnDict := entity.NewDict()
	fnDict.Set(entity.NewName("FunctionType"), entity.NewInteger(0))
	fnDict.Set(entity.NewName("Domain"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	fnDict.Set(entity.NewName("Range"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	fnDict.Set(entity.NewName("Size"), entity.NewArray(entity.NewInteger(2)))
	shadingDict.Set(entity.NewName(string(KeyFunction)), fnDict)

	functionStreamDict := entity.NewDict()
	functionStreamDict.Set(entity.NewName("FunctionType"), entity.NewInteger(0))
	functionStreamDict.Set(entity.NewName("Domain"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	functionStreamDict.Set(entity.NewName("Range"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
	functionStreamDict.Set(entity.NewName("Size"), entity.NewArray(entity.NewInteger(2)))
	shadingStreamData := []byte{0x00, 0x40, 0x80, 0xFF}
	shadingDict.Set(entity.NewName(string(KeyFunction)),
		entity.NewStream(functionStreamDict, shadingStreamData),
	)

	got, err := ParseShading(shadingDict, nil)
	require.NoError(t, err)
	require.NotNil(t, got)

	shading := got
	assert.Equal(t, entity.ShadingFunctionBased, shading.GetShadingType())
	assert.Equal(t, "DeviceRGB", shading.GetColorSpace())
	assert.Len(t, got.GetFunctions(), 1)
	assert.Equal(t, [6]float64{1, 0, 0, 1, 10, 20}, got.GetMatrix())
	assert.NotNil(t, got.GetBackground())
	assert.InDelta(t, 0, got.GetDomain()[0], 0.0001)
	background, ok := got.GetBackground().(color.RGBA)
	require.True(t, ok)
	assert.Equal(t, uint8(51), background.R)
	assert.Equal(t, uint8(77), background.G)
	assert.Equal(t, uint8(102), background.B)
}

func TestParseShading_ParsesAxialAndRadialDefaults(t *testing.T) {
	t.Run("axial", func(t *testing.T) {
		shadingDict := entity.NewDict()
		shadingDict.Set(entity.NewName("ShadingType"), entity.NewInteger(int64(entity.ShadingAxial)))
		shadingDict.Set(entity.NewName(string(KeyColorSpace)), entity.NewName("DeviceRGB"))
		shadingDict.Set(entity.NewName(string(KeyCoords)), entity.NewArray(
			entity.NewReal(0),
			entity.NewReal(1),
			entity.NewReal(2),
			entity.NewReal(3),
		))
		shadingDict.Set(entity.NewName(string(KeyExtend)), entity.NewArray(entity.NewBoolean(true), entity.NewBoolean(false)))
		axialFunctionStreamDict := entity.NewDict()
		axialFunctionStreamDict.Set(entity.NewName("FunctionType"), entity.NewInteger(4))
		shadingDict.Set(entity.NewName("Function"), entity.NewStream(axialFunctionStreamDict, []byte{1, 2, 3}))

		got, err := ParseShading(shadingDict, nil)
		require.NoError(t, err)
		assert.Equal(t, entity.ShadingAxial, got.GetShadingType())
		assert.Equal(t, []float64{0, 1, 2, 3}, got.GetCoords())
		assert.Equal(t, [2]bool{true, false}, got.GetExtend())
		assert.Len(t, got.GetFunctions(), 1)
	})

	t.Run("radial", func(t *testing.T) {
		shadingDict := entity.NewDict()
		shadingDict.Set(entity.NewName("ShadingType"), entity.NewInteger(int64(entity.ShadingRadial)))
		shadingDict.Set(entity.NewName(string(KeyColorSpace)), entity.NewName("DeviceRGB"))
		shadingDict.Set(entity.NewName(string(KeyCoords)), entity.NewArray(
			entity.NewReal(0),
			entity.NewReal(1),
			entity.NewReal(2),
			entity.NewReal(3),
			entity.NewReal(4),
			entity.NewReal(5),
		))
		shadingDict.Set(entity.NewName(string(KeyExtend)), entity.NewArray(entity.NewBoolean(false), entity.NewBoolean(true)))
		radialFunctionStreamDict := entity.NewDict()
		radialFunctionStreamDict.Set(entity.NewName("FunctionType"), entity.NewInteger(4))
		shadingDict.Set(entity.NewName("Function"), entity.NewStream(radialFunctionStreamDict, []byte{1, 2, 3}))

		got, err := ParseShading(shadingDict, nil)
		require.NoError(t, err)
		assert.Equal(t, entity.ShadingRadial, got.GetShadingType())
		assert.Equal(t, []float64{0, 1, 2, 3, 4, 5}, got.GetCoords())
		assert.Equal(t, [2]bool{false, true}, got.GetExtend())
	})
}

func TestParseShading_GouraudAndPatchPaths(t *testing.T) {
	t.Run("gouraud", func(t *testing.T) {
		shadingDict := entity.NewDict()
		shadingDict.Set(entity.NewName("ShadingType"), entity.NewInteger(int64(entity.ShadingFreeFormGouraud)))
		shadingDict.Set(entity.NewName(string(KeyColorSpace)), entity.NewName("DeviceRGB"))
		shadingDict.Set(entity.NewName(string(KeyBitsPerCoordinate)), entity.NewInteger(8))
		shadingDict.Set(entity.NewName(string(KeyBitsPerComponent)), entity.NewInteger(8))
		shadingDict.Set(entity.NewName(string(KeyDecode)), entity.NewArray(entity.NewReal(0), entity.NewReal(1), entity.NewReal(0), entity.NewReal(1)))

		got, err := ParseShading(shadingDict, nil)
		require.NoError(t, err)
		assert.Equal(t, entity.ShadingFreeFormGouraud, got.GetShadingType())
		assert.Equal(t, 8, got.GetBitsPerCoord())
		assert.Equal(t, 8, got.GetBitsPerComp())
		assert.Equal(t, []float64{0, 1, 0, 1}, got.GetDecode())
	})

	t.Run("patch_mesh", func(t *testing.T) {
		shadingDict := entity.NewDict()
		shadingDict.Set(entity.NewName("ShadingType"), entity.NewInteger(int64(entity.ShadingTensorProductPatch)))
		shadingDict.Set(entity.NewName(string(KeyColorSpace)), entity.NewName("DeviceRGB"))
		shadingDict.Set(entity.NewName(string(KeyBitsPerFlag)), entity.NewInteger(1))
		shadingDict.Set(entity.NewName(string(KeyBitsPerCoordinate)), entity.NewInteger(4))
		shadingDict.Set(entity.NewName(string(KeyBitsPerComponent)), entity.NewInteger(6))
		shadingDict.Set(entity.NewName(string(KeyDecode)), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))

		got, err := ParseShading(shadingDict, nil)
		require.NoError(t, err)
		assert.Equal(t, entity.ShadingTensorProductPatch, got.GetShadingType())
		assert.Equal(t, 1, got.GetBitsPerFlag())
		assert.Equal(t, 4, got.GetBitsPerCoord())
		assert.Equal(t, 6, got.GetBitsPerComp())
		assert.Equal(t, []float64{0, 1}, got.GetDecode())
	})
}

func TestParseFunctionHelpers(t *testing.T) {
	t.Run("parse_function_nil", func(t *testing.T) {
		fn, err := parseFunction(nil)
		require.Error(t, err)
		assert.Nil(t, fn)
	})

	t.Run("parse_sampled_function_from_dict", func(t *testing.T) {
		fnDict := entity.NewDict()
		fnDict.Set(entity.NewName("FunctionType"), entity.NewInteger(0))
		fnDict.Set(entity.NewName("Domain"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
		fnDict.Set(entity.NewName("Range"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
		fnDict.Set(entity.NewName("Size"), entity.NewArray(entity.NewInteger(2)))

		fn, err := parseFunction(fnDict)
		require.NoError(t, err)
		require.NotNil(t, fn)
		assert.IsType(t, &entity.SampledFunction{}, fn)
	})

	t.Run("parse_exponential_function", func(t *testing.T) {
		fnDict := entity.NewDict()
		fnDict.Set(entity.NewName("FunctionType"), entity.NewInteger(2))
		fnDict.Set(entity.NewName("N"), entity.NewReal(2.0))
		fn, err := parseFunction(fnDict)
		require.NoError(t, err)
		exponential, ok := fn.(*entity.ExponentialFunction)
		require.True(t, ok)
		assert.Equal(t, []float64{0}, exponential.C0)
		assert.Equal(t, []float64{1}, exponential.C1)
		assert.InDelta(t, 2.0, exponential.Exponent, 1e-9)
	})

	t.Run("parse_stitching_function", func(t *testing.T) {
		fnDict := entity.NewDict()
		fnDict.Set(entity.NewName("FunctionType"), entity.NewInteger(3))
		fnDict.Set(entity.NewName("Domain"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
		fnDict.Set(entity.NewName("Range"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))
		fnDict.Set(entity.NewName("Bounds"), entity.NewArray(entity.NewReal(0.5)))
		fnDict.Set(entity.NewName("Encode"), entity.NewArray(entity.NewReal(0), entity.NewReal(1), entity.NewReal(0), entity.NewReal(1)))
		fnSubDict := entity.NewDict()
		fnSubDict.Set(entity.NewName("FunctionType"), entity.NewInteger(4))
		fnDict.Set(entity.NewName("Functions"), entity.NewArray(fnSubDict))
		fn, err := parseFunction(fnDict)
		require.NoError(t, err)
		assert.IsType(t, &entity.StitchingFunction{}, fn)
	})

	t.Run("parse_postscript_function", func(t *testing.T) {
		fnDict := entity.NewDict()
		fnDict.Set(entity.NewName("FunctionType"), entity.NewInteger(4))
		fnDict.Set(entity.NewName("Range"), entity.NewArray(entity.NewReal(0), entity.NewReal(1)))

		stream := entity.NewStream(fnDict, []byte("12"))
		fnObj, err := parseFunction(stream)
		require.NoError(t, err)
		ps, ok := fnObj.(*entity.PostScriptFunction)
		require.True(t, ok)
		assert.Equal(t, "12", ps.Program)
	})

	t.Run("parse_function_unsupported_type", func(t *testing.T) {
		fnDict := entity.NewDict()
		fnDict.Set(entity.NewName("FunctionType"), entity.NewInteger(99))
		_, err := parseFunction(fnDict)
		require.Error(t, err)
	})

	t.Run("parse_function_array", func(t *testing.T) {
		fnSubDict := entity.NewDict()
		fnSubDict.Set(entity.NewName("FunctionType"), entity.NewInteger(4))
		fnSubStreamDict := entity.NewDict()
		fnSubStreamDict.Set(entity.NewName("FunctionType"), entity.NewInteger(4))
		arr := entity.NewArray(
			fnSubDict,
			entity.NewStream(fnSubStreamDict, []byte("99")),
		)
		functions, err := parseFunctionArray(arr)
		require.NoError(t, err)
		require.Len(t, functions, 2)

		_, err = parseFunctionArray(entity.NewInteger(1))
		require.Error(t, err)
	})

	t.Run("array_parsers", func(t *testing.T) {
		arr := entity.NewArray(entity.NewInteger(1), entity.NewInteger(2), entity.NewReal(3.5))
		values, err := parseFloatArray(arr, 3)
		require.NoError(t, err)
		assert.Equal(t, []float64{1, 2, 3.5}, values)

		_, err = parseFloatArray(entity.NewDict(), 0)
		require.Error(t, err)

		ints, err := parseIntArray(arr)
		require.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3}, ints)

		boolArr := entity.NewArray(entity.NewBoolean(true), entity.NewBoolean(false))
		ext, err := parseBoolArray(boolArr, 2)
		require.NoError(t, err)
		assert.True(t, ext[0])
		assert.False(t, ext[1])

		invalidExt, err := parseBoolArray(entity.NewArray(entity.NewBoolean(true)), 2)
		require.Error(t, err)
		assert.Equal(t, [2]bool{}, invalidExt)
	})
}

func TestPatternUtilityHelpers(t *testing.T) {
	t.Run("parse_sample_values", func(t *testing.T) {
		values := parseSampleValues([]byte{0x00, 0x80, 0xFF})
		assert.InDeltaSlice(t, []float64{0.0, 128.0 / 255.0, 1.0}, values, 0.0001)
	})

	t.Run("background_color", func(t *testing.T) {
		gray, err := parseBackgroundColor(entity.NewArray(entity.NewReal(1.2)), "DeviceGray")
		require.NoError(t, err)
		colorGray, ok := gray.(color.Gray)
		require.True(t, ok)
		assert.Equal(t, uint8(255), colorGray.Y)

		rgb, err := parseBackgroundColor(entity.NewArray(entity.NewReal(1), entity.NewReal(0), entity.NewReal(0.5)), "DeviceRGB")
		require.NoError(t, err)
		rgbColor, ok := rgb.(color.RGBA)
		require.True(t, ok)
		r, g, b, _ := rgbColor.R, rgbColor.G, rgbColor.B, rgbColor.A
		assert.Equal(t, uint8(255), r)
		assert.Equal(t, uint8(0), g)
		assert.Equal(t, uint8(128), b)

		cmyk, err := parseBackgroundColor(entity.NewArray(entity.NewReal(0), entity.NewReal(0), entity.NewReal(0), entity.NewReal(1)), "DeviceCMYK")
		require.NoError(t, err)
		cmykColor, ok := cmyk.(color.RGBA)
		require.True(t, ok)
		assert.Equal(t, uint8(0), cmykColor.R)
		assert.Equal(t, uint8(0), cmykColor.G)
		assert.Equal(t, uint8(0), cmykColor.B)

		_, err = parseBackgroundColor(entity.NewArray(entity.NewReal(1)), "UnknownSpace")
		require.Error(t, err)
	})

	t.Run("normalize_color_space", func(t *testing.T) {
		assert.Equal(t, "DeviceRGB", normalizeColorSpaceName("/DeviceRGB"))
		assert.Equal(t, "DeviceGray", normalizeColorSpaceName("G"))
		assert.Equal(t, "DeviceCMYK", normalizeColorSpaceName("/DeviceCMYK"))
		assert.Equal(t, "DeviceRGB", normalizeColorSpaceName("foo"))
	})

	t.Run("clamp01", func(t *testing.T) {
		assert.InDelta(t, 0.0, clamp01(-0.1), 1e-12)
		assert.InDelta(t, 0.0, clamp01(0), 1e-12)
		assert.InDelta(t, 1.0, clamp01(1.2), 1e-12)
	})

	t.Run("gradient_points", func(t *testing.T) {
		shading := entity.NewShading(entity.ShadingAxial, "DeviceRGB")
		shading.SetCoords([]float64{1, 2, 3, 4})
		x0, y0, x1, y1, err := ParseAxialGradientPoints(shading)
		require.NoError(t, err)
		assert.Equal(t, []float64{1, 2, 3, 4}, []float64{x0, y0, x1, y1})

		shading.SetShadingType(entity.ShadingRadial)
		shading.SetCoords([]float64{1, 2, 3, 4, 5, 6})
		rx0, ry0, rr0, rx1, ry1, rr1, err := ParseRadialGradientPoints(shading)
		require.NoError(t, err)
		assert.Equal(t, []float64{1, 2, 3, 4, 5, 6}, []float64{rx0, ry0, rr0, rx1, ry1, rr1})

		shading.SetCoords([]float64{1, 2, 3})
		_, _, _, _, err = ParseAxialGradientPoints(shading)
		require.Error(t, err)
	})
}

func TestValidateAndTransformPattern(t *testing.T) {
	t.Run("validate_tiling", func(t *testing.T) {
		p := entity.NewTilingPattern("tile", 1, entity.TilingConstantSpacing)
		p.SetXStep(10)
		p.SetYStep(0)
		assert.EqualError(t, ValidatePattern(p), errors.New("invalid tiling pattern step size").Error())

		p.SetYStep(10)
		assert.NoError(t, ValidatePattern(p))
	})
}

func TestTransformPatternPoint(t *testing.T) {
	p := entity.NewShadingPattern("shade", entity.NewShading(entity.ShadingAxial, "DeviceRGB"))
	x, y := TransformPatternPoint(p, 1, 2)
	assert.InDelta(t, 1, x, 0.0001)
	assert.InDelta(t, 2, y, 0.0001)

	p.SetMatrix([6]float64{1, 0, 0, 1, 10, 20})
	x, y = TransformPatternPoint(p, 1, 2)
	assert.InDelta(t, 11, x, 0.0001)
	assert.InDelta(t, 22, y, 0.0001)
}
