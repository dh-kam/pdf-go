package unicode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetUnicodeForGlyph_UniFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected rune
	}{
		{"uniXXXX_Copyright", "uni00A9", 0x00A9},
		{"uniXXXX_Register", "uni00AE", 0x00AE},
		{"uniXXXX_Euro", "uni20AC", 0x20AC},
		{"uniXXXX_CJK", "uni4E00", 0x4E00},
		{"uniXXXX_Arabic", "uni0600", 0x0600},
		{"uniXXXX_Emoji", "uniF600", 0xF600},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeForGlyph(tt.input, nil)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUnicodeForGlyph_UFormat(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected rune
	}{
		{"uXXXX_5chars", "u00A9", 0x00A9},
		{"uXXXX_6chars", "u20AC0", 0x20AC0},
		{"uXXXXXX_7chars", "u1F600", 0x1F600},
		{"uXXXX_Basic", "u0041", 0x0041}, // 'A'
		{"uXXXX_Extended", "u1FACE", 0x1FACE},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeForGlyph(tt.input, nil)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUnicodeForGlyph_InvalidFormat(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"NoPrefix", "XXXX"},
		{"WrongPrefix", "vniXXXX"},
		{"TooShort", "uXXX"},
		{"TooLong", "uXXXXXXXX"},
		{"Lowercase", "uni00a9"},    // lowercase hex
		{"MixedCase", "uni00aB"},    // mixed case 'B' is uppercase, 'a' is lowercase
		{"WrongFormat", "uniXXXXX"}, // 8 chars with 'uni' prefix
		{"InvalidHex", "uniGGGG"},
		{"Empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeForGlyph(tt.input, nil)
			assert.Equal(t, rune(-1), result)
		})
	}
}

func TestGetUnicodeForGlyph_WithGlyphsMap(t *testing.T) {
	glyphsMap := map[string]rune{
		"A":           0x0041,
		"B":           0x0042,
		"copyright":   0x00A9,
		"customGlyph": 0x1234,
	}

	tests := []struct {
		name     string
		input    string
		expected rune
	}{
		{"MapLookup_A", "A", 0x0041},
		{"MapLookup_B", "B", 0x0042},
		{"MapLookup_Copyright", "copyright", 0x00A9},
		{"MapLookup_Custom", "customGlyph", 0x1234},
		{"Fallback_UniFormat", "uni00AE", 0x00AE},
		{"Fallback_UFormat", "u2122", 0x2122},
		{"NotFound", "unknownGlyph", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeForGlyph(tt.input, glyphsMap)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUnicodeForGlyph_NilMap(t *testing.T) {
	// Should work fine with nil map
	result := GetUnicodeForGlyph("uni00A9", nil)
	assert.Equal(t, rune(0x00A9), result)
}

func TestGetUnicodeForGlyph_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected rune
	}{
		{"Zero", "uni0000", 0x0000},
		{"MaxBMP", "uniFFFF", 0xFFFF},
		{"SMP_Start", "u10000", 0x10000},
		{"Astral", "u1F600", 0x1F600},
		{"NegativeInvalid", "uniFFFFF", -1}, // > 0xFFFF in 4-digit format should fail
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeForGlyph(tt.input, nil)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUnicodeForGlyph_UppercaseRequirement(t *testing.T) {
	// Must be uppercase to avoid false positives
	tests := []struct {
		name     string
		input    string
		expected rune
	}{
		{"AllUppercase", "uni00A9", 0x00A9},
		{"AllLowercase", "uni00a9", -1},
		{"MixedCase", "uni00a9", -1},
		{"UpperU_Format", "u00A9", 0x00A9},
		{"LowerU_Format", "u00a9", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeForGlyph(tt.input, nil)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetUnicodeForGlyph_RealWorldExamples(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected rune
	}{
		{"Euro", "uni20AC", 0x20AC},
		{"Pound", "uni00A3", 0x00A3},
		{"Yen", "uni00A5", 0x00A5},
		{"Ellipsis", "uni2026", 0x2026},
		{"EnDash", "uni2013", 0x2013},
		{"EmDash", "uni2014", 0x2014},
		{"LeftQuote", "uni2018", 0x2018},
		{"RightQuote", "uni2019", 0x2019},
		{"Bullet", "uni2022", 0x2022},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetUnicodeForGlyph(tt.input, nil)
			assert.Equal(t, tt.expected, result)
		})
	}
}
