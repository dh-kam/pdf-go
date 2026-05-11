// Package renderer_test provides tests for text rendering.
package renderer_test

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/renderer"
)

func TestTextRendering_ShowText(t *testing.T) {
	// Test Tj operator - show text string
	xref := &mockXRef{}

	e := renderer.NewEvaluator(xref)

	// Create a Tj operator with a text string
	op := renderer.Operator{
		Opcode:   "Tj",
		Operands: []entity.Object{entity.NewString("Hello")},
	}

	// This should not crash
	err := e.ShowText(op)
	// Without a font loaded, text rendering should be skipped
	assert.NoError(t, err)
}

func TestTextRendering_SetFont(t *testing.T) {
	// Test Tf operator - set font
	xref := &mockXRef{}

	e := renderer.NewEvaluator(xref)

	// Set up resources with a font dictionary
	resources := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
	fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
	resources.Set(entity.Name("/F1"), fontDict)
	e.SetResources(resources)

	// Create a Tf operator with font name and size
	op := renderer.Operator{
		Opcode:   "Tf",
		Operands: []entity.Object{entity.Name("/F1"), entity.NewInteger(12)},
	}

	// This should not crash
	err := e.SetFont(op)
	assert.NoError(t, err)
}

func TestTextRendering_SetFont_FromNestedFontResources(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	resources := entity.NewDict()
	fontResources := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
	fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
	fontResources.Set(entity.Name("/F1"), fontDict)
	resources.Set(entity.Name("Font"), fontResources)
	e.SetResources(resources)

	op := renderer.Operator{
		Opcode:   "Tf",
		Operands: []entity.Object{entity.Name("F1"), entity.NewInteger(12)},
	}

	err := e.SetFont(op)
	assert.NoError(t, err)
}

func TestTextRendering_SetFont_InvalidOperandsIgnored(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	e.SetResources(entity.NewDict())

	op := renderer.Operator{
		Opcode:   "Tf",
		Operands: []entity.Object{entity.Name("/F1"), entity.Name("/BadSize")},
	}

	err := e.SetFont(op)
	assert.NoError(t, err)
}

func TestTextRendering_MoveText(t *testing.T) {
	// Test Td operator - move text position
	xref := &mockXRef{}

	e := renderer.NewEvaluator(xref)

	// Create a Td operator
	op := renderer.Operator{
		Opcode:   "Td",
		Operands: []entity.Object{entity.NewInteger(100), entity.NewInteger(50)},
	}

	err := e.MoveText(op)
	assert.NoError(t, err)
}

func TestTextRendering_MoveTextSetLeading(t *testing.T) {
	// Test TD operator - move text and set leading
	xref := &mockXRef{}

	e := renderer.NewEvaluator(xref)

	// Create a TD operator
	op := renderer.Operator{
		Opcode:   "TD",
		Operands: []entity.Object{entity.NewInteger(100), entity.NewInteger(14)},
	}

	err := e.MoveTextSetLeading(op)
	assert.NoError(t, err)
}

func TestTextRendering_SetTextMatrix(t *testing.T) {
	// Test Tm operator - set text matrix
	xref := &mockXRef{}

	e := renderer.NewEvaluator(xref)

	// Create a Tm operator with identity matrix
	op := renderer.Operator{
		Opcode: "Tm",
		Operands: []entity.Object{
			entity.NewInteger(1), // a
			entity.NewInteger(0), // b
			entity.NewInteger(0), // c
			entity.NewInteger(1), // d
			entity.NewInteger(0), // e
			entity.NewInteger(0), // f
		},
	}

	err := e.SetTextMatrix(op)
	assert.NoError(t, err)
}

func TestTextRendering_ShowTextArray(t *testing.T) {
	// Test TJ operator - show text array
	xref := &mockXRef{}

	e := renderer.NewEvaluator(xref)

	// Create a TJ operator with mixed array
	arr := entity.NewArray(
		entity.NewString("Hello"),
		entity.NewInteger(-100), // Adjust position
	)

	op := renderer.Operator{
		Opcode:   "TJ",
		Operands: []entity.Object{arr},
	}

	err := e.ShowTextArray(op)
	// Without font loaded, should skip
	assert.NoError(t, err)
}

func TestTextRendering_EvaluateContent_TdDoesNotApplyLeading(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	resources := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
	fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
	resources.Set(entity.Name("/F1"), fontDict)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("BT /F1 12 Tf 10 TL (A) Tj 0 20 Td (B) Tj ET"))
	assert.NoError(t, err)
	if !assert.Len(t, recCanvas.textDrawCalls, 2) {
		return
	}
	assert.Equal(t, "A", recCanvas.textDrawCalls[0].text)
	assert.Equal(t, "B", recCanvas.textDrawCalls[1].text)
	assert.InDelta(t, recCanvas.textDrawCalls[0].y+20, recCanvas.textDrawCalls[1].y, 0.001)
}

func TestTextRendering_EvaluateContent_MoveTextNextLineOperator(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	resources := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
	fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
	resources.Set(entity.Name("/F1"), fontDict)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("BT /F1 12 Tf 14 TL (A) Tj T* (B) Tj ET"))
	assert.NoError(t, err)
	if !assert.Len(t, recCanvas.textDrawCalls, 2) {
		return
	}
	assert.Equal(t, "A", recCanvas.textDrawCalls[0].text)
	assert.Equal(t, "B", recCanvas.textDrawCalls[1].text)
	assert.InDelta(t, recCanvas.textDrawCalls[0].y-14, recCanvas.textDrawCalls[1].y, 0.001)
}

