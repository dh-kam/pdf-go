package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
)

func analyzeDetailed(popplerPath, oursPath string) {
	popplerImg := loadImage(popplerPath)
	oursImg := loadImage(oursPath)

	bounds := popplerImg.Bounds()

	type region struct {
		minX, minY, maxX, maxY int
		count                  int
		maxDiff                float64
	}

	// Find bounding box of all diffs
	var minX, minY, maxX, maxY int
	minX, minY = bounds.Max.X, bounds.Max.Y
	maxX, maxY = 0, 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pr, pg, pb, _ := popplerImg.At(x, y).RGBA()
			or_, og, ob, _ := oursImg.At(x, y).RGBA()

			pr8 := pr >> 8
			pg8 := pg >> 8
			pb8 := pb >> 8
			or8 := or_ >> 8
			og8 := og >> 8
			ob8 := ob >> 8

			if pr8 != or8 || pg8 != og8 || pb8 != ob8 {
				if x < minX {
					minX = x
				}
				if x > maxX {
					maxX = x
				}
				if y < minY {
					minY = y
				}
				if y > maxY {
					maxY = y
				}
			}
		}
	}

	fmt.Printf("Diff bounding box: (%d,%d)-(%d,%d) [%dx%d]\n", minX, minY, maxX, maxY, maxX-minX+1, maxY-minY+1)

	// Now look at rows where diffs occur and print a mini-map
	type rowSummary struct {
		y      int
		count  int
		xRanges [][2]int
	}

	rowSummaries := make(map[int]*rowSummary)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			pr, pg, pb, _ := popplerImg.At(x, y).RGBA()
			or_, og, ob, _ := oursImg.At(x, y).RGBA()

			pr8 := pr >> 8
			pg8 := pg >> 8
			pb8 := pb >> 8
			or8 := or_ >> 8
			og8 := og >> 8
			ob8 := ob >> 8

			if pr8 != or8 || pg8 != og8 || pb8 != ob8 {
				if _, exists := rowSummaries[y]; !exists {
					rowSummaries[y] = &rowSummary{y: y}
				}
				rs := rowSummaries[y]
				rs.count++
				if len(rs.xRanges) == 0 || x > rs.xRanges[len(rs.xRanges)-1][1]+5 {
					rs.xRanges = append(rs.xRanges, [2]int{x, x})
				} else {
					rs.xRanges[len(rs.xRanges)-1][1] = x
				}
			}
		}
	}

	// Print row summaries
	fmt.Printf("Rows with diffs:\n")
	for y := minY; y <= maxY; y++ {
		if rs, ok := rowSummaries[y]; ok {
			fmt.Printf("  y=%d: %d diff pixels, x-ranges: %v\n", y, rs.count, rs.xRanges)
		}
	}

	// Save a zoomed image of the diff region
	if maxX > minX && maxY > minY {
		pad := 20
		x1 := minX - pad
		if x1 < 0 {
			x1 = 0
		}
		y1 := minY - pad
		if y1 < 0 {
			y1 = 0
		}
		x2 := maxX + pad
		if x2 > bounds.Max.X {
			x2 = bounds.Max.X
		}
		y2 := maxY + pad
		if y2 > bounds.Max.Y {
			y2 = bounds.Max.Y
		}

		// Save crop of poppler and ours
		saveCrop(popplerImg, x1, y1, x2, y2, "/tmp/crop_poppler.png")
		saveCrop(oursImg, x1, y1, x2, y2, "/tmp/crop_ours.png")
		fmt.Printf("Saved crops to /tmp/crop_poppler.png and /tmp/crop_ours.png (region: %d,%d to %d,%d)\n", x1, y1, x2, y2)
	}
}

func saveCrop(img image.Image, x1, y1, x2, y2 int, path string) {
	w := x2 - x1
	h := y2 - y1
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := y1; y < y2; y++ {
		for x := x1; x < x2; x++ {
			out.Set(x-x1, y-y1, img.At(x, y))
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	_ = png.Encode(f, out)
}
