package entity_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

func TestDocument_NewDocument(t *testing.T) {
	xref := &mockXRef{}
	doc := entity.NewDocument(xref)

	assert.NotNil(t, doc)
	assert.Equal(t, xref, doc.XRef())
	assert.Nil(t, doc.Catalog())
	assert.Nil(t, doc.Info())
	assert.Equal(t, int64(0), doc.FileSize())
}

func TestDocument_SetCatalog(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	catalog := entity.NewDict()

	doc.SetCatalog(catalog)

	assert.Equal(t, catalog, doc.Catalog())
}

func TestDocument_SetInfo(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	info := entity.NewDict()

	doc.SetInfo(info)

	assert.Equal(t, info, doc.Info())
}

func TestDocument_SetMetadata(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	metadata := entity.NewDict()

	doc.SetMetadata(metadata)

	assert.Equal(t, metadata, doc.Metadata())
}

func TestDocument_SetFileSize(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})

	doc.SetFileSize(12345)

	assert.Equal(t, int64(12345), doc.FileSize())
}

func TestDocument_Close(t *testing.T) {
	xref := &mockXRef{}
	doc := entity.NewDocument(xref)

	err := doc.Close()
	assert.NoError(t, err)
}

func TestPage_NewPage(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	dict := entity.NewDict()
	ref := entity.NewRef(1, 0)

	page := entity.NewPage(doc, dict, ref, 0)

	assert.NotNil(t, page)
	assert.Equal(t, doc, page.Document())
	assert.Equal(t, dict, page.Dict())
	assert.Equal(t, ref, page.Ref())
	assert.Equal(t, 0, page.Index())
}

func TestPage_MediaBox_Default(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	dict := entity.NewDict()
	page := entity.NewPage(doc, dict, entity.NewRef(1, 0), 0)

	box := page.MediaBox()

	// Default should be US Letter size
	assert.Equal(t, [4]float64{0, 0, 612, 792}, box)
}

func TestPage_MediaBox_Custom(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	dict := entity.NewDict()
	dict.Set(entity.Name("MediaBox"), entity.NewArray(
		entity.NewInteger(0),
		entity.NewInteger(0),
		entity.NewInteger(100),
		entity.NewInteger(200),
	))
	page := entity.NewPage(doc, dict, entity.NewRef(1, 0), 0)

	box := page.MediaBox()

	assert.Equal(t, [4]float64{0, 0, 100, 200}, box)
}

func TestPage_CropBox_DefaultToMediaBox(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	dict := entity.NewDict()
	dict.Set(entity.Name("MediaBox"), entity.NewArray(
		entity.NewInteger(0),
		entity.NewInteger(0),
		entity.NewInteger(100),
		entity.NewInteger(200),
	))
	page := entity.NewPage(doc, dict, entity.NewRef(1, 0), 0)

	cropBox := page.CropBox()

	// Should default to MediaBox
	assert.Equal(t, [4]float64{0, 0, 100, 200}, cropBox)
}

func TestPage_CropBox_Custom(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	dict := entity.NewDict()
	dict.Set(entity.Name("MediaBox"), entity.NewArray(
		entity.NewInteger(0),
		entity.NewInteger(0),
		entity.NewInteger(100),
		entity.NewInteger(200),
	))
	dict.Set(entity.Name("CropBox"), entity.NewArray(
		entity.NewInteger(10),
		entity.NewInteger(10),
		entity.NewInteger(90),
		entity.NewInteger(190),
	))
	page := entity.NewPage(doc, dict, entity.NewRef(1, 0), 0)

	cropBox := page.CropBox()

	assert.Equal(t, [4]float64{10, 10, 90, 190}, cropBox)
}

func TestPage_Rotate_Default(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	dict := entity.NewDict()
	page := entity.NewPage(doc, dict, entity.NewRef(1, 0), 0)

	rotate := page.Rotate()

	assert.Equal(t, 0, rotate)
}

func TestPage_Rotate_Custom(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected int
	}{
		{"0 degrees", 0, 0},
		{"90 degrees", 90, 90},
		{"180 degrees", 180, 180},
		{"270 degrees", 270, 270},
		{"44 degrees - round to 0", 44, 0},
		{"45 degrees - round to 90", 45, 90},
		{"134 degrees - round to 90", 134, 90},
		{"135 degrees - round to 180", 135, 180},
		{"360 degrees", 360, 0},
		{"-90 degrees", -90, 270},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := entity.NewDocument(&mockXRef{})
			dict := entity.NewDict()
			dict.Set(entity.Name("Rotate"), entity.NewInteger(tt.input))
			page := entity.NewPage(doc, dict, entity.NewRef(1, 0), 0)

			rotate := page.Rotate()

			assert.Equal(t, tt.expected, rotate)
		})
	}
}

