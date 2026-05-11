package pdf_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestPageAnnotationSessionMutation(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "024-annotations", "annotated_pdf.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)

	base, err := page.Annotations()
	require.NoError(t, err)
	baseCount := len(base)

	err = doc.AddPageAnnotation(0, pdf.AnnotationSpec{
		Type:       "Text",
		Rect:       [4]float64{10, 10, 30, 30},
		Contents:   "session-added",
		PgPoints:   []float64{10, 10, 30, 30},
		HeadPoints: []float64{11, 11, 29, 29},
		PathList:   [][]float64{{10, 10, 30, 30}},
		UserData: map[string]string{
			"meta": "added",
		},
	})
	require.NoError(t, err)

	afterAdd, err := page.Annotations()
	require.NoError(t, err)
	assert.Equal(t, baseCount+1, len(afterAdd))
	assert.Equal(t, "session-added", afterAdd[len(afterAdd)-1].Contents())
	assert.Equal(t, []float64{10, 10, 30, 30}, afterAdd[len(afterAdd)-1].PgPoints())
	assert.Equal(t, []float64{11, 11, 29, 29}, afterAdd[len(afterAdd)-1].HeadPoints())
	assert.Equal(t, [][]float64{{10, 10, 30, 30}}, afterAdd[len(afterAdd)-1].PathList())
	value, ok := afterAdd[len(afterAdd)-1].UserData("meta")
	require.True(t, ok)
	assert.Equal(t, "added", value)

	err = doc.RemovePageAnnotation(0, len(afterAdd)-1)
	require.NoError(t, err)

	afterRemove, err := page.Annotations()
	require.NoError(t, err)
	assert.Equal(t, baseCount, len(afterRemove))

	err = doc.ReplacePageAnnotations(0, []pdf.AnnotationSpec{
		{
			Type:       "Highlight",
			Rect:       [4]float64{1, 1, 5, 5},
			Contents:   "replaced",
			PgPoints:   []float64{1, 1, 5, 5},
			HeadPoints: []float64{1, 5, 5, 1},
			PathList:   [][]float64{{1, 1, 5, 5}},
			UserData: map[string]string{
				"meta": "replaced",
			},
		},
	})
	require.NoError(t, err)

	overridden, err := page.Annotations()
	require.NoError(t, err)
	require.Len(t, overridden, 1)
	assert.Equal(t, "Highlight", overridden[0].Type())
	assert.Equal(t, "replaced", overridden[0].Contents())
	assert.Equal(t, []float64{1, 1, 5, 5}, overridden[0].PgPoints())
	assert.Equal(t, []float64{1, 5, 5, 1}, overridden[0].HeadPoints())
	assert.Equal(t, [][]float64{{1, 1, 5, 5}}, overridden[0].PathList())
	value, ok = overridden[0].UserData("meta")
	require.True(t, ok)
	assert.Equal(t, "replaced", value)

	err = doc.SetPageAnnotationPgPoints(0, 0, []float64{2, 2, 6, 6})
	require.NoError(t, err)
	err = doc.SetPageAnnotationType(0, 0, "Square")
	require.NoError(t, err)
	err = doc.SetPageAnnotationRect(0, 0, [4]float64{2, 2, 7, 7})
	require.NoError(t, err)
	err = doc.SetPageAnnotationContents(0, 0, "updated")
	require.NoError(t, err)
	err = doc.SetPageAnnotationHeadPoints(0, 0, []float64{6, 6, 2, 2})
	require.NoError(t, err)
	err = doc.SetPageAnnotationPathList(0, 0, [][]float64{{2, 2, 6, 6}})
	require.NoError(t, err)
	err = doc.SetPageAnnotationUserData(0, 0, "flag", "on")
	require.NoError(t, err)
	err = doc.DeletePageAnnotationUserData(0, 0, "meta")
	require.NoError(t, err)

	updated, err := page.Annotations()
	require.NoError(t, err)
	require.Len(t, updated, 1)
	assert.Equal(t, "Square", updated[0].Type())
	assert.Equal(t, [4]float64{2, 2, 7, 7}, updated[0].Rect())
	assert.Equal(t, "updated", updated[0].Contents())
	assert.Equal(t, []float64{2, 2, 6, 6}, updated[0].PgPoints())
	assert.Equal(t, []float64{6, 6, 2, 2}, updated[0].HeadPoints())
	assert.Equal(t, [][]float64{{2, 2, 6, 6}}, updated[0].PathList())
	_, ok = updated[0].UserData("meta")
	assert.False(t, ok)
	value, ok = updated[0].UserData("flag")
	require.True(t, ok)
	assert.Equal(t, "on", value)

	err = doc.ClearPageAnnotationOverrides(0)
	require.NoError(t, err)

	restored, err := page.Annotations()
	require.NoError(t, err)
	assert.Equal(t, baseCount, len(restored))
}

func TestPageAnnotationSessionMutation_InvalidType(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "024-annotations", "annotated_pdf.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	err = doc.ReplacePageAnnotations(0, []pdf.AnnotationSpec{
		{
			Type: "Text",
			Rect: [4]float64{1, 1, 5, 5},
		},
	})
	require.NoError(t, err)

	err = doc.SetPageAnnotationType(0, 0, "   ")
	require.Error(t, err)
}
