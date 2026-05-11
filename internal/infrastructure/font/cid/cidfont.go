// Package cid provides CID-keyed font support for CJK fonts.
//
//revive:disable:exported
package cid

import (
	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// CIDFont implements a CID-keyed font.
type CIDFont struct {
	baseFont     entity.Font
	cmap         entity.CMap
	cidToGID     map[uint32]uint32
	name         string
	ros          entity.ROS
	cidSet       []uint32
	dw2          [2]float64
	defaultWidth float64
	isVertical   bool
}

// NewCIDFont creates a new CID-keyed font.
func NewCIDFont(name string, baseFont entity.Font, cmap entity.CMap) *CIDFont {
	return &CIDFont{
		name:         name,
		cidToGID:     make(map[uint32]uint32),
		isVertical:   false,
		defaultWidth: 1000.0,
		dw2:          [2]float64{500.0, -500.0},
		cidSet:       make([]uint32, 0),
		baseFont:     baseFont,
		cmap:         cmap,
	}
}

// Name returns the font name.
func (f *CIDFont) Name() string {
	return f.name
}

// IsCIDFont returns true for CID-keyed fonts.
func (f *CIDFont) IsCIDFont() bool {
	return true
}

// IsSymbolic returns false for CID fonts.
func (f *CIDFont) IsSymbolic() bool {
	return false
}

// UnitsPerEm returns the units per em.
func (f *CIDFont) UnitsPerEm() uint16 {
	if f.baseFont != nil {
		return f.baseFont.UnitsPerEm()
	}
	return 1000
}

// CharCodeToGlyph maps a character code to a GID via CID.
func (f *CIDFont) CharCodeToGlyph(code uint32) (uint32, error) {
	// First map character code to CID using CMap
	cid, ok := f.cmap.LookupCID(code)
	if !ok {
		// If no CID mapping, try direct character code
		cid = code
	}

	// Then map CID to GID
	gid, ok := f.CIDToGID(cid)
	if !ok {
		// If no GID mapping, use CID as GID (common for TrueType CID fonts)
		gid = cid
	}

	return gid, nil
}

// GlyphName returns the glyph name for a glyph ID.
func (f *CIDFont) GlyphName(glyph uint32) string {
	if f.baseFont != nil {
		return f.baseFont.GlyphName(glyph)
	}
	return ""
}

// GetGlyphWidth returns the width of a glyph.
func (f *CIDFont) GetGlyphWidth(glyph uint32) (float64, error) {
	if f.baseFont != nil {
		return f.baseFont.GetGlyphWidth(glyph)
	}
	return f.defaultWidth, nil
}

// GetBoundingBox returns the font bounding box.
func (f *CIDFont) GetBoundingBox() (float64, float64, float64, float64) {
	if f.baseFont != nil {
		return f.baseFont.GetBoundingBox()
	}
	return 0, 0, 1000, 1000
}

// RenderGlyph renders a glyph at the given size.
func (f *CIDFont) RenderGlyph(glyph uint32, size float64) (*entity.GlyphPath, error) {
	if f.baseFont != nil {
		return f.baseFont.RenderGlyph(glyph, size)
	}
	return &entity.GlyphPath{}, nil
}

// CIDToGID maps a CID to a GID.
func (f *CIDFont) CIDToGID(cid uint32) (uint32, bool) {
	gid, ok := f.cidToGID[cid]
	return gid, ok
}

// SetCIDToGID sets a CID to GID mapping.
func (f *CIDFont) SetCIDToGID(cid, gid uint32) {
	f.cidToGID[cid] = gid
}

// IsVertical returns true if the font uses vertical writing mode.
func (f *CIDFont) IsVertical() bool {
	return f.isVertical
}

// SetVertical sets the vertical writing mode.
func (f *CIDFont) SetVertical(v bool) {
	f.isVertical = v
}

// DefaultWidth returns the default width for glyphs.
func (f *CIDFont) DefaultWidth() float64 {
	return f.defaultWidth
}

// SetDefaultWidth sets the default width.
func (f *CIDFont) SetDefaultWidth(w float64) {
	f.defaultWidth = w
}

// DW2 returns the default width for vertical writing.
func (f *CIDFont) DW2() (float64, float64) {
	return f.dw2[0], f.dw2[1]
}

// SetDW2 sets the default width for vertical writing.
func (f *CIDFont) SetDW2(w1, w2 float64) {
	f.dw2[0] = w1
	f.dw2[1] = w2
}

// CIDSet returns the set of available CIDs.
func (f *CIDFont) CIDSet() []uint32 {
	return f.cidSet
}

// SetCIDSet sets the available CIDs.
func (f *CIDFont) SetCIDSet(cids []uint32) {
	f.cidSet = cids
}

// ROS returns the Registry-Ordering-Supplement information.
func (f *CIDFont) ROS() entity.ROS {
	return f.ros
}

// SetROS sets the Registry-Ordering-Supplement information.
func (f *CIDFont) SetROS(registry, ordering string, supplement int) {
	f.ros = entity.ROS{
		Registry:   registry,
		Ordering:   ordering,
		Supplement: supplement,
	}
}

// CMap returns the font's CMap.
func (f *CIDFont) CMap() entity.CMap {
	return f.cmap
}

// BaseFont returns the underlying descendant/base font.
func (f *CIDFont) BaseFont() entity.Font {
	return f.baseFont
}

// SetCMap sets the font's CMap.
func (f *CIDFont) SetCMap(cmap entity.CMap) {
	f.cmap = cmap
}

// ToUnicodeCMap returns the ToUnicode CMap for character extraction.
func (f *CIDFont) ToUnicodeCMap() entity.CIDtoUnicodeMap {
	// Create a simple identity mapping
	m := entity.NewMapCIDtoUnicodeMap()
	for cid := range f.cidSet {
		// In production, would use actual ToUnicode data
		m.Add(uint32(cid), rune(cid))
	}
	return m
}
