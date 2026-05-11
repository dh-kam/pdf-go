// Package pdf provides PDF document loading and rendering functionality.
//
// # Quick Start
//
//	// Open a PDF document
//	doc, err := pdf.Open("document.pdf")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer doc.Close()
//
//	// Get page count
//	pageCount, err := doc.PageCount()
//
//	// Get a page
//	page, err := doc.Page(0)
//
//	// Render the page
//	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
//	img, err := renderer.RenderPage(context.Background(), page, pdf.DefaultRenderOptions())
package pdf

import (
	"context"
	"fmt"
	"image"
	"io"
	"sync"
	"time"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
	"github.com/dh-kam/pdf-go/internal/infrastructure/renderer"
	pdfluence "github.com/dh-kam/pdf-go/internal/usecase/pdf"
)

// Document represents a PDF document.
type Document struct {
	doc                      *entity.Document
	filePath                 string
	userData                 map[string]map[string][]byte
	pageUserData             map[int]map[string]string
	formValues               map[string][]string
	formOptions              map[string][]string
	pageMediaBoxes           map[int][4]float64
	pageCropBoxes            map[int][4]float64
	pageRotations            map[int]int
	pagePieceInfo            map[int]map[string]map[string]interface{}
	bannerView               interface{}
	bookDirection            int
	zoom                     float64
	currentPage              int
	widthFit                 bool
	heightFit                bool
	nightMode                bool
	nowSaveProcessing        bool
	nowSigning               bool
	nowRenderingCount        int
	nowThumbnailRenderCount  int
	legacyEncryptEnabled     bool
	legacyOwnerPasswordOK    bool
	legacyEncryptFilter      string
	legacyEncryptPermissions entity.PermissionFlags
	savedAfterOpen           bool
	openFrom                 string
	streamingForOpen         bool
	listeners                []interface{}
	actionContentReplaceList []interface{}
	customNoteIconFactory    interface{}
	customQuizIconFactory    interface{}
	exNoteIconFactory        interface{}
	packagedDocListener      interface{}
	prohibitedPages          interface{}
	highPriorityWorkingCount int
	legacyStreams            map[int]*legacyStreamState
	nextLegacyStreamHandle   int
	paperColor               int
	screenWatermarks         []string
	instantWatermarks        []string
	screenWatermarkHide      bool
	addedAttachments         map[string]sessionAttachmentState
	deletedAttachments       map[string]bool
	hiddenTargets            map[string]bool
	annotationOverrides      map[int][]annotationSnapshot
	signatureFields          map[string]signatureFieldSnapshot
	pageOrder                []int
	outlines                 []*Outline
	nrdsTileData             map[string][]byte
	nrdsTileBitmap           map[string]interface{}
	nrdsCacheLimit           int
	mu                       sync.RWMutex
	outlinesSet              bool
}

// Open opens a PDF document from a file path.
func Open(path string) (*Document, error) {
	doc, err := pdfluence.Open(path)
	if err != nil {
		return nil, err
	}
	d := newDocument(doc)
	d.filePath = path
	d.openFrom = "LocalPath"
	d.streamingForOpen = false
	return d, nil
}

// OpenWithPassword opens a PDF document from a file path with a password.
func OpenWithPassword(path, password string) (*Document, error) {
	doc, err := pdfluence.OpenWithPassword(path, password)
	if err != nil {
		return nil, err
	}
	d := newDocument(doc)
	d.filePath = path
	d.openFrom = "LocalPath"
	d.streamingForOpen = false
	return d, nil
}

// OpenReader opens a PDF document from an io.Reader.
func OpenReader(r io.Reader) (*Document, error) {
	doc, err := pdfluence.OpenReader(r)
	if err != nil {
		return nil, err
	}

	d := newDocument(doc)
	d.openFrom = "Stream"
	d.streamingForOpen = true
	return d, nil
}

// OpenReaderWithPassword opens a PDF document from an io.Reader with a password.
func OpenReaderWithPassword(r io.Reader, password string) (*Document, error) {
	doc, err := pdfluence.OpenReaderWithPassword(r, password)
	if err != nil {
		return nil, err
	}

	d := newDocument(doc)
	d.openFrom = "Stream"
	d.streamingForOpen = true
	return d, nil
}

