// Package content provides content stream operators for PDF rendering.
//
//revive:disable:exported
package content

import (
	"image/color"

	domaincolorspace "github.com/dh-kam/pdf-go/internal/domain/colorspace"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
	"github.com/dh-kam/pdf-go/internal/domain/graphics"
)

// SaveOperator saves the current graphics state (q).
type SaveOperator struct{}

// Execute executes the operation.
func (op *SaveOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_q", nil)
	}
	// The caller will handle the state save
	return nil
}

// RestoreOperator restores the previous graphics state (Q).
type RestoreOperator struct{}

// Execute executes the operation.
func (op *RestoreOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_Q", nil)
	}
	// The caller will handle the state restore
	return nil
}

// ConcatMatrixOperator concatenates the current matrix with the specified matrix (cm).
type ConcatMatrixOperator struct{}

// Execute executes the operation.
func (op *ConcatMatrixOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 6 {
		return errors.Invalid("operator_cm", nil)
	}

	m := [6]float64{operands[0], operands[1], operands[2], operands[3], operands[4], operands[5]}
	state.Transform(m)

	return nil
}

// MoveToOperator moves to the start of a new subpath (m).
type MoveToOperator struct{}

// Execute executes the operation.
func (op *MoveToOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 2 {
		return errors.Invalid("operator_m", nil)
	}

	path := state.GetCurrentPath()
	path.AddCommand(&graphics.MoveTo{X: operands[0], Y: operands[1]})

	return nil
}

// LineToOperator draws a line from the current point to the specified point (l).
type LineToOperator struct{}

// Execute executes the operation.
func (op *LineToOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 2 {
		return errors.Invalid("operator_l", nil)
	}

	path := state.GetCurrentPath()
	path.AddCommand(&graphics.LineTo{X: operands[0], Y: operands[1]})

	return nil
}

// CurveToOperator draws a cubic Bézier curve (c).
type CurveToOperator struct{}

// Execute executes the operation.
func (op *CurveToOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 6 {
		return errors.Invalid("operator_c", nil)
	}

	path := state.GetCurrentPath()
	path.AddCommand(&graphics.CurveTo{
		X1: operands[0], Y1: operands[1],
		X2: operands[2], Y2: operands[3],
		X3: operands[4], Y3: operands[5],
	})

	return nil
}

// CurveToNoFirstControlOperator draws a curve using the current point as the first control point (v).
type CurveToNoFirstControlOperator struct{}

// Execute executes the operation.
func (op *CurveToNoFirstControlOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 4 {
		return errors.Invalid("operator_v", nil)
	}

	path := state.GetCurrentPath()
	x1, y1 := currentPoint(path)

	path.AddCommand(&graphics.CurveTo{
		X1: x1, Y1: y1,
		X2: operands[0], Y2: operands[1],
		X3: operands[2], Y3: operands[3],
	})

	return nil
}

// CurveToNoLastControlOperator draws a curve using the specified point as the last control point (y).
type CurveToNoLastControlOperator struct{}

// Execute executes the operation.
func (op *CurveToNoLastControlOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 4 {
		return errors.Invalid("operator_y", nil)
	}

	x3, y3 := operands[2], operands[3]

	path := state.GetCurrentPath()
	path.AddCommand(&graphics.CurveTo{
		X1: operands[0], Y1: operands[1],
		X2: x3, Y2: y3,
		X3: x3, Y3: y3,
	})

	return nil
}

func currentPoint(path *graphics.Path) (float64, float64) {
	if path == nil {
		return 0, 0
	}

	commands := path.GetCommands()
	if len(commands) == 0 {
		return 0, 0
	}

	var currentX, currentY float64
	var subpathStartX, subpathStartY float64

	for _, command := range commands {
		switch cmd := command.(type) {
		case *graphics.MoveTo:
			currentX, currentY = cmd.X, cmd.Y
			subpathStartX, subpathStartY = currentX, currentY
		case *graphics.LineTo:
			currentX, currentY = cmd.X, cmd.Y
		case *graphics.CurveTo:
			currentX, currentY = cmd.X3, cmd.Y3
		case *graphics.ClosePath:
			currentX, currentY = subpathStartX, subpathStartY
		}
	}

	return currentX, currentY
}