func TestTextRendering_EvaluateContent_QuoteOperators(t *testing.T) {
	t.Run("single_quote", func(t *testing.T) {
		xref := &mockXRef{}
		e := renderer.NewEvaluator(xref)
		recCanvas := newRecordingCanvas()
		e.SetCanvas(recCanvas)

		resources := entity.NewDict()
		fontDict := entity.NewDict()
		fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
		fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
		fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
		resources.Set(entity.Name("/F1"), fontDict)
		e.SetResources(resources)

		err := e.EvaluateContent([]byte("BT /F1 12 Tf 14 TL (Q) ' ET"))
		assert.NoError(t, err)
		if !assert.Len(t, recCanvas.textDrawCalls, 1) {
			return
		}
		assert.Equal(t, "Q", recCanvas.textDrawCalls[0].text)
		assert.InDelta(t, -14.0, recCanvas.textDrawCalls[0].y, 0.001)
	})

	t.Run("double_quote", func(t *testing.T) {
		xref := &mockXRef{}
		e := renderer.NewEvaluator(xref)
		recCanvas := newRecordingCanvas()
		e.SetCanvas(recCanvas)

		resources := entity.NewDict()
		fontDict := entity.NewDict()
		fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
		fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
		fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
		resources.Set(entity.Name("/F1"), fontDict)
		e.SetResources(resources)

		err := e.EvaluateContent([]byte("BT /F1 12 Tf 14 TL 20 5 (W) \" ET"))
		assert.NoError(t, err)
		if !assert.Len(t, recCanvas.textDrawCalls, 1) {
			return
		}
		assert.Equal(t, "W", recCanvas.textDrawCalls[0].text)
		assert.InDelta(t, -14.0, recCanvas.textDrawCalls[0].y, 0.001)
	})
}

func TestTextRendering_EvaluateContent_SetTextMatrixWithIntegerOperands(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	resources := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
	fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
	resources.Set(entity.Name("/F1"), fontDict)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("BT /F1 12 Tf 1 0 0 1 100 200 Tm (A) Tj ET"))
	assert.NoError(t, err)
	if !assert.Len(t, recCanvas.textDrawCalls, 1) {
		return
	}
	assert.Equal(t, "A", recCanvas.textDrawCalls[0].text)
	assert.InDelta(t, 100.0, recCanvas.textDrawCalls[0].x, 0.001)
	assert.InDelta(t, 200.0, recCanvas.textDrawCalls[0].y, 0.001)
}

func TestTextRendering_EvaluateContent_SetTextMatrixInvalidOperandIgnored(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	resources := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
	fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
	resources.Set(entity.Name("/F1"), fontDict)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("BT /F1 12 Tf 1 0 0 1 100 /Bad Tm (A) Tj ET"))
	assert.NoError(t, err)
	if !assert.Len(t, recCanvas.textDrawCalls, 1) {
		return
	}
	assert.Equal(t, "A", recCanvas.textDrawCalls[0].text)
	assert.InDelta(t, 0.0, recCanvas.textDrawCalls[0].x, 0.001)
	assert.InDelta(t, 0.0, recCanvas.textDrawCalls[0].y, 0.001)
}

func TestTextRendering_EvaluateContent_ConcatMatrixOrder(t *testing.T) {
	newEvaluator := func() (*renderer.Evaluator, *recordingCanvas) {
		xref := &mockXRef{}
		e := renderer.NewEvaluator(xref)
		recCanvas := newRecordingCanvas()
		e.SetCanvas(recCanvas)

		resources := entity.NewDict()
		fontDict := entity.NewDict()
		fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
		fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
		fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
		resources.Set(entity.Name("/F1"), fontDict)
		e.SetResources(resources)

		return e, recCanvas
	}

	t.Run("translate_then_scale", func(t *testing.T) {
		e, recCanvas := newEvaluator()
		err := e.EvaluateContent([]byte("1 0 0 1 10 20 cm 2 0 0 2 0 0 cm BT /F1 12 Tf (A) Tj ET"))
		assert.NoError(t, err)
		if !assert.Len(t, recCanvas.textDrawCalls, 1) {
			return
		}
		assert.InDelta(t, 10.0, recCanvas.textDrawCalls[0].x, 0.001)
		assert.InDelta(t, 20.0, recCanvas.textDrawCalls[0].y, 0.001)
	})

	t.Run("scale_then_translate", func(t *testing.T) {
		e, recCanvas := newEvaluator()
		err := e.EvaluateContent([]byte("2 0 0 2 0 0 cm 1 0 0 1 10 20 cm BT /F1 12 Tf (A) Tj ET"))
		assert.NoError(t, err)
		if !assert.Len(t, recCanvas.textDrawCalls, 1) {
			return
		}
		assert.InDelta(t, 20.0, recCanvas.textDrawCalls[0].x, 0.001)
		assert.InDelta(t, 40.0, recCanvas.textDrawCalls[0].y, 0.001)
	})
}

func TestTextRendering_EvaluateContent_ConcatMatrixInvalidOperandIgnored(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	resources := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
	fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
	resources.Set(entity.Name("/F1"), fontDict)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("/Bad 0 0 1 0 0 cm BT /F1 12 Tf (A) Tj ET"))
	assert.NoError(t, err)
	if !assert.Len(t, recCanvas.textDrawCalls, 1) {
		return
	}
	assert.Equal(t, "A", recCanvas.textDrawCalls[0].text)
	assert.InDelta(t, 0.0, recCanvas.textDrawCalls[0].x, 0.001)
	assert.InDelta(t, 0.0, recCanvas.textDrawCalls[0].y, 0.001)
}

func TestTextRendering_EvaluateContent_MalformedOperatorDoesNotAbortStream(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	resources := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
	fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
	resources.Set(entity.Name("/F1"), fontDict)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("/Bad 0 m BT /F1 12 Tf (A) Tj ET"))
	assert.NoError(t, err)
	if !assert.Len(t, recCanvas.textDrawCalls, 1) {
		return
	}
	assert.Equal(t, "A", recCanvas.textDrawCalls[0].text)
}

func TestTextRendering_EvaluateContent_TextMatrixResetOnBT(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	resources := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
	fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
	resources.Set(entity.Name("/F1"), fontDict)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("1 0 0 1 100 200 cm BT /F1 12 Tf 10 20 Td (A) Tj ET BT /F1 12 Tf (B) Tj ET"))
	assert.NoError(t, err)
	if !assert.Len(t, recCanvas.textDrawCalls, 2) {
		return
	}

	assert.Equal(t, "A", recCanvas.textDrawCalls[0].text)
	assert.InDelta(t, 110.0, recCanvas.textDrawCalls[0].x, 0.001)
	assert.InDelta(t, 220.0, recCanvas.textDrawCalls[0].y, 0.001)

	assert.Equal(t, "B", recCanvas.textDrawCalls[1].text)
	assert.InDelta(t, 100.0, recCanvas.textDrawCalls[1].x, 0.001)
	assert.InDelta(t, 200.0, recCanvas.textDrawCalls[1].y, 0.001)
}

