// Package text provides text extraction infrastructure for PDF documents.
package text

import (
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/dh-kam/pdf-go/internal/domain/content"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
	domainText "github.com/dh-kam/pdf-go/internal/domain/text"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"
	pdfstream "github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"
)

// Extractor extracts text from PDF pages.
type Extractor struct {
	preserveSpacing  bool
	includeInvisible bool
}

// NewExtractor creates a new text extractor.
func NewExtractor() *Extractor {
	return &Extractor{
		preserveSpacing:  true,
		includeInvisible: false,
	}
}

// SetPreserveSpacing sets whether to preserve spacing in extracted text.
func (e *Extractor) SetPreserveSpacing(preserve bool) {
	e.preserveSpacing = preserve
}

// SetIncludeInvisible sets whether to include invisible text.
func (e *Extractor) SetIncludeInvisible(include bool) {
	e.includeInvisible = include
}

// Extract extracts text from a page and returns a TextLayer.
func (e *Extractor) Extract(page *entity.Page) (*domainText.TextLayer, error) {
	contents, err := page.Contents()
	if err != nil {
		return nil, errors.Invalid("text_extraction", err)
	}
	if len(contents) == 0 {
		return &domainText.TextLayer{}, nil
	}

	// Get the XRef from the page's parent document
	doc := page.Document()
	if doc == nil {
		return nil, errors.Invalid("text_extraction", fmt.Errorf("page has no document"))
	}
	xref := doc.XRef()
	if xref == nil {
		return nil, errors.Invalid("text_extraction", fmt.Errorf("document has no XRef"))
	}

	// Create a text evaluator to process the content stream
	textEval := content.NewTextEvaluator(xref)
	decodedContents := make([][]byte, 0, len(contents))

	// Set resources if available
	resources, err := page.Resources()
	if err == nil && resources != nil {
		textEval.SetResources(resources)
	}

	// Decode stream objects with infrastructure decoders to handle filters.
	for _, contentObj := range contents {
		if streamObj, ok := contentObj.(*entity.Stream); ok {
			infraStream := pdfstream.NewFromEntity(streamObj)
			data, decodeErr := infraStream.Decode()
			if decodeErr != nil {
				return nil, errors.Invalid("text_extraction", decodeErr)
			}
			decodedContents = append(decodedContents, data)
			if processErr := textEval.ProcessBytes(data); processErr != nil && processErr != io.EOF {
				return nil, errors.Invalid("text_extraction", processErr)
			}
			continue
		}

		processErr := textEval.ProcessObject(contentObj)
		if processErr != nil && processErr != io.EOF {
			return nil, errors.Invalid("text_extraction", processErr)
		}
	}

	layer := textEval.GetTextLayer()
	fontMappings := buildFontMappings(resources, xref)
	if len(decodedContents) > 0 {
		// Fallback parser for text-showing operators (Tj/TJ/'/") with font-aware decoding.
		fallbackText := extractTextFallback(decodedContents, fontMappings)
		currentText := strings.TrimSpace(layer.Text())
		if fallbackText != "" && (currentText == "" || isLikelyGarbledText(currentText)) {
			layer = &domainText.TextLayer{}
			layer.AddItem(domainText.TextItem{
				Text:        fallbackText,
				Unicode:     fallbackText,
				WritingMode: domainText.WritingModeHorizontal,
			})
		}
	}
	if len(layer.GetItems()) == 0 {
		renderEval := domainrenderer.NewEvaluator(xref)
		if resources != nil {
			renderEval.SetResources(resources)
		}
		if evalErr := renderEval.Evaluate(contents); evalErr == nil {
			renderText := strings.TrimSpace(renderEval.ExtractedText())
			if renderText != "" {
				layer.AddItem(domainText.TextItem{
					Text:        renderText,
					Unicode:     renderText,
					WritingMode: domainText.WritingModeHorizontal,
				})
			}
		}
	}

	// Return the extracted text layer
	return layer, nil
}

// ExtractToText extracts text from a page and returns it as a string.
func (e *Extractor) ExtractToText(page *entity.Page) (string, error) {
	layer, err := e.Extract(page)
	if err != nil {
		return "", err
	}
	return layer.Text(), nil
}