// ClosePathOperator closes the current subpath (h).
type ClosePathOperator struct{}

// Execute executes the operation.
func (op *ClosePathOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_h", nil)
	}

	path := state.GetCurrentPath()
	path.AddCommand(&graphics.ClosePath{})

	return nil
}

// RectangleOperator appends a rectangle to the current path (re).
type RectangleOperator struct{}

// Execute executes the operation.
func (op *RectangleOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 4 {
		return errors.Invalid("operator_re", nil)
	}

	x := operands[0]
	y := operands[1]
	width := operands[2]
	height := operands[3]

	path := state.GetCurrentPath()
	path.AddCommand(&graphics.MoveTo{X: x, Y: y})
	path.AddCommand(&graphics.LineTo{X: x + width, Y: y})
	path.AddCommand(&graphics.LineTo{X: x + width, Y: y + height})
	path.AddCommand(&graphics.LineTo{X: x, Y: y + height})
	path.AddCommand(&graphics.ClosePath{})

	return nil
}

// EndPathOperator ends the current path without filling or stroking (n).
type EndPathOperator struct{}

// Execute executes the operation.
func (op *EndPathOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_n", nil)
	}

	// Reset the current path
	state.SetCurrentPath(graphics.NewPath())

	return nil
}

// StrokeOperator strokes the current path (S).
type StrokeOperator struct{}

// Execute executes the operation.
func (op *StrokeOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_S", nil)
	}

	// Stroke the current path
	// In a real implementation, this would render the path
	state.SetCurrentPath(graphics.NewPath())

	return nil
}

// CloseAndStrokeOperator closes and strokes the path (s).
type CloseAndStrokeOperator struct{}

// Execute executes the operation.
func (op *CloseAndStrokeOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_s", nil)
	}

	path := state.GetCurrentPath()
	path.AddCommand(&graphics.ClosePath{})

	// Stroke the path
	state.SetCurrentPath(graphics.NewPath())

	return nil
}

// FillOperator fills the current path using the nonzero winding rule (f).
type FillOperator struct{}

// Execute executes the operation.
func (op *FillOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_f", nil)
	}

	// Fill the current path
	state.SetCurrentPath(graphics.NewPath())

	return nil
}

// EOFillOperator fills the current path using the even-odd rule (f*).
type EOFillOperator struct{}

// Execute executes the operation.
func (op *EOFillOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_f*", nil)
	}

	path := state.GetCurrentPath()
	path.SetFillRule(graphics.FillRuleEvenOdd)

	// Fill the path
	state.SetCurrentPath(graphics.NewPath())

	return nil
}

// FillAndStrokeOperator fills and strokes the path using nonzero winding rule (B).
type FillAndStrokeOperator struct{}

// Execute executes the operation.
func (op *FillAndStrokeOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_B", nil)
	}

	// Fill and stroke the path
	state.SetCurrentPath(graphics.NewPath())

	return nil
}

// EOFillAndStrokeOperator fills and strokes the path using even-odd rule (B*).
type EOFillAndStrokeOperator struct{}

// Execute executes the operation.
func (op *EOFillAndStrokeOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_B*", nil)
	}

	path := state.GetCurrentPath()
	path.SetFillRule(graphics.FillRuleEvenOdd)

	// Fill and stroke the path
	state.SetCurrentPath(graphics.NewPath())

	return nil
}

// CloseFillAndStrokeOperator closes, fills, and strokes the path (b).
type CloseFillAndStrokeOperator struct{}

// Execute executes the operation.
func (op *CloseFillAndStrokeOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_b", nil)
	}

	path := state.GetCurrentPath()
	path.AddCommand(&graphics.ClosePath{})

	// Fill and stroke
	state.SetCurrentPath(graphics.NewPath())

	return nil
}

