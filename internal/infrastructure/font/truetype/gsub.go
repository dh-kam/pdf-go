// Package truetype provides TrueType/OpenType font parsing and rendering.
//
//revive:disable:exported
package truetype

import (
	"encoding/binary"
	"fmt"
)

// GSUBTable represents the Glyph Substitution table.
// This table contains information for glyph substitution, including ligatures,
// contextual alternates, and other advanced typography features.
type GSUBTable struct {
	ScriptList        *ScriptList
	FeatureList       *FeatureList
	LookupList        *LookupList
	FeatureVariations *FeatureVariations
	Version           uint32
}

// GSUB Lookup Types
const (
	GSUBLookupSingle       = 1 // Single substitution
	GSUBLookupMultiple     = 2 // Multiple substitution
	GSUBLookupAlternate    = 3 // Alternate substitution
	GSUBLookupLigature     = 4 // Ligature substitution
	GSUBLookupContext      = 5 // Contextual substitution
	GSUBLookupChainContext = 6 // Chaining contextual substitution
	GSUBLookupExtension    = 7 // Extension substitution
	GSUBLookupReverseChain = 8 // Reverse chaining contextual substitution
)

// SingleSubstFormat1 represents single substitution format 1.
type SingleSubstFormat1 struct {
	Coverage     Coverage
	SubstFormat  uint16
	DeltaGlyphID int16
}

// LookupType is an exported API.
func (s *SingleSubstFormat1) LookupType() uint16 {
	return GSUBLookupSingle
}

// SingleSubstFormat2 represents single substitution format 2.
type SingleSubstFormat2 struct {
	Coverage    Coverage
	Substitutes []uint16
	SubstFormat uint16
	GlyphCount  uint16
}

// LookupType is an exported API.
func (s *SingleSubstFormat2) LookupType() uint16 {
	return GSUBLookupSingle
}

// MultipleSubstFormat1 represents multiple substitution format 1.
type MultipleSubstFormat1 struct {
	Coverage      Coverage
	Sequences     []Sequence
	SubstFormat   uint16
	SequenceCount uint16
}

// LookupType is an exported API.
func (m *MultipleSubstFormat1) LookupType() uint16 {
	return GSUBLookupMultiple
}

// Sequence represents a glyph sequence for multiple substitution.
type Sequence struct {
	Substitutes []uint16
	GlyphCount  uint16
}

// AlternateSubstFormat1 represents alternate substitution format 1.
type AlternateSubstFormat1 struct {
	Coverage     Coverage
	AlternateSet []AlternateSet
	SubstFormat  uint16
}

// LookupType is an exported API.
func (a *AlternateSubstFormat1) LookupType() uint16 {
	return GSUBLookupAlternate
}

// AlternateSet represents an alternate set.
type AlternateSet struct {
	Alternates     []uint16
	AlternateCount uint16
}

// LigatureSubstFormat1 represents ligature substitution format 1.
type LigatureSubstFormat1 struct {
	Coverage     Coverage
	LigatureSets []LigatureSet
	SubstFormat  uint16
	LigSetCount  uint16
}

// LookupType is an exported API.
func (l *LigatureSubstFormat1) LookupType() uint16 {
	return GSUBLookupLigature
}

// LigatureSet represents a ligature set.
type LigatureSet struct {
	Ligatures     []Ligature
	LigatureCount uint16
}

// Ligature represents a ligature.
type Ligature struct {
	Components []uint16
	LigGlyph   uint16
	CompCount  uint16
}

// ContextSubstFormat1 represents contextual substitution format 1.
type ContextSubstFormat1 struct {
	Coverage           Coverage
	SubstLookupRecords []SubstLookupRecord
	SubstFormat        uint16
	GlyphCount         uint16
	SubstCount         uint16
}

// LookupType is an exported API.
func (c *ContextSubstFormat1) LookupType() uint16 {
	return GSUBLookupContext
}

// SubstLookupRecord represents a substitution lookup record.
type SubstLookupRecord struct {
	SeqIndex    uint16
	LookupIndex uint16
}

// ChainContextSubstFormat1 represents chaining contextual substitution format 1.
type ChainContextSubstFormat1 struct {
	BacktrackCoverage   []*Coverage
	InputCoverage       []*Coverage
	LookAheadCoverage   []*Coverage
	SubstLookupRecords  []SubstLookupRecord
	SubstFormat         uint16
	GlyphCount          uint16
	SubstCount          uint16
	BacktrackGlyphCount uint16
	InputGlyphCount     uint16
	LookAheadGlyphCount uint16
}

