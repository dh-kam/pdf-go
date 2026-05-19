// Package standard provides standard PDF fonts.
//
//revive:disable:exported
package standard

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/gomonoitalic"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/gofont/gosmallcaps"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
	ftcgo "github.com/dh-kam/pdf-go/internal/infrastructure/font/freetype"
)

// Standard14 contains the 14 standard PDF fonts.
var Standard14 = map[string]*StandardFont{
	"Times-Roman":           newStandardFont("Times New Roman", FontNormal, "NimbusRoman-Regular.afm", getWidths("TimesRoman"), goregular.TTF, readURWBase35Font("NimbusRoman-Regular.otf"), [4]float64{-168, -218, 1000, 898}),
	"Times-Bold":            newStandardFont("Times New Roman", FontBold, "NimbusRoman-Bold.afm", getWidths("TimesBold"), gobold.TTF, readURWBase35Font("NimbusRoman-Bold.otf"), [4]float64{-168, -218, 1000, 935}),
	"Times-Italic":          newStandardFont("Times New Roman", FontItalic, "NimbusRoman-Italic.afm", getWidths("TimesItalic"), goitalic.TTF, readURWBase35Font("NimbusRoman-Italic.otf"), [4]float64{-169, -217, 1010, 883}),
	"Times-BoldItalic":      newStandardFont("Times New Roman", FontBoldItalic, "NimbusRoman-BoldItalic.afm", getWidths("TimesBoldItalic"), gobolditalic.TTF, readURWBase35Font("NimbusRoman-BoldItalic.otf"), [4]float64{-200, -218, 996, 921}),
	"Helvetica":             newStandardFont("Helvetica", FontNormal, "NimbusSans-Regular.afm", getWidths("Helvetica"), gomedium.TTF, readURWBase35Font("NimbusSans-Regular.otf"), [4]float64{-166, -225, 1000, 931}),
	"Helvetica-Bold":        newStandardFont("Helvetica Bold", FontBold, "NimbusSans-Bold.afm", getWidths("HelveticaBold"), gomedium.TTF, readURWBase35Font("NimbusSans-Bold.otf"), [4]float64{-170, -228, 1003, 962}),
	"Helvetica-Oblique":     newStandardFont("Helvetica Oblique", FontItalic, "NimbusSans-Italic.afm", getWidths("HelveticaOblique"), goitalic.TTF, readURWBase35Font("NimbusSans-Italic.otf"), [4]float64{-170, -225, 1116, 931}),
	"Helvetica-BoldOblique": newStandardFont("Helvetica Bold Oblique", FontBoldItalic, "NimbusSans-BoldItalic.afm", getWidths("HelveticaBoldOblique"), gobolditalic.TTF, readURWBase35Font("NimbusSans-BoldItalic.otf"), [4]float64{-174, -228, 1114, 962}),
	"Courier":               newStandardFont("Courier", FontNormal, "NimbusMonoPS-Regular.afm", getWidths("Courier"), gomono.TTF, readURWBase35Font("NimbusMonoPS-Regular.otf"), [4]float64{-23, -250, 715, 805}),
	"Courier-Bold":          newStandardFont("Courier Bold", FontBold, "NimbusMonoPS-Bold.afm", getWidths("CourierBold"), gomonobold.TTF, readURWBase35Font("NimbusMonoPS-Bold.otf"), [4]float64{-113, -250, 749, 801}),
	"Courier-Oblique":       newStandardFont("Courier Oblique", FontItalic, "NimbusMonoPS-Italic.afm", getWidths("CourierOblique"), gomonoitalic.TTF, readURWBase35Font("NimbusMonoPS-Italic.otf"), [4]float64{-27, -250, 849, 805}),
	"Courier-BoldOblique":   newStandardFont("Courier Bold Oblique", FontBoldItalic, "NimbusMonoPS-BoldItalic.afm", getWidths("CourierBoldOblique"), gomonoitalic.TTF, readURWBase35Font("NimbusMonoPS-BoldItalic.otf"), [4]float64{-57, -250, 869, 801}),
	"Symbol":                newStandardFont("Symbol", FontNormal, "StandardSymbolsPS.afm", getWidths("Symbol"), gosmallcaps.TTF, nil, [4]float64{-180, -293, 1090, 1010}),
	"ZapfDingbats":          newStandardFont("Zapf Dingbats", FontNormal, "D050000L.afm", getWidths("ZapfDingbats"), gosmallcaps.TTF, nil, [4]float64{-1, -143, 981, 820}),
}

