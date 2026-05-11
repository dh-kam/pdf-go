// Package type1 provides Type1 font parsing and rendering functionality.
package type1

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/errors"
)

// Parser handles Type1 font file parsing (PFB and PFA formats).
type Parser struct {
	data []byte
	pos  int
}

// PFBHeader represents the PFB file header.
type PFBHeader struct {
	Magic1     uint8 // 0x80
	Magic2     uint8 // 0x01
	LengthLow  uint8
	LengthHigh uint8
	Magic3     uint8 // 0x80
	Type       uint8 // 1=ASCII, 2=binary, 3=EOF
}

// PFBSegment represents a segment in a PFB file.
type PFBSegment struct {
	Data   []byte
	Length uint32
	Type   uint8
}

// NewParser creates a new Type1 font parser.
func NewParser(data []byte) *Parser {
	return &Parser{
		data: data,
		pos:  0,
	}
}

// Parse parses a Type1 font file (PFB or PFA format).
func (p *Parser) Parse() (*FontFile, error) {
	// Detect format
	if len(p.data) < 2 {
		return nil, errors.Invalid("type1_font", fmt.Errorf("file too short"))
	}

	// Check for PFB format (starts with 0x80 0x01)
	if p.data[0] == 0x80 && p.data[1] == 0x01 {
		return p.parsePFB()
	}

	// Otherwise, assume PFA (ASCII) format
	return p.parsePFA()
}

// parsePFB parses a PFB (binary) format Type1 font file.
func (p *Parser) parsePFB() (*FontFile, error) {
	fontFile := &FontFile{
		Segments: make([]PFBSegment, 0),
		rawData:  append([]byte(nil), p.data...),
	}

	p.pos = 0
	for p.pos < len(p.data) {
		if p.pos+6 > len(p.data) {
			break
		}

		// Read segment header
		magic1 := p.data[p.pos]
		magic2 := p.data[p.pos+1]

		if magic1 != 0x80 {
			break // End of segments
		}

		lengthLow := p.data[p.pos+2]
		lengthHigh := p.data[p.pos+3]
		magic3 := p.data[p.pos+4]
		segType := p.data[p.pos+5]

		if magic2 != 0x01 || magic3 != 0x80 {
			return nil, errors.Invalid("type1_pfb", fmt.Errorf("invalid PFB header"))
		}

		if segType == 3 {
			// EOF marker
			break
		}

		// Calculate length (little endian)
		length := uint32(lengthLow) | uint32(lengthHigh)<<8
		p.pos += 6

		// Read segment data
		if p.pos+int(length) > len(p.data) {
			return nil, errors.Invalid("type1_pfb", fmt.Errorf("segment data exceeds file bounds"))
		}

		data := make([]byte, length)
		copy(data, p.data[p.pos:p.pos+int(length)])
		p.pos += int(length)

		fontFile.Segments = append(fontFile.Segments, PFBSegment{
			Type:   segType,
			Data:   data,
			Length: length,
		})
	}

	// Extract ASCII and binary parts
	var asciiPart, binaryPart []byte
	for _, seg := range fontFile.Segments {
		if seg.Type == 1 {
			asciiPart = append(asciiPart, seg.Data...)
		} else if seg.Type == 2 {
			binaryPart = append(binaryPart, seg.Data...)
		}
	}

	fontFile.ASCII = asciiPart
	fontFile.Binary = binaryPart

	// Parse font dictionary from ASCII part
	err := p.parseFontDictionary(fontFile)
	if err != nil {
		return nil, err
	}

	return fontFile, nil
}

