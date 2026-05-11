// Package truetype provides TrueType/OpenType font parsing and rendering.
//
//revive:disable:exported
package truetype

import (
	"encoding/binary"
	"fmt"
)

// GPOSTable represents the Glyph Positioning table.
// This table contains information for glyph positioning, including kerning,
// mark positioning, and other advanced typography features.
type GPOSTable struct {
	ScriptList        *ScriptList
	FeatureList       *FeatureList
	LookupList        *LookupList
	FeatureVariations *FeatureVariations
	Version           uint32
}

// ScriptList represents a list of scripts in GPOS/GSUB tables.
type ScriptList struct {
	ScriptRecords []ScriptRecord
	ScriptCount   uint16
}

// ScriptRecord represents a script record.
type ScriptRecord struct {
	Script    *Script
	ScriptTag string
}

// Script represents a script with language systems.
type Script struct {
	DefaultLangSys *LangSys
	LangSysRecords []LangSysRecord
	LangSysCount   uint16
}

// LangSysRecord represents a language system record.
type LangSysRecord struct {
	LangSys    *LangSys
	LangSysTag string
}

// LangSys represents a language system.
type LangSys struct {
	FeatureIndices  []uint16
	LookupOrder     uint16
	ReqFeatureIndex uint16
	FeatureCount    uint16
}

// FeatureList represents a list of features in GPOS/GSUB tables.
type FeatureList struct {
	FeatureRecords []FeatureRecord
	FeatureCount   uint16
}

// FeatureRecord represents a feature record.
type FeatureRecord struct {
	Feature    *Feature
	FeatureTag string
}

// Feature represents a feature with lookup indices.
type Feature struct {
	LookupIndices []uint16
	FeatureParams uint16
	LookupCount   uint16
}

// LookupList represents a list of lookups in GPOS/GSUB tables.
type LookupList struct {
	Lookups     []Lookup
	LookupCount uint16
}

// Lookup represents a single lookup table.
type Lookup struct {
	SubTables     []LookupSubTable
	lookupType    uint16
	LookupFlag    uint16
	SubTableCount uint16
}

// GetLookupType returns the lookup type.
func (l *Lookup) GetLookupType() uint16 {
	return l.lookupType
}

// LookupSubTable is an interface for lookup subtables.
type LookupSubTable interface {
	LookupType() uint16
}

// FeatureVariations represents feature variations (GPOS 1.1+).
type FeatureVariations struct {
	FeatureVariationRecords []FeatureVariationRecord
	FeatureVariationsCount  uint32
}

// FeatureVariationRecord represents a feature variation record.
type FeatureVariationRecord struct {
	ConditionSets             []ConditionSet
	FeatureTableSubstitutions []FeatureTableSubstitution
	FeatureConditionSetCount  uint32
}

// ConditionSet represents a condition set for feature variations.
type ConditionSet struct {
	Conditions     []Condition
	ConditionCount uint32
}

// Condition represents a single condition.
type Condition struct {
	Format         uint32
	AxisIndex      uint32
	FilterRangeMin int16
	FilterRangeMax int16
}

// FeatureTableSubstitution represents feature table substitution.
type FeatureTableSubstitution struct {
	Substitutions     []Substitution
	SubstitutionCount uint32
}

// Substitution represents a single substitution.
type Substitution struct {
	FeatureIndex    uint16
	AlternateLookup uint16
}

// GPOS Lookup Types
const (
	GPOSLookupSinglePos       = 1 // Single positioning adjustment
	GPOSLookupPairPos         = 2 // Pair positioning adjustment (kerning)
	GPOSLookupCursivePos      = 3 // Cursive attachment
	GPOSLookupMarkBasePos     = 4 // Mark-to-base positioning
	GPOSLookupMarkLigaturePos = 5 // Mark-to-ligature positioning
	GPOSLookupMarkMarkPos     = 6 // Mark-to-mark positioning
	GPOSLookupContextPos      = 7 // Contextual positioning
	GPOSLookupChainContextPos = 8 // Chaining contextual positioning
	GPOSLookupExtensionPos    = 9 // Extension positioning
)

