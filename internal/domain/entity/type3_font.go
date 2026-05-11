package entity

import "fmt"

// Type3Font represents a PDF Type 3 font where glyphs are defined as content streams.
type Type3Font struct {
	name       string
	fontMatrix [6]float64
	charProcs  map[string]*Stream // glyph name -> content stream
	encoding   map[uint32]string  // char code -> glyph name
	widths     map[uint32]float64 // char code -> width in glyph space
	firstChar  uint32
	lastChar   uint32
	bbox       [4]float64
	resources  *Dict
}

// NewType3Font creates a new Type3Font from a font dictionary's parsed data.
func NewType3Font(name string, fontMatrix [6]float64, charProcs map[string]*Stream, encoding map[uint32]string, widths map[uint32]float64, firstChar, lastChar uint32, bbox [4]float64) *Type3Font {
	return &Type3Font{
		name:       name,
		fontMatrix: fontMatrix,
		charProcs:  charProcs,
		encoding:   encoding,
		widths:     widths,
		firstChar:  firstChar,
		lastChar:   lastChar,
		bbox:       bbox,
	}
}

// FontMatrix returns the font's transformation matrix.
func (f *Type3Font) FontMatrix() [6]float64 {
	return f.fontMatrix
}

// CharProcForCode returns the content stream for a character code, or nil.
func (f *Type3Font) CharProcForCode(code uint32) *Stream {
	glyphName, ok := f.encoding[code]
	if !ok {
		return nil
	}
	return f.charProcs[glyphName]
}

// CharProcForGlyph returns the content stream for a glyph name, or nil.
func (f *Type3Font) CharProcForGlyph(glyphName string) *Stream {
	return f.charProcs[glyphName]
}

// CharCodeToGlyph maps a character code to a glyph identifier.
// For Type3 fonts, the glyph is the char code itself (identity mapping),
// since the encoding already maps char codes to glyph names.
func (f *Type3Font) CharCodeToGlyph(code uint32) (uint32, error) {
	return code, nil
}

// GlyphName returns the glyph name for a glyph identifier (char code).
func (f *Type3Font) GlyphName(glyph uint32) string {
	if name, ok := f.encoding[glyph]; ok {
		return name
	}
	return ""
}

// GetGlyphWidth returns the width for a character code in glyph space.
func (f *Type3Font) GetGlyphWidth(glyph uint32) (float64, error) {
	if w, ok := f.widths[glyph]; ok {
		return w, nil
	}
	// Default width: use 0 or the font bbox width
	return f.bbox[2] - f.bbox[0], fmt.Errorf("type3 font: no width for char code %d", glyph)
}

// GetBoundingBox returns the font bounding box.
func (f *Type3Font) GetBoundingBox() (float64, float64, float64, float64) {
	return f.bbox[0], f.bbox[1], f.bbox[2], f.bbox[3]
}

// Resources returns the font resource dictionary used by Type3 glyph programs.
func (f *Type3Font) Resources() *Dict {
	return f.resources
}

// SetResources stores the font resource dictionary used by Type3 glyph programs.
func (f *Type3Font) SetResources(resources *Dict) {
	f.resources = resources
}

// RenderGlyph returns nil for Type3 fonts since glyph rendering
// is done by evaluating the CharProcs content stream, not by path outlines.
func (f *Type3Font) RenderGlyph(glyph uint32, size float64) (*GlyphPath, error) {
	return nil, fmt.Errorf("type3 font: glyph rendering via content stream evaluation")
}

// IsCIDFont returns false for Type3 fonts.
func (f *Type3Font) IsCIDFont() bool { return false }

// IsSymbolic returns false by default for Type3 fonts.
func (f *Type3Font) IsSymbolic() bool { return false }

// UnitsPerEm returns 1000 for Type3 fonts (standard default).
func (f *Type3Font) UnitsPerEm() uint16 { return 1000 }

// Name returns the font name.
func (f *Type3Font) Name() string { return f.name }
