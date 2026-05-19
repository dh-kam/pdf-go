package renderer

import (
	"encoding/binary"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/cff"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/type1"
)

func (e *Evaluator) applyFontEncodingFromDict(dict *entity.Dict, font entity.Font) entity.Font {
	if dict == nil || font == nil || font.IsCIDFont() {
		return font
	}

	encodingMap := e.resolveSimpleFontEncoding(dict.Get(entity.Name("Encoding")))
	if len(encodingMap) == 0 {
		encodingMap = e.resolveEmbeddedType1Encoding(dict)
	}
	if len(encodingMap) == 0 {
		return font
	}

	glyphByCode := map[uint32]uint32{}
	nameByCode := map[uint32]string{}
	glyphByName := map[string]uint32{}
	if namedFont, ok := font.(glyphIDByNameFont); ok {
		for code, name := range encodingMap {
			for _, candidate := range encodingGlyphNameCandidates(name) {
				glyph, found := namedFont.GlyphIDByName(candidate)
				if !found {
					continue
				}
				glyphByCode[uint32(code)] = glyph
				nameByCode[uint32(code)] = candidate
				break
			}
		}
	}
	for code := uint32(0); code <= 255; code++ {
		glyph, err := font.CharCodeToGlyph(code)
		if err != nil {
			continue
		}
		name := font.GlyphName(glyph)
		if name == "" || name == ".notdef" {
			continue
		}
		if _, exists := glyphByName[name]; !exists {
			glyphByName[name] = glyph
		}
	}

	if len(glyphByName) == 0 {
		if len(glyphByCode) == 0 {
			return font
		}
	}

	for code, name := range encodingMap {
		if _, ok := nameByCode[uint32(code)]; ok {
			continue
		}
		for _, candidate := range encodingGlyphNameCandidates(name) {
			glyph, ok := glyphByName[candidate]
			if !ok {
				continue
			}
			glyphByCode[uint32(code)] = glyph
			nameByCode[uint32(code)] = candidate
			break
		}
	}

	if len(glyphByCode) == 0 {
		return font
	}

	return &encodedFont{
		base:        font,
		glyphByCode: glyphByCode,
		nameByCode:  nameByCode,
	}
}