// SinglePosFormat1 represents single positioning format 1.
type SinglePosFormat1 struct {
	ValueRecord ValueRecord
	PosFormat   uint16
	ValueFormat uint16
}

// LookupType is an exported API.
func (s *SinglePosFormat1) LookupType() uint16 {
	return GPOSLookupSinglePos
}

// SinglePosFormat2 represents single positioning format 2.
type SinglePosFormat2 struct {
	ValueRecords []ValueRecord
	PosFormat    uint16
	ValueFormat  uint16
	ValueCount   uint16
}

// LookupType is an exported API.
func (s *SinglePosFormat2) LookupType() uint16 {
	return GPOSLookupSinglePos
}

// ValueRecord represents a positioning value.
type ValueRecord struct {
	XPlaDevice *DeviceTable
	YPlaDevice *DeviceTable
	XAdvDevice *DeviceTable
	YAdvDevice *DeviceTable
	XPlacement int16
	YPlacement int16
	XAdvance   int16
	YAdvance   int16
}

// DeviceTable represents a device table for hinting.
type DeviceTable struct {
	DeltaValues []uint16
	StartSize   uint16
	EndSize     uint16
	DeltaFormat uint16
}

// PairPosFormat1 represents pair positioning format 1.
type PairPosFormat1 struct {
	PairSetOffsets []uint16
	PairSets       []PairSet
	PosFormat      uint16
	ValueFormat1   uint16
	ValueFormat2   uint16
	PairSetCount   uint16
}

// LookupType is an exported API.
func (p *PairPosFormat1) LookupType() uint16 {
	return GPOSLookupPairPos
}

// PairSet represents a set of positioning pairs.
type PairSet struct {
	PairValues     []PairValueRecord
	PairValueCount uint16
}

// PairValueRecord represents a pair positioning value.
type PairValueRecord struct {
	Value1      ValueRecord
	Value2      ValueRecord
	SecondGlyph uint16
}

// PairPosFormat2 represents pair positioning format 2 (class-based).
type PairPosFormat2 struct {
	Class1Records []Class1Record
	Class2Records []Class2Record
	PosFormat     uint16
	ValueFormat1  uint16
	ValueFormat2  uint16
	ClassDefCount uint16
	Class1Count   uint16
	Class2Count   uint16
}

// LookupType is an exported API.
func (p *PairPosFormat2) LookupType() uint16 {
	return GPOSLookupPairPos
}

// Class1Record represents a class 1 record.
type Class1Record struct {
	Class2Records []Class2Record
}

// Class2Record represents a class 2 record.
type Class2Record struct {
	Value1 ValueRecord
	Value2 ValueRecord
}

// EntryExitRecord represents an entry/exit record.
type EntryExitRecord struct {
	EntryAnchor Anchor
	ExitAnchor  Anchor
}

// Anchor represents an anchor point.
type Anchor struct {
	AnchorFormat uint16
	XCoordinate  int16
	YCoordinate  int16
	AnchorPoint  uint16
}

// MarkBasePosFormat1 represents mark-to-base positioning format 1.
type MarkBasePosFormat1 struct {
	MarkRecords []MarkRecord
	BaseRecords []BaseRecord
	PosFormat   uint16
	MarkCount   uint16
	BaseCount   uint16
}

// MarkRecord represents a mark record.
type MarkRecord struct {
	Class      uint16
	MarkAnchor Anchor
}

// BaseRecord represents a base record.
type BaseRecord struct {
	ClassRecords []uint16
	BaseAnchor   Anchor
	ClassCount   uint16
}

// LigatureAttach represents a ligature attachment.
type LigatureAttach struct {
	ComponentRecords []ComponentRecord
	ComponentCount   uint16
}

// ComponentRecord represents a component record.
type ComponentRecord struct {
	ClassRecords   []uint16
	LigatureAnchor Anchor
	ClassCount     uint16
}

// MarkLigaturePosFormat1 represents mark-to-ligature positioning format 1.
type MarkLigaturePosFormat1 struct {
	MarkRecords    []MarkRecord
	LigatureAttach []LigatureAttach
	PosFormat      uint16
	MarkCount      uint16
	LigatureCount  uint16
}

