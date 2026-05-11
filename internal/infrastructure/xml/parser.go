package xml

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/metadata"
)

// XMLParserError represents an error during XML parsing.
//
//revive:disable-next-line:exported
type XMLParserError int

// XML parser error codes.
const (
	NoError XMLParserError = iota
	EndOfDocument
	UnterminatedCDATA
	UnterminatedXMLDeclaration
	UnterminatedDoctypeDeclaration
	UnterminatedComment
	MalformedElement
	UnterminatedAttributeValue
	UnterminatedElement
	ElementNeverBegun
)

// Error returns the error message.
func (e XMLParserError) Error() string {
	switch e {
	case NoError:
		return "no error"
	case EndOfDocument:
		return "end of document"
	case UnterminatedCDATA:
		return "unterminated CDATA section"
	case UnterminatedXMLDeclaration:
		return "unterminated XML declaration"
	case UnterminatedDoctypeDeclaration:
		return "unterminated DOCTYPE declaration"
	case UnterminatedComment:
		return "unterminated comment"
	case MalformedElement:
		return "malformed element"
	case UnterminatedAttributeValue:
		return "unterminated attribute value"
	case UnterminatedElement:
		return "unterminated element"
	case ElementNeverBegun:
		return "element never begun"
	default:
		return "unknown error"
	}
}

// XMLParser parses XML strings into a simple DOM structure.
//
//revive:disable-next-line:exported
type XMLParser struct {
	lowerCaseName bool
	hasAttributes bool
}

// NewXMLParser creates a new XML parser with the given options.
func NewXMLParser(lowerCaseName, hasAttributes bool) *XMLParser {
	return &XMLParser{
		lowerCaseName: lowerCaseName,
		hasAttributes: hasAttributes,
	}
}

// ParseFromString parses an XML string and returns a Document.
func (p *XMLParser) ParseFromString(data string) (metadata.XMLDocument, error) {
	rootFragment := make([]*DOMNode, 0)
	parser := &xmlParserState{
		parser:          p,
		currentFragment: &rootFragment,
		stack:           make([]*[]*DOMNode, 0),
		errorCode:       NoError,
	}

	parser.parseXML(data)

	if parser.errorCode != NoError {
		return nil, parser.errorCode
	}

	if len(*parser.currentFragment) == 0 {
		return nil, fmt.Errorf("no root element found")
	}

	root := (*parser.currentFragment)[0]
	return NewDocument(root), nil
}

// xmlParserState holds the state during XML parsing.
type xmlParserState struct {
	parser          *XMLParser
	currentFragment *[]*DOMNode
	stack           []*[]*DOMNode
	errorCode       XMLParserError
}

// isWhitespace checks if a character at the given index is whitespace.
func isWhitespace(s string, index int) bool {
	if index >= len(s) {
		return false
	}
	ch := s[index]
	return ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t'
}

// isWhitespaceString checks if the entire string is whitespace.
func isWhitespaceString(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isWhitespace(s, i) {
			return false
		}
	}
	return true
}

// resolveEntities resolves HTML/XML entities in a string.
func resolveEntities(s string) string {
	// Numeric character references (&#xHHHH; and &#DDDD;)
	hexRe := regexp.MustCompile(`&#x([0-9a-fA-F]+);`)
	s = hexRe.ReplaceAllStringFunc(s, func(match string) string {
		hex := hexRe.FindStringSubmatch(match)[1]
		code, err := strconv.ParseInt(hex, 16, 32)
		if err != nil {
			return match
		}
		return string(rune(code))
	})

	decRe := regexp.MustCompile(`&#(\d+);`)
	s = decRe.ReplaceAllStringFunc(s, func(match string) string {
		dec := decRe.FindStringSubmatch(match)[1]
		code, err := strconv.ParseInt(dec, 10, 32)
		if err != nil {
			return match
		}
		return string(rune(code))
	})

	// Named entities
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&quot;", "\"")
	s = strings.ReplaceAll(s, "&apos;", "'")

	return s
}

// parseContent parses element name and attributes.
func (p *xmlParserState) parseContent(s string, start int) (name string, attrs []metadata.XMLAttribute, parsed int, err error) {
	pos := start

	skipWs := func() {
		for pos < len(s) && isWhitespace(s, pos) {
			pos++
		}
	}

	// Parse element name
	for pos < len(s) && !isWhitespace(s, pos) && s[pos] != '>' && s[pos] != '/' {
		pos++
	}
	name = s[start:pos]
	if p.parser.lowerCaseName {
		name = strings.ToLower(name)
	}
	skipWs()

	// Parse attributes
	attrs = make([]metadata.XMLAttribute, 0)
	for pos < len(s) && s[pos] != '>' && s[pos] != '/' && s[pos] != '?' {
		skipWs()

		// Parse attribute name
		attrStart := pos
		for pos < len(s) && !isWhitespace(s, pos) && s[pos] != '=' {
			pos++
		}
		if pos == attrStart {
			break
		}
		attrName := s[attrStart:pos]
		skipWs()

		if pos >= len(s) || s[pos] != '=' {
			return "", nil, 0, MalformedElement
		}
		pos++ // skip '='
		skipWs()

		// Parse attribute value
		if pos >= len(s) {
			return "", nil, 0, UnterminatedAttributeValue
		}
		quote := s[pos]
		if quote != '"' && quote != '\'' {
			return "", nil, 0, UnterminatedAttributeValue
		}
		pos++ // skip opening quote

		valueStart := pos
		for pos < len(s) && s[pos] != quote {
			pos++
		}
		if pos >= len(s) {
			return "", nil, 0, UnterminatedAttributeValue
		}
		attrValue := s[valueStart:pos]
		pos++ // skip closing quote

		attrs = append(attrs, metadata.XMLAttribute{
			Name:  attrName,
			Value: resolveEntities(attrValue),
		})
		skipWs()
	}

	return name, attrs, pos - start, nil
}

