// Package renderer provides concurrent page rendering implementation.
//
//revive:disable:exported
package renderer

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	domaincache "github.com/dh-kam/pdf-go/internal/domain/cache"
	domaincanvas "github.com/dh-kam/pdf-go/internal/domain/canvas"
	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
	"github.com/dh-kam/pdf-go/internal/infrastructure/cache"
	"github.com/dh-kam/pdf-go/internal/infrastructure/canvas"
)

// ConcurrentRenderer implements concurrent page rendering.
type ConcurrentRenderer struct {
	pageCache   domaincache.PageCache
	formCache   domainrenderer.FormOperatorCache
	bytePool    domaincache.BytePool
	maxWorkers  int
	backend     string
	renderCount atomic.Int64
	cacheHits   atomic.Int64
	cacheMisses atomic.Int64
}

type renderOptionsContextKey struct{}

// RendererOptions represents options for creating a renderer.
type RendererOptions struct {
	// MaxWorkers is the maximum number of concurrent render workers.
	MaxWorkers int
	// CacheSize is the maximum number of pages to cache.
	CacheSize int
	// CacheTTL is the time-to-live for cached pages.
	CacheTTL time.Duration
	// FormCacheSize is the maximum number of cached Form XObject operator sets.
	FormCacheSize int
	// FormCacheTTL is the time-to-live for cached Form XObject operators.
	FormCacheTTL time.Duration
	// Backend selects the page-level rendering backend. Valid values are
	// canvas.BackendImageCanvas (default) and canvas.BackendSplash. An empty
	// string is equivalent to canvas.BackendImageCanvas.
	Backend string
}

// NewConcurrentRenderer creates a new concurrent renderer.
func NewConcurrentRenderer(options RendererOptions) *ConcurrentRenderer {
	if options.MaxWorkers <= 0 {
		options.MaxWorkers = 4 // Default to 4 workers
	}
	if options.CacheSize <= 0 {
		options.CacheSize = 20 // Default to 20 cached pages
	}
	if options.CacheTTL <= 0 {
		options.CacheTTL = 10 * time.Minute // Default TTL
	}
	if options.FormCacheSize <= 0 {
		options.FormCacheSize = defaultFormCacheSize(options.CacheSize)
	}
	if options.FormCacheTTL <= 0 {
		options.FormCacheTTL = options.CacheTTL
	}

	pageCacheConfig := domaincache.CacheConfig{
		MaxSize: options.CacheSize,
		TTL:     options.CacheTTL,
	}

	return &ConcurrentRenderer{
		pageCache:  cache.NewPageCache(pageCacheConfig),
		formCache:  newFormOperatorCache(options.FormCacheSize, options.FormCacheTTL),
		bytePool:   cache.NewBytePool(domaincache.PoolConfig{MinSize: 10, MaxSize: 100}),
		maxWorkers: options.MaxWorkers,
		backend:    options.Backend,
	}
}

