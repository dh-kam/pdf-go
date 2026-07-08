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

var graphicsStatePool = sync.Pool{
	New: func() interface{} { return new(graphics.State) },
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
	type3PendingTemp     *type3PendingTemp
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
	resourceStack        []*entity.Dict
	inlineImageDict      *entity.Dict
	textBuffer           strings.Builder
	operators            []Operator
	recordOperators      bool
	stateStack           []*GraphicsState
	inlineImageData      []byte
	formOperatorCache    map[*entity.Stream][]Operator
	formStreamUseCount   map[*entity.Stream]int
	charProcCache        map[*entity.Stream][]Operator
	decodedStreamCache   map[*entity.Stream][]byte
	softMaskDetailsCache map[*entity.Stream]softMaskDetails
	rawRGBMatteCache     map[packedRawRGBMatteImageCacheKey]stdimage.Image
	imageDecoder         *image.Decoder
	fontCache            map[*entity.Dict]entity.Font
	type1FontCache       map[embeddedFontCacheKey]entity.Font
	trueTypeFontCache    map[embeddedFontCacheKey]entity.Font
	sharedFormCache      FormOperatorCache
	imageSamplingMode    string
	debugDocumentID      string
	debugPageNumber      int
	debugImageSampling   bool
	debugPath            []string
	textCodeUnitScratch  []textCodeUnit
	inInlineImage        bool
	textCurrentValid     bool
	textUserCurrentValid bool
}

type textCodeUnit struct {
	code uint32
	raw  string
}

type embeddedFontCacheKey struct {
	sum  [32]byte
	size int
}

var deviceCMYKColorSpace = colorspace.NewDeviceCMYK()

// Keep repeated medium streams cached during one page render, but avoid retaining page-sized decoded images.
const maxCachedDecodedStreamBytes = 2 << 20

// NewEvaluator creates a new content stream evaluator.
func NewEvaluator(xref entity.XRef) *Evaluator {
	identity := [6]float64{1, 0, 0, 1, 0, 0}
	return &Evaluator{
		xref:              xref,
		fontResolver:      defaultFontCandidateResolver{},
		fontFallback:      defaultFontFallbackResolver{},
		textPolicy:        newDefaultTextRenderPolicy(),
		textPlacement:     defaultTextPlacement{},
		textRenderer:      defaultTextRenderer{},
		recordOperators:   true,
		graphics:          NewGraphicsState(),
		initialTransform:  identity,
		textMatrix:        identity,
		textLineMatrix:    identity,
		textBaseMatrix:    identity,
		imageSamplingMode: ImageSamplingModeLegacy,
	}
}

func (e *Evaluator) ensureFormOperatorCache() map[*entity.Stream][]Operator {
	if e.formOperatorCache == nil {
		e.formOperatorCache = make(map[*entity.Stream][]Operator)
	}
	return e.formOperatorCache
}

func (e *Evaluator) ensureCharProcCache() map[*entity.Stream][]Operator {
	if e.charProcCache == nil {
		e.charProcCache = make(map[*entity.Stream][]Operator)
	}
	return e.charProcCache
}

func (e *Evaluator) ensureDecodedStreamCache() map[*entity.Stream][]byte {
	if e.decodedStreamCache == nil {
		e.decodedStreamCache = make(map[*entity.Stream][]byte)
	}
	return e.decodedStreamCache
}

func (e *Evaluator) ensureSoftMaskDetailsCache() map[*entity.Stream]softMaskDetails {
	if e.softMaskDetailsCache == nil {
		e.softMaskDetailsCache = make(map[*entity.Stream]softMaskDetails)
	}
	return e.softMaskDetailsCache
}

func (e *Evaluator) ensureRawRGBMatteCache() map[packedRawRGBMatteImageCacheKey]stdimage.Image {
	if e.rawRGBMatteCache == nil {
		e.rawRGBMatteCache = make(map[packedRawRGBMatteImageCacheKey]stdimage.Image)
	}
	return e.rawRGBMatteCache
}

func (e *Evaluator) ensureFontCache() map[*entity.Dict]entity.Font {
	if e.fontCache == nil {
		e.fontCache = make(map[*entity.Dict]entity.Font)
	}
	return e.fontCache
}

func (e *Evaluator) ensureType1FontCache() map[embeddedFontCacheKey]entity.Font {
	if e.type1FontCache == nil {
		e.type1FontCache = make(map[embeddedFontCacheKey]entity.Font)
	}
	return e.type1FontCache
}

func (e *Evaluator) ensureTrueTypeFontCache() map[embeddedFontCacheKey]entity.Font {
	if e.trueTypeFontCache == nil {
		e.trueTypeFontCache = make(map[embeddedFontCacheKey]entity.Font)
	}
	return e.trueTypeFontCache
}

func (e *Evaluator) decodeEntityStream(entityStream *entity.Stream) ([]byte, error) {
	return e.decodeEntityStreamWithSizeHint(entityStream, 0)
}

func (e *Evaluator) decodeImageData(data *domainimage.ImageData) (domainimage.Image, error) {
	if e.imageDecoder == nil {
		e.imageDecoder = image.NewDecoder()
	}
	return e.imageDecoder.Decode(data)
}

func (e *Evaluator) decodeEntityStreamWithSizeHint(entityStream *entity.Stream, sizeHint int) ([]byte, error) {
	if entityStream == nil {
		return nil, errors.Invalid("decode_stream", fmt.Errorf("nil stream"))
	}
	if e.decodedStreamCache != nil {
		if data, ok := e.decodedStreamCache[entityStream]; ok {
			return data, nil
		}
	}
	infra := stream.NewFromEntity(entityStream)
	if sizeHint > 0 {
		infra.SetDecodeSizeHint(sizeHint)
	}
	decoded, err := infra.Decode()
	if err != nil {
		return nil, err
	}
	if len(decoded) <= maxCachedDecodedStreamBytes {
		e.ensureDecodedStreamCache()[entityStream] = decoded
	}
	return decoded, nil
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
	Opcode      string
	Operands    []entity.Object
	DebugIndex  int
	InlineImage *InlineImageOperator
}

// InlineImageOperator stores the uncommon BI image payload out-of-line so the
// common cached Operator stays compact on large Form streams.
type InlineImageOperator struct {
	Dict *entity.Dict
	Data []byte
}

type debugPaintContextCanvas interface {
	SetDebugPaintContext(string) func()
}

type popplerOrderStrokeCanvas interface {
	StrokePathWithCTM(elements []PathElement, ctm [6]float64, lineWidth float64, dash []float64, phase float64)
}

type popplerOrderStrokePathCanvas interface {
	StrokePathObjectWithCTM(path *Path, ctm [6]float64, lineWidth float64, dash []float64, phase float64)
}

type popplerOrderFillCanvas interface {
	FillPathWithCTM(elements []PathElement, ctm [6]float64, evenOdd bool)
}

type popplerOrderFillPathCanvas interface {
	FillPathObjectWithCTM(path *Path, ctm [6]float64, evenOdd bool)
}

func isContentOperatorKeyword(keyword string) bool {
	switch len(keyword) {
	case 1:
		switch keyword[0] {
		case '"', '\'', 'B', 'F', 'G', 'J', 'K', 'M', 'Q', 'S', 'W', 'b', 'c', 'd', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'q', 's', 'v', 'w', 'y':
			return true
		}
	case 2:
		switch keyword {
		case "B*", "BI", "BT", "BX", "CS", "DP", "Do", "EI", "ET", "EX", "ID", "MP", "RG", "SC", "TD", "TJ", "TL", "T*", "Tc", "Td", "Tf", "Tj", "Tm", "Tr", "Ts", "Tw", "Tz", "W*", "b*", "cm", "cs", "d0", "d1", "f*", "gs", "re", "rg", "ri", "sc", "sh":
			return true
		}
	case 3:
		switch keyword {
		case "BDC", "BMC", "EMC", "SCN", "scn":
			return true
		}
	}
	return false
}

// Evaluate evaluates the content stream for a page.
func (e *Evaluator) Evaluate(contents []entity.Object) error {
	for i, content := range contents {
		entityStream, ok := content.(*entity.Stream)
		if !ok {
			continue
		}
		popDebugPath := func() {}
		if debugRenderContextEnabled() {
			popDebugPath = e.pushDebugPath(fmt.Sprintf("page.contents[%d]", i))
		}

		// Convert entity.Stream to decoded bytes, reusing repeated stream decodes within the page.
		data, err := e.decodeEntityStream(entityStream)
		if err != nil {
			// Some malformed/encrypted PDFs contain stream bytes that fail declared filter decoding.
			// Fall back to raw stream bytes as best-effort, and skip this stream on failure.
			raw := entityStream.RawBytes()
			if len(raw) > 0 {
				_ = e.parseOperators(raw)
			}
			popDebugPath()
			continue
		}

		// Parse operators from stream data
		if err := e.parseOperators(data); err != nil {
			popDebugPath()
			return err
		}
		popDebugPath()
	}

	return nil
}

// parseOperators parses PDF operators from binary stream data.
func (e *Evaluator) parseOperators(data []byte) error {
	if e.recordOperators && e.operators == nil {
		e.operators = make([]Operator, 0, operatorCapacityHint(data))
	}
	return e.parseOperatorsWithHandlerMode(data, func(op Operator) {
		if e.recordOperators {
			e.operators = append(e.operators, op)
		}
		if err := e.executeOperator(op); err != nil {
			// Keep rendering even if a single operator is malformed.
			return
		}
	}, e.recordOperators)
}

func (e *Evaluator) parseOperatorsOnly(data []byte) ([]Operator, error) {
	ops := make([]Operator, 0, operatorCapacityHint(data))
	err := e.parseOperatorsWithHandler(data, func(op Operator) {
		ops = append(ops, op)
	})
	if err != nil {
		return nil, err
	}
	return ops, nil
}

func operatorCapacityHint(data []byte) int {
	const (
		minOperatorCapacity = 64
		maxOperatorCapacity = 262144
		bytesPerOperator    = 8
	)
	if estimated := estimateContentOperatorCount(data); estimated > 0 {
		hint := estimated + operatorCapacitySlack(estimated)
		if hint < minOperatorCapacity {
			return minOperatorCapacity
		}
		if hint > maxOperatorCapacity {
			return maxOperatorCapacity
		}
		return hint
	}
	dataLen := len(data)
	if dataLen <= 0 {
		return minOperatorCapacity
	}
	hint := dataLen / bytesPerOperator
	if hint < minOperatorCapacity {
		return minOperatorCapacity
	}
	if hint > maxOperatorCapacity {
		return maxOperatorCapacity
	}
	return hint
}

func operatorCapacitySlack(estimated int) int {
	switch {
	case estimated >= 4096:
		return estimated/128 + 128
	case estimated >= 1024:
		return estimated/16 + 64
	default:
		return estimated/4 + 64
	}
}

func estimateContentOperatorCount(data []byte) int {
	count := 0
	for i := 0; i < len(data); {
		i = skipContentWhitespace(data, i)
		if i >= len(data) {
			break
		}

		switch data[i] {
		case '%':
			i = skipContentComment(data, i+1)
			continue
		case '(':
			i = skipContentLiteralString(data, i+1)
			continue
		case '<':
			if i+1 < len(data) && data[i+1] == '<' {
				i += 2
				continue
			}
			i = skipContentHexString(data, i+1)
			continue
		case '/':
			i = skipContentRegularToken(data, i+1)
			continue
		}

		if isContentDelimiter(data[i]) {
			i++
			continue
		}

		start := i
		i = skipContentRegularToken(data, i)
		token := data[start:i]
		switch contentOperatorTokenKind(token) {
		case contentOperatorTokenNormal:
			count++
		case contentOperatorTokenInlineImage:
			count++
			i = skipInlineImageDataForOperatorHint(data, i)
		}
	}
	return count
}

const (
	contentOperatorTokenNone = iota
	contentOperatorTokenNormal
	contentOperatorTokenInlineImage
)

func contentOperatorTokenKind(token []byte) int {
	switch len(token) {
	case 1:
		switch token[0] {
		case '"', '\'', 'B', 'F', 'G', 'J', 'K', 'M', 'Q', 'S', 'W', 'b', 'c', 'd', 'f', 'g', 'h', 'i', 'j', 'k', 'l', 'm', 'n', 'q', 's', 'v', 'w', 'y':
			return contentOperatorTokenNormal
		}
	case 2:
		switch string(token) {
		case "BI":
			return contentOperatorTokenInlineImage
		case "BT", "BX", "CS", "DP", "Do", "EI", "ET", "EX", "ID", "MP", "RG", "SC", "TD", "TJ", "TL", "Tc", "Td", "Tf", "Tj", "Tm", "Tr", "Ts", "Tw", "Tz", "W*", "b*", "cm", "cs", "d0", "d1", "f*", "re", "rg", "ri", "sc", "sh":
			return contentOperatorTokenNormal
		}
	case 3:
		switch string(token) {
		case "BDC", "BMC", "EMC", "SCN", "scn":
			return contentOperatorTokenNormal
		}
	}
	return contentOperatorTokenNone
}

func skipInlineImageDataForOperatorHint(data []byte, start int) int {
	idEnd := findContentOperatorTokenEnd(data, start, []byte("ID"))
	if idEnd < 0 {
		return len(data)
	}
	imageStart := skipInlineImageLeadingWhitespace(data, idEnd)
	end, err := findInlineImageEndOffset(data, imageStart)
	if err != nil {
		return len(data)
	}
	return end + 2
}

func findContentOperatorTokenEnd(data []byte, start int, want []byte) int {
	for i := start; i < len(data); {
		i = skipContentWhitespace(data, i)
		if i >= len(data) {
			return -1
		}
		switch data[i] {
		case '%':
			i = skipContentComment(data, i+1)
			continue
		case '(':
			i = skipContentLiteralString(data, i+1)
			continue
		case '<':
			if i+1 < len(data) && data[i+1] == '<' {
				i += 2
				continue
			}
			i = skipContentHexString(data, i+1)
			continue
		case '/':
			i = skipContentRegularToken(data, i+1)
			continue
		}
		if isContentDelimiter(data[i]) {
			i++
			continue
		}
		tokenStart := i
		i = skipContentRegularToken(data, i)
		if contentTokenEqual(data[tokenStart:i], want) {
			return i
		}
	}
	return -1
}

func contentTokenEqual(got []byte, want []byte) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func skipContentWhitespace(data []byte, i int) int {
	for i < len(data) && isContentWhitespace(data[i]) {
		i++
	}
	return i
}

func skipContentComment(data []byte, i int) int {
	for i < len(data) && data[i] != '\n' && data[i] != '\r' {
		i++
	}
	return i
}

func skipContentLiteralString(data []byte, i int) int {
	depth := 1
	for i < len(data) && depth > 0 {
		switch data[i] {
		case '\\':
			i += 2
			continue
		case '(':
			depth++
		case ')':
			depth--
		}
		i++
	}
	return i
}

func skipContentHexString(data []byte, i int) int {
	for i < len(data) && data[i] != '>' {
		i++
	}
	if i < len(data) {
		i++
	}
	return i
}

func skipContentRegularToken(data []byte, i int) int {
	for i < len(data) && !isContentWhitespace(data[i]) && !isContentDelimiter(data[i]) {
		i++
	}
	return i
}

func isContentWhitespace(b byte) bool {
	switch b {
	case 0x00, 0x09, 0x0A, 0x0C, 0x0D, 0x20:
		return true
	default:
		return false
	}
}

func isContentDelimiter(b byte) bool {
	switch b {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	default:
		return false
	}
}

func (e *Evaluator) parseOperatorsWithHandler(data []byte, handler func(op Operator)) error {
	return e.parseOperatorsWithHandlerMode(data, handler, true)
}

func (e *Evaluator) parseOperatorsForImmediateExecution(data []byte, handler func(op Operator)) error {
	return e.parseOperatorsWithHandlerMode(data, handler, false)
}

func (e *Evaluator) parseOperatorsWithHandlerMode(data []byte, handler func(op Operator), copyOperands bool) error {
	// Create lexer and parser for the content stream
	lexer := parser.NewLexerBytesNoNumberValue(data)
	p := parser.NewParser(lexer, e.xref)
	operands := make([]entity.Object, 0, 8)
	opIndex := 0

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
				op, next, ok, err := e.parseInlineImageOperatorFromLexer(lexer, p, data)
				if err != nil {
					return err
				}
				if ok {
					op.DebugIndex = opIndex
					opIndex++
					handler(op)
				}
				if next >= len(data) {
					return nil
				}
				// Continue the same handler after EI so cached/preparsed streams
				// keep trailing operators such as the Q that balances a BI-time q.
				data = data[next:]
				lexer = parser.NewLexerBytesNoNumberValue(data)
				p = parser.NewParser(lexer, e.xref)
				continue
			}

			_, err := lexer.NextToken()
			if err != nil {
				return err
			}
			opOperands := operands
			if copyOperands {
				opOperands = append([]entity.Object(nil), operands...)
			}
			// Create operator
			op := Operator{
				Opcode:     token.Value,
				Operands:   opOperands,
				DebugIndex: opIndex,
			}
			opIndex++
			handler(op)
			operands = operands[:0]
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
	op, next, ok, err := e.parseInlineImageOperatorFromLexer(lexer, p, data)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if err := e.executeInlineImageOperator(op); err != nil {
		return e.skipInlineImageAndContinue(data, next)
	}
	return e.parseOperators(data[next:])
}

func (e *Evaluator) parseInlineImageOperatorFromLexer(lexer *parser.Lexer, p *parser.Parser, data []byte) (Operator, int, bool, error) {
	op := Operator{Opcode: "BI"}
	dict := entity.NewDict()

	searchFrom := lexer.Pos()

	// Parse inline image dictionary: /Key value pairs until ID token.
	for {
		tok, err := lexer.Peek()
		if err != nil {
			next := e.findInlineImageRecoveryOffset(data, searchFrom)
			return op, next, false, nil
		}

		if tok.Type == parser.TokenEOF {
			next := e.findInlineImageRecoveryOffset(data, searchFrom)
			return op, next, false, nil
		}

		if tok.Type == parser.TokenKeyword && tok.Value == "ID" {
			if _, err := lexer.NextToken(); err != nil {
				next := e.findInlineImageRecoveryOffset(data, searchFrom)
				return op, next, false, nil
			}
			searchFrom = lexer.Pos()
			break
		}

		keyToken, err := lexer.NextToken()
		if err != nil {
			next := e.findInlineImageRecoveryOffset(data, searchFrom)
			return op, next, false, nil
		}
		if keyToken.Type != parser.TokenKeyword {
			next := e.findInlineImageRecoveryOffset(data, searchFrom)
			return op, next, false, nil
		}

		key := "/" + keyToken.Value
		value, err := p.ParseObject()
		if err != nil {
			next := e.findInlineImageRecoveryOffset(data, searchFrom)
			return op, next, false, nil
		}
		dict.Set(entity.Name(key), value)
	}

	// Parse image data bytes until EI token boundary.
	start := searchFrom
	if start < 0 || start >= len(data) {
		next := e.findInlineImageRecoveryOffset(data, searchFrom)
		return op, next, false, nil
	}

	start = skipInlineImageLeadingWhitespace(data, start)
	if start >= len(data) {
		next := e.findInlineImageRecoveryOffset(data, searchFrom)
		return op, next, false, nil
	}

	end, err := findInlineImageEndOffset(data, start)
	if err != nil {
		next := e.findInlineImageRecoveryOffset(data, searchFrom)
		return op, next, false, nil
	}

	imageData := make([]byte, end-start)
	copy(imageData, data[start:end])

	op.InlineImage = &InlineImageOperator{
		Dict: dict,
		Data: imageData,
	}
	return op, end + 2, true, nil
}

