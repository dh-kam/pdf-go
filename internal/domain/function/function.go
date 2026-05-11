// Package function defines the domain entities for PDF functions.
//
//revive:disable:exported
package function

// FunctionType represents the type of a PDF function.
type FunctionType int

const (
	// TypeSampled represents Type 0 - Sampled functions
	TypeSampled FunctionType = 0
	// TypeExponential represents Type 2 - Exponential interpolation functions
	TypeExponential FunctionType = 2
	// TypeStitched represents Type 3 - Stitched functions
	TypeStitched FunctionType = 3
	// TypePostScript represents Type 4 - PostScript calculator functions
	TypePostScript FunctionType = 4
)

// Function represents a PDF function that maps input values to output values.
// Functions are used in shading patterns, transfer functions, and color conversion.
type Function interface {
	// Evaluate evaluates the function for the given input values.
	// The input slice length must match InputSize().
	// Returns a slice of output values with length matching OutputSize().
	Evaluate(input []float64) ([]float64, error)

	// InputSize returns the number of input values required.
	InputSize() int

	// OutputSize returns the number of output values produced.
	OutputSize() int

	// Type returns the function type.
	Type() FunctionType

	// Domain returns the valid input range as [min, max] pairs.
	// Length is 2 * InputSize().
	Domain() []float64

	// Range returns the valid output range as [min, max] pairs.
	// Length is 2 * OutputSize().
	// May be nil if not specified.
	Range() []float64
}
