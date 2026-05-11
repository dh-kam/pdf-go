// Package content provides content stream evaluation interfaces for PDF rendering.
package content

import (
	"fmt"
	"image"
	"io"
	"unicode/utf8"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
	"github.com/dh-kam/pdf-go/internal/domain/graphics"
	domainText "github.com/dh-kam/pdf-go/internal/domain/text"
	domainunicode "github.com/dh-kam/pdf-go/internal/domain/unicode"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/standard"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"
)

// TextEvaluator extracts text from PDF content streams.
type TextEvaluator struct {
	xref        entity.XRef
	textFont    entity.Font
	registry    *OperatorRegistry
	state       *graphics.State
	objects     map[entity.Ref]entity.Object
	textLayer   *domainText.TextLayer
	resources   *entity.Dict
	lineMatrix  [6]float64
	textMatrix  [6]float64
	textSize    float64
	charSpacing float64
	wordSpacing float64
	horizScale  float64
	textLeading float64
	textRise    float64
	renderMode  int
	inTextBlock bool
}

// NewTextEvaluator creates a new text extraction evaluator.
func NewTextEvaluator(xref entity.XRef) *TextEvaluator {
	te := &TextEvaluator{
		registry:   NewOperatorRegistry(),
		state:      graphics.NewState(),
		objects:    make(map[entity.Ref]entity.Object),
		xref:       xref,
		textLayer:  &domainText.TextLayer{},
		textMatrix: [6]float64{1, 0, 0, 1, 0, 0},
		lineMatrix: [6]float64{1, 0, 0, 1, 0, 0},
		horizScale: 100.0,
	}

	// Register text extraction operators
	te.registerTextOperators()

	return te
}

// registerTextOperators registers operators for text extraction.
func (e *TextEvaluator) registerTextOperators() {
	// Text block operators
	e.registry.Register("BT", &textBeginOperator{te: e})
	e.registry.Register("ET", &textEndOperator{te: e})

	// Text state operators
	e.registry.Register("Tc", &textSetCharSpacing{te: e})
	e.registry.Register("Tw", &textSetWordSpacing{te: e})
	e.registry.Register("Tz", &textSetHorizScaling{te: e})
	e.registry.Register("TL", &textSetTextLeading{te: e})
	e.registry.Register("Tf", &textSetFont{te: e})
	e.registry.Register("Tr", &textSetRenderMode{te: e})
	e.registry.Register("Ts", &textSetTextRise{te: e})

	// Text positioning operators
	e.registry.Register("Td", &textMoveText{te: e})
	e.registry.Register("TD", &textMoveTextSetLeading{te: e})
	e.registry.Register("Tm", &textSetTextMatrix{te: e})
	e.registry.Register("T*", &textNextLine{te: e})

	// Text showing operators
	e.registry.Register("Tj", &textShowText{te: e})
	e.registry.Register("TJ", &textShowTextArray{te: e})
	e.registry.Register("'", &textNextLineShow{te: e})
	e.registry.Register("\"", &textSetSpacingShow{te: e})
}

// GetRegistry returns the operator registry.
func (e *TextEvaluator) GetRegistry() *OperatorRegistry {
	return e.registry
}

// GetState returns the current graphics state.
func (e *TextEvaluator) GetState() *graphics.State {
	return e.state
}

// SetState sets the graphics state.
func (e *TextEvaluator) SetState(state *graphics.State) {
	e.state = state
}

// SetResources sets the resource dictionary.
func (e *TextEvaluator) SetResources(resources *entity.Dict) {
	e.resources = resources
}

// ProcessObject processes a content stream or object for text extraction.
func (e *TextEvaluator) ProcessObject(obj entity.Object) error {
	if obj == nil {
		return nil
	}

	switch o := obj.(type) {
	case *entity.Stream:
		return e.ProcessStream(o)
	case *entity.Dict:
		return e.ProcessDict(o)
	case *entity.Array:
		return e.ProcessArray(o)
	case entity.Ref:
		if e.xref == nil {
			return errors.NotFoundf("content_ref", "xref for ref %d", o.Num())
		}
		resolved, err := e.xref.Fetch(o)
		if err != nil {
			return errors.Invalid("content_ref", err)
		}
		return e.ProcessObject(resolved)
	}
	return nil
}

