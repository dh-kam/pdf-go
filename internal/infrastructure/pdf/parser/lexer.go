// Package parser provides PDF lexical analysis and parsing functionality.
package parser

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"unicode"
)

// TokenType represents the type of a token.
type TokenType int

const (
	// TokenEOF indicates end of input.
	TokenEOF TokenType = iota
	// TokenKeyword represents a PDF keyword or name.
	TokenKeyword
	// TokenString represents a string literal.
	TokenString
	// TokenHexString represents a hex-encoded string.
	TokenHexString
	// TokenNumber represents a numeric value.
	TokenNumber
	// TokenReal represents a real number.
	TokenReal
	// TokenComment represents a comment.
	TokenComment
	// TokenDictStart represents "<<".
	TokenDictStart
	// TokenDictEnd represents ">>".
	TokenDictEnd
	// TokenArrayStart represents "[".
	TokenArrayStart
	// TokenArrayEnd represents "]".
	TokenArrayEnd
	// TokenProcStart represents "(".
	TokenProcStart
	// TokenProcEnd represents ")".
	TokenProcEnd
	// TokenHexStart represents "<".
	TokenHexStart
	// TokenHexEnd represents ">".
	TokenHexEnd
)

// String returns the string representation of the token type.
func (t TokenType) String() string {
	switch t {
	case TokenEOF:
		return "EOF"
	case TokenKeyword:
		return "Keyword"
	case TokenString:
		return "String"
	case TokenHexString:
		return "HexString"
	case TokenNumber:
		return "Number"
	case TokenReal:
		return "Real"
	case TokenComment:
		return "Comment"
	case TokenDictStart:
		return "<<"
	case TokenDictEnd:
		return ">>"
	case TokenArrayStart:
		return "["
	case TokenArrayEnd:
		return "]"
	case TokenProcStart:
		return "("
	case TokenProcEnd:
		return ")"
	case TokenHexStart:
		return "<"
	case TokenHexEnd:
		return ">"
	default:
		return "Unknown"
	}
}

// Token represents a single lexical token.
type Token struct {
	Value          string
	Type           TokenType
	Pos            int
	IntValue       int64
	RealValue      float64
	HasNumberValue bool
}

// String returns the string representation of the token.
func (t Token) String() string {
	return fmt.Sprintf("%s(%q)", t.Type, t.Value)
}

// Lexer performs lexical analysis of PDF content.
type Lexer struct {
	reader          *bufio.Reader
	peeked          Token
	hasPeeked       bool
	data            []byte
	bytesMode       bool
	omitNumberValue bool
	pos             int
	line            int
	column          int
}

// NewLexer creates a new PDF lexer.
func NewLexer(r io.Reader) *Lexer {
	return &Lexer{
		reader: bufio.NewReader(r),
		pos:    0,
		line:   1,
		column: 1,
	}
}

// NewLexerBytes creates a new PDF lexer from a byte slice.
func NewLexerBytes(data []byte) *Lexer {
	return &Lexer{
		data:      data,
		bytesMode: true,
		pos:       0,
		line:      1,
		column:    1,
	}
}

// NewLexerBytesNoNumberValue creates a byte-slice lexer that avoids materializing
// numeric token strings while still exposing parsed numeric values.
func NewLexerBytesNoNumberValue(data []byte) *Lexer {
	lexer := NewLexerBytes(data)
	lexer.omitNumberValue = true
	return lexer
}

// NextToken reads and returns the next token.
func (l *Lexer) NextToken() (Token, error) {
	// Return peeked token if available
	if l.hasPeeked {
		token := l.peeked
		l.peeked = Token{}
		l.hasPeeked = false
		return token, nil
	}

	// Skip whitespace and comments
	for {
		if err := l.skipWhitespace(); err != nil {
			return Token{}, err
		}

		// Check for comment
		if ch, err := l.peekByte(); err == nil && ch == '%' {
			if err := l.skipComment(); err != nil {
				return Token{}, err
			}
			continue
		}
		break
	}

	ch, err := l.peekByte()
	if err != nil {
		if err == io.EOF {
			return Token{Type: TokenEOF, Pos: l.pos}, nil
		}
		return Token{}, err
	}

	startPos := l.pos

	// Dictionary delimiters
	if ch == '<' {
		if _, err := l.readByte(); err != nil { // consume '<'
			return Token{}, err
		}
		next, err := l.peekByte()
		if err == nil && next == '<' {
			if _, err := l.readByte(); err != nil { // consume second '<'
				return Token{}, err
			}
			return Token{Type: TokenDictStart, Value: "<<", Pos: startPos}, nil
		}
		// Any '<' not followed by another '<' is a hex string (PDF spec §7.3.4.3).
		// This includes empty hex strings <>, whitespace-only < >, and normal <4F>.
		if err != nil {
			// EOF immediately after '<' — return best-effort token.
			return Token{Type: TokenHexStart, Value: "<", Pos: startPos}, nil
		}
		return l.scanHexString()
	}

	if ch == '>' {
		if _, err := l.readByte(); err != nil { // consume '>'
			return Token{}, err
		}
		next, err := l.peekByte()
		if err == nil && next == '>' {
			if _, err := l.readByte(); err != nil { // consume second '>'
				return Token{}, err
			}
			return Token{Type: TokenDictEnd, Value: ">>", Pos: startPos}, nil
		}
		return Token{Type: TokenHexEnd, Value: ">", Pos: startPos}, nil
	}

	// Array delimiters
	if ch == '[' {
		if _, err := l.readByte(); err != nil {
			return Token{}, err
		}
		return Token{Type: TokenArrayStart, Value: "[", Pos: startPos}, nil
	}

	if ch == ']' {
		if _, err := l.readByte(); err != nil {
			return Token{}, err
		}
		return Token{Type: TokenArrayEnd, Value: "]", Pos: startPos}, nil
	}

	// String literal (starts with '(')
	if ch == '(' {
		return l.scanString()
	}

	// Name (starts with /)
	if ch == '/' {
		return l.scanName()
	}

	// Number (starts with digit, +/- sign, or leading decimal point).
	if ch == '+' || ch == '-' || ch == '.' || unicode.IsDigit(rune(ch)) {
		return l.scanNumber()
	}

	// Keyword
	return l.scanKeyword()
}

