// Package function provides infrastructure implementations of PDF functions.
package function

import (
	"fmt"

	"github.com/dh-kam/pdf-go/internal/domain/function"
	"github.com/dh-kam/pdf-go/internal/infrastructure/function/postscript"
)

// PostScriptFunction implements Type 4 - PostScript calculator functions.
type PostScriptFunction struct {
	evaluator *postscript.Evaluator
	domain    []float64
	rng       []float64
	operators []postscript.Operator
}

// NewPostScriptFunction creates a new PostScript calculator function.
func NewPostScriptFunction(domain, rng []float64, operators []postscript.Operator) (*PostScriptFunction, error) {
	if len(domain)%2 != 0 {
		return nil, fmt.Errorf("domain length must be even, got %d", len(domain))
	}
	if len(domain) == 0 {
		return nil, fmt.Errorf("domain cannot be empty")
	}

	if rng != nil {
		if len(rng)%2 != 0 {
			return nil, fmt.Errorf("range length must be even, got %d", len(rng))
		}
	}

	// Validate operators
	if len(operators) == 0 {
		return nil, fmt.Errorf("operators cannot be empty")
	}

	return &PostScriptFunction{
		domain:    domain,
		rng:       rng,
		operators: operators,
		evaluator: postscript.NewEvaluator(operators),
	}, nil
}

// Evaluate evaluates the PostScript function.
func (f *PostScriptFunction) Evaluate(input []float64) ([]float64, error) {
	inputSize := len(f.domain) / 2
	if len(input) != inputSize {
		return nil, fmt.Errorf("expected %d inputs, got %d", inputSize, len(input))
	}

	// Clip inputs to domain
	clipped := make([]float64, len(input))
	for i := range input {
		clipped[i] = clamp(input[i], f.domain[2*i], f.domain[2*i+1])
	}

	// Execute PostScript code with clipped inputs on the stack
	result, err := f.evaluator.Execute(clipped)
	if err != nil {
		return nil, fmt.Errorf("PostScript execution failed: %w", err)
	}

	// Clip outputs to range if specified
	if f.rng != nil {
		outputSize := len(f.rng) / 2
		if len(result) < outputSize {
			return nil, fmt.Errorf("PostScript returned %d values, expected at least %d", len(result), outputSize)
		}

		// Take only the required number of outputs from the top of the stack
		output := result[len(result)-outputSize:]

		for i := range output {
			output[i] = clamp(output[i], f.rng[2*i], f.rng[2*i+1])
		}
		return output, nil
	}

	return result, nil
}

// InputSize returns the number of input values required.
func (f *PostScriptFunction) InputSize() int {
	return len(f.domain) / 2
}

// OutputSize returns the number of output values produced.
func (f *PostScriptFunction) OutputSize() int {
	if f.rng != nil {
		return len(f.rng) / 2
	}
	// If no range specified, we can't determine output size
	// This will be determined at runtime
	return 0
}

// Type returns TypePostScript.
func (f *PostScriptFunction) Type() function.FunctionType {
	return function.TypePostScript
}

// Domain returns the input domain.
func (f *PostScriptFunction) Domain() []float64 {
	return f.domain
}

// Range returns the output range.
func (f *PostScriptFunction) Range() []float64 {
	return f.rng
}
