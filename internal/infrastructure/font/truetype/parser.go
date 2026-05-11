// Package truetype provides TrueType/OpenType font parsing and rendering.
package truetype

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/dh-kam/pdf-go/internal/domain/errors"
)

// TableEntry represents a table entry in the font file.
type TableEntry struct {
	Tag    string
	Check  uint32
	Offset uint32
	Length uint32
}

// FontFile represents a TrueType/OpenType font file.
type FontFile struct {
	Head        *HeadTable
	Hhea        *HheaTable
	Maxp        *MaxpTable
	Loca        *LocaTable
	Glyf        *GlyfTable
	Cmap        *CmapTable
	Hmtx        *HmtxTable
	Name        *NameTable
	Post        *PostTable
	OS2         *OS2Table
	Tables      []TableEntry
	SFNTVersion uint32
}

// HeadTable represents the head table (font header).
type HeadTable struct {
	FontRevision       uint32
	ChecksumAdjustment uint32
	MagicNumber        uint32
	Flags              uint16
	UnitsPerEm         uint16
	Created            [2]uint32
	Modified           [2]uint32
	MacStyle           uint16
	MinX               int16
	MinY               int16
	MaxX               int16
	MaxY               int16
	IndexToLocFormat   uint16
	GlyphDataFormat    uint16
}

// HheaTable represents the horizontal header table.
type HheaTable struct {
	TableVersionNumber  uint32
	Ascender            int16
	Descender           int16
	LineGap             int16
	AdvanceWidthMax     uint16
	MinLeftSideBearing  int16
	MinRightSideBearing int16
	XMaxExtent          int16
	CaretSlopeRise      int16
	CaretSlopeRun       int16
	CaretOffset         int16
	Reserved0           int16
	Reserved1           int16
	Reserved2           int16
	Reserved3           int16
	MetricDataFormat    int16
	NumberOfHMetrics    uint16
}

// MaxpTable represents the maximum profile table.
type MaxpTable struct {
	Version               uint32
	NumGlyphs             uint16
	MaxPoints             uint16
	MaxContours           uint16
	MaxCompositePoints    uint16
	MaxCompositeContours  uint16
	MaxZones              uint16
	MaxTwilightPoints     uint16
	MaxStorage            uint16
	MaxFunctionDefs       uint16
	MaxInstructionDefs    uint16
	MaxStackElements      uint16
	MaxSizeOfInstructions uint16
	MaxComponentElements  uint16
	MaxComponentDepth     uint16
}

// LocaTable represents the index to location table.
type LocaTable struct {
	Offsets []uint32
}

// Glyph represents a single glyph.
type Glyph struct {
	Instructions     []byte
	NumberOfContours int16
	XMin             int16
	YMin             int16
	XMax             int16
	YMax             int16
}

// GlyfTable represents the glyph data table.
type GlyfTable struct {
	Glyphs []Glyph
}

// CmapEncoding represents a character encoding in the cmap table.
type CmapEncoding struct {
	Mapping    map[uint16]uint16
	PlatformID uint16
	EncodingID uint16
}

// CmapTable represents the character to glyph mapping table.
type CmapTable struct {
	Encodings []CmapEncoding
}

// HorizontalMetric represents a horizontal metric.
type HorizontalMetric struct {
	AdvanceWidth    uint16
	LeftSideBearing int16
}

// HmtxTable represents the horizontal metrics table.
type HmtxTable struct {
	Metrics          []HorizontalMetric
	LeftSideBearings []int16
}

// NameTable represents the naming table.
type NameTable struct {
	Names map[uint16]string
}

// PostTable represents the PostScript table.
type PostTable struct {
	GlyphNames         []string
	FormatType         uint32
	ItalicAngle        uint32
	IsFixedPitch       uint32
	MinMemType42       uint32
	MaxMemType42       uint32
	UnderlinePosition  int16
	UnderlineThickness int16
}

