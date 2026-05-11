// Package type1 provides Type1 font implementation.
package type1

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	ftcgo "github.com/dh-kam/pdf-go/internal/infrastructure/cgo/freetype"
	standardfont "github.com/dh-kam/pdf-go/internal/infrastructure/font/standard"
)

// Font represents a Type1 font.
type Font struct {
	file               *FontFile
	glyphs             map[uint32]*Glyph
	encoding           map[byte]string
	chars              map[string]*Glyph
	fontName           string
	subrs              [][]Command
	private            [][]Command
	fontInfo           FontInfo
	bbox               [4]float64
	ascent             float64
	descent            float64
	capHeight          float64
	sfntFont           *sfnt.Font // CFF-based OTF for sfnt rendering
	otfData            []byte     // OTF binary for FreeType rendering
	ftGlyphIndexByCode map[uint32]freeTypeGlyphIndex
}

type freeTypeGlyphIndex struct {
	source int
	index  uint32
}

// Glyph represents a Type1 glyph.
type Glyph struct {
	Path       *entity.GlyphPath
	Name       string
	CharString []byte
	Commands   []Command
	BBox       [4]float64
	Width      float64
	LSB        float64
	fallback   *standardFallbackGlyph
}

type standardFallbackGlyph struct {
	font  *standardfont.StandardFont
	glyph uint32
}

// NewFont creates a new Type1 font from a file path.
func NewFont(path string) (*Font, error) {
	file, err := ReadFromFile(path)
	if err != nil {
		return nil, err
	}
	return NewFontFromFile(file)
}

// NewFontFromBytes creates a new Type1 font from byte data.
func NewFontFromBytes(data []byte) (*Font, error) {
	file, err := Parse(data)
	if err != nil {
		return nil, err
	}

	return NewFontFromFile(file)
}

// NewFontFromData creates a new Type1 font from byte data (alias for NewFontFromBytes).
func NewFontFromData(data []byte) (*Font, error) {
	return NewFontFromBytes(data)
}

// NewFontFromReader creates a new Type1 font from a reader.
func NewFontFromReader(r io.Reader) (*Font, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return NewFontFromBytes(data)
}

// NewFontFromFile creates a new Type1 font from a parsed FontFile.
func NewFontFromFile(file *FontFile) (*Font, error) {
	font := &Font{
		file:     file,
		glyphs:   make(map[uint32]*Glyph),
		chars:    make(map[string]*Glyph),
		encoding: make(map[byte]string),
		subrs:    make([][]Command, 0),
		private:  make([][]Command, 0),
		fontName: file.FontName,
		fontInfo: file.FontInfo,
		bbox:     file.FontInfo.FontBBox,
	}

	// Parse encoding
	if len(file.Encoding) > 0 {
		font.encoding = file.Encoding
	} else {
		// Use StandardEncoding
		font.encoding = getStandardEncoding()
	}

	// Parse CharStrings
	err := font.parseCharStrings()
	if err != nil {
		return nil, err
	}

	// Extract font metrics
	font.extractMetrics()

	// Build OTF and load via sfnt for high-quality rendering
	font.buildSfntFont()
	font.buildFreeTypeNameIndex()

	return font, nil
}

// Parse parses Type1 font data and returns a FontFile.
func Parse(data []byte) (*FontFile, error) {
	parser := NewParser(data)
	return parser.Parse()
}

// parseCharStrings parses the CharStrings data from the font.
func (f *Font) parseCharStrings() error {
	charStrings, subrs, _, err := f.file.GetType1CharStringData()
	if err != nil || len(charStrings) == 0 {
		// Fallback to minimal glyphs when CharStrings are unavailable.
		f.createMinimalGlyphs()
		return nil
	}

	parsedAny := false
	for charCode, glyphName := range f.encoding {
		if glyphName == "" {
			f.createFallbackGlyph(uint32(charCode), "")
			continue
		}

		raw, ok := charStrings[glyphName]
		glyph := f.createGlyphFromCharString(glyphName, raw, subrs)
		if !ok || len(glyph.Commands) == 0 {
			f.createFallbackGlyph(uint32(charCode), glyphName)
			continue
		}

		f.glyphs[uint32(charCode)] = glyph
		f.chars[glyphName] = glyph
		parsedAny = true
	}

	if !parsedAny {
		// If all entries could not be resolved, keep renderer usable.
		f.createMinimalGlyphs()
	}

	return nil
}