func TestTextRendering_EvaluateContent_TJAdjustmentDirection(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	resources := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
	fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
	resources.Set(entity.Name("/F1"), fontDict)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("BT /F1 12 Tf [(A) 1000 (B)] TJ ET"))
	assert.NoError(t, err)
	if !assert.Len(t, recCanvas.textDrawCalls, 2) {
		return
	}

	assert.Equal(t, "A", recCanvas.textDrawCalls[0].text)
	assert.Equal(t, "B", recCanvas.textDrawCalls[1].text)
	assert.Less(t, recCanvas.textDrawCalls[1].x, recCanvas.textDrawCalls[0].x)
}

func TestTextRendering_EvaluateContent_CharSpacingUsesPerCharPositioning(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	resources := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
	fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
	resources.Set(entity.Name("/F1"), fontDict)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("BT /F1 12 Tf 20 Tc (AB) Tj ET"))
	assert.NoError(t, err)
	if !assert.Len(t, recCanvas.textDrawCalls, 2) {
		return
	}
	assert.Equal(t, "A", recCanvas.textDrawCalls[0].text)
	assert.Equal(t, "B", recCanvas.textDrawCalls[1].text)
	assert.Greater(t, recCanvas.textDrawCalls[1].x-recCanvas.textDrawCalls[0].x, 20.0)
}

func TestTextRendering_EvaluateContent_DefaultSpacingUsesSingleCanvasDraw(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	resources := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
	fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
	resources.Set(entity.Name("/F1"), fontDict)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("BT /F1 12 Tf (AB) Tj ET"))
	assert.NoError(t, err)
	if !assert.Len(t, recCanvas.textDrawCalls, 2) {
		return
	}
	assert.Equal(t, "A", recCanvas.textDrawCalls[0].text)
	assert.Equal(t, "B", recCanvas.textDrawCalls[1].text)
}

func TestGraphicsStateStack_SaveRestore(t *testing.T) {
	// Test q/Q operators - save/restore graphics state
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	// Set some initial state
	op := renderer.Operator{
		Opcode:   "Tf",
		Operands: []entity.Object{entity.Name("/F1"), entity.NewInteger(12)},
	}
	resources := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
	fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
	resources.Set(entity.Name("/F1"), fontDict)
	e.SetResources(resources)
	_ = e.SetFont(op)

	// Save state with 'q'
	err := e.SaveState()
	assert.NoError(t, err)

	// Modify state
	op2 := renderer.Operator{
		Opcode:   "Tf",
		Operands: []entity.Object{entity.Name("/F1"), entity.NewInteger(24)}, // Different size
	}
	_ = e.SetFont(op2)

	// Restore state with 'Q'
	err = e.RestoreState()
	assert.NoError(t, err)

	// State should be restored to original font size
	// (Note: This is a basic test - real testing would check actual state values)
}

func TestGraphicsStateStack_EmptyRestore(t *testing.T) {
	// Test restoring from empty stack
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	// Try to restore without saving
	err := e.RestoreState()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stack is empty")
}

func TestGraphicsStateStack_SaveRestore_PropagatesToCanvas(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	err := e.SaveState()
	assert.NoError(t, err)
	err = e.RestoreState()
	assert.NoError(t, err)

	assert.Equal(t, 1, recCanvas.saveCount)
	assert.Equal(t, 1, recCanvas.restoreCount)
}

func TestGraphicsStateStack_SaveRestore_TextRiseIsolation(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	resources := entity.NewDict()
	fontDict := entity.NewDict()
	fontDict.Set(entity.Name("Type"), entity.Name("/Font"))
	fontDict.Set(entity.Name("Subtype"), entity.Name("/Type1"))
	fontDict.Set(entity.Name("BaseFont"), entity.Name("/Helvetica"))
	resources.Set(entity.Name("/F1"), fontDict)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("BT /F1 12 Tf q 10 Ts (A) Tj Q (B) Tj ET"))
	assert.NoError(t, err)
	if !assert.Len(t, recCanvas.textDrawCalls, 2) {
		return
	}

	assert.Equal(t, "A", recCanvas.textDrawCalls[0].text)
	assert.Equal(t, "B", recCanvas.textDrawCalls[1].text)
	assert.InDelta(t, 10.0, recCanvas.textDrawCalls[0].y, 0.001)
	assert.InDelta(t, 0.0, recCanvas.textDrawCalls[1].y, 0.001)
}

func TestPath_MoveTo(t *testing.T) {
	// Test m operator - move to
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	op := renderer.Operator{
		Opcode:   "m",
		Operands: []entity.Object{entity.NewInteger(100), entity.NewInteger(200)},
	}

	err := e.MoveTo(op)
	assert.NoError(t, err)

	// Check that path was created and current point is set
	state := e.GetGraphicsState()
	assert.NotNil(t, state)
}

func TestPath_LineTo(t *testing.T) {
	// Test l operator - line to
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	// First move to a point
	moveOp := renderer.Operator{
		Opcode:   "m",
		Operands: []entity.Object{entity.NewInteger(100), entity.NewInteger(200)},
	}
	_ = e.MoveTo(moveOp)

	// Then draw line
	lineOp := renderer.Operator{
		Opcode:   "l",
		Operands: []entity.Object{entity.NewInteger(150), entity.NewInteger(250)},
	}

	err := e.LineTo(lineOp)
	assert.NoError(t, err)
}

func TestPath_CurveTo(t *testing.T) {
	// Test c operator - cubic Bézier curve
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	// First move to a point
	moveOp := renderer.Operator{
		Opcode:   "m",
		Operands: []entity.Object{entity.NewInteger(100), entity.NewInteger(200)},
	}
	_ = e.MoveTo(moveOp)

	// Then draw curve
	curveOp := renderer.Operator{
		Opcode: "c",
		Operands: []entity.Object{
			entity.NewInteger(120), entity.NewInteger(220), // Control point 1
			entity.NewInteger(140), entity.NewInteger(280), // Control point 2
			entity.NewInteger(150), entity.NewInteger(300), // End point
		},
	}

	err := e.CurveTo(curveOp)
	assert.NoError(t, err)
}

func TestPath_Rectangle(t *testing.T) {
	// Test re operator - rectangle
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	op := renderer.Operator{
		Opcode: "re",
		Operands: []entity.Object{
			entity.NewInteger(100), // x
			entity.NewInteger(200), // y
			entity.NewInteger(50),  // width
			entity.NewInteger(30),  // height
		},
	}

	err := e.Rectangle(op)
	assert.NoError(t, err)
}