func (e *Evaluator) findInlineImageRecoveryOffset(data []byte, searchFrom int) int {
	if searchFrom < 0 {
		searchFrom = 0
	}
	if searchFrom >= len(data) {
		return len(data)
	}
	end, err := findInlineImageEndOffset(data, searchFrom)
	if err != nil {
		return len(data)
	}
	return end + 2
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
	for _, op := range ops {
		if err := e.executeOperator(op); err != nil {
			continue
		}
	}
}

func (e *Evaluator) pushDebugPath(segment string) func() {
	if !debugRenderContextEnabled() || segment == "" {
		return func() {}
	}
	e.debugPath = append(e.debugPath, segment)
	return func() {
		if len(e.debugPath) > 0 {
			e.debugPath = e.debugPath[:len(e.debugPath)-1]
		}
	}
}

func (e *Evaluator) installDebugPaintContext(op Operator) func() {
	if !debugRenderContextEnabled() {
		return func() {}
	}
	canvas, ok := e.canvas.(debugPaintContextCanvas)
	if !ok {
		return func() {}
	}
	return canvas.SetDebugPaintContext(e.formatDebugPaintContext(op))
}

func (e *Evaluator) formatDebugPaintContext(op Operator) string {
	path := "page"
	if len(e.debugPath) > 0 {
		path = strings.Join(e.debugPath, "/")
	}
	resource := e.debugOperatorResource(op)
	ctm := [6]float64{}
	if e != nil && e.graphics != nil {
		ctm = e.graphics.transform
	}
	textMode := 0
	if e != nil && e.graphics != nil && e.graphics.currentState != nil {
		textMode = e.graphics.currentState.GetTextRenderMode()
	}
	return fmt.Sprintf("path=%s opIndex=%d opcode=%s resource=%s textMode=%d ctm=[%.6f %.6f %.6f %.6f %.6f %.6f]",
		path, op.DebugIndex, op.Opcode, resource,
		textMode,
		ctm[0], ctm[1], ctm[2], ctm[3], ctm[4], ctm[5],
	)
}

func (e *Evaluator) debugOperatorResource(op Operator) string {
	if len(op.Operands) == 0 {
		return "-"
	}
	nameValue := func(obj entity.Object) string {
		name, ok := obj.(entity.Name)
		if !ok {
			return ""
		}
		return strings.TrimPrefix(name.Value(), "/")
	}
	switch op.Opcode {
	case "Do":
		if name := nameValue(op.Operands[0]); name != "" {
			return e.describeDebugResource(pdfNameXObject, entity.Name(name), "XObject:"+name)
		}
	case "gs":
		if name := nameValue(op.Operands[0]); name != "" {
			return e.describeDebugResource(pdfNameExtGState, entity.Name(name), "ExtGState:"+name)
		}
	case "sh":
		if name := nameValue(op.Operands[0]); name != "" {
			return e.describeDebugResource(pdfNameShading, entity.Name(name), "Shading:"+name)
		}
	case "Tf":
		if name := nameValue(op.Operands[0]); name != "" {
			return e.describeDebugResource(pdfNameFont, entity.Name(name), "Font:"+name)
		}
	case "scn", "SCN":
		if name := nameValue(op.Operands[len(op.Operands)-1]); name != "" {
			return e.describeDebugResource(pdfNamePattern, entity.Name(name), "PatternOrColorant:"+name)
		}
	}
	return "-"
}

func (e *Evaluator) describeDebugResource(category entity.Name, name entity.Name, fallback string) string {
	if e == nil {
		return fallback
	}
	raw, resolved, frameIndex := e.lookupRawDebugResource(category, name)
	if raw == nil && resolved == nil {
		return fallback
	}
	return fmt.Sprintf("%s frame=%d raw=%s resolved=%s",
		fallback,
		frameIndex,
		debugObjectSummary(raw),
		debugObjectSummary(resolved),
	)
}

func (e *Evaluator) lookupRawDebugResource(category entity.Name, name entity.Name) (entity.Object, entity.Object, int) {
	frames := e.resourceFramesForLookup()
	for i, resources := range frames {
		raw, resolved := e.lookupRawDebugResourceInFrame(resources, category, name)
		if raw != nil || resolved != nil {
			return raw, resolved, i
		}
	}
	for i, resources := range frames {
		if resources == nil {
			continue
		}
		raw := resources.GetRaw(name)
		if raw == nil {
			continue
		}
		return raw, e.resolveResourceEntryObject(raw, 0), i
	}
	return nil, nil, -1
}

func (e *Evaluator) lookupRawDebugResourceInFrame(resources *entity.Dict, category entity.Name, name entity.Name) (entity.Object, entity.Object) {
	if resources == nil {
		return nil, nil
	}
	categoryObj := resources.GetRaw(category)
	categoryResolved := e.resolveResourceEntryObject(categoryObj, 0)
	if categoryStream, ok := categoryResolved.(*entity.Stream); ok {
		categoryResolved = categoryStream.Dict()
	}
	categoryDict, ok := categoryResolved.(*entity.Dict)
	if !ok {
		return nil, nil
	}
	raw := categoryDict.GetRaw(name)
	if raw == nil {
		return nil, nil
	}
	return raw, e.resolveResourceEntryObject(raw, 0)
}

func debugObjectSummary(obj entity.Object) string {
	switch v := obj.(type) {
	case nil:
		return "-"
	case entity.Ref:
		return v.String()
	case *entity.Stream:
		return "stream{" + debugDictTypeSummary(v.Dict()) + "}"
	case *entity.Dict:
		return "dict{" + debugDictTypeSummary(v) + "}"
	case *entity.Array:
		return fmt.Sprintf("array[%d]", v.Len())
	default:
		return obj.Type().String()
	}
}

func debugDictTypeSummary(dict *entity.Dict) string {
	if dict == nil {
		return "-"
	}
	parts := make([]string, 0, 3)
	if subtype, ok := dict.Get(pdfNameSubtype).(entity.Name); ok {
		parts = append(parts, "Subtype="+strings.TrimPrefix(subtype.Value(), "/"))
	}
	if typ, ok := dict.Get(pdfNameType).(entity.Name); ok {
		parts = append(parts, "Type="+strings.TrimPrefix(typ.Value(), "/"))
	}
	if shadingType, ok := dict.Get(pdfNameShadingType).(*entity.Integer); ok {
		parts = append(parts, fmt.Sprintf("ShadingType=%d", shadingType.Value()))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ",")
}

var debugRenderContext = debugRenderContextFromEnv()

func debugRenderContextEnabled() bool {
	return debugRenderContext
}

func debugRenderContextFromEnv() bool {
	return os.Getenv("PDF_DEBUG_RENDER_CONTEXT") != "" ||
		os.Getenv("PDF_DEBUG_SPLASH_LAST_WRITER") != "" ||
		os.Getenv("PDF_DEBUG_SPLASH_IMAGE_TRACE") != "" ||
		os.Getenv("PDF_DEBUG_SPLASH_IMAGE_MASK_TRACE") != "" ||
		os.Getenv("PDF_DEBUG_SPLASH_SOFTMASK_IMAGE_TRACE") != "" ||
		os.Getenv("PDF_DEBUG_SPLASH_SOFTMASK_TRACE") != "" ||
		os.Getenv("PDF_DEBUG_SPLASH_GROUP_COMPOSITE_TRACE") != "" ||
		os.Getenv("PDF_DEBUG_SPLASH_GLYPH_TRACE") != "" ||
		os.Getenv("PDF_DEBUG_SPLASH_PIPE_TRACE") != "" ||
		os.Getenv("PDF_DEBUG_SPLASH_TEXT_PATH_TRACE") != "" ||
		os.Getenv("PDF_DEBUG_SPLASH_CLIP_AABUF_TRACE") != "" ||
		os.Getenv("PDF_DEBUG_SPLASH_SCANNER_CLIP_FULL_TRACE") != ""
}

