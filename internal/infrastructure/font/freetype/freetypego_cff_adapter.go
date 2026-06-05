package freetype

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	ftapi "github.com/dh-kam/freetype-go/api"
	ftcff "github.com/dh-kam/freetype-go/cff"
	ftcore "github.com/dh-kam/freetype-go/core"
	ftmath "github.com/dh-kam/freetype-go/math"
	fttype1 "github.com/dh-kam/freetype-go/type1"
)

const (
	rawCFFOpCharset    = 15
	rawCFFOpEncoding   = 16
	rawCFFOpFontBBox   = 5
	rawCFFOpFontMatrix = 12<<8 | 7
	rawCFFOpROS        = 12<<8 | 30
	rawCFFOpFDArray    = 12<<8 | 36
	rawCFFUnitsPerEm   = 1000
	rawCFFStringBias   = 391
)

type rawCFFFreeTypeGoFace struct {
	font             *ftcff.CFF
	glyphSlot        *rawCFFGlyphSlot
	xPPEM            int
	yPPEM            int
	cidToGID         map[uint32]uint32
	glyphIndexByName map[string]int
	glyphIndexByCode map[uint32]int
	glyphNameByCode  map[uint32]string
	bbox             [4]float64
	hasBBox          bool
}

type rawCFFGlyphSlot struct {
	outline ftapi.Outline
	bitmap  ftapi.Bitmap
}

type rawCFFFontMatrix struct {
	xx         int32
	yx         int32
	xy         int32
	yy         int32
	offsetX    int32
	offsetY    int32
	unitsPerEm int
}

func loadRawCFFFreeTypeGoFace(stream ftapi.Stream) (ftapi.Face, error) {
	if stream == nil || stream.Size() == 0 {
		return nil, errors.New("empty raw CFF stream")
	}
	font, err := ftcff.ParseCFF(stream, 0)
	if err != nil {
		return nil, err
	}
	if font.Major != 1 || font.CharStringsIndex.Count == 0 {
		return nil, fmt.Errorf("unsupported raw CFF major %d", font.Major)
	}
	glyphNames := rawCFFGlyphNames(font)
	glyphIndexByName := rawCFFGlyphIndexByName(glyphNames)
	glyphIndexByCode, glyphNameByCode := rawCFFEncodingByCode(font, glyphNames, glyphIndexByName)
	cidToGID, _ := rawCFFCIDToGIDMap(font)
	bbox, hasBBox := rawCFFFontBBox(font)
	return &rawCFFFreeTypeGoFace{
		font:             font,
		glyphSlot:        &rawCFFGlyphSlot{},
		xPPEM:            rawCFFUnitsPerEm,
		yPPEM:            rawCFFUnitsPerEm,
		cidToGID:         cidToGID,
		glyphIndexByName: glyphIndexByName,
		glyphIndexByCode: glyphIndexByCode,
		glyphNameByCode:  glyphNameByCode,
		bbox:             bbox,
		hasBBox:          hasBBox,
	}, nil
}

func (f *rawCFFFreeTypeGoFace) GetNumGlyphs() int {
	if f == nil || f.font == nil {
		return 0
	}
	return int(f.font.CharStringsIndex.Count)
}

func (f *rawCFFFreeTypeGoFace) SetPixelSizes(width, height int) error {
	if width < 0 || height < 0 {
		return errors.New("invalid raw CFF pixel size")
	}
	if width == 0 {
		width = height
	}
	if height == 0 {
		height = width
	}
	if width == 0 || height == 0 {
		return errors.New("invalid raw CFF pixel size")
	}
	f.xPPEM = width
	f.yPPEM = height
	return nil
}

