package content_test

import (
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/content"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
	"github.com/dh-kam/pdf-go/internal/domain/graphics"
	infrastructureContent "github.com/dh-kam/pdf-go/internal/infrastructure/content"
)

func TestEvaluator_NewEvaluator(t *testing.T) {
	ev := content.NewEvaluator(nil)
	assert.NotNil(t, ev)
	assert.NotNil(t, ev.GetState())
}

func TestEvaluator_GetSetState(t *testing.T) {
	ev := content.NewEvaluator(nil)

	state := graphics.NewState()
	ev.SetState(state)

	assert.Equal(t, state, ev.GetState())
}

func TestOperatorRegistry_Register(t *testing.T) {
	reg := content.NewOperatorRegistry()

	op := &MockOperator{}
	reg.Register("TEST", op)

	retrieved, ok := reg.Get("TEST")
	assert.True(t, ok)
	assert.Equal(t, op, retrieved)
}

func TestOperatorRegistry_GetUnknown(t *testing.T) {
	reg := content.NewOperatorRegistry()

	_, ok := reg.Get("UNKNOWN")
	assert.False(t, ok)
}

func TestGraphicsState_NewState(t *testing.T) {
	state := graphics.NewState()

	assert.NotNil(t, state)
	assert.Equal(t, 1.0, state.GetLineWidth())
	assert.Equal(t, 0.0, state.GetCharSpacing())
}

func TestGraphicsState_SaveRestore(t *testing.T) {
	state := graphics.NewState()
	state.SetLineWidth(5.0)

	savedState := state.Save()
	savedState.SetLineWidth(10.0)

	// Original state should still have line width 5.0
	assert.Equal(t, 5.0, state.GetLineWidth())
	// Saved state should have line width 10.0
	assert.Equal(t, 10.0, savedState.GetLineWidth())

	// Restore should return parent
	restoredState := savedState.Restore()
	assert.Equal(t, state, restoredState)
}

func TestGraphicsState_Transform(t *testing.T) {
	state := graphics.NewState()

	// Test translation
	state.Translate(10, 20)
	ctm := state.GetCTM()
	assert.Equal(t, 10.0, ctm[4]) // tx = 1*0 + 0*0 + 10 = 10
	assert.Equal(t, 20.0, ctm[5]) // ty = 0*0 + 1*0 + 20 = 20

	// Test scaling
	state = graphics.NewState()
	state.Scale(2.0, 3.0)
	ctm = state.GetCTM()
	assert.Equal(t, 2.0, ctm[0]) // a = 2
	assert.Equal(t, 3.0, ctm[3]) // d = 3

	// Test rotation (90 degrees)
	state = graphics.NewState()
	state.Rotate(90.0)
	ctm = state.GetCTM()
	assert.InDelta(t, 0.0, ctm[0], 1e-9)
	assert.InDelta(t, 1.0, ctm[1], 1e-9)
	assert.InDelta(t, -1.0, ctm[2], 1e-9)
	assert.InDelta(t, 0.0, ctm[3], 1e-9)
}

func TestGraphicsState_Color(t *testing.T) {
	state := graphics.NewState()

	// Test line width
	state.SetLineWidth(5.0)
	assert.Equal(t, 5.0, state.GetLineWidth())

	// Test line cap
	state.SetLineCap(1)
	assert.Equal(t, 1, state.GetLineCap())

	// Test line join
	state.SetLineJoin(2)
	assert.Equal(t, 2, state.GetLineJoin())
}

func TestGraphicsState_Text(t *testing.T) {
	state := graphics.NewState()

	// Test font size
	state.SetFontSize(14.0)
	assert.Equal(t, 14.0, state.GetFontSize())

	// Test text leading
	state.SetTextLeading(2.0)
	assert.Equal(t, 2.0, state.GetTextLeading())

	// Test character spacing
	state.SetCharSpacing(0.5)
	assert.Equal(t, 0.5, state.GetCharSpacing())

	// Test word spacing
	state.SetWordSpacing(1.5)
	assert.Equal(t, 1.5, state.GetWordSpacing())

	// Test text matrix
	tm := [6]float64{1, 0, 0, 1, 100, 200}
	state.SetTextMatrix(tm)
	assert.Equal(t, tm, state.GetTextMatrix())

	// Test text render mode
	state.SetTextRenderMode(2)
	assert.Equal(t, 2, state.GetTextRenderMode())
}

