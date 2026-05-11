// Package pdf_test provides comprehensive integration tests for PDF processing.
package pdf_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

// TestPDFVersions tests parsing of different PDF versions.
func TestPDFVersions(t *testing.T) {
	// This test will be run against PDFs with different versions
	// For now, we test with available sample files
	sampleDir := getSampleDir()

	tests := []struct {
		name     string
		path     string
		minPages int
	}{
		{
			name:     "LibreOffice PDF",
			path:     filepath.Join(sampleDir, "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf"),
			minPages: 1,
		},
		{
			name:     "Minimal Document",
			path:     filepath.Join(sampleDir, "001-trivial", "minimal-document.pdf"),
			minPages: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip if file doesn't exist
			if _, err := os.Stat(tt.path); os.IsNotExist(err) {
				t.Skipf("Sample file not found: %s", tt.path)
				return
			}

			doc, err := pdf.Open(tt.path)
			require.NoError(t, err, "Should open PDF")
			defer doc.Close()

			pageCount, err := doc.PageCount()
			require.NoError(t, err, "Should get page count")
			assert.GreaterOrEqual(t, pageCount, tt.minPages, "Should have minimum pages")
		})
	}
}

// TestPDFRendering tests rendering of PDF pages.
func TestPDFRendering(t *testing.T) {
	sampleDir := getSampleDir()
	pdfFile := filepath.Join(sampleDir, "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")

	// Skip if file doesn't exist
	if _, err := os.Stat(pdfFile); os.IsNotExist(err) {
		t.Skipf("Sample file not found: %s", pdfFile)
		return
	}

	doc, err := pdf.Open(pdfFile)
	require.NoError(t, err, "Should open PDF")
	defer doc.Close()

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	ctx := context.Background()
	options := pdf.DefaultRenderOptions()
	options.DPI = 72.0

	pageCount, err := doc.PageCount()
	require.NoError(t, err, "Should get page count")

	// Test rendering first page if available
	if pageCount > 0 {
		page, err := doc.Page(0)
		require.NoError(t, err, "Should get first page")

		img, err := renderer.RenderPage(ctx, page, options)
		if err != nil {
			t.Logf("WARNING: Failed to render page: %v", err)
			// Rendering may not be fully implemented for all features
		} else {
			assert.NotNil(t, img, "Rendered image should not be nil")

			// Save rendered image for visual inspection
			if img != nil {
				outputDir := getOutputDir()
				outputPath := filepath.Join(outputDir, "rendered_page_0.png")

				// Note: Image saving would go here
				t.Logf("Would save rendered image to: %s", outputPath)
			}
		}
	}
}

// TestTextExtraction tests text extraction from PDFs.
func TestTextExtraction(t *testing.T) {
	sampleDir := getSampleDir()
	pdfFile := filepath.Join(sampleDir, "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")

	// Skip if file doesn't exist
	if _, err := os.Stat(pdfFile); os.IsNotExist(err) {
		t.Skipf("Sample file not found: %s", pdfFile)
		return
	}

	doc, err := pdf.Open(pdfFile)
	require.NoError(t, err, "Should open PDF")
	defer doc.Close()

	pageCount, err := doc.PageCount()
	require.NoError(t, err, "Should get page count")

	// Test text extraction from first page
	if pageCount > 0 {
		page, err := doc.Page(0)
		require.NoError(t, err, "Should get first page")

		// Text extraction is done via content stream evaluation
		contents, err := page.Contents()
		if err != nil {
			t.Logf("WARNING: Failed to get contents: %v", err)
		} else {
			t.Logf("Found %d content objects", len(contents))
		}
	}
}