// OS2Table represents the OS/2 and Windows metrics table.
type OS2Table struct {
	UnicodeRange1       [4]uint32
	UnicodeRange4       [4]uint32
	UnicodeRange3       [4]uint32
	UnicodeRange2       [4]uint32
	WinLineGap          uint32
	Selection           [2]uint16
	XAvgCharWidth       int16
	SubscriptYSize      int16
	YSubscriptYOffset   int16
	YSuperscriptXSize   int16
	YSuperscriptYSize   int16
	TypoDescender       int16
	YSuperscriptYOffset int16
	YStrikeoutSize      int16
	YStrikeoutPosition  int16
	FamilyClass         int16
	StrikeoutPosition   int16
	FsType              uint16
	WidthClass          uint16
	WeightClass         uint16
	FirstCharIndex      uint16
	StrikeoutSize       int16
	YSubscriptXSize     int16
	YSubscriptYSize     int16
	YSubscriptXOffset   int16
	TypoAscender        int16
	YSuperscriptXOffset int16
	LastCharIndex       uint16
	WinAscent           uint16
	WinDescent          uint16
	Version             uint16
	USWeight            uint16
	USWidthClass        uint16
	SubscriptXSize      int16
	TypoLineGap         int16
	SubscriptXOffset    int16
	SubscriptYOffset    int16
	SuperscriptXSize    int16
	SuperscriptYSize    int16
	SuperscriptXOffset  int16
	SuperscriptYOffset  int16
	Panose              [10]uint8
	VendorID            [4]uint8
}

// ParseFontFile parses a TrueType/OpenType font file.
func ParseFontFile(r io.ReadSeeker) (*FontFile, error) {
	font := &FontFile{}

	// Read SFNT version
	var sfntVersion uint32
	if err := binary.Read(r, binary.BigEndian, &sfntVersion); err != nil {
		return nil, errors.Invalid("truetype_header", err)
	}
	font.SFNTVersion = sfntVersion

	// Validate SFNT version
	// 0x00010000 = TrueType, 0x74727565 = "true" (1.0), 0x4F54544F = "OTTO" (OpenType with CFF),
	// 0x74746363 = "ttcf" (TrueType Collection)
	if sfntVersion != 0x00010000 && sfntVersion != 0x74727565 &&
		sfntVersion != 0x4F54544F &&
		sfntVersion != 0x74746363 {
		return nil, errors.Invalid("truetype_version", fmt.Errorf("unsupported SFNT version: 0x%X", sfntVersion))
	}

	// Read number of tables
	var numTables uint16
	if err := binary.Read(r, binary.BigEndian, &numTables); err != nil {
		return nil, errors.Invalid("truetype_header", err)
	}

	// Read search range, entry selector, range shift
	var searchRange uint16
	var entrySelector uint16
	var rangeShift uint16
	if err := binary.Read(r, binary.BigEndian, &searchRange); err != nil {
		return nil, errors.Invalid("truetype_header", err)
	}
	if err := binary.Read(r, binary.BigEndian, &entrySelector); err != nil {
		return nil, errors.Invalid("truetype_header", err)
	}
	if err := binary.Read(r, binary.BigEndian, &rangeShift); err != nil {
		return nil, errors.Invalid("truetype_header", err)
	}
	// Read table directory
	font.Tables = make([]TableEntry, numTables)
	for i := uint16(0); i < numTables; i++ {
		var tag [4]byte
		if _, err := io.ReadFull(r, tag[:]); err != nil {
			return nil, errors.Invalid("truetype_table", err)
		}

		entry := TableEntry{Tag: string(tag[:])}
		if err := binary.Read(r, binary.BigEndian, &entry.Check); err != nil {
			return nil, errors.Invalid("truetype_table", err)
		}
		if err := binary.Read(r, binary.BigEndian, &entry.Offset); err != nil {
			return nil, errors.Invalid("truetype_table", err)
		}
		if err := binary.Read(r, binary.BigEndian, &entry.Length); err != nil {
			return nil, errors.Invalid("truetype_table", err)
		}

		font.Tables[i] = entry
	}

	// Parse required tables
	if err := font.parseTables(r); err != nil {
		return nil, err
	}

	return font, nil
}

