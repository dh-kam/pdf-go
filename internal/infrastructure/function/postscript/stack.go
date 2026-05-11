// Package postscript provides PostScript calculator implementation for Type 4 functions.
package postscript

import (
	"fmt"
)

const maxStackSize = 100

// Stack represents a PostScript operand stack.
type Stack struct {
	stack []float64
}

// NewStack creates a new PostScript stack.
func NewStack(initialStack []float64) *Stack {
	if initialStack == nil {
		return &Stack{stack: make([]float64, 0, maxStackSize)}
	}
	s := &Stack{stack: make([]float64, len(initialStack), maxStackSize)}
	copy(s.stack, initialStack)
	return s
}

// Push pushes a value onto the stack.
func (s *Stack) Push(value float64) error {
	if len(s.stack) >= maxStackSize {
		return fmt.Errorf("stack overflow")
	}
	s.stack = append(s.stack, value)
	return nil
}

// Pop pops a value from the stack.
func (s *Stack) Pop() (float64, error) {
	if len(s.stack) == 0 {
		return 0, fmt.Errorf("stack underflow")
	}
	value := s.stack[len(s.stack)-1]
	s.stack = s.stack[:len(s.stack)-1]
	return value, nil
}

// Dup duplicates the top element.
func (s *Stack) Dup() error {
	if len(s.stack) == 0 {
		return fmt.Errorf("stack underflow")
	}
	if len(s.stack) >= maxStackSize {
		return fmt.Errorf("stack overflow")
	}
	s.stack = append(s.stack, s.stack[len(s.stack)-1])
	return nil
}

// Exch exchanges the top two elements.
func (s *Stack) Exch() error {
	if len(s.stack) < 2 {
		return fmt.Errorf("stack underflow")
	}
	n := len(s.stack)
	s.stack[n-1], s.stack[n-2] = s.stack[n-2], s.stack[n-1]
	return nil
}

// Copy copies the top n elements.
func (s *Stack) Copy(n int) error {
	if n < 0 {
		return fmt.Errorf("invalid copy count: %d", n)
	}
	if len(s.stack) < n {
		return fmt.Errorf("stack underflow")
	}
	if len(s.stack)+n > maxStackSize {
		return fmt.Errorf("stack overflow")
	}

	startIdx := len(s.stack) - n
	for i := 0; i < n; i++ {
		s.stack = append(s.stack, s.stack[startIdx+i])
	}
	return nil
}

// Index copies the nth element (0-indexed from top) to the top.
func (s *Stack) Index(n int) error {
	if n < 0 {
		return fmt.Errorf("invalid index: %d", n)
	}
	idx := len(s.stack) - n - 1
	if idx < 0 {
		return fmt.Errorf("stack underflow")
	}
	if len(s.stack) >= maxStackSize {
		return fmt.Errorf("stack overflow")
	}
	s.stack = append(s.stack, s.stack[idx])
	return nil
}

// Roll rotates the top n elements p times.
// If p is positive, rotate up; if negative, rotate down.
func (s *Stack) Roll(n, p int) error {
	if n < 0 {
		return fmt.Errorf("invalid roll count: %d", n)
	}
	if len(s.stack) < n {
		return fmt.Errorf("stack underflow")
	}
	if n == 0 || n == 1 {
		return nil // Nothing to rotate
	}

	// Normalize p to be in range [0, n)
	p %= n
	if p < 0 {
		p += n
	}
	if p == 0 {
		return nil
	}

	// Extract the top n elements
	startIdx := len(s.stack) - n
	portion := make([]float64, n)
	copy(portion, s.stack[startIdx:])

	// Rotate: move last p elements to front
	rotated := make([]float64, n)
	copy(rotated, portion[n-p:])
	copy(rotated[p:], portion[:n-p])

	// Put back
	copy(s.stack[startIdx:], rotated)

	return nil
}

// Len returns the current stack size.
func (s *Stack) Len() int {
	return len(s.stack)
}

// ToSlice returns a copy of the stack contents.
func (s *Stack) ToSlice() []float64 {
	result := make([]float64, len(s.stack))
	copy(result, s.stack)
	return result
}