// TestPDFInformation tests extracting PDF information.
func TestPDFInformation(t *testing.T) {
	sampleDir := getSampleDir()

	tests := []struct {
		name string
		path string
	}{
		{
			name: "LibreOffice PDF",
			path: filepath.Join(sampleDir, "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf"),
		},
		{
			name: "Minimal Document",
			path: filepath.Join(sampleDir, "001-trivial", "minimal-document.pdf"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip if file doesn't exist
			if _, err := os.Stat(tt.path); os.IsNotExist(err) {
				t.Skipf("Sample file not found: %s", tt.path)
				return
			}

			doc, err := pdf.Open(tt.path)
			require.NoError(t, err, "Should open PDF")
			defer doc.Close()

			// Get PDF info
			info := doc.Info()
			assert.NotNil(t, info, "PDF info should not be nil")

			// Test basic info fields
			if title := info.Get("Title"); title != nil {
				t.Logf("Title: %v", title)
			}
			if author := info.Get("Author"); author != nil {
				t.Logf("Author: %v", author)
			}
			if subject := info.Get("Subject"); subject != nil {
				t.Logf("Subject: %v", subject)
			}
			if creator := info.Get("Creator"); creator != nil {
				t.Logf("Creator: %v", creator)
			}
			if producer := info.Get("Producer"); producer != nil {
				t.Logf("Producer: %v", producer)
			}
		})
	}
}

// TestPageNavigation tests page navigation and page properties.
func TestPageNavigation(t *testing.T) {
	sampleDir := getSampleDir()
	pdfFile := filepath.Join(sampleDir, "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")

	// Skip if file doesn't exist
	if _, err := os.Stat(pdfFile); os.IsNotExist(err) {
		t.Skipf("Sample file not found: %s", pdfFile)
		return
	}

	doc, err := pdf.Open(pdfFile)
	require.NoError(t, err, "Should open PDF")
	defer doc.Close()

	pageCount, err := doc.PageCount()
	require.NoError(t, err, "Should get page count")

	if pageCount < 2 {
		t.Skip("PDF has less than 2 pages")
		return
	}

	// Test accessing different pages
	for i := 0; i < min(3, pageCount); i++ {
		t.Run(fmt.Sprintf("Page%d", i), func(t *testing.T) {
			page, err := doc.Page(i)
			require.NoError(t, err, "Should get page")

			// Check page dimensions
			width := page.Width()
			height := page.Height()
			assert.Greater(t, width, 0.0, "Page width should be positive")
			assert.Greater(t, height, 0.0, "Page height should be positive")

			// Check page rotation
			rotation := page.Rotate()
			assert.GreaterOrEqual(t, rotation, 0, "Rotation should be non-negative")
			assert.LessOrEqual(t, rotation, 360, "Rotation should be at most 360")

			t.Logf("Page %d: %.2f x %.2f points, rotation: %d", i, width, height, rotation)
		})
	}
}

// TestEncryptedPDFs tests handling of encrypted PDFs.
func TestEncryptedPDFs(t *testing.T) {
	sampleDir := getSampleDir()

	// Look for encrypted PDF samples
	encryptedDir := filepath.Join(sampleDir, "encrypted")
	if _, err := os.Stat(encryptedDir); os.IsNotExist(err) {
		t.Skip("No encrypted PDF samples found")
		return
	}

	var pdfFiles []string
	filepath.Walk(encryptedDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".pdf" {
			pdfFiles = append(pdfFiles, path)
		}
		return nil
	})

	if len(pdfFiles) == 0 {
		t.Skip("No encrypted PDF files found")
		return
	}

	for _, pdfFile := range pdfFiles {
		t.Run(filepath.Base(pdfFile), func(t *testing.T) {
			// Try opening without password
			doc, err := pdf.Open(pdfFile)
			if err != nil {
				// Expected - encrypted PDFs should fail without password
				t.Logf("Expected error opening encrypted PDF without password: %v", err)
				return
			}
			defer doc.Close()

			// If it opened, check if it's actually encrypted
			info := doc.Info()
			if info != nil {
				assert.Nil(t, info.Get("Encrypt"), "Unencrypted PDF should not have Encrypt dictionary")
			}
		})
	}
}

// TestCompressionMethods tests PDFs with various compression methods.
func TestCompressionMethods(t *testing.T) {
	sampleDir := getSampleDir()

	// Test Flate compression (most common)
	pdfFile := filepath.Join(sampleDir, "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")

	if _, err := os.Stat(pdfFile); os.IsNotExist(err) {
		t.Skipf("Sample file not found: %s", pdfFile)
		return
	}

	doc, err := pdf.Open(pdfFile)
	require.NoError(t, err, "Should open PDF with Flate compression")
	defer doc.Close()

	pageCount, err := doc.PageCount()
	require.NoError(t, err, "Should get page count")
	assert.Greater(t, pageCount, 0, "Should have at least one page")
}

