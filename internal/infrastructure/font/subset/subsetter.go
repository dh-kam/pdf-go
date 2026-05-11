// Package subset provides font subsetting functionality.
package subset

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// Subsetter creates a subset of a font containing only specified glyphs.
type Subsetter struct {
	font            entity.Font
	usedGlyphs      map[uint32]bool   // Glyph IDs that are used
	charCodeToGlyph map[uint32]uint32 // Character code to glyph mapping
}

type fontDataProvider interface {
	FontData() []byte
}

type baseFontProvider interface {
	BaseFont() entity.Font
}

// NewSubsetter creates a new font subsetter.
func NewSubsetter(font entity.Font) *Subsetter {
	return &Subsetter{
		font:            font,
		usedGlyphs:      make(map[uint32]bool),
		charCodeToGlyph: make(map[uint32]uint32),
	}
}

// AddGlyph adds a glyph to the subset.
func (s *Subsetter) AddGlyph(glyphID uint32) {
	s.usedGlyphs[glyphID] = true
}

// AddCharCodes adds glyphs for the given character codes.
func (s *Subsetter) AddCharCodes(charCodes []uint32) error {
	for _, charCode := range charCodes {
		glyphID, err := s.font.CharCodeToGlyph(charCode)
		if err != nil {
			return err
		}
		s.usedGlyphs[glyphID] = true
		s.charCodeToGlyph[charCode] = glyphID
	}
	return nil
}

// HasGlyph returns true if the glyph is in the subset.
func (s *Subsetter) HasGlyph(glyphID uint32) bool {
	return s.usedGlyphs[glyphID]
}

// GlyphCount returns the number of glyphs in the subset.
func (s *Subsetter) GlyphCount() int {
	return len(s.usedGlyphs)
}

// SubsetType1 subsets a Type1 font.
func (s *Subsetter) SubsetType1() ([]byte, error) {
	if provider, ok := s.font.(fontDataProvider); ok {
		data := provider.FontData()
		if len(data) > 0 {
			return append([]byte(nil), data...), nil
		}
	}

	return nil, fmt.Errorf("font data is required for type1 subset")
}

// SubsetTrueType subsets a TrueType/OpenType font.
func (s *Subsetter) SubsetTrueType() ([]byte, error) {
	provider, ok := s.font.(fontDataProvider)
	if !ok {
		return nil, fmt.Errorf("font data provider is required for truetype subset")
	}

	data := provider.FontData()
	if len(data) == 0 {
		return nil, fmt.Errorf("font data is required for truetype subset")
	}

	subsetter := NewTrueTypeSubsetter(data)
	for glyphID := range s.usedGlyphs {
		subsetter.glyphs[glyphID] = true
	}
	subsetter.glyphOrder = sortedGlyphIDs(subsetter.glyphs)

	return subsetter.Subset()
}

// SubsetCFF subsets a CFF/Type1C font.
func (s *Subsetter) SubsetCFF() ([]byte, error) {
	provider, ok := s.font.(fontDataProvider)
	if !ok {
		return nil, fmt.Errorf("font data provider is required for cff subset")
	}

	data := provider.FontData()
	if len(data) == 0 {
		return nil, fmt.Errorf("font data is required for cff subset")
	}

	subsetter := NewCFFSubsetter(data)
	for glyphID := range s.usedGlyphs {
		subsetter.glyphs[glyphID] = true
	}

	return subsetter.Subset()
}

// SubsetCIDFont subsets a CID-keyed font.
func (s *Subsetter) SubsetCIDFont() ([]byte, error) {
	if provider, ok := s.font.(baseFontProvider); ok {
		base := provider.BaseFont()
		if base != nil {
			baseSubsetter := NewSubsetter(base)
			for glyphID := range s.usedGlyphs {
				baseSubsetter.AddGlyph(glyphID)
			}
			return baseSubsetter.Subset()
		}
	}

	if provider, ok := s.font.(fontDataProvider); ok {
		data := provider.FontData()
		if len(data) > 0 {
			return append([]byte(nil), data...), nil
		}
	}

	return nil, fmt.Errorf("cid base font or raw data is required for cid subset")
}