// parseXML is the main parsing function.
func (p *xmlParserState) parseXML(s string) {
	i := 0
	for i < len(s) {
		ch := s[i]
		j := i

		if ch == '<' {
			j++
			if j >= len(s) {
				p.errorCode = UnterminatedElement
				return
			}

			ch2 := s[j]
			switch ch2 {
			case '/': // End tag
				j++
				endIdx := strings.Index(s[j:], ">")
				if endIdx < 0 {
					p.errorCode = UnterminatedElement
					return
				}
				endIdx += j
				tagName := s[j:endIdx]
				if p.parser.lowerCaseName {
					tagName = strings.ToLower(tagName)
				}
				p.onEndElement(tagName)
				j = endIdx + 1

			case '?': // Processing instruction
				j++
				endIdx := strings.Index(s[j:], "?>")
				if endIdx < 0 {
					p.errorCode = UnterminatedXMLDeclaration
					return
				}
				// We ignore processing instructions for now
				j += endIdx + 2

			case '!': // Comment, CDATA, or DOCTYPE
				switch {
				case strings.HasPrefix(s[j:], "!--"):
					// Comment
					endIdx := strings.Index(s[j+3:], "-->")
					if endIdx < 0 {
						p.errorCode = UnterminatedComment
						return
					}
					// Ignore comments
					j += endIdx + 6

				case strings.HasPrefix(s[j:], "![CDATA["):
					// CDATA
					endIdx := strings.Index(s[j+8:], "]]>")
					if endIdx < 0 {
						p.errorCode = UnterminatedCDATA
						return
					}
					text := s[j+8 : j+8+endIdx]
					p.onCDATA(text)
					j += endIdx + 11

				case strings.HasPrefix(s[j:], "!DOCTYPE"):
					// DOCTYPE
					endIdx := strings.Index(s[j:], ">")
					if endIdx < 0 {
						p.errorCode = UnterminatedDoctypeDeclaration
						return
					}
					// Ignore DOCTYPE
					j += endIdx + 1

				default:
					p.errorCode = MalformedElement
					return
				}

			default: // Start tag
				name, attrs, parsed, err := p.parseContent(s, j)
				if err != nil {
					parserErr, ok := err.(XMLParserError)
					if !ok {
						p.errorCode = MalformedElement
						return
					}
					p.errorCode = parserErr
					return
				}

				isClosed := false
				if j+parsed+1 < len(s) && s[j+parsed:j+parsed+2] == "/>" {
					isClosed = true
				} else if j+parsed < len(s) && s[j+parsed] != '>' {
					p.errorCode = UnterminatedElement
					return
				}

				p.onBeginElement(name, attrs, isClosed)
				j += parsed + 1
				if isClosed {
					j++
				}
			}
		} else {
			// Text content
			for j < len(s) && s[j] != '<' {
				j++
			}
			text := s[i:j]
			p.onText(resolveEntities(text))
		}

		i = j
	}
}

// Event handlers

func (p *xmlParserState) onText(text string) {
	if isWhitespaceString(text) {
		return
	}
	node := NewDOMNode("#text", text)
	*p.currentFragment = append(*p.currentFragment, node)
}

func (p *xmlParserState) onCDATA(text string) {
	node := NewDOMNode("#text", text)
	*p.currentFragment = append(*p.currentFragment, node)
}

func (p *xmlParserState) onBeginElement(name string, attrs []metadata.XMLAttribute, isEmpty bool) {
	node := NewDOMNode(name, "")
	node.childNodes = make([]*DOMNode, 0)
	if p.parser.hasAttributes {
		node.setAttributes(attrs)
	}

	*p.currentFragment = append(*p.currentFragment, node)

	if !isEmpty {
		// Save current fragment before switching to node's children
		p.stack = append(p.stack, p.currentFragment)
		// Switch to node's children
		p.currentFragment = &node.childNodes
	}
}

func (p *xmlParserState) onEndElement(name string) {
	if len(p.stack) == 0 {
		return
	}

	p.currentFragment = p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]

	if len(*p.currentFragment) == 0 {
		return
	}

	lastElement := (*p.currentFragment)[len(*p.currentFragment)-1]
	for _, child := range lastElement.childNodes {
		child.parentNode = lastElement
	}
}
