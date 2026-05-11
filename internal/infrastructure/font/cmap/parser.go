// Package cmap provides CMap parsing functionality for CJK fonts.
//
//revive:disable:exported
//nolint:errcheck,gocritic,unused
package cmap

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
)

// Parser parses CMap files (text and binary formats).
type Parser struct {
	lexer     *cmapLexer
	bufferTok Token
	buffered  bool
}

// NewParser creates a new CMap parser.
func NewParser(r io.Reader) *Parser {
	return &Parser{
		lexer: newCMapLexer(r),
	}
}

// nextToken returns the next token, with buffering support.
func (p *Parser) nextToken() (Token, error) {
	if p.buffered {
		p.buffered = false
		return p.bufferTok, nil
	}
	return p.lexer.NextToken()
}

// peekToken returns the next token without consuming it.
func (p *Parser) peekToken() (Token, error) {
	if !p.buffered {
		tok, err := p.nextToken()
		if err != nil {
			return tok, err
		}
		p.bufferTok = tok
		p.buffered = true
	}
	return p.bufferTok, nil
}

// ungetToken puts a token back into the buffer.
func (p *Parser) ungetToken(tok Token) {
	p.bufferTok = tok
	p.buffered = true
}

// Parse parses a CMap file and returns a CMap.
func (p *Parser) Parse() (entity.CMap, error) {
	cmap := &BaseCMap{
		name:       "Unknown",
		cidMapping: make(map[uint32]uint32),
		uniMapping: make(map[uint32]string),
	}

	for {
		tok, err := p.nextToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.Invalid("cmap_parse", err)
		}

		// Handle both TokenKeyword and TokenName for CMap keywords
		var keyword string
		if tok.Type == TokenKeyword {
			keyword = tok.Value
		} else if tok.Type == TokenName {
			// Check if this name is a CMap keyword
			if p.isCMapKeyword(tok.Value) {
				keyword = tok.Value
			} else {
				// Not a keyword, skip this token
				continue
			}
		} else {
			// Not a keyword token, continue
			continue
		}

		if err := p.parseKeyword(keyword, cmap); err != nil {
			return nil, err
		}
	}

	return cmap, nil
}

// isCMapKeyword checks if a name is a CMap keyword.
func (p *Parser) isCMapKeyword(name string) bool {
	keywords := map[string]bool{
		"CMapName":       true,
		"CMapType":       true,
		"WMode":          true,
		"CIDSystemInfo":  true,
		"CodeSpaceRange": true,
		"CIDRange":       true,
		"CIDChar":        true,
		"NotDefRange":    true,
		"NotDefChar":     true,
		"UseCMap":        true,
		"def":            true,
	}
	return keywords[name]
}

func (p *Parser) parseKeyword(keyword string, cmap *BaseCMap) error {
	switch keyword {
	case "CMapName":
		tok, err := p.nextToken()
		if err != nil {
			return err
		}
		if tok.Type == TokenName {
			cmap.name = tok.Value
		}
		// Skip "def"
		p.skipDef()

	case "CMapType":
		// Skip next token (type number)
		_, err := p.nextToken()
		p.skipDef()
		return err

	case "WMode":
		tok, err := p.nextToken()
		if err != nil {
			return err
		}
		if tok.Type == TokenInteger {
			if tok.Value == "1" {
				cmap.writingMode = entity.WritingModeVertical
			}
		}
		p.skipDef()

	case "CIDSystemInfo":
		return p.parseCIDSystemInfo(cmap)

	case "CodeSpaceRange":
		return p.parseCodeSpaceRange(cmap)

	case "CIDRange":
		return p.parseCIDRange(cmap)

	case "CIDChar":
		return p.parseCIDChar(cmap)

	case "NotDefRange":
		return p.parseNotDefRange(cmap)

	case "NotDefChar":
		return p.parseNotDefChar(cmap)

	case "UseCMap":
		// For now, skip UseCMap directives
		p.skipDef()
		return nil

	case "def":
		// Skip standalone "def"
		return nil
	}

	return nil
}

