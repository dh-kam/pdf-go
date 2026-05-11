package pdf

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	domainrenderer "github.com/dh-kam/pdf-go/internal/domain/renderer"
)

func TestOpenWrappers_OpenAndOpenReader(t *testing.T) {
	pdfPath := samplePDFPath(t)
	pdfData, err := os.ReadFile(pdfPath)
	require.NoError(t, err)

	docFromPath, err := Open(pdfPath)
	require.NoError(t, err)
	require.NotNil(t, docFromPath)
	assert.Equal(t, "LocalPath", docFromPath.openFrom)
	assert.False(t, docFromPath.streamingForOpen)
	require.NoError(t, docFromPath.Close())

	docWithPassword, err := OpenWithPassword(pdfPath, "")
	require.NoError(t, err)
	require.NotNil(t, docWithPassword)
	require.NoError(t, docWithPassword.Close())

	docFromReader, err := OpenReader(bytes.NewReader(pdfData))
	require.NoError(t, err)
	require.NotNil(t, docFromReader)
	assert.Equal(t, "Stream", docFromReader.openFrom)
	assert.True(t, docFromReader.streamingForOpen)
	require.NoError(t, docFromReader.Close())

	docFromReaderWithPassword, err := OpenReaderWithPassword(bytes.NewReader(pdfData), "")
	require.NoError(t, err)
	require.NotNil(t, docFromReaderWithPassword)
	assert.Equal(t, "Stream", docFromReaderWithPassword.openFrom)
	assert.True(t, docFromReaderWithPassword.streamingForOpen)
	require.NoError(t, docFromReaderWithPassword.Close())
}

func TestOpenWrappers_RejectInvalidData(t *testing.T) {
	doc, err := OpenReader(bytes.NewReader([]byte("not a pdf file")))
	assert.Nil(t, doc)
	assert.Error(t, err)
}

func TestDocumentAndPageWrappers(t *testing.T) {
	doc := newDocumentWithPageInfo([]pageInfoSpec{
		{mediaBox: [4]float64{0, 0, 100, 200}, rotate: 95},
	})

	pageCount, err := doc.PageCount()
	require.NoError(t, err)
	assert.Equal(t, 1, pageCount)

	page, err := doc.Page(0)
	require.NoError(t, err)
	require.NotNil(t, page)
	assert.Equal(t, 0, page.Index())
	assert.Equal(t, [4]float64{0, 0, 100, 200}, page.MediaBox())
	assert.Equal(t, [4]float64{0, 0, 100, 200}, page.CropBox())
	assert.Equal(t, 90, page.Rotate())

	width, err := doc.GetPageWidth(0)
	require.NoError(t, err)
	assert.Equal(t, 100.0, width)

	height, err := doc.GetPageHeight(0)
	require.NoError(t, err)
	assert.Equal(t, 200.0, height)

	rotate, err := doc.GetPageRotate(0)
	require.NoError(t, err)
	assert.Equal(t, 90, rotate)

	err = doc.SetPageMediaBoxSL(0, [4]float64{5, 6, 110, 120})
	require.NoError(t, err)
	assert.Equal(t, [4]float64{5, 6, 110, 120}, page.MediaBox())

	err = doc.SetPageCropBoxSL(0, [4]float64{1, 2, 3, 4})
	require.NoError(t, err)
	assert.Equal(t, [4]float64{1, 2, 3, 4}, page.CropBox())

	err = doc.SetPageRotate(0, 450)
	require.NoError(t, err)
	assert.Equal(t, 90, page.Rotate())

	contents, err := page.Contents()
	require.NoError(t, err)
	assert.Empty(t, contents)

	annotations, err := page.Annotations()
	require.NoError(t, err)
	assert.Empty(t, annotations)

	resources, err := page.Resources()
	require.NoError(t, err)
	assert.NotNil(t, resources)
}

func TestPageMethodsAndFallbacks(t *testing.T) {
	doc := newDocument(entity.NewDocument(nil))
	assert.Nil(t, doc.Catalog())
	assert.Nil(t, doc.Info())

	entityDoc := entity.NewDocument(nil)
	catalog := entity.NewDict()
	catalog.Set(entity.Name("Type"), entity.NewName("Catalog"))
	entityDoc.SetCatalog(catalog)
	docFromCatalog := newDocument(entityDoc)
	assert.NotNil(t, docFromCatalog.Catalog())

	page := &Page{
		doc:         doc,
		page:        entity.NewTestPage(),
		sourceIndex: 0,
	}
	assert.Equal(t, 0, page.Index())
	assert.Equal(t, [4]float64{0, 0, 612, 792}, page.MediaBox())
	assert.Equal(t, [4]float64{0, 0, 612, 792}, page.CropBox())
	assert.Equal(t, 0, page.Rotate())
	assert.Equal(t, 612.0, page.Width())
	assert.Equal(t, 792.0, page.Height())

	contents, err := page.Contents()
	require.NoError(t, err)
	assert.NotNil(t, contents)
}

func TestRenderPages_Wrapper(t *testing.T) {
	doc := newDocumentWithPageInfo([]pageInfoSpec{
		{mediaBox: [4]float64{0, 0, 80, 120}, rotate: 0},
	})

	renderer := NewRenderer(DefaultRendererOptions())
	ctx := context.Background()

	result := <-renderer.RenderPages(ctx, doc, []int{0}, DefaultRenderOptions())
	assert.Equal(t, 0, result.PageNum)
	require.NoError(t, result.Error)
	assert.NotNil(t, result.Image)

	count := 0
	for range renderer.RenderAllPages(ctx, doc, DefaultRenderOptions()) {
		count++
	}
	assert.Equal(t, 1, count)
}

func TestDefaultRenderOptions_ExposeImageSamplingMode(t *testing.T) {
	opts := DefaultRenderOptions()
	assert.Equal(t, domainrenderer.ImageSamplingModeLegacy, opts.ImageSamplingMode)
}

func TestRenderPage_WrapperPassesImageSamplingMode(t *testing.T) {
	doc := newDocumentWithPageInfo([]pageInfoSpec{
		{mediaBox: [4]float64{0, 0, 4, 4}, rotate: 0},
	})

	renderer := NewRenderer(DefaultRendererOptions())
	page, err := doc.Page(0)
	require.NoError(t, err)

	opts := DefaultRenderOptions()
	opts.EnableCache = false
	opts.ImageSamplingMode = domainrenderer.ImageSamplingModeExperimentalSplashScaleOnlyV1

	img, err := renderer.RenderPage(context.Background(), page, opts)
	require.NoError(t, err)
	assert.NotNil(t, img)
	assert.Equal(t, 4, img.Bounds().Dx())
	assert.Equal(t, 4, img.Bounds().Dy())
}

func TestArrayWrapper_LenAndGet(t *testing.T) {
	nilArray := &Array{}
	assert.Equal(t, 0, nilArray.Len())
	assert.Nil(t, nilArray.Get(0))

	wrapped := &Array{
		array: entity.NewArray(
			entity.NewString("alpha"),
			entity.NewInteger(2),
		),
	}
	assert.Equal(t, 2, wrapped.Len())
	assert.Equal(t, "alpha", wrapped.Get(0))
	assert.Equal(t, 2, wrapped.Get(1))
}
