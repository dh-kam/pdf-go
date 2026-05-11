//nolint:revive,errcheck // Java legacy parity aliases intentionally keep original exported naming and best-effort side effects.
package pdf

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// AddClosedFigure is an exported API.
func (d *Document) AddClosedFigure(annotation interface{}) bool {
	return d.addLegacyAnnotation("Polygon", annotation)
}

// AddEmptyPageAfter is an exported API.
func (d *Document) AddEmptyPageAfter(pageNumber int, _ bool) bool {
	if !d.IsValidPage(pageNumber) {
		return false
	}

	sourceIndex, err := d.resolveSourcePageIndex(pageNumber - 1)
	if err != nil {
		return false
	}

	return d.InsertPage(pageNumber, sourceIndex) == nil
}

// AddEmptyPageBefore is an exported API.
func (d *Document) AddEmptyPageBefore(pageNumber int, _ bool) bool {
	if pageNumber <= 0 {
		return false
	}

	pageCount, err := d.PageCount()
	if err != nil || pageCount <= 0 {
		return false
	}

	index := pageNumber - 1
	if index < 0 {
		index = 0
	}
	if index >= pageCount {
		index = pageCount - 1
	}

	sourceIndex, resolveErr := d.resolveSourcePageIndex(index)
	if resolveErr != nil {
		return false
	}

	return d.InsertPage(index, sourceIndex) == nil
}

// AddFileAttachment is an exported API.
func (d *Document) AddFileAttachment(args ...interface{}) bool {
	spec, path, ok := legacyAttachmentSpecFromArgs(args...)
	if !ok {
		return false
	}

	if strings.TrimSpace(path) != "" {
		if err := d.AddAttachmentFromFile(spec.Name, path); err != nil {
			return false
		}
		return true
	}

	return d.AddAttachment(spec) == nil
}

// AddImage is an exported API.
func (d *Document) AddImage(annotation interface{}) bool {
	return d.addLegacyAnnotation("Stamp", annotation)
}

// AddInk is an exported API.
func (d *Document) AddInk(annotation interface{}) bool {
	return d.addLegacyAnnotation("Ink", annotation)
}

// AddInkAnnotationPointPart is an exported API.
func (d *Document) AddInkAnnotationPointPart(value interface{}) bool {
	annotation, pageIndex, annotationIndex, pathList, ok := d.legacyAnnotationPointTarget(value)
	if !ok || annotation == nil {
		return false
	}

	current := annotation.PathList()
	next := make([][]float64, 0, len(current)+len(pathList))
	next = append(next, current...)
	next = append(next, pathList...)
	if len(next) == 0 {
		return false
	}
	return d.SetPageAnnotationPathList(pageIndex, annotationIndex, next) == nil
}

// AddNote is an exported API.
func (d *Document) AddNote(annotation interface{}) bool {
	return d.addLegacyAnnotation("Text", annotation)
}

// AddPolygon is an exported API.
func (d *Document) AddPolygon(annotation interface{}) bool {
	return d.addLegacyAnnotation("Polygon", annotation)
}

// AddScreen is an exported API.
func (d *Document) AddScreen(args ...interface{}) bool {
	if len(args) > 1 {
		if path, ok := legacyStringFromAny(args[1]); ok && strings.TrimSpace(path) != "" {
			name := ""
			if fileName := strings.TrimSpace(path); fileName != "" {
				name = fileName
			}
			_ = d.AddAttachmentFromFile(name, path)
		}
	}
	if len(args) == 0 {
		return false
	}
	return d.addLegacyAnnotation("Screen", args[0])
}

// AddTextMarkup is an exported API.
func (d *Document) AddTextMarkup(args ...interface{}) bool {
	if len(args) == 0 {
		return false
	}
	return d.addLegacyAnnotation("Highlight", args[0])
}

// AddWatermark is an exported API.
func (d *Document) AddWatermark(value interface{}) int {
	watermark, ok := legacyStringFromAny(value)
	if !ok || strings.TrimSpace(watermark) == "" {
		watermark = "watermark"
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.instantWatermarks = append(d.instantWatermarks, watermark)
	return len(d.instantWatermarks)
}

// AppendFullPageImage is an exported API.
func (d *Document) AppendFullPageImage(args ...interface{}) {
	d.appendLegacyImage(args, false, true, false)
}

// AppendFullPageMonoColorImage is an exported API.
func (d *Document) AppendFullPageMonoColorImage(args ...interface{}) {
	d.appendLegacyImage(args, true, true, false)
}

// AppendImage is an exported API.
func (d *Document) AppendImage(args ...interface{}) {
	d.appendLegacyImage(args, false, false, false)
}

// AppendImageJpeg is an exported API.
func (d *Document) AppendImageJpeg(args ...interface{}) {
	d.appendLegacyImage(args, false, false, true)
}

// AppendMonoColorImage is an exported API.
func (d *Document) AppendMonoColorImage(args ...interface{}) {
	d.appendLegacyImage(args, true, false, false)
}

// CreatePutImage is an exported API.
func (d *Document) CreatePutImage(args ...interface{}) int {
	data, _, ok := legacyImageDataFromArgs(args...)
	if !ok {
		return 0
	}
	return d.allocLegacyStreamHandle(data)
}

// CreatePutImageJpeg is an exported API.
func (d *Document) CreatePutImageJpeg(args ...interface{}) int {
	data, _, ok := legacyImageDataFromArgs(args...)
	if !ok {
		return 0
	}
	return d.allocLegacyStreamHandle(data)
}

// CreatePutMonoColorImage is an exported API.
func (d *Document) CreatePutMonoColorImage(args ...interface{}) int {
	width := 0
	height := 0
	colorValue := 0xFF000000
	ints := legacyIntArgs(args...)
	if len(ints) > 0 {
		width = ints[0]
	}
	if len(ints) > 1 {
		height = ints[1]
	}
	if len(ints) > 2 {
		colorValue = ints[2]
	}

	if width <= 0 || height <= 0 {
		if data, _, ok := legacyImageDataFromArgs(args...); ok {
			return d.allocLegacyStreamHandle(data)
		}
		return 0
	}

	if width > 4096 {
		width = 4096
	}
	if height > 4096 {
		height = 4096
	}

	payload := []byte(fmt.Sprintf("mono:%dx%d:%08x", width, height, colorValue))
	return d.allocLegacyStreamHandle(payload)
}

func (d *Document) appendLegacyImage(args []interface{}, monoColor bool, fullPage bool, forceJPEG bool) {
	if len(args) == 0 {
		return
	}

	pageNumber := d.legacyImagePageNumber(args, 1)
	if !d.IsValidPage(pageNumber) {
		return
	}
	pageIndex, resolved := d.resolveLegacyPageIndex(pageNumber)
	if !resolved {
		pageIndex = pageNumber - 1
	}

	rect := d.legacyImageRectFromArgs(args, pageNumber, fullPage)
	if rect[2] <= rect[0] || rect[3] <= rect[1] {
		return
	}

	handle := 0
	switch {
	case monoColor:
		handle = d.CreatePutMonoColorImage(args...)
	case forceJPEG:
		handle = d.CreatePutImageJpeg(args...)
	default:
		handle = d.CreatePutImage(args...)
	}
	if handle <= 0 {
		return
	}

	tag := d.legacyImageTagFromArgs(args)
	if tag == "" {
		tag = fmt.Sprintf("image-%d", handle)
	}

	spec := AnnotationSpec{
		Type:     "Stamp",
		Name:     tag,
		Rect:     rect,
		Contents: "appended-image",
		UserData: map[string]string{
			"image_handle":          strconv.Itoa(handle),
			"image_tag":             tag,
			"image_appended_as_tag": "true",
		},
	}
	_ = d.AddPageAnnotation(pageIndex, spec)
}

// CreateWriteEmptyPDF is an exported API.
func (d *Document) CreateWriteEmptyPDF(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false
	}
	const emptyPDF = "%PDF-1.4\n1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n2 0 obj << /Type /Pages /Count 0 /Kids [] >> endobj\nxref\n0 3\n0000000000 65535 f \n0000000010 00000 n \n0000000060 00000 n \ntrailer << /Size 3 /Root 1 0 R >>\nstartxref\n110\n%%EOF\n"
	return os.WriteFile(trimmed, []byte(emptyPDF), 0o644) == nil
}

// CreateWriteToStream is an exported API.
func (d *Document) CreateWriteToStream(reader io.Reader, _ bool, _ bool) int {
	if reader != nil {
		data, err := io.ReadAll(reader)
		if err == nil && len(data) > 0 {
			return d.allocLegacyStreamHandle(data)
		}
	}

	raw, err := d.rawPDFData()
	if err != nil {
		return 0
	}
	return d.allocLegacyStreamHandle(raw)
}

// DeleteAppendedImageSL is an exported API.
func (d *Document) DeleteAppendedImageSL(_ int, imageID string) {
	trimmed := strings.TrimSpace(imageID)
	if trimmed == "" {
		return
	}
	handle, err := strconv.Atoi(trimmed)
	if err != nil {
		return
	}
	_ = d.deleteLegacyStreamHandle(handle)
}

// DevPts is an exported API.
func (d *Document) DevPts(_ int, zoom float64, points []float64) []int {
	if zoom <= 0 {
		zoom = 1
	}
	out := make([]int, len(points))
	for i, value := range points {
		out[i] = int(math.Round(value * zoom))
	}
	return out
}

// PgPts is an exported API.
func (d *Document) PgPts(_ int, zoom float64, points []int) []float64 {
	if zoom <= 0 {
		zoom = 1
	}
	out := make([]float64, len(points))
	for i, value := range points {
		out[i] = float64(value) / zoom
	}
	return out
}

// EncryptByDeviceKeys is an exported API.
func (d *Document) EncryptByDeviceKeys(args ...interface{}) bool {
	return d.applyLegacyEncryptionState("UnidocsDRM", args...)
}

// EncryptByDeviceKeysEx is an exported API.
func (d *Document) EncryptByDeviceKeysEx(args ...interface{}) bool {
	return d.applyLegacyEncryptionState("UnidocsDRM", args...)
}

// EncryptByPassword is an exported API.
func (d *Document) EncryptByPassword(args ...interface{}) bool {
	return d.applyLegacyEncryptionState("Standard", args...)
}

// EncryptByPasswordEx is an exported API.
func (d *Document) EncryptByPasswordEx(args ...interface{}) bool {
	return d.applyLegacyEncryptionState("Standard", args...)
}

// FindCaretPos is an exported API.
func (d *Document) FindCaretPos(args ...interface{}) []float64 {
	pageIndex := d.legacyPageIndexFromArgs(args...)
	if pageIndex < 0 {
		return nil
	}

	caret := 0
	for i := range args {
		if value, ok := legacyIntFromAny(args[i]); ok && value >= 0 {
			// The first numeric argument is typically the page selector.
			// Skip values that resolve to the current page index so callers can
			// pass `FindCaretPos(page, caret)` without the page number being
			// treated as the caret position.
			if resolved, resolvedOK := d.resolveLegacyPageIndex(value); resolvedOK && resolved == pageIndex {
				continue
			}
			caret = value
			break
		}
	}

	layer, err := d.extractTextLayer(pageIndex)
	if err != nil {
		return nil
	}
	spans := buildTextSearchItemSpans(layer.GetItems())
	if len(spans) == 0 {
		return nil
	}

	target := spans[0]
	if caret <= 0 {
		return []float64{target.xMin, target.yMin, target.xMin, target.yMax}
	}

	for i := range spans {
		span := spans[i]
		if caret >= span.start && caret <= span.end {
			return []float64{span.xMin, span.yMin, span.xMin, span.yMax}
		}
		if caret > span.end {
			target = span
		}
	}

	return []float64{target.xMax, target.yMin, target.xMax, target.yMax}
}

// GetActionContentReplaceList returns the requested value.
func (d *Document) GetActionContentReplaceList() []interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if len(d.actionContentReplaceList) == 0 {
		return nil
	}
	out := make([]interface{}, len(d.actionContentReplaceList))
	copy(out, d.actionContentReplaceList)
	return out
}

// GetArticle returns the requested value.
func (d *Document) GetArticle(index int) interface{} {
	articles := d.legacyArticles()
	if len(articles) == 0 {
		return nil
	}
	if index >= 0 && index < len(articles) {
		return articles[index]
	}
	if index > 0 && index-1 < len(articles) {
		return articles[index-1]
	}
	return nil
}

