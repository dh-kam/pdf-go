package renderer

import (
	"context"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"

	domaincanvas "github.com/dh-kam/pdf-go/internal/domain/canvas"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
	"github.com/dh-kam/pdf-go/internal/infrastructure/canvas"
	pdfstream "github.com/dh-kam/pdf-go/internal/infrastructure/pdf/stream"
)

const (
	annotationFlagHidden = 1 << 1
	annotationFlagNoView = 1 << 5

	annotationTextFieldMultiline = 1 << 12
)

const annotationTextNoteAppearance = "3.602 24 m 20.398 24 l 22.387 24 24 22.387 24 20.398 c 24 3.602 l 24\n" +
	"1.613 22.387 0 20.398 0 c 3.602 0 l 1.613 0 0 1.613 0 3.602 c 0 20.398\n" +
	"l 0 22.387 1.613 24 3.602 24 c h\n" +
	"3.602 24 m f\n" +
	"0.533333 0.541176 0.521569 RG 2 w\n" +
	"1 J\n" +
	"1 j\n" +
	"[] 0.0 d\n" +
	"4 M 9 18 m 4 18 l 4 7 4 4 6 3 c 20 3 l 18 4 18 7 18 18 c 17 18 l S\n" +
	"1.5 w\n" +
	"0 j\n" +
	"10 16 m 14 21 l S\n" +
	"1.85625 w\n" +
	"1 j\n" +
	"15.07 20.523 m 15.07 19.672 14.379 18.977 13.523 18.977 c 12.672 18.977\n" +
	"11.977 19.672 11.977 20.523 c 11.977 21.379 12.672 22.07 13.523 22.07 c\n" +
	"14.379 22.07 15.07 21.379 15.07 20.523 c h\n" +
	"15.07 20.523 m S\n" +
	"1 w\n" +
	"0 j\n" +
	"6.5 13.5 m 15.5 13.5 l S\n" +
	"6.5 10.5 m 13.5 10.5 l S\n" +
	"6.801 7.5 m 15.5 7.5 l S\n" +
	"0.729412 0.741176 0.713725 RG 2 w\n" +
	"1 j\n" +
	"9 19 m 4 19 l 4 8 4 5 6 4 c 20 4 l 18 5 18 8 18 19 c 17 19 l S\n" +
	"1.5 w\n" +
	"0 j\n" +
	"10 17 m 14 22 l S\n" +
	"1.85625 w\n" +
	"1 j\n" +
	"15.07 21.523 m 15.07 20.672 14.379 19.977 13.523 19.977 c 12.672 19.977\n" +
	"11.977 20.672 11.977 21.523 c 11.977 22.379 12.672 23.07 13.523 23.07 c\n" +
	"14.379 23.07 15.07 22.379 15.07 21.523 c h\n" +
	"15.07 21.523 m S\n" +
	"1 w\n" +
	"0 j\n" +
	"6.5 14.5 m 15.5 14.5 l S\n" +
	"6.5 11.5 m 13.5 11.5 l S\n" +
	"6.801 8.5 m 15.5 8.5 l S\n"

type annotationQuad struct {
	x1 float64
	y1 float64
	x2 float64
	y2 float64
	x3 float64
	y3 float64
	x4 float64
	y4 float64
}

type annotationPoint struct {
	x float64
	y float64
}

func (r *ConcurrentRenderer) renderPageAnnotations(ctx context.Context, page *entity.Page, c domaincanvas.Canvas, initial [6]float64, pageYOriginPx float64) error {
	annotations, err := page.Annotations()
	if err != nil {
		return fmt.Errorf("get page annotations: %w", err)
	}
	for _, annot := range annotations {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if annot == nil || !annotationIsVisible(annot) {
			continue
		}
		dict := annot.Dict()
		if dict == nil {
			continue
		}
		annotType := annot.Type().Value()
		if annotType == "Widget" {
			if err := renderWidgetAnnotation(page, c, initial, annot); err != nil {
				return err
			}
			continue
		}
		if appearance, rect, ok := annotationExistingAppearance(page, dict); ok {
			// Poppler draws usable /AP/N appearances for all visible annotation
			// subtypes via Gfx::drawAnnot; generated fallbacks are only for missing
			// or unusable appearances.
			if err := renderAnnotationAppearanceStream(page, c, initial, rect, appearance); err != nil {
				return err
			}
			if annotType == "Link" {
				if err := renderLinkAnnotation(page, c, initial, annot); err != nil {
					return err
				}
			}
			continue
		}
		switch annotType {
		case "Link":
			if err := renderLinkAnnotation(page, c, initial, annot); err != nil {
				return err
			}
			continue
		case "Text":
			if err := renderTextAnnotation(page, c, initial, annot); err != nil {
				return err
			}
		case "Highlight":
			if err := renderHighlightAnnotation(page, c, initial, pageYOriginPx, annot); err != nil {
				return err
			}
		case "Ink":
			if err := renderInkAnnotation(page, c, initial, annot); err != nil {
				return err
			}
		}
	}
	return nil
}

