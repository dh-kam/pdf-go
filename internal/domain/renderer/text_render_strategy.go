package renderer

import (
	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type textRenderer interface {
	Render(evaluator *Evaluator, text string, font entity.Font, fontSize float64, codeUnits []textCodeUnit) error
}

type defaultTextRenderer struct{}

func (defaultTextRenderer) Render(e *Evaluator, text string, font entity.Font, fontSize float64, codeUnits []textCodeUnit) error {
	if e.textPolicy.ShouldSkipAllText() {
		e.advanceTextWithoutRendering(codeUnits, font, fontSize)
		return nil
	}

	if e.textPolicy.ShouldSkipTextFont(e.graphics.fontDebugName, font) {
		e.captureTextWithoutRendering(codeUnits, font, fontSize)
		return nil
	}

	if e.canvas != nil {
		if e.textPolicy.HasSkippedTextCodes(e.graphics.fontDebugName, font) {
			return e.renderTextCharByChar(text, font, fontSize)
		}
		rendered, err := e.tryRenderTextWithFastPath(text, font, fontSize, codeUnits)
		if err != nil {
			return err
		}
		if rendered {
			return nil
		}

		// Use evaluator-controlled per-glyph placement for all rendered text so spacing
		// follows PDF text-state metrics consistently across font backends.
		return e.renderTextCharByChar(text, font, fontSize)
	}

	e.captureTextWithoutRendering(codeUnits, font, fontSize)
	return nil
}

func (e *Evaluator) advanceTextWithoutRendering(codeUnits []textCodeUnit, font entity.Font, fontSize float64) {
	for _, codeUnit := range codeUnits {
		dx := e.textPlacement.GlyphAdvance(e, codeUnit.code, font, fontSize)
		e.textPlacement.AdvanceTextMatrix(e, dx)
	}
}

func (e *Evaluator) captureTextWithoutRendering(codeUnits []textCodeUnit, font entity.Font, fontSize float64) {
	for _, codeUnit := range codeUnits {
		x, y := e.textPlacement.CurrentPosition(e)
		dx := e.textPlacement.GlyphAdvance(e, codeUnit.code, font, fontSize)
		e.textPlacement.AdvanceTextMatrix(e, dx)
		e.addTextContent(codeUnit.code, x, y, fontSize)
	}
}

func (e *Evaluator) tryRenderTextWithFastPath(text string, font entity.Font, fontSize float64, codeUnits []textCodeUnit) (bool, error) {
	if !e.textPolicy.ShouldUseFastPathTextRenderMode() || e.canvas == nil {
		return false, nil
	}

	// Type3 fonts render glyphs via content stream evaluation, not DrawText.
	// Force char-by-char path so renderType3Glyph is invoked.
	if unwrapType3Font(font) != nil {
		return false, nil
	}

	e.syncCanvasColors()
	e.syncCanvasGlyphTransform()
	startX, startY := e.textPlacement.CurrentPosition(e)
	if err := e.canvas.DrawText(text, startX, startY, font, fontSize); err != nil {
		return false, nil
	}

	e.captureTextWithoutRendering(codeUnits, font, fontSize)
	return true, nil
}