// ExtractFromStream extracts text from a content stream.
func ExtractFromStream(stream entity.Object, xref entity.XRef) (string, error) {
	// Create a text evaluator to process the stream
	textEval := content.NewTextEvaluator(xref)

	// Process the content stream
	var err error
	if streamObj, ok := stream.(*entity.Stream); ok {
		infraStream := pdfstream.NewFromEntity(streamObj)
		data, decodeErr := infraStream.Decode()
		if decodeErr != nil {
			return "", errors.Invalid("text_extraction", decodeErr)
		}
		err = textEval.ProcessBytes(data)
	} else {
		err = textEval.ProcessObject(stream)
	}
	if err != nil && err != io.EOF {
		return "", errors.Invalid("text_extraction", err)
	}

	layer := textEval.GetTextLayer()
	if len(layer.GetItems()) == 0 {
		if streamObj, ok := stream.(*entity.Stream); ok {
			infraStream := pdfstream.NewFromEntity(streamObj)
			data, decodeErr := infraStream.Decode()
			if decodeErr == nil {
				text := extractTextFallback([][]byte{data}, nil)
				if text != "" {
					return text, nil
				}
			}
		}
	}

	// Return the extracted text
	return layer.Text(), nil
}

var textOperatorKeywords = map[string]struct{}{
	"Tj": {}, "TJ": {}, "T*": {}, "'": {}, "\"": {}, "ET": {}, "Tf": {},
}

func isTextOperatorKeyword(keyword string) bool {
	_, ok := textOperatorKeywords[keyword]
	return ok
}

type fontTextMapping struct {
	toUnicode *toUnicodeMapper
}

func extractTextFallback(contents [][]byte, fontMappings map[string]*fontTextMapping) string {
	var out strings.Builder

	for _, data := range contents {
		segment := extractTextFromBytes(data, fontMappings)
		if segment == "" {
			continue
		}
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(segment)
	}

	return strings.TrimSpace(out.String())
}

func isLikelyGarbledText(text string) bool {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return true
	}

	printableCount := 0
	for _, r := range runes {
		if r == utf8.RuneError || unicode.IsControl(r) {
			continue
		}
		if unicode.IsPrint(r) {
			printableCount++
		}
	}

	return float64(printableCount)/float64(len(runes)) < 0.80
}

func extractTextFromBytes(data []byte, fontMappings map[string]*fontTextMapping) string {
	lexer := parser.NewLexerBytes(data)
	objParser := parser.NewParser(lexer, nil)
	operands := make([]entity.Object, 0, 8)
	var out strings.Builder
	currentFont := ""

	for {
		// Flush parser-buffered operands (e.g. non-reference "num num <op>" sequences)
		// before peeking operator tokens.
		if objParser.HasBufferedObject() {
			obj, err := objParser.ParseObject()
			if err != nil {
				break
			}
			operands = append(operands, obj)
			continue
		}

		token, err := lexer.Peek()
		if err != nil || token.Type == parser.TokenEOF {
			break
		}

		if token.Type == parser.TokenKeyword && isTextOperatorKeyword(token.Value) {
			if _, err := lexer.NextToken(); err != nil {
				break
			}
			switch token.Value {
			case "Tj":
				appendTextOperand(&out, operands, 1, fontMappings, currentFont)
			case "T*":
				ensureLineBreak(&out)
			case "'":
				ensureLineBreak(&out)
				appendTextOperand(&out, operands, 1, fontMappings, currentFont)
			case "\"":
				ensureLineBreak(&out)
				appendTextOperand(&out, operands, 3, fontMappings, currentFont)
			case "TJ":
				appendTextArrayOperand(&out, operands, fontMappings, currentFont)
			case "ET":
				if out.Len() > 0 && !strings.HasSuffix(out.String(), "\n") {
					out.WriteByte('\n')
				}
			case "Tf":
				currentFont = extractCurrentFontAlias(operands)
			}
			operands = operands[:0]
			continue
		}

		obj, err := objParser.ParseObject()
		if err != nil {
			break
		}
		operands = append(operands, obj)
	}

	return strings.TrimSpace(out.String())
}

func ensureLineBreak(out *strings.Builder) {
	if out == nil || out.Len() == 0 {
		return
	}
	if !strings.HasSuffix(out.String(), "\n") {
		out.WriteByte('\n')
	}
}

