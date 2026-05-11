package pdf_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestSaveWithNativeSessionUpdates_Reopen(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "010-pdflatex-forms", "pdflatex-forms.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	page, err := doc.Page(0)
	require.NoError(t, err)
	baseAnnots, err := page.Annotations()
	require.NoError(t, err)
	addedIndex := len(baseAnnots)

	require.NoError(t, doc.SetFormFieldValue("Name", "NativeSession"))
	require.NoError(t, doc.AddOutline(nil, &pdf.Outline{
		Title:     "NativeOutline",
		PageIndex: 0,
	}))
	require.NoError(t, doc.AddPageAnnotation(0, pdf.AnnotationSpec{
		Type:       "Text",
		Rect:       [4]float64{15, 15, 35, 35},
		Contents:   "native-session-annot",
		PgPoints:   []float64{15, 15, 35, 35},
		HeadPoints: []float64{16, 16, 34, 34},
		PathList:   [][]float64{{15, 15, 35, 35}},
		UserData: map[string]string{
			"og_url": "https://example.com",
			"type":   "ogtag",
		},
	}))
	require.NoError(t, doc.SetPageAnnotationType(0, addedIndex, "Square"))
	require.NoError(t, doc.SetPageAnnotationRect(0, addedIndex, [4]float64{14, 14, 36, 36}))
	require.NoError(t, doc.SetPageAnnotationContents(0, addedIndex, "native-session-annot-updated"))
	require.NoError(t, doc.SetVisibleSignatureField(pdf.SignatureFieldSpec{
		FieldName: "SigNative",
		PageIndex: 0,
		Rect:      [4]float64{40, 40, 140, 80},
		Name:      "Tester",
		Reason:    "Approval",
		Location:  "Seoul",
		Contents:  []byte("DETACHED"),
		ByteRange: []int64{0, 10, 20, 30},
	}))

	output := filepath.Join(t.TempDir(), "native-session-updated.pdf")
	require.NoError(t, doc.SaveWithNativeSessionUpdates(output))

	reopened, err := pdf.Open(output)
	require.NoError(t, err)
	defer reopened.Close()

	xfdf, err := reopened.ExportFormDataXFDF()
	require.NoError(t, err)
	formData, err := pdf.ParseFormDataXFDF(xfdf)
	require.NoError(t, err)
	assert.Equal(t, []string{"NativeSession"}, formData.Fields["Name"])

	outlines, err := reopened.Outlines()
	require.NoError(t, err)
	require.NotEmpty(t, outlines)
	assert.Equal(t, "NativeOutline", outlines[0].Title)

	page, err = reopened.Page(0)
	require.NoError(t, err)
	annots, err := page.Annotations()
	require.NoError(t, err)
	require.NotEmpty(t, annots)
	foundSessionAnnot := false
	for _, annot := range annots {
		if annot.Contents() == "native-session-annot-updated" {
			foundSessionAnnot = true
			assert.Equal(t, "Square", annot.Type())
			assert.Equal(t, [4]float64{14, 14, 36, 36}, annot.Rect())
			assert.Equal(t, []float64{15, 15, 35, 35}, annot.PgPoints())
			assert.Equal(t, []float64{16, 16, 34, 34}, annot.HeadPoints())
			assert.Equal(t, [][]float64{{15, 15, 35, 35}}, annot.PathList())
			value, ok := annot.UserData("og_url")
			require.True(t, ok)
			assert.Equal(t, "https://example.com", value)
			break
		}
	}
	assert.True(t, foundSessionAnnot)

	signatures, err := reopened.Signatures()
	require.NoError(t, err)
	require.NotEmpty(t, signatures)
	assert.Equal(t, "SigNative", signatures[len(signatures)-1].FieldName)

	verifications, err := reopened.VerifySignatures()
	require.NoError(t, err)
	require.NotEmpty(t, verifications)
	assert.True(t, verifications[len(verifications)-1].VerificationOK)

	digest, err := reopened.SignatureDigest("SigNative", "sha256")
	require.NoError(t, err)
	require.NotNil(t, digest)
	assert.Equal(t, "sha256", digest.HashAlgorithm)
	assert.NotEmpty(t, digest.Digest)
}

func TestSaveWithNativeSessionUpdates_ChoiceOptionsReopen(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "012-libreoffice-form", "libreoffice-form.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	expected := []string{"Unknown", "Korean", "US-American"}
	require.NoError(t, doc.SetChoiceFieldItems("Nationality", expected))

	output := filepath.Join(t.TempDir(), "native-session-choice-options.pdf")
	require.NoError(t, doc.SaveWithNativeSessionUpdates(output))

	reopened, err := pdf.Open(output)
	require.NoError(t, err)
	defer reopened.Close()

	field, ok := findFieldByName(t, reopened, "Nationality")
	require.True(t, ok)
	assert.Equal(t, expected, field.Options)
}
