// Package cff provides CFF (Compact Font Format) font implementation.
//
//revive:disable:exported
//nolint:errcheck,gocritic,staticcheck,unconvert,unused
package cff

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
)

// Font represents a CFF font.
type Font struct {
	header           *Header
	nameIndex        *NameIndex
	topDict          *TopDict
	stringIndex      *StringIndex
	globalSubrs      *SubroutineIndex
	charStringsIndex *Index
	privateDict      *PrivateDict
	localSubrs       *SubroutineIndex
	data             []byte
}

// Header represents the CFF header.
type Header struct {
	Major      uint8
	Minor      uint8
	HeaderSize uint8
	OffSize    uint8
}

// TopDict contains top-level dictionary data.
type TopDict struct {
	ROS               []byte
	FontBBox          []float64
	CharStringsOffset int32
	PrivateOffset     int32
	PrivateSize       int32
}

// NameIndex represents the CFF NAME index.
type NameIndex struct {
	Names []string
}

// StringIndex represents the CFF STRING index.
type StringIndex struct {
	Strings []string
}

// SubroutineIndex represents CFF local/local subroutines.
type SubroutineIndex struct {
	Subrs []int32
	Bias  int32
}

// PrivateDict contains the CFF Private dictionary.
type PrivateDict struct {
	LocalSubrs        *SubroutineIndex
	StemSnapV         []int32
	BlueValues        []int32
	OtherBlues        []int32
	FamilyBlues       []int32
	FamilyOtherBlues  []int32
	StemSnapH         []int32
	BlueScale         float64
	DefaultWidthX     float64
	ExpansionFactor   float64
	NominalWidthX     float64
	BlueFuzz          int32
	BlueShift         int32
	StdHW             int32
	StdVW             int32
	LanguageGroup     int32
	initialRandomSeed int32
	SubrsOffset       int32
	ForceBold         bool
}

// NewFont creates a new CFF font from data.
func NewFont(data []byte) (*Font, error) {
	font := &Font{
		data: data,
	}

	err := font.parse()
	if err != nil {
		return nil, err
	}

	return font, nil
}

// parse parses the CFF font data.
func (f *Font) parse() error {
	r := bytes.NewReader(f.data)

	// Parse header
	if err := f.parseHeader(r); err != nil {
		return err
	}

	// Parse name index
	if err := f.parseNameIndex(r); err != nil {
		return err
	}

	// Parse top dict index
	topDictIndex, err := f.parseIndex(r)
	if err != nil {
		return err
	}

	// Parse top dict from index data
	f.topDict = &TopDict{}
	if topDictIndex.count > 0 {
		topDictData := topDictIndex.data
		if err := f.parseTopDict(topDictData); err != nil {
			return err
		}
	}

	// Parse string index
	if err := f.parseStringIndex(r); err != nil {
		return err
	}

	// Parse global subrs
	if err := f.parseGlobalSubrs(r); err != nil {
		return err
	}

	// Parse CharStrings index
	if f.topDict.CharStringsOffset > 0 {
		if err := f.parseCharStrings(r); err != nil {
			return err
		}
	}

	// Parse Private dict
	if f.topDict.PrivateOffset > 0 && f.topDict.PrivateSize > 0 {
		if err := f.parsePrivateDict(); err != nil {
			return err
		}
	}

	return nil
}

// parseHeader parses the CFF header.
func (f *Font) parseHeader(r *bytes.Reader) error {
	f.header = &Header{}

	// Read major version
	major, err := r.ReadByte()
	if err != nil {
		return err
	}
	f.header.Major = major

	// Read minor version
	minor, err := r.ReadByte()
	if err != nil {
		return err
	}
	f.header.Minor = minor

	// Read header size
	hdrSize, err := r.ReadByte()
	if err != nil {
		return err
	}
	f.header.HeaderSize = hdrSize

	// Read off size
	offSize, err := r.ReadByte()
	if err != nil {
		return err
	}
	f.header.OffSize = offSize

	if offSize < 1 || offSize > 4 {
		return errors.Invalid("cff_header", fmt.Errorf("invalid offsize: %d", offSize))
	}

	return nil
}

