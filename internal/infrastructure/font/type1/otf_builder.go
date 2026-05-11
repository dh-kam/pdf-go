package type1

import (
	"encoding/binary"
	"math"
)

// BuildOTF creates a minimal OTF (CFF-based OpenType) binary from Type1 font data.
// The resulting binary can be parsed by golang.org/x/image/font/sfnt.
func BuildOTF(
	fontName string,
	glyphs map[uint32]*Glyph,
	encoding map[byte]string,
	fontInfo FontInfo,
) ([]byte, error) {
	// Build glyph order: .notdef first, then by char code
	type glyphEntry struct {
		code     uint32
		name     string
		width    float64
		lsb      float64
		commands []Command
	}

	entries := make([]glyphEntry, 0, len(glyphs)+1)
	// .notdef glyph
	entries = append(entries, glyphEntry{name: ".notdef", width: 500})

	codeToGlyphIdx := make(map[uint32]int) // charCode -> index in entries
	for code := uint32(0); code < 256; code++ {
		g, ok := glyphs[code]
		if !ok || g == nil {
			continue
		}
		idx := len(entries)
		codeToGlyphIdx[code] = idx
		entries = append(entries, glyphEntry{
			code:     code,
			name:     g.Name,
			width:    g.Width,
			lsb:      g.LSB,
			commands: g.Commands,
		})
	}

	numGlyphs := len(entries)
	if numGlyphs < 2 {
		// Need at least .notdef + one real glyph
		entries = append(entries, glyphEntry{name: "space", width: 250, code: 0x20})
		codeToGlyphIdx[0x20] = 1
		numGlyphs = 2
	}

	// Ensure font name is non-empty (CFF requires it)
	if fontName == "" {
		fontName = "Type1Font"
	}

	// Build Type2 CharStrings
	defaultWidth := float64(0)
	charStringBytes := make([][]byte, numGlyphs)
	for i, e := range entries {
		charStringBytes[i] = encodeType2CharString(e.commands, e.width, e.lsb, defaultWidth)
	}

	// Build CFF binary
	cffData := buildCFF(fontName, charStringBytes, numGlyphs, defaultWidth)

	// Build OTF
	unitsPerEm := uint16(1000)
	bbox := fontInfo.FontBBox
	if bbox[2] == 0 && bbox[3] == 0 {
		bbox = [4]float64{0, -200, 1000, 800}
	}

	// Collect widths for hmtx
	widths := make([]uint16, numGlyphs)
	lsbs := make([]int16, numGlyphs)
	maxWidth := uint16(0)
	for i, e := range entries {
		w := uint16(math.Round(e.width))
		widths[i] = w
		lsbs[i] = int16(math.Round(e.lsb))
		if w > maxWidth {
			maxWidth = w
		}
	}

	// Compute ascent/descent from bbox
	ascent := int16(math.Round(bbox[3]))
	descent := int16(math.Round(bbox[1]))
	if ascent <= 0 {
		ascent = 800
	}
	if descent >= 0 {
		descent = -200
	}

	// Build tables
	headTable := buildHeadTable(unitsPerEm, bbox)
	maxpTable := buildMaxpTable(uint16(numGlyphs))
	cmapTable := buildCmapTable(codeToGlyphIdx)
	hheaTable := buildHheaTable(ascent, descent, maxWidth, uint16(numGlyphs))
	hmtxTable := buildHmtxTable(widths, lsbs)
	postTable := buildPostTable()
	nameTable := buildNameTable(fontName)

	return buildOTFFile(headTable, maxpTable, cffData, cmapTable, hheaTable, hmtxTable, postTable, nameTable), nil
}