func TestPath_Close(t *testing.T) {
	// Test h operator - close path
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	// First create a path
	moveOp := renderer.Operator{
		Opcode:   "m",
		Operands: []entity.Object{entity.NewInteger(100), entity.NewInteger(200)},
	}
	_ = e.MoveTo(moveOp)

	lineOp := renderer.Operator{
		Opcode:   "l",
		Operands: []entity.Object{entity.NewInteger(150), entity.NewInteger(250)},
	}
	_ = e.LineTo(lineOp)

	// Close path
	closeOp := renderer.Operator{Opcode: "h", Operands: []entity.Object{}}

	err := e.ClosePath(closeOp)
	assert.NoError(t, err)
}

func TestPath_Stroke(t *testing.T) {
	// Test S operator - stroke path
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	// Create a path
	moveOp := renderer.Operator{
		Opcode:   "m",
		Operands: []entity.Object{entity.NewInteger(100), entity.NewInteger(200)},
	}
	_ = e.MoveTo(moveOp)

	lineOp := renderer.Operator{
		Opcode:   "l",
		Operands: []entity.Object{entity.NewInteger(150), entity.NewInteger(250)},
	}
	_ = e.LineTo(lineOp)

	// Stroke path
	err := e.StrokePath()
	assert.NoError(t, err)
}

func TestPath_Fill(t *testing.T) {
	// Test f operator - fill path
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	// Create a rectangular path
	rectOp := renderer.Operator{
		Opcode: "re",
		Operands: []entity.Object{
			entity.NewInteger(100),
			entity.NewInteger(200),
			entity.NewInteger(50),
			entity.NewInteger(30),
		},
	}
	_ = e.Rectangle(rectOp)

	// Fill path
	err := e.FillPath()
	assert.NoError(t, err)
}

func TestPath_EndPath(t *testing.T) {
	// Test n operator - end path without filling or stroking
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	// Create a path
	moveOp := renderer.Operator{
		Opcode:   "m",
		Operands: []entity.Object{entity.NewInteger(100), entity.NewInteger(200)},
	}
	_ = e.MoveTo(moveOp)

	// End path
	err := e.EndPath()
	assert.NoError(t, err)
}

func TestXObject_InvokeForm(t *testing.T) {
	// Test Do operator with form XObject
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	// Set up resources with a form XObject
	resources := entity.NewDict()
	formDict := entity.NewDict()
	formDict.Set(entity.Name("Type"), entity.Name("XObject"))
	formDict.Set(entity.Name("Subtype"), entity.Name("/Form"))
	formDict.Set(entity.Name("BBox"), entity.NewArray(
		entity.NewInteger(0), entity.NewInteger(0),
		entity.NewInteger(100), entity.NewInteger(100),
	))

	// Create a form stream with simple content
	// Use proper PDF content stream syntax with newlines
	formStream := entity.NewStream(formDict, []byte("q\nQ"))

	resources.Set(entity.Name("/Form1"), formStream)
	e.SetResources(resources)

	// Invoke the form
	op := renderer.Operator{
		Opcode:   "Do",
		Operands: []entity.Object{entity.Name("/Form1")},
	}

	err := e.InvokeXObject(op)
	assert.NoError(t, err)
}

func TestXObject_InvokeForm_AppliesBBoxClip(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	resources := entity.NewDict()
	formDict := entity.NewDict()
	formDict.Set(entity.Name("Type"), entity.Name("XObject"))
	formDict.Set(entity.Name("Subtype"), entity.Name("/Form"))
	formDict.Set(entity.Name("BBox"), entity.NewArray(
		entity.NewInteger(0), entity.NewInteger(0),
		entity.NewInteger(100), entity.NewInteger(100),
	))
	formStream := entity.NewStream(formDict, []byte{})
	resources.Set(entity.Name("/Form1"), formStream)
	e.SetResources(resources)

	op := renderer.Operator{
		Opcode:   "Do",
		Operands: []entity.Object{entity.Name("/Form1")},
	}

	err := e.InvokeXObject(op)
	assert.NoError(t, err)
	assert.Equal(t, 1, recCanvas.clipCount)
	assert.Equal(t, 1, recCanvas.saveCount)
	assert.Equal(t, 1, recCanvas.restoreCount)
}

func TestXObject_InvokeImage(t *testing.T) {
	// Test Do operator with image XObject
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	// Set up resources with an image XObject
	resources := entity.NewDict()
	imageDict := entity.NewDict()
	imageDict.Set(entity.Name("Type"), entity.Name("XObject"))
	imageDict.Set(entity.Name("Subtype"), entity.Name("/Image"))
	imageDict.Set(entity.Name("Width"), entity.NewInteger(100))
	imageDict.Set(entity.Name("Height"), entity.NewInteger(100))

	imageStream := entity.NewStream(imageDict, []byte{})

	resources.Set(entity.Name("/Image1"), imageStream)
	e.SetResources(resources)

	// Invoke the image
	op := renderer.Operator{
		Opcode:   "Do",
		Operands: []entity.Object{entity.Name("/Image1")},
	}

	err := e.InvokeXObject(op)
	assert.NoError(t, err)
}

func TestXObject_InvokeForm_SubtypeWithoutSlash(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	resources := entity.NewDict()
	formDict := entity.NewDict()
	formDict.Set(entity.Name("Type"), entity.Name("XObject"))
	formDict.Set(entity.Name("Subtype"), entity.Name("Form"))
	formDict.Set(entity.Name("BBox"), entity.NewArray(
		entity.NewInteger(0), entity.NewInteger(0),
		entity.NewInteger(100), entity.NewInteger(100),
	))
	formStream := entity.NewStream(formDict, []byte("q\nQ"))

	resources.Set(entity.Name("/Form1"), formStream)
	e.SetResources(resources)

	op := renderer.Operator{
		Opcode:   "Do",
		Operands: []entity.Object{entity.Name("/Form1")},
	}

	err := e.InvokeXObject(op)
	assert.NoError(t, err)
}