// parseNameIndex parses the CFF NAME index.
func (f *Font) parseNameIndex(r *bytes.Reader) error {
	index, err := f.parseIndex(r)
	if err != nil {
		return err
	}

	f.nameIndex = &NameIndex{}

	// Parse names (SID or string)
	count := index.count
	data := index.data
	offset := 0

	if count == 0 {
		f.nameIndex.Names = []string{"CFF Font"}
		return nil
	}

	f.nameIndex.Names = make([]string, count)

	for i := 0; i < int(count); i++ {
		if offset >= len(data) {
			break
		}

		// Check if it's an offset (SID)
		if index.offSize > 0 {
			// Read offset
			var sid int32
			switch index.offSize {
			case 1:
				b, err := r.ReadByte()
				if err != nil {
					return err
				}
				sid = int32(b)
				offset++
			case 2:
				var val uint16
				if err := binary.Read(r, binary.BigEndian, &val); err != nil {
					return err
				}
				sid = int32(val)
				offset += 2
			case 3:
				var val uint32
				if err := binary.Read(r, binary.BigEndian, &val); err != nil {
					return err
				}
				sid = int32(val >> 8)
				offset += 3
			case 4:
				var val uint32
				if err := binary.Read(r, binary.BigEndian, &val); err != nil {
					return err
				}
				sid = int32(val)
				offset += 4
			}

			// For now, just use the SID as name
			f.nameIndex.Names[i] = fmt.Sprintf("sid%d", sid)
		} else {
			// Direct string
			strEnd := bytes.IndexByte(data[offset:], 0)
			if strEnd == -1 {
				strEnd = len(data) - offset
			}
			f.nameIndex.Names[i] = string(data[offset : offset+strEnd])
			offset += strEnd + 1
		}
	}

	return nil
}

// parseTopDict parses the top dictionary data.
func (f *Font) parseTopDict(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	r := bytes.NewReader(data)

	for r.Len() > 0 {
		op, err := r.ReadByte()
		if err != nil {
			break
		}

		switch op {
		case 0: // END
			return nil

		case 1: // version
			_, err := f.parseDelta(r)
			if err != nil {
				return err
			}

		case 2: // Notice
			_, err := f.parseDelta(r)
			if err != nil {
				return err
			}

		case 3: // FullName
			_, err := f.parseDelta(r)
			if err != nil {
				return err
			}

		case 4: // FamilyName
			_, err := f.parseDelta(r)
			if err != nil {
				return err
			}

		case 5: // Weight
			_, err := f.parseDelta(r)
			if err != nil {
				return err
			}

		case 6: // FontBBox
			bbox, err := f.parseFixedArray(r, 4)
			if err != nil {
				return err
			}
			f.topDict.FontBBox = bbox

		case 7: // BlueValues
			blues, err := f.parseDeltaArray(r)
			if err != nil {
				return err
			}
			// Store in top dict if needed
			_ = blues

		case 8: // OtherBlues
			blues, err := f.parseDeltaArray(r)
			if err != nil {
				return err
			}
			_ = blues

		case 9: // StemSnapH
			snaps, err := f.parseDeltaArray(r)
			if err != nil {
				return err
			}
			_ = snaps

		case 10: // StemSnapV
			snaps, err := f.parseDeltaArray(r)
			if err != nil {
				return err
			}
			_ = snaps

		case 11: // StdHW
			std, err := f.parseDeltaArray(r)
			if err != nil {
				return err
			}
			_ = std

		case 12: // StdVW
			std, err := f.parseDeltaArray(r)
			if err != nil {
				return err
			}
			_ = std

		case 17: // CharStrings
			deltadata, err := f.parseDelta(r)
			if err != nil {
				return err
			}
			if len(deltadata) > 0 {
				f.topDict.CharStringsOffset = deltadata[0]
			}

		case 18: // Private
			deltadata, err := f.parseDeltaArray(r)
			if err != nil {
				return err
			}
			if len(deltadata) >= 2 {
				f.topDict.PrivateSize = deltadata[0]
				if len(deltadata) >= 2 {
					f.topDict.PrivateOffset = deltadata[1]
				}
			}

		case 19: // ROS
			// CID font ROS
			if r.Len() >= 3 {
				f.topDict.ROS = make([]byte, 3)
				if _, err := r.Read(f.topDict.ROS); err != nil {
					return err
				}
			}

		case 20: // FontVersion
			_, err := f.parseDeltaArray(r)
			if err != nil {
				return err
			}

		case 21: // FontRevision
			_, err := f.parseDeltaArray(r)
			if err != nil {
				return err
			}

		case 22: // Copyright
			_, err := f.parseDelta(r)
			if err != nil {
				return err
			}

		case 23: // Encoding
			_, err := f.parseDelta(r)
			if err != nil {
				return err
			}

		case 24: // Charset
			_, err := f.parseDelta(r)
			if err != nil {
				return err
			}

		default:
			// Skip unknown operator
		}
	}

	return nil
}

// parseDelta parses a CFF delta value.
func (f *Font) parseDelta(r io.ByteReader) ([]int32, error) {
	b1, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	var result []int32
	var value int32

	if b1 >= 32 && b1 <= 246 {
		// Single byte value
		value = int32(b1) - 139
		result = []int32{value}
	} else if b1 >= 247 && b1 <= 250 {
		// Negative two-byte value
		b2, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		value = int32(b1-247)*256 + int32(b2) + 108
		result = []int32{value}
	} else if b1 >= 251 && b1 <= 254 {
		// Positive 2-5 byte array
		n := int(b1) - 251 + 2
		arr, err := f.parseDeltaArrayN(r, n)
		if err != nil {
			return nil, err
		}
		return arr, nil
	} else if b1 == 255 {
		// Reserved (not used in top dict)
		return nil, nil
	} else {
		// b1 == 0: END (handled in parseTopDict)
		return nil, nil
	}

	return result, nil
}