func newDocument(doc *entity.Document) *Document {
	pageOrder := make([]int, 0)
	if doc != nil {
		if count, err := doc.PageCount(); err == nil && count > 0 {
			pageOrder = make([]int, count)
			for i := 0; i < count; i++ {
				pageOrder[i] = i
			}
		}
	}
	currentPage := 1
	if len(pageOrder) == 0 {
		currentPage = 0
	}

	d := &Document{
		doc:                    doc,
		userData:               make(map[string]map[string][]byte),
		pageUserData:           make(map[int]map[string]string),
		formValues:             make(map[string][]string),
		formOptions:            make(map[string][]string),
		pageMediaBoxes:         make(map[int][4]float64),
		pageCropBoxes:          make(map[int][4]float64),
		pageRotations:          make(map[int]int),
		pagePieceInfo:          make(map[int]map[string]map[string]interface{}),
		bookDirection:          1,
		zoom:                   1.0,
		currentPage:            currentPage,
		openFrom:               "Stream",
		streamingForOpen:       false,
		paperColor:             0xFFFFFFFF,
		legacyStreams:          make(map[int]*legacyStreamState),
		nextLegacyStreamHandle: 1,
		addedAttachments:       make(map[string]sessionAttachmentState),
		deletedAttachments:     make(map[string]bool),
		hiddenTargets:          make(map[string]bool),
		annotationOverrides:    make(map[int][]annotationSnapshot),
		pageOrder:              pageOrder,
		signatureFields:        make(map[string]signatureFieldSnapshot),
		legacyOwnerPasswordOK:  true,
		nrdsTileData:           make(map[string][]byte),
		nrdsTileBitmap:         make(map[string]interface{}),
	}

	d.applyEmbeddedSessionState()
	return d
}

// Close closes the document and releases resources.
func (d *Document) Close() error {
	return d.doc.Close()
}

// PageCount returns the number of pages in the document.
func (d *Document) PageCount() (int, error) {
	d.mu.RLock()
	if len(d.pageOrder) > 0 {
		count := len(d.pageOrder)
		d.mu.RUnlock()
		return count, nil
	}
	d.mu.RUnlock()

	return d.doc.PageCount()
}

// Page returns the page at the specified index (0-based).
func (d *Document) Page(index int) (*Page, error) {
	d.mu.RLock()
	sourceIndex := index
	if len(d.pageOrder) > 0 {
		if index < 0 || index >= len(d.pageOrder) {
			d.mu.RUnlock()
			return nil, fmt.Errorf("page index out of range: %d", index)
		}
		sourceIndex = d.pageOrder[index]
	}
	d.mu.RUnlock()

	page, err := d.doc.GetPage(sourceIndex)
	if err != nil {
		return nil, err
	}

	return &Page{doc: d, page: page, sourceIndex: sourceIndex}, nil
}

// Catalog returns the document catalog dictionary.
func (d *Document) Catalog() *Dict {
	catalog := d.doc.Catalog()
	if catalog == nil {
		return nil
	}
	return &Dict{dict: catalog}
}

// Info returns the document info dictionary.
func (d *Document) Info() *Dict {
	info := d.doc.Info()
	if info == nil {
		return nil
	}
	return &Dict{dict: info}
}

// FileSize returns the file size in bytes.
func (d *Document) FileSize() int64 {
	return d.doc.FileSize()
}

// Page represents a PDF page.
type Page struct {
	doc         *Document
	page        *entity.Page
	sourceIndex int
}

// Index returns the page index (0-based).
func (p *Page) Index() int {
	return p.page.Index()
}

// MediaBox returns the media box rectangle [x1, y1, x2, y2].
func (p *Page) MediaBox() [4]float64 {
	if p.doc != nil {
		if box, ok := p.doc.pageMediaBoxOverride(p.sourceIndex); ok {
			return box
		}
	}
	return p.page.MediaBox()
}

// CropBox returns the crop box rectangle.
func (p *Page) CropBox() [4]float64 {
	if p.doc != nil {
		if box, ok := p.doc.pageCropBoxOverride(p.sourceIndex); ok {
			return box
		}
	}
	return p.page.CropBox()
}

// Rotate returns the page rotation in degrees (0, 90, 180, 270).
func (p *Page) Rotate() int {
	if p.doc != nil {
		if rotation, ok := p.doc.pageRotationOverride(p.sourceIndex); ok {
			return rotation
		}
	}
	return p.page.Rotate()
}

// Width returns the page width in points.
func (p *Page) Width() float64 {
	box := p.MediaBox()
	return box[2] - box[0]
}

// Height returns the page height in points.
func (p *Page) Height() float64 {
	box := p.MediaBox()
	return box[3] - box[1]
}

// Contents returns the page content streams.
func (p *Page) Contents() ([]Object, error) {
	contents, err := p.page.Contents()
	if err != nil {
		return nil, err
	}

	result := make([]Object, len(contents))
	for i, obj := range contents {
		result[i] = wrapObject(obj)
	}
	return result, nil
}