// LookupType is an exported API.
func (c *ChainContextSubstFormat1) LookupType() uint16 {
	return GSUBLookupChainContext
}

// ChainContextSubstFormat2 represents chaining contextual substitution format 2 (class-based).
type ChainContextSubstFormat2 struct {
	BacktrackClassDef   *ClassDef
	InputClassDef       *ClassDef
	LookAheadClassDef   *ClassDef
	SubstLookupRecords  []SubstLookupRecord
	SubstFormat         uint16
	BacktrackGlyphCount uint16
	InputGlyphCount     uint16
	LookAheadGlyphCount uint16
	SubstCount          uint16
}

// LookupType is an exported API.
func (c *ChainContextSubstFormat2) LookupType() uint16 {
	return GSUBLookupChainContext
}

// ClassDef represents a class definition table.
type ClassDef struct {
	Classes     map[uint16]uint16
	ClassFormat uint16
}

// ReverseChainSubstFormat1 represents reverse chaining contextual substitution format 1.
type ReverseChainSubstFormat1 struct {
	BacktrackCoverage []*Coverage
	LookAheadCoverage []*Coverage
	SubstituteGlyphs  []uint16
	SubstFormat       uint16
	GlyphCount        uint16
	SubstCount        uint16
}

// LookupType is an exported API.
func (r *ReverseChainSubstFormat1) LookupType() uint16 {
	return GSUBLookupReverseChain
}

// Coverage represents a coverage table.
type Coverage struct {
	Glyphs         []uint16
	Ranges         []RangeRecord
	CoverageFormat uint16
}

// RangeRecord represents a range record in a coverage table.
type RangeRecord struct {
	Start              uint16
	End                uint16
	StartCoverageIndex uint16
}

// ExtensionSubstFormat1 represents extension substitution format 1.
type ExtensionSubstFormat1 struct {
	SubstFormat     uint16
	lookupType      uint16
	ExtensionOffset uint32
}

// LookupType is an exported API.
func (e *ExtensionSubstFormat1) LookupType() uint16 {
	return e.lookupType
}

// GenericGSUBSubTable represents a generic GSUB subtable.
type GenericGSUBSubTable struct {
	lookupType uint16
	Format     uint16
}

// LookupType is an exported API.
func (g *GenericGSUBSubTable) LookupType() uint16 {
	return g.lookupType
}

// ParseGSUBTable parses the GSUB (Glyph Substitution) table.
func ParseGSUBTable(data []byte) (*GSUBTable, error) {
	if len(data) < 10 {
		return nil, fmt.Errorf("GSUB table too short")
	}

	gsub := &GSUBTable{}

	// Read version and offsets
	gsub.Version = binary.BigEndian.Uint32(data[0:4])
	scriptListOffset := binary.BigEndian.Uint16(data[4:6])
	featureListOffset := binary.BigEndian.Uint16(data[6:8])
	lookupListOffset := binary.BigEndian.Uint16(data[8:10])
	featureVariationsOffset := uint32(0)

	if gsub.Version >= 0x00010000 {
		if len(data) >= 14 {
			featureVariationsOffset = binary.BigEndian.Uint32(data[10:14])
		}
	}

	// Parse ScriptList (reuse from GPOS)
	scriptListData := getTableData(data, uint32(scriptListOffset))
	if scriptListData != nil {
		scriptList, err := parseScriptList(scriptListData)
		if err != nil {
			return nil, err
		}
		gsub.ScriptList = scriptList
	}

	// Parse FeatureList (reuse from GPOS)
	featureListData := getTableData(data, uint32(featureListOffset))
	if featureListData != nil {
		featureList, err := parseFeatureList(featureListData)
		if err != nil {
			return nil, err
		}
		gsub.FeatureList = featureList
	}

	// Parse LookupList
	lookupListData := getTableData(data, uint32(lookupListOffset))
	if lookupListData != nil {
		lookupList, err := parseLookupList(lookupListData, false) // false for GSUB
		if err != nil {
			return nil, err
		}
		gsub.LookupList = lookupList
	}

	// Parse FeatureVariations if present
	if featureVariationsOffset > 0 {
		variationsData := getTableData(data, featureVariationsOffset)
		if variationsData != nil {
			variations, err := parseFeatureVariations(variationsData)
			if err != nil {
				return nil, err
			}
			gsub.FeatureVariations = variations
		}
	}

	return gsub, nil
}

