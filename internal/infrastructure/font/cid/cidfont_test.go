package cid

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testMockCMap struct {
	cid map[uint32]uint32
}

func (m *testMockCMap) LookupCID(code uint32) (uint32, bool) {
	cid, ok := m.cid[code]
	return cid, ok
}

func (m *testMockCMap) LookupUnicode(code uint32) (string, bool) {
	return "", false
}

func (m *testMockCMap) Name() string { return "TestCMap" }

func (m *testMockCMap) IsCIDBased() bool { return true }

func (m *testMockCMap) IsUnicode() bool { return false }

func TestCIDFontDefaults(t *testing.T) {
	f := NewCIDFont("TestCID", nil, &testMockCMap{cid: map[uint32]uint32{}})

	assert.Equal(t, "TestCID", f.Name())
	assert.True(t, f.IsCIDFont())
	assert.False(t, f.IsSymbolic())
	assert.Equal(t, uint16(1000), f.UnitsPerEm())
	assert.Equal(t, float64(1000), f.DefaultWidth())
	assert.Equal(t, float64(500), func() float64 { w, _ := f.DW2(); return w }())
	assert.Equal(t, -float64(500), func() float64 { _, h := f.DW2(); return h }())
}

func TestCIDFontGlyphResolution(t *testing.T) {
	cmap := &testMockCMap{
		cid: map[uint32]uint32{
			65: 42,
			66: 7,
		},
	}

	f := NewCIDFont("CID", nil, cmap)
	f.SetCIDToGID(42, 420)

	glyph, err := f.CharCodeToGlyph(65)
	require.NoError(t, err)
	assert.Equal(t, uint32(420), glyph)

	glyph, err = f.CharCodeToGlyph(66)
	require.NoError(t, err)
	assert.Equal(t, uint32(7), glyph)
}

func TestCIDFontFallbackAndSetters(t *testing.T) {
	cmap := &testMockCMap{cid: map[uint32]uint32{}}
	f := NewCIDFont("CID", nil, cmap)

	glyph, err := f.CharCodeToGlyph(77)
	require.NoError(t, err)
	assert.Equal(t, uint32(77), glyph)

	f.SetDefaultWidth(888)
	assert.Equal(t, float64(888), f.DefaultWidth())
	f.SetVertical(true)
	assert.True(t, f.IsVertical())
	f.SetVertical(false)
	assert.False(t, f.IsVertical())
	f.SetDW2(1.5, -1.5)
	w1, w2 := f.DW2()
	assert.Equal(t, float64(1.5), w1)
	assert.Equal(t, float64(-1.5), w2)
}

func TestCIDFontStateAndCIDSet(t *testing.T) {
	font := NewCIDFont("CID", nil, &testMockCMap{cid: map[uint32]uint32{}})
	font.SetCIDSet([]uint32{1, 2, 3})
	font.SetROS("Adobe", "GB1", 0)
	font.SetCMap(&testMockCMap{cid: map[uint32]uint32{5: 6}})
	assert.Equal(t, []uint32{1, 2, 3}, font.CIDSet())

	_, ok := font.ToUnicodeCMap().ToUnicode(1)
	assert.True(t, ok)

	font.ToUnicodeCMap()
	assert.Equal(t, "Adobe", font.ROS().Registry)
}

func TestCIDFontWidthAndBounding(t *testing.T) {
	f := NewCIDFont("CID", nil, &testMockCMap{cid: map[uint32]uint32{}})
	width, err := f.GetGlyphWidth(32)
	require.NoError(t, err)
	assert.Equal(t, float64(1000), width)

	x1, y1, x2, y2 := f.GetBoundingBox()
	assert.Equal(t, float64(0), x1)
	assert.Equal(t, float64(0), y1)
	assert.Equal(t, float64(1000), x2)
	assert.Equal(t, float64(1000), y2)
}