func (f *Font) createGlyphFromCharString(name string, raw []byte, subrs [][]byte) *Glyph {
	glyph := &Glyph{
		Name:       name,
		CharString: append([]byte(nil), raw...),
		Width:      500,
		LSB:        0,
		BBox:       [4]float64{0, 0, 500, 0},
		Path: &entity.GlyphPath{
			Commands: []entity.PathCommand{},
			Bounds:   [4]float64{0, 0, 500, 0},
		},
	}

	if len(raw) == 0 {
		return glyph
	}

	decoder := NewCharStringDecoderWithSubrs(raw, subrs)
	commands, err := decoder.Decode()
	if err != nil {
		return glyph
	}

	glyph.Commands = commands
	glyph.Width = decoder.Width()
	glyph.LSB = decoder.LSB()
	if glyph.Width <= 0 {
		glyph.Width = 500
	}

	if glyph.LSB < 0 {
		glyph.BBox[0] = glyph.LSB
	}
	glyph.BBox[2] = glyph.Width
	path, pathErr := f.generatePath(commands, 1000)
	if pathErr == nil && len(path.Commands) > 0 {
		glyph.Path = path
	}
	if len(glyph.Path.Commands) == 0 {
		f.applyStandardFallbackToGlyph(glyph, name)
	}

	return glyph
}

func (f *Font) createFallbackGlyph(charCode uint32, name string) {
	glyph := &Glyph{
		Name:  name,
		Width: 500,
		LSB:   0,
		BBox:  [4]float64{0, 0, 500, 0},
		Path: &entity.GlyphPath{
			Commands: []entity.PathCommand{},
			Bounds:   [4]float64{0, 0, 500, 0},
		},
	}

	if fallback := f.resolveStandardFallbackGlyph(name); fallback != nil {
		f.applyResolvedStandardFallback(glyph, fallback)
	}

	f.glyphs[charCode] = glyph
	if name != "" {
		f.chars[name] = glyph
	}
}

func (f *Font) applyStandardFallbackToGlyph(glyph *Glyph, name string) {
	if glyph == nil {
		return
	}
	fallback := f.resolveStandardFallbackGlyph(name)
	if fallback == nil {
		return
	}
	f.applyResolvedStandardFallback(glyph, fallback)
}

func (f *Font) applyResolvedStandardFallback(glyph *Glyph, fallback *standardFallbackGlyph) {
	if glyph == nil || fallback == nil {
		return
	}
	if width, err := fallback.font.GetGlyphWidth(fallback.glyph); err == nil && width > 0 {
		glyph.Width = width
		glyph.BBox[2] = width
		glyph.Path.Bounds[2] = width
	}
	if path, err := fallback.font.RenderGlyph(fallback.glyph, 1000); err == nil && len(path.Commands) > 0 {
		glyph.Path = path
	}
	glyph.fallback = fallback
}

func (f *Font) resolveStandardFallbackGlyph(name string) *standardFallbackGlyph {
	if name == "" {
		return nil
	}

	font := f.standardFallbackFont()
	if font == nil {
		return nil
	}

	glyphID, ok := font.GlyphIDByName(name)
	if !ok {
		glyphID, ok = standardGlyphIDFromName(name)
	}
	if !ok {
		return nil
	}

	return &standardFallbackGlyph{
		font:  font,
		glyph: glyphID,
	}
}

func (f *Font) standardFallbackFont() *standardfont.StandardFont {
	fontName := stripType1SubsetPrefix(f.fontName)
	switch {
	case strings.HasPrefix(fontName, "SFTT"), strings.HasPrefix(fontName, "CMTT"):
		font, _ := standardfont.GetFont("Courier")
		return font
	case strings.HasPrefix(fontName, "NimbusSanL-Bold"), strings.HasPrefix(fontName, "SFBX"):
		font, _ := standardfont.GetFont("Helvetica-Bold")
		return font
	case strings.HasPrefix(fontName, "SFSX"):
		font, _ := standardfont.GetFont("Helvetica")
		return font
	default:
		font, _ := standardfont.GetFont("Times-Roman")
		return font
	}
}

func stripType1SubsetPrefix(name string) string {
	if idx := strings.IndexByte(name, '+'); idx >= 0 && idx+1 < len(name) {
		return name[idx+1:]
	}
	return name
}

func standardGlyphIDFromName(name string) (uint32, bool) {
	if len(name) == 1 {
		return uint32(name[0]), true
	}

	candidates := map[string]uint32{
		"space":       ' ',
		"exclam":      '!',
		"quotedbl":    '"',
		"numbersign":  '#',
		"dollar":      '$',
		"percent":     '%',
		"ampersand":   '&',
		"quotesingle": '\'',
		"parenleft":   '(',
		"parenright":  ')',
		"asterisk":    '*',
		"plus":        '+',
		"comma":       ',',
		"hyphen":      '-',
		"period":      '.',
		"slash":       '/',
		"colon":       ':',
		"semicolon":   ';',
		"question":    '?',
		"at":          '@',
		"backslash":   '\\',
		"underscore":  '_',
		"zero":        '0',
		"one":         '1',
		"two":         '2',
		"three":       '3',
		"four":        '4',
		"five":        '5',
		"six":         '6',
		"seven":       '7',
		"eight":       '8',
		"nine":        '9',
	}
	if glyph, ok := candidates[name]; ok {
		return glyph, true
	}
	return 0, false
}