// Subset creates a subset of the font.
func (s *Subsetter) Subset() ([]byte, error) {
	// Check font type and call appropriate subsetting method
	if s.font.IsCIDFont() {
		return s.SubsetCIDFont()
	}

	fontName := s.font.Name()

	// CFF/Type1C names should be handled by the CFF subsetter.
	if isCFFFont(fontName) {
		return s.SubsetCFF()
	}

	if provider, ok := s.font.(fontDataProvider); ok {
		fontData := provider.FontData()

		switch {
		case isTrueTypeData(fontData):
			return s.SubsetTrueType()
		case isLikelyCFFData(fontData):
			return s.SubsetCFF()
		case isLikelyType1Data(fontData):
			return s.SubsetType1()
		}
	}

	if isType1Font(fontName) {
		return s.SubsetType1()
	}

	// Default to TrueType
	return s.SubsetTrueType()
}

// isCFFFont checks if a font name suggests it's a CFF/Type1C font.
func isCFFFont(fontName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(fontName))
	if normalized == "" {
		return false
	}

	return strings.Contains(normalized, "cff") ||
		strings.Contains(normalized, "type1c") ||
		strings.Contains(normalized, "cidfonttype0c")
}

func isType1Font(fontName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(fontName))
	if normalized == "" {
		return false
	}

	return strings.Contains(normalized, "type1") ||
		strings.Contains(normalized, "type 1") ||
		strings.Contains(normalized, "pfb") ||
		strings.Contains(normalized, "pfa")
}

func isTrueTypeData(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	scalerType := binary.BigEndian.Uint32(data[:4])
	switch scalerType {
	case 0x00010000: // TrueType
		return true
	case 0x4F54544F: // "OTTO" CFF OpenType
		return true
	case 0x74727565: // "true"
		return true
	case 0x74746366: // "ttcf"
		return true
	default:
		return false
	}
}

func isLikelyCFFData(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	major := data[0]
	headerSize := int(data[2])
	offSize := data[3]

	if major != 1 {
		return false
	}
	if headerSize < 4 || headerSize > len(data) {
		return false
	}
	return offSize >= 1 && offSize <= 4
}

func isLikelyType1Data(data []byte) bool {
	if len(data) < 2 {
		return false
	}

	// PFB file signature.
	if data[0] == 0x80 && (data[1] == 0x01 || data[1] == 0x02) {
		return true
	}

	limit := len(data)
	if limit > 256 {
		limit = 256
	}
	header := bytes.ToLower(data[:limit])

	return bytes.Contains(header, []byte("%!ps-adobefont")) ||
		bytes.Contains(header, []byte("/fonttype 1")) ||
		bytes.Contains(header, []byte("currentfile eexec"))
}

// TrueTypeSubsetter handles TrueType font subsetting.
type TrueTypeSubsetter struct {
	oldToNew         map[uint32]uint32
	tables           map[string]tableEntry
	glyphs           map[uint32]bool
	glyphOrder       []uint32
	locaTable        []uint32
	glyfTable        []byte
	data             []byte
	longHorMetric    []horMetricEntry
	lsbTable         []int16
	indexToLocFormat int16
	numGlyphs        uint16
}

// horMetricEntry represents a horizontal metric entry in the hmtx table.
type horMetricEntry struct {
	advanceWidth uint16
	lsb          int16
}