// Annotations returns the page annotations.
func (p *Page) Annotations() ([]*Annotation, error) {
	if p.doc != nil {
		if overrides, ok := p.doc.pageAnnotationOverride(p.sourceIndex); ok {
			result := make([]*Annotation, len(overrides))
			for i := range overrides {
				snapshot := overrides[i]
				result[i] = &Annotation{snapshot: &snapshot}
			}
			return result, nil
		}
	}

	annots, err := p.page.Annotations()
	if err != nil {
		return nil, err
	}

	result := make([]*Annotation, len(annots))
	for i, a := range annots {
		result[i] = &Annotation{annotation: a}
	}
	return result, nil
}

// Resources returns the page resources dictionary.
func (p *Page) Resources() (*Dict, error) {
	resources, err := p.page.Resources()
	if err != nil {
		return nil, err
	}
	return &Dict{dict: resources}, nil
}

// Dict represents a PDF dictionary.
type Dict struct {
	dict *entity.Dict
}

// Get returns the value for a key.
func (d *Dict) Get(key string) Object {
	if d.dict == nil {
		return nil
	}
	val := d.dict.Get(entity.Name(key))
	return wrapObject(val)
}

// Object represents a PDF object.
type Object interface{}

func wrapObject(obj entity.Object) Object {
	if obj == nil {
		return nil
	}
	switch v := obj.(type) {
	case *entity.Dict:
		return &Dict{dict: v}
	case *entity.Array:
		return &Array{array: v}
	case *entity.String:
		return v.Value()
	case *entity.Integer:
		return int(v.Value())
	case *entity.Real:
		return v.Value()
	case entity.Name:
		return string(v)
	case entity.Ref:
		return v
	default:
		return obj
	}
}

// Array represents a PDF array.
type Array struct {
	array *entity.Array
}

// Len returns the number of elements in the array.
func (a *Array) Len() int {
	if a.array == nil {
		return 0
	}
	return a.array.Len()
}

// Get returns the element at the specified index.
func (a *Array) Get(index int) Object {
	if a.array == nil {
		return nil
	}
	val := a.array.Get(index)
	return wrapObject(val)
}

// Annotation represents a PDF annotation.
type Annotation struct {
	annotation *entity.Annotation
	snapshot   *annotationSnapshot
}

// Type returns the annotation type.
func (a *Annotation) Type() string {
	if a.snapshot != nil {
		return a.snapshot.Type
	}
	return string(a.annotation.Type())
}

// Rect returns the annotation rectangle.
func (a *Annotation) Rect() [4]float64 {
	if a.snapshot != nil {
		return a.snapshot.Rect
	}
	return a.annotation.Rect()
}

// Contents returns the annotation contents.
func (a *Annotation) Contents() string {
	if a.snapshot != nil {
		return a.snapshot.Contents
	}
	return a.annotation.Contents()
}

// PgPoints returns flattened annotation point coordinates, if present.
func (a *Annotation) PgPoints() []float64 {
	if a == nil {
		return nil
	}
	if a.snapshot != nil {
		return cloneFloat64Slice(a.snapshot.PgPoints)
	}
	if a.annotation == nil {
		return nil
	}
	return annotationPgPointsFromDict(a.annotation.Dict())
}

// HeadPoints returns annotation head point coordinates, if present.
func (a *Annotation) HeadPoints() []float64 {
	if a == nil {
		return nil
	}
	if a.snapshot != nil {
		return cloneFloat64Slice(a.snapshot.HeadPoints)
	}
	if a.annotation == nil {
		return nil
	}
	return annotationHeadPointsFromDict(a.annotation.Dict())
}

// PathList returns annotation path list coordinates, if present.
func (a *Annotation) PathList() [][]float64 {
	if a == nil {
		return nil
	}
	if a.snapshot != nil {
		return clonePathList(a.snapshot.PathList)
	}
	if a.annotation == nil {
		return nil
	}
	return annotationPathListFromDict(a.annotation.Dict())
}

// UserData returns one annotation user data value by key.
func (a *Annotation) UserData(key string) (string, bool) {
	normalizedKey := normalizeAnnotationUserDataKey(key)
	if normalizedKey == "" {
		return "", false
	}
	data := a.UserDataList()
	value, ok := data[normalizedKey]
	return value, ok
}

// UserDataList returns annotation user data as a copied map.
func (a *Annotation) UserDataList() map[string]string {
	if a == nil {
		return nil
	}
	if a.snapshot != nil {
		return cloneStringMap(a.snapshot.UserData)
	}
	if a.annotation == nil {
		return nil
	}
	return annotationUserDataFromDict(a.annotation.Dict())
}

// Renderer renders PDF pages to images.
type Renderer struct {
	renderer *renderer.ConcurrentRenderer
}