// parseGSUBSubTable parses a GSUB subtable.
func parseGSUBSubTable(data []byte, offset uint32, lookupType uint16) (LookupSubTable, error) {
	if offset >= uint32(len(data)) {
		return nil, fmt.Errorf("subtable offset out of bounds")
	}

	// Read format
	substFormat := binary.BigEndian.Uint16(data[offset : offset+2])

	switch lookupType {
	case GSUBLookupSingle:
		return parseSingleSubstSubTable(data, offset, substFormat)
	case GSUBLookupMultiple:
		return parseMultipleSubstSubTable(data, offset, substFormat)
	case GSUBLookupAlternate:
		return parseAlternateSubstSubTable(data, offset, substFormat)
	case GSUBLookupLigature:
		return parseLigatureSubstSubTable(data, offset, substFormat)
	case GSUBLookupContext:
		return parseContextSubstSubTable(data, offset, substFormat)
	case GSUBLookupChainContext:
		return parseChainContextSubstSubTable(data, offset, substFormat)
	case GSUBLookupExtension:
		return parseExtensionSubstSubTable(data, offset, substFormat)
	case GSUBLookupReverseChain:
		return parseReverseChainSubstSubTable(data, offset, substFormat)
	default:
		// Return a generic placeholder for unimplemented types
		return &GenericGSUBSubTable{
			lookupType: lookupType,
			Format:     substFormat,
		}, nil
	}
}

// parseSingleSubstSubTable parses a single substitution subtable.
func parseSingleSubstSubTable(data []byte, offset uint32, format uint16) (LookupSubTable, error) {
	if format == 1 {
		if offset+6 > uint32(len(data)) {
			return nil, fmt.Errorf("single subst format 1 truncated")
		}

		subTable := &SingleSubstFormat1{
			SubstFormat: format,
		}

		// Parse coverage table
		coverageOffset := binary.BigEndian.Uint16(data[offset+4 : offset+6])
		coverageData := getTableData(data, offset+uint32(coverageOffset))
		if coverageData != nil {
			coverage, err := parseCoverage(coverageData)
			if err != nil {
				return nil, err
			}
			subTable.Coverage = coverage
		}

		// Parse delta glyph ID
		subTable.DeltaGlyphID = int16(binary.BigEndian.Uint16(data[offset+6 : offset+8]))

		return subTable, nil
	}

	// Format 2
	if offset+8 > uint32(len(data)) {
		return nil, fmt.Errorf("single subst format 2 truncated")
	}

	subTable := &SingleSubstFormat2{
		SubstFormat: format,
		GlyphCount:  binary.BigEndian.Uint16(data[offset+6 : offset+8]),
	}

	// Parse coverage table
	coverageOffset := binary.BigEndian.Uint16(data[offset+4 : offset+6])
	coverageData := getTableData(data, offset+uint32(coverageOffset))
	if coverageData != nil {
		coverage, err := parseCoverage(coverageData)
		if err != nil {
			return nil, err
		}
		subTable.Coverage = coverage
	}

	// Parse substitute glyphs
	offset += 8
	subTable.Substitutes = make([]uint16, subTable.GlyphCount)
	for i := uint16(0); i < subTable.GlyphCount; i++ {
		if offset+2 > uint32(len(data)) {
			return nil, fmt.Errorf("substitutes truncated")
		}
		subTable.Substitutes[i] = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
	}

	return subTable, nil
}

// parseMultipleSubstSubTable parses a multiple substitution subtable.
func parseMultipleSubstSubTable(data []byte, offset uint32, format uint16) (LookupSubTable, error) {
	if format != 1 {
		return nil, fmt.Errorf("unsupported multiple subst format: %d", format)
	}

	if offset+6 > uint32(len(data)) {
		return nil, fmt.Errorf("multiple subst format 1 truncated")
	}

	subTable := &MultipleSubstFormat1{
		SubstFormat: format,
	}

	// Parse coverage table
	coverageOffset := binary.BigEndian.Uint16(data[offset+4 : offset+6])
	coverageData := getTableData(data, offset+uint32(coverageOffset))
	if coverageData != nil {
		coverage, err := parseCoverage(coverageData)
		if err != nil {
			return nil, err
		}
		subTable.Coverage = coverage
	}

	offset += 6
	subTable.SequenceCount = binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	// Parse sequence offsets
	sequenceOffsets := make([]uint16, subTable.SequenceCount)
	for i := uint16(0); i < subTable.SequenceCount; i++ {
		if offset+2 > uint32(len(data)) {
			return nil, fmt.Errorf("sequence offset truncated")
		}
		sequenceOffsets[i] = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
	}

	// Parse sequences
	subTable.Sequences = make([]Sequence, subTable.SequenceCount)
	for i, seqOffset := range sequenceOffsets {
		seqData := getTableData(data, offset+uint32(seqOffset))
		if seqData == nil {
			continue
		}

		glyphCount := binary.BigEndian.Uint16(seqData[0:2])
		subTable.Sequences[i] = Sequence{
			GlyphCount: glyphCount,
		}

		// Parse substitute glyphs
		substitutes := make([]uint16, glyphCount)
		for j := uint16(0); j < glyphCount; j++ {
			if uint32(2+j*2) > uint32(len(seqData)) {
				break
			}
			substitutes[j] = binary.BigEndian.Uint16(seqData[2+j*2 : 2+j*2+2])
		}
		subTable.Sequences[i].Substitutes = substitutes
	}

	return subTable, nil
}

