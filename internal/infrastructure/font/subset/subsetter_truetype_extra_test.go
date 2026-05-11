package subset

import (
	"bytes"
	"encoding/binary"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrueTypeSubsetter_Subset_FullFlow(t *testing.T) {
	data := buildSubsetTestSFNT(t, 0)
	sub := NewTrueTypeSubsetter(data)
	sub.glyphs[1] = true // glyph 0 should be auto-added by ensureGlyphOrder.

	out, err := sub.Subset()
	require.NoError(t, err)
	require.NotEmpty(t, out)

	require.NotEmpty(t, sub.glyphOrder)
	assert.Equal(t, uint32(0), sub.glyphOrder[0])
	assert.Equal(t, uint32(0), sub.oldToNew[0])
	assert.Equal(t, uint32(1), sub.oldToNew[1])
}

func TestTrueTypeSubsetter_ParseTableDirectory_Branches(t *testing.T) {
	short := NewTrueTypeSubsetter([]byte{0x00, 0x01})
	require.Error(t, short.parseTableDirectory())

	badVersion := make([]byte, 12)
	binary.BigEndian.PutUint32(badVersion[0:4], 0xDEADBEEF)
	binary.BigEndian.PutUint16(badVersion[4:6], 0)
	require.Error(t, NewTrueTypeSubsetter(badVersion).parseTableDirectory())

	truncatedDir := make([]byte, 20)
	binary.BigEndian.PutUint32(truncatedDir[0:4], 0x00010000)
	binary.BigEndian.PutUint16(truncatedDir[4:6], 1)
	require.Error(t, NewTrueTypeSubsetter(truncatedDir).parseTableDirectory())

	okData := buildSubsetTestSFNT(t, 0)
	tt := NewTrueTypeSubsetter(okData)
	require.NoError(t, tt.parseTableDirectory())
	_, hasHead := tt.tables["head"]
	assert.True(t, hasHead)
}

func TestTrueTypeSubsetter_ParseTables_AndHelpers(t *testing.T) {
	t.Run("missing tables", func(t *testing.T) {
		tt := NewTrueTypeSubsetter([]byte{})
		require.Error(t, tt.parseHeadTable())
		require.Error(t, tt.parseMaxpTable())
		require.Error(t, tt.parseLocaTable())
		require.Error(t, tt.parseGlyfTable())
		require.Error(t, tt.parseHmtxTable())
		_, err := tt.getNumberOfHMetrics()
		require.Error(t, err)
	})

	t.Run("short and long loca parsing", func(t *testing.T) {
		shortData := buildSubsetTestSFNT(t, 0)
		shortTT := NewTrueTypeSubsetter(shortData)
		require.NoError(t, shortTT.parseTableDirectory())
		require.NoError(t, shortTT.parseHeadTable())
		require.NoError(t, shortTT.parseMaxpTable())
		require.NoError(t, shortTT.parseLocaTable())
		assert.Equal(t, []uint32{0, 2, 4}, shortTT.locaTable)

		longData := buildSubsetTestSFNT(t, 1)
		longTT := NewTrueTypeSubsetter(longData)
		require.NoError(t, longTT.parseTableDirectory())
		require.NoError(t, longTT.parseHeadTable())
		require.NoError(t, longTT.parseMaxpTable())
		require.NoError(t, longTT.parseLocaTable())
		assert.Equal(t, []uint32{0, 2, 4}, longTT.locaTable)
	})

	t.Run("get glyph data", func(t *testing.T) {
		tt := &TrueTypeSubsetter{
			locaTable: []uint32{0, 2, 4},
			glyfTable: []byte{0xAA, 0xBB, 0xCC, 0xDD},
		}

		g, err := tt.getGlyphData(1)
		require.NoError(t, err)
		assert.Equal(t, []byte{0xCC, 0xDD}, g)

		_, err = tt.getGlyphData(3)
		require.Error(t, err)

		tt.locaTable = []uint32{0, 2, 8}
		_, err = tt.getGlyphData(1)
		require.Error(t, err)
	})
}

func TestTrueTypeSubsetter_SubsetHelpers_Branches(t *testing.T) {
	tt := NewTrueTypeSubsetter(buildSubsetTestSFNT(t, 0))
	require.NoError(t, tt.parseTableDirectory())
	require.NoError(t, tt.parseRequiredTables())

	tt.glyphs = map[uint32]bool{0: true, 1: true}
	tt.glyphOrder = []uint32{0, 1}

	glyf, loca, err := tt.subsetGlyfTable()
	require.NoError(t, err)
	require.NotEmpty(t, glyf)
	require.NotEmpty(t, loca)

	hmtx, err := tt.subsetHmtxTable()
	require.NoError(t, err)
	assert.Equal(t, 8, len(hmtx))

	maxp := tt.subsetMaxpTable()
	require.GreaterOrEqual(t, len(maxp), 6)
	assert.Equal(t, uint16(2), binary.BigEndian.Uint16(maxp[4:6]))

	cmap, err := tt.subsetCmapTable()
	require.NoError(t, err)
	require.NotEmpty(t, cmap)

	emptyCmap := tt.buildEmptyCmap()
	assert.Equal(t, 26, len(emptyCmap))

	fontData, err := tt.buildSubsetFont(glyf, loca, hmtx, maxp, cmap)
	require.NoError(t, err)
	require.NotEmpty(t, fontData)
}

func TestTrueTypeSubsetter_SubsetMaxpAndCmapFallbacks(t *testing.T) {
	tt := &TrueTypeSubsetter{
		data:       []byte{0x00},
		tables:     map[string]tableEntry{},
		glyphOrder: []uint32{0, 1, 2},
	}

	maxp := tt.subsetMaxpTable()
	require.Equal(t, 32, len(maxp))
	assert.Equal(t, uint16(3), binary.BigEndian.Uint16(maxp[4:6]))

	cmap, err := tt.subsetCmapTable()
	require.NoError(t, err)
	assert.Equal(t, 26, len(cmap))

	tt.tables["cmap"] = tableEntry{offset: 0, length: 9}
	cmap, err = tt.subsetCmapTable()
	require.NoError(t, err)
	assert.Equal(t, 26, len(cmap))

	shortHeader := make([]byte, 3)
	tt.data = shortHeader
	tt.tables["cmap"] = tableEntry{offset: 0, length: uint32(len(shortHeader))}
	cmap, err = tt.subsetCmapTable()
	require.NoError(t, err)
	assert.Equal(t, 26, len(cmap))
}

func TestLocaEntriesToBytes_Formats(t *testing.T) {
	short := locaEntriesToBytes([]uint32{0, 3, 8}, 0)
	require.Equal(t, 6, len(short))
	assert.Equal(t, uint16(0), binary.BigEndian.Uint16(short[0:2]))
	// 3 gets aligned to 4 for short format, then /2.
	assert.Equal(t, uint16(2), binary.BigEndian.Uint16(short[2:4]))

	long := locaEntriesToBytes([]uint32{0, 3, 8}, 1)
	require.Equal(t, 12, len(long))
	assert.Equal(t, uint32(3), binary.BigEndian.Uint32(long[4:8]))
}

func buildSubsetTestSFNT(t *testing.T, indexToLocFormat int16) []byte {
	t.Helper()

	head := make([]byte, 54)
	binary.BigEndian.PutUint32(head[0:4], 0x00010000)
	binary.BigEndian.PutUint16(head[50:52], uint16(indexToLocFormat))

	maxp := make([]byte, 32)
	binary.BigEndian.PutUint32(maxp[0:4], 0x00010000)
	binary.BigEndian.PutUint16(maxp[4:6], 2) // numGlyphs

	hhea := make([]byte, 36)
	binary.BigEndian.PutUint16(hhea[34:36], 1) // numberOfHMetrics

	var loca []byte
	if indexToLocFormat == 0 {
		loca = make([]byte, 6) // 3 entries * uint16
		binary.BigEndian.PutUint16(loca[0:2], 0)
		binary.BigEndian.PutUint16(loca[2:4], 1) // actual offset 2
		binary.BigEndian.PutUint16(loca[4:6], 2) // actual offset 4
	} else {
		loca = make([]byte, 12) // 3 entries * uint32
		binary.BigEndian.PutUint32(loca[0:4], 0)
		binary.BigEndian.PutUint32(loca[4:8], 2)
		binary.BigEndian.PutUint32(loca[8:12], 4)
	}

	glyf := []byte{0xAA, 0xBB, 0xCC, 0xDD}

	hmtx := make([]byte, 6)
	binary.BigEndian.PutUint16(hmtx[0:2], 500) // glyph0 advanceWidth
	binary.BigEndian.PutUint16(hmtx[2:4], 10)  // glyph0 lsb
	binary.BigEndian.PutUint16(hmtx[4:6], 20)  // glyph1 lsb

	cmap := make([]byte, 12)
	binary.BigEndian.PutUint16(cmap[0:2], 0)
	binary.BigEndian.PutUint16(cmap[2:4], 1) // numTables
	binary.BigEndian.PutUint16(cmap[4:6], 3)
	binary.BigEndian.PutUint16(cmap[6:8], 1)
	binary.BigEndian.PutUint32(cmap[8:12], 12)

	name := []byte{0, 1, 0, 0}
	post := []byte{0, 0, 0, 0}
	os2 := []byte{0, 0, 0, 0}

	tables := map[string][]byte{
		"OS/2": os2,
		"cmap": cmap,
		"glyf": glyf,
		"head": head,
		"hhea": hhea,
		"hmtx": hmtx,
		"loca": loca,
		"maxp": maxp,
		"name": name,
		"post": post,
	}
	return buildSFNT(tables)
}

func buildSFNT(tables map[string][]byte) []byte {
	tags := make([]string, 0, len(tables))
	for tag := range tables {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	var out bytes.Buffer
	numTables := uint16(len(tags))
	_ = binary.Write(&out, binary.BigEndian, uint32(0x00010000))
	_ = binary.Write(&out, binary.BigEndian, numTables)
	_ = binary.Write(&out, binary.BigEndian, uint16(0))
	_ = binary.Write(&out, binary.BigEndian, uint16(0))
	_ = binary.Write(&out, binary.BigEndian, uint16(0))

	type dir struct {
		tag    string
		offset uint32
		length uint32
		data   []byte
	}
	dirs := make([]dir, 0, len(tags))
	offset := uint32(12 + len(tags)*16)

	for _, tag := range tags {
		data := tables[tag]
		padded := PadTable(data)
		dirs = append(dirs, dir{
			tag:    tag,
			offset: offset,
			length: uint32(len(data)),
			data:   padded,
		})
		offset += uint32(len(padded))
	}

	for i := range dirs {
		item := dirs[i]
		_, _ = out.WriteString(item.tag)
		_ = binary.Write(&out, binary.BigEndian, CalculateChecksum(item.data))
		_ = binary.Write(&out, binary.BigEndian, item.offset)
		_ = binary.Write(&out, binary.BigEndian, item.length)
	}
	for i := range dirs {
		_, _ = out.Write(dirs[i].data)
	}

	return out.Bytes()
}
