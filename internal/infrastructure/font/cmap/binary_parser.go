// Package cmap provides CMap parsing functionality for CJK fonts.
package cmap

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
)

// BinaryParser parses binary CMap files.
type BinaryParser struct {
	data []byte
	pos  int
}

// NewBinaryParser creates a new binary CMap parser.
func NewBinaryParser(data []byte) *BinaryParser {
	return &BinaryParser{
		data: data,
		pos:  0,
	}
}

// Parse parses a binary CMap and returns a CMap.
func (p *BinaryParser) Parse() (entity.CMap, error) {
	if len(p.data) < 8 {
		return nil, errors.Invalid("cmap_binary", fmt.Errorf("invalid binary CMap: too short"))
	}

	// Read header
	magic := p.readUint32()
	if magic != 0x54434D66 && magic != 0x54434D20 && magic != 0x54434D32 {
		// "TMCf", "TMC ", or "TMC2" magic numbers
		return nil, errors.Invalid("cmap_binary", fmt.Errorf("invalid binary CMap magic: 0x%08x", magic))
	}

	version := p.readUint16()
	numSubtables := int(p.readUint16())

	cmap := &BaseCMap{
		name:       "BinaryCMap",
		cidMapping: make(map[uint32]uint32),
		uniMapping: make(map[uint32]string),
	}

	// Parse subtables
	for i := 0; i < numSubtables; i++ {
		if err := p.parseSubtable(cmap, version); err != nil {
			return nil, err
		}
	}

	return cmap, nil
}

// parseSubtable parses a single binary CMap subtable.
func (p *BinaryParser) parseSubtable(cmap *BaseCMap, version uint16) error {
	if p.pos+8 > len(p.data) {
		return fmt.Errorf("unexpected end of data in subtable header")
	}

	platformID := p.readUint16()
	encodingID := p.readUint16()
	subtableOffset := int(p.readUint32())

	// Save current position
	currentPos := p.pos

	// Seek to subtable
	if subtableOffset < 0 || subtableOffset > len(p.data) {
		return fmt.Errorf("invalid subtable offset: %d", subtableOffset)
	}

	p.pos = subtableOffset

	// Read subtable format
	if p.pos+2 > len(p.data) {
		return fmt.Errorf("unexpected end of data in subtable format")
	}

	format := p.readUint16()

	// Parse based on format
	switch format {
	case 0: // Byte encoding table
		if err := p.parseFormat0(cmap, platformID, encodingID); err != nil {
			return err
		}
	case 4: // Segment mapping to delta values
		if err := p.parseFormat4(cmap, platformID, encodingID); err != nil {
			return err
		}
	case 6: // Trimmed table mapping
		if err := p.parseFormat6(cmap, platformID, encodingID); err != nil {
			return err
		}
	case 12: // Segmented coverage
		if err := p.parseFormat12(cmap, platformID, encodingID); err != nil {
			return err
		}
	default:
		// Unknown format, skip
	}

	// Restore position
	p.pos = currentPos

	return nil
}

// parseFormat0 parses format 0 subtable (byte encoding).
func (p *BinaryParser) parseFormat0(cmap *BaseCMap, platformID, encodingID uint16) error {
	if p.pos+6 > len(p.data) {
		return fmt.Errorf("unexpected end of data in format 0")
	}

	length := p.readUint16()
	language := p.readUint16()

	// Skip language field for now
	_ = language

	// Read glyph index array (256 bytes)
	if p.pos+256 > len(p.data) || int(length) < 256+6 {
		return fmt.Errorf("invalid format 0 length")
	}

	for charCode := 0; charCode < 256; charCode++ {
		glyphIndex := p.data[p.pos]
		p.pos++

		if glyphIndex > 0 {
			cid := uint32(glyphIndex)
			cmap.SetCIDMapping(uint32(charCode), cid)
		}
	}

	return nil
}

// parseFormat4 parses format 4 subtable (segment mapping to delta values).
func (p *BinaryParser) parseFormat4(cmap *BaseCMap, platformID, encodingID uint16) error {
	if p.pos+14 > len(p.data) {
		return fmt.Errorf("unexpected end of data in format 4 header")
	}

	length := p.readUint16()
	language := p.readUint16()
	segCountX2 := p.readUint16()
	searchRange := p.readUint16()
	entrySelector := p.readUint16()
	rangeShift := p.readUint16()

	_ = length        // Skip length
	_ = language      // Skip language
	_ = searchRange   // Skip search range
	_ = entrySelector // Skip entry selector
	_ = rangeShift    // Skip range shift

	segCount := int(segCountX2 / 2)

	// Read end codes (segCount uint16 values)
	endCodes := make([]uint16, segCount)
	for i := 0; i < segCount; i++ {
		if p.pos+2 > len(p.data) {
			return fmt.Errorf("unexpected end of data in end codes")
		}
		endCodes[i] = p.readUint16()
	}

	// Skip reserved pad
	if p.pos+2 > len(p.data) {
		return fmt.Errorf("unexpected end of data in reserved pad")
	}
	p.pos += 2

	// Read start codes (segCount uint16 values)
	startCodes := make([]uint16, segCount)
	for i := 0; i < segCount; i++ {
		if p.pos+2 > len(p.data) {
			return fmt.Errorf("unexpected end of data in start codes")
		}
		startCodes[i] = p.readUint16()
	}

	// Read idDeltas (segCount int16 values)
	idDeltas := make([]int16, segCount)
	for i := 0; i < segCount; i++ {
		if p.pos+2 > len(p.data) {
			return fmt.Errorf("unexpected end of data in id deltas")
		}
		idDeltas[i] = int16(p.readUint16())
	}

	// Read idRangeOffsets (segCount uint16 values)
	idRangeOffsets := make([]uint16, segCount)
	for i := 0; i < segCount; i++ {
		if p.pos+2 > len(p.data) {
			return fmt.Errorf("unexpected end of data in id range offsets")
		}
		idRangeOffsets[i] = p.readUint16()
	}

	// Parse character mapping
	for i := 0; i < segCount; i++ {
		startCode := uint32(startCodes[i])
		endCode := uint32(endCodes[i])

		if startCode == 0xFFFF && endCode == 0xFFFF {
			// End of segment
			break
		}

		idDelta := int32(idDeltas[i])
		idRangeOffset := idRangeOffsets[i]

		// Special handling for format 4
		if idRangeOffset == 0 {
			// Use idDelta
			for charCode := startCode; charCode <= endCode; charCode++ {
				glyphID := uint32(int32(charCode)+idDelta) & 0xFFFF
				if glyphID > 0 {
					cmap.SetCIDMapping(charCode, glyphID)
				}
			}
		} else {
			// Use glyph index array
			// The offset is from the current position in the idRangeOffset array
			// This is complex, for now use simplified approach
			for charCode := startCode; charCode <= endCode; charCode++ {
				// Calculate offset in glyph index array
				// For simplicity, just use the character code as CID
				cmap.SetCIDMapping(charCode, charCode)
			}
		}
	}

	return nil
}

