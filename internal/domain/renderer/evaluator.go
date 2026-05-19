// Package renderer provides PDF content stream evaluation and rendering.
//
//revive:disable:exported
//nolint:errcheck,govet,ineffassign
package renderer

import (
	"encoding/hex"
	"fmt"
	stdimage "image"
	"image/color"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/dh-kam/pdf-go/internal/domain/canvas"
	"github.com/dh-kam/pdf-go/internal/domain/colorspace"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/domain/errors"
	"github.com/dh-kam/pdf-go/internal/domain/graphics"
	domainimage "github.com/dh-kam/pdf-go/internal/domain/image"
	"github.com/dh-kam/pdf-go/internal/infrastructure/image"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/parser"
	"github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"
)

// gsPool recycles GraphicsState objects to reduce heap allocations in save/restore.
var gsPool = sync.Pool{
	New: func() interface{} { return new(GraphicsState) },
}

// Evaluator evaluates PDF content streams and builds an operator list.
type Evaluator struct {
	xref                 entity.XRef
	canvas               canvas.Canvas
	fontResolver         fontCandidateResolver
	fontFallback         fontFallbackResolver
	textPolicy           textRenderPolicy
	textPlacement        textPlacement
	textRenderer         textRenderer
	graphics             *GraphicsState
	initialTransform     [6]float64
	textMatrix           [6]float64
	textLineMatrix       [6]float64
	textBaseMatrix       [6]float64
	textLineX            float64
	textLineY            float64
	textCurrentX         float64
	textCurrentY         float64
	textUserCurrentX     float64
	textUserCurrentY     float64
	resources            *entity.Dict
	inlineImageDict      *entity.Dict
	textBuffer           strings.Builder
	operators            []Operator
	stateStack           []*GraphicsState
	inlineImageData      []byte
	formOperatorCache    map[*entity.Stream][]Operator
	charProcCache        map[*entity.Stream][]Operator
	fontCache            map[*entity.Dict]entity.Font
	sharedFormCache      FormOperatorCache
	imageSamplingMode    string
	debugDocumentID      string
	debugPageNumber      int
	debugImageSampling   bool
	inInlineImage        bool
	textCurrentValid     bool
	textUserCurrentValid bool
}

type textCodeUnit struct {
	code uint32
	raw  []byte
}

var deviceCMYKColorSpace = colorspace.NewDeviceCMYK()

// NewEvaluator creates a new content stream evaluator.
func NewEvaluator(xref entity.XRef) *Evaluator {
	identity := [6]float64{1, 0, 0, 1, 0, 0}
	return &Evaluator{
		xref:              xref,
		fontResolver:      defaultFontCandidateResolver{},
		fontFallback:      defaultFontFallbackResolver{},
		textPolicy:        defaultTextRenderPolicy{},
		textPlacement:     defaultTextPlacement{},
		textRenderer:      defaultTextRenderer{},
		operators:         make([]Operator, 0, 128),
		stateStack:        make([]*GraphicsState, 0, 16),
		graphics:          NewGraphicsState(),
		initialTransform:  identity,
		textMatrix:        identity,
		textLineMatrix:    identity,
		textBaseMatrix:    identity,
		formOperatorCache: make(map[*entity.Stream][]Operator),
		charProcCache:     make(map[*entity.Stream][]Operator),
		fontCache:         make(map[*entity.Dict]entity.Font),
		imageSamplingMode: ImageSamplingModeLegacy,
	}
}

// ColorSpace represents a color with its color space.
type ColorSpace struct {
	Color interface{} // Can be *Color, *Pattern, etc.
}

// Color represents a color value.
type Color struct {
	Hex string // e.g., "FF0000" for red
}

// Operator represents a PDF graphics operator.
type Operator struct {
	Resources *entity.Dict
	Opcode    string
	Operands  []entity.Object
}

var contentOperatorKeywords = map[string]struct{}{
	// Graphics state / compatibility
	"q": {}, "Q": {}, "cm": {}, "w": {}, "J": {}, "j": {}, "M": {}, "d": {}, "ri": {}, "i": {}, "gs": {}, "BX": {}, "EX": {},
	// Marked content operators are rendering no-ops, but they must be parsed
	// as operators so following appearance-stream text operators stay aligned.
	"BMC": {}, "BDC": {}, "EMC": {}, "MP": {}, "DP": {},
	// Path construction / painting / clipping
	"m": {}, "l": {}, "c": {}, "v": {}, "y": {}, "h": {}, "re": {},
	"S": {}, "s": {}, "f": {}, "F": {}, "f*": {}, "B": {}, "B*": {}, "b": {}, "b*": {}, "n": {}, "W": {}, "W*": {},
	// Color / pattern / shading
	"CS": {}, "cs": {}, "SC": {}, "SCN": {}, "sc": {}, "scn": {}, "G": {}, "g": {}, "RG": {}, "rg": {}, "K": {}, "k": {}, "sh": {},
	// Text
	"BT": {}, "ET": {}, "Tc": {}, "Tw": {}, "Tz": {}, "TL": {}, "Tf": {}, "Tr": {}, "Ts": {},
	"Td": {}, "TD": {}, "Tm": {}, "T*": {}, "Tj": {}, "TJ": {}, "'": {}, "\"": {},
	// XObject / inline image
	"Do": {}, "BI": {}, "ID": {}, "EI": {}, "d0": {}, "d1": {},
}

func isContentOperatorKeyword(keyword string) bool {
	_, ok := contentOperatorKeywords[keyword]
	return ok
}

// Evaluate evaluates the content stream for a page.
func (e *Evaluator) Evaluate(contents []entity.Object) error {
	for _, content := range contents {
		entityStream, ok := content.(*entity.Stream)
		if !ok {
			continue
		}

		// Convert entity.Stream to infrastructure/stream.Stream for proper filter decoding
		infraStream := stream.NewFromEntity(entityStream)
		data, err := infraStream.Decode()
		if err != nil {
			// Some malformed/encrypted PDFs contain stream bytes that fail declared filter decoding.
			// Fall back to raw stream bytes as best-effort, and skip this stream on failure.
			raw := entityStream.RawBytes()
			if len(raw) > 0 {
				_ = e.parseOperators(raw)
			}
			continue
		}

		// Parse operators from stream data
		if err := e.parseOperators(data); err != nil {
			return err
		}
	}

	return nil
}

// parseOperators parses PDF operators from binary stream data.
func (e *Evaluator) parseOperators(data []byte) error {
	return e.parseOperatorsWithHandler(data, func(op Operator) {
		e.operators = append(e.operators, op)
		if err := e.executeOperator(op); err != nil {
			// Keep rendering even if a single operator is malformed.
			return
		}
	})
}

func (e *Evaluator) parseOperatorsOnly(data []byte) ([]Operator, error) {
	ops := make([]Operator, 0, 64)
	err := e.parseOperatorsWithHandler(data, func(op Operator) {
		ops = append(ops, op)
	})
	if err != nil {
		return nil, err
	}
	return ops, nil
}

func (e *Evaluator) parseOperatorsWithHandler(data []byte, handler func(op Operator)) error {
	// Create lexer and parser for the content stream
	lexer := parser.NewLexerBytes(data)
	p := parser.NewParser(lexer, e.xref)
	operands := make([]entity.Object, 0, 8)

	for {
		// Flush parser-buffered operands (e.g. non-reference "num num <op>" sequences)
		// before looking ahead to the next operator token.
		if p.HasBufferedObject() {
			obj, err := p.ParseObject()
			if err != nil {
				if err == io.EOF {
					break
				}
				// Recover from malformed operands by consuming one token and continuing.
				if _, skipErr := lexer.NextToken(); skipErr == nil {
					continue
				}
				return err
			}
			operands = append(operands, obj)
			continue
		}

		token, err := lexer.Peek()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if token.Type == parser.TokenEOF {
			break
		}

		// In PDF content streams, operands come before operators.
		if token.Type == parser.TokenKeyword && isContentOperatorKeyword(token.Value) {
			if token.Value == "BI" {
				_, err := lexer.NextToken()
				if err != nil {
					return err
				}
				operands = operands[:0]
				// Parse BI/ID/EI inline image block and continue remaining operators in-place.
				_ = e.parseInlineImageFromLexer(lexer, p, data)
				return nil
			}

			_, err := lexer.NextToken()
			if err != nil {
				return err
			}
			opOperands := append([]entity.Object(nil), operands...)
			// Create operator
			op := Operator{
				Opcode:    token.Value,
				Operands:  opOperands,
				Resources: e.resources,
			}
			operands = operands[:0]
			handler(op)
			continue
		}

		obj, err := p.ParseObject()
		if err != nil {
			if err == io.EOF {
				break
			}
			// Recover from malformed operands by consuming one token and continuing.
			if _, skipErr := lexer.NextToken(); skipErr == nil {
				continue
			}
			return err
		}
		operands = append(operands, obj)
	}

	return nil
}

func (e *Evaluator) parseInlineImageFromLexer(lexer *parser.Lexer, p *parser.Parser, data []byte) error {
	if err := e.beginInlineImage(); err != nil {
		return nil
	}

	searchFrom := lexer.Pos()

	// Parse inline image dictionary: /Key value pairs until ID token.
	for {
		tok, err := lexer.Peek()
		if err != nil {
			return e.skipInlineImageAndContinue(data, searchFrom)
		}

		if tok.Type == parser.TokenEOF {
			return e.skipInlineImageAndContinue(data, searchFrom)
		}

		if tok.Type == parser.TokenKeyword && tok.Value == "ID" {
			if _, err := lexer.NextToken(); err != nil {
				return e.skipInlineImageAndContinue(data, searchFrom)
			}
			searchFrom = lexer.Pos()
			break
		}

		keyToken, err := lexer.NextToken()
		if err != nil {
			return e.skipInlineImageAndContinue(data, searchFrom)
		}
		if keyToken.Type != parser.TokenKeyword {
			return e.skipInlineImageAndContinue(data, searchFrom)
		}

		key := "/" + keyToken.Value
		value, err := p.ParseObject()
		if err != nil {
			return e.skipInlineImageAndContinue(data, searchFrom)
		}
		e.inlineImageDict.Set(entity.Name(key), value)
	}

	// Parse image data bytes until EI token boundary.
	start := searchFrom
	if start < 0 || start >= len(data) {
		return e.skipInlineImageAndContinue(data, searchFrom)
	}

	start = skipInlineImageLeadingWhitespace(data, start)
	if start >= len(data) {
		return e.skipInlineImageAndContinue(data, searchFrom)
	}

	end, err := findInlineImageEndOffset(data, start)
	if err != nil {
		return e.skipInlineImageAndContinue(data, searchFrom)
	}

	e.inlineImageData = make([]byte, end-start)
	copy(e.inlineImageData, data[start:end])

	if err := e.endInlineImage(); err != nil {
		return e.skipInlineImageAndContinue(data, end+2)
	}

	// Continue parsing remaining operators after EI.
	return e.parseOperators(data[end+2:])
}

func (e *Evaluator) skipInlineImageAndContinue(data []byte, searchFrom int) error {
	if searchFrom < 0 {
		searchFrom = 0
	}
	if searchFrom >= len(data) {
		e.resetInlineImageState()
		return nil
	}

	end, err := findInlineImageEndOffset(data, searchFrom)
	if err != nil {
		e.resetInlineImageState()
		return nil
	}

	e.resetInlineImageState()
	return e.parseOperators(data[end+2:])
}

func (e *Evaluator) resetInlineImageState() {
	e.inInlineImage = false
	e.inlineImageDict = nil
	e.inlineImageData = nil
}

func skipInlineImageLeadingWhitespace(data []byte, start int) int {
	for start < len(data) {
		switch data[start] {
		case 0x00, 0x09, 0x0A, 0x0C, 0x0D, 0x20:
			start++
		default:
			return start
		}
	}
	return start
}

func isInlineImageTokenBoundary(b byte) bool {
	if b == 0x00 || b == 0x09 || b == 0x0A || b == 0x0C || b == 0x0D || b == 0x20 {
		return true
	}

	switch b {
	case '(', ')', '<', '>', '[', ']', '/', '%':
		return true
	default:
		return false
	}
}

func findInlineImageEndOffset(data []byte, start int) (int, error) {
	for i := start; i+1 < len(data); i++ {
		if data[i] != 'E' || data[i+1] != 'I' {
			continue
		}

		if i > 0 && !isInlineImageTokenBoundary(data[i-1]) {
			continue
		}
		if i+2 < len(data) && !isInlineImageTokenBoundary(data[i+2]) {
			continue
		}

		return i, nil
	}

	return 0, fmt.Errorf("inline image missing EI")
}

func (e *Evaluator) executeCachedOperators(ops []Operator) {
	if len(ops) == 0 {
		return
	}
	// Bulk-extend the operators slice once to avoid repeated growslice.
	n := len(ops)
	base := len(e.operators)
	if cap(e.operators) < base+n {
		newCap := base + n
		if newCap < 2*cap(e.operators) {
			newCap = 2 * cap(e.operators)
		}
		grown := make([]Operator, base, newCap)
		copy(grown, e.operators[:base])
		e.operators = grown
	}
	e.operators = append(e.operators[:base], make([]Operator, n)...)
	for i, op := range ops {
		e.operators[base+i] = op
		if err := e.executeOperator(op); err != nil {
			continue
		}
	}
}

// executeOperator executes a single graphics operator.
func (e *Evaluator) executeOperator(op Operator) error {
	// Debug: log all operators (skip frequent ones)
	if op.Opcode != "m" && op.Opcode != "l" && op.Opcode != "c" {
	}
	switch op.Opcode {
	case "q":
		// Save graphics state
		return e.saveState()
	case "Q":
		// Restore graphics state
		return e.restoreState()
	case "BT":
		// Begin text object
		e.beginTextObject()
	case "ET":
		// End text object
		e.endTextObject()
		if e.textBuffer.Len() > 0 && !strings.HasSuffix(e.textBuffer.String(), "\n") {
			e.textBuffer.WriteByte('\n')
		}
	case "Tj":
		// Show text
		return e.showText(op)
	case "TJ":
		// Show text with individual glyph positioning
		return e.showTextArray(op)
	case "Td":
		// Move to next line
		return e.moveText(op)
	case "TD":
		// Move to next line with leading
		return e.moveTextSetLeading(op)
	case "T*":
		// Move to start of next line using current leading
		return e.moveTextNextLine()
	case "'":
		// Move to next line and show text
		return e.moveTextNextLineAndShowText(op)
	case "\"":
		// Set word/char spacing, move to next line, and show text
		return e.setSpacingMoveTextNextLineAndShowText(op)
	case "Tm":
		// Set text matrix
		return e.setTextMatrix(op)
	case "Tc":
		// Set character spacing
		return e.setCharSpacing(op)
	case "Tw":
		// Set word spacing
		return e.setWordSpacing(op)
	case "Tz":
		// Set horizontal scaling
		return e.setHorizScaling(op)
	case "TL":
		// Set text leading
		return e.setTextLeading(op)
	case "Tf":
		// Set text font
		return e.setFont(op)
	case "Tr":
		// Set text rendering mode
		return e.setTextRenderMode(op)
	case "Ts":
		// Set text rise
		return e.setTextRise(op)
	case "BMC", "BDC", "EMC", "MP", "DP", "BX", "EX":
		// Marked content and compatibility-section operators do not affect
		// raster output directly.
		return nil
	case "w":
		// Set line width
		return e.setLineWidth(op)
	case "J":
		// Set line join style
		return e.setLineCap(op)
	case "j":
		// Set line cap style
		return e.setLineJoin(op)
	case "M":
		// Miter limit
		return e.setMiterLimit(op)
	case "d":
		// Set line dash pattern
		return e.setDashPattern(op)
	case "L":
		// Append straight line segment (same as l)
		return e.lineTo(op)
	case "C":
		// Append curved line segment (same as c)
		return e.curveTo(op)
	case "c":
		// Append curved line segment (cubic Bézier)
		return e.curveTo(op)
	case "v":
		// Append curved line segment (cubic Bézier, initial point replicated)
		return e.curveToNoFirstControl(op)
	case "y":
		// Append curved line segment (cubic Bézier, final point replicated)
		return e.curveToNoLastControl(op)
	case "Y":
		// Append curved line segment (cubic Bézier, initial and final points replicated)
		return e.curveToNoLastControl(op)
	case "H":
		// Close subpath (same as h)
		return e.closePath(op)
	case "h":
		// Close subpath
		return e.closePath(op)
	case "l":
		// Append straight line segment
		return e.lineTo(op)
	case "m":
		// Move to current point
		return e.moveTo(op)
	case "re":
		// Append rectangle to path
		return e.rectangle(op)
	case "f":
		// Fill path using nonzero winding rule
		return e.fillPath()
	case "F":
		// Fill path using nonzero winding rule (obsolete)
		return e.fillPath()
	case "f*":
		// Fill path using even-odd rule
		return e.fillPathEvenOdd()
	case "B":
		// Fill and stroke path using nonzero winding rule
		return e.fillAndStrokePath()
	case "B*":
		// Fill and stroke path using even-odd rule
		return e.fillAndStrokePathEvenOdd()
	case "b":
		// Close, fill, and stroke path using nonzero winding rule
		return e.closeFillAndStrokePath()
	case "S":
		// Stroke path
		return e.strokePath()
	case "s":
		// Close and stroke path
		return e.strokeAndClosePath()
	case "b*":
		// Close, fill, and stroke path using even-odd rule
		return e.closeFillAndStrokePathEvenOdd()
	case "n":
		// End path without filling or stroking
		return e.endPath()
	case "W":
		// Set clipping path using nonzero winding rule
		return e.setClipPath()
	case "W*":
		// Set clipping path using even-odd rule
		return e.setClipPathEvenOdd()
	case "CS":
		// Set color space for stroking operations
		return e.setStrokeColorSpace(op)
	case "cs":
		// Set color space for filling operations
		return e.setFillColorSpace(op)
	case "SC":
		// Set color for stroking operations
		return e.setStrokeColorBySpace(op)
	case "SCN":
		// Set color for stroking operations (ICCBased/patterns)
		return e.setStrokeColorBySpace(op)
	case "sc":
		// Set color for filling operations
		return e.setFillColorBySpace(op)
	case "scn":
		// Set color for filling operations (ICCBased/patterns)
		return e.setFillColorBySpace(op)
	case "G":
		// Set gray color for stroking operations
		return e.setGrayStroke(op)
	case "g":
		// Set gray color for filling operations
		return e.setGrayFill(op)
	case "RG":
		// Set RGB color for stroking operations
		return e.setRGBStroke(op)
	case "rg":
		// Set RGB color for filling operations
		return e.setRGBFill(op)
	case "K":
		// Set CMYK color for stroking operations
		return e.setCMYKStroke(op)
	case "k":
		// Set CMYK color for filling operations
		return e.setCMYKFill(op)
	case "sh":
		// Paint shading pattern
		return e.paintShading(op)
	case "gs":
		// Set graphics state parameter
		return e.applyGraphicsStateParameters(op)
	case "Do":
		// Invoke named XObject
		return e.invokeXObject(op)
	case "BI":
		// Begin inline image
		return e.beginInlineImage()
	case "ID":
		// Begin inline image data
		// ID is handled specially by the lexer
		// The data between ID and EI is collected as raw bytes
		return nil
	case "EI":
		// End inline image
		return e.endInlineImage()
	case "cm":
		// Concatenate matrix to current transformation matrix
		return e.concatenateMatrix(op)
	case "d0":
		// Type3 font: set width and displacement (no bbox)
		return e.executeD0(op)
	case "d1":
		// Type3 font: set width, bbox, and displacement
		return e.executeD1(op)
	default:
		// Unknown operator - ignore for now
	}

	return nil
}