// encodeType2CharString converts decoded Type1 Commands to Type2 CharString bytes.
func encodeType2CharString(commands []Command, width, lsb, defaultWidth float64) []byte {
	var buf []byte

	if len(commands) == 0 {
		// Minimal charstring: width + endchar
		buf = appendCFFInt(buf, int(math.Round(width-defaultWidth)))
		buf = append(buf, 14) // endchar
		return buf
	}

	// Type2 convention: width is optional first arg before first drawing operator.
	// If width != defaultWidth, emit it as the first value.
	widthEmitted := false
	firstDrawing := true

	for _, cmd := range commands {
		switch cmd.Type {
		case CmdRMoveto:
			if len(cmd.Args) < 2 {
				continue
			}
			if firstDrawing {
				// Emit width + args + operator
				buf = appendCFFInt(buf, int(math.Round(width-defaultWidth)))
				widthEmitted = true
				firstDrawing = false
			}
			buf = appendCFFInt(buf, int(math.Round(cmd.Args[0])))
			buf = appendCFFInt(buf, int(math.Round(cmd.Args[1])))
			buf = append(buf, 21) // rmoveto

		case CmdRLineto:
			if len(cmd.Args) < 2 {
				continue
			}
			if firstDrawing && !widthEmitted {
				buf = appendCFFInt(buf, int(math.Round(width-defaultWidth)))
				widthEmitted = true
				firstDrawing = false
				buf = appendCFFInt(buf, 0)
				buf = appendCFFInt(buf, 0)
				buf = append(buf, 21) // rmoveto to origin
			}
			buf = appendCFFInt(buf, int(math.Round(cmd.Args[0])))
			buf = appendCFFInt(buf, int(math.Round(cmd.Args[1])))
			buf = append(buf, 5) // rlineto

		case CmdRRCurveto:
			if len(cmd.Args) < 6 {
				continue
			}
			if firstDrawing && !widthEmitted {
				buf = appendCFFInt(buf, int(math.Round(width-defaultWidth)))
				widthEmitted = true
				firstDrawing = false
				buf = appendCFFInt(buf, 0)
				buf = appendCFFInt(buf, 0)
				buf = append(buf, 21) // rmoveto
			}
			for i := 0; i < 6; i++ {
				buf = appendCFFInt(buf, int(math.Round(cmd.Args[i])))
			}
			buf = append(buf, 8) // rrcurveto

		case CmdClosePath:
			// Type2 does not need explicit closepath; implicitly closed on next moveto or endchar

		case CmdEndChar:
			if !widthEmitted {
				buf = appendCFFInt(buf, int(math.Round(width-defaultWidth)))
			}
			buf = append(buf, 14) // endchar
			return buf

		case CmdReturn:
			// Skip returns in the output
		}
	}

	// If no endchar was found, add one
	if !widthEmitted {
		buf = appendCFFInt(buf, int(math.Round(width-defaultWidth)))
	}
	buf = append(buf, 14) // endchar
	return buf
}

// appendCFFInt encodes an integer in CFF format.
func appendCFFInt(buf []byte, v int) []byte {
	if v >= -107 && v <= 107 {
		return append(buf, byte(v+139))
	}
	if v >= 108 && v <= 1131 {
		v -= 108
		return append(buf, byte(v/256+247), byte(v%256))
	}
	if v >= -1131 && v <= -108 {
		v = -v - 108
		return append(buf, byte(v/256+251), byte(v%256))
	}
	// 5-byte encoding: 29 followed by 4-byte big-endian int32
	return append(buf, 29,
		byte(int32(v)>>24), byte(int32(v)>>16), byte(int32(v)>>8), byte(int32(v)))
}