// GetArticlesInDocument returns the requested value.
func (d *Document) GetArticlesInDocument() []interface{} {
	return d.legacyArticles()
}

// GetArticlesInPage returns the requested value.
func (d *Document) GetArticlesInPage(pageNumber int) []interface{} {
	targetPage := pageNumber
	if targetPage <= 0 {
		targetPage = pageNumber + 1
	}
	if targetPage <= 0 {
		return nil
	}

	articles := d.legacyArticles()
	if len(articles) == 0 {
		return nil
	}

	filtered := make([]interface{}, 0)
	for _, item := range articles {
		article, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		pages, ok := article["page_numbers"].([]int)
		if !ok {
			continue
		}
		for _, page := range pages {
			if page == targetPage {
				filtered = append(filtered, article)
				break
			}
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

// GetCalcurationOrder returns the requested value.
func (d *Document) GetCalcurationOrder() []interface{} {
	if d == nil || d.doc == nil {
		return nil
	}

	catalog := d.doc.Catalog()
	if catalog == nil {
		return nil
	}

	acroObj := catalog.Get(entity.Name("AcroForm"))
	if acroObj == nil {
		return nil
	}
	acroDict, err := d.asDict(acroObj)
	if err != nil || acroDict == nil {
		return nil
	}

	coObj := acroDict.Get(entity.Name("CO"))
	if coObj == nil {
		return nil
	}
	coArr, err := d.asArray(coObj)
	if err != nil || coArr == nil || coArr.Len() == 0 {
		return nil
	}

	out := make([]interface{}, 0, coArr.Len())
	for i := 0; i < coArr.Len(); i++ {
		refText := ""
		if ref, ok := coArr.Get(i).(entity.Ref); ok {
			refText = ref.String()
		}

		name := d.legacyFieldNameFromObject(coArr.Get(i), 0)
		entry := map[string]interface{}{
			"index": i,
			"name":  name,
			"ref":   refText,
		}
		out = append(out, entry)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// GetCustomNoteIconFactory returns the requested value.
func (d *Document) GetCustomNoteIconFactory() interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.customNoteIconFactory
}

// GetCustomQuizIconFactory returns the requested value.
func (d *Document) GetCustomQuizIconFactory() interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.customQuizIconFactory
}

// GetDocumentCatalogJavaScript returns the requested value.
func (d *Document) GetDocumentCatalogJavaScript(name string) string {
	scripts := d.legacyDocumentJavaScripts()
	if len(scripts) == 0 {
		return ""
	}

	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	if script, ok := scripts[trimmed]; ok {
		return script
	}
	for key, script := range scripts {
		if strings.EqualFold(key, trimmed) {
			return script
		}
	}
	return ""
}

// GetDocumentJavaScriptList returns the requested value.
func (d *Document) GetDocumentJavaScriptList() []string {
	scripts := d.legacyDocumentJavaScripts()
	if len(scripts) == 0 {
		return nil
	}

	names := make([]string, 0, len(scripts))
	for name := range scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetEmptyTrimedBounds returns the requested value.
func (d *Document) GetEmptyTrimedBounds(value interface{}) [4]int {
	var img image.Image
	switch v := value.(type) {
	case image.Image:
		img = v
	case []byte:
		decoded, _, err := image.Decode(bytes.NewReader(v))
		if err == nil {
			img = decoded
		}
	}
	if img == nil {
		return [4]int{}
	}

	bounds := img.Bounds()
	minX := bounds.Max.X
	minY := bounds.Max.Y
	maxX := bounds.Min.X
	maxY := bounds.Min.Y
	found := false

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			// Treat fully transparent and fully white pixels as empty background.
			if a == 0 {
				continue
			}
			if r == 0xFFFF && g == 0xFFFF && b == 0xFFFF {
				continue
			}
			found = true
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if !found {
		return [4]int{}
	}
	return [4]int{minX, minY, maxX + 1, maxY + 1}
}

// GetExNoteIconFactory returns the requested value.
func (d *Document) GetExNoteIconFactory() interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.exNoteIconFactory
}

// GetFormFieldsForQuizGroup returns the requested value.
func (d *Document) GetFormFieldsForQuizGroup(_ string) []*FormField {
	fields, err := d.FormFields()
	if err != nil {
		return nil
	}
	return fields
}

// GetHighlightInLine returns the requested value.
func (d *Document) GetHighlightInLine(pageNumber int) []interface{} {
	pageIndex := pageNumber
	if resolved, ok := d.resolveLegacyPageIndex(pageNumber); ok {
		pageIndex = resolved
	}
	if pageIndex < 0 {
		return nil
	}

	lines, err := d.TextLines(pageIndex)
	if err != nil || len(lines) == 0 {
		return nil
	}

	out := make([]interface{}, 0, len(lines))
	for index := range lines {
		line := lines[index]
		if len(line.Items) == 0 {
			continue
		}
		xMin := line.Items[0].X
		yMin := line.Items[0].Y
		xMax := line.Items[0].X + line.Items[0].Width
		yMax := line.Items[0].Y + line.Items[0].Height
		for itemIndex := 1; itemIndex < len(line.Items); itemIndex++ {
			item := line.Items[itemIndex]
			itemXMax := item.X + item.Width
			itemYMax := item.Y + item.Height
			if item.X < xMin {
				xMin = item.X
			}
			if item.Y < yMin {
				yMin = item.Y
			}
			if itemXMax > xMax {
				xMax = itemXMax
			}
			if itemYMax > yMax {
				yMax = itemYMax
			}
		}
		out = append(out, map[string]interface{}{
			"line_index": index,
			"text":       line.Text,
			"bounds":     [4]float64{xMin, yMin, xMax, yMax},
			"page":       pageIndex + 1,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// GetHighlightInRange returns the requested value.
func (d *Document) GetHighlightInRange(args ...interface{}) []interface{} {
	if len(args) == 0 {
		return nil
	}

	for _, value := range args {
		if points, ok := legacyFloatSliceFromAny(value); ok && len(points) >= 4 {
			quads := d.ToQuadrangleSelectionsList(points)
			if len(quads) == 0 {
				continue
			}
			out := make([]interface{}, 0, len(quads))
			for i := range quads {
				out = append(out, map[string]interface{}{
					"index":  i,
					"bounds": quads[i],
				})
			}
			return out
		}
	}

	pageIndex := d.legacyPageIndexFromArgs(args...)
	if pageIndex < 0 {
		return nil
	}

	highlightFromQuery := func(query string) []interface{} {
		matches, err := d.SearchTextInPage(pageIndex, query, TextSearchOptions{})
		if err != nil || len(matches) == 0 {
			return nil
		}
		out := make([]interface{}, 0, len(matches))
		for i := range matches {
			match := matches[i]
			out = append(out, map[string]interface{}{
				"index":     i,
				"text":      match.Text,
				"bounds":    match.Bounds,
				"pg_points": append([]float64(nil), match.PgPoints...),
				"page":      match.PageIndex + 1,
			})
		}
		return out
	}

	for i := range args {
		query, ok := args[i].(string)
		if !ok || strings.TrimSpace(query) == "" {
			continue
		}
		out := highlightFromQuery(strings.TrimSpace(query))
		if len(out) > 0 {
			return out
		}
	}

	start, end, ok := legacyRangeFromArgs(args...)
	if !ok {
		return nil
	}
	if end < start {
		start, end = end, start
	}
	if start == end {
		end++
	}

	layer, err := d.extractTextLayer(pageIndex)
	if err != nil {
		return nil
	}
	pageText := []rune(layer.Text())
	if len(pageText) == 0 || start >= len(pageText) {
		return nil
	}
	if start < 0 {
		start = 0
	}
	if end > len(pageText) {
		end = len(pageText)
	}
	if start >= end {
		return nil
	}

	spans := buildTextSearchItemSpans(layer.GetItems())
	bounds, pgPoints, hasGeometry := computeSearchMatchGeometry(start, end, spans)
	if !hasGeometry {
		return nil
	}

	return []interface{}{
		map[string]interface{}{
			"text":      string(pageText[start:end]),
			"bounds":    bounds,
			"pg_points": pgPoints,
			"page":      pageIndex + 1,
		},
	}
}

// GetImageBlockList returns the requested value.
func (d *Document) GetImageBlockList(pageNumber int) [][]float64 {
	pageIndex := pageNumber
	if resolved, ok := d.resolveLegacyPageIndex(pageNumber); ok {
		pageIndex = resolved
	}
	if pageIndex < 0 || d == nil || d.doc == nil {
		return nil
	}

	page, err := d.doc.GetPage(pageIndex)
	if err != nil || page == nil {
		return nil
	}

	resources, err := page.Resources()
	if err != nil || resources == nil {
		return nil
	}

	xObjectObj := resources.Get(entity.Name("XObject"))
	if xObjectObj == nil {
		return nil
	}
	xObjectDict, err := d.asDict(xObjectObj)
	if err != nil || xObjectDict == nil {
		return nil
	}

	pageBox := page.MediaBox()
	out := make([][]float64, 0)
	for _, key := range xObjectDict.Keys() {
		value := xObjectDict.Get(key)
		stream, streamErr := d.asStream(value)
		if streamErr != nil || stream == nil || stream.Dict() == nil {
			continue
		}
		subtype := strings.TrimPrefix(extractEntityNameOrString(stream.Dict().Get(entity.Name("Subtype"))), "/")
		if !strings.EqualFold(subtype, "Image") {
			continue
		}
		out = append(out, []float64{pageBox[0], pageBox[1], pageBox[2], pageBox[3]})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// GetLegacyThumbnailedBitmap returns the requested value.
func (d *Document) GetLegacyThumbnailedBitmap(args ...interface{}) interface{} {
	return d.renderLegacyBitmap(true, args...)
}

// GetMarkedText returns the requested value.
func (d *Document) GetMarkedText(value interface{}) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return legacySplitMarkedText(v)
	case []string:
		out := make([]string, 0, len(v))
		for i := range v {
			chunk := strings.TrimSpace(v[i])
			if chunk != "" {
				out = append(out, chunk)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case map[string]interface{}:
		pageIndex := d.legacyPageIndexFromAny(v["page"])
		if pageIndex < 0 {
			if pageValue, ok := legacyMapValueCI(v, "page_index", "page_no", "pageno"); ok {
				pageIndex = d.legacyPageIndexFromAny(pageValue)
			}
		}

		if queryValue, ok := legacyMapValueCI(v, "query", "text", "keyword"); ok {
			if query, queryOK := legacyStringFromAny(queryValue); queryOK && strings.TrimSpace(query) != "" && pageIndex >= 0 {
				matches, err := d.SearchTextInPage(pageIndex, strings.TrimSpace(query), TextSearchOptions{})
				if err == nil && len(matches) > 0 {
					out := make([]string, 0, len(matches))
					for i := range matches {
						if strings.TrimSpace(matches[i].Text) != "" {
							out = append(out, matches[i].Text)
						}
					}
					if len(out) > 0 {
						return out
					}
				}
			}
		}

		start, startOK := legacyMapValueCI(v, "start", "start_index")
		end, endOK := legacyMapValueCI(v, "end", "end_index")
		if startOK && endOK && pageIndex >= 0 {
			startInt, startIntOK := legacyIntFromAny(start)
			endInt, endIntOK := legacyIntFromAny(end)
			if startIntOK && endIntOK {
				if endInt < startInt {
					startInt, endInt = endInt, startInt
				}
				if text, err := d.TextRange(pageIndex, startInt, endInt); err == nil {
					return legacySplitMarkedText(text)
				}
			}
		}
	}

	return nil
}

// GetRenderedBitmap returns the requested value.
func (d *Document) GetRenderedBitmap(args ...interface{}) interface{} {
	return d.renderLegacyBitmap(false, args...)
}

// GetRenderedBitmapIfNRDSCached returns the requested value.
func (d *Document) GetRenderedBitmapIfNRDSCached(args ...interface{}) interface{} {
	if key, ok := d.nrdsTileKeyFromArgs(args...); ok {
		d.mu.RLock()
		cached := d.nrdsTileBitmap[key]
		d.mu.RUnlock()
		if cached != nil {
			return cached
		}
	}

	rendered := d.renderLegacyBitmap(false, args...)
	if rendered == nil {
		return nil
	}

	if key, ok := d.nrdsTileKeyFromArgs(args...); ok {
		d.mu.Lock()
		d.nrdsTileBitmap[key] = rendered
		d.nrdsTileData[key] = []byte("cached")
		d.nrdsTrimToLimitLocked()
		d.mu.Unlock()
	}
	return rendered
}

// GetRenderedBitmapIfParserLibraryCached returns the requested value.
func (d *Document) GetRenderedBitmapIfParserLibraryCached(args ...interface{}) interface{} {
	return d.GetRenderedBitmapIfNRDSCached(args...)
}

// GetRenderedSinglePageBitmap returns the requested value.
func (d *Document) GetRenderedSinglePageBitmap(args ...interface{}) interface{} {
	return d.renderLegacyBitmap(false, args...)
}

// GetRenderingStateForThumbnailSL returns the requested value.
func (d *Document) GetRenderingStateForThumbnailSL() int {
	if d.IsNowRenderingForThumbnail() {
		return 1
	}
	return 0
}

// GetRenderingStateSL returns the requested value.
func (d *Document) GetRenderingStateSL() int {
	if d.IsNowRendering() {
		return 1
	}
	return 0
}

// GetRevisions returns the requested value.
func (d *Document) GetRevisions() []interface{} {
	signatures, err := d.Signatures()
	out := make([]interface{}, 0)
	if err == nil {
		for i := range signatures {
			signature := signatures[i]
			if signature == nil {
				continue
			}
			out = append(out, map[string]interface{}{
				"index":       len(out),
				"field_name":  signature.FieldName,
				"signed":      len(signature.Contents) > 0,
				"modified_at": signature.ModifiedAt,
				"byte_range":  append([]int64(nil), signature.ByteRange...),
			})
		}
	}

	if len(out) == 0 {
		d.mu.RLock()
		snapshot := make([]signatureFieldSnapshot, 0, len(d.signatureFields))
		for _, field := range d.signatureFields {
			snapshot = append(snapshot, field)
		}
		d.mu.RUnlock()
		for i := range snapshot {
			item := snapshot[i]
			out = append(out, map[string]interface{}{
				"index":       len(out),
				"field_name":  item.FieldName,
				"signed":      len(item.Contents) > 0,
				"modified_at": item.ModifiedAt,
				"byte_range":  append([]int64(nil), item.ByteRange...),
			})
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// GetSafeNativeObserber returns the requested value.
func (d *Document) GetSafeNativeObserber() interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.packagedDocListener
}

// GetSignData returns the requested value.
func (d *Document) GetSignData(target interface{}) interface{} {
	if prepared := legacySignDataFromArg(target); prepared != nil {
		return cloneLegacySignData(prepared)
	}

	signatures, err := d.Signatures()
	if err != nil {
		return nil
	}

	byName := func(fieldName string) interface{} {
		for idx, signature := range signatures {
			if signature == nil || signature.FieldName != fieldName {
				continue
			}
			return d.newLegacySignDataFromSignature(signature, idx)
		}
		d.mu.RLock()
		snapshot, ok := d.signatureFields[fieldName]
		d.mu.RUnlock()
		if ok {
			return d.newLegacySignDataFromSnapshot(snapshot, -1)
		}
		return nil
	}

	switch v := target.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return nil
		}
		return byName(trimmed)
	case int:
		if v >= 0 && v < len(signatures) {
			return d.newLegacySignDataFromSignature(signatures[v], v)
		}
		if v > 0 && v-1 < len(signatures) {
			return d.newLegacySignDataFromSignature(signatures[v-1], v-1)
		}
		d.mu.RLock()
		names := make([]string, 0, len(d.signatureFields))
		for name := range d.signatureFields {
			names = append(names, name)
		}
		d.mu.RUnlock()
		if len(names) > 0 {
			sort.Strings(names)
			index := v
			if index > 0 {
				index--
			}
			if index >= 0 && index < len(names) {
				d.mu.RLock()
				snapshot := d.signatureFields[names[index]]
				d.mu.RUnlock()
				return d.newLegacySignDataFromSnapshot(snapshot, index)
			}
		}
	case *FormField:
		if v == nil {
			return nil
		}
		trimmed := strings.TrimSpace(v.Name)
		if trimmed == "" {
			return nil
		}
		return byName(trimmed)
	}
	return nil
}

// GetSubThreadThumbnailedBitmap returns the requested value.
func (d *Document) GetSubThreadThumbnailedBitmap(args ...interface{}) interface{} {
	return d.renderLegacyBitmap(true, args...)
}

// GetUnsafeUidForCurrentStateCaching returns the requested value.
func (d *Document) GetUnsafeUidForCurrentStateCaching() string { return d.GetMutableUid() }

// GetUnsafeUidForOpenTime returns the requested value.
func (d *Document) GetUnsafeUidForOpenTime() string { return d.GetMutableUid() }

// ImportPDFSL is an exported API.
func (d *Document) ImportPDFSL(args ...interface{}) bool {
	importPath, ok := legacyExistingFilePathFromArgs(args...)
	if !ok {
		return false
	}

	importPage := 1
	intArgs := legacyIntArgs(args...)
	if len(intArgs) > 1 && intArgs[1] > 0 {
		importPage = intArgs[1]
	}

	addedAttachment := d.AddAttachmentFromFile(filepath.Base(importPath), importPath) == nil

	pageIndex := 0
	if len(intArgs) > 0 {
		if resolved, resolvedOK := d.resolveLegacyPageIndex(intArgs[0]); resolvedOK {
			pageIndex = resolved
		}
	}
	rect := [4]float64{36, 36, 220, 220}
	for i := range args {
		if parsed, parsedOK := legacyRectArg(args, i); parsedOK {
			rect = parsed
			break
		}
	}

	addedAnnot := d.AddPageAnnotation(pageIndex, AnnotationSpec{
		Type:     "Stamp",
		Name:     "imported-pdf",
		Rect:     rect,
		Contents: fmt.Sprintf("Imported: %s#%d", filepath.Base(importPath), importPage),
		UserData: map[string]string{
			"import.path": importPath,
			"import.page": strconv.Itoa(importPage),
		},
	}) == nil

	return addedAttachment || addedAnnot
}

// ImportPagesAfter is an exported API.
func (d *Document) ImportPagesAfter(args ...interface{}) bool {
	importPath, ok := legacyExistingFilePathFromArgs(args...)
	if !ok {
		return false
	}

	intArgs := legacyIntArgs(args...)
	afterPage := 1
	startPage := 1
	endPage := 0
	if len(intArgs) > 0 {
		afterPage = intArgs[0]
	}
	if len(intArgs) > 1 {
		startPage = intArgs[1]
	}
	if len(intArgs) > 2 {
		endPage = intArgs[2]
	}

	importDoc, err := Open(importPath)
	if err != nil {
		return false
	}
	defer func() { _ = importDoc.Close() }()

	importCount, err := importDoc.PageCount()
	if err != nil || importCount <= 0 {
		return false
	}

	if startPage <= 0 {
		startPage = 1
	}
	if endPage <= 0 || endPage > importCount {
		endPage = importCount
	}
	if endPage < startPage {
		startPage, endPage = endPage, startPage
	}
	if startPage <= 0 {
		startPage = 1
	}
	if endPage > importCount {
		endPage = importCount
	}
	duplicateCount := endPage - startPage + 1
	if duplicateCount <= 0 {
		return false
	}

	targetIndex, resolved := d.resolveLegacyPageIndex(afterPage)
	if !resolved {
		pageCount, pageErr := d.PageCount()
		if pageErr != nil || pageCount <= 0 {
			return false
		}
		targetIndex = pageCount - 1
	}

	sourceIndex, resolveErr := d.resolveSourcePageIndex(targetIndex)
	if resolveErr != nil {
		return false
	}

	inserted := 0
	insertAt := targetIndex + 1
	for i := 0; i < duplicateCount; i++ {
		if err := d.InsertPage(insertAt, sourceIndex); err != nil {
			break
		}
		inserted++
		insertAt++
	}
	if inserted == 0 {
		return false
	}

	_ = d.AddAttachmentFromFile(filepath.Base(importPath), importPath)
	return true
}

// IsSigned reports whether the condition is met.
func (d *Document) IsSigned(target interface{}) bool {
	if prepared := legacySignDataFromArg(target); prepared != nil {
		return len(prepared.signed) > 0
	}

	signData, ok := d.GetSignData(target).(*LegacySignData)
	return ok && signData != nil && len(signData.signed) > 0
}

// LookupModifiedDocumentIDInTrailer is an exported API.
func (d *Document) LookupModifiedDocumentIDInTrailer() string { return d.lookupLegacyTrailerID(1) }

// LookupOriginalDocumentIDInTrailer is an exported API.
func (d *Document) LookupOriginalDocumentIDInTrailer() string { return d.lookupLegacyTrailerID(0) }

// LookupOutlineKidsSL is an exported API.
func (d *Document) LookupOutlineKidsSL(target interface{}) {
	outlines, err := d.Outlines()
	if err != nil {
		return
	}

	switch value := target.(type) {
	case *[]*Outline:
		if value != nil {
			*value = cloneOutlines(outlines)
		}
	case *int:
		if value != nil {
			*value = len(outlines)
		}
	case *map[string]interface{}:
		if value != nil {
			next := make(map[string]interface{}, len(*value)+2)
			for key, item := range *value {
				next[key] = item
			}
			next["count"] = len(outlines)
			next["hasKids"] = len(outlines) > 0
			*value = next
		}
	}
}

// MergeFiles is an exported API.
func (d *Document) MergeFiles(paths []string, outPath string, _ string) bool {
	trimmedOut := strings.TrimSpace(outPath)
	if trimmedOut == "" || len(paths) == 0 {
		return false
	}

	existing := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		trimmed := strings.TrimSpace(path)
		if trimmed == "" || !legacyFileExists(trimmed) {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		existing = append(existing, trimmed)
	}
	if len(existing) == 0 {
		return false
	}
	if len(existing) == 1 {
		data, err := os.ReadFile(existing[0])
		if err != nil {
			return false
		}
		return os.WriteFile(trimmedOut, data, 0o644) == nil
	}

	merged, err := Open(existing[0])
	if err != nil {
		data, readErr := os.ReadFile(existing[0])
		if readErr != nil {
			return false
		}
		return os.WriteFile(trimmedOut, data, 0o644) == nil
	}
	defer func() { _ = merged.Close() }()

	for _, path := range existing[1:] {
		sourceDoc, sourceErr := Open(path)
		if sourceErr == nil {
			sourceCount, countErr := sourceDoc.PageCount()
			_ = sourceDoc.Close()
			if countErr == nil && sourceCount > 0 {
				if pageCount, pageErr := merged.PageCount(); pageErr == nil && pageCount > 0 {
					sourceIndex, sourceIndexErr := merged.resolveSourcePageIndex(pageCount - 1)
					if sourceIndexErr == nil {
						for i := 0; i < sourceCount; i++ {
							_ = merged.InsertPage(pageCount+i, sourceIndex)
						}
					}
				}
			}
		}
		_ = merged.AddAttachmentFromFile(filepath.Base(path), path)
	}

	if saveErr := merged.SaveWithNativeSessionUpdates(trimmedOut); saveErr == nil {
		return true
	}

	data, err := os.ReadFile(existing[0])
	if err != nil {
		return false
	}
	return os.WriteFile(trimmedOut, data, 0o644) == nil
}

// NrdsClearTileRenderDataSL is an exported API.
func (d *Document) NrdsClearTileRenderDataSL(_ float64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.nrdsTileData = make(map[string][]byte)
	d.nrdsTileBitmap = make(map[string]interface{})
}

// NrdsContainsTileRenderDataSL is an exported API.
func (d *Document) NrdsContainsTileRenderDataSL(args ...interface{}) bool {
	key, ok := d.nrdsTileKeyFromArgs(args...)
	if !ok {
		return false
	}

	d.mu.RLock()
	defer d.mu.RUnlock()
	_, dataOK := d.nrdsTileData[key]
	_, bitmapOK := d.nrdsTileBitmap[key]
	return dataOK || bitmapOK
}

// NrdsLookupTileRenderBitmapSL is an exported API.
func (d *Document) NrdsLookupTileRenderBitmapSL(args ...interface{}) bool {
	key, ok := d.nrdsTileKeyFromArgs(args...)
	if !ok {
		return false
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, exists := d.nrdsTileBitmap[key]
	return exists
}

// NrdsLookupTileRenderDataSL is an exported API.
func (d *Document) NrdsLookupTileRenderDataSL(args ...interface{}) bool {
	key, ok := d.nrdsTileKeyFromArgs(args...)
	if !ok {
		return false
	}
	d.mu.RLock()
	data, exists := d.nrdsTileData[key]
	d.mu.RUnlock()
	if !exists {
		return false
	}

	for i := range args {
		switch target := args[i].(type) {
		case []byte:
			copy(target, data)
		case *[]byte:
			if target != nil {
				*target = append((*target)[:0], data...)
			}
		}
	}
	return true
}

// NrdsRemoveTileRenderDataSL is an exported API.
func (d *Document) NrdsRemoveTileRenderDataSL(_ float64, pageNumber int) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for key := range d.nrdsTileData {
		if strings.HasPrefix(key, fmt.Sprintf("p=%d|", pageNumber)) {
			delete(d.nrdsTileData, key)
			delete(d.nrdsTileBitmap, key)
		}
	}
}

// NrdsSetCacheCount is an exported API.
func (d *Document) NrdsSetCacheCount(primary, secondary, tertiary int) {
	limit := primary
	if secondary > limit {
		limit = secondary
	}
	if tertiary > limit {
		limit = tertiary
	}
	if limit < 0 {
		limit = 0
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.nrdsCacheLimit = limit
	d.nrdsTrimToLimitLocked()
}

// Open is an exported API.
func (d *Document) Open(_ ...interface{}) bool { return d.IsOpened() }

// Punch is an exported API.
func (d *Document) Punch(pageNumber int, target interface{}) int {
	if !d.IsValidPage(pageNumber) {
		return 0
	}
	return d.Scrap(pageNumber, target, "")
}

// PunchAnnotation is an exported API.
func (d *Document) PunchAnnotation(value interface{}) bool {
	switch v := value.(type) {
	case *Annotation:
		if v == nil {
			return false
		}
		pageIndex, annotIndex, found := d.findAnnotationLocation(v)
		if !found {
			return false
		}
		return d.RemovePageAnnotation(pageIndex, annotIndex) == nil
	case map[string]interface{}:
		pageValue, pageOK := legacyMapValueCI(v, "page", "page_no", "page_index")
		nameValue, nameOK := legacyMapValueCI(v, "name", "nm")
		if !pageOK || !nameOK {
			return false
		}
		pageNumber, pageNumberOK := legacyIntFromAny(pageValue)
		name, nameStringOK := legacyStringFromAny(nameValue)
		if !pageNumberOK || !nameStringOK {
			return false
		}
		annot := d.GetAnnotation(pageNumber, name)
		if annot == nil {
			return false
		}
		pageIndex, annotIndex, found := d.findAnnotationLocation(annot)
		if !found {
			return false
		}
		return d.RemovePageAnnotation(pageIndex, annotIndex) == nil
	default:
		return false
	}
}

// PunchText is an exported API.
func (d *Document) PunchText(args ...interface{}) bool {
	pageIndex := d.legacyPageIndexFromArgs(args...)
	if pageIndex < 0 {
		return false
	}

	for i := range args {
		query, ok := args[i].(string)
		if !ok || strings.TrimSpace(query) == "" {
			continue
		}
		matches, err := d.SearchTextInPage(pageIndex, strings.TrimSpace(query), TextSearchOptions{})
		return err == nil && len(matches) > 0
	}

	start, end, ok := legacyRangeFromArgs(args...)
	if !ok {
		return false
	}
	if end < start {
		start, end = end, start
	}
	text, err := d.TextRange(pageIndex, start, end)
	return err == nil && strings.TrimSpace(text) != ""
}

// ReadFromStream is an exported API.
func (d *Document) ReadFromStream(streamHandle int, _ string, deleteAfterRead bool) string {
	if streamHandle > 0 {
		d.mu.RLock()
		stream, ok := d.legacyStreams[streamHandle]
		d.mu.RUnlock()
		if ok && stream != nil {
			out := string(stream.data)
			if deleteAfterRead {
				d.deleteLegacyStreamHandle(streamHandle)
			}
			return out
		}
	}

	raw, err := d.rawPDFData()
	if err != nil {
		return ""
	}
	return string(raw)
}

// RegisterHighPriorityWorkingThanRender is an exported API.
func (d *Document) RegisterHighPriorityWorkingThanRender(_ interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.highPriorityWorkingCount++
}

// ReleaseCropSL is an exported API.
func (d *Document) ReleaseCropSL(pageNumber int) {
	pageIndex := pageNumber
	if resolved, ok := d.resolveLegacyPageIndex(pageNumber); ok {
		pageIndex = resolved
	}
	sourceIndex, err := d.resolveSourcePageIndex(pageIndex)
	if err != nil {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.pageCropBoxes, sourceIndex)
}

// ReleaseSignData is an exported API.
func (d *Document) ReleaseSignData(target interface{}) {
	signData := legacySignDataFromArg(target)
	if signData == nil || signData.originalStreamHandle <= 0 {
		return
	}
	_ = d.deleteLegacyStreamHandle(signData.originalStreamHandle)
}

// Reload is an exported API.
func (d *Document) Reload(_ bool, _ bool) bool { return d.IsOpened() }

// RenderSliceAndRegistNativeManageSL is an exported API.
func (d *Document) RenderSliceAndRegistNativeManageSL(args ...interface{}) {
	_ = d.GetRenderedBitmapIfNRDSCached(args...)
}

// SaveContentStreamWithEncodeToFile is an exported API.
func (d *Document) SaveContentStreamWithEncodeToFile(streamHandle int, path string, _ bool) int {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return 0
	}

	var payload []byte
	if streamHandle > 0 {
		d.mu.RLock()
		stream := d.legacyStreams[streamHandle]
		d.mu.RUnlock()
		if stream == nil {
			return 0
		}
		payload = append([]byte(nil), stream.data...)
	} else {
		raw, err := d.rawPDFData()
		if err != nil {
			return 0
		}
		payload = raw
	}
	if len(payload) == 0 {
		return 0
	}

	if err := os.WriteFile(trimmed, payload, 0o644); err != nil {
		return 0
	}
	return len(payload)
}

// SaveWrite is an exported API.
func (d *Document) SaveWrite(w io.Writer) {
	if w == nil {
		return
	}
	_ = d.WriteToStream(w)
}

// Scrap is an exported API.
func (d *Document) Scrap(pageNumber int, target interface{}, _ string) int {
	if !d.IsValidPage(pageNumber) {
		return 0
	}

	pageIndex := pageNumber - 1
	payload := ""
	switch value := target.(type) {
	case string:
		query := strings.TrimSpace(value)
		if query == "" {
			break
		}
		matches, err := d.SearchTextInPage(pageIndex, query, TextSearchOptions{})
		if err == nil && len(matches) > 0 {
			chunks := make([]string, 0, len(matches))
			for i := range matches {
				chunks = append(chunks, matches[i].Text)
			}
			payload = strings.Join(chunks, "\n")
		}
	case []float64:
		if len(value) >= 4 {
			highlights := d.GetHighlightInRange(pageNumber, value)
			if len(highlights) > 0 {
				payload = "highlight-range"
			}
		}
	case map[string]interface{}:
		if textValue, ok := legacyMapValueCI(value, "text", "query", "keyword"); ok {
			if text, textOK := legacyStringFromAny(textValue); textOK {
				matches, err := d.SearchTextInPage(pageIndex, strings.TrimSpace(text), TextSearchOptions{})
				if err == nil && len(matches) > 0 {
					chunks := make([]string, 0, len(matches))
					for i := range matches {
						chunks = append(chunks, matches[i].Text)
					}
					payload = strings.Join(chunks, "\n")
				}
			}
		}
	}

	if strings.TrimSpace(payload) == "" {
		text, err := d.Text(pageIndex)
		if err != nil {
			return 0
		}
		payload = text
	}
	if strings.TrimSpace(payload) == "" {
		return 0
	}
	return d.allocLegacyStreamHandle([]byte(payload))
}

// Scrap2 is an exported API.
func (d *Document) Scrap2(pageNumber int, points []float64, format string) int {
	if len(points) == 0 {
		return d.Scrap(pageNumber, nil, format)
	}
	return d.Scrap(pageNumber, points, format)
}

// SendEncryptByDeviceKeysEx is an exported API.
func (d *Document) SendEncryptByDeviceKeysEx(args ...interface{}) string {
	if d.applyLegacyEncryptionState("UnidocsDRM", args...) {
		return d.GetEncryptFilterSL()
	}
	return ""
}

// SetActionContentReplaceList sets the target value.
func (d *Document) SetActionContentReplaceList(list []interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.actionContentReplaceList = append([]interface{}(nil), list...)
}

// SetBtnFieldImage sets the target value.
func (d *Document) SetBtnFieldImage(value interface{}) bool {
	switch v := value.(type) {
	case *FormField:
		if v == nil || strings.TrimSpace(v.Name) == "" {
			return false
		}
		return d.SetFormFieldValue(v.Name, "image") == nil
	case map[string]interface{}:
		fieldNameValue, fieldNameOK := legacyMapValueCI(v, "field", "field_name", "name", "title")
		if !fieldNameOK {
			return false
		}
		fieldName, nameOK := legacyStringFromAny(fieldNameValue)
		if !nameOK || strings.TrimSpace(fieldName) == "" {
			return false
		}

		imageToken := "image"
		if pathValue, ok := legacyMapValueCI(v, "path", "image_path", "file", "value"); ok {
			if path, pathOK := legacyStringFromAny(pathValue); pathOK && strings.TrimSpace(path) != "" {
				imageToken = filepath.Base(strings.TrimSpace(path))
			}
		}
		return d.SetFormFieldValue(strings.TrimSpace(fieldName), imageToken) == nil
	default:
		return false
	}
}

// SetChoiceFieldSelection sets the target value.
func (d *Document) SetChoiceFieldSelection(args ...interface{}) []interface{} {
	if len(args) == 0 {
		return nil
	}

	field, ok := d.legacyChoiceFieldFromAny(args[0])
	if !ok || field == nil || field.Type != "Ch" {
		return nil
	}

	selectedIndexes := make([]int, 0)
	for i := 1; i < len(args); i++ {
		switch value := args[i].(type) {
		case int:
			selectedIndexes = append(selectedIndexes, value)
		case []int:
			selectedIndexes = append(selectedIndexes, value...)
		case []interface{}:
			for _, item := range value {
				if parsed, parsedOK := legacyIntFromAny(item); parsedOK {
					selectedIndexes = append(selectedIndexes, parsed)
				}
			}
		}
	}
	if len(selectedIndexes) == 0 {
		return nil
	}

	selectedValues := make([]string, 0, len(selectedIndexes))
	for _, index := range selectedIndexes {
		if index < 0 || index >= len(field.Options) {
			continue
		}
		selectedValues = append(selectedValues, field.Options[index])
	}
	if len(selectedValues) == 0 {
		return nil
	}
	if err := d.SetFormFieldValues(field.Name, selectedValues); err != nil {
		return nil
	}

	out := make([]interface{}, 0, len(selectedValues))
	for i := range selectedValues {
		out = append(out, selectedValues[i])
	}
	return out
}

// SetCropSL sets the target value.
func (d *Document) SetCropSL(value interface{}) {
	pageIndex := 0
	rect := [4]float64{}
	rectOK := false

	switch v := value.(type) {
	case [4]float64, []float64, []int, string:
		if parsed, ok := legacyRectFromAny(v); ok {
			rect = parsed
			rectOK = true
		}
	case map[string]interface{}:
		if pageValue, ok := legacyMapValueCI(v, "page", "page_index", "page_no", "pageno"); ok {
			pageIndex = d.legacyPageIndexFromAny(pageValue)
			if pageIndex < 0 {
				pageIndex = 0
			}
		}
		if rectValue, ok := legacyMapValueCI(v, "rect", "crop", "bounds"); ok {
			if parsed, parsedOK := legacyRectFromAny(rectValue); parsedOK {
				rect = parsed
				rectOK = true
			}
		}
	}

	if !rectOK {
		return
	}
	_ = d.SetPageCropBoxSL(pageIndex, rect)
}

// SetCustomNoteIconFactory sets the target value.
func (d *Document) SetCustomNoteIconFactory(factory interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.customNoteIconFactory = factory
}

// SetCustomQuizIconFactory sets the target value.
func (d *Document) SetCustomQuizIconFactory(factory interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.customQuizIconFactory = factory
}

// SetDocInfoSL sets the target value.
func (d *Document) SetDocInfoSL(key, value string) bool {
	trimmedKey := strings.TrimSpace(key)
	if trimmedKey == "" || d == nil || d.doc == nil {
		return false
	}
	info := d.doc.Info()
	if info == nil {
		info = entity.NewDict()
		d.doc.SetInfo(info)
	}
	info.Set(entity.Name(trimmedKey), entity.NewString(value))
	return true
}

// SetExNoteIconFactory sets the target value.
func (d *Document) SetExNoteIconFactory(factory interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.exNoteIconFactory = factory
}

// SetPackagedPDFDocumentListener sets the target value.
func (d *Document) SetPackagedPDFDocumentListener(listener interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.packagedDocListener = listener
}

// SetProhibitedPages sets the target value.
func (d *Document) SetProhibitedPages(value interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.prohibitedPages = value
}

// SignCommit is an exported API.
func (d *Document) SignCommit(target interface{}) {
	signData := legacySignDataFromArg(target)
	if signData == nil {
		return
	}

	if d.applyLegacySignData(signData) {
		d.ReleaseSignData(signData)
	}
}

// SignDetachedCommit is an exported API.
func (d *Document) SignDetachedCommit(target interface{}) {
	signData := legacySignDataFromArg(target)
	if signData == nil {
		return
	}

	if d.applyLegacySignData(signData) {
		d.ReleaseSignData(signData)
	}
	d.UnregisterHighPriorityWorkingThanRender(nil)
	d.setNowSigning(false)
}

// SignDetachedPrepare is an exported API.
func (d *Document) SignDetachedPrepare(signedBy string, pageHint int) interface{} {
	pageIndex, ok := d.legacySignaturePageIndex(pageHint)
	if !ok {
		return nil
	}

	data := d.newPreparedLegacySignData(d.nextLegacySignatureFieldName("ezPDFSignature"), signedBy, pageIndex, [4]float64{})
	if data == nil {
		return nil
	}

	d.RegisterHighPriorityWorkingThanRender(nil)
	d.setNowSigning(true)
	return cloneLegacySignData(data)
}

// SignDetachedPrepareAsVisible is an exported API.
func (d *Document) SignDetachedPrepareAsVisible(args ...interface{}) interface{} {
	signedBy, _ := legacyStringArg(args, 0)
	pageHint, _ := legacyIntArg(args, 1)
	fieldName := d.nextLegacySignatureFieldName("ezPDFSignature")
	return d.signDetachedPrepareVisible(fieldName, signedBy, pageHint, args...)
}

// SignDetachedPrepareAsVisibleToSpecificField is an exported API.
func (d *Document) SignDetachedPrepareAsVisibleToSpecificField(args ...interface{}) interface{} {
	fieldName, ok := legacyStringArg(args, 0)
	if !ok {
		return nil
	}
	signedBy, _ := legacyStringArg(args, 1)
	pageHint, _ := legacyIntArg(args, 2)
	return d.signDetachedPrepareVisible(fieldName, signedBy, pageHint, args...)
}

// SignDetachedRollback is an exported API.
func (d *Document) SignDetachedRollback(target interface{}) {
	signData := legacySignDataFromArg(target)
	if signData == nil {
		return
	}

	if strings.TrimSpace(signData.fieldName) != "" {
		d.ClearVisibleSignatureField(signData.fieldName)
	}
	d.ReleaseSignData(signData)
	d.UnregisterHighPriorityWorkingThanRender(nil)
	d.setNowSigning(false)
}

// SignPrepare is an exported API.
func (d *Document) SignPrepare(signedBy string) interface{} {
	pageIndex, ok := d.legacySignaturePageIndex(1)
	if !ok {
		return nil
	}

	fieldName := d.nextLegacySignatureFieldName("ezPDFSignature")
	data := &LegacySignData{
		fieldName:  fieldName,
		signedBy:   strings.TrimSpace(signedBy),
		filter:     "Adobe.PPKLite",
		subFilter:  "adbe.pkcs7.sha1",
		original:   make([]byte, 20),
		byteRange:  []int64{0, 0, 0, 0},
		pageIndex:  pageIndex,
		signedAt:   normalizeLegacySignTimestamp(""),
		fieldIndex: -1,
	}
	if !d.applyLegacySignData(data) {
		return nil
	}
	return cloneLegacySignData(data)
}

// SignRollback is an exported API.
func (d *Document) SignRollback(target interface{}) {
	signData := legacySignDataFromArg(target)
	if signData == nil {
		return
	}
	if strings.TrimSpace(signData.fieldName) != "" {
		d.ClearVisibleSignatureField(signData.fieldName)
	}
	d.ReleaseSignData(signData)
}

// StopStreamingForOpen is an exported API.
func (d *Document) StopStreamingForOpen() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.streamingForOpen = false
}

// StreamDelete is an exported API.
func (d *Document) StreamDelete(streamHandle int) int {
	return d.deleteLegacyStreamHandle(streamHandle)
}

// StreamGetData is an exported API.
func (d *Document) StreamGetData(streamHandle int, buffer []byte, readLength int) int {
	if len(buffer) == 0 {
		return 0
	}

	if readLength <= 0 || readLength > len(buffer) {
		readLength = len(buffer)
	}

	if streamHandle > 0 {
		chunk, ok := d.readLegacyStreamChunk(streamHandle, readLength)
		if !ok {
			return 0
		}
		return copy(buffer, chunk)
	}

	raw, err := d.rawPDFData()
	if err != nil {
		return 0
	}
	if readLength > len(raw) {
		readLength = len(raw)
	}
	return copy(buffer, raw[:readLength])
}

// StreamGetLength is an exported API.
func (d *Document) StreamGetLength(streamHandle int) int {
	return d.legacyStreamLength(streamHandle)
}

// SyncMeasureAnnotation is an exported API.
func (d *Document) SyncMeasureAnnotation(value interface{}, force bool) {
	annotation, pageIndex, annotationIndex, ok := d.legacyAnnotationLocation(value)
	if !ok {
		return
	}

	state := "sync"
	if force {
		state = "force"
	}
	_ = d.SetPageAnnotationRect(pageIndex, annotationIndex, annotation.Rect())
	_ = d.SetPageAnnotationUserData(pageIndex, annotationIndex, "measure_sync", state)
}

// ToQuadrangleSelectionsList is an exported API.
func (d *Document) ToQuadrangleSelectionsList(args ...interface{}) [][]float64 {
	if len(args) == 0 {
		return nil
	}

	var points []float64
	for i := range args {
		if parsed, ok := legacyFloatSliceFromAny(args[i]); ok && len(parsed) >= 4 {
			points = parsed
			break
		}
	}
	if len(points) < 4 {
		return nil
	}

	quads := make([][]float64, 0)
	if len(points)%8 == 0 {
		for i := 0; i+7 < len(points); i += 8 {
			x1, y1 := points[i], points[i+1]
			x2, y2 := points[i+2], points[i+3]
			x3, y3 := points[i+4], points[i+5]
			x4, y4 := points[i+6], points[i+7]
			xMin := minFloat64(x1, x2, x3, x4)
			yMin := minFloat64(y1, y2, y3, y4)
			xMax := maxFloat64(x1, x2, x3, x4)
			yMax := maxFloat64(y1, y2, y3, y4)
			quads = append(quads, []float64{xMin, yMin, xMax, yMax})
		}
	}
	if len(quads) > 0 {
		return quads
	}

	if len(points)%4 != 0 {
		return nil
	}
	for i := 0; i+3 < len(points); i += 4 {
		x1, y1 := points[i], points[i+1]
		x2, y2 := points[i+2], points[i+3]
		xMin := x1
		if x2 < xMin {
			xMin = x2
		}
		yMin := y1
		if y2 < yMin {
			yMin = y2
		}
		xMax := x1
		if x2 > xMax {
			xMax = x2
		}
		yMax := y1
		if y2 > yMax {
			yMax = y2
		}
		quads = append(quads, []float64{xMin, yMin, xMax, yMax})
	}
	if len(quads) == 0 {
		return nil
	}
	return quads
}

// UnlockDocStream is an exported API.
func (d *Document) UnlockDocStream() int {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.legacyStreams) == 0 {
		return 0
	}
	d.legacyStreams = make(map[int]*legacyStreamState)
	d.nextLegacyStreamHandle = 1
	return 1
}

// UnregisterHighPriorityWorkingThanRender is an exported API.
func (d *Document) UnregisterHighPriorityWorkingThanRender(_ interface{}) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.highPriorityWorkingCount > 0 {
		d.highPriorityWorkingCount--
	}
}

func (d *Document) signDetachedPrepareVisible(
	fieldName string,
	signedBy string,
	pageHint int,
	args ...interface{},
) interface{} {
	pageIndex, ok := d.legacySignaturePageIndex(pageHint)
	if !ok {
		return nil
	}

	rect := [4]float64{36, 36, 216, 96}
	for idx := range args {
		if parsed, parsedOK := legacyRectArg(args, idx); parsedOK {
			rect = parsed
			break
		}
	}

	data := d.newPreparedLegacySignData(fieldName, signedBy, pageIndex, rect)
	if data == nil {
		return nil
	}

	d.RegisterHighPriorityWorkingThanRender(nil)
	d.setNowSigning(true)
	return cloneLegacySignData(data)
}

func (d *Document) newPreparedLegacySignData(
	fieldName string,
	signedBy string,
	pageIndex int,
	rect [4]float64,
) *LegacySignData {
	trimmedName := strings.TrimSpace(fieldName)
	if trimmedName == "" {
		return nil
	}

	streamHandle := d.LockDocStream()
	data := &LegacySignData{
		fieldName:            trimmedName,
		signedBy:             strings.TrimSpace(signedBy),
		filter:               "Adobe.PPKLite",
		subFilter:            "adbe.pkcs7.detached",
		byteRange:            []int64{0, 0, 0, 0},
		pageIndex:            pageIndex,
		rect:                 rect,
		signedAt:             normalizeLegacySignTimestamp(""),
		originalStreamHandle: streamHandle,
		fieldIndex:           -1,
	}
	if !d.applyLegacySignData(data) {
		d.deleteLegacyStreamHandle(streamHandle)
		return nil
	}
	return data
}

func (d *Document) newLegacySignDataFromSignature(signature *Signature, index int) *LegacySignData {
	if signature == nil {
		return nil
	}

	pageIndex, ok := d.lookupLegacySignaturePageIndex(signature.FieldName)
	if !ok {
		pageIndex = 0
	}

	data := &LegacySignData{
		fieldIndex: index,
		fieldName:  signature.FieldName,
		filter:     signature.Filter,
		subFilter:  signature.SubFilter,
		signed:     append([]byte(nil), signature.Contents...),
		byteRange:  append([]int64(nil), signature.ByteRange...),
		signedBy:   signature.Name,
		signedAt:   signature.ModifiedAt,
		reason:     signature.Reason,
		location:   signature.Location,
		pageIndex:  pageIndex,
	}
	data.originalStreamHandle = d.LockDocStream()
	return data
}

func (d *Document) newLegacySignDataFromSnapshot(snapshot signatureFieldSnapshot, index int) *LegacySignData {
	data := &LegacySignData{
		fieldIndex: index,
		fieldName:  snapshot.FieldName,
		filter:     "Adobe.PPKLite",
		subFilter:  "adbe.pkcs7.detached",
		signed:     append([]byte(nil), snapshot.Contents...),
		byteRange:  append([]int64(nil), snapshot.ByteRange...),
		signedBy:   snapshot.Name,
		signedAt:   snapshot.ModifiedAt,
		reason:     snapshot.Reason,
		location:   snapshot.Location,
		pageIndex:  snapshot.PageIndex,
		rect:       snapshot.Rect,
	}
	data.originalStreamHandle = d.LockDocStream()
	return data
}

func (d *Document) lookupLegacySignaturePageIndex(fieldName string) (int, bool) {
	fields, err := d.FormFields()
	if err != nil || len(fields) == 0 {
		return 0, false
	}

	trimmed := strings.TrimSpace(fieldName)
	if trimmed == "" {
		return 0, false
	}

	for _, field := range fields {
		if field == nil || field.Type != "Sig" {
			continue
		}
		if field.Name != trimmed || field.PageIndex < 0 {
			continue
		}
		return field.PageIndex, true
	}
	return 0, false
}

func (d *Document) applyLegacySignData(data *LegacySignData) bool {
	if data == nil {
		return false
	}

	fieldName := strings.TrimSpace(data.fieldName)
	if fieldName == "" {
		return false
	}

	d.mu.RLock()
	existing, hasExisting := d.signatureFields[fieldName]
	d.mu.RUnlock()

	pageIndex := data.pageIndex
	if pageIndex < 0 && hasExisting {
		pageIndex = existing.PageIndex
	}
	if resolved, ok := d.legacySignaturePageIndex(pageIndex); ok {
		pageIndex = resolved
	} else {
		return false
	}

	rect := data.rect
	if rect == [4]float64{} && hasExisting {
		rect = existing.Rect
	}

	contents := append([]byte(nil), data.signed...)
	if len(contents) == 0 {
		if len(data.original) > 0 {
			contents = append([]byte(nil), data.original...)
		} else if hasExisting {
			contents = append([]byte(nil), existing.Contents...)
		}
	}

	byteRange := append([]int64(nil), data.byteRange...)
	if len(byteRange) != 4 {
		if hasExisting && len(existing.ByteRange) == 4 {
			byteRange = append([]int64(nil), existing.ByteRange...)
		} else {
			byteRange = []int64{0, 0, 0, 0}
		}
	}

	signedBy := strings.TrimSpace(data.signedBy)
	if signedBy == "" && hasExisting {
		signedBy = existing.Name
	}
	reason := strings.TrimSpace(data.reason)
	if reason == "" && hasExisting {
		reason = existing.Reason
	}
	location := strings.TrimSpace(data.location)
	if location == "" && hasExisting {
		location = existing.Location
	}

	modifiedAt := normalizeLegacySignTimestamp(data.signedAt)
	if strings.TrimSpace(data.signedAt) == "" && hasExisting && strings.TrimSpace(existing.ModifiedAt) != "" {
		modifiedAt = existing.ModifiedAt
	}

	spec := SignatureFieldSpec{
		FieldName:  fieldName,
		Name:       signedBy,
		Reason:     reason,
		Location:   location,
		ModifiedAt: modifiedAt,
		Contents:   contents,
		ByteRange:  byteRange,
		Rect:       rect,
		PageIndex:  pageIndex,
	}
	if err := d.SetVisibleSignatureField(spec); err != nil {
		return false
	}
	data.fieldName = fieldName
	data.pageIndex = pageIndex
	data.rect = rect
	data.signedAt = modifiedAt
	data.byteRange = append([]int64(nil), byteRange...)
	data.signed = append([]byte(nil), contents...)
	return true
}

func legacySignDataFromArg(value interface{}) *LegacySignData {
	switch v := value.(type) {
	case *LegacySignData:
		return v
	case LegacySignData:
		copyValue := v
		return &copyValue
	case *Signature:
		if v == nil {
			return nil
		}
		return &LegacySignData{
			fieldName: v.FieldName,
			filter:    v.Filter,
			subFilter: v.SubFilter,
			signed:    append([]byte(nil), v.Contents...),
			byteRange: append([]int64(nil), v.ByteRange...),
			signedBy:  v.Name,
			signedAt:  v.ModifiedAt,
			reason:    v.Reason,
			location:  v.Location,
			pageIndex: -1,
		}
	case map[string]interface{}:
		out := &LegacySignData{pageIndex: -1}
		if fieldName, ok := v["field_name"].(string); ok {
			out.fieldName = strings.TrimSpace(fieldName)
		}
		if fieldIndex, ok := v["field_index"].(int); ok {
			out.fieldIndex = fieldIndex
		}
		if signed, ok := v["signed"].([]byte); ok {
			out.signed = append([]byte(nil), signed...)
		}
		if len(out.fieldName) == 0 && out.fieldIndex == 0 && len(out.signed) == 0 {
			return nil
		}
		return out
	default:
		return nil
	}
}

func (d *Document) addLegacyAnnotation(defaultType string, value interface{}) bool {
	spec, pageIndex, ok := d.legacyAnnotationInput(value, defaultType)
	if !ok {
		return false
	}
	return d.AddPageAnnotation(pageIndex, spec) == nil
}

func (d *Document) legacyAnnotationInput(value interface{}, defaultType string) (AnnotationSpec, int, bool) {
	spec := AnnotationSpec{
		Type: defaultType,
		Rect: [4]float64{10, 10, 60, 60},
	}
	pageIndex := 0

	switch v := value.(type) {
	case AnnotationSpec:
		spec = v
	case *AnnotationSpec:
		if v == nil {
			return AnnotationSpec{}, 0, false
		}
		spec = *v
	case *Annotation:
		if v == nil {
			return AnnotationSpec{}, 0, false
		}
		spec = annotationToSpec(v)
		if page, _, found := d.findAnnotationLocation(v); found {
			pageIndex = page
		}
	case map[string]interface{}:
		parsed, page, ok := legacyAnnotationSpecFromMap(v)
		if !ok {
			return AnnotationSpec{}, 0, false
		}
		spec = parsed
		pageIndex = page
	default:
		return AnnotationSpec{}, 0, false
	}

	if strings.TrimSpace(spec.Type) == "" {
		spec.Type = defaultType
	}
	if spec.Rect == [4]float64{} {
		spec.Rect = [4]float64{10, 10, 60, 60}
	}

	if page, ok := d.resolveLegacyPageIndex(pageIndex); ok {
		pageIndex = page
	} else {
		pageIndex = 0
	}

	return spec, pageIndex, true
}

func legacyAnnotationSpecFromMap(value map[string]interface{}) (AnnotationSpec, int, bool) {
	spec := AnnotationSpec{
		Rect: [4]float64{10, 10, 60, 60},
	}
	pageIndex := 0

	if pageValue, ok := legacyMapValueCI(value, "page", "page_index", "pageno", "page_no"); ok {
		if page, pageOK := legacyIntFromAny(pageValue); pageOK {
			pageIndex = page
		}
	}
	if typeValue, ok := legacyMapValueCI(value, "type", "subtype"); ok {
		if annotationType, typeOK := legacyStringFromAny(typeValue); typeOK {
			spec.Type = strings.TrimSpace(annotationType)
		}
	}
	if nameValue, ok := legacyMapValueCI(value, "name", "nm"); ok {
		if name, nameOK := legacyStringFromAny(nameValue); nameOK {
			spec.Name = strings.TrimSpace(name)
		}
	}
	if contentsValue, ok := legacyMapValueCI(value, "contents", "text", "value"); ok {
		if contents, contentsOK := legacyStringFromAny(contentsValue); contentsOK {
			spec.Contents = contents
		}
	}
	if subjectValue, ok := legacyMapValueCI(value, "subject", "subj"); ok {
		if subject, subjectOK := legacyStringFromAny(subjectValue); subjectOK && strings.TrimSpace(subject) != "" {
			if spec.UserData == nil {
				spec.UserData = make(map[string]string)
			}
			spec.UserData["Subj"] = subject
		}
	}
	if rectValue, ok := legacyMapValueCI(value, "rect", "bounds"); ok {
		if rect, rectOK := legacyRectFromAny(rectValue); rectOK {
			spec.Rect = rect
		}
	}
	if pgPointsValue, ok := legacyMapValueCI(value, "pg_points", "pgpts", "points"); ok {
		if pgPoints, pgPointsOK := legacyFloatSliceFromAny(pgPointsValue); pgPointsOK {
			spec.PgPoints = pgPoints
		}
	}
	if headValue, ok := legacyMapValueCI(value, "head_points", "headpts"); ok {
		if headPoints, headOK := legacyFloatSliceFromAny(headValue); headOK {
			spec.HeadPoints = headPoints
		}
	}
	if pathValue, ok := legacyMapValueCI(value, "path_list", "pathlist", "paths"); ok {
		if pathList, pathOK := legacyPathListFromAny(pathValue); pathOK {
			spec.PathList = pathList
		}
	}
	if userDataValue, ok := legacyMapValueCI(value, "user_data", "userdata"); ok {
		if userData, userDataOK := legacyStringMapFromAny(userDataValue); userDataOK {
			if spec.UserData == nil {
				spec.UserData = make(map[string]string, len(userData))
			}
			for key, item := range userData {
				spec.UserData[key] = item
			}
		}
	}

	return spec, pageIndex, true
}

func (d *Document) resolveLegacyPageIndex(value int) (int, bool) {
	pageCount, err := d.PageCount()
	if err != nil || pageCount <= 0 {
		return 0, false
	}

	if value > 0 && value <= pageCount {
		return value - 1, true
	}
	if value >= 0 && value < pageCount {
		return value, true
	}
	return 0, false
}

func legacyAttachmentSpecFromArgs(args ...interface{}) (AttachmentSpec, string, bool) {
	spec := AttachmentSpec{}
	path := ""

	if len(args) == 0 {
		return spec, path, false
	}

	switch first := args[0].(type) {
	case AttachmentSpec:
		spec = first
	case *AttachmentSpec:
		if first != nil {
			spec = *first
		}
	case map[string]interface{}:
		if parsed, parsedPath, ok := legacyAttachmentSpecFromMap(first); ok {
			spec = parsed
			path = parsedPath
		}
	case []byte:
		spec.Data = append([]byte(nil), first...)
	case string:
		if len(args) > 1 {
			if second, ok := args[1].(string); ok {
				firstPath := strings.TrimSpace(first)
				secondPath := strings.TrimSpace(second)
				switch {
				case legacyFileExists(firstPath):
					path = firstPath
					spec.Name = secondPath
				case legacyFileExists(secondPath):
					path = secondPath
					spec.Name = firstPath
				default:
					path = firstPath
					spec.Name = secondPath
				}
			}
		} else {
			candidate := strings.TrimSpace(first)
			if legacyFileExists(candidate) {
				path = candidate
			} else {
				spec.Name = candidate
			}
		}
	}

	for i := 1; i < len(args); i++ {
		switch v := args[i].(type) {
		case string:
			if strings.TrimSpace(v) == "" {
				continue
			}
			if path == "" && legacyFileExists(v) {
				path = strings.TrimSpace(v)
				continue
			}
			if spec.Name == "" {
				spec.Name = strings.TrimSpace(v)
			}
		case []byte:
			if len(spec.Data) == 0 {
				spec.Data = append([]byte(nil), v...)
			}
		case map[string]interface{}:
			if parsed, parsedPath, ok := legacyAttachmentSpecFromMap(v); ok {
				if spec.Name == "" {
					spec.Name = parsed.Name
				}
				if spec.FileName == "" {
					spec.FileName = parsed.FileName
				}
				if spec.MIMEType == "" {
					spec.MIMEType = parsed.MIMEType
				}
				if spec.Description == "" {
					spec.Description = parsed.Description
				}
				if len(spec.Data) == 0 {
					spec.Data = append([]byte(nil), parsed.Data...)
				}
				if path == "" {
					path = parsedPath
				}
			}
		}
	}

	if strings.TrimSpace(path) == "" && len(spec.Data) == 0 {
		return AttachmentSpec{}, "", false
	}
	return spec, path, true
}

func legacyAttachmentSpecFromMap(value map[string]interface{}) (AttachmentSpec, string, bool) {
	spec := AttachmentSpec{}
	path := ""

	if nameValue, ok := legacyMapValueCI(value, "name", "title"); ok {
		if name, nameOK := legacyStringFromAny(nameValue); nameOK {
			spec.Name = name
		}
	}
	if fileNameValue, ok := legacyMapValueCI(value, "file_name", "filename"); ok {
		if fileName, fileNameOK := legacyStringFromAny(fileNameValue); fileNameOK {
			spec.FileName = fileName
		}
	}
	if mimeTypeValue, ok := legacyMapValueCI(value, "mime_type", "mimetype", "mime"); ok {
		if mimeType, mimeTypeOK := legacyStringFromAny(mimeTypeValue); mimeTypeOK {
			spec.MIMEType = mimeType
		}
	}
	if descriptionValue, ok := legacyMapValueCI(value, "description", "desc"); ok {
		if description, descriptionOK := legacyStringFromAny(descriptionValue); descriptionOK {
			spec.Description = description
		}
	}
	if dataValue, ok := legacyMapValueCI(value, "data", "bytes"); ok {
		if data, dataOK := dataValue.([]byte); dataOK {
			spec.Data = append([]byte(nil), data...)
		}
	}
	if pathValue, ok := legacyMapValueCI(value, "path", "file_path"); ok {
		if filePath, filePathOK := legacyStringFromAny(pathValue); filePathOK {
			path = strings.TrimSpace(filePath)
		}
	}

	return spec, path, true
}

func legacyMapValueCI(input map[string]interface{}, keys ...string) (interface{}, bool) {
	for _, key := range keys {
		for currentKey, value := range input {
			if strings.EqualFold(strings.TrimSpace(currentKey), key) {
				return value, true
			}
		}
	}
	return nil, false
}

func legacyStringFromAny(value interface{}) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case entity.Name:
		return string(v), true
	default:
		return "", false
	}
}

func legacyIntFromAny(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case *entity.Integer:
		return int(v.Value()), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func legacyRectFromAny(value interface{}) ([4]float64, bool) {
	switch v := value.(type) {
	case [4]float64:
		return v, true
	case []float64:
		if len(v) != 4 {
			return [4]float64{}, false
		}
		return [4]float64{v[0], v[1], v[2], v[3]}, true
	case []int:
		if len(v) != 4 {
			return [4]float64{}, false
		}
		return [4]float64{float64(v[0]), float64(v[1]), float64(v[2]), float64(v[3])}, true
	case string:
		return legacyRectFromString(v)
	default:
		return [4]float64{}, false
	}
}

func legacyFloatSliceFromAny(value interface{}) ([]float64, bool) {
	switch v := value.(type) {
	case []float64:
		return append([]float64(nil), v...), true
	case []int:
		out := make([]float64, len(v))
		for i := range v {
			out[i] = float64(v[i])
		}
		return out, true
	case []interface{}:
		out := make([]float64, 0, len(v))
		for _, item := range v {
			switch itemValue := item.(type) {
			case int:
				out = append(out, float64(itemValue))
			case float64:
				out = append(out, itemValue)
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	default:
		return nil, false
	}
}

func legacyPathListFromAny(value interface{}) ([][]float64, bool) {
	switch v := value.(type) {
	case [][]float64:
		return clonePathList(v), true
	case [][]int:
		out := make([][]float64, 0, len(v))
		for _, path := range v {
			current := make([]float64, len(path))
			for i := range path {
				current[i] = float64(path[i])
			}
			out = append(out, current)
		}
		return out, true
	case []float64:
		return [][]float64{append([]float64(nil), v...)}, true
	default:
		return nil, false
	}
}

func legacyStringMapFromAny(value interface{}) (map[string]string, bool) {
	switch v := value.(type) {
	case map[string]string:
		out := make(map[string]string, len(v))
		for key, item := range v {
			out[key] = item
		}
		return out, true
	case map[string]interface{}:
		out := make(map[string]string, len(v))
		for key, item := range v {
			text, ok := legacyStringFromAny(item)
			if ok {
				out[key] = text
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	default:
		return nil, false
	}
}

func legacyFileExists(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(strings.TrimSpace(path))
	return err == nil && !info.IsDir()
}

func (d *Document) legacyAnnotationPointTarget(value interface{}) (*Annotation, int, int, [][]float64, bool) {
	switch v := value.(type) {
	case *Annotation:
		if v == nil {
			return nil, 0, 0, nil, false
		}
		pageIndex, annotationIndex, found := d.findAnnotationLocation(v)
		if !found {
			return nil, 0, 0, nil, false
		}
		pathList := v.PathList()
		if len(pathList) == 0 {
			if pgPoints := v.PgPoints(); len(pgPoints) > 0 {
				pathList = [][]float64{pgPoints}
			}
		}
		return v, pageIndex, annotationIndex, pathList, true
	case map[string]interface{}:
		pageValue, pageOK := legacyMapValueCI(v, "page", "page_index", "page_no", "pageno")
		nameValue, nameOK := legacyMapValueCI(v, "name", "nm")
		if !pageOK || !nameOK {
			return nil, 0, 0, nil, false
		}

		page, pageParsed := legacyIntFromAny(pageValue)
		name, nameParsed := legacyStringFromAny(nameValue)
		if !pageParsed || !nameParsed {
			return nil, 0, 0, nil, false
		}

		pageIndex, resolveOK := d.resolveLegacyPageIndex(page)
		if !resolveOK {
			return nil, 0, 0, nil, false
		}

		annot := d.GetAnnotation(pageIndex+1, strings.TrimSpace(name))
		if annot == nil {
			return nil, 0, 0, nil, false
		}
		targetPageIndex, annotationIndex, found := d.findAnnotationLocation(annot)
		if !found {
			return nil, 0, 0, nil, false
		}

		pathValue, hasPath := legacyMapValueCI(v, "path_list", "pathlist", "paths", "points", "pg_points")
		if !hasPath {
			return nil, 0, 0, nil, false
		}
		pathList, pathOK := legacyPathListFromAny(pathValue)
		if !pathOK {
			if pgPoints, pgOK := legacyFloatSliceFromAny(pathValue); pgOK {
				pathList = [][]float64{pgPoints}
				pathOK = true
			}
		}
		if !pathOK || len(pathList) == 0 {
			return nil, 0, 0, nil, false
		}
		return annot, targetPageIndex, annotationIndex, pathList, true
	default:
		return nil, 0, 0, nil, false
	}
}

func (d *Document) renderLegacyBitmap(thumbnail bool, args ...interface{}) interface{} {
	pageIndex, ok := d.legacyRenderPageIndex(args...)
	if !ok {
		return nil
	}

	page, err := d.Page(pageIndex)
	if err != nil || page == nil {
		return nil
	}

	scale := 1.0
	for _, arg := range args {
		if value, ok := arg.(float64); ok && value > 0 {
			scale = value
			break
		}
	}

	d.startLegacyRendering(thumbnail)
	defer d.finishLegacyRendering(thumbnail)

	renderer := NewRenderer(DefaultRendererOptions())
	options := DefaultRenderOptions()
	options.Scale = scale
	image, err := renderer.RenderPage(context.Background(), page, options)
	if err != nil {
		return nil
	}
	return image
}

func (d *Document) legacyRenderPageIndex(args ...interface{}) (int, bool) {
	pageCount, err := d.PageCount()
	if err != nil || pageCount <= 0 {
		return 0, false
	}

	pageIndex := 0
	for _, arg := range args {
		value, ok := arg.(int)
		if !ok {
			continue
		}
		if value > 0 && value <= pageCount {
			pageIndex = value - 1
			return pageIndex, true
		}
		if value >= 0 && value < pageCount {
			pageIndex = value
			return pageIndex, true
		}
	}

	return pageIndex, true
}

func (d *Document) startLegacyRendering(thumbnail bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.nowRenderingCount++
	if thumbnail {
		d.nowThumbnailRenderCount++
	}
}

func (d *Document) finishLegacyRendering(thumbnail bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.nowRenderingCount > 0 {
		d.nowRenderingCount--
	}
	if thumbnail && d.nowThumbnailRenderCount > 0 {
		d.nowThumbnailRenderCount--
	}
}

func (d *Document) legacyArticles() []interface{} {
	if d == nil || d.doc == nil {
		return nil
	}

	catalog := d.doc.Catalog()
	if catalog == nil {
		return nil
	}

	threadsObj := catalog.Get(entity.Name("Threads"))
	if threadsObj == nil {
		return nil
	}

	threadsArr, ok := d.legacyArray(threadsObj)
	if !ok || threadsArr == nil || threadsArr.Len() == 0 {
		return nil
	}

	pageRefToIndex := d.buildPageRefToIndexMap()
	pageDictToIndex := make(map[*entity.Dict]int)
	if pageCount, err := d.PageCount(); err == nil && pageCount > 0 {
		for i := 0; i < pageCount; i++ {
			page, pageErr := d.doc.GetPage(i)
			if pageErr != nil || page == nil || page.Dict() == nil {
				continue
			}
			pageDictToIndex[page.Dict()] = i
		}
	}

	articles := make([]interface{}, 0, threadsArr.Len())
	for i := 0; i < threadsArr.Len(); i++ {
		threadDict, err := d.asDict(threadsArr.Get(i))
		if err != nil || threadDict == nil {
			continue
		}

		title := ""
		if infoObj := threadDict.Get(entity.Name("I")); infoObj != nil {
			if infoDict, infoErr := d.asDict(infoObj); infoErr == nil && infoDict != nil {
				title = extractEntityString(infoDict.Get(entity.Name("Title")))
			}
		}

		pages, beadCount := d.legacyThreadPages(threadDict, pageRefToIndex, pageDictToIndex)
		article := map[string]interface{}{
			"index":        len(articles),
			"title":        title,
			"page_numbers": pages,
			"bead_count":   beadCount,
		}
		if len(pages) > 0 {
			article["page"] = pages[0]
		}
		articles = append(articles, article)
	}

	if len(articles) == 0 {
		return nil
	}
	return articles
}

func (d *Document) legacyThreadPages(
	thread *entity.Dict,
	pageRefToIndex map[entity.Ref]int,
	pageDictToIndex map[*entity.Dict]int,
) ([]int, int) {
	if thread == nil {
		return nil, 0
	}

	firstObj := thread.Get(entity.Name("F"))
	if firstObj == nil {
		return nil, 0
	}

	bead, err := d.asDict(firstObj)
	if err != nil || bead == nil {
		return nil, 0
	}

	visited := make(map[*entity.Dict]struct{})
	seenPages := make(map[int]struct{})
	pages := make([]int, 0)
	beadCount := 0

	for bead != nil {
		if _, seen := visited[bead]; seen {
			break
		}
		visited[bead] = struct{}{}
		beadCount++

		if pageNumber, ok := d.legacyThreadPageNumber(bead.Get(entity.Name("P")), pageRefToIndex, pageDictToIndex); ok {
			if _, exists := seenPages[pageNumber]; !exists {
				seenPages[pageNumber] = struct{}{}
				pages = append(pages, pageNumber)
			}
		}

		nextObj := bead.Get(entity.Name("N"))
		if nextObj == nil {
			break
		}
		nextDict, nextErr := d.asDict(nextObj)
		if nextErr != nil || nextDict == nil {
			break
		}
		bead = nextDict
	}

	if len(pages) == 0 {
		return nil, beadCount
	}
	return pages, beadCount
}

func (d *Document) legacyThreadPageNumber(
	pageObj entity.Object,
	pageRefToIndex map[entity.Ref]int,
	pageDictToIndex map[*entity.Dict]int,
) (int, bool) {
	switch v := pageObj.(type) {
	case entity.Ref:
		if idx, ok := pageRefToIndex[v]; ok {
			return idx + 1, true
		}
		if d.doc == nil || d.doc.XRef() == nil {
			return 0, false
		}
		fetched, err := d.doc.XRef().Fetch(v)
		if err != nil {
			return 0, false
		}
		return d.legacyThreadPageNumber(fetched, pageRefToIndex, pageDictToIndex)
	case *entity.Dict:
		if idx, ok := pageDictToIndex[v]; ok {
			return idx + 1, true
		}
		return 0, false
	case *entity.Array:
		if v.Len() <= 0 {
			return 0, false
		}
		return d.legacyThreadPageNumber(v.Get(0), pageRefToIndex, pageDictToIndex)
	default:
		return 0, false
	}
}

func (d *Document) lookupLegacyTrailerID(index int) string {
	trailer := d.legacyTrailer()
	if trailer == nil {
		return ""
	}

	idObj := trailer.Get(entity.Name("ID"))
	if idObj == nil {
		return ""
	}

	if idArr, ok := d.legacyArray(idObj); ok && idArr != nil {
		if index >= 0 && index < idArr.Len() {
			if value := d.legacyTrailerIDString(idArr.Get(index)); value != "" {
				return value
			}
		}
		if idArr.Len() > 0 {
			return d.legacyTrailerIDString(idArr.Get(0))
		}
		return ""
	}

	return d.legacyTrailerIDString(idObj)
}

func (d *Document) legacyTrailer() *entity.Dict {
	if d == nil || d.doc == nil || d.doc.XRef() == nil {
		return nil
	}

	trailerProvider, ok := d.doc.XRef().(interface {
		GetTrailer() (*entity.Dict, error)
	})
	if !ok {
		return nil
	}

	trailer, err := trailerProvider.GetTrailer()
	if err != nil {
		return nil
	}
	return trailer
}

func (d *Document) legacyTrailerIDString(obj entity.Object) string {
	switch v := obj.(type) {
	case *entity.String:
		return legacyNormalizeTrailerID(v.Value())
	case entity.Name:
		return legacyNormalizeTrailerID(v.Value())
	case *entity.Integer:
		return strconv.FormatInt(v.Value(), 10)
	case *entity.Real:
		return strconv.FormatFloat(v.Value(), 'f', -1, 64)
	case entity.Ref:
		if d.doc == nil || d.doc.XRef() == nil {
			return ""
		}
		fetched, err := d.doc.XRef().Fetch(v)
		if err != nil {
			return ""
		}
		return d.legacyTrailerIDString(fetched)
	default:
		return ""
	}
}

func legacyNormalizeTrailerID(value string) string {
	if value == "" {
		return ""
	}
	for i := 0; i < len(value); i++ {
		if value[i] < 0x20 || value[i] > 0x7e {
			return hex.EncodeToString([]byte(value))
		}
	}
	return value
}

func (d *Document) legacyDocumentJavaScripts() map[string]string {
	if d == nil || d.doc == nil {
		return nil
	}

	catalog := d.doc.Catalog()
	if catalog == nil {
		return nil
	}

	namesObj := catalog.Get(entity.Name("Names"))
	if namesObj == nil {
		return nil
	}
	namesDict, err := d.asDict(namesObj)
	if err != nil || namesDict == nil {
		return nil
	}

	jsTreeObj := namesDict.Get(entity.Name("JavaScript"))
	if jsTreeObj == nil {
		return nil
	}
	jsTree, err := d.asDict(jsTreeObj)
	if err != nil || jsTree == nil {
		return nil
	}

	entries := make([]nameTreeEntry, 0)
	if err := d.collectNameTreeEntries(jsTree, 0, map[*entity.Dict]struct{}{}, &entries); err != nil {
		return nil
	}

	out := make(map[string]string)
	for _, entry := range entries {
		name := strings.TrimSpace(entry.name)
		if name == "" {
			continue
		}
		if _, exists := out[name]; exists {
			continue
		}
		script := d.legacyJavaScriptFromObject(entry.value)
		if strings.TrimSpace(script) == "" {
			continue
		}
		out[name] = script
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (d *Document) legacyJavaScriptFromObject(value entity.Object) string {
	switch v := value.(type) {
	case *entity.Dict:
		if script := extractActionJavaScript(v); script != "" {
			return script
		}
		return d.legacyJavaScriptFromObject(v.Get(entity.Name("JS")))
	case *entity.String:
		return extractEntityString(v)
	case entity.Name:
		return v.Value()
	case *entity.Stream:
		decoded, err := v.Decode()
		if err == nil {
			return string(decoded)
		}
		return string(v.RawBytes())
	case entity.Ref:
		if d.doc == nil || d.doc.XRef() == nil {
			return ""
		}
		fetched, err := d.doc.XRef().Fetch(v)
		if err != nil {
			return ""
		}
		return d.legacyJavaScriptFromObject(fetched)
	default:
		return ""
	}
}

func (d *Document) legacyArray(obj entity.Object) (*entity.Array, bool) {
	switch v := obj.(type) {
	case *entity.Array:
		return v, true
	case entity.Ref:
		if d == nil || d.doc == nil || d.doc.XRef() == nil {
			return nil, false
		}
		fetched, err := d.doc.XRef().Fetch(v)
		if err != nil {
			return nil, false
		}
		arr, ok := fetched.(*entity.Array)
		return arr, ok
	default:
		return nil, false
	}
}

func (d *Document) legacyFieldNameFromObject(obj entity.Object, depth int) string {
	if depth > 16 || obj == nil {
		return ""
	}

	fieldDict, err := d.asDict(obj)
	if err != nil || fieldDict == nil {
		return ""
	}

	partial := extractEntityString(fieldDict.Get(entity.Name("T")))
	parentObj := fieldDict.Get(entity.Name("Parent"))
	if parentObj == nil {
		return partial
	}

	parentName := d.legacyFieldNameFromObject(parentObj, depth+1)
	if parentName == "" {
		return partial
	}
	if partial == "" {
		return parentName
	}
	return parentName + "." + partial
}

func (d *Document) legacyChoiceFieldFromAny(value interface{}) (*FormField, bool) {
	switch v := value.(type) {
	case *FormField:
		return v, v != nil
	case FormField:
		copyValue := v
		return &copyValue, true
	case string:
		field, err := d.fieldByName(v)
		if err != nil {
			return nil, false
		}
		return field, true
	case int:
		field, err := d.fieldByIndex(v)
		if err == nil {
			return field, true
		}
		if v > 0 {
			field, err = d.fieldByIndex(v - 1)
			if err == nil {
				return field, true
			}
		}
		return nil, false
	default:
		return nil, false
	}
}

func legacySplitMarkedText(value string) []string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for i := range lines {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (d *Document) legacyPageIndexFromAny(value interface{}) int {
	page, ok := legacyIntFromAny(value)
	if !ok {
		return -1
	}
	if resolved, resolvedOK := d.resolveLegacyPageIndex(page); resolvedOK {
		return resolved
	}
	if page >= 0 {
		return page
	}
	return -1
}

func (d *Document) legacyPageIndexFromArgs(args ...interface{}) int {
	for i := range args {
		if pageIndex := d.legacyPageIndexFromAny(args[i]); pageIndex >= 0 {
			return pageIndex
		}
	}
	return -1
}

func legacyRangeFromArgs(args ...interface{}) (int, int, bool) {
	ints := make([]int, 0, 3)
	for i := range args {
		parsed, ok := legacyIntFromAny(args[i])
		if !ok {
			continue
		}
		ints = append(ints, parsed)
	}
	if len(ints) < 2 {
		return 0, 0, false
	}
	if len(ints) >= 3 {
		return ints[1], ints[2], true
	}
	return ints[0], ints[1], true
}

func minFloat64(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}
	minValue := values[0]
	for i := 1; i < len(values); i++ {
		if values[i] < minValue {
			minValue = values[i]
		}
	}
	return minValue
}

func maxFloat64(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}
	maxValue := values[0]
	for i := 1; i < len(values); i++ {
		if values[i] > maxValue {
			maxValue = values[i]
		}
	}
	return maxValue
}

func legacyImageDataFromArgs(args ...interface{}) ([]byte, string, bool) {
	path := ""
	for i := range args {
		switch value := args[i].(type) {
		case []byte:
			if len(value) > 0 {
				return append([]byte(nil), value...), path, true
			}
		case string:
			candidate := strings.TrimSpace(value)
			if candidate == "" {
				continue
			}
			if path == "" && legacyFileExists(candidate) {
				path = candidate
			}
		}
	}
	if path == "" {
		return nil, "", false
	}

	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil, "", false
	}
	return data, path, true
}

func (d *Document) applyLegacyEncryptionState(defaultFilter string, args ...interface{}) bool {
	filter := strings.TrimSpace(defaultFilter)
	if filter == "" {
		filter = "Standard"
	}
	ownerPasswordOK := true
	permissions := entity.PermPrint |
		entity.PermModify |
		entity.PermCopy |
		entity.PermAnnotate |
		entity.PermFillForms |
		entity.PermExtract |
		entity.PermAssemble |
		entity.PermPrintHighRes |
		entity.PermOwner

	hasMaterial := false
	for i := range args {
		switch value := args[i].(type) {
		case string:
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				continue
			}
			hasMaterial = true
			lowered := strings.ToLower(trimmed)
			if strings.Contains(lowered, "drm") || strings.Contains(lowered, "unidoc") {
				filter = "UnidocsDRM"
			}
		case int:
			permissions = entity.PermissionFlags(uint32(value))
			hasMaterial = true
		case int64:
			permissions = entity.PermissionFlags(uint32(value))
			hasMaterial = true
		case bool:
			ownerPasswordOK = value
			hasMaterial = true
		}
	}
	if !hasMaterial {
		return false
	}
	if permissions == 0 {
		permissions = entity.PermPrint | entity.PermCopy | entity.PermAnnotate
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.legacyEncryptEnabled = true
	d.legacyEncryptFilter = filter
	d.legacyEncryptPermissions = permissions
	d.legacyOwnerPasswordOK = ownerPasswordOK
	return true
}

func legacyExistingFilePathFromArgs(args ...interface{}) (string, bool) {
	for i := range args {
		value, ok := args[i].(string)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || !legacyFileExists(trimmed) {
			continue
		}
		return trimmed, true
	}
	return "", false
}

func legacyIntArgs(args ...interface{}) []int {
	out := make([]int, 0, len(args))
	for i := range args {
		value, ok := legacyIntFromAny(args[i])
		if ok {
			out = append(out, value)
		}
	}
	return out
}

func (d *Document) legacyImagePageNumber(args []interface{}, fallback int) int {
	pageCount, err := d.PageCount()
	if err != nil || pageCount <= 0 {
		return 0
	}
	for i := range args {
		pageNumber, ok := legacyIntFromAny(args[i])
		if !ok {
			continue
		}
		if pageNumber > 0 && pageNumber <= pageCount {
			return pageNumber
		}
	}
	if fallback > 0 && fallback <= pageCount {
		return fallback
	}
	return 1
}

func (d *Document) legacyImageRectFromArgs(args []interface{}, pageNumber int, fullPage bool) [4]float64 {
	if fullPage {
		if pageBox, err := d.GetPageMediaBoxSL(pageNumber - 1); err == nil {
			return pageBox
		}
	}

	for i := range args {
		if rect, ok := legacyRectFromAny(args[i]); ok && rect[2] > rect[0] && rect[3] > rect[1] {
			return rect
		}
	}

	ints := legacyIntArgs(args...)
	if len(ints) >= 4 {
		x := float64(ints[len(ints)-4])
		y := float64(ints[len(ints)-3])
		w := float64(ints[len(ints)-2])
		h := float64(ints[len(ints)-1])
		if w < 0 {
			x += w
			w = -w
		}
		if h < 0 {
			y += h
			h = -h
		}
		if w == 0 {
			w = 120
		}
		if h == 0 {
			h = 80
		}
		return [4]float64{x, y, x + w, y + h}
	}

	if pageBox, err := d.GetPageMediaBoxSL(pageNumber - 1); err == nil {
		return pageBox
	}
	return [4]float64{0, 0, 120, 80}
}

func (d *Document) legacyImageTagFromArgs(args []interface{}) string {
	for i := len(args) - 1; i >= 0; i-- {
		value, ok := legacyStringFromAny(args[i])
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if legacyFileExists(trimmed) {
			continue
		}
		return trimmed
	}
	return ""
}

func (d *Document) nrdsTileKeyFromArgs(args ...interface{}) (string, bool) {
	if len(args) == 0 {
		return "", false
	}

	pageNumber := 0
	zoom := 1.0
	coords := make([]int, 0, 4)

	for i := range args {
		switch value := args[i].(type) {
		case int:
			if pageNumber == 0 {
				pageNumber = value
				continue
			}
			coords = append(coords, value)
		case float64:
			if zoom == 1.0 {
				zoom = value
			}
		}
	}
	if pageNumber == 0 {
		return "", false
	}

	if zoom <= 0 {
		zoom = 1
	}
	for len(coords) < 4 {
		coords = append(coords, 0)
	}
	return fmt.Sprintf(
		"p=%d|z=%.4f|x=%d|y=%d|w=%d|h=%d",
		pageNumber,
		zoom,
		coords[0], coords[1], coords[2], coords[3],
	), true
}

func (d *Document) nrdsTrimToLimitLocked() {
	limit := d.nrdsCacheLimit
	if limit <= 0 {
		return
	}

	for len(d.nrdsTileBitmap) > limit {
		for key := range d.nrdsTileBitmap {
			delete(d.nrdsTileBitmap, key)
			delete(d.nrdsTileData, key)
			break
		}
	}
}
