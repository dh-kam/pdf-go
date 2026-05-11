package pdf_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestSaveWithEmbeddedSession_Reopen(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "010-pdflatex-forms", "pdflatex-forms.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)
	baseAnnots, err := page.Annotations()
	require.NoError(t, err)
	addedIndex := len(baseAnnots)

	require.NoError(t, doc.SetFormFieldValue("Name", "EmbeddedState"))
	require.NoError(t, doc.AddOutline(nil, &pdf.Outline{Title: "EmbeddedOutline", PageIndex: 0}))
	require.NoError(t, doc.AddPageAnnotation(0, pdf.AnnotationSpec{
		Type:       "Text",
		Rect:       [4]float64{10, 10, 20, 20},
		Contents:   "embedded-annot",
		PgPoints:   []float64{10, 10, 20, 20},
		HeadPoints: []float64{12, 12, 18, 18},
		PathList:   [][]float64{{10, 10, 20, 20}},
		UserData: map[string]string{
			"meta": "embedded",
		},
	}))
	require.NoError(t, doc.SetPageAnnotationType(0, addedIndex, "Circle"))
	require.NoError(t, doc.SetPageAnnotationRect(0, addedIndex, [4]float64{9, 9, 21, 21}))
	require.NoError(t, doc.SetPageAnnotationContents(0, addedIndex, "embedded-annot-updated"))
	_, err = doc.ExecuteOutlineAction(&pdf.OutlineAction{
		Type:        "Hide",
		Hide:        true,
		HideTargets: []string{"EmbeddedTarget"},
	}, pdf.ActionExecutionOptions{})
	require.NoError(t, err)

	output := filepath.Join(t.TempDir(), "embedded-session.pdf")
	require.NoError(t, doc.SaveWithEmbeddedSession(output))

	reopened, err := pdf.Open(output)
	require.NoError(t, err)
	defer reopened.Close()

	xfdf, err := reopened.ExportFormDataXFDF()
	require.NoError(t, err)
	formData, err := pdf.ParseFormDataXFDF(xfdf)
	require.NoError(t, err)
	assert.Equal(t, []string{"EmbeddedState"}, formData.Fields["Name"])

	outlines, err := reopened.Outlines()
	require.NoError(t, err)
	require.Len(t, outlines, 1)
	assert.Equal(t, "EmbeddedOutline", outlines[0].Title)

	assert.True(t, reopened.IsActionTargetHidden("EmbeddedTarget"))

	page, err = reopened.Page(0)
	require.NoError(t, err)
	annots, err := page.Annotations()
	require.NoError(t, err)
	require.NotEmpty(t, annots)
	assert.Equal(t, "embedded-annot-updated", annots[len(annots)-1].Contents())
	assert.Equal(t, "Circle", annots[len(annots)-1].Type())
	assert.Equal(t, [4]float64{9, 9, 21, 21}, annots[len(annots)-1].Rect())
	assert.Equal(t, []float64{10, 10, 20, 20}, annots[len(annots)-1].PgPoints())
	assert.Equal(t, []float64{12, 12, 18, 18}, annots[len(annots)-1].HeadPoints())
	assert.Equal(t, [][]float64{{10, 10, 20, 20}}, annots[len(annots)-1].PathList())
	value, ok := annots[len(annots)-1].UserData("meta")
	require.True(t, ok)
	assert.Equal(t, "embedded", value)
}

func TestSaveWithEmbeddedSession_ReopenChoiceOptions(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "012-libreoffice-form", "libreoffice-form.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	expected := []string{"Unknown", "Korean", "US-American"}
	require.NoError(t, doc.SetChoiceFieldItems("Nationality", expected))

	output := filepath.Join(t.TempDir(), "embedded-session-choice-options.pdf")
	require.NoError(t, doc.SaveWithEmbeddedSession(output))

	reopened, err := pdf.Open(output)
	require.NoError(t, err)
	defer reopened.Close()

	field, ok := findFieldByName(t, reopened, "Nationality")
	require.True(t, ok)
	assert.Equal(t, expected, field.Options)
}