func (f *rawCFFFreeTypeGoFace) LoadGlyph(glyphIndex int, loadFlags int) (ftapi.GlyphSlot, error) {
	if f == nil || f.font == nil {
		return nil, errors.New("nil raw CFF face")
	}
	glyphIndex = f.resolveGlyphIndex(glyphIndex)
	if glyphIndex < 0 || glyphIndex >= f.GetNumGlyphs() {
		return nil, fmt.Errorf("raw CFF glyph index %d out of range", glyphIndex)
	}
	outline, err := f.font.LoadGlyphOutline(glyphIndex)
	if err != nil {
		return nil, err
	}
	if outline == nil {
		outline = &ftcore.Outline{}
	}
	if loadFlags&ftapi.LoadNoScale == 0 {
		rawCFFScaleOutline(outline, f.font, glyphIndex, f.xPPEM, f.yPPEM)
	}
	slot := &rawCFFGlyphSlot{outline: outline}
	f.glyphSlot = slot
	return slot, nil
}

func (f *rawCFFFreeTypeGoFace) GetGlyphSlot() ftapi.GlyphSlot {
	if f == nil {
		return nil
	}
	return f.glyphSlot
}

func (f *rawCFFFreeTypeGoFace) GetUnitsPerEm() uint16 {
	return rawCFFUnitsPerEm
}

func (f *rawCFFFreeTypeGoFace) GetGlyphIndex(char rune) (int, error) {
	if f == nil || char < 0 {
		return 0, nil
	}
	glyphIndex, ok := f.glyphIndexByCode[uint32(char)]
	if !ok {
		return 0, nil
	}
	return glyphIndex, nil
}

func (f *rawCFFFreeTypeGoFace) GetGlyphIndexByName(name string) (int, bool) {
	if f == nil || name == "" {
		return 0, false
	}
	idx, ok := f.glyphIndexByName[name]
	return idx, ok
}

func (f *rawCFFFreeTypeGoFace) GetGlyphNameByCharCode(charCode uint32) (string, bool) {
	if f == nil {
		return "", false
	}
	name, ok := f.glyphNameByCode[charCode]
	return name, ok
}

func (f *rawCFFFreeTypeGoFace) CIDToGIDMap() (map[uint32]uint32, bool) {
	if f == nil {
		return nil, false
	}
	if len(f.cidToGID) > 0 {
		return f.cidToGID, true
	}
	return rawCFFCIDToGIDMap(f.font)
}

func (f *rawCFFFreeTypeGoFace) GetFaceBoundingBox() (float64, float64, float64, float64, uint16, bool) {
	if f == nil || !f.hasBBox {
		return 0, 0, 0, 0, 0, false
	}
	return f.bbox[0], f.bbox[1], f.bbox[2], f.bbox[3], rawCFFUnitsPerEm, true
}

func (f *rawCFFFreeTypeGoFace) GetGlyphMetrics(glyphIndex int) (advance int32, lsb int32, err error) {
	glyphIndex = f.resolveGlyphIndex(glyphIndex)
	if glyphIndex < 0 || glyphIndex >= f.GetNumGlyphs() {
		return 0, 0, fmt.Errorf("raw CFF glyph index %d out of range", glyphIndex)
	}
	return 0, 0, nil
}

func (f *rawCFFFreeTypeGoFace) resolveGlyphIndex(glyphIndex int) int {
	if f == nil || len(f.cidToGID) == 0 || glyphIndex < 0 {
		return glyphIndex
	}
	if localGID, ok := f.cidToGID[uint32(glyphIndex)]; ok {
		return int(localGID)
	}
	return glyphIndex
}

func (f *rawCFFFreeTypeGoFace) Shape(text string) ([]int, []ftapi.Vector) {
	glyphs := make([]int, 0, len(text))
	positions := make([]ftapi.Vector, 0, len(text))
	for _, r := range text {
		gid, _ := f.GetGlyphIndex(r)
		glyphs = append(glyphs, gid)
		positions = append(positions, ftapi.Vector{})
	}
	return glyphs, positions
}

func (f *rawCFFFreeTypeGoFace) UsesCFFOutlines() bool {
	return true
}

func (s *rawCFFGlyphSlot) GetOutline() ftapi.Outline {
	if s == nil {
		return nil
	}
	return s.outline
}

