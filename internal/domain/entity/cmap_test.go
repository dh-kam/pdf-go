// Package entity tests for CMap functionality.
package entity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRangeCIDtoUnicodeMap tests the RangeCIDtoUnicodeMap implementation.
func TestRangeCIDtoUnicodeMap(t *testing.T) {
	t.Run("NewRangeCIDtoUnicodeMap creates empty map", func(t *testing.T) {
		m := NewRangeCIDtoUnicodeMap()
		assert.NotNil(t, m)
		assert.Empty(t, m.ranges)
	})

	t.Run("AddRange adds a mapping range", func(t *testing.T) {
		m := NewRangeCIDtoUnicodeMap()
		m.AddRange(0, 10, 0x0020) // CIDs 0-10 map to U+0020-U+002A

		assert.Len(t, m.ranges, 1)
		assert.Equal(t, uint32(0), m.ranges[0].Low)
		assert.Equal(t, uint32(10), m.ranges[0].High)
		assert.Equal(t, uint32(0x0020), m.ranges[0].UnicodeLow)
	})

	t.Run("AddRange adds multiple ranges", func(t *testing.T) {
		m := NewRangeCIDtoUnicodeMap()
		m.AddRange(0, 10, 0x0020)
		m.AddRange(100, 110, 0x0041)
		m.AddRange(200, 205, 0x0061)

		assert.Len(t, m.ranges, 3)
	})

	t.Run("ToUnicode maps CID within range", func(t *testing.T) {
		m := NewRangeCIDtoUnicodeMap()
		m.AddRange(0, 10, 0x0020)

		// Test start of range
		r, ok := m.ToUnicode(0)
		assert.True(t, ok)
		assert.Equal(t, rune(0x0020), r)

		// Test middle of range
		r, ok = m.ToUnicode(5)
		assert.True(t, ok)
		assert.Equal(t, rune(0x0025), r)

		// Test end of range
		r, ok = m.ToUnicode(10)
		assert.True(t, ok)
		assert.Equal(t, rune(0x002A), r)
	})

	t.Run("ToUnicode returns false for CID outside range", func(t *testing.T) {
		m := NewRangeCIDtoUnicodeMap()
		m.AddRange(0, 10, 0x0020)

		r, ok := m.ToUnicode(11)
		assert.False(t, ok)
		assert.Equal(t, rune(0), r)

		r, ok = m.ToUnicode(100)
		assert.False(t, ok)
		assert.Equal(t, rune(0), r)
	})

	t.Run("ToUnicodeString maps CID to string", func(t *testing.T) {
		m := NewRangeCIDtoUnicodeMap()
		m.AddRange(0, 10, 0x0020)

		s, ok := m.ToUnicodeString(5)
		assert.True(t, ok)
		assert.Equal(t, "\u0025", s)
	})

	t.Run("ToUnicodeString returns false for CID outside range", func(t *testing.T) {
		m := NewRangeCIDtoUnicodeMap()
		m.AddRange(0, 10, 0x0020)

		s, ok := m.ToUnicodeString(100)
		assert.False(t, ok)
		assert.Empty(t, s)
	})

	t.Run("ToUnicode works with multiple ranges", func(t *testing.T) {
		m := NewRangeCIDtoUnicodeMap()
		m.AddRange(0, 10, 0x0020)
		m.AddRange(100, 110, 0x0041)

		// Test first range
		r, ok := m.ToUnicode(5)
		assert.True(t, ok)
		assert.Equal(t, rune(0x0025), r)

		// Test second range
		r, ok = m.ToUnicode(105)
		assert.True(t, ok)
		assert.Equal(t, rune(0x0046), r)
	})

	t.Run("ToUnicode handles zero CID", func(t *testing.T) {
		m := NewRangeCIDtoUnicodeMap()
		m.AddRange(0, 10, 0x0020)

		r, ok := m.ToUnicode(0)
		assert.True(t, ok)
		assert.Equal(t, rune(0x0020), r)
	})

	t.Run("ToUnicode handles large Unicode values", func(t *testing.T) {
		m := NewRangeCIDtoUnicodeMap()
		m.AddRange(0, 5, 0x4E00) // CJK range

		r, ok := m.ToUnicode(3)
		assert.True(t, ok)
		assert.Equal(t, rune(0x4E03), r) // U+4E03 (CJK character)
	})
}