func annotationIsVisible(annot *entity.Annotation) bool {
	flags := annotationInteger(annot.Dict().Get(entity.Name("F")))
	return flags&annotationFlagHidden == 0 && flags&annotationFlagNoView == 0
}

func renderTextAnnotation(page *entity.Page, c domaincanvas.Canvas, initial [6]float64, annot *entity.Annotation) error {
	dict := annot.Dict()
	icon := annotationName(dict.Get(entity.Name("Name")))
	if icon != "" && icon != "Note" {
		return nil
	}
	rect, ok := annotationRect(dict)
	if !ok {
		return nil
	}
	fill := annotationColor(dict, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	if opacity, ok := annotationNumber(dict.Get(entity.Name("CA"))); ok && math.Abs(opacity-1) > 1e-9 {
		return renderTextAnnotationDirect(page, c, initial, rect, fill)
	}
	appearance := annotationTextNoteAppearanceStream(fill)
	appearanceRect := [4]float64{rect[0], rect[3] - 24, rect[0] + 24, rect[3]}
	return renderAnnotationAppearanceStream(page, c, initial, appearanceRect, appearance)
}

func renderTextAnnotationDirect(page *entity.Page, c domaincanvas.Canvas, initial [6]float64, rect [4]float64, fill color.RGBA) error {
	var b strings.Builder
	b.WriteString("q\n")
	fmt.Fprintf(&b, "1 0 0 1 %.2f %.2f cm\n", rect[0], rect[3]-24)
	annotationAppendFillColor(&b, fill)
	b.WriteString(annotationTextNoteAppearance)
	b.WriteString("Q\n")
	return evaluateAnnotationContent(page, c, initial, b.String())
}

func annotationTextNoteAppearanceStream(fill color.RGBA) *entity.Stream {
	var b strings.Builder
	b.WriteString("q\n")
	annotationAppendFillColor(&b, fill)
	b.WriteString(annotationTextNoteAppearance)
	b.WriteString("Q\n")
	data := []byte(b.String())

	dict := entity.NewDict()
	dict.Set(entity.Name("Type"), entity.Name("XObject"))
	dict.Set(entity.Name("Subtype"), entity.Name("Form"))
	dict.Set(entity.Name("FormType"), entity.NewInteger(1))
	dict.Set(entity.Name("BBox"), entity.NewArray(
		entity.NewReal(0),
		entity.NewReal(0),
		entity.NewReal(24),
		entity.NewReal(24),
	))
	dict.Set(entity.Name("Resources"), entity.NewDict())
	dict.Set(entity.Name("Length"), entity.NewInteger(int64(len(data))))
	return entity.NewStream(dict, data)
}

func renderHighlightAnnotation(page *entity.Page, c domaincanvas.Canvas, initial [6]float64, pageYOriginPx float64, annot *entity.Annotation) error {
	quads := annotationQuadPoints(annot.Dict().Get(entity.Name("QuadPoints")))
	if len(quads) == 0 {
		return nil
	}
	maskCanvas := annotationMaskCanvas(c, pageYOriginPx)
	var b strings.Builder
	b.WriteString("q\n0 0 0 rg\n")
	for _, quad := range quads {
		annotationAppendHighlightPath(&b, quad)
	}
	b.WriteString("Q\n")
	if err := evaluateAnnotationContent(page, maskCanvas, initial, b.String()); err != nil {
		return err
	}
	fill := annotationColor(annot.Dict(), color.RGBA{R: 255, G: 255, B: 0, A: 255})
	annotationApplyMultiplyMask(c, maskCanvas, fill)
	return nil
}

func annotationMaskCanvas(dst domaincanvas.Canvas, pageYOriginPx float64) domaincanvas.Canvas {
	if factory, ok := dst.(interface {
		NewAnnotationMaskCanvas(bounds image.Rectangle, pageYOriginPx float64) domaincanvas.Canvas
	}); ok {
		if mask := factory.NewAnnotationMaskCanvas(dst.Bounds(), pageYOriginPx); mask != nil {
			return mask
		}
	}
	maskCanvas := canvas.NewImageCanvas(dst.Bounds())
	if setter, ok := maskCanvas.(interface{ SetPageYOriginPx(float64) }); ok && pageYOriginPx > 0 {
		setter.SetPageYOriginPx(pageYOriginPx)
	}
	return maskCanvas
}

func renderInkAnnotation(page *entity.Page, c domaincanvas.Canvas, initial [6]float64, annot *entity.Annotation) error {
	dict := annot.Dict()
	rect, ok := annotationRect(dict)
	if !ok {
		return nil
	}
	paths := annotationInkList(dict.Get(entity.Name("InkList")))
	if len(paths) == 0 {
		return nil
	}
	stroke := annotationColor(dict, color.RGBA{A: 255})
	width := annotationBorderWidth(dict)
	var b strings.Builder
	b.WriteString("q\n")
	fmt.Fprintf(&b, "1 0 0 1 %.2f %.2f cm\n", rect[0], rect[1])
	annotationAppendStrokeColor(&b, stroke)
	fmt.Fprintf(&b, "[] 0 d\n%.2f w\n", width)
	for _, path := range paths {
		if len(path) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%.2f %.2f m\n", path[0].x-rect[0], path[0].y-rect[1])
		for _, pt := range path[1:] {
			fmt.Fprintf(&b, "%.2f %.2f l\n", pt.x-rect[0], pt.y-rect[1])
		}
		b.WriteString("S\n")
	}
	b.WriteString("Q\n")
	return evaluateAnnotationContent(page, c, initial, b.String())
}

// renderLinkAnnotation draws the visible border of a Link annotation when
// /Border[2] (border width) is non-zero. Color is /C (RGB) defaulting to black.
func renderLinkAnnotation(page *entity.Page, c domaincanvas.Canvas, initial [6]float64, annot *entity.Annotation) error {
	dict := annot.Dict()
	rect, ok := annotationRect(dict)
	if !ok {
		return nil
	}
	// /Border = [HRadius VRadius Width <DashArray>]; default [0 0 1].
	width := 1.0
	if borderArr, ok := dict.Get(entity.Name("Border")).(*entity.Array); ok && borderArr.Len() >= 3 {
		switch v := borderArr.Get(2).(type) {
		case *entity.Real:
			width = v.Value()
		case *entity.Integer:
			width = float64(v.Value())
		}
	}
	if width <= 0 {
		return nil
	}
	// /C color array (RGB or gray); default black.
	borderColor := color.RGBA{R: 0, G: 0, B: 0, A: 255}
	if col, ok := annotationColorFromArray(dict.Get(entity.Name("C"))); ok {
		borderColor = col
	}
	x := rect[0]
	y := rect[1]
	w := rect[2] - rect[0]
	h := rect[3] - rect[1]
	if w <= 0 || h <= 0 {
		return nil
	}
	// Poppler draws the link border with the stroke centered ON /Rect's edges
	// (no width/2 inset) — matches AnnotLink::draw which calls drawBorder with
	// the raw rect bounds. With strokeAdjust the resulting integer-aligned bars
	// straddle the /Rect boundary by half-width on each side.
	var b strings.Builder
	b.WriteString("q\n")
	annotationAppendStrokeColor(&b, borderColor)
	fmt.Fprintf(&b, "%.4f w\n", width)
	fmt.Fprintf(&b, "%.4f %.4f %.4f %.4f re s\n", x, y, w, h)
	b.WriteString("Q\n")
	return evaluateAnnotationContent(page, c, initial, b.String())
}

// renderWidgetAnnotation draws a generated Widget (form field) border when no
// usable /AP/N Form XObject exists. Poppler generates the field appearance in
// this case and emits the border as `0.5*w 0.5*w dx-w dy-w re s`.
func renderWidgetAnnotation(page *entity.Page, c domaincanvas.Canvas, initial [6]float64, annot *entity.Annotation) error {
	dict := annot.Dict()
	rect, ok := annotationRect(dict)
	if !ok {
		return nil
	}
	appearance, hasAppearance := annotationNormalAppearanceStream(page, dict)
	useGeneratedAppearance := annotationWidgetUsesGeneratedAppearance(page, hasAppearance)
	if text, ok := annotationWidgetTextValue(page, dict); ok && useGeneratedAppearance {
		return renderGeneratedTextWidgetAnnotation(page, c, initial, rect, dict, appearance, text)
	}
	// Poppler regenerates widget appearances only when /AP is missing or
	// AcroForm/NeedAppearances is true. Otherwise the existing appearance
	// stream is drawn as-is, even if it has no text-showing operators.
	if hasAppearance && !useGeneratedAppearance {
		return renderAnnotationAppearanceStream(page, c, initial, rect, appearance)
	}
	mk, _ := dict.Get(entity.Name("MK")).(*entity.Dict)
	if mk == nil {
		return nil
	}
	backColor, hasBG := annotationColorFromArray(mk.Get(entity.Name("BG")))
	borderColor, hasBC := annotationColorFromArray(mk.Get(entity.Name("BC")))
	if !hasBC {
		borderColor = backColor
	}
	width := annotationBorderWidth(dict)
	if width <= 0 && !hasBG {
		return nil
	}
	x := rect[0]
	y := rect[1]
	dx := rect[2] - rect[0]
	dy := rect[3] - rect[1]
	if dx <= 0 || dy <= 0 {
		return nil
	}
	// Inset the rect by width/2 so the stroke center sits on the rect boundary
	// (mirrors AnnotAppearanceBuilder::drawFieldBorder).
	half := width / 2
	w := dx - width
	h := dy - width
	if w <= 0 || h <= 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("q\n")
	fmt.Fprintf(&b, "1 0 0 1 %.4f %.4f cm\n", x, y)
	// Poppler renders generated widget appearances through Gfx::drawForm(),
	// which clips the appearance stream to its local BBox before painting.
	fmt.Fprintf(&b, "0 0 %.2f %.2f re W n\n", dx, dy)
	if hasBG {
		annotationAppendFillColor(&b, backColor)
		fmt.Fprintf(&b, "0 0 %.2f %.2f re f\n", dx, dy)
	}
	if width > 0 && (hasBC || hasBG) {
		annotationAppendStrokeColor(&b, borderColor)
		fmt.Fprintf(&b, "%.2f w\n", width)
		fmt.Fprintf(&b, "%.2f %.2f %.2f %.2f re s\n", half, half, w, h)
	}
	b.WriteString("Q\n")
	return evaluateAnnotationContent(page, c, initial, b.String())
}

func annotationWidgetTextValue(page *entity.Page, dict *entity.Dict) (string, bool) {
	if annotationName(annotationInheritedObject(page, dict, entity.Name("FT"))) != "Tx" {
		return "", false
	}
	value, ok := annotationInheritedObject(page, dict, entity.Name("V")).(*entity.String)
	if !ok || value == nil {
		return "", false
	}
	text, ok := annotationTextString(value)
	if !ok || text == "" {
		return "", false
	}
	return text, true
}

func annotationInheritedObject(page *entity.Page, dict *entity.Dict, key entity.Name) entity.Object {
	for current := dict; current != nil; {
		if obj := current.GetRaw(key); obj != nil {
			return resolveAnnotationObject(page, obj)
		}
		parent := resolveAnnotationObject(page, current.GetRaw(entity.Name("Parent")))
		parentDict, _ := parent.(*entity.Dict)
		current = parentDict
	}
	return nil
}

func annotationWidgetUsesGeneratedAppearance(page *entity.Page, hasAppearance bool) bool {
	return !hasAppearance || annotationDocumentNeedAppearances(page)
}

func annotationExistingAppearance(page *entity.Page, dict *entity.Dict) (*entity.Stream, [4]float64, bool) {
	var zero [4]float64
	rect, ok := annotationRect(dict)
	if !ok {
		return nil, zero, false
	}
	appearance, ok := annotationNormalAppearanceStream(page, dict)
	if !ok {
		return nil, zero, false
	}
	return appearance, rect, true
}

func annotationDocumentNeedAppearances(page *entity.Page) bool {
	if page == nil || page.Document() == nil || page.Document().Catalog() == nil {
		return false
	}
	acroForm := resolveAnnotationObject(page, page.Document().Catalog().GetRaw(entity.Name("AcroForm")))
	acroDict, _ := acroForm.(*entity.Dict)
	if acroDict == nil {
		return false
	}
	need, _ := resolveAnnotationObject(page, acroDict.GetRaw(entity.Name("NeedAppearances"))).(*entity.Boolean)
	return need != nil && need.Value()
}

func renderGeneratedTextWidgetAnnotation(
	page *entity.Page,
	c domaincanvas.Canvas,
	initial [6]float64,
	rect [4]float64,
	dict *entity.Dict,
	appearance *entity.Stream,
	text string,
) error {
	content, resources, ok := annotationGeneratedTextWidgetContent(page, rect, dict, appearance, text)
	if !ok {
		return nil
	}
	return evaluateAnnotationContentWithResources(page, c, initial, content, resources)
}

func annotationGeneratedTextWidgetContent(
	page *entity.Page,
	rect [4]float64,
	dict *entity.Dict,
	appearance *entity.Stream,
	text string,
) (string, *entity.Dict, bool) {
	daObj, ok := annotationInheritedObject(page, dict, entity.Name("DA")).(*entity.String)
	if !ok || daObj == nil {
		return "", nil, false
	}
	da := strings.TrimSpace(daObj.Value())
	if da == "" {
		return "", nil, false
	}
	fontSize := annotationDefaultAppearanceFontSize(da)
	if fontSize <= 0 {
		return "", nil, false
	}
	dx := rect[2] - rect[0]
	dy := rect[3] - rect[1]
	if dx <= 0 || dy <= 0 {
		return "", nil, false
	}
	borderWidth := annotationExplicitBorderWidth(dict)
	x := annotationWidgetTextX(dict, dx, fontSize, borderWidth, text)
	y := 0.5*dy - 0.4*fontSize
	matrixX := x
	matrixY := y
	multiline := annotationInteger(annotationInheritedObject(page, dict, entity.Name("Ff")))&annotationTextFieldMultiline != 0
	if multiline {
		x = borderWidth + 2
		matrixX = 0
		matrixY = dy - 3
	}

	var b strings.Builder
	b.WriteString("q\n")
	fmt.Fprintf(&b, "1 0 0 1 %.4f %.4f cm\n", rect[0], rect[1])
	fmt.Fprintf(&b, "0 0 %.4f %.4f re W n\n", dx, dy)
	b.WriteString("BT\n")
	b.WriteString(annotationDAWithTextMatrix(da, matrixX, matrixY))
	b.WriteByte('\n')
	if multiline {
		fmt.Fprintf(&b, "%.2f %.2f Td\n", x, -fontSize)
	}
	b.WriteString(annotationPDFLiteralString(text))
	b.WriteString(" Tj\n")
	b.WriteString("ET\n")
	b.WriteString("Q\n")

	resources := annotationResourceDict(page, annotationInheritedObject(page, dict, entity.Name("DR")))
	if resources == nil && appearance != nil && appearance.Dict() != nil {
		resources = annotationResourceDict(page, appearance.Dict().GetRaw(entity.Name("Resources")))
	}
	return b.String(), resources, true
}

func annotationWidgetTextX(dict *entity.Dict, dx, fontSize, borderWidth float64, text string) float64 {
	quadding := annotationInteger(dict.Get(entity.Name("Q")))
	switch quadding {
	case 1:
		// Centering needs the laid-out text width to be exact. Keep a rough
		// fallback for generated appearances that otherwise would not render.
		return math.Max(0, (dx-float64(len(text))*fontSize*0.5)/2)
	case 2:
		return math.Max(borderWidth, dx-borderWidth-2-float64(len(text))*fontSize*0.5)
	default:
		return borderWidth + 2
	}
}

func annotationDAWithTextMatrix(da string, x, y float64) string {
	tokens := strings.Fields(da)
	for i, token := range tokens {
		if token == "Tm" && i >= 6 {
			tokens[i-2] = fmt.Sprintf("%.2f", x)
			tokens[i-1] = fmt.Sprintf("%.2f", y)
			return strings.Join(tokens, " ")
		}
	}
	return fmt.Sprintf("%s 1 0 0 1 %.2f %.2f Tm", da, x, y)
}

func annotationDefaultAppearanceFontSize(da string) float64 {
	tokens := strings.Fields(da)
	for i, token := range tokens {
		if token != "Tf" || i < 2 {
			continue
		}
		size, err := strconv.ParseFloat(tokens[i-1], 64)
		if err == nil {
			return size
		}
	}
	return 0
}

func annotationExplicitBorderWidth(dict *entity.Dict) float64 {
	if bs, ok := dict.Get(entity.Name("BS")).(*entity.Dict); ok {
		if width, ok := annotationNumber(bs.Get(entity.Name("W"))); ok && width >= 0 {
			return width
		}
	}
	values, ok := annotationNumberArray(dict.Get(entity.Name("Border")))
	if ok && len(values) >= 3 && values[2] >= 0 {
		return values[2]
	}
	return 0
}

func annotationResourceDict(page *entity.Page, obj entity.Object) *entity.Dict {
	resolved := resolveAnnotationObject(page, obj)
	if streamObj, ok := resolved.(*entity.Stream); ok {
		resolved = streamObj.Dict()
	}
	resources, _ := resolved.(*entity.Dict)
	return resources
}

func annotationAppearanceHasText(appearance *entity.Stream) bool {
	if appearance == nil {
		return false
	}
	infra := pdfstream.NewFromEntity(appearance)
	data, err := infra.Decode()
	if err != nil {
		data = appearance.RawBytes()
	}
	content := string(data)
	return strings.Contains(content, " Tj") ||
		strings.Contains(content, " TJ") ||
		strings.Contains(content, " '") ||
		strings.Contains(content, " \"")
}

func annotationTextString(value *entity.String) (string, bool) {
	if value == nil {
		return "", false
	}
	data := []byte(value.Value())
	if value.IsHex() {
		hexText := strings.TrimSpace(value.Value())
		if len(hexText)%2 == 1 {
			hexText += "0"
		}
		decoded, err := hex.DecodeString(hexText)
		if err == nil {
			data = decoded
		} else {
			// The parser currently stores hex strings as decoded bytes while
			// preserving IsHex=true; keep those bytes instead of dropping /V.
			data = []byte(value.Value())
		}
	}
	if len(data) >= 2 {
		switch {
		case data[0] == 0xfe && data[1] == 0xff:
			return decodeUTF16(data[2:], false), true
		case data[0] == 0xff && data[1] == 0xfe:
			return decodeUTF16(data[2:], true), true
		}
	}
	return string(data), true
}

func decodeUTF16(data []byte, littleEndian bool) string {
	if len(data)%2 == 1 {
		data = data[:len(data)-1]
	}
	words := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		if littleEndian {
			words = append(words, uint16(data[i])|uint16(data[i+1])<<8)
		} else {
			words = append(words, uint16(data[i])<<8|uint16(data[i+1]))
		}
	}
	return string(utf16.Decode(words))
}