// renderType3Glyph evaluates a Type3 font glyph's content stream to render it.
func (e *Evaluator) renderType3Glyph(font *entity.Type3Font, charCode uint32, x, y float64, fontSize float64) error {
	charProcStream := font.CharProcForCode(charCode)
	if charProcStream == nil {
		return fmt.Errorf("type3 font: no charproc for code %d", charCode)
	}

	// Use cached parsed operators if available, otherwise parse and cache.
	ops, ok := e.charProcCache[charProcStream]
	if !ok {
		infraStream := stream.NewFromEntity(charProcStream)
		data, err := infraStream.Decode()
		if err != nil {
			return fmt.Errorf("type3 font: decode charproc: %w", err)
		}

		ops, err = e.parseOperatorsOnly(data)
		if err != nil {
			return fmt.Errorf("type3 font: parse charproc: %w", err)
		}
		e.charProcCache[charProcStream] = ops
	}

	if type3CharProcUsesD1Cache(ops) {
		if quantizer, ok := e.canvas.(interface {
			QuantizeType3GlyphOrigin(x, y float64) (float64, float64)
		}); ok {
			x, y = quantizer.QuantizeType3GlyphOrigin(x, y)
		}
	}
	if marker, ok := e.canvas.(interface {
		BeginType3Glyph()
		EndType3Glyph()
	}); ok {
		marker.BeginType3Glyph()
		defer marker.EndType3Glyph()
	}

	// Save current graphics state
	if err := e.saveState(); err != nil {
		return err
	}
	defer func() { _ = e.restoreState() }()

	e.graphics.transform = e.type3GlyphCTM(font, x, y, fontSize)

	glyphResources := font.Resources()
	if charProcStream.Dict() != nil {
		if charProcResources := e.resourceDictFromObject(charProcStream.Dict().Get(entity.Name("Resources"))); charProcResources != nil {
			glyphResources = mergeType3ResourceDicts(glyphResources, charProcResources)
		}
	}
	if glyphResources != nil {
		oldResources := e.resources
		e.resources = glyphResources
		defer func() { e.resources = oldResources }()
	}

	e.executeCachedOperators(ops)
	return nil
}

func type3CharProcUsesD1Cache(ops []Operator) bool {
	for _, op := range ops {
		switch op.Opcode {
		case "d1":
			return true
		case "d0", "q", "Q":
			return false
		}
	}
	return false
}

func (e *Evaluator) type3GlyphCTM(font *entity.Type3Font, x, y float64, fontSize float64) [6]float64 {
	oldCTM := e.graphics.transform
	textMatrix := e.textMatrix
	if textRise := e.graphics.currentState.GetTextRise(); textRise != 0 {
		textMatrix = multiplyMatrix(textMatrix, [6]float64{1, 0, 0, 1, 0, textRise})
	}

	// Match Poppler Gfx::doShowText(): combine text matrix and CTM linear terms,
	// then apply the Type3 FontMatrix and font size. Translation is the already
	// transformed glyph origin, not FontMatrix e/f.
	tmp0 := textMatrix[0]*oldCTM[0] + textMatrix[1]*oldCTM[2]
	tmp1 := textMatrix[0]*oldCTM[1] + textMatrix[1]*oldCTM[3]
	tmp2 := textMatrix[2]*oldCTM[0] + textMatrix[3]*oldCTM[2]
	tmp3 := textMatrix[2]*oldCTM[1] + textMatrix[3]*oldCTM[3]

	fm := font.FontMatrix()
	ctm := [6]float64{
		(fm[0]*tmp0 + fm[1]*tmp2) * fontSize,
		(fm[0]*tmp1 + fm[1]*tmp3) * fontSize,
		(fm[2]*tmp0 + fm[3]*tmp2) * fontSize,
		(fm[2]*tmp1 + fm[3]*tmp3) * fontSize,
		x,
		y,
	}

	hScale := e.graphics.currentState.GetHorizontalScaling() / 100.0
	if hScale == 0 {
		hScale = 1.0
	}
	ctm[0] *= hScale
	ctm[1] *= hScale
	return ctm
}

// executeD0 handles the d0 operator for Type3 fonts.
// d0: wx wy d0 — sets glyph width and ensures the glyph description
// contains only width information (no bounding box cache).
func (e *Evaluator) executeD0(op Operator) error {
	// d0 is a no-op during rendering; the width is already set by the font.
	// It only matters during glyph metrics calculation.
	return nil
}

// executeD1 handles the d1 operator for Type3 fonts.
// d1: wx wy llx lly urx ury d1 — sets glyph width and cache bounding box.
// After d1, the glyph description is assumed to describe only the bbox region.
func (e *Evaluator) executeD1(op Operator) error {
	// d1 is a no-op during rendering; width and bbox are already known.
	// It only matters during glyph metrics calculation.
	return nil
}

// Save saves the current graphics state.
func (g *GraphicsState) Save() {
	// Delegate save/restore semantics to the embedded graphics state stack
	// so callers that rely on this API can preserve nested text/state changes.
	if g == nil || g.currentState == nil {
		return
	}
	g.currentState = g.currentState.Save()
}

// Restore restores the last saved graphics state.
func (g *GraphicsState) Restore() {
	if g == nil || g.currentState == nil {
		return
	}
	g.currentState = g.currentState.Restore()
}

// saveState saves the current graphics state (for 'q' operator).
func (e *Evaluator) saveState() error {
	if e.canvas != nil {
		e.canvas.Save()
	}

	// Recycle GraphicsState from pool to reduce heap allocations.
	stateCopy := gsPool.Get().(*GraphicsState)
	*stateCopy = *e.graphics
	stateCopy.textMatrix = e.textMatrix
	stateCopy.textLine = e.textLineMatrix
	stateCopy.textBaseMatrix = e.textBaseMatrix
	stateCopy.textLineX = e.textLineX
	stateCopy.textLineY = e.textLineY
	stateCopy.textUserCurrentX = e.textUserCurrentX
	stateCopy.textUserCurrentY = e.textUserCurrentY
	stateCopy.textUserCurrentValid = e.textUserCurrentValid
	stateCopy.currentState = cloneCurrentState(e.graphics.currentState)

	// Only clone the path when it has elements — empty paths are cheap to share.
	if !e.graphics.path.IsEmpty() {
		stateCopy.path = e.graphics.path.Clone()
	}

	// Push onto stack (pre-allocated with capacity to reduce growslice).
	e.stateStack = append(e.stateStack, stateCopy)
	return nil
}

// restoreState restores the last saved graphics state (for 'Q' operator).
func (e *Evaluator) restoreState() error {
	if len(e.stateStack) == 0 {
		return fmt.Errorf("graphics state stack is empty")
	}

	// Pop from stack
	state := e.stateStack[len(e.stateStack)-1]
	e.stateStack[len(e.stateStack)-1] = nil // Clear reference for GC
	e.stateStack = e.stateStack[:len(e.stateStack)-1]

	// Restore graphics state
	e.graphics.transform = state.transform
	e.graphics.baseTransform = state.baseTransform
	e.graphics.lineWidth = state.lineWidth
	e.graphics.fillAlpha = state.fillAlpha
	e.graphics.strokeAlpha = state.strokeAlpha
	e.graphics.transferRed = state.transferRed
	e.graphics.transferGreen = state.transferGreen
	e.graphics.transferBlue = state.transferBlue
	e.graphics.transferGray = state.transferGray
	e.graphics.transferActive = state.transferActive
	e.graphics.fillColor = state.fillColor
	e.graphics.strokeColor = state.strokeColor
	e.graphics.fillPattern = state.fillPattern
	e.graphics.strokePattern = state.strokePattern
	e.graphics.fillCS = state.fillCS
	e.graphics.strokeCS = state.strokeCS
	e.graphics.fillPatternBaseCS = state.fillPatternBaseCS
	e.graphics.strokePatternBaseCS = state.strokePatternBaseCS
	e.graphics.font = state.font
	e.graphics.fontDebugName = state.fontDebugName
	e.graphics.fontSize = state.fontSize
	e.graphics.currentState = state.currentState
	e.syncTextMatricesState(state.textMatrix, state.textLine)
	e.textBaseMatrix = state.textBaseMatrix
	e.textLineX = state.textLineX
	e.textLineY = state.textLineY
	e.textUserCurrentX = state.textUserCurrentX
	e.textUserCurrentY = state.textUserCurrentY
	e.textUserCurrentValid = state.textUserCurrentValid
	e.graphics.path = state.path
	e.graphics.pathClip = state.pathClip
	e.graphics.clipMode = state.clipMode
	e.graphics.pendingClip = state.pendingClip
	e.graphics.pendingClipMode = state.pendingClipMode

	// Return state to pool for reuse.
	*state = GraphicsState{}
	gsPool.Put(state)

	if e.canvas != nil {
		e.canvas.Restore()
	}

	return nil
}

// concatenateMatrix concatenates a matrix to the current transformation matrix.
func (e *Evaluator) concatenateMatrix(op Operator) error {
	if len(op.Operands) < 6 {
		return fmt.Errorf("cm operator requires 6 operands")
	}

	// Get matrix operands
	var matrix [6]float64
	for i := 0; i < 6; i++ {
		num, err := getNumberOperand(op.Operands[i])
		if err != nil {
			// Be permissive for malformed content streams and continue rendering.
			return nil
		}
		matrix[i] = num
	}

	// Concatenate with current transform.
	// PDF cm semantics: newCTM = currentCTM × matrix.
	e.graphics.transform = multiplyMatrix(e.graphics.transform, matrix)

	return nil
}

// multiplyMatrix multiplies two 3x2 matrices (represented as 6-element arrays)
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

func transformPointWithMatrix(m [6]float64, x, y float64) (float64, float64) {
	tx := m[0]*x + m[2]*y + m[4]
	ty := m[1]*x + m[3]*y + m[5]
	return tx, ty
}

func (e *Evaluator) currentImageTransform() [6]float64 {
	return e.graphics.transform
}

func (e *Evaluator) beginTextObject() {
	identity := [6]float64{1, 0, 0, 1, 0, 0}
	e.syncTextMatricesState(identity, identity)
	e.syncPopplerTextBase(identity, 0, 0)
}

func (e *Evaluator) endTextObject() {
	// Keep current text state as-is for ET. Next BT resets matrices.
	// Re-sync text matrix from current state to avoid stale references.
	e.syncTextMatrixState(e.textMatrix)
}

func (e *Evaluator) advanceTextMatrix(tx float64) {
	if tx == 0 {
		return
	}
	var currentX, currentY float64
	keepCurrent := usePopplerTextCurrentShift()
	if keepCurrent {
		trm := e.textPlacement.CurrentRenderingMatrix(e)
		if e.textCurrentValid {
			currentX, currentY = e.textCurrentX, e.textCurrentY
		} else {
			currentX, currentY = trm[4], trm[5]
		}
		currentX += trm[0] * tx
		currentY += trm[1] * tx
	}
	userCurrentX, userCurrentY := e.textMatrix[4], e.textMatrix[5]
	if e.textUserCurrentValid {
		userCurrentX = e.textUserCurrentX
		userCurrentY = e.textUserCurrentY
	}
	userCurrentX += e.textMatrix[0] * tx
	userCurrentY += e.textMatrix[1] * tx
	tm := [6]float64{1, 0, 0, 1, tx, 0}
	e.syncTextMatrixState(multiplyMatrix(e.textMatrix, tm))
	e.textUserCurrentX = userCurrentX
	e.textUserCurrentY = userCurrentY
	e.textUserCurrentValid = true
	if keepCurrent {
		e.textCurrentX = currentX
		e.textCurrentY = currentY
		e.textCurrentValid = true
	}
}

func (e *Evaluator) moveTextBy(tx, ty float64) {
	e.textLineX += tx
	e.textLineY += ty
	tm := [6]float64{1, 0, 0, 1, e.textLineX, e.textLineY}
	nextLineMatrix := multiplyMatrix(e.textBaseMatrix, tm)
	e.syncTextMatricesState(nextLineMatrix, nextLineMatrix)
	e.textUserCurrentX = e.textBaseMatrix[0]*e.textLineX + e.textBaseMatrix[2]*e.textLineY + e.textBaseMatrix[4]
	e.textUserCurrentY = e.textBaseMatrix[1]*e.textLineX + e.textBaseMatrix[3]*e.textLineY + e.textBaseMatrix[5]
	e.textUserCurrentValid = true
}

func splitTextCodeUnits(text string, font entity.Font) []textCodeUnit {
	raw := []byte(text)
	if len(raw) == 0 {
		return nil
	}

	if font != nil && font.IsCIDFont() {
		out := make([]textCodeUnit, 0, (len(raw)+1)/2)
		for i := 0; i < len(raw); {
			if i+1 < len(raw) {
				out = append(out, textCodeUnit{
					code: uint32(raw[i])<<8 | uint32(raw[i+1]),
					raw:  raw[i : i+2],
				})
				i += 2
				continue
			}
			out = append(out, textCodeUnit{
				code: uint32(raw[i]),
				raw:  raw[i : i+1],
			})
			i++
		}
		return out
	}

	out := make([]textCodeUnit, 0, len(raw))
	for i := range raw {
		out = append(out, textCodeUnit{
			code: uint32(raw[i]),
			raw:  raw[i : i+1],
		})
	}
	return out
}

func (e *Evaluator) glyphAdvance(charCode uint32, font entity.Font, fontSize float64) float64 {
	width := 500.0
	hasWidth := false
	if os.Getenv("PDF_DEBUG_SIMPLE_WIDTH_BY_CODE") == "1" {
		type charCodeWidthFont interface {
			GetCharCodeWidth(code uint32) (float64, bool)
		}
		if typed, ok := font.(charCodeWidthFont); ok {
			if codeWidth, found := typed.GetCharCodeWidth(charCode); found {
				width = codeWidth
				hasWidth = true
			}
		}
	}
	if !hasWidth {
		glyph, err := font.CharCodeToGlyph(charCode)
		if err == nil {
			if glyphWidth, widthErr := font.GetGlyphWidth(glyph); widthErr == nil {
				width = glyphWidth
			}
		}
	}

	unitsPerEm := float64(font.UnitsPerEm())
	if unitsPerEm <= 0 {
		unitsPerEm = 1000
	}

	advance := (width / unitsPerEm) * fontSize
	advance += e.graphics.currentState.GetCharSpacing()
	if charCode == ' ' {
		advance += e.graphics.currentState.GetWordSpacing()
	}

	hScale := e.graphics.currentState.GetHorizontalScaling() / 100.0
	if hScale == 0 {
		hScale = 1.0
	}

	return advance * hScale
}

func cloneCurrentState(src *graphics.State) *graphics.State {
	if src == nil {
		return graphics.NewState()
	}

	// Value copy captures all scalar fields; only slices/pointers need deep copy.
	dst := *src

	if dash := src.GetDashArray(); len(dash) > 0 {
		copiedDash := append([]float64(nil), dash...)
		dst.SetDashArray(copiedDash, src.GetDashPhase())
	}
	if currentPath := src.GetCurrentPath(); currentPath != nil {
		dst.SetCurrentPath(currentPath.Clone())
	}
	if clipPath := src.GetClipPath(); clipPath != nil {
		dst.SetClipPath(clipPath.Clone())
	}

	return &dst
}

// Text operators
func (e *Evaluator) showText(op Operator) error {
	if len(op.Operands) < 1 {
		return fmt.Errorf("tj operator requires 1 operand")
	}

	// Get text string
	if str, ok := op.Operands[0].(*entity.String); ok {
		return e.renderTextString(str.Value())
	}

	return fmt.Errorf("tj operand is not a string")
}

func (e *Evaluator) setTextMatrix(op Operator) error {
	if len(op.Operands) < 6 {
		return fmt.Errorf("tm operator requires 6 operands")
	}

	// Tm: Set text matrix directly
	// operands are [a b c d e f] representing the matrix
	var matrix [6]float64
	for i := 0; i < 6; i++ {
		num, err := getNumberOperand(op.Operands[i])
		if err != nil {
			// Be permissive for malformed content streams and continue rendering.
			return nil
		}
		matrix[i] = num
	}

	// In PDF, Tm replaces the current text matrix and text line matrix.
	e.syncTextMatricesState(matrix, matrix)
	e.syncPopplerTextBase(matrix, 0, 0)

	return nil
}

