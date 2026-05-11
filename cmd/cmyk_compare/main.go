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
	if len(os.Args) < 2 {
		fmt.Println("Usage: cmyk_compare <pdf_file> <poppler_png>")
		return
	}

	pdfPath := os.Args[1]
	popplerPath := os.Args[2]

	// Render our version
	doc, err := pdf.Open(pdfPath)
	if err != nil {
		fmt.Printf("Error opening PDF: %v\n", err)
		return
	}
	defer doc.Close()

	page, err := doc.Page(0)
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

	fmt.Printf("Our size: %v\n", ourImg.Bounds())
	fmt.Printf("Poppler size: %v\n", popplerImg.Bounds())

	// Compare center pixels
	ourBounds := ourImg.Bounds()

	centerX := ourBounds.Dx() / 2
	centerY := ourBounds.Dy() / 2

	fmt.Printf("\nCenter pixels comparison:\n")
	for dy := -5; dy <= 5; dy++ {
		for dx := -5; dx <= 5; dx++ {
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

			if diffR > 10 || diffG > 10 || diffB > 10 {
				fmt.Printf("(%d,%d): Our[%3d,%3d,%3d] Poppler[%3d,%3d,%3d] Diff[%d,%d,%d]\n",
					x, y, ourR>>8, ourG>>8, ourB>>8, popR>>8, popG>>8, popB>>8, diffR, diffG, diffB)
			}
		}
	}

	// Count total differences
	totalPixels := ourBounds.Dx() * ourBounds.Dy()
	differentPixels := 0
	totalDiff := 0

	for y := 0; y < ourBounds.Dy(); y++ {
		for x := 0; x < ourBounds.Dx(); x++ {
			ourColor := ourImg.At(x, y)
			popColor := popplerImg.At(x, y)

			ourR, ourG, ourB, _ := ourColor.RGBA()
			popR, popG, popB, _ := popColor.RGBA()

			diffR := abs(int(ourR>>8) - int(popR>>8))
			diffG := abs(int(ourG>>8) - int(popG>>8))
			diffB := abs(int(ourB>>8) - int(popB>>8))

			totalDiff += diffR + diffG + diffB

			if diffR > 0 || diffG > 0 || diffB > 0 {
				differentPixels++
			}
		}
	}

	fmt.Printf("\nSummary:\n")
	fmt.Printf("Total pixels: %d\n", totalPixels)
	fmt.Printf("Different pixels: %d (%.2f%%)\n", differentPixels, float64(differentPixels)*100/float64(totalPixels))
	fmt.Printf("Average diff per pixel: %.2f\n", float64(totalDiff)/float64(totalPixels))
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