// Peek returns the next token without consuming it.
func (l *Lexer) Peek() (Token, error) {
	if !l.hasPeeked {
		token, err := l.NextToken()
		if err != nil {
			return Token{}, err
		}
		l.peeked = token
		l.hasPeeked = true
	}
	return l.peeked, nil
}

// Pos returns the current byte offset in the input stream.
func (l *Lexer) Pos() int {
	return l.pos
}

// skipWhitespace skips whitespace characters.
func (l *Lexer) skipWhitespace() error {
	for {
		ch, err := l.peekByte()
		if err != nil {
			// EOF is not an error for whitespace skipping
			return nil
		}

		if !unicode.IsSpace(rune(ch)) {
			return nil
		}

		if _, err := l.readByte(); err != nil {
			return err
		}

		// Track line/column
		if ch == '\n' {
			l.line++
			l.column = 1
		} else {
			l.column++
		}
	}
}

// skipComment skips a comment (starts with %, ends at newline).
func (l *Lexer) skipComment() error {
	for {
		ch, err := l.readByte()
		if err != nil {
			return err
		}

		if ch == '\n' || ch == '\r' {
			return nil
		}
	}
}

// scanName scans a PDF name (starts with /).
func (l *Lexer) scanName() (Token, error) {
	startPos := l.pos

	// Consume '/'
	ch, err := l.readByte()
	if err != nil {
		return Token{}, err
	}
	if ch != '/' {
		return Token{}, fmt.Errorf("expected '/', got %c", ch)
	}

	if l.bytesMode {
		valueStart := l.pos
		for l.pos < len(l.data) {
			ch = l.data[l.pos]
			if isPDFDelimiter(ch) {
				break
			}
			if ch == '#' {
				return l.scanEscapedName(startPos, valueStart)
			}
			l.pos++
			l.column++
		}
		if value, ok := internPDFNameBytes(l.data[valueStart:l.pos]); ok {
			return Token{
				Type:  TokenKeyword,
				Value: value,
				Pos:   startPos,
			}, nil
		}
		return Token{
			Type:  TokenKeyword,
			Value: string(l.data[valueStart:l.pos]),
			Pos:   startPos,
		}, nil
	}

	var value bytes.Buffer

	for {
		ch, err := l.peekByte()
		if err != nil {
			break
		}

		// Names end at whitespace or delimiter
		if unicode.IsSpace(rune(ch)) ||
			ch == '(' || ch == ')' || ch == '<' || ch == '>' ||
			ch == '[' || ch == ']' || ch == '/' || ch == '%' {
			break
		}

		if _, err := l.readByte(); err != nil {
			return Token{}, err
		}

		// Handle hex escape (#xx)
		if ch == '#' {
			high, err := l.readByte()
			if err != nil {
				return Token{}, err
			}
			low, err := l.readByte()
			if err != nil {
				return Token{}, err
			}

			decoded := hexDecode(high, low)
			value.WriteByte(decoded)
		} else {
			value.WriteByte(ch)
		}
		l.column++
	}

	return Token{
		Type:  TokenKeyword,
		Value: value.String(),
		Pos:   startPos,
	}, nil
}

func (l *Lexer) scanEscapedName(startPos, valueStart int) (Token, error) {
	var value bytes.Buffer
	value.Write(l.data[valueStart:l.pos])

	for {
		ch, err := l.peekByte()
		if err != nil {
			break
		}
		if isPDFDelimiter(ch) {
			break
		}
		if _, err := l.readByte(); err != nil {
			return Token{}, err
		}
		if ch == '#' {
			high, err := l.readByte()
			if err != nil {
				return Token{}, err
			}
			low, err := l.readByte()
			if err != nil {
				return Token{}, err
			}
			value.WriteByte(hexDecode(high, low))
			l.column += 3
			continue
		}
		value.WriteByte(ch)
		l.column++
	}

	if interned, ok := internPDFNameBytes(value.Bytes()); ok {
		return Token{
			Type:  TokenKeyword,
			Value: interned,
			Pos:   startPos,
		}, nil
	}

	return Token{
		Type:  TokenKeyword,
		Value: value.String(),
		Pos:   startPos,
	}, nil
}

