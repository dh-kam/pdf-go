package test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestMinimalPDFOpen(t *testing.T) {
	// Test with the actual PDF
	doc, err := pdf.Open("/workspace/pdf-reader/go-pdf/test.pdf")
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to open PDF: %v", err)
	}
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
