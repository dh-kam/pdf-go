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
		obj := p.buf1
		start := p.buf1Start
		end := p.buf1End
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
		num, err := parseInteger(token.Value)
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
			gen, err := parseInteger(next1.Value)
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
		num, err := parseReal(token.Value)
		if err != nil {
			return nil, 0, 0, errors.Invalid("parse_real", err)
		}
		return entity.NewReal(num), startPos, p.lexer.Pos(), nil
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
	dict := entity.NewDictWithXRef(p.xref)

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

		// Lexer strips the '/' prefix from names, so we need to add it back
		key := entity.Name("/" + token.Value)

		// Parse value
		value, err := p.ParseObject()
		if err != nil {
			return nil, err
		}

		dict.Set(key, value)
	}

	return dict, nil
}

// parseArray parses a PDF array.
func (p *Parser) parseArray() (entity.Object, error) {
	var items []entity.Object

	for {
		// Check for buffered value first
		if p.buf1 != nil {
			// We have a buffered value from previous parsing
			// Add it to the array and clear the buffer
			items = append(items, p.buf1)
			p.buf1 = nil
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

	objNum, err := parseInteger(token.Value)
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

	genNum, err := parseInteger(token.Value)
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
