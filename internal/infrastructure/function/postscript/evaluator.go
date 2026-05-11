// Package postscript provides PostScript calculator implementation for Type 4 functions.
package postscript

import (
	"fmt"
	"math"
)

// Operator represents a PostScript operator or operand.
type Operator interface{}

// Evaluator evaluates PostScript calculator code.
type Evaluator struct {
	operators []Operator
}

// NewEvaluator creates a new PostScript evaluator.
func NewEvaluator(operators []Operator) *Evaluator {
	return &Evaluator{operators: operators}
}

// Execute executes the PostScript code with the given initial stack.
func (e *Evaluator) Execute(initialStack []float64) ([]float64, error) {
	stack := NewStack(initialStack)
	counter := 0
	length := len(e.operators)

	for counter < length {
		op := e.operators[counter]
		counter++

		// If operator is a number, push it
		if val, ok := op.(float64); ok {
			if err := stack.Push(val); err != nil {
				return nil, err
			}
			continue
		}

		// If operator is an int, convert to float64 and push
		if val, ok := op.(int); ok {
			if err := stack.Push(float64(val)); err != nil {
				return nil, err
			}
			continue
		}

		// Otherwise, it's a string operator
		opStr, ok := op.(string)
		if !ok {
			return nil, fmt.Errorf("invalid operator type: %T", op)
		}

		// Execute operator
		var err error
		counter, err = e.executeOperator(opStr, stack, counter)
		if err != nil {
			return nil, fmt.Errorf("operator %s failed: %w", opStr, err)
		}
	}

	return stack.ToSlice(), nil
}

