package main

import (
	"context"
	"fmt"
	_ "image/png"
	"os"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: analyze_pages <pdf_file>")
		return
	}

	pdfPath := os.Args[1]

	doc, err := pdf.Open(pdfPath)
	if err != nil {
		fmt.Printf("Error opening PDF: %v\n", err)
		return
	}
	defer doc.Close()

	pageCount, _ := doc.PageCount()
	fmt.Printf("PDF: %s, Pages: %d\n", pdfPath, pageCount)

	r := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 150

	ctx := context.Background()

	// Analyze each page
	for pageNum := 0; pageNum < pageCount; pageNum++ {
		page, err := doc.Page(pageNum)
		if err != nil {
			continue
		}

		fmt.Printf("\n=== Page %d ===\n", pageNum+1)
		fmt.Printf("MediaBox: %v\n", page.MediaBox())
		fmt.Printf("CropBox: %v\n", page.CropBox())

		img, err := r.RenderPage(ctx, page, opts)
		if err != nil {
			fmt.Printf("Error rendering: %v\n", err)
			continue
		}

		bounds := img.Bounds()
		fmt.Printf("Size: %dx%d\n", bounds.Dx(), bounds.Dy())

		// Count non-black pixels
		nonBlack := 0
		var maxBrightness uint32
		for y := 0; y < bounds.Dy(); y++ {
			for x := 0; x < bounds.Dx(); x++ {
				c := img.At(x, y)
				r, _, _, _ := c.RGBA()
				val := r >> 8
				if val > 0 {
					nonBlack++
				}
				if val > maxBrightness {
					maxBrightness = val
				}
			}
		}

		fmt.Printf("Non-black pixels: %d/%d\n", nonBlack, bounds.Dx()*bounds.Dy())
		fmt.Printf("Max brightness: %d\n", maxBrightness)

		// Show first row
		fmt.Printf("First row: ")
		for x := 0; x < bounds.Dx(); x++ {
			c := img.At(x, 0)
			r, _, _, _ := c.RGBA()
			fmt.Printf("%3d ", r>>8)
		}
		fmt.Printf("\n")
	}
}
