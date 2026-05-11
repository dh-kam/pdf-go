package function

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/function"
)

func TestNewExponentialFunction_Valid(t *testing.T) {
	domain := []float64{0.0, 1.0}
	c0 := []float64{0.0}
	c1 := []float64{1.0}
	n := 1.0

	fn, err := NewExponentialFunction(domain, nil, c0, c1, n)
	require.NoError(t, err)
	assert.NotNil(t, fn)
	assert.Equal(t, function.TypeExponential, fn.Type())
	assert.Equal(t, 1, fn.InputSize())
	assert.Equal(t, 1, fn.OutputSize())
}

func TestNewExponentialFunction_InvalidDomain(t *testing.T) {
	tests := []struct {
		name   string
		domain []float64
	}{
		{"empty", []float64{}},
		{"single value", []float64{0.0}},
		{"too many values", []float64{0.0, 0.5, 1.0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewExponentialFunction(tt.domain, nil, []float64{0.0}, []float64{1.0}, 1.0)
			assert.Error(t, err)
		})
	}
}

func TestNewExponentialFunction_InvalidC0C1(t *testing.T) {
	domain := []float64{0.0, 1.0}

	tests := []struct {
		name string
		c0   []float64
		c1   []float64
	}{
		{"empty c0", []float64{}, []float64{1.0}},
		{"mismatched lengths", []float64{0.0}, []float64{1.0, 2.0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewExponentialFunction(domain, nil, tt.c0, tt.c1, 1.0)
			assert.Error(t, err)
		})
	}
}

func TestNewExponentialFunction_InvalidRange(t *testing.T) {
	domain := []float64{0.0, 1.0}
	c0 := []float64{0.0, 0.0}
	c1 := []float64{1.0, 1.0}
	invalidRange := []float64{0.0, 1.0, 0.0} // Should be 4 values for 2 outputs

	_, err := NewExponentialFunction(domain, invalidRange, c0, c1, 1.0)
	assert.Error(t, err)
}

func TestNewExponentialFunction_NegativeExponent(t *testing.T) {
	domain := []float64{0.0, 1.0}
	c0 := []float64{0.0}
	c1 := []float64{1.0}

	_, err := NewExponentialFunction(domain, nil, c0, c1, -1.0)
	assert.Error(t, err)
}

func TestExponentialFunction_LinearInterpolation(t *testing.T) {
	// N=1 means linear interpolation
	domain := []float64{0.0, 1.0}
	c0 := []float64{0.0}
	c1 := []float64{1.0}
	n := 1.0

	fn, err := NewExponentialFunction(domain, nil, c0, c1, n)
	require.NoError(t, err)

	tests := []struct {
		input    float64
		expected float64
	}{
		{0.0, 0.0},
		{0.25, 0.25},
		{0.5, 0.5},
		{0.75, 0.75},
		{1.0, 1.0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("x=%.2f", tt.input), func(t *testing.T) {
			output, err := fn.Evaluate([]float64{tt.input})
			require.NoError(t, err)
			assert.InDelta(t, tt.expected, output[0], 1e-10)
		})
	}
}

func TestExponentialFunction_SquaredInterpolation(t *testing.T) {
	// N=2 means x^2 interpolation
	domain := []float64{0.0, 1.0}
	c0 := []float64{0.0}
	c1 := []float64{1.0}
	n := 2.0

	fn, err := NewExponentialFunction(domain, nil, c0, c1, n)
	require.NoError(t, err)

	tests := []struct {
		input    float64
		expected float64
	}{
		{0.0, 0.0},
		{0.5, 0.25}, // 0.5^2 = 0.25
		{1.0, 1.0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("x=%.2f", tt.input), func(t *testing.T) {
			output, err := fn.Evaluate([]float64{tt.input})
			require.NoError(t, err)
			assert.InDelta(t, tt.expected, output[0], 1e-10)
		})
	}
}

func TestExponentialFunction_DomainClipping(t *testing.T) {
	domain := []float64{0.0, 1.0}
	c0 := []float64{0.0}
	c1 := []float64{1.0}
	n := 1.0

	fn, err := NewExponentialFunction(domain, nil, c0, c1, n)
	require.NoError(t, err)

	// Values outside domain should be clipped
	tests := []struct {
		input    float64
		expected float64
	}{
		{-1.0, 0.0}, // Clipped to domain min
		{2.0, 1.0},  // Clipped to domain max
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("x=%.2f", tt.input), func(t *testing.T) {
			output, err := fn.Evaluate([]float64{tt.input})
			require.NoError(t, err)
			assert.InDelta(t, tt.expected, output[0], 1e-10)
		})
	}
}