func annotationPDFLiteralString(text string) string {
	var b strings.Builder
	b.WriteByte('(')
	for _, r := range text {
		switch r {
		case '\\', '(', ')':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r > 0x7e {
				b.WriteByte('?')
				continue
			}
			b.WriteRune(r)
		}
	}
	b.WriteByte(')')
	return b.String()
}

func annotationNormalAppearanceStream(page *entity.Page, dict *entity.Dict) (*entity.Stream, bool) {
	if dict == nil {
		return nil, false
	}
	apObj := resolveAnnotationObject(page, dict.GetRaw(entity.Name("AP")))
	apDict, ok := apObj.(*entity.Dict)
	if !ok || apDict == nil {
		return nil, false
	}
	normalObj := resolveAnnotationObject(page, apDict.GetRaw(entity.Name("N")))
	if stream, ok := normalObj.(*entity.Stream); ok {
		return stream, true
	}
	normalDict, ok := normalObj.(*entity.Dict)
	if !ok || normalDict == nil {
		return nil, false
	}
	for _, state := range annotationAppearanceStateNames(dict) {
		if state == "" {
			continue
		}
		if stream, ok := resolveAnnotationObject(page, normalDict.Get(entity.Name(state))).(*entity.Stream); ok {
			return stream, true
		}
	}
	return nil, false
}

