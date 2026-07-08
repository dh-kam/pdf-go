// Package parser provides PDF object parsing functionality.
package parser

import (
	"fmt"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
)

// Parser parses PDF objects from a token stream.
type Parser struct {
	lexer     *Lexer
	xref      entity.XRef // For resolving indirect references
	buf1      entity.Object
	buf1Start int
	buf1End   int
	buf2      entity.Object
	buf3      entity.Object
	realCount int
	realCache map[float64]*entity.Real
}

// NewParser creates a new PDF parser.
func NewParser(lexer *Lexer, xref entity.XRef) *Parser {
	return &Parser{
		lexer: lexer,
		xref:  xref,
	}
}

// HasBufferedObject reports whether ParseObject has a buffered object that
// should be returned before reading additional lexer tokens.
func (p *Parser) HasBufferedObject() bool {
	return p.buf1 != nil
}

// ParseObject parses a single PDF object.
func (p *Parser) ParseObject() (entity.Object, error) {
	obj, _, _, err := p.ParseObjectWithSpan()
	return obj, err
}

// ParseObjectWithSpan parses a single PDF object and returns its byte span
// in the original lexer buffer when available.
func (p *Parser) ParseObjectWithSpan() (entity.Object, int, int, error) {
	if p.buf1 != nil {
		start := p.buf1Start
		end := p.buf1End
		if num, ok := p.buf1.(*entity.Integer); ok {
			next1, err1 := p.lexer.Peek()
			if err1 == nil && next1.Type == TokenNumber {
				if _, err := p.lexer.NextToken(); err != nil {
					return nil, 0, 0, err
				}
				gen, err := tokenIntegerValue(next1)
				if err != nil {
					return nil, 0, 0, errors.Invalid("parse_number", err)
				}
				genEnd := p.lexer.Pos()
				next2, err2 := p.lexer.Peek()
				if err2 == nil && next2.Type == TokenKeyword && next2.Value == "R" {
					if _, err := p.lexer.NextToken(); err != nil {
						return nil, 0, 0, err
					}
					p.buf1 = nil
					p.buf1Start = 0
					p.buf1End = 0
					p.buf2 = nil
					p.buf3 = nil
					return entity.NewRef(uint32(num.Value()), uint16(gen)), start, p.lexer.Pos(), nil
				}
				p.buf1 = entity.NewInteger(gen)
				p.buf1Start = next1.Pos
				p.buf1End = genEnd
				return num, start, end, nil
			}
		}
		obj := p.buf1
		p.buf1 = nil
		p.buf1Start = 0
		p.buf1End = 0
		return obj, start, end, nil
	}

	token, err := p.lexer.NextToken()
	if err != nil {
		return nil, 0, 0, err
	}
	startPos := token.Pos

	// Handle buffered tokens
	if token.Type == TokenKeyword {
		// Check for indirect reference: num gen R
		// buf2 should hold the first number (object number)
		// buf1 should hold the second number (generation number)
		// current token should be "R"
		if p.buf2 != nil && p.buf1 != nil {
			if num, ok := p.buf2.(*entity.Integer); ok {
				if gen, ok := p.buf1.(*entity.Integer); ok {
					if token.Value == "R" {
						// Clear buffer
						result := entity.NewRef(uint32(num.Value()), uint16(gen.Value()))
						p.buf1 = nil
						p.buf1Start = 0
						p.buf1End = 0
						p.buf2 = nil
						p.buf3 = nil
						return result, startPos, p.lexer.Pos(), nil
					}
				}
			}
			// Not an indirect reference, fall through to regular keyword handling
			// The buffered values will be handled by the caller
		}

		// Check for boolean keywords
		if token.Value == "true" {
			return entity.NewBoolean(true), startPos, p.lexer.Pos(), nil
		}
		if token.Value == "false" {
			return entity.NewBoolean(false), startPos, p.lexer.Pos(), nil
		}
		if token.Value == "null" {
			return entity.NewNull(), startPos, p.lexer.Pos(), nil
		}

		// Regular keyword/name
		return entity.Name(token.Value), startPos, p.lexer.Pos(), nil
	}

	if token.Type == TokenNumber {
		num, err := tokenIntegerValue(token)
		if err != nil {
			return nil, 0, 0, errors.Invalid("parse_number", err)
		}
		numberEnd := p.lexer.Pos()

		// Check if this is the start of an indirect reference (num gen R)
		// We need to peek at the next TWO tokens before consuming anything
		next1, err1 := p.lexer.Peek()
		if err1 == nil && next1.Type == TokenNumber {
			// Second token is a number, now check the third token
			// We need to consume next1 to see what comes after it
			if _, err := p.lexer.NextToken(); err != nil { // consume the generation number
				return nil, 0, 0, err
			}
			gen, err := tokenIntegerValue(next1)
			if err != nil {
				return nil, 0, 0, errors.Invalid("parse_number", err)
			}
			genEnd := p.lexer.Pos()

			// Now peek at the third token
			next2, err2 := p.lexer.Peek()
			if err2 == nil && next2.Type == TokenKeyword && next2.Value == "R" {
				// It IS an indirect reference - consume "R" and return
				if _, err := p.lexer.NextToken(); err != nil { // consume "R"
					return nil, 0, 0, err
				}
				return entity.NewRef(uint32(num), uint16(gen)), startPos, p.lexer.Pos(), nil
			}

			// NOT an indirect reference - we need to "put back" the gen number
			// Since we can't unread tokens, we buffer it
			p.buf1 = entity.NewInteger(gen)
			p.buf1Start = next1.Pos
			p.buf1End = genEnd
		}

		// Just a regular number
		return entity.NewInteger(num), startPos, numberEnd, nil
	}

	if token.Type == TokenReal {
		num, err := tokenRealValue(token)
		if err != nil {
			return nil, 0, 0, errors.Invalid("parse_real", err)
		}
		return p.newReal(num), startPos, p.lexer.Pos(), nil
	}

	if token.Type == TokenString {
		return entity.NewString(token.Value), startPos, p.lexer.Pos(), nil
	}

	if token.Type == TokenHexString {
		// Decode hex string
		decoded, err := decodeHexString(token.Value)
		if err != nil {
			return nil, 0, 0, errors.Invalid("decode_hex_string", err)
		}
		return entity.NewHexString(decoded), startPos, p.lexer.Pos(), nil
	}

	if token.Type == TokenDictStart {
		obj, err := p.parseDict()
		return obj, startPos, p.lexer.Pos(), err
	}

	if token.Type == TokenArrayStart {
		obj, err := p.parseArray()
		return obj, startPos, p.lexer.Pos(), err
	}

	return nil, 0, 0, errors.Invalidf("parse_object", "unexpected token type: %s", token.Type)
}

