//go:build !nofreetype

package truetype

import (
	"fmt"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	ftcgo "github.com/dh-kam/pdf-go/internal/infrastructure/cgo/freetype"
)

// RenderGlyph renders a glyph to a path using FreeType, falling back to the pure-Go parser.
func (f *Font) RenderGlyph(glyph uint32, size float64) (result *entity.GlyphPath, resultErr error) {
	defer func() {
		if r := recover(); r != nil {
			result = nil
			resultErr = fmt.Errorf("glyph render panic: %v", r)
		}
	}()

	if len(f.data) > 0 {
		path, err := ftcgo.RenderGlyphByIndex(f.data, glyph, size, 72)
		if err == nil && path != nil && len(path.Commands) > 0 {
			return path, nil
		}
	}

	return f.renderGlyphGoParser(glyph, size)
}

// RenderGlyphBitmap renders the glyph as a grayscale bitmap via FreeType.
// Implements entity.BitmapGlyphRenderer for pixel-accurate rendering matching Poppler.
func (f *Font) RenderGlyphBitmap(glyph uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error) {
	if len(f.data) == 0 || !ftcgo.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("truetype: FreeType not available")
	}
	return ftcgo.RenderGlyphBitmapByIndex(f.data, glyph, sizePt, dpi)
}

// RenderGlyphBitmapPhased renders the glyph with sub-pixel phase for accurate antialiasing.
// Implements entity.BitmapGlyphRendererPhased.
func (f *Font) RenderGlyphBitmapPhased(glyph uint32, sizePt float64, dpi int, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if len(f.data) == 0 || !ftcgo.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("truetype: FreeType not available for phased rendering")
	}
	return ftcgo.RenderGlyphBitmapByIndexPhased(f.data, glyph, sizePt, dpi, phaseX, phaseY)
}

// RenderGlyphBitmapTransformedPhased renders the glyph with Poppler-style
// axis-aligned FreeType transform scaling and phase.
func (f *Font) RenderGlyphBitmapTransformedPhased(glyph uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if len(f.data) == 0 || !ftcgo.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("truetype: FreeType not available for transformed rendering")
	}
	return ftcgo.RenderGlyphBitmapByIndexTransformedPhased(f.data, glyph, sizePt, scaleX, scaleY, phaseX, phaseY)
}

// RenderGlyphBitmapMatrixPhased renders the glyph with Poppler's full 2x2
// FreeType transform matrix and phase.
func (f *Font) RenderGlyphBitmapMatrixPhased(glyph uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if len(f.data) == 0 || !ftcgo.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("truetype: FreeType not available for matrix rendering")
	}
	return ftcgo.RenderGlyphBitmapByIndexMatrixPhased(f.data, glyph, sizePt, matrix, phaseX, phaseY)
}