// RenderPage renders a single page to an image.
func (r *ConcurrentRenderer) RenderPage(ctx context.Context, page *entity.Page, options domainrenderer.RenderOptions) (image.Image, error) {
	// Apply defaults
	if options.DPI <= 0 {
		options.DPI = 72.0
	}
	if options.Scale <= 0 {
		options.Scale = 1.0
	}

	// Check cache if enabled
	if options.EnableCache {
		cacheKey := renderCacheKey(page, options)
		cached, ok := r.pageCache.Get(ctx, cacheKey)
		if ok {
			r.cacheHits.Add(1)
			r.renderCount.Add(1)
			if img, ok := cached.(image.Image); ok {
				return img, nil
			}
		}
		r.cacheMisses.Add(1)
	}

	// Create canvas from the visible page box, honoring page rotation.
	pageBox := page.CropBox()
	xMin := math.Min(pageBox[0], pageBox[2])
	xMax := math.Max(pageBox[0], pageBox[2])
	yMin := math.Min(pageBox[1], pageBox[3])
	yMax := math.Max(pageBox[1], pageBox[3])
	pageWidth := xMax - xMin
	pageHeight := yMax - yMin
	rotation := normalizePageRotation(page.Rotate())
	if rotation == 90 || rotation == 270 {
		pageWidth, pageHeight = pageHeight, pageWidth
	}

	width := pointsToPixels(pageWidth, options.DPI, options.Scale)
	height := pointsToPixels(pageHeight, options.DPI, options.Scale)

	bgColor := options.BackgroundColor
	if bgColor == nil {
		bgColor = color.White
	}

	c := canvas.NewCanvas(width, height, r.backend)
	if paperCanvas, ok := c.(interface{ SetOpaquePaperBackground(color.Color) }); ok {
		paperCanvas.SetOpaquePaperBackground(bgColor)
	} else {
		// Fill background before rendering page contents.
		c.SetFillColor(bgColor)
		c.Rectangle(0, 0, float64(width), float64(height))
		c.Fill()
	}

	// Render to canvas.
	ctxWithRenderOptions := withRenderOptions(ctx, options)
	if err := r.RenderToCanvas(ctxWithRenderOptions, page, c); err != nil {
		return nil, fmt.Errorf("render page %d: %w", page.Index(), err)
	}

	img := c.Image()
	r.renderCount.Add(1)

	// Cache the result if enabled
	if options.EnableCache {
		cacheKey := renderCacheKey(page, options)
		if err := r.pageCache.Set(ctx, cacheKey, img); err != nil {
			return nil, fmt.Errorf("cache page %d: %w", page.Index(), err)
		}
	}

	return img, nil
}

func renderCacheKey(page *entity.Page, options domainrenderer.RenderOptions) domaincache.CacheKey {
	backgroundKey := renderBackgroundCacheKey(options.BackgroundColor)
	modeKey := strings.TrimSpace(strings.ToLower(options.ImageSamplingMode))
	if page == nil {
		return domaincache.StringKey(fmt.Sprintf(
			"nil_page_%.3f_%.3f_%s_%s",
			options.DPI,
			options.Scale,
			modeKey,
			backgroundKey,
		))
	}

	if doc := page.Document(); doc != nil {
		return domaincache.StringKey(fmt.Sprintf(
			"doc_%p_page_%d_%.3f_%.3f_%s_%s",
			doc,
			page.Index(),
			options.DPI,
			options.Scale,
			modeKey,
			backgroundKey,
		))
	}

	// Fallback for test pages detached from documents.
	return domaincache.StringKey(fmt.Sprintf(
		"page_%p_page_%d_%.3f_%.3f_%s_%s",
		page,
		page.Index(),
		options.DPI,
		options.Scale,
		modeKey,
		backgroundKey,
	))
}

func renderBackgroundCacheKey(c color.Color) string {
	if c == nil {
		return "nil"
	}
	r, g, b, a := c.RGBA()
	return fmt.Sprintf("%04x%04x%04x%04x", r, g, b, a)
}

func pointsToPixels(points, dpi, scale float64) int {
	if points < 0 {
		points = -points
	}
	pixels := points * (dpi / 72.0) * scale

	// Guard against floating-point noise on values that are mathematically integers.
	roundedUp := int(math.Ceil(pixels - 1e-9))
	if roundedUp < 1 {
		return 1
	}
	return roundedUp
}

// RenderPages renders multiple pages concurrently.
func (r *ConcurrentRenderer) RenderPages(ctx context.Context, doc *entity.Document, pageNumbers []int, options domainrenderer.RenderOptions) <-chan domainrenderer.RenderResult {
	resultChan := make(chan domainrenderer.RenderResult, len(pageNumbers))

	var wg sync.WaitGroup
	workerSemaphore := make(chan struct{}, r.maxWorkers)

	for _, pageNum := range pageNumbers {
		wg.Add(1)

		go func(pn int) {
			defer wg.Done()

			// Acquire worker slot
			select {
			case workerSemaphore <- struct{}{}:
				defer func() { <-workerSemaphore }()
			case <-ctx.Done():
				resultChan <- domainrenderer.RenderResult{
					PageNum: pn,
					Error:   ctx.Err(),
				}
				return
			}

			// Get page
			page, err := doc.GetPage(pn)
			if err != nil {
				resultChan <- domainrenderer.RenderResult{
					PageNum: pn,
					Error:   fmt.Errorf("get page %d: %w", pn, err),
				}
				return
			}

			// Render page
			img, err := r.RenderPage(ctx, page, options)
			resultChan <- domainrenderer.RenderResult{
				PageNum: pn,
				Image:   img,
				Error:   err,
			}
		}(pageNum)
	}

	// Close channel when all workers complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	return resultChan
}

