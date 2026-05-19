package pdf_test

import (
	"context"
	"fmt"
	"image"
	"testing"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func TestDebug007Render(t *testing.T) {
	doc, err := pdf.Open("testdata/007-imagemagick-images/imagemagick-images.pdf")
	if err != nil {
		t.Skip("no file:", err)
	}
	defer doc.Close()

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	page, err := doc.Page(1)
	if err != nil {
		t.Fatal(err)
	}

	img, err := renderer.RenderPage(context.Background(), page, pdf.RenderOptions{DPI: 150})
	if err != nil {
		t.Fatal(err)
	}

	rgba, ok := img.(*image.RGBA)
	if !ok {
		t.Fatalf("not RGBA: %T", img)
	}

	bounds := rgba.Bounds()
	fmt.Printf("Image size: %dx%d\n", bounds.Dx(), bounds.Dy())
	fmt.Println("R channel row by row:")
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			fmt.Printf("%3d ", rgba.RGBAAt(x, y).R)
		}
		fmt.Println()
	}

	// Poppler reference
	poppler := [][]int{
		{0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 0, 0, 0, 0, 0, 0},
		{0, 0, 63, 0, 0, 0, 0, 63},
		{0, 0, 0, 0, 255, 0, 0, 0},
		{0, 0, 0, 0, 255, 0, 0, 0},
		{0, 0, 0, 0, 127, 0, 0, 0},
		{0, 0, 127, 0, 0, 0, 191, 0},
		{0, 0, 63, 127, 127, 127, 63, 0},
	}

	fmt.Println("\nDiff (ours - poppler) R channel:")
	var totalDiff int
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			ours := int(rgba.RGBAAt(x, y).R)
			pop := poppler[y][x]
			diff := ours - pop
			if diff < 0 {
				diff = -diff
			}
			totalDiff += diff * 3
			fmt.Printf("%4d ", ours-poppler[y][x])
		}
		fmt.Println()
	}
	fmt.Printf("\nTotal pixel diff: %d / %d = similarity %.2f%%\n", totalDiff, 64*255*3, 100.0*(1.0-float64(totalDiff)/float64(64*255*3)))
}
