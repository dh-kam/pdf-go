package content

import (
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domaincolorspace "github.com/dh-kam/pdf-go/internal/domain/colorspace"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/graphics"
)

func TestNewStandardEvaluator_RegistersExpectedOperators(t *testing.T) {
	evaluator := NewStandardEvaluator(nil)
	registry := evaluator.GetRegistry()
	require.NotNil(t, registry)

	requiredOperators := []string{
		"q", "Q", "cm",
		"m", "l", "c", "v", "y", "h", "re",
		"S", "s", "f", "F", "f*", "B", "B*", "b", "b*", "n",
		"W", "W*",
		"BT", "ET",
		"Tc", "Tw", "Tz", "TL", "Tf", "Tr", "Ts",
		"Td", "TD", "Tm", "T*", "Tj", "TJ",
		"CS", "cs", "SC", "SCN", "scn", "G", "g", "RG", "rg", "K", "k",
		"Do", "sh", "BX", "EX",
	}

	for _, name := range requiredOperators {
		op, ok := registry.Get(name)
		assert.True(t, ok, "operator %s should be registered", name)
		assert.NotNil(t, op, "operator %s should have implementation", name)
	}

	_, ok := registry.Get("unknown")
	assert.False(t, ok, "unknown operator should not be registered")
}

func TestEvaluator_ProcessBytes_ParsesOperators(t *testing.T) {
	evaluator := NewStandardEvaluator(nil)

	data := []byte(
		"1 2 3 4 5 6 cm q 0 1 m 2 3 l 1 2 3 4 re h f " +
			"S s B B* b b* n W W* " +
			"BT 1 2 Td 3 4 TD 1 0 0 1 5 6 Tm 8 Tz 9 Tc 10 Tw 11 TL 1 12 Tf 13 Tr 14 Ts T* ET " +
			"0 g 0.2 0.4 0.6 RG 0.1 0.2 0.3 rg 0.1 0.2 0.3 0.4 K 0.4 0.3 0.2 0.1 k " +
			"1 Do sh BX EX Q x ",
	)
	// Last token 'x' is intentionally unsupported and should be ignored.
	require.NoError(t, evaluator.ProcessBytes(data))
}

func TestEvaluator_ProcessObject_AndObjectContainers(t *testing.T) {
	evaluator := NewStandardEvaluator(nil)

	stream := entity.NewStream(entity.NewDict(), []byte("1 2 3 4 5 6 cm"))
	require.NoError(t, evaluator.ProcessObject(stream))

	contents := entity.NewArray(
		stream,
		entity.NewStream(entity.NewDict(), []byte("1 2 m 3 4 l S")),
	)
	require.NoError(t, evaluator.ProcessArray(contents))

	dict := entity.NewDict()
	dict.Set(entity.Name("Contents"), contents)
	require.NoError(t, evaluator.ProcessDict(dict))

	require.NoError(t, evaluator.ProcessObject(nil))

	refErr := evaluator.ProcessObject(entity.NewRef(1, 0))
	require.Error(t, refErr)
}

