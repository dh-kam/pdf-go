package postscript

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStack(t *testing.T) {
	s := NewStack(nil)
	assert.NotNil(t, s)
	assert.Equal(t, 0, s.Len())
}

func TestNewStack_WithInitial(t *testing.T) {
	initial := []float64{1.0, 2.0, 3.0}
	s := NewStack(initial)
	assert.Equal(t, 3, s.Len())
	assert.Equal(t, initial, s.ToSlice())
}

func TestStack_PushPop(t *testing.T) {
	s := NewStack(nil)

	err := s.Push(1.0)
	require.NoError(t, err)
	assert.Equal(t, 1, s.Len())

	err = s.Push(2.0)
	require.NoError(t, err)
	assert.Equal(t, 2, s.Len())

	val, err := s.Pop()
	require.NoError(t, err)
	assert.Equal(t, 2.0, val)
	assert.Equal(t, 1, s.Len())

	val, err = s.Pop()
	require.NoError(t, err)
	assert.Equal(t, 1.0, val)
	assert.Equal(t, 0, s.Len())
}

func TestStack_PopUnderflow(t *testing.T) {
	s := NewStack(nil)
	_, err := s.Pop()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "underflow")
}

func TestStack_PushOverflow(t *testing.T) {
	s := NewStack(nil)

	// Fill stack to max
	for i := 0; i < maxStackSize; i++ {
		err := s.Push(float64(i))
		require.NoError(t, err)
	}

	// Try to push one more
	err := s.Push(999.0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "overflow")
}

func TestStack_Dup(t *testing.T) {
	s := NewStack([]float64{1.0, 2.0, 3.0})

	err := s.Dup()
	require.NoError(t, err)

	expected := []float64{1.0, 2.0, 3.0, 3.0}
	assert.Equal(t, expected, s.ToSlice())
}

func TestStack_DupUnderflow(t *testing.T) {
	s := NewStack(nil)
	err := s.Dup()
	assert.Error(t, err)
}

func TestStack_Exch(t *testing.T) {
	s := NewStack([]float64{1.0, 2.0, 3.0})

	err := s.Exch()
	require.NoError(t, err)

	expected := []float64{1.0, 3.0, 2.0}
	assert.Equal(t, expected, s.ToSlice())
}

func TestStack_ExchUnderflow(t *testing.T) {
	s := NewStack([]float64{1.0})
	err := s.Exch()
	assert.Error(t, err)
}

func TestStack_Copy(t *testing.T) {
	s := NewStack([]float64{1.0, 2.0, 3.0})

	err := s.Copy(2)
	require.NoError(t, err)

	expected := []float64{1.0, 2.0, 3.0, 2.0, 3.0}
	assert.Equal(t, expected, s.ToSlice())
}

func TestStack_CopyZero(t *testing.T) {
	s := NewStack([]float64{1.0, 2.0, 3.0})

	err := s.Copy(0)
	require.NoError(t, err)

	expected := []float64{1.0, 2.0, 3.0}
	assert.Equal(t, expected, s.ToSlice())
}

func TestStack_CopyUnderflow(t *testing.T) {
	s := NewStack([]float64{1.0, 2.0})
	err := s.Copy(3)
	assert.Error(t, err)
}

func TestStack_Index(t *testing.T) {
	s := NewStack([]float64{1.0, 2.0, 3.0, 4.0})

	// Index 0 = top element (4.0)
	err := s.Index(0)
	require.NoError(t, err)
	expected := []float64{1.0, 2.0, 3.0, 4.0, 4.0}
	assert.Equal(t, expected, s.ToSlice())

	s = NewStack([]float64{1.0, 2.0, 3.0, 4.0})
	// Index 2 = third from top (2.0)
	err = s.Index(2)
	require.NoError(t, err)
	expected = []float64{1.0, 2.0, 3.0, 4.0, 2.0}
	assert.Equal(t, expected, s.ToSlice())
}

func TestStack_IndexUnderflow(t *testing.T) {
	s := NewStack([]float64{1.0, 2.0})
	err := s.Index(5)
	assert.Error(t, err)
}

func TestStack_Roll(t *testing.T) {
	tests := []struct {
		name     string
		initial  []float64
		expected []float64
		n        int
		p        int
	}{
		{
			name:     "roll 3 elements 1 time",
			initial:  []float64{1.0, 2.0, 3.0, 4.0, 5.0},
			n:        3,
			p:        1,
			expected: []float64{1.0, 2.0, 5.0, 3.0, 4.0},
		},
		{
			name:     "roll 3 elements -1 time",
			initial:  []float64{1.0, 2.0, 3.0, 4.0, 5.0},
			n:        3,
			p:        -1,
			expected: []float64{1.0, 2.0, 4.0, 5.0, 3.0},
		},
		{
			name:     "roll 5 elements 2 times",
			initial:  []float64{1.0, 2.0, 3.0, 4.0, 5.0},
			n:        5,
			p:        2,
			expected: []float64{4.0, 5.0, 1.0, 2.0, 3.0},
		},
		{
			name:     "roll 0 elements",
			initial:  []float64{1.0, 2.0, 3.0},
			n:        0,
			p:        1,
			expected: []float64{1.0, 2.0, 3.0},
		},
		{
			name:     "roll 1 element",
			initial:  []float64{1.0, 2.0, 3.0},
			n:        1,
			p:        5,
			expected: []float64{1.0, 2.0, 3.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStack(tt.initial)
			err := s.Roll(tt.n, tt.p)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, s.ToSlice())
		})
	}
}

func TestStack_RollUnderflow(t *testing.T) {
	s := NewStack([]float64{1.0, 2.0})
	err := s.Roll(3, 1)
	assert.Error(t, err)
}

func TestStack_ToSlice(t *testing.T) {
	original := []float64{1.0, 2.0, 3.0}
	s := NewStack(original)

	result := s.ToSlice()
	assert.Equal(t, original, result)

	// Ensure it's a copy, not the same slice
	result[0] = 999.0
	assert.Equal(t, 1.0, s.ToSlice()[0])
}
