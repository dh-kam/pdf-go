package unicode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCharUnicodeCategory_Whitespace(t *testing.T) {
	tests := []struct {
		name string
		char rune
	}{
		{"Space", ' '},
		{"Tab", '\t'},
		{"Newline", '\n'},
		{"CarriageReturn", '\r'},
		{"NoBreakSpace", 0x00A0},
		{"EnSpace", 0x2002},
		{"EmSpace", 0x2003},
		{"ThinSpace", 0x2009},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := GetCharUnicodeCategory(tt.char)
			assert.True(t, category.IsWhitespace, "Expected whitespace character")
			assert.False(t, category.IsZeroWidthDiacritic, "Whitespace should not be diacritic")
			assert.False(t, category.IsInvisibleFormatMark, "Whitespace should not be format mark")
		})
	}
}

func TestGetCharUnicodeCategory_ZeroWidthDiacritic(t *testing.T) {
	tests := []struct {
		name string
		char rune
	}{
		{"CombiningGrave", 0x0300},      // Combining Grave Accent
		{"CombiningAcute", 0x0301},      // Combining Acute Accent
		{"CombiningCircumflex", 0x0302}, // Combining Circumflex Accent
		{"CombiningTilde", 0x0303},      // Combining Tilde
		{"CombiningMacron", 0x0304},     // Combining Macron
		{"CombiningDiaeresis", 0x0308},  // Combining Diaeresis (umlaut)
		{"CombiningRingAbove", 0x030A},  // Combining Ring Above
		{"CombiningCedilla", 0x0327},    // Combining Cedilla
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := GetCharUnicodeCategory(tt.char)
			assert.True(t, category.IsZeroWidthDiacritic, "Expected zero-width diacritic")
			assert.False(t, category.IsWhitespace, "Diacritic should not be whitespace")
			assert.False(t, category.IsInvisibleFormatMark, "Diacritic should not be format mark")
		})
	}
}

func TestGetCharUnicodeCategory_InvisibleFormatMark(t *testing.T) {
	tests := []struct {
		name string
		char rune
	}{
		{"ZeroWidthSpace", 0x200B},        // Zero Width Space
		{"ZeroWidthNonJoiner", 0x200C},    // Zero Width Non-Joiner
		{"ZeroWidthJoiner", 0x200D},       // Zero Width Joiner
		{"LeftToRightMark", 0x200E},       // Left-to-Right Mark
		{"RightToLeftMark", 0x200F},       // Right-to-Left Mark
		{"SoftHyphen", 0x00AD},            // Soft Hyphen
		{"ZeroWidthNoBreakSpace", 0xFEFF}, // Zero Width No-Break Space (BOM)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := GetCharUnicodeCategory(tt.char)
			assert.True(t, category.IsInvisibleFormatMark, "Expected invisible format mark")
			assert.False(t, category.IsWhitespace, "Format mark should not be whitespace")
			assert.False(t, category.IsZeroWidthDiacritic, "Format mark should not be diacritic")
		})
	}
}

func TestGetCharUnicodeCategory_RegularCharacters(t *testing.T) {
	tests := []struct {
		name string
		char rune
	}{
		{"LatinA", 'A'},
		{"LatinZ", 'Z'},
		{"Digit0", '0'},
		{"Digit9", '9'},
		{"Copyright", 0x00A9},
		{"Euro", 0x20AC},
		{"CJK", 0x4E00},
		{"Emoji", 0x1F600},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := GetCharUnicodeCategory(tt.char)
			assert.False(t, category.IsWhitespace, "Regular character should not be whitespace")
			assert.False(t, category.IsZeroWidthDiacritic, "Regular character should not be diacritic")
			assert.False(t, category.IsInvisibleFormatMark, "Regular character should not be format mark")
		})
	}
}

