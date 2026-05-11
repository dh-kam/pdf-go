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
	if len(os.Args) < 3 {
		fmt.Println("Usage: compare_pixels <pdf_file> <page_number> <poppler_png>")
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

	// Compare pixels
	ourBounds := ourImg.Bounds()
	popplerBounds := popplerImg.Bounds()

	fmt.Printf("Our size: %v\n", ourBounds)
	fmt.Printf("Poppler size: %v\n", popplerBounds)

	// Use the smaller bounds for comparison
	minX := ourBounds.Min.X
	minY := ourBounds.Min.Y
	maxX := min(ourBounds.Max.X, popplerBounds.Max.X)
	maxY := min(ourBounds.Max.Y, popplerBounds.Max.Y)

	if ourBounds != popplerBounds {
		fmt.Printf("Warning: Size mismatch! Using common bounds.\n")
	}

	totalPixels := 0
	differentPixels := 0
	totalDiff := 0

	fmt.Println("\nPixel-by-pixel comparison:")
	fmt.Println("Format: (x,y) -> Our: RGBA | Poppler: RGBA | Diff")
	fmt.Println("Showing first 20 differing pixels:")

	count := 0
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			totalPixels++
			ourColor := ourImg.At(x, y)
			popplerColor := popplerImg.At(x, y)

			ourR, ourG, ourB, ourA := ourColor.RGBA()
			popR, popG, popB, popA := popplerColor.RGBA()

			diffR := abs(int(ourR>>8) - int(popR>>8))
			diffG := abs(int(ourG>>8) - int(popG>>8))
			diffB := abs(int(ourB>>8) - int(popB>>8))
			diffA := abs(int(ourA>>8) - int(popA>>8))

			totalDiff += diffR + diffG + diffB + diffA

			if diffR > 0 || diffG > 0 || diffB > 0 || diffA > 0 {
				differentPixels++
				if count < 20 {
					fmt.Printf("(%d,%d): Our[%3d,%3d,%3d,%3d] Poppler[%3d,%3d,%3d,%3d] Diff[%d,%d,%d,%d]\n",
						x, y,
						ourR>>8, ourG>>8, ourB>>8, ourA>>8,
						popR>>8, popG>>8, popB>>8, popA>>8,
						diffR, diffG, diffB, diffA)
					count++
				}
			}
		}
	}

	fmt.Printf("\nSummary:\n")
	fmt.Printf("Total pixels: %d\n", totalPixels)
	fmt.Printf("Different pixels: %d (%.2f%%)\n", differentPixels, float64(differentPixels)*100/float64(totalPixels))
	fmt.Printf("Average diff per pixel: %.2f\n", float64(totalDiff)/float64(totalPixels))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
