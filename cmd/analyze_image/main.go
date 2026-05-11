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
		fmt.Println("Usage: analyze_image <pdf_file> <page_number> <poppler_png>")
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

	fmt.Printf("=== Image Grid Analysis (8x8) ===\n")
	fmt.Printf("Format: Our | Poppler | Diff\n\n")

	// Print full grid comparison
	fmt.Printf("   Y=0     Y=1     Y=2     Y=3     Y=4     Y=5     Y=6     Y=7\n")
	fmt.Printf("   ---------------------------------------------------------------\n")

	for y := 0; y < ourBounds.Dy(); y++ {
		fmt.Printf("X=%d|", y)
		for x := 0; x < ourBounds.Dx(); x++ {
			ourColor := ourImg.At(x, y)
			popColor := popplerImg.At(x, y)

			ourR, _, _, _ := ourColor.RGBA()
			popR, _, _, _ := popColor.RGBA()

			diff := abs(int(ourR>>8) - int(popR>>8))
			fmt.Printf("%3d|%3d|%2d  ", ourR>>8, popR>>8, diff)
		}
		fmt.Printf("\n")
	}

	fmt.Printf("\n=== Pattern Analysis ===\n")
	// Check if there's a transpose or swap pattern
	fmt.Printf("\nLooking for transpose/swap patterns...\n")

	// Check if Our(x,y) matches Poppler(y,x) or other patterns
	fmt.Printf("\nChecking if Our(x,y) == Poppler(7-y, x):\n")
	matches := 0
	for y := 0; y < ourBounds.Dy(); y++ {
		for x := 0; x < ourBounds.Dx(); x++ {
			ourColor := ourImg.At(x, y)
			ourR, _, _, _ := ourColor.RGBA()

			popX := 7 - y
			popY := x
			if popX >= 0 && popX < ourBounds.Dx() && popY >= 0 && popY < ourBounds.Dy() {
				popColor := popplerImg.At(popX, popY)
				popR, _, _, _ := popColor.RGBA()

				diff := abs(int(ourR>>8) - int(popR>>8))
				if diff <= 5 {
					matches++
				}
			}
		}
	}
	fmt.Printf("Matches: %d / 64\n", matches)

	fmt.Printf("\nChecking if Our(x,y) == Poppler(x, 7-y):\n")
	matches = 0
	for y := 0; y < ourBounds.Dy(); y++ {
		for x := 0; x < ourBounds.Dx(); x++ {
			ourColor := ourImg.At(x, y)
			ourR, _, _, _ := ourColor.RGBA()

			popX := x
			popY := 7 - y
			if popX >= 0 && popX < ourBounds.Dx() && popY >= 0 && popY < ourBounds.Dy() {
				popColor := popplerImg.At(popX, popY)
				popR, _, _, _ := popColor.RGBA()

				diff := abs(int(ourR>>8) - int(popR>>8))
				if diff <= 5 {
					matches++
				}
			}
		}
	}
	fmt.Printf("Matches: %d / 64\n", matches)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