func TestGetCharUnicodeCategory_Caching(t *testing.T) {
	// Clear cache before test
	ClearUnicodeCaches()

	// First call should compute and cache
	char := 'A'
	category1 := GetCharUnicodeCategory(char)

	// Cache should have 1 entry
	assert.Equal(t, 1, GetCacheSize(), "Expected cache to have 1 entry")

	// Second call should use cache
	category2 := GetCharUnicodeCategory(char)

	// Should be identical
	assert.Equal(t, category1, category2, "Cached result should match")

	// Cache should still have 1 entry
	assert.Equal(t, 1, GetCacheSize(), "Expected cache to still have 1 entry")
}

func TestClearUnicodeCaches(t *testing.T) {
	// Add some entries to cache
	GetCharUnicodeCategory('A')
	GetCharUnicodeCategory('B')
	GetCharUnicodeCategory('C')

	// Verify cache has entries
	assert.Greater(t, GetCacheSize(), 0, "Expected cache to have entries")

	// Clear cache
	ClearUnicodeCaches()

	// Cache should be empty
	assert.Equal(t, 0, GetCacheSize(), "Expected cache to be empty")
}

func TestGetCharUnicodeCategory_MultipleCategories(t *testing.T) {
	// Some characters might theoretically belong to multiple categories
	// Test edge cases
	tests := []struct {
		name             string
		char             rune
		expectWhitespace bool
		expectDiacritic  bool
		expectFormatMark bool
	}{
		// Normal cases
		{"Space", ' ', true, false, false},
		{"Letter", 'A', false, false, false},
		{"Diacritic", 0x0300, false, true, false},
		{"FormatMark", 0x200B, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := GetCharUnicodeCategory(tt.char)
			assert.Equal(t, tt.expectWhitespace, category.IsWhitespace)
			assert.Equal(t, tt.expectDiacritic, category.IsZeroWidthDiacritic)
			assert.Equal(t, tt.expectFormatMark, category.IsInvisibleFormatMark)
		})
	}
}

func TestGetCharUnicodeCategory_ArabicDiacritics(t *testing.T) {
	// Test Arabic combining marks
	tests := []struct {
		name string
		char rune
	}{
		{"Fatha", 0x064E},  // Arabic Fatha
		{"Damma", 0x064F},  // Arabic Damma
		{"Kasra", 0x0650},  // Arabic Kasra
		{"Sukun", 0x0652},  // Arabic Sukun
		{"Shadda", 0x0651}, // Arabic Shadda
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := GetCharUnicodeCategory(tt.char)
			assert.True(t, category.IsZeroWidthDiacritic, "Expected Arabic diacritic to be zero-width")
		})
	}
}

func TestGetCharUnicodeCategory_ConcurrentAccess(t *testing.T) {
	// Test that concurrent access doesn't cause race conditions
	ClearUnicodeCaches()

	const numGoroutines = 10
	const numIterations = 100

	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < numIterations; j++ {
				char := rune('A' + (id+j)%26)
				GetCharUnicodeCategory(char)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to finish
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Cache should have at most 26 entries (A-Z)
	cacheSize := GetCacheSize()
	assert.LessOrEqual(t, cacheSize, 26, "Cache should not exceed expected size")
	assert.Greater(t, cacheSize, 0, "Cache should have entries")
}

func TestGetCacheSize(t *testing.T) {
	ClearUnicodeCaches()

	// Initially empty
	assert.Equal(t, 0, GetCacheSize())

	// Add one entry
	GetCharUnicodeCategory('A')
	assert.Equal(t, 1, GetCacheSize())

	// Add another
	GetCharUnicodeCategory('B')
	assert.Equal(t, 2, GetCacheSize())

	// Same character doesn't increase size
	GetCharUnicodeCategory('A')
	assert.Equal(t, 2, GetCacheSize())
}

func TestGetCharUnicodeCategory_HebrewDiacritics(t *testing.T) {
	// Test Hebrew combining marks
	tests := []struct {
		name string
		char rune
	}{
		{"HebrewPoint_Sheva", 0x05B0},
		{"HebrewPoint_Qamats", 0x05B8},
		{"HebrewPoint_Dagesh", 0x05BC},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category := GetCharUnicodeCategory(tt.char)
			assert.True(t, category.IsZeroWidthDiacritic, "Expected Hebrew diacritic")
		})
	}
}