func TestOperatorExecute_ValidationAndStateChanges(t *testing.T) {
	t.Run("valid operators", func(t *testing.T) {
		validCases := []struct {
			name string
			op   interface {
				Execute(state *graphics.State, operands []float64) error
			}
			operands   []float64
			setupState func(*testing.T, *graphics.State)
			assertFn   func(*testing.T, *graphics.State)
		}{
			{"SaveOperator", &SaveOperator{}, []float64{}, nil, nil},
			{"RestoreOperator", &RestoreOperator{}, []float64{}, nil, nil},
			{
				"ConcatMatrixOperator",
				&ConcatMatrixOperator{},
				[]float64{1, 0, 0, 1, 10, 20},
				nil,
				func(t *testing.T, state *graphics.State) {
					assert.Equal(t, [6]float64{1, 0, 0, 1, 10, 20}, state.GetCTM())
				},
			},
			{
				"MoveToOperator",
				&MoveToOperator{},
				[]float64{3, 4},
				nil,
				func(t *testing.T, state *graphics.State) {
					cmds := state.GetCurrentPath().GetCommands()
					require.Len(t, cmds, 1)
					_, ok := cmds[0].(*graphics.MoveTo)
					require.True(t, ok)
				},
			},
			{
				"LineToOperator",
				&LineToOperator{},
				[]float64{5, 6},
				func(_ *testing.T, state *graphics.State) {
					state.GetCurrentPath().AddCommand(&graphics.MoveTo{X: 1, Y: 2})
				},
				func(t *testing.T, state *graphics.State) {
					cmds := state.GetCurrentPath().GetCommands()
					require.Len(t, cmds, 2)
					_, ok := cmds[1].(*graphics.LineTo)
					require.True(t, ok)
				},
			},
			{
				"CurveToOperator",
				&CurveToOperator{},
				[]float64{1, 2, 3, 4, 5, 6},
				nil,
				func(t *testing.T, state *graphics.State) {
					cmds := state.GetCurrentPath().GetCommands()
					require.Len(t, cmds, 1)
					_, ok := cmds[0].(*graphics.CurveTo)
					require.True(t, ok)
				},
			},
			{
				"CurveToNoFirstControlOperator",
				&CurveToNoFirstControlOperator{},
				[]float64{1, 2, 3, 4},
				func(_ *testing.T, state *graphics.State) {
					state.GetCurrentPath().AddCommand(&graphics.MoveTo{X: 7, Y: 8})
				},
				func(t *testing.T, state *graphics.State) {
					cmds := state.GetCurrentPath().GetCommands()
					require.Len(t, cmds, 2)
					_, ok := cmds[1].(*graphics.CurveTo)
					require.True(t, ok)
				},
			},
			{
				"CurveToNoLastControlOperator",
				&CurveToNoLastControlOperator{},
				[]float64{1, 2, 3, 4},
				nil,
				func(t *testing.T, state *graphics.State) {
					cmds := state.GetCurrentPath().GetCommands()
					require.Len(t, cmds, 1)
					_, ok := cmds[0].(*graphics.CurveTo)
					require.True(t, ok)
				},
			},
			{
				"RectangleOperator",
				&RectangleOperator{},
				[]float64{1, 2, 3, 4},
				nil,
				func(t *testing.T, state *graphics.State) {
					cmds := state.GetCurrentPath().GetCommands()
					require.Len(t, cmds, 5)
				},
			},
			{"ClosePathOperator", &ClosePathOperator{}, []float64{}, nil, nil},
			{"EndPathOperator", &EndPathOperator{}, []float64{}, nil, nil},
			{
				"StrokeOperator",
				&StrokeOperator{},
				[]float64{},
				func(_ *testing.T, state *graphics.State) {
					state.GetCurrentPath().AddCommand(&graphics.LineTo{X: 1, Y: 1})
				},
				func(t *testing.T, state *graphics.State) {
					assert.Empty(t, state.GetCurrentPath().GetCommands())
				},
			},
			{"EOFillOperator", &EOFillOperator{}, []float64{}, nil, nil},
			{"FillAndStrokeOperator", &FillAndStrokeOperator{}, []float64{}, nil, nil},
			{"EOFillAndStrokeOperator", &EOFillAndStrokeOperator{}, []float64{}, nil, nil},
			{"CloseFillAndStrokeOperator", &CloseFillAndStrokeOperator{}, []float64{}, nil, nil},
			{"EOCloseFillAndStrokeOperator", &EOCloseFillAndStrokeOperator{}, []float64{}, nil, nil},
			{
				"ClipOperator",
				&ClipOperator{},
				[]float64{},
				func(_ *testing.T, state *graphics.State) {
					state.GetCurrentPath().AddCommand(&graphics.MoveTo{X: 1, Y: 1})
				},
				func(t *testing.T, state *graphics.State) {
					clipPath := state.GetClipPath()
					require.NotNil(t, clipPath)
					assert.Equal(t, 1, len(clipPath.GetCommands()))
					assert.NotSame(t, state.GetCurrentPath(), clipPath)
				},
			},
			{
				"EOClipOperator",
				&EOClipOperator{},
				[]float64{},
				func(_ *testing.T, state *graphics.State) {
					state.GetCurrentPath().AddCommand(&graphics.MoveTo{X: 1, Y: 1})
				},
				func(t *testing.T, state *graphics.State) {
					clipPath := state.GetClipPath()
					require.NotNil(t, clipPath)
					assert.Equal(t, graphics.FillRuleEvenOdd, clipPath.GetFillRule())
				},
			},
			{
				"BeginTextOperator",
				&BeginTextOperator{},
				[]float64{},
				nil,
				func(t *testing.T, state *graphics.State) {
					assert.Equal(t, [6]float64{1, 0, 0, 1, 0, 0}, state.GetTextMatrix())
					assert.Equal(t, 0.0, state.GetTextLeading())
				},
			},
			{"EndTextOperator", &EndTextOperator{}, []float64{}, nil, nil},
			{"SetCharSpacingOperator", &SetCharSpacingOperator{}, []float64{1.5}, nil, func(t *testing.T, state *graphics.State) { assert.Equal(t, 1.5, state.GetCharSpacing()) }},
			{"SetWordSpacingOperator", &SetWordSpacingOperator{}, []float64{2.5}, nil, func(t *testing.T, state *graphics.State) { assert.Equal(t, 2.5, state.GetWordSpacing()) }},
			{"SetHorizontalScalingOperator", &SetHorizontalScalingOperator{}, []float64{75}, nil, func(t *testing.T, state *graphics.State) { assert.Equal(t, 75.0, state.GetHorizontalScaling()) }},
			{"SetTextLeadingOperator", &SetTextLeadingOperator{}, []float64{11}, nil, func(t *testing.T, state *graphics.State) { assert.Equal(t, 11.0, state.GetTextLeading()) }},
			{"SetFontOperator", &SetFontOperator{}, []float64{10, 18}, nil, func(t *testing.T, state *graphics.State) { assert.Equal(t, 18.0, state.GetFontSize()) }},
			{"SetTextRenderModeOperator", &SetTextRenderModeOperator{}, []float64{2}, nil, func(t *testing.T, state *graphics.State) { assert.Equal(t, 2, state.GetTextRenderMode()) }},
			{"SetTextRiseOperator", &SetTextRiseOperator{}, []float64{1.2}, nil, func(t *testing.T, state *graphics.State) { assert.Equal(t, 1.2, state.GetTextRise()) }},
			{"MoveTextOperator", &MoveTextOperator{}, []float64{2, 3}, nil, func(t *testing.T, state *graphics.State) {
				assert.Equal(t, [6]float64{1, 0, 0, 1, 2, 3}, state.GetTextMatrix())
			}},
			{"MoveTextSetLeadingOperator", &MoveTextSetLeadingOperator{}, []float64{4, 6}, nil, func(t *testing.T, state *graphics.State) {
				assert.Equal(t, -6.0, state.GetTextLeading())
				assert.Equal(t, [6]float64{1, 0, 0, 1, 4, 6}, state.GetTextMatrix())
			}},
			{"SetTextMatrixOperator", &SetTextMatrixOperator{}, []float64{1, 2, 3, 4, 5, 6}, nil, func(t *testing.T, state *graphics.State) {
				assert.Equal(t, [6]float64{1, 2, 3, 4, 5, 6}, state.GetTextMatrix())
			}},
			{
				"MoveTextNextLineOperator",
				&MoveTextNextLineOperator{},
				[]float64{},
				func(_ *testing.T, state *graphics.State) {
					state.SetTextLeading(7)
					state.SetTextMatrix([6]float64{1, 2, 3, 4, 5, 6})
				},
				func(t *testing.T, state *graphics.State) {
					assert.Equal(t, [6]float64{1, 2, 3, 4, 5, 13}, state.GetTextMatrix())
				},
			},
			{"ShowTextOperator", &ShowTextOperator{}, []float64{1, 2}, nil, nil},
			{"ShowTextArrayOperator", &ShowTextArrayOperator{}, []float64{1, 2}, nil, nil},
			{"SetStrokeColorSpaceOperator", &SetStrokeColorSpaceOperator{}, []float64{1}, nil, nil},
			{"SetFillColorSpaceOperator", &SetFillColorSpaceOperator{}, []float64{1}, nil, nil},
			{"SetStrokeColorOperator", &SetStrokeColorOperator{}, []float64{0.25}, nil, nil},
			{"SetStrokeColorNOperator", &SetStrokeColorNOperator{}, []float64{0.25, 0.5}, nil, nil},
			{"SetFillColorNOperator", &SetFillColorNOperator{}, []float64{0.25, 0.5}, nil, nil},
			{"SetGrayStrokeOperator", &SetGrayStrokeOperator{}, []float64{0.5}, nil, nil},
			{
				"SetGrayFillOperator",
				&SetGrayFillOperator{},
				[]float64{0.5},
				nil,
				func(t *testing.T, state *graphics.State) {
					c := state.GetFillColor()
					r, g, b, _ := c.RGBA()
					assert.EqualValues(t, 128, r>>8)
					assert.EqualValues(t, 128, g>>8)
					assert.EqualValues(t, 128, b>>8)
				},
			},
			{
				"SetRGBFillOperator",
				&SetRGBFillOperator{},
				[]float64{0.2, 0.4, 0.6},
				nil,
				func(t *testing.T, state *graphics.State) {
					r, g, b, _ := state.GetFillColor().RGBA()
					assert.EqualValues(t, 51, r>>8)
					assert.EqualValues(t, 102, g>>8)
					assert.EqualValues(t, 153, b>>8)
				},
			},
			{
				"SetCMYKFillOperator",
				&SetCMYKFillOperator{},
				[]float64{0, 0, 0, 0},
				nil,
				func(t *testing.T, state *graphics.State) {
					r, g, b, _ := state.GetFillColor().RGBA()
					assert.EqualValues(t, 255, r>>8)
					assert.EqualValues(t, 255, g>>8)
					assert.EqualValues(t, 255, b>>8)
				},
			},
			{"SetCMYKStrokeOperator", &SetCMYKStrokeOperator{}, []float64{255, 0, 0, 0}, nil, nil},
			{"SetRGBStrokeOperator", &SetRGBStrokeOperator{}, []float64{1, 1, 1}, nil, nil},
			{"ExecuteXObjectOperator", &ExecuteXObjectOperator{}, []float64{1}, nil, nil},
			{"BeginCompatibilityOperator", &BeginCompatibilityOperator{}, []float64{}, nil, nil},
			{"EndCompatibilityOperator", &EndCompatibilityOperator{}, []float64{}, nil, nil},
			{"ShadingOperator", &ShadingOperator{}, []float64{}, nil, nil},
		}

		for _, tt := range validCases {
			t.Run(tt.name, func(t *testing.T) {
				state := graphics.NewState()
				if tt.setupState != nil {
					tt.setupState(t, state)
				}
				err := tt.op.Execute(state, tt.operands)
				require.NoError(t, err)
				if tt.assertFn != nil {
					tt.assertFn(t, state)
				}
			})
		}
	})

	t.Run("invalid operand counts", func(t *testing.T) {
		invalidCases := []struct {
			name string
			op   interface {
				Execute(state *graphics.State, operands []float64) error
			}
			operands []float64
		}{
			{"SaveOperator", &SaveOperator{}, []float64{1}},
			{"RestoreOperator", &RestoreOperator{}, []float64{1}},
			{"ConcatMatrixOperator", &ConcatMatrixOperator{}, []float64{1}},
			{"MoveToOperator", &MoveToOperator{}, []float64{1}},
			{"LineToOperator", &LineToOperator{}, []float64{1}},
			{"CurveToOperator", &CurveToOperator{}, []float64{1, 2}},
			{"CurveToNoFirstControlOperator", &CurveToNoFirstControlOperator{}, []float64{1}},
			{"CurveToNoLastControlOperator", &CurveToNoLastControlOperator{}, []float64{1}},
			{"ClosePathOperator", &ClosePathOperator{}, []float64{1}},
			{"RectangleOperator", &RectangleOperator{}, []float64{1, 2, 3}},
			{"EndPathOperator", &EndPathOperator{}, []float64{1}},
			{"StrokeOperator", &StrokeOperator{}, []float64{1}},
			{"CloseAndStrokeOperator", &CloseAndStrokeOperator{}, []float64{1}},
			{"FillOperator", &FillOperator{}, []float64{1}},
			{"EOFillOperator", &EOFillOperator{}, []float64{1}},
			{"FillAndStrokeOperator", &FillAndStrokeOperator{}, []float64{1}},
			{"EOFillAndStrokeOperator", &EOFillAndStrokeOperator{}, []float64{1}},
			{"CloseFillAndStrokeOperator", &CloseFillAndStrokeOperator{}, []float64{1}},
			{"EOCloseFillAndStrokeOperator", &EOCloseFillAndStrokeOperator{}, []float64{1}},
			{"ClipOperator", &ClipOperator{}, []float64{1}},
			{"EOClipOperator", &EOClipOperator{}, []float64{1}},
			{"BeginTextOperator", &BeginTextOperator{}, []float64{1}},
			{"EndTextOperator", &EndTextOperator{}, []float64{1}},
			{"SetCharSpacingOperator", &SetCharSpacingOperator{}, []float64{}},
			{"SetWordSpacingOperator", &SetWordSpacingOperator{}, []float64{}},
			{"SetHorizontalScalingOperator", &SetHorizontalScalingOperator{}, []float64{}},
			{"SetTextLeadingOperator", &SetTextLeadingOperator{}, []float64{}},
			{"SetFontOperator", &SetFontOperator{}, []float64{1}},
			{"SetTextRenderModeOperator", &SetTextRenderModeOperator{}, []float64{}},
			{"SetTextRiseOperator", &SetTextRiseOperator{}, []float64{}},
			{"MoveTextOperator", &MoveTextOperator{}, []float64{1}},
			{"MoveTextSetLeadingOperator", &MoveTextSetLeadingOperator{}, []float64{1}},
			{"SetTextMatrixOperator", &SetTextMatrixOperator{}, []float64{1, 2, 3}},
			{"MoveTextNextLineOperator", &MoveTextNextLineOperator{}, []float64{1}},
			{"SetStrokeColorSpaceOperator", &SetStrokeColorSpaceOperator{}, []float64{}},
			{"SetFillColorSpaceOperator", &SetFillColorSpaceOperator{}, []float64{}},
			{"SetGrayStrokeOperator", &SetGrayStrokeOperator{}, []float64{}},
			{"SetGrayFillOperator", &SetGrayFillOperator{}, []float64{}},
			{"SetRGBStrokeOperator", &SetRGBStrokeOperator{}, []float64{1}},
			{"SetRGBFillOperator", &SetRGBFillOperator{}, []float64{1}},
			{"SetCMYKStrokeOperator", &SetCMYKStrokeOperator{}, []float64{1}},
			{"SetCMYKFillOperator", &SetCMYKFillOperator{}, []float64{1}},
			{"SetStrokeColorOperator", &SetStrokeColorOperator{}, []float64{}},
			{"SetFillColorNOperator", &SetFillColorNOperator{}, []float64{}},
			{"SetStrokeColorNOperator", &SetStrokeColorNOperator{}, []float64{}},
			{"ExecuteXObjectOperator", &ExecuteXObjectOperator{}, []float64{}},
			{"BeginCompatibilityOperator", &BeginCompatibilityOperator{}, []float64{1}},
			{"EndCompatibilityOperator", &EndCompatibilityOperator{}, []float64{1}},
			{"ShadingOperator", &ShadingOperator{}, []float64{1}},
		}

		for _, tt := range invalidCases {
			t.Run(tt.name, func(t *testing.T) {
				state := graphics.NewState()
				err := tt.op.Execute(state, tt.operands)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "operator_")
			})
		}
	})
}

