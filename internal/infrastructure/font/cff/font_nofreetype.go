//go:build nofreetype

package cff

import (
	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// GlyphIDByName returns false when FreeType is disabled.
func (f *Font) GlyphIDByName(_ string) (uint32, bool) {
	return 0, false
}

// FreeTypeBoundingBox returns false when FreeType is disabled.
func (f *Font) FreeTypeBoundingBox() (float64, float64, float64, float64, uint16, bool) {
	return 0, 0, 0, 0, 0, false
}

// CharCodeToGlyph falls back to identity mapping when FreeType is disabled.
func (f *Font) CharCodeToGlyph(code uint32) (uint32, error) {
	return code, nil
}

// RenderGlyph falls back to the pure-Go charstring renderer when FreeType is disabled.
func (f *Font) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return f.renderGlyphCharString(glyph, size)
}
