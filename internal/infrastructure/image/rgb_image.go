package image

import (
	stdimage "image"
	"image/color"
)

// RGBImage stores an opaque RGB image as packed 8-bit RGB rows.
type RGBImage struct {
	Pix    []byte
	Stride int
	Rect   stdimage.Rectangle
}

// NewRGBImage creates an RGBImage backed by pix.
func NewRGBImage(rect stdimage.Rectangle, pix []byte, stride int) *RGBImage {
	return &RGBImage{
		Pix:    pix,
		Stride: stride,
		Rect:   rect,
	}
}

// ColorModel returns the image color model.
func (i *RGBImage) ColorModel() color.Model {
	return color.RGBAModel
}

// Bounds returns the image bounds.
func (i *RGBImage) Bounds() stdimage.Rectangle {
	if i == nil {
		return stdimage.Rectangle{}
	}
	return i.Rect
}

// At returns the color of the pixel at x, y.
func (i *RGBImage) At(x, y int) color.Color {
	if i == nil || !(stdimage.Point{X: x, Y: y}.In(i.Rect)) {
		return color.RGBA{}
	}
	off := (y-i.Rect.Min.Y)*i.Stride + (x-i.Rect.Min.X)*3
	if off < 0 || off+2 >= len(i.Pix) {
		return color.RGBA{}
	}
	return color.RGBA{R: i.Pix[off], G: i.Pix[off+1], B: i.Pix[off+2], A: 0xff}
}

// RGB8Data returns packed RGB bytes and row stride.
func (i *RGBImage) RGB8Data() ([]byte, int) {
	if i == nil {
		return nil, 0
	}
	return i.Pix, i.Stride
}
