// Package freetype provides stub implementation when FreeType is not available.
//go:build nofreetype

package freetype

import (
	"fmt"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// IsAvailable returns false when FreeType is not available.
func IsAvailable() bool {
	return false
}

// GetGlyphIndexByCharCode returns no match when FreeType is not available.
func GetGlyphIndexByCharCode(fontData []byte, charCode uint32) (uint32, bool) {
	return 0, false
}

// GetGlyphIndexByName returns no match when FreeType is not available.
func GetGlyphIndexByName(fontData []byte, glyphName string) (uint32, bool) {
	return 0, false
}

// GetGlyphNameByCharCode returns no match when FreeType is not available.
func GetGlyphNameByCharCode(fontData []byte, charCode uint32) (string, bool) {
	return "", false
}

// GetFaceBoundingBox returns no bounds when FreeType is not available.
func GetFaceBoundingBox(fontData []byte) (float64, float64, float64, float64, uint16, bool) {
	return 0, 0, 0, 0, 0, false
}

// RenderGlyph returns an error indicating FreeType is not available.
func RenderGlyph(fontData []byte, glyphCode uint32, size float64, dpi int) (*entity.GlyphPath, error) {
	return nil, fmt.Errorf("FreeType rendering not available (built with 'nofreetype' tag)")
}

// RenderGlyphByIndex returns an error indicating FreeType is not available.
func RenderGlyphByIndex(fontData []byte, glyphIndex uint32, sizePt float64, dpi int) (*entity.GlyphPath, error) {
	return nil, fmt.Errorf("FreeType rendering not available (built with 'nofreetype' tag)")
}

// RenderGlyphBitmap returns an error indicating FreeType is not available.
func RenderGlyphBitmap(fontData []byte, glyphCode uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error) {
	return nil, 0, 0, 0, 0, fmt.Errorf("FreeType rendering not available (built with 'nofreetype' tag)")
}

// RenderGlyphBitmapByIndex returns an error indicating FreeType is not available.
func RenderGlyphBitmapByIndex(fontData []byte, glyphIndex uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error) {
	return nil, 0, 0, 0, 0, fmt.Errorf("FreeType rendering not available (built with 'nofreetype' tag)")
}

// RenderGlyphBitmapPhased returns an error indicating FreeType is not available.
func RenderGlyphBitmapPhased(fontData []byte, glyphCode uint32, sizePt float64, dpi int, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	return nil, 0, 0, 0, 0, fmt.Errorf("FreeType rendering not available (built with 'nofreetype' tag)")
}

// RenderGlyphBitmapByIndexPhased returns an error indicating FreeType is not available.
func RenderGlyphBitmapByIndexPhased(fontData []byte, glyphIndex uint32, sizePt float64, dpi int, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	return nil, 0, 0, 0, 0, fmt.Errorf("FreeType rendering not available (built with 'nofreetype' tag)")
}

// RenderGlyphBitmapTransformedPhased returns an error indicating FreeType is not available.
func RenderGlyphBitmapTransformedPhased(fontData []byte, glyphCode uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	return nil, 0, 0, 0, 0, fmt.Errorf("FreeType rendering not available (built with 'nofreetype' tag)")
}

// RenderGlyphBitmapByIndexTransformedPhased returns an error indicating FreeType is not available.
func RenderGlyphBitmapByIndexTransformedPhased(fontData []byte, glyphIndex uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	return nil, 0, 0, 0, 0, fmt.Errorf("FreeType rendering not available (built with 'nofreetype' tag)")
}

// RenderGlyphBitmapMatrixPhased returns an error indicating FreeType is not available.
func RenderGlyphBitmapMatrixPhased(fontData []byte, glyphCode uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	return nil, 0, 0, 0, 0, fmt.Errorf("FreeType rendering not available (built with 'nofreetype' tag)")
}

// RenderGlyphBitmapByIndexMatrixPhased returns an error indicating FreeType is not available.
func RenderGlyphBitmapByIndexMatrixPhased(fontData []byte, glyphIndex uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	return nil, 0, 0, 0, 0, fmt.Errorf("FreeType rendering not available (built with 'nofreetype' tag)")
}