// buildCFF creates a minimal CFF binary.
func buildCFF(fontName string, charStrings [][]byte, numGlyphs int, defaultWidth float64) []byte {
	var cff []byte

	// Header: major=1, minor=0, hdrSize=4, offSize=4
	cff = append(cff, 1, 0, 4, 4)

	// Name INDEX: 1 entry
	cff = appendCFFIndex(cff, [][]byte{[]byte(fontName)})

	// Top DICT INDEX: 1 entry (placeholder - we'll fill offsets later)
	topDictStart := len(cff)
	topDict := buildTopDict(numGlyphs, defaultWidth)
	cff = appendCFFIndex(cff, [][]byte{topDict})
	topDictEnd := len(cff)

	// String INDEX: 0 entries
	cff = appendCFFIndex(cff, nil)

	// Global Subr INDEX: 0 entries
	cff = appendCFFIndex(cff, nil)

	// Charset (format 0): .notdef is implicit, then SID for each glyph
	charsetOffset := len(cff)
	cff = append(cff, 0) // format 0
	for i := 1; i < numGlyphs; i++ {
		// Use SIDs starting at 1 (space=1 in standard strings)
		sid := uint16(i)
		cff = append(cff, byte(sid>>8), byte(sid))
	}

	// CharStrings INDEX
	charStringsOffset := len(cff)
	cff = appendCFFIndex(cff, charStrings)

	// Private DICT
	privateDictOffset := len(cff)
	privateDict := buildPrivateDict(defaultWidth)
	cff = append(cff, privateDict...)
	privateDictLength := len(cff) - privateDictOffset

	// Now patch the Top DICT with correct offsets
	topDict = buildTopDictWithOffsets(charsetOffset, charStringsOffset, privateDictOffset, privateDictLength)
	// Rebuild the Top DICT INDEX
	var newCFF []byte
	newCFF = append(newCFF, cff[:topDictStart]...)
	newCFF = appendCFFIndex(newCFF, [][]byte{topDict})
	// Adjust offsets if Top DICT INDEX size changed
	sizeDiff := len(newCFF) - topDictEnd
	if sizeDiff != 0 {
		// Rebuild everything with correct offsets
		return buildCFFComplete(fontName, charStrings, numGlyphs, defaultWidth)
	}
	copy(cff[topDictStart:topDictEnd], newCFF[topDictStart:])
	return cff
}

// buildCFFComplete builds CFF with a two-pass approach to get offsets right.
func buildCFFComplete(fontName string, charStrings [][]byte, numGlyphs int, defaultWidth float64) []byte {
	// First pass: measure sizes with placeholder TopDict
	headerSize := 4
	nameIndex := makeCFFIndex([][]byte{[]byte(fontName)})
	stringIndex := makeCFFIndex(nil)
	globalSubrIndex := makeCFFIndex(nil)

	// Charset
	charsetData := []byte{0} // format 0
	for i := 1; i < numGlyphs; i++ {
		sid := uint16(i)
		charsetData = append(charsetData, byte(sid>>8), byte(sid))
	}

	charStringsIndex := makeCFFIndex(charStrings)
	privateDict := buildPrivateDict(defaultWidth)

	// Two-pass: first calculate with estimated TopDict, then stabilize
	var lastTopDictIndex []byte
	for attempt := 0; attempt < 5; attempt++ {
		// Use previous TopDict size or estimate
		var estTopDict []byte
		if lastTopDictIndex != nil {
			estTopDict = lastTopDictIndex
		} else {
			td := buildTopDictWithOffsets(100, 200, 300, len(privateDict))
			estTopDict = makeCFFIndex([][]byte{td})
		}

		afterName := headerSize + len(nameIndex)
		afterTopDict := afterName + len(estTopDict)
		afterStrings := afterTopDict + len(stringIndex)
		afterGlobalSubrs := afterStrings + len(globalSubrIndex)

		charsetOffset := afterGlobalSubrs
		charStringsOffset := charsetOffset + len(charsetData)
		privateDictOffset := charStringsOffset + len(charStringsIndex)

		topDict := buildTopDictWithOffsets(charsetOffset, charStringsOffset, privateDictOffset, len(privateDict))
		topDictIndex := makeCFFIndex([][]byte{topDict})

		if len(topDictIndex) == len(estTopDict) {
			// Stable — build final CFF
			var cff []byte
			cff = append(cff, 1, 0, 4, 4) // header
			cff = append(cff, nameIndex...)
			cff = append(cff, topDictIndex...)
			cff = append(cff, stringIndex...)
			cff = append(cff, globalSubrIndex...)
			cff = append(cff, charsetData...)
			cff = append(cff, charStringsIndex...)
			cff = append(cff, privateDict...)
			return cff
		}
		lastTopDictIndex = topDictIndex
	}

	// Use last computed values
	afterName := headerSize + len(nameIndex)
	afterTopDict := afterName + len(lastTopDictIndex)
	afterStrings := afterTopDict + len(stringIndex)
	afterGlobalSubrs := afterStrings + len(globalSubrIndex)
	charsetOffset := afterGlobalSubrs
	charStringsOffset := charsetOffset + len(charsetData)
	privateDictOffset := charStringsOffset + len(charStringsIndex)
	topDict := buildTopDictWithOffsets(charsetOffset, charStringsOffset, privateDictOffset, len(privateDict))
	topDictIndex := makeCFFIndex([][]byte{topDict})

	var cff []byte
	cff = append(cff, 1, 0, 4, 4)
	cff = append(cff, nameIndex...)
	cff = append(cff, topDictIndex...)
	cff = append(cff, stringIndex...)
	cff = append(cff, globalSubrIndex...)
	cff = append(cff, charsetData...)
	cff = append(cff, charStringsIndex...)
	cff = append(cff, privateDict...)
	return cff
}

