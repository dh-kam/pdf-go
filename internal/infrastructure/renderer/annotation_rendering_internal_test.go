package renderer

import (
	"image/color"
	"testing"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/stretchr/testify/assert"
)

func TestAnnotationDocumentNeedAppearances(t *testing.T) {
	doc := entity.NewDocument(nil)
	page := entity.NewPage(doc, entity.NewDict(), entity.Ref{}, 0)

	assert.False(t, annotationDocumentNeedAppearances(page))

	acroForm := entity.NewDict()
	acroForm.Set(entity.Name("NeedAppearances"), entity.NewBoolean(true))
	catalog := entity.NewDict()
	catalog.Set(entity.Name("AcroForm"), acroForm)
	doc.SetCatalog(catalog)

	assert.True(t, annotationDocumentNeedAppearances(page))

	acroForm.Set(entity.Name("NeedAppearances"), entity.NewBoolean(false))

	assert.False(t, annotationDocumentNeedAppearances(page))
}

func TestAnnotationWidgetUsesGeneratedAppearanceMatchesPopplerGate(t *testing.T) {
	doc := entity.NewDocument(nil)
	acroForm := entity.NewDict()
	acroForm.Set(entity.Name("NeedAppearances"), entity.NewBoolean(true))
	catalog := entity.NewDict()
	catalog.Set(entity.Name("AcroForm"), acroForm)
	doc.SetCatalog(catalog)
	page := entity.NewPage(doc, entity.NewDict(), entity.Ref{}, 0)

	assert.True(t, annotationWidgetUsesGeneratedAppearance(page, false))
	assert.True(t, annotationWidgetUsesGeneratedAppearance(page, true))

	acroForm.Set(entity.Name("NeedAppearances"), entity.NewBoolean(false))

	assert.True(t, annotationWidgetUsesGeneratedAppearance(page, false))
	assert.False(t, annotationWidgetUsesGeneratedAppearance(page, true))
}

func TestAnnotationExistingAppearanceUsesAnySubtypeNormalAppearance(t *testing.T) {
	doc := entity.NewDocument(nil)
	page := entity.NewPage(doc, entity.NewDict(), entity.Ref{}, 0)
	appearanceDict := entity.NewDict()
	appearanceDict.Set(entity.Name("BBox"), entity.NewArray(
		entity.NewReal(0),
		entity.NewReal(0),
		entity.NewReal(10),
		entity.NewReal(10),
	))
	appearance := entity.NewStream(appearanceDict, []byte("q Q"))
	ap := entity.NewDict()
	ap.Set(entity.Name("N"), appearance)
	dict := entity.NewDict()
	dict.Set(entity.Name("Subtype"), entity.Name("Highlight"))
	dict.Set(entity.Name("Rect"), entity.NewArray(
		entity.NewReal(10),
		entity.NewReal(20),
		entity.NewReal(30),
		entity.NewReal(40),
	))
	dict.Set(entity.Name("AP"), ap)

	gotAppearance, rect, ok := annotationExistingAppearance(page, dict)

	assert.True(t, ok)
	assert.Same(t, appearance, gotAppearance)
	assert.Equal(t, [4]float64{10, 20, 30, 40}, rect)
}

func TestAnnotationExistingAppearanceRequiresRectAndNormalAppearance(t *testing.T) {
	doc := entity.NewDocument(nil)
	page := entity.NewPage(doc, entity.NewDict(), entity.Ref{}, 0)
	dict := entity.NewDict()
	dict.Set(entity.Name("Subtype"), entity.Name("Highlight"))

	_, _, ok := annotationExistingAppearance(page, dict)

	assert.False(t, ok)

	dict.Set(entity.Name("Rect"), entity.NewArray(
		entity.NewReal(10),
		entity.NewReal(20),
		entity.NewReal(30),
		entity.NewReal(40),
	))

	_, _, ok = annotationExistingAppearance(page, dict)

	assert.False(t, ok)
}

func TestAnnotationTextNoteAppearanceStreamUsesPopplerFormBBox(t *testing.T) {
	stream := annotationTextNoteAppearanceStream(color.RGBA{R: 255, G: 128, B: 0, A: 255})
	dict := stream.Dict()

	assert.Equal(t, entity.Name("XObject"), dict.Get(entity.Name("Type")))
	assert.Equal(t, entity.Name("Form"), dict.Get(entity.Name("Subtype")))
	assert.Equal(t, int64(1), dict.Get(entity.Name("FormType")).(*entity.Integer).Value())

	bbox, ok := annotationNumberArray(dict.Get(entity.Name("BBox")))
	assert.True(t, ok)
	assert.Equal(t, []float64{0, 0, 24, 24}, bbox)
	assert.NotNil(t, dict.Get(entity.Name("Resources")))
	assert.Contains(t, string(stream.RawBytes()), "1.00000 0.50196 0.00000 rg")
}