// createMinimalGlyphs creates minimal glyph definitions for basic rendering.
func (f *Font) createMinimalGlyphs() {
	// Create a minimal glyph set for common characters
	// In a full implementation, these would be parsed from the CharStrings

	commonGlyphs := []struct {
		name     string
		width    float64
		charCode byte
	}{
		{name: "space", width: 250, charCode: 0x20},
		{name: "A", width: 667, charCode: 0x41},
		{name: "B", width: 667, charCode: 0x42},
		{name: "C", width: 722, charCode: 0x43},
		{name: "D", width: 722, charCode: 0x44},
		{name: "E", width: 667, charCode: 0x45},
		{name: "F", width: 611, charCode: 0x46},
		{name: "G", width: 778, charCode: 0x47},
		{name: "H", width: 778, charCode: 0x48},
		{name: "I", width: 389, charCode: 0x49},
		{name: "J", width: 500, charCode: 0x4A},
		{name: "K", width: 778, charCode: 0x4B},
		{name: "L", width: 667, charCode: 0x4C},
		{name: "M", width: 944, charCode: 0x4D},
		{name: "N", width: 722, charCode: 0x4E},
		{name: "O", width: 778, charCode: 0x4F},
		{name: "P", width: 611, charCode: 0x50},
		{name: "Q", width: 778, charCode: 0x51},
		{name: "R", width: 667, charCode: 0x52},
		{name: "S", width: 556, charCode: 0x53},
		{name: "T", width: 667, charCode: 0x54},
		{name: "U", width: 778, charCode: 0x55},
		{name: "V", width: 722, charCode: 0x56},
		{name: "W", width: 1000, charCode: 0x57},
		{name: "X", width: 667, charCode: 0x58},
		{name: "Y", width: 667, charCode: 0x59},
		{name: "Z", width: 611, charCode: 0x5A},
		{name: "a", width: 500, charCode: 0x61},
		{name: "b", width: 556, charCode: 0x62},
		{name: "c", width: 444, charCode: 0x63},
		{name: "d", width: 556, charCode: 0x64},
		{name: "e", width: 444, charCode: 0x65},
		{name: "f", width: 333, charCode: 0x66},
		{name: "g", width: 500, charCode: 0x67},
		{name: "h", width: 556, charCode: 0x68},
		{name: "i", width: 278, charCode: 0x69},
		{name: "j", width: 333, charCode: 0x6A},
		{name: "k", width: 556, charCode: 0x6B},
		{name: "l", width: 278, charCode: 0x6C},
		{name: "m", width: 833, charCode: 0x6D},
		{name: "n", width: 556, charCode: 0x6E},
		{name: "o", width: 500, charCode: 0x6F},
		{name: "p", width: 556, charCode: 0x70},
		{name: "q", width: 556, charCode: 0x71},
		{name: "r", width: 389, charCode: 0x72},
		{name: "s", width: 389, charCode: 0x73},
		{name: "t", width: 333, charCode: 0x74},
		{name: "u", width: 556, charCode: 0x75},
		{name: "v", width: 500, charCode: 0x76},
		{name: "w", width: 722, charCode: 0x77},
		{name: "x", width: 500, charCode: 0x78},
		{name: "y", width: 500, charCode: 0x79},
		{name: "z", width: 444, charCode: 0x7A},
	}

	for _, g := range commonGlyphs {
		glyph := &Glyph{
			Name:  g.name,
			Width: g.width,
			BBox:  [4]float64{0, 0, g.width, 0},
			LSB:   0,
			Path: &entity.GlyphPath{
				Commands: []entity.PathCommand{},
				Bounds:   [4]float64{0, 0, g.width, 0},
			},
		}

		f.chars[g.name] = glyph
		f.glyphs[uint32(g.charCode)] = glyph
	}
}

// extractMetrics extracts font metrics from the font data.
func (f *Font) extractMetrics() {
	// Calculate bounding box
	f.bbox = f.file.FontInfo.FontBBox

	// Estimate ascent/descent from bounding box
	f.ascent = f.bbox[3]
	f.descent = f.bbox[1]
	f.capHeight = f.ascent * 0.7 // Approximate
}

// CharCodeToGlyph maps a character code to a glyph ID.
func (f *Font) CharCodeToGlyph(code uint32) (uint32, error) {
	// Check if glyph exists
	if _, ok := f.glyphs[code]; ok {
		return code, nil
	}

	// Try to create a default glyph
	f.createDefaultGlyph(code)
	return code, nil
}

// GlyphName returns the name for a glyph ID.
func (f *Font) GlyphName(glyph uint32) string {
	if g, ok := f.glyphs[glyph]; ok {
		return g.Name
	}

	// Return notdef
	return ".notdef"
}

// ftNamedGlyphBase is a sentinel offset for glyph IDs obtained via FT_Get_Name_Index.
// Glyph IDs >= ftNamedGlyphBase are FreeType glyph indices, not char codes.
// This allows unencoded glyphs (e.g., "odieresis" not in OT1 encoding) to be
// looked up by name and rendered by index directly.
const ftNamedGlyphBase = uint32(0x80000000)