func appendTextOperand(out *strings.Builder, operands []entity.Object, minOperands int, fontMappings map[string]*fontTextMapping, currentFont string) {
	if len(operands) < minOperands {
		return
	}
	for i := len(operands) - 1; i >= 0; i-- {
		if str, ok := operands[i].(*entity.String); ok {
			out.WriteString(decodeTextString(str, fontMappings, currentFont))
			return
		}
	}
}

func appendTextArrayOperand(out *strings.Builder, operands []entity.Object, fontMappings map[string]*fontTextMapping, currentFont string) {
	if len(operands) == 0 {
		return
	}
	arr, ok := operands[len(operands)-1].(*entity.Array)
	if !ok {
		return
	}
	for i := 0; i < arr.Len(); i++ {
		if item, ok := arr.Get(i).(*entity.String); ok {
			out.WriteString(decodeTextString(item, fontMappings, currentFont))
		}
	}
}

func decodeTextString(str *entity.String, fontMappings map[string]*fontTextMapping, currentFont string) string {
	if str == nil {
		return ""
	}

	raw := []byte(str.Value())
	mapping := lookupFontMapping(fontMappings, currentFont)
	if mapping != nil && mapping.toUnicode != nil {
		if decoded, ok := mapping.toUnicode.Decode(raw); ok {
			return decoded
		}
	}

	if text, ok := decodeUTF16String(raw); ok {
		return text
	}

	if utf8.Valid(raw) {
		return string(raw)
	}

	runes := make([]rune, len(raw))
	for i, b := range raw {
		runes[i] = rune(b)
	}
	return string(runes)
}

func extractCurrentFontAlias(operands []entity.Object) string {
	for i := len(operands) - 1; i >= 0; i-- {
		if name, ok := operands[i].(entity.Name); ok {
			return normalizeResourceName(name.Value())
		}
	}
	return ""
}

func lookupFontMapping(fontMappings map[string]*fontTextMapping, currentFont string) *fontTextMapping {
	if len(fontMappings) == 0 {
		return nil
	}
	if mapping, ok := fontMappings[currentFont]; ok {
		return mapping
	}
	normalized := normalizeResourceName(currentFont)
	if mapping, ok := fontMappings[normalized]; ok {
		return mapping
	}
	return nil
}

func normalizeResourceName(name string) string {
	return strings.TrimPrefix(name, "/")
}

