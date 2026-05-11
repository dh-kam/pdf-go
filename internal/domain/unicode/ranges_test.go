package unicode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetUnicodeRangeFor_BasicLatin(t *testing.T) {
	tests := []struct {
		name  string
		value rune
	}{
		{"Space", ' '},
		{"A", 'A'},
		{"Z", 'Z'},
		{"a", 'a'},
		{"z", 'z'},
		{"0", '0'},
		{"9", '9'},
		{"Tilde", '~'},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeRangeFor(tt.value, -1)
			assert.Equal(t, 0, result, "Expected Basic Latin range (0)")
		})
	}
}

func TestGetUnicodeRangeFor_Latin1Supplement(t *testing.T) {
	tests := []struct {
		name  string
		value rune
	}{
		{"Copyright", 0x00A9},
		{"Register", 0x00AE},
		{"Degree", 0x00B0},
		{"AE", 0x00C6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeRangeFor(tt.value, -1)
			assert.Equal(t, 1, result, "Expected Latin-1 Supplement range (1)")
		})
	}
}

func TestGetUnicodeRangeFor_Greek(t *testing.T) {
	tests := []struct {
		name  string
		value rune
	}{
		{"Alpha", 0x03B1},
		{"Beta", 0x03B2},
		{"Gamma", 0x03B3},
		{"Omega", 0x03C9},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeRangeFor(tt.value, -1)
			assert.Equal(t, 7, result, "Expected Greek and Coptic range (7)")
		})
	}
}

func TestGetUnicodeRangeFor_Hebrew(t *testing.T) {
	result := GetUnicodeRangeFor(0x05D0, -1) // Alef
	assert.Equal(t, 11, result, "Expected Hebrew range (11)")
}

func TestGetUnicodeRangeFor_Arabic(t *testing.T) {
	tests := []struct {
		name  string
		value rune
	}{
		{"ArabicBasic", 0x0627}, // Alif
		{"ArabicSupplement", 0x0750},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeRangeFor(tt.value, -1)
			assert.Equal(t, 13, result, "Expected Arabic range (13)")
		})
	}
}

func TestGetUnicodeRangeFor_CJK(t *testing.T) {
	tests := []struct {
		name     string
		value    rune
		rangeIdx int
	}{
		{"Hiragana", 0x3042, 49},
		{"Katakana", 0x30A2, 50},
		{"Hangul", 0xAC00, 56},
		{"CJK_Unified", 0x4E00, 59},
		{"CJK_ExtA", 0x3400, 59},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeRangeFor(tt.value, -1)
			assert.Equal(t, tt.rangeIdx, result)
		})
	}
}

func TestGetUnicodeRangeFor_Symbols(t *testing.T) {
	tests := []struct {
		name     string
		value    rune
		rangeIdx int
	}{
		{"Copyright", 0x00A9, 1},
		{"Euro", 0x20AC, 33},
		{"Arrow", 0x2190, 37},
		{"MathOperator", 0x2200, 38},
		{"BoxDrawing", 0x2500, 43},
		{"GeometricShape", 0x25A0, 45},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeRangeFor(tt.value, -1)
			assert.Equal(t, tt.rangeIdx, result)
		})
	}
}

func TestGetUnicodeRangeFor_PrivateUseArea(t *testing.T) {
	tests := []struct {
		name  string
		value rune
	}{
		{"PUA_Start", 0xE000},
		{"PUA_Mid", 0xF000},
		{"PUA_End", 0xF8FF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeRangeFor(tt.value, -1)
			assert.Equal(t, 60, result, "Expected Private Use Area range (60)")
		})
	}
}

func TestGetUnicodeRangeFor_NotFound(t *testing.T) {
	// Test values that are not in any defined range
	tests := []struct {
		name  string
		value rune
	}{
		{"BetweenRanges1", 0x0900 - 1}, // Just before Devanagari
		{"BetweenRanges2", 0x1000000},  // Way beyond defined ranges
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeRangeFor(tt.value, -1)
			assert.Equal(t, -1, result, "Expected -1 for undefined range")
		})
	}
}