// parseTables parses the font tables in dependency order.
// head and maxp must be parsed before loca, and loca before glyf.
func (f *FontFile) parseTables(r io.ReadSeeker) error {
	tableMap := make(map[string]TableEntry, len(f.Tables))
	for _, table := range f.Tables {
		tableMap[table.Tag] = table
	}

	// Parse in dependency order: head, maxp, hhea first, then loca, then glyf, then rest
	parseOrder := []string{"head", "maxp", "hhea", "loca", "hmtx", "cmap", "name", "glyf"}
	parsed := make(map[string]bool)

	for _, tag := range parseOrder {
		table, ok := tableMap[tag]
		if !ok {
			continue
		}
		if err := f.parseOneTable(r, table); err != nil {
			return err
		}
		parsed[tag] = true
	}

	// Parse remaining tables not in the explicit order
	for _, table := range f.Tables {
		if parsed[table.Tag] {
			continue
		}
		if err := f.parseOneTable(r, table); err != nil {
			return err
		}
	}

	return nil
}

func (f *FontFile) parseOneTable(r io.ReadSeeker, table TableEntry) error {
	if _, err := r.Seek(int64(table.Offset), io.SeekStart); err != nil {
		return fmt.Errorf("seek to table %s: %w", table.Tag, err)
	}

	switch table.Tag {
	case "head":
		if err := f.parseHeadTable(r, table); err != nil {
			return err
		}
	case "hhea":
		if err := f.parseHheaTable(r, table); err != nil {
			return err
		}
	case "maxp":
		if err := f.parseMaxpTable(r, table); err != nil {
			return err
		}
	case "loca":
		if err := f.parseLocaTable(r, table); err != nil {
			return err
		}
	case "glyf":
		if err := f.parseGlyfTable(r, table); err != nil {
			return err
		}
	case "cmap":
		if err := f.parseCmapTable(r, table); err != nil {
			return err
		}
	case "hmtx":
		if err := f.parseHmtxTable(r, table); err != nil {
			return err
		}
	case "name":
		if err := f.parseNameTable(r, table); err != nil {
			return err
		}
	case "post":
		if err := f.parsePostTable(r, table); err != nil {
			return err
		}
	case "OS/2":
		if err := f.parseOS2Table(r, table); err != nil {
			return err
		}
	}

	return nil
}

// parseHeadTable parses the head table.
func (f *FontFile) parseHeadTable(r io.Reader, entry TableEntry) error {
	table := &HeadTable{}

	// Skip version
	var version uint32
	if err := binary.Read(r, binary.BigEndian, &version); err != nil {
		return err
	}
	// Read rest of table
	if err := binary.Read(r, binary.BigEndian, &table.FontRevision); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.ChecksumAdjustment); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MagicNumber); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.Flags); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.UnitsPerEm); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.Created[0]); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.Created[1]); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.Modified[0]); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.Modified[1]); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MinX); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MinY); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MaxX); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MaxY); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MacStyle); err != nil {
		return err
	}
	// Skip lowestRecPPEM and fontDirectionHint (2 bytes each)
	var skip uint32
	if err := binary.Read(r, binary.BigEndian, &skip); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.IndexToLocFormat); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.GlyphDataFormat); err != nil {
		return err
	}
	f.Head = table
	return nil
}

// parseHheaTable parses the hhea table.
func (f *FontFile) parseHheaTable(r io.Reader, entry TableEntry) error {
	table := &HheaTable{}

	if err := binary.Read(r, binary.BigEndian, &table.TableVersionNumber); err != nil {

		return err

	}
	if err := binary.Read(r, binary.BigEndian, &table.Ascender); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.Descender); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.LineGap); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.AdvanceWidthMax); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MinLeftSideBearing); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MinRightSideBearing); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.XMaxExtent); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.CaretSlopeRise); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.CaretSlopeRun); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.CaretOffset); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.Reserved0); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.Reserved1); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.Reserved2); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.Reserved3); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MetricDataFormat); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.NumberOfHMetrics); err != nil {
		return err
	}
	f.Hhea = table
	return nil
}