func annotationAppearanceStateNames(dict *entity.Dict) []string {
	states := make([]string, 0, 3)
	if state := annotationName(dict.Get(entity.Name("AS"))); state != "" {
		states = append(states, state)
	}
	if state := annotationName(dict.Get(entity.Name("V"))); state != "" {
		states = append(states, state)
	}
	states = append(states, "Off")
	return states
}

func resolveAnnotationObject(page *entity.Page, obj entity.Object) entity.Object {
	if ref, ok := obj.(entity.Ref); ok && page != nil && page.Document() != nil && page.Document().XRef() != nil {
		if resolved, err := page.Document().XRef().Fetch(ref); err == nil {
			return resolved
		}
	}
	return obj
}

func renderAnnotationAppearanceStream(page *entity.Page, c domaincanvas.Canvas, initial [6]float64, rect [4]float64, appearance *entity.Stream) error {
	if appearance == nil || appearance.Dict() == nil {
		return nil
	}
	matrix, ok := annotationAppearanceMatrixForRect(appearance, rect)
	if !ok {
		return nil
	}
	clonedObj := appearance.Clone()
	cloned, ok := clonedObj.(*entity.Stream)
	if !ok {
		return nil
	}
	clonedDict := cloned.Dict()
	if clonedDict == nil {
		clonedDict = entity.NewDict()
	}
	clonedDict.Set(entity.Name("Matrix"), annotationMatrixArray(matrix))
	cloned.SetDict(clonedDict)

	evaluator := domainrenderer.NewEvaluator(page.Document().XRef())
	evaluator.SetCanvas(c)
	evaluator.SetInitialTransform(initial)
	if err := evaluator.EvaluateFormXObject(cloned, entity.Name("AnnotAP")); err != nil {
		return fmt.Errorf("render annotation appearance stream: %w", err)
	}
	return nil
}

