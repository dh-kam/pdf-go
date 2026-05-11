package renderer

import (
	"context"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
	"github.com/dh-kam/pdf-go/internal/infrastructure/canvas"
)

type testXRef struct{}

func (testXRef) Fetch(entity.Ref) (entity.Object, error) { return nil, nil }
func (testXRef) FetchCached(entity.Ref) (entity.Object, bool) {
	return nil, false
}
func (testXRef) Cache(entity.Ref, entity.Object) {}
func (testXRef) GetCatalog() (*entity.Dict, error) {
	return nil, nil
}
func (testXRef) GetTrailer() (*entity.Dict, error) {
	return nil, nil
}
func (testXRef) GetNumObjects() int { return 0 }

func TestConcurrentRenderer_Defaults(t *testing.T) {
	renderer := NewConcurrentRenderer(RendererOptions{})
	assert.NotNil(t, renderer)

	assert.Equal(t, int64(0), renderer.Stats().RenderCount)
}

func TestConcurrentRenderer_RenderPage_WithCache(t *testing.T) {
	renderer := NewConcurrentRenderer(RendererOptions{
		CacheSize: 10,
		CacheTTL:  time.Minute,
	})

	page := entity.NewTestPage()
	options := domainrenderer.DefaultRenderOptions()
	options.EnableCache = true

	ctx := context.Background()

	image1, err := renderer.RenderPage(ctx, page, options)
	require.NoError(t, err)
	require.NotNil(t, image1)

	image2, err := renderer.RenderPage(ctx, page, options)
	require.NoError(t, err)
	require.NotNil(t, image2)

	stats := renderer.Stats()
	assert.Equal(t, int64(2), stats.RenderCount)
	assert.Equal(t, int64(1), stats.CacheHits)
	assert.Equal(t, int64(1), stats.CacheMisses)
}

func TestConcurrentRenderer_RenderPage_CacheKeySeparatesImageSamplingMode(t *testing.T) {
	renderer := NewConcurrentRenderer(RendererOptions{
		CacheSize: 10,
		CacheTTL:  time.Minute,
	})

	page := entity.NewTestPage()

	legacy := domainrenderer.DefaultRenderOptions()
	legacy.EnableCache = true
	legacy.ImageSamplingMode = domainrenderer.ImageSamplingModeLegacy

	experimental := domainrenderer.DefaultRenderOptions()
	experimental.EnableCache = true
	experimental.ImageSamplingMode = domainrenderer.ImageSamplingModeExperimentalSplashScaleOnlyV1

	_, err := renderer.RenderPage(context.Background(), page, legacy)
	require.NoError(t, err)
	_, err = renderer.RenderPage(context.Background(), page, experimental)
	require.NoError(t, err)

	stats := renderer.Stats()
	assert.Equal(t, int64(2), stats.CacheMisses)
	assert.Equal(t, int64(0), stats.CacheHits)
}

func TestConcurrentRenderer_RenderPage_CacheKeySeparatesScaleAndBackground(t *testing.T) {
	renderer := NewConcurrentRenderer(RendererOptions{
		CacheSize: 10,
		CacheTTL:  time.Minute,
	})

	page := entity.NewTestPage()

	base := domainrenderer.DefaultRenderOptions()
	base.EnableCache = true
	base.Scale = 1.0
	base.BackgroundColor = color.White

	variant := domainrenderer.DefaultRenderOptions()
	variant.EnableCache = true
	variant.Scale = 2.0
	variant.BackgroundColor = color.Black

	_, err := renderer.RenderPage(context.Background(), page, base)
	require.NoError(t, err)
	_, err = renderer.RenderPage(context.Background(), page, variant)
	require.NoError(t, err)

	stats := renderer.Stats()
	assert.Equal(t, int64(2), stats.CacheMisses)
	assert.Equal(t, int64(0), stats.CacheHits)
}

func TestConcurrentRenderer_RenderPage_WithContentStream(t *testing.T) {
	doc := entity.NewDocument(testXRef{})
	dict := entity.NewDict()
	dict.Set(entity.Name("/Contents"), entity.NewStream(entity.NewDict(), []byte("q Q")))
	page := entity.NewPage(doc, dict, entity.NewRef(1, 0), 0)

	renderer := NewConcurrentRenderer(RendererOptions{})

	options := domainrenderer.DefaultRenderOptions()
	options.EnableCache = false

	img, err := renderer.RenderPage(context.Background(), page, options)
	require.NoError(t, err)
	require.NotNil(t, img)

	stats := renderer.Stats()
	assert.Equal(t, int64(1), stats.RenderCount)
}