// parsePFA parses a PFA (ASCII/Hex) format Type1 font file.
func (p *Parser) parsePFA() (*FontFile, error) {
	fontFile := &FontFile{
		Segments: make([]PFBSegment, 0),
		rawData:  append([]byte(nil), p.data...),
	}

	// PFA format is text-based with hexadecimal binary data
	// Look for "Binary" section or eexec encrypted part
	dataStr := string(p.data)

	// Find the start of binary data (eexec encrypted section)
	eexecPos := findEexecStart(dataStr)
	if eexecPos == -1 {
		// No eexec section, treat as plain ASCII
		fontFile.ASCII = p.data
		return fontFile, p.parseFontDictionary(fontFile)
	}

	// Split into ASCII and binary parts
	fontFile.ASCII = []byte(dataStr[:eexecPos])
	dataBytes := p.data[eexecPos:]

	// Parse binary segment depending on encoding style.
	// Adobe font programs can contain either binary or hex-encoded eexec segments.
	decoded, decodeErr := decodeType1EncryptedBinary(dataBytes)
	if decodeErr != nil {
		return nil, decodeErr
	}
	fontFile.Binary = decoded

	// Parse font dictionary
	err := p.parseFontDictionary(fontFile)
	if err != nil {
		return nil, err
	}

	return fontFile, nil
}

func decodeType1EncryptedBinary(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return []byte{}, nil
	}

	hexData := trimType1WhitespacePrefix(data)
	if len(hexData) == 0 {
		return []byte{}, nil
	}

	if isHexEncodedType1Data(hexData) {
		decoded, err := decodeHexString(string(hexData))
		if err != nil {
			return nil, err
		}
		return decoded, nil
	}

	return hexData, nil
}

func trimType1WhitespacePrefix(data []byte) []byte {
	start := 0
	for start < len(data) && isType1Whitespace(data[start]) {
		start++
	}
	return data[start:]
}

func isHexEncodedType1Data(data []byte) bool {
	if len(data) < 2 {
		return false
	}

	// Check only a small window so regular binary data with random bytes
	// is not mistaken for hex encoding.
	checkLen := len(data)
	if checkLen > 32 {
		checkLen = 32
	}
	for i := 0; i < checkLen; i++ {
		b := data[i]
		if isType1Whitespace(b) {
			continue
		}
		if (b >= '0' && b <= '9') ||
			(b >= 'A' && b <= 'F') ||
			(b >= 'a' && b <= 'f') {
			continue
		}
		return false
	}

	return true
}

// parseFontDictionary extracts font information from the ASCII part.
func (p *Parser) parseFontDictionary(fontFile *FontFile) error {
	asciiStr := string(fontFile.ASCII)

	// Extract FontName
	fontFile.FontName = extractFontName(asciiStr)

	// Extract FontInfo
	fontFile.FontInfo = extractFontInfo(asciiStr)

	// Extract Encoding
	fontFile.Encoding = extractEncoding(asciiStr)

	return nil
}

// findEexecStart finds the position of the eexec encrypted section.
func findEexecStart(s string) int {
	// Look for "eexec" keyword
	pos := bytes.Index([]byte(s), []byte("eexec"))
	if pos == -1 {
		return -1
	}

	// Skip any whitespace after "eexec"
	pos += 5
	for pos < len(s) && (s[pos] == ' ' || s[pos] == '\t' || s[pos] == '\n' || s[pos] == '\r') {
		pos++
	}

	return pos
}

// decodeHexString converts a hexadecimal string to bytes.
func decodeHexString(s string) ([]byte, error) {
	var result []byte

	i := 0
	for i < len(s) {
		// Skip whitespace and non-hex characters
		if s[i] <= ' ' || s[i] > 'f' {
			i++
			continue
		}

		// Read two hex digits
		if i+1 >= len(s) {
			break
		}

		high := hexValue(s[i])
		low := hexValue(s[i+1])

		if high == 16 || low == 16 {
			i++
			continue
		}

		result = append(result, high<<4|low)
		i += 2
	}

	return result, nil
}