// parseMaxpTable parses the maxp table.
func (f *FontFile) parseMaxpTable(r io.Reader, entry TableEntry) error {
	table := &MaxpTable{}

	if err := binary.Read(r, binary.BigEndian, &table.Version); err != nil {

		return err

	}
	if err := binary.Read(r, binary.BigEndian, &table.NumGlyphs); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MaxPoints); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MaxContours); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MaxCompositePoints); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MaxCompositeContours); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MaxZones); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MaxTwilightPoints); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MaxStorage); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MaxFunctionDefs); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MaxInstructionDefs); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MaxStackElements); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MaxSizeOfInstructions); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MaxComponentElements); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MaxComponentDepth); err != nil {
		return err
	}
	f.Maxp = table
	return nil
}

// parseLocaTable parses the loca table.
func (f *FontFile) parseLocaTable(r io.Reader, entry TableEntry) error {
	table := &LocaTable{
		Offsets: make([]uint32, 0),
	}

	if f.Head == nil {
		return errors.Invalid("truetype_loca", nil)
	}

	// Number of glyphs is in maxp table
	if f.Maxp == nil {
		return errors.Invalid("truetype_loca", nil)
	}
	numGlyphs := f.Maxp.NumGlyphs

	// Read offsets based on IndexToLocFormat
	if f.Head.IndexToLocFormat == 0 {
		// Short offsets (16-bit)
		offsets := make([]uint16, numGlyphs+1)
		for i := uint16(0); i < numGlyphs+1; i++ {
			if err := binary.Read(r, binary.BigEndian, &offsets[i]); err != nil {
				return err
			}
		}

		// Convert to uint32
		table.Offsets = make([]uint32, len(offsets))
		for i, offset := range offsets {
			table.Offsets[i] = uint32(offset) * 2
		}
	} else {
		// Long offsets (32-bit)
		table.Offsets = make([]uint32, numGlyphs+1)
		for i := uint16(0); i < numGlyphs+1; i++ {
			if err := binary.Read(r, binary.BigEndian, &table.Offsets[i]); err != nil {
				return err
			}
		}
	}

	f.Loca = table
	return nil
}

// parseGlyfTable parses the glyf table.
func (f *FontFile) parseGlyfTable(r io.ReadSeeker, entry TableEntry) error {
	table := &GlyfTable{
		Glyphs: make([]Glyph, 0),
	}

	if f.Maxp == nil || f.Loca == nil {
		return errors.Invalid("truetype_glyf", nil)
	}

	numGlyphs := f.Maxp.NumGlyphs

	// Read each glyph
	for i := uint16(0); i < numGlyphs; i++ {
		offset := f.Loca.Offsets[i]
		nextOffset := f.Loca.Offsets[i+1]

		// Empty glyph
		if nextOffset == offset {
			table.Glyphs = append(table.Glyphs, Glyph{})
			continue
		}

		// Seek to glyph position
		if _, err := r.Seek(int64(entry.Offset+offset), io.SeekStart); err != nil {
			return err
		}

		glyph := Glyph{}
		if err := binary.Read(r, binary.BigEndian, &glyph.NumberOfContours); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &glyph.XMin); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &glyph.YMin); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &glyph.XMax); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &glyph.YMax); err != nil {
			return err
		}
		// Read instructions if present
		glyphDataSize := nextOffset - offset - 10 // 10 = header size
		if glyphDataSize > 0 {
			glyph.Instructions = make([]byte, glyphDataSize)
			if _, err := io.ReadFull(r, glyph.Instructions); err != nil {
				return err
			}
		}

		table.Glyphs = append(table.Glyphs, glyph)
	}

	f.Glyf = table
	return nil
}

