package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	_ "image/png"
	"os"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func main() {
	dpiFlag := flag.Float64("dpi", 150.0, "DPI for rendering")
	backendFlag := flag.String("backend", "image-canvas", "Renderer backend (image-canvas, splash)")

	flag.Parse()

	args := flag.Args()
	if len(args) < 3 {
		fmt.Println("Usage: compare_pixels [-dpi <dpi>] [-backend <backend>] <pdf_file> <page_number> <poppler_png>")
		return
	}

	pdfPath := args[0]
	pageNum := 0
	fmt.Sscanf(args[1], "%d", &pageNum)
	popplerPath := args[2]

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

	renderOpts := pdf.DefaultRendererOptions()
	renderOpts.Backend = *backendFlag
	if backend := os.Getenv("PDF_BACKEND"); backend != "" {
		renderOpts.Backend = backend
	}
	r := pdf.NewRenderer(renderOpts)

	opts := pdf.DefaultRenderOptions()
	opts.DPI = *dpiFlag

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

			ourR, ourG, ourB, _ := ourColor.RGBA()
			popR, popG, popB, _ := popplerColor.RGBA()

			diffR := abs(int(ourR>>8) - int(popR>>8))
			diffG := abs(int(ourG>>8) - int(popG>>8))
			diffB := abs(int(ourB>>8) - int(popB>>8))

			totalDiff += diffR + diffG + diffB

			if diffR > 0 || diffG > 0 || diffB > 0 {
				differentPixels++
				if count < 20 {
					fmt.Printf("(%d,%d): Our[%3d,%3d,%3d] Poppler[%3d,%3d,%3d] Diff[%d,%d,%d]\n",
						x, y,
						ourR>>8, ourG>>8, ourB>>8,
						popR>>8, popG>>8, popB>>8,
						diffR, diffG, diffB)
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
