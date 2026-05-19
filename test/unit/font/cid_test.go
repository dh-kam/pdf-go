package font_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/infrastructure/font/cid"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/standard"
)

func TestCIDFont_NewCIDFont(t *testing.T) {
	baseFont, _ := standard.GetFont("Helvetica")
	testCmap := &mockCMap{}

	font := cid.NewCIDFont("TestCIDFont", baseFont, testCmap)

	assert.Equal(t, "TestCIDFont", font.Name())
	assert.True(t, font.IsCIDFont())
	assert.False(t, font.IsSymbolic())
	assert.False(t, font.IsVertical())
}

func TestCIDFont_CIDToGID(t *testing.T) {
	font := cid.NewCIDFont("Test", nil, nil)

	// Initially empty
	gid, ok := font.CIDToGID(100)
	assert.False(t, ok)
	assert.Equal(t, uint32(0), gid)

	// Set mapping
	font.SetCIDToGID(100, 200)
	gid, ok = font.CIDToGID(100)
	assert.True(t, ok)
	assert.Equal(t, uint32(200), gid)
}

func TestCIDFont_CharCodeToGlyph(t *testing.T) {
	testCmap := &mockCMap{
		cidMapping: map[uint32]uint32{
			0x41: 100,
			0x42: 200,
		},
	}

	font := cid.NewCIDFont("Test", nil, testCmap)

	// Test with CID mapping
	gid, err := font.CharCodeToGlyph(0x41)
	require.NoError(t, err)
	assert.Equal(t, uint32(100), gid)

	// Test with direct character code (no CMap mapping)
	gid, err = font.CharCodeToGlyph(0x43)
	require.NoError(t, err)
	assert.Equal(t, uint32(0x43), gid)
}

func TestCIDFont_VerticalMode(t *testing.T) {
	font := cid.NewCIDFont("Test", nil, nil)

	assert.False(t, font.IsVertical())

	font.SetVertical(true)
	assert.True(t, font.IsVertical())

	font.SetVertical(false)
	assert.False(t, font.IsVertical())
}

func TestCIDFont_DefaultWidth(t *testing.T) {
	font := cid.NewCIDFont("Test", nil, nil)

	assert.Equal(t, 1000.0, font.DefaultWidth())

	font.SetDefaultWidth(500.0)
	assert.Equal(t, 500.0, font.DefaultWidth())
}

func TestCIDFont_DW2(t *testing.T) {
	font := cid.NewCIDFont("Test", nil, nil)

	w1, w2 := font.DW2()
	assert.Equal(t, 500.0, w1)
	assert.Equal(t, -500.0, w2)

	font.SetDW2(1000.0, -1000.0)
	w1, w2 = font.DW2()
	assert.Equal(t, 1000.0, w1)
	assert.Equal(t, -1000.0, w2)
}

func TestCIDFont_CIDSet(t *testing.T) {
	font := cid.NewCIDFont("Test", nil, nil)

	assert.Empty(t, font.CIDSet())

	cids := []uint32{1, 2, 3, 4, 5}
	font.SetCIDSet(cids)

	assert.Equal(t, cids, font.CIDSet())
}

func TestCIDFont_ROS(t *testing.T) {
	font := cid.NewCIDFont("Test", nil, nil)

	font.SetROS("Adobe", "Japan1", 4)

	ros := font.ROS()
	assert.Equal(t, "Adobe", ros.Registry)
	assert.Equal(t, "Japan1", ros.Ordering)
	assert.Equal(t, 4, ros.Supplement)
}

func TestCIDFont_CMap(t *testing.T) {
	testCmap := &mockCMap{}
	font := cid.NewCIDFont("Test", nil, testCmap)

	assert.Equal(t, testCmap, font.CMap())

	newCmap := &mockCMap{}
	font.SetCMap(newCmap)
	assert.Equal(t, newCmap, font.CMap())
}

func TestCIDFont_ToUnicodeCMap(t *testing.T) {
	font := cid.NewCIDFont("Test", nil, nil)

	cids := []uint32{1, 2, 3, 4, 5}
	font.SetCIDSet(cids)

	toUnicode := font.ToUnicodeCMap()
	assert.NotNil(t, toUnicode)
}

func TestCIDFont_WithBaseFont(t *testing.T) {
	baseFont, _ := standard.GetFont("Helvetica")
	testCmap := &mockCMap{
		cidMapping: map[uint32]uint32{
			0x41: 100,
		},
	}

	font := cid.NewCIDFont("Test", baseFont, testCmap)

	// Test that base font methods are delegated
	assert.Equal(t, baseFont.UnitsPerEm(), font.UnitsPerEm())

	width, err := font.GetGlyphWidth(100)
	require.NoError(t, err)
	assert.Greater(t, width, 0.0)

	xMin, yMin, xMax, yMax := font.GetBoundingBox()
	assert.NotNil(t, font)
	assert.True(t, xMin <= xMax)
	assert.True(t, yMin <= yMax)
}

func TestCIDFont_RenderGlyph(t *testing.T) {
	baseFont, _ := standard.GetFont("Helvetica")
	font := cid.NewCIDFont("Test", baseFont, nil)

	path, err := font.RenderGlyph(100, 12.0)
	require.NoError(t, err)
	assert.NotNil(t, path)
}

func TestCIDFont_GlyphName(t *testing.T) {
	baseFont, _ := standard.GetFont("Helvetica")
	font := cid.NewCIDFont("Test", baseFont, nil)

	name := font.GlyphName(65)
	assert.NotEmpty(t, name)
}

// mockCMap is a simple mock for testing
type mockCMap struct {
	cidMapping map[uint32]uint32
	name       string
}

func (m *mockCMap) LookupCID(code uint32) (uint32, bool) {
	if m.cidMapping == nil {
		return 0, false
	}
	cid, ok := m.cidMapping[code]
	return cid, ok
}

func (m *mockCMap) LookupUnicode(code uint32) (string, bool) {
	return "", false
}

func (m *mockCMap) Name() string {
	if m.name == "" {
		return "MockCMap"
	}
	return m.name
}

func (m *mockCMap) IsCIDBased() bool {
	return true
}

func (m *mockCMap) IsUnicode() bool {
	return false
}
