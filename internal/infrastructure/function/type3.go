// Package function provides infrastructure implementations of PDF functions.
package function

import (
	"fmt"

	"github.com/dh-kam/pdf-go/internal/domain/function"
)

// StitchedFunction implements Type 3 - Stitched functions.
// Connects multiple sub-functions across different intervals.
type StitchedFunction struct {
	domain    []float64           // [xmin, xmax]
	rng       []float64           // [ymin, ymax, ...] (optional)
	functions []function.Function // Sub-functions
	bounds    []float64           // Subdivision points
	encode    []float64           // Input encoding for each sub-function
}

// NewStitchedFunction creates a new stitched function.
func NewStitchedFunction(domain, rng []float64, functions []function.Function, bounds, encode []float64) (*StitchedFunction, error) {
	if len(domain) != 2 {
		return nil, fmt.Errorf("domain must have exactly 2 values, got %d", len(domain))
	}
	if len(functions) == 0 {
		return nil, fmt.Errorf("functions cannot be empty")
	}
	if len(bounds) != len(functions)-1 {
		return nil, fmt.Errorf("bounds length %d must be functions length - 1 (%d)", len(bounds), len(functions)-1)
	}
	if len(encode) != len(functions)*2 {
		return nil, fmt.Errorf("encode length %d must be 2 * functions length (%d)", len(encode), len(functions)*2)
	}

	// Verify all sub-functions have same input/output size
	if len(functions) > 0 {
		expectedInputSize := functions[0].InputSize()
		expectedOutputSize := functions[0].OutputSize()

		for i, fn := range functions {
			if fn.InputSize() != expectedInputSize {
				return nil, fmt.Errorf("function %d has input size %d, expected %d", i, fn.InputSize(), expectedInputSize)
			}
			if fn.OutputSize() != expectedOutputSize {
				return nil, fmt.Errorf("function %d has output size %d, expected %d", i, fn.OutputSize(), expectedOutputSize)
			}
		}

		// Verify range if specified
		if rng != nil && len(rng) != 2*expectedOutputSize {
			return nil, fmt.Errorf("range length %d must be 2 * output size %d", len(rng), expectedOutputSize)
		}
	}

	return &StitchedFunction{
		domain:    domain,
		rng:       rng,
		functions: functions,
		bounds:    bounds,
		encode:    encode,
	}, nil
}

// Evaluate evaluates the stitched function.
func (f *StitchedFunction) Evaluate(input []float64) ([]float64, error) {
	if len(input) != 1 {
		return nil, fmt.Errorf("stitched function requires exactly 1 input, got %d", len(input))
	}

	// Clamp input to domain
	x := clamp(input[0], f.domain[0], f.domain[1])

	// Find which sub-function interval x falls into
	i := f.findInterval(x)

	// Determine the bounds of this interval
	dmin := f.domain[0]
	if i > 0 {
		dmin = f.bounds[i-1]
	}

	dmax := f.domain[1]
	if i < len(f.bounds) {
		dmax = f.bounds[i]
	}

	// Encode x into the sub-function's domain
	rmin := f.encode[2*i]
	rmax := f.encode[2*i+1]

	var xEncoded float64
	if dmin == dmax {
		// Prevent division by zero
		xEncoded = rmin
	} else {
		xEncoded = rmin + (x-dmin)*(rmax-rmin)/(dmax-dmin)
	}

	// Evaluate the sub-function
	output, err := f.functions[i].Evaluate([]float64{xEncoded})
	if err != nil {
		return nil, fmt.Errorf("sub-function %d evaluation failed: %w", i, err)
	}

	// Apply range clipping if specified
	if f.rng != nil {
		for j := range output {
			output[j] = clamp(output[j], f.rng[2*j], f.rng[2*j+1])
		}
	}

	return output, nil
}

// findInterval finds which interval x falls into based on bounds.
func (f *StitchedFunction) findInterval(x float64) int {
	for i := 0; i < len(f.bounds); i++ {
		if x < f.bounds[i] {
			return i
		}
	}
	return len(f.bounds) // Last interval
}

// InputSize returns 1 (stitched functions have single input).
func (f *StitchedFunction) InputSize() int {
	return 1
}

// OutputSize returns the number of output values.
func (f *StitchedFunction) OutputSize() int {
	if len(f.functions) > 0 {
		return f.functions[0].OutputSize()
	}
	return 0
}

// Type returns TypeStitched.
func (f *StitchedFunction) Type() function.FunctionType {
	return function.TypeStitched
}

// Domain returns the input domain.
func (f *StitchedFunction) Domain() []float64 {
	return f.domain
}

// Range returns the output range.
func (f *StitchedFunction) Range() []float64 {
	return f.rng
}