// ProcessStream processes a content stream.
func (e *TextEvaluator) ProcessStream(stream *entity.Stream) error {
	data, err := stream.Decode()
	if err != nil {
		return err
	}

	return e.ProcessBytes(data)
}

// ProcessBytes processes raw content stream bytes.
func (e *TextEvaluator) ProcessBytes(data []byte) error {
	lexer := parser.NewLexerBytes(data)
	objParser := parser.NewParser(lexer, e.xref)
	operands := make([]entity.Object, 0, 8)

	for {
		// Flush parser-buffered operands (e.g. non-reference "num num <op>" sequences)
		// before peeking the next operator token.
		if objParser.HasBufferedObject() {
			obj, err := objParser.ParseObject()
			if err != nil {
				if err == io.EOF {
					break
				}
				return errors.Invalid("content_parse", err)
			}
			operands = append(operands, obj)
			continue
		}

		token, err := lexer.Peek()
		if err != nil {
			if err == io.EOF {
				break
			}
			return errors.Invalid("content_parse", err)
		}
		if token.Type == parser.TokenEOF {
			break
		}

		if token.Type == parser.TokenKeyword && e.isOperatorToken(token.Value) {
			if _, err := lexer.NextToken(); err != nil {
				return errors.Invalid("content_parse", err)
			}
			if err := e.executeOperatorWithObjects(token.Value, operands); err != nil {
				return err
			}
			operands = operands[:0]
			continue
		}

		obj, err := objParser.ParseObject()
		if err != nil {
			return errors.Invalid("content_parse", err)
		}
		operands = append(operands, obj)
	}

	return nil
}

// ProcessDict processes a dictionary (for Form XObjects).
func (e *TextEvaluator) ProcessDict(dict *entity.Dict) error {
	if dict == nil {
		return nil
	}

	// Temporarily override resources when dictionary provides local resources.
	oldResources := e.resources
	if resourcesObj := dict.Get(entity.Name("Resources")); resourcesObj != nil {
		switch v := resourcesObj.(type) {
		case *entity.Dict:
			e.resources = v
		case entity.Ref:
			if e.xref != nil {
				resolved, err := e.xref.Fetch(v)
				if err == nil {
					if resourcesDict, ok := resolved.(*entity.Dict); ok {
						e.resources = resourcesDict
					}
				}
			}
		}
	}
	defer func() {
		e.resources = oldResources
	}()

	contents := dict.Get(entity.Name("Contents"))
	if contents == nil {
		return nil
	}
	return e.ProcessObject(contents)
}

// GetResources returns the currently active resource dictionary.
func (e *TextEvaluator) GetResources() *entity.Dict {
	return e.resources
}

// GetFont returns the currently active text font.
func (e *TextEvaluator) GetFont() entity.Font {
	return e.textFont
}

// SetFont sets the current text font for decoding.
func (e *TextEvaluator) SetFont(font entity.Font) {
	e.textFont = font
}

// DecodeText decodes text bytes using the active evaluator font.
func (e *TextEvaluator) DecodeText(data []byte) string {
	return e.decodeText(data)
}

// AddTextItem appends one text item at evaluator position.
func (e *TextEvaluator) AddTextItem(text string, x, y float64) {
	e.addTextItem(text, x, y)
}

// ProcessArray processes an array.
func (e *TextEvaluator) ProcessArray(arr *entity.Array) error {
	for i := 0; i < arr.Len(); i++ {
		obj := arr.Get(i)
		if obj == nil {
			continue
		}
		if err := e.ProcessObject(obj); err != nil {
			return err
		}
	}
	return nil
}

// ExecuteOperator executes an operator with its operands.
func (e *TextEvaluator) ExecuteOperator(name string, operands []float64) error {
	op, ok := e.registry.Get(name)
	if !ok {
		return nil
	}
	return op.Execute(e.state, operands)
}

