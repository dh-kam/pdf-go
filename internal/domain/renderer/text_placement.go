package renderer

import (
	"os"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type textPlacement interface {
	CurrentRenderingMatrix(evaluator *Evaluator) [6]float64
	CurrentPosition(evaluator *Evaluator) (float64, float64)
	GlyphAdvance(evaluator *Evaluator, charCode uint32, font entity.Font, fontSize float64) float64
	AdvanceTextMatrix(evaluator *Evaluator, tx float64)
	MoveTextBy(evaluator *Evaluator, tx, ty float64)
	ApplyTextAdjustment(evaluator *Evaluator, adjustment, fontSize float64)
}

type defaultTextPlacement struct{}

func (defaultTextPlacement) CurrentRenderingMatrix(e *Evaluator) [6]float64 {
	textMatrix := e.textMatrix
	textRise := e.graphics.currentState.GetTextRise()
	if textRise != 0 {
		riseMatrix := [6]float64{1, 0, 0, 1, 0, textRise}
		textMatrix = multiplyMatrix(textMatrix, riseMatrix)
	}
	return multiplyMatrix(e.graphics.transform, textMatrix)
}

func (p defaultTextPlacement) CurrentPosition(e *Evaluator) (float64, float64) {
	if usePopplerTextCurrentShift() {
		if e.textCurrentValid {
			return e.textCurrentX, e.textCurrentY
		}
	}
	trm := p.CurrentRenderingMatrix(e)
	if usePopplerTextCurrentShift() {
		e.textCurrentX = trm[4]
		e.textCurrentY = trm[5]
		e.textCurrentValid = true
	}
	return trm[4], trm[5]
}

func usePopplerTextCurrentShift() bool {
	return os.Getenv("PDF_DEBUG_POPPLER_TEXT_SHIFT") == "1"
}

func (defaultTextPlacement) GlyphAdvance(e *Evaluator, charCode uint32, font entity.Font, fontSize float64) float64 {
	return e.glyphAdvance(charCode, font, fontSize)
}

func (defaultTextPlacement) AdvanceTextMatrix(e *Evaluator, tx float64) {
	e.advanceTextMatrix(tx)
}

func (defaultTextPlacement) MoveTextBy(e *Evaluator, tx, ty float64) {
	e.moveTextBy(tx, ty)
}

func (defaultTextPlacement) ApplyTextAdjustment(e *Evaluator, adjustment, fontSize float64) {
	hScale := e.graphics.currentState.GetHorizontalScaling() / 100.0
	if hScale == 0 {
		hScale = 1.0
	}

	tx := -adjustment / 1000.0 * fontSize * hScale
	e.advanceTextMatrix(tx)
}
