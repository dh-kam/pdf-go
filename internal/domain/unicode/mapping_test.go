package unicode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapSpecialUnicodeValues_PUASymbols(t *testing.T) {
	tests := []struct {
		name     string
		input    rune
		expected rune
	}{
		// Copyright symbols
		{"CopyrightSans", 0xF8E9, 0x00A9},
		{"CopyrightSerif", 0xF6D9, 0x00A9},

		// Register symbols
		{"RegisterSans", 0xF8E8, 0x00AE},
		{"RegisterSerif", 0xF6DA, 0x00AE},

		// Trademark symbols
		{"TrademarkSans", 0xF8EA, 0x2122},
		{"TrademarkSerif", 0xF6DB, 0x2122},

		// Brace characters
		{"BraceLeftTop", 0xF8F1, 0x23A7},
		{"BraceLeftMid", 0xF8F2, 0x23A8},
		{"BraceLeftBottom", 0xF8F3, 0x23A9},
		{"BraceRightTop", 0xF8FC, 0x23AB},
		{"BraceRightMid", 0xF8FD, 0x23AC},
		{"BraceRightBottom", 0xF8FE, 0x23AD},

		// Bracket characters
		{"BracketLeftTop", 0xF8EE, 0x23A1},
		{"BracketLeftEx", 0xF8EF, 0x23A2},
		{"BracketLeftBottom", 0xF8F0, 0x23A3},
		{"BracketRightTop", 0xF8F9, 0x23A4},
		{"BracketRightEx", 0xF8FA, 0x23A5},
		{"BracketRightBottom", 0xF8FB, 0x23A6},

		// Parenthesis characters
		{"ParenLeftTop", 0xF8EB, 0x239B},
		{"ParenLeftEx", 0xF8EC, 0x239C},
		{"ParenLeftBottom", 0xF8ED, 0x239D},
		{"ParenRightTop", 0xF8F6, 0x239E},
		{"ParenRightEx", 0xF8F7, 0x239F},
		{"ParenRightBottom", 0xF8F8, 0x23A0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapSpecialUnicodeValues(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapSpecialUnicodeValues_SoftHyphen(t *testing.T) {
	// Soft hyphen (0x00AD) should be mapped to regular hyphen (0x002D)
	result := MapSpecialUnicodeValues(0x00AD)
	assert.Equal(t, rune(0x002D), result)
}

func TestMapSpecialUnicodeValues_SpecialsBlock(t *testing.T) {
	// Specials unicode block (0xFFF0-0xFFFF) should return 0
	tests := []rune{
		0xFFF0,
		0xFFF5,
		0xFFFA,
		0xFFFF,
	}

	for _, code := range tests {
		t.Run("", func(t *testing.T) {
			result := MapSpecialUnicodeValues(code)
			assert.Equal(t, rune(0), result)
		})
	}
}

func TestMapSpecialUnicodeValues_UnmappedPUA(t *testing.T) {
	// PUA characters not in the mapping table should be returned as-is
	tests := []rune{
		0xF600, // Start of PUA range
		0xF700, // Middle
		0xF8FF, // End of PUA range
	}

	for _, code := range tests {
		t.Run("", func(t *testing.T) {
			result := MapSpecialUnicodeValues(code)
			assert.Equal(t, code, result)
		})
	}
}

func TestMapSpecialUnicodeValues_RegularCharacters(t *testing.T) {
	// Regular characters should pass through unchanged
	tests := []struct {
		name  string
		input rune
	}{
		{"LatinA", 'A'},
		{"LatinZ", 'Z'},
		{"Digit0", '0'},
		{"Digit9", '9'},
		{"Space", ' '},
		{"Copyright", 0x00A9}, // Standard copyright
		{"Register", 0x00AE},  // Standard register
		{"Trademark", 0x2122}, // Standard trademark
		{"CJK", 0x4E00},       // CJK Unified Ideograph
		{"Emoji", 0x1F600},    // Emoji
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapSpecialUnicodeValues(tt.input)
			assert.Equal(t, tt.input, result)
		})
	}
}

func TestMapSpecialUnicodeValues_BoundaryValues(t *testing.T) {
	tests := []struct {
		name     string
		input    rune
		expected rune
	}{
		{"BeforePUA", 0xF5FF, 0xF5FF},      // Just before PUA range
		{"AfterPUA", 0xF900, 0xF900},       // Just after PUA range
		{"BeforeSpecials", 0xFFEF, 0xFFEF}, // Just before Specials
		{"StartPUA", 0xF600, 0xF600},       // Start of PUA (unmapped)
		{"EndPUA", 0xF8FF, 0xF8FF},         // End of PUA (unmapped)
		{"StartSpecials", 0xFFF0, 0},       // Start of Specials
		{"EndSpecials", 0xFFFF, 0},         // End of Specials
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapSpecialUnicodeValues(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