// skipDef skips a "def" keyword if present.
func (p *Parser) skipDef() error {
	tok, err := p.peekToken()
	if err != nil {
		return err
	}
	if tok.Type == TokenKeyword && tok.Value == "def" {
		// Consume the "def" token
		_, _ = p.nextToken()
	}
	// Otherwise, leave the token in the buffer for next read
	return nil
}

func (p *Parser) parseCIDSystemInfo(cmap *BaseCMap) error {
	// Expect dict begin
	tok, err := p.nextToken()
	if err != nil || tok.Type != TokenDictBegin {
		return nil // Skip if not a dict
	}

	// Parse dict contents until dict end
	depth := 1
	for depth > 0 {
		tok, err := p.nextToken()
		if err != nil {
			return err
		}

		switch tok.Type {
		case TokenDictBegin:
			depth++
		case TokenDictEnd:
			depth--
		case TokenKeyword:
			if tok.Value == "Registry" || tok.Value == "Ordering" {
				nextTok, _ := p.nextToken()
				if nextTok.Type == TokenString {
					if tok.Value == "Registry" {
						cmap.systemInfo.Registry = nextTok.Value
					} else {
						cmap.systemInfo.Ordering = nextTok.Value
					}
				}
			} else if tok.Value == "Supplement" {
				nextTok, _ := p.nextToken()
				if nextTok.Type == TokenInteger {
					val, _ := strconv.Atoi(nextTok.Value)
					cmap.systemInfo.Supplement = val
				}
			}
		}
	}

	// Skip "def"
	p.skipDef()

	return nil
}

func (p *Parser) parseCodeSpaceRange(cmap *BaseCMap) error {
	// Expect array begin
	tok, err := p.nextToken()
	if err != nil || tok.Type != TokenArrayBegin {
		return nil
	}

	for {
		tok, err := p.nextToken()
		if err != nil {
			return err
		}

		if tok.Type == TokenArrayEnd {
			break
		}

		if tok.Type == TokenHexString {
			low := parseHex(tok.Value)
			nextTok, err := p.nextToken()
			if err != nil {
				return err
			}
			if nextTok.Type == TokenHexString {
				high := parseHex(nextTok.Value)
				cmap.codeSpaceRanges = append(cmap.codeSpaceRanges, entity.CMapRange{
					Low:  low,
					High: high,
				})
			}
		}
	}

	// Skip "def"
	p.skipDef()

	return nil
}

func (p *Parser) parseCIDRange(cmap *BaseCMap) error {
	// Check if this is array format or single-range format
	tok, err := p.nextToken()
	if err != nil {
		return err
	}

	if tok.Type == TokenArrayBegin {
		// Array format: /CIDRange [<00> <20> 1] def
		for {
			tok, err := p.nextToken()
			if err != nil {
				return err
			}

			if tok.Type == TokenArrayEnd {
				break
			}

			if tok.Type == TokenHexString {
				codeLow := parseHex(tok.Value)

				nextTok, err := p.nextToken()
				if err != nil {
					return err
				}

				if nextTok.Type == TokenHexString {
					codeHigh := parseHex(nextTok.Value)

					nextTok, err := p.nextToken()
					if err != nil {
						return err
					}

					if nextTok.Type == TokenInteger {
						cidStart, _ := strconv.ParseUint(nextTok.Value, 10, 32)
						cid := uint32(cidStart)

						// Map range of codes to consecutive CIDs
						for code := codeLow; code <= codeHigh; code++ {
							cmap.cidMapping[code] = cid
							cid++
						}
					}
				}
			}
		}
	} else {
		// Single-range format: /CIDRange <10> <15> 100 def
		// tok is already the first hex string (codeLow)
		if tok.Type == TokenHexString {
			codeLow := parseHex(tok.Value)

			nextTok, err := p.nextToken()
			if err != nil {
				return err
			}

			if nextTok.Type == TokenHexString {
				codeHigh := parseHex(nextTok.Value)

				nextTok, err := p.nextToken()
				if err != nil {
					return err
				}

				if nextTok.Type == TokenInteger {
					cidStart, _ := strconv.ParseUint(nextTok.Value, 10, 32)
					cid := uint32(cidStart)

					// Map range of codes to consecutive CIDs
					for code := codeLow; code <= codeHigh; code++ {
						cmap.cidMapping[code] = cid
						cid++
					}
				}
			}
		}
	}

	// Skip "def"
	p.skipDef()

	cmap.isCIDBased = true
	return nil
}