func TestPath_NewPath(t *testing.T) {
	path := graphics.NewPath()

	assert.NotNil(t, path)
	assert.NotNil(t, path.GetCommands())
	assert.Equal(t, 0, len(path.GetCommands()))
}

func TestPath_Rectangle(t *testing.T) {
	path := graphics.Rectangle(10, 20, 30, 40)

	commands := path.GetCommands()
	assert.Equal(t, 5, len(commands)) // MoveTo, 3x LineTo, ClosePath
}

func TestPath_Clone(t *testing.T) {
	path := graphics.NewPath()
	path.AddCommand(&graphics.MoveTo{X: 10, Y: 20})
	path.AddCommand(&graphics.LineTo{X: 30, Y: 40})

	cloned := path.Clone()
	assert.Equal(t, len(path.GetCommands()), len(cloned.GetCommands()))

	// Modify original
	path.AddCommand(&graphics.LineTo{X: 50, Y: 60})

	// Clone should be unaffected
	assert.Equal(t, 2, len(cloned.GetCommands()))
	assert.Equal(t, 3, len(path.GetCommands()))
}

func TestPath_GetBounds(t *testing.T) {
	path := graphics.Rectangle(10, 20, 30, 40)

	xMin, yMin, xMax, yMax := path.GetBounds()

	assert.Equal(t, 10.0, xMin)
	assert.Equal(t, 20.0, yMin)
	assert.Equal(t, 40.0, xMax)
	assert.Equal(t, 60.0, yMax)
}

func TestPath_FillRule(t *testing.T) {
	path := graphics.NewPath()

	// Default fill rule
	assert.Equal(t, graphics.FillRuleNonZero, path.GetFillRule())

	// Set fill rule
	path.SetFillRule(graphics.FillRuleEvenOdd)
	assert.Equal(t, graphics.FillRuleEvenOdd, path.GetFillRule())
}

func TestMatrixOperations(t *testing.T) {
	// Test identity matrix
	identity := graphics.IdentityMatrix()
	assert.Equal(t, [6]float64{1, 0, 0, 1, 0, 0}, identity)

	// Test matrix multiplication
	a := [6]float64{1, 2, 3, 4, 5, 6}
	b := [6]float64{7, 8, 9, 10, 11, 12}

	result := graphics.MultiplyMatrix(a, b)
	// Manual calculation:
	// result[0] = 1*7 + 3*8 = 7 + 24 = 31
	// result[1] = 2*7 + 4*8 = 14 + 32 = 46
	// etc.
	assert.NotEqual(t, [6]float64{0, 0, 0, 0, 0, 0}, result)

	// Test point transformation
	m := [6]float64{2, 0, 0, 2, 10, 20}
	x, y := graphics.TransformPoint(m, 5, 10)
	assert.Equal(t, 20.0, x) // 2*5 + 0*10 + 10 = 20
	assert.Equal(t, 40.0, y) // 0*5 + 2*10 + 20 = 40
}

func TestOperatorLexer_Empty(t *testing.T) {
	data := []byte("")

	lexer := content.NewOperatorLexer(data)

	_, _, err := lexer.NextOperator()
	assert.ErrorIs(t, err, io.EOF)
}

func TestOperatorLexer_Comments(t *testing.T) {
	data := []byte("% This is a comment\nBT ET")

	lexer := content.NewOperatorLexer(data)

	op, operands, err := lexer.NextOperator()
	require.NoError(t, err)
	assert.Equal(t, "BT", op)
	assert.Equal(t, 0, len(operands))
}

func TestOperatorLexer_Numbers(t *testing.T) {
	tests := []struct {
		data    string
		op      string
		numbers []float64
	}{
		{"10 20 Td", "Td", []float64{10, 20}},
		{"1.5 2.5 Tc", "Tc", []float64{1.5, 2.5}},
		{"-5 cm", "cm", []float64{-5}},
	}

	for _, tt := range tests {
		t.Run(tt.data, func(t *testing.T) {
			lexer := content.NewOperatorLexer([]byte(tt.data))

			op, operands, err := lexer.NextOperator()
			require.NoError(t, err)

			if tt.op == "" {
				// Should have read just a number
				assert.Equal(t, 1, len(operands))
			} else {
				assert.Equal(t, tt.op, op)
				assert.Equal(t, tt.numbers, operands)
			}
		})
	}
}

