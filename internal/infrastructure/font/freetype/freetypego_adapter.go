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

func useFreeTypeGoFillRule(face ftapi.Face) bool {
	if _, ok := face.(*fttype1.Face); ok {
		return true
	}
	if sfntFace, ok := face.(interface{ UsesCFFOutlines() bool }); ok {
		return sfntFace.UsesCFFOutlines()
	}
	return false
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