func (p *Parser) parseCIDChar(cmap *BaseCMap) error {
	// Check if this is array format or single-character format
	tok, err := p.nextToken()
	if err != nil {
		return err
	}

	if tok.Type == TokenArrayBegin {
		// Array format: /CIDChar [<41> 100 <42> 200] def
		for {
			tok, err := p.nextToken()
			if err != nil {
				return err
			}

			if tok.Type == TokenArrayEnd {
				break
			}

			if tok.Type == TokenHexString {
				code := parseHex(tok.Value)

				nextTok, err := p.nextToken()
				if err != nil {
					return err
				}

				if nextTok.Type == TokenInteger {
					cid, _ := strconv.ParseUint(nextTok.Value, 10, 32)
					cmap.cidMapping[code] = uint32(cid)
				}
			}
		}
	} else {
		// Single-character format: /CIDChar <41> 100 def
		// tok is already the hex string
		if tok.Type == TokenHexString {
			code := parseHex(tok.Value)

			nextTok, err := p.nextToken()
			if err != nil {
				return err
			}

			if nextTok.Type == TokenInteger {
				cid, _ := strconv.ParseUint(nextTok.Value, 10, 32)
				cmap.cidMapping[code] = uint32(cid)
			}
		}
	}

	// Skip "def"
	p.skipDef()

	cmap.isCIDBased = true
	return nil
}

func (p *Parser) parseNotDefRange(cmap *BaseCMap) error {
	// Similar to CIDRange but stores notdef values
	// For simplicity, skip for now
	return p.skipArray()
}

func (p *Parser) parseNotDefChar(cmap *BaseCMap) error {
	// Similar to CIDChar but stores notdef values
	// For simplicity, skip for now
	return p.skipArray()
}

func (p *Parser) skipArray() error {
	tok, err := p.nextToken()
	if err != nil || tok.Type != TokenArrayBegin {
		return nil
	}

	depth := 1
	for depth > 0 {
		tok, err := p.nextToken()
		if err != nil {
			return err
		}

		switch tok.Type {
		case TokenArrayBegin:
			depth++
		case TokenArrayEnd:
			depth--
		}
	}

	return nil
}

// parseHex parses a hexadecimal string.
func parseHex(s string) uint32 {
	s = strings.TrimPrefix(s, "<")
	s = strings.TrimSuffix(s, ">")
	var result uint32
	for _, c := range s {
		var v uint32
		if c >= '0' && c <= '9' {
			v = uint32(c - '0')
		} else if c >= 'A' && c <= 'F' {
			v = uint32(c - 'A' + 10)
		} else if c >= 'a' && c <= 'f' {
			v = uint32(c - 'a' + 10)
		}
		result = result*16 + v
	}
	return result
}

// TokenType represents token types for the CMap lexer.
type TokenType int

const (
	TokenKeyword TokenType = iota
	TokenName
	TokenString
	TokenHexString
	TokenInteger
	TokenArrayBegin
	TokenArrayEnd
	TokenDictBegin
	TokenDictEnd
)

// Token represents a CMap token.
type Token struct {
	Value string
	Type  TokenType
}

// cmapLexer tokenizes CMap files.
type cmapLexer struct {
	scanner *bufio.Scanner
	tokens  []Token
	pos     int
}

func newCMapLexer(r io.Reader) *cmapLexer {
	return &cmapLexer{
		scanner: bufio.NewScanner(r),
		tokens:  make([]Token, 0, 10),
		pos:     0,
	}
}

