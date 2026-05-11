package renderer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type userSpaceTextDrawer interface {
	DrawTextUserSpace(text string, x, y float64, ctm [6]float64, font entity.Font, fontSize float64) error
}

// renderTextString renders a text string character by character.
func (e *Evaluator) renderTextString(text string) error {
	state := e.graphics.currentState

	font := state.GetFont()
	if font == nil {
		return nil
	}

	fontSize := state.GetFontSize()
	codeUnits := splitTextCodeUnits(text, font)
	if len(codeUnits) == 0 {
		return nil
	}
	return e.textRenderer.Render(e, text, font, fontSize, codeUnits)
}

// renderTextCharByChar renders text character by character as fallback.
func (e *Evaluator) renderTextCharByChar(text string, font entity.Font, fontSize float64) error {
	if e.canvas != nil {
		e.syncCanvasColors()
		e.syncCanvasGlyphTransform()
	}

	// Check if this is a Type3 font — use content stream evaluation instead of DrawText.
	type3Font := unwrapType3Font(font)

	for _, codeUnit := range splitTextCodeUnits(text, font) {
		x, y := e.textPlacement.CurrentPosition(e)

		if e.textPolicy.ShouldSkipTextCode(e.graphics.fontDebugName, font, codeUnit.code) {
			e.addTextContent(codeUnit.code, x, y, fontSize)
			dx := e.textPlacement.GlyphAdvance(e, codeUnit.code, font, fontSize)
			e.textPlacement.AdvanceTextMatrix(e, dx)
			continue
		}

		if e.canvas != nil {
			if type3Font != nil {
				if err := e.renderType3Glyph(type3Font, codeUnit.code, x, y, fontSize); err != nil {
					// Log but continue rendering other glyphs
					continue
				}
			} else {
				rawText := string(codeUnit.raw)
				if drawer, ok := e.canvas.(userSpaceTextDrawer); ok {
					userX, userY := e.currentTextUserPosition()
					if err := drawer.DrawTextUserSpace(rawText, userX, userY, e.graphics.transform, font, fontSize); err != nil {
						return err
					}
				} else if err := e.canvas.DrawText(rawText, x, y, font, fontSize); err != nil {
					return err
				}
			}
		}

		e.addTextContent(codeUnit.code, x, y, fontSize)

		dx := e.textPlacement.GlyphAdvance(e, codeUnit.code, font, fontSize)
		e.textPlacement.AdvanceTextMatrix(e, dx)
	}

	return nil
}

func (e *Evaluator) currentTextUserPosition() (float64, float64) {
	textMatrix := e.textMatrix
	x, y := textMatrix[4], textMatrix[5]
	if e.textUserCurrentValid {
		x = e.textUserCurrentX
		y = e.textUserCurrentY
	}
	textRise := e.graphics.currentState.GetTextRise()
	if textRise != 0 {
		x += textMatrix[2] * textRise
		y += textMatrix[3] * textRise
	}
	return x, y
}

// unwrapType3Font unwraps font wrappers to find the underlying Type3Font, if any.
func unwrapType3Font(font entity.Font) *entity.Type3Font {
	for f := font; f != nil; {
		if t3, ok := f.(*entity.Type3Font); ok {
			return t3
		}
		switch w := f.(type) {
		case *widthMappedFont:
			f = w.base
		case *encodedFont:
			f = w.base
		case *glyphSourceOverrideFont:
			f = w.base
		case *type1CCodeToGIDFont:
			f = w.base
		default:
			return nil
		}
	}
	return nil
}

// addTextContent stores text content for extraction.
func (e *Evaluator) addTextContent(charCode uint32, x, y, fontSize float64) {
	font := e.graphics.currentState.GetFont()
	if font != nil {
		if glyph, err := font.CharCodeToGlyph(charCode); err == nil {
			if glyphName := font.GlyphName(glyph); glyphName != "" {
				if r, ok := decodeGlyphName(glyphName); ok {
					e.textBuffer.WriteRune(r)
					return
				}
			}
		}
	}

	if charCode <= 0xFF {
		e.textBuffer.WriteByte(byte(charCode))
		return
	}
	if charCode <= 0x10FFFF {
		e.textBuffer.WriteRune(rune(charCode))
	}
}