func buildTopDict(numGlyphs int, defaultWidth float64) []byte {
	return buildTopDictWithOffsets(0, 0, 0, 0)
}

func buildTopDictWithOffsets(charset, charStrings, privateOffset, privateLength int) []byte {
	var d []byte
	// charset offset (operator 15)
	d = appendCFFDictInt(d, charset)
	d = append(d, 15)
	// charStrings offset (operator 17)
	d = appendCFFDictInt(d, charStrings)
	d = append(d, 17)
	// Private dict (length + offset) (operator 18)
	d = appendCFFDictInt(d, privateLength)
	d = appendCFFDictInt(d, privateOffset)
	d = append(d, 18)
	return d
}

func buildPrivateDict(defaultWidth float64) []byte {
	var d []byte
	// defaultWidthX (operator 20): default is 0
	d = appendCFFDictInt(d, int(defaultWidth))
	d = append(d, 20)
	// nominalWidthX (operator 21): 0
	d = appendCFFDictInt(d, 0)
	d = append(d, 21)
	return d
}

func appendCFFDictInt(buf []byte, v int) []byte {
	// CFF DICT integer encoding (same as CharString for small values, different for large)
	if v >= -107 && v <= 107 {
		return append(buf, byte(v+139))
	}
	if v >= 108 && v <= 1131 {
		v -= 108
		return append(buf, byte(v/256+247), byte(v%256))
	}
	if v >= -1131 && v <= -108 {
		v = -v - 108
		return append(buf, byte(v/256+251), byte(v%256))
	}
	// 5-byte encoding for DICT: operator 29 followed by 4 bytes
	return append(buf, 29,
		byte(int32(v)>>24), byte(int32(v)>>16), byte(int32(v)>>8), byte(int32(v)))
}

// makeCFFIndex creates a CFF INDEX structure.
func makeCFFIndex(data [][]byte) []byte {
	count := len(data)
	if count == 0 {
		return []byte{0, 0} // count = 0
	}

	// Calculate total data size to determine offSize
	totalSize := 0
	for _, d := range data {
		totalSize += len(d)
	}

	offSize := byte(1)
	if totalSize+1 > 0xFF {
		offSize = 2
	}
	if totalSize+1 > 0xFFFF {
		offSize = 3
	}
	if totalSize+1 > 0xFFFFFF {
		offSize = 4
	}

	var idx []byte
	// Count (2 bytes)
	idx = append(idx, byte(count>>8), byte(count))
	// offSize (1 byte)
	idx = append(idx, offSize)
	// Offsets (count+1 entries, 1-based)
	offset := 1
	for i := 0; i <= count; i++ {
		switch offSize {
		case 1:
			idx = append(idx, byte(offset))
		case 2:
			idx = append(idx, byte(offset>>8), byte(offset))
		case 3:
			idx = append(idx, byte(offset>>16), byte(offset>>8), byte(offset))
		case 4:
			idx = append(idx, byte(offset>>24), byte(offset>>16), byte(offset>>8), byte(offset))
		}
		if i < count {
			offset += len(data[i])
		}
	}
	// Data
	for _, d := range data {
		idx = append(idx, d...)
	}
	return idx
}

