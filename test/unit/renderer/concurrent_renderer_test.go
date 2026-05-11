// Package renderer_test provides tests for concurrent page rendering.
package renderer_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
	"github.com/dh-kam/pdf-go/internal/infrastructure/renderer"
)

func TestNewConcurrentRenderer(t *testing.T) {
	options := renderer.RendererOptions{
		MaxWorkers: 8,
		CacheSize:  50,
		CacheTTL:   30 * time.Minute,
	}

	r := renderer.NewConcurrentRenderer(options)
	assert.NotNil(t, r)
}

func TestNewConcurrentRenderer_Defaults(t *testing.T) {
	options := renderer.RendererOptions{}

	r := renderer.NewConcurrentRenderer(options)
	assert.NotNil(t, r)
	// Defaults should be applied
	stats := r.Stats()
	assert.Equal(t, int64(0), stats.RenderCount)
}

func TestConcurrentRenderer_RenderPage(t *testing.T) {
	r := renderer.NewConcurrentRenderer(renderer.RendererOptions{})

	// Create a test page
	page := entity.NewTestPage()

	options := domainrenderer.DefaultRenderOptions()
	options.EnableCache = false

	ctx := context.Background()
	img, err := r.RenderPage(ctx, page, options)

	// For a test page with no content, we expect a valid image (even if empty)
	assert.NoError(t, err)
	assert.NotNil(t, img)

	stats := r.Stats()
	assert.Equal(t, int64(1), stats.RenderCount)
}

func TestConcurrentRenderer_RenderPage_WithCache(t *testing.T) {
	r := renderer.NewConcurrentRenderer(renderer.RendererOptions{
		CacheSize: 10,
		CacheTTL:  time.Minute,
	})

	// Create a test page
	page := entity.NewTestPage()

	options := domainrenderer.DefaultRenderOptions()
	options.EnableCache = true
	options.DPI = 72.0

	ctx := context.Background()

	// First render - should be a cache miss
	img1, err := r.RenderPage(ctx, page, options)
	require.NoError(t, err)
	require.NotNil(t, img1)

	stats := r.Stats()
	assert.Equal(t, int64(1), stats.RenderCount)
	assert.Equal(t, int64(0), stats.CacheHits)
	assert.Equal(t, int64(1), stats.CacheMisses)

	// Second render - should be a cache hit
	img2, err := r.RenderPage(ctx, page, options)
	require.NoError(t, err)
	require.NotNil(t, img2)

	stats = r.Stats()
	assert.Equal(t, int64(2), stats.RenderCount)
	assert.Equal(t, int64(1), stats.CacheHits)
	assert.Equal(t, int64(1), stats.CacheMisses)

	// Different DPI - should be a cache miss
	options.DPI = 150.0
	img3, err := r.RenderPage(ctx, page, options)
	require.NoError(t, err)
	require.NotNil(t, img3)

	stats = r.Stats()
	assert.Equal(t, int64(3), stats.RenderCount)
	assert.Equal(t, int64(1), stats.CacheHits)
	assert.Equal(t, int64(2), stats.CacheMisses)
}

func TestConcurrentRenderer_RenderPage_WithCache_DifferentDocuments(t *testing.T) {
	r := renderer.NewConcurrentRenderer(renderer.RendererOptions{
		CacheSize: 10,
		CacheTTL:  time.Minute,
	})

	options := domainrenderer.DefaultRenderOptions()
	options.EnableCache = true
	options.DPI = 72.0

	ctx := context.Background()

	docA := entity.NewDocument(nil)
	docB := entity.NewDocument(nil)
	pageA := newTestPageWithMediaBox(docA, 100, 100)
	pageB := newTestPageWithMediaBox(docB, 200, 200)

	imgA, err := r.RenderPage(ctx, pageA, options)
	require.NoError(t, err)
	require.NotNil(t, imgA)
	assert.Equal(t, 100, imgA.Bounds().Dx())
	assert.Equal(t, 100, imgA.Bounds().Dy())

	imgB, err := r.RenderPage(ctx, pageB, options)
	require.NoError(t, err)
	require.NotNil(t, imgB)
	assert.Equal(t, 200, imgB.Bounds().Dx())
	assert.Equal(t, 200, imgB.Bounds().Dy())

	// Re-render docA/page0 should hit cache and still keep doc-specific dimensions.
	imgA2, err := r.RenderPage(ctx, pageA, options)
	require.NoError(t, err)
	require.NotNil(t, imgA2)
	assert.Equal(t, 100, imgA2.Bounds().Dx())
	assert.Equal(t, 100, imgA2.Bounds().Dy())

	stats := r.Stats()
	assert.Equal(t, int64(1), stats.CacheHits)
	assert.Equal(t, int64(2), stats.CacheMisses)
}