func internPDFNameBytes(data []byte) (string, bool) {
	switch len(data) {
	case 1:
		switch data[0] {
		case '.':
			return ".", true
		case '+':
			return "+", true
		case '-':
			return "-", true
		case '_':
			return "_", true
		case '0':
			return "0", true
		case '1':
			return "1", true
		case '2':
			return "2", true
		case '3':
			return "3", true
		case '4':
			return "4", true
		case '5':
			return "5", true
		case '6':
			return "6", true
		case '7':
			return "7", true
		case '8':
			return "8", true
		case '9':
			return "9", true
		case 'A':
			return "A", true
		case 'B':
			return "B", true
		case 'C':
			return "C", true
		case 'D':
			return "D", true
		case 'E':
			return "E", true
		case 'F':
			return "F", true
		case 'G':
			return "G", true
		case 'H':
			return "H", true
		case 'I':
			return "I", true
		case 'J':
			return "J", true
		case 'K':
			return "K", true
		case 'L':
			return "L", true
		case 'M':
			return "M", true
		case 'N':
			return "N", true
		case 'O':
			return "O", true
		case 'P':
			return "P", true
		case 'Q':
			return "Q", true
		case 'R':
			return "R", true
		case 'S':
			return "S", true
		case 'T':
			return "T", true
		case 'U':
			return "U", true
		case 'V':
			return "V", true
		case 'W':
			return "W", true
		case 'X':
			return "X", true
		case 'Y':
			return "Y", true
		case 'Z':
			return "Z", true
		case 'a':
			return "a", true
		case 'b':
			return "b", true
		case 'c':
			return "c", true
		case 'd':
			return "d", true
		case 'e':
			return "e", true
		case 'f':
			return "f", true
		case 'g':
			return "g", true
		case 'h':
			return "h", true
		case 'i':
			return "i", true
		case 'j':
			return "j", true
		case 'k':
			return "k", true
		case 'l':
			return "l", true
		case 'm':
			return "m", true
		case 'n':
			return "n", true
		case 'o':
			return "o", true
		case 'p':
			return "p", true
		case 'q':
			return "q", true
		case 'r':
			return "r", true
		case 's':
			return "s", true
		case 't':
			return "t", true
		case 'u':
			return "u", true
		case 'v':
			return "v", true
		case 'w':
			return "w", true
		case 'x':
			return "x", true
		case 'y':
			return "y", true
		case 'z':
			return "z", true
		}
	case 2:
		switch data[0] {
		case 'A':
			if byteSliceEqualString(data, "A1") {
				return "A1", true
			}
			if byteSliceEqualString(data, "A2") {
				return "A2", true
			}
			if byteSliceEqualString(data, "A3") {
				return "A3", true
			}
			if byteSliceEqualString(data, "A4") {
				return "A4", true
			}
		case 'B':
			if byteSliceEqualString(data, "BM") {
				return "BM", true
			}
		case 'C':
			if byteSliceEqualString(data, "CA") {
				return "CA", true
			}
			if byteSliceEqualString(data, "CS") {
				return "CS", true
			}
			if byteSliceEqualString(data, "C0") {
				return "C0", true
			}
			if byteSliceEqualString(data, "C1") {
				return "C1", true
			}
		case 'D':
			if byteSliceEqualString(data, "DP") {
				return "DP", true
			}
			if byteSliceEqualString(data, "DW") {
				return "DW", true
			}
		case 'I':
			if byteSliceEqualString(data, "ID") {
				return "ID", true
			}
			if byteSliceEqualString(data, "IM") {
				return "IM", true
			}
		case 'L':
			if byteSliceEqualString(data, "LC") {
				return "LC", true
			}
			if byteSliceEqualString(data, "LJ") {
				return "LJ", true
			}
			if byteSliceEqualString(data, "LW") {
				return "LW", true
			}
		case 'M':
			if byteSliceEqualString(data, "ML") {
				return "ML", true
			}
			if byteSliceEqualString(data, "M0") {
				return "M0", true
			}
			if byteSliceEqualString(data, "M1") {
				return "M1", true
			}
			if byteSliceEqualString(data, "M2") {
				return "M2", true
			}
			if byteSliceEqualString(data, "M3") {
				return "M3", true
			}
		case 'O':
			if byteSliceEqualString(data, "OP") {
				return "OP", true
			}
		case 'P':
			if byteSliceEqualString(data, "P0") {
				return "P0", true
			}
		case 'S':
			if byteSliceEqualString(data, "SA") {
				return "SA", true
			}
		case 'W':
			if byteSliceEqualString(data, "W2") {
				return "W2", true
			}
		case 'c':
			if byteSliceEqualString(data, "ca") {
				return "ca", true
			}
		}
	case 3:
		switch data[0] {
		case 'A':
			if byteSliceEqualString(data, "AIS") {
				return "AIS", true
			}
		case 'B':
			if byteSliceEqualString(data, "BPC") {
				return "BPC", true
			}
		case 'D':
			if byteSliceEqualString(data, "DW2") {
				return "DW2", true
			}
		case 'O':
			if byteSliceEqualString(data, "OPM") {
				return "OPM", true
			}
		case 'X':
			if byteSliceEqualString(data, "XYZ") {
				return "XYZ", true
			}
		}
	case 4:
		switch data[0] {
		case 'B':
			if byteSliceEqualString(data, "BBox") {
				return "BBox", true
			}
		case 'G':
			if byteSliceEqualString(data, "GoTo") {
				return "GoTo", true
			}
		case 'L':
			if byteSliceEqualString(data, "Lang") {
				return "Lang", true
			}
			if byteSliceEqualString(data, "Link") {
				return "Link", true
			}
		case 'M':
			if byteSliceEqualString(data, "Mask") {
				return "Mask", true
			}
			if byteSliceEqualString(data, "MCID") {
				return "MCID", true
			}
		case 'F':
			if byteSliceEqualString(data, "Font") {
				return "Font", true
			}
			if byteSliceEqualString(data, "Form") {
				return "Form", true
			}
		case 'K':
			if byteSliceEqualString(data, "Kids") {
				return "Kids", true
			}
		case 'P':
			if byteSliceEqualString(data, "Page") {
				return "Page", true
			}
		case 'N':
			if byteSliceEqualString(data, "Name") {
				return "Name", true
			}
		case 'R':
			if byteSliceEqualString(data, "Rect") {
				return "Rect", true
			}
			if byteSliceEqualString(data, "Root") {
				return "Root", true
			}
		case 'S':
			if byteSliceEqualString(data, "Span") {
				return "Span", true
			}
			if byteSliceEqualString(data, "Size") {
				return "Size", true
			}
		case 'T':
			if byteSliceEqualString(data, "Type") {
				return "Type", true
			}
			if byteSliceEqualString(data, "Type0") {
				return "Type0", true
			}
			if byteSliceEqualString(data, "Type1") {
				return "Type1", true
			}
			if byteSliceEqualString(data, "Type3") {
				return "Type3", true
			}
		case 'W':
			if byteSliceEqualString(data, "WMode") {
				return "WMode", true
			}
		}
	case 5:
		switch data[0] {
		case 'A':
			if byteSliceEqualString(data, "Annot") {
				return "Annot", true
			}
		case 'F':
			if byteSliceEqualString(data, "Flags") {
				return "Flags", true
			}
		case 'C':
			if byteSliceEqualString(data, "Count") {
				return "Count", true
			}
		case 'S':
			if byteSliceEqualString(data, "StemV") {
				return "StemV", true
			}
			if byteSliceEqualString(data, "SMask") {
				return "SMask", true
			}
		case 'G':
			if byteSliceEqualString(data, "Group") {
				return "Group", true
			}
		case 'I':
			if byteSliceEqualString(data, "Index") {
				return "Index", true
			}
			if byteSliceEqualString(data, "Image") {
				return "Image", true
			}
		case 'M':
			if byteSliceEqualString(data, "Matte") {
				return "Matte", true
			}
		case 'P':
			if byteSliceEqualString(data, "Pages") {
				return "Pages", true
			}
		case 'R':
			if byteSliceEqualString(data, "Range") {
				return "Range", true
			}
		case 'W':
			if byteSliceEqualString(data, "Width") {
				return "Width", true
			}
		case 'X':
			if byteSliceEqualString(data, "XStep") {
				return "XStep", true
			}
		case 'Y':
			if byteSliceEqualString(data, "YStep") {
				return "YStep", true
			}
		}
	case 6:
		switch data[0] {
		case 'A':
			if byteSliceEqualString(data, "Ascent") {
				return "Ascent", true
			}
		case 'C':
			if byteSliceEqualString(data, "CIDSet") {
				return "CIDSet", true
			}
		case 'D':
			if byteSliceEqualString(data, "Decode") {
				return "Decode", true
			}
			if byteSliceEqualString(data, "Domain") {
				return "Domain", true
			}
		case 'B':
			if byteSliceEqualString(data, "Border") {
				return "Border", true
			}
		case 'F':
			if byteSliceEqualString(data, "Filter") {
				return "Filter", true
			}
		case 'H':
			if byteSliceEqualString(data, "Height") {
				return "Height", true
			}
		case 'L':
			if byteSliceEqualString(data, "Length") {
				return "Length", true
			}
			if byteSliceEqualString(data, "Length1") {
				return "Length1", true
			}
			if byteSliceEqualString(data, "Length2") {
				return "Length2", true
			}
			if byteSliceEqualString(data, "Length3") {
				return "Length3", true
			}
		case 'M':
			if byteSliceEqualString(data, "Matrix") {
				return "Matrix", true
			}
		case 'P':
			if byteSliceEqualString(data, "Parent") {
				return "Parent", true
			}
		case 'W':
			if byteSliceEqualString(data, "Widths") {
				return "Widths", true
			}
		}
	case 7:
		switch data[0] {
		case 'C':
			if byteSliceEqualString(data, "CMapName") {
				return "CMapName", true
			}
			if byteSliceEqualString(data, "CropBox") {
				return "CropBox", true
			}
		case 'F':
			if byteSliceEqualString(data, "FontBBox") {
				return "FontBBox", true
			}
		case 'I':
			if byteSliceEqualString(data, "Indexed") {
				return "Indexed", true
			}
		case 'O':
			if byteSliceEqualString(data, "Ordering") {
				return "Ordering", true
			}
		case 'P':
			if byteSliceEqualString(data, "Pattern") {
				return "Pattern", true
			}
			if byteSliceEqualString(data, "ProcSet") {
				return "ProcSet", true
			}
		case 'S':
			if byteSliceEqualString(data, "Shading") {
				return "Shading", true
			}
			if byteSliceEqualString(data, "Subtype") {
				return "Subtype", true
			}
		case 'X':
			if byteSliceEqualString(data, "XObject") {
				return "XObject", true
			}
		}
	case 8:
		switch data[0] {
		case 'B':
			if byteSliceEqualString(data, "BaseFont") {
				return "BaseFont", true
			}
		case 'R':
			if byteSliceEqualString(data, "Registry") {
				return "Registry", true
			}
		case 'C':
			if byteSliceEqualString(data, "Contents") {
				return "Contents", true
			}
		case 'E':
			if byteSliceEqualString(data, "Encoding") {
				return "Encoding", true
			}
		case 'F':
			if byteSliceEqualString(data, "FontFile") {
				return "FontFile", true
			}
			if byteSliceEqualString(data, "Function") {
				return "Function", true
			}
		case 'I':
			if byteSliceEqualString(data, "ICCBased") {
				return "ICCBased", true
			}
		case 'L':
			if byteSliceEqualString(data, "LastChar") {
				return "LastChar", true
			}
		case 'M':
			if byteSliceEqualString(data, "MediaBox") {
				return "MediaBox", true
			}
		case 'T':
			if byteSliceEqualString(data, "TrueType") {
				return "TrueType", true
			}
		case 'X':
			if byteSliceEqualString(data, "XHeight") {
				return "XHeight", true
			}
		}
	case 9:
		switch data[0] {
		case 'C':
			if byteSliceEqualString(data, "CapHeight") {
				return "CapHeight", true
			}
			if byteSliceEqualString(data, "CharProcs") {
				return "CharProcs", true
			}
		case 'D':
			if byteSliceEqualString(data, "DCTDecode") {
				return "DCTDecode", true
			}
			if byteSliceEqualString(data, "DeviceRGB") {
				return "DeviceRGB", true
			}
			if byteSliceEqualString(data, "Differences") {
				return "Differences", true
			}
		case 'E':
			if byteSliceEqualString(data, "ExtGState") {
				return "ExtGState", true
			}
		case 'F':
			if byteSliceEqualString(data, "FirstChar") {
				return "FirstChar", true
			}
			if byteSliceEqualString(data, "FontFile2") {
				return "FontFile2", true
			}
			if byteSliceEqualString(data, "FontFile3") {
				return "FontFile3", true
			}
			if byteSliceEqualString(data, "FontMatrix") {
				return "FontMatrix", true
			}
		case 'I':
			if byteSliceEqualString(data, "ImageMask") {
				return "ImageMask", true
			}
		case 'P':
			if byteSliceEqualString(data, "PaintType") {
				return "PaintType", true
			}
		case 'R':
			if byteSliceEqualString(data, "Resources") {
				return "Resources", true
			}
		case 'T':
			if byteSliceEqualString(data, "ToUnicode") {
				return "ToUnicode", true
			}
		}
	case 10:
		switch data[0] {
		case 'C':
			if byteSliceEqualString(data, "ColorSpace") {
				return "ColorSpace", true
			}
		case 'D':
			if byteSliceEqualString(data, "DeviceCMYK") {
				return "DeviceCMYK", true
			}
			if byteSliceEqualString(data, "DeviceGray") {
				return "DeviceGray", true
			}
		case 'T':
			if byteSliceEqualString(data, "TilingType") {
				return "TilingType", true
			}
		case 'S':
			if byteSliceEqualString(data, "Supplement") {
				return "Supplement", true
			}
		}
	case 11:
		switch data[0] {
		case 'C':
			if byteSliceEqualString(data, "CIDToGIDMap") {
				return "CIDToGIDMap", true
			}
		case 'D':
			if byteSliceEqualString(data, "DecodeParms") {
				return "DecodeParms", true
			}
		case 'F':
			if byteSliceEqualString(data, "FlateDecode") {
				return "FlateDecode", true
			}
		case 'I':
			if byteSliceEqualString(data, "Interpolate") {
				return "Interpolate", true
			}
			if byteSliceEqualString(data, "ItalicAngle") {
				return "ItalicAngle", true
			}
		case 'P':
			if byteSliceEqualString(data, "PatternType") {
				return "PatternType", true
			}
		case 'S':
			if byteSliceEqualString(data, "ShadingType") {
				return "ShadingType", true
			}
		}
	case 12:
		if byteSliceEqualString(data, "BaseEncoding") {
			return "BaseEncoding", true
		}
		if byteSliceEqualString(data, "CIDFontType0") {
			return "CIDFontType0", true
		}
		if byteSliceEqualString(data, "CIDFontType2") {
			return "CIDFontType2", true
		}
		if byteSliceEqualString(data, "FunctionType") {
			return "FunctionType", true
		}
	case 13:
		if byteSliceEqualString(data, "CIDSystemInfo") {
			return "CIDSystemInfo", true
		}
	case 14:
		if byteSliceEqualString(data, "FontDescriptor") {
			return "FontDescriptor", true
		}
	case 15:
		if byteSliceEqualString(data, "DescendantFonts") {
			return "DescendantFonts", true
		}
	case 16:
		if byteSliceEqualString(data, "BitsPerComponent") {
			return "BitsPerComponent", true
		}
	}
	return "", false
}

