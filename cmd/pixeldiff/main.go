package main

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"sort"
)

type pixelDiff struct {
	x, y       int
	popplerR   uint32
	popplerG   uint32
	popplerB   uint32
	oursR      uint32
	oursG      uint32
	oursB      uint32
	diff       float64
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: pixeldiff <poppler.png> <ours.png>")
		os.Exit(1)
	}

	popplerPath := os.Args[1]
	oursPath := os.Args[2]

	popplerImg := loadImage(popplerPath)
	oursImg := loadImage(oursPath)

	bounds := popplerImg.Bounds()
	oursBounds := oursImg.Bounds()

	fmt.Printf("Poppler size: %dx%d\n", bounds.Max.X, bounds.Max.Y)
	fmt.Printf("Ours size: %dx%d\n", oursBounds.Max.X, oursBounds.Max.Y)

	if bounds != oursBounds {
		fmt.Println("WARNING: sizes differ!")
	}

	totalPixels := 0
	diffPixels := 0
	var diffs []pixelDiff

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			totalPixels++
			pr, pg, pb, _ := popplerImg.At(x, y).RGBA()
			or_, og, ob, _ := oursImg.At(x, y).RGBA()

			// Convert from 16-bit to 8-bit
			pr8 := pr >> 8
			pg8 := pg >> 8
			pb8 := pb >> 8
			or8 := or_ >> 8
			og8 := og >> 8
			ob8 := ob >> 8

			if pr8 != or8 || pg8 != og8 || pb8 != ob8 {
				diffPixels++
				d := math.Sqrt(float64((pr8-or8)*(pr8-or8)+(pg8-og8)*(pg8-og8)+(pb8-ob8)*(pb8-ob8)))
				if len(diffs) < 50 {
					diffs = append(diffs, pixelDiff{x, y, pr8, pg8, pb8, or8, og8, ob8, d})
				}
			}
		}
	}

	fmt.Printf("Total pixels: %d\n", totalPixels)
	fmt.Printf("Diff pixels: %d (%.4f%%)\n", diffPixels, float64(diffPixels)/float64(totalPixels)*100)

	// Sort diffs by y then x
	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].y != diffs[j].y {
			return diffs[i].y < diffs[j].y
		}
		return diffs[i].x < diffs[j].x
	})

	fmt.Println("\nFirst 50 differing pixels:")
	fmt.Println("x\ty\tPoppler(R,G,B)\tOurs(R,G,B)\tDiff")
	for _, d := range diffs {
		fmt.Printf("%d\t%d\t(%d,%d,%d)\t(%d,%d,%d)\t%.1f\n",
			d.x, d.y,
			d.popplerR, d.popplerG, d.popplerB,
			d.oursR, d.oursG, d.oursB,
			d.diff)
	}

	// Analyze y ranges to understand what regions differ
	if len(diffs) > 0 {
		fmt.Printf("\nY range of first diffs: %d to %d\n", diffs[0].y, diffs[len(diffs)-1].y)
	}

	fmt.Println("\n--- Detailed analysis ---")
	analyzeDetailed(popplerPath, oursPath)
}

func loadImage(path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		fmt.Printf("Error opening %s: %v\n", path, err)
		os.Exit(1)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		fmt.Printf("Error decoding %s: %v\n", path, err)
		os.Exit(1)
	}
	return img
}
