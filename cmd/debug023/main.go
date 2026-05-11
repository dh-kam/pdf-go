package main

import (
	"context"
	"fmt"
	"image/png"
	"os"

	"github.com/dh-kam/pdf-go/internal/domain/renderer"
	"github.com/dh-kam/pdf-go/pkg/pdf"
)

func main() {
	doc, err := pdf.Open("/workspace/pdf-reader/go-pdf/test/integration/pdf/testdata/023-cmyk-image/cmyk-image.pdf")
	if err != nil {
		fmt.Println("Error opening:", err)
		os.Exit(1)
	}
	defer doc.Close()

	page, err := doc.Page(0)
	if err != nil {
		fmt.Println("Error getting page:", err)
		os.Exit(1)
	}

	r := pdf.NewRenderer(pdf.DefaultRendererOptions())
	opts := pdf.DefaultRenderOptions()
	opts.DPI = 150
	opts.ImageSamplingMode = renderer.ImageSamplingModeExperimentalIndexedCMYKLUTV1

	img, err := r.RenderPage(context.Background(), page, opts)
	if err != nil {
		fmt.Println("Error rendering:", err)
		os.Exit(1)
	}

	f, _ := os.Create("/tmp/debug_render.png")
	png.Encode(f, img)
	f.Close()
	fmt.Fprintf(os.Stderr, "Rendered image size: %v\n", img.Bounds())
}