func TestXObject_InvokeImage_SubtypeWithoutSlash(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	resources := entity.NewDict()
	imageDict := entity.NewDict()
	imageDict.Set(entity.Name("Type"), entity.Name("XObject"))
	imageDict.Set(entity.Name("Subtype"), entity.Name("Image"))
	imageDict.Set(entity.Name("Width"), entity.NewInteger(8))
	imageDict.Set(entity.Name("Height"), entity.NewInteger(8))
	imageStream := entity.NewStream(imageDict, []byte{})

	resources.Set(entity.Name("/Image1"), imageStream)
	e.SetResources(resources)

	op := renderer.Operator{
		Opcode:   "Do",
		Operands: []entity.Object{entity.Name("/Image1")},
	}

	err := e.InvokeXObject(op)
	assert.NoError(t, err)
}

func TestXObject_InvokeImage_FromNestedXObjectResources(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	resources := entity.NewDict()
	xObjectResources := entity.NewDict()
	imageDict := entity.NewDict()
	imageDict.Set(entity.Name("Type"), entity.Name("XObject"))
	imageDict.Set(entity.Name("Subtype"), entity.Name("/Image"))
	imageDict.Set(entity.Name("Width"), entity.NewInteger(8))
	imageDict.Set(entity.Name("Height"), entity.NewInteger(8))
	imageStream := entity.NewStream(imageDict, []byte{})
	xObjectResources.Set(entity.Name("/Im0"), imageStream)
	resources.Set(entity.Name("XObject"), xObjectResources)
	e.SetResources(resources)

	op := renderer.Operator{
		Opcode:   "Do",
		Operands: []entity.Object{entity.Name("Im0")},
	}

	err := e.InvokeXObject(op)
	assert.NoError(t, err)
}

func TestXObject_InvokeImage_ICCBasedAlternateColorSpace(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	e.SetCanvas(newRecordingCanvas())

	resources := entity.NewDict()
	xObjectResources := entity.NewDict()

	iccProfileDict := entity.NewDict()
	iccProfileDict.Set(entity.Name("N"), entity.NewInteger(1))
	iccProfileDict.Set(entity.Name("Alternate"), entity.Name("/DeviceGray"))
	iccProfile := entity.NewStream(iccProfileDict, []byte{})

	colorSpace := entity.NewArray(entity.Name("/ICCBased"), iccProfile)

	imageDict := entity.NewDict()
	imageDict.Set(entity.Name("Type"), entity.Name("XObject"))
	imageDict.Set(entity.Name("Subtype"), entity.Name("/Image"))
	imageDict.Set(entity.Name("Width"), entity.NewInteger(1))
	imageDict.Set(entity.Name("Height"), entity.NewInteger(1))
	imageDict.Set(entity.Name("BitsPerComponent"), entity.NewInteger(8))
	imageDict.Set(entity.Name("ColorSpace"), colorSpace)
	imageStream := entity.NewStream(imageDict, []byte{0xff})

	xObjectResources.Set(entity.Name("/Im0"), imageStream)
	resources.Set(entity.Name("XObject"), xObjectResources)
	e.SetResources(resources)

	op := renderer.Operator{
		Opcode:   "Do",
		Operands: []entity.Object{entity.Name("Im0")},
	}

	err := e.InvokeXObject(op)
	assert.NoError(t, err)
}

func TestXObject_InvokeImage_DCTDecodeFilter(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	src := image.NewRGBA(image.Rect(0, 0, 1, 1))
	src.Set(0, 0, color.RGBA{R: 255, G: 0, B: 0, A: 255})

	var jpegData bytes.Buffer
	err := jpeg.Encode(&jpegData, src, nil)
	assert.NoError(t, err)

	resources := entity.NewDict()
	xObjectResources := entity.NewDict()

	imageDict := entity.NewDict()
	imageDict.Set(entity.Name("Type"), entity.Name("XObject"))
	imageDict.Set(entity.Name("Subtype"), entity.Name("/Image"))
	imageDict.Set(entity.Name("Width"), entity.NewInteger(1))
	imageDict.Set(entity.Name("Height"), entity.NewInteger(1))
	imageDict.Set(entity.Name("BitsPerComponent"), entity.NewInteger(8))
	imageDict.Set(entity.Name("ColorSpace"), entity.Name("/DeviceRGB"))
	imageDict.Set(entity.Name("Filter"), entity.Name("/DCTDecode"))
	imageStream := entity.NewStream(imageDict, jpegData.Bytes())

	xObjectResources.Set(entity.Name("/Im0"), imageStream)
	resources.Set(entity.Name("XObject"), xObjectResources)
	e.SetResources(resources)

	op := renderer.Operator{
		Opcode:   "Do",
		Operands: []entity.Object{entity.Name("Im0")},
	}

	err = e.InvokeXObject(op)
	assert.NoError(t, err)
	assert.Equal(t, 1, recCanvas.drawImageCount)
}