// GlyphIDByName resolves a glyph name to the font's glyph ID when available.
// For unencoded glyphs (present in CharStrings but not in the font's encoding),
// it falls back to FreeType's FT_Get_Name_Index and returns ftNamedGlyphBase|ftIndex.
func (f *Font) GlyphIDByName(name string) (uint32, bool) {
	if f == nil {
		return 0, false
	}
	glyph, ok := f.chars[name]
	if ok && glyph != nil {
		for glyphID, candidate := range f.glyphs {
			if candidate == glyph {
				return glyphID, true
			}
		}
	}
	// Glyph name not in encoding — try FreeType name lookup for unencoded glyphs.
	// Validate that the located glyph has a non-empty outline before returning it.
	// Math fonts (CMSY, CMMI) contain stub/empty CharStrings for many common glyph names
	// (e.g. "exclam", "ff") that FT_Get_Name_Index finds, but which render as empty bitmaps
	// because the actual symbol at that char code has a different name in the charmap.
	if ftcgo.IsAvailable() {
		for _, fontData := range f.freeTypeSourceData() {
			if ftIdx, found := ftcgo.GetGlyphIndexByName(fontData, name); found {
				buf, bw, bh, _, _, err := ftcgo.RenderGlyphBitmapByIndex(fontData, ftIdx, 12.0, 72)
				if err == nil && bw > 0 && bh > 0 && len(buf) > 0 {
					return ftNamedGlyphBase | ftIdx, true
				}
			}
		}
	}
	return 0, false
}

// EncodingName returns the font program encoding name for the given code.
func (f *Font) EncodingName(code byte) string {
	if f == nil {
		return ""
	}
	return f.encoding[code]
}

// GetGlyphWidth returns the width of a glyph in font units.
func (f *Font) GetGlyphWidth(glyph uint32) (float64, error) {
	if g, ok := f.glyphs[glyph]; ok {
		return g.Width, nil
	}

	// Return default width
	return 500, nil
}

// GetBoundingBox returns the bounding box of the font.
func (f *Font) GetBoundingBox() (float64, float64, float64, float64) {
	return f.bbox[0], f.bbox[1], f.bbox[2], f.bbox[3]
}

// RenderGlyph renders a glyph to a path.
func (f *Font) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	// For glyphs found via FT_Get_Name_Index (unencoded glyphs), use RenderGlyphByIndex.
	if glyph >= ftNamedGlyphBase {
		ftIdx := glyph - ftNamedGlyphBase
		if ftcgo.IsAvailable() {
			for _, fontData := range f.freeTypeSourceData() {
				path, err := ftcgo.RenderGlyphByIndex(fontData, ftIdx, size, 72)
				if err == nil && len(path.Commands) > 0 {
					return path, nil
				}
			}
		}
		return nil, fmt.Errorf("type1: unencoded glyph %d not renderable", ftIdx)
	}

	// Try FreeType rendering first using the original Type1 bytes when available.
	// This avoids outline drift introduced by the synthetic Type1->OTF conversion.
	if ftcgo.IsAvailable() {
		for _, fontData := range f.freeTypeSourceData() {
			path, err := ftcgo.RenderGlyph(fontData, glyph, size, 72)
			if err == nil && len(path.Commands) > 0 {
				return path, nil
			}
		}
	}

	// Try sfnt-based rendering (same coordinate system as standard fonts)
	if f.sfntFont != nil {
		path, err := f.renderGlyphViaSfnt(glyph, size)
		if err == nil && len(path.Commands) > 0 {
			return path, nil
		}
	}

	if g, ok := f.glyphs[glyph]; ok {
		// Fallback: generate path from decoded commands
		path, err := f.generatePath(g.Commands, size)
		if err != nil {
			return nil, err
		}
		if len(path.Commands) == 0 && g.fallback != nil {
			return g.fallback.font.RenderGlyph(g.fallback.glyph, size)
		}

		g.Path = path
		return path, nil
	}

	return &entity.GlyphPath{
		Commands: []entity.PathCommand{},
		Bounds:   [4]float64{0, 0, 500, 0},
	}, nil
}

// buildSfntFont builds an OTF binary from the parsed Type1 data and loads it via sfnt.
func (f *Font) buildSfntFont() {
	if len(f.glyphs) == 0 {
		return
	}

	otfData, err := BuildOTF(f.fontName, f.glyphs, f.encoding, f.fontInfo)
	if err != nil || len(otfData) == 0 {
		return
	}

	f.otfData = otfData

	sfntFont, err := sfnt.Parse(otfData)
	if err != nil {
		return
	}

	f.sfntFont = sfntFont
}