// RendererOptions represents options for creating a renderer.
type RendererOptions struct {
	MaxWorkers int
	CacheSize  int
	CacheTTL   time.Duration
}

// DefaultRendererOptions returns default renderer options.
func DefaultRendererOptions() RendererOptions {
	return RendererOptions{
		MaxWorkers: 4,
		CacheSize:  20,
		CacheTTL:   10 * time.Minute,
	}
}

// NewRenderer creates a new renderer.
func NewRenderer(options RendererOptions) *Renderer {
	opts := renderer.RendererOptions{
		MaxWorkers: options.MaxWorkers,
		CacheSize:  options.CacheSize,
		CacheTTL:   options.CacheTTL,
	}

	r := renderer.NewConcurrentRenderer(opts)

	return &Renderer{renderer: r}
}

// RenderOptions represents options for rendering a page.
type RenderOptions struct {
	BackgroundColor   interface{}
	DPI               float64
	Scale             float64
	EnableCache       bool
	ImageSamplingMode string
}

// DefaultRenderOptions returns default render options.
func DefaultRenderOptions() RenderOptions {
	return RenderOptions{
		DPI:               72.0,
		Scale:             1.0,
		EnableCache:       true,
		ImageSamplingMode: domainrenderer.ImageSamplingModeLegacy,
	}
}

// RenderPage renders a single page to an image.
func (r *Renderer) RenderPage(ctx context.Context, page *Page, options RenderOptions) (image.Image, error) {
	opts := domainrenderer.RenderOptions{
		DPI:               options.DPI,
		Scale:             options.Scale,
		EnableCache:       options.EnableCache,
		ImageSamplingMode: options.ImageSamplingMode,
	}

	return r.renderer.RenderPage(ctx, page.page, opts)
}

// RenderResult represents a rendered page result.
type RenderResult struct {
	Image   image.Image
	Error   error
	PageNum int
}

// RenderPages renders multiple pages concurrently.
func (r *Renderer) RenderPages(ctx context.Context, doc *Document, pageNumbers []int, options RenderOptions) <-chan RenderResult {
	opts := domainrenderer.RenderOptions{
		DPI:               options.DPI,
		Scale:             options.Scale,
		EnableCache:       options.EnableCache,
		ImageSamplingMode: options.ImageSamplingMode,
	}

	resultChan := r.renderer.RenderPages(ctx, doc.doc, pageNumbers, opts)

	// Convert to public RenderResult
	outChan := make(chan RenderResult, cap(resultChan))
	go func() {
		for result := range resultChan {
			outChan <- RenderResult{
				PageNum: result.PageNum,
				Image:   result.Image,
				Error:   result.Error,
			}
		}
		close(outChan)
	}()

	return outChan
}

// RenderAllPages renders all pages in the document concurrently.
func (r *Renderer) RenderAllPages(ctx context.Context, doc *Document, options RenderOptions) <-chan RenderResult {
	opts := domainrenderer.RenderOptions{
		DPI:         options.DPI,
		Scale:       options.Scale,
		EnableCache: options.EnableCache,
	}

	resultChan := r.renderer.RenderAllPages(ctx, doc.doc, opts)

	// Convert to public RenderResult
	outChan := make(chan RenderResult, cap(resultChan))
	go func() {
		for result := range resultChan {
			outChan <- RenderResult{
				PageNum: result.PageNum,
				Image:   result.Image,
				Error:   result.Error,
			}
		}
		close(outChan)
	}()

	return outChan
}

// Stats returns renderer statistics.
func (r *Renderer) Stats() RendererStats {
	stats := r.renderer.Stats()
	return RendererStats{
		RenderCount:     stats.RenderCount,
		CacheHits:       stats.CacheHits,
		CacheMisses:     stats.CacheMisses,
		FormCacheHits:   stats.FormCacheHits,
		FormCacheMisses: stats.FormCacheMisses,
		FormCacheSets:   stats.FormCacheSets,
	}
}

// RendererStats represents renderer statistics.
type RendererStats struct {
	RenderCount     int64
	CacheHits       int64
	CacheMisses     int64
	FormCacheHits   int64
	FormCacheMisses int64
	FormCacheSets   int64
}

// HitRate returns the cache hit rate as a percentage.
func (s RendererStats) HitRate() float64 {
	total := s.CacheHits + s.CacheMisses
	if total == 0 {
		return 0
	}
	return float64(s.CacheHits) / float64(total) * 100
}

// FormCacheHitRate returns the form-operator cache hit rate as a percentage.
func (s RendererStats) FormCacheHitRate() float64 {
	total := s.FormCacheHits + s.FormCacheMisses
	if total == 0 {
		return 0
	}
	return float64(s.FormCacheHits) / float64(total) * 100
}