// parseCmapTable parses the cmap table.
func (f *FontFile) parseCmapTable(r io.ReadSeeker, entry TableEntry) error {
	table := &CmapTable{
		Encodings: make([]CmapEncoding, 0),
	}

	// Read cmap header
	var version uint16
	var numTables uint16
	if err := binary.Read(r, binary.BigEndian, &version); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &numTables); err != nil {
		return err
	}
	// Read encoding records
	for i := uint16(0); i < numTables; i++ {
		var platformID uint16
		var encodingID uint16
		var subtableOffset uint32

		if err := binary.Read(r, binary.BigEndian, &platformID); err != nil {

			return err

		}
		if err := binary.Read(r, binary.BigEndian, &encodingID); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &subtableOffset); err != nil {
			return err
		}
		// Parse Unicode cmaps plus Mac Roman subset cmaps used by some PDF
		// embedded 8-bit TrueType fonts. Poppler builds its Splash codeToGID
		// table from FoFiTrueType for these fonts before rendering.
		isWindowsUnicode := platformID == 3 && (encodingID == 1 || encodingID == 0)
		isUnicodePlatform := platformID == 0 && (encodingID == 3 || encodingID == 4 || encodingID == 6)
		isMacRoman := platformID == 1 && encodingID == 0
		if isWindowsUnicode || isUnicodePlatform || isMacRoman {
			// Remember current position
			currentPos, err := r.Seek(0, io.SeekCurrent)
			if err != nil {
				return err
			}

			// Seek to subtable
			if _, err := r.Seek(int64(entry.Offset+subtableOffset), io.SeekStart); err != nil {
				return err
			}

			subtableStart := int64(entry.Offset + subtableOffset)

			// Read subtable
			var format uint16
			var length uint16
			var language uint16
			if err := binary.Read(r, binary.BigEndian, &format); err != nil {
				return err
			}
			if err := binary.Read(r, binary.BigEndian, &length); err != nil {
				return err
			}
			if err := binary.Read(r, binary.BigEndian, &language); err != nil {
				return err
			}
			switch format {
			case 0:
				mapping := make(map[uint16]uint16, 256)
				for code := 0; code < 256; code++ {
					var glyphID uint8
					if err := binary.Read(r, binary.BigEndian, &glyphID); err != nil {
						return err
					}
					mapping[uint16(code)] = uint16(glyphID)
				}
				table.Encodings = append(table.Encodings, CmapEncoding{
					PlatformID: platformID,
					EncodingID: encodingID,
					Mapping:    mapping,
				})

			case 4:
				// Format 4: Segment mapping to delta values
				var segCountX2 uint16
				var searchRange uint16
				var entrySelector uint16
				var rangeShift uint16
				if err := binary.Read(r, binary.BigEndian, &segCountX2); err != nil {
					return err
				}
				if err := binary.Read(r, binary.BigEndian, &searchRange); err != nil {
					return err
				}
				if err := binary.Read(r, binary.BigEndian, &entrySelector); err != nil {
					return err
				}
				if err := binary.Read(r, binary.BigEndian, &rangeShift); err != nil {
					return err
				}
				segCount := segCountX2 / 2

				// Read end codes
				endCodes := make([]uint16, segCount)
				for i := uint16(0); i < segCount; i++ {
					if err := binary.Read(r, binary.BigEndian, &endCodes[i]); err != nil {
						return err
					}
				}

				// Skip reserved pad
				var pad uint16
				if err := binary.Read(r, binary.BigEndian, &pad); err != nil {
					return err
				}
				// Read start codes
				startCodes := make([]uint16, segCount)
				for i := uint16(0); i < segCount; i++ {
					if err := binary.Read(r, binary.BigEndian, &startCodes[i]); err != nil {
						return err
					}
				}

				// Read idDeltas
				idDeltas := make([]int16, segCount)
				for i := uint16(0); i < segCount; i++ {
					if err := binary.Read(r, binary.BigEndian, &idDeltas[i]); err != nil {
						return err
					}
				}

				// Read idRangeOffsets
				idRangeOffsets := make([]uint16, segCount)
				idRangeOffsetStart, err := r.Seek(0, io.SeekCurrent)
				if err != nil {
					return err
				}
				for i := uint16(0); i < segCount; i++ {
					if err := binary.Read(r, binary.BigEndian, &idRangeOffsets[i]); err != nil {
						return err
					}
				}

				// Build glyph mapping
				mapping := make(map[uint16]uint16)
				for i := uint16(0); i < segCount; i++ {
					start := startCodes[i]
					end := endCodes[i]
					delta := idDeltas[i]
					rangeOffset := idRangeOffsets[i]
					rangeOffsetAddr := idRangeOffsetStart + int64(i)*2

					// Use uint32 to avoid uint16 wrap-around when end==0xFFFF.
					for c := uint32(start); c <= uint32(end); c++ {
						if rangeOffset == 0 {
							mapping[uint16(c)] = uint16(int16(c) + delta)
							continue
						}

						glyphAddr := rangeOffsetAddr + int64(rangeOffset) + int64(c-uint32(start))*2
						if length > 0 && (glyphAddr < subtableStart || glyphAddr+2 > subtableStart+int64(length)) {
							continue
						}
						if _, err := r.Seek(glyphAddr, io.SeekStart); err != nil {
							return err
						}
						var glyphID uint16
						if err := binary.Read(r, binary.BigEndian, &glyphID); err != nil {
							return err
						}
						if glyphID != 0 {
							glyphID = uint16(int32(glyphID) + int32(delta))
						}
						mapping[uint16(c)] = glyphID
					}
				}

				table.Encodings = append(table.Encodings, CmapEncoding{
					PlatformID: platformID,
					EncodingID: encodingID,
					Mapping:    mapping,
				})

			case 6:
				var firstCode uint16
				var entryCount uint16
				if err := binary.Read(r, binary.BigEndian, &firstCode); err != nil {
					return err
				}
				if err := binary.Read(r, binary.BigEndian, &entryCount); err != nil {
					return err
				}
				mapping := make(map[uint16]uint16, entryCount)
				for j := uint16(0); j < entryCount; j++ {
					var glyphID uint16
					if err := binary.Read(r, binary.BigEndian, &glyphID); err != nil {
						return err
					}
					mapping[firstCode+j] = glyphID
				}
				table.Encodings = append(table.Encodings, CmapEncoding{
					PlatformID: platformID,
					EncodingID: encodingID,
					Mapping:    mapping,
				})
			}

			// Restore position
			if _, err := r.Seek(currentPos, io.SeekStart); err != nil {
				return err
			}
		}
	}

	f.Cmap = table
	return nil
}