// parseAlternateSubstSubTable parses an alternate substitution subtable.
func parseAlternateSubstSubTable(data []byte, offset uint32, format uint16) (LookupSubTable, error) {
	if format != 1 {
		return nil, fmt.Errorf("unsupported alternate subst format: %d", format)
	}

	if offset+6 > uint32(len(data)) {
		return nil, fmt.Errorf("alternate subst format 1 truncated")
	}

	subTable := &AlternateSubstFormat1{
		SubstFormat: format,
	}

	// Parse coverage table
	coverageOffset := binary.BigEndian.Uint16(data[offset+4 : offset+6])
	coverageData := getTableData(data, offset+uint32(coverageOffset))
	if coverageData != nil {
		coverage, err := parseCoverage(coverageData)
		if err != nil {
			return nil, err
		}
		subTable.Coverage = coverage
	}

	offset += 6
	alternateSetCount := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	// Parse alternate set offsets
	alternateSetOffsets := make([]uint16, alternateSetCount)
	for i := uint16(0); i < alternateSetCount; i++ {
		if offset+2 > uint32(len(data)) {
			return nil, fmt.Errorf("alternate set offset truncated")
		}
		alternateSetOffsets[i] = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
	}

	// Parse alternate sets
	subTable.AlternateSet = make([]AlternateSet, alternateSetCount)
	for i, setOffset := range alternateSetOffsets {
		setData := getTableData(data, offset+uint32(setOffset))
		if setData == nil {
			continue
		}

		alternateCount := binary.BigEndian.Uint16(setData[0:2])
		subTable.AlternateSet[i] = AlternateSet{
			AlternateCount: alternateCount,
		}

		// Parse alternates
		alternates := make([]uint16, alternateCount)
		for j := uint16(0); j < alternateCount; j++ {
			if uint32(2+j*2) > uint32(len(setData)) {
				break
			}
			alternates[j] = binary.BigEndian.Uint16(setData[2+j*2 : 2+j*2+2])
		}
		subTable.AlternateSet[i].Alternates = alternates
	}

	return subTable, nil
}

