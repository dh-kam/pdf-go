package main

import (
	"context"
	"fmt"
	"image"
	_ "image/png"
	"os"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Println("Usage: compare_region <pdf_file> <page_number> <poppler_png>")
		return
	}

	pdfPath := os.Args[1]
	pageNum := 0
	fmt.Sscanf(os.Args[2], "%d", &pageNum)
	popplerPath := os.Args[3]

	// Render our version
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

	r := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 150

	ctx := context.Background()
	ourImg, err := r.RenderPage(ctx, page, opts)
	if err != nil {
		fmt.Printf("Error rendering: %v\n", err)
		return
	}

	// Load Poppler version
	popplerFile, err := os.Open(popplerPath)
	if err != nil {
		fmt.Printf("Error opening Poppler PNG: %v\n", err)
		return
	}
	defer popplerFile.Close()

	popplerImg, _, err := image.Decode(popplerFile)
	if err != nil {
		fmt.Printf("Error decoding Poppler PNG: %v\n", err)
		return
	}

	ourBounds := ourImg.Bounds()

	// Check center region specifically
	centerX := ourBounds.Dx() / 2
	centerY := ourBounds.Dy() / 2

	fmt.Printf("=== Center Region Comparison (around %d, %d) ===\n", centerX, centerY)
	fmt.Printf("Showing pixels where difference > 50:\n")

	count := 0
	maxDiffPixels := 50
	for dy := -100; dy <= 100; dy += 2 {
		for dx := -100; dx <= 100; dx += 2 {
			x := centerX + dx
			y := centerY + dy

			if x < 0 || x >= ourBounds.Dx() || y < 0 || y >= ourBounds.Dy() {
				continue
			}

			ourColor := ourImg.At(x, y)
			popColor := popplerImg.At(x, y)

			ourR, ourG, ourB, _ := ourColor.RGBA()
			popR, popG, popB, _ := popColor.RGBA()

			diffR := abs(int(ourR>>8) - int(popR>>8))
			diffG := abs(int(ourG>>8) - int(popG>>8))
			diffB := abs(int(ourB>>8) - int(popB>>8))

			totalDiff := diffR + diffG + diffB
			if totalDiff > 50 {
				fmt.Printf("(%4d,%4d): Our[%3d,%3d,%3d] Poppler[%3d,%3d,%3d] Diff[%d,%d,%d] Total=%d\n",
					x, y, ourR>>8, ourG>>8, ourB>>8, popR>>8, popG>>8, popB>>8, diffR, diffG, diffB, totalDiff)
				count++
				if count >= maxDiffPixels {
					fmt.Printf("... (showing first %d differing pixels)\n", maxDiffPixels)
					return
				}
			}
		}
	}

	if count == 0 {
		fmt.Printf("No significant differences found in center region!\n")
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