// FontWeight represents font weight.
type FontWeight int

const (
	FontNormal FontWeight = iota
	FontBold
	FontItalic
	FontBoldItalic
)

// StandardFont represents a standard PDF font.
type StandardFont struct {
	name        string
	sfntData    []byte
	rasterData  []byte
	widths      []float64
	boundingBox entity.BoundingBox
	weight      FontWeight
	syncOnce    sync.Once
	sfntFont    *sfnt.Font
	sfntErr     error
}

const urwBase35Dir = "/usr/share/fonts/opentype/urw-base35"
const urwBase35AFMDir = "/usr/share/fonts/type1/urw-base35"
const standardWidthScale = 2048.0 / 1000.0

func newStandardFont(name string, weight FontWeight, afmFileName string, fallbackWidths []float64, sfntData, rasterData []byte, fallbackBBox [4]float64) *StandardFont {
	return &StandardFont{
		name:        name,
		weight:      weight,
		widths:      readURWAFMWidths(afmFileName, fallbackWidths),
		boundingBox: readURWAFMFontBBox(afmFileName, fallbackBBox),
		sfntData:    sfntData,
		rasterData:  rasterData,
	}
}

func readURWBase35Font(fileName string) []byte {
	data, err := os.ReadFile(urwBase35Dir + "/" + fileName)
	if err != nil {
		return nil
	}
	return data
}

func readURWAFMWidths(fileName string, fallback []float64) []float64 {
	widths := cloneWidths(fallback)
	data, err := os.ReadFile(urwBase35AFMDir + "/" + fileName)
	if err != nil {
		return widths
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 || fields[0] != "C" || fields[2] != ";" || fields[3] != "WX" {
			continue
		}
		code, err := strconv.Atoi(fields[1])
		if err != nil || code < 0 || code >= len(widths) {
			continue
		}
		width, err := strconv.ParseFloat(fields[4], 64)
		if err != nil {
			continue
		}
		widths[code] = width * standardWidthScale
	}
	return widths
}

func readURWAFMFontBBox(fileName string, fallback [4]float64) entity.BoundingBox {
	bbox := fallback
	data, err := os.ReadFile(urwBase35AFMDir + "/" + fileName)
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			fields := strings.Fields(line)
			if len(fields) != 5 || fields[0] != "FontBBox" {
				continue
			}
			parsed, ok := parseAFMFontBBox(fields[1:])
			if ok {
				bbox = parsed
			}
			break
		}
	}
	return scaleStandardFontBBox(bbox)
}

func parseAFMFontBBox(fields []string) ([4]float64, bool) {
	if len(fields) != 4 {
		return [4]float64{}, false
	}
	var bbox [4]float64
	for i, field := range fields {
		value, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return [4]float64{}, false
		}
		bbox[i] = value
	}
	return bbox, true
}

func scaleStandardFontBBox(bbox [4]float64) entity.BoundingBox {
	return entity.BoundingBox{
		XMin: bbox[0] * standardWidthScale,
		YMin: bbox[1] * standardWidthScale,
		XMax: bbox[2] * standardWidthScale,
		YMax: bbox[3] * standardWidthScale,
	}
}

func cloneWidths(widths []float64) []float64 {
	cloned := make([]float64, len(widths))
	copy(cloned, widths)
	return cloned
}