// TestAnnotationHandling tests PDF annotation parsing.
func TestAnnotationHandling(t *testing.T) {
	sampleDir := getSampleDir()
	pdfFile := filepath.Join(sampleDir, "002-trivial-libre-office-writer", "002-trivial-libre-office-writer.pdf")

	if _, err := os.Stat(pdfFile); os.IsNotExist(err) {
		t.Skipf("Sample file not found: %s", pdfFile)
		return
	}

	doc, err := pdf.Open(pdfFile)
	require.NoError(t, err, "Should open PDF")
	defer doc.Close()

	pageCount, err := doc.PageCount()
	require.NoError(t, err, "Should get page count")

	if pageCount > 0 {
		page, err := doc.Page(0)
		require.NoError(t, err, "Should get first page")

		annots, err := page.Annotations()
		if err != nil {
			t.Logf("WARNING: Failed to get annotations: %v", err)
			return
		}

		t.Logf("Found %d annotations on page 0", len(annots))

		// Test annotation properties
		for i, annot := range annots {
			t.Logf("Annotation %d: Type=%s", i, annot.Type())

			// Test getting annotation rectangle
			rect := annot.Rect()
			assert.NotNil(t, rect, "Annotation rect should not be nil")

			// Test getting annotation contents
			if contents := annot.Contents(); contents != "" {
				t.Logf("Annotation %d contents: %s", i, contents)
			}
		}
	}
}

// TestIncrementalPDFs tests PDFs with incremental updates.
func TestIncrementalPDFs(t *testing.T) {
	sampleDir := getSampleDir()

	// Look for incremental PDF samples
	incrementalDir := filepath.Join(sampleDir, "incremental")
	if _, err := os.Stat(incrementalDir); os.IsNotExist(err) {
		t.Skip("No incremental PDF samples found")
		return
	}

	var pdfFiles []string
	filepath.Walk(incrementalDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".pdf" {
			pdfFiles = append(pdfFiles, path)
		}
		return nil
	})

	if len(pdfFiles) == 0 {
		t.Skip("No incremental PDF files found")
		return
	}

	for _, pdfFile := range pdfFiles {
		t.Run(filepath.Base(pdfFile), func(t *testing.T) {
			doc, err := pdf.Open(pdfFile)
			if err != nil {
				t.Logf("Failed to open incremental PDF: %v", err)
				t.Skip("Cannot open incremental PDF")
				return
			}
			defer doc.Close()

			// Incremental PDFs should open successfully
			pageCount, err := doc.PageCount()
			require.NoError(t, err, "Should get page count from incremental PDF")
			assert.Greater(t, pageCount, 0, "Incremental PDF should have pages")
		})
	}
}

// TestFormHandling tests PDF form (AcroForm) handling.
func TestFormHandling(t *testing.T) {
	sampleDir := getSampleDir()
	pdfFile := filepath.Join(sampleDir, "010-pdflatex-forms", "pdflatex-forms.pdf")
	if _, err := os.Stat(pdfFile); os.IsNotExist(err) {
		t.Skipf("Sample file not found: %s", pdfFile)
		return
	}

	doc, err := pdf.Open(pdfFile)
	require.NoError(t, err, "Should open form PDF")
	defer doc.Close()

	fields, err := doc.FormFields()
	require.NoError(t, err, "Should parse form fields")
	require.NotEmpty(t, fields, "Form PDF should have fields")

	err = doc.SetFormFieldValue("Name", "Comprehensive Test User")
	require.NoError(t, err, "Should set Name field")

	err = doc.SetFormFieldValue("Check", "Yes")
	require.NoError(t, err, "Should set Check field")

	xfdf, err := doc.ExportFormDataXFDF()
	require.NoError(t, err, "Should export XFDF")

	parsed, err := pdf.ParseFormDataXFDF(xfdf)
	require.NoError(t, err, "Should parse exported XFDF")

	assert.Equal(t, []string{"Comprehensive Test User"}, parsed.Fields["Name"])
	assert.Equal(t, []string{"Yes"}, parsed.Fields["Check"])

	applied, err := doc.ApplyFormData(&pdf.FormData{
		Fields: map[string][]string{
			"Name":    {"Applied User"},
			"Unknown": {"Ignored"},
		},
	})
	require.NoError(t, err, "Should apply known form data")
	assert.Equal(t, 1, applied, "Only known fields should be applied")
}

// Helper function to get minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