// NextToken returns the next token.
func (l *cmapLexer) NextToken() (Token, error) {
	// Return buffered tokens first
	if l.pos < len(l.tokens) {
		tok := l.tokens[l.pos]
		l.pos++
		return tok, nil
	}

	// Read next line and tokenize
	for l.scanner.Scan() {
		line := strings.TrimSpace(l.scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "%") {
			continue
		}

		// Handle multiline definitions
		for strings.HasSuffix(line, "\\") {
			line = strings.TrimSuffix(line, "\\")
			if l.scanner.Scan() {
				line += strings.TrimSpace(l.scanner.Text())
			}
		}

		// Tokenize the line
		l.tokens = l.tokenizeLine(line)
		l.pos = 0

		if len(l.tokens) > 0 {
			tok := l.tokens[l.pos]
			l.pos++
			return tok, nil
		}
	}

	if err := l.scanner.Err(); err != nil {
		return Token{}, err
	}

	return Token{}, io.EOF
}

func (l *cmapLexer) tokenizeLine(line string) []Token {
	tokens := make([]Token, 0)

	// First, check if the line contains an array
	arrayStart := strings.IndexByte(line, '[')
	arrayEnd := strings.IndexByte(line, ']')

	if arrayStart >= 0 && arrayEnd > arrayStart {
		// Process tokens before the array
		beforeArray := strings.TrimSpace(line[:arrayStart])
		if beforeArray != "" {
			beforeFields := strings.Fields(beforeArray)
			for _, field := range beforeFields {
				tok := l.tokenizeField(field)
				tokens = append(tokens, tok)
			}
		}

		// Add array begin token
		tokens = append(tokens, Token{Type: TokenArrayBegin, Value: "["})

		// Extract and tokenize array contents
		contents := strings.TrimSpace(line[arrayStart+1 : arrayEnd])
		if contents != "" {
			arrayFields := strings.Fields(contents)
			for _, field := range arrayFields {
				tok := l.tokenizeField(field)
				tokens = append(tokens, tok)
			}
		}

		// Add array end token
		tokens = append(tokens, Token{Type: TokenArrayEnd, Value: "]"})

		// Process tokens after the array
		afterArray := strings.TrimSpace(line[arrayEnd+1:])
		if afterArray != "" {
			afterFields := strings.Fields(afterArray)
			for _, field := range afterFields {
				tok := l.tokenizeField(field)
				tokens = append(tokens, tok)
			}
		}

		return tokens
	}

	// No array in line, just split by whitespace
	fields := strings.Fields(line)
	for _, field := range fields {
		tok := l.tokenizeField(field)
		tokens = append(tokens, tok)
	}

	return tokens
}

func (l *cmapLexer) tokenizeField(field string) Token {
	switch {
	case field == "begin":
		return Token{Type: TokenDictBegin, Value: field}
	case field == "end":
		return Token{Type: TokenDictEnd, Value: field}
	case field == "<<":
		return Token{Type: TokenDictBegin, Value: field}
	case field == ">>":
		return Token{Type: TokenDictEnd, Value: field}
	case field == "[":
		return Token{Type: TokenArrayBegin, Value: field}
	case field == "]":
		return Token{Type: TokenArrayEnd, Value: field}
	case strings.HasPrefix(field, "/"):
		return Token{Type: TokenName, Value: strings.TrimPrefix(field, "/")}
	case strings.HasPrefix(field, "("):
		// String literal - find closing paren
		end := strings.IndexByte(field[1:], ')')
		if end >= 0 {
			return Token{Type: TokenString, Value: field[1 : end+1]}
		}
		return Token{Type: TokenString, Value: field[1:]}
	case strings.HasPrefix(field, "<") && !strings.HasSuffix(field, ">"):
		// Incomplete hex string, skip for now
		return Token{Type: TokenKeyword, Value: field}
	case strings.HasPrefix(field, "<") && strings.HasSuffix(field, ">"):
		// Hex string
		return Token{Type: TokenHexString, Value: field}
	case isInteger(field):
		return Token{Type: TokenInteger, Value: field}
	default:
		return Token{Type: TokenKeyword, Value: field}
	}
}