// TestMapCIDtoUnicodeMap tests the MapCIDtoUnicodeMap implementation.
func TestMapCIDtoUnicodeMap(t *testing.T) {
	t.Run("NewMapCIDtoUnicodeMap creates empty map", func(t *testing.T) {
		m := NewMapCIDtoUnicodeMap()
		assert.NotNil(t, m)
		assert.NotNil(t, m.mapping)
		assert.Empty(t, m.mapping)
	})

	t.Run("Add adds a single mapping", func(t *testing.T) {
		m := NewMapCIDtoUnicodeMap()
		m.Add(1, 'A')
		m.Add(2, 'B')
		m.Add(3, 'C')

		assert.Len(t, m.mapping, 3)
		assert.Equal(t, rune('A'), m.mapping[1])
		assert.Equal(t, rune('B'), m.mapping[2])
		assert.Equal(t, rune('C'), m.mapping[3])
	})

	t.Run("Add overwrites existing mapping", func(t *testing.T) {
		m := NewMapCIDtoUnicodeMap()
		m.Add(1, 'A')
		m.Add(1, 'B')

		assert.Len(t, m.mapping, 1)
		assert.Equal(t, rune('B'), m.mapping[1])
	})

	t.Run("ToUnicode returns mapped value", func(t *testing.T) {
		m := NewMapCIDtoUnicodeMap()
		m.Add(100, 'X')
		m.Add(200, 'Y')

		r, ok := m.ToUnicode(100)
		assert.True(t, ok)
		assert.Equal(t, rune('X'), r)

		r, ok = m.ToUnicode(200)
		assert.True(t, ok)
		assert.Equal(t, rune('Y'), r)
	})

	t.Run("ToUnicode returns false for unmapped CID", func(t *testing.T) {
		m := NewMapCIDtoUnicodeMap()
		m.Add(100, 'X')

		r, ok := m.ToUnicode(200)
		assert.False(t, ok)
		assert.Equal(t, rune(0), r)
	})

	t.Run("ToUnicodeString returns mapped string", func(t *testing.T) {
		m := NewMapCIDtoUnicodeMap()
		m.Add(1, '中')
		m.Add(2, '文')

		s, ok := m.ToUnicodeString(1)
		assert.True(t, ok)
		assert.Equal(t, "中", s)

		s, ok = m.ToUnicodeString(2)
		assert.True(t, ok)
		assert.Equal(t, "文", s)
	})

	t.Run("ToUnicodeString returns false for unmapped CID", func(t *testing.T) {
		m := NewMapCIDtoUnicodeMap()
		m.Add(1, 'A')

		s, ok := m.ToUnicodeString(999)
		assert.False(t, ok)
		assert.Empty(t, s)
	})

	t.Run("ToUnicode handles zero CID", func(t *testing.T) {
		m := NewMapCIDtoUnicodeMap()
		m.Add(0, 'N')

		r, ok := m.ToUnicode(0)
		assert.True(t, ok)
		assert.Equal(t, rune('N'), r)
	})

	t.Run("ToUnicode handles large CID values", func(t *testing.T) {
		m := NewMapCIDtoUnicodeMap()
		largeCID := uint32(65535)
		m.Add(largeCID, 'Z')

		r, ok := m.ToUnicode(largeCID)
		assert.True(t, ok)
		assert.Equal(t, rune('Z'), r)
	})

	t.Run("ToUnicode handles Unicode surrogate pairs", func(t *testing.T) {
		m := NewMapCIDtoUnicodeMap()
		// U+1F600 (grinning face emoji) - outside BMP
		m.Add(1, 0x1F600)

		r, ok := m.ToUnicode(1)
		assert.True(t, ok)
		assert.Equal(t, rune(0x1F600), r)
	})

	t.Run("ToUnicodeString handles multiple mappings", func(t *testing.T) {
		m := NewMapCIDtoUnicodeMap()
		for i := uint32(0); i < 100; i++ {
			m.Add(i, rune(0x0020+i))
		}

		s, ok := m.ToUnicodeString(50)
		assert.True(t, ok)
		assert.Equal(t, string(rune(0x0020+50)), s)
	})
}

// TestCMapRange tests the CMapRange struct.
func TestCMapRange(t *testing.T) {
	t.Run("CMapRange holds correct values", func(t *testing.T) {
		r := CMapRange{
			Low:  100,
			High: 200,
		}
		assert.Equal(t, uint32(100), r.Low)
		assert.Equal(t, uint32(200), r.High)
	})

	t.Run("CMapRange with zero values", func(t *testing.T) {
		r := CMapRange{}
		assert.Equal(t, uint32(0), r.Low)
		assert.Equal(t, uint32(0), r.High)
	})
}