func (e *TextEvaluator) isOperatorToken(keyword string) bool {
	if _, ok := e.registry.Get(keyword); ok {
		return true
	}

	switch keyword {
	case "Tj", "TJ", "'", "\"":
		return true
	default:
		return false
	}
}

func (e *TextEvaluator) executeOperatorWithObjects(name string, operands []entity.Object) error {
	switch name {
	case "Tj":
		return e.handleShowTextOperand(operands)
	case "TJ":
		return e.handleShowTextArrayOperand(operands)
	case "'":
		if err := e.ExecuteOperator("T*", nil); err != nil {
			return err
		}
		return e.handleShowTextOperand(operands)
	case "\"":
		return e.handleSetSpacingAndShowText(operands)
	case "Tf":
		e.handleSetFont(operands)
		return nil
	default:
		return e.ExecuteOperator(name, objectOperandsToFloat64(operands))
	}
}

func objectOperandsToFloat64(operands []entity.Object) []float64 {
	values := make([]float64, 0, len(operands))
	for _, obj := range operands {
		if value, ok := numberFromObject(obj); ok {
			values = append(values, value)
		}
	}
	return values
}

func numberFromObject(obj entity.Object) (float64, bool) {
	switch v := obj.(type) {
	case *entity.Integer:
		return float64(v.Value()), true
	case *entity.Real:
		return v.Value(), true
	default:
		return 0, false
	}
}

func (e *TextEvaluator) handleSetFont(operands []entity.Object) {
	if len(operands) == 0 {
		return
	}

	fontName := ""
	for _, operand := range operands {
		if name, ok := operand.(entity.Name); ok {
			fontName = name.Value()
			break
		}
	}

	for i := len(operands) - 1; i >= 0; i-- {
		if size, ok := numberFromObject(operands[i]); ok {
			e.textSize = size
			break
		}
	}

	if fontName == "" {
		return
	}
	e.resolveStandardFont(fontName)
}

func (e *TextEvaluator) resolveStandardFont(alias string) {
	resolvedAlias := normalizeResourceName(alias)
	if e.resources == nil {
		return
	}

	fontsObj := e.resources.Get(entity.Name("Font"))
	if fontsObj == nil {
		fontsObj = e.resources.Get(entity.Name("/Font"))
	}

	var fontsDict *entity.Dict
	switch v := fontsObj.(type) {
	case *entity.Dict:
		fontsDict = v
	case entity.Ref:
		if e.xref != nil {
			if resolved, err := e.xref.Fetch(v); err == nil {
				if dict, ok := resolved.(*entity.Dict); ok {
					fontsDict = dict
				}
			}
		}
	}
	if fontsDict == nil {
		return
	}

	fontObj := fontsDict.Get(entity.Name(resolvedAlias))
	if fontObj == nil {
		fontObj = fontsDict.Get(entity.Name("/" + resolvedAlias))
	}
	if fontObj == nil {
		return
	}

	var fontDict *entity.Dict
	switch v := fontObj.(type) {
	case *entity.Dict:
		fontDict = v
	case entity.Ref:
		if e.xref != nil {
			if resolved, err := e.xref.Fetch(v); err == nil {
				if dict, ok := resolved.(*entity.Dict); ok {
					fontDict = dict
				}
			}
		}
	}
	if fontDict == nil {
		return
	}

	baseFont := ""
	if baseObj := fontDict.Get(entity.Name("BaseFont")); baseObj != nil {
		if name, ok := baseObj.(entity.Name); ok {
			baseFont = normalizeResourceName(name.Value())
		}
	}
	if baseFont == "" {
		baseFont = resolvedAlias
	}

	if stdFont, ok := standard.GetFont(baseFont); ok {
		e.textFont = stdFont
	}
}