// EOCloseFillAndStrokeOperator closes, fills, and strokes the path using even-odd rule (b*).
type EOCloseFillAndStrokeOperator struct{}

// Execute executes the operation.
func (op *EOCloseFillAndStrokeOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_b*", nil)
	}

	path := state.GetCurrentPath()
	path.AddCommand(&graphics.ClosePath{})
	path.SetFillRule(graphics.FillRuleEvenOdd)

	// Fill and stroke
	state.SetCurrentPath(graphics.NewPath())

	return nil
}

// ClipOperator establishes the current path as a clipping path (W).
type ClipOperator struct{}

// Execute executes the operation.
func (op *ClipOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_W", nil)
	}

	// Set clipping path
	path := state.GetCurrentPath()
	state.SetClipPath(path.Clone())

	return nil
}

// EOClipOperator establishes the current path as a clipping path using even-odd rule (W*).
type EOClipOperator struct{}

// Execute executes the operation.
func (op *EOClipOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_W*", nil)
	}

	// Set clipping path with even-odd fill rule
	path := state.GetCurrentPath()
	path.SetFillRule(graphics.FillRuleEvenOdd)
	state.SetClipPath(path.Clone())

	return nil
}

// BeginTextOperator begins a text object (BT).
type BeginTextOperator struct{}

// Execute executes the operation.
func (op *BeginTextOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_BT", nil)
	}

	// Initialize text matrix
	state.SetTextMatrix([6]float64{1, 0, 0, 1, 0, 0})
	state.SetTextLeading(0)

	return nil
}

// EndTextOperator ends a text object (ET).
type EndTextOperator struct{}

// Execute executes the operation.
func (op *EndTextOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_ET", nil)
	}

	// End text mode
	// In a real implementation, this would flush text buffer

	return nil
}

// SetCharSpacingOperator sets character spacing (Tc).
type SetCharSpacingOperator struct{}

// Execute executes the operation.
func (op *SetCharSpacingOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 1 {
		return errors.Invalid("operator_Tc", nil)
	}

	state.SetCharSpacing(operands[0])

	return nil
}

// SetWordSpacingOperator sets word spacing (Tw).
type SetWordSpacingOperator struct{}

// Execute executes the operation.
func (op *SetWordSpacingOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 1 {
		return errors.Invalid("operator_Tw", nil)
	}

	state.SetWordSpacing(operands[0])

	return nil
}

// SetHorizontalScalingOperator sets horizontal text scaling (Tz).
type SetHorizontalScalingOperator struct{}

// Execute executes the operation.
func (op *SetHorizontalScalingOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 1 {
		return errors.Invalid("operator_Tz", nil)
	}

	state.SetHorizontalScaling(operands[0])

	return nil
}

// SetTextLeadingOperator sets text leading (TL).
type SetTextLeadingOperator struct{}

// Execute executes the operation.
func (op *SetTextLeadingOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 1 {
		return errors.Invalid("operator_TL", nil)
	}

	state.SetTextLeading(operands[0])

	return nil
}

// SetFontOperator sets the font and size (Tf).
type SetFontOperator struct{}

// Execute executes the operation.
func (op *SetFontOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 2 {
		return errors.Invalid("operator_Tf", nil)
	}

	// operands[0] is a font name (as an object reference)
	// For now, we just use the size
	state.SetFontSize(operands[1])

	// In a real implementation, we would look up the font by name

	return nil
}

// SetTextRenderModeOperator sets the text rendering mode (Tr).
type SetTextRenderModeOperator struct{}

// Execute executes the operation.
func (op *SetTextRenderModeOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 1 {
		return errors.Invalid("operator_Tr", nil)
	}

	state.SetTextRenderMode(int(operands[0]))

	return nil
}

// SetTextRiseOperator sets text rise (Ts).
type SetTextRiseOperator struct{}

// Execute executes the operation.
func (op *SetTextRiseOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 1 {
		return errors.Invalid("operator_Ts", nil)
	}

	state.SetTextRise(operands[0])

	return nil
}

// MoveTextOperator moves text position (Td).
type MoveTextOperator struct{}