// parseHmtxTable parses the hmtx table.
func (f *FontFile) parseHmtxTable(r io.Reader, entry TableEntry) error {
	table := &HmtxTable{}

	if f.Maxp == nil || f.Hhea == nil {
		return errors.Invalid("truetype_hmtx", nil)
	}

	numGlyphs := f.Maxp.NumGlyphs
	numHMetrics := f.Hhea.NumberOfHMetrics

	table.Metrics = make([]HorizontalMetric, numHMetrics)
	table.LeftSideBearings = make([]int16, 0)

	// Read long hor metrics
	for i := uint16(0); i < numHMetrics; i++ {
		metric := HorizontalMetric{}
		if err := binary.Read(r, binary.BigEndian, &metric.AdvanceWidth); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &metric.LeftSideBearing); err != nil {
			return err
		}
		table.Metrics[i] = metric
	}

	// Read left side bearings for remaining glyphs
	if int(numGlyphs) > int(numHMetrics) {
		for i := numHMetrics; i < numGlyphs; i++ {
			var lsb int16
			if err := binary.Read(r, binary.BigEndian, &lsb); err != nil {
				return err
			}
			table.LeftSideBearings = append(table.LeftSideBearings, lsb)
		}
	}

	f.Hmtx = table
	return nil
}

// parseNameTable parses the name table.
func (f *FontFile) parseNameTable(r io.ReadSeeker, entry TableEntry) error {
	table := &NameTable{
		Names: make(map[uint16]string),
	}

	// Read header
	var format uint16
	var count uint16
	var stringOffset uint16
	if err := binary.Read(r, binary.BigEndian, &format); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &stringOffset); err != nil {
		return err
	}
	if format != 0 {
		// Only format 0 is supported
		return nil
	}

	// Read name records
	for i := uint16(0); i < count; i++ {
		var platformID uint16
		var encodingID uint16
		var languageID uint16
		var nameID uint16
		var length uint16
		var offset uint16

		if err := binary.Read(r, binary.BigEndian, &platformID); err != nil {

			return err

		}
		if err := binary.Read(r, binary.BigEndian, &encodingID); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &languageID); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &nameID); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &length); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &offset); err != nil {
			return err
		}
		// Only read names for platform 1 (Mac) or 3 (Windows)
		if platformID == 1 || platformID == 3 {
			// Remember current position
			currentPos, err := r.Seek(0, io.SeekCurrent)
			if err != nil {
				return err
			}

			// Seek to string
			if _, err := r.Seek(int64(entry.Offset)+int64(stringOffset)+int64(offset), io.SeekStart); err != nil {
				return err
			}

			// Read string
			nameBytes := make([]byte, length)
			if _, err := io.ReadFull(r, nameBytes); err != nil {
				return err
			}
			table.Names[nameID] = string(nameBytes)

			// Restore position
			if _, err := r.Seek(currentPos, io.SeekStart); err != nil {
				return err
			}
		}
	}

	f.Name = table
	return nil
}

