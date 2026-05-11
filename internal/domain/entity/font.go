// Package entity provides PDF font handling functionality.
package entity

// Font represents a PDF font.
type Font interface {
	// Character mapping
	CharCodeToGlyph(code uint32) (uint32, error)
	GlyphName(glyph uint32) string

	// Metrics
	GetGlyphWidth(glyph uint32) (float64, error)
	GetBoundingBox() (float64, float64, float64, float64)

	// Rendering
	RenderGlyph(glyph uint32, size float64) (*GlyphPath, error)

	// Properties - no setters, immutable
	IsCIDFont() bool
	IsSymbolic() bool
	UnitsPerEm() uint16
	Name() string
}

// GlyphPath represents a glyph outline as a path.
type GlyphPath struct {
	Commands []PathCommand
	Bounds   [4]float64 // [xMin, yMin, xMax, yMax]
}

// PathCommand represents a path drawing command.
type PathCommand interface {
	Type() PathCmdType
}

// PathCmdType represents the type of path command.
type PathCmdType int

const (
	// CmdMoveTo moves the current point.
	CmdMoveTo PathCmdType = iota
	// CmdLineTo draws a line to a point.
	CmdLineTo
	// CmdCurveTo draws a cubic bezier curve.
	CmdCurveTo
	// CmdClose closes the current path.
	CmdClose
)

// PathMoveTo represents a move-to command.
type PathMoveTo struct {
	X, Y float64
}

// Type returns the command type.
func (p *PathMoveTo) Type() PathCmdType {
	return CmdMoveTo
}

// PathLineTo represents a line-to command.
type PathLineTo struct {
	X, Y float64
}

// Type returns the command type.
func (p *PathLineTo) Type() PathCmdType {
	return CmdLineTo
}

// PathCurveTo represents a curve-to command (cubic Bézier).
type PathCurveTo struct {
	X1, Y1, X2, Y2, X3, Y3 float64
}

// Type returns the command type.
func (p *PathCurveTo) Type() PathCmdType {
	return CmdCurveTo
}

// PathClose represents a close-path command.
type PathClose struct{}

// Type returns the command type.
func (p *PathClose) Type() PathCmdType {
	return CmdClose
}

// BitmapGlyphRenderer is an optional interface for fonts that can render
// glyphs to bitmaps directly (e.g., via FreeType). This gives pixel-identical
// rendering to Poppler for Type1 fonts.
type BitmapGlyphRenderer interface {
	// RenderGlyphBitmap renders a glyph to a grayscale alpha bitmap.
	// Returns alpha buffer, width, height, bearingX (left offset), bearingY (top offset from baseline).
	RenderGlyphBitmap(glyph uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error)
}

// BitmapGlyphRendererPhased extends BitmapGlyphRenderer with sub-pixel phase support.
// phaseX shifts the glyph rightward (0.0 to <1.0 pixels), phaseY shifts downward in canvas coords.
// Using the correct sub-pixel phase produces antialiasing that matches Poppler's rendering.
type BitmapGlyphRendererPhased interface {
	BitmapGlyphRenderer
	RenderGlyphBitmapPhased(glyph uint32, sizePt float64, dpi int, phaseX, phaseY float64) ([]byte, int, int, int, int, error)
}

// FontDescriptor contains font metrics.
type FontDescriptor struct {
	FontName     string
	FontFamily   string
	Flags        uint32
	ItalicAngle  float64
	Ascent       float64
	Descent      float64
	CapHeight    float64
	StemV        float64
	MissingWidth float64
}

// FontFlags constants.
const (
	FlagFixedPitch  uint32 = 1 << 0
	FlagSerif       uint32 = 1 << 1
	FlagSymbolic    uint32 = 1 << 2
	FlagScript      uint32 = 1 << 3
	FlagNonsymbolic uint32 = 1 << 5
	FlagItalic      uint32 = 1 << 6
	FlagAllCap      uint32 = 1 << 16
	FlagSmallCap    uint32 = 1 << 17
	ForceBold       uint32 = 1 << 18
)

// BoundingBox represents a font bounding box.
type BoundingBox struct {
	XMin float64
	YMin float64
	XMax float64
	YMax float64
}

// Encoding represents a character to glyph mapping.
type Encoding interface {
	Encode(charCode uint32) (uint32, error)
	Decode(glyph uint32) (uint32, error)
}

// StandardEncoding is the standard PDF encoding.
type StandardEncoding struct {
	encoding map[byte]uint32
}

// NewStandardEncoding creates a new StandardEncoding.
func NewStandardEncoding() *StandardEncoding {
	// Standard PDF encoding for Latin fonts
	return &StandardEncoding{
		encoding: map[byte]uint32{
			// Characters 32-126 follow ASCII
			// Special character mappings
		},
	}
}

// Encode maps a character code to a glyph code.
func (e *StandardEncoding) Encode(charCode uint32) (uint32, error) {
	if charCode < 256 {
		return charCode, nil
	}
	return charCode, nil
}

// Decode maps a glyph code to a character code.
func (e *StandardEncoding) Decode(glyph uint32) (uint32, error) {
	return glyph, nil
}

// UnicodeEncoding maps Unicode to glyphs.
type UnicodeEncoding struct {
	cmap map[rune]uint32
}

// NewUnicodeEncoding creates a new UnicodeEncoding.
func NewUnicodeEncoding() *UnicodeEncoding {
	return &UnicodeEncoding{
		cmap: make(map[rune]uint32),
	}
}

// Encode maps a character code to a glyph code.
func (e *UnicodeEncoding) Encode(charCode uint32) (uint32, error) {
	return charCode, nil
}

// Decode maps a glyph code to a character code.
func (e *UnicodeEncoding) Decode(glyph uint32) (uint32, error) {
	return glyph, nil
}

// ToUnicode maps character codes to Unicode strings.
type ToUnicode interface {
	ToUnicode(charCode uint32) (rune, bool)
}

// SimpleToUnicode is a basic identity mapping.
type SimpleToUnicode struct{}

// ToUnicode maps a character code to one Unicode rune.
func (t *SimpleToUnicode) ToUnicode(charCode uint32) (rune, bool) {
	return rune(charCode), charCode < 0x10FFFF
}