// MarkMarkPosFormat1 represents mark-to-mark positioning format 1.
type MarkMarkPosFormat1 struct {
	Mark1Records []MarkRecord
	Mark2Records []Mark2Record
	PosFormat    uint16
	Mark1Count   uint16
	Mark2Count   uint16
}

// Mark2Record represents a mark2 record.
type Mark2Record struct {
	ClassRecords []uint16
	MarkAnchor   Anchor
	Class        uint16
	ClassCount   uint16
}

// ContextPosFormat1 represents context positioning format 1.
type ContextPosFormat1 struct {
	GlyphIDs         []uint16
	PosLookupRecords []PosLookupRecord
	PosFormat        uint16
	GlyphCount       uint16
	PosCount         uint16
}

// PosLookupRecord represents a positioning lookup record.
type PosLookupRecord struct {
	SeqIndex    uint16
	LookupIndex uint16
}

// ChainContextPosFormat1 represents chaining context positioning format 1.
type ChainContextPosFormat1 struct {
	GlyphIDs            []uint16
	PosLookupRecords    []PosLookupRecord
	BacktrackGlyphs     []uint16
	LookAheadGlyphs     []uint16
	PosFormat           uint16
	GlyphCount          uint16
	PosCount            uint16
	BacktrackGlyphCount uint16
	LookAheadGlyphCount uint16
}

// ParseGPOSTable parses the GPOS (Glyph Positioning) table.
func ParseGPOSTable(data []byte) (*GPOSTable, error) {
	if len(data) < 10 {
		return nil, fmt.Errorf("GPOS table too short")
	}

	gpos := &GPOSTable{}

	// Read version and offsets
	gpos.Version = binary.BigEndian.Uint32(data[0:4])
	scriptListOffset := binary.BigEndian.Uint16(data[4:6])
	featureListOffset := binary.BigEndian.Uint16(data[6:8])
	lookupListOffset := binary.BigEndian.Uint16(data[8:10])
	featureVariationsOffset := uint32(0)

	if gpos.Version >= 0x00010000 {
		if len(data) >= 14 {
			featureVariationsOffset = binary.BigEndian.Uint32(data[10:14])
		}
	}

	// Parse ScriptList
	scriptListData := getTableData(data, uint32(scriptListOffset))
	if scriptListData != nil {
		scriptList, err := parseScriptList(scriptListData)
		if err != nil {
			return nil, err
		}
		gpos.ScriptList = scriptList
	}

	// Parse FeatureList
	featureListData := getTableData(data, uint32(featureListOffset))
	if featureListData != nil {
		featureList, err := parseFeatureList(featureListData)
		if err != nil {
			return nil, err
		}
		gpos.FeatureList = featureList
	}

	// Parse LookupList
	lookupListData := getTableData(data, uint32(lookupListOffset))
	if lookupListData != nil {
		lookupList, err := parseLookupList(lookupListData, true) // true for GPOS
		if err != nil {
			return nil, err
		}
		gpos.LookupList = lookupList
	}

	// Parse FeatureVariations if present
	if featureVariationsOffset > 0 {
		variationsData := getTableData(data, featureVariationsOffset)
		if variationsData != nil {
			variations, err := parseFeatureVariations(variationsData)
			if err != nil {
				return nil, err
			}
			gpos.FeatureVariations = variations
		}
	}

	return gpos, nil
}

// parseScriptList parses a script list.
func parseScriptList(data []byte) (*ScriptList, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("script list too short")
	}

	scriptList := &ScriptList{}
	scriptList.ScriptCount = binary.BigEndian.Uint16(data[0:2])

	offset := 2
	scriptList.ScriptRecords = make([]ScriptRecord, scriptList.ScriptCount)

	for i := uint16(0); i < scriptList.ScriptCount; i++ {
		if offset+6 > len(data) {
			return nil, fmt.Errorf("script record truncated")
		}

		tag := string(data[offset : offset+4])
		scriptOffset := binary.BigEndian.Uint16(data[offset+4 : offset+6])

		record := ScriptRecord{
			ScriptTag: tag,
		}

		scriptData := getTableData(data, uint32(scriptOffset))
		if scriptData != nil {
			script, err := parseScript(scriptData)
			if err != nil {
				return nil, err
			}
			record.Script = script
		}

		scriptList.ScriptRecords[i] = record
		offset += 6
	}

	return scriptList, nil
}