// Subset creates a subset of the TrueType font.
func (t *TrueTypeSubsetter) Subset() ([]byte, error) {
	t.ensureGlyphOrder()

	// Parse table directory
	if err := t.parseTableDirectory(); err != nil {
		return nil, fmt.Errorf("parse table directory: %w", err)
	}

	// Ensure we have at least glyph 0 (notdef)
	if !t.glyphs[0] {
		t.glyphs[0] = true
		t.glyphOrder = append([]uint32{0}, t.glyphOrder...)
	}

	// Parse required tables
	if err := t.parseRequiredTables(); err != nil {
		return nil, fmt.Errorf("parse required tables: %w", err)
	}

	// Extract subset glyph data
	newGlyf, newLoca, err := t.subsetGlyfTable()
	if err != nil {
		return nil, fmt.Errorf("subset glyf table: %w", err)
	}

	// Subset hmtx table
	newHmtx, err := t.subsetHmtxTable()
	if err != nil {
		return nil, fmt.Errorf("subset hmtx table: %w", err)
	}

	// Update maxp table
	newMaxp := t.subsetMaxpTable()

	// Subset cmap table
	newCmap, err := t.subsetCmapTable()
	if err != nil {
		return nil, fmt.Errorf("subset cmap table: %w", err)
	}

	// Build subsetted font
	return t.buildSubsetFont(newGlyf, newLoca, newHmtx, newMaxp, newCmap)
}

func (t *TrueTypeSubsetter) ensureGlyphOrder() {
	if len(t.glyphOrder) == 0 {
		t.glyphOrder = sortedGlyphIDs(t.glyphs)
	}
	if len(t.glyphOrder) == 0 {
		t.glyphOrder = []uint32{0}
		t.glyphs[0] = true
		return
	}

	if !t.glyphs[0] {
		t.glyphs[0] = true
		t.glyphOrder = append([]uint32{0}, t.glyphOrder...)
	}
}

func sortedGlyphIDs(glyphSet map[uint32]bool) []uint32 {
	if len(glyphSet) == 0 {
		return nil
	}

	ids := make([]uint32, 0, len(glyphSet))
	for glyphID, used := range glyphSet {
		if !used {
			continue
		}
		ids = append(ids, glyphID)
	}
	sort.Slice(ids, func(i, j int) bool {
		return ids[i] < ids[j]
	})
	return ids
}

type tableEntry struct {
	offset   uint32
	length   uint32
	checksum uint32
}

// NewTrueTypeSubsetter creates a new TrueType subsetter.
func NewTrueTypeSubsetter(data []byte) *TrueTypeSubsetter {
	return &TrueTypeSubsetter{
		data:             data,
		tables:           make(map[string]tableEntry),
		glyphs:           make(map[uint32]bool),
		oldToNew:         make(map[uint32]uint32),
		longHorMetric:    make([]horMetricEntry, 0),
		lsbTable:         make([]int16, 0),
		indexToLocFormat: 0,
		numGlyphs:        0,
	}
}

// parseTableDirectory parses the TrueType table directory.
func (t *TrueTypeSubsetter) parseTableDirectory() error {
	if len(t.data) < 12 {
		return fmt.Errorf("invalid TrueType font: too short")
	}

	// Read SFNT version
	scalarType := binary.BigEndian.Uint32(t.data[0:4])
	if scalarType != 0x00010000 && scalarType != 0x74727565 { // "true"
		return fmt.Errorf("invalid TrueType font: bad SFNT version 0x%08x", scalarType)
	}

	numTables := binary.BigEndian.Uint16(t.data[4:6])
	// Skip searchRange, entrySelector, rangeShift (bytes 6-12)

	// Read table directory entries
	offset := 12
	for i := uint16(0); i < numTables; i++ {
		if offset+16 > len(t.data) {
			return fmt.Errorf("invalid TrueType font: truncated table directory")
		}

		tableTag := string(t.data[offset : offset+4])
		tableChecksum := binary.BigEndian.Uint32(t.data[offset+4 : offset+8])
		tableOffset := binary.BigEndian.Uint32(t.data[offset+8 : offset+12])
		tableLength := binary.BigEndian.Uint32(t.data[offset+12 : offset+16])

		t.tables[tableTag] = tableEntry{
			offset:   tableOffset,
			length:   tableLength,
			checksum: tableChecksum,
		}

		offset += 16
	}

	return nil
}

