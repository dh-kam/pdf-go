package truetype

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseGPOSTableAndGSUBTable_HeaderPaths(t *testing.T) {
	gposData := make([]byte, 14)
	binary.BigEndian.PutUint32(gposData[0:4], 0x00010000)
	binary.BigEndian.PutUint16(gposData[4:6], 32)
	binary.BigEndian.PutUint16(gposData[6:8], 32)
	binary.BigEndian.PutUint16(gposData[8:10], 32)
	binary.BigEndian.PutUint32(gposData[10:14], 0)

	gpos, err := ParseGPOSTable(gposData)
	require.NoError(t, err)
	require.NotNil(t, gpos)
	assert.Equal(t, uint32(0x00010000), gpos.Version)
	assert.Nil(t, gpos.ScriptList)
	assert.Nil(t, gpos.FeatureList)
	assert.Nil(t, gpos.LookupList)

	gsubData := make([]byte, 14)
	binary.BigEndian.PutUint32(gsubData[0:4], 0x00010000)
	binary.BigEndian.PutUint16(gsubData[4:6], 40)
	binary.BigEndian.PutUint16(gsubData[6:8], 40)
	binary.BigEndian.PutUint16(gsubData[8:10], 40)
	binary.BigEndian.PutUint32(gsubData[10:14], 0)

	gsub, err := ParseGSUBTable(gsubData)
	require.NoError(t, err)
	require.NotNil(t, gsub)
	assert.Equal(t, uint32(0x00010000), gsub.Version)
	assert.Nil(t, gsub.ScriptList)
	assert.Nil(t, gsub.FeatureList)
	assert.Nil(t, gsub.LookupList)
}

func TestParseGPOSTableAndGSUBTable_ShortInput(t *testing.T) {
	_, err := ParseGPOSTable([]byte{0x00})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too short")

	_, err = ParseGSUBTable([]byte{0x00})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

func TestParseScriptFeatureAndLookupLists(t *testing.T) {
	scriptData := make([]byte, 18)
	binary.BigEndian.PutUint16(scriptData[0:2], 1) // scriptCount
	copy(scriptData[2:6], []byte("latn"))
	binary.BigEndian.PutUint16(scriptData[6:8], 8)
	binary.BigEndian.PutUint16(scriptData[8:10], 4) // default langsys offset
	binary.BigEndian.PutUint16(scriptData[10:12], 0)
	binary.BigEndian.PutUint16(scriptData[12:14], 0) // lookupOrder
	binary.BigEndian.PutUint16(scriptData[14:16], 0) // reqFeatureIndex
	binary.BigEndian.PutUint16(scriptData[16:18], 0) // featureCount

	scriptList, err := parseScriptList(scriptData)
	require.NoError(t, err)
	require.Len(t, scriptList.ScriptRecords, 1)
	assert.Equal(t, "latn", scriptList.ScriptRecords[0].ScriptTag)
	require.NotNil(t, scriptList.ScriptRecords[0].Script)
	require.NotNil(t, scriptList.ScriptRecords[0].Script.DefaultLangSys)

	featureData := make([]byte, 14)
	binary.BigEndian.PutUint16(featureData[0:2], 1) // featureCount
	copy(featureData[2:6], []byte("kern"))
	binary.BigEndian.PutUint16(featureData[6:8], 8)
	binary.BigEndian.PutUint16(featureData[8:10], 0) // featureParams
	binary.BigEndian.PutUint16(featureData[10:12], 1)
	binary.BigEndian.PutUint16(featureData[12:14], 7)

	featureList, err := parseFeatureList(featureData)
	require.NoError(t, err)
	require.Len(t, featureList.FeatureRecords, 1)
	assert.Equal(t, "kern", featureList.FeatureRecords[0].FeatureTag)
	require.NotNil(t, featureList.FeatureRecords[0].Feature)
	assert.Equal(t, []uint16{7}, featureList.FeatureRecords[0].Feature.LookupIndices)

	lookupData := make([]byte, 12)
	binary.BigEndian.PutUint16(lookupData[0:2], 1)
	binary.BigEndian.PutUint16(lookupData[2:4], 4)
	binary.BigEndian.PutUint16(lookupData[4:6], GPOSLookupSinglePos)
	binary.BigEndian.PutUint16(lookupData[6:8], 0)
	binary.BigEndian.PutUint16(lookupData[8:10], 0) // no subtables
	binary.BigEndian.PutUint16(lookupData[10:12], 0)

	lookupList, err := parseLookupList(lookupData, true)
	require.NoError(t, err)
	require.Len(t, lookupList.Lookups, 1)
	assert.Equal(t, uint16(GPOSLookupSinglePos), lookupList.Lookups[0].GetLookupType())
}

func TestParseListErrors(t *testing.T) {
	_, err := parseScriptList([]byte{0x00, 0x01, 0x6c, 0x61})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "script record truncated")

	_, err = parseFeatureList([]byte{0x00, 0x01, 0x6b, 0x65})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "feature record truncated")

	_, err = parseLookupList([]byte{0x00, 0x01, 0x00}, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lookup offset truncated")
}

