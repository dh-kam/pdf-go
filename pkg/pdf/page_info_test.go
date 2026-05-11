package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestGetPageCountAlias(t *testing.T) {
	doc := newDocumentWithPageInfo([]pageInfoSpec{
		{mediaBox: [4]float64{0, 0, 100, 200}, rotate: 0},
		{mediaBox: [4]float64{0, 0, 300, 400}, rotate: 90},
	})

	count, err := doc.GetPageCount()
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestGetPageMetricsAliases(t *testing.T) {
	doc := newDocumentWithPageInfo([]pageInfoSpec{
		{mediaBox: [4]float64{0, 0, 200, 500}, rotate: 95},
	})

	width, err := doc.GetPageWidth(0)
	require.NoError(t, err)
	assert.Equal(t, 200.0, width)

	height, err := doc.GetPageHeight(0)
	require.NoError(t, err)
	assert.Equal(t, 500.0, height)

	rotate, err := doc.GetPageRotate(0)
	require.NoError(t, err)
	assert.Equal(t, 90, rotate)
}

func TestGetPageMetricsAliases_InvalidPageIndex(t *testing.T) {
	doc := newDocumentWithPageInfo([]pageInfoSpec{
		{mediaBox: [4]float64{0, 0, 100, 100}, rotate: 0},
	})

	_, err := doc.GetPageWidth(1)
	require.Error(t, err)

	_, err = doc.GetPageHeight(-1)
	require.Error(t, err)

	_, err = doc.GetPageRotate(99)
	require.Error(t, err)
}

func TestGetSinglePageMetricsAndInvalidZoomError(t *testing.T) {
	doc := newDocumentWithPageInfo([]pageInfoSpec{
		{mediaBox: [4]float64{0, 0, 100, 200}, rotate: 0},
	})

	width, err := doc.GetSinglePageWidth(0, 1.5)
	require.NoError(t, err)
	assert.Equal(t, 150, width)

	height, err := doc.GetSinglePageHeight(0, 0.5)
	require.NoError(t, err)
	assert.Equal(t, 100, height)

	_, err = doc.GetSinglePageWidth(0, 0)
	require.ErrorIs(t, err, ErrInvalidZoom)
	assert.Equal(t, "zoom must be positive", ErrInvalidZoom.Error())
}

type pageInfoSpec struct {
	mediaBox [4]float64
	rotate   int
}

func newDocumentWithPageInfo(specs []pageInfoSpec) *Document {
	entityDoc := entity.NewDocument(nil)

	kidItems := make([]entity.Object, 0, len(specs))
	for _, spec := range specs {
		page := entity.NewDict()
		page.Set(entity.Name("Type"), entity.NewName("Page"))
		page.Set(entity.Name("MediaBox"), entity.NewArray(
			entity.NewReal(spec.mediaBox[0]),
			entity.NewReal(spec.mediaBox[1]),
			entity.NewReal(spec.mediaBox[2]),
			entity.NewReal(spec.mediaBox[3]),
		))
		page.Set(entity.Name("Rotate"), entity.NewInteger(int64(spec.rotate)))
		kidItems = append(kidItems, page)
	}
	kids := entity.NewArray(kidItems...)

	pages := entity.NewDict()
	pages.Set(entity.Name("Type"), entity.NewName("Pages"))
	pages.Set(entity.Name("Count"), entity.NewInteger(int64(len(specs))))
	pages.Set(entity.Name("Kids"), kids)

	catalog := entity.NewDict()
	catalog.Set(entity.Name("Type"), entity.NewName("Catalog"))
	catalog.Set(entity.Name("Pages"), pages)

	entityDoc.SetCatalog(catalog)
	return newDocument(entityDoc)
}