func appendCFFIndex(buf []byte, data [][]byte) []byte {
	return append(buf, makeCFFIndex(data)...)
}

// OTF table builders

func buildHeadTable(unitsPerEm uint16, bbox [4]float64) []byte {
	t := make([]byte, 54)
	// majorVersion=1, minorVersion=0
	binary.BigEndian.PutUint16(t[0:], 1)
	binary.BigEndian.PutUint16(t[2:], 0)
	// fontRevision
	binary.BigEndian.PutUint32(t[4:], 0x00010000)
	// checksumAdjustment (0 for now)
	// magicNumber
	binary.BigEndian.PutUint32(t[12:], 0x5F0F3CF5)
	// flags
	binary.BigEndian.PutUint16(t[16:], 0x000B)
	// unitsPerEm
	binary.BigEndian.PutUint16(t[18:], unitsPerEm)
	// created, modified (skip - 8 bytes each)
	// xMin, yMin, xMax, yMax
	binary.BigEndian.PutUint16(t[36:], uint16(int16(bbox[0])))
	binary.BigEndian.PutUint16(t[38:], uint16(int16(bbox[1])))
	binary.BigEndian.PutUint16(t[40:], uint16(int16(bbox[2])))
	binary.BigEndian.PutUint16(t[42:], uint16(int16(bbox[3])))
	// macStyle, lowestRecPPEM
	binary.BigEndian.PutUint16(t[46:], 8) // lowestRecPPEM
	// indexToLocFormat
	binary.BigEndian.PutUint16(t[50:], 0)
	// glyphDataFormat
	binary.BigEndian.PutUint16(t[52:], 0)
	return t
}

func buildMaxpTable(numGlyphs uint16) []byte {
	t := make([]byte, 6)
	// version 0.5 for CFF
	binary.BigEndian.PutUint16(t[0:], 0x0000)
	binary.BigEndian.PutUint16(t[2:], 0x5000)
	binary.BigEndian.PutUint16(t[4:], numGlyphs)
	return t
}

