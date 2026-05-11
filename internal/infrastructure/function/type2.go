// Package function provides infrastructure implementations of PDF functions.
package function

import (
	"fmt"
	"math"

	"github.com/dh-kam/pdf-go/internal/domain/function"
)

// ExponentialFunction implements Type 2 - Exponential interpolation function.
// f(x) = C0 + x^N * (C1 - C0)
type ExponentialFunction struct {
	domain []float64 // [xmin, xmax]
	rng    []float64 // [ymin, ymax, ...] (range is reserved keyword)
	c0     []float64 // Starting values
	c1     []float64 // Ending values
	n      float64   // Exponent
}

// NewExponentialFunction creates a new exponential interpolation function.
func NewExponentialFunction(domain, rng, c0, c1 []float64, n float64) (*ExponentialFunction, error) {
	if len(domain) != 2 {
		return nil, fmt.Errorf("domain must have exactly 2 values, got %d", len(domain))
	}
	if len(c0) == 0 {
		return nil, fmt.Errorf("c0 cannot be empty")
	}
	if len(c1) != len(c0) {
		return nil, fmt.Errorf("c1 length %d must match c0 length %d", len(c1), len(c0))
	}
	if rng != nil && len(rng) != 2*len(c0) {
		return nil, fmt.Errorf("range length %d must be 2 * output size %d", len(rng), len(c0))
	}
	if n < 0 {
		return nil, fmt.Errorf("exponent cannot be negative: %f", n)
	}

	return &ExponentialFunction{
		domain: domain,
		rng:    rng,
		c0:     c0,
		c1:     c1,
		n:      n,
	}, nil
}

// Evaluate evaluates the exponential function.
func (f *ExponentialFunction) Evaluate(input []float64) ([]float64, error) {
	if len(input) != 1 {
		return nil, fmt.Errorf("exponential function requires exactly 1 input, got %d", len(input))
	}

	// Clip input to domain
	x := clamp(input[0], f.domain[0], f.domain[1])

	// Calculate output values
	output := make([]float64, len(f.c0))
	for i := range output {
		// f(x) = C0 + x^N * (C1 - C0)
		output[i] = f.c0[i] + math.Pow(x, f.n)*(f.c1[i]-f.c0[i])

		// Clip output to range if specified
		if f.rng != nil {
			output[i] = clamp(output[i], f.rng[2*i], f.rng[2*i+1])
		}
	}

	return output, nil
}

// InputSize returns 1 (exponential functions have single input).
func (f *ExponentialFunction) InputSize() int {
	return 1
}

// OutputSize returns the number of output values.
func (f *ExponentialFunction) OutputSize() int {
	return len(f.c0)
}

// Type returns TypeExponential.
func (f *ExponentialFunction) Type() function.FunctionType {
	return function.TypeExponential
}

// Domain returns the input domain.
func (f *ExponentialFunction) Domain() []float64 {
	return f.domain
}

// Range returns the output range.
func (f *ExponentialFunction) Range() []float64 {
	return f.rng
}

// clamp restricts a value to the given range.
func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
