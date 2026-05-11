//revive:disable:exported
package unicode

// UnicodeRange represents a Unicode range with start and end code points.
// Multiple ranges can be grouped together (e.g., Latin Extended has multiple sub-ranges).
type UnicodeRange []rune

// UnicodeRanges contains all Unicode ranges as defined in OpenType OS/2 table specification.
// Based on https://learn.microsoft.com/en-us/typography/opentype/spec/os2
// Each range is represented as pairs of [start, end] values.
var UnicodeRanges = []UnicodeRange{
	{0x0000, 0x007F}, // 0 - Basic Latin
	{0x0080, 0x00FF}, // 1 - Latin-1 Supplement
	{0x0100, 0x017F}, // 2 - Latin Extended-A
	{0x0180, 0x024F}, // 3 - Latin Extended-B
	{0x0250, 0x02AF, 0x1D00, 0x1D7F, 0x1D80, 0x1DBF},                 // 4 - IPA Extensions + Phonetic Extensions + Phonetic Extensions Supplement
	{0x02B0, 0x02FF, 0xA700, 0xA71F},                                 // 5 - Spacing Modifier Letters + Modifier Tone Letters
	{0x0300, 0x036F, 0x1DC0, 0x1DFF},                                 // 6 - Combining Diacritical Marks + Combining Diacritical Marks Supplement
	{0x0370, 0x03FF},                                                 // 7 - Greek and Coptic
	{0x2C80, 0x2CFF},                                                 // 8 - Coptic
	{0x0400, 0x04FF, 0x0500, 0x052F, 0x2DE0, 0x2DFF, 0xA640, 0xA69F}, // 9 - Cyrillic + Cyrillic Supplement + Cyrillic Extended-A + Cyrillic Extended-B
	{0x0530, 0x058F},                                                 // 10 - Armenian
	{0x0590, 0x05FF},                                                 // 11 - Hebrew
	{0xA500, 0xA63F},                                                 // 12 - Vai
	{0x0600, 0x06FF, 0x0750, 0x077F},                                 // 13 - Arabic + Arabic Supplement
	{0x07C0, 0x07FF},                                                 // 14 - NKo
	{0x0900, 0x097F},                                                 // 15 - Devanagari
	{0x0980, 0x09FF},                                                 // 16 - Bengali
	{0x0A00, 0x0A7F},                                                 // 17 - Gurmukhi
	{0x0A80, 0x0AFF},                                                 // 18 - Gujarati
	{0x0B00, 0x0B7F},                                                 // 19 - Oriya
	{0x0B80, 0x0BFF},                                                 // 20 - Tamil
	{0x0C00, 0x0C7F},                                                 // 21 - Telugu
	{0x0C80, 0x0CFF},                                                 // 22 - Kannada
	{0x0D00, 0x0D7F},                                                 // 23 - Malayalam
	{0x0E00, 0x0E7F},                                                 // 24 - Thai
	{0x0E80, 0x0EFF},                                                 // 25 - Lao
	{0x10A0, 0x10FF, 0x2D00, 0x2D2F},                                 // 26 - Georgian + Georgian Supplement
	{0x1B00, 0x1B7F},                                                 // 27 - Balinese
	{0x1100, 0x11FF},                                                 // 28 - Hangul Jamo
	{0x1E00, 0x1EFF, 0x2C60, 0x2C7F, 0xA720, 0xA7FF},                 // 29 - Latin Extended Additional + Latin Extended-C + Latin Extended-D
	{0x1F00, 0x1FFF},                                                 // 30 - Greek Extended
	{0x2000, 0x206F, 0x2E00, 0x2E7F},                                 // 31 - General Punctuation + Supplemental Punctuation
	{0x2070, 0x209F},                                                 // 32 - Superscripts And Subscripts
	{0x20A0, 0x20CF},                                                 // 33 - Currency Symbol
	{0x20D0, 0x20FF},                                                 // 34 - Combining Diacritical Marks for Symbols
	{0x2100, 0x214F},                                                 // 35 - Letterlike Symbols
	{0x2150, 0x218F},                                                 // 36 - Number Forms
	{0x2190, 0x21FF, 0x27F0, 0x27FF, 0x2900, 0x297F, 0x2B00, 0x2BFF}, // 37 - Arrows + Supplemental Arrows-A + Supplemental Arrows-B + Miscellaneous Symbols and Arrows
	{0x2200, 0x22FF, 0x2A00, 0x2AFF, 0x27C0, 0x27EF, 0x2980, 0x29FF}, // 38 - Mathematical Operators + Supplemental Mathematical Operators + Miscellaneous Mathematical Symbols-A + Miscellaneous Mathematical Symbols-B
	{0x2300, 0x23FF},                 // 39 - Miscellaneous Technical
	{0x2400, 0x243F},                 // 40 - Control Pictures
	{0x2440, 0x245F},                 // 41 - Optical Character Recognition
	{0x2460, 0x24FF},                 // 42 - Enclosed Alphanumerics
	{0x2500, 0x257F},                 // 43 - Box Drawing
	{0x2580, 0x259F},                 // 44 - Block Elements
	{0x25A0, 0x25FF},                 // 45 - Geometric Shapes
	{0x2600, 0x26FF},                 // 46 - Miscellaneous Symbols
	{0x2700, 0x27BF},                 // 47 - Dingbats
	{0x3000, 0x303F},                 // 48 - CJK Symbols And Punctuation
	{0x3040, 0x309F},                 // 49 - Hiragana
	{0x30A0, 0x30FF, 0x31F0, 0x31FF}, // 50 - Katakana + Katakana Phonetic Extensions
	{0x3100, 0x312F, 0x31A0, 0x31BF}, // 51 - Bopomofo + Bopomofo Extended
	{0x3130, 0x318F},                 // 52 - Hangul Compatibility Jamo
	{0xA840, 0xA87F},                 // 53 - Phags-pa
	{0x3200, 0x32FF},                 // 54 - Enclosed CJK Letters And Months
	{0x3300, 0x33FF},                 // 55 - CJK Compatibility
	{0xAC00, 0xD7AF},                 // 56 - Hangul Syllables
	{0xD800, 0xDFFF},                 // 57 - Non-Plane 0 (Surrogates)
	{0x10900, 0x1091F},               // 58 - Phoenician
	{0x4E00, 0x9FFF, 0x2E80, 0x2EFF, 0x2F00, 0x2FDF, 0x2FF0, 0x2FFF, 0x3400, 0x4DBF, 0x20000, 0x2A6DF, 0x3190, 0x319F}, // 59 - CJK Unified Ideographs + CJK Radicals Supplement + Kangxi Radicals + Ideographic Description Characters + CJK Unified Ideographs Extension A + CJK Unified Ideographs Extension B + Kanbun
	{0xE000, 0xF8FF}, // 60 - Private Use Area (plane 0)
	{0x31C0, 0x31EF, 0xF900, 0xFAFF, 0x2F800, 0x2FA1F}, // 61 - CJK Strokes + CJK Compatibility Ideographs + CJK Compatibility Ideographs Supplement
	{0xFB00, 0xFB4F}, // 62 - Alphabetic Presentation Forms
	{0xFB50, 0xFDFF}, // 63 - Arabic Presentation Forms-A
	{0xFE20, 0xFE2F}, // 64 - Combining Half Marks
	{0xFE10, 0xFE1F}, // 65 - Vertical Forms
	{0xFE50, 0xFE6F}, // 66 - Small Form Variants
	{0xFE70, 0xFEFF}, // 67 - Arabic Presentation Forms-B
	{0xFF00, 0xFFEF}, // 68 - Halfwidth And Fullwidth Forms
	{0xFFF0, 0xFFFF}, // 69 - Specials
	{0x0F00, 0x0FFF}, // 70 - Tibetan
	{0x0700, 0x074F}, // 71 - Syriac
	{0x0780, 0x07BF}, // 72 - Thaana
	{0x0D80, 0x0DFF}, // 73 - Sinhala
	{0x1000, 0x109F}, // 74 - Myanmar
	{0x1200, 0x137F, 0x1380, 0x139F, 0x2D80, 0x2DDF}, // 75 - Ethiopic + Ethiopic Supplement + Ethiopic Extended
	{0x13A0, 0x13FF}, // 76 - Cherokee
	{0x1400, 0x167F}, // 77 - Unified Canadian Aboriginal Syllabics
	{0x1680, 0x169F}, // 78 - Ogham
	{0x16A0, 0x16FF}, // 79 - Runic
	{0x1780, 0x17FF}, // 80 - Khmer
	{0x1800, 0x18AF}, // 81 - Mongolian
	{0x2800, 0x28FF}, // 82 - Braille Patterns
	{0xA000, 0xA48F}, // 83 - Yi Syllables
	{0x1700, 0x171F, 0x1720, 0x173F, 0x1740, 0x175F, 0x1760, 0x177F}, // 84 - Tagalog + Hanunoo + Buhid + Tagbanwa
	{0x10300, 0x1032F}, // 85 - Old Italic
	{0x10330, 0x1034F}, // 86 - Gothic
	{0x10400, 0x1044F}, // 87 - Deseret
	{0x1D000, 0x1D0FF, 0x1D100, 0x1D1FF, 0x1D200, 0x1D24F}, // 88 - Byzantine Musical Symbols + Musical Symbols + Ancient Greek Musical Notation
	{0x1D400, 0x1D7FF},                 // 89 - Mathematical Alphanumeric Symbols
	{0xFF000, 0xFFFFF},                 // 90 - Private Use (plane 15)
	{0xFE00, 0xFE0F, 0xE0100, 0xE01EF}, // 91 - Variation Selectors + Variation Selectors Supplement
	{0xE0000, 0xE007F},                 // 92 - Tags
	{0x1900, 0x194F},                   // 93 - Limbu
	{0x1950, 0x197F},                   // 94 - Tai Le
	{0x1980, 0x19DF},                   // 95 - New Tai Lue
	{0x1A00, 0x1A1F},                   // 96 - Buginese
	{0x2C00, 0x2C5F},                   // 97 - Glagolitic
	{0x2D30, 0x2D7F},                   // 98 - Tifinagh
	{0x4DC0, 0x4DFF},                   // 99 - Yijing Hexagram Symbols
	{0xA800, 0xA82F},                   // 100 - Syloti Nagri
	{0x10000, 0x1007F, 0x10080, 0x100FF, 0x10100, 0x1013F}, // 101 - Linear B Syllabary + Linear B Ideograms + Aegean Numbers
	{0x10140, 0x1018F},                   // 102 - Ancient Greek Numbers
	{0x10380, 0x1039F},                   // 103 - Ugaritic
	{0x103A0, 0x103DF},                   // 104 - Old Persian
	{0x10450, 0x1047F},                   // 105 - Shavian
	{0x10480, 0x104AF},                   // 106 - Osmanya
	{0x10800, 0x1083F},                   // 107 - Cypriot Syllabary
	{0x10A00, 0x10A5F},                   // 108 - Kharoshthi
	{0x1D300, 0x1D35F},                   // 109 - Tai Xuan Jing Symbols
	{0x12000, 0x123FF, 0x12400, 0x1247F}, // 110 - Cuneiform + Cuneiform Numbers and Punctuation
	{0x1D360, 0x1D37F},                   // 111 - Counting Rod Numerals
	{0x1B80, 0x1BBF},                     // 112 - Sundanese
	{0x1C00, 0x1C4F},                     // 113 - Lepcha
	{0x1C50, 0x1C7F},                     // 114 - Ol Chiki
	{0xA880, 0xA8DF},                     // 115 - Saurashtra
	{0xA900, 0xA92F},                     // 116 - Kayah Li
	{0xA930, 0xA95F},                     // 117 - Rejang
	{0xAA00, 0xAA5F},                     // 118 - Cham
	{0x10190, 0x101CF},                   // 119 - Ancient Symbols
	{0x101D0, 0x101FF},                   // 120 - Phaistos Disc
	{0x102A0, 0x102DF, 0x10280, 0x1029F, 0x10920, 0x1093F}, // 121 - Carian + Lycian + Lydian
	{0x1F030, 0x1F09F, 0x1F000, 0x1F02F},                   // 122 - Domino Tiles + Mahjong Tiles
}

// GetUnicodeRangeFor returns the index of the Unicode range for the given value.
// If lastPosition is provided (>= 0), it checks that range first for performance.
// Returns -1 if the value is not in any defined range.
func GetUnicodeRangeFor(value rune, lastPosition int) int {
	// Check last position first if provided (optimization hint)
	if lastPosition >= 0 && lastPosition < len(UnicodeRanges) {
		if isInRange(value, UnicodeRanges[lastPosition]) {
			return lastPosition
		}
	}

	// Linear search through all ranges
	for i, rang := range UnicodeRanges {
		if isInRange(value, rang) {
			return i
		}
	}

	return -1
}

// isInRange checks if a value is within any of the sub-ranges in a UnicodeRange.
// Ranges are stored as pairs of [start, end] values.
func isInRange(value rune, rang UnicodeRange) bool {
	for i := 0; i < len(rang); i += 2 {
		if value >= rang[i] && value <= rang[i+1] {
			return true
		}
	}
	return false
}