func buildCmapTable(codeToGlyphIdx map[uint32]int) []byte {
	// Build format 4 cmap subtable
	// Collect segments
	type seg struct {
		start, end uint16
		delta      int16
	}
	var segments []seg

	for code, idx := range codeToGlyphIdx {
		if code > 0xFFFF {
			continue
		}
		segments = append(segments, seg{
			start: uint16(code),
			end:   uint16(code),
			delta: int16(idx) - int16(code),
		})
	}

	// Sort segments by start
	for i := 0; i < len(segments); i++ {
		for j := i + 1; j < len(segments); j++ {
			if segments[j].start < segments[i].start {
				segments[i], segments[j] = segments[j], segments[i]
			}
		}
	}

	// Merge consecutive segments with same delta
	merged := make([]seg, 0, len(segments))
	for _, s := range segments {
		if len(merged) > 0 && merged[len(merged)-1].end+1 == s.start && merged[len(merged)-1].delta == s.delta {
			merged[len(merged)-1].end = s.end
		} else {
			merged = append(merged, s)
		}
	}
	// Add sentinel segment
	merged = append(merged, seg{start: 0xFFFF, end: 0xFFFF, delta: 1})

	segCount := len(merged)
	searchRange := uint16(1)
	entrySelector := uint16(0)
	for searchRange*2 <= uint16(segCount) {
		searchRange *= 2
		entrySelector++
	}
	searchRange *= 2
	rangeShift := uint16(segCount)*2 - searchRange

	// Format 4 subtable
	subtableLen := 14 + segCount*8
	var sub []byte
	sub = appendU16(sub, 4)                   // format
	sub = appendU16(sub, uint16(subtableLen))  // length
	sub = appendU16(sub, 0)                    // language
	sub = appendU16(sub, uint16(segCount*2))   // segCountX2
	sub = appendU16(sub, searchRange)          // searchRange
	sub = appendU16(sub, entrySelector)        // entrySelector
	sub = appendU16(sub, rangeShift)           // rangeShift

	for _, s := range merged {
		sub = appendU16(sub, s.end)
	}
	sub = appendU16(sub, 0) // reservedPad
	for _, s := range merged {
		sub = appendU16(sub, s.start)
	}
	for _, s := range merged {
		sub = appendU16(sub, uint16(s.delta))
	}
	for range merged {
		sub = appendU16(sub, 0) // idRangeOffset = 0 (use delta)
	}

	// cmap header
	var cmap []byte
	cmap = appendU16(cmap, 0) // version
	cmap = appendU16(cmap, 1) // numTables
	// Encoding record: platform=3 (Windows), encoding=1 (UCS-2)
	cmap = appendU16(cmap, 3)
	cmap = appendU16(cmap, 1)
	cmap = appendU32(cmap, uint32(12)) // offset to subtable
	cmap = append(cmap, sub...)

	return cmap
}

func buildHheaTable(ascent, descent int16, maxWidth, numGlyphs uint16) []byte {
	t := make([]byte, 36)
	// majorVersion=1, minorVersion=0
	binary.BigEndian.PutUint16(t[0:], 1)
	binary.BigEndian.PutUint16(t[2:], 0)
	// ascender
	binary.BigEndian.PutUint16(t[4:], uint16(ascent))
	// descender
	binary.BigEndian.PutUint16(t[6:], uint16(descent))
	// lineGap
	binary.BigEndian.PutUint16(t[8:], 0)
	// advanceWidthMax
	binary.BigEndian.PutUint16(t[10:], maxWidth)
	// numberOfHMetrics
	binary.BigEndian.PutUint16(t[34:], numGlyphs)
	return t
}

func buildHmtxTable(widths []uint16, lsbs []int16) []byte {
	var t []byte
	for i := range widths {
		t = appendU16(t, widths[i])
		t = appendU16(t, uint16(lsbs[i]))
	}
	return t
}

func buildPostTable() []byte {
	t := make([]byte, 32)
	// version 3.0 (no glyph names)
	binary.BigEndian.PutUint32(t[0:], 0x00030000)
	return t
}

func buildNameTable(fontName string) []byte {
	nameBytes := []byte(fontName)
	if len(nameBytes) == 0 {
		nameBytes = []byte("Type1Font")
	}

	// Simple name table with one entry (nameID=4, full name)
	var t []byte
	t = appendU16(t, 0) // format
	t = appendU16(t, 1) // count
	stringOffset := uint16(6 + 12) // header(6) + 1 record(12)
	t = appendU16(t, stringOffset) // stringOffset
	// Record: platformID=3, encodingID=1, languageID=0x0409, nameID=4
	t = appendU16(t, 3)                      // platformID
	t = appendU16(t, 1)                      // encodingID
	t = appendU16(t, 0x0409)                 // languageID
	t = appendU16(t, 4)                      // nameID (full font name)
	t = appendU16(t, uint16(len(nameBytes)*2)) // length (UTF-16BE)
	t = appendU16(t, 0)                      // offset

	// String data (UTF-16BE)
	for _, b := range nameBytes {
		t = append(t, 0, b)
	}
	return t
}