func (e *TextEvaluator) handleSetSpacingAndShowText(operands []entity.Object) error {
	if len(operands) < 3 {
		return nil
	}

	if wordSpacing, ok := numberFromObject(operands[0]); ok {
		e.wordSpacing = wordSpacing
	}
	if charSpacing, ok := numberFromObject(operands[1]); ok {
		e.charSpacing = charSpacing
	}

	if err := e.ExecuteOperator("T*", nil); err != nil {
		return err
	}

	textObj, ok := operands[len(operands)-1].(*entity.String)
	if !ok {
		return nil
	}
	e.addTextFromString(textObj)
	return nil
}

func (e *TextEvaluator) handleShowTextOperand(operands []entity.Object) error {
	if len(operands) == 0 {
		return nil
	}

	for i := len(operands) - 1; i >= 0; i-- {
		if textObj, ok := operands[i].(*entity.String); ok {
			e.addTextFromString(textObj)
			return nil
		}
	}
	return nil
}

func (e *TextEvaluator) handleShowTextArrayOperand(operands []entity.Object) error {
	if len(operands) == 0 {
		return nil
	}

	arr, ok := operands[len(operands)-1].(*entity.Array)
	if !ok {
		return nil
	}

	for i := 0; i < arr.Len(); i++ {
		item := arr.Get(i)
		if item == nil {
			continue
		}
		switch v := item.(type) {
		case *entity.String:
			e.addTextFromString(v)
		case *entity.Integer:
			e.applyTextAdjustment(float64(v.Value()))
		case *entity.Real:
			e.applyTextAdjustment(v.Value())
		}
	}
	return nil
}

func (e *TextEvaluator) addTextFromString(strObj *entity.String) {
	if strObj == nil || !e.inTextBlock {
		return
	}

	raw := []byte(strObj.Value())
	text := e.decodeText(raw)
	if text == "" {
		return
	}

	x := e.textMatrix[4]
	y := e.textMatrix[5] + e.textRise
	e.addTextItem(text, x, y)
	e.advanceTextMatrix(raw, text)
}

func (e *TextEvaluator) advanceTextMatrix(raw []byte, decoded string) {
	advance := e.estimateTextAdvance(raw, decoded)
	if advance == 0 {
		return
	}
	tm := [6]float64{1, 0, 0, 1, advance, 0}
	e.textMatrix = multiplyMatrix(tm, e.textMatrix)
}

func (e *TextEvaluator) estimateTextAdvance(raw []byte, decoded string) float64 {
	fontSize := e.textSize
	if fontSize == 0 {
		fontSize = 12
	}

	hScale := e.horizScale / 100.0
	if hScale == 0 {
		hScale = 1.0
	}

	if e.textFont == nil || len(raw) == 0 {
		return float64(utf8.RuneCountInString(decoded)) * fontSize * 0.5 * hScale
	}

	unitsPerEm := float64(e.textFont.UnitsPerEm())
	if unitsPerEm <= 0 {
		unitsPerEm = 1000
	}

	var codes []uint32
	if e.textFont.IsCIDFont() {
		codes = make([]uint32, 0, len(raw)/2+1)
		for i := 0; i+1 < len(raw); i += 2 {
			codes = append(codes, uint32(raw[i])<<8|uint32(raw[i+1]))
		}
		if len(raw)%2 == 1 {
			codes = append(codes, uint32(raw[len(raw)-1]))
		}
	} else {
		codes = make([]uint32, len(raw))
		for i, b := range raw {
			codes[i] = uint32(b)
		}
	}

	textAdvance := 0.0
	for _, code := range codes {
		width := 500.0
		glyphID, err := e.textFont.CharCodeToGlyph(code)
		if err == nil {
			if glyphWidth, widthErr := e.textFont.GetGlyphWidth(glyphID); widthErr == nil {
				width = glyphWidth
			}
		}
		textAdvance += (width / unitsPerEm) * fontSize
		textAdvance += e.charSpacing
		if code == uint32(' ') {
			textAdvance += e.wordSpacing
		}
	}

	return textAdvance * hScale
}