// getGlyphData extracts glyph data from the glyf table.
func (t *TrueTypeSubsetter) getGlyphData(glyphID uint32) ([]byte, error) {
	if int(glyphID) >= len(t.locaTable)-1 {
		return nil, fmt.Errorf("glyph ID %d out of range", glyphID)
	}

	offset := t.locaTable[glyphID]
	nextOffset := t.locaTable[glyphID+1]

	if offset >= uint32(len(t.glyfTable)) || nextOffset > uint32(len(t.glyfTable)) {
		return nil, fmt.Errorf("invalid glyph offset for glyph %d", glyphID)
	}

	return t.glyfTable[offset:nextOffset], nil
}

// parseRequiredTables parses the tables required for subsetting.
func (t *TrueTypeSubsetter) parseRequiredTables() error {
	// Parse head table for indexToLocFormat
	if err := t.parseHeadTable(); err != nil {
		return fmt.Errorf("parse head table: %w", err)
	}

	// Parse maxp table for numGlyphs
	if err := t.parseMaxpTable(); err != nil {
		return fmt.Errorf("parse maxp table: %w", err)
	}

	// Parse loca table
	if err := t.parseLocaTable(); err != nil {
		return fmt.Errorf("parse loca table: %w", err)
	}

	// Parse glyf table
	if err := t.parseGlyfTable(); err != nil {
		return fmt.Errorf("parse glyf table: %w", err)
	}

	// Parse hmtx table
	if err := t.parseHmtxTable(); err != nil {
		return fmt.Errorf("parse hmtx table: %w", err)
	}

	return nil
}

// parseHeadTable parses the head table to get indexToLocFormat.
func (t *TrueTypeSubsetter) parseHeadTable() error {
	entry, ok := t.tables["head"]
	if !ok {
		return fmt.Errorf("head table not found")
	}

	if entry.offset+54 > uint32(len(t.data)) {
		return fmt.Errorf("head table truncated")
	}

	// indexToLocFormat is at offset 50 in the head table
	t.indexToLocFormat = int16(binary.BigEndian.Uint16(t.data[entry.offset+50 : entry.offset+52]))

	return nil
}

// parseMaxpTable parses the maxp table to get numGlyphs.
func (t *TrueTypeSubsetter) parseMaxpTable() error {
	entry, ok := t.tables["maxp"]
	if !ok {
		return fmt.Errorf("maxp table not found")
	}

	if entry.offset+6 > uint32(len(t.data)) {
		return fmt.Errorf("maxp table truncated")
	}

	// numGlyphs is at offset 4 in the maxp table (version 1.0)
	t.numGlyphs = binary.BigEndian.Uint16(t.data[entry.offset+4 : entry.offset+6])

	return nil
}

// parseLocaTable parses the loca (index to location) table.
func (t *TrueTypeSubsetter) parseLocaTable() error {
	entry, ok := t.tables["loca"]
	if !ok {
		return fmt.Errorf("loca table not found")
	}

	// Number of loca entries is numGlyphs + 1
	numEntries := int(t.numGlyphs) + 1

	if t.indexToLocFormat == 0 {
		// Short format (16-bit offsets)
		if entry.offset+uint32(numEntries*2) > uint32(len(t.data)) {
			return fmt.Errorf("loca table truncated")
		}

		t.locaTable = make([]uint32, numEntries)
		for i := 0; i < numEntries; i++ {
			offset := binary.BigEndian.Uint16(t.data[entry.offset+uint32(i*2) : entry.offset+uint32(i*2+2)])
			t.locaTable[i] = uint32(offset) * 2 // Convert to actual offset
		}
	} else {
		// Long format (32-bit offsets)
		if entry.offset+uint32(numEntries*4) > uint32(len(t.data)) {
			return fmt.Errorf("loca table truncated")
		}

		t.locaTable = make([]uint32, numEntries)
		for i := 0; i < numEntries; i++ {
			t.locaTable[i] = binary.BigEndian.Uint32(t.data[entry.offset+uint32(i*4) : entry.offset+uint32(i*4+4)])
		}
	}

	return nil
}

