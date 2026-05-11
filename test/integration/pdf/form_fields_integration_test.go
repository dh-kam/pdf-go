package pdf_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestFormFields_WithFormSamples(t *testing.T) {
	samples := []string{
		filepath.Join(getSampleDir(), "010-pdflatex-forms", "pdflatex-forms.pdf"),
		filepath.Join(getSampleDir(), "012-libreoffice-form", "libreoffice-form.pdf"),
	}

	for _, sample := range samples {
		t.Run(filepath.Base(sample), func(t *testing.T) {
			doc, err := pdf.Open(sample)
			require.NoError(t, err)
			defer doc.Close()

			tree, err := doc.FormFieldTree()
			require.NoError(t, err)
			require.NotEmpty(t, tree)

			flat, err := doc.FormFields()
			require.NoError(t, err)
			require.NotEmpty(t, flat)

			hasNamedField := false
			hasTypedField := false
			for _, field := range flat {
				if field.Name != "" {
					hasNamedField = true
				}
				if field.Type != "" {
					hasTypedField = true
				}
			}

			require.True(t, hasNamedField, "expected at least one named form field")
			require.True(t, hasTypedField, "expected at least one typed form field")
		})
	}
}

func TestFormFields_WithoutFormDocument(t *testing.T) {
	sample := filepath.Join(getSampleDir(), "001-trivial", "minimal-document.pdf")

	doc, err := pdf.Open(sample)
	require.NoError(t, err)
	defer doc.Close()

	tree, err := doc.FormFieldTree()
	require.NoError(t, err)
	require.Len(t, tree, 0)

	flat, err := doc.FormFields()
	require.NoError(t, err)
	require.Len(t, flat, 0)
}
