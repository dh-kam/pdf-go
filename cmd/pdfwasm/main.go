//go:build js && wasm

// Package main exposes the PDF renderer to browser JavaScript through WebAssembly.
package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"log"
	"math"
	"runtime/debug"
	"sync"
	"syscall/js"
	"time"

	"github.com/dh-kam/pdf-go/internal/version"
	"github.com/dh-kam/pdf-go/pkg/pdf"
)

const (
	defaultDPI           = 144.0
	defaultScale         = 1.0
	defaultMaxPagePixels = int64(80_000_000)
	defaultTimeout       = 60 * time.Second
	defaultBackend       = pdf.RendererBackendSplash
)

type wasmDocument struct {
	doc      *pdf.Document
	renderer *pdf.Renderer
	backend  string
}

var documentStore = struct {
	sync.Mutex
	nextID int
	docs   map[int]*wasmDocument
}{
	nextID: 1,
	docs:   make(map[int]*wasmDocument),
}

var registeredFuncs []js.Func

func main() {
	registerAPI()
	select {}
}

func registerAPI() {
	api := js.Global().Get("Object").New()
	setFunc(api, "openDocument", openDocument)
	setFunc(api, "closeDocument", closeDocument)
	setFunc(api, "pageInfo", pageInfo)
	setFunc(api, "renderPage", renderPage)
	setFunc(api, "version", apiVersion)
	js.Global().Set("pdfgo", api)
	log.Printf("pdfwasm: API registered with default backend %s", defaultBackend)
}

func setFunc(target js.Value, name string, fn func(js.Value, []js.Value) (any, error)) {
	wrapped := js.FuncOf(func(this js.Value, args []js.Value) (result any) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("pdfwasm: %s panic: %v", name, recovered)
				result = panicResult(recovered)
			}
		}()
		value, err := fn(this, args)
		if err != nil {
			log.Printf("pdfwasm: %s error: %v", name, err)
			return errorResult(err)
		}
		return okResult(value)
	})
	registeredFuncs = append(registeredFuncs, wrapped)
	target.Set(name, wrapped)
}

func openDocument(_ js.Value, args []js.Value) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("openDocument requires a Uint8Array PDF payload")
	}
	dataValue := args[0]
	if dataValue.IsUndefined() || dataValue.IsNull() {
		return nil, fmt.Errorf("openDocument received an empty PDF payload")
	}

	data := make([]byte, dataValue.Get("byteLength").Int())
	if copied := js.CopyBytesToGo(data, dataValue); copied != len(data) {
		return nil, fmt.Errorf("copied %d of %d PDF bytes", copied, len(data))
	}

	options := optionalObject(args, 1)
	password := stringOption(options, "password", "")
	backend := stringOption(options, "backend", defaultBackend)
	maxWorkers := intOption(options, "maxWorkers", 1)
	cacheSize := intOption(options, "cacheSize", 4)

	startedAt := time.Now()
	log.Printf("pdfwasm: openDocument start bytes=%d backend=%s maxWorkers=%d cacheSize=%d password=%t", len(data), backend, maxWorkers, cacheSize, password != "")

	doc, err := openPDF(data, password)
	if err != nil {
		return nil, err
	}
	pageCount, err := doc.PageCount()
	if err != nil {
		_ = doc.Close()
		return nil, err
	}

	renderer := pdf.NewRenderer(pdf.RendererOptions{
		MaxWorkers: maxWorkers,
		CacheSize:  cacheSize,
		Backend:    backend,
	})

	documentStore.Lock()
	id := documentStore.nextID
	documentStore.nextID++
	documentStore.docs[id] = &wasmDocument{doc: doc, renderer: renderer, backend: backend}
	documentStore.Unlock()

	log.Printf("pdfwasm: openDocument done id=%d pages=%d backend=%s elapsed=%s", id, pageCount, backend, time.Since(startedAt))

	return map[string]any{
		"id":        id,
		"pageCount": pageCount,
		"backend":   backend,
		"version":   version.Current,
	}, nil
}

func closeDocument(_ js.Value, args []js.Value) (any, error) {
	if len(args) < 1 {
		return nil, fmt.Errorf("closeDocument requires a document id")
	}
	id := args[0].Int()
	log.Printf("pdfwasm: closeDocument start id=%d", id)

	documentStore.Lock()
	entry, ok := documentStore.docs[id]
	if ok {
		delete(documentStore.docs, id)
	}
	documentStore.Unlock()

	if !ok {
		return nil, fmt.Errorf("document id %d not found", id)
	}
	if err := entry.doc.Close(); err != nil {
		return nil, err
	}
	log.Printf("pdfwasm: closeDocument done id=%d", id)
	return map[string]any{"id": id, "closed": true}, nil
}

