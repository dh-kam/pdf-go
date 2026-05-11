// Package entity provides CMap (Character Map) functionality for CJK fonts.
package entity

// CMap maps character codes to Unicode characters and CID/GID values.
type CMap interface {
	// LookupCID maps a character code to a CID (Character ID).
	LookupCID(code uint32) (uint32, bool)

	// LookupUnicode maps a character code to a Unicode string.
	LookupUnicode(code uint32) (string, bool)

	// Name returns the CMap name (e.g., "UniGB-UTF16-H").
	Name() string

	// IsCIDBased returns true if this CMap maps to CIDs.
	IsCIDBased() bool

	// IsUnicode returns true if this CMap maps to Unicode.
	IsUnicode() bool
}

// CMapRange represents a range of character codes.
type CMapRange struct {
	Low  uint32 // Start of range (inclusive)
	High uint32 // End of range (inclusive)
}

// CMapMapping represents a single code to value mapping.
type CMapMapping struct {
	Code  uint32 // Character code
	Value uint32 // CID or Unicode code point
}

// CIDFont represents a CID-keyed font.
type CIDFont interface {
	Font

	// CIDToGID maps a CID to a GID (Glyph ID).
	CIDToGID(cid uint32) (uint32, bool)

	// IsVertical returns true if the font uses vertical writing mode.
	IsVertical() bool

	// DefaultWidth returns the default width for glyphs.
	DefaultWidth() float64

	// DW2 returns the default width for vertical writing (w1, w2).
	DW2() (float64, float64)

	// CIDSet returns the set of available CIDs.
	CIDSet() []uint32
}

// ROS (Registry, Ordering, Supplement) identifies a CID character collection.
type ROS struct {
	Registry   string // e.g., "Adobe", "ISO"
	Ordering   string // e.g., "GB1", "CNS1"
	Supplement int    // Supplement number
}

// CIDSystemInfo describes the CID character collection.
type CIDSystemInfo struct {
	Registry   string
	Ordering   string
	Supplement int
}

// WritingMode represents the writing direction.
type WritingMode int

const (
	// WritingModeHorizontal is left-to-right horizontal writing.
	WritingModeHorizontal WritingMode = iota
	// WritingModeVertical is top-to-bottom vertical writing.
	WritingModeVertical
)

// CMapType represents the CMap encoding type.
type CMapType int

const (
	// CMapTypeType0 maps character codes to CIDs.
	CMapTypeType0 CMapType = iota // Code to CID
	// CMapTypeType1 maps character codes to Unicode.
	CMapTypeType1 // Code to Unicode
	// CMapTypeType2 maps character codes to character names.
	CMapTypeType2 // Code to character name (not commonly used)
)

// CIDtoUnicodeMap maps CIDs to Unicode values.
type CIDtoUnicodeMap interface {
	// ToUnicode maps a CID to a Unicode character.
	ToUnicode(cid uint32) (rune, bool)

	// ToUnicodeString maps a CID to a Unicode string (can be multiple chars).
	ToUnicodeString(cid uint32) (string, bool)
}

// RangeCIDtoUnicodeMap implements CIDtoUnicodeMap using ranges.
type RangeCIDtoUnicodeMap struct {
	ranges []CIDUnicodeRange
}

// CIDUnicodeRange represents a range of CIDs mapping to a range of Unicode values.
type CIDUnicodeRange struct {
	Low        uint32 // Start CID
	High       uint32 // End CID
	UnicodeLow uint32 // Start Unicode value
}

// NewRangeCIDtoUnicodeMap creates a new range-based CID-to-Unicode map.
func NewRangeCIDtoUnicodeMap() *RangeCIDtoUnicodeMap {
	return &RangeCIDtoUnicodeMap{
		ranges: make([]CIDUnicodeRange, 0),
	}
}

// AddRange adds a new mapping range.
func (m *RangeCIDtoUnicodeMap) AddRange(low, high, unicodeLow uint32) {
	m.ranges = append(m.ranges, CIDUnicodeRange{
		Low:        low,
		High:       high,
		UnicodeLow: unicodeLow,
	})
}

// ToUnicode maps a CID to a Unicode character.
func (m *RangeCIDtoUnicodeMap) ToUnicode(cid uint32) (rune, bool) {
	for _, r := range m.ranges {
		if cid >= r.Low && cid <= r.High {
			return rune(r.UnicodeLow + (cid - r.Low)), true
		}
	}
	return 0, false
}

// ToUnicodeString maps a CID to a Unicode string.
func (m *RangeCIDtoUnicodeMap) ToUnicodeString(cid uint32) (string, bool) {
	r, ok := m.ToUnicode(cid)
	if ok {
		return string(r), true
	}
	return "", false
}

// MapCIDtoUnicodeMap implements CIDtoUnicodeMap using a direct map.
type MapCIDtoUnicodeMap struct {
	mapping map[uint32]rune
}

// NewMapCIDtoUnicodeMap creates a new map-based CID-to-Unicode map.
func NewMapCIDtoUnicodeMap() *MapCIDtoUnicodeMap {
	return &MapCIDtoUnicodeMap{
		mapping: make(map[uint32]rune),
	}
}

// Add adds a single CID to Unicode mapping.
func (m *MapCIDtoUnicodeMap) Add(cid uint32, unicode rune) {
	m.mapping[cid] = unicode
}

// ToUnicode maps a CID to a Unicode character.
func (m *MapCIDtoUnicodeMap) ToUnicode(cid uint32) (rune, bool) {
	r, ok := m.mapping[cid]
	return r, ok
}

// ToUnicodeString maps a CID to a Unicode string.
func (m *MapCIDtoUnicodeMap) ToUnicodeString(cid uint32) (string, bool) {
	r, ok := m.ToUnicode(cid)
	if ok {
		return string(r), true
	}
	return "", false
}