func (s *rawCFFGlyphSlot) SetOutline(outline ftapi.Outline) {
	s.outline = outline
}

func (s *rawCFFGlyphSlot) GetBitmap() ftapi.Bitmap {
	if s == nil {
		return nil
	}
	return s.bitmap
}

func (s *rawCFFGlyphSlot) GetImage() *ftapi.Image {
	return nil
}

func rawCFFScaleOutline(outline *ftcore.Outline, font *ftcff.CFF, glyphIndex int, xPPEM, yPPEM int) {
	if outline == nil {
		return
	}
	matrix, hasMatrix := rawCFFGlyphFontMatrix(font, glyphIndex)
	if hasMatrix && !rawCFFDisableFontMatrixNormalization() {
		rawCFFApplyFontMatrix(outline, matrix)
		rawCFFScaleOutlineByUnits(outline, xPPEM, yPPEM, matrix.unitsPerEm)
		return
	}
	xScale := rawCFFSizeScale(xPPEM)
	yScale := rawCFFSizeScale(yPPEM)
	for i := range outline.Points {
		outline.Points[i].X = ftmath.MulFix(rawCFFDesignUnit(outline.Points[i].X), xScale)
		outline.Points[i].Y = ftmath.MulFix(rawCFFDesignUnit(outline.Points[i].Y), yScale)
	}
}

func rawCFFGlyphFontMatrix(font *ftcff.CFF, glyphIndex int) (rawCFFFontMatrix, bool) {
	if font == nil {
		return rawCFFFontMatrix{}, false
	}
	if matrix, ok := rawCFFFontMatrixFromDict(font.TopDict); ok {
		return matrix, true
	}
	if font.FDSelect == nil || len(font.FontDicts) == 0 {
		return rawCFFFontMatrix{}, false
	}
	fdIndex, err := font.FDSelect.FDIndex(glyphIndex)
	if err != nil || fdIndex < 0 || fdIndex >= len(font.FontDicts) {
		return rawCFFFontMatrix{}, false
	}
	return rawCFFFontMatrixFromDict(font.FontDicts[fdIndex].Dict)
}

func rawCFFFontMatrixFromDict(dict map[int][]float64) (rawCFFFontMatrix, bool) {
	operands, ok := dict[rawCFFOpFontMatrix]
	if !ok || len(operands) < 6 {
		return rawCFFFontMatrix{}, false
	}
	var matrix [6]float64
	for i := range matrix {
		if math.IsInf(operands[i], 0) || math.IsNaN(operands[i]) {
			return rawCFFFontMatrix{}, false
		}
		matrix[i] = operands[i]
	}
	if rawCFFDefaultFontMatrix(matrix) {
		return rawCFFFontMatrix{}, false
	}
	return rawCFFNormalizeFontMatrix(matrix)
}

func rawCFFDefaultFontMatrix(matrix [6]float64) bool {
	const eps = 1e-12
	return math.Abs(matrix[0]-0.001) < eps &&
		math.Abs(matrix[1]) < eps &&
		math.Abs(matrix[2]) < eps &&
		math.Abs(matrix[3]-0.001) < eps &&
		math.Abs(matrix[4]) < eps &&
		math.Abs(matrix[5]) < eps
}

func rawCFFNormalizeFontMatrix(matrix [6]float64) (rawCFFFontMatrix, bool) {
	temp := math.Abs(matrix[3])
	if temp == 0 {
		temp = math.Abs(matrix[1])
	}
	if temp == 0 || math.IsInf(temp, 0) || math.IsNaN(temp) {
		return rawCFFFontMatrix{}, false
	}
	unitsPerEm := int(math.Floor((1.0 / temp) + 0.5))
	if unitsPerEm <= 0 {
		return rawCFFFontMatrix{}, false
	}

	fixed := func(v float64) int32 {
		return int32(math.Round((v / temp) * 65536.0))
	}
	offset := func(v float64) int32 {
		return int32(math.Round(v / temp))
	}
	return rawCFFFontMatrix{
		xx:         fixed(matrix[0]),
		yx:         fixed(matrix[1]),
		xy:         fixed(matrix[2]),
		yy:         fixed(matrix[3]),
		offsetX:    offset(matrix[4]),
		offsetY:    offset(matrix[5]),
		unitsPerEm: unitsPerEm,
	}, true
}