func byteSliceEqualString(data []byte, value string) bool {
	if len(data) != len(value) {
		return false
	}
	for i, b := range data {
		if b != value[i] {
			return false
		}
	}
	return true
}

// scanNumber scans a number (integer or real).
func (l *Lexer) scanNumber() (Token, error) {
	startPos := l.pos
	if l.bytesMode {
		tokenType, intValue, realValue, err := l.scanNumberBytes(startPos)
		if err != nil {
			return Token{}, err
		}
		token := Token{
			Type:           tokenType,
			Pos:            startPos,
			IntValue:       intValue,
			RealValue:      realValue,
			HasNumberValue: true,
		}
		if !l.omitNumberValue {
			token.Value = string(l.data[startPos:l.pos])
		}
		return token, nil
	}

	var value bytes.Buffer

	// Optional sign (already validated by caller)
	ch, err := l.peekByte()
	if err == nil && (ch == '+' || ch == '-') {
		if _, err := l.readByte(); err != nil {
			return Token{}, err
		}
		value.WriteByte(ch)
		l.column++
	}

	// Integer part
	hasDigit := false
	for {
		ch, err = l.peekByte()
		if err != nil || !unicode.IsDigit(rune(ch)) {
			break
		}
		if _, err := l.readByte(); err != nil {
			return Token{}, err
		}
		value.WriteByte(ch)
		hasDigit = true
		l.column++
	}

	// Check for real number
	ch, err = l.peekByte()
	if err == nil && ch == '.' {
		// Check if next char is digit (real number)
		if _, err := l.readByte(); err != nil {
			return Token{}, err
		}
		value.WriteByte(ch)
		l.column++

		for {
			ch, err = l.peekByte()
			if err != nil || !unicode.IsDigit(rune(ch)) {
				break
			}
			if _, err := l.readByte(); err != nil {
				return Token{}, err
			}
			value.WriteByte(ch)
			l.column++
		}

		return Token{
			Type:  TokenReal,
			Value: value.String(),
			Pos:   startPos,
		}, nil
	}

	if !hasDigit {
		return Token{}, fmt.Errorf("invalid number at position %d", startPos)
	}

	return Token{
		Type:  TokenNumber,
		Value: value.String(),
		Pos:   startPos,
	}, nil
}

