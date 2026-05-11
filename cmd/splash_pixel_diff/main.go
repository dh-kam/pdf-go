// Command splash_pixel_diff compares two PNG files pixel-by-pixel and reports
// statistics + the first 10 mismatches with 3x3 neighborhoods.
//
// Usage: splash_pixel_diff <reference.png> <actual.png>
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	return img, nil
}

func rgba(c color.Color) (uint8, uint8, uint8, uint8) {
	r, g, b, a := c.RGBA()
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)
}

func absDelta(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: splash_pixel_diff <reference.png> <actual.png>")
		os.Exit(2)
	}
	refPath, actPath := os.Args[1], os.Args[2]
	refImg, err := loadPNG(refPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load reference: %v\n", err)
		os.Exit(2)
	}
	actImg, err := loadPNG(actPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load actual: %v\n", err)
		os.Exit(2)
	}

	rb := refImg.Bounds()
	ab := actImg.Bounds()
	fmt.Printf("== %s vs %s ==\n", refPath, actPath)
	fmt.Printf("ref bounds: %v  type=%T\n", rb, refImg)
	fmt.Printf("act bounds: %v  type=%T\n", ab, actImg)
	if rb != ab {
		fmt.Printf("BOUNDS MISMATCH\n")
	}

	w := rb.Dx()
	h := rb.Dy()
	total := w * h
	identical := 0
	off1 := 0
	off2 := 0
	type mm struct {
		x, y                       int
		ar, ag, ab, aa             uint8
		rr, rg, rb, ra             uint8
		dr, dg, dbb, da            int
	}
	var mismatches []mm
	rowMM := make(map[int]int)
	colMM := make(map[int]int)
	deltaHist := make(map[int]int)

	for y := rb.Min.Y; y < rb.Max.Y; y++ {
		for x := rb.Min.X; x < rb.Max.X; x++ {
			rR, rG, rB, rA := rgba(refImg.At(x, y))
			aR, aG, aB, aA := rgba(actImg.At(x, y))
			if rR == aR && rG == aG && rB == aB && rA == aA {
				identical++
				continue
			}
			dR := absDelta(rR, aR)
			dG := absDelta(rG, aG)
			dB := absDelta(rB, aB)
			dA := absDelta(rA, aA)
			maxD := dR
			if dG > maxD {
				maxD = dG
			}
			if dB > maxD {
				maxD = dB
			}
			if dA > maxD {
				maxD = dA
			}
			if maxD == 1 {
				off1++
			} else {
				off2++
			}
			if len(mismatches) < 10 {
				mismatches = append(mismatches, mm{x, y, aR, aG, aB, aA, rR, rG, rB, rA, dR, dG, dB, dA})
			}
			rowMM[y]++
			colMM[x]++
			deltaHist[dR]++
		}
	}

	fmt.Printf("total px:    %d\n", total)
	fmt.Printf("identical:   %d (%.4f%%)\n", identical, 100*float64(identical)/float64(total))
	fmt.Printf("off-by-1:    %d\n", off1)
	fmt.Printf("off-by-2+:   %d\n", off2)
	fmt.Printf("mismatched:  %d\n", off1+off2)
	fmt.Println()

	// row distribution
	if len(rowMM) > 0 {
		fmt.Println("rows with mismatches (y: count):")
		// sort keys
		ys := make([]int, 0, len(rowMM))
		for y := range rowMM {
			ys = append(ys, y)
		}
		// simple sort
		for i := 0; i < len(ys); i++ {
			for j := i + 1; j < len(ys); j++ {
				if ys[j] < ys[i] {
					ys[i], ys[j] = ys[j], ys[i]
				}
			}
		}
		for _, y := range ys {
			fmt.Printf("  y=%3d  %d\n", y, rowMM[y])
		}
		fmt.Println("delta histogram (R-channel):")
		ds := make([]int, 0, len(deltaHist))
		for d := range deltaHist {
			ds = append(ds, d)
		}
		for i := 0; i < len(ds); i++ {
			for j := i + 1; j < len(ds); j++ {
				if ds[j] < ds[i] {
					ds[i], ds[j] = ds[j], ds[i]
				}
			}
		}
		for _, d := range ds {
			fmt.Printf("  delta=%3d  %d\n", d, deltaHist[d])
		}
	}

	if len(mismatches) > 0 {
		fmt.Println("first mismatches:")
		for _, m := range mismatches {
			fmt.Printf("  (%3d,%3d) act=#%02X%02X%02X%02X ref=#%02X%02X%02X%02X delta=(%+d,%+d,%+d,%+d)\n",
				m.x, m.y, m.ar, m.ag, m.ab, m.aa, m.rr, m.rg, m.rb, m.ra, m.dr, m.dg, m.dbb, m.da)
			// 3x3 neighborhoods
			fmt.Print("    ref 3x3 R:")
			for dy := -1; dy <= 1; dy++ {
				fmt.Print("  ")
				for dx := -1; dx <= 1; dx++ {
					nr, _, _, _ := rgba(refImg.At(m.x+dx, m.y+dy))
					fmt.Printf("%02X ", nr)
				}
			}
			fmt.Println()
			fmt.Print("    act 3x3 R:")
			for dy := -1; dy <= 1; dy++ {
				fmt.Print("  ")
				for dx := -1; dx <= 1; dx++ {
					nr, _, _, _ := rgba(actImg.At(m.x+dx, m.y+dy))
					fmt.Printf("%02X ", nr)
				}
			}
			fmt.Println()
		}
	}
}