func TestOperators_Execute(t *testing.T) {
	t.Run("MoveTo", func(t *testing.T) {
		state := graphics.NewState()
		op := &infrastructureContent.MoveToOperator{}
		err := op.Execute(state, []float64{10, 20})
		require.NoError(t, err)

		path := state.GetCurrentPath()
		commands := path.GetCommands()
		assert.Equal(t, 1, len(commands))
	})

	t.Run("LineTo", func(t *testing.T) {
		state := graphics.NewState()
		op := &infrastructureContent.LineToOperator{}
		err := op.Execute(state, []float64{30, 40})
		require.NoError(t, err)

		path := state.GetCurrentPath()
		commands := path.GetCommands()
		assert.Equal(t, 1, len(commands))
	})

	t.Run("CurveTo", func(t *testing.T) {
		state := graphics.NewState()
		op := &infrastructureContent.CurveToOperator{}
		err := op.Execute(state, []float64{10, 20, 30, 40, 50, 60})
		require.NoError(t, err)

		path := state.GetCurrentPath()
		commands := path.GetCommands()
		assert.Equal(t, 1, len(commands))
	})

	t.Run("CurveToNoFirstControl", func(t *testing.T) {
		state := graphics.NewState()
		move := &infrastructureContent.MoveToOperator{}
		require.NoError(t, move.Execute(state, []float64{5, 6}))

		op := &infrastructureContent.CurveToNoFirstControlOperator{}
		err := op.Execute(state, []float64{10, 20, 30, 40})
		require.NoError(t, err)

		commands := state.GetCurrentPath().GetCommands()
		require.Len(t, commands, 2)
		curve, ok := commands[1].(*graphics.CurveTo)
		require.True(t, ok)
		assert.Equal(t, 5.0, curve.X1)
		assert.Equal(t, 6.0, curve.Y1)
		assert.Equal(t, 10.0, curve.X2)
		assert.Equal(t, 20.0, curve.Y2)
		assert.Equal(t, 30.0, curve.X3)
		assert.Equal(t, 40.0, curve.Y3)
	})

	t.Run("CurveToNoLastControl", func(t *testing.T) {
		state := graphics.NewState()
		move := &infrastructureContent.MoveToOperator{}
		require.NoError(t, move.Execute(state, []float64{1, 2}))

		op := &infrastructureContent.CurveToNoLastControlOperator{}
		err := op.Execute(state, []float64{11, 12, 13, 14})
		require.NoError(t, err)

		commands := state.GetCurrentPath().GetCommands()
		require.Len(t, commands, 2)
		curve, ok := commands[1].(*graphics.CurveTo)
		require.True(t, ok)
		assert.Equal(t, 11.0, curve.X1)
		assert.Equal(t, 12.0, curve.Y1)
		assert.Equal(t, 13.0, curve.X2)
		assert.Equal(t, 14.0, curve.Y2)
		assert.Equal(t, 13.0, curve.X3)
		assert.Equal(t, 14.0, curve.Y3)
	})

	t.Run("ClosePath", func(t *testing.T) {
		state := graphics.NewState()
		op := &infrastructureContent.ClosePathOperator{}
		err := op.Execute(state, []float64{})
		require.NoError(t, err)

		path := state.GetCurrentPath()
		commands := path.GetCommands()
		assert.Equal(t, 1, len(commands))
	})

	t.Run("Rectangle", func(t *testing.T) {
		state := graphics.NewState()
		op := &infrastructureContent.RectangleOperator{}
		err := op.Execute(state, []float64{10, 20, 30, 40})
		require.NoError(t, err)

		commands := state.GetCurrentPath().GetCommands()
		require.Len(t, commands, 5)
		_, ok := commands[0].(*graphics.MoveTo)
		assert.True(t, ok)
		_, ok = commands[1].(*graphics.LineTo)
		assert.True(t, ok)
		_, ok = commands[4].(*graphics.ClosePath)
		assert.True(t, ok)
	})

	t.Run("Text operators", func(t *testing.T) {
		t.Run("SetFont", func(t *testing.T) {
			state := graphics.NewState()
			op := &infrastructureContent.SetFontOperator{}
			err := op.Execute(state, []float64{1, 12})
			require.NoError(t, err)
			assert.Equal(t, 12.0, state.GetFontSize())
		})

		t.Run("SetCharSpacing", func(t *testing.T) {
			state := graphics.NewState()
			op := &infrastructureContent.SetCharSpacingOperator{}
			err := op.Execute(state, []float64{0.5})
			require.NoError(t, err)
			assert.Equal(t, 0.5, state.GetCharSpacing())
		})

		t.Run("SetHorizontalScaling", func(t *testing.T) {
			state := graphics.NewState()
			op := &infrastructureContent.SetHorizontalScalingOperator{}
			err := op.Execute(state, []float64{85})
			require.NoError(t, err)
			assert.Equal(t, 85.0, state.GetHorizontalScaling())
		})

		t.Run("MoveText", func(t *testing.T) {
			state := graphics.NewState()
			op := &infrastructureContent.MoveTextOperator{}
			err := op.Execute(state, []float64{10, 20})
			require.NoError(t, err)
			// Td translates the text matrix by (tx, ty)
			tm := state.GetTextMatrix()
			assert.Equal(t, 10.0, tm[4]) // tx translation
			assert.Equal(t, 20.0, tm[5]) // ty translation
		})

		t.Run("MoveTextSetLeading", func(t *testing.T) {
			state := graphics.NewState()
			op := &infrastructureContent.MoveTextSetLeadingOperator{}
			err := op.Execute(state, []float64{10, 20})
			require.NoError(t, err)
			// TD sets leading to -ty and translates by (tx, ty)
			assert.Equal(t, -20.0, state.GetTextLeading())
		})

		t.Run("MoveTextNextLine", func(t *testing.T) {
			state := graphics.NewState()
			state.SetTextLeading(-14)
			op := &infrastructureContent.MoveTextNextLineOperator{}
			err := op.Execute(state, []float64{})
			require.NoError(t, err)
			tm := state.GetTextMatrix()
			assert.Equal(t, -14.0, tm[5])
		})
	})
}