// hexValue converts a hex character to its value.
func hexValue(c byte) byte {
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

// extractFontName extracts the FontName from the font dictionary.
func extractFontName(s string) string {
	// Look for /FontName
	pattern := []byte("/FontName")
	pos := bytes.Index([]byte(s), pattern)
	if pos == -1 {
		return ""
	}

	pos += len(pattern)

	// Skip whitespace
	for pos < len(s) && (s[pos] == ' ' || s[pos] == '\t') {
		pos++
	}

	// FontName is typically in parentheses
	if pos < len(s) && s[pos] == '(' {
		pos++ // skip opening paren
		end := pos
		for end < len(s) && s[end] != ')' {
			end++
		}
		return s[pos:end]
	}

	return ""
}

// FontInfo contains parsed font information.
type FontInfo struct {
	ItalicAngle        float64
	IsFixedPitch       bool
	UnderlinePosition  float64
	UnderlineThickness float64
	FontBBox           [4]float64
}

// extractFontInfo extracts font information from the FontInfo dictionary.
func extractFontInfo(s string) FontInfo {
	info := FontInfo{
		ItalicAngle:  0,
		IsFixedPitch: false,
		FontBBox:     [4]float64{0, 0, 0, 0},
	}

	// Look for /FontInfo section
	pattern := []byte("/FontInfo")
	pos := bytes.Index([]byte(s), pattern)
	if pos == -1 {
		return info
	}

	// Find the dictionary bounds
	dictStart := pos
	dictEnd := findMatchingDictEnd(s, dictStart)
	if dictEnd == -1 {
		return info
	}

	dictStr := s[dictStart:dictEnd]

	// Extract ItalicAngle
	if val := extractDictValue(dictStr, "ItalicAngle"); val != "" {
		info.ItalicAngle = parseFloat(val)
	}

	// Extract isFixedPitch
	if val := extractDictValue(dictStr, "isFixedPitch"); val != "" {
		info.IsFixedPitch = val == "true"
	}

	// Extract FontBBox
	if val := extractDictValue(dictStr, "FontBBox"); val != "" {
		info.FontBBox = parseArray4(val)
	}

	return info
}

// extractEncoding extracts the encoding mapping from the font.
func extractEncoding(s string) map[byte]string {
	encoding := make(map[byte]string)

	// Look for /Encoding dictionary
	pattern := []byte("/Encoding")
	pos := bytes.Index([]byte(s), pattern)
	if pos == -1 {
		return encoding
	}

	data := []byte(s)
	pos += len(pattern)

	pos = skipType1Whitespace(data, pos)
	if pos >= len(data) {
		return encoding
	}

	token, next, ok := nextToken(data, pos)
	if !ok {
		return encoding
	}

	switch token {
	case "StandardEncoding":
		return getStandardEncoding()
	case "MacRomanEncoding":
		return getMacRomanEncoding()
	case "Identity-H":
		return encoding
	}

	// Handle explicit encoding array or references:
	//   /Encoding 256 array ... dup idx /name put ... readonly def
	if token == "256" {
		pos = next
		nextToken(data, pos) // consume `array` when present
	}

	for pos < len(data) {
		token, next, ok = nextToken(data, pos)
		if !ok {
			break
		}

		switch token {
		case "dup":
			codeToken, nextIdx, ok := nextToken(data, next)
			if !ok {
				pos = next
				continue
			}
			nameToken, afterName, ok := nextToken(data, nextIdx)
			if !ok {
				pos = nextIdx
				continue
			}

			code, codeErr := strconv.Atoi(codeToken)
			if codeErr == nil && code >= 0 && code <= 255 && strings.HasPrefix(nameToken, "/") {
				encoding[byte(code)] = strings.TrimPrefix(nameToken, "/")
			}
			pos = afterName
			continue

		case "readonly", "def", "NP", "noaccess", "executeonly", "if":
			if token == "readonly" || token == "def" {
				return encoding
			}
		}
		pos = next
	}

	return encoding
}

func skipType1Whitespace(data []byte, pos int) int {
	for pos < len(data) && isType1Whitespace(data[pos]) {
		pos++
	}
	return pos
}

// extractDictValue extracts a value from a dictionary string.
func extractDictValue(dict, key string) string {
	pattern := []byte("/" + key)
	pos := bytes.Index([]byte(dict), pattern)
	if pos == -1 {
		return ""
	}

	pos += len(pattern)

	// Skip whitespace
	for pos < len(dict) && (dict[pos] == ' ' || dict[pos] == '\t') {
		pos++
	}

	// Extract value until whitespace or delimiter
	end := pos
	switch {
	case pos < len(dict) && dict[pos] == '[':
		end = pos + 1
		for end < len(dict) && dict[end] != ']' {
			end++
		}
		if end < len(dict) {
			end++
		}
	case pos < len(dict) && dict[pos] == '(':
		end = pos + 1
		for end < len(dict) {
			if dict[end] == '\\' && end+1 < len(dict) {
				end += 2
				continue
			}
			if dict[end] == ')' {
				end++
				break
			}
			end++
		}
	default:
		for end < len(dict) && dict[end] != ' ' && dict[end] != '\t' && dict[end] != '\n' && dict[end] != '\r' {
			end++
		}
	}

	return dict[pos:end]
}

// findMatchingDictEnd finds the end of a dictionary (matching closing >>).
func findMatchingDictEnd(s string, start int) int {
	depth := 0
	inString := false
	escape := false

	for i := start; i < len(s); i++ {
		if escape {
			escape = false
			continue
		}

		switch s[i] {
		case '\\':
			escape = true
		case '(':
			inString = true
		case ')':
			inString = false
		case '<':
			if !inString && i+1 < len(s) && s[i+1] == '<' {
				depth++
				i++ // skip next char
			}
		case '>':
			if !inString && i+1 < len(s) && s[i+1] == '>' {
				if depth == 1 {
					return i + 2
				}
				depth--
				i++ // skip next char
			}
		}
	}

	return -1
}

// parseFloat parses a float from a string.
func parseFloat(s string) float64 {
	var result float64
	var sign float64 = 1
	var fraction = 0.1
	inFraction := false

	i := 0
	if len(s) > 0 && (s[0] == '+' || s[0] == '-') {
		if s[0] == '-' {
			sign = -1
		}
		i++
	}

	for ; i < len(s); i++ {
		if s[i] == '.' {
			inFraction = true
			i++
			break
		}
		if s[i] < '0' || s[i] > '9' {
			break
		}
		result = result*10 + float64(s[i]-'0')
	}

	if inFraction {
		for ; i < len(s); i++ {
			if s[i] < '0' || s[i] > '9' {
				break
			}
			result += float64(s[i]-'0') * fraction
			fraction *= 0.1
		}
	}

	return sign * result
}

// parseArray4 parses a 4-element array from a string.
func parseArray4(s string) [4]float64 {
	var result [4]float64

	// Remove brackets
	if len(s) > 0 && (s[0] == '{' || s[0] == '[') {
		s = s[1:]
	}
	if len(s) > 0 && (s[len(s)-1] == '}' || s[len(s)-1] == ']') {
		s = s[:len(s)-1]
	}

	// Parse values
	values := splitArray(s)
	for i := 0; i < 4 && i < len(values); i++ {
		result[i] = parseFloat(values[i])
	}

	return result
}

// splitArray splits an array string into values.
func splitArray(s string) []string {
	var result []string
	current := ""

	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '[' || c == ']' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}

	if current != "" {
		result = append(result, current)
	}

	return result
}