// RenderAllPages renders all pages in the document concurrently.
func (r *ConcurrentRenderer) RenderAllPages(ctx context.Context, doc *entity.Document, options domainrenderer.RenderOptions) <-chan domainrenderer.RenderResult {
	pageCount, err := doc.PageCount()
	if err != nil {
		resultChan := make(chan domainrenderer.RenderResult, 1)
		go func() {
			resultChan <- domainrenderer.RenderResult{
				PageNum: 0,
				Error:   fmt.Errorf("get page count: %w", err),
			}
			close(resultChan)
		}()
		return resultChan
	}

	pageNumbers := make([]int, pageCount)
	for i := 0; i < pageCount; i++ {
		pageNumbers[i] = i
	}

	return r.RenderPages(ctx, doc, pageNumbers, options)
}

// RenderToCanvas renders a page to a canvas.
func (r *ConcurrentRenderer) RenderToCanvas(ctx context.Context, page *entity.Page, c domaincanvas.Canvas) error {
	// Get page contents
	contents, err := page.Contents()
	if err != nil {
		return fmt.Errorf("get page contents: %w", err)
	}
	doc := page.Document()
	if doc == nil {
		if len(contents) == 0 {
			return nil
		}
		return fmt.Errorf("page document is nil")
	}

	// Create evaluator
	evaluator := domainrenderer.NewEvaluator(doc.XRef())
	evaluator.SetCanvas(c)
	renderOpts := renderOptionsFromContext(ctx)
	if formCache := r.resolveFormCacheForOptions(renderOpts); formCache != nil {
		evaluator.SetFormOperatorCache(formCache)
	}
	evaluator.SetImageSamplingMode(renderOpts.ImageSamplingMode)
	if renderOpts.DebugImageSampling {
		docID := "nil"
		if doc := page.Document(); doc != nil {
			docID = fmt.Sprintf("%p", doc)
		}
		evaluator.SetImageSamplingDebug(true, docID, page.Index()+1)
	}
	if resources, err := page.Resources(); err == nil && resources != nil {
		evaluator.SetResources(resources)
	}

	pageBox := page.CropBox()
	xMin := math.Min(pageBox[0], pageBox[2])
	xMax := math.Max(pageBox[0], pageBox[2])
	yMin := math.Min(pageBox[1], pageBox[3])
	yMax := math.Max(pageBox[1], pageBox[3])
	pageWidth := xMax - xMin
	pageHeight := yMax - yMin
	initial := [6]float64{1, 0, 0, 1, 0, 0}
	pageYOriginPx := 0.0
	initialSet := false
	if pageWidth > 0 && pageHeight > 0 {
		rotation := normalizePageRotation(page.Rotate())

		// Use DPI/72 as the scale factor to match Poppler's coordinate system exactly.
		// Using int_width/pageWidth introduces a tiny scale error that accumulates across
		// the page, causing text glyphs to appear shifted by ~1 pixel vs Poppler's output.
		dpi := renderOpts.DPI
		if dpi <= 0 {
			dpi = 72.0
		}
		scale := renderOpts.Scale
		if scale <= 0 {
			scale = 1.0
		}
		scaleFactor := dpi * scale / 72.0
		scaleX := scaleFactor
		scaleY := scaleFactor
		if rotation == 90 || rotation == 270 {
			scaleX = scaleFactor
			scaleY = scaleFactor
		}

		// Compute the exact float Y origin for glyph baseline calculation.
		// Without this, DrawText falls back to the integer canvas height (which is
		// ceil(...) of a float), introducing a sub-pixel error that shifts text by
		// ~1 pixel vs Poppler when the baseline lands near a pixel boundary.
		//   - rotations 0/180: canvas Y origin = (yMax - yMin) * scaleY
		//   - rotations 90/270: canvas physical height comes from the original page
		//     width (because pageWidth/pageHeight are swapped before pointsToPixels),
		//     so the Y origin = (xMax - xMin) * scaleY (== pageWidth_orig * scaleY).
		// In every rotation the initial transform leaves the glyph Y in a coordinate
		// system that DrawText flips with `glyphYOrigin - y`, so all four cases
		// benefit from the exact float origin.
		type pageYOriginSetter interface {
			SetPageYOriginPx(yOrigin float64)
		}
		setPageYOrigin := func(yOrigin float64) {
			if setter, ok := c.(pageYOriginSetter); ok {
				pageYOriginPx = yOrigin
				setter.SetPageYOriginPx(pageYOriginPx)
			}
		}

		switch rotation {
		case 90:
			// x' = (y - yMin), y' = (xMin + pageWidth - x)
			initial = [6]float64{
				0,
				-scaleY,
				scaleX,
				0,
				-yMin * scaleX,
				(xMin + pageWidth) * scaleY,
			}
			setPageYOrigin((xMax - xMin) * scaleY)
		case 180:
			// x' = (xMin + pageWidth - x), y' = (yMin + pageHeight - y)
			initial = [6]float64{
				-scaleX,
				0,
				0,
				-scaleY,
				(xMin + pageWidth) * scaleX,
				(yMin + pageHeight) * scaleY,
			}
			setPageYOrigin((yMax - yMin) * scaleY)
		case 270:
			// x' = (yMin + pageHeight - y), y' = (x - xMin)
			initial = [6]float64{
				0,
				scaleY,
				-scaleX,
				0,
				(yMin + pageHeight) * scaleX,
				-xMin * scaleY,
			}
			setPageYOrigin((xMax - xMin) * scaleY)
		default:
			initial = [6]float64{
				scaleX,
				0,
				0,
				scaleY,
				-xMin * scaleX,
				0, // Y transform is handled in DrawText (baseCanvasY = height - y)
			}
			setPageYOrigin((yMax - yMin) * scaleY)
		}

		evaluator.SetInitialTransform(initial)
		initialSet = true
	}

	// Check cancellation before running full evaluation.
	for range contents {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	if len(contents) > 0 {
		if err := evaluator.Evaluate(contents); err != nil {
			return fmt.Errorf("process content: %w", err)
		}
	}

	if initialSet {
		if err := r.renderPageAnnotations(ctx, page, c, initial, pageYOriginPx); err != nil {
			return err
		}
	}

	return nil
}

// Stats returns rendering statistics.
func (r *ConcurrentRenderer) Stats() RendererStats {
	formStats := formOperatorCacheStats{}
	if fc, ok := r.formCache.(*formOperatorCache); ok {
		formStats = fc.Stats()
	}

	return RendererStats{
		RenderCount:     r.renderCount.Load(),
		CacheHits:       r.cacheHits.Load(),
		CacheMisses:     r.cacheMisses.Load(),
		FormCacheHits:   formStats.Hits,
		FormCacheMisses: formStats.Misses,
		FormCacheSets:   formStats.Sets,
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

// FormCacheHitRate returns the form operator cache hit rate as a percentage.
func (s RendererStats) FormCacheHitRate() float64 {
	total := s.FormCacheHits + s.FormCacheMisses
	if total == 0 {
		return 0
	}
	return float64(s.FormCacheHits) / float64(total) * 100
}

func normalizePageRotation(rotation int) int {
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

func withRenderOptions(ctx context.Context, opts domainrenderer.RenderOptions) context.Context {
	return context.WithValue(ctx, renderOptionsContextKey{}, opts)
}

func renderOptionsFromContext(ctx context.Context) domainrenderer.RenderOptions {
	if ctx == nil {
		return domainrenderer.DefaultRenderOptions()
	}
	if v := ctx.Value(renderOptionsContextKey{}); v != nil {
		if opts, ok := v.(domainrenderer.RenderOptions); ok {
			return opts
		}
	}
	return domainrenderer.DefaultRenderOptions()
}

func (r *ConcurrentRenderer) resolveFormCacheForOptions(opts domainrenderer.RenderOptions) domainrenderer.FormOperatorCache {
	if !opts.EnableCache {
		return nil
	}
	return r.formCache
}

func defaultFormCacheSize(pageCacheSize int) int {
	size := pageCacheSize * 4
	if size < 16 {
		return 16
	}
	if size > 256 {
		return 256
	}
	return size
}