func pageInfo(_ js.Value, args []js.Value) (any, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("pageInfo requires document id and zero-based page index")
	}
	entry, err := lookupDocument(args[0].Int())
	if err != nil {
		return nil, err
	}
	page, err := entry.doc.Page(args[1].Int())
	if err != nil {
		return nil, err
	}
	log.Printf("pdfwasm: pageInfo id=%d page=%d backend=%s", args[0].Int(), args[1].Int(), entry.backend)
	return pageInfoResult(page), nil
}

func renderPage(_ js.Value, args []js.Value) (any, error) {
	if len(args) < 2 {
		return nil, fmt.Errorf("renderPage requires document id and zero-based page index")
	}
	entry, err := lookupDocument(args[0].Int())
	if err != nil {
		return nil, err
	}
	pageIndex := args[1].Int()
	page, err := entry.doc.Page(pageIndex)
	if err != nil {
		return nil, err
	}

	options := optionalObject(args, 2)
	dpi := numberOption(options, "dpi", defaultDPI)
	scale := numberOption(options, "scale", defaultScale)
	timeout := durationOption(options, "timeoutMs", defaultTimeout)
	maxPagePixels := int64Option(options, "maxPagePixels", defaultMaxPagePixels)
	enableCache := boolOption(options, "enableCache", true)
	imageSamplingMode := stringOption(options, "imageSamplingMode", pdf.DefaultRenderOptions().ImageSamplingMode)

	startedAt := time.Now()
	log.Printf("pdfwasm: renderPage start id=%d page=%d dpi=%.2f scale=%.2f backend=%s timeout=%s maxPagePixels=%d cache=%t sampling=%s", args[0].Int(), pageIndex, dpi, scale, entry.backend, timeout, maxPagePixels, enableCache, imageSamplingMode)

	if dpi <= 0 {
		return nil, fmt.Errorf("dpi must be positive")
	}
	if scale <= 0 {
		return nil, fmt.Errorf("scale must be positive")
	}
	if maxPagePixels > 0 {
		estimatedPixels := estimatePagePixels(page, dpi, scale)
		log.Printf("pdfwasm: renderPage estimate id=%d page=%d pixels=%d", args[0].Int(), pageIndex, estimatedPixels)
		if estimatedPixels > maxPagePixels {
			return nil, fmt.Errorf("estimated page size %d pixels exceeds maxPagePixels %d", estimatedPixels, maxPagePixels)
		}
	}

	ctx := context.Background()
	cancel := func() {}
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	img, err := entry.renderer.RenderPage(ctx, page, pdf.RenderOptions{
		DPI:               dpi,
		Scale:             scale,
		EnableCache:       enableCache,
		ImageSamplingMode: imageSamplingMode,
	})
	if err != nil {
		return nil, err
	}
	renderElapsed := time.Since(startedAt)

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if maxPagePixels > 0 && int64(width)*int64(height) > maxPagePixels {
		return nil, fmt.Errorf("rendered page size %d pixels exceeds maxPagePixels %d", int64(width)*int64(height), maxPagePixels)
	}

	copyStartedAt := time.Now()
	pixels := imageToRGBABytes(img)
	data := js.Global().Get("Uint8ClampedArray").New(len(pixels))
	js.CopyBytesToJS(data, pixels)
	log.Printf("pdfwasm: renderPage done id=%d page=%d size=%dx%d rgbaBytes=%d render=%s copy=%s total=%s", args[0].Int(), pageIndex, width, height, len(pixels), renderElapsed, time.Since(copyStartedAt), time.Since(startedAt))

	return map[string]any{
		"pageIndex": pageIndex,
		"width":     width,
		"height":    height,
		"dpi":       dpi,
		"scale":     scale,
		"data":      data,
	}, nil
}

func apiVersion(_ js.Value, _ []js.Value) (any, error) {
	return map[string]any{"version": version.Current}, nil
}

func openPDF(data []byte, password string) (*pdf.Document, error) {
	reader := bytes.NewReader(data)
	if password != "" {
		return pdf.OpenReaderWithPassword(reader, password)
	}
	return pdf.OpenReader(reader)
}