func (f *Font) buildFreeTypeNameIndex() {
	if f == nil || !ftcgo.IsAvailable() {
		return
	}
	sources := f.freeTypeSourceData()
	if len(sources) == 0 {
		return
	}

	indexByCode := make(map[uint32]freeTypeGlyphIndex, len(f.glyphs))
	for code := range f.glyphs {
		name := f.GlyphName(code)
		if name == "" || name == ".notdef" {
			continue
		}
		for sourceIdx, fontData := range sources {
			ftIdx, ok := ftcgo.GetGlyphIndexByName(fontData, name)
			if !ok || ftIdx == 0 {
				continue
			}
			indexByCode[code] = freeTypeGlyphIndex{
				source: sourceIdx,
				index:  ftIdx,
			}
			break
		}
	}
	if len(indexByCode) > 0 {
		f.ftGlyphIndexByCode = indexByCode
	}
}

func (f *Font) freeTypeNameIndexForGlyph(glyph uint32) ([]byte, uint32, bool) {
	if f == nil || f.ftGlyphIndexByCode == nil {
		return nil, 0, false
	}
	idx, ok := f.ftGlyphIndexByCode[glyph]
	if !ok {
		return nil, 0, false
	}
	sources := f.freeTypeSourceData()
	if idx.source < 0 || idx.source >= len(sources) {
		return nil, 0, false
	}
	return sources[idx.source], idx.index, true
}

// OTFData returns the generated OTF binary for external renderers (e.g., FreeType).
func (f *Font) OTFData() []byte {
	return f.otfData
}

// RenderGlyphBitmap renders a glyph to a grayscale bitmap via FreeType.
// Implements entity.BitmapGlyphRenderer.
func (f *Font) RenderGlyphBitmap(glyph uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error) {
	if !ftcgo.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType bitmap rendering not available")
	}
	// For FreeType-indexed glyphs (unencoded), use RenderGlyphBitmapByIndex.
	if glyph >= ftNamedGlyphBase {
		ftIdx := glyph - ftNamedGlyphBase
		for _, fontData := range f.freeTypeSourceData() {
			buf, bw, bh, bleft, btop, err := ftcgo.RenderGlyphBitmapByIndex(fontData, ftIdx, sizePt, dpi)
			if err == nil {
				return buf, bw, bh, bleft, btop, nil
			}
		}
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType bitmap rendering failed for index %d", ftIdx)
	}
	for _, fontData := range f.freeTypeSourceData() {
		buf, bw, bh, bleft, btop, err := ftcgo.RenderGlyphBitmap(fontData, glyph, sizePt, dpi)
		if err == nil {
			return buf, bw, bh, bleft, btop, nil
		}
	}
	return nil, 0, 0, 0, 0, fmt.Errorf("FreeType bitmap rendering failed")
}

// RenderGlyphBitmapPhased renders a glyph bitmap with sub-pixel phase for accurate antialiasing.
// Implements entity.BitmapGlyphRendererPhased.
func (f *Font) RenderGlyphBitmapPhased(glyph uint32, sizePt float64, dpi int, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if !ftcgo.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType bitmap rendering not available")
	}
	if glyph >= ftNamedGlyphBase {
		ftIdx := glyph - ftNamedGlyphBase
		for _, fontData := range f.freeTypeSourceData() {
			buf, bw, bh, bleft, btop, err := ftcgo.RenderGlyphBitmapByIndexPhased(fontData, ftIdx, sizePt, dpi, phaseX, phaseY)
			if err == nil {
				return buf, bw, bh, bleft, btop, nil
			}
		}
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType phased bitmap rendering failed for index %d", ftIdx)
	}
	if fontData, ftIdx, ok := f.freeTypeNameIndexForGlyph(glyph); ok {
		buf, bw, bh, bleft, btop, err := ftcgo.RenderGlyphBitmapByIndexPhased(fontData, ftIdx, sizePt, dpi, phaseX, phaseY)
		if err == nil {
			return buf, bw, bh, bleft, btop, nil
		}
	}
	for _, fontData := range f.freeTypeSourceData() {
		buf, bw, bh, bleft, btop, err := ftcgo.RenderGlyphBitmapPhased(fontData, glyph, sizePt, dpi, phaseX, phaseY)
		if err == nil {
			return buf, bw, bh, bleft, btop, nil
		}
	}
	return nil, 0, 0, 0, 0, fmt.Errorf("FreeType phased bitmap rendering failed")
}

