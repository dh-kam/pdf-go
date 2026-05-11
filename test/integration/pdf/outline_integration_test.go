package pdf_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestOutlines_WithOutlineDocument(t *testing.T) {
	outlinePDF := filepath.Join(getSampleDir(), "006-pdflatex-outline", "pdflatex-outline.pdf")

	doc, err := pdf.Open(outlinePDF)
	require.NoError(t, err)
	defer doc.Close()

	outlines, err := doc.Outlines()
	require.NoError(t, err)
	require.NotEmpty(t, outlines)

	flat := flattenOutlines(outlines)
	require.NotEmpty(t, flat)

	hasTitle := false
	hasDestinationInfo := false

	for _, item := range flat {
		if item.Title != "" {
			hasTitle = true
		}
		if item.Dest != nil || item.Action != nil {
			hasDestinationInfo = true
		}
	}

	require.True(t, hasTitle, "expected at least one outline title")
	require.True(t, hasDestinationInfo, "expected at least one outline destination/action")
}

func TestOutlines_WithoutOutlineDocument(t *testing.T) {
	minimalPDF := filepath.Join(getSampleDir(), "001-trivial", "minimal-document.pdf")

	doc, err := pdf.Open(minimalPDF)
	require.NoError(t, err)
	defer doc.Close()

	outlines, err := doc.Outlines()
	require.NoError(t, err)
	require.Len(t, outlines, 0)
}

func flattenOutlines(items []*pdf.Outline) []*pdf.Outline {
	flat := make([]*pdf.Outline, 0, len(items))
	for _, item := range items {
		flat = append(flat, item)
		if len(item.Children) > 0 {
			flat = append(flat, flattenOutlines(item.Children)...)
		}
	}
	return flat
}
