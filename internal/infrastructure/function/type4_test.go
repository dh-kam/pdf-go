package function

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/function"
	"github.com/dh-kam/pdf-go/internal/infrastructure/function/postscript"
)

func TestNewPostScriptFunction_Valid(t *testing.T) {
	domain := []float64{0.0, 1.0}
	rng := []float64{0.0, 1.0}
	ops := []postscript.Operator{2.0, "mul"} // multiply input by 2

	fn, err := NewPostScriptFunction(domain, rng, ops)
	require.NoError(t, err)
	assert.NotNil(t, fn)
	assert.Equal(t, function.TypePostScript, fn.Type())
	assert.Equal(t, 1, fn.InputSize())
	assert.Equal(t, 1, fn.OutputSize())
}

func TestNewPostScriptFunction_InvalidDomain(t *testing.T) {
	ops := []postscript.Operator{1.0}

	tests := []struct {
		name   string
		domain []float64
	}{
		{"empty", []float64{}},
		{"odd length", []float64{0.0, 1.0, 2.0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewPostScriptFunction(tt.domain, nil, ops)
			assert.Error(t, err)
		})
	}
}

func TestNewPostScriptFunction_InvalidRange(t *testing.T) {
	domain := []float64{0.0, 1.0}
	ops := []postscript.Operator{1.0}

	_, err := NewPostScriptFunction(domain, []float64{0.0, 1.0, 2.0}, ops)
	assert.Error(t, err)
}

func TestNewPostScriptFunction_EmptyOperators(t *testing.T) {
	domain := []float64{0.0, 1.0}
	_, err := NewPostScriptFunction(domain, nil, []postscript.Operator{})
	assert.Error(t, err)
}

func TestPostScriptFunction_Simple(t *testing.T) {
	// f(x) = 2x
	domain := []float64{0.0, 1.0}
	rng := []float64{0.0, 2.0}
	ops := []postscript.Operator{2.0, "mul"}

	fn, err := NewPostScriptFunction(domain, rng, ops)
	require.NoError(t, err)

	tests := []struct {
		input    float64
		expected float64
	}{
		{0.0, 0.0},
		{0.5, 1.0},
		{1.0, 2.0},
	}

	for _, tt := range tests {
		output, err := fn.Evaluate([]float64{tt.input})
		require.NoError(t, err)
		assert.InDelta(t, tt.expected, output[0], 1e-10)
	}
}

func TestPostScriptFunction_DomainClipping(t *testing.T) {
	// f(x) = x (identity function)
	domain := []float64{0.0, 1.0}
	rng := []float64{0.0, 1.0}
	ops := []postscript.Operator{"dup", "pop"} // x -> x x -> x (identity)

	fn, err := NewPostScriptFunction(domain, rng, ops)
	require.NoError(t, err)

	// Input outside domain should be clipped
	output, err := fn.Evaluate([]float64{-1.0})
	require.NoError(t, err)
	assert.Equal(t, []float64{0.0}, output)

	output, err = fn.Evaluate([]float64{2.0})
	require.NoError(t, err)
	assert.Equal(t, []float64{1.0}, output)
}

func TestPostScriptFunction_RangeClipping(t *testing.T) {
	// f(x) = 10x (but range is [0, 5])
	domain := []float64{0.0, 1.0}
	rng := []float64{0.0, 5.0}
	ops := []postscript.Operator{10.0, "mul"}

	fn, err := NewPostScriptFunction(domain, rng, ops)
	require.NoError(t, err)

	// x=1 gives 10, but should be clipped to 5
	output, err := fn.Evaluate([]float64{1.0})
	require.NoError(t, err)
	assert.Equal(t, []float64{5.0}, output)
}

func TestPostScriptFunction_ComplexExpression(t *testing.T) {
	// f(x) = x^2 + 2x + 1 = (x+1)^2
	// Stack: x -> x x -> x^2 -> x^2 2 -> x^2 2 x -> x^2 2x -> x^2+2x -> x^2+2x+1
	domain := []float64{0.0, 10.0}
	ops := []postscript.Operator{
		"dup",  // x -> x x
		"dup",  // x x -> x x x
		"mul",  // x x x -> x x^2
		"exch", // x x^2 -> x^2 x
		2.0,    // x^2 x -> x^2 x 2
		"mul",  // x^2 x 2 -> x^2 2x
		"add",  // x^2 2x -> x^2+2x
		1.0,    // x^2+2x -> x^2+2x 1
		"add",  // x^2+2x 1 -> x^2+2x+1
	}

	fn, err := NewPostScriptFunction(domain, nil, ops)
	require.NoError(t, err)

	// Test (2+1)^2 = 9
	output, err := fn.Evaluate([]float64{2.0})
	require.NoError(t, err)
	assert.InDelta(t, 9.0, output[0], 1e-10)

	// Test (3+1)^2 = 16
	output, err = fn.Evaluate([]float64{3.0})
	require.NoError(t, err)
	assert.InDelta(t, 16.0, output[0], 1e-10)
}