// parseDeltaArray parses a CFF delta array.
func (f *Font) parseDeltaArray(r io.ByteReader) ([]int32, error) {
	deltadata, err := f.parseDelta(r)
	if err != nil {
		return nil, err
	}
	return deltadata, nil
}

// parseDeltaArrayN parses a CFF delta array with known length.
func (f *Font) parseDeltaArrayN(r io.ByteReader, n int) ([]int32, error) {
	result := make([]int32, 0, n)

	for i := 0; i < n; i++ {
		b1, err := r.ReadByte()
		if err != nil {
			return nil, err
		}

		var value int32

		if b1 >= 32 && b1 <= 246 {
			value = int32(b1) - 139
		} else if b1 >= 247 && b1 <= 250 {
			b2, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			value = int32(b1-247)*256 + int32(b2) + 108
		} else {
			return nil, errors.Invalid("cff_delta", fmt.Errorf("invalid delta byte: %d", b1))
		}

		result = append(result, value)
	}

	return result, nil
}

// parseFixedArray parses a CFF Fixed array.
func (f *Font) parseFixedArray(r io.Reader, count int) ([]float64, error) {
	if count <= 0 {
		return nil, nil
	}

	result := make([]float64, count)

	for i := 0; i < count; i++ {
		// Read 4 bytes for Fixed number (16.16 fixed point)
		data := make([]byte, 4)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}

		// Convert Fixed to float64
		value := float64(binary.BigEndian.Uint32(data)) / 65536.0
		result[i] = value
	}

	return result, nil
}

// Index represents a CFF INDEX structure.
type Index struct {
	data    []byte
	count   int32
	offSize uint8
}

// parseIndex parses a CFF INDEX structure.
func (f *Font) parseIndex(r *bytes.Reader) (*Index, error) {
	index := &Index{}

	// Read count (offSize bytes)
	countBytes := make([]byte, f.header.OffSize)
	if _, err := r.Read(countBytes); err != nil {
		return nil, err
	}

	if f.header.OffSize == 1 {
		index.count = int32(countBytes[0])
	} else if f.header.OffSize == 2 {
		index.count = int32(binary.BigEndian.Uint16(countBytes))
	} else if f.header.OffSize == 3 {
		index.count = int32(binary.BigEndian.Uint32([]byte{0, 0, 0, countBytes[0]})) >> 8
	} else if f.header.OffSize == 4 {
		index.count = int32(binary.BigEndian.Uint32(countBytes))
	}

	// Read offsize
	offSizeByte, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	index.offSize = uint8(offSizeByte)

	// Check for empty index
	if index.count == 0 {
		return index, nil
	}

	// Read offsets
	offCount := index.count + 1
	offsets := make([]int32, offCount)

	for i := 0; i < int(offCount); i++ {
		var offset int32
		switch index.offSize {
		case 1:
			b, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			offset = int32(b)
		case 2:
			var val uint16
			if err := binary.Read(r, binary.BigEndian, &val); err != nil {
				return nil, err
			}
			offset = int32(val)
		case 3:
			var val uint32
			if err := binary.Read(r, binary.BigEndian, &val); err != nil {
				return nil, err
			}
			offset = int32(val >> 8)
		case 4:
			var val uint32
			if err := binary.Read(r, binary.BigEndian, &val); err != nil {
				return nil, err
			}
			offset = int32(val)
		}
		offsets[i] = offset
	}

	// Calculate data size
	lastOffset := offsets[len(offsets)-1]
	index.data = make([]byte, lastOffset)

	// Read data
	if _, err := r.Read(index.data); err != nil {
		return nil, err
	}

	return index, nil
}

// parseStringIndex parses the CFF STRING index.
func (f *Font) parseStringIndex(r *bytes.Reader) error {
	index, err := f.parseIndex(r)
	if err != nil {
		return err
	}

	f.stringIndex = &StringIndex{}

	if index.count == 0 {
		f.stringIndex.Strings = []string{}
		return nil
	}

	f.stringIndex.Strings = make([]string, index.count)

	for i := int32(0); i < index.count; i++ {
		// Get next offset
		var nextOffset int32
		if i < index.count-1 {
			// Read the offset for next string
			var offset int32
			switch index.offSize {
			case 1:
				b, err := r.ReadByte()
				if err != nil {
					return err
				}
				offset = int32(b)
			case 2:
				var val uint16
				if err := binary.Read(r, binary.BigEndian, &val); err != nil {
					return err
				}
				offset = int32(val)
			case 3:
				var val uint32
				if err := binary.Read(r, binary.BigEndian, &val); err != nil {
					return err
				}
				offset = int32(val >> 8)
			case 4:
				var val uint32
				if err := binary.Read(r, binary.BigEndian, &val); err != nil {
					return err
				}
				offset = int32(val)
			}
			nextOffset = offset
		} else {
			nextOffset = int32(len(index.data))
		}

		// String is stored at offset
		if i == 0 {
			// First string starts at 0
			if nextOffset > 0 {
				f.stringIndex.Strings[i] = string(index.data[0:nextOffset])
			}
		} else {
			// Get previous offset
			var prevOffset int32
			switch index.offSize {
			case 1:
				b, err := r.ReadByte()
				if err != nil {
					return err
				}
				prevOffset = int32(b)
			case 2:
				var val uint16
				if err := binary.Read(r, binary.BigEndian, &val); err != nil {
					return err
				}
				prevOffset = int32(val)
			case 3:
				var val uint32
				if err := binary.Read(r, binary.BigEndian, &val); err != nil {
					return err
				}
				prevOffset = int32(val >> 8)
			case 4:
				var val uint32
				if err := binary.Read(r, binary.BigEndian, &val); err != nil {
					return err
				}
				prevOffset = int32(val)
			}

			// Extract string between previous and next offset
			if prevOffset < int32(len(index.data)) && nextOffset <= int32(len(index.data)) {
				f.stringIndex.Strings[i] = string(index.data[prevOffset:nextOffset])
			}
		}
	}

	return nil
}

