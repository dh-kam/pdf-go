package renderer

import (
	"fmt"
	"os"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type widthMappedFont struct {
	base         entity.Font
	widths       map[uint32]float64
	widthsByCode map[uint32]float64
	defaultWidth float64
}

type encodedFont struct {
	base         entity.Font
	glyphByCode  map[uint32]uint32
	nameByCode   map[uint32]string
	defaultWidth float64
}

type glyphSourceOverrideFont struct {
	base      entity.Font
	overrides map[uint32]glyphSourceOverride
}

type type1CCodeToGIDFont struct {
	base         entity.Font
	sourceByCode map[uint32]glyphSourceOverride
	targetByCode map[uint32]uint32
	nameByCode   map[uint32]string
	cacheBBox    [4]float64
	cacheUnits   uint16
	hasCacheBBox bool
}

type fontBBoxOverrideFont struct {
	base entity.Font
	bbox [4]float64
}

type glyphSourceOverride struct {
	font  entity.Font
	glyph uint32
}

type transformedGlyphBitmapRenderer interface {
	RenderGlyphBitmapTransformedPhased(glyph uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error)
}

type matrixGlyphBitmapRenderer interface {
	RenderGlyphBitmapMatrixPhased(glyph uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error)
}

type fontBaseUnwrapper interface {
	BaseFont() entity.Font
}

const type1CCodeGlyphTokenBase = uint32(0xC1000000)

func encodeType1CCodeGlyphToken(code uint32) uint32 {
	return type1CCodeGlyphTokenBase + (code & 0xff)
}

func decodeType1CCodeGlyphToken(glyph uint32) (uint32, bool) {
	if glyph < type1CCodeGlyphTokenBase || glyph >= type1CCodeGlyphTokenBase+256 {
		return 0, false
	}
	return glyph - type1CCodeGlyphTokenBase, true
}

func unwrapBitmapGlyphRenderer(font entity.Font) (entity.BitmapGlyphRenderer, bool) {
	if renderer, ok := font.(entity.BitmapGlyphRenderer); ok {
		return renderer, true
	}
	if unwrapper, ok := font.(fontBaseUnwrapper); ok {
		return unwrapBitmapGlyphRenderer(unwrapper.BaseFont())
	}
	return nil, false
}

func unwrapBitmapGlyphRendererPhased(font entity.Font) (entity.BitmapGlyphRendererPhased, bool) {
	if renderer, ok := font.(entity.BitmapGlyphRendererPhased); ok {
		return renderer, true
	}
	if unwrapper, ok := font.(fontBaseUnwrapper); ok {
		return unwrapBitmapGlyphRendererPhased(unwrapper.BaseFont())
	}
	return nil, false
}

func unwrapTransformedGlyphBitmapRenderer(font entity.Font) (transformedGlyphBitmapRenderer, bool) {
	if renderer, ok := font.(transformedGlyphBitmapRenderer); ok {
		return renderer, true
	}
	if unwrapper, ok := font.(fontBaseUnwrapper); ok {
		return unwrapTransformedGlyphBitmapRenderer(unwrapper.BaseFont())
	}
	return nil, false
}

func unwrapMatrixGlyphBitmapRenderer(font entity.Font) (matrixGlyphBitmapRenderer, bool) {
	if renderer, ok := font.(matrixGlyphBitmapRenderer); ok {
		return renderer, true
	}
	if unwrapper, ok := font.(fontBaseUnwrapper); ok {
		return unwrapMatrixGlyphBitmapRenderer(unwrapper.BaseFont())
	}
	return nil, false
}

// BaseFont returns the underlying font for unwrapping.
func (f *widthMappedFont) BaseFont() entity.Font { return f.base }

// BaseFont returns the underlying font for unwrapping.
func (f *encodedFont) BaseFont() entity.Font { return f.base }

// BaseFont returns the underlying font for unwrapping.
func (f *glyphSourceOverrideFont) BaseFont() entity.Font { return f.base }

// BaseFont returns the underlying font for unwrapping.
func (f *type1CCodeToGIDFont) BaseFont() entity.Font { return f.base }

// BaseFont returns the underlying font for unwrapping.
func (f *fontBBoxOverrideFont) BaseFont() entity.Font { return f.base }

type glyphIDByNameFont interface {
	GlyphIDByName(name string) (uint32, bool)
}

type encodingNameFont interface {
	EncodingName(code byte) string
}

type freeTypeBoundingBoxFont interface {
	FreeTypeBoundingBox() (float64, float64, float64, float64, uint16, bool)
}

func (f *encodedFont) CharCodeToGlyph(code uint32) (uint32, error) {
	if f == nil || f.base == nil {
		return 0, fmt.Errorf("font is nil")
	}
	if f.glyphByCode != nil {
		if glyph, ok := f.glyphByCode[code]; ok {
			return glyph, nil
		}
	}
	return f.base.CharCodeToGlyph(code)
}

func (f *encodedFont) GlyphName(glyph uint32) string {
	if f == nil || f.base == nil {
		return ""
	}
	return f.base.GlyphName(glyph)
}

func (f *encodedFont) GetGlyphWidth(glyph uint32) (float64, error) {
	if f == nil || f.base == nil {
		return 0, fmt.Errorf("font is nil")
	}
	return f.base.GetGlyphWidth(glyph)
}

func (f *encodedFont) GetBoundingBox() (float64, float64, float64, float64) {
	return f.base.GetBoundingBox()
}

func (f *encodedFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return f.base.RenderGlyph(glyph, size)
}

func (f *encodedFont) IsCIDFont() bool {
	return f.base.IsCIDFont()
}

func (f *encodedFont) IsSymbolic() bool {
	return f.base.IsSymbolic()
}

func (f *encodedFont) UnitsPerEm() uint16 {
	return f.base.UnitsPerEm()
}

func (f *encodedFont) Name() string {
	return f.base.Name()
}

func (f *encodedFont) GlyphIDByName(name string) (uint32, bool) {
	if f == nil || f.base == nil {
		return 0, false
	}
	namedFont, ok := f.base.(glyphIDByNameFont)
	if !ok {
		return 0, false
	}
	return namedFont.GlyphIDByName(name)
}

func encodingGlyphNameCandidates(name string) []string {
	if name == "" {
		return nil
	}

	candidates := []string{name}
	appendAlias := func(alias string) {
		if alias == "" {
			return
		}
		for _, existing := range candidates {
			if existing == alias {
				return
			}
		}
		candidates = append(candidates, alias)
	}

	switch name {
	case "quotedblleft", "quotedblright":
		appendAlias(`"`)
	case "quotedblbase":
		appendAlias(",")
	case "quoteleft", "quoteright":
		appendAlias("'")
	case "quotesinglbase":
		appendAlias(",")
	case "endash", "emdash", "hyphen":
		appendAlias("-")
	case "minus":
		appendAlias("-")
	case "periodcentered":
		appendAlias(".")
	case "plusminus":
		appendAlias("+")
	case "reflexsubset":
		appendAlias("<")
	case "fi", "fl":
		appendAlias("f")
	case "ff", "ffi":
		appendAlias("f")
	}

	return candidates
}

func (f *widthMappedFont) CharCodeToGlyph(code uint32) (uint32, error) {
	if f == nil || f.base == nil {
		return 0, fmt.Errorf("font is nil")
	}
	return f.base.CharCodeToGlyph(code)
}

func (f *widthMappedFont) GlyphName(glyph uint32) string {
	if f == nil || f.base == nil {
		return ""
	}
	return f.base.GlyphName(glyph)
}

func (f *widthMappedFont) GetGlyphWidth(glyph uint32) (float64, error) {
	if f == nil || f.base == nil {
		return 0, fmt.Errorf("font is nil")
	}

	if f.widths != nil {
		if width, ok := f.widths[glyph]; ok {
			if width == 500 && shouldIgnoreMappedWidth500ForDebug(f.base.Name()) {
				return f.base.GetGlyphWidth(glyph)
			}
			return width, nil
		}
	}

	if f.defaultWidth > 0 {
		if _, err := f.base.GetGlyphWidth(glyph); err == nil {
			width, err := f.base.GetGlyphWidth(glyph)
			if err == nil {
				return width, nil
			}
		}
		return f.defaultWidth, nil
	}

	width, err := f.base.GetGlyphWidth(glyph)
	if err != nil && len(f.widths) > 0 {
		width = 500
	}
	return width, err
}

func (f *widthMappedFont) GetCharCodeWidth(code uint32) (float64, bool) {
	if f == nil {
		return 0, false
	}
	if f.widthsByCode == nil {
		return 0, false
	}
	width, ok := f.widthsByCode[code]
	return width, ok
}

func (f *widthMappedFont) GetBoundingBox() (float64, float64, float64, float64) {
	return f.base.GetBoundingBox()
}

func (f *widthMappedFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return f.base.RenderGlyph(glyph, size)
}

func (f *widthMappedFont) GlyphIDByName(name string) (uint32, bool) {
	if f == nil || f.base == nil {
		return 0, false
	}
	namedFont, ok := f.base.(glyphIDByNameFont)
	if !ok {
		return 0, false
	}
	return namedFont.GlyphIDByName(name)
}

func (f *widthMappedFont) IsCIDFont() bool {
	return f.base.IsCIDFont()
}

func (f *widthMappedFont) IsSymbolic() bool {
	return f.base.IsSymbolic()
}

func (f *widthMappedFont) UnitsPerEm() uint16 {
	return f.base.UnitsPerEm()
}

func (f *widthMappedFont) Name() string {
	return f.base.Name()
}

func (f *glyphSourceOverrideFont) CharCodeToGlyph(code uint32) (uint32, error) {
	if f == nil || f.base == nil {
		return 0, fmt.Errorf("font is nil")
	}
	return f.base.CharCodeToGlyph(code)
}

func (f *glyphSourceOverrideFont) GlyphName(glyph uint32) string {
	if f == nil || f.base == nil {
		return ""
	}
	return f.base.GlyphName(glyph)
}

func (f *glyphSourceOverrideFont) GetGlyphWidth(glyph uint32) (float64, error) {
	if f == nil || f.base == nil {
		return 0, fmt.Errorf("font is nil")
	}
	return f.base.GetGlyphWidth(glyph)
}

func (f *glyphSourceOverrideFont) GetBoundingBox() (float64, float64, float64, float64) {
	return f.base.GetBoundingBox()
}

func (f *glyphSourceOverrideFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	if f == nil || f.base == nil {
		return nil, fmt.Errorf("font is nil")
	}
	if override, ok := f.overrides[glyph]; ok && override.font != nil {
		return override.font.RenderGlyph(override.glyph, size)
	}
	return f.base.RenderGlyph(glyph, size)
}

func (f *glyphSourceOverrideFont) RenderGlyphBitmap(glyph uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error) {
	if f == nil || f.base == nil {
		return nil, 0, 0, 0, 0, fmt.Errorf("font is nil")
	}
	if override, ok := f.overrides[glyph]; ok && override.font != nil {
		if renderer, ok := override.font.(entity.BitmapGlyphRenderer); ok {
			return renderer.RenderGlyphBitmap(override.glyph, sizePt, dpi)
		}
	}
	renderer, ok := f.base.(entity.BitmapGlyphRenderer)
	if !ok {
		return nil, 0, 0, 0, 0, fmt.Errorf("font does not render bitmap glyphs")
	}
	return renderer.RenderGlyphBitmap(glyph, sizePt, dpi)
}

func (f *glyphSourceOverrideFont) RenderGlyphBitmapPhased(glyph uint32, sizePt float64, dpi int, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if f == nil || f.base == nil {
		return nil, 0, 0, 0, 0, fmt.Errorf("font is nil")
	}
	if override, ok := f.overrides[glyph]; ok && override.font != nil {
		if renderer, ok := override.font.(entity.BitmapGlyphRendererPhased); ok {
			return renderer.RenderGlyphBitmapPhased(override.glyph, sizePt, dpi, phaseX, phaseY)
		}
	}
	renderer, ok := f.base.(entity.BitmapGlyphRendererPhased)
	if !ok {
		return nil, 0, 0, 0, 0, fmt.Errorf("font does not render phased bitmap glyphs")
	}
	return renderer.RenderGlyphBitmapPhased(glyph, sizePt, dpi, phaseX, phaseY)
}

func (f *glyphSourceOverrideFont) RenderGlyphBitmapTransformedPhased(glyph uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if f == nil || f.base == nil {
		return nil, 0, 0, 0, 0, fmt.Errorf("font is nil")
	}
	if override, ok := f.overrides[glyph]; ok && override.font != nil {
		if os.Getenv("PDF_DEBUG_TYPE1C_GLYPH_SOURCE") == "1" {
			fmt.Fprintf(os.Stderr, "GLYPH_SOURCE_TRANSFORMED hit glyph=%d sourceGlyph=%d\n", glyph, override.glyph)
		}
		if renderer, ok := override.font.(transformedGlyphBitmapRenderer); ok {
			return renderer.RenderGlyphBitmapTransformedPhased(override.glyph, sizePt, scaleX, scaleY, phaseX, phaseY)
		}
	}
	if os.Getenv("PDF_DEBUG_TYPE1C_GLYPH_SOURCE") == "1" {
		fmt.Fprintf(os.Stderr, "GLYPH_SOURCE_TRANSFORMED miss glyph=%d\n", glyph)
	}
	renderer, ok := f.base.(transformedGlyphBitmapRenderer)
	if !ok {
		return nil, 0, 0, 0, 0, fmt.Errorf("font does not render transformed bitmap glyphs")
	}
	return renderer.RenderGlyphBitmapTransformedPhased(glyph, sizePt, scaleX, scaleY, phaseX, phaseY)
}

func (f *glyphSourceOverrideFont) RenderGlyphBitmapMatrixPhased(glyph uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if f == nil || f.base == nil {
		return nil, 0, 0, 0, 0, fmt.Errorf("font is nil")
	}
	if override, ok := f.overrides[glyph]; ok && override.font != nil {
		if renderer, ok := override.font.(matrixGlyphBitmapRenderer); ok {
			return renderer.RenderGlyphBitmapMatrixPhased(override.glyph, sizePt, matrix, phaseX, phaseY)
		}
	}
	renderer, ok := f.base.(matrixGlyphBitmapRenderer)
	if !ok {
		return nil, 0, 0, 0, 0, fmt.Errorf("font does not render matrix bitmap glyphs")
	}
	return renderer.RenderGlyphBitmapMatrixPhased(glyph, sizePt, matrix, phaseX, phaseY)
}

func (f *glyphSourceOverrideFont) IsCIDFont() bool {
	return f.base.IsCIDFont()
}

func (f *glyphSourceOverrideFont) IsSymbolic() bool {
	return f.base.IsSymbolic()
}

func (f *glyphSourceOverrideFont) UnitsPerEm() uint16 {
	return f.base.UnitsPerEm()
}

func (f *glyphSourceOverrideFont) Name() string {
	return f.base.Name()
}

func (f *type1CCodeToGIDFont) CharCodeToGlyph(code uint32) (uint32, error) {
	if f == nil || f.base == nil {
		return 0, fmt.Errorf("font is nil")
	}
	if _, ok := f.sourceByCode[code]; ok {
		return encodeType1CCodeGlyphToken(code), nil
	}
	return f.base.CharCodeToGlyph(code)
}

func (f *type1CCodeToGIDFont) GlyphName(glyph uint32) string {
	if f == nil || f.base == nil {
		return ""
	}
	if code, ok := decodeType1CCodeGlyphToken(glyph); ok {
		if name := f.nameByCode[code]; name != "" {
			return name
		}
		if target, ok := f.targetByCode[code]; ok {
			return f.base.GlyphName(target)
		}
		return ".notdef"
	}
	return f.base.GlyphName(glyph)
}

func (f *type1CCodeToGIDFont) GetGlyphWidth(glyph uint32) (float64, error) {
	if f == nil || f.base == nil {
		return 0, fmt.Errorf("font is nil")
	}
	if code, ok := decodeType1CCodeGlyphToken(glyph); ok {
		if target, ok := f.targetByCode[code]; ok {
			return f.base.GetGlyphWidth(target)
		}
		return 500, nil
	}
	return f.base.GetGlyphWidth(glyph)
}

func (f *type1CCodeToGIDFont) GetBoundingBox() (float64, float64, float64, float64) {
	return f.base.GetBoundingBox()
}

func (f *type1CCodeToGIDFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	if f == nil || f.base == nil {
		return nil, fmt.Errorf("font is nil")
	}
	if override, ok := f.type1COverrideForGlyph(glyph); ok && override.font != nil {
		return override.font.RenderGlyph(override.glyph, size)
	}
	return f.base.RenderGlyph(glyph, size)
}

func (f *type1CCodeToGIDFont) RenderGlyphBitmap(glyph uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error) {
	if f == nil || f.base == nil {
		return nil, 0, 0, 0, 0, fmt.Errorf("font is nil")
	}
	if override, ok := f.type1COverrideForGlyph(glyph); ok && override.font != nil {
		if renderer, ok := override.font.(entity.BitmapGlyphRenderer); ok {
			return renderer.RenderGlyphBitmap(override.glyph, sizePt, dpi)
		}
	}
	if renderer, ok := unwrapBitmapGlyphRenderer(f.base); ok {
		return renderer.RenderGlyphBitmap(glyph, sizePt, dpi)
	}
	return nil, 0, 0, 0, 0, fmt.Errorf("font does not render bitmap glyphs")
}

func (f *type1CCodeToGIDFont) RenderGlyphBitmapPhased(glyph uint32, sizePt float64, dpi int, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if f == nil || f.base == nil {
		return nil, 0, 0, 0, 0, fmt.Errorf("font is nil")
	}
	if override, ok := f.type1COverrideForGlyph(glyph); ok && override.font != nil {
		if renderer, ok := override.font.(entity.BitmapGlyphRendererPhased); ok {
			return renderer.RenderGlyphBitmapPhased(override.glyph, sizePt, dpi, phaseX, phaseY)
		}
	}
	if renderer, ok := unwrapBitmapGlyphRendererPhased(f.base); ok {
		return renderer.RenderGlyphBitmapPhased(glyph, sizePt, dpi, phaseX, phaseY)
	}
	return nil, 0, 0, 0, 0, fmt.Errorf("font does not render phased bitmap glyphs")
}

func (f *type1CCodeToGIDFont) RenderGlyphBitmapTransformedPhased(glyph uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if f == nil || f.base == nil {
		return nil, 0, 0, 0, 0, fmt.Errorf("font is nil")
	}
	if override, ok := f.type1COverrideForGlyph(glyph); ok && override.font != nil {
		if os.Getenv("PDF_DEBUG_TYPE1C_GLYPH_SOURCE") == "1" {
			code, _ := decodeType1CCodeGlyphToken(glyph)
			fmt.Fprintf(os.Stderr, "TYPE1C_CODETOGID_TRANSFORMED hit code=%d token=%d sourceGlyph=%d\n", code, glyph, override.glyph)
		}
		if renderer, ok := override.font.(transformedGlyphBitmapRenderer); ok {
			return renderer.RenderGlyphBitmapTransformedPhased(override.glyph, sizePt, scaleX, scaleY, phaseX, phaseY)
		}
	}
	if os.Getenv("PDF_DEBUG_TYPE1C_GLYPH_SOURCE") == "1" {
		fmt.Fprintf(os.Stderr, "TYPE1C_CODETOGID_TRANSFORMED miss glyph=%d\n", glyph)
	}
	if renderer, ok := unwrapTransformedGlyphBitmapRenderer(f.base); ok {
		return renderer.RenderGlyphBitmapTransformedPhased(glyph, sizePt, scaleX, scaleY, phaseX, phaseY)
	}
	return nil, 0, 0, 0, 0, fmt.Errorf("font does not render transformed bitmap glyphs")
}

func (f *type1CCodeToGIDFont) RenderGlyphBitmapMatrixPhased(glyph uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	if f == nil || f.base == nil {
		return nil, 0, 0, 0, 0, fmt.Errorf("font is nil")
	}
	if override, ok := f.type1COverrideForGlyph(glyph); ok && override.font != nil {
		if renderer, ok := override.font.(matrixGlyphBitmapRenderer); ok {
			return renderer.RenderGlyphBitmapMatrixPhased(override.glyph, sizePt, matrix, phaseX, phaseY)
		}
	}
	if renderer, ok := unwrapMatrixGlyphBitmapRenderer(f.base); ok {
		return renderer.RenderGlyphBitmapMatrixPhased(glyph, sizePt, matrix, phaseX, phaseY)
	}
	return nil, 0, 0, 0, 0, fmt.Errorf("font does not render matrix bitmap glyphs")
}

func (f *type1CCodeToGIDFont) GlyphIDByName(name string) (uint32, bool) {
	if f == nil || f.base == nil {
		return 0, false
	}
	namedFont, ok := f.base.(glyphIDByNameFont)
	if !ok {
		return 0, false
	}
	return namedFont.GlyphIDByName(name)
}

func (f *type1CCodeToGIDFont) IsCIDFont() bool {
	return f.base.IsCIDFont()
}

func (f *type1CCodeToGIDFont) IsSymbolic() bool {
	return f.base.IsSymbolic()
}

func (f *type1CCodeToGIDFont) UnitsPerEm() uint16 {
	return f.base.UnitsPerEm()
}

func (f *type1CCodeToGIDFont) Name() string {
	return f.base.Name()
}

func (f *type1CCodeToGIDFont) PopplerGlyphCacheBBox() (float64, float64, float64, float64, uint16, bool) {
	if f == nil || !f.hasCacheBBox || f.cacheUnits == 0 {
		return 0, 0, 0, 0, 0, false
	}
	return f.cacheBBox[0], f.cacheBBox[1], f.cacheBBox[2], f.cacheBBox[3], f.cacheUnits, true
}

func (f *type1CCodeToGIDFont) type1COverrideForGlyph(glyph uint32) (glyphSourceOverride, bool) {
	code, ok := decodeType1CCodeGlyphToken(glyph)
	if !ok {
		return glyphSourceOverride{}, false
	}
	override, ok := f.sourceByCode[code]
	return override, ok
}

func (f *fontBBoxOverrideFont) CharCodeToGlyph(code uint32) (uint32, error) {
	if f == nil || f.base == nil {
		return 0, fmt.Errorf("font is nil")
	}
	return f.base.CharCodeToGlyph(code)
}

func (f *fontBBoxOverrideFont) GlyphName(glyph uint32) string {
	if f == nil || f.base == nil {
		return ""
	}
	return f.base.GlyphName(glyph)
}

func (f *fontBBoxOverrideFont) GetGlyphWidth(glyph uint32) (float64, error) {
	if f == nil || f.base == nil {
		return 0, fmt.Errorf("font is nil")
	}
	return f.base.GetGlyphWidth(glyph)
}

func (f *fontBBoxOverrideFont) GetBoundingBox() (float64, float64, float64, float64) {
	if f == nil || f.base == nil {
		return 0, 0, 0, 0
	}
	return f.bbox[0], f.bbox[1], f.bbox[2], f.bbox[3]
}

func (f *fontBBoxOverrideFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	if f == nil || f.base == nil {
		return nil, fmt.Errorf("font is nil")
	}
	return f.base.RenderGlyph(glyph, size)
}

func (f *fontBBoxOverrideFont) IsCIDFont() bool {
	return f.base.IsCIDFont()
}

func (f *fontBBoxOverrideFont) IsSymbolic() bool {
	return f.base.IsSymbolic()
}

func (f *fontBBoxOverrideFont) UnitsPerEm() uint16 {
	return f.base.UnitsPerEm()
}

func (f *fontBBoxOverrideFont) Name() string {
	return f.base.Name()
}

// cidIdentityFont wraps a TrueType font for use as a CIDFontType2 descendant.
// IsCIDFont returns true (so text is processed as 2-byte CIDs), and
// CharCodeToGlyph returns the CID directly as the GID (Identity mapping).
// Per PDF spec, CIDFontType2 with Identity CIDToGIDMap maps CID directly to GID.
type cidIdentityFont struct {
	base      entity.Font
	toUnicode map[uint32]rune // optional CID→Unicode from ToUnicode CMap (for text extraction only)
}

func (f *cidIdentityFont) CharCodeToGlyph(code uint32) (uint32, error) {
	// For Identity CIDToGIDMap, CID == GID directly.
	// Do NOT use toUnicode for glyph resolution: toUnicode is for text extraction,
	// not for glyph rendering. Subsetted TrueType fonts assign glyph slots by CID,
	// not by Unicode codepoint, so going via Unicode→cmap would yield wrong glyph IDs.
	return code, nil
}

func (f *cidIdentityFont) GlyphName(glyph uint32) string { return f.base.GlyphName(glyph) }
func (f *cidIdentityFont) GetGlyphWidth(glyph uint32) (float64, error) {
	return f.base.GetGlyphWidth(glyph)
}
func (f *cidIdentityFont) GetBoundingBox() (float64, float64, float64, float64) {
	return f.base.GetBoundingBox()
}
func (f *cidIdentityFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	return f.base.RenderGlyph(glyph, size)
}
func (f *cidIdentityFont) IsCIDFont() bool       { return true }
func (f *cidIdentityFont) IsSymbolic() bool      { return f.base.IsSymbolic() }
func (f *cidIdentityFont) UnitsPerEm() uint16    { return f.base.UnitsPerEm() }
func (f *cidIdentityFont) Name() string          { return f.base.Name() }
func (f *cidIdentityFont) BaseFont() entity.Font { return f.base }
