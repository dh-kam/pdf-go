package function

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/function"
)

func TestNewStitchedFunction_Valid(t *testing.T) {
	domain := []float64{0.0, 1.0}

	// Create two simple linear functions
	fn1, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0}, []float64{0.5}, 1.0)
	fn2, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.5}, []float64{1.0}, 1.0)

	functions := []function.Function{fn1, fn2}
	bounds := []float64{0.5}
	encode := []float64{0.0, 1.0, 0.0, 1.0}

	stitched, err := NewStitchedFunction(domain, nil, functions, bounds, encode)
	require.NoError(t, err)
	assert.NotNil(t, stitched)
	assert.Equal(t, function.TypeStitched, stitched.Type())
	assert.Equal(t, 1, stitched.InputSize())
	assert.Equal(t, 1, stitched.OutputSize())
}

func TestNewStitchedFunction_InvalidDomain(t *testing.T) {
	fn1, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0}, []float64{1.0}, 1.0)
	functions := []function.Function{fn1}

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
			_, err := NewStitchedFunction(tt.domain, nil, functions, []float64{}, []float64{0.0, 1.0})
			assert.Error(t, err)
		})
	}
}

func TestNewStitchedFunction_EmptyFunctions(t *testing.T) {
	domain := []float64{0.0, 1.0}
	_, err := NewStitchedFunction(domain, nil, []function.Function{}, []float64{}, []float64{})
	assert.Error(t, err)
}

func TestNewStitchedFunction_InvalidBounds(t *testing.T) {
	domain := []float64{0.0, 1.0}
	fn1, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0}, []float64{1.0}, 1.0)
	fn2, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0}, []float64{1.0}, 1.0)
	functions := []function.Function{fn1, fn2}

	// Bounds length should be len(functions) - 1 = 1
	_, err := NewStitchedFunction(domain, nil, functions, []float64{}, []float64{0.0, 1.0, 0.0, 1.0})
	assert.Error(t, err)

	_, err = NewStitchedFunction(domain, nil, functions, []float64{0.5, 0.7}, []float64{0.0, 1.0, 0.0, 1.0})
	assert.Error(t, err)
}

func TestNewStitchedFunction_InvalidEncode(t *testing.T) {
	domain := []float64{0.0, 1.0}
	fn1, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0}, []float64{1.0}, 1.0)
	fn2, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0}, []float64{1.0}, 1.0)
	functions := []function.Function{fn1, fn2}
	bounds := []float64{0.5}

	// Encode length should be 2 * len(functions) = 4
	_, err := NewStitchedFunction(domain, nil, functions, bounds, []float64{0.0, 1.0})
	assert.Error(t, err)

	_, err = NewStitchedFunction(domain, nil, functions, bounds, []float64{0.0, 1.0, 0.0, 1.0, 0.0, 1.0})
	assert.Error(t, err)
}

func TestNewStitchedFunction_MismatchedFunctionSizes(t *testing.T) {
	domain := []float64{0.0, 1.0}

	// fn1 has 1 output, fn2 has 2 outputs
	fn1, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0}, []float64{1.0}, 1.0)
	fn2, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0, 0.0}, []float64{1.0, 1.0}, 1.0)

	functions := []function.Function{fn1, fn2}
	bounds := []float64{0.5}
	encode := []float64{0.0, 1.0, 0.0, 1.0}

	_, err := NewStitchedFunction(domain, nil, functions, bounds, encode)
	assert.Error(t, err)
}

func TestStitchedFunction_TwoFunctions(t *testing.T) {
	domain := []float64{0.0, 1.0}

	// First half: 0.0 -> 0.5
	fn1, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0}, []float64{0.5}, 1.0)
	// Second half: 0.5 -> 1.0
	fn2, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.5}, []float64{1.0}, 1.0)

	functions := []function.Function{fn1, fn2}
	bounds := []float64{0.5}
	encode := []float64{0.0, 1.0, 0.0, 1.0}

	stitched, err := NewStitchedFunction(domain, nil, functions, bounds, encode)
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
			output, err := stitched.Evaluate([]float64{tt.input})
			require.NoError(t, err)
			assert.InDelta(t, tt.expected, output[0], 1e-10)
		})
	}
}