func isInteger(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// BaseCMap is a base implementation of the CMap interface.
type BaseCMap struct {
	cidMapping      map[uint32]uint32
	uniMapping      map[uint32]string
	notdefMapping   map[uint32]uint32
	name            string
	systemInfo      entity.CIDSystemInfo
	codeSpaceRanges []entity.CMapRange
	cmapType        entity.CMapType
	writingMode     entity.WritingMode
	isCIDBased      bool
	isUnicodeBased  bool
}

// Name returns the CMap name.
func (m *BaseCMap) Name() string {
	return m.name
}

// LookupCID maps a character code to a CID.
func (m *BaseCMap) LookupCID(code uint32) (uint32, bool) {
	cid, ok := m.cidMapping[code]
	return cid, ok
}

// LookupUnicode maps a character code to a Unicode string.
func (m *BaseCMap) LookupUnicode(code uint32) (string, bool) {
	// First check direct Unicode mapping
	if uni, ok := m.uniMapping[code]; ok {
		return uni, true
	}

	// If CID-based, look up CID then convert to Unicode if available
	if m.isCIDBased {
		if cid, ok := m.cidMapping[code]; ok {
			// Simple CID to Unicode conversion for common CJK ranges
			// In production, this would use a proper CID-to-Unicode map
			return m.cidToUnicode(cid)
		}
	}

	return "", false
}

// IsCIDBased returns true if this CMap maps to CIDs.
func (m *BaseCMap) IsCIDBased() bool {
	return m.isCIDBased
}

// IsUnicode returns true if this CMap maps to Unicode.
func (m *BaseCMap) IsUnicode() bool {
	return m.isUnicodeBased
}

// cidToUnicode performs a simple CID to Unicode conversion.
func (m *BaseCMap) cidToUnicode(cid uint32) (string, bool) {
	// This is a simplified implementation.
	// In production, would use proper character collection data.

	// Common ranges for Simplified Chinese (GB1)
	if m.systemInfo.Ordering == "GB1" {
		if cid >= 0x21 && cid <= 0x7E {
			// ASCII compatible
			return string(rune(cid)), true
		}
		// For actual CJK characters, would need proper mapping
		return "", false
	}

	// Common ranges for Traditional Chinese (CNS1)
	if m.systemInfo.Ordering == "CNS1" {
		if cid >= 0x21 && cid <= 0x7E {
			return string(rune(cid)), true
		}
		return "", false
	}

	// Common ranges for Japanese (Japan1)
	if m.systemInfo.Ordering == "Japan1" {
		if cid >= 0x21 && cid <= 0x7E {
			return string(rune(cid)), true
		}
		// Simple mapping for common JIS X 0208 range
		if cid >= 1 && cid <= 94 {
			return "", false
		}
		return "", false
	}

	// Common ranges for Korean (Korea1)
	if m.systemInfo.Ordering == "Korea1" {
		if cid >= 0x21 && cid <= 0x7E {
			return string(rune(cid)), true
		}
		return "", false
	}

	return "", false
}

// SetUnicodeMapping sets a direct code to Unicode mapping.
func (m *BaseCMap) SetUnicodeMapping(code uint32, unicode string) {
	if m.uniMapping == nil {
		m.uniMapping = make(map[uint32]string)
	}
	m.uniMapping[code] = unicode
	m.isUnicodeBased = true
}

// SetCIDMapping sets a code to CID mapping.
func (m *BaseCMap) SetCIDMapping(code, cid uint32) {
	if m.cidMapping == nil {
		m.cidMapping = make(map[uint32]uint32)
	}
	m.cidMapping[code] = cid
	m.isCIDBased = true
}

// SystemInfo returns the CID system information.
func (m *BaseCMap) SystemInfo() entity.CIDSystemInfo {
	return m.systemInfo
}

// WritingMode returns the writing mode.
func (m *BaseCMap) WritingMode() entity.WritingMode {
	return m.writingMode
}

// ParseString parses a CMap from a string.
func ParseString(data string) (entity.CMap, error) {
	r := bytes.NewBufferString(data)
	return NewParser(r).Parse()
}

// ParseBytes parses a CMap from bytes.
func ParseBytes(data []byte) (entity.CMap, error) {
	r := bytes.NewReader(data)
	return NewParser(r).Parse()
}

// PredefinedCMap returns a predefined CMap by name.
func PredefinedCMap(name string) (entity.CMap, error) {
	// Return predefined CMaps
	// In production, these would be embedded as data
	data, ok := predefinedCMaps[name]
	if !ok {
		return nil, errors.NotFound("cmap_predefined", fmt.Errorf("cmap %s not found", name))
	}
	return ParseString(data)
}

// predefinedCMaps contains predefined CMap data.
var predefinedCMaps = map[string]string{
	// Simplified Chinese CMaps
	"GBK-EUC-H":  gbkEucH,
	"GBK2K-H":    gbk2KH,
	"GBpc-EUC-H": gbpcEucH,

	// Traditional Chinese CMaps
	"B5pc-H":      b5pcH,
	"B5pc-V":      b5pcV,
	"CNS-EUC-H":   cnsEucH,
	"CNS-EUC-V":   cnsEucV,
	"CNS1-H":      cns1H,
	"CNS1-V":      cns1V,
	"ETen-B5-H":   etenB5H,
	"ETen-B5-V":   etenB5V,
	"ETenms-B5-H": etenmsB5H,
	"ETenms-B5-V": etenmsB5V,

	// Japanese CMaps
	"83pv-RKSJ-H":        rksjH,
	"90ms-RKSJ-H":        rksjH,
	"90msp-RKSJ-H":       rksjH,
	"90pv-RKSJ-H":        rksjH,
	"Add-RKSJ-H":         rksjH,
	"Add-RKSJ-V":         rksjV,
	"Adobe-Japan1-0":     adobeJapan1,
	"Adobe-Japan1-1":     adobeJapan1,
	"Adobe-Japan1-2":     adobeJapan1,
	"Adobe-Japan1-3":     adobeJapan1,
	"Adobe-Japan1-4":     adobeJapan1,
	"Adobe-Japan1-5":     adobeJapan1,
	"Adobe-Japan1-6":     adobeJapan1,
	"Adobe-Japan2-0":     adobeJapan2,
	"EUC-H":              eucH,
	"EUC-V":              eucV,
	"Ext-RKSJ-H":         rksjH,
	"Ext-RKSJ-V":         rksjV,
	"H":                  eucH,
	"V":                  eucV,
	"RKSJ-H":             rksjH,
	"RKSJ-V":             rksjV,
	"UniJIS-UTF16-H":     uniJISUtf16H,
	"UniJIS-UTF16-V":     uniJISUtf16V,
	"UniJIS2004-UTF16-H": uniJIS2004Utf16H,
	"UniJIS2004-UTF16-V": uniJIS2004Utf16V,
	"UniJISPro-UTF8-H":   uniJISProUtf8H,
	"UniJISPro-UTF8-V":   uniJISProUtf8V,

	// Korean CMaps
	"Adobe-Korea1-0": adobeKorea1,
	"Adobe-Korea1-1": adobeKorea1,
	"Adobe-Korea1-2": adobeKorea1,
	"KSC-EUC-H":      kscEucH,
	"KSC-EUC-V":      kscEucV,
	"KSCms-UHC-H":    kscmsUhCH,
	"KSCms-UHC-HW-H": kscmsUhCH,
	"KSCms-UHC-HW-V": kscmsUhcV,
	"KSC-Johab-H":    kscJohabH,
	"KSC-Johab-V":    kscJohabV,
}

// Predefined CMap data (simplified for implementation)
const (
	rksjH = `/CMapName /RKSJ-H def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 4 >> def
/WMode 0 def
/CodeSpaceRange [<00> <80>] def
/CIDRange [<00> <80> 1] def`

	rksjV = `/CMapName /RKSJ-V def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 4 >> def
/WMode 1 def
/CodeSpaceRange [<00> <80>] def
/CIDRange [<00> <80> 1] def`

	adobeJapan1 = `/CMapName /Adobe-Japan1-0 def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 0 >> def
/WMode 0 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	adobeJapan2 = `/CMapName /Adobe-Japan2-0 def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan2) /Supplement 0 >> def
/WMode 0 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	eucH = `/CMapName /EUC-H def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 4 >> def
/WMode 0 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	eucV = `/CMapName /EUC-V def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 4 >> def
/WMode 1 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	uniJISUtf16H = `/CMapName /UniJIS-UTF16-H def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 4 >> def
/WMode 0 def
/CodeSpaceRange [<0000> <FFFF>] def
/CIDRange [<0000> <FFFF> 1] def`

	uniJISUtf16V = `/CMapName /UniJIS-UTF16-V def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 4 >> def
/WMode 1 def
/CodeSpaceRange [<0000> <FFFF>] def
/CIDRange [<0000> <FFFF> 1] def`

	uniJIS2004Utf16H = `/CMapName /UniJIS2004-UTF16-H def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 4 >> def
/WMode 0 def
/CodeSpaceRange [<0000> <FFFF>] def
/CIDRange [<0000> <FFFF> 1] def`

	uniJIS2004Utf16V = `/CMapName /UniJIS2004-UTF16-V def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 4 >> def
/WMode 1 def
/CodeSpaceRange [<0000> <FFFF>] def
/CIDRange [<0000> <FFFF> 1] def`

	uniJISProUtf8H = `/CMapName /UniJISPro-UTF8-H def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 4 >> def
/WMode 0 def
/CodeSpaceRange [<0000> <FFFF>] def
/CIDRange [<0000> <FFFF> 1] def`

	uniJISProUtf8V = `/CMapName /UniJISPro-UTF8-V def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Japan1) /Supplement 4 >> def
/WMode 1 def
/CodeSpaceRange [<0000> <FFFF>] def
/CIDRange [<0000> <FFFF> 1] def`

	gbkEucH = `/CMapName /GBK-EUC-H def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >> def
/WMode 0 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	gbk2KH = `/CMapName /GBK2K-H def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >> def
/WMode 0 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	gbpcEucH = `/CMapName /GBpc-EUC-H def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >> def
/WMode 0 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	b5pcH = `/CMapName /B5pc-H def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (CNS1) /Supplement 0 >> def
/WMode 0 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	b5pcV = `/CMapName /B5pc-V def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (CNS1) /Supplement 0 >> def
/WMode 1 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	cnsEucH = `/CMapName /CNS-EUC-H def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (CNS1) /Supplement 0 >> def
/WMode 0 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	cnsEucV = `/CMapName /CNS-EUC-V def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (CNS1) /Supplement 0 >> def
/WMode 1 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	cns1H = `/CMapName /CNS1-H def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (CNS1) /Supplement 0 >> def
/WMode 0 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	cns1V = `/CMapName /CNS1-V def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (CNS1) /Supplement 0 >> def
/WMode 1 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	etenB5H = `/CMapName /ETen-B5-H def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (CNS1) /Supplement 0 >> def
/WMode 0 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	etenB5V = `/CMapName /ETen-B5-V def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (CNS1) /Supplement 0 >> def
/WMode 1 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	etenmsB5H = `/CMapName /ETenms-B5-H def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (CNS1) /Supplement 0 >> def
/WMode 0 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	etenmsB5V = `/CMapName /ETenms-B5-V def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (CNS1) /Supplement 0 >> def
/WMode 1 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	adobeKorea1 = `/CMapName /Adobe-Korea1-0 def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Korea1) /Supplement 0 >> def
/WMode 0 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	kscEucH = `/CMapName /KSC-EUC-H def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Korea1) /Supplement 0 >> def
/WMode 0 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	kscEucV = `/CMapName /KSC-EUC-V def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Korea1) /Supplement 0 >> def
/WMode 1 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	kscmsUhCH = `/CMapName /KSCms-UHC-H def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Korea1) /Supplement 0 >> def
/WMode 0 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	kscmsUhcV = `/CMapName /KSCms-UHC-HW-V def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Korea1) /Supplement 0 >> def
/WMode 1 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	kscJohabH = `/CMapName /KSC-Johab-H def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Korea1) /Supplement 0 >> def
/WMode 0 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`

	kscJohabV = `/CMapName /KSC-Johab-V def
/CMapType 1 def
/CIDSystemInfo << /Registry (Adobe) /Ordering (Korea1) /Supplement 0 >> def
/WMode 1 def
/CodeSpaceRange [<00> <FF>] def
/CIDRange [<00> <FF> 1] def`
)
