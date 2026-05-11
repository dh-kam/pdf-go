package main

import (
	"fmt"
	"os"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/truetype"
)

func main() {
	data, err := os.ReadFile("/tmp/XGBNKK.ttf")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	font, err := truetype.NewFontFromBytes(data)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	var intf entity.Font = font
	renderer, ok := intf.(entity.BitmapGlyphRendererPhased)
	fmt.Println("TrueType implements BitmapGlyphRendererPhased:", ok)
	if !ok {
		return
	}

	// Test rendering
	buf, w, h, left, top, err2 := renderer.RenderGlyphBitmapPhased(75, 14, 150, 0.0, 0.0)
	fmt.Printf("Phased render GID 75: buf=%d, w=%d, h=%d, left=%d, top=%d, err=%v\n", len(buf), w, h, left, top, err2)
}
