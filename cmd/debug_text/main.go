package main

import (
	"context"
	"fmt"
	"os"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: debug_text <pdf_file> [page_number]")
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

	page, err := doc.Page(pageNum)
	if err != nil {
		fmt.Printf("Error getting page: %v\n", err)
		return
	}

	fmt.Printf("=== Page %d ===\n", pageNum+1)

	// Render to check if text appears
	r := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 150

	ctx := context.Background()
	img, err := r.RenderPage(ctx, page, opts)
	if err != nil {
		fmt.Printf("Error rendering: %v\n", err)
		return
	}

	bounds := img.Bounds()
	fmt.Printf("Rendered size: %dx%d\n", bounds.Dx(), bounds.Dy())

	// Sample center region where text might appear
	centerX := bounds.Dx() / 2
	centerY := bounds.Dy() / 2

	fmt.Printf("\nCenter pixel samples:\n")
	for dy := -10; dy <= 10; dy += 2 {
		for dx := -10; dx <= 10; dx += 2 {
			x := centerX + dx
			y := centerY + dy
			if x >= 0 && x < bounds.Dx() && y >= 0 && y < bounds.Dy() {
				c := img.At(x, y)
				r, g, b, a := c.RGBA()
				if r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
					fmt.Printf("  (%d,%d): R=%d G=%d B=%d A=%d\n", x, y, r>>8, g>>8, b>>8, a>>8)
				}
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