// parseGlyfTable reads the glyf table data.
func (t *TrueTypeSubsetter) parseGlyfTable() error {
	entry, ok := t.tables["glyf"]
	if !ok {
		return fmt.Errorf("glyf table not found")
	}

	if entry.offset+entry.length > uint32(len(t.data)) {
		return fmt.Errorf("glyf table truncated")
	}

	t.glyfTable = t.data[entry.offset : entry.offset+entry.length]
	return nil
}

// parseHmtxTable parses the hmtx (horizontal metrics) table.
func (t *TrueTypeSubsetter) parseHmtxTable() error {
	entry, ok := t.tables["hmtx"]
	if !ok {
		return fmt.Errorf("hmtx table not found")
	}

	if entry.offset+entry.length > uint32(len(t.data)) {
		return fmt.Errorf("hmtx table truncated")
	}

	data := t.data[entry.offset : entry.offset+entry.length]

	// Get numberOfHMetrics from head table (hhea table actually)
	numHMetrics, err := t.getNumberOfHMetrics()
	if err != nil {
		return fmt.Errorf("get number of HMetrics: %w", err)
	}

	// Parse hmtx table
	// Each longHorMetric record is 4 bytes (advanceWidth + lsb)
	pos := 0
	t.longHorMetric = make([]horMetricEntry, numHMetrics)

	for i := 0; i < int(numHMetrics) && pos+4 <= len(data); i++ {
		t.longHorMetric[i] = horMetricEntry{
			advanceWidth: binary.BigEndian.Uint16(data[pos : pos+2]),
			lsb:          int16(binary.BigEndian.Uint16(data[pos+2 : pos+4])),
		}
		pos += 4
	}

	// Remaining glyphs share the last advanceWidth
	remainingGlyphs := int(t.numGlyphs) - int(numHMetrics)
	t.lsbTable = make([]int16, remainingGlyphs)

	for i := 0; i < remainingGlyphs && pos+2 <= len(data); i++ {
		t.lsbTable[i] = int16(binary.BigEndian.Uint16(data[pos : pos+2]))
		pos += 2
	}

	return nil
}

// getNumberOfHMetrics gets the numberOfHMetrics from the hhea table.
func (t *TrueTypeSubsetter) getNumberOfHMetrics() (uint16, error) {
	entry, ok := t.tables["hhea"]
	if !ok {
		return 0, fmt.Errorf("hhea table not found")
	}

	if entry.offset+36 > uint32(len(t.data)) {
		return 0, fmt.Errorf("hhea table truncated")
	}

	// numberOfHMetrics is at offset 34 in the hhea table
	return binary.BigEndian.Uint16(t.data[entry.offset+34 : entry.offset+36]), nil
}

// subsetGlyfTable creates subsetted glyf and loca tables.
func (t *TrueTypeSubsetter) subsetGlyfTable() ([]byte, []byte, error) {
	var glyfData bytes.Buffer
	locaEntries := make([]uint32, 0, len(t.glyphOrder)+1)
	t.oldToNew = make(map[uint32]uint32)

	currentOffset := uint32(0)

	// Add glyphs in the order they were added (glyph 0 first if present)
	for _, oldGlyphID := range t.glyphOrder {
		if !t.glyphs[oldGlyphID] {
			continue
		}

		// Map old glyph ID to new glyph ID
		newGlyphID := uint32(len(locaEntries))
		t.oldToNew[oldGlyphID] = newGlyphID

		glyphData, err := t.getGlyphData(oldGlyphID)
		if err != nil {
			return nil, nil, err
		}

		locaEntries = append(locaEntries, currentOffset)

		// Write glyph data
		glyfData.Write(glyphData)
		currentOffset += uint32(len(glyphData))
	}

	// Add final loca entry
	locaEntries = append(locaEntries, currentOffset)

	return glyfData.Bytes(), locaEntriesToBytes(locaEntries, t.indexToLocFormat), nil
}