func annotationAppearanceMatrixForRect(appearance *entity.Stream, rect [4]float64) ([6]float64, bool) {
	var zero [6]float64
	dict := appearance.Dict()
	bboxValues, ok := annotationNumberArray(dict.Get(entity.Name("BBox")))
	if !ok || len(bboxValues) != 4 {
		return zero, false
	}
	matrix := [6]float64{1, 0, 0, 1, 0, 0}
	if matrixValues, ok := annotationNumberArray(dict.Get(entity.Name("Matrix"))); ok && len(matrixValues) >= 6 {
		copy(matrix[:], matrixValues[:6])
	}

	formXMin, formYMin, formXMax, formYMax := transformedBBox(bboxValues, matrix)
	sx := 1.0
	if formXMin != formXMax {
		sx = (rect[2] - rect[0]) / (formXMax - formXMin)
	}
	sy := 1.0
	if formYMin != formYMax {
		sy = (rect[3] - rect[1]) / (formYMax - formYMin)
	}
	tx := -formXMin*sx + rect[0]
	ty := -formYMin*sy + rect[1]

	matrix[0] *= sx
	matrix[1] *= sy
	matrix[2] *= sx
	matrix[3] *= sy
	matrix[4] = matrix[4]*sx + tx
	matrix[5] = matrix[5]*sy + ty
	return matrix, true
}

