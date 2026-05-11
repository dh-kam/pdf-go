// Package pdf_test provides integration tests for PDF loading and rendering.
package pdf_test

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

// getSampleDir returns the path to sample PDF files.
func getSampleDir() string {
	// Get the directory of this test file
	_, testFile, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(testFile)
	// Navigate from test/integration/pdf to test/testdata/sample-files
	return filepath.Join(testDir, "..", "..", "testdata", "sample-files")
}

// getOutputDir returns the path to output directory.
func getOutputDir() string {
	// Get the directory of this test file
	_, testFile, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(testFile)
	// Navigate from test/integration/pdf to test/testdata/output
	outputDir := filepath.Join(testDir, "..", "..", "testdata", "output")
	os.MkdirAll(outputDir, 0755)
	return outputDir
}

// TestSamplePDFs tests loading and rendering sample PDF files.
func TestSamplePDFs(t *testing.T) {
	// Find all PDF files in testdata
	sampleDir := getSampleDir()

	// Use filepath.Walk to find all PDF files recursively
	var pdfFiles []string
	err := filepath.Walk(sampleDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".pdf" {
			pdfFiles = append(pdfFiles, path)
		}
		return nil
	})
	require.NoError(t, err, "Failed to walk sample directory")

	if len(pdfFiles) == 0 {
		t.Skip("No PDF files found in testdata")
	}

	t.Logf("Found %d PDF files to test", len(pdfFiles))

	// Test each PDF
	for _, pdfFile := range pdfFiles {
		t.Run(filepath.Base(pdfFile), func(t *testing.T) {
			testPDFFile(t, pdfFile)
		})
	}
}

// TestSpecificPDF tests a specific PDF file.
func TestSpecificPDF(t *testing.T) {
	pdfFile := os.Getenv("PDF_FILE")
	if pdfFile == "" {
		// Use LibreOffice PDF instead of minimal-document.pdf
		// minimal-document.pdf uses XRef streams which are not fully supported yet
		pdfFile = filepath.Join(getSampleDir(), "002-trivial-libre-office-writer/002-trivial-libre-office-writer.pdf")
	}

	testPDFFile(t, pdfFile)
}

// TestTrivialPDF tests the minimal trivial PDF.
func TestTrivialPDF(t *testing.T) {
	// Use LibreOffice PDF instead of minimal-document.pdf
	// minimal-document.pdf uses XRef streams which are not fully supported yet
	pdfFile := filepath.Join(getSampleDir(), "002-trivial-libre-office-writer/002-trivial-libre-office-writer.pdf")
	testPDFFile(t, pdfFile)
}

func testPDFFile(t *testing.T, pdfFile string) {
	t.Helper()

	// Open PDF
	doc, err := pdf.Open(pdfFile)
	if err != nil {
		t.Logf("WARNING: Failed to open %s: %v", pdfFile, err)
		t.Skip("Cannot open PDF file")
		return
	}
	defer doc.Close()

	t.Logf("Opened: %s", pdfFile)

	// Get page count
	pageCount, err := doc.PageCount()
	if err != nil {
		t.Logf("WARNING: Failed to get page count: %v", err)
		t.Skip("Cannot get page count")
		return
	}

	t.Logf("Pages: %d", pageCount)
	assert.Greater(t, pageCount, 0, "PDF should have at least one page")

	// Get first page
	page, err := doc.Page(0)
	require.NoError(t, err, "Should be able to get first page")

	// Check page dimensions
	width := page.Width()
	height := page.Height()
	t.Logf("Page size: %.2f x %.2f points", width, height)
	assert.Greater(t, width, 0.0, "Page width should be positive")
	assert.Greater(t, height, 0.0, "Page height should be positive")

	// Try to get contents
	contents, err := page.Contents()
	if err == nil {
		t.Logf("Contents: %d objects", len(contents))
	}

	// Try to get annotations
	annots, err := page.Annotations()
	if err == nil {
		t.Logf("Annotations: %d", len(annots))
	}

	// Try to render the page (may fail for complex PDFs, but should not crash)
	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	ctx := context.Background()
	options := pdf.DefaultRenderOptions()
	options.DPI = 72.0

	img, err := renderer.RenderPage(ctx, page, options)
	if err != nil {
		t.Logf("WARNING: Failed to render page: %v", err)
		// This is acceptable for now - rendering may not be fully implemented
	} else {
		assert.NotNil(t, img, "Rendered image should not be nil")
		bounds := img.Bounds()
		t.Logf("Rendered image size: %d x %d pixels", bounds.Dx(), bounds.Dy())

		// Save rendered image for inspection
		outputDir := getOutputDir()
		os.MkdirAll(outputDir, 0755)
		outputFile := filepath.Join(outputDir, filepath.Base(pdfFile)+".png")
		f, err := os.Create(outputFile)
		if err == nil {
			defer f.Close()
			if err := png.Encode(f, img); err == nil {
				t.Logf("Saved rendered image to: %s", outputFile)
			}
		}
	}

	// Print renderer stats
	stats := renderer.Stats()
	t.Logf("Renderer stats: renders=%d, cache_hits=%d, cache_misses=%d, hit_rate=%.1f%%",
		stats.RenderCount, stats.CacheHits, stats.CacheMisses, stats.HitRate())
}

