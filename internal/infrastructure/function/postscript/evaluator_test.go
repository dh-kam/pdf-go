package postscript

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvaluator_Arithmetic(t *testing.T) {
	tests := []struct {
		name      string
		operators []Operator
		initial   []float64
		expected  []float64
	}{
		{
			name:      "add",
			operators: []Operator{5.0, 3.0, "add"},
			expected:  []float64{8.0},
		},
		{
			name:      "sub",
			operators: []Operator{10.0, 3.0, "sub"},
			expected:  []float64{7.0},
		},
		{
			name:      "mul",
			operators: []Operator{4.0, 5.0, "mul"},
			expected:  []float64{20.0},
		},
		{
			name:      "div",
			operators: []Operator{20.0, 4.0, "div"},
			expected:  []float64{5.0},
		},
		{
			name:      "idiv",
			operators: []Operator{22.0, 4.0, "idiv"},
			expected:  []float64{5.0},
		},
		{
			name:      "mod",
			operators: []Operator{22.0, 4.0, "mod"},
			expected:  []float64{2.0},
		},
		{
			name:      "neg",
			operators: []Operator{5.0, "neg"},
			expected:  []float64{-5.0},
		},
		{
			name:      "abs positive",
			operators: []Operator{5.0, "abs"},
			expected:  []float64{5.0},
		},
		{
			name:      "abs negative",
			operators: []Operator{-5.0, "abs"},
			expected:  []float64{5.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := NewEvaluator(tt.operators)
			result, err := eval.Execute(tt.initial)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEvaluator_Math(t *testing.T) {
	tests := []struct {
		name      string
		operators []Operator
		expected  float64
		delta     float64
	}{
		{
			name:      "sqrt",
			operators: []Operator{16.0, "sqrt"},
			expected:  4.0,
			delta:     1e-10,
		},
		{
			name:      "sin 0",
			operators: []Operator{0.0, "sin"},
			expected:  0.0,
			delta:     1e-10,
		},
		{
			name:      "sin 90",
			operators: []Operator{90.0, "sin"},
			expected:  1.0,
			delta:     1e-10,
		},
		{
			name:      "cos 0",
			operators: []Operator{0.0, "cos"},
			expected:  1.0,
			delta:     1e-10,
		},
		{
			name:      "cos 90",
			operators: []Operator{90.0, "cos"},
			expected:  0.0,
			delta:     1e-10,
		},
		{
			name:      "exp",
			operators: []Operator{2.0, 3.0, "exp"},
			expected:  8.0,
			delta:     1e-10,
		},
		{
			name:      "ln",
			operators: []Operator{math.E, "ln"},
			expected:  1.0,
			delta:     1e-10,
		},
		{
			name:      "log",
			operators: []Operator{100.0, "log"},
			expected:  2.0,
			delta:     1e-10,
		},
		{
			name:      "floor",
			operators: []Operator{3.7, "floor"},
			expected:  3.0,
			delta:     1e-10,
		},
		{
			name:      "ceiling",
			operators: []Operator{3.2, "ceiling"},
			expected:  4.0,
			delta:     1e-10,
		},
		{
			name:      "round",
			operators: []Operator{3.5, "round"},
			expected:  4.0,
			delta:     1e-10,
		},
		{
			name:      "truncate positive",
			operators: []Operator{3.7, "truncate"},
			expected:  3.0,
			delta:     1e-10,
		},
		{
			name:      "truncate negative",
			operators: []Operator{-3.7, "truncate"},
			expected:  -3.0,
			delta:     1e-10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := NewEvaluator(tt.operators)
			result, err := eval.Execute(nil)
			require.NoError(t, err)
			require.Len(t, result, 1)
			assert.InDelta(t, tt.expected, result[0], tt.delta)
		})
	}
}

func TestEvaluator_Comparison(t *testing.T) {
	tests := []struct {
		name      string
		operators []Operator
		expected  float64
	}{
		{
			name:      "eq true",
			operators: []Operator{5.0, 5.0, "eq"},
			expected:  1.0,
		},
		{
			name:      "eq false",
			operators: []Operator{5.0, 3.0, "eq"},
			expected:  0.0,
		},
		{
			name:      "ne true",
			operators: []Operator{5.0, 3.0, "ne"},
			expected:  1.0,
		},
		{
			name:      "ne false",
			operators: []Operator{5.0, 5.0, "ne"},
			expected:  0.0,
		},
		{
			name:      "gt true",
			operators: []Operator{5.0, 3.0, "gt"},
			expected:  1.0,
		},
		{
			name:      "gt false",
			operators: []Operator{3.0, 5.0, "gt"},
			expected:  0.0,
		},
		{
			name:      "ge true equal",
			operators: []Operator{5.0, 5.0, "ge"},
			expected:  1.0,
		},
		{
			name:      "ge true greater",
			operators: []Operator{5.0, 3.0, "ge"},
			expected:  1.0,
		},
		{
			name:      "lt true",
			operators: []Operator{3.0, 5.0, "lt"},
			expected:  1.0,
		},
		{
			name:      "le true",
			operators: []Operator{3.0, 5.0, "le"},
			expected:  1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := NewEvaluator(tt.operators)
			result, err := eval.Execute(nil)
			require.NoError(t, err)
			require.Len(t, result, 1)
			assert.Equal(t, tt.expected, result[0])
		})
	}
}

func TestEvaluator_Stack(t *testing.T) {
	tests := []struct {
		name      string
		operators []Operator
		expected  []float64
	}{
		{
			name:      "dup",
			operators: []Operator{5.0, "dup"},
			expected:  []float64{5.0, 5.0},
		},
		{
			name:      "exch",
			operators: []Operator{1.0, 2.0, "exch"},
			expected:  []float64{2.0, 1.0},
		},
		{
			name:      "pop",
			operators: []Operator{1.0, 2.0, 3.0, "pop"},
			expected:  []float64{1.0, 2.0},
		},
		{
			name:      "copy",
			operators: []Operator{1.0, 2.0, 3.0, 2.0, "copy"},
			expected:  []float64{1.0, 2.0, 3.0, 2.0, 3.0},
		},
		{
			name:      "index",
			operators: []Operator{1.0, 2.0, 3.0, 4.0, 2.0, "index"},
			expected:  []float64{1.0, 2.0, 3.0, 4.0, 2.0},
		},
		{
			name:      "roll",
			operators: []Operator{1.0, 2.0, 3.0, 4.0, 5.0, 3.0, 1.0, "roll"},
			expected:  []float64{1.0, 2.0, 5.0, 3.0, 4.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := NewEvaluator(tt.operators)
			result, err := eval.Execute(nil)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEvaluator_Bitwise(t *testing.T) {
	tests := []struct {
		name      string
		operators []Operator
		expected  float64
	}{
		{
			name:      "and",
			operators: []Operator{12.0, 10.0, "and"}, // 1100 & 1010 = 1000 = 8
			expected:  8.0,
		},
		{
			name:      "or",
			operators: []Operator{12.0, 10.0, "or"}, // 1100 | 1010 = 1110 = 14
			expected:  14.0,
		},
		{
			name:      "xor",
			operators: []Operator{12.0, 10.0, "xor"}, // 1100 ^ 1010 = 0110 = 6
			expected:  6.0,
		},
		{
			name:      "bitshift left",
			operators: []Operator{3.0, 2.0, "bitshift"}, // 3 << 2 = 12
			expected:  12.0,
		},
		{
			name:      "bitshift right",
			operators: []Operator{12.0, -2.0, "bitshift"}, // 12 >> 2 = 3
			expected:  3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eval := NewEvaluator(tt.operators)
			result, err := eval.Execute(nil)
			require.NoError(t, err)
			require.Len(t, result, 1)
			assert.Equal(t, tt.expected, result[0])
		})
	}
}

func TestEvaluator_ControlFlow(t *testing.T) {
	// Test conditional jump (jz)
	// If top of stack is 0, jump to position
	t.Run("jz true", func(t *testing.T) {
		ops := []Operator{
			0.0,   // condition (false/zero)
			4.0,   // jump target
			"jz",  // jump if zero
			999.0, // this should be skipped
			42.0,  // jump here
		}
		eval := NewEvaluator(ops)
		result, err := eval.Execute(nil)
		require.NoError(t, err)
		assert.Equal(t, []float64{42.0}, result)
	})

	t.Run("jz false", func(t *testing.T) {
		ops := []Operator{
			1.0,  // condition (true/non-zero)
			4.0,  // jump target
			"jz", // jump if zero
			42.0, // this should execute
		}
		eval := NewEvaluator(ops)
		result, err := eval.Execute(nil)
		require.NoError(t, err)
		assert.Equal(t, []float64{42.0}, result)
	})

	t.Run("unconditional jump", func(t *testing.T) {
		ops := []Operator{
			3.0,   // jump target
			"j",   // unconditional jump
			999.0, // skipped
			42.0,  // jump here
		}
		eval := NewEvaluator(ops)
		result, err := eval.Execute(nil)
		require.NoError(t, err)
		assert.Equal(t, []float64{42.0}, result)
	})
}

func TestEvaluator_Constants(t *testing.T) {
	t.Run("true", func(t *testing.T) {
		eval := NewEvaluator([]Operator{"true"})
		result, err := eval.Execute(nil)
		require.NoError(t, err)
		assert.Equal(t, []float64{1.0}, result)
	})

	t.Run("false", func(t *testing.T) {
		eval := NewEvaluator([]Operator{"false"})
		result, err := eval.Execute(nil)
		require.NoError(t, err)
		assert.Equal(t, []float64{0.0}, result)
	})
}

func TestEvaluator_TypeConversion(t *testing.T) {
	t.Run("cvi", func(t *testing.T) {
		eval := NewEvaluator([]Operator{3.7, "cvi"})
		result, err := eval.Execute(nil)
		require.NoError(t, err)
		assert.Equal(t, []float64{3.0}, result)
	})

	t.Run("cvr", func(t *testing.T) {
		eval := NewEvaluator([]Operator{3.0, "cvr"})
		result, err := eval.Execute(nil)
		require.NoError(t, err)
		assert.Equal(t, []float64{3.0}, result)
	})
}

func TestEvaluator_ComplexExpression(t *testing.T) {
	// Calculate: (5 + 3) * 2 - 4 = 12
	ops := []Operator{
		5.0,
		3.0,
		"add",
		2.0,
		"mul",
		4.0,
		"sub",
	}
	eval := NewEvaluator(ops)
	result, err := eval.Execute(nil)
	require.NoError(t, err)
	assert.Equal(t, []float64{12.0}, result)
}

func TestEvaluator_WithInitialStack(t *testing.T) {
	// Initial stack has values, add to them
	ops := []Operator{"add"}
	eval := NewEvaluator(ops)
	result, err := eval.Execute([]float64{5.0, 3.0})
	require.NoError(t, err)
	assert.Equal(t, []float64{8.0}, result)
}

func TestEvaluator_Errors(t *testing.T) {
	t.Run("division by zero", func(t *testing.T) {
		eval := NewEvaluator([]Operator{5.0, 0.0, "div"})
		_, err := eval.Execute(nil)
		assert.Error(t, err)
	})

	t.Run("stack underflow", func(t *testing.T) {
		eval := NewEvaluator([]Operator{"add"})
		_, err := eval.Execute(nil)
		assert.Error(t, err)
	})

	t.Run("unknown operator", func(t *testing.T) {
		eval := NewEvaluator([]Operator{"unknown"})
		_, err := eval.Execute(nil)
		assert.Error(t, err)
	})
}