// GetFont retrieves a standard font by name.
func GetFont(name string) (*StandardFont, bool) {
	font, ok := Standard14[name]
	return font, ok
}

// GetStandard14Names returns all 14 standard font names.
func GetStandard14Names() []string {
	return []string{
		"Times-Roman",
		"Times-Bold",
		"Times-Italic",
		"Times-BoldItalic",
		"Helvetica",
		"Helvetica-Bold",
		"Helvetica-Oblique",
		"Helvetica-BoldOblique",
		"Courier",
		"Courier-Bold",
		"Courier-Oblique",
		"Courier-BoldOblique",
		"Symbol",
		"ZapfDingbats",
	}
}

// Name returns the font name.
func (f *StandardFont) Name() string {
	return f.name
}

// IsCIDFont returns false for standard fonts.
func (f *StandardFont) IsCIDFont() bool {
	return false
}

// IsSymbolic returns true for Symbol and ZapfDingbats.
func (f *StandardFont) IsSymbolic() bool {
	return f.name == "Symbol" || f.name == "Zapf Dingbats"
}

// SourceExactGlyphBlend reports whether glyphs come from Poppler's Standard14
// substitute path and can use Splash's exact truncating AA color blend.
func (f *StandardFont) SourceExactGlyphBlend() bool {
	return len(f.rasterData) > 0
}

// UnitsPerEm returns the units per em (typically 1000 or 2048).
func (f *StandardFont) UnitsPerEm() uint16 {
	sfntFont, err := f.loadFont()
	if err == nil && sfntFont != nil {
		if units := sfntFont.UnitsPerEm(); units > 0 {
			return uint16(units)
		}
	}

	return 1000
}

// CharCodeToGlyph maps a character code to a glyph ID.
func (f *StandardFont) CharCodeToGlyph(code uint32) (uint32, error) {
	if code > 0x10FFFF {
		return 0, &errors.OutOfRangeError{Code: code}
	}
	return code, nil
}

// GlyphName returns the glyph name for a glyph ID.
func (f *StandardFont) GlyphName(glyph uint32) string {
	// Standard fonts use simple glyph names
	if glyph > 255 && glyph <= 0x10FFFF {
		return string(rune(glyph))
	}
	return getGlyphName(glyph)
}

// GlyphIDByName resolves a small set of Adobe glyph names to glyph IDs.
func (f *StandardFont) GlyphIDByName(name string) (uint32, bool) {
	switch name {
	case "gamma":
		return uint32('γ'), true
	case "kappa":
		return uint32('κ'), true
	case "ff", "fi", "fl", "ffi":
		return uint32('f'), true
	}

	if glyph, ok := standardGlyphByName[name]; ok {
		return glyph, true
	}
	return 0, false
}

// GetGlyphWidth returns the width of a glyph.
func (f *StandardFont) GetGlyphWidth(glyph uint32) (float64, error) {
	if glyph < uint32(len(f.widths)) {
		return f.widths[glyph], nil
	}

	sfntFont, err := f.loadFont()
	if err == nil {
		glyphIndex, err := f.glyphIndex(glyph)
		if err == nil && glyphIndex != 0 {
			advance, err := sfntFont.GlyphAdvance(nil, glyphIndex, fixed.I(int(sfntFont.UnitsPerEm())), font.HintingNone)
			if err == nil && advance > 0 {
				return float64(advance) / 64.0, nil
			}
		}
	}

	if glyph >= 256 {
		return 0, &errors.OutOfRangeError{Code: glyph}
	}
	return f.widths[glyph], nil
}

// GetBoundingBox returns the font bounding box.
func (f *StandardFont) GetBoundingBox() (float64, float64, float64, float64) {
	return f.boundingBox.XMin, f.boundingBox.YMin,
		f.boundingBox.XMax, f.boundingBox.YMax
}