func (e *Evaluator) resolveEmbeddedType1Encoding(dict *entity.Dict) map[byte]string {
	if dict == nil {
		return nil
	}
	if nameValueForEncoding(dict.Get(entity.Name("Subtype"))) != "Type1" {
		return nil
	}

	fontData, err := e.getEmbeddedFontData(dict)
	if err != nil || len(fontData) == 0 {
		return nil
	}

	if looksLikeCFFEmbeddedFont(fontData) {
		if out := embeddedCFFEncodingNames(fontData); len(out) > 0 {
			return out
		}
	}

	font, err := type1.NewFontFromBytes(fontData)
	if err != nil {
		return nil
	}

	encoded, ok := any(font).(encodingNameFont)
	if !ok {
		return nil
	}

	out := map[byte]string{}
	for code := 0; code <= 255; code++ {
		if name := encoded.EncodingName(byte(code)); name != "" {
			out[byte(code)] = name
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func embeddedCFFEncodingNames(fontData []byte) map[byte]string {
	rawEncoding := parseEmbeddedCFFEncodingNames(fontData)
	font, err := cff.NewFont(fontData)
	if err != nil {
		return nil
	}

	// Poppler uses FoFiType1C's built-in encoding and fills empty slots from
	// StandardEncoding for simple embedded Type1C fonts.
	out := simpleASCIIEncodingNames()
	for code, name := range rawEncoding {
		out[code] = name
	}
	if encoded, ok := any(font).(encodingNameFont); ok {
		for code := 0; code <= 255; code++ {
			name := encoded.EncodingName(byte(code))
			if name == "" || name == ".notdef" {
				continue
			}
			out[byte(code)] = name
		}
	}
	if len(rawEncoding) == 0 {
		if _, ok := any(font).(encodingNameFont); !ok {
			return nil
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

type cffIndexData struct {
	objects [][]byte
	end     int
}

func parseEmbeddedCFFEncodingNames(data []byte) map[byte]string {
	if len(data) < 4 || !looksLikeCFFEmbeddedFont(data) {
		return nil
	}

	pos := int(data[2])
	if pos > len(data) {
		return nil
	}
	nameIndex, ok := readCFFIndexData(data, pos)
	if !ok {
		return nil
	}
	topDictIndex, ok := readCFFIndexData(data, nameIndex.end)
	if !ok || len(topDictIndex.objects) == 0 {
		return nil
	}
	stringIndex, ok := readCFFIndexData(data, topDictIndex.end)
	if !ok {
		return nil
	}

	topDict := parseCFFTopDictOffsets(topDictIndex.objects[0])
	encodingOffset, hasEncoding := topDict[16]
	charsetOffset, hasCharset := topDict[15]
	charStringsOffset, hasCharStrings := topDict[17]
	if !hasEncoding || !hasCharset || !hasCharStrings {
		return nil
	}

	charStrings, ok := readCFFIndexData(data, charStringsOffset)
	if !ok || len(charStrings.objects) == 0 {
		return nil
	}
	charset := parseCFFCharsetNames(data, charsetOffset, len(charStrings.objects), stringIndex.objects)
	if len(charset) == 0 {
		return nil
	}
	return parseCFFEncodingNames(data, encodingOffset, charset)
}

func readCFFIndexData(data []byte, pos int) (cffIndexData, bool) {
	if pos < 0 || pos+2 > len(data) {
		return cffIndexData{}, false
	}
	count := int(binary.BigEndian.Uint16(data[pos : pos+2]))
	pos += 2
	if count == 0 {
		return cffIndexData{end: pos}, true
	}
	if pos >= len(data) {
		return cffIndexData{}, false
	}
	offSize := int(data[pos])
	pos++
	if offSize < 1 || offSize > 4 || pos+(count+1)*offSize > len(data) {
		return cffIndexData{}, false
	}

	offsets := make([]int, count+1)
	for i := range offsets {
		offsets[i] = readCFFOffset(data[pos:pos+offSize], offSize)
		pos += offSize
	}
	if offsets[0] != 1 || offsets[count] < offsets[0] {
		return cffIndexData{}, false
	}

	dataStart := pos
	dataEnd := dataStart + offsets[count] - 1
	if dataEnd > len(data) {
		return cffIndexData{}, false
	}

	objects := make([][]byte, 0, count)
	for i := 0; i < count; i++ {
		start := dataStart + offsets[i] - 1
		end := dataStart + offsets[i+1] - 1
		if start < dataStart || end < start || end > dataEnd {
			return cffIndexData{}, false
		}
		objects = append(objects, data[start:end])
	}
	return cffIndexData{objects: objects, end: dataEnd}, true
}

func readCFFOffset(data []byte, offSize int) int {
	out := 0
	for i := 0; i < offSize; i++ {
		out = (out << 8) | int(data[i])
	}
	return out
}

func parseCFFTopDictOffsets(data []byte) map[int]int {
	out := map[int]int{}
	stack := []int{}
	for i := 0; i < len(data); {
		b := data[i]
		if b <= 21 {
			op := int(b)
			i++
			if b == 12 {
				if i >= len(data) {
					return out
				}
				op = 1200 + int(data[i])
				i++
			}
			if len(stack) > 0 {
				out[op] = stack[len(stack)-1]
			}
			stack = stack[:0]
			continue
		}

		value, ok, next := readCFFDictInteger(data, i)
		if next <= i {
			return out
		}
		i = next
		if ok {
			stack = append(stack, value)
		}
	}
	return out
}

func readCFFDictInteger(data []byte, pos int) (int, bool, int) {
	if pos >= len(data) {
		return 0, false, pos
	}
	b := data[pos]
	switch {
	case b >= 32 && b <= 246:
		return int(b) - 139, true, pos + 1
	case b >= 247 && b <= 250:
		if pos+1 >= len(data) {
			return 0, false, len(data)
		}
		return (int(b)-247)*256 + int(data[pos+1]) + 108, true, pos + 2
	case b >= 251 && b <= 254:
		if pos+1 >= len(data) {
			return 0, false, len(data)
		}
		return -(int(b)-251)*256 - int(data[pos+1]) - 108, true, pos + 2
	case b == 28:
		if pos+2 >= len(data) {
			return 0, false, len(data)
		}
		return int(int16(binary.BigEndian.Uint16(data[pos+1 : pos+3]))), true, pos + 3
	case b == 29:
		if pos+4 >= len(data) {
			return 0, false, len(data)
		}
		return int(int32(binary.BigEndian.Uint32(data[pos+1 : pos+5]))), true, pos + 5
	case b == 30:
		next := pos + 1
		for next < len(data) {
			current := data[next]
			next++
			if current>>4 == 0x0f || current&0x0f == 0x0f {
				break
			}
		}
		return 0, false, next
	default:
		return 0, false, pos + 1
	}
}

func parseCFFCharsetNames(data []byte, offset int, glyphCount int, strings [][]byte) []string {
	if glyphCount <= 0 {
		return nil
	}
	names := make([]string, glyphCount)
	names[0] = ".notdef"
	if offset <= 2 || offset >= len(data) {
		return names
	}

	format := data[offset]
	pos := offset + 1
	gid := 1
	switch format {
	case 0:
		for gid < glyphCount && pos+2 <= len(data) {
			sid := int(binary.BigEndian.Uint16(data[pos : pos+2]))
			pos += 2
			names[gid] = cffSIDName(sid, strings)
			gid++
		}
	case 1, 2:
		for gid < glyphCount && pos+3 <= len(data) {
			firstSID := int(binary.BigEndian.Uint16(data[pos : pos+2]))
			pos += 2
			nLeft := int(data[pos])
			pos++
			if format == 2 {
				if pos+2 > len(data) {
					break
				}
				nLeft = int(binary.BigEndian.Uint16(data[pos : pos+2]))
				pos += 2
			}
			for sid := firstSID; sid <= firstSID+nLeft && gid < glyphCount; sid++ {
				names[gid] = cffSIDName(sid, strings)
				gid++
			}
		}
	}
	return names
}

func parseCFFEncodingNames(data []byte, offset int, charset []string) map[byte]string {
	if offset < 0 || offset >= len(data) || len(charset) == 0 {
		return nil
	}
	format := data[offset] & 0x7f
	pos := offset + 1
	gid := 1
	out := map[byte]string{}
	switch format {
	case 0:
		if pos >= len(data) {
			return nil
		}
		nCodes := int(data[pos])
		pos++
		for i := 0; i < nCodes && gid < len(charset) && pos < len(data); i++ {
			if name := charset[gid]; name != "" && name != ".notdef" {
				out[data[pos]] = name
			}
			pos++
			gid++
		}
	case 1:
		if pos >= len(data) {
			return nil
		}
		nRanges := int(data[pos])
		pos++
		for i := 0; i < nRanges && gid < len(charset) && pos+2 <= len(data); i++ {
			first := data[pos]
			nLeft := int(data[pos+1])
			pos += 2
			for code := int(first); code <= int(first)+nLeft && gid < len(charset); code++ {
				if name := charset[gid]; name != "" && name != ".notdef" {
					out[byte(code)] = name
				}
				gid++
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cffSIDName(sid int, strings [][]byte) string {
	if sid == 0 {
		return ".notdef"
	}
	if name, ok := cffStandardStringBySID[sid]; ok {
		return name
	}
	custom := sid - 391
	if custom >= 0 && custom < len(strings) {
		return string(strings[custom])
	}
	return ""
}

var cffStandardStringBySID = map[int]string{
	166: "intersection",
}

func nameValueForEncoding(obj entity.Object) string {
	if name, ok := obj.(entity.Name); ok {
		return name.Value()
	}
	return ""
}

func (e *Evaluator) resolveSimpleFontEncoding(obj entity.Object) map[byte]string {
	resolved := e.resolveDirectObject(obj)
	switch v := resolved.(type) {
	case nil:
		return nil
	case entity.Name:
		return simpleEncodingBaseNames(v.Value())
	case *entity.Dict:
		base := simpleEncodingBaseNames("")
		if baseName, ok := v.Get(entity.Name("BaseEncoding")).(entity.Name); ok {
			base = simpleEncodingBaseNames(baseName.Value())
		}
		differences, ok := v.Get(entity.Name("Differences")).(*entity.Array)
		if !ok || differences.Len() == 0 {
			return base
		}
		if base == nil {
			base = map[byte]string{}
		}
		applyEncodingDifferences(base, differences)
		return base
	default:
		return nil
	}
}

func (e *Evaluator) resolveDirectObject(obj entity.Object) entity.Object {
	if ref, ok := obj.(entity.Ref); ok && e.xref != nil {
		resolved, err := e.xref.Fetch(ref)
		if err == nil {
			return resolved
		}
	}
	return obj
}

func simpleEncodingBaseNames(name string) map[byte]string {
	switch name {
	case "", "StandardEncoding", "MacRomanEncoding", "WinAnsiEncoding":
		out := map[byte]string{}
		for code, glyph := range simpleASCIIEncodingNames() {
			out[code] = glyph
		}
		return out
	default:
		return map[byte]string{}
	}
}

func applyEncodingDifferences(base map[byte]string, differences *entity.Array) {
	currentCode := -1
	for i := 0; i < differences.Len(); i++ {
		item := differences.Get(i)
		if code, ok := objectInt(item); ok {
			currentCode = code
			continue
		}
		name, ok := item.(entity.Name)
		if !ok || currentCode < 0 || currentCode > 255 {
			continue
		}
		base[byte(currentCode)] = name.Value()
		currentCode++
	}
}

func simpleASCIIEncodingNames() map[byte]string {
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
		0x60: "quoteleft",
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