func TestStitchedFunction_ThreeFunctions(t *testing.T) {
	domain := []float64{0.0, 1.0}

	// [0.0, 0.3]: 0.0 -> 0.3
	fn1, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0}, []float64{0.3}, 1.0)
	// [0.3, 0.7]: 0.3 -> 0.7
	fn2, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.3}, []float64{0.7}, 1.0)
	// [0.7, 1.0]: 0.7 -> 1.0
	fn3, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.7}, []float64{1.0}, 1.0)

	functions := []function.Function{fn1, fn2, fn3}
	bounds := []float64{0.3, 0.7}
	encode := []float64{0.0, 1.0, 0.0, 1.0, 0.0, 1.0}

	stitched, err := NewStitchedFunction(domain, nil, functions, bounds, encode)
	require.NoError(t, err)

	tests := []struct {
		input    float64
		expected float64
	}{
		{0.0, 0.0},
		{0.15, 0.15},
		{0.3, 0.3},
		{0.5, 0.5},
		{0.7, 0.7},
		{0.85, 0.85},
		{1.0, 1.0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("x=%.2f", tt.input), func(t *testing.T) {
			output, err := stitched.Evaluate([]float64{tt.input})
			require.NoError(t, err)
			assert.InDelta(t, tt.expected, output[0], 1e-10)
		})
	}
}

func TestStitchedFunction_DomainClipping(t *testing.T) {
	domain := []float64{0.0, 1.0}
	fn1, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0}, []float64{0.5}, 1.0)
	fn2, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.5}, []float64{1.0}, 1.0)

	functions := []function.Function{fn1, fn2}
	bounds := []float64{0.5}
	encode := []float64{0.0, 1.0, 0.0, 1.0}

	stitched, err := NewStitchedFunction(domain, nil, functions, bounds, encode)
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
			output, err := stitched.Evaluate([]float64{tt.input})
			require.NoError(t, err)
			assert.InDelta(t, tt.expected, output[0], 1e-10)
		})
	}
}

func TestStitchedFunction_RangeClipping(t *testing.T) {
	domain := []float64{0.0, 1.0}
	rng := []float64{0.2, 0.8} // Clip output to [0.2, 0.8]

	fn1, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0}, []float64{0.5}, 1.0)
	fn2, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.5}, []float64{1.0}, 1.0)

	functions := []function.Function{fn1, fn2}
	bounds := []float64{0.5}
	encode := []float64{0.0, 1.0, 0.0, 1.0}

	stitched, err := NewStitchedFunction(domain, rng, functions, bounds, encode)
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
			output, err := stitched.Evaluate([]float64{tt.input})
			require.NoError(t, err)
			assert.InDelta(t, tt.expected, output[0], 1e-10)
		})
	}
}

func TestStitchedFunction_NonLinearEncoding(t *testing.T) {
	domain := []float64{0.0, 1.0}

	// First function maps [0, 1] -> [0, 1]
	fn1, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0}, []float64{1.0}, 1.0)
	// Second function maps [0, 1] -> [0, 1]
	fn2, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0}, []float64{1.0}, 1.0)

	functions := []function.Function{fn1, fn2}
	bounds := []float64{0.5}
	// Encode first function to [0, 0.5], second to [0.5, 1.0]
	encode := []float64{0.0, 0.5, 0.5, 1.0}

	stitched, err := NewStitchedFunction(domain, nil, functions, bounds, encode)
	require.NoError(t, err)

	// At x=0.25 (in first interval [0, 0.5]):
	// Normalized in interval: (0.25 - 0) / (0.5 - 0) = 0.5
	// Encoded: 0.0 + 0.5 * (0.5 - 0.0) = 0.25
	// fn1 evaluates: 0.25
	output, err := stitched.Evaluate([]float64{0.25})
	require.NoError(t, err)
	assert.InDelta(t, 0.25, output[0], 1e-10)

	// At x=0.75 (in second interval [0.5, 1.0]):
	// Normalized in interval: (0.75 - 0.5) / (1.0 - 0.5) = 0.5
	// Encoded: 0.5 + 0.5 * (1.0 - 0.5) = 0.75
	// fn2 evaluates: 0.75
	output, err = stitched.Evaluate([]float64{0.75})
	require.NoError(t, err)
	assert.InDelta(t, 0.75, output[0], 1e-10)
}