// parseLigatureSubstSubTable parses a ligature substitution subtable.
func parseLigatureSubstSubTable(data []byte, offset uint32, format uint16) (LookupSubTable, error) {
	if format != 1 {
		return nil, fmt.Errorf("unsupported ligature subst format: %d", format)
	}

	if offset+6 > uint32(len(data)) {
		return nil, fmt.Errorf("ligature subst format 1 truncated")
	}

	subTable := &LigatureSubstFormat1{
		SubstFormat: format,
	}

	// Parse coverage table
	coverageOffset := binary.BigEndian.Uint16(data[offset+4 : offset+6])
	coverageData := getTableData(data, offset+uint32(coverageOffset))
	if coverageData != nil {
		coverage, err := parseCoverage(coverageData)
		if err != nil {
			return nil, err
		}
		subTable.Coverage = coverage
	}

	offset += 6
	subTable.LigSetCount = binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	// Parse ligature set offsets
	ligSetOffsets := make([]uint16, subTable.LigSetCount)
	for i := uint16(0); i < subTable.LigSetCount; i++ {
		if offset+2 > uint32(len(data)) {
			return nil, fmt.Errorf("ligature set offset truncated")
		}
		ligSetOffsets[i] = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
	}

	// Parse ligature sets
	subTable.LigatureSets = make([]LigatureSet, subTable.LigSetCount)
	for i, setOffset := range ligSetOffsets {
		setData := getTableData(data, offset+uint32(setOffset))
		if setData == nil {
			continue
		}

		ligCount := binary.BigEndian.Uint16(setData[0:2])
		subTable.LigatureSets[i] = LigatureSet{
			LigatureCount: ligCount,
		}

		// Parse ligature offsets
		ligOffsets := make([]uint16, ligCount)
		setOffset += 2
		for j := uint16(0); j < ligCount; j++ {
			if uint32(setOffset)+2 > uint32(len(setData)) {
				break
			}
			ligOffsets[j] = binary.BigEndian.Uint16(setData[setOffset : setOffset+2])
			setOffset += 2
		}

		// Parse ligatures
		ligatures := make([]Ligature, ligCount)
		for j, ligOffset := range ligOffsets {
			ligData := getTableData(setData, uint32(ligOffset))
			if ligData == nil || len(ligData) < 6 {
				continue
			}

			ligGlyph := binary.BigEndian.Uint16(ligData[0:2])
			compCount := binary.BigEndian.Uint16(ligData[2:4])

			ligatures[j] = Ligature{
				LigGlyph:  ligGlyph,
				CompCount: compCount,
			}

			// Parse components
			components := make([]uint16, compCount-1)
			for k := uint16(0); k < compCount-1; k++ {
				if uint32(4+k*2) > uint32(len(ligData)) {
					break
				}
				components[k] = binary.BigEndian.Uint16(ligData[4+k*2 : 4+k*2+2])
			}
			ligatures[j].Components = components
		}
		subTable.LigatureSets[i].Ligatures = ligatures
	}

	return subTable, nil
}

// parseContextSubstSubTable parses a contextual substitution subtable.
func parseContextSubstSubTable(data []byte, offset uint32, format uint16) (LookupSubTable, error) {
	if format != 1 {
		return nil, fmt.Errorf("unsupported context subst format: %d", format)
	}

	if offset+6 > uint32(len(data)) {
		return nil, fmt.Errorf("context subst format 1 truncated")
	}

	subTable := &ContextSubstFormat1{
		SubstFormat: format,
	}

	// Parse coverage table
	coverageOffset := binary.BigEndian.Uint16(data[offset+4 : offset+6])
	coverageData := getTableData(data, offset+uint32(coverageOffset))
	if coverageData != nil {
		coverage, err := parseCoverage(coverageData)
		if err != nil {
			return nil, err
		}
		subTable.Coverage = coverage
	}

	offset += 6
	glyphCount := binary.BigEndian.Uint16(data[offset : offset+2])
	subTable.GlyphCount = glyphCount

	offset += 2
	subTable.SubstCount = binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	// Parse subst lookup records
	subTable.SubstLookupRecords = make([]SubstLookupRecord, subTable.SubstCount)
	for i := uint16(0); i < subTable.SubstCount; i++ {
		if offset+4 > uint32(len(data)) {
			return nil, fmt.Errorf("subst lookup record truncated")
		}
		subTable.SubstLookupRecords[i] = SubstLookupRecord{
			SeqIndex:    binary.BigEndian.Uint16(data[offset : offset+2]),
			LookupIndex: binary.BigEndian.Uint16(data[offset+2 : offset+4]),
		}
		offset += 4
	}

	return subTable, nil
}

// parseChainContextSubstSubTable parses a chaining contextual substitution subtable.
func parseChainContextSubstSubTable(data []byte, offset uint32, format uint16) (LookupSubTable, error) {
	if format == 1 {
		return parseChainContextSubstFormat1(data, offset)
	}
	if format == 2 {
		return parseChainContextSubstFormat2(data, offset)
	}
	return nil, fmt.Errorf("unsupported chain context subst format: %d", format)
}

