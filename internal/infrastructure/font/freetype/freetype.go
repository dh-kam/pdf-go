// Package freetype provides a pure Go fallback when FreeType is not linked.
package freetype

import (
	"crypto/sha256"
	"fmt"
	"image"
	"math"
	"sync"

	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

type sfntCacheKey struct {
	sum  [32]byte
	size int
}

type sfntCacheEntry struct {
	font *sfnt.Font
	err  error
}

var pureGoSFNTCache sync.Map

// IsAvailable returns true because the pure Go sfnt fallback is available.
func IsAvailable() bool {
	return true
}

// UsesPureGoFallback reports whether this package is using the no-CGo sfnt fallback.
func UsesPureGoFallback() bool {
	return true
}

func parsePureGoSFNT(fontData []byte) (*sfnt.Font, error) {
	if len(fontData) == 0 {
		return nil, fmt.Errorf("empty font data")
	}
	key := sfntCacheKey{sum: sha256.Sum256(fontData), size: len(fontData)}
	if cached, ok := pureGoSFNTCache.Load(key); ok {
		entry := cached.(sfntCacheEntry)
		return entry.font, entry.err
	}
	font, err := sfnt.Parse(fontData)
	entry := sfntCacheEntry{font: font, err: err}
	pureGoSFNTCache.Store(key, entry)
	return font, err
}

// GetGlyphIndexByCharCode returns the pure Go sfnt glyph index for a char code.
func GetGlyphIndexByCharCode(fontData []byte, charCode uint32) (uint32, bool) {
	if glyphIndex, ok, handled := getGlyphIndexByCharCodeFreeTypeGo(fontData, charCode); handled {
		return glyphIndex, ok
	}
	font, err := parsePureGoSFNT(fontData)
	if err != nil {
		return 0, false
	}
	var buf sfnt.Buffer
	glyph, err := font.GlyphIndex(&buf, rune(charCode))
	if err != nil || glyph == 0 {
		return 0, false
	}
	return uint32(glyph), true
}

// GetGlyphIndexByName returns the pure Go sfnt glyph index for a glyph name.
func GetGlyphIndexByName(fontData []byte, glyphName string) (uint32, bool) {
	if glyphName == "" {
		return 0, false
	}
	if glyphIndex, ok, handled := getGlyphIndexByNameFreeTypeGo(fontData, glyphName); handled {
		return glyphIndex, ok
	}
	font, err := parsePureGoSFNT(fontData)
	if err != nil {
		return 0, false
	}
	var buf sfnt.Buffer
	for i := 0; i < font.NumGlyphs(); i++ {
		name, err := font.GlyphName(&buf, sfnt.GlyphIndex(i))
		if err == nil && name == glyphName {
			return uint32(i), true
		}
	}
	return 0, false
}

// GetGlyphNameByCharCode returns the glyph name selected by the sfnt cmap.
func GetGlyphNameByCharCode(fontData []byte, charCode uint32) (string, bool) {
	if name, ok, handled := getGlyphNameByCharCodeFreeTypeGo(fontData, charCode); handled {
		return name, ok
	}
	font, err := parsePureGoSFNT(fontData)
	if err != nil {
		return "", false
	}
	var buf sfnt.Buffer
	glyph, err := font.GlyphIndex(&buf, rune(charCode))
	if err != nil || glyph == 0 {
		return "", false
	}
	name, err := font.GlyphName(&buf, glyph)
	if err != nil || name == "" {
		return "", false
	}
	return name, true
}

// GetFaceBoundingBox returns the sfnt face bounds in font units.
func GetFaceBoundingBox(fontData []byte) (float64, float64, float64, float64, uint16, bool) {
	if xMin, yMin, xMax, yMax, units, ok, handled := getFaceBoundingBoxFreeTypeGo(fontData); handled {
		return xMin, yMin, xMax, yMax, units, ok
	}
	font, err := parsePureGoSFNT(fontData)
	if err != nil {
		return 0, 0, 0, 0, 0, false
	}
	units := font.UnitsPerEm()
	if units == 0 {
		return 0, 0, 0, 0, 0, false
	}
	var buf sfnt.Buffer
	bounds, err := font.Bounds(&buf, fixed.Int26_6(units), xfont.HintingNone)
	if err != nil {
		return 0, 0, 0, 0, 0, false
	}
	xMin := fixedToFloat(bounds.Min.X)
	xMax := fixedToFloat(bounds.Max.X)
	yMin := -fixedToFloat(bounds.Max.Y)
	yMax := -fixedToFloat(bounds.Min.Y)
	return xMin, yMin, xMax, yMax, uint16(units), true
}

// RenderGlyph renders a glyph outline by first resolving the character code.
func RenderGlyph(fontData []byte, glyphCode uint32, size float64, dpi int) (*entity.GlyphPath, error) {
	glyphIndex, ok := GetGlyphIndexByCharCode(fontData, glyphCode)
	if !ok {
		return nil, fmt.Errorf("purego sfnt: glyph not found for code %d", glyphCode)
	}
	return RenderGlyphByIndex(fontData, glyphIndex, size, dpi)
}

// RenderGlyphByIndex renders a glyph outline using its sfnt glyph index.
func RenderGlyphByIndex(fontData []byte, glyphIndex uint32, sizePt float64, dpi int) (*entity.GlyphPath, error) {
	font, err := parsePureGoSFNT(fontData)
	if err != nil {
		return nil, err
	}
	ppem := fixed.Int26_6(math.Round(sizePt * 64))
	if dpi > 0 {
		ppem = fixed.Int26_6(math.Round(sizePt * float64(dpi) / 72 * 64))
	}
	segments, err := font.LoadGlyph(&sfnt.Buffer{}, sfnt.GlyphIndex(glyphIndex), ppem, nil)
	if err != nil {
		return nil, err
	}
	return glyphPathFromSFNTSegments(segments)
}

// RenderGlyphBitmap renders a glyph to a grayscale alpha bitmap.
func RenderGlyphBitmap(fontData []byte, glyphCode uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error) {
	glyphIndex, ok := GetGlyphIndexByCharCode(fontData, glyphCode)
	if !ok {
		return nil, 0, 0, 0, 0, fmt.Errorf("purego sfnt: glyph not found for code %d", glyphCode)
	}
	return RenderGlyphBitmapByIndex(fontData, glyphIndex, sizePt, dpi)
}

// RenderGlyphBitmapByIndex renders a glyph bitmap by sfnt glyph index.
func RenderGlyphBitmapByIndex(fontData []byte, glyphIndex uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error) {
	return renderGlyphBitmapByIndexMode(fontData, glyphIndex, sizePt, identityMatrix(), 0, 0, dpi, true, false)
}

// RenderGlyphBitmapByIndexLegacy renders a glyph with the pre-FreeType-normalization pure-Go path.
func RenderGlyphBitmapByIndexLegacy(fontData []byte, glyphIndex uint32, sizePt float64, dpi int) ([]byte, int, int, int, int, error) {
	return renderGlyphBitmapByIndexLegacy(fontData, glyphIndex, sizePt, identityMatrix(), 0, 0, dpi)
}

// RenderGlyphBitmapPhased renders a glyph bitmap with sub-pixel phase.
func RenderGlyphBitmapPhased(fontData []byte, glyphCode uint32, sizePt float64, dpi int, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	glyphIndex, ok := GetGlyphIndexByCharCode(fontData, glyphCode)
	if !ok {
		return nil, 0, 0, 0, 0, fmt.Errorf("purego sfnt: glyph not found for code %d", glyphCode)
	}
	return RenderGlyphBitmapByIndexPhased(fontData, glyphIndex, sizePt, dpi, phaseX, phaseY)
}

// RenderGlyphBitmapByIndexPhased renders a glyph bitmap by sfnt glyph index with sub-pixel phase.
func RenderGlyphBitmapByIndexPhased(fontData []byte, glyphIndex uint32, sizePt float64, dpi int, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	return renderGlyphBitmapByIndexMode(fontData, glyphIndex, sizePt, identityMatrix(), phaseX, phaseY, dpi, true, false)
}

// RenderGlyphBitmapByIndexPhasedLegacy renders a phased glyph with the legacy pure-Go path.
func RenderGlyphBitmapByIndexPhasedLegacy(fontData []byte, glyphIndex uint32, sizePt float64, dpi int, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	return renderGlyphBitmapByIndexLegacy(fontData, glyphIndex, sizePt, identityMatrix(), phaseX, phaseY, dpi)
}

// RenderGlyphBitmapTransformedPhased renders a glyph bitmap with axis-aligned scaling.
func RenderGlyphBitmapTransformedPhased(fontData []byte, glyphCode uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	glyphIndex, ok := GetGlyphIndexByCharCode(fontData, glyphCode)
	if !ok {
		return nil, 0, 0, 0, 0, fmt.Errorf("purego sfnt: glyph not found for code %d", glyphCode)
	}
	return RenderGlyphBitmapByIndexTransformedPhased(fontData, glyphIndex, sizePt, scaleX, scaleY, phaseX, phaseY)
}

// RenderGlyphBitmapByIndexTransformedPhased renders a glyph bitmap by sfnt glyph index with axis-aligned scaling.
func RenderGlyphBitmapByIndexTransformedPhased(fontData []byte, glyphIndex uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	return renderGlyphBitmapByIndexMode(fontData, glyphIndex, sizePt, [4]float64{scaleX, 0, 0, scaleY}, phaseX, phaseY, 72, true, true)
}

// RenderGlyphBitmapByIndexTransformedPhasedLegacy renders a transformed glyph with the legacy pure-Go path.
func RenderGlyphBitmapByIndexTransformedPhasedLegacy(fontData []byte, glyphIndex uint32, sizePt, scaleX, scaleY, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	return renderGlyphBitmapByIndexLegacy(fontData, glyphIndex, sizePt, [4]float64{scaleX, 0, 0, scaleY}, phaseX, phaseY, 72)
}

// RenderGlyphBitmapMatrixPhased renders a glyph bitmap with a 2x2 transform matrix.
func RenderGlyphBitmapMatrixPhased(fontData []byte, glyphCode uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	glyphIndex, ok := GetGlyphIndexByCharCode(fontData, glyphCode)
	if !ok {
		return nil, 0, 0, 0, 0, fmt.Errorf("purego sfnt: glyph not found for code %d", glyphCode)
	}
	return RenderGlyphBitmapByIndexMatrixPhased(fontData, glyphIndex, sizePt, matrix, phaseX, phaseY)
}

// RenderGlyphBitmapByIndexMatrixPhased renders a glyph bitmap by sfnt glyph index with a 2x2 transform matrix.
func RenderGlyphBitmapByIndexMatrixPhased(fontData []byte, glyphIndex uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	return renderGlyphBitmapByIndexMode(fontData, glyphIndex, sizePt, matrix, phaseX, phaseY, 72, true, true)
}

// RenderGlyphBitmapByIndexMatrixPhasedLegacy renders a matrix-transformed glyph with the legacy pure-Go path.
func RenderGlyphBitmapByIndexMatrixPhasedLegacy(fontData []byte, glyphIndex uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64) ([]byte, int, int, int, int, error) {
	return renderGlyphBitmapByIndexLegacy(fontData, glyphIndex, sizePt, matrix, phaseX, phaseY, 72)
}

func renderGlyphBitmapByIndex(fontData []byte, glyphIndex uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64, dpi int) ([]byte, int, int, int, int, error) {
	return renderGlyphBitmapByIndexMode(fontData, glyphIndex, sizePt, matrix, phaseX, phaseY, dpi, true, false)
}

func renderGlyphBitmapByIndexLegacy(fontData []byte, glyphIndex uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64, dpi int) ([]byte, int, int, int, int, error) {
	return renderGlyphBitmapByIndexMode(fontData, glyphIndex, sizePt, matrix, phaseX, phaseY, dpi, false, false)
}

func renderGlyphBitmapByIndexMode(fontData []byte, glyphIndex uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64, dpi int, normalizeMatrix bool, floorPhase bool) ([]byte, int, int, int, int, error) {
	if buf, w, h, left, top, handled, err := renderGlyphBitmapByIndexFreeTypeGo(fontData, glyphIndex, sizePt, matrix, phaseX, phaseY, dpi, normalizeMatrix, floorPhase); handled {
		return buf, w, h, left, top, err
	}
	font, err := parsePureGoSFNT(fontData)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	ppem, rasterMatrix := legacySFNTBitmapMatrix(sizePt, matrix, dpi)
	if normalizeMatrix {
		ppem, rasterMatrix = popplerSFNTBitmapMatrix(sizePt, matrix, dpi)
	}
	if ppem <= 0 {
		return nil, 0, 0, 0, 0, fmt.Errorf("invalid glyph ppem")
	}
	segments, err := font.LoadGlyph(&sfnt.Buffer{}, sfnt.GlyphIndex(glyphIndex), ppem, nil)
	if err != nil {
		return nil, 0, 0, 0, 0, err
	}
	points := transformedSFNTBounds(segments, rasterMatrix, phaseX, phaseY)
	if !points.ok {
		return nil, 0, 0, 0, 0, nil
	}
	left := int(math.Floor(points.minX))
	topY := int(math.Floor(points.minY))
	right := int(math.Ceil(points.maxX))
	bottom := int(math.Ceil(points.maxY))
	width := right - left
	height := bottom - topY
	if width <= 0 || height <= 0 {
		return nil, 0, 0, left, int(math.Ceil(-points.minY)), nil
	}

	ras := vector.NewRasterizer(width, height)
	for _, segment := range segments {
		switch segment.Op {
		case sfnt.SegmentOpMoveTo:
			x, y := transformSFNTPoint(segment.Args[0], rasterMatrix, phaseX, phaseY)
			ras.MoveTo(float32(x-float64(left)), float32(y-float64(topY)))
		case sfnt.SegmentOpLineTo:
			x, y := transformSFNTPoint(segment.Args[0], rasterMatrix, phaseX, phaseY)
			ras.LineTo(float32(x-float64(left)), float32(y-float64(topY)))
		case sfnt.SegmentOpQuadTo:
			x1, y1 := transformSFNTPoint(segment.Args[0], rasterMatrix, phaseX, phaseY)
			x2, y2 := transformSFNTPoint(segment.Args[1], rasterMatrix, phaseX, phaseY)
			ras.QuadTo(float32(x1-float64(left)), float32(y1-float64(topY)), float32(x2-float64(left)), float32(y2-float64(topY)))
		case sfnt.SegmentOpCubeTo:
			x1, y1 := transformSFNTPoint(segment.Args[0], rasterMatrix, phaseX, phaseY)
			x2, y2 := transformSFNTPoint(segment.Args[1], rasterMatrix, phaseX, phaseY)
			x3, y3 := transformSFNTPoint(segment.Args[2], rasterMatrix, phaseX, phaseY)
			ras.CubeTo(float32(x1-float64(left)), float32(y1-float64(topY)), float32(x2-float64(left)), float32(y2-float64(topY)), float32(x3-float64(left)), float32(y3-float64(topY)))
		}
	}

	mask := image.NewAlpha(image.Rect(0, 0, width, height))
	ras.Draw(mask, mask.Bounds(), image.Opaque, image.Point{})
	buf := make([]byte, width*height)
	for y := 0; y < height; y++ {
		copy(buf[y*width:(y+1)*width], mask.Pix[y*mask.Stride:y*mask.Stride+width])
	}
	return buf, width, height, left, int(math.Ceil(-points.minY)), nil
}

func legacySFNTBitmapMatrix(sizePt float64, matrix [4]float64, dpi int) (fixed.Int26_6, [4]float64) {
	if dpi <= 0 {
		dpi = 72
	}
	ppem := fixed.Int26_6(math.Round(sizePt * float64(dpi) / 72.0 * 64.0))
	if ppem < 64 {
		ppem = 64
	}
	return ppem, matrix
}

func popplerSFNTBitmapMatrix(sizePt float64, matrix [4]float64, dpi int) (fixed.Int26_6, [4]float64) {
	if dpi <= 0 {
		dpi = 72
	}
	dpiScale := float64(dpi) / 72.0
	scaled := [4]float64{
		sizePt * dpiScale * matrix[0],
		sizePt * dpiScale * matrix[1],
		sizePt * dpiScale * matrix[2],
		sizePt * dpiScale * matrix[3],
	}
	ppemSize := int(math.Floor(math.Hypot(scaled[2], scaled[3]) + 0.5))
	if ppemSize < 1 {
		ppemSize = 1
	}
	size := float64(ppemSize)
	return fixed.Int26_6(ppemSize * 64), [4]float64{
		scaled[0] / size,
		scaled[1] / size,
		scaled[2] / size,
		scaled[3] / size,
	}
}

type glyphBounds struct {
	minX float64
	minY float64
	maxX float64
	maxY float64
	ok   bool
}

func transformedSFNTBounds(segments sfnt.Segments, matrix [4]float64, phaseX, phaseY float64) glyphBounds {
	bounds := glyphBounds{minX: math.MaxFloat64, minY: math.MaxFloat64, maxX: -math.MaxFloat64, maxY: -math.MaxFloat64}
	for _, segment := range segments {
		count := 1
		switch segment.Op {
		case sfnt.SegmentOpQuadTo:
			count = 2
		case sfnt.SegmentOpCubeTo:
			count = 3
		}
		for i := 0; i < count; i++ {
			x, y := transformSFNTPoint(segment.Args[i], matrix, phaseX, phaseY)
			bounds.minX = math.Min(bounds.minX, x)
			bounds.minY = math.Min(bounds.minY, y)
			bounds.maxX = math.Max(bounds.maxX, x)
			bounds.maxY = math.Max(bounds.maxY, y)
			bounds.ok = true
		}
	}
	return bounds
}

func glyphPathFromSFNTSegments(segments sfnt.Segments) (*entity.GlyphPath, error) {
	commands := make([]entity.PathCommand, 0, len(segments))
	var currentX, currentY float64
	var bounds glyphBounds
	bounds.minX = math.MaxFloat64
	bounds.minY = math.MaxFloat64
	bounds.maxX = -math.MaxFloat64
	bounds.maxY = -math.MaxFloat64
	addPoint := func(x, y float64) {
		bounds.minX = math.Min(bounds.minX, x)
		bounds.minY = math.Min(bounds.minY, y)
		bounds.maxX = math.Max(bounds.maxX, x)
		bounds.maxY = math.Max(bounds.maxY, y)
		bounds.ok = true
	}
	for _, segment := range segments {
		switch segment.Op {
		case sfnt.SegmentOpMoveTo:
			x, y := sfntPathPoint(segment.Args[0])
			commands = append(commands, &entity.PathMoveTo{X: x, Y: y})
			currentX, currentY = x, y
			addPoint(x, y)
		case sfnt.SegmentOpLineTo:
			x, y := sfntPathPoint(segment.Args[0])
			commands = append(commands, &entity.PathLineTo{X: x, Y: y})
			currentX, currentY = x, y
			addPoint(x, y)
		case sfnt.SegmentOpQuadTo:
			qx, qy := sfntPathPoint(segment.Args[0])
			x, y := sfntPathPoint(segment.Args[1])
			c1x := currentX + (2.0/3.0)*(qx-currentX)
			c1y := currentY + (2.0/3.0)*(qy-currentY)
			c2x := x + (2.0/3.0)*(qx-x)
			c2y := y + (2.0/3.0)*(qy-y)
			commands = append(commands, &entity.PathCurveTo{X1: c1x, Y1: c1y, X2: c2x, Y2: c2y, X3: x, Y3: y})
			currentX, currentY = x, y
			addPoint(qx, qy)
			addPoint(x, y)
		case sfnt.SegmentOpCubeTo:
			x1, y1 := sfntPathPoint(segment.Args[0])
			x2, y2 := sfntPathPoint(segment.Args[1])
			x3, y3 := sfntPathPoint(segment.Args[2])
			commands = append(commands, &entity.PathCurveTo{X1: x1, Y1: y1, X2: x2, Y2: y2, X3: x3, Y3: y3})
			currentX, currentY = x3, y3
			addPoint(x1, y1)
			addPoint(x2, y2)
			addPoint(x3, y3)
		}
	}
	if len(commands) == 0 || !bounds.ok {
		return nil, fmt.Errorf("purego sfnt: empty glyph")
	}
	return &entity.GlyphPath{
		Commands: commands,
		Bounds:   [4]float64{bounds.minX, bounds.minY, bounds.maxX, bounds.maxY},
	}, nil
}

func identityMatrix() [4]float64 {
	return [4]float64{1, 0, 0, 1}
}

func transformSFNTPoint(point fixed.Point26_6, matrix [4]float64, phaseX, phaseY float64) (float64, float64) {
	x := fixedToFloat(point.X)
	y := fixedToFloat(point.Y)
	return matrix[0]*x + matrix[2]*y + phaseX, matrix[1]*x + matrix[3]*y + phaseY
}

func sfntPathPoint(point fixed.Point26_6) (float64, float64) {
	return fixedToFloat(point.X), -fixedToFloat(point.Y)
}

func fixedToFloat(value fixed.Int26_6) float64 {
	return float64(value) / 64.0
}