// TestCMapMapping tests the CMapMapping struct.
func TestCMapMapping(t *testing.T) {
	t.Run("CMapMapping holds correct values", func(t *testing.T) {
		m := CMapMapping{
			Code:  100,
			Value: 200,
		}
		assert.Equal(t, uint32(100), m.Code)
		assert.Equal(t, uint32(200), m.Value)
	})

	t.Run("CMapMapping with zero values", func(t *testing.T) {
		m := CMapMapping{}
		assert.Equal(t, uint32(0), m.Code)
		assert.Equal(t, uint32(0), m.Value)
	})
}

// TestCIDUnicodeRange tests the CIDUnicodeRange struct.
func TestCIDUnicodeRange(t *testing.T) {
	t.Run("CIDUnicodeRange holds correct values", func(t *testing.T) {
		r := CIDUnicodeRange{
			Low:        100,
			High:       200,
			UnicodeLow: 0x4E00,
		}
		assert.Equal(t, uint32(100), r.Low)
		assert.Equal(t, uint32(200), r.High)
		assert.Equal(t, uint32(0x4E00), r.UnicodeLow)
	})

	t.Run("CIDUnicodeRange with zero values", func(t *testing.T) {
		r := CIDUnicodeRange{}
		assert.Equal(t, uint32(0), r.Low)
		assert.Equal(t, uint32(0), r.High)
		assert.Equal(t, uint32(0), r.UnicodeLow)
	})
}

// TestCIDSystemInfo tests the CIDSystemInfo struct.
func TestCIDSystemInfo(t *testing.T) {
	t.Run("CIDSystemInfo holds correct values", func(t *testing.T) {
		info := CIDSystemInfo{
			Registry:   "Adobe",
			Ordering:   "GB1",
			Supplement: 0,
		}
		assert.Equal(t, "Adobe", info.Registry)
		assert.Equal(t, "GB1", info.Ordering)
		assert.Equal(t, 0, info.Supplement)
	})

	t.Run("CIDSystemInfo with empty values", func(t *testing.T) {
		info := CIDSystemInfo{}
		assert.Empty(t, info.Registry)
		assert.Empty(t, info.Ordering)
		assert.Equal(t, 0, info.Supplement)
	})
}

// TestROS tests the ROS struct.
func TestROS(t *testing.T) {
	t.Run("ROS holds correct values", func(t *testing.T) {
		ros := ROS{
			Registry:   "Adobe",
			Ordering:   "CNS1",
			Supplement: 1,
		}
		assert.Equal(t, "Adobe", ros.Registry)
		assert.Equal(t, "CNS1", ros.Ordering)
		assert.Equal(t, 1, ros.Supplement)
	})

	t.Run("ROS with zero values", func(t *testing.T) {
		ros := ROS{}
		assert.Empty(t, ros.Registry)
		assert.Empty(t, ros.Ordering)
		assert.Equal(t, 0, ros.Supplement)
	})
}

// TestWritingMode tests the WritingMode type.
func TestWritingMode(t *testing.T) {
	t.Run("WritingModeHorizontal has correct value", func(t *testing.T) {
		assert.Equal(t, WritingMode(0), WritingModeHorizontal)
	})

	t.Run("WritingModeVertical has correct value", func(t *testing.T) {
		assert.Equal(t, WritingMode(1), WritingModeVertical)
	})

	t.Run("WritingMode values are distinct", func(t *testing.T) {
		assert.NotEqual(t, WritingModeHorizontal, WritingModeVertical)
	})
}

// TestCMapType tests the CMapType type.
func TestCMapType(t *testing.T) {
	t.Run("CMapTypeType0 has correct value", func(t *testing.T) {
		assert.Equal(t, CMapType(0), CMapTypeType0)
	})

	t.Run("CMapTypeType1 has correct value", func(t *testing.T) {
		assert.Equal(t, CMapType(1), CMapTypeType1)
	})

	t.Run("CMapTypeType2 has correct value", func(t *testing.T) {
		assert.Equal(t, CMapType(2), CMapTypeType2)
	})

	t.Run("CMapType values are distinct", func(t *testing.T) {
		assert.NotEqual(t, CMapTypeType0, CMapTypeType1)
		assert.NotEqual(t, CMapTypeType1, CMapTypeType2)
		assert.NotEqual(t, CMapTypeType0, CMapTypeType2)
	})
}