func (e *TextEvaluator) applyTextAdjustment(adjustment float64) {
	fontSize := e.textSize
	if fontSize == 0 {
		fontSize = 12
	}
	hScale := e.horizScale / 100.0
	if hScale == 0 {
		hScale = 1.0
	}
	tx := -adjustment / 1000.0 * fontSize * hScale
	tm := [6]float64{1, 0, 0, 1, tx, 0}
	e.textMatrix = multiplyMatrix(tm, e.textMatrix)
}

func normalizeResourceName(name string) string {
	if name == "" {
		return ""
	}
	if name[0] == '/' {
		return name[1:]
	}
	return name
}

// GetTextLayer returns the extracted text layer.
func (e *TextEvaluator) GetTextLayer() *domainText.TextLayer {
	return e.textLayer
}

// addTextItem adds a text item to the text layer.
func (e *TextEvaluator) addTextItem(text string, x, y float64) {
	if text == "" {
		return
	}

	fontName := "unknown"
	if e.textFont != nil {
		fontName = e.textFont.Name()
	}

	item := domainText.TextItem{
		Text:        text,
		Unicode:     text,
		Font:        fontName,
		FontSize:    e.textSize,
		BoundingBox: image.Rect(int(x), int(y), int(x)+100, int(y)+int(e.textSize)), // Simplified bounding box
		WritingMode: domainText.WritingModeHorizontal,
	}

	e.textLayer.AddItem(item)
}

// decodeText decodes text from a byte string using font encoding.
func (e *TextEvaluator) decodeText(data []byte) string {
	if e.textFont == nil {
		// Try to decode as UTF-8 or Latin-1
		if utf8.Valid(data) {
			return string(data)
		}
		// Fall back to Latin-1
		result := make([]rune, len(data))
		for i, b := range data {
			result[i] = rune(b)
		}
		return string(result)
	}

	result := make([]rune, 0, len(data))
	appendDecodedRune := func(code uint32) {
		// Try to get glyph from font
		glyph, err := e.textFont.CharCodeToGlyph(code)
		if err == nil && glyph > 0 {
			// Try to get glyph name and convert to unicode
			glyphName := e.textFont.GlyphName(glyph)
			if decoded, ok := decodeGlyphNameToRune(glyphName); ok {
				result = append(result, decoded)
				return
			}
		}

		// Fall back to raw character code.
		if code <= utf8.MaxRune {
			fallback := domainunicode.MapSpecialUnicodeValues(rune(code))
			if fallback != 0 {
				result = append(result, fallback)
			}
		}
	}

	if e.textFont.IsCIDFont() {
		// CID fonts commonly use 2-byte big-endian character codes.
		for i := 0; i+1 < len(data); i += 2 {
			code := uint32(data[i])<<8 | uint32(data[i+1])
			appendDecodedRune(code)
		}
		if len(data)%2 == 1 {
			appendDecodedRune(uint32(data[len(data)-1]))
		}
		return string(result)
	}

	for _, b := range data {
		appendDecodedRune(uint32(b))
	}
	return string(result)
}