// parseDict parses a PDF dictionary.
func (p *Parser) parseDict() (entity.Object, error) {
	dict := entity.NewDictWithXRefCapacity(p.xref, p.estimateDictCapacity())

	for {
		// Check for buffered value first
		if p.buf1 != nil {
			// We have a buffered value from previous parsing
			// This is an error state for dictionary key parsing
			// Keys are always names (keywords), not numbers
			return nil, errors.Invalidf("parse_dict", "buffered value when expecting dict key: %v", p.buf1)
		}

		token, err := p.lexer.NextToken()
		if err != nil {
			return nil, err
		}

		if token.Type == TokenDictEnd {
			break
		}

		// Key should be a name
		if token.Type != TokenKeyword {
			return nil, errors.Invalidf("parse_dict", "expected name key, got %s", token.Type)
		}

		// Lexer strips the '/' prefix from names, so add it back for dictionary keys.
		key := internPDFDictKey(token.Value)

		// Parse value
		value, err := p.ParseObject()
		if err != nil {
			return nil, err
		}

		dict.Set(key, value)
	}

	return dict, nil
}

func (p *Parser) estimateDictCapacity() int {
	const defaultDictCapacity = 4
	if p == nil || p.lexer == nil || !p.lexer.bytesMode || p.lexer.pos >= len(p.lexer.data) {
		return defaultDictCapacity
	}
	data := p.lexer.data[p.lexer.pos:]
	if len(data) > 512 {
		data = data[:512]
	}
	nameCount := 0
	depth := 0
	for i := 0; i < len(data); i++ {
		switch data[i] {
		case '%':
			for i+1 < len(data) && data[i+1] != '\n' && data[i+1] != '\r' {
				i++
			}
		case '(':
			i = skipLiteralStringEstimate(data, i)
		case '<':
			if i+1 < len(data) && data[i+1] == '<' {
				depth++
				i++
				continue
			}
			i = skipHexStringEstimate(data, i)
		case '>':
			if i+1 < len(data) && data[i+1] == '>' {
				if depth == 0 {
					if nameCount >= 6 {
						return 8
					}
					return defaultDictCapacity
				}
				depth--
				i++
			}
		case '/':
			if depth == 0 {
				nameCount++
				if nameCount >= 6 {
					return 8
				}
			}
		}
	}
	if nameCount >= 6 {
		return 8
	}
	return defaultDictCapacity
}

