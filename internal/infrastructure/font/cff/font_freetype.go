//go:build !nofreetype

package cff

import (
	"fmt"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/cgo/freetype"
)

// GlyphIDByName maps an Adobe glyph name to a FreeType glyph index.
// Uses FT_Get_Name_Index so CFF fonts without a cmap can be looked up by name.
func (f *Font) GlyphIDByName(name string) (uint32, bool) {
	if len(f.data) == 0 || !freetype.IsAvailable() {
		return 0, false
	}
	return freetype.GetGlyphIndexByName(f.data, name)
}

// FreeTypeBoundingBox returns FreeType's face bbox used by Poppler glyph cache sizing.
func (f *Font) FreeTypeBoundingBox() (float64, float64, float64, float64, uint16, bool) {
	if len(f.data) == 0 || !freetype.IsAvailable() {
		return 0, 0, 0, 0, 0, false
	}
	return freetype.GetFaceBoundingBox(f.data)
}

// EncodingName returns the CFF built-in encoding name for a character code.
// This mirrors Poppler's FoFiType1C::getEncoding path for simple Type1C fonts.
func (f *Font) EncodingName(code byte) string {
	if len(f.data) == 0 || !freetype.IsAvailable() {
		return ""
	}
	name, ok := freetype.GetGlyphNameByCharCode(f.data, uint32(code))
	if !ok {
		return ""
	}
	return name
}

// CharCodeToGlyph maps a char code to a FreeType glyph index using the font's
// built-in charmap (with Adobe encoding preference). This correctly handles
// CFF fonts with custom or standard encoding without identity-mapping char codes.
func (f *Font) CharCodeToGlyph(code uint32) (uint32, error) {
	if len(f.data) > 0 && freetype.IsAvailable() {
		if idx, ok := freetype.GetGlyphIndexByCharCode(f.data, code); ok {
			return idx, nil
		}
	}
	return code, nil
}

// RenderGlyph renders the glyph using FreeType for pixel-accurate CFF outlines.
// Falls back to the pure-Go charstring renderer if FreeType is unavailable.
func (f *Font) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	if len(f.data) > 0 && freetype.IsAvailable() && glyph > 0 {
		path, err := freetype.RenderGlyphByIndex(f.data, glyph, size, 300)
		if err == nil {
			return path, nil
		}
	}
	return f.renderGlyphCharString(glyph, size)
}

// RenderGlyphBitmap renders the glyph as a grayscale bitmap via FreeType.
// Implements entity.BitmapGlyphRenderer for pixel-accurate rendering.
func (f *Font) RenderGlyphBitmap(glyph uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error) {
	if len(f.data) == 0 || !freetype.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("cff: FreeType not available")
	}
	return freetype.RenderGlyphBitmapByIndex(f.data, glyph, sizePt, dpi)
}

// RenderGlyphBitmapPhased renders the glyph with sub-pixel phase for accurate antialiasing.
// Implements entity.BitmapGlyphRendererPhased.
func (f *Font) RenderGlyphBitmapPhased(glyph uint32, sizePt float64, dpi int, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if len(f.data) == 0 || !freetype.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("cff: FreeType not available for phased rendering")
	}
	return freetype.RenderGlyphBitmapByIndexPhased(f.data, glyph, sizePt, dpi, phaseX, phaseY)
}

// RenderGlyphBitmapTransformedPhased renders the glyph with Poppler-style
// axis-aligned FreeType transform scaling and phase.
func (f *Font) RenderGlyphBitmapTransformedPhased(glyph uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if len(f.data) == 0 || !freetype.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("cff: FreeType not available for transformed rendering")
	}
	return freetype.RenderGlyphBitmapByIndexTransformedPhased(f.data, glyph, sizePt, scaleX, scaleY, phaseX, phaseY)
}

// RenderGlyphBitmapMatrixPhased renders the glyph with Poppler's full 2x2
// FreeType transform matrix and phase.
func (f *Font) RenderGlyphBitmapMatrixPhased(glyph uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if len(f.data) == 0 || !freetype.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("cff: FreeType not available for matrix rendering")
	}
	return freetype.RenderGlyphBitmapByIndexMatrixPhased(f.data, glyph, sizePt, matrix, phaseX, phaseY)
}