func (l *Lexer) scanNumberBytes(startPos int) (TokenType, int64, float64, error) {
	var sign int64 = 1
	realSign := 1.0
	if l.pos < len(l.data) && (l.data[l.pos] == '+' || l.data[l.pos] == '-') {
		if l.data[l.pos] == '-' {
			sign = -1
			realSign = -1
		}
		l.pos++
		l.column++
	}

	hasDigit := false
	var intValue int64
	var realValue float64
	for l.pos < len(l.data) && l.data[l.pos] >= '0' && l.data[l.pos] <= '9' {
		digit := int64(l.data[l.pos] - '0')
		intValue = intValue*10 + digit
		realValue = realValue*10 + float64(digit)
		l.pos++
		l.column++
		hasDigit = true
	}

	if l.pos < len(l.data) && l.data[l.pos] == '.' {
		l.pos++
		l.column++
		fraction := 0.1
		for l.pos < len(l.data) && l.data[l.pos] >= '0' && l.data[l.pos] <= '9' {
			realValue += float64(l.data[l.pos]-'0') * fraction
			fraction *= 0.1
			l.pos++
			l.column++
		}
		return TokenReal, 0, realSign * realValue, nil
	}

	if !hasDigit {
		return TokenEOF, 0, 0, fmt.Errorf("invalid number at position %d", startPos)
	}
	return TokenNumber, sign * intValue, 0, nil
}

