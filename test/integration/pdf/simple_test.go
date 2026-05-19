// Package pdf_test provides integration tests for PDF loading.
package pdf_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

// TestOpenPDF tests opening a simple PDF file.
func TestOpenPDF(t *testing.T) {
	// Use test PDF in project root
	doc, err := pdf.Open(rootTestPDFPath(t))
	require.NoError(t, err, "Failed to open PDF")
	defer doc.Close()

	t.Logf("Successfully opened PDF")

	// Get page count
	pageCount, err := doc.PageCount()
	if err != nil {
		t.Logf("WARNING: Failed to get page count: %v", err)
	} else {
		t.Logf("Page count: %d", pageCount)
	}

	// Try to get first page
	page, err := doc.Page(0)
	if err != nil {
		t.Logf("WARNING: Failed to get page: %v", err)
	} else {
		t.Logf("Page 0: %.0f x %.0f points", page.Width(), page.Height())
	}
}