// RenderGlyphBitmapTransformedPhased renders a glyph bitmap with axis-aligned
// transform scaling and phase matching Poppler's SplashFTFont path.
func (f *Font) RenderGlyphBitmapTransformedPhased(glyph uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if !ftcgo.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType bitmap rendering not available")
	}
	if glyph >= ftNamedGlyphBase {
		ftIdx := glyph - ftNamedGlyphBase
		for _, fontData := range f.freeTypeSourceData() {
			buf, bw, bh, bleft, btop, err := ftcgo.RenderGlyphBitmapByIndexTransformedPhased(fontData, ftIdx, sizePt, scaleX, scaleY, phaseX, phaseY)
			if err == nil {
				return buf, bw, bh, bleft, btop, nil
			}
		}
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType transformed bitmap rendering failed for index %d", ftIdx)
	}
	if fontData, ftIdx, ok := f.freeTypeNameIndexForGlyph(glyph); ok {
		buf, bw, bh, bleft, btop, err := ftcgo.RenderGlyphBitmapByIndexTransformedPhased(fontData, ftIdx, sizePt, scaleX, scaleY, phaseX, phaseY)
		if err == nil {
			return buf, bw, bh, bleft, btop, nil
		}
	}
	for _, fontData := range f.freeTypeSourceData() {
		buf, bw, bh, bleft, btop, err := ftcgo.RenderGlyphBitmapTransformedPhased(fontData, glyph, sizePt, scaleX, scaleY, phaseX, phaseY)
		if err == nil {
			return buf, bw, bh, bleft, btop, nil
		}
	}
	return nil, 0, 0, 0, 0, fmt.Errorf("FreeType transformed bitmap rendering failed")
}

// RenderGlyphBitmapMatrixPhased renders a glyph bitmap with Poppler's full 2x2
// FreeType transform matrix and phase.
func (f *Font) RenderGlyphBitmapMatrixPhased(glyph uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if !ftcgo.IsAvailable() {
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType bitmap rendering not available")
	}
	if glyph >= ftNamedGlyphBase {
		ftIdx := glyph - ftNamedGlyphBase
		for _, fontData := range f.freeTypeSourceData() {
			buf, bw, bh, bleft, btop, err := ftcgo.RenderGlyphBitmapByIndexMatrixPhased(fontData, ftIdx, sizePt, matrix, phaseX, phaseY)
			if err == nil {
				return buf, bw, bh, bleft, btop, nil
			}
		}
		return nil, 0, 0, 0, 0, fmt.Errorf("FreeType matrix bitmap rendering failed for index %d", ftIdx)
	}
	if fontData, ftIdx, ok := f.freeTypeNameIndexForGlyph(glyph); ok {
		buf, bw, bh, bleft, btop, err := ftcgo.RenderGlyphBitmapByIndexMatrixPhased(fontData, ftIdx, sizePt, matrix, phaseX, phaseY)
		if err == nil {
			return buf, bw, bh, bleft, btop, nil
		}
	}
	for _, fontData := range f.freeTypeSourceData() {
		buf, bw, bh, bleft, btop, err := ftcgo.RenderGlyphBitmapMatrixPhased(fontData, glyph, sizePt, matrix, phaseX, phaseY)
		if err == nil {
			return buf, bw, bh, bleft, btop, nil
		}
	}
	return nil, 0, 0, 0, 0, fmt.Errorf("FreeType matrix bitmap rendering failed")
}

func (f *Font) freeTypeSourceData() [][]byte {
	if f == nil {
		return nil
	}

	switch strings.TrimSpace(os.Getenv("PDF_DEBUG_TYPE1_FT_SOURCE")) {
	case "raw":
		if f.file != nil && len(f.file.rawData) > 0 {
			return [][]byte{f.file.rawData}
		}
		return nil
	case "otf":
		if len(f.otfData) > 0 {
			return [][]byte{f.otfData}
		}
		return nil
	}

	sources := make([][]byte, 0, 2)
	if f.file != nil && len(f.file.rawData) > 0 {
		sources = append(sources, f.file.rawData)
	}
	if len(f.otfData) > 0 {
		sources = append(sources, f.otfData)
	}
	return sources
}