// scanString scans a string literal.
func (l *Lexer) scanString() (Token, error) {
	startPos := l.pos

	// Consume '('
	ch, err := l.readByte()
	if err != nil {
		return Token{}, err
	}
	if ch != '(' {
		return Token{}, fmt.Errorf("expected '(', got %c", ch)
	}

	var value bytes.Buffer
	parenLevel := 1

	for {
		ch, err = l.readByte()
		if err != nil {
			return Token{}, err
		}
		l.column++

		switch ch {
		case '(':
			parenLevel++
			value.WriteByte(ch)
		case ')':
			parenLevel--
			if parenLevel == 0 {
				return Token{
					Type:  TokenString,
					Value: value.String(),
					Pos:   startPos,
				}, nil
			}
			value.WriteByte(ch)
		case '\\':
			// Escape sequence
			next, err := l.peekByte()
			if err != nil {
				value.WriteByte(ch)
				break
			}

			switch next {
			case 'n':
				if _, err := l.readByte(); err != nil {
					return Token{}, err
				}
				value.WriteByte('\n')
			case 'r':
				if _, err := l.readByte(); err != nil {
					return Token{}, err
				}
				value.WriteByte('\r')
			case 't':
				if _, err := l.readByte(); err != nil {
					return Token{}, err
				}
				value.WriteByte('\t')
			case 'b':
				if _, err := l.readByte(); err != nil {
					return Token{}, err
				}
				value.WriteByte('\b')
			case 'f':
				if _, err := l.readByte(); err != nil {
					return Token{}, err
				}
				value.WriteByte('\f')
			case '(', ')', '\\':
				if _, err := l.readByte(); err != nil {
					return Token{}, err
				}
				value.WriteByte(next)
			case '\n', '\r':
				// Line continuation - skip the newline
				if _, err := l.readByte(); err != nil {
					return Token{}, err
				}
				if next == '\r' {
					peeked, peekErr := l.peekByte()
					if peekErr == nil && peeked == '\n' {
						if _, err := l.readByte(); err != nil {
							return Token{}, err
						}
					}
				}
			case '0', '1', '2', '3', '4', '5', '6', '7':
				// Octal escape (up to 3 digits)
				var octal bytes.Buffer
				for i := 0; i < 3; i++ {
					if ch, err := l.peekByte(); err == nil && ch >= '0' && ch <= '7' {
						if _, err := l.readByte(); err != nil {
							return Token{}, err
						}
						octal.WriteByte(ch)
					}
				}
				decoded := octalDecode(octal.Bytes())
				value.WriteByte(decoded)
			default:
				value.WriteByte(ch)
			}
		default:
			value.WriteByte(ch)
		}
	}
}