func TestEvaluator_ProcessDict_Contents(t *testing.T) {
	ev := infrastructureContent.NewStandardEvaluator(nil)
	dict := entity.NewDict()
	dict.Set(entity.Name("Contents"), entity.NewStream(entity.NewDict(), []byte("10 20 m")))

	err := ev.ProcessDict(dict)
	require.NoError(t, err)
	assert.Len(t, ev.GetState().GetCurrentPath().GetCommands(), 1)
}

func TestOperators_ErrorHandling(t *testing.T) {
	state := graphics.NewState()

	tests := []struct {
		name     string
		operator interface{}
		operands []float64
	}{
		{"MoveTo wrong operands", &infrastructureContent.MoveToOperator{}, []float64{10}},       // Too few
		{"LineTo wrong operands", &infrastructureContent.LineToOperator{}, []float64{10}},       // Too few
		{"CurveTo wrong operands", &infrastructureContent.CurveToOperator{}, []float64{10, 20}}, // Too few
		{"Rectangle wrong operands", &infrastructureContent.RectangleOperator{}, []float64{1, 2, 3}},
		{"SetFont wrong operands", &infrastructureContent.SetFontOperator{}, []float64{1}}, // Too few
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.operator.(interface {
				Execute(state *graphics.State, operands []float64) error
			}).Execute(state, tt.operands)

			assert.Error(t, err)
			assert.IsType(t, &errors.PDFError{}, err)
		})
	}
}

// Mock operator for testing
type MockOperator struct{}

func (m *MockOperator) Execute(state *graphics.State, operands []float64) error {
	return nil
}

type mockEvalXRef struct {
	objects map[entity.Ref]entity.Object
}

func (m *mockEvalXRef) Fetch(ref entity.Ref) (entity.Object, error) {
	if m.objects == nil {
		return nil, nil
	}
	return m.objects[ref], nil
}

func TestEvaluator_ProcessObject_ResolveRef(t *testing.T) {
	ref := entity.NewRef(10, 0)
	streamObj := entity.NewStream(entity.NewDict(), []byte("10 20 m"))
	xref := &mockEvalXRef{
		objects: map[entity.Ref]entity.Object{
			ref: streamObj,
		},
	}

	ev := infrastructureContent.NewStandardEvaluator(xref)
	err := ev.ProcessObject(ref)
	require.NoError(t, err)
	assert.Len(t, ev.GetState().GetCurrentPath().GetCommands(), 1)
}