// renderGlyphViaSfnt renders a glyph using the sfnt library (same path as standard fonts).
func (f *Font) renderGlyphViaSfnt(glyph uint32, size float64) (*entity.GlyphPath, error) {
	glyphIndex, err := f.sfntFont.GlyphIndex(nil, rune(glyph))
	if err != nil || glyphIndex == 0 {
		return nil, fmt.Errorf("glyph not found: %d", glyph)
	}

	segments, err := f.sfntFont.LoadGlyph(nil, glyphIndex, fixed.I(int(f.sfntFont.UnitsPerEm())), nil)
	if err != nil {
		return nil, err
	}

	scale := size / float64(f.sfntFont.UnitsPerEm())
	commands := make([]entity.PathCommand, 0, len(segments))
	const pointScale = 1.0 / 64.0
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64
	hasPoint := false

	updateBounds := func(x, y float64) {
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
		if y < minY {
			minY = y
		}
		if y > maxY {
			maxY = y
		}
		hasPoint = true
	}

	for _, seg := range segments {
		switch seg.Op {
		case sfnt.SegmentOpMoveTo:
			x := float64(seg.Args[0].X) * pointScale * scale
			y := float64(seg.Args[0].Y) * pointScale * scale
			commands = append(commands, &entity.PathMoveTo{X: x, Y: y})
			updateBounds(x, y)
		case sfnt.SegmentOpLineTo:
			x := float64(seg.Args[0].X) * pointScale * scale
			y := float64(seg.Args[0].Y) * pointScale * scale
			commands = append(commands, &entity.PathLineTo{X: x, Y: y})
			updateBounds(x, y)
		case sfnt.SegmentOpQuadTo:
			c1X := float64(seg.Args[0].X) * pointScale * scale
			c1Y := float64(seg.Args[0].Y) * pointScale * scale
			x := float64(seg.Args[1].X) * pointScale * scale
			y := float64(seg.Args[1].Y) * pointScale * scale
			commands = append(commands, &entity.PathCurveTo{
				X1: c1X, Y1: c1Y, X2: c1X, Y2: c1Y, X3: x, Y3: y,
			})
			updateBounds(c1X, c1Y)
			updateBounds(x, y)
		case sfnt.SegmentOpCubeTo:
			c1X := float64(seg.Args[0].X) * pointScale * scale
			c1Y := float64(seg.Args[0].Y) * pointScale * scale
			c2X := float64(seg.Args[1].X) * pointScale * scale
			c2Y := float64(seg.Args[1].Y) * pointScale * scale
			x := float64(seg.Args[2].X) * pointScale * scale
			y := float64(seg.Args[2].Y) * pointScale * scale
			commands = append(commands, &entity.PathCurveTo{
				X1: c1X, Y1: c1Y, X2: c2X, Y2: c2Y, X3: x, Y3: y,
			})
			updateBounds(c1X, c1Y)
			updateBounds(c2X, c2Y)
			updateBounds(x, y)
		}
	}

	if !hasPoint || len(commands) == 0 {
		return nil, fmt.Errorf("empty glyph")
	}

	return &entity.GlyphPath{
		Commands: commands,
		Bounds:   [4]float64{minX, minY, maxX, maxY},
	}, nil
}

// generatePath generates a rendering path from CharString commands.
// Type1 font coordinates have Y going up, but the renderer expects Y going
// down (matching TrueType/sfnt convention where baseline is at Y=0 and
// ascenders go to negative Y). We negate Y to match.
func (f *Font) generatePath(commands []Command, size float64) (*entity.GlyphPath, error) {
	path := &entity.GlyphPath{
		Commands: make([]entity.PathCommand, 0),
		Bounds:   [4]float64{0, 0, 0, 0},
	}

	scale := size / 1000.0 // Type1 fonts typically use 1000 units per em

	x, y := 0.0, 0.0
	pendingMoveTo := false // defer moveto until actual drawing command
	for _, cmd := range commands {
		switch cmd.Type {
		case CmdRMoveto:
			if len(cmd.Args) >= 2 {
				x += cmd.Args[0] * scale
				y += cmd.Args[1] * scale
				pendingMoveTo = true
			}

		case CmdRLineto:
			if len(cmd.Args) >= 2 {
				if pendingMoveTo {
					path.Commands = append(path.Commands, &entity.PathMoveTo{X: x, Y: -y})
					pendingMoveTo = false
				}
				x += cmd.Args[0] * scale
				y += cmd.Args[1] * scale
				path.Commands = append(path.Commands, &entity.PathLineTo{X: x, Y: -y})
			}

		case CmdRRCurveto:
			if len(cmd.Args) >= 6 {
				if pendingMoveTo {
					path.Commands = append(path.Commands, &entity.PathMoveTo{X: x, Y: -y})
					pendingMoveTo = false
				}
				x1 := x + cmd.Args[0]*scale
				y1 := y + cmd.Args[1]*scale
				x2 := x1 + cmd.Args[2]*scale
				y2 := y1 + cmd.Args[3]*scale
				x3 := x2 + cmd.Args[4]*scale
				y3 := y2 + cmd.Args[5]*scale
				x = x3
				y = y3

				path.Commands = append(path.Commands, &entity.PathCurveTo{
					X1: x1, Y1: -y1,
					X2: x2, Y2: -y2,
					X3: x3, Y3: -y3,
				})
			}
		case CmdClosePath:
			if pendingMoveTo {
				// Emit moveto before close so the subpath has a proper start
				path.Commands = append(path.Commands, &entity.PathMoveTo{X: x, Y: -y})
				pendingMoveTo = false
			}
			path.Commands = append(path.Commands, &entity.PathClose{})

		case CmdEndChar:
			return path, nil

		case CmdHStem, CmdVStem, CmdHStemHM, CmdVStemHM:
			// Hint commands - ignore for rendering
			continue

		default:
			// Other commands not implemented yet
		}
	}

	return path, nil
}

// createDefaultGlyph creates a default glyph for missing characters.
func (f *Font) createDefaultGlyph(code uint32) {
	width := 500.0

	glyph := &Glyph{
		Name:  fmt.Sprintf("glyph%d", code),
		Width: width,
		BBox:  [4]float64{0, 0, width, 0},
		LSB:   0,
		Path: &entity.GlyphPath{
			Commands: []entity.PathCommand{},
			Bounds:   [4]float64{0, 0, width, 0},
		},
	}

	f.glyphs[code] = glyph
}