// parseGlobalSubrs parses the global subroutine index.
func (f *Font) parseGlobalSubrs(r *bytes.Reader) error {
	index, err := f.parseIndex(r)
	if err != nil {
		return err
	}

	if index.count == 0 {
		return nil
	}

	// Parse subroutine offsets
	subrs := make([]int32, index.count)

	for i := int32(0); i < index.count; i++ {
		var offset int32
		switch index.offSize {
		case 1:
			b, err := r.ReadByte()
			if err != nil {
				return err
			}
			offset = int32(b)
		case 2:
			var val uint16
			if err := binary.Read(r, binary.BigEndian, &val); err != nil {
				return err
			}
			offset = int32(val)
		case 3:
			var val uint32
			if err := binary.Read(r, binary.BigEndian, &val); err != nil {
				return err
			}
			offset = int32(val >> 8)
		case 4:
			var val uint32
			if err := binary.Read(r, binary.BigEndian, &val); err != nil {
				return err
			}
			offset = int32(val)
		}
		subrs[i] = offset
	}

	// Calculate bias
	bias := f.calculateBias(subrs)

	f.globalSubrs = &SubroutineIndex{
		Bias:  bias,
		Subrs: subrs,
	}

	return nil
}

// calculateBias calculates the subroutine bias.
func (f *Font) calculateBias(subrs []int32) int32 {
	if len(subrs) == 0 {
		return 0
	}

	// Bias is based on the number of subroutines
	// For 1240 or more, bias = 107
	// For 124 or less, bias = 113
	if subrs[0] >= 1240 {
		return 107
	}
	return 113
}

// parseCharStrings parses the CharStrings index.
func (f *Font) parseCharStrings(r *bytes.Reader) error {
	// Seek to CharStrings offset
	offset := f.topDict.CharStringsOffset
	if offset < 0 || int(offset) >= len(f.data) {
		return errors.Invalid("cff_charstrings", fmt.Errorf("invalid CharStrings offset: %d", offset))
	}

	if _, err := r.Seek(int64(offset), io.SeekStart); err != nil {
		return err
	}

	index, err := f.parseIndex(r)
	if err != nil {
		return err
	}

	f.charStringsIndex = index
	return nil
}

// parsePrivateDict parses the Private dictionary.
func (f *Font) parsePrivateDict() error {
	f.privateDict = &PrivateDict{
		DefaultWidthX:   0,
		NominalWidthX:   0,
		BlueScale:       0.039625,
		BlueShift:       7,
		BlueFuzz:        1,
		ExpansionFactor: 0.06,
	}

	offset := f.topDict.PrivateOffset
	size := f.topDict.PrivateSize

	if offset <= 0 || size <= 0 || int(offset+size) > len(f.data) {
		// No private dict, use defaults
		return nil
	}

	data := f.data[offset : offset+size]
	r := bytes.NewReader(data)

	// Parse private dict operators
	for r.Len() > 0 {
		val, err := f.parseOperand(r)
		if err != nil {
			break
		}

		if val == nil {
			break
		}

		// Check for operators
		if r.Len() > 0 {
			op, _ := r.ReadByte()
			switch op {
			case 6: // BlueValues
				if arr, ok := val.([]int32); ok {
					f.privateDict.BlueValues = arr
				}
			case 7: // OtherBlues
				if arr, ok := val.([]int32); ok {
					f.privateDict.OtherBlues = arr
				}
			case 8: // FamilyBlues
				if arr, ok := val.([]int32); ok {
					f.privateDict.FamilyBlues = arr
				}
			case 9: // FamilyOtherBlues
				if arr, ok := val.([]int32); ok {
					f.privateDict.FamilyOtherBlues = arr
				}
			case 10: // BlueScale
				if fval, ok := val.(float64); ok {
					f.privateDict.BlueScale = fval
				}
			case 11: // BlueShift
				if ival, ok := val.(int32); ok {
					f.privateDict.BlueShift = ival
				}
			case 12: // BlueFuzz
				if ival, ok := val.(int32); ok {
					f.privateDict.BlueFuzz = ival
				}
			case 19: // Subrs
				if arr, ok := val.([]int32); ok && len(arr) > 0 {
					f.privateDict.SubrsOffset = arr[0]
					// Parse local subrs
					if err := f.parseLocalSubrs(); err != nil {
						return err
					}
				}
			case 20: // DefaultWidthX
				if fval, ok := val.(float64); ok {
					f.privateDict.DefaultWidthX = fval
				}
			case 21: // NominalWidthX
				if fval, ok := val.(float64); ok {
					f.privateDict.NominalWidthX = fval
				}
			}
		}
	}

	return nil
}