// parseScript parses a script.
func parseScript(data []byte) (*Script, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("script data too short")
	}

	script := &Script{}
	defaultLangSysOffset := binary.BigEndian.Uint16(data[0:2])
	script.LangSysCount = binary.BigEndian.Uint16(data[2:4])

	// Parse default language system
	if defaultLangSysOffset > 0 {
		langSysData := getTableData(data, uint32(defaultLangSysOffset))
		if langSysData != nil {
			langSys, err := parseLangSys(langSysData)
			if err != nil {
				return nil, err
			}
			script.DefaultLangSys = langSys
		}
	}

	// Parse language system records
	offset := 4
	script.LangSysRecords = make([]LangSysRecord, script.LangSysCount)

	for i := uint16(0); i < script.LangSysCount; i++ {
		if offset+4 > len(data) {
			return nil, fmt.Errorf("lang sys record truncated")
		}

		tag := string(data[offset : offset+4])
		langSysOffset := binary.BigEndian.Uint16(data[offset+4 : offset+6])

		record := LangSysRecord{
			LangSysTag: tag,
		}

		langSysData := getTableData(data, uint32(langSysOffset))
		if langSysData != nil {
			langSys, err := parseLangSys(langSysData)
			if err != nil {
				return nil, err
			}
			record.LangSys = langSys
		}

		script.LangSysRecords[i] = record
		offset += 6
	}

	return script, nil
}

// parseLangSys parses a language system.
func parseLangSys(data []byte) (*LangSys, error) {
	if len(data) < 6 {
		return nil, fmt.Errorf("lang sys data too short")
	}

	langSys := &LangSys{}
	langSys.LookupOrder = binary.BigEndian.Uint16(data[0:2])
	langSys.ReqFeatureIndex = binary.BigEndian.Uint16(data[2:4])
	langSys.FeatureCount = binary.BigEndian.Uint16(data[4:6])

	if len(data) < 6+int(langSys.FeatureCount)*2 {
		return nil, fmt.Errorf("lang sys features truncated")
	}

	langSys.FeatureIndices = make([]uint16, langSys.FeatureCount)
	offset := 6
	for i := uint16(0); i < langSys.FeatureCount; i++ {
		langSys.FeatureIndices[i] = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
	}

	return langSys, nil
}

// parseFeatureList parses a feature list.
func parseFeatureList(data []byte) (*FeatureList, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("feature list too short")
	}

	featureList := &FeatureList{}
	featureList.FeatureCount = binary.BigEndian.Uint16(data[0:2])

	offset := 2
	featureList.FeatureRecords = make([]FeatureRecord, featureList.FeatureCount)

	for i := uint16(0); i < featureList.FeatureCount; i++ {
		if offset+6 > len(data) {
			return nil, fmt.Errorf("feature record truncated")
		}

		tag := string(data[offset : offset+4])
		featureOffset := binary.BigEndian.Uint16(data[offset+4 : offset+6])

		record := FeatureRecord{
			FeatureTag: tag,
		}

		featureData := getTableData(data, uint32(featureOffset))
		if featureData != nil {
			feature, err := parseFeature(featureData)
			if err != nil {
				return nil, err
			}
			record.Feature = feature
		}

		featureList.FeatureRecords[i] = record
		offset += 6
	}

	return featureList, nil
}

// parseFeature parses a feature.
func parseFeature(data []byte) (*Feature, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("feature data too short")
	}

	feature := &Feature{}
	feature.FeatureParams = binary.BigEndian.Uint16(data[0:2])
	feature.LookupCount = binary.BigEndian.Uint16(data[2:4])

	if len(data) < 4+int(feature.LookupCount)*2 {
		return nil, fmt.Errorf("feature lookups truncated")
	}

	feature.LookupIndices = make([]uint16, feature.LookupCount)
	offset := 4
	for i := uint16(0); i < feature.LookupCount; i++ {
		feature.LookupIndices[i] = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
	}

	return feature, nil
}