// Execute executes the operation.
func (op *MoveTextOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 2 {
		return errors.Invalid("operator_Td", nil)
	}

	// Td translates the text matrix
	// In a real implementation, this would update the text matrix
	tm := state.GetTextMatrix()
	tm[4] += operands[0]
	tm[5] += operands[1]
	state.SetTextMatrix(tm)

	// Also update text leading if Td is used
	state.SetTextLeading(state.GetTextLeading())

	return nil
}

// MoveTextSetLeadingOperator moves text position and sets leading (TD).
type MoveTextSetLeadingOperator struct{}

// Execute executes the operation.
func (op *MoveTextSetLeadingOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 2 {
		return errors.Invalid("operator_TD", nil)
	}

	// TD sets the leading to -ty and translates by (tx, ty)
	state.SetTextLeading(-operands[1])

	// Translate text matrix
	tm := state.GetTextMatrix()
	tm[4] += operands[0]
	tm[5] += operands[1]
	state.SetTextMatrix(tm)

	return nil
}

// SetTextMatrixOperator sets the text matrix (Tm).
type SetTextMatrixOperator struct{}

// Execute executes the operation.
func (op *SetTextMatrixOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 6 {
		return errors.Invalid("operator_Tm", nil)
	}

	// Tm sets the text matrix directly (6-element affine transformation)
	tm := [6]float64{operands[0], operands[1], operands[2], operands[3], operands[4], operands[5]}
	state.SetTextMatrix(tm)

	return nil
}

// MoveTextNextLineOperator moves to the start of the next text line (T*).
type MoveTextNextLineOperator struct{}

// Execute executes the operation.
func (op *MoveTextNextLineOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_T*", nil)
	}

	tm := state.GetTextMatrix()
	tm[5] += state.GetTextLeading()
	state.SetTextMatrix(tm)

	return nil
}

// ShowTextOperator shows text (Tj).
type ShowTextOperator struct{}

// Execute executes the operation.
func (op *ShowTextOperator) Execute(state *graphics.State, operands []float64) error {
	// Tj takes a string operand, not numbers
	// For now, this is a placeholder
	return nil
}

// ShowTextArrayOperator shows text array with individual positioning (TJ).
type ShowTextArrayOperator struct{}

// Execute executes the operation.
func (op *ShowTextArrayOperator) Execute(state *graphics.State, operands []float64) error {
	// TJ takes an array operand
	// For now, this is a placeholder
	return nil
}

// SetStrokeColorSpaceOperator sets the stroke color space (CS).
type SetStrokeColorSpaceOperator struct{}

// Execute executes the operation.
func (op *SetStrokeColorSpaceOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 1 {
		return errors.Invalid("operator_CS", nil)
	}

	// operands[0] is a name
	// In a real implementation, this would set the color space

	return nil
}

// SetFillColorSpaceOperator sets the fill color space (cs).
type SetFillColorSpaceOperator struct{}

// Execute executes the operation.
func (op *SetFillColorSpaceOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 1 {
		return errors.Invalid("operator_cs", nil)
	}

	// operands[0] is a name
	// In a real implementation, this would set the color space

	return nil
}

// SetStrokeColorOperator sets the stroke color (SC).
type SetStrokeColorOperator struct{}

// Execute executes the operation.
func (op *SetStrokeColorOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) == 0 {
		return errors.Invalid("operator_SC", nil)
	}

	return nil
}

// SetFillColorNOperator sets the fill color (scn).
type SetFillColorNOperator struct{}

// Execute executes the operation.
func (op *SetFillColorNOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) == 0 {
		return errors.Invalid("operator_scn", nil)
	}

	return nil
}

// SetStrokeColorNOperator sets the stroke color (SCN).
type SetStrokeColorNOperator struct{}

// Execute executes the operation.
func (op *SetStrokeColorNOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) == 0 {
		return errors.Invalid("operator_SCN", nil)
	}

	return nil
}

// SetGrayStrokeOperator sets the grayscale stroke color (G).
type SetGrayStrokeOperator struct{}