// RenderGlyph renders a glyph at the given size.
func (f *StandardFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	sfntFont, err := f.loadFont()
	if err != nil {
		return nil, err
	}

	glyphIndex, err := f.glyphIndex(glyph)
	if err != nil {
		return nil, err
	}
	if glyphIndex == 0 {
		return nil, &errors.OutOfRangeError{Code: glyph}
	}

	segments, err := sfntFont.LoadGlyph(nil, glyphIndex, fixed.I(int(sfntFont.UnitsPerEm())), nil)
	if err != nil {
		return nil, err
	}

	scale := size / float64(sfntFont.UnitsPerEm())
	commands := make([]entity.PathCommand, 0, len(segments))
	const pointScale = 1.0 / 64.0
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64
	hasPoint := false

	for _, seg := range segments {
		switch seg.Op {
		case sfnt.SegmentOpMoveTo:
			x := float64(seg.Args[0].X) * pointScale * scale
			y := float64(seg.Args[0].Y) * pointScale * scale
			commands = append(commands, &entity.PathMoveTo{X: x, Y: y})
			hasPoint = true
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
		case sfnt.SegmentOpLineTo:
			x := float64(seg.Args[0].X) * pointScale * scale
			y := float64(seg.Args[0].Y) * pointScale * scale
			commands = append(commands, &entity.PathLineTo{X: x, Y: y})
			hasPoint = true
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
		case sfnt.SegmentOpQuadTo:
			c1X := float64(seg.Args[0].X) * pointScale * scale
			c1Y := float64(seg.Args[0].Y) * pointScale * scale
			x := float64(seg.Args[1].X) * pointScale * scale
			y := float64(seg.Args[1].Y) * pointScale * scale
			commands = append(commands, &entity.PathCurveTo{
				X1: c1X, Y1: c1Y, X2: c1X, Y2: c1Y, X3: x, Y3: y,
			})
			hasPoint = true
			for _, xVal := range []float64{c1X, x} {
				if xVal < minX {
					minX = xVal
				}
				if xVal > maxX {
					maxX = xVal
				}
			}
			for _, yVal := range []float64{c1Y, y} {
				if yVal < minY {
					minY = yVal
				}
				if yVal > maxY {
					maxY = yVal
				}
			}
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
			hasPoint = true
			for _, xVal := range []float64{c1X, c2X, x} {
				if xVal < minX {
					minX = xVal
				}
				if xVal > maxX {
					maxX = xVal
				}
			}
			for _, yVal := range []float64{c1Y, c2Y, y} {
				if yVal < minY {
					minY = yVal
				}
				if yVal > maxY {
					maxY = yVal
				}
			}
		}
	}

	if len(commands) == 0 || !hasPoint {
		return nil, &errors.OutOfRangeError{Code: glyph}
	}

	path := &entity.GlyphPath{
		Commands: commands,
		Bounds:   [4]float64{minX, minY, maxX, maxY},
	}
	return path, nil
}

// RenderGlyphBitmap renders a standard-font glyph through FreeType.
func (f *StandardFont) RenderGlyphBitmap(glyph uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error) {
	data, err := f.rasterFontData()
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	return ftcgo.RenderGlyphBitmap(data, glyph, sizePt, dpi)
}

// RenderGlyphBitmapPhased renders a standard-font glyph through FreeType with sub-pixel phase.
func (f *StandardFont) RenderGlyphBitmapPhased(glyph uint32, sizePt float64, dpi int, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	data, err := f.rasterFontData()
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	return ftcgo.RenderGlyphBitmapPhased(data, glyph, sizePt, dpi, phaseX, phaseY)
}

// RenderGlyphBitmapTransformedPhased renders a standard-font glyph through FreeType with Poppler-style axis scaling.
func (f *StandardFont) RenderGlyphBitmapTransformedPhased(glyph uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	data, err := f.rasterFontData()
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	return ftcgo.RenderGlyphBitmapTransformedPhased(data, glyph, sizePt, scaleX, scaleY, phaseX, phaseY)
}