func skipLiteralStringEstimate(data []byte, start int) int {
	depth := 1
	for i := start + 1; i < len(data); i++ {
		switch data[i] {
		case '\\':
			if i+1 < len(data) {
				i++
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(data) - 1
}

func skipHexStringEstimate(data []byte, start int) int {
	for i := start + 1; i < len(data); i++ {
		if data[i] == '>' {
			return i
		}
	}
	return len(data) - 1
}

func internPDFDictKey(name string) entity.Name {
	switch name {
	case "A":
		return entity.Name("/A")
	case "AIS":
		return entity.Name("/AIS")
	case "Ascent":
		return entity.Name("/Ascent")
	case "BBox":
		return entity.Name("/BBox")
	case "BM":
		return entity.Name("/BM")
	case "BPC":
		return entity.Name("/BPC")
	case "BaseEncoding":
		return entity.Name("/BaseEncoding")
	case "BaseFont":
		return entity.Name("/BaseFont")
	case "BitsPerComponent":
		return entity.Name("/BitsPerComponent")
	case "Border":
		return entity.Name("/Border")
	case "CA":
		return entity.Name("/CA")
	case "CIDSet":
		return entity.Name("/CIDSet")
	case "CIDFontType0":
		return entity.Name("/CIDFontType0")
	case "CIDFontType2":
		return entity.Name("/CIDFontType2")
	case "CIDSystemInfo":
		return entity.Name("/CIDSystemInfo")
	case "CIDToGIDMap":
		return entity.Name("/CIDToGIDMap")
	case "CMapName":
		return entity.Name("/CMapName")
	case "CS":
		return entity.Name("/CS")
	case "CapHeight":
		return entity.Name("/CapHeight")
	case "CharProcs":
		return entity.Name("/CharProcs")
	case "ColorSpace":
		return entity.Name("/ColorSpace")
	case "Contents":
		return entity.Name("/Contents")
	case "Count":
		return entity.Name("/Count")
	case "CropBox":
		return entity.Name("/CropBox")
	case "D":
		return entity.Name("/D")
	case "DCTDecode":
		return entity.Name("/DCTDecode")
	case "DP":
		return entity.Name("/DP")
	case "DW":
		return entity.Name("/DW")
	case "DW2":
		return entity.Name("/DW2")
	case "Decode":
		return entity.Name("/Decode")
	case "DecodeParms":
		return entity.Name("/DecodeParms")
	case "DescendantFonts":
		return entity.Name("/DescendantFonts")
	case "DeviceCMYK":
		return entity.Name("/DeviceCMYK")
	case "DeviceGray":
		return entity.Name("/DeviceGray")
	case "DeviceRGB":
		return entity.Name("/DeviceRGB")
	case "Differences":
		return entity.Name("/Differences")
	case "Domain":
		return entity.Name("/Domain")
	case "Encoding":
		return entity.Name("/Encoding")
	case "ExtGState":
		return entity.Name("/ExtGState")
	case "F":
		return entity.Name("/F")
	case "Filter":
		return entity.Name("/Filter")
	case "FirstChar":
		return entity.Name("/FirstChar")
	case "Flags":
		return entity.Name("/Flags")
	case "FlateDecode":
		return entity.Name("/FlateDecode")
	case "Font":
		return entity.Name("/Font")
	case "FontBBox":
		return entity.Name("/FontBBox")
	case "FontDescriptor":
		return entity.Name("/FontDescriptor")
	case "FontFile":
		return entity.Name("/FontFile")
	case "FontFile2":
		return entity.Name("/FontFile2")
	case "FontFile3":
		return entity.Name("/FontFile3")
	case "FontMatrix":
		return entity.Name("/FontMatrix")
	case "Form":
		return entity.Name("/Form")
	case "Function":
		return entity.Name("/Function")
	case "FunctionType":
		return entity.Name("/FunctionType")
	case "Group":
		return entity.Name("/Group")
	case "H":
		return entity.Name("/H")
	case "Height":
		return entity.Name("/Height")
	case "I":
		return entity.Name("/I")
	case "ID":
		return entity.Name("/ID")
	case "ICCBased":
		return entity.Name("/ICCBased")
	case "IM":
		return entity.Name("/IM")
	case "Image":
		return entity.Name("/Image")
	case "ImageMask":
		return entity.Name("/ImageMask")
	case "Index":
		return entity.Name("/Index")
	case "Indexed":
		return entity.Name("/Indexed")
	case "Interpolate":
		return entity.Name("/Interpolate")
	case "ItalicAngle":
		return entity.Name("/ItalicAngle")
	case "K":
		return entity.Name("/K")
	case "Kids":
		return entity.Name("/Kids")
	case "LC":
		return entity.Name("/LC")
	case "LJ":
		return entity.Name("/LJ")
	case "LW":
		return entity.Name("/LW")
	case "LastChar":
		return entity.Name("/LastChar")
	case "Length":
		return entity.Name("/Length")
	case "Length1":
		return entity.Name("/Length1")
	case "Length2":
		return entity.Name("/Length2")
	case "Length3":
		return entity.Name("/Length3")
	case "ML":
		return entity.Name("/ML")
	case "Mask":
		return entity.Name("/Mask")
	case "Matte":
		return entity.Name("/Matte")
	case "Matrix":
		return entity.Name("/Matrix")
	case "MediaBox":
		return entity.Name("/MediaBox")
	case "N":
		return entity.Name("/N")
	case "Name":
		return entity.Name("/Name")
	case "OP":
		return entity.Name("/OP")
	case "OPM":
		return entity.Name("/OPM")
	case "Ordering":
		return entity.Name("/Ordering")
	case "Page":
		return entity.Name("/Page")
	case "Pages":
		return entity.Name("/Pages")
	case "PaintType":
		return entity.Name("/PaintType")
	case "Parent":
		return entity.Name("/Parent")
	case "Pattern":
		return entity.Name("/Pattern")
	case "PatternType":
		return entity.Name("/PatternType")
	case "ProcSet":
		return entity.Name("/ProcSet")
	case "Range":
		return entity.Name("/Range")
	case "Registry":
		return entity.Name("/Registry")
	case "Resources":
		return entity.Name("/Resources")
	case "Rect":
		return entity.Name("/Rect")
	case "Root":
		return entity.Name("/Root")
	case "S":
		return entity.Name("/S")
	case "SA":
		return entity.Name("/SA")
	case "SMask":
		return entity.Name("/SMask")
	case "Shading":
		return entity.Name("/Shading")
	case "ShadingType":
		return entity.Name("/ShadingType")
	case "Size":
		return entity.Name("/Size")
	case "StemV":
		return entity.Name("/StemV")
	case "Subtype":
		return entity.Name("/Subtype")
	case "Supplement":
		return entity.Name("/Supplement")
	case "TilingType":
		return entity.Name("/TilingType")
	case "ToUnicode":
		return entity.Name("/ToUnicode")
	case "Title":
		return entity.Name("/Title")
	case "Type0":
		return entity.Name("/Type0")
	case "Type1":
		return entity.Name("/Type1")
	case "Type3":
		return entity.Name("/Type3")
	case "TrueType":
		return entity.Name("/TrueType")
	case "Type":
		return entity.Name("/Type")
	case "W":
		return entity.Name("/W")
	case "W2":
		return entity.Name("/W2")
	case "WMode":
		return entity.Name("/WMode")
	case "Width":
		return entity.Name("/Width")
	case "Widths":
		return entity.Name("/Widths")
	case "XObject":
		return entity.Name("/XObject")
	case "XHeight":
		return entity.Name("/XHeight")
	case "XStep":
		return entity.Name("/XStep")
	case "YStep":
		return entity.Name("/YStep")
	case "ca":
		return entity.Name("/ca")
	}
	return entity.Name("/" + name)
}

// parseArray parses a PDF array.
func (p *Parser) parseArray() (entity.Object, error) {
	items := make([]entity.Object, 0, 4)

	for {
		// Check for buffered value first
		if p.buf1 != nil {
			// A buffered number may still be the object number in "obj gen R".
			// Route it back through ParseObject so indirect references after a
			// preceding integer are reconstructed instead of split into items.
			obj, err := p.ParseObject()
			if err != nil {
				return nil, err
			}
			items = append(items, obj)
			continue
		}

		token, err := p.lexer.Peek()
		if err != nil {
			return nil, err
		}

		if token.Type == TokenArrayEnd {
			if _, err := p.lexer.NextToken(); err != nil { // consume ']'
				return nil, err
			}
			break
		}

		obj, err := p.ParseObject()
		if err != nil {
			return nil, err
		}

		items = append(items, obj)
	}

	return entity.NewArray(items...), nil
}

func tokenIntegerValue(token Token) (int64, error) {
	if token.HasNumberValue && token.Type == TokenNumber {
		return token.IntValue, nil
	}
	return parseInteger(token.Value)
}

func tokenRealValue(token Token) (float64, error) {
	if token.HasNumberValue && token.Type == TokenReal {
		return token.RealValue, nil
	}
	return parseReal(token.Value)
}

func (p *Parser) newReal(value float64) *entity.Real {
	const (
		realCacheMinStreamBytes = 128 << 10
		realCacheMinTokens      = 1024
		realCacheMaxEntries     = 8192
	)
	if p == nil || p.lexer == nil || !p.lexer.omitNumberValue {
		return entity.NewReal(value)
	}
	if len(p.lexer.data) < realCacheMinStreamBytes {
		return entity.NewReal(value)
	}
	if p.realCache != nil {
		if cached, ok := p.realCache[value]; ok {
			return cached
		}
		real := entity.NewReal(value)
		if len(p.realCache) < realCacheMaxEntries {
			p.realCache[value] = real
		}
		return real
	}
	p.realCount++
	real := entity.NewReal(value)
	if p.realCount >= realCacheMinTokens {
		p.realCache = map[float64]*entity.Real{value: real}
	}
	return real
}

// parseInteger parses an integer from a string.
func parseInteger(s string) (int64, error) {
	var result int64
	var sign int64 = 1

	i := 0
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		if s[0] == '-' {
			sign = -1
		}
		i++
	}

	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("invalid integer: %s", s)
		}
		result = result*10 + int64(s[i]-'0')
	}

	return sign * result, nil
}