func TestParseGPOSSubtables(t *testing.T) {
	format1 := []byte{
		0x00, 0x01, // posFormat
		0x00, 0x05, // valueFormat: XPlacement + XAdvance
		0x00, 0x0a, // XPlacement
		0x00, 0x14, // XAdvance
	}
	sub1, err := parseSinglePosSubTable(format1, 0, 1)
	require.NoError(t, err)
	s1, ok := sub1.(*SinglePosFormat1)
	require.True(t, ok)
	assert.Equal(t, int16(10), s1.ValueRecord.XPlacement)
	assert.Equal(t, int16(20), s1.ValueRecord.XAdvance)

	format2 := []byte{
		0x00, 0x02, // posFormat
		0x00, 0x01, // valueFormat: XPlacement
		0x00, 0x02, // valueCount
		0x00, 0x0b, // value #1
		0x00, 0x16, // value #2
	}
	sub2, err := parseSinglePosSubTable(format2, 0, 2)
	require.NoError(t, err)
	s2, ok := sub2.(*SinglePosFormat2)
	require.True(t, ok)
	require.Len(t, s2.ValueRecords, 2)
	assert.Equal(t, int16(11), s2.ValueRecords[0].XPlacement)
	assert.Equal(t, int16(22), s2.ValueRecords[1].XPlacement)

	pair1 := []byte{
		0x00, 0x01,
		0x00, 0x01,
		0x00, 0x00,
		0x00, 0x01, // pairSetCount
		0x00, 0x0a, // pair set offset
	}
	p1, err := parsePairPosSubTable(pair1, 0, 1)
	require.NoError(t, err)
	pairFmt1, ok := p1.(*PairPosFormat1)
	require.True(t, ok)
	assert.Equal(t, uint16(1), pairFmt1.PairSetCount)
	assert.Equal(t, []uint16{10}, pairFmt1.PairSetOffsets)

	pair2 := []byte{
		0x00, 0x02,
		0x00, 0x01,
		0x00, 0x02,
		0x00, 0x03,
		0x00, 0x04,
	}
	p2, err := parsePairPosSubTable(pair2, 0, 2)
	require.NoError(t, err)
	pairFmt2, ok := p2.(*PairPosFormat2)
	require.True(t, ok)
	assert.Equal(t, uint16(3), pairFmt2.ClassDefCount)
	assert.Equal(t, uint16(4), pairFmt2.Class1Count)

	cursive := []byte{
		0x00, 0x01, // posFormat
		0x00, 0x00,
		0x00, 0x01, // entryExitCount
		0x00, 0x01, 0x00, 0x05, 0x00, 0x06, // entry anchor (fmt1)
		0x00, 0x02, 0x00, 0x07, 0x00, 0x08, 0x00, 0x09, // exit anchor (fmt2)
	}
	c, err := parseCursivePosSubTable(cursive, 0, 1)
	require.NoError(t, err)
	cf1, ok := c.(*CursiveAttachFormat1)
	require.True(t, ok)
	require.Len(t, cf1.EntryExitRecords, 1)
	assert.Equal(t, int16(5), cf1.EntryExitRecords[0].EntryAnchor.XCoordinate)
	assert.Equal(t, uint16(9), cf1.EntryExitRecords[0].ExitAnchor.AnchorPoint)

	_, err = parsePairPosSubTable([]byte{0x00, 0x01}, 0, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "truncated")
}

