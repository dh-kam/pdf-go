package function

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testFunction struct {
	inputSize  int
	outputSize int
}

func (f *testFunction) Evaluate(input []float64) ([]float64, error) {
	return make([]float64, f.outputSize), nil
}

func (f *testFunction) InputSize() int {
	return f.inputSize
}

func (f *testFunction) OutputSize() int {
	return f.outputSize
}

func (f *testFunction) Type() FunctionType {
	return TypeExponential
}

func (f *testFunction) Domain() []float64 {
	return []float64{0.0, 1.0}
}

func (f *testFunction) Range() []float64 {
	return []float64{0.0, 1.0}
}

func TestFunctionTypeConstants(t *testing.T) {
	assert.Equal(t, FunctionType(0), TypeSampled)
	assert.Equal(t, FunctionType(2), TypeExponential)
	assert.Equal(t, FunctionType(3), TypeStitched)
	assert.Equal(t, FunctionType(4), TypePostScript)
}

func TestFunctionInterface(t *testing.T) {
	f := &testFunction{inputSize: 2, outputSize: 3}
	assert.Equal(t, 2, f.InputSize())
	assert.Equal(t, 3, f.OutputSize())
	assert.Equal(t, TypeExponential, f.Type())
	assert.Len(t, f.Domain(), 2)
	assert.Len(t, f.Range(), 2)

	out, err := f.Evaluate([]float64{0.1, 0.2})
	assert.NoError(t, err)
	assert.Len(t, out, 3)
}