// parseReal parses a real number from a string.
func parseReal(s string) (float64, error) {
	var result float64
	var sign float64 = 1
	var fraction = 0.1
	var inFraction bool

	i := 0
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		if s[0] == '-' {
			sign = -1
		}
		i++
	}

	// Parse integer part
	for ; i < len(s); i++ {
		if s[i] == '.' {
			inFraction = true
			i++
			break
		}
		if s[i] < '0' || s[i] > '9' {
			return 0, fmt.Errorf("invalid real: %s", s)
		}
		result = result*10 + float64(s[i]-'0')
	}

	// Parse fraction part
	if inFraction {
		for ; i < len(s); i++ {
			if s[i] < '0' || s[i] > '9' {
				return 0, fmt.Errorf("invalid real: %s", s)
			}
			result += float64(s[i]-'0') * fraction
			fraction *= 0.1
		}
	}

	return sign * result, nil
}

// decodeHexString decodes a hex-encoded string.
func decodeHexString(s string) (string, error) {
	var result []byte

	// Pad to even length
	if len(s)%2 != 0 {
		s += "0"
	}

	for i := 0; i < len(s); i += 2 {
		high := decodeHexDigit(s[i])
		low := decodeHexDigit(s[i+1])
		if high == 16 || low == 16 {
			return "", fmt.Errorf("invalid hex digit at position %d", i)
		}
		result = append(result, high<<4|low)
	}

	return string(result), nil
}

