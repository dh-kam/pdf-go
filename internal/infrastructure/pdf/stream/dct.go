package stream

import (
	"bytes"
	"fmt"
	"image/color"
	"image/jpeg"
)

func init() {
	RegisterDecoder(FilterDCT, &DCTFactory{})
}

// DCTFactory creates DCT (JPEG) decoders.
type DCTFactory struct{}

// CreateDecoder creates a new DCT decoder.
func (f *DCTFactory) CreateDecoder() (Decoder, error) {
	return &DCTDecoder{}, nil
}

// DCTDecoder implements DCTDecode (JPEG) decompression.
type DCTDecoder struct{}

// Decode decodes JPEG-compressed data to raw pixel bytes.
// The output format is determined by the image's color model.
func (d *DCTDecoder) Decode(data []byte) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("dct decode: %w", err)
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	switch img.ColorModel() {
	case color.GrayModel:
		buf := make([]byte, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				g := color.GrayModel.Convert(img.At(bounds.Min.X+x, bounds.Min.Y+y)).(color.Gray)
				buf[y*w+x] = g.Y
			}
		}
		return buf, nil
	default:
		// RGB / CMYK — emit as 3-byte RGB
		buf := make([]byte, w*h*3)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
				off := (y*w + x) * 3
				buf[off] = byte(r >> 8)
				buf[off+1] = byte(g >> 8)
				buf[off+2] = byte(b >> 8)
			}
		}
		return buf, nil
	}
}