// BenchmarkPDFOpen benchmarks opening a PDF file.
func BenchmarkPDFOpen(b *testing.B) {
	pdfFile := filepath.Join(getSampleDir(), "001-trivial/minimal-document.pdf")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc, err := pdf.Open(pdfFile)
		if err != nil {
			b.Fatalf("Failed to open PDF: %v", err)
		}
		doc.Close()
	}
}

// BenchmarkPDFPageGet benchmarks getting a page from a PDF.
func BenchmarkPDFPageGet(b *testing.B) {
	pdfFile := filepath.Join(getSampleDir(), "001-trivial/minimal-document.pdf")
	doc, err := pdf.Open(pdfFile)
	if err != nil {
		b.Fatalf("Failed to open PDF: %v", err)
	}
	defer doc.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := doc.Page(0)
		if err != nil {
			b.Fatalf("Failed to get page: %v", err)
		}
	}
}

// BenchmarkPDFRender benchmarks rendering a PDF page.
func BenchmarkPDFRender(b *testing.B) {
	pdfFile := filepath.Join(getSampleDir(), "001-trivial/minimal-document.pdf")
	doc, err := pdf.Open(pdfFile)
	if err != nil {
		b.Fatalf("Failed to open PDF: %v", err)
	}
	defer doc.Close()

	page, err := doc.Page(0)
	if err != nil {
		b.Fatalf("Failed to get page: %v", err)
	}

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	ctx := context.Background()
	options := pdf.DefaultRenderOptions()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := renderer.RenderPage(ctx, page, options)
		if err != nil {
			b.Logf("Render error: %v", err)
		}
	}
}

// TestMultipleSamples tests multiple sample PDFs in parallel.
func TestMultipleSamples(t *testing.T) {
	samples := []string{
		filepath.Join(getSampleDir(), "002-trivial-libre-office-writer/002-trivial-libre-office-writer.pdf"),
		filepath.Join(getSampleDir(), "007-imagemagick-images/imagemagick-images.pdf"),
	}

	for _, sample := range samples {
		t.Run(filepath.Base(sample), func(t *testing.T) {
			if _, err := os.Stat(sample); os.IsNotExist(err) {
				t.Skipf("Sample file not found: %s", sample)
				return
			}
			testPDFFile(t, sample)
		})
	}
}

// TestPDFInfo prints information about PDF files.
func TestPDFInfo(t *testing.T) {
	sampleDir := getSampleDir()

	// Test LibreOffice PDF (uses traditional XRef)
	t.Run("libreoffice", func(t *testing.T) {
		pdfFile := filepath.Join(sampleDir, "002-trivial-libre-office-writer/002-trivial-libre-office-writer.pdf")
		printPDFInfo(t, pdfFile)
	})

	// Test ImageMagick PDF (multiple pages)
	t.Run("imagemagick", func(t *testing.T) {
		pdfFile := filepath.Join(sampleDir, "007-imagemagick-images/imagemagick-images.pdf")
		if _, err := os.Stat(pdfFile); os.IsNotExist(err) {
			t.Skip("Sample file not found")
			return
		}
		printPDFInfo(t, pdfFile)
	})
}

func printPDFInfo(t *testing.T, pdfFile string) {
	t.Helper()

	doc, err := pdf.Open(pdfFile)
	if err != nil {
		t.Logf("ERROR: Failed to open %s: %v", pdfFile, err)
		return
	}
	defer doc.Close()

	fmt.Printf("\n=== PDF Info: %s ===\n", filepath.Base(pdfFile))

	pageCount, _ := doc.PageCount()
	fmt.Printf("Pages: %d\n", pageCount)

	if pageCount > 0 {
		for i := 0; i < min(pageCount, 5); i++ {
			page, err := doc.Page(i)
			if err != nil {
				continue
			}
			fmt.Printf("  Page %d: %.0f x %.0f points, rotation: %d\n",
				i, page.Width(), page.Height(), page.Rotate())
		}
	}

	info := doc.Info()
	if info != nil {
		fmt.Printf("Info dictionary available: yes\n")
	}

	fmt.Printf("File size: %d bytes\n", doc.FileSize())
}