// decodeHexDigit decodes a single hex digit.
func decodeHexDigit(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	default:
		return 16 // invalid
	}
}

// ParseIndirectReference parses an indirect reference (obj_num gen R).
func (p *Parser) ParseIndirectReference() (entity.Ref, error) {
	token, err := p.lexer.NextToken()
	if err != nil {
		return entity.Ref{}, err
	}

	if token.Type != TokenNumber {
		return entity.Ref{}, errors.Invalidf("parse_ref", "expected object number, got %s", token.Type)
	}

	objNum, err := tokenIntegerValue(token)
	if err != nil {
		return entity.Ref{}, errors.Invalid("parse_ref", err)
	}

	token, err = p.lexer.NextToken()
	if err != nil {
		return entity.Ref{}, err
	}

	if token.Type != TokenNumber {
		return entity.Ref{}, errors.Invalidf("parse_ref", "expected generation number, got %s", token.Type)
	}

	genNum, err := tokenIntegerValue(token)
	if err != nil {
		return entity.Ref{}, errors.Invalid("parse_ref", err)
	}

	token, err = p.lexer.NextToken()
	if err != nil {
		return entity.Ref{}, err
	}

	if token.Type != TokenKeyword || token.Value != "R" {
		return entity.Ref{}, errors.Invalidf("parse_ref", "expected 'R', got %s", token.Value)
	}

	return entity.NewRef(uint32(objNum), uint16(genNum)), nil
}
