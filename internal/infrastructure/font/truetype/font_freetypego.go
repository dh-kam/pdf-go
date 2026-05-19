package truetype

import (
	"fmt"
	"os"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	ftcgo "github.com/dh-kam/pdf-go/internal/infrastructure/font/freetype"
)

// RenderGlyph renders a glyph to a path using our pure-Go TrueType parser.
func (f *Font) RenderGlyph(glyph uint32, size float64) (result *entity.GlyphPath, resultErr error) {
	defer func() {
		if r := recover(); r != nil {
			result = nil
			resultErr = fmt.Errorf("glyph render panic: %v", r)
		}
	}()

	return f.renderGlyphGoParser(glyph, size)
}

// RenderGlyphBitmap renders the glyph as a grayscale bitmap through the pure Go FreeType adapter.
func (f *Font) RenderGlyphBitmap(glyph uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error) {
	if !freeTypeGoBitmapAdapterEnabled() || len(f.data) == 0 || !ftcgo.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("truetype: pure Go FreeType adapter not available")
	}
	return ftcgo.RenderGlyphBitmapByIndex(f.data, glyph, sizePt, dpi)
}

// RenderGlyphBitmapPhased renders the glyph with sub-pixel phase through the pure Go FreeType adapter.
func (f *Font) RenderGlyphBitmapPhased(glyph uint32, sizePt float64, dpi int, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if !freeTypeGoBitmapAdapterEnabled() || len(f.data) == 0 || !ftcgo.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("truetype: pure Go FreeType adapter not available for phased rendering")
	}
	return ftcgo.RenderGlyphBitmapByIndexPhased(f.data, glyph, sizePt, dpi, phaseX, phaseY)
}

// RenderGlyphBitmapTransformedPhased renders the glyph with axis-aligned transform scaling.
func (f *Font) RenderGlyphBitmapTransformedPhased(glyph uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if !freeTypeGoBitmapAdapterEnabled() || len(f.data) == 0 || !ftcgo.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("truetype: pure Go FreeType adapter not available for transformed rendering")
	}
	return ftcgo.RenderGlyphBitmapByIndexTransformedPhased(f.data, glyph, sizePt, scaleX, scaleY, phaseX, phaseY)
}

// RenderGlyphBitmapMatrixPhased renders the glyph with the full 2x2 glyph transform matrix.
func (f *Font) RenderGlyphBitmapMatrixPhased(glyph uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if !freeTypeGoBitmapAdapterEnabled() || len(f.data) == 0 || !ftcgo.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("truetype: pure Go FreeType adapter not available for matrix rendering")
	}
	return ftcgo.RenderGlyphBitmapByIndexMatrixPhased(f.data, glyph, sizePt, matrix, phaseX, phaseY)
}

func freeTypeGoBitmapAdapterEnabled() bool {
	switch os.Getenv("PDF_FREETYPE_GO") {
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	default:
		return true
	}
}