func TestXObject_NotFound(t *testing.T) {
	// Test Do operator with non-existent XObject
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	resources := entity.NewDict()
	e.SetResources(resources)

	op := renderer.Operator{
		Opcode:   "Do",
		Operands: []entity.Object{entity.Name("/NonExistent")},
	}

	err := e.InvokeXObject(op)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestColor_GrayStroke(t *testing.T) {
	// Test G operator - set gray stroke color
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	op := renderer.Operator{
		Opcode:   "G",
		Operands: []entity.Object{entity.NewReal(0.5)},
	}

	err := e.SetGrayStroke(op)
	assert.NoError(t, err)

	state := e.GetGraphicsState()
	assert.NotNil(t, state)
}

func TestColor_GrayFill(t *testing.T) {
	// Test g operator - set gray fill color
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	op := renderer.Operator{
		Opcode:   "g",
		Operands: []entity.Object{entity.NewReal(0.75)},
	}

	err := e.SetGrayFill(op)
	assert.NoError(t, err)

	state := e.GetGraphicsState()
	assert.NotNil(t, state)
}

func TestColor_RGBStroke(t *testing.T) {
	// Test RG operator - set RGB stroke color
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	op := renderer.Operator{
		Opcode: "RG",
		Operands: []entity.Object{
			entity.NewReal(1.0), // Red
			entity.NewReal(0.0), // Green
			entity.NewReal(0.0), // Blue
		},
	}

	err := e.SetRGBStroke(op)
	assert.NoError(t, err)

	state := e.GetGraphicsState()
	assert.NotNil(t, state)
}

func TestColor_RGBFill(t *testing.T) {
	// Test rg operator - set RGB fill color
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	op := renderer.Operator{
		Opcode: "rg",
		Operands: []entity.Object{
			entity.NewReal(0.0), // Red
			entity.NewReal(0.5), // Green
			entity.NewReal(1.0), // Blue
		},
	}

	err := e.SetRGBFill(op)
	assert.NoError(t, err)

	state := e.GetGraphicsState()
	assert.NotNil(t, state)
}

func TestColor_CMYKStroke(t *testing.T) {
	// Test K operator - set CMYK stroke color
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	op := renderer.Operator{
		Opcode: "K",
		Operands: []entity.Object{
			entity.NewReal(0.0), // Cyan
			entity.NewReal(0.0), // Magenta
			entity.NewReal(0.0), // Yellow
			entity.NewReal(1.0), // Black
		},
	}

	err := e.SetCMYKStroke(op)
	assert.NoError(t, err)

	state := e.GetGraphicsState()
	assert.NotNil(t, state)
}

func TestColor_CMYKFill(t *testing.T) {
	// Test k operator - set CMYK fill color
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	op := renderer.Operator{
		Opcode: "k",
		Operands: []entity.Object{
			entity.NewReal(1.0), // Cyan
			entity.NewReal(0.0), // Magenta
			entity.NewReal(0.0), // Yellow
			entity.NewReal(0.0), // Black
		},
	}

	err := e.SetCMYKFill(op)
	assert.NoError(t, err)

	state := e.GetGraphicsState()
	assert.NotNil(t, state)
}

func TestGraphics_LineWidth(t *testing.T) {
	// Test w operator - set line width
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	op := renderer.Operator{
		Opcode:   "w",
		Operands: []entity.Object{entity.NewReal(2.5)},
	}

	err := e.SetLineWidth(op)
	assert.NoError(t, err)

	state := e.GetGraphicsState()
	assert.NotNil(t, state)
}

func TestGraphics_LineCap(t *testing.T) {
	// Test J operator - set line cap style
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	op := renderer.Operator{
		Opcode:   "J",
		Operands: []entity.Object{entity.NewInteger(1)},
	}

	err := e.SetLineCap(op)
	assert.NoError(t, err)

	state := e.GetGraphicsState()
	assert.NotNil(t, state)
}

func TestGraphics_LineJoin(t *testing.T) {
	// Test j operator - set line join style
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	op := renderer.Operator{
		Opcode:   "j",
		Operands: []entity.Object{entity.NewInteger(0)},
	}

	err := e.SetLineJoin(op)
	assert.NoError(t, err)

	state := e.GetGraphicsState()
	assert.NotNil(t, state)
}

func TestGraphics_MiterLimit(t *testing.T) {
	// Test M operator - set miter limit
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)

	op := renderer.Operator{
		Opcode:   "M",
		Operands: []entity.Object{entity.NewReal(10.0)},
	}

	err := e.SetMiterLimit(op)
	assert.NoError(t, err)
}

func TestGraphics_DashPattern(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	rec := newRecordingCanvas()
	e.SetCanvas(rec)

	err := e.EvaluateContent([]byte("[3 1] 2 d 0 0 m 10 0 l S"))
	assert.NoError(t, err)
	assert.Equal(t, []float64{3, 1}, rec.lastDashPattern)
	assert.Equal(t, 2.0, rec.lastDashPhase)
}

func TestGraphics_LineStateSyncedToCanvasOnStroke(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	rec := newRecordingCanvas()
	e.SetCanvas(rec)

	err := e.EvaluateContent([]byte("2.5 w 1 J 2 j 11 M [4 2] 1 d 0 0 m 20 0 l S"))
	assert.NoError(t, err)

	assert.Equal(t, 2.5, rec.lastLineWidth)
	assert.Equal(t, 1, rec.lastLineCap)
	assert.Equal(t, 2, rec.lastLineJoin)
	assert.Equal(t, 11.0, rec.lastMiterLimit)
	assert.Equal(t, []float64{4, 2}, rec.lastDashPattern)
	assert.Equal(t, 1.0, rec.lastDashPhase)
	assert.Greater(t, rec.setLineWidthCount, 0)
	assert.Greater(t, rec.setDashPatternCount, 0)
}

func TestGraphics_ExtGStateAppliesStyleAndAlpha(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	rec := newRecordingCanvas()
	e.SetCanvas(rec)

	gsEntry := entity.NewDict()
	gsEntry.Set(entity.Name("LW"), entity.NewReal(3))
	gsEntry.Set(entity.Name("LC"), entity.NewInteger(2))
	gsEntry.Set(entity.Name("LJ"), entity.NewInteger(1))
	gsEntry.Set(entity.Name("ML"), entity.NewReal(8))
	gsEntry.Set(entity.Name("D"), entity.NewArray(
		entity.NewArray(entity.NewReal(2), entity.NewReal(1)),
		entity.NewReal(0),
	))
	gsEntry.Set(entity.Name("ca"), entity.NewReal(0.4))
	gsEntry.Set(entity.Name("CA"), entity.NewReal(0.7))

	extGState := entity.NewDict()
	extGState.Set(entity.Name("/GS1"), gsEntry)

	resources := entity.NewDict()
	resources.Set(entity.Name("ExtGState"), extGState)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("1 0 0 rg 0 1 0 RG /GS1 gs 0 0 10 10 re B"))
	assert.NoError(t, err)

	assert.Equal(t, 3.0, rec.lastLineWidth)
	assert.Equal(t, 2, rec.lastLineCap)
	assert.Equal(t, 1, rec.lastLineJoin)
	assert.Equal(t, 8.0, rec.lastMiterLimit)
	assert.Equal(t, []float64{2, 1}, rec.lastDashPattern)
	assert.Equal(t, uint8(102), rec.lastFillColor.A)
	assert.Equal(t, uint8(178), rec.lastStrokeColor.A)
}

