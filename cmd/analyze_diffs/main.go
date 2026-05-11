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
		fmt.Println("Usage: analyze_diffs <pdf_file> <page_number> <poppler_png>")
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

	// Analyze difference patterns
	fmt.Printf("=== Difference Pattern Analysis ===\n")

	// Count different brightness levels
	ourBright := 0       // Our pixels brighter than Poppler
	ourDark := 0         // Our pixels darker than Poppler
	popplerBlack := 0    // Poppler has black, we don't
	ourBlack := 0        // We have black, Poppler doesn't
	bothBlack := 0       // Both have black
	bothWhite := 0       // Both have white

	// Histogram of Poppler colors when we have white
	popplerColorWhenOurWhite := make(map[[3]int]int)

	for y := 0; y < ourBounds.Dy(); y += 10 {
		for x := 0; x < ourBounds.Dx(); x += 10 {
			ourColor := ourImg.At(x, y)
			popColor := popplerImg.At(x, y)

			ourR, ourG, ourB, _ := ourColor.RGBA()
			popR, popG, popB, _ := popColor.RGBA()

			ourBrightness := (ourR + ourG + ourB) / 3
			popBrightness := (popR + popG + popB) / 3

			if ourBrightness > popBrightness {
				ourBright++
			} else if ourBrightness < popBrightness {
				ourDark++
			}

			// Check black pixel patterns
			ourIsBlack := ourR < 2560 && ourG < 2560 && ourB < 2560 // < 10/255
			popIsBlack := popR < 2560 && popG < 2560 && popB < 2560

			if ourIsBlack && popIsBlack {
				bothBlack++
			} else if ourIsBlack && !popIsBlack {
				ourBlack++
			} else if !ourIsBlack && popIsBlack {
				popplerBlack++
			}

			ourIsWhite := ourR > 65025 && ourG > 65025 && ourB > 65025 // > 255/256
			popIsWhite := popR > 65025 && popG > 65025 && popB > 65025

			if ourIsWhite && popIsWhite {
				bothWhite++
			}

			// Track Poppler colors when we have white
			if ourIsWhite && !popIsWhite {
				key := [3]int{int(popR >> 8), int(popG >> 8), int(popB >> 8)}
				popplerColorWhenOurWhite[key]++
			}
		}
	}

	fmt.Printf("\nBrightness comparison:\n")
	fmt.Printf("  Our brighter than Poppler: %d\n", ourBright)
	fmt.Printf("  Our darker than Poppler: %d\n", ourDark)

	fmt.Printf("\nBlack pixel patterns (threshold < 10):\n")
	fmt.Printf("  Both have black: %d\n", bothBlack)
	fmt.Printf("  Poppler has black, we don't: %d\n", popplerBlack)
	fmt.Printf("  We have black, Poppler doesn't: %d\n", ourBlack)

	fmt.Printf("\nWhite pixel patterns (threshold > 255):\n")
	fmt.Printf("  Both have white: %d\n", bothWhite)

	fmt.Printf("\nTop 10 Poppler colors when we have white:\n")
	count := 0
	for color, n := range popplerColorWhenOurWhite {
		if count >= 10 {
			break
		}
		fmt.Printf("  RGB[%d,%d,%d]: %d times\n", color[0], color[1], color[2], n)
		count++
	}
}