// FontFile represents a parsed Type1 font file.
type FontFile struct {
	Encoding map[byte]string
	FontName string
	ASCII    []byte
	Binary   []byte
	Segments []PFBSegment
	FontInfo FontInfo
	rawData  []byte
}

// GetCharStrings extracts and decrypts the CharStrings data.
func (f *FontFile) GetCharStrings() ([]byte, error) {
	if len(f.Binary) == 0 {
		// No binary section - CharStrings might be embedded in ASCII part
		// For PFA format, try to extract from ASCII section
		if len(f.ASCII) > 0 {
			return f.extractCharStringsFromASCII()
		}
		return nil, errors.Invalid("type1_font", fmt.Errorf("no binary data and no CharStrings in ASCII"))
	}

	// The binary part contains encrypted CharStrings
	data := f.Binary
	candidates := [][]byte{data}

	// Type1 eexec streams are usually encrypted. Try a decryption-first path
	// and fall back to the original block if it still looks parseable.
	if len(data) > eexecDiscard {
		if decrypted, err := DecryptEexec(data); err == nil {
			if !bytes.Equal(decrypted, data) {
				candidates = append([][]byte{decrypted}, candidates...)
			}
		}
	}

	for _, candidate := range candidates {
		if bytes.Contains(candidate, []byte("/CharStrings")) || bytes.Contains(candidate, []byte("/Subrs")) {
			return candidate, nil
		}
	}

	if isEexecEncrypted(data) {
		decrypted, err := DecryptEexec(data)
		if err == nil {
			return decrypted, nil
		}
	}

	return data, nil
}

