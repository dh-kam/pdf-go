package pdf_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestSaveWithNativeFormUpdates_Reopen(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "010-pdflatex-forms", "pdflatex-forms.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	require.NoError(t, doc.SetFormFieldValue("Name", "NativeUpdated"))

	output := filepath.Join(t.TempDir(), "native-form-updated.pdf")
	require.NoError(t, doc.SaveWithNativeFormUpdates(output))

	reopened, err := pdf.Open(output)
	require.NoError(t, err)
	defer reopened.Close()

	xfdf, err := reopened.ExportFormDataXFDF()
	require.NoError(t, err)
	formData, err := pdf.ParseFormDataXFDF(xfdf)
	require.NoError(t, err)
	assert.Equal(t, []string{"NativeUpdated"}, formData.Fields["Name"])
	assert.Equal(t, []string{"Off"}, formData.Fields["Check"])
}

func TestSaveWithNativeFormUpdates_ChoiceOptionsReopen(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "012-libreoffice-form", "libreoffice-form.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	expected := []string{"Unknown", "Korean", "US-American"}
	require.NoError(t, doc.SetChoiceFieldItems("Nationality", expected))

	output := filepath.Join(t.TempDir(), "native-form-choice-options.pdf")
	require.NoError(t, doc.SaveWithNativeFormUpdates(output))

	reopened, err := pdf.Open(output)
	require.NoError(t, err)
	defer reopened.Close()

	field, ok := findFieldByName(t, reopened, "Nationality")
	require.True(t, ok)
	assert.Equal(t, expected, field.Options)
}

func findFieldByName(t *testing.T, doc *pdf.Document, name string) (*pdf.FormField, bool) {
	t.Helper()

	fields, err := doc.FormFields()
	require.NoError(t, err)
	for _, field := range fields {
		if field != nil && field.Name == name {
			return field, true
		}
	}
	return nil, false
}