// IsCIDFont returns false for Type1 fonts.
func (f *Font) IsCIDFont() bool {
	return false
}

// IsSymbolic returns whether this is a symbolic font.
func (f *Font) IsSymbolic() bool {
	return f.file.FontInfo.IsFixedPitch
}

// UnitsPerEm returns the units per em value (Type1 fonts use 1000).
func (f *Font) UnitsPerEm() uint16 {
	return 1000
}

// Name returns the font name.
func (f *Font) Name() string {
	return f.fontName
}

// FontData returns original Type1 font bytes when available.
func (f *Font) FontData() []byte {
	if f.file == nil {
		return nil
	}
	return f.file.RawData()
}

// GetAdvanceWidth returns the advance width for a character.
func (f *Font) GetAdvanceWidth(charCode uint32, size float64) (float64, error) {
	glyph, err := f.CharCodeToGlyph(charCode)
	if err != nil {
		return 0, err
	}

	width, err := f.GetGlyphWidth(glyph)
	if err != nil {
		return 0, err
	}

	// Scale to requested size
	upem := float64(f.UnitsPerEm())
	if upem == 0 {
		upem = 1000
	}
	return (width * size) / upem, nil
}

// HasGlyph returns true if the font contains a glyph for the character.
func (f *Font) HasGlyph(charCode uint32) bool {
	_, ok := f.glyphs[charCode]
	return ok
}

// GetFontDescriptor returns the font descriptor.
func (f *Font) GetFontDescriptor() *entity.FontDescriptor {
	return &entity.FontDescriptor{
		FontName:     f.fontName,
		FontFamily:   f.fontName,
		Flags:        0,
		ItalicAngle:  f.file.FontInfo.ItalicAngle,
		Ascent:       f.ascent,
		Descent:      f.descent,
		CapHeight:    f.capHeight,
		StemV:        80, // Default value
		MissingWidth: 500,
	}
}

// getStandardEncoding returns the standard Type1 encoding.
func getStandardEncoding() map[byte]string {
	return map[byte]string{
		0x20: "space",
		0x21: "exclam",
		0x22: "quotedbl",
		0x23: "numbersign",
		0x24: "dollar",
		0x25: "percent",
		0x26: "ampersand",
		0x27: "quoteright",
		0x28: "parenleft",
		0x29: "parenright",
		0x2A: "asterisk",
		0x2B: "plus",
		0x2C: "comma",
		0x2D: "hyphen",
		0x2E: "period",
		0x2F: "slash",
		0x30: "zero",
		0x31: "one",
		0x32: "two",
		0x33: "three",
		0x34: "four",
		0x35: "five",
		0x36: "six",
		0x37: "seven",
		0x38: "eight",
		0x39: "nine",
		0x3A: "colon",
		0x3B: "semicolon",
		0x3C: "less",
		0x3D: "equal",
		0x3E: "greater",
		0x3F: "question",
		0x40: "at",
		0x41: "A",
		0x42: "B",
		0x43: "C",
		0x44: "D",
		0x45: "E",
		0x46: "F",
		0x47: "G",
		0x48: "H",
		0x49: "I",
		0x4A: "J",
		0x4B: "K",
		0x4C: "L",
		0x4D: "M",
		0x4E: "N",
		0x4F: "O",
		0x50: "P",
		0x51: "Q",
		0x52: "R",
		0x53: "S",
		0x54: "T",
		0x55: "U",
		0x56: "V",
		0x57: "W",
		0x58: "X",
		0x59: "Y",
		0x5A: "Z",
		0x5B: "bracketleft",
		0x5C: "backslash",
		0x5D: "bracketright",
		0x5E: "asciicircum",
		0x5F: "underscore",
		0x60: "grave",
		0x61: "a",
		0x62: "b",
		0x63: "c",
		0x64: "d",
		0x65: "e",
		0x66: "f",
		0x67: "g",
		0x68: "h",
		0x69: "i",
		0x6A: "j",
		0x6B: "k",
		0x6C: "l",
		0x6D: "m",
		0x6E: "n",
		0x6F: "o",
		0x70: "p",
		0x71: "q",
		0x72: "r",
		0x73: "s",
		0x74: "t",
		0x75: "u",
		0x76: "v",
		0x77: "w",
		0x78: "x",
		0x79: "y",
		0x7A: "z",
		0x7B: "braceleft",
		0x7C: "bar",
		0x7D: "braceright",
		0x7E: "asciitilde",
	}
}

func getMacRomanEncoding() map[byte]string {
	// MacRoman and StandardEncoding share the same base glyph names for the subset
	// required by this project. A full MacRoman table can be added later if needed.
	return getStandardEncoding()
}