func (e *Evaluator) setFont(op Operator) error {
	if len(op.Operands) < 2 {
		return fmt.Errorf("tf operator requires 2 operands")
	}

	// Get font name and size
	fontName, ok := op.Operands[0].(entity.Name)
	if !ok {
		// Be permissive for malformed content streams and continue rendering.
		return nil
	}

	fontSize, err := getNumberOperand(op.Operands[1])
	if err != nil {
		// Be permissive for malformed content streams and continue rendering.
		return nil
	}

	// Load font from resources
	if e.resources == nil {
		// Be permissive for malformed content streams and continue rendering.
		return nil
	}

	// Get font dictionary from resources
	fontObj := e.getResourceEntry(entity.Name("Font"), fontName)
	if fontObj == nil {
		// Font not found in resources, try to use a default font
		// For now, just set the font size and continue
		e.graphics.currentState.SetFontSize(fontSize)
		return nil
	}

	// Parse font dictionary
	fontDict, ok := fontObj.(*entity.Dict)
	if !ok {
		if ref, ok := fontObj.(entity.Ref); ok && e.xref != nil {
			resolved, err := e.xref.Fetch(ref)
			if err != nil {
				return fmt.Errorf("failed to resolve font %s: %w", fontName, err)
			}
			var resolvedDict *entity.Dict
			resolvedDict, ok = resolved.(*entity.Dict)
			if !ok {
				return fmt.Errorf("font %s is not a dictionary", fontName)
			}
			fontDict = resolvedDict
		} else {
			return fmt.Errorf("font %s is not a dictionary", fontName)
		}
	}

	// Get font type
	baseFont := ""
	if obj := fontDict.Get(entity.Name("BaseFont")); obj != nil {
		if name, ok := obj.(entity.Name); ok {
			baseFont = name.Value()
		}
	}

	// Create or get font instance — use cache to avoid re-resolving the same dict.
	var font entity.Font
	if cached, ok := e.fontCache[fontDict]; ok {
		font = cached
	} else {
		var resolveErr error
		font, resolveErr = e.getFontFromDict(fontDict, baseFont)
		if resolveErr != nil {
			font, _ = e.getDefaultFont(baseFont)
		}
		subtypeStr := nameValueForEncoding(fontDict.Get(entity.Name("Subtype")))
		embeddedFontData, embeddedErr := e.getEmbeddedFontData(fontDict)
		trustEmbeddedSubset := embeddedErr == nil && shouldSkipRenderabilityProbe(subtypeStr, embeddedFontData)
		if !trustEmbeddedSubset && !e.isRenderableFont(font) {
			font, _ = e.getDefaultFont(baseFont)
		}
		if font != nil {
			e.fontCache[fontDict] = font
		}
	}

	// Set font in graphics state
	e.graphics.currentState.SetFont(font)
	e.graphics.currentState.SetFontSize(fontSize)
	e.graphics.font = font
	e.graphics.fontDebugName = baseFont
	e.graphics.fontSize = fontSize

	return nil
}

// getFontFromDict creates a Font instance from a font dictionary
func (e *Evaluator) getFontFromDict(dict *entity.Dict, baseFont string) (entity.Font, error) {
	if dict == nil {
		return nil, fmt.Errorf("font dictionary is nil")
	}

	// Check font subtype
	subtypeObj := dict.Get(entity.Name("Subtype"))
	if subtypeObj == nil {
		return nil, fmt.Errorf("font dictionary missing Subtype")
	}

	subtypeStr := ""
	if name, ok := subtypeObj.(entity.Name); ok {
		subtypeStr = name.Value()
	}

	embeddedFontData, embeddedErr := e.getEmbeddedFontData(dict)
	candidateFont := e.fontResolver.ResolveCandidate(e, dict, subtypeStr, baseFont, embeddedFontData, embeddedErr)

	if candidateFont == nil {
		font, err := e.fontFallback.ResolveMissingCandidate(e, dict, subtypeStr, baseFont)
		if err != nil {
			return nil, err
		}
		candidateFont = font
	}
	if !shouldSkipRenderabilityProbe(subtypeStr, embeddedFontData) && !e.isRenderableFont(candidateFont) {
		if font, ok := e.fontFallback.ResolveNonRenderableCandidate(e, dict, subtypeStr, baseFont, candidateFont); ok {
			candidateFont = font
		}
	}

	candidateFont = e.applyFontEncodingFromDict(dict, candidateFont)
	candidateFont = e.applyFontMetricsFromDict(dict, candidateFont)
	candidateFont = e.applyEmbeddedType1CGlyphSourceFromDict(dict, candidateFont, embeddedFontData)
	candidateFont = e.applyEmbeddedSimpleFontBBoxFromDict(dict, candidateFont, embeddedFontData)
	return applyGlyphSourceOverrideFontForDebug(baseFont, candidateFont), nil
}

// resolveType3FontCandidate creates a Type3Font from a Type3 font dictionary.
func (e *Evaluator) resolveType3FontCandidate(dict *entity.Dict, baseFont string) entity.Font {
	if dict == nil {
		return nil
	}

	// Parse FontMatrix [a b c d e f]
	var fontMatrix [6]float64
	if fmObj := dict.Get(entity.Name("FontMatrix")); fmObj != nil {
		if fmArr, ok := fmObj.(*entity.Array); ok && fmArr.Len() == 6 {
			for i := 0; i < 6; i++ {
				if num, err := getNumberOperand(fmArr.Get(i)); err == nil {
					fontMatrix[i] = num
				}
			}
		}
	} else {
		// Default font matrix for Type3
		fontMatrix = [6]float64{0.001, 0, 0, 0.001, 0, 0}
	}

	// Parse FontBBox
	var bbox [4]float64
	if bbObj := dict.Get(entity.Name("FontBBox")); bbObj != nil {
		if bbArr, ok := bbObj.(*entity.Array); ok && bbArr.Len() == 4 {
			for i := 0; i < 4; i++ {
				if num, err := getNumberOperand(bbArr.Get(i)); err == nil {
					bbox[i] = num
				}
			}
		}
	}

	// Parse CharProcs dictionary
	charProcs := make(map[string]*entity.Stream)
	if cpObj := dict.Get(entity.Name("CharProcs")); cpObj != nil {
		if cpDict, ok := cpObj.(*entity.Dict); ok {
			for _, key := range cpDict.Keys() {
				val := cpDict.Get(key)
				if stream, ok := val.(*entity.Stream); ok {
					charProcs[strings.TrimPrefix(key.Value(), "/")] = stream
				} else if ref, ok := val.(entity.Ref); ok && e.xref != nil {
					if resolved, err := e.xref.Fetch(ref); err == nil {
						if stream, ok := resolved.(*entity.Stream); ok {
							charProcs[strings.TrimPrefix(key.Value(), "/")] = stream
						}
					}
				}
			}
		}
	}

	// Parse Encoding /Differences
	encoding := make(map[uint32]string)
	if encObj := dict.Get(entity.Name("Encoding")); encObj != nil {
		if encDict, ok := encObj.(*entity.Dict); ok {
			if diffObj := encDict.Get(entity.Name("Differences")); diffObj != nil {
				if diffArr, ok := diffObj.(*entity.Array); ok {
					parseEncodingDifferences(diffArr, encoding)
				}
			}
		}
	}

	// Parse Widths array
	var firstChar, lastChar uint32
	if fcObj := dict.Get(entity.Name("FirstChar")); fcObj != nil {
		if num, err := getNumberOperand(fcObj); err == nil {
			firstChar = uint32(num)
		}
	}
	if lcObj := dict.Get(entity.Name("LastChar")); lcObj != nil {
		if num, err := getNumberOperand(lcObj); err == nil {
			lastChar = uint32(num)
		}
	}

	widths := make(map[uint32]float64)
	if wObj := dict.Get(entity.Name("Widths")); wObj != nil {
		if wArr, ok := wObj.(*entity.Array); ok {
			for i := 0; i < wArr.Len() && firstChar+uint32(i) <= lastChar; i++ {
				if num, err := getNumberOperand(wArr.Get(i)); err == nil {
					widths[firstChar+uint32(i)] = num
				}
			}
		}
	}

	name := baseFont
	if name == "" {
		name = "Type3"
	}

	font := entity.NewType3Font(name, fontMatrix, charProcs, encoding, widths, firstChar, lastChar, bbox)
	if resources := e.resourceDictFromObject(dict.Get(entity.Name("Resources"))); resources != nil {
		font.SetResources(resources)
	}
	return font
}

func (e *Evaluator) resourceDictFromObject(obj entity.Object) *entity.Dict {
	resolved := e.resolveResourceEntryObject(obj, 0)
	if streamObj, ok := resolved.(*entity.Stream); ok {
		resolved = streamObj.Dict()
	}
	resources, _ := resolved.(*entity.Dict)
	return resources
}

func mergeType3ResourceDicts(base, override *entity.Dict) *entity.Dict {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}

	merged := entity.NewDict()
	for _, key := range base.Keys() {
		merged.Set(key, base.GetRaw(key))
	}
	for _, key := range override.Keys() {
		merged.Set(key, mergeType3ResourceEntry(merged.GetRaw(key), override.GetRaw(key)))
	}
	return merged
}

func mergeType3ResourceEntry(base, override entity.Object) entity.Object {
	baseDict, baseOK := base.(*entity.Dict)
	overrideDict, overrideOK := override.(*entity.Dict)
	if !baseOK || !overrideOK {
		return override
	}

	merged := entity.NewDict()
	for _, key := range baseDict.Keys() {
		merged.Set(key, baseDict.GetRaw(key))
	}
	for _, key := range overrideDict.Keys() {
		merged.Set(key, overrideDict.GetRaw(key))
	}
	return merged
}

// parseEncodingDifferences parses an Encoding /Differences array.
// Format: [code1 name1 name2 code2 name3 ...]
func parseEncodingDifferences(arr *entity.Array, encoding map[uint32]string) {
	currentCode := uint32(0)
	for i := 0; i < arr.Len(); i++ {
		item := arr.Get(i)
		if num, err := getNumberOperand(item); err == nil {
			currentCode = uint32(num)
		} else if name, ok := item.(entity.Name); ok {
			encoding[currentCode] = name.Value()
			currentCode++
		}
	}
}

func (e *Evaluator) isRenderableFont(font entity.Font) bool {
	if font == nil {
		return false
	}

	// Type3 fonts are rendered via content stream evaluation, not RenderGlyph.
	// Must unwrap wrappers (encodedFont, widthMappedFont, glyphSourceOverrideFont)
	// because the Type3Font may be buried under several layers.
	if unwrapType3Font(font) != nil {
		return true
	}

	// CID fonts (e.g., CIDFontType2 with Identity CIDToGIDMap) are subsetted TrueType fonts.
	// Their glyph IDs correspond to document-specific CIDs, not standard ASCII codes.
	// The standard test glyphs below ('A', 'a', '0', etc.) are not guaranteed to be in the
	// subset, so renderability tests would falsely fail. Trust embedded CID fonts as renderable.
	if font.IsCIDFont() {
		return true
	}

	testGlyphs := []uint32{'A', 'a', '0', ' '}
	for _, ch := range testGlyphs {
		glyph, err := font.CharCodeToGlyph(ch)
		if err != nil {
			continue
		}

		path, err := font.RenderGlyph(glyph, 12)
		if err != nil || path == nil {
			continue
		}

		if len(path.Commands) > 0 {
			return true
		}
	}

	// Also test low char codes (1-10) for TrueType subset fonts that use
	// sequential char codes starting from 1 (PDF spec 9.6.6.4).
	for ch := uint32(1); ch <= 10; ch++ {
		glyph, err := font.CharCodeToGlyph(ch)
		if err != nil {
			continue
		}
		path, err := font.RenderGlyph(glyph, 12)
		if err != nil || path == nil {
			continue
		}
		if len(path.Commands) > 0 {
			return true
		}
	}

	return false
}

func shouldSkipRenderabilityProbe(subtypeStr string, embeddedFontData []byte) bool {
	switch subtypeStr {
	case "CIDFontType0", "CIDFontType2":
		return true
	case "Type1":
		// Embedded Type1 subsets can contain only the glyphs used by the PDF.
		// Probing ASCII glyphs would incorrectly replace them with fallback fonts.
		return len(embeddedFontData) > 0
	default:
		return false
	}
}

// Text Style Operators

// setCharSpacing sets the character spacing - 'Tc' operator.
func (e *Evaluator) setCharSpacing(op Operator) error {
	if len(op.Operands) < 1 {
		return fmt.Errorf("tc operator requires 1 operand")
	}

	spacing, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("tc operator: invalid spacing value: %w", err)
	}

	e.graphics.currentState.SetCharSpacing(spacing)
	return nil
}

// setWordSpacing sets the word spacing - 'Tw' operator.
func (e *Evaluator) setWordSpacing(op Operator) error {
	if len(op.Operands) < 1 {
		return fmt.Errorf("tw operator requires 1 operand")
	}

	spacing, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("tw operator: invalid spacing value: %w", err)
	}

	e.graphics.currentState.SetWordSpacing(spacing)
	return nil
}

// setHorizScaling sets the horizontal scaling - 'Tz' operator.
func (e *Evaluator) setHorizScaling(op Operator) error {
	if len(op.Operands) < 1 {
		return fmt.Errorf("tz operator requires 1 operand")
	}

	scaling, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("tz operator: invalid scaling value: %w", err)
	}

	// Horizontal scaling is stored as a percentage (default 100)
	e.graphics.currentState.SetHorizontalScaling(scaling)

	return nil
}

// setTextLeading sets the text leading - 'TL' operator.
func (e *Evaluator) setTextLeading(op Operator) error {
	if len(op.Operands) < 1 {
		return fmt.Errorf("tl operator requires 1 operand")
	}

	leading, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("tl operator: invalid leading value: %w", err)
	}

	e.graphics.currentState.SetTextLeading(leading)
	return nil
}

// setTextRenderMode sets the text rendering mode - 'Tr' operator.
func (e *Evaluator) setTextRenderMode(op Operator) error {
	if len(op.Operands) < 1 {
		return fmt.Errorf("tr operator requires 1 operand")
	}

	if modeVal, ok := op.Operands[0].(*entity.Integer); ok {
		mode := int(modeVal.Value())
		e.graphics.currentState.SetTextRenderMode(mode)
	}

	return nil
}

// setTextRise sets the text rise - 'Ts' operator.
func (e *Evaluator) setTextRise(op Operator) error {
	if len(op.Operands) < 1 {
		return fmt.Errorf("ts operator requires 1 operand")
	}

	rise, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("ts operator: invalid rise value: %w", err)
	}

	e.graphics.currentState.SetTextRise(rise)
	return nil
}

// XObject handling
func (e *Evaluator) invokeXObject(op Operator) error {
	if len(op.Operands) < 1 {
		return fmt.Errorf("do operator requires 1 operand")
	}

	xname, ok := op.Operands[0].(entity.Name)
	if !ok {
		return fmt.Errorf("do operand is not a name")
	}
	if shouldSkipAllXObjectsForDebug() {
		return nil
	}

	// Get XObject from resources
	if e.resources == nil {
		return errors.NotFound("get_xobject", fmt.Errorf("no resources available"))
	}

	xobjVal := e.getResourceEntry(entity.Name("XObject"), xname)
	if xobjVal == nil {
		return errors.NotFound("get_xobject", fmt.Errorf("xobject %s not found", xname))
	}

	// XObject should be a stream dictionary
	xobj, ok := xobjVal.(*entity.Stream)
	if !ok {
		return fmt.Errorf("xobject %s is not a stream", xname)
	}

	// Get XObject subtype to determine how to handle it
	dict := xobj.Dict()
	subtypeVal := dict.Get(entity.Name("Subtype"))
	if subtypeVal == nil {
		return fmt.Errorf("xobject %s has no subtype", xname)
	}

	subtype, ok := subtypeVal.(entity.Name)
	if !ok {
		return fmt.Errorf("xobject subtype is not a name")
	}

	switch strings.TrimPrefix(subtype.Value(), "/") {
	case "Form":
		// Form XObject - evaluate its content stream
		return e.evaluateFormXObject(xobj, xname)
	case "Image":
		// Image XObject - handle image rendering
		return e.evaluateImageXObject(xobj, xname)
	default:
		return fmt.Errorf("unsupported XObject Subtype: %s", subtype)
	}
}

// evaluateFormXObject evaluates a form XObject's content stream.
func (e *Evaluator) evaluateFormXObject(xobj *entity.Stream, name entity.Name) error {
	ops, err := e.cachedFormOperators(xobj)
	if err != nil {
		return err
	}

	// Save current graphics state
	if err := e.saveState(); err != nil {
		return err
	}
	defer func() {
		// Always restore state after form evaluation
		_ = e.restoreState()
	}()

	// Poppler's Gfx::drawForm kills any pre-existing path before applying the
	// form matrix/BBox, while keeping the caller state available for restore.
	e.clearCurrentPathForForm()

	// Get form's dictionary for additional parameters
	dict := xobj.Dict()

	// Get form's matrix (transformation) if present
	matrixVal := dict.Get(entity.Name("Matrix"))
	if matrixVal != nil {
		if matrixArr, ok := matrixVal.(*entity.Array); ok && matrixArr.Len() == 6 {
			var formMatrix [6]float64
			for i := 0; i < 6; i++ {
				if elem := matrixArr.Get(i); elem != nil {
					num, err := getNumberOperand(elem)
					if err == nil {
						formMatrix[i] = num
					}
				}
			}
			// Concatenate form's matrix with current CTM
			e.concatenateMatrixToCTM(formMatrix)
		}
	}
	e.graphics.baseTransform = e.graphics.transform

	// Get form's resources if present, otherwise use current resources
	resourcesVal := dict.Get(entity.Name("Resources"))
	if resourcesVal != nil {
		if ref, ok := resourcesVal.(entity.Ref); ok && e.xref != nil {
			fetched, err := e.xref.Fetch(ref)
			if err == nil {
				resourcesVal = fetched
			}
		}
		if resourcesStream, ok := resourcesVal.(*entity.Stream); ok {
			resourcesVal = resourcesStream.Dict()
		}
		if resourcesDict, ok := resourcesVal.(*entity.Dict); ok {
			// Temporarily save current resources and use form's resources
			oldResources := e.resources
			e.resources = resourcesDict
			defer func() { e.resources = oldResources }()
		}
	}

	// Apply form bounding box clipping when present.
	bboxVal := dict.Get(entity.Name("BBox"))
	if bboxVal != nil {
		if bboxArr, ok := bboxVal.(*entity.Array); ok {
			if err := e.applyFormBBoxClip(bboxArr); err != nil {
				return errors.Invalid("form_bbox_clip", err)
			}
			e.clearCurrentPathForForm()
		}
	}

	e.executeCachedOperators(ops)

	return nil
}

// EvaluateFormXObject evaluates a Form XObject stream with the current evaluator state.
func (e *Evaluator) EvaluateFormXObject(xobj *entity.Stream, name entity.Name) error {
	return e.evaluateFormXObject(xobj, name)
}

func (e *Evaluator) clearCurrentPathForForm() {
	if e.graphics == nil {
		return
	}
	if e.graphics.path == nil {
		e.graphics.path = NewPath()
	} else {
		e.graphics.path.Clear()
	}
	e.graphics.pendingClip = false
	e.graphics.pendingClipMode = ClipNonZeroWinding
}