func lookupDocument(id int) (*wasmDocument, error) {
	documentStore.Lock()
	entry, ok := documentStore.docs[id]
	documentStore.Unlock()
	if !ok {
		return nil, fmt.Errorf("document id %d not found", id)
	}
	return entry, nil
}

func pageInfoResult(page *pdf.Page) map[string]any {
	mediaBox := page.MediaBox()
	cropBox := page.CropBox()
	return map[string]any{
		"pageIndex": page.Index(),
		"width":     math.Abs(page.Width()),
		"height":    math.Abs(page.Height()),
		"rotate":    page.Rotate(),
		"mediaBox":  floatArray(mediaBox),
		"cropBox":   floatArray(cropBox),
	}
}

func estimatePagePixels(page *pdf.Page, dpi, scale float64) int64 {
	box := page.CropBox()
	width := math.Abs(box[2] - box[0])
	height := math.Abs(box[3] - box[1])
	rotation := normalizeRotation(page.Rotate())
	if rotation == 90 || rotation == 270 {
		width, height = height, width
	}
	pixelWidth := pointsToPixels(width, dpi, scale)
	pixelHeight := pointsToPixels(height, dpi, scale)
	return int64(pixelWidth) * int64(pixelHeight)
}

func pointsToPixels(points, dpi, scale float64) int {
	pixels := math.Abs(points) * (dpi / 72.0) * scale
	roundedUp := int(math.Ceil(pixels - 1e-9))
	if roundedUp < 1 {
		return 1
	}
	return roundedUp
}

func normalizeRotation(rotation int) int {
	normalized := rotation % 360
	if normalized < 0 {
		normalized += 360
	}
	switch normalized {
	case 90, 180, 270:
		return normalized
	default:
		return 0
	}
}

func imageToRGBABytes(img image.Image) []byte {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	rgba := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(rgba, rgba.Bounds(), img, bounds.Min, draw.Src)

	pixels := make([]byte, width*height*4)
	rowBytes := width * 4
	for y := 0; y < height; y++ {
		srcStart := y * rgba.Stride
		copy(pixels[y*rowBytes:(y+1)*rowBytes], rgba.Pix[srcStart:srcStart+rowBytes])
	}
	return pixels
}

func optionalObject(args []js.Value, index int) js.Value {
	if len(args) <= index {
		return js.Undefined()
	}
	value := args[index]
	if value.IsUndefined() || value.IsNull() || value.Type() != js.TypeObject {
		return js.Undefined()
	}
	return value
}

func numberOption(options js.Value, name string, fallback float64) float64 {
	value := optionValue(options, name)
	if value.IsUndefined() {
		return fallback
	}
	return value.Float()
}

func intOption(options js.Value, name string, fallback int) int {
	value := optionValue(options, name)
	if value.IsUndefined() {
		return fallback
	}
	return value.Int()
}

func int64Option(options js.Value, name string, fallback int64) int64 {
	value := optionValue(options, name)
	if value.IsUndefined() {
		return fallback
	}
	return int64(value.Int())
}

func boolOption(options js.Value, name string, fallback bool) bool {
	value := optionValue(options, name)
	if value.IsUndefined() {
		return fallback
	}
	return value.Bool()
}

func stringOption(options js.Value, name string, fallback string) string {
	value := optionValue(options, name)
	if value.IsUndefined() {
		return fallback
	}
	return value.String()
}

func durationOption(options js.Value, name string, fallback time.Duration) time.Duration {
	value := optionValue(options, name)
	if value.IsUndefined() {
		return fallback
	}
	ms := value.Int()
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

func optionValue(options js.Value, name string) js.Value {
	if options.IsUndefined() || options.IsNull() {
		return js.Undefined()
	}
	value := options.Get(name)
	if value.IsUndefined() || value.IsNull() {
		return js.Undefined()
	}
	return value
}

func okResult(value any) map[string]any {
	if value == nil {
		return map[string]any{"ok": true}
	}
	if result, ok := value.(map[string]any); ok {
		result["ok"] = true
		return result
	}
	return map[string]any{"ok": true, "value": value}
}

func errorResult(err error) map[string]any {
	return map[string]any{
		"ok":    false,
		"error": err.Error(),
	}
}

func panicResult(recovered any) map[string]any {
	return map[string]any{
		"ok":    false,
		"error": fmt.Sprintf("panic: %v", recovered),
		"stack": string(debug.Stack()),
	}
}

func floatArray(values [4]float64) []any {
	return []any{values[0], values[1], values[2], values[3]}
}