func transformedBBox(bbox []float64, matrix [6]float64) (float64, float64, float64, float64) {
	x0, y0 := transformAnnotationPoint(matrix, bbox[0], bbox[1])
	xMin, yMin, xMax, yMax := x0, y0, x0, y0
	for _, pt := range [][2]float64{
		{bbox[0], bbox[3]},
		{bbox[2], bbox[1]},
		{bbox[2], bbox[3]},
	} {
		x, y := transformAnnotationPoint(matrix, pt[0], pt[1])
		xMin = math.Min(xMin, x)
		yMin = math.Min(yMin, y)
		xMax = math.Max(xMax, x)
		yMax = math.Max(yMax, y)
	}
	return xMin, yMin, xMax, yMax
}

func transformAnnotationPoint(matrix [6]float64, x, y float64) (float64, float64) {
	return x*matrix[0] + y*matrix[2] + matrix[4], x*matrix[1] + y*matrix[3] + matrix[5]
}

func annotationMatrixArray(matrix [6]float64) *entity.Array {
	return entity.NewArray(
		entity.NewReal(matrix[0]),
		entity.NewReal(matrix[1]),
		entity.NewReal(matrix[2]),
		entity.NewReal(matrix[3]),
		entity.NewReal(matrix[4]),
		entity.NewReal(matrix[5]),
	)
}

