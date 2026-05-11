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
	Value string
	Type  TokenType
	Pos   int
}

// String returns the string representation of the token.
func (t Token) String() string {
	return fmt.Sprintf("%s(%q)", t.Type, t.Value)
}

// Lexer performs lexical analysis of PDF content.
type Lexer struct {
	reader *bufio.Reader
	peeked *Token
	buf    []byte
	pos    int
	line   int
	column int
}

// NewLexer creates a new PDF lexer.
func NewLexer(r io.Reader) *Lexer {
	return &Lexer{
		reader: bufio.NewReader(r),
		buf:    make([]byte, 0, 4096),
		pos:    0,
		line:   1,
		column: 1,
	}
}

// NewLexerBytes creates a new PDF lexer from a byte slice.
func NewLexerBytes(data []byte) *Lexer {
	return &Lexer{
		reader: bufio.NewReader(bytes.NewReader(data)),
		buf:    data,
		pos:    0,
		line:   1,
		column: 1,
	}
}

// NextToken reads and returns the next token.
func (l *Lexer) NextToken() (Token, error) {
	// Return peeked token if available
	if l.peeked != nil {
		token := *l.peeked
		l.peeked = nil
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
	if l.peeked == nil {
		token, err := l.NextToken()
		if err != nil {
			return Token{}, err
		}
		l.peeked = &token
	}
	return *l.peeked, nil
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

// scanNumber scans a number (integer or real).
func (l *Lexer) scanNumber() (Token, error) {
	startPos := l.pos
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

// readByte reads a single byte.
func (l *Lexer) readByte() (byte, error) {
	l.pos++
	return l.reader.ReadByte()
}

// peekByte peeks at the next byte without consuming it.
func (l *Lexer) peekByte() (byte, error) {
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