func TestConcurrentRenderer_RenderToCanvas_ContextCancelled(t *testing.T) {
	doc := entity.NewDocument(testXRef{})
	dict := entity.NewDict()
	dict.Set(entity.Name("/Contents"), entity.NewStream(entity.NewDict(), []byte("q Q")))
	page := entity.NewPage(doc, dict, entity.NewRef(2, 0), 1)

	renderer := NewConcurrentRenderer(RendererOptions{})
	imageCanvas := canvas.NewImageCanvas(image.Rect(0, 0, 10, 10))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := renderer.RenderToCanvas(ctx, page, imageCanvas)
	assert.Equal(t, context.Canceled, err)
}

func TestConcurrentRenderer_StatsHitRate(t *testing.T) {
	stats := RendererStats{
		RenderCount: 10,
		CacheHits:   3,
		CacheMisses: 7,
	}

	assert.InDelta(t, 30.0, stats.HitRate(), 0.0001)

	zero := RendererStats{}
	assert.Equal(t, 0.0, zero.HitRate())
	assert.Equal(t, 0.0, zero.FormCacheHitRate())
}

func TestConcurrentRenderer_StatsFormCacheHitRate(t *testing.T) {
	stats := RendererStats{
		FormCacheHits:   9,
		FormCacheMisses: 3,
	}
	assert.InDelta(t, 75.0, stats.FormCacheHitRate(), 0.0001)
}

func TestConcurrentRenderer_RenderPage_AppliesDPIAndCeilPixelSizing(t *testing.T) {
	doc := entity.NewDocument(testXRef{})
	dict := entity.NewDict()
	dict.Set(entity.Name("/MediaBox"), entity.NewArray(
		entity.NewReal(0),
		entity.NewReal(0),
		entity.NewReal(3.84),
		entity.NewReal(3.84),
	))
	page := entity.NewPage(doc, dict, entity.NewRef(3, 0), 0)

	renderer := NewConcurrentRenderer(RendererOptions{})
	options := domainrenderer.DefaultRenderOptions()
	options.EnableCache = false

	options.DPI = 72
	options.Scale = 1.0
	img72, err := renderer.RenderPage(context.Background(), page, options)
	require.NoError(t, err)
	assert.Equal(t, 4, img72.Bounds().Dx())
	assert.Equal(t, 4, img72.Bounds().Dy())

	options.DPI = 150
	img150, err := renderer.RenderPage(context.Background(), page, options)
	require.NoError(t, err)
	assert.Equal(t, 8, img150.Bounds().Dx())
	assert.Equal(t, 8, img150.Bounds().Dy())
}

func TestPointsToPixels(t *testing.T) {
	assert.Equal(t, 4, pointsToPixels(3.84, 72, 1.0))
	assert.Equal(t, 8, pointsToPixels(3.84, 150, 1.0))
	assert.Equal(t, 8, pointsToPixels(3.84, 72, 2.0))
	assert.Equal(t, 1, pointsToPixels(0, 72, 1.0))
}

func TestDefaultFormCacheSize(t *testing.T) {
	assert.Equal(t, 16, defaultFormCacheSize(0))
	assert.Equal(t, 16, defaultFormCacheSize(1))
	assert.Equal(t, 80, defaultFormCacheSize(20))
	assert.Equal(t, 256, defaultFormCacheSize(100))
}

func TestRenderOptionsContextRoundTrip(t *testing.T) {
	base := context.Background()
	defaults := renderOptionsFromContext(base)
	assert.Equal(t, domainrenderer.DefaultRenderOptions(), defaults)

	opts := domainrenderer.DefaultRenderOptions()
	opts.DebugImageSampling = true
	opts.DPI = 150
	ctx := withRenderOptions(base, opts)

	got := renderOptionsFromContext(ctx)
	assert.Equal(t, opts, got)
}

func TestRenderCacheKeyIncludesOutputAffectingOptions(t *testing.T) {
	page := entity.NewTestPage()

	base := domainrenderer.DefaultRenderOptions()
	base.DPI = 72
	base.Scale = 1
	base.ImageSamplingMode = domainrenderer.ImageSamplingModeLegacy
	base.BackgroundColor = color.White

	modeVariant := base
	modeVariant.ImageSamplingMode = domainrenderer.ImageSamplingModeExperimentalSplashScaleOnlyV1

	scaleVariant := base
	scaleVariant.Scale = 2

	bgVariant := base
	bgVariant.BackgroundColor = color.Black

	assert.NotEqual(t, renderCacheKey(page, base), renderCacheKey(page, modeVariant))
	assert.NotEqual(t, renderCacheKey(page, base), renderCacheKey(page, scaleVariant))
	assert.NotEqual(t, renderCacheKey(page, base), renderCacheKey(page, bgVariant))
}

func TestResolveFormCacheForOptions(t *testing.T) {
	r := NewConcurrentRenderer(RendererOptions{})

	opts := domainrenderer.DefaultRenderOptions()
	opts.EnableCache = true
	assert.NotNil(t, r.resolveFormCacheForOptions(opts))

	opts.EnableCache = false
	assert.Nil(t, r.resolveFormCacheForOptions(opts))
}