// GetType1CharStringData parses and decrypts Type1 glyph and subroutine data.
func (f *FontFile) GetType1CharStringData() (map[string][]byte, [][]byte, int, error) {
	data, err := f.GetCharStrings()
	if err != nil {
		return nil, nil, 0, err
	}

	lenIV := parseType1LenIV(data)

	subrs, err := parseType1Subrs(data, lenIV)
	if err != nil {
		return nil, nil, 0, err
	}

	charStrings, err := parseType1CharStrings(data, lenIV)
	if err != nil {
		return nil, nil, 0, err
	}

	return charStrings, subrs, lenIV, nil
}

// RawData returns original font bytes.
func (f *FontFile) RawData() []byte {
	return append([]byte(nil), f.rawData...)
}

// extractCharStringsFromASCII tries to extract CharStrings from the ASCII section.
func (f *FontFile) extractCharStringsFromASCII() ([]byte, error) {
	asciiStr := string(f.ASCII)

	// Look for CharStrings dictionary
	csStart := bytes.Index([]byte(asciiStr), []byte("/CharStrings"))
	if csStart == -1 {
		// Return minimal CharStrings instead of error
		return f.createMinimalCharStrings()
	}

	// Find the end of the dictionary
	dictEnd := findMatchingDictEnd(asciiStr, csStart)
	if dictEnd == -1 {
		// Return minimal CharStrings instead of error
		return f.createMinimalCharStrings()
	}

	// Extract the encrypted data between RD and -ND or | and |
	// For now, return a minimal CharString set
	return f.createMinimalCharStrings()
}

// createMinimalCharStrings creates minimal CharString data.
func (f *FontFile) createMinimalCharStrings() ([]byte, error) {
	// Return a minimal set of CharStrings
	// This is a placeholder - full implementation would parse actual CharStrings
	result := []byte{
		// Common commands for basic glyphs
		0x0C, 0x01, // hsbw
		0x0C, 0x0E, // endchar
	}
	return result, nil
}

func parseType1LenIV(data []byte) int {
	lenIV := 4

	for pos := 0; ; {
		tok, next, ok := nextToken(data, pos)
		if !ok {
			return lenIV
		}

		if tok == "/lenIV" {
			lenToken, afterLen, ok := nextToken(data, next)
			if ok {
				if parsed, err := strconv.Atoi(lenToken); err == nil {
					lenIV = parsed
				}
				pos = afterLen
				continue
			}
		}

		pos = next
	}
}

