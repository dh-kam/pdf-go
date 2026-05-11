package graphics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStateDefaults(t *testing.T) {
	state := NewState()

	assert.Equal(t, [6]float64{1, 0, 0, 1, 0, 0}, state.GetCTM())
	assert.Equal(t, 1.0, state.GetLineWidth())
	assert.Equal(t, 100.0, state.GetHorizontalScaling())
	assert.Equal(t, 0, state.GetLineCap())
	assert.Equal(t, 0, state.GetLineJoin())
	assert.Equal(t, 12.0, state.GetFontSize())
	assert.NotNil(t, state.GetCurrentPath())
}

func TestStateSaveRestore(t *testing.T) {
	state := NewState()
	state.SetLineWidth(2.5)
	state.Translate(2, 3)

	saved := state.Save()
	saved.SetLineWidth(5)
	saved.Translate(10, 20)

	restored := saved.Restore()
	assert.Equal(t, 2.5, restored.GetLineWidth())
	assert.Equal(t, 0.0, restored.GetTextLeading())
	assert.Equal(t, [6]float64{1, 0, 0, 1, 2, 3}, restored.GetCTM())
}

func TestStateTransformMatrix(t *testing.T) {
	state := NewState()
	state.Translate(3, 4)
	state.Scale(2, 3)

	got := state.GetCTM()
	assert.Equal(t, 2.0, got[0])
	assert.Equal(t, 0.0, got[1])
	assert.Equal(t, 0.0, got[2])
	assert.Equal(t, 3.0, got[3])
	assert.Equal(t, 6.0, got[4])
	assert.Equal(t, 12.0, got[5])

	state = NewState()
	state.Rotate(90)
	got = state.GetCTM()
	assert.InDelta(t, 0.0, got[0], 1e-12)
	assert.InDelta(t, 1.0, got[1], 1e-12)
	assert.InDelta(t, -1.0, got[2], 1e-12)
	assert.InDelta(t, 0.0, got[3], 1e-12)
}

func TestSetMiterLimitClampsToOne(t *testing.T) {
	state := NewState()
	state.SetMiterLimit(0.5)
	assert.Equal(t, 1.0, state.GetMiterLimit())
	state.SetMiterLimit(3)
	assert.Equal(t, 3.0, state.GetMiterLimit())
}

func TestPathCommands(t *testing.T) {
	path := NewPath()
	assert.Equal(t, FillRuleNonZero, path.GetFillRule())

	path.AddCommand(&MoveTo{X: 1, Y: 2})
	path.AddCommand(&LineTo{X: 4, Y: 6})
	path.AddCommand(&ClosePath{})
	path.SetFillRule(FillRuleEvenOdd)

	assert.Equal(t, FillRuleEvenOdd, path.GetFillRule())
	assert.Equal(t, 3, len(path.GetCommands()))

	clone := path.Clone()
	assert.Equal(t, path.GetFillRule(), clone.GetFillRule())
	assert.Equal(t, len(path.GetCommands()), len(clone.GetCommands()))

	xMin, yMin, xMax, yMax := path.GetBounds()
	assert.Equal(t, 1.0, xMin)
	assert.Equal(t, 2.0, yMin)
	assert.Equal(t, 4.0, xMax)
	assert.Equal(t, 6.0, yMax)

	rect := Rectangle(1, 2, 3, 4)
	require.NotNil(t, rect)
	assert.Equal(t, 5, len(rect.GetCommands()))
}

func TestCurveBounds(t *testing.T) {
	path := NewPath()
	path.AddCommand(&CurveTo{X1: 1, Y1: 2, X2: 3, Y2: 4, X3: -1, Y3: 10})

	xMin, yMin, xMax, yMax := path.GetBounds()
	assert.Equal(t, -1.0, xMin)
	assert.Equal(t, 2.0, yMin)
	assert.Equal(t, 3.0, xMax)
	assert.Equal(t, 10.0, yMax)
}

func TestPathGetCommandsReturnsSlice(t *testing.T) {
	path := NewPath()
	tb := path.GetCommands()
	assert.Len(t, tb, 0)
}

func TestMatrixHelpers(t *testing.T) {
	ident := IdentityMatrix()
	assert.Equal(t, [6]float64{1, 0, 0, 1, 0, 0}, ident)

	mul := MultiplyMatrix([6]float64{1, 0, 0, 2, 3, 4}, [6]float64{0, 1, 0, 1, 5, 6})
	assert.Equal(t, [6]float64{0, 2, 0, 2, 8, 16}, mul)

	inv, err := InverseMatrix([6]float64{1, 0, 0, 1, 0, 0})
	require.NoError(t, err)
	assert.Equal(t, [6]float64{1, 0, 0, 1, 0, 0}, inv)

	_, err = InverseMatrix([6]float64{1, 1, 2, 2, 0, 0})
	require.Error(t, err)

	tx, ty := TransformPoint([6]float64{1, 0, 0, 1, 5, 6}, 3, 4)
	assert.Equal(t, 8.0, tx)
	assert.Equal(t, 10.0, ty)
}

func TestPathCommandExecuteMethods(t *testing.T) {
	state := NewState()

	x, y := (&MoveTo{X: 3, Y: 4}).Execute(state)
	assert.Equal(t, 3.0, x)
	assert.Equal(t, 4.0, y)

	x, y = (&LineTo{X: 5, Y: 6}).Execute(state)
	assert.Equal(t, 5.0, x)
	assert.Equal(t, 6.0, y)

	x, y = (&CurveTo{X1: 1, Y1: 2, X2: 3, Y2: 4, X3: 7, Y3: 8}).Execute(state)
	assert.Equal(t, 7.0, x)
	assert.Equal(t, 8.0, y)

	x, y = (&ClosePath{}).Execute(state)
	assert.Equal(t, 0.0, x)
	assert.Equal(t, 0.0, y)
}