// parseFormat6 parses format 6 subtable (trimmed table mapping).
func (p *BinaryParser) parseFormat6(cmap *BaseCMap, platformID, encodingID uint16) error {
	if p.pos+10 > len(p.data) {
		return fmt.Errorf("unexpected end of data in format 6 header")
	}

	length := p.readUint16()
	language := p.readUint16()
	firstCode := p.readUint16()
	entryCount := p.readUint16()

	_ = length   // Skip length
	_ = language // Skip language

	// Read glyph index array (entryCount uint16 values)
	if p.pos+int(entryCount)*2 > len(p.data) {
		return fmt.Errorf("unexpected end of data in format 6 entries")
	}

	for i := uint16(0); i < entryCount; i++ {
		glyphIndex := p.readUint16()
		charCode := uint32(firstCode) + uint32(i)

		if glyphIndex > 0 {
			cmap.SetCIDMapping(charCode, uint32(glyphIndex))
		}
	}

	return nil
}

// parseFormat12 parses format 12 subtable (segmented coverage).
func (p *BinaryParser) parseFormat12(cmap *BaseCMap, platformID, encodingID uint16) error {
	if p.pos+12 > len(p.data) {
		return fmt.Errorf("unexpected end of data in format 12 header")
	}

	// Skip reserved (uint32)
	p.pos += 4

	length := p.readUint32()
	language := p.readUint32()
	numGroups := p.readUint32()

	_ = length   // Skip length
	_ = language // Skip language

	// Read groups (numGroups * 12 bytes)
	for i := uint32(0); i < numGroups; i++ {
		if p.pos+12 > len(p.data) {
			return fmt.Errorf("unexpected end of data in format 12 groups")
		}

		startCharCode := p.readUint32()
		endCharCode := p.readUint32()
		startGlyphID := p.readUint32()

		// Create mapping for this group
		delta := int32(startGlyphID) - int32(startCharCode)
		for charCode := startCharCode; charCode <= endCharCode; charCode++ {
			glyphID := uint32(int32(charCode) + delta)
			cmap.SetCIDMapping(charCode, glyphID)
		}
	}

	return nil
}

// readUint16 reads a uint16 from the data (big-endian).
func (p *BinaryParser) readUint16() uint16 {
	if p.pos+2 > len(p.data) {
		return 0
	}
	v := binary.BigEndian.Uint16(p.data[p.pos : p.pos+2])
	p.pos += 2
	return v
}

// readUint32 reads a uint32 from the data (big-endian).
func (p *BinaryParser) readUint32() uint32 {
	if p.pos+4 > len(p.data) {
		return 0
	}
	v := binary.BigEndian.Uint32(p.data[p.pos : p.pos+4])
	p.pos += 4
	return v
}

// ParseBinaryBytes parses binary CMap data and returns a CMap.
func ParseBinaryBytes(data []byte) (entity.CMap, error) {
	parser := NewBinaryParser(data)
	return parser.Parse()
}

// DetectFormat detects whether CMap data is in text or binary format.
func DetectFormat(data []byte) string {
	if len(data) < 4 {
		return "text"
	}

	// Check for binary magic numbers
	magic := binary.BigEndian.Uint32(data[0:4])
	if magic == 0x54434D66 || magic == 0x54434D20 || magic == 0x54434D32 {
		return "binary"
	}

	// Check for text format markers
	if bytes.HasPrefix(data, []byte("/CIDInit")) ||
		bytes.HasPrefix(data, []byte("/CMap")) ||
		bytes.HasPrefix(data, []byte("%!PS-Adobe")) {
		return "text"
	}

	// Default to text
	return "text"
}

// ParseAuto detects the format and parses accordingly.
func ParseAuto(data []byte) (entity.CMap, error) {
	format := DetectFormat(data)

	switch format {
	case "binary":
		return ParseBinaryBytes(data)
	default:
		return ParseBytes(data)
	}
}