// annotationColorFromArray reads a PDF color array (1, 3, or 4 components in
// 0..1) and returns an RGBA. Returns false if the value is not a color array.
func annotationColorFromArray(obj entity.Object) (color.RGBA, bool) {
	arr, ok := obj.(*entity.Array)
	if !ok || arr.Len() == 0 {
		return color.RGBA{}, false
	}
	clamp := func(v float64) uint8 {
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		return uint8(v*255 + 0.5)
	}
	get := func(i int) (float64, bool) {
		switch v := arr.Get(i).(type) {
		case *entity.Real:
			return v.Value(), true
		case *entity.Integer:
			return float64(v.Value()), true
		}
		return 0, false
	}
	switch arr.Len() {
	case 1:
		v, ok := get(0)
		if !ok {
			return color.RGBA{}, false
		}
		g := clamp(v)
		return color.RGBA{R: g, G: g, B: g, A: 255}, true
	case 3:
		r, ok1 := get(0)
		g, ok2 := get(1)
		b, ok3 := get(2)
		if !ok1 || !ok2 || !ok3 {
			return color.RGBA{}, false
		}
		return color.RGBA{R: clamp(r), G: clamp(g), B: clamp(b), A: 255}, true
	case 4:
		c, ok1 := get(0)
		m, ok2 := get(1)
		y, ok3 := get(2)
		k, ok4 := get(3)
		if !ok1 || !ok2 || !ok3 || !ok4 {
			return color.RGBA{}, false
		}
		// Naive CMYK→RGB (matches widget border convention which is rarely CMYK).
		return color.RGBA{
			R: clamp((1 - c) * (1 - k)),
			G: clamp((1 - m) * (1 - k)),
			B: clamp((1 - y) * (1 - k)),
			A: 255,
		}, true
	}
	return color.RGBA{}, false
}

func evaluateAnnotationContent(page *entity.Page, c domaincanvas.Canvas, initial [6]float64, content string) error {
	return evaluateAnnotationContentWithResources(page, c, initial, content, nil)
}

func evaluateAnnotationContentWithResources(page *entity.Page, c domaincanvas.Canvas, initial [6]float64, content string, resources *entity.Dict) error {
	evaluator := domainrenderer.NewEvaluator(page.Document().XRef())
	evaluator.SetCanvas(c)
	evaluator.SetInitialTransform(initial)
	if resources != nil {
		evaluator.SetResources(resources)
	}
	if err := evaluator.EvaluateContent([]byte(content)); err != nil {
		return fmt.Errorf("render annotation appearance: %w", err)
	}
	return nil
}

func annotationAppendHighlightPath(b *strings.Builder, quad annotationQuad) {
	h4 := math.Abs(quad.y1-quad.y3) / 4
	fmt.Fprintf(b, "%.2f %.2f m\n", quad.x3, quad.y3)
	fmt.Fprintf(b, "%.2f %.2f %.2f %.2f %.2f %.2f c\n", quad.x3-h4, quad.y3+h4, quad.x1-h4, quad.y1-h4, quad.x1, quad.y1)
	fmt.Fprintf(b, "%.2f %.2f l\n", quad.x2, quad.y2)
	fmt.Fprintf(b, "%.2f %.2f %.2f %.2f %.2f %.2f c\n", quad.x2+h4, quad.y2-h4, quad.x4+h4, quad.y4+h4, quad.x4, quad.y4)
	b.WriteString("f\n")
}

func annotationApplyMultiplyMask(dstCanvas, maskCanvas domaincanvas.Canvas, fill color.RGBA) {
	if applier, ok := dstCanvas.(interface {
		ApplyAnnotationMultiplyMaskCanvas(mask domaincanvas.Canvas, fill color.RGBA)
	}); ok {
		applier.ApplyAnnotationMultiplyMaskCanvas(maskCanvas, fill)
		return
	}
	if applier, ok := dstCanvas.(interface {
		ApplyAnnotationMultiplyMask(mask image.Image, fill color.RGBA)
	}); ok {
		applier.ApplyAnnotationMultiplyMask(maskCanvas.Image(), fill)
		return
	}

	dst, ok := dstCanvas.Image().(*image.RGBA)
	if !ok {
		return
	}
	mask, ok := maskCanvas.Image().(*image.RGBA)
	if !ok {
		return
	}
	bounds := dst.Bounds().Intersect(mask.Bounds())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			maskAlpha := mask.RGBAAt(x, y).A
			if maskAlpha == 0 {
				continue
			}
			dstColor := dst.RGBAAt(x, y)
			multiplied := color.RGBA{
				R: annotationDiv255(uint32(dstColor.R) * uint32(fill.R)),
				G: annotationDiv255(uint32(dstColor.G) * uint32(fill.G)),
				B: annotationDiv255(uint32(dstColor.B) * uint32(fill.B)),
				A: dstColor.A,
			}
			dst.SetRGBA(x, y, color.RGBA{
				R: annotationBlendByMask(dstColor.R, multiplied.R, maskAlpha),
				G: annotationBlendByMask(dstColor.G, multiplied.G, maskAlpha),
				B: annotationBlendByMask(dstColor.B, multiplied.B, maskAlpha),
				A: dstColor.A,
			})
		}
	}
}