func TestGetUnicodeRangeFor_WithLastPosition(t *testing.T) {
	// Test the lastPosition optimization
	value := rune('A') // Basic Latin

	// First call without hint
	result1 := GetUnicodeRangeFor(value, -1)
	assert.Equal(t, 0, result1)

	// Second call with correct hint
	result2 := GetUnicodeRangeFor(value, 0)
	assert.Equal(t, 0, result2)

	// Call with wrong hint should still find correct range
	result3 := GetUnicodeRangeFor(value, 5)
	assert.Equal(t, 0, result3)
}

func TestGetUnicodeRangeFor_Boundaries(t *testing.T) {
	tests := []struct {
		name     string
		value    rune
		expected int
	}{
		{"BasicLatin_Start", 0x0000, 0},
		{"BasicLatin_End", 0x007F, 0},
		{"Latin1_Start", 0x0080, 1},
		{"Latin1_End", 0x00FF, 1},
		{"JustBefore_BasicLatin", -1, -1}, // Invalid
		{"JustAfter_Latin1", 0x0100, 2},   // Latin Extended-A
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeRangeFor(tt.value, -1)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUnicodeRangeFor_MultipleSubRanges(t *testing.T) {
	// Test ranges that have multiple sub-ranges
	// Range 59 (CJK) has many sub-ranges
	tests := []struct {
		name  string
		value rune
	}{
		{"CJK_Unified", 0x4E00},
		{"CJK_Radicals", 0x2E80},
		{"Kangxi", 0x2F00},
		{"IdeographicDesc", 0x2FF0},
		{"CJK_ExtA", 0x3400},
		{"Kanbun", 0x3190},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeRangeFor(tt.value, -1)
			assert.Equal(t, 59, result, "Expected CJK range (59)")
		})
	}
}

func TestGetUnicodeRangeFor_AstralPlanes(t *testing.T) {
	// Test characters beyond BMP (Basic Multilingual Plane)
	tests := []struct {
		name     string
		value    rune
		expected int
	}{
		{"LinearB", 0x10000, 101},
		{"Byzantine", 0x1D000, 88},
		{"MathAlpha", 0x1D400, 89},
		{"Emoji", 0x1F600, -1}, // Not in defined ranges
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeRangeFor(tt.value, -1)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUnicodeRangeFor_IndianScripts(t *testing.T) {
	tests := []struct {
		name     string
		value    rune
		expected int
	}{
		{"Devanagari", 0x0900, 15},
		{"Bengali", 0x0980, 16},
		{"Gurmukhi", 0x0A00, 17},
		{"Gujarati", 0x0A80, 18},
		{"Oriya", 0x0B00, 19},
		{"Tamil", 0x0B80, 20},
		{"Telugu", 0x0C00, 21},
		{"Kannada", 0x0C80, 22},
		{"Malayalam", 0x0D00, 23},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeRangeFor(tt.value, -1)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestUnicodeRanges_Count(t *testing.T) {
	// Verify we have 123 ranges (0-122)
	assert.Equal(t, 123, len(UnicodeRanges), "Expected 123 Unicode ranges")
}

func TestIsInRange(t *testing.T) {
	// Test the helper function directly
	tests := []struct {
		name     string
		rang     UnicodeRange
		value    rune
		expected bool
	}{
		{"InSingleRange", UnicodeRange{0x0000, 0x007F}, 'A', true},
		{"NotInSingleRange", UnicodeRange{0x0000, 0x007F}, 0x0100, false},
		{"InFirstSubRange", UnicodeRange{0x0250, 0x02AF, 0x1D00, 0x1D7F}, 0x0250, true},
		{"InSecondSubRange", UnicodeRange{0x0250, 0x02AF, 0x1D00, 0x1D7F}, 0x1D00, true},
		{"BetweenSubRanges", UnicodeRange{0x0250, 0x02AF, 0x1D00, 0x1D7F}, 0x0500, false},
		{"JustBefore", UnicodeRange{0x0250, 0x02AF}, 0x024F, false},
		{"JustAfter", UnicodeRange{0x0250, 0x02AF}, 0x02B0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isInRange(tt.value, tt.rang)
			assert.Equal(t, tt.expected, result)
		})
	}
}