func rawCFFApplyFontMatrix(outline *ftcore.Outline, matrix rawCFFFontMatrix) {
	for i, p := range outline.Points {
		x := int64(ftmath.MulFix(p.X, matrix.xx)) + int64(ftmath.MulFix(p.Y, matrix.xy)) + int64(matrix.offsetX)
		y := int64(ftmath.MulFix(p.X, matrix.yx)) + int64(ftmath.MulFix(p.Y, matrix.yy)) + int64(matrix.offsetY)
		outline.Points[i].X = rawCFFClampInt32(x)
		outline.Points[i].Y = rawCFFClampInt32(y)
	}
}

func rawCFFClampInt32(v int64) int32 {
	const (
		maxInt32 = int64(1<<31 - 1)
		minInt32 = -1 << 31
	)
	if v > maxInt32 {
		return int32(maxInt32)
	}
	if v < minInt32 {
		return int32(minInt32)
	}
	return int32(v)
}

func rawCFFScaleOutlineFractional(outline *ftcore.Outline, xPPEM, yPPEM int) {
	rawCFFScaleOutlineByUnits(outline, xPPEM, yPPEM, rawCFFUnitsPerEm)
}

func rawCFFScaleOutlineByUnits(outline *ftcore.Outline, xPPEM, yPPEM, unitsPerEm int) {
	if rawCFFDisableFractionalScale() {
		xScale := rawCFFSizeScaleForUnits(xPPEM, unitsPerEm)
		yScale := rawCFFSizeScaleForUnits(yPPEM, unitsPerEm)
		for i := range outline.Points {
			outline.Points[i].X = ftmath.MulFix(rawCFFDesignUnit(outline.Points[i].X), xScale)
			outline.Points[i].Y = ftmath.MulFix(rawCFFDesignUnit(outline.Points[i].Y), yScale)
		}
		return
	}
	for i := range outline.Points {
		outline.Points[i].X = rawCFFScaleFractionalDesignCoord(outline.Points[i].X, xPPEM, unitsPerEm)
		outline.Points[i].Y = rawCFFScaleFractionalDesignCoord(outline.Points[i].Y, yPPEM, unitsPerEm)
	}
}