func TestExponentialFunction_RangeClipping(t *testing.T) {
	domain := []float64{0.0, 1.0}
	rng := []float64{0.2, 0.8} // Clip output to [0.2, 0.8]
	c0 := []float64{0.0}
	c1 := []float64{1.0}
	n := 1.0

	fn, err := NewExponentialFunction(domain, rng, c0, c1, n)
	require.NoError(t, err)

	tests := []struct {
		input    float64
		expected float64
	}{
		{0.0, 0.2}, // Would be 0.0, clipped to 0.2
		{0.5, 0.5}, // Within range
		{1.0, 0.8}, // Would be 1.0, clipped to 0.8
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("x=%.2f", tt.input), func(t *testing.T) {
			output, err := fn.Evaluate([]float64{tt.input})
			require.NoError(t, err)
			assert.InDelta(t, tt.expected, output[0], 1e-10)
		})
	}
}

func TestExponentialFunction_MultiDimensionalOutput(t *testing.T) {
	// RGB gradient from black (0,0,0) to white (1,1,1)
	domain := []float64{0.0, 1.0}
	c0 := []float64{0.0, 0.0, 0.0}
	c1 := []float64{1.0, 1.0, 1.0}
	n := 1.0

	fn, err := NewExponentialFunction(domain, nil, c0, c1, n)
	require.NoError(t, err)

	assert.Equal(t, 3, fn.OutputSize())

	// Test midpoint
	output, err := fn.Evaluate([]float64{0.5})
	require.NoError(t, err)
	assert.Len(t, output, 3)
	assert.InDelta(t, 0.5, output[0], 1e-10)
	assert.InDelta(t, 0.5, output[1], 1e-10)
	assert.InDelta(t, 0.5, output[2], 1e-10)
}

func TestExponentialFunction_NonStandardDomain(t *testing.T) {
	// Poppler clamps to the function domain but does not normalize x to [0, 1].
	domain := []float64{10.0, 20.0}
	c0 := []float64{0.0}
	c1 := []float64{100.0}
	n := 1.0

	fn, err := NewExponentialFunction(domain, nil, c0, c1, n)
	require.NoError(t, err)

	tests := []struct {
		input    float64
		expected float64
	}{
		{10.0, 1000.0},
		{15.0, 1500.0},
		{20.0, 2000.0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("x=%.2f", tt.input), func(t *testing.T) {
			output, err := fn.Evaluate([]float64{tt.input})
			require.NoError(t, err)
			assert.InDelta(t, tt.expected, output[0], 1e-10)
		})
	}
}

func TestExponentialFunction_ZeroExponent(t *testing.T) {
	// N=0 means x^0 = 1, so output = C0 + 1 * (C1 - C0) = C1
	domain := []float64{0.0, 1.0}
	c0 := []float64{0.5}
	c1 := []float64{1.0}
	n := 0.0

	fn, err := NewExponentialFunction(domain, nil, c0, c1, n)
	require.NoError(t, err)

	// All inputs should produce C1 (since x^0 = 1)
	for x := 0.0; x <= 1.0; x += 0.25 {
		output, err := fn.Evaluate([]float64{x})
		require.NoError(t, err)
		assert.InDelta(t, 1.0, output[0], 1e-10)
	}
}

func TestExponentialFunction_InvalidInput(t *testing.T) {
	domain := []float64{0.0, 1.0}
	c0 := []float64{0.0}
	c1 := []float64{1.0}
	n := 1.0

	fn, err := NewExponentialFunction(domain, nil, c0, c1, n)
	require.NoError(t, err)

	// Too many inputs
	_, err = fn.Evaluate([]float64{0.5, 0.5})
	assert.Error(t, err)

	// No inputs
	_, err = fn.Evaluate([]float64{})
	assert.Error(t, err)
}

func TestClamp(t *testing.T) {
	tests := []struct {
		value    float64
		min      float64
		max      float64
		expected float64
	}{
		{0.5, 0.0, 1.0, 0.5},  // Within range
		{-1.0, 0.0, 1.0, 0.0}, // Below min
		{2.0, 0.0, 1.0, 1.0},  // Above max
		{0.0, 0.0, 1.0, 0.0},  // At min
		{1.0, 0.0, 1.0, 1.0},  // At max
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("clamp(%.1f,%.1f,%.1f)", tt.value, tt.min, tt.max), func(t *testing.T) {
			result := clamp(tt.value, tt.min, tt.max)
			assert.Equal(t, tt.expected, result)
		})
	}
}