func (e *Evaluator) cachedFormOperators(xobj *entity.Stream) ([]Operator, error) {
	if xobj == nil {
		return nil, errors.Invalid("decode_form_xobject", fmt.Errorf("nil form xobject"))
	}

	if e.sharedFormCache != nil {
		if cached, ok := e.sharedFormCache.Get(xobj); ok {
			e.formOperatorCache[xobj] = cached
			return cached, nil
		}
	}

	if cached, ok := e.formOperatorCache[xobj]; ok {
		return cached, nil
	}

	infraStream := stream.NewFromEntity(xobj)
	data, err := infraStream.Decode()
	if err != nil {
		return nil, errors.Invalid("decode_form_xobject", err)
	}

	ops, err := e.parseOperatorsOnly(data)
	if err != nil {
		return nil, errors.Invalid("evaluate_form_xobject", err)
	}
	e.formOperatorCache[xobj] = ops
	if e.sharedFormCache != nil {
		e.sharedFormCache.Set(xobj, ops)
	}
	return ops, nil
}

func (e *Evaluator) applyFormBBoxClip(bboxArr *entity.Array) error {
	if bboxArr == nil || bboxArr.Len() != 4 {
		return nil
	}

	x0, err := getNumberOperand(bboxArr.Get(0))
	if err != nil {
		return fmt.Errorf("bbox x0 is not a number: %w", err)
	}
	y0, err := getNumberOperand(bboxArr.Get(1))
	if err != nil {
		return fmt.Errorf("bbox y0 is not a number: %w", err)
	}
	x1, err := getNumberOperand(bboxArr.Get(2))
	if err != nil {
		return fmt.Errorf("bbox x1 is not a number: %w", err)
	}
	y1, err := getNumberOperand(bboxArr.Get(3))
	if err != nil {
		return fmt.Errorf("bbox y1 is not a number: %w", err)
	}

	tx0, ty0 := e.transformPoint(x0, y0)
	tx1, ty1 := e.transformPoint(x1, y0)
	tx2, ty2 := e.transformPoint(x1, y1)
	tx3, ty3 := e.transformPoint(x0, y1)

	clipPath := NewPath()
	clipPath.AddElement(&MoveTo{X: tx0, Y: ty0})
	clipPath.AddElement(&LineTo{X: tx1, Y: ty1})
	clipPath.AddElement(&LineTo{X: tx2, Y: ty2})
	clipPath.AddElement(&LineTo{X: tx3, Y: ty3})
	clipPath.AddElement(&Close{})

	e.graphics.pathClip = clipPath
	e.graphics.clipMode = ClipNonZeroWinding
	if e.canvas != nil {
		e.applyClippingPath()
	}

	return nil
}

// evaluateImageXObject renders an image XObject to the canvas.
func resolveXObjectImageSourceFilter(filterObj entity.Object) domainimage.ImageFilter {
	filter, _ := resolveXObjectImageFilter(filterObj)
	return filter
}

func (e *Evaluator) resolveImageColorSpace(colorSpaceVal entity.Object) (string, bool) {
	return e.resolveImageColorSpaceWithDepth(colorSpaceVal, 0)
}

func (e *Evaluator) resolveICCBasedComponentCount(colorSpaceObj entity.Object) int {
	if colorSpaceObj == nil {
		return 0
	}

	switch colorSpaceObj.(type) {
	case *entity.Array:
		// ICCBased component count is carried by its profile dictionary N entry.
		n, ok := e.resolveICCBasedComponents(colorSpaceObj.(*entity.Array), 0)
		if ok {
			return n
		}
	case entity.Ref:
		if e.xref == nil {
			return 0
		}
		resolved, err := e.xref.Fetch(colorSpaceObj.(entity.Ref))
		if err != nil {
			return 0
		}
		return e.resolveICCBasedComponentCount(resolved)
	case *entity.Stream, *entity.Dict:
		n, ok := e.resolveICCBasedComponentValue(colorSpaceObj)
		if ok {
			return n
		}
	}

	return 0
}

func (e *Evaluator) resolveICCBasedProfile(colorSpaceVal entity.Object, depth int) ([]byte, bool) {
	return e.resolveICCBasedProfileWithDepth(colorSpaceVal, depth)
}

func (e *Evaluator) resolveICCBasedProfileWithDepth(colorSpaceVal entity.Object, depth int) ([]byte, bool) {
	if depth > 8 || colorSpaceVal == nil {
		return nil, false
	}

	switch cs := colorSpaceVal.(type) {
	case entity.Ref:
		if e.xref == nil {
			return nil, false
		}
		obj, err := e.xref.Fetch(cs)
		if err != nil {
			return nil, false
		}
		return e.resolveICCBasedProfileWithDepth(obj, depth+1)
	case *entity.Array:
		if cs.Len() == 0 {
			return nil, false
		}

		baseName, ok := e.resolveColorSpaceName(cs.Get(0), depth+1)
		if !ok {
			return nil, false
		}

		base := strings.TrimPrefix(baseName, "/")
		if strings.EqualFold(base, "ICCBased") {
			if cs.Len() < 2 {
				return nil, false
			}
			return e.resolveICCProfileObjectWithDepth(cs.Get(1), depth+1)
		}
		if strings.EqualFold(base, "Indexed") && cs.Len() >= 2 {
			return e.resolveICCBasedProfileWithDepth(cs.Get(1), depth+1)
		}
	case *entity.Stream:
		infra := stream.NewFromEntity(cs)
		raw, err := infra.Decode()
		if err == nil {
			return raw, true
		}
		if bytes := cs.RawBytes(); len(bytes) > 0 {
			return bytes, true
		}
	}

	return nil, false
}

func (e *Evaluator) resolveICCProfileObjectWithDepth(obj entity.Object, depth int) ([]byte, bool) {
	if depth > 8 || obj == nil {
		return nil, false
	}

	switch v := obj.(type) {
	case entity.Ref:
		if e.xref == nil {
			return nil, false
		}
		resolved, err := e.xref.Fetch(v)
		if err != nil {
			return nil, false
		}
		return e.resolveICCProfileObjectWithDepth(resolved, depth+1)
	case *entity.Stream:
		infra := stream.NewFromEntity(v)
		raw, err := infra.Decode()
		if err == nil {
			return raw, true
		}
		if bytes := v.RawBytes(); len(bytes) > 0 {
			return bytes, true
		}
	}

	return nil, false
}

func (e *Evaluator) isICCBasedColorSpace(colorSpaceVal entity.Object) bool {
	return e.isICCBasedColorSpaceWithDepth(colorSpaceVal, 0)
}

func (e *Evaluator) isICCBasedColorSpaceWithDepth(colorSpaceVal entity.Object, depth int) bool {
	if depth > 8 || colorSpaceVal == nil {
		return false
	}

	switch cs := colorSpaceVal.(type) {
	case entity.Name:
		name := normalizeImageColorSpaceName(cs.Value())
		return strings.EqualFold(name, "ICCBased")
	case entity.Ref:
		if e.xref == nil {
			return false
		}
		obj, err := e.xref.Fetch(cs)
		if err != nil {
			return false
		}
		return e.isICCBasedColorSpaceWithDepth(obj, depth+1)
	case *entity.Array:
		if cs.Len() == 0 {
			return false
		}

		baseName, ok := e.resolveColorSpaceName(cs.Get(0), depth+1)
		if !ok {
			return false
		}

		base := strings.TrimPrefix(baseName, "/")
		if strings.EqualFold(base, "ICCBased") {
			return true
		}
		if strings.EqualFold(base, "Indexed") && cs.Len() >= 2 {
			return e.isICCBasedColorSpaceWithDepth(cs.Get(1), depth+1)
		}
		return false
	default:
		return false
	}
}

func (e *Evaluator) resolveImageColorSpaceWithDepth(colorSpaceVal entity.Object, depth int) (string, bool) {
	if depth > 8 {
		return "", false
	}

	if colorSpaceVal == nil {
		return "DeviceRGB", true
	}

	switch cs := colorSpaceVal.(type) {
	case entity.Name:
		colorSpace := normalizeImageColorSpaceName(cs.Value())
		return colorSpace, isSupportedImageColorSpace(colorSpace)
	case entity.Ref:
		if e.xref == nil {
			return "", false
		}
		obj, err := e.xref.Fetch(cs)
		if err != nil {
			return "", false
		}
		return e.resolveImageColorSpaceWithDepth(obj, depth+1)
	case *entity.Array:
		if cs.Len() == 0 {
			return "", false
		}

		baseName, ok := e.resolveColorSpaceName(cs.Get(0), depth+1)
		if !ok {
			return "", false
		}

		base := strings.TrimPrefix(baseName, "/")
		if strings.EqualFold(base, "ICCBased") {
			components, ok := e.resolveICCBasedComponents(cs, depth+1)
			if !ok {
				return "", false
			}
			switch components {
			case 1:
				return "DeviceGray", true
			case 3:
				return "DeviceRGB", true
			case 4:
				return "DeviceCMYK", true
			default:
				return "", false
			}
		}

		colorSpace := normalizeImageColorSpaceName(base)
		return colorSpace, isSupportedImageColorSpace(colorSpace)
	default:
		return "", false
	}
}

func (e *Evaluator) resolveIndexedColorSpace(colorSpaceVal entity.Object, depth int) (string, []byte, bool) {
	if depth > 8 {
		return "", nil, false
	}

	switch cs := colorSpaceVal.(type) {
	case entity.Ref:
		if e.xref == nil {
			return "", nil, false
		}
		obj, err := e.xref.Fetch(cs)
		if err != nil {
			return "", nil, false
		}
		return e.resolveIndexedColorSpace(obj, depth+1)
	case *entity.Array:
		if cs.Len() < 4 {
			return "", nil, false
		}
		baseName, ok := e.resolveColorSpaceName(cs.Get(0), depth+1)
		baseTrimmed := strings.TrimPrefix(baseName, "/")
		if !ok || (!strings.EqualFold(baseTrimmed, "Indexed") && !strings.EqualFold(baseTrimmed, "I")) {
			return "", nil, false
		}

		base, ok := e.resolveImageColorSpaceWithDepth(cs.Get(1), depth+1)
		if !ok {
			return "", nil, false
		}

		lookup, ok := e.resolveIndexedLookupBytes(cs.Get(3), depth+1)
		if !ok {
			return "", nil, false
		}
		return base, lookup, true
	default:
		return "", nil, false
	}
}

func (e *Evaluator) resolveIndexedLookupBytes(obj entity.Object, depth int) ([]byte, bool) {
	if depth > 8 {
		return nil, false
	}

	switch v := obj.(type) {
	case entity.Ref:
		if e.xref == nil {
			return nil, false
		}
		resolved, err := e.xref.Fetch(v)
		if err != nil {
			return nil, false
		}
		return e.resolveIndexedLookupBytes(resolved, depth+1)
	case *entity.String:
		if v.IsHex() {
			hexText := strings.TrimSpace(v.Value())
			if len(hexText)%2 == 1 {
				hexText += "0"
			}
			decoded, err := hex.DecodeString(hexText)
			if err == nil {
				return decoded, true
			}
			// Some parser paths already materialize hex strings as raw bytes.
			return []byte(v.Value()), true
		}
		return []byte(v.Value()), true
	case *entity.Stream:
		infra := stream.NewFromEntity(v)
		decoded, err := infra.Decode()
		if err == nil {
			return decoded, true
		}
		return v.RawBytes(), true
	default:
		return nil, false
	}
}

func (e *Evaluator) resolveImageDecodeArray(obj entity.Object) []float64 {
	return e.resolveImageDecodeArrayWithDepth(obj, 0)
}

func (e *Evaluator) resolveImageDecodeArrayWithDepth(obj entity.Object, depth int) []float64 {
	if obj == nil || depth > 8 {
		return nil
	}
	switch v := obj.(type) {
	case entity.Ref:
		if e.xref == nil {
			return nil
		}
		resolved, err := e.xref.Fetch(v)
		if err != nil {
			return nil
		}
		return e.resolveImageDecodeArrayWithDepth(resolved, depth+1)
	case *entity.Array:
		if v.Len() == 0 {
			return nil
		}
		decode := make([]float64, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			n, err := getNumberOperand(v.Get(i))
			if err != nil {
				return nil
			}
			decode = append(decode, n)
		}
		return decode
	default:
		return nil
	}
}

func (e *Evaluator) resolveColorKeyMask(obj entity.Object, colorSpace string) *image.ColorKeyMask {
	arr := e.resolveMaskArray(obj, 0)
	if arr == nil || arr.Len() == 0 || arr.Len()%2 != 0 {
		return nil
	}

	components := arr.Len() / 2
	switch colorSpace {
	case "DeviceGray":
		if components != 1 {
			return nil
		}
	case "DeviceRGB":
		if components != 3 {
			return nil
		}
	case "DeviceCMYK":
		if components != 4 {
			return nil
		}
	default:
		// Do not apply color-key masks to unsupported/indirect spaces.
		return nil
	}

	ranges := make([][2]uint8, 0, components)
	for i := 0; i < arr.Len(); i += 2 {
		minVal, err := getNumberOperand(arr.Get(i))
		if err != nil {
			return nil
		}
		maxVal, err := getNumberOperand(arr.Get(i + 1))
		if err != nil {
			return nil
		}
		minByte := uint8(clamp(minVal, 0, 255))
		maxByte := uint8(clamp(maxVal, 0, 255))
		ranges = append(ranges, [2]uint8{minByte, maxByte})
	}

	return image.NewColorKeyMask(ranges, components)
}

func (e *Evaluator) resolveMaskArray(obj entity.Object, depth int) *entity.Array {
	if obj == nil || depth > 8 {
		return nil
	}
	switch v := obj.(type) {
	case *entity.Array:
		return v
	case entity.Ref:
		if e.xref == nil {
			return nil
		}
		resolved, err := e.xref.Fetch(v)
		if err != nil {
			return nil
		}
		return e.resolveMaskArray(resolved, depth+1)
	default:
		return nil
	}
}

func (e *Evaluator) resolveSoftMask(maskObj entity.Object) domainimage.ImageMask {
	if maskObj == nil {
		return nil
	}

	maskStream, ok := e.resolveStreamObject(maskObj)
	if !ok || maskStream == nil {
		return nil
	}

	width, ok := objectInt(maskStream.Dict().Get(entity.Name("Width")))
	if !ok || width <= 0 {
		return nil
	}
	height, ok := objectInt(maskStream.Dict().Get(entity.Name("Height")))
	if !ok || height <= 0 {
		return nil
	}

	data, maskFilter, err := decodeSoftMaskImageStream(maskStream)
	if err != nil {
		return nil
	}

	maskCS := "DeviceGray"
	if csObj := maskStream.Dict().Get(entity.Name("ColorSpace")); csObj != nil {
		cs, ok := e.resolveImageColorSpace(csObj)
		if !ok {
			return nil
		}
		maskCS = cs
	}
	bpc := 8
	if v, ok := objectInt(maskStream.Dict().Get(entity.Name("BitsPerComponent"))); ok && v > 0 {
		bpc = v
	}

	decoded, err := image.NewDecoder().Decode(&domainimage.ImageData{
		Data:             data,
		Width:            width,
		Height:           height,
		BitsPerComponent: bpc,
		ColorSpace:       domainimage.ColorSpace(maskCS),
		Filter:           maskFilter,
	})
	if err != nil || decoded == nil || decoded.Image() == nil {
		return nil
	}

	gray := stdimage.NewGray(decoded.Image().Bounds())
	for y := gray.Bounds().Min.Y; y < gray.Bounds().Max.Y; y++ {
		for x := gray.Bounds().Min.X; x < gray.Bounds().Max.X; x++ {
			gray.Set(x, y, color.GrayModel.Convert(decoded.Image().At(x, y)))
		}
	}

	return image.NewBitmapMaskFromImage(gray, false)
}

func decodeSoftMaskImageStream(maskStream *entity.Stream) ([]byte, domainimage.ImageFilter, error) {
	filterObj := maskStream.Dict().Get(entity.Name("Filter"))
	maskFilter, useEncodedData := resolveXObjectImageFilter(filterObj)
	encodedPrefixLen := 0
	if encodedFilter, prefixLen, ok := resolveXObjectEncodedFilterPipeline(filterObj); ok {
		maskFilter = encodedFilter
		useEncodedData = true
		encodedPrefixLen = prefixLen
	}

	if useEncodedData {
		data, err := decodeImageEncodedFilterPrefix(maskStream, encodedPrefixLen)
		if err != nil {
			return nil, domainimage.FilterNone, err
		}
		return data, maskFilter, nil
	}

	data, err := stream.NewFromEntity(maskStream).Decode()
	if err != nil {
		return nil, domainimage.FilterNone, err
	}
	return data, domainimage.FilterNone, nil
}

func (e *Evaluator) resolveStreamObject(obj entity.Object) (*entity.Stream, bool) {
	switch v := obj.(type) {
	case *entity.Stream:
		return v, true
	case entity.Ref:
		if e.xref == nil {
			return nil, false
		}
		resolved, err := e.xref.Fetch(v)
		if err != nil {
			return nil, false
		}
		s, ok := resolved.(*entity.Stream)
		return s, ok
	default:
		return nil, false
	}
}

func objectInt(obj entity.Object) (int, bool) {
	switch v := obj.(type) {
	case *entity.Integer:
		return int(v.Value()), true
	case *entity.Real:
		return int(v.Value()), true
	default:
		return 0, false
	}
}

func objectFloat(obj entity.Object) (float64, bool) {
	switch v := obj.(type) {
	case *entity.Integer:
		return float64(v.Value()), true
	case *entity.Real:
		return v.Value(), true
	default:
		return 0, false
	}
}

func (e *Evaluator) resolveColorSpaceName(obj entity.Object, depth int) (string, bool) {
	if depth > 8 {
		return "", false
	}
	switch v := obj.(type) {
	case entity.Name:
		return v.Value(), true
	case entity.Ref:
		if e.xref == nil {
			return "", false
		}
		resolved, err := e.xref.Fetch(v)
		if err != nil {
			return "", false
		}
		return e.resolveColorSpaceName(resolved, depth+1)
	default:
		return "", false
	}
}

func (e *Evaluator) resolveICCBasedComponents(cs *entity.Array, depth int) (int, bool) {
	if cs == nil || cs.Len() < 2 || depth > 8 {
		return 0, false
	}

	profileObj := cs.Get(1)
	switch v := profileObj.(type) {
	case entity.Ref:
		if e.xref == nil {
			return 0, false
		}
		resolved, err := e.xref.Fetch(v)
		if err != nil {
			return 0, false
		}
		return e.resolveICCBasedComponentValue(resolved)
	default:
		return e.resolveICCBasedComponentValue(v)
	}
}