func TestPostScriptFunction_MultiInput(t *testing.T) {
	// f(x, y) = x + y
	domain := []float64{0.0, 10.0, 0.0, 10.0}
	rng := []float64{0.0, 20.0}
	ops := []postscript.Operator{"add"}

	fn, err := NewPostScriptFunction(domain, rng, ops)
	require.NoError(t, err)

	assert.Equal(t, 2, fn.InputSize())
	assert.Equal(t, 1, fn.OutputSize())

	output, err := fn.Evaluate([]float64{3.0, 5.0})
	require.NoError(t, err)
	assert.Equal(t, []float64{8.0}, output)
}

func TestPostScriptFunction_Conditional(t *testing.T) {
	// f(x) = x < 0.5 ? 0 : 1
	domain := []float64{0.0, 1.0}
	rng := []float64{0.0, 1.0}
	ops := []postscript.Operator{
		0.5,  // x 0.5
		"lt", // (x<0.5)
		7.0,  // (x<0.5) 7
		"jz", // jump to 7 if x >= 0.5
		0.0,  // push 0
		9.0,  // 0 9
		"j",  // unconditional jump to 9 (skip the 1)
		1.0,  // push 1 (position 7)
		// position 9: stack has result
	}

	fn, err := NewPostScriptFunction(domain, rng, ops)
	require.NoError(t, err)

	// x=0.3 < 0.5 -> 0
	output, err := fn.Evaluate([]float64{0.3})
	require.NoError(t, err)
	assert.Equal(t, []float64{0.0}, output)

	// x=0.7 >= 0.5 -> 1
	output, err = fn.Evaluate([]float64{0.7})
	require.NoError(t, err)
	assert.Equal(t, []float64{1.0}, output)
}

func TestPostScriptFunction_InvalidInput(t *testing.T) {
	domain := []float64{0.0, 1.0}
	ops := []postscript.Operator{2.0, "mul"}

	fn, err := NewPostScriptFunction(domain, nil, ops)
	require.NoError(t, err)

	// Wrong number of inputs
	_, err = fn.Evaluate([]float64{0.5, 0.5})
	assert.Error(t, err)

	_, err = fn.Evaluate([]float64{})
	assert.Error(t, err)
}

func TestPostScriptFunction_ExecutionError(t *testing.T) {
	domain := []float64{0.0, 1.0}
	ops := []postscript.Operator{"add"} // Will cause underflow

	fn, err := NewPostScriptFunction(domain, nil, ops)
	require.NoError(t, err)

	_, err = fn.Evaluate([]float64{0.5})
	assert.Error(t, err)
}

func TestPostScriptFunction_NoRange(t *testing.T) {
	// Function without explicit range
	domain := []float64{0.0, 1.0}
	ops := []postscript.Operator{2.0, "mul"}

	fn, err := NewPostScriptFunction(domain, nil, ops)
	require.NoError(t, err)

	assert.Equal(t, 0, fn.OutputSize()) // Unknown without range

	output, err := fn.Evaluate([]float64{0.5})
	require.NoError(t, err)
	// Returns whatever is on the stack
	assert.Len(t, output, 1)
	assert.InDelta(t, 1.0, output[0], 1e-10)
}
func TestPostScriptFunction_MultiOutput(t *testing.T) {
	// f(x) = (x, 2x, 3x) - simple linear multiples
	domain := []float64{0.0, 10.0}
	rng := []float64{0.0, 10.0, 0.0, 20.0, 0.0, 30.0}
	ops := []postscript.Operator{
		"dup",  // x -> x x
		"dup",  // x x -> x x x
		2.0,    // x x x -> x x x 2
		"mul",  // x x x 2 -> x x 2x
		"exch", // x x 2x -> x 2x x
		3.0,    // x 2x x -> x 2x x 3
		"mul",  // x 2x x 3 -> x 2x 3x
		// Stack: x 2x 3x (exactly what we want!)
	}

	fn, err := NewPostScriptFunction(domain, rng, ops)
	require.NoError(t, err)

	assert.Equal(t, 1, fn.InputSize())
	assert.Equal(t, 3, fn.OutputSize())

	// f(2) = (2, 4, 6)
	output, err := fn.Evaluate([]float64{2.0})
	require.NoError(t, err)
	assert.Len(t, output, 3)
	assert.InDelta(t, 2.0, output[0], 1e-10)
	assert.InDelta(t, 4.0, output[1], 1e-10)
	assert.InDelta(t, 6.0, output[2], 1e-10)
}
