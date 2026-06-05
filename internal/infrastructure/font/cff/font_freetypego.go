package cff

import (
	"fmt"
	"os"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/freetype"
)

// GlyphIDByName maps an Adobe glyph name through the pure Go FreeType adapter when possible.
func (f *Font) GlyphIDByName(name string) (uint32, bool) {
	if !freeTypeGoAdapterEnabled() || len(f.data) == 0 || !freetype.IsAvailable() {
		return 0, false
	}
	return freetype.GetGlyphIndexByName(f.data, name)
}

// FreeTypeBoundingBox returns face bbox data through the pure Go FreeType adapter when possible.
func (f *Font) FreeTypeBoundingBox() (float64, float64, float64, float64, uint16, bool) {
	if !freeTypeGoAdapterEnabled() || len(f.data) == 0 || !freetype.IsAvailable() {
		return 0, 0, 0, 0, 0, false
	}
	return freetype.GetFaceBoundingBox(f.data)
}

// EncodingName returns the CFF built-in encoding name for a character code when available.
func (f *Font) EncodingName(code byte) string {
	if !freeTypeGoAdapterEnabled() || len(f.data) == 0 || !freetype.IsAvailable() {
		return ""
	}
	name, ok := freetype.GetGlyphNameByCharCode(f.data, uint32(code))
	if !ok {
		return ""
	}
	return name
}

// CIDToGIDMap returns a Poppler-style CID-to-GID map for CID-keyed CFF fonts.
func (f *Font) CIDToGIDMap() (map[uint32]uint32, bool) {
	if !freeTypeGoAdapterEnabled() || len(f.data) == 0 || !freetype.IsAvailable() {
		return nil, false
	}
	return freetype.GetCIDToGIDMap(f.data)
}

// CharCodeToGlyph maps char codes through the pure Go FreeType adapter when possible.
func (f *Font) CharCodeToGlyph(code uint32) (uint32, error) {
	if freeTypeGoAdapterEnabled() && len(f.data) > 0 && freetype.IsAvailable() {
		if idx, ok := freetype.GetGlyphIndexByCharCode(f.data, code); ok {
			return idx, nil
		}
	}
	return code, nil
}

// RenderGlyph falls back to the pure-Go charstring renderer when FreeType is disabled.
func (f *Font) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	if freeTypeGoAdapterEnabled() && len(f.data) > 0 && freetype.IsAvailable() && glyph > 0 {
		path, err := freetype.RenderGlyphByIndex(f.data, glyph, size, 300)
		if err == nil {
			return path, nil
		}
	}
	return f.renderGlyphCharString(glyph, size)
}

// RenderGlyphBitmap renders the glyph through the pure Go FreeType adapter.
func (f *Font) RenderGlyphBitmap(glyph uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error) {
	if !freeTypeGoAdapterEnabled() || len(f.data) == 0 || !freetype.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("cff: pure Go FreeType adapter not available")
	}
	return freetype.RenderGlyphBitmapByIndex(f.data, glyph, sizePt, dpi)
}

// RenderGlyphBitmapPhased renders the glyph with sub-pixel phase.
func (f *Font) RenderGlyphBitmapPhased(glyph uint32, sizePt float64, dpi int, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if !freeTypeGoAdapterEnabled() || len(f.data) == 0 || !freetype.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("cff: pure Go FreeType adapter not available for phased rendering")
	}
	return freetype.RenderGlyphBitmapByIndexPhased(f.data, glyph, sizePt, dpi, phaseX, phaseY)
}

// RenderGlyphBitmapTransformedPhased renders the glyph with axis-aligned transform scaling.
func (f *Font) RenderGlyphBitmapTransformedPhased(glyph uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if !freeTypeGoAdapterEnabled() || len(f.data) == 0 || !freetype.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("cff: pure Go FreeType adapter not available for transformed rendering")
	}
	return freetype.RenderGlyphBitmapByIndexTransformedPhased(f.data, glyph, sizePt, scaleX, scaleY, phaseX, phaseY)
}

// RenderGlyphBitmapMatrixPhased renders the glyph with the full 2x2 glyph transform matrix.
func (f *Font) RenderGlyphBitmapMatrixPhased(glyph uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if !freeTypeGoAdapterEnabled() || len(f.data) == 0 || !freetype.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("cff: pure Go FreeType adapter not available for matrix rendering")
	}
	return freetype.RenderGlyphBitmapByIndexMatrixPhased(f.data, glyph, sizePt, matrix, phaseX, phaseY)
}

// RenderGlyphPathMatrix renders a CFF glyph outline through the same pure Go
// FreeType matrix path used by Poppler SplashFTFont::getGlyphPath.
func (f *Font) RenderGlyphPathMatrix(glyph uint32, sizePt float64, matrix [4]float64) (*entity.GlyphPath, error) {
	if !freeTypeGoAdapterEnabled() || len(f.data) == 0 || !freetype.IsAvailable() {
		return nil, fmt.Errorf("cff: pure Go FreeType adapter not available for matrix path rendering")
	}
	return freetype.RenderGlyphPathByIndexMatrix(f.data, glyph, sizePt, matrix)
}

// RenderGlyphPathTextMatrix renders a CFF glyph outline in Poppler callback text
// coordinates so Splash can apply the final device matrix during path setup.
func (f *Font) RenderGlyphPathTextMatrix(glyph uint32, sizePt float64, matrix [4]float64) (*entity.GlyphPath, error) {
	if !freeTypeGoAdapterEnabled() || len(f.data) == 0 || !freetype.IsAvailable() {
		return nil, fmt.Errorf("cff: pure Go FreeType adapter not available for text-matrix path rendering")
	}
	return freetype.RenderGlyphPathByIndexTextMatrix(f.data, glyph, sizePt, matrix)
}

func freeTypeGoAdapterEnabled() bool {
	switch os.Getenv("PDF_FREETYPE_GO") {
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	default:
		return true
	}
}