func TestParseGSUBSubtables(t *testing.T) {
	single1 := []byte{
		0x00, 0x01, // substFormat
		0x00, 0x00,
		0x00, 0x00, // coverage offset (to self)
		0x00, 0x03, // deltaGlyphID
	}
	sub1, err := parseSingleSubstSubTable(single1, 0, 1)
	require.NoError(t, err)
	s1, ok := sub1.(*SingleSubstFormat1)
	require.True(t, ok)
	assert.Equal(t, int16(3), s1.DeltaGlyphID)

	single2 := []byte{
		0x00, 0x02, // substFormat
		0x00, 0x00,
		0x00, 0x00, // coverage offset (to self)
		0x00, 0x02, // glyphCount
		0x00, 0x10, // substitute #1
		0x00, 0x11, // substitute #2
	}
	sub2, err := parseSingleSubstSubTable(single2, 0, 2)
	require.NoError(t, err)
	s2, ok := sub2.(*SingleSubstFormat2)
	require.True(t, ok)
	assert.Equal(t, []uint16{16, 17}, s2.Substitutes)

	_, err = parseMultipleSubstSubTable([]byte{0x00, 0x02}, 0, 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")

	_, err = parseAlternateSubstSubTable([]byte{0x00, 0x02}, 0, 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")

	_, err = parseLigatureSubstSubTable([]byte{0x00, 0x02}, 0, 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")

	_, err = parseContextSubstSubTable([]byte{0x00, 0x02}, 0, 2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")

	_, err = parseChainContextSubstSubTable([]byte{0x00, 0x03}, 0, 3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")

	ext := []byte{
		0x00, 0x01, // format
		0x00, 0x04, // lookupType
		0x00, 0x00, 0x00, 0x20, // extensionOffset
	}
	extSub, err := parseExtensionSubstSubTable(ext, 0, 1)
	require.NoError(t, err)
	ext1, ok := extSub.(*ExtensionSubstFormat1)
	require.True(t, ok)
	assert.Equal(t, uint16(4), ext1.LookupType())

	reverse := []byte{
		0x00, 0x01, // format
		0x00, 0x00,
		0x00, 0x00, // glyphCount
	}
	revSub, err := parseReverseChainSubstSubTable(reverse, 0, 1)
	require.NoError(t, err)
	rev, ok := revSub.(*ReverseChainSubstFormat1)
	require.True(t, ok)
	assert.Empty(t, rev.SubstituteGlyphs)
}

func TestParseGSUBSubtables_SupportedFormatSuccessPaths(t *testing.T) {
	multiple := []byte{
		0x00, 0x01, // format
		0x00, 0x01, // coverage glyph count (coverage offset=0)
		0x00, 0x00, // coverage offset
		0x00, 0x01, // sequenceCount
		0x00, 0x00, // sequence offset (from current internal offset)
		0x00, 0x02, // sequence glyphCount
		0x00, 0x30, // substitute #1
		0x00, 0x31, // substitute #2
	}
	mSub, err := parseMultipleSubstSubTable(multiple, 0, 1)
	require.NoError(t, err)
	mf1, ok := mSub.(*MultipleSubstFormat1)
	require.True(t, ok)
	require.Len(t, mf1.Sequences, 1)
	assert.Equal(t, uint16(2), mf1.Sequences[0].GlyphCount)
	assert.Equal(t, []uint16{0x30, 0x31}, mf1.Sequences[0].Substitutes)

	alternate := []byte{
		0x00, 0x01, // format
		0x00, 0x01, // coverage glyph count (coverage offset=0)
		0x00, 0x00, // coverage offset
		0x00, 0x01, // alternateSetCount
		0x00, 0x00, // alternate set offset
		0x00, 0x02, // alternateCount
		0x00, 0x40,
		0x00, 0x41,
	}
	aSub, err := parseAlternateSubstSubTable(alternate, 0, 1)
	require.NoError(t, err)
	af1, ok := aSub.(*AlternateSubstFormat1)
	require.True(t, ok)
	require.Len(t, af1.AlternateSet, 1)
	assert.Equal(t, []uint16{0x40, 0x41}, af1.AlternateSet[0].Alternates)

	ligature := []byte{
		0x00, 0x01, // format
		0x00, 0x01, // coverage glyph count (coverage offset=0)
		0x00, 0x00, // coverage offset
		0x00, 0x01, // ligSetCount
		0x00, 0x00, // ligSet offset
		0x00, 0x01, // ligCount
		0x00, 0x04, // ligature offset (from ligSet data)
		0x00, 0x4d, // ligGlyph
		0x00, 0x02, // compCount
		0x00, 0x37, // component #1
	}
	lSub, err := parseLigatureSubstSubTable(ligature, 0, 1)
	require.NoError(t, err)
	lf1, ok := lSub.(*LigatureSubstFormat1)
	require.True(t, ok)
	require.Len(t, lf1.LigatureSets, 1)
	require.Len(t, lf1.LigatureSets[0].Ligatures, 1)
	assert.Equal(t, uint16(0x4d), lf1.LigatureSets[0].Ligatures[0].LigGlyph)
	assert.Equal(t, []uint16{0x37}, lf1.LigatureSets[0].Ligatures[0].Components)

	context := []byte{
		0x00, 0x01, // format
		0x00, 0x01, // coverage glyph count (coverage offset=0)
		0x00, 0x00, // coverage offset
		0x00, 0x02, // glyphCount
		0x00, 0x01, // substCount
		0x00, 0x01, // seqIndex
		0x00, 0x02, // lookupIndex
	}
	cSub, err := parseContextSubstSubTable(context, 0, 1)
	require.NoError(t, err)
	cf1, ok := cSub.(*ContextSubstFormat1)
	require.True(t, ok)
	assert.Equal(t, uint16(2), cf1.GlyphCount)
	require.Len(t, cf1.SubstLookupRecords, 1)
	assert.Equal(t, uint16(1), cf1.SubstLookupRecords[0].SeqIndex)
	assert.Equal(t, uint16(2), cf1.SubstLookupRecords[0].LookupIndex)

	chainFmt1 := []byte{
		0x00, 0x01, // format
		0x00, 0x00,
		0x00, 0x01, // backtrackCount
		0x00, 0x01, // inputCount
		0x00, 0x01, // lookAheadCount
		0x00, 0x00, // backtrack offset
		0x00, 0x00, // input offset
		0x00, 0x00, // lookahead offset
		0x00, 0x01, // substCount
		0x00, 0x00, // seqIndex
		0x00, 0x03, // lookupIndex
	}
	ch1, err := parseChainContextSubstSubTable(chainFmt1, 0, 1)
	require.NoError(t, err)
	chf1, ok := ch1.(*ChainContextSubstFormat1)
	require.True(t, ok)
	require.Len(t, chf1.SubstLookupRecords, 1)
	assert.Equal(t, uint16(3), chf1.SubstLookupRecords[0].LookupIndex)

	chainFmt2 := make([]byte, 32)
	binary.BigEndian.PutUint16(chainFmt2[0:2], 2)  // format
	binary.BigEndian.PutUint16(chainFmt2[4:6], 20) // backtrack class def offset
	binary.BigEndian.PutUint16(chainFmt2[10:12], 1)
	binary.BigEndian.PutUint16(chainFmt2[12:14], 0)
	binary.BigEndian.PutUint16(chainFmt2[14:16], 7)
	// classDef at offset 20: format1, rangeCount1, range 5..5 => class 2
	binary.BigEndian.PutUint16(chainFmt2[20:22], 1)
	binary.BigEndian.PutUint16(chainFmt2[22:24], 1)
	binary.BigEndian.PutUint16(chainFmt2[24:26], 5)
	binary.BigEndian.PutUint16(chainFmt2[26:28], 5)
	binary.BigEndian.PutUint16(chainFmt2[28:30], 2)

	ch2, err := parseChainContextSubstSubTable(chainFmt2, 0, 2)
	require.NoError(t, err)
	chf2, ok := ch2.(*ChainContextSubstFormat2)
	require.True(t, ok)
	require.Len(t, chf2.SubstLookupRecords, 1)
	assert.Equal(t, uint16(7), chf2.SubstLookupRecords[0].LookupIndex)
	require.NotNil(t, chf2.BacktrackClassDef)
	assert.Equal(t, uint16(2), chf2.BacktrackClassDef.Classes[5])

	reverse := make([]byte, 30)
	binary.BigEndian.PutUint16(reverse[0:2], 1)  // format
	binary.BigEndian.PutUint16(reverse[4:6], 1)  // glyphCount
	binary.BigEndian.PutUint16(reverse[6:8], 12) // backtrack coverage offset => 20
	binary.BigEndian.PutUint16(reverse[8:10], 14)
	binary.BigEndian.PutUint16(reverse[10:12], 0x33) // substitute glyph
	// backtrack coverage at 20
	binary.BigEndian.PutUint16(reverse[20:22], 1)
	binary.BigEndian.PutUint16(reverse[22:24], 1)
	binary.BigEndian.PutUint16(reverse[24:26], 0x21)
	// lookahead coverage at 24 (overlaps start intentionally, still valid parser path)
	binary.BigEndian.PutUint16(reverse[24:26], 1)
	binary.BigEndian.PutUint16(reverse[26:28], 1)
	binary.BigEndian.PutUint16(reverse[28:30], 0x22)

	rSub, err := parseReverseChainSubstSubTable(reverse, 0, 1)
	require.NoError(t, err)
	rf1, ok := rSub.(*ReverseChainSubstFormat1)
	require.True(t, ok)
	assert.Equal(t, []uint16{0x33}, rf1.SubstituteGlyphs)
}

func TestParseCoverageAndClassDef(t *testing.T) {
	coverageFmt1 := []byte{
		0x00, 0x01,
		0x00, 0x02,
		0x00, 0x05,
		0x00, 0x07,
	}
	c1, err := parseCoverage(coverageFmt1)
	require.NoError(t, err)
	assert.True(t, c1.Contains(5))
	assert.False(t, c1.Contains(6))

	coverageFmt2 := []byte{
		0x00, 0x02,
		0x00, 0x01,
		0x00, 0x0a,
		0x00, 0x0c,
		0x00, 0x00,
	}
	c2, err := parseCoverage(coverageFmt2)
	require.NoError(t, err)
	assert.True(t, c2.Contains(11))
	assert.False(t, c2.Contains(13))

	classDefFmt1 := []byte{
		0x00, 0x01,
		0x00, 0x01,
		0x00, 0x03, 0x00, 0x04, 0x00, 0x09, // 3..4 => class 9
	}
	cd1, err := parseClassDef(classDefFmt1)
	require.NoError(t, err)
	assert.Equal(t, uint16(9), cd1.Classes[3])
	assert.Equal(t, uint16(9), cd1.Classes[4])

	classDefFmt2 := []byte{
		0x00, 0x02,
		0x00, 0x01,
		0x00, 0x06, 0x00, 0x06, 0x00, 0x02, // 6 => class 2
	}
	cd2, err := parseClassDef(classDefFmt2)
	require.NoError(t, err)
	assert.Equal(t, uint16(2), cd2.Classes[6])

	_, err = parseClassDef([]byte{0x00})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too short")
}

func TestGPOSAndGSUBConvenienceMethods(t *testing.T) {
	gpos := &GPOSTable{
		FeatureList: &FeatureList{
			FeatureRecords: []FeatureRecord{
				{
					FeatureTag: "kern",
					Feature: &Feature{
						LookupIndices: []uint16{1, 3},
					},
				},
			},
		},
		ScriptList: &ScriptList{
			ScriptRecords: []ScriptRecord{
				{
					ScriptTag: "latn",
					Script: &Script{
						DefaultLangSys: &LangSys{},
						LangSysRecords: []LangSysRecord{
							{LangSysTag: "KOR "},
							{LangSysTag: "ENG "},
						},
					},
				},
			},
		},
		LookupList: &LookupList{
			Lookups: []Lookup{
				{lookupType: GPOSLookupPairPos},
			},
		},
	}

	lookups, err := gpos.GetFeatureLookups("kern")
	require.NoError(t, err)
	assert.Equal(t, []uint16{1, 3}, lookups)

	langs, err := gpos.GetScriptLanguages("latn")
	require.NoError(t, err)
	assert.Equal(t, []string{"DFLT", "KOR ", "ENG "}, langs)

	kerns, err := gpos.GetKerningPairAdjustments()
	require.NoError(t, err)
	require.NotNil(t, kerns)

	gsub := &GSUBTable{
		LookupList: &LookupList{
			Lookups: []Lookup{
				{
					lookupType: GSUBLookupLigature,
					SubTables: []LookupSubTable{
						&LigatureSubstFormat1{
							LigatureSets: []LigatureSet{
								{
									Ligatures: []Ligature{
										{LigGlyph: 101, Components: []uint16{11, 12}, CompCount: 3},
									},
								},
							},
						},
					},
				},
				{
					lookupType: GSUBLookupSingle,
					SubTables: []LookupSubTable{
						&SingleSubstFormat1{
							Coverage: Coverage{
								CoverageFormat: 1,
								Glyphs:         []uint16{20},
							},
							DeltaGlyphID: 2,
						},
					},
				},
			},
		},
	}

	ligatures, err := gsub.GetLigatureSubstitutions()
	require.NoError(t, err)
	require.Contains(t, ligatures, uint16(101))
	require.Len(t, ligatures[101], 1)

	substituted, err := gsub.ApplySingleSubstitution(20, 1)
	require.NoError(t, err)
	assert.Equal(t, uint16(22), substituted)

	noSub, err := gsub.ApplySingleSubstitution(99, 1)
	require.NoError(t, err)
	assert.Equal(t, uint16(99), noSub)
}

func TestLookupTypeMethodsAndDispatcherPaths(t *testing.T) {
	// GPOS lookup type methods.
	assert.Equal(t, uint16(GPOSLookupSinglePos), (&SinglePosFormat1{}).LookupType())
	assert.Equal(t, uint16(GPOSLookupSinglePos), (&SinglePosFormat2{}).LookupType())
	assert.Equal(t, uint16(GPOSLookupPairPos), (&PairPosFormat1{}).LookupType())
	assert.Equal(t, uint16(GPOSLookupPairPos), (&PairPosFormat2{}).LookupType())
	assert.Equal(t, uint16(GPOSLookupCursivePos), (&CursiveAttachFormat1{}).LookupType())
	assert.Equal(t, uint16(77), (&GenericGPOSSubTable{lookupType: 77}).LookupType())

	// GSUB lookup type methods.
	assert.Equal(t, uint16(GSUBLookupSingle), (&SingleSubstFormat1{}).LookupType())
	assert.Equal(t, uint16(GSUBLookupSingle), (&SingleSubstFormat2{}).LookupType())
	assert.Equal(t, uint16(GSUBLookupMultiple), (&MultipleSubstFormat1{}).LookupType())
	assert.Equal(t, uint16(GSUBLookupAlternate), (&AlternateSubstFormat1{}).LookupType())
	assert.Equal(t, uint16(GSUBLookupLigature), (&LigatureSubstFormat1{}).LookupType())
	assert.Equal(t, uint16(GSUBLookupContext), (&ContextSubstFormat1{}).LookupType())
	assert.Equal(t, uint16(GSUBLookupChainContext), (&ChainContextSubstFormat1{}).LookupType())
	assert.Equal(t, uint16(GSUBLookupChainContext), (&ChainContextSubstFormat2{}).LookupType())
	assert.Equal(t, uint16(GSUBLookupReverseChain), (&ReverseChainSubstFormat1{}).LookupType())
	assert.Equal(t, uint16(8), (&ExtensionSubstFormat1{lookupType: 8}).LookupType())
	assert.Equal(t, uint16(55), (&GenericGSUBSubTable{lookupType: 55}).LookupType())

	// Feature variations parser.
	v, err := parseFeatureVariations([]byte{0x00, 0x00, 0x00, 0x02})
	require.NoError(t, err)
	require.NotNil(t, v)
	assert.Equal(t, uint32(2), v.FeatureVariationsCount)
	_, err = parseFeatureVariations([]byte{0x00})
	require.Error(t, err)

	// Dispatcher out-of-bounds guards.
	_, err = parseGPOSSubTable([]byte{0x00}, 2, GPOSLookupSinglePos)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of bounds")
	_, err = parseGSUBSubTable([]byte{0x00}, 2, GSUBLookupSingle)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out of bounds")

	// GPOS dispatcher success and generic fallback.
	gposSingle := []byte{
		0x00, 0x01, // format
		0x00, 0x05, // value format
		0x00, 0x0a, // x placement
		0x00, 0x14, // x advance
	}
	gposSub, err := parseGPOSSubTable(gposSingle, 0, GPOSLookupSinglePos)
	require.NoError(t, err)
	_, ok := gposSub.(*SinglePosFormat1)
	require.True(t, ok)

	gposPair := []byte{0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x08}
	gposSub, err = parseGPOSSubTable(gposPair, 0, GPOSLookupPairPos)
	require.NoError(t, err)
	_, ok = gposSub.(*PairPosFormat1)
	require.True(t, ok)

	gposCursive := []byte{
		0x00, 0x01,
		0x00, 0x00,
		0x00, 0x01,
		0x00, 0x01, 0x00, 0x01, 0x00, 0x01,
		0x00, 0x01, 0x00, 0x02, 0x00, 0x03,
	}
	gposSub, err = parseGPOSSubTable(gposCursive, 0, GPOSLookupCursivePos)
	require.NoError(t, err)
	_, ok = gposSub.(*CursiveAttachFormat1)
	require.True(t, ok)

	gposGeneric, err := parseGPOSSubTable([]byte{0x00, 0x09}, 0, 99)
	require.NoError(t, err)
	_, ok = gposGeneric.(*GenericGPOSSubTable)
	require.True(t, ok)

	// GSUB dispatcher success and generic fallback.
	gsubSingle := []byte{0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}
	gsubSub, err := parseGSUBSubTable(gsubSingle, 0, GSUBLookupSingle)
	require.NoError(t, err)
	_, ok = gsubSub.(*SingleSubstFormat1)
	require.True(t, ok)

	gsubMultiple := []byte{
		0x00, 0x01, 0x00, 0x01, 0x00, 0x00,
		0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x20,
	}
	gsubSub, err = parseGSUBSubTable(gsubMultiple, 0, GSUBLookupMultiple)
	require.NoError(t, err)
	_, ok = gsubSub.(*MultipleSubstFormat1)
	require.True(t, ok)

	gsubAlternate := []byte{
		0x00, 0x01, 0x00, 0x01, 0x00, 0x00,
		0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x21,
	}
	gsubSub, err = parseGSUBSubTable(gsubAlternate, 0, GSUBLookupAlternate)
	require.NoError(t, err)
	_, ok = gsubSub.(*AlternateSubstFormat1)
	require.True(t, ok)

	gsubLigature := []byte{
		0x00, 0x01, 0x00, 0x01, 0x00, 0x00,
		0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x04,
		0x00, 0x2a, 0x00, 0x02, 0x00, 0x2b,
	}
	gsubSub, err = parseGSUBSubTable(gsubLigature, 0, GSUBLookupLigature)
	require.NoError(t, err)
	_, ok = gsubSub.(*LigatureSubstFormat1)
	require.True(t, ok)

	gsubContext := []byte{
		0x00, 0x01, 0x00, 0x01, 0x00, 0x00,
		0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	}
	gsubSub, err = parseGSUBSubTable(gsubContext, 0, GSUBLookupContext)
	require.NoError(t, err)
	_, ok = gsubSub.(*ContextSubstFormat1)
	require.True(t, ok)

	gsubChain1 := []byte{
		0x00, 0x01, 0x00, 0x00,
		0x00, 0x01, 0x00, 0x01, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	}
	gsubSub, err = parseGSUBSubTable(gsubChain1, 0, GSUBLookupChainContext)
	require.NoError(t, err)
	_, ok = gsubSub.(*ChainContextSubstFormat1)
	require.True(t, ok)

	gsubChain2 := make([]byte, 30)
	binary.BigEndian.PutUint16(gsubChain2[0:2], 2)
	binary.BigEndian.PutUint16(gsubChain2[4:6], 20)
	binary.BigEndian.PutUint16(gsubChain2[10:12], 1)
	binary.BigEndian.PutUint16(gsubChain2[12:14], 0)
	binary.BigEndian.PutUint16(gsubChain2[14:16], 2)
	binary.BigEndian.PutUint16(gsubChain2[20:22], 1)
	binary.BigEndian.PutUint16(gsubChain2[22:24], 1)
	binary.BigEndian.PutUint16(gsubChain2[24:26], 4)
	binary.BigEndian.PutUint16(gsubChain2[26:28], 4)
	binary.BigEndian.PutUint16(gsubChain2[28:30], 3)
	gsubSub, err = parseGSUBSubTable(gsubChain2, 0, GSUBLookupChainContext)
	require.NoError(t, err)
	_, ok = gsubSub.(*ChainContextSubstFormat2)
	require.True(t, ok)

	gsubExt := []byte{0x00, 0x01, 0x00, 0x07, 0x00, 0x00, 0x00, 0x10}
	gsubSub, err = parseGSUBSubTable(gsubExt, 0, GSUBLookupExtension)
	require.NoError(t, err)
	_, ok = gsubSub.(*ExtensionSubstFormat1)
	require.True(t, ok)

	gsubReverse := make([]byte, 30)
	binary.BigEndian.PutUint16(gsubReverse[0:2], 1)
	binary.BigEndian.PutUint16(gsubReverse[4:6], 1)
	binary.BigEndian.PutUint16(gsubReverse[6:8], 12)
	binary.BigEndian.PutUint16(gsubReverse[8:10], 14)
	binary.BigEndian.PutUint16(gsubReverse[10:12], 0x44)
	binary.BigEndian.PutUint16(gsubReverse[20:22], 1)
	binary.BigEndian.PutUint16(gsubReverse[22:24], 1)
	binary.BigEndian.PutUint16(gsubReverse[24:26], 0x31)
	binary.BigEndian.PutUint16(gsubReverse[24:26], 1)
	binary.BigEndian.PutUint16(gsubReverse[26:28], 1)
	binary.BigEndian.PutUint16(gsubReverse[28:30], 0x32)
	gsubSub, err = parseGSUBSubTable(gsubReverse, 0, GSUBLookupReverseChain)
	require.NoError(t, err)
	_, ok = gsubSub.(*ReverseChainSubstFormat1)
	require.True(t, ok)

	gsubGeneric, err := parseGSUBSubTable([]byte{0x00, 0x09}, 0, 99)
	require.NoError(t, err)
	_, ok = gsubGeneric.(*GenericGSUBSubTable)
	require.True(t, ok)
}