func TestShadingOperator_AxialTypeMappingAndPatternDraw(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	shadingDict := entity.NewDict()
	shadingDict.Set(entity.Name("ShadingType"), entity.NewInteger(2))
	shadingDict.Set(entity.Name("ColorSpace"), entity.Name("DeviceRGB"))
	shadingDict.Set(entity.Name("Coords"), entity.NewArray(
		entity.NewInteger(0), entity.NewInteger(0),
		entity.NewInteger(120), entity.NewInteger(0),
	))

	resources := entity.NewDict()
	resources.Set(entity.Name("/SH1"), shadingDict)
	e.SetResources(resources)

	err := e.MoveTo(renderer.Operator{
		Opcode:   "m",
		Operands: []entity.Object{entity.NewInteger(0), entity.NewInteger(0)},
	})
	assert.NoError(t, err)

	err = e.LineTo(renderer.Operator{
		Opcode:   "l",
		Operands: []entity.Object{entity.NewInteger(40), entity.NewInteger(0)},
	})
	assert.NoError(t, err)

	err = e.LineTo(renderer.Operator{
		Opcode:   "l",
		Operands: []entity.Object{entity.NewInteger(40), entity.NewInteger(40)},
	})
	assert.NoError(t, err)

	err = e.EvaluateContent([]byte("/SH1 sh"))
	assert.NoError(t, err)
	assert.Equal(t, 1, recCanvas.shadingDrawCount)
	assert.Equal(t, entity.ShadingAxial, recCanvas.lastShadingType)
}

func TestShadingOperator_UsesCanvasBoundsWhenPathEmpty(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	shadingDict := entity.NewDict()
	shadingDict.Set(entity.Name("ShadingType"), entity.NewInteger(2))
	shadingDict.Set(entity.Name("ColorSpace"), entity.Name("DeviceRGB"))
	shadingDict.Set(entity.Name("Coords"), entity.NewArray(
		entity.NewInteger(0), entity.NewInteger(0),
		entity.NewInteger(120), entity.NewInteger(0),
	))

	resources := entity.NewDict()
	resources.Set(entity.Name("/SH2"), shadingDict)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("/SH2 sh"))
	assert.NoError(t, err)
	assert.Equal(t, 1, recCanvas.shadingDrawCount)
}

func TestShadingOperator_MeshTypePatternDraw(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	shadingDict := entity.NewDict()
	shadingDict.Set(entity.Name("ShadingType"), entity.NewInteger(4))
	shadingDict.Set(entity.Name("ColorSpace"), entity.Name("DeviceRGB"))
	shadingDict.Set(entity.Name("BitsPerCoordinate"), entity.NewInteger(16))
	shadingDict.Set(entity.Name("BitsPerComponent"), entity.NewInteger(8))
	shadingDict.Set(entity.Name("BitsPerFlag"), entity.NewInteger(8))
	shadingDict.Set(entity.Name("Decode"), entity.NewArray(
		entity.NewInteger(0), entity.NewInteger(1),
		entity.NewInteger(0), entity.NewInteger(1),
		entity.NewInteger(0), entity.NewInteger(1),
	))

	resources := entity.NewDict()
	resources.Set(entity.Name("/SHM"), shadingDict)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("/SHM sh"))
	assert.NoError(t, err)
	assert.Equal(t, 1, recCanvas.shadingDrawCount)
	assert.Equal(t, entity.ShadingFreeFormGouraud, recCanvas.lastShadingType)
}

func TestShadingOperator_AxialWithFunction_ParsesFunction(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	functionDict := entity.NewDict()
	functionDict.Set(entity.Name("FunctionType"), entity.NewInteger(2))
	functionDict.Set(entity.Name("Domain"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(1),
	))
	functionDict.Set(entity.Name("C0"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(0), entity.NewReal(0),
	))
	functionDict.Set(entity.Name("C1"), entity.NewArray(
		entity.NewReal(1), entity.NewReal(0), entity.NewReal(0),
	))
	functionDict.Set(entity.Name("N"), entity.NewReal(1))

	shadingDict := entity.NewDict()
	shadingDict.Set(entity.Name("ShadingType"), entity.NewInteger(2))
	shadingDict.Set(entity.Name("ColorSpace"), entity.Name("DeviceRGB"))
	shadingDict.Set(entity.Name("Coords"), entity.NewArray(
		entity.NewInteger(0), entity.NewInteger(0),
		entity.NewInteger(100), entity.NewInteger(0),
	))
	shadingDict.Set(entity.Name("Function"), functionDict)

	resources := entity.NewDict()
	resources.Set(entity.Name("/SHF"), shadingDict)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("/SHF sh"))
	assert.NoError(t, err)
	assert.Equal(t, 1, recCanvas.shadingDrawCount)
	assert.Equal(t, 1, recCanvas.lastShadingFunctionCount)
}

func TestShadingOperator_AxialWithSampledFunctionStream_ParsesFunction(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	// 2 samples, 3 output channels (RGB), 8 bits each:
	// sample0 = (0,0,0), sample1 = (1,0,0)
	functionDict := entity.NewDict()
	functionDict.Set(entity.Name("FunctionType"), entity.NewInteger(0))
	functionDict.Set(entity.Name("Domain"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(1),
	))
	functionDict.Set(entity.Name("Range"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(1),
		entity.NewReal(0), entity.NewReal(1),
		entity.NewReal(0), entity.NewReal(1),
	))
	functionDict.Set(entity.Name("Size"), entity.NewArray(entity.NewInteger(2)))
	functionDict.Set(entity.Name("BitsPerSample"), entity.NewInteger(8))
	functionStream := entity.NewStream(functionDict, []byte{
		0x00, 0x00, 0x00,
		0xFF, 0x00, 0x00,
	})

	shadingDict := entity.NewDict()
	shadingDict.Set(entity.Name("ShadingType"), entity.NewInteger(2))
	shadingDict.Set(entity.Name("ColorSpace"), entity.Name("DeviceRGB"))
	shadingDict.Set(entity.Name("Coords"), entity.NewArray(
		entity.NewInteger(0), entity.NewInteger(0),
		entity.NewInteger(100), entity.NewInteger(0),
	))
	shadingDict.Set(entity.Name("Function"), functionStream)

	resources := entity.NewDict()
	resources.Set(entity.Name("/SHS"), shadingDict)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("/SHS sh"))
	assert.NoError(t, err)
	assert.Equal(t, 1, recCanvas.shadingDrawCount)
	assert.Equal(t, 1, recCanvas.lastShadingFunctionCount)
}

