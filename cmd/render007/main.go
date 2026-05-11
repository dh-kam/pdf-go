package main

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"

	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func main() {
	doc, err := pdf.Open("test/testdata/sample-files/007-imagemagick-images/imagemagick-images.pdf")
	if err != nil {
		log.Fatal(err)
	}
	defer doc.Close()

	page, err := doc.Page(0)
	if err != nil {
		log.Fatal(err)
	}

	renderer := pdf.NewRenderer(pdf.DefaultRendererOptions())
	options := pdf.DefaultRenderOptions()
	options.DPI = 150
	img, err := renderer.RenderPage(context.Background(), page, options)
	if err != nil {
		log.Fatal(err)
	}

	out, err := os.Create("/tmp/render007_page1.png")
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(out, img); err != nil {
		out.Close()
		log.Fatal(err)
	}
	if err := out.Close(); err != nil {
		log.Fatal(err)
	}

	rgba := img.(*image.RGBA)
	bounds := rgba.Bounds()
	fmt.Printf("Image size: %dx%d\n", bounds.Dx(), bounds.Dy())
	fmt.Println("All pixel values (R channel, row by row):")
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := rgba.RGBAAt(x, y)
			fmt.Printf("%3d ", c.R)
		}
		fmt.Println()
	}
}