func TestHelpers_CurrentPointAndCmykConversion(t *testing.T) {
	path := graphics.NewPath()
	x, y := currentPoint(path)
	assert.Equal(t, 0.0, x)
	assert.Equal(t, 0.0, y)

	x, y = currentPoint(nil)
	assert.Equal(t, 0.0, x)
	assert.Equal(t, 0.0, y)

	path.AddCommand(&graphics.MoveTo{X: 1, Y: 2})
	path.AddCommand(&graphics.LineTo{X: 5, Y: 6})
	path.AddCommand(&graphics.CurveTo{X1: 7, Y1: 8, X2: 9, Y2: 10, X3: 11, Y3: 12})
	x, y = currentPoint(path)
	assert.Equal(t, 11.0, x)
	assert.Equal(t, 12.0, y)

	path2 := graphics.NewPath()
	path2.AddCommand(&graphics.MoveTo{X: 1, Y: 2})
	path2.AddCommand(&graphics.ClosePath{})
	x, y = currentPoint(path2)
	assert.Equal(t, 1.0, x)
	assert.Equal(t, 2.0, y)

	assert.Equal(t, color.RGBA{255, 255, 255, 255}, domaincolorspace.ConvertDeviceCMYKToRGBA([]float64{0, 0, 0, 0}))
	assert.Equal(t, color.RGBA{236, 0, 140, 255}, domaincolorspace.ConvertDeviceCMYKToRGBA([]float64{0, 1, 0, 0}))
	assert.Equal(t, color.RGBA{35, 31, 32, 255}, domaincolorspace.ConvertDeviceCMYKToRGBA([]float64{0, 0, 0, 1}))
}