// executeOperator executes a single graphics operator.
func (e *Evaluator) executeOperator(op Operator) error {
	if debugRenderContextEnabled() {
		restoreDebugContext := e.installDebugPaintContext(op)
		defer restoreDebugContext()
	}

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
		if op.InlineImage != nil {
			return e.executeInlineImageOperator(op)
		}
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
		data, err := e.decodeEntityStream(charProcStream)
		if err != nil {
			return fmt.Errorf("type3 font: decode charproc: %w", err)
		}

		ops, err = e.parseOperatorsOnly(data)
		if err != nil {
			return fmt.Errorf("type3 font: parse charproc: %w", err)
		}
		e.ensureCharProcCache()[charProcStream] = ops
	}

	glyphCTM := e.type3GlyphCTM(font, x, y, fontSize)
	if os.Getenv("PDF_DEBUG_TYPE3_CTM") != "" {
		fm := font.FontMatrix()
		oc := e.graphics.transform
		tm := e.textMatrix
		hs := e.graphics.currentState.GetHorizontalScaling() / 100.0
		fmt.Fprintf(os.Stderr, "T3CTM code=%d fs=%.17g hs=%.17g fm=[%.17g %.17g %.17g %.17g] tm=[%.17g %.17g %.17g %.17g %.17g %.17g] octm=[%.17g %.17g %.17g %.17g %.17g %.17g] lin=[%.17g %.17g %.17g %.17g] xy=[%.17g %.17g]\n",
			charCode, fontSize, hs, fm[0], fm[1], fm[2], fm[3],
			tm[0], tm[1], tm[2], tm[3], tm[4], tm[5],
			oc[0], oc[1], oc[2], oc[3], oc[4], oc[5],
			glyphCTM[0], glyphCTM[1], glyphCTM[2], glyphCTM[3], x, y)
	}
	usesD1Cache := type3CharProcUsesD1Cache(font, ops, glyphCTM)
	if usesD1Cache {
		if quantizer, ok := e.canvas.(interface {
			QuantizeType3GlyphOrigin(x, y float64) (float64, float64)
		}); ok {
			x, y = quantizer.QuantizeType3GlyphOrigin(x, y)
			glyphCTM = e.type3GlyphCTM(font, x, y, fontSize)
		}
	}

	if usesD1Cache {
		if marker, ok := e.canvas.(interface {
			BeginType3GlyphCache()
			EndType3GlyphCache()
		}); ok {
			marker.BeginType3GlyphCache()
			defer marker.EndType3GlyphCache()
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
	e.preserveNonEmptyCallerPathOnSavedState()
	defer func() { _ = e.restoreState() }()

	e.graphics.transform = glyphCTM

	if glyphResources := font.Resources(); glyphResources != nil {
		defer e.pushResources(glyphResources)()
	}
	if charProcStream.Dict() != nil {
		if charProcResources := e.resourceDictFromObject(charProcStream.Dict().Get(pdfNameResources)); charProcResources != nil {
			defer e.pushResources(charProcResources)()
		}
	}

	// Poppler-matching temp-bitmap render (SplashOutputDev::type3D1 →
	// drawType3Glyph) for cacheable (d1, cache-fitting) glyphs. Replaying the
	// CharProc in y-down cell space at glyph-relative magnitude eliminates the
	// page-height flip float cancellation at 45° cap edges (doc_027 DejaVuSans
	// Type3: 97px → 0). The temp splash is created when the d1 OPERATOR
	// executes (Poppler creates it in type3D1) — ops BEFORE d1 (e.g. samsung's
	// color set) must apply to the page state, not the glyph cell. Default on;
	// set PDF_TYPE3_TEMPBITMAP=0 to fall back to the direct quantized-pen
	// render.
	if usesD1Cache && os.Getenv("PDF_TYPE3_TEMPBITMAP") != "0" {
		if tb, ok := e.canvas.(type3TempBitmapCanvas); ok {
			llx, lly, urx, ury := font.GetBoundingBox()
			pending := &type3PendingTemp{
				canvas:   tb,
				glyphCTM: glyphCTM,
				fontBBox: [4]float64{llx, lly, urx, ury},
			}
			e.type3PendingTemp = pending
			defer func() {
				e.type3PendingTemp = nil
				if pending.active {
					tb.FinishType3TempBitmap()
				}
			}()
		}
	}

	e.executeCachedOperators(ops)
	return nil
}

type type3TempBitmapCanvas interface {
	PrepareType3TempBitmap(glyphCTM [6]float64, fontBBox [4]float64) (tempCTM [6]float64, w, h, blitX, blitY int, ok bool)
	FinishType3TempBitmap()
}

type type3PendingTemp struct {
	canvas   type3TempBitmapCanvas
	glyphCTM [6]float64
	fontBBox [4]float64
	active   bool
}

func type3CharProcUsesD1Cache(font *entity.Type3Font, ops []Operator, glyphCTM [6]float64) bool {
	d1BBox, ok := type3CharProcFirstD1BBox(ops)
	if !ok {
		return false
	}
	return type3D1BBoxFitsPopplerCache(font, glyphCTM, d1BBox)
}

func type3CharProcFirstD1BBox(ops []Operator) ([4]float64, bool) {
	for _, op := range ops {
		switch op.Opcode {
		case "d1":
			if len(op.Operands) < 6 {
				return [4]float64{}, false
			}
			llx, err := getNumberOperand(op.Operands[2])
			if err != nil {
				return [4]float64{}, false
			}
			lly, err := getNumberOperand(op.Operands[3])
			if err != nil {
				return [4]float64{}, false
			}
			urx, err := getNumberOperand(op.Operands[4])
			if err != nil {
				return [4]float64{}, false
			}
			ury, err := getNumberOperand(op.Operands[5])
			if err != nil {
				return [4]float64{}, false
			}
			return [4]float64{llx, lly, urx, ury}, true
		case "d0", "q", "Q":
			return [4]float64{}, false
		}
	}
	return [4]float64{}, false
}

func type3D1BBoxFitsPopplerCache(font *entity.Type3Font, glyphCTM [6]float64, d1BBox [4]float64) bool {
	if font == nil {
		return false
	}

	xt, yt := transformPointWithMatrix(glyphCTM, 0, 0)
	llx, lly, urx, ury := font.GetBoundingBox()
	fontBBox := [4]float64{llx, lly, urx, ury}
	validBBox := !(fontBBox[0] == 0 && fontBBox[1] == 0 && fontBBox[2] == 0 && fontBBox[3] == 0)

	var xMin, yMin, xMax, yMax float64
	if validBBox {
		xMin, yMin, xMax, yMax = transformedBBoxBounds(glyphCTM, fontBBox)
	} else {
		// Poppler guesses a cache box for an unspecified Type3 FontBBox.
		xMin = xt - 5
		xMax = xMin + 30
		yMax = yt + 15
		yMin = yMax - 45
	}

	glyphX := math.Floor(xMin-xt) - 2
	glyphY := math.Floor(yMin-yt) - 2
	glyphW := math.Ceil(xMax) - math.Floor(xMin) + 4
	glyphH := math.Ceil(yMax) - math.Floor(yMin) + 4
	if glyphW <= 0 || glyphH <= 0 || glyphW*glyphH > 100000 {
		glyphW = 100
		glyphH = 100
	}

	d1XMin, d1YMin, d1XMax, d1YMax := transformedBBoxBounds(glyphCTM, d1BBox)
	return d1XMin-xt >= glyphX &&
		d1YMin-yt >= glyphY &&
		d1XMax-xt <= glyphX+glyphW &&
		d1YMax-yt <= glyphY+glyphH
}

func transformedBBoxBounds(m [6]float64, bbox [4]float64) (float64, float64, float64, float64) {
	points := [4][2]float64{
		{bbox[0], bbox[1]},
		{bbox[0], bbox[3]},
		{bbox[2], bbox[1]},
		{bbox[2], bbox[3]},
	}
	xMin, yMin := transformPointWithMatrix(m, points[0][0], points[0][1])
	xMax, yMax := xMin, yMin
	for _, pt := range points[1:] {
		x, y := transformPointWithMatrix(m, pt[0], pt[1])
		if x < xMin {
			xMin = x
		} else if x > xMax {
			xMax = x
		}
		if y < yMin {
			yMin = y
		} else if y > yMax {
			yMax = y
		}
	}
	return xMin, yMin, xMax, yMax
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
	// Width and bbox are already known from glyph metrics. Rendering-wise this
	// mirrors SplashOutputDev::type3D1: for a cacheable glyph the temp cell
	// splash is created HERE — ops before d1 applied to the page state.
	if pending := e.type3PendingTemp; pending != nil && !pending.active {
		tempCTM, _, _, _, _, tempOK := pending.canvas.PrepareType3TempBitmap(pending.glyphCTM, pending.fontBBox)
		if tempOK {
			pending.active = true
			e.graphics.transform = tempCTM
			// Poppler's temp splash starts from a DEFAULT SplashState
			// (fill/stroke alpha 1 — type3D1's "this should copy other
			// state" is deliberately not done); the page alpha applies ONCE
			// at the drawType3Glyph blit.
			e.graphics.fillAlpha = 1
			e.graphics.strokeAlpha = 1
		}
	}
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

	// Push onto stack (pre-allocated with capacity to reduce growslice).
	e.stateStack = append(e.stateStack, stateCopy)
	return nil
}

func (e *Evaluator) preserveNonEmptyCallerPathOnSavedState() {
	if e == nil || e.graphics == nil || len(e.stateStack) == 0 {
		return
	}
	saved := e.stateStack[len(e.stateStack)-1]
	if e.graphics.path != nil && !e.graphics.path.IsEmpty() {
		saved.path = e.graphics.path.Clone()
	}
	if e.graphics.rawPath != nil && !e.graphics.rawPath.IsEmpty() {
		saved.rawPath = e.graphics.rawPath.Clone()
	}
}

// restoreState restores the last saved graphics state (for 'Q' operator).
func (e *Evaluator) restoreState() error {
	if len(e.stateStack) == 0 {
		return fmt.Errorf("graphics state stack is empty")
	}

	currentState := e.graphics.currentState

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
	e.graphics.blendMode = state.blendMode
	e.graphics.alphaIsShape = state.alphaIsShape
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
	e.graphics.fillParsedCS = state.fillParsedCS
	e.graphics.strokeParsedCS = state.strokeParsedCS
	e.graphics.fillPatternBaseCS = state.fillPatternBaseCS
	e.graphics.strokePatternBaseCS = state.strokePatternBaseCS
	e.graphics.font = state.font
	e.graphics.fontDebugName = state.fontDebugName
	e.graphics.fontSize = state.fontSize
	restoredCurrentState := state.currentState
	e.graphics.currentState = restoredCurrentState
	e.graphics.strokeAdjust = state.strokeAdjust
	e.syncTextMatricesState(state.textMatrix, state.textLine)
	e.textBaseMatrix = state.textBaseMatrix
	e.textLineX = state.textLineX
	e.textLineY = state.textLineY
	e.textUserCurrentX = state.textUserCurrentX
	e.textUserCurrentY = state.textUserCurrentY
	e.textUserCurrentValid = state.textUserCurrentValid
	e.graphics.path = state.path
	e.graphics.rawPath = state.rawPath
	e.graphics.pathClip = state.pathClip
	e.graphics.clipMode = state.clipMode
	e.graphics.pendingClip = state.pendingClip
	e.graphics.pendingClipMode = state.pendingClipMode

	// Return state to pool for reuse.
	*state = GraphicsState{}
	gsPool.Put(state)
	if currentState != restoredCurrentState {
		releaseCurrentState(currentState)
	}

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
	if os.Getenv("PDF_DEBUG_CM") == "1" {
		fmt.Fprintf(os.Stderr, "CM: matrix=[%f %f %f %f %f %f]\n", matrix[0], matrix[1], matrix[2], matrix[3], matrix[4], matrix[5])
	}
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
	if notifier, ok := e.canvas.(interface{ BeginPDFTextObject() }); ok {
		notifier.BeginPDFTextObject()
	}
}

func (e *Evaluator) endTextObject() {
	// Keep current text state as-is for ET. Next BT resets matrices.
	// Re-sync text matrix from current state to avoid stale references.
	e.syncTextMatrixState(e.textMatrix)
	if notifier, ok := e.canvas.(interface{ EndPDFTextObject() }); ok {
		notifier.EndPDFTextObject()
	}
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
	if len(text) == 0 {
		return nil
	}

	if font != nil && font.IsCIDFont() {
		out := make([]textCodeUnit, 0, (len(text)+1)/2)
		for i := 0; i < len(text); {
			if i+1 < len(text) {
				out = append(out, textCodeUnit{
					code: uint32(text[i])<<8 | uint32(text[i+1]),
					raw:  text[i : i+2],
				})
				i += 2
				continue
			}
			out = append(out, textCodeUnit{
				code: uint32(text[i]),
				raw:  text[i : i+1],
			})
			i++
		}
		return out
	}

	out := make([]textCodeUnit, 0, len(text))
	for i := range text {
		out = append(out, textCodeUnit{
			code: uint32(text[i]),
			raw:  text[i : i+1],
		})
	}
	return out
}

func (e *Evaluator) splitTextCodeUnitsScratch(text string, font entity.Font) []textCodeUnit {
	if len(text) == 0 {
		return nil
	}
	if e == nil {
		return splitTextCodeUnits(text, font)
	}

	needed := len(text)
	if font != nil && font.IsCIDFont() {
		needed = (len(text) + 1) / 2
	}
	if cap(e.textCodeUnitScratch) < needed {
		e.textCodeUnitScratch = make([]textCodeUnit, 0, needed)
	} else {
		e.textCodeUnitScratch = e.textCodeUnitScratch[:0]
	}
	out := e.textCodeUnitScratch

	if font != nil && font.IsCIDFont() {
		for i := 0; i < len(text); {
			if i+1 < len(text) {
				out = append(out, textCodeUnit{
					code: uint32(text[i])<<8 | uint32(text[i+1]),
					raw:  text[i : i+2],
				})
				i += 2
				continue
			}
			out = append(out, textCodeUnit{
				code: uint32(text[i]),
				raw:  text[i : i+1],
			})
			i++
		}
		e.textCodeUnitScratch = out
		return out
	}

	for i := range text {
		out = append(out, textCodeUnit{
			code: uint32(text[i]),
			raw:  text[i : i+1],
		})
	}
	e.textCodeUnitScratch = out
	return out
}

func (e *Evaluator) glyphAdvance(charCode uint32, font entity.Font, fontSize float64) float64 {
	width := 500.0
	hasWidth := false
	if typed, ok := unwrapCharCodeWidthFont(font); ok {
		if codeWidth, found := typed.GetCharCodeWidth(charCode); found {
			width = codeWidth
			hasWidth = true
		}
	}
	if !hasWidth {
		// For CID fonts, look up width by CID directly (not by CIDToGID-mapped GID).
		// This matches Poppler's behavior where /W array is indexed by CID.
		if font.IsCIDFont() {
			if glyphWidth, widthErr := font.GetGlyphWidth(charCode); widthErr == nil {
				width = glyphWidth
				hasWidth = true
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
		return graphics.NewStateWithoutCurrentPath()
	}

	// Value copy captures all scalar fields; only slices/pointers need deep copy.
	dst := graphicsStatePool.Get().(*graphics.State)
	*dst = *src

	if dash := src.GetDashArray(); len(dash) > 0 {
		copiedDash := append([]float64(nil), dash...)
		dst.SetDashArray(copiedDash, src.GetDashPhase())
	}
	// Renderer path/clip state is tracked on GraphicsState itself; the embedded
	// graphics.State path fields are legacy content-operator state and are not
	// consulted by this evaluator.
	dst.SetCurrentPath(nil)
	dst.SetClipPath(nil)

	return dst
}

func releaseCurrentState(state *graphics.State) {
	if state == nil {
		return
	}
	*state = graphics.State{}
	graphicsStatePool.Put(state)
}

func (e *Evaluator) releaseSavedGraphicsStates() {
	for i, state := range e.stateStack {
		if state == nil {
			continue
		}
		releaseCurrentState(state.currentState)
		*state = GraphicsState{}
		gsPool.Put(state)
		e.stateStack[i] = nil
	}
	e.stateStack = e.stateStack[:0]
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
	if !e.hasResourceFrames() {
		// Be permissive for malformed content streams and continue rendering.
		return nil
	}

	// Get font dictionary from resources
	fontObj := e.getResourceEntry(pdfNameFont, fontName)
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
	if obj := fontDict.Get(pdfNameBaseFont); obj != nil {
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
		if font != nil {
			e.ensureFontCache()[fontDict] = font
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
	subtypeObj := dict.Get(pdfNameSubtype)
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

	candidateFont = e.applyFontEncodingFromDictWithEmbeddedData(dict, candidateFont, embeddedFontData)
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
	if fmObj := dict.Get(pdfNameFontMatrix); fmObj != nil {
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
	if bbObj := dict.Get(pdfNameFontBBox); bbObj != nil {
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
	if cpObj := dict.Get(pdfNameCharProcs); cpObj != nil {
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
	if encObj := dict.Get(pdfNameEncoding); encObj != nil {
		if encDict, ok := encObj.(*entity.Dict); ok {
			if diffObj := encDict.Get(pdfNameDifferences); diffObj != nil {
				if diffArr, ok := diffObj.(*entity.Array); ok {
					parseEncodingDifferences(diffArr, encoding)
				}
			}
		}
	}

	// Parse Widths array
	var firstChar, lastChar uint32
	if fcObj := dict.Get(pdfNameFirstChar); fcObj != nil {
		if num, err := getNumberOperand(fcObj); err == nil {
			firstChar = uint32(num)
		}
	}
	if lcObj := dict.Get(pdfNameLastChar); lcObj != nil {
		if num, err := getNumberOperand(lcObj); err == nil {
			lastChar = uint32(num)
		}
	}

	widths := make(map[uint32]float64)
	if wObj := dict.Get(pdfNameWidths); wObj != nil {
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
	if resources := e.resourceDictFromObject(dict.Get(pdfNameResources)); resources != nil {
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
	if !e.hasResourceFrames() {
		return errors.NotFound("get_xobject", fmt.Errorf("no resources available"))
	}

	xobjVal := e.getResourceEntry(pdfNameXObject, xname)
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
	subtypeVal := dict.Get(pdfNameSubtype)
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
	content, err := e.formOperatorContentForExecution(xobj)
	if err != nil {
		return err
	}
	popDebugPath := e.pushDebugPath("form:" + strings.TrimPrefix(name.Value(), "/"))
	defer popDebugPath()

	// Save current graphics state
	if err := e.saveState(); err != nil {
		return err
	}
	e.preserveNonEmptyCallerPathOnSavedState()
	restoreState := true
	defer func() {
		if restoreState {
			_ = e.restoreState()
		}
	}()

	// Poppler's Gfx::drawForm kills any pre-existing path before applying the
	// form matrix/BBox, while keeping the caller state available for restore.
	e.clearCurrentPathForForm()

	// Get form's dictionary for additional parameters
	dict := xobj.Dict()

	if os.Getenv("PDF_DEBUG_EVAL_GS") != "" {
		hasGroup := "no"
		if d, ok := e.resolveDictObject(dict.Get(pdfNameGroup)); ok && isTransparencyGroupDict(d) {
			hasGroup = "GROUP"
		}
		fmt.Fprintf(os.Stderr, "FORMENTER name=%v hasGroup=%s fillAlpha=%.4f blend=%v\n",
			name, hasGroup, e.graphics.fillAlpha, e.graphics.blendMode)
	}

	// Get form's matrix (transformation) if present
	matrixVal := dict.Get(pdfNameMatrix)
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
	var formResources *entity.Dict
	resourcesVal := dict.Get(pdfNameResources)
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
			formResources = resourcesDict
		}
	}
	defer e.pushResources(formResources)()

	// Apply form bounding box clipping when present.
	bboxVal := dict.Get(pdfNameBBox)
	var bbox [4]float64
	hasBBox := false
	if bboxVal != nil {
		if bboxArr, ok := bboxVal.(*entity.Array); ok {
			if err := e.applyFormBBoxClip(bboxArr); err != nil {
				return errors.Invalid("form_bbox_clip", err)
			}
			bbox, hasBBox = numericArray4(bboxArr)
			e.clearCurrentPathForForm()
		}
	}

	if groupController, ok := e.canvas.(transparencyGroupCanvas); ok && hasBBox && enableFormTransparencyGroup() {
		if groupDict, ok := e.resolveDictObject(dict.Get(pdfNameGroup)); ok && isTransparencyGroupDict(groupDict) {
			isolated := boolDictValue(groupDict, pdfNameI, false)
			knockout := boolDictValue(groupDict, pdfNameK, false)
			if os.Getenv("PDF_DEBUG_EVAL_GS") != "" {
				inNonIso := false
				if c, ok := e.canvas.(nonIsolatedGroupCanvas); ok {
					inNonIso = c.InNonIsolatedGroup()
				}
				fmt.Fprintf(os.Stderr, "FORMGATE name=%v isolated=%t knockout=%t required=%t fillAlpha=%.4f blend=%v inNonIso=%t\n",
					name, isolated, knockout, e.formTransparencyGroupRequired(isolated, knockout, formResources),
					e.graphics.fillAlpha, e.graphics.blendMode, inNonIso)
			}
			if e.formTransparencyGroupRequired(isolated, knockout, formResources) {
				if deviceGroupController, ok := e.canvas.(transparencyGroupDeviceBBoxCanvas); ok {
					x0, y0, x1, y1 := transformedBBoxBounds(e.graphics.transform, bbox)
					deviceBBox := [4]float64{x0, y0, x1, y1}
					if os.Getenv("PDF_DEBUG_EVAL_GS") != "" {
						fmt.Fprintf(os.Stderr, "FORMBEGIN name=%v devbbox=[%.0f %.0f %.0f %.0f] savedFillAlpha=%.4f savedBlend=%v\n",
							name, x0, y0, x1, y1, e.graphics.fillAlpha, e.graphics.blendMode)
					}
					if croppedController, ok := e.canvas.(transparencyGroupCroppedDeviceBBoxCanvas); ok && enableFormTransparencyGroupCropped() {
						tx, ty, err := croppedController.BeginTransparencyGroupCroppedDeviceBBox(deviceBBox, isolated, knockout)
						if err != nil {
							return err
						}
						e.graphics.transform[4] -= float64(tx)
						if e.canvasYDownBase() {
							// y-down CTM: the cropped group origin shift is a
							// plain device translation in both axes.
							e.graphics.transform[5] -= float64(ty)
						} else {
							e.graphics.transform[5] += float64(ty)
						}
						e.graphics.baseTransform = e.graphics.transform
					} else if err := deviceGroupController.BeginTransparencyGroupDeviceBBox(deviceBBox, isolated, knockout); err != nil {
						return err
					}
				} else if err := groupController.BeginTransparencyGroup(bbox, isolated, knockout); err != nil {
					return err
				}
				// A Form transparency group composites through the soft mask active
				// at Do time (Poppler's parent Splash state keeps its softMask for
				// the paintTransparencyGroup composite). Begin captured the parent
				// mask as savedSoftMask (restored by PaintTransparencyGroup); only
				// now — AFTER capture — clear it so the group's content renders in a
				// fresh-child state. Clearing before Begin snapshotted a nil mask, so
				// an ExtGState luminosity SMask at Do (e.g. an Illustrator drop
				// shadow) was ignored at composite and the group flooded its bbox.
				if softMaskController, ok := e.canvas.(softMaskGroupCanvas); ok {
					softMaskController.ClearSoftMask()
				}
				// A Form transparency group composites with the fill/stroke alpha
				// active at Do time (Poppler restores the parent state before
				// paintTransparencyGroup). BeginTransparencyGroup captured that
				// parent alpha above as the group's savedFillAlpha/savedStrokeAlpha
				// (restored by PaintTransparencyGroup for the composite). Only now —
				// AFTER the parent alpha is captured — reset to 1.0 so the group's
				// content renders in a fresh-child state. Resetting before Begin
				// clobbered a low parent /ca (e.g. 0.05), making the composite opaque
				// where Poppler shows a faint tint.
				e.graphics.fillAlpha = 1
				e.graphics.strokeAlpha = 1
				e.syncCanvasFillAlpha()
				e.syncCanvasStrokeAlpha()
				// A Form transparency group composites with the blend mode active at
				// Do time (Poppler restores the parent state before paint), so
				// BeginTransparencyGroup captured the parent blend as savedBlendFunc
				// (restored by PaintTransparencyGroup for the composite). Only now —
				// AFTER capture — reset to Normal so the group's content renders in a
				// fresh-child state (Poppler's child Splash uses Normal blend). Resetting
				// before Begin captured Normal instead of the parent's Multiply/Overlay,
				// so the composite used Normal where Poppler used the parent blend.
				if setter, ok := e.canvas.(blendModeSetter); ok {
					setter.SetBlendMode("Normal")
				}
				e.graphics.blendMode = "Normal"
				if err := e.executeFormOperatorContent(content); err != nil {
					_ = groupController.DiscardTransparencyGroup()
					return errors.Invalid("evaluate_form_xobject", err)
				}
				// Restore the caller state BEFORE compositing: Poppler's
				// Gfx::drawForm calls restoreStateStack and only then
				// paintTransparencyGroup, so the form BBox clip (applied before
				// Begin) is NOT active at composite time — only the caller's
				// clip is. Compositing under the BBox clip binary-rejected the
				// fractional edge column that Poppler's composite keeps (DallE
				// p63/p67: card right edge at x=700.0 exactly, xMaxI=699 cut
				// the 12.5%-coverage column the group content carried).
				if err := e.restoreState(); err != nil {
					restoreState = false
					_ = groupController.DiscardTransparencyGroup()
					return err
				}
				restoreState = false
				if err := groupController.PaintTransparencyGroup(); err != nil {
					return err
				}
				return nil
			}
		}
	}

	if err := e.executeFormOperatorContent(content); err != nil {
		return errors.Invalid("evaluate_form_xobject", err)
	}

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
	e.clearCurrentPath()
	e.graphics.pendingClip = false
	e.graphics.pendingClipMode = ClipNonZeroWinding
}

func (e *Evaluator) clearCurrentPath() {
	if e == nil || e.graphics == nil {
		return
	}
	if e.graphics.path == nil {
		e.graphics.path = NewPath()
	} else {
		e.graphics.path.Clear()
	}
	if e.graphics.rawPath == nil {
		e.graphics.rawPath = NewPath()
	} else {
		e.graphics.rawPath.Clear()
	}
}

func (e *Evaluator) currentRawPath() *Path {
	if e == nil || e.graphics == nil {
		return nil
	}
	if e.graphics.rawPath == nil {
		e.graphics.rawPath = NewPath()
	}
	return e.graphics.rawPath
}

type formOperatorContent struct {
	ops  []Operator
	data []byte
}

func (e *Evaluator) formOperatorContentForExecution(xobj *entity.Stream) (formOperatorContent, error) {
	if xobj == nil {
		return formOperatorContent{}, errors.Invalid("decode_form_xobject", fmt.Errorf("nil form xobject"))
	}

	if cached, ok := e.formOperatorCache[xobj]; ok {
		return formOperatorContent{ops: cached}, nil
	}

	if e.sharedFormCache != nil {
		if cached, ok := e.sharedFormCache.Get(xobj); ok {
			e.ensureFormOperatorCache()[xobj] = cached
			return formOperatorContent{ops: cached}, nil
		}
	}

	data, err := e.decodeEntityStreamWithSizeHint(xobj, formStreamDecodeSizeHint(xobj))
	if err != nil {
		return formOperatorContent{}, errors.Invalid("decode_form_xobject", err)
	}

	if e.shouldStreamFirstFormUse(xobj, data) {
		return formOperatorContent{data: data}, nil
	}

	ops, err := e.parseOperatorsOnly(data)
	if err != nil {
		return formOperatorContent{}, errors.Invalid("evaluate_form_xobject", err)
	}
	e.ensureFormOperatorCache()[xobj] = ops
	if e.sharedFormCache != nil {
		e.sharedFormCache.Set(xobj, ops)
	}
	return formOperatorContent{ops: ops}, nil
}

func (e *Evaluator) shouldStreamFirstFormUse(xobj *entity.Stream, data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if e.formStreamUseCount == nil {
		e.formStreamUseCount = make(map[*entity.Stream]int)
	}
	count := e.formStreamUseCount[xobj]
	e.formStreamUseCount[xobj] = count + 1
	return count == 0
}

func (e *Evaluator) executeFormOperatorContent(content formOperatorContent) error {
	if content.ops != nil {
		e.executeCachedOperators(content.ops)
		return nil
	}
	if len(content.data) == 0 {
		return nil
	}
	return e.parseOperatorsForImmediateExecution(content.data, func(op Operator) {
		if err := e.executeOperator(op); err != nil {
			return
		}
	})
}

func (e *Evaluator) cachedFormOperators(xobj *entity.Stream) ([]Operator, error) {
	if xobj == nil {
		return nil, errors.Invalid("decode_form_xobject", fmt.Errorf("nil form xobject"))
	}

	if cached, ok := e.formOperatorCache[xobj]; ok {
		return cached, nil
	}

	if e.sharedFormCache != nil {
		if cached, ok := e.sharedFormCache.Get(xobj); ok {
			e.ensureFormOperatorCache()[xobj] = cached
			return cached, nil
		}
	}

	data, err := e.decodeEntityStreamWithSizeHint(xobj, formStreamDecodeSizeHint(xobj))
	if err != nil {
		return nil, errors.Invalid("decode_form_xobject", err)
	}

	ops, err := e.parseOperatorsOnly(data)
	if err != nil {
		return nil, errors.Invalid("evaluate_form_xobject", err)
	}
	e.ensureFormOperatorCache()[xobj] = ops
	if e.sharedFormCache != nil {
		e.sharedFormCache.Set(xobj, ops)
	}
	return ops, nil
}

func formStreamDecodeSizeHint(xobj *entity.Stream) int {
	if xobj == nil || xobj.Dict() == nil {
		return 0
	}
	subtype, _ := xobj.Dict().Get(pdfNameSubtype).(entity.Name)
	if strings.TrimPrefix(subtype.Value(), "/") != "Form" {
		return 0
	}
	rawLen := len(xobj.RawBytes())
	const mediumFormDecodeHintMin = 32 << 10
	const largeFormDecodeHintMin = 128 << 10
	if rawLen < mediumFormDecodeHintMin {
		return 0
	}
	const maxFormDecodeSizeHint = 64 << 20
	multiplier := 6
	if rawLen >= largeFormDecodeHintMin {
		multiplier = 16
	}
	hint := rawLen * multiplier
	if hint <= 0 || hint > maxFormDecodeSizeHint {
		return 0
	}
	return hint
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
	clipPath.AddRect(tx0, ty0, tx1, ty1, tx2, ty2, tx3, ty3)

	e.graphics.pathClip = clipPath
	e.setCurrentPathClipBounds(clipPath)
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
		if isSupportedImageColorSpace(colorSpace) {
			return colorSpace, true
		}
		if resourceObj := e.getResourceEntry(pdfNameColorSpace, cs); resourceObj != nil {
			return e.resolveImageColorSpaceWithDepth(resourceObj, depth+1)
		}
		return colorSpace, false
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

func (e *Evaluator) resolveImageColorMapper(colorSpace string, colorSpaceVal entity.Object) (colorspace.ColorSpace, bool) {
	switch colorSpace {
	case "Separation":
		parsed, ok := e.resolveTypedGraphicsColorSpace(colorSpaceVal)
		if !ok || parsed.Type() != colorspace.ColorSpaceSeparation || parsed.GetNumComponents() != 1 {
			return nil, false
		}
		return parsed, true
	case "CalRGB":
		parsed, ok := e.resolveTypedGraphicsColorSpace(colorSpaceVal)
		if !ok || parsed.Type() != colorspace.ColorSpaceCalRGB || parsed.GetNumComponents() != 3 {
			return nil, false
		}
		return parsed, true
	default:
		return nil, true
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

type softMaskDetails struct {
	stream *entity.Stream
	mask   domainimage.ImageMask
	matte  []float64
}

func (e *Evaluator) resolveSoftMask(maskObj entity.Object) domainimage.ImageMask {
	return e.resolveSoftMaskDetails(maskObj).mask
}

func (e *Evaluator) resolveSoftMaskDetails(maskObj entity.Object) softMaskDetails {
	if maskObj == nil {
		return softMaskDetails{}
	}

	maskStream, ok := e.resolveStreamObject(maskObj)
	if !ok || maskStream == nil {
		return softMaskDetails{}
	}
	if e.softMaskDetailsCache != nil {
		if details, ok := e.softMaskDetailsCache[maskStream]; ok {
			return details
		}
	}

	maskDict := maskStream.Dict()
	width, ok := objectInt(dictGetAny(maskDict, pdfNameWidth, pdfNameW))
	if !ok || width <= 0 {
		return softMaskDetails{}
	}
	height, ok := objectInt(dictGetAny(maskDict, pdfNameHeight, pdfNameH))
	if !ok || height <= 0 {
		return softMaskDetails{}
	}

	data, maskFilter, err := e.decodeSoftMaskImageStream(maskStream)
	if err != nil {
		return softMaskDetails{}
	}

	maskCS := "DeviceGray"
	if csObj := dictGetAny(maskDict, pdfNameColorSpace, pdfNameCS); csObj != nil {
		cs, ok := e.resolveImageColorSpace(csObj)
		if !ok || !strings.EqualFold(cs, "DeviceGray") {
			return softMaskDetails{}
		}
		maskCS = cs
	}
	bpc := 8
	if v, ok := objectInt(dictGetAny(maskDict, pdfNameBitsPerComponent, pdfNameBPC)); ok && v > 0 {
		bpc = v
	}

	var gray *stdimage.Gray
	decode := e.resolveImageDecodeArray(dictGetAny(maskDict, pdfNameDecode, pdfNameD))
	if maskFilter == domainimage.FilterNone {
		rawGray, ok := rawDeviceGraySoftMask(data, width, height, bpc, decode)
		if ok {
			gray = rawGray
		}
	}
	if gray == nil {
		decoded, err := e.decodeImageData(&domainimage.ImageData{
			Data:             data,
			Width:            width,
			Height:           height,
			BitsPerComponent: bpc,
			ColorSpace:       domainimage.ColorSpace(maskCS),
			Filter:           maskFilter,
			Decode:           decode,
		})
		if err != nil || decoded == nil || decoded.Image() == nil {
			return softMaskDetails{}
		}

		srcImg := decoded.Image()
		bounds := srcImg.Bounds()
		switch m := srcImg.(type) {
		case *stdimage.Gray:
			gray = m
		case *stdimage.Alpha:
			gray = stdimage.NewGray(bounds)
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				srcOff := m.PixOffset(bounds.Min.X, y)
				dstOff := gray.PixOffset(bounds.Min.X, y)
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					gray.Pix[dstOff] = m.Pix[srcOff]
					srcOff++
					dstOff++
				}
			}
		case *stdimage.RGBA:
			gray = stdimage.NewGray(bounds)
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				srcOff := m.PixOffset(bounds.Min.X, y)
				dstOff := gray.PixOffset(bounds.Min.X, y)
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					r := uint32(m.Pix[srcOff])
					r |= r << 8
					g := uint32(m.Pix[srcOff+1])
					g |= g << 8
					b := uint32(m.Pix[srcOff+2])
					b |= b << 8
					yVal := (19595*r + 38470*g + 7471*b + 1<<15) >> 16
					gray.Pix[dstOff] = byte(yVal >> 8)
					srcOff += 4
					dstOff++
				}
			}
		case *stdimage.NRGBA:
			gray = stdimage.NewGray(bounds)
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				srcOff := m.PixOffset(bounds.Min.X, y)
				dstOff := gray.PixOffset(bounds.Min.X, y)
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					a := uint32(m.Pix[srcOff+3])
					r := (uint32(m.Pix[srcOff]) | uint32(m.Pix[srcOff])<<8) * a / 0xff
					g := (uint32(m.Pix[srcOff+1]) | uint32(m.Pix[srcOff+1])<<8) * a / 0xff
					b := (uint32(m.Pix[srcOff+2]) | uint32(m.Pix[srcOff+2])<<8) * a / 0xff
					yVal := (19595*r + 38470*g + 7471*b + 1<<15) >> 16
					gray.Pix[dstOff] = byte(yVal >> 8)
					srcOff += 4
					dstOff++
				}
			}
		default:
			gray = stdimage.NewGray(bounds)
			for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
				for x := bounds.Min.X; x < bounds.Max.X; x++ {
					gray.Set(x, y, color.GrayModel.Convert(srcImg.At(x, y)))
				}
			}
		}
	}

	details := softMaskDetails{
		stream: maskStream,
		mask:   image.NewBitmapMaskFromImage(gray, false),
		matte:  e.resolveSoftMaskMatte(maskDict.Get(pdfNameMatte)),
	}
	if details.mask != nil {
		e.ensureSoftMaskDetailsCache()[maskStream] = details
	}
	return details
}

func dictGetAny(dict *entity.Dict, names ...entity.Name) entity.Object {
	if dict == nil {
		return nil
	}
	for _, name := range names {
		if obj := dict.Get(name); obj != nil {
			return obj
		}
	}
	return nil
}

func rawDeviceGraySoftMask(data []byte, width, height, bpc int, decode []float64) (*stdimage.Gray, bool) {
	if width <= 0 || height <= 0 || bpc != 8 || !isDefaultGrayDecodeArray(decode) {
		return nil, false
	}
	needed := width * height
	if needed <= 0 || len(data) < needed {
		return nil, false
	}
	return &stdimage.Gray{
		Pix:    data[:needed],
		Stride: width,
		Rect:   stdimage.Rect(0, 0, width, height),
	}, true
}

func isDefaultGrayDecodeArray(decode []float64) bool {
	return len(decode) == 0 || (len(decode) == 2 && decode[0] == 0 && decode[1] == 1)
}

func (e *Evaluator) resolveSoftMaskMatte(obj entity.Object) []float64 {
	arr, ok := e.resolveArrayObject(obj)
	if !ok || arr.Len() == 0 {
		return nil
	}
	matte := make([]float64, 0, arr.Len())
	for i := 0; i < arr.Len(); i++ {
		n, err := getNumberOperand(arr.Get(i))
		if err != nil {
			return nil
		}
		matte = append(matte, clamp(n, 0, 1))
	}
	return matte
}

func (e *Evaluator) resolveExplicitImageMask(maskObj entity.Object) domainimage.ImageMask {
	if maskObj == nil {
		return nil
	}

	maskStream, ok := e.resolveStreamObject(maskObj)
	if !ok || maskStream == nil || !isImageMaskDict(maskStream.Dict()) {
		return nil
	}

	width, ok := dictIntWithAliases(maskStream.Dict(), pdfNameWidth, pdfNameW)
	if !ok || width <= 0 {
		return nil
	}
	height, ok := dictIntWithAliases(maskStream.Dict(), pdfNameHeight, pdfNameH)
	if !ok || height <= 0 {
		return nil
	}

	data, err := e.decodeEntityStream(maskStream)
	if err != nil {
		return nil
	}
	decodeObj := firstNonNilObject(
		maskStream.Dict().Get(pdfNameDecode),
		maskStream.Dict().Get(pdfNameD),
	)
	paintBitOne := popplerExplicitImageMaskPaintBitOne(e.resolveImageDecodeArray(decodeObj))
	return decodeExplicitImageMaskData(data, width, height, paintBitOne)
}

func (e *Evaluator) resolveImageMaskInterpolate(maskObj entity.Object) bool {
	maskStream, ok := e.resolveStreamObject(maskObj)
	if !ok || maskStream == nil {
		return false
	}
	interpolate, _ := resolveImageInterpolateOption(firstNonNilObject(
		maskStream.Dict().Get(pdfNameInterpolate),
		maskStream.Dict().Get(pdfNameI),
	), false)
	return interpolate
}

func popplerExplicitImageMaskPaintBitOne(decode []float64) bool {
	if len(decode) == 0 {
		return false
	}
	return decode[0] >= 0.9
}

func decodeExplicitImageMaskData(data []byte, width, height int, paintBitOne bool) domainimage.ImageMask {
	if width <= 0 || height <= 0 {
		return nil
	}

	mask := stdimage.NewGray(stdimage.Rect(0, 0, width, height))
	bytesPerRow := (width + 7) / 8
	for y := 0; y < height; y++ {
		rowStart := y * bytesPerRow
		for x := 0; x < width; x++ {
			byteIdx := rowStart + x/8
			bitIdx := 7 - (x % 8)
			bitSet := byte(0)
			if byteIdx < len(data) {
				bitSet = (data[byteIdx] >> bitIdx) & 1
			}
			opaque := bitSet == 0
			if paintBitOne {
				opaque = bitSet == 1
			}
			alpha := uint8(0)
			if opaque {
				alpha = 0xFF
			}
			mask.SetGray(x, y, color.Gray{Y: alpha})
		}
	}
	return image.NewBitmapMaskFromImage(mask, false)
}

func decodeSoftMaskImageStream(maskStream *entity.Stream) ([]byte, domainimage.ImageFilter, error) {
	filterObj := maskStream.Dict().Get(pdfNameFilter)
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

	infra := stream.NewFromEntity(maskStream)
	if hint := softMaskDecodeSizeHint(maskStream.Dict()); hint > 0 {
		infra.SetDecodeSizeHint(hint)
	}
	data, err := infra.Decode()
	if err != nil {
		return nil, domainimage.FilterNone, err
	}
	return data, domainimage.FilterNone, nil
}

func (e *Evaluator) decodeSoftMaskImageStream(maskStream *entity.Stream) ([]byte, domainimage.ImageFilter, error) {
	filterObj := maskStream.Dict().Get(pdfNameFilter)
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

	data, err := e.decodeEntityStreamWithSizeHint(maskStream, softMaskDecodeSizeHint(maskStream.Dict()))
	if err != nil {
		return nil, domainimage.FilterNone, err
	}
	return data, domainimage.FilterNone, nil
}

func softMaskDecodeSizeHint(maskDict *entity.Dict) int {
	width, ok := objectInt(dictGetAny(maskDict, pdfNameWidth, pdfNameW))
	if !ok || width <= 0 {
		return 0
	}
	height, ok := objectInt(dictGetAny(maskDict, pdfNameHeight, pdfNameH))
	if !ok || height <= 0 {
		return 0
	}
	bpc := 8
	if v, ok := objectInt(dictGetAny(maskDict, pdfNameBitsPerComponent, pdfNameBPC)); ok && v > 0 {
		bpc = v
	}
	rowBits := int64(width) * int64(bpc)
	if rowBits <= 0 {
		return 0
	}
	size := ((rowBits + 7) / 8) * int64(height)
	const maxDecodeSizeHint = 512 << 20
	if size <= 0 || size > maxDecodeSizeHint {
		return 0
	}
	return int(size)
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

func dictIntWithAliases(dict *entity.Dict, names ...entity.Name) (int, bool) {
	if dict == nil {
		return 0, false
	}
	for _, name := range names {
		if value, ok := objectInt(dict.Get(name)); ok {
			return value, true
		}
	}
	return 0, false
}

func firstNonNilObject(objects ...entity.Object) entity.Object {
	for _, obj := range objects {
		if obj != nil {
			return obj
		}
	}
	return nil
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
	nObj := dict.Get(pdfNameN)
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

	filterArray, ok := xobj.Dict().Get(pdfNameFilter).(*entity.Array)
	if !ok || filterArray.Len() < prefixLen {
		return xobj.RawBytes(), nil
	}

	prefixDict := entity.NewDict()
	if prefixLen == 1 {
		prefixDict.Set(pdfNameFilter, filterArray.Get(0))
	} else {
		filters := make([]entity.Object, 0, prefixLen)
		for i := 0; i < prefixLen; i++ {
			filters = append(filters, filterArray.Get(i))
		}
		prefixDict.Set(pdfNameFilter, entity.NewArray(filters...))
	}
	if decodeParms := prefixDecodeParms(xobj.Dict().Get(pdfNameDecodeParms), prefixLen); decodeParms != nil {
		prefixDict.Set(pdfNameDecodeParms, decodeParms)
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
	switch normalized {
	case "AHx", "ASCIIHex", "ASCIIHexDecode":
		return domainimage.FilterASCIIHex
	case "A85", "ASCII85", "ASCII85Decode":
		return domainimage.FilterASCII85
	case "Fl", "Flate", "FlateDecode":
		return domainimage.FilterFlate
	case "LZW", "LZWDecode":
		return domainimage.FilterLZW
	case "RL", "RunLength", "RunLengthDecode":
		return domainimage.FilterRunLength
	case "CCF", "CCITT", "CCITTFax", "CCITTFaxDecode":
		return domainimage.FilterCCITTFax
	case "DCT", "DCTDecode":
		return domainimage.FilterDCT
	case "JPX", "JPXDecode":
		return domainimage.FilterJPX
	case "JBIG2", "JBIG2Decode":
		return domainimage.FilterJBIG2
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
		return normalizeImageFilterNameFolded(normalized)
	}
}

func normalizeImageFilterNameFolded(name string) domainimage.ImageFilter {
	switch {
	case strings.EqualFold(name, "AHx"), strings.EqualFold(name, "ASCIIHex"), strings.EqualFold(name, "ASCIIHexDecode"):
		return domainimage.FilterASCIIHex
	case strings.EqualFold(name, "A85"), strings.EqualFold(name, "ASCII85"), strings.EqualFold(name, "ASCII85Decode"):
		return domainimage.FilterASCII85
	case strings.EqualFold(name, "Fl"), strings.EqualFold(name, "Flate"), strings.EqualFold(name, "FlateDecode"):
		return domainimage.FilterFlate
	case strings.EqualFold(name, "LZW"), strings.EqualFold(name, "LZWDecode"):
		return domainimage.FilterLZW
	case strings.EqualFold(name, "RL"), strings.EqualFold(name, "RunLength"), strings.EqualFold(name, "RunLengthDecode"):
		return domainimage.FilterRunLength
	case strings.EqualFold(name, "CCF"), strings.EqualFold(name, "CCITT"), strings.EqualFold(name, "CCITTFax"), strings.EqualFold(name, "CCITTFaxDecode"):
		return domainimage.FilterCCITTFax
	case strings.EqualFold(name, "DCT"), strings.EqualFold(name, "DCTDecode"):
		return domainimage.FilterDCT
	case strings.EqualFold(name, "JPX"), strings.EqualFold(name, "JPXDecode"):
		return domainimage.FilterJPX
	case strings.EqualFold(name, "JBIG2"), strings.EqualFold(name, "JBIG2Decode"):
		return domainimage.FilterJBIG2
	default:
		return domainimage.ImageFilter(name)
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
	case "DeviceGray", "DeviceRGB", "DeviceCMYK", "Indexed", "Separation", "CalRGB":
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
	if os.Getenv("PDF_DEBUG_CM") == "1" {
		fmt.Fprintf(os.Stderr, "CM: matrix=[%f %f %f %f %f %f]\n", matrix[0], matrix[1], matrix[2], matrix[3], matrix[4], matrix[5])
	}
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
	e.graphics.strokeCS, e.graphics.strokePatternBaseCS, e.graphics.strokeParsedCS = e.resolveGraphicsColorSpaceDetails(csName)
	e.graphics.strokePattern = nil
	if !strings.EqualFold(e.graphics.strokeCS, "Pattern") {
		e.graphics.strokePatternBaseCS = ""
	}
	e.resetColorToColorSpaceDefault(true)
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
	e.graphics.fillCS, e.graphics.fillPatternBaseCS, e.graphics.fillParsedCS = e.resolveGraphicsColorSpaceDetails(csName)
	if os.Getenv("PDF_DEBUG_PATTERN_RESOLVE") == "1" {
		fmt.Fprintf(os.Stderr, "DEBUG setFillColorSpace: name=%s fillCS=%s patternBase=%s resources=%p\n",
			csName.String(), e.graphics.fillCS, e.graphics.fillPatternBaseCS, e.resources)
	}
	e.graphics.fillPattern = nil
	if !strings.EqualFold(e.graphics.fillCS, "Pattern") {
		e.graphics.fillPatternBaseCS = ""
	}
	e.resetColorToColorSpaceDefault(false)
	return nil
}

func (e *Evaluator) resolveGraphicsColorSpace(name entity.Name) string {
	colorSpace, _ := e.resolveGraphicsColorSpaceAndPatternBase(name)
	return colorSpace
}

func (e *Evaluator) resolveGraphicsColorSpaceAndPatternBase(name entity.Name) (string, string) {
	colorSpace, patternBase, _ := e.resolveGraphicsColorSpaceDetails(name)
	return colorSpace, patternBase
}

func (e *Evaluator) resolveGraphicsColorSpaceDetails(name entity.Name) (string, string, colorspace.ColorSpace) {
	base := normalizeImageColorSpaceName(name.Value())
	if strings.EqualFold(base, "Pattern") {
		return "Pattern", "", nil
	}
	if patternBase, ok := e.resolvePatternColorSpaceBase(name); ok {
		return "Pattern", patternBase, nil
	}

	if e.hasResourceFrames() {
		if csObj := e.getResourceEntry(pdfNameColorSpace, name); csObj != nil {
			if parsed, ok := e.resolveTypedGraphicsColorSpace(csObj); ok {
				return parsed.Type().String(), "", parsed
			}
			if resolved, ok := e.resolveImageColorSpace(csObj); ok {
				return resolved, "", nil
			}
		}
	}

	if isSupportedImageColorSpace(base) {
		return base, "", nil
	}

	return "DeviceRGB", "", nil
}

func (e *Evaluator) resolveTypedGraphicsColorSpace(obj entity.Object) (colorspace.ColorSpace, bool) {
	resolved := e.resolveColorSpaceObjectForRegistry(obj, 0)
	parsed, err := colorspace.NewRegistry().ParseColorSpace(resolved)
	if err != nil || parsed == nil {
		return nil, false
	}
	return parsed, true
}

func (e *Evaluator) resolveColorSpaceObjectForRegistry(obj entity.Object, depth int) entity.Object {
	if obj == nil || depth > 8 {
		return obj
	}

	resolved := e.resolveResourceEntryObject(obj, depth)
	arr, ok := resolved.(*entity.Array)
	if !ok {
		return resolved
	}

	items := make([]entity.Object, 0, arr.Len())
	for i := 0; i < arr.Len(); i++ {
		items = append(items, e.resolveColorSpaceObjectForRegistry(arr.Get(i), depth+1))
	}
	return entity.NewArray(items...)
}

func (e *Evaluator) isPatternColorSpaceResource(name entity.Name) bool {
	_, ok := e.resolvePatternColorSpaceBase(name)
	return ok
}

func (e *Evaluator) resolvePatternColorSpaceBase(name entity.Name) (string, bool) {
	if !e.hasResourceFrames() {
		return "", false
	}
	colorSpaceObj := e.getResourceEntry(pdfNameColorSpace, name)
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
		if baseRef, ok := typed.Get(1).(entity.Name); ok && e.hasResourceFrames() {
			if resolvedObj := e.getResourceEntry(pdfNameColorSpace, baseRef); resolvedObj != nil {
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

func (e *Evaluator) resetColorToColorSpaceDefault(stroke bool) {
	colorSpaceName := e.graphics.fillCS
	parsedColorSpace := e.graphics.fillParsedCS
	if stroke {
		colorSpaceName = e.graphics.strokeCS
		parsedColorSpace = e.graphics.strokeParsedCS
	}
	values := defaultColorValues(colorSpaceName, parsedColorSpace)
	if len(values) == 0 {
		return
	}
	if e.applyParsedColorSpaceForGraphicsState(values, parsedColorSpace, stroke) {
		return
	}
	e.applyColorValuesBySpaceForGraphicsState(values, colorSpaceName, stroke)
}

func defaultColorValues(colorSpaceName string, parsedColorSpace colorspace.ColorSpace) []float64 {
	if parsedColorSpace != nil {
		switch parsedColorSpace.Type() {
		case colorspace.ColorSpaceDeviceCMYK:
			return []float64{0, 0, 0, 1}
		case colorspace.ColorSpaceSeparation, colorspace.ColorSpaceDeviceN:
			return repeatedDefaultColorValue(parsedColorSpace.GetNumComponents(), 1)
		case colorspace.ColorSpacePattern:
			return []float64{0}
		default:
			return repeatedDefaultColorValue(parsedColorSpace.GetNumComponents(), 0)
		}
	}

	switch strings.TrimPrefix(colorSpaceName, "/") {
	case "DeviceCMYK":
		return []float64{0, 0, 0, 1}
	case "Separation":
		return []float64{1}
	case "DeviceN":
		return []float64{1}
	case "Pattern":
		return []float64{0}
	case "DeviceRGB", "CalRGB", "Lab":
		return []float64{0, 0, 0}
	case "Indexed", "DeviceGray", "CalGray", "ICCBased":
		return []float64{0}
	default:
		return []float64{0, 0, 0}
	}
}

func repeatedDefaultColorValue(components int, value float64) []float64 {
	if components <= 0 {
		return nil
	}
	values := make([]float64, components)
	for i := range values {
		values[i] = value
	}
	return values
}

func (e *Evaluator) setStrokeColorBySpace(op Operator) error {
	return e.setColorBySpace(op, true)
}

func (e *Evaluator) setFillColorBySpace(op Operator) error {
	return e.setColorBySpace(op, false)
}

func (e *Evaluator) setColorBySpace(op Operator, stroke bool) error {
	colorSpace := e.graphics.fillCS
	parsedColorSpace := e.graphics.fillParsedCS
	patternBaseCS := e.graphics.fillPatternBaseCS
	if stroke {
		colorSpace = e.graphics.strokeCS
		parsedColorSpace = e.graphics.strokeParsedCS
		patternBaseCS = e.graphics.strokePatternBaseCS
	}
	isPatternSpace := strings.EqualFold(colorSpace, "Pattern")
	if os.Getenv("PDF_DEBUG_PATTERN_RESOLVE") == "1" {
		values, patternName := splitColorAndPatternOperands(op.Operands)
		patternLabel := "<nil>"
		if patternName != nil {
			patternLabel = patternName.String()
		}
		fmt.Fprintf(os.Stderr, "DEBUG setColorBySpace: opcode=%s stroke=%v cs=%s patternBase=%s values=%v pattern=%s operands=%v resources=%p\n",
			op.Opcode, stroke, colorSpace, patternBaseCS, values, patternLabel, op.Operands, e.resources)
	}
	if len(op.Operands) == 0 {
		return nil
	}

	if isPatternSpace {
		if !isColorNOperator(op.Opcode) {
			return nil
		}
		e.setPatternColorBySpace(op.Operands, patternBaseCS, stroke)
		return nil
	}

	var values []float64
	if components := graphicsColorComponentCount(colorSpace, parsedColorSpace); components > 0 {
		if len(op.Operands) != components {
			return nil
		}
		if isColorNOperator(op.Opcode) {
			values = colorOperandsWithPopplerFallback(op.Operands)
		} else {
			var ok bool
			values, ok = strictColorOperands(op.Operands)
			if !ok {
				return nil
			}
		}
	} else {
		var ok bool
		values, ok = strictColorOperands(op.Operands)
		if !ok || len(values) == 0 {
			return nil
		}
	}
	if e.applyParsedColorSpaceForGraphicsState(values, parsedColorSpace, stroke) {
		e.clearPatternForGraphicsState(stroke)
		return nil
	}
	e.applyColorValuesBySpaceForGraphicsState(values, colorSpace, stroke)
	e.clearPatternForGraphicsState(stroke)
	return nil
}

func (e *Evaluator) setPatternColorBySpace(operands []entity.Object, patternBaseCS string, stroke bool) {
	numArgs := len(operands)
	if numArgs == 0 {
		return
	}

	if numArgs > 1 {
		components := colorComponentCountBySpace(patternBaseCS)
		if components <= 0 || numArgs-1 != components {
			return
		}
		e.applyColorValuesBySpaceForGraphicsState(colorOperandsWithPopplerFallback(operands[:numArgs-1]), patternBaseCS, stroke)
	}

	patternName, ok := operands[numArgs-1].(entity.Name)
	if !ok {
		return
	}
	pattern, err := e.resolvePattern(patternName)
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
	if err != nil || pattern == nil {
		return
	}
	if stroke {
		e.graphics.strokePattern = pattern
		return
	}
	e.graphics.fillPattern = pattern
}

func colorOperandsWithPopplerFallback(operands []entity.Object) []float64 {
	values := make([]float64, 0, len(operands))
	for _, obj := range operands {
		v, err := getNumberOperand(obj)
		if err != nil {
			v = 0
		}
		values = append(values, clamp(v, 0, 1))
	}
	return values
}

func strictColorOperands(operands []entity.Object) ([]float64, bool) {
	values := make([]float64, 0, len(operands))
	for _, obj := range operands {
		v, err := getNumberOperand(obj)
		if err != nil {
			return nil, false
		}
		values = append(values, clamp(v, 0, 1))
	}
	return values, true
}

func isColorNOperator(opcode string) bool {
	return opcode == "scn" || opcode == "SCN"
}

func trailingOperands(operands []entity.Object, n int) ([]entity.Object, bool) {
	if n <= 0 || len(operands) < n {
		return nil, false
	}
	return operands[len(operands)-n:], true
}

func colorComponentCountBySpace(colorSpace string) int {
	switch strings.TrimPrefix(colorSpace, "/") {
	case "DeviceGray", "CalGray", "Indexed", "ICCBased", "Separation":
		return 1
	case "DeviceRGB", "CalRGB", "Lab":
		return 3
	case "DeviceCMYK":
		return 4
	case "DeviceN":
		return 1
	default:
		return 0
	}
}

func graphicsColorComponentCount(colorSpace string, parsed colorspace.ColorSpace) int {
	if parsed != nil {
		return parsed.GetNumComponents()
	}
	return colorComponentCountBySpace(colorSpace)
}

func (e *Evaluator) clearPatternForGraphicsState(stroke bool) {
	if stroke {
		e.graphics.strokePattern = nil
		e.graphics.strokePatternBaseCS = ""
		return
	}
	e.graphics.fillPattern = nil
	e.graphics.fillPatternBaseCS = ""
}

func (e *Evaluator) applyParsedColorSpaceForGraphicsState(
	values []float64,
	colorSpace colorspace.ColorSpace,
	stroke bool,
) bool {
	if colorSpace == nil {
		return false
	}
	if components := colorSpace.GetNumComponents(); components > 0 && len(values) != components {
		return true
	}
	rgba := colorSpace.ConvertToRGBA(values)
	hex := fmt.Sprintf("%02X%02X%02X", rgba.R, rgba.G, rgba.B)
	if stroke {
		e.graphics.strokeColor = &ColorSpace{Color: &Color{Hex: hex}}
		return true
	}
	e.graphics.fillColor = &ColorSpace{Color: &Color{Hex: hex}}
	return true
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
	if !e.hasResourceFrames() {
		return nil, fmt.Errorf("no resources for pattern %s", name)
	}

	patternObj, patternFrames := e.getResourceEntryWithFrames(pdfNamePattern, name)
	if patternObj == nil {
		return nil, fmt.Errorf("pattern %s not found", name)
	}
	if len(patternFrames) > 0 {
		defer e.useResourceStack(patternFrames)()
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
	if v := patternDict.Get(pdfNamePatternType); v != nil {
		parsed, err := objectIntStrict(v)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern type: %w", err)
		}
		patternType = parsed
	}

	matrix, ok := parseMatrix(patternDict.Get(pdfNameMatrix))
	if !ok {
		matrix = [6]float64{1, 0, 0, 1, 0, 0}
	}

	switch patternType {
	case 1:
		paintType := 1
		tilingType := entity.TilingConstantSpacing

		if v := patternDict.Get(pdfNamePaintType); v != nil {
			if parsed, err := objectIntStrict(v); err == nil {
				paintType = parsed
			}
		}
		if v := patternDict.Get(pdfNameTilingType); v != nil {
			if parsed, err := objectIntStrict(v); err == nil {
				tilingType = entity.TilingType(parsed)
			}
		}

		pattern := entity.NewTilingPattern(name.String(), paintType, tilingType)
		pattern.SetXRef(e.xref)
		pattern.SetMatrix(matrix)

		if bboxObj := patternDict.Get(pdfNameBBox); bboxObj != nil {
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
		if xStep := patternDict.Get(pdfNameXStep); xStep != nil {
			if value, err := getNumberOperand(xStep); err == nil {
				pattern.SetXStep(value)
			}
		}
		if yStep := patternDict.Get(pdfNameYStep); yStep != nil {
			if value, err := getNumberOperand(yStep); err == nil {
				pattern.SetYStep(value)
			}
		}
		if resourcesObj := patternDict.Get(pdfNameResources); resourcesObj != nil {
			if resources, ok := resourcesObj.(*entity.Dict); ok {
				pattern.SetResources(resources)
			}
		}
		pattern.SetContent(streamContent)
		return pattern, nil

	case 2:
		shadingObj := patternDict.Get(pdfNameShading)
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
	operands, ok := trailingOperands(op.Operands, 1)
	if !ok {
		return fmt.Errorf("operator G requires 1 operand")
	}

	gray, err := getNumberOperand(operands[0])
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
	e.graphics.strokeParsedCS = nil
	e.graphics.strokePattern = nil
	e.graphics.strokePatternBaseCS = ""

	return nil
}

// setGrayFill sets the gray color for filling operations - 'g' operator.
func (e *Evaluator) setGrayFill(op Operator) error {
	operands, ok := trailingOperands(op.Operands, 1)
	if !ok {
		return fmt.Errorf("g operator requires 1 operand")
	}

	gray, err := getNumberOperand(operands[0])
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
	e.graphics.fillParsedCS = nil
	e.graphics.fillPattern = nil
	e.graphics.fillPatternBaseCS = ""

	return nil
}

// setRGBStroke sets the RGB color for stroking operations - 'RG' operator.
func (e *Evaluator) setRGBStroke(op Operator) error {
	operands, ok := trailingOperands(op.Operands, 3)
	if !ok {
		return fmt.Errorf("RG operator requires 3 operands")
	}

	r, err := getNumberOperand(operands[0])
	if err != nil {
		return fmt.Errorf("RG operator: invalid red value: %w", err)
	}

	g, err := getNumberOperand(operands[1])
	if err != nil {
		return fmt.Errorf("RG operator: invalid green value: %w", err)
	}

	b, err := getNumberOperand(operands[2])
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
	e.graphics.strokeParsedCS = nil
	e.graphics.strokePattern = nil
	e.graphics.strokePatternBaseCS = ""

	return nil
}

// setRGBFill sets the RGB color for filling operations - 'rg' operator.
func (e *Evaluator) setRGBFill(op Operator) error {
	operands, ok := trailingOperands(op.Operands, 3)
	if !ok {
		return fmt.Errorf("rg operator requires 3 operands")
	}

	r, err := getNumberOperand(operands[0])
	if err != nil {
		return fmt.Errorf("rg operator: invalid red value: %w", err)
	}

	g, err := getNumberOperand(operands[1])
	if err != nil {
		return fmt.Errorf("rg operator: invalid green value: %w", err)
	}

	b, err := getNumberOperand(operands[2])
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
	e.graphics.fillParsedCS = nil
	e.graphics.fillPattern = nil
	e.graphics.fillPatternBaseCS = ""

	return nil
}

// setCMYKStroke sets the CMYK color for stroking operations - 'K' operator.
func (e *Evaluator) setCMYKStroke(op Operator) error {
	operands, ok := trailingOperands(op.Operands, 4)
	if !ok {
		return fmt.Errorf("operator K requires 4 operands")
	}

	c, err := getNumberOperand(operands[0])
	if err != nil {
		return fmt.Errorf("operator K: invalid cyan value: %w", err)
	}

	m, err := getNumberOperand(operands[1])
	if err != nil {
		return fmt.Errorf("operator K: invalid magenta value: %w", err)
	}

	y, err := getNumberOperand(operands[2])
	if err != nil {
		return fmt.Errorf("operator K: invalid yellow value: %w", err)
	}

	k, err := getNumberOperand(operands[3])
	if err != nil {
		return fmt.Errorf("operator K: invalid black value: %w", err)
	}

	// Convert CMYK to RGB
	r, g, b := cmykToRGB(c, m, y, k)

	// Convert RGB to hex string
	hex := grayToHex(r, g, b)
	e.graphics.strokeColor = &ColorSpace{Color: &Color{Hex: hex}}
	e.graphics.strokeCS = "DeviceCMYK"
	e.graphics.strokeParsedCS = nil
	e.graphics.strokePattern = nil
	e.graphics.strokePatternBaseCS = ""

	return nil
}

// setCMYKFill sets the CMYK color for filling operations - 'k' operator.
func (e *Evaluator) setCMYKFill(op Operator) error {
	operands, ok := trailingOperands(op.Operands, 4)
	if !ok {
		return fmt.Errorf("k operator requires 4 operands")
	}

	c, err := getNumberOperand(operands[0])
	if err != nil {
		return fmt.Errorf("k operator: invalid cyan value: %w", err)
	}

	m, err := getNumberOperand(operands[1])
	if err != nil {
		return fmt.Errorf("k operator: invalid magenta value: %w", err)
	}

	y, err := getNumberOperand(operands[2])
	if err != nil {
		return fmt.Errorf("k operator: invalid yellow value: %w", err)
	}

	k, err := getNumberOperand(operands[3])
	if err != nil {
		return fmt.Errorf("k operator: invalid black value: %w", err)
	}

	// Convert CMYK to RGB
	r, g, b := cmykToRGB(c, m, y, k)

	// Convert RGB to hex string
	hex := grayToHex(r, g, b)
	e.graphics.fillColor = &ColorSpace{Color: &Color{Hex: hex}}
	e.graphics.fillCS = "DeviceCMYK"
	e.graphics.fillParsedCS = nil
	e.graphics.fillPattern = nil
	e.graphics.fillPatternBaseCS = ""

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

	gsObj := e.getResourceEntry(pdfNameExtGState, gsName)
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

	if lw := gsDict.Get(pdfNameLW); lw != nil {
		_ = e.setLineWidth(Operator{Opcode: "w", Operands: []entity.Object{lw}})
	}
	if lc := gsDict.Get(pdfNameLC); lc != nil {
		_ = e.setLineCap(Operator{Opcode: "J", Operands: []entity.Object{lc}})
	}
	if lj := gsDict.Get(pdfNameLJ); lj != nil {
		_ = e.setLineJoin(Operator{Opcode: "j", Operands: []entity.Object{lj}})
	}
	if ml := gsDict.Get(pdfNameML); ml != nil {
		_ = e.setMiterLimit(Operator{Opcode: "M", Operands: []entity.Object{ml}})
	}
	if dash := gsDict.Get(pdfNameD); dash != nil {
		if dashArray, ok := dash.(*entity.Array); ok && dashArray.Len() >= 2 {
			_ = e.setDashPattern(Operator{
				Opcode:   "d",
				Operands: []entity.Object{dashArray.Get(0), dashArray.Get(1)},
			})
		}
	}

	if strokeAlpha := gsDict.Get(pdfNameCA); strokeAlpha != nil {
		if value, err := getNumberOperand(strokeAlpha); err == nil {
			e.graphics.strokeAlpha = clamp(value, 0, 1)
			e.syncCanvasStrokeAlpha()
		}
	}
	if fillAlpha := gsDict.Get(pdfNameca); fillAlpha != nil {
		if value, err := getNumberOperand(fillAlpha); err == nil {
			e.graphics.fillAlpha = clamp(value, 0, 1)
			e.syncCanvasFillAlpha()
		}
	}
	if blendMode := gsDict.Get(pdfNameBM); blendMode != nil {
		e.applyBlendModeObject(blendMode)
	}
	if alphaIsShape := gsDict.Get(pdfNameAIS); alphaIsShape != nil {
		if value, ok := alphaIsShape.(*entity.Boolean); ok {
			e.graphics.alphaIsShape = value.Value()
		}
	}
	if transfer := gsDict.Get(pdfNameTR2); transfer != nil {
		e.applyTransferObject(transfer)
	} else if transfer := gsDict.Get(pdfNameTR); transfer != nil {
		e.applyTransferObject(transfer)
	}
	if strokeAdjust := gsDict.Get(pdfNameSA); strokeAdjust != nil {
		if value, ok := strokeAdjust.(*entity.Boolean); ok {
			e.graphics.strokeAdjust = value.Value()
			e.syncCanvasStrokeAdjust()
		}
	}
	if softMask := gsDict.Get(pdfNameSMask); softMask != nil && enableExtGStateSoftMask() {
		if err := e.applyExtGStateSoftMask(softMask); err != nil {
			return err
		}
	}

	if os.Getenv("PDF_DEBUG_EVAL_GS") != "" {
		fmt.Fprintf(os.Stderr, "GSAPPLY name=%v fillAlpha=%.4f blendMode=%v\n",
			gsName, e.graphics.fillAlpha, e.graphics.blendMode)
	}

	return nil
}

type blendModeSetter interface {
	SetBlendMode(name string)
}

type fillAlphaSetter interface {
	SetFillAlpha(alpha float64)
}

type strokeAlphaSetter interface {
	SetStrokeAlpha(alpha float64)
}

type strokeAdjustSetter interface {
	SetStrokeAdjust(adj bool)
}

type softMaskGroupCanvas interface {
	ClearSoftMask()
	BeginSoftMaskGroup(bbox [4]float64, isolated, knockout bool) error
	EndSoftMaskGroup(alpha bool) error
	InstallPendingSoftMask() error
}

type softMaskGroupDeviceBBoxCanvas interface {
	BeginSoftMaskGroupDeviceBBox(bbox [4]float64, isolated, knockout bool) error
}

type softMaskGroupCroppedDeviceBBoxCanvas interface {
	BeginSoftMaskGroupCroppedDeviceBBox(bbox [4]float64, isolated, knockout bool) (int, int, error)
}

// SoftMaskOptions carries Poppler soft-mask parameters from the evaluator to a
// rendering backend.
type SoftMaskOptions struct {
	Alpha          bool
	HasBackdrop    bool
	BackdropRGB    [3]uint8
	BackdropLum    uint8
	Transfer       [256]uint8
	TransferActive bool
}

type softMaskGroupCanvasWithOptions interface {
	EndSoftMaskGroupWithOptions(options SoftMaskOptions) error
}

type softMaskPresenceCanvas interface {
	HasSoftMask() bool
}

// nonIsolatedGroupCanvas reports whether the current Splash state is rendering
// inside a non-isolated transparency group (Poppler's setInNonIsolatedGroup).
type nonIsolatedGroupCanvas interface {
	InNonIsolatedGroup() bool
}

type transparencyGroupCanvas interface {
	BeginTransparencyGroup(bbox [4]float64, isolated, knockout bool) error
	PaintTransparencyGroup() error
	DiscardTransparencyGroup() error
}

type transparencyGroupDeviceBBoxCanvas interface {
	BeginTransparencyGroupDeviceBBox(bbox [4]float64, isolated, knockout bool) error
}

type transparencyGroupCroppedDeviceBBoxCanvas interface {
	BeginTransparencyGroupCroppedDeviceBBox(bbox [4]float64, isolated, knockout bool) (int, int, error)
}

func enableExtGStateSoftMask() bool {
	if isEnvTrue(os.Getenv("GO_PDF_DISABLE_EXTGSTATE_SMASK")) {
		return false
	}
	if isEnvFalse(os.Getenv("GO_PDF_ENABLE_EXTGSTATE_SMASK")) {
		return false
	}
	return true
}

func isEnvTrue(value string) bool {
	value = strings.TrimSpace(value)
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

func isEnvFalse(value string) bool {
	value = strings.TrimSpace(value)
	return value == "0" || strings.EqualFold(value, "false") || strings.EqualFold(value, "no")
}

func enableExtGStateSoftMaskCroppedGroup() bool {
	return os.Getenv("GO_PDF_EXTGSTATE_SMASK_CROPPED_GROUP") == "1"
}

func enableFormTransparencyGroup() bool {
	v := strings.TrimSpace(os.Getenv("GO_PDF_ENABLE_FORM_TRANSPARENCY_GROUP"))
	return v != "0" && !strings.EqualFold(v, "false")
}

func enableFormTransparencyGroupCropped() bool {
	return !isEnvFalse(os.Getenv("GO_PDF_FORM_GROUP_CROPPED"))
}

func (e *Evaluator) syncCanvasFillAlpha() {
	if e.canvas == nil {
		return
	}
	if setter, ok := e.canvas.(fillAlphaSetter); ok {
		setter.SetFillAlpha(e.graphics.fillAlpha)
	}
}

func (e *Evaluator) syncCanvasStrokeAlpha() {
	if e.canvas == nil {
		return
	}
	if setter, ok := e.canvas.(strokeAlphaSetter); ok {
		setter.SetStrokeAlpha(e.graphics.strokeAlpha)
	}
}

func (e *Evaluator) syncCanvasStrokeAdjust() {
	if e.canvas == nil {
		return
	}
	if setter, ok := e.canvas.(strokeAdjustSetter); ok {
		setter.SetStrokeAdjust(e.graphics.strokeAdjust)
	}
}

// canvasYDownBase reports whether the canvas consumes y-down composed CTMs.
func (e *Evaluator) canvasYDownBase() bool {
	if c, ok := e.canvas.(interface{ YDownBase() bool }); ok {
		return c.YDownBase()
	}
	return false
}

func (e *Evaluator) applyExtGStateSoftMask(obj entity.Object) error {
	controller, ok := e.canvas.(softMaskGroupCanvas)
	if !ok {
		return nil
	}
	resolved := e.resolveResourceEntryObject(obj, 0)
	if name, ok := resolved.(entity.Name); ok {
		if strings.EqualFold(name.Value(), "None") {
			controller.ClearSoftMask()
		}
		return nil
	}
	maskDict, ok := resolved.(*entity.Dict)
	if !ok {
		return nil
	}
	return e.evaluateExtGStateSoftMask(maskDict, controller)
}

func (e *Evaluator) evaluateExtGStateSoftMask(maskDict *entity.Dict, controller softMaskGroupCanvas) error {
	if maskDict == nil || controller == nil {
		return nil
	}
	if os.Getenv("PDF_DEBUG_EVAL_GS") != "" {
		subtype := "(none)"
		if s, ok := e.resolveResourceEntryObject(maskDict.Get(pdfNameS), 0).(entity.Name); ok {
			subtype = s.Value()
		}
		fmt.Fprintf(os.Stderr, "SMASKENTER subtype=%s bbox-pending fillAlpha=%.4f\n", subtype, e.graphics.fillAlpha)
	}
	groupStream, ok := e.resolveStreamObject(maskDict.Get(pdfNameG))
	if !ok || groupStream == nil || groupStream.Dict() == nil {
		return nil
	}
	groupDict, ok := e.resolveDictObject(groupStream.Dict().Get(pdfNameGroup))
	if !ok || !isTransparencyGroupDict(groupDict) {
		return nil
	}
	bboxArr, ok := e.resolveArrayObject(groupStream.Dict().Get(pdfNameBBox))
	if !ok || bboxArr.Len() != 4 {
		return nil
	}
	bbox, ok := numericArray4(bboxArr)
	if !ok {
		return nil
	}
	content, err := e.formOperatorContentForExecution(groupStream)
	if err != nil {
		return err
	}

	if err := e.saveState(); err != nil {
		return err
	}
	e.preserveNonEmptyCallerPathOnSavedState()
	restore := true
	defer func() {
		if restore {
			_ = e.restoreState()
		}
	}()

	e.clearCurrentPathForForm()
	if matrixArr, ok := e.resolveArrayObject(groupStream.Dict().Get(pdfNameMatrix)); ok && matrixArr.Len() == 6 {
		e.concatenateMatrixToCTM(numericMatrix6(matrixArr))
	}
	e.graphics.baseTransform = e.graphics.transform

	var maskResources *entity.Dict
	if resourcesDict, ok := e.resolveFormResources(groupStream.Dict().Get(pdfNameResources)); ok {
		maskResources = resourcesDict
	}
	defer e.pushResources(maskResources)()

	if err := e.applyFormBBoxClip(bboxArr); err != nil {
		return errors.Invalid("soft_mask_form_bbox_clip", err)
	}
	e.clearCurrentPathForForm()

	if setter, ok := e.canvas.(blendModeSetter); ok {
		setter.SetBlendMode("Normal")
	}
	e.graphics.blendMode = "Normal"
	e.graphics.fillAlpha = 1
	e.graphics.strokeAlpha = 1
	e.syncCanvasFillAlpha()
	e.syncCanvasStrokeAlpha()
	controller.ClearSoftMask()

	isolated := boolDictValue(groupDict, entity.Name("I"), false)
	knockout := boolDictValue(groupDict, entity.Name("K"), false)
	x0, y0, x1, y1 := transformedBBoxBounds(e.graphics.transform, bbox)
	deviceBBox := [4]float64{x0, y0, x1, y1}
	if croppedController, ok := controller.(softMaskGroupCroppedDeviceBBoxCanvas); ok && enableExtGStateSoftMaskCroppedGroup() {
		tx, ty, err := croppedController.BeginSoftMaskGroupCroppedDeviceBBox(deviceBBox, isolated, knockout)
		if err != nil {
			return err
		}
		e.graphics.transform[4] -= float64(tx)
		if e.canvasYDownBase() {
			e.graphics.transform[5] -= float64(ty)
		} else {
			e.graphics.transform[5] += float64(ty)
		}
		e.graphics.baseTransform = e.graphics.transform
	} else if deviceController, ok := controller.(softMaskGroupDeviceBBoxCanvas); ok {
		if err := deviceController.BeginSoftMaskGroupDeviceBBox(deviceBBox, isolated, knockout); err != nil {
			return err
		}
	} else {
		if err := controller.BeginSoftMaskGroup(bbox, isolated, knockout); err != nil {
			return err
		}
	}
	alpha := false
	if subtype, ok := e.resolveResourceEntryObject(maskDict.Get(pdfNameS), 0).(entity.Name); ok {
		alpha = strings.EqualFold(subtype.Value(), "Alpha")
	}
	options := e.resolveSoftMaskOptions(maskDict, groupDict, alpha)
	if os.Getenv("PDF_DEBUG_EVAL_GS") != "" {
		fmt.Fprintf(os.Stderr, "SMASKEXEC alpha=%t isolated=%t knockout=%t bbox=%.1f,%.1f,%.1f,%.1f\n", alpha, isolated, knockout, bbox[0], bbox[1], bbox[2], bbox[3])
	}
	if err := e.executeFormOperatorContent(content); err != nil {
		return errors.Invalid("soft_mask_form", err)
	}
	if os.Getenv("PDF_DEBUG_EVAL_GS") != "" {
		fmt.Fprintf(os.Stderr, "SMASKDONE\n")
	}
	if advanced, ok := controller.(softMaskGroupCanvasWithOptions); ok {
		if err := advanced.EndSoftMaskGroupWithOptions(options); err != nil {
			return err
		}
	} else if err := controller.EndSoftMaskGroup(alpha); err != nil {
		return err
	}
	if err := e.restoreState(); err != nil {
		restore = false
		return err
	}
	restore = false
	return controller.InstallPendingSoftMask()
}

func (e *Evaluator) resolveSoftMaskOptions(maskDict, groupDict *entity.Dict, alpha bool) SoftMaskOptions {
	options := SoftMaskOptions{Alpha: alpha}
	if maskDict == nil {
		return options
	}
	if transfer := maskDict.Get(pdfNameTR); transfer != nil {
		if fn, err := e.parseTransferFunction(transfer); err == nil {
			for i := 0; i < 256; i++ {
				options.Transfer[i] = evaluateTransferByte(fn, float64(i)/255.0)
			}
			options.TransferActive = true
		}
	}

	colorSpace, ok := e.resolveSoftMaskBlendingColorSpace(groupDict)
	if !ok || colorSpace == nil {
		return options
	}
	values := defaultColorValues(colorSpace.Type().String(), colorSpace)
	if bc, ok := e.resolveArrayObject(maskDict.Get(pdfNameBC)); ok {
		values = numericColorOperandsFromArray(bc)
	}
	rgba := colorSpace.ConvertToRGBA(values)
	options.HasBackdrop = true
	options.BackdropRGB = [3]uint8{rgba.R, rgba.G, rgba.B}
	options.BackdropLum = softMaskBackdropLuminosity(rgba.R, rgba.G, rgba.B)
	return options
}

func (e *Evaluator) resolveSoftMaskBlendingColorSpace(groupDict *entity.Dict) (colorspace.ColorSpace, bool) {
	if groupDict == nil {
		return nil, false
	}
	csObj := groupDict.Get(pdfNameCS)
	if csObj == nil {
		return nil, false
	}
	if parsed, ok := e.resolveTypedGraphicsColorSpace(csObj); ok {
		return parsed, true
	}
	name, ok := e.resolveImageColorSpace(csObj)
	if !ok {
		return nil, false
	}
	switch name {
	case "DeviceGray":
		return colorspace.NewDeviceGray(), true
	case "DeviceRGB":
		return colorspace.NewDeviceRGB(), true
	case "DeviceCMYK":
		return colorspace.NewDeviceCMYK(), true
	default:
		return nil, false
	}
}

func numericColorOperandsFromArray(arr *entity.Array) []float64 {
	if arr == nil {
		return nil
	}
	values := make([]float64, 0, arr.Len())
	for i := 0; i < arr.Len(); i++ {
		value, err := getNumberOperand(arr.Get(i))
		if err != nil {
			continue
		}
		values = append(values, clamp(value, 0, 1))
	}
	return values
}

func softMaskBackdropLuminosity(r, g, b uint8) uint8 {
	return uint8((30*int(r) + 59*int(g) + 11*int(b) + 50) / 100)
}

func (e *Evaluator) resolveDictObject(obj entity.Object) (*entity.Dict, bool) {
	resolved := e.resolveResourceEntryObject(obj, 0)
	if streamObj, ok := resolved.(*entity.Stream); ok {
		resolved = streamObj.Dict()
	}
	dict, ok := resolved.(*entity.Dict)
	return dict, ok && dict != nil
}

func (e *Evaluator) resolveArrayObject(obj entity.Object) (*entity.Array, bool) {
	resolved := e.resolveResourceEntryObject(obj, 0)
	arr, ok := resolved.(*entity.Array)
	return arr, ok && arr != nil
}

func (e *Evaluator) resolveFormResources(obj entity.Object) (*entity.Dict, bool) {
	return e.resolveDictObject(obj)
}

func isTransparencyGroupDict(dict *entity.Dict) bool {
	if dict == nil {
		return false
	}
	if subtype, ok := dict.Get(pdfNameS).(entity.Name); ok {
		return strings.EqualFold(subtype.Value(), "Transparency")
	}
	return false
}

func (e *Evaluator) formTransparencyGroupRequired(isolated, knockout bool, resources *entity.Dict) bool {
	if isolated || e.currentStateNeedsTransparencyGroup(knockout) {
		return true
	}
	return e.resourcesNeedTransparencyGroup(resources)
}

func (e *Evaluator) currentStateNeedsTransparencyGroup(knockout bool) bool {
	if knockout || e.graphics == nil {
		return knockout
	}
	if e.graphics.fillAlpha != 1 || e.graphics.strokeAlpha != 1 || e.graphics.alphaIsShape {
		return true
	}
	if normalizeBlendModeName(e.graphics.blendMode) != "Normal" {
		return true
	}
	if canvas, ok := e.canvas.(softMaskPresenceCanvas); ok && canvas.HasSoftMask() {
		return true
	}
	return false
}

func (e *Evaluator) resourcesNeedTransparencyGroup(resources *entity.Dict) bool {
	if resources == nil {
		return false
	}
	extGStateObj := e.resolveResourceEntryObject(resources.Get(pdfNameExtGState), 0)
	if streamObj, ok := extGStateObj.(*entity.Stream); ok {
		extGStateObj = streamObj.Dict()
	}
	extGStates, ok := extGStateObj.(*entity.Dict)
	if !ok || extGStates == nil {
		return false
	}
	for _, key := range extGStates.Keys() {
		gsObj := e.resolveResourceEntryObject(extGStates.Get(key), 0)
		if streamObj, ok := gsObj.(*entity.Stream); ok {
			gsObj = streamObj.Dict()
		}
		if gsDict, ok := gsObj.(*entity.Dict); ok && e.extGStateNeedsTransparencyGroup(gsDict) {
			return true
		}
	}
	return false
}

func (e *Evaluator) extGStateNeedsTransparencyGroup(gsDict *entity.Dict) bool {
	if gsDict == nil {
		return false
	}
	if blendMode := gsDict.Get(pdfNameBM); blendMode != nil {
		if name, ok := e.firstSupportedBlendModeName(blendMode, 0); ok && normalizeBlendModeName(name) != "Normal" {
			return true
		}
	}
	for _, key := range []entity.Name{entity.Name("ca"), entity.Name("CA")} {
		if alphaObj := gsDict.Get(key); alphaObj != nil {
			if value, err := getNumberOperand(alphaObj); err == nil && clamp(value, 0, 1) != 1 {
				return true
			}
		}
	}
	if alphaIsShape, ok := gsDict.Get(pdfNameAIS).(*entity.Boolean); ok && alphaIsShape.Value() {
		return true
	}
	if softMask := gsDict.Get(pdfNameSMask); softMask != nil {
		if name, ok := softMask.(entity.Name); !ok || !strings.EqualFold(name.Value(), "None") {
			return true
		}
	}
	return false
}

func boolDictValue(dict *entity.Dict, key entity.Name, fallback bool) bool {
	if dict == nil {
		return fallback
	}
	if value, ok := dict.Get(key).(*entity.Boolean); ok {
		return value.Value()
	}
	return fallback
}

func numericArray4(arr *entity.Array) ([4]float64, bool) {
	var out [4]float64
	if arr == nil || arr.Len() != 4 {
		return out, false
	}
	for i := 0; i < 4; i++ {
		value, err := getNumberOperand(arr.Get(i))
		if err != nil {
			return out, false
		}
		out[i] = value
	}
	return out, true
}

func numericMatrix6(arr *entity.Array) [6]float64 {
	matrix := [6]float64{1, 0, 0, 1, 0, 0}
	if arr == nil || arr.Len() != 6 {
		return matrix
	}
	for i := 0; i < 6; i++ {
		value, err := getNumberOperand(arr.Get(i))
		if err != nil {
			matrix[i] = 0
			continue
		}
		matrix[i] = value
	}
	return matrix
}

func (e *Evaluator) applyBlendModeObject(obj entity.Object) {
	name, ok := e.firstSupportedBlendModeName(obj, 0)
	if !ok {
		name = "Normal"
	}
	e.graphics.blendMode = name
	if setter, ok := e.canvas.(blendModeSetter); ok {
		setter.SetBlendMode(name)
	}
}

func (e *Evaluator) firstSupportedBlendModeName(obj entity.Object, depth int) (string, bool) {
	if obj == nil || depth > 8 {
		return "", false
	}
	switch v := obj.(type) {
	case entity.Name:
		return supportedBlendModeName(v.Value())
	case entity.Ref:
		if e.xref == nil {
			return "", false
		}
		resolved, err := e.xref.Fetch(v)
		if err != nil {
			return "", false
		}
		return e.firstSupportedBlendModeName(resolved, depth+1)
	case *entity.Array:
		for i := 0; i < v.Len(); i++ {
			if name, ok := e.firstSupportedBlendModeName(v.Get(i), depth+1); ok {
				return name, true
			}
		}
	}
	return "", false
}

func supportedBlendModeName(name string) (string, bool) {
	normalized := normalizeBlendModeName(name)
	switch normalized {
	case "Compatible":
		return "Normal", true
	case "Normal", "Multiply", "Screen", "Overlay", "Darken", "Lighten",
		"ColorDodge", "ColorBurn", "HardLight", "SoftLight", "Difference", "Exclusion",
		"Hue", "Saturation", "Color", "Luminosity":
		return normalized, true
	default:
		return "", false
	}
}

func normalizeBlendModeName(name string) string {
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	if name == "" {
		return ""
	}
	lower := strings.ToLower(name)
	switch lower {
	case "colordodge":
		return "ColorDodge"
	case "colorburn":
		return "ColorBurn"
	case "hardlight":
		return "HardLight"
	case "softlight":
		return "SoftLight"
	default:
		return strings.ToUpper(lower[:1]) + lower[1:]
	}
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
	e.graphics.path.MoveTo(tx, ty)
	if rawPath := e.currentRawPath(); rawPath != nil {
		rawPath.MoveTo(x, y)
	}

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
	e.graphics.path.LineTo(tx, ty)
	if rawPath := e.currentRawPath(); rawPath != nil {
		rawPath.LineTo(x, y)
	}

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
	e.graphics.path.CurveTo(tx1, ty1, tx2, ty2, tx, ty)
	if rawPath := e.currentRawPath(); rawPath != nil {
		rawPath.CurveTo(x1, y1, x2, y2, x, y)
	}

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
	rawCX, rawCY := 0.0, 0.0
	if rawPath := e.currentRawPath(); rawPath != nil {
		rawCX, rawCY = rawPath.CurrentPoint()
	}

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
	e.graphics.path.CurveTo(cx, cy, tx2, ty2, tx, ty)
	if rawPath := e.currentRawPath(); rawPath != nil {
		rawPath.CurveTo(rawCX, rawCY, x2, y2, x, y)
	}

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
	e.graphics.path.CurveTo(tx1, ty1, tx, ty, tx, ty)
	if rawPath := e.currentRawPath(); rawPath != nil {
		rawPath.CurveTo(x1, y1, x, y, x, y)
	}

	return nil
}

// closePath closes the current subpath - 'h' operator.
func (e *Evaluator) closePath(op Operator) error {
	e.graphics.path.ClosePath()
	if rawPath := e.currentRawPath(); rawPath != nil {
		rawPath.ClosePath()
	}
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
	if rawPath := e.currentRawPath(); rawPath != nil {
		rawPath.AddRect(x, y, xw, y, xw, yh, x, yh)
	}

	return nil
}

// Path Painting Operators

// strokePath strokes the current path - 'S' operator.
func (e *Evaluator) strokePath() error {
	if e.graphics.path.IsEmpty() {
		return nil
	}

	// If canvas is set, render to it
	if e.canvas != nil && e.shouldPaintStrokePath() {
		e.renderPathToCanvas(false)
	}

	e.applyPendingClipAtPathEnd()

	// Clear the path after rendering
	e.clearCurrentPath()

	return nil
}

// strokeAndClosePath closes and strokes the current path - 's' operator.
func (e *Evaluator) strokeAndClosePath() error {
	// Close the path first
	e.graphics.path.ClosePath()
	if rawPath := e.currentRawPath(); rawPath != nil {
		rawPath.ClosePath()
	}

	return e.strokePath()
}

// fillPath fills the current path using nonzero winding rule - 'f' operator.
func (e *Evaluator) fillPath() error {
	if e.graphics.path.IsEmpty() {
		return nil
	}

	// If canvas is set, render to it
	if e.canvas != nil && e.shouldPaintFillPath() {
		e.renderPathToCanvas(true)
	}

	e.applyPendingClipAtPathEnd()

	// Clear the path after rendering
	e.clearCurrentPath()

	return nil
}

// fillPathEvenOdd fills the current path using even-odd rule - 'f*' operator.
func (e *Evaluator) fillPathEvenOdd() error {
	if e.graphics.path.IsEmpty() {
		return nil
	}

	// If canvas is set, render to it using even-odd rule
	if e.canvas != nil && e.shouldPaintFillPath() {
		e.renderPathToCanvasEvenOdd()
	}

	e.applyPendingClipAtPathEnd()

	// Clear the path after rendering
	e.clearCurrentPath()

	return nil
}

// fillAndStrokePath fills and strokes the current path - 'B' operator.
func (e *Evaluator) fillAndStrokePath() error {
	if e.graphics.path.IsEmpty() {
		return nil
	}

	if e.canvas != nil {
		paintFill := e.shouldPaintFillPath()
		paintStroke := e.shouldPaintStrokePath()
		e.syncCanvasColors()
		e.replayPathToCanvas()
		if paintFill && paintStroke {
			if e.shouldUsePopplerOrderFillStrokePath() {
				e.canvas.Fill()
				if !e.tryRenderStrokePathInPopplerOrder() {
					e.replayPathToCanvas()
					e.canvas.Stroke()
				}
			} else if fillStrokeCanvas, ok := e.canvas.(interface{ FillAndStroke() }); ok {
				fillStrokeCanvas.FillAndStroke()
			} else {
				e.canvas.Fill()
				e.replayPathToCanvas()
				e.canvas.Stroke()
			}
		} else if paintFill {
			e.canvas.Fill()
		} else if paintStroke {
			if !(e.shouldUsePopplerOrderStrokePath() && e.tryRenderStrokePathInPopplerOrder()) {
				e.replayPathToCanvas()
				e.canvas.Stroke()
			}
		}
	}

	e.applyPendingClipAtPathEnd()

	e.clearCurrentPath()
	return nil
}

// fillAndStrokePathEvenOdd fills and strokes using even-odd rule - 'B*' operator.
func (e *Evaluator) fillAndStrokePathEvenOdd() error {
	if e.graphics.path.IsEmpty() {
		return nil
	}

	if e.canvas != nil {
		paintFill := e.shouldPaintFillPath()
		paintStroke := e.shouldPaintStrokePath()
		e.syncCanvasColors()
		e.replayPathToCanvas()
		if paintFill && paintStroke {
			if e.shouldUsePopplerOrderFillStrokePath() {
				if evenOddCanvas, ok := e.canvas.(interface{ FillEvenOdd() }); ok {
					evenOddCanvas.FillEvenOdd()
				} else {
					e.canvas.Fill()
				}
				if !e.tryRenderStrokePathInPopplerOrder() {
					e.replayPathToCanvas()
					e.canvas.Stroke()
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
		} else if paintFill {
			if evenOddCanvas, ok := e.canvas.(interface{ FillEvenOdd() }); ok {
				evenOddCanvas.FillEvenOdd()
			} else {
				e.canvas.Fill()
			}
		} else if paintStroke {
			if !(e.shouldUsePopplerOrderStrokePath() && e.tryRenderStrokePathInPopplerOrder()) {
				e.replayPathToCanvas()
				e.canvas.Stroke()
			}
		}
	}

	e.applyPendingClipAtPathEnd()

	e.clearCurrentPath()
	return nil
}

func (e *Evaluator) shouldPaintFillPath() bool {
	if shouldSkipFillPathsForDebug() {
		return false
	}
	return !strings.EqualFold(e.graphics.fillCS, "Pattern") || e.graphics.fillPattern != nil
}

func (e *Evaluator) shouldPaintStrokePath() bool {
	if shouldSkipStrokePathsForDebug() {
		return false
	}
	return !strings.EqualFold(e.graphics.strokeCS, "Pattern") || e.graphics.strokePattern != nil
}

// closeFillAndStrokePath closes, fills, and strokes the current path - 'b' operator.
func (e *Evaluator) closeFillAndStrokePath() error {
	// Close the path first
	e.graphics.path.ClosePath()
	if rawPath := e.currentRawPath(); rawPath != nil {
		rawPath.ClosePath()
	}

	return e.fillAndStrokePath()
}

// closeFillAndStrokePathEvenOdd closes, fills, and strokes using even-odd rule - 'b*' operator.
func (e *Evaluator) closeFillAndStrokePathEvenOdd() error {
	// Close the path first
	e.graphics.path.ClosePath()
	if rawPath := e.currentRawPath(); rawPath != nil {
		rawPath.ClosePath()
	}

	return e.fillAndStrokePathEvenOdd()
}

// endPath ends the current path without filling or stroking - 'n' operator.
func (e *Evaluator) endPath() error {
	e.applyPendingClipAtPathEnd()
	e.clearCurrentPath()
	return nil
}

// renderPathToCanvas renders the current path to the canvas.
func (e *Evaluator) renderPathToCanvas(fill bool) {
	if e.canvas == nil {
		return
	}
	e.syncCanvasColors()
	if fill && e.shouldUsePopplerOrderFillPath() && e.tryRenderFillPathInPopplerOrder(false) {
		return
	}
	if !fill && e.shouldUsePopplerOrderStrokePath() && e.tryRenderStrokePathInPopplerOrder() {
		return
	}
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
	if e.shouldUsePopplerOrderFillPath() && e.tryRenderFillPathInPopplerOrder(true) {
		return
	}
	e.replayPathToCanvas()

	if evenOddCanvas, ok := e.canvas.(interface{ FillEvenOdd() }); ok {
		evenOddCanvas.FillEvenOdd()
		return
	}

	e.canvas.Fill()
}

func (e *Evaluator) tryRenderFillPathInPopplerOrder(evenOdd bool) bool {
	canvas, ok := e.canvas.(popplerOrderFillCanvas)
	if !ok || e.graphics == nil || e.graphics.path == nil || e.graphics.path.IsEmpty() {
		return false
	}
	if strings.EqualFold(e.graphics.fillCS, "Pattern") {
		return false
	}
	rawPath := e.graphics.rawPath
	if rawPath == nil || rawPath.IsEmpty() {
		inv, ok := invertMatrix(e.graphics.transform)
		if !ok {
			return false
		}
		rawPath = transformRendererPath(e.graphics.path, inv)
	}
	if rawPath == nil || rawPath.IsEmpty() {
		return false
	}
	force := forcePopplerOrderFillPath()
	if !force && !popplerOrderFillPathCandidate(rawPath) {
		tracePopplerOrderFillPathCandidate("skip", rawPath, e.graphics.transform, evenOdd)
		return false
	}
	tracePopplerOrderFillPathCandidate("use", rawPath, e.graphics.transform, evenOdd)
	if pathCanvas, ok := e.canvas.(popplerOrderFillPathCanvas); ok {
		pathCanvas.FillPathObjectWithCTM(rawPath, e.graphics.transform, evenOdd)
		return true
	}
	canvas.FillPathWithCTM(rawPath.Elements(), e.graphics.transform, evenOdd)
	return true
}

func (e *Evaluator) tryRenderStrokePathInPopplerOrder() bool {
	canvas, ok := e.canvas.(popplerOrderStrokeCanvas)
	if !ok || e.graphics == nil || e.graphics.path == nil || e.graphics.path.IsEmpty() {
		return false
	}
	rawPath := e.graphics.rawPath
	if rawPath == nil || rawPath.IsEmpty() {
		inv, ok := invertMatrix(e.graphics.transform)
		if !ok {
			return false
		}
		rawPath = transformRendererPath(e.graphics.path, inv)
	}
	if rawPath == nil || rawPath.IsEmpty() {
		return false
	}
	dash := e.graphics.currentState.GetDashArray()
	if len(dash) > 0 {
		dash = append([]float64(nil), dash...)
	}
	if pathCanvas, ok := e.canvas.(popplerOrderStrokePathCanvas); ok {
		pathCanvas.StrokePathObjectWithCTM(
			rawPath,
			e.graphics.transform,
			e.graphics.lineWidth,
			dash,
			e.graphics.currentState.GetDashPhase(),
		)
	} else {
		canvas.StrokePathWithCTM(
			rawPath.Elements(),
			e.graphics.transform,
			e.graphics.lineWidth,
			dash,
			e.graphics.currentState.GetDashPhase(),
		)
	}
	return true
}

func (e *Evaluator) shouldUsePopplerOrderStrokePath() bool {
	if os.Getenv("PDF_DISABLE_SPLASH_POPPLER_ORDER_STROKE_PATH") == "1" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_POPPLER_ORDER_STROKE_PATH"))) {
	case "0", "false", "off", "legacy":
		return false
	default:
		return true
	}
}

func (e *Evaluator) shouldUsePopplerOrderFillPath() bool {
	if os.Getenv("PDF_DISABLE_SPLASH_POPPLER_ORDER_FILL_PATH") == "1" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_POPPLER_ORDER_FILL_PATH"))) {
	case "0", "false", "off", "legacy":
		return false
	case "1", "true", "on", "all", "broad", "scoped", "candidate", "closed", "line":
		return true
	default:
		return true
	}
}

func forcePopplerOrderFillPath() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_POPPLER_ORDER_FILL_PATH"))) {
	case "1", "true", "on", "all", "broad":
		return true
	default:
		return false
	}
}

func allowSmallOpenCurvedPopplerOrderFillPath() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_POPPLER_ORDER_FILL_OPEN_CURVE_PATH"))) {
	case "0", "false", "off", "legacy":
		return false
	case "1", "true", "on":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PDF_DEBUG_SPLASH_POPPLER_ORDER_FILL_PATH"))) {
	case "closed", "line":
		return false
	default:
		return true
	}
}

func popplerOrderFillPathCandidate(path *Path) bool {
	if path == nil || path.IsEmpty() {
		return false
	}
	elementCount := path.ElementCount()
	if elementCount > 64 {
		return false
	}
	moveCount := 0
	curveCount := 0
	closeCount := 0
	for i := 0; i < elementCount; i++ {
		typ, _, _, _, _, _, _, ok := path.ElementAt(i)
		if !ok {
			continue
		}
		switch typ {
		case PathMoveTo:
			moveCount++
		case PathClose:
			closeCount++
		case PathCurveTo:
			curveCount++
		}
	}
	hasClose := closeCount > 0
	hasCurve := curveCount > 0
	allowOpenCurve := allowSmallOpenCurvedPopplerOrderFillPath()
	if hasCurve && !allowOpenCurve && !hasClose {
		return false
	}
	if !hasClose && !(hasCurve && allowOpenCurve) {
		return false
	}
	xMin, yMin, xMax, yMax := path.GetBounds()
	width := xMax - xMin
	height := yMax - yMin
	if width <= 0 || height <= 0 {
		return false
	}
	if width <= 40 && height <= 40 {
		return true
	}
	return moderateClosedCurvedPopplerOrderFillPathCandidate(elementCount, moveCount, curveCount, closeCount, width, height)
}

func moderateClosedCurvedPopplerOrderFillPathCandidate(elementCount, moveCount, curveCount, closeCount int, width, height float64) bool {
	if elementCount > 64 || moveCount > 8 || curveCount < 4 || closeCount == 0 {
		return false
	}
	return width <= 260 && height <= 240
}

func tracePopplerOrderFillPathCandidate(decision string, path *Path, ctm [6]float64, evenOdd bool) {
	if os.Getenv("PDF_DEBUG_SPLASH_POPPLER_ORDER_FILL_CANDIDATE_TRACE") != "1" {
		return
	}
	moveCount := 0
	lineCount := 0
	curveCount := 0
	closeCount := 0
	elementCount := path.ElementCount()
	for i := 0; i < elementCount; i++ {
		typ, _, _, _, _, _, _, ok := path.ElementAt(i)
		if !ok {
			continue
		}
		switch typ {
		case PathMoveTo:
			moveCount++
		case PathLineTo:
			lineCount++
		case PathCurveTo:
			curveCount++
		case PathClose:
			closeCount++
		}
	}
	xMin, yMin, xMax, yMax := path.GetBounds()
	devicePath := transformRendererPath(path, ctm)
	dxMin, dyMin, dxMax, dyMax := devicePath.GetBounds()
	fmt.Fprintf(
		os.Stderr,
		"poppler-fill-candidate decision=%s evenOdd=%v elems=%d moves=%d lines=%d curves=%d closes=%d raw=[%.6f %.6f %.6f %.6f] rawSize=[%.6f %.6f] dev=[%.6f %.6f %.6f %.6f] devSize=[%.6f %.6f]\n",
		decision, evenOdd, elementCount, moveCount, lineCount, curveCount, closeCount,
		xMin, yMin, xMax, yMax, xMax-xMin, yMax-yMin,
		dxMin, dyMin, dxMax, dyMax, dxMax-dxMin, dyMax-dyMin,
	)
}

func (e *Evaluator) shouldUsePopplerOrderFillStrokePath() bool {
	return os.Getenv("PDF_DEBUG_SPLASH_POPPLER_ORDER_FILL_STROKE_PATH") == "1"
}

func invertMatrix(m [6]float64) ([6]float64, bool) {
	det := m[0]*m[3] - m[1]*m[2]
	if math.Abs(det) < 1e-12 {
		return [6]float64{}, false
	}
	invDet := 1.0 / det
	return [6]float64{
		m[3] * invDet,
		-m[1] * invDet,
		-m[2] * invDet,
		m[0] * invDet,
		(m[2]*m[5] - m[3]*m[4]) * invDet,
		(m[1]*m[4] - m[0]*m[5]) * invDet,
	}, true
}

func transformRendererPath(path *Path, matrix [6]float64) *Path {
	if path == nil {
		return nil
	}
	out := NewPath()
	for i := 0; i < path.ElementCount(); i++ {
		typ, x1, y1, x2, y2, x, y, ok := path.ElementAt(i)
		if !ok {
			continue
		}
		switch typ {
		case PathMoveTo:
			tx, ty := transformPointWithMatrix(matrix, x, y)
			out.MoveTo(tx, ty)
		case PathLineTo:
			tx, ty := transformPointWithMatrix(matrix, x, y)
			out.LineTo(tx, ty)
		case PathCurveTo:
			tx1, ty1 := transformPointWithMatrix(matrix, x1, y1)
			tx2, ty2 := transformPointWithMatrix(matrix, x2, y2)
			tx, ty := transformPointWithMatrix(matrix, x, y)
			out.CurveTo(tx1, ty1, tx2, ty2, tx, ty)
		case PathClose:
			out.ClosePath()
		}
	}
	return out
}

func (e *Evaluator) replayPathToCanvas() {
	if e.canvas == nil {
		return
	}
	path := e.graphics.path
	for i := 0; i < path.ElementCount(); i++ {
		typ, x1, y1, x2, y2, x, y, ok := path.ElementAt(i)
		if !ok {
			continue
		}
		switch typ {
		case PathMoveTo:
			e.canvas.MoveTo(x, y)
		case PathLineTo:
			e.canvas.LineTo(x, y)
		case PathCurveTo:
			e.canvas.CurveTo(x1, y1, x2, y2, x, y)
		case PathClose:
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
	if setter, ok := e.canvas.(interface{ SetTextRenderMode(mode int) }); ok {
		setter.SetTextRenderMode(e.graphics.currentState.GetTextRenderMode())
	}
	type glyphTransformSetter interface {
		SetGlyphTransform(t [4]float64)
	}
	setter, ok := e.canvas.(glyphTransformSetter)
	if !ok {
		return
	}
	trm := e.textPlacement.CurrentRenderingMatrix(e)
	hScale := e.graphics.currentState.GetHorizontalScaling() / 100.0
	if hScale == 0 {
		hScale = 1.0
	}
	trm[0] *= hScale
	trm[1] *= hScale
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
	// Re-assert the ExtGState (/ca, /CA) opacities after the color setters. The
	// splash canvas derives stroke/fill alpha from the color.Color's alpha channel
	// (quantized through the color-component byte conversion), which clobbers the
	// exact /ca-/CA float (e.g. 0.7 -> 178/255 instead of splashRound(0.7*255)=179).
	// Poppler keeps color (setFillPattern, opaque) and opacity (setFillAlpha, double)
	// fully decoupled (SplashOutputDev.cc:1738-1757); mirror that by restoring the
	// unquantized opacity here so the pipe computes splashRound(alpha*255) exactly.
	e.syncCanvasFillAlpha()
	e.syncCanvasStrokeAlpha()
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
	d1 := (m[0]+m[2])*(m[0]+m[2]) + (m[1]+m[3])*(m[1]+m[3])
	d2 := (m[0]-m[2])*(m[0]-m[2]) + (m[1]-m[3])*(m[1]-m[3])
	if d2 > d1 {
		d1 = d2
	}
	d1 *= 0.5
	if d1 <= 0 {
		return 1.0
	}
	return math.Sqrt(d1)
}

// popplerShadingPatternCTM replicates Gfx::doShadingPatternFill's matrix
// construction bit-for-bit: ictm = adjugate-inverse of the current CTM
// (det reciprocal FIRST), m1 = ptm×btm, m = m1×ictm, result = concat(m, ctm)
// with GfxState::concatCTM's exact expression order.
func popplerShadingPatternCTM(ptm, btm, ctm [6]float64) [6]float64 {
	det := ctm[0]*ctm[3] - ctm[1]*ctm[2]
	if math.Abs(det) < 0.000001 {
		return multiplyMatrix(btm, ptm)
	}
	det = 1 / det
	var ictm [6]float64
	ictm[0] = ctm[3] * det
	ictm[1] = -ctm[1] * det
	ictm[2] = -ctm[2] * det
	ictm[3] = ctm[0] * det
	ictm[4] = (ctm[2]*ctm[5] - ctm[3]*ctm[4]) * det
	ictm[5] = (ctm[1]*ctm[4] - ctm[0]*ctm[5]) * det
	var m1, m, out [6]float64
	m1[0] = ptm[0]*btm[0] + ptm[1]*btm[2]
	m1[1] = ptm[0]*btm[1] + ptm[1]*btm[3]
	m1[2] = ptm[2]*btm[0] + ptm[3]*btm[2]
	m1[3] = ptm[2]*btm[1] + ptm[3]*btm[3]
	m1[4] = ptm[4]*btm[0] + ptm[5]*btm[2] + btm[4]
	m1[5] = ptm[4]*btm[1] + ptm[5]*btm[3] + btm[5]
	m[0] = m1[0]*ictm[0] + m1[1]*ictm[2]
	m[1] = m1[0]*ictm[1] + m1[1]*ictm[3]
	m[2] = m1[2]*ictm[0] + m1[3]*ictm[2]
	m[3] = m1[2]*ictm[1] + m1[3]*ictm[3]
	m[4] = m1[4]*ictm[0] + m1[5]*ictm[2] + ictm[4]
	m[5] = m1[4]*ictm[1] + m1[5]*ictm[3] + ictm[5]
	// GfxState::concatCTM(m): new = m ∘ ctm
	out[0] = m[0]*ctm[0] + m[1]*ctm[2]
	out[1] = m[0]*ctm[1] + m[1]*ctm[3]
	out[2] = m[2]*ctm[0] + m[3]*ctm[2]
	out[3] = m[2]*ctm[1] + m[3]*ctm[3]
	out[4] = m[4]*ctm[0] + m[5]*ctm[2] + ctm[4]
	out[5] = m[4]*ctm[1] + m[5]*ctm[3] + ctm[5]
	return out
}

func (e *Evaluator) patternForCanvas(pattern entity.Pattern) entity.Pattern {
	if pattern == nil {
		return nil
	}

	switch typed := pattern.(type) {
	case *entity.ShadingPattern:
		clone := entity.NewShadingPattern(typed.Name(), typed.GetShading())
		effectiveMatrix := multiplyMatrix(e.graphics.baseTransform, typed.Matrix())
		if os.Getenv("PDF_DEBUG_SHADING_PATTERN_MATRIX_POPPLER_ORDER") == "1" {
			// Probe Poppler Gfx::doShadingPatternFill's explicit PTM * baseMatrix
			// order without changing the current default matrix convention.
			effectiveMatrix = multiplyMatrix(typed.Matrix(), e.graphics.baseTransform)
		}
		if e.canvasYDownBase() {
			// Poppler Gfx::doShadingPatternFill (Gfx.cc:2229-2264) does NOT use
			// PTM×BTM directly: it composes m = (PTM×BTM)×inverse(CTM) and then
			// concatCTM(m), i.e. the effective pattern CTM makes a float
			// ROUND-TRIP through inverse(ctm)×ctm. That round-trip leaves ~2e-13
			// residue (e.g. day1 f: 1125.0 → 1125.0000000000002) which decides
			// shading colour bands (t at band edges). Replicate it verbatim.
			effectiveMatrix = popplerShadingPatternCTM(typed.Matrix(), e.graphics.baseTransform, e.graphics.transform)
		}
		clone.SetMatrix(effectiveMatrix)
		return clone
	case *entity.TilingPattern:
		// Poppler Gfx::doTilingPatternFill builds the tiling grid from
		// PTM * baseMatrix and cancels the current CTM before replaying the
		// pattern form. Keep tiling phase anchored to page/form base space.
		effectiveMatrix := multiplyMatrix(e.graphics.baseTransform, pattern.Matrix())
		if os.Getenv("PDF_DEBUG_TILING_PATTERN_MATRIX_POPPLER_ORDER") == "1" {
			effectiveMatrix = multiplyMatrix(pattern.Matrix(), e.graphics.baseTransform)
		}
		clone := entity.NewTilingPattern(typed.Name(), typed.GetPaintType(), typed.GetTilingType())
		clone.SetMatrix(effectiveMatrix)
		clone.SetBBox(typed.GetBBox())
		clone.SetXStep(typed.GetXStep())
		clone.SetYStep(typed.GetYStep())
		clone.SetResources(typed.GetResources())
		if e.xref != nil {
			clone.SetXRef(e.xref)
		} else {
			clone.SetXRef(typed.XRef())
		}
		resourceStack := e.currentResourceStack()
		patternResources := typed.GetResources()
		if len(resourceStack) > 0 || patternResources != nil {
			stack := make([]*entity.Dict, 0, len(resourceStack)+1)
			stack = append(stack, patternResources)
			stack = append(stack, resourceStack...)
			clone.SetResourceStack(stack)
		}
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

	clipMode := e.graphics.pendingClipMode
	clipPath := e.graphics.path

	if e.canvas != nil {
		e.applyClippingPathObject(clipPath, clipMode)
	}

	e.graphics.pathClip = nil
	e.setCurrentPathClipBounds(clipPath)
	e.graphics.clipMode = clipMode
	e.graphics.pendingClip = false
}

func (e *Evaluator) setCurrentPathClipBounds(path *Path) {
	if e == nil || e.graphics == nil {
		return
	}
	e.graphics.pathClipBounds = [4]float64{}
	e.graphics.pathClipBoundsValid = false
	if path == nil || path.IsEmpty() {
		return
	}
	xMin, yMin, xMax, yMax := path.GetBounds()
	if xMax <= xMin || yMax <= yMin {
		return
	}
	e.graphics.pathClipBounds = [4]float64{xMin, yMin, xMax, yMax}
	e.graphics.pathClipBoundsValid = true
}

func (e *Evaluator) currentPathClipBounds() (float64, float64, float64, float64, bool) {
	if e == nil || e.graphics == nil {
		return 0, 0, 0, 0, false
	}
	if e.graphics.pathClipBoundsValid {
		b := e.graphics.pathClipBounds
		return b[0], b[1], b[2], b[3], true
	}
	if e.graphics.pathClip == nil || e.graphics.pathClip.IsEmpty() {
		return 0, 0, 0, 0, false
	}
	xMin, yMin, xMax, yMax := e.graphics.pathClip.GetBounds()
	if xMax <= xMin || yMax <= yMin {
		return 0, 0, 0, 0, false
	}
	return xMin, yMin, xMax, yMax, true
}

// applyClippingPath applies the current clipping path to the canvas.
func (e *Evaluator) applyClippingPath() {
	if e.canvas == nil || e.graphics.pathClip == nil {
		return
	}
	e.applyClippingPathObject(e.graphics.pathClip, ClipNonZeroWinding)
}

func (e *Evaluator) applyClippingPathObject(path *Path, mode ClipMode) {
	if e.canvas == nil || path == nil {
		return
	}
	// Try to use the canvas implementation's SetClipPathDirect method
	type clipCanvas interface {
		SetClipPathDirect(elements []interface{}, fillRule graphics.FillRule)
	}

	if clipImpl, ok := e.canvas.(clipCanvas); ok {
		// Convert []PathElement to []interface{}
		pathElements := path.Elements()
		elements := make([]interface{}, len(pathElements))
		for i, elem := range pathElements {
			elements[i] = elem
		}
		fillRule := graphics.FillRuleNonZero
		if mode == ClipEvenOdd {
			fillRule = graphics.FillRuleEvenOdd
		}
		clipImpl.SetClipPathDirect(elements, fillRule)
		return
	}

	// Fallback: replay clipping path elements to canvas (may interfere with current path)
	e.replayPathObjectToCanvas(path)

	// Apply clipping
	if mode == ClipEvenOdd {
		e.canvas.EoClip()
	} else {
		e.canvas.Clip()
	}
}

// applyClippingPathEvenOdd applies the clipping path using even-odd rule.
func (e *Evaluator) applyClippingPathEvenOdd() {
	if e.canvas == nil || e.graphics.pathClip == nil {
		return
	}
	e.applyClippingPathObject(e.graphics.pathClip, ClipEvenOdd)
}

func (e *Evaluator) replayPathObjectToCanvas(path *Path) {
	if e == nil || e.canvas == nil || path == nil {
		return
	}
	for i := 0; i < path.ElementCount(); i++ {
		typ, x1, y1, x2, y2, x, y, ok := path.ElementAt(i)
		if !ok {
			continue
		}
		switch typ {
		case PathMoveTo:
			e.canvas.MoveTo(x, y)
		case PathLineTo:
			e.canvas.LineTo(x, y)
		case PathCurveTo:
			e.canvas.CurveTo(x1, y1, x2, y2, x, y)
		case PathClose:
			e.canvas.ClosePath()
		}
	}
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

// SetOperatorRecording controls whether Evaluate/EvaluateContent retains parsed operators for GetOperators.
func (e *Evaluator) SetOperatorRecording(enabled bool) {
	e.recordOperators = enabled
	if !enabled {
		e.operators = e.operators[:0]
	}
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
	if resources == nil {
		e.replaceResourceStack(nil)
		return
	}
	e.replaceResourceStack([]*entity.Dict{resources})
}

// SetResourceStack sets the evaluator resource lookup chain, with the first
// dictionary searched before later parent dictionaries.
func (e *Evaluator) SetResourceStack(resources []*entity.Dict) {
	e.replaceResourceStack(resources)
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

// ResetForTilingPatternTile restores volatile rendering state before replaying a pattern tile.
func (e *Evaluator) ResetForTilingPatternTile(matrix [6]float64) {
	if e == nil {
		return
	}
	identity := [6]float64{1, 0, 0, 1, 0, 0}
	e.releaseSavedGraphicsStates()
	if e.graphics == nil {
		e.graphics = NewGraphicsState()
	} else {
		e.graphics.ResetForReuse()
	}
	e.initialTransform = matrix
	e.graphics.transform = matrix
	e.graphics.baseTransform = matrix
	e.textMatrix = identity
	e.textLineMatrix = identity
	e.textBaseMatrix = identity
	e.textLineX = 0
	e.textLineY = 0
	e.textCurrentX = 0
	e.textCurrentY = 0
	e.textUserCurrentX = 0
	e.textUserCurrentY = 0
	e.textCurrentValid = false
	e.textUserCurrentValid = false
	e.resetInlineImageState()
	e.operators = e.operators[:0]
	e.debugPath = e.debugPath[:0]
	e.textBuffer.Reset()
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
		e.graphics.fillColor = defaultBlackColorSpace
		e.graphics.fillAlpha = 1.0
		e.graphics.fillCS = "DeviceRGB"
		e.graphics.fillParsedCS = nil
		return
	}

	r, g, b, a := c.RGBA()
	e.graphics.fillColor = &ColorSpace{Color: &Color{Hex: fmt.Sprintf("%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8))}}
	e.graphics.fillAlpha = float64(a) / 65535.0
	e.graphics.fillCS = "DeviceRGB"
	e.graphics.fillParsedCS = nil
}

// SetStrokeColor sets the current stroke color for evaluator-rendered content.
func (e *Evaluator) SetStrokeColor(c color.Color) {
	if c == nil {
		e.graphics.strokeColor = defaultBlackColorSpace
		e.graphics.strokeAlpha = 1.0
		e.graphics.strokeCS = "DeviceRGB"
		e.graphics.strokeParsedCS = nil
		return
	}

	r, g, b, a := c.RGBA()
	e.graphics.strokeColor = &ColorSpace{Color: &Color{Hex: fmt.Sprintf("%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8))}}
	e.graphics.strokeAlpha = float64(a) / 65535.0
	e.graphics.strokeCS = "DeviceRGB"
	e.graphics.strokeParsedCS = nil
}

// SetFillPattern sets the current fill pattern for evaluator-rendered content.
func (e *Evaluator) SetFillPattern(pattern entity.Pattern) {
	e.graphics.fillPattern = pattern
}

// SetStrokePattern sets the current stroke pattern for evaluator-rendered content.
func (e *Evaluator) SetStrokePattern(pattern entity.Pattern) {
	e.graphics.strokePattern = pattern
}

// SetLineWidthValue sets the line width without allocating a synthetic operator.
func (e *Evaluator) SetLineWidthValue(width float64) {
	if e == nil || e.graphics == nil || e.graphics.currentState == nil {
		return
	}
	if width < 0 {
		width = 0
	}
	e.graphics.lineWidth = width
	e.graphics.currentState.SetLineWidth(width)
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

func (e *Evaluator) executeInlineImageOperator(op Operator) error {
	if op.InlineImage == nil || op.InlineImage.Dict == nil {
		return nil
	}

	prevInInlineImage := e.inInlineImage
	prevInlineImageDict := e.inlineImageDict
	prevInlineImageData := e.inlineImageData
	defer func() {
		e.inInlineImage = prevInInlineImage
		e.inlineImageDict = prevInlineImageDict
		e.inlineImageData = prevInlineImageData
	}()

	e.inInlineImage = true
	e.inlineImageDict = op.InlineImage.Dict
	e.inlineImageData = op.InlineImage.Data
	return e.endInlineImage()
}

// renderInlineImage renders an inline image to the canvas.
func (e *Evaluator) renderInlineImage() error {
	dict := e.inlineImageDict

	// Get image dimensions
	widthVal := dict.GetTry(pdfNameW, pdfNameWidth)
	if widthVal == nil {
		return fmt.Errorf("inline image has no Width")
	}
	width, err := getNumberOperand(widthVal)
	if err != nil {
		return fmt.Errorf("inline image: invalid Width: %w", err)
	}

	heightVal := dict.GetTry(pdfNameH, pdfNameHeight)
	if heightVal == nil {
		return fmt.Errorf("inline image has no Height")
	}
	height, err := getNumberOperand(heightVal)
	if err != nil {
		return fmt.Errorf("inline image: invalid Height: %w", err)
	}

	bpcObj := dict.GetTry(pdfNameBPC, pdfNameBitsPerComponent)
	bpc := getImageBitsPerComponent(bpcObj)
	filterObj := dict.GetTry(pdfNameFilter, pdfNameF)
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

	imageMask := isImageMaskDictValue(dict.Get(pdfNameImageMask))
	if !imageMask {
		imageMask = isImageMaskDictValue(dict.Get(pdfNameIM))
	}
	if imageMask && bpcObj == nil {
		bpc = 1
	}
	if shouldSkipAllImagesForDebug() {
		return nil
	}
	if imageMask {
		interpolate, interpolateExplicit := resolveImageInterpolateOption(dict.Get(pdfNameI), false)
		if !interpolateExplicit {
			interpolate, interpolateExplicit = resolveImageInterpolateOption(dict.Get(pdfNameInterpolate), false)
		}
		decode := e.resolveImageDecodeArray(dict.Get(pdfNameDecode))
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

	colorSpaceVal := dict.GetTry(pdfNameCS, pdfNameColorSpace)
	colorSpace, ok := e.resolveImageColorSpace(colorSpaceVal)
	if !ok {
		return nil
	}
	colorMapper, ok := e.resolveImageColorMapper(colorSpace, colorSpaceVal)
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
		interpolate, interpolateExplicit := resolveImageInterpolateOption(dict.Get(pdfNameI), false)
		if !interpolateExplicit {
			interpolate, interpolateExplicit = resolveImageInterpolateOption(dict.Get(pdfNameInterpolate), false)
		}
		sourceICCBased := e.isICCBasedColorSpace(colorSpaceVal)
		var iccProfile []byte
		iccComponents := 0
		if sourceICCBased {
			iccProfile, _ = e.resolveICCBasedProfile(colorSpaceVal, 0)
			iccComponents = e.resolveICCBasedComponentCount(colorSpaceVal)
		}
		decode := e.resolveImageDecodeArray(dict.GetTry(pdfNameDecode, pdfNameD))
		smaskObj := dict.Get(pdfNameSMask)
		softMask := e.resolveSoftMaskDetails(smaskObj)
		mask := softMask.mask
		maskMatte := softMask.matte
		maskInterpolate := false
		if mask != nil {
			maskInterpolate = e.resolveImageMaskInterpolate(smaskObj)
		}
		if mask == nil {
			maskObj := dict.Get(pdfNameMask)
			// Inline image masks follow the same Poppler default as XObject masks:
			// mask interpolation is false unless the mask image explicitly opts in.
			softMask = e.resolveSoftMaskDetails(maskObj)
			mask = softMask.mask
			maskMatte = softMask.matte
			if mask != nil {
				maskInterpolate = e.resolveImageMaskInterpolate(maskObj)
			}
		}
		colorKeyMask := e.resolveColorKeyMask(dict.Get(pdfNameMask), colorSpace)
		if mask != nil {
			// When soft mask is present, favor SMask alpha and ignore color-key masking.
			colorKeyMask = nil
		}
		if os.Getenv("PDF_DEBUG_EVAL_GS") != "" {
			fmt.Fprintf(os.Stderr, "IMGDRAW w=%.0f h=%.0f cs=%s filter=%s smask=%t mask=%t matte=%t colorKey=%t fillAlpha=%.4f blend=%s\n",
				width, height, colorSpace, imageFilter, dict.Get(pdfNameSMask) != nil, dict.Get(pdfNameMask) != nil, maskMatte != nil, colorKeyMask != nil,
				e.graphics.fillAlpha, e.graphics.blendMode)
		}
		e.renderImageToCanvas(
			nil,
			data,
			width,
			height,
			colorSpace,
			colorMapper,
			sourceICCBased,
			iccProfile,
			iccComponents,
			indexedBase,
			indexedLookup,
			bpc,
			imageFilter,
			resolveXObjectImageSourceFilter(filterObj),
			e.resolveImageDecodeParms(dict.GetTry(pdfNameDecodeParms, pdfNameDP), 0),
			decode,
			mask,
			maskMatte,
			softMask.stream,
			false,
			maskInterpolate,
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

	if filter := dict.GetTry(pdfNameFilter, pdfNameF); filter != nil {
		normalized.Set(pdfNameFilter, normalizeInlineImageFilterObject(filter))
	}
	if decodeParms := dict.GetTry(pdfNameDecodeParms, pdfNameDP); decodeParms != nil {
		normalized.Set(pdfNameDecodeParms, decodeParms)
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