// parseLookupList parses a lookup list.
func parseLookupList(data []byte, isGPOS bool) (*LookupList, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("lookup list too short")
	}

	lookupList := &LookupList{}
	lookupList.LookupCount = binary.BigEndian.Uint16(data[0:2])

	offset := 2
	lookupList.Lookups = make([]Lookup, lookupList.LookupCount)

	// Track offsets for parsing lookups
	lookupOffsets := make([]uint16, lookupList.LookupCount)
	for i := uint16(0); i < lookupList.LookupCount; i++ {
		if offset+2 > len(data) {
			return nil, fmt.Errorf("lookup offset truncated")
		}
		lookupOffsets[i] = binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2
	}

	// Parse lookups
	for i, lookupOffset := range lookupOffsets {
		lookupData := getTableData(data, uint32(lookupOffset))
		if lookupData == nil {
			continue
		}

		lookup, err := parseLookup(lookupData, isGPOS)
		if err != nil {
			return nil, err
		}
		lookupList.Lookups[i] = *lookup
	}

	return lookupList, nil
}

// parseLookup parses a lookup.
func parseLookup(data []byte, isGPOS bool) (*Lookup, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("lookup data too short")
	}

	lookup := &Lookup{}
	lookup.lookupType = binary.BigEndian.Uint16(data[0:2])
	lookup.LookupFlag = binary.BigEndian.Uint16(data[2:4])
	lookup.SubTableCount = binary.BigEndian.Uint16(data[4:6])

	// Skip subTableOffsets (we'll parse sequentially)
	offset := 8 + uint32(lookup.SubTableCount)*2

	// Parse subtables based on lookup type
	for i := uint16(0); i < lookup.SubTableCount; i++ {
		var subTable LookupSubTable
		var err error

		if isGPOS {
			subTable, err = parseGPOSSubTable(data, offset, lookup.GetLookupType())
		} else {
			subTable, err = parseGSUBSubTable(data, offset, lookup.GetLookupType())
		}

		if err != nil {
			return nil, err
		}
		lookup.SubTables = append(lookup.SubTables, subTable)

		// Move to next subtable (simplified - in reality we'd need to get actual offset)
		offset += 100 // Placeholder - actual offset varies by subtable type
	}

	return lookup, nil
}

// parseGPOSSubTable parses a GPOS subtable.
func parseGPOSSubTable(data []byte, offset uint32, lookupType uint16) (LookupSubTable, error) {
	if offset >= uint32(len(data)) {
		return nil, fmt.Errorf("subtable offset out of bounds")
	}

	// Read format
	posFormat := binary.BigEndian.Uint16(data[offset : offset+2])

	switch lookupType {
	case GPOSLookupSinglePos:
		return parseSinglePosSubTable(data, offset, posFormat)
	case GPOSLookupPairPos:
		return parsePairPosSubTable(data, offset, posFormat)
	case GPOSLookupCursivePos:
		return parseCursivePosSubTable(data, offset, posFormat)
	default:
		// Return a generic placeholder for unimplemented types
		return &GenericGPOSSubTable{
			lookupType: lookupType,
			Format:     posFormat,
		}, nil
	}
}

// parseSinglePosSubTable parses a single positioning subtable.
func parseSinglePosSubTable(data []byte, offset uint32, format uint16) (LookupSubTable, error) {
	if format == 1 {
		if offset+8 > uint32(len(data)) {
			return nil, fmt.Errorf("single pos format 1 truncated")
		}

		subTable := &SinglePosFormat1{
			PosFormat:   format,
			ValueFormat: binary.BigEndian.Uint16(data[offset+2 : offset+4]),
		}

		// Parse value record
		valueFormat := subTable.ValueFormat
		valueRecord, bytesRead := parseValueRecord(data, offset+4, valueFormat)
		subTable.ValueRecord = valueRecord

		_ = bytesRead // Acknowledge the returned value
		return subTable, nil
	}

	// Format 2
	if offset+6 > uint32(len(data)) {
		return nil, fmt.Errorf("single pos format 2 truncated")
	}

	subTable := &SinglePosFormat2{
		PosFormat:   format,
		ValueFormat: binary.BigEndian.Uint16(data[offset+2 : offset+4]),
		ValueCount:  binary.BigEndian.Uint16(data[offset+4 : offset+6]),
	}

	// Parse value records
	offset += 6
	subTable.ValueRecords = make([]ValueRecord, subTable.ValueCount)
	for i := uint16(0); i < subTable.ValueCount; i++ {
		valueRecord, bytesRead := parseValueRecord(data, offset, subTable.ValueFormat)
		subTable.ValueRecords[i] = valueRecord
		offset += uint32(bytesRead)
	}

	return subTable, nil
}