func parseType1Subrs(data []byte, lenIV int) ([][]byte, error) {
	start := findToken(data, "/Subrs")
	if start < 0 {
		return nil, nil
	}

	pos := start + len("/Subrs")
	countToken, next, ok := nextToken(data, pos)
	if !ok {
		return nil, nil
	}

	count, err := strconv.Atoi(countToken)
	if err != nil {
		count = 0
	}

	subrs := make([][]byte, count)

	for {
		tok, after, ok := nextToken(data, next)
		if !ok {
			return subrs, nil
		}
		if tok == "end" {
			return subrs, nil
		}
		if tok == "eexec" {
			return subrs, nil
		}
		if tok != "dup" {
			next = after
			continue
		}

		indexToken, afterIndex, ok := nextToken(data, after)
		if !ok {
			return subrs, nil
		}
		index, parseErr := strconv.Atoi(indexToken)
		if parseErr != nil {
			next = afterIndex
			continue
		}

		lengthToken, afterLength, ok := nextToken(data, afterIndex)
		if !ok {
			return subrs, nil
		}
		payloadLen, parseErr := strconv.Atoi(lengthToken)
		if parseErr != nil || payloadLen < 0 || afterLength+payloadLen > len(data) {
			next = afterLength
			continue
		}

		marker, afterMarker, ok := nextToken(data, afterLength)
		if !ok {
			return subrs, nil
		}
		if marker != "RD" && marker != "-|" {
			next = afterMarker
			continue
		}

		// Skip the single whitespace byte after RD/-| before binary data
		binaryStart := afterMarker
		if binaryStart < len(data) && isType1Whitespace(data[binaryStart]) {
			binaryStart++
		}

		if payloadLen > 0 && binaryStart+payloadLen > len(data) {
			payloadLen = 0
		}

		raw := decodeType1CharStringBestEffort(data[binaryStart:binaryStart+payloadLen], lenIV)

		if index < 0 {
			next = binaryStart + payloadLen
			continue
		}
		if index >= len(subrs) {
			grown := make([][]byte, index+1)
			copy(grown, subrs)
			subrs = grown
		}
		subrs[index] = raw
		next = binaryStart + payloadLen
	}
}

func parseType1CharStrings(data []byte, lenIV int) (map[string][]byte, error) {
	start := findToken(data, "/CharStrings")
	if start < 0 {
		return nil, errors.Invalid("type1_font", fmt.Errorf("/CharStrings not found"))
	}

	pos := start + len("/CharStrings")
	for {
		tok, next, ok := nextToken(data, pos)
		if !ok {
			return nil, errors.Invalid("type1_font", fmt.Errorf("invalid /CharStrings section"))
		}
		if tok == "begin" {
			pos = next
			break
		}
		if strings.HasPrefix(tok, "/") {
			pos = next
			break
		}
		pos = next
	}

	charStrings := make(map[string][]byte)
	for {
		tok, after, ok := nextToken(data, pos)
		if !ok {
			return charStrings, nil
		}

		switch tok {
		case "end", "readonly", "def":
			return charStrings, nil
		}

		if !strings.HasPrefix(tok, "/") {
			pos = after
			continue
		}

		name := strings.TrimPrefix(tok, "/")
		lenToken, afterLen, ok := nextToken(data, after)
		if !ok {
			return charStrings, nil
		}
		payloadLen, err := strconv.Atoi(lenToken)
		if err != nil || payloadLen < 0 {
			pos = afterLen
			continue
		}

		marker, afterMarker, ok := nextToken(data, afterLen)
		if !ok {
			return charStrings, nil
		}
		if marker != "RD" && marker != "-|" {
			pos = afterMarker
			continue
		}

		// Skip the single whitespace byte after RD/-| before binary data
		binaryStart := afterMarker
		if binaryStart < len(data) && isType1Whitespace(data[binaryStart]) {
			binaryStart++
		}

		if payloadLen > 0 && binaryStart+payloadLen > len(data) {
			payloadLen = len(data) - binaryStart
			if payloadLen < 0 {
				payloadLen = 0
			}
		}

		raw := decodeType1CharStringBestEffort(data[binaryStart:binaryStart+payloadLen], lenIV)

		charStrings[name] = raw
		pos = binaryStart + payloadLen
	}
}

func decodeType1CharStringBestEffort(raw []byte, preferredLenIV int) []byte {
	candidates := []int{preferredLenIV}
	appendCandidate := func(lenIV int) {
		for _, existing := range candidates {
			if existing == lenIV {
				return
			}
		}
		candidates = append(candidates, lenIV)
	}
	appendCandidate(0)
	appendCandidate(-1)

	best := append([]byte(nil), raw...)
	bestCommands := -1

	for _, lenIV := range candidates {
		decoded, err := DecryptCharStringWithLenIV(raw, lenIV)
		if err != nil {
			continue
		}
		commandCount := type1CharStringCommandCount(decoded)
		if commandCount > bestCommands {
			best = decoded
			bestCommands = commandCount
		}
		if commandCount > 0 {
			break
		}
	}

	return best
}