// parseLocalSubrs parses the local subroutines.
func (f *Font) parseLocalSubrs() error {
	if f.privateDict.SubrsOffset <= 0 {
		return nil
	}

	// Calculate absolute offset
	absOffset := f.topDict.PrivateOffset + f.privateDict.SubrsOffset
	if absOffset < 0 || int(absOffset) >= len(f.data) {
		return nil
	}

	r := bytes.NewReader(f.data[absOffset:])
	index, err := f.parseIndex(r)
	if err != nil {
		return err
	}

	if index.count == 0 {
		return nil
	}

	// Parse subroutine offsets
	subrs := make([]int32, index.count)
	for i := int32(0); i < index.count; i++ {
		var offset int32
		switch index.offSize {
		case 1:
			b, err := r.ReadByte()
			if err != nil {
				return err
			}
			offset = int32(b)
		case 2:
			var val uint16
			if err := binary.Read(r, binary.BigEndian, &val); err != nil {
				return err
			}
			offset = int32(val)
		case 3:
			var val uint32
			if err := binary.Read(r, binary.BigEndian, &val); err != nil {
				return err
			}
			offset = int32(val >> 8)
		case 4:
			var val uint32
			if err := binary.Read(r, binary.BigEndian, &val); err != nil {
				return err
			}
			offset = int32(val)
		}
		subrs[i] = offset
	}

	bias := f.calculateBias(subrs)
	f.privateDict.LocalSubrs = &SubroutineIndex{
		Bias:  bias,
		Subrs: subrs,
	}

	return nil
}

// parseOperand parses a CFF operand (integer or array).
func (f *Font) parseOperand(r *bytes.Reader) (interface{}, error) {
	if r.Len() == 0 {
		return nil, nil
	}

	b1, err := r.ReadByte()
	if err != nil {
		return nil, err
	}

	// Check for operator
	if b1 == 12 || b1 <= 31 {
		// Operator byte
		r.Seek(-1, io.SeekCurrent)
		return nil, nil
	}

	// Integer value
	if b1 >= 32 && b1 <= 246 {
		val := int32(b1) - 139
		return val, nil
	} else if b1 >= 247 && b1 <= 250 {
		b2, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		val := int32(b1-247)*256 + int32(b2) + 108
		return val, nil
	} else if b1 >= 251 && b1 <= 254 {
		b2, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		val := int32(b1-251)*256 + int32(b2) - 108
		return val, nil
	} else if b1 == 255 {
		// Fixed 16.16
		data := make([]byte, 4)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, err
		}
		val := float64(binary.BigEndian.Uint32(data)) / 65536.0
		return val, nil
	}

	// Array of integers (for delta arrays)
	return nil, errors.Invalid("cff_operand", fmt.Errorf("invalid operand byte: %d", b1))
}

// IsCIDFont returns true if this is a CID-keyed CFF font.
func (f *Font) IsCIDFont() bool {
	return f.topDict != nil && f.topDict.ROS != nil
}

// Name returns the font name.
func (f *Font) Name() string {
	if f.nameIndex != nil && len(f.nameIndex.Names) > 0 {
		return f.nameIndex.Names[0]
	}
	return "CFF Font"
}

// FontData returns the raw CFF font bytes.
func (f *Font) FontData() []byte {
	return append([]byte(nil), f.data...)
}

// GlyphName returns the name for a glyph index.
func (f *Font) GlyphName(glyph uint32) string {
	return fmt.Sprintf("gid%d", glyph)
}

// GetGlyphWidth returns the width of a glyph.
func (f *Font) GetGlyphWidth(glyph uint32) (float64, error) {
	// Placeholder - would need to parse CharStrings to get widths
	return 500.0, nil
}