// parseChainContextSubstFormat1 parses chaining context format 1.
func parseChainContextSubstFormat1(data []byte, offset uint32) (LookupSubTable, error) {
	if offset+10 > uint32(len(data)) {
		return nil, fmt.Errorf("chain context subst format 1 truncated")
	}

	subTable := &ChainContextSubstFormat1{
		SubstFormat: 1,
	}

	backtrackCount := binary.BigEndian.Uint16(data[offset+4 : offset+6])
	inputCount := binary.BigEndian.Uint16(data[offset+6 : offset+8])
	lookAheadCount := binary.BigEndian.Uint16(data[offset+8 : offset+10])

	offset += 10

	// Parse backtrack coverage offsets
	backtrackOffsets := make([]uint16, backtrackCount)
	for i := uint16(0); i < backtrackCount; i++ {
		if offset+2 > uint32(len(data)) {
			return nil, fmt.Errorf("backtrack offset truncated")
		}
		backtrackOffsets[i] = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
	}

	// Parse backtrack coverages
	subTable.BacktrackCoverage = make([]*Coverage, backtrackCount)
	for i, covOffset := range backtrackOffsets {
		covData := getTableData(data, offset+uint32(covOffset))
		if covData != nil {
			coverage, err := parseCoverage(covData)
			if err != nil {
				return nil, err
			}
			subTable.BacktrackCoverage[i] = &coverage
		}
	}

	// Parse input coverage offsets
	inputOffsets := make([]uint16, inputCount)
	for i := uint16(0); i < inputCount; i++ {
		if offset+2 > uint32(len(data)) {
			return nil, fmt.Errorf("input offset truncated")
		}
		inputOffsets[i] = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
	}

	// Parse input coverages
	subTable.InputCoverage = make([]*Coverage, inputCount)
	for i, covOffset := range inputOffsets {
		covData := getTableData(data, offset+uint32(covOffset))
		if covData != nil {
			coverage, err := parseCoverage(covData)
			if err != nil {
				return nil, err
			}
			subTable.InputCoverage[i] = &coverage
		}
	}

	// Parse lookahead coverage offsets
	lookAheadOffsets := make([]uint16, lookAheadCount)
	for i := uint16(0); i < lookAheadCount; i++ {
		if offset+2 > uint32(len(data)) {
			return nil, fmt.Errorf("lookahead offset truncated")
		}
		lookAheadOffsets[i] = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
	}

	// Parse lookahead coverages
	subTable.LookAheadCoverage = make([]*Coverage, lookAheadCount)
	for i, covOffset := range lookAheadOffsets {
		covData := getTableData(data, offset+uint32(covOffset))
		if covData != nil {
			coverage, err := parseCoverage(covData)
			if err != nil {
				return nil, err
			}
			subTable.LookAheadCoverage[i] = &coverage
		}
	}

	// Parse subst count and records
	substCount := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	subTable.SubstLookupRecords = make([]SubstLookupRecord, substCount)
	for i := uint16(0); i < substCount; i++ {
		if offset+4 > uint32(len(data)) {
			return nil, fmt.Errorf("subst lookup record truncated")
		}
		subTable.SubstLookupRecords[i] = SubstLookupRecord{
			SeqIndex:    binary.BigEndian.Uint16(data[offset : offset+2]),
			LookupIndex: binary.BigEndian.Uint16(data[offset+2 : offset+4]),
		}
		offset += 4
	}

	return subTable, nil
}

// parseChainContextSubstFormat2 parses chaining context format 2 (class-based).
func parseChainContextSubstFormat2(data []byte, offset uint32) (LookupSubTable, error) {
	if offset+10 > uint32(len(data)) {
		return nil, fmt.Errorf("chain context subst format 2 truncated")
	}

	subTable := &ChainContextSubstFormat2{
		SubstFormat: 2,
	}

	backtrackClassDefOffset := binary.BigEndian.Uint16(data[offset+4 : offset+6])
	inputClassDefOffset := binary.BigEndian.Uint16(data[offset+6 : offset+8])
	lookAheadClassDefOffset := binary.BigEndian.Uint16(data[offset+8 : offset+10])

	// Parse class definitions
	if backtrackClassDefOffset > 0 {
		classDefData := getTableData(data, offset+uint32(backtrackClassDefOffset))
		if classDefData != nil {
			classDef, err := parseClassDef(classDefData)
			if err != nil {
				return nil, err
			}
			subTable.BacktrackClassDef = classDef
		}
	}

	if inputClassDefOffset > 0 {
		classDefData := getTableData(data, offset+uint32(inputClassDefOffset))
		if classDefData != nil {
			classDef, err := parseClassDef(classDefData)
			if err != nil {
				return nil, err
			}
			subTable.InputClassDef = classDef
		}
	}

	if lookAheadClassDefOffset > 0 {
		classDefData := getTableData(data, offset+uint32(lookAheadClassDefOffset))
		if classDefData != nil {
			classDef, err := parseClassDef(classDefData)
			if err != nil {
				return nil, err
			}
			subTable.LookAheadClassDef = classDef
		}
	}

	offset += 10
	substCount := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	subTable.SubstLookupRecords = make([]SubstLookupRecord, substCount)
	for i := uint16(0); i < substCount; i++ {
		if offset+4 > uint32(len(data)) {
			return nil, fmt.Errorf("subst lookup record truncated")
		}
		subTable.SubstLookupRecords[i] = SubstLookupRecord{
			SeqIndex:    binary.BigEndian.Uint16(data[offset : offset+2]),
			LookupIndex: binary.BigEndian.Uint16(data[offset+2 : offset+4]),
		}
		offset += 4
	}

	return subTable, nil
}