// decodeGlyphNameToRune converts a glyph name to a rune.
// This is a simplified implementation for common glyph names.
func decodeGlyphNameToRune(name string) (rune, bool) {
	if unicodeVal := domainunicode.GetUnicodeForGlyph(name, nil); unicodeVal >= 0 {
		mapped := domainunicode.MapSpecialUnicodeValues(unicodeVal)
		if mapped != 0 {
			return mapped, true
		}
		return 0, false
	}

	// Common Adobe glyph names
	glyphMap := map[string]rune{
		"space":        ' ',
		"exclam":       '!',
		"quotedbl":     '"',
		"numbersign":   '#',
		"dollar":       '$',
		"percent":      '%',
		"ampersand":    '&',
		"quoteright":   '\'',
		"parenleft":    '(',
		"parenright":   ')',
		"asterisk":     '*',
		"plus":         '+',
		"comma":        ',',
		"hyphen":       '-',
		"period":       '.',
		"slash":        '/',
		"zero":         '0',
		"one":          '1',
		"two":          '2',
		"three":        '3',
		"four":         '4',
		"five":         '5',
		"six":          '6',
		"seven":        '7',
		"eight":        '8',
		"nine":         '9',
		"colon":        ':',
		"semicolon":    ';',
		"less":         '<',
		"equal":        '=',
		"greater":      '>',
		"question":     '?',
		"at":           '@',
		"A":            'A',
		"B":            'B',
		"C":            'C',
		"D":            'D',
		"E":            'E',
		"F":            'F',
		"G":            'G',
		"H":            'H',
		"I":            'I',
		"J":            'J',
		"K":            'K',
		"L":            'L',
		"M":            'M',
		"N":            'N',
		"O":            'O',
		"P":            'P',
		"Q":            'Q',
		"R":            'R',
		"S":            'S',
		"T":            'T',
		"U":            'U',
		"V":            'V',
		"W":            'W',
		"X":            'X',
		"Y":            'Y',
		"Z":            'Z',
		"bracketleft":  '[',
		"backslash":    '\\',
		"bracketright": ']',
		"asciicircum":  '^',
		"underscore":   '_',
		"grave":        '`',
		"a":            'a',
		"b":            'b',
		"c":            'c',
		"d":            'd',
		"e":            'e',
		"f":            'f',
		"g":            'g',
		"h":            'h',
		"i":            'i',
		"j":            'j',
		"k":            'k',
		"l":            'l',
		"m":            'm',
		"n":            'n',
		"o":            'o',
		"p":            'p',
		"q":            'q',
		"r":            'r',
		"s":            's',
		"t":            't',
		"u":            'u',
		"v":            'v',
		"w":            'w',
		"x":            'x',
		"y":            'y',
		"z":            'z',
		"braceleft":    '{',
		"bar":          '|',
		"braceright":   '}',
		"asciitilde":   '~',
	}

	if r, ok := glyphMap[name]; ok {
		mapped := domainunicode.MapSpecialUnicodeValues(r)
		if mapped != 0 {
			return mapped, true
		}
	}
	return 0, false
}

// multiplyMatrix multiplies two matrices.
func multiplyMatrix(a, b [6]float64) [6]float64 {
	return [6]float64{
		a[0]*b[0] + a[2]*b[1],
		a[1]*b[0] + a[3]*b[1],
		a[0]*b[2] + a[2]*b[3],
		a[1]*b[2] + a[3]*b[3],
		a[0]*b[4] + a[2]*b[5] + a[4],
		a[1]*b[4] + a[3]*b[5] + a[5],
	}
}

// Text extraction operators

type textBeginOperator struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textBeginOperator) Execute(state *graphics.State, operands []float64) error {
	op.te.inTextBlock = true
	// Reset text and line matrices to identity
	op.te.textMatrix = [6]float64{1, 0, 0, 1, 0, 0}
	op.te.lineMatrix = [6]float64{1, 0, 0, 1, 0, 0}
	return nil
}

type textEndOperator struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textEndOperator) Execute(state *graphics.State, operands []float64) error {
	op.te.inTextBlock = false
	return nil
}

type textSetCharSpacing struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textSetCharSpacing) Execute(state *graphics.State, operands []float64) error {
	if len(operands) > 0 {
		op.te.charSpacing = operands[0]
	}
	return nil
}

type textSetWordSpacing struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textSetWordSpacing) Execute(state *graphics.State, operands []float64) error {
	if len(operands) > 0 {
		op.te.wordSpacing = operands[0]
	}
	return nil
}

type textSetHorizScaling struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textSetHorizScaling) Execute(state *graphics.State, operands []float64) error {
	if len(operands) > 0 {
		op.te.horizScale = operands[0]
	}
	return nil
}

type textSetTextLeading struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textSetTextLeading) Execute(state *graphics.State, operands []float64) error {
	if len(operands) > 0 {
		op.te.textLeading = operands[0]
	}
	return nil
}

type textSetFont struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textSetFont) Execute(state *graphics.State, operands []float64) error {
	if len(operands) > 0 {
		// OperatorLexer currently tokenizes only numeric operands, so `/FontName`
		// is not available here. Still apply the font size for layout parity.
		op.te.textSize = operands[len(operands)-1]
	}
	return nil
}