// RenderGlyph renders a glyph to a path using the pure-Go charstring parser.
func (f *Font) renderGlyphCharString(glyph uint32, size float64) (*entity.GlyphPath, error) {
	if f.charStringsIndex == nil || int(glyph) >= len(f.charStringsIndex.data) {
		return &entity.GlyphPath{
			Commands: []entity.PathCommand{},
			Bounds:   [4]float64{0, 0, 500, 0},
		}, nil
	}

	// Get CharString data for the glyph
	charStringData, err := f.getCharStringData(glyph)
	if err != nil {
		return nil, err
	}

	// Parse CharString commands
	commands, err := f.parseCharString(charStringData)
	if err != nil {
		return nil, err
	}

	// Scale by font size
	scale := size / float64(f.UnitsPerEm())

	// Convert to entity path
	entityPath := &entity.GlyphPath{
		Commands: make([]entity.PathCommand, 0, len(commands)),
		Bounds:   [4]float64{0, 0, 500, 0},
	}

	// Track bounds for scaling
	minX, minY := 1e100, 1e100
	maxX, maxY := -1e100, -1e100

	for _, cmd := range commands {
		switch cmd.opcode {
		case opMoveTo:
			sx := cmd.args[0] * scale
			sy := cmd.args[1] * scale
			entityPath.Commands = append(entityPath.Commands, &entity.PathMoveTo{X: sx, Y: sy})
			updateBounds(&minX, &minY, &maxX, &maxY, sx, sy)

		case opLineTo:
			sx := cmd.args[0] * scale
			sy := cmd.args[1] * scale
			entityPath.Commands = append(entityPath.Commands, &entity.PathLineTo{X: sx, Y: sy})
			updateBounds(&minX, &minY, &maxX, &maxY, sx, sy)

		case opCurveTo:
			c1x := cmd.args[0] * scale
			c1y := cmd.args[1] * scale
			c2x := cmd.args[2] * scale
			c2y := cmd.args[3] * scale
			ex := cmd.args[4] * scale
			ey := cmd.args[5] * scale
			entityPath.Commands = append(entityPath.Commands, &entity.PathCurveTo{
				X1: c1x, Y1: c1y,
				X2: c2x, Y2: c2y,
				X3: ex, Y3: ey,
			})
			updateBounds(&minX, &minY, &maxX, &maxY, c1x, c1y)
			updateBounds(&minX, &minY, &maxX, &maxY, c2x, c2y)
			updateBounds(&minX, &minY, &maxX, &maxY, ex, ey)

		case opClosePath:
			entityPath.Commands = append(entityPath.Commands, &entity.PathClose{})
		}
	}

	if minX < 1e100 {
		entityPath.Bounds = [4]float64{minX, minY, maxX, maxY}
	}

	return entityPath, nil
}

func updateBounds(minX, minY, maxX, maxY *float64, x, y float64) {
	if x < *minX {
		*minX = x
	}
	if x > *maxX {
		*maxX = x
	}
	if y < *minY {
		*minY = y
	}
	if y > *maxY {
		*maxY = y
	}
}

// getCharStringData returns the CharString data for a glyph.
func (f *Font) getCharStringData(glyph uint32) ([]byte, error) {
	if f.charStringsIndex == nil {
		return nil, errors.NotFoundf("charstring", "glyph %d", glyph)
	}

	if int32(glyph) >= f.charStringsIndex.count {
		return nil, errors.NotFoundf("charstring", "glyph %d", glyph)
	}

	// Get offsets from index
	offsets := make([]int32, f.charStringsIndex.count+1)
	r := bytes.NewReader(f.charStringsIndex.data)

	for i := 0; i <= int(f.charStringsIndex.count); i++ {
		var offset int32
		switch f.charStringsIndex.offSize {
		case 1:
			b, err := r.ReadByte()
			if err != nil {
				return nil, err
			}
			offset = int32(b)
		case 2:
			var val uint16
			if err := binary.Read(r, binary.BigEndian, &val); err != nil {
				return nil, err
			}
			offset = int32(val)
		case 3:
			var val uint32
			if err := binary.Read(r, binary.BigEndian, &val); err != nil {
				return nil, err
			}
			offset = int32(val >> 8)
		case 4:
			var val uint32
			if err := binary.Read(r, binary.BigEndian, &val); err != nil {
				return nil, err
			}
			offset = int32(val)
		}
		offsets[i] = offset
	}

	// Extract CharString data (offsets are 1-based)
	start := offsets[glyph] - 1
	end := offsets[glyph+1] - 1

	if start < 0 || end > int32(len(f.charStringsIndex.data)) || start >= end {
		return nil, errors.Invalid("charstring", fmt.Errorf("invalid offset for glyph %d", glyph))
	}

	return f.charStringsIndex.data[start:end], nil
}

// CharString opcodes
const (
	opMoveTo = iota + 1
	opLineTo
	opCurveTo
	opClosePath
)

// charStringCommand represents a parsed CharString command.
type charStringCommand struct {
	args   []float64
	opcode int
}

