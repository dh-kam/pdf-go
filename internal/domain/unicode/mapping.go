package unicode

// MapSpecialUnicodeValues maps special unicode values to their equivalents.
// Some characters, e.g. copyrightserif, are mapped to the private use area
// and might not be displayed using standard fonts. This function maps well-known
// chars to similar equivalents in the normal characters range.
func MapSpecialUnicodeValues(code rune) rune {
	// Specials unicode block - return 0 to filter out
	if code >= 0xFFF0 && code <= 0xFFFF {
		return 0
	}

	// Private Use Area (PUA) - map to standard equivalents
	if code >= 0xF600 && code <= 0xF8FF {
		if mapped, ok := specialPUASymbols[code]; ok {
			return mapped
		}
		return code
	}

	// Soft hyphen → hyphen
	if code == 0x00AD {
		return 0x002D
	}

	return code
}

// specialPUASymbols maps Private Use Area characters to standard Unicode equivalents.
// Based on PDF.js getSpecialPUASymbols lookup table.
var specialPUASymbols = map[rune]rune{
	// Copyright symbols
	0xF8E9: 0x00A9, // copyrightsans (63721) => copyright
	0xF6D9: 0x00A9, // copyrightserif (63193) => copyright

	// Register symbols
	0xF8E8: 0x00AE, // registersans (63720) => registered
	0xF6DA: 0x00AE, // registerserif (63194) => registered

	// Trademark symbols
	0xF8EA: 0x2122, // trademarksans (63722) => trademark
	0xF6DB: 0x2122, // trademarkserif (63195) => trademark

	// Brace characters (left)
	0xF8F1: 0x23A7, // bracelefttp (63729)
	0xF8F2: 0x23A8, // braceleftmid (63730)
	0xF8F3: 0x23A9, // braceleftbt (63731)

	// Brace characters (right)
	0xF8FC: 0x23AB, // bracerighttp (63740)
	0xF8FD: 0x23AC, // bracerightmid (63741)
	0xF8FE: 0x23AD, // bracerightbt (63742)

	// Bracket characters (left)
	0xF8EE: 0x23A1, // bracketlefttp (63726)
	0xF8EF: 0x23A2, // bracketleftex (63727)
	0xF8F0: 0x23A3, // bracketleftbt (63728)

	// Bracket characters (right)
	0xF8F9: 0x23A4, // bracketrighttp (63737)
	0xF8FA: 0x23A5, // bracketrightex (63738)
	0xF8FB: 0x23A6, // bracketrightbt (63739)

	// Parenthesis characters (left)
	0xF8EB: 0x239B, // parenlefttp (63723)
	0xF8EC: 0x239C, // parenleftex (63724)
	0xF8ED: 0x239D, // parenleftbt (63725)

	// Parenthesis characters (right)
	0xF8F6: 0x239E, // parenrighttp (63734)
	0xF8F7: 0x239F, // parenrightex (63735)
	0xF8F8: 0x23A0, // parenrightbt (63736)
}