func (e *Evaluator) resolveICCBasedComponentValue(obj entity.Object) (int, bool) {
	switch v := obj.(type) {
	case *entity.Stream:
		return parseICCBasedN(v.Dict())
	case *entity.Dict:
		return parseICCBasedN(v)
	default:
		return 0, false
	}
}

func parseICCBasedN(dict *entity.Dict) (int, bool) {
	if dict == nil {
		return 0, false
	}
	nObj := dict.Get(entity.Name("N"))
	switch n := nObj.(type) {
	case *entity.Integer:
		return int(n.Value()), true
	case *entity.Real:
		return int(n.Value()), true
	default:
		return 0, false
	}
}

func resolveXObjectImageFilter(filterObj entity.Object) (domainimage.ImageFilter, bool) {
	switch v := filterObj.(type) {
	case entity.Name:
		filter := normalizeImageFilterName(v.Value())
		return filter, isEncodedImageFilter(filter)
	case *entity.Array:
		if v.Len() == 0 {
			return domainimage.FilterNone, false
		}
		if v.Len() != 1 {
			// Keep behavior explicit: array filters are decoded through stream layer.
			// Use encoded pass-through only for single filter images.
			return domainimage.FilterNone, false
		}

		name, ok := v.Get(0).(entity.Name)
		if !ok {
			return domainimage.FilterNone, false
		}
		filter := normalizeImageFilterName(name.Value())
		return filter, isEncodedImageFilter(filter)
	default:
		return domainimage.FilterNone, false
	}
}

func resolveXObjectEncodedFilterPipeline(filterObj entity.Object) (domainimage.ImageFilter, int, bool) {
	switch v := filterObj.(type) {
	case entity.Name:
		filter := normalizeImageFilterName(v.Value())
		return filter, 0, isEncodedImageFilter(filter)
	case *entity.Array:
		if v.Len() == 0 {
			return domainimage.FilterNone, 0, false
		}
		lastName, ok := v.Get(v.Len() - 1).(entity.Name)
		if !ok {
			return domainimage.FilterNone, 0, false
		}
		filter := normalizeImageFilterName(lastName.Value())
		if !isEncodedImageFilter(filter) {
			return filter, 0, false
		}
		for i := 0; i < v.Len()-1; i++ {
			name, ok := v.Get(i).(entity.Name)
			if !ok {
				return domainimage.FilterNone, 0, false
			}
			if !isGenericImageEncodedPrefixFilter(normalizeImageFilterName(name.Value())) {
				return domainimage.FilterNone, 0, false
			}
		}
		return filter, v.Len() - 1, true
	default:
		return domainimage.FilterNone, 0, false
	}
}

func isGenericImageEncodedPrefixFilter(filter domainimage.ImageFilter) bool {
	switch filter {
	case domainimage.FilterASCIIHex, domainimage.FilterASCII85, domainimage.FilterFlate, domainimage.FilterLZW, domainimage.FilterRunLength:
		return true
	default:
		return false
	}
}

func decodeImageEncodedFilterPrefix(xobj *entity.Stream, prefixLen int) ([]byte, error) {
	if xobj == nil {
		return nil, fmt.Errorf("image stream is nil")
	}
	if prefixLen <= 0 {
		return xobj.RawBytes(), nil
	}

	filterArray, ok := xobj.Dict().Get(entity.Name("Filter")).(*entity.Array)
	if !ok || filterArray.Len() < prefixLen {
		return xobj.RawBytes(), nil
	}

	prefixDict := entity.NewDict()
	if prefixLen == 1 {
		prefixDict.Set(entity.Name("Filter"), filterArray.Get(0))
	} else {
		filters := make([]entity.Object, 0, prefixLen)
		for i := 0; i < prefixLen; i++ {
			filters = append(filters, filterArray.Get(i))
		}
		prefixDict.Set(entity.Name("Filter"), entity.NewArray(filters...))
	}
	if decodeParms := prefixDecodeParms(xobj.Dict().Get(entity.Name("DecodeParms")), prefixLen); decodeParms != nil {
		prefixDict.Set(entity.Name("DecodeParms"), decodeParms)
	}

	return stream.NewFromEntity(entity.NewStream(prefixDict, xobj.RawBytes())).Decode()
}

func prefixDecodeParms(decodeParms entity.Object, prefixLen int) entity.Object {
	if decodeParms == nil || prefixLen <= 0 {
		return nil
	}
	if prefixLen == 1 {
		if arr, ok := decodeParms.(*entity.Array); ok && arr.Len() > 0 {
			return arr.Get(0)
		}
		return decodeParms
	}

	arr, ok := decodeParms.(*entity.Array)
	if !ok {
		return decodeParms
	}
	items := make([]entity.Object, 0, prefixLen)
	for i := 0; i < prefixLen && i < arr.Len(); i++ {
		items = append(items, arr.Get(i))
	}
	return entity.NewArray(items...)
}

func normalizeImageFilterName(name string) domainimage.ImageFilter {
	normalized := strings.TrimSpace(strings.TrimPrefix(name, "/"))
	switch strings.ToLower(normalized) {
	case "ahx", "asciihex", "asciihexdecode":
		return domainimage.FilterASCIIHex
	case "a85", "ascii85", "ascii85decode":
		return domainimage.FilterASCII85
	case "fl", "flate", "flatedecode":
		return domainimage.FilterFlate
	case "lzw", "lzwdecode":
		return domainimage.FilterLZW
	case "rl", "runlength", "runlengthdecode":
		return domainimage.FilterRunLength
	case "ccf", "ccitt", "ccittfax", "ccittfaxdecode":
		return domainimage.FilterCCITTFax
	case "dct", "dctdecode":
		return domainimage.FilterDCT
	case "jpx", "jpxdecode":
		return domainimage.FilterJPX
	case "jbig2", "jbig2decode":
		return domainimage.FilterJBIG2
	default:
		return domainimage.ImageFilter(normalized)
	}
}

func isEncodedImageFilter(filter domainimage.ImageFilter) bool {
	switch filter {
	case domainimage.FilterDCT, domainimage.FilterJPX, domainimage.FilterJBIG2:
		return true
	default:
		return false
	}
}

func normalizeImageColorSpaceName(name string) string {
	normalized := strings.TrimPrefix(name, "/")
	switch normalized {
	case "G":
		return "DeviceGray"
	case "RGB":
		return "DeviceRGB"
	case "CMYK":
		return "DeviceCMYK"
	case "I":
		return "Indexed"
	default:
		return normalized
	}
}

func isSupportedImageColorSpace(name string) bool {
	switch name {
	case "DeviceGray", "DeviceRGB", "DeviceCMYK", "Indexed":
		return true
	default:
		return false
	}
}

// renderPlaceholderImage renders a placeholder rectangle for failed image decoding.
func (e *Evaluator) renderPlaceholderImage(width, height float64) {
	x, y := e.transformPoint(0, 0)
	e.canvas.MoveTo(x, y)
	e.canvas.LineTo(x+width, y)
	e.canvas.LineTo(x+width, y+height)
	e.canvas.LineTo(x, y+height)
	e.canvas.ClosePath()
	e.canvas.Stroke()
}

// concatenateMatrixToCTM concatenates a matrix to the current transformation matrix.
func (e *Evaluator) concatenateMatrixToCTM(matrix [6]float64) {
	e.graphics.transform = multiplyMatrix(e.graphics.transform, matrix)
}

// Color Operators

func (e *Evaluator) setStrokeColorSpace(op Operator) error {
	if len(op.Operands) < 1 {
		return nil
	}
	csName, ok := op.Operands[0].(entity.Name)
	if !ok {
		return nil
	}
	e.graphics.strokeCS, e.graphics.strokePatternBaseCS = e.resolveGraphicsColorSpaceAndPatternBase(csName)
	if !strings.EqualFold(e.graphics.strokeCS, "Pattern") {
		e.graphics.strokePattern = nil
		e.graphics.strokePatternBaseCS = ""
	}
	return nil
}

func (e *Evaluator) setFillColorSpace(op Operator) error {
	if len(op.Operands) < 1 {
		return nil
	}
	csName, ok := op.Operands[0].(entity.Name)
	if !ok {
		return nil
	}
	e.graphics.fillCS, e.graphics.fillPatternBaseCS = e.resolveGraphicsColorSpaceAndPatternBase(csName)
	if os.Getenv("PDF_DEBUG_PATTERN_RESOLVE") == "1" {
		fmt.Fprintf(os.Stderr, "DEBUG setFillColorSpace: name=%s fillCS=%s patternBase=%s resources=%p\n",
			csName.String(), e.graphics.fillCS, e.graphics.fillPatternBaseCS, e.resources)
	}
	if !strings.EqualFold(e.graphics.fillCS, "Pattern") {
		e.graphics.fillPattern = nil
		e.graphics.fillPatternBaseCS = ""
	}
	return nil
}

func (e *Evaluator) resolveGraphicsColorSpace(name entity.Name) string {
	colorSpace, _ := e.resolveGraphicsColorSpaceAndPatternBase(name)
	return colorSpace
}

func (e *Evaluator) resolveGraphicsColorSpaceAndPatternBase(name entity.Name) (string, string) {
	base := normalizeImageColorSpaceName(name.Value())
	if strings.EqualFold(base, "Pattern") {
		return "Pattern", ""
	}
	if patternBase, ok := e.resolvePatternColorSpaceBase(name); ok {
		return "Pattern", patternBase
	}

	if e.resources != nil {
		if csObj := e.getResourceEntry(entity.Name("ColorSpace"), name); csObj != nil {
			if resolved, ok := e.resolveImageColorSpace(csObj); ok {
				return resolved, ""
			}
		}
	}

	if isSupportedImageColorSpace(base) {
		return base, ""
	}

	return "DeviceRGB", ""
}

func (e *Evaluator) isPatternColorSpaceResource(name entity.Name) bool {
	_, ok := e.resolvePatternColorSpaceBase(name)
	return ok
}

func (e *Evaluator) resolvePatternColorSpaceBase(name entity.Name) (string, bool) {
	if e.resources == nil {
		return "", false
	}
	colorSpaceObj := e.getResourceEntry(entity.Name("ColorSpace"), name)
	return e.resolvePatternColorSpaceBaseObject(colorSpaceObj, 0)
}

func (e *Evaluator) resolvePatternColorSpaceBaseObject(obj entity.Object, depth int) (string, bool) {
	if obj == nil || depth > 8 {
		return "", false
	}
	switch typed := obj.(type) {
	case entity.Ref:
		if e.xref == nil {
			return "", false
		}
		resolved, err := e.xref.Fetch(typed)
		if err != nil {
			return "", false
		}
		return e.resolvePatternColorSpaceBaseObject(resolved, depth+1)
	case entity.Name:
		if strings.EqualFold(normalizeImageColorSpaceName(typed.Value()), "Pattern") {
			return "", true
		}
		return "", false
	case *entity.Array:
		if typed.Len() < 1 {
			return "", false
		}
		baseName, ok := e.resolveColorSpaceName(typed.Get(0), depth+1)
		if !ok || !strings.EqualFold(normalizeImageColorSpaceName(baseName), "Pattern") {
			return "", false
		}
		if typed.Len() < 2 {
			return "", true
		}
		if resolved, ok := e.resolveImageColorSpaceWithDepth(typed.Get(1), depth+1); ok {
			return resolved, true
		}
		if baseRef, ok := typed.Get(1).(entity.Name); ok && e.resources != nil {
			if resolvedObj := e.getResourceEntry(entity.Name("ColorSpace"), baseRef); resolvedObj != nil {
				if resolved, ok := e.resolveImageColorSpaceWithDepth(resolvedObj, depth+1); ok {
					return resolved, true
				}
			}
		}
		return "", true
	default:
		return "", false
	}
}

func numericColorOperands(operands []entity.Object) []float64 {
	out := make([]float64, 0, len(operands))
	for _, obj := range operands {
		v, err := getNumberOperand(obj)
		if err != nil {
			continue
		}
		out = append(out, clamp(v, 0, 1))
	}
	return out
}

func splitColorAndPatternOperands(operands []entity.Object) ([]float64, *entity.Name) {
	values := make([]float64, 0, len(operands))
	var pattern *entity.Name
	for i, obj := range operands {
		if i == len(operands)-1 {
			if name, ok := obj.(entity.Name); ok {
				pattern = &name
				continue
			}
		}

		v, err := getNumberOperand(obj)
		if err != nil {
			continue
		}
		values = append(values, clamp(v, 0, 1))
	}
	return values, pattern
}

func (e *Evaluator) setStrokeColorBySpace(op Operator) error {
	return e.setColorBySpace(op, true)
}

func (e *Evaluator) setFillColorBySpace(op Operator) error {
	return e.setColorBySpace(op, false)
}

func (e *Evaluator) setColorBySpace(op Operator, stroke bool) error {
	values, patternName := splitColorAndPatternOperands(op.Operands)
	colorSpace := e.graphics.fillCS
	patternBaseCS := e.graphics.fillPatternBaseCS
	if stroke {
		colorSpace = e.graphics.strokeCS
		patternBaseCS = e.graphics.strokePatternBaseCS
	}
	isPatternSpace := strings.EqualFold(colorSpace, "Pattern")
	if os.Getenv("PDF_DEBUG_PATTERN_RESOLVE") == "1" {
		patternLabel := "<nil>"
		if patternName != nil {
			patternLabel = patternName.String()
		}
		fmt.Fprintf(os.Stderr, "DEBUG setColorBySpace: opcode=%s stroke=%v cs=%s patternBase=%s values=%v pattern=%s operands=%v resources=%p\n",
			op.Opcode, stroke, colorSpace, patternBaseCS, values, patternLabel, op.Operands, e.resources)
	}
	if len(values) == 0 && patternName == nil {
		if isPatternSpace {
			if stroke {
				e.graphics.strokePattern = nil
			} else {
				e.graphics.fillPattern = nil
			}
		} else if stroke {
			e.graphics.strokePattern = nil
		} else {
			e.graphics.fillPattern = nil
		}
		return nil
	}

	if isPatternSpace {
		if len(values) > 0 {
			e.applyColorValuesBySpaceForGraphicsState(values, patternBaseCS, stroke)
		}
		if patternName != nil {
			pattern, err := e.resolvePattern(*patternName)
			if os.Getenv("PDF_DEBUG_PATTERN_RESOLVE") == "1" {
				shadingType := "<nil>"
				patches := 0
				if shp, ok := pattern.(*entity.ShadingPattern); ok && shp.GetShading() != nil {
					shadingType = shp.GetShading().GetShadingType().String()
					patches = len(shp.GetShading().GetPatches())
				}
				fmt.Fprintf(os.Stderr, "DEBUG resolvePattern: name=%s pattern=%T err=%v shadingType=%s patches=%d\n",
					patternName.String(), pattern, err, shadingType, patches)
			}
			if err == nil && pattern != nil {
				if stroke {
					e.graphics.strokePattern = pattern
				} else {
					e.graphics.fillPattern = pattern
				}
			} else {
				if stroke {
					e.graphics.strokePattern = nil
				} else {
					e.graphics.fillPattern = nil
				}
			}
		} else if stroke {
			e.graphics.strokePattern = nil
		} else {
			e.graphics.fillPattern = nil
		}
		return nil
	}

	if len(values) == 0 {
		return nil
	}

	e.applyColorValuesBySpaceForGraphicsState(values, colorSpace, stroke)
	return nil
}

func (e *Evaluator) applyColorValuesBySpaceForGraphicsState(
	values []float64,
	colorSpace string,
	stroke bool,
) {
	setHex := func(hex string) {
		if stroke {
			e.graphics.strokeColor = &ColorSpace{Color: &Color{Hex: hex}}
			return
		}
		e.graphics.fillColor = &ColorSpace{Color: &Color{Hex: hex}}
	}

	switch colorSpace {
	case "DeviceGray":
		setHex(grayToHex(values[0], values[0], values[0]))
		return
	case "DeviceCMYK":
		if len(values) < 4 {
			return
		}
		r, g, b := cmykToRGB(values[0], values[1], values[2], values[3])
		setHex(grayToHex(r, g, b))
		return
	default:
		// DeviceRGB and Pattern fallback use first RGB components when present.
		if len(values) >= 3 {
			setHex(grayToHex(values[0], values[1], values[2]))
			return
		}
		setHex(grayToHex(values[0], values[0], values[0]))
		return
	}
}

func (e *Evaluator) resolvePattern(name entity.Name) (entity.Pattern, error) {
	if e.resources == nil {
		return nil, fmt.Errorf("no resources for pattern %s", name)
	}

	patternObj := e.getResourceEntry(entity.Name("Pattern"), name)
	if patternObj == nil {
		return nil, fmt.Errorf("pattern %s not found", name)
	}

	var patternDict *entity.Dict
	var streamContent []byte

	switch patternValue := patternObj.(type) {
	case *entity.Stream:
		patternDict = patternValue.Dict()
		decoded, err := stream.NewFromEntity(patternValue).Decode()
		if err != nil {
			streamContent = patternValue.RawBytes()
		} else {
			streamContent = decoded
		}
	case *entity.Dict:
		patternDict = patternValue
	default:
		return nil, fmt.Errorf("unsupported pattern object type: %T", patternObj)
	}
	if patternDict == nil {
		return nil, fmt.Errorf("pattern dictionary missing for %s", name)
	}

	patternType := 1
	if v := patternDict.Get(entity.Name("PatternType")); v != nil {
		parsed, err := objectIntStrict(v)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern type: %w", err)
		}
		patternType = parsed
	}

	matrix, ok := parseMatrix(patternDict.Get(entity.Name("Matrix")))
	if !ok {
		matrix = [6]float64{1, 0, 0, 1, 0, 0}
	}

	switch patternType {
	case 1:
		paintType := 1
		tilingType := entity.TilingConstantSpacing

		if v := patternDict.Get(entity.Name("PaintType")); v != nil {
			if parsed, err := objectIntStrict(v); err == nil {
				paintType = parsed
			}
		}
		if v := patternDict.Get(entity.Name("TilingType")); v != nil {
			if parsed, err := objectIntStrict(v); err == nil {
				tilingType = entity.TilingType(parsed)
			}
		}

		pattern := entity.NewTilingPattern(name.String(), paintType, tilingType)
		pattern.SetMatrix(matrix)

		if bboxObj := patternDict.Get(entity.Name("BBox")); bboxObj != nil {
			if arr, ok := bboxObj.(*entity.Array); ok && arr.Len() >= 4 {
				bbox := [4]float64{
					getNumericOrZero(arr.Get(0)),
					getNumericOrZero(arr.Get(1)),
					getNumericOrZero(arr.Get(2)),
					getNumericOrZero(arr.Get(3)),
				}
				pattern.SetBBox(bbox)
			}
		}
		if xStep := patternDict.Get(entity.Name("XStep")); xStep != nil {
			if value, err := getNumberOperand(xStep); err == nil {
				pattern.SetXStep(value)
			}
		}
		if yStep := patternDict.Get(entity.Name("YStep")); yStep != nil {
			if value, err := getNumberOperand(yStep); err == nil {
				pattern.SetYStep(value)
			}
		}
		if resourcesObj := patternDict.Get(entity.Name("Resources")); resourcesObj != nil {
			if resources, ok := resourcesObj.(*entity.Dict); ok {
				pattern.SetResources(resources)
			}
		}
		pattern.SetContent(streamContent)
		return pattern, nil

	case 2:
		shadingObj := patternDict.Get(entity.Name("Shading"))
		shading, err := e.parsePatternShading(shadingObj)
		if err != nil {
			return nil, err
		}

		pattern := entity.NewShadingPattern(name.String(), shading)
		pattern.SetMatrix(matrix)
		return pattern, nil

	default:
		return nil, fmt.Errorf("unsupported pattern type: %d", patternType)
	}
}