func rawCFFDisableFontMatrixNormalization() bool {
	v := strings.TrimSpace(os.Getenv("PDF_DEBUG_RAW_CFF_DISABLE_FONT_MATRIX_NORMALIZATION"))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

func rawCFFDisableFractionalScale() bool {
	v := strings.TrimSpace(os.Getenv("PDF_DEBUG_RAW_CFF_DISABLE_FRACTIONAL_SCALE"))
	return v != "" && v != "0" && !strings.EqualFold(v, "false")
}

func rawCFFScaleFractionalDesignCoord(v int32, ppem, unitsPerEm int) int32 {
	if ppem <= 0 || unitsPerEm <= 0 {
		return v
	}
	scale := int32((int64(ppem) << 16) / int64(unitsPerEm))
	return ftmath.MulFix(v, scale)
}

func rawCFFSizeScale(ppem int) int32 {
	return rawCFFSizeScaleForUnits(ppem, rawCFFUnitsPerEm)
}

func rawCFFSizeScaleForUnits(ppem, unitsPerEm int) int32 {
	if ppem <= 0 {
		return 1 << 16
	}
	if unitsPerEm <= 0 {
		unitsPerEm = rawCFFUnitsPerEm
	}
	return ftmath.DivFix(int32(ppem)<<6, int32(unitsPerEm))
}

func rawCFFDesignUnit(v int32) int32 {
	return v / 64
}

func rawCFFGlyphNames(font *ftcff.CFF) []string {
	glyphCount := 0
	if font != nil {
		glyphCount = int(font.CharStringsIndex.Count)
	}
	if glyphCount <= 0 {
		return nil
	}
	names := make([]string, glyphCount)
	names[0] = ".notdef"
	if font == nil || font.Stream == nil || font.CharStringsIndex.Count == 0 {
		return names
	}
	charsetOffset, ok := rawCFFTopDictUint(font.TopDict, rawCFFOpCharset)
	if !ok || charsetOffset <= 2 {
		return names
	}
	format, err := rawCFFReadByte(font.Stream, int64(charsetOffset))
	if err != nil {
		return names
	}
	pos := int64(charsetOffset + 1)
	gid := 1
	switch format {
	case 0:
		for gid < glyphCount {
			sid, err := rawCFFReadUint16(font.Stream, pos)
			if err != nil {
				return names
			}
			pos += 2
			rawCFFSetGlyphName(names, font, sid, gid)
			gid++
		}
	case 1:
		for gid < glyphCount {
			first, err := rawCFFReadUint16(font.Stream, pos)
			if err != nil {
				return names
			}
			left, err := rawCFFReadByte(font.Stream, pos+2)
			if err != nil {
				return names
			}
			pos += 3
			for i := 0; i <= int(left) && gid < glyphCount; i++ {
				rawCFFSetGlyphName(names, font, first+uint16(i), gid)
				gid++
			}
		}
	case 2:
		for gid < glyphCount {
			first, err := rawCFFReadUint16(font.Stream, pos)
			if err != nil {
				return names
			}
			left, err := rawCFFReadUint16(font.Stream, pos+2)
			if err != nil {
				return names
			}
			pos += 4
			for i := 0; i <= int(left) && gid < glyphCount; i++ {
				rawCFFSetGlyphName(names, font, first+uint16(i), gid)
				gid++
			}
		}
	}
	return names
}

func rawCFFGlyphIndexByName(glyphNames []string) map[string]int {
	names := map[string]int{".notdef": 0}
	for gid, name := range glyphNames {
		if name == "" {
			continue
		}
		if _, exists := names[name]; !exists {
			names[name] = gid
		}
	}
	return names
}

func rawCFFCIDToGIDMap(font *ftcff.CFF) (map[uint32]uint32, bool) {
	if font == nil || len(font.Charset) == 0 {
		return nil, false
	}
	if _, ok := font.TopDict[rawCFFOpROS]; !ok {
		if _, ok := font.TopDict[rawCFFOpFDArray]; !ok {
			return nil, false
		}
	}

	cidToGID := map[uint32]uint32{0: 0}
	for gid, name := range font.Charset {
		if gid == 0 || !strings.HasPrefix(name, "cid") {
			continue
		}
		cid, err := strconv.ParseUint(strings.TrimPrefix(name, "cid"), 10, 32)
		if err != nil {
			continue
		}
		if _, exists := cidToGID[uint32(cid)]; !exists {
			cidToGID[uint32(cid)] = uint32(gid)
		}
	}
	return cidToGID, len(cidToGID) > 1
}

func rawCFFSetGlyphName(names []string, font *ftcff.CFF, sid uint16, gid int) {
	name, ok := rawCFFStringBySID(font, sid)
	if !ok || name == "" {
		return
	}
	if gid >= 0 && gid < len(names) {
		names[gid] = name
	}
}

func rawCFFEncodingByCode(font *ftcff.CFF, glyphNames []string, glyphIndexByName map[string]int) (map[uint32]int, map[uint32]string) {
	indexByCode := map[uint32]int{}
	nameByCode := map[uint32]string{}
	if font == nil || font.Stream == nil || len(glyphNames) == 0 {
		return indexByCode, nameByCode
	}

	encodingOffset, ok := rawCFFTopDictUint(font.TopDict, rawCFFOpEncoding)
	if !ok || encodingOffset == 0 {
		rawCFFAddPredefinedEncoding(indexByCode, nameByCode, fttype1.StandardEncoding(), glyphIndexByName)
		return indexByCode, nameByCode
	}
	if encodingOffset == 1 {
		rawCFFAddPredefinedEncoding(indexByCode, nameByCode, fttype1.ExpertEncoding(), glyphIndexByName)
		return indexByCode, nameByCode
	}
	if encodingOffset < 0 {
		return indexByCode, nameByCode
	}

	formatByte, err := rawCFFReadByte(font.Stream, int64(encodingOffset))
	if err != nil {
		return indexByCode, nameByCode
	}
	format := formatByte & 0x7f
	hasSupplement := formatByte&0x80 != 0
	pos := int64(encodingOffset + 1)
	gid := 1
	switch format {
	case 0:
		nCodes, err := rawCFFReadByte(font.Stream, pos)
		if err != nil {
			return indexByCode, nameByCode
		}
		pos++
		for i := 0; i < int(nCodes) && gid < len(glyphNames); i++ {
			code, err := rawCFFReadByte(font.Stream, pos)
			if err != nil {
				return indexByCode, nameByCode
			}
			pos++
			rawCFFMapEncodingGlyph(indexByCode, nameByCode, code, glyphNames, gid)
			gid++
		}
	case 1:
		nRanges, err := rawCFFReadByte(font.Stream, pos)
		if err != nil {
			return indexByCode, nameByCode
		}
		pos++
		for i := 0; i < int(nRanges) && gid < len(glyphNames); i++ {
			first, err := rawCFFReadByte(font.Stream, pos)
			if err != nil {
				return indexByCode, nameByCode
			}
			left, err := rawCFFReadByte(font.Stream, pos+1)
			if err != nil {
				return indexByCode, nameByCode
			}
			pos += 2
			for code := int(first); code <= int(first)+int(left) && gid < len(glyphNames); code++ {
				rawCFFMapEncodingGlyph(indexByCode, nameByCode, byte(code), glyphNames, gid)
				gid++
			}
		}
	default:
		return indexByCode, nameByCode
	}

	if hasSupplement {
		nSups, err := rawCFFReadByte(font.Stream, pos)
		if err != nil {
			return indexByCode, nameByCode
		}
		pos++
		for i := 0; i < int(nSups); i++ {
			code, err := rawCFFReadByte(font.Stream, pos)
			if err != nil {
				return indexByCode, nameByCode
			}
			sid, err := rawCFFReadUint16(font.Stream, pos+1)
			if err != nil {
				return indexByCode, nameByCode
			}
			pos += 3
			name, ok := rawCFFStringBySID(font, sid)
			if !ok {
				continue
			}
			rawCFFMapEncodingName(indexByCode, nameByCode, code, name, glyphIndexByName)
		}
	}

	return indexByCode, nameByCode
}

func rawCFFAddPredefinedEncoding(indexByCode map[uint32]int, nameByCode map[uint32]string, encoding [256]string, glyphIndexByName map[string]int) {
	for code, name := range encoding {
		rawCFFMapEncodingName(indexByCode, nameByCode, byte(code), name, glyphIndexByName)
	}
}

func rawCFFMapEncodingGlyph(indexByCode map[uint32]int, nameByCode map[uint32]string, code byte, glyphNames []string, gid int) {
	if gid <= 0 || gid >= len(glyphNames) {
		return
	}
	name := glyphNames[gid]
	if name == "" || name == ".notdef" {
		return
	}
	indexByCode[uint32(code)] = gid
	nameByCode[uint32(code)] = name
}

func rawCFFMapEncodingName(indexByCode map[uint32]int, nameByCode map[uint32]string, code byte, name string, glyphIndexByName map[string]int) {
	if name == "" || name == ".notdef" {
		return
	}
	gid, ok := glyphIndexByName[name]
	if !ok || gid == 0 {
		return
	}
	indexByCode[uint32(code)] = gid
	nameByCode[uint32(code)] = name
}

func rawCFFFontBBox(font *ftcff.CFF) ([4]float64, bool) {
	if font == nil {
		return [4]float64{}, false
	}
	values, ok := font.TopDict[rawCFFOpFontBBox]
	if !ok || len(values) < 4 {
		return [4]float64{}, false
	}
	xMin, yMin, xMax, yMax := values[0], values[1], values[2], values[3]
	if xMax <= xMin || yMax <= yMin {
		return [4]float64{}, false
	}
	return [4]float64{xMin, yMin, xMax, yMax}, true
}

func rawCFFStringBySID(font *ftcff.CFF, sid uint16) (string, bool) {
	if name, ok := rawCFFStandardStringBySID(sid); ok {
		return name, true
	}
	if sid < rawCFFStringBias || font == nil {
		return "", false
	}
	data, err := font.StringIndex.Get(int(sid - rawCFFStringBias))
	if err != nil {
		return "", false
	}
	return string(data), true
}

func rawCFFStandardStringBySID(sid uint16) (string, bool) {
	switch {
	case sid >= 17 && sid <= 26:
		return []string{"zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine"}[sid-17], true
	case sid >= 34 && sid <= 59:
		return string(rune('A' + sid - 34)), true
	case sid >= 66 && sid <= 91:
		return string(rune('a' + sid - 66)), true
	}
	name, ok := rawCFFCommonStandardStrings[sid]
	return name, ok
}

var rawCFFCommonStandardStrings = map[uint16]string{
	0:   ".notdef",
	1:   "space",
	2:   "exclam",
	3:   "quotedbl",
	4:   "numbersign",
	5:   "dollar",
	6:   "percent",
	7:   "ampersand",
	8:   "quoteright",
	9:   "parenleft",
	10:  "parenright",
	11:  "asterisk",
	12:  "plus",
	13:  "comma",
	14:  "hyphen",
	15:  "period",
	16:  "slash",
	27:  "colon",
	28:  "semicolon",
	29:  "less",
	30:  "equal",
	31:  "greater",
	32:  "question",
	33:  "at",
	60:  "bracketleft",
	61:  "backslash",
	62:  "bracketright",
	63:  "asciicircum",
	64:  "underscore",
	65:  "quoteleft",
	92:  "braceleft",
	93:  "bar",
	94:  "braceright",
	95:  "asciitilde",
	96:  "exclamdown",
	97:  "cent",
	98:  "sterling",
	99:  "fraction",
	100: "yen",
	101: "florin",
	102: "section",
	103: "currency",
	104: "quotesingle",
	105: "quotedblleft",
	106: "guillemotleft",
	107: "guilsinglleft",
	108: "guilsinglright",
	109: "fi",
	110: "fl",
	111: "endash",
	112: "dagger",
	113: "daggerdbl",
	114: "periodcentered",
	115: "paragraph",
	116: "bullet",
	117: "quotesinglbase",
	118: "quotedblbase",
	119: "quotedblright",
	120: "guillemotright",
	121: "ellipsis",
	122: "perthousand",
	123: "questiondown",
	124: "grave",
	125: "acute",
	126: "circumflex",
	127: "tilde",
	128: "macron",
	129: "breve",
	130: "dotaccent",
	131: "dieresis",
	132: "ring",
	133: "cedilla",
	134: "hungarumlaut",
	135: "ogonek",
	136: "caron",
	137: "emdash",
	138: "AE",
	139: "ordfeminine",
	140: "Lslash",
	141: "Oslash",
	142: "OE",
	143: "ordmasculine",
	144: "ae",
	145: "dotlessi",
	146: "lslash",
	147: "oslash",
	148: "oe",
	149: "germandbls",
	150: "onesuperior",
	151: "logicalnot",
	152: "mu",
	153: "trademark",
	154: "Eth",
	155: "onehalf",
	156: "plusminus",
	157: "Thorn",
	158: "onequarter",
	159: "divide",
	160: "brokenbar",
	161: "degree",
	162: "thorn",
	163: "threequarters",
	164: "twosuperior",
	165: "registered",
	166: "minus",
	167: "eth",
	168: "multiply",
	169: "threesuperior",
	170: "copyright",
	171: "Aacute",
	172: "Acircumflex",
	173: "Adieresis",
	174: "Agrave",
	175: "Aring",
	176: "Atilde",
	177: "Ccedilla",
	178: "Eacute",
	179: "Ecircumflex",
	180: "Edieresis",
	181: "Egrave",
	182: "Iacute",
	183: "Icircumflex",
	184: "Idieresis",
	185: "Igrave",
	186: "Ntilde",
	187: "Oacute",
	188: "Ocircumflex",
	189: "Odieresis",
	190: "Ograve",
	191: "Otilde",
	192: "Scaron",
	193: "Uacute",
	194: "Ucircumflex",
	195: "Udieresis",
	196: "Ugrave",
	197: "Yacute",
	198: "Ydieresis",
	199: "Zcaron",
	200: "aacute",
	201: "acircumflex",
	202: "adieresis",
	203: "agrave",
	204: "aring",
	205: "atilde",
	206: "ccedilla",
	207: "eacute",
	208: "ecircumflex",
	209: "edieresis",
	210: "egrave",
	211: "iacute",
	212: "icircumflex",
	213: "idieresis",
	214: "igrave",
	215: "ntilde",
	216: "oacute",
	217: "ocircumflex",
	218: "odieresis",
	219: "ograve",
	220: "otilde",
	221: "scaron",
	222: "uacute",
	223: "ucircumflex",
	224: "udieresis",
	225: "ugrave",
	226: "yacute",
	227: "ydieresis",
	228: "zcaron",
	229: "exclamsmall",
	230: "Hungarumlautsmall",
	231: "dollaroldstyle",
	232: "dollarsuperior",
	233: "ampersandsmall",
	234: "Acutesmall",
	235: "parenleftsuperior",
	236: "parenrightsuperior",
	237: "twodotenleader",
	238: "onedotenleader",
	239: "zerooldstyle",
	240: "oneoldstyle",
	241: "twooldstyle",
	242: "threeoldstyle",
	243: "fouroldstyle",
	244: "fiveoldstyle",
	245: "sixoldstyle",
	246: "sevenoldstyle",
	247: "eightoldstyle",
	248: "nineoldstyle",
	249: "commasuperior",
	250: "threequartersemdash",
	251: "periodsuperior",
	252: "questionsmall",
	253: "asuperior",
	254: "bsuperior",
	255: "centsuperior",
	256: "dsuperior",
	257: "esuperior",
	258: "isuperior",
	259: "lsuperior",
	260: "msuperior",
	261: "nsuperior",
	262: "osuperior",
	263: "rsuperior",
	264: "ssuperior",
	265: "tsuperior",
	266: "ff",
	267: "ffi",
	268: "ffl",
}

func rawCFFTopDictUint(dict map[int][]float64, op int) (int, bool) {
	values, ok := dict[op]
	if !ok || len(values) == 0 {
		return 0, false
	}
	return int(values[0]), true
}

func rawCFFReadByte(stream ftapi.Stream, off int64) (byte, error) {
	buf := []byte{0}
	n, err := stream.ReadAt(buf, off)
	if err != nil && n == 0 {
		return 0, err
	}
	if n != 1 {
		return 0, errors.New("short raw CFF read")
	}
	return buf[0], nil
}

func rawCFFReadUint16(stream ftapi.Stream, off int64) (uint16, error) {
	buf := make([]byte, 2)
	n, err := stream.ReadAt(buf, off)
	if err != nil && n == 0 {
		return 0, err
	}
	if n != 2 {
		return 0, errors.New("short raw CFF uint16 read")
	}
	return binary.BigEndian.Uint16(buf), nil
}