// locaEntriesToBytes converts loca entries to byte array.
func locaEntriesToBytes(entries []uint32, format int16) []byte {
	if format == 0 {
		// Short format (16-bit)
		data := make([]byte, len(entries)*2)
		for i, entry := range entries {
			if entry%2 != 0 {
				// Offset must be even for short format
				entry++
			}
			binary.BigEndian.PutUint16(data[i*2:i*2+2], uint16(entry/2))
		}
		return data
	}

	// Long format (32-bit)
	data := make([]byte, len(entries)*4)
	for i, entry := range entries {
		binary.BigEndian.PutUint32(data[i*4:i*4+4], entry)
	}
	return data
}

// subsetHmtxTable creates subsetted hmtx table.
func (t *TrueTypeSubsetter) subsetHmtxTable() ([]byte, error) {
	var hmtxData bytes.Buffer

	// Get number of HMetrics from original
	numHMetrics, err := t.getNumberOfHMetrics()
	if err != nil {
		return nil, err
	}

	for _, oldGlyphID := range t.glyphOrder {
		var metric horMetricEntry

		if oldGlyphID < uint32(numHMetrics) {
			metric = t.longHorMetric[oldGlyphID]
		} else {
			// Use last advanceWidth
			metric.advanceWidth = t.longHorMetric[numHMetrics-1].advanceWidth
			metric.lsb = t.lsbTable[oldGlyphID-uint32(numHMetrics)]
		}

		if err := binary.Write(&hmtxData, binary.BigEndian, metric.advanceWidth); err != nil {
			return nil, err
		}
		if err := binary.Write(&hmtxData, binary.BigEndian, uint16(metric.lsb)); err != nil {
			return nil, err
		}
	}

	return hmtxData.Bytes(), nil
}

// subsetMaxpTable creates subsetted maxp table.
func (t *TrueTypeSubsetter) subsetMaxpTable() []byte {
	entry, ok := t.tables["maxp"]
	if !ok {
		// Return basic maxp table if original doesn't exist
		data := make([]byte, 32)
		binary.BigEndian.PutUint32(data[0:4], 0x00010000)                // Version 1.0
		binary.BigEndian.PutUint16(data[4:6], uint16(len(t.glyphOrder))) // numGlyphs
		return data
	}

	if entry.offset+6 > uint32(len(t.data)) {
		return make([]byte, 32)
	}

	// Copy original maxp table
	maxpData := make([]byte, entry.length)
	copy(maxpData, t.data[entry.offset:entry.offset+entry.length])

	// Update numGlyphs at offset 4
	binary.BigEndian.PutUint16(maxpData[4:6], uint16(len(t.glyphOrder)))

	return maxpData
}

// subsetCmapTable creates subsetted cmap table.
func (t *TrueTypeSubsetter) subsetCmapTable() ([]byte, error) {
	entry, ok := t.tables["cmap"]
	if !ok {
		// Return empty cmap table
		return t.buildEmptyCmap(), nil
	}

	if entry.offset+entry.length > uint32(len(t.data)) {
		return t.buildEmptyCmap(), nil
	}

	cmapData := t.data[entry.offset : entry.offset+entry.length]

	// Parse cmap header
	if len(cmapData) < 4 {
		return t.buildEmptyCmap(), nil
	}

	numTables := binary.BigEndian.Uint16(cmapData[2:4])
	if len(cmapData) < int(4+numTables*8) {
		return t.buildEmptyCmap(), nil
	}

	// For simplicity, just copy the original cmap table
	// In a full implementation, you would:
	// 1. Parse each subtable
	// 2. Map character codes to new glyph IDs
	// 3. Rebuild only the mappings that point to used glyphs
	return cmapData, nil
}