func newTestPageWithMediaBox(doc *entity.Document, width, height int) *entity.Page {
	dict := entity.NewDict()
	mediaBox := entity.NewArray(
		entity.NewInteger(0),
		entity.NewInteger(0),
		entity.NewInteger(int64(width)),
		entity.NewInteger(int64(height)),
	)
	dict.Set(entity.Name("MediaBox"), mediaBox)
	return entity.NewPage(doc, dict, entity.Ref{}, 0)
}

func TestConcurrentRenderer_RenderPages_Concurrent(t *testing.T) {
	r := renderer.NewConcurrentRenderer(renderer.RendererOptions{
		MaxWorkers: 2,
	})

	// Mock document - for now we'll just test the channel behavior
	// This will need to be updated when GetPage is implemented
	options := domainrenderer.DefaultRenderOptions()
	options.EnableCache = false

	ctx := context.Background()

	// Create a simple mock document for testing
	doc := entity.NewDocument(nil)

	// Request pages 0, 1, 2
	resultChan := r.RenderPages(ctx, doc, []int{0, 1, 2}, options)

	results := []domainrenderer.RenderResult{}
	for result := range resultChan {
		results = append(results, result)
	}

	// Should get 3 results
	assert.Len(t, results, 3)

	// All should have errors (since GetPage is not implemented)
	for _, result := range results {
		assert.Error(t, result.Error)
	}
}

func TestConcurrentRenderer_RenderPages_ContextCancellation(t *testing.T) {
	r := renderer.NewConcurrentRenderer(renderer.RendererOptions{
		MaxWorkers: 2,
	})

	ctx, cancel := context.WithCancel(context.Background())

	doc := entity.NewDocument(nil)

	options := domainrenderer.DefaultRenderOptions()

	// Start rendering
	resultChan := r.RenderPages(ctx, doc, []int{0, 1, 2, 3, 4}, options)

	// Cancel after first result
	cancel()

	results := []domainrenderer.RenderResult{}
	for result := range resultChan {
		results = append(results, result)
		// Break after getting some results
		if len(results) >= 2 {
			break
		}
	}

	// Should get some results before cancellation
	assert.GreaterOrEqual(t, len(results), 1)
}

func TestConcurrentRenderer_RenderAllPages(t *testing.T) {
	r := renderer.NewConcurrentRenderer(renderer.RendererOptions{})

	// Create a document with a catalog
	doc := entity.NewDocument(nil)
	catalog := entity.NewDict()
	catalog.Set(entity.Name("Type"), entity.Name("Catalog"))
	catalog.Set(entity.Name("Pages"), entity.NewDict())
	doc.SetCatalog(catalog)

	options := domainrenderer.DefaultRenderOptions()

	ctx := context.Background()
	resultChan := r.RenderAllPages(ctx, doc, options)

	// Should get at least one result (possibly an error since pages aren't set up)
	results := []domainrenderer.RenderResult{}
	for result := range resultChan {
		results = append(results, result)
	}

	// We expect some results even if they're errors
	assert.NotEmpty(t, results)
}

func TestRendererStats_HitRate(t *testing.T) {
	stats := renderer.RendererStats{
		RenderCount: 100,
		CacheHits:   75,
		CacheMisses: 25,
	}

	hitRate := stats.HitRate()
	assert.Equal(t, 75.0, hitRate)

	// Test with no cache activity
	emptyStats := renderer.RendererStats{}
	assert.Equal(t, 0.0, emptyStats.HitRate())
}

func TestDefaultRenderOptions(t *testing.T) {
	options := domainrenderer.DefaultRenderOptions()

	assert.Equal(t, 72.0, options.DPI)
	assert.Equal(t, 1.0, options.Scale)
	assert.True(t, options.EnableCache)
	assert.Nil(t, options.BackgroundColor)
}