func buildFontMappings(resources *entity.Dict, xref entity.XRef) map[string]*fontTextMapping {
	if resources == nil {
		return nil
	}

	fontObj := resources.Get(entity.Name("Font"))
	fontObj = resolveRef(fontObj, xref)

	fontDict, ok := fontObj.(*entity.Dict)
	if !ok {
		return nil
	}

	result := make(map[string]*fontTextMapping, fontDict.Len())
	for _, key := range fontDict.Keys() {
		fontEntry := resolveRef(fontDict.Get(key), xref)
		entryDict, ok := fontEntry.(*entity.Dict)
		if !ok {
			continue
		}

		toUnicodeObj := resolveRef(entryDict.Get(entity.Name("ToUnicode")), xref)
		streamObj, ok := toUnicodeObj.(*entity.Stream)
		if !ok {
			continue
		}

		infraStream := pdfstream.NewFromEntity(streamObj)
		data, err := infraStream.Decode()
		if err != nil {
			continue
		}

		mapper := parseToUnicodeCMap(data)
		if mapper == nil {
			continue
		}

		alias := normalizeResourceName(key.Value())
		result[alias] = &fontTextMapping{toUnicode: mapper}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

func resolveRef(obj entity.Object, xref entity.XRef) entity.Object {
	if obj == nil {
		return nil
	}
	ref, ok := obj.(entity.Ref)
	if !ok || xref == nil {
		return obj
	}

	resolved, err := xref.Fetch(ref)
	if err != nil {
		return obj
	}
	return resolved
}

type toUnicodeMapper struct {
	codeToUnicode map[string]string
	codeLengths   []int
}

// Decode is an exported API.
func (m *toUnicodeMapper) Decode(data []byte) (string, bool) {
	if m == nil || len(m.codeToUnicode) == 0 || len(data) == 0 {
		return "", false
	}

	var out strings.Builder
	matched := false

	for i := 0; i < len(data); {
		found := false
		for _, codeLen := range m.codeLengths {
			if i+codeLen > len(data) {
				continue
			}

			if unicodeText, ok := m.codeToUnicode[string(data[i:i+codeLen])]; ok {
				out.WriteString(unicodeText)
				i += codeLen
				matched = true
				found = true
				break
			}
		}

		if found {
			continue
		}

		out.WriteByte(data[i])
		i++
	}

	return out.String(), matched
}

func parseToUnicodeCMap(data []byte) *toUnicodeMapper {
	tokens := tokenizeCMap(data)
	if len(tokens) == 0 {
		return nil
	}

	codeToUnicode := make(map[string]string)
	codeLengthSet := make(map[int]struct{})

	for i := 0; i < len(tokens); i++ {
		switch tokens[i] {
		case "begincodespacerange":
			i = parseCodeSpaceRange(tokens, i+1, codeLengthSet)
		case "beginbfchar":
			i = parseBFChar(tokens, i+1, codeToUnicode, codeLengthSet)
		case "beginbfrange":
			i = parseBFRange(tokens, i+1, codeToUnicode, codeLengthSet)
		}
	}

	if len(codeToUnicode) == 0 {
		return nil
	}

	codeLengths := make([]int, 0, len(codeLengthSet))
	if len(codeLengthSet) == 0 {
		for src := range codeToUnicode {
			codeLengthSet[len(src)] = struct{}{}
		}
	}
	for l := range codeLengthSet {
		if l > 0 {
			codeLengths = append(codeLengths, l)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(codeLengths)))

	return &toUnicodeMapper{
		codeToUnicode: codeToUnicode,
		codeLengths:   codeLengths,
	}
}

func tokenizeCMap(data []byte) []string {
	tokens := make([]string, 0, 256)

	for i := 0; i < len(data); {
		c := data[i]

		if isCMapWhitespace(c) {
			i++
			continue
		}
		if c == '%' {
			for i < len(data) && data[i] != '\n' && data[i] != '\r' {
				i++
			}
			continue
		}
		if c == '<' {
			if i+1 < len(data) && data[i+1] == '<' {
				tokens = append(tokens, "<<")
				i += 2
				continue
			}
			j := i + 1
			for j < len(data) && data[j] != '>' {
				j++
			}
			if j >= len(data) {
				break
			}
			tokens = append(tokens, string(data[i:j+1]))
			i = j + 1
			continue
		}
		if c == '>' {
			if i+1 < len(data) && data[i+1] == '>' {
				tokens = append(tokens, ">>")
				i += 2
				continue
			}
			i++
			continue
		}
		if c == '[' || c == ']' {
			tokens = append(tokens, string(c))
			i++
			continue
		}

		j := i
		for j < len(data) && !isCMapDelimiter(data[j]) {
			j++
		}
		if j > i {
			tokens = append(tokens, string(data[i:j]))
		}
		i = j
	}

	return tokens
}

func parseCodeSpaceRange(tokens []string, start int, codeLengthSet map[int]struct{}) int {
	i := start
	for i < len(tokens) && tokens[i] != "endcodespacerange" {
		if i+1 >= len(tokens) {
			return len(tokens) - 1
		}
		if isHexToken(tokens[i]) && isHexToken(tokens[i+1]) {
			if b, ok := parseHexToken(tokens[i]); ok {
				codeLengthSet[len(b)] = struct{}{}
			}
			i += 2
			continue
		}
		i++
	}
	return i
}

func parseBFChar(tokens []string, start int, codeToUnicode map[string]string, codeLengthSet map[int]struct{}) int {
	i := start
	for i < len(tokens) && tokens[i] != "endbfchar" {
		if i+1 >= len(tokens) {
			return len(tokens) - 1
		}

		if !isHexToken(tokens[i]) || !isHexToken(tokens[i+1]) {
			i++
			continue
		}

		src, srcOK := parseHexToken(tokens[i])
		dst, dstOK := parseHexToken(tokens[i+1])
		if srcOK && dstOK {
			codeToUnicode[string(src)] = decodeUnicodeHex(dst)
			codeLengthSet[len(src)] = struct{}{}
		}
		i += 2
	}
	return i
}

func parseBFRange(tokens []string, start int, codeToUnicode map[string]string, codeLengthSet map[int]struct{}) int {
	i := start
	for i < len(tokens) && tokens[i] != "endbfrange" {
		if i+2 >= len(tokens) {
			return len(tokens) - 1
		}

		if !isHexToken(tokens[i]) || !isHexToken(tokens[i+1]) {
			i++
			continue
		}

		lowBytes, lowOK := parseHexToken(tokens[i])
		highBytes, highOK := parseHexToken(tokens[i+1])
		low, lowNumOK := parseCodeValue(lowBytes)
		high, highNumOK := parseCodeValue(highBytes)
		if !lowOK || !highOK || !lowNumOK || !highNumOK || len(lowBytes) == 0 || low > high {
			i++
			continue
		}

		codeLen := len(lowBytes)
		codeLengthSet[codeLen] = struct{}{}
		i += 2

		if i >= len(tokens) {
			return len(tokens) - 1
		}

		switch {
		case tokens[i] == "[":
			i++
			code := low
			for i < len(tokens) && tokens[i] != "]" && code <= high {
				if isHexToken(tokens[i]) {
					if dst, ok := parseHexToken(tokens[i]); ok {
						codeToUnicode[string(uint32ToFixedBytes(code, codeLen))] = decodeUnicodeHex(dst)
						code++
					}
				}
				i++
			}
			for i < len(tokens) && tokens[i] != "]" {
				i++
			}
		case isHexToken(tokens[i]):
			baseDst, ok := parseHexToken(tokens[i])
			if ok {
				for code := low; code <= high; code++ {
					delta := code - low
					codeToUnicode[string(uint32ToFixedBytes(code, codeLen))] = incrementUnicodeHex(baseDst, delta)
				}
			}
		}
	}
	return i
}

func isHexToken(token string) bool {
	return len(token) >= 2 && token[0] == '<' && token[len(token)-1] == '>' && token != "<<" && token != ">>"
}

func parseHexToken(token string) ([]byte, bool) {
	if !isHexToken(token) {
		return nil, false
	}

	hexText := token[1 : len(token)-1]
	if len(hexText)%2 != 0 {
		hexText += "0"
	}
	if hexText == "" {
		return []byte{}, true
	}

	decoded, err := hex.DecodeString(hexText)
	if err != nil {
		return nil, false
	}
	return decoded, true
}

func parseCodeValue(code []byte) (uint32, bool) {
	if len(code) == 0 || len(code) > 4 {
		return 0, false
	}

	var v uint32
	for _, b := range code {
		v = (v << 8) | uint32(b)
	}
	return v, true
}

func uint32ToFixedBytes(v uint32, width int) []byte {
	if width <= 0 {
		return nil
	}

	out := make([]byte, width)
	for i := width - 1; i >= 0; i-- {
		out[i] = byte(v & 0xFF)
		v >>= 8
	}
	return out
}

func incrementUnicodeHex(base []byte, delta uint32) string {
	if len(base) == 0 {
		return ""
	}

	if len(base)%2 == 0 {
		units := make([]uint16, len(base)/2)
		for i := 0; i < len(units); i++ {
			units[i] = uint16(base[2*i])<<8 | uint16(base[2*i+1])
		}

		last := len(units) - 1
		units[last] = uint16(uint32(units[last]) + delta)
		return string(utf16.Decode(units))
	}

	if len(base) <= 4 {
		value, ok := parseCodeValue(base)
		if ok {
			return string(rune(value + delta))
		}
	}

	return decodeUnicodeHex(base)
}

func decodeUnicodeHex(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	if len(data)%2 == 0 {
		units := make([]uint16, len(data)/2)
		for i := 0; i < len(units); i++ {
			units[i] = uint16(data[2*i])<<8 | uint16(data[2*i+1])
		}
		return string(utf16.Decode(units))
	}

	if utf8.Valid(data) {
		return string(data)
	}

	runes := make([]rune, len(data))
	for i, b := range data {
		runes[i] = rune(b)
	}
	return string(runes)
}

func decodeUTF16String(data []byte) (string, bool) {
	if len(data) < 2 {
		return "", false
	}
	if data[0] != 0xFE || data[1] != 0xFF {
		return "", false
	}

	raw := data[2:]
	if len(raw)%2 != 0 {
		raw = append(raw, 0x00)
	}
	units := make([]uint16, len(raw)/2)
	for i := 0; i < len(units); i++ {
		units[i] = uint16(raw[2*i])<<8 | uint16(raw[2*i+1])
	}
	return string(utf16.Decode(units)), true
}

func isCMapWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

func isCMapDelimiter(c byte) bool {
	return isCMapWhitespace(c) || c == '<' || c == '>' || c == '[' || c == ']' || c == '%'
}