// parseExtensionSubstSubTable parses an extension substitution subtable.
func parseExtensionSubstSubTable(data []byte, offset uint32, format uint16) (LookupSubTable, error) {
	if format != 1 {
		return nil, fmt.Errorf("unsupported extension subst format: %d", format)
	}

	if offset+8 > uint32(len(data)) {
		return nil, fmt.Errorf("extension subst format 1 truncated")
	}

	subTable := &ExtensionSubstFormat1{
		SubstFormat:     format,
		lookupType:      binary.BigEndian.Uint16(data[offset+2 : offset+4]),
		ExtensionOffset: binary.BigEndian.Uint32(data[offset+4 : offset+8]),
	}

	return subTable, nil
}

// parseReverseChainSubstSubTable parses a reverse chaining substitution subtable.
func parseReverseChainSubstSubTable(data []byte, offset uint32, format uint16) (LookupSubTable, error) {
	if format != 1 {
		return nil, fmt.Errorf("unsupported reverse chain subst format: %d", format)
	}

	if offset+6 > uint32(len(data)) {
		return nil, fmt.Errorf("reverse chain subst format 1 truncated")
	}

	subTable := &ReverseChainSubstFormat1{
		SubstFormat: format,
	}

	glyphCount := binary.BigEndian.Uint16(data[offset+4 : offset+6])
	offset += 6

	// Parse backtrack coverage offsets
	subTable.BacktrackCoverage = make([]*Coverage, glyphCount)
	for i := uint16(0); i < glyphCount; i++ {
		if offset+2 > uint32(len(data)) {
			return nil, fmt.Errorf("backtrack offset truncated")
		}
		covOffset := binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2

		covData := getTableData(data, offset+uint32(covOffset))
		if covData != nil {
			coverage, err := parseCoverage(covData)
			if err != nil {
				return nil, err
			}
			subTable.BacktrackCoverage[i] = &coverage
		}
	}

	// Parse lookahead coverage offsets
	subTable.LookAheadCoverage = make([]*Coverage, glyphCount)
	for i := uint16(0); i < glyphCount; i++ {
		if offset+2 > uint32(len(data)) {
			return nil, fmt.Errorf("lookahead offset truncated")
		}
		covOffset := binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2

		covData := getTableData(data, offset+uint32(covOffset))
		if covData != nil {
			coverage, err := parseCoverage(covData)
			if err != nil {
				return nil, err
			}
			subTable.LookAheadCoverage[i] = &coverage
		}
	}

	// Parse substitute glyphs
	subTable.SubstituteGlyphs = make([]uint16, glyphCount)
	for i := uint16(0); i < glyphCount; i++ {
		if offset+2 > uint32(len(data)) {
			return nil, fmt.Errorf("substitute glyph truncated")
		}
		subTable.SubstituteGlyphs[i] = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
	}

	return subTable, nil
}

// parseCoverage parses a coverage table.
func parseCoverage(data []byte) (Coverage, error) {
	if len(data) < 2 {
		return Coverage{}, fmt.Errorf("coverage table too short")
	}

	coverage := Coverage{}
	coverage.CoverageFormat = binary.BigEndian.Uint16(data[0:2])

	if coverage.CoverageFormat == 1 {
		// List of glyphs
		glyphCount := binary.BigEndian.Uint16(data[2:4])
		coverage.Glyphs = make([]uint16, glyphCount)

		for i := uint16(0); i < glyphCount; i++ {
			offset := 4 + uint32(i)*2
			if offset+2 > uint32(len(data)) {
				return coverage, nil
			}
			coverage.Glyphs[i] = binary.BigEndian.Uint16(data[offset : offset+2])
		}
	} else if coverage.CoverageFormat == 2 {
		// Range-based coverage
		rangeCount := binary.BigEndian.Uint16(data[2:4])
		coverage.Ranges = make([]RangeRecord, rangeCount)

		for i := uint16(0); i < rangeCount; i++ {
			offset := 4 + uint32(i)*6
			if offset+6 > uint32(len(data)) {
				return coverage, nil
			}
			coverage.Ranges[i] = RangeRecord{
				Start:              binary.BigEndian.Uint16(data[offset : offset+2]),
				End:                binary.BigEndian.Uint16(data[offset+2 : offset+4]),
				StartCoverageIndex: binary.BigEndian.Uint16(data[offset+4 : offset+6]),
			}
		}
	}

	return coverage, nil
}