func buildOTFFile(head, maxp, cff, cmap, hhea, hmtx, post, name []byte) []byte {
	type tableEntry struct {
		tag  [4]byte
		data []byte
	}
	tables := []tableEntry{
		{tag: [4]byte{'C', 'F', 'F', ' '}, data: cff},
		{tag: [4]byte{'O', 'S', '/', '2'}, data: buildOS2Table()},
		{tag: [4]byte{'c', 'm', 'a', 'p'}, data: cmap},
		{tag: [4]byte{'h', 'e', 'a', 'd'}, data: head},
		{tag: [4]byte{'h', 'h', 'e', 'a'}, data: hhea},
		{tag: [4]byte{'h', 'm', 't', 'x'}, data: hmtx},
		{tag: [4]byte{'m', 'a', 'x', 'p'}, data: maxp},
		{tag: [4]byte{'n', 'a', 'm', 'e'}, data: name},
		{tag: [4]byte{'p', 'o', 's', 't'}, data: post},
	}

	numTables := uint16(len(tables))
	searchRange := uint16(1)
	entrySelector := uint16(0)
	for searchRange*2 <= numTables {
		searchRange *= 2
		entrySelector++
	}
	searchRange *= 16
	rangeShift := numTables*16 - searchRange

	// SFNT header
	var file []byte
	file = append(file, 'O', 'T', 'T', 'O') // sfVersion
	file = appendU16(file, numTables)
	file = appendU16(file, searchRange)
	file = appendU16(file, entrySelector)
	file = appendU16(file, rangeShift)

	// Calculate table offsets
	headerSize := 12 + int(numTables)*16
	offset := headerSize
	offsets := make([]int, len(tables))
	for i, t := range tables {
		offsets[i] = offset
		offset += len(t.data)
		// Pad to 4-byte boundary
		if offset%4 != 0 {
			offset += 4 - offset%4
		}
	}

	// Table directory
	for i, t := range tables {
		file = append(file, t.tag[:]...)
		file = appendU32(file, checksum(t.data))
		file = appendU32(file, uint32(offsets[i]))
		file = appendU32(file, uint32(len(t.data)))
	}

	// Table data
	for _, t := range tables {
		file = append(file, t.data...)
		// Pad to 4-byte boundary
		for len(file)%4 != 0 {
			file = append(file, 0)
		}
	}

	return file
}

func buildOS2Table() []byte {
	// Minimal OS/2 table (version 1, 78 bytes)
	t := make([]byte, 78)
	// version = 1
	binary.BigEndian.PutUint16(t[0:], 1)
	// xAvgCharWidth
	binary.BigEndian.PutUint16(t[2:], 500)
	// usWeightClass = 400 (normal)
	binary.BigEndian.PutUint16(t[4:], 400)
	// usWidthClass = 5 (medium)
	binary.BigEndian.PutUint16(t[6:], 5)
	// sTypoAscender
	binary.BigEndian.PutUint16(t[68:], uint16(int16(800)))
	// sTypoDescender
	putInt16(t[70:], -200)
	// sTypoLineGap
	binary.BigEndian.PutUint16(t[72:], 0)
	// usWinAscent
	binary.BigEndian.PutUint16(t[74:], 800)
	// usWinDescent
	binary.BigEndian.PutUint16(t[76:], 200)
	return t
}

func checksum(data []byte) uint32 {
	var sum uint32
	for i := 0; i+3 < len(data); i += 4 {
		sum += binary.BigEndian.Uint32(data[i:])
	}
	// Handle remaining bytes
	remaining := len(data) % 4
	if remaining > 0 {
		var last [4]byte
		copy(last[:], data[len(data)-remaining:])
		sum += binary.BigEndian.Uint32(last[:])
	}
	return sum
}

func putInt16(buf []byte, v int16) {
	binary.BigEndian.PutUint16(buf, uint16(v))
}

func appendU16(buf []byte, v uint16) []byte {
	return append(buf, byte(v>>8), byte(v))
}

func appendU32(buf []byte, v uint32) []byte {
	return append(buf, byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}