// buildEmptyCmap builds a minimal cmap table.
func (t *TrueTypeSubsetter) buildEmptyCmap() []byte {
	data := make([]byte, 26) // Header + one minimal format-4 subtable header

	// Cmap header
	binary.BigEndian.PutUint16(data[0:2], 0) // Version
	binary.BigEndian.PutUint16(data[2:4], 1) // Number of tables

	// Subtable 0: Format 4 (Segment mapping to delta values)
	binary.BigEndian.PutUint16(data[4:6], 0)   // Platform ID (Unicode)
	binary.BigEndian.PutUint16(data[6:8], 4)   // Encoding ID
	binary.BigEndian.PutUint32(data[8:12], 12) // Offset to subtable

	// Format 4 subtable header (minimal - maps glyph 0 to char 0)
	binary.BigEndian.PutUint16(data[12:14], 4)  // Format
	binary.BigEndian.PutUint16(data[14:16], 26) // Length (including this header)
	binary.BigEndian.PutUint16(data[16:18], 0)  // Language
	binary.BigEndian.PutUint16(data[18:20], 2)  // segCountX2 (1 segment)
	binary.BigEndian.PutUint16(data[20:22], 1)  // searchRange
	binary.BigEndian.PutUint16(data[22:24], 0)  // entrySelector
	binary.BigEndian.PutUint16(data[24:26], 0)  // rangeShift

	return data
}

// buildSubsetFont builds the final subsetted font.
func (t *TrueTypeSubsetter) buildSubsetFont(glyf, loca, hmtx, maxp, cmap []byte) ([]byte, error) {
	var result bytes.Buffer

	// Create new tables map
	newTables := make(map[string][]byte)

	// Add subsetted tables
	newTables["glyf"] = PadTable(glyf)
	newTables["loca"] = PadTable(loca)
	newTables["hmtx"] = PadTable(hmtx)
	newTables["maxp"] = PadTable(maxp)
	newTables["cmap"] = PadTable(cmap)

	// Copy other required tables
	requiredTables := []string{"head", "hhea", "name", "post", "OS/2"}
	for _, tag := range requiredTables {
		if entry, ok := t.tables[tag]; ok {
			if entry.offset+entry.length <= uint32(len(t.data)) {
				data := make([]byte, entry.length)
				copy(data, t.data[entry.offset:entry.offset+entry.length])
				newTables[tag] = PadTable(data)
			}
		}
	}

	// Update head table checksum adjustment
	if headData, ok := newTables["head"]; ok && len(headData) > 12 {
		// Clear checksum adjustment (set to 0)
		copy(headData[8:12], []byte{0, 0, 0, 0})
		newTables["head"] = headData
	}

	// Build sorted tag list
	tags := make([]string, 0, len(newTables))
	for tag := range newTables {
		tags = append(tags, tag)
	}
	for i := 0; i < len(tags); i++ {
		for j := i + 1; j < len(tags); j++ {
			if tags[i] > tags[j] {
				tags[i], tags[j] = tags[j], tags[i]
			}
		}
	}

	// Write SFNT header
	numTables := uint16(len(newTables))
	if err := binary.Write(&result, binary.BigEndian, uint32(0x00010000)); err != nil { // SFNT version
		return nil, err
	}
	if err := binary.Write(&result, binary.BigEndian, numTables); err != nil {
		return nil, err
	}

	// Calculate search range
	searchRange := uint16(16)
	for (1 << (searchRange / 16)) < numTables {
		searchRange *= 2
	}
	entrySelector := uint16(0)
	for (1 << (entrySelector + 1)) <= numTables {
		entrySelector++
	}
	rangeShift := uint32(numTables)*16 - uint32(searchRange)

	if err := binary.Write(&result, binary.BigEndian, searchRange); err != nil {
		return nil, err
	}
	if err := binary.Write(&result, binary.BigEndian, entrySelector); err != nil {
		return nil, err
	}
	if err := binary.Write(&result, binary.BigEndian, rangeShift); err != nil {
		return nil, err
	}

	// Calculate table offset (after SFNT header and table directory)
	tableOffset := uint32(12 + len(newTables)*16)

	// Write table directory entries
	currentOffset := tableOffset
	for _, tag := range tags {
		data := newTables[tag]
		checksum := CalculateChecksum(data)

		if _, err := result.WriteString(tag); err != nil {
			return nil, err
		}
		if err := binary.Write(&result, binary.BigEndian, checksum); err != nil {
			return nil, err
		}
		if err := binary.Write(&result, binary.BigEndian, currentOffset); err != nil {
			return nil, err
		}
		if err := binary.Write(&result, binary.BigEndian, uint32(len(data))); err != nil {
			return nil, err
		}

		currentOffset += uint32(len(PadTable(data)))
	}

	// Write table data
	for _, tag := range tags {
		if _, err := result.Write(newTables[tag]); err != nil {
			return nil, err
		}
	}

	// Calculate and set head table checksum adjustment
	fontData := result.Bytes()
	checksumAdjustment := 0xB1B0AFBA - CalculateChecksum(fontData)

	// Find head table and update checksum adjustment
	for _, tag := range tags {
		if tag == "head" {
			// Skip to checksum adjustment field in head data
			for j, tag2 := range tags {
				if tag2 == "head" {
					dataOffset := uint32(12 + len(newTables)*16)
					for k := 0; k < j; k++ {
						dataOffset += uint32(len(PadTable(newTables[tags[k]])))
					}
					// checksum adjustment is at offset 8 in head table
					headOffset := dataOffset + 8
					if headOffset+4 <= uint32(len(fontData)) {
						binary.BigEndian.PutUint32(fontData[headOffset:headOffset+4], checksumAdjustment)
					}
					break
				}
			}
			break
		}
	}

	return fontData, nil
}

