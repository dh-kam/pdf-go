package entity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSampledFunctionEvaluate_OneDimensionalInterpolation(t *testing.T) {
	fn := &SampledFunction{
		Domain:      [][2]float64{{0, 1}},
		RangeVal:    [][2]float64{{0, 1}},
		Size:        []int{2},
		Samples:     []float64{0, 1},
		Encode:      [][2]float64{{0, 1}},
		Decode:      [][2]float64{{0, 1}},
		Interpolate: true,
	}

	out, err := fn.Evaluate([]float64{0.25})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.InDelta(t, 0.25, out[0], 1e-6)
}

func TestSampledFunctionEvaluate_TwoDimensionalBilinear(t *testing.T) {
	fn := &SampledFunction{
		Domain:      [][2]float64{{0, 1}, {0, 1}},
		RangeVal:    [][2]float64{{0, 1}},
		Size:        []int{2, 2},
		Samples:     []float64{0, 1, 1, 0},
		Encode:      [][2]float64{{0, 1}, {0, 1}},
		Decode:      [][2]float64{{0, 1}},
		Interpolate: true,
	}

	out, err := fn.Evaluate([]float64{0.5, 0.5})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.InDelta(t, 0.5, out[0], 1e-6)
}

func TestSampledFunctionEvaluate_DecodeMapping(t *testing.T) {
	fn := &SampledFunction{
		Domain:      [][2]float64{{0, 1}},
		RangeVal:    [][2]float64{{10, 20}},
		Size:        []int{2},
		Samples:     []float64{0, 1},
		Encode:      [][2]float64{{0, 1}},
		Decode:      [][2]float64{{10, 20}},
		Interpolate: true,
	}

	out, err := fn.Evaluate([]float64{0.5})
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.InDelta(t, 15, out[0], 1e-6)
}

func TestSampledFunctionEvaluate_ValidateSampleLength(t *testing.T) {
	fn := &SampledFunction{
		Domain:      [][2]float64{{0, 1}},
		RangeVal:    [][2]float64{{0, 1}},
		Size:        []int{2},
		Samples:     []float64{0},
		Encode:      [][2]float64{{0, 1}},
		Decode:      [][2]float64{{0, 1}},
		Interpolate: true,
	}

	_, err := fn.Evaluate([]float64{0.5})
	require.Error(t, err)
}

func TestPattern_GetterSetterAndFunctionMetadata(t *testing.T) {
	tiling := NewTilingPattern("T1", 1, TilingConstantSpacing)
	assert.Equal(t, PatternTiling, tiling.Type())
	assert.Equal(t, "T1", tiling.Name())

	tiling.SetContent([]byte("q Q"))
	assert.Equal(t, []byte("q Q"), tiling.GetContent())
	assert.True(t, tiling.IsColored())
	assert.False(t, tiling.IsUncolored())
	tiling.SetPaintType(2)
	assert.True(t, tiling.IsUncolored())

	shading := NewShading(ShadingAxial, "DeviceRGB")
	shading.SetColorSpace("DeviceCMYK")
	assert.Equal(t, "DeviceCMYK", shading.GetColorSpace())
	shading.SetVertices([]Vertex{{X: 1, Y: 2}})
	require.Len(t, shading.GetVertices(), 1)

	shadingPattern := NewShadingPattern("S1", shading)
	assert.Equal(t, PatternShading, shadingPattern.Type())
	assert.Equal(t, "S1", shadingPattern.Name())
	assert.Equal(t, shading, shadingPattern.GetShading())
	shadingPattern.SetShading(nil)
	assert.Nil(t, shadingPattern.GetShading())

	sampled := &SampledFunction{
		Domain:   [][2]float64{{0, 1}},
		RangeVal: [][2]float64{{0, 1}, {0, 1}},
		Size:     []int{2, 3},
	}
	assert.Equal(t, 2, sampled.GetInputSize())
	assert.Equal(t, 2, sampled.GetOutputSize())
	assert.Equal(t, sampled.Domain, sampled.GetDomain())
	assert.Equal(t, sampled.RangeVal, sampled.GetRange())

	exp := &ExponentialFunction{
		Domain:   [][2]float64{{0, 1}},
		RangeVal: [][2]float64{{0, 1}},
		N:        3,
	}
	assert.Equal(t, 1, exp.GetInputSize())
	assert.Equal(t, 3, exp.GetOutputSize())
	assert.Equal(t, exp.Domain, exp.GetDomain())
	assert.Equal(t, exp.RangeVal, exp.GetRange())

	post := &PostScriptFunction{
		Domain:   [][2]float64{{0, 1}, {0, 1}},
		RangeVal: [][2]float64{{0, 1}},
	}
	assert.Equal(t, 2, post.GetInputSize())
	assert.Equal(t, 1, post.GetOutputSize())
	assert.Equal(t, post.Domain, post.GetDomain())
	assert.Equal(t, post.RangeVal, post.GetRange())

	stitch := &StitchingFunction{
		Domain:   [][2]float64{{0, 1}},
		RangeVal: [][2]float64{{0, 1}},
		Functions: []Function{
			exp,
		},
	}
	assert.Equal(t, 1, stitch.GetInputSize())
	assert.Equal(t, exp.GetOutputSize(), stitch.GetOutputSize())
	assert.Equal(t, stitch.Domain, stitch.GetDomain())
	assert.Equal(t, stitch.RangeVal, stitch.GetRange())
}