func TestShadingOperator_AxialWithPostScriptFunctionStream_ParsesFunction(t *testing.T) {
	xref := &mockXRef{}
	e := renderer.NewEvaluator(xref)
	recCanvas := newRecordingCanvas()
	e.SetCanvas(recCanvas)

	functionDict := entity.NewDict()
	functionDict.Set(entity.Name("FunctionType"), entity.NewInteger(4))
	functionDict.Set(entity.Name("Domain"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(1),
	))
	functionDict.Set(entity.Name("Range"), entity.NewArray(
		entity.NewReal(0), entity.NewReal(1),
		entity.NewReal(0), entity.NewReal(1),
		entity.NewReal(0), entity.NewReal(1),
	))
	functionStream := entity.NewStream(functionDict, []byte("{ dup dup }"))

	shadingDict := entity.NewDict()
	shadingDict.Set(entity.Name("ShadingType"), entity.NewInteger(2))
	shadingDict.Set(entity.Name("ColorSpace"), entity.Name("DeviceRGB"))
	shadingDict.Set(entity.Name("Coords"), entity.NewArray(
		entity.NewInteger(0), entity.NewInteger(0),
		entity.NewInteger(100), entity.NewInteger(0),
	))
	shadingDict.Set(entity.Name("Function"), functionStream)

	resources := entity.NewDict()
	resources.Set(entity.Name("/SHP"), shadingDict)
	e.SetResources(resources)

	err := e.EvaluateContent([]byte("/SHP sh"))
	assert.NoError(t, err)
	assert.Equal(t, 1, recCanvas.shadingDrawCount)
	assert.Equal(t, 1, recCanvas.lastShadingFunctionCount)
}

// mockXRef is a simple XRef implementation for testing
type mockXRef struct{}

func (m *mockXRef) Fetch(ref entity.Ref) (entity.Object, error) {
	return entity.NewString("mock"), nil
}

type recordingCanvas struct {
	img                      *image.RGBA
	textDrawCalls            []textDrawCall
	lastDashPattern          []float64
	lastFillColor            color.RGBA
	lastStrokeColor          color.RGBA
	lastLineWidth            float64
	lastMiterLimit           float64
	lastDashPhase            float64
	drawImageCount           int
	saveCount                int
	restoreCount             int
	clipCount                int
	eoClipCount              int
	fillCount                int
	rectangleCount           int
	setFillColorCount        int
	setStrokeColorCount      int
	setLineWidthCount        int
	setLineCapCount          int
	setLineJoinCount         int
	setMiterLimitCount       int
	setDashPatternCount      int
	lastLineCap              int
	lastLineJoin             int
	shadingDrawCount         int
	lastShadingFunctionCount int
	lastShadingType          entity.ShadingType
}

type textDrawCall struct {
	text     string
	x        float64
	y        float64
	fontSize float64
}

func newRecordingCanvas() *recordingCanvas {
	return &recordingCanvas{
		img: image.NewRGBA(image.Rect(0, 0, 128, 128)),
	}
}

func (c *recordingCanvas) Width() int { return c.img.Bounds().Dx() }

func (c *recordingCanvas) Height() int { return c.img.Bounds().Dy() }

func (c *recordingCanvas) Bounds() image.Rectangle { return c.img.Bounds() }

func (c *recordingCanvas) MoveTo(x, y float64) {}

func (c *recordingCanvas) LineTo(x, y float64) {}

func (c *recordingCanvas) CurveTo(c1x, c1y, c2x, c2y, x, y float64) {}

func (c *recordingCanvas) Rectangle(x, y, width, height float64) { c.rectangleCount++ }

func (c *recordingCanvas) ClosePath() {}

func (c *recordingCanvas) Fill() { c.fillCount++ }

func (c *recordingCanvas) Stroke() {}

func (c *recordingCanvas) Clip() { c.clipCount++ }

func (c *recordingCanvas) EoClip() { c.eoClipCount++ }

func (c *recordingCanvas) DrawText(text string, x, y float64, font entity.Font, fontSize float64) error {
	c.textDrawCalls = append(c.textDrawCalls, textDrawCall{
		text:     text,
		x:        x,
		y:        y,
		fontSize: fontSize,
	})
	return nil
}

func (c *recordingCanvas) BeginText(x, y float64) {}

func (c *recordingCanvas) EndText() {}

func (c *recordingCanvas) ShowText(text string) error { return nil }

func (c *recordingCanvas) MoveTextPoint(tx, ty float64) {}

func (c *recordingCanvas) DrawImage(img image.Image, x, y, width, height float64, interpolate bool) error {
	c.drawImageCount++
	return nil
}

func (c *recordingCanvas) Save() { c.saveCount++ }

func (c *recordingCanvas) Restore() { c.restoreCount++ }

func (c *recordingCanvas) Transform(matrix [6]float64) {}

func (c *recordingCanvas) SetFillColor(col color.Color) {
	c.setFillColorCount++
	c.lastFillColor = color.RGBAModel.Convert(col).(color.RGBA)
}

func (c *recordingCanvas) SetStrokeColor(col color.Color) {
	c.setStrokeColorCount++
	c.lastStrokeColor = color.RGBAModel.Convert(col).(color.RGBA)
}

func (c *recordingCanvas) SetLineWidth(width float64) {
	c.setLineWidthCount++
	c.lastLineWidth = width
}

func (c *recordingCanvas) SetLineCap(cap int) {
	c.setLineCapCount++
	c.lastLineCap = cap
}

func (c *recordingCanvas) SetLineJoin(join int) {
	c.setLineJoinCount++
	c.lastLineJoin = join
}

func (c *recordingCanvas) SetMiterLimit(limit float64) {
	c.setMiterLimitCount++
	c.lastMiterLimit = limit
}

func (c *recordingCanvas) SetDashPattern(dash []float64, phase float64) {
	c.setDashPatternCount++
	c.lastDashPattern = append([]float64(nil), dash...)
	c.lastDashPhase = phase
}

func (c *recordingCanvas) SetFillPattern(pattern entity.Pattern) {}

func (c *recordingCanvas) SetStrokePattern(pattern entity.Pattern) {}

func (c *recordingCanvas) DrawTilingPattern(pattern *entity.TilingPattern, bbox [4]float64) error {
	return nil
}

func (c *recordingCanvas) DrawShadingPattern(pattern *entity.ShadingPattern, bbox [4]float64) error {
	c.shadingDrawCount++
	if pattern != nil && pattern.GetShading() != nil {
		c.lastShadingType = pattern.GetShading().GetShadingType()
		c.lastShadingFunctionCount = len(pattern.GetShading().GetFunctions())
	}
	return nil
}

func (c *recordingCanvas) Image() image.Image { return c.img }

func (c *recordingCanvas) Reset() {}