// scanHexString scans a hex-encoded string (<...>).
func (l *Lexer) scanHexString() (Token, error) {
	startPos := l.pos
	var value bytes.Buffer

	for {
		ch, err := l.readByte()
		if err != nil {
			return Token{}, err
		}
		l.column++

		if ch == '>' {
			break
		}

		if isHexDigit(ch) {
			value.WriteByte(ch)
		}
		// Ignore non-hex characters (whitespace)
	}

	return Token{
		Type:  TokenHexString,
		Value: value.String(),
		Pos:   startPos,
	}, nil
}

// scanKeyword scans a keyword (sequence of non-delimiter characters).
func (l *Lexer) scanKeyword() (Token, error) {
	startPos := l.pos
	if l.bytesMode {
		for l.pos < len(l.data) {
			ch := l.data[l.pos]
			if isPDFDelimiter(ch) {
				break
			}
			l.pos++
			l.column++
		}
		if value, ok := internPDFKeywordBytes(l.data[startPos:l.pos]); ok {
			return Token{
				Type:  TokenKeyword,
				Value: value,
				Pos:   startPos,
			}, nil
		}
		return Token{
			Type:  TokenKeyword,
			Value: string(l.data[startPos:l.pos]),
			Pos:   startPos,
		}, nil
	}

	var value bytes.Buffer

	for {
		ch, err := l.peekByte()
		if err != nil {
			break
		}

		// Delimiters
		if unicode.IsSpace(rune(ch)) ||
			ch == '(' || ch == ')' || ch == '<' || ch == '>' ||
			ch == '[' || ch == ']' || ch == '/' || ch == '%' {
			break
		}

		if _, err := l.readByte(); err != nil {
			return Token{}, err
		}
		value.WriteByte(ch)
		l.column++
	}

	return Token{
		Type:  TokenKeyword,
		Value: value.String(),
		Pos:   startPos,
	}, nil
}