// parsePostTable parses the post table.
func (f *FontFile) parsePostTable(r io.ReadSeeker, entry TableEntry) error {
	table := &PostTable{
		GlyphNames: make([]string, 0),
	}

	// Read format
	var format uint32
	if err := binary.Read(r, binary.BigEndian, &format); err != nil {
		return err
	}
	table.FormatType = format

	// Read common fields
	if err := binary.Read(r, binary.BigEndian, &table.ItalicAngle); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.UnderlinePosition); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.UnderlineThickness); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.IsFixedPitch); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MinMemType42); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.MaxMemType42); err != nil {
		return err
	}
	if format == 0x00020000 {
		// Format 2: contains glyph names
		var numGlyphs uint16
		if err := binary.Read(r, binary.BigEndian, &numGlyphs); err != nil {
			return err
		}
		// Read glyph name indices
		indices := make([]uint16, numGlyphs)
		for i := uint16(0); i < numGlyphs; i++ {
			if err := binary.Read(r, binary.BigEndian, &indices[i]); err != nil {
				return err
			}
		}

		// For now, we don't parse the actual glyph names
		// A full implementation would read the string data
	}

	f.Post = table
	return nil
}

// parseOS2Table parses the OS/2 table.
func (f *FontFile) parseOS2Table(r io.Reader, entry TableEntry) error {
	table := &OS2Table{}

	// Read version
	if err := binary.Read(r, binary.BigEndian, &table.Version); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.XAvgCharWidth); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.WeightClass); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.WidthClass); err != nil {
		return err
	}
	if err := binary.Read(r, binary.BigEndian, &table.FsType); err != nil {
		return err
	}
	// Read more fields for version 1 and later
	if table.Version >= 1 {
		if err := binary.Read(r, binary.BigEndian, &table.YSubscriptXSize); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.YSubscriptYSize); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.YSubscriptXOffset); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.YSubscriptYOffset); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.YSuperscriptXSize); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.YSuperscriptYSize); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.YSuperscriptXOffset); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.YSuperscriptYOffset); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.YStrikeoutSize); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.YStrikeoutPosition); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.FamilyClass); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.Panose[0]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.Panose[1]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.Panose[2]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.Panose[3]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.Panose[4]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.Panose[5]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.Panose[6]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.Panose[7]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.Panose[8]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.Panose[9]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.UnicodeRange1[0]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.UnicodeRange1[1]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.UnicodeRange1[2]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.UnicodeRange1[3]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.UnicodeRange2[0]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.UnicodeRange2[1]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.UnicodeRange2[2]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.UnicodeRange2[3]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.UnicodeRange3[0]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.UnicodeRange3[1]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.UnicodeRange3[2]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.UnicodeRange3[3]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.UnicodeRange4[0]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.UnicodeRange4[1]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.UnicodeRange4[2]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.UnicodeRange4[3]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.VendorID[0]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.VendorID[1]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.VendorID[2]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.VendorID[3]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.Selection[0]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.Selection[1]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.FirstCharIndex); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.LastCharIndex); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.TypoAscender); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.TypoDescender); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.TypoLineGap); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.WinAscent); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.WinDescent); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.WinLineGap); err != nil {
			return err
		}
	}

	if table.Version >= 2 {
		if err := binary.Read(r, binary.BigEndian, &table.USWeight); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.USWidthClass); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.SubscriptXSize); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.SubscriptYSize); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.SubscriptXOffset); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.SubscriptYOffset); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.SuperscriptXSize); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.SuperscriptYSize); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.SuperscriptXOffset); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.SuperscriptYOffset); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.StrikeoutSize); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.StrikeoutPosition); err != nil {
			return err
		}
	}

	if table.Version >= 5 {
		if err := binary.Read(r, binary.BigEndian, &table.VendorID[0]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.VendorID[1]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.VendorID[2]); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &table.VendorID[3]); err != nil {
			return err
		}
	}

	f.OS2 = table
	return nil
}