// RenderGlyphBitmapMatrixPhased renders a standard-font glyph through FreeType with Poppler's full 2x2 matrix.
func (f *StandardFont) RenderGlyphBitmapMatrixPhased(glyph uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	data, err := f.rasterFontData()
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	return ftcgo.RenderGlyphBitmapMatrixPhased(data, glyph, sizePt, matrix, phaseX, phaseY)
}

func (f *StandardFont) rasterFontData() ([]byte, error) {
	if len(f.rasterData) == 0 {
		return nil, fmt.Errorf("missing Poppler substitute raster data for font %s", f.name)
	}
	return f.rasterData, nil
}

func (f *StandardFont) loadFont() (*sfnt.Font, error) {
	f.syncOnce.Do(func() {
		if f.sfntData == nil {
			f.sfntErr = fmt.Errorf("missing sfnt data for font %s", f.name)
			return
		}
		f.sfntFont, f.sfntErr = sfnt.Parse(f.sfntData)
	})
	if f.sfntFont == nil {
		return nil, f.sfntErr
	}
	return f.sfntFont, f.sfntErr
}

func (f *StandardFont) glyphIndex(glyph uint32) (sfnt.GlyphIndex, error) {
	if glyph > 0x10FFFF {
		return 0, &errors.OutOfRangeError{Code: glyph}
	}
	sfntFont, err := f.loadFont()
	if err != nil {
		return 0, err
	}
	index, err := sfntFont.GlyphIndex(nil, rune(glyph))
	if err != nil {
		return 0, err
	}
	if index == 0 {
		return 0, &errors.OutOfRangeError{Code: glyph}
	}
	return index, nil
}

// getGlyphName returns the Adobe glyph name for a glyph ID.
func getGlyphName(glyph uint32) string {
	if glyph < 32 || glyph > 126 {
		return fmt.Sprintf(".notdef.%d", glyph)
	}
	if name, ok := standardGlyphNameByCode[glyph]; ok {
		return name
	}
	return string(byte(glyph))
}

var standardGlyphNameByCode = map[uint32]string{
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
	0x5B: "bracketleft",
	0x5C: "backslash",
	0x5D: "bracketright",
	0x5E: "asciicircum",
	0x5F: "underscore",
	0x60: "quoteleft",
	0x7B: "braceleft",
	0x7C: "bar",
	0x7D: "braceright",
	0x7E: "asciitilde",
}

var standardGlyphByName = func() map[string]uint32 {
	out := map[string]uint32{
		`"`: '"',
		"'": '\'',
		",": ',',
		"-": '-',
		".": '.',
		"+": '+',
		"<": '<',
	}
	for code, name := range standardGlyphNameByCode {
		out[name] = code
	}
	return out
}()

// Font width tables (simplified - would have actual metrics in production)
func getWidths(fontName string) []float64 {
	// Return simplified widths (all 500 for simplicity)
	// In production, these would be actual font metrics
	widths := make([]float64, 256)
	for i := range widths {
		widths[i] = 500.0
	}

	// Set some reasonable variations
	switch fontName {
	case "Times-Roman", "Helvetica":
		// Proportional fonts have variable widths
		for c := 'A'; c <= 'Z'; c++ {
			widths[c] = 600.0
		}
		for c := 'a'; c <= 'z'; c++ {
			widths[c] = 500.0
		}
		widths[' '] = 250.0
		widths['i'] = 250.0
		widths['I'] = 350.0
		widths['M'] = 700.0
		widths['W'] = 800.0
		widths['m'] = 500.0
		widths['w'] = 500.0
	case "Courier", "Courier-Bold", "Courier-Oblique", "Courier-BoldOblique":
		// Monospace - all characters same width
		for i := range widths {
			widths[i] = 600.0
		}
	}

	return widths
}