// parseCharString parses a CharString into commands.
func (f *Font) parseCharString(data []byte) ([]charStringCommand, error) {
	if len(data) == 0 {
		return nil, nil
	}

	r := bytes.NewReader(data)
	var commands []charStringCommand
	var stack []float64

	for r.Len() > 0 {
		b0, err := r.ReadByte()
		if err != nil {
			break
		}

		// Parse operand
		if b0 >= 32 && b0 <= 246 {
			val := float64(int32(b0) - 139)
			stack = append(stack, val)
		} else if b0 >= 247 && b0 <= 250 {
			b1, err := r.ReadByte()
			if err != nil {
				break
			}
			val := float64((int32(b0)-247)*256 + int32(b1) + 108)
			stack = append(stack, val)
		} else if b0 >= 251 && b0 <= 254 {
			b1, err := r.ReadByte()
			if err != nil {
				break
			}
			val := float64((int32(b0)-251)*256 + int32(b1) - 108)
			stack = append(stack, val)
		} else if b0 == 255 {
			// 16.16 fixed point
			data := make([]byte, 4)
			if _, err := io.ReadFull(r, data); err != nil {
				break
			}
			val := float64(binary.BigEndian.Uint32(data)) / 65536.0
			stack = append(stack, val)
		} else {
			// Operator
			switch b0 {
			case 1: // hstem
				stack = stack[:0]
			case 3: // vstem
				stack = stack[:0]
			case 4: // vmoveto
				if len(stack) >= 1 {
					commands = append(commands, charStringCommand{opcode: opMoveTo, args: []float64{0, stack[0]}})
					stack = stack[:0]
				}
			case 5: // rlineto
				for i := 0; i < len(stack); i += 2 {
					if i+1 < len(stack) {
						commands = append(commands, charStringCommand{opcode: opLineTo, args: []float64{stack[i], stack[i+1]}})
					}
				}
				stack = stack[:0]
			case 6: // hlineto
				_ = 0.0 // placeholder x
				for i := 0; i < len(stack); i++ {
					if i%2 == 0 {
						commands = append(commands, charStringCommand{opcode: opLineTo, args: []float64{stack[i], 0}})
					} else {
						commands = append(commands, charStringCommand{opcode: opLineTo, args: []float64{stack[i], stack[i]}})
					}
				}
				stack = stack[:0]
			case 7: // vlineto
				y := 0.0
				for i := 0; i < len(stack); i++ {
					if i%2 == 0 {
						commands = append(commands, charStringCommand{opcode: opLineTo, args: []float64{0, y}})
					} else {
						y += stack[i]
						commands = append(commands, charStringCommand{opcode: opLineTo, args: []float64{stack[i], y}})
					}
				}
				stack = stack[:0]
			case 8: // rrcurveto
				for i := 0; i < len(stack); i += 6 {
					if i+5 < len(stack) {
						commands = append(commands, charStringCommand{opcode: opCurveTo, args: []float64{
							stack[i], stack[i+1], stack[i+2], stack[i+3], stack[i+4], stack[i+5],
						}})
					}
				}
				stack = stack[:0]
			case 10: // callsubr
				if len(stack) >= 1 && f.privateDict != nil && f.privateDict.LocalSubrs != nil {
					_ = int32(stack[len(stack)-1])
					stack = stack[:len(stack)-1]
					// Apply bias - subroutine parsing deferred for simplicity
					_ = f.privateDict.LocalSubrs.Bias
					// Would need to recurse to parse subroutine
					// For now, skip
				}
			case 14: // endchar
				// seac accent handling is intentionally deferred.
				stack = stack[:0]
				commands = append(commands, charStringCommand{opcode: opClosePath})
				return commands, nil
			case 21: // rmoveto
				if len(stack) >= 2 {
					commands = append(commands, charStringCommand{opcode: opMoveTo, args: []float64{stack[0], stack[1]}})
					stack = stack[:0]
				}
			case 22: // hmoveto
				if len(stack) >= 1 {
					commands = append(commands, charStringCommand{opcode: opMoveTo, args: []float64{stack[0], 0}})
					stack = stack[:0]
				}
			case 24: // rcurveline
				i := 0
				for i+5 < len(stack) {
					commands = append(commands, charStringCommand{opcode: opCurveTo, args: []float64{
						stack[i], stack[i+1], stack[i+2], stack[i+3], stack[i+4], stack[i+5],
					}})
					i += 6
				}
				if i+1 < len(stack) {
					commands = append(commands, charStringCommand{opcode: opLineTo, args: []float64{stack[i], stack[i+1]}})
				}
				stack = stack[:0]
			case 25: // rlinecurve
				i := 0
				// Find last curve (6 args)
				numCurves := (len(stack) - 2) / 6
				for c := 0; c < numCurves; c++ {
					commands = append(commands, charStringCommand{opcode: opCurveTo, args: []float64{
						stack[i], stack[i+1], stack[i+2], stack[i+3], stack[i+4], stack[i+5],
					}})
					i += 6
				}
				if i+1 < len(stack) {
					commands = append(commands, charStringCommand{opcode: opLineTo, args: []float64{stack[i], stack[i+1]}})
				}
				stack = stack[:0]
			case 26: // vvcurveto
				i := 0
				if len(stack)%2 == 1 {
					// First argument is dx
					i = 1
				}
				for i+3 < len(stack) {
					dx := 0.0
					if i > 0 {
						dx = stack[0]
					}
					commands = append(commands, charStringCommand{opcode: opCurveTo, args: []float64{
						dx, stack[i], stack[i+1], stack[i+2], stack[i+3], 0,
					}})
					i += 4
				}
				stack = stack[:0]
			case 27: // hhcurveto
				i := 0
				if len(stack)%2 == 1 {
					// First argument is dy
					i = 1
				}
				for i+3 < len(stack) {
					dy := 0.0
					if i > 0 {
						dy = stack[0]
					}
					commands = append(commands, charStringCommand{opcode: opCurveTo, args: []float64{
						stack[i], dy, stack[i+1], stack[i+2], 0, stack[i+3],
					}})
					i += 4
				}
				stack = stack[:0]
			case 30: // vhcurveto
				i := 0
				for i+3 < len(stack) {
					if len(stack)-i >= 8 {
						commands = append(commands, charStringCommand{opcode: opCurveTo, args: []float64{
							stack[i], stack[i+1], stack[i+2], stack[i+3], stack[i+4], stack[i+5],
						}})
						i += 6
					} else {
						commands = append(commands, charStringCommand{opcode: opCurveTo, args: []float64{
							stack[i], stack[i+1], stack[i+2], stack[i+3], stack[i+4], 0,
						}})
						i += 4
					}
				}
				stack = stack[:0]
			case 31: // hvcurveto
				i := 0
				for i+3 < len(stack) {
					if len(stack)-i >= 8 {
						commands = append(commands, charStringCommand{opcode: opCurveTo, args: []float64{
							stack[i], stack[i+1], stack[i+2], stack[i+3], stack[i+4], stack[i+5],
						}})
						i += 6
					} else {
						commands = append(commands, charStringCommand{opcode: opCurveTo, args: []float64{
							stack[i], stack[i+1], stack[i+2], stack[i+3], 0, stack[i+4],
						}})
						i += 4
					}
				}
				stack = stack[:0]
			case 12: // Two-byte operator
				if r.Len() > 0 {
					b1, _ := r.ReadByte()
					switch b1 {
					case 3: // and
					case 4: // or
					case 5: // not
					case 6: // abs
					case 8: // add
					case 9: // sub
					case 10: // div
					case 11: // neg
					case 12: // eq
					case 13: // dropout
					case 14: // put
					case 15: // get
					case 16: // ifelse
					case 17: // random
					case 18: // mul
					case 20: // sqrt
					case 21: // dup
					case 22: // exch
					case 23: // index
					case 24: // roll
					case 25: // hflex
						if len(stack) >= 7 {
							commands = append(commands, charStringCommand{opcode: opCurveTo, args: []float64{
								stack[0], 0, stack[1], stack[2], stack[3], 0,
							}})
							commands = append(commands, charStringCommand{opcode: opCurveTo, args: []float64{
								stack[4], 0, stack[5], -stack[6], 0, 0,
							}})
							stack = stack[:0]
						}
					case 26: // hflex1
						if len(stack) >= 9 {
							commands = append(commands, charStringCommand{opcode: opCurveTo, args: []float64{
								stack[0], stack[1], stack[2], stack[3], stack[4], 0,
							}})
							commands = append(commands, charStringCommand{opcode: opCurveTo, args: []float64{
								stack[5], 0, stack[6], stack[7], stack[8], 0,
							}})
							stack = stack[:0]
						}
					case 27: // flex1
						if len(stack) >= 11 {
							// Determine final axis based on stack size
							dx := 0.0
							dy := 0.0
							commands = append(commands, charStringCommand{opcode: opCurveTo, args: []float64{
								stack[0], stack[1], stack[2], stack[3], stack[4], stack[5],
							}})
							commands = append(commands, charStringCommand{opcode: opCurveTo, args: []float64{
								stack[6], stack[7], stack[8], stack[9], dx, dy,
							}})
							stack = stack[:0]
						}
					case 33: // hstemhm
						stack = stack[:0]
					case 34: // hintmask
						stack = stack[:0]
					case 35: // cntrmask
						stack = stack[:0]
					case 36: // vstemhm
						stack = stack[:0]
					}
				}
			}
		}
	}

	return commands, nil
}

// UnitsPerEm returns the units per em value.
func (f *Font) UnitsPerEm() uint16 {
	// CFF fonts typically use 1000 units per em
	return 1000
}

// IsSymbolic returns false; CFF body text fonts are not symbolic.
func (f *Font) IsSymbolic() bool {
	return false
}

// GetBoundingBox returns a default font bounding box for CFF fonts.
func (f *Font) GetBoundingBox() (float64, float64, float64, float64) {
	if f.topDict != nil && len(f.topDict.FontBBox) == 4 {
		return f.topDict.FontBBox[0], f.topDict.FontBBox[1],
			f.topDict.FontBBox[2], f.topDict.FontBBox[3]
	}
	return -100, -200, 900, 800
}

// GetFontMatrix returns the font matrix.
func (f *Font) GetFontMatrix() [6]float64 {
	return [6]float64{0.001, 0, 0, 0.001, 0, 0}
}

