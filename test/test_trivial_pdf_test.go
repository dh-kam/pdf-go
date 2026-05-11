package test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestTrivialPDF(t *testing.T) {
	doc, err := pdf.Open("/workspace/pdf-reader/go-pdf/test/testdata/sample-files/001-trivial/minimal-document.pdf")
	if err != nil {
		require.FailNowf(t, "test failed", "Failed to open PDF: %v", err)
	}
	defer doc.Close()

	t.Logf("Successfully opened PDF")

	pageCount, err := doc.PageCount()
	if err != nil {
		t.Logf("Page count: %v", err)
	} else {
		t.Logf("Page count: %d", pageCount)
	}

	if pageCount > 0 {
		page, err := doc.Page(0)
		if err != nil {
			t.Logf("Failed to get page: %v", err)
		} else {
			t.Logf("Page 0: %.0f x %.0f points", page.Width(), page.Height())
		}
	}
}