func decodeGlyphName(name string) (rune, bool) {
	if len(name) == 0 {
		return 0, false
	}

	if strings.HasPrefix(name, "uni") && len(name) == 7 {
		if v, err := strconv.ParseInt(name[3:], 16, 32); err == nil {
			return rune(v), true
		}
	}
	if strings.HasPrefix(name, "u") && len(name) == 5 {
		if v, err := strconv.ParseInt(name[1:], 16, 32); err == nil {
			return rune(v), true
		}
	}

	switch name {
	case "space":
		return ' ', true
	case "tab":
		return '\t', true
	case "hyphen":
		return '-', true
	case "period":
		return '.', true
	case "comma":
		return ',', true
	case "colon":
		return ':', true
	case "semicolon":
		return ';', true
	}

	if len(name) == 1 {
		return rune(name[0]), true
	}

	return 0, false
}

func (e *Evaluator) showTextArray(op Operator) error {
	if len(op.Operands) < 1 {
		return fmt.Errorf("tj operator requires 1 operand")
	}

	arr, ok := op.Operands[0].(*entity.Array)
	if !ok {
		return fmt.Errorf("tj operand is not an array")
	}

	state := e.graphics.currentState
	font := state.GetFont()
	if font == nil {
		return nil
	}

	fontSize := state.GetFontSize()

	for i := 0; i < arr.Len(); i++ {
		item := arr.Get(i)

		switch v := item.(type) {
		case *entity.String:
			if err := e.renderTextString(v.Value()); err != nil {
				return err
			}
		case *entity.Integer:
			e.textPlacement.ApplyTextAdjustment(e, float64(v.Value()), fontSize)
		case *entity.Real:
			e.textPlacement.ApplyTextAdjustment(e, v.Value(), fontSize)
		}
	}

	return nil
}

func (e *Evaluator) moveText(op Operator) error {
	if len(op.Operands) < 2 {
		return fmt.Errorf("td operator requires 2 operands")
	}

	tx, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("td tx is not a number")
	}
	ty, err := getNumberOperand(op.Operands[1])
	if err != nil {
		return fmt.Errorf("td ty is not a number")
	}

	e.textPlacement.MoveTextBy(e, tx, ty)
	return nil
}

func (e *Evaluator) moveTextSetLeading(op Operator) error {
	if len(op.Operands) < 2 {
		return fmt.Errorf("td operator requires 2 operands")
	}

	tx, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("td tx is not a number")
	}
	ty, err := getNumberOperand(op.Operands[1])
	if err != nil {
		return fmt.Errorf("td ty is not a number")
	}

	e.graphics.currentState.SetTextLeading(-ty)
	e.textPlacement.MoveTextBy(e, tx, ty)
	return nil
}

func (e *Evaluator) moveTextNextLine() error {
	leading := e.graphics.currentState.GetTextLeading()
	e.textPlacement.MoveTextBy(e, 0, -leading)
	return nil
}

func (e *Evaluator) moveTextNextLineAndShowText(op Operator) error {
	if len(op.Operands) != 1 {
		return fmt.Errorf("' operator requires 1 operand")
	}

	if err := e.moveTextNextLine(); err != nil {
		return err
	}

	return e.showText(Operator{
		Opcode:   "Tj",
		Operands: op.Operands,
	})
}

func (e *Evaluator) setSpacingMoveTextNextLineAndShowText(op Operator) error {
	if len(op.Operands) != 3 {
		return fmt.Errorf("\" operator requires 3 operands")
	}

	if err := e.setWordSpacing(Operator{
		Opcode:   "Tw",
		Operands: []entity.Object{op.Operands[0]},
	}); err != nil {
		return err
	}

	if err := e.setCharSpacing(Operator{
		Opcode:   "Tc",
		Operands: []entity.Object{op.Operands[1]},
	}); err != nil {
		return err
	}

	if err := e.moveTextNextLine(); err != nil {
		return err
	}

	return e.showText(Operator{
		Opcode:   "Tj",
		Operands: []entity.Object{op.Operands[2]},
	})
}