// CFFSubsetter handles CFF/Type1C font subsetting.
type CFFSubsetter struct {
	charStrings map[uint32][]byte
	subrs       map[uint32][]byte
	glyphs      map[uint32]bool
	oldToNew    map[uint32]uint32
	data        []byte
}

// NewCFFSubsetter creates a new CFF subsetter.
func NewCFFSubsetter(data []byte) *CFFSubsetter {
	return &CFFSubsetter{
		data:        data,
		charStrings: make(map[uint32][]byte),
		subrs:       make(map[uint32][]byte),
		glyphs:      make(map[uint32]bool),
		oldToNew:    make(map[uint32]uint32),
	}
}

// Subset creates a subset of the CFF font.
func (c *CFFSubsetter) Subset() ([]byte, error) {
	// CFF subsetting requires:
	// 1. Parse CFF header, name index, top dict, string index, global subrs
	// 2. Parse CharStrings index
	// 3. Extract only used CharStrings
	// 4. Update local and global subrs
	// 5. Rebuild CFF data

	// For now, return the original data
	return c.data, nil
}

// FontSubset interface for font-specific subsetting.
type FontSubset interface {
	// Subset creates a subset of the font data.
	Subset() ([]byte, error)
}

type subsetterAdapter struct {
	subsetter *Subsetter
}

// Subset is an exported API.
func (a *subsetterAdapter) Subset() ([]byte, error) {
	return a.subsetter.Subset()
}

// GetSubsetter returns the appropriate subsetter for the font type.
func GetSubsetter(font entity.Font) FontSubset {
	if font.IsCIDFont() {
		return &CIDSubsetter{font: font}
	}

	return &subsetterAdapter{subsetter: NewSubsetter(font)}
}

// CIDSubsetter handles CID-keyed font subsetting.
type CIDSubsetter struct {
	font entity.Font
}

// Subset creates a subset of the CID-keyed font.
func (c *CIDSubsetter) Subset() ([]byte, error) {
	return NewSubsetter(c.font).SubsetCIDFont()
}

// Helper functions for font subsetting

// CalculateChecksum calculates the checksum for TrueType table data.
func CalculateChecksum(data []byte) uint32 {
	var sum uint32
	for i := 0; i < len(data)-3; i += 4 {
		sum += binary.BigEndian.Uint32(data[i : i+4])
	}

	// Add remaining bytes
	remainder := len(data) % 4
	if remainder > 0 {
		var lastWord uint32
		for i := 0; i < remainder; i++ {
			lastWord |= uint32(data[len(data)-remainder+i]) << uint32((3-i)*8)
		}
		sum += lastWord
	}

	return sum
}

// PadTable pads table data to 4-byte boundary.
func PadTable(data []byte) []byte {
	padding := (4 - (len(data) % 4)) % 4
	if padding > 0 {
		padded := make([]byte, len(data)+padding)
		copy(padded, data)
		return padded
	}
	return data
}