func (e *Evaluator) parsePatternShading(obj entity.Object) (*entity.Shading, error) {
	return e.parseShadingObject(obj)
}

func objectIntStrict(obj entity.Object) (int, error) {
	switch v := obj.(type) {
	case *entity.Integer:
		return int(v.Value()), nil
	case *entity.Real:
		return int(v.Value()), nil
	default:
		return 0, fmt.Errorf("not a number")
	}
}

func getNumericOrZero(obj entity.Object) float64 {
	num, err := getNumberOperand(obj)
	if err != nil {
		return 0
	}
	return num
}

func parseMatrix(obj entity.Object) ([6]float64, bool) {
	var matrix [6]float64
	matrix = [6]float64{1, 0, 0, 1, 0, 0}
	arr, ok := obj.(*entity.Array)
	if !ok || arr.Len() < 6 {
		return matrix, false
	}

	for i := 0; i < 6; i++ {
		v, err := getNumberOperand(arr.Get(i))
		if err != nil {
			return matrix, false
		}
		matrix[i] = v
	}
	return matrix, true
}

// setGrayStroke sets the gray color for stroking operations - 'G' operator.
func (e *Evaluator) setGrayStroke(op Operator) error {
	if len(op.Operands) < 1 {
		return fmt.Errorf("operator G requires 1 operand")
	}

	gray, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("operator G: invalid gray value: %w", err)
	}

	// Clamp value to [0, 1]
	if gray < 0 {
		gray = 0
	} else if gray > 1 {
		gray = 1
	}

	// Convert gray to RGB hex string
	hex := grayToHex(gray, gray, gray)
	e.graphics.strokeColor = &ColorSpace{Color: &Color{Hex: hex}}
	e.graphics.strokeCS = "DeviceGray"

	return nil
}

// setGrayFill sets the gray color for filling operations - 'g' operator.
func (e *Evaluator) setGrayFill(op Operator) error {
	if len(op.Operands) < 1 {
		return fmt.Errorf("g operator requires 1 operand")
	}

	gray, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("g operator: invalid gray value: %w", err)
	}

	// Clamp value to [0, 1]
	if gray < 0 {
		gray = 0
	} else if gray > 1 {
		gray = 1
	}

	// Convert gray to RGB hex string
	hex := grayToHex(gray, gray, gray)
	e.graphics.fillColor = &ColorSpace{Color: &Color{Hex: hex}}
	e.graphics.fillCS = "DeviceGray"

	return nil
}

// setRGBStroke sets the RGB color for stroking operations - 'RG' operator.
func (e *Evaluator) setRGBStroke(op Operator) error {
	if len(op.Operands) < 3 {
		return fmt.Errorf("RG operator requires 3 operands")
	}

	r, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("RG operator: invalid red value: %w", err)
	}

	g, err := getNumberOperand(op.Operands[1])
	if err != nil {
		return fmt.Errorf("RG operator: invalid green value: %w", err)
	}

	b, err := getNumberOperand(op.Operands[2])
	if err != nil {
		return fmt.Errorf("RG operator: invalid blue value: %w", err)
	}

	// Clamp values to [0, 1]
	r = clamp(r, 0, 1)
	g = clamp(g, 0, 1)
	b = clamp(b, 0, 1)

	// Convert RGB to hex string
	hex := grayToHex(r, g, b)
	e.graphics.strokeColor = &ColorSpace{Color: &Color{Hex: hex}}
	e.graphics.strokeCS = "DeviceRGB"

	return nil
}

// setRGBFill sets the RGB color for filling operations - 'rg' operator.
func (e *Evaluator) setRGBFill(op Operator) error {
	if len(op.Operands) < 3 {
		return fmt.Errorf("rg operator requires 3 operands")
	}

	r, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("rg operator: invalid red value: %w", err)
	}

	g, err := getNumberOperand(op.Operands[1])
	if err != nil {
		return fmt.Errorf("rg operator: invalid green value: %w", err)
	}

	b, err := getNumberOperand(op.Operands[2])
	if err != nil {
		return fmt.Errorf("rg operator: invalid blue value: %w", err)
	}

	// Clamp values to [0, 1]
	r = clamp(r, 0, 1)
	g = clamp(g, 0, 1)
	b = clamp(b, 0, 1)

	// Convert RGB to hex string
	hex := grayToHex(r, g, b)
	e.graphics.fillColor = &ColorSpace{Color: &Color{Hex: hex}}
	e.graphics.fillCS = "DeviceRGB"

	return nil
}

// setCMYKStroke sets the CMYK color for stroking operations - 'K' operator.
func (e *Evaluator) setCMYKStroke(op Operator) error {
	if len(op.Operands) < 4 {
		return fmt.Errorf("operator K requires 4 operands")
	}

	c, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("operator K: invalid cyan value: %w", err)
	}

	m, err := getNumberOperand(op.Operands[1])
	if err != nil {
		return fmt.Errorf("operator K: invalid magenta value: %w", err)
	}

	y, err := getNumberOperand(op.Operands[2])
	if err != nil {
		return fmt.Errorf("operator K: invalid yellow value: %w", err)
	}

	k, err := getNumberOperand(op.Operands[3])
	if err != nil {
		return fmt.Errorf("operator K: invalid black value: %w", err)
	}

	// Convert CMYK to RGB
	r, g, b := cmykToRGB(c, m, y, k)

	// Convert RGB to hex string
	hex := grayToHex(r, g, b)
	e.graphics.strokeColor = &ColorSpace{Color: &Color{Hex: hex}}
	e.graphics.strokeCS = "DeviceCMYK"

	return nil
}

// setCMYKFill sets the CMYK color for filling operations - 'k' operator.
func (e *Evaluator) setCMYKFill(op Operator) error {
	if len(op.Operands) < 4 {
		return fmt.Errorf("k operator requires 4 operands")
	}

	c, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("k operator: invalid cyan value: %w", err)
	}

	m, err := getNumberOperand(op.Operands[1])
	if err != nil {
		return fmt.Errorf("k operator: invalid magenta value: %w", err)
	}

	y, err := getNumberOperand(op.Operands[2])
	if err != nil {
		return fmt.Errorf("k operator: invalid yellow value: %w", err)
	}

	k, err := getNumberOperand(op.Operands[3])
	if err != nil {
		return fmt.Errorf("k operator: invalid black value: %w", err)
	}

	// Convert CMYK to RGB
	r, g, b := cmykToRGB(c, m, y, k)

	// Convert RGB to hex string
	hex := grayToHex(r, g, b)
	e.graphics.fillColor = &ColorSpace{Color: &Color{Hex: hex}}
	e.graphics.fillCS = "DeviceCMYK"

	return nil
}

// setLineWidth sets the line width - 'w' operator.
func (e *Evaluator) setLineWidth(op Operator) error {
	if len(op.Operands) < 1 {
		return fmt.Errorf("w operator requires 1 operand")
	}

	width, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("w operator: invalid width value: %w", err)
	}

	// Line width must be positive
	if width < 0 {
		width = 0
	}

	e.graphics.lineWidth = width
	e.graphics.currentState.SetLineWidth(width)

	return nil
}

// setLineCap sets the line cap style - 'J' operator.
func (e *Evaluator) setLineCap(op Operator) error {
	if len(op.Operands) < 1 {
		return fmt.Errorf("j operator requires 1 operand")
	}

	// Store line cap style in currentState
	if styleVal, ok := op.Operands[0].(*entity.Integer); ok {
		e.graphics.currentState.SetLineCap(int(styleVal.Value()))
	}

	return nil
}

// setLineJoin sets the line join style - 'j' operator.
func (e *Evaluator) setLineJoin(op Operator) error {
	if len(op.Operands) < 1 {
		return fmt.Errorf("j operator requires 1 operand")
	}

	// Store line join style in currentState
	if styleVal, ok := op.Operands[0].(*entity.Integer); ok {
		e.graphics.currentState.SetLineJoin(int(styleVal.Value()))
	}

	return nil
}

// setMiterLimit sets the miter limit - 'M' operator.
func (e *Evaluator) setMiterLimit(op Operator) error {
	if len(op.Operands) < 1 {
		return fmt.Errorf("m operator requires 1 operand")
	}

	limit, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("m operator: invalid limit value: %w", err)
	}

	// Store miter limit in the graphics state
	// The miter limit is used when stroke rendering with miter joins
	// If the miter length would exceed the miter limit, a bevel join is used instead
	e.graphics.currentState.SetMiterLimit(limit)

	return nil
}

// setDashPattern sets the line dash pattern - 'd' operator.
func (e *Evaluator) setDashPattern(op Operator) error {
	if len(op.Operands) < 2 {
		return fmt.Errorf("d operator requires 2 operands")
	}

	dashArrayObj, ok := op.Operands[0].(*entity.Array)
	if !ok {
		return fmt.Errorf("d operator: first operand must be an array")
	}

	phase, err := getNumberOperand(op.Operands[1])
	if err != nil {
		return fmt.Errorf("d operator: invalid phase value: %w", err)
	}

	dashArray := make([]float64, 0, dashArrayObj.Len())
	for i := 0; i < dashArrayObj.Len(); i++ {
		value, numErr := getNumberOperand(dashArrayObj.Get(i))
		if numErr != nil {
			return fmt.Errorf("d operator: invalid dash value at index %d: %w", i, numErr)
		}
		dashArray = append(dashArray, value)
	}

	e.graphics.currentState.SetDashArray(dashArray, phase)
	return nil
}

// applyGraphicsStateParameters applies an ExtGState dictionary - 'gs' operator.
func (e *Evaluator) applyGraphicsStateParameters(op Operator) error {
	if len(op.Operands) < 1 {
		return nil
	}

	gsName, ok := op.Operands[0].(entity.Name)
	if !ok {
		return nil
	}

	gsObj := e.getResourceEntry(entity.Name("ExtGState"), gsName)
	if gsObj == nil {
		return nil
	}

	if ref, isRef := gsObj.(entity.Ref); isRef {
		fetched, err := e.xref.Fetch(ref)
		if err != nil {
			return nil
		}
		gsObj = fetched
	}

	var gsDict *entity.Dict
	switch resolved := gsObj.(type) {
	case *entity.Dict:
		gsDict = resolved
	case *entity.Stream:
		gsDict = resolved.Dict()
	default:
		return nil
	}
	if gsDict == nil {
		return nil
	}

	if lw := gsDict.Get(entity.Name("LW")); lw != nil {
		_ = e.setLineWidth(Operator{Opcode: "w", Operands: []entity.Object{lw}})
	}
	if lc := gsDict.Get(entity.Name("LC")); lc != nil {
		_ = e.setLineCap(Operator{Opcode: "J", Operands: []entity.Object{lc}})
	}
	if lj := gsDict.Get(entity.Name("LJ")); lj != nil {
		_ = e.setLineJoin(Operator{Opcode: "j", Operands: []entity.Object{lj}})
	}
	if ml := gsDict.Get(entity.Name("ML")); ml != nil {
		_ = e.setMiterLimit(Operator{Opcode: "M", Operands: []entity.Object{ml}})
	}
	if dash := gsDict.Get(entity.Name("D")); dash != nil {
		if dashArray, ok := dash.(*entity.Array); ok && dashArray.Len() >= 2 {
			_ = e.setDashPattern(Operator{
				Opcode:   "d",
				Operands: []entity.Object{dashArray.Get(0), dashArray.Get(1)},
			})
		}
	}

	if strokeAlpha := gsDict.Get(entity.Name("CA")); strokeAlpha != nil {
		if value, err := getNumberOperand(strokeAlpha); err == nil {
			e.graphics.strokeAlpha = clamp(value, 0, 1)
		}
	}
	if fillAlpha := gsDict.Get(entity.Name("ca")); fillAlpha != nil {
		if value, err := getNumberOperand(fillAlpha); err == nil {
			e.graphics.fillAlpha = clamp(value, 0, 1)
		}
	}
	if transfer := gsDict.Get(entity.Name("TR2")); transfer != nil {
		e.applyTransferObject(transfer)
	} else if transfer := gsDict.Get(entity.Name("TR")); transfer != nil {
		e.applyTransferObject(transfer)
	}

	return nil
}

func (e *Evaluator) applyTransferObject(obj entity.Object) {
	switch v := obj.(type) {
	case entity.Name:
		if v.Value() == "Default" || v.Value() == "Identity" {
			e.graphics.transferActive = false
		}
		return
	case *entity.Null:
		return
	}

	if arr, ok := obj.(*entity.Array); ok {
		if arr.Len() != 4 {
			return
		}
		var funcs [4]entity.Function
		for i := 0; i < 4; i++ {
			fn, err := e.parseTransferFunction(arr.Get(i))
			if err != nil {
				return
			}
			funcs[i] = fn
		}
		e.setTransferFunctions(funcs[:])
		return
	}

	fn, err := e.parseTransferFunction(obj)
	if err != nil {
		return
	}
	e.setTransferFunctions([]entity.Function{fn})
}

func (e *Evaluator) parseTransferFunction(obj entity.Object) (entity.Function, error) {
	if ref, ok := obj.(entity.Ref); ok && e.xref != nil {
		resolved, err := e.xref.Fetch(ref)
		if err != nil {
			return nil, err
		}
		obj = resolved
	}
	fn, err := e.parseShadingFunctionObject(obj)
	if err != nil {
		return nil, err
	}
	if fn.GetInputSize() != 1 || fn.GetOutputSize() != 1 {
		return nil, fmt.Errorf("invalid transfer function dimensions")
	}
	return fn, nil
}

func (e *Evaluator) setTransferFunctions(funcs []entity.Function) {
	if len(funcs) == 0 || funcs[0] == nil {
		e.graphics.transferActive = false
		return
	}

	var red, green, blue, gray [256]uint8
	for i := 0; i < 256; i++ {
		x := float64(i) / 255.0
		if len(funcs) == 4 {
			red[i] = evaluateTransferByte(funcs[0], x)
			green[i] = evaluateTransferByte(funcs[1], x)
			blue[i] = evaluateTransferByte(funcs[2], x)
			gray[i] = evaluateTransferByte(funcs[3], x)
			continue
		}
		value := evaluateTransferByte(funcs[0], x)
		red[i] = value
		green[i] = value
		blue[i] = value
		gray[i] = value
	}

	e.graphics.transferRed = red
	e.graphics.transferGreen = green
	e.graphics.transferBlue = blue
	e.graphics.transferGray = gray
	e.graphics.transferActive = true
}

func evaluateTransferByte(fn entity.Function, x float64) uint8 {
	output, err := fn.Evaluate([]float64{x})
	if err != nil || len(output) == 0 {
		return uint8(x*255.0 + 0.5)
	}
	y := clamp(output[0], 0, 1)
	return uint8(y*255.0 + 0.5)
}

// Helper functions for color conversion

// grayToHex converts RGB values in [0,1] range to hex string.
func grayToHex(r, g, b float64) string {
	rr := colorspace.ConvertComponentToByte(r)
	gg := colorspace.ConvertComponentToByte(g)
	bb := colorspace.ConvertComponentToByte(b)
	return fmt.Sprintf("%02X%02X%02X", rr, gg, bb)
}

// clamp returns value clamped to [min, max] range.
func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// cmykToRGB converts CMYK values in [0,1] range to RGB in [0,1] range.
func cmykToRGB(c, m, y, k float64) (float64, float64, float64) {
	rgba := deviceCMYKColorSpace.ConvertToRGBA([]float64{
		clamp(c, 0, 1),
		clamp(m, 0, 1),
		clamp(y, 0, 1),
		clamp(k, 0, 1),
	})

	return float64(rgba.R) / 255.0,
		float64(rgba.G) / 255.0,
		float64(rgba.B) / 255.0
}

// Path Construction Operators

// moveTo begins a new subpath by moving to (x, y) - 'm' operator.
func (e *Evaluator) moveTo(op Operator) error {
	if len(op.Operands) < 2 {
		return fmt.Errorf("m operator requires 2 operands")
	}

	x, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("m operator: invalid x coordinate: %w", err)
	}

	y, err := getNumberOperand(op.Operands[1])
	if err != nil {
		return fmt.Errorf("m operator: invalid y coordinate: %w", err)
	}

	// Apply current transform matrix to get user space coordinates
	tx, ty := e.transformPoint(x, y)

	// Add move-to element to path
	e.graphics.path.AddElement(&MoveTo{X: tx, Y: ty})

	return nil
}

// lineTo appends a straight line segment from the current point to (x, y) - 'l' operator.
func (e *Evaluator) lineTo(op Operator) error {
	if len(op.Operands) < 2 {
		return fmt.Errorf("l operator requires 2 operands")
	}

	x, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("l operator: invalid x coordinate: %w", err)
	}

	y, err := getNumberOperand(op.Operands[1])
	if err != nil {
		return fmt.Errorf("l operator: invalid y coordinate: %w", err)
	}

	// Apply current transform matrix
	tx, ty := e.transformPoint(x, y)

	// Add line-to element to path
	e.graphics.path.AddElement(&LineTo{X: tx, Y: ty})

	return nil
}

