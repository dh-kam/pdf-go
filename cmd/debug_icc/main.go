package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	
	domainimage "github.com/dh-kam/pdf-go/internal/domain/image"
	infraimage "github.com/dh-kam/pdf-go/internal/infrastructure/image"
)

func main() {
	// Read the actual ICC profile from the PDF
	iccData := readICCFromPDF()
	
	srcData := []byte{
		0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
		0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
		0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
		0,0,0,255,0,0,0,0,0,0,0,0,255,0,0,0,
		0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
		0,0,0,0,0,0,0,255,0,0,0,0,0,0,0,0,
		0,0,0,0,0,0,0,255,0,0,0,0,0,0,0,0,
		0,0,0,0,0,0,0,255,0,0,0,0,0,0,0,0,
		0,0,0,0,0,0,0,255,0,0,0,0,0,0,0,0,
		0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
		0,0,0,255,0,0,0,0,0,0,0,255,0,0,0,0,
		0,0,0,255,0,0,0,0,0,0,255,255,0,0,0,0,
		0,0,0,0,255,255,255,255,255,255,255,0,0,0,0,0,
		0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
		0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
		0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,
	}
	
	decoder := infraimage.NewDecoder()
	
	// With ICCBased
	imgData2 := &domainimage.ImageData{
		Data: srcData, Width: 16, Height: 16, BitsPerComponent: 8,
		ColorSpace: "DeviceGray",
		ICCProfile: iccData, ICCComponents: 1,
		Filter: domainimage.FilterNone,
	}
	decoded2, err := decoder.Decode(imgData2)
	if err != nil { fmt.Println("Error2:", err); return }
	img2 := decoded2.Image()
	fmt.Println("ICCBased gray (components=1) non-zero pixels:")
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			r, _, _, _ := img2.At(x, y).RGBA()
			v := r >> 8
			if v != 0 {
				fmt.Printf("  (%d,%d) = %d\n", x, y, v)
			}
		}
	}
	
	// Now simulate 8x8 box downscale
	fmt.Println("8x8 box downscale of ICCBased result:")
	for dy := 0; dy < 8; dy++ {
		for dx := 0; dx < 8; dx++ {
			var sum uint64
			for sy := dy*2; sy < dy*2+2; sy++ {
				for sx := dx*2; sx < dx*2+2; sx++ {
					r, _, _, _ := img2.At(sx, sy).RGBA()
					sum += uint64(r >> 8)
				}
			}
			avg := sum / 4
			if avg > 0 {
				fmt.Printf("  (%d,%d) = %d\n", dx, dy, avg)
			}
		}
	}
	
	// White-background version: render on white background (flip: 0->255, 255->0)
	fmt.Println("Simulating inverted rendering:")
	invImg := image.NewGray(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			v := srcData[y*16+x]
			invImg.SetGray(x, y, color.Gray{Y: 255 - v})
		}
	}
	fmt.Println("8x8 box downscale of INVERTED source:")
	for dy := 0; dy < 8; dy++ {
		for dx := 0; dx < 8; dx++ {
			var sum uint64
			for sy := dy*2; sy < dy*2+2; sy++ {
				for sx := dx*2; sx < dx*2+2; sx++ {
					r, _, _, _ := invImg.At(sx, sy).RGBA()
					sum += uint64(r >> 8)
				}
			}
			avg := sum / 4
			if avg != 255 {
				fmt.Printf("  (%d,%d) = %d\n", dx, dy, avg)
			}
		}
	}
	_ = png.Encode; _ = os.Create
}

func readICCFromPDF() []byte {
	// Return nil for now - we'll add this if needed
	return nil
}