func TestPage_Width(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	dict := entity.NewDict()
	dict.Set(entity.Name("MediaBox"), entity.NewArray(
		entity.NewInteger(0),
		entity.NewInteger(0),
		entity.NewInteger(100),
		entity.NewInteger(200),
	))
	page := entity.NewPage(doc, dict, entity.NewRef(1, 0), 0)

	width := page.Width()

	assert.Equal(t, float64(100), width)
}

func TestPage_Height(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	dict := entity.NewDict()
	dict.Set(entity.Name("MediaBox"), entity.NewArray(
		entity.NewInteger(0),
		entity.NewInteger(0),
		entity.NewInteger(100),
		entity.NewInteger(200),
	))
	page := entity.NewPage(doc, dict, entity.NewRef(1, 0), 0)

	height := page.Height()

	assert.Equal(t, float64(200), height)
}

func TestPage_Resources_Empty(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	dict := entity.NewDict()
	page := entity.NewPage(doc, dict, entity.NewRef(1, 0), 0)

	resources, err := page.Resources()

	require.NoError(t, err)
	assert.NotNil(t, resources)
	assert.Equal(t, 0, resources.Len())
}

func TestPage_Resources_Inline(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	dict := entity.NewDict()
	resources := entity.NewDict()
	resources.Set(entity.Name("Font"), entity.NewDict())
	dict.Set(entity.Name("Resources"), resources)
	page := entity.NewPage(doc, dict, entity.NewRef(1, 0), 0)

	result, err := page.Resources()

	require.NoError(t, err)
	assert.Equal(t, resources, result)
}

func TestPage_Contents_Empty(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	dict := entity.NewDict()
	page := entity.NewPage(doc, dict, entity.NewRef(1, 0), 0)

	contents, err := page.Contents()

	require.NoError(t, err)
	assert.NotNil(t, contents)
	assert.Equal(t, 0, len(contents))
}

func TestPage_Contents_Single(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	dict := entity.NewDict()
	stream := entity.NewDict() // Simulated stream
	dict.Set(entity.Name("Contents"), stream)
	page := entity.NewPage(doc, dict, entity.NewRef(1, 0), 0)

	contents, err := page.Contents()

	require.NoError(t, err)
	assert.Equal(t, 1, len(contents))
}

func TestPage_Annotations_Empty(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	dict := entity.NewDict()
	page := entity.NewPage(doc, dict, entity.NewRef(1, 0), 0)

	annots, err := page.Annotations()

	require.NoError(t, err)
	assert.Equal(t, 0, len(annots))
}

func TestPage_Annotations_Single(t *testing.T) {
	doc := entity.NewDocument(&mockXRef{})
	dict := entity.NewDict()
	annotDict := entity.NewDict()
	annotDict.Set(entity.Name("Subtype"), entity.Name("Text"))
	annotDict.Set(entity.Name("Contents"), entity.NewString("Test annotation"))
	dict.Set(entity.Name("Annots"), entity.NewArray(annotDict))
	page := entity.NewPage(doc, dict, entity.NewRef(1, 0), 0)

	annots, err := page.Annotations()

	require.NoError(t, err)
	assert.Equal(t, 1, len(annots))

	assert.Equal(t, entity.Name("Text"), annots[0].Type())
	assert.Equal(t, "Test annotation", annots[0].Contents())
}

func TestAnnotation_Rect(t *testing.T) {
	dict := entity.NewDict()
	dict.Set(entity.Name("Rect"), entity.NewArray(
		entity.NewInteger(10),
		entity.NewInteger(20),
		entity.NewInteger(30),
		entity.NewInteger(40),
	))
	annot := entity.NewAnnotation(dict)

	rect := annot.Rect()

	assert.Equal(t, [4]float64{10, 20, 30, 40}, rect)
}

func TestAnnotation_Type(t *testing.T) {
	tests := []struct {
		name    string
		subtype entity.Name
	}{
		{"Text", entity.Name("Text")},
		{"Link", entity.Name("Link")},
		{"Highlight", entity.Name("Highlight")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dict := entity.NewDict()
			dict.Set(entity.Name("Subtype"), tt.subtype)
			annot := entity.NewAnnotation(dict)

			assert.Equal(t, tt.subtype, annot.Type())
		})
	}
}

// mockXRef is a mock implementation for testing.
type mockXRef struct{}

func (m *mockXRef) Fetch(ref entity.Ref) (entity.Object, error) {
	return entity.NewDict(), nil
}

func (m *mockXRef) FetchCached(ref entity.Ref) (entity.Object, bool) {
	return nil, false
}

func (m *mockXRef) Cache(ref entity.Ref, obj entity.Object) {}

func (m *mockXRef) GetCatalog() (*entity.Dict, error) {
	return nil, nil
}

func (m *mockXRef) GetTrailer() (*entity.Dict, error) {
	return nil, nil
}

func (m *mockXRef) GetNumObjects() int {
	return 0
}