func (e *Evaluator) executeOperator(op string, stack *Stack, counter int) (int, error) {
	var a, b float64
	var err error

	switch op {
	// Control flow
	case "jz": // jump if zero/false
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if a == 0 {
			return int(b), nil
		}

	case "j": // unconditional jump
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		return int(a), nil

	// Arithmetic operators
	case "abs":
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Push(math.Abs(a)); err != nil {
			return counter, err
		}

	case "add":
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Push(a + b); err != nil {
			return counter, err
		}

	case "div":
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if b == 0 {
			return counter, fmt.Errorf("division by zero")
		}
		if err := stack.Push(a / b); err != nil {
			return counter, err
		}

	case "idiv": // integer division
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if b == 0 {
			return counter, fmt.Errorf("division by zero")
		}
		if err := stack.Push(math.Floor(a / b)); err != nil {
			return counter, err
		}

	case "mod":
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Push(math.Mod(a, b)); err != nil {
			return counter, err
		}

	case "mul":
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Push(a * b); err != nil {
			return counter, err
		}

	case "neg":
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Push(-a); err != nil {
			return counter, err
		}

	case "sub":
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Push(a - b); err != nil {
			return counter, err
		}

	// Math functions
	case "atan":
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		result := math.Atan2(a, b) * 180 / math.Pi
		if result < 0 {
			result += 360
		}
		if err := stack.Push(result); err != nil {
			return counter, err
		}

	case "ceiling":
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Push(math.Ceil(a)); err != nil {
			return counter, err
		}

	case "cos":
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		// Angle in degrees
		rad := math.Mod(a, 360) / 180 * math.Pi
		if err := stack.Push(math.Cos(rad)); err != nil {
			return counter, err
		}

	case "exp":
		b, err = stack.Pop() // exponent
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop() // base
		if err != nil {
			return counter, err
		}
		if err := stack.Push(math.Pow(a, b)); err != nil {
			return counter, err
		}

	case "floor":
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Push(math.Floor(a)); err != nil {
			return counter, err
		}

	case "ln":
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Push(math.Log(a)); err != nil {
			return counter, err
		}

	case "log":
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Push(math.Log10(a)); err != nil {
			return counter, err
		}

	case "round":
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Push(math.Round(a)); err != nil {
			return counter, err
		}

	case "sin":
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		// Angle in degrees
		rad := math.Mod(a, 360) / 180 * math.Pi
		if err := stack.Push(math.Sin(rad)); err != nil {
			return counter, err
		}

	case "sqrt":
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Push(math.Sqrt(a)); err != nil {
			return counter, err
		}

	case "truncate":
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if a < 0 {
			if err := stack.Push(math.Ceil(a)); err != nil {
				return counter, err
			}
		} else {
			if err := stack.Push(math.Floor(a)); err != nil {
				return counter, err
			}
		}

	// Comparison operators (use 1.0 for true, 0.0 for false)
	case "eq":
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if a == b {
			if err := stack.Push(1.0); err != nil {
				return counter, err
			}
		} else {
			if err := stack.Push(0.0); err != nil {
				return counter, err
			}
		}

	case "ge":
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if a >= b {
			if err := stack.Push(1.0); err != nil {
				return counter, err
			}
		} else {
			if err := stack.Push(0.0); err != nil {
				return counter, err
			}
		}

	case "gt":
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if a > b {
			if err := stack.Push(1.0); err != nil {
				return counter, err
			}
		} else {
			if err := stack.Push(0.0); err != nil {
				return counter, err
			}
		}

	case "le":
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if a <= b {
			if err := stack.Push(1.0); err != nil {
				return counter, err
			}
		} else {
			if err := stack.Push(0.0); err != nil {
				return counter, err
			}
		}

	case "lt":
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if a < b {
			if err := stack.Push(1.0); err != nil {
				return counter, err
			}
		} else {
			if err := stack.Push(0.0); err != nil {
				return counter, err
			}
		}

	case "ne":
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if a != b {
			if err := stack.Push(1.0); err != nil {
				return counter, err
			}
		} else {
			if err := stack.Push(0.0); err != nil {
				return counter, err
			}
		}

	// Bitwise operators (treat floats as integers)
	case "and":
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Push(float64(int(a) & int(b))); err != nil {
			return counter, err
		}

	case "bitshift":
		b, err = stack.Pop() // shift amount
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop() // value
		if err != nil {
			return counter, err
		}
		shift := int(b)
		val := int(a)
		var result int
		if shift > 0 {
			result = val << uint(shift)
		} else {
			result = val >> uint(-shift)
		}
		if err := stack.Push(float64(result)); err != nil {
			return counter, err
		}

	case "not":
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		// Bitwise not
		if err := stack.Push(float64(^int(a))); err != nil {
			return counter, err
		}

	case "or":
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Push(float64(int(a) | int(b))); err != nil {
			return counter, err
		}

	case "xor":
		b, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Push(float64(int(a) ^ int(b))); err != nil {
			return counter, err
		}

	// Stack operators
	case "copy":
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Copy(int(a)); err != nil {
			return counter, err
		}

	case "dup":
		if err := stack.Dup(); err != nil {
			return counter, err
		}

	case "exch":
		if err := stack.Exch(); err != nil {
			return counter, err
		}

	case "index":
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Index(int(a)); err != nil {
			return counter, err
		}

	case "pop":
		_, err = stack.Pop()
		if err != nil {
			return counter, err
		}

	case "roll":
		b, err = stack.Pop() // p (times)
		if err != nil {
			return counter, err
		}
		a, err = stack.Pop() // n (count)
		if err != nil {
			return counter, err
		}
		if err := stack.Roll(int(a), int(b)); err != nil {
			return counter, err
		}

	// Type conversion
	case "cvi": // convert to integer
		a, err = stack.Pop()
		if err != nil {
			return counter, err
		}
		if err := stack.Push(math.Floor(a)); err != nil {
			return counter, err
		}

	case "cvr": // convert to real (no-op in our case)
		// Do nothing, already float64

	// Constants
	case "true":
		if err := stack.Push(1.0); err != nil {
			return counter, err
		}

	case "false":
		if err := stack.Push(0.0); err != nil {
			return counter, err
		}

	default:
		return counter, fmt.Errorf("unknown operator: %s", op)
	}

	return counter, nil
}
