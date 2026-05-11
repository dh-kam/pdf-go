package unicode

import (
	"strconv"
	"strings"
)

// GetUnicodeForGlyph returns the Unicode code point for a glyph name.
// It tries to recover valid Unicode values from 'uniXXXX'/'uXXXX{XX}' glyphs.
// Returns -1 if the Unicode value cannot be determined.
//
// Parameters:
//   - name: The glyph name (e.g., "uniXXXX", "uXXXX")
//   - glyphsUnicodeMap: Optional mapping of glyph names to Unicode values
//
// Returns the Unicode code point, or -1 if not found.
func GetUnicodeForGlyph(name string, glyphsUnicodeMap map[string]rune) rune {
	// Check the provided glyph map first
	if glyphsUnicodeMap != nil {
		if unicode, ok := glyphsUnicodeMap[name]; ok {
			return unicode
		}
	}

	if name == "" {
		return -1
	}

	// Try to recover valid Unicode values from 'uniXXXX'/'uXXXX{XX}' glyphs
	if name[0] != 'u' {
		return -1
	}

	nameLen := len(name)
	var hexStr string

	switch {
	case nameLen == 7 && name[1] == 'n' && name[2] == 'i':
		// 'uniXXXX' format (7 characters)
		hexStr = name[3:]
	case nameLen >= 5 && nameLen <= 7:
		// 'uXXXX' or 'uXXXXXX' format (5-7 characters)
		hexStr = name[1:]
	default:
		return -1
	}

	// Check for upper-case hexadecimal characters to avoid false positives
	if hexStr != strings.ToUpper(hexStr) {
		return -1
	}

	// Parse hexadecimal string
	unicode, err := strconv.ParseInt(hexStr, 16, 32)
	if err != nil || unicode < 0 {
		return -1
	}

	return rune(unicode)
}
