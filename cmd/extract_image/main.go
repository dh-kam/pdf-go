package main

import (
	"context"
	"fmt"
	"image/png"
	"os"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: extract_image <pdf_file> [page_number]")
		return
	}

	pdfPath := os.Args[1]
	pageNum := 3
	if len(os.Args) > 2 {
		fmt.Sscanf(os.Args[2], "%d", &pageNum)
	}

	doc, err := pdf.Open(pdfPath)
	if err != nil {
		fmt.Printf("Error opening PDF: %v\n", err)
		return
	}
	defer doc.Close()

	page, err := doc.Page(pageNum)
	if err != nil {
		fmt.Printf("Error getting page: %v\n", err)
		return
	}

	// Get the raw image from the PDF
	resources, _ := page.Resources()
	if resources == nil {
		fmt.Printf("No resources\n")
		return
	}

	fmt.Printf("Resources: %+v\n", resources)

	// Try to get the image
	xObjects := resources.Get("XObject")
	if xObjects == nil {
		fmt.Printf("No XObjects\n")
		return
	}

	fmt.Printf("XObjects type: %T\n", xObjects)

	// Render the page to see what we get
	r := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 150

	img, err := r.RenderPage(context.Background(), page, opts)
	if err != nil {
		fmt.Printf("Error rendering: %v\n", err)
		return
	}

	bounds := img.Bounds()
	fmt.Printf("Rendered size: %dx%d\n", bounds.Dx(), bounds.Dy())

	// Save the rendered image
	outFile, _ := os.Create("extracted_raw.png")
	png.Encode(outFile, img)
	outFile.Close()

	fmt.Printf("Saved to extracted_raw.png\n")

	// Print pixel values
	fmt.Printf("\nPixel grid:\n")
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			c := img.At(x, y)
			r, _, _, _ := c.RGBA()
			fmt.Printf("%3d ", r>>8)
		}
		fmt.Printf("\n")
	}
}