// parseClassDef parses a class definition table.
func parseClassDef(data []byte) (*ClassDef, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("class def table too short")
	}

	classDef := &ClassDef{}
	classFormat := binary.BigEndian.Uint16(data[0:2])
	classDef.ClassFormat = classFormat
	classDef.Classes = make(map[uint16]uint16)

	if classFormat == 1 {
		// Format 1: Class ranges
		classRangeCount := binary.BigEndian.Uint16(data[2:4])

		offset := 4
		for i := uint16(0); i < classRangeCount; i++ {
			if uint32(offset+6) > uint32(len(data)) {
				break
			}
			startGlyph := binary.BigEndian.Uint16(data[offset : offset+2])
			endGlyph := binary.BigEndian.Uint16(data[offset+2 : offset+4])
			class := binary.BigEndian.Uint16(data[offset+4 : offset+6])

			for glyphID := startGlyph; glyphID <= endGlyph; glyphID++ {
				classDef.Classes[glyphID] = class
			}
			offset += 6
		}
	} else if classFormat == 2 {
		// Format 2: Class ranges (more efficient)
		classRangeCount := binary.BigEndian.Uint16(data[2:4])

		offset := 4
		for i := uint16(0); i < classRangeCount; i++ {
			if uint32(offset+6) > uint32(len(data)) {
				break
			}
			start := binary.BigEndian.Uint16(data[offset : offset+2])
			end := binary.BigEndian.Uint16(data[offset+2 : offset+4])
			class := binary.BigEndian.Uint16(data[offset+4 : offset+6])

			for glyphID := start; glyphID <= end; glyphID++ {
				classDef.Classes[glyphID] = class
			}
			offset += 6
		}
	}

	return classDef, nil
}

// GetLigatureSubstitutions extracts ligature substitutions from the GSUB table.
func (g *GSUBTable) GetLigatureSubstitutions() (map[uint16][]Ligature, error) {
	if g.LookupList == nil {
		return nil, nil
	}

	ligatureMap := make(map[uint16][]Ligature)

	for _, lookup := range g.LookupList.Lookups {
		if lookup.GetLookupType() != GSUBLookupLigature {
			continue
		}

		for _, subTable := range lookup.SubTables {
			if ligatureSubst, ok := subTable.(*LigatureSubstFormat1); ok {
				// Extract ligatures from the ligature substitution
				for _, ligatureSet := range ligatureSubst.LigatureSets {
					for _, ligature := range ligatureSet.Ligatures {
						// Store ligatures - keyed by first component would be more complex
						// For now, store all ligatures
						if ligatureMap[ligature.LigGlyph] == nil {
							ligatureMap[ligature.LigGlyph] = []Ligature{}
						}
						ligatureMap[ligature.LigGlyph] = append(ligatureMap[ligature.LigGlyph], ligature)
					}
				}
			}
		}
	}

	return ligatureMap, nil
}

// ApplySingleSubstitution applies a single substitution to a glyph.
func (g *GSUBTable) ApplySingleSubstitution(glyphID uint16, lookupIndex uint16) (uint16, error) {
	if g.LookupList == nil || int(lookupIndex) >= len(g.LookupList.Lookups) {
		return glyphID, nil
	}

	lookup := g.LookupList.Lookups[lookupIndex]
	if lookup.GetLookupType() != GSUBLookupSingle {
		return glyphID, nil
	}

	for _, subTable := range lookup.SubTables {
		if singleSubst, ok := subTable.(*SingleSubstFormat1); ok {
			if singleSubst.Coverage.Contains(glyphID) {
				newGlyphID := uint32(int32(glyphID) + int32(singleSubst.DeltaGlyphID))
				return uint16(newGlyphID), nil
			}
		}
	}

	return glyphID, nil
}

// Contains checks if a glyph ID is in the coverage table.
func (c *Coverage) Contains(glyphID uint16) bool {
	if c.CoverageFormat == 1 {
		for _, glyph := range c.Glyphs {
			if glyph == glyphID {
				return true
			}
		}
	} else if c.CoverageFormat == 2 {
		for _, rng := range c.Ranges {
			if glyphID >= rng.Start && glyphID <= rng.End {
				return true
			}
		}
	}
	return false
}

//revive:enable:exported