type textSetRenderMode struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textSetRenderMode) Execute(state *graphics.State, operands []float64) error {
	if len(operands) > 0 {
		op.te.renderMode = int(operands[0])
	}
	return nil
}

type textSetTextRise struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textSetTextRise) Execute(state *graphics.State, operands []float64) error {
	if len(operands) > 0 {
		op.te.textRise = operands[0]
	}
	return nil
}

type textMoveText struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textMoveText) Execute(state *graphics.State, operands []float64) error {
	if len(operands) < 2 {
		return fmt.Errorf("td operator requires 2 operands")
	}

	tx, ty := operands[0], operands[1]

	// Td operator: Set line matrix and text matrix
	// Td is equivalent to: 1 0 0 1 tx ty Tm
	tm := [6]float64{1, 0, 0, 1, tx, ty}
	op.te.lineMatrix = multiplyMatrix(tm, op.te.lineMatrix)
	op.te.textMatrix = op.te.lineMatrix

	return nil
}

type textMoveTextSetLeading struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textMoveTextSetLeading) Execute(state *graphics.State, operands []float64) error {
	if len(operands) < 2 {
		return fmt.Errorf("td operator requires 2 operands")
	}

	tx, ty := operands[0], operands[1]

	// TD operator: Set text leading and move text
	op.te.textLeading = -ty

	// Then apply Td
	tm := [6]float64{1, 0, 0, 1, tx, ty}
	op.te.lineMatrix = multiplyMatrix(tm, op.te.lineMatrix)
	op.te.textMatrix = op.te.lineMatrix

	return nil
}

type textSetTextMatrix struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textSetTextMatrix) Execute(state *graphics.State, operands []float64) error {
	if len(operands) < 6 {
		return fmt.Errorf("tm operator requires 6 operands")
	}

	// Tm operator: Set text matrix and line matrix
	var tm [6]float64
	for i := 0; i < 6; i++ {
		tm[i] = operands[i]
	}

	op.te.textMatrix = tm
	op.te.lineMatrix = tm

	return nil
}

type textNextLine struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textNextLine) Execute(state *graphics.State, operands []float64) error {
	// T* operator: Move to start of next line
	// Equivalent to: 0 leading Td
	tm := [6]float64{1, 0, 0, 1, 0, -op.te.textLeading}
	op.te.lineMatrix = multiplyMatrix(tm, op.te.lineMatrix)
	op.te.textMatrix = op.te.lineMatrix

	return nil
}

type textShowText struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textShowText) Execute(state *graphics.State, operands []float64) error {
	// String operand handling is performed in ProcessBytes object parsing flow.
	return nil
}

type textShowTextArray struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textShowTextArray) Execute(state *graphics.State, operands []float64) error {
	// Array operand handling is performed in ProcessBytes object parsing flow.
	return nil
}

type textNextLineShow struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textNextLineShow) Execute(state *graphics.State, operands []float64) error {
	// Move to next line then show text
	// T* then Tj
	tm := [6]float64{1, 0, 0, 1, 0, -op.te.textLeading}
	op.te.lineMatrix = multiplyMatrix(tm, op.te.lineMatrix)
	op.te.textMatrix = op.te.lineMatrix

	// Then show text (handled separately)
	return nil
}

type textSetSpacingShow struct {
	te *TextEvaluator
}

// Execute executes the operation.
func (op *textSetSpacingShow) Execute(state *graphics.State, operands []float64) error {
	if len(operands) < 3 {
		return fmt.Errorf("\" operator requires 3 operands")
	}

	// Set word and character spacing
	op.te.wordSpacing = operands[0]
	op.te.charSpacing = operands[1]

	// Move to next line
	tm := [6]float64{1, 0, 0, 1, 0, -op.te.textLeading}
	op.te.lineMatrix = multiplyMatrix(tm, op.te.lineMatrix)
	op.te.textMatrix = op.te.lineMatrix

	// Then show text (handled separately)
	return nil
}