// curveTo appends a cubic Bézier curve - 'c' operator.
// Operands: x1 y1 x2 y2 x y
func (e *Evaluator) curveTo(op Operator) error {
	if len(op.Operands) < 6 {
		return fmt.Errorf("c operator requires 6 operands")
	}

	x1, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("c operator: invalid x1 coordinate: %w", err)
	}

	y1, err := getNumberOperand(op.Operands[1])
	if err != nil {
		return fmt.Errorf("c operator: invalid y1 coordinate: %w", err)
	}

	x2, err := getNumberOperand(op.Operands[2])
	if err != nil {
		return fmt.Errorf("c operator: invalid x2 coordinate: %w", err)
	}

	y2, err := getNumberOperand(op.Operands[3])
	if err != nil {
		return fmt.Errorf("c operator: invalid y2 coordinate: %w", err)
	}

	x, err := getNumberOperand(op.Operands[4])
	if err != nil {
		return fmt.Errorf("c operator: invalid x coordinate: %w", err)
	}

	y, err := getNumberOperand(op.Operands[5])
	if err != nil {
		return fmt.Errorf("c operator: invalid y coordinate: %w", err)
	}

	// Apply current transform matrix
	tx1, ty1 := e.transformPoint(x1, y1)
	tx2, ty2 := e.transformPoint(x2, y2)
	tx, ty := e.transformPoint(x, y)

	// Add curve-to element to path
	e.graphics.path.AddElement(&CurveTo{
		X1: tx1, Y1: ty1,
		X2: tx2, Y2: ty2,
		X: tx, Y: ty,
	})

	return nil
}

// curveToNoFirstControl appends a cubic Bézier curve with first control point = current point - 'v' operator.
// Operands: x2 y2 x y
func (e *Evaluator) curveToNoFirstControl(op Operator) error {
	if len(op.Operands) < 4 {
		return fmt.Errorf("v operator requires 4 operands")
	}

	// First control point is the current point
	cx, cy := e.graphics.path.CurrentPoint()

	x2, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("v operator: invalid x2 coordinate: %w", err)
	}

	y2, err := getNumberOperand(op.Operands[1])
	if err != nil {
		return fmt.Errorf("v operator: invalid y2 coordinate: %w", err)
	}

	x, err := getNumberOperand(op.Operands[2])
	if err != nil {
		return fmt.Errorf("v operator: invalid x coordinate: %w", err)
	}

	y, err := getNumberOperand(op.Operands[3])
	if err != nil {
		return fmt.Errorf("v operator: invalid y coordinate: %w", err)
	}

	// Apply current transform matrix
	tx2, ty2 := e.transformPoint(x2, y2)
	tx, ty := e.transformPoint(x, y)

	// Add curve-to element to path
	e.graphics.path.AddElement(&CurveTo{
		X1: cx, Y1: cy,
		X2: tx2, Y2: ty2,
		X: tx, Y: ty,
	})

	return nil
}

// curveToNoLastControl appends a cubic Bézier curve with last control point = end point - 'y' operator.
// Operands: x1 y1 x y
func (e *Evaluator) curveToNoLastControl(op Operator) error {
	if len(op.Operands) < 4 {
		return fmt.Errorf("y operator requires 4 operands")
	}

	x1, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("y operator: invalid x1 coordinate: %w", err)
	}

	y1, err := getNumberOperand(op.Operands[1])
	if err != nil {
		return fmt.Errorf("y operator: invalid y1 coordinate: %w", err)
	}

	x, err := getNumberOperand(op.Operands[2])
	if err != nil {
		return fmt.Errorf("y operator: invalid x coordinate: %w", err)
	}

	y, err := getNumberOperand(op.Operands[3])
	if err != nil {
		return fmt.Errorf("y operator: invalid y coordinate: %w", err)
	}

	// Apply current transform matrix
	tx1, ty1 := e.transformPoint(x1, y1)
	tx, ty := e.transformPoint(x, y)

	// Add curve-to element to path (last control point equals end point)
	e.graphics.path.AddElement(&CurveTo{
		X1: tx1, Y1: ty1,
		X2: tx, Y2: ty,
		X: tx, Y: ty,
	})

	return nil
}

// closePath closes the current subpath - 'h' operator.
func (e *Evaluator) closePath(op Operator) error {
	e.graphics.path.AddElement(&Close{})
	return nil
}

// rectangle appends a rectangle to the path - 're' operator.
// Operands: x y width height
func (e *Evaluator) rectangle(op Operator) error {
	if len(op.Operands) < 4 {
		return fmt.Errorf("re operator requires 4 operands")
	}

	x, err := getNumberOperand(op.Operands[0])
	if err != nil {
		return fmt.Errorf("re operator: invalid x coordinate: %w", err)
	}

	y, err := getNumberOperand(op.Operands[1])
	if err != nil {
		return fmt.Errorf("re operator: invalid y coordinate: %w", err)
	}

	width, err := getNumberOperand(op.Operands[2])
	if err != nil {
		return fmt.Errorf("re operator: invalid width: %w", err)
	}

	height, err := getNumberOperand(op.Operands[3])
	if err != nil {
		return fmt.Errorf("re operator: invalid height: %w", err)
	}

	// Rectangle is constructed as: move to (x, y), line to (x+w, y),
	// line to (x+w, y+h), line to (x, y+h), close path
	// Apply transform to each corner and bulk-add to path.
	m := e.graphics.transform
	tx1 := m[0]*x + m[2]*y + m[4]
	ty1 := m[1]*x + m[3]*y + m[5]
	xw := x + width
	yh := y + height
	tx2 := m[0]*xw + m[2]*y + m[4]
	ty2 := m[1]*xw + m[3]*y + m[5]
	tx3 := m[0]*xw + m[2]*yh + m[4]
	ty3 := m[1]*xw + m[3]*yh + m[5]
	tx4 := m[0]*x + m[2]*yh + m[4]
	ty4 := m[1]*x + m[3]*yh + m[5]

	e.graphics.path.AddRect(tx1, ty1, tx2, ty2, tx3, ty3, tx4, ty4)

	return nil
}

// Path Painting Operators

// strokePath strokes the current path - 'S' operator.
func (e *Evaluator) strokePath() error {
	if e.graphics.path.IsEmpty() {
		return nil
	}

	// If canvas is set, render to it
	if e.canvas != nil && !shouldSkipStrokePathsForDebug() {
		e.renderPathToCanvas(false)
	}

	e.applyPendingClipAtPathEnd()

	// Clear the path after rendering
	e.graphics.path.Clear()

	return nil
}

// strokeAndClosePath closes and strokes the current path - 's' operator.
func (e *Evaluator) strokeAndClosePath() error {
	// Close the path first
	e.graphics.path.AddElement(&Close{})

	return e.strokePath()
}

// fillPath fills the current path using nonzero winding rule - 'f' operator.
func (e *Evaluator) fillPath() error {
	if e.graphics.path.IsEmpty() {
		return nil
	}

	// If canvas is set, render to it
	if e.canvas != nil && !shouldSkipFillPathsForDebug() {
		e.renderPathToCanvas(true)
	}

	e.applyPendingClipAtPathEnd()

	// Clear the path after rendering
	e.graphics.path.Clear()

	return nil
}

// fillPathEvenOdd fills the current path using even-odd rule - 'f*' operator.
func (e *Evaluator) fillPathEvenOdd() error {
	if e.graphics.path.IsEmpty() {
		return nil
	}

	// If canvas is set, render to it using even-odd rule
	if e.canvas != nil && !shouldSkipFillPathsForDebug() {
		e.renderPathToCanvasEvenOdd()
	}

	e.applyPendingClipAtPathEnd()

	// Clear the path after rendering
	e.graphics.path.Clear()

	return nil
}

// fillAndStrokePath fills and strokes the current path - 'B' operator.
func (e *Evaluator) fillAndStrokePath() error {
	if e.graphics.path.IsEmpty() {
		return nil
	}

	if e.canvas != nil {
		e.syncCanvasColors()
		e.replayPathToCanvas()
		if shouldSkipFillPathsForDebug() {
			if !shouldSkipStrokePathsForDebug() {
				e.canvas.Stroke()
			}
		} else if shouldSkipStrokePathsForDebug() {
			e.canvas.Fill()
		} else if fillStrokeCanvas, ok := e.canvas.(interface{ FillAndStroke() }); ok {
			fillStrokeCanvas.FillAndStroke()
		} else {
			e.canvas.Fill()
			e.replayPathToCanvas()
			e.canvas.Stroke()
		}
	}

	e.applyPendingClipAtPathEnd()

	e.graphics.path.Clear()
	return nil
}

// fillAndStrokePathEvenOdd fills and strokes using even-odd rule - 'B*' operator.
func (e *Evaluator) fillAndStrokePathEvenOdd() error {
	if e.graphics.path.IsEmpty() {
		return nil
	}

	if e.canvas != nil {
		e.syncCanvasColors()
		e.replayPathToCanvas()
		if shouldSkipFillPathsForDebug() {
			if !shouldSkipStrokePathsForDebug() {
				e.canvas.Stroke()
			}
		} else if shouldSkipStrokePathsForDebug() {
			if evenOddCanvas, ok := e.canvas.(interface{ FillEvenOdd() }); ok {
				evenOddCanvas.FillEvenOdd()
			} else {
				e.canvas.Fill()
			}
		} else if fillStrokeCanvas, ok := e.canvas.(interface{ FillEvenOddAndStroke() }); ok {
			fillStrokeCanvas.FillEvenOddAndStroke()
		} else {
			if evenOddCanvas, ok := e.canvas.(interface{ FillEvenOdd() }); ok {
				evenOddCanvas.FillEvenOdd()
			} else {
				e.canvas.Fill()
			}
			e.replayPathToCanvas()
			e.canvas.Stroke()
		}
	}

	e.applyPendingClipAtPathEnd()

	e.graphics.path.Clear()
	return nil
}

// closeFillAndStrokePath closes, fills, and strokes the current path - 'b' operator.
func (e *Evaluator) closeFillAndStrokePath() error {
	// Close the path first
	e.graphics.path.AddElement(&Close{})

	return e.fillAndStrokePath()
}

// closeFillAndStrokePathEvenOdd closes, fills, and strokes using even-odd rule - 'b*' operator.
func (e *Evaluator) closeFillAndStrokePathEvenOdd() error {
	// Close the path first
	e.graphics.path.AddElement(&Close{})

	return e.fillAndStrokePathEvenOdd()
}

// endPath ends the current path without filling or stroking - 'n' operator.
func (e *Evaluator) endPath() error {
	e.applyPendingClipAtPathEnd()
	e.graphics.path.Clear()
	return nil
}

// renderPathToCanvas renders the current path to the canvas.
func (e *Evaluator) renderPathToCanvas(fill bool) {
	if e.canvas == nil {
		return
	}
	e.syncCanvasColors()
	e.replayPathToCanvas()

	if fill {
		e.canvas.Fill()
	} else {
		e.canvas.Stroke()
	}
}

// renderPathToCanvasEvenOdd renders the current path to canvas using even-odd rule.
func (e *Evaluator) renderPathToCanvasEvenOdd() {
	if e.canvas == nil {
		return
	}
	e.syncCanvasColors()
	e.replayPathToCanvas()

	if evenOddCanvas, ok := e.canvas.(interface{ FillEvenOdd() }); ok {
		evenOddCanvas.FillEvenOdd()
		return
	}

	e.canvas.Fill()
}

func (e *Evaluator) replayPathToCanvas() {
	if e.canvas == nil {
		return
	}
	for _, elem := range e.graphics.path.Elements() {
		switch el := elem.(type) {
		case *MoveTo:
			e.canvas.MoveTo(el.X, el.Y)
		case *LineTo:
			e.canvas.LineTo(el.X, el.Y)
		case *CurveTo:
			e.canvas.CurveTo(el.X1, el.Y1, el.X2, el.Y2, el.X, el.Y)
		case *Close:
			e.canvas.ClosePath()
		}
	}
}

// syncCanvasGlyphTransform sets the canvas glyph transform to the linear part of
// the current text rendering matrix (TRM = CTM × textMatrix). This scales and
// rotates glyph path coordinates from font user space to device (pixel) space.
func (e *Evaluator) syncCanvasGlyphTransform() {
	if e.canvas == nil {
		return
	}
	type glyphTransformSetter interface {
		SetGlyphTransform(t [4]float64)
	}
	setter, ok := e.canvas.(glyphTransformSetter)
	if !ok {
		return
	}
	trm := e.textPlacement.CurrentRenderingMatrix(e)
	if os.Getenv("PDF_DEBUG_SPLASH_GLYPH_HSCALE_MATRIX") == "1" {
		hScale := e.graphics.currentState.GetHorizontalScaling() / 100.0
		if hScale == 0 {
			hScale = 1.0
		}
		trm[0] *= hScale
		trm[1] *= hScale
	}
	if os.Getenv("PDF_DEBUG_SPLASH_GLYPH_POPPLER_SIGNED_MATRIX") == "1" {
		trm[1] = -trm[1]
		trm[3] = -trm[3]
	}
	setter.SetGlyphTransform([4]float64{trm[0], trm[1], trm[2], trm[3]})
}

func (e *Evaluator) syncCanvasColors() {
	if e.canvas == nil {
		return
	}
	type colorTransferSetter interface {
		SetColorTransfer(red, green, blue, gray [256]uint8, active bool)
	}
	if setter, ok := e.canvas.(colorTransferSetter); ok {
		setter.SetColorTransfer(
			e.graphics.transferRed,
			e.graphics.transferGreen,
			e.graphics.transferBlue,
			e.graphics.transferGray,
			e.graphics.transferActive,
		)
	}
	if os.Getenv("PDF_DEBUG_PATTERN") == "1" {
		fc := colorFromGraphicsState(e.graphics.fillColor, e.graphics.fillAlpha)
		sc := colorFromGraphicsState(e.graphics.strokeColor, e.graphics.strokeAlpha)
		fr, fg, fb, fa := fc.RGBA()
		sr, sg, sb, sa := sc.RGBA()
		fmt.Fprintf(os.Stderr, "DEBUG syncCanvasColors: fill=RGBA(%d,%d,%d,%d) stroke=RGBA(%d,%d,%d,%d)\n",
			fr>>8, fg>>8, fb>>8, fa>>8, sr>>8, sg>>8, sb>>8, sa>>8)
	}
	e.canvas.SetFillColor(colorFromGraphicsState(e.graphics.fillColor, e.graphics.fillAlpha))
	e.canvas.SetStrokeColor(colorFromGraphicsState(e.graphics.strokeColor, e.graphics.strokeAlpha))
	e.canvas.SetFillPattern(e.patternForCanvas(e.graphics.fillPattern))
	e.canvas.SetStrokePattern(e.patternForCanvas(e.graphics.strokePattern))
	strokeScale := e.ctmStrokeScale()
	e.canvas.SetLineWidth(e.graphics.lineWidth * strokeScale)
	e.canvas.SetLineCap(e.graphics.currentState.GetLineCap())
	e.canvas.SetLineJoin(e.graphics.currentState.GetLineJoin())
	e.canvas.SetMiterLimit(e.graphics.currentState.GetMiterLimit())
	dashArray := e.graphics.currentState.GetDashArray()
	if len(dashArray) > 0 {
		scaled := make([]float64, len(dashArray))
		for i, v := range dashArray {
			scaled[i] = v * strokeScale
		}
		e.canvas.SetDashPattern(scaled, e.graphics.currentState.GetDashPhase()*strokeScale)
	} else {
		e.canvas.SetDashPattern(nil, 0)
	}
}

func (e *Evaluator) ctmStrokeScale() float64 {
	m := e.graphics.transform
	scaleX := math.Hypot(m[0], m[1])
	scaleY := math.Hypot(m[2], m[3])
	switch {
	case scaleX > 0 && scaleY > 0:
		return (scaleX + scaleY) / 2.0
	case scaleX > 0:
		return scaleX
	case scaleY > 0:
		return scaleY
	default:
		return 1.0
	}
}

func (e *Evaluator) patternForCanvas(pattern entity.Pattern) entity.Pattern {
	if pattern == nil {
		return nil
	}

	switch typed := pattern.(type) {
	case *entity.ShadingPattern:
		clone := entity.NewShadingPattern(typed.Name(), typed.GetShading())
		clone.SetMatrix(multiplyMatrix(e.graphics.baseTransform, typed.Matrix()))
		return clone
	case *entity.TilingPattern:
		// Poppler Gfx::doTilingPatternFill builds the tiling grid from
		// PTM * baseMatrix and cancels the current CTM before replaying the
		// pattern form. Keep tiling phase anchored to page/form base space.
		effectiveMatrix := multiplyMatrix(e.graphics.baseTransform, pattern.Matrix())
		clone := entity.NewTilingPattern(typed.Name(), typed.GetPaintType(), typed.GetTilingType())
		clone.SetMatrix(effectiveMatrix)
		clone.SetBBox(typed.GetBBox())
		clone.SetXStep(typed.GetXStep())
		clone.SetYStep(typed.GetYStep())
		clone.SetResources(typed.GetResources())
		clone.SetContent(typed.GetContent())
		return clone
	default:
		return pattern
	}
}

func colorFromGraphicsState(cs *ColorSpace, alpha float64) color.Color {
	if cs == nil {
		return color.Black
	}
	c, ok := cs.Color.(*Color)
	if !ok || c == nil {
		return color.Black
	}
	hexText := strings.TrimPrefix(strings.TrimSpace(c.Hex), "#")
	if len(hexText) != 6 {
		return color.Black
	}
	value, err := strconv.ParseUint(hexText, 16, 32)
	if err != nil {
		return color.Black
	}
	alpha = clamp(alpha, 0, 1)
	a := colorspace.ConvertComponentToByte(alpha)
	r := uint8(value >> 16)
	g := uint8((value >> 8) & 0xFF)
	b := uint8(value & 0xFF)
	if a == 255 {
		return color.RGBA{R: r, G: g, B: b, A: 255}
	}
	return color.NRGBA{R: r, G: g, B: b, A: a}
}

// Clipping Operators

// setClipPath sets the clipping path using nonzero winding rule - 'W' operator.
func (e *Evaluator) setClipPath() error {
	if e.graphics.path.IsEmpty() {
		return nil
	}

	e.graphics.pendingClip = true
	e.graphics.pendingClipMode = ClipNonZeroWinding
	return nil
}

// setClipPathEvenOdd sets the clipping path using even-odd rule - 'W*' operator.
func (e *Evaluator) setClipPathEvenOdd() error {
	if e.graphics.path.IsEmpty() {
		return nil
	}

	e.graphics.pendingClip = true
	e.graphics.pendingClipMode = ClipEvenOdd
	return nil
}

