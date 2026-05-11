package pdf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlattenAnnotation_RemovesOne(t *testing.T) {
	doc := newDocumentWithAnnotationPages([]annotationPageSpec{
		{
			annotations: []annotationSpec{
				{typ: "Text", contents: "a", rect: [4]float64{0, 0, 10, 10}},
				{typ: "Widget", contents: "f", rect: [4]float64{1, 1, 11, 11}},
			},
		},
	})

	require.NoError(t, doc.FlattenAnnotation(0, 1))

	page, err := doc.Page(0)
	require.NoError(t, err)
	annots, err := page.Annotations()
	require.NoError(t, err)
	require.Len(t, annots, 1)
	assert.Equal(t, "Text", annots[0].Type())
}

func TestFlattenAllFormField_RemovesWidgetAnnotations(t *testing.T) {
	doc := newDocumentWithAnnotationPages([]annotationPageSpec{
		{
			annotations: []annotationSpec{
				{typ: "Widget", contents: "field", rect: [4]float64{0, 0, 10, 10}},
				{typ: "Text", contents: "note", rect: [4]float64{1, 1, 11, 11}},
			},
		},
	})

	require.NoError(t, doc.FlattenAllFormField(true))

	page, err := doc.Page(0)
	require.NoError(t, err)
	annots, err := page.Annotations()
	require.NoError(t, err)
	require.Len(t, annots, 1)
	assert.Equal(t, "Text", annots[0].Type())
}

func TestFlattenAnnotation_InvalidIndex(t *testing.T) {
	doc := newDocumentWithAnnotationPages([]annotationPageSpec{
		{
			annotations: []annotationSpec{
				{typ: "Text", contents: "a", rect: [4]float64{0, 0, 10, 10}},
			},
		},
	})

	err := doc.FlattenAnnotation(0, 3)
	require.Error(t, err)
}