func internPDFKeywordBytes(data []byte) (string, bool) {
	switch len(data) {
	case 1:
		switch data[0] {
		case 'B':
			return "B", true
		case 'F':
			return "F", true
		case 'G':
			return "G", true
		case 'J':
			return "J", true
		case 'K':
			return "K", true
		case 'M':
			return "M", true
		case 'Q':
			return "Q", true
		case 'R':
			return "R", true
		case 'S':
			return "S", true
		case 'W':
			return "W", true
		case 'b':
			return "b", true
		case 'c':
			return "c", true
		case 'd':
			return "d", true
		case 'f':
			return "f", true
		case 'g':
			return "g", true
		case 'h':
			return "h", true
		case 'j':
			return "j", true
		case 'k':
			return "k", true
		case 'l':
			return "l", true
		case 'm':
			return "m", true
		case 'n':
			return "n", true
		case 'q':
			return "q", true
		case 's':
			return "s", true
		case 'v':
			return "v", true
		case 'w':
			return "w", true
		case 'y':
			return "y", true
		case '\'':
			return "'", true
		case '"':
			return "\"", true
		}
	case 2:
		switch data[0] {
		case 'B':
			if byteSliceEqualString(data, "BI") {
				return "BI", true
			}
			if byteSliceEqualString(data, "BT") {
				return "BT", true
			}
			if byteSliceEqualString(data, "BX") {
				return "BX", true
			}
			if byteSliceEqualString(data, "B*") {
				return "B*", true
			}
		case 'C':
			if byteSliceEqualString(data, "CS") {
				return "CS", true
			}
		case 'D':
			if byteSliceEqualString(data, "DP") {
				return "DP", true
			}
			if byteSliceEqualString(data, "Do") {
				return "Do", true
			}
		case 'E':
			if byteSliceEqualString(data, "EI") {
				return "EI", true
			}
			if byteSliceEqualString(data, "ET") {
				return "ET", true
			}
			if byteSliceEqualString(data, "EX") {
				return "EX", true
			}
		case 'I':
			if byteSliceEqualString(data, "ID") {
				return "ID", true
			}
		case 'M':
			if byteSliceEqualString(data, "MP") {
				return "MP", true
			}
		case 'R':
			if byteSliceEqualString(data, "RG") {
				return "RG", true
			}
		case 'S':
			if byteSliceEqualString(data, "SC") {
				return "SC", true
			}
		case 'T':
			if byteSliceEqualString(data, "TD") {
				return "TD", true
			}
			if byteSliceEqualString(data, "TJ") {
				return "TJ", true
			}
			if byteSliceEqualString(data, "TL") {
				return "TL", true
			}
			if byteSliceEqualString(data, "Tc") {
				return "Tc", true
			}
			if byteSliceEqualString(data, "Td") {
				return "Td", true
			}
			if byteSliceEqualString(data, "Tf") {
				return "Tf", true
			}
			if byteSliceEqualString(data, "Tj") {
				return "Tj", true
			}
			if byteSliceEqualString(data, "Tm") {
				return "Tm", true
			}
			if byteSliceEqualString(data, "Tr") {
				return "Tr", true
			}
			if byteSliceEqualString(data, "Ts") {
				return "Ts", true
			}
			if byteSliceEqualString(data, "Tw") {
				return "Tw", true
			}
			if byteSliceEqualString(data, "Tz") {
				return "Tz", true
			}
			if byteSliceEqualString(data, "T*") {
				return "T*", true
			}
		case 'W':
			if byteSliceEqualString(data, "W*") {
				return "W*", true
			}
		case 'b':
			if byteSliceEqualString(data, "b*") {
				return "b*", true
			}
		case 'c':
			if byteSliceEqualString(data, "cm") {
				return "cm", true
			}
			if byteSliceEqualString(data, "cs") {
				return "cs", true
			}
		case 'f':
			if byteSliceEqualString(data, "f*") {
				return "f*", true
			}
		case 'g':
			if byteSliceEqualString(data, "gs") {
				return "gs", true
			}
		case 'r':
			if byteSliceEqualString(data, "re") {
				return "re", true
			}
			if byteSliceEqualString(data, "rg") {
				return "rg", true
			}
		case 's':
			if byteSliceEqualString(data, "sc") {
				return "sc", true
			}
			if byteSliceEqualString(data, "sh") {
				return "sh", true
			}
		}
	case 3:
		switch data[0] {
		case 'B':
			if byteSliceEqualString(data, "BDC") {
				return "BDC", true
			}
			if byteSliceEqualString(data, "BMC") {
				return "BMC", true
			}
		case 'E':
			if byteSliceEqualString(data, "EMC") {
				return "EMC", true
			}
		case 'S':
			if byteSliceEqualString(data, "SCN") {
				return "SCN", true
			}
		case 'o':
			if byteSliceEqualString(data, "obj") {
				return "obj", true
			}
		case 's':
			if byteSliceEqualString(data, "scn") {
				return "scn", true
			}
		}
	case 4:
		switch data[0] {
		case 'n':
			if byteSliceEqualString(data, "null") {
				return "null", true
			}
		case 't':
			if byteSliceEqualString(data, "true") {
				return "true", true
			}
		case 'x':
			if byteSliceEqualString(data, "xref") {
				return "xref", true
			}
		}
	case 5:
		switch data[0] {
		case 'f':
			if byteSliceEqualString(data, "false") {
				return "false", true
			}
		}
	case 6:
		switch data[0] {
		case 'e':
			if byteSliceEqualString(data, "endobj") {
				return "endobj", true
			}
		case 's':
			if byteSliceEqualString(data, "stream") {
				return "stream", true
			}
		}
	case 7:
		switch data[0] {
		case 't':
			if byteSliceEqualString(data, "trailer") {
				return "trailer", true
			}
		}
	case 9:
		if byteSliceEqualString(data, "endstream") {
			return "endstream", true
		}
		if byteSliceEqualString(data, "startxref") {
			return "startxref", true
		}
	}
	return "", false
}

func isPDFDelimiter(ch byte) bool {
	return unicode.IsSpace(rune(ch)) ||
		ch == '(' || ch == ')' || ch == '<' || ch == '>' ||
		ch == '[' || ch == ']' || ch == '/' || ch == '%'
}

// readByte reads a single byte.
func (l *Lexer) readByte() (byte, error) {
	if l.bytesMode {
		if l.pos >= len(l.data) {
			return 0, io.EOF
		}
		ch := l.data[l.pos]
		l.pos++
		return ch, nil
	}
	l.pos++
	return l.reader.ReadByte()
}

// peekByte peeks at the next byte without consuming it.
func (l *Lexer) peekByte() (byte, error) {
	if l.bytesMode {
		if l.pos >= len(l.data) {
			return 0, io.EOF
		}
		return l.data[l.pos], nil
	}
	bytes, err := l.reader.Peek(1)
	if err != nil {
		return 0, err
	}
	if len(bytes) == 0 {
		return 0, io.EOF
	}
	return bytes[0], nil
}

// isHexDigit returns true if b is a hexadecimal digit.
func isHexDigit(b byte) bool {
	return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
}

// hexDecode decodes two hex digits to a byte.
func hexDecode(high, low byte) byte {
	decode := func(b byte) byte {
		switch {
		case b >= '0' && b <= '9':
			return b - '0'
		case b >= 'a' && b <= 'f':
			return b - 'a' + 10
		case b >= 'A' && b <= 'F':
			return b - 'A' + 10
		default:
			return 0
		}
	}
	return decode(high)<<4 | decode(low)
}

// octalDecode decodes up to 3 octal digits to a byte.
func octalDecode(digits []byte) byte {
	var result byte
	for _, d := range digits {
		if d >= '0' && d <= '7' {
			result = result<<3 + (d - '0')
		}
	}
	return result
}
