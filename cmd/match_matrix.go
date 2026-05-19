//go:build ignore

package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"github.com/dh-kam/pdf-go/internal/infrastructure/canvas"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
)

func countDiff(a, b *image.RGBA) int {
	c := 0
	for y := a.Bounds().Min.Y; y < a.Bounds().Max.Y; y++ {
		for x := a.Bounds().Min.X; x < a.Bounds().Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				c++
			}
		}
	}
	return c
}

func transformPoint(m [6]float64, x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

func renderExpr(t [6]float64, c *canvas.ImageCanvas, img image.Image, x, y, w, h, phaseX, phaseY float64) *image.RGBA {
	x0, y0 := transformPoint(t, x, y)
	x1, y1 := transformPoint(t, x+w, y)
	x2, y2 := transformPoint(t, x, y+h)

	sx := float64(img.Bounds().Dx())
	sy := float64(img.Bounds().Dy())
	scaleX := (x1 - x0) / sx
	scaleY := (y1 - y0) / sx
	shearX := (x2 - x0) / sy
	shearY := (y2 - y0) / sy

	exp := image.NewRGBA(c.Bounds())
	dest := f64.Aff3{scaleX, shearX, x0 + scaleX*phaseX + shearX*phaseY, -scaleY, -shearY, float64(c.Height()) - y0 - (scaleY*phaseX + shearY*phaseY)}
	xdraw.NearestNeighbor.Transform(exp, dest, img, img.Bounds(), draw.Src, nil)
	return exp
}

func main() {
	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			src.SetRGBA(x, y, color.RGBA{uint8(x * 16), uint8(y * 16), 0, 255})
		}
	}

	cases := []struct {
		name string
		t    [6]float64
	}{
		{"id", [6]float64{1, 0, 0, 1, 0, 0}},
		{"rot90", [6]float64{0, 1, -1, 0, 0, 0}},
		{"rot", [6]float64{1, 0.4, 0.35, 1, 2, 3}},
	}

	for _, c := range cases {
		canv := canvas.NewImageCanvas(image.Rect(0, 0, 64, 64)).(*canvas.ImageCanvas)
		canv.Transform(c.t)
		if err := canv.DrawImageWithPhase(src, 0, 0, float64(src.Bounds().Dx()), float64(src.Bounds().Dy()), false, 0.5, 0.5); err != nil {
			panic(err)
		}
		actual := canv.Image().(*image.RGBA)
		expected := renderExpr(c.t, canv, src, 0, 0, float64(src.Bounds().Dx()), float64(src.Bounds().Dy()), 0.5, 0.5)
		fmt.Println(c.name, "diff", countDiff(actual, expected), "bbox", actual.Bounds())
	}

	cc := canvas.NewImageCanvas(image.Rect(0, 0, 64, 64)).(*canvas.ImageCanvas)
	t := [6]float64{1, 0.4, 0.35, 1, 2, 3}
	cc.Transform(t)
	if err := cc.DrawImageWithPhase(src, 0, 0, float64(src.Bounds().Dx()), float64(src.Bounds().Dy()), false, 0.5, 0.5); err != nil {
		panic(err)
	}
	actual := cc.Image().(*image.RGBA)
	nz := 0
	for i := 3; i < len(actual.Pix); i += 4 {
		if actual.Pix[i] != 0 {
			nz++
		}
	}
	fmt.Println("non-transparent pixels", nz)
}