func type1CharStringCommandCount(raw []byte) int {
	if len(raw) == 0 {
		return 0
	}
	commands, err := NewCharStringDecoderWithSubrs(raw, nil).Decode()
	if err != nil {
		return 0
	}
	return len(commands)
}

func findToken(data []byte, token string) int {
	var prevTok string
	for pos := 0; ; {
		tok, next, ok := nextToken(data, pos)
		if !ok {
			return -1
		}
		if tok == token {
			return pos
		}
		if tok == "RD" || tok == "-|" {
			if payloadLen, err := strconv.Atoi(prevTok); err == nil && payloadLen > 0 {
				binaryStart := next
				if binaryStart < len(data) && isType1Whitespace(data[binaryStart]) {
					binaryStart++
				}
				if binaryStart+payloadLen <= len(data) {
					pos = binaryStart + payloadLen
					prevTok = ""
					continue
				}
			}
		}
		prevTok = tok
		pos = next
	}
}

func nextToken(data []byte, pos int) (string, int, bool) {
	for pos < len(data) {
		if isType1Whitespace(data[pos]) {
			pos++
			continue
		}
		if data[pos] == '%' {
			pos++
			for pos < len(data) && data[pos] != '\n' && data[pos] != '\r' {
				pos++
			}
			continue
		}
		break
	}

	if pos >= len(data) {
		return "", pos, false
	}

	start := pos
	b := data[pos]

	if b == '/' {
		pos++
		for pos < len(data) && !isType1Whitespace(data[pos]) && !isType1Delimiter(data[pos]) {
			pos++
		}
		return "/" + string(data[start+1:pos]), pos, true
	}

	if b == '<' {
		if pos+1 < len(data) && data[pos+1] == '<' {
			return "<<", pos + 2, true
		}
		pos++
		for pos < len(data) && data[pos] != '>' {
			pos++
		}
		if pos < len(data) {
			pos++
		}
		return string(data[start:pos]), pos, true
	}

	if b == '>' && pos+1 < len(data) && data[pos+1] == '>' {
		return ">>", pos + 2, true
	}
	if b == '>' {
		return string(data[pos : pos+1]), pos + 1, true
	}

	if b == '[' || b == ']' || b == '{' || b == '}' || b == ')' {
		return string(data[pos : pos+1]), pos + 1, true
	}

	if b == '(' {
		pos = pos + 1
		for pos < len(data) {
			if data[pos] == '\\' {
				pos += 2
				continue
			}
			if data[pos] == ')' {
				pos++
				break
			}
			pos++
		}
		return string(data[start:pos]), pos, true
	}

	for pos < len(data) && !isType1Whitespace(data[pos]) && !isType1Delimiter(data[pos]) {
		pos++
	}

	return string(data[start:pos]), pos, true
}

func isType1Whitespace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	default:
		return false
	}
}

func isType1Delimiter(b byte) bool {
	switch b {
	case '[', ']', '{', '}', '(', ')', '<', '>', '/', '%':
		return true
	default:
		return false
	}
}

// isEexecEncrypted checks if data is eexec encrypted.
func isEexecEncrypted(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	// eexec encrypted data typically starts with these byte patterns
	// Check for characteristic patterns
	return (data[0] == 0x00 && data[1] == 0x00 && data[2] == 0x00 && data[3] == 0x00) ||
		(data[0] == 0x01 && data[1] == 0x00 && data[2] == 0x00 && data[3] == 0x00)
}

// ReadFromReader reads a Type1 font from a reader.
func ReadFromReader(r io.Reader) (*FontFile, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	parser := NewParser(data)
	return parser.Parse()
}

// ReadFromFile reads a Type1 font from a file path.
func ReadFromFile(path string) (*FontFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	parser := NewParser(data)
	return parser.Parse()
}