// Execute executes the operation.
func (op *SetGrayStrokeOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 1 {
		return errors.Invalid("operator_G", nil)
	}

	gray := domaincolorspace.ConvertComponentToByte(operands[0])
	state.SetStrokeColor(color.Gray{Y: gray})

	return nil
}

// SetGrayFillOperator sets the grayscale fill color (g).
type SetGrayFillOperator struct{}

// Execute executes the operation.
func (op *SetGrayFillOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 1 {
		return errors.Invalid("operator_g", nil)
	}

	gray := domaincolorspace.ConvertComponentToByte(operands[0])
	state.SetFillColor(color.Gray{Y: gray})

	return nil
}

// SetRGBStrokeOperator sets the RGB stroke color (RG).
type SetRGBStrokeOperator struct{}

// Execute executes the operation.
func (op *SetRGBStrokeOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 3 {
		return errors.Invalid("operator_RG", nil)
	}

	r := domaincolorspace.ConvertComponentToByte(operands[0])
	g := domaincolorspace.ConvertComponentToByte(operands[1])
	b := domaincolorspace.ConvertComponentToByte(operands[2])
	state.SetStrokeColor(color.RGBA{R: r, G: g, B: b, A: 255})

	return nil
}

// SetRGBFillOperator sets the RGB fill color (rg).
type SetRGBFillOperator struct{}

// Execute executes the operation.
func (op *SetRGBFillOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 3 {
		return errors.Invalid("operator_rg", nil)
	}

	r := domaincolorspace.ConvertComponentToByte(operands[0])
	g := domaincolorspace.ConvertComponentToByte(operands[1])
	b := domaincolorspace.ConvertComponentToByte(operands[2])
	state.SetFillColor(color.RGBA{R: r, G: g, B: b, A: 255})

	return nil
}

// SetCMYKStrokeOperator sets the CMYK stroke color (K).
type SetCMYKStrokeOperator struct{}

// Execute executes the operation.
func (op *SetCMYKStrokeOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 4 {
		return errors.Invalid("operator_K", nil)
	}

	state.SetStrokeColor(domaincolorspace.ConvertDeviceCMYKToRGBA(operands))

	return nil
}

// SetCMYKFillOperator sets the CMYK fill color (k).
type SetCMYKFillOperator struct{}

// Execute executes the operation.
func (op *SetCMYKFillOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 4 {
		return errors.Invalid("operator_k", nil)
	}

	state.SetFillColor(domaincolorspace.ConvertDeviceCMYKToRGBA(operands))

	return nil
}

// ExecuteXObjectOperator executes an XObject (Do).
type ExecuteXObjectOperator struct{}

// Execute executes the operation.
func (op *ExecuteXObjectOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 1 {
		return errors.Invalid("operator_Do", nil)
	}

	// operands[0] is an XObject name
	// In a real implementation, this would look up and execute the XObject

	return nil
}

// BeginCompatibilityOperator begins a compatibility section (BX).
type BeginCompatibilityOperator struct{}

// Execute executes the operation.
func (op *BeginCompatibilityOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_BX", nil)
	}

	// Begin compatibility section
	// In a real implementation, this would save the current graphics state

	return nil
}

// EndCompatibilityOperator ends a compatibility section (EX).
type EndCompatibilityOperator struct{}

// Execute executes the operation.
func (op *EndCompatibilityOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_EX", nil)
	}

	// End compatibility section
	// In a real implementation, this would restore the graphics state

	return nil
}

// ShadingOperator paints a shading pattern (sh).
type ShadingOperator struct{}

// Execute executes the operation.
func (op *ShadingOperator) Execute(state *graphics.State, operands []float64) error {
	if len(operands) != 0 {
		return errors.Invalid("operator_sh", nil)
	}

	// Paint the shading pattern
	// In a real implementation, this would:
	// 1. Get the current shading from the color space
	// 2. Render the shading to the current path or bounding box
	// For now, just mark that shading should be applied

	return nil
}

//revive:enable:exported
