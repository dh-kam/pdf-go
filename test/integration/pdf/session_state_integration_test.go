package pdf_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestSessionState_ExportImport(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "010-pdflatex-forms", "pdflatex-forms.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)
	baseAnnots, err := page.Annotations()
	require.NoError(t, err)
	addedIndex := len(baseAnnots)

	require.NoError(t, doc.SetFormFieldValue("Name", "Persisted"))
	require.NoError(t, doc.AddOutline(nil, &pdf.Outline{Title: "SessionRoot", PageIndex: 0}))
	require.NoError(t, doc.AddPageAnnotation(0, pdf.AnnotationSpec{
		Type:       "Text",
		Rect:       [4]float64{10, 10, 20, 20},
		Contents:   "persist-annot",
		PgPoints:   []float64{10, 10, 20, 20},
		HeadPoints: []float64{11, 11, 19, 19},
		PathList:   [][]float64{{10, 10, 20, 20}},
		UserData: map[string]string{
			"meta": "persisted",
		},
	}))
	require.NoError(t, doc.SetPageAnnotationType(0, addedIndex, "Stamp"))
	require.NoError(t, doc.SetPageAnnotationRect(0, addedIndex, [4]float64{8, 8, 22, 22}))
	require.NoError(t, doc.SetPageAnnotationContents(0, addedIndex, "persist-annot-updated"))
	require.NoError(t, doc.SetVisibleSignatureField(pdf.SignatureFieldSpec{
		FieldName: "SigState",
		PageIndex: 0,
		Rect:      [4]float64{30, 30, 90, 60},
		Name:      "StateUser",
	}))

	_, err = doc.ExecuteOutlineAction(&pdf.OutlineAction{
		Type:        "Hide",
		Hide:        true,
		HideTargets: []string{"HideTargetA"},
	}, pdf.ActionExecutionOptions{})
	require.NoError(t, err)

	state, err := doc.ExportSessionState()
	require.NoError(t, err)
	require.NotEmpty(t, state)

	doc.ClearFormDataOverrides()
	require.NoError(t, doc.ClearPageAnnotationOverrides(0))
	require.NoError(t, doc.RemoveOutline([]int{0}))
	_, err = doc.ExecuteOutlineAction(&pdf.OutlineAction{
		Type:        "Hide",
		Hide:        false,
		HideTargets: []string{"HideTargetA"},
	}, pdf.ActionExecutionOptions{})
	require.NoError(t, err)

	require.NoError(t, doc.ImportSessionState(state))

	xfdf, err := doc.ExportFormDataXFDF()
	require.NoError(t, err)
	formData, err := pdf.ParseFormDataXFDF(xfdf)
	require.NoError(t, err)
	assert.Equal(t, []string{"Persisted"}, formData.Fields["Name"])

	outlines, err := doc.Outlines()
	require.NoError(t, err)
	require.Len(t, outlines, 1)
	assert.Equal(t, "SessionRoot", outlines[0].Title)

	assert.True(t, doc.IsActionTargetHidden("HideTargetA"))

	page, err = doc.Page(0)
	require.NoError(t, err)
	annots, err := page.Annotations()
	require.NoError(t, err)
	require.NotEmpty(t, annots)
	assert.Equal(t, "persist-annot-updated", annots[len(annots)-1].Contents())
	assert.Equal(t, "Stamp", annots[len(annots)-1].Type())
	assert.Equal(t, [4]float64{8, 8, 22, 22}, annots[len(annots)-1].Rect())
	assert.Equal(t, []float64{10, 10, 20, 20}, annots[len(annots)-1].PgPoints())
	assert.Equal(t, []float64{11, 11, 19, 19}, annots[len(annots)-1].HeadPoints())
	assert.Equal(t, [][]float64{{10, 10, 20, 20}}, annots[len(annots)-1].PathList())
	value, ok := annots[len(annots)-1].UserData("meta")
	require.True(t, ok)
	assert.Equal(t, "persisted", value)

	signatures := doc.VisibleSignatureFields()
	require.Len(t, signatures, 1)
	assert.Equal(t, "SigState", signatures[0].FieldName)
}