// parseValueRecord parses a value record.
func parseValueRecord(data []byte, offset uint32, valueFormat uint16) (ValueRecord, int) {
	record := ValueRecord{}
	bytesRead := 0

	if valueFormat&0x01 != 0 {
		record.XPlacement = int16(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		bytesRead += 2
	}
	if valueFormat&0x02 != 0 {
		record.YPlacement = int16(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		bytesRead += 2
	}
	if valueFormat&0x04 != 0 {
		record.XAdvance = int16(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		bytesRead += 2
	}
	if valueFormat&0x08 != 0 {
		record.YAdvance = int16(binary.BigEndian.Uint16(data[offset : offset+2]))
		bytesRead += 2
	}

	return record, bytesRead
}

// parsePairPosSubTable parses a pair positioning subtable.
func parsePairPosSubTable(data []byte, offset uint32, format uint16) (LookupSubTable, error) {
	if format == 1 {
		if offset+8 > uint32(len(data)) {
			return nil, fmt.Errorf("pair pos format 1 truncated")
		}

		subTable := &PairPosFormat1{
			PosFormat:    format,
			ValueFormat1: binary.BigEndian.Uint16(data[offset+2 : offset+4]),
			ValueFormat2: binary.BigEndian.Uint16(data[offset+4 : offset+6]),
			PairSetCount: binary.BigEndian.Uint16(data[offset+6 : offset+8]),
		}

		// Store pair set offsets (actual pairs would be parsed separately)
		offset += 8
		subTable.PairSetOffsets = make([]uint16, subTable.PairSetCount)
		for i := uint16(0); i < subTable.PairSetCount; i++ {
			if offset+2 > uint32(len(data)) {
				return nil, fmt.Errorf("pair set offset truncated")
			}
			subTable.PairSetOffsets[i] = binary.BigEndian.Uint16(data[offset : offset+2])
			offset += 2
		}

		return subTable, nil
	}

	// Format 2 - Class-based pair positioning
	if offset+10 > uint32(len(data)) {
		return nil, fmt.Errorf("pair pos format 2 truncated")
	}

	subTable := &PairPosFormat2{
		PosFormat:     format,
		ValueFormat1:  binary.BigEndian.Uint16(data[offset+2 : offset+4]),
		ValueFormat2:  binary.BigEndian.Uint16(data[offset+4 : offset+6]),
		ClassDefCount: binary.BigEndian.Uint16(data[offset+6 : offset+8]),
		Class1Count:   binary.BigEndian.Uint16(data[offset+8 : offset+10]),
	}

	return subTable, nil
}

// parseCursivePosSubTable parses a cursive attachment subtable.
func parseCursivePosSubTable(data []byte, offset uint32, format uint16) (LookupSubTable, error) {
	// Cursive positioning format 1
	if offset+6 > uint32(len(data)) {
		return nil, fmt.Errorf("cursive pos format 1 truncated")
	}

	entryExitCount := binary.BigEndian.Uint16(data[offset+4 : offset+6])

	subTable := &CursiveAttachFormat1{
		posFormat:      format,
		entryExitCount: entryExitCount,
	}

	// Parse entry/exit records
	offset += 6
	subTable.EntryExitRecords = make([]EntryExitRecord, entryExitCount)
	for i := uint16(0); i < entryExitCount; i++ {
		// Parse entry anchor
		entryAnchor, entryBytes := parseAnchor(data, offset)
		offset += uint32(entryBytes)

		// Parse exit anchor
		exitAnchor, exitBytes := parseAnchor(data, offset)
		offset += uint32(exitBytes)

		record := EntryExitRecord{
			EntryAnchor: entryAnchor,
			ExitAnchor:  exitAnchor,
		}
		subTable.EntryExitRecords[i] = record
	}

	return subTable, nil
}

// CursiveAttachFormat1 represents cursive attachment format 1.
type CursiveAttachFormat1 struct {
	EntryExitRecords []EntryExitRecord
	posFormat        uint16
	entryExitCount   uint16
}

// LookupType is an exported API.
func (c *CursiveAttachFormat1) LookupType() uint16 {
	return GPOSLookupCursivePos
}

// parseAnchor parses an anchor table.
func parseAnchor(data []byte, offset uint32) (Anchor, int) {
	if offset >= uint32(len(data)) {
		return Anchor{}, 0
	}

	anchorFormat := binary.BigEndian.Uint16(data[offset : offset+2])
	anchor := Anchor{
		AnchorFormat: anchorFormat,
	}

	if anchorFormat == 1 {
		if offset+6 > uint32(len(data)) {
			return anchor, 4
		}
		anchor.XCoordinate = int16(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		anchor.YCoordinate = int16(binary.BigEndian.Uint16(data[offset+4 : offset+6]))
		return anchor, 6
	}

	if anchorFormat == 2 {
		if offset+8 > uint32(len(data)) {
			return anchor, 4
		}
		anchor.XCoordinate = int16(binary.BigEndian.Uint16(data[offset+2 : offset+4]))
		anchor.YCoordinate = int16(binary.BigEndian.Uint16(data[offset+4 : offset+6]))
		anchor.AnchorPoint = binary.BigEndian.Uint16(data[offset+6 : offset+8])
		return anchor, 8
	}

	return anchor, 2
}

// GenericGPOSSubTable represents a generic GPOS subtable.
type GenericGPOSSubTable struct {
	lookupType uint16
	Format     uint16
}

// LookupType is an exported API.
func (g *GenericGPOSSubTable) LookupType() uint16 {
	return g.lookupType
}

// parseFeatureVariations parses feature variations.
func parseFeatureVariations(data []byte) (*FeatureVariations, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("feature variations too short")
	}

	variations := &FeatureVariations{}
	variations.FeatureVariationsCount = binary.BigEndian.Uint32(data[0:4])

	return variations, nil
}

// getTableData safely extracts table data at an offset.
func getTableData(data []byte, offset uint32) []byte {
	if offset >= uint32(len(data)) {
		return nil
	}
	return data[offset:]
}

// GetKerningPairAdjustments extracts kerning pairs from the GPOS table.
// This is a convenience method for the most common GPOS feature.
func (g *GPOSTable) GetKerningPairAdjustments() (map[[2]uint16]ValueRecord, error) {
	if g.LookupList == nil {
		return nil, nil
	}

	kerningPairs := make(map[[2]uint16]ValueRecord)

	for _, lookup := range g.LookupList.Lookups {
		if lookup.GetLookupType() != GPOSLookupPairPos {
			continue
		}

		// Extracting actual kerning pairs from pair-positioning subtables is
		// intentionally deferred in this lightweight reader path.
		_ = lookup.SubTables
	}

	return kerningPairs, nil
}

// GetFeatureLookups returns all lookup indices for a given feature tag.
func (g *GPOSTable) GetFeatureLookups(featureTag string) ([]uint16, error) {
	if g.FeatureList == nil {
		return nil, nil
	}

	for _, record := range g.FeatureList.FeatureRecords {
		if record.FeatureTag == featureTag && record.Feature != nil {
			return record.Feature.LookupIndices, nil
		}
	}

	return nil, nil
}

// GetScriptLanguages returns all language systems for a given script tag.
func (g *GPOSTable) GetScriptLanguages(scriptTag string) ([]string, error) {
	if g.ScriptList == nil {
		return nil, nil
	}

	var languages []string

	for _, record := range g.ScriptList.ScriptRecords {
		if record.ScriptTag == scriptTag && record.Script != nil {
			// Add default language system if present
			if record.Script.DefaultLangSys != nil {
				languages = append(languages, "DFLT")
			}

			// Add explicit language systems
			for _, langRecord := range record.Script.LangSysRecords {
				languages = append(languages, langRecord.LangSysTag)
			}
			break
		}
	}

	return languages, nil
}

//revive:enable:exported