func (e *Evaluator) applyPendingClipAtPathEnd() {
	if !e.graphics.pendingClip || e.graphics.path.IsEmpty() {
		return
	}

	e.graphics.pathClip = e.graphics.path.Clone()
	e.graphics.clipMode = e.graphics.pendingClipMode

	if e.canvas != nil {
		if e.graphics.pendingClipMode == ClipEvenOdd {
			e.applyClippingPathEvenOdd()
		} else {
			e.applyClippingPath()
		}
	}

	e.graphics.pendingClip = false
}

// applyClippingPath applies the current clipping path to the canvas.
func (e *Evaluator) applyClippingPath() {
	if e.canvas == nil || e.graphics.pathClip == nil {
		return
	}

	// Try to use the canvas implementation's SetClipPathDirect method
	type clipCanvas interface {
		SetClipPathDirect(elements []interface{}, fillRule graphics.FillRule)
	}

	if clipImpl, ok := e.canvas.(clipCanvas); ok {
		// Convert []PathElement to []interface{}
		elements := make([]interface{}, len(e.graphics.pathClip.Elements()))
		for i, elem := range e.graphics.pathClip.Elements() {
			elements[i] = elem
		}
		clipImpl.SetClipPathDirect(elements, graphics.FillRuleNonZero)
		return
	}

	// Fallback: replay clipping path elements to canvas (may interfere with current path)
	for _, elem := range e.graphics.pathClip.Elements() {
		switch el := elem.(type) {
		case *MoveTo:
			e.canvas.MoveTo(el.X, el.Y)
		case *LineTo:
			e.canvas.LineTo(el.X, el.Y)
		case *CurveTo:
			e.canvas.CurveTo(el.X1, el.Y1, el.X2, el.Y2, el.X, el.Y)
		case *Close:
			e.canvas.ClosePath()
		}
	}

	// Apply clipping
	e.canvas.Clip()
}

// applyClippingPathEvenOdd applies the clipping path using even-odd rule.
func (e *Evaluator) applyClippingPathEvenOdd() {
	if e.canvas == nil || e.graphics.pathClip == nil {
		return
	}

	// Replay clipping path elements to canvas
	for _, elem := range e.graphics.pathClip.Elements() {
		switch el := elem.(type) {
		case *MoveTo:
			e.canvas.MoveTo(el.X, el.Y)
		case *LineTo:
			e.canvas.LineTo(el.X, el.Y)
		case *CurveTo:
			e.canvas.CurveTo(el.X1, el.Y1, el.X2, el.Y2, el.X, el.Y)
		case *Close:
			e.canvas.ClosePath()
		}
	}

	// Apply even-odd clipping
	e.canvas.EoClip()
}

// Helper function to get a numeric value from an operand
func getNumberOperand(obj entity.Object) (float64, error) {
	switch v := obj.(type) {
	case *entity.Integer:
		return float64(v.Value()), nil
	case *entity.Real:
		return v.Value(), nil
	default:
		return 0, fmt.Errorf("operand is not a number")
	}
}

// transformPoint applies the current transform matrix to a point.
func (e *Evaluator) transformPoint(x, y float64) (float64, float64) {
	m := e.graphics.transform
	return transformPointWithMatrix(m, x, y)
}

// GetOperators returns the parsed operators.
func (e *Evaluator) GetOperators() []Operator {
	return e.operators
}

// GetGraphicsState returns the current graphics state.
func (e *Evaluator) GetGraphicsState() *GraphicsState {
	return e.graphics
}

// ExtractedText returns text collected while evaluating content streams.
func (e *Evaluator) ExtractedText() string {
	return strings.TrimSpace(e.textBuffer.String())
}

// SetResources sets the resource dictionary for the evaluator.
func (e *Evaluator) SetResources(resources *entity.Dict) {
	e.resources = resources
}

// SetCanvas sets the canvas for rendering output.
func (e *Evaluator) SetCanvas(c canvas.Canvas) {
	e.canvas = c
}

// SetFormOperatorCache sets the shared Form XObject operator cache.
func (e *Evaluator) SetFormOperatorCache(cache FormOperatorCache) {
	e.sharedFormCache = cache
}

// SetInitialTransform sets the initial CTM used before parsing page operators.
func (e *Evaluator) SetInitialTransform(matrix [6]float64) {
	e.initialTransform = matrix
	e.graphics.transform = matrix
	e.graphics.baseTransform = matrix
}

// SetImageSamplingDebug toggles image sampling trace output.
func (e *Evaluator) SetImageSamplingDebug(enabled bool, documentID string, pageNumber int) {
	e.debugImageSampling = enabled
	e.debugDocumentID = documentID
	e.debugPageNumber = pageNumber
}

// SetImageSamplingMode configures automatic image sampling mode.
func (e *Evaluator) SetImageSamplingMode(mode string) {
	e.imageSamplingMode = normalizeImageSamplingMode(mode)
}

// SetFillColor sets the current fill color for evaluator-rendered content.
func (e *Evaluator) SetFillColor(c color.Color) {
	if c == nil {
		e.graphics.fillColor = &ColorSpace{Color: &Color{Hex: "000000"}}
		e.graphics.fillAlpha = 1.0
		e.graphics.fillCS = "DeviceRGB"
		return
	}

	r, g, b, a := c.RGBA()
	e.graphics.fillColor = &ColorSpace{Color: &Color{Hex: fmt.Sprintf("%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8))}}
	e.graphics.fillAlpha = float64(a) / 65535.0
	e.graphics.fillCS = "DeviceRGB"
}

// SetStrokeColor sets the current stroke color for evaluator-rendered content.
func (e *Evaluator) SetStrokeColor(c color.Color) {
	if c == nil {
		e.graphics.strokeColor = &ColorSpace{Color: &Color{Hex: "000000"}}
		e.graphics.strokeAlpha = 1.0
		e.graphics.strokeCS = "DeviceRGB"
		return
	}

	r, g, b, a := c.RGBA()
	e.graphics.strokeColor = &ColorSpace{Color: &Color{Hex: fmt.Sprintf("%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8))}}
	e.graphics.strokeAlpha = float64(a) / 65535.0
	e.graphics.strokeCS = "DeviceRGB"
}

// SetFillPattern sets the current fill pattern for evaluator-rendered content.
func (e *Evaluator) SetFillPattern(pattern entity.Pattern) {
	e.graphics.fillPattern = pattern
}

// SetStrokePattern sets the current stroke pattern for evaluator-rendered content.
func (e *Evaluator) SetStrokePattern(pattern entity.Pattern) {
	e.graphics.strokePattern = pattern
}

// EvaluateContent evaluates raw content stream bytes (for pattern cells).
func (e *Evaluator) EvaluateContent(data []byte) error {
	// Parse operators from raw bytes
	return e.parseOperators(data)
}

// ParseContentOperators parses PDF content stream bytes into a list of operators
// without executing them. Useful for pre-parsing pattern cells that are tiled many times.
func (e *Evaluator) ParseContentOperators(data []byte) ([]Operator, error) {
	return e.parseOperatorsOnly(data)
}

// ExecuteOperators executes a pre-parsed list of operators.
// Used to replay pattern tile content with per-tile transforms.
func (e *Evaluator) ExecuteOperators(ops []Operator) {
	e.executeCachedOperators(ops)
}

// Text operator methods - public wrappers for internal operators

// ShowText displays a text string (Tj operator).
func (e *Evaluator) ShowText(op Operator) error {
	return e.showText(op)
}

// ShowTextArray displays a text array with spacing adjustments (TJ operator).
func (e *Evaluator) ShowTextArray(op Operator) error {
	return e.showTextArray(op)
}

// MoveText moves the text position (Td operator).
func (e *Evaluator) MoveText(op Operator) error {
	return e.moveText(op)
}

// MoveTextSetLeading moves text position and sets leading (TD operator).
func (e *Evaluator) MoveTextSetLeading(op Operator) error {
	return e.moveTextSetLeading(op)
}

// SetTextMatrix sets the text matrix (Tm operator).
func (e *Evaluator) SetTextMatrix(op Operator) error {
	return e.setTextMatrix(op)
}

// SetFont sets the current font (Tf operator).
func (e *Evaluator) SetFont(op Operator) error {
	return e.setFont(op)
}

// SaveState saves the current graphics state (q operator).
func (e *Evaluator) SaveState() error {
	return e.saveState()
}

// RestoreState restores the last saved graphics state (Q operator).
func (e *Evaluator) RestoreState() error {
	return e.restoreState()
}

// Path operator methods - public wrappers for internal operators

// MoveTo begins a new subpath by moving to (x, y) - 'm' operator.
func (e *Evaluator) MoveTo(op Operator) error {
	return e.moveTo(op)
}

// LineTo appends a straight line segment from the current point to (x, y) - 'l' operator.
func (e *Evaluator) LineTo(op Operator) error {
	return e.lineTo(op)
}

// CurveTo appends a cubic Bézier curve - 'c' operator.
func (e *Evaluator) CurveTo(op Operator) error {
	return e.curveTo(op)
}

// Rectangle appends a rectangle to the path - 're' operator.
func (e *Evaluator) Rectangle(op Operator) error {
	return e.rectangle(op)
}

// ClosePath closes the current subpath - 'h' operator.
func (e *Evaluator) ClosePath(op Operator) error {
	return e.closePath(op)
}

// StrokePath strokes the current path - 'S' operator.
func (e *Evaluator) StrokePath() error {
	return e.strokePath()
}

// FillPath fills the current path using nonzero winding rule - 'f' operator.
func (e *Evaluator) FillPath() error {
	return e.fillPath()
}

// FillPathEvenOdd fills the current path using even-odd rule - 'f*' operator.
func (e *Evaluator) FillPathEvenOdd() error {
	return e.fillPathEvenOdd()
}

// EndPath ends the current path without filling or stroking - 'n' operator.
func (e *Evaluator) EndPath() error {
	return e.endPath()
}

// InvokeXObject invokes a named XObject - 'Do' operator.
func (e *Evaluator) InvokeXObject(op Operator) error {
	return e.invokeXObject(op)
}

// Color operator methods - public wrappers for internal operators

// SetGrayStroke sets the gray color for stroking - 'G' operator.
func (e *Evaluator) SetGrayStroke(op Operator) error {
	return e.setGrayStroke(op)
}

// SetGrayFill sets the gray color for filling - 'g' operator.
func (e *Evaluator) SetGrayFill(op Operator) error {
	return e.setGrayFill(op)
}

// SetRGBStroke sets the RGB color for stroking - 'RG' operator.
func (e *Evaluator) SetRGBStroke(op Operator) error {
	return e.setRGBStroke(op)
}

// SetRGBFill sets the RGB color for filling - 'rg' operator.
func (e *Evaluator) SetRGBFill(op Operator) error {
	return e.setRGBFill(op)
}

// SetCMYKStroke sets the CMYK color for stroking - 'K' operator.
func (e *Evaluator) SetCMYKStroke(op Operator) error {
	return e.setCMYKStroke(op)
}

// SetCMYKFill sets the CMYK color for filling - 'k' operator.
func (e *Evaluator) SetCMYKFill(op Operator) error {
	return e.setCMYKFill(op)
}

// SetLineWidth sets the line width - 'w' operator.
func (e *Evaluator) SetLineWidth(op Operator) error {
	return e.setLineWidth(op)
}

// SetLineCap sets the line cap style - 'J' operator.
func (e *Evaluator) SetLineCap(op Operator) error {
	return e.setLineCap(op)
}

// SetLineJoin sets the line join style - 'j' operator.
func (e *Evaluator) SetLineJoin(op Operator) error {
	return e.setLineJoin(op)
}

// SetMiterLimit sets the miter limit - 'M' operator.
func (e *Evaluator) SetMiterLimit(op Operator) error {
	return e.setMiterLimit(op)
}

// Inline Image Operators

// beginInlineImage begins an inline image - 'BI' operator.
func (e *Evaluator) beginInlineImage() error {
	// Mark that we're parsing an inline image
	e.inInlineImage = true
	e.inlineImageDict = entity.NewDict()
	e.inlineImageData = nil
	return nil
}

// endInlineImageData ends the inline image data - 'EI' operator.
func (e *Evaluator) endInlineImage() error {
	if !e.inInlineImage {
		return fmt.Errorf("EI operator without corresponding BI")
	}
	// Process the inline image
	if e.inlineImageDict != nil && len(e.inlineImageData) > 0 {
		if err := e.renderInlineImage(); err != nil {
			return fmt.Errorf("failed to render inline image: %w", err)
		}
	}

	// Reset inline image state
	e.inInlineImage = false
	e.inlineImageDict = nil
	e.inlineImageData = nil

	return nil
}

// renderInlineImage renders an inline image to the canvas.
func (e *Evaluator) renderInlineImage() error {
	dict := e.inlineImageDict

	// Get image dimensions
	widthVal := dict.GetTry(entity.Name("W"), entity.Name("Width"))
	if widthVal == nil {
		return fmt.Errorf("inline image has no Width")
	}
	width, err := getNumberOperand(widthVal)
	if err != nil {
		return fmt.Errorf("inline image: invalid Width: %w", err)
	}

	heightVal := dict.GetTry(entity.Name("H"), entity.Name("Height"))
	if heightVal == nil {
		return fmt.Errorf("inline image has no Height")
	}
	height, err := getNumberOperand(heightVal)
	if err != nil {
		return fmt.Errorf("inline image: invalid Height: %w", err)
	}

	bpc := getImageBitsPerComponent(dict.GetTry(entity.Name("BPC"), entity.Name("BitsPerComponent")))
	filterObj := dict.GetTry(entity.Name("Filter"), entity.Name("F"))
	imageFilter, useEncodedData := resolveXObjectImageFilter(filterObj)
	data := e.inlineImageData
	if !useEncodedData {
		infraStream := stream.NewFromEntity(entity.NewStream(normalizeInlineImageStreamDict(dict), data))
		decoded, err := infraStream.Decode()
		if err != nil {
			return errors.Invalid("decode_inline_image", err)
		}
		data = decoded
		imageFilter = domainimage.FilterNone
	}

	imageMask := isImageMaskDictValue(dict.Get(entity.Name("ImageMask")))
	if !imageMask {
		imageMask = isImageMaskDictValue(dict.Get(entity.Name("IM")))
	}
	if shouldSkipAllImagesForDebug() {
		return nil
	}
	if imageMask {
		interpolate, interpolateExplicit := resolveImageInterpolateOption(dict.Get(entity.Name("I")), false)
		if !interpolateExplicit {
			interpolate, interpolateExplicit = resolveImageInterpolateOption(dict.Get(entity.Name("Interpolate")), false)
		}
		decode := e.resolveImageDecodeArray(dict.Get(entity.Name("Decode")))
		paintBitOne := resolveImageMaskPaintBit(decode)
		if e.canvas != nil {
			if err := e.renderImageMaskToCanvas(
				data,
				width,
				height,
				bpc,
				resolveXObjectImageSourceFilter(filterObj),
				paintBitOne,
				interpolate,
				interpolateExplicit,
			); err != nil {
				e.renderPlaceholderImage(width, height)
			}
		}
		return nil
	}

	colorSpaceVal := dict.GetTry(entity.Name("CS"), entity.Name("ColorSpace"))
	colorSpace, ok := e.resolveImageColorSpace(colorSpaceVal)
	if !ok {
		return nil
	}

	indexedBase := ""
	indexedLookup := []byte{}
	if colorSpace == "Indexed" {
		base, lookup, indexedOK := e.resolveIndexedColorSpace(colorSpaceVal, 0)
		if !indexedOK {
			return nil
		}
		indexedBase = base
		indexedLookup = lookup
	}

	// If canvas is set, render the image
	if e.canvas != nil {
		interpolate, interpolateExplicit := resolveImageInterpolateOption(dict.Get(entity.Name("I")), false)
		if !interpolateExplicit {
			interpolate, interpolateExplicit = resolveImageInterpolateOption(dict.Get(entity.Name("Interpolate")), false)
		}
		sourceICCBased := e.isICCBasedColorSpace(colorSpaceVal)
		var iccProfile []byte
		iccComponents := 0
		if sourceICCBased {
			iccProfile, _ = e.resolveICCBasedProfile(colorSpaceVal, 0)
			iccComponents = e.resolveICCBasedComponentCount(colorSpaceVal)
		}
		decode := e.resolveImageDecodeArray(dict.GetTry(entity.Name("Decode"), entity.Name("D")))
		mask := e.resolveSoftMask(dict.Get(entity.Name("SMask")))
		if mask == nil {
			// Explicit image mask can be provided in /Mask as an image stream.
			mask = e.resolveSoftMask(dict.Get(entity.Name("Mask")))
		}
		colorKeyMask := e.resolveColorKeyMask(dict.Get(entity.Name("Mask")), colorSpace)
		if mask != nil {
			// When soft mask is present, favor SMask alpha and ignore color-key masking.
			colorKeyMask = nil
		}
		e.renderImageToCanvas(
			data,
			width,
			height,
			colorSpace,
			sourceICCBased,
			iccProfile,
			iccComponents,
			indexedBase,
			indexedLookup,
			bpc,
			imageFilter,
			resolveXObjectImageSourceFilter(filterObj),
			e.resolveImageDecodeParms(dict.GetTry(entity.Name("DecodeParms"), entity.Name("DP")), 0),
			decode,
			mask,
			colorKeyMask,
			interpolate,
			interpolateExplicit,
		)
	}

	return nil
}

func normalizeInlineImageStreamDict(dict *entity.Dict) *entity.Dict {
	if dict == nil {
		return entity.NewDict()
	}

	normalized := entity.NewDict()
	for _, key := range dict.Keys() {
		normalized.Set(key, dict.GetRaw(key))
	}

	if filter := dict.GetTry(entity.Name("Filter"), entity.Name("F")); filter != nil {
		normalized.Set(entity.Name("Filter"), normalizeInlineImageFilterObject(filter))
	}
	if decodeParms := dict.GetTry(entity.Name("DecodeParms"), entity.Name("DP")); decodeParms != nil {
		normalized.Set(entity.Name("DecodeParms"), decodeParms)
	}

	return normalized
}

func normalizeInlineImageFilterObject(filter entity.Object) entity.Object {
	switch v := filter.(type) {
	case entity.Name:
		return entity.Name(normalizeImageFilterName(string(v)))
	case *entity.Array:
		items := make([]entity.Object, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			item := v.Get(i)
			if name, ok := item.(entity.Name); ok {
				items = append(items, entity.Name(normalizeImageFilterName(string(name))))
				continue
			}
			items = append(items, item)
		}
		return entity.NewArray(items...)
	default:
		return filter
	}
}