// GetGlyphData returns the glyph data for a given glyph ID.
func (f *FontFile) GetGlyphData(glyphID uint16) (*Glyph, error) {
	if f.Glyf == nil || uint16(len(f.Glyf.Glyphs)) <= glyphID {
		return nil, errors.NotFoundf("glyph", "glyph ID %d not found", glyphID)
	}
	return &f.Glyf.Glyphs[glyphID], nil
}

// CharCodeToGlyph maps a character code to a glyph ID.
func (f *FontFile) CharCodeToGlyph(charCode uint16) (uint16, bool) {
	if f.Cmap == nil {
		return 0, false
	}

	// Try each encoding
	for _, encoding := range f.Cmap.Encodings {
		if glyphID, ok := encoding.Mapping[charCode]; ok {
			return glyphID, true
		}
	}

	return 0, false
}

// GetGlyphWidth returns the advance width for a glyph.
func (f *FontFile) GetGlyphWidth(glyphID uint16) (uint16, error) {
	if f.Hmtx == nil {
		return 0, errors.Invalid("truetype_metrics", nil)
	}

	if uint16(len(f.Hmtx.Metrics)) > glyphID {
		return f.Hmtx.Metrics[glyphID].AdvanceWidth, nil
	}

	// Use last metric for remaining glyphs
	if len(f.Hmtx.Metrics) == 0 {
		return 0, errors.Invalid("truetype_metrics", nil)
	}
	return f.Hmtx.Metrics[len(f.Hmtx.Metrics)-1].AdvanceWidth, nil
}

// UnitsPerEm returns the units per em value.
func (f *FontFile) UnitsPerEm() uint16 {
	if f.Head == nil {
		return 1000 // Default value
	}
	return f.Head.UnitsPerEm
}

// GetGlyphBoundingBox returns the bounding box for a glyph.
func (f *FontFile) GetGlyphBoundingBox(glyphID uint16) (xMin, yMin, xMax, yMax int16, err error) {
	glyph, err := f.GetGlyphData(glyphID)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return glyph.XMin, glyph.YMin, glyph.XMax, glyph.YMax, nil
}

// GetFontName returns the font name.
func (f *FontFile) GetFontName() string {
	if f.Name == nil {
		return ""
	}
	// Name ID 1 is the Font Family name
	if name, ok := f.Name.Names[1]; ok {
		return name
	}
	return ""
}
