package freetype

import (
	"fmt"
	"math"
	"os"

	ftapi "github.com/dh-kam/freetype-go/api"
	ftcore "github.com/dh-kam/freetype-go/core"
	fthelper "github.com/dh-kam/freetype-go/helper"
	ftraster "github.com/dh-kam/freetype-go/raster"
	ftsfnt "github.com/dh-kam/freetype-go/sfnt"
	fttype1 "github.com/dh-kam/freetype-go/type1"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func useFreeTypeGoAdapter() bool {
	switch os.Getenv("PDF_FREETYPE_GO") {
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	default:
		return true
	}
}

func useFreeTypeGoType1Adapter() bool {
	switch os.Getenv("PDF_FREETYPE_GO_TYPE1") {
	case "0", "false", "FALSE", "no", "NO", "off", "OFF":
		return false
	case "1", "true", "TRUE", "yes", "YES", "on", "ON":
		return true
	default:
		return true
	}
}

func renderGlyphBitmapByIndexFreeTypeGo(fontData []byte, glyphIndex uint32, sizePt float64, matrix [4]float64, phaseX, phaseY float64, dpi int, normalizeMatrix bool, floorPhase bool) ([]byte, int, int, int, int, bool, error) {
	if !useFreeTypeGoAdapter() {
		return nil, 0, 0, 0, 0, false, nil
	}
	ppem, rasterMatrix := legacySFNTBitmapMatrix(sizePt, matrix, dpi)
	if normalizeMatrix {
		ppem, rasterMatrix = popplerSFNTBitmapMatrix(sizePt, matrix, dpi)
	}
	sizePx := int(ppem >> 6)
	if sizePx <= 0 {
		return nil, 0, 0, 0, 0, true, fmt.Errorf("invalid glyph ppem")
	}
	face, err := loadFreeTypeGoFace(fontData)
	if err != nil {
		return nil, 0, 0, 0, 0, false, nil
	}
	if err := face.SetPixelSizes(0, sizePx); err != nil {
		return nil, 0, 0, 0, 0, false, nil
	}
	slot, err := face.LoadGlyph(int(glyphIndex), ftapi.LoadNoBitmap|ftapi.LoadNoHinting)
	if err != nil {
		return nil, 0, 0, 0, 0, false, nil
	}
	outline := cloneFreeTypeGoOutline(slot.GetOutline())
	if outline == nil || len(outline.Points) == 0 {
		return nil, 0, 0, 0, 0, true, nil
	}
	transformFreeTypeGoOutline(outline, rasterMatrix, phaseX, phaseY, floorPhase)

	renderOutline, bitmap, _, ok := ftcore.PrepareBitmapForOutline(outline, -1, ftapi.RenderModeNormal)
	if !ok || bitmap == nil {
		return nil, 0, 0, 0, 0, true, nil
	}
	if renderOutline != nil {
		rasterizer := ftraster.NewSmoothRasterizer()
		if useFreeTypeGoFillRule(face) {
			rasterizer.SetFreeTypeFillRule(true)
		}
		if err := rasterizer.Render(renderOutline, bitmap); err != nil {
			return nil, 0, 0, 0, 0, true, err
		}
	}
	return copyFreeTypeGoGrayBitmap(bitmap), bitmap.GetWidth(), bitmap.GetRows(), bitmap.GetLeft(), bitmap.GetTop(), true, nil
}

func renderGlyphPathByIndexFreeTypeGo(fontData []byte, glyphIndex uint32, sizePt float64, matrix [4]float64) (*entity.GlyphPath, bool, error) {
	if !useFreeTypeGoAdapter() {
		return nil, false, nil
	}
	ppem, rasterMatrix := popplerSFNTBitmapMatrix(sizePt, matrix, 72)
	sizePx := int(ppem >> 6)
	if sizePx <= 0 {
		return nil, true, fmt.Errorf("invalid glyph ppem")
	}
	face, err := loadFreeTypeGoFace(fontData)
	if err != nil {
		return nil, false, nil
	}
	if err := face.SetPixelSizes(0, sizePx); err != nil {
		return nil, true, err
	}
	slot, err := face.LoadGlyph(int(glyphIndex), ftapi.LoadNoBitmap|ftapi.LoadNoHinting)
	if err != nil {
		return nil, true, err
	}
	outline := cloneFreeTypeGoOutline(slot.GetOutline())
	if outline == nil || len(outline.Points) == 0 {
		return nil, true, nil
	}
	var path *entity.GlyphPath
	if os.Getenv("PDF_DEBUG_FTGO_PATH_FIXED_TRANSFORM") == "1" {
		transformFreeTypeGoOutline(outline, rasterMatrix, 0, 0, true)
		path = freeTypeGoOutlineToGlyphPath(outline)
	} else {
		path = freeTypeGoOutlineToPopplerTextGlyphPath(outline, rasterMatrix)
	}
	if path == nil || len(path.Commands) == 0 {
		return nil, true, nil
	}
	debugDumpFreeTypeGoGlyphPath(glyphIndex, path)
	return path, true, nil
}

func renderGlyphPathByIndexTextMatrixFreeTypeGo(fontData []byte, glyphIndex uint32, sizePt float64, matrix [4]float64) (*entity.GlyphPath, bool, error) {
	if !useFreeTypeGoAdapter() {
		return nil, false, nil
	}
	ppem, rasterMatrix := popplerSFNTBitmapMatrix(sizePt, matrix, 72)
	sizePx := int(ppem >> 6)
	if sizePx <= 0 {
		return nil, true, fmt.Errorf("invalid glyph ppem")
	}
	textScale := sizePt / float64(sizePx)
	if textScale <= 0 {
		return nil, true, fmt.Errorf("invalid glyph text scale")
	}
	face, err := loadFreeTypeGoFace(fontData)
	if err != nil {
		return nil, false, nil
	}
	if err := face.SetPixelSizes(0, sizePx); err != nil {
		return nil, true, err
	}
	slot, err := face.LoadGlyph(int(glyphIndex), ftapi.LoadNoBitmap|ftapi.LoadNoHinting)
	if err != nil {
		return nil, true, err
	}
	outline := cloneFreeTypeGoOutline(slot.GetOutline())
	if outline == nil || len(outline.Points) == 0 {
		return nil, true, nil
	}
	path := freeTypeGoOutlineToPopplerTextGlyphPathWithScale(outline, rasterMatrix, textScale)
	if path == nil || len(path.Commands) == 0 {
		return nil, true, nil
	}
	debugDumpFreeTypeGoGlyphPath(glyphIndex, path)
	return path, true, nil
}

func debugDumpFreeTypeGoGlyphPath(glyphIndex uint32, path *entity.GlyphPath) {
	filter := os.Getenv("PDF_DEBUG_FTGO_PATH_DUMP_GLYPH")
	if filter == "" || path == nil {
		return
	}
	if filter != "*" && filter != fmt.Sprintf("%d", glyphIndex) {
		return
	}
	for i, cmd := range path.Commands {
		switch v := cmd.(type) {
		case *entity.PathMoveTo:
			fmt.Fprintf(os.Stderr, "FTGO_PATH_DUMP glyph=%d index=%d op=M x=%.17g y=%.17g\n", glyphIndex, i, v.X, v.Y)
		case *entity.PathLineTo:
			fmt.Fprintf(os.Stderr, "FTGO_PATH_DUMP glyph=%d index=%d op=L x=%.17g y=%.17g\n", glyphIndex, i, v.X, v.Y)
		case *entity.PathCurveTo:
			fmt.Fprintf(os.Stderr, "FTGO_PATH_DUMP glyph=%d index=%d op=C x1=%.17g y1=%.17g x2=%.17g y2=%.17g x3=%.17g y3=%.17g\n",
				glyphIndex, i, v.X1, v.Y1, v.X2, v.Y2, v.X3, v.Y3)
		case *entity.PathClose:
			fmt.Fprintf(os.Stderr, "FTGO_PATH_DUMP glyph=%d index=%d op=Z\n", glyphIndex, i)
		}
	}
}

func freeTypeGoOutlineToPopplerTextGlyphPath(outline *ftcore.Outline, matrix [4]float64) *entity.GlyphPath {
	scale := freeTypeGoOutlinePopplerScale(matrix)
	if scale <= 0 {
		return nil
	}
	return freeTypeGoOutlineToPopplerTextGlyphPathWithScale(outline, matrix, scale)
}

func freeTypeGoOutlineToPopplerTextGlyphPathWithScale(outline *ftcore.Outline, matrix [4]float64, pathScale float64) *entity.GlyphPath {
	scale := math.Hypot(matrix[2], matrix[3])
	if scale <= 0 {
		scale = math.Hypot(matrix[0], matrix[1])
	}
	if scale <= 0 {
		return nil
	}
	unitMatrix := [4]float64{
		matrix[0] / scale,
		matrix[1] / scale,
		matrix[2] / scale,
		matrix[3] / scale,
	}
	transformFreeTypeGoOutline(outline, unitMatrix, 0, 0, false)
	return freeTypeGoOutlineToGlyphPathWithScale(outline, pathScale)
}

func freeTypeGoOutlinePopplerScale(matrix [4]float64) float64 {
	scale := math.Hypot(matrix[2], matrix[3])
	if scale <= 0 {
		scale = math.Hypot(matrix[0], matrix[1])
	}
	return scale
}

func useFreeTypeGoFillRule(face ftapi.Face) bool {
	// Poppler's SplashFTFont renders all FreeType outlines through ftgrays'
	// default non-zero fill rule. That rule quantizes negative coverage with
	// `coverage = ~coverage`, which differs by one alpha level from taking the
	// absolute area before shifting. Use the FreeType-compatible rule for
	// TrueType as well as Type1/CFF outlines.
	_ = face
	return true
}

func getGlyphIndexByCharCodeFreeTypeGo(fontData []byte, charCode uint32) (uint32, bool, bool) {
	if !useFreeTypeGoAdapter() {
		return 0, false, false
	}
	face, err := loadFreeTypeGoFace(fontData)
	if err != nil {
		return 0, false, false
	}
	glyphIndex, err := face.GetGlyphIndex(rune(charCode))
	if err != nil || glyphIndex == 0 {
		return 0, false, true
	}
	return uint32(glyphIndex), true, true
}

type freeTypeGoGlyphNameIndexer interface {
	GetGlyphIndexByName(name string) (int, bool)
}

type freeTypeGoGlyphNameByCharCoder interface {
	GetGlyphNameByCharCode(charCode uint32) (string, bool)
}

type freeTypeGoCIDToGIDMapper interface {
	CIDToGIDMap() (map[uint32]uint32, bool)
}

type freeTypeGoFaceBBoxProvider interface {
	GetFaceBoundingBox() (float64, float64, float64, float64, uint16, bool)
}

func getGlyphIndexByNameFreeTypeGo(fontData []byte, glyphName string) (uint32, bool, bool) {
	if !useFreeTypeGoAdapter() {
		return 0, false, false
	}
	face, err := loadFreeTypeGoFace(fontData)
	if err != nil {
		return 0, false, false
	}
	indexer, ok := face.(freeTypeGoGlyphNameIndexer)
	if !ok {
		return 0, false, false
	}
	glyphIndex, ok := indexer.GetGlyphIndexByName(glyphName)
	if !ok || glyphName == ".notdef" {
		return 0, false, true
	}
	return uint32(glyphIndex), true, true
}

func getGlyphNameByCharCodeFreeTypeGo(fontData []byte, charCode uint32) (string, bool, bool) {
	if !useFreeTypeGoAdapter() {
		return "", false, false
	}
	face, err := loadFreeTypeGoFace(fontData)
	if err != nil {
		return "", false, false
	}
	indexer, ok := face.(freeTypeGoGlyphNameByCharCoder)
	if !ok {
		return "", false, false
	}
	name, ok := indexer.GetGlyphNameByCharCode(charCode)
	if !ok || name == ".notdef" {
		return "", false, true
	}
	return name, true, true
}

func getCIDToGIDMapFreeTypeGo(fontData []byte) (map[uint32]uint32, bool, bool) {
	if !useFreeTypeGoAdapter() {
		return nil, false, false
	}
	face, err := loadFreeTypeGoFace(fontData)
	if err != nil {
		return nil, false, false
	}
	mapper, ok := face.(freeTypeGoCIDToGIDMapper)
	if !ok {
		return nil, false, false
	}
	cidToGID, ok := mapper.CIDToGIDMap()
	return cidToGID, ok, true
}

func getFaceBoundingBoxFreeTypeGo(fontData []byte) (float64, float64, float64, float64, uint16, bool, bool) {
	if !useFreeTypeGoAdapter() {
		return 0, 0, 0, 0, 0, false, false
	}
	face, err := loadFreeTypeGoFace(fontData)
	if err != nil {
		return 0, 0, 0, 0, 0, false, false
	}
	provider, ok := face.(freeTypeGoFaceBBoxProvider)
	if !ok {
		return 0, 0, 0, 0, 0, false, false
	}
	xMin, yMin, xMax, yMax, units, ok := provider.GetFaceBoundingBox()
	return xMin, yMin, xMax, yMax, units, ok, true
}

func loadFreeTypeGoFace(fontData []byte) (ftapi.Face, error) {
	if len(fontData) == 0 {
		return nil, fmt.Errorf("empty font data")
	}
	sys := ftcore.NewSystem()
	stream := freeTypeGoStream(fontData)
	if face, err := ftsfnt.LoadFaceIndex(sys, stream, 0); err == nil {
		return face, nil
	}
	if face, err := loadRawCFFFreeTypeGoFace(ftcore.NewMemoryStream(fontData)); err == nil {
		return face, nil
	}
	if !useFreeTypeGoType1Adapter() {
		return nil, fmt.Errorf("freetype-go Type1 adapter disabled")
	}
	return fttype1.NewLoader(sys).LoadFace(ftcore.NewMemoryStream(fontData))
}

func freeTypeGoStream(fontData []byte) ftapi.Stream {
	var stream ftapi.Stream = ftcore.NewMemoryStream(fontData)
	if decoded, err := fthelper.DecodeWOFFIfNeeded(stream); err == nil {
		return decoded
	}
	return stream
}

func cloneFreeTypeGoOutline(outline ftapi.Outline) *ftcore.Outline {
	if outline == nil {
		return nil
	}
	points := outline.GetPoints()
	tags := outline.GetTags()
	contours := outline.GetContours()
	return &ftcore.Outline{
		Points:   append([]ftapi.Vector(nil), points...),
		Tags:     append([]byte(nil), tags...),
		Contours: append([]int(nil), contours...),
		Flags:    ftcore.OutlineFlags(outline),
	}
}

func transformFreeTypeGoOutline(outline *ftcore.Outline, matrix [4]float64, phaseX, phaseY float64, floorPhase bool) {
	if outline == nil {
		return
	}
	xx := freeTypeGoFixed(matrix[0])
	yx := freeTypeGoFixed(matrix[1])
	xy := freeTypeGoFixed(matrix[2])
	yy := freeTypeGoFixed(matrix[3])
	phaseX26Dot6 := freeTypeGoPhase26Dot6(phaseX, floorPhase)
	phaseY26Dot6 := freeTypeGoPhase26Dot6(-phaseY, floorPhase)
	for i, p := range outline.Points {
		outline.Points[i].X = freeTypeGoMulFix(p.X, xx) + freeTypeGoMulFix(p.Y, xy) + phaseX26Dot6
		outline.Points[i].Y = freeTypeGoMulFix(p.X, yx) + freeTypeGoMulFix(p.Y, yy) + phaseY26Dot6
	}
}

func freeTypeGoFixed(v float64) int32 {
	return int32(v * 65536.0)
}

func freeTypeGoPhase26Dot6(v float64, floorPhase bool) int32 {
	if floorPhase {
		return int32(math.Floor(v * 64.0))
	}
	return int32(math.Round(v * 64.0))
}

func freeTypeGoMulFix(a, b int32) int32 {
	ret := int64(a) * int64(b)
	ret += 0x8000 + (ret >> 63)
	return int32(ret >> 16)
}

func copyFreeTypeGoGrayBitmap(bitmap ftapi.Bitmap) []byte {
	if bitmap == nil || bitmap.GetRows() <= 0 || bitmap.GetWidth() <= 0 {
		return nil
	}
	width := bitmap.GetWidth()
	rows := bitmap.GetRows()
	pitch := bitmap.GetPitch()
	src := bitmap.GetBuffer()
	out := make([]byte, width*rows)
	for y := 0; y < rows; y++ {
		srcOff := y * pitch
		dstOff := y * width
		if srcOff < 0 || srcOff >= len(src) {
			break
		}
		n := width
		if srcOff+n > len(src) {
			n = len(src) - srcOff
		}
		copy(out[dstOff:dstOff+n], src[srcOff:srcOff+n])
	}
	return out
}

func freeTypeGoOutlineToGlyphPath(outline *ftcore.Outline) *entity.GlyphPath {
	return freeTypeGoOutlineToGlyphPathWithPoint(outline, func(point ftapi.Vector) (float64, float64) {
		return float64(point.X) / 64.0, -float64(point.Y) / 64.0
	})
}

func freeTypeGoOutlineToGlyphPathWithScale(outline *ftcore.Outline, scale float64) *entity.GlyphPath {
	return freeTypeGoOutlineToGlyphPathWithPoint(outline, func(point ftapi.Vector) (float64, float64) {
		return float64(point.X) * scale / 64.0, -float64(point.Y) * scale / 64.0
	})
}

func freeTypeGoOutlineToGlyphPathWithMatrix(outline *ftcore.Outline, matrix [4]float64) *entity.GlyphPath {
	return freeTypeGoOutlineToGlyphPathWithPoint(outline, func(point ftapi.Vector) (float64, float64) {
		x := float64(point.X) / 64.0
		y := float64(point.Y) / 64.0
		return x*matrix[0] + y*matrix[2], -(x*matrix[1] + y*matrix[3])
	})
}

func freeTypeGoOutlineToGlyphPathWithPoint(outline *ftcore.Outline, point func(ftapi.Vector) (float64, float64)) *entity.GlyphPath {
	if outline == nil || len(outline.Points) == 0 {
		return nil
	}
	points := outline.Points
	tags := outline.Tags
	contours := outline.Contours
	if len(tags) < len(points) {
		return nil
	}
	if point == nil {
		return nil
	}
	pointAt := func(i int) (float64, float64) {
		return point(points[i])
	}
	midpoint := func(a, b ftapi.Vector) (float64, float64) {
		// FT_Outline_Decompose computes virtual conic on-points in 26.6
		// integer space before callbacks see scaled coordinates.
		return point(ftapi.Vector{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2})
	}
	path := &entity.GlyphPath{
		Commands: make([]entity.PathCommand, 0, len(points)+len(contours)),
		Bounds:   [4]float64{0, 0, 0, 0},
	}
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
	moveTo := func(x, y float64) {
		path.Commands = append(path.Commands, &entity.PathMoveTo{X: x, Y: y})
		updateBounds(x, y)
	}
	lineTo := func(x, y float64) {
		path.Commands = append(path.Commands, &entity.PathLineTo{X: x, Y: y})
		updateBounds(x, y)
	}
	curveTo := func(x1, y1, x2, y2, x3, y3 float64) {
		path.Commands = append(path.Commands, &entity.PathCurveTo{X1: x1, Y1: y1, X2: x2, Y2: y2, X3: x3, Y3: y3})
		updateBounds(x1, y1)
		updateBounds(x2, y2)
		updateBounds(x3, y3)
	}
	quadTo := func(cx0, cy0 *float64, ctrlX, ctrlY, endX, endY float64) {
		c1x := (*cx0 + 2*ctrlX) / 3
		c1y := (*cy0 + 2*ctrlY) / 3
		c2x := (2*ctrlX + endX) / 3
		c2y := (2*ctrlY + endY) / 3
		curveTo(c1x, c1y, c2x, c2y, endX, endY)
		*cx0, *cy0 = endX, endY
	}
	tagKind := func(i int) byte {
		return tags[i] & 0x03
	}
	const (
		conicTag = byte(0)
		onTag    = byte(1)
		cubicTag = byte(2)
	)
	start := 0
	for _, end := range contours {
		if end < start || end >= len(points) {
			start = end + 1
			continue
		}
		firstX, firstY := pointAt(start)
		lastX, lastY := pointAt(end)
		var curX, curY, contourStartX, contourStartY float64
		i := start
		switch {
		case tagKind(start) == onTag:
			moveTo(firstX, firstY)
			curX, curY = firstX, firstY
			contourStartX, contourStartY = firstX, firstY
			i = start + 1
		case tagKind(end) == onTag:
			moveTo(lastX, lastY)
			curX, curY = lastX, lastY
			contourStartX, contourStartY = lastX, lastY
		default:
			midX, midY := midpoint(points[start], points[end])
			moveTo(midX, midY)
			curX, curY = midX, midY
			contourStartX, contourStartY = midX, midY
		}
		for i <= end {
			switch tagKind(i) {
			case onTag:
				x, y := pointAt(i)
				lineTo(x, y)
				curX, curY = x, y
				i++
			case conicTag:
				ctrlX, ctrlY := pointAt(i)
				if i == end {
					quadTo(&curX, &curY, ctrlX, ctrlY, contourStartX, contourStartY)
					i++
					continue
				}
				switch tagKind(i + 1) {
				case onTag:
					nextX, nextY := pointAt(i + 1)
					quadTo(&curX, &curY, ctrlX, ctrlY, nextX, nextY)
					i += 2
				case conicTag:
					midX, midY := midpoint(points[i], points[i+1])
					quadTo(&curX, &curY, ctrlX, ctrlY, midX, midY)
					i++
				default:
					lineTo(ctrlX, ctrlY)
					curX, curY = ctrlX, ctrlY
					i++
				}
			case cubicTag:
				if i+2 > end {
					i = end + 1
					continue
				}
				c1x, c1y := pointAt(i)
				c2x, c2y := pointAt(i + 1)
				endX, endY := pointAt(i + 2)
				curveTo(c1x, c1y, c2x, c2y, endX, endY)
				curX, curY = endX, endY
				i += 3
			default:
				i++
			}
		}
		path.Commands = append(path.Commands, &entity.PathClose{})
		start = end + 1
	}
	if !hasPoint {
		return nil
	}
	path.Bounds = [4]float64{minX, minY, maxX, maxY}
	return path
}