func annotationBlendByMask(dst, src, alpha uint8) uint8 {
	if alpha == 255 {
		return src
	}
	return annotationDiv255(uint32(dst)*uint32(255-alpha) + uint32(src)*uint32(alpha))
}

func annotationDiv255(x uint32) uint8 {
	return uint8((x + (x >> 8) + 0x80) >> 8)
}

func annotationAppendFillColor(b *strings.Builder, c color.RGBA) {
	fmt.Fprintf(b, "%.5f %.5f %.5f rg\n", float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
}

func annotationAppendStrokeColor(b *strings.Builder, c color.RGBA) {
	fmt.Fprintf(b, "%.5f %.5f %.5f RG\n", float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
}

func annotationRect(dict *entity.Dict) ([4]float64, bool) {
	var rect [4]float64
	values, ok := annotationNumberArray(dict.Get(entity.Name("Rect")))
	if !ok || len(values) != 4 {
		return rect, false
	}
	copy(rect[:], values)
	if rect[0] > rect[2] {
		rect[0], rect[2] = rect[2], rect[0]
	}
	if rect[1] > rect[3] {
		rect[1], rect[3] = rect[3], rect[1]
	}
	return rect, true
}

func annotationQuadPoints(obj entity.Object) []annotationQuad {
	values, ok := annotationNumberArray(obj)
	if !ok || len(values) < 8 {
		return nil
	}
	quads := make([]annotationQuad, 0, len(values)/8)
	for i := 0; i+7 < len(values); i += 8 {
		quads = append(quads, annotationQuad{
			x1: values[i],
			y1: values[i+1],
			x2: values[i+2],
			y2: values[i+3],
			x3: values[i+4],
			y3: values[i+5],
			x4: values[i+6],
			y4: values[i+7],
		})
	}
	return quads
}

func annotationInkList(obj entity.Object) [][]annotationPoint {
	arr, ok := obj.(*entity.Array)
	if !ok {
		return nil
	}
	paths := make([][]annotationPoint, 0, arr.Len())
	for i := 0; i < arr.Len(); i++ {
		values, ok := annotationNumberArray(arr.Get(i))
		if !ok || len(values) < 2 {
			continue
		}
		path := make([]annotationPoint, 0, len(values)/2)
		for j := 0; j+1 < len(values); j += 2 {
			path = append(path, annotationPoint{x: values[j], y: values[j+1]})
		}
		paths = append(paths, path)
	}
	return paths
}

func annotationBorderWidth(dict *entity.Dict) float64 {
	if bs, ok := dict.Get(entity.Name("BS")).(*entity.Dict); ok {
		if width, ok := annotationNumber(bs.Get(entity.Name("W"))); ok && width >= 0 {
			return width
		}
	}
	values, ok := annotationNumberArray(dict.Get(entity.Name("Border")))
	if ok && len(values) >= 3 && values[2] >= 0 {
		return values[2]
	}
	return 1
}

func annotationColor(dict *entity.Dict, fallback color.RGBA) color.RGBA {
	values, ok := annotationNumberArray(dict.Get(entity.Name("C")))
	if !ok {
		return fallback
	}
	switch len(values) {
	case 1:
		v := annotationUnitToByte(values[0])
		return color.RGBA{R: v, G: v, B: v, A: 255}
	case 3:
		return color.RGBA{
			R: annotationUnitToByte(values[0]),
			G: annotationUnitToByte(values[1]),
			B: annotationUnitToByte(values[2]),
			A: 255,
		}
	default:
		return fallback
	}
}

func annotationNumberArray(obj entity.Object) ([]float64, bool) {
	arr, ok := obj.(*entity.Array)
	if !ok {
		return nil, false
	}
	values := make([]float64, 0, arr.Len())
	for i := 0; i < arr.Len(); i++ {
		value, ok := annotationNumber(arr.Get(i))
		if !ok {
			return nil, false
		}
		values = append(values, value)
	}
	return values, true
}

func annotationNumber(obj entity.Object) (float64, bool) {
	switch v := obj.(type) {
	case *entity.Integer:
		return float64(v.Value()), true
	case *entity.Real:
		return v.Value(), true
	default:
		return 0, false
	}
}

func annotationInteger(obj entity.Object) int {
	switch v := obj.(type) {
	case *entity.Integer:
		return int(v.Value())
	case *entity.Real:
		return int(v.Value())
	default:
		return 0
	}
}

func annotationName(obj entity.Object) string {
	if name, ok := obj.(entity.Name); ok {
		return name.Value()
	}
	return ""
}

func annotationUnitToByte(value float64) uint8 {
	if value <= 0 {
		return 0
	}
	if value >= 1 {
		return 255
	}
	return uint8(math.Floor(value*255 + 0.5))
}
