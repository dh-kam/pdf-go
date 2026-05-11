package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"

	"github.com/dh-kam/pdf-go/internal/infrastructure/canvas"
	"github.com/dh-kam/pdf-go/internal/infrastructure/font/truetype"
)

func main() {
	data, err := os.ReadFile("/tmp/XGBNKK.ttf")
	if err != nil {
		fmt.Println("Error reading font:", err)
		return
	}

	font, err := truetype.NewFontFromBytes(data)
	if err != nil {
		fmt.Println("Error creating font:", err)
		return
	}

	fmt.Println("Font created successfully")
	fmt.Println("Units per em:", font.UnitsPerEm())

	// Try to render GIDs 68, 69, 75, 76
	for _, gid := range []uint32{68, 69, 75, 76} {
		path, err := font.RenderGlyph(gid, 100)
		if err != nil {
			fmt.Printf("GID %d: RenderGlyph error: %v\n", gid, err)
		} else if path == nil {
			fmt.Printf("GID %d: nil path\n", gid)
		} else {
			fmt.Printf("GID %d: %d path commands\n", gid, len(path.Commands))
		}

		w, werr := font.GetGlyphWidth(gid)
		fmt.Printf("GID %d width: %v (err: %v)\n", gid, w, werr)
		fmt.Println()
	}

	// Test rendering individual path
	path75, err := font.RenderGlyph(75, 20)
	if err != nil {
		fmt.Printf("RenderGlyph(75, 20) error: %v\n", err)
	} else if path75 == nil {
		fmt.Println("RenderGlyph(75, 20) returned nil path")
	} else {
		fmt.Printf("RenderGlyph(75, 20): %d commands\n", len(path75.Commands))
		for i, cmd := range path75.Commands[:min(5, len(path75.Commands))] {
			fmt.Printf("  cmd[%d]: type=%v %v\n", i, cmd.Type(), cmd)
		}
	}

	// Render to a canvas
	c := canvas.NewImageCanvas(image.Rect(0, 0, 400, 150))
	c.SetFillColor(color.White)
	c.Rectangle(0, 0, 400, 150)
	c.Fill()
	c.SetFillColor(color.Black)

	// Draw single glyph
	if err := c.DrawText("\x4b", 10, 100, font, 20); err != nil {
		fmt.Println("DrawText error:", err)
	}

	out, _ := os.Create("/tmp/test_render.png")
	defer out.Close()
	_ = png.Encode(out, c.Image())
	fmt.Println("Rendered to /tmp/test_render.png")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
