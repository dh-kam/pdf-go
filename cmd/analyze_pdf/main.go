package main

import (
	"context"
	"fmt"
	"os"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: analyze_pdf <pdf_file> [page_number]")
		return
	}

	pdfPath := os.Args[1]
	pageNum := 0
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &pageNum)
	}

	doc, err := pdf.Open(pdfPath)
	if err != nil {
		fmt.Printf("Error opening PDF: %v\n", err)
		return
	}
	defer doc.Close()

	pageCount, _ := doc.PageCount()
	fmt.Printf("PDF: %s, Pages: %d\n", pdfPath, pageCount)

	if pageNum >= pageCount {
		pageNum = 0
	}

	page, err := doc.Page(pageNum)
	if err != nil {
		fmt.Printf("Error getting page: %v\n", err)
		return
	}

	fmt.Printf("\n=== Page %d ===\n", pageNum+1)
	fmt.Printf("MediaBox: %v\n", page.MediaBox())
	fmt.Printf("CropBox: %v\n", page.CropBox())
	fmt.Printf("Rotate: %d\n", page.Rotate())

	contents, _ := page.Contents()
	fmt.Printf("Content streams: %d\n", len(contents))

	resources, _ := page.Resources()
	if resources != nil {
		fmt.Printf("Resources available\n")
	}

	// Render to check
	r := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 150  // Match test DPI

	ctx := context.Background()
	img, err := r.RenderPage(ctx, page, opts)
	if err != nil {
		fmt.Printf("Error rendering: %v\n", err)
		return
	}

	bounds := img.Bounds()
	fmt.Printf("Rendered size: %dx%d\n", bounds.Dx(), bounds.Dy())

	// Sample some pixels
	fmt.Printf("\nPixel samples:\n")
	samplePoints := [][2]int{
		{0, 0}, {bounds.Dx() / 2, bounds.Dy() / 2}, {bounds.Dx() - 1, bounds.Dy() - 1},
	}

	for _, pt := range samplePoints {
		c := img.At(pt[0], pt[1])
		r, g, b, a := c.RGBA()
		fmt.Printf("  (%d,%d): R=%d G=%d B=%d A=%d\n", pt[0], pt[1], r>>8, g>>8, b>>8, a>>8)
	}
}