func TestStitchedFunction_MultiDimensionalOutput(t *testing.T) {
	domain := []float64{0.0, 1.0}

	// RGB gradient: first half black->red, second half red->white
	fn1, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0, 0.0, 0.0}, []float64{1.0, 0.0, 0.0}, 1.0)
	fn2, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{1.0, 0.0, 0.0}, []float64{1.0, 1.0, 1.0}, 1.0)

	functions := []function.Function{fn1, fn2}
	bounds := []float64{0.5}
	encode := []float64{0.0, 1.0, 0.0, 1.0}

	stitched, err := NewStitchedFunction(domain, nil, functions, bounds, encode)
	require.NoError(t, err)

	assert.Equal(t, 3, stitched.OutputSize())

	// At x=0.25 (first interval): [0.5, 0, 0]
	output, err := stitched.Evaluate([]float64{0.25})
	require.NoError(t, err)
	assert.Len(t, output, 3)
	assert.InDelta(t, 0.5, output[0], 1e-10)
	assert.InDelta(t, 0.0, output[1], 1e-10)
	assert.InDelta(t, 0.0, output[2], 1e-10)

	// At x=0.75 (second interval): [1.0, 0.5, 0.5]
	output, err = stitched.Evaluate([]float64{0.75})
	require.NoError(t, err)
	assert.InDelta(t, 1.0, output[0], 1e-10)
	assert.InDelta(t, 0.5, output[1], 1e-10)
	assert.InDelta(t, 0.5, output[2], 1e-10)
}

func TestStitchedFunction_InvalidInput(t *testing.T) {
	domain := []float64{0.0, 1.0}
	fn1, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0}, []float64{1.0}, 1.0)
	functions := []function.Function{fn1}
	encode := []float64{0.0, 1.0}

	stitched, err := NewStitchedFunction(domain, nil, functions, []float64{}, encode)
	require.NoError(t, err)

	// Too many inputs
	_, err = stitched.Evaluate([]float64{0.5, 0.5})
	assert.Error(t, err)

	// No inputs
	_, err = stitched.Evaluate([]float64{})
	assert.Error(t, err)
}

func TestStitchedFunction_BoundaryValues(t *testing.T) {
	domain := []float64{0.0, 1.0}

	fn1, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0}, []float64{0.5}, 1.0)
	fn2, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.5}, []float64{1.0}, 1.0)

	functions := []function.Function{fn1, fn2}
	bounds := []float64{0.5}
	encode := []float64{0.0, 1.0, 0.0, 1.0}

	stitched, err := NewStitchedFunction(domain, nil, functions, bounds, encode)
	require.NoError(t, err)

	// Test exactly at the boundary
	output, err := stitched.Evaluate([]float64{0.5})
	require.NoError(t, err)
	// Should use second function (x >= 0.5)
	assert.InDelta(t, 0.5, output[0], 1e-10)
}

func TestStitchedFunction_FindInterval(t *testing.T) {
	domain := []float64{0.0, 1.0}
	fn, _ := NewExponentialFunction([]float64{0.0, 1.0}, nil, []float64{0.0}, []float64{1.0}, 1.0)

	stitched := &StitchedFunction{
		domain:    domain,
		functions: []function.Function{fn, fn, fn},
		bounds:    []float64{0.3, 0.7},
		encode:    []float64{0.0, 1.0, 0.0, 1.0, 0.0, 1.0},
	}

	tests := []struct {
		x        float64
		expected int
	}{
		{0.0, 0},
		{0.2, 0},
		{0.3, 1},
		{0.5, 1},
		{0.7, 2},
		{0.9, 2},
		{1.0, 2},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("x=%.1f", tt.x), func(t *testing.T) {
			interval := stitched.findInterval(tt.x)
			assert.Equal(t, tt.expected, interval)
		})
	}
}
